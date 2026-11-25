package engine

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"github.com/phpdave11/gofpdf"
	"math"
)

// Text/image fragments are indivisible. A table fragment can supply a smaller
// measured page through recursive continuation. Text defaults to 10pt/1.4 in the
// compact cell/note style; running stories carry their measured style.
// Measurement has no drawing or page-admission effects.
type pdfFragment struct {
	Anchors     []string
	TextSize    float64 // zero keeps the compact 10pt default
	Line        pdfLine
	Indent      float64
	Prefix      pdfLine
	PrefixX     float64
	Height      float64
	Image       *pdfImageLayout
	Table       *pdfTableSlice
	Break       bool
	Conditional bool
}
type pdfImageLayout struct {
	image         *Image
	kind          string
	width, height float64
}
type pdfTableLayout struct {
	borderless             bool // parallel list columns; genuine nested tables retain borders
	width, height          float64
	columns                []float64
	anchors                []string
	offsets                []float64
	cells                  []pdfMeasuredCell
	groups                 []pdfTableGroup
	header                 *pdfTableHeader
	outputHeaderPainted    bool
	headerRows, headerLast int
}

// Header copies share original measured geometry; they never own body cursors.
type pdfTableHeader struct {
	group  pdfTableGroup
	height float64
	keys   []noteKey
}
type pdfMeasuredCell struct {
	position *tableCellPosition
	flow     []pdfFragment
	height   float64
}
type pdfTableGroup struct {
	start, end, first, last int
	height                  float64
	keys                    []noteKey
	breaks                  bool
}
type pdfTableSlice struct {
	table  *pdfTableLayout
	group  pdfTableGroup
	cursor *pdfTableCursor // nil for an original, unsplit group
	page   *pdfTablePage   // an immutable, measured placement selected for drawing
	gap    float64         // original trailing spacing, retained by the last part
}

func flowHeight(flow []pdfFragment) float64 {
	h := 0.0
	for _, f := range flow {
		h += f.Height
	}
	return h
}

func flowHasBreak(flow []pdfFragment) bool {
	for _, f := range flow {
		if f.Break || f.Table != nil && f.Table.group.breaks {
			return true
		}
	}
	return false
}
func flowHasImage(flow []pdfFragment) bool {
	for _, f := range flow {
		if f.Image != nil {
			return true
		}
		if f.Table != nil {
			for _, c := range f.Table.table.cells[f.Table.group.first:f.Table.group.last] {
				if flowHasImage(c.flow) {
					return true
				}
			}
		}
	}
	return false
}
func flowNoteKeys(flow []pdfFragment) []noteKey {
	var keys []noteKey
	for _, f := range flow {
		keys = append(keys, lineNoteKeys(f.Line)...)
		if f.Table != nil {
			if f.Table.page != nil {
				keys = append(keys, f.Table.page.keys...)
			} else {
				keys = append(keys, f.Table.group.keys...)
			}
		}
	}
	return keys
}

var errPDFImageSpace = errors.New("image container is too small")

func (r *pdfRenderer) measureImage(im *Image, width, height float64) (*pdfImageLayout, error) {
	if err := r.ctx.Err(); err != nil {
		return nil, err
	}
	if im == nil || !finite(width) || width <= 0 || !finite(height) || height <= 0 {
		return nil, fmt.Errorf("%w: no space for PDF image: %w", ErrUnsupported, errPDFImageSpace)
	}
	config, kind, err := decodeOutputImage(r.ctx, im.Data, r.validatedImages)
	if err != nil {
		return nil, err
	}
	if kind != "png" && kind != "jpeg" {
		return nil, fmt.Errorf("%w: PDF image %s", ErrUnsupported, kind)
	}
	iw, ih, err := fitImageBox(im, config.Width, config.Height, width, height)
	if err != nil {
		return nil, err
	}
	if fmt.Sprintf("%.5f", iw) == "0.00000" || fmt.Sprintf("%.5f", ih) == "0.00000" {
		return nil, fmt.Errorf("%w: fitted PDF image is below coordinate precision: %w", ErrUnsupported, errPDFImageSpace)
	}
	return &pdfImageLayout{im, kind, iw, ih}, nil
}
func (r *pdfRenderer) drawImage(im *pdfImageLayout, x, y float64) error {
	if err := r.ctx.Err(); err != nil {
		return err
	}
	name := ""
	var digest [32]byte
	if r.paintingRunning {
		digest = sha256.Sum256(im.image.Data)
		name = r.runningImages[digest]
	}
	if name == "" {
		r.images++
		name = fmt.Sprintf("image%d", r.images)
		r.pdf.RegisterImageOptionsReader(name, gofpdf.ImageOptions{ImageType: im.kind}, bytes.NewReader(im.image.Data))
		if r.paintingRunning {
			if r.runningImages == nil {
				r.runningImages = make(map[[32]byte]string)
			}
			r.runningImages[digest] = name
		}
	}
	marked := r.beginOutputMark(im.image.pdfOutput)
	r.pdf.ImageOptions(name, x, y, im.width, im.height, false, gofpdf.ImageOptions{ImageType: im.kind}, 0, "")
	if marked {
		r.pdf.RawWriteStr("EMC")
	}
	return r.pdf.Error()
}

