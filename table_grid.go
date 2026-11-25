package manifold

import "fmt"

// A cell occurs only at its top-left origin in Rows. Slots covered by a row
// span are omitted from subsequent Rows' Cells, as in ProseMirror tables.
// Keeping geometry in one implementation prevents import/export/layout drift.
type tableCellPosition struct {
	Row, Cell, Column, ColSpan, RowSpan int
}
type tableGeometry struct {
	Columns int
	Slots   [][]*tableCellPosition
	Origins []*tableCellPosition
}

func tableGrid(rows []Row) (*tableGeometry, error) {
	g := &tableGeometry{Slots: make([][]*tableCellPosition, len(rows))}
	headers, err := tableHeaderRows(rows)
	if err != nil {
		return nil, err
	}
	const maxSlots = 1_000_000
	allocated := 0
	for ri, row := range rows {
		column := 0
		for ci, c := range row.Cells {
			if c.ColSpan < 0 || c.ColSpan > 1000 || c.RowSpan < 0 || c.RowSpan > len(rows)-ri {
				return nil, fmt.Errorf("invalid table span at row %d cell %d", ri+1, ci+1)
			}
			cs, rs := max(1, c.ColSpan), max(1, c.RowSpan)
			if ri < headers && ri+rs > headers {
				return nil, fmt.Errorf("table cell crosses repeating header boundary at row %d", ri+1)
			}
			for column < len(g.Slots[ri]) && g.Slots[ri][column] != nil {
				column++
			}
			end := column + cs
			if end > 10000 {
				return nil, fmt.Errorf("table exceeds 10000 columns")
			}
			p := &tableCellPosition{Row: ri, Cell: ci, Column: column, ColSpan: cs, RowSpan: rs}
			for rr := ri; rr < ri+rs; rr++ {
				if end > len(g.Slots[rr]) {
					grow := end - len(g.Slots[rr])
					allocated += grow
					if allocated > maxSlots {
						return nil, fmt.Errorf("table grid exceeds %d slots", maxSlots)
					}
					g.Slots[rr] = append(g.Slots[rr], make([]*tableCellPosition, grow)...)
				}
				for col := column; col < end; col++ {
					if g.Slots[rr][col] != nil {
						return nil, fmt.Errorf("overlapping table span at row %d column %d", rr+1, col+1)
					}
					g.Slots[rr][col] = p
				}
			}
			g.Origins = append(g.Origins, p)
			g.Columns = max(g.Columns, end)
			column = end
		}
	}
	for ri, slots := range g.Slots {
		for col, slot := range slots {
			if slot == nil {
				return nil, fmt.Errorf("table has uncovered grid slot at row %d column %d", ri+1, col+1)
			}
		}
	}
	return g, nil
}

func tableHeaderRows(rows []Row) (int, error) {
	count := 0
	for i, row := range rows {
		if row.RepeatHeader {
			if i != count {
				return 0, fmt.Errorf("repeating table headers must form a contiguous prefix")
			}
			count++
		}
	}
	return count, nil
}
