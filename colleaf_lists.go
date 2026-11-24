package manydoc

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strconv"
)

const colleafListResource = "manydoc.list-ownership.v1"

type colleafBullet struct {
	Type  string          `json:"listType"`
	ID    string          `json:"listId"`
	Level int             `json:"nestingLevel"`
	Start *int            `json:"startNumber,omitempty"`
	Image json.RawMessage `json:"image,omitempty"`
	Style json.RawMessage `json:"textStyle,omitempty"`
}
type colleafListLevel struct {
	Format     string          `json:"glyphFormat"`
	Glyph      *int            `json:"glyphType,omitempty"`
	Symbol     string          `json:"glyphSymbol,omitempty"`
	Start      int             `json:"startNumber"`
	Align      int             `json:"bulletAlignment"`
	Legal      bool            `json:"isLegal,omitempty"`
	Properties json.RawMessage `json:"paragraphProperties,omitempty"`
	Style      json.RawMessage `json:"textStyle,omitempty"`
}
type colleafNativeList struct {
	Type   string             `json:"listType"`
	Levels []colleafListLevel `json:"nestingLevel"`
}
type colleafListItemBinding struct {
	Type  string `json:"type,omitempty"`
	First string `json:"first"`
	Last  string `json:"last"`
}
type colleafListBinding struct {
	Projections      map[string]colleafNumberProjection `json:"projections,omitempty"`
	Type             string                             `json:"type"`
	Group            string                             `json:"group"`
	First            string                             `json:"first"`
	Last             string                             `json:"last"`
	Items            []colleafListItemBinding           `json:"items"`
	Implicit         bool                               `json:"implicitNumbering,omitempty"`
	OriginalTemplate *string                            `json:"originalTemplate,omitempty"`
	Independent      bool                               `json:"independent,omitempty"`
}
type colleafListOwnership struct {
	Stories map[string][]colleafListBinding `json:"stories"`
}
type colleafFlowBlock struct {
	After    []Block
	Block    Block
	ID       string
	Bullet   json.RawMessage
	Position int
	Start    int
}
type colleafListParagraph struct {
	Bullet          colleafBullet
	Kind            string
	Value           int
	Numbering       *ListNumbering
	ActiveAncestors []int
	Loss            bool
	PolicyLoss      bool
	StaleProjection bool
}

var colleafNumberFormats = []string{"decimal", "decimalZero", "upperLetter", "lowerLetter", "upperRoman", "lowerRoman"}

func colleafNumberGlyph(format string) (int, error) {
	for i, f := range colleafNumberFormats {
		if f == format {
			return i + 2, nil
		}
	}
	return 0, fmt.Errorf("%w: native numbering format%s", ErrUnsupported, format)
}

