package engine

import (
	"encoding/xml"
	"errors"
	"fmt"
	"github.com/benoitkugler/textlayout/fonts/glyphsnames"
	"github.com/benoitkugler/textlayout/fonts/simpleencodings"
	"golang.org/x/text/language"
	"golang.org/x/text/unicode/bidi"
	"math"
	"strconv"
	"strings"
)

const docxMetadataNamespaces = ` xmlns:md="` + docxNotesNS + `" xmlns:mc="` + markupCompatibilityNS + `" mc:Ignorable="md"`

func docxOnOff(n *xmlNode) (bool, error) {
	if n == nil {
		return false, nil
	}
	value, present := "", false
	for _, a := range n.Attr {
		if a.Name.Local == "val" {
			if present {
				return false, fmt.Errorf("duplicate DOCX boolean value")
			}
			value, present = a.Value, true
		}
	}
	if !present {
		return true, nil
	}
	switch value {
	case "true", "on", "1":
		return true, nil
	case "false", "off", "0":
		return false, nil
	}
	return false, fmt.Errorf("invalid DOCX boolean value %q", value)
}
func paragraphBreakProperty(pr *xmlNode) (*xmlNode, error) {
	return paragraphNamedProperty(pr, "pageBreakBefore")
}
func paragraphNamedProperty(pr *xmlNode, property string) (*xmlNode, error) {
	var found *xmlNode
	if pr != nil {
		for _, c := range pr.Children {
			if c.Name.Local == property {
				if found != nil {
					return nil, fmt.Errorf("duplicate %s property", property)
				}
				found = c
			}
		}
	}
	return found, nil
}
func (r *docxReader) paragraphOnOff(pr *xmlNode, styleID, property string) (bool, bool, error) {
	present := false
	value := false
	apply := func(pr *xmlNode) error {
		p, err := paragraphNamedProperty(pr, property)
		if err != nil {
			return err
		}
		if p != nil {
			present = true
			value, err = docxOnOff(p)
		}
		return err
	}
	if err := apply(r.defaultPPr); err != nil {
		return false, false, err
	}
	if styleID == "" {
		styleID = r.defaultParagraphStyle
	}
	visited := map[string]bool{}
	var chain []*xmlNode
	for styleID != "" {
		if err := r.ctx.Err(); err != nil {
			return false, false, err
		}
		if visited[styleID] {
			return false, false, fmt.Errorf("cyclic DOCX paragraph style inheritance")
		}
		visited[styleID] = true
		s := r.styles[styleID]
		if s == nil {
			break
		}
		chain = append(chain, s)
		styleID = s.child("basedOn").attr("val")
	}
	for i := len(chain) - 1; i >= 0; i-- {
		if err := apply(chain[i].child("pPr")); err != nil {
			return false, false, err
		}
	}
	if err := apply(pr); err != nil {
		return false, false, err
	}
	return value, present, nil
}

func (r *docxReader) paragraphPageBreak(pr *xmlNode, styleID string) (bool, error) {
	value, _, err := r.paragraphOnOff(pr, styleID, "pageBreakBefore")
	return value, err
}
func (r *docxReader) paragraphDirection(pr *xmlNode, styleID string) (string, error) {
	value, present, err := r.paragraphOnOff(pr, styleID, "bidi")
	if err != nil || !present {
		return "", err
	}
	if value {
		return "rtl", nil
	}
	return "ltr", nil
}

