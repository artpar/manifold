package manydoc

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/url"
	"reflect"
	"sort"
)

type colleafBookmarkHint struct {
	Version      int    `json:"version"`
	Role         string `json:"role"`
	ParagraphID  string `json:"paragraphId,omitempty"`
	OwnerID      string `json:"ownerId,omitempty"`
	PointScope   string `json:"pointScope,omitempty"`
	PointOrder   int    `json:"pointOrder,omitempty"`
	PointIndex   int    `json:"pointIndex,omitempty"`
	PointCarrier string `json:"pointCarrier,omitempty"`
}
type colleafBookmarkProperties struct {
	FlowOrigin *colleafOrigin       `json:"manydocFlowOrigin,omitempty"`
	Origin     *colleafOrigin       `json:"manydocOrigin,omitempty"`
	BookmarkID string               `json:"bookmarkId,omitempty"`
	Hint       *colleafBookmarkHint `json:"manydocNavigation,omitempty"`
}

func readColleafBookmark(a colleafRangeRecord) (colleafBookmarkProperties, error) {
	var p colleafBookmarkProperties
	if len(a.Properties) > 0 {
		if err := validateTypedJSON(context.Background(), a.Properties, reflect.TypeFor[colleafBookmarkProperties]()); err != nil {
			return p, err
		}
	}
	if len(a.Properties) > 0 {
		if err := json.Unmarshal(a.Properties, &p); err != nil {
			return p, err
		}
	}
	if p.FlowOrigin != nil {
		if _, err := readColleafOriginProfile(sfAttrs(p.FlowOrigin), colleafFlowOriginProfile); err != nil {
			return p, err
		}
	}
	if p.Origin != nil {
		if _, err := readColleafOriginProfile(sfAttrs(p.Origin), colleafAnchorOriginProfile); err != nil {
			return p, err
		}
	}
	if p.BookmarkID == "" {
		p.BookmarkID = a.ID
	}
	if err := destinationID(p.BookmarkID); err != nil {
		return p, err
	}
	if p.Hint != nil {
		fields, err := colleafStyleObject(a.Properties)
		if err != nil {
			return p, err
		}
		if err = strictSFAttrs(fields["manydocNavigation"], p.Hint); err != nil {
			return p, err
		}
		validRole := p.Hint.Role == "run" || p.Hint.Role == "block" || p.Hint.Role == "table" || p.Hint.Role == "picture" || p.Hint.Role == "list" || p.Hint.Role == "break" || p.Hint.Role == "section" || p.Hint.Role == "point"
		if p.Hint.Version != 1 || !validRole {
			return p, fmt.Errorf("%w: native bookmark placement intent", ErrUnsupported)
		}
		structural := p.Hint.Role == "table" || p.Hint.Role == "picture" || p.Hint.Role == "list" || p.Hint.Role == "break" || p.Hint.Role == "section" || p.Hint.Role == "point"
		if structural && (p.Hint.OwnerID == "" || p.Hint.ParagraphID != "") || !structural && p.Hint.OwnerID != "" {
			return p, fmt.Errorf("invalid native bookmark owner binding")
		}
		if p.Hint.Role == "point" {
			if (p.Hint.PointScope != "section" && p.Hint.PointScope != "item") || p.Hint.PointOrder <= 0 || p.Hint.PointIndex < 0 {
				return p, fmt.Errorf("invalid standalone point binding")
			}
		} else if p.Hint.PointScope != "" || p.Hint.PointOrder != 0 || p.Hint.PointIndex != 0 || p.Hint.PointCarrier != "" {
			return p, fmt.Errorf("unexpected standalone point binding")
		}
	}
	return p, nil
}
func writeColleafBookmark(id string, offset int, pid, role string) colleafRangeRecord {
	sum := sha256.Sum256([]byte(id))
	return colleafRangeRecord{ID: fmt.Sprintf("manydoc-bookmark-%x", sum), Type: 3, StartIndex: offset, EndIndex: offset, Properties: sfAttrs(colleafBookmarkProperties{BookmarkID: id, Hint: &colleafBookmarkHint{Version: 1, Role: role, ParagraphID: pid}})}
}

