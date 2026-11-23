package manydoc

import (
	"fmt"
	"math"
	"strings"

	"github.com/benoitkugler/textlayout/language"
	"golang.org/x/text/unicode/norm"
)

const pdfShapingSegmentLimit = 65536

func pdfAdvance(line pdfLine) float64 {
	width := 0.0
	for _, span := range line {
		width += span.width
	}
	return width
}
func shapingSpan(span pdfSpan) bool {
	return span.run.Note == nil && span.glyphs != nil && pdfShapableText(span.text)
}
func shapingAtom(atom pdfTextAtom) bool {
	if len(atom.spans) == 0 || !shapingSpan(atom.spans[0]) {
		return false
	}
	for _, span := range atom.spans[1:] {
		if span.run.Note == nil {
			return false
		}
	}
	return true
}

// Resolve Common/Inherited graphemes from adjacent text inside each shaping
// segment. Keep distinct strong scripts separate instead of shaping a mixed
// paragraph with whichever OpenType script happens to appear first.
func resolveShapingScripts(atoms []pdfTextAtom, size float64) {
	weak := func(s language.Script) bool { return s == language.Common || s == language.Inherited || s == 0 }
	for start := 0; start < len(atoms); {
		if !shapingAtom(atoms[start]) {
			start++
			continue
		}
		end := start + 1
		for len(atoms[end-1].spans) == 1 && end < len(atoms) && shapingAtom(atoms[end]) && sameShapingContext(atoms[start].spans[0], atoms[end].spans[0], size) {
			end++
		}
		next := make([]language.Script, end-start)
		following := language.Common
		for i := end - 1; i >= start; i-- {
			s := atoms[i].spans[0].glyphs.script
			if !weak(s) {
				following = s
			}
			next[i-start] = following
		}
		previous := language.Common
		for i := start; i < end; i++ {
			c := atoms[i].spans[0].glyphs
			if weak(c.script) {
				c.script = previous
				if weak(c.script) {
					c.script = next[i-start]
				}
			} else {
				previous = c.script
			}
		}
		start = end
	}
}

// Shape before choosing break opportunities. HarfBuzz cluster advances supply
// exact paragraph measurements; a ligature's advance is apportioned among its
// input graphemes for emergency wrapping, then the actual chosen line is shaped
// afresh. Never split or paint a previously shaped ligature at a guessed offset.
func (r *pdfRenderer) measureShapedAtoms(atoms []pdfTextAtom, size float64) error {
	resolveShapingScripts(atoms, size)
	for start := 0; start < len(atoms); {
		if !shapingAtom(atoms[start]) {
			start++
			continue
		}
		end := start + 1
		for len(atoms[end-1].spans) == 1 && end < len(atoms) && shapingAtom(atoms[end]) && sameShapingContext(atoms[start].spans[0], atoms[end].spans[0], size) && atoms[start].spans[0].glyphs.script == atoms[end].spans[0].glyphs.script {
			end++
		}
		var text strings.Builder
		var owners []int
		for i := start; i < end; i++ {
			s := atoms[i].spans[0].text
			text.WriteString(s)
			for range []rune(norm.NFC.String(s)) {
				owners = append(owners, i-start)
			}
		}
		run := atoms[start].spans[0].run
		face, err := r.resolveFont(run)
		if err != nil {
			return err
		}
		shaped, err := r.shapeGlyphTextScript(face, text.String(), run, pdfShapingSegmentLimit, atoms[start].spans[0].glyphs.script)
		if err != nil {
			return err
		}
		widths := make([]float64, end-start)
		positions := shaped.positions
		for first := 0; first < len(positions); {
			last := first + 1
			for last < len(positions) && positions[last].cluster == positions[first].cluster {
				last++
			}
			from, to := positions[first].cluster, len(owners)
			if last < len(positions) {
				to = positions[last].cluster
			}
			if from < 0 || from >= to || to > len(owners) {
				return fmt.Errorf("%w: invalid LTR shaping cluster order", ErrUnsupported)
			}
			a, b := owners[from], owners[to-1]+1
			advance, total := 0.0, 0.0
			for _, g := range positions[first:last] {
				advance += g.advance
			}
			if !finite(advance) || advance < 0 {
				return fmt.Errorf("%w: invalid shaping cluster advance", ErrUnsupported)
			}
			for i := a; i < b; i++ {
				total += atoms[start+i].spans[0].width
			}
			for i := a; i < b; i++ {
				fraction := 1 / float64(b-a)
				if total > 0 {
					fraction = atoms[start+i].spans[0].width / total
				}
				widths[i] += advance * textSize(run, size) * fraction
			}
			first = last
		}
		for i, value := range widths {
			atom := &atoms[start+i]
			atom.width += value - atom.spans[0].width
			atom.spans[0].width = value
		}
		start = end
	}
	return nil
}

// The line boundary is a new shaping boundary. Recompute substitutions, kerning,
// ink extents and advances before committing a line. The caller backs off and
// retries if its paragraph measurement underestimated the final line width.
func (r *pdfRenderer) shapeTextLine(line pdfLine, size float64) (pdfLine, error) {
	var result pdfLine
	for start := 0; start < len(line); {
		if !shapingSpan(line[start]) {
			result = appendPDFSpan(result, line[start])
			start++
			continue
		}
		end := start + 1
		for end < len(line) && shapingSpan(line[end]) && sameShapingContext(line[start], line[end], size) && line[start].glyphs.script == line[end].glyphs.script {
			end++
		}
		var text strings.Builder
		span := line[start]
		for _, s := range line[start:end] {
			text.WriteString(s.text)
			span.ascent = math.Max(span.ascent, s.ascent)
			span.descent = math.Max(span.descent, s.descent)
		}
		face, err := r.resolveFont(span.run)
		if err != nil {
			return nil, err
		}
		shaped, err := r.shapeGlyphTextScript(face, text.String(), span.run, pdfShapingSegmentLimit, span.glyphs.script)
		if err != nil {
			return nil, err
		}
		drawSize := textSize(span.run, size)
		for _, p := range shaped.positions {
			if !finite(p.x) || !finite(p.y) || math.Abs(p.x*drawSize) > 14400 || math.Abs(p.y*drawSize) > 14400 {
				return nil, fmt.Errorf("%w: shaped glyph offsets exceed PDF coordinate bounds", ErrUnsupported)
			}
		}
		painted, err := splitShapedPaint(line[start:end], text.String(), shaped, size)
		if err != nil {
			return nil, err
		}
		result = append(result, painted...)
		start = end
	}
	return result, nil
}
