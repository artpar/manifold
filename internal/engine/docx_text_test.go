package engine

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"reflect"
	"strings"
	"testing"
)

func paragraphDocument(t testing.TB) *Document {
	t.Helper()
	data, err := os.ReadFile("examples/paragraph-breaks/document.sf")
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
func canonicalSF(t testing.TB, d *Document) []byte {
	t.Helper()
	s, err := ToStructured(d)
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
func checkParagraphRoundtrip(t testing.TB, d *Document) {
	t.Helper()
	want := canonicalSF(t, d)
	for _, format := range []Format{SF, DOCX} {
		var out bytes.Buffer
		if err := Write(context.Background(), &out, d, format, WriteOptions{}); err != nil {
			t.Fatal(err)
		}
		got, err := Read(context.Background(), &out, format, ReadOptions{})
		if err != nil {
			t.Fatal(err)
		}
		actual := canonicalSF(t, got)
		if !bytes.Equal(want, actual) {
			t.Fatalf("%s changed break semantics\nwant %s\ngot %s", format, want, actual)
		}
		if !bytes.Equal(want, canonicalSF(t, d)) {
			t.Fatal("writer changed source")
		}
	}
}
func TestParagraphBreakPropertiesRoundtripAndPDF(t *testing.T) {
	d := paragraphDocument(t)
	checkParagraphRoundtrip(t, d)
	got, _ := renderPageBreakDocument(t, d)
	if got.Metadata.PageCount != 8 {
		t.Fatalf("conditional break pages=%d want8", got.Metadata.PageCount)
	}
	for token, page := range map[string]int{"FIRST": 1, "SECOND": 2, "NOTE_FIRST": 3, "NOTE_SECOND": 4, "ITEM_FIRST": 5, "ITEM_CONT": 6, "CELL_FIRST": 7, "CELL_SECOND": 8, "SIDE_ZERO": 7, "SIDE_ONE": 8, "SIDE_TWO": 8, "AFTER_TABLE": 8} {
		checkTokenPage(t, got, token, page)
	}
}
func TestParagraphPropertyAtPageStartDoesNotCreateBlankPage(t *testing.T) {
	for _, table := range []bool{false, true} {
		p := mergeParagraph("TOP")
		p.PageBreakBefore = true
		d := &Document{Layout: Layout{Width: 300, Height: 180, Margin: 20}, Blocks: []Block{p}}
		if table {
			d.Blocks = []Block{cellTable([]Block{p})}
		}
		got, _ := renderPageBreakDocument(t, d)
		if got.Metadata.PageCount != 1 {
			t.Fatal("conditional property added blank first page")
		}
		checkTokenPage(t, got, "TOP", 1)
	}
}
func TestDOCXParagraphInheritanceAndCancellation(t *testing.T) {
	styles := `<w:docDefaults><w:pPrDefault><w:pPr><w:pageBreakBefore/></w:pPr></w:pPrDefault></w:docDefaults><w:style w:type="paragraph" w:styleId="Normal" w:default="1"><w:pPr><w:pageBreakBefore w:val="false"/></w:pPr></w:style><w:style w:type="paragraph" w:styleId="Base"><w:pPr><w:pageBreakBefore w:val="on"/></w:pPr></w:style><w:style w:type="paragraph" w:styleId="Derived"><w:basedOn w:val="Base"/></w:style>`
	body := `<w:p><w:r><w:t>DEFAULT</w:t></w:r></w:p><w:p><w:pPr><w:pStyle w:val="Derived"/></w:pPr><w:r><w:t>INHERITED</w:t></w:r></w:p><w:p><w:pPr><w:pStyle w:val="Derived"/><w:pageBreakBefore w:val="0"/></w:pPr><w:r><w:t>CANCELLED</w:t></w:r></w:p><w:p><w:pPr><w:pageBreakBefore/></w:pPr><w:r><w:t>DIRECT</w:t></w:r></w:p>`
	d, err := ReadDOCX(context.Background(), bytes.NewReader(numberedDOCX(t, body+`<w:sectPr><w:pgSz w:w="6000" w:h="3600"/><w:pgMar w:top="400" w:right="400" w:bottom="400" w:left="400"/></w:sectPr>`, "", styles)))
	if err != nil {
		t.Fatal(err)
	}
	for i, want := range []bool{false,

		// Equal effective choices must not create spurious SF boundaries or losses.
		true, false, true} {
		if d.Blocks[i].PageBreakBefore != want {
			t.Fatalf("property%d=%t", i, d.Blocks[i].PageBreakBefore)
		}
	}
	if len(d.Diagnostics) > 0 {
		t.Fatalf("represented property diagnosed as loss: %v", d.Diagnostics)
	}
	checkParagraphRoundtrip(t, d)
	for _, value := range []string{"false", "off", "0", "true", "on", "1"} {
		source := `<w:p><w:pPr><w:pageBreakBefore w:val="` + value + `"/></w:pPr><w:r><w:t>A</w:t></w:r></w:p>`
		d, err := ReadDOCX(context.Background(), bytes.NewReader(docxWithBody(t, source)))
		if err != nil {
			t.Fatal(err)
		}
		if d.Blocks[0].PageBreakBefore != (value == "true" || value == "on" || value == "1") {
			t.Fatal("on/off value lost")
		}
	}
}
func TestDOCXParagraphBreakInvalidData(t *testing.T) {
	for _, pr := range []string{`<w:pageBreakBefore w:val="maybe"/>`, `<w:pageBreakBefore w:val=""/>`, `<w:pageBreakBefore/><w:pageBreakBefore/>`} {
		source := `<w:p><w:pPr>` + pr + `</w:pPr><w:r><w:t>VISIBLE</w:t></w:r></w:p>`
		if _, err := ReadDOCX(context.Background(), bytes.NewReader(docxWithBody(t, source))); err == nil {
			t.Fatal("invalid paragraph boolean accepted")
		}
	}
	styles := `<w:style w:styleId="A"><w:basedOn w:val="B"/></w:style><w:style w:styleId="B"><w:basedOn w:val="A"/></w:style>`
	if _, err := ReadDOCX(context.Background(), bytes.NewReader(numberedDOCX(t, `<w:p><w:pPr><w:pStyle w:val="A"/></w:pPr></w:p>`, "", styles))); err == nil {
		t.Fatal("style cycle accepted")
	}
}
func TestDOCXExplicitAndConditionalBreaksRemainDistinct(t *testing.T) {
	p := mergeParagraph("VALUE")
	p.PageBreakBefore = true
	flow := []Block{{Kind: PageBreak}, {Kind: PageBreak}, p, {Kind: PageBreak}, {Kind: PageBreak}}
	d := &Document{Layout: Layout{Width: 300, Height: 180, Margin: 20}, Blocks: flow}
	checkParagraphRoundtrip(t, d)
	got, _ := renderPageBreakDocument(t, d)
	if got.Metadata.PageCount != 5 {
		t.Fatalf("lost unconditional blank pages: %d", got.Metadata.PageCount)
	}
	checkTokenPage(t, got, "VALUE", 3)
	d = &Document{Layout: d.Layout, Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "REF"}, noteRun(Footnote, "n")}}}, Notes: []Note{{Kind: Footnote, ID: "n", Blocks: flow}}}
	checkParagraphRoundtrip(t, d)
}
func TestDOCXBreakMetadataCannotDiscardVisibleContent(t *testing.T) {
	d := &Document{Blocks: []Block{{Kind: PageBreak},

		// With no themeFontLang, the region node supplies the default family.

		{Kind: PageBreak}, mergeParagraph("VISIBLE")}}
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, DOCX, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(string) string{
		func(s string) string {
			return strings.Replace(s, `<w:p md:breakRole="anchor"/>`, `<w:p md:breakRole="anchor"><w:r><w:t>KEEP</w:t></w:r></w:p>`, 1)
		},
		func(s string) string {
			return strings.Replace(s, `md:breakRole="control"`, `md:breakRole="unknown"`, 1)
		},
		func(s string) string {
			return strings.Replace(s, `<w:pageBreakBefore/>`, `<w:pageBreakBefore w:val="false"/>`, 1)
		},
		func(s string) string {
			return strings.Replace(s, `md:breakRole="before"`, `md:breakRole="before" md:conditionalBefore="0"`, 1)
		},
	} {
		bad := rewriteListDOCX(t, out.Bytes(), change)
		if _, err := ReadDOCX(context.Background(), bytes.NewReader(bad)); err == nil {
			t.Fatal("contradictory metadata accepted")
		}
	}
}
func TestParagraphBreakBeforeAppliesOnlyToFirstInlineSegment(t *testing.T) {
	source := `<w:p><w:pPr><w:pageBreakBefore/></w:pPr><w:r><w:br w:type="page"/><w:t>AFTER_INLINE</w:t></w:r></w:p>`
	d, err := ReadDOCX(context.Background(), bytes.NewReader(docxWithBody(t, source)))
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Blocks) != 3 || !d.Blocks[0].PageBreakBefore || d.Blocks[1].Kind != PageBreak || d.Blocks[2].PageBreakBefore {
		t.Fatal("inline split duplicated/reordered paragraph property")
	}
}
func FuzzDOCXParagraphBreakRoundtrip(f *testing.F) {
	f.Add([]byte{0, 0, 1, 2, 0}, byte(0))
	f.Add([]byte{1, 2, 0, 3, 0, 0}, byte(2))
	f.Fuzz(func(t *testing.T, data []byte, depth byte) {
		if len(data) > 16 {
			data = data[:16]
		}
		flow := []Block{}
		for i, b := range data {
			if b%4 == 0 {
				flow = append(flow, Block{Kind: PageBreak})
			} else {
				p := mergeParagraph(fmt.Sprintf("TOKEN_%02d", i))
				p.PageBreakBefore = b%4 == 1
				if b%4 == 3 {
					p.Kind = Heading
					p.Level = 2
				}
				flow = append(flow, p)
			}
		}
		for i := 0; i < int(depth%3); i++ {
			flow = []Block{cellTable(flow), mergeParagraph(fmt.Sprintf("WRAP_END_%d", i))}
		}
		d := &Document{Layout: Layout{Width: 480, Height: 600, Margin: 20}, Blocks: append([]Block{{Kind: Paragraph, Runs: []Run{{Text: "REF"}, noteRun(Footnote, "n")}}}, flow...), Notes: []Note{{Kind: Footnote, ID: "n", Blocks: flow}}}
		checkParagraphRoundtrip(t, d)
	})
}

