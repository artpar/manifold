package manifold

import (
	"context"
	"encoding/binary"
	"fmt"

	"golang.org/x/image/font"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

// Validate the TrueType tables consumed by our PDF subsetter before passing it
// caller-controlled data. sfnt also validates every outline, including composites.
func validatePDFTrueType(ctx context.Context, data []byte, parsed *sfnt.Font) error {
	bad := func() error { return fmt.Errorf("%w: malformed or unsupported PDF TrueType tables", ErrUnsupported) }
	if len(data) < 12 {
		return bad()
	}
	u16 := binary.BigEndian.Uint16
	u32 := binary.BigEndian.Uint32
	version := u32(data)
	if version != 0x10000 && version != 0x74727565 {
		return bad()
	}
	count := int(u16(data[4:]))
	if count > (len(data)-12)/16 {
		return bad()
	}
	tables := map[string][]byte{}
	for i := 0; i < count; i++ {
		p := data[12+16*i:]
		name := string(p[:4])
		off, n := uint64(u32(p[8:])), uint64(u32(p[12:]))
		if off+n > uint64(len(data)) {
			return bad()
		}
		if _, ok := tables[name]; ok {
			return bad()
		}
		tables[name] = data[off : off+n]
	}
	for _, name := range []string{"COLR", "CPAL", "CBDT", "CBLC", "sbix", "SVG "} {
		if _, ok := tables[name]; ok {
			return fmt.Errorf("%w: color/bitmap font tables are not supported by the PDF subsetter", ErrUnsupported)
		}
	}
	for name, size := range map[string]int{"head": 54, "hhea": 36, "maxp": 6, "OS/2": 78, "post": 32, "name": 6, "cmap": 4, "hmtx": 4, "loca": 2, "glyf": 0} {
		if table, ok := tables[name]; !ok || len(table) < size {
			return bad()
		}
	}
	if u16(tables["head"][52:]) != 0 || u16(tables["hhea"][32:]) != 0 || u16(tables["hhea"][34:]) == 0 {
		return bad()
	}
	names := tables["name"]
	if u16(names) != 0 {
		return bad()
	}
	records, stringsAt := int(u16(names[2:])), int(u16(names[4:]))
	if records > (len(names)-6)/12 {
		return bad()
	}
	for i := 0; i < records; i++ {
		p := names[6+i*12:]
		end := stringsAt + int(u16(p[8:])) + int(u16(p[10:]))
		if end > len(names) {
			return bad()
		}
		if u16(p) == 3 && u16(p[2:]) == 1 && u16(p[8:])%2 != 0 {
			return bad()
		}
	}
	os2 := tables["OS/2"]
	if u16(os2) >= 2 && len(os2) < 96 {
		return bad()
	}
	if flags := u16(os2[8:]); flags&0x0302 != 0 {
		return fmt.Errorf("%w: font flags prohibit this embedding/subsetting mode", ErrUnsupported)
	}
	cmap := tables["cmap"]
	records = int(u16(cmap[2:]))
	if records > (len(cmap)-4)/8 {
		return bad()
	}
	found := false
	var buffer sfnt.Buffer
	for i := 0; i < records; i++ {
		p := cmap[4+8*i:]
		platform, enc, off := u16(p), u16(p[2:]), uint64(u32(p[4:]))
		if off+2 > uint64(len(cmap)) {
			return bad()
		}
		if (platform == 0 || platform == 3 && enc == 1) && u16(cmap[off:]) == 4 {
			m := cmap[off:]
			if len(m) < 16 {
				return bad()
			}
			length, segments := int(u16(m[2:])), int(u16(m[6:]))/2
			if length > len(m) || segments == 0 || length < 16+8*segments {
				return bad()
			}
			m = m[:length]
			previous := -1
			for j := 0; j < segments; j++ {
				end, start := int(u16(m[14+2*j:])), int(u16(m[16+2*segments+2*j:]))
				if start > end || start <= previous {
					return bad()
				}
				previous = end
				at := 16 + 6*segments + 2*j
				offset := int(u16(m[at:]))
				if offset != 0 && (at+offset+2*(end-start)+2 > len(m) || offset%2 != 0) {
					return bad()
				}
				if !found {
					delta := u16(m[16+4*segments+2*j:])
					for ch := start; ch <= end; ch++ {
						if err := ctx.Err(); err != nil {
							return err
						}
						glyph := uint16(ch) + delta
						if offset != 0 {
							glyph = u16(m[at+offset+2*(ch-start):])
							if glyph != 0 {
								glyph += delta
							}
						}
						if int(glyph) >= parsed.NumGlyphs() {
							return bad()
						}
						expected, err := parsed.GlyphIndex(&buffer, rune(ch))
						if err != nil || expected != sfnt.GlyphIndex(glyph) {
							return fmt.Errorf("%w: inconsistent TrueType Unicode mappings", ErrUnsupported)
						}
					}
				}
			}
			found = true
		}
	}
	if !found {
		return fmt.Errorf("%w: PDF TrueType subsetter requires a Unicode BMP cmap", ErrUnsupported)
	}
	for glyph := 0; glyph < parsed.NumGlyphs(); glyph++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, err := parsed.LoadGlyph(&buffer, sfnt.GlyphIndex(glyph), fixed.I(16), &sfnt.LoadGlyphOptions{}); err != nil {
			return fmt.Errorf("font outline %d: %w", glyph, err)
		}
	}
	if _, err := parsed.Metrics(&buffer, fixed.I(16), font.HintingNone); err != nil {
		return err
	}
	return nil
}
