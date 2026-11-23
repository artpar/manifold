package manydoc

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"io"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

var inlineKeys = map[string]string{"W": "Width", "H": "Height", "BPC": "BitsPerComponent", "CS": "ColorSpace", "D": "Decode", "DP": "DecodeParms", "F": "Filter", "IM": "ImageMask", "I": "Interpolate"}
var inlineNames = map[string]string{"G": "DeviceGray", "RGB": "DeviceRGB", "CMYK": "DeviceCMYK", "I": "Indexed", "AHx": "ASCIIHexDecode", "A85": "ASCII85Decode", "LZW": "LZWDecode", "Fl": "FlateDecode", "RL": "RunLengthDecode", "CCF": "CCITTFaxDecode", "DCT": "DCTDecode"}

func expandInlineName(v types.Object) types.Object {
	if n, ok := v.(types.Name); ok {
		if full, yes := inlineNames[string(n)]; yes {
			return types.Name(full)
		}
	}
	return v
}
func (r *pdfReader) inlineImage(input *string, resources types.Dict, state pdfTextState) error {
	d := types.Dict{"Type": types.Name("XObject"), "Subtype": types.Name("Image")}
	for {
		key, e := nextPDFToken(r.ctx, input)
		if e != nil {
			return e
		}
		if key == pdfOperator("ID") {
			break
		}
		name, e := pdfName(key)
		if e != nil {
			return fmt.Errorf("invalid inline-image key: %w", e)
		}
		if full, ok := inlineKeys[name]; ok {
			name = full
		}
		if _, exists := d[name]; exists {
			return fmt.Errorf("duplicate inline-image key %s", name)
		}
		value, e := nextPDFToken(r.ctx, input)
		if e != nil {
			return e
		}
		obj, ok := value.(types.Object)
		if !ok || obj == nil {
			return fmt.Errorf("invalid inline-image value")
		}
		if name == "ColorSpace" || name == "Filter" {
			if arr, ok := obj.(types.Array); ok {
				for i, v := range arr {
					arr[i] = expandInlineName(v)
				}
			} else {
				obj = expandInlineName(obj)
			}
		}
		d[name] = obj
	}
	if len(*input) == 0 || !pdfSpace((*input)[0]) {
		return fmt.Errorf("inline-image ID requires whitespace")
	}
	if strings.HasPrefix(*input, "\r\n") {
		*input = (*input)[2:]
	} else {
		*input = (*input)[1:]
	}
	sd := types.StreamDict{Dict: d}
	w, h, e := r.imageDimensions(&sd)
	if e != nil {
		return e
	}
	filters := types.Array{}
	if f := d["Filter"]; f != nil {
		if a, ok := f.(types.Array); ok {
			filters = a
		} else {
			filters = append(filters, f)
		}
	}
	parms := types.Array{}
	if p := d["DecodeParms"]; p != nil {
		if a, ok := p.(types.Array); ok {
			parms = a
		} else {
			parms = append(parms, p)
		}
	}
	if len(parms) > 0 && len(parms) != len(filters) {
		return fmt.Errorf("inline-image DecodeParms length mismatch")
	}
	for i, f := range filters {
		name, e := pdfName(f)
		if e != nil {
			return e
		}
		var dp types.Dict
		if len(parms) > 0 && parms[i] != nil {
			var ok bool
			dp, ok = parms[i].(types.Dict)
			if !ok {
				return fmt.Errorf("invalid inline-image DecodeParms")
			}
		}
		sd.FilterPipeline = append(sd.FilterPipeline, types.PDFFilter{Name: name, DecodeParms: dp})
	}
	n := 0
	data := []byte(*input)
	if len(filters) == 0 {
		components := 0
		switch d["ColorSpace"] {
		case types.Name("DeviceGray"):
			components = 1
		case types.Name("DeviceRGB"):
			components = 3
		case types.Name("DeviceCMYK"):
			components = 4
		}
		if a, ok := d["ColorSpace"].(types.Array); ok && len(a) > 0 && a[0] == types.Name("Indexed") {
			components = 1
		}
		bpc, e := r.pdf.DereferenceInteger(d["BitsPerComponent"])
		if e != nil {
			return e
		}
		bits := 0
		if bpc != nil {
			bits = int(*bpc)
		}
		if mask := d.BooleanEntry("ImageMask"); mask != nil && *mask {
			components = 1
			bits = 1
		}
		if components == 0 || (bits != 1 && bits != 2 && bits != 4 && bits != 8 && bits != 16) {
			return fmt.Errorf("%w: inline-image raw sample format", ErrUnsupported)
		}
		n = ((w*components*bits + 7) / 8) * h
	} else {
		switch sd.FilterPipeline[0].Name {
		case "FlateDecode":
			reader := bytes.NewReader(data)
			z, e := zlib.NewReader(reader)
			if e != nil {
				return e
			}
			count, e := io.Copy(io.Discard, io.LimitReader(z, 200_000_001))
			closeErr := z.Close()
			if e != nil {
				return e
			}
			if closeErr != nil {
				return closeErr
			}
			if count > 200_000_000 {
				return fmt.Errorf("inline-image decoded size exceeds limit")
			}
			n = len(data) - reader.Len()
		case "ASCIIHexDecode":
			n = strings.IndexByte(*input, '>') + 1
		case "ASCII85Decode":
			if end := strings.Index(*input, "~>"); end >= 0 {
				n = end + 2
			}
		case "RunLengthDecode":
			terminated := false
			for n < len(data) {
				b := data[n]
				n++
				if b == 128 {
					terminated = true
					break
				}
				if b < 128 {
					n += int(b) + 1
				} else {
					n++
				}
				if n > len(data) {
					return fmt.Errorf("truncated inline RLE image")
				}
			}
			if !terminated {
				return fmt.Errorf("unterminated inline RLE image")
			}
		case "DCTDecode":
			var e error
			n, e = inlineJPEGLength(data)
			if e != nil {
				return e
			}
		default:
			return fmt.Errorf("%w: inline-image boundary for filter %s", ErrUnsupported, sd.FilterPipeline[0].Name)
		}
	}
	if n <= 0 || n > len(data) {
		return fmt.Errorf("truncated inline-image data")
	}
	sd.Raw = data[:n]
	length := int64(n)
	sd.StreamLength = &length
	sd.Dict["Length"] = types.Integer(n)
	*input = (*input)[n:]
	if len(*input) == 0 || !pdfSpace((*input)[0]) {
		return fmt.Errorf("inline image missing EI delimiter")
	}
	end, e := nextPDFToken(r.ctx, input)
	if e != nil {
		return e
	}
	if end != pdfOperator("EI") {
		return fmt.Errorf("inline image missing EI")
	}
	return r.imageXObject(&sd, types.Name(fmt.Sprintf("inline:%s:%d", r.stream, r.order+1)), resources, state)
}

// JPEG entropy data may contain escaped FF bytes and restart markers. Segment
// lengths, rather than an EI byte search, identify the actual image boundary.
func inlineJPEGLength(data []byte) (int, error) {
	if len(data) < 2 || data[0] != 255 || data[1] != 216 {
		return 0, fmt.Errorf("invalid inline JPEG SOI")
	}
	for i := 2; i < len(data); {
		if data[i] != 255 {
			i++
			continue
		}
		i++
		for i < len(data) && data[i] == 255 {
			i++
		}
		if i >= len(data) {
			break
		}
		marker := data[i]
		i++
		if marker == 0 || marker == 1 || (marker >= 208 && marker <= 215) {
			continue
		}
		if marker == 217 {
			return i, nil
		}
		if i+2 > len(data) {
			break
		}
		length := int(data[i])<<8 | int(data[i+1])
		if length < 2 || length > len(data)-i {
			return 0, fmt.Errorf("invalid inline JPEG segment length")
		}
		i += length
	}
	return 0, fmt.Errorf("inline JPEG missing EOI")
}
