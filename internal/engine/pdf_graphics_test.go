package engine

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"reflect"
	"strings"
	"testing"
)

func paintContextRenderer(t testing.TB) *pdfRenderer {
	t.Helper()
	data, err := os.ReadFile("testdata/assets/NotoSans-Regular.ttf")
	if err != nil {
		t.Fatal(err)
	}
	r := textTestRenderer()
	if err := r.addFonts([]FontFace{{Family: "Noto Sans", Data: data}}); err != nil {
		t.Fatal(err)
	}
	return r
}
func TestPDFPaintChangesKeepKerningAndLigature(t *testing.T) {
	r := paintContextRenderer(t)
	run := func(text, color string) Run {
		return Run{Text: text, FontFamily: "Noto Sans", FontSize: 20, Color: color}
	}
	lines, err := r.lines([]Run{run("A", "#FF0000"), run("V", "#0000FF")}, 100, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 1 || len(lines[0]) != 2 || math.Abs(pdfAdvance(lines[0])-23.98) > 1e-8 || math.Abs(lines[0][0].width-11.98) > 1e-8 {
		t.Fatalf("color interrupted kerning: %+v", lines)
	}
	first, middle, last := run("f", "#FF0000"), run("f", "#0000FF"), run("i", "#008800")
	first.Underline = true
	middle.Strike = true
	middle.Link = "https://example.invalid/paint"
	lines, err = r.lines([]Run{first, middle, last}, 100, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 1 || len(lines[0]) != 3 {
		t.Fatalf("paint parts %+v", lines)
	}
	// Original Noto GDEF gives ffi component carets 315/631 units; the
	// independent native HarfBuzz ffi advance is 946 units (UPEM 1000).
	for i, want := range []float64{6.3, 6.32, 6.3} {
		s := lines[0][i]
		if math.Abs(s.width-want) > 1e-8 || len(s.glyphs.positions) != 1 || s.glyphs.positions[0].glyph != 1654 || s.glyphs.positions[0].logical != s.text {
			t.Fatalf("component %d %+v glyphs %+v", i, s, s.glyphs.positions)
		}
		if s.glyphs.clipLeft != (i > 0) || s.glyphs.clipRight != (i < 2) {
			t.Fatalf("clip component %d %+v", i, s.glyphs)
		}
	}
	// Wrapping re-shapes the selected line; it must not slice an existing
	// clipped ligature or forget the color/link on the next line.
	lines, err = r.lines([]Run{first, middle, last}, 8, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 3 {
		t.Fatalf("emergency lines %+v", lines)
	}
	for i, want := range []uint16{73, 73, 76} {
		s := lines[i][0]
		if s.glyphs.positions[0].glyph != want || s.glyphs.clipLeft || s.glyphs.clipRight || s.width > 8+1e-7 {
			t.Fatalf("stale glyph clip on line %d %+v", i, s)
		}
	}
}
func TestPDFPaintLigatureTextStylesAndLinksRecover(t *testing.T) {
	data, err := os.ReadFile("testdata/assets/NotoSans-Regular.ttf")
	if err != nil {
		t.Fatal(err)
	}
	runs := []Run{{Text: "of", Color: "#FF0000", Underline: true}, {Text: "fi", Color: "#0000FF", Link: "https://example.invalid/paint"}, {Text: "ce", Color: "#008800", Strike: true}}
	for i := range runs {
		runs[i].FontFamily = "Noto Sans"
		runs[i].FontSize = 20
	}
	d := &Document{Blocks: []Block{{Kind: Paragraph, Runs: runs}}}
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, PDF, WriteOptions{PDFFonts: []FontFace{{Family: "Noto Sans", Data: data}}}); err != nil {
		t.Fatal(err)
	}
	back, err := ReadPDF(context.Background(), &out, ReadOptions{PDFSkipInference: true, PDFSkipStructure: true})
	if err != nil {
		t.Fatal(err)
	}
	if back.Text() != "office" {
		t.Fatalf("clipped duplicate glyph changed logical text %q", back.Text())
	}
	var actual []Run
	for _, r := range recoveryRuns(back) {
		if r.Text != "" {
			actual = append(actual, r)
		}
	}
	if len(actual) != 3 || actual[0].Text != "of" || actual[0].Color != "#FF0000" || actual[1].Text != "fi" || actual[1].Color != "#0000FF" || actual[1].Link != "https://example.invalid/paint" || actual[2].Text != "ce" || actual[2].Color != "#008800" {
		t.Fatalf("paint styles %+v", actual)
	}
	if actual[0].Source.Precision != "replacement_span_bounds" || actual[1].Source.Precision != "replacement_span_bounds" || actual[0].Source.RenderMode != 3 || actual[1].Source.RenderMode != 3 {
		t.Fatal("logical carrier bounds/render mode are not explicit")
	}
	if !hasDiagnostic(back, "pdf_invisible_text") {
		t.Fatal("semantic carrier render mode not disclosed")
	}
	if hasDiagnostic(back, "pdf_clipping") {
		t.Fatal("supported rectangular text clipping reported as unapplied")
	}
	assertRecoverySF(t, back)
}
func TestPaintContextKeepsFontSizeAndLanguageBoundaries(t *testing.T) {
	r := paintContextRenderer(t)
	for _, runs := range [][]Run{
		{{Text: "f", FontFamily: "Noto Sans", FontSize: 20}, {Text: "fi", FontFamily: "Noto Sans", FontSize: 21}},
		{{Text: "f", FontFamily: "Noto Sans", Language: "en"}, {Text: "fi", FontFamily: "Noto Sans", Language: "sr"}},
	} {
		lines, err := r.lines(runs, 200, 20)
		if err != nil {
			t.Fatal(err)
		}
		if len(lines) != 1 || len(lines[0]) != 2 {
			t.Fatalf("incompatible contexts merged %+v", lines)
		}
	}
	// A color change inside a grapheme still needs component-paint semantics;
	// it must not silently change the color of the entire base+mark cluster.
	_, err := r.lines([]Run{{Text: "B", FontFamily: "Noto Sans", Color: "#FF0000"}, {Text: "\u0315", FontFamily: "Noto Sans", Color: "#0000FF"}}, 200, 20)
	if err == nil || !strings.Contains(err.Error(), "grapheme") {
		t.Fatalf("mixed grapheme accepted: %v", err)
	}
}

func TestPaintCaretFallbackAndAliasResolution(t *testing.T) {
	r := paintContextRenderer(t)
	face, err := r.resolveFont(Run{FontFamily: "Noto Sans"})
	if err != nil {
		t.Fatal(err)
	}
	c, err := r.glyphClusterLanguage(face, "A", Run{})
	if err != nil {
		t.Fatal(err)
	}
	// A has no GDEF ligature carets. A controlled cluster verifies the declared
	// equal-grapheme fallback independently of an accidental font-specific list.

	// Independently hand-placed grid: top-left colspan 2; lower-left rowspan 2;
	// middle-right empty. No numeric-text heuristic can establish these cells.

	c.positions[0].logical = "ABC"
	c.positions[0].advance = 1
	c.width = 1
	clusters, err := paintClusters("ABC", c)
	if err != nil {
		t.Fatal(err)
	}
	if len(clusters) != 1 || math.Abs(clusters[0].edges[1]-1.0/3) > 1e-8 || math.Abs(clusters[0].edges[2]-2.0/3) > 1e-8 {
		t.Fatalf("caret fallback %+v", clusters)
	}
	r.fontMap = map[string]string{fontFamilyKey("Alias"): "Noto Sans"}
	lines, err := r.lines([]Run{{Text: "A", FontFamily: "Alias", Color: "#FF0000"}, {Text: "V", FontFamily: "Noto Sans", Color: "#0000FF"}}, 100, 20)
	if err != nil || len(lines) != 1 || math.Abs(pdfAdvance(lines[0])-23.98) > 1e-8 {
		t.Fatalf("resolved aliases interrupted shaping: %+v %v", lines, err)
	}
}
func FuzzPDFPaintContext(f *testing.F) {
	data, err := os.ReadFile("testdata/assets/NotoSans-Regular.ttf")
	if err != nil {
		f.Fatal(err)
	}
	f.Add([]byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}, byte(40))
	f.Add([]byte{1, 1, 2, 1, 1, 2, 0, 3}, byte(70))
	f.Fuzz(func(t *testing.T, input []byte, width byte) {
		if len(input) > 24 {
			return
		}
		alphabet :=

			// Half-stroke overhangs into an adjacent merged cell are join
			// nubs, not partial dividers across that otherwise absent edge.

			[]rune("officeAVTo")
		colors := []string{"#FF0000", "#0000FF", "#008800"}
		var text strings.Builder
		var runs []Run
		for i, b := range input {
			ch := string(alphabet[int(b)%len(alphabet)])
			text.WriteString(ch)
			run := Run{Text: ch, FontFamily: "Noto Sans", FontSize: 14, Color: colors[(int(b)+i)%3], Underline: b&1 != 0, Strike: b&2 != 0}
			if b&4 != 0 {
				run.Link = "https://example.invalid/context"
			}
			runs = append(runs, run)
		}
		r := paintContextRenderer(t)
		available := 20 + float64(width)
		styled, err := r.lines(runs, available, 14)
		if err != nil {
			if !errors.Is(err, ErrUnsupported) {
				t.Fatal(err)
			}
			return
		}
		plain, err := r.lines([]Run{{Text: text.String(), FontFamily: "Noto Sans", FontSize: 14}}, available, 14)
		if err != nil {
			t.Fatal(err)
		}
		if len(styled) != len(plain) {
			t.Fatal("paint changed line breaks")

			// A thick crossing horizontal stroke can overhang an absent vertical
			// divider by more than the thin vertical stroke's own half-width.

		}
		for i, line := range styled {
			if lineText(line) != lineText(plain[i]) || math.Abs(pdfAdvance(line)-pdfAdvance(plain[i])) > 1e-7 {
				t.Fatal("paint changed glyph layout")
			}
			var logical strings.Builder
			for _, span := range line {
				if span.glyphs == nil {
					continue
				}
				for _, g := range span.glyphs.positions {
					logical.WriteString(g.logical)
				}
			}
			if logical.String() != lineText(line) {
				t.Fatal("ToUnicode paint fragments duplicate/omit logical text")
			}
		}
		d := &Document{Layout: Layout{Width: available + 40, Height: 320, Margin: 20}, Blocks: []Block{{Kind: Paragraph, Runs: runs}}}
		var out bytes.Buffer
		if err := Write(context.Background(), &out, d, PDF, WriteOptions{PDFFonts: []FontFace{{Family: "Noto Sans", Data: data}}}); err != nil {
			t.Fatal(err)
		}
		back, err := ReadPDF(context.Background(), &out, ReadOptions{PDFSkipInference: true, PDFSkipStructure: true})
		if err != nil {
			t.Fatal(err)
		}
		if strings.ReplaceAll(back.Text(), "\n", "") != text.String() {
			t.Fatalf("paint roundtrip %q -> %q", text.String(), back.Text())
		}
		assertRecoverySF(t, back)
	})
}

func FuzzPDFPaintFont(f *testing.F) {
	original, err := os.ReadFile("testdata/assets/NotoSans-Regular.ttf")
	if err != nil {
		f.Fatal(err)
	}
	f.Add(uint32(10), byte(1))
	f.Add(uint32(2048), byte(255))
	f.Fuzz(func(t *testing.T, offset uint32, value byte) {
		data := append([]byte(nil), original...)
		data[int(offset)%len(data)] ^= value
		d := &Document{Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "f", FontFamily: "Font", Color: "#FF0000"}, {Text: "f", FontFamily: "Font", Color: "#0000FF"}, {Text: "i", FontFamily: "Font", Color: "#008800"}}}}}
		var out bytes.Buffer
		err := Write(context.Background(), &out, d, PDF, WriteOptions{PDFFonts: []FontFace{{Family: "Font", Data: data}}})
		if err != nil {
			if out.Len() != 0 {
				t.Fatal("partial output for malformed paint font")
			}
			return
		}
		back, err := ReadPDF(context.Background(), &out, ReadOptions{PDFSkipInference: true, PDFSkipStructure: true})
		if err != nil {
			t.Fatal(err)
		}
		if back.Text() != "ffi" {
			t.Fatalf("font mutation changed logical text %q", back.Text())
		}
	})
}

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
			if e := Write(context.Background(), &pdf, d, PDF, WriteOptions{AllowLossy: true, PDFFontMap: map[string]string{"Helvetica": "Go"}}); e != nil {
				t.Fatal(e)
			}
			decoded, e := ReadPDF(context.Background(), bytes.NewReader(pdf.Bytes()), ReadOptions{PDFSkipStructure: true})
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
	if e := Write(context.Background(), &output, d, PDF, WriteOptions{AllowLossy: true, PDFFontMap: map[string]string{"Helvetica": "Go"}}); e != nil {
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
