package manydoc

import (
	"container/heap"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

type colleafStyleSpan struct{ start, end, record int }
type colleafStyleEvent struct {
	position, record int
	start            bool
}
type colleafStyleHeap []int

func (h colleafStyleHeap) Len() int           { return len(h) }
func (h colleafStyleHeap) Less(i, j int) bool { return h[i] > h[j] }
func (h colleafStyleHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *colleafStyleHeap) Push(x any)        { *h = append(*h, x.(int)) }
func (h *colleafStyleHeap) Pop() any          { a := *h; x := a[len(a)-1]; *h = a[:len(a)-1]; return x }

// Actual Univer selects the last matching whole text-run record. A sweep avoids
// quadratic per-character scans for valid overlapping/unsorted native intervals.
func colleafStyleSpans(ctx context.Context, index colleafTextIndex, runs []colleafTextRun) ([]colleafStyleSpan, error) {
	events := make([]colleafStyleEvent, 0, 2*len(runs))
	for i, r := range runs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if _, err := index.exclusive(r.St, r.Ed); err != nil {
			return nil, err
		}
		if r.St != r.Ed {
			events = append(events, colleafStyleEvent{r.St, i, true}, colleafStyleEvent{r.Ed, i, false})
		}
	}
	sort.Slice(events, func(i, j int) bool { return events[i].position < events[j].position })
	active := make([]bool, len(runs))
	var priorities colleafStyleHeap
	var out []colleafStyleSpan
	previous, record := 0, -1
	emit := func(end int) {
		if previous == end {
			return
		}
		if len(out) > 0 && out[len(out)-1].record == record {
			out[len(out)-1].end = end
		} else {
			out = append(out, colleafStyleSpan{previous, end, record})
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

type colleafNamedStyle struct {
	BasedOn        string          `json:"basedOn,omitempty"`
	TextStyle      json.RawMessage `json:"textStyle,omitempty"`
	ParagraphStyle json.RawMessage `json:"paragraphStyle,omitempty"`
}
type colleafStyleMap map[string]json.RawMessage

func colleafStyleObject(raw json.RawMessage) (colleafStyleMap, error) {
	if len(raw) == 0 {
		return colleafStyleMap{}, nil
	}
	if !strings.HasPrefix(strings.TrimSpace(string(raw)), "{") {
		return nil, fmt.Errorf("native style must be an object")
	}
	var v colleafStyleMap
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, err
	}
	return v, nil
}
func mergeColleafStyle(a, b colleafStyleMap) colleafStyleMap {
	out := colleafStyleMap{}
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		out[k] = v
	}
	return out
}

type colleafStyleResolver struct {
	named  map[string]colleafNamedStyle
	cache  map[string]colleafNamedStyle
	active map[string]bool
}

func (r *colleafStyleResolver) resolve(id string, depth int) (colleafNamedStyle, error) {
	if id == "" {
		return colleafNamedStyle{}, nil
	}
	if depth > 64 || r.active[id] {
		return colleafNamedStyle{}, fmt.Errorf("cyclic or excessive native style inheritance")
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
		a, err := colleafStyleObject(*pair[0])
		if err != nil {
			return s, err
		}
		b, err := colleafStyleObject(*pair[1])
		if err != nil {
			return s, err
		}
		*pair[1] = sfAttrs(mergeColleafStyle(a, b))
	}
	if r.cache == nil {
		r.cache = map[string]colleafNamedStyle{}
	}
	r.cache[id] = s
	return s, nil
}
func colleafBool(raw json.RawMessage) (bool, error) {
	var n *int
	if err := json.Unmarshal(raw, &n); err != nil || n == nil || (*n != 0 && *n != 1) {
		return false, fmt.Errorf("native BooleanNumber must be 0 or 1")
	}
	return *n == 1, nil
}
func colleafStyleRun(style colleafStyleMap, doc *Document) (Run, error) {
	var out Run
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
			doc.warn("colleaf_run_origin_context", "Native source properties outside direct text intervals do not assert run ownership")
		case "lang":
			if err := json.Unmarshal(raw, &out.Language); err != nil {
				return out, err
			}
		case "va":
			value, _, err := colleafRawInt(style, "va")
			if err != nil {
				return out, err
			}
			if value != 0 {
				doc.warn("colleaf_text_presentation", "Native baseline offset is not represented by ordinary SF text; visible text is retained")
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
			v, err := colleafBool(raw)
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
			show, err := colleafBool(decoration.Show)
			if err != nil {
				return out, err
			}
			if key == "ul" {
				out.Underline = show
			} else {
				out.Strike = show
			}
			decorationFields, err := colleafStyleObject(raw)
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
					doc.warn("colleaf_decoration", "Native decoration property "+field+" is not represented; enabled underline/strike and visible text are retained")
				}
			}
			if show && decoration.Type != nil && *decoration.Type != 12 {
				doc.warn("colleaf_decoration", "Native decoration pattern is normalized to a single line; visible text and enabled underline/strike are retained")
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
				out.Color, err = colleafRGB(*color.RGB)
				if err != nil {
					return out, err
				}
			} else if len(color.Theme) > 0 {
				return out, fmt.Errorf("%w: unresolved native theme color", ErrUnsupported)
			} else {
				out.Color = "auto"
			}
		default:
			doc.warn("colleaf_text_presentation", "Native text property "+key+" is not represented; supported typography and literal text are retained")
		}
	}
	return out, validateTextStyle(out)
}
func colleafRGB(color string) (string, error) {
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
func colleafRunStyle(run Run) (json.RawMessage, error) {
	if err := validateTextStyle(run); err != nil {
		return nil, err
	}
	if run.EastAsiaLanguage != "" || run.ComplexLanguage != "" || run.ComplexScript || run.Direction != "" {
		return nil, fmt.Errorf("%w: native nonvisual run annotations need the checked profile mapping", ErrUnsupported)
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
	origin, err := newColleafRunOrigin(run)
	if err != nil {
		return nil, err
	}
	if origin != nil {
		style["manydocOrigin"] = origin
	}
	return sfAttrs(style), nil
}
