package manydoc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"testing"
)

func colleafStandaloneExample(t testing.TB) *Document {
	t.Helper()
	point := func(ids ...string) Block { return Block{Kind: AnchorPoint, Anchors: ids} }
	p := func(s string) Block { return Block{Kind: Paragraph, Runs: []Run{{Text: s}}} }
	list := Block{Kind: ListBlock, List: "bullet", Anchors: []string{"list"}, Items: []ListItem{{Blocks: []Block{p("Item"), point("item-first"), point("item-mid"), p("Tail"), point("item-end")}}, {Blocks: []Block{p("Two"), point("item-two-end")}}}}
	return &Document{Blocks: []Block{point("a", "alias"), point("b"), p("Lead 😀"), point("before-list"), list, point("after-list"), {Kind: Table, Rows: []Row{{Cells: []Cell{{Blocks: []Block{point("cell-start"), p("Cell"), point("cell-end")}}, {Blocks: []Block{point("only-cell")}}}}}}, point("before-image"), {Kind: Picture, Image: &Image{Data: runningPNG(t, false), MediaType: "image/png", Width: 12, Height: 9}}, point("before-break"), {Kind: PageBreak}, p("After"), point("end")}, Headers: []Block{point("header-start"), p("Header"), point("header-end")}, Notes: []Note{{Kind: Footnote, ID: "n", Blocks: []Block{point("only-note")}}}}
}
func TestColleafStandalonePointsRoundTrip(t *testing.T) {
	ctx := context.Background()
	want := colleafStandaloneExample(t)
	// Exercise references to point groups and terminal/empty native cursors.
	var links []Run
	for _, id := range []string{"a", "alias", "b", "before-list", "item-first", "item-mid", "item-end", "item-two-end", "after-list", "cell-start", "cell-end", "only-cell", "before-image", "before-break", "end"} {
		links = append(links, Run{Text: id, Destination: id})
	}
	links = append(links, Run{Note: &NoteReference{Kind: Footnote, ID: "n"}})
	want.Blocks[11].Runs = links
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
		t.Fatalf("point structure changed\ngot%s\nwant%s", a, b)
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
		t.Fatal("SF point grouping/flow ownership changed")
	}
	if file := os.Getenv("MANYDOC_COLLEAF_STANDALONE_QA"); file != "" {
		if err = os.WriteFile(file, before, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if file := os.Getenv("MANYDOC_COLLEAF_STANDALONE_SAVED"); file != "" {
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
		saved, err := readColleaf(ctx, bytes.NewReader(envelope))
		if err != nil {
			t.Fatal(err)
		}
		actual, err := ToStructured(saved)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(actual, sf) {
			a, _ := json.Marshal(actual)
			b, _ := json.Marshal(sf)
			t.Fatalf("actual native save changed\ngot%s\nwant%s", a, b)
		}
	}
	if file := os.Getenv("MANYDOC_COLLEAF_STANDALONE_EDITED"); file != "" {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var native colleafNoteSnapshot
		if err = json.Unmarshal(raw, &native); err != nil {
			t.Fatal(err)
		}
		actual, err := decodeColleafDocument(ctx, native)
		if err != nil {
			t.Fatal(err)
		}
		expected, err := FromStructured(sf)
		if err != nil {
			t.Fatal(err)
		}
		expected.Blocks = expected.Blocks[2:]
		expected.Blocks[0].Runs = append([]Run{{Text: "Q"}, {Anchor: "a"}, {Anchor: "alias"}, {Anchor: "b"}}, expected.Blocks[0].Runs...)
		expected.Diagnostics = []Diagnostic{{Code: "colleaf_stale_point_ownership", Message: "Standalone point retains its current native location because its owning flow or boundary changed"}}
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
			t.Fatalf("actual moved points changed graph\ngot%s\nwant%s", a, b)
		}
	}
}
func TestColleafStandaloneDenseItems(t *testing.T) {
	d := &Document{Blocks: []Block{{Kind: ListBlock, List: "bullet"}}}
	for i := 0; i < 256; i++ {
		d.Blocks[0].Items = append(d.Blocks[0].Items, ListItem{Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: fmt.Sprint(i)}}}, {Kind: AnchorPoint, Anchors: []string{fmt.Sprint("end-", i)}}}})
	}
	wire, err := encodeColleafDocument(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	got, err := decodeColleafDocument(context.Background(), wire)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(d, got) {
		t.Fatal("dense item-end points changed grouping")
	}
}

