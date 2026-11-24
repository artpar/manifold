package manydoc

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strings"
)

// Native tables retain a real stream body plus ordinal source records. SF omits
// vertically covered cells; native rows require zero-row-span continuation tokens.
type colleafNumberUnit struct {
	Value float64 `json:"v"`
	Unit  int     `json:"u"`
}
type colleafTableWidth struct {
	Type  int               `json:"type"`
	Width colleafNumberUnit `json:"width"`
}
type colleafNativeRow struct {
	Cells      []json.RawMessage `json:"tableCells"`
	Repeat     json.RawMessage   `json:"repeatHeaderRow,omitempty"`
	GridBefore int               `json:"gridBefore,omitempty"`
	GridAfter  int               `json:"gridAfter,omitempty"`
}
type colleafNativeTable struct {
	ID      string            `json:"tableId"`
	Rows    []json.RawMessage `json:"tableRows"`
	Columns []struct {
		Size colleafTableWidth `json:"size"`
	} `json:"tableColumns"`
	Size colleafTableWidth `json:"size"`
}

type colleafBodyWriter struct {
	pointScopeSerial       int
	pointScope, pointOwner string
	pointSerial            int
	citationProjection     map[noteKey]string
	ctx                    context.Context
	stream                 strings.Builder
	body                   colleafBody
	tables                 map[string]json.RawMessage
	offset, serial         int
	noteIDs                map[noteKey]string
	lists                  map[string]json.RawMessage
	listBindings           []colleafListBinding
	listDepth              int
	listGroup              string
	documentID, storyID    string
	drawings               map[string]json.RawMessage
	drawingOrder           []string
	imageGeometry          []colleafImageBinding
	imageCache             outputImageCache
}

func (w *colleafBodyWriter) text(s string) { w.stream.WriteString(s); w.offset += colleafUTF16Len(s) }
func (w *colleafBodyWriter) id(prefix string) string {
	w.serial++
	return fmt.Sprintf("%s%d", prefix, w.serial)
}
func (w *colleafBodyWriter) paragraph(block Block) error {
	sourceBlock := block
	block.Source = nil
	fragment, err := encodeColleafTextBodyWithNotes(w.ctx, []Block{block}, w.noteIDs, w.citationProjection)
	if err != nil {
		return err
	}
	origin, err := newColleafOrigin(sourceBlock)
	if err != nil {
		return err
	}
	if origin != nil {
		for i := range fragment.Paragraphs {
			props, err := colleafStyleObject(fragment.Paragraphs[i].Style)
			if err != nil {
				return err
			}
			props["manydocOrigin"] = sfAttrs(origin)
			fragment.Paragraphs[i].Style = sfAttrs(props)
		}
	}
	paragraphIDs := map[string]string{}
	shift := w.offset
	w.text(strings.TrimSuffix(fragment.DataStream, "\n"))
	for _, p := range fragment.Paragraphs {
		p.StartIndex += shift
		oldID := p.ID
		p.ID = w.id("p")
		paragraphIDs[oldID] = p.ID
		w.body.Paragraphs = append(w.body.Paragraphs, p)
	}
	for _, r := range fragment.TextRuns {
		r.St += shift
		r.Ed += shift
		w.body.TextRuns = append(w.body.TextRuns, r)
	}
	for _, r := range fragment.CustomRanges {
		r.StartIndex += shift
		r.EndIndex += shift
		if r.Type != 3 {
			r.ID = w.id("link")
		}
		if err := rebindColleafBookmark(&r, paragraphIDs); err != nil {
			return err
		}
		nameColleafCitationRange(&r)
		w.body.CustomRanges = append(w.body.CustomRanges, r)
	}
	for _, atom := range fragment.CustomBlocks {
		atom.StartIndex += shift
		atom.ID = w.id("note")
		w.body.CustomBlocks = append(w.body.CustomBlocks, atom)
	}
	return nil
}
func (w *colleafBodyWriter) blocks(blocks []Block, depth int) error {
	if depth > 64 {
		return fmt.Errorf("native body nesting exceeds64")
	}
	allPoints := len(blocks) > 0
	for _, b := range blocks {
		allPoints = allPoints && b.Kind == AnchorPoint
	}
	if len(blocks) == 0 {
		if err := w.paragraph(Block{Kind: Paragraph}); err != nil {
			return err
		}
	}
	scope := w.pointScopeID()
	firstRange := len(w.body.CustomRanges)
	if err := w.pointFlow(blocks, depth, "section", scope); err != nil {
		return err
	}
	if allPoints {
		if err := w.paragraph(Block{Kind: Paragraph}); err != nil {
			return err
		}
		carrier := w.body.Paragraphs[len(w.body.Paragraphs)-1].ID
		for i := firstRange; i < len(w.body.CustomRanges); i++ {
			a := &w.body.CustomRanges[i]
			if a.Type != 3 {
				continue
			}
			p, err := readColleafBookmark(*a)
			if err != nil {
				return err
			}
			if p.Hint != nil && p.Hint.Role == "point" && p.Hint.OwnerID == scope {
				p.Hint.PointCarrier = carrier
				a.Properties = sfAttrs(p)
			}
		}
	}
	sectionID := w.id("s")
	if err := rebindColleafPointScope(&w.body, scope, sectionID, firstRange); err != nil {
		return err
	}
	w.body.SectionBreaks = append(w.body.SectionBreaks, colleafSectionRecord{StartIndex: w.offset, ID: sectionID})
	w.text("\n")
	return nil
}
func (w *colleafBodyWriter) flow(blocks []Block, depth int) error {
	if depth > 64 {
		return fmt.Errorf("native body nesting exceeds64")
	}
	for _, b := range blocks {
		if err := w.ctx.Err(); err != nil {
			return err
		}
		if b.Kind == AnchorPoint {
			if err := w.standalonePoint(b); err != nil {
				return err
			}
		} else if b.Kind == PageBreak {
			if b.Source != nil {
				return fmt.Errorf("%w: native page-break provenance requires mapping", ErrUnsupported)
			}
			if err := w.containerAnchors(b.Anchors, w.offset, "break", "page-break"); err != nil {
				return err
			}
			w.text("\f")
			w.body.Paragraphs = append(w.body.Paragraphs, colleafParagraphRecord{StartIndex: w.offset, ID: w.id("p")})
			w.text("\r")
		} else if b.Kind == Picture {
			if err := w.picture(b); err != nil {
				return err
			}
		} else if b.Kind == ListBlock {
			if err := w.list(b, depth+1); err != nil {
				return err
			}
		} else if b.Kind == Table {
			if err := w.table(b, depth+1); err != nil {
				return err
			}
		} else {
			if err := w.paragraph(b); err != nil {
				return err
			}
		}
	}
	return nil
}

