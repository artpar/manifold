package manydoc

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestColleafLiteralMarkerInternalRoundtrip(t *testing.T) {
	d := literalMarkerDocument()
	wire, err := encodeColleafDocument(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	got, err := decodeColleafDocument(context.Background(), wire)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(canonicalSF(t, d), canonicalSF(t, got)) {
		t.Fatalf("literal graph changed\n%s\n%s", canonicalSF(t, d), canonicalSF(t, got))
	}
	if path := os.Getenv("MANYDOC_NATIVE_LITERAL_QA"); path != "" {
		raw, _ := json.MarshalIndent(wire, "", "  ")
		if err := os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestColleafLiteralMarkerSourcesAndAssets(t *testing.T) {
	d := literalMarkerDocument()
	d.Blocks[0].Source = &SourceInfo{Format: PDF, Role: "L", Element: "list", Structure: "explicit", Confidence: 1, Evidence: "source_list"}
	d.Blocks[0].Items[0].Source = &SourceInfo{Format: PDF, Role: "LI", Element: "item", ParentElement: "list", Structure: "explicit", Confidence: 1, Evidence: "source_item"}
	d.Blocks[0].Items[0].Marker.Source = &SourceInfo{Format: PDF, Role: "Lbl", Element: "label", ParentElement: "item", Structure: "explicit", Confidence: 1, Evidence: "source_label"}
	before := canonicalSF(t, d)
	wire, err := encodeColleafDocument(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	got, err := decodeColleafDocument(context.Background(), wire)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, canonicalSF(t, got)) {
		t.Fatalf("source owners changed\n%s\n%s", before, canonicalSF(t, got))
	}
	checkNativeLiteral(t, "sources", d)
}

func checkNativeLiteral(t *testing.T, name string, d *Document) colleafNoteSnapshot {
	t.Helper()
	d = resolveNumberingDocument(d)
	before := canonicalSF(t, d)
	wire, err := encodeColleafDocument(context.Background(), d)
	if err != nil {
		t.Fatal(name, err)
	}
	got, err := decodeColleafDocument(context.Background(), wire)
	if err != nil {
		t.Fatal(name, err)
	}
	if !bytes.Equal(before, canonicalSF(t, got)) {
		t.Fatalf("%s changed\n%s\n%s", name, before, canonicalSF(t, got))
	}
	if !bytes.Equal(before, canonicalSF(t, d)) {
		t.Fatal("caller mutated")
	}
	if dir := os.Getenv("MANYDOC_NATIVE_LITERAL_QA_DIR"); dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		raw, _ := json.MarshalIndent(wire, "", "  ")
		if err := os.WriteFile(filepath.Join(dir, name+".json"), raw, 0600); err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		if err := writeColleaf(context.Background(), &out, d); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name+".vdocs"), out.Bytes(), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return wire
}

func nativeLiteralAsset(t *testing.T) Block {
	t.Helper()
	im := image.NewRGBA(image.Rect(0, 0, 1, 1))
	im.Set(0, 0, color.RGBA{R: 255, A: 255})
	var data bytes.Buffer
	if err := png.Encode(&data, im); err != nil {
		t.Fatal(err)
	}
	return Block{Kind: Picture, Image: &Image{Data: data.Bytes(), MediaType: "image/png", Width: 16, Height: 16, Alt: "RED_LABEL"}}
}

func TestColleafLiteralMarkerRichCases(t *testing.T) {
	checkNativeLiteral(t, "rich", literalMarkerDocument())
	checkNativeLiteral(t, "image-control", &Document{Blocks: []Block{nativeLiteralAsset(t), mergeParagraph("IMAGE_CONTROL")}})
	mixed := &Document{Blocks: []Block{{Kind: ListBlock, List: "number", Start: listStartPointer(7), Numbering: &ListNumbering{Template: "Item %1)", Formats: []string{"decimal"}}, Items: []ListItem{
		{Blocks: []Block{mergeParagraph("AUTO_FIRST")}},
		{Marker: &ListMarker{Blocks: []Block{mergeParagraph("[done]")}}, Blocks: []Block{mergeParagraph("DONE_BODY")}},
		{Blocks: []Block{mergeParagraph("AUTO_RESUMED")}},
	}}}}
	checkNativeLiteral(t, "mixed", mixed)
	image := nativeLiteralAsset(t)
	nested := Block{Kind: ListBlock, List: "bullet", Anchors: []string{"list-point"}, Items: []ListItem{{Marker: &ListMarker{Blocks: []Block{{Kind: Heading, Level: 2, Runs: []Run{{Text: "RICH_LABEL", Bold: true}}}, image, cellTable([]Block{mergeParagraph("INNER_TABLE")}), {Kind: ListBlock, List: "number", Items: []ListItem{{Blocks: []Block{mergeParagraph("INNER_LIST")}}}}}}, Blocks: []Block{mergeParagraph("RICH_BODY"), {Kind: AnchorPoint, Anchors: []string{"item-end"}}}}}}
	d := &Document{Blocks: []Block{cellTable([]Block{nested})}}
	checkNativeLiteral(t, "nested", d)
	// Reuse rich marker content in notes and running stories without note refs.
	nested.Anchors = nil
	nested.Items[0].Blocks = []Block{mergeParagraph("RICH_BODY")}
	d = &Document{Headers: []Block{nested}, Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "CALL"}, {Note: &NoteReference{Kind: Footnote, ID: "n"}}}}}, Notes: []Note{{Kind: Footnote, ID: "n", Blocks: []Block{nested}}}}
	checkNativeLiteral(t, "stories", d)
	rtl := Block{Kind: ListBlock, List: "bullet", Items: []ListItem{{Marker: &ListMarker{Blocks: []Block{mergeParagraph("RIGHT_LABEL")}}, Blocks: []Block{{Kind: Paragraph, Direction: "rtl", Runs: []Run{{Text: "RTL_BODY"}}}}}}}
	checkNativeLiteral(t, "rtl", &Document{Blocks: []Block{rtl}})
}

