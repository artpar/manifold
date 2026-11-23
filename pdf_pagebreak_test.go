package manydoc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

func pageBreakDocument(t testing.TB) *Document {
	t.Helper()
	data, err := os.ReadFile("examples/page-breaks/document.sf")
	if err != nil {
		t.Fatal(err)
	}
	s, err := ReadStructured(context.Background(), bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	d, err := FromStructured(s)
	if err != nil {
		t.Fatal(err)
	}
	return d
}
func renderPageBreakDocument(t testing.TB, d *Document) (*Document, []byte) {
	t.Helper()
	before, _ := json.Marshal(d)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var out bytes.Buffer
	if err := Write(ctx, &out, d, PDF, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(d)
	if !bytes.Equal(before, after) {
		t.Fatal("break planning mutated source")
	}
	got, err := ReadPDF(ctx, bytes.NewReader(out.Bytes()), ReadOptions{PDFSkipInference: true})
	if err != nil {
		t.Fatal(err)
	}
	return got, out.Bytes()
}
func checkTokenPage(t testing.TB, d *Document, token string, want int) {
	t.Helper()
	found := 0
	for _, b := range d.Blocks {
		for _, run := range b.Runs {
			if run.Text == token {
				found++
				if run.Source.Page != want {
					t.Fatalf("%s on page %d, want %d", token, run.Source.Page, want)
				}
			}
		}
	}
	if found != 1 {
		t.Fatalf("%s occurrences=%d", token, found)
	}
}
func cellTable(blocks ...[]Block) Block {
	row := Row{}
	for _, flow := range blocks {
		row.Cells = append(row.Cells, Cell{Blocks: flow})
	}
	return Block{Kind: Table, Rows: []Row{row}}
}

func TestPDFParallelCellPageBreaks(t *testing.T) {
	for _, kind := range []string{"single", "simultaneous", "staggered"} {
		t.Run(kind, func(t *testing.T) {
			left := []Block{mergeParagraph("LEFT_BEFORE"), {Kind: PageBreak}, mergeParagraph("LEFT_AFTER")}
			right := []Block{mergeParagraph("RIGHT_ZERO"), mergeParagraph("RIGHT_ONE"), mergeParagraph("RIGHT_TWO")}
			wantPages := 2
			if kind == "simultaneous" {
				right = []Block{right[0], {Kind: PageBreak}, right[1], right[2]}
			}
			if kind == "staggered" {
				right = []Block{right[0], right[1], {Kind: PageBreak}, right[2]}
				wantPages = 3
			}
			d := &Document{Layout: Layout{Width: 480, Height: 600, Margin: 20}, Blocks: []Block{cellTable(left, right)}}
			got, _ := renderPageBreakDocument(t, d)
			if got.Metadata.PageCount != wantPages {
				t.Fatalf("pages=%d want%d", got.Metadata.PageCount, wantPages)
			}
			checkTokenPage(t, got, "LEFT_BEFORE", 1)
			checkTokenPage(t, got, "LEFT_AFTER", 2)
			checkTokenPage(t, got, "RIGHT_ZERO", 1)
			checkTokenPage(t, got, "RIGHT_ONE", 2)
			checkTokenPage(t, got, "RIGHT_TWO", wantPages)
		})
	}
}

func TestPDFCellLeadingConsecutiveTrailingBreaks(t *testing.T) {
	for depth := 0; depth < 4; depth++ {
		t.Run(fmt.Sprint(depth), func(t *testing.T) {
			b := cellTable([]Block{{Kind: PageBreak}, {Kind: PageBreak}, mergeParagraph("CELL_VISIBLE"), {Kind: PageBreak}})
			for n := 0; n < depth; n++ {
				b = cellTable([]Block{b})
			}
			d := &Document{Layout: Layout{Width: 480, Height: 600, Margin: 20}, Blocks: []Block{b}}
			got, _ := renderPageBreakDocument(t, d)
			if got.Metadata.PageCount != 4 {
				t.Fatalf("break page count depth%d got%d", depth, got.Metadata.PageCount)
			}
			checkTokenPage(t, got, "CELL_VISIBLE", 3)
		})
	}
}

func checkNoteTarget(t testing.TB, data []byte, wants ...int) {
	t.Helper()
	ctx := context.Background()
	p, err := pdfcpu.ReadWithContext(ctx, bytes.NewReader(data), &model.Configuration{Reader15: true, ValidationMode: model.ValidationStrict, Offline: true})
	if err != nil {
		t.Fatal(err)
	}
	if err = p.EnsurePageCount(); err != nil {
		t.Fatal(err)
	}
	first, _, _, err := p.PageDict(1, false)
	if err != nil {
		t.Fatal(err)
	}
	targets := []types.IndirectRef{}
	for _, want := range wants {
		_, ref, _, err := p.PageDict(want, false)
		if err != nil {
			t.Fatal(err)
		}
		targets = append(targets, *ref)
	}
	annots, err := p.DereferenceArray(first["Annots"])
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	for _, a := range annots {
		dict, err := p.DereferenceDict(a)
		if err != nil {
			t.Fatal(err)
		}
		if dict["Dest"] == nil {
			continue
		}
		dest, err := p.DereferenceArray(dict["Dest"])
		if err != nil {
			t.Fatal(err)
		}
		ref, ok := dest[0].(types.IndirectRef)
		if !ok || found >= len(targets) || ref != targets[found] {
			t.Fatalf("wrong note destination %v; wanted pages%v", dest, wants)
		}
		found++
	}
	if found != len(wants) {
		t.Fatal("missing note destination")
	}
}

func TestPDFFootnoteLeadingConsecutiveTrailingBreaks(t *testing.T) {
	for _, empty := range []bool{false, true} {
		for _, resume := range []bool{false, true} {
			t.Run(fmt.Sprintf("empty%t_resume%t", empty, resume), func(t *testing.T) {
				flow := []Block{{Kind: PageBreak}, {Kind: PageBreak}}
				wantPages := 3
				if !empty {
					flow = append(flow, mergeParagraph("NOTE_VISIBLE"), Block{Kind: PageBreak})
					wantPages = 4
				}
				d := &Document{Layout: Layout{Width: 480, Height: 600, Margin: 20}, Notes: []Note{{Kind: Footnote, ID: "n", Blocks: flow}}, Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "REFERENCE"}, noteRun(Footnote, "n")}}}}
				if resume {
					d.Blocks = append(d.Blocks, Block{Kind: PageBreak}, mergeParagraph("BODY_RESUMED"))
				}
				got, data := renderPageBreakDocument(t, d)
				if got.Metadata.PageCount != wantPages {
					t.Fatalf("footnote pages=%d want%d", got.Metadata.PageCount, wantPages)
				}
				if !empty {
					checkTokenPage(t, got, "NOTE_VISIBLE", 3)
				}
				if resume {
					checkTokenPage(t, got, "BODY_RESUMED", wantPages)
				}
				checkNoteTarget(t, data, 3)
			})
		}
	}
}