type pdfFlowStyle struct {
	size, leading float64
	running       bool
}

func (r *pdfRenderer) cellFlow(bs []Block, width, indent, height float64) ([]pdfFragment, error) {
	return r.cellFlowStyle(bs, width, indent, height, pdfFlowStyle{size: 10, leading: 1.4})
}
func (r *pdfRenderer) cellFlowStyle(bs []Block, width, indent, height float64, style pdfFlowStyle) ([]pdfFragment, error) {
	if !finite(width) || width <= indent {
		return nil, fmt.Errorf("%w: PDF cell indentation/padding leaves no content width", ErrUnsupported)
	}
	var flow []pdfFragment
	for _, b := range legacyLists(bs) {
		if err := r.ctx.Err(); err != nil {
			return nil, err
		}
		start := len(flow)
		switch b.Kind {
		case AnchorPoint:
			flow = append(flow, pdfFragment{})
		case PageBreak:
			if len(b.Anchors) > 0 {
				flow = append(flow, pdfFragment{Anchors: b.Anchors})
			}
			flow = append(flow, pdfFragment{Break: true})
		case Paragraph, Heading:
			if b.PageBreakBefore {
				flow = append(flow, pdfFragment{Break: true, Conditional: true})
			}
			runs := append([]Run(nil), b.Runs...)
			if b.Kind == Heading {
				for i := range runs {
					runs[i].Bold = true
				}
			}
			lines, err := r.linesDirection(runs, width-indent, style.size, b.Direction)
			if err != nil {
				return nil, err
			}
			for _, line := range lines {
				flow = append(flow, pdfFragment{Line: line, Indent: indent, TextSize: style.size, Height: pdfLineHeight(line, style.size, style.leading)})
			}
		case ListBlock:
			for index, item := range b.Items {
				if item.Marker != nil {
					plan, err := r.measureLiteralItem(item, width-indent, height, style)
					if err != nil {
						return nil, err
					}
					for _, g := range plan.groups {
						flow = append(flow, pdfFragment{Table: &pdfTableSlice{table: plan, group: g}, Indent: indent, Height: g.height})
					}
					continue
				}
				marker, step, err := r.listMarker(b, index, width-indent, style.size)
				if err != nil {
					return nil, err
				}
				childWidth, childIndent, prefixX := width, indent+step, indent
				if item.Blocks[0].Direction == "rtl" {
					childWidth, childIndent, prefixX = width-step, indent, width-pdfAdvance(marker)
				}
				children, err := r.cellFlowStyle(item.Blocks, childWidth, childIndent, height, style)
				if err != nil {
					return nil, err
				}
				children[0].Prefix, children[0].PrefixX = marker, prefixX
				flow = append(flow, children...)
			}
		case Picture:
			im, err := r.measureImage(b.Image, width-indent, height-8)
			if err != nil {
				return nil, err
			}
			flow = append(flow, pdfFragment{Image: im, Indent: indent, Height: im.height + 8})
		case Table:
			plan, err := r.measureTableStyle(b, width-indent, height-8, style)
			if err != nil {
				return nil, err
			}
			for i, g := range plan.groups {
				height := g.height
				if i == len(plan.groups)-1 {
					height += 8
				}
				flow = append(flow, pdfFragment{Table: &pdfTableSlice{table: plan, group: g, gap: height - g.height}, Indent: indent, Height: height})
			}
		default:
			return nil, fmt.Errorf("%w: unknown PDF cell/footnote block %s", ErrUnsupported, b.Kind)
		}
		if b.Kind != PageBreak {
			for i := start; i < len(flow); i++ {
				if !flow[i].Break {
					flow[i].Anchors = append(append([]string(nil), b.Anchors...), flow[i].Anchors...)
					break
				}
			}
		}
	}
	if len(flow) == 0 {
		flow = append(flow, pdfFragment{TextSize: style.size, Height: style.size * style.leading})
	}
	return flow, nil
}

