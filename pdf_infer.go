package manydoc

// Geometric inference is deliberately separate from tagged-tree interpretation.
// No geometric rule establishes authorial semantics. Source evidence and the
// document diagnostic disclose the heuristics, and original run geometry stays
// available after grouping/reordering.
import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/rivo/uniseg"
)

type pdfLayoutBox struct {
	block                      Block
	x0, y0, x1, y1, size, base float64
	column                     string
	tableBarrier               bool
}

func inferredPDFSource(bs []Block, evidence string, confidence float64) *SourceInfo {
	s := sourceFromBlocks(bs, &pdfStructureNode{})
	s.Structure, s.Evidence, s.Confidence = "inferred", evidence, confidence
	return s
}

func pdfLayoutBlock(b Block) (pdfLayoutBox, bool) {
	if b.Source == nil || b.Source.Quad == nil {
		return pdfLayoutBox{}, false
	}
	x0, y0, x1, y1 := quadBounds(*b.Source.Quad)
	size, base := b.Source.FontSize, y1
	if len(b.Runs) > 0 && b.Runs[0].Source != nil {
		size = b.Runs[0].Source.FontSize
		if baseline := b.Runs[0].Source.Baseline; baseline != nil {
			base = baseline[0].Y
		}
	}
	if size <= 0 {
		size = 10
	}
	return pdfLayoutBox{block: b, x0: x0, y0: y0, x1: x1, y1: y1, size: size, base: base}, true
}

func (r *pdfReader) inferStructure() error {
	var out []Block
	pages := map[int][]Block{}
	pageNumber := 1
	for _, b := range r.doc.Blocks {
		if b.Kind == PageBreak {
			pageNumber++
			continue
		}
		pages[pageNumber] = append(pages[pageNumber], b)
	}
	for _, page := range r.doc.Pages {
		if err := r.ctx.Err(); err != nil {
			return err
		}
		originals := pages[page.Number]
		if page.Number > 1 {
			out = append(out, Block{Kind: PageBreak})
		}
		var boxes []pdfLayoutBox
		eligible := true
		for _, b := range originals {
			if b.Kind == Picture {
				box, ok := pdfLayoutBlock(b)
				if !ok {
					eligible = false
					break
				}
				boxes = append(boxes, box)
				continue
			}
			for _, run := range b.Runs {
				if run.Source != nil && run.Source.Evidence == "positioned_word_gap" {
					continue
				}
				box, ok := pdfLayoutBlock(Block{Kind: Paragraph, Runs: []Run{run}, Source: run.Source})
				if !ok {
					eligible = false
					break
				}
				if baseline := run.Source.Baseline; baseline != nil {
					dx, dy := baseline[1].X-baseline[0].X, baseline[1].Y-baseline[0].Y
					if dx < -0.01 || math.Abs(dy) > max(.1, math.Abs(dx)*.02) {
						eligible = false
						break
					}
				} else if strings.HasSuffix(run.Source.Precision, "replacement_span_bounds") {
					// A noncollinear replacement has bounds, but no established
					// logical baseline. Keep paint order instead of sorting it by
					// its bounding-box bottom as though it were an ordinary line.
					eligible = false
					break
				}
				for _, ch := range run.Text {
					if unicode.Is(unicode.Arabic, ch) || unicode.Is(unicode.Hebrew, ch) {
						eligible = false
						break
					}
				}
				boxes = append(boxes, box)
			}
		}
		if !eligible {
			r.doc.warn("pdf_inference_uncertain", "A page has missing geometry, nonhorizontal text or bidirectional text; paint order and original line groups are retained rather than assigning uncertain geometric structure")
			out = append(out, originals...)
			continue
		}
		em := pdfBodySize(boxes)
		for i, image := range boxes {
			if image.block.Kind != Picture {
				continue
			}
			for j, other := range boxes {
				if i != j && min(image.x1, other.x1) > max(image.x0, other.x0) && min(image.y1, other.y1) > max(image.y0, other.y0) {
					r.doc.warn("pdf_inference_uncertain", "Image/content regions overlap; both are retained and their geometric reading order is uncertain")
					break
				}
			}
		}
		boxes = r.inferRuledTables(boxes, page.Number, em)
		boxes = r.inferTables(boxes, em)
		boxes = r.layoutOrder(boxes, em, "", 0)
		if err := r.ctx.Err(); err != nil {
			return err
		}
		lines := pdfLayoutLines(boxes)
		r.inferHeadings(lines, em)
		out = append(out, r.inferFlow(lines)...)
		if err := r.ctx.Err(); err != nil {
			return err
		}
	}
	r.doc.Blocks = out
	r.doc.warn("pdf_structure_inferred", "Untagged reading order, paragraphs, typographic heading ranks, marker lists and aligned/painted-grid tables are geometric inferences, not explicit semantics; ambiguous layouts and original authorial boundaries may differ")
	return nil
}

