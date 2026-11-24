package manydoc

import (
	"fmt"
	"math"
)

// A measured label retains ordinary inner text runs and one outer note atom.
// Wrapping, bidi anchoring, footnote admission and the link see the same object.
type pdfCitationBox struct {
	line       pdfLine
	size, rise float64
}

func (r *pdfRenderer) citationAtom(run Run, size float64, bidi pdfBidiRange, breaking int) (pdfTextAtom, error) {
	if r.notes == nil {
		return pdfTextAtom{}, fmt.Errorf("missing note catalog")
	}
	if _, ok := r.notes.labels[referenceKey(run.Note)]; !ok {
		return pdfTextAtom{}, fmt.Errorf("missing note reference label")
	}
	children := run.Note.LabelContent
	if children == nil {
		literal := run
		literal.Note = nil
		literal.Text = noteReferenceLabel(r.notes, run.Note)
		children = []Run{literal}
	}
	children = append([]Run(nil), children...)
	full := 0.0
	for i := range children {
		point := textSize(children[i], size)
		if point > 14400 || fmt.Sprintf("%.2f", point) == "0.00" || math.Abs(point-math.Round(point*100)/100) > 1e-9 {
			return pdfTextAtom{}, fmt.Errorf("%w: PDF citation font size exceeds coordinate range or 0.01pt precision", ErrUnsupported)
		}
		full = math.Max(full, point)
		children[i].pdfOutput = run.pdfOutput
		children[i].FontSize = math.Round(point*.7*100) / 100
	}
	fallback := math.Round(size*.7*100) / 100
	lines, err := r.linesDirection(children, 14400, fallback, "")
	if err != nil {
		return pdfTextAtom{}, err
	}
	if len(lines) != 1 {
		return pdfTextAtom{}, fmt.Errorf("%w: citation exceeds one atomic label box", ErrUnsupported)
	}
	line := lines[0]
	width := pdfAdvance(line)
	placePDFLine(line, width)
	rise := full * .3
	baseline := pdfLineSize(line, fallback)
	outer := Run{pdfOutput: run.pdfOutput, Note: run.Note, Source: run.Source, FontSize: full}
	span := pdfSpan{bidi: bidi, run: outer, text: noteReferenceLabel(r.notes, run.Note), width: width, ascent: math.Max(full, baseline+rise), descent: pdfLineHeight(line, fallback, 1.4) - baseline - rise, citation: &pdfCitationBox{line: line, size: fallback, rise: rise}}
	return pdfTextAtom{spans: pdfLine{span}, width: width, breaking: breaking}, nil
}
