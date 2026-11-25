package engine

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"
)

func vdocsFittedImageExample(t testing.TB) *Document {
	p := func(s string) Block { return Block{Kind: Paragraph, Runs: []Run{{Text: s}}} }
	pic := func(alt string) Block {
		return Block{Kind: Picture, Image: &Image{Data: runningPNG(t, false), MediaType: "image/png", Width: 1000, Height: 2000, Alt: alt}}
	}
	ref := Block{Kind: Paragraph, Runs: []Run{{Text: "Citation"}, {Note: &NoteReference{Kind: Footnote, ID: "n"}}}}
	return &Document{Layout: Layout{Width: 300, Height: 450, Margin: 30, HeaderDistance: vdocsFloat(10), FooterDistance: vdocsFloat(10)}, Blocks: []Block{
		{Kind: SectionBlock, Section: &Section{Headers: RunningStories{Default: &RunningStory{Blocks: []Block{pic("Inherited header")}}}, Footers: RunningStories{Default: &RunningStory{Blocks: []Block{pic("Footer")}}}, Blocks: []Block{
			pic("Body tall"), {Kind: PageBreak}, {Kind: Table, ColumnWidths: []float64{72, 144}, Rows: []Row{{Cells: []Cell{{ColSpan: 2, Blocks: []Block{{Kind: Table, Rows: []Row{{Cells: []Cell{{Blocks: []Block{pic("Nested merged cell")}}, {Blocks: []Block{p("Other cell")}}}}}}}}}}}},
			{Kind: ListBlock, List: "bullet", Items: []ListItem{{Blocks: []Block{p("List"), pic("List continuation")}}}}, ref,
		}}},
		{Kind: SectionBlock, Section: &Section{Layout: &Layout{Width: 240, Height: 400, Margins: &PageMargins{Top: 20, Right: 30, Bottom: 30, Left: 30}, HeaderDistance: vdocsFloat(10), FooterDistance: vdocsFloat(10)}, Blocks: []Block{p("Smaller section"), ref}}},
	}, Notes: []Note{{ID: "n", Kind: Footnote, Blocks: []Block{pic("Shared note")}}}}
}
func TestVDOCSImageContainerProjectionAndSF(t *testing.T) {
	ctx := context.Background()
	want := vdocsFittedImageExample(t)
	wire, err := encodeColleafDocument(ctx, want)
	if err != nil {
		t.Fatal(err)
	}
	got, err := decodeColleafDocument(ctx, wire)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatal("fitting replaced preferred SF dimensions", got.Diagnostics)
	}
	sf, err := ToStructured(got)
	if err != nil {
		t.Fatal(err)
	}
	again, err := FromStructured(sf)
	if err != nil {
		t.Fatal(err)
	}
	native, err := encodeColleafDocument(ctx, again)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(native, wire) {
		t.Fatal("SF changed container projection")
	}
	expected := map[string]vdocsImageSize{"Body tall": {191 * 4.0 / 3, 382 * 4.0 / 3}, "Nested merged cell": {85.5 * 4.0 / 3, 171 * 4.0 / 3}, "List continuation": {191 * 4.0 / 3, 382 * 4.0 / 3}, "Inherited header": {5 * 4.0 / 3, 10 * 4.0 / 3}, "Footer": {10 * 4.0 / 3, 20 * 4.0 / 3}, "Shared note": {143 * 4.0 / 3, 286 * 4.0 / 3}}
	// Second section has350pt body height before the8pt anchor allowance; note
	// projection reserves the same56pt separator/reference area as the DOCX path.
	drawings := map[string]json.RawMessage{}
	for id, raw := range wire.Drawings {
		drawings[id] = raw
	}
	for _, n := range wire.Notes {
		for id, raw := range n.Drawings {
			drawings[id] = raw
		}
	}
	for _, raw := range drawings {
		var drawing vdocsNativeImage
		if err = json.Unmarshal(raw, &drawing); err != nil {
			t.Fatal(err)
		}
		want, ok := expected[drawing.Description]
		if !ok {
			t.Fatal("unexpected image", drawing.Description)
		}
		if drawing.Transform.Size != want {
			t.Fatalf("%s frame got%+v want%+v", drawing.Description, drawing.Transform.Size, want)
		}
		delete(expected, drawing.Description)
	}
	if len(expected) > 0 {
		t.Fatal("missing frames", expected)
	}
	if file := os.Getenv("MANYDOC_COLLEAF_FIT_QA"); file != "" {
		data, err := json.MarshalIndent(wire, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(file, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if file := os.Getenv("MANYDOC_COLLEAF_FIT_SAVED"); file != "" {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var saved vdocsNoteSnapshot
		if err = json.Unmarshal(data, &saved); err != nil {
			t.Fatal(err)
		}
		got, err = decodeColleafDocument(ctx, saved)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatal("saved preferred sizes changed", got.Diagnostics)
		}
	}
	// The editor's actual frame wins over old fitting intent.
	id := wire.Body.CustomBlocks[0].ID
	var resized vdocsNativeImage
	_ = json.Unmarshal(wire.Drawings[id], &resized)
	resized.Transform.Size = vdocsImageSize{Width: 80, Height: 40}
	wire.Drawings[id] = sfAttrs(resized)
	// A native editor edit updates both its actual source and registered mirror.
	// Leaving the old mirror is a contradictory snapshot, not a valid resize.
	wire.Resources = vdocsReplaceResource(wire.Resources, "DOC_DRAWING_PLUGIN", map[string]any{"data": wire.Drawings, "order": wire.DrawingOrder})
	got, err = decodeColleafDocument(ctx, wire)
	if err != nil {
		t.Fatal(err)
	}
	image := got.Blocks[0].Section.Blocks[0].Image
	if image.Width != 60 || image.Height != 30 || len(got.Diagnostics) != 1 || got.Diagnostics[0].Code != "colleaf_stale_image_geometry" {
		t.Fatal("stale fitted size won", image, got.Diagnostics)
	}
}
func vdocsOriginalJPEG(t testing.TB) []byte {
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
			if vdocsImageHash(data) != "fe9737d0cb4f905917054c7938fc3ccf4b83f0623df9cd5d62192dbaae26b510" {
				t.Fatal("original independent image oracle changed")
			}
			return data
		}
	}
	t.Fatal("original JPEG absent")
	return nil
}
func vdocsImageExample(t testing.TB) *Document {
	t.Helper()
	png := runningPNG(t, false)
	jpeg := vdocsOriginalJPEG(t)
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
func TestVDOCSEmbeddedImagesAllStoriesAndSF(t *testing.T) {
	ctx := context.Background()
	want := vdocsImageExample(t)
	wire, err := encodeColleafNotes(ctx, want)
	if err != nil {
		t.Fatal(err)
	}
	if len(wire.Drawings) != 4 || len(wire.Notes["manydoc-note-1"].Drawings) != 1 {
		t.Fatal("image scope ownership lost")
	}
	got, err := decodeColleafNotes(ctx, wire, vdocsTextContext{})
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
		var edited vdocsNoteSnapshot
		if err = json.Unmarshal(data, &edited); err != nil {
			t.Fatal(err)
		}
		doc, err := decodeColleafNotes(ctx, edited, vdocsTextContext{})
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
func TestVDOCSNativeMixedInlineImageRetainsAllContent(t *testing.T) {
	wire, err := encodeColleafNotes(context.Background(), &Document{Blocks: []Block{{Kind: Picture, Image: &Image{Data: runningPNG(t, false), MediaType: "image/png"}}}})
	if err != nil {
		t.Fatal(err)
	}
	id := wire.Body.CustomBlocks[0].ID
	zero := 0
	wire.Body = vdocsBody{DataStream: "Left 😀\bRight\r\n", CustomBlocks: []vdocsBlockRecord{{StartIndex: 7, Type: &zero, ID: id}}, Paragraphs: []vdocsParagraphRecord{{StartIndex: 13, ID: "p"}}, TextRuns: []vdocsTextRun{{St: 0, Ed: 7, TS: sfAttrs(map[string]int{"bl": 1})}}}
	doc, err := decodeColleafNotes(context.Background(), wire, vdocsTextContext{})
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
func TestVDOCSImageRejectsBadRelationshipsAppearanceAndBytes(t *testing.T) {
	for _, mutate := range []func(*vdocsNoteSnapshot){
		func(w *vdocsNoteSnapshot) { delete(w.Drawings, w.Body.CustomBlocks[0].ID) },
		func(w *vdocsNoteSnapshot) { w.DrawingOrder = append(w.DrawingOrder, w.DrawingOrder[0]) },
		func(w *vdocsNoteSnapshot) {
			for id, raw := range w.Drawings {
				var d vdocsNativeImage
				_ = json.Unmarshal(raw, &d)
				d.SourceType = "URL"
				d.Source = "https://example.org/image.png"
				w.Drawings[id] = sfAttrs(d)
				break
			}
		},
		func(w *vdocsNoteSnapshot) {
			for id, raw := range w.Drawings {
				var d vdocsNativeImage
				_ = json.Unmarshal(raw, &d)
				d.Source = "data:image/png;base64,garbage"
				w.Drawings[id] = sfAttrs(d)
				break
			}
		},
		func(w *vdocsNoteSnapshot) {
			for id, raw := range w.Drawings {
				var d vdocsNativeImage
				_ = json.Unmarshal(raw, &d)
				d.Transform.Angle = 30
				w.Drawings[id] = sfAttrs(d)
				break
			}
		},
		func(w *vdocsNoteSnapshot) {
			for id, raw := range w.Drawings {
				var d vdocsNativeImage
				_ = json.Unmarshal(raw, &d)
				d.Hidden = true
				w.Drawings[id] = sfAttrs(d)
				break
			}
		},
		func(w *vdocsNoteSnapshot) { w.Drawings["orphan"] = w.Drawings[w.Body.CustomBlocks[0].ID] },
		func(w *vdocsNoteSnapshot) {
			for id, raw := range w.Drawings {
				var d vdocsNativeImage
				_ = json.Unmarshal(raw, &d)
				d.Cache = "data:image/jpeg;base64,AA=="
				w.Drawings[id] = sfAttrs(d)
				break
			}
		},
	} {
		wire, err := encodeColleafNotes(context.Background(), vdocsImageExample(t))
		if err != nil {
			t.Fatal(err)
		}
		mutate(&wire)
		if _, err = decodeColleafNotes(context.Background(), wire, vdocsTextContext{}); err == nil {
			t.Fatal("invalid image source/appearance accepted")
		}
	}
}
func TestVDOCSNativeImageAssetEditInvalidatesOldIntent(t *testing.T) {
	wire, err := encodeColleafNotes(context.Background(), vdocsImageExample(t))
	if err != nil {
		t.Fatal(err)
	}
	id := wire.Body.CustomBlocks[0].ID
	var drawing vdocsNativeImage
	_ = json.Unmarshal(wire.Drawings[id], &drawing)
	different := runningPNG(t, true)
	drawing.Source = "data:image/png;base64," + base64.StdEncoding.EncodeToString(different)
	wire.Drawings[id] = sfAttrs(drawing)
	doc, err := decodeColleafNotes(context.Background(), wire, vdocsTextContext{})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(doc.Blocks[0].Image.Data, different) || doc.Blocks[0].Image.Width != 24 || doc.Blocks[0].Image.Height != 12 {
		t.Fatal("old size intent/bytes won over native asset")
	}
	if len(doc.Diagnostics) !=

		// A genuine foreign document has no manifold layout intent.

		// Independently authored native row/cell records, not produced by the exporter.

		// Covered native cells must never hide visible text or nested structures.

		1 || doc.Diagnostics[0].Code != "colleaf_stale_image_geometry" {
		t.Fatal(doc.Diagnostics)
	}
}

func TestVDOCSImageAnchorHiddenAndPageBreakPolicies(t *testing.T) {
	makeWire := func() vdocsNoteSnapshot {
		wire, err := encodeColleafNotes(context.Background(), &Document{Blocks: []Block{{Kind: Picture, Image: &Image{Data: runningPNG(t, false), MediaType: "image/png"}}}})
		if err != nil {
			t.Fatal(err)
		}
		return wire
	}
	wire := makeWire()
	wire.Body.TextRuns = []vdocsTextRun{{St: 0, Ed: 1, TS: json.RawMessage(`{"hidden":true}`)}}
	if _, err := decodeColleafNotes(context.Background(), wire, vdocsTextContext{}); err == nil {
		t.Fatal("hidden anchor became visible image")
	}
	wire = makeWire()
	wire.Body.Paragraphs[0].Style = sfAttrs(map[string]int{"pageBreakBefore": 1})
	doc, err := decodeColleafNotes(context.Background(), wire, vdocsTextContext{})
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
		var input vdocsNoteSnapshot
		if json.Unmarshal(data, &input) != nil {
			return
		}
		doc, err := decodeColleafNotes(context.Background(), input, vdocsTextContext{})
		if err == nil {
			if err = doc.Validate(); err != nil {
				t.Fatal(

					// A successful import must retain valid table ownership and dimensions.

					"accepted invalid image document", err)
			}
		}
	})
}

func TestVDOCSNativeInlineImageRemainsInItsMarkedParagraphItem(t *testing.T) {
	wire, err := encodeColleafNotes(context.Background(), &Document{Blocks: []Block{{Kind: Picture, Image: &Image{Data: runningPNG(t, false), MediaType: "image/png"}}}})
	if err != nil {
		t.Fatal(err)
	}
	id := wire.Body.CustomBlocks[0].ID
	zero, glyph := 0, 2
	wire.Body = vdocsBody{DataStream: "Item\bTail\r\n", CustomBlocks: []vdocsBlockRecord{{StartIndex: 4, Type: &zero, ID: id}}, Paragraphs: []vdocsParagraphRecord{{StartIndex: 9, ID: "native-item", Bullet: sfAttrs(vdocsBullet{Type: "list", ID: "l", Level: 0})}}}
	wire.Lists = map[string]json.RawMessage{"list": sfAttrs(vdocsNativeList{Type: "list", Levels: []vdocsListLevel{{Format: "%1.", Glyph: &glyph, Start: 0, Align: 1}}})}
	doc, err := decodeColleafNotes(context.Background(), wire, vdocsTextContext{})
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
func vdocsSectionExample(t testing.TB) *Document {
	p := func(s string) Block { return Block{Kind: Paragraph, Runs: []Run{{Text: s}}} }
	story := func(s string) *RunningStory { return &RunningStory{Blocks: []Block{p(s)}} }
	picture := Block{Kind: Picture, Image: &Image{Data: runningPNG(t, false), MediaType: "image/png", Alt: "Header logo", Width: 24, Height: 12}}
	return &Document{Metadata: Metadata{Title: "Native sections", Author: "Manydoc", Subject: "Page and running story ownership"}, Layout: Layout{Width: 450.125, Height: 600.5, Margins: &PageMargins{Top: 40.125, Right: 30, Bottom: 40, Left: 30}, HeaderDistance: vdocsFloat(12), FooterDistance: vdocsFloat(9)}, EvenAndOddHeaders: true, Blocks: []Block{
		{Kind: SectionBlock, Section: &Section{FirstPage: true, Headers: RunningStories{Default: &RunningStory{Blocks: []Block{p("Odd header"), picture, {Kind: Table, ColumnWidths: []float64{72, 72}, Rows: []Row{{Cells: []Cell{{ColSpan: 2, Blocks: []Block{p("Header table")}}}}}}, {Kind: ListBlock, List: "bullet", Items: []ListItem{{Blocks: []Block{p("Header bullet")}}}}}}, First: story("First header"), Even: story("Even header")}, Footers: RunningStories{Default: story("Footer")}, Blocks: []Block{p("First body 😀"), {Kind: PageBreak}, p("First even body"), {Kind: PageBreak}, p("First odd body")}}},
		{Kind: SectionBlock, Section: &Section{Start: SectionNextPage, Headers: RunningStories{Default: &RunningStory{}}, Blocks: []Block{p("Second body"), {Kind: PageBreak}, p("Second next page")}}},
		{Kind: SectionBlock, Section: &Section{Start: SectionOddPage, Layout: &Layout{Width: 400, Height: 550, Margins: &PageMargins{Top: 0, Right: 0, Bottom: 0, Left: 0}, HeaderDistance: vdocsFloat(0), FooterDistance: vdocsFloat(0)}, Headers: RunningStories{Default: story("Third header")}, Footers: RunningStories{Default: &RunningStory{}}, Blocks: []Block{p("Third body")}}},
	}}
}
func TestVDOCSDocumentSectionsStoriesAndSFRoundTrip(t *testing.T) {
	ctx := context.Background()
	want := vdocsSectionExample(t)
	wire, err := encodeColleafDocument(ctx, want)
	if err != nil {
		t.Fatal(err)
	}
	got, err := decodeColleafDocument(ctx, wire)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		a, _ := json.Marshal(got)
		b, _ := json.Marshal(want)
		t.Fatalf("section semantics lost\ngot%s\nwant%s", a, b)
	}
	sf, err := ToStructured(got)
	if err != nil {
		t.Fatal(err)
	}
	model, err := FromStructured(sf)
	if err != nil {
		t.Fatal(err)
	}
	again, err := encodeColleafDocument(ctx, model)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(wire, again) {
		t.Fatal("SF changed native section/story sources")
	}
	if file := os.Getenv("MANYDOC_COLLEAF_SECTION_QA"); file != "" {
		data, err := json.MarshalIndent(wire, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(file, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if file := os.Getenv("MANYDOC_COLLEAF_SECTION_SAVED"); file != "" {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var saved vdocsNoteSnapshot
		if err = json.Unmarshal(data, &saved); err != nil {
			t.Fatal(err)
		}
		got, err = decodeColleafDocument(ctx, saved)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatal("facade changed sections/stories", got.Diagnostics)
		}
	}
}
func TestVDOCSPlainDocumentLayoutMetadataRunningStories(t *testing.T) {
	d := &Document{Metadata: Metadata{Author: "Author", Subject: "Subject"}, Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "Body"}}}}, Headers: []Block{{Kind: Paragraph, Runs: []Run{{Text: "Header"}}}}, Footers: []Block{{Kind: Paragraph, Runs: []Run{{Text: "Footer"}}}}}
	wire, err := encodeColleafDocument(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	got, err := decodeColleafDocument(context.Background(), wire)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, d) {
		a, _ := json.Marshal(got)
		b, _ := json.Marshal(d)
		t.Fatalf("plain doc changed\ngot%s\nwant%s", a, b)
	}
}
func TestVDOCSNativeLayoutEditsWinAndStoryReferencesValidate(t *testing.T) {
	d := vdocsSectionExample(t)
	wire, err := encodeColleafDocument(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	var style vdocsPageProperties
	_ = json.Unmarshal(wire.DocumentStyle, &style)
	style.PageSize.Width = 800
	wire.DocumentStyle = sfAttrs(style)
	got, err := decodeColleafDocument(context.Background(), wire)
	if err != nil {
		t.Fatal(err)
	}
	if got.Layout.Width != 600 || len(got.Diagnostics) != 1 || got.Diagnostics[0].Code != "colleaf_stale_layout" {
		t.Fatal("native paper edit lost", got.Layout, got.Diagnostics)
	}
	wire, err = encodeColleafDocument(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	delete(wire.Headers, *wire.Body.SectionBreaks[0].DefaultHeader)
	if _, err = decodeColleafDocument(context.Background(), wire); err == nil {
		t.Fatal("missing running body accepted")
	}
}

func TestVDOCSNativeSectionPolicies(t *testing.T) {
	ctx := context.Background()
	makeWire := func(t *testing.T) vdocsNoteSnapshot {
		t.Helper()
		wire, err := encodeColleafDocument(ctx, vdocsSectionExample(t))
		if err != nil {
			t.Fatal(err)
		}
		return wire
	}
	for name, mutate := range map[string]func(*vdocsNoteSnapshot){
		"orphan local table": func(w *vdocsNoteSnapshot) {
			id := *w.Body.SectionBreaks[0].DefaultHeader
			h := w.Headers[id]
			h.Tables = map[string]json.RawMessage{"lost": json.RawMessage(`{}`)}
			w.Headers[id] = h
		},
		"orphan local drawing": func(w *vdocsNoteSnapshot) {
			id := *w.Body.SectionBreaks[0].DefaultHeader
			h := w.Headers[id]
			h.Drawings = map[string]json.RawMessage{"lost": json.RawMessage(`{}`)}
			w.Headers[id] = h
		},
		"orphan header": func(w *vdocsNoteSnapshot) {
			w.Headers["orphan"] = vdocsRunningStory{HeaderID: "orphan", Body: vdocsBody{DataStream: "Lost\r\n"}}
		},
		"wrong story identity": func(w *vdocsNoteSnapshot) {
			id := *w.Body.SectionBreaks[0].DefaultHeader
			h := w.Headers[id]
			h.HeaderID = "wrong"
			w.Headers[id] = h
		},
		"next column":        func(w *vdocsNoteSnapshot) { w.Body.SectionBreaks[0].Start = vdocsInt(2) },
		"section even mode":  func(w *vdocsNoteSnapshot) { w.Body.SectionBreaks[0].EvenOdd = vdocsInt(0) },
		"invalid first flag": func(w *vdocsNoteSnapshot) { w.Body.SectionBreaks[0].FirstPage = vdocsInt(2) },
		"negative page": func(w *vdocsNoteSnapshot) {
			w.Body.SectionBreaks[0].PageSize = &vdocsImageSize{Width: -1, Height: 100}
		},
		"note settings": func(w *vdocsNoteSnapshot) { w.NoteSettings = json.RawMessage(`{"footnoteNumberStart":7}`) },
		"section note settings": func(w *vdocsNoteSnapshot) {
			w.Body.SectionBreaks[0].Extra = map[string]json.RawMessage{"noteProperties": json.RawMessage(`{"numberStart":7}`)}
		},
		"unpaginated canvas": func(w *vdocsNoteSnapshot) { w.DocumentStyle = json.RawMessage(`{}`) },
	} {
		t.Run(name, func(t *testing.T) {
			wire := makeWire(t)
			mutate(&wire)
			if _, err := decodeColleafDocument(ctx, wire); err == nil {
				t.Fatal("unmapped/contradictory source accepted")
			}
		})
	}
	wire := makeWire(t)
	opened, err := openColleafIntent(ctx, wire)
	if err != nil {
		t.Fatal(err)
	}
	opened.Resources = nil
	got, err := decodeColleafDocument(ctx, opened)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Blocks) != 3 || got.Blocks[0].Kind != SectionBlock || got.Blocks[0].Section.Layout == nil || !got.Blocks[0].Section.FirstPage {
		t.Fatal("foreign section properties flattened")
	}
	wire = makeWire(t)
	wire.Body.SectionBreaks[0].Extra = map[string]json.RawMessage{"columnProperties": json.RawMessage(`{"columnCount":2}`)}
	got, err = decodeColleafDocument(ctx, wire)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Diagnostics) != 1 || got.Diagnostics[0].Code != "colleaf_section_style" {
		t.Fatal("unmapped column layout not disclosed", got.Diagnostics)
	}
}

