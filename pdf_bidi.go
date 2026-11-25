package manifold

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/artpar/manifold/internal/bidilevels"
	"golang.org/x/text/unicode/bidi"
)

// Source positions count original scalars, before NFC, note-label substitution
// or glyph clustering. A hard paragraph separator belongs to its preceding
// paragraph, but is excluded from that paragraph's bidi input (P1). CRLF is one
// boundary. U+2028 is a line boundary and retains its paragraph's context.
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
		lang := strings.ToLower(strings.SplitN(r.run.Language, "-", 2)[0])
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