func (w *colleafBodyWriter) list(block Block, depth int) error {
	for _, item := range block.Items {
		if item.Marker != nil {
			return w.literalList(block, depth)
		}
	}
	if depth > 64 || len(block.Items) == 0 || (block.List != "number" && block.List != "bullet") {
		return fmt.Errorf("invalid native list")
	}
	if err := validateNumbering(block.Numbering); err != nil {
		return err
	}

	level := w.listDepth
	if block.Numbering != nil {
		level = block.Numbering.Level
	}
	if level > 8 {
		return fmt.Errorf("%w: native lists support nine numbering levels", ErrUnsupported)
	}
	oldDepth, oldGroup := w.listDepth, w.listGroup
	defer func() { w.listDepth, w.listGroup = oldDepth, oldGroup }()
	group := oldGroup
	independent := block.Numbering != nil && block.Numbering.Independent
	if group == "" || independent {
		group = w.id("list-group")
	}
	w.listGroup = group
	w.listDepth = level + 1
	typeID := w.id("list-type")
	source := colleafNativeList{Type: typeID}
	for l := 0; l <= level; l++ {
		format := "decimal"
		start := 0
		if block.Numbering != nil {
			format = block.Numbering.Formats[l]
			if l < level {
				start = block.Numbering.Ancestors[l] - 1
			}
		}
		glyph, err := colleafNumberGlyph(format)
		if err != nil {
			return err
		}
		entry := colleafListLevel{Format: "%" + strconv.Itoa(l+1) + ".", Glyph: &glyph, Start: start, Align: 1}
		if l == level {
			entry.Start = listStart(block) - 1
			if block.List == "bullet" {
				g := 0
				entry.Glyph = &g
				entry.Symbol = "•"
				entry.Format = "%" + strconv.Itoa(l+1)
			} else if block.Numbering != nil {
				entry.Format = block.Numbering.Template
				entry.Legal = block.Numbering.Legal
			}
		}
		source.Levels = append(source.Levels, entry)
	}
	if block.List == "number" && block.Numbering != nil {
		format := projectColleafListTemplate(block.Numbering.Template, level, &source)
		source.Levels[level].Format = format
	}
	unprojected := source
	normalValue := listStart(block)
	if normalValue == 0 {
		normalValue = 1
	}
	source, normalProjection := projectColleafNumbering(unprojected, block.Numbering, normalValue)
	firstSource, firstProjection := projectColleafNumbering(unprojected, block.Numbering, listStart(block))
	firstType := typeID
	if !reflect.DeepEqual(firstSource.Levels, source.Levels) {
		firstType = w.id("list-zero-type")
		firstSource.Type = firstType
		w.lists[firstType] = sfAttrs(firstSource)
	}
	w.lists[typeID] = sfAttrs(source)
	binding := colleafListBinding{Type: typeID, Group: group, Implicit: block.Numbering == nil, Independent: independent}
	if normalProjection != nil || firstProjection != nil {
		binding.Projections = map[string]colleafNumberProjection{}
	}
	if normalProjection != nil {
		binding.Projections[typeID] = *normalProjection
	}
	if firstProjection != nil {
		binding.Projections[firstType] = *firstProjection
	}
	if block.Numbering != nil && source.Levels[level].Format != block.Numbering.Template {
		original := block.Numbering.Template
		binding.OriginalTemplate = &original
	}
	for i, item := range block.Items {
		if len(item.Blocks) == 0 || (item.Blocks[0].Kind != Paragraph && item.Blocks[0].Kind != Heading) {
			return fmt.Errorf("%w: native list item requires a leading paragraph without unmapped provenance", ErrUnsupported)
		}
		pointOffset := w.offset
		if err := w.paragraph(item.Blocks[0]); err != nil {
			return err
		}
		first := len(w.body.Paragraphs) - 1
		bullet := colleafBullet{Type: typeID, ID: group, Level: level}
		if i == 0 {
			bullet.Type = firstType
			start := listStart(block) - 1
			bullet.Start = &start
		}
		w.body.Paragraphs[first].Bullet = sfAttrs(bullet)

		props, err := colleafStyleObject(w.body.Paragraphs[first].Style)
		if err != nil {
			return err
		}
		if i == 0 && block.Source != nil {
			hash, err := colleafListFingerprint(block)
			if err != nil {
				return err
			}
			origin, err := newColleafSourceOrigin(block.Source, hash, colleafListOriginProfile)
			if err != nil {
				return err
			}
			props["manydocListOrigin"] = sfAttrs(origin)
		}
		if item.Source != nil {
			hash, err := colleafItemFingerprint(block, i)
			if err != nil {
				return err
			}
			origin, err := newColleafSourceOrigin(item.Source, hash, colleafItemOriginProfile)
			if err != nil {
				return err
			}
			props["manydocItemOrigin"] = sfAttrs(origin)
		}
		if len(props) > 0 {
			w.body.Paragraphs[first].Style = sfAttrs(props)
		}
		firstID := w.body.Paragraphs[first].ID
		if i == 0 {
			if err := w.containerAnchors(block.Anchors, pointOffset, "list", firstID); err != nil {
				return err
			}
		}
		if err := w.pointFlow(item.Blocks[1:], depth, "item", firstID); err != nil {
			return err
		}
		lastID := w.body.Paragraphs[len(w.body.Paragraphs)-1].ID
		itemBinding := colleafListItemBinding{First: firstID, Last: lastID}
		if bullet.Type != typeID {
			itemBinding.Type = bullet.Type
		}
		binding.Items = append(binding.Items, itemBinding)
	}
	binding.First = binding.Items[0].First
	binding.Last = binding.Items[len(binding.Items)-1].Last
	w.listBindings = append(w.listBindings, binding)
	return nil
}