func TestVDOCSAuthoredAndRenderedPageBreaks(t *testing.T) {
	for _, soft := range []bool{false, true} {
		body := vdocsBody{DataStream: "A😀\fB\r\n", Paragraphs: []vdocsParagraphRecord{{StartIndex: 5}}, SectionBreaks: []vdocsSectionRecord{{StartIndex: 6}}}
		if soft {
			body.RenderedPageBreaks = []int{3}
		}
		got, err := decodeColleafBody(context.Background(), body, vdocsTextContext{}, nil)
		if err != nil {
			t.Fatal(err)
		}
		if soft {
			if len(got.Blocks) != 1 || len(got.Diagnostics) != 1 || got.Diagnostics[0].Code != "colleaf_soft_page_break" {
				t.Fatal("soft break not disclosed/joined", got)
			}
			var text string
			for _, run := range got.Blocks[0].Runs {
				text += run.Text
			}
			if text != "A😀B" {
				t.Fatal(text)
			}
		} else if len(got.Blocks) != 3 || got.Blocks[1].Kind != PageBreak || got.Blocks[0].Runs[0].Text != "A😀" || got.Blocks[2].Runs[0].Text != "B" {
			t.Fatal("authored page boundary lost", got)
		}
	}
}

func TestVDOCSSectionsShareNotes(t *testing.T) {
	ctx := context.Background()
	d := vdocsSectionExample(t)
	notes := vdocsNoteExample()
	d.Notes = notes.Notes
	d.Blocks[1].Section.Blocks = append(d.Blocks[1].Section.Blocks, notes.Blocks...)
	wire, err := encodeColleafDocument(ctx, d)
	if err != nil {
		t.Fatal(err)
	}
	if wire.Notes["manydoc-note-1"].Mark == nil || *wire.Notes["manydoc-note-1"].Mark != "1" || wire.Notes["manydoc-note-2"].Mark == nil || *wire.Notes["manydoc-note-2"].Mark != "1" {
		t.Fatal("later-section automatic labels not projected")
	}
	got, err := decodeColleafDocument(ctx, wire)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, d) {
		t.Fatal("notes in later sections lost", got.Diagnostics)
	}
	id := *wire.Body.SectionBreaks[0].DefaultHeader
	h := wire.Headers[id]
	h.Tables = map[string]json.RawMessage{"lost": json.RawMessage(`{}`)}
	wire.Headers[id] = h
	if _, err = decodeColleafDocument(ctx, wire); err == nil {
		t.Fatal("orphan local table silently dropped")
	}
}

