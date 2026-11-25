package manifold

import (
	"math"
	"sort"
)

const pdfRulingTolerance = .25

// PDF path operands are commonly rounded to five decimal places. Keep the
// geometric tolerance stable while allowing one such quantization unit at joins.
const pdfRulingEpsilon = 1e-5

type pdfGridRegion struct {
	rect   pdfRulingRect
	xs, ys []float64
	cells  []pdfGridCell
	valid  bool
}
type pdfGridCell struct{ row, col, rows, cols int }
type pdfGridSets []int

func newPDFGridSets(n int) pdfGridSets {
	s := make(pdfGridSets, n)
	for i := range s {
		s[i] = i
	}
	return s
}
func (s pdfGridSets) root(i int) int {
	for s[i] != i {
		s[i] = s[s[i]]
		i = s[i]
	}
	return i
}
func (s pdfGridSets) join(a, b int) {
	a, b = s.root(a), s.root(b)
	if a != b {
		s[b] = a
	}
}

func mergePDFRulings(lines []pdfRuling) []pdfRuling {
	lines = append([]pdfRuling{}, lines...)
	sort.Slice(lines, func(i, j int) bool {
		a, b := lines[i], lines[j]
		if a.horizontal != b.horizontal {
			return !a.horizontal
		}
		if a.pos != b.pos {
			return a.pos < b.pos
		}
		return a.start < b.start
	})
	var out []pdfRuling
	for i := 0; i < len(lines); {
		j := i + 1
		for j < len(lines) && lines[j].horizontal == lines[i].horizontal && lines[j].pos-lines[i].pos <= pdfRulingTolerance {
			j++
		}
		band := append([]pdfRuling{}, lines[i:j]...)
		pos := lines[i].pos
		sort.Slice(band, func(i, j int) bool { return band[i].start < band[j].start })
		for k, line := range band {
			line.pos = pos
			if k > 0 && line.start <= out[len(out)-1].end+pdfRulingTolerance {
				last := &out[len(out)-1]
				last.end = max(last.end, line.end)
				last.width = max(last.width, line.width)
			} else {
				out = append(out, line)
			}
		}
		i = j
	}
	return out
}
func rulingCoordinates(values []float64) []float64 {
	sort.Float64s(values)
	var out []float64
	for _, v := range values {
		if len(out) == 0 || v-out[len(out)-1] > pdfRulingTolerance {
			out = append(out, v)
		}
	}
	return out
}

// -1 is partial/ambiguous, 0 absent and 1 covered. The union may contain
// multiple paint operations, including duplicated adjacent-cell borders.
func rulingCoverage(lines []pdfRuling, horizontal bool, pos, start, end float64) int {
	startJoin, endJoin := pdfRulingTolerance, pdfRulingTolerance
	for _, l := range lines {
		if l.horizontal == horizontal || pos < l.start-pdfRulingTolerance-pdfRulingEpsilon || pos > l.end+pdfRulingTolerance+pdfRulingEpsilon {
			continue
		}
		if math.Abs(l.pos-start) <= pdfRulingTolerance+pdfRulingEpsilon {
			startJoin = max(startJoin, l.width/2+pdfRulingTolerance)
		}
		if math.Abs(l.pos-end) <= pdfRulingTolerance+pdfRulingEpsilon {
			endJoin = max(endJoin, l.width/2+pdfRulingTolerance)
		}
	}
	var intervals [][2]float64
	for _, l := range lines {
		if l.horizontal == horizontal && math.Abs(l.pos-pos) <= pdfRulingTolerance+pdfRulingEpsilon && l.end > start && l.start < end {
			if l.start >= end-max(endJoin, l.width/2)-pdfRulingEpsilon || l.end <= start+max(startJoin, l.width/2)+pdfRulingEpsilon {
				continue
			}
			// At an intersection, the crossing stroke contributes half its
			// thickness, in addition to the coordinate tolerance. Writer exports
			// commonly stop centerlines at that edge with separately rounded caps.
			a, b := max(start, l.start), min(end, l.end)
			if a <= start+max(startJoin, l.width/2)+pdfRulingEpsilon {
				a = start
			}
			if b >= end-max(endJoin, l.width/2)-pdfRulingEpsilon {
				b = end
			}
			intervals = append(intervals, [2]float64{a, b})
		}
	}
	sort.Slice(intervals, func(i, j int) bool { return intervals[i][0] < intervals[j][0] })
	covered, reach := false, start
	for _, v := range intervals {
		if v[1]-v[0] <= 1e-7 {
			continue
		}
		covered = true
		if v[0] > reach+pdfRulingTolerance+pdfRulingEpsilon {
			return -1
		}
		reach = max(reach, v[1])
	}
	if !covered {
		return 0
	}
	if reach >= end-pdfRulingTolerance-1e-7 {
		return 1
	}
	return -1
}

