package manifold

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"os"
	"reflect"
	"testing"
)

func colleafOriginalJPEG(t testing.TB) []byte {
	t.Helper()
	archive, err := zip.OpenReader("testdata/real/mqtt-v5.0-os.docx")
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	for _, f := range archive.File {
		if f.Name == "word/media/image1.jpg" {
			r, err := f.Open()
			if err != nil {
				t.Fatal(err)
			}
			data, err := io.ReadAll(r)
			_ = r.Close()
			if err != nil {
				t.Fatal(err)
			}
			if colleafImageHash(data) != "fe9737d0cb4f905917054c7938fc3ccf4b83f0623df9cd5d62192dbaae26b510" {
				t.Fatal("original independent image oracle changed")
			}
			return data
		}
	}
	t.Fatal("original JPEG absent")
	return nil
}
func colleafImageExample(t testing.TB) *Document {
	t.Helper()
	png := runningPNG(t, false)
	jpeg := colleafOriginalJPEG(t)
	picture := func(data []byte, kind, alt string, w, h float64) Block {
		return Block{Kind: Picture, Image: &Image{Data: data, MediaType: kind, Alt: alt, Width: w, Height: h}}
	}
	p := func(s string) Block { return Block{Kind: Paragraph, Runs: []Run{{Text: s}}} }
	return &Document{Blocks: []Block{
		picture(png, "image/png", "Transparent PNG", 0, 0),
		{Kind: Paragraph, Runs: []Run{{Text: "After 😀"}, {Note: &NoteReference{Kind: Footnote, ID: "n"}}}},
		{Kind: Table, Rows: []Row{{Cells: []Cell{{Blocks: []Block{p("Cell"), picture(png, "image/png", "Cell PNG", 10.1, 0)}}}}}},
		{Kind: ListBlock, List: "bullet", Items: []ListItem{{Blocks: []Block{p("Image item"), picture(jpeg, "image/jpeg", "Original JPEG", 0, 12.3)}}}},
		picture(jpeg, "image/jpeg", "Stretched JPEG", 33.3, 11.1),
	}, Notes: []Note{{Kind: Footnote, ID: "n", Blocks: []Block{picture(png, "image/png", "Note PNG", 7.125, 9.25), p("Image note body")}}}}
}
func TestColleafEmbeddedImagesAllStoriesAndSF(t *testing.T) {
	ctx := context.Background()
	want := colleafImageExample(t)
	wire, err := encodeColleafNotes(ctx, want)
	if err != nil {
		t.Fatal(err)
	}
	if len(wire.Drawings) != 4 || len(wire.Notes["manydoc-note-1"].Drawings) != 1 {
		t.Fatal("image scope ownership lost")
	}
	got, err := decodeColleafNotes(ctx, wire, colleafTextContext{})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		a, _ := json.Marshal(got)
		b, _ := json.Marshal(want)
		t.Fatalf("image semantics lost\ngot%s\nwant%s", a, b)
	}
	sf, err := ToStructured(got)
	if err != nil {
		t.Fatal(err)
	}
	model, err := FromStructured(sf)
	if err != nil {
		t.Fatal(err)
	}
	again, err := encodeColleafNotes(ctx, model)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(wire, again) {
		t.Fatal("SF changed native image bytes/relationships/geometry intent")
	}
	if file := os.Getenv("MANYDOC_COLLEAF_IMAGE_QA"); file != "" {
		data, err := json.MarshalIndent(wire, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(file, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if file := os.Getenv("MANYDOC_COLLEAF_IMAGE_EDIT"); file != "" {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var edited colleafNoteSnapshot
		if err = json.Unmarshal(data, &edited); err != nil {
			t.Fatal(err)
		}
		doc, err := decodeColleafNotes(ctx, edited, colleafTextContext{})
		if err != nil {
			t.Fatal(err)
		}
		if doc.Blocks[0].Image.Width != 48 || doc.Blocks[0].Image.Height != 24 || doc.Blocks[0].Image.Alt != "Resized PNG" {
			t.Fatal("native resize/alt did not win", doc.Blocks[0].Image)
		}
		if len(doc.Diagnostics) != 1 || doc.Diagnostics[0].Code != "colleaf_stale_image_geometry" {
			t.Fatal(doc.Diagnostics)
		}
	}
}
func TestColleafNativeMixedInlineImageRetainsAllContent(t *testing.T) {
	wire, err := encodeColleafNotes(context.Background(), &Document{Blocks: []Block{{Kind: Picture, Image: &Image{Data: runningPNG(t, false), MediaType: "image/png"}}}})
	if err != nil {
		t.Fatal(err)
	}
	id := wire.Body.CustomBlocks[0].ID
	zero := 0
	wire.Body = colleafBody{DataStream: "Left 😀\bRight\r\n", CustomBlocks: []colleafBlockRecord{{StartIndex: 7, Type: &zero, ID: id}}, Paragraphs: []colleafParagraphRecord{{StartIndex: 13, ID: "p"}}, TextRuns: []colleafTextRun{{St: 0, Ed: 7, TS: sfAttrs(map[string]int{"bl": 1})}}}
	doc, err := decodeColleafNotes(context.Background(), wire, colleafTextContext{})
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Blocks) != 3 || doc.Blocks[0].Runs[0].Text != "Left 😀" || !doc.Blocks[0].Runs[0].Bold || doc.Blocks[1].Kind != Picture || doc.Blocks[2].Runs[0].Text != "Right" {
		t.Fatal("inline content lost", doc.Blocks)
	}
	if len(doc.Diagnostics) != 1 || doc.Diagnostics[0].Code != "colleaf_inline_image_reflow" {
		t.Fatal(doc.Diagnostics)
	}
}
func TestColleafImageRejectsBadRelationshipsAppearanceAndBytes(t *testing.T) {
	for _, mutate := range []func(*colleafNoteSnapshot){
		func(w *colleafNoteSnapshot) { delete(w.Drawings, w.Body.CustomBlocks[0].ID) },
		func(w *colleafNoteSnapshot) { w.DrawingOrder = append(w.DrawingOrder, w.DrawingOrder[0]) },
		func(w *colleafNoteSnapshot) {
			for id, raw := range w.Drawings {
				var d colleafNativeImage
				_ = json.Unmarshal(raw, &d)
				d.SourceType = "URL"
				d.Source = "https://example.org/image.png"
				w.Drawings[id] = sfAttrs(d)
				break
			}
		},
		func(w *colleafNoteSnapshot) {
			for id, raw := range w.Drawings {
				var d colleafNativeImage
				_ = json.Unmarshal(raw, &d)
				d.Source = "data:image/png;base64,garbage"
				w.Drawings[id] = sfAttrs(d)
				break
			}
		},
		func(w *colleafNoteSnapshot) {
			for id, raw := range w.Drawings {
				var d colleafNativeImage
				_ = json.Unmarshal(raw, &d)
				d.Transform.Angle = 30
				w.Drawings[id] = sfAttrs(d)
				break
			}
		},
		func(w *colleafNoteSnapshot) {
			for id, raw := range w.Drawings {
				var d colleafNativeImage
				_ = json.Unmarshal(raw, &d)
				d.Hidden = true
				w.Drawings[id] = sfAttrs(d)
				break
			}
		},
		func(w *colleafNoteSnapshot) { w.Drawings["orphan"] = w.Drawings[w.Body.CustomBlocks[0].ID] },
		func(w *colleafNoteSnapshot) {
			for id, raw := range w.Drawings {
				var d colleafNativeImage
				_ = json.Unmarshal(raw, &d)
				d.Cache = "data:image/jpeg;base64,AA=="
				w.Drawings[id] = sfAttrs(d)
				break
			}
		},
	} {
		wire, err := encodeColleafNotes(context.Background(), colleafImageExample(t))
		if err != nil {
			t.Fatal(err)
		}
		mutate(&wire)
		if _, err = decodeColleafNotes(context.Background(), wire, colleafTextContext{}); err == nil {
			t.Fatal("invalid image source/appearance accepted")
		}
	}
}
func TestColleafNativeImageAssetEditInvalidatesOldIntent(t *testing.T) {
	wire, err := encodeColleafNotes(context.Background(), colleafImageExample(t))
	if err != nil {
		t.Fatal(err)
	}
	id := wire.Body.CustomBlocks[0].ID
	var drawing colleafNativeImage
	_ = json.Unmarshal(wire.Drawings[id], &drawing)
	different := runningPNG(t, true)
	drawing.Source = "data:image/png;base64," + base64.StdEncoding.EncodeToString(different)
	wire.Drawings[id] = sfAttrs(drawing)
	doc, err := decodeColleafNotes(context.Background(), wire, colleafTextContext{})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(doc.Blocks[0].Image.Data, different) || doc.Blocks[0].Image.Width != 24 || doc.Blocks[0].Image.Height != 12 {
		t.Fatal("old size intent/bytes won over native asset")
	}
	if len(doc.Diagnostics) != 1 || doc.Diagnostics[0].Code != "colleaf_stale_image_geometry" {
		t.Fatal(doc.Diagnostics)
	}
}

func TestColleafImageAnchorHiddenAndPageBreakPolicies(t *testing.T) {
	makeWire := func() colleafNoteSnapshot {
		wire, err := encodeColleafNotes(context.Background(), &Document{Blocks: []Block{{Kind: Picture, Image: &Image{Data: runningPNG(t, false), MediaType: "image/png"}}}})
		if err != nil {
			t.Fatal(err)
		}
		return wire
	}
	wire := makeWire()
	wire.Body.TextRuns = []colleafTextRun{{St: 0, Ed: 1, TS: json.RawMessage(`{"hidden":true}`)}}
	if _, err := decodeColleafNotes(context.Background(), wire, colleafTextContext{}); err == nil {
		t.Fatal("hidden anchor became visible image")
	}
	wire = makeWire()
	wire.Body.Paragraphs[0].Style = sfAttrs(map[string]int{"pageBreakBefore": 1})
	doc, err := decodeColleafNotes(context.Background(), wire, colleafTextContext{})
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Blocks) != 2 || !doc.Blocks[0].PageBreakBefore || doc.Blocks[1].Kind != Picture {
		t.Fatal("picture break-before lost", doc.Blocks)
	}
}
func FuzzColleafImages(f *testing.F) {
	wire, err := encodeColleafNotes(context.Background(), &Document{Blocks: []Block{{Kind: Picture, Image: &Image{Data: runningPNG(f, false), MediaType: "image/png"}}}})
	if err != nil {
		f.Fatal(err)
	}
	data, err := json.Marshal(wire)
	if err != nil {
		f.Fatal(err)
	}
	f.Add(data)
	f.Add([]byte(`{"id":"doc","body":{"dataStream":"\b\r\n","customBlocks":[{"startIndex":0,"blockType":0,"blockId":"i"}]},"drawings":{"i":{"drawingId":"i","drawingType":0,"layoutType":0,"imageSourceType":"BASE64","source":"data:image/png;base64,","docTransform":{"size":{"width":1,"height":1}}}}}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		var input colleafNoteSnapshot
		if json.Unmarshal(data, &input) != nil {
			return
		}
		doc, err := decodeColleafNotes(context.Background(), input, colleafTextContext{})
		if err == nil {
			if err = doc.Validate(); err != nil {
				t.Fatal("accepted invalid image document", err)
			}
		}
	})
}

func TestColleafNativeInlineImageRemainsInItsMarkedParagraphItem(t *testing.T) {
	wire, err := encodeColleafNotes(context.Background(), &Document{Blocks: []Block{{Kind: Picture, Image: &Image{Data: runningPNG(t, false), MediaType: "image/png"}}}})
	if err != nil {
		t.Fatal(err)
	}
	id := wire.Body.CustomBlocks[0].ID
	zero, glyph := 0, 2
	wire.Body = colleafBody{DataStream: "Item\bTail\r\n", CustomBlocks: []colleafBlockRecord{{StartIndex: 4, Type: &zero, ID: id}}, Paragraphs: []colleafParagraphRecord{{StartIndex: 9, ID: "native-item", Bullet: sfAttrs(colleafBullet{Type: "list", ID: "l", Level: 0})}}}
	wire.Lists = map[string]json.RawMessage{"list": sfAttrs(colleafNativeList{Type: "list", Levels: []colleafListLevel{{Format: "%1.", Glyph: &glyph, Start: 0, Align: 1}}})}
	doc, err := decodeColleafNotes(context.Background(), wire, colleafTextContext{})
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Blocks) != 1 || doc.Blocks[0].Kind != ListBlock || len(doc.Blocks[0].Items) != 1 {
		t.Fatal(doc.Blocks)
	}
	blocks := doc.Blocks[0].Items[0].Blocks
	if len(blocks) != 3 || blocks[0].Runs[0].Text != "Item" || blocks[1].Kind != Picture || blocks[2].Runs[0].Text != "Tail" {
		t.Fatal("native item lost inline contents", blocks)
	}
	if err = doc.Validate(); err != nil {
		t.Fatal(err)
	}
}
