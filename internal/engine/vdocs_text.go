package engine

import (
	"container/heap"
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"
)

const vdocsHyperlinkResource = "DOC_HYPER_LINK_PLUGIN"

type vdocsHyperlinkResourceData struct {
	Links *[]vdocsHyperlinkResourceEntry `json:"links"`
}
type vdocsHyperlinkResourceEntry struct {
	ID      *string `json:"id"`
	Payload *string `json:"payload"`
}

// Pinned plugin onLoad applies resource payloads to main/header/footer ranges by
// global rangeId. Notes are not visited. Follow that precedence explicitly and
// reject ambiguous identities instead of depending on map/story iteration order.
// Copy only the slices/maps modified here; caller-owned snapshots remain intact.

// Native altLang remains authoritative. The marker identifies the SF slot it
// represents and retains the distinct authored complex-language setting, which
// the pinned engine does not expose as a separate rendering property.

// Actual Univer selects the last matching whole text-run record. A sweep avoids
// quadratic per-character scans for valid overlapping/unsorted native intervals.
func applyVDOCSHyperlinkResource(ctx context.Context, input vdocsNoteSnapshot) (vdocsNoteSnapshot, []Diagnostic, error) {
	var resource *vdocsResource
	var remaining []vdocsResource
	for _, r := range input.Resources {
		if r.Name == vdocsHyperlinkResource {
			if resource != nil {
				return input, nil, fmt.Errorf("duplicate native hyperlink resource")
			}
			copy := r
			resource = &copy
		} else {
			remaining = append(remaining, r)
		}

		// These private text workers are shared by future main/story/note mappings.
		// Unsupported rich containers reject before returning partial document content.
	}
	if resource == nil {
		return input, nil, ctx.Err()
	}
	raw := []byte(resource.Data)
	if err := validateTypedJSON(ctx, raw, reflect.TypeFor[vdocsHyperlinkResourceData]()); err != nil {
		return input, nil, fmt.Errorf("native hyperlink resource JSON: %w", err)
	}
	var data vdocsHyperlinkResourceData
	if err := strictSFAttrs(raw, &data); err != nil {
		return input, nil, err
	}
	if data.Links == nil {
		return input, nil, fmt.Errorf("native hyperlink resource requires links array")
	}
	out := input
	out.Resources = remaining
	ranges := map[string]*vdocsRangeRecord{}
	index := func(body *vdocsBody) error {
		body.CustomRanges = append([]vdocsRangeRecord(nil), body.CustomRanges...)
		for i := range body.CustomRanges {
			if err := ctx.Err(); err != nil {
				return err
			}
			a := &body.CustomRanges[i]
			if a.Type != 0 {
				continue
			}
			if a.ID == "" || ranges[a.ID] != nil {
				return fmt.Errorf("ambiguous native hyperlink ID across stories")
			}
			ranges[a.ID] = a
		}
		return nil
	}
	if err := index(&out.Body); err != nil {
		return input, nil, err
	}
	copyStories := func(source map[string]vdocsRunningStory) (map[string]vdocsRunningStory, error) {
		if source == nil {
			return nil, nil
		}
		copied := make(map[string]vdocsRunningStory, len(source))
		ids := make([]string, 0, len(source))
		for id := range source {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			story := source[id]
			if err := index(&story.Body); err != nil {
				return nil, err
			}
			copied[id] = story
		}
		return copied, nil
	}
	var err error
	if out.Headers, err = copyStories(input.Headers); err != nil {
		return input, nil, err
	}
	if out.Footers, err = copyStories(input.Footers); err != nil {
		return input, nil, err
	}
	seen := map[string]bool{}
	changed, orphan := false, false
	for _, entry := range *data.Links {
		if err := ctx.Err(); err != nil {
			return input, nil, err
		}
		if entry.ID == nil || *entry.ID == "" || entry.Payload == nil || seen[*entry.ID] {
			return input, nil, fmt.Errorf("invalid/duplicate native hyperlink resource entry")
		}
		seen[*entry.ID] = true
		a, ok := ranges[*entry.ID]
		if !ok {
			orphan = true
			continue
		}
		props, err := vdocsStyleObject(a.Properties)
		if err != nil {
			return input, nil, err
		}
		var current string
		if raw, ok := props["url"]; ok {
			if err = json.Unmarshal(raw, &current); err != nil {
				return input, nil, err
			}
		}
		if *entry.Payload != "" {
			for _, key := range []string{"bookmarkId", "headingId"} {
				var internal string
				if raw, ok := props[key]; ok {
					if err = json.Unmarshal(raw, &internal); err != nil {
						return input, nil, err
					}
				}
				if internal != "" && !vdocsInternalHref(*entry.Payload, internal) {
					return input, nil, fmt.Errorf("%w: native hyperlink resource has conflicting external/internal targets", ErrUnsupported)
				}
			}
		}
		if current != *entry.Payload {
			props["url"] = sfAttrs(*entry.Payload)
			a.Properties = sfAttrs(props)
			changed = true
		}
	}
	var diagnostics []Diagnostic
	if changed {
		diagnostics = append(diagnostics, Diagnostic{"vdocs_hyperlink_resource_target", "Native hyperlink resource overrides a different body URL; the current editor target is retained"})
	}
	if orphan {
		diagnostics = append(diagnostics, Diagnostic{"vdocs_hyperlink_resource_orphan", "Native hyperlink resource contains inactive identities; current visible hyperlink ranges are retained"})
	}
	return out, diagnostics, nil
}

