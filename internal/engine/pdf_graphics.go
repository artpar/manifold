package engine

import (
	"fmt"
	fonts "github.com/go-text/typesetting/font"
	"github.com/go-text/typesetting/harfbuzz"
	"github.com/go-text/typesetting/language"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
	"github.com/rivo/uniseg"
	"math"
	"sort"
	"strings"
	// Paint and annotation changes do not change the font, size or language used
	// for shaping. Compare resolved faces, including explicit family mappings.
)

func sameShapingContext(a, b pdfSpan, size float64) bool {
	if a.glyphs == nil || b.glyphs == nil || a.run.Note != nil || b.run.Note != nil || a.bidi.paragraph != b.bidi.paragraph || a.bidi.level != b.bidi.level {
		return false
	}
	if a.glyphs.face != b.glyphs.face || textSize(a.run, size) != textSize(b.run, size) || (a.run.ComplexScript || a.run.Direction == "rtl") != (b.run.ComplexScript || b.run.Direction == "rtl") {
		return false
	}
	tag := func(s string) language.Language {
		if s == "" {
			s = "und"
		}
		return language.NewLanguage(s)
	}
	return tag(a.run.Language) == tag(b.run.Language) && tag(a.run.EastAsiaLanguage) == tag(b.run.EastAsiaLanguage) && tag(a.run.ComplexLanguage) == tag(b.run.ComplexLanguage)
}

type pdfPaintCluster struct {
	first, last int
	start, end  int // original UTF-8 byte interval
	pen, width  float64
	edges       map[int]float64 // original grapheme boundaries -> shaping-group em position
}

// GDEF carets describe the internal component boundaries of a ligature. Fonts
// without a matching usable list divide the cluster advance equally among its
// original graphemes. This is a declared paint allocation, not a glyph split.
func paintClusters(text string, c *pdfGlyphCluster) (out []pdfPaintCluster, err error) {
	defer func() {
		if p := recover(); p != nil {
			out = nil
			err = fmt.Errorf("%w: ligature caret lookup failed: %v", ErrUnsupported, p)
		}
	}()
	pen, consumed := 0.0, 0
	for first := 0; first < len(c.positions); {
		last := first + 1
		for last < len(c.positions) && c.positions[last].cluster == c.positions[first].cluster {
			last++
		}
		start, end := c.positions[first].textStart, c.positions[first].textEnd
		if c.positions[first].logical != "" {
			// The logical string is authoritative for independently assembled
			// clusters too; production shaping also records its original start.
			end = start + len(c.positions[first].logical)
		}
		width := 0.0
		for _, g := range c.positions[first:last] {
			width += g.advance
		}
		if !finite(width) || width < 0 {
			return nil, fmt.Errorf("%w: invalid paint cluster advance", ErrUnsupported)
		}
		cluster := pdfPaintCluster{first: first, last: last, start: start, end: end, pen: pen, width: width, edges: map[int]float64{start: pen, end: pen + width}}
		if c.rtl {
			cluster.edges[start], cluster.edges[end] = pen+width, pen
		}
		var ends []int
		graphemes := uniseg.NewGraphemes(text[start:end])
		for graphemes.Next() {
			_, b := graphemes.Positions()
			ends = append(ends, start+b)
		}
		points := make([]float64, max(0, len(ends)-1))
		for i := range points {
			points[i] = pen + width*float64(i+1)/float64(len(ends))
			if c.rtl {
				points[i] = pen + width - width*float64(i+1)/float64(len(ends))
			}
		}
		if last == first+1 && len(points) > 0 {
			g := c.positions[first]
			direction := harfbuzz.LeftToRight
			if c.rtl {
				direction = harfbuzz.RightToLeft
			}
			carets := c.face.shaper.GetOTLigatureCarets(direction, fonts.GID(g.glyph))
			if len(carets) == len(points) {
				candidate := make([]float64, len(points))
				previous := pen
				valid := true
				for i, v := range carets {
					x := g.x + float64(v)/float64(c.face.original.parsed.UnitsPerEm())
					if !finite(x) || x <= previous || x >= pen+width {
						valid = false
						break
					}
					candidate[i] = x
					previous = x
				}
				if valid {
					if c.rtl {
						for i, j := 0, len(candidate)-1; i < j; i, j = i+1, j-1 {
							candidate[i], candidate[j] = candidate[j], candidate[i]
						}
					}
					points = candidate
				}
			}
		}
		for i, p := range points {
			cluster.edges[ends[i]] = p
		}
		out = append(out, cluster)
		pen += width
		consumed = max(consumed, end)
		first = last
	}
	if consumed != len(text) {
		return nil, fmt.Errorf("%w: paint clusters omit logical text", ErrUnsupported)
	}
	return out, nil
}

