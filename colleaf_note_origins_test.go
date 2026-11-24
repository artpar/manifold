package manydoc

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
)

func colleafNoteOriginExample(t *testing.T) *Document {
	base := colleafOriginExample(t)
	source := func(id string) *SourceInfo { s := cloneSourceInfo(base.Blocks[0].Source); s.Element = id; return s }
	return &Document{Metadata: Metadata{Title: "Note origins"}, Pages: base.Pages, Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "😀 Notes ", Bold: true, FontSize: 13}, {Note: &NoteReference{Kind: Footnote, ID: "n"}}, {Text: " / "}, {Note: &NoteReference{Kind: Endnote, ID: "n"}}, {Text: " / "}, {Note: &NoteReference{Kind: Footnote, ID: "inference"}}}}}, Notes: []Note{
		{Kind: Footnote, ID: "n", KindInferred: true, Label: "*", BodyLabelContent: []Run{{Text: "α", Bold: true, FontSize: 13}}, Source: source("foot-definition"), LabelSource: source("foot-label"), Blocks: []Block{{Kind: Paragraph, Source: source("foot-paragraph"), Runs: []Run{{Text: "Footnote body"}}}}},
		{Kind: Endnote, ID: "n", Label: "!", Source: source("end-definition"), Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "Endnote body"}}}}},
		{Kind: Footnote, ID: "inference", KindInferred: true, Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "Inferred placement"}}}}},
	}}
}
func TestColleafNoteOriginsAndActualSave(t *testing.T) {
	ctx := context.Background()
	want := colleafNoteOriginExample(t)
	native, err := encodeColleafDocument(ctx, want)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(native)
	got, err := decodeColleafDocument(ctx, native)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(native)
	if !bytes.Equal(before, after) {
		t.Fatal("input mutated")
	}
	assertEvidenceSF(t, want, got)
	got.Notes[0].Source.StructureID[0] = 0
	got, err = decodeColleafDocument(ctx, native)
	if err != nil {
		t.Fatal(err)
	}
	assertEvidenceSF(t, want, got)
	if file := os.Getenv("MANYDOC_COLLEAF_NOTE_ORIGIN_QA"); file != "" {
		if err = os.WriteFile(file, before, 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, spec := range []struct{ env, mode string }{{"MANYDOC_COLLEAF_NOTE_ORIGIN_SAVED", "saved"}, {"MANYDOC_COLLEAF_NOTE_ORIGIN_BODY_EDITED", "body"}, {"MANYDOC_COLLEAF_NOTE_ORIGIN_MARK_EDITED", "mark"}, {"MANYDOC_COLLEAF_NOTE_ORIGIN_DETACHED", "detach"}} {
		t.Run(spec.mode, func(t *testing.T) {
			file := os.Getenv(spec.env)
			if file == "" {
				t.Skip("optional independent reader artifact")
			}
			raw, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			var wire colleafNoteSnapshot
			if err = json.Unmarshal(raw, &wire); err != nil {
				t.Fatal(err)
			}
			envelope, err := json.Marshal(colleafEnvelope{colleafEnvelopeType, 3, "univer", colleafEngineVersion, wire})
			if err != nil {
				t.Fatal(err)
			}
			actual, err := readColleaf(ctx, bytes.NewReader(envelope))
			if err != nil {
				t.Fatal(err)
			}
			expected := colleafNoteOriginExample(t)
			switch spec.mode {
			case "body":
				expected.Notes[0].Blocks[0].Runs = []Run{{Text: "Q"}, {Text: "Footnote body"}}
				expected.Notes[0].Blocks[0].Source.Historical = true
				expected.Notes[0].Source.Historical = true
				expected.Diagnostics = []Diagnostic{{Code: "colleaf_source_historical", Message: "Original PDF source facts are retained as historical after current paragraph content or presentation changed"}, {Code: "colleaf_note_source_historical", Message: "Original note source facts are retained as historical after current body or marker semantics changed"}}
			case "mark":
				expected.Notes[0].BodyLabelContent[0].Text = "β"
				expected.Notes[0].Source.Historical = true
				expected.Notes[0].LabelSource.Historical = true
				expected.Diagnostics = []Diagnostic{{Code: "colleaf_note_source_historical", Message: "Original note source facts are retained as historical after current body or marker semantics changed"}, {Code: "colleaf_note_label_source_historical", Message: "Original note-label source facts are retained as historical after current marker semantics changed"}}
			case "detach":
				expected.Notes[2].ID = "manydoc-note-3"
				expected.Notes[2].KindInferred = false
				expected.Notes[2].Label = "1"
				expected.Blocks[0].Runs[5].Note.ID = "manydoc-note-3"
				expected.Diagnostics = []Diagnostic{{Code: "colleaf_note_source_unbound", Message: "Original footnote \"inference\" source facts are retained unbound after its native identity binding disappeared"}, {Code: "colleaf_note_kind_history", Message: "Original footnote \"inference\" placement was inferred; no current note-kind assertion is made"}}
			}
			assertEvidenceSF(t, expected, actual)
			sf, err := ToStructured(actual)
			if err != nil {
				t.Fatal(err)
			}
			sfRaw, _ := json.Marshal(sf)
			if err = os.WriteFile(file+".sf", sfRaw, 0600); err != nil {
				t.Fatal(err)
			}
			next, err := encodeColleafDocument(ctx, actual)
			if err != nil {
				t.Fatal(err)
			}
			stable, err := decodeColleafDocument(ctx, next)
			if err != nil {
				t.Fatal(err)
			}
			assertEvidenceSF(t, actual, stable)
		})
	}
}
func TestColleafNoteOriginsMalformedAndUnbound(t *testing.T) {
	ctx := context.Background()
	want := colleafNoteOriginExample(t)
	native, err := encodeColleafDocument(ctx, want)
	if err != nil {
		t.Fatal(err)
	}
	opened, err := openColleafIntent(ctx, native)
	if err != nil {
		t.Fatal(err)
	}
	var intent colleafDocumentIntent
	if err = colleafResourceMap(opened.Resources, colleafDocumentResource, &intent); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"version", "fingerprint", "source", "duplicate"} {
		t.Run(name, func(t *testing.T) {
			raw, _ := json.Marshal(intent)
			var copy colleafDocumentIntent
			_ = json.Unmarshal(raw, &copy)
			switch name {
			case "version":
				copy.Evidence.NoteOrigins[0].Version = 2
			case "fingerprint":
				copy.Evidence.NoteOrigins[0].BodyFingerprint = "wrong"
			case "source":
				copy.Evidence.NoteOrigins[0].Source.Page = 99
			case "duplicate":
				copy.Evidence.NoteOrigins = append(copy.Evidence.NoteOrigins, copy.Evidence.NoteOrigins[0])
			}
			wire := opened
			wire.Resources = colleafReplaceResource(wire.Resources, colleafDocumentResource, copy)
			if _, err = decodeColleafDocument(ctx, wire); err == nil {
				t.Fatal("malformed source record accepted")
			}
		})
	}
	// Remove just the first identity binding, retaining visible note body/mark.
	var ids colleafNoteIdentities
	if err = colleafResourceMap(opened.Resources, colleafNoteIdentityResource, &ids); err != nil {
		t.Fatal(err)
	}
	ids.Bindings = ids.Bindings[1:]
	opened.Resources = colleafReplaceResource(opened.Resources, colleafNoteIdentityResource, ids)
	got, err := decodeColleafDocument(ctx, opened)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.UnboundSources) != 2 || got.Notes[2].Source != nil || got.Notes[2].LabelSource != nil || got.Notes[2].KindInferred {
		t.Fatal("lost identity incorrectly asserts current note provenance", got)
	}
	again, err := encodeColleafDocument(ctx, got)
	if err != nil {
		t.Fatal(err)
	}
	stable, err := decodeColleafDocument(ctx, again)
	if err != nil {
		t.Fatal(err)
	}
	assertEvidenceSF(t, got, stable)
}

