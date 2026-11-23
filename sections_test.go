package manydoc

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
)

func sectionParagraph(text string) Block { return Block{Kind: Paragraph, Runs: []Run{{Text: text}}} }
func sectionStory(text string) *RunningStory {
	return &RunningStory{Blocks: []Block{sectionParagraph(text)}}
}
func sectionTestDocument(t testing.TB) *Document {
	blob := runningPNG(t, false)
	landscape := Layout{Width: 480, Height: 360, Margin: 29, Margins: &PageMargins{Top: 45, Right: 40, Bottom: 50, Left: 30}, HeaderDistance: pagePoint(12), FooterDistance: pagePoint(15)}
	image := Block{Kind: Picture, Image: &Image{MediaType: "image/png", Data: blob, Width: 32, Height: 16}}
	a := &Section{FirstPage: true, Headers: RunningStories{Default: sectionStory("A DEFAULT"), First: sectionStory("A FIRST"), Even: sectionStory("A EVEN")}, Footers: RunningStories{Default: sectionStory("A FOOT"), First: &RunningStory{}, Even: sectionStory("A EVEN FOOT")}, Blocks: []Block{sectionParagraph("A BODY"), {Kind: Table, Rows: []Row{{Cells: []Cell{{ColSpan: 2, Blocks: []Block{image}}}}, {Cells: []Cell{{Blocks: []Block{sectionParagraph("CELL LEFT")}}, {Blocks: []Block{sectionParagraph("CELL RIGHT")}}}}}}}}
	a.Headers.Default.Blocks = append(a.Headers.Default.Blocks, image)
	a.Headers.First.Blocks[0].Runs[0].Link = "https://example.com/first"
	b := &Section{Layout: &landscape, Start: SectionOddPage, FirstPage: true, Headers: RunningStories{First: &RunningStory{}}, Blocks: []Block{{Kind: ListBlock, List: "number", Items: []ListItem{{Blocks: []Block{sectionParagraph("B LIST")}}}}, sectionParagraph("B BODY")}}
	b.Blocks[1].Runs = append(b.Blocks[1].Runs, Run{Note: &NoteReference{ID: "section-note", Kind: Footnote}})
	c := &Section{Start: SectionNextPage, Headers: RunningStories{Default: &RunningStory{}}, Blocks: []Block{sectionParagraph("C BODY")}}
	return &Document{Layout: Layout{Width: 360, Height: 440, Margin: 40}, EvenAndOddHeaders: true, Blocks: []Block{{Kind: SectionBlock, Section: a}, {Kind: SectionBlock, Section: b}, {Kind: SectionBlock, Section: c}}, Notes: []Note{{ID: "section-note", Kind: Footnote, Blocks: []Block{sectionParagraph("SECTION NOTE"), image}}}}
}
func TestSectionSFAndDOCXSemantics(t *testing.T) {
	d := sectionTestDocument(t)
	before := canonicalSF(t, d)
	for _, f := range []Format{SF, JSON, DOCX} {
		var out bytes.Buffer
		if err := Write(context.Background(), &out, d, f, WriteOptions{}); err != nil {
			t.Fatal(f, err)
		}
		back, err := Read(context.Background(), &out, f, ReadOptions{})
		if err != nil {
			t.Fatal(f, err)
		}
		if got := canonicalSF(t, back); !bytes.Equal(before, got) {
			t.Fatalf("%s section semantics changed\nwant %s\ngot %s", f, before, got)
		}
	}
	if !bytes.Equal(before, canonicalSF(t, d)) {
		t.Fatal("source changed")
	}
	resolved := documentSections(d)
	if len(resolved) != 3 || resolved[1].headers.Default != d.Blocks[0].Section.Headers.Default || len(resolved[1].headers.First.Blocks) != 0 || len(resolved[2].headers.Default.Blocks) != 0 || resolved[2].layout.Width != 480 {
		t.Fatal("story/layout inheritance")
	}
	sf, err := ToStructured(d)
	if err != nil {
		t.Fatal(err)
	}
	sf.Content[1].Attrs = bytes.Replace(sf.Content[1].Attrs, []byte(`"width":480`), []byte(`"width":481`), 1)
	changed, err := FromStructured(sf)
	if err != nil {
		t.Fatal(err)
	}
	changed.Blocks[1].Section.Layout.Margins.Left = 31
	if d.Blocks[1].Section.Layout.Width != 480 || d.Blocks[1].Section.Layout.Margins.Left != 30 {
		t.Fatal("SF layout aliases source")
	}
}
func TestSectionValidation(t *testing.T) {
	for name, mutate := range map[string]func(*Document){
		"mixed-root":   func(d *Document) { d.Blocks = append(d.Blocks, sectionParagraph("VISIBLE")) },
		"nested":       func(d *Document) { d.Blocks[0].Section.Blocks = []Block{{Kind: SectionBlock, Section: &Section{}}} },
		"bad-start":    func(d *Document) { d.Blocks[0].Section.Start = "nextColumn" },
		"dual-stories": func(d *Document) { d.Headers = []Block{sectionParagraph("GLOBAL")} },
		"running-note": func(d *Document) {
			d.Blocks[0].Section.Headers.First.Blocks[0].Runs = []Run{{Note: &NoteReference{Kind: Footnote, ID: "section-note"}}}
		},
		"hidden-fields": func(d *Document) { d.Blocks[0].Runs = []Run{{Text: "VISIBLE"}} },
		"cyclic":        func(d *Document) { d.Blocks[0].Section.Blocks = d.Blocks },
	} {
		t.Run(name, func(t *testing.T) {
			d := sectionTestDocument(t)
			mutate(d)
			if err := d.Validate(); err == nil {
				t.Fatal("invalid sections accepted")
			}
		})
	}
	for _, attrs := range []string{`{"headers":{"unexpected":[]}}`, `{"headers":{"first":null}}`, `{"firstPage":"true"}`, `{"start":"invalid"}`} {
		sf := &StructuredDocument{Type: "doc", Attrs: StructuredAttributes{Schema: StructuredSchema}, Content: []StructuredNode{{Type: "section", Attrs: []byte(attrs)}}}
		if _, err := FromStructured(sf); err == nil {
			t.Fatal("invalid section attrs accepted", attrs)
		}
	}
}
func TestDOCXSectionMetadataCannotHideContent(t *testing.T) {
	d := sectionTestDocument(t)
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, DOCX, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	for i, change := range []func(string) string{
		func(s string) string {
			return strings.Replace(s, `md:sectionTail="1"><w:pPr>`, `md:sectionTail="1"><w:r><w:t>VISIBLE</w:t></w:r><w:pPr>`, 1)
		},
		func(s string) string { return strings.Replace(s, `w:w="7200"`, `w:w="7201"`, 1) },
		func(s string) string { return strings.Replace(s, `w:type="default"`, `w:type="unknown"`, 1) },
		func(s string) string {
			return strings.Replace(s, `md:sectionStartImplicit="1"`, `md:sectionStartImplicit="2"`, 1)
		},
	} {
		if _, err := ReadDOCX(context.Background(), bytes.NewReader(rewriteListDOCX(t, out.Bytes(), change))); err == nil {
			t.Fatal("contradictory section metadata accepted", i)
		}
	}
}

