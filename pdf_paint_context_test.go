package manydoc

import (
	"bytes"
	"context"
	"errors"
	"math"
	"os"
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
		alphabet := []rune("officeAVTo")
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
