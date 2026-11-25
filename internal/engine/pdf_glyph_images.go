package engine

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"strconv"
	"strings"

	"github.com/go-text/typesetting/font/cff"
	fonts "github.com/go-text/typesetting/font/opentype"
	"github.com/go-text/typesetting/font/opentype/tables"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
	"golang.org/x/image/vector"
)

type pdfOutlineFont struct {
	font   *cff.CFF
	names  map[string]fonts.GID
	matrix Matrix
}

// Some real PDFs contain glyph names such as gid00009, no ToUnicode, and no
// Unicode-bearing font cmap. Preserve source outlines as individual span images;
// do not guess text from GIDs, font family, geometry, or OCR.
func (r *pdfReader) preserveUnmappedGlyphs(data []byte, s *pdfTextState) (bool, error) {
	f := s.font
	if f.composite || f.dictionary == nil || len(data) > 65536 {
		return false, nil
	}
	if s.renderMode != 0 && s.renderMode != 3 {
		return false, nil
	}
	if s.fillUnknown {
		return false, nil
	}
	if f.outlines == nil {
		descriptor, e := r.pdf.DereferenceDict(f.dictionary["FontDescriptor"])
		if e != nil {
			return false, e
		}
		program, _, e := r.pdf.DereferenceStreamDict(descriptor["FontFile3"])
		if e != nil {
			return false, e
		}
		if program == nil || program.Dict["Subtype"] != types.Name("Type1C") {
			return false, nil
		}
		raw, e := r.decodePDFStream(descriptor["FontFile3"], program)
		if e != nil {
			return false, e
		}
		matrix, e := cffFontMatrix(raw)
		if e != nil {
			return false, e
		}
		parsed, e := cff.Parse(raw)
		if e != nil {
			return false, e
		}
		names := map[string]fonts.GID{}
		for i := 0; i < len(parsed.Charstrings); i++ {
			name := parsed.GlyphName(fonts.GID(i))
			if _, dup := names[name]; dup && name != "" {
				return false, fmt.Errorf("ambiguous embedded CFF glyph name")
			}
			names[name] = fonts.GID(i)
		}
		f.outlines = &pdfOutlineFont{parsed, names, matrix}
	}
	// Separate known Unicode from genuinely unmapped sequences. Known text must
	// remain text, not disappear into the raster fallback of a neighbouring glyph.
	for start := 0; start < len(data); {
		known := false
		if _, e := f.decode(data[start : start+1]); e == nil {
			known = true
		}
		end := start + 1
		for end < len(data) {
			_, e := f.decode(data[end : end+1])
			if (e == nil) != known {
				break
			}
			end++
		}
		if known {
			if e := r.show(types.HexLiteral(fmt.Sprintf("%x", data[start:end])), s); e != nil {
				return false, e
			}
		} else {
			if e := r.paintGlyphSpan(data[start:end], s); e != nil {
				return false, e
			}
		}
		start = end
	}
	return true, nil
}
func (r *pdfReader) paintGlyphSpan(data []byte, s *pdfTextState) error {
	// Raster density must include the source CTM; real publisher PDFs often
	// use a unit text size and scale the entire frame by ~10x.
	base := r.pageInfo.Transform.compose(s.ctm).compose(s.tm)
	pixelsPerPoint := 4 * max(math.Hypot(base[0], base[1]), math.Hypot(base[2], base[3]))
	if !finite(pixelsPerPoint) || pixelsPerPoint <= 0 {
		return fmt.Errorf("invalid glyph frame scale")
	}
	f := s.font
	advance, estimated, e := f.displacement(data, s.size, s.charSpace, s.wordSpace, s.hscale)
	if e != nil {
		return e
	}
	if s.size <= 0 || s.hscale <= 0 || advance < 0 {
		return fmt.Errorf("%w: unmapped glyph span transform", ErrUnsupported)
	}
	type glyph struct {
		segments []fonts.Segment
		offset   float64
	}
	var glyphs []glyph
	cursor := 0.0
	left, right := 0.0, advance
	bottom, top := f.descent*s.size/1000, f.ascent*s.size/1000
	segments := 0
	for _, code := range data {
		if e := r.ctx.Err(); e != nil {
			return e
		}
		gid, ok := f.outlines.names[f.encoding[code]]
		if !ok {
			return fmt.Errorf("%w: embedded font lacks glyph %q", ErrUnsupported, f.encoding[code])
		}
		path, _, e := f.outlines.font.LoadGlyph(tables.GlyphID(gid))
		if e != nil {
			return e
		}
		segments += len(path)
		if segments > 1_000_000 {
			return fmt.Errorf("glyph outline segment budget exceeded")
		}
		for _, seg := range path {
			for _, point := range seg.ArgsSlice() {
				p := f.outlines.matrix.point(float64(point.X), float64(point.Y))
				x, y := cursor+p.X*s.size*s.hscale, p.Y*s.size
				left = min(left, x)
				right = max(right, x)
				bottom = min(bottom, y)
				top = max(top, y)
			}
		}
		glyphs = append(glyphs, glyph{path, cursor})
		width, exact := f.widths[uint32(code)]
		if !exact {
			width = f.defaultWidth
		}
		cursor += (width*s.size/1000 + s.charSpace) * s.hscale
		if code == 32 {
			cursor += s.wordSpace * s.hscale
		}
	}
	if !finite(left) || !finite(right) || !finite(top) || !finite(bottom) || right <= left || top <= bottom {
		return fmt.Errorf("invalid glyph outline bounds")
	}
	fw, fh := math.Ceil((right-left)*pixelsPerPoint), math.Ceil((top-bottom)*pixelsPerPoint)
	if !finite(fw) || !finite(fh) || fw <= 0 || fh <= 0 || fw > float64(maxPDFImagePixels)/fh {
		return fmt.Errorf("glyph raster exceeds image limit")
	}
	w, h := int(fw), int(fh)
	if int64(w) > maxPDFImagePixels/int64(max(1, h)) {
		return fmt.Errorf("glyph raster exceeds image limit")
	}
	if e = r.chargePDFRaster(w, h); e != nil {
		return e
	}
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	raster := vector.NewRasterizer(w, h)
	for _, glyph := range glyphs {
		point := func(p fonts.SegmentPoint) (float32, float32) {
			v := f.outlines.matrix.point(float64(p.X), float64(p.Y))
			return float32((glyph.offset + v.X*s.size*s.hscale - left) * pixelsPerPoint), float32((top - v.Y*s.size) * pixelsPerPoint)
		}
		for _, seg := range glyph.segments {
			x, y := point(seg.Args[0])
			switch seg.Op {
			case fonts.SegmentOpMoveTo:
				raster.ClosePath()
				raster.MoveTo(x, y)
			case fonts.SegmentOpLineTo:
				raster.LineTo(x, y)
			case fonts.SegmentOpQuadTo:
				x1, y1 := point(seg.Args[1])
				raster.QuadTo(x, y, x1, y1)
			case fonts.SegmentOpCubeTo:
				x1, y1 := point(seg.Args[1])
				x2, y2 := point(seg.Args[2])
				raster.CubeTo(x, y, x1, y1, x2, y2)
			}
		}
		raster.ClosePath()
	}
	alpha := uint8(math.Round(max(0, min(1, s.rulingFillAlpha)) * 255))
	if s.renderMode == 3 {
		alpha = 0
	}
	fill := color.NRGBA{uint8(math.Round(s.fill[0] * 255)), uint8(math.Round(s.fill[1] * 255)), uint8(math.Round(s.fill[2] * 255)), alpha}
	raster.Draw(dst, dst.Bounds(), image.NewUniform(fill), image.Point{})
	var encoded bytes.Buffer
	if e = png.Encode(&encoded, dst); e != nil {
		return e
	}
	source := r.sourceFor(s, advance, estimated)
	q := Quad{base.point(left, s.rise+top), base.point(right, s.rise+top), base.point(right, s.rise+bottom), base.point(left, s.rise+bottom)}
	source.Quad = &q
	source.Evidence = "embedded_glyph_outline_raster"
	source.Precision = "glyph_outline_bounds"
	r.newline()
	r.lines = append(r.lines, Block{Kind: Picture, Source: source, Image: &Image{Data: encoded.Bytes(), MediaType: "image/png", Width: math.Hypot(q[1].X-q[0].X, q[1].Y-q[0].Y), Height: math.Hypot(q[3].X-q[0].X, q[3].Y-q[0].Y), Alt: fmt.Sprintf("Unmapped embedded glyphs: font %s; source codes %X", f.name, data)}})
	r.warnPage("pdf_unmapped_glyph_outline", "Embedded CFF glyphs without Unicode mappings are preserved as 288-DPI transparent span images with source font/codes/geometry; searchable/editable text is unavailable and no characters are invented")
	s.advanceText(advance)
	return nil
}

