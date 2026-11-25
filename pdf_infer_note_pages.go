package manifold

import (
	"math"
	"strings"
	"unicode"
	"unicode/utf8"
)

type pdfNoteExtentPlan struct {
	blocks []int
	joins  map[int]bool
	pages  int
}

// Joining is a separate inference from note ownership. Explicit paragraphs,
// authorial starts, incompatible styles and sentence/indent boundaries survive.
func (r *pdfReader) pdfNoteParagraphContinues(previous, next *Block, a, b pdfNoteExtentFacts) bool {
	if previous.Kind != Paragraph || next.Kind != Paragraph || next.PageBreakBefore || previous.Direction != next.Direction ||
		previous.Source != nil && previous.Source.Structure == "explicit" || next.Source != nil && next.Source.Structure == "explicit" {
		return false
	}
	last, first := -1, -1
	for i := len(previous.Runs) - 1; i >= 0; i-- {
		if !r.noteInferenceStep() {
			return false
		}
		if strings.TrimSpace(previous.Runs[i].Text) != "" {
			last = i
			break
		}
	}
	for i := range next.Runs {
		if !r.noteInferenceStep() {
			return false
		}
		if strings.TrimSpace(next.Runs[i].Text) != "" {
			first = i
			break
		}
	}
	if last < 0 || first < 0 {
		return false
	}
	x, y := previous.Runs[last], next.Runs[first]
	if _, mixed := commonTextStyle(x, y); mixed || x.Direction != y.Direction || x.Language != y.Language || x.EastAsiaLanguage != y.EastAsiaLanguage || x.ComplexLanguage != y.ComplexLanguage || x.ComplexScript != y.ComplexScript {
		return false
	}
	if x.Source == nil || y.Source == nil || x.Source.Baseline == nil || y.Source.Baseline == nil || math.Abs(a.x0-b.x0) > max(1, a.em*.8) || x.Source.Baseline[1].X < a.x1-3*a.em {
		return false
	}
	left, _ := utf8.DecodeLastRuneInString(strings.TrimSpace(x.Text))
	right, _ := utf8.DecodeRuneInString(strings.TrimSpace(y.Text))
	return !strings.ContainsRune(".!?。！？", left) && (unicode.IsLower(right) || unicode.IsDigit(right))
}

func pdfNoteCoherentBlock(block *Block, f pdfNoteExtentFacts, page int, left, em, width float64) bool {
	if !f.ok || f.page != page || f.x0 < left-1.5*em || f.x1 > width+1 || f.y0 < 0 {
		return false
	}
	if f.em > 0 && (f.em < em*.85 || f.em > em*1.8) {
		return false
	}
	return (block.Kind != Paragraph && block.Kind != Heading && block.Kind != ListBlock) || f.x0 <= left+3*em
}

func (r *pdfReader) pdfNoteBoundaryStyle(previous, next *Block) bool {
	if previous.Kind != Paragraph || next.Kind != Paragraph {
		return false
	}
	var a, b *Run
	for i := len(previous.Runs) - 1; i >= 0; i-- {
		if !r.noteInferenceStep() {
			return false
		}
		if strings.TrimSpace(previous.Runs[i].Text) != "" {
			a = &previous.Runs[i]
			break
		}
	}
	for i := range next.Runs {
		if !r.noteInferenceStep() {
			return false
		}
		if strings.TrimSpace(next.Runs[i].Text) != "" {
			b = &next.Runs[i]
			break
		}
	}
	if a == nil || b == nil {
		return false
	}
	_, mixed := commonTextStyle(*a, *b)
	return !mixed && a.Direction == b.Direction && a.Language == b.Language && a.EastAsiaLanguage == b.EastAsiaLanguage && a.ComplexLanguage == b.ComplexLanguage && a.ComplexScript == b.ComplexScript
}

