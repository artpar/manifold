package manydoc

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"strconv"
	"strings"
)

const docxListTag = "manydoc:list:v1:"
const docxItemTag = "manydoc:list-item:v1"

type docxListLevel struct {
	Kind                         string
	Legal, Implicit, Independent bool
	Presentation                 bool
	EmptyOverride                bool
	OriginalFormats              []string
	Start, Restart               int
	Format, Label                string
}
type docxListRef struct {
	ID         string
	Level      int
	Definition docxListLevel
}

func (w *docxWriter) list(b Block, depth int) (string, error) {
	if depth > 8 {
		return "", fmt.Errorf("%w: DOCX numbering supports nine levels", ErrUnsupported)
	}
	outputLevel := depth
	if b.Numbering != nil {
		outputLevel = b.Numbering.Level
	}
	id := len(w.numberings) + 1
	var abstract strings.Builder
	fmt.Fprintf(&abstract, `<w:abstractNum w:abstractNumId="%d"><w:multiLevelType w:val="multilevel"/>`, id-1)
	for level := 0; level < 9; level++ {
		format, label := "decimal", fmt.Sprintf("%%%d.", level+1)
		if b.List == "bullet" {
			format = "bullet"
			label = "•"
		}
		start := 1
		if level == outputLevel {
			start = listStart(b)
		}
		extra := ""
		if b.Numbering != nil {
			if level <= outputLevel {
				format = b.Numbering.Formats[level]
				if b.Numbering.Legal {
					format = "decimal"
				}
			}
			if level < outputLevel {
				start = b.Numbering.Ancestors[level]
			}
			if level == outputLevel {
				label = b.Numbering.Template
				if b.Numbering.Legal {
					extra = "<w:isLgl/>"
				}
			}
		}
		implicit := ""
		if level == outputLevel && b.Numbering != nil && b.Numbering.Legal {
			raw, _ := json.Marshal(b.Numbering.Formats)
			implicit = ` md:numberingFormats="` + escape(string(raw)) + `"`
		}
		if level == outputLevel && b.Numbering != nil && b.Numbering.Independent {
			implicit += ` md:numberingIndependent="1"`
		}
		if level == outputLevel && b.Numbering == nil {
			implicit = ` md:numberingImplicit="1"`
		}
		fmt.Fprintf(&abstract, `<w:lvl w:ilvl="%d"%s><w:start w:val="%d"/><w:numFmt w:val="%s"/><w:lvlText w:val="%s"/>%s<w:lvlJc w:val="left"/><w:pPr><w:tabs><w:tab w:val="num" w:pos="%d"/></w:tabs><w:ind w:left="%d" w:hanging="240"/></w:pPr></w:lvl>`, level, implicit, start, format, escape(label), extra, (depth+1)*360, (depth+1)*360)
	}
	abstract.WriteString(`</w:abstractNum>`)
	number := fmt.Sprintf(`<w:num w:numId="%d"><w:abstractNumId w:val="%d"/><w:lvlOverride w:ilvl="%d"><w:startOverride w:val="%d"/></w:lvlOverride></w:num>`, id, id-1, outputLevel, listStart(b))
	w.numberings = append(w.numberings, docxNumbering{abstract.String(), number})
	var out strings.Builder
	w.bookmarks++
	listBookmark := w.bookmarks
	fmt.Fprintf(&out, `<w:bookmarkStart w:id="%d" w:name="mdlist1_%s_%d_%d"/>`, listBookmark, b.List, listStart(b), listBookmark)
	for itemIndex, item := range b.Items {
		w.bookmarks++
		itemBookmark := w.bookmarks
		fmt.Fprintf(&out, `<w:bookmarkStart w:id="%d" w:name="mditem1_%d"/>`, itemBookmark, itemBookmark)
		blocks := append([]Block(nil), item.Blocks...)
		if itemIndex == 0 && b.docxBeforeList {
			blocks[0].docxBeforeList = true
			blocks[0].docxExplicitBreakBefore = true
		}
		blocks[0].docxNumberID = id
		blocks[0].docxNumberLevel = outputLevel
		for i := 1; i < len(blocks); i++ {
			if blocks[i].Kind != ListBlock {
				blocks[i].docxIndent = (depth + 1) * 360
			}
		}
		value, err := w.blocksAt(blocks, depth+1)
		if err != nil {
			return "", err
		}
		out.WriteString(value)
		fmt.Fprintf(&out, `<w:bookmarkEnd w:id="%d"/>`, itemBookmark)
	}
	fmt.Fprintf(&out, `<w:bookmarkEnd w:id="%d"/>`, listBookmark)
	return out.String(), nil
}

