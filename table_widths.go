package manydoc

import "fmt"

func validateColumnWidths(widths []float64, columns int) error {
	if len(widths) == 0 {
		return nil
	}
	if len(widths) != columns {
		return fmt.Errorf("table column widths must match %d logical columns", columns)
	}
	total := 0.0
	for _, w := range widths {
		if !finite(w) || w <= 0 {
			return fmt.Errorf("table column widths must be positive finite points")
		}
		total += w
	}
	if !finite(total) || total > 1e7 {
		return fmt.Errorf("table column width total exceeds coordinate range")
	}
	return nil
}

// Shared by measuring, painting, nested DOCX containers and story admission.
func fittedColumnWidths(preferred []float64, columns int, available float64) ([]float64, error) {
	if err := validateColumnWidths(preferred, columns); err != nil {
		return nil, err
	}
	if !finite(available) || available <= 0 {
		return nil, fmt.Errorf("invalid table container width")
	}
	widths := make([]float64, columns)
	if columns == 0 {
		return widths, nil
	}
	if len(preferred) == 0 {
		for i := range widths {
			widths[i] = available / float64(columns)
		}
		return widths, nil
	}
	total := 0.0
	for _, w := range preferred {
		total += w
	}
	scale := min(1.0, available/total)
	for i, w := range preferred {
		widths[i] = w * scale
	}
	return widths, nil
}
