package manydoc

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

func colleafCitationsExample() *Document {
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
func TestColleafLiteralRichCitationsAndSF(t *testing.T) {
	ctx := context.Background()
	want := colleafCitationsExample()
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
	again, err := encodeColleafDocument(ctx, d)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(wire, again) {
		t.Fatal("SF changed native citation text/styles/relationships")
	}
	if file := os.Getenv("MANYDOC_COLLEAF_CITATION_QA"); file != "" {
		data, err := json.MarshalIndent(wire, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(file, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if file := os.Getenv("MANYDOC_COLLEAF_CITATION_SAVED"); file != "" {
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
			t.Fatalf("facade changed citations\ngot%s\nwant%s", a, b)
		}
	}
	if file := os.Getenv("MANYDOC_COLLEAF_CITATION_EDITED"); file != "" {
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
		want := colleafCitationsExample()
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
func FuzzColleafCitations(f *testing.F) {
	wire, err := encodeColleafDocument(context.Background(), colleafCitationsExample())
	if err != nil {
		f.Fatal(err)
	}
	data, err := json.Marshal(wire)
	if err != nil {
		f.Fatal(err)
	}
	f.Add(data)
	f.Fuzz(func(t *testing.T, data []byte) {
		var w colleafNoteSnapshot
		if json.Unmarshal(data, &w) != nil {
			return
		}
		d, err := decodeColleafDocument(context.Background(), w)
		if err == nil {
			if err = d.Validate(); err != nil {
				t.Fatal(err)
			}
		}
	})
}

func TestColleafLostCitationOwnershipKeepsVisibleLabel(t *testing.T) {
	ctx := context.Background()
	w, err := encodeColleafDocument(ctx, colleafCitationsExample())
	if err != nil {
		t.Fatal(err)
	}
	var ranges []colleafRangeRecord
	removed := false
	for _, a := range w.Body.CustomRanges {
		var h colleafCitationMark
		_ = json.Unmarshal(a.Properties, &h)
		if h.Profile == colleafCitationProfile && !removed {
			removed = true
			continue
		}
		ranges = append(ranges, a)
	}
	w.Body.CustomRanges = ranges
	got, err := decodeColleafDocument(ctx, w)
	if err != nil {
		t.Fatal(err)
	}
	refs := got.Blocks[0].Section.Blocks[0].Runs
	if refs[1].Note == nil || refs[1].Note.Label == nil || *refs[1].Note.Label != "" || refs[2].Text != "1" {
		t.Fatal("lost ownership hid or invented a visible mark", refs)
	}
	found := false
	for _, d := range got.Diagnostics {
		found = found || d.Code == "colleaf_stale_citation"
	}
	if !found {
		t.Fatal("ownership loss not disclosed", got.Diagnostics)
	}
}
func TestColleafMalformedCitationOwnershipRejects(t *testing.T) {
	for _, mutate := range []func(*colleafRangeRecord){
		func(a *colleafRangeRecord) { active := true; a.Active = &active },
		func(a *colleafRangeRecord) { a.EndIndex = 999999 },
		func(a *colleafRangeRecord) {
			var h colleafCitationMark
			_ = json.Unmarshal(a.Properties, &h)
			h.Version = 2
			a.Properties = sfAttrs(h)
		},
	} {
		w, err := encodeColleafDocument(context.Background(), colleafCitationsExample())
		if err != nil {
			t.Fatal(err)
		}
		for i, a := range w.Body.CustomRanges {
			var h colleafCitationMark
			_ = json.Unmarshal(a.Properties, &h)
			if h.Profile == colleafCitationProfile {
				mutate(&w.Body.CustomRanges[i])
				break
			}
		}
		if _, err = decodeColleafDocument(context.Background(), w); err == nil {
			t.Fatal("malformed citation ownership accepted")
		}
	}
}

func TestColleafLongCitationParagraphPreservesEveryReference(t *testing.T) {
	label := "II"
	d := &Document{Blocks: []Block{{Kind: Paragraph}}, Notes: []Note{{ID: "n", Kind: Footnote, Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "Shared body"}}}}}}}
	for i := 0; i < 2048; i++ {
		ref := &NoteReference{ID: "n", Kind: Footnote}
		if i == 0 {
			ref.Label = &label
		}
		d.Blocks[0].Runs = append(d.Blocks[0].Runs, Run{Note: ref})
	}
	wire, err := encodeColleafDocument(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	got, err := decodeColleafDocument(context.Background(), wire)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, d) {
		t.Fatal("long paragraph lost reference order/defaults/ownership", len(got.Blocks[0].Runs), got.Diagnostics)
	}
}
