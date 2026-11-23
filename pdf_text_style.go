package manydoc

import (
	"bytes"
	"compress/zlib"
	"context"
	"fmt"
	"io"
	"math"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
	"golang.org/x/image/font/sfnt"
)

func pdfSubsetName(name string) string {
	if len(name) > 7 && name[6] == '+' {
		valid := true
		for _, c := range name[:6] {
			valid = valid && c >= 'A' && c <= 'Z'
		}
		if valid {
			return name[7:]
		}
	}
	return name
}
func (r *pdfReader) fontFamily(f *pdfFont, descriptor types.Dict) {
	accept := func(value, evidence string) bool {
		value = strings.TrimSpace(value)
		if value == "" || validateTextStyle(Run{FontFamily: value}) != nil {
			return false
		}
		f.family, f.familyEvidence = value, evidence
		return true
	}
	if value := descriptor["FontFamily"]; value != nil {
		obj, err := r.pdf.Dereference(value)
		if err == nil {
			if text, err := pdfTextString(obj); err == nil && accept(text, "descriptor") {
				return
			}
		}
		r.doc.warn("pdf_font_family", "Invalid font-family metadata is ignored while text and font identity are retained")
	}
	for _, key := range []string{"FontFile2", "FontFile3"} {
		if descriptor[key] == nil {
			continue
		}
		stream, _, err := r.pdf.DereferenceStreamDict(descriptor[key])
		if err != nil || stream == nil {
			r.doc.warn("pdf_font_family", "Embedded font metadata is unavailable; a face-name fallback may be needed")
			continue
		}
		data, err := boundedFontMetadata(r.ctx, stream)
		if err == nil {
			var font *sfnt.Font
			font, err = sfnt.Parse(data)
			if err == nil {
				var buffer sfnt.Buffer
				for _, id := range []sfnt.NameID{16, 1} {
					name, e := font.Name(&buffer, id)
					if e == nil && accept(name, "embedded_name") {
						return
					}
				}
			}
		}
		r.doc.warn("pdf_font_family", "Embedded font family could not be recovered within supported metadata bounds; text is retained")
	}
	name := pdfSubsetName(f.name)
	family := ""
	switch name {
	case "Helvetica", "Helvetica-Bold", "Helvetica-Oblique", "Helvetica-BoldOblique":
		family = "Helvetica"
	case "Times-Roman", "Times-Bold", "Times-Italic", "Times-BoldItalic":
		family = "Times"
	case "Courier", "Courier-Bold", "Courier-Oblique", "Courier-BoldOblique":
		family = "Courier"
	case "Symbol", "ZapfDingbats":
		family = name
	}
	if family != "" && accept(family, "core_font") {
		return
	}
	if name != "unknown" && accept(name, "postscript_fallback") {
		r.doc.warn("pdf_font_family_inferred", "A PostScript face name is used as a family lookup identifier; its family grouping is uncertain")
		return
	}
	f.familyEvidence = "unresolved"
	r.doc.warn("pdf_font_family", "Font family is unresolved; font identity and content remain in source evidence")
}

const maxFontMetadataBytes = 32 << 20

