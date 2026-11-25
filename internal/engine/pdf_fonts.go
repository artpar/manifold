package engine

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"github.com/benoitkugler/textlayout/fonts/simpleencodings"
	"github.com/benoitkugler/textlayout/fonts/truetype"
	"github.com/benoitkugler/textlayout/harfbuzz"

	// Parse source assignments once. Applying them from the end can skip CIDs already
	// owned by a later assignment, preserving existing last-assignment semantics
	// without expanding every overlapping range repeatedly.
	"github.com/benoitkugler/textlayout/language"
	pdffont "github.com/pdfcpu/pdfcpu/pkg/font"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
	"github.com/rivo/uniseg"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/gomonobold"
	"golang.org/x/image/font/gofont/gomonobolditalic"
	"golang.org/x/image/font/gofont/gomonoitalic"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
	"math"
	"sort"
	"strings"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"
)

type pdfCIDWidthAssignment struct {
	first, last uint32
	width       float64
	values      []float64
}

func (r *pdfReader) readCIDWidths(f *pdfFont, widths types.Array) error {
	target := f.widths
	var assignments []pdfCIDWidthAssignment
	arrays := map[types.IndirectRef][]float64{}
	expanded, maxCode := 0, uint32(0)
	for i := 0; i < len(widths); {
		if err := r.ctx.Err(); err != nil {
			return err
		}
		start, err := r.pdf.DereferenceInteger(widths[i])
		if err != nil || start == nil || *start < 0 || *start > 65535 {
			return fmt.Errorf("invalid CID width start")
		}
		i++
		if i >= len(widths) {
			return fmt.Errorf("incomplete CID widths")
		}
		source := widths[i]
		value, err := r.pdf.Dereference(source)
		if err != nil {
			return err
		}
		i++
		a := pdfCIDWidthAssignment{first: uint32(*start)}
		if list, ok := value.(types.Array); ok {
			if int(*start)+len(list) > 65536 {
				return fmt.Errorf("CID widths outside range")
			}
			ref, indirect := source.(types.IndirectRef)
			values, cached := arrays[ref]
			if !indirect || !cached {
				values = make([]float64, len(list))
				for j, w := range list {
					if j%1024 == 0 {
						if err := r.ctx.Err(); err != nil {
							return err
						}
					}
					values[j], err = r.pdf.DereferenceNumber(w)
					if err != nil {
						return err
					}
					if !finite(values[j]) {
						return fmt.Errorf("nonfinite glyph width")
					}
				}
				if indirect {
					arrays[ref] = values
				}
			}
			if len(values) == 0 {
				continue
			}
			a.values = values
			a.last = a.first + uint32(len(values)) - 1
		} else {
			end, ok := value.(types.Integer)
			if !ok || int(end) < int(*start) || int(end) > 65535 || i >= len(widths) {
				return fmt.Errorf("invalid CID width range")
			}
			a.width, err = r.pdf.DereferenceNumber(widths[i])
			if err != nil {
				return err
			}
			i++
			if !finite(a.width) {
				return fmt.Errorf("nonfinite glyph width")
			}
			a.last = uint32(end)
		}
		assignments = append(assignments, a)
		maxCode = max(maxCode, a.last)
		// Saturation only selects the bounded-application strategy; it never rejects.
		expanded = min(65537, expanded+int(a.last-a.first)+1)
	}
	f.cidWidthOverlap = expanded > 65536
	write := func(a pdfCIDWidthAssignment, code uint32) {
		if _, exists := target[code]; exists {
			f.cidWidthOverlap = true
		}
		width := a.width
		if a.values != nil {
			width = a.values[code-a.first]
		}
		target[code] = width
	}
	if expanded <= 65536 {
		for _, a := range assignments {
			if err := r.ctx.Err(); err != nil {
				return err
			}
			for code := a.first; code <= a.last; code++ {
				if code%1024 == 0 {
					if err := r.ctx.Err(); err != nil {
						return err
					}
				}
				write(a, code)
			}
		}
		return nil
	}
	// Successor links skip codes already assigned by later source entries. At most
	// 65536 codes are materialized, plus linear parsing of the actual source.

	// FontFace supplies one embeddable TTF program under a lookup family name.
	// Bold/Italic select its request slot; the supplied outlines determine appearance.
	// The renderer does not synthesize missing variants.

	// retain established default output metrics/resources

	// unit renderers using the original built-in font setup

	next := make([]uint32, int(maxCode)+2)
	for i := range next {
		next[i] = uint32(i)
	}
	find := func(code uint32) uint32 {
		root := code
		for next[root] != root {
			root = next[root]
		}
		for next[code] != code {
			parent := next[code]
			next[code] = root
			code = parent
		}
		return root
	}
	for i := len(assignments) - 1; i >= 0; i-- {
		if err := r.ctx.Err(); err != nil {
			return err
		}
		a := assignments[i]
		for code := find(a.first); code <= a.last; code = find(code) {
			if code%1024 == 0 {
				if err := r.ctx.Err(); err != nil {
					return err
				}
			}
			write(a, code)
			next[code] = find(

				// Register only used faces; the catalog still validates every supplied program.

				code + 1)
		}
	}
	return nil
}