func TestDOCXHeadingDefaultDoesNotInventExplicitBoldMark(t *testing.T) {
	d := &Document{Layout: Layout{Width: 300, Height: 180, Margin: 20}, Blocks: []Block{{Kind: Heading, Level: 2, Runs: []Run{{Text: "INHERITED"}, {Text: "EXPLICIT", Bold: true}}, PageBreakBefore: true}}}
	checkParagraphRoundtrip(t, d)
}

func TestDOCXStructuralParagraphsDoNotInventCellContent(t *testing.T) {
	d := &Document{Layout: Layout{Width: 300, Height: 180, Margin: 20}, Blocks: []Block{cellTable([]Block{cellTable(nil)})}}
	checkParagraphRoundtrip(t, d)
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, DOCX, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	bad := rewriteListDOCX(t, out.Bytes(), func(s string) string {
		return strings.Replace(s, `<w:p md:emptyCell="1"/>`, `<w:p md:emptyCell="1"><w:r><w:t>VISIBLE</w:t></w:r></w:p>`, 1)
	})
	if _, err := ReadDOCX(context.Background(), bytes.NewReader(bad)); err == nil {
		t.Fatal("structural metadata hid visible cell content")

		// Unknown unused theme slots cannot prevent extraction of unrelated text.

	}
}

func FuzzPDFParagraphStarts(f *testing.F) {
	f.Add(byte(63), byte(63), false)
	f.Add(byte(1), byte(32), true)
	f.Fuzz(func(t *testing.T, left, right byte, prefix bool) {
		flows := [][]Block{}
		masks := []byte{left, right}
		for side, name := range []string{"AA_", "BB_"} {
			flow := []Block{}
			for i := 0; i < 6; i++ {
				p := mergeParagraph(fmt.Sprintf("%s%02d", name, i))
				p.PageBreakBefore = masks[side]&(1<<i) != 0
				flow = append(flow, p)
			}
			flows = append(flows, flow)
		}
		d := &Document{Layout: Layout{Width: 480, Height: 600, Margin: 20}, Blocks: []Block{cellTable(flows...)}}
		if prefix {
			d.Blocks = append([]Block{mergeParagraph("PREFIX")}, d.Blocks...)
		}
		got, _ := renderPageBreakDocument(t, d)
		page := 1
		for i := 0; i < 6; i++ {
			if (i > 0 || prefix) && (left|right)&(1<<i) != 0 {
				page++
			}
			for _, name := range []string{"AA_", "BB_"} {
				checkTokenPage(t, got, fmt.Sprintf("%s%02d", name, i), page)
			}
		}
		if got.Metadata.PageCount != page {
			t.Fatalf("conditional property added blank page %d want%d", got.Metadata.PageCount, page)
		}
	})
}