// Metadata distinguishes an exported unconditional control from a normal
// conditional paragraph property. Synthetic anchors/controls must be empty:
// never drop visible content merely because a metadata attribute requests it.
func docxBreakRole(n *xmlNode) (role string, conditional bool, err error) {
	present := false
	conditionalPresent := false
	for _, a := range n.Attr {
		if a.Name.Space != docxNotesNS {
			continue
		}
		switch a.Name.Local {
		case "breakRole":
			if present {
				return "", false, fmt.Errorf("duplicate break metadata")
			}
			present = true
			role = a.Value
		case "conditionalBefore":
			if conditionalPresent || a.Value != "1" {
				return "", false, fmt.Errorf("invalid conditional break metadata")
			}
			conditionalPresent = true
			conditional = true
		}
	}
	if !present {
		if conditionalPresent {
			return "", false, fmt.Errorf("conditional metadata requires explicit break role")
		}
		return "", false, nil
	}
	if role == "anchor" {
		if conditional || len(n.Children) > 0 || strings.TrimSpace(n.content()) != "" {
			return "", false, fmt.Errorf("break anchor metadata contradicts content")
		}
		return role, false, nil
	}
	if role != "control" && role != "before" && role != "beforeList" {
		return "", false, fmt.Errorf("invalid break role %q", role)
	}
	p, err := paragraphBreakProperty(n.child("pPr"))
	if err != nil {
		return "", false, err
	}
	on, err := docxOnOff(p)
	if err != nil {
		return "", false, err
	}
	if p == nil || !on {
		return "", false, fmt.Errorf("break metadata requires enabled pageBreakBefore")
	}
	if role == "control" {
		if conditional || len(n.Children) != 1 || n.Children[0].Name.Local != "pPr" || len(n.Children[0].Children) != 1 || strings.TrimSpace(n.content()) != "" {
			return "", false, fmt.Errorf("break control metadata contradicts content")
		}
	}
	return role, conditional, nil
}

// Required OOXML paragraphs are physical structure, not extra authored SF
// paragraphs. Only recognize our checked empty markers in their exact context.

// Standard Unicode isolates work in readers that ignore OOXML w:dir. Checked
// roles remove only these exact generated controls, never arbitrary user text.

// A rich citation is an ordinary hyperlink with visible literal runs. The body
// target and reference style are checked before reusing ordinary text decoding.

// This is import state, not a second canonical document representation. Text
// leaves resolve it into ordinary public runs; latent selectors diagnose loss.

// A direct family and its theme selector form a pair at each level. A later
// family replaces an inherited theme, and a later theme replaces a family.

const docxListTableBoundary = `<w:p md:listTableBoundary="1"><w:pPr><w:numPr><w:numId w:val="0"/></w:numPr></w:pPr></w:p>`

func docxStructuralParagraph(parent, n *xmlNode) (bool, error) {
	if value := n.attributeNS(docxNotesNS, "listTableBoundary"); value != "" {
		pr := n.child("pPr")
		num := pr.child("numPr")
		id := num.child("numId")
		if pr == nil || num == nil || id == nil || value != "1" || len(n.Attr) != 1 || n.content() != "" || len(n.Children) != 1 || len(pr.Children) != 1 || len(num.Children) != 1 || len(id.Attr) != 1 || id.attr("val") != "0" || len(id.Children) != 0 {
			return false, fmt.Errorf("list/table boundary metadata contradicts content")
		}
		index := -1
		for i, c := range parent.Children {
			if c == n {
				index = i
				break
			}
		}
		if

		// UTF-16 surrogate classification in MS-OI29500.

		index < 1 || parent.Children[index-1].Name.Local != "bookmarkEnd" {
			return false, fmt.Errorf("list/table boundary lacks preceding list end")
		}
		endID := parent.Children[index-1].attr("id")
		matched := false
		for _, c := range parent.Children[:index-1] {
			if c.Name.Local == "bookmarkStart" && c.attr("id") == endID && strings.HasPrefix(c.attr("name"), "mdlist1_") {
				matched = true
			}
		}
		next := index + 1
		for next < len(parent.Children) && (parent.Children[next].Name.Local == "bookmarkStart" || parent.Children[next].Name.Local == "bookmarkEnd") {
			next++
		}
		if !matched || next >= len(parent.Children) || parent.Children[next].Name.Local != "tbl" {
			return false, fmt.Errorf("list/table boundary contradicts sibling structure")
		}
		return true, nil
	}

	empty, tail := false, false
	for _, a := range n.Attr {
		if a.Name.Space != docxNotesNS {
			continue
		}
		if a.Name.Local == "emptyCell" {
			if empty || a.Value != "1" {
				return false, fmt.Errorf("invalid empty cell metadata")
			}
			empty = true
		}
		if a.Name.Local == "tableTail" {
			if tail || a.Value != "1" {
				return false, fmt.Errorf("invalid table tail metadata")
			}
			tail = true
		}
	}
	if !empty && !tail {
		return false, nil
	}
	if empty && tail || parent.Name.Local != "tc" || len(n.Children) > 0 || strings.TrimSpace(n.Text) != "" ||

		// Font-table charset participates only in this particular hint exception.

		n.attributeNS(docxNotesNS, "breakRole") != "" {
		return false, fmt.Errorf("structural paragraph metadata contradicts content")
	}
	var content []*xmlNode
	for _, c := range parent.Children {
		if c.Name.Local != "tcPr" && c.Name.Local != "bookmarkStart" && c.Name.Local != "bookmarkEnd" {
			content = append(content, c)
		}
	}
	if empty && (len(content) != 1 || content[0] != n) {
		return false, fmt.Errorf("empty cell metadata contradicts sibling content")
	}
	if tail && (len(content) < 2 || content[len(content)-1] != n || content[len(content)-2].Name.Local != "tbl") {
		return false, fmt.Errorf("table tail metadata contradicts placement")
	}
	return true, nil
}

