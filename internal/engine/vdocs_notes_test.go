package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
)

func vdocsCitationsExample() *Document {
	literal := "II"
	empty := ""
	body := "9"
	basic := func(kind string) Run { return Run{Note: &NoteReference{ID: "same", Kind: kind}} }
	p := func(s string) Block { return Block{Kind: Paragraph, Runs: []Run{{Text: s}}} }
	rich := Run{Note: &NoteReference{ID: "same", Kind: Footnote, LabelContent: []Run{{Text: "*", Bold: true, FontSize: 12, Color: "#FF0000"}, {Text: "A", Italic: true, FontSize: 8, Color: "#006633"}}}}
	return &Document{Metadata: Metadata{Title: "Per-citation native marks"}, Blocks: []Block{
		{Kind: SectionBlock, Section: &Section{Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "A😀 "}, basic(Footnote), {Text: " / "}, {Bold: true, FontSize: 11, Note: &NoteReference{ID: "same", Kind: Footnote, Label: &literal}}, {Text: " / "}, rich, {Text: " / "}, {Note: &NoteReference{ID: "same", Kind: Footnote, Label: &empty}}, {Text: " end"}}}}}},
		{Kind: SectionBlock, Section: &Section{Start: SectionNextPage, Blocks: []Block{
			{Kind: Table, Rows: []Row{{Cells: []Cell{{Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "Cell "}, rich}}}}}}}},
			{Kind: ListBlock, List: "bullet", Items: []ListItem{{Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "List "}, {Note: &NoteReference{ID: "same", Kind: Endnote, Label: &literal, Inferred: true}}}}}}}},
			{Kind: Paragraph, Runs: []Run{{Text: "End "}, basic(Endnote)}},
		}}},
	}, Notes: []Note{{ID: "same", Kind: Footnote, BodyLabel: &body, Blocks: []Block{p("Foot body")}}, {ID: "same", Kind: Endnote, Blocks: []Block{p("End body")}}}}
}
func TestVDOCSLiteralRichCitationsAndSF(t *testing.T) {
	ctx := context.Background()
	want := vdocsCitationsExample()
	wire, err := encodeVDOCSDocument(ctx, want)
	if err != nil {
		t.Fatal(err)
	}
	got, err := decodeVDOCSDocument(ctx, wire)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		a, _ := json.Marshal(got)
		b, _ := json.Marshal(want)
		t.Fatalf("citation semantics changed\ngot%s\nwant%s", a, b)
	}
	sf, err := ToStructured(got)
	if err != nil {
		t.Fatal(err)
	}
	d, err := FromStructured(sf)
	if err != nil {
		t.Fatal(err)
	}
	again, err := encodeVDOCSDocument(ctx, d)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(wire, again) {
		t.Fatal("SF changed native citation text/styles/relationships")
	}
	if file := os.Getenv("MANYDOC_VDOCS_CITATION_QA"); file != "" {
		data, err := json.MarshalIndent(wire, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(file, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if file := os.Getenv("MANYDOC_VDOCS_CITATION_SAVED"); file != "" {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var saved vdocsNoteSnapshot
		if err = json.Unmarshal(data, &saved); err != nil {
			t.Fatal(err)
		}
		got, err = decodeVDOCSDocument(ctx, saved)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want) {
			a, _ := json.Marshal(got)
			b, _ := json.Marshal(want)
			t.Fatalf("facade changed citations\ngot%s\nwant%s", a, b)
		}
	}
	if file := os.Getenv("MANYDOC_VDOCS_CITATION_EDITED"); file != "" {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var saved vdocsNoteSnapshot
		if err = json.Unmarshal(data, &saved); err != nil {
			t.Fatal(err)
		}
		got, err = decodeVDOCSDocument(ctx, saved)
		if err != nil {
			t.Fatal(err)
		}
		want := vdocsCitationsExample()
		changed := "III"
		want.Blocks[0].Section.Blocks[0].Runs[3].Note.Label = &changed
		for _, r := range []Run{want.Blocks[0].Section.Blocks[0].Runs[5], want.Blocks[1].Section.Blocks[0].Rows[0].Cells[0].Blocks[0].Runs[1]} {
			r.Note.LabelContent[0].Bold = false
			r.Note.LabelContent[0].FontSize = 14
		}
		if !reflect.DeepEqual(got, want) {
			a, _ := json.Marshal(got)
			b, _ := json.Marshal(want)
			t.Fatalf("actual citation edits lost\ngot%s\nwant%s", a, b)
		}
	}

}
func FuzzVDOCSCitations(f *testing.F) {
	wire, err := encodeVDOCSDocument(context.Background(), vdocsCitationsExample())
	if err != nil {
		f.Fatal(err)
	}
	data, err := json.Marshal(wire)
	if err != nil {
		f.Fatal(err)
	}
	f.Add(data)
	f.Fuzz(func(t *testing.T, data []byte) {
		var w vdocsNoteSnapshot
		if json.Unmarshal(data, &w) != nil {
			return
		}
		d, err := decodeVDOCSDocument(context.Background(), w)
		if err == nil {
			if err = d.Validate(); err != nil {
				t.Fatal(err)
			}
		}
	})
}

func TestVDOCSLostCitationOwnershipKeepsVisibleLabel(t *testing.T) {
	ctx := context.Background()
	w, err := encodeVDOCSDocument(ctx, vdocsCitationsExample())
	if err != nil {
		t.Fatal(err)
	}
	var ranges []vdocsRangeRecord
	removed := false
	for _, a := range w.Body.CustomRanges {
		var h vdocsCitationMark
		_ = json.Unmarshal(a.Properties,

			// Enabling the actual generated marker invalidates the projected ownership;
			// both it and the visible prefix must remain, rather than dropping one.

			&h)
		if h.Profile == vdocsCitationProfile && !removed {
			removed = true
			continue
		}
		ranges = append(ranges, a)
	}
	w.Body.CustomRanges = ranges
	got, err := decodeVDOCSDocument(ctx, w)
	if err != nil {
		t.

			// Keep just the first authored section; defaults are independently significant.

			Fatal(err)
	}
	refs := got.Blocks[0].Section.Blocks[0].Runs
	if refs[1].Note == nil || refs[1].Note.Label == nil || *refs[1].Note.Label != "" || refs[2].Text != "1" {
		t.Fatal("lost ownership hid or invented a visible mark", refs)
	}
	found := false
	for _, d := range got.Diagnostics {
		found = found || d.Code == "vdocs_stale_citation"
	}
	if !found {
		t.Fatal("ownership loss not disclosed", got.Diagnostics)
	}
}
func TestVDOCSMalformedCitationOwnershipRejects(t *testing.T) {
	for _, mutate := range []func(*vdocsRangeRecord){
		func(a *vdocsRangeRecord) { active := true; a.Active = &active },
		func(a *vdocsRangeRecord) { a.EndIndex = 999999 },
		func(a *vdocsRangeRecord) {
			var h vdocsCitationMark
			_ = json.Unmarshal(a.Properties, &h)
			h.Version = 2
			a.Properties = sfAttrs(h)
		},
	} {
		w, err := encodeVDOCSDocument(context.Background(), vdocsCitationsExample())
		if err != nil {
			t.Fatal(err)
		}
		for i, a := range w.Body.CustomRanges {
			var h vdocsCitationMark
			_ = json.Unmarshal(a.Properties, &h)
			if h.Profile == vdocsCitationProfile {
				mutate(&w.Body.CustomRanges[i])
				break
			}
		}
		if _, err = decodeVDOCSDocument(context.Background(), w); err ==

			// Remove just the first identity binding, retaining visible note body/mark.

			nil {
			t.Fatal("malformed citation ownership accepted")
		}
	}
}

func TestVDOCSLongCitationParagraphPreservesEveryReference(t *testing.T) {
	label := "II"
	d := &Document{Blocks: []Block{{Kind: Paragraph}}, Notes: []Note{{ID: "n", Kind: Footnote, Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "Shared body"}}}}}}}
	for i := 0; i < 2048; i++ {
		ref := &NoteReference{ID: "n", Kind: Footnote}
		if i == 0 {
			ref.Label = &label
		}
		d.Blocks[0].Runs = append(d.Blocks[0].Runs, Run{Note: ref})
	}
	wire, err := encodeVDOCSDocument(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	got, err := decodeVDOCSDocument(context.Background(), wire)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, d) {
		t.Fatal("long paragraph lost reference order/defaults/ownership", len(got.Blocks[0].Runs), got.Diagnostics)
	}
}
func vdocsBodyMarksExample(t testing.TB) *Document {
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
func TestVDOCSIndependentRichBodyMarksAndSF(t *testing.T) {
	ctx := context.Background()
	want := vdocsBodyMarksExample(t)
	wire, err := encodeVDOCSDocument(ctx, want)
	if err != nil {
		t.Fatal(err)
	}
	got, err := decodeVDOCSDocument(ctx, wire)
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
	native, err := encodeVDOCSDocument(ctx, again)
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
	if file := os.Getenv("MANYDOC_VDOCS_BODY_MARK_QA"); file != "" {
		data, err := json.MarshalIndent(wire, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(file, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if file := os.Getenv("MANYDOC_VDOCS_BODY_MARK_SAVED"); file != "" {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var saved vdocsNoteSnapshot
		if err = json.Unmarshal(data, &saved); err != nil {
			t.Fatal(err)
		}
		got, err = decodeVDOCSDocument(ctx, saved)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want) {
			a, _ := json.Marshal(got)
			b, _ := json.Marshal(want)
			t.Fatalf("facade changed body-label semantics\ngot%s\nwant%s", a, b)
		}
	}
	if file := os.Getenv("MANYDOC_VDOCS_BODY_MARK_EDITED"); file != "" {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var saved vdocsNoteSnapshot
		if err = json.Unmarshal(data, &saved); err != nil {
			t.Fatal(err)
		}
		got, err = decodeVDOCSDocument(ctx, saved)
		if err != nil {
			t.Fatal(err)
		}
		want := vdocsBodyMarksExample(t)
		edited := "8"
		want.Notes[0].BodyLabel = &edited
		want.Notes[1].BodyLabelContent[0].Bold = false
		want.Notes[1].BodyLabelContent[0].FontSize = 14
		if !reflect.DeepEqual(got, want) {
			t.Fatal("actual editor edits lost", got.Diagnostics)
		}
	}

}
func TestVDOCSBodyMarkEditsKeepActualTextAndStyles(t *testing.T) {
	ctx := context.Background()
	wire, err := encodeVDOCSDocument(ctx, vdocsBodyMarksExample(t))
	if err != nil {
		t.Fatal(err)
	}
	n := wire.Notes["manydoc-note-1"]
	n.Body.DataStream = strings.Replace(n.Body.DataStream, "9", "8", 1)
	wire.Notes[n.ID] = n
	got, err := decodeVDOCSDocument(ctx, wire)
	if err != nil {
		t.Fatal(err)
	}
	if got.Notes[0].BodyLabel == nil || *got.Notes[0].BodyLabel != "8" || len(got.Diagnostics) != 0 {
		t.Fatal("native label edit lost", got.Notes[0], got.Diagnostics)
	}
	n = wire.Notes["manydoc-note-1"]
	n.Body.TextRuns[0].TS = sfAttrs(map[string]int{"bl": 1})
	wire.Notes[n.ID] = n
	got, err = decodeVDOCSDocument(ctx, wire)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Notes[0].BodyLabelContent) != 1 || got.Notes[0].BodyLabelContent[0].Text != "8" || !got.Notes[0].BodyLabelContent[0].Bold || len(got.Diagnostics) != 1 {
		t.Fatal("edited formatting lost", got.Notes[0], got.Diagnostics)
	}

	n = wire.Notes["manydoc-note-1"]
	show := true
	n.ShowBodyReference = &show
	wire.Notes[n.ID] = n
	got, err = decodeVDOCSDocument(ctx, wire)
	if err != nil {
		t.Fatal(err)
	}
	if got.Notes[0].BodyLabel != nil || !strings.HasPrefix(noteLabelContentText(got.Notes[0].Blocks[0].Runs), "8\tLiteral body") || len(got.Diagnostics) != 1 {
		t.Fatal("stale ownership hid actual body text", got.Notes[0], got.Diagnostics)
	}
}
func TestVDOCSMalformedBodyMarkOwnershipRejects(t *testing.T) {
	for _, mutate := range []func(*vdocsNativeNote){
		func(n *vdocsNativeNote) {
			active := true
			n.Body.CustomRanges[len(n.Body.CustomRanges)-1].Active = &active
		},
		func(n *vdocsNativeNote) {
			a := n.Body.CustomRanges[len(n.Body.CustomRanges)-1]
			a.ID = "duplicate"
			n.Body.CustomRanges = append(n.Body.CustomRanges, a)
		},
		func(n *vdocsNativeNote) { n.Body.CustomRanges[len(n.Body.CustomRanges)-1].EndIndex = 99999 },
		func(n *vdocsNativeNote) {
			n.Body.CustomRanges[len(n.Body.CustomRanges)-1].Properties = json.RawMessage(`{"profile":"manydoc.note-body-mark","version":2,"noteId":"manydoc-note-1","mode":"literal"}`)
		},
	} {
		w, err := encodeVDOCSDocument(context.Background(), vdocsBodyMarksExample(t))
		if err != nil {
			t.Fatal(err)
		}
		n := w.Notes["manydoc-note-1"]
		mutate(&n)
		w.Notes[n.ID] = n
		if _, err = decodeVDOCSDocument(context.Background(), w); err == nil {
			t.Fatal("bad body ownership accepted")
		}
	}
}

func TestVDOCSLostBodyMarkOwnershipRetainsVisiblePrefix(t *testing.T) {
	ctx := context.Background()
	w, err := encodeVDOCSDocument(ctx, vdocsBodyMarksExample(t))
	if err != nil {
		t.Fatal(err)
	}
	n := w.Notes["manydoc-note-1"]
	n.Body.CustomRanges = nil
	w.Notes[n.ID] = n
	got, err := decodeVDOCSDocument(ctx, w)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(noteLabelContentText(got.Notes[0].Blocks[0].Runs), "9\tLiteral body") || got.Notes[0].BodyLabel == nil || *got.Notes[0].BodyLabel != "" || len(got.Diagnostics) != 1 || got.Diagnostics[0].Code != "vdocs_stale_note_body_mark" {
		t.Fatal("missing annotation silently discarded text/semantics", got.Notes[0], got.Diagnostics)
	}
}

func FuzzVDOCSBodyMarks(f *testing.F) {
	wire, err := encodeVDOCSDocument(context.Background(), vdocsBodyMarksExample(f))
	if err != nil {
		f.Fatal(err)
	}
	data, err := json.Marshal(wire)
	if err != nil {
		f.Fatal(err)
	}
	f.Add(data)
	f.Fuzz(func(t *testing.T, data []byte) {
		var input vdocsNoteSnapshot
		if json.Unmarshal(data, &input) != nil {
			return
		}
		doc, err := decodeVDOCSDocument(context.Background(), input)
		if err == nil {
			if err = doc.Validate(); err != nil {
				t.Fatal(err)
			}
		}
	})
}
func vdocsForeignNumberingExample() vdocsNoteSnapshot {
	w := vdocsNoteSnapshot{Body: vdocsBody{TextRuns: []vdocsTextRun{}}, ID: "foreign-numbering", Title: "Native automatic marks", DocumentStyle: json.RawMessage(`{"documentFlavor":1,"pageSize":{"width":800,"height":1000},"marginTop":40,"marginBottom":40,"marginLeft":40,"marginRight":40,"autoHyphenation":0}`), NoteSettings: json.RawMessage(`{"footnote":{"numberFormat":"upperRoman","startNumber":3,"restart":"eachSect","position":"beneathText"},"endnote":{"numberFormat":"upperLetter","startNumber":27,"restart":"continuous","position":"sectEnd"}}`), Notes: map[string]vdocsNativeNote{}}
	var stream strings.Builder
	serial := 0
	offset := 0
	add := func(text string) { stream.WriteString(text); offset += vdocsUTF16Len(text) }
	section := func(id string, refs []string, properties string) {
		add("A😀 ")
		for _, id := range refs {
			kind := Footnote
			typ := 8
			if strings.HasPrefix(id, "e") {
				kind = Endnote
				typ = 9
			}
			serial++
			w.Body.CustomRanges = append(w.Body.CustomRanges, vdocsRangeRecord{ID: "citation-" + string(rune('a'+serial)), StartIndex: offset, EndIndex: offset, Type: typ, Whole: true, Properties: sfAttrs(map[string]string{"noteId": id})})
			add("\ufffc ")
			body := "Body " + id
			w.Notes[id] = vdocsNativeNote{ID: id, Kind: kind, Body: vdocsBody{TextRuns: []vdocsTextRun{}, DataStream: body + "\r\n", Paragraphs: []vdocsParagraphRecord{{ID: "body-" + id, StartIndex: len(body)}}}}
		}
		w.Body.Paragraphs = append(w.Body.Paragraphs, vdocsParagraphRecord{ID: "p-" + id, StartIndex: offset})
		add("\r")
		record := vdocsSectionRecord{ID: id, StartIndex: offset}
		if properties != "" {
			record.Extra = map[string]json.RawMessage{"noteProperties": json.RawMessage(properties)}
		}
		w.Body.SectionBreaks = append(w.Body.SectionBreaks, record)
		add("\n")
	}
	section("one", []string{"f1", "e1", "shared", "shared", "custom", "f2"}, "")
	section("two", []string{"f3", "e2", "shared"}, `{"footnote":{"numberFormat":"decimalZero","startNumber":0,"restart":"eachSect"}}`)
	section("three", []string{"silent"}, `{"footnote":{"numberFormat":"none","startNumber":1,"restart":"eachSect"}}`)
	n := w.Notes["custom"]
	mark := "§"
	n.Mark = &mark
	w.Notes["custom"] = n
	w.Body.DataStream = stream.String()
	return w
}
func TestVDOCSForeignAutomaticNoteNumbers(t *testing.T) {
	w := vdocsForeignNumberingExample()
	doc, err := decodeVDOCSDocument(context.Background(), w)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Layout.HeaderDistance == nil || *doc.Layout.HeaderDistance != 22.5 || doc.Layout.FooterDistance == nil || *doc.Layout.FooterDistance != 22.5 {
		t.Fatal("native omitted-distance default changed", doc.Layout)
	}
	c, err := catalogNotes(doc, false)
	if err != nil {
		t.Fatal(err)
	}
	var labels []string
	if err = walkNoteRuns(doc.Blocks, func(r Run) error {
		if r.Note != nil {
			labels = append(labels, noteReferenceLabel(c, r.Note))
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	want := []string{"III", "AA", "IV", "V", "§", "VI", "00", "BB", "01", ""}
	if !reflect.DeepEqual(labels, want) {
		t.Fatalf("visible native labels changed: got%q want%q", labels, want)
	}
	foundShared := false
	foundSilent := false
	for _, n := range doc.Notes {
		if n.ID == "shared" {
			foundShared = true
			if n.Label != "IV" || n.BodyLabel == nil || *n.BodyLabel != "01" {
				t.Fatal("repeated native body mark lost", n)
			}
		}
		if n.ID == "silent" {
			foundSilent = true
			if n.BodyLabel == nil || *n.BodyLabel != "" {
				t.Fatal("markerless note became automatic", n)
			}
		}
	}
	if !foundShared || !foundSilent || len(doc.Diagnostics) != 2 {
		t.Fatal("native settings/numbering losses not disclosed", doc.Diagnostics)
	}
	sf, err := ToStructured(doc)
	if err != nil {
		t.Fatal(err)
	}
	got, err := FromStructured(sf)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, doc) {
		t.Fatal("SF changed actual note marks")
	}
	if file := os.Getenv("MANYDOC_VDOCS_NUMBER_QA"); file != "" {
		data, err := json.MarshalIndent(w, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(file, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if file := os.Getenv("MANYDOC_VDOCS_NUMBER_SAVED"); file != "" {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var saved vdocsNoteSnapshot
		if err = json.Unmarshal(data, &saved); err != nil {
			t.Fatal(err)
		}
		got, err = decodeVDOCSDocument(context.Background(), saved)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, doc) {
			a, _ := json.Marshal(got)
			b, _ := json.Marshal(doc)
			t.Fatalf("actual facade changed imported note semantics\ngot%s\nwant%s", a, b)
		}
	}
}
func TestVDOCSDefaultNativeEndnoteRomanAndEmptyMark(t *testing.T) {
	w := vdocsForeignNumberingExample()
	w.NoteSettings = nil
	w.Body.SectionBreaks = w.Body.SectionBreaks[:1]
	w.Body.SectionBreaks[0].Extra = nil

	w.Body.DataStream = strings.SplitN(w.Body.DataStream, "\n", 2)[0] + "\n"
	w.Body.Paragraphs = w.Body.Paragraphs[:1]
	w.Body.CustomRanges = w.Body.CustomRanges[:6]
	for _, id := range []string{"f3", "e2", "silent"} {
		delete(w.Notes, id)
	}
	doc, err := decodeVDOCSDocument(context.Background(), w)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range doc.Notes {
		if n.ID == "e1" && n.Label != "i" {
			t.Fatal("native endnote Roman default changed", n.Label)
		}
	}
	n := w.Notes["custom"]
	empty := ""
	n.Mark = &empty
	w.Notes["custom"] = n
	doc, err = decodeVDOCSDocument(context.Background(), w)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range doc.Notes {
		if n.ID == "custom" && (n.BodyLabel == nil || *n.BodyLabel != "") {
			t.Fatal("empty custom mark lost")
		}
	}
}
func TestVDOCSNativeNumberingRejectsUnprovenOrMalformedSettings(t *testing.T) {
	for _, raw := range []string{`{"footnote":{"restart":"eachPage"}}`, `{"footnote":{"startNumber":-1}}`, `{"footnote":{"startNumber":null}}`, `{"endnote":{"columnCount":2}}`, `{"footnote":{"position":"docEnd"}}`, `{"footnote":{"separator":{"dataStream":"Visible separator\r\n"}}}`, `{"footnote":{"numberFormat":"not-a-format"}}`} {
		w := vdocsForeignNumberingExample()
		w.NoteSettings = json.RawMessage(raw)
		if _, err := decodeVDOCSDocument(context.Background(), w); err == nil {
			t.Fatal("unproven setting accepted", raw)
		}
	}
	w := vdocsForeignNumberingExample()
	w.Body.CustomRanges[0], w.Body.CustomRanges[1] = w.Body.CustomRanges[1], w.Body.CustomRanges[0]
	if _, err := decodeVDOCSDocument(context.Background(), w); err == nil {
		t.Fatal("counter order silently changed")
	}
}
func TestVDOCSNativeNoteFormatZeroAndSymbols(t *testing.T) {
	for _, tc := range []struct {
		format string
		value  int
		want   string
	}{{"decimalZero", 0, "00"}, {"lowerRoman", 0, ""}, {"upperLetter", 0, ""}, {"chicago", 7, "**"}, {"decimalFullWidth", 12, "１２"}, {"ideographDigital", 10, "一〇"}, {"decimalEnclosedCircle", 36, "㊱"}, {"ideographZodiac", 12, "亥"}, {"upperRoman", 14, "XIV"}, {"lowerLetter", 27, "aa"}, {"decimalFullWidth2", 0, "０"}, {"taiwaneseDigital", 0, "〇"}, {"decimalEnclosedCircle", 51, "51"}, {"ideographEnclosedCircle", 10, "㈩"}, {"ideographTraditional", 10, "癸"}} {
		got, err := vdocsNoteNumber(tc.format, tc.value)
		if err != nil || got != tc.want {
			t.Fatal(tc, got, err)
		}
	}
}

func TestVDOCSNativeLiteralMarksDoNotRequirePageNumberInference(t *testing.T) {
	w := vdocsForeignNumberingExample()
	w.NoteSettings = json.RawMessage(`{"footnote":{"restart":"eachPage","columnCount":2,"position":"beneathText"}}`)

	// Dropping or qualifying source bindings alone does not edit note semantics.

	for id, n := range w.Notes {
		label := "fixed-" + id
		n.Mark = &label
		w.Notes[id] = n
	}
	for i := range w.Body.SectionBreaks {
		w.Body.SectionBreaks[i].Extra = nil
	}
	doc, err := decodeVDOCSDocument(context.Background(), w)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Diagnostics) != 1 || doc.Diagnostics[0].Code != "vdocs_note_settings" {
		t.Fatal(doc.Diagnostics)
	}
}
func TestVDOCSImportedNativeMarksUseSharedOutputPaths(t *testing.T) {
	ctx := context.Background()
	doc, err := decodeVDOCSDocument(ctx, vdocsForeignNumberingExample())
	if err != nil {
		t.Fatal(err)
	}
	font, err := os.ReadFile("testdata/assets/DejaVuSans.ttf")
	if err != nil {
		t.Fatal(err)
	}
	bold, err := os.ReadFile("testdata/assets/DejaVuSans-Bold.ttf")
	if err != nil {
		t.Fatal(err)
	}
	options := WriteOptions{AllowLossy: true, PDFFonts: []FontFace{{Family: "DejaVu", Data: font}, {Family: "DejaVu", Data: bold, Bold: true}}, PDFFontMap: map[string]string{"Go": "DejaVu"}}
	for _, format := range []Format{DOCX, PDF} {
		t.Run(string(format), func(t *testing.T) {
			var out bytes.Buffer
			if err := Write(ctx, &out, doc, format, options); err != nil {
				t.Fatal(err)
			}
			if format == DOCX {
				back, err := Read(ctx, bytes.NewReader(out.Bytes()), DOCX, ReadOptions{})
				if err != nil {
					t.Fatal(err)
				}
				c, err := catalogNotes(back, false)
				if err != nil {
					t.Fatal(err)
				}
				var labels []string
				if err = walkNoteRuns(back.Blocks, func(r Run) error {
					if r.Note != nil {
						labels = append(labels, noteReferenceLabel(c, r.Note))
					}
					return nil
				}); err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(labels, []string{"III", "AA", "IV", "V", "§", "VI", "00", "BB", "01", ""}) {
					t.Fatal("Word conversion changed native marks", labels)
				}
			}
			if file := os.Getenv("MANYDOC_VDOCS_NUMBER_OUTPUT"); file != "" {
				if err = os.WriteFile(file+"."+string(format), out.Bytes(), 0600); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}
func vdocsNoteOriginExample(t *testing.T) *Document {
	base := vdocsOriginExample(t)
	source := func(id string) *SourceInfo { s := cloneSourceInfo(base.Blocks[0].Source); s.Element = id; return s }
	return &Document{Metadata: Metadata{Title: "Note origins"}, Pages: base.Pages, Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "😀 Notes ", Bold: true, FontSize: 13}, {Note: &NoteReference{Kind: Footnote, ID: "n"}}, {Text: " / "}, {Note: &NoteReference{Kind: Endnote, ID: "n"}}, {Text: " / "}, {Note: &NoteReference{Kind: Footnote, ID: "inference"}}}}}, Notes: []Note{
		{Kind: Footnote, ID: "n", KindInferred: true, Label: "*", BodyLabelContent: []Run{{Text: "α", Bold: true, FontSize: 13}}, Source: source("foot-definition"), LabelSource: source("foot-label"), Blocks: []Block{{Kind: Paragraph, Source: source("foot-paragraph"), Runs: []Run{{Text: "Footnote body"}}}}},
		{Kind: Endnote, ID: "n", Label: "!", Source: source("end-definition"), Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "Endnote body"}}}}},
		{Kind: Footnote, ID: "inference", KindInferred: true, Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "Inferred placement"}}}}},
	}}
}
func TestVDOCSNoteOriginsAndActualSave(t *testing.T) {
	ctx := context.Background()
	want := vdocsNoteOriginExample(t)
	native, err := encodeVDOCSDocument(ctx, want)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(native)
	got, err := decodeVDOCSDocument(ctx, native)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(native)
	if !bytes.Equal(before, after) {
		t.Fatal("input mutated")
	}
	assertEvidenceSF(t, want, got)
	got.Notes[0].Source.StructureID[0] = 0
	got, err = decodeVDOCSDocument(ctx, native)
	if err != nil {
		t.Fatal(err)
	}
	assertEvidenceSF(t, want, got)
	if file := os.Getenv("MANYDOC_VDOCS_NOTE_ORIGIN_QA"); file != "" {
		if err = os.WriteFile(file, before, 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, spec := range []struct{ env, mode string }{{"MANYDOC_VDOCS_NOTE_ORIGIN_SAVED", "saved"}, {"MANYDOC_VDOCS_NOTE_ORIGIN_BODY_EDITED", "body"}, {"MANYDOC_VDOCS_NOTE_ORIGIN_MARK_EDITED", "mark"}, {"MANYDOC_VDOCS_NOTE_ORIGIN_DETACHED", "detach"}} {
		t.Run(spec.mode, func(t *testing.T) {
			file := os.Getenv(spec.env)
			if file == "" {
				t.Skip("optional independent reader artifact")
			}
			raw, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			var wire vdocsNoteSnapshot
			if err = json.Unmarshal(raw, &wire); err != nil {
				t.Fatal(err)
			}
			envelope, err := json.Marshal(vdocsEnvelope{vdocsEnvelopeType, 3, "univer", vdocsEngineVersion, wire})
			if err != nil {
				t.Fatal(err)
			}
			actual, err := readVDOCS(ctx, bytes.NewReader(envelope))
			if err != nil {
				t.Fatal(err)
			}
			expected := vdocsNoteOriginExample(t)
			switch spec.mode {
			case "body":
				expected.Notes[0].Blocks[0].Runs = []Run{{Text: "Q"}, {Text: "Footnote body"}}
				expected.Notes[0].Blocks[0].Source.Historical = true
				expected.Notes[0].Source.Historical = true
				expected.Diagnostics = []Diagnostic{{Code: "vdocs_source_historical", Message: "Original PDF source facts are retained as historical after current paragraph content or presentation changed"}, {Code: "vdocs_note_source_historical", Message: "Original note source facts are retained as historical after current body or marker semantics changed"}}
			case "mark":
				expected.Notes[0].BodyLabelContent[0].Text = "β"
				expected.Notes[0].Source.Historical = true
				expected.Notes[0].LabelSource.Historical = true
				expected.Diagnostics = []Diagnostic{{Code: "vdocs_note_source_historical", Message: "Original note source facts are retained as historical after current body or marker semantics changed"}, {Code: "vdocs_note_label_source_historical", Message: "Original note-label source facts are retained as historical after current marker semantics changed"}}
			case "detach":
				expected.Notes[2].ID = "manydoc-note-3"
				expected.Notes[2].KindInferred = false
				expected.Notes[2].Label = "1"
				expected.Blocks[0].Runs[5].Note.ID = "manydoc-note-3"
				expected.Diagnostics = []Diagnostic{{Code: "vdocs_note_source_unbound", Message: "Original footnote \"inference\" source facts are retained unbound after its native identity binding disappeared"}, {Code: "vdocs_note_kind_history", Message: "Original footnote \"inference\" placement was inferred; no current note-kind assertion is made"}}
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
			next, err := encodeVDOCSDocument(ctx, actual)
			if err != nil {
				t.Fatal(err)
			}
			stable, err := decodeVDOCSDocument(ctx, next)
			if err != nil {
				t.Fatal(err)
			}
			assertEvidenceSF(t, actual, stable)
		})
	}
}
func TestVDOCSNoteOriginsMalformedAndUnbound(t *testing.T) {
	ctx := context.Background()
	want := vdocsNoteOriginExample(t)
	native, err := encodeVDOCSDocument(ctx, want)
	if err != nil {
		t.Fatal(err)
	}
	opened, err := openVDOCSIntent(ctx, native)
	if err != nil {
		t.Fatal(err)
	}
	var intent vdocsDocumentIntent
	if err = vdocsResourceMap(opened.Resources, vdocsDocumentResource, &intent); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"version", "fingerprint", "source", "duplicate"} {
		t.Run(name, func(t *testing.T) {
			raw, _ := json.Marshal(intent)
			var copy vdocsDocumentIntent
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
			wire.Resources = vdocsReplaceResource(wire.Resources, vdocsDocumentResource, copy)
			if _, err = decodeVDOCSDocument(ctx, wire); err == nil {
				t.Fatal("malformed source record accepted")
			}
		})
	}

	var ids vdocsNoteIdentities
	if err = vdocsResourceMap(opened.Resources, vdocsNoteIdentityResource, &ids); err != nil {
		t.Fatal(err)
	}
	ids.Bindings = ids.Bindings[1:]
	opened.Resources = vdocsReplaceResource(opened.Resources, vdocsNoteIdentityResource, ids)
	got, err := decodeVDOCSDocument(ctx, opened)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.UnboundSources) != 2 || got.Notes[2].Source != nil || got.Notes[2].LabelSource != nil || got.Notes[2].KindInferred {
		t.Fatal("lost identity incorrectly asserts current note provenance", got)
	}
	again, err := encodeVDOCSDocument(ctx, got)
	if err != nil {
		t.Fatal(err)
	}
	stable, err := decodeVDOCSDocument(ctx, again)
	if err != nil {
		t.Fatal(err)
	}
	assertEvidenceSF(t, got, stable)
}

func FuzzVDOCSNoteOrigins(f *testing.F) {
	seed := vdocsNoteOrigin{Version: 1, NativeID: "native", Kind: Footnote, ID: "n", KindInferred: true, BodyFingerprint: strings.Repeat("0", 64), LabelFingerprint: strings.Repeat("1", 64)}
	f.Add([]byte(sfAttrs(seed)))
	f.Add([]byte(`{"version":2}`))
	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) > 32768 {
			return
		}
		if validateTypedJSON(context.Background(), raw, reflect.TypeFor[vdocsNoteOrigin]()) != nil {
			return
		}
		var origin vdocsNoteOrigin
		if strictSFAttrs(raw, &origin) != nil {
			return
		}
		_ = origin.validate(nil)
	})
}