type docxNumbering struct{ abstract, number string }

func (w *docxWriter) numberingXML() string {
	var out strings.Builder
	out.WriteString(xmlHeader + `<w:numbering xmlns:w="` + wordNS + `" xmlns:md="` + docxNotesNS + `" xmlns:mc="http://schemas.openxmlformats.org/markup-compatibility/2006" mc:Ignorable="md">`)
	for _, n := range w.numberings {
		out.WriteString(n.abstract)
	}
	for _, n := range w.numberings {
		out.WriteString(n.number)
	}
	out.WriteString(`</w:numbering>`)
	return out.String()
}

func (r *docxReader) taggedList(n *xmlNode, rels map[string]docxRelation, depth int) (Block, error) {
	parts := strings.Split(strings.TrimPrefix(n.child("sdtPr").child("tag").attr("val"), docxListTag), ":")
	if len(parts) != 2 || (parts[0] != "bullet" && parts[0] != "number") {
		return Block{}, fmt.Errorf("invalid manydoc list tag")
	}
	start, err := strconv.Atoi(parts[1])
	if err != nil || start < 0 || start > 2147483647 {
		return Block{}, fmt.Errorf("invalid manydoc list start")
	}
	b := Block{Kind: ListBlock, List: parts[0]}
	if b.List == "number" {
		b.Start = listStartPointer(start)
	} else if start != 1 {
		return Block{}, fmt.Errorf("bullet list cannot have a start")
	}
	content := n.child("sdtContent")
	if content == nil {
		return Block{}, fmt.Errorf("missing tagged list content")
	}
	r.inTaggedList++
	defer func() { r.inTaggedList-- }()
	firstID := ""
	firstLevel := -1
	for _, item := range content.Children {
		if item.Name.Local != "sdt" || item.child("sdtPr").child("tag").attr("val") != docxItemTag || item.child("sdtContent") == nil {
			return Block{}, fmt.Errorf("list range requires item boundaries")
		}
		bs, err := r.blocks(item.child("sdtContent"), rels, depth+1)
		if err != nil {
			return Block{}, err
		}
		if len(bs) > 0 && bs[0].Kind == ListBlock && bs[0].docxList != nil && len(bs[0].Items) == 1 {
			ref := bs[0].docxList
			unfolded := append([]Block(nil), bs[0].Items[0].Blocks...)
			unfolded[0].docxList = ref
			unfolded[0].List = ref.Definition.Kind
			bs = append(unfolded, bs[1:]...)
		}
		if len(bs) == 0 || (bs[0].Kind != Paragraph && bs[0].Kind != Heading) || bs[0].docxList == nil || bs[0].docxList.Definition.Kind != b.List {
			return Block{}, fmt.Errorf("list item range requires a matching numbered paragraph")
		}
		if bs[0].docxBeforeList {
			if len(b.Items) > 0 {
				return Block{}, fmt.Errorf("list break metadata requires first item")
			}
			b.docxBeforeList = true
			bs[0].docxBeforeList = false
		}
		ref := bs[0].docxList
		if ref.Definition.Start != start {
			return Block{}, fmt.Errorf("list tag start disagrees with DOCX numbering")
		}
		if firstID == "" {
			firstID = ref.ID
			firstLevel = ref.Level
			b.Numbering = r.publicListNumbering(ref, nil)
		} else if ref.ID != firstID || ref.Level != firstLevel {
			return Block{}, fmt.Errorf("list item ranges disagree on numbering identity/level")
		}
		bs[0].List = ""
		bs[0].docxList = nil
		b.Items = append(b.Items, ListItem{Blocks: bs})
	}
	if firstID != "" {
		if r.rangeNumIDs == nil {
			r.rangeNumIDs = map[string]bool{}
		}
		if r.rangeNumIDs[firstID] {
			return Block{}, fmt.Errorf("numbering identity reused by separate explicit list ranges")
		}
		r.rangeNumIDs[firstID] = true
	}
	return b, nil
}

