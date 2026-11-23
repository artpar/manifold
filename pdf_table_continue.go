package manydoc

import (
	"fmt"
	"math"
)

// A cursor owns only unconsumed cell flows for the current logical row. Planning
// copies these slices, never the caller's blocks or the measured table.
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

// Plan one portion of one logical row. Row-spanning cells carry their own
// unconsumed flow forward. Content ending in this row can extend its measured
// height to account for padding added at page continuations.
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
		// Retry all cells from the same immutable cursor at the first break.
		// No sibling may paint text beyond that boundary on this page.
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
			// Only ending cells force this logical row to grow; spanning content
			// can continue in the next row, where its ownership is unchanged.
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

// A repeating prefix is an immutable physical copy. Keep it with a body band;
// it never consumes authored body flow or creates new note identities.
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
	y := top
	if page.header != nil {
		if err := r.paintTableGroup(plan, page.header.group, x, y); err != nil {
			return err
		}
		y += page.header.height
	}
	bodyTop := y
	r.pdf.SetDrawColor(120, 120, 120)
	for _, band := range page.bands {
		for _, c := range band.cells {
			if err := r.ctx.Err(); err != nil {
				return err
			}
			p := c.position
			cx, cw := x+float64(p.Column)*plan.unit, float64(p.ColSpan)*plan.unit
			r.pdf.SetDrawColor(120, 120, 120)
			r.pdf.Line(cx, y, cx, y+band.height)
			r.pdf.Line(cx+cw, y, cx+cw, y+band.height)
			if c.top {
				r.pdf.Line(cx, y, cx+cw, y)
			}
			if c.bottom {
				r.pdf.Line(cx, y+band.height, cx+cw, y+band.height)
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
	// Close page fragments, while interior bands retain true rowspan borders.
	r.pdf.SetDrawColor(120, 120, 120)
	for _, edge := range []struct {
		band pdfTableBand
		y    float64
	}{{page.bands[0], bodyTop}, {page.bands[len(page.bands)-1], y}} {
		for _, c := range edge.band.cells {
			p := c.position
			cx := x + float64(p.Column)*plan.unit
			r.pdf.Line(cx, edge.y, cx+float64(p.ColSpan)*plan.unit, edge.y)
		}
	}
	return r.pdf.Error()
}

func (r *pdfRenderer) continueTableGroup(plan *pdfTableLayout, g pdfTableGroup, x float64) error {
	contentHeight := r.bodyHeight()
	// Reject indivisible starts before spending pages on preceding rows. Nested
	// groups can now supply a smaller band through recursive selection.
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