// Read the CFF Top DICT FontMatrix rather than assuming every font has the
// default 0.001 scale. The outline parser deliberately leaves that transform to
// consumers. INDEX offsets and DICT numbers follow Adobe Technical Note 5176.
func cffFontMatrix(data []byte) (Matrix, error) {
	matrix := Matrix{.001, 0, 0, .001, 0, 0}
	if len(data) < 4 || data[0] != 1 {
		return matrix, fmt.Errorf("unsupported CFF header")
	}
	index := func(pos int) ([][]byte, int, error) {
		if pos < 0 || pos+2 > len(data) {
			return nil, 0, fmt.Errorf("truncated CFF INDEX")
		}
		n := int(binary.BigEndian.Uint16(data[pos:]))
		pos += 2
		if n == 0 {
			return nil, pos, nil
		}
		if pos >= len(data) {
			return nil, 0, fmt.Errorf("truncated CFF INDEX")
		}
		size := int(data[pos])
		pos++
		if size < 1 || size > 4 || n > 65535 || pos+(n+1)*size > len(data) {
			return nil, 0, fmt.Errorf("invalid CFF INDEX")
		}
		offsets := make([]int, n+1)
		for i := range offsets {
			for j := 0; j < size; j++ {
				offsets[i] = offsets[i]<<8 | int(data[pos+i*size+j])
			}
		}
		start := pos + (n+1)*size
		if offsets[0] != 1 || offsets[n] < 1 || offsets[n]-1 > len(data)-start {
			return nil, 0, fmt.Errorf("invalid CFF INDEX offsets")
		}
		out := make([][]byte, n)
		for i := range out {
			if offsets[i+1] < offsets[i] {
				return nil, 0, fmt.Errorf("unordered CFF INDEX")
			}
			out[i] = data[start+offsets[i]-1 : start+offsets[i+1]-1]
		}
		return out, start + offsets[n] - 1, nil
	}
	_, pos, e := index(int(data[2]))
	if e != nil {
		return matrix, e
	}
	dicts, _, e := index(pos)
	if e != nil || len(dicts) != 1 {
		return matrix, fmt.Errorf("invalid CFF Top DICT")
	}
	dict := dicts[0]
	var stack []float64
	for i := 0; i < len(dict); i++ {
		b := dict[i]
		var value float64
		switch {
		case b >= 32 && b <= 246:
			value = float64(int(b) - 139)
		case b >= 247 && b <= 254:
			if i+1 >= len(dict) {
				return matrix, fmt.Errorf("truncated CFF number")
			}
			i++
			if b <= 250 {
				value = float64((int(b)-247)*256 + int(dict[i]) + 108)
			} else {
				value = float64(-(int(b)-251)*256 - int(dict[i]) - 108)
			}
		case b == 28:
			if i+2 >= len(dict) {
				return matrix, fmt.Errorf("truncated CFF number")
			}
			value = float64(int16(binary.BigEndian.Uint16(dict[i+1:])))
			i += 2
		case b == 29:
			if i+4 >= len(dict) {
				return matrix, fmt.Errorf("truncated CFF number")
			}
			value = float64(int32(binary.BigEndian.Uint32(dict[i+1:])))
			i += 4
		case b == 30:
			var number strings.Builder
			ended := false
			for !ended {
				i++
				if i >= len(dict) {
					return matrix, fmt.Errorf("truncated CFF real")
				}
				for _, n := range []byte{dict[i] >> 4, dict[i] & 15} {
					if n == 15 {
						ended = true
						break
					}
					switch {
					case n <= 9:
						number.WriteByte('0' + n)
					case n == 10:
						number.WriteByte('.')
					case n == 11:
						number.WriteByte('E')
					case n == 12:
						number.WriteString("E-")
					case n == 14:
						number.WriteByte('-')
					default:
						return matrix, fmt.Errorf("invalid CFF real")
					}
				}
			}
			var e error
			value, e = strconv.ParseFloat(number.String(), 64)
			if e != nil || !finite(value) {
				return matrix, fmt.Errorf("invalid CFF real")
			}
		default:
			op := int(b)
			if b == 12 {
				if i+1 >= len(dict) {
					return matrix, fmt.Errorf("truncated CFF operator")
				}
				i++
				op = 1200 + int(dict[i])
			}
			if op == 1207 {
				if len(stack) != 6 {
					return matrix, fmt.Errorf("invalid CFF FontMatrix")
				}
				copy(matrix[:], stack)
				for _, v := range matrix {
					if !finite(v) {
						return matrix, fmt.Errorf("invalid CFF FontMatrix")
					}
				}
			}
			stack = nil
			continue
		}
		stack = append(stack, value)
		if len(stack) > 48 {
			return matrix, fmt.Errorf("CFF DICT stack exceeds limit")
		}
	}
	return matrix, nil
}