func docxInteger(n *xmlNode, fallback, maximum int) (int, error) {
	if n == nil {
		return fallback, nil
	}
	value, err := strconv.Atoi(n.attr("val"))
	if err != nil || value < 0 || value > maximum {
		return 0, fmt.Errorf("invalid DOCX %s value %q", n.Name.Local, n.attr("val"))
	}
	return value, nil
}
func parseDOCXListLevel(n *xmlNode, level int) (docxListLevel, error) {
	start, err := docxInteger(n.child("start"), 0, 2147483647)
	if err != nil {
		return docxListLevel{}, err
	}
	restart, err := docxInteger(n.child("lvlRestart"), level, 9)
	if err != nil {
		return docxListLevel{}, err
	}
	if restart > level {
		restart = level
	}
	format := n.child("numFmt").attr("val")
	kind := "number"
	if format == "bullet" {
		kind = "bullet"
	}
	legal := false
	var errLegal error
	if n.child("isLgl") != nil {
		legal, errLegal = docxOnOff(n.child("isLgl"))
	}
	err = errLegal
	if err != nil {
		return docxListLevel{}, err
	}
	implicit := n.attributeNS(docxNotesNS, "numberingImplicit")
	if implicit != "" && implicit != "1" {
		return docxListLevel{}, fmt.Errorf("invalid numbering implicit marker")
	}
	independent := n.attributeNS(docxNotesNS, "numberingIndependent")
	if independent != "" && independent != "1" {
		return docxListLevel{}, fmt.Errorf("invalid independent numbering marker")
	}
	var originals []string
	if raw := n.attributeNS(docxNotesNS, "numberingFormats"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &originals); err != nil || !legal || len(originals) != level+1 {
			return docxListLevel{}, fmt.Errorf("invalid legal numbering format metadata")
		}
		for _, f := range originals {
			if !supportedNumberFormat(f) {
				return docxListLevel{}, fmt.Errorf("invalid legal numbering source format")
			}
		}
	}
	label := n.child("lvlText").attr("val")
	if implicit == "1" && (independent != "" || legal || (kind == "number" && (format != "decimal" || label != fmt.Sprintf("%%%d.", level+1)))) {
		return docxListLevel{}, fmt.Errorf("numbering implicit marker disagrees with definition")
	}
	return docxListLevel{Kind: kind, Start: start, Restart: restart, Format: format, Label: label, Legal: legal, Implicit: implicit == "1", Independent: independent == "1", Presentation: n.child("rPr") != nil, OriginalFormats: originals}, nil
}
func (r *docxReader) readNumbering(n *xmlNode) error {
	if n.Name.Local != "numbering" {
		return fmt.Errorf("invalid DOCX numbering root")
	}
	abstract := map[string]map[int]docxListLevel{}
	for _, a := range n.Children {
		if a.Name.Local == "abstractNum" {
			levels := map[int]docxListLevel{}
			for _, lvl := range a.Children {
				if lvl.Name.Local != "lvl" {
					continue
				}
				level, err := strconv.Atoi(lvl.attr("ilvl"))
				if err != nil || level < 0 || level > 8 {
					return fmt.Errorf("invalid DOCX numbering level")
				}
				if _, ok := levels[level]; ok {
					return fmt.Errorf("duplicate DOCX numbering level")
				}
				def, err := parseDOCXListLevel(lvl, level)
				if err != nil {
					return err
				}
				levels[level] = def
			}
			id := a.attr("abstractNumId")
			if abstract[id] != nil {
				return fmt.Errorf("duplicate DOCX abstract numbering ID")
			}
			abstract[id] = levels
		}
	}
	for _, a := range n.Children {
		if a.Name.Local == "num" {
			id := a.attr("numId")
			if r.num[id] != nil {
				return fmt.Errorf("duplicate DOCX numbering ID")
			}
			base := abstract[a.child("abstractNumId").attr("val")]
			if base == nil {
				return fmt.Errorf("missing DOCX abstract numbering definition")
			}
			levels := map[int]docxListLevel{}
			for k, v := range base {
				levels[k] = v
			}
			for _, override := range a.Children {
				if override.Name.Local != "lvlOverride" {
					continue
				}
				level, err := strconv.Atoi(override.attr("ilvl"))
				if err != nil || level < 0 || level > 8 {
					return fmt.Errorf("invalid numbering override level")
				}
				def, ok := levels[level]
				if lvl := override.child("lvl"); lvl != nil {
					def, err = parseDOCXListLevel(lvl, level)
					if err != nil {
						return err
					}
					ok = true
				}
				if !ok {
					return fmt.Errorf("missing overridden numbering level")
				}
				if override.child("startOverride") == nil && override.child("lvl") == nil {
					def.Start = 0
					def.EmptyOverride = true
				}
				if start := override.child("startOverride"); start != nil {
					def.Start, err = docxInteger(start, 0, 2147483647)
					if err != nil {
						return err
					}
				}
				levels[level] = def
			}
			for level, def := range levels {
				if len(def.OriginalFormats) > 0 {
					for ancestor := 0; ancestor <= level; ancestor++ {
						if levels[ancestor].Format != "decimal" {
							return fmt.Errorf("legal numbering metadata disagrees with visible formats")
						}
					}
				}
			}
			r.num[id] = levels
		}
	}
	return nil
}
func (r *docxReader) paragraphList(b *Block, ppr *xmlNode, styleID string) error {
	id := ""
	level := 0
	apply := func(pr *xmlNode) error {
		num := pr.child("numPr")
		if num == nil {
			return nil
		}
		if value := num.child("numId"); value != nil {
			id = value.attr("val")
		}
		var err error
		level, err = docxInteger(num.child("ilvl"), level, 8)
		return err
	}
	seen := map[string]bool{}
	var style func(string) error
	style = func(id string) error {
		if id == "" || seen[id] {
			return nil
		}
		seen[id] = true
		s := r.styles[id]
		if s == nil {
			return nil
		}
		if err := style(s.child("basedOn").attr("val")); err != nil {
			return err
		}
		return apply(s.child("pPr"))
	}
	if err := style(styleID); err != nil {
		return err
	}
	if err := apply(ppr); err != nil {
		return err
	}
	if id == "0" {
		return nil
	}
	if id != "" {
		def, ok := r.num[id][level]
		if !ok {
			return fmt.Errorf("missing DOCX numbering ID %q level %d", id, level)
		}
		if def.Presentation {
			r.doc.warn("numbering_presentation", "Numbering label font/color/run presentation is normalized; label text and formats retained")
		}
		if def.EmptyOverride {
			r.doc.warn("numbering_empty_override", "Empty numbering-level override uses Word zero-start compatibility; other readers may inherit the abstract start")
		}
		if !supportedNumberFormat(def.Format) && def.Format != "bullet" {
			r.doc.warn("numbering_format", "Numbering format "+def.Format+" represented as decimal")
		}
		expected := fmt.Sprintf("%%%d.", level+1)
		if def.Kind == "bullet" {
			expected = "•"
		}
		if def.Kind == "bullet" && def.Label != expected {
			r.doc.warn("numbering_label", "Numbering label "+def.Label+" normalized to "+expected)
		}
		b.List = def.Kind
		b.docxList = &docxListRef{ID: id, Level: level, Definition: def}
		return nil
	}
	lower := strings.ToLower(styleID)
	kind := ""
	if strings.Contains(lower, "listbullet") {
		kind = "bullet"
	}
	if strings.Contains(lower, "listnumber") {
		kind = "number"
	}
	if kind != "" {
		r.doc.warn("list_inference", "List membership inferred from style name without a numbering definition")
		b.List = kind
		b.docxList = &docxListRef{ID: "style:" + styleID, Definition: docxListLevel{Kind: kind, Start: 1, Implicit: true}}
	}
	return nil
}

