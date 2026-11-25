package manifold

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

type colleafNoteProperties struct {
	Format       *string         `json:"numberFormat,omitempty"`
	Start        *int            `json:"startNumber,omitempty"`
	Restart      *string         `json:"restart,omitempty"`
	Position     *string         `json:"position,omitempty"`
	Columns      *int            `json:"columnCount,omitempty"`
	Separator    json.RawMessage `json:"separator,omitempty"`
	Continuation json.RawMessage `json:"continuationSeparator,omitempty"`
	Notice       json.RawMessage `json:"continuationNotice,omitempty"`
}
type colleafNoteSettings struct {
	Footnote colleafNoteProperties `json:"footnote,omitempty"`
	Endnote  colleafNoteProperties `json:"endnote,omitempty"`
}
type colleafNativeNoteMarks struct{ First, Last string }
type colleafNoteNumberPlan struct {
	Overrides   map[string]string
	Marks       map[string]colleafNativeNoteMarks
	Diagnostics []Diagnostic
}

func readColleafNoteSettings(raw json.RawMessage) (colleafNoteSettings, error) {
	var settings colleafNoteSettings
	if len(raw) == 0 {
		return settings, nil
	}
	if err := strictSFAttrs(raw, &settings); err != nil {
		return settings, err
	}
	var families map[string]map[string]json.RawMessage
	if err := json.Unmarshal(raw, &families); err != nil {
		return settings, err
	}
	for _, properties := range families {
		for key, value := range properties {
			if strings.TrimSpace(string(value)) == "null" && key != "separator" && key != "continuationSeparator" && key != "continuationNotice" {
				return settings, fmt.Errorf("native note property %q must not be null", key)
			}
		}
	}
	for kind, p := range map[string]colleafNoteProperties{Footnote: settings.Footnote, Endnote: settings.Endnote} {
		if p.Start != nil && (*p.Start < 0 || *p.Start > 2147483647) {
			return settings, fmt.Errorf("invalid native note start")
		}
		if p.Columns != nil && (*p.Columns < 0 || *p.Columns > 10000 || kind == Endnote) {
			return settings, fmt.Errorf("invalid native note columns")
		}
		if p.Restart != nil && *p.Restart != "continuous" && *p.Restart != "eachSect" && (*p.Restart != "eachPage" || kind == Endnote) {
			return settings, fmt.Errorf("invalid native note restart")
		}
		if p.Position != nil {
			good := kind == Footnote && (*p.Position == "pageBottom" || *p.Position == "beneathText") || kind == Endnote && (*p.Position == "docEnd" || *p.Position == "sectEnd")
			if !good {
				return settings, fmt.Errorf("invalid native note placement")
			}
		}
		for _, body := range []json.RawMessage{p.Separator, p.Continuation, p.Notice} {
			if len(body) > 0 && string(body) != "null" {
				return settings, fmt.Errorf("%w: native editable note separator/notice needs a visible-content mapping", ErrUnsupported)
			}
		}
	}
	return settings, nil
}
func colleafHasNoteSettings(s colleafNoteSettings) bool {
	for _, p := range []colleafNoteProperties{s.Footnote, s.Endnote} {
		if p.Format != nil || p.Start != nil || p.Restart != nil || p.Position != nil || p.Columns != nil {
			return true
		}
	}
	return false
}
func mergeColleafNoteProperties(base, local colleafNoteProperties) colleafNoteProperties {
	if local.Format != nil {
		base.Format = local.Format
	}
	if local.Start != nil {
		base.Start = local.Start
	}
	if local.Restart != nil {
		base.Restart = local.Restart
	}
	if local.Position != nil {
		base.Position = local.Position
	}
	if local.Columns != nil {
		base.Columns = local.Columns
	}
	return base
}

// Number formatting is independently authored from the published format names
// and executable outcomes, including native zero rules that differ from SF lists.
func colleafNoteNumber(format string, value int) (string, error) {
	if value < 0 || value > 2147483647 {
		return "", fmt.Errorf("native note counter outside supported range")
	}
	if format == "none" {
		return "", nil
	}
	if format == "decimalZero" && value < 10 {
		return "0" + strconv.Itoa(value), nil
	}
	if value == 0 && (format == "upperRoman" || format == "lowerRoman" || format == "upperLetter" || format == "lowerLetter") {
		return "", nil
	}
	if supportedNumberFormat(format) {
		return numberText(format, value)
	}
	if format == "chicago" {
		symbols := []string{"*", "†", "‡", "§", "‖", "¶"}
		index := max(0, value-1)
		if index/6 >= 4096 {
			return "", fmt.Errorf("%w: native note mark expansion exceeds supported label size", ErrUnsupported)
		}
		return strings.Repeat(symbols[index%6], index/6+1), nil
	}
	digits := ""
	switch format {
	case "decimalFullWidth", "decimalFullWidth2":
		digits = "０１２３４５６７８９"
	case "ideographDigital", "taiwaneseDigital":
		digits = "〇一二三四五六七八九"
	}
	if digits != "" {
		alphabet := []rune(digits)
		var b strings.Builder
		for _, ch := range strconv.Itoa(value) {
			b.WriteRune(alphabet[ch-'0'])
		}
		return b.String(), nil
	}
	if format == "decimalEnclosedCircle" {
		if value == 0 {
			return "⓪", nil
		}
		if value <= 20 {
			return string(rune(0x2460 + value - 1)), nil
		}
		if value <= 35 {
			return string(rune(0x3251 + value - 21)), nil
		}
		if value <= 50 {
			return string(rune(0x32b1 + value - 36)), nil
		}
		return strconv.Itoa(value), nil
	}
	if format == "ideographEnclosedCircle" {
		if value >= 1 && value <= 10 {
			return string(rune(0x3220 + value - 1)), nil
		}
		return strconv.Itoa(value), nil
	}
	if format == "ideographTraditional" || format == "ideographZodiac" {
		alphabet := []rune("甲乙丙丁戊己庚辛壬癸")
		if format == "ideographZodiac" {
			alphabet = []rune("子丑寅卯辰巳午未申酉戌亥")
		}
		if value >= 1 && value <= len(alphabet) {
			return string(alphabet[value-1]), nil
		}
		return strconv.Itoa(value), nil
	}
	return "", fmt.Errorf("%w: native note number format %q needs mapping", ErrUnsupported, format)
}

