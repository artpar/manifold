package manifold

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/benoitkugler/textlayout/language"
	"io"
	"math"
	"os"
	"strings"
	"testing"
)

func languageDocument() *Document {
	p := Block{Kind: Paragraph, Direction: "rtl", Runs: []Run{{Text: "Visible", ComplexScript: true, Language: "en-GB", EastAsiaLanguage: "ja-JP", ComplexLanguage: "ar-SA", Direction: "ltr", Bold: true, Link: "https://example.invalid/language"}}}
	return &Document{Layout: Layout{Width: 595.3, Height: 841.9, Margin: 54}, Blocks: []Block{p, {Kind: Table, Rows: []Row{{Cells: []Cell{{Blocks: []Block{p}}}}}}, {Kind: ListBlock, List: "bullet", Items: []ListItem{{Blocks: []Block{p}}}}}, Headers: []Block{p}, Footers: []Block{p}}
}
func TestLanguageDirectionStructuredAndDOCX(t *testing.T) {
	d := languageDocument()
	before := canonicalSF(t, d)
	checkParagraphRoundtrip(t, d)
	var docx bytes.Buffer
	if err := Write(context.Background(), &docx, d, DOCX, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	parts := map[string][]byte{}
	zr, err := zip.NewReader(bytes.NewReader(docx.Bytes()), int64(docx.Len()))
	if err != nil {
		t.Fatal(err)
	}
	for _, part := range zr.File {
		reader, err := part.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(reader)
		reader.Close()
		if err != nil {
			t.Fatal(err)
		}
		parts[part.Name] = data
	}
	for _, name := range []string{"word/document.xml", "word/header1.xml", "word/footer1.xml"} {
		value := string(parts[name])
		for _, property := range []string{`<w:bidi w:val="true"/>`, `<w:cs/>`, `<w:rtl w:val="false"/>`, `<w:lang w:val="en-GB" w:eastAsia="ja-JP" w:bidi="ar-SA"/>`} {
			if !strings.Contains(value, property) {
				t.Fatalf("missing %s in %s", property, name)
			}
		}
	}
	if !bytes.Equal(before, canonicalSF(t, d)) {
		t.Fatal("writer mutated source")
	}
}
func TestDOCXLanguageDirectionInheritance(t *testing.T) {
	styles := `<w:docDefaults><w:rPrDefault><w:rPr><w:lang w:val="en-US" w:eastAsia="ja-JP" w:bidi="ar-SA"/><w:rtl/></w:rPr></w:rPrDefault><w:pPrDefault><w:pPr><w:bidi/></w:pPr></w:pPrDefault></w:docDefaults><w:style w:type="paragraph" w:default="1" w:styleId="Normal"><w:pPr><w:bidi w:val="0"/></w:pPr><w:rPr><w:lang w:val="fr-CA"/></w:rPr></w:style><w:style w:type="paragraph" w:styleId="Derived"><w:basedOn w:val="Normal"/><w:pPr><w:bidi/></w:pPr><w:rPr><w:lang w:eastAsia="zh-TW"/></w:rPr></w:style><w:style w:type="character" w:styleId="Character"><w:rPr><w:lang w:bidi="he-IL"/><w:rtl w:val="false"/></w:rPr></w:style>`
	body := `<w:p><w:r><w:t>DEFAULT</w:t></w:r></w:p><w:p><w:pPr><w:pStyle w:val="Derived"/><w:bidi w:val="off"/></w:pPr><w:r><w:rPr><w:rStyle w:val="Character"/><w:lang w:val="sr-RS"/></w:rPr><w:t>OVERRIDE</w:t></w:r></w:p><w:p><w:pPr><w:pStyle w:val="Derived"/></w:pPr><w:r><w:t>INHERITED</w:t></w:r></w:p>`
	d, err := ReadDOCX(context.Background(), bytes.NewReader(numberedDOCX(t, body, "", styles)))
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Blocks) != 3 {
		t.Fatalf("paragraphs %+v", d.Blocks)
	}
	for i, want := range []struct{ paragraph, run, lang, ea, cs string }{{"ltr", "rtl", "fr-CA", "ja-JP", "ar-SA"}, {"ltr", "ltr", "sr-RS", "zh-TW", "he-IL"}, {"rtl", "rtl", "fr-CA", "zh-TW", "ar-SA"}} {
		b := d.Blocks[i]
		r := b.Runs[0]
		if b.Direction != want.paragraph || r.Direction != want.run || r.Language != want.lang || r.EastAsiaLanguage != want.ea || r.ComplexLanguage != want.cs {
			t.Fatalf("%d: %+v %+v", i, b, r)
		}
	}
	if hasDiagnostic(d, "run_language") || hasDiagnostic(d, "run_direction") {
		t.Fatal("retained semantics diagnosed as lost")
	}
	if hasDiagnostic(d, "font_selectors") {
		t.Fatal("retained RTL flag diagnosed as lost font selection")
	}
	d.Layout = Layout{Width: 595.3, Height: 841.9, Margin: 54}
	checkParagraphRoundtrip(t, d)
}
func TestLanguageDirectionInvalidAtomic(t *testing.T) {
	for _, value := range []string{"en_US", " en-US", "en-US ", "not a tag", "12", "en\nUS", strings.Repeat("a", 256)} {
		d := &Document{Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "VISIBLE", Language: value}}}}}
		for _, f := range []Format{SF, DOCX, PDF} {
			var out bytes.Buffer
			if err := Write(context.Background(), &out, d, f, WriteOptions{}); err == nil || out.Len() != 0 {
				t.Fatalf("accepted %q/%s", value, f)
			}
		}
	}
	for _, d := range []*Document{{Blocks: []Block{{Kind: Paragraph, Direction: "auto"}}}, {Blocks: []Block{{Kind: Table, Direction: "rtl"}}}, {Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "V", Direction: "auto"}}}}}} {
		if d.Validate() == nil {
			t.Fatal("invalid direction accepted")
		}
	}
	base := `{"type":"doc","attrs":{"schema":"manydoc-prosemirror/1"},"content":[{"type":"paragraph","attrs":{"dir":null}}]}`
	if _, err := Read(context.Background(), strings.NewReader(base), SF, ReadOptions{}); err == nil {
		t.Fatal("null direction accepted")
	}
	for _, prop := range []string{`<w:lang w:val=""/>`, `<w:lang w:val="en_US"/>`, `<w:lang/><w:lang/>`, `<w:rtl w:val="bad"/>`} {
		input := numberedDOCX(t, `<w:p><w:r><w:rPr>`+prop+`</w:rPr><w:t>VISIBLE</w:t></w:r></w:p>`, "", "")
		if _, err := ReadDOCX(context.Background(), bytes.NewReader(input)); err == nil {
			t.Fatalf("accepted %s", prop)
		}
	}
	for _, prop := range []string{`<w:bidi/><w:bidi/>`, `<w:bidi w:val="bad"/>`} {
		input := numberedDOCX(t, `<w:p><w:pPr>`+prop+`</w:pPr><w:r><w:t>VISIBLE</w:t></w:r></w:p>`, "", "")
		if _, err := ReadDOCX(context.Background(), bytes.NewReader(input)); err == nil {
			t.Fatalf("accepted %s", prop)
		}
	}
}
func TestPDFDirectionAllStories(t *testing.T) {
	rtl := Block{Kind: Paragraph, Direction: "rtl"}
	cases := []*Document{{Blocks: []Block{rtl}}, {Headers: []Block{rtl}}, {Footers: []Block{rtl}}, {Blocks: []Block{{Kind: Table, Rows: []Row{{Cells: []Cell{{Blocks: []Block{rtl}}}}}}}}, {Blocks: []Block{{Kind: ListBlock, List: "bullet", Items: []ListItem{{Blocks: []Block{rtl}}}}}}, {Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "V", Direction: "rtl"}}}}}}
	for i, d := range cases {
		var out bytes.Buffer
		if err := Write(context.Background(), &out, d, PDF, WriteOptions{}); err != nil || out.Len() == 0 {
			t.Fatalf("story %d: %v bytes=%d", i, err, out.Len())
		}
	}
	d := &Document{Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Note: &NoteReference{Kind: Footnote, ID: "n"}}}}}, Notes: []Note{{Kind: Footnote, ID: "n", Blocks: []Block{rtl}}}}
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, PDF, WriteOptions{}); err != nil || out.Len() == 0 {
		t.Fatalf("note %v", err)
	}
}
func TestPDFLanguageLocalizedGlyph(t *testing.T) {
	data, err := os.ReadFile("testdata/assets/NotoSans-Regular.ttf")
	if err != nil {
		t.Fatal(err)
	}
	r := textTestRenderer()
	if err := r.addFonts([]FontFace{{Family: "Noto Sans", Data: data}}); err != nil {
		t.Fatal(err)
	}
	face, err := r.resolveFont(Run{FontFamily: "Noto Sans"})
	if err != nil {
		t.Fatal(err)
	}
	// Native HarfBuzz 0.57.0 with original Noto Sans: Serbian/Macedonian locl
	// substitutes U+0431 to GID 2090; default/Russian keep GID 457.
	for locale, want := range map[string]uint16{"": 457, "ru-RU": 457, "sr-RS": 2090, "mk-MK": 2090} {
		c, err := r.glyphClusterLanguage(face, "б", Run{Language: locale})
		if err != nil {
			t.Fatal(err)
		}
		if len(c.positions) != 1 || c.positions[0].glyph != want {
			t.Fatalf("%s: %+v", locale, c.positions)
		}
	}
	c, err := r.glyphClusterLanguage(face, "б", Run{Language: "ru-RU", ComplexLanguage: "sr-RS", ComplexScript: true})
	if err != nil || len(c.positions) != 1 || c.positions[0].glyph != 2090 {
		t.Fatalf("complex locale selection %+v %v", c, err)
	}
	d := &Document{Blocks: []Block{{Kind: Paragraph, Direction: "ltr", Runs: []Run{{Text: "б\nб\u00adб", Language: "sr-RS", FontFamily: "Noto Sans"}}}}}
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, PDF, WriteOptions{PDFFonts: []FontFace{{Family: "Noto Sans", Data: data}}}); err != nil {
		t.Fatal(err)
	}
	got, err := ReadPDF(context.Background(), &out, ReadOptions{PDFSkipInference: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got.Text(), "б") {
		t.Fatal("lost logical text")
	}
}
func FuzzLanguageDirectionRoundtrip(f *testing.F) {
	f.Add("en-US", "ja-JP", "ar-SA", false)
	f.Add("sr-RS", "zh-TW", "he-IL", true)
	f.Fuzz(func(t *testing.T, lang, ea, cs string, rtl bool) {
		if len(lang)+len(ea)+len(cs) > 1024 {
			return
		}
		for _, value := range []string{lang, ea, cs} {
			if validateLanguage(value) != nil {
				return
			}
		}
		dir := "ltr"
		if rtl {
			dir = "rtl"
		}
		d := &Document{Layout: Layout{Width: 595.3, Height: 841.9, Margin: 54}, Blocks: []Block{{Kind: Paragraph, Direction: dir, Runs: []Run{{Text: "VISIBLE", Language: lang, EastAsiaLanguage: ea, ComplexLanguage: cs, Direction: dir, ComplexScript: rtl}}}}}
		checkParagraphRoundtrip(t, d)
		b, err := json.Marshal(d)
		if err != nil {
			t.Fatal(err)
		}
		if !json.Valid(b) {
			t.Fatal("invalid JSON")
		}
	})
}

