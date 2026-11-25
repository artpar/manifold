package manifold

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
)

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
	// Exactly one ordinary text run: no cached fields, drawings, tabs or hidden content.
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
