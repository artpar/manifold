package manifold

import "sort"

// Reconcile a page-local painted grid with explicit row/cell membership. A
// candidate never changes the input table. Empty vertical continuations may be
// coalesced, but their explicit identities remain in the origin's provenance.
func (s *pdfStructureRecovery) recoverTableSpans(table Block, nodes []*pdfStructureNode) (Block, bool) {
	source := table.Source
	if source == nil || source.Page < 1 || source.Quad == nil || len(source.Regions) > 1 {
		return table, false
	}
	if s.grids == nil {
		s.grids = map[int][]pdfGridRegion{}
	}
	regions, ok := s.grids[source.Page]
	if !ok {
		regions = s.reader.rulingRegions(source.Page)
		s.grids[source.Page] = regions
	}
	var chosen Block
	matches := 0
	spansChanged := false
	x0, y0, x1, y1 := quadBounds(*source.Quad)
	for _, g := range regions {
		if s.reader.ctx.Err() != nil {
			return table, false
		}
		if !g.valid || len(g.ys)-1 != len(table.Rows) || !rulingContains(g.rect, pdfRulingRect{x0, y0, x1, y1}) {
			continue
		}
		candidate, changed, ok := s.matchTaggedGrid(table, nodes, g)
		if !ok {
			continue
		}
		if !changed {
			candidate = table
		}
		candidate.ColumnWidths = gridColumnWidths(g)
		src := *candidate.Source
		src.TableWidths = gridWidthSource(g)
		candidate.Source = &src
		chosen = candidate
		spansChanged = changed
		matches++
		if matches > 1 {
			return table, false
		}
	}
	if matches != 1 {
		return table, false
	}
	s.warn("pdf_table_widths_inferred", "Column widths are measured from a uniquely matching painted grid, independently of explicit logical membership")
	if spansChanged {
		s.warn("pdf_structure_spans_inferred", "Missing tagged-table spans are recovered from a uniquely matching painted grid; logical membership stays explicit and empty continuation identities are retained")
	}
	return chosen, true
}

func taggedCellEmpty(c Cell) bool {
	for _, b := range c.Blocks {
		if b.Kind != Paragraph || len(b.Runs) > 0 || b.PageBreakBefore {
			return false
		}
	}
	return true
}
func taggedCellInside(c Cell, rect pdfRulingRect, page int) bool {
	if c.Source != nil && c.Source.Page != 0 && c.Source.Page != page {
		return false
	}
	if taggedCellEmpty(c) {
		return true
	}
	if c.Source == nil || c.Source.Page != page || c.Source.Quad == nil {
		return false
	}
	x0, y0, x1, y1 := quadBounds(*c.Source.Quad)
	return rulingContains(rect, pdfRulingRect{x0, y0, x1, y1})
}
func taggedGridRect(g pdfGridRegion, c pdfGridCell) pdfRulingRect {
	return pdfRulingRect{g.xs[c.col], g.ys[c.row], g.xs[c.col+c.cols], g.ys[c.row+c.rows]}
}
func taggedSpanMatches(n *pdfStructureNode, c Cell, g pdfGridCell, continuation bool) bool {
	if continuation && (n.attrs["RowSpan"] != nil || n.id == "") {
		return false
	}
	if n.attrs["ColSpan"] != nil && max(1, c.ColSpan) != g.cols {
		return false
	}
	if continuation {
		return n.attrs["RowSpan"] == nil && taggedCellEmpty(c)
	}
	return n.attrs["RowSpan"] == nil || max(1, c.RowSpan) == g.rows
}

