package manydoc

import (
	"fmt"
	"math"
)

func listPrefix(b Block, index int) string {
	if b.List == "number" {
		return fmt.Sprintf("%d.", listStart(b)+index)
	}
	return "•"
}
func (r *pdfRenderer) listMarker(b Block, index int, width, size float64) (pdfLine, float64, error) {
	lines, err := r.lines([]Run{{Text: listPrefix(b, index)}}, width, size)
	if err != nil {
		return nil, 0, err
	}
	if len(lines) != 1 {
		return nil, 0, fmt.Errorf("%w: list marker exceeds content width", ErrUnsupported)
	}
	markerWidth := 0.0
	for _, s := range lines[0] {
		markerWidth += s.width
	}
	indent := math.Max(18, markerWidth+6)
	if width <= indent {
		return nil, 0, fmt.Errorf("%w: list indentation leaves no text width", ErrUnsupported)
	}
	return lines[0], indent, nil
}
func (r *pdfRenderer) list(b Block, width, x float64) error {
	for index, item := range b.Items {
		size := 11.0
		first := item.Blocks[0]
		if err := r.paragraphBreakBefore(first); err != nil {
			return err
		}
		if first.Kind == Heading {
			size = 26 - float64(first.Level)*2
		}
		marker, indent, err := r.listMarker(b, index, width, size)
		if err != nil {
			return err
		}
		minimum := size * 1.4
		if first.Kind == Heading {
			minimum *= 2
		}
		firstLines, err := r.linesDirection(first.Runs, width-indent, size, first.Direction)
		if err != nil {
			return err
		}
		minimum = pdfLineHeight(firstLines[0], size, 1.4)
		if first.Kind == Heading {
			minimum = math.Min(minimum*2, r.height-2*r.margin)
		}
		if err := r.ensureWithNotes(minimum, lineNoteKeys(firstLines[0])); err != nil {
			return err
		}
		markerX, bodyX := x, x+indent
		if first.Direction == "rtl" {
			markerX = x + width - pdfAdvance(marker)
			bodyX = x
		}
		r.drawLine(marker, markerX, r.y+pdfLineSize(firstLines[0], size)-size, size)
		if err := r.blocks(item.Blocks, width-indent, bodyX); err != nil {
			return err
		}
	}
	return r.pdf.Error()
}
