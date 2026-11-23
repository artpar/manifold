package manydoc

import (
	"encoding/xml"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

const docxNotesNS = "urn:manydoc:metadata:1"
const markupCompatibilityNS = "http://schemas.openxmlformats.org/markup-compatibility/2006"

// The canonical model numbers each note kind once across the document. Emit
// this policy explicitly in every section rather than relying on producer
// defaults (notably for endnotes), including sections with an empty body.
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
			blocks := append([]Block(nil), n.Blocks...)
			for i := range blocks {
				if blocks[i].Kind != ListBlock {
					blocks[i].docxIndent = 360
				}
			}
			previousHeight := w.contentHeight
			w.contentHeight -= math.Min(docxImageNoteHeightAllowance, w.contentHeight/2)
			body, err := w.blocksAt(blocks, 1)
			w.contentHeight = previousHeight
			if err != nil {
				w.rels = mainRels
				return err
			}
			marker := `<w:r><w:rPr><w:rStyle w:val="` + strings.Title(kind) + `Reference"/></w:rPr><w:` + kind + `Ref/><w:tab/></w:r>`
			leading := ""
			if len(n.Blocks) > 0 && (n.Blocks[0].Kind == Paragraph || n.Blocks[0].Kind == Heading) {
				at := strings.Index(body, `</w:pPr>`)
				if at < 0 {
					w.rels = mainRels
					return fmt.Errorf("missing note paragraph properties")
				}
				// The first line's number hangs before the note's text edge.
				body = strings.Replace(body, `<w:ind w:left="360"/>`, `<w:tabs><w:tab w:val="left" w:pos="360"/></w:tabs><w:ind w:left="360" w:hanging="360"/>`, 1)
				at = strings.Index(body, `</w:pPr>`)
				at += len(`</w:pPr>`)
				body = body[:at] + marker + body[at:]
			} else {
				body = `<w:p><w:pPr><w:tabs><w:tab w:val="left" w:pos="360"/></w:tabs><w:ind w:left="360" w:hanging="360"/></w:pPr>` + marker + `</w:p>` + body
				leading = ` md:leadingMarker="1"`
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
			r.inNote++
			bs, err := r.blocks(n, partRels, 0)
			r.inNote--
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
			if order := n.attributeNS(docxNotesNS, "order"); order != "" {
				value, err := strconv.Atoi(order)
				if err != nil || value < 0 {
					return fmt.Errorf("invalid note order metadata")
				}
				orders[key] = value
			}
			r.doc.Notes = append(r.doc.Notes, Note{ID: publicID, Kind: kind, Blocks: bs})
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
				run.Note = &NoteReference{ID: id, Kind: key.kind}
			}
			for ii := range b.Items {
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