type FontFace struct {
	Family       string
	Bold, Italic bool
	Data         []byte
}
type pdfFontFace struct {
	key        string
	id         string
	parsed     *sfnt.Font
	style      string
	data       []byte
	registered bool
}

var pdfMonoFonts = []struct {
	style string
	data  []byte
}{{"", gomono.TTF}, {"B", gomonobold.TTF}, {"I", gomonoitalic.TTF}, {"BI", gomonobolditalic.TTF}}

func fontKey(family, style string) string {
	if family == "" {
		family = "Go"
	}
	return fontFamilyKey(family) + "/" + style
}
func (r *pdfRenderer) addFonts(extra []FontFace) error {
	r.fonts = map[string]pdfFontFace{}
	add := func(family, style string, data []byte, check bool) error {
		data = append([]byte(nil), data...)
		if err := r.ctx.Err(); err != nil {
			return err
		}
		key := fontKey(family, style)
		if _, ok := r.fonts[key]; ok {
			return fmt.Errorf("duplicate PDF font face %q %s", family, style)
		}
		parsed, err := sfnt.Parse(data)
		if err != nil {
			return fmt.Errorf("font %q: %w", family, err)
		}
		if check {
			if err := validatePDFTrueType(r.ctx, data, parsed); err != nil {
				return err
			}
		}
		id := fmt.Sprintf("manydoc%d", len(r.fonts))
		if family == "Go" {
			id = "Go"
		}
		r.fonts[key] = pdfFontFace{key: key, id: id, parsed: parsed, style: style, data: data}
		return nil
	}
	for _, f := range pdfFonts {
		if err := add("Go", f.style, f.data, false); err != nil {
			return err
		}
	}
	for _, f := range pdfMonoFonts {
		if err := add("Go Mono", f.style, f.data, false); err != nil {
			return err
		}
	}
	for _, f := range extra {
		if f.Family == "" {
			return fmt.Errorf("custom font requires family")
		}
		if err := validateTextStyle(Run{FontFamily: f.Family}); err != nil {
			return err
		}
		if err := add(f.Family, runStyle(Run{Bold: f.Bold, Italic: f.Italic}), f.Data, true); err != nil {
			return err
		}
	}
	return nil
}
func (r *pdfRenderer) resolveFont(run Run) (pdfFontFace, error) {
	seen := map[string]bool{}
	for {
		family := run.FontFamily
		if family == "" {
			family = "Go"
		}
		key := fontFamilyKey(family)
		target, ok := r.fontMap[key]
		if !ok {
			break
		}
		if seen[key] {
			return pdfFontFace{}, fmt.Errorf("cyclic PDF font mapping")
		}
		seen[key] = true
		run.FontFamily = target
	}
	if r.fonts == nil {
		if run.FontFamily != "" && run.FontFamily != "Go" {
			return pdfFontFace{}, fmt.Errorf("%w: unavailable PDF font %q", ErrUnsupported, run.FontFamily)
		}
		if err := validatePDFText(Run{}); err != nil {
			return pdfFontFace{}, err
		}
		for _, f := range pdfFonts {
			if f.style == runStyle(run) {
				return pdfFontFace{id: "Go", style: f.style, parsed: parsedPDFFonts[f.style], data: f.data}, nil
			}
		}
		return pdfFontFace{}, fmt.Errorf("missing default font")
	}
	face, ok := r.fonts[fontKey(run.FontFamily, runStyle(run))]
	if !ok {
		return face, fmt.Errorf("%w: unavailable PDF font family/style %q %s", ErrUnsupported, run.FontFamily, runStyle(run))
	}
	return face, nil
}

func (r *pdfRenderer) font(run Run) (pdfFontFace, error) {
	face, err := r.resolveFont(run)
	if err != nil {
		return face, err
	}
	if r.fonts == nil {
		return face, nil
	}
	return r.registerFont(face.key)
}

func (r *pdfRenderer) registerFont(key string) (pdfFontFace, error) {
	face := r.fonts[key]
	if face.registered {
		return face, nil
	}
	if err := r.ctx.Err(); err != nil {
		return face, err
	}
	r.pdf.AddUTF8FontFromBytes(face.id, face.style, append([]byte(nil), face.data...))
	if err := r.ctx.Err(); err != nil {
		return face, err
	}
	if err := r.pdf.Error(); err != nil {
		return face, err
	}
	face.registered = true
	r.fonts[key] = face
	return face, nil
}

