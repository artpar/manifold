package manifold

import (
	"fmt"
	"sort"
)

// SF has no row/cell control-token cursor. Retain each bookmark relationship at
// the corresponding logical boundary and disclose the native-position loss.
func (r *colleafBodyReader) indexStructuralCursors() error {
	r.cursorKinds = map[int]string{}
	r.cursorBookmarks = map[int][]colleafRangeRecord{}
	var walk func([]colleafStreamNode) error
	walk = func(nodes []colleafStreamNode) error {
		for _, n := range nodes {
			if err := r.ctx.Err(); err != nil {
				return err
			}
			switch n.kind {
			case "section":
				r.cursorKinds[n.end-1] = "section terminator"
			case "table":
				r.cursorKinds[n.end-1] = "table terminator"
			case "row":
				r.cursorKinds[n.start] = "row start"
				r.cursorKinds[n.end-1] = "row end"
			case "cell":
				r.cursorKinds[n.start] = "cell start"
				r.cursorKinds[n.end-1] = "cell end"
			case "paragraph":
				if n.end-1 == n.start+1 {
					token, err := r.index.text.exclusive(n.start, n.start+1)
					if err != nil {
						return err
					}
					if token == "\f" {
						r.cursorKinds[n.end-1] = "page-break paragraph terminator"
					}
				}
				if len(n.children) > 0 && n.children[len(n.children)-1].end == n.end-1 {
					r.cursorKinds[n.end-1] = "table anchoring paragraph terminator"
				}
				if a, ok := r.index.blocks[n.start]; ok && a.Type != nil && *a.Type == 0 && n.end-1 == n.start+1 {
					r.cursorKinds[n.end-1] = "image paragraph terminator"
				}
			}
			if err := walk(n.children); err != nil {
				return err
			}
		}
		return nil
	}
	return walk(r.index.sections)
}
func (r *colleafBodyReader) cursorBlocks(position int, boundary string, ordinary bool) ([]Block, error) {
	records := r.cursorBookmarks[position]
	delete(r.cursorBookmarks, position)
	if ordinary {
		first := sort.Search(len(r.bookmarks), func(i int) bool { return r.bookmarks[i].StartIndex >= position })
		for _, a := range r.bookmarks[first:] {
			if a.StartIndex != position {
				break
			}
			if !r.bookmarkUsed[a.ID] {
				records = append(records, a)
				r.bookmarkUsed[a.ID] = true
			}
		}
	}
	var blocks []Block
	for _, a := range records {
		p, err := readColleafBookmark(a)
		if err != nil {
			return nil, err
		}
		losses := &Document{}
		if err = colleafBookmarkLosses(losses, a); err != nil {
			return nil, err
		}
		for _, d := range losses.Diagnostics {
			r.cursorWarn(d.Code, d.Message)
		}
		kind := r.cursorKinds[position]
		if kind == "" {
			kind = "paragraph point"
		}
		location := fmt.Sprintf("UTF-16 offset %d (%s)", position, kind)
		if r.settings.NavigationScope != "" {
			location += fmt.Sprintf(" in story %q", r.settings.NavigationScope)
		}
		r.cursorWarn("colleaf_structural_cursor", fmt.Sprintf("Native bookmark at %s projects to %s; exact native control cursor identity is not retained", location, boundary))
		if p.Hint != nil {
			r.cursorWarn("colleaf_stale_bookmark_placement", "Native bookmark placement intent is normalized at its current structural boundary")
		}
		blocks = append(blocks, Block{Kind: AnchorPoint, Anchors: []string{p.BookmarkID}})
	}
	return blocks, nil
}

type colleafCellCursorProjection struct {
	origin        *tableCellPosition
	before, after []Block
}

func (r *colleafBodyReader) cellCursorProjection(row, cell colleafStreamNode, index, count int, origin *tableCellPosition, covered bool) (colleafCellCursorProjection, error) {
	out := colleafCellCursorProjection{origin: origin}
	where := fmt.Sprintf("logical table cell origin row %d column %d", origin.Row, origin.Column)
	if covered {
		where = "merged continuation's " + where
	}
	take := func(position int, edge string, ordinary bool) error {
		blocks, err := r.cursorBlocks(position, where+" "+edge, ordinary)
		if err != nil {
			return err
		}
		if edge == "start" {
			out.before = append(out.before, blocks...)
		} else {
			out.after = append(out.after, blocks...)
		}
		return nil
	}
	if index == 0 {
		if err := take(row.start, "start", false); err != nil {
			return out, err
		}
	}
	if err := take(cell.start, "start", false); err != nil {
		return out, err
	}
	if covered {
		if err := take(cell.start+1, "start", true); err != nil {
			return out, err
		}
		if err := take(cell.end-2, "end", true); err != nil {
			return out, err
		}
	}
	if err := take(cell.end-1, "end", false); err != nil {
		return out, err
	}
	if index == count-1 {
		if err := take(row.end-1, "end", false); err != nil {
			return out, err
		}
	}
	return out, nil
}

// Position-specific loss evidence can be numerous. Deduplicate this projection's
// warnings in constant time rather than scanning all prior cursor diagnostics.
func (r *colleafBodyReader) cursorWarn(code, message string) {
	if r.cursorWarningSet == nil {
		r.cursorWarningSet = map[string]bool{}
		for _, d := range r.doc.Diagnostics {
			r.cursorWarningSet[d.Code+"\x00"+d.Message] = true
		}
	}
	key := code + "\x00" + message
	if !r.cursorWarningSet[key] {
		r.cursorWarningSet[key] = true
		r.doc.Diagnostics = append(r.doc.Diagnostics, Diagnostic{Code: code, Message: message})
	}
}
