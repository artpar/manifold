package manydoc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"testing"
)

func colleafNavigationExample(t testing.TB) *Document {
	return &Document{Metadata: Metadata{Title: "Native point navigation"}, Blocks: []Block{
		{Kind: Paragraph, Anchors: []string{"paragraph"}, Runs: []Run{{Anchor: "before"}, {Text: "😀A", Bold: true, Color: "#123456", FontSize: 13}, {Anchor: "middle"}, {Text: " end"}, {Anchor: "end"}}},
		{Kind: Heading, Level: 1, Anchors: []string{"heading"}, Runs: []Run{{Text: "Heading"}}},
		{Kind: Paragraph, Anchors: []string{"empty"}},
		{Kind: ListBlock, List: "number", Items: []ListItem{{Blocks: []Block{{Kind: Paragraph, Anchors: []string{"list"}, Runs: []Run{{Text: "Item 😀"}}}}}}},
		{Kind: Table, Rows: []Row{{Cells: []Cell{{Blocks: []Block{{Kind: Paragraph, Anchors: []string{"cell"}, Runs: []Run{{Anchor: "cell-run"}, {Text: "Cell"}}}}}}}}},
		{Kind: Paragraph, Runs: []Run{{Text: "paragraph ", Destination: "paragraph"}, {Text: "before ", Destination: "before"}, {Text: "middle ", Destination: "middle"}, {Text: "end ", Destination: "end"}, {Text: "heading ", Destination: "heading"}, {Text: "empty ", Destination: "empty"}, {Text: "list ", Destination: "list"}, {Text: "cell ", Destination: "cell"}, {Text: "cell-run ", Destination: "cell-run"}, {Text: "missing", Destination: "missing"}}},
		{Kind: Paragraph, Runs: []Run{{Text: "Note "}, {Note: &NoteReference{Kind: Footnote, ID: "n"}}}},
	}, Headers: []Block{{Kind: Paragraph, Runs: []Run{{Text: "Header jump", Destination: "middle"}}}}, Footers: []Block{{Kind: Paragraph, Runs: []Run{{Text: "Footer jump", Destination: "heading"}}}}, Notes: []Note{{Kind: Footnote, ID: "n", Blocks: []Block{{Kind: Paragraph, Anchors: []string{"note-point"}, Runs: []Run{{Text: "Note body", Destination: "heading"}}}}}}, UnresolvedDestinations: []string{"missing", "unused"}}
}
func TestColleafNavigationPointsSFAndActualSave(t *testing.T) {
	ctx := context.Background()
	want := colleafNavigationExample(t)
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
		t.Fatalf("point semantics differ\ngot%s\nwant%s", a, b)
	}
	sf, err := ToStructured(got)
	if err != nil {
		t.Fatal(err)
	}
	model, err := FromStructured(sf)
	if err != nil {
		t.Fatal(err)
	}
	again, err := encodeColleafDocument(ctx, model)
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := decodeColleafDocument(ctx, again)
	if err != nil {
		t.Fatal(err)
	}
	recoveredSF, err := ToStructured(recovered)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(sf, recoveredSF) {
		t.Fatal("SF normalization/round trip changed point graph, formatting or structure")
	}
	if file := os.Getenv("MANYDOC_COLLEAF_NAVIGATION_QA"); file != "" {
		raw, err := json.Marshal(wire)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(file, raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if file := os.Getenv("MANYDOC_COLLEAF_NAVIGATION_HEADING"); file != "" {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var native colleafNoteSnapshot
		if err = json.Unmarshal(raw, &native); err != nil {
			t.Fatal(err)
		}
		foreign, err := decodeColleafDocument(ctx, native)
		if err != nil {
			t.Fatal(err)
		}
		id := colleafHeadingID("", "foreign-heading")
		if len(foreign.Blocks[1].Anchors) != 2 || foreign.Blocks[1].Anchors[0] != id || foreign.Blocks[5].Runs[4].Destination != id || foreign.Footers[0].Runs[0].Destination != id {
			t.Fatal("native heading target relationship lost")
		}
		found := false
		for _, diag := range foreign.Diagnostics {
			found = found || diag.Code == "colleaf_heading_navigation"
		}
		if !found {
			t.Fatal("heading namespace normalization not disclosed")
		}
	}
	for _, spec := range []struct {
		env    string
		edited bool
	}{{"MANYDOC_COLLEAF_NAVIGATION_SAVED", false}, {"MANYDOC_COLLEAF_NAVIGATION_EDITED", true}} {
		file := os.Getenv(spec.env)
		if file == "" {
			continue
		}
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var native colleafNoteSnapshot
		if err = json.Unmarshal(raw, &native); err != nil {
			t.Fatal(err)
		}
		before, _ := json.Marshal(native)
		got, err = decodeColleafDocument(ctx, native)
		if err != nil {
			t.Fatal(spec.env, err)
		}
		after, _ := json.Marshal(native)
		if !bytes.Equal(before, after) {
			t.Fatal("navigation import mutated native snapshot")
		}
		envelope, err := json.Marshal(colleafEnvelope{colleafEnvelopeType, 3, "univer", colleafEngineVersion, native})
		if err != nil {
			t.Fatal(err)
		}
		strict, err := readColleaf(ctx, bytes.NewReader(envelope))
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(strict, got) {
			t.Fatal("strict byte codec changed navigation")
		}
		if !spec.edited {
			if !reflect.DeepEqual(got, want) {
				a, _ := json.Marshal(got)
				b, _ := json.Marshal(want)
				t.Fatalf("actual save changed navigation\ngot%s\nwant%s", a, b)
			}
		} else {
			if got.Blocks[0].Runs[0].Text != "Q" || len(got.Blocks[0].Anchors) != 0 {
				t.Fatal("current native point edit was hidden by old block ownership", got.Blocks[0])
			}
			positions := navigationParagraphPositions(got.Blocks[0])
			if positions["paragraph"] != 1 || positions["before"] != 1 || positions["middle"] != 4 || positions["end"] != 8 {
				t.Fatal("edited UTF16 point offsets differ", positions)
			}
			found := false
			for _, diag := range got.Diagnostics {
				found = found || diag.Code == "colleaf_stale_bookmark_placement"
			}
			if !found {
				t.Fatal("stale block placement not diagnosed")
			}
		}
	}
}
func navigationParagraphPositions(b Block) map[string]int {
	positions := map[string]int{}
	for _, id := range b.Anchors {
		positions[id] = 0
	}
	offset := 0
	for _, r := range b.Runs {
		if r.Anchor != "" {
			positions[r.Anchor] = offset
		}
		offset += colleafUTF16Len(r.Text)
		if r.Note != nil {
			offset++
		}
	}
	return positions
}
func TestColleafForeignHeadingBookmarkAliasAndMissingTargets(t *testing.T) {
	ctx := context.Background()
	base := &Document{Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "Jump"}}}, {Kind: Heading, Level: 1, Runs: []Run{{Text: "😀 heading"}}}}}
	wire, err := encodeColleafDocument(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	props, _ := colleafStyleObject(wire.Body.Paragraphs[1].Style)
	props["headingId"] = sfAttrs("foreign-heading")
	wire.Body.Paragraphs[1].Style = sfAttrs(props)
	wire.Body.CustomRanges = append(wire.Body.CustomRanges, colleafRangeRecord{ID: "heading-link", Type: 0, StartIndex: 0, EndIndex: 3, Properties: sfAttrs(map[string]string{"headingId": "foreign-heading", "url": "#foreign-heading", "textStyleMode": "text"})})
	got, err := decodeColleafDocument(ctx, wire)
	if err != nil {
		t.Fatal(err)
	}
	id := colleafHeadingID("", "foreign-heading")
	if got.Blocks[0].Runs[0].Destination != id || !reflect.DeepEqual(got.Blocks[1].Anchors, []string{id}) {
		t.Fatal("heading target did not resolve to its explicit point")
	}
	// A native range ID is also a valid bookmark alias, even when its primary
	// bookmarkId differs. Normalize the relationship, not the visible text.
	wire, err = encodeColleafDocument(ctx, colleafNavigationExample(t))
	if err != nil {
		t.Fatal(err)
	}
	var rangeID string
	for _, r := range wire.Body.CustomRanges {
		if r.Type == 3 {
			p, _ := readColleafBookmark(r)
			if p.BookmarkID == "before" {
				rangeID = r.ID
			}
		}
	}
	for i, r := range wire.Body.CustomRanges {
		if r.Type == 0 {
			wire.Body.CustomRanges[i].Properties = sfAttrs(map[string]string{"bookmarkId": rangeID, "url": "#" + rangeID, "textStyleMode": "text"})
			break
		}
	}
	got, err = decodeColleafDocument(ctx, wire)
	if err != nil {
		t.Fatal(err)
	}
	if got.Blocks[5].Runs[0].Destination != "before" {
		t.Fatal("native bookmark range alias lost")
	}
	for i, r := range wire.Body.CustomRanges {
		if r.Type == 0 {
			wire.Body.CustomRanges[i].Properties = sfAttrs(map[string]string{"bookmarkId": "new-missing", "textStyleMode": "text"})
			break
		}
	}
	got, err = decodeColleafDocument(ctx, wire)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, id := range got.UnresolvedDestinations {
		found = found || id == "new-missing"
	}
	if !found {
		t.Fatal("missing native target silently discarded")
	}
}
func TestColleafNavigationMalformedAndStructuralPoints(t *testing.T) {
	ctx := context.Background()
	wire, err := encodeColleafDocument(ctx, colleafNavigationExample(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, offset := range []int{-1, 1, colleafUTF16Len(wire.Body.DataStream)} {
		bad := wire
		bad.Body.CustomRanges = append([]colleafRangeRecord(nil), wire.Body.CustomRanges...)
		bad.Body.CustomRanges = append(bad.Body.CustomRanges, colleafRangeRecord{ID: "bad-point", Type: 3, StartIndex: offset, EndIndex: offset, Properties: sfAttrs(map[string]string{"bookmarkId": "bad"})})
		if _, err = decodeColleafDocument(ctx, bad); err == nil {
			t.Fatal("invalid/surrogate-split bookmark accepted", offset)
		}
	}
	bad := wire
	bad.Body.CustomRanges = append([]colleafRangeRecord(nil), wire.Body.CustomRanges...)
	bad.Body.CustomRanges = append(bad.Body.CustomRanges, colleafRangeRecord{ID: "structural", Type: 3, StartIndex: wire.Body.Tables[0].StartIndex, EndIndex: wire.Body.Tables[0].StartIndex, Properties: sfAttrs(map[string]string{"bookmarkId": "structural"})})
	recovered, err := decodeColleafDocument(ctx, bad)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(recovered.Blocks[4].Anchors, []string{"structural"}) {
		t.Fatal("native table-start point lost")
	}
	for _, raw := range []string{`{"BookmarkID":"x"}`, `{"bookmarkId":"x","manydocNavigation":{"Version":1,"role":"run","paragraphId":"p"}}`} {
		if _, err := readColleafBookmark(colleafRangeRecord{ID: "a", Properties: json.RawMessage(raw)}); err == nil {
			t.Fatal("noncanonical native point field accepted")
		}
	}
	for _, hint := range []string{`{"version":2,"role":"run","paragraphId":"p"}`, `{"version":1,"role":"unknown","paragraphId":"p"}`, `{"version":1,"role":"run","paragraphId":"p","hidden":true}`} {
		_, err := readColleafBookmark(colleafRangeRecord{ID: "a", Properties: json.RawMessage(`{"bookmarkId":"a","manydocNavigation":` + hint + `}`)})
		if err == nil {
			t.Fatal("malformed point hint accepted")
		}
	}
}
func FuzzColleafNavigation(f *testing.F) {
	wire, err := encodeColleafDocument(context.Background(), colleafNavigationExample(f))
	if err != nil {
		f.Fatal(err)
	}
	raw, _ := json.Marshal(wire)
	f.Add(raw)
	f.Fuzz(func(t *testing.T, raw []byte) {
		var input colleafNoteSnapshot
		if json.Unmarshal(raw, &input) != nil {
			return
		}
		d, err := decodeColleafDocument(context.Background(), input)
		if err != nil {
			return
		}
		if err = d.Validate(); err != nil {
			t.Fatal(err)
		}
		if _, err = ToStructured(d); err != nil {
			t.Fatal(err)
		}
	})
}

func TestColleafNavigationGeneratedAliasesAndNonmainData(t *testing.T) {
	ctx := context.Background()
	want := colleafNavigationExample(t)
	wire, err := encodeColleafDocument(ctx, want)
	if err != nil {
		t.Fatal(err)
	}
	var accidental string
	for _, a := range wire.Body.CustomRanges {
		if a.Type == 3 {
			p, _ := readColleafBookmark(a)
			if p.BookmarkID == "before" {
				accidental = a.ID
			}
		}
	}
	want.UnresolvedDestinations = append(want.UnresolvedDestinations, accidental)
	want.Blocks = append(want.Blocks, Block{Kind: Paragraph, Runs: []Run{{Text: "must remain missing", Destination: accidental}}})
	reserved, err := encodeColleafDocument(ctx, want)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range reserved.Body.CustomRanges {
		if a.Type == 3 && a.ID == accidental {
			t.Fatal("transport ID accidentally resolves a declared missing target")
		}
	}
	if _, err = decodeColleafDocument(ctx, reserved); err != nil {
		t.Fatal(err)
	}
	want = colleafNavigationExample(t)
	want.Blocks = append(want.Blocks, Block{Kind: Paragraph, Runs: []Run{{Text: "nonmain target", Destination: "note-point"}}})
	wire, err = encodeColleafDocument(ctx, want)
	if err != nil {
		t.Fatal(err)
	}
	got, err := decodeColleafDocument(ctx, wire)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, diag := range got.Diagnostics {
		found = found || diag.Code == "colleaf_nonmain_navigation"
	}
	if !found {
		t.Fatal("SDK main-story target limitation was not disclosed")
	}
}

func TestColleafNavigationDenseParagraphGraph(t *testing.T) {
	d := &Document{}
	for i := 0; i < 1024; i++ {
		d.Blocks = append(d.Blocks, Block{Kind: Paragraph, Anchors: []string{fmt.Sprintf("point-%d", i)}, Runs: []Run{{Text: "😀x"}}})
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
		t.Fatal("dense point graph changed structure/content/identities")
	}
}
func TestColleafNativeBookmarkSpanLossIsExplicit(t *testing.T) {
	wire, err := encodeColleafDocument(context.Background(), colleafNavigationExample(t))
	if err != nil {
		t.Fatal(err)
	}
	for i, a := range wire.Body.CustomRanges {
		if a.Type == 3 {
			wire.Body.CustomRanges[i].EndIndex = 2
			break
		}
	}
	d, err := decodeColleafDocument(context.Background(), wire)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, diag := range d.Diagnostics {
		found = found || diag.Code == "colleaf_bookmark_span"
	}
	if !found {
		t.Fatal("native bookmark editing extent was silently lost")
	}
	if navigationParagraphPositions(d.Blocks[0])["paragraph"] != 0 {
		t.Fatal("bookmark start moved")
	}
}

func FuzzColleafNavigationPoint(f *testing.F) {
	ctx := context.Background()
	base, err := encodeColleafDocument(ctx, &Document{Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "😀A"}}}}})
	if err != nil {
		f.Fatal(err)
	}
	f.Add([]byte(`{"bookmarkId":"a"}`), 0, 0)
	f.Add([]byte(`{"bookmarkId":"a","manydocNavigation":{"version":1,"role":"block","paragraphId":"p1"}}`), 0, 2)
	f.Fuzz(func(t *testing.T, props []byte, start, end int) {
		input := base
		input.Body.CustomRanges = append([]colleafRangeRecord(nil), base.Body.CustomRanges...)
		input.Body.CustomRanges = append(input.Body.CustomRanges, colleafRangeRecord{ID: "point", Type: 3, StartIndex: start, EndIndex: end, Properties: props})
		d, err := decodeColleafDocument(ctx, input)
		if err != nil {
			return
		}
		if err = d.Validate(); err != nil {
			t.Fatal(err)
		}
		if _, err = ToStructured(d); err != nil {
			t.Fatal(err)
		}
	})
}

func TestColleafStaleBookmarkStillReportsUnknownMetadata(t *testing.T) {
	wire, err := encodeColleafDocument(context.Background(), colleafNavigationExample(t))
	if err != nil {
		t.Fatal(err)
	}
	for i, a := range wire.Body.CustomRanges {
		if a.Type == 3 {
			p, _ := readColleafBookmark(a)
			if p.BookmarkID == "paragraph" {
				props, _ := colleafStyleObject(a.Properties)
				props["title"] = sfAttrs("unsupported bookmark metadata")
				wire.Body.CustomRanges[i].Properties = sfAttrs(props)
				wire.Body.CustomRanges[i].StartIndex = 3
				wire.Body.CustomRanges[i].EndIndex = 3
				break
			}
		}
	}
	d, err := decodeColleafDocument(context.Background(), wire)
	if err != nil {
		t.Fatal(err)
	}
	codes := map[string]bool{}
	for _, diag := range d.Diagnostics {
		codes[diag.Code] = true
	}
	if !codes["colleaf_bookmark_metadata"] || !codes["colleaf_stale_bookmark_placement"] {
		t.Fatal("stale placement hid metadata loss", d.Diagnostics)
	}
}
