package engine

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"github.com/rivo/uniseg"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
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

		// A styled label is ordinary portable Word text/hyperlink content. Its checked
		// role restores a style template instead of duplicating it in the body story.

	// Decode a nonempty sentinel after checking the actual visible mark. The
	// ordinary parser drops empty text, while a markerless note still has style.

	// Only the derived layout of our checked styled mark is implicit. A foreign
	// indentation/tab edit still gets the ordinary representation-loss diagnostic.

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

			// Exactly one ordinary text run: no cached fields, drawings, tabs or hidden content.

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
	return inline + docxBodyLabelSeparator, nil
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
	if n.attributeNS(docxNotesNS, "noteBodySeparator") != "1" || (!r.expectedBodyStyle && !r.expectedBodyRich) || !r.bodyLabelSeen || r.bodySeparatorSeen || r.inNote == 0 || len(n.Children) != 2 || n.Children[0].Name.Local != "tab" || len(n.Children[0].Children) != 0 || n.Children[1].Name.Local != "t" || len(n.Children[1].Children) != 0 || n.Children[1].Text != "  " || n.content() != "  " {
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

func (r *docxReader) implicitNoteIndent(n *xmlNode) string {
	if r.expectedBodyRich {
		return r.richBodyIndent(n)
	}
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

const docxNoteAnchorMark = "\u200b"

type docxNoteDisplay struct {
	Rich      bool    `json:"rich,omitempty"`
	Label     string  `json:"label"`
	BodyLabel *string `json:"body_label"`
	Value     string  `json:"value"`
	Variant   bool    `json:"variant"`
	Styled    bool    `json:"styled,omitempty"`
}

func noteBodyName(key noteKey) string { return "mdnotebody1_" + key.kind + "_" + key.id }
func parseNoteBodyName(name string) (*NoteReference, error) {
	if !strings.HasPrefix(name, "mdnotebody1_") {
		return nil, fmt.Errorf("invalid note body name")
	}
	return parseNoteBookmark("mdnote1_" + strings.TrimPrefix(name, "mdnotebody1_"))
}
func (w *docxWriter) variantNoteReference(run Run, block Block) (string, error) {
	run = canonicalCitationRun(run)
	key := referenceKey(run.Note)
	id := w.noteIDs[key]
	wire := noteKey{key.kind, strconv.Itoa(id)}
	var out strings.Builder
	if !w.seenNoteRefs[key] {
		w.seenNoteRefs[key] = true
		w.bookmarks++
		name := fmt.Sprintf("mdnote1_%s_%d", key.kind, id)
		fmt.Fprintf(&out, `<w:bookmarkStart w:id="%d" w:name="%s"/><w:r md:noteAnchor="%s"><w:%sReference w:id="%d" w:customMarkFollows="1"/><w:t>%s</w:t></w:r><w:bookmarkEnd w:id="%d"/>`, w.bookmarks, name, name, key.kind, id, docxNoteAnchorMark, w.bookmarks)
	}
	literal := run
	literal.Note = nil
	literal.Text = noteReferenceLabel(w.noteCatalog, run.Note)
	rich := run.Note.LabelContent != nil
	runs := []Run{literal}
	kind, level := block.Kind, block.Level
	if rich {
		runs = run.Note.LabelContent
		kind = Paragraph
		level = 0
	}
	encoded, err := w.blocksAt([]Block{{Kind: kind, Level: level, Runs: runs}}, 0)
	if err != nil {
		return "", err
	}
	if rich {
		encoded = strings.ReplaceAll(encoded, `<w:rPr>`, `<w:rPr><w:rStyle w:val="`+strings.Title(key.kind)+`Reference"/>`)
	} else {
		encoded = strings.Replace(encoded, `<w:rPr>`, `<w:rPr><w:rStyle w:val="`+strings.Title(key.kind)+`Reference"/>`, 1)
	}
	if !rich && literal.Text == "" {
		encoded = strings.Replace(encoded, `</w:r>`, `<w:t/></w:r>`, 1)
	}
	start := strings.Index(encoded, `</w:pPr>`)
	end := strings.LastIndex(encoded, `</w:p>`)
	if start < 0 || end < start {
		return "", fmt.Errorf("missing citation paragraph")
	}
	override := "0"
	if run.Note.Label != nil {
		override = "1"
	}
	if rich {
		override = "rich"
	}
	name := noteBodyName(wire)
	content := encoded[start+len(`</w:pPr>`) : end]
	isolation := ""
	if rich || citationNeedsIsolation(literal.Text) {
		isolation = ` md:noteIsolation="ltr"`
		content = docxCitationIsolationStart + content + docxCitationIsolationEnd
	}
	fmt.Fprintf(&out, `<w:hyperlink w:anchor="%s" md:noteCitation="%s" md:noteOverride="%s"%s%s>%s</w:hyperlink>`, name, name, override, docxNoteInference(run.Note), isolation, content)
	return out.String(), nil
}
func (w *docxWriter) noteDisplayMetadata(n Note) string {
	if len(w.noteVariants) == 0 {
		return ""
	}
	key := noteIdentity(n)
	value := docxNoteDisplay{Label: n.Label, BodyLabel: n.BodyLabel, Value: w.noteLabels[key], Variant: w.noteVariants[key], Styled: n.BodyLabelStyle != nil, Rich: n.BodyLabelContent != nil}
	raw, _ := json.Marshal(value)
	return ` md:display="` + escape(string(raw)) + `"`
}
func readNoteDisplay(n *xmlNode) (*docxNoteDisplay, error) {
	raw := n.attributeNS(docxNotesNS, "display")
	if raw == "" {
		return nil, nil
	}
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.DisallowUnknownFields()
	var value docxNoteDisplay
	if err := dec.Decode(&value); err != nil {
		return nil, err
	}
	if dec.Decode(new(any)) != io.EOF {
		return nil, fmt.Errorf("invalid note display metadata")
	}
	if (value.BodyLabel != nil || value.Styled || value.Rich) && !value.Variant {
		return nil, fmt.Errorf("body override requires variant wire format")
	}
	if value.Rich && (value.Styled || value.BodyLabel != nil) {
		return nil, fmt.Errorf("rich label has conflicting body text/style authorities")
	}
	if value.Value == "" {
		return nil, fmt.Errorf("empty canonical note mark metadata")
	}
	for _, s := range []string{value.Label, value.Value} {
		if err := validateNoteLabel(s); err != nil {
			return nil, err
		}
	}
	if value.BodyLabel != nil {
		if err := validateNoteLabel(*value.BodyLabel); err != nil {
			return nil, err
		}
	}
	return &value, nil
}
func (r *docxReader) hiddenNoteAnchor(n *xmlNode) (*NoteReference, error) {
	if err := checkVariantNamespaces(n); err != nil {
		return nil, err
	}
	ref, err := parseNoteBookmark(n.attributeNS(docxNotesNS, "noteAnchor"))
	if err != nil {
		return nil, err
	}
	if len(n.Children) != 2 || n.Children[0].Name.Local != ref.Kind+"Reference" || n.Children[0].attr("id") != ref.ID || n.Children[0].attr("customMarkFollows") != "1" || n.Children[1].Name.Local != "t" || n.Children[1].content() != docxNoteAnchorMark || n.content() != docxNoteAnchorMark {
		return nil, fmt.Errorf("note anchor metadata contradicts content")
	}
	if r.variantAnchors == nil {
		r.variantAnchors = map[noteKey]bool{}
	}
	key := referenceKey(ref)
	if r.variantAnchors[key] {
		return nil, fmt.Errorf("duplicate variant note anchor")
	}
	r.variantAnchors[key] = true
	return ref, nil
}
func (r *docxReader) variantCitation(n *xmlNode) (*NoteReference, error) {
	if err := checkVariantNamespaces(n); err != nil {
		return nil, err
	}
	name := n.attributeNS(docxNotesNS, "noteCitation")
	ref, err := parseNoteBodyName(name)
	if err != nil {
		return nil, err
	}
	if err := readDocxNoteInference(n, ref); err != nil {
		return nil, err
	}
	if n.attributeNS(docxNotesNS, "noteOverride") == "rich" {
		return r.richCitation(n, ref)
	}
	if n.attr("anchor") != name || n.attr("id") != "" || len(n.Children) != 1 || n.Children[0].Name.Local != "r" {
		return nil, fmt.Errorf("note citation target/shape contradicts metadata")
	}
	run := n.Children[0]

	if len(run.Children) == 0 || len(run.Children) > 2 || run.child("t") == nil || (len(run.Children) == 2 && run.Children[0].Name.Local != "rPr") || len(run.child("t").Children) != 0 {
		return nil, fmt.Errorf("note citation metadata requires literal text")
	}
	if props := run.child("rPr"); props != nil {
		for _, prop := range props.Children {
			switch prop.Name.Local {
			case "rFonts", "sz", "szCs", "color", "lang", "cs", "rtl", "b", "bCs", "i", "iCs", "u", "strike":
			case "rStyle":
				if prop.attr("val") != strings.Title(ref.Kind)+"Reference" {
					return nil, fmt.Errorf("invalid citation reference style")
				}
			default:
				return nil, fmt.Errorf("unsupported property in checked note citation: %s", prop.Name.Local)
			}
		}
	}
	text := run.child("t").content()
	if err := validateNoteLabel(text); err != nil {
		return nil, err
	}
	switch n.attributeNS(docxNotesNS, "noteOverride") {
	case "1":
		ref.Label = &text
	case "0":
		if r.variantDefaultMarks == nil {
			r.variantDefaultMarks = map[noteKey][]string{}
		}
		r.variantDefaultMarks[referenceKey(ref)] = append(r.variantDefaultMarks[referenceKey(ref)], text)
	default:
		return nil, fmt.Errorf("invalid note citation override metadata")
	}
	if r.variantCitations == nil {
		r.variantCitations = map[noteKey]bool{}
	}
	r.variantCitations[referenceKey(ref)] = true
	return ref, nil
}
func (r *docxReader) variantBodyMark(n *xmlNode) error {
	if err := checkVariantNamespaces(n); err != nil {
		return err
	}
	expected := ""
	if r.expectedBodyLabel != nil {
		expected = *r.expectedBodyLabel
		if expected != "" {
			expected += " "
		}
	}
	if n.attributeNS(docxNotesNS, "noteBodyLabel") != "1" || r.expectedBodyLabel == nil || r.inNote == 0 || r.bodyLabelSeen || len(n.Children) != 3 || n.Children[0].Name.Local != "rPr" || n.Children[1].Name.Local != "t" || n.Children[1].content() != expected || len(n.Children[1].Children) != 0 || n.Children[2].Name.Local != "tab" || len(n.Children[2].Children) != 0 {
		return fmt.Errorf("note body label metadata contradicts content")
	}
	props := n.Children[0]
	if len(props.Children) != 2 || props.Children[0].Name.Local != "sz" || props.Children[1].Name.Local != "szCs" {
		return fmt.Errorf("invalid generated body mark style")
	}
	for _, p := range props.Children {
		if p.attr("val") != "16" || len(p.Attr) != 1 || len(p.Children) != 0 {
			return fmt.Errorf("invalid generated body mark size")
		}
	}
	r.bodyLabelSeen = true
	return nil
}

func validateNoteBodyBookmark(n *xmlNode, key noteKey) error {
	name := noteBodyName(key)
	id := ""
	count := 0
	for _, node := range n.all("bookmarkStart") {
		if node.attr("name") == name {
			if err := checkVariantNamespaces(node); err != nil {
				return err
			}
			id = node.attr("id")
			count++
		}
	}
	if count != 1 {
		return fmt.Errorf("variant note lacks unique body target")
	}
	ends := 0
	for _, node := range n.all("bookmarkEnd") {
		if node.attr("id") == id {
			ends++
		}
	}
	if ends != 1 {
		return fmt.Errorf("variant note body target is unclosed")
	}
	return nil
}

// These checked generated fragments cannot borrow lookalike element/attribute
// names from another namespace to hide visible content or retarget a note.

// The canonical model numbers each note kind once across the document. Emit
// this policy explicitly in every section rather than relying on producer
// defaults (notably for endnotes), including sections with an empty body.

// The first line's number hangs before the note's text edge.

func checkVariantNamespaces(n *xmlNode) error {
	if n.Name.Space != wordNS && n.Name.Space != strictWordNS {
		return fmt.Errorf("invalid namespace in generated note fragment")
	}
	for _, a := range n.Attr {
		if n.Name.Local == "hyperlink" && a.Name.Local == "id" && (a.Name.Space == relNS || a.Name.Space == "http://purl.oclc.org/ooxml/officeDocument/relationships") {
			continue
		}
		if a.Name.Local == "id" || a.Name.Local == "name" || a.Name.Local == "anchor" || a.Name.Local == "customMarkFollows" {
			if a.Name.Space != n.Name.Space {
				return fmt.Errorf("lookalike note binding attribute")
			}
		}
		switch a.Name.Space {
		case n.Name.Space, docxNotesNS, "http://www.w3.org/XML/1998/namespace", "xmlns":
		default:
			return fmt.Errorf("invalid attribute namespace in generated note fragment")
		}
	}
	for _, child := range n.Children {
		if err := checkVariantNamespaces(child); err != nil {
			return err
		}
	}
	return nil
}

func docxNoteInference(ref *NoteReference) string {
	if ref.Inferred {
		return ` md:inferred="1"`
	}
	return ""
}
func readDocxNoteInference(n *xmlNode, ref *NoteReference) error {
	switch n.attributeNS(docxNotesNS, "inferred") {
	case "":
	case "1":
		ref.Inferred = true
	default:
		return fmt.Errorf("invalid note relationship inference metadata")
	}
	return nil
}

const docxNotesNS = "urn:manydoc:metadata:1"
const markupCompatibilityNS = "http://schemas.openxmlformats.org/markup-compatibility/2006"

func docxNoteProperties(counts map[string]int) string {
	var properties strings.Builder
	for _, kind := range []string{Footnote, Endnote} {
		if counts[kind] > 0 {
			properties.WriteString(`<w:` + kind + `Pr><w:numFmt w:val="decimal"/><w:numRestart w:val="continuous"/></w:` + kind + `Pr>`)
		}
	}
	return properties.String()
}

func (n *xmlNode) attributeNS(space, name string) string {
	if n == nil {
		return ""
	}
	for _, a := range n.Attr {
		if a.Name.Space == space && a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}
func (w *docxWriter) writeNoteParts(notes []Note) error {
	for _, kind := range []string{Footnote, Endnote} {
		var out strings.Builder
		count := 0
		fmt.Fprintf(&out, `%s<w:%ss%s xmlns:md="%s" xmlns:mc="%s" mc:Ignorable="md">`, xmlHeader, kind, wordNamespaces, docxNotesNS, markupCompatibilityNS)
		fmt.Fprintf(&out, `<w:%s w:type="separator" w:id="-1"><w:p><w:r><w:separator/></w:r></w:p></w:%s><w:%s w:type="continuationSeparator" w:id="0"><w:p><w:r><w:continuationSeparator/></w:r></w:p></w:%s>`, kind, kind, kind, kind)
		mainRels := w.rels
		w.rels = nil
		for order, n := range notes {
			if err := w.ctx.Err(); err != nil {
				w.rels = mainRels
				return err
			}
			if n.Kind != kind {
				continue
			}
			count++
			indent := 360
			if n.BodyLabelStyle != nil || n.BodyLabelContent != nil {
				gutter := docxNoteContentGutter(noteBodyLabelRuns(w.noteCatalog, noteIdentity(n)))
				if gutter >= w.contentWidth {
					return fmt.Errorf("%w: styled note label leaves no DOCX body width", ErrUnsupported)
				}
				indent = int(math.Ceil(gutter * 20))
			}
			indentXML := strconv.Itoa(indent)
			blocks := append([]Block(nil), n.Blocks...)
			for i := range blocks {
				if blocks[i].Kind != ListBlock {
					blocks[i].docxIndent = indent
				}
			}
			previousHeight := w.contentHeight
			w.contentHeight -= math.Min(docxImageNoteHeightAllowance, w.contentHeight/2)
			previousNoteStory := w.noteStory
			w.noteStory = true
			body, err := w.blocksAt(blocks, 1)
			w.noteStory = previousNoteStory
			w.contentHeight = previousHeight
			if err != nil {
				w.rels = mainRels
				return err
			}
			marker := `<w:r><w:rPr><w:rStyle w:val="` + strings.Title(kind) + `Reference"/></w:rPr><w:` + kind + `Ref/><w:tab/></w:r>`

			if w.noteVariants[noteIdentity(n)] {
				label := noteBodyLabel(w.noteCatalog, noteIdentity(n))
				if label != "" {
					label += " "
				}
				marker = `<w:r><w:` + kind + `Ref/></w:r><w:r md:noteBodyLabel="1"><w:rPr><w:sz w:val="16"/><w:szCs w:val="16"/></w:rPr><w:t xml:space="preserve">` + escape(label) + `</w:t><w:tab/></w:r>`
				if n.BodyLabelStyle != nil {
					styled, err := w.styledNoteBodyMark(n)
					if err != nil {
						return err
					}
					marker = `<w:r><w:` + kind + `Ref/></w:r>` + styled
				}
				if n.BodyLabelContent != nil {
					rich, err := w.richNoteBodyMark(n)
					if err != nil {
						return err
					}
					marker = `<w:r><w:` + kind + `Ref/></w:r>` + rich
				}
				w.bookmarks++
				marker = fmt.Sprintf(`<w:bookmarkStart w:id="%d" w:name="%s"/><w:bookmarkEnd w:id="%d"/>`, w.bookmarks, noteBodyName(noteKey{kind, strconv.Itoa(w.noteIDs[noteIdentity(n)])}), w.bookmarks) + marker
			}
			leading := ""
			if len(n.Blocks) > 0 && (n.Blocks[0].Kind == Paragraph || n.Blocks[0].Kind == Heading) {
				at := strings.Index(body, `</w:pPr>`)
				if at < 0 {
					w.rels = mainRels
					return fmt.Errorf("missing note paragraph properties")
				}

				body = strings.Replace(body, `<w:ind w:left="`+indentXML+`"/>`, `<w:tabs><w:tab w:val="left" w:pos="`+indentXML+`"/></w:tabs><w:ind w:left="`+indentXML+`" w:hanging="`+indentXML+`"/>`, 1)
				at = strings.Index(body, `</w:pPr>`)
				at += len(`</w:pPr>`)
				body = body[:at] + marker + body[at:]
			} else {
				body = `<w:p><w:pPr><w:tabs><w:tab w:val="left" w:pos="` + indentXML + `"/></w:tabs><w:ind w:left="` + indentXML + `" w:hanging="` + indentXML + `"/></w:pPr>` + marker + `</w:p>` + body
				leading = ` md:leadingMarker="1"`
			}
			leading += w.noteDisplayMetadata(n)
			if n.KindInferred {
				leading += ` md:kindInferred="1"`
			}
			if n.Label != "" {
				leading += ` md:label="` + escape(n.Label) + `"`
			}
			fmt.Fprintf(&out, `<w:%s w:id="%d" md:originalId="%s" md:order="%d"%s>%s</w:%s>`, kind, w.noteIDs[noteIdentity(n)], escape(n.ID), order, leading, body, kind)
		}
		name := kind + "s.xml"
		partRels := w.rels
		w.rels = mainRels
		if count == 0 {
			continue
		}
		fmt.Fprintf(&out, `</w:%ss>`, kind)
		w.parts["word/"+name] = []byte(out.String())
		if len(partRels) > 0 {
			w.parts["word/_rels/"+name+".rels"] = []byte(xmlHeader + `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` + strings.Join(partRels, "") + `</Relationships>`)
		}
		w.addRel(kind+"s", name, false)
	}
	return nil
}
func (r *docxReader) readNoteParts(rels map[string]docxRelation) error {
	identities := map[noteKey]string{}
	orders := map[noteKey]int{}
	displays := map[noteKey]*docxNoteDisplay{}
	total := 0
	customSeparators := map[string]bool{}
	for _, kind := range []string{Footnote, Endnote} {
		var relationship *docxRelation
		for _, v := range rels {
			if !v.External && strings.HasSuffix(v.Type, "/"+kind+"s") {
				if relationship != nil {
					return fmt.Errorf("multiple %s parts", kind)
				}
				value := v
				relationship = &value
			}
		}
		if relationship == nil {
			continue
		}
		root, err := r.xml(relationship.Target)
		if err != nil {
			return err
		}
		if root.Name.Local != kind+"s" {
			return fmt.Errorf("invalid %s part root", kind)
		}
		partRels, err := r.rels(relationship.Target)
		if err != nil {
			return err
		}
		for _, n := range root.Children {
			if err := r.ctx.Err(); err != nil {
				return err
			}
			if n.Name.Local != kind {
				return fmt.Errorf("unsupported %s part child %s", kind, n.Name.Local)
			}
			switch n.attr("type") {
			case "separator", "continuationSeparator", "continuationNotice":
				for _, text := range n.all("t") {
					if strings.TrimSpace(text.content()) != "" {
						customSeparators[kind] = true
					}
				}
				for _, name := range []string{"drawing", "pict", "sym", "imagedata", "object", "oMath"} {
					if len(n.all(name)) > 0 {
						customSeparators[kind] = true
					}
				}
				continue
			case "", "normal":
			default:
				return fmt.Errorf("%w: note type %s", ErrUnsupported, n.attr("type"))
			}
			numeric, err := strconv.Atoi(n.attr("id"))
			if err != nil || numeric < 0 || numeric > 2147483647 {
				return fmt.Errorf("invalid %s ID", kind)
			}
			serialized := noteKey{kind, strconv.Itoa(numeric)}
			if _, ok := identities[serialized]; ok {
				return fmt.Errorf("duplicate %s ID", kind)
			}
			originalID := n.attributeNS(docxNotesNS, "originalId")
			orderMetadata := n.attributeNS(docxNotesNS, "order")
			leadingMetadata := n.attributeNS(docxNotesNS, "leadingMarker")
			if (originalID == "") != (orderMetadata == "") || (leadingMetadata != "" && (leadingMetadata != "1" || originalID == "")) {
				return fmt.Errorf("incomplete or invalid note identity metadata")
			}
			publicID := n.attributeNS(docxNotesNS, "originalId")
			if publicID == "" {
				publicID = serialized.id
			}
			identities[serialized] = publicID
			previousImplicit := r.implicitNoteLayout
			r.implicitNoteLayout = n.attributeNS(docxNotesNS, "originalId") != "" && n.attributeNS(docxNotesNS, "order") != ""
			display, err := readNoteDisplay(n)
			if err != nil {
				return err
			}
			r.expectedBodyLabel = nil
			r.bodyLabelSeen = false
			r.expectedBodyStyle = display != nil && display.Styled
			r.expectedBodyRich = display != nil && display.Rich
			r.expectedBodyWire = serialized
			r.parsedBodyContent = nil
			r.parsedBodyMarkerless = false
			r.parsedBodyStyle = nil
			r.bodySeparatorSeen = false
			if display != nil && display.Variant {
				value := display.Value
				if display.BodyLabel != nil {
					value = *display.BodyLabel
				}
				r.expectedBodyLabel = &value
				if err := validateNoteBodyBookmark(n, serialized); err != nil {
					return err
				}
			}
			r.inNote++
			bs, err := r.blocks(n, partRels, 0)
			r.inNote--
			if display != nil && display.Variant && (!r.bodyLabelSeen || ((display.Styled || display.Rich) && !r.bodySeparatorSeen)) {
				return fmt.Errorf("variant note body label missing")
			}
			r.expectedBodyLabel = nil
			r.implicitNoteLayout = previousImplicit
			if err != nil {
				return err
			}
			if n.attributeNS(docxNotesNS, "leadingMarker") == "1" {
				if len(bs) == 0 || bs[0].Kind != Paragraph || len(bs[0].Runs) != 0 {
					return fmt.Errorf("note leading-marker metadata contradicts content")
				}
				bs = bs[1:]
			}
			key := noteKey{kind, publicID}
			if display != nil {
				displays[key] = display
			}
			if order := n.attributeNS(docxNotesNS, "order"); order != "" {
				value, err := strconv.Atoi(order)
				if err != nil || value < 0 {
					return fmt.Errorf("invalid note order metadata")
				}
				orders[key] = value
			}
			label := r.customNoteLabels[serialized]
			var bodyLabel *string
			if display != nil {
				if display.Variant {
					if !r.variantAnchors[serialized] || !r.variantCitations[serialized] {
						return fmt.Errorf("variant note lacks anchor/citation graph")
					}
				} else if label != display.Value {
					return fmt.Errorf("static note label contradicts display metadata")
				}
				label = display.Label
				bodyLabel = display.BodyLabel
				if display.Rich && r.parsedBodyMarkerless {
					empty := ""
					bodyLabel = &empty
				}
			}

			if metadata := n.attributeNS(docxNotesNS, "label"); metadata != "" && metadata != label {
				return fmt.Errorf("note label metadata contradicts reference")
			}
			inferred := n.attributeNS(docxNotesNS, "kindInferred")
			if inferred != "" && inferred != "1" {
				return fmt.Errorf("invalid inferred note kind metadata")
			}
			r.doc.Notes = append(r.doc.Notes, Note{ID: publicID, Kind: kind, Blocks: bs, Label: label, KindInferred: inferred == "1", BodyLabel: bodyLabel, BodyLabelStyle: r.parsedBodyStyle, BodyLabelContent: r.parsedBodyContent})
			total++
		}
	}
	if len(orders) > 0 {
		if len(orders) != total {
			return fmt.Errorf("partial note order metadata")
		}
		seen := map[int]bool{}
		for _, order := range orders {
			if order >= total || seen[order] {
				return fmt.Errorf("conflicting note order metadata")
			}
			seen[order] = true
		}
		sort.SliceStable(r.doc.Notes, func(i, j int) bool {
			return orders[noteIdentity(r.doc.Notes[i])] < orders[noteIdentity(r.doc.Notes[j])]
		})
	}
	// References are read before their parts. Resolve serialized IDs only once
	// all parts and arbitrary public identifiers are available.
	var resolve func([]Block) error
	resolve = func(bs []Block) error {
		for bi := range bs {
			b := &bs[bi]
			if b.Section != nil {
				if err := resolve(b.Section.Blocks); err != nil {
					return err
				}
				for _, story := range sectionStories(b.Section) {
					if story != nil {
						if err := resolve(story.Blocks); err != nil {
							return err
						}
					}
				}
			}
			for ri := range b.Runs {
				run := &b.Runs[ri]
				if run.Note == nil {
					continue
				}
				key := referenceKey(run.Note)
				id, ok := identities[key]
				if !ok {
					return fmt.Errorf("missing %s note %q", key.kind, key.id)
				}
				copy := *run.Note
				copy.ID = id
				run.Note = &copy
			}
			for ii := range b.Items {
				if b.Items[ii].Marker != nil {
					if err := resolve(b.Items[ii].Marker.Blocks); err != nil {
						return err
					}
				}
				if err := resolve(b.Items[ii].Blocks); err != nil {
					return err
				}
			}
			for rr := range b.Rows {
				for cc := range b.Rows[rr].Cells {
					if err := resolve(b.Rows[rr].Cells[cc].Blocks); err != nil {
						return err
					}
				}
			}
		}
		return nil
	}
	for _, bs := range [][]Block{r.doc.Blocks, r.doc.Headers, r.doc.Footers} {
		if err := resolve(bs); err != nil {
			return err
		}
	}
	for _, note := range r.doc.Notes {
		if customSeparators[note.Kind] {
			r.doc.warn("note_separator", "Custom note separator/continuation content is not represented")
		}
	}
	r.checkEffectiveNoteProperties()

	catalog, err := catalogNotes(r.doc, false)
	if err != nil {
		return err
	}
	for key, display := range displays {
		if catalog.labels[key] != display.Value {
			return fmt.Errorf("note display metadata contradicts canonical label")
		}
	}
	for key, marks := range r.variantDefaultMarks {
		id, ok := identities[key]
		if !ok {
			return fmt.Errorf("variant citation has missing note")
		}
		for _, mark := range marks {
			if mark != catalog.labels[noteKey{key.kind, id}] {
				return fmt.Errorf("default citation label contradicts canonical numbering")
			}
		}
	}
	for key := range r.variantCitations {
		id, ok := identities[key]
		if !ok || !r.variantAnchors[key] || displays[noteKey{key.kind, id}] == nil || !displays[noteKey{key.kind, id}].Variant {
			return fmt.Errorf("variant citation lacks verified anchor/body graph")
		}
	}
	for key := range r.variantAnchors {
		if !r.variantCitations[key] {
			return fmt.Errorf("unreferenced variant anchor")
		}
		if _, ok := identities[key]; !ok {
			return fmt.Errorf("missing variant note definition")
		}
	}
	for id := range r.noteBookmarkIDs {
		if !r.noteBookmarkEnds[id] {
			return fmt.Errorf("unclosed note reference bookmark")
		}
	}
	if len(r.cachedNoteMarks) > 0 {
		catalog, err := catalogNotes(r.doc, false)
		if err != nil {
			return err
		}
		for name, marks := range r.cachedNoteMarks {
			if !r.noteBookmarkNames[name] {
				return fmt.Errorf("note cross-reference has no bookmark target")
			}
			ref, err := parseNoteBookmark(name)
			if err != nil {
				return err
			}
			id, ok := identities[referenceKey(ref)]
			if !ok {
				return fmt.Errorf("note cross-reference has no note")
			}
			label := catalog.labels[noteKey{ref.Kind, id}]
			for _, mark := range marks {
				if mark != label {
					r.doc.warn("note_cached_mark", "Cached note cross-reference mark normalized to automatic numbering")
				}
			}
		}
	}

	return nil
}

func noteStyles() string {
	return `<w:style w:type="character" w:styleId="FootnoteReference"><w:name w:val="footnote reference"/><w:rPr><w:vertAlign w:val="superscript"/></w:rPr></w:style><w:style w:type="character" w:styleId="EndnoteReference"><w:name w:val="endnote reference"/><w:rPr><w:vertAlign w:val="superscript"/></w:rPr></w:style>`
}

func (r *docxReader) noteProperties(section *xmlNode) {
	for _, kind := range []string{Footnote, Endnote} {
		present := false
		for _, note := range r.doc.Notes {
			if note.Kind == kind {
				present = true
				break
			}
		}
		if !present {
			continue
		}
		pr := section.child(kind + "Pr")
		if pr == nil {
			continue
		}
		for _, c := range pr.Children {
			switch c.Name.Local {
			case "numFmt":
				if c.attr("val") != "decimal" {
					r.doc.warn("note_numbering", "Note numbering format "+c.attr("val")+" normalized to decimal")
				}
			case "numStart":
				if c.attr("val") != "1" {
					r.doc.warn("note_numbering", "Note numbering start normalized to one")
				}
			case "numRestart":
				if c.attr("val") != "continuous" {
					r.doc.warn("note_numbering", "Note numbering restart normalized to continuous")
				}
			case "footnote", "endnote": // Separator references do not carry note bodies.
			default:
				r.doc.warn("note_layout", "Note property not represented: "+c.Name.Local)
			}
		}
	}
}

func parseNoteBookmark(name string) (*NoteReference, error) {
	parts := strings.Split(strings.TrimPrefix(name, "mdnote1_"), "_")
	if len(parts) != 2 || (parts[0] != Footnote && parts[0] != Endnote) {
		return nil, fmt.Errorf("invalid note bookmark identity")
	}
	id, err := strconv.Atoi(parts[1])
	if err != nil || id < 1 || id > 2147483647 {
		return nil, fmt.Errorf("invalid note bookmark ID")
	}
	return &NoteReference{Kind: parts[0], ID: strconv.Itoa(id)}, nil
}
func (r *docxReader) noteField(n *xmlNode) (*NoteReference, error) {
	fields := strings.Fields(n.attr("instr"))
	if len(fields) != 4 || fields[0] != "NOTEREF" || fields[2] != `\h` || fields[3] != `\f` {
		return nil, fmt.Errorf("%w: note cross-reference instruction", ErrUnsupported)
	}
	ref, err := parseNoteBookmark(fields[1])
	if err == nil {
		err = readDocxNoteInference(n, ref)
	}
	if err != nil {
		return nil, err
	}
	var text strings.Builder
	for _, node := range n.all("t") {
		text.WriteString(node.content())
	}
	if r.cachedNoteMarks == nil {
		r.cachedNoteMarks = map[string][]string{}
	}
	r.cachedNoteMarks[fields[1]] = append(r.cachedNoteMarks[fields[1]], text.String())
	return ref, nil
}

func (r *docxReader) checkEffectiveNoteProperties() {
	var defaults *xmlNode
	var sections []*xmlNode
	for _, source := range r.notePropertySources {
		if source.Name.Local == "settings" {
			defaults = source
		} else {
			sections = append(sections, source)
		}
	}
	if len(sections) == 0 {
		sections = []*xmlNode{nil}
	}
	explicitEnd := false
	hasEnd := false
	for _, note := range r.doc.Notes {
		if note.Kind == Endnote {
			hasEnd = true
		}
	}
	for _, section := range sections {
		effective := &xmlNode{}
		for _, kind := range []string{Footnote, Endnote} {
			properties := map[string]*xmlNode{}
			for _, source := range []*xmlNode{defaults, section} {
				if pr := source.child(kind + "Pr"); pr != nil {
					for _, value := range pr.Children {
						properties[value.Name.Local] = value
					}
				}
			}
			if len(properties) == 0 {
				continue
			}
			pr := &xmlNode{Name: xml.Name{Local: kind + "Pr"}}
			names := make([]string, 0, len(properties))
			for name := range properties {
				names = append(names, name)
			}
			sort.Strings(names)
			for _, name := range names {
				pr.Children = append(pr.Children, properties[name])
			}
			effective.Children = append(effective.Children, pr)
			if kind == Endnote && properties["numFmt"] != nil {
				explicitEnd = true
			}
		}
		r.noteProperties(effective)
	}
	if hasEnd && !explicitEnd {
		r.doc.warn("note_default_numbering", "Endnote format omitted; OOXML decimal default used (Word may display lowerRoman)")
	}
}

// Both sectioned and ordinary packages must use the same note identity/display
// catalog; per-story relationships and geometry remain owned by each writer.
func (w *docxWriter) initializeNotes(d *Document, notes *noteCatalog) map[string]int {
	w.noteIDs = map[noteKey]int{}
	w.noteLabels = notes.labels
	w.noteCatalog = notes
	w.noteVariants = variantNoteKeys(d)
	w.noteCustomLabels = map[noteKey]string{}
	for key, note := range notes.definitions {
		w.noteCustomLabels[key] = note.Label
		if len(w.noteVariants) > 0 {
			w.noteCustomLabels[key] = notes.labels[key]
		}
	}
	w.seenNoteRefs = map[noteKey]bool{}
	noteCounts := map[string]int{}
	for _, n := range d.Notes {
		noteCounts[n.Kind]++
		w.noteIDs[noteIdentity(n)] = noteCounts[n.Kind]
	}
	return noteCounts
}
