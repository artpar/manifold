package manifold

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
)

func tableContinuationDocument(t testing.TB) *Document {
	t.Helper()
	data, err := os.ReadFile("examples/table-continuation/document.sf")
	if err != nil {
		t.Fatal(err)
	}
	sf, err := ReadStructured(context.Background(), bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	d, err := FromStructured(sf)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func checkTableContinuation(t *testing.T, d *Document) *Document {
	t.Helper()
	before, _ := json.Marshal(d)
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, PDF, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(d)
	if !bytes.Equal(before, after) {
		t.Fatal("continuation mutated input")
	}
	got, err := ReadPDF(context.Background(), bytes.NewReader(out.Bytes()), ReadOptions{PDFSkipInference: true, PDFSkipStructure: true})
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Join(strings.Fields(got.Text()), "")
	for _, token := range []string{"RIGHT_A", "RIGHT_C", "SPANNING_LAST", "AFTER_GRID", "NOTE_EARLY", "NOTE_LATE"} {
		if strings.Count(text, token) != 1 {
			t.Fatalf("lost/duplicated %s", token)
		}
	}
	for _, prefix := range []struct {
		name string
		n    int
	}{{"SPANNING_", 20}, {"RIGHT_B_", 12}} {
		for i := 0; i < prefix.n; i++ {
			token := fmt.Sprintf("%s%02d", prefix.name, i)
			if strings.Count(text, token) != 1 {
				t.Fatalf("lost/duplicated %s", token)
			}
		}
	}
	images := 0
	bold, link := false, false
	pages := map[string]int{}
	for _, b := range got.Blocks {
		if b.Kind == Picture {
			images++
			sameImagePixels(t, d.Blocks[0].Rows[0].Cells[0].Blocks[20].Image.Data, b.Image.Data)
			x0, y0, x1, y1 := quadBounds(*b.Source.Quad)
			if x0 < d.Layout.Margin || y0 < d.Layout.Margin || x1 > d.Layout.Width-d.Layout.Margin || y1 > d.Layout.Height-d.Layout.Margin {
				t.Fatal("continued image escaped page")
			}
		}
		for _, run := range b.Runs {
			if strings.TrimSpace(run.Text) == "" {
				continue
			}
			if run.Source == nil || run.Source.Quad == nil {
				t.Fatalf("missing physical provenance: %q", run.Text)
			}
			x0, y0, x1, y1 := quadBounds(*run.Source.Quad)
			if x0 < d.Layout.Margin-.05 || y0 < d.Layout.Margin-.05 || x1 > d.Layout.Width-d.Layout.Margin+.05 || y1 > d.Layout.Height-d.Layout.Margin+.05 {
				t.Fatalf("continued text escaped page: %q %v", run.Text, *run.Source.Quad)
			}
			if strings.Contains(run.Text, "SPANNING_02") {
				pages["early_ref"] = run.Source.Page
			}
			if strings.Contains(run.Text, "SPANNING_14") {
				pages["late_ref"] = run.Source.Page
			}
			if strings.Contains(run.Text, "NOTE_EARLY") {
				pages["early_note"] = run.Source.Page
			}
			if strings.Contains(run.Text, "NOTE_LATE") {
				pages["late_note"] = run.Source.Page
			}
			bold = bold || (run.Text == "SPANNING_00" && run.Bold)
			link = link || (run.Text == "SPANNING_LAST" && run.Link == "https://example.invalid/continued")
		}
	}
	if images != 1 {
		t.Fatalf("continued image count %d", images)
	}
	if !bold || !link {
		t.Fatal("continued cell lost formatting/relationship")
	}
	if pages["early_ref"] != pages["early_note"] || pages["late_ref"] != pages["late_note"] || pages["early_ref"] > pages["late_ref"] {
		t.Fatalf("footnote/reference page ownership %v", pages)
	}
	return got
}

func TestPDFTableContinuationContentAssetsAndNoteOwnership(t *testing.T) {
	d := tableContinuationDocument(t)
	got := checkTableContinuation(t, d)
	if got.Metadata.PageCount < 3 {
		t.Fatal("oversized group did not paginate")
	}
	for _, height := range []float64{180, 220, 260} {
		t.Run(fmt.Sprint(height), func(t *testing.T) {
			d := tableContinuationDocument(t)
			d.Layout.Height = height
			checkTableContinuation(t, d)
		})
	}
}

func TestPDFTableContinuationAfterExistingNote(t *testing.T) {
	d := &Document{Layout: Layout{Width: 240, Height: 110, Margin: 20},
		Notes:  []Note{{Kind: Footnote, ID: "before", Blocks: []Block{mergeParagraph("NOTE_BEFORE")}}, {Kind: Footnote, ID: "cell", Blocks: []Block{mergeParagraph("NOTE_CELL")}}},
		Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "PRE"}, noteRun(Footnote, "before")}}, {Kind: Table, Rows: []Row{{Cells: []Cell{{Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "CELL_REFERENCE"}, noteRun(Footnote, "cell")}}, mergeParagraph("CELL_SECOND"), mergeParagraph("CELL_THIRD")}}}}}}}}
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, PDF, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	got, err := ReadPDF(context.Background(), &out, ReadOptions{PDFSkipInference: true, PDFSkipStructure: true})
	if err != nil {
		t.Fatal(err)
	}
	pages := map[string]int{}
	for _, b := range got.Blocks {
		for _, run := range b.Runs {
			for _, token := range []string{"PRE", "NOTE_BEFORE", "CELL_REFERENCE", "NOTE_CELL", "CELL_SECOND", "CELL_THIRD"} {
				if strings.Contains(run.Text, token) {
					pages[token] = run.Source.Page
				}
			}
		}
	}
	if len(pages) != 6 || pages["PRE"] != pages["NOTE_BEFORE"] || pages["CELL_REFERENCE"] != pages["NOTE_CELL"] || pages["CELL_THIRD"] <= pages["CELL_REFERENCE"] {
		t.Fatalf("existing/new note admission or continuation lost: %v", pages)
	}
}

func TestTableContinuationSFDOCXSemantics(t *testing.T) {
	d := tableContinuationDocument(t)
	for _, format := range []Format{SF, DOCX} {
		var out bytes.Buffer
		if err := Write(context.Background(), &out, d, format, WriteOptions{}); err != nil {
			t.Fatal(err)
		}
		got, err := Read(context.Background(), &out, format, ReadOptions{})
		if err != nil {
			t.Fatal(err)
		}
		cell := got.Blocks[0].Rows[0].Cells[0]
		if cell.ColSpan != 2 || cell.RowSpan != 3 || len(cell.Blocks) != 22 || len(got.Notes) != 2 {
			t.Fatalf("lost merged flow semantics through %s", format)
		}
		if cell.Blocks[2].Runs[1].Note.ID != "early" || cell.Blocks[14].Runs[1].Note.ID != "late" || cell.Blocks[21].Runs[0].Link != "https://example.invalid/continued" {
			t.Fatal("note/URI ownership changed")
		}
		sameImagePixels(t, d.Blocks[0].Rows[0].Cells[0].Blocks[20].Image.Data, cell.Blocks[20].Image.Data)
	}
}

func FuzzPDFTableContinuation(f *testing.F) {
	f.Add(uint16(0), uint16(0))
	f.Add(uint16(99), uint16(70))
	f.Fuzz(func(t *testing.T, w, h uint16) {
		d := tableContinuationDocument(t)
		d.Layout.Width = 320 + float64(w%200)
		d.Layout.Height = 180 + float64(h%200)
		checkTableContinuation(t, d)
	})
}
