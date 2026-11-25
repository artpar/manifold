package manifold

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

func colleafContainerNavigationExample(t testing.TB) *Document {
	t.Helper()
	p := func(s string) Block { return Block{Kind: Paragraph, Runs: []Run{{Text: s}}} }
	image := func(id string) Block {
		return Block{Kind: Picture, Anchors: []string{id}, Image: &Image{Data: runningPNG(t, false), MediaType: "image/png", Alt: id, Width: 12, Height: 9}}
	}
	table := func(id string, bs ...Block) Block {
		return Block{Kind: Table, Anchors: []string{id, id + "-alias"}, ColumnWidths: []float64{80}, Rows: []Row{{Cells: []Cell{{Blocks: bs}}}}}
	}
	return &Document{Blocks: []Block{
		p("Lead 😀"),
		table("table", p("Cell"), image("cell-picture"), table("nested", p("Nested"))),
		image("picture"),
		{Kind: ListBlock, List: "bullet", Items: []ListItem{{Blocks: []Block{p("Item"), table("list-table", p("List table")), image("list-picture")}}}},
		{Kind: Paragraph, Runs: []Run{{Text: "Table", Destination: "table"}, {Text: "Alias", Destination: "table-alias"}, {Text: "Cell picture", Destination: "cell-picture"}, {Text: "Nested", Destination: "nested"}, {Text: "Picture", Destination: "picture"}, {Text: "List table", Destination: "list-table"}, {Text: "List picture", Destination: "list-picture"}, {Note: &NoteReference{Kind: Footnote, ID: "n"}}}},
	}, Headers: []Block{table("header-table", p("Header"), image("header-picture"))}, Footers: []Block{image("footer-picture")}, Notes: []Note{{Kind: Footnote, ID: "n", Blocks: []Block{table("note-table", p("Note"), image("note-picture"))}}}}
}
func TestColleafContainerNavigationRoundTrip(t *testing.T) {
	ctx := context.Background()
	want := colleafContainerNavigationExample(t)
	wire, err := encodeColleafDocument(ctx, want)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(wire)
	got, err := decodeColleafDocument(ctx, wire)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(wire)
	if !bytes.Equal(before, after) {
		t.Fatal("source mutated")
	}
	if !reflect.DeepEqual(got, want) {
		a, _ := json.Marshal(got)
		b, _ := json.Marshal(want)
		t.Fatalf("container graph changed\ngot%s\nwant%s", a, b)
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
	recovered, err := decodeColleafDocument(ctx, again)
	if err != nil {
		t.Fatal(err)
	}
	sfAgain, err := ToStructured(recovered)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(sf, sfAgain) {
		t.Fatal("SF container structure/assets/relationships changed")
	}
	if file := os.Getenv("MANYDOC_COLLEAF_CONTAINER_QA"); file != "" {
		if err = os.WriteFile(file, before, 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, spec := range []struct {
		env    string
		edited bool
	}{{"MANYDOC_COLLEAF_CONTAINER_SAVED", false}, {"MANYDOC_COLLEAF_CONTAINER_EDITED", true}} {
		file := os.Getenv(spec.env)
		if file == "" {
			continue
		}
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var native colleafNoteSnapshot
		if err = json.Unmarshal(raw, &native); err != nil {
			t.Fatal(err)
		}
		envelope, err := json.Marshal(colleafEnvelope{colleafEnvelopeType, 3, "univer", colleafEngineVersion, native})
		if err != nil {
			t.Fatal(err)
		}
		actual, err := readColleaf(ctx, bytes.NewReader(envelope))
		if err != nil {
			t.Fatal(err)
		}
		expected, err := FromStructured(sf)
		if err != nil {
			t.Fatal(err)
		}
		if spec.edited {
			// TextX adds an independently styled interval before the existing run.
			expected.Blocks[0].Runs = append([]Run{{Text: "Q"}}, expected.Blocks[0].Runs...)
		}
		expectedSF, err := ToStructured(expected)
		if err != nil {
			t.Fatal(err)
		}
		actualSF, err := ToStructured(actual)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(actualSF, expectedSF) {
			a, _ := json.Marshal(actualSF)
			b, _ := json.Marshal(expectedSF)
			t.Fatalf("actual saved/edited graph changed %s\ngot%s\nwant%s", spec.env, a, b)
		}
	}
}
func TestColleafContainerNavigationCurrentNativePosition(t *testing.T) {
	ctx := context.Background()
	wire, err := encodeColleafDocument(ctx, colleafContainerNavigationExample(t))
	if err != nil {
		t.Fatal(err)
	}
	// A native editor moved the table point into text. The current point wins.
	for i, a := range wire.Body.CustomRanges {
		if a.Type != 3 {
			continue
		}
		p, err := readColleafBookmark(a)
		if err != nil {
			t.Fatal(err)
		}
		if p.BookmarkID == "table" {
			wire.Body.CustomRanges[i].StartIndex = 2
			wire.Body.CustomRanges[i].EndIndex = 2
			break
		}
	}
	got, err := decodeColleafDocument(ctx, wire)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Blocks[1].Anchors, []string{"table-alias"}) || len(got.Blocks[0].Runs) != 3 || got.Blocks[0].Runs[1].Anchor != "table" {
		t.Fatal("stale container metadata overrode current point")
	}
	found := false
	for _, d := range got.Diagnostics {
		found = found || d.Code == "colleaf_stale_bookmark_placement"
	}
	if !found {
		t.Fatal("stale placement not diagnosed")
	}
}
func TestColleafContainerNavigationMetadataAndBindings(t *testing.T) {
	for _, role := range []string{"table", "picture"} {
		for _, hint := range []map[string]any{{"version": 1, "role": role}, {"version": 1, "role": role, "ownerId": "o", "paragraphId": "p"}, {"version": 1, "role": role, "OwnerId": "o"}} {
			_, err := readColleafBookmark(colleafRangeRecord{ID: "a", Properties: sfAttrs(map[string]any{"bookmarkId": "a", "manydocNavigation": hint})})
			if err == nil {
				t.Fatal("malformed owner accepted", hint)
			}
		}
	}
	ctx := context.Background()
	wire, err := encodeColleafDocument(ctx, colleafContainerNavigationExample(t))
	if err != nil {
		t.Fatal(err)
	}
	for i, a := range wire.Body.CustomRanges {
		if a.Type != 3 {
			continue
		}
		p, err := readColleafBookmark(a)
		if err != nil {
			t.Fatal(err)
		}
		if p.BookmarkID == "table" {
			p.Hint.OwnerID = "changed"
			props := map[string]any{"bookmarkId": p.BookmarkID, "manydocNavigation": p.Hint, "foreign": "extra"}
			wire.Body.CustomRanges[i].Properties = sfAttrs(props)
			wire.Body.CustomRanges[i].EndIndex = a.StartIndex + 1
			break
		}
	}
	got, err := decodeColleafDocument(ctx, wire)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Blocks[1].Anchors, []string{"table", "table-alias"}) {
		t.Fatal("current container point lost")
	}
	for _, code := range []string{"colleaf_stale_bookmark_placement", "colleaf_bookmark_metadata", "colleaf_bookmark_span"} {
		found := false
		for _, d := range got.Diagnostics {
			found = found || d.Code == code
		}
		if !found {
			t.Fatal("loss not diagnosed", code)
		}
	}
}
