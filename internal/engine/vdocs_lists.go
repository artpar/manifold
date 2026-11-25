package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
)

// Native glyphFormat reads whole decimal placeholder indices and has no percent
// escape. SF/OOXML uses single-digit %1..%9. Non-active fixed-symbol entries bridge
// literal percent and digit suffixes while leaving actual counters automatic.
const vdocsMaxListEntries = 9 + 512

type vdocsListPart struct {
	Literal string
	Level   int
	Dynamic bool
}

func appendColleafListLiteral(parts []vdocsListPart, text string) []vdocsListPart {
	if text == "" {
		return parts
	}
	if len(parts) > 0 && !parts[len(parts)-1].Dynamic {
		parts[len(parts)-1].Literal += text
		return parts
	}
	return append(parts, vdocsListPart{Literal: text})
}
func vdocsSFListParts(template string, level int) []vdocsListPart {
	var parts []vdocsListPart
	for i := 0; i < len(template); i++ {
		if template[i] == '%' && i+1 < len(template) && template[i+1] >= '1' && template[i+1] <= '9' {
			n := int(template[i+1] - '1')
			i++
			if n <= level {
				parts = append(parts, vdocsListPart{Level: n, Dynamic: true})
			}
		} else {
			parts = appendColleafListLiteral(parts, template[i:i+1])
		}
	}
	return parts
}
func projectColleafListTemplate(template string, level int, source *vdocsNativeList) string {
	var out strings.Builder
	helpers := map[string]int{}
	for _, part := range vdocsSFListParts(template, level) {
		if part.Dynamic {
			out.WriteByte('%')
			out.WriteString(strconv.Itoa(part.Level + 1))
			continue
		}
		text := part.Literal
		digitSuffix := out.Len() > 0 && len(text) > 0 && text[0] >= '0' && text[0] <= '9'
		if !strings.Contains(text, "%") && !digitSuffix {
			out.WriteString(text)
			continue
		}
		index, ok := helpers[text]
		if !ok {
			// Keep all nine real outline levels available; helpers are never an active
			// paragraph nestingLevel or a newly invented SF numbering level.
			for len(source.Levels) < 9 {
				n := len(source.Levels)
				glyph := 2
				source.Levels = append(source.Levels, vdocsListLevel{Format: "%" + strconv.Itoa(n+1) + ".", Glyph: &glyph, Align: 1})
			}
			index = len(source.Levels)
			helpers[text] = index
			glyph := 2
			source.Levels = append(source.Levels, vdocsListLevel{Symbol: text, Glyph: &glyph, Align: 1})
		}
		out.WriteByte('%')
		out.WriteString(strconv.Itoa(index + 1))
	}
	return out.String()
}