func TestVDOCSNoteSourceFreeFingerprints(t *testing.T) {
	d := vdocsNoteOriginExample(t)
	d.Notes[0].Blocks[0].Runs[0].Source = cloneSourceInfo(d.Notes[0].Source)
	d.Notes[0].BodyLabelContent[0].Source = cloneSourceInfo(d.Notes[0].LabelSource)
	sf, err := ToStructured(d)
	if err != nil {
		t.Fatal(err)
	}
	body, label, err := vdocsNoteFingerprints(sf.Attrs.Notes[0])
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
	sameBody, sameLabel, err := vdocsNoteFingerprints(clean.Attrs.Notes[0])
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
	editBody, editLabel, err := vdocsNoteFingerprints(edited.Attrs.Notes[0])
	if err != nil {
		t.Fatal(err)
	}
	if body == editBody || label == editLabel {
		t.Fatal("visible edits did not change fingerprints")
	}
}
func vdocsNoteExample() *Document {
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
func TestVDOCSNoteIdentityReferenceBodyAndSFRoundTrip(t *testing.T) {
	ctx := context.Background()
	original := vdocsNoteExample()
	original.Blocks[0].Runs[1].Bold = true
	original.Blocks[0].Runs[1].FontSize = 12
	original.Blocks[0].Runs[1].Language = "en-US"
	wire, err := encodeVDOCSNotes(ctx, original)
	if err != nil {
		t.Fatal(err)
	}
	if len(wire.Body.CustomRanges) != 5 || len(wire.Notes) != 3 || wire.Body.CustomRanges[0].StartIndex != 3 {
		t.Fatal("note range identity/UTF-16 anchors", wire.Body.CustomRanges)
	}
	doc, err := decodeVDOCSNotes(ctx, wire, vdocsTextContext{})
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
	wireAgain, err := encodeVDOCSNotes(ctx, through)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(wire, wireAgain) {
		t.Fatal("SF changed native text/marks/notes/relations")
	}
	if file := os.Getenv("MANYDOC_VDOCS_NOTE_QA"); file != "" {
		data, err := json.MarshalIndent(wire, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(file, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if file := os.Getenv("MANYDOC_VDOCS_NOTE_EDIT"); file != "" {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var edited vdocsNoteSnapshot
		if err = json.Unmarshal(data, &edited); err != nil {
			t.Fatal(err)
		}
		doc, err = decodeVDOCSNotes(ctx, edited, vdocsTextContext{})
		if err != nil {
			t.Fatal(err)
		}
		if noteLabelContentText(doc.Notes[1].Blocks[0].Runs) != "Edit body" || doc.Notes[2].Label != "†" {
			t.Fatal("saved native note edits did not win", doc.Notes)
		}
	}
}
func TestVDOCSNoteNativeIdentityAndStaleBindings(t *testing.T) {
	ctx := context.Background()
	wire, err := encodeVDOCSNotes(ctx, vdocsNoteExample())
	if err != nil {
		t.Fatal(err)
	}
	wire, err = openVDOCSIntent(ctx, wire)
	if err != nil {
		t.Fatal(err)
	}
	wire.Resources = nil
	doc, err := decodeVDOCSNotes(ctx, wire, vdocsTextContext{})
	if err != nil {
		t.Fatal(err)
	}
	if doc.Notes[0].ID != "manydoc-note-1" || doc.Notes[0].Kind != Endnote {
		t.Fatal(doc.Notes)
	}
	wire, err = encodeVDOCSNotes(ctx, vdocsNoteExample())
	if err != nil {
		t.Fatal(err)
	}
	note := wire.Notes["manydoc-note-3"]
	delete(wire.Notes, "manydoc-note-3")
	note.ID = "edited-id"
	wire.Notes[note.ID] = note
	for i, a := range wire.Body.CustomRanges {
		if a.Type == 8 && string(a.Properties) == `{"noteId":"manydoc-note-3"}` {
			wire.Body.CustomRanges[i].Properties = sfAttrs(map[string]string{"noteId": note.ID})
		}
	}
	doc, err = decodeVDOCSNotes(ctx, wire, vdocsTextContext{})
	if err != nil {
		t.Fatal(err)
	}
	if doc.Notes[2].ID != "edited-id" || len(doc.Diagnostics) != 1 || doc.Diagnostics[0].Code != "vdocs_stale_note_identity" {
		t.Fatal(doc.Notes, doc.Diagnostics)
	}
}
func TestVDOCSNoteRejectsDanglingAndContradictoryGraph(t *testing.T) {
	ctx := context.Background()
	for _, mutate := range []func(*vdocsNoteSnapshot){
		func(w *vdocsNoteSnapshot) { delete(w.Notes, "manydoc-note-2") },
		func(w *vdocsNoteSnapshot) { w.Body.CustomRanges[0].Type = 9 },
		func(w *vdocsNoteSnapshot) {
			n := w.Notes["manydoc-note-1"]
			n.ID = "wrong"
			w.Notes["manydoc-note-1"] = n
		},
		func(w *vdocsNoteSnapshot) {
			var err error
			*w, err = openVDOCSIntent(ctx, *w)
			if err != nil {
				t.Fatal(err)
			}
			w.Resources = append(w.Resources, w.Resources[0])
		},
		func(w *vdocsNoteSnapshot) {
			n := w.Notes["manydoc-note-1"]
			n.Body = w.Body
			w.Notes["manydoc-note-1"] = n
		},
	} {
		wire, err := encodeVDOCSNotes(ctx, vdocsNoteExample())
		if err != nil {
			t.Fatal(err)
		}
		mutate(&wire)
		if _, err = decodeVDOCSNotes(ctx, wire, vdocsTextContext{}); err == nil {
			t.Fatal("invalid note graph accepted")
		}
	}
}

func TestVDOCSNativeExplicitBodyMarkerIsNotRecursiveCitation(t *testing.T) {
	one := 1
	native := vdocsNativeNote{ID: "n", Kind: Footnote, Mark: nil, Body: vdocsBody{DataStream: "\bBody\r\n", CustomBlocks: []vdocsBlockRecord{{StartIndex: 0, Type: &one, ID: "body-marker", NoteID: "n", NoteType: Footnote}}, TextRuns: []vdocsTextRun{{St: 0, Ed: 1, TS: sfAttrs(map[string]any{"it": 1, "fs": 10})}}}}
	wire := vdocsNoteSnapshot{Body: vdocsBody{DataStream: "Main\b\r\n", CustomBlocks: []vdocsBlockRecord{{StartIndex: 4, Type: &one, ID: "reference", NoteID: "n", NoteType: Footnote}}}, Notes: map[string]vdocsNativeNote{"n": native}}
	doc, err := decodeVDOCSNotes(context.Background(), wire, vdocsTextContext{})
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
	if _, err = decodeVDOCSNotes(context.Background(), wire, vdocsTextContext{}); err == nil {
		t.Fatal("non-leading recursive reference accepted")
	}
}

func FuzzVDOCSNotes(f *testing.F) {
	wire, err := encodeVDOCSNotes(context.Background(), vdocsNoteExample())
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
		var input vdocsNoteSnapshot
		if json.Unmarshal(data, &input) != nil {
			return
		}
		doc, err := decodeVDOCSNotes(context.Background(), input, vdocsTextContext{})
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

func TestVDOCSNativeNoteRangesAndChangedAutomaticOrder(t *testing.T) {
	ctx := context.Background()
	d := &Document{Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Note: &NoteReference{Kind: Footnote, ID: "a"}}, {Note: &NoteReference{Kind: Footnote, ID: "b"}}, {Note: &NoteReference{Kind: Footnote, ID: "a"}}}}}, Notes: []Note{{Kind: Footnote, ID: "a", Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "A"}}}}}, {Kind: Footnote, ID: "b", Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "B"}}}}}}}
	wire, err := encodeVDOCSNotes(ctx, d)
	if err != nil {
		t.Fatal(err)
	}
	if wire.Body.DataStream != "\ufffc\ufffc\ufffc\r\n" || len(wire.Body.CustomBlocks) != 0 {
		t.Fatal("non-rendering custom block note projection")
	}
	if *wire.Notes["manydoc-note-1"].Mark != "1" || *wire.Notes["manydoc-note-2"].Mark != "2" {
		t.Fatal("shared automatic label not stabilized")
	}
	a, b := wire.Body.CustomRanges[0].Properties, wire.Body.CustomRanges[1].Properties
	wire.Body.CustomRanges[0].Properties = b
	wire.Body.CustomRanges[1].Properties = a
	got, err := decodeVDOCSNotes(ctx, wire, vdocsTextContext{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Notes[0].Label != "1" || got.Notes[1].Label != "2" || len(got.Diagnostics) != 1 || got.Diagnostics[0].Code != "vdocs_stale_note_numbering" {
		t.Fatal("reordered native marks replaced by old automatic intent", got.Notes, got.Diagnostics)
	}
	for _, mutate := range []func(*vdocsNoteSnapshot){
		func(w *vdocsNoteSnapshot) { w.Body.CustomRanges[0].EndIndex++ },
		func(w *vdocsNoteSnapshot) {
			w.Body.CustomRanges[0].Properties = json.RawMessage(`{"noteId":"absent"}`)
		},
		func(w *vdocsNoteSnapshot) { w.Body.CustomRanges[0].Type = 9 },
		func(w *vdocsNoteSnapshot) {
			w.Body.CustomRanges[0].Properties = json.RawMessage(`{"noteId":"manydoc-note-1","hidden":true}`)
		},
		func(w *vdocsNoteSnapshot) {
			a := w.Body.CustomRanges[0]
			a.ID = "overlap"
			w.Body.CustomRanges = append(w.Body.CustomRanges, a)
		},
	} {
		wire, err := encodeVDOCSNotes(ctx, d)
		if err != nil {
			t.Fatal(err)
		}
		mutate(&wire)
		if _, err = decodeVDOCSNotes(ctx, wire, vdocsTextContext{}); err == nil {
			t.Fatal("invalid note range accepted")
		}
	}
}
