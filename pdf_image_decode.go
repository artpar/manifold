package manifold

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
	"golang.org/x/image/ccitt"
)

const maxPDFImageBytes = 200_000_000
const maxPDFRasterPixels = int64(100_000_000)

func (r *pdfReader) chargePDFRaster(w, h int) error {
	if err := r.ctx.Err(); err != nil {
		return err
	}
	n := int64(w) * int64(h)
	if n > maxPDFRasterPixels-r.rasterPixels {
		return fmt.Errorf("PDF raster work exceeds %d pixels", maxPDFRasterPixels)
	}
	r.rasterPixels += n
	return nil
}

func (r *pdfReader) imageBoolParam(d types.Dict, key string, fallback bool) (bool, error) {
	if d[key] == nil {
		return fallback, nil
	}
	v, err := r.pdf.Dereference(d[key])
	if err != nil {
		return false, err
	}
	if b, ok := v.(types.Boolean); ok {
		return bool(b), nil
	}
	return false, fmt.Errorf("invalid PDF image boolean %s", key)
}

// Decode byte filters before invoking a raster renderer. Terminal JPEG headers
// are checked before any consumer can expand them; fax decoding is bounded too.
// No cached syntax entry is used for a partial image-filter pipeline.
func (r *pdfReader) preparePDFImage(sd *types.StreamDict, w, h int) error {
	if err := r.chargePDFRaster(w, h); err != nil {
		return err
	}
	mask, err := r.imageBoolParam(sd.Dict, "ImageMask", false)
	if err != nil {
		return err
	}
	bits, err := r.pdfStreamParam(sd.Dict, "BitsPerComponent", 8)
	if err != nil {
		return err
	}
	if mask {
		bits = 1
	}
	if sd.Dict["ImageMask"] != nil {
		sd.Dict["ImageMask"] = types.Boolean(mask)
	}
	if bits != 1 && bits != 2 && bits != 4 && bits != 8 && bits != 16 {
		return fmt.Errorf("invalid PDF image sample depth")
	}
	sd.Dict["BitsPerComponent"] = types.Integer(bits)
	filters := sd.FilterPipeline
	if len(filters) > maxPDFStreamFilters {
		return fmt.Errorf("PDF stream exceeds %d filters", maxPDFStreamFilters)
	}
	last := ""
	if len(filters) > 0 {
		last = filters[len(filters)-1].Name
	}
	if last == "DCTDecode" && bits != 8 || last == "CCITTFaxDecode" && bits != 1 {
		return fmt.Errorf("PDF image sample depth contradicts terminal codec")
	}
	prefix := *sd
	terminal := last == "DCTDecode" || last == "CCITTFaxDecode"
	if terminal {
		prefix.FilterPipeline = filters[:len(filters)-1]
		prefix.Content = nil
	}
	if last == "JPXDecode" || last == "JBIG2Decode" {
		return fmt.Errorf("%w: PDF image codec %s", ErrUnsupported, last)
	}
	data, err := r.decodePDFStreamLimit(nil, &prefix, maxPDFImageBytes)
	if err != nil {
		return err
	}
	if last == "DCTDecode" {
		cfg, err := jpeg.DecodeConfig(bytes.NewReader(data))
		if err != nil {
			return fmt.Errorf("invalid PDF JPEG header: %w", err)
		}
		if cfg.Width != w || cfg.Height != h {
			return fmt.Errorf("image stream dimensions contradict image dictionary")
		}
		sd.Raw = data
		sd.Content = nil
		sd.FilterPipeline = filters[len(filters)-1:]
		return nil
	}
	if last == "CCITTFaxDecode" {
		parms := filters[len(filters)-1].DecodeParms
		columns, err := r.pdfStreamParam(parms, "Columns", 1728)
		if err != nil {
			return err
		}
		rows, err := r.pdfStreamParam(parms, "Rows", h)
		if err != nil {
			return err
		}
		if rows == 0 {
			rows = h
		}
		if columns != w || rows != h {
			return fmt.Errorf("CCITT parameters contradict image dimensions")
		}
		k, err := r.pdfStreamParam(parms, "K", 0)
		if err != nil {
			return err
		}
		if k > 0 {
			return fmt.Errorf("%w: mixed CCITT Group3 encoding", ErrUnsupported)
		}
		invert, err := r.imageBoolParam(parms, "BlackIs1", false)
		if err != nil {
			return err
		}
		align, err := r.imageBoolParam(parms, "EncodedByteAlign", false)
		if err != nil {
			return err
		}
		mode := ccitt.Group3
		if k < 0 {
			mode = ccitt.Group4
		}
		reader := ccitt.NewReader(contextReader{r.ctx, bytes.NewReader(data)}, ccitt.MSB, mode, w, h, &ccitt.Options{Invert: invert, Align: align})
		limit := min(maxPDFImageBytes, maxPDFDecodedBytes-r.streamBudget.total)
		data, err = readPDFDecoded(r.ctx, reader, limit)
		if err != nil {
			return err
		}
		r.streamBudget.total += len(data)
		if sd.Dict["ColorSpace"] == nil {
			sd.Dict["ColorSpace"] = types.Name("DeviceGray")
		}
		bits = 1
		sd.Dict["BitsPerComponent"] = types.Integer(1)
	}
	sd.Raw = data
	sd.Content = nil
	sd.FilterPipeline = nil
	return nil
}