func TestParagraphPropertyKindAndRunningContentValidation(t *testing.T) {
	if err := (&Document{Blocks: []Block{{Kind: PageBreak, PageBreakBefore: true}}}).Validate(); err == nil {
		t.Fatal("paragraph property accepted on a control")
	}
	for _, attrs := range []string{`{"pageBreakBefore":"true"}`, `{"pageBreakBefore":1}`, `{"pageBreakBefore":false,"level":0}`} {
		source := `{"type":"doc","attrs":{"schema":"manydoc-prosemirror/1"},"content":[{"type":"paragraph","attrs":` + attrs + `}]}`
		sf, err := ReadStructured(context.Background(), strings.NewReader(source))
		if err == nil {
			_, err = FromStructured(sf)
		}
		if err == nil {
			t.Fatal("invalid paragraph attributes accepted")
		}
	}
}
func fontSelectionDOCX(t testing.TB, body, styles, settings, scheme, table string) []byte {
	t.Helper()
	input := themeColorDOCX(t, body, themeColorPalette, settings, styles)
	zr, err := zip.NewReader(bytes.NewReader(input), int64(len(input)))
	if err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	zw := zip.NewWriter(&b)
	for _, f := range zr.File {
		reader, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(reader)
		reader.Close()
		if err != nil {
			t.Fatal(err)
		}
		value := string(data)
		if f.Name == "word/theme/theme1.xml" {
			value = strings.Replace(value, "</a:themeElements>", `<a:fontScheme name="test">`+scheme+`</a:fontScheme></a:themeElements>`, 1)
		}
		if f.Name == "word/_rels/document.xml.rels" && table != "" {
			value = strings.Replace(value, "</Relationships>", `<Relationship Id="fonts" Type="`+relNS+`/fontTable" Target="fontTable.xml"/></Relationships>`, 1)
		}
		if f.Name == "word/_rels/document.xml.rels" && strings.Contains(body, `r:id="fontLink"`) {
			value = strings.Replace(value, "</Relationships>", `<Relationship Id="fontLink" Type="`+relNS+`/hyperlink" Target="https://example.invalid/fonts" TargetMode="External"/></Relationships>`, 1)
		}
		w, err := zw.Create(f.Name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = w.Write([]byte(value)); err != nil {
			t.Fatal(err)
		}
	}
	if table != "" {
		w, err := zw.Create("word/fontTable.xml")
		if err != nil {
			t.Fatal(err)
		}
		_, err = w.Write([]byte(`<w:fonts xmlns:w="` + wordNS + `">` + table + `</w:fonts>`))
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
func fontRun(properties, text string) string {
	return `<w:r><w:rPr>` + properties + `</w:rPr><w:t xml:space="preserve">` + text + `</w:t></w:r>`
}
func readFontSelection(t *testing.T, body, styles, settings, scheme, table string) *Document {
	t.Helper()
	d, err := ReadDOCX(context.Background(), bytes.NewReader(fontSelectionDOCX(t, body, styles, settings, scheme, table)))
	if err != nil {
		t.Fatal(err)
	}
	return d
}

const selectionFonts = `<w:rFonts w:ascii="ASCII" w:hAnsi="High" w:eastAsia="East" w:cs="Complex"/>`

func TestDOCXFontSlotsPreserveActualSpans(t *testing.T) {
	d := readFontSelection(t, `<w:p>`+fontRun(selectionFonts+`<w:b/><w:color w:val="123456"/>`, "Aé中文ΩZ")+`</w:p>`, "", "", "", "")
	runs := d.Blocks[0].Runs
	wantText := []string{"A", "é", "中文", "Ω", "Z"}
	wantFonts := []string{"ASCII", "High", "East", "High", "ASCII"}
	if len(runs) != len(wantText) {
		t.Fatalf("spans %+v", runs)
	}
	for i, r := range runs {
		if r.Text != wantText[i] || r.FontFamily != wantFonts[i] || !r.Bold || r.Color != "#123456" {
			t.Fatalf("span %d %+v", i, r)
		}
	}
	if !hasDiagnostic(d, "font_selectors") {
		t.Fatal("latent selector loss not disclosed")
	}
	assertRecoverySF(t, d)
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, DOCX, WriteOptions{AllowLossy: true}); err != nil {
		t.Fatal(err)
	}
	back, err := ReadDOCX(context.Background(), &out)
	if err != nil {
		t.Fatal(err)
	}
	for i, r := range back.Blocks[0].Runs {
		if r.Text != runs[i].Text || r.FontFamily != runs[i].FontFamily || r.Bold != runs[i].Bold || r.Color != runs[i].Color {
			t.Fatalf("resolved DOCX span %+v", r)
		}
	}

	d = readFontSelection(t, `<w:p>`+fontRun(`<w:rFonts w:ascii="Go" w:hAnsi="Go" w:eastAsia="Go" w:cs="Go"/>`, "AéΩ中文Z")+`</w:p>`, "", "", "", "")
	if len(d.Blocks[0].Runs) != 1 || hasDiagnostic(d, "font_selectors") {
		t.Fatalf("identical choices altered spans %+v", d)
	}
}
func TestDOCXFontInheritanceAndThemeLanguage(t *testing.T) {
	scheme := `<a:majorFont><a:latin typeface="Major Latin"/><a:ea typeface="Major East"/><a:cs typeface="Major Complex"/><a:font script="Jpan" typeface="Japanese Major"/><a:font script="Arab" typeface="Arabic Major"/></a:majorFont><a:minorFont><a:latin typeface="Minor Latin"/><a:ea typeface="Minor East"/><a:cs typeface="Minor Complex"/><a:font script="Hans" typeface="Chinese Minor"/></a:minorFont>`
	defaults := `<w:docDefaults><w:rPrDefault><w:rPr><w:rFonts w:asciiTheme="majorAscii" w:hAnsiTheme="minorHAnsi" w:eastAsiaTheme="minorEastAsia" w:cstheme="majorBidi"/></w:rPr></w:rPrDefault></w:docDefaults>`
	styles := defaults + `<w:style w:type="paragraph" w:styleId="P"><w:rPr><w:rFonts w:ascii="Direct ASCII"/></w:rPr></w:style><w:style w:type="character" w:styleId="C"><w:rPr><w:rFonts w:hAnsi="Direct High"/></w:rPr></w:style>`
	body := `<w:p><w:pPr><w:pStyle w:val="P"/></w:pPr>` + fontRun(`<w:rStyle w:val="C"/>`, "Aé中") + `</w:p>`
	settings := `<w:themeFontLang w:val="ja-JP" w:eastAsia="zh-CN" w:bidi="ar-SA"/>`
	d := readFontSelection(t, body, styles, settings, scheme, "")
	for i, want := range []string{"Direct ASCII", "Direct High", "Chinese Minor"} {
		if d.Blocks[0].Runs[i].FontFamily != want {
			t.Fatalf("inheritance %+v", d.Blocks[0].Runs)
		}
	}
	d = readFontSelection(t, `<w:p>`+fontRun("", "Aé中")+`</w:p>`, defaults, settings, scheme, "")
	for i, want := range []string{"Japanese Major", "Minor Latin", "Chinese Minor"} {
		if d.Blocks[0].Runs[i].FontFamily != want {
			t.Fatalf("theme language %+v", d.Blocks[0].Runs)
		}
	}
	d = readFontSelection(t, `<w:p>`+fontRun(`<w:rtl/><w:bCs/><w:szCs w:val="30"/>`, "ABC")+`</w:p>`, defaults, settings, scheme, "")
	run := d.Blocks[0].Runs[0]
	if run.FontFamily != "Arabic Major" || !run.Bold || run.FontSize != 15 || run.Direction != "rtl" || hasDiagnostic(d, "run_direction") {
		t.Fatalf("complex theme %+v", run)
	}

	d = readFontSelection(t, `<w:p>`+fontRun("", "Aé中")+`</w:p>`, defaults, "", scheme, "")
	for i, want := range []string{"Major Latin", "Minor Latin", "Minor East"} {
		if d.Blocks[0].Runs[i].FontFamily != want {
			t.Fatalf("region defaults %+v", d.Blocks[0].Runs)
		}
	}
}
func TestDOCXFontHintsAndComplexProperties(t *testing.T) {
	for _, tc := range []struct{ name, text, props, table, want string }{
		{"hint Greek", "Ω", selectionFonts + `<w:lang w:eastAsia="ja-JP"/>`, "", "High"},
		{"East Asian Greek", "Ω", strings.Replace(selectionFonts, "/>", ` w:hint="eastAsia"/>`, 1), "", "East"},
		{"Chinese accented", "à", strings.Replace(selectionFonts, "/>", ` w:hint="eastAsia"/>`, 1) + `<w:lang w:eastAsia="zh-TW"/>`, "", "East"},
		{"Latin accented", "à", strings.Replace(selectionFonts, "/>", ` w:hint="eastAsia"/>`, 1) + `<w:lang w:eastAsia="ja-JP"/>`, "", "High"},
		{"charset exception", "Ā", strings.Replace(selectionFonts, "/>", ` w:hint="eastAsia"/>`, 1), `<w:font w:name="East"><w:charset w:val="86"/></w:font>`, "East"},
		{"no charset guess", "Ā", strings.Replace(selectionFonts, "/>", ` w:hint="eastAsia"/>`, 1), "", "High"},
		{"forced complex", "ABC", selectionFonts + `<w:cs/>`, "", "Complex"},
		{"force disabled", "ABC", selectionFonts + `<w:cs w:val="0"/>`, "", "ASCII"},
		{"TNR exception", "中", `<w:rFonts w:ascii="Go" w:hAnsi="Go" w:eastAsia="Times New Roman"/>`, "", "Go"},
		{"supplementary", "𐐀", selectionFonts, "", "East"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := readFontSelection(t, `<w:p>`+fontRun(tc.props, tc.text)+`</w:p>`, "", "", "", tc.table)
			r := d.Blocks[0].Runs[0]
			if r.Text != tc.text || r.FontFamily != tc.want {
				t.Fatalf("font %+v", r)
			}
			if tc.name == "no charset guess" && !hasDiagnostic(d, "font_charset") {
				t.Fatal("uncertain charset not disclosed")
			}
			assertRecoverySF(t, d)
		})
	}
	styles := `<w:style w:type="paragraph" w:styleId="P"><w:rPr><w:cs/><w:bCs/><w:iCs/><w:szCs w:val="32"/></w:rPr></w:style><w:style w:type="character" w:styleId="C"><w:rPr><w:bCs/></w:rPr></w:style>`
	d := readFontSelection(t, `<w:p><w:pPr><w:pStyle w:val="P"/></w:pPr>`+fontRun(selectionFonts+`<w:rStyle w:val="C"/>`, "A")+fontRun(selectionFonts+`<w:rStyle w:val="C"/><w:bCs/>`, "B")+`</w:p>`, styles, "", "", "")
	a, b := d.Blocks[0].Runs[0], d.Blocks[0].Runs[1]
	if a.Bold || !b.Bold || !a.Italic || !b.Italic || a.FontSize != 16 || a.FontFamily != "Complex" {
		t.Fatalf("complex toggles/direct %+v %+v", a, b)
	}
}
func TestDOCXFontSelectionInvalidAndFallback(t *testing.T) {
	for _, pr := range []string{`<w:rFonts w:asciiTheme="majorBogus"/>`, `<w:rFonts w:hint="bogus"/>`, `<w:rFonts w:cstheme="majorBidi" w:csTheme="minorBidi"/>`, `<w:cs w:val="maybe"/>`, `<w:szCs w:val="0"/>`} {
		if _, err := ReadDOCX(context.Background(), bytes.NewReader(fontSelectionDOCX(t, `<w:p>`+fontRun(pr, "ABC")+`</w:p>`, "", "", "", ""))); err == nil {
			t.Fatalf("invalid font property accepted %s", pr)
		}
	}
	for _, settings := range []string{`<w:themeFontLang/><w:themeFontLang/>`} {
		if _, err := ReadDOCX(context.Background(), bytes.NewReader(fontSelectionDOCX(t, `<w:p>`+fontRun(selectionFonts, "A")+`</w:p>`, "", settings, "", ""))); err == nil {
			t.Fatal("duplicate theme languages accepted")
		}
	}

	d := readFontSelection(t, `<w:p>`+fontRun(`<w:rFonts w:ascii="Go" w:eastAsiaTheme="majorEastAsia"/>`, "ASCII")+`</w:p>`, "", "", "", "")
	if d.Blocks[0].Runs[0].FontFamily != "Go" {
		t.Fatal("unused theme slot changed text")
	}
	d = readFontSelection(t, `<w:p>`+fontRun(`<w:rFonts w:ascii="Cached" w:asciiTheme="majorAscii"/>`, "ASCII")+`</w:p>`, "", "", "", "")
	if d.Blocks[0].Runs[0].FontFamily != "Cached" || !hasDiagnostic(d, "font_theme_cache") {
		t.Fatal("missing theme cache evidence")
	}
	if _, err := ReadDOCX(context.Background(), bytes.NewReader(fontSelectionDOCX(t, `<w:p>`+fontRun(`<w:rFonts w:eastAsiaTheme="majorEastAsia"/>`, "中")+`</w:p>`, "", "", "", ""))); err == nil {
		t.Fatal("used unresolved theme accepted")
	}
}
func FuzzDOCXScriptFontSelection(f *testing.F) {
	f.Add("Aé中ΩZ", false)
	f.Add("Latin Ελληνικά Кириллица", true)
	f.Fuzz(func(t *testing.T, text string, hint bool) {
		if len(text) == 0 || len(text) > 2048 || strings.ContainsAny(text, "<>&") || validateString(text) != nil || strings.ContainsAny(text, "\x00\x01\x02\x03\x04\x05\x06\x07\x08\x0b\x0c\x0e\x0f\x10\x11\x12\x13\x14\x15\x16\x17\x18\x19\x1a\x1b\x1c\x1d\x1e\x1f\r") {
			t.Skip()
		}
		props := selectionFonts
		if hint {
			props = strings.Replace(props, "/>", ` w:hint="eastAsia"/>`, 1)
		}
		input := fontSelectionDOCX(t, `<w:p>`+fontRun(props, text)+`</w:p>`, "", "", "", "")
		d, err := ReadDOCX(context.Background(), bytes.NewReader(input))
		if err != nil {
			t.Fatal(err)
		}
		var actual strings.Builder
		for _, r := range d.Blocks[0].Runs {
			actual.WriteString(r.Text)
			if r.FontFamily == "" {
				t.Fatalf("used family lost: %+v", r)
			}
		}
		if actual.String() != text {
			t.Fatalf("visible content changed %q -> %q", text, actual.String())
		}
		assertRecoverySF(t, d)
		var out bytes.Buffer
		if err := Write(context.Background(), &out, d, DOCX, WriteOptions{AllowLossy: true}); err != nil {
			t.Fatal(err)
		}
		back, err := ReadDOCX(context.Background(), &out)
		if err != nil {
			t.Fatal(err)
		}
		if back.Text() != d.Text() {
			t.Fatalf("resolved text changed %q", text)
		}
	})
}

func TestDOCXScriptFontsKeepUnsupportedPDFAtomic(t *testing.T) {
	for _, text := range []string{"e\u0301", "中"} {
		props := strings.Replace(selectionFonts, "/>", ` w:hint="eastAsia"/>`, 1)
		d := readFontSelection(t, `<w:p>`+fontRun(props, text)+`</w:p>`, "", "", "", "")
		var out bytes.Buffer
		err := Write(context.Background(), &out, d, PDF, WriteOptions{AllowLossy: true, PDFFontMap: map[string]string{"ASCII": "Go", "High": "Go Mono", "East": "Go"}})
		if !errors.Is(err, ErrUnsupported) || out.Len() != 0 {
			t.Fatalf("unhandled glyph/cluster emitted output: %v", err)
		}
		if d.Text() != text {
			t.Fatal("unsupported rendering changed source content")
		}
	}
}

func TestDOCXFontSpansPreserveTableAndLinkOwnership(t *testing.T) {
	body := `<w:tbl><w:tr><w:tc><w:p xmlns:r="` + relNS + `"><w:hyperlink r:id="fontLink">` + fontRun(selectionFonts, "AΩ") + `</w:hyperlink></w:p></w:tc></w:tr></w:tbl>`
	d := readFontSelection(t, body, "", "", "", "")
	check := func(d *Document) {
		if len(d.Blocks) != 1 || d.Blocks[0].Kind != Table {
			t.Fatal("table flattened")
		}
		rr := d.Blocks[0].Rows[0].Cells[0].Blocks[0].Runs
		if len(rr) != 2 || rr[0].Text != "A" || rr[0].FontFamily != "ASCII" || rr[1].Text != "Ω" || rr[1].FontFamily != "High" || rr[0].Link != "https://example.invalid/fonts" || rr[1].Link != rr[0].Link {
			t.Fatalf("font/link ownership %+v", rr)
		}
	}
	check(d)
	// DOCX leaves default cell spans at zero; the SF profile spells them as
	// one. Compare declared SF semantics, not those equivalent Go spellings.

	// Stripping ignorable intent exposes the ordinary portable half-point size.

	// Independently authored OPC parts. No manifold writer supplies source colors.

	sf, err := ToStructured(d)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := FromStructured(sf)
	if err != nil {
		t.Fatal(err)
	}
	check(decoded)
	if !bytes.Equal(canonicalSF(t, d), canonicalSF(t, decoded)) {
		t.Fatal("SF font/table/link semantics changed")
	}

	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, DOCX, WriteOptions{AllowLossy: true}); err != nil {
		t.Fatal(err)
	}
	back, err := ReadDOCX(context.Background(), &out)
	if err != nil {
		t.Fatal(err)
	}
	check(back)
}

