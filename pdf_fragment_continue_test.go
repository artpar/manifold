package manydoc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
)

func recursiveTableDocument(t testing.TB) *Document {
	t.Helper()
	data, err := os.ReadFile("examples/recursive-tables/document.sf")
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

func recursiveTokens() []string {
	tokens := []string{"OUTER_START", "OUTER_END", "OUTER_A", "OUTER_B", "MIDDLE_START", "MIDDLE_END", "MID_A", "MID_B", "SIDE_A", "SIDE_C", "DEEP_END", "BODY_AFTER", "NOTE_EARLY", "NOTE_LATE", "NOTE_A", "NOTE_B", "NOTE_END", "NI_A", "NI_B"}
	for _, set := range []struct {
		prefix string
		count  int
	}{{"D_", 18}, {"SB_", 8}, {"N_", 14}, {"NI_", 10}} {
		for i := 0; i < set.count; i++ {
			tokens = append(tokens, fmt.Sprintf("%s%02d", set.prefix, i))
		}
	}
	return tokens
}

type pdfLocation struct {
	page int
	x, y float64
}

func checkRecursiveTables(t *testing.T, d *Document) *Document {
	t.Helper()
	before, _ := json.Marshal(d)
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, PDF, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(d)
	if !bytes.Equal(before, after) {
		t.Fatal("recursive planning mutated source")
	}
	got, err := ReadPDF(context.Background(), bytes.NewReader(out.Bytes()), ReadOptions{PDFSkipInference: true})
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Join(strings.Fields(got.Text()), "")
	for _, token := range recursiveTokens() {
		if strings.Count(text, token) != 1 {
			t.Fatalf("recursive content lost/repeated: %s", token)
		}
	}
	locations := map[string]pdfLocation{}
	pictures := 0
	bold, bodyLink, noteLink := false, false, false
	originalBody := d.Blocks[0].Rows[0].Cells[0].Blocks[1].Rows[0].Cells[0].Blocks[1].Rows[0].Cells[0].Blocks[18].Image
	originalNote := d.Notes[1].Blocks[1].Rows[0].Cells[0].Blocks[15].Image
	for _, b := range got.Blocks {
		if b.Kind == Picture {
			pictures++
			original := originalBody
			if b.Image.Width < 29 {
				original = originalNote
			}
			sameImagePixels(t, original.Data, b.Image.Data)
			x0, y0, x1, y1 := quadBounds(*b.Source.Quad)
			if x0 < 19.95 || y0 < 19.95 || x1 > d.Layout.Width-19.95 || y1 > d.Layout.Height-19.95 {
				t.Fatal("recursive image escaped page")
			}
		}
		for _, run := range b.Runs {
			if strings.TrimSpace(run.Text) == "" {
				continue
			}
			if run.Source == nil || run.Source.Quad == nil {
				t.Fatalf("missing source for %q", run.Text)
			}
			x0, y0, x1, y1 := quadBounds(*run.Source.Quad)
			if x0 < 19.95 || y0 < 19.95 || x1 > d.Layout.Width-19.95 || y1 > d.Layout.Height-19.95 {
				t.Fatalf("recursive text escaped page: %q %+v", run.Text, *run.Source.Quad)
			}
			for _, token := range recursiveTokens() {
				if strings.Contains(run.Text, token) {
					locations[token] = pdfLocation{run.Source.Page, x0, y0}
				}
			}
			bold = bold || run.Text == "D_00" && run.Bold
			bodyLink = bodyLink || run.Text == "DEEP_END" && run.Link == "https://example.invalid/deep"
			noteLink = noteLink || run.Text == "NOTE_END" && run.Link == "https://example.invalid/note"
		}
	}
	if pictures != 2 || !bold || !bodyLink || !noteLink {
		t.Fatal("recursive assets/marks/relationships changed")
	}
	if locations["D_01"].page != locations["NOTE_EARLY"].page || locations["D_14"].page != locations["NOTE_LATE"].page {
		t.Fatalf("recursive note starts moved away from references: %v", locations)
	}
	for _, set := range []struct {
		prefix string
		count  int
	}{{"D_", 18}, {"SB_", 8}, {"N_", 14}, {"NI_", 10}} {
		var previous pdfLocation
		for i := 0; i < set.count; i++ {
			loc, ok := locations[fmt.Sprintf("%s%02d", set.prefix, i)]
			if !ok {
				t.Fatalf("missing token position %s%d", set.prefix, i)
			}
			if loc.page < previous.page || loc.page == previous.page && loc.y <= previous.y {
				t.Fatalf("recursive cell order changed: %s%d", set.prefix, i)
			}
			previous = loc
		}
	}
	// A reference's admitted body fragment can include later text on that same
	// page. Subsequent pages draining its note must contain only note content.
	for _, token := range recursiveTokens() {
		if !(strings.HasPrefix(token, "D_") || strings.HasPrefix(token, "SB_") || strings.HasPrefix(token, "OUTER_") || strings.HasPrefix(token, "MID") || strings.HasPrefix(token, "SIDE_") || token == "DEEP_END" || token == "BODY_AFTER") {
			continue
		}
		loc := locations[token]
		if loc.page > locations["D_14"].page && loc.page <= locations["NOTE_END"].page {
			t.Fatalf("body resumed on note continuation page: %s", token)
		}
	}
	return got
}

func TestPDFRecursiveFootnoteStartsWithOversizedTable(t *testing.T) {
	d := recursiveTableDocument(t)
	d.Notes[1].Blocks = d.Notes[1].Blocks[1:]
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, PDF, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	got, err := ReadPDF(context.Background(), &out, ReadOptions{PDFSkipInference: true})
	if err != nil {
		t.Fatal(err)
	}
	pages := map[string]int{}
	for _, b := range got.Blocks {
		for _, run := range b.Runs {
			for _, token := range []string{"D_14", "N_00", "NOTE_END"} {
				if strings.Contains(run.Text, token) {
					pages[token] = run.Source.Page
				}
			}
		}
	}
	if len(pages) != 3 || pages["D_14"] != pages["N_00"] || pages["NOTE_END"] <= pages["N_00"] {
		t.Fatalf("oversized table note did not start at its reference: %v", pages)
	}
}

func TestPDFRecursiveTableAndFootnoteContinuation(t *testing.T) {
	for _, height := range []float64{240, 220, 300} {
		t.Run(fmt.Sprint(height), func(t *testing.T) {
			d := recursiveTableDocument(t)
			d.Layout.Height = height
			checkRecursiveTables(t, d)
		})
	}
}

func TestRecursiveTableSFDOCXSemantics(t *testing.T) {
	d := recursiveTableDocument(t)
	for _, format := range []Format{SF, DOCX} {
		var out bytes.Buffer
		if err := Write(context.Background(), &out, d, format, WriteOptions{}); err != nil {
			t.Fatal(err)
		}
		got, err := Read(context.Background(), &out, format, ReadOptions{})
		if err != nil {
			t.Fatal(err)
		}
		outer := got.Blocks[0].Rows[0].Cells[0]
		middle := outer.Blocks[1].Rows[0].Cells[0]
		deep := middle.Blocks[1].Rows[0].Cells[0]
		if outer.RowSpan != 2 || outer.ColSpan != 2 || middle.RowSpan != 2 || middle.ColSpan != 2 || deep.RowSpan != 3 || deep.ColSpan != 2 {
			t.Fatalf("recursive merges changed through %s", format)
		}
		if deep.Blocks[1].Runs[1].Note.ID != "early" || deep.Blocks[14].Runs[1].Note.ID != "late" || deep.Blocks[19].Runs[0].Link != "https://example.invalid/deep" {
			t.Fatal("recursive relationships changed")
		}
		if got.Notes[1].Blocks[1].Rows[0].Cells[0].RowSpan != 2 {
			t.Fatal("footnote merge lost")
		}
	}
}

func TestPDFRecursiveUnrepresentableImageAtomicFailure(t *testing.T) {
	d := recursiveTableDocument(t)
	d.Notes[1].Blocks[1].Rows[0].Cells[0].Blocks[15].Image.Width = 1
	d.Notes[1].Blocks[1].Rows[0].Cells[0].Blocks[15].Image.Height = 1e308
	var out bytes.Buffer
	err := Write(context.Background(), &out, d, PDF, WriteOptions{})
	if !errors.Is(err, ErrUnsupported) || out.Len() != 0 {
		t.Fatalf("recursive unsupported output not atomic: %v bytes=%d", err, out.Len())
	}
}

func FuzzPDFRecursiveContinuation(f *testing.F) {
	f.Add(uint16(0), uint16(0))
	f.Add(uint16(130), uint16(83))
	f.Fuzz(func(t *testing.T, w, h uint16) {
		d := recursiveTableDocument(t)
		d.Layout.Width = 480 + float64(w%180)
		d.Layout.Height = 220 + float64(h%180)
		checkRecursiveTables(t, d)
	})
}
