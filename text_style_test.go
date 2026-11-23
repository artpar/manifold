package manydoc

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"math"
	"os"
	"strings"
	"testing"

	"golang.org/x/image/font/gofont/goregular"
)

func textStyleDocument(t testing.TB) *Document {
	t.Helper()
	b, err := os.ReadFile("examples/text-style/document.sf")
	if err != nil {
		t.Fatal(err)
	}
	d, err := Read(context.Background(), bytes.NewReader(b), SF, ReadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return d
}
func TestTextStyleSFDOCXAndPDFFonts(t *testing.T) {
	d := textStyleDocument(t)
	checkParagraphRoundtrip(t, d)
	before := canonicalSF(t, d)
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, PDF, WriteOptions{PDFFonts: []FontFace{{Family: "App Sans", Data: goregular.TTF}}}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, canonicalSF(t, d)) {
		t.Fatal("font layout mutated source")
	}
	got, err := ReadPDF(context.Background(), &out, ReadOptions{PDFSkipInference: true})
	if err != nil {
		t.Fatal(err)
	}
	if got.Metadata.PageCount != 4 {
		t.Fatalf("rich style pages=%d", got.Metadata.PageCount)
	}
	for token, page := range map[string]int{"COLOR_AND_SIZE": 1, "LARGE_LIST": 2, "CELL_00": 3, "CELL_05": 4, "CUSTOM_FACE": 4, "STYLED_NOTE": 1} {
		checkTokenPage(t, got, token, page)
	}
}
func TestPDFExplicitFontMappingAndMissingFaces(t *testing.T) {
	d := &Document{Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "EXPLICIT", FontFamily: "Requested Sans", FontSize: 14}}}}}
	for name, opts := range map[string]WriteOptions{
		"missing":        {},
		"missing_target": {PDFFontMap: map[string]string{"Requested Sans": "Not Installed"}},
		"cycle":          {PDFFontMap: map[string]string{"Requested Sans": "Loop", "Loop": "Requested Sans"}},
		"missing_style":  {PDFFonts: []FontFace{{Family: "Requested Sans", Italic: true, Data: goregular.TTF}}},
	} {
		t.Run(name, func(t *testing.T) {
			var out bytes.Buffer
			if err := Write(context.Background(), &out, d, PDF, opts); err == nil || out.Len() != 0 {
				t.Fatal("unavailable font silently substituted/partially written")
			}
		})
	}
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, PDF, WriteOptions{PDFFontMap: map[string]string{"Requested Sans": "Go Mono"}}); err != nil {
		t.Fatal(err)
	}
	if d.Blocks[0].Runs[0].FontFamily != "Requested Sans" {
		t.Fatal("mapping changed requested family")
	}
}
func TestTextStyleInvalidAndUnrepresentableAtomic(t *testing.T) {
	for _, run := range []Run{{FontSize: -1}, {FontSize: math.Inf(1)}, {FontSize: math.NaN()}, {Color: "red"}, {Color: "#ZZ0000"}, {FontFamily: " family "}} {
		run.Text = "VISIBLE"
		d := &Document{Blocks: []Block{{Kind: Paragraph, Runs: []Run{run}}}}
		for _, format := range []Format{SF, DOCX, PDF} {
			var out bytes.Buffer
			if err := Write(context.Background(), &out, d, format, WriteOptions{}); err == nil || out.Len() != 0 {
				t.Fatal("invalid style accepted")
			}
		}
	}
	d := &Document{Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "fraction", FontSize: 10.25}}}}}
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, DOCX, WriteOptions{}); !errors.Is(err, ErrUnsupported) || out.Len() != 0 {
		t.Fatal("DOCX rounded unrepresentable size")
	}
	if err := Write(context.Background(), &out, d, PDF, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
}
func TestDOCXTextDefaultsStyleAndOverrides(t *testing.T) {
	styles := `<w:docDefaults><w:rPrDefault><w:rPr><w:rFonts w:ascii="Go" w:hAnsi="Go"/><w:sz w:val="22"/><w:color w:val="112233"/></w:rPr></w:rPrDefault></w:docDefaults><w:style w:type="paragraph" w:default="1" w:styleId="Normal"><w:rPr><w:sz w:val="24"/></w:rPr></w:style><w:style w:type="paragraph" w:styleId="Base"><w:basedOn w:val="Normal"/><w:rPr><w:rFonts w:ascii="Go Mono" w:hAnsi="Go Mono"/></w:rPr></w:style><w:style w:type="paragraph" w:styleId="Derived"><w:basedOn w:val="Base"/><w:rPr><w:color w:val="445566"/></w:rPr></w:style><w:style w:type="character" w:styleId="Character"><w:rPr><w:b/><w:sz w:val="36"/></w:rPr></w:style>`
	body := `<w:p><w:r><w:t>DEFAULT</w:t></w:r></w:p><w:p><w:pPr><w:pStyle w:val="Derived"/></w:pPr><w:r><w:rPr><w:rStyle w:val="Character"/><w:color w:val="auto"/></w:rPr><w:t>OVERRIDE</w:t></w:r></w:p><w:sectPr><w:pgSz w:w="11906" w:h="16838"/><w:pgMar w:top="1080" w:right="1080" w:bottom="1080" w:left="1080"/></w:sectPr>`
	d, err := ReadDOCX(context.Background(), bytes.NewReader(numberedDOCX(t, body, "", styles)))
	if err != nil {
		t.Fatal(err)
	}
	a, b := d.Blocks[0].Runs[0], d.Blocks[1].Runs[0]
	if a.FontFamily != "Go" || a.FontSize != 12 || a.Color != "#112233" || b.FontFamily != "Go Mono" || b.FontSize != 18 || b.Color != "auto" || !b.Bold {
		t.Fatalf("style resolution: %+v %+v", a, b)
	}
	if len(d.Diagnostics) != 0 {
		t.Fatalf("supported properties diagnosed: %v", d.Diagnostics)
	}
	checkParagraphRoundtrip(t, d)
}
func TestDOCXTextImplicitMetadataCannotHideProperties(t *testing.T) {
	var out bytes.Buffer
	if err := Write(context.Background(), &out, &Document{Blocks: []Block{mergeParagraph("VISIBLE")}}, DOCX, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	changed := rewriteListDOCX(t, out.Bytes(), func(xml string) string { return strings.Replace(xml, `<w:rPr>`, `<w:rPr><w:sz w:val="28"/>`, 1) })
	if _, err := ReadDOCX(context.Background(), bytes.NewReader(changed)); err == nil {
		t.Fatal("implicit metadata hid modified size")
	}
}
func TestDOCXTextStyleTogglesAndDirectCancellation(t *testing.T) {
	styles := `<w:style w:type="paragraph" w:styleId="Base"><w:rPr><w:b/><w:i/></w:rPr></w:style><w:style w:type="paragraph" w:styleId="Derived"><w:basedOn w:val="Base"/><w:rPr><w:b/><w:i w:val="false"/></w:rPr></w:style><w:style w:type="character" w:styleId="Character"><w:rPr><w:b/><w:i/></w:rPr></w:style>`
	body := `<w:p><w:pPr><w:pStyle w:val="Derived"/></w:pPr><w:r><w:t>PARAGRAPH</w:t></w:r><w:r><w:rPr><w:rStyle w:val="Character"/></w:rPr><w:t>CHARACTER</w:t></w:r><w:r><w:rPr><w:rStyle w:val="Character"/><w:b w:val="false"/><w:i/></w:rPr><w:t>DIRECT</w:t></w:r></w:p>`
	d, err := ReadDOCX(context.Background(), bytes.NewReader(numberedDOCX(t, body, "", styles)))
	if err != nil {
		t.Fatal(err)
	}
	runs := d.Blocks[0].Runs
	if len(runs) != 3 || runs[0].Bold || !runs[0].Italic || !runs[1].Bold || runs[1].Italic || runs[2].Bold || !runs[2].Italic {
		t.Fatalf("toggle cascade: %+v", runs)
	}
}
func TestCustomFontValidationAtomic(t *testing.T) {
	missing := append([]byte(nil), goregular.TTF...)
	n := int(binary.BigEndian.Uint16(missing[4:]))
	for i := 0; i < n; i++ {
		p := missing[12+16*i:]
		if string(p[:4]) == "OS/2" {
			copy(p[:4], "xxxx")
		}
	}
	for _, data := range [][]byte{nil, []byte("invalid"), goregular.TTF[:100], missing} {
		var out bytes.Buffer
		if err := Write(context.Background(), &out, &Document{Blocks: []Block{mergeParagraph("CONTENT")}}, PDF, WriteOptions{PDFFonts: []FontFace{{Family: "Custom", Data: data}}}); err == nil || out.Len() != 0 {
			t.Fatal("bad font registered or output leaked")
		}
	}
}
func FuzzCustomPDFFont(f *testing.F) {
	f.Add(goregular.TTF)
	f.Add([]byte("invalid"))
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 2<<20 {
			t.Skip()
		}
		var out bytes.Buffer
		d := &Document{Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "ABC " + string([]rune{0x2007, 0x2011}), FontFamily: "Custom", FontSize: 14}}}}}
		err := Write(context.Background(), &out, d, PDF, WriteOptions{PDFFonts: []FontFace{{Family: "Custom", Data: data}}})
		if err != nil && out.Len() != 0 {
			t.Fatal("partial font output")
		}
	})
}
func TestExplicitSmallFontControlsLineHeightIncludingTrailingBreak(t *testing.T) {
	r := textTestRenderer()
	lines, err := r.lines([]Run{{Text: "FIRST\nSECOND\n", FontSize: 6}}, 200, 11)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 3 {
		t.Fatal("lost trailing empty line")
	}
	for _, line := range lines {
		if math.Abs(pdfLineHeight(line, 11, 1.4)-8.4) > .001 || math.Abs(pdfLineSize(line, 11)-6) > .001 {
			t.Fatal("explicit font inherited unrelated default metrics")
		}
	}
}
func FuzzRichTextStyles(f *testing.F) {
	f.Add(uint8(12), uint8(18), true, false)
	f.Add(uint8(0), uint8(8), false, true)
	f.Fuzz(func(t *testing.T, a, b uint8, mono, note bool) {
		family := "Go"
		if mono {
			family = "Go Mono"
		}
		body := Block{Kind: Paragraph, Runs: []Run{{Text: "STYLE_ONE ", FontFamily: family, FontSize: 6 + float64(a%38)/2, Color: "#225588"}, {Text: "STYLE_TWO", FontSize: 6 + float64(b%38)/2, Color: "#881122", Bold: true}}}
		d := &Document{Layout: Layout{Width: 400, Height: 300, Margin: 30}, Blocks: []Block{body}}
		if note {
			d.Blocks = []Block{{Kind: Paragraph, Runs: []Run{{Text: "REFERENCE"}, noteRun(Footnote, "n")}}}
			d.Notes = []Note{{Kind: Footnote, ID: "n", Blocks: []Block{cellTable([]Block{body})}}}
		}
		checkParagraphRoundtrip(t, d)
		before := canonicalSF(t, d)
		got, _ := renderPageBreakDocument(t, d)
		if !bytes.Equal(before, canonicalSF(t, d)) {
			t.Fatal("style layout mutated source")
		}
		text := strings.Join(strings.Fields(got.Text()), "")
		if strings.Count(text, "STYLE_ONE") != 1 || strings.Count(text, "STYLE_TWO") != 1 {
			t.Fatal("style wrapping lost or duplicated content")
		}
		for _, block := range got.Blocks {
			for _, run := range block.Runs {
				if run.Source == nil || run.Source.Quad == nil || strings.TrimSpace(run.Text) == "" {
					continue
				}
				x0, y0, x1, y1 := quadBounds(*run.Source.Quad)
				if x0 < 29.95 || y0 < 29.95 || x1 > 370.05 || y1 > 270.05 {
					t.Fatalf("rich style outside page: %+v", run)
				}
			}
		}
	})
}
func TestSFTextStyleRejectsInvalidAttributeTypes(t *testing.T) {
	for _, attrs := range []string{`{"fontSize":null}`, `{"fontSize":"12pt"}`, `{"fontFamily":null}`, `{"color":null}`, `{"unknown":1}`} {
		sf, err := ToStructured(textStyleDocument(t))
		if err != nil {
			t.Fatal(err)
		}
		sf.Content[0].Content[0].Marks = []StructuredMark{{Type: "textStyle", Attrs: json.RawMessage(attrs)}}
		if _, err := FromStructured(sf); err == nil {
			t.Fatalf("accepted invalid style attributes %s", attrs)
		}
	}
}
func TestPDFTextStylePrecisionIsExplicit(t *testing.T) {
	for _, size := range []float64{.001, 12.001} {
		var out bytes.Buffer
		d := &Document{Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "VISIBLE", FontSize: size}}}}}
		if err := Write(context.Background(), &out, d, PDF, WriteOptions{}); !errors.Is(err, ErrUnsupported) || out.Len() != 0 {
			t.Fatalf("unrepresentable font size %v", size)
		}
	}
}
func TestSuppliedNotoFaceUsesItsOwnMetricsAndOutlines(t *testing.T) {
	data, err := os.ReadFile("testdata/assets/NotoSans-Regular.ttf")
	if err != nil {
		t.Fatal(err)
	}
	original := append([]byte(nil), data...)
	d := &Document{Layout: Layout{Width: 400, Height: 300, Margin: 30}, Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "Noto Ω Привет", FontFamily: "Noto Sans", FontSize: 13, Color: "#224466"}}}}}
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, PDF, WriteOptions{PDFFonts: []FontFace{{Family: "Noto Sans", Data: data}}}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, original) {
		t.Fatal("subsetting mutated caller font data")
	}
	got, err := ReadPDF(context.Background(), &out, ReadOptions{PDFSkipInference: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got.Text(), "Noto Ω Привет") {
		t.Fatal("custom font lost Unicode")
	}
	for _, b := range got.Blocks {
		for _, r := range b.Runs {
			if r.Source == nil || r.Source.Quad == nil {
				continue
			}
			x0, y0, x1, y1 := quadBounds(*r.Source.Quad)
			if x0 < 29.95 || y0 < 29.95 || x1 > 370.05 || y1 > 270.05 {
				t.Fatal("custom metrics escaped page")
			}
		}
	}
}
func BenchmarkPDFUsedFontRegistration(b *testing.B) {
	d := &Document{Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "Only the regular default face is used."}}}}}
	b.ReportAllocs()
	size := 0
	for i := 0; i < b.N; i++ {
		var out bytes.Buffer
		if err := Write(context.Background(), &out, d, PDF, WriteOptions{}); err != nil {
			b.Fatal(err)
		}
		size = out.Len()
	}
	b.ReportMetric(float64(size), "bytes/pdf")
}
func TestPDFFontFamilyLookupUsesUnicodeCaseFolding(t *testing.T) {
	var out bytes.Buffer
	d := &Document{Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "FONT_NAME", FontFamily: "ΣΊΣΥΦΟΣ"}}}}}
	if err := Write(context.Background(), &out, d, PDF, WriteOptions{PDFFonts: []FontFace{{Family: "Σίσυφος", Data: goregular.TTF}}}); err != nil {
		t.Fatal(err)
	}
}