func TestDOCXFontImplicitMetadataChecksInheritedComplexSize(t *testing.T) {
	styles := `<w:docDefaults><w:rPrDefault><w:rPr><w:sz w:val="22"/><w:szCs w:val="24"/></w:rPr></w:rPrDefault></w:docDefaults>`
	run := fontRun(selectionFonts+`<w:cs/>`, "VISIBLE")
	run = strings.Replace(run, "<w:r>", `<w:r xmlns:md="`+docxNotesNS+`" md:textImplicit="s">`, 1)
	if _, err := ReadDOCX(context.Background(), bytes.NewReader(fontSelectionDOCX(t, `<w:p>`+run+`</w:p>`, styles, "", "", ""))); err == nil {
		t.Fatal("inherited changed complex size hidden by metadata")
	}
}
func TestDOCXSectionedStyledNoteSemantics(t *testing.T) {
	d := variantNoteDocument()
	d.Blocks[0].Anchors = []string{"return"}
	d.Notes[0].BodyLabelStyle = &Run{FontSize: 9.5, Bold: true, Link: "#return", Destination: "return"}
	d.Blocks = []Block{{Kind: SectionBlock, Section: &Section{Blocks: d.Blocks}}}
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, DOCX, WriteOptions{AllowLossy: true}); err != nil {
		t.Fatal(err)
	}
	got, err := ReadDOCX(context.Background(), &out)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := ToStructured(d)
	after, _ := ToStructured(got)
	if !reflect.DeepEqual(before.Content, after.Content) || !reflect.DeepEqual(before.Attrs.Notes, after.Attrs.Notes) {
		t.Fatal("sectioned note semantics changed")
	}
}