const vdocsLanguageProfile = "manydoc.run-languages"

type vdocsLanguages struct {
	Profile         string `json:"profile"`
	Version         int    `json:"version"`
	AlternateKind   string `json:"alternateKind"`
	ComplexLanguage string `json:"complexLanguage,omitempty"`
}

func readVDOCSLanguages(raw json.RawMessage) (*vdocsLanguages, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var value vdocsLanguages
	if err := validateTypedJSON(context.Background(), raw, reflect.TypeFor[vdocsLanguages]()); err != nil {
		return nil, err
	}
	if err := strictSFAttrs(raw, &value); err != nil {
		return nil, err
	}
	if value.Profile != vdocsLanguageProfile || value.Version != 1 || value.AlternateKind != "eastAsia" {
		return nil, fmt.Errorf("%w: native run language intent profile/version/slot", ErrUnsupported)
	}
	if err := validateLanguage(value.ComplexLanguage); err != nil {
		return nil, err
	}
	return &value, nil
}

type vdocsStyleSpan struct{ start, end, record int }
type vdocsStyleEvent struct {
	position, record int
	start            bool
}
type vdocsStyleHeap []int

func (h vdocsStyleHeap) Len() int           { return len(h) }
func (h vdocsStyleHeap) Less(i, j int) bool { return h[i] > h[j] }
func (h vdocsStyleHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *vdocsStyleHeap) Push(x any)        { *h = append(*h, x.(int)) }
func (h *vdocsStyleHeap) Pop() any          { a := *h; x := a[len(a)-1]; *h = a[:len(a)-1]; return x }

func vdocsStyleSpans(ctx context.Context, index vdocsTextIndex, runs []vdocsTextRun) ([]vdocsStyleSpan, error) {
	events := make([]vdocsStyleEvent, 0, 2*len(runs))
	for i, r := range runs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if _, err := index.exclusive(r.St, r.Ed); err != nil {
			return nil, err
		}
		if r.St != r.Ed {
			events = append(events, vdocsStyleEvent{r.St, i, true}, vdocsStyleEvent{r.Ed, i, false})
		}
	}
	sort.Slice(events, func(i, j int) bool { return events[i].position < events[j].position })
	active := make([]bool, len(runs))
	var priorities vdocsStyleHeap
	var out []vdocsStyleSpan
	previous, record := 0, -1
	emit := func(end int) {
		if previous == end {
			return
		}
		if len(out) > 0 && out[len(out)-1].record == record {
			out[len(out)-1].end = end
		} else {
			out = append(out, vdocsStyleSpan{previous, end, record})
		}
		previous = end
	}
	for i := 0; i < len(events); {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		position := events[i].position
		emit(position)
		for i < len(events) && events[i].position == position {
			e := events[i]
			active[e.record] = e.start
			if e.start {
				heap.Push(&priorities, e.record)
			}
			i++
		}
		for len(priorities) > 0 && !active[priorities[0]] {
			heap.Pop(&priorities)
		}
		record = -1
		if len(priorities) > 0 {
			record = priorities[0]
		}
	}
	emit(index.length())
	return out, nil
}

type vdocsNamedStyle struct {
	BasedOn        string          `json:"basedOn,omitempty"`
	TextStyle      json.RawMessage `json:"textStyle,omitempty"`
	ParagraphStyle json.RawMessage `json:"paragraphStyle,omitempty"`
}
type vdocsStyleMap map[string]json.RawMessage

func vdocsStyleObject(raw json.RawMessage) (vdocsStyleMap, error) {
	if len(raw) == 0 {
		return vdocsStyleMap{}, nil
	}
	if !strings.HasPrefix(strings.TrimSpace(string(raw)), "{") {
		return nil, fmt.Errorf("native style must be an object")
	}
	var v vdocsStyleMap
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, err
	}
	return v, nil
}
func mergeVDOCSStyle(a, b vdocsStyleMap) vdocsStyleMap {
	out := vdocsStyleMap{}
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		out[k] = v
	}
	return out
}

type vdocsStyleResolver struct {
	named  map[string]vdocsNamedStyle
	cache  map[string]vdocsNamedStyle
	active map[string]bool
}