func vdocsTemplateMatchesOriginal(template string, level int, source vdocsNativeList) bool {
	if len(template) > 512 {
		return false
	}
	// Only the active template and helper symbols affect its literal projection.
	// Current formats, counters, styles and ancestry continue to come from native.

	// Indent logical levels using the native preset's 21px step. A zero hanging
	// width retains the engine's measured marker advance instead of forcing long
	// custom counters into the preset's fixed 21px frame. Without the indent, a
	// hanging marker sits outside a clipped cell. Custom indents still diagnose.

	projected := vdocsNativeList{Levels: append([]vdocsListLevel(nil), source.Levels[:level+1]...)}
	format := projectColleafListTemplate(template, level, &projected)
	if format != source.Levels[level].Format || source.Levels[level].Symbol != "" {
		return false
	}
	for i := 9; i < len(projected.Levels); i++ {
		if i >= len(source.Levels) || source.Levels[i].Symbol != projected.Levels[i].Symbol {
			return false
		}
	}
	return true
}
func readColleafListTemplate(

	// Existing native documents may omit preset layout properties.

	source vdocsNativeList, level int, binding vdocsListBinding) (string, error) {
	if binding.OriginalTemplate != nil && vdocsTemplateMatchesOriginal(*binding.OriginalTemplate, level, source) {
		return *binding.OriginalTemplate, nil
	}
	var parts []vdocsListPart
	logicalBytes := 0
	literal := func(text string) error {
		if logicalBytes+len(text) > 512 {
			return fmt.Errorf("%w: native list template exceeds SF representation", ErrUnsupported)
		}
		logicalBytes += len(text)
		parts = appendColleafListLiteral(parts, text)
		return nil
	}
	if source.Levels[level].Symbol != "" {
		if err := literal(source.Levels[level].Symbol); err != nil {
			return "", err
		}
	} else {
		format := source.Levels[level].Format
		if len(format) > 3072 {
			return "", fmt.Errorf("%w: native list format exceeds SF projection", ErrUnsupported)
		}
		for i := 0; i < len(format); {
			if format[i] != '%' {
				start := i
				for i < len(format) && format[i] != '%' {
					i++
				}
				if err := literal(format[start:i]); err != nil {
					return "", err
				}
				continue
			}
			i++
			start := i
			for i < len(format) && format[i] >= '0' && format[i] <= '9' {
				i++
			}
			if i == start {
				return "", fmt.Errorf("%w: native percent without a numeric placeholder", ErrUnsupported)
			}
			index, err := strconv.Atoi(format[start:i])
			if err != nil || index < 1 || index > len(source.Levels) {
				return "", fmt.Errorf("invalid native list placeholder")
			}
			index--
			spec := source.Levels[index]
			if spec.Symbol != "" {
				if err := literal(spec.Symbol); err != nil {
					return "", err
				}
			} else if index <= level {
				logicalBytes += 2
				if logicalBytes > 512 {
					return "", fmt.Errorf("%w: native list template exceeds SF representation", ErrUnsupported)
				}
				parts = append(parts, vdocsListPart{Level: index, Dynamic: true})
			} else {
				return "", fmt.Errorf("%w: native numbering refers to a dynamic absent level",

					// This is an empty, right-aligned label-only cell. Its marker uses
					// its measured width; a preset hanging width clips long prefixes.

					ErrUnsupported)
			}
		}
	}
	var out strings.Builder
	for _, part := range parts {
		if part.Dynamic {
			out.WriteByte('%')
			out.WriteString(strconv.Itoa(part.Level + 1))
		} else {
			out.WriteString(part.Literal)
		}
	}
	candidate := out.String()
	if !reflect.DeepEqual(parts, vdocsSFListParts(candidate, level)) {
		return "", fmt.Errorf("%w: native literal marker cannot be expressed by the SF numbering template", ErrUnsupported)
	}
	if len(candidate) > 512 {
		return "", fmt.Errorf("%w: native list template exceeds SF representation", ErrUnsupported)
	}
	return candidate, nil
}

const vdocsListResource = "manydoc.list-ownership.v1"

type vdocsBullet struct {
	Type  string          `json:"listType"`
	ID    string          `json:"listId"`
	Level int             `json:"nestingLevel"`
	Start *int            `json:"startNumber,omitempty"`
	Image json.RawMessage `json:"image,omitempty"`
	Style json.RawMessage `json:"textStyle,omitempty"`
}
type vdocsListLevel struct {
	Format     string          `json:"glyphFormat"`
	Glyph      *int            `json:"glyphType,omitempty"`
	Symbol     string          `json:"glyphSymbol,omitempty"`
	Start      int             `json:"startNumber"`
	Align      int             `json:"bulletAlignment"`
	Legal      bool            `json:"isLegal,omitempty"`
	Properties json.RawMessage `json:"paragraphProperties,omitempty"`
	Style      json.RawMessage `json:"textStyle,omitempty"`
}
type vdocsNativeList struct {
	Type   string           `json:"listType"`
	Levels []vdocsListLevel `json:"nestingLevel"`
}
type vdocsListItemBinding struct {
	Type  string `json:"type,omitempty"`
	First string `json:"first"`
	Last  string `json:"last"`
}
type vdocsListBinding struct {
	Projections      map[string]vdocsNumberProjection `json:"projections,omitempty"`
	Type             string                           `json:"type"`
	Group            string                           `json:"group"`
	First            string                           `json:"first"`
	Last             string                           `json:"last"`
	Items            []vdocsListItemBinding           `json:"items"`
	Implicit         bool                             `json:"implicitNumbering,omitempty"`
	OriginalTemplate *string                          `json:"originalTemplate,omitempty"`
	Independent      bool                             `json:"independent,omitempty"`
}
type vdocsListOwnership struct {
	Stories map[string][]vdocsListBinding `json:"stories"`
}
type vdocsFlowBlock struct {
	After    []Block
	Block    Block
	ID       string
	Bullet   json.RawMessage
	Position int
	Start    int
}
type vdocsListParagraph struct {
	Bullet          vdocsBullet
	Kind            string
	Value           int
	Numbering       *ListNumbering
	ActiveAncestors []int
	Loss            bool
	PolicyLoss      bool
	StaleProjection bool
}

