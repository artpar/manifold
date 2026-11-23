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
func (r *pdfRenderer) takeFragments(flow []pdfFragment, capacity float64) (head, tail []pdfFragment, used float64, err error) {
	for i, f := range flow {
		if err = r.ctx.Err(); err != nil {
			return nil, nil, 0, err
		}
		if f.Height <= capacity-used+1e-7 && (f.Table == nil || f.Table.cursor == nil) {
			head = append(head, f)
			used += f.Height
			continue
		}
		if f.Table == nil || f.Table.page != nil {
			return head, flow[i:], used, nil
		}
		cursor := tableFragmentCursor(f)
		page, next, e := r.planTablePage(f.Table.table, f.Table.group, cursor, capacity-used-f.Table.gap)
		if e != nil {
			return nil, nil, 0, e
		}
		if len(page.bands) == 0 {
			return head, flow[i:], used, nil
		}
		part := f
		part.Table = &pdfTableSlice{table: f.Table.table, group: f.Table.group, page: &page}
		part.Height = page.height
		if next.row == f.Table.group.end {
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
			remainder, _, e := r.planTablePage(f.Table.table, f.Table.group, next, math.Inf(1))
			if e != nil {
				return nil, nil, 0, e
			}
			rest.Height = remainder.height + f.Table.gap
			if !finite(rest.Height) {
				return nil, nil, 0, fmt.Errorf("%w: continued PDF table exceeds the coordinate range", ErrUnsupported)
			}
			tail = append([]pdfFragment{rest}, flow[i+1:]...)
			return head, tail, used, nil
		}
	}
	return head, nil, used, nil
}
