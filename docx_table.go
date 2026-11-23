package manydoc

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

func docxSpan(n *xmlNode) (int, error) {
	if n == nil {
		return 0, nil
	}
	value, err := strconv.Atoi(n.attr("val"))
	if err != nil || value < 1 || value > 1000 {
		return 0, fmt.Errorf("invalid DOCX gridSpan %q", n.attr("val"))
	}
	return value, nil
}

func (r *docxReader) table(n *xmlNode, rels map[string]docxRelation, depth int) (Block, error) {
	b := Block{Kind: Table}
	// Only the immediately preceding row can continue a merge. A missing vMerge
	// closes it; a later continuation cannot reopen a historical origin.
	type origin struct{ row, cell, span int }
	active := map[int]origin{}
	for _, row := range n.Children {
		if row.Name.Local != "tr" {
			continue
		}
		if err := r.ctx.Err(); err != nil {
			return Block{}, err
		}
		for _, name := range []string{"gridBefore", "gridAfter"} {
			if prop := row.child("trPr").child(name); prop != nil && prop.attr("val") != "0" {
				return Block{}, fmt.Errorf("%w: DOCX skipped table grid cells (%s)", ErrUnsupported, name)
			}
		}
		ri := len(b.Rows)
		repeat, err := docxTableHeader(row.child("trPr"))
		if err != nil {
			return Block{}, err
		}
		if repeat && (ri > 0 && !b.Rows[ri-1].RepeatHeader) {
			r.doc.warn("table_header_ignored", "Noncontiguous DOCX table header property is ignored according to OOXML")
			repeat = false
		}
		b.Rows = append(b.Rows, Row{RepeatHeader: repeat})
		next := map[int]origin{}
		column := 0
		for _, cell := range row.Children {
			if cell.Name.Local != "tc" {
				continue
			}
			bs, err := r.blocks(cell, rels, depth+1)
			if err != nil {
				return Block{}, err
			}
			span, err := docxSpan(cell.child("tcPr").child("gridSpan"))
			if err != nil {
				return Block{}, err
			}
			width := max(1, span)
			merge := cell.child("tcPr").child("vMerge")
			if merge != nil && (merge.attr("val") == "" || merge.attr("val") == "continue") {
				o, ok := active[column]
				if !ok || o.span != width {
					return Block{}, fmt.Errorf("DOCX vertical merge continuation has no matching origin at row %d column %d", ri+1, column+1)
				}
				target := &b.Rows[o.row].Cells[o.cell]
				target.RowSpan++
				// Required empty continuation paragraphs carry no visible content. Any
				// real content is retained at the origin, with an explicit placement loss.
				if !emptyMergeContinuation(bs) {
					target.Blocks = append(target.Blocks, bs...)
					r.doc.warn("merge_continuation_content", "Vertical merge continuation content retained at its origin; original placement is normalized")
				}
				next[column] = o
			} else {
				if merge != nil && merge.attr("val") != "restart" {
					return Block{}, fmt.Errorf("invalid DOCX vertical merge value %q", merge.attr("val"))
				}
				c := Cell{Blocks: bs, ColSpan: span}
				if merge != nil {
					c.RowSpan = 1
					next[column] = origin{ri, len(b.Rows[ri].Cells), width}
				}
				b.Rows[ri].Cells = append(b.Rows[ri].Cells, c)
			}
			column += width
		}
		active = next
	}
	headers, _ := tableHeaderRows(b.Rows)
	for ri := 0; ri < headers; ri++ {
		for _, c := range b.Rows[ri].Cells {
			if ri+max(1, c.RowSpan) > headers {
				for i := range b.Rows {
					b.Rows[i].RepeatHeader = false
				}
				r.doc.warn("table_header_merge", "DOCX merge crosses the repeating-header boundary; content and merge retained, header repetition disabled")
				break
			}
		}
	}
	if _, err := tableGrid(b.Rows); err != nil {
		return Block{}, err
	}
	return b, nil
}

func emptyMergeContinuation(bs []Block) bool {
	for _, b := range bs {
		if b.Kind != Paragraph || b.List != "" || b.PageBreakBefore {
			return false
		}
		for _, run := range b.Runs {
			if run.Text != "" || run.Note != nil {
				return false
			}
		}
	}
	return true
}

