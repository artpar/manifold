package engine

import (
	"bytes"
	"compress/zlib"
	"context"
	"encoding/hex"
	"fmt"
	"github.com/artpar/manifold/internal/bidilevels"
	"github.com/benoitkugler/textlayout/language"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	// Source positions count original scalars, before NFC, note-label substitution
	// or glyph clustering. A hard paragraph separator belongs to its preceding
	// paragraph, but is excluded from that paragraph's bidi input (P1). CRLF is one
	// boundary. U+2028 is a line boundary and retains its paragraph's context.
	"github.com/rivo/uniseg"
	"golang.org/x/image/font"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
	"golang.org/x/text/unicode/bidi"
	"golang.org/x/text/unicode/norm"
	"io"
	"math"
	"sort"
	"strings"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"
)

type pdfBidiParagraph struct {
	start, end int
	resolved   *bidilevels.Paragraph
	levels     []uint8
}

type pdfBidiRange struct {
	paragraph  *pdfBidiParagraph
	start, end int
	level      uint8
}

func pdfBidiParagraphs(ctx context.Context, text string, direction string) ([]*pdfBidiParagraph, error) {
	return pdfBidiParagraphsRuns(ctx, text, direction, nil)
}
func pdfBidiParagraphsRuns(ctx context.Context, text string, direction string, runs []pdfRunRange) ([]*pdfBidiParagraph, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	base := 0 // The model's unset paragraph flow has the standard LTR default.
	if direction == "rtl" {
		base = 1
	}
	runes := []rune(text)
	classes := pdfRunBidiTypes(text, runs)
	var paragraphs []*pdfBidiParagraph
	for start := 0; start < len(runes); {
		end := start
		for end < len(runes) {
			if end%1024 == 0 {
				if err := ctx.Err(); err != nil {
					return nil, err
				}
			}
			properties, _ := bidi.LookupRune(runes[end])
			if properties.Class() == bidi.B {
				break
			}
			end++
		}
		resolved, err := bidilevels.ResolveWithTypes(ctx, string(runes[start:end]), base, classes[start:end])
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, fmt.Errorf("%w: PDF paragraph bidi: %v", ErrUnsupported, err)
		}
		limit := end
		if limit < len(runes) {
			limit++
			if runes[end] == '\r' && limit < len(runes) && runes[limit] == '\n' {
				limit++
			}
		}
		paragraphs = append(paragraphs, &pdfBidiParagraph{start: start, end: limit, resolved: resolved, levels: resolved.Levels()})
		start = limit
	}
	return paragraphs, nil
}

func (p *pdfBidiParagraph) sourceRange(start, end int) (pdfBidiRange, error) {
	if start < p.start || end > p.end || end < start {
		return pdfBidiRange{}, fmt.Errorf("%w: grapheme crosses a PDF bidi paragraph", ErrUnsupported)
	}
	level := p.resolved.Base()
	a, b := min(start-p.start, len(p.levels)), min(end-p.start, len(p.levels))
	if a < b {
		level = p.levels[a]
		for _, l := range p.levels[a:b] {
			if l != level {
				return pdfBidiRange{}, fmt.Errorf("%w: inconsistent bidi levels inside a PDF grapheme", ErrUnsupported)
			}
		}
	}
	return pdfBidiRange{paragraph: p, start: start, end: end, level: level}, nil
}

func samePDFBidiContext(a, b pdfBidiRange) bool {
	return a.paragraph == b.paragraph && a.level == b.level && (a.paragraph == nil || a.end == b.start)
}

// A line keeps paragraph resolution; L1 resets only the selected line's trailing
// whitespace/control levels. Call while spans are still individual graphemes so
// that a reset cannot be hidden inside an already coalesced glyph span.
func applyPDFLineLevels(line pdfLine) error {
	if len(line) == 0 || line[0].bidi.paragraph == nil {
		return nil
	}
	p := line[0].bidi.paragraph
	start, end := line[0].bidi.start, line[len(line)-1].bidi.end
	a, b := min(start-p.start, p.resolved.Len()), min(end-p.start, p.resolved.Len())
	resolved, err := p.resolved.Line(a, b)
	if err != nil {
		return fmt.Errorf("%w: PDF bidi line: %v", ErrUnsupported, err)
	}
	for i := range line {
		s := &line[i]
		if s.bidi.paragraph != p {
			return fmt.Errorf("%w: PDF line crosses bidi paragraphs", ErrUnsupported)
		}
		from, to := min(s.bidi.start-p.start, p.resolved.Len()), min(s.bidi.end-p.start, p.resolved.Len())
		s.bidi.level = p.resolved.Base()
		if from < to {
			s.bidi.level = resolved.Levels[from-a]
			for _, l := range resolved.Levels[from-a : to-a] {
				if l != s.bidi.level {
					return fmt.Errorf("%w: inconsistent line bidi levels inside a PDF grapheme", ErrUnsupported)
				}
			}
		}
	}
	return nil
}

func extendPDFBidiRange(first pdfBidiRange, last pdfBidiRange) pdfBidiRange {
	first.end = last.end
	return first
}

func originalScalarLength(text string) int { return utf8.RuneCountInString(text) }

// Keep operators in logical order for ActualText/style ownership; place their
// glyph groups using L2. Each shaped span already has one level and its internal
// glyph order/offsets. Note labels stay atomic with their preceding anchor, as
// they do during line admission; their digits retain their own glyph direction.
func placePDFLine(line pdfLine, width float64) {
	if len(line) == 0 || line[0].bidi.paragraph == nil {
		return
	}
	levels := make([]uint8, len(line))
	order := make([]int, len(line))
	maximum, lowestOdd := uint8(0), uint8(255)
	for i, s := range line {
		l := s.bidi.level
		if s.run.Note != nil && i > 0 && !pdfHardBreak(line[i-1].text) {
			l = levels[i-1]
		}
		levels[i], order[i] = l, i
		maximum = max(maximum, l)
		if l%2 != 0 {
			lowestOdd = min(lowestOdd, l)
		}
	}
	for l := int(maximum); l >= int(lowestOdd); l-- {
		for start := 0; start < len(order); {
			if int(levels[order[start]]) < l {
				start++
				continue
			}
			end := start + 1
			for end < len(order) && int(levels[order[end]]) >= l {
				end++
			}
			for a, b := start, end-1; a < b; a, b = a+1, b-1 {
				order[a], order[b] = order[b], order[a]
			}
			start = end
		}
	}
	x := 0.0
	if line[0].bidi.paragraph.resolved.Base()%2 != 0 {
		x = max(0, width-pdfAdvance(line))
	}
	for _, i := range order {
		line[i].placed, line[i].visualX = true, x
		x += line[i].width
	}
}

