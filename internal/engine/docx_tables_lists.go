package engine

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"math"
	"reflect"
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
	automaticID, resumeCounter := id, false
	w.bookmarks++
	listBookmark := w.bookmarks
	fmt.Fprintf(&out, `<w:bookmarkStart w:id="%d" w:name="mdlist1_%s_%d_%d"/>`, listBookmark, b.List, listStart(b), listBookmark)
	for itemIndex, item := range b.Items {
		w.bookmarks++
		itemBookmark := w.bookmarks
		fmt.Fprintf(&out, `<w:bookmarkStart w:id="%d" w:name="mditem1_%d"/>`, itemBookmark, itemBookmark)
		if item.Marker != nil {
			resumeCounter = true
			if itemIndex == 0 && b.docxBeforeList {
				item.Blocks = append([]Block(nil), item.Blocks...)
				item.Blocks[0].docxBeforeList = true
				item.Blocks[0].docxExplicitBreakBefore = true
			}
			value, err := w.literalListItem(item, id, outputLevel)
			if err != nil {
				return "", err
			}
			out.WriteString(value)
			fmt.Fprintf(&out, `<w:bookmarkEnd w:id="%d"/>`, itemBookmark)
			continue
		}
		blocks := append([]Block(nil), item.Blocks...)
		if itemIndex == 0 && b.docxBeforeList {
			blocks[0].docxBeforeList = true
			blocks[0].docxExplicitBreakBefore = true
		}
		if resumeCounter {
			automaticID = len(w.numberings) + 1
			resume := fmt.Sprintf(`<w:num w:numId="%d"><w:abstractNumId w:val="%d"/><w:lvlOverride w:ilvl="%d"><w:startOverride w:val="%d"/></w:lvlOverride></w:num>`, automaticID, id-1, outputLevel, listStart(b)+itemIndex)
			w.numberings = append(w.numberings, docxNumbering{number: resume})
			resumeCounter = false
		}
		blocks[0].docxNumberID = automaticID
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
	automaticID := ""
	resumeCounter := false
	usedIDs := map[string]bool{}
	for _, item := range content.Children {
		if item.Name.Local != "sdt" || item.child("sdtPr").child("tag").attr("val") != docxItemTag || item.child("sdtContent") == nil {
			return Block{}, fmt.Errorf("list range requires item boundaries")
		}
		var marker *ListMarker
		var bs []Block
		var err error
		children := item.child("sdtContent").Children
		pageStart := ""
		if len(children) > 0 && children[0].attributeNS(docxNotesNS, "literalPageStart") != "" {
			pageStart, err = literalPageStart(children[0])
			if err != nil {
				return Block{}, err
			}
			children = children[1:]
		}
		if len(children) == 1 && children[0].attributeNS(docxNotesNS, "literalMarker") != "" {
			var decoded ListItem
			var ref *docxListRef
			decoded, ref, err = r.literalListItem(children[0], rels, depth+1)
			marker, bs = decoded.Marker, decoded.Blocks
			if err == nil {
				bs[0].docxList = ref
				bs[0].List = ref.Definition.Kind
			}
		} else if len(children) > 0 && children[0].Name.Space == "" && strings.HasPrefix(children[0].child("sdtPr").child("tag").attr("val"), docxMarkerTag) {
			var decoded ListItem
			var ref *docxListRef
			decoded, ref, err = r.sequentialLiteralItem(children, rels, depth+1)
			marker, bs = decoded.Marker, decoded.Blocks
			if err == nil {
				bs[0].docxList = ref
				bs[0].List = ref.Definition.Kind
			}
		} else {
			bs, err = r.blocks(&xmlNode{Children: children}, rels, depth+1)
		}
		if err != nil {
			return Block{}, err
		}
		if pageStart != "" {
			if marker == nil {
				return Block{}, fmt.Errorf("literal page-start requires literal item")
			}
			bs[0].PageBreakBefore = pageStart == "conditional" || pageStart == "both"
			bs[0].docxBeforeList = pageStart == "explicit" || pageStart == "both"
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
		usedIDs[ref.ID] = true
		if firstID == "" {
			if ref.Definition.Start != start {
				return Block{}, fmt.Errorf("list tag start disagrees with DOCX numbering")
			}
			firstID, firstLevel = ref.ID, ref.Level
			automaticID = ref.ID
			b.Numbering = r.publicListNumbering(ref, nil)
		} else if ref.Level != firstLevel {
			return Block{}, fmt.Errorf("list item ranges disagree on numbering level")
		}
		if marker != nil {
			if ref.ID != firstID || ref.Definition.Start != start {
				return Block{}, fmt.Errorf("literal item disagrees with list numbering")
			}
			resumeCounter = true
		} else if resumeCounter {
			if ref.Definition.Start != start+len(b.Items) || !r.sameListCounter(firstID, ref.ID, firstLevel) {
				return Block{}, fmt.Errorf("resumed list counter disagrees with item position")
			}
			automaticID, resumeCounter = ref.ID, false
		} else if ref.ID != automaticID {
			return Block{}, fmt.Errorf("list item ranges disagree on numbering identity")
		}

		bs[0].List = ""
		bs[0].docxList = nil
		b.Items = append(b.Items, ListItem{Marker: marker, Blocks: bs})
	}
	if firstID != "" {
		if r.rangeNumIDs == nil {
			r.rangeNumIDs = map[string]bool{}
		}
		for id := range usedIDs {
			if r.rangeNumIDs[id] {
				return Block{}, fmt.Errorf("numbering identity reused by separate explicit list ranges")
			}
		}
		for id := range usedIDs {
			r.rangeNumIDs[id] = true
		}
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
		if n.Name.Local != "bookmarkStart" || (!strings.HasPrefix(name, "mdlist1_") && !strings.HasPrefix(name, "mditem1_") && !strings.HasPrefix(name, "mdmarker1_")) {
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
		} else if strings.HasPrefix(name, "mdmarker1_") {
			parts := strings.Split(strings.TrimPrefix(name, "mdmarker1_"), "_")
			if len(parts) != 3 || parts[2] != id {
				return nil, fmt.Errorf("invalid literal marker bookmark name")
			}
			tag = docxMarkerTag + parts[0] + ":" + parts[1]
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

// Literal items consume a logical index without a numbered Word paragraph.
// A resumed automatic run uses a real start override. Compare every definition
// level before accepting its new identity inside the checked list range.

// Geometry only: content remains in ordinary OOXML cells. Counter intent is
// carried by the actual standard numbering definition, not serialized SF text.

// The layout owner may discard its container only when no extra visible row,
// cell or border feature is hidden by doing so. Real cell content is decoded by
// the normal checked reader, including structural empty/tail paragraphs.

// Only the immediately preceding row can continue a merge. A missing vMerge
// closes it; a later continuation cannot reopen a historical origin.

// Required empty continuation paragraphs carry no visible content. Any
// real content is retained at the origin, with an explicit placement loss.

// Retain the preference before container fitting/twip quantization only
// when its independently visible grid agrees with the serialized result.

// Grid widths are initial preferences, not a complete Word layout.

// Word can override the initial grid through total/cell preferences and auto-fit.
// Preserve the grid and disclose conflicts rather than implying full layout.

func (r *docxReader) sameListCounter(base, candidate string, level int) bool {
	a, b := r.num[base], r.num[candidate]
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		other, ok := b[k]
		if !ok {
			return false
		}
		if k == level {
			other.Start = v.Start
		}
		if !reflect.DeepEqual(v, other) {
			return false
		}
	}
	return true
}

func docxLiteralTable(item ListItem, width float64) (Block, string) {
	label := math.Min(120, width/4)
	cells := []Cell{{Blocks: item.Marker.Blocks}, {Blocks: item.Blocks}}
	widths := []float64{label, width - label}
	side := "left"
	if item.Blocks[0].Direction == "rtl" {
		cells[0], cells[1] = cells[1], cells[0]
		widths[0], widths[1] = widths[1], widths[0]
		side = "right"
	}
	return Block{Kind: Table, ColumnWidths: widths, Rows: []Row{{Cells: cells}}}, side
}

func (w *docxWriter) literalListItem(item ListItem, id, level int) (string, error) {
	prefix := ""
	first := item.Blocks[0]
	if first.PageBreakBefore || first.docxExplicitBreakBefore {
		role := "conditional"
		if first.docxExplicitBreakBefore {
			role = "explicit"
			if first.PageBreakBefore {
				role = "both"
			}
		}
		prefix = fmt.Sprintf(`<w:p md:literalPageStart="%s"><w:pPr><w:pageBreakBefore/><w:spacing w:before="0" w:after="0" w:line="20" w:lineRule="exact"/></w:pPr></w:p>`, role)
		item.Blocks = append([]Block(nil), item.Blocks...)
		item.Blocks[0].PageBreakBefore = false
		item.Blocks[0].docxExplicitBreakBefore = false
		item.Blocks[0].docxBeforeList = false
	}
	if w.noteStory {
		w.bookmarks++
		mark := w.bookmarks
		label, err := w.blocks(item.Marker.Blocks)
		if err != nil {
			return "", err
		}
		body, err := w.blocks(item.Blocks)
		if err != nil {
			return "", err
		}
		return prefix + fmt.Sprintf(`<w:bookmarkStart w:id="%d" w:name="mdmarker1_%d_%d_%d"/>%s<w:bookmarkEnd w:id="%d"/>%s`, mark, id, level, mark, label, mark, body), nil
	}
	table, side := docxLiteralTable(item, w.contentWidth)
	attrs := fmt.Sprintf(` md:literalMarker="1" md:numberId="%d" md:numberLevel="%d" md:markerSide="%s"`, id, level, side)
	value, err := w.tableLayout(table, attrs, true)
	return prefix + value, err
}

func (r *docxReader) literalListItem(n *xmlNode, rels map[string]docxRelation, depth int) (ListItem, *docxListRef, error) {
	fail := func() (ListItem, *docxListRef, error) {
		return ListItem{}, nil, fmt.Errorf("invalid literal marker table ownership/geometry")
	}
	if n.Name.Local != "tbl" || n.attributeNS(docxNotesNS, "literalMarker") != "1" {
		return fail()
	}
	id := n.attributeNS(docxNotesNS, "numberId")
	parsed, err := strconv.Atoi(id)
	if err != nil || parsed < 1 || strconv.Itoa(parsed) != id {
		return fail()
	}
	levelText := n.attributeNS(docxNotesNS, "numberLevel")
	level, err := strconv.Atoi(levelText)
	if err != nil || level < 0 || level > 8 || strconv.Itoa(level) != levelText {
		return fail()
	}
	def, ok := r.num[id][level]
	if !ok {
		return fail()
	}
	side := n.attributeNS(docxNotesNS, "markerSide")
	if side != "left" && side != "right" {
		return fail()
	}
	if !validDOCXLiteralLayout(n) {
		return fail()
	}
	table, err := r.table(n, rels, depth)
	if err != nil {
		return ListItem{}, nil, err
	}
	if len(table.Rows) != 1 || table.Rows[0].RepeatHeader || len(table.Rows[0].Cells) != 2 {
		return fail()
	}
	for _, c := range table.Rows[0].Cells {
		if c.ColSpan > 1 || c.RowSpan > 1 {
			return fail()
		}
	}
	marker, body := table.Rows[0].Cells[0].Blocks, table.Rows[0].Cells[1].Blocks
	if side == "right" {
		marker, body = body, marker
	}
	if len(marker) == 0 || len(body) == 0 || body[0].Kind != Paragraph && body[0].Kind != Heading || body[0].docxList != nil || body[0].List != "" {
		return fail()
	}
	if (body[0].Direction == "rtl") != (side == "right") {
		return fail()
	}
	if err := validateLiteralMarker(marker, 0); err != nil {
		return ListItem{}, nil, err
	}
	return ListItem{Marker: &ListMarker{Blocks: marker}, Blocks: body}, &docxListRef{ID: id, Level: level, Definition: def}, nil
}

func validDOCXLiteralLayout(n *xmlNode) bool {
	counts := map[string]int{}
	for _, c := range n.Children {
		counts[c.Name.Local]++
		if counts[c.Name.Local] > 1 {
			return false
		}
		if c.Name.Space != wordNS {
			return false
		}
		switch c.Name.Local {
		case "tblPr", "tblGrid":
		case "tr":
			for _, cell := range c.Children {
				if cell.Name.Space != wordNS || cell.Name.Local != "tc" {
					return false
				}
				pr := cell.child("tcPr")
				if pr == nil {
					return false
				}
				for _, p := range pr.Children {
					if p.Name.Space != wordNS || p.Name.Local != "tcW" {
						return false
					}
				}
			}
		default:
			return false
		}
	}
	pr := n.child("tblPr")
	if pr == nil || counts["tblGrid"] != 1 || counts["tr"] != 1 {
		return false
	}
	seenProperties := map[string]bool{}
	for _, p := range pr.Children {
		if p.Name.Space != wordNS || seenProperties[p.Name.Local] {
			return false
		}
		seenProperties[p.Name.Local] = true
		switch p.Name.Local {
		case "tblW", "tblBorders", "tblLayout", "tblCellMar":
		default:
			return false
		}
	}
	borders := pr.child("tblBorders")
	if borders == nil || len(borders.Children) != 6 {
		return false
	}
	seen := map[string]bool{}
	for _, border := range borders.Children {
		switch border.Name.Local {
		case "top", "left", "bottom", "right", "insideH", "insideV":
		default:
			return false
		}
		if seen[border.Name.Local] || border.attr("val") != "nil" && border.attr("val") != "none" {
			return false
		}
		seen[border.Name.Local] = true
	}
	return true
}

const docxMarkerTag = "manydoc:literal-marker:v1:"

func (r *docxReader) sequentialLiteralItem(children []*xmlNode, rels map[string]docxRelation, depth int) (ListItem, *docxListRef, error) {
	fail := func() (ListItem, *docxListRef, error) {
		return ListItem{}, nil, fmt.Errorf("invalid sequential literal marker ownership")
	}
	if len(children) < 2 || children[0].Name.Space != "" {
		return fail()
	}
	marker := children[0]
	tag := marker.child("sdtPr").child("tag").attr("val")
	parts := strings.Split(strings.TrimPrefix(tag, docxMarkerTag), ":")
	if !strings.HasPrefix(tag, docxMarkerTag) || len(parts) != 2 {
		return fail()
	}
	id, err := strconv.Atoi(parts[0])
	if err != nil || id < 1 || strconv.Itoa(id) != parts[0] {
		return fail()
	}
	level, err := strconv.Atoi(parts[1])
	if err != nil || level < 0 || level > 8 || strconv.Itoa(level) != parts[1] {
		return fail()
	}
	def, ok := r.num[parts[0]][level]
	if !ok {
		return fail()
	}
	if marker.child("sdtContent") == nil {
		return fail()
	}
	label, err := r.blocks(marker.child("sdtContent"), rels, depth+1)
	if err != nil {
		return ListItem{}, nil, err
	}
	body, err := r.blocks(&xmlNode{Children: children[1:]}, rels, depth+1)
	if err != nil {
		return ListItem{}, nil, err
	}
	if len(label) == 0 || len(body) == 0 || body[0].Kind != Paragraph && body[0].Kind != Heading || body[0].List != "" || body[0].docxList != nil {
		return fail()
	}
	if err := validateLiteralMarker(label, 0); err != nil {
		return ListItem{}, nil, err
	}
	return ListItem{Marker: &ListMarker{Blocks: label}, Blocks: body}, &docxListRef{ID: parts[0], Level: level, Definition: def}, nil
}

func literalPageStart(n *xmlNode) (string, error) {
	role := n.attributeNS(docxNotesNS, "literalPageStart")
	if role != "conditional" && role != "explicit" && role != "both" {
		return "", fmt.Errorf("invalid literal item page-start role")
	}
	if n.Name.Space != wordNS || n.Name.Local != "p" || len(n.Children) != 1 || n.Children[0].Name.Local != "pPr" || len(n.Children[0].Children) != 2 {
		return "", fmt.Errorf("literal item page-start hides content")
	}
	pr := n.Children[0]
	if pr.Name.Space != wordNS || pr.child("pageBreakBefore") == nil || pr.child("pageBreakBefore").Name.Space != wordNS {
		return "", fmt.Errorf("invalid literal page-start namespace")
	}
	if pr.child("pageBreakBefore") == nil || len(pr.child("pageBreakBefore").Attr) != 0 || len(pr.child("pageBreakBefore").Children) != 0 {
		return "", fmt.Errorf("invalid literal item page-start control")
	}
	sp := pr.child("spacing")
	if sp == nil || sp.Name.Space != wordNS || sp.attr("before") != "0" || sp.attr("after") != "0" || sp.attr("line") != "20" || sp.attr("lineRule") != "exact" || len(sp.Children) != 0 {
		return "", fmt.Errorf("invalid literal item page-start geometry")
	}
	return role, nil
}
func docxSpan(n *xmlNode) (int, error) {
	if n == nil {
		return 0, nil
	}
	value, err := strconv.Atoi(n.attr("val"))
	if err != nil || value < 1 || value > 1000 {
		return 0, fmt.Errorf("invalid DOCX gridSpan %q", n.attr("val"))
	}
	return value, nil
}

func (r *docxReader) table(n *xmlNode, rels map[string]docxRelation, depth int) (Block, error) {
	b := Block{Kind: Table}

	type origin struct{ row, cell, span int }
	active := map[int]origin{}
	for _, row := range n.Children {
		if row.Name.Local != "tr" {
			continue
		}
		if err := r.ctx.Err(); err != nil {
			return Block{}, err
		}
		for _, name := range []string{"gridBefore", "gridAfter"} {
			if prop := row.child("trPr").child(name); prop != nil && prop.attr("val") != "0" {
				return Block{}, fmt.Errorf("%w: DOCX skipped table grid cells (%s)", ErrUnsupported, name)
			}
		}
		ri := len(b.Rows)
		repeat, err := docxTableHeader(row.child("trPr"))
		if err != nil {
			return Block{}, err
		}
		if repeat && (ri > 0 && !b.Rows[ri-1].RepeatHeader) {
			r.doc.warn("table_header_ignored", "Noncontiguous DOCX table header property is ignored according to OOXML")
			repeat = false
		}
		b.Rows = append(b.Rows, Row{RepeatHeader: repeat})
		next := map[int]origin{}
		column := 0
		for _, cell := range row.Children {
			if cell.Name.Local != "tc" {
				continue
			}
			bs, err := r.blocks(cell, rels, depth+1)
			if err != nil {
				return Block{}, err
			}
			span, err := docxSpan(cell.child("tcPr").child("gridSpan"))
			if err != nil {
				return Block{}, err
			}
			width := max(1, span)
			merge := cell.child("tcPr").child("vMerge")
			if merge != nil && (merge.attr("val") == "" || merge.attr("val") == "continue") {
				o, ok := active[column]
				if !ok || o.span != width {
					return Block{}, fmt.Errorf("DOCX vertical merge continuation has no matching origin at row %d column %d", ri+1, column+1)
				}
				target := &b.Rows[o.row].Cells[o.cell]
				target.RowSpan++

				if emptyMergeContinuation(bs) {
					var anchors []string
					for _, block := range bs {
						anchors = append(anchors, block.Anchors...)
						for _, run := range block.Runs {
							if run.Anchor != "" {
								anchors = append(anchors, run.Anchor)
							}
						}
					}
					if len(anchors) > 0 {
						target.Blocks = append(target.Blocks, Block{Kind: AnchorPoint, Anchors: anchors})
						r.doc.warn("merge_continuation_anchor", "Vertical merge continuation navigation points retained at the origin; original placement is normalized")
					}
				} else {
					target.Blocks = append(target.Blocks, bs...)
					r.doc.warn("merge_continuation_content", "Vertical merge continuation content retained at its origin; original placement is normalized")
				}
				next[column] = o
			} else {
				if merge != nil && merge.attr("val") != "restart" {
					return Block{}, fmt.Errorf("invalid DOCX vertical merge value %q", merge.attr("val"))
				}
				c := Cell{Blocks: bs, ColSpan: span}
				if merge != nil {
					c.RowSpan = 1
					next[column] = origin{ri, len(b.Rows[ri].Cells), width}
				}
				b.Rows[ri].Cells = append(b.Rows[ri].Cells, c)
			}
			column += width
		}
		active = next
	}
	headers, _ := tableHeaderRows(b.Rows)
	for ri := 0; ri < headers; ri++ {
		for _, c := range b.Rows[ri].Cells {
			if ri+max(1, c.RowSpan) > headers {
				for i := range b.Rows {
					b.Rows[i].RepeatHeader = false
				}
				r.doc.warn("table_header_merge", "DOCX merge crosses the repeating-header boundary; content and merge retained, header repetition disabled")
				break
			}
		}
	}
	g, err := tableGrid(b.Rows)
	if err != nil {
		return Block{}, err
	}
	if err := r.tableWidths(n, &b, g.Columns); err != nil {
		return Block{}, err
	}
	return b, nil
}

func emptyMergeContinuation(bs []Block) bool {
	for _, b := range bs {
		if (b.Kind != Paragraph && b.Kind != AnchorPoint) || b.List != "" || b.PageBreakBefore {
			return false
		}
		for _, run := range b.Runs {
			if run.Text != "" || run.Note != nil {
				return false
			}
		}
	}
	return true
}

func docxTableColumns(b Block, width float64) (*tableGeometry, []int, error) {
	g, err := tableGrid(b.Rows)
	if err != nil {
		return nil, nil, err
	}
	available := width - float64(b.docxIndent)/20
	points, err := fittedColumnWidths(b.ColumnWidths, g.Columns, available)
	if err != nil {
		return nil, nil, err
	}
	widths := make([]int, g.Columns)
	if len(b.ColumnWidths) == 0 {
		total := int(math.Round(available * 20))
		for i := range widths {
			widths[i] = total / g.Columns
			if i < total%g.Columns {
				widths[i]++
			}
		}
	} else {
		position, previous := 0.0, 0
		for i, pt := range points {
			position += pt * 20
			next := int(math.Round(position))
			widths[i] = next - previous
			previous = next
		}
	}
	for _, w := range widths {
		if w < 1 {
			return nil, nil, fmt.Errorf("%w: DOCX table column narrower than one twip", ErrUnsupported)
		}
	}

	return g, widths, nil
}

func (w *docxWriter) table(b Block) (string, error) {
	return w.tableLayout(b, "", false)
}

func (w *docxWriter) tableLayout(b Block, extra string, borderless bool) (string, error) {
	g, widths, err := docxTableColumns(b, w.contentWidth)
	if err != nil {
		return "", err
	}
	total := 0
	for _, width := range widths {
		total += width
	}
	var out strings.Builder
	metadata, err := json.Marshal(b.ColumnWidths)
	if err != nil {
		return "", err
	}
	fmt.Fprintf(&out, `<w:tbl%s md:columnWidths="%s" md:columnWidthContainer="%s"><w:tblPr><w:tblW w:w="%d" w:type="dxa"/>`, extra, escape(string(metadata)), strconv.FormatFloat(w.contentWidth-float64(b.docxIndent)/20, 'g', -1, 64), total)
	if b.docxIndent > 0 {
		fmt.Fprintf(&out, `<w:tblInd w:w="%d" w:type="dxa"/>`, b.docxIndent)
	}
	if borderless {
		out.WriteString(`<w:tblBorders><w:top w:val="nil"/><w:left w:val="nil"/><w:bottom w:val="nil"/><w:right w:val="nil"/><w:insideH w:val="nil"/><w:insideV w:val="nil"/></w:tblBorders>`)
	} else {
		out.WriteString(`<w:tblBorders><w:top w:val="single" w:sz="4"/><w:left w:val="single" w:sz="4"/><w:bottom w:val="single" w:sz="4"/><w:right w:val="single" w:sz="4"/><w:insideH w:val="single" w:sz="4"/><w:insideV w:val="single" w:sz="4"/></w:tblBorders>`)
	}
	out.WriteString(`<w:tblLayout w:type="fixed"/><w:tblCellMar><w:left w:w="80" w:type="dxa"/><w:right w:w="80" w:type="dxa"/></w:tblCellMar></w:tblPr><w:tblGrid>`)

	for _, width := range widths {
		fmt.Fprintf(&out, `<w:gridCol w:w="%d"/>`, width)
	}
	out.WriteString(`</w:tblGrid>`)
	for ri, slots := range g.Slots {
		if err := w.ctx.Err(); err != nil {
			return "", err
		}
		out.WriteString(`<w:tr>`)
		if b.Rows[ri].RepeatHeader {
			out.WriteString(`<w:trPr><w:tblHeader/></w:trPr>`)
		}
		for col := 0; col < len(slots); {
			p := slots[col]
			c := b.Rows[p.Row].Cells[p.Cell]
			cellWidth := 0
			for _, v := range widths[col : col+p.ColSpan] {
				cellWidth += v
			}
			fmt.Fprintf(&out, `<w:tc><w:tcPr><w:tcW w:w="%d" w:type="dxa"/>`, cellWidth)
			if p.ColSpan > 1 {
				fmt.Fprintf(&out, `<w:gridSpan w:val="%d"/>`, p.ColSpan)
			}
			if p.RowSpan > 1 {
				if ri == p.Row {
					out.WriteString(`<w:vMerge w:val="restart"/>`)
				} else {
					out.WriteString(`<w:vMerge/>`)
				}
			}
			out.WriteString(`</w:tcPr>`)
			if ri == p.Row {
				previousWidth := w.contentWidth
				previousHeight := w.contentHeight
				w.contentHeight -= docxImageCellHeightAllowance
				w.contentWidth = float64(cellWidth)/20 - 8
				v, err := w.blocks(c.Blocks)
				w.contentWidth = previousWidth
				w.contentHeight = previousHeight
				if err != nil {
					return "", err
				}
				out.WriteString(v)
				last := len(c.Blocks) - 1
				for last >= 0 && c.Blocks[last].Kind == AnchorPoint {
					last--
				}
				if last < 0 {
					out.WriteString(`<w:p md:emptyCell="1"/>`)
				} else if c.Blocks[last].Kind == Table {
					out.WriteString(`<w:p md:tableTail="1"/>`)
				}
			} else {
				out.WriteString(`<w:p/>`)
			}
			out.WriteString(`</w:tc>`)
			col += p.ColSpan
		}
		out.WriteString(`</w:tr>`)
	}
	out.WriteString(`</w:tbl>`)
	return out.String(), nil
}

func docxTableHeader(pr *xmlNode) (bool, error) {
	var found *xmlNode
	if pr != nil {
		for _, c := range pr.Children {
			if c.Name.Local == "tblHeader" {
				if found != nil {
					return false, fmt.Errorf("duplicate DOCX tblHeader property")
				}
				found = c
			}
		}
	}
	return docxOnOff(found)
}
func (r *docxReader) tableWidths(n *xmlNode, b *Block, columns int) error {
	var widths []float64
	complete := true
	grid := n.child("tblGrid")
	var children []*xmlNode
	if grid != nil {
		children = grid.Children
	}
	for _, col := range children {
		if col.Name.Local != "gridCol" {
			continue
		}
		if len(widths) >= 10000 {
			return fmt.Errorf("DOCX table grid exceeds column limit")
		}
		w, err := docxPageNumber(col, "w", 0)
		if err != nil || w < 0 {
			return fmt.Errorf("invalid DOCX table grid column width")
		}
		if w == 0 {
			complete = false
		}
		widths = append(widths, w)
	}
	if len(widths) != columns {
		complete = false
	}
	if !complete {
		if len(widths) > 0 {
			r.doc.warn("table_width_incomplete", "Incomplete or unspecified DOCX grid widths use equal container columns; content and logical spans retained")
		}
		if n.attributeNS(docxNotesNS, "columnWidths") != "" {
			return fmt.Errorf("invalid DOCX table width metadata without complete grid")
		}
		r.tableWidthOverrides(n, widths)
		return nil
	}
	if err := validateColumnWidths(widths, columns); err != nil {
		return err
	}
	if marker := n.attributeNS(docxNotesNS, "columnWidths"); marker != "" {

		var original []float64
		if err := json.Unmarshal([]byte(marker), &original); err != nil {
			return fmt.Errorf("invalid DOCX table width metadata")
		}
		if columns == 0 {
			if len(original) != 0 {
				return fmt.Errorf("nonempty widths on empty table")
			}
			return nil
		}
		candidate := Block{Rows: b.Rows, ColumnWidths: original}
		available, err := strconv.ParseFloat(n.attributeNS(docxNotesNS, "columnWidthContainer"), 64)
		if err != nil || !finite(available) || available <= 0 || available > 1e7 {
			return fmt.Errorf("invalid DOCX table width container metadata")
		}
		_, expected, err := docxTableColumns(candidate, available)
		if err != nil {
			return err
		}
		for i, w := range widths {
			if math.Abs(w*20-float64(expected[i])) > 1e-6 {
				return fmt.Errorf("DOCX table width metadata disagrees with grid")
			}
		}
		b.ColumnWidths = original
	} else {
		b.ColumnWidths = widths

		if layout := n.child("tblPr").child("tblLayout"); layout == nil || layout.attr("type") != "fixed" {
			r.doc.warn("table_autofit", "DOCX initial grid widths retained; Word automatic content-based table resizing is normalized to fixed preferred columns")
		}
	}
	r.tableWidthOverrides(n, widths)
	return nil
}

func (r *docxReader) tableWidthOverrides(n *xmlNode, widths []float64) {
	total := 0.0
	for _, w := range widths {
		total += w
	}
	if pref := n.child("tblPr").child("tblW"); pref != nil {
		kind := pref.attr("type")
		w, err := docxPageNumber(pref, "w", 0)
		if kind == "pct" || (kind == "dxa" && (err != nil || math.Abs(w-total) > .025)) {
			r.doc.warn("table_width_preference", "DOCX table total-width override differs from initial grid preferences; grid widths retained without Word layout resolution")
		}
	}
	for _, row := range n.Children {
		if row.Name.Local != "tr" {
			continue
		}
		column := 0
		for _, cell := range row.Children {
			if cell.Name.Local != "tc" {
				continue
			}
			span, _ := docxSpan(cell.child("tcPr").child("gridSpan"))
			span = max(1, span)
			if pref := cell.child("tcPr").child("tcW"); pref != nil {
				kind := pref.attr("type")
				w, err := docxPageNumber(pref, "w", 0)
				sum := 0.0
				for c := column; c < min(column+span, len(widths)); c++ {
					sum += widths[c]
				}
				if kind == "pct" || (kind == "dxa" && (err != nil || math.Abs(w-sum) > .025)) {
					r.doc.warn("table_cell_width_preference", "DOCX cell-width overrides differ from initial grid preferences; grid and spans retained without Word layout resolution")
				}
			}
			column += span
		}
	}
}