func (w *docxWriter) table(b Block) (string, error) {
	g, err := tableGrid(b.Rows)
	if err != nil {
		return "", err
	}
	total := int(math.Round(w.contentWidth*20) - float64(b.docxIndent))
	if total < max(1, g.Columns) {
		return "", fmt.Errorf("%w: DOCX table too narrow for %d columns", ErrUnsupported, g.Columns)
	}
	widths := make([]int, g.Columns)
	for col := range widths {
		widths[col] = total / g.Columns
		if col < total%g.Columns {
			widths[col]++
		}
	}
	var out strings.Builder
	fmt.Fprintf(&out, `<w:tbl><w:tblPr><w:tblW w:w="%d" w:type="dxa"/>`, total)
	if b.docxIndent > 0 {
		fmt.Fprintf(&out, `<w:tblInd w:w="%d" w:type="dxa"/>`, b.docxIndent)
	}
	out.WriteString(`<w:tblBorders><w:top w:val="single" w:sz="4"/><w:left w:val="single" w:sz="4"/><w:bottom w:val="single" w:sz="4"/><w:right w:val="single" w:sz="4"/><w:insideH w:val="single" w:sz="4"/><w:insideV w:val="single" w:sz="4"/></w:tblBorders><w:tblLayout w:type="fixed"/><w:tblCellMar><w:left w:w="80" w:type="dxa"/><w:right w:w="80" w:type="dxa"/></w:tblCellMar></w:tblPr><w:tblGrid>`)
	for _, width := range widths {
		fmt.Fprintf(&out, `<w:gridCol w:w="%d"/>`, width)
	}
	out.WriteString(`</w:tblGrid>`)
	for ri, slots := range g.Slots {
		if err := w.ctx.Err(); err != nil {
			return "", err
		}
		out.WriteString(`<w:tr>`)
		if b.Rows[ri].RepeatHeader {
			out.WriteString(`<w:trPr><w:tblHeader/></w:trPr>`)
		}
		for col := 0; col < len(slots); {
			p := slots[col]
			c := b.Rows[p.Row].Cells[p.Cell]
			cellWidth := 0
			for _, v := range widths[col : col+p.ColSpan] {
				cellWidth += v
			}
			fmt.Fprintf(&out, `<w:tc><w:tcPr><w:tcW w:w="%d" w:type="dxa"/>`, cellWidth)
			if p.ColSpan > 1 {
				fmt.Fprintf(&out, `<w:gridSpan w:val="%d"/>`, p.ColSpan)
			}
			if p.RowSpan > 1 {
				if ri == p.Row {
					out.WriteString(`<w:vMerge w:val="restart"/>`)
				} else {
					out.WriteString(`<w:vMerge/>`)
				}
			}
			out.WriteString(`</w:tcPr>`)
			if ri == p.Row {
				previousWidth := w.contentWidth
				previousHeight := w.contentHeight
				w.contentHeight -= docxImageCellHeightAllowance
				w.contentWidth = float64(cellWidth)/20 - 8
				v, err := w.blocks(c.Blocks)
				w.contentWidth = previousWidth
				w.contentHeight = previousHeight
				if err != nil {
					return "", err
				}
				out.WriteString(v)
				if len(c.Blocks) == 0 {
					out.WriteString(`<w:p md:emptyCell="1"/>`)
				} else if c.Blocks[len(c.Blocks)-1].Kind == Table {
					out.WriteString(`<w:p md:tableTail="1"/>`)
				}
			} else {
				out.WriteString(`<w:p/>`)
			}
			out.WriteString(`</w:tc>`)
			col += p.ColSpan
		}
		out.WriteString(`</w:tr>`)
	}
	out.WriteString(`</w:tbl>`)
	return out.String(), nil
}

func docxTableHeader(pr *xmlNode) (bool, error) {
	var found *xmlNode
	if pr != nil {
		for _, c := range pr.Children {
			if c.Name.Local == "tblHeader" {
				if found != nil {
					return false, fmt.Errorf("duplicate DOCX tblHeader property")
				}
				found = c
			}
		}
	}
	return docxOnOff(found)
}
