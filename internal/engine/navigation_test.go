package engine

import (
	"bytes"
	"context"
	"os"
	"reflect"
	"testing"
)

func destinationDocument() *Document {
	return &Document{Layout: Layout{Width: 320, Height: 220, Margin: 20}, UnresolvedDestinations: []string{"missing"}, Blocks: []Block{
		{Kind: Paragraph, Anchors: []string{"start"}, Runs: []Run{{Text: "FORWARD", Link: "#late", Destination: "late"}, {Text: " URI", Link: "#literal"}, {Text: " BROKEN", Link: "#missing", Destination: "missing"}}},
		{Kind: Paragraph, Runs: []Run{{Text: "of"}, {Anchor: "ligature"}, {Text: "fice"}}},
		{Kind: PageBreak},
		{Kind: Paragraph, Anchors: []string{"late"}, Runs: []Run{{Anchor: "章 space–%"}, {Text: "TARGET"}, {Text: " BACK", Destination: "start"}}},
		{Kind: AnchorPoint, Anchors: []string{"tail"}},
	}}
}

func TestDestinationSFAndDOCXSemantics(t *testing.T) {
	d := destinationDocument()
	ctx := context.Background()
	expected := canonicalSF(t, d)
	for _, format := range []Format{SF, DOCX, JSON} {
		var out bytes.Buffer
		if err := Write(ctx, &out, d, format, WriteOptions{}); err != nil {
			t.Fatal(format, err)
		}
		back, err := Read(ctx, &out, format, ReadOptions{})
		if err != nil {
			t.Fatal(format, err)
		}
		if !bytes.Equal(expected, canonicalSF(t, back)) {
			t.Fatalf("%s anchor/destination semantics changed\nwant %s\ngot %s", format, expected, canonicalSF(t, back))
		}
		if len(back.Diagnostics) > 0 {
			t.Fatal(format, back.Diagnostics)
		}
	}
	if !bytes.Equal(expected, canonicalSF(t, d)) {
		t.Fatal("caller mutated")
	}
	var pdf bytes.Buffer
	if err := Write(ctx, &pdf, d, PDF, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	back, err := ReadPDF(ctx, &pdf, ReadOptions{PDFSkipInference: true})
	if err != nil {
		t.Fatal(err)
	}
	if back.Metadata.PageCount != 2 {
		t.Fatal("anchor changed page flow", back.Metadata.PageCount)
	}
}

func TestDestinationGraphAdmission(t *testing.T) {
	for _, mutate := range []func(*Document){
		func(d *Document) { d.Blocks[1].Runs[1].Anchor = "start" },
		func(d *Document) { d.UnresolvedDestinations = append(d.UnresolvedDestinations, "start") },
		func(d *Document) { d.UnresolvedDestinations = nil },
		func(d *Document) { d.Blocks[0].Runs[0].Link = "https://example.invalid/#late" },
		func(d *Document) { d.Blocks[1].Runs[1].Text = "VISIBLE" },
		func(d *Document) { d.Blocks[1].Runs[1].Bold = true },
	} {
		d := destinationDocument()
		mutate(d)
		var out bytes.Buffer
		if err := Write(context.Background(), &out, d, SF, WriteOptions{}); err == nil || out.Len() != 0 {
			t.Fatal("bad destination graph emitted", err)
		}
	}
}

func TestRealWordBookmarkIdentity(t *testing.T) {
	raw, err := os.ReadFile("testdata/real/mqtt-v5.0-os.docx")
	if err != nil {
		t.Fatal(err)
	}
	d, err := ReadDOCX(context.Background(), bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	c, err := catalogDestinations(d, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.anchors) != 3106 || len(c.links) != 379 || !reflect.DeepEqual(d.UnresolvedDestinations, []string{"_Introduction"}) {
		t.Fatal("original bookmark/link identities lost", len(c.anchors), len(c.links), d.UnresolvedDestinations)
	}
}

func TestDestinationCellAndBoundarySemantics(t *testing.T) {
	d := destinationDocument()
	d.Blocks = append(d.Blocks[:2], append([]Block{
		{Kind: AnchorPoint, Anchors: []string{"between", "alias"}},
		{Kind: Table, Rows: []Row{{Cells: []Cell{
			{Blocks: []Block{{Kind: AnchorPoint, Anchors: []string{"empty-cell", "cell-alias"}}}},
			{Blocks: []Block{{Kind: Table, Rows: []Row{{Cells: []Cell{{Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "NESTED"}}}}}}}}}, {Kind: AnchorPoint, Anchors: []string{"nested-tail"}}}},
		}}}},
	}, d.Blocks[2:]...)...)
	expected := canonicalSF(t, d)
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, DOCX, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	back, err := ReadDOCX(context.Background(), &out)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(expected, canonicalSF(t, back)) {
		t.Fatalf("boundary/cell anchors changed\nwant %s\ngot %s", expected, canonicalSF(t, back))
	}
}

func TestDestinationSectionSemantics(t *testing.T) {
	d := &Document{Blocks: []Block{
		{Kind: SectionBlock, Anchors: []string{"section-one"}, Section: &Section{Blocks: []Block{{Kind: Paragraph, Anchors: []string{"paragraph"}, Runs: []Run{{Text: "FORWARD", Destination: "section-two"}}}}}},
		{Kind: SectionBlock, Anchors: []string{"section-two"}, Section: &Section{Start: SectionNextPage}},
	}}
	expected := canonicalSF(t, d)
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, DOCX, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	back, err := ReadDOCX(context.Background(), &out)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(expected, canonicalSF(t, back)) {
		t.Fatalf("section anchors changed\nwant %s\ngot %s", expected, canonicalSF(t, back))
	}
	var pdf bytes.Buffer
	if err := Write(context.Background(), &pdf, d, PDF, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
}