func visualPDFLine(line pdfLine) pdfLine {
	if len(line) == 0 || !line[0].placed {
		return line
	}
	visual := append(pdfLine(nil), line...)
	sort.SliceStable(visual, func(i, j int) bool { return visual[i].visualX < visual[j].visualX })
	return visual
}

// OOXML run rtl supplies higher-level direction characteristics for neutral and
// selected weak characters. Strong letters and the numeric exceptions keep their
// intrinsic types; explicit ltr never becomes a strong-letter override.

// A measured label retains ordinary inner text runs and one outer note atom.
// Wrapping, bidi anchoring, footnote admission and the link see the same object.

// Bidi format/mark controls affect paragraph levels but have no visible advance.
// Keep their original logical text using the existing zero-width ActualText path.

// Resolve Common/Inherited graphemes from adjacent text inside each shaping
// segment. Keep distinct strong scripts separate instead of shaping a mixed
// paragraph with whichever OpenType script happens to appear first.

// Shape before choosing break opportunities. HarfBuzz cluster advances supply
// exact paragraph measurements; a ligature's advance is apportioned among its
// input graphemes for emergency wrapping, then the actual chosen line is shaped
// afresh. Never split or paint a previously shaped ligature at a guessed offset.

// The line boundary is a new shaping boundary. Recompute substitutions, kerning,
// ink extents and advances before committing a line. The caller backs off and
// retries if its paragraph measurement underestimated the final line width.

func pdfRunBidiTypes(text string, runs []pdfRunRange) []bidilevels.Class {
	runes := []rune(text)
	classes := make([]bidilevels.Class, len(runes))
	for i, ch := range runes {
		p, _ := bidi.LookupRune(ch)
		classes[i] = bidilevels.Class(p.Class())
	}
	hebrewRuns := make([]bool, len(runs))
	for i, r := range runs {
		if r.run.Direction != "rtl" {
			continue
		}
		lang :=

			// HarfBuzz item context selects joining forms across style/font/size/language
			// and transparent-control boundaries without forming cross-face ligatures.
			// The actual final line is the context boundary; no adjacent line is borrowed.

			// Unicode-defined advances use the current em. Thin/hair/narrow spaces use the
			// declared 1/5, 1/10 and 1/5 em defaults; figure/punctuation spaces use font metrics.

			// Segment the whole paragraph, not each formatting run. A grapheme and a word
			// can cross run boundaries. Note markers form an atom with the preceding cluster.

			strings.ToLower(strings.SplitN(r.run.Language, "-", 2)[0])
		complexLang := strings.ToLower(strings.SplitN(r.run.ComplexLanguage, "-", 2)[0])
		hebrewRuns[i] = lang == "he" || complexLang == "he"
		for _, ch := range text[r.start:r.end] {
			hebrewRuns[i] = hebrewRuns[i] || unicode.Is(unicode.Hebrew, ch)
		}
	}
	owner, scalar := 0, 0
	for offset, ch := range text {
		for owner < len(runs) && runs[owner].end <= offset {
			owner++
		}
		if owner < len(runs) && runs[owner].run.Direction == "rtl" && !pdfHardBreak(string(ch)) {
			switch classes[scalar] {
			case bidilevels.WS, bidilevels.ON, bidilevels.NSM:

				// Generated decimal note labels are an LTR inline object even in
				// an RTL source context. Placement still follows the anchor level.

				classes[scalar] = bidilevels.R
			case bidilevels.ES:
				number := false
				if hebrewRuns[owner] {
					for _, i := range []int{scalar - 1, scalar + 1} {
						if i >= 0 && i < len(runes) {
							p, _ := bidi.LookupRune(runes[i])
							number = number || p.Class() == bidi.EN || p.Class() == bidi.AN
						}
					}
				}
				if !number {
					classes[scalar] = bidilevels.R
				}
			}
		}
		scalar++
	}

	return classes
}

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

func pdfBidiControl(ch rune) bool {
	return ch == 0x061c || ch == 0x200e || ch == 0x200f || ch >= 0x202a && ch <= 0x202e || ch >= 0x2066 && ch <= 0x2069
}

func pdfHasBidiText(text string) bool {
	for _, ch := range text {
		p, _ := bidi.LookupRune(ch)
		if p.Class() == bidi.R || p.Class() == bidi.AL || p.Class() == bidi.AN || pdfBidiControl(ch) {
			return true
		}
	}
	return false
}

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