func (r *pdfRenderer) drawFragment(f pdfFragment, x, y float64) error {
	if err := r.ctx.Err(); err != nil {
		return err
	}
	r.bindDestinations(f.Anchors, y)
	if f.Image != nil {
		if err := r.drawImage(f.Image, x+f.Indent, y); err != nil {
			return err
		}
	}
	if f.Table != nil {
		var err error
		if f.Table.page != nil {
			err = r.paintTablePage(f.Table.table, *f.Table.page, x+f.Indent, y)
		} else {
			err = r.paintTableGroup(f.Table.table, f.Table.group, x+f.Indent, y)
		}
		if err != nil {
			return err
		}
	}
	size := f.TextSize
	if size == 0 {
		size = 10
	}
	r.drawLine(f.Line, x+f.Indent, y, size)
	if len(f.Prefix) > 0 {
		r.drawLine(f.Prefix, x+f.PrefixX, y, size)
	}
	return r.pdf.Error()
}

func (r *pdfRenderer) measureTable(b Block, width, height float64) (*pdfTableLayout, error) {
	return r.measureTableStyle(b, width, height, pdfFlowStyle{size: 10, leading: 1.4})
}
func (r *pdfRenderer) measureTableStyle(b Block, width, height float64, style pdfFlowStyle) (*pdfTableLayout, error) {
	if style.running {
		// The whole story repeats; continuation-prefix admission is inapplicable.
		b.Rows = append([]Row(nil), b.Rows...)
		for i := range b.Rows {
			b.Rows[i].RepeatHeader = false
		}
	}
	return r.measureTableHeaderBudget(b, width, height, height, math.Inf(1), style)
}

