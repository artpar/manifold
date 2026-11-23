package manydoc

import (
	"fmt"

	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/gomonobold"
	"golang.org/x/image/font/gofont/gomonobolditalic"
	"golang.org/x/image/font/gofont/gomonoitalic"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

// FontFace supplies one embeddable TTF program under a lookup family name.
// Bold/Italic select its request slot; the supplied outlines determine appearance.
// The renderer does not synthesize missing variants.
type FontFace struct {
	Family       string
	Bold, Italic bool
	Data         []byte
}
type pdfFontFace struct {
	id         string
	parsed     *sfnt.Font
	style      string
	data       []byte
	registered bool
}

var pdfMonoFonts = []struct {
	style string
	data  []byte
}{{"", gomono.TTF}, {"B", gomonobold.TTF}, {"I", gomonoitalic.TTF}, {"BI", gomonobolditalic.TTF}}

func fontKey(family, style string) string {
	if family == "" {
		family = "Go"
	}
	return fontFamilyKey(family) + "/" + style
}
func (r *pdfRenderer) addFonts(extra []FontFace) error {
	r.fonts = map[string]pdfFontFace{}
	add := func(family, style string, data []byte, check bool) error {
		data = append([]byte(nil), data...)
		if err := r.ctx.Err(); err != nil {
			return err
		}
		key := fontKey(family, style)
		if _, ok := r.fonts[key]; ok {
			return fmt.Errorf("duplicate PDF font face %q %s", family, style)
		}
		parsed, err := sfnt.Parse(data)
		if err != nil {
			return fmt.Errorf("font %q: %w", family, err)
		}
		if check {
			if err := validatePDFTrueType(r.ctx, data, parsed); err != nil {
				return err
			}
		}
		id := fmt.Sprintf("manydoc%d", len(r.fonts))
		if family == "Go" {
			id = "Go"
		} // retain established default output metrics/resources
		r.fonts[key] = pdfFontFace{id: id, parsed: parsed, style: style, data: data}
		return nil
	}
	for _, f := range pdfFonts {
		if err := add("Go", f.style, f.data, false); err != nil {
			return err
		}
	}
	for _, f := range pdfMonoFonts {
		if err := add("Go Mono", f.style, f.data, false); err != nil {
			return err
		}
	}
	for _, f := range extra {
		if f.Family == "" {
			return fmt.Errorf("custom font requires family")
		}
		if err := validateTextStyle(Run{FontFamily: f.Family}); err != nil {
			return err
		}
		if err := add(f.Family, runStyle(Run{Bold: f.Bold, Italic: f.Italic}), f.Data, true); err != nil {
			return err
		}
	}
	return nil
}
func (r *pdfRenderer) font(run Run) (pdfFontFace, error) {
	seen := map[string]bool{}
	for {
		family := run.FontFamily
		if family == "" {
			family = "Go"
		}
		key := fontFamilyKey(family)
		target, ok := r.fontMap[key]
		if !ok {
			break
		}
		if seen[key] {
			return pdfFontFace{}, fmt.Errorf("cyclic PDF font mapping")
		}
		seen[key] = true
		run.FontFamily = target
	}
	if r.fonts == nil { // unit renderers using the original built-in font setup
		if run.FontFamily != "" && run.FontFamily != "Go" {
			return pdfFontFace{}, fmt.Errorf("%w: unavailable PDF font %q", ErrUnsupported, run.FontFamily)
		}
		if err := validatePDFText(Run{}); err != nil {
			return pdfFontFace{}, err
		}
		return pdfFontFace{id: "Go", parsed: parsedPDFFonts[runStyle(run)]}, nil
	}
	face, ok := r.fonts[fontKey(run.FontFamily, runStyle(run))]
	if !ok {
		return face, fmt.Errorf("%w: unavailable PDF font family/style %q %s", ErrUnsupported, run.FontFamily, runStyle(run))
	}
	return r.registerFont(fontKey(run.FontFamily, runStyle(run)))
}

// Register only used faces; the catalog still validates every supplied program.
func (r *pdfRenderer) registerFont(key string) (pdfFontFace, error) {
	face := r.fonts[key]
	if face.registered {
		return face, nil
	}
	if err := r.ctx.Err(); err != nil {
		return face, err
	}
	r.pdf.AddUTF8FontFromBytes(face.id, face.style, append([]byte(nil), face.data...))
	if err := r.ctx.Err(); err != nil {
		return face, err
	}
	if err := r.pdf.Error(); err != nil {
		return face, err
	}
	face.registered = true
	r.fonts[key] = face
	return face, nil
}

func fontFamilyKey(family string) string {
	if family == "" || family == "Go" {
		return "go"
	}
	if family == "Go Mono" {
		return "go mono"
	}
	return cases.Fold().String(norm.NFC.String(family))
}
