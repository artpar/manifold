package manifold

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Half-point output uses nearest rounding, with ties upward. Values outside the
// representable range reject; acknowledgement never enables clamping/overflow.
func docxFontHalfPoints(size float64) (float64, error) {
	h := math.Round(size * 2)
	if !finite(size) || size <= 0 || h < 1 || h > math.MaxUint32 || math.Abs(h/2-size) > .25 {
		return 0, fmt.Errorf("%w: DOCX font size is outside the half-point range", ErrUnsupported)
	}
	return h, nil
}
func docxFontSizeIntent(run Run) string {
	if run.FontSize == 0 || run.FontSize*2 == math.Trunc(run.FontSize*2) {
		return ""
	}
	return ` md:fontSize="` + strconv.FormatFloat(run.FontSize, 'g', -1, 64) + `"`
}

// Metadata restores original authoring precision only when both explicit standard
// sizes agree with its deterministic projection. No text/property is suppressed.
func (r *docxReader) restoreFontSizeIntent(style *docxRunStyle, n *xmlNode) error {
	value := n.attributeNS(docxNotesNS, "fontSize")
	if value == "" {
		if n != nil {
			for _, attr := range n.Attr {
				if attr.Name.Space == docxNotesNS && attr.Name.Local == "fontSize" {
					return fmt.Errorf("empty DOCX original font size")
				}
			}
		}
		return nil
	}
	size, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return fmt.Errorf("invalid DOCX original font size")
	}
	h, err := docxFontHalfPoints(size)
	if err != nil || size*2 == h {
		return fmt.Errorf("invalid DOCX font size precision metadata")
	}
	expected := strconv.FormatFloat(h, 'f', 0, 64)
	pr := n.child("rPr")
	validSize := func(p *xmlNode) bool {
		return p != nil && p.Name.Space == n.Name.Space && (p.Name.Space == wordNS || p.Name.Space == strictWordNS) && p.attributeNS(p.Name.Space, "val") == expected && len(p.Children) == 0 && strings.TrimSpace(p.Text) == ""
	}
	if pr == nil || pr.Name.Space != n.Name.Space || !validSize(pr.child("sz")) || !validSize(pr.child("szCs")) || style.FontSize != h/2 || style.sizeCS != h/2 {
		return fmt.Errorf("DOCX font size precision metadata contradicts standard sizes")
	}
	style.FontSize = size
	style.sizeCS = size
	r.doc.warn("docx_font_size_precision", "Portable DOCX uses nearest half-point sizes; checked original authoring precision is restored")
	return nil
}