func precisionDocument(sectioned, variants bool) *Document {
	d := noteDocument()
	if variants {
		d = variantNoteDocument()
		d.Blocks[0].Anchors = []string{"return"}
		d.Notes[0].BodyLabelStyle = &Run{FontSize: 6.959646, Bold: true, Destination: "return", Link: "#return"}
	}
	for i := range d.Blocks {
		for j := range d.Blocks[i].Runs {
			d.Blocks[i].Runs[j].FontSize = 10.24999
		}
	}
	for i := range d.Blocks[1].Items {
		for j := range d.Blocks[1].Items[i].Blocks {
			for k := range d.Blocks[1].Items[i].Blocks[j].Runs {
				d.Blocks[1].Items[i].Blocks[j].Runs[k].FontSize = 10.24999
			}
		}
	}
	d.Blocks = append(d.Blocks, Block{Kind: Heading, Level: 2, Runs: []Run{{Text: "FRACTIONAL_HEADING", FontSize: 18.49}}})
	d.Notes[0].Blocks = []Block{{Kind: Paragraph, Runs: []Run{{Text: "FRACTIONAL_NOTE", FontSize: 9.959491}}}}
	d.Blocks = append(d.Blocks, Block{Kind: Table, Rows: []Row{{Cells: []Cell{{Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "FRACTIONAL_CELL", FontSize: 10.25, Bold: true}}}}}}}}})
	d.Headers = []Block{{Kind: Paragraph, Runs: []Run{{Text: "FRACTIONAL_HEADER", FontSize: 8.51}}}}
	d.Footers = []Block{{Kind: Paragraph, Runs: []Run{{Text: "FRACTIONAL_FOOTER", FontSize: 8.49}}}}
	if sectioned {
		d.Blocks = []Block{{Kind: SectionBlock, Section: &Section{Blocks: d.Blocks, Headers: RunningStories{Default: &RunningStory{Blocks: d.Headers}}, Footers: RunningStories{Default: &RunningStory{Blocks: d.Footers}}}}}
		d.Headers = nil
		d.Footers = nil
		d.EvenAndOddHeaders = true
		s := d.Blocks[0].Section
		s.FirstPage = true
		story := func(text string, size float64) *RunningStory {
			return &RunningStory{Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: text, FontSize: size}}}}}
		}
		s.Headers.First = story("FIRST_HEADER", 8.25)
		s.Headers.Even = story("EVEN_HEADER", 8.75)
		s.Footers.First = story("FIRST_FOOTER", 9.25)
		s.Footers.Even = story("EVEN_FOOTER", 9.75)
	}
	return d
}