func (r *pdfRenderer) measureShapedAtoms(atoms []pdfTextAtom, size float64) error {
	resolveShapingScripts(atoms, size)
	var paragraph pdfLine
	atomSpan := make([]int, len(atoms))
	for i, a := range atoms {
		atomSpan[i] = len(paragraph)
		paragraph = append(paragraph, a.spans...)
	}
	context, offsets := pdfShapingContext(paragraph)
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
		shaped, err := r.shapeGlyphTextContext(face, text.String(), run, pdfShapingSegmentLimit, atoms[start].spans[0].glyphs.script, atoms[start].spans[0].bidi.level, context, offsets[atomSpan[start]])
		if err != nil {
			return err
		}
		widths := make([]float64, end-start)
		positions := shaped.positions
		if shaped.rtl {
			positions = append([]pdfGlyphPosition(nil), positions...)
			sort.SliceStable(positions, func(i, j int) bool { return positions[i].cluster < positions[j].cluster })
		}
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

func (r *pdfRenderer) shapeTextLine(line pdfLine, size float64) (pdfLine, error) {
	context, offsets := pdfShapingContext(line)
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
		shaped, err := r.shapeGlyphTextContext(face, text.String(), span.run, pdfShapingSegmentLimit, span.glyphs.script, span.bidi.level, context, offsets[start])
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

func pdfShapingContext(line pdfLine) ([]rune, []int) {
	var context []rune
	offsets := make([]int, len(line))
	for i, s := range line {
		offsets[i] = len(context)
		if s.run.Note != nil {
			context = append(context, '\ufffc')
		} else {
			context = append(context, []rune(norm.NFC.String(s.text))...)
		}
	}
	return context, offsets
}

func pdfSpecial(ch rune) bool {
	if pdfBidiControl(ch) {
		return true
	}
	switch ch {
	case '\t', '\r', '\n', 0x85, 0xa0, 0xad, 0x2000, 0x2001, 0x2002, 0x2003, 0x2004, 0x2005, 0x2006, 0x2007, 0x2008, 0x2009, 0x200a, 0x200b, 0x2010, 0x2011, 0x2028, 0x2029, 0x202f, 0x205f, 0x2060, 0x3000, 0xfeff:
		return true
	}
	return false
}
func pdfHardBreak(text string) bool {
	return text == "\r\n" || text == "\n" || text == "\r" || text == "\u0085" || text == "\u2028" || text == "\u2029"
}
func pdfGlue(text string) bool {
	return strings.ContainsAny(text, "\u00a0\u2007\u2011\u202f\u2060\ufeff")
}
func pdfTrimSpace(text string) bool {
	if text == "" || pdfGlue(text) || pdfHardBreak(text) {
		return false
	}
	for _, ch := range text {
		if !unicode.IsSpace(ch) {
			return false
		}
	}
	return true
}
func (r *pdfRenderer) specialAdvance(ch rune, size float64) float64 {
	switch ch {
	case '\t':
		return 4 * r.pdf.GetStringWidth(" ")
	case 0xa0:
		return r.pdf.GetStringWidth(" ")
	case 0x2000, 0x2002:
		return size / 2
	case 0x2001, 0x2003, 0x3000:
		return size
	case 0x2004:
		return size / 3
	case 0x2005:
		return size / 4
	case 0x2006:
		return size / 6
	case 0x2007:
		return r.pdf.GetStringWidth("0")
	case 0x2008:
		return r.pdf.GetStringWidth(".")
	case 0x2009, 0x202f:
		return size / 5
	case 0x200a:
		return size / 10
	case 0x205f:
		return 2 * size / 9
	case 0x2010, 0x2011:
		return r.pdf.GetStringWidth("-")
	}
	return 0
}
func (r *pdfRenderer) textAdvance(text string, size float64) float64 {
	width := 0.0
	for _, ch := range norm.NFC.String(text) {
		if pdfSpecial(ch) {
			width += r.specialAdvance(ch, size)
		} else {
			width += r.pdf.GetStringWidth(string(ch))
		}
	}
	return width
}
func samePDFStyle(a, b Run) bool {
	return a.ComplexScript == b.ComplexScript && a.Language == b.Language && a.EastAsiaLanguage == b.EastAsiaLanguage && a.ComplexLanguage == b.ComplexLanguage && a.Direction == b.Direction && a.Bold == b.Bold && a.Italic == b.Italic && a.Underline == b.Underline && a.Strike == b.Strike && a.Link == b.Link && a.Destination == b.Destination && a.Note == nil && b.Note == nil && a.FontFamily == b.FontFamily && a.FontSize == b.FontSize && canonicalTextColor(a.Color) == canonicalTextColor(b.Color)
}
func appendPDFSpan(line pdfLine, span pdfSpan) pdfLine {
	zeroControl := span.width == 0 && span.text != "\u00ad" && !pdfHardBreak(span.text) && !pdfTrimSpace(span.text)
	if span.glyphs == nil && len(line) > 0 && line[len(line)-1].glyphs == nil && span.text != "\u00ad" && line[len(line)-1].text != "\u00ad" && (span.width > 0 || zeroControl) && line[len(line)-1].width > 0 && samePDFStyle(line[len(line)-1].run, span.run) && samePDFBidiContext(line[len(line)-1].bidi, span.bidi) {
		last := &line[len(line)-1]
		last.text += span.text
		last.width += span.width
		last.bidi = extendPDFBidiRange(last.bidi, span.bidi)
		return line
	}
	return append(line, span)
}

type pdfTextAtom struct {
	spans                             pdfLine
	width, hyphenWidth                float64
	breaking                          int
	hard, trim, protected, softHyphen bool
}
type pdfRunRange struct {
	start, end int
	run        Run
}

func (r *pdfRenderer) textAtoms(runs []Run, size float64) ([]pdfTextAtom, error) {
	return r.textAtomsDirection(runs, size, "")
}
func (r *pdfRenderer) textAtomsDirection(runs []Run, size float64, direction string) ([]pdfTextAtom, error) {
	var text strings.Builder
	var ranges []pdfRunRange
	for _, run := range runs {
		run = canonicalCitationRun(run)
		start := text.Len()
		if run.Note != nil {
			text.WriteRune('\ufffc')
		} else {
			text.WriteString(run.Text)
		}
		if text.Len() > start {
			ranges = append(ranges, pdfRunRange{start, text.Len(), run})
		}
	}
	paragraphs, err := pdfBidiParagraphsRuns(r.ctx, text.String(), direction, ranges)
	if err != nil {
		return nil, err
	}
	paragraphIndex, scalarOffset := 0, 0
	var atoms []pdfTextAtom
	appendAtom := func(atom pdfTextAtom, note bool) {
		if note && len(atoms) > 0 && !atoms[len(atoms)-1].hard {
			previous := &atoms[len(atoms)-1]
			previous.spans = append(previous.spans, atom.spans...)
			previous.width += atom.width
			previous.breaking = atom.breaking
			previous.trim = false
			previous.softHyphen = false
		} else {
			atoms = append(atoms, atom)
		}
	}
	rangeIndex := 0
	graphemes := uniseg.NewGraphemes(text.String())
	for graphemes.Next() {
		if err := r.ctx.Err(); err != nil {
			return nil, err
		}
		start, end := graphemes.Positions()
		for rangeIndex < len(ranges) && ranges[rangeIndex].end <= start {
			rangeIndex++
		}
		run := ranges[rangeIndex].run
		for i := rangeIndex + 1; i < len(ranges) && ranges[i].start < end; i++ {
			if !samePDFStyle(run, ranges[i].run) {
				return nil, fmt.Errorf("%w: formatting boundary inside a grapheme cluster", ErrUnsupported)
			}
		}
		atom := pdfTextAtom{breaking: graphemes.LineBreak()}
		cluster := graphemes.Str()
		scalarEnd := scalarOffset + originalScalarLength(cluster)
		for paragraphIndex+1 < len(paragraphs) && paragraphs[paragraphIndex].end <= scalarOffset {
			paragraphIndex++
		}
		sourceRange, err := paragraphs[paragraphIndex].sourceRange(scalarOffset, scalarEnd)
		if err != nil {
			return nil, err
		}
		scalarOffset = scalarEnd
		if run.Note != nil && r.notes != nil {
			label := noteReferenceLabel(r.notes, run.Note)
			_, directionErr := r.noteMarkLevel(label, run.Direction)
			if run.Note.LabelContent != nil || directionErr != nil {
				composite, err := r.citationAtom(run, size, sourceRange, atom.breaking)
				if err != nil {
					return nil, err
				}
				appendAtom(composite, true)
				continue
			}
		}
		if full := textSize(run, size); full > 14400 || fmt.Sprintf("%.2f", full) == "0.00" || math.Abs(full-math.Round(full*100)/100) > 1e-9 {
			return nil, fmt.Errorf("%w: PDF font size exceeds coordinate range or 0.01pt precision", ErrUnsupported)
		}
		drawSize := textSize(run, size)
		if run.Note != nil {
			if r.notes == nil {
				return nil, fmt.Errorf("missing note catalog")
			}
			_, ok := r.notes.labels[referenceKey(run.Note)]
			if !ok {
				return nil, fmt.Errorf("missing note reference label")
			}
			cluster = noteReferenceLabel(r.notes, run.Note)
			drawSize = math.Round(drawSize*.7*100) / 100
		}
		checked := run
		checked.Text = cluster
		face, err := r.resolveFont(run)
		if err != nil {
			return nil, err
		}
		if err := validateFontText(checked, face.parsed); err != nil {
			return nil, err
		}
		var glyphs *pdfGlyphCluster
		if needsGlyphOutput(cluster) || pdfShapableText(cluster) {
			glyphLevel := sourceRange.level

			if run.Note != nil {
				glyphLevel, err = r.noteMarkLevel(cluster, run.Direction)
				if err != nil {
					return nil, err
				}
			}
			glyphs, err = r.shapeGlyphTextLevel(face, cluster, run, 1024, 0, glyphLevel)
			if err != nil {
				return nil, err
			}
			atom.width = glyphs.width * drawSize
			for _, position := range glyphs.positions {
				if !finite(position.x) || !finite(position.y) || math.Abs(position.x*drawSize) > 14400 || math.Abs(position.y*drawSize) > 14400 {
					return nil, fmt.Errorf("%w: glyph offsets exceed PDF coordinate bounds", ErrUnsupported)
				}
			}
		} else {
			if r.fonts != nil {
				face, err = r.registerFont(face.key)
				if err != nil {
					return nil, err
				}
			}
			r.pdf.SetFont(face.id, runStyle(run), drawSize)
			atom.width = r.textAdvance(cluster, drawSize)
		}
		fullSize := textSize(run, size)
		if fullSize > 14400 {
			return nil, fmt.Errorf("%w: PDF font size exceeds page coordinate range", ErrUnsupported)
		}
		var buffer sfnt.Buffer
		metrics, err := face.parsed.Metrics(&buffer, fixed.Int26_6(drawSize*64), font.HintingNone)
		if err != nil {
			return nil, err
		}
		rise := 0.0
		if run.Note != nil {
			rise = fullSize * .3
		}
		atom.spans = pdfLine{{bidi: sourceRange, run: run, text: cluster, width: atom.width, ascent: math.Max(fullSize, float64(metrics.Ascent)/64+rise), descent: float64(metrics.Descent)/64 - rise, glyphs: glyphs}}
		if glyphs != nil {
			atom.spans[0].ascent = math.Max(atom.spans[0].ascent, glyphs.ascent*drawSize+rise)
			atom.spans[0].descent = math.Max(atom.spans[0].descent, glyphs.descent*drawSize-rise)
		}
		atom.hard = run.Note == nil && pdfHardBreak(cluster)
		atom.trim = run.Note == nil && pdfTrimSpace(cluster)
		atom.protected = run.Note == nil && pdfGlue(cluster)
		atom.softHyphen = run.Note == nil && cluster == "\u00ad"
		if atom.softHyphen {
			atom.hyphenWidth = r.pdf.GetStringWidth("-")
		}
		appendAtom(atom, run.Note != nil)
	}
	if err := r.measureShapedAtoms(atoms, size); err != nil {
		return nil, err
	}
	return atoms, nil
}

func (r *pdfRenderer) lines(runs []Run, width, size float64) ([]pdfLine, error) {
	return r.linesDirection(runs, width, size, "")
}
func (r *pdfRenderer) linesDirection(runs []Run, width, size float64, direction string) ([]pdfLine, error) {
	if err := r.ctx.Err(); err != nil {
		return nil, err
	}
	if !finite(width) || width <= 0 || !finite(size) || size <= 0 {
		return nil, fmt.Errorf("invalid PDF text dimensions")
	}
	atoms, err := r.textAtomsDirection(runs, size, direction)
	if err != nil {
		return nil, err
	}
	if len(atoms) == 0 {
		return bindPDFLineAnchors(runs, []pdfLine{{}}), nil
	}
	protectedGroup := make([]bool, len(atoms))
	for i := len(atoms) - 1; i >= 0; i-- {
		protectedGroup[i] = atoms[i].protected
		if i+1 < len(atoms) && atoms[i].breaking == uniseg.LineDontBreak && !atoms[i].hard {
			protectedGroup[i] = protectedGroup[i] || protectedGroup[i+1]
		}
	}
	var lines []pdfLine
	emit := func(start, end int, optional bool) (bool, error) {
		// Whitespace at a soft line boundary may extend past the text area. Keep
		// its logical value using a zero-advance ActualText span instead of dropping it.
		trailing := end
		for trailing > start && (atoms[trailing-1].trim || atoms[trailing-1].hard) {
			trailing--
		}
		used := 0.0
		line := pdfLine{}
		for i := start; i < end; i++ {
			for _, span := range atoms[i].spans {
				if i == end-1 && optional && atoms[i].softHyphen {
					span.width = atoms[i].hyphenWidth
				}
				if i >= trailing && atoms[i].trim && used+span.width > width+1e-7 {
					span.width = 0
					span.glyphs = nil
				}
				line = append(line, span)
				used += span.width
			}
		}
		if err := applyPDFLineLevels(line); err != nil {
			return false, err
		}
		shaped, err := r.shapeTextLine(line, size)
		if err != nil {
			return false, err
		}
		if pdfAdvance(shaped) > width+1e-7 && trailing < end {
			// Re-shaping can remove kerning across the selected line boundary.
			// Keep overflowing terminal whitespace logically, at zero advance.
			for i := len(line) - 1; i >= 0; i-- {
				if pdfHardBreak(line[i].text) {
					continue
				}
				if !pdfTrimSpace(line[i].text) {
					break
				}
				line[i].width = 0
				line[i].glyphs = nil
			}
			shaped, err = r.shapeTextLine(line, size)
			if err != nil {
				return false, err
			}
		}
		if pdfAdvance(shaped) > width+1e-7 {
			return false, nil
		}
		placePDFLine(shaped, width)
		lines = append(lines, shaped)
		return true, nil
	}
	emitFit := func(start, end int, optional bool) (int, error) {
		for end > start {
			fits, err := emit(start, end, optional)
			if err != nil {
				return start, err
			}
			if fits {
				return end, nil
			}
			if protectedGroup[start] {
				return start, fmt.Errorf("%w: reshaped nonbreaking PDF text exceeds the available width", ErrUnsupported)
			}
			end--
			for j := end; j > start; j-- {
				if atoms[j-1].breaking != uniseg.LineDontBreak {
					end = j
					break
				}
			}
		}
		return start, fmt.Errorf("%w: reshaped grapheme or note anchor wider than PDF content area", ErrUnsupported)
	}
	for start := 0; start < len(atoms); {
		used, trailing := 0.0, 0.0
		candidate := start
		finished := false
		for i := start; i < len(atoms); i++ {
			if err := r.ctx.Err(); err != nil {
				return nil, err
			}
			atom := atoms[i]
			if atom.hard {
				next, err := emitFit(start, i+1, false)
				if err != nil {
					return nil, err
				}
				start = next
				finished = true
				if start == len(atoms) {
					blank := atom.spans[len(atom.spans)-1]
					blank.text, blank.width = "", 0
					blank.bidi.start = blank.bidi.end
					lines = append(lines, pdfLine{blank})
				}
				break
			}
			used += atom.width
			if atom.trim {
				trailing += atom.width
			} else {
				trailing = 0
			}
			if !atom.trim && used > width+1e-7 {
				if candidate > start {
					next, err := emitFit(start, candidate, true)
					if err != nil {
						return nil, err
					}
					start = next
					finished = true
					break
				}
				// Emergency wrapping observes grapheme boundaries, while protected words
				// containing glue/joiners remain indivisible and reject genuine overflow.
				if protectedGroup[start] {
					return nil, fmt.Errorf("%w: nonbreaking PDF text exceeds the available width", ErrUnsupported)
				}
				if i == start {
					return nil, fmt.Errorf("%w: grapheme or note anchor wider than PDF content area", ErrUnsupported)
				}
				end := i
				prefix := used - atom.width
				// Consecutive discretionary controls can all end at the same
				// advance. Back off every overflowing hyphen candidate, not
				// only the last one, before choosing an emergency boundary.
				for end > start && atoms[end-1].softHyphen && prefix+atoms[end-1].hyphenWidth > width+1e-7 {
					end--
					prefix -= atoms[end].width
				}
				if end == start {
					return nil, fmt.Errorf("%w: discretionary hyphen cannot fit", ErrUnsupported)
				}
				next, err := emitFit(start, end, true)
				if err != nil {
					return nil, err
				}
				start = next
				finished = true
				break
			}
			if atom.breaking != uniseg.LineDontBreak && used-trailing+atom.hyphenWidth <= width+1e-7 {
				candidate = i + 1
			}
			if i == len(atoms)-1 {
				next, err := emitFit(start, len(atoms), false)
				if err != nil {
					return nil, err
				}
				start = next
				finished = true
				break
			}
		}
		if !finished {
			return nil, fmt.Errorf("PDF line layout made no progress")
		}
	}
	return bindPDFLineAnchors(runs, lines), nil
}

func pdfActualText(text string) string {
	data := []byte{0xfe, 0xff}
	for _, value := range utf16.Encode([]rune(text)) {
		data = append(data, byte(value>>8), byte(value))
	}
	return "/Span << /ActualText <" + strings.ToUpper(hex.EncodeToString(data)) + "> >> BDC"
}
func (r *pdfRenderer) replacementText(x, y float64, display string, width float64) {
	r.pdf.RawWriteStr("q")
	base := r.pdf.GetStringWidth(display)
	if base > 0 {
		r.pdf.RawWriteStr(fmt.Sprintf("%.8f Tz", 100*width/base))
	}
	r.pdf.Text(x, y, display)
	r.pdf.RawWriteStr("Q")
}
func (r *pdfRenderer) paintSpan(span pdfSpan, x, y, size float64) {
	if span.glyphs != nil {
		r.paintGlyphSpan(span, x, y, size)
		return
	}
	logical := norm.NFC.String(span.text) != span.text
	for _, ch := range span.text {
		logical = logical || pdfSpecial(ch)
	}
	collapsed := span.width == 0 && strings.TrimFunc(span.text, unicode.IsSpace) == ""
	if logical || collapsed {
		r.pdf.RawWriteStr("q\n" + pdfActualText(span.text))
		defer r.pdf.RawWriteStr("EMC\nQ")
	}
	if collapsed {
		r.replacementText(x, y, " ", 0)
		return
	}
	var chunk strings.Builder
	flush := func() {
		if chunk.Len() == 0 {
			return
		}
		display := norm.NFC.String(chunk.String())
		r.pdf.Text(x, y, display)
		x += r.pdf.GetStringWidth(display)
		chunk.Reset()
	}
	for _, ch := range span.text {
		if !pdfSpecial(ch) {
			chunk.WriteRune(ch)
			continue
		}
		flush()
		width := r.specialAdvance(ch, size)
		display := " "
		if ch == 0x2010 || ch == 0x2011 || (ch == 0xad && span.width > 0 && span.text == "\u00ad") {
			display = "-"
			width = r.pdf.GetStringWidth("-")
		}
		r.replacementText(x, y, display, width)
		x += width
	}
	flush()
}
func (r *pdfRenderer) drawLine(line pdfLine, x, y, size float64) {
	baseline := pdfLineSize(line, size)
	origin := x
	cursor := x
	var actualGroup *pdfPaintActualGroup
	var outputOwner *pdfOutputNode
	marked := false
	for _, span := range line {
		if span.run.Anchor != "" {
			r.bindDestinations([]string{span.run.Anchor}, y)
			continue
		}
		x = cursor
		if span.placed {
			x = origin + span.visualX
		}
		var group *pdfPaintActualGroup
		if span.glyphs != nil {
			group = span.glyphs.actualGroup
		}
		if r.outputStructure != nil {
			if span.citation != nil || span.text == "" && group == nil {
				if actualGroup != nil {
					r.pdf.RawWriteStr("EMC")
					actualGroup = nil
				}
				if marked {
					r.pdf.RawWriteStr("EMC")
					marked = false
				}
				outputOwner = nil
			} else if span.run.pdfOutput != outputOwner || !marked {
				if actualGroup != nil {
					r.pdf.RawWriteStr("EMC")
					actualGroup = nil
				}
				if marked {
					r.pdf.RawWriteStr("EMC")
				}
				outputOwner = span.run.pdfOutput
				marked = r.beginOutputMark(outputOwner)
			}
		}
		if group != actualGroup {
			if actualGroup != nil {
				r.pdf.RawWriteStr("EMC")
			}
			if group != nil {
				r.pdf.RawWriteStr(pdfActualText(group.text))
			}
			actualGroup = group
		}
		drawSize, rise := textSize(span.run, size), 0.0
		if span.run.Note != nil {
			drawSize = math.Round(drawSize*.7*100) / 100
			rise = textSize(span.run, size) * .3
		}
		if span.citation != nil {
			box := span.citation
			r.drawLine(box.line, x, y+baseline-box.rise-pdfLineSize(box.line, box.size), box.size)
		} else {
			if span.glyphs == nil {
				face, err := r.font(span.run)
				if err != nil {
					r.pdf.SetError(err)
					return
				}
				r.pdf.SetFont(face.id, runStyle(span.run), drawSize)
			}
			red, green, blue := textRGB(span.run.Color)
			r.pdf.SetTextColor(red, green, blue)
			r.paintSpan(span, x, y+baseline-rise, drawSize)
		}
		if span.run.Note != nil && span.width > 0 {
			r.pdf.Link(x, y+baseline-span.ascent, span.width, math.Max(textSize(span.run, size), span.ascent+span.descent), r.noteLinks[referenceKey(span.run.Note)])
		}
		if span.run.Destination != "" && span.width > 0 {
			r.pdf.LinkString(x, y+baseline-span.ascent, span.width, math.Max(textSize(span.run, size)*1.3, span.ascent+span.descent), r.destinationURIs[span.run.Destination])
		}
		if span.run.Destination == "" && span.run.Link != "" && span.width > 0 {
			r.pdf.LinkString(x, y+baseline-span.ascent, span.width, math.Max(textSize(span.run, size)*1.3, span.ascent+span.descent), pdfASCIIURI(span.run.Link))
		}

		cursor += span.width
	}
	if actualGroup != nil {
		r.pdf.RawWriteStr("EMC")
	}
	if marked {
		r.pdf.RawWriteStr("EMC")
	}
	visual := visualPDFLine(line)
	if len(visual) > 0 && visual[0].placed {
		origin += visual[0].visualX
	}
	r.drawLineDecorations(visual, origin, y, baseline, size)
}

// Shaping/control/script boundaries do not break a continuous decoration.
// Keep one stroke over adjacent spans with the same authored style.
func (r *pdfRenderer) drawLineDecorations(line pdfLine, x, y, baseline, size float64) {
	for start := 0; start < len(line); {
		span := line[start]
		end := start + 1
		width := span.width
		for end < len(line) && samePDFStyle(span.run, line[end].run) {
			width += line[end].width
			end++
		}
		if span.run.Underline || span.run.Strike {
			drawSize, rise := textSize(span.run, size), 0.0
			if span.run.Note != nil {
				drawSize = math.Round(drawSize*.7*100) / 100
				rise = textSize(span.run, size) * .3
			}
			red, green, blue := textRGB(span.run.Color)
			old := r.pdf.GetLineWidth()
			or, og, ob := r.pdf.GetDrawColor()
			r.pdf.SetDrawColor(red, green, blue)
			r.pdf.SetLineWidth(math.Max(.3, drawSize*.05))
			if span.run.Underline {
				r.pdf.Line(x, y+baseline-rise+drawSize*.1, x+width, y+baseline-rise+drawSize*.1)
			}
			if span.run.Strike {
				r.pdf.Line(x, y+baseline-rise-drawSize*.4, x+width, y+baseline-rise-drawSize*.4)
			}
			r.pdf.SetLineWidth(old)
			r.pdf.SetDrawColor(or, og, ob)
		}
		x += width
		start = end
	}
}

func pdfLineSize(line pdfLine, fallback float64) float64 {
	size := 0.0
	painted := false
	for _, span := range line {
		if span.run.Anchor != "" {
			continue
		}
		painted = true
		size = math.Max(size, math.Max(textSize(span.run, fallback), span.ascent))
	}
	if !painted {
		return fallback
	}
	return size
}
func pdfLineHeight(line pdfLine, fallback, factor float64) float64 {
	below := 0.0
	painted := false
	for _, span := range line {
		if span.run.Anchor != "" {
			continue
		}
		painted = true
		below = math.Max(below, math.Max(textSize(span.run, fallback)*(factor-1), span.descent))
	}
	if !painted {
		below = fallback * (factor - 1)
	}
	return pdfLineSize(line, fallback) + below
}

// An atomic mark has its own directional content. Decimal marks remain LTR
// inside RTL prose; homogeneous RTL marks must be shaped RTL, not reversed LTR.

// Recover displayed axis-aligned font-advance bounds under known rectangular
// graphics clips. These remain font boxes, not exact glyph ink outlines.

// Retain only values shared by all contributors to an unmappable replacement.

func (r *pdfRenderer) noteMarkLevel(text, direction string) (uint8, error) {
	paragraphs, err := pdfBidiParagraphs(r.ctx, text, direction)
	if err != nil {
		return 0, err
	}
	if len(paragraphs) == 0 {
		return 0, nil
	}
	if len(paragraphs) != 1 {
		return 0, fmt.Errorf("%w: multiline note mark", ErrUnsupported)
	}
	levels := paragraphs[0].levels
	if len(levels) == 0 {
		return 0, nil
	}
	level := levels[0]
	for _, other := range levels {
		if other%2 != level%2 {
			return 0, fmt.Errorf("%w: mixed-direction atomic note mark requires independent internal layout", ErrUnsupported)
		}
	}
	return level, nil
}

func (r *pdfReader) clipTextSource(source *SourceInfo, s *pdfTextState) {
	if s.rulingClipUnknown {
		source.Confidence = min(source.Confidence, .8)
		source.Precision = "unresolved_clip_" + source.Precision
		r.warnPage("pdf_clipping", "Complex text clipping is not applied to recovered bounds")
		return
	}
	if s.rulingClip == nil {
		return
	}
	q := *source.Quad
	rect, ok := rulingRect([]Point{q[0], q[1], q[2], q[3], q[0]})
	if !ok {
		source.Confidence = min(source.Confidence, .8)
		source.Precision = "unresolved_clip_" + source.Precision
		r.warnPage("pdf_clipping", "Rectangular clipping of non-axis-aligned text bounds is unresolved")
		return
	}
	clipped := intersectRulingClip(&rect, s.rulingClip)
	if clipped.x1 <= clipped.x0 || clipped.y1 <= clipped.y0 {
		source.Precision = "fully_clipped_" + source.Precision
		source.Confidence = min(source.Confidence, .5)
		r.warnPage("pdf_clipped_text", "Fully clipped text is retained with original operator bounds; it does not establish visible content")
		return
	}
	if math.Abs(rect.x0-clipped.x0) < 1e-8 && math.Abs(rect.y0-clipped.y0) < 1e-8 && math.Abs(rect.x1-clipped.x1) < 1e-8 && math.Abs(rect.y1-clipped.y1) < 1e-8 {
		return
	}
	result := Quad{{clipped.x0, clipped.y0}, {clipped.x1, clipped.y0}, {clipped.x1, clipped.y1}, {clipped.x0, clipped.y1}}
	source.Quad = &result
	source.Precision = "rect_clipped_" + source.Precision
	if r.actualFrame() == nil {
		r.warnPage("pdf_text_partial_clip", "Partially clipped text operator strings are retained; clipping may hide some character ink")
	}
	if baseline, ok := clipBaseline(*source.Baseline, s.rulingClip); ok {
		source.Baseline = &baseline
	}
}

func clipBaseline(line [2]Point, rect *pdfRulingRect) ([2]Point, bool) {
	a, b := line[0], line[1]
	dx, dy := b.X-a.X, b.Y-a.Y
	low, high := 0.0, 1.0
	for _, edge := range []struct{ p, q float64 }{{-dx, a.X - rect.x0}, {dx, rect.x1 - a.X}, {-dy, a.Y - rect.y0}, {dy, rect.y1 - a.Y}} {
		if edge.p == 0 {
			if edge.q < 0 {
				return line, false
			}
			continue
		}
		t := edge.q / edge.p
		if edge.p < 0 {
			low = max(low, t)
		} else {
			high = min(high, t)
		}
		if low > high {
			return line, false
		}
	}
	return [2]Point{{a.X + low*dx, a.Y + low*dy}, {a.X + high*dx, a.Y + high*dy}}, true
}
func replacementClipState(precision string) uint8 {
	switch {
	case strings.HasPrefix(precision, "rect_clipped_"):
		return 1
	case strings.HasPrefix(precision, "unresolved_clip_"):
		return 2
	case strings.HasPrefix(precision, "fully_clipped_"):
		return 4
	}
	return 0
}
func clippedReplacementPrecision(state uint8) string {
	switch {
	case state&2 != 0:
		return "unresolved_clip_replacement_span_bounds"
	case state == 4:
		return "fully_clipped_replacement_span_bounds"
	case state&4 != 0:
		return "mixed_clip_replacement_span_bounds"
	case state&1 != 0:
		return "rect_clipped_replacement_span_bounds"
	}
	return "replacement_span_bounds"
}
func pdfSubsetName(name string) string {
	if len(name) > 7 && name[6] == '+' {
		valid := true
		for _, c := range name[:6] {
			valid = valid && c >= 'A' && c <= 'Z'
		}
		if valid {
			return name[7:]
		}
	}
	return name
}
func (r *pdfReader) fontFamily(f *pdfFont, descriptor types.Dict) {
	accept := func(value, evidence string) bool {
		value = strings.TrimSpace(value)
		if value == "" || validateTextStyle(Run{FontFamily: value}) != nil {
			return false
		}
		f.family, f.familyEvidence = value, evidence
		return true
	}
	if value := descriptor["FontFamily"]; value != nil {
		obj, err := r.pdf.Dereference(value)
		if err == nil {
			if text, err := pdfTextString(obj); err == nil && accept(text, "descriptor") {
				return
			}
		}
		r.doc.warn("pdf_font_family", "Invalid font-family metadata is ignored while text and font identity are retained")
	}
	for _, key := range []string{"FontFile2", "FontFile3"} {
		if descriptor[key] == nil {
			continue
		}
		stream, _, err := r.pdf.DereferenceStreamDict(descriptor[key])
		if err != nil || stream == nil {
			r.doc.warn("pdf_font_family", "Embedded font metadata is unavailable; a face-name fallback may be needed")
			continue
		}
		data, err := boundedFontMetadata(r.ctx, stream)
		if err == nil {
			var font *sfnt.Font
			font, err = sfnt.Parse(data)
			if err == nil {
				var buffer sfnt.Buffer
				for _, id := range []sfnt.NameID{16, 1} {
					name, e := font.Name(&buffer, id)
					if e == nil && accept(name, "embedded_name") {
						return
					}
				}
			}
		}
		r.doc.warn("pdf_font_family", "Embedded font family could not be recovered within supported metadata bounds; text is retained")
	}
	name := pdfSubsetName(f.name)
	family := ""
	switch name {
	case "Helvetica", "Helvetica-Bold", "Helvetica-Oblique", "Helvetica-BoldOblique":
		family = "Helvetica"
	case "Times-Roman", "Times-Bold", "Times-Italic", "Times-BoldItalic":
		family = "Times"
	case "Courier", "Courier-Bold", "Courier-Oblique", "Courier-BoldOblique":
		family = "Courier"
	case "Symbol", "ZapfDingbats":
		family = name
	}
	if family != "" && accept(family, "core_font") {
		return
	}
	if name != "unknown" && accept(name, "postscript_fallback") {
		r.doc.warn("pdf_font_family_inferred", "A PostScript face name is used as a family lookup identifier; its family grouping is uncertain")
		return
	}
	f.familyEvidence = "unresolved"
	r.doc.warn("pdf_font_family", "Font family is unresolved; font identity and content remain in source evidence")
}

const maxFontMetadataBytes = 32 << 20

func boundedFontMetadata(ctx context.Context, s *types.StreamDict) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s.Content != nil {
		if len(s.Content) > maxFontMetadataBytes {
			return nil, fmt.Errorf("font metadata exceeds bound")
		}
		return s.Content, nil
	}
	if len(s.FilterPipeline) == 0 {
		if len(s.Raw) > maxFontMetadataBytes {
			return nil, fmt.Errorf("font metadata exceeds bound")
		}
		return s.Raw, nil
	}
	if len(s.FilterPipeline) != 1 || s.FilterPipeline[0].Name != "FlateDecode" || len(s.FilterPipeline[0].DecodeParms) > 0 {
		return nil, fmt.Errorf("unsupported font metadata filter")
	}
	z, err := zlib.NewReader(bytes.NewReader(s.Raw))
	if err != nil {
		return nil, err
	}
	defer z.Close()
	data, err := io.ReadAll(io.LimitReader(contextReader{ctx, z}, maxFontMetadataBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxFontMetadataBytes {
		return nil, fmt.Errorf("font metadata exceeds bound")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return data, nil
}

func (r *pdfReader) textStyle(s *pdfTextState, source *SourceInfo) Run {
	run := Run{Bold: s.font.bold, Italic: s.font.italic, FontFamily: s.font.family, FontSize: source.FontSize}
	source.FontFamilyEvidence = s.font.familyEvidence
	space, values, unknown := s.fillSpace, s.fillValues, s.fillUnknown
	if s.renderMode == 1 || s.renderMode == 5 {
		space, values, unknown = s.strokeSpace, s.strokeValues, s.strokeUnknown
	}
	if s.renderMode != 0 {
		r.warnPage("pdf_text_paint", "Text render mode is retained as source evidence; stroke/clip/invisible appearance is not an editable text property")
	}
	if s.rulingEffects || s.rulingFillAlpha != 1 || s.rulingStrokeAlpha != 1 {
		r.warnPage("pdf_text_paint", "Base text paint is retained; opacity/blend/mask effects are not represented by opaque RGB")
	}
	if !unknown {
		count := map[string]int{"DeviceGray": 1, "DeviceRGB": 3, "DeviceCMYK": 4}[space]
		if count > 0 {
			source.ColorSpace = space
			source.ColorComponents = append([]float64(nil), values[:count]...)
			rgb := colorRGB(space, values)
			run.Color = fmt.Sprintf("#%02X%02X%02X", int(math.Round(rgb[0]*255)), int(math.Round(rgb[1]*255)), int(math.Round(rgb[2]*255)))
			if space == "DeviceCMYK" {
				r.warnPage("pdf_text_color_approximation", "DeviceCMYK text uses an approximate RGB conversion; original components remain in source evidence")
			}
			if space != "DeviceCMYK" {
				for _, v := range rgb {
					if math.Abs(v*255-math.Round(v*255)) > 1e-7 {
						r.warnPage("pdf_text_color_quantized", "Text color is quantized to 8-bit RGB; original device components remain in source evidence")
						break
					}
				}
			}
		}
	} else {
		r.warnPage("pdf_text_color", "Unresolved text color is omitted from editable style; it must not be treated as a recovered black color")
	}
	if source.Matrix != nil {
		m := *source.Matrix
		x, y := math.Hypot(m[0], m[1]), math.Hypot(m[2], m[3])
		if math.Abs(x-y) > 1e-6*math.Max(1, y) || math.Abs(m[0]*m[2]+m[1]*m[3]) > 1e-6*math.Max(1, x*y) {
			r.warnPage("pdf_text_transform", "Point size retains vertical scale; nonuniform scaling/skew remains in source geometry and is not editable font geometry")
		}
	}
	return run
}
func colorRGB(space string, v [4]float64) [3]float64 {
	switch space {
	case "DeviceGray":
		return [3]float64{v[0], v[0], v[0]}
	case "DeviceCMYK":
		return [3]float64{1 - math.Min(1, v[0]+v[3]), 1 - math.Min(1, v[1]+v[3]), 1 - math.Min(1, v[2]+v[3])}
	}
	return [3]float64{v[0], v[1], v[2]}
}

func commonTextStyle(a, b Run) (Run, bool) {
	linkMixed := a.Destination != b.Destination || (a.Destination == "" && a.Link != b.Link)
	mixed := a.Bold != b.Bold || a.Italic != b.Italic || a.Underline != b.Underline || a.Strike != b.Strike || linkMixed || a.FontFamily != b.FontFamily || math.Abs(a.FontSize-b.FontSize) > 1e-7 || a.Color != b.Color
	a.Bold = a.Bold && b.Bold
	a.Italic = a.Italic && b.Italic
	a.Underline = a.Underline && b.Underline
	a.Strike = a.Strike && b.Strike
	if linkMixed {
		a.Link = ""
		a.Destination = ""
	}
	if a.FontFamily != b.FontFamily {
		a.FontFamily = ""
	}
	if math.Abs(a.FontSize-b.FontSize) > 1e-7 {
		a.FontSize = 0
	}
	if a.Color != b.Color {
		a.Color = ""
	}
	return a, mixed
}
func sameSourcePaint(a, b *SourceInfo) bool {
	if a == nil || b == nil || a.ColorSpace != b.ColorSpace || len(a.ColorComponents) != len(b.ColorComponents) {
		return false
	}
	for i, v := range a.ColorComponents {
		if math.Abs(v-b.ColorComponents[i]) > 1e-9 {
			return false
		}
	}
	return true
}

func (r *pdfReader) deviceColorSpace(value types.Object, resources types.Dict) (string, error) {
	seen := map[string]bool{}
	for depth := 0; depth < 64; depth++ {
		if err := r.ctx.Err(); err != nil {
			return "", err
		}
		obj, err := r.pdf.Dereference(value)
		if err != nil {
			return "", err
		}
		name, ok := obj.(types.Name)
		if !ok {
			return "", nil
		}
		key := string(name)
		switch key {
		case "DeviceGray", "DeviceRGB", "DeviceCMYK":
			return key, nil
		case "Pattern":
			return "", nil
		}
		if seen[key] {
			return "", fmt.Errorf("cyclic PDF color space")
		}
		seen[key] = true
		spaces, err := r.pdf.DereferenceDict(resources["ColorSpace"])
		if err != nil {
			return "", err
		}
		value = spaces[key]
		if value == nil {
			return "", nil
		}
	}
	return "", fmt.Errorf("PDF color space nesting exceeds limit")
}
func (r *pdfReader) selectTextColorSpace(args []any, resources types.Dict, s *pdfTextState, stroke bool) error {
	if len(args) != 1 {
		return fmt.Errorf("PDF color space requires one operand")
	}
	if _, ok := args[0].(types.Name); !ok {
		return fmt.Errorf("invalid PDF color space name")
	}
	space, err := r.deviceColorSpace(args[0].(types.Name), resources)
	if err != nil {
		return err
	}
	values := [4]float64{}
	if space == "DeviceCMYK" {
		values[3] = 1
	}
	if stroke {
		s.strokeSpace, s.strokeValues, s.strokeUnknown = space, values, space == ""
	} else {
		s.fillSpace, s.fillValues, s.fillUnknown = space, values, space == ""
		s.fill = colorRGB(space, values)
	}
	if space == "" {
		r.warnPage("pdf_color_state", "General color space is unresolved; editable text color is omitted and color-dependent stencils may reject")
	}
	return nil
}
func setTextColorValues(args []any, s *pdfTextState, stroke bool) error {
	space := s.fillSpace
	if stroke {
		space = s.strokeSpace
	}
	count := map[string]int{"DeviceGray": 1, "DeviceRGB": 3, "DeviceCMYK": 4}[space]
	if count == 0 {
		return nil
	}
	if len(args) != count {
		return fmt.Errorf("wrong device color operand count")
	}
	values := [4]float64{}
	for i, v := range args {
		n, err := pdfNumber(v)
		if err != nil || !finite(n) {
			return fmt.Errorf("invalid device color component")
		}
		values[i] = math.Max(0, math.Min(1, n))
	}
	if stroke {
		s.strokeValues = values
		s.strokeUnknown = false
	} else {
		s.fillValues = values
		s.fillUnknown = false
		s.fill = colorRGB(space, values)
	}
	return nil
}