var vdocsNumberFormats = []string{"decimal", "decimalZero", "upperLetter", "lowerLetter", "upperRoman", "lowerRoman"}

func vdocsListLayout(level int) json.RawMessage {
	return sfAttrs(map[string]any{"hanging": map[string]int{"v": 0}, "indentStart": map[string]int{"v": 21 * (level + 1)}})
}

func standardColleafListLayout(raw json.RawMessage, level int) bool {
	if len(raw) == 0 {
		return true
	}
	var actual, expected any
	if json.Unmarshal(raw, &actual) != nil || json.Unmarshal(vdocsListLayout(level), &expected) != nil {
		return false
	}
	return reflect.DeepEqual(actual, expected)
}

func vdocsNumberGlyph(format string) (int, error) {
	for i, f := range vdocsNumberFormats {
		if f == format {
			return i + 2, nil
		}
	}
	return 0, fmt.Errorf("%w: native numbering format%s", ErrUnsupported, format)
}

func (w *vdocsBodyWriter) list(block Block, depth int) error {
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
	source := vdocsNativeList{Type: typeID}
	for l := 0; l <= level; l++ {
		format := "decimal"
		start := 0
		if block.Numbering != nil {
			format = block.Numbering.Formats[l]
			if l < level {
				start = block.Numbering.Ancestors[l] - 1
			}
		}
		glyph, err := vdocsNumberGlyph(format)
		if err != nil {
			return err
		}
		entry := vdocsListLevel{Format: "%" + strconv.Itoa(l+1) + ".", Glyph: &glyph, Start: start, Align: 1, Properties: vdocsListLayout(l)}
		if len(block.Items[0].Blocks) > 0 && block.Items[0].Blocks[0].vdocsAutoLabel {

			entry.Properties = nil
		}
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
	binding := vdocsListBinding{Type: typeID, Group: group, Implicit: block.Numbering == nil, Independent: independent}
	if normalProjection != nil || firstProjection != nil {
		binding.Projections = map[string]vdocsNumberProjection{}
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
		bullet := vdocsBullet{Type: typeID, ID: group, Level: level}
		if i == 0 {
			bullet.Type = firstType
			start := listStart(block) - 1
			bullet.Start = &start
		}
		w.body.Paragraphs[first].Bullet = sfAttrs(bullet)

		props, err := vdocsStyleObject(w.body.Paragraphs[first].Style)
		if err != nil {
			return err
		}
		if i == 0 && block.Source != nil {
			hash, err := vdocsListFingerprint(block)
			if err != nil {
				return err
			}
			origin, err := newColleafSourceOrigin(block.Source, hash, vdocsListOriginProfile)
			if err != nil {
				return err
			}
			props["manydocListOrigin"] = sfAttrs(origin)
		}
		if item.Source != nil {
			hash, err := vdocsItemFingerprint(block, i)
			if err != nil {
				return err
			}
			origin, err := newColleafSourceOrigin(item.Source, hash, vdocsItemOriginProfile)
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
		itemBinding := vdocsListItemBinding{First: firstID, Last: lastID}
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

func vdocsListParagraphs(ctx context.Context, body vdocsBody, lists map[string]json.RawMessage, bindings map[string]vdocsListBinding) (map[int]vdocsListParagraph, error) {
	result := map[int]vdocsListParagraph{}
	sources := map[string]vdocsNativeList{}
	for key, raw := range lists {
		var source vdocsNativeList
		if err := json.Unmarshal(raw, &source); err != nil {
			return nil, err
		}
		if key == "" || source.Type != key || len(source.Levels) == 0 || len(source.Levels) > vdocsMaxListEntries {
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
	var paragraphs []vdocsParagraphRecord
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
		var bullet vdocsBullet
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
		info := vdocsListParagraph{Bullet: bullet, Value: value, Kind: "number"}
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
				n.Formats = append(n.Formats, vdocsNumberFormats[*spec.Glyph-2])
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
		info.Loss = info.Loss || len(level.Style) > 0 || !standardColleafListLayout(level.Properties, bullet.Level) || len(bullet.Style) > 0 || level.Align != 1
		result[p.StartIndex] = info
		state.values[bullet.Level] = value
		state.seen[bullet.Level] = true
		for l := bullet.Level + 1; l < 9; l++ {
			state.seen[l] = false
		}
	}
	return result, nil
}

func nativeListBlock(info vdocsListParagraph, binding *vdocsListBinding) Block {
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
func (r *vdocsBodyReader) groupLists(flow []vdocsFlowBlock, depth int, sectionID string, lower, upper int) ([]Block, error) {
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

			// Only control/ownership intent is persisted here. Both cells contain actual
			// native editable content. No text, assets or SF subtree is cached in metadata.

			// Lists grouped by ordinary bullets can contain literal layout tables too.

			// Sparse format intent only: actual templates, marker text, counters, item bodies
			// and styles remain native. A zero-only type shares the normal list counter group.

			// Integer member names must be canonical: encoding/json otherwise aliases "01"
			// and "1" to the same Go map key before intent validation can see ambiguity.

			// Recover the actual foreign native presentation without changing SF's existing
			// zero/legal rules. A foreign zero can need a distinct first-item template; group
			// assembly checks semantic changes instead of hiding them under one list type.

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

type vdocsLiteralItem struct {
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

func (m *vdocsLiteralItem) validate() error {
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
func (w *vdocsBodyWriter) literalList(block Block, depth int) error {
	if depth > 64 || len(block.Items) == 0 {
		return fmt.Errorf("native literal list nesting/size")
	}
	group := w.id("literal-list")
	for i, item := range block.Items {
		if err := w.ctx.Err(); err != nil {
			return err
		}
		m := &vdocsLiteralItem{Version: 1, Group: group, Index: i, Count: len(block.Items), Kind: block.List, Start: block.Start, Numbering: block.Numbering, Automatic: item.Marker == nil, Right: item.Blocks[0].Direction == "rtl"}
		if err := m.validate(); err != nil {
			return err
		}
		table := vdocsLiteralTable(block, item, i)
		body := &table.Rows[0].Cells[1]
		if m.Right {
			body = &table.Rows[0].Cells[0]
		}
		body.Blocks = append([]Block(nil), body.Blocks...)
		p := &body.Blocks[0]
		if i == 0 && block.Source != nil {
			hash, err := vdocsListFingerprint(block)
			if err != nil {
				return err
			}
			p.vdocsListOrigin, err = newColleafSourceOrigin(block.Source, hash, vdocsListOriginProfile)
			if err != nil {
				return err
			}
		}
		if item.Source != nil {
			hash, err := vdocsItemFingerprint(block, i)
			if err != nil {
				return err
			}
			p.vdocsItemOrigin, err = newColleafSourceOrigin(item.Source, hash, vdocsItemOriginProfile)
			if err != nil {
				return err
			}
		}
		if item.Marker != nil && item.Marker.Source != nil {
			hash, err := vdocsMarkerFingerprint(item.Marker)
			if err != nil {
				return err
			}
			p.vdocsMarkerOrigin, err = newColleafSourceOrigin(item.Marker.Source, hash, vdocsMarkerOriginProfile)
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
		props, err := vdocsStyleObject(w.tables[id])
		if err != nil {
			return err
		}
		props["manydocLiteralItem"] = sfAttrs(m)
		w.tables[id] = sfAttrs(props)
	}
	return nil
}

func literalNativeItem(b Block, m *vdocsLiteralItem) (ListItem, bool) {
	if b.Kind != Table || len(b.Rows) != 1 || b.Rows[0].RepeatHeader || len(b.Rows[0].Cells) != 2 || len(b.ColumnWidths) != 0 || b.vdocsTableOrigin != nil {
		return ListItem{}, false
	}
	for _, c := range b.Rows[0].Cells {
		if c.ColSpan > 1 || c.RowSpan > 1 || c.Source != nil || c.vdocsOrigin != nil {
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
		if !p.vdocsAutoLabel || p.Kind != Paragraph || len(p.Runs) != 0 || len(p.Anchors) != 0 || p.PageBreakBefore || !reflect.DeepEqual(label[0].Numbering, m.Numbering) {
			return ListItem{}, false
		}
		if m.Kind == "number" && listStart(label[0]) != listStart(Block{Start: m.Start})+m.Index {
			return ListItem{}, false
		}
	}
	return item, true
}

func (r *vdocsBodyReader) literalLists(bs []Block, depth int) ([]Block, error) {
	if depth > 64 {
		return nil, fmt.Errorf("native literal ownership nesting exceeds64")
	}

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
		m := bs[i].vdocsLiteral
		if m == nil {
			out = append(out, bs[i])
			i++
			continue
		}
		valid := m.Index == 0 && m.Count <= len(bs)-i
		b := Block{Kind: ListBlock, List: m.Kind, Start: m.Start, Numbering: m.Numbering, Anchors: bs[i].Anchors}
		if valid {
			for j := 0; j < m.Count; j++ {
				current := bs[i+j].vdocsLiteral
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
		bs[i].vdocsLiteral = nil
		out = append(out, bs[i])
		i++
	}
	return out, nil
}

func vdocsLiteralTable(block Block, item ListItem, index int) Block {
	var label []Block
	if item.Marker != nil {
		label = item.Marker.Blocks
	} else {
		automatic := Block{Kind: ListBlock, List: block.List, Numbering: block.Numbering, Items: []ListItem{{Blocks: []Block{{Kind: Paragraph, vdocsAutoLabel: true}}}}}
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

type vdocsNumberProjection struct {
	Formats    vdocsProjectedFormats `json:"formats"`
	ZeroLevels []int                 `json:"zeroLevels,omitempty"`
	Legal      bool                  `json:"legal,omitempty"`
}

type vdocsProjectedFormats map[int]string

func (f *vdocsProjectedFormats) UnmarshalJSON(data []byte) error {
	if !bytes.HasPrefix(bytes.TrimSpace(data), []byte("{")) {
		return fmt.Errorf("native projected formats must be an object")
	}
	var entries map[string]json.RawMessage
	if err := json.Unmarshal(data, &entries); err != nil {
		return err
	}
	if len(entries) > 9 {
		return fmt.Errorf("too many native projected formats")
	}
	result := vdocsProjectedFormats{}
	for key, raw := range entries {
		level, err := strconv.Atoi(key)
		if err != nil || level < 0 || level > 8 || strconv.Itoa(level) != key {
			return fmt.Errorf("noncanonical native projected format level")
		}
		var format *string
		if err = json.Unmarshal(raw, &format); err != nil || format == nil {
			return fmt.Errorf("native projected format must be a string")
		}
		result[level] = *format
	}
	*f = result
	return nil
}

func projectColleafNumbering(source vdocsNativeList, n *ListNumbering, value int) (vdocsNativeList, *vdocsNumberProjection) {
	if n == nil {
		return source, nil
	}
	source.Levels = append([]vdocsListLevel(nil), source.Levels...)
	p := &vdocsNumberProjection{Formats: map[int]string{}, Legal: n.Legal}
	for l, format := range n.Formats {
		v := value
		if l < n.Level {
			v = n.Ancestors[l]
		}
		zero := !n.Legal && v == 0 && format != "decimal"
		if zero || n.Legal && format == "decimalZero" {
			source.Levels[l].Glyph = vdocsInt(2)
			p.Formats[l] = format
			if zero {
				p.ZeroLevels = append(p.ZeroLevels, l)
			}
		}
	}
	if len(p.Formats) == 0 {
		return source, nil
	}
	return source, p
}
func validateColleafNumberProjection(p vdocsNumberProjection, level int) error {
	if len(p.Formats) == 0 || len(p.Formats) > 9 || len(p.ZeroLevels) > 9 {
		return fmt.Errorf("invalid native numbering projection")
	}
	zeros := map[int]bool{}
	for _, l := range p.ZeroLevels {
		if l < 0 || l > level || zeros[l] || p.Formats[l] == "" {
			return fmt.Errorf("invalid native zero-level intent")
		}
		zeros[l] = true
	}
	for l, format := range p.Formats {
		if l < 0 || l > level || !supportedNumberFormat(format) || format == "decimal" || (p.Legal && (format != "decimalZero" || len(zeros) != 0)) || (!p.Legal && !zeros[l]) {
			return fmt.Errorf("invalid native format intent")
		}
	}
	return nil
}
func restoreColleafNumberProjection(n *ListNumbering, p vdocsNumberProjection, value int, ancestors []int, source vdocsNativeList) bool {
	if n.Legal != p.Legal || source.Levels[n.Level].Symbol != "" {
		return false
	}
	for l := range p.Formats {
		if n.Formats[l] != "decimal" || source.Levels[l].Symbol != "" {
			return false
		}
	}
	for _, l := range p.ZeroLevels {
		v := value
		if l < n.Level {
			v = ancestors[l]
		}
		if v != 0 {
			return false
		}
	}
	for l, format := range p.Formats {
		n.Formats[l] = format
	}
	return true
}

func normalizeColleafNativeNumbering(n *ListNumbering, value int, ancestors []int) (bool, error) {
	loss := false
	if n.Legal {
		padded := false
		for _, f := range n.Formats {
			padded = padded || f == "decimalZero"
		}
		if padded {
			n.Legal = false
			loss = true
			for i, f := range n.Formats {
				if f != "decimalZero" {
					n.Formats[i] = "decimal"
				}
			}
		}
	}
	if n.Legal {
		return loss, nil
	}
	parts := vdocsSFListParts(n.Template, n.Level)
	var actual []vdocsListPart
	for _, part := range parts {
		if part.Dynamic {
			v := value
			if part.Level < n.Level {
				v = ancestors[part.Level]
			}
			if v == 0 && n.Formats[part.Level] != "decimal" {
				zero := ""
				if n.Formats[part.Level] == "decimalZero" {
					zero = "00"
				}
				actual = appendColleafListLiteral(actual, zero)
				loss = true
				continue
			}
			actual = append(actual, part)
		} else {
			actual = appendColleafListLiteral(actual, part.Literal)
		}
	}
	if !loss {
		return false, nil
	}
	candidate := vdocsListPartsText(actual)
	if len(candidate) > 512 || !reflect.DeepEqual(actual, vdocsSFListParts(candidate, n.Level)) {
		return false, fmt.Errorf("%w: native zero marker cannot be expressed in the SF numbering template", ErrUnsupported)
	}
	n.Template = candidate
	return true, nil
}
func vdocsListPartsText(parts []vdocsListPart) string {
	var out string
	for _, part := range parts {
		if part.Dynamic {
			out += "%" + string(rune('1'+part.Level))
		} else {
			out += part.Literal
		}
	}
	return out
}
func sameColleafNumbering(a, b vdocsListParagraph) bool {
	return a.Kind == b.Kind && reflect.DeepEqual(a.Numbering, b.Numbering)
}
func vdocsListBindingTypes(b vdocsListBinding) ([]string, error) {
	if b.Type == "" {
		return nil, fmt.Errorf("invalid native list binding type")
	}
	seen := map[string]bool{b.Type: true}
	for _, item := range b.Items {
		if item.Type != "" {
			seen[item.Type] = true
		}
	}
	for name := range b.Projections {
		if !seen[name] {
			return nil, fmt.Errorf("unowned native numbering projection")
		}
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}