func (r *vdocsStyleResolver) resolve(id string, depth int) (vdocsNamedStyle, error) {
	if id == "" {
		return vdocsNamedStyle{}, nil
	}
	if depth > 64 || r.active[id] {
		return vdocsNamedStyle{}, fmt.Errorf("cyclic or excessive native style inheritance")
	}
	if s, ok := r.cache[id]; ok {
		return s, nil
	}
	s, ok := r.named[id]
	if !ok {
		return s, fmt.Errorf("undefined native style %q", id)
	}
	if r.active == nil {
		r.active = map[string]bool{}
	}
	r.active[id] = true
	defer delete(r.active, id)
	parent, err := r.resolve(s.BasedOn, depth+1)
	if err != nil {
		return s, err
	}
	for _, pair := range [][2]*json.RawMessage{{&parent.TextStyle, &s.TextStyle}, {&parent.ParagraphStyle, &s.ParagraphStyle}} {
		a, err := vdocsStyleObject(*pair[0])
		if err != nil {
			return s, err
		}
		b, err := vdocsStyleObject(*pair[1])
		if err != nil {
			return s, err
		}
		*pair[1] = sfAttrs(mergeVDOCSStyle(a, b))
	}
	if r.cache == nil {
		r.cache = map[string]vdocsNamedStyle{}
	}
	r.cache[id] = s
	return s, nil
}
func vdocsBool(raw json.RawMessage) (bool, error) {
	var n *int
	if err := json.Unmarshal(raw, &n); err != nil || n == nil || (*n != 0 && *n != 1) {
		return false, fmt.Errorf("native BooleanNumber must be 0 or 1")
	}
	return *n == 1, nil
}
func vdocsStyleRun(style vdocsStyleMap, doc *Document) (Run, error) {
	var out Run
	qualifiedAlternate := false
	keys := make([]string, 0, len(style))
	for key := range style {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		raw := style[key]
		if string(raw) == "null" {
			continue
		}
		switch key {
		case "manydocOrigin":
			doc.warn("vdocs_run_origin_context", "Native source properties outside direct text intervals do not assert run ownership")
		case "manydocLanguages":
			value, err := readVDOCSLanguages(raw)
			if err != nil {
				return out, err
			}
			if value != nil {
				out.ComplexLanguage = value.ComplexLanguage
				qualifiedAlternate = true
			}
		case "altLang":
			if err := json.Unmarshal(raw, &out.EastAsiaLanguage); err != nil {
				return out, err
			}
		case "lang":
			if err := json.Unmarshal(raw, &out.Language); err != nil {
				return out, err
			}
		case "va":
			value, _, err := vdocsRawInt(style, "va")
			if err != nil {
				return out, err
			}
			if value != 0 {
				doc.warn("vdocs_text_presentation", "Native baseline offset is not represented by ordinary SF text; visible text is retained")
			}
		case "hidden":
			var hidden bool
			if err := json.Unmarshal(raw, &hidden); err != nil {
				return out, err
			}
			if hidden {
				return out, fmt.Errorf("%w: hidden native text needs visibility mapping", ErrUnsupported)
			}
		case "ff":
			if err := json.Unmarshal(raw, &out.FontFamily); err != nil {
				return out, err
			}
		case "fs":
			if err := json.Unmarshal(raw, &out.FontSize); err != nil {
				return out, err
			}
			if out.FontSize <= 0 {
				return out, fmt.Errorf("native font size must be positive points")
			}
		case "bl", "it":
			v, err := vdocsBool(raw)
			if err != nil {
				return out, err
			}
			if key == "bl" {
				out.Bold = v
			} else {
				out.Italic = v
			}
		case "ul", "st":
			var decoration struct {
				Show json.RawMessage `json:"s"`
				Type *int            `json:"t"`
			}
			if err := json.Unmarshal(raw, &decoration); err != nil {
				return out, err
			}
			if len(decoration.Show) == 0 {
				return out, fmt.Errorf("native decoration lacks its show flag")
			}
			show, err := vdocsBool(decoration.Show)
			if err != nil {
				return out, err
			}
			if key == "ul" {
				out.Underline = show
			} else {
				out.Strike = show
			}
			decorationFields, err := vdocsStyleObject(raw)
			if err != nil {
				return out, err
			}
			decorationKeys := make([]string, 0, len(decorationFields))
			for field := range decorationFields {
				decorationKeys = append(decorationKeys, field)
			}
			sort.Strings(decorationKeys)
			for _, field := range decorationKeys {
				if field != "s" && field != "t" {
					doc.warn("vdocs_decoration", "Native decoration property "+field+" is not represented; enabled underline/strike and visible text are retained")
				}
			}
			if show && decoration.Type != nil && *decoration.Type != 12 {
				doc.warn("vdocs_decoration", "Native decoration pattern is normalized to a single line; visible text and enabled underline/strike are retained")
			}
		case "cl":
			var color struct {
				RGB   *string         `json:"rgb"`
				Theme json.RawMessage `json:"th"`
			}
			if err := json.Unmarshal(raw, &color); err != nil {
				return out, err
			}
			if color.RGB != nil {
				var err error
				out.Color, err = vdocsRGB(*color.RGB)
				if err != nil {
					return out, err
				}
			} else if len(color.Theme) > 0 {
				return out, fmt.Errorf("%w: unresolved native theme color", ErrUnsupported)
			} else {
				out.Color = "auto"
			}
		default:
			doc.warn("vdocs_text_presentation", "Native text property "+key+" is not represented; supported typography and literal text are retained")
		}
	}
	if out.EastAsiaLanguage != "" && !qualifiedAlternate {
		doc.warn("vdocs_alternate_language_inferred", "Native alternate language is retained in the SF East Asian language slot without an explicit slot binding")
	}
	return out, validateTextStyle(out)
}
func vdocsRGB(color string) (string, error) {
	color = strings.TrimSpace(color)
	if strings.HasPrefix(strings.ToLower(color), "rgb(") && strings.HasSuffix(color, ")") {
		parts := strings.Split(color[4:len(color)-1], ",")
		if len(parts) != 3 {
			return "", fmt.Errorf("invalid native RGB color")
		}
		channels := make([]int, 3)
		for i, p := range parts {
			n, err := strconv.Atoi(strings.TrimSpace(p))
			if err != nil || n < 0 || n > 255 {
				return "", fmt.Errorf("invalid native RGB channel")
			}
			channels[i] = n
		}
		color = fmt.Sprintf("#%02X%02X%02X", channels[0], channels[1], channels[2])
	}
	if len(color) == 4 && color[0] == '#' {
		color = fmt.Sprintf("#%c%c%c%c%c%c", color[1], color[1], color[2], color[2], color[3], color[3])
	}
	check := Run{Color: strings.ToUpper(color)}
	if err := validateTextStyle(check); err != nil {
		return "", err
	}
	return check.Color, nil
}
func vdocsRunStyle(run Run) (json.RawMessage, error) {
	if err := validateTextStyle(run); err != nil {
		return nil, err
	}
	if run.ComplexScript || run.Direction != "" {
		return nil, fmt.Errorf("%w: native forced complex-script or run direction needs a qualified rendering mapping", ErrUnsupported)
	}

	if run.Source != nil && run.Text == "" && run.Note == nil {
		return nil, fmt.Errorf("%w: empty native run provenance requires a point binding", ErrUnsupported)
	}
	style := map[string]any{"bl": 0, "it": 0, "ul": map[string]int{"s": 0}, "st": map[string]int{"s": 0}}
	if run.Bold {
		style["bl"] = 1
	}
	if run.Italic {
		style["it"] = 1
	}
	if run.Underline {
		style["ul"] = map[string]int{"s": 1, "t": 12}
	}
	if run.Strike {
		style["st"] = map[string]int{"s": 1, "t": 12}
	}
	if run.Language != "" {
		style["lang"] = run.Language
	}
	if run.EastAsiaLanguage != "" {
		style["altLang"] = run.EastAsiaLanguage
	}
	if run.EastAsiaLanguage != "" || run.ComplexLanguage != "" {
		style["manydocLanguages"] = vdocsLanguages{Profile: vdocsLanguageProfile, Version: 1, AlternateKind: "eastAsia", ComplexLanguage: run.ComplexLanguage}
	}
	if run.FontFamily != "" {
		style["ff"] = run.FontFamily
	}
	if run.FontSize != 0 {
		style["fs"] = run.FontSize
	}
	if run.Color != "" {
		if run.Color == "auto" {
			style["cl"] = map[string]any{"rgb": nil}
		} else {
			style["cl"] = map[string]string{"rgb": run.Color}
		}
	}
	origin, err := newVDOCSRunOrigin(run)
	if err != nil {
		return nil, err
	}
	if origin != nil {
		style["manydocOrigin"] = origin
	}
	return sfAttrs(style), nil
}

