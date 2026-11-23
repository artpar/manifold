package manydoc

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

func docxPageMargins(l Layout) string {
	g := pageLayout(l)
	m := g.margins
	metadata := ""
	if l.Margins != nil {
		metadata += ` md:layoutMargins="1" md:layoutMargin="` + strconv.FormatFloat(l.Margin, 'g', -1, 64) + `"`
	}
	implicit := ""
	if l.HeaderDistance == nil {
		implicit += "h"
	}
	if l.FooterDistance == nil {
		implicit += "f"
	}
	if implicit != "" {
		metadata += ` md:layoutImplicit="` + implicit + `"`
	}
	return fmt.Sprintf(`<w:pgMar w:top="%.0f" w:right="%.0f" w:bottom="%.0f" w:left="%.0f" w:header="%.0f" w:footer="%.0f"%s/>`, m.Top*20, m.Right*20, m.Bottom*20, m.Left*20, g.headerDistance()*20, g.footerDistance()*20, metadata)
}
func docxPageNumber(n *xmlNode, key string, fallback float64) (float64, error) {
	v := n.attr(key)
	if v == "" {
		return fallback, nil
	}
	v = strings.TrimSpace(v)
	factor := 0.0
	for _, unit := range []struct {
		suffix string
		points float64
	}{{"mm", 72 / 25.4}, {"cm", 72 / 2.54}, {"in", 72}, {"pt", 1}, {"pc", 12}, {"pi", 12}} {
		if strings.HasSuffix(v, unit.suffix) {
			v = strings.TrimSuffix(v, unit.suffix)
			factor = unit.points
			break
		}
	}
	x, err := strconv.ParseFloat(v, 64)
	if err != nil || !finite(x) || !finite(x*factor) {
		return 0, fmt.Errorf("invalid DOCX page %s", key)
	}
	if factor == 0 {
		return x / 20, nil
	}
	return x * factor, nil
}
func (r *docxReader) pageMargins(n *xmlNode) error {
	left, err := docxPageNumber(n, "left", 54)
	if err != nil {
		return err
	}
	m := PageMargins{Left: left}
	for _, item := range []struct {
		key string
		p   *float64
	}{{"top", &m.Top}, {"right", &m.Right}, {"bottom", &m.Bottom}} {
		v, err := docxPageNumber(n, item.key, left)
		if err != nil {
			return err
		}
		*item.p = v
	}
	marker := n.attributeNS(docxNotesNS, "layoutMargins")
	if marker != "" && marker != "1" {
		return fmt.Errorf("invalid DOCX layoutMargins metadata")
	}
	r.doc.Layout.Margin = left
	r.doc.Layout.Margins = nil
	if marker == "1" || left == 0 || m.Top != left || m.Right != left || m.Bottom != left {
		r.doc.Layout.Margins = &m
	}
	if legacy := n.attributeNS(docxNotesNS, "layoutMargin"); legacy != "" {
		v, err := strconv.ParseFloat(legacy, 64)
		if marker != "1" || err != nil || !finite(v) || v < 0 {
			return fmt.Errorf("invalid DOCX legacy margin metadata")
		}
		r.doc.Layout.Margin = v
	}
	flags := n.attributeNS(docxNotesNS, "layoutImplicit")
	if flags != "" && flags != "h" && flags != "f" && flags != "hf" {
		return fmt.Errorf("invalid DOCX layoutImplicit metadata")
	}
	for _, item := range []struct {
		key, flag string
		p         **float64
	}{{"header", "h", &r.doc.Layout.HeaderDistance}, {"footer", "f", &r.doc.Layout.FooterDistance}} {
		*item.p = nil
		value := n.attr(item.key)
		if strings.Contains(flags, item.flag) {
			v, err := docxPageNumber(n, item.key, math.NaN())
			if err != nil || v != docxRunningDistance {
				return fmt.Errorf("DOCX implicit %s distance contradicts physical value", item.key)
			}
			continue
		}
		if value != "" {
			v, err := docxPageNumber(n, item.key, 0)
			if err != nil {
				return err
			}
			*item.p = &v
		}
	}
	if v := n.attr("gutter"); v != "" && v != "0" {
		r.doc.warn("docx_page_gutter", "Page gutter/binding offsets are not yet represented")
	}
	return nil
}

// Current visible section properties exclude revision-history snapshots, deleted
// stories and text-box content. A sectPr's children are not additional sections.
func docxActiveSections(n *xmlNode) []*xmlNode {
	if n == nil {
		return nil
	}
	var sections []*xmlNode
	for _, c := range n.Children {
		switch c.Name.Local {
		case "sectPr":
			sections = append(sections, c)
		case "sectPrChange", "pPrChange", "del", "moveFrom", "txbxContent":
		default:
			sections = append(sections, docxActiveSections(c)...)
		}
	}
	return sections
}
func (r *docxReader) pageSettings(n *xmlNode, section bool) error {
	for _, c := range n.Children {
		switch c.Name.Local {
		case "mirrorMargins", "gutterAtTop", "bookFoldPrinting", "bookFoldRevPrinting", "printTwoOnOne", "evenAndOddHeaders", "titlePg":
			on, err := docxOnOff(c)
			if err != nil {
				return err
			}
			if on {
				r.doc.warn("docx_page_settings", "Conditional page/running settings are not yet represented: "+c.Name.Local)
			}
		case "cols":
			if section && (c.attr("num") != "" && c.attr("num") != "1" || len(c.Children) > 0) {
				r.doc.warn("docx_page_columns", "Multiple page columns/column widths are not represented by the single flow")
			}
		case "pgBorders", "textDirection", "paperSrc", "vAlign", "lnNumType":
			if section {
				r.doc.warn("docx_page_settings", "Section page presentation is not represented: "+c.Name.Local)
			}
		case "sectPrChange":
			r.doc.warn("revisions", "Section revision history is omitted; current section properties are retained")
		}
	}
	return nil
}

func validateDOCXPageGeometry(g pageGeometry) error {
	values := []float64{g.width, g.height, g.margins.Top, g.margins.Right, g.margins.Bottom, g.margins.Left, g.headerDistance(), g.footerDistance()}
	for _, v := range values {
		if !finite(v * 20) {
			return fmt.Errorf("%w: DOCX page dimensions exceed twip serialization", ErrUnsupported)
		}
	}
	round := func(v float64) float64 { return math.RoundToEven(v*20) / 20 }
	w, h := round(g.width), round(g.height)
	m := g.margins
	if w <= 0 || h <= 0 || w-round(m.Left)-round(m.Right) <= 0 || h-math.Abs(round(m.Top))-math.Abs(round(m.Bottom)) <= 0 {
		return fmt.Errorf("%w: DOCX page/margins round to an empty physical container", ErrUnsupported)
	}
	if m.Top < 0 && round(m.Top) == 0 || m.Bottom < 0 && round(m.Bottom) == 0 {
		return fmt.Errorf("%w: DOCX signed page offset rounds to zero", ErrUnsupported)
	}
	return nil
}
