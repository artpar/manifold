package manydoc

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

func colleafFlowNavigationExample() *Document {
	p := func(s string) Block { return Block{Kind: Paragraph, Runs: []Run{{Text: s}}} }
	list := func(id string) Block {
		return Block{Kind: ListBlock, List: "bullet", Anchors: []string{id}, Items: []ListItem{{Blocks: []Block{p("Item")}}}}
	}
	main := list("list")
	main.Anchors = append(main.Anchors, "list-alias")
	main.Items[0].Blocks[0].Anchors = []string{"item"}
	main.Items[0].Blocks = append(main.Items[0].Blocks, list("nested-list"), Block{Kind: PageBreak, Anchors: []string{"list-break"}}, p("Tail"))
	links := Block{Kind: Paragraph, Runs: []Run{{Text: "S1", Destination: "section-1"}, {Text: "List", Destination: "list"}, {Text: "Alias", Destination: "list-alias"}, {Text: "Item", Destination: "item"}, {Text: "Nested", Destination: "nested-list"}, {Text: "Break", Destination: "list-break"}, {Text: "S2", Destination: "section-2"}, {Text: "B2", Destination: "break-2"}, {Text: "Empty", Destination: "empty-section"}, {Note: &NoteReference{Kind: Footnote, ID: "n"}}}}
	return &Document{Blocks: []Block{
		{Kind: SectionBlock, Anchors: []string{"section-1"}, Section: &Section{Blocks: []Block{p("Lead 😀"), main, links}, Headers: RunningStories{Default: &RunningStory{Blocks: []Block{list("header-list")}}}}},
		{Kind: SectionBlock, Anchors: []string{"section-2"}, Section: &Section{Start: SectionNextPage, Blocks: []Block{{Kind: PageBreak, Anchors: []string{"break-2"}}, p("Second")}}},
		{Kind: SectionBlock, Anchors: []string{"empty-section"}, Section: &Section{Start: SectionContinuous}},
	}, Notes: []Note{{Kind: Footnote, ID: "n", Blocks: []Block{list("note-list")}}}}
}
func TestColleafFlowNavigationRoundTrip(t *testing.T) {
	ctx := context.Background()
	want := colleafFlowNavigationExample()
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
		t.Fatalf("flow point semantics changed\ngot%s\nwant%s", a, b)
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
	round, err := decodeColleafDocument(ctx, again)
	if err != nil {
		t.Fatal(err)
	}
	roundSF, err := ToStructured(round)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(sf, roundSF) {
		t.Fatal("SF graph changed")
	}
	if file := os.Getenv("MANYDOC_COLLEAF_FLOW_QA"); file != "" {
		if err = os.WriteFile(file, before, 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, spec := range []struct {
		env    string
		edited bool
	}{{"MANYDOC_COLLEAF_FLOW_SAVED", false}, {"MANYDOC_COLLEAF_FLOW_EDITED", true}} {
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
			expected.Blocks[0].Section.Blocks[0].Runs = append([]Run{{Text: "Q"}}, expected.Blocks[0].Section.Blocks[0].Runs...)
		}
		// Inserting at the first section cursor moves its point away from its start.
		if spec.edited {
			expected.Blocks[0].Anchors = nil
			expected.Blocks[0].Section.Blocks[0].Runs = append([]Run{{Text: "Q"}, {Anchor: "section-1"}}, expected.Blocks[0].Section.Blocks[0].Runs[1:]...)
			expected.Diagnostics = []Diagnostic{{Code: "colleaf_stale_bookmark_placement", Message: "Edited native bookmark position is retained as a current inline point rather than old block placement"}}
		}
		expectedSF, err := ToStructured(expected)
		if err != nil {
			t.Fatal(err)
		}
		actualSF, err := ToStructured(actual)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(expectedSF, actualSF) {
			a, _ := json.Marshal(actualSF)
			b, _ := json.Marshal(expectedSF)
			t.Fatalf("actual flow save/edit changed %s\ngot%s\nwant%s", spec.env, a, b)
		}
		if spec.edited {
			found := false
			for _, d := range actual.Diagnostics {
				found = found || d.Code == "colleaf_stale_bookmark_placement"
			}
			if !found {
				t.Fatal("moved section ownership not disclosed")
			}
		}
	}
}
func TestColleafFlowNavigationMovedListPoint(t *testing.T) {
	ctx := context.Background()
	wire, err := encodeColleafDocument(ctx, colleafFlowNavigationExample())
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
		if p.BookmarkID == "list" {
			wire.Body.CustomRanges[i].StartIndex++
			wire.Body.CustomRanges[i].EndIndex++
			break
		}
	}
	got, err := decodeColleafDocument(ctx, wire)
	if err != nil {
		t.Fatal(err)
	}
	list := got.Blocks[0].Section.Blocks[1]
	if !reflect.DeepEqual(list.Anchors, []string{"list-alias"}) || list.Items[0].Blocks[0].Runs[1].Anchor != "list" {
		t.Fatal("moved list point restored stale block ownership")
	}
	found := false
	for _, d := range got.Diagnostics {
		found = found || d.Code == "colleaf_stale_bookmark_placement"
	}
	if !found {
		t.Fatal("list ownership loss not disclosed")
	}
}

func TestColleafFlowNavigationSoftBreakKeepsPoint(t *testing.T) {
	ctx := context.Background()
	wire, err := encodeColleafDocument(ctx, colleafFlowNavigationExample())
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range wire.Body.CustomRanges {
		if a.Type != 3 {
			continue
		}
		p, err := readColleafBookmark(a)
		if err != nil {
			t.Fatal(err)
		}
		if p.BookmarkID == "list-break" {
			wire.Body.RenderedPageBreaks = []int{a.StartIndex}
			break
		}
	}
	got, err := decodeColleafDocument(ctx, wire)
	if err != nil {
		t.Fatal(err)
	}
	bs := got.Blocks[0].Section.Blocks[1].Items[0].Blocks
	if len(bs) != 4 || bs[2].Kind != AnchorPoint || !reflect.DeepEqual(bs[2].Anchors, []string{"list-break"}) {
		t.Fatal("soft layout boundary swallowed point")
	}
	found := false
	for _, d := range got.Diagnostics {
		found = found || d.Code == "colleaf_soft_page_break"
	}
	if !found {
		t.Fatal("soft boundary loss not disclosed")
	}
}
