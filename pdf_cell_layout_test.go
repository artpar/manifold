package manydoc

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