func (w *colleafBodyWriter) table(block Block, depth int) error {
	if len(block.Runs) > 0 || block.Image != nil || len(block.Items) > 0 || block.Section != nil || block.List != "" || block.Level != 0 || block.Start != nil || block.Numbering != nil {
		return fmt.Errorf("invalid native table block")
	}
	if block.Source != nil || block.PageBreakBefore || block.Direction != "" {
		return fmt.Errorf("%w: native table annotations require mapping", ErrUnsupported)
	}
	grid, err := tableGrid(block.Rows)
	if err != nil {
		return err
	}
	if grid.Columns == 0 || len(block.Rows) == 0 {
		return fmt.Errorf("native table needs a nonempty grid")
	}
	if err = validateColumnWidths(block.ColumnWidths, grid.Columns); err != nil {
		return err
	}
	id := w.id("table")
	start := w.offset
	if err := w.containerAnchors(block.Anchors, start, "table", id); err != nil {
		return err
	}
	w.text("\x1a")
	rows := make([]any, 0, len(block.Rows))
	for ri, row := range block.Rows {
		w.text("\x1b")
		cells := []any{}
		for col := 0; col < grid.Columns; {
			// The shared SF grid tolerates trailing uncovered slots in ragged rows;
			// native rectangular rows must never invent a missing visible cell.
			if col >= len(grid.Slots[ri]) || grid.Slots[ri][col] == nil {
				return fmt.Errorf("%w: ragged native table grid", ErrUnsupported)
			}
			origin := grid.Slots[ri][col]
			if origin.Column != col {
				return fmt.Errorf("invalid native span origin")
			}
			w.text("\x1c")
			rs := 0
			if origin.Row == ri {
				cell := row.Cells[origin.Cell]
				if cell.Source != nil {
					return fmt.Errorf("%w: native cell provenance requires mapping", ErrUnsupported)
				}
				rs = origin.RowSpan
				if err = w.blocks(cell.Blocks, depth); err != nil {
					return err
				}
			} else {
				if err = w.blocks(nil, depth); err != nil {
					return err
				}
			}
			w.text("\x1d")
			cells = append(cells, map[string]any{"rowSpan": rs, "columnSpan": origin.ColSpan})
			col += origin.ColSpan
		}
		w.text("\x0e")
		nativeRow := map[string]any{"tableCells": cells, "trHeight": map[string]any{"val": colleafNumberUnit{0, 3}, "hRule": 0}}
		if row.RepeatHeader {
			nativeRow["repeatHeaderRow"] = 1
		}
		rows = append(rows, nativeRow)
	}
	w.text("\x0f")
	w.body.Tables = append(w.body.Tables, colleafTableRecord{StartIndex: start, EndIndex: w.offset, ID: id})
	columns := make([]any, grid.Columns)
	total := 0.0
	unit := 3
	for i := range columns {
		width := 100.0 / float64(grid.Columns)
		if len(block.ColumnWidths) > 0 {
			width = block.ColumnWidths[i] * 4 / 3
		} else {
			unit = 4
		}
		columns[i] = map[string]any{"size": colleafTableWidth{1, colleafNumberUnit{width, unit}}}
		total += width
	}
	if unit == 4 {
		total = 100
	}
	w.tables[id] = sfAttrs(map[string]any{"tableId": id, "tableRows": rows, "tableColumns": columns, "align": 0, "indent": colleafNumberUnit{0, 3}, "textWrap": 0, "position": map[string]any{"positionH": map[string]int{"relativeFrom": 0}, "positionV": map[string]int{"relativeFrom": 0}}, "dist": map[string]int{"distT": 0, "distB": 0, "distL": 0, "distR": 0}, "size": colleafTableWidth{1, colleafNumberUnit{total, unit}}, "layout": 1})
	// Native tables are children of an anchoring paragraph, whose CR is structural.
	w.body.Paragraphs = append(w.body.Paragraphs, colleafParagraphRecord{StartIndex: w.offset, ID: w.id("p")})
	w.text("\r")
	return nil
}
func encodeColleafBody(ctx context.Context, blocks []Block) (colleafBody, map[string]json.RawMessage, error) {
	return encodeColleafBodyWithNotes(ctx, blocks, nil)
}
func encodeColleafBodyWithNotes(ctx context.Context, blocks []Block, noteIDs map[noteKey]string) (colleafBody, map[string]json.RawMessage, error) {
	body, tables, lists, bindings, err := encodeColleafListBody(ctx, blocks, noteIDs)
	if err != nil {
		return colleafBody{}, nil, err
	}
	if len(lists) > 0 || len(bindings) > 0 {
		return colleafBody{}, nil, fmt.Errorf("%w: native list body requires list source/ownership transport", ErrUnsupported)
	}
	return body, tables, nil
}