func fontFamilyKey(family string) string {
	if family == "" || family == "Go" {
		return "go"
	}
	if family == "Go Mono" {
		return "go mono"
	}
	return cases.Fold().String(norm.NFC.String(family))
}
func (f *pdfFont) codeChunks(b []byte) ([]uint32, []int, error) {
	var codes []uint32
	var sizes []int
	for len(b) > 0 {
		size := 1
		if f.unicode || f.composite {
			size = 2
		}
		if f.mapping != nil && len(f.mapping.spaces) > 0 {
			size = 0
			for _, s := range f.mapping.spaces {
				if len(b) >= s.size {
					v := codeNumber(b[:s.size])
					if v >= s.low && v <= s.high {
						size = s.size
						break
					}
				}
			}
			if size == 0 {
				return nil, nil, fmt.Errorf("font code outside declared code spaces")
			}
		}
		if len(b) < size {
			return nil, nil, fmt.Errorf("truncated PDF font code")
		}
		codes = append(codes, codeNumber(b[:size]))
		sizes = append(sizes, size)
		b = b[size:]
	}
	return codes, sizes, nil
}
func (f *pdfFont) displacement(b []byte, size, charSpace, wordSpace, scale float64) (float64, bool, error) {
	codes, sizes, err := f.codeChunks(b)
	if err != nil {
		return 0, false, err
	}
	advance := 0.0
	estimated := f.cidWidthOverlap
	for i, code := range codes {
		width, ok := f.widths[code]
		if f.composite && !f.identityCID {
			ok = false
		}
		if !ok {
			width = f.defaultWidth
			if !f.defaultExact {
				estimated = true
			}
		}
		if f.composite && !f.identityCID {
			estimated = true
		}
		advance += (width*size/1000 + charSpace) * scale
		if sizes[i] == 1 && code == 32 {
			advance += wordSpace * scale
		}
	}
	return advance, estimated, nil
}
func (r *pdfReader) fontMetrics(f *pdfFont, d types.Dict) error {
	f.defaultWidth = 500
	f.ascent = 800
	f.descent = -200
	f.boundsEstimate = true
	descriptor, err := r.pdf.DereferenceDict(d["FontDescriptor"])
	if err != nil {
		return err
	}
	if sub := d.NameEntry("Subtype"); sub != nil && *sub == "Type0" {
		f.composite = true
		f.defaultWidth = 1000
		f.defaultExact = true
		if enc := d.NameEntry("Encoding"); enc != nil {
			f.identityCID = *enc == "Identity-H"
			f.vertical = *enc == "Identity-V" || strings.HasSuffix(*enc, "-V")
		}
		descendants, err := r.pdf.DereferenceArray(d["DescendantFonts"])
		if err != nil {
			return err
		}
		if len(descendants) != 1 {
			return fmt.Errorf("composite font requires one descendant")
		}
		cid, err := r.pdf.DereferenceDict(descendants[0])
		if err != nil {
			return err
		}
		descriptor, err = r.pdf.DereferenceDict(cid["FontDescriptor"])
		if err != nil {
			return err
		}
		if dw := cid["DW"]; dw != nil {
			value, err := r.pdf.DereferenceNumber(dw)
			if err != nil {
				return err
			}
			f.defaultWidth = value
		}
		widths, err := r.pdf.DereferenceArray(cid["W"])
		if err != nil {
			return err
		}
		if err := r.readCIDWidths(f, widths); err != nil {
			return err
		}
		if f.cidWidthOverlap {
			r.warnPage("pdf_cid_width_overlap", "Overlapping CID width declarations use the last source assignment; advance geometry may differ between readers")
		}
	}
	if descriptor != nil {
		if value := descriptor["Flags"]; value != nil {
			flags, err := r.pdf.DereferenceInteger(value)
			if err != nil || flags == nil || *flags < 0 {
				return fmt.Errorf("invalid PDF font flags")
			}
			f.bold = f.bold || int64(*flags)&(1<<18) != 0
			f.italic = f.italic || int64(*flags)&(1<<6) != 0
		}
		if value := descriptor["ItalicAngle"]; value != nil {
			angle, err := r.pdf.DereferenceNumber(value)
			if err != nil || !finite(angle) {
				return fmt.Errorf("invalid PDF font italic angle")
			}
			f.italic = f.italic || angle != 0
		}
		if value := descriptor["FontWeight"]; value != nil {
			weight, err := r.pdf.DereferenceNumber(value)
			if err != nil || !finite(weight) {
				return fmt.Errorf("invalid PDF font weight")
			}
			f.bold = f.bold || weight >= 700
		}
		if w := descriptor["MissingWidth"]; w != nil && !f.composite {
			value, err := r.pdf.DereferenceNumber(w)
			if err != nil {
				return err
			}
			f.defaultWidth = value
			f.defaultExact = true
		}
		a, da := descriptor["Ascent"], descriptor["Descent"]
		if a != nil && da != nil {
			f.ascent, err = r.pdf.DereferenceNumber(a)
			if err != nil {
				return err
			}
			f.descent, err = r.pdf.DereferenceNumber(da)
			if err != nil {
				return err
			}
			f.boundsEstimate = false
		}
	}
	r.fontFamily(f, descriptor)
	core := f.name
	if len(core) > 7 && core[6] == '+' {
		core = core[7:]
	}
	if pdffont.IsCoreFont(core) {
		if f.boundsEstimate {
			box := pdffont.BoundingBox(core)
			f.ascent = box.UR.Y
			f.descent = box.LL.Y
			f.boundsEstimate = false
		}
		// The public metrics API expects the core font's encoded byte. Resolve glyph
		// names rather than applying WinAnsi widths to an arbitrary source encoding.
		encoding := simpleencodings.WinAnsi
		if core == "Symbol" {
			encoding = simpleencodings.Symbol
		}
		if core == "ZapfDingbats" {
			encoding = simpleencodings.ZapfDingbats
		}
		inverse := map[string]int{}
		for code, name := range encoding {
			if name != "" {
				inverse[name] = code
			}
		}
		if core != "Symbol" && core != "ZapfDingbats" {
			// The reusable encoding includes Poppler aliases at NBSP/SHY and
			// undefined CP1252 bytes. pdfcpu's metrics lookup uses canonical
			// CP1252 codes, where those alias slots have no metrics. Resolve
			// duplicate glyph names to the actual space/hyphen/bullet slots.
			inverse["space"], inverse["hyphen"], inverse["bullet"] = 32, 45, 149
		}
		for code, name := range f.encoding {
			if _, ok := f.widths[uint32(code)]; ok {
				continue
			}
			if c, ok := inverse[name]; ok {
				f.widths[uint32(code)] = float64(pdffont.CharWidth(core, rune(c)))
			}
		}
	}
	if !finite(f.ascent) || !finite(f.descent) || f.ascent < f.descent || !finite(f.defaultWidth) {
		return fmt.Errorf("invalid font metrics")
	}
	for _, w := range f.widths {
		if !finite(w) {
			return fmt.Errorf("nonfinite glyph width")
		}
	}
	return nil
}