func colleafListParagraphs(ctx context.Context, body colleafBody, lists map[string]json.RawMessage, bindings map[string]colleafListBinding) (map[int]colleafListParagraph, error) {
	result := map[int]colleafListParagraph{}
	sources := map[string]colleafNativeList{}
	for key, raw := range lists {
		var source colleafNativeList
		if err := json.Unmarshal(raw, &source); err != nil {
			return nil, err
		}
		if key == "" || source.Type != key || len(source.Levels) == 0 || len(source.Levels) > colleafMaxListEntries {
			return nil, fmt.Errorf("invalid native list source")
		}
		sources[key] = source
	}
	checkedBindings := map[string]bool{}
	for _, binding := range bindings {
		if checkedBindings[binding.Type] {
			continue
		}
		checkedBindings[binding.Type] = true
		for name, p := range binding.Projections {
			source, ok := sources[name]
			if !ok {
				return nil, fmt.Errorf("native numbering intent lacks its source type")
			}
			if err := validateColleafNumberProjection(p, min(8, len(source.Levels)-1)); err != nil {
				return nil, err
			}
		}
	}
	var paragraphs []colleafParagraphRecord
	for _, p := range body.Paragraphs {
		if len(p.Bullet) > 0 && string(p.Bullet) != "null" {
			paragraphs = append(paragraphs, p)
		}
	}
	sort.Slice(paragraphs, func(i, j int) bool { return paragraphs[i].StartIndex < paragraphs[j].StartIndex })
	type counters struct {
		values [9]int
		seen   [9]bool
	}
	groups := map[string]*counters{}
	for _, p := range paragraphs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if len(p.Bullet) == 0 || string(p.Bullet) == "null" {
			continue
		}
		var bullet colleafBullet
		if err := json.Unmarshal(p.Bullet, &bullet); err != nil {
			return nil, err
		}
		source, ok := sources[bullet.Type]
		if !ok || bullet.ID == "" || bullet.Level < 0 || bullet.Level > 8 || bullet.Level >= len(source.Levels) {
			return nil, fmt.Errorf("native bullet lacks valid source/group/level")
		}
		level := source.Levels[bullet.Level]
		start := level.Start
		if bullet.Start != nil {
			start = *bullet.Start
		}
		if start < -1 || start > 2147483646 {
			return nil, fmt.Errorf("invalid native list start")
		}
		state := groups[bullet.ID]
		if state == nil {
			state = &counters{}
			groups[bullet.ID] = state
		}
		value := start + 1
		if bullet.Start == nil && state.seen[bullet.Level] {
			value = state.values[bullet.Level] + 1
		}
		if value > 2147483647 {
			return nil, fmt.Errorf("native list counter overflow")
		}
		info := colleafListParagraph{Bullet: bullet, Value: value, Kind: "number"}
		if len(bullet.Image) > 0 {
			return nil, fmt.Errorf("%w: native picture bullet requires asset mapping", ErrUnsupported)
		}
		if level.Glyph == nil || *level.Glyph == 0 {
			info.Kind = "bullet"
			info.Loss = level.Symbol != "•" && level.Symbol != "●"
		} else if *level.Glyph < 2 || *level.Glyph > 7 {
			return nil, fmt.Errorf("%w: native glyph type%d", ErrUnsupported, *level.Glyph)
		}
		if info.Kind == "number" {
			template, err := readColleafListTemplate(source, bullet.Level, bindings[bullet.Type])
			if err != nil {
				return nil, err
			}

			n := &ListNumbering{Level: bullet.Level, Template: template, Legal: level.Legal}
			for l := 0; l <= bullet.Level; l++ {
				spec := source.Levels[l]
				if spec.Glyph == nil || *spec.Glyph < 2 || *spec.Glyph > 7 {
					return nil, fmt.Errorf("%w: native compound numbering format", ErrUnsupported)
				}
				n.Formats = append(n.Formats, colleafNumberFormats[*spec.Glyph-2])
				if l < bullet.Level {
					if spec.Start < -1 || spec.Start > 2147483646 {
						return nil, fmt.Errorf("invalid native ancestor start")
					}
					n.Ancestors = append(n.Ancestors, spec.Start+1)
					v := spec.Start + 1
					if state.seen[l] {
						v = state.values[l]
					}
					info.ActiveAncestors = append(info.ActiveAncestors, v)
				}
			}
			if err := validateNumbering(n); err != nil {
				return nil, err
			}
			binding := bindings[bullet.Type]
			restored := false
			if p, ok := binding.Projections[bullet.Type]; ok {
				if err := validateColleafNumberProjection(p, bullet.Level); err != nil {
					return nil, err
				}
				restored = restoreColleafNumberProjection(n, p, value, info.ActiveAncestors, source)
				info.StaleProjection = !restored
			}
			if !restored {
				var err error
				info.PolicyLoss, err = normalizeColleafNativeNumbering(n, value, info.ActiveAncestors)
				if err != nil {
					return nil, err
				}
			}

			info.Numbering = n
		}
		info.Loss = info.Loss || len(level.Style) > 0 || len(level.Properties) > 0 || len(bullet.Style) > 0 || level.Align != 1
		result[p.StartIndex] = info
		state.values[bullet.Level] = value
		state.seen[bullet.Level] = true
		for l := bullet.Level + 1; l < 9; l++ {
			state.seen[l] = false
		}
	}
	return result, nil
}