func encodeColleafListBody(ctx context.Context, blocks []Block, noteIDs map[noteKey]string) (colleafBody, map[string]json.RawMessage, map[string]json.RawMessage, []colleafListBinding, error) {
	encoded, err := encodeColleafAssetBody(ctx, blocks, noteIDs, "manydoc", "", nil)
	if err != nil {
		return colleafBody{}, nil, nil, nil, err
	}
	if len(encoded.Drawings) > 0 {
		return colleafBody{}, nil, nil, nil, fmt.Errorf("%w: native images require drawing source/geometry transport", ErrUnsupported)
	}
	return encoded.Body, encoded.Tables, encoded.Lists, encoded.ListsOwnership, nil
}

// Decode a paragraph fragment using original effective intervals and hyperlink
// identities. Offsets are translated only after intersecting scalar-safe ranges.
func (r *colleafBodyReader) paragraphFragment(start, end int, p colleafParagraphRecord) (colleafBody, error) {
	text, err := r.index.text.exclusive(start, end)
	if err != nil {
		return colleafBody{}, err
	}
	out := colleafBody{DataStream: text + "\r\n"}
	originalEnd := p.StartIndex
	if start != r.paragraphStarts[originalEnd] && len(p.Style) > 0 {
		props, err := colleafStyleObject(p.Style)
		if err != nil {
			return out, err
		}
		delete(props, "headingId")
		p.Style = sfAttrs(props)
	}
	p.StartIndex = end - start
	out.Paragraphs = []colleafParagraphRecord{p}
	first := sort.Search(len(r.spans), func(i int) bool { return r.spans[i].end > start })
	for _, span := range r.spans[first:] {
		if span.start >= end {
			break
		}
		if span.record < 0 {
			continue
		}
		run := r.body.TextRuns[span.record]
		run.St = max(start, span.start) - start
		run.Ed = min(end, span.end) - start
		out.TextRuns = append(out.TextRuns, run)
	}
	first = sort.Search(len(r.ranges), func(i int) bool { return r.ranges[i].EndIndex >= start })
	for _, a := range r.ranges[first:] {
		if a.StartIndex >= end {
			break
		}
		a.StartIndex = max(start, a.StartIndex) - start
		a.EndIndex = min(end-1, a.EndIndex) - start
		out.CustomRanges = append(out.CustomRanges, a)
	}
	first = sort.Search(len(r.bookmarks), func(i int) bool { return r.bookmarks[i].StartIndex >= start })
	for _, a := range r.bookmarks[first:] {
		if a.StartIndex > end {
			break
		}
		if r.bookmarkUsed[a.ID] {
			continue
		}
		bp, err := readColleafBookmark(a)
		if err != nil {
			return out, err
		}
		if bp.Hint != nil && bp.Hint.Role == "block" && (a.StartIndex != r.paragraphStarts[originalEnd] || bp.Hint.ParagraphID != p.ID) {
			hint := *bp.Hint
			hint.Role = "run"
			fields, err := colleafStyleObject(a.Properties)
			if err != nil {
				return out, err
			}
			fields["manydocNavigation"] = sfAttrs(hint)
			a.Properties = sfAttrs(fields)
			r.doc.warn("colleaf_stale_bookmark_placement", "Edited native bookmark position is retained as a current inline point rather than old block placement")
		}
		if a.EndIndex != a.StartIndex {
			r.doc.warn("colleaf_bookmark_span", "Native bookmark range editing extent becomes an SF point at its current start")
		}
		a.StartIndex -= start
		a.EndIndex = a.StartIndex
		out.CustomRanges = append(out.CustomRanges, a)
		r.bookmarkUsed[a.ID] = true
	}
	first = sort.SearchInts(r.atomPositions, start)
	for _, position := range r.atomPositions[first:] {
		if position >= end {
			break
		}
		atom := r.index.blocks[position]
		raw, err := r.index.text.exclusive(position, position+1)
		if err != nil {
			return out, err
		}
		atom.StartIndex -= start
		if raw == "\ufffc" && atom.NoteID != "" {
			kind := 8
			if atom.NoteType == Endnote {
				kind = 9
			}
			out.CustomRanges = append(out.CustomRanges, colleafRangeRecord{StartIndex: atom.StartIndex, EndIndex: atom.StartIndex, ID: atom.ID, Type: kind, Whole: true, Properties: sfAttrs(map[string]string{"noteId": atom.NoteID})})
		} else {
			out.CustomBlocks = append(out.CustomBlocks, atom)
		}
	}

	return out, nil
}

