package manydoc

import (
	"fmt"
	"strings"

	"github.com/benoitkugler/textlayout/fonts/simpleencodings"
	pdffont "github.com/pdfcpu/pdfcpu/pkg/font"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

func (f *pdfFont) codeChunks(b []byte) ([]uint32, []int, error) {
	var codes []uint32
	var sizes []int
	for len(b) > 0 {
		size := 1
		if f.unicode || f.composite {
			size = 2
		}
		if f.mapping != nil && len(f.mapping.spaces) > 0 {
			size = 0
			for _, s := range f.mapping.spaces {
				if len(b) >= s.size {
					v := codeNumber(b[:s.size])
					if v >= s.low && v <= s.high {
						size = s.size
						break
					}
				}
			}
			if size == 0 {
				return nil, nil, fmt.Errorf("font code outside declared code spaces")
			}
		}
		if len(b) < size {
			return nil, nil, fmt.Errorf("truncated PDF font code")
		}
		codes = append(codes, codeNumber(b[:size]))
		sizes = append(sizes, size)
		b = b[size:]
	}
	return codes, sizes, nil
}
func (f *pdfFont) displacement(b []byte, size, charSpace, wordSpace, scale float64) (float64, bool, error) {
	codes, sizes, err := f.codeChunks(b)
	if err != nil {
		return 0, false, err
	}
	advance := 0.0
	estimated := f.cidWidthOverlap
	for i, code := range codes {
		width, ok := f.widths[code]
		if f.composite && !f.identityCID {
			ok = false
		}
		if !ok {
			width = f.defaultWidth
			if !f.defaultExact {
				estimated = true
			}
		}
		if f.composite && !f.identityCID {
			estimated = true
		}
		advance += (width*size/1000 + charSpace) * scale
		if sizes[i] == 1 && code == 32 {
			advance += wordSpace * scale
		}
	}
	return advance, estimated, nil
}
func (r *pdfReader) fontMetrics(f *pdfFont, d types.Dict) error {
	f.defaultWidth = 500
	f.ascent = 800
	f.descent = -200
	f.boundsEstimate = true
	descriptor, err := r.pdf.DereferenceDict(d["FontDescriptor"])
	if err != nil {
		return err
	}
	if sub := d.NameEntry("Subtype"); sub != nil && *sub == "Type0" {
		f.composite = true
		f.defaultWidth = 1000
		f.defaultExact = true
		if enc := d.NameEntry("Encoding"); enc != nil {
			f.identityCID = *enc == "Identity-H"
			f.vertical = *enc == "Identity-V" || strings.HasSuffix(*enc, "-V")
		}
		descendants, err := r.pdf.DereferenceArray(d["DescendantFonts"])
		if err != nil {
			return err
		}
		if len(descendants) != 1 {
			return fmt.Errorf("composite font requires one descendant")
		}
		cid, err := r.pdf.DereferenceDict(descendants[0])
		if err != nil {
			return err
		}
		descriptor, err = r.pdf.DereferenceDict(cid["FontDescriptor"])
		if err != nil {
			return err
		}
		if dw := cid["DW"]; dw != nil {
			value, err := r.pdf.DereferenceNumber(dw)
			if err != nil {
				return err
			}
			f.defaultWidth = value
		}
		widths, err := r.pdf.DereferenceArray(cid["W"])
		if err != nil {
			return err
		}
		if err := r.readCIDWidths(f, widths); err != nil {
			return err
		}
		if f.cidWidthOverlap {
			r.warnPage("pdf_cid_width_overlap", "Overlapping CID width declarations use the last source assignment; advance geometry may differ between readers")
		}
	}
	if descriptor != nil {
		if value := descriptor["Flags"]; value != nil {
			flags, err := r.pdf.DereferenceInteger(value)
			if err != nil || flags == nil || *flags < 0 {
				return fmt.Errorf("invalid PDF font flags")
			}
			f.bold = f.bold || int64(*flags)&(1<<18) != 0
			f.italic = f.italic || int64(*flags)&(1<<6) != 0
		}
		if value := descriptor["ItalicAngle"]; value != nil {
			angle, err := r.pdf.DereferenceNumber(value)
			if err != nil || !finite(angle) {
				return fmt.Errorf("invalid PDF font italic angle")
			}
			f.italic = f.italic || angle != 0
		}
		if value := descriptor["FontWeight"]; value != nil {
			weight, err := r.pdf.DereferenceNumber(value)
			if err != nil || !finite(weight) {
				return fmt.Errorf("invalid PDF font weight")
			}
			f.bold = f.bold || weight >= 700
		}
		if w := descriptor["MissingWidth"]; w != nil && !f.composite {
			value, err := r.pdf.DereferenceNumber(w)
			if err != nil {
				return err
			}
			f.defaultWidth = value
			f.defaultExact = true
		}
		a, da := descriptor["Ascent"], descriptor["Descent"]
		if a != nil && da != nil {
			f.ascent, err = r.pdf.DereferenceNumber(a)
			if err != nil {
				return err
			}
			f.descent, err = r.pdf.DereferenceNumber(da)
			if err != nil {
				return err
			}
			f.boundsEstimate = false
		}
	}
	r.fontFamily(f, descriptor)
	core := f.name
	if len(core) > 7 && core[6] == '+' {
		core = core[7:]
	}
	if pdffont.IsCoreFont(core) {
		if f.boundsEstimate {
			box := pdffont.BoundingBox(core)
			f.ascent = box.UR.Y
			f.descent = box.LL.Y
			f.boundsEstimate = false
		}
		// The public metrics API expects the core font's encoded byte. Resolve glyph
		// names rather than applying WinAnsi widths to an arbitrary source encoding.
		encoding := simpleencodings.WinAnsi
		if core == "Symbol" {
			encoding = simpleencodings.Symbol
		}
		if core == "ZapfDingbats" {
			encoding = simpleencodings.ZapfDingbats
		}
		inverse := map[string]int{}
		for code, name := range encoding {
			if name != "" {
				inverse[name] = code
			}
		}
		if core != "Symbol" && core != "ZapfDingbats" {
			// The reusable encoding includes Poppler aliases at NBSP/SHY and
			// undefined CP1252 bytes. pdfcpu's metrics lookup uses canonical
			// CP1252 codes, where those alias slots have no metrics. Resolve
			// duplicate glyph names to the actual space/hyphen/bullet slots.
			inverse["space"], inverse["hyphen"], inverse["bullet"] = 32, 45, 149
		}
		for code, name := range f.encoding {
			if _, ok := f.widths[uint32(code)]; ok {
				continue
			}
			if c, ok := inverse[name]; ok {
				f.widths[uint32(code)] = float64(pdffont.CharWidth(core, rune(c)))
			}
		}
	}
	if !finite(f.ascent) || !finite(f.descent) || f.ascent < f.descent || !finite(f.defaultWidth) {
		return fmt.Errorf("invalid font metrics")
	}
	for _, w := range f.widths {
		if !finite(w) {
			return fmt.Errorf("nonfinite glyph width")
		}
	}
	return nil
}

// Simple-font character codes are one byte (ISO 32000-1, 9.6). Some InDesign
// exports contradict that width through two-byte codespaces or zero-padded
// keys. Unambiguous numeric byte mappings win, with a repair diagnostic.
func (r *pdfReader) normalizeSimpleCMap(f *pdfFont) error {
	if f.mapping == nil || f.composite || f.unicode {
		return nil
	}
	repaired := false
	chars := map[string]string{}
	for key, text := range f.mapping.chars {
		code := codeNumber([]byte(key))
		if code > 255 {
			return fmt.Errorf("%w: ToUnicode key exceeds simple-font code range", ErrUnsupported)
		}
		canonical := string([]byte{byte(code)})
		if previous, exists := chars[canonical]; exists && previous != text {
			return fmt.Errorf("ambiguous padded simple-font ToUnicode keys")
		}
		chars[canonical] = text
		if len(key) != 1 {
			repaired = true
		}
	}
	for i, key := range f.mapping.ranges {
		if key.high > 255 {
			return fmt.Errorf("%w: ToUnicode range exceeds simple-font code range", ErrUnsupported)
		}
		if key.size != 1 {
			f.mapping.ranges[i].size = 1
			repaired = true
		}
	}
	f.mapping.chars = chars
	for _, space := range f.mapping.spaces {
		if space.size != 1 {
			f.mapping.spaces = []codeRange{{size: 1, low: 0, high: 255}}
			repaired = true
			break
		}
	}
	if repaired {
		r.warnPage("pdf_cmap_codespace", "Simple-font ToUnicode codespace/key padding contradicts single-byte font codes; unambiguous numeric byte mappings are used")
	}
	return nil
}
