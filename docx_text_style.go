package manydoc

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

func (r *docxReader) textStyleChain(run *Run, id string) error {
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
		if err := applyStyleRunProps(run, chain[i].child("rPr")); err != nil {
			return err
		}
		if err := r.textProperties(run, chain[i].child("rPr")); err != nil {
			return err
		}
	}
	return nil
}
func (r *docxReader) paragraphTextStyle(id string) (Run, error) {
	run := Run{}
	r.warnTextStyleProperties(r.defaultRPr, "")
	applyRunProps(&run, r.defaultRPr)
	if err := r.textProperties(&run, r.defaultRPr); err != nil {
		return run, err
	}
	if id == "" {
		id = r.defaultParagraphStyle
	}
	err := r.textStyleChain(&run, id)
	return run, err
}
func (r *docxReader) themeFont(value string) string {
	name := ""
	if strings.HasPrefix(value, "major") {
		name = "majorFont"
	}
	if strings.HasPrefix(value, "minor") {
		name = "minorFont"
	}
	if name == "" {
		return ""
	}
	return r.theme.child("themeElements").child("fontScheme").child(name).child("latin").attr("typeface")
}
func (r *docxReader) textProperties(run *Run, pr *xmlNode) error {
	if pr == nil {
		return nil
	}
	seen := map[string]bool{}
	for _, p := range pr.Children {
		if err := r.ctx.Err(); err != nil {
			return err
		}
		kind := p.Name.Local
		if kind != "rFonts" && kind != "sz" && kind != "color" {
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
			family := p.attr("ascii")
			if family == "" {
				family = p.attr("hAnsi")
			}
			theme := p.attr("asciiTheme")
			if theme == "" {
				theme = p.attr("hAnsiTheme")
			}
			if theme != "" {
				resolved := r.themeFont(theme)
				if resolved != "" {
					family = resolved
				}
				r.doc.warn("font_theme", "Theme font is resolved to a family; editable theme binding is not retained")
				if family == "" {
					return fmt.Errorf("%w: unresolved DOCX theme font %q", ErrUnsupported, theme)
				}
			}
			if family != "" {
				run.FontFamily = family
			}
			for _, name := range []string{"hAnsi", "eastAsia", "cs"} {
				if v := p.attr(name); v != "" && v != family {
					r.doc.warn("font_scripts", "Distinct script font choices are normalized to the primary family")
				}
			}
			if p.attr("eastAsiaTheme") != "" || p.attr("cstheme") != "" || p.attr("csTheme") != "" {
				r.doc.warn("font_scripts", "Script-specific theme font choices are not retained")
			}
		case "color":
			value, err := r.textColor(p)
			if err != nil {
				return err
			}
			run.Color = value
		}
	}
	return validateTextStyle(*run)
}
func docxTextStyle(run Run) (string, error) {
	var out strings.Builder
	if run.FontFamily != "" {
		fmt.Fprintf(&out, `<w:rFonts w:ascii="%s" w:hAnsi="%s" w:eastAsia="%s" w:cs="%s"/>`, escape(run.FontFamily), escape(run.FontFamily), escape(run.FontFamily), escape(run.FontFamily))
	}
	if run.FontSize > 0 {
		h := run.FontSize * 2
		if h != math.Trunc(h) || h < 1 || h > math.MaxUint32 {
			return "", fmt.Errorf("%w: DOCX font size requires positive half-points", ErrUnsupported)
		}
		fmt.Fprintf(&out, `<w:sz w:val="%.0f"/>`, h)
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

// OOXML style toggles differ from absolute direct run formatting.
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
		case "b", "i", "strike", "u", "rFonts", "sz", "color", "rStyle":
		case "vertAlign":
			if id != "FootnoteReference" && id != "EndnoteReference" {
				r.doc.warn("run_formatting", "Style property not represented: vertAlign")
			}
		default:
			r.doc.warn("run_formatting", "Style property not represented: "+p.Name.Local)
		}
	}
}