func (r *pdfRenderer) measureTableHeaderBudget(b Block, width, height, headerBudget, previousHeaderHeight float64, style pdfFlowStyle) (*pdfTableLayout, error) {
	g, err := tableGrid(b.Rows)
	if err != nil {
		return nil, err
	}
	plan := &pdfTableLayout{width: width, anchors: b.Anchors}
	if g.Columns == 0 {
		return plan, nil
	}
	if !finite(width) || width <= 0 {
		return nil, fmt.Errorf("%w: no width for PDF table", ErrUnsupported)
	}
	headers, _ := tableHeaderRows(b.Rows)
	plan.headerRows = headers
	widths, err := fittedColumnWidths(b.ColumnWidths, g.Columns, width)
	if err != nil {
		return nil, err
	}
	plan.columns = make([]float64, g.Columns+1)
	for i, w := range widths {
		plan.columns[i+1] = plan.columns[i] + w
	}
	plan.width = plan.columns[g.Columns]
	heights := make([]float64, len(b.Rows))
	for i := range heights {
		heights[i] = 22
	}
	headerHeight := 0.0
	for _, p := range g.Origins {
		if p.Row >= headers && headers > 0 && headerHeight == 0 {
			for _, c := range plan.cells {
				pos := c.position
				total := 0.0
				for _, h := range heights[pos.Row : pos.Row+pos.RowSpan] {
					total += h
				}
				if total < c.height {
					heights[pos.Row+pos.RowSpan-1] += c.height - total
				}
			}
			for _, h := range heights[:headers] {
				headerHeight += h
			}
		}
		if err := r.ctx.Err(); err != nil {
			return nil, err
		}
		cellHeight := height - headerHeight - 12
		if p.Row < headers {
			cellHeight = math.Min(height, headerBudget) - 12
			if headers < len(b.Rows) {
				cellHeight -= 26
			} // one compact body line plus padding
		}
		flow, err := r.cellFlowStyle(b.Rows[p.Row].Cells[p.Cell].Blocks, (plan.columns[p.Column+p.ColSpan]-plan.columns[p.Column])-12, 0, cellHeight, style)
		if err != nil {
			return nil, err
		}
		if p.Row < headers && flowHasBreak(flow) {
			return nil, fmt.Errorf("%w: page controls inside repeating table headers", ErrUnsupported)
		}
		if p.Row < headers {
			plan.headerLast++
		}
		h := math.Max(22, flowHeight(flow)+12)
		if !finite(h) {
			return nil, fmt.Errorf("invalid measured PDF cell height")
		}
		plan.cells = append(plan.cells, pdfMeasuredCell{p, flow, h})
		if p.RowSpan == 1 {
			heights[p.Row] = math.Max(heights[p.Row], h)
		}
	}
	for _, c := range plan.cells {
		p := c.position
		if p.RowSpan == 1 {
			continue
		}
		total := 0.0
		for _, h := range heights[p.Row : p.Row+p.RowSpan] {
			total += h
		}
		if total < c.height {
			heights[p.Row+p.RowSpan-1] += c.height - total
		}
	}
	plan.offsets = make([]float64, len(heights)+1)
	for i, h := range heights {
		plan.offsets[i+1] = plan.offsets[i] + h
		if !finite(plan.offsets[i+1]) {
			return nil, fmt.Errorf("%w: PDF table height exceeds the coordinate range", ErrUnsupported)
		}
	}
	plan.height = plan.offsets[len(heights)]
	cursor := 0
	for start := 0; start < len(heights); {
		end, last := start+1, cursor
		for last < len(plan.cells) && plan.cells[last].position.Row < end {
			p := plan.cells[last].position
			end = max(end, p.Row+p.RowSpan)
			last++
		}
		g := pdfTableGroup{start: start, end: end, first: cursor, last: last, height: plan.offsets[end] - plan.offsets[start]}
		for _, cell := range plan.cells[cursor:last] {
			g.keys = append(g.keys, flowNoteKeys(cell.flow)...)
			g.breaks = g.breaks || flowHasBreak(cell.flow)
		}
		plan.groups = append(plan.groups, g)
		cursor, start = last, end
	}

	if headers > 0 {
		if headers == len(b.Rows) && plan.offsets[headers] > height+1e-7 {
			excess := plan.offsets[headers] - height
			if plan.offsets[headers] >= previousHeaderHeight-1e-7 || headerBudget-excess <= 0 {
				return nil, fmt.Errorf("%w: repeating table header exceeds full container height", ErrUnsupported)
			}
			return r.measureTableHeaderBudget(b, width, height, headerBudget-excess, plan.offsets[headers], style)
		}
		if headers < len(b.Rows) {
			hg := pdfTableGroup{start: 0, end: headers, first: 0, last: plan.headerLast}
			header := pdfTableHeader{group: hg, height: plan.offsets[headers]}
			for _, c := range plan.cells[:plan.headerLast] {
				header.keys = append(header.keys, flowNoteKeys(c.flow)...)
			}
			plan.header = &header
			combined := pdfTableGroup{start: 0, end: len(b.Rows), first: 0, last: len(plan.cells), height: plan.height}
			for _, group := range plan.groups {
				combined.keys = append(combined.keys, group.keys...)
				combined.breaks = combined.breaks || group.breaks
			}
			plan.groups = []pdfTableGroup{combined}
			minimum := 0.0
			for _, cell := range plan.cells[plan.headerLast:] {
				for _, f := range cell.flow {
					minimum = math.Max(minimum, minimumPDFFragmentHeight(f)+12)
				}
			}
			if excess := header.height + minimum - height; excess > 1e-7 {
				// A deeper body start needs more than the default single-line reservation.
				// Reduce only the header's image budget, then remeasure the whole table;
				// body figures keep the full remaining container. Text cannot be shrunk.

				// The minimum start is a legal first band, not the height of a whole connected
				// group. Padding and final-table spacing belong to their respective containers.

				// Selection is pure with respect to the measured input and page state. The
				// returned tail owns its continuation cursors; retries at a smaller capacity
				// start from the same original flows, including the same nested note references.

				// a list marker belongs only to its first placement

				// Original row offsets do not include padding added by earlier
				// continuations. Measure the remaining flow without a page limit;
				// its true height prevents a parent from allocating too little space
				// and drawing two closed pieces of this table on the same page.

				// A cursor owns only unconsumed cell flows for the current logical row. Planning
				// copies these slices, never the caller's blocks or the measured table.

				// Plan one portion of one logical row. Row-spanning cells carry their own
				// unconsumed flow forward. Content ending in this row can extend its measured
				// height to account for padding added at page continuations.

				// Retry all cells from the same immutable cursor at the first break.
				// No sibling may paint text beyond that boundary on this page.

				// Only ending cells force this logical row to grow; spanning content
				// can continue in the next row, where its ownership is unchanged.

				// A repeating prefix is an immutable physical copy. Keep it with a body band;
				// it never consumes authored body flow or creates new note identities.

				// Close page fragments, while interior bands retain true rowspan borders.

				// Reject indivisible starts before spending pages on preceding rows. Nested
				// groups can now supply a smaller band through recursive selection.

				if header.height >= previousHeaderHeight-1e-7 || headerBudget-excess <= 0 {
					return nil, fmt.Errorf("%w: repeating header leaves no legal body fragment", ErrUnsupported)
				}
				return r.measureTableHeaderBudget(b, width, height, headerBudget-excess, header.height, style)
			}
		}
	}
	return plan, nil
}
func (r *pdfRenderer) paintTableGroup(plan *pdfTableLayout, g pdfTableGroup, x, top float64) error {
	r.bindDestinations(plan.anchors, top)
	for _, c := range plan.cells[g.first:g.last] {
		if err := r.ctx.Err(); err != nil {
			return err
		}
		p := c.position
		cx := x + plan.columns[p.Column]
		cy := top + plan.offsets[p.Row] - plan.offsets[g.start]
		ch := plan.offsets[p.Row+p.RowSpan] - plan.offsets[p.Row]
		r.pdf.SetDrawColor(120, 120, 120)
		if !plan.borderless {
			r.pdf.Rect(cx, cy, (plan.columns[p.Column+p.ColSpan] - plan.columns[p.Column]), ch, "D")
		}
		y := cy + 6
		for _, f := range c.flow {
			if err := r.drawFragment(f, cx+6, y); err != nil {
				return err
			}
			y += f.Height
		}
	}
	return r.pdf.Error()
}
func (r *pdfRenderer) table(b Block, width, x float64) error {
	plan, err := r.measureTable(b, width, r.bodyHeight())
	if err != nil {
		return err
	}
	return r.renderTablePlan(plan, x)
}