func boundedFontMetadata(ctx context.Context, s *types.StreamDict) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s.Content != nil {
		if len(s.Content) > maxFontMetadataBytes {
			return nil, fmt.Errorf("font metadata exceeds bound")
		}
		return s.Content, nil
	}
	if len(s.FilterPipeline) == 0 {
		if len(s.Raw) > maxFontMetadataBytes {
			return nil, fmt.Errorf("font metadata exceeds bound")
		}
		return s.Raw, nil
	}
	if len(s.FilterPipeline) != 1 || s.FilterPipeline[0].Name != "FlateDecode" || len(s.FilterPipeline[0].DecodeParms) > 0 {
		return nil, fmt.Errorf("unsupported font metadata filter")
	}
	z, err := zlib.NewReader(bytes.NewReader(s.Raw))
	if err != nil {
		return nil, err
	}
	defer z.Close()
	data, err := io.ReadAll(io.LimitReader(contextReader{ctx, z}, maxFontMetadataBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxFontMetadataBytes {
		return nil, fmt.Errorf("font metadata exceeds bound")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return data, nil
}

func (r *pdfReader) textStyle(s *pdfTextState, source *SourceInfo) Run {
	run := Run{Bold: s.font.bold, Italic: s.font.italic, FontFamily: s.font.family, FontSize: source.FontSize}
	source.FontFamilyEvidence = s.font.familyEvidence
	space, values, unknown := s.fillSpace, s.fillValues, s.fillUnknown
	if s.renderMode == 1 || s.renderMode == 5 {
		space, values, unknown = s.strokeSpace, s.strokeValues, s.strokeUnknown
	}
	if s.renderMode != 0 {
		r.warnPage("pdf_text_paint", "Text render mode is retained as source evidence; stroke/clip/invisible appearance is not an editable text property")
	}
	if s.rulingEffects || s.rulingFillAlpha != 1 || s.rulingStrokeAlpha != 1 {
		r.warnPage("pdf_text_paint", "Base text paint is retained; opacity/blend/mask effects are not represented by opaque RGB")
	}
	if !unknown {
		count := map[string]int{"DeviceGray": 1, "DeviceRGB": 3, "DeviceCMYK": 4}[space]
		if count > 0 {
			source.ColorSpace = space
			source.ColorComponents = append([]float64(nil), values[:count]...)
			rgb := colorRGB(space, values)
			run.Color = fmt.Sprintf("#%02X%02X%02X", int(math.Round(rgb[0]*255)), int(math.Round(rgb[1]*255)), int(math.Round(rgb[2]*255)))
			if space == "DeviceCMYK" {
				r.warnPage("pdf_text_color_approximation", "DeviceCMYK text uses an approximate RGB conversion; original components remain in source evidence")
			}
			if space != "DeviceCMYK" {
				for _, v := range rgb {
					if math.Abs(v*255-math.Round(v*255)) > 1e-7 {
						r.warnPage("pdf_text_color_quantized", "Text color is quantized to 8-bit RGB; original device components remain in source evidence")
						break
					}
				}
			}
		}
	} else {
		r.warnPage("pdf_text_color", "Unresolved text color is omitted from editable style; it must not be treated as a recovered black color")
	}
	if source.Matrix != nil {
		m := *source.Matrix
		x, y := math.Hypot(m[0], m[1]), math.Hypot(m[2], m[3])
		if math.Abs(x-y) > 1e-6*math.Max(1, y) || math.Abs(m[0]*m[2]+m[1]*m[3]) > 1e-6*math.Max(1, x*y) {
			r.warnPage("pdf_text_transform", "Point size retains vertical scale; nonuniform scaling/skew remains in source geometry and is not editable font geometry")
		}
	}
	return run
}
func colorRGB(space string, v [4]float64) [3]float64 {
	switch space {
	case "DeviceGray":
		return [3]float64{v[0], v[0], v[0]}
	case "DeviceCMYK":
		return [3]float64{1 - math.Min(1, v[0]+v[3]), 1 - math.Min(1, v[1]+v[3]), 1 - math.Min(1, v[2]+v[3])}
	}
	return [3]float64{v[0], v[1], v[2]}
}

// Retain only values shared by all contributors to an unmappable replacement.
func commonTextStyle(a, b Run) (Run, bool) {
	mixed := a.Bold != b.Bold || a.Italic != b.Italic || a.Underline != b.Underline || a.Strike != b.Strike || a.Link != b.Link || a.FontFamily != b.FontFamily || math.Abs(a.FontSize-b.FontSize) > 1e-7 || a.Color != b.Color
	a.Bold = a.Bold && b.Bold
	a.Italic = a.Italic && b.Italic
	a.Underline = a.Underline && b.Underline
	a.Strike = a.Strike && b.Strike
	if a.Link != b.Link {
		a.Link = ""
	}
	if a.FontFamily != b.FontFamily {
		a.FontFamily = ""
	}
	if math.Abs(a.FontSize-b.FontSize) > 1e-7 {
		a.FontSize = 0
	}
	if a.Color != b.Color {
		a.Color = ""
	}
	return a, mixed
}
func sameSourcePaint(a, b *SourceInfo) bool {
	if a == nil || b == nil || a.ColorSpace != b.ColorSpace || len(a.ColorComponents) != len(b.ColorComponents) {
		return false
	}
	for i, v := range a.ColorComponents {
		if math.Abs(v-b.ColorComponents[i]) > 1e-9 {
			return false
		}
	}
	return true
}

func (r *pdfReader) deviceColorSpace(value types.Object, resources types.Dict) (string, error) {
	seen := map[string]bool{}
	for depth := 0; depth < 64; depth++ {
		if err := r.ctx.Err(); err != nil {
			return "", err
		}
		obj, err := r.pdf.Dereference(value)
		if err != nil {
			return "", err
		}
		name, ok := obj.(types.Name)
		if !ok {
			return "", nil
		}
		key := string(name)
		switch key {
		case "DeviceGray", "DeviceRGB", "DeviceCMYK":
			return key, nil
		case "Pattern":
			return "", nil
		}
		if seen[key] {
			return "", fmt.Errorf("cyclic PDF color space")
		}
		seen[key] = true
		spaces, err := r.pdf.DereferenceDict(resources["ColorSpace"])
		if err != nil {
			return "", err
		}
		value = spaces[key]
		if value == nil {
			return "", nil
		}
	}
	return "", fmt.Errorf("PDF color space nesting exceeds limit")
}
func (r *pdfReader) selectTextColorSpace(args []any, resources types.Dict, s *pdfTextState, stroke bool) error {
	if len(args) != 1 {
		return fmt.Errorf("PDF color space requires one operand")
	}
	if _, ok := args[0].(types.Name); !ok {
		return fmt.Errorf("invalid PDF color space name")
	}
	space, err := r.deviceColorSpace(args[0].(types.Name), resources)
	if err != nil {
		return err
	}
	values := [4]float64{}
	if space == "DeviceCMYK" {
		values[3] = 1
	}
	if stroke {
		s.strokeSpace, s.strokeValues, s.strokeUnknown = space, values, space == ""
	} else {
		s.fillSpace, s.fillValues, s.fillUnknown = space, values, space == ""
		s.fill = colorRGB(space, values)
	}
	if space == "" {
		r.warnPage("pdf_color_state", "General color space is unresolved; editable text color is omitted and color-dependent stencils may reject")
	}
	return nil
}
func setTextColorValues(args []any, s *pdfTextState, stroke bool) error {
	space := s.fillSpace
	if stroke {
		space = s.strokeSpace
	}
	count := map[string]int{"DeviceGray": 1, "DeviceRGB": 3, "DeviceCMYK": 4}[space]
	if count == 0 {
		return nil
	}
	if len(args) != count {
		return fmt.Errorf("wrong device color operand count")
	}
	values := [4]float64{}
	for i, v := range args {
		n, err := pdfNumber(v)
		if err != nil || !finite(n) {
			return fmt.Errorf("invalid device color component")
		}
		values[i] = math.Max(0, math.Min(1, n))
	}
	if stroke {
		s.strokeValues = values
		s.strokeUnknown = false
	} else {
		s.fillValues = values
		s.fillUnknown = false
		s.fill = colorRGB(space, values)
	}
	return nil
}