func splitShapedPaint(input pdfLine, text string, c *pdfGlyphCluster, size float64) (pdfLine, error) {
	// Keep the compact single-span representation for the common uniform case.
	uniform := true
	for _, s := range input[1:] {
		uniform = uniform && samePDFStyle(input[0].run, s.run)
	}
	if uniform {
		span := input[0]
		span.text = text
		span.bidi = extendPDFBidiRange(span.bidi, input[len(input)-1].bidi)
		span.glyphs = c
		if c.level > 0 {
			c.logicalCarrier = true
		}
		span.width = c.width * textSize(span.run, size)
		for _, s := range input {
			span.ascent = math.Max(span.ascent, s.ascent)
			span.descent = math.Max(span.descent, s.descent)
		}
		span.ascent = math.Max(span.ascent, c.ascent*textSize(span.run, size))
		span.descent = math.Max(span.descent, c.descent*textSize(span.run, size))
		return pdfLine{span}, nil
	}
	clusters, err := paintClusters(text, c)
	if err != nil {
		return nil, err
	}
	edge := func(index int) (float64, error) {
		for _, cluster := range clusters {
			if p, ok := cluster.edges[index]; ok {
				return p, nil
			}
		}
		return 0, fmt.Errorf("%w: paint boundary inside an indivisible grapheme", ErrUnsupported)
	}
	var result pdfLine
	group := &pdfPaintActualGroup{text: text}
	offset := 0
	for start := 0; start < len(input); {
		end := start + 1
		var content strings.Builder
		content.WriteString(input[start].text)
		for end < len(input) && samePDFStyle(input[start].run, input[end].run) {
			content.WriteString(input[end].text)
			end++
		}
		finish := offset + content.Len()
		left, err := edge(offset)
		if err != nil {
			return nil, err
		}
		right, err := edge(finish)
		if err != nil {
			return nil, err
		}
		if c.rtl {
			left, right = right, left
		}
		part := &pdfGlyphCluster{rtl: c.rtl, level: c.level, actualGroup: group, script: c.script, face: c.face, width: right - left, ascent: c.ascent, descent: c.descent, origin: left}
		if part.width < 0 {
			return nil, fmt.Errorf("%w: invalid glyph paint interval", ErrUnsupported)
		}
		for _, cluster := range clusters {
			if cluster.start == cluster.end {
				// Continuation clusters inside the same original grapheme belong to its
				// existing paint run. No separate logical text may be manufactured.

				// PDF path operands are commonly rounded to five decimal places. Keep the
				// geometric tolerance stable while allowing one such quantization unit at joins.

				// -1 is partial/ambiguous, 0 absent and 1 covered. The union may contain
				// multiple paint operations, including duplicated adjacent-cell borders.

				// At an intersection, the crossing stroke contributes half its
				// thickness, in addition to the coordinate tolerance. Writer exports
				// commonly stop centerlines at that edge with separately rounded caps.

				// Resolve inner grids first, allowing an outer cell to own the recovered
				// inner table once. Stable geometric tie breaks avoid map-order dependence.

				// Project actual filled patches onto existing axes only. Sorting/merging joins
				// the short connector pieces to adjacent longitudinal paint before coverage is
				// tested. A patch cannot invent a new row/column or connect separate components.

				// Rulings are painted centerlines in displayed points. They are evidence for
				// inference, not a second editable representation of the PDF graphics program.

				// Small filled corner/join pieces support coverage along established ruling
				// axes. They never introduce axes or connect independent ruling components.

				if cluster.start <= offset || cluster.start > finish {
					continue
				}
			} else if cluster.start >= finish || cluster.end <= offset {
				continue
			}
			a, b := max(offset, cluster.start), min(finish, cluster.end)
			clipStart := offset > cluster.start && offset < cluster.end
			clipEnd := finish > cluster.start && finish < cluster.end
			if c.rtl {
				clipStart, clipEnd = clipEnd, clipStart
			}
			part.clipLeft = part.clipLeft || clipStart
			part.clipRight = part.clipRight || clipEnd
			for i, g := range c.positions[cluster.first:cluster.last] {
				g.x -= left
				g.logical = ""
				if i == 0 && b > a {
					g.logical = text[a:b]
				}
				part.positions = append(part.positions, g)
			}
		}
		span := input[start]
		span.text = content.String()
		span.bidi = extendPDFBidiRange(span.bidi, input[end-1].bidi)
		span.glyphs = part
		span.width = part.width * textSize(span.run, size)
		for _, s := range input[start:end] {
			span.ascent = math.Max(span.ascent, s.ascent)
			span.descent = math.Max(span.descent, s.descent)
		}
		span.ascent = math.Max(span.ascent, c.ascent*textSize(span.run, size))
		span.descent = math.Max(span.descent, c.descent*textSize(span.run, size))
		result = append(result, span)
		offset = finish
		start = end
	}
	needsCarrier := c.level > 0
	for _, span := range result {
		needsCarrier = needsCarrier || span.glyphs.clipLeft || span.glyphs.clipRight
	}
	if needsCarrier {
		for i := range result {
			c := result[i].glyphs
			c.actualGroup = nil
			c.logicalCarrier = c.level > 0 || c.clipLeft || c.clipRight
		}
	}
	return result, nil
}

