package manydoc

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

const colleafBodyMarkProfile = "manydoc.note-body-mark"

type colleafBodyMark struct {
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
type colleafBodyMarkScope struct {
	colleafBodyMark
	Start, End int
	RangeID    string
	Restored   bool
}

func prepareColleafBodyMark(n Note, catalog *noteCatalog) ([]Block, *colleafBodyMark, int, error) {
	if n.BodyLabel == nil && n.BodyLabelContent == nil && n.BodyLabelStyle == nil {
		return n.Blocks, nil, 0, nil
	}
	runs := noteBodyLabelRuns(catalog, noteIdentity(n))
	text := noteLabelContentText(runs)
	// The existing native markerless representation needs no ownership annotation.
	if text == "" && n.BodyLabelStyle == nil {
		return n.Blocks, nil, 0, nil
	}
	mode := "literal"
	if n.BodyLabelContent != nil {
		mode = "rich"
	} else if n.BodyLabelStyle != nil {
		mode = "style"
	}
	hint := &colleafBodyMark{Profile: colleafBodyMarkProfile, Version: 1, TextColorMode: "text", Mode: mode, UseDefault: n.BodyLabel == nil && n.BodyLabelContent == nil, Empty: text == ""}
	if hint.Empty {
		if n.BodyLabelStyle.Link != "" || n.BodyLabelStyle.Destination != "" {
			return nil, nil, 0, fmt.Errorf("%w: native empty body-mark navigation has no visible anchor", ErrUnsupported)
		}
		return n.Blocks, hint, 0, nil
	}
	for _, r := range runs {
		if _, err := colleafRunStyle(r); err != nil {
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
			blocks[0].colleafOrigin = origin
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
	return blocks, hint, colleafUTF16Len(text) + 1, nil
}
func sealColleafBodyMark(n *colleafNativeNote, hint colleafBodyMark, length int) error {
	hint.NoteID = n.ID
	start, end := 0, length-1
	if hint.Empty {
		start = colleafUTF16Len(n.Body.DataStream) - 1
		end = start
	} else {
		if len(n.Body.Paragraphs) == 0 {
			return fmt.Errorf("native body mark lacks paragraph")
		}
		hint.ParagraphID = n.Body.Paragraphs[0].ID
	}
	inactive := false
	n.Body.CustomRanges = append(n.Body.CustomRanges, colleafRangeRecord{StartIndex: start, EndIndex: end, ID: "manydoc-body-mark-" + n.ID, Type: 5, Active: &inactive, Properties: sfAttrs(hint)})
	hidden := false
	n.ShowBodyReference = &hidden
	return nil
}
func openColleafBodyMark(ctx context.Context, n colleafNativeNote) (colleafNativeNote, *colleafBodyMarkScope, error) {
	var scope *colleafBodyMarkScope
	var ranges []colleafRangeRecord
	for _, a := range n.Body.CustomRanges {
		var marker struct {
			Profile string `json:"profile"`
		}
		_ = json.Unmarshal(a.Properties, &marker)
		if a.Type != 5 || marker.Profile != colleafBodyMarkProfile {
			ranges = append(ranges, a)
			continue
		}
		if scope != nil {
			return n, nil, fmt.Errorf("duplicate native note body-mark ownership")
		}
		var hint colleafBodyMark
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
		scope = &colleafBodyMarkScope{colleafBodyMark: hint, Start: a.StartIndex, End: a.EndIndex, RangeID: a.ID}
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
		size := colleafUTF16Len(r.Text)
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
func restoreColleafBodyMark(ctx context.Context, n colleafNativeNote, scope *colleafBodyMarkScope, note *Note, doc *Document) error {
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
		if scope.Mode != "style" || scope.Start != colleafUTF16Len(n.Body.DataStream)-1 || scope.End != scope.Start || len(n.ReferenceStyle) == 0 {
			stale()
			return nil
		}
		if scope.UseDefault {
			note.BodyLabel = nil
		}
		scope.Restored = true
		return nil // real referenceTextStyle was decoded normally, without text caches
	}
	if len(n.Body.Paragraphs) == 0 || n.Body.Paragraphs[0].ID != scope.ParagraphID || scope.End >= n.Body.Paragraphs[0].StartIndex || len(note.Blocks) == 0 || (note.Blocks[0].Kind != Paragraph && note.Blocks[0].Kind != Heading) {
		stale()
		return nil
	}
	// TextX replacement can detach the range's start from newly inserted label
	// text. Paragraph identity and the owned terminal tab bind the leading mark;
	// using End retains edits without guessing at stale original text.
	runs, remaining, err := splitColleafRuns(ctx, note.Blocks[0].Runs, scope.End+1)
	if err != nil {
		return err
	}
	text := noteLabelContentText(runs)
	if !strings.HasSuffix(text, "\t") {
		stale()
		return nil
	}
	runs, _, err = splitColleafRuns(ctx, runs, colleafUTF16Len(text)-1)
	if err != nil {
		return err
	}
	text = noteLabelContentText(runs)
	if text == "" || validateNoteLabel(text) != nil {
		stale()
		return nil
	}
	// Commit the structural collapse only after the whole ownership guard passes.
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
