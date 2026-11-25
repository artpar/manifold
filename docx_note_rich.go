package manifold

import (
	"encoding/xml"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/rivo/uniseg"
)

const docxBodyLabelSeparator = `<w:r md:noteBodySeparator="1"><w:tab/><w:t xml:space="preserve">  </w:t></w:r>`

func noteRichBodyName(key noteKey) string { return "mdnoterich1_" + key.kind + "_" + key.id }
func docxNoteContentGutter(runs []Run) float64 {
	width := 0.0
	for _, run := range runs {
		size := run.FontSize
		if size == 0 {
			size = 11
		}
		width += float64(uniseg.StringWidth(run.Text)) * size
	}
	return math.Max(18, width+6)
}
func (w *docxWriter) richNoteBodyMark(n Note) (string, error) {
	encoded, err := w.blocksAt([]Block{{Kind: Paragraph, Runs: n.BodyLabelContent}}, 0)
	if err != nil {
		return "", err
	}
	start, end := strings.Index(encoded, `</w:pPr>`), strings.LastIndex(encoded, `</w:p>`)
	if start < 0 || end < start {
		return "", fmt.Errorf("missing rich label paragraph")
	}
	inline := encoded[start+len(`</w:pPr>`) : end]
	w.bookmarks++
	key := noteKey{n.Kind, strconv.Itoa(w.noteIDs[noteIdentity(n)])}
	return fmt.Sprintf(`<w:bookmarkStart w:id="%d" w:name="%s" md:noteBodyContent="1"/>%s<w:bookmarkEnd w:id="%d" md:noteBodyContent="1"/>`, w.bookmarks, noteRichBodyName(key), inline, w.bookmarks) + docxBodyLabelSeparator, nil
}

