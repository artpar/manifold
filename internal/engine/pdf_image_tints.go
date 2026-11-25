package engine

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"math"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// ISO 32000-1 Separation + type-2 exponential tint function. Apply source Decode
// to the single component BEFORE evaluating the declared alternate-space tint.
func (r *pdfReader) separationImage(sd *types.StreamDict, w, h int) (image.Image, bool, error) {
	cs, ok := sd.Dict["ColorSpace"].(types.Array)
	if !ok || len(cs) < 4 || len(cs) > 5 || (cs[0] != types.Name("Separation") && cs[0] != types.Name("DeviceN")) {
		return nil, false, nil
	}
	if cs[0] == types.Name("DeviceN") {
		names, err := r.pdf.DereferenceArray(cs[1])
		if err != nil || len(names) != 1 {
			return nil, false, nil
		}
	}
	n := 0
	switch cs[2] {
	case types.Name("DeviceGray"):
		n = 1
	case types.Name("DeviceRGB"):
		n = 3
	case types.Name("DeviceCMYK"):
		n = 4
	}
	if n == 0 {
		return nil, false, nil
	}
	fnObj, e := r.pdf.Dereference(cs[3])
	if e != nil {
		return nil, true, e
	}
	var fn types.Dict
	var calculator []byte
	switch v := fnObj.(type) {
	case types.Dict:
		fn = v
	case types.StreamDict:
		fn = v.Dict
		calculator, e = r.decodePDFStream(cs[3], &v)
		if e != nil {
			return nil, true, e
		}
	}

	if e != nil {
		return nil, true, e
	}
	if fn == nil || fn.IntEntry("FunctionType") == nil || (*fn.IntEntry("FunctionType") != 2 && *fn.IntEntry("FunctionType") != 4) {
		return nil, false, nil
	}
	numbers := func(obj types.Object, size int, defaults []float64) ([]float64, error) {
		if obj == nil {
			return defaults, nil
		}
		a, e := r.pdf.DereferenceArray(obj)
		if e != nil || len(a) != size {
			return nil, fmt.Errorf("invalid tint function array")
		}
		out := make([]float64, size)
		for i, o := range a {
			out[i], e = r.pdf.DereferenceNumber(o)
			if e != nil || !finite(out[i]) {
				return nil, fmt.Errorf("invalid tint function number")
			}
		}
		return out, nil
	}
	domain, e := numbers(fn["Domain"], 2, nil)
	if e != nil || len(domain) != 2 || domain[0] > domain[1] {
		return nil, true, fmt.Errorf("invalid tint function Domain")
	}
	var c0, c1 []float64
	var exponent float64
	if *fn.IntEntry("FunctionType") == 2 {
		c0, e = numbers(fn["C0"], n, []float64{0})
		if e != nil {
			return nil, true, e
		}
		c1, e = numbers(fn["C1"], n, []float64{1})
		if e != nil || len(c0) != n || len(c1) != n {
			return nil, true, fmt.Errorf("invalid tint component count")
		}
		exponent, e = r.pdf.DereferenceNumber(fn["N"])
		if e != nil || !finite(exponent) || exponent < 0 {
			return nil, true, fmt.Errorf("invalid tint exponent")
		}
	}
	ranges, e := numbers(fn["Range"], 2*n, nil)
	if e != nil {
		return nil, true, e
	}
	for i := 0; i < len(ranges); i += 2 {
		if ranges[i] > ranges[i+1] {
			return nil, true, fmt.Errorf("invalid tint Range")
		}
	}
	decode, e := numbers(sd.Dict["Decode"], 2, []float64{0, 1})
	if e != nil {
		return nil, true, e
	}
	bits := int(*sd.IntEntry("BitsPerComponent"))
	var sample func(int, int) float64
	if len(sd.FilterPipeline) > 0 {
		if len(sd.FilterPipeline) != 1 || sd.FilterPipeline[0].Name != "DCTDecode" {
			return nil, false, nil
		}
		im, e := jpeg.Decode(bytes.NewReader(sd.Raw))
		if e != nil {
			return nil, true, e
		}
		gray, ok := im.(*image.Gray)
		if !ok {
			return nil, true, fmt.Errorf("Separation JPEG has more than one component")
		}
		sample = func(x, y int) float64 { return float64(gray.GrayAt(x, y).Y) / 255 }
	} else {
		row := (w*bits + 7) / 8
		if len(sd.Raw) < row*h {
			return nil, true, fmt.Errorf("truncated tint samples")
		}
		sample = func(x, y int) float64 {
			var v uint32
			for i := 0; i < bits; i++ {
				bit := x*bits + i
				v = v<<1 | uint32(sd.Raw[y*row+bit/8]>>uint(7-bit%8)&1)
			}
			return float64(v) / float64((uint32(1)<<uint(bits))-1)
		}
	}
	var tint func(float64) ([]float64, error)
	if *fn.IntEntry("FunctionType") == 4 {
		tint = func(t float64) ([]float64, error) { return pdfCalculatorTint(calculator, t, n) }
	} else {
		tint = func(t float64) ([]float64, error) {
			power := math.Pow(t, exponent)
			if !finite(power) {
				return nil, fmt.Errorf("undefined tint exponent")
			}
			values := make([]float64, n)
			for i := range values {
				values[i] = c0[i] + power*(c1[i]-c0[i])
			}
			return values, nil
		}
	}
	cache := map[float64][]float64{}
	dst := image.NewNRGBA64(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		if e := r.ctx.Err(); e != nil {
			return nil, true, e
		}
		for x := 0; x < w; x++ {
			t := sample(x, y)
			t = decode[0] + t*(decode[1]-decode[0])
			t = max(domain[0], min(domain[1], t))
			components, ok := cache[t]
			if !ok {
				components, e = tint(t)
				if e != nil {
					return nil, true, e
				}
				if len(cache) < 65536 {
					cache[t] = components
				}
			}
			var values [4]uint16
			for i := 0; i < n; i++ {
				v := components[i]
				if len(ranges) > 0 {
					v = max(ranges[2*i], min(ranges[2*i+1], v))
				}
				values[i] = uint16(math.Round(max(0, min(1, v)) * 65535))
			}
			red, green, blue := values[0], values[0], values[0]
			if n == 3 {
				green, blue = values[1], values[2]
			} else if n == 4 {
				k := uint32(65535 - values[3])
				red = uint16(uint32(65535-values[0]) * k / 65535)
				green = uint16(uint32(65535-values[1]) * k / 65535)
				blue = uint16(uint32(65535-values[2]) * k / 65535)
			}
			dst.SetNRGBA64(x, y, color.NRGBA64{red, green, blue, 65535})
		}
	}
	return dst, true, nil
}
