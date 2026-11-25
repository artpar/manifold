package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"math"
	"os"
	"strings"
	"testing"
)

func richCellDocument(t testing.TB) *Document {
	t.Helper()
	data, e := os.ReadFile("examples/rich-cells/document.sf")
	if e != nil {
		t.Fatal(e)
	}
	s, e := ReadStructured(context.Background(), bytes.NewReader(data))
	if e != nil {
		t.Fatal(e)
	}
	d, e := FromStructured(s)
	if e != nil {
		t.Fatal(e)
	}
	return d
}
func sameImagePixels(t *testing.T, a, b []byte) {
	t.Helper()
	x, _, e := image.Decode(bytes.NewReader(a))
	if e != nil {
		t.Fatal(e)
	}
	y, _, e := image.Decode(bytes.NewReader(b))
	if e != nil {
		t.Fatal(e)
	}
	if x.Bounds().Size() != y.Bounds().Size() {
		t.Fatal("image pixel dimensions changed")
	}
	for row := 0; row < x.Bounds().Dy(); row++ {
		for col := 0; col < x.Bounds().Dx(); col++ {
			if color.NRGBAModel.Convert(x.At(col, row)) != color.NRGBAModel.Convert(y.At(col, row)) {
				t.Fatal("image pixel changed")
			}
		}
	}
}
func TestPDFRichCellsMergedGeometryNotesAndImages(t *testing.T) {
	d := richCellDocument(t)
	before, _ := json.Marshal(d)
	var out bytes.Buffer
	if e := Write(context.Background(), &out, d, PDF, WriteOptions{}); e != nil {
		t.Fatal(e)
	}
	got, e := ReadPDF(context.Background(), bytes.NewReader(out.Bytes()), ReadOptions{PDFSkipInference: true, PDFSkipStructure: true})
	if e != nil {
		t.Fatal(e)
	}
	if got.Metadata.PageCount != 2 {
		t.Fatalf("merged group/note admission pages: %d", got.Metadata.PageCount)
	}
	var pictures []Block
	for _, b := range got.Blocks {
		if b.Kind == Picture {
			pictures = append(pictures, b)
		}
	}
	if len(pictures) != 3 {
		t.Fatalf("got %d image placements, want 3", len(pictures))
	}
	want := [][4]float64{{36, 50, 228, 114}, {42, 192, 30, 20}, {48, 336, 12, 6}}
	original := [][]byte{d.Blocks[1].Rows[0].Cells[0].Blocks[1].Image.Data, d.Blocks[1].Rows[0].Cells[0].Blocks[2].Rows[0].Cells[0].Blocks[1].Image.Data, d.Notes[1].Blocks[1].Image.Data}
	for i, b := range pictures {
		if b.Source.Page != 2 {
			t.Fatal("image escaped admitted table/note page")
		}
		x0, y0, x1, y1 := quadBounds(*b.Source.Quad)
		values := []float64{x0, y0, x1 - x0, y1 - y0}
		for j, v := range values {
			if math.Abs(v-want[i][j]) > .02 {
				t.Fatalf("image %d coordinates: %v want %v", i, values, want[i])
			}
		}
		sameImagePixels(t, original[i], b.Image.Data)
	}
	for _, word := range []string{"CELL_START", "CELL_END", "INNER_A", "INNER_B", "INNER_C", "TOP_RIGHT", "BOTTOM_RIGHT", "AFTER_TABLE", "NOTE_CELL", "NOTE_NESTED"} {
		if strings.Count(got.Text(), word) != 1 {
			t.Fatalf("omitted/repeated content: %s", word)
		}
	}
	after, _ := json.Marshal(d)
	if !bytes.Equal(before, after) {
		t.Fatal("measurement/rendering mutated input")
	}
	// The SF persists the requested sizes and full rich tree independently of
	// the format-specific physical fitting performed during rendering.
	var sf bytes.Buffer
	if e = Write(context.Background(), &sf, d, SF, WriteOptions{}); e != nil {
		t.Fatal(e)
	}
	back, e := Read(context.Background(), &sf, SF, ReadOptions{})
	if e != nil {
		t.Fatal(e)
	}
	encoded, _ := json.Marshal(back)
	if !bytes.Equal(before, encoded) {
		t.Fatal("SF rich semantics changed")
	}
}

