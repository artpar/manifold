package manifold

import (
	"fmt"
	"reflect"
)

// Only control/ownership intent is persisted here. Both cells contain actual
// native editable content. No text, assets or SF subtree is cached in metadata.
type colleafLiteralItem struct {
	Version   int            `json:"version"`
	Group     string         `json:"group"`
	Index     int            `json:"index"`
	Count     int            `json:"count"`
	Kind      string         `json:"kind"`
	Start     *int           `json:"start,omitempty"`
	Numbering *ListNumbering `json:"numbering,omitempty"`
	Automatic bool           `json:"automatic,omitempty"`
	Right     bool           `json:"right,omitempty"`
}

func (m *colleafLiteralItem) validate() error {
	if m.Version != 1 || m.Group == "" || len(m.Group) > 256 || m.Count < 1 || m.Count > 1_000_000 || m.Index < 0 || m.Index >= m.Count || m.Kind != "number" && m.Kind != "bullet" {
		return fmt.Errorf("invalid native literal item ownership")
	}
	if err := validateString(m.Group); err != nil {
		return err
	}
	if err := validateNumbering(m.Numbering); err != nil {
		return err
	}
	if m.Kind == "bullet" && (m.Start != nil || m.Numbering != nil) {
		return fmt.Errorf("bullet literal ownership has counter intent")
	}
	b := Block{Start: m.Start}
	start := listStart(b)
	if start < 0 || int64(start)+int64(m.Count)-1 > 2147483647 {
		return fmt.Errorf("invalid native literal counter range")
	}
	return nil
}
func (w *colleafBodyWriter) literalList(block Block, depth int) error {
	if depth > 64 || len(block.Items) == 0 {
		return fmt.Errorf("native literal list nesting/size")
	}
	group := w.id("literal-list")
	for i, item := range block.Items {
		if err := w.ctx.Err(); err != nil {
			return err
		}
		m := &colleafLiteralItem{Version: 1, Group: group, Index: i, Count: len(block.Items), Kind: block.List, Start: block.Start, Numbering: block.Numbering, Automatic: item.Marker == nil, Right: item.Blocks[0].Direction == "rtl"}
		if err := m.validate(); err != nil {
			return err
		}
		table := colleafLiteralTable(block, item, i)
		body := &table.Rows[0].Cells[1]
		if m.Right {
			body = &table.Rows[0].Cells[0]
		}
		body.Blocks = append([]Block(nil), body.Blocks...)
		p := &body.Blocks[0]
		if i == 0 && block.Source != nil {
			hash, err := colleafListFingerprint(block)
			if err != nil {
				return err
			}
			p.colleafListOrigin, err = newColleafSourceOrigin(block.Source, hash, colleafListOriginProfile)
			if err != nil {
				return err
			}
		}
		if item.Source != nil {
			hash, err := colleafItemFingerprint(block, i)
			if err != nil {
				return err
			}
			p.colleafItemOrigin, err = newColleafSourceOrigin(item.Source, hash, colleafItemOriginProfile)
			if err != nil {
				return err
			}
		}
		if item.Marker != nil && item.Marker.Source != nil {
			hash, err := colleafMarkerFingerprint(item.Marker)
			if err != nil {
				return err
			}
			p.colleafMarkerOrigin, err = newColleafSourceOrigin(item.Marker.Source, hash, colleafMarkerOriginProfile)
			if err != nil {
				return err
			}
		}
		if i == 0 {
			table.Anchors = block.Anchors
		}
		if err := w.table(table, depth+1); err != nil {
			return err
		}
		id := w.body.Tables[len(w.body.Tables)-1].ID
		props, err := colleafStyleObject(w.tables[id])
		if err != nil {
			return err
		}
		props["manydocLiteralItem"] = sfAttrs(m)
		w.tables[id] = sfAttrs(props)
	}
	return nil
}

