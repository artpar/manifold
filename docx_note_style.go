package manydoc

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/rivo/uniseg"
)

// A styled label is ordinary portable Word text/hyperlink content. Its checked
// role restores a style template instead of duplicating it in the body story.
func (w *docxWriter) styledNoteBodyMark(n Note) (string, error) {
	run := noteBodyLabelRun(w.noteCatalog, noteIdentity(n))
	if run.Text != "" {
		run.Text += " "
	}
	encoded, err := w.blocksAt([]Block{{Kind: Paragraph, Runs: []Run{run}}}, 0)
	if err != nil {
		return "", err
	}
	if run.Text == "" {
		encoded = strings.Replace(encoded, `</w:r>`, `<w:t/></w:r>`, 1)
	}
	start := strings.Index(encoded, `</w:pPr>`)
	end := strings.LastIndex(encoded, `</w:p>`)
	if start < 0 || end < start {
		return "", fmt.Errorf("missing styled note label paragraph")
	}
	inline := encoded[start+len(`</w:pPr>`) : end]
	if strings.HasPrefix(inline, `<w:hyperlink`) {
		inline = strings.Replace(inline, `<w:hyperlink`, `<w:hyperlink md:noteBodyStyle="1"`, 1)
	} else {
		inline = strings.Replace(inline, `<w:r`, `<w:r md:noteBodyStyle="1"`, 1)
	}
	return inline + `<w:r md:noteBodySeparator="1"><w:tab/><w:t xml:space="preserve">  </w:t></w:r>`, nil
}
func (r *docxReader) readStyledBodyMark(n *xmlNode, rels map[string]docxRelation) error {
	if !r.expectedBodyStyle || r.expectedBodyLabel == nil || r.inNote == 0 || r.bodyLabelSeen || n.attributeNS(docxNotesNS, "noteBodyStyle") != "1" {
		return fmt.Errorf("unexpected styled note body mark")
	}
	if err := checkVariantNamespaces(n); err != nil {
		return err
	}
	run := n
	if n.Name.Local == "hyperlink" {
		if len(n.Children) != 1 || n.Children[0].Name.Local != "r" {
			return fmt.Errorf("styled note label must have one text run")
		}
		run = n.Children[0]
	}
	if run.Name.Local != "r" || len(run.Children) == 0 || len(run.Children) > 2 || run.child("t") == nil || (len(run.Children) == 2 && run.Children[0].Name.Local != "rPr") || len(run.child("t").Children) != 0 {
		return fmt.Errorf("styled note label contains nontext content")
	}
	expected := *r.expectedBodyLabel
	if expected != "" {
		expected += " "
	}
	if n.content() != expected {
		return fmt.Errorf("styled note label contradicts expected text")
	}
	if props := run.child("rPr"); props != nil {
		for _, p := range props.Children {
			if len(p.Children) != 0 || p.Text != "" {
				return fmt.Errorf("nonempty styled note property")
			}
			switch p.Name.Local {
			case "rFonts", "sz", "szCs", "color", "lang", "cs", "rtl", "b", "bCs", "i", "iCs", "u", "strike":
			default:
				return fmt.Errorf("unsupported styled note label property %s", p.Name.Local)
			}
		}
	}
	copy := *n
	copy.Attr = nil
	for _, a := range n.Attr {
		if a.Name.Space != docxNotesNS || a.Name.Local != "noteBodyStyle" {
			copy.Attr = append(copy.Attr, a)
		}
	}
	// Decode a nonempty sentinel after checking the actual visible mark. The
	// ordinary parser drops empty text, while a markerless note still has style.
	runCopy := *run
	runCopy.Children = append([]*xmlNode(nil), run.Children...)
	for i, child := range runCopy.Children {
		if child.Name.Local == "t" {
			textCopy := *child
			textCopy.Text = "label"
			runCopy.Children[i] = &textCopy
		}
	}
	if n.Name.Local == "hyperlink" {
		copy.Children = []*xmlNode{&runCopy}
	} else {
		copy.Children = runCopy.Children
	}
	paragraph := &xmlNode{Name: n.Name, Children: []*xmlNode{&copy}}
	paragraph.Name.Local = "p"
	blocks, err := r.paragraph(paragraph, rels)
	if err != nil {
		return err
	}
	if len(blocks) != 1 || blocks[0].Kind != Paragraph || len(blocks[0].Runs) != 1 || blocks[0].Runs[0].Text != "label" || blocks[0].Runs[0].Note != nil || blocks[0].Runs[0].Anchor != "" {
		return fmt.Errorf("styled note label contradicts expected text")
	}
	style := blocks[0].Runs[0]
	style.Text = ""
	r.parsedBodyStyle = &style
	r.bodyLabelSeen = true
	return nil
}
func (r *docxReader) readBodyLabelSeparator(n *xmlNode) error {
	if n.attributeNS(docxNotesNS, "noteBodySeparator") != "1" || !r.expectedBodyStyle || !r.bodyLabelSeen || r.bodySeparatorSeen || r.inNote == 0 || len(n.Children) != 2 || n.Children[0].Name.Local != "tab" || len(n.Children[0].Children) != 0 || n.Children[1].Name.Local != "t" || len(n.Children[1].Children) != 0 || n.Children[1].Text != "  " || n.content() != "  " {
		return fmt.Errorf("invalid styled label separator")
	}
	if err := checkVariantNamespaces(n); err != nil {
		return err
	}
	r.bodySeparatorSeen = true
	return nil
}

func docxNoteLabelGutter(label string, size float64) float64 {
	if size == 0 {
		size = 11
	}
	return math.Max(18, float64(uniseg.StringWidth(label))*size+6)
}

// Only the derived layout of our checked styled mark is implicit. A foreign
// indentation/tab edit still gets the ordinary representation-loss diagnostic.
func (r *docxReader) implicitNoteIndent(n *xmlNode) string {
	if !r.expectedBodyStyle || r.expectedBodyLabel == nil {
		return "360"
	}
	size := 0.0
	if r.parsedBodyStyle != nil {
		size = r.parsedBodyStyle.FontSize
	} else {
		var run *xmlNode
		for _, child := range n.Children {
			if child.attributeNS(docxNotesNS, "noteBodyStyle") == "1" {
				run = child
				if child.Name.Local == "hyperlink" {
					run = child.child("r")
				}
				break
			}
		}
		if run != nil && !strings.Contains(run.attributeNS(docxNotesNS, "textImplicit"), "s") {
			value, err := strconv.ParseUint(run.child("rPr").child("sz").attr("val"), 10, 32)
			if err == nil {
				size = float64(value) / 2
			}
			if original := run.attributeNS(docxNotesNS, "fontSize"); original != "" {
				if exact, err := strconv.ParseFloat(original, 64); err == nil && finite(exact) {
					size = exact
				}
			}
		}
	}
	return strconv.Itoa(int(math.Ceil(docxNoteLabelGutter(*r.expectedBodyLabel, size) * 20)))
}