func (r *pdfReader) rulingRegions(page int) []pdfGridRegion {
	lines := mergePDFRulings(r.rulings[page])
	if len(lines) > 4096 {
		r.doc.warn("pdf_ruling_limit", "Merged ruling count exceeds the inference budget; text/images persist without a partial grid")
		return nil
	}
	sets := newPDFGridSets(len(lines))
	for i, a := range lines {
		if r.ctx.Err() != nil {
			return nil
		}
		for j := 0; j < i; j++ {
			b := lines[j]
			if a.horizontal == b.horizontal {
				continue
			}
			h, v := a, b
			if !h.horizontal {
				h, v = v, h
			}
			tol := max(pdfRulingTolerance, (h.width+v.width)/2) + 1e-7
			if v.pos >= h.start-tol && v.pos <= h.end+tol && h.pos >= v.start-tol && h.pos <= v.end+tol {
				sets.join(i, j)
			}
		}
	}
	components := map[int][]pdfRuling{}
	for i, line := range lines {
		key := sets.root(i)
		components[key] = append(components[key], line)
	}
	var out []pdfGridRegion
	for _, group := range components {
		if r.ctx.Err() != nil {
			return nil
		}
		var xs, ys []float64
		for _, line := range group {
			if line.horizontal {
				ys = append(ys, line.pos)
			} else {
				xs = append(xs, line.pos)
			}
		}
		xs, ys = rulingCoordinates(xs), rulingCoordinates(ys)
		if len(xs) < 2 || len(ys) < 2 || len(xs) == 2 && len(ys) == 2 {
			continue
		}
		g := pdfGridRegion{rect: pdfRulingRect{xs[0], ys[0], xs[len(xs)-1], ys[len(ys)-1]}, xs: xs, ys: ys}
		if len(xs) > 1001 || len(ys) > 1001 || (len(xs)-1)*(len(ys)-1) > 100_000 {
			r.doc.warn("pdf_ruling_limit", "Candidate grid exceeds axis/slot inference budgets; text/images persist")
			out = append(out, g)
			continue
		}
		coverage, ok := r.rulingPatchLines(page, group, xs, ys)
		if ok {
			g.cells, g.valid = r.ruledPDFCells(coverage, xs, ys)
		}
		out = append(out, g)
	}
	// Resolve inner grids first, allowing an outer cell to own the recovered
	// inner table once. Stable geometric tie breaks avoid map-order dependence.
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i].rect, out[j].rect
		aa, bb := (a.x1-a.x0)*(a.y1-a.y0), (b.x1-b.x0)*(b.y1-b.y0)
		if aa != bb {
			return aa < bb
		}
		if a.y0 != b.y0 {
			return a.y0 < b.y0
		}
		return a.x0 < b.x0
	})
	return out
}

// Project actual filled patches onto existing axes only. Sorting/merging joins
// the short connector pieces to adjacent longitudinal paint before coverage is
// tested. A patch cannot invent a new row/column or connect separate components.
func (r *pdfReader) rulingPatchLines(page int, group []pdfRuling, xs, ys []float64) ([]pdfRuling, bool) {
	if len(r.rulingPatches[page]) == 0 {
		return group, true
	}
	lines := append([]pdfRuling{}, group...)
	for _, p := range r.rulingPatches[page] {
		if r.ctx.Err() != nil {
			return nil, false
		}
		if p.x1 < xs[0] || p.x0 > xs[len(xs)-1] || p.y1 < ys[0] || p.y0 > ys[len(ys)-1] {
			continue
		}
		for i := sort.SearchFloat64s(ys, p.y0-pdfRulingEpsilon); i < len(ys) && ys[i] <= p.y1+pdfRulingEpsilon; i++ {
			lines = append(lines, pdfRuling{horizontal: true, pos: ys[i], start: p.x0, end: p.x1, width: p.y1 - p.y0})
		}
		for i := sort.SearchFloat64s(xs, p.x0-pdfRulingEpsilon); i < len(xs) && xs[i] <= p.x1+pdfRulingEpsilon; i++ {
			lines = append(lines, pdfRuling{pos: xs[i], start: p.y0, end: p.y1, width: p.x1 - p.x0})
		}
		if len(lines) > 100000 {
			r.doc.warn("pdf_ruling_limit", "Projected ruling patches exceed the inference budget; candidate grid is skipped")
			return nil, false
		}
	}
	lines = mergePDFRulings(lines)
	if len(lines) > 4096 {
		r.doc.warn("pdf_ruling_limit", "Merged ruling patches exceed the inference budget; candidate grid is skipped")
		return nil, false
	}
	return lines, true
}

