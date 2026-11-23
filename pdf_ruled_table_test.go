package manydoc

import (
	"bytes"
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// Independently hand-placed grid: top-left colspan 2; lower-left rowspan 2;
// middle-right empty. No numeric-text heuristic can establish these cells.
var ruledEdges = [][4]float64{
	{40, 480, 340, 480}, {40, 600, 340, 600}, {40, 560, 340, 560}, {140, 520, 340, 520},
	{40, 480, 40, 600}, {340, 480, 340, 600}, {240, 480, 240, 600}, {140, 480, 140, 560},
}

func ruledPath(filled bool) string {
	var s strings.Builder
	for _, p := range ruledEdges {
		if !filled {
			fmt.Fprintf(&s, "%g %g m %g %g l\n", p[0], p[1], p[2], p[3])
			continue
		}
		if p[1] == p[3] {
			fmt.Fprintf(&s, "%g %g %g 1 re f\n", p[0], p[1]-.5, p[2]-p[0])
		} else {
			fmt.Fprintf(&s, "%g %g 1 %g re f\n", p[0]-.5, p[1], p[3]-p[1])
		}
	}
	if !filled {
		s.WriteString("S\n")
	}
	return s.String()
}
func ruledContent() string {
	return inferenceText("F", 10, 250, 500, "Linked") + inferenceText("B", 10, 50, 580, "MERGED TOP") +
		inferenceText("F", 10, 50, 542, "Two lines") + inferenceText("F", 10, 50, 528, "in this cell") +
		inferenceText("F", 10, 250, 580, "Top right") + inferenceText("F", 10, 150, 540, "Middle") + inferenceText("F", 10, 150, 500, "Bottom")
}
func assertRuledTable(t *testing.T, d *Document) Block {
	t.Helper()
	if len(d.Blocks) != 1 || d.Blocks[0].Kind != Table {
		t.Fatalf("expected one ruling table, got %+v", d.Blocks)
	}
	b := d.Blocks[0]
	if len(b.Rows) != 3 || len(b.Rows[0].Cells) != 2 || len(b.Rows[1].Cells) != 3 || len(b.Rows[2].Cells) != 2 {
		t.Fatalf("grid membership: %+v", b.Rows)
	}
	if b.Rows[0].Cells[0].ColSpan != 2 || b.Rows[1].Cells[0].RowSpan != 2 || layoutText(b.Rows[0].Cells[0].Blocks[0]) != "MERGED TOP" || !b.Rows[0].Cells[0].Blocks[0].Runs[0].Bold {
		t.Fatal("merged origins/text/formatting lost")
	}
	if layoutText(b.Rows[1].Cells[0].Blocks[0]) != "Two lines in this cell" || len(b.Rows[1].Cells[2].Blocks) != 1 || len(b.Rows[1].Cells[2].Blocks[0].Runs) != 0 {
		t.Fatal("multiline/empty cell lost")
	}
	grid, e := tableGrid(b.Rows)
	if e != nil || grid.Columns != 3 {
		t.Fatalf("grid validation: %+v %v", grid, e)
	}
	if b.Source.Structure != "inferred" || b.Source.Evidence != "painted_grid_adjacency" || !hasDiagnostic(d, "pdf_table_rulings_inferred") {
		t.Fatal("ruling inference promoted to explicit")
	}
	for _, row := range b.Rows {
		for _, cell := range row.Cells {
			if cell.Source == nil || cell.Source.Quad == nil || cell.Source.Structure != "inferred" {
				t.Fatal("cell bounds/provenance missing")
			}
		}
	}
	assertRecoverySF(t, d)
	return b
}
func TestPDFRuledMergedEmptyMultilineAndLinks(t *testing.T) {
	for _, filled := range []bool{false, true} {
		t.Run(fmt.Sprint(filled), func(t *testing.T) {
			objects := inferenceObjects("0 0 600 800 re W n\n" + ruledPath(filled) + ruledContent())
			objects[2] = strings.Replace(objects[2], "/Contents 6 0 R", "/Annots [8 0 R] /Contents 6 0 R", 1)
			objects = append(objects, "<< /Type /Annot /Subtype /Link /Rect [249 497 330 512] /A << /S /URI /URI (https://example.invalid/cell) >> >>")
			d := readInferred(t, objects)
			b := assertRuledTable(t, d)
			if b.Rows[2].Cells[1].Blocks[0].Runs[0].Link != "https://example.invalid/cell" {
				t.Fatal("cell URI lost")
			}
			closePoint(t, b.Source.Quad[0], 40, 200)
			closePoint(t, b.Source.Quad[2], 340, 320)
			empty := b.Rows[1].Cells[2]
			closePoint(t, empty.Source.Quad[0], 240, 240)
			closePoint(t, empty.Source.Quad[2], 340, 280)
			var pdf, word bytes.Buffer
			if e := Write(context.Background(), &pdf, d, PDF, WriteOptions{AllowLossy: true}); e != nil {
				t.Fatal(e)
			}
			decoded, e := ReadPDF(context.Background(), bytes.NewReader(pdf.Bytes()), ReadOptions{})
			if e != nil {
				t.Fatal(e)
			}
			assertRuledTable(t, decoded)
			if e := Write(context.Background(), &word, d, DOCX, WriteOptions{AllowLossy: true}); e != nil {
				t.Fatal(e)
			}
			back, e := Read(context.Background(), &word, DOCX, ReadOptions{})
			if e != nil {
				t.Fatal(e)
			}
			if back.Blocks[0].Rows[0].Cells[0].ColSpan != 2 || back.Blocks[0].Rows[1].Cells[0].RowSpan != 2 || layoutText(back.Blocks[0].Rows[1].Cells[0].Blocks[0]) != "Two lines in this cell" {
				t.Fatal("DOCX spans/cell content changed")
			}
		})
	}
}
func TestPDFRuledOpacityAndUnknownEffects(t *testing.T) {
	for _, tc := range []struct {
		name, gs, prefix string
		want             bool
	}{
		{"opaque", "<< /CA 1 /ca 1 /BM /Normal /SMask /None >>", "/G gs\n", true},
		{"transparent", "<< /CA 0 /ca 0 >>", "/G gs\n", false},
		{"mask", "<< /SMask << /S /Alpha >> >>", "/G gs\n", false},
		{"restored", "<< /CA 0 /ca 0 >>", "q /G gs Q\n", true},
		{"neutralized", "<< /SMask << /S /Alpha >> >>", "/G gs /Clear gs\n", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			objects := inferenceObjects(tc.prefix + ruledPath(false) + ruledContent())
			objects[2] = strings.Replace(objects[2], "/Resources <<", "/Resources << /ExtGState << /G 8 0 R /Clear << /SMask /None >> >>", 1)
			objects = append(objects, tc.gs)
			d := readInferred(t, objects)
			found := false
			for _, b := range d.Blocks {
				found = found || b.Kind == Table
			}
			if found != tc.want {
				t.Fatalf("opacity/effect ruling assignment: %+v", d.Blocks)
			}
		})
	}
}
func TestPDFRuledPathStateAndFormTransforms(t *testing.T) {
	path := strings.Replace(ruledPath(false), "40 480 m 340 480 l", "q 1 0 0 1 40 480 cm 0 0 m Q 340 480 l", 1)
	assertRuledTable(t, readInferred(t, inferenceObjects(path+ruledContent())))
	objects := inferenceObjects("q /Grid Do Q")
	objects[2] = strings.Replace(objects[2], "/Im 7 0 R", "/Im 7 0 R /Grid 8 0 R", 1)
	objects = append(objects, recoveryStream("/Type /XObject /Subtype /Form /BBox [0 0 400 700] /Matrix [.5 0 0 .5 10 20] /Resources << /Font << /F 4 0 R /B 5 0 R >> >>", ruledPath(false)+ruledContent()))
	b := assertRuledTable(t, readInferred(t, objects))
	closePoint(t, b.Source.Quad[0], 30, 480)
	closePoint(t, b.Source.Quad[2], 180, 540)
}