type colleafLinkProperties struct {
	URL      string `json:"url"`
	Bookmark string `json:"bookmarkId"`
	Heading  string `json:"headingId"`
	Mode     string `json:"textStyleMode"`
}

func colleafNativeHeadingID(raw json.RawMessage) (string, error) {
	props, err := colleafStyleObject(raw)
	if err != nil {
		return "", err
	}
	value, ok := props["headingId"]
	if !ok {
		return "", nil
	}
	var id *string
	if err = json.Unmarshal(value, &id); err != nil {
		return "", err
	}
	if id == nil {
		return "", nil
	}
	return *id, nil
}
func rebindColleafBookmark(a *colleafRangeRecord, names map[string]string) error {
	if a.Type != 3 {
		return nil
	}
	p, err := readColleafBookmark(*a)
	if err != nil {
		return err
	}
	if p.Hint != nil {
		if name, ok := names[p.Hint.ParagraphID]; ok {
			p.Hint.ParagraphID = name
			a.Properties = sfAttrs(p)
		}
	}
	return nil
}
func colleafParagraphStarts(index *colleafBodyIndex) map[int]int {
	starts := map[int]int{}
	var walk func([]colleafStreamNode)
	walk = func(nodes []colleafStreamNode) {
		for _, n := range nodes {
			if n.kind == "paragraph" {
				starts[n.end-1] = n.start
			}
			walk(n.children)
		}
	}
	walk(index.sections)
	return starts
}
func colleafHeadingID(scope, id string) string {
	if scope == "" {
		scope = "main"
	}
	sum := sha256.Sum256([]byte(scope + "\x00" + id))
	return fmt.Sprintf("univer-heading-%x", sum)
}
func colleafInternalHref(href, id string) bool {
	u, err := url.Parse(href)
	return err == nil && u.Scheme == "" && u.Host == "" && u.Path == "" && u.RawQuery == "" && u.Opaque == "" && u.Fragment == id
}
func colleafBookmarkAliases(body colleafBody) (map[string]string, error) {
	aliases := map[string]string{}
	for _, a := range body.CustomRanges {
		if a.Type != 3 {
			continue
		}
		p, err := readColleafBookmark(a)
		if err != nil {
			return nil, err
		}
		for _, name := range []string{p.BookmarkID, a.ID} {
			if previous, ok := aliases[name]; ok && previous != p.BookmarkID {
				return nil, fmt.Errorf("ambiguous native bookmark alias")
			}
			aliases[name] = p.BookmarkID
		}
	}
	return aliases, nil
}

// Avoid accidental range-ID aliases of real or explicitly unresolved SF names.
// The native click handler tests both bookmarkId and rangeId in the same lookup.
func scopeColleafBookmarkIDs(ctx context.Context, out *colleafNoteSnapshot, d *Document) error {
	catalog, err := catalogDestinations(d, true)
	if err != nil {
		return err
	}
	reserved := map[string]bool{}
	for id := range catalog.anchors {
		reserved[id] = true
	}
	for id := range catalog.links {
		reserved[id] = true
	}
	for id := range catalog.missing {
		reserved[id] = true
	}
	bodies := []*colleafBody{&out.Body}
	type storySlot struct {
		id     string
		header bool
		story  colleafRunningStory
	}
	var slots []storySlot
	for _, header := range []bool{true, false} {
		source := out.Footers
		if header {
			source = out.Headers
		}
		ids := make([]string, 0, len(source))
		for id := range source {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			slots = append(slots, storySlot{id, header, source[id]})
		}
	}
	for i := range slots {
		bodies = append(bodies, &slots[i].story.Body)
	}
	noteIDs := make([]string, 0, len(out.Notes))
	for id := range out.Notes {
		noteIDs = append(noteIDs, id)
	}
	sort.Strings(noteIDs)
	notes := make([]colleafNativeNote, len(noteIDs))
	for i, id := range noteIDs {
		notes[i] = out.Notes[id]
		bodies = append(bodies, &notes[i].Body)
	}
	for _, body := range bodies {
		for _, a := range body.CustomRanges {
			if a.Type != 3 {
				reserved[a.ID] = true
			}
		}
	}
	for _, body := range bodies {
		for i := range body.CustomRanges {
			if err := ctx.Err(); err != nil {
				return err
			}
			a := &body.CustomRanges[i]
			if a.Type != 3 {
				continue
			}
			p, err := readColleafBookmark(*a)
			if err != nil {
				return err
			}
			for salt := 0; ; salt++ {
				sum := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%d", p.BookmarkID, salt)))
				id := fmt.Sprintf("manydoc-bookmark-%x", sum)
				if !reserved[id] {
					a.ID = id
					reserved[id] = true
					break
				}
			}
		}
	}
	for _, slot := range slots {
		if slot.header {
			out.Headers[slot.id] = slot.story
		} else {
			out.Footers[slot.id] = slot.story
		}
	}
	for i, id := range noteIDs {
		out.Notes[id] = notes[i]
	}
	return nil
}

