package manifold

import (
	"fmt"
	"strconv"
	"strings"
)

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
