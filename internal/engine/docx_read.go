package engine

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math"
	"path"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

func docxDestinationName(id string) string {
	valid := utf8.RuneCountInString(id) <= 40 && id != ""
	for i, ch := range id {
		if !(unicode.IsLetter(ch) || ch == '_' || (i > 0 && unicode.IsDigit(ch))) {
			valid = false
		}
	}
	for _, prefix := range []string{"mdnote1_", "mdnotebody1_", "mdlist1_", "mditem1_", "mdmarker1_", "mdanchor1_", "mdnoterich1_"} {
		if strings.HasPrefix(id, prefix) {
			valid = false
		}
	}
	if valid {
		return id
	}
	return fmt.Sprintf("mdanchor1_%x", sha256.Sum256([]byte(id)))[:39]
}

func (w *docxWriter) destinationName(id string) (string, error) {
	name := docxDestinationName(id)
	if w.destinationNames == nil {
		w.destinationNames = map[string]string{}
	}
	if previous, ok := w.destinationNames[name]; ok && previous != id {
		return "", fmt.Errorf("DOCX destination name collision")
	}
	w.destinationNames[name] = id
	return name, nil
}
func (w *docxWriter) anchor(id string) (string, error) {
	name, err := w.destinationName(id)
	if err != nil {
		return "", err
	}
	w.bookmarks++
	return fmt.Sprintf(`<w:bookmarkStart w:id="%d" w:name="%s" md:anchor="%s"/><w:bookmarkEnd w:id="%d"/>`, w.bookmarks, escape(name), escape(id), w.bookmarks), nil
}

type docxBookmarkPoint struct {
	name      string
	rangeLoss bool
	group     string
}

