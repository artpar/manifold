package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
)

const vdocsCitationProfile = "manydoc.note-citation"
const vdocsCitationEnd = "\u2060"

type vdocsCitationMark struct {
	Token         string `json:"token"`
	Profile       string `json:"profile"`
	Version       int    `json:"version"`
	TextColorMode string `json:"textColorMode"`
	NoteID        string `json:"noteId"`
	Mode          string `json:"mode"`
	Inferred      bool   `json:"inferred,omitempty"`
}
type vdocsCitationScope struct {
	vdocsCitationMark
	Take int
}
type vdocsCitationPending struct{ Scope vdocsCitationScope }
type vdocsCitationState struct {
	Scopes       map[string]vdocsCitationScope
	Pending      map[*NoteReference]vdocsCitationPending
	Definitions  map[string]string
	BodyDefaults map[noteKey]bool
	Diagnostics  []Diagnostic
}

func openColleafCitations(ctx context.Context, input vdocsNoteSnapshot) (vdocsNoteSnapshot, *vdocsCitationState, error) {
	state := &vdocsCitationState{Scopes: map[string]vdocsCitationScope{}, Pending: map[*NoteReference]vdocsCitationPending{}, Definitions: map[string]string{}, BodyDefaults: map[noteKey]bool{}}
	var err error
	var identities vdocsNoteIdentities
	if err := vdocsResourceMap(input.Resources, vdocsNoteIdentityResource, &identities); err != nil {
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
	input.Body.TextRuns = append([]vdocsTextRun(nil), input.Body.TextRuns...)
	originalRuns := input.Body.TextRuns
	spans, err := vdocsStyleSpans(ctx, index.text, originalRuns)
	if err != nil {
		return input, nil, err
	}
	var atoms []int
	for position := range index.blocks {
		atoms = append(atoms, position)
	}
	sort.Ints(atoms)

	var ranges []vdocsRangeRecord
	for _, a := range input.Body.CustomRanges {
		var marker struct {
			Profile string `json:"profile"`
		}
		_ = json.Unmarshal(a.Properties, &marker)
		if a.Type != 5 || marker.Profile != vdocsCitationProfile {
			ranges = append(ranges, a)
			continue
		}
		var hint vdocsCitationMark
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
		if !ok || !hasNote || atom.NoteID != hint.NoteID || n.Mark == nil || *n.Mark != "" || !strings.HasPrefix(text, "\ufffc") || !strings.HasSuffix(text, vdocsCitationEnd) || strings.ContainsAny(text, "\r\n\t\b\f\v") {
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
			style, err := vdocsStyleObject(originalRuns[span.record].TS)
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
			style, _ := vdocsStyleObject(r.TS)
			style["va"] = json.RawMessage("0")
			r.TS = sfAttrs(style)
			input.Body.TextRuns = append(input.Body.TextRuns, r)
		}
		state.Scopes[atom.ID] = vdocsCitationScope{vdocsCitationMark: hint, Take: a.EndIndex - a.StartIndex}
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
func (s *vdocsCitationState) adaptPlan(plan *vdocsNoteNumberPlan) {
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
func (s *vdocsCitationState) restoreDefinitions(doc *Document, ids map[string]noteKey) {
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
func (s *vdocsCitationState) collapse(ctx context.Context, doc *Document) error {
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
				if !strings.HasSuffix(text, vdocsCitationEnd) {
					return fmt.Errorf("native citation extent lost its owned delimiter")
				}
				label, _, err = splitColleafRuns(ctx, label, vdocsUTF16Len(text)-1)
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
					base.vdocsRunOrigin = nil
					for _, r := range label {
						r.Text = ""
						r.vdocsRunOrigin = nil
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

func vdocsCitationProjection(d *Document, catalog *noteCatalog) map[noteKey]string {
	out := map[noteKey]string{}
	_ = walkNoteRuns(d.Blocks, func(r Run) error {
		if r.Note != nil && (r.Note.Label != nil || r.Note.LabelContent != nil || r.Note.Inferred) {
			out[referenceKey(r.Note)] = catalog.labels[referenceKey(r.Note)]
		}
		return nil
	})
	return out
}

func writeColleafCitation(ctx context.Context, run Run, id, mode string, label []Run) (vdocsBody, error) {
	fragment, err := encodeColleafTextBodyWithNotes(ctx, []Block{{Kind: Paragraph, Runs: label}}, nil)
	if err != nil {
		return fragment, err
	}
	fragment.DataStream = strings.TrimSuffix(fragment.DataStream, "\r\n") + vdocsCitationEnd
	length := vdocsUTF16Len(fragment.DataStream)
	fragment.Paragraphs = nil
	fragment.SectionBreaks = nil
	for i := range fragment.TextRuns {
		r := &fragment.TextRuns[i]
		r.St++
		r.Ed++
		style, err := vdocsStyleObject(r.TS)
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
	anchorStyle, err := vdocsRunStyle(run)
	if err != nil {
		return fragment, err
	}
	fragment.TextRuns = append([]vdocsTextRun{{St: 0, Ed: 1, TS: anchorStyle}}, fragment.TextRuns...)
	fragment.DataStream = "\ufffc" + fragment.DataStream
	kind := 8
	if run.Note.Kind == Endnote {
		kind = 9
	}
	inactive := false
	fragment.CustomRanges = append([]vdocsRangeRecord{{ID: "anchor", Type: kind, StartIndex: 0, EndIndex: 0, Whole: true, Properties: sfAttrs(map[string]string{"noteId": id})}}, fragment.CustomRanges...)
	fragment.CustomRanges = append(fragment.CustomRanges, vdocsRangeRecord{ID: "owner", Type: 5, Active: &inactive, StartIndex: 0, EndIndex: length, Properties: sfAttrs(vdocsCitationMark{Profile: vdocsCitationProfile, Version: 1, TextColorMode: "text", NoteID: id, Mode: mode, Inferred: run.Note.Inferred})})
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
		size := vdocsUTF16Len(r.Text)
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

func nameColleafCitationRange(a *vdocsRangeRecord) {
	var hint vdocsCitationMark
	if a.Type == 5 && json.Unmarshal(a.Properties, &hint) == nil && hint.Profile == vdocsCitationProfile {
		hint.Token = a.ID
		a.Properties = sfAttrs(hint)
	}
}

// Native TextX may split an owned range around replacement text. Its copied
// token/properties, protected anchor and terminal delimiter retain the one label.

// The existing native markerless representation needs no ownership annotation.

// real referenceTextStyle was decoded normally, without text caches

// TextX replacement can detach the range's start from newly inserted label
// text. Paragraph identity and the owned terminal tab bind the leading mark;
// using End retains edits without guessing at stale original text.

// Commit the structural collapse only after the whole ownership guard passes.

// Number formatting is independently authored from the published format names
// and executable outcomes, including native zero rules that differ from SF lists.

// Existing custom marks already have a faithful literal definition mapping.
// Only implicit/empty marks need the additional citation/body distinctions.

// Note source records bind to checked native identity metadata, never to a note
// array position or to a cached copy of the editable body or labels.

// Fingerprints describe current SF semantics, excluding provenance. This runs
// only on independently encoded SF, not arbitrary native plugin objects.

// Native note IDs share one namespace; SF identities are scoped by kind. This
// sparse resource binds identities/order only, never another copy of note text.

// A stale identity record cannot override an edited native ID or kind. It
// simply ceases to apply; actual native notes/references remain authoritative.

// The schema permits the matching body-number atom at offset zero.
// It is the note's body marker, not a recursive citation to itself.

func joinColleafCitationFragments(body vdocsBody) (vdocsBody, error) {
	var out []vdocsRangeRecord
	groups := map[string]int{}
	for _, a := range body.CustomRanges {
		var hint vdocsCitationMark
		if a.Type != 5 || json.Unmarshal(a.Properties, &hint) != nil || hint.Profile != vdocsCitationProfile {
			out = append(out, a)
			continue
		}
		if hint.Token == "" {
			return body, fmt.Errorf("native citation ownership lacks stable token")
		}
		if i, ok := groups[hint.Token]; ok {
			var old vdocsCitationMark
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

const vdocsBodyMarkProfile = "manydoc.note-body-mark"

type vdocsBodyMark struct {
	TextColorMode string `json:"textColorMode"`
	Profile       string `json:"profile"`
	Version       int    `json:"version"`
	NoteID        string `json:"noteId"`
	Mode          string `json:"mode"`
	ParagraphID   string `json:"paragraphId,omitempty"`
	Separate      bool   `json:"separate,omitempty"`
	UseDefault    bool   `json:"useDefault,omitempty"`
	Empty         bool   `json:"empty,omitempty"`
}
type vdocsBodyMarkScope struct {
	vdocsBodyMark
	Start, End int
	RangeID    string
	Restored   bool
}

func prepareColleafBodyMark(n Note, catalog *noteCatalog) ([]Block, *vdocsBodyMark, int, error) {
	if n.BodyLabel == nil && n.BodyLabelContent == nil && n.BodyLabelStyle == nil {
		return n.Blocks, nil, 0, nil
	}
	runs := noteBodyLabelRuns(catalog, noteIdentity(n))
	text := noteLabelContentText(runs)

	if text == "" && n.BodyLabelStyle == nil {
		return n.Blocks, nil, 0, nil
	}
	mode := "literal"
	if n.BodyLabelContent != nil {
		mode = "rich"
	} else if n.BodyLabelStyle != nil {
		mode = "style"
	}
	hint := &vdocsBodyMark{Profile: vdocsBodyMarkProfile, Version: 1, TextColorMode: "text", Mode: mode, UseDefault: n.BodyLabel == nil && n.BodyLabelContent == nil, Empty: text == ""}
	if hint.Empty {
		if n.BodyLabelStyle.Link != "" || n.BodyLabelStyle.Destination != "" {
			return nil, nil, 0, fmt.Errorf("%w: native empty body-mark navigation has no visible anchor", ErrUnsupported)
		}
		return n.Blocks, hint, 0, nil
	}
	for _, r := range runs {
		if _, err := vdocsRunStyle(r); err != nil {
			return nil, nil, 0, err
		}
	}
	blocks := append([]Block(nil), n.Blocks...)
	prefix := append([]Run(nil), runs...)
	prefix = append(prefix, Run{Text: "\t"})
	if len(blocks) > 0 && (blocks[0].Kind == Paragraph || blocks[0].Kind == Heading) {
		if blocks[0].Source != nil {
			origin, err := newColleafOrigin(blocks[0])
			if err != nil {
				return nil, nil, 0, err
			}
			blocks[0].vdocsOrigin = origin
		}
		prepared, err := prepareColleafAnchorOrigins(blocks[0])
		if err != nil {
			return nil, nil, 0, err
		}
		blocks[0] = prepared
		blocks[0].Runs = append(prefix, blocks[0].Runs...)
	} else {
		hint.Separate = true
		blocks = append([]Block{{Kind: Paragraph, Runs: prefix}}, blocks...)
	}
	return blocks, hint, vdocsUTF16Len(text) + 1, nil
}
func sealColleafBodyMark(n *vdocsNativeNote, hint vdocsBodyMark, length int) error {
	hint.NoteID = n.ID
	start, end := 0, length-1
	if hint.Empty {
		start = vdocsUTF16Len(n.Body.DataStream) - 1
		end = start
	} else {
		if len(n.Body.Paragraphs) == 0 {
			return fmt.Errorf("native body mark lacks paragraph")
		}
		hint.ParagraphID = n.Body.Paragraphs[0].ID
	}
	inactive := false
	n.Body.CustomRanges = append(n.Body.CustomRanges, vdocsRangeRecord{StartIndex: start, EndIndex: end, ID: "manydoc-body-mark-" + n.ID, Type: 5, Active: &inactive, Properties: sfAttrs(hint)})
	hidden := false
	n.ShowBodyReference = &hidden
	return nil
}
func openColleafBodyMark(ctx context.Context, n vdocsNativeNote) (vdocsNativeNote, *vdocsBodyMarkScope, error) {
	var scope *vdocsBodyMarkScope
	var ranges []vdocsRangeRecord
	for _, a := range n.Body.CustomRanges {
		var marker struct {
			Profile string `json:"profile"`
		}
		_ = json.Unmarshal(a.Properties, &marker)
		if a.Type != 5 || marker.Profile != vdocsBodyMarkProfile {
			ranges = append(ranges, a)
			continue
		}
		if scope != nil {
			return n, nil, fmt.Errorf("duplicate native note body-mark ownership")
		}
		var hint vdocsBodyMark
		if err := strictSFAttrs(a.Properties, &hint); err != nil {
			return n, nil, err
		}
		if hint.Version != 1 || (hint.Mode != "literal" && hint.Mode != "style" && hint.Mode != "rich") {
			return n, nil, fmt.Errorf("%w: native body-mark ownership version/mode", ErrUnsupported)
		}
		if a.Active == nil || *a.Active || hint.TextColorMode != "text" {
			return n, nil, fmt.Errorf("%w: native body-mark ownership appearance is no longer neutral", ErrUnsupported)
		}
		index, err := newColleafTextIndex(ctx, n.Body.DataStream)
		if err != nil {
			return n, nil, err
		}
		if _, err = index.inclusive(a.StartIndex, a.EndIndex); err != nil {
			return n, nil, err
		}
		scope = &vdocsBodyMarkScope{vdocsBodyMark: hint, Start: a.StartIndex, End: a.EndIndex, RangeID: a.ID}
	}
	n.Body.CustomRanges = ranges
	return n, scope, nil
}
func splitColleafRuns(ctx context.Context, runs []Run, cut int) ([]Run, []Run, error) {
	var before, after []Run
	for _, r := range runs {
		if cut <= 0 {
			after = append(after, r)
			continue
		}
		if r.Note != nil {
			return nil, nil, fmt.Errorf("native body mark contains recursive note")
		}
		size := vdocsUTF16Len(r.Text)
		if size <= cut {
			before = append(before, r)
			cut -= size
			continue
		}
		index, err := newColleafTextIndex(ctx, r.Text)
		if err != nil {
			return nil, nil, err
		}
		left, right := r, r
		left.Text, err = index.exclusive(0, cut)
		if err != nil {
			return nil, nil, err
		}
		right.Text, err = index.exclusive(cut, size)
		if err != nil {
			return nil, nil, err
		}
		before = append(before, left)
		after = append(after, right)
		cut = 0
	}
	if cut != 0 {
		return nil, nil, fmt.Errorf("native body-mark extent exceeds paragraph")
	}
	return before, after, nil
}
func restoreColleafBodyMark(ctx context.Context, n vdocsNativeNote, scope *vdocsBodyMarkScope, note *Note, doc *Document) error {
	if scope == nil {
		return nil
	}
	stale := func() {
		doc.warn("colleaf_stale_note_body_mark", "Stale native body-mark ownership ignored; actual visible story text and generated marker state are retained")
	}
	if scope.NoteID != n.ID || n.ShowBodyReference == nil || *n.ShowBodyReference {
		stale()
		return nil
	}
	if scope.Empty {
		if scope.Mode != "style" || scope.Start != vdocsUTF16Len(n.Body.DataStream)-1 || scope.End != scope.Start || len(n.ReferenceStyle) == 0 {
			stale()
			return nil
		}
		if scope.UseDefault {
			note.BodyLabel = nil
		}
		scope.Restored = true
		return nil
	}
	if len(n.Body.Paragraphs) == 0 || n.Body.Paragraphs[0].ID != scope.ParagraphID || scope.End >= n.Body.Paragraphs[0].StartIndex || len(note.Blocks) == 0 || (note.Blocks[0].Kind != Paragraph && note.Blocks[0].Kind != Heading) {
		stale()
		return nil
	}

	runs, remaining, err := splitColleafRuns(ctx, note.Blocks[0].Runs, scope.End+1)
	if err != nil {
		return err
	}
	text := noteLabelContentText(runs)
	if !strings.HasSuffix(text, "\t") {
		stale()
		return nil
	}
	runs, _, err = splitColleafRuns(ctx, runs, vdocsUTF16Len(text)-1)
	if err != nil {
		return err
	}
	text = noteLabelContentText(runs)
	if text == "" || validateNoteLabel(text) != nil {
		stale()
		return nil
	}

	scope.Restored = true
	note.Blocks[0].Runs = remaining
	if scope.Separate && len(remaining) == 0 {
		note.Blocks = note.Blocks[1:]
	}
	if len(note.Blocks) == 0 {
		note.Blocks = nil
	}
	note.BodyLabel = nil
	note.BodyLabelContent = nil
	note.BodyLabelStyle = nil
	if scope.Mode == "rich" {
		note.BodyLabelContent = runs
		return nil
	}
	first := runs[0]
	first.Text = ""
	uniform := true
	for _, r := range runs {
		r.Text = ""
		uniform = uniform && reflect.DeepEqual(r, first)
	}
	if scope.Mode == "literal" && uniform && reflect.DeepEqual(first, Run{}) {
		label := text
		note.BodyLabel = &label
		return nil
	}
	if scope.Mode == "style" && uniform {
		note.BodyLabelStyle = &first
		if !scope.UseDefault || n.Mark == nil || *n.Mark != text {
			label := text
			note.BodyLabel = &label
		}
		return nil
	}
	note.BodyLabelContent = runs
	doc.warn("colleaf_note_body_mark_edit", "Native body-mark formatting is retained as rich label content after editing its original representation")
	return nil
}

type vdocsNoteProperties struct {
	Format       *string         `json:"numberFormat,omitempty"`
	Start        *int            `json:"startNumber,omitempty"`
	Restart      *string         `json:"restart,omitempty"`
	Position     *string         `json:"position,omitempty"`
	Columns      *int            `json:"columnCount,omitempty"`
	Separator    json.RawMessage `json:"separator,omitempty"`
	Continuation json.RawMessage `json:"continuationSeparator,omitempty"`
	Notice       json.RawMessage `json:"continuationNotice,omitempty"`
}
type vdocsNoteSettings struct {
	Footnote vdocsNoteProperties `json:"footnote,omitempty"`
	Endnote  vdocsNoteProperties `json:"endnote,omitempty"`
}
type vdocsNativeNoteMarks struct{ First, Last string }
type vdocsNoteNumberPlan struct {
	Overrides   map[string]string
	Marks       map[string]vdocsNativeNoteMarks
	Diagnostics []Diagnostic
}

func readColleafNoteSettings(raw json.RawMessage) (vdocsNoteSettings, error) {
	var settings vdocsNoteSettings
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
	for kind, p := range map[string]vdocsNoteProperties{Footnote: settings.Footnote, Endnote: settings.Endnote} {
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
func vdocsHasNoteSettings(s vdocsNoteSettings) bool {
	for _, p := range []vdocsNoteProperties{s.Footnote, s.Endnote} {
		if p.Format != nil || p.Start != nil || p.Restart != nil || p.Position != nil || p.Columns != nil {
			return true
		}
	}
	return false
}
func mergeColleafNoteProperties(base, local vdocsNoteProperties) vdocsNoteProperties {
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

func vdocsNoteNumber(format string, value int) (string, error) {
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

func planColleafNoteNumbers(ctx context.Context, input vdocsNoteSnapshot) (vdocsNoteNumberPlan, error) {
	plan := vdocsNoteNumberPlan{Overrides: map[string]string{}, Marks: map[string]vdocsNativeNoteMarks{}}
	settings, err := readColleafNoteSettings(input.NoteSettings)
	if err != nil {
		return plan, err
	}
	index, err := indexColleafBody(ctx, input.Body)
	if err != nil {
		return plan, err
	}
	records := map[int]vdocsSectionRecord{}
	for _, record := range input.Body.SectionBreaks {
		records[record.StartIndex] = record
	}
	sectionSettings := make([]vdocsNoteSettings, len(index.sections))
	settingsPresent := vdocsHasNoteSettings(settings)
	for i, node := range index.sections {
		raw := records[node.end-1].Extra["noteProperties"]
		sectionSettings[i], err = readColleafNoteSettings(raw)
		if err != nil {
			return plan, err
		}
		settingsPresent = settingsPresent || vdocsHasNoteSettings(sectionSettings[i])
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
			label, err = vdocsNoteNumber(format, state.Next)
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
func applyColleafNoteNumbers(doc *Document, plan vdocsNoteNumberPlan, ids map[string]noteKey) {
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

type vdocsNoteOrigin struct {
	Version          int         `json:"version"`
	NativeID         string      `json:"nativeId"`
	Kind             string      `json:"kind"`
	ID               string      `json:"id"`
	Source           *SourceInfo `json:"source,omitempty"`
	LabelSource      *SourceInfo `json:"labelSource,omitempty"`
	KindInferred     bool        `json:"kindInferred,omitempty"`
	BodyFingerprint  string      `json:"bodyFingerprint"`
	LabelFingerprint string      `json:"labelFingerprint"`
}

func vdocsSourceFreeHash(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	var tree any
	if err = json.Unmarshal(raw, &tree); err != nil {
		return "", err
	}
	var strip func(any)
	strip = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			delete(x, "source")
			delete(x, "label_source")
			if marks, ok := x["marks"].([]any); ok {
				kept := marks[:0]
				for _, mark := range marks {
					m, object := mark.(map[string]any)
					if !object || m["type"] != "source" {
						kept = append(kept, mark)
					}
				}
				if len(kept) == 0 {
					delete(x, "marks")
				} else {
					x["marks"] = kept
				}
			}
			for _, child := range x {
				strip(child)
			}
			if attrs, ok := x["attrs"].(map[string]any); ok && len(attrs) == 0 {
				delete(x, "attrs")
			}
		case []any:
			for _, child := range x {
				strip(child)
			}
		}
	}
	strip(tree)
	raw, err = json.Marshal(tree)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}
func vdocsNoteFingerprints(n StructuredNote) (string, string, error) {
	body, err := vdocsSourceFreeHash(n.Content)
	if err != nil {
		return "", "", err
	}
	label, err := vdocsSourceFreeHash(StructuredNote{Label: n.Label, BodyLabel: n.BodyLabel, BodyLabelMarks: n.BodyLabelMarks, BodyLabelContent: n.BodyLabelContent})
	return body, label, err
}
func collectColleafNoteOrigins(d *Document, e **vdocsDocumentEvidence) error {
	var sf *StructuredDocument
	for i, n := range d.Notes {
		if n.Source == nil && n.LabelSource == nil && !n.KindInferred {
			continue
		}
		if sf == nil {
			var err error
			sf, err = ToStructured(d)
			if err != nil {
				return err
			}
		}
		body, label, err := vdocsNoteFingerprints(sf.Attrs.Notes[i])
		if err != nil {
			return err
		}
		if *e == nil {
			*e = &vdocsDocumentEvidence{Version: 1}
		}
		(*e).NoteOrigins = append((*e).NoteOrigins, vdocsNoteOrigin{Version: 1, NativeID: fmt.Sprintf("manydoc-note-%d", i+1), Kind: n.Kind, ID: n.ID, Source: cloneSourceInfo(n.Source), LabelSource: cloneSourceInfo(n.LabelSource), KindInferred: n.KindInferred, BodyFingerprint: body, LabelFingerprint: label})
	}
	return nil
}
func (o vdocsNoteOrigin) validate(pages []PDFPage) error {
	if o.Version != 1 || o.NativeID == "" || o.ID == "" || (o.Kind != Footnote && o.Kind != Endnote) || (o.Source == nil && o.LabelSource == nil && !o.KindInferred) {
		return fmt.Errorf("invalid native note source record")
	}
	for _, s := range []string{o.NativeID, o.ID} {
		if err := validateString(s); err != nil {
			return err
		}
	}
	for _, hash := range []string{o.BodyFingerprint, o.LabelFingerprint} {
		if len(hash) != 64 {
			return fmt.Errorf("invalid native note semantic fingerprint")
		}
		if _, err := hex.DecodeString(hash); err != nil {
			return err
		}
	}
	for _, s := range []*SourceInfo{o.Source, o.LabelSource} {
		if s != nil {
			if err := s.validate(pages); err != nil {
				return err
			}
		}
	}
	return nil
}
func restoreColleafNoteOrigins(d *Document, e *vdocsDocumentEvidence, input vdocsNoteSnapshot, ids vdocsNoteIdentities) error {
	if e == nil || len(e.NoteOrigins) == 0 {
		return nil
	}
	sf, err := ToStructured(d)
	if err != nil {
		return err
	}
	current := map[noteKey]int{}
	for i, n := range d.Notes {
		current[noteIdentity(n)] = i
	}
	bound := map[string]noteKey{}
	for _, b := range ids.Bindings {
		if n, ok := input.Notes[b.NativeID]; ok && n.ID == b.NativeID && n.Kind == b.Kind {
			bound[b.NativeID] = noteKey{b.Kind, b.ID}
		}
	}
	archive := map[string]bool{}
	for _, s := range d.UnboundSources {
		archive[string(sfAttrs(s))] = true
	}
	for _, o := range e.NoteOrigins {
		if err := o.validate(d.Pages); err != nil {
			return err
		}
		key := noteKey{o.Kind, o.ID}
		i, exists := current[key]
		if !exists || bound[o.NativeID] != key {
			for _, s := range []*SourceInfo{o.Source, o.LabelSource} {
				if s == nil {
					continue
				}
				raw := string(sfAttrs(s))
				if !archive[raw] {
					d.UnboundSources = append(d.UnboundSources, *cloneSourceInfo(s))
					archive[raw] = true
				}
			}
			d.warn("colleaf_note_source_unbound", fmt.Sprintf("Original %s %q source facts are retained unbound after its native identity binding disappeared", o.Kind, o.ID))
			if o.KindInferred {
				d.warn("colleaf_note_kind_history", fmt.Sprintf("Original %s %q placement was inferred; no current note-kind assertion is made", o.Kind, o.ID))
			}
			continue
		}
		body, label, err := vdocsNoteFingerprints(sf.Attrs.Notes[i])
		if err != nil {
			return err
		}
		n := &d.Notes[i]
		n.Source = cloneSourceInfo(o.Source)
		n.LabelSource = cloneSourceInfo(o.LabelSource)
		n.KindInferred = o.KindInferred
		if n.Source != nil && (body != o.BodyFingerprint || label != o.LabelFingerprint) {
			n.Source.Historical = true
			d.warn("colleaf_note_source_historical", "Original note source facts are retained as historical after current body or marker semantics changed")
		}
		if n.LabelSource != nil && label != o.LabelFingerprint {
			n.LabelSource.Historical = true
			d.warn("colleaf_note_label_source_historical", "Original note-label source facts are retained as historical after current marker semantics changed")
		}
	}
	return nil
}

const vdocsNoteIdentityResource = "manydoc.note-identities.v1"

type vdocsResource struct {
	Name string `json:"name"`
	Data string `json:"data"`
}
type vdocsNativeNote struct {
	ID                string                     `json:"noteId"`
	Kind              string                     `json:"type"`
	Body              vdocsBody                  `json:"body"`
	Tables            map[string]json.RawMessage `json:"tableSource,omitempty"`
	Mark              *string                    `json:"customMark,omitempty"`
	ReferenceStyle    json.RawMessage            `json:"referenceTextStyle,omitempty"`
	ShowBodyReference *bool                      `json:"showBodyReference,omitempty"`
	Drawings          map[string]json.RawMessage `json:"drawings,omitempty"`
	DrawingOrder      []string                   `json:"drawingsOrder,omitempty"`
	Lists             map[string]json.RawMessage `json:"lists,omitempty"`
}
type vdocsNoteBinding struct {
	LiteralCitations *string `json:"literalCitations,omitempty"`
	BodyMarkRange    string  `json:"bodyMarkRange,omitempty"`
	AutoLabel        *string `json:"autoLabel,omitempty"`
	NativeID         string  `json:"nativeId"`
	Kind             string  `json:"kind"`
	ID               string  `json:"id"`
}
type vdocsNoteIdentities struct {
	Bindings []vdocsNoteBinding `json:"bindings"`
}
type vdocsNoteSnapshot struct {
	Settings                 json.RawMessage              `json:"settings,omitempty"`
	Locale                   string                       `json:"locale,omitempty"`
	Body                     vdocsBody                    `json:"body"`
	Tables                   map[string]json.RawMessage   `json:"tableSource,omitempty"`
	Notes                    map[string]vdocsNativeNote   `json:"notes,omitempty"`
	Resources                []vdocsResource              `json:"resources,omitempty"`
	Lists                    map[string]json.RawMessage   `json:"lists,omitempty"`
	ID                       string                       `json:"id,omitempty"`
	Drawings                 map[string]json.RawMessage   `json:"drawings,omitempty"`
	DrawingOrder             []string                     `json:"drawingsOrder,omitempty"`
	Title                    string                       `json:"title,omitempty"`
	DocumentStyle            json.RawMessage              `json:"documentStyle,omitempty"`
	Headers                  map[string]vdocsRunningStory `json:"headers,omitempty"`
	Footers                  map[string]vdocsRunningStory `json:"footers,omitempty"`
	Styles                   map[string]vdocsNamedStyle   `json:"styles,omitempty"`
	HeaderFooterDrawingOrder []string                     `json:"headerFooterDrawingsOrder,omitempty"`
	NoteSettings             json.RawMessage              `json:"noteSettings,omitempty"`
}

func encodeColleafNotes(ctx context.Context, d *Document) (vdocsNoteSnapshot, error) {
	return encodeColleafNotesWithCatalog(ctx, d, nil)
}
func encodeColleafNotesWithCatalog(ctx context.Context, d *Document, catalog *noteCatalog, projection ...map[noteKey]string) (vdocsNoteSnapshot, error) {
	out := vdocsNoteSnapshot{ID: "manydoc"}
	if err := d.Validate(); err != nil {
		return out, err
	}
	d = resolveNumberingDocument(d)
	if catalog == nil {
		var err error
		catalog, err = catalogNotes(d, false)
		if err != nil {
			return out, err
		}
	}

	citationProjection := vdocsCitationProjection(d, catalog)
	if len(projection) > 0 {
		citationProjection = projection[0]
	}
	ids := map[noteKey]string{}
	bindings := vdocsNoteIdentities{}
	ownership := vdocsListOwnership{Stories: map[string][]vdocsListBinding{}}
	imageGeometry := vdocsImageGeometry{Stories: map[string][]vdocsImageBinding{}}
	imageCache := outputImageCache{}
	out.Notes = map[string]vdocsNativeNote{}
	for i, n := range d.Notes {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		if n.Source != nil || n.LabelSource != nil || n.KindInferred {
			return out, fmt.Errorf("%w: native note provenance requires checked intent mapping", ErrUnsupported)
		}

		id := fmt.Sprintf("manydoc-note-%d", i+1)
		ids[noteIdentity(n)] = id
		binding := vdocsNoteBinding{NativeID: id, Kind: n.Kind, ID: n.ID}
		if n.Label == "" && catalog.labels[noteIdentity(n)] != "" {
			label := catalog.labels[noteIdentity(n)]
			binding.AutoLabel = &label
		}
		bodyInput := n
		if _, projected := citationProjection[noteIdentity(n)]; projected {
			label := n.Label
			binding.LiteralCitations = &label
			if n.BodyLabel == nil && n.BodyLabelContent == nil && n.BodyLabelStyle == nil {
				actual := catalog.labels[noteIdentity(n)]
				bodyInput.BodyLabel = &actual
			}
		}
		blocks, bodyMark, markLength, err := prepareColleafBodyMark(bodyInput, catalog)
		if bodyMark != nil && binding.LiteralCitations != nil && n.BodyLabel == nil && n.BodyLabelContent == nil {
			bodyMark.UseDefault = true
		}
		if err != nil {
			return out, err
		}
		if bodyMark != nil {
			binding.BodyMarkRange = "manydoc-body-mark-" + id
		}
		bindings.Bindings = append(bindings.Bindings, binding)
		encoded, err := encodeColleafAssetBody(ctx, blocks, nil, out.ID, id, imageCache)
		if err != nil {
			return out, err
		}
		note := vdocsNativeNote{ID: id, Kind: n.Kind, Body: encoded.Body, Tables: encoded.Tables, Lists: encoded.Lists, Drawings: encoded.Drawings, DrawingOrder: encoded.DrawingOrder}
		if len(encoded.ListsOwnership) > 0 {
			ownership.Stories[id] = encoded.ListsOwnership
		}
		if len(encoded.ImageGeometry) > 0 {
			imageGeometry.Stories[id] = encoded.ImageGeometry
		}
		if n.Label != "" || binding.AutoLabel != nil {
			mark := n.Label
			if binding.AutoLabel != nil {
				mark = *binding.AutoLabel
			}
			note.Mark = &mark
		}
		if n.BodyLabel != nil {
			show := false
			note.ShowBodyReference = &show
		}
		if bodyMark != nil {
			if bodyMark.Empty {
				style, err := vdocsRunStyle(*n.BodyLabelStyle)
				if err != nil {
					return out, err
				}
				note.ReferenceStyle = style
			}
			if err := sealColleafBodyMark(&note, *bodyMark, markLength); err != nil {
				return out, err
			}
		}
		if binding.LiteralCitations != nil {
			empty := ""
			note.Mark = &empty
		}
		out.Notes[id] = note
	}
	encoded, err := encodeColleafAssetBody(ctx, d.Blocks, ids, out.ID, "", imageCache, citationProjection)
	if err != nil {
		return out, err
	}
	out.Body = encoded.Body
	out.Tables = encoded.Tables
	out.Lists = encoded.Lists
	out.Drawings = encoded.Drawings
	out.DrawingOrder = encoded.DrawingOrder
	if len(encoded.ListsOwnership) > 0 {
		ownership.Stories[""] = encoded.ListsOwnership
	}
	if len(encoded.ImageGeometry) > 0 {
		imageGeometry.Stories[""] = encoded.ImageGeometry
	}
	if len(bindings.Bindings) > 0 {
		data, err := json.Marshal(bindings)
		if err != nil {
			return out, err
		}
		out.Resources = []vdocsResource{{Name: vdocsNoteIdentityResource, Data: string(data)}}
	}
	if len(ownership.Stories) > 0 {
		data, err := json.Marshal(ownership)
		if err != nil {
			return out, err
		}
		out.Resources = append(out.Resources, vdocsResource{Name: vdocsListResource, Data: string(data)})
	}
	if len(imageGeometry.Stories) > 0 {
		data, err := json.Marshal(imageGeometry)
		if err != nil {
			return out, err
		}
		out.Resources = append(out.Resources, vdocsResource{Name: vdocsImageResource, Data: string(data)})
	}
	if err := sealColleafIntent(&out); err != nil {
		return vdocsNoteSnapshot{}, err
	}
	return out, nil
}

func decodeColleafNotes(ctx context.Context, input vdocsNoteSnapshot, settings vdocsTextContext) (*Document, error) {
	var err error
	input, err = openColleafIntent(ctx, input)
	if err != nil {
		return nil, err
	}
	input, hyperlinkDiagnostics, err := applyColleafHyperlinkResource(ctx, input)
	if err != nil {
		return nil, err
	}
	if settings.NavigationAliases == nil {
		settings.NavigationAliases, err = vdocsBookmarkAliases(input.Body)
		if err != nil {
			return nil, err
		}
	}
	if settings.Citations == nil {
		input, settings.Citations, err = openColleafCitations(ctx, input)
		if err != nil {
			return nil, err
		}
	}
	var numberPlan vdocsNoteNumberPlan
	if !settings.DeferNoteNumbering {
		numberPlan, err = planColleafNoteNumbers(ctx, input)
		if err != nil {
			return nil, err
		}
		settings.Citations.adaptPlan(&numberPlan)
		settings.NoteLabels = numberPlan.Overrides
	}
	doc := &Document{Diagnostics: hyperlinkDiagnostics}
	ids := map[string]noteKey{}
	autoLabels := map[noteKey]string{}
	expectedBodyMarks := map[string]string{}
	keys := make([]string, 0, len(input.Notes))
	for id, n := range input.Notes {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if id == "" || id != n.ID || (n.Kind != Footnote && n.Kind != Endnote) {
			return nil, fmt.Errorf("native note identity/kind mismatch")
		}
		if err := validateString(id); err != nil {
			return nil, err
		}
		ids[id] = noteKey{n.Kind, id}
		keys = append(keys, id)
	}
	sort.Strings(keys)

	var order []string
	seenResource := false
	ownership := vdocsListOwnership{}
	seenLists := false
	seenImages := false
	imageGeometry := vdocsImageGeometry{}
	if settings.ImageCache == nil {
		settings.ImageCache = outputImageCache{}
	}
	for _, resource := range input.Resources {
		if resource.Name == "DOC_OBJECT_PERMISSION_PLUGIN" {
			continue
		}
		if resource.Name == "DOC_DRAWING_PLUGIN" {
			if err := checkColleafDrawingMirror(input, resource); err != nil {
				return nil, err
			}
			continue
		}
		if emptyColleafBuiltinResource(resource) {
			continue
		}
		if resource.Name == vdocsImageResource {
			if seenImages {
				return nil, fmt.Errorf("duplicate native image geometry resource")
			}
			seenImages = true
			if err := strictSFAttrs(json.RawMessage(resource.Data), &imageGeometry); err != nil {
				return nil, err
			}
			continue
		}
		if resource.Name == vdocsListResource {
			if seenLists {
				return nil, fmt.Errorf("duplicate native list ownership resource")
			}
			seenLists = true
			if err := strictSFAttrs(json.RawMessage(resource.Data), &ownership); err != nil {
				return nil, err
			}
			continue
		}
		if resource.Name != vdocsNoteIdentityResource {
			doc.warn("colleaf_resource", "Native plugin resource "+resource.Name+" is not represented; supported visible content is retained")
			continue
		}
		if seenResource {
			return nil, fmt.Errorf("duplicate native note identity resource")
		}
		seenResource = true
		var bindings vdocsNoteIdentities
		if err := strictSFAttrs(json.RawMessage(resource.Data), &bindings); err != nil {
			return nil, err
		}
		seen := map[string]bool{}
		for _, binding := range bindings.Bindings {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if seen[binding.NativeID] || binding.NativeID == "" || binding.ID == "" || (binding.Kind != Footnote && binding.Kind != Endnote) {
				return nil, fmt.Errorf("invalid/duplicate native note identity binding")
			}
			seen[binding.NativeID] = true
			if err := validateString(binding.ID); err != nil {
				return nil, err
			}
			n, ok := input.Notes[binding.NativeID]
			if !ok || n.ID != binding.NativeID || n.Kind != binding.Kind {
				doc.warn("colleaf_stale_note_identity", "Stale note identity metadata ignored; actual native note identity/kind is retained")
				continue
			}
			ids[binding.NativeID] = noteKey{binding.Kind, binding.ID}
			expectedBodyMarks[binding.NativeID] = binding.BodyMarkRange
			if binding.AutoLabel != nil && n.Mark != nil && *n.Mark == *binding.AutoLabel {
				autoLabels[ids[binding.NativeID]] = *binding.AutoLabel
			}

			order = append(order, binding.NativeID)
		}
	}
	ordered := map[string]bool{}
	originals := map[noteKey]bool{}
	for _, id := range order {
		ordered[id] = true
	}
	for _, id := range keys {
		if !ordered[id] {
			order = append(order, id)
		}
	}
	for _, id := range order {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		n, bodyMarkScope, err := openColleafBodyMark(ctx, input.Notes[id])
		if err != nil {
			return nil, err
		}
		if expected := expectedBodyMarks[id]; expected != "" && (bodyMarkScope == nil || bodyMarkScope.RangeID != expected) {
			doc.warn("colleaf_stale_note_body_mark", "Expected native body-mark ownership is absent or changed; actual visible story text is retained")
			bodyMarkScope = nil
		}
		key := ids[id]
		if originals[key] {
			return nil, fmt.Errorf("native note identity bindings alias distinct definitions")
		}
		originals[key] = true

		bodySettings := settings
		bodySettings.NavigationScope = "note:" + id
		bodySettings.Citations = nil
		bodySettings.NoteIDs = map[string]noteKey{id: ids[id]}
		bodySettings.Lists = n.Lists
		bodySettings.ListBindings = ownership.Stories[id]
		bodySettings.Drawings = n.Drawings
		bodySettings.DrawingOrder = n.DrawingOrder
		bodySettings.ImageBindings = imageGeometry.Stories[id]
		body, err := decodeColleafBody(ctx, n.Body, bodySettings, n.Tables)
		if err != nil {
			return nil, err
		}
		note := Note{ID: key.id, Kind: key.kind, Blocks: body.Blocks}

		var bodyMarker *Run
		if len(n.Body.CustomBlocks) > 0 {
			for _, atom := range n.Body.CustomBlocks {
				if atom.StartIndex != 0 || atom.NoteID != id || (atom.Type != nil && *atom.Type == 0) {
					continue
				}
				if len(note.Blocks) == 0 || (note.Blocks[0].Kind != Paragraph && note.Blocks[0].Kind != Heading) || len(note.Blocks[0].Runs) == 0 || note.Blocks[0].Runs[0].Note == nil {
					return nil, fmt.Errorf("invalid native note body marker")
				}
				if n.ShowBodyReference != nil && !*n.ShowBodyReference {
					return nil, fmt.Errorf("%w: contradictory native hidden/explicit body marker", ErrUnsupported)
				}
				style := note.Blocks[0].Runs[0]
				style.Note = nil
				bodyMarker = &style
				note.Blocks[0].Runs = note.Blocks[0].Runs[1:]
				if len(note.Blocks[0].Runs) == 0 {
					note.Blocks[0].Runs = nil
				}
			}
		}

		doc.Diagnostics = append(doc.Diagnostics, body.Diagnostics...)
		if n.Mark != nil {
			if *n.Mark == "" {
				empty := ""
				note.BodyLabel = &empty
			}
			if err := validateNoteLabel(*n.Mark); err != nil {
				return nil, err
			}
			note.Label = *n.Mark
			if _, ok := autoLabels[key]; ok {
				note.Label = ""
			}

		}
		if n.ShowBodyReference != nil && !*n.ShowBodyReference {
			empty := ""
			note.BodyLabel = &empty
		}
		if len(n.ReferenceStyle) > 0 {
			style, err := vdocsStyleObject(n.ReferenceStyle)
			if err != nil {
				return nil, err
			}
			run, err := vdocsStyleRun(style, doc)
			if err != nil {
				return nil, err
			}
			note.BodyLabelStyle = &run
			if bodyMarkScope == nil {
				doc.warn("colleaf_note_marker_style", "Native generated marker style is retained on the note body; citation style comes from its native text interval")
			}
		}
		if bodyMarker != nil {
			note.BodyLabelStyle = bodyMarker
		}
		if err = restoreColleafBodyMark(ctx, n, bodyMarkScope, &note, doc); err != nil {
			return nil, err
		}
		if bodyMarkScope != nil && bodyMarkScope.UseDefault && bodyMarkScope.Restored {
			settings.Citations.BodyDefaults[key] = true
		}

		doc.Notes = append(doc.Notes, note)
	}
	settings.NoteIDs = ids
	settings.Lists = input.Lists
	settings.ListBindings = ownership.Stories[""]
	settings.Drawings = input.Drawings
	settings.DrawingOrder = input.DrawingOrder
	settings.ImageBindings = imageGeometry.Stories[""]
	for story := range ownership.Stories {
		if story != "" {
			if _, ok := input.Notes[story]; !ok {
				doc.warn("colleaf_stale_list_ownership", "List ownership for an absent native story ignored; actual native content is retained")
			}
		}
	}
	body, err := decodeColleafBody(ctx, input.Body, settings, input.Tables)
	if err != nil {
		return nil, err
	}
	doc.Blocks = body.Blocks
	doc.Diagnostics = append(doc.Diagnostics, body.Diagnostics...)
	if !settings.DeferNoteNumbering {
		applyColleafNoteNumbers(doc, numberPlan, ids)
		settings.Citations.restoreDefinitions(doc, ids)
		if err = finishColleafNoteNumbering(doc, autoLabels); err != nil {
			return nil, err
		}
	}
	if !settings.DeferNoteNumbering {
		if err = settings.Citations.collapse(ctx, doc); err != nil {
			return nil, err
		}
	}
	if !settings.DeferNoteNumbering {
		if err = finishColleafNavigation(ctx, input, doc, nil); err != nil {
			return nil, err
		}
	}
	if err = validateNotes(doc); err != nil {
		return nil, err
	}
	return doc, nil
}

func finishColleafNoteNumbering(doc *Document, autoLabels map[noteKey]string) error {
	if len(autoLabels) > 0 {
		actual, err := catalogNotes(doc, false)
		if err != nil {
			return err
		}
		for i, n := range doc.Notes {
			if label, ok := autoLabels[noteIdentity(n)]; ok && actual.labels[noteIdentity(n)] != label {
				doc.Notes[i].Label = label
				doc.warn("colleaf_stale_note_numbering", "Native custom mark retained because current citation order no longer matches original automatic numbering")
			}
		}
	}

	return nil
}
