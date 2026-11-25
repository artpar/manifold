package engine

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
	"io"
	"math"
	"math/rand"
	"os"
	"reflect"
	"strings"
	"testing"
)

func repeatedHeaderDocument(t testing.TB) *Document {
	t.Helper()
	b, err := os.ReadFile("examples/repeated-headers/document.sf")
	if err != nil {
		t.Fatal(err)
	}
	d, err := Read(context.Background(), bytes.NewReader(b), SF, ReadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func checkRepeatedHeaders(t testing.TB, d *Document, header, bodyPrefix string, count int) *Document {
	t.Helper()
	want := canonicalSF(t, d)
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, PDF, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(want, canonicalSF(t, d)) {
		t.Fatal("PDF mutated authored rows")
	}
	got, err := ReadPDF(context.Background(), bytes.NewReader(out.Bytes()), ReadOptions{PDFSkipInference: true, PDFSkipStructure: true})
	if err != nil {
		t.Fatal(err)
	}
	pages := map[int]string{}
	for _, b := range got.Blocks {
		for _, run := range b.Runs {
			if run.Source == nil || run.Source.Quad == nil {
				continue
			}
			pages[run.Source.Page] += strings.Join(strings.Fields(run.Text), "")
			x0, y0, x1, y1 := quadBounds(*run.Source.Quad)
			if x0 < d.Layout.Margin-.05 || y0 < d.Layout.Margin-.05 || x1 > d.Layout.Width-d.Layout.Margin+.05 || y1 > d.Layout.Height-d.Layout.Margin+.05 {
				t.Fatalf("text outside page: %q", run.Text)
			}
		}
	}
	all := strings.Join(strings.Fields(got.Text()), "")
	for n := 0; n < count; n++ {
		token := fmt.Sprintf("%s%02d", bodyPrefix, n)
		if strings.Count(all, token) != 1 {
			t.Fatalf("body content lost/duplicated: %s", token)
		}
	}
	for page, text := range pages {
		if strings.Contains(text, bodyPrefix) && strings.Count(text, header) != 1 {
			t.Fatalf("page %d lacks exactly one header: %s", page, text)
		}
	}
	return got
}

func TestRepeatingHeaderSFDOCXAndPDF(t *testing.T) {
	d := repeatedHeaderDocument(t)
	checkParagraphRoundtrip(t, d) // Whole SF, including row flags, merges, marks, notes and assets.
	got := checkRepeatedHeaders(t, d, "HEADER_MAIN", "SPANNING_", 20)
	if got.Metadata.PageCount < 3 {
		t.Fatal("fixture did not paginate")
	}
	all := strings.Join(strings.Fields(got.Text()), "")
	for _, token := range []string{"RIGHT_A", "RIGHT_C", "SPANNING_LAST", "AFTER_GRID", "NOTE_EARLY", "NOTE_LATE"} {
		if strings.Count(all, token) != 1 {
			t.Fatalf("content lost/duplicated: %s", token)
		}
	}
	for _, b := range got.Blocks {
		if b.Kind == Picture {
			x0, y0, x1, y1 := quadBounds(*b.Source.Quad)
			if x0 < 20 || y0 < 20 || x1 > 300 || y1 > 180 {
				t.Fatal("header/body image escaped page")
			}
		}
	}
}

func TestRepeatingHeadersNestedAndFootnote(t *testing.T) {
	for depth := 0; depth < 4; depth++ {
		for _, note := range []bool{false, true} {
			t.Run(fmt.Sprintf("depth%d/note%t", depth, note), func(t *testing.T) {
				table := Block{Kind: Table, Rows: []Row{{RepeatHeader: true, Cells: []Cell{{Blocks: []Block{mergeParagraph("NEST_HEADER")}}}}}}
				for i := 0; i < 18; i++ {
					table.Rows = append(table.Rows, Row{Cells: []Cell{{Blocks: []Block{mergeParagraph(fmt.Sprintf("BODY_%02d", i))}}}})
				}
				for i := 0; i < depth; i++ {
					table = cellTable([]Block{table})
				}
				d := &Document{Layout: Layout{Width: 320, Height: 240, Margin: 20}, Blocks: []Block{table}}
				if note {
					d.Blocks = []Block{{Kind: Paragraph, Runs: []Run{{Text: "REFERENCE"}, {Note: &NoteReference{Kind: Footnote, ID: "n"}}}}}
					d.Notes = []Note{{Kind: Footnote, ID: "n", Blocks: []Block{table}}}
				}
				checkParagraphRoundtrip(t, d)
				got := checkRepeatedHeaders(t, d, "NEST_HEADER", "BODY_", 18)
				if got.Metadata.PageCount < 3 {
					t.Fatal("table did not paginate")
				}
			})
		}
	}
}

func TestDOCXTableHeaderBooleanAndNormalization(t *testing.T) {
	row := func(pr, text string) string {
		return `<w:tr><w:trPr>` + pr + `</w:trPr><w:tc><w:p><w:r><w:t>` + text + `</w:t></w:r></w:p></w:tc></w:tr>`
	}
	for _, value := range []string{"true", "on", "1", "false", "off", "0"} {
		d, err := ReadDOCX(context.Background(), bytes.NewReader(docxWithBody(t, `<w:tbl>`+row(`<w:tblHeader w:val="`+value+`"/>`, "HEAD")+row("", "BODY")+`</w:tbl>`)))
		if err != nil {
			t.Fatal(err)
		}
		if d.Blocks[0].Rows[0].RepeatHeader != (value == "true" || value == "on" || value == "1") {
			t.Fatal("boolean changed")
		}
	}
	for _, pr := range []string{`<w:tblHeader w:val="maybe"/>`, `<w:tblHeader w:val=""/>`, `<w:tblHeader/><w:tblHeader/>`} {
		if _, err := ReadDOCX(context.Background(), bytes.NewReader(docxWithBody(t, `<w:tbl>`+row(pr, "HEAD")+`</w:tbl>`))); err == nil {
			t.Fatal("invalid property accepted")
		}
	}
	d, err := ReadDOCX(context.Background(), bytes.NewReader(docxWithBody(t, `<w:tbl>`+row("", "BODY")+row(`<w:tblHeader/>`, "LATE")+`</w:tbl>`)))
	if err != nil {
		t.Fatal(err)
	}
	if d.Blocks[0].Rows[1].RepeatHeader || len(d.Diagnostics) != 1 || !strings.Contains(d.Text(), "LATE") {
		t.Fatal("noncontiguous header not diagnosed/preserved")
	}
}

func TestInvalidRepeatHeadersAtomic(t *testing.T) {
	for name, rows := range map[string][]Row{
		"gap":            {{Cells: []Cell{{}}}, {RepeatHeader: true, Cells: []Cell{{}}}},
		"crossing_merge": {{RepeatHeader: true, Cells: []Cell{{RowSpan: 2}}}, {}},
	} {
		t.Run(name, func(t *testing.T) {
			d := &Document{Blocks: []Block{{Kind: Table, Rows: rows}}}
			for _, format := range []Format{SF, DOCX, PDF} {
				var b bytes.Buffer
				if err := Write(context.Background(), &b, d, format, WriteOptions{}); err == nil || b.Len() != 0 {
					t.Fatalf("%s accepted invalid headers", format)
				}
			}
		})
	}
}

func TestRepeatingHeaderNotesReuseActualDestination(t *testing.T) {
	table := Block{Kind: Table, Rows: []Row{{RepeatHeader: true, Cells: []Cell{{Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "HEADER"}, {Note: &NoteReference{Kind: Footnote, ID: "h"}}}}}}}}}}
	for i := 0; i < 20; i++ {
		table.Rows = append(table.Rows, Row{Cells: []Cell{{Blocks: []Block{mergeParagraph(fmt.Sprintf("BODY_%02d", i))}}}})
	}
	d := &Document{Layout: Layout{Width: 320, Height: 220, Margin: 20}, Blocks: []Block{table}, Notes: []Note{{Kind: Footnote, ID: "h", Blocks: []Block{mergeParagraph("HEADER_NOTE")}}}}
	got, data := renderPageBreakDocument(t, d)
	checkParagraphRoundtrip(t, d)
	if strings.Count(strings.Join(strings.Fields(got.Text()), ""), "HEADER_NOTE") != 1 {
		t.Fatal("repeated header duplicated its note body")
	}
	p, err := pdfcpu.ReadWithContext(context.Background(), bytes.NewReader(data), &model.Configuration{Reader15: true, ValidationMode: model.ValidationStrict, Offline: true})
	if err != nil {
		t.Fatal(err)
	}
	if err = p.EnsurePageCount(); err != nil {
		t.Fatal(err)
	}
	_, target, _, err := p.PageDict(1, false)
	if err != nil {
		t.Fatal(err)
	}
	for page := 1; page <= got.Metadata.PageCount; page++ {
		dict, _, _, err := p.PageDict(page, false)
		if err != nil {
			t.Fatal(err)
		}
		annots, err := p.DereferenceArray(dict["Annots"])
		if err != nil {
			t.Fatal(err)
		}
		found := 0
		for _, a := range annots {
			annot, err := p.DereferenceDict(a)
			if err != nil {
				t.Fatal(err)
			}
			if annot["Dest"] == nil {
				continue
			}
			dest, err := p.DereferenceArray(annot["Dest"])
			if err != nil {
				t.Fatal(err)
			}
			ref, ok := dest[0].(types.IndirectRef)
			if !ok || ref != *target {
				t.Fatalf("header page%d targets wrong note page: %v", page, dest)
			}
			found++
		}
		if found != 1 {
			t.Fatalf("page%d header link count%d", page, found)
		}
	}
}

