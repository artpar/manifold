package manydoc

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"
	"sort"
	"strings"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/benoitkugler/textlayout/fonts/truetype"
	"github.com/benoitkugler/textlayout/harfbuzz"
	"github.com/benoitkugler/textlayout/language"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
	"github.com/rivo/uniseg"
	"golang.org/x/image/font"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
	"golang.org/x/text/unicode/norm"
)

type pdfGlyphPosition struct {
	glyph              uint16
	cluster            int
	logical            string
	textStart, textEnd int // original UTF-8 interval for the shaped cluster
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
	width, ascent, descent float64 // em units, y offsets point upwards
	origin                 float64 // fragment start relative to the shared shaping origin
	clipLeft, clipRight    bool
	logicalCarrier         bool
}
type pdfGlyphCode struct {
	glyph         uint16
	text          string
	metricWidth   float64 // em when widthOverride is true
	widthOverride bool
}
type pdfGlyphFace struct {
	name     string
	original pdfFontFace
	shaper   *harfbuzz.Font
	codes    map[pdfGlyphCode]uint16
	entries  []pdfGlyphCode
}

// Residual marks need glyph positioning even when their grapheme starts with
// an explicit spacing character. Keep the pre-existing marked-space path.
func needsGlyphOutput(text string) bool {
	for _, ch := range norm.NFC.String(text) {
		if ch > 0xFFFF || unicode.Is(unicode.Mn, ch) || unicode.Is(unicode.Mc, ch) || unicode.Is(unicode.Me, ch) {
			return true
		}
	}
	return false
}

// Explicit spacing/break controls retain their declared layout policy. Every
// ordinary text cluster is shaped, whether or not it carries a language binding.
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
	// ToUnicode maps each shaped cluster, including ligatures, to its own
	// original text. ActualText can then wrap a whole painted run without
	// assigning that entire run to its first glyph in fallback readers.
	var originalEnds []int
	graphemes := uniseg.NewGraphemes(text)
	for graphemes.Next() {
		_, end := graphemes.Positions()
		for range []rune(norm.NFC.String(graphemes.Str())) {
			originalEnds = append(originalEnds, end)
		}
	}
	// HarfBuzz returns monotonically descending cluster indices for RTL.
	// Assign original logical intervals in ascending input order, independently
	// of visual glyph order. Keep repeated glyphs in each cluster together.
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
func (f *pdfGlyphFace) embed(p *model.Context) (*types.IndirectRef, error) {
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
	program, err := glyphStream(p, f.original.data)
	if err != nil {
		return nil, err
	}
	stream, _, err := p.DereferenceStreamDict(*program)
	if err != nil {
		return nil, err
	}
	stream.InsertInt("Length1", len(f.original.data))
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
	if len(r.glyphFaces) == 0 {
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
	for _, face := range ordered {
		if err = r.ctx.Err(); err != nil {
			return nil, err
		}
		if len(face.entries) == 0 {
			continue
		}
		ref, e := face.embed(p)
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
