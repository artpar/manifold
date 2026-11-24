package manydoc

import (
	"fmt"
	"math"
)

func (r *pdfRenderer) listMarker(b Block, index int, width, size float64) (pdfLine, float64, error) {
	text := "•"
	if b.List == "number" {
		var err error
		text, err = numberedLabel(b, index)
		if err != nil {
			return nil, 0, err
		}
	}
	if b.pdfListMarker != nil {
		text = *b.pdfListMarker
	}
	if text == "" {
		return nil, 0, nil
	}
	run := Run{Text: text}
	if b.pdfListMarkerStyle != nil {
		run = *b.pdfListMarkerStyle
		run.Text = text
	}
	runs := []Run{run}
	if b.pdfListMarkerContent != nil {
		runs = b.pdfListMarkerContent
	}
	lines, err := r.lines(runs, width, size)
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
	placePDFLine(lines[0], markerWidth)
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
			minimum = math.Min(minimum*2, r.bodyHeight())
		}
		markerHeight := pdfLineHeight(marker, size, 1.4)
		minimum = math.Max(minimum, markerHeight)
		if err := r.ensureWithNotes(minimum, lineNoteKeys(firstLines[0])); err != nil {
			return err
		}
		if index == 0 {
			r.bindDestinations(b.Anchors, r.y)
		}
		markerX, bodyX := x, x+indent
		if first.Direction == "rtl" {
			markerX = x + width - pdfAdvance(marker)
			bodyX = x
		}
		markerTop, markerPage := r.y, r.pdf.PageNo()
		markerY := markerTop + pdfLineSize(firstLines[0], size) - size
		if b.pdfListMarkerStyle != nil || b.pdfListMarkerContent != nil {
			markerY = markerTop
		}
		r.drawLine(marker, markerX, markerY, size)
		if err := r.blocks(item.Blocks, width-indent, bodyX); err != nil {
			return err
		}
		if r.pdf.PageNo() == markerPage {
			r.y = math.Max(r.y, markerTop+markerHeight)
		}
	}
	return r.pdf.Error()
}
