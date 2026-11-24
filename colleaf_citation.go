package manydoc

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
)

const colleafCitationProfile = "manydoc.note-citation"
const colleafCitationEnd = "\u2060"

type colleafCitationMark struct {
	Token         string `json:"token"`
	Profile       string `json:"profile"`
	Version       int    `json:"version"`
	TextColorMode string `json:"textColorMode"`
	NoteID        string `json:"noteId"`
	Mode          string `json:"mode"`
	Inferred      bool   `json:"inferred,omitempty"`
}
type colleafCitationScope struct {
	colleafCitationMark
	Take int
}
type colleafCitationPending struct{ Scope colleafCitationScope }
type colleafCitationState struct {
	Scopes       map[string]colleafCitationScope
	Pending      map[*NoteReference]colleafCitationPending
	Definitions  map[string]string
	BodyDefaults map[noteKey]bool
	Diagnostics  []Diagnostic
}

func openColleafCitations(ctx context.Context, input colleafNoteSnapshot) (colleafNoteSnapshot, *colleafCitationState, error) {
	state := &colleafCitationState{Scopes: map[string]colleafCitationScope{}, Pending: map[*NoteReference]colleafCitationPending{}, Definitions: map[string]string{}, BodyDefaults: map[noteKey]bool{}}
	var err error
	var identities colleafNoteIdentities
	if err := colleafResourceMap(input.Resources, colleafNoteIdentityResource, &identities); err != nil {
		return input, nil, err
	}
	for _, b := range identities.Bindings {
		if b.LiteralCitations != nil {
			if n, ok := input.Notes[b.NativeID]; ok && n.Kind == b.Kind && n.Mark != nil && *n.Mark == "" {
				state.Definitions[b.NativeID] = *b.LiteralCitations
			}
		}
	}
	// Validate original fragment identities/bounds before combining them, so
	// normalization cannot conceal a malformed individual native record.
	index, err := indexColleafBody(ctx, input.Body)
	if err != nil {
		return input, nil, err
	}
	input.Body, err = joinColleafCitationFragments(input.Body)
	if err != nil {
		return input, nil, err
	}
	input.Body.TextRuns = append([]colleafTextRun(nil), input.Body.TextRuns...)
	originalRuns := input.Body.TextRuns
	spans, err := colleafStyleSpans(ctx, index.text, originalRuns)
	if err != nil {
		return input, nil, err
	}
	var atoms []int
	for position := range index.blocks {
		atoms = append(atoms, position)
	}
	sort.Ints(atoms)

	var ranges []colleafRangeRecord
	for _, a := range input.Body.CustomRanges {
		var marker struct {
			Profile string `json:"profile"`
		}
		_ = json.Unmarshal(a.Properties, &marker)
		if a.Type != 5 || marker.Profile != colleafCitationProfile {
			ranges = append(ranges, a)
			continue
		}
		var hint colleafCitationMark
		if err = strictSFAttrs(a.Properties, &hint); err != nil {
			return input, nil, err
		}
		if hint.Version != 1 || (hint.Mode != "default" && hint.Mode != "literal" && hint.Mode != "rich") || a.Active == nil || *a.Active || hint.TextColorMode != "text" {
			return input, nil, fmt.Errorf("%w: native citation ownership version/mode/appearance", ErrUnsupported)
		}
		text, err := index.text.inclusive(a.StartIndex, a.EndIndex)
		if err != nil {
			return input, nil, err
		}
		atom, ok := index.blocks[a.StartIndex]
		n, hasNote := input.Notes[hint.NoteID]
		if !ok || !hasNote || atom.NoteID != hint.NoteID || n.Mark == nil || *n.Mark != "" || !strings.HasPrefix(text, "\ufffc") || !strings.HasSuffix(text, colleafCitationEnd) || strings.ContainsAny(text, "\r\n\t\b\f\v") {
			state.Diagnostics = append(state.Diagnostics, Diagnostic{Code: "colleaf_stale_citation", Message: "Stale native citation ownership ignored; actual note anchor and visible label text are retained"})
			continue
		}
		if _, exists := state.Scopes[atom.ID]; exists {
			return input, nil, fmt.Errorf("duplicate native citation ownership")
		}
		// Effective spans retain the native last-record-wins rule. Binary search
		// avoids rescanning all text records for each citation in a large body.
		if i := sort.SearchInts(atoms, a.StartIndex+1); i < len(atoms) && atoms[i] <= a.EndIndex {
			return input, nil, fmt.Errorf("native citation ownership crosses another atom")
		}
		first := sort.Search(len(spans), func(i int) bool { return spans[i].end > a.StartIndex+1 })
		valid := true
		for _, span := range spans[first:] {
			if span.start >= a.EndIndex {
				break
			}
			if span.record < 0 {
				valid = false
				continue
			}
			style, err := colleafStyleObject(originalRuns[span.record].TS)
			if err != nil {
				return input, nil, err
			}
			if raw, ok := style["va"]; !ok || string(raw) != "3" {
				valid = false
			}
		}
		if !valid {
			state.Diagnostics = append(state.Diagnostics, Diagnostic{Code: "colleaf_stale_citation", Message: "Edited native citation baseline is retained as visible story text rather than hidden by stale ownership"})
			continue
		}
		for _, span := range spans[first:] {
			if span.start >= a.EndIndex {
				break
			}
			r := originalRuns[span.record]
			r.St = max(span.start, a.StartIndex+1)
			r.Ed = min(span.end, a.EndIndex)
			style, _ := colleafStyleObject(r.TS)
			style["va"] = json.RawMessage("0")
			r.TS = sfAttrs(style)
			input.Body.TextRuns = append(input.Body.TextRuns, r)
		}
		state.Scopes[atom.ID] = colleafCitationScope{colleafCitationMark: hint, Take: a.EndIndex - a.StartIndex}
	}
	for _, atom := range index.blocks {
		if _, projected := state.Definitions[atom.NoteID]; projected {
			if _, owned := state.Scopes[atom.ID]; !owned {
				state.Diagnostics = append(state.Diagnostics, Diagnostic{Code: "colleaf_stale_citation", Message: "Expected native citation ownership is absent or stale; actual note anchor and visible label text are retained"})
			}
		}
	}
	input.Body.CustomRanges = ranges
	return input, state, nil
}
func (s *colleafCitationState) adaptPlan(plan *colleafNoteNumberPlan) {
	for id := range s.Definitions {
		delete(plan.Marks, id)
	}
	if len(plan.Marks) == 0 {
		var keep []Diagnostic
		for _, d := range plan.Diagnostics {
			if d.Code != "colleaf_note_numbering" {
				keep = append(keep, d)
			}
		}
		plan.Diagnostics = keep
	}
}
func (s *colleafCitationState) restoreDefinitions(doc *Document, ids map[string]noteKey) {
	lookup := map[noteKey]*Note{}
	for i := range doc.Notes {
		lookup[noteIdentity(doc.Notes[i])] = &doc.Notes[i]
	}
	for id, label := range s.Definitions {
		if n := lookup[ids[id]]; n != nil {
			n.Label = label
		}
	}
}
func (s *colleafCitationState) collapse(ctx context.Context, doc *Document) error {
	var walk func([]Block) error
	walk = func(bs []Block) error {
		for bi := range bs {
			b := &bs[bi]
			working := append([]Run(nil), b.Runs...)
			var out []Run
			for i := 0; i < len(working); i++ {
				run := working[i]
				pending, ok := s.Pending[run.Note]
				if !ok {
					out = append(out, run)
					continue
				}
				label, next, tail, err := takeColleafCitationRuns(ctx, working, i+1, pending.Scope.Take)
				if err != nil {
					return err
				}
				text := noteLabelContentText(label)
				if !strings.HasSuffix(text, colleafCitationEnd) {
					return fmt.Errorf("native citation extent lost its owned delimiter")
				}
				label, _, err = splitColleafRuns(ctx, label, colleafUTF16Len(text)-1)
				if err != nil {
					return err
				}
				text = noteLabelContentText(label)
				valid := validateNoteLabel(text) == nil
				for _, r := range label {
					valid = valid && r.Note == nil && r.Link == "" && r.Destination == "" && r.Anchor == ""
				}
				if !valid {
					doc.warn("colleaf_stale_citation", "Edited native citation text is retained in story flow because it is no longer a supported single-line mark")
					out = append(out, run)
					continue
				}
				ref := run.Note
				ref.Label = nil
				ref.LabelContent = nil
				ref.Inferred = pending.Scope.Inferred
				if pending.Scope.Mode == "rich" {
					if text == "" {
						empty := ""
						ref.Label = &empty
						doc.warn("colleaf_citation_edit", "Empty edited rich citation retained as markerless")
					} else {
						ref.LabelContent = label
					}
				} else {
					uniform := true
					base := run
					base.Note = nil
					base.Text = ""
					base.colleafRunOrigin = nil
					for _, r := range label {
						r.Text = ""
						r.colleafRunOrigin = nil
						uniform = uniform && reflect.DeepEqual(r, base)
					}
					if uniform {
						value := text
						ref.Label = &value
					} else {
						ref.LabelContent = label
						doc.warn("colleaf_citation_edit", "Edited native citation formatting retained as rich label content")
					}
				}
				out = append(out, run)
				if tail != nil {
					working[next] = *tail
				}
				i = next - 1
			}
			if b.Runs != nil {
				b.Runs = out
			}
			if b.Section != nil {
				if err := walk(b.Section.Blocks); err != nil {
					return err
				}
			}
			for i := range b.Items {
				if b.Items[i].Marker != nil {
					if err := walk(b.Items[i].Marker.Blocks); err != nil {
						return err
					}
				}
				if err := walk(b.Items[i].Blocks); err != nil {
					return err
				}
			}
			for ri := range b.Rows {
				for ci := range b.Rows[ri].Cells {
					if err := walk(b.Rows[ri].Cells[ci].Blocks); err != nil {
						return err
					}
				}
			}
		}
		return nil
	}
	if len(s.Pending) > 0 {
		if err := walk(doc.Blocks); err != nil {
			return err
		}
	}
	catalog, err := catalogNotes(doc, false)
	if err != nil {
		return err
	}
	for ref, p := range s.Pending {
		if p.Scope.Mode == "default" && ref.Label != nil {
			if *ref.Label == catalog.labels[referenceKey(ref)] {
				ref.Label = nil
			} else {
				doc.warn("colleaf_citation_default_edit", "Actual citation mark retained literally because the current shared default no longer agrees")
			}
		}
	}
	for i := range doc.Notes {
		n := &doc.Notes[i]
		if s.BodyDefaults[noteIdentity(*n)] && n.BodyLabel != nil {
			if *n.BodyLabel == catalog.labels[noteIdentity(*n)] {
				n.BodyLabel = nil
			} else {
				doc.warn("colleaf_citation_default_edit", "Actual body mark retained literally because the current shared default no longer agrees")
			}
		}
	}
	for _, d := range s.Diagnostics {
		doc.warn(d.Code, d.Message)
	}
	return nil
}