func TestRepeatingHeaderKeepsFirstBodyAndFitsImage(t *testing.T) {
	figure := imageFitDocument(t).Blocks[0]
	table := Block{Kind: Table, Rows: []Row{{RepeatHeader: true, Cells: []Cell{{Blocks: []Block{mergeParagraph("HEADER")}}}}, {Cells: []Cell{{Blocks: []Block{figure}}}}}}
	d := &Document{Layout: Layout{Width: 300, Height: 200, Margin: 20}, Blocks: []Block{mergeParagraph("PREFACE"), table}}
	got, _ := renderPageBreakDocument(t, d)
	checkTokenPage(t, got, "PREFACE", 1)
	checkTokenPage(t, got, "HEADER", 2)
	if got.Metadata.PageCount != 2 {
		t.Fatal("header was orphaned from fitted image")
	}
	images := 0
	for _, b := range got.Blocks {
		if b.Kind == Picture {
			images++
			x0, y0, x1, y1 := quadBounds(*b.Source.Quad)
			if b.Source.Page != 2 || math.Abs(x0-26) > .04 || math.Abs(y0-52) > .04 || math.Abs(x1-37.4) > .04 || math.Abs(y1-166) > .04 {
				t.Fatalf("header image budget/placement: %v", *b.Source.Quad)
			}
			sameImagePixels(t, figure.Image.Data, b.Image.Data)
		}
	}
	if images != 1 {
		t.Fatal("body image was duplicated/lost")
	}
}

