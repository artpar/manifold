package engine

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"github.com/go-text/typesetting/language"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
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

type corpusCase struct {
	Name    string   `json:"name"`
	Kind    string   `json:"kind"`
	Expect  string   `json:"expect"`
	Markers []string `json:"markers"`
	Pages   int      `json:"pages"`
	SHA256  string   `json:"sha256"`
}

func TestCorpusHashes(t *testing.T) {
	for _, c := range corpus(t) {
		actual := fmt.Sprintf("%x", sha256.Sum256(fixture(t, c.Name)))
		if actual != c.SHA256 {
			t.Errorf("fixture %s hash mismatch: %s", c.Name, actual)
		}
	}
}

func TestInvalidModelAndIO(t *testing.T) {
	for _, d := range []*Document{nil, {Blocks: []Block{{Kind: "unknown"}}}, {Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "bad\x00text"}}}}}, {Layout: Layout{Width: 10, Height: 10, Margin: 20}}} {
		if e := Write(context.Background(), &bytes.Buffer{}, d, DOCX, WriteOptions{}); e == nil {
			t.Fatal("invalid document accepted")
		}
	}
	if _, e := Read(context.Background(), nil, DOCX, ReadOptions{}); e == nil {
		t.Fatal("nil reader accepted")
	}
	if e := Write(context.Background(), nil, &Document{}, DOCX, WriteOptions{}); e == nil {
		t.Fatal("nil writer accepted")
	}
	if _, e := Read(context.Background(), strings.NewReader(`{"blocks":[],"typo":true}`), JSON, ReadOptions{}); e == nil {
		t.Fatal("unknown JSON field accepted")
	}
	d, e := Read(context.Background(), strings.NewReader("first\fsecond"), Text, ReadOptions{})
	if e != nil {
		t.Fatal(e)
	}
	if len(d.Blocks) != 3 || d.Blocks[1].Kind != PageBreak {
		t.Fatal("text page boundary lost")
	}
}

