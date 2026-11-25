package manifold

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// ST_ThemeColor aliases and ST_ColorSchemeIndex values use different names
// from the DrawingML palette. Mapping targets name palette slots directly;
// do not recursively remap them (swapping two accents is valid).
func docxPaletteSlot(name string) string {
	switch name {
	case "dark1", "text1":
		return "dk1"
	case "light1", "background1":
		return "lt1"
	case "dark2", "text2":
		return "dk2"
	case "light2", "background2":
		return "lt2"
	case "hyperlink":
		return "hlink"
	case "followedHyperlink":
		return "folHlink"
	case "accent1", "accent2", "accent3", "accent4", "accent5", "accent6":
		return name
	}
	return ""
}
func docxMappingKey(name string) string {
	switch name {
	case "text1":
		return "t1"
	case "text2":
		return "t2"
	case "background1":
		return "bg1"
	case "background2":
		return "bg2"
	case "accent1", "accent2", "accent3", "accent4", "accent5", "accent6", "hyperlink", "followedHyperlink":
		return name
	}
	return ""
}
func (r *docxReader) readColorMapping(settings *xmlNode) error {
	for _, child := range settings.Children {
		if err := r.ctx.Err(); err != nil {
			return err
		}
		if child.Name.Local != "clrSchemeMapping" {
			continue
		}
		if r.colorMapping != nil {
			return fmt.Errorf("duplicate DOCX color scheme mapping")
		}
		mapping := map[string]string{}
		for _, attr := range child.Attr {
			if attr.Name.Space != wordNS && attr.Name.Space != strictWordNS {
				continue
			}
			switch attr.Name.Local {
			case "t1", "t2", "bg1", "bg2", "accent1", "accent2", "accent3", "accent4", "accent5", "accent6", "hyperlink", "followedHyperlink":
				if _, duplicate := mapping[attr.Name.Local]; duplicate {
					return fmt.Errorf("duplicate DOCX color mapping attribute")
				}
				if attr.Value == "text1" || attr.Value == "text2" || attr.Value == "background1" || attr.Value == "background2" || docxPaletteSlot(attr.Value) == "" {
					return fmt.Errorf("invalid DOCX color mapping target %q", attr.Value)
				}
				mapping[attr.Name.Local] = attr.Value
			default:
				return fmt.Errorf("%w: DOCX color mapping attribute %s", ErrUnsupported, attr.Name.Local)
			}
		}
		r.colorMapping = mapping
	}
	return nil
}
func docxRGB(value string) ([3]float64, error) {
	var rgb [3]float64
	if len(value) != 6 {
		return rgb, fmt.Errorf("invalid DOCX RGB color")
	}
	if !docxHexDigits(value) {
		return rgb, fmt.Errorf("invalid DOCX RGB color")
	}
	for i := range rgb {
		n, e := strconv.ParseUint(value[i*2:i*2+2], 16, 8)
		if e != nil {
			return rgb, fmt.Errorf("invalid DOCX RGB color")
		}
		rgb[i] = float64(n) / 255
	}
	return rgb, nil
}
func docxHex(rgb [3]float64) string {
	return fmt.Sprintf("%02X%02X%02X", int(math.Round(rgb[0]*255)), int(math.Round(rgb[1]*255)), int(math.Round(rgb[2]*255)))
}
func docxThemeByte(value string) (float64, error) {
	if len(value) != 2 || !docxHexDigits(value) {
		return 0, fmt.Errorf("DOCX theme tint/shade must be one hex byte")
	}
	n, e := strconv.ParseUint(value, 16, 8)
	if e != nil {
		return 0, fmt.Errorf("DOCX theme tint/shade must be one hex byte")
	}
	return float64(n) / 255, nil
}

func docxHexDigits(value string) bool {
	for _, c := range value {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F') {
			return false
		}
	}
	return true
}