// Validate that every source point survived rich-container interpretation. Recover
// explicit missing targets from the final tree, rather than silently dropping or
// inventing a destination. Heading identities use a separate scoped namespace.
func finishColleafNavigation(ctx context.Context, input colleafNoteSnapshot, d *Document, declared []string) error {
	c, err := catalogDestinations(d, false)
	if err != nil {
		return err
	}
	expected := map[string]bool{}
	heading := false
	check := func(body colleafBody, scope string) error {
		for _, a := range body.CustomRanges {
			if err := ctx.Err(); err != nil {
				return err
			}
			if a.Type != 3 {
				continue
			}
			p, err := readColleafBookmark(a)
			if err != nil {
				return err
			}
			if expected[p.BookmarkID] {
				return fmt.Errorf("duplicate native bookmark identity")
			}
			expected[p.BookmarkID] = true
			if !c.anchors[p.BookmarkID] {
				return fmt.Errorf("%w: native bookmark at an unrepresented structural cursor", ErrUnsupported)
			}
		}
		for _, p := range body.Paragraphs {
			nativeID, err := colleafNativeHeadingID(p.Style)
			if err != nil {
				return err
			}
			if nativeID != "" {
				id := colleafHeadingID(scope, nativeID)
				if expected[id] {
					return fmt.Errorf("duplicate native heading identity")
				}
				expected[id] = true
				if !c.anchors[id] {
					return fmt.Errorf("%w: native heading at an unrepresented structural cursor", ErrUnsupported)
				}
				heading = true
			}
		}

		return nil
	}
	if err = check(input.Body, ""); err != nil {
		return err
	}
	mainPoints := map[string]bool{}
	for id := range expected {
		mainPoints[id] = true
	}
	for id, s := range input.Headers {
		if err = check(s.Body, "header:"+id); err != nil {
			return err
		}
	}
	for id, s := range input.Footers {
		if err = check(s.Body, "footer:"+id); err != nil {
			return err
		}
	}
	for id, n := range input.Notes {
		if err = check(n.Body, "note:"+id); err != nil {
			return err
		}
	}
	if heading {
		d.warn("colleaf_heading_navigation", "Native heading identities become scoped SF navigation points; current internal targets are retained")
	}
	known := map[string]bool{}
	for _, id := range declared {
		if err = destinationID(id); err != nil {
			return err
		}
		if known[id] {
			return fmt.Errorf("duplicate native unresolved-target intent")
		}
		known[id] = true
	}
	missing := map[string]bool{}
	for id := range known {
		if !c.anchors[id] {
			missing[id] = true
		}
	}
	nonmain, newMissing := false, false
	for id := range c.links {
		nonmain = nonmain || (c.anchors[id] && !mainPoints[id])
		if !c.anchors[id] {
			missing[id] = true
			newMissing = newMissing || !known[id]
		}
	}
	if nonmain {
		d.warn("colleaf_nonmain_navigation", "Native internal lookup targets the main story only; nonmain point identities and link data are retained in SF")
	}
	if newMissing {
		d.warn("colleaf_unresolved_navigation", "Native internal link has no current target; its explicit unresolved identity is retained")
	}

	d.UnresolvedDestinations = nil
	for id := range missing {
		d.UnresolvedDestinations = append(d.UnresolvedDestinations, id)
	}
	sort.Strings(d.UnresolvedDestinations)
	return nil
}