type vdocsTextContext struct {
	NavigationAliases  map[string]string
	NavigationScope    string
	DeferNoteNumbering bool
	Citations          *vdocsCitationState
	NoteLabels         map[string]string
	DefaultText        json.RawMessage
	DefaultParagraph   json.RawMessage
	Styles             map[string]vdocsNamedStyle
	NoteIDs            map[string]noteKey
	Lists              map[string]json.RawMessage
	ListBindings       []vdocsListBinding
	Drawings           map[string]json.RawMessage
	DrawingOrder       []string
	ImageBindings      []vdocsImageBinding
	ImageCache         outputImageCache
}

func vdocsRawInt(style vdocsStyleMap, key string) (int, bool, error) {
	raw, ok := style[key]
	if !ok {
		return 0, false, nil
	}
	var n *int
	if err := json.Unmarshal(raw, &n); err != nil || n == nil {
		return 0, true, fmt.Errorf("native %s must be an integer", key)
	}
	return *n, true, nil
}
func vdocsUTF16Len(text string) int {
	n := 0
	for _, r := range text {
		n += utf16.RuneLen(r)
	}
	return n
}

func encodeVDOCSTextBody(ctx context.Context, blocks []Block) (vdocsBody, error) {
	return encodeVDOCSTextBodyWithNotes(ctx, blocks, nil)
}
func encodeVDOCSTextBodyWithNotes(ctx context.Context, blocks []Block, noteIDs map[noteKey]string, projection ...map[noteKey]string) (vdocsBody, error) {
	body := vdocsBody{TextRuns: []vdocsTextRun{}}
	var stream strings.Builder
	offset := 0
	serial := 0
	appendText := func(text string) { stream.WriteString(text); offset += vdocsUTF16Len(text) }
	for _, block := range blocks {
		if err := ctx.Err(); err != nil {
			return body, err
		}
		if (block.Kind != Paragraph && block.Kind != Heading) || block.Source != nil {
			return body, fmt.Errorf("%w: native text worker requires paragraph/heading content without unmapped provenance", ErrUnsupported)
		}
		var err error
		block, err = prepareVDOCSAnchorOrigins(block)
		if err != nil {
			return body, err
		}
		p := vdocsParagraphRecord{ID: fmt.Sprintf("p%d", serial)}
		serial++
		properties := map[string]any{}
		if block.Kind == Heading {
			if block.Level < 1 || block.Level > 6 {
				return body, fmt.Errorf("invalid heading rank")
			}
			properties["outlineLevel"] = block.Level - 1
			if block.Level <= 5 {
				properties["namedStyleType"] = block.Level + 3
			}
		}
		if block.Direction == "ltr" {
			properties["direction"] = 1
		} else if block.Direction == "rtl" {
			properties["direction"] = 2
		} else if block.Direction != "" {
			return body, fmt.Errorf("invalid paragraph direction")
		}
		if block.PageBreakBefore {
			properties["pageBreakBefore"] = 1
		}
		if len(properties) > 0 {
			p.Style = sfAttrs(properties)
		}
		for _, id := range block.Anchors {
			if err := destinationID(id); err != nil {
				return body, err
			}
			body.CustomRanges = append(body.CustomRanges, writeVDOCSBookmark(id, offset, p.ID, "block"))
		}
		for _, run := range block.Runs {
			if run.Anchor != "" {
				if err := destinationID(run.Anchor); err != nil {
					return body, err
				}
				copy := run
				copy.Anchor = ""
				copy.Source = nil
				copy.vdocsAnchorOrigin = nil
				if !reflect.DeepEqual(copy, Run{}) {
					return body, fmt.Errorf("%w: annotated native anchor atom", ErrUnsupported)
				}
				point := writeVDOCSBookmark(run.Anchor, offset, p.ID, "run")
				if run.vdocsAnchorOrigin != nil {
					props, err := readVDOCSBookmark(point)
					if err != nil {
						return body, err
					}
					props.Origin = run.vdocsAnchorOrigin
					point.Properties = sfAttrs(props)
				}
				body.CustomRanges = append(body.CustomRanges, point)
				continue
			}
			if strings.ContainsAny(run.Text, "\r\f\v\b\x1a\x1b\x1c\x1d\x0e\x0f\x1e\x1f") {
				return body, fmt.Errorf("%w: native text atom/break needs its full structural mapping", ErrUnsupported)
			}
			if err := validateString(run.Text); err != nil {
				return body, err
			}
			if run.Note != nil && len(projection) > 0 {
				if shared, projected := projection[0][referenceKey(run.Note)]; projected {
					id, ok := noteIDs[referenceKey(run.Note)]
					if !ok {
						return body, fmt.Errorf("native literal citation lacks definition")
					}
					run = canonicalCitationRun(run)
					mode := "default"
					var label []Run
					if run.Note.LabelContent != nil {
						mode = "rich"
						label = run.Note.LabelContent
					} else {
						text := shared
						if run.Note.Label != nil {
							mode = "literal"
							text = *run.Note.Label
						}
						r := run
						r.Note = nil
						r.Source = nil
						r.vdocsRunOrigin = nil
						r.Text = text
						label = []Run{r}
					}
					fragment, err := writeVDOCSCitation(ctx, run, id, mode, label)
					if err != nil {
						return body, err
					}
					for _, r := range fragment.TextRuns {
						r.St += offset
						r.Ed += offset
						body.TextRuns = append(body.TextRuns, r)
					}
					for _, a := range fragment.CustomRanges {
						a.StartIndex += offset
						a.EndIndex += offset
						a.ID = fmt.Sprintf("citation%d", len(body.CustomRanges))
						nameVDOCSCitationRange(&a)
						body.CustomRanges = append(body.CustomRanges, a)
					}
					appendText(fragment.DataStream)
					continue
				}
			}
			if run.Note != nil {
				id, ok := noteIDs[referenceKey(run.Note)]
				if !ok || run.Text != "" || run.Link != "" || run.Destination != "" || run.Note.Label != nil || run.Note.LabelContent != nil || run.Note.Inferred {
					return body, fmt.Errorf("%w: native note citation requires a mapped definition and shared marker semantics", ErrUnsupported)
				}
				style, err := vdocsRunStyle(run)
				if err != nil {
					return body, err
				}
				kind := 8
				if run.Note.Kind == Endnote {
					kind = 9
				}
				body.CustomRanges = append(body.CustomRanges, vdocsRangeRecord{StartIndex: offset, EndIndex: offset, ID: fmt.Sprintf("note%d", len(body.CustomRanges)), Type: kind, Whole: true, Properties: sfAttrs(map[string]string{"noteId": id})})
				body.TextRuns = append(body.TextRuns, vdocsTextRun{St: offset, Ed: offset + 1, TS: style})
				appendText("\ufffc")
				continue
			}
			if run.Text == "" {
				if run.Source != nil {
					return body, fmt.Errorf("%w: empty native run provenance requires a point binding", ErrUnsupported)
				}
				continue
			}
			style, err := vdocsRunStyle(run)
			if err != nil {
				return body, err
			}
			start := offset
			appendText(strings.ReplaceAll(run.Text, "\n", "\v"))
			body.TextRuns = append(body.TextRuns, vdocsTextRun{St: start, Ed: offset, TS: style})
			if run.Link != "" || run.Destination != "" {
				if run.Link != "" && run.Destination != "" && !vdocsInternalHref(run.Link, run.Destination) {
					return body, fmt.Errorf("conflicting native link targets")
				}
				properties := map[string]any{"textStyleMode": "text"}
				if run.Destination != "" {
					properties["bookmarkId"] = run.Destination
					properties["url"] = destinationHref(Run{Destination: run.Destination})
				} else {
					properties["url"] = run.Link
				}
				body.CustomRanges = append(body.CustomRanges, vdocsRangeRecord{StartIndex: start, EndIndex: offset - 1, ID: fmt.Sprintf("link%d", len(body.CustomRanges)), Type: 0, Properties: sfAttrs(properties)})
			}
		}
		p.StartIndex = offset
		appendText("\r")
		body.Paragraphs = append(body.Paragraphs, p)
	}
	if len(blocks) == 0 {
		body.Paragraphs = append(body.Paragraphs, vdocsParagraphRecord{StartIndex: 0, ID: "p0"})
		appendText("\r")
	}
	body.SectionBreaks = []vdocsSectionRecord{{StartIndex: offset, ID: "s0"}}
	appendText("\n")
	body.DataStream = stream.String()
	if _, err := indexVDOCSBody(ctx, body); err != nil {
		return body, err
	}
	return body, nil
}