func TestRepeatingHeaderControlsAndHeightFailAtomically(t *testing.T) {
	tall := make([]Block, 30)
	for i := range tall {
		tall[i] = mergeParagraph("HEADER")
	}
	for name, blocks := range map[string][]Block{"forced": {{Kind: PageBreak}}, "conditional": {{Kind: Paragraph, PageBreakBefore: true, Runs: []Run{{Text: "HEADER"}}}}, "too_tall": tall} {
		t.Run(name, func(t *testing.T) {
			d := &Document{Layout: Layout{Width: 300, Height: 200, Margin: 20}, Blocks: []Block{{Kind: Table, Rows: []Row{{RepeatHeader: true, Cells: []Cell{{Blocks: blocks}}}, {Cells: []Cell{{Blocks: []Block{mergeParagraph("BODY")}}}}}}}}
			var out bytes.Buffer
			if err := Write(context.Background(), &out, d, PDF, WriteOptions{}); err == nil || out.Len() != 0 {
				t.Fatal("unrepresentable header produced partial output")
			}
			// Persist the original control/height request for an editor/other writer.

			// Independent, hand-authored OOXML. In particular, continuation cells are
			// present here although they must be omitted from the SF rows.

			// A fully covered row still exists even though its SF Cells are empty.

			// crosses occupied second column
			// missing first-column cell

			checkParagraphRoundtrip(t, d)
		})
	}
}

func TestRepeatingHeaderOnExplicitBodyContinuation(t *testing.T) {
	table := Block{Kind: Table, Rows: []Row{
		{RepeatHeader: true, Cells: []Cell{{Blocks: []Block{mergeParagraph("HEADER")}}}},
		{Cells: []Cell{{Blocks: []Block{mergeParagraph("BODY_A"), {Kind: PageBreak}, mergeParagraph("BODY_B")}}}},
	}}
	d := &Document{Layout: Layout{Width: 300, Height: 200, Margin: 20}, Blocks: []Block{table}}
	checkParagraphRoundtrip(t, d)
	got, _ := renderPageBreakDocument(t, d)
	if got.Metadata.PageCount != 2 {
		t.Fatal("explicit table continuation lost boundary")
	}
	checkTokenPage(t, got, "BODY_A", 1)
	checkTokenPage(t, got, "BODY_B", 2)
	if strings.Count(strings.Join(strings.Fields(

		// Move the connected 3-row merged region as a whole to the next page.
		got.Text()), ""), "HEADER") != 2 {
		t.Fatal("header not repeated on explicit continuation")
	}
}

func TestDOCXCrossingHeaderMergeRetainsContentWithDiagnostic(t *testing.T) {
	body := `<w:tbl><w:tr><w:trPr><w:tblHeader/></w:trPr><w:tc><w:tcPr><w:vMerge w:val="restart"/></w:tcPr><w:p><w:r><w:t>ORIGIN</w:t></w:r></w:p></w:tc></w:tr><w:tr><w:tc><w:tcPr><w:vMerge/></w:tcPr><w:p/></w:tc></w:tr></w:tbl>`
	d, err := ReadDOCX(context.Background(), bytes.NewReader(docxWithBody(t, body)))
	if err != nil {
		t.Fatal(err)
	}
	if d.Blocks[0].Rows[0].RepeatHeader || d.Blocks[0].Rows[0].Cells[0].RowSpan != 2 || len(d.Diagnostics) != 1 || d.Diagnostics[0].Code != "table_header_merge" || !strings.Contains(d.Text(), "ORIGIN") {
		t.Fatalf("ambiguous repeat merge lost content/diagnostic: %+v",

			// The connected group now continues over several pages.

			// Widths may be positive in SF yet not representable in DOCX twips.

			// Independent original OOXML grid, not an output-derived golden value.

			// Removing the ruling paint retains explicit membership but supplies no widths.

			d)
	}
}

func FuzzRepeatingTableHeaders(f *testing.F) {
	f.Add(uint8(1), uint8(12), uint8(0), uint8(180), false)
	f.Add(uint8(2), uint8(24), uint8(2), uint8(80), true)
	f.Fuzz(func(t *testing.T, h, n, depth, size uint8, note bool) {
		headers := 1 + int(h%3)
		table := Block{Kind: Table}
		for i := 0; i < headers; i++ {
			table.Rows = append(table.Rows, Row{RepeatHeader: true, Cells: []Cell{{Blocks: []Block{mergeParagraph(fmt.Sprintf("HEADER_%d", i))}}}})
		}
		count := 1 + int(n%28)
		for i := 0; i < count; i++ {
			table.Rows = append(table.Rows, Row{Cells: []Cell{{Blocks: []Block{mergeParagraph(fmt.Sprintf("BODY_%02d", i))}}}})
		}
		for i := 0; i < int(depth%3); i++ {
			table = cellTable([]Block{table})
		}
		d := &Document{Layout: Layout{Width: 320, Height: 240 + float64(size), Margin: 20}, Blocks: []Block{table}}
		if note {
			d.Notes = []Note{{Kind: Footnote, ID: "n", Blocks: []Block{table}}}
			d.Blocks = []Block{{Kind: Paragraph, Runs: []Run{{Text: "REF"}, {Note: &NoteReference{Kind: Footnote, ID: "n"}}}}}
		}
		checkParagraphRoundtrip(t, d)
		got := checkRepeatedHeaders(t, d, "HEADER_0", "BODY_", count)
		text := strings.Join(strings.Fields(got.Text()), "")
		copies := strings.Count(text, "HEADER_0")
		for i := 1; i < headers; i++ {
			if strings.Count(text, fmt.Sprintf("HEADER_%d", i)) != copies {
				t.Fatal("only part of the header prefix repeated")
			}
		}
	})
}