func pdfBodySize(boxes []pdfLayoutBox) float64 {
	weights := map[int]int{}
	for _, box := range boxes {
		for _, run := range box.block.Runs {
			if run.Source != nil && run.Source.FontSize > 0 {
				weights[int(math.Round(run.Source.FontSize*10))] += uniseg.GraphemeClusterCount(strings.TrimSpace(run.Text))
			}
		}
	}
	best, weight := 100, 0
	for size, w := range weights {
		if w > weight || w == weight && size < best {
			best, weight = size, w
		}
	}
	return float64(best) / 10
}

// A bounding-box whitespace cut, independently authored from the published
// recursive X-Y cut idea. Wide vertical gutters precede horizontal bands, so
// a full-width title can be separated before columns underneath are ordered.
func (r *pdfReader) layoutOrder(boxes []pdfLayoutBox, em float64, column string, depth int) []pdfLayoutBox {
	if r.ctx.Err() != nil {
		return boxes
	}
	if len(boxes) < 2 {
		for i := range boxes {
			boxes[i].column = column
		}
		return boxes
	}
	if depth >= 64 {
		r.doc.warn("pdf_inference_uncertain", "Geometric subdivision reached its depth limit; remaining content uses top-to-bottom order")
	}
	if depth < 64 {
		// Peel isolated wide edge bands before looking for a persistent gutter.
		// Otherwise a title/footer masks the column whitespace, and selecting a
		// larger gap halfway through the body interleaves the two columns.
		left, right, top, bottom := boxes[0].x0, boxes[0].x1, 0, 0
		for i, b := range boxes {
			left = min(left, b.x0)
			right = max(right, b.x1)
			if b.y0 < boxes[top].y0 {
				top = i
			}
			if b.y1 > boxes[bottom].y1 {
				bottom = i
			}
		}
		for _, edge := range []int{top, bottom} {
			b := boxes[edge]
			if b.x1-b.x0 < (right-left)*.65 {
				continue
			}
			separate := true
			for i, other := range boxes {
				if i != edge && ((edge == top && other.y0-b.y1 <= max(.5, em*.12)) || (edge != top && b.y0-other.y1 <= max(.5, em*.12))) {
					separate = false
					break
				}
			}
			if !separate {
				continue
			}
			rest := append(append([]pdfLayoutBox{}, boxes[:edge]...), boxes[edge+1:]...)
			b.column = column
			if edge == top {
				return append([]pdfLayoutBox{b}, r.layoutOrder(rest, em, column, depth+1)...)
			}
			return append(r.layoutOrder(rest, em, column, depth+1), b)
		}
		xs := append([]pdfLayoutBox(nil), boxes...)
		sort.SliceStable(xs, func(i, j int) bool { return xs[i].x0 < xs[j].x0 })
		right = xs[0].x1
		for i := 1; i < len(xs); i++ {
			if xs[i].x0-right >= max(2*em, 12) && pdfColumnExtent(xs[:i], em) && pdfColumnExtent(xs[i:], em) {
				return append(r.layoutOrder(xs[:i], em, column+"L", depth+1), r.layoutOrder(xs[i:], em, column+"R", depth+1)...)
			}
			right = max(right, xs[i].x1)
		}
		ys := append([]pdfLayoutBox(nil), boxes...)
		sort.SliceStable(ys, func(i, j int) bool { return ys[i].y0 < ys[j].y0 })
		yBottom, cut := ys[0].y1, 0
		for i := 1; i < len(ys); i++ {
			if ys[i].y0-yBottom > max(.5, em*.12) {
				cut = i
				break
			}
			yBottom = max(yBottom, ys[i].y1)
		}
		if cut > 0 {
			return append(r.layoutOrder(ys[:cut], em, column, depth+1), r.layoutOrder(ys[cut:], em, column, depth+1)...)
		}
	}
	for i := range boxes {
		boxes[i].column = column
	}
	sort.SliceStable(boxes, func(i, j int) bool {
		if math.Round(boxes[i].base*1000) == math.Round(boxes[j].base*1000) {
			return boxes[i].x0 < boxes[j].x0
		}
		return boxes[i].base < boxes[j].base
	})
	return boxes
}