func FuzzColleafNoteOrigins(f *testing.F) {
	seed := colleafNoteOrigin{Version: 1, NativeID: "native", Kind: Footnote, ID: "n", KindInferred: true, BodyFingerprint: strings.Repeat("0", 64), LabelFingerprint: strings.Repeat("1", 64)}
	f.Add([]byte(sfAttrs(seed)))
	f.Add([]byte(`{"version":2}`))
	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) > 32768 {
			return
		}
		if validateTypedJSON(context.Background(), raw, reflect.TypeFor[colleafNoteOrigin]()) != nil {
			return
		}
		var origin colleafNoteOrigin
		if strictSFAttrs(raw, &origin) != nil {
			return
		}
		_ = origin.validate(nil)
	})
}

// Dropping or qualifying source bindings alone does not edit note semantics.
func TestColleafNoteSourceFreeFingerprints(t *testing.T) {
	d := colleafNoteOriginExample(t)
	d.Notes[0].Blocks[0].Runs[0].Source = cloneSourceInfo(d.Notes[0].Source)
	d.Notes[0].BodyLabelContent[0].Source = cloneSourceInfo(d.Notes[0].LabelSource)
	sf, err := ToStructured(d)
	if err != nil {
		t.Fatal(err)
	}
	body, label, err := colleafNoteFingerprints(sf.Attrs.Notes[0])
	if err != nil {
		t.Fatal(err)
	}
	d.Notes[0].Blocks[0].Source = nil
	d.Notes[0].Blocks[0].Runs[0].Source = nil
	d.Notes[0].BodyLabelContent[0].Source = nil
	clean, err := ToStructured(d)
	if err != nil {
		t.Fatal(err)
	}
	sameBody, sameLabel, err := colleafNoteFingerprints(clean.Attrs.Notes[0])
	if err != nil {
		t.Fatal(err)
	}
	if body != sameBody || label != sameLabel {
		t.Fatal("source-only differences changed semantic fingerprints")
	}
	d.Notes[0].Blocks[0].Runs[0].Text = "Changed"
	d.Notes[0].BodyLabelContent[0].Text = "Changed mark"
	edited, err := ToStructured(d)
	if err != nil {
		t.Fatal(err)
	}
	editBody, editLabel, err := colleafNoteFingerprints(edited.Attrs.Notes[0])
	if err != nil {
		t.Fatal(err)
	}
	if body == editBody || label == editLabel {
		t.Fatal("visible edits did not change fingerprints")
	}
}