const docxCitationIsolationStart = `<w:r md:noteIsolation="start"><w:t>` + "\u2066" + `</w:t></w:r>`
const docxCitationIsolationEnd = `<w:r md:noteIsolation="end"><w:t>` + "\u2069" + `</w:t></w:r>`

func citationNeedsIsolation(text string) bool {
	var rtl, other bool
	for _, ch := range text {
		p, _ := bidi.LookupRune(ch)
		switch p.Class() {
		case bidi.R, bidi.AL:
			rtl = true
		case bidi.L, bidi.EN, bidi.AN:
			other = true
		}
	}
	return rtl && other
}

func checkedCitationIsolation(n *xmlNode) (*xmlNode, error) {
	role := n.attributeNS(docxNotesNS, "noteIsolation")
	if role == "" {
		return n, nil
	}
	if err := checkVariantNamespaces(n); err != nil {
		return nil, err
	}
	if role != "ltr" || len(n.Children) < 3 {
		return nil, fmt.Errorf("invalid citation isolation range")
	}
	for i, expected := range []struct{ role, text string }{{"start", "\u2066"}, {"end", "\u2069"}} {
		child := n.Children[0]
		if i == 1 {
			child = n.Children[len(n.Children)-1]
		}
		if child.Name.Local != "r" || len(child.Attr) != 1 || child.attributeNS(docxNotesNS, "noteIsolation") != expected.role || len(child.Children) != 1 || child.Children[0].Name.Local != "t" || len(child.Children[0].Attr) != 0 || len(child.Children[0].Children) != 0 || child.content() != expected.text {
			return nil, fmt.Errorf("citation isolation contradicts literal controls")
		}
	}
	copy := *n
	copy.Children = n.Children[1 : len(n.Children)-1]
	return &copy, nil
}