func literalNativeItem(b Block, m *colleafLiteralItem) (ListItem, bool) {
	if b.Kind != Table || len(b.Rows) != 1 || b.Rows[0].RepeatHeader || len(b.Rows[0].Cells) != 2 || len(b.ColumnWidths) != 0 || b.colleafTableOrigin != nil {
		return ListItem{}, false
	}
	for _, c := range b.Rows[0].Cells {
		if c.ColSpan > 1 || c.RowSpan > 1 || c.Source != nil || c.colleafOrigin != nil {
			return ListItem{}, false
		}
	}
	label, body := b.Rows[0].Cells[0].Blocks, b.Rows[0].Cells[1].Blocks
	if m.Right {
		label, body = body, label
	}
	if len(label) == 0 || len(body) == 0 || body[0].Kind != Paragraph && body[0].Kind != Heading || (body[0].Direction == "rtl") != m.Right {
		return ListItem{}, false
	}
	item := ListItem{Blocks: body}
	if !m.Automatic {
		if validateLiteralMarker(label, 0) != nil {
			return ListItem{}, false
		}
		item.Marker = &ListMarker{Blocks: label}
	} else {
		if len(label) != 1 || label[0].Kind != ListBlock || label[0].List != m.Kind || len(label[0].Items) != 1 || len(label[0].Anchors) != 0 || len(label[0].Items[0].Blocks) != 1 || label[0].Items[0].Marker != nil {
			return ListItem{}, false
		}
		p := label[0].Items[0].Blocks[0]
		if !p.colleafAutoLabel || p.Kind != Paragraph || len(p.Runs) != 0 || len(p.Anchors) != 0 || p.PageBreakBefore || !reflect.DeepEqual(label[0].Numbering, m.Numbering) {
			return ListItem{}, false
		}
		if m.Kind == "number" && listStart(label[0]) != listStart(Block{Start: m.Start})+m.Index {
			return ListItem{}, false
		}
	}
	return item, true
}

func (r *colleafBodyReader) literalLists(bs []Block, depth int) ([]Block, error) {
	if depth > 64 {
		return nil, fmt.Errorf("native literal ownership nesting exceeds64")
	}
	// Lists grouped by ordinary bullets can contain literal layout tables too.
	for i := range bs {
		for j := range bs[i].Items {
			var err error
			bs[i].Items[j].Blocks, err = r.literalLists(bs[i].Items[j].Blocks, depth+1)
			if err != nil {
				return nil, err
			}
		}
	}
	var out []Block
	for i := 0; i < len(bs); {
		m := bs[i].colleafLiteral
		if m == nil {
			out = append(out, bs[i])
			i++
			continue
		}
		valid := m.Index == 0 && m.Count <= len(bs)-i
		b := Block{Kind: ListBlock, List: m.Kind, Start: m.Start, Numbering: m.Numbering, Anchors: bs[i].Anchors}
		if valid {
			for j := 0; j < m.Count; j++ {
				current := bs[i+j].colleafLiteral
				if current == nil {
					valid = false
					break
				}
				want := *m
				want.Index = j
				want.Automatic = current.Automatic
				want.Right = current.Right
				if !reflect.DeepEqual(&want, current) || j > 0 && len(bs[i+j].Anchors) > 0 {
					valid = false
					break
				}
				item, ok := literalNativeItem(bs[i+j], current)
				if !ok {
					valid = false
					break
				}
				b.Items = append(b.Items, item)
			}
		}
		if valid {
			out = append(out, b)
			i += m.Count
			continue
		}
		r.doc.warn("colleaf_stale_literal_ownership", "Stale literal list ownership ignored; actual native label/body table content is retained")
		bs[i].colleafLiteral = nil
		out = append(out, bs[i])
		i++
	}
	return out, nil
}

func colleafLiteralTable(block Block, item ListItem, index int) Block {
	var label []Block
	if item.Marker != nil {
		label = item.Marker.Blocks
	} else {
		automatic := Block{Kind: ListBlock, List: block.List, Numbering: block.Numbering, Items: []ListItem{{Blocks: []Block{{Kind: Paragraph, colleafAutoLabel: true}}}}}
		if block.List == "number" {
			automatic.Start = listStartPointer(listStart(block) + index)
		}
		label = []Block{automatic}
	}
	cells := []Cell{{Blocks: label}, {Blocks: item.Blocks}}
	if item.Blocks[0].Direction == "rtl" {
		cells[0], cells[1] = cells[1], cells[0]
	}
	return Block{Kind: Table, Rows: []Row{{Cells: cells}}}
}
func hasColleafLiteralItems(b Block) bool {
	for _, item := range b.Items {
		if item.Marker != nil {
			return true
		}
	}
	return false
}
