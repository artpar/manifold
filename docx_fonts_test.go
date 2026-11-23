package manydoc

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

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
	// Equal effective choices must not create spurious SF boundaries or losses.
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
	if run.FontFamily != "Arabic Major" || !run.Bold || run.FontSize != 15 || !hasDiagnostic(d, "run_direction") {
		t.Fatalf("complex theme %+v", run)
	}
	// With no themeFontLang, the region node supplies the default family.
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
	// Unknown unused theme slots cannot prevent extraction of unrelated text.
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