func (r *docxReader) richCitation(n *xmlNode, ref *NoteReference) (*NoteReference, error) {
	name := noteBodyName(referenceKey(ref))
	if n.attr("anchor") != name || n.attr("id") != "" || len(n.Children) == 0 {
		return nil, fmt.Errorf("invalid rich citation target/content")
	}
	children := make([]*xmlNode, 0, len(n.Children))
	for _, child := range n.Children {
		if child.Name.Local != "r" || child.child("t") == nil || child.child("t").Text == "" {
			return nil, fmt.Errorf("rich citation requires literal nonempty text runs")
		}
		copy := *child
		copy.Children = append([]*xmlNode(nil), child.Children...)
		if props := child.child("rPr"); props != nil {
			clean := *props
			clean.Children = nil
			found := false
			for _, prop := range props.Children {
				if prop.Name.Local == "rStyle" {
					if found || prop.attr("val") != strings.Title(ref.Kind)+"Reference" || len(prop.Children) != 0 {
						return nil, fmt.Errorf("invalid rich citation reference style")
					}
					found = true
					continue
				}
				clean.Children = append(clean.Children, prop)
			}
			if !found {
				return nil, fmt.Errorf("missing rich citation reference style")
			}
			for i, c := range copy.Children {
				if c == props {
					copy.Children[i] = &clean
				}
			}
		} else {
			return nil, fmt.Errorf("missing rich citation properties")
		}
		if err := validateRichBodyInline(&copy); err != nil {
			return nil, err
		}
		children = append(children, &copy)
	}
	blocks, err := r.paragraph(&xmlNode{Name: xml.Name{Space: n.Name.Space, Local: "p"}, Children: children}, nil)
	if err != nil {
		return nil, err
	}
	if len(blocks) != 1 || blocks[0].Kind != Paragraph {
		return nil, fmt.Errorf("rich citation contains nontext content")
	}
	ref.LabelContent = blocks[0].Runs
	if err := validateCitationContent(ref, r.doc.Pages); err != nil {
		return nil, err
	}
	if r.variantCitations == nil {
		r.variantCitations = map[noteKey]bool{}
	}
	r.variantCitations[referenceKey(ref)] = true
	return ref, nil
}

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
func (r *docxReader) themeFont(value, runLanguage string) (string, error) {
	group, region, index := docxThemeRegion(value)
	if group == "" {
		return "", fmt.Errorf("invalid DOCX theme font %q", value)
	}
	node := r.theme.child("themeElements").child("fontScheme").child(group)
	locale := r.themeLanguages[index]
	if locale == "" && node.child(region).attr("typeface") == "" && runLanguage != "" {
		locale = runLanguage
		r.doc.warn("font_theme_run_language", "An empty theme-region face is resolved from its supplemental script face using the explicit run language")
	}
	if locale != "" {
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
		_, _, index := docxThemeRegion(choice.theme)
		locale := style.Language
		if index == 1 {
			locale = style.EastAsiaLanguage
		} else if index == 2 {
			locale = style.ComplexLanguage
		}
		resolved, err := r.themeFont(choice.theme, locale)
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
	}
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

	// Half-point output uses nearest rounding, with ties upward. Values outside the
	// representable range reject; acknowledgement never enables clamping/overflow.

	// Metadata restores original authoring precision only when both explicit standard
	// sizes agree with its deterministic projection. No text/property is suppressed.

	// OOXML style toggles differ from absolute direct run formatting.

	// ST_ThemeColor aliases and ST_ColorSchemeIndex values use different names
	// from the DrawingML palette. Mapping targets name palette slots directly;
	// do not recursively remap them (swapping two accents is valid).

	// Word run colors use HSL luminance, unlike DrawingML component tint/shade.
	// See MS-OI29500 2.1.72: when both are specified, themeTint takes precedence.

	// Retain hue/saturation by scaling the chroma around the new luminance.
	// RGB = L + (component-L) * newChroma/oldChroma.

	// A cached value already includes the producer's tint/shade calculation.
	// Never transform it a second time or use auto as recovered theme RGB.

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

func docxFontHalfPoints(size float64) (float64, error) {
	h := math.Round(size * 2)
	if !finite(size) || size <= 0 || h < 1 || h > math.MaxUint32 || math.Abs(h/2-size) > .25 {
		return 0, fmt.Errorf("%w: DOCX font size is outside the half-point range", ErrUnsupported)
	}
	return h, nil
}
func docxFontSizeIntent(run Run) string {
	if run.FontSize == 0 || run.FontSize*2 == math.Trunc(run.FontSize*2) {
		return ""
	}
	return ` md:fontSize="` + strconv.FormatFloat(run.FontSize, 'g', -1, 64) + `"`
}

func (r *docxReader) restoreFontSizeIntent(style *docxRunStyle, n *xmlNode) error {
	value := n.attributeNS(docxNotesNS, "fontSize")
	if value == "" {
		if n != nil {
			for _, attr := range n.Attr {
				if attr.Name.Space == docxNotesNS && attr.Name.Local == "fontSize" {
					return fmt.Errorf("empty DOCX original font size")
				}
			}
		}
		return nil
	}
	size, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return fmt.Errorf("invalid DOCX original font size")
	}
	h, err := docxFontHalfPoints(size)
	if err != nil || size*2 == h {
		return fmt.Errorf("invalid DOCX font size precision metadata")
	}
	expected := strconv.FormatFloat(h, 'f', 0, 64)
	pr := n.child("rPr")
	validSize := func(p *xmlNode) bool {
		return p != nil && p.Name.Space == n.Name.Space && (p.Name.Space == wordNS || p.Name.Space == strictWordNS) && p.attributeNS(p.Name.Space, "val") == expected && len(p.Children) == 0 && strings.TrimSpace(p.Text) == ""
	}
	if pr == nil || pr.Name.Space != n.Name.Space || !validSize(pr.child("sz")) || !validSize(pr.child("szCs")) || style.FontSize != h/2 || style.sizeCS != h/2 {
		return fmt.Errorf("DOCX font size precision metadata contradicts standard sizes")
	}
	style.FontSize = size
	style.sizeCS = size
	r.doc.warn("docx_font_size_precision", "Portable DOCX uses nearest half-point sizes; checked original authoring precision is restored")
	return nil
}
func (r *docxReader) textStyleChain(run *docxRunStyle, id string) error {
	seen := map[string]bool{}
	var chain []*xmlNode
	for id != "" {
		if err := r.ctx.Err(); err != nil {
			return err
		}
		if seen[id] {
			return fmt.Errorf("cyclic DOCX text style inheritance")
		}
		seen[id] = true
		node := r.styles[id]
		if node == nil {
			break
		}
		chain = append(chain, node)
		id = node.child("basedOn").attr("val")
	}
	for i := len(chain) - 1; i >= 0; i-- {
		r.warnTextStyleProperties(chain[i].child("rPr"), chain[i].attr("styleId"))
		if err := applyStyleRunProps(&run.Run, chain[i].child("rPr")); err != nil {
			return err
		}
		if err := r.textProperties(run, chain[i].child("rPr"), true); err != nil {
			return err
		}
	}
	return nil
}
func (r *docxReader) paragraphTextStyle(id string) (docxRunStyle, error) {
	run := docxRunStyle{}
	r.warnTextStyleProperties(r.defaultRPr, "")
	applyRunProps(&run.Run, r.defaultRPr)
	if err := r.textProperties(&run, r.defaultRPr, false); err != nil {
		return run, err
	}
	if id == "" {
		id = r.defaultParagraphStyle
	}
	err := r.textStyleChain(&run, id)
	return run, err
}
func (r *docxReader) textProperties(run *docxRunStyle, pr *xmlNode, styleDefinition bool) error {
	if pr == nil {
		return nil
	}
	seen := map[string]bool{}
	for _, p := range pr.Children {
		if err := r.ctx.Err(); err != nil {
			return err
		}
		kind := p.Name.Local
		if kind != "rFonts" && kind != "sz" && kind != "szCs" && kind != "bCs" && kind != "iCs" && kind != "cs" && kind != "rtl" && kind != "lang" && kind != "color" {
			continue
		}
		if seen[kind] {
			return fmt.Errorf("duplicate DOCX text property %s", kind)
		}
		seen[kind] = true
		switch kind {
		case "sz":
			v, err := strconv.ParseUint(p.attr("val"), 10, 32)
			if err != nil || v == 0 {
				return fmt.Errorf("invalid DOCX font size")
			}
			run.FontSize = float64(v) / 2
		case "rFonts":
			if err := r.applyFontProperties(run, p); err != nil {
				return err
			}
		case "lang":
			attrs := map[string]bool{}
			for _, a := range p.Attr {
				var target *string
				switch a.Name.Local {
				case "val":
					target = &run.Language
				case "eastAsia":
					target = &run.EastAsiaLanguage
				case "bidi":
					target = &run.ComplexLanguage
				default:
					continue
				}
				if attrs[a.Name.Local] || a.Value == "" {
					return fmt.Errorf("invalid or duplicate DOCX language attribute %s", a.Name.Local)
				}
				attrs[a.Name.Local] = true
				if err := validateLanguage(a.Value); err != nil {
					return err
				}
				*target = a.Value
			}
		case "szCs":
			v, err := strconv.ParseUint(p.attr("val"), 10, 32)
			if err != nil || v == 0 {
				return fmt.Errorf("invalid DOCX complex font size")
			}
			run.sizeCS = float64(v) / 2
			run.hasCSStyle = true
		case "bCs", "iCs", "cs", "rtl":
			on, err := docxOnOff(p)
			if err != nil {
				return err
			}
			if kind == "bCs" || kind == "iCs" {
				run.hasCSStyle = true
			}
			target := &run.cs
			switch kind {
			case "cs":
				run.ComplexScript = on
			case "rtl":
				target = &run.rtl
				run.Direction = "ltr"
				if on {
					run.Direction = "rtl"
				}
			case "bCs":
				target = &run.boldCS
			case "iCs":
				target = &run.italicCS
			}
			if styleDefinition && (kind == "bCs" || kind == "iCs") {
				if on {
					*target = !*target
				}
			} else {
				*target = on
			}
		case "color":
			value, err := r.textColor(p)
			if err != nil {
				return err
			}
			run.Color = value
		}
	}
	return validateTextStyle(run.Run)
}
func docxTextStyle(run Run, allowLossy bool) (string, error) {
	var out strings.Builder
	if run.ComplexScript {
		out.WriteString(`<w:cs/>`)
	}
	if run.Direction != "" {
		fmt.Fprintf(&out, `<w:rtl w:val="%t"/>`, run.Direction == "rtl")
	}
	if run.Language != "" || run.EastAsiaLanguage != "" || run.ComplexLanguage != "" {
		out.WriteString(`<w:lang`)
		for _, a := range []struct{ name, value string }{{"val", run.Language}, {"eastAsia", run.EastAsiaLanguage}, {"bidi", run.ComplexLanguage}} {
			if a.value != "" {
				fmt.Fprintf(&out, ` w:%s="%s"`, a.name, escape(a.value))
			}
		}
		out.WriteString(`/>`)
	}
	if run.FontFamily != "" {
		fmt.Fprintf(&out, `<w:rFonts w:ascii="%s" w:hAnsi="%s" w:eastAsia="%s" w:cs="%s"/>`, escape(run.FontFamily), escape(run.FontFamily), escape(run.FontFamily), escape(run.FontFamily))
	}
	if run.FontSize > 0 {
		h, err := docxFontHalfPoints(run.FontSize)
		if err != nil {
			return "", err
		}
		if run.FontSize*2 != h && !allowLossy {
			return "", fmt.Errorf("%w: DOCX font size requires half-points; AllowLossy permits rounding with checked original intent", ErrUnsupported)
		}
		fmt.Fprintf(&out, `<w:sz w:val="%.0f"/><w:szCs w:val="%.0f"/>`, h, h)
	}
	if run.Color != "" {
		value := strings.TrimPrefix(canonicalTextColor(run.Color), "#")
		fmt.Fprintf(&out, `<w:color w:val="%s"/>`, value)
	}
	return out.String(), nil
}
func docxTextImplicit(run Run) string {
	value := ""
	if run.FontFamily == "" {
		value += "f"
	}
	if run.FontSize == 0 {
		value += "s"
	}
	if run.Color == "" {
		value += "c"
	}
	if value == "" {
		return ""
	}
	return ` md:textImplicit="` + value + `"`
}
func restoreDocxTextImplicit(run *Run, n *xmlNode, heading int) error {
	flags := n.attributeNS(docxNotesNS, "textImplicit")
	if flags == "" {
		return nil
	}
	expected := ""
	for _, c := range "fsc" {
		if strings.ContainsRune(flags, c) {
			expected += string(c)
		}
	}
	if flags != expected {
		return fmt.Errorf("invalid text implicit metadata")
	}
	pr := n.child("rPr")
	if strings.Contains(flags, "f") {
		if pr.child("rFonts") != nil || run.FontFamily != "Noto Sans" {
			return fmt.Errorf("implicit font metadata contradicts properties")
		}
		run.FontFamily = ""
	}
	if strings.Contains(flags, "s") {
		size := 11.0
		if heading > 0 {
			size = float64(44-heading*3) / 2
		}
		if pr.child("sz") != nil || run.FontSize != size {
			return fmt.Errorf("implicit size metadata contradicts properties")
		}
		run.FontSize = 0
	}
	if strings.Contains(flags, "c") {
		if pr.child("color") != nil || run.Color != "" {
			return fmt.Errorf("implicit color metadata contradicts properties")
		}
		run.Color = ""
	}
	return nil
}