func (r *pdfRenderer) renderTablePlan(plan *pdfTableLayout, x float64) error {
	for _, g := range plan.groups {
		minimum, err := r.minimumNewFootnotes(g.keys, true, g.height)
		if err != nil {
			return err
		}
		if g.breaks || g.height+minimum > r.bodyHeight() {
			if err := r.continueTableGroup(plan, g, x); err != nil {
				return fmt.Errorf("PDF table row group %d-%d: %w", g.start+1, g.end, err)
			}
			continue
		}
		if err := r.ensureWithNotes(g.height, g.keys); err != nil {
			return fmt.Errorf("PDF table row group %d-%d: %w", g.start+1, g.end, err)
		}
		if err := r.paintTableGroup(plan, g, x, r.y); err != nil {
			return err
		}
		r.y += g.height
	}
	return r.pdf.Error()
}
func tableFragmentCursor(f pdfFragment) pdfTableCursor {
	if f.Table.cursor != nil {
		return *f.Table.cursor
	}
	return enterPDFTableRow(f.Table.table, f.Table.group, pdfTableCursor{row: f.Table.group.start, next: f.Table.group.first})
}

func minimumPDFFragmentHeight(f pdfFragment) float64 {
	if f.Table == nil || f.Table.page != nil {
		return f.Height
	}
	cursor := tableFragmentCursor(f)
	header := 0.0
	if plan := f.Table.table; plan.header != nil {
		header = plan.header.height
		if cursor.row < plan.headerRows {
			cursor = enterPDFTableRow(plan, f.Table.group, pdfTableCursor{row: plan.headerRows, next: plan.headerLast})
		}
	}
	minimum := 22.0
	for _, c := range cursor.cells {
		p := f.Table.table.cells[c.cell].position
		if p.Row+p.RowSpan == cursor.row+1 && len(c.flow) > 0 {
			minimum = math.Max(minimum, minimumPDFFragmentHeight(c.flow[0])+12)
		}
	}
	return header + minimum + f.Table.gap
}