func TestPDFRichFootnoteContinuationUsesFragmentHeights(t *testing.T) {
	asset := richCellDocument(t).Notes[1].Blocks[1].Image
	copy := *asset
	copy.Width, copy.Height = 40, 20
	rows := []Row{}
	for i := 0; i < 12; i++ {
		rows = append(rows, Row{Cells: []Cell{{Blocks: []Block{mergeParagraph(fmt.Sprintf("NOTE_ROW_%02d", i))}}}})
	}
	d := &Document{Layout: Layout{Width: 300, Height: 200, Margin: 20}, Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "REFERENCE"}, noteRun(Footnote, "rich")}}, mergeParagraph("BODY_AFTER")}, Notes: []Note{{Kind: Footnote, ID: "rich", Blocks: []Block{mergeParagraph("NOTE_INTRO"), {Kind: Picture, Image: &copy}, {Kind: Table, Rows: rows}}}}}
	var out bytes.Buffer
	if e := Write(context.Background(), &out, d, PDF, WriteOptions{}); e != nil {
		t.Fatal(e)
	}
	got, e := ReadPDF(context.Background(), bytes.NewReader(out.Bytes()), ReadOptions{PDFSkipInference: true, PDFSkipStructure: true})
	if e != nil {
		t.Fatal(e)
	}
	if got.Metadata.PageCount != 4 {
		t.Fatalf("rich note continuation pages: got %d want 4", got.Metadata.PageCount)
	}
	counts := map[int]int{}
	images := 0
	for _, b := range got.Blocks {
		if b.Kind == Picture {
			images++
			if b.Source.Page != 1 {
				t.Fatal("note image duplicated/moved")
			}
		}
		for _, run := range b.Runs {
			if strings.HasPrefix(run.Text, "NOTE_ROW_") {
				counts[run.Source.Page]++
			}
			if strings.Contains(run.Text, "BODY_AFTER") && run.Source.Page != 4 {
				t.Fatal("body overlapped continuations")
			}
		}
	}
	if images != 1 || counts[1] != 3 || counts[2] != 5 || counts[3] != 4 {
		t.Fatalf("fragment row/image ownership: %v images=%d", counts, images)
	}
	for i := 0; i < 12; i++ {
		if strings.Count(got.Text(), fmt.Sprintf("NOTE_ROW_%02d", i)) != 1 {
			t.Fatal("note row omitted/repeated")
		}
	}
}

func TestImageHeightOnlyAndCellFitting(t *testing.T) {
	base := *richCellDocument(t).Blocks[1].Rows[0].Cells[0].Blocks[1].Image
	base.Width, base.Height = 0, 30
	d := &Document{Blocks: []Block{{Kind: Picture, Image: &base}}}
	for _, format := range []Format{DOCX, PDF} {
		t.Run(string(format), func(t *testing.T) {
			var out bytes.Buffer
			if e := Write(context.Background(), &out, d, format, WriteOptions{}); e != nil {
				t.Fatal(e)
			}
			got, e := Read(context.Background(), &out, format, ReadOptions{PDFSkipInference: true, PDFSkipStructure: true})
			if e != nil {
				t.Fatal(e)
			}
			if len(got.Blocks) != 1 || got.Blocks[0].Kind != Picture {
				t.Fatal("image lost")
			}
			im := got.Blocks[0].Image
			if math.Abs(im.Width-60) > .01 || math.Abs(im.Height-30) > .01 {
				t.Fatalf("height-only aspect: %+v", im)
			}
		})
	}
	// DOCX and PDF use their declared cell padding independently, while both
	// fit oversized requested images proportionally inside the available width.

	// A reference's admitted body fragment can include later text on that same
	// page. Subsequent pages draining its note must contain only note content.

	d = richCellDocument(t)
	var out bytes.Buffer
	if e := Write(context.Background(), &out, d, DOCX, WriteOptions{}); e != nil {
		t.Fatal(e)
	}
	got, e := ReadDOCX(context.Background(), &out)
	if e != nil {
		t.Fatal(e)
	}
	im := got.Blocks[1].Rows[0].Cells[0].Blocks[1].Image
	if math.Abs(im.Width-232) > .01 || math.Abs(im.Height-116) > .01 {
		t.Fatalf("DOCX image overflow/ratio: %+v", im)
	}
}