func applyStyleRunProps(run *Run, pr *xmlNode) error {
	if pr == nil {
		return nil
	}
	seen := map[string]bool{}
	for _, p := range pr.Children {
		var target *bool
		switch p.Name.Local {
		case "b":
			target = &run.Bold
		case "i":
			target = &run.Italic
		case "strike":
			target = &run.Strike
		case "u":
			applyRunProps(run, &xmlNode{Children: []*xmlNode{p}})
		}
		if target != nil {
			if seen[p.Name.Local] {
				return fmt.Errorf("duplicate DOCX style toggle")
			}
			seen[p.Name.Local] = true
			on, err := docxOnOff(p)
			if err != nil {
				return err
			}
			if on {
				*target = !*target
			}
		}
	}
	return nil
}

func (r *docxReader) warnTextStyleProperties(pr *xmlNode, id string) {
	if pr == nil {
		return
	}
	for _, p := range pr.Children {
		if r.ctx.Err() != nil {
			return
		}
		switch p.Name.Local {
		case "b", "i", "strike", "u", "rFonts", "sz", "szCs", "bCs", "iCs", "cs", "lang", "color", "rStyle", "rtl":
		case "vertAlign":
			if id != "FootnoteReference" && id != "EndnoteReference" {
				r.doc.warn("run_formatting", "Style property not represented: vertAlign")
			}
		default:
			r.doc.warn("run_formatting", "Style property not represented: "+p.Name.Local)
		}
	}
}

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

func docxSymbolUnicode(font, hexCode string) (string, error) {
	code, e := strconv.ParseUint(hexCode, 16, 16)
	if e != nil {
		return "", fmt.Errorf("invalid DOCX symbol code")
	}
	if code >= 0xf000 && code <= 0xf0ff {
		code -= 0xf000
	}
	if code > 255 {
		return "", fmt.Errorf("%w: DOCX symbol-font code outside known encoding", ErrUnsupported)
	}
	var enc simpleencodings.Encoding
	switch strings.ToLower(strings.TrimSpace(font)) {
	case "symbol":
		enc = simpleencodings.Symbol
	case "zapfdingbats", "zapf dingbats":
		enc = simpleencodings.ZapfDingbats
	default:
		return "", fmt.Errorf("%w: DOCX symbol-font %q requires an explicit font-to-Unicode mapping", ErrUnsupported, font)
	}
	ch, ok := glyphsnames.GlyphToRune(enc[code])
	if !ok || enc[code] == "" {
		return "", fmt.Errorf("%w: unmapped DOCX symbol code", ErrUnsupported)
	}
	return string(ch), nil
}