func (r *pdfReader) ruledPDFCells(lines []pdfRuling, xs, ys []float64) ([]pdfGridCell, bool) {
	nx, ny := len(xs)-1, len(ys)-1
	vertical := make([][]int, ny)
	horizontal := make([][]int, ny+1)
	for row := 0; row < ny; row++ {
		if r.ctx.Err() != nil {
			return nil, false
		}
		vertical[row] = make([]int, nx+1)
		for col := 0; col <= nx; col++ {
			v := rulingCoverage(lines, false, xs[col], ys[row], ys[row+1])
			if v < 0 || ((col == 0 || col == nx) && v != 1) {
				return nil, false
			}
			vertical[row][col] = v
		}
	}
	for row := 0; row <= ny; row++ {
		if r.ctx.Err() != nil {
			return nil, false
		}
		horizontal[row] = make([]int, nx)
		for col := 0; col < nx; col++ {
			v := rulingCoverage(lines, true, ys[row], xs[col], xs[col+1])
			if v < 0 || ((row == 0 || row == ny) && v != 1) {
				return nil, false
			}
			horizontal[row][col] = v
		}
	}
	sets := newPDFGridSets(nx * ny)
	for row := 0; row < ny; row++ {
		for col := 0; col < nx; col++ {
			id := row*nx + col
			if col+1 < nx && vertical[row][col+1] == 0 {
				sets.join(id, id+1)
			}
			if row+1 < ny && horizontal[row+1][col] == 0 {
				sets.join(id, id+nx)
			}
		}
	}
	type extent struct{ r0, c0, r1, c1, count int }
	groups := map[int]extent{}
	for row := 0; row < ny; row++ {
		for col := 0; col < nx; col++ {
			key := sets.root(row*nx + col)
			g, ok := groups[key]
			if !ok {
				g = extent{row, col, row, col, 0}
			}
			g.r0 = min(g.r0, row)
			g.c0 = min(g.c0, col)
			g.r1 = max(g.r1, row)
			g.c1 = max(g.c1, col)
			g.count++
			groups[key] = g
			if col+1 < nx && sets.root(row*nx+col+1) == key && vertical[row][col+1] != 0 {
				return nil, false
			}
			if row+1 < ny && sets.root((row+1)*nx+col) == key && horizontal[row+1][col] != 0 {
				return nil, false
			}
		}
	}
	var cells []pdfGridCell
	for _, g := range groups {
		if g.count != (g.r1-g.r0+1)*(g.c1-g.c0+1) {
			return nil, false
		}
		cells = append(cells, pdfGridCell{g.r0, g.c0, g.r1 - g.r0 + 1, g.c1 - g.c0 + 1})
	}
	sort.Slice(cells, func(i, j int) bool {
		if cells[i].row != cells[j].row {
			return cells[i].row < cells[j].row
		}
		return cells[i].col < cells[j].col
	})
	return cells, true
}

func rulingBoundsSource(page int, rect pdfRulingRect, bs []Block, role string) *SourceInfo {
	s := inferredPDFSource(bs, "painted_grid_adjacency", .7)
	q := Quad{{rect.x0, rect.y0}, {rect.x1, rect.y0}, {rect.x1, rect.y1}, {rect.x0, rect.y1}}
	s.Page, s.Quad, s.Regions, s.Role, s.Precision = page, &q, []SourceRegion{{Page: page, Quad: q}}, role, "ruling_boundary_bounds"
	return s
}
func rulingContains(a, b pdfRulingRect) bool {
	return b.x0 >= a.x0-pdfRulingTolerance && b.x1 <= a.x1+pdfRulingTolerance && b.y0 >= a.y0-pdfRulingTolerance && b.y1 <= a.y1+pdfRulingTolerance
}
func rulingIntersects(a, b pdfRulingRect) bool {
	return b.x1 > a.x0+1e-7 && b.x0 < a.x1-1e-7 && b.y1 > a.y0+1e-7 && b.y0 < a.y1-1e-7
}