func FuzzColleafDocument(f *testing.F) {
	wire, err := encodeColleafDocument(context.Background(), vdocsSectionExample(f))
	if err != nil {
		f.Fatal(err)
	}
	data, err := json.Marshal(wire)
	if err != nil {
		f.Fatal(err)
	}
	f.Add(data)
	f.Add([]byte(`{"documentStyle":{"documentFlavor":1},"body":{"dataStream":"Text\r\n"}}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		var input vdocsNoteSnapshot
		if json.Unmarshal(data, &input) != nil {
			return
		}
		doc, err := decodeColleafDocument(context.Background(), input)
		if err == nil {
			if err = doc.Validate(); err != nil {
				t.Fatal(err)
			}
		}
	})
}
func vdocsTableExample() []Block {
	p := func(s string) Block { return Block{Kind: Paragraph, Runs: []Run{{Text: s}}} }
	nested := Block{Kind: Table, ColumnWidths: []float64{72}, Rows: []Row{{Cells: []Cell{{Blocks: []Block{p("Inner 😀")}}}}}}
	return []Block{p("Before"), {Kind: Table, ColumnWidths: []float64{72, 108, 144}, Rows: []Row{
		{Cells: []Cell{{RowSpan: 2, ColSpan: 2, Blocks: []Block{p("Merge 😀"), nested}}, {Blocks: []Block{p("B")}}}},
		{Cells: []Cell{{Blocks: []Block{p("C")}}}},
		{Cells: []Cell{{ColSpan: 3, Blocks: []Block{p("Tail")}}}},
	}}, p("After"), {Kind: Table, Rows: []Row{
		{RepeatHeader: true, Cells: []Cell{{Blocks: []Block{p("Header")}}}},
		{Cells: []Cell{{Blocks: []Block{p("Body")}}}},
	}}}
}
func TestVDOCSMergedNestedTableSFRoundTrip(t *testing.T) {
	ctx := context.Background()
	blocks := vdocsTableExample()
	body, tables, err := encodeColleafBody(ctx, blocks)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := decodeColleafBody(ctx, body, vdocsTextContext{}, tables)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(blocks, doc.Blocks) {
		a, _ := json.Marshal(blocks)
		b, _ := json.Marshal(doc.Blocks)
		t.Fatalf("table semantics differ\nwant%s\ngot%s", a, b)
	}
	if len(doc.Diagnostics) != 0 {
		t.Fatal(doc.Diagnostics)
	}
	if !reflect.DeepEqual(doc.Blocks, blocks) {
		t.Fatalf("native table changed SF content/spans/styles\ngot%+v\nwant%+v", doc.Blocks, blocks)
	}
	if len(doc.Diagnostics) > 0 {
		t.Fatal("supported native tables emitted losses", doc.Diagnostics)
	}
	sf, err := ToStructured(doc)
	if err != nil {
		t.Fatal(err)
	}
	throughSF, err := FromStructured(sf)
	if err != nil {
		t.Fatal(err)
	}
	body2, tables2, err := encodeColleafBody(ctx, throughSF.Blocks)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(body, body2) || !reflect.DeepEqual(tables, tables2) {
		t.Fatal("canonical SF changed native cells/relations/styles")
	}
	if path := os.Getenv("MANYDOC_COLLEAF_TABLE_QA"); path != "" {
		data, err := json.MarshalIndent(map[string]any{"id": "manydoc-table-qa", "body": body, "tableSource": tables, "documentStyle": map[string]any{"documentFlavor": 1, "pageSize": map[string]int{"width": 800, "height": 1000}, "marginLeft": 40, "marginRight": 40}}, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if path := os.Getenv("MANYDOC_COLLEAF_TABLE_EDIT"); path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var saved struct {
			Body   vdocsBody                  `json:"body"`
			Tables map[string]json.RawMessage `json:"tableSource"`
		}
		if err = json.Unmarshal(data, &saved); err != nil {
			t.Fatal(err)
		}
		edited, err := decodeColleafBody(ctx, saved.Body, vdocsTextContext{}, saved.Tables)
		if err != nil {
			t.Fatal(err)
		}
		got := edited.Blocks[1].Rows[1].Cells[0].Blocks[0].Runs[0].Text
		if got != "Z" {
			t.Fatal("native cell edit did not win", got)
		}
		if edited.Blocks[1].Rows[0].Cells[0].RowSpan != 2 || edited.Blocks[1].Rows[0].Cells[0].ColSpan != 2 {
			t.Fatal("native save changed merge ownership")
		}
	}
}

func nativeTableFixture(values [][]string, spans [][][2]int) (vdocsBody, map[string]json.RawMessage) {
	stream := "\x1a"
	rows := []any{}
	for ri, row := range values {
		stream += "\x1b"
		cells := []any{}
		for ci, text := range row {
			stream += "\x1c" + text + "\r\n\x1d"
			cells = append(cells, map[string]int{"rowSpan": spans[ri][ci][0], "columnSpan": spans[ri][ci][1]})
		}
		stream += "\x0e"
		rows = append(rows, map[string]any{"tableCells": cells})
	}
	stream += "\x0f"
	end := vdocsUTF16Len(stream)
	stream += "\r\n"
	width := func() any { return map[string]any{"size": vdocsTableWidth{1, vdocsNumberUnit{96, 3}}} }
	return vdocsBody{DataStream: stream, Tables: []vdocsTableRecord{{StartIndex: 0, EndIndex: end, ID: "native"}}}, map[string]json.RawMessage{"native": sfAttrs(map[string]any{"tableId": "native", "tableRows": rows, "tableColumns": []any{width(), width(), width()}})}
}
func TestVDOCSNativeMergeOriginsContinuationsAndContent(t *testing.T) {
	values := [][]string{{"A", "", "B"}, {"", "", "C"}}
	spans := [][][2]int{{{2, 2}, {0, 0}, {1, 1}}, {{0, 2}, {0, 0}, {1, 1}}}
	body, tables := nativeTableFixture(values, spans)
	doc, err := decodeColleafBody(context.Background(), body, vdocsTextContext{}, tables)
	if err != nil {
		t.Fatal(err)
	}
	table := doc.Blocks[0]
	if len(table.Rows) != 2 || len(table.Rows[0].Cells) != 2 || len(table.Rows[1].Cells) != 1 || table.Rows[0].Cells[0].ColSpan != 2 || table.Rows[0].Cells[0].RowSpan != 2 || table.Rows[1].Cells[0].Blocks[0].Runs[0].Text != "C" {
		t.Fatal(table)
	}

	values[1][0] = "Do not lose me"
	body, tables = nativeTableFixture(values, spans)
	if _, err = decodeColleafBody(context.Background(), body, vdocsTextContext{}, tables); err == nil || !strings.Contains(err.Error(), "contains content") {
		t.Fatal("covered content discarded", err)
	}
}
func TestVDOCSTableRejectsContradictorySources(t *testing.T) {
	cases := []struct {
		name   string
		values [][]string
		spans  [][][2]int
	}{
		{"overlap", [][]string{{"A", "B"}, {"C", "D"}}, [][][2]int{{{2, 2}, {1, 1}}, {{1, 2}, {1, 1}}}},
		{"unowned vertical", [][]string{{"A", "B"}}, [][][2]int{{{0, 2}, {1, 1}}}},
		{"unowned horizontal", [][]string{{"A", "B"}}, [][][2]int{{{0, 0}, {1, 3}}}},
		{"out of bounds", [][]string{{"A"}}, [][][2]int{{{2, 3}}}},
		{"incomplete grid", [][]string{{"A"}}, [][][2]int{{{1, 2}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body, tables := nativeTableFixture(tc.values, tc.spans)
			if _, err := decodeColleafBody(context.Background(), body, vdocsTextContext{}, tables); err == nil {
				t.Fatal("invalid span accepted")
			}
		})
	}
	body, tables := nativeTableFixture([][]string{{"A"}}, [][][2]int{{{1, 3}}})
	if _, err := decodeColleafBody(context.Background(), body, vdocsTextContext{}, nil); err == nil {
		t.Fatal("missing source accepted")
	}
	tables["orphan"] = tables["native"]
	if _, err := decodeColleafBody(context.Background(), body, vdocsTextContext{}, tables); err == nil {
		t.Fatal("orphan source accepted")
	}
}

func TestVDOCSNativeTableLegacyGeometryUnits(t *testing.T) {
	body, tables := nativeTableFixture([][]string{{"A"}}, [][][2]int{{{1, 3}}})
	var source map[string]json.RawMessage
	if err := json.Unmarshal(tables["native"], &source); err != nil {
		t.Fatal(err)
	}
	source["tableColumns"] = sfAttrs([]any{
		map[string]any{"size": vdocsTableWidth{1, vdocsNumberUnit{96, 0}}},
		map[string]any{"size": vdocsTableWidth{1, vdocsNumberUnit{96, 3}}},
		map[string]any{"size": vdocsTableWidth{1, vdocsNumberUnit{96, 0}}},
	})
	source["size"] = sfAttrs(vdocsTableWidth{1, vdocsNumberUnit{288, 0}})
	tables["native"] = sfAttrs(source)
	doc, err := decodeColleafBody(context.Background(), body, vdocsTextContext{}, tables)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(doc.Blocks[0].ColumnWidths, []float64{72, 72, 72}) || len(doc.Diagnostics) > 0 {
		t.Fatal(doc.Blocks, doc.Diagnostics)
	}
}

func TestVDOCSMixedTableParagraphKeepsTextOrderAndStyles(t *testing.T) {
	body, tables := nativeTableFixture([][]string{{"Cell"}}, [][][2]int{{{1, 3}}})
	end := body.Tables[0].EndIndex
	body.DataStream = "Prefix" + strings.TrimSuffix(body.DataStream, "\r\n") + "Suffix\r\n"
	body.Tables[0].StartIndex += 6
	body.Tables[0].EndIndex += 6
	body.Paragraphs = []vdocsParagraphRecord{{StartIndex: 6 + end + 6, Style: sfAttrs(map[string]int{"direction": 2})}}
	body.TextRuns = []vdocsTextRun{{St: 0, Ed: 6, TS: sfAttrs(map[string]int{"bl": 1})}}
	doc, err := decodeColleafBody(context.Background(), body, vdocsTextContext{}, tables)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Blocks) != 3 || doc.Blocks[0].Runs[0].Text != "Prefix" || !doc.Blocks[0].Runs[0].Bold || doc.Blocks[1].Kind != Table || doc.Blocks[2].Runs[0].Text != "Suffix" || doc.Blocks[2].Direction != "rtl" {
		t.Fatal(doc.Blocks)
	}
	if len(doc.Diagnostics) != 1 || doc.Diagnostics[0].Code != "colleaf_table_paragraph" {
		t.Fatal(doc.Diagnostics)
	}
}

func FuzzColleafTables(f *testing.F) {
	type wire struct {
		Body   vdocsBody                  `json:"body"`
		Tables map[string]json.RawMessage `json:"tableSource"`
	}
	body, tables, err := encodeColleafBody(context.Background(), vdocsTableExample())
	if err != nil {
		f.Fatal(err)
	}
	seed, err := json.Marshal(wire{body, tables})
	if err != nil {
		f.Fatal(err)
	}
	f.Add(seed)
	body, tables = nativeTableFixture([][]string{{"A", "B"}, {"", "C"}}, [][][2]int{{{2, 2}, {1, 1}}, {{0, 2}, {1, 1}}})
	seed, err = json.Marshal(wire{body, tables})
	if err != nil {
		f.Fatal(err)
	}
	f.Add(seed)
	f.Fuzz(func(t *testing.T, data []byte) {
		var input wire
		if json.Unmarshal(data, &input) != nil {
			return
		}
		doc, err := decodeColleafBody(context.Background(), input.Body, vdocsTextContext{}, input.Tables)
		if err == nil {
			if len(doc.Blocks) == 0 {
				t.Fatal("accepted empty native document")
			}

			var walk func([]Block)
			walk = func(blocks []Block) {
				for _, b := range blocks {
					if b.Kind == Table {
						grid, err := tableGrid(b.Rows)
						if err != nil {
							t.Fatal(err)
						}
						if err = validateColumnWidths(b.ColumnWidths, grid.Columns); err != nil {
							t.Fatal(err)
						}
						for _, row := range b.Rows {
							for _, cell := range row.Cells {
								walk(cell.Blocks)
							}
						}
					}
				}
			}
			walk(doc.Blocks)
		}
	})
}