func TestSFRepeatingHeaderRequiresBoolean(t *testing.T) {
	for _, value := range []string{`null`, `0`, `"true"`, `[]`, `{}`} {
		sf, err := ToStructured(repeatedHeaderDocument(t))
		if err != nil {
			t.Fatal(err)
		}
		sf.Content[0].Content[0].Attrs = json.RawMessage(`{"repeatHeader":` + value + `}`)
		if _, err := FromStructured(sf); err == nil {
			t.Fatalf("accepted repeatHeader:%s", value)
		}
	}
}

func TestRepeatingHeaderFigureLeavesLegalBodyStart(t *testing.T) {
	for extra := 0; extra < 3; extra++ {
		for depth := 0; depth < 4; depth++ {
			for _, note := range []bool{false, true} {
				t.Run(fmt.Sprintf("headers%d/depth%d/note%t", 1+extra, depth, note), func(t *testing.T) {
					figure := imageFitDocument(t).Blocks[0]
					body := mergeParagraph("BODY")
					for i := 0; i < depth; i++ {
						body = cellTable([]Block{body})
					}
					table := Block{Kind: Table, Rows: []Row{{RepeatHeader: true, Cells: []Cell{{Blocks: []Block{figure}}}}, {Cells: []Cell{{Blocks: []Block{body}}}}}}
					for i := 0; i < extra; i++ {
						table.Rows = append(table.Rows[:1+i], append([]Row{{RepeatHeader: true, Cells: []Cell{{Blocks: []Block{mergeParagraph("EXTRA_HEADER")}}}}}, table.Rows[1+i:]...)...)
					}
					pageHeight := 200.0
					if extra > 0 {
						pageHeight = 300
					}
					d := &Document{Layout: Layout{Width: 300, Height: pageHeight, Margin: 20}, Blocks: []Block{table}}
					if note {
						d.Blocks = []Block{{Kind: Paragraph, Runs: []Run{{Text: "REF"}, {Note: &NoteReference{Kind: Footnote, ID: "n"}}}}}
						d.Notes = []Note{{Kind: Footnote, ID: "n", Blocks: []Block{table}}}
					}
					before := canonicalSF(t, d)
					got, data := renderPageBreakDocument(t, d)
					if !bytes.Equal(before, canonicalSF(t, d)) {
						t.Fatal("header fitting changed authored preferences")
					}
					want := 1
					if note {
						want = 2
						checkNoteTarget(t, data, 2, 2)
					}
					checkTokenPage(t, got, "BODY", want)
					if got.Metadata.PageCount != want {
						t.Fatal("header figure failed to share a legal body start")
					}
					images := 0
					for _, b := range got.Blocks {
						if b.Kind == Picture {
							images++
							sameImagePixels(t, figure.Image.Data, b.Image.Data)
							x0, y0, x1, y1 := quadBounds(*b.Source.Quad)
							if b.Source.Page != want || x0 < 20 || y0 < 20 || x1 > 280 || y1 > pageHeight-20 {
								t.Fatal("fitted header figure outside its actual page")
							}
						}
					}
					if images != 1 {
						t.Fatal("header figure duplicated/lost")
					}
				})
			}
		}
	}
}

func TestHeaderOnlyTablePaintsEachAuthoredRowOnce(t *testing.T) {
	figure := imageFitDocument(t).Blocks[0]
	d := &Document{Layout: Layout{Width: 300, Height: 200, Margin: 20}, Blocks: []Block{{Kind: Table, Rows: []Row{
		{RepeatHeader: true, Cells: []Cell{{Blocks: []Block{figure}}}},
		{RepeatHeader: true, Cells: []Cell{{Blocks: []Block{mergeParagraph("ONLY_A")}}}},
		{RepeatHeader: true, Cells: []Cell{{Blocks: []Block{mergeParagraph("ONLY_B")}}}},
	}}}}
	got, _ := renderPageBreakDocument(t, d)
	if got.Metadata.PageCount != 1 {
		t.Fatal("header-only table repeated/paginated unexpectedly")
	}
	text := strings.Join(strings.Fields(got.Text()), "")
	if strings.Count(text, "ONLY_A") != 1 || strings.Count(text, "ONLY_B") != 1 {
		t.Fatal("header-only content lost/duplicated")
	}
}
func mergeParagraph(text string) Block { return Block{Kind: Paragraph, Runs: []Run{{Text: text}}} }
func mergedTableExample() Block {
	b := Block{Kind: Table, Rows: []Row{
		{Cells: []Cell{
			{ColSpan: 2, RowSpan: 2, Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "Merged A", Bold: true, Link: "https://example.invalid/merge"}}}}},
			{RowSpan: 3, Blocks: []Block{mergeParagraph("Merged B")}},
			{Blocks: []Block{mergeParagraph("Right C")}},
		}},
		{Cells: []Cell{{Blocks: []Block{mergeParagraph("Right D")}}}},
		{Cells: []Cell{{ColSpan: 2, Blocks: []Block{mergeParagraph("Bottom E")}}, {Blocks: []Block{mergeParagraph("Right F")}}}},
		{Cells: []Cell{{ColSpan: 4, Blocks: []Block{mergeParagraph("Full G")}}}},
	}}
	for ri := range b.Rows {
		for ci := range b.Rows[ri].Cells {
			c := &b.Rows[ri].Cells[ci]
			c.ColSpan = max(1, c.ColSpan)
		}
	}
	return b
}