func TestPDFRuledStrokeIntersectionEdges(t *testing.T) {
	for _, width := range []float64{.25, .5, 1, 2} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			var path strings.Builder
			fmt.Fprintf(&path, "%g w\n", width)
			for _, edge := range ruledEdges {
				x0, y0, x1, y1 := edge[0], edge[1], edge[2], edge[3]
				if y0 == y1 {
					x0 += width / 2
					x1 -= width / 2
				} else {
					y0 += width / 2
					y1 -= width / 2
				}
				fmt.Fprintf(&path, "%g %g m %g %g l S\n", x0, y0, x1, y1)
			}
			assertRuledTable(t, readInferred(t, inferenceObjects(path.String()+ruledContent())))
			// Half-stroke overhangs into an adjacent merged cell are join
			// nubs, not partial dividers across that otherwise absent edge.
			overhang := strings.Replace(ruledPath(false), "140 480 m 140 560 l", fmt.Sprintf("140 480 m 140 %g l", 560+width/2), 1)
			overhang = strings.Replace(overhang, "140 520 m 340 520 l", fmt.Sprintf("%g 520 m 340 520 l", 140-width/2), 1)
			assertRuledTable(t, readInferred(t, inferenceObjects(fmt.Sprintf("%g w\n", width)+overhang+ruledContent())))
		})
	}
}