// Match each row to physical cell segments, allowing an empty continuation to
// be present or omitted in the logical tree. Count at most two alignments: an
// ambiguous sequence must never pick an arbitrary merged origin.
func (s *pdfStructureRecovery) matchTaggedRow(table Block, nodes []*pdfStructureNode, g pdfGridRegion, row int) ([]int, bool) {
	var segments []int
	for i, c := range g.cells {
		if c.row <= row && c.row+c.rows > row {
			segments = append(segments, i)
		}
	}
	// Grid cells are origin-sorted; spanning cells can precede new origins.
	sort.Slice(segments, func(i, j int) bool { return g.cells[segments[i]].col < g.cells[segments[j]].col })
	cells := table.Rows[row].Cells
	if len(cells) > len(segments) {
		return nil, false
	}
	required := make([]int, len(segments)+1)
	for i := len(segments) - 1; i >= 0; i-- {
		required[i] = required[i+1]
		if g.cells[segments[i]].row == row {
			required[i]++
		}
	}
	// Some Word exports list all empty continuation tags before the new
	// cells in a row. When the arity forces every eligible empty cell to be a
	// continuation, match the two ordered classes independently. No visible
	// cell is reordered, and inferred attribution keeps every explicit ID.
	var starts, continued, empty []int
	for _, gi := range segments {
		if g.cells[gi].row == row {
			starts = append(starts, gi)
		} else {
			continued = append(continued, gi)
		}
	}
	needed := len(cells) - len(starts)
	if needed < 0 || needed > len(continued) {
		return nil, false
	}
	for ci, c := range cells {
		if taggedCellEmpty(c) && nodes[row].kids[ci].attrs["RowSpan"] == nil && nodes[row].kids[ci].id != "" {
			empty = append(empty, ci)
		}
	}
	if needed > 0 && needed == len(empty) && needed == len(continued) {
		mapping := make([]int, len(cells))
		isContinuation := make(map[int]bool, len(empty))
		for i, ci := range empty {
			gi := continued[i]
			if !taggedSpanMatches(nodes[row].kids[ci], cells[ci], g.cells[gi], true) || !taggedCellInside(cells[ci], taggedGridRect(g, g.cells[gi]), table.Source.Page) {
				return nil, false
			}
			mapping[ci], isContinuation[ci] = gi, true
		}
		next := 0
		for ci, c := range cells {
			if isContinuation[ci] {
				continue
			}
			if next >= len(starts) {
				return nil, false
			}
			gi := starts[next]
			if !taggedSpanMatches(nodes[row].kids[ci], c, g.cells[gi], false) || !taggedCellInside(c, taggedGridRect(g, g.cells[gi]), table.Source.Page) {
				return nil, false
			}
			mapping[ci] = gi
			next++
		}
		return mapping, next == len(starts)
	}
	type state struct{ segment, cell int }
	memo := map[state]int{}
	exhausted := false
	var count func(int, int) int
	count = func(si, ci int) int {
		if s.reader.ctx.Err() != nil {
			return 0
		}
		if len(cells)-ci < required[si] || len(cells)-ci > len(segments)-si {
			return 0
		}
		if exhausted || len(memo) >= 100000 {
			exhausted = true
			return 0
		}
		key := state{si, ci}
		if value, ok := memo[key]; ok {
			return value
		}
		if si == len(segments) {
			if ci == len(cells) {
				return 1
			}
			return 0
		}
		grid := g.cells[segments[si]]
		continuation := grid.row < row
		total := 0
		if continuation {
			total = count(si+1, ci)
		}
		if total > 1 {
			memo[key] = 2
			return 2
		}
		if ci < len(cells) && taggedSpanMatches(nodes[row].kids[ci], cells[ci], grid, continuation) && taggedCellInside(cells[ci], taggedGridRect(g, grid), table.Source.Page) {
			total += count(si+1, ci+1)
		}
		total = min(2, total)
		memo[key] = total
		return total
	}
	alignments := count(0, 0)
	if exhausted {
		s.warn("pdf_structure_spans_limit", "Tagged span alignment exceeds the inference budget; original logical cells remain")
	}
	if exhausted || alignments != 1 {
		return nil, false
	}
	mapping := make([]int, len(cells))
	ci := 0
	for si, gi := range segments {
		grid := g.cells[gi]
		if grid.row < row && count(si+1, ci) > 0 {
			continue
		}
		if ci >= len(cells) {
			return nil, false
		}
		mapping[ci] = gi
		ci++
	}
	return mapping, ci == len(cells)
}

func (s *pdfStructureRecovery) matchTaggedGrid(table Block, nodes []*pdfStructureNode, g pdfGridRegion) (Block, bool, bool) {
	mappings := make([][]int, len(table.Rows))
	for ri := range table.Rows {
		var ok bool
		mappings[ri], ok = s.matchTaggedRow(table, nodes, g, ri)
		if !ok {
			return Block{}, false, false
		}
	}
	result := table
	result.Rows = make([]Row, len(table.Rows))
	origins := make(map[int]*Cell)
	changed := false
	for ri, row := range table.Rows {
		result.Rows[ri].RepeatHeader = row.RepeatHeader
		// Allocate enough space before retaining pointers into this row.
		result.Rows[ri].Cells = make([]Cell, 0, len(row.Cells))
		for ci, cell := range row.Cells {
			gi := mappings[ri][ci]
			grid := g.cells[gi]
			node := nodes[ri].kids[ci]
			if grid.row < ri {
				origin := origins[gi]
				if origin == nil {
					return Block{}, false, false
				}
				origin.Source.TableSpans.Continuations = append(origin.Source.TableSpans.Continuations, SourceElementReference{Row: ri, Element: node.id, Role: node.role, Tag: node.tag, Page: node.page})
				changed = true
				continue
			}
			if max(1, cell.ColSpan) != grid.cols || max(1, cell.RowSpan) != grid.rows {
				changed = true
			}
			cell.ColSpan = grid.cols
			cell.RowSpan = grid.rows
			if cell.RowSpan == 1 {
				cell.RowSpan = 0
			}
			if cell.Source == nil {
				return Block{}, false, false
			}
			src := *cell.Source
			if src.Page == 0 {
				src.Page = table.Source.Page
			}
			rect := taggedGridRect(g, grid)
			src.TableSpans = &TableSpanSource{ColSpan: "inferred", RowSpan: "inferred", Evidence: "painted_grid_adjacency", Confidence: .7, Bounds: Quad{{rect.x0, rect.y0}, {rect.x1, rect.y0}, {rect.x1, rect.y1}, {rect.x0, rect.y1}}}
			if node.attrs["ColSpan"] != nil {
				src.TableSpans.ColSpan = "explicit"
			}
			if node.attrs["RowSpan"] != nil {
				src.TableSpans.RowSpan = "explicit"
			}
			cell.Source = &src
			result.Rows[ri].Cells = append(result.Rows[ri].Cells, cell)
			origins[gi] = &result.Rows[ri].Cells[len(result.Rows[ri].Cells)-1]
		}
	}
	if _, err := tableGrid(result.Rows); err != nil {
		return Block{}, false, false
	}
	if !changed {
		return table, false, true
	}
	return result, changed, true
}