func pdfColumnExtent(boxes []pdfLayoutBox, em float64) bool {
	if len(boxes) < 2 {
		return false
	}
	x0, y0, x1, y1 := boxes[0].x0, boxes[0].y0, boxes[0].x1, boxes[0].y1
	for _, b := range boxes {
		x0 = min(x0, b.x0)
		y0 = min(y0, b.y0)
		x1 = max(x1, b.x1)
		y1 = max(y1, b.y1)
	}
	return x1-x0 >= 4*em && y1-y0 >= 1.5*em
}

func pdfLayoutLines(boxes []pdfLayoutBox) []pdfLayoutBox {
	var out []pdfLayoutBox
	for _, b := range boxes {
		if len(out) > 0 {
			last := &out[len(out)-1]
			if b.block.Kind == Paragraph && last.block.Kind == Paragraph && b.column == last.column && math.Abs(b.base-last.base) <= max(1, min(b.size, last.size)*.25) && b.x0 >= last.x0-.1 {
				last.block.Runs = joinPDFLayoutRuns(last.block.Runs, b.block.Runs)
				last.block.Source = inferredPDFSource([]Block{{Runs: last.block.Runs}}, "geometric_baseline_line", .7)
				last.x1 = max(last.x1, b.x1)
				last.y0 = min(last.y0, b.y0)
				last.y1 = max(last.y1, b.y1)
				continue
			}
		}
		if b.block.Kind == Paragraph {
			b.block.Source = inferredPDFSource([]Block{b.block}, "geometric_baseline_line", .7)
		}
		out = append(out, b)
	}
	return out
}

func joinPDFLayoutRuns(a, b []Run) []Run {
	if len(a) > 0 && len(b) > 0 && needsPDFSeparator(a[len(a)-1], b[0]) {
		x, y := a[len(a)-1], b[0]
		s := inferredPDFSource([]Block{{Runs: []Run{x, y}}}, "geometric_word_gap", .6)
		s.Quad, s.Regions, s.Precision = nil, nil, "synthetic_separator"
		link := ""
		if x.Link == y.Link {
			link = x.Link
		}
		gap, _ := commonTextStyle(x, y)
		gap.Text = " "
		gap.Source = s
		gap.Link = link
		a = append(a, gap)
	}
	return append(a, b...)
}

func layoutText(b Block) string {
	var s strings.Builder
	for _, r := range b.Runs {
		s.WriteString(r.Text)
	}
	return s.String()
}