// Parse each story before its paragraphs so forward targets and matching end
// IDs are known. Numeric bookmark IDs are scoped to the package part.
func (r *docxReader) scanDestinations(root *xmlNode) error {
	type start struct {
		node     *xmlNode
		point    docxBookmarkPoint
		position int
	}
	starts := map[string]start{}
	ends := map[string]bool{}
	position := 0
	if r.bookmarkPoints == nil {
		r.bookmarkPoints = map[*xmlNode]docxBookmarkPoint{}
	}
	if r.destinationAliases == nil {
		r.destinationAliases = map[string]string{}
	}
	var walk func(*xmlNode) error
	walk = func(n *xmlNode) error {
		if n.Name.Local == "p" || n.Name.Local == "t" || n.Name.Local == "drawing" {
			position++
		}
		if n.Name.Local == "bookmarkStart" {
			id := n.attr("id")
			v, err := strconv.Atoi(id)
			if err != nil || v < 0 || v > 2147483647 {
				return fmt.Errorf("invalid DOCX bookmark ID")
			}
			if _, ok := starts[id]; ok {
				return fmt.Errorf("duplicate DOCX bookmark ID")
			}
			name := n.attr("name")
			original := n.attributeNS(docxNotesNS, "anchor")
			if original != "" {
				if docxDestinationName(original) != name {
					return fmt.Errorf("bookmark metadata contradicts name")
				}
				name = original
			}
			internal := original == "" && (strings.HasPrefix(name, "mdlist1_") || strings.HasPrefix(name, "mditem1_") || strings.HasPrefix(name, "mdmarker1_") || strings.HasPrefix(name, "mdnote1_") || strings.HasPrefix(name, "mdnotebody1_") || (strings.HasPrefix(name, "mdnoterich1_") && n.attributeNS(docxNotesNS, "noteBodyContent") == "1"))
			point := docxBookmarkPoint{}
			if !internal {
				if err := destinationID(name); err != nil {
					return err
				}
				point.name = name
				if marker := n.attributeNS(docxNotesNS, "standaloneAnchor"); marker != "" {
					if destinationID(marker) != nil || original == "" {
						return fmt.Errorf("invalid standalone anchor metadata")
					}
					point.group = marker
				}
				if alias, ok := r.destinationAliases[n.attr("name")]; ok && alias != name {
					return fmt.Errorf("ambiguous DOCX bookmark name")
				}
				r.destinationAliases[n.attr("name")] = name
				point.rangeLoss = n.attr("colFirst") != "" || n.attr("colLast") != ""
			}
			starts[id] = start{n, point, position}
		} else if n.Name.Local == "bookmarkEnd" {
			id := n.attr("id")
			s, ok := starts[id]
			if !ok || ends[id] {
				return fmt.Errorf("DOCX bookmark end lacks a unique preceding start")
			}
			ends[id] = true
			if s.point.name != "" {
				s.point.rangeLoss = s.point.rangeLoss || position != s.position
				if s.point.group != "" && s.point.rangeLoss {
					return fmt.Errorf("standalone anchor metadata contradicts range")
				}
				r.bookmarkPoints[s.node] = s.point
				r.bookmarkPoints[n] = s.point
			}
		}
		for _, c := range n.Children {
			if err := walk(c); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(root); err != nil {
		return err
	}
	for id := range starts {
		if !ends[id] {
			return fmt.Errorf("DOCX bookmark lacks matching end")
		}
	}
	return nil
}

func (r *docxReader) bookmark(n *xmlNode) string {
	point := r.bookmarkPoints[n]
	if point.rangeLoss {
		r.doc.warn("bookmark_range", "Bookmark navigation points retained; range editing and table-column selection semantics are not represented")
	}
	if n.Name.Local == "bookmarkStart" {
		return point.name
	}
	return ""
}
func packBlockAnchors(bs []Block) []Block {
	var out []Block
	var pending []string
	for _, b := range bs {
		if b.Kind == AnchorPoint && b.docxAnchorGroup == "" {
			pending = append(pending, b.Anchors...)
			continue
		}
		if len(pending) > 0 {
			b.Anchors = append(append([]string(nil), pending...), b.Anchors...)
			pending = nil
		}
		out = append(out, b)
	}
	if len(pending) > 0 {
		out = append(out, Block{Kind: AnchorPoint, Anchors: pending})
	}
	return out
}
func (r *docxReader) finishDestinations() error {
	c, err := catalogDestinations(r.doc, false)
	if err != nil {
		return err
	}
	discovered := false
	for id := range c.links {
		if !c.anchors[id] && !c.missing[id] {
			r.doc.UnresolvedDestinations = append(r.doc.UnresolvedDestinations, id)
			discovered = true
		}
	}
	sort.Strings(r.doc.UnresolvedDestinations)
	if discovered {
		r.doc.warn("internal_destination_missing", "Internal links retain explicitly unresolved source bookmark names")
	}
	return nil
}

func docxUnresolvedDestinations(d *Document) string {
	if len(d.UnresolvedDestinations) == 0 {
		return ""
	}
	ids := append([]string(nil), d.UnresolvedDestinations...)
	sort.Strings(ids)
	raw, _ := json.Marshal(ids)
	return ` md:unresolvedDestinations="` + escape(string(raw)) + `"`
}

const wordNS = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"
const strictWordNS = "http://purl.oclc.org/ooxml/wordprocessingml/main"
const relNS = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"

type xmlNode struct {
	rawStart, rawEnd int64
	Name             xml.Name
	Attr             []xml.Attr
	Text             string
	Children         []*xmlNode
}

func parseXML(ctx context.Context, b []byte) (*xmlNode, error) {
	dec := xml.NewDecoder(bytes.NewReader(b))
	var root *xmlNode
	stack := []*xmlNode{}
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		startOffset := dec.InputOffset()
		t, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch v := t.(type) {
		case xml.StartElement:
			if len(v.Attr) > 1 {
				seen := make(map[xml.Name]bool, len(v.Attr))
				for _, attr := range v.Attr {
					if seen[attr.Name] {
						return nil, fmt.Errorf("duplicate XML attribute %s", attr.Name.Local)
					}
					seen[attr.Name] = true
				}
			}
			if len(stack) > 256 {
				return nil, errors.New("XML nesting exceeds 256 levels")
			}
			n := &xmlNode{Name: v.Name, Attr: v.Attr, rawStart: startOffset}
			if len(stack) == 0 {
				if root != nil {
					return nil, errors.New("multiple XML roots")
				}
				root = n
			} else {
				p := stack[len(stack)-1]
				p.Children = append(p.Children, n)
			}
			stack = append(stack, n)
		case xml.CharData:
			if len(stack) > 0 {
				stack[len(stack)-1].Text += string(v)
			} else if strings.TrimSpace(string(v)) != "" {
				return nil, errors.New("text outside XML root")
			}
		case xml.EndElement:
			if len(stack) == 0 {
				return nil, errors.New("unexpected XML closing element")
			}
			stack[len(stack)-1].rawEnd = dec.InputOffset()
			stack = stack[:len(stack)-1]
		case xml.Directive:
			return nil, errors.New("XML directives are not supported")
		}
	}
	if root == nil || len(stack) > 0 {
		return nil, errors.New("missing or incomplete XML root")
	}
	return root, nil
}
func (n *xmlNode) attr(k string) string {
	if n == nil {
		return ""
	}
	for _, a := range n.Attr {
		if a.Name.Local == k {
			return a.Value
		}
	}
	return ""
}
func (n *xmlNode) child(k string) *xmlNode {
	if n == nil {
		return nil
	}
	for _, c := range n.Children {
		if c.Name.Local == k {
			return c
		}
	}
	return nil
}
func (n *xmlNode) all(k string) []*xmlNode {
	var out []*xmlNode
	if n == nil {
		return out
	}
	if n.Name.Local == k {
		out = append(out, n)
	}
	for _, c := range n.Children {
		out = append(out, c.all(k)...)
	}
	return out
}
func (n *xmlNode) content() string {
	if n == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString(n.Text)
	for _, c := range n.Children {
		b.WriteString(c.content())
	}
	return b.String()
}

type docxRelation struct {
	Target, Type string
	External     bool
}
type docxReader struct {
	bookmarkPoints        map[*xmlNode]docxBookmarkPoint
	destinationAliases    map[string]string
	ctx                   context.Context
	files                 map[string]*zip.File
	doc                   *Document
	styles                map[string]*xmlNode
	defaultPPr            *xmlNode
	defaultRPr            *xmlNode
	theme                 *xmlNode
	themeLanguages        [3]string
	themeLanguagesSet     bool
	fontCharsets          map[string]string
	colorMapping          map[string]string
	defaultParagraphStyle string
	num                   map[string]map[int]docxListLevel
	inTaggedList          int
	inNote                int
	implicitNoteLayout    bool
	notePropertySources   []*xmlNode
	noteBookmarkIDs       map[string]bool
	noteBookmarkEnds      map[string]bool
	noteBookmarkNames     map[string]bool
	cachedNoteMarks       map[string][]string
	customNoteLabels      map[noteKey]string
	variantAnchors        map[noteKey]bool
	variantCitations      map[noteKey]bool
	variantDefaultMarks   map[noteKey][]string
	expectedBodyLabel     *string
	expectedBodyStyle     bool
	expectedBodyRich      bool
	expectedBodyWire      noteKey
	parsedBodyContent     []Run
	parsedBodyMarkerless  bool
	parsedBodyStyle       *Run
	bodySeparatorSeen     bool
	bodyLabelSeen         bool
	rangeNumIDs           map[string]bool
	listSequences         map[*ListNumbering]string
	types                 map[string]string
}

func (r *docxReader) part(name string) ([]byte, error) {
	f := r.files[name]
	if f == nil {
		return nil, fmt.Errorf("missing DOCX part %q", name)
	}
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return readContext(r.ctx, rc)
}
func (r *docxReader) xml(name string) (*xmlNode, error) {
	b, err := r.part(name)
	if err != nil {
		return nil, err
	}
	n, err := parseXML(r.ctx, b)
	if err != nil {
		return nil, fmt.Errorf("DOCX %s: %w", name, err)
	}
	if err := r.scanDestinations(n); err != nil {
		return nil, fmt.Errorf("DOCX %s: %w", name, err)
	}
	return n, nil
}
func (r *docxReader) rels(part string) (map[string]docxRelation, error) {
	name := "_rels/.rels"
	if part != "" {
		name = path.Join(path.Dir(part), "_rels", path.Base(part)+".rels")
	}
	out := map[string]docxRelation{}
	if r.files[name] == nil {
		return out, nil
	}
	n, err := r.xml(name)
	if err != nil {
		return nil, err
	}
	if n.Name.Local != "Relationships" {
		return nil, errors.New("invalid DOCX relationships root")
	}
	for _, c := range n.Children {
		if c.Name.Local != "Relationship" {
			continue
		}
		id, target := c.attr("Id"), c.attr("Target")
		if id == "" || target == "" {
			return nil, errors.New("invalid DOCX relationship")
		}
		if _, ok := out[id]; ok {
			return nil, errors.New("duplicate DOCX relationship ID")
		}
		external := c.attr("TargetMode") == "External"
		if !external {
			if strings.HasPrefix(target, "/") {
				target = strings.TrimPrefix(target, "/")
			} else {
				target = path.Join(path.Dir(part), target)
			}
			if target == ".." || strings.HasPrefix(target, "../") {
				return nil, errors.New("DOCX relationship escapes package")
			}
		}
		out[id] = docxRelation{target, c.attr("Type"), external}
	}
	return out, nil
}

// ReadDOCX reads the final visible view: insertions are included and deletions
// excluded. Revision history and unsupported presentation are reported explicitly.
// No relationships cause network or filesystem access.
func ReadDOCX(ctx context.Context, src io.Reader) (*Document, error) {
	b, err := readContext(ctx, src)
	if err != nil {
		return nil, err
	}
	z, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		return nil, fmt.Errorf("DOCX archive: %w", err)
	}
	r := &docxReader{ctx: ctx, files: map[string]*zip.File{}, doc: &Document{}, styles: map[string]*xmlNode{}, num: map[string]map[int]docxListLevel{}, types: map[string]string{}}
	for _, f := range z.File {
		if f.FileInfo().IsDir() {
			continue
		}
		if strings.Contains(f.Name, "\\") || path.Clean(f.Name) != f.Name || strings.HasPrefix(f.Name, "/") || strings.HasPrefix(f.Name, "../") {
			return nil, errors.New("invalid DOCX archive path")
		}
		if r.files[f.Name] != nil {
			return nil, errors.New("duplicate DOCX archive member")
		}
		r.files[f.Name] = f
	}
	ct, err := r.xml("[Content_Types].xml")
	if err != nil {
		return nil, err
	}
	if ct.Name.Local != "Types" {
		return nil, errors.New("invalid content types root")
	}
	for _, v := range ct.Children {
		if v.Name.Local == "Default" {
			r.types[strings.ToLower(v.attr("Extension"))] = v.attr("ContentType")
		}
	}
	rels, err := r.rels("")
	if err != nil {
		return nil, err
	}
	main := ""
	for _, v := range rels {
		if strings.HasSuffix(v.Type, "/officeDocument") {
			if main != "" {
				return nil, errors.New("multiple DOCX main document relationships")
			}
			if v.External {
				return nil, errors.New("external DOCX main document")
			}
			main = v.Target
		}
		if strings.HasSuffix(v.Type, "/metadata/core-properties") && !v.External {
			n, e := r.xml(v.Target)
			if e != nil {
				return nil, e
			}
			r.doc.Metadata.Title = n.child("title").content()
			r.doc.Metadata.Author = n.child("creator").content()
			r.doc.Metadata.Subject = n.child("subject").content()
		}
	}
	if main == "" {
		return nil, errors.New("DOCX main document relationship missing")
	}
	n, err := r.xml(main)
	if err != nil {
		return nil, err
	}
	if n.Name.Local != "document" || (n.Name.Space != wordNS && n.Name.Space != strictWordNS) || n.child("body") == nil {
		return nil, errors.New("invalid WordprocessingML document root or body")
	}
	if n.child("background") != nil {
		r.doc.warn("page_background", "Page background is not represented; retained text colors may require that background")
	}
	mainRels, err := r.rels(main)
	if err != nil {
		return nil, err
	}
	for _, v := range mainRels {
		if v.External {
			continue
		}
		switch {
		case strings.HasSuffix(v.Type, "/fontTable"):
			table, e := r.xml(v.Target)
			if e != nil {
				return nil, e
			}
			if e := r.readFontCharsets(table); e != nil {
				return nil, e
			}
			for _, name := range []string{"embedRegular", "embedBold", "embedItalic", "embedBoldItalic"} {
				if len(table.all(name)) > 0 {
					r.doc.warn("font_embedding", "Embedded DOCX font programs are not retained; named families require application-supplied output fonts")
				}
			}
		case strings.HasSuffix(v.Type, "/theme"):
			var e error
			r.theme, e = r.xml(v.Target)
			if e != nil {
				return nil, e
			}
		case strings.HasSuffix(v.Type, "/styles"):
			s, e := r.xml(v.Target)
			if e != nil {
				return nil, e
			}
			r.defaultRPr = s.child("docDefaults").child("rPrDefault").child("rPr")
			r.defaultPPr = s.child("docDefaults").child("pPrDefault").child("pPr")
			for _, v := range s.Children {
				if v.Name.Local == "style" {
					if v.attr("type") == "paragraph" && v.attr("default") != "" {
						on, err := docxOnOff(&xmlNode{Attr: []xml.Attr{{Name: xml.Name{Local: "val"}, Value: v.attr("default")}}})
						if err != nil {
							return nil, err
						}
						if on {
							if r.defaultParagraphStyle != "" {
								return nil, fmt.Errorf("multiple default paragraph styles")
							}
							r.defaultParagraphStyle = v.attr("styleId")
						}
					}
					r.styles[v.attr("styleId")] = v
				}
			}
		case strings.HasSuffix(v.Type, "/settings"):
			settings, e := r.xml(v.Target)
			if e != nil {
				return nil, e
			}
			if settings.Name.Local != "settings" {
				return nil, fmt.Errorf("invalid DOCX settings root")
			}
			if e := r.readThemeLanguages(settings); e != nil {
				return nil, e
			}
			if e := r.readColorMapping(settings); e != nil {
				return nil, e
			}
			if err := r.pageSettings(settings, false); err != nil {
				return nil, err
			}
			r.notePropertySources = append(r.notePropertySources, settings)
		case strings.HasSuffix(v.Type, "/numbering"):
			s, e := r.xml(v.Target)
			if e != nil {
				return nil, e
			}
			if e := r.readNumbering(s); e != nil {
				return nil, e
			}
		}
	}
	if raw := n.attributeNS(docxNotesNS, "unresolvedDestinations"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &r.doc.UnresolvedDestinations); err != nil {
			return nil, fmt.Errorf("invalid unresolved destination metadata")
		}
	}
	r.doc.Blocks, err = r.blocks(n.child("body"), mainRels, 0)
	if err != nil {
		return nil, err
	}
	sections := docxActiveSections(n.child("body"))
	if err := r.readDocumentSections(sections, mainRels); err != nil {
		return nil, err
	}
	if err := r.readNoteParts(mainRels); err != nil {
		return nil, err
	}

	if err := r.foldDocumentLists(); err != nil {
		return nil, err
	}
	if err := r.finishDestinations(); err != nil {
		return nil, err
	}
	return r.doc, r.doc.Validate()
}
func number(s string) float64 { v, _ := strconv.ParseFloat(s, 64); return v }
func (r *docxReader) blocks(n *xmlNode, rels map[string]docxRelation, depth int) ([]Block, error) {
	if depth > 64 {
		return nil, errors.New("DOCX block nesting exceeds 64 levels")
	}
	children, err := expandListBookmarks(n.Children, depth)
	if err != nil {
		return nil, err
	}
	var out []Block
	for _, c := range children {
		if err := r.ctx.Err(); err != nil {
			return nil, err
		}
		switch c.Name.Local {
		case "bookmarkStart", "bookmarkEnd":
			if c.attributeNS(docxNotesNS, "noteBodyContent") != "" {
				return nil, fmt.Errorf("rich note label range must be inline")
			}
			if name := r.bookmark(c); name != "" {
				group := r.bookmarkPoints[c].group
				if group != "" && len(out) > 0 && out[len(out)-1].docxAnchorGroup == group {
					out[len(out)-1].Anchors = append(out[len(out)-1].Anchors, name)
				} else {
					if group != "" && group != name {
						return nil, fmt.Errorf("standalone anchor group lacks its first point")
					}
					out = append(out, Block{Kind: AnchorPoint, Anchors: []string{name}, docxAnchorGroup: group})
				}
			}
		case "p":
			if c.attributeNS(docxNotesNS, "literalPageStart") != "" {
				return nil, fmt.Errorf("literal page-start requires checked item ownership")
			}
			structural, err := docxStructuralParagraph(n, c)
			if err != nil {
				return nil, err
			}
			if structural {
				continue
			}
			role, _, err := docxBreakRole(c)
			if err != nil {
				return nil, err
			}
			if role == "anchor" {
				continue
			}
			if role == "control" {
				out = append(out, Block{Kind: PageBreak})
				continue
			}
			if role == "before" {
				out = append(out, Block{Kind: PageBreak})
			}
			bs, err := r.paragraph(c, rels)
			if err != nil {
				return nil, err
			}
			if section := c.child("pPr").child("sectPr"); section != nil {
				if len(bs) == 0 {
					return nil, fmt.Errorf("section paragraph produced no boundary")
				}
				bs[len(bs)-1].docxSection = section
			}
			if marker := c.attributeNS(docxNotesNS, "sectionTail"); marker != "" {
				if marker != "1" || c.child("pPr").child("sectPr") == nil || len(c.Children) != 1 || len(c.child("pPr").Children) != 1 || len(bs) != 1 || len(bs[0].Runs) != 0 {
					return nil, fmt.Errorf("invalid structural section tail")
				}
				bs[0].docxSectionTail = true
			}

			if section := c.child("pPr").child("sectPr"); section != nil {
				if len(bs) == 0 {
					return nil, fmt.Errorf("section paragraph has no body boundary")
				}
				bs[len(bs)-1].docxSection = section
				if marker := c.attributeNS(docxNotesNS, "sectionTail"); marker != "" {
					if marker != "1" || len(c.Children) != 1 || c.Children[0].Name.Local != "pPr" || len(c.Children[0].Children) != 1 || len(bs) != 1 || len(bs[0].Runs) != 0 {
						return nil, fmt.Errorf("invalid structural section tail")
					}
					bs[0].docxSectionTail = true
				}
			} else if c.attributeNS(docxNotesNS, "sectionTail") != "" {
				return nil, fmt.Errorf("section tail has no section properties")
			}
			out = append(out, bs...)
		case "tbl":
			if c.attributeNS(docxNotesNS, "literalMarker") != "" {
				return nil, fmt.Errorf("literal marker table requires checked list/item ownership")
			}
			b, err := r.table(c, rels, depth)
			if err != nil {
				return nil, err
			}
			out = append(out, b)
		case "sdt":
			if c.Name.Space == "" && strings.HasPrefix(c.child("sdtPr").child("tag").attr("val"), docxMarkerTag) {
				return nil, fmt.Errorf("literal marker bookmark requires checked item ownership")
			}
			if c.Name.Space == "" && strings.HasPrefix(c.child("sdtPr").child("tag").attr("val"), docxListTag) {
				b, err := r.taggedList(c, rels, depth)
				if err != nil {
					return nil, err
				}
				out = append(out, b)
				continue
			}
			r.doc.warn("content_control", "Content control content is retained; control properties are not represented")
			if content := c.child("sdtContent"); content != nil {
				bs, err := r.blocks(content, rels, depth+1)
				if err != nil {
					return nil, err
				}
				out = append(out, bs...)
			}
		case "ins", "moveTo":
			r.doc.warn("revision_history", "Final visible revision content is retained; revision history is not represented")
			bs, err := r.blocks(c, rels, depth+1)
			if err != nil {
				return nil, err
			}
			out = append(out, bs...)
		case "del", "moveFrom":
			r.doc.warn("revision_history", "Final visible revision content is retained; revision history is not represented")
		case "sectPr", "tcPr":
		default:
			r.doc.warn("unsupported_block", "Unsupported Word block: "+c.Name.Local)
		}
	}
	return packBlockAnchors(out), nil
}
func (r *docxReader) paragraph(n *xmlNode, rels map[string]docxRelation) ([]Block, error) {
	children, richRanges, err := r.collectRichBodyRanges(n)
	if err != nil {
		return nil, err
	}
	b := Block{Kind: Paragraph}
	ppr := n.child("pPr")
	noteIndent := r.implicitNoteIndent(n)
	if ppr != nil {
		for _, p := range ppr.Children {
			switch p.Name.Local {
			case "pStyle", "numPr", "outlineLvl", "sectPr", "keepNext", "pageBreakBefore", "bidi":
			case "tabs":
				if !(r.implicitNoteLayout && len(p.Children) == 1 && p.Children[0].Name.Local == "tab" && p.Children[0].attr("val") == "left" && p.Children[0].attr("pos") == noteIndent) {
					r.doc.warn("paragraph_formatting", "Paragraph property not represented: tabs")
				}
			case "ind":
				if r.inTaggedList == 0 && !(r.implicitNoteLayout && p.attr("left") == noteIndent) {
					r.doc.warn("paragraph_formatting", "Paragraph property not represented: ind")
				}
			default:
				r.doc.warn("paragraph_formatting", "Paragraph property not represented: "+p.Name.Local)
			}
		}
	}
	styleID := ppr.child("pStyle").attr("val")
	before, err := r.paragraphPageBreak(ppr, styleID)
	if err != nil {
		return nil, err
	}
	role, conditional, err := docxBreakRole(n)
	if err != nil {
		return nil, err
	}
	if role == "before" || role == "beforeList" {
		before = conditional
	}
	b.Direction, err = r.paragraphDirection(ppr, styleID)
	if err != nil {
		return nil, err
	}
	b.PageBreakBefore = before
	b.docxBeforeList = role == "beforeList"
	if strings.HasPrefix(strings.ToLower(styleID), "heading") {
		level, _ := strconv.Atoi(strings.TrimPrefix(strings.ToLower(styleID), "heading"))
		if level >= 1 && level <= 6 {
			b.Kind = Heading
			b.Level = level
		}
	}
	if level := ppr.child("outlineLvl"); level != nil {
		v := int(number(level.attr("val"))) + 1
		if v >= 1 && v <= 6 {
			b.Kind = Heading
			b.Level = v
		}
	}
	if err := r.paragraphList(&b, ppr, styleID); err != nil {
		return nil, err
	}
	originalList := b.docxList
	out := []Block{}
	base, err := r.paragraphTextStyle(styleID)
	if err != nil {
		return nil, err
	}
	flush := func() { out = append(out, b); b.Runs = nil; b.PageBreakBefore = false }
	skipNoteSeparator := false
	activeNoteBookmark := ""
	var activeNoteReference *NoteReference
	seenNoteReference := false
	var pendingCustom *NoteReference
	var customMark strings.Builder
	var walk func(*xmlNode, docxRunStyle) error
	walk = func(c *xmlNode, style docxRunStyle) error {
		if rich, ok := richRanges[c]; ok {
			return r.readRichBodyRuns(rich, rels)
		}
		if c.attributeNS(docxNotesNS, "noteBodyContent") != "" {
			return fmt.Errorf("unbound rich note label role")
		}
		if pendingCustom != nil && c.Name.Local != "t" {
			return fmt.Errorf("%w: nontext content inside custom note mark", ErrUnsupported)
		}
		switch c.Name.Local {
		case "pPr", "rPr":
			return nil
		case "del", "moveFrom":
			r.doc.warn("revision_history", "Final visible revision content is retained; revision history is not represented")
			return nil
		case "ins", "moveTo":
			r.doc.warn("revision_history", "Final visible revision content is retained; revision history is not represented")
		case "r":
			if c.attributeNS(docxNotesNS, "noteBodyStyle") != "" {
				if err := r.readStyledBodyMark(c, rels); err != nil {
					return err
				}
				return nil
			}
			if c.attributeNS(docxNotesNS, "noteBodySeparator") != "" {
				if err := r.readBodyLabelSeparator(c); err != nil {
					return err
				}
				skipNoteSeparator = false
				return nil
			}
			if c.attributeNS(docxNotesNS, "noteBodyLabel") != "" {
				if err := r.variantBodyMark(c); err != nil {
					return err
				}
				skipNoteSeparator = false
				return nil
			}
			if c.attributeNS(docxNotesNS, "noteAnchor") != "" {
				ref, err := r.hiddenNoteAnchor(c)
				if err != nil {
					return err
				}
				if activeNoteReference == nil || referenceKey(activeNoteReference) != referenceKey(ref) || activeNoteReference.Inferred != ref.Inferred || seenNoteReference {
					return fmt.Errorf("variant note anchor lacks matching reference bookmark")
				}
				seenNoteReference = true
				return nil
			}
			if props := c.child("rPr"); props != nil {
				for _, p := range props.Children {
					switch p.Name.Local {
					case "b", "i", "u", "strike", "rFonts", "sz", "szCs", "bCs", "iCs", "cs", "lang", "color", "rtl":
					case "rStyle":
						if r.styles[p.attr("val")] == nil && p.attr("val") != "FootnoteReference" && p.attr("val") != "EndnoteReference" {
							r.doc.warn("run_formatting", "Run property not represented: rStyle")
						}
					default:
						r.doc.warn("run_formatting", "Run property not represented: "+p.Name.Local)
					}
				}
			}
			if err := r.textStyleChain(&style, c.child("rPr").child("rStyle").attr("val")); err != nil {
				return err
			}
			if err := r.textProperties(&style, c.child("rPr"), false); err != nil {
				return err
			}
			if err := r.restoreTextImplicit(&style, c, b.Level); err != nil {
				return err
			}
			applyRunProps(&style.Run, c.child("rPr"))
			if value := c.attributeNS(docxNotesNS, "headingBold"); value != "" {
				if b.Kind != Heading || (value != "0" && value != "1") {
					return fmt.Errorf("invalid heading bold metadata")
				}
				explicit, err := docxOnOff(c.child("rPr").child("b"))
				if err != nil || explicit != (value == "1") {
					return fmt.Errorf("heading bold metadata contradicts run properties")
				}
				style.Bold = value == "1"
			}
		case "hyperlink":
			if c.attributeNS(docxNotesNS, "noteBodyStyle") != "" {
				if err := r.readStyledBodyMark(c, rels); err != nil {
					return err
				}
				return nil
			}
			if c.attributeNS(docxNotesNS, "noteCitation") != "" {
				c, err := checkedCitationIsolation(c)
				if err != nil {
					return err
				}
				ref, err := r.variantCitation(c)
				if err != nil {
					return err
				}
				if ref.LabelContent != nil {
					b.Runs = append(b.Runs, Run{Note: ref})
					return nil
				}
				run := c.child("r")
				if err := r.textStyleChain(&style, run.child("rPr").child("rStyle").attr("val")); err != nil {
					return err
				}
				if err := r.textProperties(&style, run.child("rPr"), false); err != nil {
					return err
				}
				if err := r.restoreTextImplicit(&style, run, b.Level); err != nil {
					return err
				}
				applyRunProps(&style.Run, run.child("rPr"))
				if value := run.attributeNS(docxNotesNS, "headingBold"); value != "" {
					explicit, err := docxOnOff(run.child("rPr").child("b"))
					if b.Kind != Heading || (value != "0" && value != "1") || err != nil || explicit != (value == "1") {
						return fmt.Errorf("citation heading bold metadata contradicts content")
					}
					style.Bold = value == "1"
				}
				style.Text = ""
				style.Note = ref
				parts, err := r.textRuns(style)
				if err != nil {
					return err
				}
				b.Runs = append(b.Runs, parts...)
				return nil
			}
			if id := c.attr("id"); id != "" {
				rel, ok := rels[id]
				if !ok {
					return errors.New("missing hyperlink relationship")
				}
				style.Link = rel.Target
				if anchor := c.attr("anchor"); anchor != "" {
					// Word appends the anchor even when r:id is present (MS-OI29500
					// 2.1.521). Keep its actual URI; strict OOXML readers may ignore
					// this second attribute, so disclose the compatibility choice.

					// Word stores a section's properties at its end; type controls its start.

					// A final section property belongs directly to body, including an empty body.

					style.Link += "#" + anchor
					r.doc.warn("hyperlink_word_anchor", "Hyperlink relationship plus anchor uses Word fragment composition; strict OOXML readers may ignore the anchor")
				}
			} else if a := c.attr("anchor"); a != "" {
				target := a
				if alias, ok := r.destinationAliases[a]; ok {
					target = alias
				}
				if original := c.attributeNS(docxNotesNS, "destination"); original != "" {
					if docxDestinationName(original) != a {
						return fmt.Errorf("internal hyperlink metadata contradicts anchor")
					}
					if alias, ok := r.destinationAliases[a]; ok && alias != original {
						return fmt.Errorf("internal hyperlink metadata contradicts bookmark")
					}
					target = original
				}
				style.Destination = target
				style.Link = "#" + target
				if href := c.attributeNS(docxNotesNS, "href"); href != "" {
					style.Link = href
				}
			}
		case "t":
			if pendingCustom != nil {
				customMark.WriteString(c.content())
				return nil
			}
			skipNoteSeparator = false
			style.Text = c.content()
			parts, err := r.textRuns(style)
			if err != nil {
				return err
			}
			b.Runs = append(b.Runs, parts...)
			return nil
		case "tab", "ptab":
			if c.Name.Local == "ptab" {
				if len(c.Children) != 0 || strings.TrimSpace(c.Text) != "" {
					return fmt.Errorf("positional tab must not contain content")
				}
				skipNoteSeparator = false
				r.doc.warn("position_tab", "Positional tab separation is retained as a tab; alignment, positioning base and leader presentation are not represented")
			}
			if skipNoteSeparator && c.Name.Local == "tab" {
				skipNoteSeparator = false
				return nil
			}
			style.Text = "\t"
			parts, err := r.textRuns(style)
			if err != nil {
				return err
			}
			b.Runs = append(b.Runs, parts...)
			return nil
		case "br", "cr":
			if c.attr("type") == "page" {
				if len(b.Runs) > 0 || b.PageBreakBefore {
					flush()
				}
				out = append(out, Block{Kind: PageBreak})
			} else {
				style.Text = "\n"
				parts, err := r.textRuns(style)
				if err != nil {
					return err
				}
				b.Runs = append(b.Runs, parts...)
			}
			return nil
		case "oMath", "oMathPara":
			if c.Name.Space != officeMathNS {
				return fmt.Errorf("unexpected math namespace")
			}
			native, err := serializeOfficeMath(c)
			if err != nil {
				return err
			}
			if len(b.Runs) > 0 || b.PageBreakBefore {
				flush()
			}
			out = append(out, Block{Kind: OfficeMathBlock, Math: &OfficeMath{OMML: native}})
			r.doc.warn("office_math_native", "Native Office Math markup retained; inline equations use independent flow blocks and plain text exposes tokens only")
			return nil
		case "object", "pict":
			images, err := r.legacyPictures(c, rels)
			if err != nil {
				return err
			}
			if len(images) > 0 {
				if len(b.Runs) > 0 || b.PageBreakBefore {
					flush()
				}
				out = append(out, images...)
				return nil
			}
			if len(c.all("txbxContent")) == 0 {
				return fmt.Errorf("%w: legacy drawing has no supported preview or text", ErrUnsupported)
			}
		case "drawing":
			blips := c.all("blip")
			for _, blip := range blips {
				id := blip.attr("embed")
				if id == "" {
					r.doc.warn("external_image", "External images are not fetched")
					continue
				}
				rel, ok := rels[id]
				if !ok || rel.External {
					return errors.New("missing embedded image relationship")
				}
				data, err := r.part(rel.Target)
				if err != nil {
					return err
				}
				media := r.types[strings.TrimPrefix(strings.ToLower(path.Ext(rel.Target)), ".")]
				im := &Image{Data: data, MediaType: media}
				if ext := c.all("extent"); len(ext) > 0 {
					im.Width = number(ext[0].attr("cx")) / 12700
					im.Height = number(ext[0].attr("cy")) / 12700
				}
				if props := c.all("docPr"); len(props) > 0 {
					im.Alt = props[0].attr("descr")
				}
				if len(b.Runs) > 0 || b.PageBreakBefore {
					flush()
				}
				out = append(out, Block{Kind: Picture, Image: im})
			}
			if len(c.all("anchor")) > 0 {
				r.doc.warn("floating_image", "Floating image is represented inline")
			}
			return nil
		case "txbxContent":
			r.doc.warn("text_box_layout", "Text box text is retained without floating layout")
			bs, err := r.blocks(c, rels, 0)
			if err != nil {
				return err
			}
			if len(b.Runs) > 0 || b.PageBreakBefore {
				flush()
			}
			out = append(out, bs...)
			return nil
		case "fldSimple":
			if strings.HasPrefix(strings.TrimSpace(c.attr("instr")), "NOTEREF mdnote1_") {
				ref, err := r.noteField(c)
				if err != nil {
					return err
				}
				if err := r.textStyleChain(&style, c.child("r").child("rPr").child("rStyle").attr("val")); err != nil {
					return err
				}
				if err := r.textProperties(&style, c.child("r").child("rPr"), false); err != nil {
					return err
				}
				if err := r.restoreTextImplicit(&style, c.child("r"), b.Level); err != nil {
					return err
				}
				applyRunProps(&style.Run, c.child("r").child("rPr"))
				style.Text = ""
				style.Note = ref
				parts, err := r.textRuns(style)
				if err != nil {
					return err
				}
				b.Runs = append(b.Runs, parts...)
				return nil
			}
			r.doc.warn("field", "Cached field text is retained; field evaluation is not supported")
		case "instrText", "fldChar":
			r.doc.warn("field", "Cached field text is retained; field evaluation is not supported")
			return nil
		case "sdt":
			r.doc.warn("content_control", "Content control content is retained; control properties are not represented")
		case "bookmarkStart", "bookmarkEnd":
			if c.Name.Local == "bookmarkStart" && strings.HasPrefix(c.attr("name"), "mdnote1_") {
				ref, err := parseNoteBookmark(c.attr("name"))
				if err != nil {
					return err
				}
				bookmarkID, err := strconv.Atoi(c.attr("id"))
				if err != nil || bookmarkID < 0 || bookmarkID > 2147483647 {
					return fmt.Errorf("invalid note bookmark range ID")
				}
				if activeNoteBookmark != "" {
					return fmt.Errorf("nested note reference bookmarks")
				}
				activeNoteBookmark = c.attr("id")
				activeNoteReference = ref
				seenNoteReference = false
				if r.noteBookmarkIDs == nil {
					r.noteBookmarkIDs = map[string]bool{}
					r.noteBookmarkEnds = map[string]bool{}
					r.noteBookmarkNames = map[string]bool{}
				}
				if r.noteBookmarkIDs[c.attr("id")] || r.noteBookmarkNames[c.attr("name")] {
					return fmt.Errorf("duplicate note bookmark")
				}
				r.noteBookmarkIDs[c.attr("id")] = true
				r.noteBookmarkNames[c.attr("name")] = true
				return nil
			}
			if c.Name.Local == "bookmarkEnd" && r.noteBookmarkIDs[c.attr("id")] {
				if activeNoteBookmark != c.attr("id") || !seenNoteReference {
					return fmt.Errorf("note bookmark does not enclose its matching reference")
				}
				activeNoteBookmark = ""
				activeNoteReference = nil
				if r.noteBookmarkEnds[c.attr("id")] {
					return fmt.Errorf("duplicate note bookmark end")
				}
				r.noteBookmarkEnds[c.attr("id")] = true
				return nil
			}
			if name := r.bookmark(c); name != "" {
				b.Runs = append(b.Runs, Run{Anchor: name})
			}
			return nil
		case "footnoteReference", "endnoteReference":
			id, err := strconv.Atoi(c.attr("id"))
			if err != nil || id < 0 || id > 2147483647 {
				return fmt.Errorf("invalid note reference ID")
			}
			style.Text = ""
			style.Note = &NoteReference{ID: strconv.Itoa(id), Kind: strings.TrimSuffix(c.Name.Local, "Reference")}
			if err := readDocxNoteInference(c, style.Note); err != nil {
				return err
			}
			if activeNoteReference != nil {
				if referenceKey(activeNoteReference) != referenceKey(style.Note) || seenNoteReference {
					return fmt.Errorf("note bookmark identity contradicts enclosed reference")
				}
				seenNoteReference = true
			}
			switch c.attr("customMarkFollows") {
			case "", "0", "false", "off":
			case "1", "true", "on":
				if pendingCustom != nil {
					return fmt.Errorf("missing custom note mark")
				}
				pendingCustom = style.Note
			default:
				return fmt.Errorf("invalid customMarkFollows boolean")
			}
			parts, err := r.textRuns(style)
			if err != nil {
				return err
			}
			b.Runs = append(b.Runs, parts...)
			return nil
		case "footnoteRef", "endnoteRef":
			skipNoteSeparator = r.inNote > 0
			return nil
		case "commentReference", "commentRangeStart", "commentRangeEnd":
			r.doc.warn("comments", "Comment annotations are not represented")
			return nil
		case "sym":
			text, err := docxSymbolUnicode(c.attr("font"), c.attr("char"))
			if err != nil {
				return err
			}
			style.Text = text
			for i := range style.fonts {
				style.fonts[i] = docxFontChoice{family: c.attr("font"), set: true}
			}
			parts, err := r.textRuns(style)
			if err != nil {
				return err
			}
			b.Runs = append(b.Runs, parts...)
			r.doc.warn("symbol_font_unicode", "Legacy Symbol/ZapfDingbats glyph codes are normalized to explicit Unicode text with the declared font family")
			return nil
		case "noBreakHyphen":
			style.Text = "\u2011"
			parts, err := r.textRuns(style)
			if err != nil {
				return err
			}
			b.Runs = append(b.Runs, parts...)
			return nil
		case "softHyphen":
			style.Text = "\u00ad"
			parts, err := r.textRuns(style)
			if err != nil {
				return err
			}
			b.Runs = append(b.Runs, parts...)
			return nil
		}
		for _, cc := range c.Children {
			if err := walk(cc, style); err != nil {
				return err
			}
		}
		if c.Name.Local == "r" && pendingCustom != nil {
			label := customMark.String()
			if label == "" {
				return fmt.Errorf("missing custom note mark in reference run")
			}
			if err := validateNoteLabel(label); err != nil {
				return err
			}
			if r.customNoteLabels == nil {
				r.customNoteLabels = map[noteKey]string{}
			}
			key := referenceKey(pendingCustom)
			if old, ok := r.customNoteLabels[key]; ok && old != label {
				return fmt.Errorf("conflicting custom note labels")
			}
			r.customNoteLabels[key] = label
			pendingCustom = nil
			customMark.Reset()
		}

		return nil
	}
	for _, c := range children {
		if err := walk(c, base); err != nil {
			return nil, err
		}
	}
	if activeNoteBookmark != "" {
		return nil, fmt.Errorf("unclosed note bookmark range in paragraph")
	}
	if len(b.Runs) > 0 || len(out) == 0 {
		flush()
	}
	if originalList != nil && (len(out) > 1 || (len(out) > 0 && out[0].Kind != Paragraph && out[0].Kind != Heading)) {
		for i := range out {
			out[i].List = ""
			out[i].docxList = nil
		}
		if out[0].Kind != Paragraph && out[0].Kind != Heading {
			out = append([]Block{{Kind: Paragraph}}, out...)
		}
		r.doc.warn("list_inline_layout", "Numbered paragraph inline content retained as item blocks; inline layout normalized")
		return []Block{{Kind: ListBlock, List: originalList.Definition.Kind, Items: []ListItem{{Blocks: out}}, docxList: originalList}}, nil
	}
	return out, nil
}
func applyRunProps(r *Run, n *xmlNode) {
	if n == nil {
		return
	}
	for _, p := range n.Children {
		v := p.attr("val")
		enabled := v != "0" && v != "false" && v != "off" && v != "none"
		switch p.Name.Local {
		case "b":
			r.Bold = enabled
		case "i":
			r.Italic = enabled
		case "u":
			r.Underline = enabled
		case "strike":
			r.Strike = enabled
		}
	}
}