func planColleafNoteNumbers(ctx context.Context, input colleafNoteSnapshot) (colleafNoteNumberPlan, error) {
	plan := colleafNoteNumberPlan{Overrides: map[string]string{}, Marks: map[string]colleafNativeNoteMarks{}}
	settings, err := readColleafNoteSettings(input.NoteSettings)
	if err != nil {
		return plan, err
	}
	index, err := indexColleafBody(ctx, input.Body)
	if err != nil {
		return plan, err
	}
	records := map[int]colleafSectionRecord{}
	for _, record := range input.Body.SectionBreaks {
		records[record.StartIndex] = record
	}
	sectionSettings := make([]colleafNoteSettings, len(index.sections))
	settingsPresent := colleafHasNoteSettings(settings)
	for i, node := range index.sections {
		raw := records[node.end-1].Extra["noteProperties"]
		sectionSettings[i], err = readColleafNoteSettings(raw)
		if err != nil {
			return plan, err
		}
		settingsPresent = settingsPresent || colleafHasNoteSettings(sectionSettings[i])
	}
	type counter struct {
		Next, Section int
		Seen          bool
	}
	counters := map[string]counter{}
	previous := -1
	section := 0
	labels := map[string]string{}
	sourceIDs := map[string]string{}
	for _, a := range input.Body.CustomRanges {
		if err = ctx.Err(); err != nil {
			return plan, err
		}
		if a.Type != 8 && a.Type != 9 {
			continue
		}
		if a.StartIndex <= previous {
			return plan, fmt.Errorf("native note ranges must follow scalar document order")
		}
		previous = a.StartIndex
		atom := index.blocks[a.StartIndex]
		note, ok := input.Notes[atom.NoteID]
		if !ok || note.Kind != atom.NoteType {
			return plan, fmt.Errorf("native note reference lacks matching definition")
		}
		for section < len(index.sections)-1 && a.StartIndex >= index.sections[section].end {
			section++
		}
		props := mergeColleafNoteProperties(settings.Footnote, sectionSettings[section].Footnote)
		format := "decimal"
		if note.Kind == Endnote {
			props = mergeColleafNoteProperties(settings.Endnote, sectionSettings[section].Endnote)
			format = "lowerRoman"
		}
		if props.Format != nil {
			format = *props.Format
		}
		state := counters[note.Kind]
		if !state.Seen || (props.Restart != nil && *props.Restart == "eachSect" && state.Section != section) {
			state.Next = 1
			if props.Start != nil {
				state.Next = *props.Start
			}
		}
		label := ""
		if note.Mark != nil {
			label = *note.Mark
		} else {
			if props.Restart != nil && *props.Restart == "eachPage" {
				return plan, fmt.Errorf("%w: automatic native per-page note numbering requires authoritative page layout", ErrUnsupported)
			}
			label, err = colleafNoteNumber(format, state.Next)
			if err != nil {
				return plan, err
			}
			state.Next++
		}
		state.Seen = true
		state.Section = section
		counters[note.Kind] = state
		if err = validateNoteLabel(label); err != nil {
			return plan, err
		}
		// Existing custom marks already have a faithful literal definition mapping.
		// Only implicit/empty marks need the additional citation/body distinctions.
		if note.Mark != nil && label != "" {
			continue
		}
		marks, seen := plan.Marks[note.ID]
		if !seen {
			marks.First = label
		}
		marks.Last = label
		plan.Marks[note.ID] = marks
		labels[a.ID] = label
		sourceIDs[a.ID] = note.ID
	}
	for id, label := range labels {
		if label == "" || label != plan.Marks[sourceIDs[id]].First {
			plan.Overrides[id] = label
		}
	}
	if len(plan.Marks) > 0 {
		plan.Diagnostics = append(plan.Diagnostics, Diagnostic{Code: "colleaf_note_numbering", Message: "Native automatic or markerless note marks are fixed to their current literal values; repeated citations and body marks are retained, while live counter behavior is not represented"})
	}
	if settingsPresent {
		plan.Diagnostics = append(plan.Diagnostics, Diagnostic{Code: "colleaf_note_settings", Message: "Native note numbering, placement and column policies are normalized to current marks and SF foot/endnote kinds; live restart and placement settings are not represented"})
	}
	return plan, nil
}
func applyColleafNoteNumbers(doc *Document, plan colleafNoteNumberPlan, ids map[string]noteKey) {
	notes := map[noteKey]*Note{}
	for i := range doc.Notes {
		notes[noteIdentity(doc.Notes[i])] = &doc.Notes[i]
	}
	for nativeID, marks := range plan.Marks {
		if n := notes[ids[nativeID]]; n != nil {
			n.Label = marks.First
			if n.BodyLabel == nil && (marks.First != marks.Last || marks.Last == "") {
				label := marks.Last
				n.BodyLabel = &label
			}
		}
	}
	doc.Diagnostics = append(doc.Diagnostics, plan.Diagnostics...)
}