func TestColleafStandaloneCurrentNativeEdits(t *testing.T) {
	ctx := context.Background()
	d := &Document{Blocks: []Block{{Kind: AnchorPoint, Anchors: []string{"point"}}, {Kind: Paragraph, Runs: []Run{{Text: "Text"}}}}}
	wire, err := encodeColleafDocument(ctx, d)
	if err != nil {
		t.Fatal(err)
	}
	for i, a := range wire.Body.CustomRanges {
		if a.Type == 3 {
			wire.Body.CustomRanges[i].StartIndex = 2
			wire.Body.CustomRanges[i].EndIndex = 2
			break
		}
	}
	got, err := decodeColleafDocument(ctx, wire)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Blocks) != 1 || len(got.Blocks[0].Runs) != 3 || got.Blocks[0].Runs[1].Anchor != "point" {
		t.Fatal("moved point restored obsolete boundary")
	}
	if len(got.Diagnostics) != 1 || got.Diagnostics[0].Code != "colleaf_stale_point_ownership" {
		t.Fatal("ownership change not disclosed")
	}
	// Declaring an unrelated empty paragraph as the carrier cannot remove it.
	wire, err = encodeColleafDocument(ctx, &Document{Blocks: []Block{{Kind: AnchorPoint, Anchors: []string{"point"}}, {Kind: Paragraph, Runs: []Run{{Text: "Text"}}}, {Kind: Paragraph}}})
	if err != nil {
		t.Fatal(err)
	}
	for i, a := range wire.Body.CustomRanges {
		if a.Type == 3 {
			p, err := readColleafBookmark(a)
			if err != nil {
				t.Fatal(err)
			}
			p.Hint.PointCarrier = wire.Body.Paragraphs[1].ID
			wire.Body.CustomRanges[i].Properties = sfAttrs(p)
		}
	}
	got, err = decodeColleafDocument(ctx, wire)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Blocks) != 3 || got.Blocks[2].Kind != Paragraph {
		t.Fatal("unrelated authored empty paragraph discarded")
	}
	found := false
	for _, diag := range got.Diagnostics {
		found = found || diag.Code == "colleaf_stale_point_carrier"
	}
	if !found {
		t.Fatal("invalid carrier binding not disclosed")
	}
}
func TestColleafStandalonePointOnlyAndMalformed(t *testing.T) {
	ctx := context.Background()
	d := &Document{Blocks: []Block{{Kind: AnchorPoint, Anchors: []string{"a"}}, {Kind: AnchorPoint, Anchors: []string{"b", "c"}}}}
	wire, err := encodeColleafDocument(ctx, d)
	if err != nil {
		t.Fatal(err)
	}
	if wire.Body.DataStream != "\r\n" {
		t.Fatal("point-only body inserted extra visible structure")
	}
	got, err := decodeColleafDocument(ctx, wire)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(d, got) {
		t.Fatal("point-only groups changed")
	}
	for _, field := range []string{"pointScope", "pointOrder"} {
		bad := wire
		bad.Body.CustomRanges = append([]colleafRangeRecord(nil), wire.Body.CustomRanges...)
		p, err := readColleafBookmark(bad.Body.CustomRanges[0])
		if err != nil {
			t.Fatal(err)
		}
		if field == "pointScope" {
			p.Hint.PointScope = "unknown"
		} else {
			p.Hint.PointOrder = 0
		}
		bad.Body.CustomRanges[0].Properties = sfAttrs(p)
		if _, err = decodeColleafDocument(ctx, bad); err == nil {
			t.Fatal("malformed standalone intent accepted", field)
		}
	}
	bad := wire
	bad.Body.CustomRanges = append([]colleafRangeRecord(nil), wire.Body.CustomRanges...)
	p, err := readColleafBookmark(bad.Body.CustomRanges[2])
	if err != nil {
		t.Fatal(err)
	}
	p.Hint.PointIndex = 0
	bad.Body.CustomRanges[2].Properties = sfAttrs(p)
	if _, err = decodeColleafDocument(ctx, bad); err == nil {
		t.Fatal("duplicate alias order accepted")
	}
}

func TestColleafStandaloneExplicitSections(t *testing.T) {
	d := &Document{Blocks: []Block{{Kind: SectionBlock, Anchors: []string{"section"}, Section: &Section{Blocks: []Block{{Kind: AnchorPoint, Anchors: []string{"only"}}}}}, {Kind: SectionBlock, Section: &Section{Start: SectionNextPage, Blocks: []Block{{Kind: AnchorPoint, Anchors: []string{"start"}}, {Kind: Paragraph, Runs: []Run{{Text: "Back", Destination: "only"}, {Text: "Section", Destination: "section"}}}, {Kind: AnchorPoint, Anchors: []string{"end"}}}}}}}
	wire, err := encodeColleafDocument(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	got, err := decodeColleafDocument(context.Background(), wire)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(d, got) {
		a, _ := json.Marshal(got)
		b, _ := json.Marshal(d)
		t.Fatalf("section point graph changed\ngot%s\nwant%s", a, b)
	}
}