func (r *docxReader) readDocumentSections(properties []*xmlNode, rels map[string]docxRelation) error {
	sectioned := len(properties) > 1 || r.doc.EvenAndOddHeaders
	for _, p := range properties {
		if p.attributeNS(docxNotesNS, "sectionModel") != "" || p.child("titlePg") != nil {
			sectioned = true
		}
		for _, c := range p.Children {
			if (c.Name.Local == "headerReference" || c.Name.Local == "footerReference") && c.attr("type") != "" && c.attr("type") != "default" {
				sectioned = true
			}
		}
	}
	var bodies [][]Block
	var pending []Block
	var boundaryProps []*xmlNode
	for _, b := range r.doc.Blocks {
		section := b.docxSection
		b.docxSection = nil
		if !b.docxSectionTail {
			pending = append(pending, b)
		}
		if section != nil {
			bodies = append(bodies, pending)
			pending = nil
			boundaryProps = append(boundaryProps, section)
		}
	}

	if len(properties) > len(boundaryProps) {
		bodies = append(bodies, pending)
	} else if len(pending) > 0 || len(properties) == 0 {
		bodies = append(bodies, pending)
	}
	if len(properties) > 0 && len(bodies) != len(properties) {
		return fmt.Errorf("section properties outside root body boundaries")
	}
	if len(properties) == 0 {
		return nil
	}
	storyCache := map[string]*RunningStory{}
	var sections []Block
	previous := cloneLayout(r.doc.Layout)
	base := cloneLayout(previous)
	for i, p := range properties {
		if err := r.pageSettings(p, true); err != nil {
			return err
		}
		r.notePropertySources = append(r.notePropertySources, p)
		if i == 0 && p.attributeNS(docxNotesNS, "sectionModel") == "1" {
			raw := p.attributeNS(docxNotesNS, "sectionBase")
			var err error
			base, err = decodeSectionLayout(raw)
			if err != nil {
				return fmt.Errorf("section base: %w", err)
			}
			previous = cloneLayout(base)
		}
		r.doc.Layout = cloneLayout(previous)
		if size := p.child("pgSz"); size != nil {
			var err error
			r.doc.Layout.Width, err = docxPageNumber(size, "w", previous.Width)
			if err != nil {
				return err
			}
			r.doc.Layout.Height, err = docxPageNumber(size, "h", previous.Height)
			if err != nil {
				return err
			}
		}
		if margins := p.child("pgMar"); margins != nil {
			if err := r.pageMargins(margins); err != nil {
				return err
			}
		}
		actual := cloneLayout(r.doc.Layout)
		s := &Section{Blocks: bodies[i], Start: SectionNextPage}
		s.Layout = &actual
		if t := p.child("type"); t != nil {
			s.Start = SectionStart(t.attr("val"))
			if s.Start == "" {
				return fmt.Errorf("missing section start value")
			}
		}
		if t := p.child("titlePg"); t != nil {
			var err error
			s.FirstPage, err = docxOnOff(t)
			if err != nil {
				return err
			}
		}
		model := p.attributeNS(docxNotesNS, "sectionModel")
		if model != "" {
			if model != "1" {
				return fmt.Errorf("invalid section model marker")
			}
			raw := p.attributeNS(docxNotesNS, "sectionLayout")
			expected := previous
			if raw == "inherit" {
				s.Layout = nil
			} else {
				l, err := decodeSectionLayout(raw)
				if err != nil {
					return fmt.Errorf("section layout: %w", err)
				}
				s.Layout = &l
				expected = l
			}
			if !sameDOCXSectionGeometry(expected, actual) {
				return fmt.Errorf("section metadata contradicts physical page settings")
			}
			if flag := p.attributeNS(docxNotesNS, "sectionStartImplicit"); flag != "" {
				if flag != "1" || s.Start != SectionNextPage {
					return fmt.Errorf("invalid implicit section start")
				}
				s.Start = ""
			}
		}
		for _, c := range p.Children {
			if c.Name.Local != "headerReference" && c.Name.Local != "footerReference" {
				continue
			}
			kind, root := "header", "hdr"
			target := &s.Headers
			if c.Name.Local == "footerReference" {
				kind, root, target = "footer", "ftr", &s.Footers
			}
			variant := c.attr("type")
			if variant == "" {
				variant = "default"
			}
			var destination **RunningStory
			switch variant {
			case "default":
				destination = &target.Default
			case "first":
				destination = &target.First
			case "even":
				destination = &target.Even
			default:
				return fmt.Errorf("invalid running story variant %q", variant)
			}
			if *destination != nil {
				return fmt.Errorf("duplicate section %s %s", kind, variant)
			}
			rel, ok := rels[c.attr("id")]
			if !ok || rel.External || !strings.HasSuffix(rel.Type, "/"+kind) {
				return fmt.Errorf("missing or invalid %s relationship", kind)
			}
			if cached := storyCache[rel.Target]; cached != nil {
				*destination = cached
				continue
			}
			part, err := r.xml(rel.Target)
			if err != nil {
				return err
			}
			if part.Name.Local != root {
				return fmt.Errorf("invalid %s root", kind)
			}
			pr, err := r.rels(rel.Target)
			if err != nil {
				return err
			}
			blocks, err := r.blocks(part, pr, 0)
			if err != nil {
				return err
			}
			if len(part.Children) == 1 && part.Children[0].attributeNS(docxNotesNS, "emptyStory") != "" {
				n := part.Children[0]
				if n.Name.Local != "p" || n.attributeNS(docxNotesNS, "emptyStory") != "1" || len(n.Children) != 0 || strings.TrimSpace(n.Text) != "" {
					return fmt.Errorf("invalid empty running story marker")
				}
				blocks = nil
			}
			for _, b := range blocks {
				if b.docxSection != nil {
					return fmt.Errorf("section boundary inside running story")
				}
			}
			*destination = &RunningStory{Blocks: blocks}
			storyCache[rel.Target] = *destination
		}
		var anchors []string
		if raw := p.attributeNS(docxNotesNS, "sectionAnchors"); raw != "" {
			if model != "1" {
				return fmt.Errorf("section anchor metadata requires exported section model")
			}
			if err := json.Unmarshal([]byte(raw), &anchors); err != nil || len(anchors) == 0 {
				return fmt.Errorf("invalid section anchor metadata")
			}
			if len(s.Blocks) == 0 || len(s.Blocks[0].Anchors) < len(anchors) {
				return fmt.Errorf("section anchor metadata lacks standard bookmarks")
			}
			for j, id := range anchors {
				if s.Blocks[0].Anchors[j] != id {
					return fmt.Errorf("section anchor metadata contradicts standard bookmarks")
				}
			}
			s.Blocks[0].Anchors = s.Blocks[0].Anchors[len(anchors):]
			if s.Blocks[0].Kind == AnchorPoint && len(s.Blocks[0].Anchors) == 0 {
				s.Blocks = s.Blocks[1:]
			}
		}
		sections = append(sections, Block{Kind: SectionBlock, Section: s, Anchors: anchors})
		if s.Layout != nil {
			previous = cloneLayout(*s.Layout)
		}
	}
	if sectioned {
		r.doc.Blocks = sections
		r.doc.Layout = base
		if properties[0].attributeNS(docxNotesNS, "sectionModel") == "" {
			r.doc.Layout = cloneLayout(*sections[0].Section.Layout)
		}
	} else {
		s := sections[0].Section
		r.doc.Blocks = s.Blocks
		r.doc.Layout = cloneLayout(*s.Layout)
		if s.Headers.Default != nil {
			r.doc.Headers = s.Headers.Default.Blocks
		}
		if s.Footers.Default != nil {
			r.doc.Footers = s.Footers.Default.Blocks
		}
	}
	return nil
}
func sameDOCXSectionGeometry(a, b Layout) bool {
	x, y := pageLayout(a), pageLayout(b)
	values := func(g pageGeometry) []float64 {
		return []float64{g.width, g.height, g.margins.Left, g.margins.Right, g.margins.Top, g.margins.Bottom, g.headerDistance(), g.footerDistance()}
	}
	av, bv := values(x), values(y)
	for i, v := range av {
		if math.RoundToEven(v*20) != math.RoundToEven(bv[i]*20) {
			return false
		}
	}
	return true
}
