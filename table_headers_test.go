package manifold

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
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
	if strings.Count(strings.Join(strings.Fields(got.Text()), ""), "HEADER") != 2 {
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
		t.Fatalf("ambiguous repeat merge lost content/diagnostic: %+v", d)
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