// Simple-font character codes are one byte (ISO 32000-1, 9.6). Some InDesign
// exports contradict that width through two-byte codespaces or zero-padded
// keys. Unambiguous numeric byte mappings win, with a repair diagnostic.

// original UTF-8 interval for the shaped cluster

// em units, y offsets point upwards
// fragment start relative to the shared shaping origin

// em when widthOverride is true

// Residual marks need glyph positioning even when their grapheme starts with
// an explicit spacing character. Keep the pre-existing marked-space path.

// Explicit spacing/break controls retain their declared layout policy. Every
// ordinary text cluster is shaped, whether or not it carries a language binding.

func (r *pdfReader) normalizeSimpleCMap(f *pdfFont) error {
	if f.mapping == nil || f.composite || f.unicode {
		return nil
	}
	repaired := false
	chars := map[string]string{}
	for key, text := range f.mapping.chars {
		code := codeNumber([]byte(key))
		if code > 255 {
			return fmt.Errorf("%w: ToUnicode key exceeds simple-font code range", ErrUnsupported)
		}
		canonical := string([]byte{byte(code)})
		if previous, exists := chars[canonical]; exists && previous != text {
			return fmt.Errorf("ambiguous padded simple-font ToUnicode keys")
		}
		chars[canonical] = text
		if len(key) != 1 {
			repaired = true
		}
	}
	for i, key := range f.mapping.ranges {

		// ToUnicode maps each shaped cluster, including ligatures, to its own
		// original text. ActualText can then wrap a whole painted run without
		// assigning that entire run to its first glyph in fallback readers.

		if key.high > 255 {
			return fmt.Errorf("%w: ToUnicode range exceeds simple-font code range", ErrUnsupported)
		}
		if key.size != 1 {
			f.mapping.ranges[i].size = 1
			repaired = true
		}
	}
	f.mapping.chars = chars
	for _, space := range f.mapping.spaces {
		if space.size != 1 {
			f.mapping.spaces = []codeRange{{size: 1, low: 0, high: 255}}
			repaired = true
			break
		}
	}
	if repaired {
		r.warnPage("pdf_cmap_codespace", "Simple-font ToUnicode codespace/key padding contradicts single-byte font codes; unambiguous numeric byte mappings are used")

		// HarfBuzz returns monotonically descending cluster indices for RTL.
		// Assign original logical intervals in ascending input order, independently
		// of visual glyph order. Keep repeated glyphs in each cluster together.

	}
	return nil
}

type pdfGlyphPosition struct {
	glyph              uint16
	cluster            int
	logical            string
	textStart, textEnd int
	x, y, advance      float64
}
type pdfPaintActualGroup struct{ text string }
type pdfGlyphCluster struct {
	rtl                    bool
	level                  uint8
	actualGroup            *pdfPaintActualGroup
	script                 language.Script
	face                   *pdfGlyphFace
	positions              []pdfGlyphPosition
	width, ascent, descent float64
	origin                 float64
	clipLeft, clipRight    bool
	logicalCarrier         bool
}
type pdfGlyphCode struct {
	glyph         uint16
	text          string
	metricWidth   float64
	widthOverride bool
}
type pdfGlyphFace struct {
	name     string
	original pdfFontFace
	shaper   *harfbuzz.Font
	codes    map[pdfGlyphCode]uint16
	entries  []pdfGlyphCode
}

func needsGlyphOutput(text string) bool {
	for _, ch := range norm.NFC.String(text) {
		if ch > 0xFFFF || unicode.Is(unicode.Mn, ch) || unicode.Is(unicode.Mc, ch) || unicode.Is(unicode.Me, ch) {
			return true
		}
	}
	return false
}

func pdfShapableText(text string) bool {
	if text == "" {
		return false
	}
	for _, ch := range text {
		if pdfSpecial(ch) {
			return false
		}
	}
	return true
}
func pdfGlyphLocale(run Run, script language.Script) language.Language {
	locale := run.Language
	complex := run.ComplexScript || run.Direction == "rtl"
	if complex && run.ComplexLanguage != "" {
		locale = run.ComplexLanguage
	}
	switch script {
	case language.Han, language.Hiragana, language.Katakana, language.Hangul, language.Bopomofo, language.Yi:
		if !complex && run.EastAsiaLanguage != "" {
			locale = run.EastAsiaLanguage
		}
	}
	if locale == "" {
		locale = "und"
	}
	return language.NewLanguage(locale)
}