func (r *pdfReader) inferHeadings(lines []pdfLayoutBox, em float64) {
	var sizes []float64
	for _, b := range lines {
		if b.block.Kind == Paragraph && b.size >= em*1.25 && uniseg.GraphemeClusterCount(layoutText(b.block)) <= 160 {
			sizes = append(sizes, b.size)
		}
	}
	sort.Sort(sort.Reverse(sort.Float64Slice(sizes)))
	var ranks []float64
	for _, size := range sizes {
		if len(ranks) == 0 || math.Abs(size-ranks[len(ranks)-1]) > .5 {
			ranks = append(ranks, size)
		}
	}
	for i := range lines {
		b := &lines[i]
		if b.block.Kind != Paragraph || b.size < em*1.25 || uniseg.GraphemeClusterCount(layoutText(b.block)) > 160 {
			continue
		}
		uniform := true
		for _, run := range b.block.Runs {
			if strings.TrimSpace(run.Text) != "" && (run.Source == nil || run.Source.FontSize < em*1.25) {
				uniform = false
			}
		}
		if !uniform {
			continue
		}
		b.block.Kind = Heading
		b.block.Level = 1
		for j, size := range ranks {
			if math.Abs(size-b.size) <= .5 {
				b.block.Level = min(6, j+1)
				break
			}
		}
		b.block.Source = inferredPDFSource([]Block{b.block}, "relative_font_size_heading_rank", .6)
		b.block.Source.Role = fmt.Sprintf("H%d", b.block.Level)
	}
}

func layoutContinues(a, b pdfLayoutBox) bool {
	if a.block.Kind != Paragraph || b.block.Kind != Paragraph || a.column != b.column || math.Abs(a.size-b.size) > max(.5, a.size*.1) {
		return false
	}
	gap := b.base - a.base
	return gap >= a.size*.65 && gap <= a.size*1.65 && math.Abs(a.x0-b.x0) <= a.size*.8 && !strings.ContainsAny(layoutText(a.block), "\r\n\u2028\u2029")
}

func (r *pdfReader) inferFlow(lines []pdfLayoutBox) []Block {
	var out []Block
	var previous *pdfLayoutBox
	for i := 0; i < len(lines); {
		if list, end, ok := r.inferList(lines, i); ok {
			out = append(out, list)
			i = end
			previous = nil
			continue
		}
		line := lines[i]
		if previous != nil && layoutContinues(*previous, line) {
			last := &out[len(out)-1]
			last.Runs = joinPDFLayoutRuns(last.Runs, line.block.Runs)
			last.Source = inferredPDFSource([]Block{{Runs: last.Runs}}, "aligned_baselines_paragraph", .65)
		} else {
			out = append(out, line.block)
		}
		previous = &line
		i++
	}
	return out
}

var pdfGeometricMarker = regexp.MustCompile(`^\s*(?:([0-9]{1,10})([.)])|([•◦▪●-]))[ \t\x{00A0}]+`)

type pdfLayoutMarker struct {
	kind, label   string
	number, bytes int
}

func layoutMarker(b Block) (pdfLayoutMarker, bool) {
	if b.Kind != Paragraph {
		return pdfLayoutMarker{}, false
	}
	text := layoutText(b)
	m := pdfGeometricMarker.FindStringSubmatchIndex(text)
	if m == nil || m[1] >= len(text) {
		return pdfLayoutMarker{}, false
	}
	marker := pdfLayoutMarker{kind: "bullet", label: strings.TrimSpace(text[:m[1]]), bytes: m[1]}
	if m[2] >= 0 {
		n, e := strconv.ParseInt(text[m[2]:m[3]], 10, 32)
		if e != nil {
			return pdfLayoutMarker{}, false
		}
		marker.kind, marker.number = "number", int(n)
	}
	left := marker.bytes
	for _, run := range b.Runs {
		if left <= 0 {
			break
		}
		if run.Link != "" {
			return pdfLayoutMarker{}, false
		}
		left -= len(run.Text)
	}
	return marker, true
}

