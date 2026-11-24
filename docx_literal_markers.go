package manydoc

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Geometry only: content remains in ordinary OOXML cells. Counter intent is
// carried by the actual standard numbering definition, not serialized SF text.
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

// The layout owner may discard its container only when no extra visible row,
// cell or border feature is hidden by doing so. Real cell content is decoded by
// the normal checked reader, including structural empty/tail paragraphs.
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