func (r *pdfRenderer) glyphCluster(face pdfFontFace, text string) (*pdfGlyphCluster, error) {
	return r.glyphClusterLanguage(face, text, Run{})
}
func (r *pdfRenderer) glyphClusterLanguage(face pdfFontFace, text string, run Run) (*pdfGlyphCluster, error) {
	return r.shapeGlyphText(face, text, run, 1024)
}
func (r *pdfRenderer) shapeGlyphText(face pdfFontFace, text string, run Run, limit int) (*pdfGlyphCluster, error) {
	return r.shapeGlyphTextScript(face, text, run, limit, 0)
}
func (r *pdfRenderer) shapeGlyphTextScript(face pdfFontFace, text string, run Run, limit int, script language.Script) (*pdfGlyphCluster, error) {
	return r.shapeGlyphTextLevel(face, text, run, limit, script, 0)
}
func (r *pdfRenderer) shapeGlyphTextLevel(face pdfFontFace, text string, run Run, limit int, script language.Script, level uint8) (*pdfGlyphCluster, error) {
	return r.shapeGlyphTextContext(face, text, run, limit, script, level, nil, 0)
}
func (r *pdfRenderer) shapeGlyphTextContext(face pdfFontFace, text string, run Run, limit int, script language.Script, level uint8, context []rune, offset int) (cluster *pdfGlyphCluster, err error) {
	defer func() {
		if p := recover(); p != nil {
			cluster = nil
			err = fmt.Errorf("%w: font shaping failed: %v", ErrUnsupported, p)
		}
	}()
	if err = r.ctx.Err(); err != nil {
		return nil, err
	}
	if utf8.RuneCountInString(text) > limit {
		return nil, fmt.Errorf("%w: glyph shaping segment exceeds %d scalar resource limit", ErrUnsupported, limit)
	}
	if r.glyphFaces == nil {
		r.glyphFaces = map[string]*pdfGlyphFace{}
	}
	key := face.id + "/" + face.style
	gf := r.glyphFaces[key]
	if gf == nil {
		parsed, e := truetype.Parse(bytes.NewReader(face.data))
		if e != nil {
			return nil, fmt.Errorf("%w: TrueType shaping tables: %v", ErrUnsupported, e)
		}
		gf = &pdfGlyphFace{name: fmt.Sprintf("MDG%d", len(r.glyphFaces)+1), original: face, shaper: harfbuzz.NewFont(parsed), codes: map[pdfGlyphCode]uint16{}}
		r.glyphFaces[key] = gf
	}
	b := harfbuzz.NewBuffer()
	b.Props.Language = "und"
	b.Flags = harfbuzz.DoNotinsertDottedCircle
	normalized := []rune(norm.NFC.String(text))
	if context == nil {
		context = normalized
		offset = 0
	}
	if offset < 0 || offset+len(normalized) > len(context) || string(context[offset:offset+len(normalized)]) != string(normalized) {
		return nil, fmt.Errorf("%w: shaping item/context mapping mismatch", ErrUnsupported)
	}
	b.AddRunes(context, offset, len(normalized))
	b.GuessSegmentProperties()
	if script != 0 {
		b.Props.Script = script
	}
	b.Props.Language = pdfGlyphLocale(run, b.Props.Script)
	b.Props.Direction = harfbuzz.LeftToRight
	if level%2 != 0 {
		b.Props.Direction = harfbuzz.RightToLeft
	}

	b.Shape(gf.shaper, nil)
	if len(b.Info) != len(b.Pos) || len(b.Info) == 0 {
		return nil, fmt.Errorf("%w: empty/invalid shaping result", ErrUnsupported)
	}
	upem := float64(face.parsed.UnitsPerEm())
	c := &pdfGlyphCluster{face: gf, script: b.Props.Script, rtl: level%2 != 0, level: level}
	var buf sfnt.Buffer
	x, y := 0.0, 0.0
	for i, info := range b.Info {
		if info.Glyph == 0 || int(info.Glyph) >= face.parsed.NumGlyphs() {
			return nil, fmt.Errorf("%w: shaper produced missing/invalid glyph", ErrUnsupported)
		}
		pos := b.Pos[i]
		if pos.YAdvance != 0 {
			return nil, fmt.Errorf("%w: vertical glyph advances", ErrUnsupported)
		}
		g := pdfGlyphPosition{glyph: uint16(info.Glyph), cluster: info.Cluster - offset, x: x + float64(pos.XOffset)/upem, y: y + float64(pos.YOffset)/upem, advance: float64(pos.XAdvance) / upem}
		bounds, _, e := face.parsed.GlyphBounds(&buf, sfnt.GlyphIndex(g.glyph), fixed.I(1000), font.HintingNone)
		if e != nil {
			return nil, e
		}
		c.ascent = math.Max(c.ascent, g.y-float64(bounds.Min.Y)/64000)
		c.descent = math.Max(c.descent, float64(bounds.Max.Y)/64000-g.y)
		c.positions = append(c.positions, g)
		x += g.advance
	}
	if !finite(x) || x < 0 {
		return nil, fmt.Errorf("%w: invalid horizontal glyph advance", ErrUnsupported)
	}

	var originalEnds []int
	graphemes := uniseg.NewGraphemes(text)
	for graphemes.Next() {
		_, end := graphemes.Positions()
		for range []rune(norm.NFC.String(graphemes.Str())) {
			originalEnds = append(originalEnds, end)
		}
	}

	starts := make([]int, 0)
	for i, g := range c.positions {
		if i > 0 {
			previous := c.positions[i-1].cluster
			if (!c.rtl && g.cluster < previous) || (c.rtl && g.cluster > previous) {
				return nil, fmt.Errorf("%w: nonmonotone shaping clusters", ErrUnsupported)
			}
		}
		if i == 0 || g.cluster != c.positions[i-1].cluster {
			starts = append(starts, g.cluster)
		}
	}
	sort.Ints(starts)
	intervals := make(map[int][2]int, len(starts))
	consumed := 0
	for i, from := range starts {
		to := len(originalEnds)
		if i+1 < len(starts) {
			to = starts[i+1]
		}
		if from < 0 || from >= to || to > len(originalEnds) {
			return nil, fmt.Errorf("%w: invalid horizontal shaping cluster order", ErrUnsupported)
		}
		end := max(consumed, originalEnds[to-1])
		intervals[from] = [2]int{consumed, end}
		consumed = end
	}
	for i := range c.positions {
		g := &c.positions[i]
		interval := intervals[g.cluster]
		g.textStart, g.textEnd = interval[0], interval[1]
		if i == 0 || c.positions[i-1].cluster != g.cluster {
			g.logical = text[g.textStart:g.textEnd]
		}
	}
	if consumed != len(text) {
		return nil, fmt.Errorf("%w: shaping omitted logical text", ErrUnsupported)
	}

	c.width = x
	if err = r.ctx.Err(); err != nil {
		return nil, err
	}
	return c, nil
}
func (f *pdfGlyphFace) cid(glyph uint16, text string) (uint16, error) {
	key := pdfGlyphCode{glyph: glyph, text: text}
	if code, ok := f.codes[key]; ok {
		return code, nil
	}
	if len(f.entries) >= 65535 {
		return 0, fmt.Errorf("%w: per-face CID inventory exceeds 65535", ErrUnsupported)
	}
	code := uint16(len(f.entries) + 1)
	f.codes[key] = code
	f.entries = append(f.entries, key)
	return code, nil
}
func (f *pdfGlyphFace) carrierCID(text string, width float64) (uint16, error) {
	var b sfnt.Buffer
	gid, err := f.original.parsed.GlyphIndex(&b, ' ')
	if err != nil || gid == 0 {
		return 0, fmt.Errorf("%w: logical glyph carrier needs a space glyph", ErrUnsupported)
	}
	if !finite(width) || width < 0 {
		return 0, fmt.Errorf("%w: invalid logical carrier advance", ErrUnsupported)
	}
	key := pdfGlyphCode{glyph: uint16(gid), text: text, metricWidth: width, widthOverride: true}
	if code, ok := f.codes[key]; ok {
		return code, nil
	}
	if len(f.entries) >= 65535 {
		return 0, fmt.Errorf("%w: per-face CID inventory exceeds 65535", ErrUnsupported)
	}
	code := uint16(len(f.entries) + 1)
	f.codes[key] = code
	f.entries = append(f.entries, key)
	return code, nil
}

