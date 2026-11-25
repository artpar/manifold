package engine

import (
	"fmt"
	"github.com/rivo/uniseg"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
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
		r.alignPDFLinkedLabels(boxes, page.Number)
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
				last.size = max(last.size, b.size)
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

	// Extent is an editorial grouping, separate from the immutable physical bounds.
	// A separator-backed footnote zone or a leading endnote page supports following
	// coherent blocks. Missing/conflicting evidence leaves those blocks in flow.

	// Plan all moves before assigning references or removing blocks. Existing body
	// roots and citation owners are barriers, as are missing geometry, unqualified page
	// changes, large gaps, incompatible type sizes and unrelated text columns.

	// A leading note page has no prior ordinary same-page content.

	// Joining is a separate inference from note ownership. Explicit paragraphs,
	// authorial starts, incompatible styles and sentence/indent boundaries survive.

	// Seek the next physical note region while leaving ordinary content above it in
	// flow. Matching separator geometry supports rich blocks; matching
	// text-wrap evidence additionally permits a changed separator or a leading endnote page.

	// Continuation separators may expand to the column width. Matching text
	// style supplies another cue without forcing two paragraphs to merge.

	// A new small leading mark can indicate an unresolved distinct note.

	// Remove a synthetic boundary only when its entire physical-page flow moved to
	// notes. Real blank pages and pages with ordinary content/anchors remain. Pages
	// and source regions independently retain every original physical boundary.

	// Inference exhaustion keeps original flow and physical evidence, with a diagnostic.

	// Only layout boxes move: physical glyph quads/baselines remain immutable facts.
	// Link-owned raised labels align with adjacent larger text for ordering/grouping.

	// Reciprocal physical GoTo views plus matching raised marks support an inferred
	// note graph; neither a number alone nor a destination alone establishes a note.

	// Resolve all assignments before mutating: one mark cannot name two bodies.

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

type pdfNoteExtentFacts struct {
	page               int
	x0, y0, x1, y1, em float64
	ok                 bool
}

func (r *pdfReader) pdfNoteExtentFacts(b *Block) pdfNoteExtentFacts {
	f := pdfNoteExtentFacts{x0: math.Inf(1), y0: math.Inf(1), x1: math.Inf(-1), y1: math.Inf(-1), ok: true}
	var source func(*SourceInfo)
	source = func(s *SourceInfo) {
		if s == nil || !f.ok {
			return
		}
		if !r.noteInferenceStep() {
			f.ok = false
			return
		}
		if len(s.References) != 0 || len(s.Artifacts) != 0 {
			f.ok = false
			return
		}
		add := func(page int, q Quad) {
			if page < 1 || f.page != 0 && page != f.page {
				f.ok = false
				return
			}
			f.page = page
			x0, y0, x1, y1 := quadBounds(q)
			f.x0, f.y0, f.x1, f.y1 = min(f.x0, x0), min(f.y0, y0), max(f.x1, x1), max(f.y1, y1)
		}
		if s.Quad != nil {
			add(s.Page, *s.Quad)
		}
		for _, region := range s.Regions {
			if !r.noteInferenceStep() {
				f.ok = false
				return
			}
			add(region.Page, region.Quad)
		}
	}
	var walk func(*Block)
	walk = func(block *Block) {
		if !f.ok {
			return
		}
		if !r.noteInferenceStep() {
			f.ok = false
			return
		}
		switch block.Kind {
		case Paragraph, Heading, ListBlock, Table, Picture:
		default:
			f.ok = false
			return
		}
		source(block.Source)
		for _, run := range block.Runs {
			if !r.noteInferenceStep() {
				f.ok = false
				return
			}
			if run.Note != nil {
				f.ok = false
				return
			}
			source(run.Source)
			f.em = max(f.em, run.FontSize)
		}
		for i := range block.Items {
			item := &block.Items[i]
			source(item.Source)
			if item.Marker != nil {
				source(item.Marker.Source)
				for j := range item.Marker.Blocks {
					walk(&item.Marker.Blocks[j])
				}
			}
			for j := range item.Blocks {
				walk(&item.Blocks[j])
			}
		}
		for i := range block.Rows {
			for j := range block.Rows[i].Cells {
				for k := range block.Rows[i].Cells[j].Blocks {
					walk(&block.Rows[i].Cells[j].Blocks[k])
				}
			}
		}
	}
	walk(b)
	f.ok = f.ok && f.page > 0 && finite(f.x0) && finite(f.y0) && finite(f.x1) && finite(f.y1)
	return f
}

func (r *pdfReader) pdfNoteSeparator(page int, left, top, em float64) bool {
	return r.pdfNoteSeparatorWidth(page, left, top, em) > 0
}

func (r *pdfReader) pdfNoteSeparatorWidth(page int, left, top, em float64) float64 {
	for _, line := range r.rulings[page] {
		if !r.noteInferenceStep() {
			return 0
		}
		if line.horizontal && line.width <= max(3, em*.3) && line.pos <= top+1 && line.pos >= top-4*em && line.start <= left+em && line.end >= left+4*em {
			return line.end - line.start
		}
	}
	for _, patch := range r.rulingPatches[page] {
		if !r.noteInferenceStep() {
			return 0
		}
		if patch.y1-patch.y0 <= max(3, em*.3) && patch.y1 <= top+1 && patch.y0 >= top-4*em && patch.x0 <= left+em && patch.x1 >= left+4*em {
			return patch.x1 - patch.x0
		}
	}
	return 0
}

func (r *pdfReader) planPDFNoteExtents(marks []pdfLinkedNoteMark, bodies map[int]int, kinds map[int]string, citations map[int][]int) (map[int]pdfNoteExtentPlan, bool) {
	indices := map[*Block]int{}
	for i := range r.doc.Blocks {
		indices[&r.doc.Blocks[i]] = i
	}
	protected := map[*Block]bool{}
	rootLinks := map[*Block]int{}
	bodyOrder := make([]int, 0, len(bodies))
	for link, mark := range bodies {
		protected[marks[mark].block] = true
		rootLinks[marks[mark].block] = link
		bodyOrder = append(bodyOrder, link)
	}
	sort.Slice(bodyOrder, func(i, j int) bool {
		a, b := indices[marks[bodies[bodyOrder[i]]].block], indices[marks[bodies[bodyOrder[j]]].block]
		if a == b {
			return bodyOrder[i] < bodyOrder[j]
		}
		return a < b
	})
	for _, refs := range citations {
		for _, mark := range refs {
			protected[marks[mark].block] = true
		}
	}
	facts := map[int]pdfNoteExtentFacts{}
	fact := func(index int) pdfNoteExtentFacts {
		if f, ok := facts[index]; ok {
			return f
		}
		f := r.pdfNoteExtentFacts(&r.doc.Blocks[index])
		facts[index] = f
		return f
	}
	extents := map[int]pdfNoteExtentPlan{}
	claims := map[int]int{}
	sharedZone := map[int]bool{}
	sharedWidth := map[int]float64{}
	for _, bodyLink := range bodyOrder {
		markIndex := bodies[bodyLink]
		if len(citations[bodyLink]) == 0 {
			continue
		}
		mark := marks[markIndex]
		index, ok := indices[mark.block]
		if !ok {
			continue
		}
		base := fact(index)
		if !base.ok || base.em <= 0 || base.page > len(r.doc.Pages) {
			continue
		}
		left, _, _, _ := quadBounds(*mark.run().Source.Quad)
		zone := sharedZone[index]
		width := sharedWidth[index]
		if !zone && kinds[bodyLink] == Footnote {
			width = r.pdfNoteSeparatorWidth(base.page, left, base.y0, base.em)
			zone = width > 0
		} else if !zone && kinds[bodyLink] == Endnote && base.y0 < r.doc.Pages[base.page-1].Height*.35 {

			zone = true
			for i := index - 1; i >= 0; i-- {
				if !r.noteInferenceStep() {
					return nil, false
				}
				if r.doc.Blocks[i].Kind == PageBreak {
					break
				}
				prior := fact(i)
				if prior.ok && prior.page == base.page || !prior.ok && r.doc.Blocks[i].Kind != AnchorPoint {
					zone = false
					break
				}
			}
		}
		if !zone {
			continue
		}
		bottom := base.y1
		currentPage, lastIndex := base.page, index
		plan := pdfNoteExtentPlan{joins: map[int]bool{}}
		for i := index + 1; i < len(r.doc.Blocks); i++ {
			if !r.noteInferenceStep() {
				return nil, false
			}
			b := &r.doc.Blocks[i]
			if b.Kind == PageBreak {
				next, join, ok := r.pdfNoteNextPage(i+1, lastIndex, fact(lastIndex), left, base.em, width, kinds[bodyLink], rootLinks, protected, fact)
				if !ok {
					return nil, false
				}
				if next < 0 {
					break
				}
				i = next
				b = &r.doc.Blocks[i]
				currentPage = fact(i).page
				bottom = fact(i).y0
				plan.pages++
				plan.joins[i] = join
			}
			f := fact(i)
			if !pdfNoteCoherentBlock(b, f, currentPage, left, base.em, r.doc.Pages[currentPage-1].Width) || f.y0 < bottom-1 || f.y0-bottom > 4*base.em {
				break
			}
			if protected[b] {
				if next, exists := rootLinks[b]; exists && kinds[next] == kinds[bodyLink] {
					sharedZone[i] = true
					sharedWidth[i] = width
				}
				break
			}
			if owner, exists := claims[i]; exists && owner != bodyLink {
				r.doc.warn("pdf_note_inference_uncertain", "Competing note bodies claim one continuation; the unassigned source remains in flow")
				return nil, false
			}
			claims[i] = bodyLink
			plan.blocks = append(plan.blocks, i)
			lastIndex = i
			bottom = f.y1
		}
		extents[bodyLink] = plan
	}
	if r.noteInferenceWork > pdfNoteInferenceLimit || r.ctx.Err() != nil {
		return nil, false
	}
	return extents, true
}

type pdfNoteExtentPlan struct {
	blocks []int
	joins  map[int]bool
	pages  int
}

func (r *pdfReader) pdfNoteParagraphContinues(previous, next *Block, a, b pdfNoteExtentFacts) bool {
	if previous.Kind != Paragraph || next.Kind != Paragraph || next.PageBreakBefore || previous.Direction != next.Direction ||
		previous.Source != nil && previous.Source.Structure == "explicit" || next.Source != nil && next.Source.Structure == "explicit" {
		return false
	}
	last, first := -1, -1
	for i := len(previous.Runs) - 1; i >= 0; i-- {
		if !r.noteInferenceStep() {
			return false
		}
		if strings.TrimSpace(previous.Runs[i].Text) != "" {
			last = i
			break
		}
	}
	for i := range next.Runs {
		if !r.noteInferenceStep() {
			return false
		}
		if strings.TrimSpace(next.Runs[i].Text) != "" {
			first = i
			break
		}
	}
	if last < 0 || first < 0 {
		return false
	}
	x, y := previous.Runs[last], next.Runs[first]
	if _, mixed := commonTextStyle(x, y); mixed || x.Direction != y.Direction || x.Language != y.Language || x.EastAsiaLanguage != y.EastAsiaLanguage || x.ComplexLanguage != y.ComplexLanguage || x.ComplexScript != y.ComplexScript {
		return false
	}
	if x.Source == nil || y.Source == nil || x.Source.Baseline == nil || y.Source.Baseline == nil || math.Abs(a.x0-b.x0) > max(1, a.em*.8) || x.Source.Baseline[1].X < a.x1-3*a.em {
		return false
	}
	left, _ := utf8.DecodeLastRuneInString(strings.TrimSpace(x.Text))
	right, _ := utf8.DecodeRuneInString(strings.TrimSpace(y.Text))
	return !strings.ContainsRune(".!?。！？", left) && (unicode.IsLower(right) || unicode.IsDigit(right))
}

func pdfNoteCoherentBlock(block *Block, f pdfNoteExtentFacts, page int, left, em, width float64) bool {
	if !f.ok || f.page != page || f.x0 < left-1.5*em || f.x1 > width+1 || f.y0 < 0 {
		return false
	}
	if f.em > 0 && (f.em < em*.85 || f.em > em*1.8) {
		return false
	}
	return (block.Kind != Paragraph && block.Kind != Heading && block.Kind != ListBlock) || f.x0 <= left+3*em
}

func (r *pdfReader) pdfNoteBoundaryStyle(previous, next *Block) bool {
	if previous.Kind != Paragraph || next.Kind != Paragraph {
		return false
	}
	var a, b *Run
	for i := len(previous.Runs) - 1; i >= 0; i-- {
		if !r.noteInferenceStep() {
			return false
		}
		if strings.TrimSpace(previous.Runs[i].Text) != "" {
			a = &previous.Runs[i]
			break
		}
	}
	for i := range next.Runs {
		if !r.noteInferenceStep() {
			return false
		}
		if strings.TrimSpace(next.Runs[i].Text) != "" {
			b = &next.Runs[i]
			break
		}
	}
	if a == nil || b == nil {
		return false
	}
	_, mixed := commonTextStyle(*a, *b)
	return !mixed && a.Direction == b.Direction && a.Language == b.Language && a.EastAsiaLanguage == b.EastAsiaLanguage && a.ComplexLanguage == b.ComplexLanguage && a.ComplexScript == b.ComplexScript
}

func (r *pdfReader) pdfNoteNextPage(start, last int, current pdfNoteExtentFacts, left, em, signature float64, kind string, roots map[*Block]int, protected map[*Block]bool, fact func(int) pdfNoteExtentFacts) (int, bool, bool) {
	page := current.page + 1
	if current.page < 1 || page > len(r.doc.Pages) || current.y1 < r.doc.Pages[current.page-1].Height*.8 || current.x1-current.x0 < 8*em {
		return -1, false, true
	}
	for i := start; i < len(r.doc.Blocks); i++ {
		if !r.noteInferenceStep() {
			return -1, false, false
		}
		b := &r.doc.Blocks[i]
		if b.Kind == PageBreak {
			break
		}
		f := fact(i)
		if !f.ok {
			if b.Kind == AnchorPoint {
				continue
			}
			break
		}
		if f.page != page {
			break
		}
		if _, root := roots[b]; root && math.Abs(f.x0-left) <= 3*em {
			break
		}
		if !pdfNoteCoherentBlock(b, f, page, left, em, r.doc.Pages[page-1].Width) {
			continue
		}
		join := r.pdfNoteParagraphContinues(&r.doc.Blocks[last], b, current, f)
		width := 0.0
		if kind == Footnote {
			width = r.pdfNoteSeparatorWidth(page, left, f.y0, em)
		}
		matching := signature > 0 && width > 0 && math.Abs(width-signature) <= max(2*em, signature*.2)

		expanded := signature > 0 && width >= signature && r.pdfNoteBoundaryStyle(&r.doc.Blocks[last], b)
		eligible := kind == Footnote && width > 0 && (matching || join || expanded) || kind == Endnote && i == start && f.y0 < r.doc.Pages[page-1].Height*.35 && join
		if eligible {
			if protected[b] {
				break
			}

			for j, run := range b.Runs {
				if !r.noteInferenceStep() {
					return -1, false, false
				}
				if strings.TrimSpace(run.Text) == "" {
					continue
				}
				if pdfCompactNoteLabel(run.Text) && pdfRaisedNoteMark(b.Runs, j) {
					return -1, false, true
				}
				break
			}
			return i, join, true
		}
		if kind == Endnote {
			break
		}
	}
	return -1, false, r.ctx.Err() == nil && r.noteInferenceWork <= pdfNoteInferenceLimit
}

func (r *pdfReader) removePDFNoteFlow(removed map[*Block]bool) {
	var out []Block
	boundary, start := -1, 0
	flush := func(end int) {
		moved, remaining := false, false
		for i := start; i < end; i++ {
			if removed[&r.doc.Blocks[i]] {
				moved = true
			} else {
				remaining = true
			}
		}
		if boundary >= 0 {
			b := r.doc.Blocks[boundary]
			if !moved || remaining || b.Source != nil {
				out = append(out, b)
			} else {
				r.doc.warn("pdf_note_flow_boundary_inferred", "A physical page whose complete flow moved to inferred notes no longer creates an empty reflow page; original physical pages remain recorded")
			}
		}
		for i := start; i < end; i++ {
			if !removed[&r.doc.Blocks[i]] {
				out = append(out, r.doc.Blocks[i])
			}
		}
	}
	for i, b := range r.doc.Blocks {
		if b.Kind == PageBreak {
			flush(i)
			boundary, start = i, i+1
		}
	}
	flush(len(r.doc.Blocks))
	r.doc.Blocks = out
}

const pdfNoteInferenceLimit = 4_000_000

func (r *pdfReader) noteInferenceStep() bool {
	if r.ctx.Err() != nil {
		return false
	}
	r.noteInferenceWork++
	if r.noteInferenceWork > pdfNoteInferenceLimit {
		if r.noteInferenceWork > pdfNoteInferenceLimit+1 {
			r.noteInferenceWork = pdfNoteInferenceLimit + 1
			return false
		}
		r.doc.warn("pdf_note_inference_uncertain", "Note-link inference reached its comparison bound; unproven content remains in flow with original physical navigation")
		return false
	}
	return true
}

func pdfCompactNoteLabel(text string) bool {
	if text == "" || strings.TrimSpace(text) != text || utf8.RuneCountInString(text) > 8 {
		return false
	}
	for _, c := range text {
		if !strings.ContainsRune("0123456789ivxlcdmIVXLCDMabcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ*†‡§¶", c) {
			return false
		}
	}
	return true
}
func pdfQuadOverlap(a, b Quad) float64 {
	ax, ay, ar, ab := quadBounds(a)
	bx, by, br, bb := quadBounds(b)
	area := (ar - ax) * (ab - ay)
	if area <= 0 {
		return 0
	}
	return max(0, min(ar, br)-max(ax, bx)) * max(0, min(ab, bb)-max(ay, by)) / area
}
func pdfPointInQuad(p Point, q Quad) bool {
	x, y, right, bottom := quadBounds(q)
	return p.X >= x-1 && p.X <= right+1 && p.Y >= y-1 && p.Y <= bottom+1
}

func (r *pdfReader) alignPDFLinkedLabels(boxes []pdfLayoutBox, page int) {
	if r.doc.PDFNavigation == nil {
		return
	}
	var regions []Quad
	for _, link := range r.doc.PDFNavigation.Links {
		if link.Page == page && link.Kind == "GoTo" {
			regions = append(regions, link.Regions...)
		}
	}
	type correction struct {
		index        int
		base, y0, y1 float64
	}
	var corrections []correction
	for i := range boxes {
		b := &boxes[i]
		if b.block.Kind != Paragraph || len(b.block.Runs) != 1 || !pdfCompactNoteLabel(b.block.Runs[0].Text) || b.block.Source == nil || b.block.Source.Quad == nil {
			continue
		}
		owned := false
		for _, q := range regions {
			if !r.noteInferenceStep() {
				return
			}
			if pdfQuadOverlap(*b.block.Source.Quad, q) >= .6 {
				owned = true
				break
			}
		}
		if !owned {
			continue
		}
		chosen := -1
		ambiguous := false
		for j, other := range boxes {
			if !r.noteInferenceStep() {
				return
			}
			if i == j || other.block.Kind != Paragraph || other.size < b.size*1.25 || other.base-b.base < other.size*.1 || other.base-b.base > other.size*.6 {
				continue
			}
			gap := math.Inf(1)
			if other.x1 <= b.x0+.5 {
				gap = b.x0 - other.x1
			} else if b.x1 <= other.x0+.5 {
				gap = other.x0 - b.x1
			}
			if gap > other.size*.5 {
				continue
			}
			if chosen >= 0 && math.Abs(boxes[chosen].base-other.base) > 1 {
				ambiguous = true
				break
			}
			chosen = j
		}
		if chosen >= 0 && !ambiguous {
			other := boxes[chosen]
			corrections = append(corrections, correction{i, other.base, other.y0, other.y1})
		}
	}
	for _, c := range corrections {
		boxes[c.index].base, boxes[c.index].y0, boxes[c.index].y1 = c.base, c.y0, c.y1
	}
}

type pdfLinkedNoteMark struct {
	block *Block
	index int
	label string
	top   bool
}

func (m pdfLinkedNoteMark) run() *Run { return &m.block.Runs[m.index] }
func pdfRaisedNoteMark(runs []Run, index int) bool {
	mark := runs[index].Source
	if mark == nil || mark.Baseline == nil {
		return false
	}
	for _, neighbor := range []int{index - 1, index + 1} {
		if neighbor < 0 || neighbor >= len(runs) {
			continue
		}
		other := runs[neighbor].Source
		if other == nil || other.Baseline == nil || other.Page != mark.Page || other.FontSize < mark.FontSize*1.25 {
			continue
		}
		rise := other.Baseline[0].Y - mark.Baseline[0].Y
		if rise >= other.FontSize*.1 && rise <= other.FontSize*.6 {
			return true
		}
	}
	return false
}
func (m pdfLinkedNoteMark) leadingBody() bool {
	if !m.top || m.block.Kind != Paragraph || m.block.Direction == "rtl" {
		return false
	}
	for _, run := range m.block.Runs {
		if run.Note != nil {
			return false
		}
	}
	for i, run := range m.block.Runs {
		if run.Text == "" {
			continue
		}
		if i < m.index {
			return false
		}
		if i > m.index && strings.TrimSpace(run.Text) != "" && run.Source != nil && run.Source.FontSize >= m.run().Source.FontSize*1.25 {
			return true
		}
	}
	return false
}

func (r *pdfReader) inferLinkedPDFNotes() error {
	nav := r.doc.PDFNavigation
	if nav == nil {
		return nil
	}
	var marks []pdfLinkedNoteMark
	var walk func([]Block, bool)
	walk = func(bs []Block, top bool) {
		for i := range bs {
			b := &bs[i]
			largest := 0.0
			for _, run := range b.Runs {
				if run.Source != nil {
					largest = max(largest, run.Source.FontSize)
				}
			}
			for j, run := range b.Runs {
				if run.Note == nil && pdfCompactNoteLabel(run.Text) && run.Source != nil && run.Source.Quad != nil && run.Source.Baseline != nil && largest >= run.Source.FontSize*1.25 && pdfRaisedNoteMark(b.Runs, j) {
					marks = append(marks, pdfLinkedNoteMark{b, j, run.Text, top})
				}
			}
			for j := range b.Items {
				walk(b.Items[j].Blocks, false)
			}
			for j := range b.Rows {
				for k := range b.Rows[j].Cells {
					walk(b.Rows[j].Cells[k].Blocks, false)
				}
			}
		}
	}
	walk(r.doc.Blocks, true)
	if len(marks) == 0 {
		return nil
	}
	destinations := map[string]PDFDestination{}
	for _, d := range nav.Destinations {
		destinations[d.ID] = d
	}
	owner := make([]int, len(nav.Links))
	for i := range owner {
		owner[i] = -1
	}
	for i, link := range nav.Links {
		if err := r.ctx.Err(); err != nil {
			return err
		}
		if link.Kind != "GoTo" {
			continue
		}
		for j, m := range marks {
			if !r.noteInferenceStep() {
				return r.ctx.Err()
			}
			s := m.run().Source
			if s.Page != link.Page {
				continue
			}
			for _, q := range link.Regions {
				if !r.noteInferenceStep() {
					return r.ctx.Err()
				}
				if pdfQuadOverlap(*s.Quad, q) >= .6 {
					if owner[i] >= 0 && owner[i] != j {
						owner[i] = -2
						break
					}
					owner[i] = j
				}
			}
			if owner[i] == -2 {
				break
			}
		}
	}
	target := make([]int, len(nav.Links))
	for i := range target {
		target[i] = -1
	}
	for i, link := range nav.Links {
		if err := r.ctx.Err(); err != nil {
			return err
		}
		d, ok := destinations[link.Destination]
		if !ok || owner[i] < 0 || d.View != "XYZ" || d.Point == nil || len(d.Parameters) != 3 || d.Parameters[0] == nil || d.Parameters[1] == nil {
			continue
		}
		for j, other := range nav.Links {
			if !r.noteInferenceStep() {
				return r.ctx.Err()
			}
			if i == j || owner[j] < 0 || other.Page != d.Page {
				continue
			}
			for _, q := range other.Regions {
				if !r.noteInferenceStep() {
					return r.ctx.Err()
				}
				if pdfPointInQuad(*d.Point, q) {
					if target[i] >= 0 && target[i] != j {
						target[i] = -2
						break
					}
					target[i] = j
				}
			}
			if target[i] == -2 {
				break
			}
		}
	}
	bodies := map[int]int{}
	kinds := map[int]string{}
	for i, j := range target {
		if j < 0 || target[j] != i {
			continue
		}
		citation, body := marks[owner[i]], marks[owner[j]]
		if citation.label != body.label || !body.leadingBody() {
			continue
		}
		cs, bs := citation.run().Source, body.run().Source
		kind := Endnote
		if bs.Page < cs.Page {
			continue
		}
		if bs.Page == cs.Page {
			left, y, _, _ := quadBounds(*bs.Quad)
			if bs.Page < 1 || bs.Page > len(r.doc.Pages) || bs.Baseline[0].Y-cs.Baseline[0].Y < 40 {
				continue
			}
			if y < r.doc.Pages[bs.Page-1].Height*.6 {
				em := bs.FontSize
				for _, run := range body.block.Runs {
					if !r.noteInferenceStep() {
						return r.ctx.Err()
					}
					em = max(em, run.FontSize)
				}
				if !r.pdfNoteSeparator(bs.Page, left, y, em) {
					continue
				}
			}
			kind = Footnote
		}
		bodies[j] = owner[j]
		kinds[j] = kind
	}

	markTarget := map[int]int{}
	for i, targetLink := range target {
		if targetLink < 0 || owner[i] < 0 {
			continue
		}
		mark, destinationMark := owner[i], owner[targetLink]
		if previous, ok := markTarget[mark]; ok && previous != destinationMark {
			markTarget[mark] = -2
		} else if !ok {
			markTarget[mark] = destinationMark
		}
	}
	bodyBlocks := map[*Block]bool{}
	for _, mark := range bodies {
		bodyBlocks[marks[mark].block] = true
	}
	citationsByBody := map[int][]int{}
	seen := map[[2]int]bool{}
	for i, bodyLink := range target {
		markIndex, ok := bodies[bodyLink]
		if !ok || owner[i] < 0 || owner[i] == markIndex {
			continue
		}
		citation := owner[i]
		if markTarget[citation] != markIndex || markTarget[markIndex] < 0 || bodyBlocks[marks[citation].block] || marks[citation].label != marks[markIndex].label {
			continue
		}
		key := [2]int{bodyLink, citation}
		if !seen[key] {
			citationsByBody[bodyLink] = append(citationsByBody[bodyLink], citation)
			seen[key] = true
		}
	}
	if err := r.ctx.Err(); err != nil {
		return err
	}
	extents, ok := r.planPDFNoteExtents(marks, bodies, kinds, citationsByBody)
	if !ok {
		return r.ctx.Err()
	}
	removed := map[*Block]bool{}
	continuations := 0
	pageContinuations, paragraphJoins := 0, 0
	bodyLinks := make([]int, 0, len(bodies))
	for bodyLink := range bodies {
		bodyLinks = append(bodyLinks, bodyLink)
	}
	sort.Ints(bodyLinks)
	for _, bodyLink := range bodyLinks {
		markIndex := bodies[bodyLink]
		body := marks[markIndex]
		if removed[body.block] {
			continue
		}
		citations := citationsByBody[bodyLink]
		if len(citations) == 0 {
			continue
		}
		labelRun := *body.run()
		labelSource := cloneSourceInfo(labelRun.Source)
		id := fmt.Sprintf("pdf:inferred-note:%d:%d", labelSource.Page, labelSource.Order)
		originalBody := []Block{*body.block}
		for _, index := range extents[bodyLink].blocks {
			originalBody = append(originalBody, r.doc.Blocks[index])
		}
		source := inferredPDFSource(originalBody, "pdf_note_reciprocal_views", .8)
		noteBody := *body.block
		noteBody.Runs = append(append([]Run(nil), body.block.Runs[:body.index]...), body.block.Runs[body.index+1:]...)
		noteBody.Source = inferredPDFSource([]Block{noteBody}, "pdf_note_reciprocal_body", .8)
		noteBlocks := []Block{noteBody}
		for _, index := range extents[bodyLink].blocks {
			next := r.doc.Blocks[index]
			if extents[bodyLink].joins[index] {
				paragraphJoins++
				previous := &noteBlocks[len(noteBlocks)-1]
				source := inferredPDFSource([]Block{*previous, next}, "pdf_note_page_paragraph_continuation", .65)
				previous.Runs = joinPDFLayoutRuns(append([]Run(nil), previous.Runs...), next.Runs)
				previous.Anchors = append(append([]string(nil), previous.Anchors...), next.Anchors...)
				previous.Source = source
			} else {
				noteBlocks = append(noteBlocks, next)
			}
		}
		if len(noteBlocks) > 1 {
			source.Evidence = "pdf_note_reciprocal_views;separated_note_zone_continuation"
			source.Confidence = .7
			continuations += len(noteBlocks) - 1
		}
		if extents[bodyLink].pages > 0 {
			pageContinuations += extents[bodyLink].pages
			source.Evidence += ";cross_page_note_zone"
			source.Confidence = .65
		}
		labelRun.Text, labelRun.Source, labelRun.Anchor = "", nil, ""
		r.doc.Notes = append(r.doc.Notes, Note{ID: id, Kind: kinds[bodyLink], KindInferred: true, Label: body.label, LabelSource: labelSource, BodyLabelStyle: &labelRun, Source: source, Blocks: noteBlocks})
		for _, index := range citations {
			run := marks[index].run()
			run.Source = cloneSourceInfo(run.Source)
			run.Source.Structure, run.Source.Evidence, run.Source.Confidence = "inferred", "pdf_note_reciprocal_views", .8
			run.Text, run.Link, run.Destination = "", "", ""
			run.Note = &NoteReference{ID: id, Kind: kinds[bodyLink], Inferred: true}
		}
		removed[body.block] = true
		for _, index := range extents[bodyLink].blocks {
			removed[&r.doc.Blocks[index]] = true
		}
	}
	if len(removed) > 0 {
		r.removePDFNoteFlow(removed)
		r.doc.warn("pdf_note_relationship_inferred", "Matching raised marks and reciprocal physical GoTo views infer note relationships; original annotations, views and glyph geometry remain source evidence")
		r.doc.warn("pdf_note_body_extent_inferred", "Note body extent is inferred from its linked leading paragraph and available note-zone evidence; unqualified or ambiguous continuation remains in flow")
		if continuations > 0 {
			r.doc.warn("pdf_note_continuation_inferred", "Consecutive coherent rich blocks in a separator-backed footnote zone or leading endnote page are grouped as inferred continuation; physical bounds remain source facts")
		}
		if pageContinuations > 0 {
			r.doc.warn("pdf_note_page_continuation_inferred", "Matching physical note zones infer ownership across pages; ordinary page content and original physical boundaries remain independently represented")
		}
		if paragraphJoins > 0 {
			r.doc.warn("pdf_note_paragraph_continuation_inferred", "Compatible wrap geometry and boundary text infer one note paragraph across physical pages; explicit or ambiguous paragraph boundaries remain separate")
		}
		r.doc.warn("pdf_note_kind_inferred", "Foot/endnote reflow kind is inferred from page position, not an explicit authorial note policy")
		r.doc.warn("pdf_note_label_presentation", "Inferred body marks retain original style and label_source; reflow may normalize their independent placement")
	}
	return nil
}
