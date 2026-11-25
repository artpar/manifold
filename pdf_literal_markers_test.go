package manifold

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPDFLiteralMarkersRichOwnership(t *testing.T) {
	d := literalMarkerDocument()
	before := canonicalSF(t, d)
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, PDF, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, canonicalSF(t, d)) {
		t.Fatal("caller mutated")
	}
	got, err := ReadPDF(context.Background(), bytes.NewReader(out.Bytes()), ReadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Blocks) != 1 || got.Blocks[0].Kind != ListBlock || len(got.Blocks[0].Items) != 3 {
		t.Fatalf("list hierarchy: %#v", got.Blocks)
	}
	items := got.Blocks[0].Items
	for i, item := range items {
		if item.Marker == nil || item.Marker.Source.Role != "Lbl" || item.Marker.Source.ParentElement != item.Source.Element {
			t.Fatalf("marker %d ownership lost", i)
		}
		if !strings.Contains(layoutText(item.Blocks[0]), []string{"FIRST", "SECOND", "EMPTY"}[i]) {
			t.Fatal("body correspondence")
		}
	}
	first := items[0].Marker.Blocks[0].Runs
	if len(first) == 0 || first[0].Text != "α)" || !first[0].Bold || first[0].Link != "https://example.org/alpha" {
		t.Fatalf("label formatting/URI lost: %+v", first)
	}
	noteFound := false
	for _, b := range items[1].Marker.Blocks {
		for _, r := range b.Runs {
			if r.Note != nil {
				noteFound = true
			}
		}
	}
	if !noteFound || len(got.Notes) != 1 || !strings.Contains(got.Text(), "Marker note") {
		t.Fatalf("marker citation graph lost: %+v", got.Notes)
	}
	assertRecoverySF(t, got)
	exportLiteralOutput(t, "rich", out.Bytes(), got)
}