func decodeVDOCSTextBody(ctx context.Context, body vdocsBody, settings vdocsTextContext) (*Document, error) {
	index, err := indexVDOCSBody(ctx, body)
	if err != nil {
		return nil, err
	}
	if len(index.tables) > 0 || len(index.sections) != 1 {
		return nil, fmt.Errorf("%w: native containers/sections require the rich body mapping", ErrUnsupported)
	}
	var blockBounds []int
	for position, block := range index.blocks {
		key, ok := settings.NoteIDs[block.NoteID]
		if !ok || (block.Type != nil && *block.Type != 1) || (block.NoteType != "" && block.NoteType != key.kind) {
			return nil, fmt.Errorf("%w: unmapped/contradictory native note atom", ErrUnsupported)
		}
		blockBounds = append(blockBounds, position, position+1)
	}
	sort.Ints(blockBounds)
	doc := &Document{}
	resolver := vdocsStyleResolver{named: settings.Styles}
	baseText, err := vdocsStyleObject(settings.DefaultText)
	if err != nil {
		return nil, err
	}
	baseParagraph, err := vdocsStyleObject(settings.DefaultParagraph)
	if err != nil {
		return nil, err
	}
	paragraphMetadata := map[int]vdocsParagraphRecord{}
	for _, p := range body.Paragraphs {
		paragraphMetadata[p.StartIndex] = p
	}
	spans, err := vdocsStyleSpans(ctx, index.text, body.TextRuns)
	if err != nil {
		return nil, err
	}
	var annotations []vdocsRangeRecord
	for _, a := range body.CustomRanges {
		if a.Type != 8 && a.Type != 9 && a.Type != 3 {
			annotations = append(annotations, a)
		}
	}
	sort.Slice(annotations, func(i, j int) bool { return annotations[i].StartIndex < annotations[j].StartIndex })
	var annotationBounds []int
	for i, annotation := range annotations {
		if annotation.Type != 0 {
			return nil, fmt.Errorf("%w: native annotation type %d requires its semantic mapping", ErrUnsupported, annotation.Type)
		}
		if i > 0 && annotations[i-1].EndIndex >= annotation.StartIndex {
			return nil, fmt.Errorf("conflicting native hyperlink extents")
		}
		annotationBounds = append(annotationBounds, annotation.StartIndex, annotation.EndIndex+1)
	}
	bookmarks := []vdocsRangeRecord{}
	for _, a := range body.CustomRanges {
		if a.Type == 3 {
			bookmarks = append(bookmarks, a)
		}
	}
	sort.SliceStable(bookmarks, func(i, j int) bool { return bookmarks[i].StartIndex < bookmarks[j].StartIndex })
	if settings.NavigationAliases == nil {
		settings.NavigationAliases, err = vdocsBookmarkAliases(body)
		if err != nil {
			return nil, err
		}
	}
	for _, node := range index.sections[0].children {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if node.kind != "paragraph" || len(node.children) > 0 {
			return nil, fmt.Errorf("%w: nontext native paragraph", ErrUnsupported)
		}
		p := paragraphMetadata[node.end-1]
		named, err := resolver.resolve(p.StyleID, 0)
		if err != nil {
			return nil, err
		}
		namedParagraph, err := vdocsStyleObject(named.ParagraphStyle)
		if err != nil {
			return nil, err
		}
		directParagraph, err := vdocsStyleObject(p.Style)
		if err != nil {
			return nil, err
		}
		properties := mergeVDOCSStyle(mergeVDOCSStyle(baseParagraph, namedParagraph), directParagraph)
		block := Block{Kind: Paragraph}
		origin, err := readVDOCSOrigin(directParagraph["manydocOrigin"])
		if err != nil {
			return nil, err
		}
		block.vdocsOrigin = origin
		block.vdocsListOrigin, err = readVDOCSOriginProfile(directParagraph["manydocListOrigin"], vdocsListOriginProfile)
		if err != nil {
			return nil, err
		}
		block.vdocsItemOrigin, err = readVDOCSOriginProfile(directParagraph["manydocItemOrigin"], vdocsItemOriginProfile)
		if err != nil {
			return nil, err
		}
		block.vdocsMarkerOrigin, err = readVDOCSOriginProfile(directParagraph["manydocMarkerOrigin"], vdocsMarkerOriginProfile)
		if err != nil {
			return nil, err
		}
		level, hasLevel, err := vdocsRawInt(properties, "outlineLevel")
		if err != nil {
			return nil, err
		}
		namedType, hasNamed, err := vdocsRawInt(properties, "namedStyleType")
		if err != nil {
			return nil, err
		}
		if hasLevel && level >= 0 && level <= 5 {
			block.Kind = Heading
			block.Level = level + 1
		} else if !hasLevel && hasNamed && namedType >= 4 && namedType <= 8 {
			block.Kind = Heading
			block.Level = namedType - 3
		} else if hasLevel && level != 9 {
			return nil, fmt.Errorf("%w: native outline level %d", ErrUnsupported, level)
		}
		direction, _, err := vdocsRawInt(properties, "direction")
		if err != nil {
			return nil, err
		}
		switch direction {
		case 0:
		case 1:
			block.Direction = "ltr"
		case 2:
			block.Direction = "rtl"
		default:
			return nil, fmt.Errorf("invalid native paragraph direction")
		}
		if raw, ok := properties["pageBreakBefore"]; ok {
			block.PageBreakBefore, err = vdocsBool(raw)
			if err != nil {
				return nil, err
			}
		}
		pText, err := vdocsStyleObject(properties["textStyle"])
		if err != nil {
			return nil, err
		}
		namedText, err := vdocsStyleObject(named.TextStyle)
		if err != nil {
			return nil, err
		}
		defaults := mergeVDOCSStyle(mergeVDOCSStyle(baseText, namedText), pText)
		headingID, err := vdocsNativeHeadingID(p.Style)
		if err != nil {
			return nil, err
		}
		if headingID != "" {
			block.Anchors = append(block.Anchors, vdocsHeadingID(settings.NavigationScope, headingID))
		}

		points := map[int][]Run{}
		firstPoint := sort.Search(len(bookmarks), func(i int) bool { return bookmarks[i].StartIndex >= node.start })
		for _, a := range bookmarks[firstPoint:] {
			if a.StartIndex > node.end-1 {
				break
			}
			fields, err := vdocsStyleObject(a.Properties)
			if err != nil {
				return nil, err
			}
			for name := range fields {
				if name != "bookmarkId" && name != "manydocNavigation" && name != "manydocOrigin" && name != "manydocFlowOrigin" {
					doc.warn("vdocs_bookmark_metadata", "Native bookmark metadata outside identity and placement is not represented in SF")
				}
			}
			bp, err := readVDOCSBookmark(a)
			if err != nil {
				return nil, err
			}
			if a.EndIndex != a.StartIndex {
				doc.warn("vdocs_bookmark_span", "Native bookmark range editing extent becomes an SF point at its current start")
			}
			if bp.Hint != nil && bp.Hint.Role == "block" && a.StartIndex == node.start && bp.Hint.ParagraphID == p.ID {
				block.Anchors = append(block.Anchors, bp.BookmarkID)
			} else {
				points[a.StartIndex] = append(points[a.StartIndex], Run{Anchor: bp.BookmarkID, vdocsAnchorOrigin: bp.Origin})
				if bp.Hint != nil && bp.Hint.Role != "run" && bp.Hint.Role != "list" && bp.Hint.Role != "point" {
					doc.warn("vdocs_stale_bookmark_placement", "Edited native bookmark position is retained as a current inline point rather than old block placement")
				}
			}
		}
		boundaries := []int{node.start, node.end - 1}
		for offset := range points {
			boundaries = append(boundaries, offset)
		}
		firstSpan := sort.Search(len(spans), func(i int) bool { return spans[i].end > node.start })
		for _, span := range spans[firstSpan:] {
			if span.start >= node.end-1 {
				break
			}
			if span.start > node.start && span.start < node.end-1 {
				boundaries = append(boundaries, span.start)
			}
			if span.end > node.start && span.end < node.end-1 {
				boundaries = append(boundaries, span.end)
			}
		}
		firstAnnotation := sort.SearchInts(annotationBounds, node.start+1)
		for _, boundary := range annotationBounds[firstAnnotation:] {
			if boundary >= node.end-1 {
				break
			}
			boundaries = append(boundaries, boundary)
		}
		firstBlock := sort.SearchInts(blockBounds, node.start+1)
		for _, boundary := range blockBounds[firstBlock:] {
			if boundary >= node.end-1 {
				break
			}
			boundaries = append(boundaries, boundary)
		}
		sort.Ints(boundaries)
		previous := -1
		for i := 0; i < len(boundaries)-1; i++ {
			start, end := boundaries[i], boundaries[i+1]
			if start == end || start == previous {
				continue
			}
			previous = start
			block.Runs = append(block.Runs, points[start]...)
			delete(points, start)
			text, err := index.text.exclusive(start, end)
			if err != nil {
				return nil, err
			}
			if strings.ContainsAny(text, "\f\x1e\x1f") {
				return nil, fmt.Errorf("%w: native text control requires semantic mapping", ErrUnsupported)
			}
			record := -1
			position := sort.Search(len(spans), func(i int) bool { return spans[i].end > start })
			if position < len(spans) {
				record = spans[position].record
			}
			style := defaults
			var runOrigin *vdocsOrigin
			if record >= 0 {
				raw := body.TextRuns[record]
				character, err := resolver.resolve(raw.StyleID, 0)
				if err != nil {
					return nil, err
				}
				inherited, err := vdocsStyleObject(character.TextStyle)
				if err != nil {
					return nil, err
				}
				direct, err := vdocsStyleObject(raw.TS)
				if err != nil {
					return nil, err
				}
				runOrigin, err = readVDOCSOriginProfile(direct["manydocOrigin"], vdocsRunOriginProfile)
				if err != nil {
					return nil, err
				}
				style = mergeVDOCSStyle(mergeVDOCSStyle(defaults, inherited), direct)
			}
			decodedStyle := style
			if len(style["manydocOrigin"]) > 0 {
				decodedStyle = mergeVDOCSStyle(nil, style)
				delete(decodedStyle, "manydocOrigin")
			}
			run, err := vdocsStyleRun(decodedStyle, doc)
			if err != nil {
				return nil, err
			}
			run.vdocsRunOrigin = runOrigin
			if runOrigin == nil && len(style["manydocOrigin"]) > 0 {
				doc.warn("vdocs_run_origin_inheritance", "Inherited native source properties are not treated as direct run ownership")
			}
			run.Text = strings.ReplaceAll(text, "\v", "\n")
			if text == "\b" || (text == "\ufffc" && index.blocks[start].NoteID != "") {
				atom, ok := index.blocks[start]
				if !ok {
					return nil, fmt.Errorf("unowned native note atom")
				}
				key := settings.NoteIDs[atom.NoteID]
				run.Text = ""
				run.Note = &NoteReference{ID: key.id, Kind: key.kind}
				if settings.Citations != nil {
					if scope, ok := settings.Citations.Scopes[atom.ID]; ok {
						settings.Citations.Pending[run.Note] = vdocsCitationPending{Scope: scope}
					}
				}

				if label, ok := settings.NoteLabels[atom.ID]; ok {
					copy := label
					run.Note.Label = &copy
				}
			} else if strings.ContainsRune(text, '\b') {
				return nil, fmt.Errorf("native note atom was not isolated")
			}
			var link *vdocsRangeRecord
			position = sort.Search(len(annotations), func(i int) bool { return annotations[i].EndIndex >= start })
			if position < len(annotations) && annotations[position].StartIndex <= start {
				link = &annotations[position]
			}
			if link != nil {
				if run.Note != nil {
					return nil, fmt.Errorf("%w: hyperlink on native note citation requires navigation mapping", ErrUnsupported)
				}
				var target vdocsLinkProperties
				if err := validateTypedJSON(ctx, link.Properties, reflect.TypeFor[vdocsLinkProperties]()); err != nil {
					return nil, err
				}

				if err := json.Unmarshal(link.Properties, &target); err != nil {
					return nil, err
				}
				if target.Bookmark != "" && target.Heading != "" {
					return nil, fmt.Errorf("conflicting native internal target kinds")
				}
				nativeID := target.Bookmark
				if target.Heading != "" {
					nativeID = target.Heading
				}
				alias := nativeID != "" && vdocsInternalHref(target.URL, nativeID)
				count := 0
				for _, value := range []string{target.URL, target.Bookmark, target.Heading} {
					if value != "" {
						count++
					}
				}
				if count != 1 && !(count == 2 && alias) {
					return nil, fmt.Errorf("native hyperlink requires one target")
				}
				run.Link = target.URL
				run.Destination = target.Bookmark
				if target.Heading != "" {
					run.Destination = vdocsHeadingID("", target.Heading)
				}
				if target.Bookmark != "" {
					if canonical, ok := settings.NavigationAliases[target.Bookmark]; ok {
						run.Destination = canonical
					}
				}
				if alias {
					run.Link = ""
				}
				if target.Mode != "text" {
					doc.warn("vdocs_link_presentation", "Native automatic hyperlink presentation may differ; target and authored text style are retained")
				}
			}
			block.Runs = append(block.Runs, run)
		}
		block.Runs = append(block.Runs, points[node.end-1]...)
		if len(p.Bullet) > 0 && string(p.Bullet) != "null" {
			return nil, fmt.Errorf("%w: native paragraph numbering needs list ownership mapping", ErrUnsupported)
		}
		keys := make([]string, 0, len(properties))
		for key := range properties {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			switch key {
			case "manydocFlowOrigin":
				doc.warn("vdocs_flow_origin_context", "Native flow source metadata outside a current standalone flow owner is not asserted")
			case "manydocListOrigin", "manydocItemOrigin", "manydocMarkerOrigin":
				if (key == "manydocListOrigin" && block.vdocsListOrigin == nil) || (key == "manydocItemOrigin" && block.vdocsItemOrigin == nil) {
					doc.warn("vdocs_list_origin_inheritance", "Inherited native source properties are not treated as direct list/item ownership")
				}
			case "manydocOrigin":
				if origin == nil {
					doc.warn("vdocs_origin_inheritance", "Inherited native source-binding properties are not treated as direct paragraph ownership")
				}
			case "outlineLevel", "namedStyleType", "direction", "pageBreakBefore", "textStyle", "headingId":
			default:
				doc.warn("vdocs_paragraph_style", "Native paragraph property "+key+" is not represented; text and supported paragraph semantics are retained")
			}
		}
		doc.Blocks = append(doc.Blocks, block)
	}
	return doc, nil
}