func (r *pdfRenderer) paintGlyphSpan(span pdfSpan, x, y, size float64) {
	c := span.glyphs
	// Match the existing primitive writer's baseline precision. Keep GPOS
	// offsets precise relative to that baseline instead of manufacturing tiny
	// baseline differences between adjacent primitive and glyph spans.
	x, y = math.Round((x-c.origin*size)*100)/100+c.origin*size, math.Round(y*100)/100
	red, green, blue := textRGB(span.run.Color)
	r.pdf.RawWriteStr("q")
	if c.clipLeft || c.clipRight {
		left, right := 0.0, r.width
		if c.clipLeft {
			left = x
		}
		if c.clipRight {
			right = x + span.width
		}
		if right < left {
			r.pdf.SetError(fmt.Errorf("invalid glyph fragment clip"))
			return
		}
		r.pdf.RawWriteStr(fmt.Sprintf("%.8f 0 %.8f %.8f re W n", left, right-left, r.height))
	}
	if c.logicalCarrier {
		r.pdf.RawWriteStr("/Artifact << /ActualText <FEFF> >> BDC")
	} else if c.actualGroup == nil {
		r.pdf.RawWriteStr(pdfActualText(span.text))
	}
	r.pdf.RawWriteStr(fmt.Sprintf("%.8f %.8f %.8f rg", float64(red)/255, float64(green)/255, float64(blue)/255))
	for _, g := range c.positions {
		logical := g.logical
		if c.logicalCarrier {
			logical = ""
		}
		code, err := c.face.cid(g.glyph, logical)
		if err != nil {
			r.pdf.SetError(err)
			return
		}
		r.pdf.RawWriteStr(fmt.Sprintf("BT /%s %.8f Tf 1 0 0 1 %.8f %.8f Tm <%04X> Tj ET", c.face.name, size, x+g.x*size, r.height-y+g.y*size, code))
	}
	if c.actualGroup == nil || c.logicalCarrier {
		r.pdf.RawWriteStr("EMC")
	}
	r.pdf.RawWriteStr("Q")
	if c.logicalCarrier {
		code, err := c.face.carrierCID(span.text, c.width)
		if err != nil {
			r.pdf.SetError(err)
			return
		}
		r.pdf.RawWriteStr("q\n" + pdfActualText(span.text))
		r.pdf.RawWriteStr(fmt.Sprintf("%.8f %.8f %.8f rg", float64(red)/255, float64(green)/255, float64(blue)/255))
		r.pdf.RawWriteStr(fmt.Sprintf("BT /%s %.8f Tf 3 Tr 1 0 0 1 %.8f %.8f Tm <%04X> Tj ET", c.face.name, size, x, r.height-y, code))
		r.pdf.RawWriteStr("EMC\nQ")
	}
}
func glyphUTF16(text string) string {
	data := make([]byte, 0, len(text)*2)
	for _, v := range utf16.Encode([]rune(text)) {
		data = append(data, byte(v>>8), byte(v))
	}
	return strings.ToUpper(hex.EncodeToString(data))
}
func glyphLiteral(text string) types.HexLiteral { return types.HexLiteral("FEFF" + glyphUTF16(text)) }
func glyphStream(p *model.Context, data []byte) (*types.IndirectRef, error) {
	s, err := p.NewStreamDictForBuf(data)
	if err != nil {
		return nil, err
	}
	if err = s.Encode(); err != nil {
		return nil, err
	}
	return p.IndRefForNewObject(*s)
}
func (f *pdfGlyphFace) embed(ctx context.Context, p *model.Context, tag string) (*types.IndirectRef, error) {
	parsed := f.original.parsed
	var buf sfnt.Buffer
	name, err := parsed.Name(&buf, sfnt.NameIDPostScript)
	if err != nil || name == "" || len(name) > 127 || strings.IndexFunc(name, func(ch rune) bool { return ch < 33 || ch > 126 || strings.ContainsRune("[](){}<>/%", ch) }) >= 0 {
		return nil, fmt.Errorf("%w: glyph font needs a PostScript name", ErrUnsupported)
	}
	// Font names are encoded PDF names, not injected content tokens.
	family, _ := parsed.Name(&buf, sfnt.NameIDFamily)
	if preferred, e := parsed.Name(&buf, sfnt.NameID(16)); e == nil && preferred != "" {
		family = preferred
	}
	if err := validateTextStyle(Run{FontFamily: family}); err != nil {
		return nil, err
	}
	metrics, err := parsed.Metrics(&buf, fixed.I(1000), font.HintingNone)
	if err != nil {
		return nil, err
	}
	bounds, err := parsed.Bounds(&buf, fixed.I(1000), font.HintingNone)
	if err != nil {
		return nil, err
	}
	used := make([]uint16, len(f.entries))
	for i, e := range f.entries {
		used[i] = e.glyph
	}
	fontProgram, err := subsetGlyphTrueType(ctx, f.original.data, used)
	if err != nil {
		return nil, err
	}
	name = tag + "+" + name
	program, err := glyphStream(p, fontProgram)
	if err != nil {
		return nil, err
	}
	stream, _, err := p.DereferenceStreamDict(*program)
	if err != nil {
		return nil, err
	}
	stream.InsertInt("Length1", len(fontProgram))
	flags := 32
	subfamily, _ := parsed.Name(&buf, sfnt.NameIDSubfamily)
	lower := strings.ToLower(subfamily)
	italic := 0.0
	if post := parsed.PostTable(); post != nil {
		italic = post.ItalicAngle
		if post.IsFixedPitch {
			flags |= 1
		}
	}
	if italic != 0 || strings.Contains(lower, "italic") || strings.Contains(lower, "oblique") {
		flags |= 64
	}
	weight := 400
	if strings.Contains(lower, "bold") {
		weight = 700
	}
	desc := types.Dict{"Type": types.Name("FontDescriptor"), "FontName": types.Name(name), "FontFamily": glyphLiteral(family), "Flags": types.Integer(flags), "FontWeight": types.Integer(weight), "ItalicAngle": types.Float(italic), "Ascent": types.Float(float64(metrics.Ascent) / 64), "Descent": types.Float(-float64(metrics.Descent) / 64), "CapHeight": types.Float(float64(metrics.CapHeight) / 64), "StemV": types.Integer(80), "FontBBox": types.Array{types.Float(float64(bounds.Min.X) / 64), types.Float(-float64(bounds.Max.Y) / 64), types.Float(float64(bounds.Max.X) / 64), types.Float(-float64(bounds.Min.Y) / 64)}, "FontFile2": *program}
	descriptor, err := p.IndRefForNewObject(desc)
	if err != nil {
		return nil, err
	}
	mapping := make([]byte, 2*(len(f.entries)+1))
	widths := types.Array{}
	var cmap strings.Builder
	cmap.WriteString("/CIDInit /ProcSet findresource begin\n12 dict begin\nbegincmap\n/CIDSystemInfo << /Registry (Adobe) /Ordering (UCS) /Supplement 0 >> def\n/CMapName /ManydocGlyphUnicode def\n/CMapType 2 def\n1 begincodespacerange\n<0000> <FFFF>\nendcodespacerange\n")
	for start := 0; start < len(f.entries); start += 100 {
		end := min(start+100, len(f.entries))
		fmt.Fprintf(&cmap, "%d beginbfchar\n", end-start)
		for i := start; i < end; i++ {
			e := f.entries[i]
			code := i + 1
			binary.BigEndian.PutUint16(mapping[2*code:], e.glyph)
			width, err := parsed.GlyphAdvance(&buf, sfnt.GlyphIndex(e.glyph), fixed.I(1000), font.HintingNone)
			if err != nil {
				return nil, err
			}
			metric := float64(width) / 64
			if e.widthOverride {
				metric = e.metricWidth * 1000
			}
			widths = append(widths, types.Integer(code), types.Array{types.Float(metric)})
			fmt.Fprintf(&cmap, "<%04X> <%s>\n", code, glyphUTF16(e.text))
		}
		cmap.WriteString("endbfchar\n")
	}
	cmap.WriteString("endcmap\nCMapName currentdict /CMap defineresource pop\nend\nend\n")
	gidMap, err := glyphStream(p, mapping)
	if err != nil {
		return nil, err
	}
	unicodeMap, err := glyphStream(p, []byte(cmap.String()))
	if err != nil {
		return nil, err
	}
	cid := types.Dict{"Type": types.Name("Font"), "Subtype": types.Name("CIDFontType2"), "BaseFont": types.Name(name), "CIDSystemInfo": types.Dict{"Registry": types.StringLiteral("Adobe"), "Ordering": types.StringLiteral("Identity"), "Supplement": types.Integer(0)}, "FontDescriptor": *descriptor, "CIDToGIDMap": *gidMap, "DW": types.Integer(1000), "W": widths}
	descendant, err := p.IndRefForNewObject(cid)
	if err != nil {
		return nil, err
	}
	return p.IndRefForNewObject(types.Dict{"Type": types.Name("Font"), "Subtype": types.Name("Type0"), "BaseFont": types.Name(name), "Encoding": types.Name("Identity-H"), "DescendantFonts": types.Array{*descendant}, "ToUnicode": *unicodeMap})
}
func (r *pdfRenderer) finishGlyphFonts(input []byte) (output []byte, err error) {
	if r.outputStructure == nil && len(r.glyphFaces) == 0 && len(r.destinationPoints)+len(r.destinationTokens) == 0 {
		return input, nil
	}
	defer func() {
		if p := recover(); p != nil {
			output = nil
			err = fmt.Errorf("%w: glyph font serialization failed: %v", ErrUnsupported, p)
		}
	}()
	conf := &model.Configuration{Reader15: true, ValidationMode: model.ValidationRelaxed, Offline: true, Eol: "\n"}
	p, err := pdfcpu.ReadWithContext(r.ctx, bytes.NewReader(input), conf)
	if err != nil {
		return nil, err
	}
	if err = p.EnsurePageCount(); err != nil {
		return nil, err
	}
	fonts := types.Dict{}
	ordered := make([]*pdfGlyphFace, 0, len(r.glyphFaces))
	for _, face := range r.glyphFaces {
		ordered = append(ordered, face)
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].name < ordered[j].name })
	for index, face := range ordered {
		if err = r.ctx.Err(); err != nil {
			return nil, err
		}
		if len(face.entries) == 0 {
			continue
		}
		var tag [6]byte
		n := index
		for i := 5; i >= 0; i-- {
			tag[i] = byte('A' + n%26)
			n /= 26
		}
		if n != 0 {
			return nil, fmt.Errorf("%w: too many glyph font subsets", ErrUnsupported)
		}
		ref, e := face.embed(r.ctx, p, string(tag[:]))
		if e != nil {
			return nil, e
		}
		fonts[face.name] = *ref
	}
	for page := 1; page <= p.PageCount; page++ {
		if err = r.ctx.Err(); err != nil {
			return nil, err
		}
		dict, _, inherited, e := p.PageDict(page, false)
		if e != nil {
			return nil, e
		}
		resources := types.Dict{}
		for k, v := range inherited.Resources {
			resources[k] = v
		}
		existing, e := p.DereferenceDict(resources["Font"])
		if e != nil {
			return nil, e
		}
		merged := types.Dict{}
		for k, v := range existing {
			merged[k] = v
		}
		for k, v := range fonts {
			if _, collision := merged[k]; collision {
				return nil, fmt.Errorf("glyph font resource collision")
			}
			merged[k] = v
		}
		resources["Font"] = merged
		dict["Resources"] = resources
	}
	if err := r.finishOutputStructure(p); err != nil {
		return nil, err
	}
	if err := r.finishDestinations(p); err != nil {
		return nil, err
	}
	var b bytes.Buffer
	p.Write.Writer = bufio.NewWriter(&b)
	if err = pdfcpu.WriteContext(p); err != nil {
		return nil, err
	}
	if err = p.Write.Writer.Flush(); err != nil {
		return nil, err
	}
	if err = r.ctx.Err(); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}
