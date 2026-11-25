package manifold

import (
	"encoding/xml"
	"fmt"
	"strings"
)

// A rich citation is an ordinary hyperlink with visible literal runs. The body
// target and reference style are checked before reusing ordinary text decoding.
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