func corpus(t testing.TB) []corpusCase {
	t.Helper()
	b, e := os.ReadFile("testdata/corpus/manifest.json")
	if e != nil {
		t.Fatal(e)
	}
	var c []corpusCase
	if e = json.Unmarshal(b, &c); e != nil {
		t.Fatal(e)
	}
	return c
}
func fixture(t testing.TB, name string) []byte {
	t.Helper()
	b, e := os.ReadFile(filepath.Join("testdata/corpus", name))
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func readFixture(t testing.TB, name string) *Document {
	t.Helper()
	f, e := FormatFromName(name)
	if e != nil {
		t.Fatal(e)
	}
	d, e := Read(context.Background(), bytes.NewReader(fixture(t, name)), f, ReadOptions{})
	if e != nil {
		t.Fatal(e)
	}
	return d
}

func TestDOCXCorpus(t *testing.T) {
	for _, c := range corpus(t) {
		if c.Kind != "docx" {
			continue
		}
		t.Run(c.Name, func(t *testing.T) {
			d, e := ReadDOCX(context.Background(), bytes.NewReader(fixture(t, c.Name)))
			if c.Expect == "reject" {
				if e == nil {
					t.Fatal("invalid DOCX accepted")
				}
				return
			}
			if e != nil {
				t.Fatal(e)
			}
			if d == nil {
				t.Fatal("nil document")
			}
		})
	}
}
func TestDOCXVisibleContent(t *testing.T) {
	cases := map[string][]string{
		"docx_minimal.docx": {"MINIMAL_MARKER"}, "docx_hyperlink.docx": {"Before Linked text after"},
		"docx_tracked.docx": {"INSERTED_MARKER"}, "docx_content_control.docx": {"CONTROL_MARKER"},
		"docx_textbox.docx": {"TEXTBOX_MARKER"}, "docx_field.docx": {"FIELD_RESULT_MARKER"},
		"docx_bookmark.docx": {"BOOKMARK_MARKER"}, "docx_unicode.docx": {"Ελληνικά", "Привет", "中文", "हिन्दी", "😀", "A&B <tag>"},
		"docx_nested_table.docx": {"Merged heading", "Outer cell", "Nested cell"}, "docx_table.docx": {"Before table", "CELL-0-0", "CELL-2-1", "After table"},
	}
	for name, markers := range cases {
		t.Run(name, func(t *testing.T) {
			d := readFixture(t, name)
			for _, m := range markers {
				if !strings.Contains(d.Text(), m) {
					t.Fatalf("missing %q in %q", m, d.Text())
				}
			}
			if name == "docx_tracked.docx" && strings.Contains(d.Text(), "DELETED_MARKER") {
				t.Fatal("deleted text in final visible view")
			}
		})
	}
}
func TestPDFCorpus(t *testing.T) {
	for _, c := range corpus(t) {
		if c.Kind != "pdf" {
			continue
		}
		t.Run(c.Name, func(t *testing.T) {
			opts := ReadOptions{}
			if c.Expect == "password" {
				opts.Password = "secret"
			}
			d, e := ReadPDF(context.Background(), bytes.NewReader(fixture(t, c.Name)), opts)
			if c.Expect == "reject" {
				if e == nil {
					t.Fatal("invalid PDF accepted")
				}
				return
			}
			if e != nil {
				t.Fatal(e)
			}
			text := d.Text()
			for _, m := range c.Markers {
				if !strings.Contains(text, m) {
					t.Fatalf("missing %q in %.400q", m, text)
				}
			}
			if c.Pages > 0 && d.Metadata.PageCount != c.Pages {
				t.Fatalf("pages=%d, want %d", d.Metadata.PageCount, c.Pages)
			}
			if strings.ContainsRune(text, 0) {
				t.Fatal("NUL in extracted text")
			}
			if c.Name == "pdf_unicode.pdf" && strings.Contains(text, "ПриветП") {
				t.Fatal("spurious font-mapped newline character")
			}
		})
	}
}
func TestPDFWidgetAppearance(t *testing.T) {
	d := readFixture(t, "pdf_form_field.pdf")
	if !strings.Contains(d.Text(), "FIELD_VALUE_MARKER") {
		t.Fatalf("missing field appearance: %q", d.Text())
	}
}
func TestPDFPasswords(t *testing.T) {
	for _, name := range []string{"pdf_rc4_40.pdf", "pdf_rc4_128.pdf", "pdf_aes-128.pdf", "pdf_aes-256.pdf"} {
		for _, password := range []string{"", "wrong"} {
			t.Run(name+password, func(t *testing.T) {
				if _, e := ReadPDF(context.Background(), bytes.NewReader(fixture(t, name)), ReadOptions{Password: password}); e == nil {
					t.Fatal("wrong password accepted")
				}
			})
		}
	}
}
func TestGenerationRoundTrip(t *testing.T) {
	d := &Document{Metadata: Metadata{Title: "A&B report", Author: "Fixture"}, Headers: []Block{{Kind: Paragraph, Runs: []Run{{Text: "Header", Link: "https://example.invalid/"}}}}, Footers: []Block{{Kind: Paragraph, Runs: []Run{{Text: "Footer"}}}}, Blocks: []Block{
		{Kind: Heading, Level: 1, Runs: []Run{{Text: "Report title"}}},
		{Kind: Paragraph, Runs: []Run{{Text: "Bold ", Bold: true}, {Text: "italic ", Italic: true}, {Text: "link", Link: "https://example.invalid/"}}},
		{Kind: Paragraph, List: "bullet", Runs: []Run{{Text: "café Ελληνικά Привет"}}},
		{Kind: Table, Rows: []Row{{Cells: []Cell{{Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "Product"}}}}}, {Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "Quantity"}}}}}}}, {Cells: []Cell{{Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "Daptin"}}}}}, {Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "42"}}}}}}}}},
		{Kind: PageBreak}, {Kind: Paragraph, Runs: []Run{{Text: "Second page"}}},
	}}
	for _, format := range []Format{DOCX, PDF, JSON, Text} {
		t.Run(string(format), func(t *testing.T) {
			var out bytes.Buffer
			if e := Write(context.Background(), &out, d, format, WriteOptions{}); e != nil {
				t.Fatal(e)
			}
			decoded, e := Read(context.Background(), &out, format, ReadOptions{})
			if e != nil {
				t.Fatal(e)
			}
			for _, m := range []string{"Report title", "Bold", "italic", "link", "Ελληνικά", "Привет", "Product", "Daptin", "42", "Second page"} {
				if !strings.Contains(decoded.Text(), m) {
					t.Fatalf("missing %q: %q", m, decoded.Text())
				}
			}
			if format == DOCX {
				if decoded.Metadata.Title != d.Metadata.Title {
					t.Fatal("metadata lost")
				}
				if len(decoded.Headers) != 1 || decoded.Headers[0].Runs[0].Link != "https://example.invalid/" {
					t.Fatal("header link lost")
				}
			}
		})
	}
}
func TestPDFOutputRejectsMissingGlyph(t *testing.T) {
	d := &Document{Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "中文"}}}}}
	var out bytes.Buffer
	if e := Write(context.Background(), &out, d, PDF, WriteOptions{}); !errors.Is(e, ErrUnsupported) {
		t.Fatalf("want unsupported glyph, got %v", e)
	}
	if out.Len() != 0 {
		t.Fatal("partial PDF emitted")
	}
}
func TestDiagnosticsRequireAcknowledgement(t *testing.T) {
	d := readFixture(t, "docx_content_control.docx")
	if len(d.Diagnostics) == 0 {
		t.Fatal("lost control semantics not reported")
	}
	var out bytes.Buffer
	if e := Write(context.Background(), &out, d, DOCX, WriteOptions{}); !errors.Is(e, ErrLossy) {
		t.Fatalf("got %v", e)
	}
	if e := Write(context.Background(), &out, d, DOCX, WriteOptions{AllowLossy: true}); e != nil {
		t.Fatal(e)
	}
}
func TestCancelledOperations(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := Read(ctx, strings.NewReader("x"), Text, ReadOptions{}); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if e := Write(ctx, &bytes.Buffer{}, &Document{}, DOCX, WriteOptions{}); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
}
func TestConcurrentOperations(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 3; j++ {
				d := readFixture(t, "docx_basic.docx")
				for _, f := range []Format{DOCX, PDF} {
					var out bytes.Buffer
					if e := Write(context.Background(), &out, d, f, WriteOptions{AllowLossy: true, PDFFontMap: map[string]string{"Calibri": "Go", "Cambria": "Go"}}); e != nil {
						t.Error(e)
					}
				}
				readFixture(t, "pdf_unicode.pdf")
			}
		}()
	}
	wg.Wait()
}
func controlSection(id string, start SectionStart, blocks ...Block) *Section {
	stories := func(role string) RunningStories {
		return RunningStories{Default: sectionStory(id + "_" + role + "_DEFAULT"), First: sectionStory(id + "_" + role + "_FIRST"), Even: sectionStory(id + "_" + role + "_EVEN")}
	}
	return &Section{Start: start, FirstPage: true, Headers: stories("HEAD"), Footers: stories("FOOT"), Blocks: blocks}
}