const pdfRulingTolerance = .25

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
		r.warn("pdf_ruling_limit", "Merged ruling count exceeds the inference budget; text/images persist without a partial grid")
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
			r.warn("pdf_ruling_limit", "Candidate grid exceeds axis/slot inference budgets; text/images persist")
			out = append(out, g)
			continue
		}
		coverage, ok := r.rulingPatchLines(page, group, xs, ys)
		if ok {
			g.cells, g.valid = r.ruledPDFCells(coverage, xs, ys)
		}
		out = append(out, g)
	}

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
			r.warn("pdf_ruling_limit", "Projected ruling patches exceed the inference budget; candidate grid is skipped")
			return nil, false
		}
	}
	lines = mergePDFRulings(lines)
	if len(lines) > 4096 {
		r.warn("pdf_ruling_limit", "Merged ruling patches exceed the inference budget; candidate grid is skipped")
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
				sets.

					// Only state relevant to whether a ruling is painted is used here. Other
					// extended-state interpretation retains the existing explicit loss diagnostic.
					join(id, id+nx)
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
			r.warn("pdf_table_rulings_ambiguous", "Painted grid is incomplete, nonrectangular, partially divided or crossed by a content span; grid semantics are not assigned and original content persists")
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
			r.warn("pdf_table_rulings_ambiguous", "Inferred ruling grid failed shared span validation; content persists without that table")
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
		r.warn("pdf_table_rulings_inferred", "Cell boundaries, rectangular merges and empty cells are inferred from painted grid adjacency; border appearance and authorial table/header semantics are not established")
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

type pdfRuling struct {
	horizontal             bool
	pos, start, end, width float64
}
type pdfRulingRect struct{ x0, y0, x1, y1 float64 }
type pdfSubpath struct {
	points []Point
	curved bool
}
type pdfCurrentPath struct {
	parts []pdfSubpath
	clip  bool
}

func (p *pdfCurrentPath) move(point Point) {
	p.parts = append(p.parts, pdfSubpath{points: []Point{point}})
}
func (p *pdfCurrentPath) line(point Point) error {
	if len(p.parts) == 0 {
		return fmt.Errorf("PDF line/curve has no current point")
	}
	i := len(p.parts) - 1
	p.parts[i].points = append(p.parts[i].points, point)
	return nil
}
func (p *pdfCurrentPath) close() {
	if len(p.parts) == 0 {
		return
	}
	i := len(p.parts) - 1
	points := p.parts[i].points
	if len(points) > 1 && points[len(points)-1] != points[0] {
		p.parts[i].points = append(points, points[0])
	}
}
func rulingRect(points []Point) (pdfRulingRect, bool) {
	if len(points) == 4 {
		points = append(append([]Point{}, points...), points[0])
	}
	if len(points) != 5 || math.Hypot(points[0].X-points[4].X, points[0].Y-points[4].Y) > 1e-6 {
		return pdfRulingRect{}, false
	}
	x0, y0, x1, y1 := quadBounds(Quad{points[0], points[1], points[2], points[3]})
	if x1 <= x0 || y1 <= y0 {
		return pdfRulingRect{}, false
	}
	seen := 0
	for _, p := range points[:4] {
		corner := 0
		if math.Abs(p.X-x1) < 1e-6 {
			corner |= 1
		} else if math.Abs(p.X-x0) >= 1e-6 {
			return pdfRulingRect{}, false
		}
		if math.Abs(p.Y-y1) < 1e-6 {
			corner |= 2
		} else if math.Abs(p.Y-y0) >= 1e-6 {
			return pdfRulingRect{}, false
		}
		if seen&(1<<corner) != 0 {
			return pdfRulingRect{}, false
		}
		seen |= 1 << corner
	}
	for i := 1; i < 5; i++ {
		a, b := points[i-1], points[i]
		if math.Abs(a.X-b.X) > 1e-6 && math.Abs(a.Y-b.Y) > 1e-6 {
			return pdfRulingRect{}, false
		}
	}
	return pdfRulingRect{x0, y0, x1, y1}, true
}
func intersectRulingClip(a, b *pdfRulingRect) *pdfRulingRect {
	if a == nil {
		c := *b
		return &c
	}
	if b == nil {
		c := *a
		return &c
	}
	return &pdfRulingRect{max(a.x0, b.x0), max(a.y0, b.y0), min(a.x1, b.x1), min(a.y1, b.y1)}
}
func (r *pdfReader) addRuling(a, b Point, width float64, state pdfTextState) {
	if !finite(a.X) || !finite(a.Y) || !finite(b.X) || !finite(b.Y) || !finite(width) {
		r.warnPage("pdf_ruling_geometry", "Nonfinite transformed ruling geometry is not used for inference")
		return
	}
	if state.rulingEffects || state.rulingClipUnknown {
		return
	}
	line := pdfRuling{width: width}
	switch {
	case math.Abs(a.Y-b.Y) < 1e-6 && math.Abs(a.X-b.X) > 1e-6:
		line.horizontal = true
		line.pos = (a.Y + b.Y) / 2
		line.start = min(a.X, b.X)
		line.end = max(a.X, b.X)
	case math.Abs(a.X-b.X) < 1e-6 && math.Abs(a.Y-b.Y) > 1e-6:
		line.pos = (a.X + b.X) / 2
		line.start = min(a.Y, b.Y)
		line.end = max(a.Y, b.Y)
	default:
		return
	}
	clip := intersectRulingClip(state.rulingClip, &pdfRulingRect{0, 0, r.pageInfo.Width, r.pageInfo.Height})
	if clip.x1 <= clip.x0 || clip.y1 <= clip.y0 {
		return
	}
	if line.horizontal {
		if line.pos < clip.y0 || line.pos > clip.y1 {
			return
		}
		line.start = max(line.start, clip.x0)
		line.end = min(line.end, clip.x1)
	} else {
		if line.pos < clip.x0 || line.pos > clip.x1 {
			return
		}
		line.start = max(line.start, clip.y0)
		line.end = min(line.end, clip.y1)
	}
	if line.end <= line.start {
		return
	}
	if !r.admitRuling() {
		return
	}
	r.rulings[r.page] = append(r.rulings[r.page], line)
}

func (r *pdfReader) addRulingPatch(rect pdfRulingRect, state pdfTextState) {
	if state.rulingEffects || state.rulingClipUnknown {
		return
	}
	if !finite(rect.x0) || !finite(rect.y0) || !finite(rect.x1) || !finite(rect.y1) {
		r.warnPage("pdf_ruling_geometry", "Nonfinite transformed ruling geometry is not used for inference")
		return
	}
	clip := intersectRulingClip(state.rulingClip, &pdfRulingRect{0, 0, r.pageInfo.Width, r.pageInfo.Height})
	rect = pdfRulingRect{max(rect.x0, clip.x0), max(rect.y0, clip.y0), min(rect.x1, clip.x1), min(rect.y1, clip.y1)}
	if rect.x1 <= rect.x0 || rect.y1 <= rect.y0 || !r.admitRuling() {
		return
	}
	r.rulingPatches[r.page] = append(r.rulingPatches[r.page], rect)
}

func (r *pdfReader) admitRuling() bool {
	if r.rulingOverflow == nil {
		r.rulingOverflow = map[int]bool{}
		r.rulings = map[int][]pdfRuling{}
		r.rulingPatches = map[int][]pdfRulingRect{}
	}
	if r.rulingOverflow[r.page] {
		return false
	}
	if len(r.rulings[r.page])+len(r.rulingPatches[r.page]) >= 100_000 {
		r.rulingOverflow[r.page] = true
		r.rulings[r.page] = nil
		r.rulingPatches[r.page] = nil
		r.warnPage("pdf_ruling_limit", "Painted line and patch count exceeds the per-page inference budget; ruled-table inference is skipped while text/images persist")
		return false
	}
	return true
}

func (r *pdfReader) pathOperator(op string, args []any, path *pdfCurrentPath, state *pdfTextState) error {
	need := func(n int) error {
		if len(args) != n {
			return fmt.Errorf("PDF operator %s expects %d operands, got %d", op, n, len(args))
		}
		return nil
	}
	nums := func(n int) ([]float64, error) {
		if err := need(n); err != nil {
			return nil, err
		}
		out := make([]float64, n)
		for i, arg := range args {
			v, e := pdfNumber(arg)
			if e != nil || !finite(v) {
				return nil, fmt.Errorf("invalid PDF path operand")
			}
			out[i] = v
		}
		return out, nil
	}
	m := r.pageInfo.Transform.compose(state.ctm)
	switch op {
	case "m", "l":
		a, e := nums(2)
		if e != nil {
			return e
		}
		point := m.point(a[0], a[1])
		if op == "m" {
			path.move(point)
		} else {
			return path.line(point)
		}
	case "c", "v", "y":
		n := 4
		if op == "c" {
			n = 6
		}
		a, e := nums(n)
		if e != nil {
			return e
		}
		if err := path.line(m.point(a[n-2], a[n-1])); err != nil {
			return err
		}
		path.parts[len(path.parts)-1].curved = true
	case "re":
		a, e := nums(4)
		if e != nil {
			return e
		}
		x, y, w, h := a[0], a[1], a[2], a[3]
		path.move(m.point(x, y))
		for _, p := range []Point{m.point(x+w, y), m.point(x+w, y+h), m.point(x, y+h)} {
			if err := path.line(p); err != nil {
				return err
			}
		}
		path.close()
	case "h":
		if err := need(0); err != nil {
			return err
		}
		path.close()
	case "W", "W*":
		if err := need(0); err != nil {
			return err
		}
		path.clip = true
	case "w":
		a, e := nums(1)
		if e != nil {
			return e
		}
		if a[0] < 0 {
			return fmt.Errorf("negative PDF line width")
		}
		state.rulingWidth = a[0]
	default:
		if err := need(0); err != nil {
			return err
		}
		stroke := op == "S" || op == "s" || op == "B" || op == "B*" || op == "b" || op == "b*"
		fill := op == "f" || op == "F" || op == "f*" || op == "B" || op == "B*" || op == "b" || op == "b*"
		if op == "s" || op == "b" || op == "b*" {
			path.close()
		}
		for _, part := range path.parts {
			if part.curved {
				continue
			}
			if stroke && state.rulingStrokeAlpha > 0 {
				for i := 1; i < len(part.points); i++ {
					a, b := part.points[i-1], part.points[i]
					scale := math.Hypot(m[2], m[3])
					if math.Abs(a.X-b.X) < 1e-6 {
						scale = math.Hypot(m[0], m[1])
					}
					r.addRuling(a, b, state.rulingWidth*scale, *state)
				}
			}
			if fill && state.rulingFillAlpha > 0 {
				points := part.points
				if len(points) == 4 {
					points = append(append([]Point{}, points...), points[0])
				}
				if rect, ok := rulingRect(points); ok {
					w, h := rect.x1-rect.x0, rect.y1-rect.y0
					if w <= 3 && h <= 3 {
						r.addRulingPatch(rect, *state)
					}
					if h <= 3 && w >= max(4, h*5) {
						r.addRuling(Point{rect.x0, (rect.y0 + rect.y1) / 2}, Point{rect.x1, (rect.y0 + rect.y1) / 2}, h, *state)
					}
					if w <= 3 && h >= max(4, w*5) {
						r.addRuling(Point{(rect.x0 + rect.x1) / 2, rect.y0}, Point{(rect.x0 + rect.x1) / 2, rect.y1}, w, *state)
					}
				}
			}
		}
		if path.clip {
			if len(path.parts) == 1 && !path.parts[0].curved {
				if rect, ok := rulingRect(path.parts[0].points); ok {
					state.rulingClip = intersectRulingClip(state.rulingClip, &rect)
				} else {
					state.rulingClipUnknown = true
				}
			} else {
				state.rulingClipUnknown = true
			}
		}
		*path = pdfCurrentPath{}
		if op != "n" {
			r.warnPage("pdf_vector_graphics", "Painted vector appearance is not preserved as editable content; eligible rulings may inform inferred tables")
		}
	}
	return nil
}

func (r *pdfReader) rulingGraphicsState(args []any, resources types.Dict, state *pdfTextState) error {
	if len(args) != 1 {
		return fmt.Errorf("PDF gs expects one name")
	}
	name, err := pdfName(args[0])
	if err != nil {
		return err
	}
	states, err := r.pdf.DereferenceDict(resources["ExtGState"])
	if err != nil {
		return err
	}
	if states[name] == nil {
		return fmt.Errorf("missing PDF ExtGState %q", name)
	}
	d, err := r.pdf.DereferenceDict(states[name])
	if err != nil || d == nil {
		return fmt.Errorf("invalid PDF ExtGState")
	}
	for _, entry := range []struct {
		key    string
		target *float64
	}{{"CA", &state.rulingStrokeAlpha}, {"ca", &state.rulingFillAlpha}, {"LW", &state.rulingWidth}} {
		if value := d[entry.key]; value != nil {
			n, e := r.pdf.DereferenceNumber(value)
			if e != nil || !finite(n) || n < 0 || (entry.key != "LW" && n > 1) {
				return fmt.Errorf("invalid PDF ExtGState %s", entry.key)
			}
			*entry.target = n
		}
	}
	for _, key := range []string{"SMask", "BM", "TR", "TR2"} {
		if value := d[key]; value != nil {
			v, e := r.pdf.Dereference(value)
			if e != nil {
				return e
			}
			n, ok := v.(types.Name)
			safe := ok && ((key == "SMask" && n == "None") || (key == "BM" && (n == "Normal" || n == "Compatible")) || ((key == "TR" || key == "TR2") && (n == "Identity" || n == "Default")))
			switch key {
			case "SMask":
				state.rulingMask = !safe
			case "BM":
				state.rulingBlend = !safe
			default:
				state.rulingTransfer = !safe
			}
		}
	}
	state.rulingEffects = state.rulingMask || state.rulingBlend || state.rulingTransfer
	return nil
}