func TestPDFNestedCellAndNotePageBreaks(t *testing.T) {
	d := pageBreakDocument(t)
	got, _ := renderPageBreakDocument(t, d)
	if got.Metadata.PageCount != 5 {
		t.Fatalf("nested breaks: pages%d", got.Metadata.PageCount)
	}
	for token, page := range map[string]int{"OUTER_BEFORE": 1, "INNER_BEFORE": 1, "INNER_AFTER": 2, "OUTER_AFTER": 2, "INNER_SIDE_B": 2, "NOTE_BEFORE": 2, "NOTE_AFTER": 3, "NOTE_INNER_BEFORE": 3, "NOTE_INNER_AFTER": 4, "NOTE_END": 4, "BODY_AFTER": 5} {
		checkTokenPage(t, got, token, page)
	}
	images := 0
	for _, b := range got.Blocks {
		if b.Kind == Picture {
			images++
			if b.Source.Page != 2 && b.Source.Page != 4 {
				t.Fatal("break misplaced image")
			}
		}
	}
	if images != 2 {
		t.Fatal("break lost or repeated asset")
	}
}

func countBlockBreaks(bs []Block) int {
	n := 0
	for _, b := range bs {
		if b.Kind == PageBreak {
			n++
		}
		for _, row := range b.Rows {
			for _, c := range row.Cells {
				n += countBlockBreaks(c.Blocks)
			}
		}
		for _, item := range b.Items {
			n += countBlockBreaks(item.Blocks)
		}
	}
	return n
}
func TestPageBreakSFDOCXPreservesStoryPlacement(t *testing.T) {
	d := pageBreakDocument(t)
	for _, format := range []Format{SF, DOCX} {
		var out bytes.Buffer
		if err := Write(context.Background(), &out, d, format, WriteOptions{}); err != nil {
			t.Fatal(err)
		}
		got, err := Read(context.Background(), &out, format, ReadOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if countBlockBreaks(got.Blocks) != 2 || countBlockBreaks(got.Notes[0].Blocks) != 2 {
			t.Fatalf("lost break ownership in%s", format)
		}
		inner := got.Blocks[0].Rows[0].Cells[0].Blocks[1].Rows[0].Cells[0]
		if inner.RowSpan != 2 || inner.Blocks[1].Kind != PageBreak || inner.Blocks[2].Runs[1].Note.ID != "broken" {
			t.Fatal("cell break/note/span placement changed")
		}
	}
}

func TestPDFRichLeadingCellBreakPreservesContent(t *testing.T) {
	d := richCellDocument(t)
	d.Blocks[1].Rows[0].Cells[0].Blocks = append([]Block{{Kind: PageBreak}}, d.Blocks[1].Rows[0].Cells[0].Blocks...)
	got, _ := renderPageBreakDocument(t, d)
	for _, token := range []string{"CELL_START", "CELL_END", "INNER_A", "INNER_B", "INNER_C", "TOP_RIGHT", "BOTTOM_RIGHT", "AFTER_TABLE", "NOTE_CELL", "NOTE_NESTED"} {
		if strings.Count(strings.Join(strings.Fields(got.Text()), ""), token) != 1 {
			t.Fatalf("lost rich content after break: %s", token)
		}
	}
	pages := map[string]int{}
	for _, b := range got.Blocks {
		for _, run := range b.Runs {
			for _, token := range []string{"PREFACE", "CELL_START"} {
				if strings.Contains(run.Text, token) {
					pages[token] = run.Source.Page
				}
			}
		}
	}
	if pages["CELL_START"] <= pages["PREFACE"] {
		t.Fatal("leading cell break not honored")
	}
}

func tokenLocations(d *Document) map[string]pdfLocation {
	locations := map[string]pdfLocation{}
	for _, b := range d.Blocks {
		for _, run := range b.Runs {
			if run.Source != nil && run.Source.Quad != nil {
				x, y, _, _ := quadBounds(*run.Source.Quad)
				locations[run.Text] = pdfLocation{run.Source.Page, x, y}
			}
		}
	}
	return locations
}

func FuzzPDFCellPageBreaks(f *testing.F) {
	f.Add(byte(0), byte(2), byte(1), byte(2), byte(2), uint16(0))
	f.Add(byte(6), byte(6), byte(0), byte(0), byte(0), uint16(150))
	f.Fuzz(func(t *testing.T, lp, rp, lc, rc, depth byte, h uint16) {
		positions := []int{int(lp % 7), int(rp % 7)}
		counts := []int{1 + int(lc%3), 1 + int(rc%3)}
		flows := [][]Block{}
		for side, prefix := range []string{"LF_", "RX_"} {
			flow := []Block{}
			for i := 0; i <= 6; i++ {
				if i == positions[side] {
					for n := 0; n < counts[side]; n++ {
						flow = append(flow, Block{Kind: PageBreak})
					}
				}
				if i < 6 {
					flow = append(flow, mergeParagraph(fmt.Sprintf("%s%02d", prefix, i)))
				}
			}
			flows = append(flows, flow)
		}
		b := cellTable(flows...)
		for i := 0; i < int(depth%3); i++ {
			b = cellTable([]Block{b})
		}
		d := &Document{Layout: Layout{Width: 480, Height: 300 + float64(h%300), Margin: 20}, Blocks: []Block{b}}
		got, _ := renderPageBreakDocument(t, d)
		locations := tokenLocations(got)
		for side, prefix := range []string{"LF_", "RX_"} {
			var previous pdfLocation
			for i := 0; i < 6; i++ {
				token := fmt.Sprintf("%s%02d", prefix, i)
				loc, ok := locations[token]
				if !ok || strings.Count(got.Text(), token) != 1 {
					t.Fatalf("break lost/duplicated %s", token)
				}
				if loc.page < previous.page || loc.page == previous.page && loc.y <= previous.y {
					t.Fatal("break reordered cell")
				}
				if i == positions[side] && loc.page-previous.page < counts[side] {
					t.Fatalf("coalesced consecutive breaks in one cell: %v %v count%d", previous, loc, counts[side])
				}
				if positions[side] == 0 && i == 0 && loc.page < 1+counts[side] {
					t.Fatal("leading breaks lost")
				}
				if i >= min(positions[0], positions[1]) && loc.page < 2 {
					t.Fatal("sibling crossed first forced boundary")
				}
				previous = loc
			}
			if positions[side] == 6 && got.Metadata.PageCount < previous.page+counts[side] {
				t.Fatal("trailing breaks lost")
			}
		}
	})
}

func FuzzPDFFootnotePageBreaks(f *testing.F) {
	f.Add(byte(0), byte(2), byte(0), byte(0), true)
	f.Add(byte(2), byte(0), byte(2), byte(2), false)
	f.Fuzz(func(t *testing.T, leading, position, consecutive, trailing byte, resume bool) {
		lead, pos, count, trail := int(leading%3), int(position%7), 1+int(consecutive%3), int(trailing%3)
		flow := []Block{}
		for n := 0; n < lead; n++ {
			flow = append(flow, Block{Kind: PageBreak})
		}
		for i := 0; i <= 6; i++ {
			if i == pos {
				for n := 0; n < count; n++ {
					flow = append(flow, Block{Kind: PageBreak})
				}
			}
			if i < 6 {
				flow = append(flow, mergeParagraph(fmt.Sprintf("FN_%02d", i)))
			}
		}
		for n := 0; n < trail; n++ {
			flow = append(flow, Block{Kind: PageBreak})
		}
		d := &Document{Layout: Layout{Width: 480, Height: 600, Margin: 20}, Notes: []Note{{Kind: Footnote, ID: "n", Blocks: flow}}, Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "REF"}, noteRun(Footnote, "n")}}}}
		if resume {
			d.Blocks = append(d.Blocks, Block{Kind: PageBreak}, mergeParagraph("RESUMED"))
		}
		got, data := renderPageBreakDocument(t, d)
		wantPages := 1 + lead + count + trail
		// A final break already creates the fresh page on which body resumes.
		if resume && trail == 0 && pos < 6 {
			wantPages++
		}
		if got.Metadata.PageCount != wantPages {
			t.Fatalf("footnote page breaks got%d want%d", got.Metadata.PageCount, wantPages)
		}
		for i := 0; i < 6; i++ {
			want := 1 + lead
			if i >= pos {
				want += count
			}
			checkTokenPage(t, got, fmt.Sprintf("FN_%02d", i), want)
		}
		target := 1 + lead
		if pos == 0 {
			target += count
		}
		checkNoteTarget(t, data, target)
		if resume {
			checkTokenPage(t, got, "RESUMED", wantPages)
		}
	})
}