type controlCase struct {
	name     string
	sections []*Section
	pages    []string
}

func sectionControlCases() []controlCase {
	p := sectionParagraph
	br := Block{Kind: PageBreak}
	before := func(s string) Block { b := p(s); b.PageBreakBefore = true; return b }
	return []controlCase{
		{"leading", []*Section{controlSection("A", SectionEvenPage, br, br, p("A_BODY"))}, []string{"A_FIRST", "A_EVEN", "A_DEFAULT A_BODY"}},
		{"adjacent", []*Section{controlSection("A", "", p("A_BODY"), br), controlSection("B", SectionContinuous, br, p("B_BODY"))}, []string{"A_FIRST A_BODY", "A_EVEN", "B_EVEN B_BODY"}},
		{"trailing-next", []*Section{controlSection("A", "", p("A_BODY"), br), controlSection("B", SectionNextPage, p("B_BODY"))}, []string{"A_FIRST A_BODY", "A_EVEN", "B_FIRST B_BODY"}},
		{"empty-continuous", []*Section{controlSection("A", ""), controlSection("B", SectionContinuous), controlSection("C", SectionContinuous, p("C_BODY"), br, p("C_NEXT"))}, []string{"A_FIRST C_BODY", "C_EVEN C_NEXT"}},
		{"empty-next", []*Section{controlSection("A", "", p("A_BODY")), controlSection("B", SectionNextPage), controlSection("C", SectionContinuous, p("C_BODY"), br, p("C_NEXT"))}, []string{"A_FIRST A_BODY", "B_FIRST C_BODY", "C_EVEN C_NEXT"}},
		{"empty-parity", []*Section{controlSection("A", "", p("A_BODY")), controlSection("B", SectionOddPage), controlSection("C", SectionEvenPage, p("C_BODY"))}, []string{"A_FIRST A_BODY", "", "B_FIRST", "C_FIRST C_BODY"}},
		{"ending-empty", []*Section{controlSection("A", "", p("A_BODY"), br, p("A_NEXT")), controlSection("B", SectionContinuous), controlSection("C", SectionContinuous)}, []string{"A_FIRST A_BODY", "A_EVEN A_NEXT"}},
		{"control-only", []*Section{controlSection("A", "", p("A_BODY")), controlSection("B", SectionContinuous, br, br), controlSection("C", SectionContinuous, p("C_BODY"), br, p("C_NEXT"))}, []string{"A_FIRST A_BODY", "B_EVEN", "B_DEFAULT C_BODY", "C_EVEN C_NEXT"}},
		{"conditional", []*Section{controlSection("A", ""), controlSection("B", SectionContinuous, before("B_BODY"), before("B_NEXT")), controlSection("C", SectionContinuous, before("C_BODY"))}, []string{"A_FIRST B_BODY", "B_EVEN B_NEXT", "C_EVEN C_BODY"}},
	}
}
func controlDocument(sections []*Section) *Document {
	d := &Document{Layout: Layout{Width: 300, Height: 400, Margin: 30}, EvenAndOddHeaders: true}
	for _, s := range sections {
		d.Blocks = append(d.Blocks, Block{Kind: SectionBlock, Section: s})
	}
	return d
}
func assertControlPages(t *testing.T, d *Document, want []string) *Document {
	t.Helper()
	before := canonicalSF(t, d)
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, PDF, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	back, err := ReadPDF(context.Background(), &out, ReadOptions{PDFSkipInference: true, PDFSkipStructure: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(back.Pages) != len(want) {
		t.Fatalf("pages=%d want=%d; text=%q", len(back.Pages), len(want), back.Text())
	}
	texts := make([]string, len(want))
	for _, b := range back.Blocks {
		if b.Source != nil && len(b.Source.Regions) > 0 {
			texts[b.Source.Regions[0].Page-1] += (&Document{Blocks: []Block{b}}).Text() + "\n"
		}
	}
	for page, expected := range want {
		tokens := strings.Fields(expected)
		var bodyTokens []string
		if len(tokens) > 0 {
			bodyTokens = tokens[1:]
		}
		head, foot := "", ""
		if len(tokens) > 0 {
			owners := strings.Split(tokens[0], "/")
			if len(owners) == 1 {
				owners = append(owners, owners[0])
			}
			for i, role := range []string{"HEAD", "FOOT"} {
				if owners[i] == "-" {
					continue
				}
				bits := strings.SplitN(owners[i], "_", 2)
				if i == 0 {
					head = bits[0] + "_" + role + "_" + bits[1]
				} else {
					foot = bits[0] + "_" + role + "_" + bits[1]
				}
			}
		}
		for _, id := range []string{"A", "B", "C"} {
			for _, role := range []string{"HEAD", "FOOT"} {
				for _, variant := range []string{"FIRST", "EVEN", "DEFAULT"} {
					token := id + "_" + role + "_" + variant
					count := 0
					if token == head || token == foot {
						count = 1
					}
					if strings.Count(texts[page], token) != count {
						t.Fatalf("page %d owner=%s want=%s; text=%q", page+1, token, expected, texts[page])
					}
				}
			}
		}
		for _, s := range d.Blocks {
			for _, block := range s.Section.Blocks {
				for _, run := range block.Runs {
					if strings.Contains(run.Text, "_BODY") || strings.Contains(run.Text, "_NEXT") {
						expectedCount := 0
						for _, token := range bodyTokens {
							if token == run.Text {
								expectedCount++
							}
						}
						if strings.Count(texts[page], run.Text) != expectedCount {
							t.Fatalf("page %d body %s: %q", page+1, run.Text, texts[page])
						}
					}
				}
			}
		}
		if expected == "" && strings.TrimSpace(texts[page]) != "" {
			t.Fatal("parity filler must be completely blank", texts[page])
		}
	}
	if !bytes.Equal(before, canonicalSF(t, d)) {
		t.Fatal("source controls/section settings mutated")
	}
	checkParagraphRoundtrip(t, d)
	return back
}
func TestPDFSectionControlOwnership(t *testing.T) {
	for _, c := range sectionControlCases() {
		t.Run(c.name, func(t *testing.T) { assertControlPages(t, controlDocument(c.sections), c.pages) })
	}
}
func TestPDFSectionControlsDrainNotesBeforeBoundary(t *testing.T) {
	br := Block{Kind: PageBreak}
	a := controlSection("A", "", sectionParagraph("A_BODY"), br)
	a.Blocks[0].Runs = append(a.Blocks[0].Runs, Run{Note: &NoteReference{ID: "old", Kind: Footnote}})
	b := controlSection("B", SectionContinuous, br, sectionParagraph("B_BODY"))
	c := controlSection("C", SectionOddPage, sectionParagraph("C_BODY"))
	d := controlDocument([]*Section{a, b, c})
	var lines []Block
	for i := 0; i < 32; i++ {
		lines = append(lines, sectionParagraph(fmt.Sprintf("NOTE_%02d", i)))
	}
	d.Notes = []Note{{ID: "old", Kind: Footnote, Blocks: lines}}
	back := assertControlPages(t, d, []string{"A_FIRST A_BODY", "A_EVEN", "A_DEFAULT", "B_EVEN B_BODY", "C_FIRST C_BODY"})
	for i := 0; i < 32; i++ {
		if strings.Count(back.Text(), fmt.Sprintf("NOTE_%02d", i)) != 1 {
			t.Fatal("note content lost/repeated", back.Text())
		}
	}
	for _, block := range back.Blocks {
		for _, run := range block.Runs {
			if strings.HasPrefix(run.Text, "NOTE_") && run.Source.Page > 2 {
				t.Fatal("notes crossed explicit control boundary", run.Source)
			}
		}
	}
}

func TestPDFSectionControlIndependentInheritance(t *testing.T) {
	a := controlSection("A", "", sectionParagraph("A_BODY"))
	b := &Section{Start: SectionContinuous, FirstPage: true, Footers: RunningStories{Even: &RunningStory{}}, Blocks: []Block{{Kind: PageBreak}, sectionParagraph("B_BODY")}}
	c := &Section{Start: SectionNextPage, FirstPage: true, Headers: RunningStories{First: &RunningStory{}}, Blocks: []Block{sectionParagraph("C_BODY")}}
	assertControlPages(t, controlDocument([]*Section{a, b, c}), []string{"A_FIRST A_BODY", "A_EVEN/- B_BODY", "-/A_FIRST C_BODY"})
}
func TestPDFEmptyPendingSectionKeepsParityCanvasAndFooterOwner(t *testing.T) {
	a := controlSection("A", "", sectionParagraph("A_BODY"))
	b := controlSection("B", SectionContinuous)
	b.Layout = &Layout{Width: 300, Height: 400, Margins: &PageMargins{Top: 40, Right: 20, Bottom: 45, Left: 90}}
	c := controlSection("C", SectionOddPage, sectionParagraph("C_BODY"), Block{Kind: PageBreak}, sectionParagraph("C_NEXT"))
	c.Layout = &Layout{Width: 400, Height: 300, Margin: 20}
	back := assertControlPages(t, controlDocument([]*Section{a, b, c}), []string{"A_FIRST A_BODY", "", "C_FIRST C_BODY", "C_EVEN C_NEXT"})
	for i, p := range back.Pages {
		w, h := 300.0, 400.0
		if i >= 2 {
			w, h = 400, 300
		}
		if p.Width != w || p.Height != h {
			t.Fatal("parity/empty pending section changed canvas owner", i+1, p)
		}
	}
	for _, b := range back.Blocks {
		for _, run := range b.Runs {
			if run.Text == "A_FOOT_FIRST" && (run.Source.Baseline == nil || run.Source.Baseline[0].X != 30) {
				t.Fatal("empty pending section moved old footer", run.Source)
			}
		}
	}
}
func TestPDFEmptyContinuousFinalOwnerBindsEndnotes(t *testing.T) {
	a := controlSection("A", "", sectionParagraph("A_BODY"), Block{Kind: PageBreak}, sectionParagraph("A_NEXT"))
	a.Blocks[0].Runs = append(a.Blocks[0].Runs, Run{Note: &NoteReference{Kind: Endnote, ID: "ending"}})
	b := controlSection("B", SectionContinuous)
	c := controlSection("C", SectionContinuous)
	c.Layout = &Layout{Width: 300, Height: 400, Margins: &PageMargins{Top: 60, Right: 20, Bottom: 50, Left: 70}}
	d := controlDocument([]*Section{a, b, c})
	d.Notes = []Note{{Kind: Endnote, ID: "ending", Blocks: []Block{sectionParagraph("ENDNOTE_CONTENT")}}}
	back := assertControlPages(t, d, []string{"A_FIRST A_BODY", "A_EVEN A_NEXT", "C_EVEN"})
	found := false
	for _, b := range back.Blocks {
		for _, run := range b.Runs {
			if run.Text == "ENDNOTE_CONTENT" {
				found = true
				if run.Source.Page != 3 || run.Source.Baseline == nil || run.Source.Baseline[0].X < 70 || run.Source.Baseline[0].Y < 60 {
					t.Fatal("endnote left final empty continuous frame", run.Source)
				}
			}
		}
	}
	if !found {
		t.Fatal("empty final owner discarded endnote", back.Text())
	}
}

func TestSectionedDOCXDeclaresGlobalNoteNumberingInEverySection(t *testing.T) {
	for _, even := range []bool{false, true} {
		t.Run(fmt.Sprint(even), func(t *testing.T) {
			a, b := controlSection("A", "", sectionParagraph("A_BODY")), controlSection("B", SectionNextPage, sectionParagraph("B_BODY"))
			d := controlDocument([]*Section{a, b, controlSection("C", SectionContinuous)})
			d.EvenAndOddHeaders = even
			for i, s := range []*Section{a, b} {
				for _, kind := range []string{Footnote, Endnote} {
					id := fmt.Sprintf("%s-%d", kind, i)
					s.Blocks[0].Runs = append(s.Blocks[0].Runs, Run{Note: &NoteReference{Kind: kind, ID: id}})
					d.Notes = append(d.Notes, Note{Kind: kind, ID: id, Blocks: []Block{sectionParagraph(id + "_CONTENT")}})
				}
			}
			checkParagraphRoundtrip(t, d)
			var out bytes.Buffer
			if err := Write(context.Background(), &out, d, DOCX, WriteOptions{}); err != nil {
				t.Fatal(err)
			}
			archive, err := zip.NewReader(bytes.NewReader(out.Bytes()), int64(out.Len()))
			if err != nil {
				t.Fatal(err)
			}
			var body []byte
			for _, file := range archive.File {
				if file.Name == "word/document.xml" {
					r, err := file.Open()
					if err != nil {
						t.Fatal(err)
					}
					body, err = io.ReadAll(r)
					r.Close()
					if err != nil {
						t.Fatal(err)
					}
				}
			}
			var document struct {
				Body struct {
					Paragraphs []struct {
						Properties struct {
							Section *numberedSection `xml:"sectPr"`
						} `xml:"pPr"`
					} `xml:"p"`
					Final *numberedSection `xml:"sectPr"`
				} `xml:"body"`
			}
			if err := xml.Unmarshal(body, &document); err != nil {
				t.Fatal(err)
			}
			var sections []*numberedSection
			for _, p := range document.Body.Paragraphs {
				if p.Properties.Section != nil {
					sections = append(sections, p.Properties.Section)
				}
			}
			sections = append(sections, document.Body.Final)
			if len(sections) != 3 {
				t.Fatal("section properties lost", len(sections))
			}
			for _, s := range sections {
				if s == nil {
					t.Fatal("missing final section")
				}
				for _, p := range []numberedNote{s.Footnote, s.Endnote} {
					if p.Format.Value != "decimal" || p.Restart.Value != "continuous" {
						t.Fatal("section falls back to producer note policy", p)
					}
				}
			}
		})
	}
}

type numberedNote struct {
	Format struct {
		Value string `xml:"val,attr"`
	} `xml:"numFmt"`
	Restart struct {
		Value string `xml:"val,attr"`
	} `xml:"numRestart"`
}
type numberedSection struct {
	Footnote numberedNote `xml:"footnotePr"`
	Endnote  numberedNote `xml:"endnotePr"`
}