// Word run colors use HSL luminance, unlike DrawingML component tint/shade.
// See MS-OI29500 2.1.72: when both are specified, themeTint takes precedence.
func docxLuminance(rgb [3]float64, factor float64, tint bool) [3]float64 {
	lo, hi := min(rgb[0], rgb[1], rgb[2]), max(rgb[0], rgb[1], rgb[2])
	old := (lo + hi) / 2
	next := old * factor
	if tint {
		next += 1 - factor
	}
	if hi == lo {
		return [3]float64{next, next, next}
	}
	// Retain hue/saturation by scaling the chroma around the new luminance.
	// RGB = L + (component-L) * newChroma/oldChroma.
	scale := next / old
	if old > .5 {
		scale = next / (1 - old)
	}
	if next > .5 {
		scale = (1 - next) / old
		if old > .5 {
			scale = (1 - next) / (1 - old)
		}
	}
	for i, v := range rgb {
		rgb[i] = math.Max(0, math.Min(1, next+(v-old)*scale))
	}
	return rgb
}
func (r *docxReader) themePaletteColor(theme string) (string, error) {
	slot := docxPaletteSlot(theme)
	if slot == "" {
		return "", fmt.Errorf("invalid DOCX theme color %q", theme)
	}
	if mapped := r.colorMapping[docxMappingKey(theme)]; mapped != "" {
		slot = docxPaletteSlot(mapped)
	}
	palette := r.theme.child("themeElements").child("clrScheme")
	if palette == nil {
		return "", nil
	}
	var color *xmlNode
	for _, p := range palette.Children {
		if p.Name.Local == slot {
			if color != nil {
				return "", fmt.Errorf("duplicate DOCX theme palette slot")
			}
			color = p
		}
	}
	if color == nil {
		return "", nil
	}
	if len(color.Children) != 1 {
		return "", fmt.Errorf("%w: DOCX palette color choice", ErrUnsupported)
	}
	choice := color.Children[0]
	if len(choice.Children) > 0 {
		return "", fmt.Errorf("%w: DrawingML palette color transforms", ErrUnsupported)
	}
	switch choice.Name.Local {
	case "srgbClr":
		if _, err := docxRGB(choice.attr("val")); err != nil {
			return "", err
		}
		return strings.ToUpper(choice.attr("val")), nil
	case "sysClr":
		value := choice.attr("lastClr")
		if value == "" {
			return "", nil
		}
		if _, err := docxRGB(value); err != nil {
			return "", err
		}
		r.doc.warn("color_system", "System theme colors use their stored lastClr; platform-dependent system color binding is not retained")
		return strings.ToUpper(value), nil
	default:
		return "", fmt.Errorf("%w: DrawingML palette color %s", ErrUnsupported, choice.Name.Local)
	}
}

func (r *docxReader) textColor(p *xmlNode) (string, error) {
	value := p.attr("val")
	if value != "" && value != "auto" {
		if _, err := docxRGB(value); err != nil {
			return "", err
		}
	}
	tint, shade := p.attr("themeTint"), p.attr("themeShade")
	var tintFactor, shadeFactor float64
	var err error
	if tint != "" {
		if tintFactor, err = docxThemeByte(tint); err != nil {
			return "", err
		}
	}
	if shade != "" {
		if shadeFactor, err = docxThemeByte(shade); err != nil {
			return "", err
		}
	}
	theme := p.attr("themeColor")
	if theme != "" && theme != "none" {
		if docxPaletteSlot(theme) == "" {
			return "", fmt.Errorf("invalid DOCX theme color %q", theme)
		}
		resolved, err := r.themePaletteColor(theme)
		if err != nil && !errors.Is(err, ErrUnsupported) {
			return "", err
		}
		if err != nil || resolved == "" {
			// A cached value already includes the producer's tint/shade calculation.
			// Never transform it a second time or use auto as recovered theme RGB.
			if _, cacheErr := docxRGB(value); cacheErr != nil {
				return "", fmt.Errorf("%w: unresolved DOCX theme color without usable RGB cache", ErrUnsupported)
			}
			r.doc.warn("color_theme_cache", "Unresolved/unsupported theme color uses the stored final RGB cache; its current theme appearance cannot be verified")
		} else {
			rgb, _ := docxRGB(resolved)
			if tint != "" {
				rgb = docxLuminance(rgb, tintFactor, true)
			} else if shade != "" {
				rgb = docxLuminance(rgb, shadeFactor, false)
			}
			calculated := docxHex(rgb)
			if value != "" && value != "auto" && !strings.EqualFold(value, calculated) {
				r.doc.warn("color_theme_cache_mismatch", "Stored theme RGB differs from calculated current theme color; calculated RGB is used (stale cache or producer rounding may differ)")
			}
			value = calculated
		}
		r.doc.warn("color_theme", "Theme color resolves to RGB; editable theme binding is not retained")
	} else if tint != "" || shade != "" {
		r.doc.warn("color_theme_unused", "Tint/shade without an active theme color has no effect; its unused binding is not retained")
	}
	if value == "auto" {
		return "auto", nil
	}
	if _, err := docxRGB(value); err != nil {
		return "", err
	}
	return "#" + strings.ToUpper(value), nil
}
