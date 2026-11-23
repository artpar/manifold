package manydoc

import (
	"fmt"
	"math"
)

func tableFragmentCursor(f pdfFragment) pdfTableCursor {
	if f.Table.cursor != nil {
		return *f.Table.cursor
	}
	return enterPDFTableRow(f.Table.table, f.Table.group, pdfTableCursor{row: f.Table.group.start, next: f.Table.group.first})
}

// The minimum start is a legal first band, not the height of a whole connected
// group. Padding and final-table spacing belong to their respective containers.
func minimumPDFFragmentHeight(f pdfFragment) float64 {
	if f.Table == nil || f.Table.page != nil {
		return f.Height
	}
	cursor := tableFragmentCursor(f)
	minimum := 22.0
	for _, c := range cursor.cells {
		p := f.Table.table.cells[c.cell].position
		if p.Row+p.RowSpan == cursor.row+1 && len(c.flow) > 0 {
			minimum = math.Max(minimum, minimumPDFFragmentHeight(c.flow[0])+12)
		}
	}
	return minimum + f.Table.gap
}

// Selection is pure with respect to the measured input and page state. The
// returned tail owns its continuation cursors; retries at a smaller capacity
// start from the same original flows, including the same nested note references.
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
			rest.Prefix = nil // a list marker belongs only to its first placement
			rest.Table = &pdfTableSlice{table: f.Table.table, group: f.Table.group, cursor: &next, gap: f.Table.gap}
			// Original row offsets do not include padding added by earlier
			// continuations. Measure the remaining flow without a page limit;
			// its true height prevents a parent from allocating too little space
			// and drawing two closed pieces of this table on the same page.
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
