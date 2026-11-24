package manydoc

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
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
	for _, prefix := range []string{"mdnote1_", "mdnotebody1_", "mdlist1_", "mditem1_", "mdanchor1_", "mdnoterich1_"} {
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
			internal := original == "" && (strings.HasPrefix(name, "mdlist1_") || strings.HasPrefix(name, "mditem1_") || strings.HasPrefix(name, "mdnote1_") || strings.HasPrefix(name, "mdnotebody1_") || (strings.HasPrefix(name, "mdnoterich1_") && n.attributeNS(docxNotesNS, "noteBodyContent") == "1"))
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
