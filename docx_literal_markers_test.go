package manydoc

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestDOCXLiteralMarkerRichRoundtrip(t *testing.T) {
	d := literalMarkerDocument()
	d.Layout = Layout{Width: 595.3, Height: 841.9, Margin: 54}
	before := canonicalSF(t, d)
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, DOCX, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	got, err := ReadDOCX(context.Background(), bytes.NewReader(out.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, canonicalSF(t, got)) {
		t.Fatalf("literal graph changed\n%s\n%s", before, canonicalSF(t, got))
	}
	if !bytes.Equal(before, canonicalSF(t, d)) {
		t.Fatal("caller mutated")
	}
	if dir := os.Getenv("MANYDOC_LITERAL_DOCX_QA"); dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "rich.docx"), out.Bytes(), 0600); err != nil {
			t.Fatal(err)
		}
		var sf bytes.Buffer
		if err := Write(context.Background(), &sf, got, SF, WriteOptions{}); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "rich.sf"), sf.Bytes(), 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func exportDOCXLiteral(t *testing.T, name string, d *Document) *Document {
	t.Helper()
	before := canonicalSF(t, d)
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, DOCX, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	got, err := ReadDOCX(context.Background(), bytes.NewReader(out.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, canonicalSF(t, d)) {
		t.Fatal("caller mutated")
	}
	if !bytes.Equal(before, canonicalSF(t, got)) {
		t.Fatalf("%s semantic graph changed\n%s\n%s", name, canonicalSF(t, d), canonicalSF(t, got))
	}
	if dir := os.Getenv("MANYDOC_LITERAL_DOCX_QA"); dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name+".docx"), out.Bytes(), 0600); err != nil {
			t.Fatal(err)
		}
		var sf bytes.Buffer
		if err := Write(context.Background(), &sf, got, SF, WriteOptions{}); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name+".sf"), sf.Bytes(), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return got
}

func TestDOCXLiteralMarkerMixedAutomaticCounters(t *testing.T) {
	for _, start := range []int{0, 7} {
		b := Block{Kind: ListBlock, List: "number", Start: listStartPointer(start), Numbering: &ListNumbering{Template: "Item %1)", Formats: []string{"decimal"}}, Items: []ListItem{
			{Blocks: []Block{mergeParagraph("AUTO_FIRST")}},
			{Marker: &ListMarker{Blocks: []Block{mergeParagraph("[done]")}}, Blocks: []Block{mergeParagraph("LITERAL_BODY")}},
			{Blocks: []Block{mergeParagraph("AUTO_RESUMED")}},
			{Blocks: []Block{mergeParagraph("AUTO_NEXT")}},
			{Marker: &ListMarker{Blocks: []Block{{Kind: Paragraph}}}, Blocks: []Block{mergeParagraph("EMPTY_LABEL_BODY")}},
			{Blocks: []Block{mergeParagraph("AUTO_LAST")}},
		}}
		d := &Document{Layout: Layout{Width: 480, Height: 600, Margin: 24}, Blocks: []Block{b}}
		exportDOCXLiteral(t, fmt.Sprintf("mixed-%d", start), d)
	}
}