func (r *pdfRenderer) takeFragments(flow []pdfFragment, capacity float64, atStart bool) (head, tail []pdfFragment, used float64, forced bool, err error) {
	for i, f := range flow {
		if err = r.ctx.Err(); err != nil {
			return nil, nil, 0, false, err
		}
		if f.Break {
			if f.Conditional && atStart && used <= 1e-7 {
				continue
			}
			return head, flow[i+1:], used, true, nil
		}
		if f.Height <= capacity-used+1e-7 && (f.Table == nil || f.Table.cursor == nil && !f.Table.group.breaks || f.Table.page != nil) {
			head = append(head, f)
			used += f.Height
			continue
		}
		if f.Table == nil || f.Table.page != nil {
			return head, flow[i:], used, false, nil
		}
		cursor := tableFragmentCursor(f)
		budget := capacity - used - f.Table.gap
		if f.Table.group.breaks {
			budget = capacity - used
		}
		page, next, e := r.planTablePage(f.Table.table, f.Table.group, cursor, budget, atStart && used <= 1e-7)
		if e == nil && !page.forced && next.row == f.Table.group.end && page.height+f.Table.gap > capacity-used+1e-7 {
			page, next, e = r.planTablePage(f.Table.table, f.Table.group, cursor, capacity-used-f.Table.gap, atStart && used <= 1e-7)
		}
		if e != nil {
			return nil, nil, 0, false, e
		}
		if len(page.bands) == 0 {
			return head, flow[i:], used, false, nil
		}
		part := f
		part.Table = &pdfTableSlice{table: f.Table.table, group: f.Table.group, page: &page}
		part.Height = page.height
		if next.row == f.Table.group.end && !page.forced {
			part.Height += f.Table.gap
		}
		head = append(head, part)
		used += part.Height
		if next.row < f.Table.group.end {
			rest := f
			rest.Prefix = nil
			rest.Table = &pdfTableSlice{table: f.Table.table, group: f.Table.group, cursor: &next, gap: f.Table.gap}

			rest.Height, e = r.measureTableRemainder(f.Table.table, f.Table.group, next)
			if e != nil {
				return nil, nil, 0, false, e
			}
			rest.Height += f.Table.gap
			if !finite(rest.Height) {
				return nil, nil, 0, false, fmt.Errorf("%w: continued PDF table exceeds the coordinate range", ErrUnsupported)
			}
			tail = append([]pdfFragment{rest}, flow[i+1:]...)
			return head, tail, used, page.forced, nil
		}
		if page.forced {
			tail = flow[i+1:]
			if f.Table.gap > 0 {
				tail = append([]pdfFragment{{Height: f.Table.gap}}, tail...)
			}
			return head, tail, used, true, nil
		}
	}
	return head, nil, used, false, nil
}

func (r *pdfRenderer) measureTableRemainder(plan *pdfTableLayout, g pdfTableGroup, cursor pdfTableCursor) (float64, error) {
	height := 0.0
	for cursor.row < g.end {
		page, next, err := r.planTablePage(plan, g, cursor, math.Inf(1), true)
		if err != nil {
			return 0, err
		}
		height += page.height
		cursor = next
	}
	return height, nil
}

type pdfTableCursor struct {
	row, next int
	remaining float64
	first     bool
	cells     []pdfCellCursor
}
type pdfCellCursor struct {
	cell int
	flow []pdfFragment
}
type pdfTableBand struct {
	forced bool
	height float64
	cells  []pdfTableBandCell
}
type pdfTableBandCell struct {
	position    *tableCellPosition
	flow        []pdfFragment
	top, bottom bool
}
type pdfTablePage struct {
	header *pdfTableHeader
	forced bool
	bands  []pdfTableBand
	height float64
	keys   []noteKey
}

func enterPDFTableRow(plan *pdfTableLayout, g pdfTableGroup, cursor pdfTableCursor) pdfTableCursor {
	cursor.remaining = plan.offsets[cursor.row+1] - plan.offsets[cursor.row]
	cursor.first = true
	active := make([]pdfCellCursor, 0, len(cursor.cells)+1)
	for _, c := range cursor.cells {
		p := plan.cells[c.cell].position
		if p.Row+p.RowSpan > cursor.row {
			active = append(active, c)
		}
	}
	for cursor.next < g.last && plan.cells[cursor.next].position.Row == cursor.row {
		active = append(active, pdfCellCursor{cursor.next, plan.cells[cursor.next].flow})
		cursor.next++
	}
	cursor.cells = active
	return cursor
}

