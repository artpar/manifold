package manifold

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
)

func colleafBodyMarksExample(t testing.TB) *Document {
	p := func(s string) Block { return Block{Kind: Paragraph, Runs: []Run{{Text: s}}} }
	ref := func(id string) Run { return Run{Note: &NoteReference{Kind: Footnote, ID: id}} }
	independent := "9"
	empty := ""
	return &Document{Metadata: Metadata{Title: "Editable body marks"}, Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "Citations "}, ref("literal"), {Text: " / "}, ref("rich"), {Text: " / "}, ref("style"), {Text: " / "}, ref("table"), {Text: " / "}, ref("empty"), {Text: " / "}, ref("empty-style")}}}, Notes: []Note{
		{ID: "literal", Kind: Footnote, BodyLabel: &independent, Blocks: []Block{p("Literal body")}},
		{ID: "rich", Kind: Footnote, BodyLabelContent: []Run{{Text: "*", Bold: true, FontSize: 12, Color: "#FF0000"}, {Text: "2", Italic: true, FontSize: 8, Link: "https://example.org/note"}}, Blocks: []Block{p("Rich body")}},
		{ID: "style", Kind: Footnote, BodyLabelStyle: &Run{Bold: true, Underline: true, FontSize: 10, Color: "#006633"}, Blocks: []Block{p("Styled body")}},
		{ID: "table", Kind: Footnote, BodyLabel: &independent, Blocks: []Block{{Kind: Table, ColumnWidths: []float64{72}, Rows: []Row{{Cells: []Cell{{Blocks: []Block{p("Table body")}}}}}}}},
		{ID: "empty", Kind: Footnote, BodyLabel: &independent},
		{ID: "empty-style", Kind: Footnote, BodyLabel: &empty, BodyLabelStyle: &Run{Italic: true, Color: "#FF0000"}, Blocks: []Block{p("Markerless styled intent")}},
	}}
}
func TestColleafIndependentRichBodyMarksAndSF(t *testing.T) {
	ctx := context.Background()
	want := colleafBodyMarksExample(t)
	wire, err := encodeColleafDocument(ctx, want)
	if err != nil {
		t.Fatal(err)
	}
	got, err := decodeColleafDocument(ctx, wire)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		a, _ := json.Marshal(got)
		b, _ := json.Marshal(want)
		t.Fatalf("body mark semantics changed\ngot%s\nwant%s", a, b)
	}
	sf, err := ToStructured(got)
	if err != nil {
		t.Fatal(err)
	}
	again, err := FromStructured(sf)
	if err != nil {
		t.Fatal(err)
	}
	native, err := encodeColleafDocument(ctx, again)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(native, wire) {
		t.Fatal("SF changed real body-label source records")
	}
	for id, n := range wire.Notes {
		if n.ShowBodyReference == nil || *n.ShowBodyReference {
			t.Fatal("duplicate generated body marker", id)
		}
	}
	if file := os.Getenv("MANYDOC_COLLEAF_BODY_MARK_QA"); file != "" {
		data, err := json.MarshalIndent(wire, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(file, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if file := os.Getenv("MANYDOC_COLLEAF_BODY_MARK_SAVED"); file != "" {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var saved colleafNoteSnapshot
		if err = json.Unmarshal(data, &saved); err != nil {
			t.Fatal(err)
		}
		got, err = decodeColleafDocument(ctx, saved)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want) {
			a, _ := json.Marshal(got)
			b, _ := json.Marshal(want)
			t.Fatalf("facade changed body-label semantics\ngot%s\nwant%s", a, b)
		}
	}
	if file := os.Getenv("MANYDOC_COLLEAF_BODY_MARK_EDITED"); file != "" {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var saved colleafNoteSnapshot
		if err = json.Unmarshal(data, &saved); err != nil {
			t.Fatal(err)
		}
		got, err = decodeColleafDocument(ctx, saved)
		if err != nil {
			t.Fatal(err)
		}
		want := colleafBodyMarksExample(t)
		edited := "8"
		want.Notes[0].BodyLabel = &edited
		want.Notes[1].BodyLabelContent[0].Bold = false
		want.Notes[1].BodyLabelContent[0].FontSize = 14
		if !reflect.DeepEqual(got, want) {
			t.Fatal("actual editor edits lost", got.Diagnostics)
		}
	}

}
func TestColleafBodyMarkEditsKeepActualTextAndStyles(t *testing.T) {
	ctx := context.Background()
	wire, err := encodeColleafDocument(ctx, colleafBodyMarksExample(t))
	if err != nil {
		t.Fatal(err)
	}
	n := wire.Notes["manydoc-note-1"]
	n.Body.DataStream = strings.Replace(n.Body.DataStream, "9", "8", 1)
	wire.Notes[n.ID] = n
	got, err := decodeColleafDocument(ctx, wire)
	if err != nil {
		t.Fatal(err)
	}
	if got.Notes[0].BodyLabel == nil || *got.Notes[0].BodyLabel != "8" || len(got.Diagnostics) != 0 {
		t.Fatal("native label edit lost", got.Notes[0], got.Diagnostics)
	}
	n = wire.Notes["manydoc-note-1"]
	n.Body.TextRuns[0].TS = sfAttrs(map[string]int{"bl": 1})
	wire.Notes[n.ID] = n
	got, err = decodeColleafDocument(ctx, wire)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Notes[0].BodyLabelContent) != 1 || got.Notes[0].BodyLabelContent[0].Text != "8" || !got.Notes[0].BodyLabelContent[0].Bold || len(got.Diagnostics) != 1 {
		t.Fatal("edited formatting lost", got.Notes[0], got.Diagnostics)
	}
	// Enabling the actual generated marker invalidates the projected ownership;
	// both it and the visible prefix must remain, rather than dropping one.
	n = wire.Notes["manydoc-note-1"]
	show := true
	n.ShowBodyReference = &show
	wire.Notes[n.ID] = n
	got, err = decodeColleafDocument(ctx, wire)
	if err != nil {
		t.Fatal(err)
	}
	if got.Notes[0].BodyLabel != nil || !strings.HasPrefix(noteLabelContentText(got.Notes[0].Blocks[0].Runs), "8\tLiteral body") || len(got.Diagnostics) != 1 {
		t.Fatal("stale ownership hid actual body text", got.Notes[0], got.Diagnostics)
	}
}
func TestColleafMalformedBodyMarkOwnershipRejects(t *testing.T) {
	for _, mutate := range []func(*colleafNativeNote){
		func(n *colleafNativeNote) {
			active := true
			n.Body.CustomRanges[len(n.Body.CustomRanges)-1].Active = &active
		},
		func(n *colleafNativeNote) {
			a := n.Body.CustomRanges[len(n.Body.CustomRanges)-1]
			a.ID = "duplicate"
			n.Body.CustomRanges = append(n.Body.CustomRanges, a)
		},
		func(n *colleafNativeNote) { n.Body.CustomRanges[len(n.Body.CustomRanges)-1].EndIndex = 99999 },
		func(n *colleafNativeNote) {
			n.Body.CustomRanges[len(n.Body.CustomRanges)-1].Properties = json.RawMessage(`{"profile":"manydoc.note-body-mark","version":2,"noteId":"manydoc-note-1","mode":"literal"}`)
		},
	} {
		w, err := encodeColleafDocument(context.Background(), colleafBodyMarksExample(t))
		if err != nil {
			t.Fatal(err)
		}
		n := w.Notes["manydoc-note-1"]
		mutate(&n)
		w.Notes[n.ID] = n
		if _, err = decodeColleafDocument(context.Background(), w); err == nil {
			t.Fatal("bad body ownership accepted")
		}
	}
}

