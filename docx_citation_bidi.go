package manifold

import (
	"fmt"

	"golang.org/x/text/unicode/bidi"
)

// Standard Unicode isolates work in readers that ignore OOXML w:dir. Checked
// roles remove only these exact generated controls, never arbitrary user text.
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