func colleafCitationProjection(d *Document, catalog *noteCatalog) map[noteKey]string {
	out := map[noteKey]string{}
	_ = walkNoteRuns(d.Blocks, func(r Run) error {
		if r.Note != nil && (r.Note.Label != nil || r.Note.LabelContent != nil || r.Note.Inferred) {
			out[referenceKey(r.Note)] = catalog.labels[referenceKey(r.Note)]
		}
		return nil
	})
	return out
}

func writeColleafCitation(ctx context.Context, run Run, id, mode string, label []Run) (colleafBody, error) {
	fragment, err := encodeColleafTextBodyWithNotes(ctx, []Block{{Kind: Paragraph, Runs: label}}, nil)
	if err != nil {
		return fragment, err
	}
	fragment.DataStream = strings.TrimSuffix(fragment.DataStream, "\r\n") + colleafCitationEnd
	length := colleafUTF16Len(fragment.DataStream)
	fragment.Paragraphs = nil
	fragment.SectionBreaks = nil
	for i := range fragment.TextRuns {
		r := &fragment.TextRuns[i]
		r.St++
		r.Ed++
		style, err := colleafStyleObject(r.TS)
		if err != nil {
			return fragment, err
		}
		style["va"] = json.RawMessage("3")
		r.TS = sfAttrs(style)
	}
	for i := range fragment.CustomRanges {
		fragment.CustomRanges[i].StartIndex++
		fragment.CustomRanges[i].EndIndex++
	}
	anchorStyle, err := colleafRunStyle(run)
	if err != nil {
		return fragment, err
	}
	fragment.TextRuns = append([]colleafTextRun{{St: 0, Ed: 1, TS: anchorStyle}}, fragment.TextRuns...)
	fragment.DataStream = "\ufffc" + fragment.DataStream
	kind := 8
	if run.Note.Kind == Endnote {
		kind = 9
	}
	inactive := false
	fragment.CustomRanges = append([]colleafRangeRecord{{ID: "anchor", Type: kind, StartIndex: 0, EndIndex: 0, Whole: true, Properties: sfAttrs(map[string]string{"noteId": id})}}, fragment.CustomRanges...)
	fragment.CustomRanges = append(fragment.CustomRanges, colleafRangeRecord{ID: "owner", Type: 5, Active: &inactive, StartIndex: 0, EndIndex: length, Properties: sfAttrs(colleafCitationMark{Profile: colleafCitationProfile, Version: 1, TextColorMode: "text", NoteID: id, Mode: mode, Inferred: run.Note.Inferred})})
	return fragment, nil
}

