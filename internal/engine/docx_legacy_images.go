package engine

import (
	"context"
	"encoding/binary"
	"fmt"
	"image"
	"math"
	"path"
	"strconv"
	"strings"
)

// Preserve the published static preview. Embedded application code is never run.
func (r *docxReader) legacyPictures(n *xmlNode, rels map[string]docxRelation) ([]Block, error) {
	var out []Block
	for _, shape := range n.all("shape") {
		for _, asset := range shape.all("imagedata") {
			rel, ok := rels[asset.attr("id")]
			if !ok || rel.External {
				return nil, fmt.Errorf("%w: missing internal legacy image relationship", ErrUnsupported)
			}
			data, err := r.part(rel.Target)
			if err != nil {
				return nil, err
			}
			im := &Image{Data: data, MediaType: r.types[strings.TrimPrefix(strings.ToLower(path.Ext(rel.Target)), ".")], Alt: asset.attr("title")}
			for _, property := range strings.Split(shape.attr("style"), ";") {
				key, val, _ := strings.Cut(property, ":")
				switch strings.TrimSpace(key) {
				case "width":
					im.Width = legacyLength(val)
				case "height":
					im.Height = legacyLength(val)
				}
			}
			if im.Width <= 0 {
				im.Width = number(n.attr("dxaOrig")) / 20
			}
			if im.Height <= 0 {
				im.Height = number(n.attr("dyaOrig")) / 20
			}
			if _, _, err := decodeDOCXImage(r.ctx, data, nil); err != nil {
				return nil, fmt.Errorf("legacy image: %w", err)
			}
			out = append(out, Block{Kind: Picture, Image: im})
		}
	}
	if len(out) > 0 {
		r.doc.warn("legacy_drawing_preview", "Original legacy drawing preview bytes retained as flow images; floating geometry and embedded OLE application behavior are not represented")
	}
	return out, nil
}
func legacyLength(s string) float64 {
	s = strings.TrimSpace(s)
	for unit, factor := range map[string]float64{"pt": 1, "in": 72, "cm": 72 / 2.54, "mm": 72 / 25.4, "px": .75} {
		if strings.HasSuffix(s, unit) {
			v, e := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(s, unit)), 64)
			if e == nil && finite(v) && v > 0 {
				return v * factor
			}
			return 0
		}
	}
	return 0
}

// Validate bounded record framing without executing metafile drawing commands.
// Dimensions are intrinsic coordinate ratios, never raster allocation requests.
func decodeDOCXImage(ctx context.Context, data []byte, cache outputImageCache) (image.Config, string, error) {
	if err := ctx.Err(); err != nil {
		return image.Config{}, "", err
	}
	u16 := func(i int) uint16 { return binary.LittleEndian.Uint16(data[i:]) }
	u32 := func(i int) uint32 { return binary.LittleEndian.Uint32(data[i:]) }
	cfg := image.Config{}
	kind := ""
	offset := 0
	end := len(data)
	eof := false
	if len(data) >= 88 && u32(0) == 1 && u32(40) == 0x464d4520 {
		kind = "emf"
		if uint64(u32(48)) != uint64(len(data)) {
			return cfg, kind, fmt.Errorf("invalid EMF byte count")
		}
		cfg.Width = int(int32(u32(16)) - int32(u32(8)))
		cfg.Height = int(int32(u32(20)) - int32(u32(12)))
		for records := 0; offset < end; records++ {
			if records >= 1_000_000 || end-offset < 8 {
				return cfg, kind, fmt.Errorf("invalid EMF records")
			}
			size := uint64(u32(offset + 4))
			if size < 8 || size%4 != 0 || size > uint64(end-offset) {
				return cfg, kind, fmt.Errorf("invalid EMF record size")
			}
			if u32(offset) == 14 {
				eof = true
				if offset+int(size) != end {
					return cfg, kind, fmt.Errorf("EMF data follows EOF")
				}
			}
			offset += int(size)
		}
	} else {
		if len(data) >= 22 && u32(0) == 0x9ac6cdd7 {
			var sum uint16
			for i := 0; i < 20; i += 2 {
				sum ^= u16(i)
			}
			if sum != u16(20) || u16(14) == 0 {
				return cfg, "wmf", fmt.Errorf("invalid placeable WMF header")
			}
			cfg.Width = int(int16(u16(10))) - int(int16(u16(6)))
			cfg.Height = int(int16(u16(12))) - int(int16(u16(8)))
			offset = 22
		}
		if len(data)-offset < 18 || (u16(offset) != 1 && u16(offset) != 2) || u16(offset+2) != 9 || (u16(offset+4) != 0x100 && u16(offset+4) != 0x300) {
			return decodeOutputImage(ctx, data, cache)
		}
		kind = "wmf"
		if uint64(u32(offset+6))*2 != uint64(len(data)-offset) {
			return cfg, kind, fmt.Errorf("invalid WMF byte count")
		}
		offset += 18
		for records := 0; offset < end; records++ {
			if records >= 1_000_000 || end-offset < 6 {
				return cfg, kind, fmt.Errorf("invalid WMF records")
			}
			size := uint64(u32(offset)) * 2
			if size < 6 || size > uint64(end-offset) {
				return cfg, kind, fmt.Errorf("invalid WMF record size")
			}
			fn := u16(offset + 4)
			if fn == 0x020c && size >= 10 && (cfg.Width <= 0 || cfg.Height <= 0) {
				cfg.Height = int(math.Abs(float64(int16(u16(offset + 6)))))
				cfg.Width = int(math.Abs(float64(int16(u16(offset + 8)))))
			}
			if fn == 0 {
				eof = true
				if offset+int(size) != end {
					return cfg, kind, fmt.Errorf("WMF data follows EOF")
				}
			}
			offset += int(size)
		}
	}
	if !eof || cfg.Width <= 0 || cfg.Height <= 0 {
		return cfg, kind, fmt.Errorf("invalid %s geometry or EOF", kind)
	}
	return cfg, kind, nil
}