func nativeListBlock(info colleafListParagraph, binding *colleafListBinding) Block {
	block := Block{Kind: ListBlock, List: info.Kind}
	if info.Kind == "number" {
		block.Start = listStartPointer(info.Value)
		n := *info.Numbering
		n.Formats = append([]string(nil), n.Formats...)
		n.Ancestors = append([]int(nil), n.Ancestors...)
		if binding == nil {
			n.Ancestors = append([]int(nil), info.ActiveAncestors...)
		} else {
			n.Independent = binding.Independent
		}
		implicit := binding != nil && binding.Implicit && n.Template == "%"+strconv.Itoa(n.Level+1)+"." && n.Formats[n.Level] == "decimal" && !n.Legal

		if !implicit {
			block.Numbering = &n
		}
	}
	return block
}
func (r *colleafBodyReader) groupLists(flow []colleafFlowBlock, depth int, sectionID string, lower, upper int) ([]Block, error) {
	ids := map[string]int{}
	duplicates := map[string]bool{}
	for i, f := range flow {
		if _, ok := ids[f.ID]; ok {
			duplicates[f.ID] = true
		}
		ids[f.ID] = i
	}
	bindings := r.listBindings
	var consume func(int, int, string) (Block, int, error)
	var group func(int, int, int, string, string, string, int, int) ([]Block, error)
	group = func(start, end, depth int, parent, scope, owner string, lower, upper int) ([]Block, error) {
		if depth > 64 {
			return nil, fmt.Errorf("native list ownership nesting exceeds64")
		}
		var out []Block
		seen := map[int]bool{}
		if start == end {
			out = append(out, r.standaloneBlocks(scope, owner, lower)...)
			seen[lower] = true
		}
		for i := start; i < end; {
			pos := flow[i].Start
			if !seen[pos] {
				out = append(out, r.standaloneBlocks(scope, owner, pos)...)
				seen[pos] = true
			}
			info, ok := r.listParagraphs[flow[i].Position]
			if !ok || len(flow[i].Bullet) == 0 {
				out = append(out, flow[i].Block)
				out = append(out, flow[i].After...)
				i++
				continue
			}
			block, next, err := consume(i, depth, parent)
			if err != nil {
				return nil, err
			}
			if next > end {
				return nil, fmt.Errorf("native list ownership crosses its item")
			}
			out = append(out, block)
			i = next
			if info.Loss {
				r.doc.warn("colleaf_list_presentation", "Native marker presentation is normalized; supported numbering and item text are retained")
			}
		}
		if !seen[upper] {
			out = append(out, r.standaloneBlocks(scope, owner, upper)...)
		}
		return out, nil
	}
	consume = func(start, depth int, parent string) (Block, int, error) {
		if depth > 64 {
			return Block{}, 0, fmt.Errorf("native list ownership nesting exceeds64")
		}
		info := r.listParagraphs[flow[start].Position]
		if flow[start].Block.Kind != Paragraph && flow[start].Block.Kind != Heading {
			return Block{}, 0, fmt.Errorf("%w: native table/bullet paragraph needs ownership mapping", ErrUnsupported)
		}
		if binding, ok := bindings[info.Bullet.Type]; ok && binding.First == flow[start].ID {
			end, valid := ids[binding.Last]
			valid = valid && !duplicates[binding.First] && !duplicates[binding.Last] && end >= start && binding.Group == info.Bullet.ID && len(binding.Items) > 0
			if binding.Independent && parent != "" && parent == binding.Group {
				valid = false
			}
			cursor := start
			type interval struct{ start, end int }
			var items []interval
			for _, item := range binding.Items {
				a, oka := ids[item.First]
				b, okb := ids[item.Last]
				valid = valid && oka && okb && !duplicates[item.First] && !duplicates[item.Last] && a == cursor && b >= a && b <= end
				if !valid {
					break
				}
				expectedType := binding.Type
				if item.Type != "" {
					expectedType = item.Type
				}
				first, exists := r.listParagraphs[flow[a].Position]
				valid = exists && first.Bullet.Type == expectedType && first.Bullet.ID == info.Bullet.ID && first.Bullet.Level == info.Bullet.Level
				for k := a + 1; k <= b && valid; k++ {
					if nested, ok := r.listParagraphs[flow[k].Position]; ok && len(flow[k].Bullet) > 0 && nested.Bullet.Level <= info.Bullet.Level {
						valid = false
					}
				}
				items = append(items, interval{a, b})
				cursor = b + 1
			}
			valid = valid && cursor == end+1 && binding.Items[0].First == binding.First && binding.Items[len(binding.Items)-1].Last == binding.Last
			if valid {
				block := nativeListBlock(info, &binding)
				for itemIndex, item := range items {
					// Authored restarts/changed counters must become native-derived groups,
					// rather than being hidden inside a stale old list container.
					current := r.listParagraphs[flow[item.start].Position]
					if !sameColleafNumbering(info, current) || info.Kind == "number" && current.Value != info.Value+itemIndex {
						valid = false
						break
					}
					children, err := group(item.start+1, item.end+1, depth+1, info.Bullet.ID, "item", flow[item.start].ID, flow[item.start].Position+1, flow[item.end].Position+1)
					if err != nil {
						return Block{}, 0, err
					}
					firstBlocks := append([]Block{flow[item.start].Block}, flow[item.start].After...)
					block.Items = append(block.Items, ListItem{Blocks: append(firstBlocks, children...)})
				}
				if valid {
					r.promoteListPoints(&block, flow[start].ID, flow[start].Position)
					return block, end + 1, nil
				}
			}
			r.doc.warn("colleaf_stale_list_ownership", "Stale list ownership metadata ignored; actual native bullets and visible block order are retained")
		}
		// Foreign/unbound native lists have explicit bullets but no continuation
		// ownership. Infer only consecutive marked paragraphs and deeper marked runs.
		block := nativeListBlock(info, nil)
		cursor := start
		for cursor < len(flow) {
			current, ok := r.listParagraphs[flow[cursor].Position]
			if !ok || len(flow[cursor].Bullet) == 0 || current.Bullet.Level != info.Bullet.Level || current.Bullet.ID != info.Bullet.ID || current.Bullet.Type != info.Bullet.Type || !sameColleafNumbering(info, current) || (info.Kind == "number" && current.Value != info.Value+len(block.Items)) {
				break
			}
			item := ListItem{Blocks: append([]Block{flow[cursor].Block}, flow[cursor].After...)}
			position := flow[cursor].Position
			cursor++
			// A single native paragraph explicitly owns its inline image/table
			// fragments even when no list-item continuation resource exists.
			for cursor < len(flow) && flow[cursor].Position == position {
				item.Blocks = append(item.Blocks, flow[cursor].Block)
				item.Blocks = append(item.Blocks, flow[cursor].After...)
				cursor++
			}
			for cursor < len(flow) {
				nested, ok := r.listParagraphs[flow[cursor].Position]
				if !ok || len(flow[cursor].Bullet) == 0 || nested.Bullet.Level <= info.Bullet.Level {
					break
				}
				child, next, err := consume(cursor, depth+1, info.Bullet.ID)
				if err != nil {
					return Block{}, 0, err
				}
				item.Blocks = append(item.Blocks, child)
				cursor = next
			}
			block.Items = append(block.Items, item)
		}
		r.doc.warn("colleaf_list_ownership", "Native bullet records infer list containers; unmarked continuation ownership is not explicit")
		r.promoteListPoints(&block, flow[start].ID, flow[start].Position)
		return block, cursor, nil
	}
	return group(0, len(flow), depth, "", "section", sectionID, lower, upper)
}