type docxListCounter struct {
	values  [9]int
	started [9]bool
}

func (r *docxReader) foldDocumentLists() error {
	for _, target := range []*[]Block{&r.doc.Blocks, &r.doc.Headers, &r.doc.Footers} {
		bs, err := r.foldLists(*target, map[string]*docxListCounter{}, 0)
		if err != nil {
			return err
		}
		*target = bs
	}
	for i := range r.doc.Notes {
		bs, err := r.foldLists(r.doc.Notes[i].Blocks, map[string]*docxListCounter{}, 0)
		if err != nil {
			return err
		}
		r.doc.Notes[i].Blocks = bs
	}
	return nil
}
func (r *docxReader) foldLists(bs []Block, counters map[string]*docxListCounter, depth int) ([]Block, error) {
	if depth > 64 {
		return nil, fmt.Errorf("DOCX list nesting exceeds 64 levels")
	}
	var out []Block
	for index := 0; index < len(bs); {
		b := bs[index]
		if b.Section != nil {
			var err error
			b.Section.Blocks, err = r.foldLists(b.Section.Blocks, counters, depth+1)
			if err != nil {
				return nil, err
			}
			for _, story := range sectionStories(b.Section) {
				if story != nil {
					story.Blocks, err = r.foldLists(story.Blocks, map[string]*docxListCounter{}, depth+1)
					if err != nil {
						return nil, err
					}
				}
			}
		}
		if b.docxBeforeList && b.Kind != ListBlock {
			return nil, fmt.Errorf("list break metadata outside an explicit list range")
		}
		if b.docxList != nil {
			if r.rangeNumIDs[b.docxList.ID] {
				return nil, fmt.Errorf("numbering identity mixed between explicit ranges and unmarked paragraphs")
			}
			end := index + 1
			for end < len(bs) && bs[end].docxList != nil {
				end++
			}
			cursor := 0
			run := bs[index:end]
			if run[0].docxList.Level > 0 {
				r.doc.warn("list_level_gap", "Numbering starts below level zero; missing ancestors are not invented")
			}
			lists, err := r.foldListLevel(run, &cursor, run[0].docxList.Level, counters, depth+1)
			if err != nil {
				return nil, err
			}
			out = append(out, lists...)
			index += cursor
			continue
		}
		for ri := range b.Rows {
			b.Rows[ri].Cells = append([]Cell(nil), b.Rows[ri].Cells...)
			for ci := range b.Rows[ri].Cells {
				c := &b.Rows[ri].Cells[ci]
				var err error
				c.Blocks, err = r.foldLists(c.Blocks, counters, depth+1)
				if err != nil {
					return nil, err
				}
			}
		}
		for ii := range b.Items {
			var err error
			b.Items[ii].Blocks, err = r.foldLists(b.Items[ii].Blocks, counters, depth+1)
			if err != nil {
				return nil, err
			}
		}
		if b.docxBeforeList {
			out = append(out, Block{Kind: PageBreak})
			b.docxBeforeList = false
		}
		out = append(out, b)
		index++
	}
	return out, nil
}
func (r *docxReader) foldListLevel(run []Block, index *int, level int, counters map[string]*docxListCounter, depth int) ([]Block, error) {
	if depth > 64 {
		return nil, fmt.Errorf("DOCX list nesting exceeds 64 levels")
	}
	var lists []Block
	lastID := ""
	for *index < len(run) {
		b := run[*index]
		ref := b.docxList
		if r.rangeNumIDs[ref.ID] {
			return nil, fmt.Errorf("numbering identity mixed between explicit ranges and unmarked paragraphs")
		}
		if ref.Level < level {
			break
		}
		if ref.Level > level {
			if ref.Level != level+1 {
				r.doc.warn("list_level_gap", "Numbering skips a hierarchy level; missing ancestors are not invented")
			}
			children, err := r.foldListLevel(run, index, ref.Level, counters, depth+1)
			if err != nil {
				return nil, err
			}
			if len(lists) == 0 {
				lists = append(lists, children...)
			} else {
				parent := &lists[len(lists)-1]
				for _, child := range children {
					if child.Numbering != nil && r.listSequences[child.Numbering] != lastID {
						child.Numbering.Independent = true
					}
				}
				item := &parent.Items[len(parent.Items)-1]
				item.Blocks = append(item.Blocks, children...)
			}
			continue
		}
		counter := counters[ref.ID]
		if counter == nil {
			counter = &docxListCounter{}
			counters[ref.ID] = counter
		}
		for ancestor := 0; ancestor < level; ancestor++ {
			if !counter.started[ancestor] {
				def, ok := r.num[ref.ID][ancestor]
				if ok {
					counter.values[ancestor] = def.Start
					counter.started[ancestor] = true
				}
			}
		}
		if !counter.started[level] {
			counter.values[level] = ref.Definition.Start
			counter.started[level] = true
		} else {
			if counter.values[level] == 2147483647 {
				return nil, fmt.Errorf("DOCX list numbering overflows supported integer range")
			}
			counter.values[level]++
			if counter.values[level] > 2147483647 {
				return nil, fmt.Errorf("DOCX list numbering overflows supported integer range")
			}
		}
		for deeper := level + 1; deeper < 9; deeper++ {
			if def, ok := r.num[ref.ID][deeper]; ok && def.Restart > 0 && def.Restart-1 == level {
				counter.started[deeper] = false
			}
		}
		if lastID != ref.ID || len(lists) == 0 || lists[len(lists)-1].List != ref.Definition.Kind {
			list := Block{Kind: ListBlock, List: ref.Definition.Kind, Numbering: r.publicListNumbering(ref, counter)}
			if list.List == "number" {
				list.Start = listStartPointer(counter.values[level])
			}
			lists = append(lists, list)
			lastID = ref.ID
		}
		var item ListItem
		if b.Kind == ListBlock {
			item = b.Items[0]
			var err error
			item.Blocks, err = r.foldLists(item.Blocks, counters, depth+1)
			if err != nil {
				return nil, err
			}
		} else {
			b.docxList = nil
			b.List = ""
			item = ListItem{Blocks: []Block{b}}
		}
		lists[len(lists)-1].Items = append(lists[len(lists)-1].Items, item)
		*index = *index + 1
	}
	return lists, nil
}