func TestPDFSectionFramesVariantsAndNotes(t *testing.T) {
	d := sectionTestDocument(t)
	a := d.Blocks[0].Section
	a.Blocks = []Block{sectionParagraph("A PAGE ONE"), {Kind: PageBreak}, sectionParagraph("A PAGE TWO"), {Kind: PageBreak}, sectionParagraph("A PAGE THREE")}
	b := d.Blocks[1].Section
	b.Blocks = append([]Block{sectionParagraph("B PAGE ONE"), {Kind: PageBreak}, sectionParagraph("B PAGE TWO"), {Kind: PageBreak}}, b.Blocks...)
	before := canonicalSF(t, d)
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, PDF, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	got, err := ReadPDF(context.Background(), bytes.NewReader(out.Bytes()), ReadOptions{PDFSkipInference: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Pages) != 8 {
		t.Fatalf("pages=%d text=%q", len(got.Pages), got.Text())
	}
	texts := make([]string, 8)
	for _, block := range got.Blocks {
		if block.Source != nil && len(block.Source.Regions) > 0 {
			n := block.Source.Regions[0].Page
			if n > 0 && n <= 8 {
				texts[n-1] += (&Document{Blocks: []Block{block}}).Text() + "\n"
			}
		}
	}
	for _, c := range []struct {
		page         int
		want, absent string
	}{{1, "A FIRST", "A DEFAULT"}, {2, "A EVEN", "A FIRST"}, {3, "A DEFAULT", "A FIRST"}, {4, "", "A FOOT"}, {5, "B PAGE ONE", "A FIRST"}, {6, "A EVEN", "A DEFAULT"}, {7, "A DEFAULT", "A FIRST"}, {8, "C BODY", "A DEFAULT"}} {
		if !strings.Contains(texts[c.page-1], c.want) || c.absent != "" && strings.Contains(texts[c.page-1], c.absent) {
			t.Fatalf("page %d text=%q", c.page, texts[c.page-1])
		}
	}
	if strings.TrimSpace(texts[3]) != "" {
		t.Fatal("parity page is not blank", texts[3])
	}
	for i, p := range got.Pages {
		wantW, wantH := 360.0, 440.0
		if i >= 4 {
			wantW, wantH = 480, 360
		}
		if p.Width != wantW || p.Height != wantH {
			t.Fatalf("page %d geometry=%+v", i+1, p)
		}
	}
	if !bytes.Equal(before, canonicalSF(t, d)) {
		t.Fatal("PDF planning mutated source")
	}
}
func TestPDFContinuousSectionContract(t *testing.T) {
	d := &Document{Layout: Layout{Width: 300, Height: 400, Margin: 30}, Blocks: []Block{{Kind: SectionBlock, Section: &Section{Blocks: []Block{sectionParagraph("ONE")}}}, {Kind: SectionBlock, Section: &Section{Start: SectionContinuous, Blocks: []Block{sectionParagraph("TWO")}}}}}
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, PDF, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	got, err := ReadPDF(context.Background(), bytes.NewReader(out.Bytes()), ReadOptions{PDFSkipInference: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Pages) != 1 || !strings.Contains(got.Text(), "TWO") {
		t.Fatal("continuous section started a page", got.Text())
	}
	d.Blocks[1].Section.Layout = &Layout{Width: 400, Height: 300, Margin: 30}
	out.Reset()
	if err := Write(context.Background(), &out, d, PDF, WriteOptions{}); err == nil || out.Len() != 0 {
		t.Fatal("unqualified shared-page geometry accepted or partial output")
	}
}

func FuzzSectionRoundTrips(f *testing.F) {
	f.Add(byte(0), byte(0), uint16(300), uint16(400))
	f.Add(byte(7), byte(3), uint16(500), uint16(320))
	f.Fuzz(func(t *testing.T, flags, start byte, width, height uint16) {
		layout := Layout{Width: float64(width%401 + 300), Height: float64(height%401 + 300), Margin: 30}
		s := &Section{Layout: &layout, Start: []SectionStart{"", SectionNextPage, SectionOddPage, SectionEvenPage}[start%4], FirstPage: flags&1 != 0, Blocks: []Block{sectionParagraph("SECTION TWO")}}
		if flags&2 != 0 {
			s.Headers.Default = &RunningStory{}
		}
		if flags&4 != 0 {
			s.Headers.First = sectionStory("NEW FIRST")
		}
		d := &Document{Layout: Layout{Width: 360, Height: 440, Margin: 30}, EvenAndOddHeaders: true, Blocks: []Block{{Kind: SectionBlock, Section: &Section{Headers: RunningStories{Default: sectionStory("DEFAULT"), First: sectionStory("FIRST"), Even: sectionStory("EVEN")}, Blocks: []Block{sectionParagraph("SECTION ONE")}}}, {Kind: SectionBlock, Section: s}}}
		before := canonicalSF(t, d)
		for _, format := range []Format{SF, DOCX} {
			var out bytes.Buffer
			if err := Write(context.Background(), &out, d, format, WriteOptions{}); err != nil {
				t.Fatal(err)
			}
			back, err := Read(context.Background(), &out, format, ReadOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(before, canonicalSF(t, back)) {
				t.Fatal("section semantics changed", format)
			}
		}
		var out bytes.Buffer
		if err := Write(context.Background(), &out, d, PDF, WriteOptions{}); err != nil {
			t.Fatal(err)
		}
		back, err := ReadPDF(context.Background(), &out, ReadOptions{PDFSkipInference: true})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(back.Text(), "SECTION ONE") || !strings.Contains(back.Text(), "SECTION TWO") {
			t.Fatal("section body lost")
		}
	})
}

func FuzzSectionBindings(f *testing.F) {
	f.Add(byte(0), byte(0), uint16(300), uint16(400))
	f.Add(byte(7), byte(3), uint16(500), uint16(320))
	f.Fuzz(func(t *testing.T, flags, start byte, width, height uint16) {
		starts := []SectionStart{"", SectionNextPage, SectionEvenPage, SectionOddPage}
		d := &Document{Layout: Layout{Width: 360, Height: 440, Margin: 40}, EvenAndOddHeaders: flags&1 != 0}
		for i := 0; i < 3; i++ {
			s := &Section{Start: starts[(int(start)+i)%4], FirstPage: flags&(1<<i) != 0, Blocks: []Block{sectionParagraph("BODY")}}
			if i == 1 {
				s.Layout = &Layout{Width: float64(width%400 + 300), Height: float64(height%400 + 300), Margin: 30}
			}
			if flags&2 != 0 {
				s.Headers.Default = sectionStory("DEFAULT")
			}
			if flags&4 != 0 {
				s.Headers.First = &RunningStory{}
			}
			if flags&8 != 0 {
				s.Headers.Even = sectionStory("EVEN")
			}
			d.Blocks = append(d.Blocks, Block{Kind: SectionBlock, Section: s})
		}
		want := canonicalSF(t, d)
		var out bytes.Buffer
		if err := Write(context.Background(), &out, d, DOCX, WriteOptions{}); err != nil {
			t.Fatal(err)
		}
		back, err := ReadDOCX(context.Background(), &out)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(want, canonicalSF(t, back)) {
			t.Fatal("section layout/story/binding semantics changed")
		}
		out.Reset()
		if err := Write(context.Background(), &out, d, PDF, WriteOptions{}); err != nil {
			t.Fatal(err)
		}
		if _, err := ReadPDF(context.Background(), &out, ReadOptions{PDFSkipInference: true}); err != nil {
			t.Fatal(err)
		}
	})
}

func TestDOCXForeignSectionBoundaries(t *testing.T) {
	body := `<w:p><w:pPr><w:sectPr><w:type w:val="nextPage"/><w:pgSz w:w="7200" w:h="8800"/><w:pgMar w:top="800" w:right="800" w:bottom="800" w:left="800"/></w:sectPr></w:pPr><w:r><w:t>FIRST SECTION END</w:t></w:r></w:p><w:p><w:r><w:t>SECOND SECTION BODY</w:t></w:r></w:p><w:sectPr><w:type w:val="evenPage"/><w:pgSz w:w="9600" w:h="7200"/><w:pgMar w:top="900" w:right="1000" w:bottom="1100" w:left="1200"/><w:titlePg/></w:sectPr>`
	data := fontSelectionDOCX(t, body, "", "<w:evenAndOddHeaders/>", "", "")
	got, err := ReadDOCX(context.Background(), bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Blocks) != 2 || got.Blocks[0].Kind != SectionBlock || got.Blocks[1].Kind != SectionBlock {
		t.Fatal("foreign sections flattened")
	}
	a, b := got.Blocks[0].Section, got.Blocks[1].Section
	if a.Layout.Width != 360 || b.Layout.Width != 480 || b.Start != SectionEvenPage || !b.FirstPage || !got.EvenAndOddHeaders || len(a.Blocks) != 1 || len(b.Blocks) != 1 {
		t.Fatal("foreign section settings/content lost")
	}
	if b.Blocks[0].Runs[0].Text != "SECOND SECTION BODY" || a.Blocks[0].Runs[0].Text != "FIRST SECTION END" {
		t.Fatal("end properties shifted the body boundary")
	}
	if a.Headers.Default != nil || b.Headers.Even != nil {
		t.Fatal("missing stories fabricated")
	}
	for _, diag := range got.Diagnostics {
		if diag.Code == "multiple_sections" || diag.Code == "docx_running_variants" || strings.Contains(diag.Message, "titlePg") || strings.Contains(diag.Message, "evenAndOddHeaders") {
			t.Fatal("represented settings still diagnosed as loss", diag)
		}
	}
	checkParagraphRoundtrip(t, got)
}

func TestDOCXEmptySectionStoryCannotHideContent(t *testing.T) {
	d := sectionTestDocument(t)
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, DOCX, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(out.Bytes()), int64(out.Len()))
	if err != nil {
		t.Fatal(err)
	}
	var edited bytes.Buffer
	zw := zip.NewWriter(&edited)
	changed := false
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		if strings.HasPrefix(f.Name, "word/header") && bytes.Contains(data, []byte(`md:emptyStory="1"/>`)) {
			data = bytes.Replace(data, []byte(`md:emptyStory="1"/>`), []byte(`md:emptyStory="1"><w:r><w:t>VISIBLE CONTENT</w:t></w:r></w:p>`), 1)
			changed = true
		}
		w, err := zw.Create(f.Name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = w.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("no empty story edited")
	}
	if _, err := ReadDOCX(context.Background(), &edited); err == nil {
		t.Fatal("empty story metadata hid visible content")
	}
}