func TestDOCXLiteralMarkerMalformedAndVisibleEdits(t *testing.T) {
	d := literalMarkerDocument()
	d.Layout = Layout{Width: 595.3, Height: 841.9, Margin: 54}
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, DOCX, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(string) string{
		func(s string) string { return strings.Replace(s, `<w:tblGrid>`, `<w:tblPr/><w:tblGrid>`, 1) },
		func(s string) string { return strings.Replace(s, `md:literalMarker="1"`, `md:literalMarker="2"`, 1) },
		func(s string) string { return strings.Replace(s, `md:numberId="1"`, `md:numberId="99999"`, 1) },
		func(s string) string { return strings.Replace(s, `md:numberLevel="0"`, `md:numberLevel="8"`, 1) },
		func(s string) string { return strings.Replace(s, `md:markerSide="left"`, `md:markerSide="right"`, 1) },
		func(s string) string { return strings.Replace(s, `<w:top w:val="nil"/>`, `<w:top w:val="single"/>`, 1) },
		func(s string) string {
			return strings.Replace(s, `</w:tr>`, `<w:tc><w:p><w:r><w:t>EXTRA_VISIBLE</w:t></w:r></w:p></w:tc></w:tr>`, 1)
		},
	} {
		raw := rewriteListDOCX(t, out.Bytes(), change)
		if _, err := ReadDOCX(context.Background(), bytes.NewReader(raw)); err == nil {
			t.Fatal("invalid marker ownership/geometry accepted")
		}
	}
	raw := rewriteListDOCX(t, out.Bytes(), func(s string) string { return strings.Replace(s, "α)", "EDITED_LABEL", 1) })
	got, err := ReadDOCX(context.Background(), bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if paragraphText(got.Blocks[0].Items[0].Marker.Blocks[0]) != "EDITED_LABEL" {
		t.Fatal("cached original label hid actual XML edit")
	}
}

func TestDOCXLiteralMarkerNestedAssetsNotesAndRunning(t *testing.T) {
	raster := image.NewRGBA(image.Rect(0, 0, 1, 1))
	raster.Set(0, 0, color.RGBA{R: 255, A: 255})
	var data bytes.Buffer
	if err := png.Encode(&data, raster); err != nil {
		t.Fatal(err)
	}
	marker := &ListMarker{Blocks: []Block{mergeParagraph("RICH_LABEL"), {Kind: Picture, Image: &Image{Data: data.Bytes(), MediaType: "image/png", Width: 16, Height: 16, Alt: "RED_MARKER"}}, cellTable([]Block{mergeParagraph("INNER_TABLE")}), {Kind: ListBlock, List: "bullet", Items: []ListItem{{Blocks: []Block{mergeParagraph("INNER_LIST")}}}}}}
	list := Block{Kind: ListBlock, List: "bullet", Items: []ListItem{{Marker: marker, Blocks: []Block{mergeParagraph("RICH_BODY")}}}}
	d := &Document{Layout: Layout{Width: 480, Height: 600, Margin: 24}, Blocks: []Block{cellTable([]Block{list}), {Kind: Paragraph, Runs: []Run{{Text: "CALL"}, {Note: &NoteReference{Kind: Footnote, ID: "n"}}}}}, Notes: []Note{{Kind: Footnote, ID: "n", Blocks: []Block{list}}}}
	exportDOCXLiteral(t, "nested", d)
	d = &Document{Layout: d.Layout, Headers: []Block{list}, Blocks: []Block{mergeParagraph("PAGE_ONE"), {Kind: PageBreak}, mergeParagraph("PAGE_TWO")}}
	exportDOCXLiteral(t, "running", d)
}

func TestDOCXLiteralMarkerContinuesAndPageStart(t *testing.T) {
	item := ListItem{Marker: &ListMarker{Blocks: []Block{mergeParagraph("LABEL_FIRST")}}, Blocks: []Block{mergeParagraph("BODY_FIRST")}}
	for i := 0; i < 35; i++ {
		item.Marker.Blocks = append(item.Marker.Blocks, mergeParagraph(fmt.Sprintf("L%02d", i)))
		item.Blocks = append(item.Blocks, mergeParagraph(fmt.Sprintf("B%02d", i)))
	}
	d := &Document{Layout: Layout{Width: 300, Height: 240, Margin: 24}, Blocks: []Block{{Kind: ListBlock, List: "bullet", Items: []ListItem{item}}}}
	exportDOCXLiteral(t, "continued", d)
	item = ListItem{Marker: &ListMarker{Blocks: []Block{mergeParagraph("START_LABEL")}}, Blocks: []Block{{Kind: Paragraph, PageBreakBefore: true, Runs: []Run{{Text: "START_BODY"}}}}}
	d.Blocks = []Block{mergeParagraph("BEFORE"), {Kind: ListBlock, List: "bullet", Items: []ListItem{item}}}
	exportDOCXLiteral(t, "page-start", d)
	item.Blocks[0].PageBreakBefore = false
	d.Blocks = []Block{mergeParagraph("BEFORE"), {Kind: PageBreak}, {Kind: ListBlock, List: "bullet", Items: []ListItem{item}}}
	exportDOCXLiteral(t, "explicit-break", d)
}

func TestDOCXLiteralMarkerRTL(t *testing.T) {
	d := &Document{Layout: Layout{Width: 300, Height: 240, Margin: 24}, Blocks: []Block{{Kind: ListBlock, List: "bullet", Items: []ListItem{{Marker: &ListMarker{Blocks: []Block{mergeParagraph("RIGHT_LABEL")}}, Blocks: []Block{{Kind: Paragraph, Direction: "rtl", Runs: []Run{{Text: "RTL_BODY"}}}}}}}}}
	exportDOCXLiteral(t, "rtl", d)
}

func TestDOCXLiteralMarkerCheckedControls(t *testing.T) {
	d := literalMarkerDocument()
	d.Layout = Layout{Width: 480, Height: 600, Margin: 24}
	d.Blocks[0].Items[0].Blocks[0].PageBreakBefore = true
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, DOCX, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(string) string{
		func(s string) string {
			return strings.Replace(s, `md:literalPageStart="conditional"`, `md:literalPageStart="unknown"`, 1)
		},
		func(s string) string {
			s = strings.Replace(s, `<w:pPr><w:pageBreakBefore/>`, `<x:pPr xmlns:x="urn:invalid"><w:pageBreakBefore/>`, 1)
			return strings.Replace(s, `</w:pPr>`, `</x:pPr>`, 1)
		},
		func(s string) string {
			return strings.Replace(s, `</w:pPr></w:p><w:tbl md:literalMarker`, `</w:pPr><w:r><w:t>HIDDEN</w:t></w:r></w:p><w:tbl md:literalMarker`, 1)
		},
	} {
		raw := rewriteListDOCX(t, out.Bytes(), change)
		if bytes.Equal(raw, out.Bytes()) {
			t.Fatal("test did not mutate source")
		}
		if _, err := ReadDOCX(context.Background(), bytes.NewReader(raw)); err == nil {
			t.Fatal("invalid page-start accepted")
		}
	}
	// Literal markers in note stories use real block ranges, not a nested layout
	// cell that LibreOffice omits. The standard counter reference is still checked.
	d = &Document{Layout: d.Layout, Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Note: &NoteReference{Kind: Endnote, ID: "e"}}}}}, Notes: []Note{{Kind: Endnote, ID: "e", Blocks: []Block{{Kind: ListBlock, List: "bullet", Items: []ListItem{{Marker: &ListMarker{Blocks: []Block{mergeParagraph("END_LABEL")}}, Blocks: []Block{mergeParagraph("END_BODY")}}}}}}}}
	out.Reset()
	if err := Write(context.Background(), &out, d, DOCX, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(string) string{
		func(s string) string {
			return regexp.MustCompile(`mdmarker1_([0-9]+)_([0-9]+)_`).ReplaceAllString(s, `mdmarker1_99999_${2}_`)
		},
		func(s string) string {
			return regexp.MustCompile(`mdmarker1_([0-9]+)_([0-9]+)_`).ReplaceAllString(s, `mdmarker1_${1}_9_`)
		},
	} {
		raw := rewriteRichNoteParts(t, out.Bytes(), change)
		if _, err := ReadDOCX(context.Background(), bytes.NewReader(raw)); err == nil {
			t.Fatal("invalid note marker binding accepted")
		}
	}
	exportDOCXLiteral(t, "endnote", d)
}
