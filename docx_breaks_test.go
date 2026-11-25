package manifold

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
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
	for i, want := range []bool{false, true, false, true} {
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
	d := &Document{Blocks: []Block{{Kind: PageBreak}, {Kind: PageBreak}, mergeParagraph("VISIBLE")}}
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