func TestPDFRuledMixedStrokeWidths(t *testing.T) {
	path := "2 w 40.25 480 m 339.75 480 l 40.25 600 m 339.75 600 l S\n"
	path += ".5 w 40.25 560 m 339.75 560 l 140.25 520 m 339.75 520 l S\n"
	path += "40 481 m 40 599 l 340 481 m 340 599 l 240 481 m 240 599 l 140 481 m 140 559.75 l S\n"
	assertRuledTable(t, readInferred(t, inferenceObjects(path+ruledContent())))
	// A thick crossing horizontal stroke can overhang an absent vertical
	// divider by more than the thin vertical stroke's own half-width.
	path = "2 w 40 560 m 340 560 l S\n.5 w\n" + strings.Replace(ruledPath(false), "140 480 m 140 560 l", "140 480 m 140 561 l", 1)
	path = strings.Replace(path, "40 560 m 340 560 l\n", "", 1)
	assertRuledTable(t, readInferred(t, inferenceObjects(path+ruledContent())))
}
func TestPDFRuledNestedAndImageCellPreservation(t *testing.T) {
	outer := "40 400 360 200 re 200 400 m 200 600 l S\n"
	inner := "60 480 80 80 re 100 480 m 100 560 l 60 520 m 140 520 l S\n"
	d := readInferred(t, inferenceObjects(outer+inner+inferenceText("F", 10, 65, 540, "A")))
	if len(d.Blocks) != 1 || d.Blocks[0].Kind != Table || len(d.Blocks[0].Rows[0].Cells) != 2 || d.Blocks[0].Rows[0].Cells[0].Blocks[0].Kind != Table {
		t.Fatalf("nested grids duplicated/flattened: %+v", d.Blocks)
	}
	image := "q 20 0 0 20 250 532 cm /Im Do Q\n"
	d = readInferred(t, inferenceObjects(ruledPath(false)+ruledContent()+image))
	b := d.Blocks[0]
	cell := b.Rows[1].Cells[2]
	if len(d.Blocks) != 1 || b.Kind != Table || len(cell.Blocks) != 1 || cell.Blocks[0].Kind != Picture || len(cell.Blocks[0].Image.Data) == 0 {
		t.Fatal("cell image lost or duplicated")
	}
	var output bytes.Buffer
	if e := Write(context.Background(), &output, d, PDF, WriteOptions{AllowLossy: true}); e != nil {
		t.Fatal(e)
	}
	back, e := ReadPDF(context.Background(), bytes.NewReader(output.Bytes()), ReadOptions{})
	if e != nil {
		t.Fatal(e)
	}
	if len(back.Blocks) != 1 || back.Blocks[0].Kind != Table || len(back.Blocks[0].Rows) != 3 {
		t.Fatal("rendered image table geometry lost")
	}
	imageCell := back.Blocks[0].Rows[1].Cells[2]
	if len(imageCell.Blocks) != 1 || imageCell.Blocks[0].Kind != Picture || len(imageCell.Blocks[0].Image.Data) == 0 {
		t.Fatal("rendered cell image lost or displaced")
	}
	output.Reset()
	if e := Write(context.Background(), &output, d, DOCX, WriteOptions{AllowLossy: true}); e != nil {
		t.Fatal(e)
	}
}
func TestPDFRuledAmbiguityDoesNotConsumeContent(t *testing.T) {
	for name, path := range map[string]string{
		"unpainted":     strings.TrimSuffix(ruledPath(false), "S\n") + "n\n",
		"clipped":       "0 0 10 10 re W n\n" + ruledPath(false),
		"partial":       strings.Replace(ruledPath(false), "140 480 m 140 560 l", "140 480 m 140 545 l", 1),
		"crossing_text": ruledPath(false),
	} {
		t.Run(name, func(t *testing.T) {
			content := ruledContent()
			if name == "crossing_text" {
				content += inferenceText("F", 10, 230, 540, "CROSSES")
			}
			d := readInferred(t, inferenceObjects(path+content))
			for _, b := range d.Blocks {
				if b.Kind == Table {
					t.Fatalf("unjustified grid: %+v", b)
				}
			}
			if !strings.Contains(d.Text(), "MERGED TOP") || !strings.Contains(d.Text(), "Linked") {
				t.Fatal("ambiguous grid discarded text")
			}
			if name == "partial" || name == "crossing_text" {
				if !hasDiagnostic(d, "pdf_table_rulings_ambiguous") {
					t.Fatal("ambiguity not diagnosed")
				}
			}
		})
	}
}
func TestPDFRuledOriginalRunsAndEmptyGrid(t *testing.T) {
	objects := inferenceObjects(ruledPath(false) + ruledContent())
	data := recoveryPDF(objects...)
	raw, e := ReadPDF(context.Background(), bytes.NewReader(data), ReadOptions{PDFSkipInference: true})
	if e != nil {
		t.Fatal(e)
	}
	d := readInferred(t, objects)
	want := map[int]Run{}
	for _, b := range raw.Blocks {
		for _, run := range b.Runs {
			if run.Source.Precision != "synthetic_separator" {
				want[run.Source.Order] = run
			}
		}
	}
	got := map[int]Run{}
	for _, row := range d.Blocks[0].Rows {
		for _, cell := range row.Cells {
			for _, b := range cell.Blocks {
				for _, run := range b.Runs {
					if run.Source.Precision != "synthetic_separator" {
						got[run.Source.Order] = run
					}
				}
			}
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatal("grid assignment changed or omitted original text/marks/positions")
	}
	d = readInferred(t, inferenceObjects(ruledPath(false)))
	if len(d.Blocks) != 1 || d.Blocks[0].Kind != Table || len(d.Blocks[0].Rows) != 3 || d.Blocks[0].Rows[1].Cells[0].RowSpan != 2 {
		t.Fatal("empty ruled structure disappeared")
	}
}

func TestPDFRuledImplicitClipAndDegenerateFill(t *testing.T) {
	clip := "0 0 m 600 0 l 600 800 l 0 800 l W n\n"
	assertRuledTable(t, readInferred(t, inferenceObjects(clip+ruledPath(false)+ruledContent())))
	var path strings.Builder
	for _, e := range ruledEdges {
		x0, y0, x1, y1 := e[0], e[1], e[2], e[3]
		if y0 == y1 {
			fmt.Fprintf(&path, "%g %g m %g %g l %g %g l %g %g l h f\n", x0, y0-.5, x0, y0+.5, x0, y0-.5, x1, y0-.5)
		} else {
			fmt.Fprintf(&path, "%g %g m %g %g l %g %g l %g %g l h f\n", x0-.5, y0, x0+.5, y0, x0-.5, y0, x0-.5, y1)
		}
	}
	d := readInferred(t, inferenceObjects(path.String()+ruledContent()))
	for _, b := range d.Blocks {
		if b.Kind == Table {
			t.Fatal("zero-area retraced fill invented table boundaries")
		}
	}
}

func FuzzPDFRuledGrid(f *testing.F) {
	f.Add(byte(0), false)
	f.Add(byte(3), true)
	f.Add(byte(7), false)
	f.Fuzz(func(t *testing.T, mask byte, filled bool) {
		// Author a known rectangular partition, then draw each cell perimeter.
		// The expected origins/spans come from that partition, not from the
		// interpreter's missing-edge/connected-component implementation.
		type cell struct {
			r, c, rs, cs int
			text         string
		}
		cells := []cell{}
		for row := 0; row < 3; row++ {
			for col := 0; col < 3; col++ {
				if mask&1 != 0 && row == 0 && col == 1 {
					continue
				}
				if mask&2 != 0 && row == 2 && col == 0 {
					continue
				}
				c := cell{row, col, 1, 1, fmt.Sprintf("C%d%d", row, col)}
				if mask&1 != 0 && row == 0 && col == 0 {
					c.cs = 2
				}
				if mask&2 != 0 && row == 1 && col == 0 {
					c.rs = 2
				}
				if mask&4 != 0 && row == 1 && col == 2 {
					c.text = ""
				}
				cells = append(cells, c)
			}
		}
		var content strings.Builder
		dx, dy := float64((mask>>3)&3)*7, float64(mask>>5)*3
		fmt.Fprintf(&content, "q 1 0 0 1 %g %g cm %g w\n", dx, dy, .25+float64((mask>>3)&7)*.25)
		for _, c := range cells {
			x, y := 40+float64(c.c)*100, 600-float64(c.r+c.rs)*40
			w, h := float64(c.cs)*100, float64(c.rs)*40
			if !filled {
				fmt.Fprintf(&content, "%g %g %g %g re S\n", x, y, w, h)
			} else {
				fmt.Fprintf(&content, "%g %g %g 1 re f %g %g %g 1 re f %g %g 1 %g re f %g %g 1 %g re f\n", x, y-.5, w, x, y+h-.5, w, x-.5, y, h, x+w-.5, y, h)
			}
			if c.text != "" {
				content.WriteString(inferenceText("F", 10, int(x+10), int(y+h-20), c.text))
			}
		}
		content.WriteString("Q\n")
		d := readInferred(t, inferenceObjects(content.String()))
		if len(d.Blocks) != 1 || d.Blocks[0].Kind != Table || len(d.Blocks[0].Rows) != 3 {
			t.Fatal("known grid lost")
		}
		b := d.Blocks[0]
		closePoint(t, b.Source.Quad[0], 40+dx, 200-dy)
		closePoint(t, b.Source.Quad[2], 340+dx, 320-dy)
		index := 0
		for row, rr := range b.Rows {
			for _, actual := range rr.Cells {
				if index >= len(cells) {
					t.Fatal("invented cell")
				}
				want := cells[index]
				if row != want.r || actual.ColSpan != want.cs || max(1, actual.RowSpan) != want.rs || len(actual.Blocks) != 1 || layoutText(actual.Blocks[0]) != want.text {
					t.Fatalf("partition mismatch at %d: %+v want %+v", index, actual, want)
				}
				index++
			}
		}
		if index != len(cells) {
			t.Fatal("omitted cells")
		}
	})
}