// Bookmark ranges preserve block membership without introducing editable
// content controls. Rendering remains ordinary OOXML numbering and hyperlinks.
// Build temporary wrappers only inside the reader to reuse the checked tree
// reconstruction; no wrapper is persisted as a second document representation.
func expandListBookmarks(children []*xmlNode, depth int) ([]*xmlNode, error) {
	if depth > 64 {
		return nil, fmt.Errorf("DOCX list range nesting exceeds 64")
	}
	var result []*xmlNode
	for index := 0; index < len(children); index++ {
		n := children[index]
		name := n.attr("name")
		if n.Name.Local != "bookmarkStart" || (!strings.HasPrefix(name, "mdlist1_") && !strings.HasPrefix(name, "mditem1_")) {
			result = append(result, n)
			continue
		}
		id := n.attr("id")
		numeric, err := strconv.Atoi(id)
		if err != nil || numeric < 1 || numeric > 2147483647 {
			return nil, fmt.Errorf("invalid list bookmark ID")
		}
		tag := docxItemTag
		if strings.HasPrefix(name, "mdlist1_") {
			parts := strings.Split(strings.TrimPrefix(name, "mdlist1_"), "_")
			if len(parts) != 3 || parts[2] != id {
				return nil, fmt.Errorf("invalid list bookmark name")
			}
			tag = docxListTag + parts[0] + ":" + parts[1]
		} else if strings.TrimPrefix(name, "mditem1_") != id {
			return nil, fmt.Errorf("invalid item bookmark name")
		}
		end := index + 1
		for end < len(children) && !(children[end].Name.Local == "bookmarkEnd" && children[end].attr("id") == id) {
			end++
		}
		if end == len(children) {
			return nil, fmt.Errorf("unclosed list bookmark range")
		}
		content, err := expandListBookmarks(children[index+1:end], depth+1)
		if err != nil {
			return nil, err
		}
		wrapper := &xmlNode{Name: xml.Name{Local: "sdt"}, Children: []*xmlNode{
			{Name: xml.Name{Local: "sdtPr"}, Children: []*xmlNode{{Name: xml.Name{Local: "tag"}, Attr: []xml.Attr{{Name: xml.Name{Local: "val"}, Value: tag}}}}},
			{Name: xml.Name{Local: "sdtContent"}, Children: content},
		}}
		result = append(result, wrapper)
		index = end
	}
	return result, nil
}