// Walk each paragraph once; do not repeatedly copy the whole remainder for every
// citation in a long paragraph. A partial last run is committed only on success.
func takeColleafCitationRuns(ctx context.Context, runs []Run, start, count int) ([]Run, int, *Run, error) {
	var label []Run
	for i := start; i < len(runs); i++ {
		if count == 0 {
			return label, i, nil, nil
		}
		r := runs[i]
		if r.Note != nil {
			return nil, 0, nil, fmt.Errorf("native citation ownership crosses another note anchor")
		}
		size := colleafUTF16Len(r.Text)
		if size <= count {
			label = append(label, r)
			count -= size
			continue
		}
		index, err := newColleafTextIndex(ctx, r.Text)
		if err != nil {
			return nil, 0, nil, err
		}
		left, right := r, r
		left.Text, err = index.exclusive(0, count)
		if err != nil {
			return nil, 0, nil, err
		}
		right.Text, err = index.exclusive(count, size)
		if err != nil {
			return nil, 0, nil, err
		}
		label = append(label, left)
		return label, i, &right, nil
	}
	if count != 0 {
		return nil, 0, nil, fmt.Errorf("native citation ownership exceeds its paragraph")
	}
	return label, len(runs), nil, nil
}

func nameColleafCitationRange(a *colleafRangeRecord) {
	var hint colleafCitationMark
	if a.Type == 5 && json.Unmarshal(a.Properties, &hint) == nil && hint.Profile == colleafCitationProfile {
		hint.Token = a.ID
		a.Properties = sfAttrs(hint)
	}
}

// Native TextX may split an owned range around replacement text. Its copied
// token/properties, protected anchor and terminal delimiter retain the one label.
func joinColleafCitationFragments(body colleafBody) (colleafBody, error) {
	var out []colleafRangeRecord
	groups := map[string]int{}
	for _, a := range body.CustomRanges {
		var hint colleafCitationMark
		if a.Type != 5 || json.Unmarshal(a.Properties, &hint) != nil || hint.Profile != colleafCitationProfile {
			out = append(out, a)
			continue
		}
		if hint.Token == "" {
			return body, fmt.Errorf("native citation ownership lacks stable token")
		}
		if i, ok := groups[hint.Token]; ok {
			var old colleafCitationMark
			_ = json.Unmarshal(out[i].Properties, &old)
			if !reflect.DeepEqual(old, hint) || !reflect.DeepEqual(out[i].Active, a.Active) {
				return body, fmt.Errorf("conflicting native citation fragments")
			}
			out[i].StartIndex = min(out[i].StartIndex, a.StartIndex)
			out[i].EndIndex = max(out[i].EndIndex, a.EndIndex)
		} else {
			groups[hint.Token] = len(out)
			out = append(out, a)
		}
	}
	body.CustomRanges = out
	return body, nil
}