// Seek the next physical note region while leaving ordinary content above it in
// flow. Matching separator geometry supports rich blocks; matching
// text-wrap evidence additionally permits a changed separator or a leading endnote page.
func (r *pdfReader) pdfNoteNextPage(start, last int, current pdfNoteExtentFacts, left, em, signature float64, kind string, roots map[*Block]int, protected map[*Block]bool, fact func(int) pdfNoteExtentFacts) (int, bool, bool) {
	page := current.page + 1
	if current.page < 1 || page > len(r.doc.Pages) || current.y1 < r.doc.Pages[current.page-1].Height*.8 || current.x1-current.x0 < 8*em {
		return -1, false, true
	}
	for i := start; i < len(r.doc.Blocks); i++ {
		if !r.noteInferenceStep() {
			return -1, false, false
		}
		b := &r.doc.Blocks[i]
		if b.Kind == PageBreak {
			break
		}
		f := fact(i)
		if !f.ok {
			if b.Kind == AnchorPoint {
				continue
			}
			break
		}
		if f.page != page {
			break
		}
		if _, root := roots[b]; root && math.Abs(f.x0-left) <= 3*em {
			break
		}
		if !pdfNoteCoherentBlock(b, f, page, left, em, r.doc.Pages[page-1].Width) {
			continue
		}
		join := r.pdfNoteParagraphContinues(&r.doc.Blocks[last], b, current, f)
		width := 0.0
		if kind == Footnote {
			width = r.pdfNoteSeparatorWidth(page, left, f.y0, em)
		}
		matching := signature > 0 && width > 0 && math.Abs(width-signature) <= max(2*em, signature*.2)
		// Continuation separators may expand to the column width. Matching text
		// style supplies another cue without forcing two paragraphs to merge.
		expanded := signature > 0 && width >= signature && r.pdfNoteBoundaryStyle(&r.doc.Blocks[last], b)
		eligible := kind == Footnote && width > 0 && (matching || join || expanded) || kind == Endnote && i == start && f.y0 < r.doc.Pages[page-1].Height*.35 && join
		if eligible {
			if protected[b] {
				break
			}
			// A new small leading mark can indicate an unresolved distinct note.
			for j, run := range b.Runs {
				if !r.noteInferenceStep() {
					return -1, false, false
				}
				if strings.TrimSpace(run.Text) == "" {
					continue
				}
				if pdfCompactNoteLabel(run.Text) && pdfRaisedNoteMark(b.Runs, j) {
					return -1, false, true
				}
				break
			}
			return i, join, true
		}
		if kind == Endnote {
			break
		}
	}
	return -1, false, r.ctx.Err() == nil && r.noteInferenceWork <= pdfNoteInferenceLimit
}

// Remove a synthetic boundary only when its entire physical-page flow moved to
// notes. Real blank pages and pages with ordinary content/anchors remain. Pages
// and source regions independently retain every original physical boundary.
func (r *pdfReader) removePDFNoteFlow(removed map[*Block]bool) {
	var out []Block
	boundary, start := -1, 0
	flush := func(end int) {
		moved, remaining := false, false
		for i := start; i < end; i++ {
			if removed[&r.doc.Blocks[i]] {
				moved = true
			} else {
				remaining = true
			}
		}
		if boundary >= 0 {
			b := r.doc.Blocks[boundary]
			if !moved || remaining || b.Source != nil {
				out = append(out, b)
			} else {
				r.doc.warn("pdf_note_flow_boundary_inferred", "A physical page whose complete flow moved to inferred notes no longer creates an empty reflow page; original physical pages remain recorded")
			}
		}
		for i := start; i < end; i++ {
			if !removed[&r.doc.Blocks[i]] {
				out = append(out, r.doc.Blocks[i])
			}
		}
	}
	for i, b := range r.doc.Blocks {
		if b.Kind == PageBreak {
			flush(i)
			boundary, start = i, i+1
		}
	}
	flush(len(r.doc.Blocks))
	r.doc.Blocks = out
}