// Native container points use their real token and source identity, never a
// synthetic paragraph or cached block. Current positions remain authoritative.
func (w *colleafBodyWriter) containerAnchors(ids []string, offset int, role, owner string) error {
	for _, id := range ids {
		if err := destinationID(id); err != nil {
			return err
		}
		a := writeColleafBookmark(id, offset, "", role)
		a.Properties = sfAttrs(colleafBookmarkProperties{BookmarkID: id, Hint: &colleafBookmarkHint{Version: 1, Role: role, OwnerID: owner}})
		w.body.CustomRanges = append(w.body.CustomRanges, a)
	}
	return nil
}
func colleafBookmarkLosses(doc *Document, a colleafRangeRecord) error {
	fields, err := colleafStyleObject(a.Properties)
	if err != nil {
		return err
	}
	for name := range fields {
		if name != "bookmarkId" && name != "manydocNavigation" && name != "manydocOrigin" && name != "manydocFlowOrigin" {
			doc.warn("colleaf_bookmark_metadata", "Native bookmark metadata outside identity and placement is not represented in SF")
		}
	}
	if a.StartIndex != a.EndIndex {
		doc.warn("colleaf_bookmark_span", "Native bookmark range editing extent becomes an SF point at its current start")
	}
	return nil
}
func (r *colleafBodyReader) containerBookmark(a colleafRangeRecord) (bool, error) {
	role, owner := "", ""
	if table, ok := r.index.tables[a.StartIndex]; ok {
		role, owner = "table", table.ID
	} else if atom, ok := r.index.blocks[a.StartIndex]; ok && atom.Type != nil && *atom.Type == 0 {
		role, owner = "picture", atom.ID
	}
	if role == "" {
		token, err := r.index.text.exclusive(a.StartIndex, a.StartIndex+1)
		if err == nil && token == "\f" {
			role, owner = "break", "page-break"
		}
	}
	if role == "" {
		return false, nil
	}
	p, err := readColleafBookmark(a)
	if err != nil {
		return false, err
	}
	if err = colleafBookmarkLosses(r.doc, a); err != nil {
		return false, err
	}
	if p.Hint != nil && p.Hint.Role != "point" && (p.Hint.Role != role || p.Hint.OwnerID != owner) {
		r.doc.warn("colleaf_stale_bookmark_placement", "Edited native bookmark position binds to its current container rather than stale placement intent")
	}
	r.containerPoints[a.StartIndex] = append(r.containerPoints[a.StartIndex], p.BookmarkID)
	return true, nil
}

// A native list has no token of its own. Restore container ownership only when
// the current list starts at the point's current first paragraph; otherwise keep
// the real inline point and disclose the lost ownership.
func (r *colleafBodyReader) promoteListPoints(block *Block, firstID string, position int) {
	if len(block.Items) == 0 || len(block.Items[0].Blocks) == 0 {
		return
	}
	first := 0
	for first < len(block.Items[0].Blocks) && block.Items[0].Blocks[first].Kind == AnchorPoint {
		first++
	}
	if first == len(block.Items[0].Blocks) {
		return
	}
	p := &block.Items[0].Blocks[first]
	runs := make([]Run, 0, len(p.Runs))
	for _, run := range p.Runs {
		if owner, ok := r.listPointOwners[run.Anchor]; ok && owner == firstID && r.listPointPositions[run.Anchor] == r.paragraphStarts[position] {
			block.Anchors = append(block.Anchors, run.Anchor)
			r.promotedListPoints[run.Anchor] = true
		} else {
			runs = append(runs, run)
		}
	}
	if len(runs) == 0 {
		p.Runs = nil
	} else {
		p.Runs = runs
	}
}
func (r *colleafBodyReader) finishListPoints() {
	for id := range r.listPointOwners {
		if !r.promotedListPoints[id] {
			r.doc.warn("colleaf_stale_bookmark_placement", "Native list point retains its current inline position because current list ownership differs")
			return
		}
	}
}
