package manydoc

import (
	"encoding/hex"
	"fmt"
	"math"
	"strings"
	"unicode"
	"unicode/utf16"

	"github.com/rivo/uniseg"
	"golang.org/x/image/font"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
	"golang.org/x/text/unicode/norm"
)

// Unicode-defined advances use the current em. Thin/hair/narrow spaces use the
// declared 1/5, 1/10 and 1/5 em defaults; figure/punctuation spaces use font metrics.
func pdfSpecial(ch rune) bool {
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
	return a.ComplexScript == b.ComplexScript && a.Language == b.Language && a.EastAsiaLanguage == b.EastAsiaLanguage && a.ComplexLanguage == b.ComplexLanguage && a.Direction == b.Direction && a.Bold == b.Bold && a.Italic == b.Italic && a.Underline == b.Underline && a.Strike == b.Strike && a.Link == b.Link && a.Note == nil && b.Note == nil && a.FontFamily == b.FontFamily && a.FontSize == b.FontSize && canonicalTextColor(a.Color) == canonicalTextColor(b.Color)
}
func appendPDFSpan(line pdfLine, span pdfSpan) pdfLine {
	zeroControl := span.width == 0 && span.text != "\u00ad" && !pdfHardBreak(span.text) && !pdfTrimSpace(span.text)
	if span.glyphs == nil && len(line) > 0 && line[len(line)-1].glyphs == nil && span.text != "\u00ad" && line[len(line)-1].text != "\u00ad" && (span.width > 0 || zeroControl) && line[len(line)-1].width > 0 && samePDFStyle(line[len(line)-1].run, span.run) {
		last := &line[len(line)-1]
		last.text += span.text
		last.width += span.width
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

// Segment the whole paragraph, not each formatting run. A grapheme and a word
// can cross run boundaries. Note markers form an atom with the preceding cluster.
func (r *pdfRenderer) textAtoms(runs []Run, size float64) ([]pdfTextAtom, error) {
	var text strings.Builder
	var ranges []pdfRunRange
	for _, run := range runs {
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
	var atoms []pdfTextAtom
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
		if full := textSize(run, size); full > 14400 || fmt.Sprintf("%.2f", full) == "0.00" || math.Abs(full-math.Round(full*100)/100) > 1e-9 {
			return nil, fmt.Errorf("%w: PDF font size exceeds coordinate range or 0.01pt precision", ErrUnsupported)
		}
		drawSize := textSize(run, size)
		if run.Note != nil {
			if r.notes == nil {
				return nil, fmt.Errorf("missing note catalog")
			}
			label, ok := r.notes.labels[referenceKey(run.Note)]
			if !ok {
				return nil, fmt.Errorf("missing note reference label")
			}
			cluster = label
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
			glyphs, err = r.glyphClusterLanguage(face, cluster, run)
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
		atom.spans = pdfLine{{run: run, text: cluster, width: atom.width, ascent: math.Max(fullSize, float64(metrics.Ascent)/64+rise), descent: float64(metrics.Descent)/64 - rise, glyphs: glyphs}}
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
		if run.Note != nil && len(atoms) > 0 && !atoms[len(atoms)-1].hard {
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
	if err := r.measureShapedAtoms(atoms, size); err != nil {
		return nil, err
	}
	return atoms, nil
}

func (r *pdfRenderer) lines(runs []Run, width, size float64) ([]pdfLine, error) {
	if err := r.ctx.Err(); err != nil {
		return nil, err
	}
	if !finite(width) || width <= 0 || !finite(size) || size <= 0 {
		return nil, fmt.Errorf("invalid PDF text dimensions")
	}
	atoms, err := r.textAtoms(runs, size)
	if err != nil {
		return nil, err
	}
	if len(atoms) == 0 {
		return []pdfLine{{}}, nil
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
				line = appendPDFSpan(line, span)
				used += span.width
			}
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
	return lines, nil
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
	for _, span := range line {
		drawSize, rise := textSize(span.run, size), 0.0
		if span.run.Note != nil {
			drawSize = math.Round(drawSize*.7*100) / 100
			rise = textSize(span.run, size) * .3
		}
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
		if span.run.Note != nil {
			r.pdf.Link(x, y+baseline-span.ascent, span.width, math.Max(textSize(span.run, size), span.ascent+span.descent), r.noteLinks[referenceKey(span.run.Note)])
		}
		if span.run.Link != "" && span.width > 0 {
			r.pdf.LinkString(x, y+baseline-span.ascent, span.width, math.Max(textSize(span.run, size)*1.3, span.ascent+span.descent), span.run.Link)
		}

		x += span.width
	}
	r.drawLineDecorations(line, origin, y, baseline, size)
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
	size := fallback
	if len(line) > 0 {
		size = 0
	}
	for _, span := range line {
		size = math.Max(size, math.Max(textSize(span.run, fallback), span.ascent))
	}
	return size
}
func pdfLineHeight(line pdfLine, fallback, factor float64) float64 {
	below := fallback * (factor - 1)
	if len(line) > 0 {
		below = 0
	}
	for _, span := range line {
		below = math.Max(below, math.Max(textSize(span.run, fallback)*(factor-1), span.descent))
	}
	return pdfLineSize(line, fallback) + below
}
