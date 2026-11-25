package engine

import (
	"fmt"
	"image"
	"image/color"
	"math"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// Decode palette indices before device-color conversion. In particular an
// indexed CMYK image need not pass through a TIFF format the Go decoder rejects.
func (r *pdfReader) indexedDeviceImage(sd *types.StreamDict, w, h int, cs types.Array) (image.Image, bool, error) {
	if len(cs) != 4 {
		return nil, true, fmt.Errorf("invalid Indexed color space")
	}
	channels := 0
	switch cs[1] {
	case types.Name("DeviceGray"):
		channels = 1
	case types.Name("DeviceRGB"):
		channels = 3
	case types.Name("DeviceCMYK"):
		channels = 4
	}
	if channels == 0 {
		return nil, false, nil
	}
	high, e := r.pdf.DereferenceInteger(cs[2])
	if e != nil || high == nil || *high < 0 || *high > 255 {
		return nil, true, fmt.Errorf("invalid Indexed high value")
	}
	lookup, e := r.pdf.Dereference(cs[3])
	if e != nil {
		return nil, true, e
	}
	palette, e := pdfString(lookup)
	if e != nil {
		return nil, true, e
	}
	if len(palette) < (int(*high)+1)*channels {
		return nil, true, fmt.Errorf("truncated Indexed palette")
	}
	bits := int(*sd.IntEntry("BitsPerComponent"))
	if bits == 16 {
		return nil, true, fmt.Errorf("invalid Indexed sample depth")
	}
	row := (w*bits + 7) / 8
	if len(sd.Raw) < row*h {
		return nil, true, fmt.Errorf("truncated Indexed samples")
	}
	lo, hi := 0.0, float64((uint32(1)<<uint(bits))-1)
	if arr, e := r.pdf.DereferenceArray(sd.Dict["Decode"]); e != nil {
		return nil, true, e
	} else if len(arr) > 0 {
		if len(arr) != 2 {
			return nil, true, fmt.Errorf("invalid Indexed Decode")
		}
		lo, e = r.pdf.DereferenceNumber(arr[0])
		if e != nil {
			return nil, true, e
		}
		hi, e = r.pdf.DereferenceNumber(arr[1])
		if e != nil || !finite(lo) || !finite(hi) {
			return nil, true, fmt.Errorf("invalid Indexed Decode")
		}
	}
	if channels == 4 {
		r.warnPage("pdf_image_color", "Device CMYK samples are converted to RGB; original device-color semantics are not retained")
	}
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		if e := r.ctx.Err(); e != nil {
			return nil, true, e
		}
		for x := 0; x < w; x++ {
			value := 0
			for i := 0; i < bits; i++ {
				bit := x*bits + i
				value = value<<1 | int(sd.Raw[y*row+bit/8]>>uint(7-bit%8)&1)
			}
			index := int(math.Round(lo + float64(value)/float64((uint32(1)<<uint(bits))-1)*(hi-lo)))
			index = max(0, min(int(*high), index))
			offset := index * channels
			red, green, blue := palette[offset], palette[offset], palette[offset]
			if channels == 3 {
				red, green, blue = palette[offset], palette[offset+1], palette[offset+2]
			} else if channels == 4 {
				red, green, blue = color.CMYKToRGB(palette[offset], palette[offset+1], palette[offset+2], palette[offset+3])
			}
			dst.SetNRGBA(x, y, color.NRGBA{red, green, blue, 255})
		}
	}
	return dst, true, nil
}