func TestColleafLostBodyMarkOwnershipRetainsVisiblePrefix(t *testing.T) {
	ctx := context.Background()
	w, err := encodeColleafDocument(ctx, colleafBodyMarksExample(t))
	if err != nil {
		t.Fatal(err)
	}
	n := w.Notes["manydoc-note-1"]
	n.Body.CustomRanges = nil
	w.Notes[n.ID] = n
	got, err := decodeColleafDocument(ctx, w)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(noteLabelContentText(got.Notes[0].Blocks[0].Runs), "9\tLiteral body") || got.Notes[0].BodyLabel == nil || *got.Notes[0].BodyLabel != "" || len(got.Diagnostics) != 1 || got.Diagnostics[0].Code != "colleaf_stale_note_body_mark" {
		t.Fatal("missing annotation silently discarded text/semantics", got.Notes[0], got.Diagnostics)
	}
}

func FuzzColleafBodyMarks(f *testing.F) {
	wire, err := encodeColleafDocument(context.Background(), colleafBodyMarksExample(f))
	if err != nil {
		f.Fatal(err)
	}
	data, err := json.Marshal(wire)
	if err != nil {
		f.Fatal(err)
	}
	f.Add(data)
	f.Fuzz(func(t *testing.T, data []byte) {
		var input colleafNoteSnapshot
		if json.Unmarshal(data, &input) != nil {
			return
		}
		doc, err := decodeColleafDocument(context.Background(), input)
		if err == nil {
			if err = doc.Validate(); err != nil {
				t.Fatal(err)
			}
		}
	})
}
