package engine

import (
	"context"
	"encoding/binary"
	"fmt"
	"golang.org/x/image/font"
	"golang.org/x/image/font/sfnt"

	// subsetGlyphTrueType is an embedding program for already shaped PDF glyphs,
	// not a general font editor. Keep GIDs, metrics, names/notices, cmap and hinting;
	// omit unused outlines and layout/signature tables. Composite references remain
	// valid because their transitive closure retains its original GIDs.
	"golang.org/x/image/math/fixed"
	"sort"
)

func subsetGlyphTrueType(ctx context.Context, data []byte, requested []uint16) ([]byte, error) {
	bad := func() error { return fmt.Errorf("%w: invalid TrueType glyph subset", ErrUnsupported) }
	u16, u32 := binary.BigEndian.Uint16, binary.BigEndian.Uint32
	if len(data) < 12 || (u32(data) != 0x10000 && u32(data) != 0x74727565) {
		return nil, bad()
	}
	n := int(u16(data[4:]))
	if n > (len(data)-12)/16 {
		return nil, bad()
	}
	tables := map[string][]byte{}
	for i := 0; i < n; i++ {
		p := data[12+16*i:]
		tag := string(p[:4])
		off, size := uint64(u32(p[8:])), uint64(u32(p[12:]))
		if off+size > uint64(len(data)) {
			return nil, bad()
		}
		if _, exists := tables[tag]; exists {
			return nil, bad()
		}
		tables[tag] = data[off : off+size]
	}
	for tag, size := range map[string]int{"head": 54, "maxp": 6, "loca": 2, "glyf": 0, "hhea": 36, "hmtx": 4, "name": 6, "cmap": 4, "OS/2": 78, "post": 32} {
		if b, ok := tables[tag]; !ok || len(b) < size {
			return nil, bad()
		}
	}
	if u16(tables["OS/2"][8:])&0x0302 != 0 {
		return nil, fmt.Errorf("%w: font flags prohibit glyph subsetting", ErrUnsupported)
	}
	count := int(u16(tables["maxp"][4:]))
	format := u16(tables["head"][50:])
	if count == 0 || format > 1 {
		return nil, bad()
	}
	step := 2
	if format == 1 {
		step = 4
	}
	if len(tables["loca"])/step < count+1 {
		return nil, bad()
	}
	offsets := make([]uint32, count+1)
	for i := range offsets {
		if format == 0 {
			offsets[i] = uint32(u16(tables["loca"][2*i:])) * 2
		} else {
			offsets[i] = u32(tables["loca"][4*i:])
		}
		if uint64(offsets[i]) > uint64(len(tables["glyf"])) || (i > 0 && offsets[i] < offsets[i-1]) {
			return nil, bad()
		}
	}
	keep := make([]bool, count)
	keep[0] = true
	queue := []uint16{0}
	for _, gid := range requested {
		if int(gid) >= count {
			return nil, bad()
		}
		if !keep[gid] {
			keep[gid] = true
			queue = append(queue, gid)
		}
	}
	components := make([][]uint16, count)
	indegree := make([]int, count)
	for at := 0; at < len(queue); at++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		gid := queue[at]
		b := tables["glyf"][offsets[gid]:offsets[int(gid)+1]]
		if len(b) == 0 {
			continue
		}
		if len(b) < 10 {
			return nil, bad()
		}
		if int16(u16(b)) >= 0 {
			continue
		}
		pos := 10
		instructions := false
		for {
			if pos+4 > len(b) {
				return nil, bad()
			}
			flags, child := u16(b[pos:]), u16(b[pos+2:])
			pos += 4
			if int(child) >= count {
				return nil, bad()
			}
			components[gid] = append(components[gid], child)
			indegree[child]++
			if !keep[child] {
				keep[child] = true
				queue = append(queue, child)
			}
			if flags&1 != 0 {
				pos += 4
			} else {
				pos += 2
			}
			scale := flags & (8 | 64 | 128)
			switch scale {
			case 0:
			case 8:
				pos += 2
			case 64:
				pos += 4
			case 128:
				pos += 8
			default:
				return nil, bad()
			}
			if pos > len(b) {
				return nil, bad()
			}
			instructions = instructions || flags&256 != 0
			if flags&32 == 0 {
				break
			}
		}
		if instructions && (pos+2 > len(b) || pos+2+int(u16(b[pos:])) > len(b)) {
			return nil, bad()
		}
	}
	// Topological validation avoids recursive traversal and rejects cycles even
	// when a component was encountered earlier through another requested glyph.

	// Validate the TrueType tables consumed by our PDF subsetter before passing it
	// caller-controlled data. sfnt also validates every outline, including composites.

	var roots []uint16
	for _, gid := range queue {
		if indegree[gid] == 0 {
			roots = append(roots, gid)
		}
	}
	for at := 0; at < len(roots); at++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		for _, child := range components[roots[at]] {
			indegree[child]--
			if indegree[child] == 0 {
				roots = append(roots, child)
			}
		}
	}
	if len(roots) != len(queue) {
		return nil, bad()
	}
	loca := make([]byte, 4*(count+1))
	var glyf []byte
	for gid := 0; gid < count; gid++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		binary.BigEndian.PutUint32(loca[4*gid:], uint32(len(glyf)))
		if keep[gid] {
			glyf = append(glyf, tables["glyf"][offsets[gid]:offsets[gid+1]]...)
			for len(glyf)%4 != 0 {
				glyf = append(glyf, 0)
			}
		}
		if uint64(len(glyf)) > 0xffffffff {
			return nil, bad()
		}
	}
	binary.BigEndian.PutUint32(loca[4*count:], uint32(len(glyf)))
	head := append([]byte{}, tables["head"]...)
	binary.BigEndian.PutUint32(head[8:], 0)
	binary.BigEndian.PutUint16(head[50:], 1)
	outTables := map[string][]byte{}
	for _, tag := range []string{"OS/2", "cmap", "cvt ", "fpgm", "gasp", "glyf", "head", "hhea", "hmtx", "loca", "maxp", "name", "post", "prep"} {
		if b, ok := tables[tag]; ok {
			outTables[tag] = b
		}
	}
	outTables["head"], outTables["glyf"], outTables["loca"] = head, glyf, loca
	var tags []string
	total := uint64(12 + 16*len(outTables))
	for tag, b := range outTables {
		tags = append(tags, tag)
		total += (uint64(len(b)) + 3) &^ 3
	}
	if total > 0xffffffff || total > uint64(^uint(0)>>1) {
		return nil, bad()
	}
	sort.Strings(tags)
	out := make([]byte, int(total))
	copy(out[:4], data[:4])
	binary.BigEndian.PutUint16(out[4:], uint16(len(tags)))
	power, selector := 1, 0
	for power*2 <= len(tags) {
		power *= 2
		selector++
	}
	binary.BigEndian.PutUint16(out[6:], uint16(power*16))
	binary.BigEndian.PutUint16(out[8:], uint16(selector))
	binary.BigEndian.PutUint16(out[10:], uint16(len(tags)*16-power*16))
	off, headAt := 12+16*len(tags), 0
	for i, tag := range tags {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		b := outTables[tag]
		record := out[12+16*i:]
		copy(record, tag)
		binary.BigEndian.PutUint32(record[4:], trueTypeChecksum(b))
		binary.BigEndian.PutUint32(record[8:], uint32(off))
		binary.BigEndian.PutUint32(record[12:], uint32(len(b)))
		copy(out[off:], b)
		if tag == "head" {
			headAt = off
		}
		off += (len(b) + 3) &^ 3
	}
	binary.BigEndian.PutUint32(out[headAt+8:], 0xb1b0afba-trueTypeChecksum(out))
	return out, nil
}

func trueTypeChecksum(b []byte) uint32 {
	var sum uint32
	for len(b) >= 4 {
		sum += binary.BigEndian.Uint32(b)
		b = b[4:]
	}
	var tail [4]byte
	copy(tail[:], b)
	return sum + binary.BigEndian.Uint32(tail[:])
}

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
