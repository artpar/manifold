package manydoc

import (
	"fmt"
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
	var found *xmlNode
	if pr != nil {
		for _, c := range pr.Children {
			if c.Name.Local == "pageBreakBefore" {
				if found != nil {
					return nil, fmt.Errorf("duplicate pageBreakBefore property")
				}
				found = c
			}
		}
	}
	return found, nil
}
func (r *docxReader) paragraphPageBreak(pr *xmlNode, styleID string) (bool, error) {
	value := false
	apply := func(pr *xmlNode) error {
		p, err := paragraphBreakProperty(pr)
		if err != nil {
			return err
		}
		if p != nil {
			value, err = docxOnOff(p)
		}
		return err
	}
	if err := apply(r.defaultPPr); err != nil {
		return false, err
	}
	if styleID == "" {
		styleID = r.defaultParagraphStyle
	}
	visited := map[string]bool{}
	var chain []*xmlNode
	for styleID != "" {
		if err := r.ctx.Err(); err != nil {
			return false, err
		}
		if visited[styleID] {
			return false, fmt.Errorf("cyclic DOCX paragraph style inheritance")
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
			return false, err
		}
	}
	if err := apply(pr); err != nil {
		return false, err
	}
	return value, nil
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
func docxStructuralParagraph(parent, n *xmlNode) (bool, error) {
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
	if empty && tail || parent.Name.Local != "tc" || len(n.Children) > 0 || strings.TrimSpace(n.Text) != "" || n.attributeNS(docxNotesNS, "breakRole") != "" {
		return false, fmt.Errorf("structural paragraph metadata contradicts content")
	}
	var content []*xmlNode
	for _, c := range parent.Children {
		if c.Name.Local != "tcPr" {
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
