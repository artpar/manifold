package manydoc

import (
	"bytes"
	"context"
	"errors"
	"math"
	"os"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/phpdave11/gofpdf"
)

func textTestRenderer() *pdfRenderer {
	p := gofpdf.NewCustom(&gofpdf.InitType{UnitStr: "pt", Size: gofpdf.SizeType{Wd: 300, Ht: 400}})
	for _, font := range pdfFonts {
		p.AddUTF8FontFromBytes("Go", font.style, append([]byte(nil), font.data...))
	}
	return &pdfRenderer{ctx: context.Background(), pdf: p, notes: &noteCatalog{labels: map[noteKey]string{{Footnote, "n"}: "1"}}}
}
func lineText(line pdfLine) string {
	var b strings.Builder
	for _, span := range line {
		b.WriteString(span.text)
	}
	return b.String()
}
func lineAdvance(line pdfLine) float64 {
	w := 0.0
	for _, span := range line {
		w += span.width
	}
	return w
}

func TestPDFTextWordsCrossFormattingRuns(t *testing.T) {
	r := textTestRenderer()
	lines, e := r.lines([]Run{{Text: "prefix "}, {Text: "inter", Bold: true}, {Text: "operability"}}, 80, 11)
	if e != nil {
		t.Fatal(e)
	}
	if len(lines) != 2 || strings.TrimSpace(lineText(lines[0])) != "prefix" || lineText(lines[1]) != "interoperability" || !lines[1][0].run.Bold {
		t.Fatalf("format boundary split a word: %+v", lines)
	}
	for _, line := range lines {
		if lineAdvance(line) > 80+1e-7 {
			t.Fatal("wrapped text exceeds measured width")
		}
	}
}
func TestPDFTextGlueAcrossRuns(t *testing.T) {
	for _, glue := range []string{"\u00a0", "\u2007", "\u202f", "\u2060", "\ufeff", "\u2011"} {
		t.Run(glue, func(t *testing.T) {
			r := textTestRenderer()
			r.pdf.SetFont("Go", "", 11)
			word := "A" + glue + "B"
			width := r.textAdvance(word, 11) + 1
			runs := []Run{{Text: "X A"}, {Text: glue}, {Text: "B"}}
			lines, e := r.lines(runs, width, 11)
			if e != nil {
				t.Fatal(e)
			}
			if len(lines) != 2 || strings.TrimSpace(lineText(lines[0])) != "X" || lineText(lines[1]) != word {
				t.Fatalf("glue split across runs: %+v", lines)
			}
			if _, e = r.lines([]Run{{Text: word}}, width-2, 11); !errors.Is(e, ErrUnsupported) {
				t.Fatalf("overwide glue did not reject: %v", e)
			}
		})
	}
}
func TestPDFTextGraphemesAndHardBreaks(t *testing.T) {
	r := textTestRenderer()
	original := "aaaaae\u0301bbbbbe\u0301ccc"
	lines, e := r.lines([]Run{{Text: original}}, 25, 11)
	if e != nil {
		t.Fatal(e)
	}
	var joined strings.Builder
	for _, line := range lines {
		text := lineText(line)
		first, _ := utf8.DecodeRuneInString(text)
		if unicode.IsMark(first) {
			t.Fatal("emergency wrap split a grapheme")
		}
		joined.WriteString(text)
		if lineAdvance(line) > 25+1e-7 {
			t.Fatal("grapheme overflow")
		}
	}
	if joined.String() != original {
		t.Fatal("emergency wrap changed logical Unicode")
	}
	if _, e = r.lines([]Run{{Text: "e"}, {Text: "\u0301", Bold: true}}, 100, 11); !errors.Is(e, ErrUnsupported) {
		t.Fatal("unsupported mixed-style combining cluster accepted")
	}
	lines, e = r.lines([]Run{{Text: "A\r"}, {Text: "\nB\u2028C"}}, 100, 11)
	if e != nil {
		t.Fatal(e)
	}
	if len(lines) != 3 || lineText(lines[0]) != "A\r\n" || lineText(lines[1]) != "B\u2028" || lineText(lines[2]) != "C" {
		t.Fatalf("hard break handling %+v", lines)
	}
}
func TestPDFDiscretionaryHyphenAndNoteAnchor(t *testing.T) {
	r := textTestRenderer()
	r.pdf.SetFont("Go", "", 11)
	width := r.pdf.GetStringWidth("hy-") + .01
	lines, e := r.lines([]Run{{Text: "hy\u00adphen"}}, width, 11)
	if e != nil {
		t.Fatal(e)
	}
	if lineText(lines[0]) != "hy\u00ad" || len(lines[0]) < 2 || math.Abs(lines[0][len(lines[0])-1].width-r.pdf.GetStringWidth("-")) > 1e-7 {
		t.Fatal("selected soft hyphen has no measured visible hyphen")
	}
	lines, e = r.lines([]Run{{Text: "hy\u00adphen"}}, 100, 11)
	if e != nil {
		t.Fatal(e)
	}
	if len(lines) != 1 {
		t.Fatal("unnecessary discretionary break")
	}
	for _, span := range lines[0] {
		if span.text == "\u00ad" && span.width != 0 {
			t.Fatal("unused soft hyphen painted")
		}
	}
	lines, e = r.lines([]Run{{Text: "prefix anchor"}, noteRun(Footnote, "n")}, 60, 11)
	if e != nil {
		t.Fatal(e)
	}
	if len(lines) != 2 || lineText(lines[1]) != "anchor1" || len(lineNoteKeys(lines[1])) != 1 || len(lineNoteKeys(lines[0])) != 0 {
		t.Fatal("note marker separated from anchor")
	}
}
func TestPDFTypographyLogicalTextAndGeometry(t *testing.T) {
	text := "A\u00a0B C\u2007D E\u202fF G\u2060H I\u200bJ café cafe\u0301 K\u2011L M\u2003N O\u200aP"
	doc := &Document{Layout: Layout{Width: 1000, Height: 160, Margin: 20}, Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: text, Bold: true, Italic: true, Underline: true, Strike: true, Link: "https://example.invalid/spacing"}}}}}
	var output bytes.Buffer
	if e := Write(context.Background(), &output, doc, PDF, WriteOptions{}); e != nil {
		t.Fatal(e)
	}
	if !bytes.HasPrefix(output.Bytes(), []byte("%PDF-1.4")) {
		t.Fatal("ActualText output did not declare PDF 1.4")
	}
	got, e := ReadPDF(context.Background(), bytes.NewReader(output.Bytes()), ReadOptions{})
	if e != nil {
		t.Fatal(e)
	}
	if got.Text() != text {
		t.Fatalf("logical characters changed\nwant %q\ngot  %q", text, got.Text())
	}
	for _, run := range recoveryRuns(got) {
		if !run.Bold || !run.Italic {
			t.Fatal("homogeneous ActualText formatting lost")
		}
	}

	foundEm := false
	for _, run := range recoveryRuns(got) {
		if strings.Contains(run.Text, "\u2003") && run.Source != nil {
			for _, region := range run.Source.Regions {
				a, _, b, _ := quadBounds(region.Quad)
				if math.Abs(b-a-11) < .02 {
					foundEm = true
				}
			}
		}
	}
	if !foundEm {
		t.Fatal("em space has no measured 11pt source region")
	}
	// Font/model limitations remain explicit and never yield partial PDF bytes.
	for _, bad := range []string{"q\u0301", "中文", "مرحبا"} {
		doc.Blocks[0].Runs[0].Text = bad
		output.Reset()
		if e = Write(context.Background(), &output, doc, PDF, WriteOptions{}); !errors.Is(e, ErrUnsupported) || output.Len() != 0 {
			t.Fatalf("unsupported shaping/glyph emitted: %v", e)
		}
	}
}
func TestPDFLineBoundaryWhitespaceAndCancellation(t *testing.T) {
	r := textTestRenderer()
	lines, e := r.lines([]Run{{Text: "A\u2003B"}}, 12, 11)
	if e != nil {
		t.Fatal(e)
	}
	if len(lines) != 2 || lineText(lines[0]) != "A\u2003" || lineText(lines[1]) != "B" || lineAdvance(lines[0]) > 12 {
		t.Fatalf("line-edge space discarded/overflowed %+v", lines)
	}
	var cancel context.CancelFunc
	r.ctx, cancel = context.WithCancel(context.Background())
	cancel()
	if _, e = r.lines([]Run{{Text: "text"}}, 100, 11); !errors.Is(e, context.Canceled) {
		t.Fatal("layout ignored cancellation")
	}
}
func TestPDFPublishedExportNowRenders(t *testing.T) {
	input, e := os.ReadFile("testdata/real/copyright-office-circular-1-2021.pdf")
	if e != nil {
		t.Fatal(e)
	}
	imported := readRecovery(t, input)
	sf, e := ToStructured(imported)
	if e != nil {
		t.Fatal(e)
	}
	shared, e := FromStructured(sf)
	if e != nil {
		t.Fatal(e)
	}
	var out bytes.Buffer
	if e = Write(context.Background(), &out, shared, PDF, WriteOptions{AllowLossy: true}); e != nil {
		t.Fatal(e)
	}
	rendered, e := ReadPDF(context.Background(), bytes.NewReader(out.Bytes()), ReadOptions{})
	if e != nil {
		t.Fatal(e)
	}
	if rendered.Metadata.Title != shared.Metadata.Title || rendered.Metadata.PageCount < 2 {
		t.Fatal("published output metadata/pagination missing")
	}
	for _, space := range []string{"\u2003", "\u200a", "\u2008"} {
		if strings.Count(rendered.Text(), space) != strings.Count(shared.Text(), space) {
			t.Fatalf("publication spacing lost: %U", []rune(space)[0])
		}
	}
	for _, run := range recoveryRuns(rendered) {
		if run.Source == nil || run.Source.Quad == nil {
			continue
		}
		_, _, right, bottom := quadBounds(*run.Source.Quad)
		if right > rendered.Pages[run.Source.Page-1].Width-54+.1 || bottom > rendered.Pages[run.Source.Page-1].Height-54+.1 {
			t.Fatalf("published text outside content bounds: %+v", run.Source)
		}
	}
}