func TestPDFLanguageLineShapingAndBoundaries(t *testing.T) {
	data, err := os.ReadFile("testdata/assets/NotoSans-Regular.ttf")
	if err != nil {
		t.Fatal(err)
	}
	r := textTestRenderer()
	if err := r.addFonts([]FontFace{{Family: "Noto Sans", Data: data}}); err != nil {
		t.Fatal(err)
	}
	run := Run{Text: "office", FontFamily: "Noto Sans", Language: "en-US", FontSize: 20}
	lines, err := r.lines([]Run{run}, 200, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 1 || len(lines[0]) != 1 || len(lines[0][0].glyphs.positions) != 4 {
		t.Fatalf("ligature did not shape across graphemes: %+v", lines)
	}
	for i, want := range []uint16{82, 1654, 70, 72} {
		if lines[0][0].glyphs.positions[i].glyph != want {
			t.Fatalf("glyph %d", i)
		}
	}
	for i, want := range []string{"o", "ffi", "c", "e"} {
		if lines[0][0].glyphs.positions[i].logical != want {
			t.Fatalf("cluster logical %d: %+v", i, lines[0][0].glyphs.positions)
		}
	}
	if math.Abs(pdfAdvance(lines[0])-51.9) > 1e-7 {
		t.Fatalf("advance %f", pdfAdvance(lines[0]))
	}
	a, b := run, run
	a.Text = "of"
	b.Text = "fice"
	split, err := r.lines([]Run{a, b}, 200, 20)
	if err != nil || len(split) != 1 || len(split[0][0].glyphs.positions) != 4 {
		t.Fatalf("equal runs broke shaping: %+v %v", split, err)
	}
	run.Text = "ffi"
	lines, err = r.lines([]Run{run}, 8, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 3 {
		t.Fatalf("emergency ligature wrapping: %+v", lines)
	}
	for i, want := range []uint16{73, 73, 76} {
		if pdfAdvance(lines[i]) > 8+1e-7 || len(lines[i]) != 1 || len(lines[i][0].glyphs.positions) != 1 || lines[i][0].glyphs.positions[0].glyph != want {
			t.Fatalf("stale ligature or overflow on line %d: %+v", i, lines[i])
		}
	}
	run.Text = "AV"
	if _, err = r.lines([]Run{run}, 12.2, 20); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("un-kerned A overflow should reject, got %v", err)
	}
	lines, err = r.lines([]Run{run}, 12.8, 20)
	if err != nil || len(lines) != 2 {
		t.Fatalf("boundary kerning re-shape %v %+v", err, lines)
	}
	for _, line := range lines {
		if pdfAdvance(line) > 12.8+1e-7 {
			t.Fatal("reshaped line overflow")
		}
	}
	run.Text = "Latin ΩЖ"
	lines, err = r.lines([]Run{run}, 200, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 1 || len(lines[0]) != 3 {
		t.Fatalf("script segmentation %+v", lines)
	}
	for i, want := range []language.Script{language.Latin, language.Greek, language.Cyrillic} {
		if lines[0][i].glyphs.script != want {
			t.Fatalf("script %d: %+v", i, lines[0][i].glyphs)
		}
	}
}

func FuzzPDFLanguageLines(f *testing.F) {
	data, err := os.ReadFile("testdata/assets/NotoSans-Regular.ttf")
	if err != nil {
		f.Fatal(err)
	}
	f.Add([]byte{0, 1, 2, 3, 4}, byte(20), false)
	f.Add([]byte{5, 6, 7, 8, 9, 10, 11}, byte(8), true)
	f.Fuzz(func(t *testing.T, input []byte, width byte, serbian bool) {
		if len(input) > 120 {
			return
		}
		alphabet := []rune("officeAVTo бгдптΩЖ\u00a0\u00ad\n")
		var s strings.Builder
		for _, b := range input {
			s.WriteRune(alphabet[int(b)%len(alphabet)])
		}
		run := Run{Text: s.String(), Language: "en-US", FontFamily: "Noto Sans", FontSize: 20}
		if serbian {
			run.Language = "sr-RS"
		}
		r := textTestRenderer()
		if err := r.addFonts([]FontFace{{Family: "Noto Sans", Data: data}}); err != nil {
			t.Fatal(err)
		}
		lines, err := r.lines([]Run{run}, 8+float64(width), 20)
		if err != nil {
			if !errors.Is(err, ErrUnsupported) {
				t.Fatal(err)
			}
			return
		}
		var logical strings.Builder
		for _, line := range lines {
			if pdfAdvance(line) > 8+float64(width)+1e-7 {
				t.Fatal("line overflow")
			}
			logical.WriteString(lineText(line))
			for _, span := range line {
				if span.glyphs == nil {
					continue
				}
				var fallback strings.Builder
				for _, g := range span.glyphs.positions {
					fallback.WriteString(g.logical)
				}
				if fallback.String() != span.text {
					t.Fatalf("ToUnicode cluster text %q != %q", fallback.String(), span.text)
				}
			}
		}
		if logical.String() != s.String() {
			t.Fatalf("logical text %q != %q", logical.String(), s.String())
		}
	})
}
