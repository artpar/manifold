package engine

import (
	"fmt"
	"image"
	"image/color"
	"math"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// ISO 32000-1 11.6.5.3: undo preblending in the original image's color
// component space after Decode, before attaching its soft-mask alpha.
func (r *pdfReader) pdfImageMatte(mask, parent *types.StreamDict, w, h, mw, mh int) ([]float64, error) {
	if mask.Dict["Matte"] == nil {
		return nil, nil
	}
	a, e := r.pdf.DereferenceArray(mask.Dict["Matte"])
	if e != nil {
		return nil, e
	}
	n := 0
	switch parent.Dict["ColorSpace"] {
	case types.Name("DeviceRGB"):
		n = 3
	case types.Name("DeviceGray"):
		n = 1
	}
	if n == 0 {
		return nil, fmt.Errorf("%w: soft-mask Matte requires original device RGB/Gray components", ErrUnsupported)
	}
	if len(a) != n || w != mw || h != mh {
		return nil, fmt.Errorf("soft-mask Matte components/dimensions disagree")
	}
	out := make([]float64, n)
	for i, v := range a {
		out[i], e = r.pdf.DereferenceNumber(v)
		if e != nil || !finite(out[i]) || out[i] < 0 || out[i] > 1 {
			return nil, fmt.Errorf("invalid soft-mask Matte component")
		}
	}
	return out, nil
}
func unblendPDFMatte(c color.NRGBA64, a uint16, matte []float64) color.NRGBA64 {
	if a == 0 {
		c.R, c.G, c.B = 0, 0, 0
		return c
	}
	recover := func(v uint16, i int) uint16 {
		m := matte[0]
		if len(matte) == 3 {
			m = matte[i]
		}
		alpha := float64(a) / 65535
		return uint16(math.Round(max(0, min(1, (float64(v)/65535-(1-alpha)*m)/alpha)) * 65535))
	}
	c.R, c.G, c.B = recover(c.R, 0), recover(c.G, 1), recover(c.B, 2)
	return c
}
func (r *pdfReader) applyPDFExplicitMask(pixels image.Image, obj types.Object, parent *types.StreamDict, w, h int) (image.Image, error) {
	value, e := r.pdf.Dereference(obj)
	if e != nil {
		return nil, e
	}
	var alpha func(int, int) uint16
	if ranges, ok := value.(types.Array); ok {
		channels := 0
		switch parent.Dict["ColorSpace"] {
		case types.Name("DeviceRGB"):
			channels = 3
		case types.Name("DeviceGray"):
			channels = 1
		case types.Name("DeviceCMYK"):
			channels = 4
		}
		if cs, ok := parent.Dict["ColorSpace"].(types.Array); ok && len(cs) > 0 && cs[0] == types.Name("Indexed") {
			channels = 1
		}
		if channels == 0 || len(ranges) != channels*2 || len(parent.FilterPipeline) != 0 {
			return nil, fmt.Errorf("%w: color-key mask requires decoded source components", ErrUnsupported)
		}
		bits := int(*parent.IntEntry("BitsPerComponent"))
		row := (w*channels*bits + 7) / 8
		if len(parent.Raw) < row*h {
			return nil, fmt.Errorf("truncated color-key samples")
		}
		bounds := make([]int, len(ranges))
		for i, o := range ranges {
			v, e := r.pdf.DereferenceInteger(o)
			if e != nil || v == nil || *v < 0 || int(*v) > int((uint32(1)<<uint(bits))-1) {
				return nil, fmt.Errorf("invalid color-key range")
			}
			bounds[i] = int(*v)
		}
		for i := 0; i < len(bounds); i += 2 {
			if bounds[i] > bounds[i+1] {
				return nil, fmt.Errorf("reversed color-key range")
			}
		}
		alpha = func(x, y int) uint16 {
			for c := 0; c < channels; c++ {
				start := (x*channels + c) * bits
				v := 0
				for bit := 0; bit < bits; bit++ {
					pos := start + bit
					v = v<<1 | int(parent.Raw[y*row+pos/8]>>uint(7-pos%8)&1)
				}
				if v < bounds[2*c] || v > bounds[2*c+1] {
					return 65535
				}
			}
			return 0
		}
	} else {
		sd, _, e := r.pdf.DereferenceStreamDict(obj)
		if e != nil {
			return nil, e
		}
		if sd == nil {
			return nil, fmt.Errorf("invalid explicit image mask")
		}
		mw, mh, e := r.imageDimensions(sd)
		if e != nil {
			return nil, e
		}
		mask := *sd
		mask.Dict = types.Dict{}
		for k, v := range sd.Dict {
			mask.Dict[k] = v
		}
		isMask, e := r.imageBoolParam(mask.Dict, "ImageMask", false)
		if e != nil || !isMask {
			return nil, fmt.Errorf("explicit mask must be an image mask")
		}
		if mask.Dict["Mask"] != nil || mask.Dict["SMask"] != nil {
			return nil, fmt.Errorf("nested explicit mask")
		}
		if e = r.preparePDFImage(&mask, mw, mh); e != nil {
			return nil, e
		}
		invert := false
		if decode, e := r.pdf.DereferenceArray(mask.Dict["Decode"]); e != nil {
			return nil, e
		} else if len(decode) > 0 {
			if len(decode) != 2 {
				return nil, fmt.Errorf("invalid explicit mask Decode")
			}
			a, e := r.pdf.DereferenceNumber(decode[0])
			if e != nil {
				return nil, e
			}
			b, e := r.pdf.DereferenceNumber(decode[1])
			if e != nil {
				return nil, e
			}
			if a == 1 && b == 0 {
				invert = true
			} else if a != 0 || b != 1 {
				return nil, fmt.Errorf("invalid explicit mask Decode")
			}
		}
		row := (mw + 7) / 8
		if len(mask.Raw) != row*mh {
			return nil, fmt.Errorf("invalid explicit mask sample length")
		}
		if mw != w || mh != h {
			r.warnPage("pdf_mask_resampling", "Differently sized explicit mask is resampled using nearest neighbors")
		}
		alpha = func(x, y int) uint16 {
			mx, my := x*mw/w, y*mh/h
			v := mask.Raw[my*row+mx/8] >> uint(7-mx%8) & 1
			if invert {
				v ^= 1
			}
			if v == 0 {
				return 65535
			}
			return 0
		}
	}
	dst := image.NewNRGBA64(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		if e := r.ctx.Err(); e != nil {
			return nil, e
		}
		for x := 0; x < w; x++ {
			c := color.NRGBA64Model.Convert(pixels.At(x, y)).(color.NRGBA64)
			c.A = uint16(uint32(c.A) * uint32(alpha(x, y)) / 65535)
			dst.SetNRGBA64(x, y, c)
		}
	}
	return dst, nil
}