// Capture unpainted ancestor counters for a list resumed outside its ancestors.
// Enclosing SF lists can subsequently replace these fallbacks when edited.
func (r *docxReader) publicListNumbering(ref *docxListRef, counter *docxListCounter) *ListNumbering {
	if ref.Definition.Kind != "number" || ref.Definition.Implicit {
		return nil
	}
	n := &ListNumbering{Level: ref.Level, Template: ref.Definition.Label, Legal: ref.Definition.Legal, Independent: ref.Definition.Independent, Formats: make([]string, ref.Level+1), Ancestors: nil}
	if ref.Level > 0 {
		n.Ancestors = make([]int, ref.Level)
	}
	for level := 0; level <= ref.Level; level++ {
		def, ok := r.num[ref.ID][level]
		if level == ref.Level {
			def = ref.Definition
			ok = true
		}
		if !ok {
			def = docxListLevel{Format: "decimal", Start: 1}
			r.doc.warn("numbering_context_inferred", "Missing ancestor numbering definition uses decimal start one")
		}
		if def.EmptyOverride {
			r.doc.warn("numbering_empty_override", "Empty numbering-level override uses Word zero-start compatibility; other readers may inherit the abstract start")
		}
		format := def.Format
		if !supportedNumberFormat(format) {
			format = "decimal"
			r.doc.warn("numbering_format", "Numbering format "+def.Format+" represented as decimal")
		}
		n.Formats[level] = format
		if level < ref.Level {
			n.Ancestors[level] = def.Start
			if counter != nil && counter.started[level] {
				n.Ancestors[level] = counter.values[level]
			}
		}
	}
	if len(ref.Definition.OriginalFormats) > 0 {
		n.Formats = append([]string(nil), ref.Definition.OriginalFormats...)
	}
	if r.listSequences == nil {
		r.listSequences = map[*ListNumbering]string{}
	}
	r.listSequences[n] = ref.ID
	return n
}