func stripLayoutMarker(b Block, m pdfLayoutMarker) Block {
	runs := []Run{}
	left := m.bytes
	for _, run := range b.Runs {
		if left >= len(run.Text) {
			left -= len(run.Text)
			continue
		}
		if left > 0 {
			run.Text = run.Text[left:]
			if run.Source != nil {
				s := *run.Source
				s.Precision = "partial_text_span_bounds"
				s.Evidence += ";list_prefix_removed"
				s.Confidence = min(s.Confidence, .6)
				run.Source = &s
			}
			left = 0
		}
		runs = append(runs, run)
	}
	b.Runs = runs
	b.Source = inferredPDFSource([]Block{{Runs: runs}}, "marker_list_body", .65)
	return b
}

func (r *pdfReader) inferList(lines []pdfLayoutBox, start int) (Block, int, bool) {
	return r.inferListAt(lines, start, 0)
}

func (r *pdfReader) inferListAt(lines []pdfLayoutBox, start, depth int) (Block, int, bool) {
	if depth >= 16 {
		r.doc.warn("pdf_inference_uncertain", "List inference reached its nesting limit; remaining markers stay literal text")
		return Block{}, start, false
	}
	if r.ctx.Err() != nil {
		return Block{}, start, false
	}
	first, ok := layoutMarker(lines[start].block)
	if !ok {
		return Block{}, start, false
	}
	list := Block{Kind: ListBlock, List: first.kind}
	if first.kind == "number" {
		list.Start = listStartPointer(first.number)
	}
	i := start
	for i < len(lines) {
		line := lines[i]
		m, ok := layoutMarker(line.block)
		if !ok || m.kind != first.kind || line.column != lines[start].column || math.Abs(line.x0-lines[start].x0) > line.size*.5 || (m.kind == "number" && m.number != first.number+len(list.Items)) {
			break
		}
		if i > start && line.base-lines[i-1].base > line.size*2.5 {
			break
		}
		body := stripLayoutMarker(line.block, m)
		item := ListItem{Blocks: []Block{body}, Source: inferredPDFSource([]Block{line.block}, "consecutive_list_markers", .7)}
		item.Source.Label = m.label
		i++
		for i < len(lines) && lines[i].column == line.column && lines[i].block.Kind == Paragraph {
			next := lines[i]
			_, marked := layoutMarker(next.block)
			if marked && next.x0 > line.x0+line.size*.75 {
				nested, end, valid := r.inferListAt(lines, i, depth+1)
				if !valid {
					break
				}
				item.Blocks = append(item.Blocks, nested)
				i = end
				continue
			}
			if marked || next.x0 < line.x0+line.size || next.x0 > line.x0+4*line.size || next.base-lines[i-1].base > line.size*1.65 || next.base <= lines[i-1].base {
				break
			}
			last := &item.Blocks[len(item.Blocks)-1]
			if last.Kind != Paragraph {
				break
			}
			last.Runs = joinPDFLayoutRuns(last.Runs, next.block.Runs)
			last.Source = inferredPDFSource([]Block{{Runs: last.Runs}}, "indented_list_continuation", .6)
			i++
		}
		item.Source = inferredPDFSource(append([]Block{line.block}, item.Blocks...), "consecutive_list_markers", .65)
		item.Source.Label = m.label
		list.Items = append(list.Items, item)
	}
	if len(list.Items) < 2 {
		return Block{}, start, false
	}
	list.Source = inferredPDFSource([]Block{list}, "consecutive_list_markers", .65)
	r.doc.warn("pdf_list_inferred", "List membership and nesting are inferred from consecutive markers and indentation; marker styling/punctuation normalize and original labels persist in source provenance")
	return list, i, true
}

var pdfNumericCell = regexp.MustCompile(`^[+−-]?(?:[$€£¥]\s*)?[0-9]+(?:[, .][0-9]+)*(?:\s*[%$€£¥])?$`)
var pdfMarkerCell = regexp.MustCompile(`^(?:[0-9]{1,10}[.)]|[•◦▪●-])$`)

