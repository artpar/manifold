package manifold

import (
	"fmt"
	"strings"

	"golang.org/x/text/language"
)

const (
	docxASCII = iota
	docxHAnsi
	docxEastAsia
	docxCS
)

type docxFontChoice struct {
	family, theme string
	set           bool
}

// This is import state, not a second canonical document representation. Text
// leaves resolve it into ordinary public runs; latent selectors diagnose loss.
type docxRunStyle struct {
	Run
	fonts            [4]docxFontChoice
	hint             string
	cs, rtl          bool
	boldCS, italicCS bool
	sizeCS           float64
	hasCSStyle       bool
}

func (r *docxReader) readThemeLanguages(settings *xmlNode) error {
	for _, n := range settings.Children {
		if n.Name.Local != "themeFontLang" {
			continue
		}
		if r.themeLanguagesSet {
			return fmt.Errorf("duplicate DOCX theme font languages")
		}
		r.themeLanguagesSet = true
		r.themeLanguages = [3]string{n.attr("val"), n.attr("eastAsia"), n.attr("bidi")}
	}
	return nil
}
func docxThemeRegion(value string) (group, region string, languageIndex int) {
	switch value {
	case "majorAscii", "majorHAnsi":
		return "majorFont", "latin", 0
	case "minorAscii", "minorHAnsi":
		return "minorFont", "latin", 0
	case "majorEastAsia":
		return "majorFont", "ea", 1
	case "minorEastAsia":
		return "minorFont", "ea", 1
	case "majorBidi":
		return "majorFont", "cs", 2
	case "minorBidi":
		return "minorFont", "cs", 2
	}
	return "", "", 0
}
func (r *docxReader) themeFont(value string) (string, error) {
	group, region, index := docxThemeRegion(value)
	if group == "" {
		return "", fmt.Errorf("invalid DOCX theme font %q", value)
	}
	node := r.theme.child("themeElements").child("fontScheme").child(group)
	if locale := r.themeLanguages[index]; locale != "" {
		tag, err := language.Parse(locale)
		if err == nil {
			script, confidence := tag.Script()
			if confidence != language.No && tag.String() != "und" {
				if confidence == language.Low {
					r.doc.warn("font_theme_language_inferred", "Theme font script is inferred from language/region with limited certainty; resolved family and binding-loss diagnostics are retained")
				}
				for _, font := range paletteChildrenFont(node) {
					if font.Name.Local == "font" && font.attr("script") == script.String() && font.attr("typeface") != "" {
						return font.attr("typeface"), nil
					}
				}
				if script.String() != "Latn" {
					r.doc.warn("font_theme_language", "Theme language has no matching supplemental face; the region's default family is used when available")
				}
			} else {
				r.doc.warn("font_theme_language", "Theme font language has no reliable script mapping; region defaults/cache may be needed")
			}
		} else {
			r.doc.warn("font_theme_language", "Theme font language is not resolved; region defaults/cache may be needed")
		}
	}
	return node.child(region).attr("typeface"), nil
}
func paletteChildrenFont(node *xmlNode) []*xmlNode {
	if node == nil {
		return nil
	}
	return node.Children
}
func (r *docxReader) applyFontProperties(style *docxRunStyle, p *xmlNode) error {
	for i, name := range []string{"ascii", "hAnsi", "eastAsia", "cs"} {
		family, theme := p.attr(name), p.attr(name+"Theme")
		if i == docxCS {
			theme = p.attr("cstheme")
			if v := p.attr("csTheme"); v != "" {
				if theme != "" && theme != v {
					return fmt.Errorf("contradictory DOCX complex theme aliases")
				}
				theme = v
			}
		}
		if family == "" && theme == "" {
			continue
		}
		if family != "" {
			if err := validateTextStyle(Run{FontFamily: family}); err != nil {
				return err
			}
		}
		if theme != "" {
			if group, _, _ := docxThemeRegion(theme); group == "" {
				return fmt.Errorf("invalid DOCX theme font %q", theme)
			}
			r.doc.warn("font_theme", "Theme fonts resolve to explicit span families; editable theme binding is not retained")
		}
		// A direct family and its theme selector form a pair at each level. A later
		// family replaces an inherited theme, and a later theme replaces a family.
		style.fonts[i] = docxFontChoice{family: family, theme: theme, set: true}
	}
	if hint := p.attr("hint"); hint != "" {
		if hint != "default" && hint != "eastAsia" && hint != "cs" {
			return fmt.Errorf("invalid DOCX font hint")
		}
		style.hint = hint
	}
	return nil
}
func (r *docxReader) selectedFont(style docxRunStyle, slot int) (string, error) {
	choice := style.fonts[slot]
	if !choice.set {
		for _, other := range style.fonts {
			if other.set {
				r.doc.warn("font_slot_default", "A used font slot has no declared family; application-defined defaults remain unresolved")
				break
			}
		}
		return "", nil
	}
	family := choice.family
	if choice.theme != "" {
		resolved, err := r.themeFont(choice.theme)
		if err != nil {
			return "", err
		}
		if resolved != "" {
			family = resolved
		} else {
			if family == "" {
				return "", fmt.Errorf("%w: unresolved DOCX theme font %q for used slot", ErrUnsupported, choice.theme)
			}
			r.doc.warn("font_theme_cache", "Unresolved theme font uses its stored family cache; current theme choice is uncertain")
		}
	}
	return family, validateTextStyle(Run{FontFamily: family})
}
func chineseFontLanguage(value string) bool {
	tag, err := language.Parse(value)
	if err != nil {
		return false
	}
	base, _ := tag.Base()
	return base.String() == "zh"
}
func (r *docxReader) fontSlot(ch rune, style docxRunStyle) int {
	if style.cs || style.rtl {
		return docxCS
	}
	if ch <= 0x7F {
		return docxASCII
	}
	if ch > 0xFFFF {
		return docxEastAsia
	} // UTF-16 surrogate classification in MS-OI29500.
	if ch >= 0x1100 && ch <= 0x11FF || ch >= 0x2F00 && ch <= 0x2FDF || ch >= 0x2FF0 && ch <= 0x4DBF || ch >= 0x4E00 && ch <= 0x9FAF || ch >= 0xA000 && ch <= 0xA4CF || ch >= 0xAC00 && ch <= 0xD7AF || ch >= 0xF900 && ch <= 0xFAFF || ch >= 0xFE30 && ch <= 0xFE6F || ch >= 0xFF00 && ch <= 0xFFEF {
		return docxEastAsia
	}
	if ch >= 0x590 && ch <= 0x7BF || ch >= 0xFB1D && ch <= 0xFDFF || ch >= 0xFE70 && ch <= 0xFEFE {
		return docxASCII
	}
	if style.hint != "eastAsia" {
		return docxHAnsi
	}
	if ch >= 0x2B0 && ch <= 0x3CF || ch >= 0x400 && ch <= 0x4FF || ch >= 0x2000 && ch <= 0x27BF || ch >= 0x2E80 && ch <= 0x2EFF || ch >= 0xE000 && ch <= 0xF8FF || ch >= 0xFB00 && ch <= 0xFB1C {
		return docxEastAsia
	}
	if ch == 0xA1 || ch == 0xA4 || ch >= 0xA7 && ch <= 0xA8 || ch == 0xAA || ch == 0xAD || ch == 0xAF || ch >= 0xB0 && ch <= 0xB4 || ch >= 0xB6 && ch <= 0xBA || ch >= 0xBC && ch <= 0xBF || ch == 0xD7 || ch == 0xF7 {
		return docxEastAsia
	}
	chinese := chineseFontLanguage(style.EastAsiaLanguage)
	if chinese && (ch >= 0xE0 && ch <= 0xE1 || ch >= 0xE8 && ch <= 0xEA || ch >= 0xEC && ch <= 0xED || ch >= 0xF2 && ch <= 0xF3 || ch >= 0xF9 && ch <= 0xFA || ch == 0xFC || ch >= 0x1E00 && ch <= 0x1EFF) {
		return docxEastAsia
	}
	if ch >= 0x100 && ch <= 0x2AF {
		if chinese {
			return docxEastAsia
		}
		// Font-table charset participates only in this particular hint exception.
		family, err := r.selectedFont(style, docxEastAsia)
		if err != nil {
			r.doc.warn("font_charset", "East Asian face metadata is unresolved for a hint-dependent Latin range; hAnsi is retained with uncertain slot selection")
		}
		if err == nil && family != "" && r.fontCharsets[fontFamilyKey(family)] == "" {
			r.doc.warn("font_charset", "East Asian charset metadata is unavailable for a hint-dependent Latin range; hAnsi is retained with uncertain slot selection")
		}
		if err == nil && (r.fontCharsets[fontFamilyKey(family)] == "86" || r.fontCharsets[fontFamilyKey(family)] == "88") {
			return docxEastAsia
		}
	}
	return docxHAnsi
}
func (r *docxReader) textRuns(style docxRunStyle) ([]Run, error) {
	var first *docxFontChoice
	conditional := style.hint == "eastAsia"
	for i, choice := range style.fonts {
		if choice.set {
			if first == nil {
				first = &style.fonts[i]
			} else if choice != *first {
				conditional = true
			}
		}
	}
	conditional = conditional || style.hasCSStyle && (style.boldCS != style.Bold || style.italicCS != style.Italic || style.sizeCS != style.FontSize)
	if conditional {
		r.doc.warn("font_selectors", "Conditional font selectors are resolved for current text; unused font choices and selection hints are not retained")
	}

	if style.Note != nil {
		slot := docxASCII
		if style.cs || style.rtl {
			slot = docxCS
		}
		run := style.Run
		family, err := r.selectedFont(style, slot)
		if err != nil {
			return nil, err
		}
		run.FontFamily = family
		if style.cs || style.rtl {
			run.Bold, run.Italic, run.FontSize = style.boldCS, style.italicCS, style.sizeCS
		}
		return []Run{run}, nil
	}
	var out []Run
	var previous int = -1
	start := 0
	appendPart := func(end, slot int) error {
		run := style.Run
		run.Text = style.Text[start:end]
		family, err := r.selectedFont(style, slot)
		if err != nil {
			return err
		}
		run.FontFamily = family
		if style.cs || style.rtl {
			run.Bold, run.Italic, run.FontSize = style.boldCS, style.italicCS, style.sizeCS
		}
		if len(out) > 0 && out[len(out)-1].FontFamily == run.FontFamily && out[len(out)-1].Bold == run.Bold && out[len(out)-1].Italic == run.Italic && out[len(out)-1].FontSize == run.FontSize {
			out[len(out)-1].Text += run.Text
		} else {
			out = append(out, run)
		}
		start = end
		return nil
	}
	// Times New Roman's East Asian exception is defined on effective slot values.
	ascii, ea, han := "", "", ""
	if !style.cs && !style.rtl {
		var err error
		if style.fonts[docxEastAsia].set {
			ea, err = r.selectedFont(style, docxEastAsia)
			if err != nil {
				ea = ""
			}
		}
		if ea == "Times New Roman" {
			ascii, err = r.selectedFont(style, docxASCII)
			if err != nil {
				return nil, err
			}
			han, err = r.selectedFont(style, docxHAnsi)
			if err != nil {
				return nil, err
			}
		}
	}
	for i, ch := range style.Text {
		if err := r.ctx.Err(); err != nil {
			return nil, err
		}
		slot := r.fontSlot(ch, style)
		if ea == "Times New Roman" && ascii != "" && ascii == han {
			slot = docxASCII
		}
		if previous != -1 && slot != previous {
			if err := appendPart(i, previous); err != nil {
				return nil, err
			}
		}
		previous = slot
	}
	if previous != -1 {
		if err := appendPart(len(style.Text), previous); err != nil {
			return nil, err
		}
	}
	if len(out) == 0 {
		run := style.Run
		run.FontFamily = ""
		out = []Run{run}
	}
	return out, nil
}
func (r *docxReader) restoreTextImplicit(style *docxRunStyle, n *xmlNode, heading int) error {
	flags := n.attributeNS(docxNotesNS, "textImplicit")
	if strings.Contains(flags, "s") && style.sizeCS != 0 && style.sizeCS != style.FontSize {
		return fmt.Errorf("implicit size metadata contradicts inherited complex size")
	}
	if strings.Contains(flags, "f") {
		family, err := r.selectedFont(*style, docxASCII)
		if err != nil {
			return err
		}
		style.FontFamily = family
		for slot, choice := range style.fonts {
			if choice.set {
				value, err := r.selectedFont(*style, slot)
				if err != nil {
					return err
				}
				if value != "Noto Sans" {
					return fmt.Errorf("implicit font metadata contradicts script properties")
				}
			}
		}
	}
	if err := restoreDocxTextImplicit(&style.Run, n, heading); err != nil {
		return err
	}
	if strings.Contains(flags, "f") {
		style.fonts = [4]docxFontChoice{}
	}
	if strings.Contains(flags, "s") {
		if n.child("rPr").child("szCs") != nil {
			return fmt.Errorf("implicit size metadata contradicts complex size")
		}
		style.sizeCS = 0
	}
	return r.restoreFontSizeIntent(style, n)
}

func (r *docxReader) readFontCharsets(table *xmlNode) error {
	if r.fontCharsets == nil {
		r.fontCharsets = map[string]string{}
	}
	for _, font := range table.Children {
		if err := r.ctx.Err(); err != nil {
			return err
		}
		if font.Name.Local != "font" {
			continue
		}
		value := font.child("charset").attr("val")
		if value == "" {
			continue
		}
		if len(value) != 2 || !docxHexDigits(value) {
			return fmt.Errorf("invalid DOCX font charset byte")
		}
		if font.attr("name") == "" {
			return fmt.Errorf("DOCX font charset requires a named font")
		}
		key := fontFamilyKey(font.attr("name"))
		if old, ok := r.fontCharsets[key]; ok && old != strings.ToUpper(value) {
			return fmt.Errorf("contradictory DOCX font charsets")
		}
		r.fontCharsets[key] = strings.ToUpper(value)
	}
	return nil
}
