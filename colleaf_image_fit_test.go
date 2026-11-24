package manydoc

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

func colleafFittedImageExample(t testing.TB) *Document {
	p := func(s string) Block { return Block{Kind: Paragraph, Runs: []Run{{Text: s}}} }
	pic := func(alt string) Block {
		return Block{Kind: Picture, Image: &Image{Data: runningPNG(t, false), MediaType: "image/png", Width: 1000, Height: 2000, Alt: alt}}
	}
	ref := Block{Kind: Paragraph, Runs: []Run{{Text: "Citation"}, {Note: &NoteReference{Kind: Footnote, ID: "n"}}}}
	return &Document{Layout: Layout{Width: 300, Height: 450, Margin: 30, HeaderDistance: colleafFloat(10), FooterDistance: colleafFloat(10)}, Blocks: []Block{
		{Kind: SectionBlock, Section: &Section{Headers: RunningStories{Default: &RunningStory{Blocks: []Block{pic("Inherited header")}}}, Footers: RunningStories{Default: &RunningStory{Blocks: []Block{pic("Footer")}}}, Blocks: []Block{
			pic("Body tall"), {Kind: PageBreak}, {Kind: Table, ColumnWidths: []float64{72, 144}, Rows: []Row{{Cells: []Cell{{ColSpan: 2, Blocks: []Block{{Kind: Table, Rows: []Row{{Cells: []Cell{{Blocks: []Block{pic("Nested merged cell")}}, {Blocks: []Block{p("Other cell")}}}}}}}}}}}},
			{Kind: ListBlock, List: "bullet", Items: []ListItem{{Blocks: []Block{p("List"), pic("List continuation")}}}}, ref,
		}}},
		{Kind: SectionBlock, Section: &Section{Layout: &Layout{Width: 240, Height: 400, Margins: &PageMargins{Top: 20, Right: 30, Bottom: 30, Left: 30}, HeaderDistance: colleafFloat(10), FooterDistance: colleafFloat(10)}, Blocks: []Block{p("Smaller section"), ref}}},
	}, Notes: []Note{{ID: "n", Kind: Footnote, Blocks: []Block{pic("Shared note")}}}}
}
func TestColleafImageContainerProjectionAndSF(t *testing.T) {
	ctx := context.Background()
	want := colleafFittedImageExample(t)
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
	expected := map[string]colleafImageSize{"Body tall": {191 * 4.0 / 3, 382 * 4.0 / 3}, "Nested merged cell": {85.5 * 4.0 / 3, 171 * 4.0 / 3}, "List continuation": {191 * 4.0 / 3, 382 * 4.0 / 3}, "Inherited header": {5 * 4.0 / 3, 10 * 4.0 / 3}, "Footer": {10 * 4.0 / 3, 20 * 4.0 / 3}, "Shared note": {143 * 4.0 / 3, 286 * 4.0 / 3}}
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
		var drawing colleafNativeImage
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
		var saved colleafNoteSnapshot
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
	var resized colleafNativeImage
	_ = json.Unmarshal(wire.Drawings[id], &resized)
	resized.Transform.Size = colleafImageSize{Width: 80, Height: 40}
	wire.Drawings[id] = sfAttrs(resized)
	// A native editor edit updates both its actual source and registered mirror.
	// Leaving the old mirror is a contradictory snapshot, not a valid resize.
	wire.Resources = colleafReplaceResource(wire.Resources, "DOC_DRAWING_PLUGIN", map[string]any{"data": wire.Drawings, "order": wire.DrawingOrder})
	got, err = decodeColleafDocument(ctx, wire)
	if err != nil {
		t.Fatal(err)
	}
	image := got.Blocks[0].Section.Blocks[0].Image
	if image.Width != 60 || image.Height != 30 || len(got.Diagnostics) != 1 || got.Diagnostics[0].Code != "colleaf_stale_image_geometry" {
		t.Fatal("stale fitted size won", image, got.Diagnostics)
	}
}