func exportLiteralOutput(t *testing.T, name string, raw []byte, d *Document) {
	t.Helper()
	dir := os.Getenv("MANYDOC_LITERAL_OUTPUT_QA")
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name+".pdf"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	var sf bytes.Buffer
	if err := Write(context.Background(), &sf, d, SF, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name+".sf"), sf.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestPDFLiteralMarkersParallelPagination(t *testing.T) {
	d := literalMarkerDocument()
	d.Layout = Layout{Width: 300, Height: 240, Margin: 24}
	item := &d.Blocks[0].Items[0]
	for i := 0; i < 28; i++ {
		item.Marker.Blocks = append(item.Marker.Blocks, mergeParagraph(fmt.Sprintf("LBL_%02d", i)))
		item.Blocks = append(item.Blocks, mergeParagraph(fmt.Sprintf("BODY_%02d", i)))
	}
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, PDF, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	got, err := ReadPDF(context.Background(), bytes.NewReader(out.Bytes()), ReadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Pages) < 3 || len(got.Blocks) != 1 || got.Blocks[0].Kind != ListBlock {
		t.Fatal("missing continued logical list")
	}
	for _, prefix := range []string{"LBL_", "BODY_"} {
		for i := 0; i < 28; i++ {
			token := fmt.Sprintf("%s%02d", prefix, i)
			if strings.Count(got.Text(), token) != 1 {
				t.Fatal("lost/duplicated", token)
			}
		}
	}
	// Every painted line is within the page, in its independent parallel column.
	labelPages := map[int]bool{}
	bodyPages := map[int]bool{}
	for _, pair := range []struct {
		blocks []Block
		label  bool
	}{{got.Blocks[0].Items[0].Marker.Blocks, true}, {got.Blocks[0].Items[0].Blocks, false}} {
		for _, b := range pair.blocks {
			for _, r := range b.Runs {
				if r.Source == nil || r.Source.Quad == nil {
					continue
				}
				x0, y0, x1, y1 := quadBounds(*r.Source.Quad)
				if x0 < 24 || x1 > 276 || y0 < 24 || y1 > 216 {
					t.Fatalf("escaped page: %+v", r.Source)
				}
				if strings.HasPrefix(r.Text, "LBL_") {
					if x1 > 87 {
						t.Fatal("label invaded body")
					}
					labelPages[r.Source.Page] = true
				}
				if strings.HasPrefix(r.Text, "BODY_") {
					if x0 < 93 {
						t.Fatal("body invaded label")
					}
					bodyPages[r.Source.Page] = true
				}
			}
		}
	}
	if len(labelPages) < 3 || len(bodyPages) < 3 {
		t.Fatal("independent columns did not continue")
	}
	exportLiteralOutput(t, "continued", out.Bytes(), got)
}

func TestPDFLiteralMarkersNestedAssetsAndStories(t *testing.T) {
	raster := image.NewRGBA(image.Rect(0, 0, 1, 1))
	raster.Set(0, 0, color.RGBA{R: 255, A: 255})
	var asset bytes.Buffer
	if err := png.Encode(&asset, raster); err != nil {
		t.Fatal(err)
	}
	label := []Block{mergeParagraph("RICH_LABEL"), {Kind: Picture, Image: &Image{Data: asset.Bytes(), Width: 16, Height: 16, Alt: "RED_MARKER"}}, {Kind: Table, Rows: []Row{{Cells: []Cell{{Blocks: []Block{mergeParagraph("INNER_LABEL")}}}}}}}
	list := Block{Kind: ListBlock, List: "bullet", Items: []ListItem{{Marker: &ListMarker{Blocks: label}, Blocks: []Block{mergeParagraph("RICH_BODY")}}}}
	d := &Document{Blocks: []Block{{Kind: Table, Rows: []Row{{Cells: []Cell{{Blocks: []Block{list}}}}}}}, Notes: []Note{{Kind: Footnote, ID: "n", Blocks: []Block{list}}}}
	d.Blocks = append(d.Blocks, Block{Kind: Paragraph, Runs: []Run{{Text: "CALL"}, {Note: &NoteReference{Kind: Footnote, ID: "n"}}}})
	before := canonicalSF(t, d)
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, PDF, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, canonicalSF(t, d)) {
		t.Fatal("nested caller mutated")
	}
	got, err := ReadPDF(context.Background(), bytes.NewReader(out.Bytes()), ReadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Blocks[0].Kind != Table {
		t.Fatal("outer table lost")
	}
	child := got.Blocks[0].Rows[0].Cells[0].Blocks[0]
	if child.Kind != ListBlock || child.Items[0].Marker == nil {
		t.Fatal("nested list lost")
	}
	marker := child.Items[0].Marker.Blocks
	if len(marker) != 3 || marker[1].Kind != Picture || marker[2].Kind != Table {
		t.Fatalf("marker rich structure lost: %+v", marker)
	}
	sameImagePixels(t, asset.Bytes(), marker[1].Image.Data)
	if marker[1].Image.Alt != "RED_MARKER" || len(got.Notes) != 1 {
		t.Fatal("alt or note lost")
	}
	noteList := got.Notes[0].Blocks[0]
	if noteList.Kind != ListBlock || noteList.Items[0].Marker == nil || noteList.Items[0].Marker.Blocks[1].Kind != Picture {
		t.Fatal("note marker lost")
	}
	exportLiteralOutput(t, "nested", out.Bytes(), got)
	// Running-story markers use the same immutable planner and artifact painting.
	d = &Document{Blocks: []Block{mergeParagraph("BODY_PAGE_ONE"), {Kind: PageBreak}, mergeParagraph("BODY_PAGE_TWO")}, Headers: []Block{list}}
	before = canonicalSF(t, d)
	out.Reset()
	if err := Write(context.Background(), &out, d, PDF, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, canonicalSF(t, d)) {
		t.Fatal("running caller mutated")
	}
	physical, err := ReadPDF(context.Background(), bytes.NewReader(out.Bytes()), ReadOptions{PDFSkipStructure: true, PDFSkipInference: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(physical.Pages) != 2 || strings.Count(physical.Text(), "RICH_LABEL") != 2 || strings.Count(physical.Text(), "INNER_LABEL") != 2 {
		t.Fatal("running marker missing")
	}
	exportLiteralOutput(t, "running", out.Bytes(), physical)
}

func TestPDFLiteralMarkerUnsupportedGlyphAtomic(t *testing.T) {
	d := literalMarkerDocument()
	d.Blocks[0].Items[0].Marker.Blocks[0].Runs[0].Text = "😀"
	var out bytes.Buffer
	err := Write(context.Background(), &out, d, PDF, WriteOptions{})
	if !errors.Is(err, ErrUnsupported) || out.Len() != 0 {
		t.Fatal("missing marker glyph did not reject atomically", err)
	}
}

func TestPDFLiteralNumericLabelsRemainEditableBlocks(t *testing.T) {
	for _, label := range []string{"1.", "•", ""} {
		item := ListItem{Marker: &ListMarker{Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: label, Bold: true}}}}}, Blocks: []Block{mergeParagraph("BODY")}}
		d := &Document{Blocks: []Block{{Kind: ListBlock, List: "number", Items: []ListItem{item}}}}
		var out bytes.Buffer
		if err := Write(context.Background(), &out, d, PDF, WriteOptions{}); err != nil {
			t.Fatal(err)
		}
		got, err := ReadPDF(context.Background(), &out, ReadOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if len(got.Blocks) != 1 || got.Blocks[0].Kind != ListBlock || got.Blocks[0].Items[0].Marker == nil {
			t.Fatal("literal block mistaken for automatic counter", label)
		}
		marker := got.Blocks[0].Items[0].Marker
		if paragraphText(marker.Blocks[0]) != label || label != "" && !marker.Blocks[0].Runs[0].Bold {
			t.Fatal("literal text/style changed", label)
		}
	}
}

func TestPDFLiteralMarkerNestedListAndRTLColumns(t *testing.T) {
	nested := Block{Kind: ListBlock, List: "number", Items: []ListItem{{Blocks: []Block{mergeParagraph("CHILD")}}}}
	marker := &ListMarker{Blocks: []Block{mergeParagraph("LABEL"), nested}}
	item := ListItem{Marker: marker, Blocks: []Block{{Kind: Paragraph, Direction: "rtl", Runs: []Run{{Text: "BODY"}}}}}
	d := &Document{Layout: Layout{Width: 300, Height: 240, Margin: 24}, Blocks: []Block{{Kind: ListBlock, List: "bullet", Items: []ListItem{item}}}}
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, PDF, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	got, err := ReadPDF(context.Background(), bytes.NewReader(out.Bytes()), ReadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	exportLiteralOutput(t, "narrow-rtl", out.Bytes(), got)
	label := got.Blocks[0].Items[0].Marker.Blocks
	if len(label) != 2 || label[1].Kind != ListBlock || paragraphText(label[1].Items[0].Blocks[0]) != "CHILD" {
		t.Fatalf("nested marker list lost: %s", canonicalSF(t, got))
	}
	labelX, _, _, _ := quadBounds(*label[0].Runs[0].Source.Quad)
	bodyX, _, _, _ := quadBounds(*got.Blocks[0].Items[0].Blocks[0].Runs[0].Source.Quad)
	if labelX <= bodyX || labelX < 219 {
		t.Fatal("RTL marker column not on right", labelX, bodyX)
	}
}