func TestDOCXFractionalSizeIntentAllStories(t *testing.T) {
	for _, sectioned := range []bool{false, true} {
		for _, variants := range []bool{false, true} {
			d := precisionDocument(sectioned, variants)
			original, _ := json.Marshal(d)
			var out bytes.Buffer
			if err := Write(context.Background(), &out, d, DOCX, WriteOptions{AllowLossy: true}); err != nil {
				t.Fatal(sectioned, variants, err)
			}
			got, err := ReadDOCX(context.Background(), &out)
			if err != nil {
				t.Fatal(sectioned, variants, err)
			}
			before, _ := ToStructured(d)
			after, err := ToStructured(got)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before.Content, after.Content) || !reflect.DeepEqual(before.Attrs.Notes, after.Attrs.Notes) || !reflect.DeepEqual(before.Attrs.Headers, after.Attrs.Headers) || !reflect.DeepEqual(before.Attrs.Footers, after.Attrs.Footers) || before.Attrs.EvenAndOddHeaders != after.Attrs.EvenAndOddHeaders {
				t.Fatalf("fractional semantics changed: sectioned %v variants %v", sectioned, variants)
			}
			if !hasDiagnostic(got, "docx_font_size_precision") {
				t.Fatal("precision projection not disclosed")
			}
			current, _ := json.Marshal(d)
			if !bytes.Equal(original, current) {
				t.Fatal("writer mutated original precision")
			}
			var sf bytes.Buffer
			if err := Write(context.Background(), &sf, got, SFGzip, WriteOptions{}); err != nil {
				t.Fatal(err)
			}
			exact, err := Read(context.Background(), &sf, SFGzip, ReadOptions{})
			if err != nil {
				t.Fatal(err)
			}
			exactSF, _ := ToStructured(exact)
			if !reflect.DeepEqual(exactSF, after) {
				t.Fatal("restored precision changed through gzip SF")
			}
		}
	}
}

func TestDOCXSizeProjectionBoundsAndStrictAdmission(t *testing.T) {
	for _, size := range []float64{.25, .24999, .1, 10.24999, 10.25, 10.25001, 6.959646, 2147483647.49, 2147483648} {
		d := &Document{Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "EXACT", FontSize: size}}}}}
		for _, allow := range []bool{false, true} {
			var out bytes.Buffer
			err := Write(context.Background(), &out, d, DOCX, WriteOptions{AllowLossy: allow})
			valid := allow && size >= .25 && size < 2147483647.75
			if !valid {
				if !errors.Is(err, ErrUnsupported) || out.Len() != 0 {
					t.Fatal("unrepresentable/strict size accepted", size, allow, err)
				}
				continue
			}
			if err != nil {
				t.Fatal(size, err)
			}
			got, err := ReadDOCX(context.Background(), &out)
			if err != nil {
				t.Fatal(size, err)
			}
			if got.Blocks[0].Runs[0].FontSize != size {
				t.Fatal("original precision changed", size, got.Blocks[0].Runs[0].FontSize)
			}
		}
	}
}

func TestDOCXSizePrecisionRejectsContradictions(t *testing.T) {
	d := &Document{Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "VISIBLE", FontSize: 10.25}}}}}
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, DOCX, WriteOptions{AllowLossy: true}); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(string) string{
		func(s string) string { return strings.Replace(s, `md:fontSize="10.25"`, `md:fontSize="NaN"`, 1) },
		func(s string) string { return strings.Replace(s, `md:fontSize="10.25"`, `md:fontSize=""`, 1) },
		func(s string) string {
			return strings.Replace(s, `<w:sz w:val="21"/>`, `<x:sz xmlns:x="urn:lookalike" w:val="21"/>`, 1)
		},
		func(s string) string {
			return strings.Replace(s, `<w:sz w:val="21"/>`, `<w:sz xmlns:x="urn:lookalike" x:val="21"/>`, 1)
		},
		func(s string) string { return strings.Replace(s, `md:fontSize="10.25"`, `md:fontSize="10"`, 1) },
		func(s string) string { return strings.Replace(s, `md:fontSize="10.25"`, `md:fontSize="9.25"`, 1) },
		func(s string) string { return strings.Replace(s, `md:fontSize="10.25"`, `md:fontSize="0.1"`, 1) },
		func(s string) string { return strings.Replace(s, `<w:sz w:val="21"/>`, `<w:sz w:val="20"/>`, 1) },
		func(s string) string { return strings.Replace(s, `<w:szCs w:val="21"/>`, ``, 1) },
		func(s string) string {
			return strings.Replace(s, `md:fontSize="10.25"`, `md:fontSize="10.25" md:textImplicit="s"`, 1)
		},
	} {
		bad := rewriteListDOCX(t, out.Bytes(), change)
		if _, err := ReadDOCX(context.Background(), bytes.NewReader(bad)); err == nil {
			t.Fatal("contradictory precision accepted")
		}
	}

	plain := rewriteListDOCX(t, out.Bytes(), func(s string) string { return strings.Replace(s, ` md:fontSize="10.25"`, "", 1) })
	got, err := ReadDOCX(context.Background(), bytes.NewReader(plain))
	if err != nil || got.Blocks[0].Runs[0].FontSize != 10.5 {
		t.Fatal("standard output size is not authoritative", err)
	}
}

func TestXMLDuplicateExpandedAttributesReject(t *testing.T) {
	for _, input := range []string{`<r a="1" a="2"/>`, `<r xmlns:m="urn:x" xmlns:n="urn:x" m:a="1" n:a="2"/>`} {
		if _, err := parseXML(context.Background(), []byte(input)); err == nil {
			t.Fatal("duplicate expanded XML attribute accepted")
		}
	}
	if _, err := parseXML(context.Background(), []byte(`<r xmlns:m="urn:x" xmlns:n="urn:y" m:a="1" n:a="2"/>`)); err != nil {
		t.Fatal("distinct namespaces conflated", err)
	}
}

