package manydoc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
)

type nativeCursorCase struct {
	id             string
	offset         int
	kind, boundary string
}

func colleafStructuralCases() []nativeCursorCase {
	return []nativeCursorCase{
		{"row-start", 9, "row start", "logical table cell origin row 0 column 0 start"},
		{"origin-start", 10, "cell start", "logical table cell origin row 0 column 0 start"},
		{"origin-lf", 18, "section terminator", "end of current section flow"},
		{"origin-end", 19, "cell end", "logical table cell origin row 0 column 0 end"},
		{"b-start", 20, "cell start", "logical table cell origin row 0 column 1 start"},
		{"b-lf", 23, "section terminator", "end of current section flow"},
		{"b-end", 24, "cell end", "logical table cell origin row 0 column 1 end"},
		{"row-end", 25, "row end", "logical table cell origin row 0 column 1 end"},
		{"row-two-start", 26, "row start", "merged continuation's logical table cell origin row 0 column 0 start"},
		{"covered-start", 27, "cell start", "merged continuation's logical table cell origin row 0 column 0 start"},
		{"covered-cr", 28, "paragraph point", "merged continuation's logical table cell origin row 0 column 0 start"},
		{"covered-lf", 29, "section terminator", "merged continuation's logical table cell origin row 0 column 0 end"},
		{"covered-end", 30, "cell end", "merged continuation's logical table cell origin row 0 column 0 end"},
		{"c-start", 31, "cell start", "logical table cell origin row 1 column 1 start"},
		{"c-lf", 34, "section terminator", "end of current section flow"},
		{"c-end", 35, "cell end", "logical table cell origin row 1 column 1 end"},
		{"row-two-end", 36, "row end", "logical table cell origin row 1 column 1 end"},
		{"table-end", 37, "table terminator", "boundary after table"},
		{"table-cr", 38, "table anchoring paragraph terminator", "boundary after table"},
		{"image-cr", 40, "image paragraph terminator", "boundary after image paragraph"},
		{"main-lf", 63, "section terminator", "end of current section flow"},
	}
}
func colleafForeignCursorExample(t testing.TB) (colleafNoteSnapshot, *Document) {
	t.Helper()
	p := func(s string) Block { return Block{Kind: Paragraph, Runs: []Run{{Text: s}}} }
	table := Block{Kind: Table, ColumnWidths: []float64{72, 72}, Rows: []Row{{Cells: []Cell{{RowSpan: 2, Blocks: []Block{p("Origin")}}, {Blocks: []Block{p("B")}}}}, {Cells: []Cell{{Blocks: []Block{p("C")}}}}}}
	cases := colleafStructuralCases()
	var links []Run
	var missing []string
	for _, c := range cases {
		links = append(links, Run{Text: "x", Destination: c.id})
		missing = append(missing, c.id)
	}
	d := &Document{Blocks: []Block{p("Lead 😀"), table, {Kind: Picture, Image: &Image{Data: runningPNG(t, false), MediaType: "image/png", Width: 12, Height: 9}}, {Kind: Paragraph, Runs: links}}, UnresolvedDestinations: missing}
	wire, err := encodeColleafDocument(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	if wire.Body.DataStream[:len("Lead 😀\r")] != "Lead 😀\r" {
		t.Fatal("unexpected leading stream")
	}
	if colleafUTF16Len(wire.Body.DataStream) != 64 {
		t.Fatalf("independent cursor fixture length changed: %d", colleafUTF16Len(wire.Body.DataStream))
	}
	for _, c := range cases {
		wire.Body.CustomRanges = append(wire.Body.CustomRanges, colleafRangeRecord{ID: "native-" + c.id, Type: 3, StartIndex: c.offset, EndIndex: c.offset, Properties: sfAttrs(map[string]string{"bookmarkId": c.id})})
	}
	// The expected projection is authored independently of the native cursor walker.
	point := func(id string) Block { return Block{Kind: AnchorPoint, Anchors: []string{id}} }
	origin := []Block{}
	for _, id := range []string{"row-start", "origin-start", "row-two-start", "covered-start", "covered-cr"} {
		origin = append(origin, point(id))
	}
	origin = append(origin, p("Origin"))
	for _, id := range []string{"origin-lf", "origin-end", "covered-lf", "covered-end"} {
		origin = append(origin, point(id))
	}
	table.Rows[0].Cells[0].Blocks = origin
	table.Rows[0].Cells[1].Blocks = []Block{point("b-start"), p("B"), point("b-lf"), point("b-end"), point("row-end")}
	table.Rows[1].Cells[0].Blocks = []Block{point("c-start"), p("C"), point("c-lf"), point("c-end"), point("row-two-end")}
	expected := &Document{Blocks: []Block{p("Lead 😀"), table, point("table-end"), point("table-cr"), d.Blocks[2], point("image-cr"), d.Blocks[3], point("main-lf")}}
	for _, c := range cases {
		expected.Diagnostics = append(expected.Diagnostics, Diagnostic{Code: "colleaf_structural_cursor", Message: fmt.Sprintf("Native bookmark at UTF-16 offset %d (%s) projects to %s; exact native control cursor identity is not retained", c.offset, c.kind, c.boundary)})
	}
	return wire, expected
}
func TestColleafForeignStructuralCursorProjection(t *testing.T) {
	ctx := context.Background()
	wire, expected := colleafForeignCursorExample(t)
	before, _ := json.Marshal(wire)
	got, err := decodeColleafDocument(ctx, wire)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(wire)
	if !bytes.Equal(before, after) {
		t.Fatal("source snapshot mutated")
	}
	// Diagnostics follow interpretation order; compare their exact set independently.
	wantDiagnostics := map[string]bool{}
	for _, d := range expected.Diagnostics {
		wantDiagnostics[d.Message] = true
	}
	if len(got.Diagnostics) != len(wantDiagnostics) {
		t.Fatalf("wrong diagnostic count: %+v", got.Diagnostics)
	}
	for _, d := range got.Diagnostics {
		if d.Code != "colleaf_structural_cursor" || !wantDiagnostics[d.Message] {
			t.Fatal("unexpected or missing loss evidence", d)
		}
	}
	expected.Diagnostics = got.Diagnostics
	if !reflect.DeepEqual(expected, got) {
		a, _ := json.Marshal(got)
		b, _ := json.Marshal(expected)
		t.Fatalf("logical target/content/grid/asset graph changed\ngot%s\nwant%s", a, b)
	}
	sf, err := ToStructured(got)
	if err != nil {
		t.Fatal(err)
	}
	model, err := FromStructured(sf)
	if err != nil {
		t.Fatal(err)
	}
	sfAgain, err := ToStructured(model)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(sf, sfAgain) {
		t.Fatal("SF lost controls projection or diagnostics")
	}
	// This new document represents the disclosed normalized semantics, including
	// its original diagnostic history through checked native intent.
	normalizedInput, err := FromStructured(sf)
	if err != nil {
		t.Fatal(err)
	}
	normalized, err := encodeColleafDocument(ctx, normalizedInput)
	if err != nil {
		t.Fatal(err)
	}
	normalizedModel, err := decodeColleafDocument(ctx, normalized)
	if err != nil {
		t.Fatal(err)
	}
	normalizedSF, err := ToStructured(normalizedModel)
	if err != nil {
		t.Fatal(err)
	}
	expectedNormalizedSF, err := ToStructured(normalizedInput)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(expectedNormalizedSF, normalizedSF) {
		t.Fatal("normalized native fixed point changed data/relationships")
	}
	if file := os.Getenv("MANYDOC_COLLEAF_CURSOR_NORMALIZED_QA"); file != "" {
		raw, err := json.Marshal(normalized)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(file, raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if file := os.Getenv("MANYDOC_COLLEAF_CURSOR_NORMALIZED_SAVED"); file != "" {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var saved colleafNoteSnapshot
		if err = json.Unmarshal(raw, &saved); err != nil {
			t.Fatal(err)
		}
		envelope, err := json.Marshal(colleafEnvelope{colleafEnvelopeType, 3, "univer", colleafEngineVersion, saved})
		if err != nil {
			t.Fatal(err)
		}
		actual, err := readColleaf(ctx, bytes.NewReader(envelope))
		if err != nil {
			t.Fatal(err)
		}
		actualSF, err := ToStructured(actual)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(actualSF, expectedNormalizedSF) {
			t.Fatal("actual normalized save changed data/relationships")
		}
	}
	if file := os.Getenv("MANYDOC_COLLEAF_CURSOR_QA"); file != "" {
		if err = os.WriteFile(file, before, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if file := os.Getenv("MANYDOC_COLLEAF_CURSOR_SAVED"); file != "" {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var saved colleafNoteSnapshot
		if err = json.Unmarshal(raw, &saved); err != nil {
			t.Fatal(err)
		}
		envelope, err := json.Marshal(colleafEnvelope{colleafEnvelopeType, 3, "univer", colleafEngineVersion, saved})
		if err != nil {
			t.Fatal(err)
		}
		actual, err := readColleaf(ctx, bytes.NewReader(envelope))
		if err != nil {
			t.Fatal(err)
		}
		actualSF, err := ToStructured(actual)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(sf, actualSF) {
			t.Fatal("actual native save changed projected SF graph")
		}
	}

	if file := os.Getenv("MANYDOC_COLLEAF_CURSOR_EDITED"); file != "" {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var edited colleafNoteSnapshot
		if err = json.Unmarshal(raw, &edited); err != nil {
			t.Fatal(err)
		}
		envelope, err := json.Marshal(colleafEnvelope{colleafEnvelopeType, 3, "univer", colleafEngineVersion, edited})
		if err != nil {
			t.Fatal(err)
		}
		actual, err := readColleaf(ctx, bytes.NewReader(envelope))
		if err != nil {
			t.Fatal(err)
		}
		want, err := FromStructured(sf)
		if err != nil {
			t.Fatal(err)
		}
		want.Blocks[0].Runs = append([]Run{{Text: "Q"}}, want.Blocks[0].Runs...)
		for i, d := range want.Diagnostics {
			for _, c := range colleafStructuralCases() {
				prefix := fmt.Sprintf("Native bookmark at UTF-16 offset %d (", c.offset)
				if strings.HasPrefix(d.Message, prefix) {
					want.Diagnostics[i].Message = strings.Replace(d.Message, prefix, fmt.Sprintf("Native bookmark at UTF-16 offset %d (", c.offset+1), 1)
					break
				}
			}
		}
		expectedSF, err := ToStructured(want)
		if err != nil {
			t.Fatal(err)
		}
		actualSF, err := ToStructured(actual)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(expectedSF, actualSF) {
			a, _ := json.Marshal(actualSF)
			b, _ := json.Marshal(expectedSF)
			t.Fatalf("actual edited cursor recovery changed\ngot%s\nwant%s", a, b)
		}
	}
}
func TestColleafForeignStructuralCursorListOwnership(t *testing.T) {
	ctx := context.Background()
	base := colleafContainerNavigationExample(t)
	wire, err := encodeColleafDocument(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	// The last main table is a list continuation. Its new terminal point must
	// remain in that item without invalidating paragraph-ID ownership metadata.
	table := wire.Body.Tables[len(wire.Body.Tables)-1]
	wire.Body.CustomRanges = append(wire.Body.CustomRanges, colleafRangeRecord{ID: "end", Type: 3, StartIndex: table.EndIndex - 1, EndIndex: table.EndIndex - 1})
	got, err := decodeColleafDocument(ctx, wire)
	if err != nil {
		t.Fatal(err)
	}
	item := got.Blocks[3].Items[0].Blocks
	if len(item) != 4 || item[2].Kind != AnchorPoint || !reflect.DeepEqual(item[2].Anchors, []string{"end"}) || item[3].Kind != Picture {
		t.Fatal("table terminal point broke actual item ownership")
	}
	for _, d := range got.Diagnostics {
		if strings.Contains(d.Code, "list_ownership") {
			t.Fatal("new point invalidated valid list ownership", d)
		}
	}
}

func TestColleafForeignStructuralHorizontalContinuations(t *testing.T) {
	body, tables := nativeTableFixture([][]string{{"A", "", ""}}, [][][2]int{{{1, 3}, {0, 0}, {0, 0}}})
	for i, position := range []int{7, 9, 14} {
		body.CustomRanges = append(body.CustomRanges, colleafRangeRecord{ID: fmt.Sprintf("horizontal-%d", i), Type: 3, StartIndex: position, EndIndex: position})
	}
	d, err := decodeColleafBody(context.Background(), body, colleafTextContext{}, tables)
	if err != nil {
		t.Fatal(err)
	}
	cell := d.Blocks[0].Rows[0].Cells[0]
	if cell.ColSpan != 3 || len(d.Blocks[0].Rows[0].Cells) != 1 || len(cell.Blocks) != 4 || cell.Blocks[0].Anchors[0] != "horizontal-0" || cell.Blocks[1].Runs[0].Text != "A" || cell.Blocks[2].Anchors[0] != "horizontal-1" || cell.Blocks[3].Anchors[0] != "horizontal-2" {
		t.Fatal("horizontal continuation origin/targets changed")
	}
	if len(d.Diagnostics) != 3 {
		t.Fatal("horizontal position normalization not disclosed", d.Diagnostics)
	}
	if err = d.Validate(); err != nil {
		t.Fatal(err)
	}
}
func TestColleafForeignStructuralRunningAndNoteStories(t *testing.T) {
	p := func(s string) Block { return Block{Kind: Paragraph, Runs: []Run{{Text: s}}} }
	table := func(s string) Block { return Block{Kind: Table, Rows: []Row{{Cells: []Cell{{Blocks: []Block{p(s)}}}}}} }
	wire, err := encodeColleafDocument(context.Background(), &Document{Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "Body"}, {Note: &NoteReference{Kind: Footnote, ID: "n"}}}}}, Headers: []Block{table("Header")}, Footers: []Block{table("Footer")}, Notes: []Note{{Kind: Footnote, ID: "n", Blocks: []Block{table("Note")}}}})
	if err != nil {
		t.Fatal(err)
	}
	for id, story := range wire.Headers {
		story.Body.CustomRanges = append(story.Body.CustomRanges, colleafRangeRecord{ID: "header-row", Type: 3, StartIndex: 1, EndIndex: 1})
		wire.Headers[id] = story
	}
	for id, story := range wire.Footers {
		story.Body.CustomRanges = append(story.Body.CustomRanges, colleafRangeRecord{ID: "footer-row", Type: 3, StartIndex: 1, EndIndex: 1})
		wire.Footers[id] = story
	}
	for id, note := range wire.Notes {
		note.Body.CustomRanges = append(note.Body.CustomRanges, colleafRangeRecord{ID: "note-lf", Type: 3, StartIndex: 8, EndIndex: 8})
		wire.Notes[id] = note
	}
	got, err := decodeColleafDocument(context.Background(), wire)
	if err != nil {
		t.Fatal(err)
	}
	if got.Headers[0].Rows[0].Cells[0].Blocks[0].Anchors[0] != "header-row" || got.Footers[0].Rows[0].Cells[0].Blocks[0].Anchors[0] != "footer-row" || got.Notes[0].Blocks[0].Rows[0].Cells[0].Blocks[1].Anchors[0] != "note-lf" {
		t.Fatal("story ownership changed")
	}
	if len(got.Diagnostics) != 3 {
		t.Fatal("story cursor losses not explicit", got.Diagnostics)
	}
}
func FuzzColleafForeignStructuralCursor(f *testing.F) {
	ctx := context.Background()
	base, err := encodeColleafDocument(ctx, &Document{Blocks: []Block{{Kind: Table, Rows: []Row{{Cells: []Cell{{RowSpan: 2, Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "😀"}}}}}, {Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "B"}}}}}}}, {Cells: []Cell{{Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "C"}}}}}}}}}}})
	if err != nil {
		f.Fatal(err)
	}
	for _, position := range []int{1, 2, 6, 9, 12, 17, 21, 26} {
		f.Add(position, []byte(`{"bookmarkId":"point"}`))
	}
	f.Fuzz(func(t *testing.T, position int, props []byte) {
		wire := base
		wire.Body.CustomRanges = append([]colleafRangeRecord(nil), base.Body.CustomRanges...)
		wire.Body.CustomRanges = append(wire.Body.CustomRanges, colleafRangeRecord{ID: "fuzz-point", Type: 3, StartIndex: position, EndIndex: position, Properties: props})
		d, err := decodeColleafDocument(ctx, wire)
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

func TestColleafForeignStructuralEveryScalarBoundary(t *testing.T) {
	ctx := context.Background()
	base, _ := colleafForeignCursorExample(t)
	index, err := newColleafTextIndex(ctx, base.Body.DataStream)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for position := 0; position < index.length(); position++ {
		if _, err = index.byteBoundary(position); err != nil {
			continue
		}
		count++
		wire := base
		wire.Body.CustomRanges = append([]colleafRangeRecord(nil), base.Body.CustomRanges...)
		wire.Body.CustomRanges = append(wire.Body.CustomRanges, colleafRangeRecord{ID: "probe", Type: 3, StartIndex: position, EndIndex: position})
		d, err := decodeColleafDocument(ctx, wire)
		if err != nil {
			t.Fatalf("valid scalar cursor %d: %v", position, err)
		}
		found := 0
		if err = documentDestinationRuns(d, func(r Run) error {
			if r.Anchor == "probe" {
				found++
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		if found != 1 {
			t.Fatalf("cursor %d has %d projected identities", position, found)
		}
	}
	if count != 63 {
		t.Fatalf("reference scalar-boundary count changed: %d", count)
	}
}
func TestColleafForeignStructuralPageBreakParagraph(t *testing.T) {
	ctx := context.Background()
	wire, err := encodeColleafDocument(ctx, &Document{Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "Before"}}}, {Kind: PageBreak}, {Kind: Paragraph, Runs: []Run{{Text: "After"}}}}})
	if err != nil {
		t.Fatal(err)
	}
	wire.Body.CustomRanges = append(wire.Body.CustomRanges, colleafRangeRecord{ID: "break-end", Type: 3, StartIndex: 8, EndIndex: 8})
	got, err := decodeColleafDocument(ctx, wire)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Blocks) != 4 || got.Blocks[1].Kind != PageBreak || got.Blocks[2].Kind != AnchorPoint || got.Blocks[2].Anchors[0] != "break-end" {
		t.Fatal("hard break terminal cursor changed boundary")
	}
	if len(got.Diagnostics) != 1 || !strings.Contains(got.Diagnostics[0].Message, "page-break paragraph terminator") {
		t.Fatal("break cursor normalization not disclosed")
	}
}
