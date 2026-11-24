package manydoc

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

func colleafNoteExample() *Document {
	p := func(text string) Block { return Block{Kind: Paragraph, Runs: []Run{{Text: text}}} }
	ref := func(kind, id string) Run { return Run{Note: &NoteReference{Kind: kind, ID: id}} }
	empty := ""
	return &Document{Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "A😀"}, ref(Footnote, "same"), {Text: " end"}, ref(Endnote, "same"), ref(Footnote, "same")}},
		{Kind: Table, Rows: []Row{{Cells: []Cell{{Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "In cell"}, ref(Footnote, "custom")}}}}}}}}}, Notes: []Note{
		{ID: "same", Kind: Endnote, Blocks: []Block{p("End body 😀")}},
		{ID: "same", Kind: Footnote, Blocks: []Block{p("Foot body"), {Kind: Table, ColumnWidths: []float64{72, 72}, Rows: []Row{{Cells: []Cell{{ColSpan: 2, Blocks: []Block{p("Merged note cell")}}}}}}}},
		{ID: "custom", Kind: Footnote, Label: "§", BodyLabel: &empty, Blocks: []Block{p("Custom body")}},
	}}
}
func TestColleafNoteIdentityReferenceBodyAndSFRoundTrip(t *testing.T) {
	ctx := context.Background()
	original := colleafNoteExample()
	original.Blocks[0].Runs[1].Bold = true
	original.Blocks[0].Runs[1].FontSize = 12
	original.Blocks[0].Runs[1].Language = "en-US"
	wire, err := encodeColleafNotes(ctx, original)
	if err != nil {
		t.Fatal(err)
	}
	if len(wire.Body.CustomBlocks) != 4 || len(wire.Notes) != 3 || wire.Body.CustomBlocks[0].StartIndex != 3 {
		t.Fatal("note identity/UTF-16 anchors", wire.Body.CustomBlocks)
	}
	doc, err := decodeColleafNotes(ctx, wire, colleafTextContext{})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(doc, original) {
		t.Fatalf("note semantics changed\ngot%+v\nwant%+v", doc, original)
	}
	sf, err := ToStructured(doc)
	if err != nil {
		t.Fatal(err)
	}
	through, err := FromStructured(sf)
	if err != nil {
		t.Fatal(err)
	}
	wireAgain, err := encodeColleafNotes(ctx, through)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(wire, wireAgain) {
		t.Fatal("SF changed native text/marks/notes/relations")
	}
	if file := os.Getenv("MANYDOC_COLLEAF_NOTE_QA"); file != "" {
		data, err := json.MarshalIndent(wire, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(file, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if file := os.Getenv("MANYDOC_COLLEAF_NOTE_EDIT"); file != "" {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var edited colleafNoteSnapshot
		if err = json.Unmarshal(data, &edited); err != nil {
			t.Fatal(err)
		}
		doc, err = decodeColleafNotes(ctx, edited, colleafTextContext{})
		if err != nil {
			t.Fatal(err)
		}
		if noteLabelContentText(doc.Notes[1].Blocks[0].Runs) != "Edit body" || doc.Notes[2].Label != "†" {
			t.Fatal("saved native note edits did not win", doc.Notes)
		}
	}
}
func TestColleafNoteNativeIdentityAndStaleBindings(t *testing.T) {
	ctx := context.Background()
	wire, err := encodeColleafNotes(ctx, colleafNoteExample())
	if err != nil {
		t.Fatal(err)
	}
	wire.Resources = nil
	doc, err := decodeColleafNotes(ctx, wire, colleafTextContext{})
	if err != nil {
		t.Fatal(err)
	}
	if doc.Notes[0].ID != "manydoc-note-1" || doc.Notes[0].Kind != Endnote {
		t.Fatal(doc.Notes)
	}
	wire, err = encodeColleafNotes(ctx, colleafNoteExample())
	if err != nil {
		t.Fatal(err)
	}
	note := wire.Notes["manydoc-note-3"]
	delete(wire.Notes, "manydoc-note-3")
	note.ID = "edited-id"
	wire.Notes[note.ID] = note
	for i := range wire.Body.CustomBlocks {
		if wire.Body.CustomBlocks[i].NoteID == "manydoc-note-3" {
			wire.Body.CustomBlocks[i].NoteID = note.ID
		}
	}
	doc, err = decodeColleafNotes(ctx, wire, colleafTextContext{})
	if err != nil {
		t.Fatal(err)
	}
	if doc.Notes[2].ID != "edited-id" || len(doc.Diagnostics) != 1 || doc.Diagnostics[0].Code != "colleaf_stale_note_identity" {
		t.Fatal(doc.Notes, doc.Diagnostics)
	}
}
func TestColleafNoteRejectsDanglingAndContradictoryGraph(t *testing.T) {
	ctx := context.Background()
	for _, mutate := range []func(*colleafNoteSnapshot){
		func(w *colleafNoteSnapshot) { delete(w.Notes, w.Body.CustomBlocks[0].NoteID) },
		func(w *colleafNoteSnapshot) { w.Body.CustomBlocks[0].NoteType = Endnote },
		func(w *colleafNoteSnapshot) {
			n := w.Notes["manydoc-note-1"]
			n.ID = "wrong"
			w.Notes["manydoc-note-1"] = n
		},
		func(w *colleafNoteSnapshot) { w.Resources = append(w.Resources, w.Resources[0]) },
		func(w *colleafNoteSnapshot) {
			n := w.Notes["manydoc-note-1"]
			n.Body = w.Body
			w.Notes["manydoc-note-1"] = n
		},
	} {
		wire, err := encodeColleafNotes(ctx, colleafNoteExample())
		if err != nil {
			t.Fatal(err)
		}
		mutate(&wire)
		if _, err = decodeColleafNotes(ctx, wire, colleafTextContext{}); err == nil {
			t.Fatal("invalid note graph accepted")
		}
	}
}

func TestColleafNativeExplicitBodyMarkerIsNotRecursiveCitation(t *testing.T) {
	one := 1
	native := colleafNativeNote{ID: "n", Kind: Footnote, Mark: nil, Body: colleafBody{DataStream: "\bBody\r\n", CustomBlocks: []colleafBlockRecord{{StartIndex: 0, Type: &one, ID: "body-marker", NoteID: "n", NoteType: Footnote}}, TextRuns: []colleafTextRun{{St: 0, Ed: 1, TS: sfAttrs(map[string]any{"it": 1, "fs": 10})}}}}
	wire := colleafNoteSnapshot{Body: colleafBody{DataStream: "Main\b\r\n", CustomBlocks: []colleafBlockRecord{{StartIndex: 4, Type: &one, ID: "reference", NoteID: "n", NoteType: Footnote}}}, Notes: map[string]colleafNativeNote{"n": native}}
	doc, err := decodeColleafNotes(context.Background(), wire, colleafTextContext{})
	if err != nil {
		t.Fatal(err)
	}
	note := doc.Notes[0]
	if len(note.Blocks[0].Runs) != 1 || note.Blocks[0].Runs[0].Text != "Body" || note.BodyLabelStyle == nil || !note.BodyLabelStyle.Italic || note.BodyLabelStyle.FontSize != 10 {
		t.Fatal(note)
	}
	if err = doc.Validate(); err != nil {
		t.Fatal(err)
	}
	native.Body.DataStream = "Text\b\r\n"
	native.Body.CustomBlocks[0].StartIndex = 4
	native.Body.TextRuns = nil
	wire.Notes["n"] = native
	if _, err = decodeColleafNotes(context.Background(), wire, colleafTextContext{}); err == nil {
		t.Fatal("non-leading recursive reference accepted")
	}
}

func FuzzColleafNotes(f *testing.F) {
	wire, err := encodeColleafNotes(context.Background(), colleafNoteExample())
	if err != nil {
		f.Fatal(err)
	}
	data, err := json.Marshal(wire)
	if err != nil {
		f.Fatal(err)
	}
	f.Add(data)
	f.Add([]byte(`{"body":{"dataStream":"\r\n"},"notes":{},"resources":[]}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		var input colleafNoteSnapshot
		if json.Unmarshal(data, &input) != nil {
			return
		}
		doc, err := decodeColleafNotes(context.Background(), input, colleafTextContext{})
		if err == nil {
			if err = validateNotes(doc); err != nil {
				t.Fatal(err)
			}
			if len(doc.Blocks) == 0 {
				t.Fatal("accepted empty body")
			}
		}
	})
}