func (r *pdfRenderer) nextPDFTableBand(plan *pdfTableLayout, g pdfTableGroup, cursor pdfTableCursor, capacity float64, atStart bool) (pdfTableBand, pdfTableCursor, bool, error) {
	minimum := 22.0
	desired := cursor.remaining
	for _, c := range cursor.cells {
		p := plan.cells[c.cell].position
		if p.Row+p.RowSpan == cursor.row+1 && len(c.flow) > 0 {
			minimum = math.Max(minimum, minimumPDFFragmentHeight(c.flow[0])+12)
			desired = math.Max(desired, flowHeight(c.flow)+12)
		}
	}
	if capacity+1e-7 < 12 {
		return pdfTableBand{}, cursor, false, nil
	}
	height := math.Min(capacity, math.Max(desired, minimum))
	selectCells := func(height float64) (pdfTableBand, []pdfCellCursor, float64, error) {
		band := pdfTableBand{height: height}
		cells := make([]pdfCellCursor, len(cursor.cells))
		cut := height
		for i, c := range cursor.cells {
			p := plan.cells[c.cell].position
			head, tail, used, forced, err := r.takeFragments(c.flow, height-12, atStart)
			if err != nil {
				return band, nil, 0, err
			}
			cells[i] = pdfCellCursor{c.cell, tail}
			band.cells = append(band.cells, pdfTableBandCell{position: p, flow: head, top: cursor.first && p.Row == cursor.row})
			if forced {
				band.forced = true
				cut = math.Min(cut, used+12)
			}
		}
		return band, cells, cut, nil
	}
	band, cells, cut, err := selectCells(height)
	if err != nil {
		return pdfTableBand{}, cursor, false, err
	}
	if !band.forced && capacity+1e-7 < minimum {
		return pdfTableBand{}, cursor, false, nil
	}
	if band.forced {

		band, cells, _, err = selectCells(cut)
		if err != nil {
			return pdfTableBand{}, cursor, false, err
		}
		band.forced = true
		height = cut
	}
	next := cursor
	next.cells = cells
	next.remaining = math.Max(0, cursor.remaining-height)
	done := next.remaining <= 1e-7
	for _, c := range cells {
		p := plan.cells[c.cell].position
		if p.Row+p.RowSpan == cursor.row+1 && len(c.flow) > 0 {
			done = false
		}
	}
	for i := range band.cells {
		p := band.cells[i].position
		band.cells[i].bottom = done && p.Row+p.RowSpan == cursor.row+1
	}
	if done {
		next.row++
		if next.row < g.end {
			next = enterPDFTableRow(plan, g, next)
		}
	} else {
		next.first = false
		if next.remaining <= 1e-7 {

			for _, c := range next.cells {
				p := plan.cells[c.cell].position
				if p.Row+p.RowSpan == cursor.row+1 {
					next.remaining = math.Max(next.remaining, flowHeight(c.flow)+12)
				}
			}
		}
	}
	return band, next, true, nil
}

func (r *pdfRenderer) planTablePageRaw(plan *pdfTableLayout, g pdfTableGroup, cursor pdfTableCursor, capacity float64, atStart bool) (pdfTablePage, pdfTableCursor, error) {
	var page pdfTablePage
	for cursor.row < g.end {
		if err := r.ctx.Err(); err != nil {
			return page, cursor, err
		}
		band, next, ok, err := r.nextPDFTableBand(plan, g, cursor, capacity-page.height, atStart && page.height <= 1e-7)
		if err != nil {
			return page, cursor, err
		}
		if !ok {
			break
		}
		page.bands = append(page.bands, band)
		page.height += band.height
		for _, c := range band.cells {
			page.keys = append(page.keys, flowNoteKeys(c.flow)...)
		}
		cursor = next
		if band.forced {
			page.forced = true
			break
		}
	}
	return page, cursor, nil
}

func (r *pdfRenderer) planTablePage(plan *pdfTableLayout, g pdfTableGroup, cursor pdfTableCursor, capacity float64, atStart bool) (pdfTablePage, pdfTableCursor, error) {
	if plan.header == nil {
		return r.planTablePageRaw(plan, g, cursor, capacity, atStart)
	}
	original := cursor
	if cursor.row < plan.headerRows {
		cursor = enterPDFTableRow(plan, g, pdfTableCursor{row: plan.headerRows, next: plan.headerLast})
	}
	if cursor.row >= g.end || capacity+1e-7 < plan.header.height {
		return pdfTablePage{}, original, nil
	}
	body, next, err := r.planTablePageRaw(plan, g, cursor, capacity-plan.header.height, false)
	if err != nil || len(body.bands) == 0 {
		return pdfTablePage{}, original, err
	}
	page := pdfTablePage{height: plan.header.height + body.height, forced: body.forced, header: plan.header}
	page.bands = append(page.bands, body.bands...)
	page.keys = append(page.keys, plan.header.keys...)
	page.keys = append(page.keys, body.keys...)
	return page, next, nil
}