type colleafBodyReader struct {
	cursorWarningSet   map[string]bool
	cursorKinds        map[int]string
	cursorBookmarks    map[int][]colleafRangeRecord
	pointSlots         map[colleafPointSlot]bool
	standalone         []colleafStandalonePoint
	standaloneIDs      map[string]bool
	pointsAt           map[int][]colleafStandalonePoint
	pointCarriers      map[string]bool
	sectionIDs         map[int]string
	sectionPoints      map[int][]string
	listPointPositions map[string]int
	listPointOwners    map[string]string
	promotedListPoints map[string]bool
	containerPoints    map[int][]string
	ctx                context.Context
	body               colleafBody
	index              *colleafBodyIndex
	settings           colleafTextContext
	tables             map[string]json.RawMessage
	doc                *Document
	paragraphs         map[int]colleafParagraphRecord
	used               map[string]bool
	spans              []colleafStyleSpan
	ranges             []colleafRangeRecord
	bookmarks          []colleafRangeRecord
	bookmarkUsed       map[string]bool
	paragraphStarts    map[int]int
	atomPositions      []int
	listParagraphs     map[int]colleafListParagraph
	listBindings       map[string]colleafListBinding
	images             map[string]*Image
	imageBindings      map[string]colleafImageBinding
}

func (r *colleafBodyReader) paragraph(start, end int, p colleafParagraphRecord) ([]Block, error) {
	return r.breakParagraph(start, end, p)
}
func (r *colleafBodyReader) textParagraph(start, end int, p colleafParagraphRecord) ([]Block, error) {
	p.Bullet = nil
	fragment, err := r.paragraphFragment(start, end, p)
	if err != nil {
		return nil, err
	}
	doc, err := decodeColleafTextBody(r.ctx, fragment, r.settings)
	if err != nil {
		return nil, err
	}
	r.doc.Diagnostics = append(r.doc.Diagnostics, doc.Diagnostics...)
	return doc.Blocks, nil
}
func (r *colleafBodyReader) sections(sections []colleafStreamNode, depth int) ([]Block, error) {
	if depth > 64 {
		return nil, fmt.Errorf("native body nesting exceeds64")
	}
	if len(sections) != 1 {
		return nil, fmt.Errorf("%w: native sections require settings mapping", ErrUnsupported)
	}
	var flow []colleafFlowBlock
	push := func(bs []Block, p colleafParagraphRecord, start int) {
		for _, b := range bs {
			if b.Kind == AnchorPoint && len(flow) > 0 && flow[len(flow)-1].Position == p.StartIndex {
				flow[len(flow)-1].After = append(flow[len(flow)-1].After, b)
				continue
			}
			if len(flow) > 0 && flow[len(flow)-1].Position == p.StartIndex {
				p.Bullet = nil
			}
			flow = append(flow, colleafFlowBlock{Block: b, ID: p.ID, Bullet: p.Bullet, Position: p.StartIndex, Start: start})
		}
	}
	for _, node := range sections[0].children {
		if err := r.ctx.Err(); err != nil {
			return nil, err
		}
		if node.kind != "paragraph" {
			return nil, fmt.Errorf("invalid native body child")
		}
		p := r.paragraphs[node.end-1]
		p.StartIndex = node.end - 1
		if len(node.children) == 0 {
			part, err := r.paragraph(node.start, node.end-1, p)
			if err != nil {
				return nil, err
			}
			if !r.emptyPointCarrier(p, node.start, node.end-1, r.sectionIDs[sections[0].end-1], part) {
				push(part, p, node.start)
			}
			continue
		}
		cursor := node.start
		mixed := false
		if len(p.Bullet) > 0 && node.children[0].start == node.start {
			empty, err := r.paragraph(node.start, node.start, p)
			if err != nil {
				return nil, err
			}
			push(empty, p, node.start)
		}
		for _, child := range node.children {
			if cursor < child.start {
				part, err := r.paragraph(cursor, child.start, p)
				if err != nil {
					return nil, err
				}
				push(part, p, cursor)
				mixed = true
			}
			if child.kind != "table" {
				return nil, fmt.Errorf("%w: native container", ErrUnsupported)
			}
			table, err := r.table(child, depth+1)
			if err != nil {
				return nil, err
			}
			push([]Block{table}, p, child.start)
			after, err := r.cursorBlocks(child.end-1, "boundary after table", false)
			if err != nil {
				return nil, err
			}
			flow[len(flow)-1].After = append(flow[len(flow)-1].After, after...)
			if child.end == node.end-1 {
				after, err = r.cursorBlocks(node.end-1, "boundary after table", false)
				if err != nil {
					return nil, err
				}
				flow[len(flow)-1].After = append(flow[len(flow)-1].After, after...)
			}
			cursor = child.end
		}
		// Preserve authored suffix text/paragraph properties. An otherwise empty
		// bare anchoring CR is not an extra SF paragraph after every table.
		if cursor < node.end-1 || len(p.Style) > 0 || p.StyleID != "" || len(p.Bullet) > 0 {
			part, err := r.paragraph(cursor, node.end-1, p)
			if err != nil {
				return nil, err
			}
			push(part, p, cursor)
			mixed = true
		}
		if mixed {
			r.doc.warn("colleaf_table_paragraph", "Native table and text sharing one paragraph become ordered SF blocks; visible content and supported paragraph styles are retained")
		}
	}
	blocks, err := r.groupLists(flow, depth, r.sectionIDs[sections[0].end-1], sections[0].start, sections[0].end-1)
	if err != nil {
		return nil, err
	}
	// A point at a terminal LF has no text carrier. Retain a changed scope at
	// this actual current boundary rather than dropping it.
	scope := r.sectionIDs[sections[0].end-1]
	represented := colleafStandaloneBlockIDs(blocks)
	for _, p := range r.pointsAt[sections[0].end-1] {
		if !represented[p.id] && (p.scope != "section" || p.owner != scope) {
			blocks = append(blocks, Block{Kind: AnchorPoint, Anchors: []string{p.id}})
			r.doc.warn("colleaf_stale_point_ownership", "Standalone point retains its current native location because its owning flow or boundary changed")
		}
	}
	terminal, err := r.cursorBlocks(sections[0].end-1, "end of current section flow", false)
	if err != nil {
		return nil, err
	}
	blocks = append(blocks, terminal...)
	return blocks, nil
}
func colleafUnitPoints(unit colleafNumberUnit) (float64, error) {
	if math.IsNaN(unit.Value) || math.IsInf(unit.Value, 0) || unit.Value < 0 {
		return 0, fmt.Errorf("invalid native table dimension")
	}
	switch unit.Unit {
	// The pinned table layout treats legacy POINT(0) values as geometry
	// pixels too; executable reader probes establish this field-specific rule.
	case 0, 3:
		return unit.Value * 3 / 4, nil
	default:
		return 0, fmt.Errorf("%w: relative native table dimension unit%d", ErrUnsupported, unit.Unit)
	}
}
func (r *colleafBodyReader) table(node colleafStreamNode, depth int) (Block, error) {
	block := Block{Kind: Table, Anchors: r.containerPoints[node.start]}
	record := r.index.tables[node.start]
	raw, ok := r.tables[record.ID]
	if !ok {
		return block, fmt.Errorf("native table %q lacks source", record.ID)
	}
	if r.used[record.ID] {
		return block, fmt.Errorf("duplicate native table source ownership")
	}
	r.used[record.ID] = true
	var table colleafNativeTable
	if err := json.Unmarshal(raw, &table); err != nil {
		return block, err
	}
	if table.ID != record.ID || len(table.Rows) != len(node.children) || len(table.Columns) == 0 {
		return block, fmt.Errorf("native table source/stream identity or grid mismatch")
	}
	columns := len(table.Columns)
	if columns > 10000 || len(table.Rows) > 1_000_000/columns {
		return block, fmt.Errorf("native table grid exceeds supported bounds")
	}
	relativeEqual := table.Size.Type == 1 && table.Size.Width.Unit == 4 && math.Abs(table.Size.Width.Value-100) < 1e-9
	for _, col := range table.Columns {
		relativeEqual = relativeEqual && col.Size.Type == 1 && col.Size.Width.Unit == 4 && math.Abs(col.Size.Width.Value-100/float64(columns)) < 1e-9
	}
	explicit := false
	if !relativeEqual {
		for _, col := range table.Columns {
			if col.Size.Type == 0 && col.Size.Width.Value == 0 {
				block.ColumnWidths = append(block.ColumnWidths, 0)
				continue
			}
			if col.Size.Type != 1 {
				return block, fmt.Errorf("%w: native table column sizing", ErrUnsupported)
			}
			width, err := colleafUnitPoints(col.Size.Width)
			if err != nil {
				return block, err
			}
			if width <= 0 {
				return block, fmt.Errorf("native table column must be positive")
			}
			explicit = true
			block.ColumnWidths = append(block.ColumnWidths, width)
		}
		if !explicit {
			block.ColumnWidths = nil
			r.doc.warn("colleaf_table_width", "Native unspecified zero widths become SF equal available widths; content and grid are retained")
		} else {
			for _, width := range block.ColumnWidths {
				if width == 0 {
					return block, fmt.Errorf("%w: mixed automatic/fixed native columns", ErrUnsupported)
				}
			}
		}
	}
	if err := validateColumnWidths(block.ColumnWidths, columns); err != nil {
		return block, err
	}
	// Occupancy names the single origin covering each logical slot. Vertical
	// continuation bodies are validated as empty before they can be omitted in SF.
	var cursorProjections []colleafCellCursorProjection
	occupied := make([][]*tableCellPosition, len(table.Rows))
	for i := range occupied {
		occupied[i] = make([]*tableCellPosition, columns)
	}
	for ri, rowRaw := range table.Rows {
		var row colleafNativeRow
		if err := json.Unmarshal(rowRaw, &row); err != nil {
			return block, err
		}
		rowNode := node.children[ri]
		if len(row.Cells) != len(rowNode.children) || row.GridBefore != 0 || row.GridAfter != 0 {
			return block, fmt.Errorf("%w: native row token/source counts or omitted grid columns", ErrUnsupported)
		}
		outRow := Row{}
		if len(row.Repeat) > 0 {
			repeat, err := colleafBool(row.Repeat)
			if err != nil {
				return block, err
			}
			outRow.RepeatHeader = repeat
		}
		col := 0
		previousSpan := 0
		var previousOrigin *tableCellPosition
		for ci, cellRaw := range row.Cells {
			properties, err := colleafStyleObject(cellRaw)
			if err != nil {
				return block, err
			}
			rs, hasRS, err := colleafRawInt(properties, "rowSpan")
			if err != nil {
				return block, err
			}
			cs, hasCS, err := colleafRawInt(properties, "columnSpan")
			if err != nil {
				return block, err
			}
			if !hasRS {
				rs = 1
			}
			if !hasCS {
				cs = 1
			}
			r.presentationLosses(cellRaw, []string{"rowSpan", "columnSpan"}, "colleaf_cell_presentation")
			if rs < 0 || cs < 0 || rs > len(table.Rows)-ri || cs > columns {
				return block, fmt.Errorf("invalid native cell span")
			}
			cellNode := rowNode.children[ci]
			if rs == 0 {
				content, err := r.index.text.exclusive(cellNode.start+1, cellNode.end-1)
				if err != nil {
					return block, err
				}
				if content != "\r\n" {
					return block, fmt.Errorf("native covered cell contains content; refusing to discard it")
				}
				if cs == 0 {
					if previousSpan <= 1 {
						return block, fmt.Errorf("unowned native horizontal continuation")
					}
					projection, err := r.cellCursorProjection(rowNode, cellNode, ci, len(row.Cells), previousOrigin, true)
					if err != nil {
						return block, err
					}
					if len(projection.before)+len(projection.after) > 0 {
						cursorProjections = append(cursorProjections, projection)
					}
					previousSpan--
					continue
				}
				if col+cs > columns || occupied[ri][col] == nil || occupied[ri][col].Row >= ri || occupied[ri][col].Column != col || occupied[ri][col].ColSpan != cs {
					return block, fmt.Errorf("unowned native vertical continuation")
				}
				for cc := col; cc < col+cs; cc++ {
					if occupied[ri][cc] != occupied[ri][col] {
						return block, fmt.Errorf("contradictory native continuation")
					}
				}
				previousOrigin = occupied[ri][col]
				projection, err := r.cellCursorProjection(rowNode, cellNode, ci, len(row.Cells), previousOrigin, true)
				if err != nil {
					return block, err
				}
				if len(projection.before)+len(projection.after) > 0 {
					cursorProjections = append(cursorProjections, projection)
				}
				col += cs
				previousSpan = cs
				continue
			}
			if cs == 0 || col+cs > columns {
				return block, fmt.Errorf("invalid native cell column span")
			}
			origin := &tableCellPosition{Row: ri, Cell: len(outRow.Cells), Column: col, ColSpan: cs, RowSpan: rs}
			for rr := ri; rr < ri+rs; rr++ {
				for cc := col; cc < col+cs; cc++ {
					if occupied[rr][cc] != nil {
						return block, fmt.Errorf("overlapping native cell span")
					}
					occupied[rr][cc] = origin
				}
			}
			content, err := r.sections(cellNode.children, depth)
			if err != nil {
				return block, err
			}
			projection, err := r.cellCursorProjection(rowNode, cellNode, ci, len(row.Cells), origin, false)
			if err != nil {
				return block, err
			}
			if len(projection.before)+len(projection.after) > 0 {
				cursorProjections = append(cursorProjections, projection)
			}
			previousOrigin = origin
			sfCell := Cell{Blocks: content}
			if rs > 1 {
				sfCell.RowSpan = rs
			}
			if cs > 1 {
				sfCell.ColSpan = cs
			}
			outRow.Cells = append(outRow.Cells, sfCell)
			col += cs
			previousSpan = cs
		}
		if col != columns {
			return block, fmt.Errorf("native table row does not cover its declared grid")
		}
		r.presentationLosses(rowRaw, []string{"tableCells", "repeatHeaderRow", "gridBefore", "gridAfter", "trHeight"}, "colleaf_row_presentation")
		var props map[string]json.RawMessage
		_ = json.Unmarshal(rowRaw, &props)
		if height := props["trHeight"]; len(height) > 0 {
			var h struct {
				Val  colleafNumberUnit `json:"val"`
				Rule int               `json:"hRule"`
			}
			if err := json.Unmarshal(height, &h); err != nil {
				return block, err
			}
			if h.Val.Value != 0 || h.Rule != 0 {
				r.doc.warn("colleaf_row_height", "Native authored row height is not represented; row content and merge ownership are retained")
			}
		}
		block.Rows = append(block.Rows, outRow)
	}
	// Apply all projections after origin cells exist, including earlier-row
	// origins of covered native cells. Preserve source order within each edge.
	beforeByOrigin := map[*tableCellPosition][]Block{}
	afterByOrigin := map[*tableCellPosition][]Block{}
	var origins []*tableCellPosition
	seenOrigin := map[*tableCellPosition]bool{}
	for _, p := range cursorProjections {
		if !seenOrigin[p.origin] {
			seenOrigin[p.origin] = true
			origins = append(origins, p.origin)
		}
		beforeByOrigin[p.origin] = append(beforeByOrigin[p.origin], p.before...)
		afterByOrigin[p.origin] = append(afterByOrigin[p.origin], p.after...)
	}
	for _, origin := range origins {
		cell := &block.Rows[origin.Row].Cells[origin.Cell]
		cell.Blocks = append(append(beforeByOrigin[origin], cell.Blocks...), afterByOrigin[origin]...)
	}
	if _, err := tableGrid(block.Rows); err != nil {
		return block, err
	}
	// Neutral flow defaults are represented by SF's ordinary table flow.
	var props map[string]json.RawMessage
	if err := json.Unmarshal(raw, &props); err != nil {
		return block, err
	}
	total := 0.0
	for _, col := range table.Columns {
		total += col.Size.Width.Value
	}
	typeValue, layout, unit := 0, 0, 3
	if explicit || relativeEqual {
		typeValue, layout = 1, 1
	}
	if relativeEqual {
		unit = 4
		total = 100
	} else if explicit && table.Size.Width.Unit == 0 {
		unit = 0
	}
	neutral := map[string]any{
		"align": 0, "indent": colleafNumberUnit{0, 3}, "textWrap": 0,
		"position": map[string]any{"positionH": map[string]int{"relativeFrom": 0}, "positionV": map[string]int{"relativeFrom": 0}},
		"dist":     map[string]int{"distT": 0, "distB": 0, "distL": 0, "distR": 0},
		"size":     colleafTableWidth{typeValue, colleafNumberUnit{total, unit}}, "layout": layout,
	}
	allowed := []string{"tableId", "tableRows", "tableColumns"}
	for key, expected := range neutral {
		var actual, canonical any
		_ = json.Unmarshal(props[key], &actual)
		_ = json.Unmarshal(sfAttrs(expected), &canonical)
		if reflect.DeepEqual(actual, canonical) {
			allowed = append(allowed, key)
		}
	}
	r.presentationLosses(raw, allowed, "colleaf_table_presentation")
	return block, nil
}
func (r *colleafBodyReader) presentationLosses(raw json.RawMessage, allowed []string, code string) {
	var props map[string]json.RawMessage
	if json.Unmarshal(raw, &props) != nil {
		return
	}
	keys := []string{}
	for key := range props {
		found := false
		for _, a := range allowed {
			if a == key {
				found = true
				break
			}
		}
		if !found {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		r.doc.warn(code, "Native property "+key+" is not represented in SF; table content and supported geometry are retained")
	}
}
func newColleafBodyReader(ctx context.Context, body colleafBody, settings colleafTextContext, tables map[string]json.RawMessage) (*colleafBodyReader, error) {
	index, err := indexColleafBody(ctx, body)
	if err != nil {
		return nil, err
	}
	for _, a := range body.CustomRanges {
		if a.Type != 0 && a.Type != 3 && a.Type != 8 && a.Type != 9 {
			return nil, fmt.Errorf("%w: native annotation type %d requires mapping", ErrUnsupported, a.Type)
		}
	}
	for _, atom := range index.blocks {
		if atom.Type != nil && *atom.Type == 0 {
			if _, ok := settings.Drawings[atom.ID]; !ok {
				return nil, fmt.Errorf("native drawing atom lacks source")
			}
			if atom.NoteID != "" || atom.NoteType != "" {
				return nil, fmt.Errorf("contradictory native image/note atom")
			}
			continue
		}
		key, ok := settings.NoteIDs[atom.NoteID]
		if !ok || (atom.Type != nil && *atom.Type != 1) || (atom.NoteType != "" && key.kind != atom.NoteType) {
			return nil, fmt.Errorf("%w: unmapped/contradictory native atom", ErrUnsupported)
		}
	}
	r := colleafBodyReader{ctx: ctx, body: body, index: index, settings: settings, tables: tables, doc: &Document{}, paragraphs: map[int]colleafParagraphRecord{}, used: map[string]bool{}}
	r.images = map[string]*Image{}
	r.imageBindings = map[string]colleafImageBinding{}
	for _, binding := range settings.ImageBindings {
		if binding.ID == "" {
			return nil, fmt.Errorf("invalid image geometry binding")
		}
		if _, ok := r.imageBindings[binding.ID]; ok {
			return nil, fmt.Errorf("duplicate image geometry binding")
		}
		r.imageBindings[binding.ID] = binding
	}
	order := map[string]bool{}
	for _, id := range settings.DrawingOrder {
		if order[id] {
			return nil, fmt.Errorf("duplicate native drawing order ID")
		}
		if _, ok := settings.Drawings[id]; !ok {
			return nil, fmt.Errorf("native drawing order lacks source")
		}
		order[id] = true
	}
	for position := range index.blocks {
		r.atomPositions = append(r.atomPositions, position)
	}
	sort.Ints(r.atomPositions)
	r.spans, err = colleafStyleSpans(ctx, index.text, body.TextRuns)
	if err != nil {
		return nil, err
	}
	if err := r.indexStructuralCursors(); err != nil {
		return nil, err
	}
	r.bookmarkUsed = map[string]bool{}
	r.containerPoints = map[int][]string{}
	r.sectionPoints = map[int][]string{}
	r.standaloneIDs = map[string]bool{}
	r.pointSlots = map[colleafPointSlot]bool{}
	r.pointsAt = map[int][]colleafStandalonePoint{}
	r.pointCarriers = map[string]bool{}
	r.sectionIDs = map[int]string{}
	sectionEndIDs := map[int]string{}
	for _, s := range body.SectionBreaks {
		sectionEndIDs[s.StartIndex] = s.ID
		r.sectionIDs[s.StartIndex] = s.ID
	}
	sectionStartIDs := map[int]string{}
	for _, s := range index.sections {
		sectionStartIDs[s.start] = sectionEndIDs[s.end-1]
	}
	r.listPointOwners = map[string]string{}
	r.listPointPositions = map[string]int{}
	r.promotedListPoints = map[string]bool{}
	r.paragraphStarts = colleafParagraphStarts(index)
	for _, a := range body.CustomRanges {
		if a.Type == 3 {
			p, err := readColleafBookmark(a)
			if err != nil {
				return nil, err
			}
			if p.Hint != nil && p.Hint.Role == "point" {
				if err := r.registerStandalone(a, p); err != nil {
					return nil, err
				}
			}
			if p.Hint != nil && p.Hint.Role == "section" && p.Hint.OwnerID == sectionStartIDs[a.StartIndex] {
				if err = colleafBookmarkLosses(r.doc, a); err != nil {
					return nil, err
				}
				r.sectionPoints[a.StartIndex] = append(r.sectionPoints[a.StartIndex], p.BookmarkID)
				continue
			}
			claimed, err := r.containerBookmark(a)
			if err != nil {
				return nil, err
			}
			if claimed {
				continue
			}
			if _, ok := r.cursorKinds[a.StartIndex]; ok && !(p.Hint != nil && p.Hint.Role == "point" && r.cursorKinds[a.StartIndex] == "section terminator") {
				r.cursorBookmarks[a.StartIndex] = append(r.cursorBookmarks[a.StartIndex], a)
				continue
			}
			if p.Hint != nil && p.Hint.Role == "list" {
				r.listPointOwners[p.BookmarkID] = p.Hint.OwnerID
				r.listPointPositions[p.BookmarkID] = a.StartIndex
			}
			r.bookmarks = append(r.bookmarks, a)
			continue
		}
		if a.Type != 8 && a.Type != 9 {
			r.ranges = append(r.ranges, a)
		}
	}
	for _, p := range r.standalone {
		r.pointsAt[p.position] = append(r.pointsAt[p.position], p)
	}
	sort.SliceStable(r.bookmarks, func(i, j int) bool { return r.bookmarks[i].StartIndex < r.bookmarks[j].StartIndex })
	sort.Slice(r.ranges, func(i, j int) bool { return r.ranges[i].StartIndex < r.ranges[j].StartIndex })
	for i := 1; i < len(r.ranges); i++ {
		if r.ranges[i-1].EndIndex >= r.ranges[i].StartIndex {
			return nil, fmt.Errorf("conflicting native hyperlink extents")
		}
	}
	r.listBindings = map[string]colleafListBinding{}
	for _, binding := range settings.ListBindings {
		names, err := colleafListBindingTypes(binding)
		if err != nil {
			return nil, err
		}
		for _, name := range names {
			if _, ok := r.listBindings[name]; ok {
				return nil, fmt.Errorf("duplicate native list binding alias")
			}
			r.listBindings[name] = binding
		}
	}

	r.listParagraphs, err = colleafListParagraphs(ctx, body, settings.Lists, r.listBindings)
	if err != nil {
		return nil, err
	}
	policyLoss, staleProjection := false, false
	for _, info := range r.listParagraphs {
		policyLoss = policyLoss || info.PolicyLoss
		staleProjection = staleProjection || info.StaleProjection
	}
	if policyLoss {
		r.doc.warn("colleaf_numbering_policy", "Native zero/legal marker presentation is retained with explicit SF templates/formats; live native numbering policy is not retained")
	}
	if staleProjection {
		r.doc.warn("colleaf_stale_numbering_projection", "Changed native numbering projection retained; old source format intent was not applied")
	}

	for _, p := range body.Paragraphs {
		r.paragraphs[p.StartIndex] = p
	}
	return &r, nil
}

func decodeColleafBody(ctx context.Context, body colleafBody, settings colleafTextContext, tables map[string]json.RawMessage) (*Document, error) {
	r, err := newColleafBodyReader(ctx, body, settings, tables)
	if err != nil {
		return nil, err
	}
	blocks, err := r.sections(r.index.sections, 0)
	if err != nil {
		return nil, err
	}
	r.doc.Blocks = blocks
	r.finishStandalone(blocks)
	r.finishListPoints()
	if len(r.used) != len(tables) {
		return nil, fmt.Errorf("native table source is orphaned")
	}
	if len(r.images) != len(settings.Drawings) {
		return nil, fmt.Errorf("native drawing source is unbound/orphaned")
	}
	return r.doc, nil
}
