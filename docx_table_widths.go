package manydoc

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
)

func (r *docxReader) tableWidths(n *xmlNode, b *Block, columns int) error {
	var widths []float64
	complete := true
	grid := n.child("tblGrid")
	var children []*xmlNode
	if grid != nil {
		children = grid.Children
	}
	for _, col := range children {
		if col.Name.Local != "gridCol" {
			continue
		}
		if len(widths) >= 10000 {
			return fmt.Errorf("DOCX table grid exceeds column limit")
		}
		w, err := docxPageNumber(col, "w", 0)
		if err != nil || w < 0 {
			return fmt.Errorf("invalid DOCX table grid column width")
		}
		if w == 0 {
			complete = false
		}
		widths = append(widths, w)
	}
	if len(widths) != columns {
		complete = false
	}
	if !complete {
		if len(widths) > 0 {
			r.doc.warn("table_width_incomplete", "Incomplete or unspecified DOCX grid widths use equal container columns; content and logical spans retained")
		}
		if n.attributeNS(docxNotesNS, "columnWidths") != "" {
			return fmt.Errorf("invalid DOCX table width metadata without complete grid")
		}
		r.tableWidthOverrides(n, widths)
		return nil
	}
	if err := validateColumnWidths(widths, columns); err != nil {
		return err
	}
	if marker := n.attributeNS(docxNotesNS, "columnWidths"); marker != "" {
		// Retain the preference before container fitting/twip quantization only
		// when its independently visible grid agrees with the serialized result.
		var original []float64
		if err := json.Unmarshal([]byte(marker), &original); err != nil {
			return fmt.Errorf("invalid DOCX table width metadata")
		}
		if columns == 0 {
			if len(original) != 0 {
				return fmt.Errorf("nonempty widths on empty table")
			}
			return nil
		}
		candidate := Block{Rows: b.Rows, ColumnWidths: original}
		available, err := strconv.ParseFloat(n.attributeNS(docxNotesNS, "columnWidthContainer"), 64)
		if err != nil || !finite(available) || available <= 0 || available > 1e7 {
			return fmt.Errorf("invalid DOCX table width container metadata")
		}
		_, expected, err := docxTableColumns(candidate, available)
		if err != nil {
			return err
		}
		for i, w := range widths {
			if math.Abs(w*20-float64(expected[i])) > 1e-6 {
				return fmt.Errorf("DOCX table width metadata disagrees with grid")
			}
		}
		b.ColumnWidths = original
	} else {
		b.ColumnWidths = widths
		// Grid widths are initial preferences, not a complete Word layout.
		if layout := n.child("tblPr").child("tblLayout"); layout == nil || layout.attr("type") != "fixed" {
			r.doc.warn("table_autofit", "DOCX initial grid widths retained; Word automatic content-based table resizing is normalized to fixed preferred columns")
		}
	}
	r.tableWidthOverrides(n, widths)
	return nil
}

// Word can override the initial grid through total/cell preferences and auto-fit.
// Preserve the grid and disclose conflicts rather than implying full layout.
func (r *docxReader) tableWidthOverrides(n *xmlNode, widths []float64) {
	total := 0.0
	for _, w := range widths {
		total += w
	}
	if pref := n.child("tblPr").child("tblW"); pref != nil {
		kind := pref.attr("type")
		w, err := docxPageNumber(pref, "w", 0)
		if kind == "pct" || (kind == "dxa" && (err != nil || math.Abs(w-total) > .025)) {
			r.doc.warn("table_width_preference", "DOCX table total-width override differs from initial grid preferences; grid widths retained without Word layout resolution")
		}
	}
	for _, row := range n.Children {
		if row.Name.Local != "tr" {
			continue
		}
		column := 0
		for _, cell := range row.Children {
			if cell.Name.Local != "tc" {
				continue
			}
			span, _ := docxSpan(cell.child("tcPr").child("gridSpan"))
			span = max(1, span)
			if pref := cell.child("tcPr").child("tcW"); pref != nil {
				kind := pref.attr("type")
				w, err := docxPageNumber(pref, "w", 0)
				sum := 0.0
				for c := column; c < min(column+span, len(widths)); c++ {
					sum += widths[c]
				}
				if kind == "pct" || (kind == "dxa" && (err != nil || math.Abs(w-sum) > .025)) {
					r.doc.warn("table_cell_width_preference", "DOCX cell-width overrides differ from initial grid preferences; grid and spans retained without Word layout resolution")
				}
			}
			column += span
		}
	}
}