func (r *pdfReader) inferRuledTables(boxes []pdfLayoutBox, page int, em float64) []pdfLayoutBox {
	for _, g := range r.rulingRegions(page) {
		if r.ctx.Err() != nil {
			return boxes
		}
		valid := g.valid
		content := make([][]pdfLayoutBox, len(g.cells))
		consumed := map[int]bool{}
		for i, b := range boxes {
			if r.ctx.Err() != nil {
				return boxes
			}
			rect := pdfRulingRect{b.x0, b.y0, b.x1, b.y1}
			if !rulingIntersects(g.rect, rect) {
				continue
			}
			found := -1
			if rulingContains(g.rect, rect) {
				for j, cell := range g.cells {
					c := pdfRulingRect{g.xs[cell.col], g.ys[cell.row], g.xs[cell.col+cell.cols], g.ys[cell.row+cell.rows]}
					if rulingContains(c, rect) {
						if found >= 0 {
							found = -2
							break
						}
						found = j
					}
				}
			}
			if found < 0 {
				valid = false
				continue
			}
			content[found] = append(content[found], b)
			consumed[i] = true
		}
		if !valid {
			r.doc.warn("pdf_table_rulings_ambiguous", "Painted grid is incomplete, nonrectangular, partially divided or crossed by a content span; grid semantics are not assigned and original content persists")
			for i, b := range boxes {
				if rulingIntersects(g.rect, pdfRulingRect{b.x0, b.y0, b.x1, b.y1}) {
					boxes[i].tableBarrier = true
				}
			}
			continue
		}
		table := Block{Kind: Table, Rows: make([]Row, len(g.ys)-1), ColumnWidths: gridColumnWidths(g)}
		for i, c := range g.cells {
			ordered := r.layoutOrder(content[i], em, "", 0)
			lines := pdfLayoutLines(ordered)
			bs := r.inferFlow(lines)
			if len(bs) == 0 {
				bs = []Block{{Kind: Paragraph}}
			}
			rect := pdfRulingRect{g.xs[c.col], g.ys[c.row], g.xs[c.col+c.cols], g.ys[c.row+c.rows]}
			source := rulingBoundsSource(page, rect, bs, "TD")
			if len(content[i]) == 0 {
				bs[0].Source = rulingBoundsSource(page, rect, nil, "P")
			}
			rs := c.rows
			if rs == 1 {
				rs = 0
			}
			table.Rows[c.row].Cells = append(table.Rows[c.row].Cells, Cell{ColSpan: c.cols, RowSpan: rs, Blocks: bs, Source: source})
		}
		if _, err := tableGrid(table.Rows); err != nil {
			r.doc.warn("pdf_table_rulings_ambiguous", "Inferred ruling grid failed shared span validation; content persists without that table")
			continue
		}
		table.Source = rulingBoundsSource(page, g.rect, []Block{table}, "Table")
		table.Source.TableWidths = gridWidthSource(g)
		var remaining []pdfLayoutBox
		for i, b := range boxes {
			if !consumed[i] {
				remaining = append(remaining, b)
			}
		}
		b, _ := pdfLayoutBlock(table)
		boxes = append(remaining, b)
		r.doc.warn("pdf_table_rulings_inferred", "Cell boundaries, rectangular merges and empty cells are inferred from painted grid adjacency; border appearance and authorial table/header semantics are not established")
	}
	return boxes
}

func gridColumnWidths(g pdfGridRegion) []float64 {
	widths := make([]float64, len(g.xs)-1)
	for i := range widths {
		widths[i] = g.xs[i+1] - g.xs[i]
	}
	return widths
}
func gridWidthSource(g pdfGridRegion) *TableWidthSource {
	return &TableWidthSource{Evidence: "painted_grid_adjacency", Confidence: .7, Bounds: Quad{{g.rect.x0, g.rect.y0}, {g.rect.x1, g.rect.y0}, {g.rect.x1, g.rect.y1}, {g.rect.x0, g.rect.y1}}}
}