const mergedTableOOXML = `<w:tbl><w:tblGrid><w:gridCol/><w:gridCol/><w:gridCol/><w:gridCol/></w:tblGrid>
<w:tr><w:tc><w:tcPr><w:gridSpan w:val="2"/><w:vMerge w:val="restart"/></w:tcPr><w:p><w:r><w:t>Merged A</w:t></w:r></w:p></w:tc>
<w:tc><w:tcPr><w:vMerge w:val="restart"/></w:tcPr><w:p><w:r><w:t>Merged B</w:t></w:r></w:p></w:tc>
<w:tc><w:p><w:r><w:t>Right C</w:t></w:r></w:p></w:tc></w:tr>
<w:tr><w:tc><w:tcPr><w:gridSpan w:val="2"/><w:vMerge/></w:tcPr><w:p/></w:tc>
<w:tc><w:tcPr><w:vMerge w:val="continue"/></w:tcPr><w:p/></w:tc>
<w:tc><w:p><w:r><w:t>Right D</w:t></w:r></w:p></w:tc></w:tr>
<w:tr><w:tc><w:tcPr><w:gridSpan w:val="2"/></w:tcPr><w:p><w:r><w:t>Bottom E</w:t></w:r></w:p></w:tc>
<w:tc><w:tcPr><w:vMerge/></w:tcPr><w:p/></w:tc>
<w:tc><w:p><w:r><w:t>Right F</w:t></w:r></w:p></w:tc></w:tr>
<w:tr><w:tc><w:tcPr><w:gridSpan w:val="4"/></w:tcPr><w:p><w:r><w:t>Full G</w:t></w:r></w:p></w:tc></w:tr></w:tbl>`

func docxWithBody(t testing.TB, body string) []byte {
	t.Helper()
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	parts := map[string]string{
		"[Content_Types].xml": `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/></Types>`,
		"_rels/.rels":         `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="` + relNS + `/officeDocument" Target="word/document.xml"/></Relationships>`,
		"word/document.xml":   `<w:document xmlns:w="` + wordNS + `"><w:body>` + body + `</w:body></w:document>`,
	}
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
	return buf.Bytes()
}

func TestDOCXVerticalMergeImport(t *testing.T) {
	d, err := ReadDOCX(context.Background(), bytes.NewReader(docxWithBody(t, mergedTableOOXML)))
	if err != nil {
		t.Fatal(err)
	}
	b := d.Blocks[0]
	if len(b.Rows) != 4 || len(b.Rows[1].Cells) != 1 || len(b.Rows[2].Cells) != 2 {
		t.Fatalf("continuation cells not folded: %#v", b.Rows)
	}
	if b.Rows[0].Cells[0].RowSpan != 2 || b.Rows[0].Cells[0].ColSpan != 2 || b.Rows[0].Cells[1].RowSpan != 3 {
		t.Fatal("merge geometry lost")
	}
	for _, diag := range d.Diagnostics {
		if diag.Code == "vertical_merge" {
			t.Fatal("supported merge was diagnosed as lost")
		}
	}
	s, err := ToStructured(d)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(s.Content[0].Content[0].Content[0].Attrs), `"rowspan":2`) {
		t.Fatal("SF rowspan lost")
	}
	if len(s.Content[0].Content[1].Content) != 1 {
		t.Fatal("SF duplicated continuation cells")
	}
	for _, marker := range []string{"Merged A", "Merged B", "Right C", "Right D", "Bottom E", "Right F", "Full G"} {
		if strings.Count(d.Text(), marker) != 1 {
			t.Fatalf("lost or repeated %s", marker)
		}
	}

	full := `<w:tbl><w:tr><w:tc><w:tcPr><w:vMerge w:val="restart"/></w:tcPr><w:p><w:r><w:t>All rows</w:t></w:r></w:p></w:tc></w:tr><w:tr><w:tc><w:tcPr><w:vMerge/></w:tcPr><w:p/></w:tc></w:tr></w:tbl>`
	d, err = ReadDOCX(context.Background(), bytes.NewReader(docxWithBody(t, full)))
	if err != nil {
		t.Fatal(err)
	}
	if d.Blocks[0].Rows[0].Cells[0].RowSpan != 2 || len(d.Blocks[0].Rows[1].Cells) != 0 {
		t.Fatal("covered row lost")
	}
	for _, format := range []Format{SF, DOCX, PDF} {
		var out bytes.Buffer
		if err := Write(context.Background(), &out, d, format, WriteOptions{}); err != nil {
			t.Fatal(err)
		}
		decoded, err := Read(context.Background(), &out, format, ReadOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if strings.Count(decoded.Text(), "All rows") != 1 {
			t.Fatal("fully merged cell content duplicated or lost")
		}
		if format != PDF && (len(decoded.Blocks[0].Rows) != 2 || decoded.Blocks[0].Rows[0].Cells[0].RowSpan != 2 || len(decoded.Blocks[0].Rows[1].Cells) != 0) {
			t.Fatal("fully covered row changed on round trip")
		}
	}
}