func TestPDFEmptyBreakNoteNeedsNoSeparatorHeight(t *testing.T) {
	d := &Document{Layout: Layout{Width: 200, Height: 60, Margin: 20}, Notes: []Note{{Kind: Footnote, ID: "n", Blocks: []Block{{Kind: PageBreak}, {Kind: PageBreak}}}}, Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "REF"}, noteRun(Footnote, "n")}}}}
	got, data := renderPageBreakDocument(t, d)
	if got.Metadata.PageCount != 3 {
		t.Fatalf("empty note break pages%d", got.Metadata.PageCount)
	}
	checkNoteTarget(t, data, 3)
}

func TestPDFLeadingBreakNoteAlongsideVisibleNote(t *testing.T) {
	d := &Document{Layout: Layout{Width: 480, Height: 240, Margin: 20}, Notes: []Note{{Kind: Footnote, ID: "moved", Blocks: []Block{{Kind: PageBreak}, mergeParagraph("NOTE_MOVED")}}, {Kind: Footnote, ID: "same", Blocks: []Block{mergeParagraph("NOTE_STAYS")}}}, Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "REF"}, noteRun(Footnote, "moved"), noteRun(Footnote, "same")}}}}
	got, data := renderPageBreakDocument(t, d)
	if got.Metadata.PageCount != 2 {
		t.Fatalf("mixed note pages%d", got.Metadata.PageCount)
	}
	checkTokenPage(t, got, "NOTE_MOVED", 2)
	checkTokenPage(t, got, "NOTE_STAYS", 1)
	checkNoteTarget(t, data, 2, 1)
}
