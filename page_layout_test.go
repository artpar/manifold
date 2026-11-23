package manydoc

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"
)

func pagePoint(v float64) *float64 { return &v }
func pageTestDoc(l Layout) *Document {
	return &Document{Layout: l, Headers: []Block{{Kind: Paragraph, Runs: []Run{{Text: "HEADER", FontSize: 8}}}}, Footers: []Block{{Kind: Paragraph, Runs: []Run{{Text: "FOOTER", FontSize: 8}}}}, Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "BODY"}}}, {Kind: PageBreak}, {Kind: Paragraph, Runs: []Run{{Text: "BODY TWO"}}}}}
}
func TestPageLayoutSFAndDOCX(t *testing.T) {
	cases := []Layout{
		{Width: 360, Height: 420, Margin: 17, Margins: &PageMargins{60, 44, 55, 22}, HeaderDistance: pagePoint(12), FooterDistance: pagePoint(14)},
		{Width: 200, Height: 240, Margin: 42, Margins: &PageMargins{}, HeaderDistance: pagePoint(0), FooterDistance: pagePoint(0)},
		{Width: 300, Height: 400, Margins: &PageMargins{30, 30, 30, 30}},
		{Width: 300, Height: 400, Margin: 30, HeaderDistance: pagePoint(0)},
		{Width: 360, Height: 420, Margins: &PageMargins{-60, 44, -55, 22}, HeaderDistance: pagePoint(12), FooterDistance: pagePoint(14)},
	}
	for i, l := range cases {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			d := pageTestDoc(l)
			before := canonicalSF(t, d)
			for _, f := range []Format{SF, JSON, DOCX} {
				var buf bytes.Buffer
				if err := Write(context.Background(), &buf, d, f, WriteOptions{}); err != nil {
					t.Fatal(f, err)
				}
				back, err := Read(context.Background(), bytes.NewReader(buf.Bytes()), f, ReadOptions{})
				if err != nil {
					t.Fatal(f, err)
				}
				if !bytes.Equal(before, canonicalSF(t, back)) {
					t.Fatalf("%s lost page/settings/content semantics", f)
				}
			}
			if !bytes.Equal(before, canonicalSF(t, d)) {
				t.Fatal("source changed")
			}
		})
	}
}
func TestPageLayoutBridgeOwnership(t *testing.T) {
	d := pageTestDoc(Layout{Width: 300, Height: 400, Margins: &PageMargins{40, 20, 30, 25}, HeaderDistance: pagePoint(10), FooterDistance: pagePoint(12)})
	s, err := ToStructured(d)
	if err != nil {
		t.Fatal(err)
	}
	back, err := FromStructured(s)
	if err != nil {
		t.Fatal(err)
	}
	s.Attrs.Layout.Margins.Left = 100
	*s.Attrs.Layout.HeaderDistance = 33
	*s.Attrs.Layout.FooterDistance = 44
	if d.Layout.Margins.Left != 25 || back.Layout.Margins.Left != 25 || *d.Layout.HeaderDistance != 10 || *back.Layout.FooterDistance != 12 {
		t.Fatal("page settings share mutable ownership")
	}
}
func TestPageLayoutPDFPositions(t *testing.T) {
	d := pageTestDoc(Layout{Width: 360, Height: 420, Margin: 17, Margins: &PageMargins{60, 44, 55, 22}, HeaderDistance: pagePoint(12), FooterDistance: pagePoint(14)})
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, PDF, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	back, err := ReadPDF(context.Background(), bytes.NewReader(out.Bytes()), ReadOptions{PDFSkipInference: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(back.Pages) != 2 {
		t.Fatal("page count")
	}
	counts := map[string]int{}
	for _, b := range back.Blocks {
		if b.Source == nil || b.Source.Quad == nil {
			continue
		}
		var text strings.Builder
		for _, r := range b.Runs {
			text.WriteString(r.Text)
		}
		s := text.String()
		if s == "" {
			continue
		}
		counts[s]++
		x, y, x1, y1 := quadBounds(*b.Source.Quad)
		if x < 21.99 || x1 > 316.01 {
			t.Fatalf("horizontal body/running bounds %q %.3f..%.3f", s, x, x1)
		}
		if strings.HasPrefix(s, "BODY") && (y < 59.99 || y1 > 365.01) {
			t.Fatalf("body bounds %q %.3f..%.3f", s, y, y1)
		}
		if s == "HEADER" && (y < 11.9 || y1 > 23) {
			t.Fatal("header edge distance")
		}
		if s == "FOOTER" && (y < 395.9 || y1 > 406.5) {
			t.Fatalf("footer bottom distance %.3f..%.3f", y, y1)
		}
	}
	if counts["HEADER"] != 2 || counts["FOOTER"] != 2 || counts["BODY"] != 1 || counts["BODY TWO"] != 1 {
		t.Fatalf("content %v", counts)
	}
	assertRecoverySF(t, back)
}
func TestPageLayoutInvalidAndNegativePDF(t *testing.T) {
	for _, l := range []Layout{{Margins: &PageMargins{0, -1, 0, 0}}, {Margins: &PageMargins{0, 0, 0, -1}}, {Margins: &PageMargins{300, 0, 600, 0}}, {Margins: &PageMargins{0, 500, 0, 500}}, {HeaderDistance: pagePoint(math.NaN())}, {FooterDistance: pagePoint(math.Inf(1))}, {HeaderDistance: pagePoint(-1)}, {Margins: &PageMargins{math.Inf(1), 0, 0, 0}}} {
		for _, f := range []Format{SF, DOCX, PDF} {
			var out bytes.Buffer
			if err := Write(context.Background(), &out, pageTestDoc(l), f, WriteOptions{}); err == nil || out.Len() != 0 {
				t.Fatal("invalid layout accepted/wrote output", l, f, err)
			}
		}
	}
	d := pageTestDoc(Layout{Width: 300, Height: 400, Margins: &PageMargins{-10, 20, 30, 25}})
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, PDF, WriteOptions{}); !errors.Is(err, ErrUnsupported) || out.Len() != 0 {
		t.Fatal("negative running overlap must reject atomically until qualified", err)
	}
	d.Headers, d.Footers = nil, nil
	if err := Write(context.Background(), &out, d, PDF, WriteOptions{}); err != nil {
		t.Fatal("signed offsets without overlapping running content", err)
	}
}
func TestDOCXPageSettingsExternalAndMetadata(t *testing.T) {
	d := pageTestDoc(Layout{Width: 360, Height: 420, Margin: 30})
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, DOCX, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	plain := rewriteListDOCX(t, out.Bytes(), func(s string) string { return strings.ReplaceAll(s, ` md:layoutImplicit="hf"`, "") })
	back, err := ReadDOCX(context.Background(), bytes.NewReader(plain))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(back.Layout.HeaderDistance, pagePoint(18)) || !reflect.DeepEqual(back.Layout.FooterDistance, pagePoint(18)) {
		t.Fatal("external physical distances not recovered")
	}
	for _, change := range []func(string) string{
		func(s string) string { return strings.Replace(s, `w:header="360"`, `w:header="361"`, 1) },
		func(s string) string { return strings.Replace(s, `layoutImplicit="hf"`, `layoutImplicit="hff"`, 1) },
		func(s string) string { return strings.Replace(s, `w:top="600"`, `w:top="NaN"`, 1) },
	} {
		if _, err := ReadDOCX(context.Background(), bytes.NewReader(rewriteListDOCX(t, out.Bytes(), change))); err == nil {
			t.Fatal("malformed/contradictory page metadata accepted")
		}
	}
	units := rewriteListDOCX(t, plain, func(s string) string {
		s = strings.ReplaceAll(s, `w:top="600"`, `w:top="25.4mm"`)
		s = strings.ReplaceAll(s, `w:left="600"`, `w:left="1in"`)
		s = strings.ReplaceAll(s, `w:right="600"`, `w:right="2.54cm"`)
		s = strings.ReplaceAll(s, `w:bottom="600"`, `w:bottom="6pc"`)
		s = strings.ReplaceAll(s, `w:header="360"`, `w:header="12pt"`)
		return strings.ReplaceAll(s, `w:footer="360"`, `w:footer="1pi"`)
	})
	back, err = ReadDOCX(context.Background(), bytes.NewReader(units))
	if err != nil {
		t.Fatal(err)
	}
	g := pageLayout(back.Layout)
	for _, v := range []float64{g.margins.Top, g.margins.Right, g.margins.Bottom, g.margins.Left} {
		if math.Abs(v-72) > 1e-8 {
			t.Fatal("unit conversion", v)
		}
	}
	if *back.Layout.HeaderDistance != 12 || *back.Layout.FooterDistance != 12 {
		t.Fatal("distance units")
	}
}

func FuzzPageLayoutRoundTrips(f *testing.F) {
	f.Add(uint16(480), uint16(420), byte(70), byte(60), byte(55), byte(30), byte(12))
	f.Add(uint16(300), uint16(400), byte(0), byte(0), byte(0), byte(0), byte(0))
	blob := runningPNG(f, false)
	f.Fuzz(func(t *testing.T, width, height uint16, top, right, bottom, left, dist byte) {
		l := Layout{Width: float64(width%301 + 300), Height: float64(height%301 + 400), Margin: float64(dist) / 20, Margins: &PageMargins{float64(top % 91), float64(right % 81), float64(bottom % 91), float64(left % 81)}}
		if dist&1 == 0 {
			l.HeaderDistance = pagePoint(float64(dist % 21))
		}
		if dist&2 == 0 {
			l.FooterDistance = pagePoint(float64(dist % 21))
		}
		d := pageTestDoc(l)
		d.Blocks = append([]Block{{Kind: Picture, Image: &Image{MediaType: "image/png", Data: blob, Width: 32, Height: 8}}}, d.Blocks...)
		before := canonicalSF(t, d)
		var word bytes.Buffer
		if err := Write(context.Background(), &word, d, DOCX, WriteOptions{}); err != nil {
			t.Fatal(err)
		}
		back, err := ReadDOCX(context.Background(), bytes.NewReader(word.Bytes()))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(before, canonicalSF(t, back)) {
			t.Fatal("page settings/content/assets changed through DOCX")
		}
		var pdf bytes.Buffer
		if err := Write(context.Background(), &pdf, d, PDF, WriteOptions{}); err != nil {
			t.Fatal(err)
		}
		back, err = ReadPDF(context.Background(), bytes.NewReader(pdf.Bytes()), ReadOptions{PDFSkipInference: true})
		if err != nil {
			t.Fatal(err)
		}
		if len(back.Pages) != 2 || strings.Count(back.Text(), "HEADER") != 2 || strings.Count(back.Text(), "FOOTER") != 2 || strings.Count(back.Text(), "BODY TWO") != 1 {
			t.Fatal("layout lost content/pages")
		}
		images := 0
		for _, b := range back.Blocks {
			if b.Image != nil {
				images++
				sameImagePixels(t, blob, b.Image.Data)
				x, y, x1, y1 := quadBounds(*b.Source.Quad)
				g := pageLayout(l)
				if x < g.margins.Left-.01 || x1 > g.width-g.margins.Right+.01 || y < g.margins.Top-.01 || y1 > g.height-g.margins.Bottom+.01 {
					t.Fatal("body figure outside explicit margins")
				}
			}
		}
		if images != 1 || !bytes.Equal(before, canonicalSF(t, d)) {
			t.Fatal("source mutation/image loss")
		}
		assertRecoverySF(t, back)
	})
}

func TestDOCXPageCurrentPropertiesAndLossDiagnostics(t *testing.T) {
	section := `<w:sectPr><w:pgSz w:w="8000" w:h="10000"/><w:pgMar w:top="1000" w:right="800" w:bottom="900" w:left="600" w:header="240" w:footer="280"/><w:cols w:num="2"/><w:pgBorders/><w:titlePg/><w:sectPrChange><w:sectPr><w:pgSz w:w="12000" w:h="14000"/><w:pgMar w:top="3000" w:right="2000" w:bottom="3000" w:left="2000"/><w:headerReference xmlns:r="` + relNS + `" r:id="missing-old" w:type="even"/></w:sectPr></w:sectPrChange></w:sectPr>`
	body := `<w:p><w:r><w:t>CURRENT</w:t></w:r></w:p><w:del><w:p><w:pPr><w:sectPr><w:pgMar w:left="9999"/></w:sectPr></w:pPr><w:r><w:delText>DELETED</w:delText></w:r></w:p></w:del>` + section
	data := themeColorDOCX(t, body, themeColorPalette, `<w:mirrorMargins/><w:evenAndOddHeaders/>`, "")
	d, err := ReadDOCX(context.Background(), bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if d.Text() != "CURRENT" || d.Layout.Width != 400 || d.Layout.Height != 500 || d.Layout.Margins == nil || *d.Layout.Margins != (PageMargins{50, 40, 45, 30}) || *d.Layout.HeaderDistance != 12 || *d.Layout.FooterDistance != 14 {
		t.Fatalf("history/deleted properties replaced current geometry: %+v %q", d.Layout, d.Text())
	}
	for _, code := range []string{"docx_page_settings", "docx_page_columns", "revisions"} {
		if !hasDiagnostic(d, code) {
			t.Fatal("missing loss diagnostic", code)
		}
	}
	if hasDiagnostic(d, "multiple_sections") {
		t.Fatal("history/deleted snapshots counted as actual sections")
	}
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, DOCX, WriteOptions{}); err == nil || out.Len() != 0 {
		t.Fatal("unacknowledged page-property loss exported")
	}
}

func TestPageMarginsDoNotShrinkReservedRunningImages(t *testing.T) {
	d := pageTestDoc(Layout{Width: 300, Height: 500, Margins: &PageMargins{200, 30, 100, 30}, HeaderDistance: pagePoint(12), FooterDistance: pagePoint(18)})
	d.Headers = []Block{{Kind: Picture, Image: &Image{MediaType: "image/png", Data: runningPNG(t, true), Width: 18.75, Height: 150}}}
	before := canonicalSF(t, d)
	var word bytes.Buffer
	if err := Write(context.Background(), &word, d, DOCX, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	back, err := ReadDOCX(context.Background(), bytes.NewReader(word.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, canonicalSF(t, back)) {
		t.Fatal("reserved header picture was shrunk to the small body container")
	}
	var pdf bytes.Buffer
	if err := Write(context.Background(), &pdf, d, PDF, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	back, err = ReadPDF(context.Background(), bytes.NewReader(pdf.Bytes()), ReadOptions{PDFSkipInference: true})
	if err != nil {
		t.Fatal(err)
	}
	images := 0
	for _, b := range back.Blocks {
		if b.Image != nil {
			images++
			x, y, x1, y1 := quadBounds(*b.Source.Quad)
			if math.Abs(x-30) > .01 || math.Abs(y-12) > .01 || math.Abs(x1-x-18.75) > .01 || math.Abs(y1-y-150) > .01 {
				t.Fatal("reserved running image bounds", b.Source.Quad)
			}
		}
	}
	if images != 2 {
		t.Fatal("running repetition")
	}
}

func TestPageLayoutSignedNonoverlapPDF(t *testing.T) {
	d := pageTestDoc(Layout{Width: 360, Height: 420, Margins: &PageMargins{-60, 44, -55, 22}, HeaderDistance: pagePoint(12), FooterDistance: pagePoint(14)})
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, PDF, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	back, err := ReadPDF(context.Background(), bytes.NewReader(out.Bytes()), ReadOptions{PDFSkipInference: true})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, b := range back.Blocks {
		for _, run := range b.Runs {
			if run.Text == "BODY" {
				// Source quads describe font bounds, whose top is below the
				// paragraph origin. Check the real 11pt baseline at 60+11.
				if run.Source == nil || run.Source.Baseline == nil {
					t.Fatal("missing source baseline")
				}
				point := run.Source.Baseline[0]
				if math.Abs(point.X-22) > .01 || math.Abs(point.Y-71) > .01 {
					t.Fatal("signed body baseline was not fixed", point)
				}
				found = true
			}
		}
	}
	if !found {
		t.Fatal("missing body")
	}
}

func TestPageLayoutPhysicalSerializationAtomic(t *testing.T) {
	for _, item := range []struct {
		format Format
		layout Layout
	}{
		{DOCX, Layout{Width: .001, Height: 200, Margins: &PageMargins{}}},
		{PDF, Layout{Width: .001, Height: 200, Margins: &PageMargins{}}},
		{DOCX, Layout{Width: 200, Height: 200.0001, Margins: &PageMargins{100, 0, 100, 0}}},
		{DOCX, Layout{Width: 200, Height: 200, Margins: &PageMargins{-.001, 0, 0, 0}}},
		{DOCX, Layout{Width: math.MaxFloat64, Height: 200, Margins: &PageMargins{}}},
	} {
		d := &Document{Layout: item.layout}
		if err := d.Validate(); err != nil {
			t.Fatal("finite persistent layout should remain representable in SF", err)
		}
		var out bytes.Buffer
		if err := Write(context.Background(), &out, d, item.format, WriteOptions{}); !errors.Is(err, ErrUnsupported) || out.Len() != 0 {
			t.Fatalf("unrepresentable %s page emitted: %v (%d bytes)", item.format, err, out.Len())
		}
	}
}

func TestPageLayoutUnusedDistancesPersist(t *testing.T) {
	d := pageTestDoc(Layout{Width: 360, Height: 420, Margin: 30, HeaderDistance: pagePoint(900), FooterDistance: pagePoint(900)})
	d.Headers, d.Footers = nil, nil
	before := canonicalSF(t, d)
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, DOCX, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	back, err := ReadDOCX(context.Background(), bytes.NewReader(out.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, canonicalSF(t, back)) {
		t.Fatal("unused distances were dropped or normalized")
	}
	d.Headers = []Block{{Kind: Paragraph, Runs: []Run{{Text: "HEADER"}}}}
	for _, format := range []Format{DOCX, PDF} {
		out.Reset()
		if err := Write(context.Background(), &out, d, format, WriteOptions{}); !errors.Is(err, ErrUnsupported) || out.Len() != 0 {
			t.Fatal("out-of-page running story was not rejected atomically", format, err)
		}
	}
}