func TestPDFRejectsOversizedPageWithoutPartialOutput(t *testing.T) {
	d := &Document{Layout: Layout{Width: 15000, Height: 600, Margin: 20}, Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "text"}}}}}
	var out bytes.Buffer
	if e := Write(context.Background(), &out, d, PDF, WriteOptions{}); !errors.Is(e, ErrUnsupported) || out.Len() != 0 {
		t.Fatalf("oversized default-unit PDF emitted: %v", e)
	}
}
func FuzzPDFTextLayout(f *testing.F) {
	for _, text := range []string{"prefix interoperability", "A\u2003B", "A\u00a0B", "cafe\u0301", "a\r\nb\u2028c", "hy\u00adphenation", "ab\u2060cd", "Latin Ελληνικά Привет"} {
		f.Add(text, uint16(80), uint16(5))
	}
	f.Fuzz(func(t *testing.T, text string, widthValue, splitValue uint16) {
		if len(text) > 8192 || validateString(text) != nil {
			return
		}
		runes := []rune(text)
		split := int(splitValue) % (len(runes) + 1)
		runs := []Run{{Text: string(runes[:split])}, {Text: string(runes[split:]), Bold: true}}
		width := 12 + float64(widthValue%400)
		r := textTestRenderer()
		lines, e := r.lines(runs, width, 11)
		if e != nil {
			if !errors.Is(e, ErrUnsupported) {
				t.Fatal(e)
			}
			return
		}
		var reconstructed strings.Builder
		for _, line := range lines {
			if lineAdvance(line) > width+1e-6 {
				t.Fatalf("layout exceeded content area %g > %g", lineAdvance(line), width)
			}
			reconstructed.WriteString(lineText(line))
		}
		if reconstructed.String() != text {
			t.Fatalf("layout changed logical text %q -> %q", text, reconstructed.String())
		}
	})
}
