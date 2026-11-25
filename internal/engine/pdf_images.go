package engine

import (
	"bytes"
	"fmt"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
	_ "golang.org/x/image/tiff"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
)

// Bound image allocations before asking a decoder to expand untrusted pixels.
const maxPDFImagePixels = maxOutputImagePixels

func (r *pdfReader) imageDimensions(sd *types.StreamDict) (int, int, error) {
	w, e := r.pdf.DereferenceInteger(sd.Dict["Width"])
	if e != nil {
		return 0, 0, e
	}
	h, e := r.pdf.DereferenceInteger(sd.Dict["Height"])
	if e != nil {
		return 0, 0, e
	}
	if w == nil || h == nil || *w <= 0 || *h <= 0 || int64(*w) > maxPDFImagePixels/int64(*h) {
		return 0, 0, fmt.Errorf("invalid or excessive PDF image dimensions")
	}
	return int(*w), int(*h), nil
}
func (r *pdfReader) imageXObject(sd *types.StreamDict, object types.Object, resources types.Dict, state pdfTextState) error {
	if state.rulingClip != nil || state.rulingClipUnknown {
		r.warnPage("pdf_clipping", "Image clipping is not applied to recovered raster pixels or bounds")
	}
	w, h, err := r.imageDimensions(sd)
	if err != nil {
		return err
	}
	copySD := *sd
	copySD.Dict = types.Dict{}
	for k, v := range sd.Dict {
		copySD.Dict[k] = v
	}
	// Resolve a resource-local color space before handing the stream to pdfcpu.
	if name, ok := copySD.Dict["ColorSpace"].(types.Name); ok && name != "DeviceRGB" && name != "DeviceGray" && name != "DeviceCMYK" {
		spaces, e := r.pdf.DereferenceDict(resources["ColorSpace"])
		if e != nil {
			return e
		}
		if spaces[string(name)] == nil {
			return fmt.Errorf("missing image color space %s", name)
		}
		copySD.Dict["ColorSpace"] = spaces[string(name)]
	}
	if cs := copySD.Dict["ColorSpace"]; cs != nil {
		copySD.Dict["ColorSpace"], err = r.preparePDFColorSpace(cs, 0)
		if err != nil {
			return err
		}
	}
	if err := r.preparePDFImage(&copySD, w, h); err != nil {
		return err
	}

	explicit := copySD.Dict["Mask"]
	delete(copySD.Dict, "Mask")
	soft := copySD.Dict["SMask"]
	delete(copySD.Dict, "SMask")
	pixels, separated, err := r.separationImage(&copySD, w, h)
	if err != nil {
		return err
	}
	var dctDecode []float64
	if len(copySD.FilterPipeline) > 0 && copySD.FilterPipeline[len(copySD.FilterPipeline)-1].Name == "DCTDecode" && copySD.Dict["Decode"] != nil && !separated {
		a, e := r.pdf.DereferenceArray(copySD.Dict["Decode"])
		if e != nil {
			return e
		}
		cs := copySD.Dict["ColorSpace"]
		components := 0
		if cs == types.Name("DeviceRGB") {
			components = 3
		}
		if cs == types.Name("DeviceGray") {
			components = 1
		}
		if cs == types.Name("DeviceCMYK") {
			components = 4
		}
		if components == 0 || len(a) != 2*components {
			return fmt.Errorf("%w: DCT Decode for this color space", ErrUnsupported)
		}
		for _, v := range a {
			n, e := pdfNumber(v)
			if e != nil || !finite(n) {
				return fmt.Errorf("invalid image Decode")
			}
			dctDecode = append(dctDecode, n)
		}
		// pdfcpu's DCT CMYK decoder applies Decode in CMYK component space,
		// before RGB conversion (including Adobe JPEG sample inversion).
		if components == 4 {
			dctDecode = nil
		}
	}
	if copySD.Dict["ColorSpace"] == types.Name("DeviceCMYK") {
		r.warnPage("pdf_image_color", "Device CMYK samples are converted to RGB; original device-color semantics are not retained")
	}
	if cs, ok := copySD.Dict["ColorSpace"].(types.Array); ok && len(cs) > 0 && (cs[0] == types.Name("ICCBased") || cs[0] == types.Name("Separation") || cs[0] == types.Name("DeviceN")) {
		r.warnPage("pdf_image_color", "Image extraction uses the decoder's alternate/device color conversion; original color-profile semantics are not retained")
	}

	if separated {
		// Source Decode and tint transformation have already been applied.
	} else if stencil := copySD.BooleanEntry("ImageMask"); stencil != nil && *stencil {
		if state.fillUnknown {
			return fmt.Errorf("%w: stencil requires unresolved general fill color", ErrUnsupported)
		}
		copySD.Content = copySD.Raw
		row := (w + 7) / 8
		if len(copySD.Content) != row*h {
			return fmt.Errorf("invalid stencil image sample length")
		}
		invert := false
		if d, err := r.pdf.DereferenceArray(copySD.Dict["Decode"]); err != nil {
			return err
		} else if len(d) > 0 {
			if len(d) != 2 {
				return fmt.Errorf("invalid stencil Decode")
			}
			a, e := pdfNumber(d[0])
			if e != nil {
				return e
			}
			b, e := pdfNumber(d[1])
			if e != nil {
				return e
			}
			if a == 1 && b == 0 {
				invert = true
			} else if a != 0 || b != 1 {
				return fmt.Errorf("invalid stencil Decode")
			}
		}
		dst := image.NewNRGBA(image.Rect(0, 0, w, h))
		c := color.NRGBA{uint8(math.Round(state.fill[0] * 255)), uint8(math.Round(state.fill[1] * 255)), uint8(math.Round(state.fill[2] * 255)), 255}
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				bit := copySD.Content[y*row+x/8] >> (7 - uint(x%8)) & 1
				paint := bit == 0
				if invert {
					paint = !paint
				}
				if paint {
					dst.SetNRGBA(x, y, c)
				}
			}
		}
		pixels = dst
	} else if decoded, handled, e := r.deviceImage(&copySD, w, h); handled {
		if e != nil {
			return e
		}
		pixels = decoded
	} else {
		extracted, e := pdfcpu.ExtractImage(r.pdf, &copySD, false, "", 0, false)
		if e != nil {
			return e
		}
		if extracted == nil || extracted.Reader == nil {
			return fmt.Errorf("%w: PDF image decoding produced no asset", ErrUnsupported)
		}
		data, e := readPDFDecoded(r.ctx, extracted.Reader, maxPDFImageBytes)
		if e != nil {
			return e
		}
		if len(data) > 200_000_000 {
			return fmt.Errorf("PDF image asset exceeds limit")
		}
		cfg, _, e := image.DecodeConfig(bytes.NewReader(data))
		if e != nil {
			return fmt.Errorf("%w: PDF image codec %s: %v", ErrUnsupported, extracted.FileType, e)
		}
		if cfg.Width != w || cfg.Height != h {
			return fmt.Errorf("image stream dimensions contradict image dictionary")
		}
		pixels, _, err = image.Decode(bytes.NewReader(data))
		if err != nil {
			return err
		}
	}
	if len(dctDecode) > 0 {
		dst := image.NewNRGBA(image.Rect(0, 0, w, h))
		convert := func(value uint8, i int) uint8 {
			x := dctDecode[i] + float64(value)/255*(dctDecode[i+1]-dctDecode[i])
			return uint8(math.Round(max(0, min(1, x)) * 255))
		}
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				c := color.NRGBAModel.Convert(pixels.At(x, y)).(color.NRGBA)
				if len(dctDecode) == 2 {
					v := convert(color.GrayModel.Convert(pixels.At(x, y)).(color.Gray).Y, 0)
					c.R = v
					c.G = v
					c.B = v
				} else {
					c.R = convert(c.R, 0)
					c.G = convert(c.G, 2)
					c.B = convert(c.B, 4)
				}
				dst.SetNRGBA(x, y, c)
			}
		}
		pixels = dst
	}
	if explicit != nil && soft == nil {
		pixels, err = r.applyPDFExplicitMask(pixels, explicit, &copySD, w, h)
		if err != nil {
			return err
		}
	}
	if soft != nil {
		mask, _, e := r.pdf.DereferenceStreamDict(soft)
		if e != nil {
			return e
		}
		if mask == nil {
			return fmt.Errorf("invalid soft mask")
		}
		mw, mh, e := r.imageDimensions(mask)
		if e != nil {
			return e
		}
		matte, e := r.pdfImageMatte(mask, &copySD, w, h, mw, mh)
		if e != nil {
			return e
		}
		if cs := mask.NameEntry("ColorSpace"); cs == nil || *cs != "DeviceGray" {
			return fmt.Errorf("invalid soft-mask color space")
		}
		if mask.Dict["SMask"] != nil || mask.Dict["Mask"] != nil {
			return fmt.Errorf("invalid nested soft mask")
		}
		maskCopy := *mask
		maskCopy.Dict = types.Dict{}
		for k, v := range mask.Dict {
			maskCopy.Dict[k] = v
		}
		if e = r.preparePDFImage(&maskCopy, mw, mh); e != nil {
			return e
		}
		alpha, handled, e := r.deviceImage(&maskCopy, mw, mh)
		if e != nil {
			return e
		}
		if !handled {
			im, e := pdfcpu.ExtractImage(r.pdf, &maskCopy, false, "", 0, false)
			if e != nil {
				return e
			}
			if im == nil || im.Reader == nil {
				return fmt.Errorf("unsupported soft-mask codec")
			}
			encoded, e := readPDFDecoded(r.ctx, im.Reader, maxPDFImageBytes)
			if e != nil {
				return e
			}
			cfg, _, e := image.DecodeConfig(bytes.NewReader(encoded))
			if e != nil {
				return e
			}
			if cfg.Width != mw || cfg.Height != mh {
				return fmt.Errorf("soft-mask dimensions disagree")
			}
			alpha, _, e = image.Decode(bytes.NewReader(encoded))
			if e != nil {
				return e
			}
		}
		if alpha.Bounds().Dx() != mw || alpha.Bounds().Dy() != mh {
			return fmt.Errorf("soft-mask dimensions disagree")
		}
		if mw != w || mh != h {
			r.warnPage("pdf_mask_resampling", "Differently sized soft mask is resampled using nearest neighbors")
		}
		highDepth := func(im image.Image) bool {
			m := im.ColorModel()
			return m == color.NRGBA64Model || m == color.RGBA64Model || m == color.Gray16Model
		}
		if highDepth(pixels) || highDepth(alpha) {
			dst := image.NewNRGBA64(image.Rect(0, 0, w, h))
			for y := 0; y < h; y++ {
				if err := r.ctx.Err(); err != nil {
					return err
				}
				for x := 0; x < w; x++ {
					c := color.NRGBA64Model.Convert(pixels.At(x, y)).(color.NRGBA64)
					a := color.Gray16Model.Convert(alpha.At(x*mw/w, y*mh/h)).(color.Gray16).Y
					if len(matte) > 0 {
						c = unblendPDFMatte(c, a, matte)
					}
					c.A = uint16(uint32(c.A) * uint32(a) / 65535)
					dst.SetNRGBA64(x, y, c)
				}
			}
			pixels = dst
		} else {
			dst := image.NewNRGBA(image.Rect(0, 0, w, h))
			draw.Draw(dst, dst.Bounds(), pixels, pixels.Bounds().Min, draw.Src)
			for y := 0; y < h; y++ {
				for x := 0; x < w; x++ {
					a := color.GrayModel.Convert(alpha.At(x*mw/w, y*mh/h)).(color.Gray).Y
					offset := dst.PixOffset(x, y)
					if len(matte) > 0 {
						c := dst.NRGBAAt(x, y)
						c16 := unblendPDFMatte(color.NRGBA64{R: uint16(c.R) * 257, G: uint16(c.G) * 257, B: uint16(c.B) * 257, A: uint16(c.A) * 257}, uint16(a)*257, matte)
						dst.Pix[offset], dst.Pix[offset+1], dst.Pix[offset+2] = uint8(c16.R>>8), uint8(c16.G>>8), uint8(c16.B>>8)
					}
					dst.Pix[offset+3] = uint8(uint16(dst.Pix[offset+3]) * uint16(a) / 255)
				}
			}
			pixels = dst
		}
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, pixels); err != nil {
		return err
	}
	m := r.pageInfo.Transform.compose(state.ctm)
	q := Quad{m.point(0, 1), m.point(1, 1), m.point(1, 0), m.point(0, 0)}
	r.checkPageBounds(q)
	r.order++
	source := &SourceInfo{Format: PDF, Page: r.page, Object: r.objectID(object, "image"), Stream: r.stream, Order: r.order, Quad: &q, Matrix: &m, Structure: "unstructured", Evidence: "pdf_image_xobject", Confidence: 1, Precision: "image_unit_square"}
	r.applyMarked(source)
	r.newline()
	r.lines = append(r.lines, Block{Kind: Picture, Source: source, Image: &Image{Data: encoded.Bytes(), MediaType: "image/png", Width: math.Hypot(m[0], m[1]), Height: math.Hypot(m[2], m[3])}})
	return nil
}