func (r *pdfRenderer) paintTablePage(plan *pdfTableLayout, page pdfTablePage, x, top float64) error {
	r.bindDestinations(plan.anchors, top)
	y := top
	if page.header != nil {
		before := r.suppressOutputStructure
		r.suppressOutputStructure = before || plan.outputHeaderPainted
		plan.outputHeaderPainted = true
		defer func() { r.suppressOutputStructure = before }()
		if err := r.paintTableGroup(plan, page.header.group, x, y); err != nil {
			return err
		}
		y += page.header.height
		r.suppressOutputStructure = before
	}
	bodyTop := y
	r.pdf.SetDrawColor(120, 120, 120)
	for _, band := range page.bands {
		for _, c := range band.cells {
			if err := r.ctx.Err(); err != nil {
				return err
			}
			p := c.position
			cx, cw := x+plan.columns[p.Column], (plan.columns[p.Column+p.ColSpan] - plan.columns[p.Column])
			r.pdf.SetDrawColor(120, 120, 120)
			if !plan.borderless {
				r.pdf.Line(cx, y, cx, y+band.height)
				r.pdf.Line(cx+cw, y, cx+cw, y+band.height)
				if c.top {
					r.pdf.Line(cx, y, cx+cw, y)
				}
				if c.bottom {
					r.pdf.Line(cx, y+band.height, cx+cw, y+band.height)
				}
			}
			fy := y + 6
			for _, f := range c.flow {
				if err := r.drawFragment(f, cx+6, fy); err != nil {
					return err
				}
				fy += f.Height
			}
		}
		y += band.height
	}
	if plan.borderless {
		return r.pdf.Error()
	}

	r.pdf.SetDrawColor(120, 120, 120)
	for _, edge := range []struct {
		band pdfTableBand
		y    float64
	}{{page.bands[0], bodyTop}, {page.bands[len(page.bands)-1], y}} {
		for _, c := range edge.band.cells {
			p := c.position
			cx := x + plan.columns[p.Column]
			r.pdf.Line(cx, edge.y, cx+(plan.columns[p.Column+p.ColSpan]-plan.columns[p.Column]), edge.y)
		}
	}
	return r.pdf.Error()
}

func (r *pdfRenderer) continueTableGroup(plan *pdfTableLayout, g pdfTableGroup, x float64) error {
	contentHeight := r.bodyHeight()

	for _, c := range plan.cells[g.first:g.last] {
		for _, f := range c.flow {
			if minimumPDFFragmentHeight(f)+12 > contentHeight+1e-7 {
				return fmt.Errorf("%w: PDF cell fragment is taller than a continuation page", ErrUnsupported)
			}
		}
	}
	cursor := enterPDFTableRow(plan, g, pdfTableCursor{row: g.start, next: g.first})
	for cursor.row < g.end {
		available := r.bodyBottom() - r.y - r.noteReserve
		budget := available
		var page pdfTablePage
		var next pdfTableCursor
		for {
			var err error
			page, next, err = r.planTablePage(plan, g, cursor, budget, r.y <= r.bodyTop()+1e-7)
			if err != nil {
				return err
			}
			if len(page.bands) == 0 {
				break
			}
			minimum, err := r.minimumNewFootnotes(page.keys, false, page.height)
			if err != nil {
				return err
			}
			freshMinimum, err := r.minimumNewFootnotes(page.keys, true, page.height)
			if err != nil {
				return err
			}
			if page.height+minimum <= available+1e-7 && page.height+freshMinimum <= contentHeight+1e-7 {
				break
			}
			budget = math.Min(budget-1e-6, math.Min(available-minimum, contentHeight-freshMinimum))
		}
		if len(page.bands) == 0 {
			if r.y <= r.bodyTop()+1e-7 && r.noteReserve == 0 {
				return fmt.Errorf("%w: PDF table continuation cannot fit a cell fragment and its footnote start", ErrUnsupported)
			}
			if err := r.nextPage(true); err != nil {
				return err
			}
			continue
		}
		if err := r.ensureWithNotes(page.height, page.keys); err != nil {
			return err
		}
		if err := r.paintTablePage(plan, page, x, r.y); err != nil {
			return err
		}
		r.y += page.height
		cursor = next
		if cursor.row < g.end || page.forced {
			if err := r.nextPage(true); err != nil {
				return err
			}
		}
	}
	return r.pdf.Error()
}