func TestPDFRichCellsFailuresAreAtomic(t *testing.T) {
	for _, kind := range []string{"no_image_space", "bad_image", "unrepresentable_note"} {
		t.Run(kind, func(t *testing.T) {
			d := richCellDocument(t)
			switch kind {
			case "no_image_space":
				d.Layout.Height = 84
			case "bad_image":
				d.Blocks[1].Rows[0].Cells[0].Blocks[1].Image.Data = []byte("not an image")
			case "unrepresentable_note":
				d.Notes[1].Blocks[1].Image.Width = 1
				d.Notes[1].Blocks[1].Image.Height = 1e308
			}
			var out bytes.Buffer
			if e := Write(context.Background(), &out, d, PDF, WriteOptions{}); e == nil || out.Len() != 0 {
				t.Fatalf("failed rich layout exposed partial output: %v, bytes=%d", e, out.Len())
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var out bytes.Buffer
	if e := Write(ctx, &out, richCellDocument(t), PDF, WriteOptions{}); !errors.Is(e, context.Canceled) || out.Len() != 0 {
		t.Fatal("canceled rich output was committed")
	}
}

func TestImagePreferredSizesAvoidOverflow(t *testing.T) {
	base := *richCellDocument(t).Notes[1].Blocks[1].Image
	for _, sizes := range [][2]float64{{1e308, 0}, {0, 1e308}} {
		for _, format := range []Format{PDF, DOCX} {
			im := base
			im.Width, im.Height = sizes[0], sizes[1]
			d := &Document{Layout: Layout{Width: 200, Height: 200, Margin: 20}, Blocks: []Block{{Kind: Picture, Image: &im}}}
			var out bytes.Buffer
			if e := Write(context.Background(), &out, d, format, WriteOptions{}); e != nil {
				t.Fatal(e)
			}
			got, e := Read(context.Background(), &out, format, ReadOptions{PDFSkipInference: true, PDFSkipStructure: true})
			if e != nil {
				t.Fatal(e)
			}
			if math.Abs(got.Blocks[0].Image.Width-160) > .01 || math.Abs(got.Blocks[0].Image.Height-80) > .01 {
				t.Fatal("large preferred image did not fit proportionally")
			}
		}
	}
	base.Width, base.Height = 1, 1e308
	for _, format := range []Format{PDF, DOCX} {
		var out bytes.Buffer
		if e := Write(context.Background(), &out, &Document{Blocks: []Block{{Kind: Picture, Image: &base}}}, format, WriteOptions{}); e == nil || out.Len() != 0 {
			t.Fatal("unrepresentable image committed invalid output")
		}
	}
}

func FuzzPDFRichLayout(f *testing.F) {
	seed, _ := json.Marshal(richCellDocument(f))
	f.Add(uint16(240), uint16(220), byte(0), uint16(30))
	f.Add(uint16(0), uint16(440), byte(5), uint16(150))
	f.Fuzz(func(t *testing.T, width, height uint16, rows byte, imageHeight uint16) {
		var d Document
		if e := json.Unmarshal(seed, &d); e != nil {
			t.Fatal(e)
		}
		d.Layout = Layout{Width: 180 + float64(width%500), Height: 160 + float64(height%600), Margin: 20}
		image := d.Blocks[1].Rows[0].Cells[0].Blocks[1].Image
		image.Height = 1 + float64(imageHeight%200)
		image.Width = image.Height * 2
		inner := &d.Blocks[1].Rows[0].Cells[0].Blocks[2]
		var tokens []string
		for i := 0; i < int(rows%10); i++ {
			a, b := fmt.Sprintf("EXTRA_%02d_A", i), fmt.Sprintf("EXTRA_%02d_B", i)
			tokens = append(tokens, a, b)
			inner.Rows = append(inner.Rows, Row{Cells: []Cell{{Blocks: []Block{mergeParagraph(a)}}, {Blocks: []Block{mergeParagraph(b)}}}})
		}
		before, _ := json.Marshal(&d)
		var out bytes.Buffer
		e := Write(context.Background(), &out, &d, PDF, WriteOptions{})
		after, _ := json.Marshal(&d)
		if !bytes.Equal(before, after) {
			t.Fatal("rich layout mutated caller values")
		}
		if e != nil {
			if !errors.Is(e, ErrUnsupported) || out.Len() != 0 {
				t.Fatalf("invalid failure outcome: %v bytes=%d", e, out.Len())
			}
			return
		}
		got, e := ReadPDF(context.Background(), bytes.NewReader(out.Bytes()), ReadOptions{PDFSkipInference: true, PDFSkipStructure: true})
		if e != nil {
			t.Fatal(e)
		}
		text := strings.Join(strings.Fields(got.Text()), "")
		for _, token := range append(tokens, "CELL_START", "CELL_END", "INNER_A", "INNER_B", "INNER_C", "TOP_RIGHT", "BOTTOM_RIGHT", "AFTER_TABLE", "NOTE_CELL", "NOTE_NESTED") {
			if strings.Count(text, token) != 1 {
				t.Fatalf("painted rich content lost/repeated: %s", token)
			}
		}
		pictures := 0
		for _, b := range got.Blocks {
			if b.Kind == Picture {
				pictures++
				x0, y0, x1, y1 := quadBounds(*b.Source.Quad)
				if x0 < 19.95 || y0 < 19.95 || x1 > d.Layout.Width-19.95 || y1 > d.Layout.Height-19.95 {
					t.Fatal("rich image escaped page content area")
				}
			}
		}
		if pictures != 3 {
			t.Fatalf("image appearances lost/repeated: %d", pictures)
		}
	})
}
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
	got, err := ReadPDF(context.Background(), bytes.NewReader(out.Bytes()), ReadOptions{PDFSkipInference: true, PDFSkipStructure: true})
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
	got, err := ReadPDF(context.Background(), &out, ReadOptions{PDFSkipInference: true, PDFSkipStructure: true})
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