func TestDOCXFractionalComplexScriptSize(t *testing.T) {
	d := &Document{Blocks: []Block{{Kind: Paragraph, Direction: "rtl", Runs: []Run{{Text: "مرحبا", FontSize: 13.333, ComplexScript: true, Direction: "rtl", Language: "ar", FontFamily: "DejaVu Sans", Bold: true}}}}}
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, DOCX, WriteOptions{AllowLossy: true}); err != nil {
		t.Fatal(err)
	}
	got, err := ReadDOCX(context.Background(), &out)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(d.Blocks, got.Blocks) {
		t.Fatalf("complex size intent changed: %+v", got.Blocks)
	}
}

const themeColorPalette = `<a:dk1><a:sysClr val="windowText" lastClr="102030"/></a:dk1><a:lt1><a:sysClr val="window" lastClr="F0E0D0"/></a:lt1><a:dk2><a:srgbClr val="204060"/></a:dk2><a:lt2><a:srgbClr val="E0C0A0"/></a:lt2><a:accent1><a:srgbClr val="4F81BD"/></a:accent1><a:accent2><a:srgbClr val="C0504D"/></a:accent2><a:accent3><a:srgbClr val="FF0000"/></a:accent3><a:accent4><a:srgbClr val="808080"/></a:accent4><a:accent5><a:srgbClr val="FFFFFF"/></a:accent5><a:accent6><a:srgbClr val="000000"/></a:accent6><a:hlink><a:srgbClr val="0000FF"/></a:hlink><a:folHlink><a:srgbClr val="800080"/></a:folHlink>`

