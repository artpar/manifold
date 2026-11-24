package manydoc

import (
	"fmt"
	"reflect"
	"sort"
)

func (w *colleafBodyWriter) pointScopeID() string {
	w.pointScopeSerial++
	return fmt.Sprintf("manydoc-point-scope-%d", w.pointScopeSerial)
}
func (w *colleafBodyWriter) pointFlow(blocks []Block, depth int, scope, owner string) error {
	oldScope, oldOwner := w.pointScope, w.pointOwner
	w.pointScope, w.pointOwner = scope, owner
	defer func() { w.pointScope, w.pointOwner = oldScope, oldOwner }()
	return w.flow(blocks, depth)
}
func (w *colleafBodyWriter) standalonePoint(b Block) error {
	copy := b
	copy.Anchors = nil
	copy.Source = nil
	if !reflect.DeepEqual(copy, Block{Kind: AnchorPoint}) || len(b.Anchors) == 0 {
		return fmt.Errorf("invalid standalone native point")
	}
	origin, err := newColleafFlowOrigin(b)
	if err != nil {
		return err
	}
	w.pointSerial++
	for i, id := range b.Anchors {
		if err := destinationID(id); err != nil {
			return err
		}
		a := writeColleafBookmark(id, w.offset, "", "point")
		a.Properties = sfAttrs(colleafBookmarkProperties{BookmarkID: id, Hint: &colleafBookmarkHint{Version: 1, Role: "point", OwnerID: w.pointOwner, PointScope: w.pointScope, PointOrder: w.pointSerial, PointIndex: i}})
		if i == 0 && origin != nil {
			props, err := readColleafBookmark(a)
			if err != nil {
				return err
			}
			props.FlowOrigin = origin
			a.Properties = sfAttrs(props)
		}
		w.body.CustomRanges = append(w.body.CustomRanges, a)
	}
	return nil
}
func rebindColleafPointScope(body *colleafBody, from, to string, first int) error {
	for i := first; i < len(body.CustomRanges); i++ {
		a := &body.CustomRanges[i]
		if a.Type != 3 {
			continue
		}
		p, err := readColleafBookmark(*a)
		if err != nil {
			return err
		}
		if p.Hint != nil && p.Hint.Role == "point" && p.Hint.OwnerID == from {
			p.Hint.OwnerID = to
			a.Properties = sfAttrs(p)
		}
	}
	return nil
}

type colleafPointSlot struct {
	scope, owner           string
	position, order, index int
}
type colleafStandalonePoint struct {
	origin                    *colleafOrigin
	id, scope, owner, carrier string
	position, order, index    int
}

func (r *colleafBodyReader) registerStandalone(a colleafRangeRecord, p colleafBookmarkProperties) error {
	h := p.Hint
	point := colleafStandalonePoint{origin: p.FlowOrigin, id: p.BookmarkID, scope: h.PointScope, owner: h.OwnerID, carrier: h.PointCarrier, position: a.StartIndex, order: h.PointOrder, index: h.PointIndex}
	key := colleafPointSlot{point.scope, point.owner, point.position, point.order, point.index}
	if r.pointSlots[key] {
		return fmt.Errorf("duplicate standalone point order slot")
	}
	r.pointSlots[key] = true
	r.standalone = append(r.standalone, point)
	r.standaloneIDs[p.BookmarkID] = true
	if h.PointCarrier != "" {
		r.pointCarriers[h.PointCarrier] = true
	}
	return colleafBookmarkLosses(r.doc, a)
}
func (r *colleafBodyReader) standaloneBlocks(scope, owner string, position int) []Block {
	var points []colleafStandalonePoint
	for _, p := range r.pointsAt[position] {
		if p.scope == scope && p.owner == owner {
			points = append(points, p)
		}
	}
	sort.SliceStable(points, func(i, j int) bool {
		if points[i].order != points[j].order {
			return points[i].order < points[j].order
		}
		return points[i].index < points[j].index
	})
	var blocks []Block
	last := -1
	for _, p := range points {
		if p.order != last {
			blocks = append(blocks, Block{Kind: AnchorPoint})
			last = p.order
		}
		n := len(blocks) - 1
		blocks[n].Anchors = append(blocks[n].Anchors, p.id)
		if blocks[n].colleafFlowOrigin == nil {
			blocks[n].colleafFlowOrigin = p.origin
		}
	}
	return blocks
}

