package engine

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"strings"
)

const officeMathNS = "http://schemas.openxmlformats.org/officeDocument/2006/math"

// OfficeMath stores native equation structure, not an inferred Unicode formula.
// PlainText returns source text tokens only; fractions/operators/layout remain in OMML.
type OfficeMath struct {
	OMML string `json:"omml"`
}

func parseOfficeMath(raw string) (*xmlNode, error) {
	if len(raw) == 0 || len(raw) > 1<<20 {
		return nil, fmt.Errorf("Office Math exceeds bounded native markup size")
	}
	root, err := parseXML(context.Background(), []byte(raw))
	if err != nil {
		return nil, err
	}
	if root.Name.Space != officeMathNS || (root.Name.Local != "oMath" && root.Name.Local != "oMathPara") {
		return nil, fmt.Errorf("invalid Office Math root")
	}
	var check func(*xmlNode) error
	check = func(n *xmlNode) error {
		if n.Name.Space != officeMathNS && n.Name.Space != wordNS {
			return fmt.Errorf("%w: Office Math namespace %s", ErrUnsupported, n.Name.Space)
		}
		if len(n.Children) > 0 && strings.TrimSpace(n.Text) != "" {
			return fmt.Errorf("unsupported mixed Office Math text")
		}
		for _, a := range n.Attr {
			if a.Name.Space == relNS {
				return fmt.Errorf("%w: Office Math package relationship", ErrUnsupported)
			}
		}
		for _, c := range n.Children {
			if err := check(c); err != nil {
				return err
			}
		}
		return nil
	}
	return root, check(root)
}
func serializeOfficeMath(n *xmlNode) (string, error) {
	var b bytes.Buffer
	enc := xml.NewEncoder(&b)
	var emit func(*xmlNode) error
	emit = func(n *xmlNode) error {
		start := xml.StartElement{Name: n.Name}
		for _, a := range n.Attr {
			if a.Name.Space != "xmlns" && !(a.Name.Space == "" && a.Name.Local == "xmlns") {
				start.Attr = append(start.Attr, a)
			}
		}
		if err := enc.EncodeToken(start); err != nil {
			return err
		}
		if err := enc.EncodeToken(xml.CharData(n.Text)); err != nil {
			return err
		}
		for _, c := range n.Children {
			if err := emit(c); err != nil {
				return err
			}
		}
		return enc.EncodeToken(start.End())
	}
	if err := emit(n); err != nil {
		return "", err
	}
	if err := enc.Flush(); err != nil {
		return "", err
	}
	raw := b.String()
	_, err := parseOfficeMath(raw)
	return raw, err
}
func (m *OfficeMath) PlainText() string {
	if m == nil {
		return ""
	}
	n, err := parseOfficeMath(m.OMML)
	if err != nil {
		return ""
	}
	var b strings.Builder
	var visit func(*xmlNode)
	visit = func(n *xmlNode) {
		if n.Name.Local == "t" {
			b.WriteString(n.Text)
		}
		for _, c := range n.Children {
			visit(c)
		}
	}
	visit(n)
	return b.String()
}
func documentHasOfficeMath(d *Document) bool {
	var walk func([]Block) bool
	walk = func(bs []Block) bool {
		for _, b := range bs {
			if b.Math != nil {
				return true
			}
			if b.Section != nil {
				if walk(b.Section.Blocks) {
					return true
				}
				for _, s := range sectionStories(b.Section) {
					if s != nil && walk(s.Blocks) {
						return true
					}
				}
			}
			for _, i := range b.Items {
				if walk(i.Blocks) || (i.Marker != nil && walk(i.Marker.Blocks)) {
					return true
				}
			}
			for _, r := range b.Rows {
				for _, c := range r.Cells {
					if walk(c.Blocks) {
						return true
					}
				}
			}
		}
		return false
	}
	if walk(d.Blocks) || walk(d.Headers) || walk(d.Footers) {
		return true
	}
	for _, n := range d.Notes {
		if walk(n.Blocks) {
			return true
		}
	}
	return false
}