func (r *pdfReader) inferTables(boxes []pdfLayoutBox, em float64) []pdfLayoutBox {
	// First cluster equal baselines, then split only conspicuous cell gutters.
	// Repeated aligned rows alone could be prose columns. Require at least two
	// numeric values in the same column before assigning table semantics.
	type row struct {
		cells []pdfLayoutBox
		ids   []int
		base  float64
	}
	ids := make([]int, len(boxes))
	for i := range ids {
		ids[i] = i
	}
	sort.SliceStable(ids, func(i, j int) bool {
		a, b := boxes[ids[i]], boxes[ids[j]]
		if math.Round(a.base*1000) == math.Round(b.base*1000) {
			return a.x0 < b.x0
		}
		return a.base < b.base
	})
	var rows []row
	for _, id := range ids {
		b := boxes[id]
		if b.block.Kind != Paragraph || b.tableBarrier {
			continue
		}
		if len(rows) == 0 || math.Abs(rows[len(rows)-1].base-b.base) > max(1, min(em, b.size)*.25) {
			rows = append(rows, row{base: b.base})
		}
		rw := &rows[len(rows)-1]
		rw.ids = append(rw.ids, id)
		if len(rw.cells) > 0 && b.x0-rw.cells[len(rw.cells)-1].x1 < max(12, em*1.5) {
			last := &rw.cells[len(rw.cells)-1]
			last.block.Runs = joinPDFLayoutRuns(last.block.Runs, b.block.Runs)
			last.x1 = max(last.x1, b.x1)
			last.y0 = min(last.y0, b.y0)
			last.y1 = max(last.y1, b.y1)
		} else {
			rw.cells = append(rw.cells, b)
		}
	}
	used := map[int]bool{}
	var tables []pdfLayoutBox
	for i := 0; i < len(rows); {
		if r.ctx.Err() != nil {
			return boxes
		}
		first := rows[i]
		j := i + 1
		if len(first.cells) < 2 || pdfMarkerCell.MatchString(strings.TrimSpace(layoutText(first.cells[0].block))) {
			i++
			continue
		}
		for j < len(rows) && len(rows[j].cells) == len(first.cells) && rows[j].base-rows[j-1].base > em*.5 && rows[j].base-rows[j-1].base <= em*2.5 {
			if pdfMarkerCell.MatchString(strings.TrimSpace(layoutText(rows[j].cells[0].block))) {
				break
			}
			aligned := true
			for c, cell := range first.cells {
				if math.Abs(cell.x0-rows[j].cells[c].x0) > max(2, em*.3) {
					aligned = false
				}
			}
			if !aligned {
				break
			}
			j++
		}
		numeric := false
		for c := range first.cells {
			n := 0
			for k := i; k < j; k++ {
				if pdfNumericCell.MatchString(strings.TrimSpace(layoutText(rows[k].cells[c].block))) {
					n++
				}
			}
			if n >= 2 && n*2 >= j-i {
				numeric = true
			}
		}
		if j-i < 2 || !numeric {
			i++
			continue
		}
		table := Block{Kind: Table}
		for k := i; k < j; k++ {
			rw := Row{}
			for _, cell := range rows[k].cells {
				p := cell.block
				p.Source = inferredPDFSource([]Block{{Runs: p.Runs}}, "aligned_numeric_table_cell", .6)
				rw.Cells = append(rw.Cells, Cell{ColSpan: 1, Blocks: []Block{p}, Source: p.Source})
			}
			table.Rows = append(table.Rows, rw)
			for _, id := range rows[k].ids {
				used[id] = true
			}
		}
		table.Source = inferredPDFSource([]Block{table}, "aligned_rows_numeric_columns", .6)
		box, _ := pdfLayoutBlock(table)
		tables = append(tables, box)
		i = j
		r.doc.warn("pdf_table_inferred", "Table cells are inferred from repeated aligned rows and numeric columns; header roles, multiline cells, empty cells and spanning cells are not established by this heuristic")
	}
	var out []pdfLayoutBox
	for i, b := range boxes {
		if !used[i] {
			out = append(out, b)
		}
	}
	return append(out, tables...)
}