// Clean carriers only after actual list grouping has settled. Tentative grouping
// can be discarded; only point blocks in the final tree authorize carrier removal.
func (r *colleafBodyReader) finishStandalone(blocks []Block) {
	promoted := map[string]bool{}
	var walk func([]Block, func(*Block))
	walk = func(bs []Block, f func(*Block)) {
		for i := range bs {
			b := &bs[i]
			f(b)
			if b.Section != nil {
				walk(b.Section.Blocks, f)
			}
			for j := range b.Items {
				if b.Items[j].Marker != nil {
					walk(b.Items[j].Marker.Blocks, f)
				}
				walk(b.Items[j].Blocks, f)
			}
			for j := range b.Rows {
				for k := range b.Rows[j].Cells {
					walk(b.Rows[j].Cells[k].Blocks, f)
				}
			}
		}
	}
	walk(blocks, func(b *Block) {
		if b.Kind == AnchorPoint {
			for _, id := range b.Anchors {
				if r.standaloneIDs[id] {
					promoted[id] = true
				}
			}
		}
	})
	walk(blocks, func(b *Block) {
		if b.Kind != AnchorPoint {
			var ids []string
			for _, id := range b.Anchors {
				if !promoted[id] {
					ids = append(ids, id)
				}
			}
			b.Anchors = ids
		}
		if b.Runs != nil {
			var runs []Run
			for _, run := range b.Runs {
				if run.Anchor == "" || !promoted[run.Anchor] {
					runs = append(runs, run)
				}
			}
			b.Runs = runs
		}
	})
	for _, p := range r.standalone {
		if !promoted[p.id] {
			r.doc.warn("colleaf_stale_point_ownership", "Standalone point retains its current native location because its owning flow or boundary changed")
			break
		}
	}
}
func (r *colleafBodyReader) emptyPointCarrier(p colleafParagraphRecord, start, end int, owner string, bs []Block) bool {
	if !r.pointCarriers[p.ID] {
		return false
	}
	bound := false
	for _, point := range r.pointsAt[start] {
		bound = bound || point.carrier == p.ID && point.scope == "section" && point.owner == owner
	}
	valid := bound && start == end && len(p.Style) == 0 && p.StyleID == "" && len(p.Bullet) == 0 && len(bs) == 1
	if valid {
		copy := bs[0]
		for _, run := range copy.Runs {
			if !r.standaloneIDs[run.Anchor] {
				valid = false
			}
		}
		copy.Runs = nil
		valid = valid && reflect.DeepEqual(copy, Block{Kind: Paragraph})
	}
	if !valid {
		r.doc.warn("colleaf_stale_point_carrier", "Edited native point carrier is retained as current visible content and presentation")
	}
	return valid
}

func colleafStandaloneBlockIDs(blocks []Block) map[string]bool {
	found := map[string]bool{}
	var walk func([]Block)
	walk = func(bs []Block) {
		for _, b := range bs {
			if b.Kind == AnchorPoint {
				for _, id := range b.Anchors {
					found[id] = true
				}
			}
			if b.Section != nil {
				walk(b.Section.Blocks)
			}
			for _, item := range b.Items {
				if item.Marker != nil {
					walk(item.Marker.Blocks)
				}
				walk(item.Blocks)
			}
			for _, row := range b.Rows {
				for _, cell := range row.Cells {
					walk(cell.Blocks)
				}
			}
		}
	}
	walk(blocks)
	return found
}