func TestColleafLiteralMarkerActualSavedEdit(t *testing.T) {
	path := os.Getenv("MANYDOC_NATIVE_LITERAL_SAVED")
	if path == "" {
		t.Skip("optional pinned facade-save input")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var wire colleafNoteSnapshot
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatal(err)
	}
	got, err := decodeColleafDocument(context.Background(), wire)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got.Text(), "Z)") {
		t.Fatal("native label edit hidden", got.Text())
	}
	if got.Blocks[0].Items[0].Marker == nil || got.Blocks[0].Items[1].Marker.Blocks[0].Runs[2].Note == nil {
		t.Fatal("native marker/note hierarchy lost")
	}
}

func TestColleafLiteralMarkerActualSavedCases(t *testing.T) {
	dir := os.Getenv("MANYDOC_NATIVE_LITERAL_SAVED_DIR")
	if dir == "" {
		t.Skip("optional pinned facade-save cases")
	}
	for _, name := range []string{"mixed", "nested", "stories", "rtl", "sources"} {
		decode := func(path string) *Document {
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var wire colleafNoteSnapshot
			if err := json.Unmarshal(raw, &wire); err != nil {
				t.Fatal(err)
			}
			d, err := decodeColleafDocument(context.Background(), wire)
			if err != nil {
				t.Fatal(name, err)
			}
			return d
		}
		original := decode(filepath.Join(dir, name+".json"))
		saved := decode(filepath.Join(dir, name+"-saved.json"))
		if !bytes.Equal(canonicalSF(t, original), canonicalSF(t, saved)) {
			t.Fatalf("%s native facade changed semantic graph", name)
		}
	}
}

func TestColleafLiteralMarkerMalformedAndStaleOwnership(t *testing.T) {
	ctx := context.Background()
	original := literalMarkerDocument()
	base, err := encodeColleafDocument(ctx, original)
	if err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(map[string]json.RawMessage){
		func(m map[string]json.RawMessage) { m["version"] = json.RawMessage(`2`) },
		func(m map[string]json.RawMessage) { m["count"] = json.RawMessage(`0`) },
		func(m map[string]json.RawMessage) { m["index"] = json.RawMessage(`1000000`) },
		func(m map[string]json.RawMessage) { m["content"] = json.RawMessage(`"forbidden cache"`) },
	} {
		raw, _ := json.Marshal(base)
		var wire colleafNoteSnapshot
		_ = json.Unmarshal(raw, &wire)
		id := wire.Body.Tables[0].ID
		props, _ := colleafStyleObject(wire.Tables[id])
		var meta map[string]json.RawMessage
		_ = json.Unmarshal(props["manydocLiteralItem"], &meta)
		change(meta)
		props["manydocLiteralItem"] = sfAttrs(meta)
		wire.Tables[id] = sfAttrs(props)
		if _, err := decodeColleafDocument(ctx, wire); err == nil {
			t.Fatal("malformed private literal intent accepted")
		}
	}
	// Losing a complete group must retain actual cells, rather than invent a
	// replacement list or silently dropping its visible labels/relationships.
	raw, _ := json.Marshal(base)
	var wire colleafNoteSnapshot
	_ = json.Unmarshal(raw, &wire)
	id := wire.Body.Tables[1].ID
	props, _ := colleafStyleObject(wire.Tables[id])
	delete(props, "manydocLiteralItem")
	wire.Tables[id] = sfAttrs(props)
	got, err := decodeColleafDocument(ctx, wire)
	if err != nil {
		t.Fatal(err)
	}
	if got.Blocks[0].Kind != Table || !hasDiagnostic(got, "colleaf_stale_literal_ownership") {
		t.Fatal("stale ownership did not retain actual table")
	}
	for _, token := range []string{"α)", "β)", "FIRST", "SECOND", "EMPTY", "Marker note"} {
		if !strings.Contains(got.Text(), token) {
			t.Fatal("stale ownership lost", token)
		}
	}
}