func TestVerticalMergeRoundTrip(t *testing.T) {
	d := &Document{Blocks: []Block{mergedTableExample()}}
	s, err := ToStructured(d)
	if err != nil {
		t.Fatal(err)
	}
	shared, err := FromStructured(s)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(d, shared) {
		t.Fatal("SF merge/formatting/content changed")
	}
	var encoded bytes.Buffer
	if err = Write(context.Background(), &encoded, shared, DOCX, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	z, err := zip.NewReader(bytes.NewReader(encoded.Bytes()), int64(encoded.Len()))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range z.File {
		if f.Name == "word/document.xml" {
			rc, err := f.Open()
			if err != nil {
				t.Fatal(err)
			}
			xml, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				t.Fatal(err)
			}
			if strings.Count(string(xml), `<w:vMerge w:val="restart"/>`) != 2 || strings.Count(string(xml), `<w:vMerge/>`) != 3 {
				t.Fatalf("incorrect OOXML merge markers: %s", xml)
			}
			for _, m := range []string{"Merged A", "Merged B"} {
				if strings.Count(string(xml), m) != 1 {
					t.Fatal("origin content duplicated")
				}
			}
		}
	}
	decoded, err := ReadDOCX(context.Background(), &encoded)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := ToStructured(decoded)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(s.Content, actual.Content) {
		t.Fatal("DOCX merge/formatting/relationship round trip changed")
	}
}

func TestMergeContinuationContentAndInvalidDOCX(t *testing.T) {
	extra := strings.Replace(mergedTableOOXML, `<w:vMerge/></w:tcPr><w:p/>`, `<w:vMerge/></w:tcPr><w:p><w:r><w:t>Continuation text</w:t></w:r></w:p>`, 1)
	d, err := ReadDOCX(context.Background(), bytes.NewReader(docxWithBody(t, extra)))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(d.Blocks[0].Rows[0].Cells[0].Blocks[1].Runs[0].Text, "Continuation text") || len(d.Diagnostics) == 0 || d.Diagnostics[0].Code != "merge_continuation_content" {
		t.Fatal("continuation content silently discarded")
	}
	closed := `<w:tbl><w:tr><w:tc><w:tcPr><w:vMerge w:val="restart"/></w:tcPr><w:p/></w:tc></w:tr><w:tr><w:tc><w:p/></w:tc></w:tr><w:tr><w:tc><w:tcPr><w:vMerge/></w:tcPr><w:p/></w:tc></w:tr></w:tbl>`
	for _, bad := range []string{closed,
		strings.Replace(mergedTableOOXML, `w:val="restart"`, `w:val="continue"`, 1),
		strings.Replace(mergedTableOOXML, `w:val="restart"`, `w:val="bogus"`, 1),
		strings.Replace(mergedTableOOXML, `<w:gridSpan w:val="2"/><w:vMerge/>`, `<w:gridSpan w:val="1"/><w:vMerge/>`, 1),
		strings.Replace(mergedTableOOXML, `<w:gridSpan w:val="2"/>`, `<w:gridSpan w:val="0"/>`, 1),
	} {
		if _, err := ReadDOCX(context.Background(), bytes.NewReader(docxWithBody(t, bad))); err == nil {
			t.Fatal("invalid DOCX merge accepted")
		}
	}
}

func TestTableGridRejectsInvalidSpans(t *testing.T) {
	for _, rows := range [][]Row{
		{{Cells: []Cell{{RowSpan: 2}}}},
		{{Cells: []Cell{{RowSpan: -1}}}},
		{{Cells: []Cell{{ColSpan: 1001}}}},
		{{Cells: []Cell{{}, {RowSpan: 2}}}, {Cells: []Cell{{ColSpan: 2}}}},
		{{Cells: []Cell{{}, {RowSpan: 2}}}, {}},
	} {
		if err := (&Document{Blocks: []Block{{Kind: Table, Rows: rows}}}).Validate(); err == nil {
			t.Fatal("invalid table accepted")
		}
	}
}