func themeColorDOCX(t testing.TB, body, palette, settings, styles string) []byte {
	t.Helper()
	parts := map[string]string{
		"[Content_Types].xml":          `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/></Types>`,
		"_rels/.rels":                  `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="main" Type="` + relNS + `/officeDocument" Target="word/document.xml"/></Relationships>`,
		"word/document.xml":            `<w:document xmlns:w="` + wordNS + `"><w:body>` + body + `</w:body></w:document>`,
		"word/_rels/document.xml.rels": `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="theme" Type="` + relNS + `/theme" Target="theme/theme1.xml"/><Relationship Id="settings" Type="` + relNS + `/settings" Target="settings.xml"/><Relationship Id="styles" Type="` + relNS + `/styles" Target="styles.xml"/></Relationships>`,
		"word/theme/theme1.xml":        `<a:theme xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"><a:themeElements><a:clrScheme name="Independent">` + palette + `</a:clrScheme></a:themeElements></a:theme>`,
		"word/settings.xml":            `<w:settings xmlns:w="` + wordNS + `">` + settings + `</w:settings>`,
		"word/styles.xml":              `<w:styles xmlns:w="` + wordNS + `">` + styles + `</w:styles>`,
	}
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	for name, value := range parts {
		w, err := z.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = w.Write([]byte(value)); err != nil {
			t.Fatal(err)
		}
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
func themeColorRun(attrs string) string {
	return `<w:p><w:r><w:rPr><w:color ` + attrs + `/></w:rPr><w:t>VISIBLE</w:t></w:r></w:p>`
}
func readThemeColor(t *testing.T, attrs, palette, mapping string) *Document {
	t.Helper()
	d, err := ReadDOCX(context.Background(), bytes.NewReader(themeColorDOCX(t, themeColorRun(attrs), palette, mapping, "")))
	if err != nil {
		t.Fatal(err)
	}
	return d
}
func TestDOCXThemeColorCalculationAndMapping(t *testing.T) {
	for _, tc := range []struct{ name, attrs, mapping, want string }{
		{"theme beats stale cache", `w:themeColor="accent1" w:val="FF0000"`, "", "#4F81BD"},
		{"tint without cache", `w:themeColor="accent1" w:themeTint="99"`, "", "#95B3D7"},
		{"shade red", `w:themeColor="accent3" w:themeShade="80"`, "", "#800000"},
		{"tint red", `w:themeColor="accent3" w:themeTint="80"`, "", "#FF7F7F"},
		{"tint wins", `w:themeColor="accent3" w:themeTint="80" w:themeShade="00"`, "", "#FF7F7F"},
		{"shade zero", `w:themeColor="accent1" w:themeShade="00"`, "", "#000000"},
		{"tint zero", `w:themeColor="accent1" w:themeTint="00"`, "", "#FFFFFF"},
		{"shade identity", `w:themeColor="accent1" w:themeShade="ff"`, "", "#4F81BD"},
		{"tint white", `w:themeColor="accent5" w:themeTint="80"`, "", "#FFFFFF"},
		{"shade black", `w:themeColor="accent6" w:themeShade="80"`, "", "#000000"},
		{"gray shade", `w:themeColor="accent4" w:themeShade="80"`, "", "#404040"},
		{"dark alias", `w:themeColor="dark2"`, "", "#204060"},
		{"light alias", `w:themeColor="light2"`, "", "#E0C0A0"},
		{"text mapping", `w:themeColor="text1"`, `<w:clrSchemeMapping w:t1="accent3"/>`, "#FF0000"},
		{"background mapping", `w:themeColor="background2"`, `<w:clrSchemeMapping w:bg2="accent4"/>`, "#808080"},
		{"direct dark unaffected", `w:themeColor="dark1"`, `<w:clrSchemeMapping w:t1="accent3"/>`, "#102030"},
		{"swap not recursive", `w:themeColor="accent1"`, `<w:clrSchemeMapping w:accent1="accent3" w:accent3="accent1"/>`, "#FF0000"},
		{"hyperlink slot", `w:themeColor="hyperlink"`, "", "#0000FF"},
		{"followed slot", `w:themeColor="followedHyperlink"`, "", "#800080"},
		{"inactive none", `w:themeColor="none" w:themeTint="00" w:val="123456"`, "", "#123456"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := readThemeColor(t, tc.attrs, themeColorPalette, tc.mapping)
			if got := d.Blocks[0].Runs[0].Color; got != tc.want {
				t.Fatalf("color %s, want %s", got, tc.want)
			}
			if strings.Contains(tc.attrs, `="none"`) {
				if !hasDiagnostic(d, "color_theme_unused") {
					t.Fatal("unused modifier not disclosed")
				}
			} else if !hasDiagnostic(d, "color_theme") || hasDiagnostic(d, "color_theme_cache") {
				t.Fatal("resolved theme evidence incorrect")
			}
			if tc.name == "theme beats stale cache" && !hasDiagnostic(d, "color_theme_cache_mismatch") {
				t.Fatal("stale RGB cache replacement not disclosed")
			}
			assertRecoverySF(t, d)
			var out bytes.Buffer
			if err := Write(context.Background(), &out, d, DOCX, WriteOptions{AllowLossy: true}); err != nil {
				t.Fatal(err)
			}
			back, err := ReadDOCX(context.Background(), &out)
			if err != nil || back.Blocks[0].Runs[0].Color != tc.want {
				t.Fatalf("DOCX resolved color round trip %v", err)
			}
		})
	}
}
func TestDOCXThemeColorInheritanceAndCache(t *testing.T) {
	styles := `<w:docDefaults><w:rPrDefault><w:rPr><w:color w:themeColor="accent3" w:themeShade="80"/></w:rPr></w:rPrDefault></w:docDefaults><w:style w:type="paragraph" w:styleId="P"><w:rPr><w:color w:themeColor="accent1" w:themeTint="99"/></w:rPr></w:style><w:style w:type="character" w:styleId="C"><w:rPr><w:color w:themeColor="hyperlink"/></w:rPr></w:style>`
	body := `<w:p><w:r><w:t>DEFAULT</w:t></w:r></w:p><w:p><w:pPr><w:pStyle w:val="P"/></w:pPr><w:r><w:t>PARAGRAPH</w:t></w:r><w:r><w:rPr><w:rStyle w:val="C"/></w:rPr><w:t>CHARACTER</w:t></w:r><w:r><w:rPr><w:rStyle w:val="C"/><w:color w:val="auto"/></w:rPr><w:t>DIRECT</w:t></w:r></w:p>`
	d, err := ReadDOCX(context.Background(), bytes.NewReader(themeColorDOCX(t, body, themeColorPalette, "", styles)))
	if err != nil {
		t.Fatal(err)
	}
	rr := recoveryRuns(d)
	for i, want := range []string{"#800000", "#95B3D7", "#0000FF", "auto"} {
		if rr[i].Color != want {
			t.Fatalf("inheritance %+v", rr)
		}
	}
	for _, palette := range []string{"", `<a:accent1><a:prstClr val="red"/></a:accent1>`, `<a:accent1><a:srgbClr val="FF0000"><a:lumMod val="50000"/></a:srgbClr></a:accent1>`} {
		d = readThemeColor(t, `w:themeColor="accent1" w:themeTint="00" w:val="123456"`, palette, "")
		if d.Blocks[0].Runs[0].Color != "#123456" || !hasDiagnostic(d, "color_theme_cache") {
			t.Fatal("cache transformed twice or unsupported theme hidden")
		}
	}
	d = readThemeColor(t, `w:themeColor="text1"`, themeColorPalette, "")
	if d.Blocks[0].Runs[0].Color != "#102030" || !hasDiagnostic(d, "color_system") {
		t.Fatal("system lastClr not disclosed")
	}
}
func TestDOCXThemeColorInvalid(t *testing.T) {
	for _, attrs := range []string{`w:themeColor="unknown" w:val="123456"`, `w:themeColor="accent1" w:themeTint="100"`, `w:themeColor="accent1" w:themeShade="-1"`, `w:themeColor="accent1" w:themeTint="GG"`, `w:themeColor="accent1" w:themeTint="80" w:themeShade="x"`, `w:val="12"`, `w:themeColor="accent1" w:val="GGGGGG"`, `w:themeColor="accent1" w:themeTint="+1"`} {
		if _, err := ReadDOCX(context.Background(), bytes.NewReader(themeColorDOCX(t, themeColorRun(attrs), themeColorPalette, "", ""))); err == nil {
			t.Fatalf("invalid color accepted %s", attrs)
		}
	}
	for _, mapping := range []string{`<w:clrSchemeMapping w:t1="text1"/>`, `<w:clrSchemeMapping w:t1="not-a-slot"/>`, `<w:clrSchemeMapping/><w:clrSchemeMapping/>`} {
		if _, err := ReadDOCX(context.Background(), bytes.NewReader(themeColorDOCX(t, themeColorRun(`w:themeColor="text1"`), themeColorPalette, mapping, ""))); err == nil {
			t.Fatal("invalid mapping accepted")
		}
	}
	for _, cache := range []string{"", `w:val="auto"`} {
		if _, err := ReadDOCX(context.Background(), bytes.NewReader(themeColorDOCX(t, themeColorRun(`w:themeColor="accent1" `+cache), "", "", ""))); !errors.Is(err, ErrUnsupported) {
			t.Fatalf("missing theme/cache %v", err)
		}
	}
}
func FuzzDOCXThemeLuminance(f *testing.F) {
	f.Add(uint32(0x4F81BD), byte(0x99), true)
	f.Add(uint32(0xC0504D), byte(0xBF), false)
	f.Fuzz(func(t *testing.T, color uint32, modifier byte, tint bool) {
		color &= 0xFFFFFF
		input := fmt.Sprintf("%06X", color)
		rgb, err := docxRGB(input)
		if err != nil {
			t.Fatal(err)
		}
		transformed := docxLuminance(rgb, float64(modifier)/255, tint)
		for _, v := range transformed {
			if !finite(v) || v < 0 || v > 1 {
				t.Fatal("invalid color")
			}
		}
		factor := float64(modifier) / 255
		luminance := func(c [3]float64) float64 { return (min(c[0], c[1], c[2]) + max(c[0], c[1], c[2])) / 2 }
		want := luminance(rgb) * factor
		if tint {
			want += 1 - factor
		}
		if math.Abs(luminance(transformed)-want) > 1e-12 {
			t.Fatal("HSL luminance mismatch")
		}
		if modifier == 255 && docxHex(transformed) != input {
			t.Fatal("identity altered RGB")
		}
	})
}

func FuzzDOCXThemeColorPackage(f *testing.F) {
	f.Add(uint32(0x4F81BD), byte(0x99), true)
	f.Add(uint32(0xC0504D), byte(0xBF), false)
	f.Fuzz(func(t *testing.T, color uint32, modifier byte, tint bool) {
		color &= 0xFFFFFF
		palette := fmt.Sprintf(`<a:accent1><a:srgbClr val="%06X"/></a:accent1>`, color)
		attr := "themeShade"
		if tint {
			attr = "themeTint"
		}
		props := fmt.Sprintf(`w:themeColor="accent1" w:%s="%02X" w:val="123456"`, attr, modifier)
		d := readThemeColor(t, props, palette, "")
		if d.Text() != "VISIBLE" || !hasDiagnostic(d, "color_theme") {
			t.Fatal("visible text/binding diagnostics lost")
		}
		assertRecoverySF(t, d)
		var output bytes.Buffer
		if err := Write(context.Background(), &output, d, DOCX, WriteOptions{AllowLossy: true}); err != nil {
			t.Fatal(err)
		}
		back, err := ReadDOCX(context.Background(), &output)
		if err != nil || back.Blocks[0].Runs[0].Color != d.Blocks[0].Runs[0].Color {
			t.Fatalf("resolved color round trip changed: %v", err)
		}
	})
}