func (r *pdfReader) preparePDFColorSpace(o types.Object, depth int) (types.Object, error) {
	if depth > 16 {
		return nil, fmt.Errorf("image color space nesting exceeds limit")
	}
	v, err := r.pdf.Dereference(o)
	if err != nil {
		return nil, err
	}
	a, ok := v.(types.Array)
	if !ok {
		return v, nil
	}
	a = append(types.Array(nil), a...)
	if len(a) == 4 && a[0] == types.Name("Indexed") {
		a[1], err = r.preparePDFColorSpace(a[1], depth+1)
		if err != nil {
			return nil, err
		}
		lookup, err := r.pdf.Dereference(a[3])
		if err != nil {
			return nil, err
		}
		if sd, ok := lookup.(types.StreamDict); ok {
			data, err := r.decodePDFStream(a[3], &sd)
			if err != nil {
				return nil, err
			}
			a[3] = types.HexLiteral(hex.EncodeToString(data))
		}
	}
	return a, nil
}

// Device samples are read by component, including row padding and16-bit values.
// Keeping high-depth samples in NRGBA64 avoids silently dropping their low byte.
func (r *pdfReader) deviceImage(sd *types.StreamDict, w, h int) (image.Image, bool, error) {
	if len(sd.FilterPipeline) != 0 {
		return nil, false, nil
	}
	name, ok := sd.Dict["ColorSpace"].(types.Name)
	if !ok {
		return nil, false, nil
	}
	bits := int(*sd.IntEntry("BitsPerComponent"))
	channels := 0
	switch name {
	case "DeviceGray":
		channels = 1
	case "DeviceRGB":
		channels = 3
	case "DeviceCMYK":
		if bits == 8 {
			return nil, false, nil
		}
		channels = 4
		r.warnPage("pdf_image_color", "Device CMYK samples are converted to RGB; original device-color semantics are not retained")
	default:
		return nil, false, nil
	}
	row := (w*channels*bits + 7) / 8
	if len(sd.Raw) < row*h {
		return nil, true, fmt.Errorf("truncated PDF image samples")
	}
	decode := make([]float64, channels*2)
	for c := 0; c < channels; c++ {
		decode[2*c+1] = 1
	}
	if d, err := r.pdf.DereferenceArray(sd.Dict["Decode"]); err != nil {
		return nil, true, err
	} else if len(d) > 0 {
		if len(d) != len(decode) {
			return nil, true, fmt.Errorf("invalid PDF image Decode array")
		}
		for i, v := range d {
			decode[i], err = r.pdf.DereferenceNumber(v)
			if err != nil || !finite(decode[i]) {
				return nil, true, fmt.Errorf("invalid PDF image Decode value")
			}
		}
	}
	sample := func(y, index int) uint16 {
		start := index * bits
		var value uint32
		for bit := 0; bit < bits; bit++ {
			n := start + bit
			value = value<<1 | uint32(sd.Raw[y*row+n/8]>>uint(7-n%8)&1)
		}
		c := index % channels
		fraction := float64(value) / float64((uint32(1)<<uint(bits))-1)
		normalized := (1-fraction)*decode[2*c] + fraction*decode[2*c+1]
		return uint16(max(0, min(1, normalized))*65535 + .5)
	}
	colorSample := func(y, x int) (uint16, uint16, uint16) {
		a := sample(y, x*channels)
		if channels == 1 {
			return a, a, a
		}
		b, c := sample(y, x*channels+1), sample(y, x*channels+2)
		if channels == 4 {
			k := uint32(65535 - sample(y, x*channels+3))
			return uint16(uint32(65535-a) * k / 65535), uint16(uint32(65535-b) * k / 65535), uint16(uint32(65535-c) * k / 65535)
		}
		return a, b, c
	}
	if bits == 16 {
		dst := image.NewNRGBA64(image.Rect(0, 0, w, h))
		for y := 0; y < h; y++ {
			if err := r.ctx.Err(); err != nil {
				return nil, true, err
			}
			for x := 0; x < w; x++ {
				red, green, blue := colorSample(y, x)
				dst.SetNRGBA64(x, y, color.NRGBA64{R: red, G: green, B: blue, A: 65535})
			}
		}
		return dst, true, nil
	}
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		if err := r.ctx.Err(); err != nil {
			return nil, true, err
		}
		for x := 0; x < w; x++ {
			r, g, b := colorSample(y, x)
			red, green, blue := uint8(r>>8), uint8(g>>8), uint8(b>>8)
			dst.SetNRGBA(x, y, color.NRGBA{R: red, G: green, B: blue, A: 255})
		}
	}
	return dst, true, nil
}