func TestPDFMergedCellGeometryAndPagination(t *testing.T) {
	ctx := context.Background()
	d := &Document{Layout: Layout{Width: 360, Height: 300, Margin: 30}, Blocks: []Block{mergedTableExample()}}
	var out bytes.Buffer
	if err := Write(ctx, &out, d, PDF, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	p, err := pdfcpu.ReadWithContext(ctx, bytes.NewReader(out.Bytes()), &model.Configuration{Reader15: true, ValidationMode: model.ValidationStrict, Offline: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := p.EnsurePageCount(); err != nil {
		t.Fatal(err)
	}
	pd, _, _, err := p.PageDict(1, false)
	if err != nil {
		t.Fatal(err)
	}
	data, err := p.PageContent(pd, 1)
	if err != nil {
		t.Fatal(err)
	}
	stream := string(data)
	var operands []any
	var rects [][]float64
	for stream != "" {
		token, err := nextPDFToken(ctx, &stream)
		if err != nil {
			t.Fatal(err)
		}
		if token == nil {
			break
		}
		if op, ok := token.(pdfOperator); ok {
			if op == "re" {
				values := make([]float64, len(operands))
				for i, v := range operands {
					var err error
					values[i], err = pdfNumber(v)
					if err != nil {
						t.Fatal(err)
					}
				}
				rects = append(rects, values)
			}
			operands = nil
		} else {
			operands = append(operands, token)
		}
	}
	want := [][]float64{{30, 270, 150, -52}, {180, 270, 75, -78}, {255, 270, 75, -26}, {255, 244, 75, -26}, {30, 218, 150, -26}, {255, 218, 75, -26}, {30, 192, 300, -26}}
	if len(rects) != len(want) {
		t.Fatalf("got %d rectangles, want 7", len(rects))
	}
	for i := range want {
		for j, v := range want[i] {
			if math.Abs(rects[i][j]-v) > .01 {
				t.Fatalf("rectangle %d: got %v, want %v", i, rects[i], want[i])
			}
		}
	}

	d.Layout = Layout{Width: 360, Height: 180, Margin: 20}
	d.Blocks = append([]Block{mergeParagraph(strings.Repeat("Preface\n", 5))}, d.Blocks...)
	out.Reset()
	if err := Write(ctx, &out, d, PDF, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	decoded, err := ReadPDF(ctx, &out, ReadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Metadata.PageCount != 2 {
		t.Fatalf("unexpected pagination: %d", decoded.Metadata.PageCount)
	}
	for _, m := range []string{"Merged A", "Merged B", "Right C", "Right D", "Bottom E", "Right F", "Full G"} {
		if strings.Count(decoded.Text(), m) != 1 {
			t.Fatalf("lost/repeated text across pages: %s", m)
		}
	}
	d.Layout.Height = 90
	out.Reset()
	if err := Write(ctx, &out, d, PDF, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	decoded, err = ReadPDF(ctx, &out, ReadOptions{PDFSkipInference: true})
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Metadata.PageCount < 3 {
		t.Fatal("oversized group did not continue")
	}
	for _, m := range []string{"Merged A", "Merged B", "Right C", "Right D", "Bottom E", "Right F", "Full G"} {
		if strings.Count(decoded.Text(), m) != 1 {
			t.Fatalf("lost/repeated continued-cell text: %s", m)
		}
	}
}

func TestDOCXTableWidthsStayInsideContainer(t *testing.T) {
	nested := Block{Kind: Table, Rows: []Row{{Cells: []Cell{{Blocks: []Block{mergeParagraph("Nested left")}}, {Blocks: []Block{mergeParagraph("Nested right")}}}}}}
	outer := Block{Kind: Table, Rows: []Row{{Cells: []Cell{{Blocks: []Block{nested}}, {Blocks: []Block{mergeParagraph("Outer right")}}}}}}
	d := &Document{Layout: Layout{Width: 360, Height: 300, Margin: 30}, Blocks: []Block{outer}}
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, DOCX, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	z, err := zip.NewReader(bytes.NewReader(out.Bytes()), int64(out.Len()))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range z.File {
		if f.Name == "word/document.xml" {
			rc, err := f.Open()
			if err != nil {
				t.Fatal(err)
			}
			raw, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				t.Fatal(err)
			}
			root, err := parseXML(context.Background(), raw)
			if err != nil {
				t.Fatal(err)
			}
			tables := root.all("tbl")
			if len(tables) != 2 {
				t.Fatal("nested table lost")
			}
			for i, want := range []string{"6000", "2840"} {
				if tables[i].child("tblPr").child("tblW").attr("w") != want {
					t.Fatalf("table %d overflows available container: %s", i, raw)
				}
				total := 0.0
				for _, col := range tables[i].child("tblGrid").Children {
					total += number(col.attr("w"))
				}
				if total != number(want) {
					t.Fatal("grid width does not match available width")
				}
			}
		}
	}
}
func TestTableWidthQuantizationRoundtrip(t *testing.T) {
	rng := rand.New(rand.NewSource(729))
	for i := 0; i < 120; i++ {
		scale := 900.0
		if i%2 == 0 {
			scale = 60
		}
		d := &Document{Layout: Layout{Width: 400, Height: 500, Margin: 40}, Blocks: []Block{widthTable(10+rng.Float64()*scale, 10+rng.Float64()*scale, 10+rng.Float64()*scale)}}
		var data bytes.Buffer
		if err := Write(context.Background(), &data, d, DOCX, WriteOptions{}); err != nil {
			t.Fatal(err)
		}
		back, err := ReadDOCX(context.Background(), &data)
		if err != nil {
			t.Fatal(i, d.Blocks[0].ColumnWidths, err)
		}
		if !reflect.DeepEqual(back.Blocks[0].ColumnWidths, d.Blocks[0].ColumnWidths) {
			t.Fatal("preferred widths rounded")
		}
	}
}

func widthTable(widths ...float64) Block {
	return Block{Kind: Table, ColumnWidths: widths, Rows: []Row{
		{RepeatHeader: true, Cells: []Cell{{ColSpan: 2, Blocks: []Block{mergeParagraph("HEADER")}}, {Blocks: []Block{mergeParagraph("END")}}}},
		{Cells: []Cell{{RowSpan: 2, Blocks: []Block{mergeParagraph("LEFT")}}, {Blocks: []Block{mergeParagraph("DESCRIPTION")}}, {Blocks: []Block{mergeParagraph("VALUE")}}}},
		{Cells: []Cell{{ColSpan: 2, Blocks: []Block{mergeParagraph("BOTTOM")}}}},
	}}
}

func TestTableWidthsSFAndDOCXRoundtrip(t *testing.T) {
	for _, widths := range [][]float64{nil, {45, 180, 75}, {91.123, 360.456, 150.789}, {.8, 179.35, 27.4}} {
		d := &Document{Layout: Layout{Width: 400, Height: 500, Margin: 50}, Blocks: []Block{widthTable(widths...)}}
		for _, format := range []Format{SF, DOCX, JSON} {
			var data bytes.Buffer
			if err := Write(context.Background(), &data, d, format, WriteOptions{}); err != nil {
				t.Fatal(widths, format, err)
			}
			back, err := Read(context.Background(), &data, format, ReadOptions{})
			if err != nil {
				t.Fatal(widths, format, err)
			}
			if !reflect.DeepEqual(back.Blocks[0].ColumnWidths, widths) {
				t.Fatalf("%s preferred widths changed: %v != %v", format, back.Blocks[0].ColumnWidths, widths)
			}
			if len(back.Diagnostics) != 0 {
				t.Fatal(back.Diagnostics)
			}
		}
	}
}

func TestTableWidthAdmission(t *testing.T) {
	for _, widths := range [][]float64{{1}, {1, 0, 2}, {1, -1, 2}, {1, math.NaN(), 2}, {1, math.Inf(1), 2}, {1, 1e8, 2}} {
		d := &Document{Blocks: []Block{widthTable(widths...)}}
		var out bytes.Buffer
		if err := Write(context.Background(), &out, d, SF, WriteOptions{}); err == nil || out.Len() != 0 {
			t.Fatal("invalid widths admitted", widths, err)
		}
	}
	d := &Document{Blocks: []Block{{Kind: Paragraph, ColumnWidths: []float64{1}}}}
	if _, err := ToStructured(d); err == nil {
		t.Fatal("non-table widths admitted")
	}

	d.Blocks = []Block{widthTable(.001, 200, 30)}
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, DOCX, WriteOptions{}); err == nil || out.Len() != 0 {
		t.Fatal("sub-twip width silently rounded away")
	}
}

func TestDOCXWidthMetadataChecked(t *testing.T) {
	for _, attrs := range []string{`md:columnWidths="[10,20,30]"`, `md:columnWidths="[90,180,30]"`, `md:columnWidths="[0,180,75]"`} {
		xml := `<w:tbl xmlns:md="` + docxNotesNS + `" ` + attrs + ` md:columnWidthContainer="300"><w:tblGrid><w:gridCol w:w="900"/><w:gridCol w:w="3600"/><w:gridCol w:w="1500"/></w:tblGrid><w:tr><w:tc/><w:tc/><w:tc/></w:tr></w:tbl>`
		n, err := parseXML(context.Background(), []byte(xml))
		if err != nil {
			t.Fatal(err)
		}
		r := docxReader{ctx: context.Background(), doc: &Document{}}
		if _, err = r.table(n, nil, 0); err == nil {
			t.Fatal("contradictory metadata accepted", attrs)
		}
	}
}

func TestRealWordTableWidthGrid(t *testing.T) {
	raw, err := os.ReadFile("testdata/real/mqtt-v5.0-os.docx")
	if err != nil {
		t.Fatal(err)
	}
	d, err := ReadDOCX(context.Background(), bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	var tables []Block
	var walk func([]Block)
	walk = func(bs []Block) {
		for _, b := range bs {
			if b.Kind == Table {
				tables = append(tables, b)
			}
			if b.Section != nil {
				walk(b.Section.Blocks)
			}
			for _, row := range b.Rows {
				for _, c := range row.Cells {
					walk(c.Blocks)
				}
			}
		}
	}
	walk(d.Blocks)

	want := []float64{77.15, 179.35, 1, 25.05, 26.1, .95, 25.15, .9, 25.2, .85, 25.25, .8, 25.3, .75, 25.35, .95, 27.4}
	if len(tables) != 59 || !reflect.DeepEqual(tables[51].ColumnWidths, want) {
		t.Fatalf("source physical grid changed: %v", tables[51].ColumnWidths)
	}
	widths, err := fittedColumnWidths(tables[51].ColumnWidths, 17, 468)
	if err != nil {
		t.Fatal(err)
	}
	if widths[1]+widths[2] < 180 {
		t.Fatal("description cell narrowed to equal logical columns")
	}
}

func TestPDFTableWidthsCarrySeparateProvenance(t *testing.T) {
	d := readRecovery(t, taggedSpanPDF())
	b := d.Blocks[0]
	if !reflect.DeepEqual(b.ColumnWidths, []float64{80, 80, 80}) || b.Source.Structure != "explicit" || b.Source.TableWidths == nil || b.Source.TableWidths.Evidence != "painted_grid_adjacency" {
		t.Fatal("physical width evidence conflated with explicit membership", b.ColumnWidths, b.Source)
	}
	assertRecoverySF(t, d)

	d = readRecovery(t, recoveryPDF(taggedSpanObjects("")...))
	if len(d.Blocks[0].ColumnWidths) != 0 || d.Blocks[0].Source.TableWidths != nil {
		t.Fatal("widths invented without painted grid")
	}
}