// Collect only a direct, checked inline bookmark range. All label text and styles
// remain ordinary visible runs; the range stores identity, never duplicate text.
func (r *docxReader) collectRichBodyRanges(n *xmlNode) ([]*xmlNode, map[*xmlNode][]*xmlNode, error) {
	children := n.Children
	if !r.expectedBodyRich || r.inNote == 0 || r.bodyLabelSeen {
		return children, nil, nil
	}
	var ranges map[*xmlNode][]*xmlNode
	var out []*xmlNode
	for i := 0; i < len(children); i++ {
		c := children[i]
		role := c.attributeNS(docxNotesNS, "noteBodyContent")
		if role == "" {
			if ranges != nil {
				out = append(out, c)
			}
			continue
		}
		if role != "1" || c.Name.Local != "bookmarkStart" || !r.expectedBodyRich || r.inNote == 0 || r.bodyLabelSeen || c.attr("name") != noteRichBodyName(r.expectedBodyWire) || len(c.Children) != 0 || strings.TrimSpace(c.Text) != "" {
			return nil, nil, fmt.Errorf("unexpected rich note label range")
		}
		if err := checkVariantNamespaces(c); err != nil {
			return nil, nil, err
		}
		if len(ranges) != 0 {
			return nil, nil, fmt.Errorf("duplicate rich note body range")
		}
		end := i + 1
		for end < len(children) && children[end].Name.Local != "bookmarkEnd" {
			end++
		}
		if end == len(children) {
			return nil, nil, fmt.Errorf("unclosed rich note body range")
		}
		close := children[end]
		if close.attr("id") != c.attr("id") || close.attributeNS(docxNotesNS, "noteBodyContent") != "1" || len(close.Children) != 0 || strings.TrimSpace(close.Text) != "" {
			return nil, nil, fmt.Errorf("rich note body range boundary mismatch")
		}
		if err := checkVariantNamespaces(close); err != nil {
			return nil, nil, err
		}
		if ranges == nil {
			ranges = map[*xmlNode][]*xmlNode{}
			out = append(out, children[:i]...)
		}
		ranges[c] = children[i+1 : end]
		out = append(out, c)
		i = end
	}
	if ranges == nil {
		return children, nil, nil
	}
	return out, ranges, nil
}
func (r *docxReader) readRichBodyRuns(children []*xmlNode, rels map[string]docxRelation) error {
	if !r.expectedBodyRich || r.inNote == 0 || r.bodyLabelSeen {
		return fmt.Errorf("invalid rich note body label")
	}
	for _, child := range children {
		if err := checkVariantNamespaces(child); err != nil {
			return err
		}
		if err := validateRichBodyInline(child); err != nil {
			return err
		}
	}
	blocks, err := r.paragraph(&xmlNode{Name: xml.Name{Space: wordNS, Local: "p"}, Children: children}, rels)
	if err != nil {
		return err
	}
	if len(blocks) != 1 || blocks[0].Kind != Paragraph {
		return fmt.Errorf("rich note label contains nontext content")
	}
	var runs []Run
	for _, run := range blocks[0].Runs {
		if run.Note != nil || run.Anchor != "" {
			return fmt.Errorf("invalid rich note label atom")
		}
		if run.Text == "" {
			r.doc.warn("docx_rich_label_empty_span", "Empty body-mark spans are normalized; their formatting/navigation is not represented")
			continue
		}
		runs = append(runs, run)
	}
	if len(runs) == 0 {
		r.bodyLabelSeen = true
		r.parsedBodyMarkerless = true
		r.doc.warn("docx_rich_label_empty", "An empty final-visible rich body mark is an explicit markerless override; empty-span formatting/navigation is not represented")
		return nil
	}
	if err := validateNoteLabel(noteLabelContentText(runs)); err != nil {
		return err
	}
	r.parsedBodyContent = runs
	r.bodyLabelSeen = true
	return nil
}
func validateRichBodyInline(n *xmlNode) error {
	if strings.TrimSpace(n.Text) != "" {
		return fmt.Errorf("unexpected rich label container text")
	}
	var checkMetadata func(*xmlNode) error
	checkMetadata = func(v *xmlNode) error {
		for _, a := range v.Attr {
			if a.Name.Space == docxNotesNS {
				switch a.Name.Local {
				case "textImplicit", "fontSize", "destination", "href":
				default:
					return fmt.Errorf("unexpected private rich label role")
				}
			}
		}
		for _, kid := range v.Children {
			if err := checkMetadata(kid); err != nil {
				return err
			}
		}
		return nil
	}
	if err := checkMetadata(n); err != nil {
		return err
	}
	switch n.Name.Local {
	case "del", "moveFrom":
		return nil // Ordinary paragraph decoding diagnoses/excludes deleted history.
	case "ins", "moveTo":
		for _, child := range n.Children {
			if err := validateRichBodyInline(child); err != nil {
				return err
			}
		}
		return nil
	case "commentRangeStart", "commentRangeEnd", "commentReference":
		if len(n.Children) != 0 {
			return fmt.Errorf("nonempty rich label annotation marker")
		}
		return nil
	}
	run := n
	if n.Name.Local == "hyperlink" {
		if len(n.Children) != 1 || n.Children[0].Name.Local != "r" {
			return fmt.Errorf("rich label link must contain a single run")
		}
		run = n.Children[0]
	} else if n.Name.Local != "r" {
		return fmt.Errorf("unsupported rich label element %s", n.Name.Local)
	}
	if marker := run.child("commentReference"); marker != nil && (len(run.Children) == 1 || (len(run.Children) == 2 && run.Children[0].Name.Local == "rPr")) {
		return validateRichBodyInline(marker)
	}
	text := run.child("t")
	if len(run.Children) > 2 || strings.TrimSpace(run.Text) != "" || (len(run.Children) == 2 && run.Children[0].Name.Local != "rPr") || (text != nil && len(text.Children) != 0) || (text == nil && len(run.Children) > 0 && (len(run.Children) != 1 || run.Children[0].Name.Local != "rPr")) {
		return fmt.Errorf("rich label requires ordinary literal text runs")
	}
	if pr := run.child("rPr"); pr != nil {
		for _, p := range pr.Children {
			if len(p.Children) != 0 || strings.TrimSpace(p.Text) != "" {
				return fmt.Errorf("nonempty rich label property")
			}
			switch p.Name.Local {
			case "rFonts", "sz", "szCs", "color", "lang", "cs", "rtl", "b", "bCs", "i", "iCs", "u", "strike":
			default:
				return fmt.Errorf("unsupported rich label property %s", p.Name.Local)
			}
		}
	}
	return nil
}

func (r *docxReader) richBodyIndent(n *xmlNode) string {
	runs := r.parsedBodyContent
	if runs == nil {
		inside := false
		for _, child := range n.Children {
			if child.Name.Local == "bookmarkStart" && child.attributeNS(docxNotesNS, "noteBodyContent") == "1" {
				inside = true
				continue
			}
			if inside && child.Name.Local == "bookmarkEnd" {
				break
			}
			if !inside {
				continue
			}
			run := child
			if child.Name.Local == "hyperlink" {
				run = child.child("r")
			}
			if run == nil {
				continue
			}
			size := 0.0
			if !strings.Contains(run.attributeNS(docxNotesNS, "textImplicit"), "s") {
				if h, err := strconv.ParseUint(run.child("rPr").child("sz").attr("val"), 10, 32); err == nil {
					size = float64(h) / 2
				}
			}
			if original := run.attributeNS(docxNotesNS, "fontSize"); original != "" {
				if exact, err := strconv.ParseFloat(original, 64); err == nil && finite(exact) {
					size = exact
				}
			}
			runs = append(runs, Run{Text: run.child("t").content(), FontSize: size})
		}
	}
	return strconv.Itoa(int(math.Ceil(docxNoteContentGutter(runs) * 20)))
}
