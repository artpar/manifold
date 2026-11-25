package engine

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
)

func vdocsContainerNavigationExample(t testing.TB) *Document {
	t.Helper()
	p := func(s string) Block { return Block{Kind: Paragraph, Runs: []Run{{Text: s}}} }
	image := func(id string) Block {
		return Block{Kind: Picture, Anchors: []string{id}, Image: &Image{Data: runningPNG(t, false), MediaType: "image/png", Alt: id, Width: 12, Height: 9}}
	}
	table := func(id string, bs ...Block) Block {
		return Block{Kind: Table, Anchors: []string{id, id + "-alias"}, ColumnWidths: []float64{80}, Rows: []Row{{Cells: []Cell{{Blocks: bs}}}}}
	}
	return &Document{Blocks: []Block{
		p("Lead 😀"),
		table("table", p("Cell"), image("cell-picture"), table("nested", p("Nested"))),
		image("picture"),
		{Kind: ListBlock, List: "bullet", Items: []ListItem{{Blocks: []Block{p("Item"), table("list-table", p("List table")), image("list-picture")}}}},
		{Kind: Paragraph, Runs: []Run{{Text: "Table", Destination: "table"}, {Text: "Alias", Destination: "table-alias"}, {Text: "Cell picture", Destination: "cell-picture"}, {Text: "Nested", Destination: "nested"}, {Text: "Picture", Destination: "picture"}, {Text: "List table", Destination: "list-table"}, {Text: "List picture", Destination: "list-picture"}, {Note: &NoteReference{Kind: Footnote, ID: "n"}}}},
	}, Headers: []Block{table("header-table", p("Header"), image("header-picture"))}, Footers: []Block{image("footer-picture")}, Notes: []Note{{Kind: Footnote, ID: "n", Blocks: []Block{table("note-table", p("Note"), image("note-picture"))}}}}
}
func TestVDOCSContainerNavigationRoundTrip(t *testing.T) {
	ctx := context.Background()
	want := vdocsContainerNavigationExample(t)
	wire, err := encodeVDOCSDocument(ctx, want)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(wire)
	got, err := decodeVDOCSDocument(ctx, wire)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(wire)
	if !bytes.Equal(before, after) {
		t.Fatal("source mutated")
	}
	if !reflect.DeepEqual(got, want) {
		a, _ := json.Marshal(got)
		b, _ := json.Marshal(want)
		t.Fatalf("container graph changed\ngot%s\nwant%s", a, b)
	}
	sf, err := ToStructured(got)
	if err != nil {
		t.Fatal(err)
	}
	model, err := FromStructured(sf)
	if err != nil {
		t.Fatal(err)
	}
	again, err := encodeVDOCSDocument(ctx, model)
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := decodeVDOCSDocument(ctx, again)
	if err != nil {
		t.Fatal(err)
	}
	sfAgain, err := ToStructured(recovered)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(sf, sfAgain) {
		t.Fatal("SF container structure/assets/relationships changed")
	}
	if file := os.Getenv("MANYDOC_VDOCS_CONTAINER_QA"); file != "" {
		if err = os.WriteFile(file, before, 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, spec := range []struct {
		env    string
		edited bool
	}{{"MANYDOC_VDOCS_CONTAINER_SAVED", false}, {"MANYDOC_VDOCS_CONTAINER_EDITED", true}} {
		file := os.Getenv(spec.env)
		if file == "" {
			continue
		}
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var native vdocsNoteSnapshot
		if err = json.Unmarshal(raw, &native); err != nil {
			t.Fatal(err)
		}
		envelope, err := json.Marshal(vdocsEnvelope{vdocsEnvelopeType, 3, "univer", vdocsEngineVersion, native})
		if err != nil {
			t.Fatal(err)
		}
		actual, err := readVDOCS(ctx, bytes.NewReader(envelope))
		if err != nil {
			t.Fatal(err)
		}
		expected, err := FromStructured(sf)
		if err != nil {
			t.Fatal(err)
		}
		if spec.edited {
			// TextX adds an independently styled interval before the existing run.
			expected.Blocks[0].Runs = append([]Run{{Text: "Q"}}, expected.Blocks[0].Runs...)
		}
		expectedSF, err := ToStructured(expected)
		if err != nil {
			t.Fatal(err)
		}
		actualSF, err := ToStructured(actual)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(actualSF, expectedSF) {
			a, _ := json.Marshal(actualSF)
			b, _ := json.Marshal(expectedSF)
			t.Fatalf("actual saved/edited graph changed %s\ngot%s\nwant%s", spec.env, a, b)
		}
	}
}
func TestVDOCSContainerNavigationCurrentNativePosition(t *testing.T) {
	ctx := context.Background()
	wire, err := encodeVDOCSDocument(ctx, vdocsContainerNavigationExample(t))
	if err != nil {
		t.Fatal(err)
	}
	// A native editor moved the table point into text. The current point wins.

	// The native controller resets its source map from this resource on load. A
	// valid standalone source descriptor alone therefore does not establish paint.

	// Contradictory mirrors still reject; registration cannot override content.

	// Inserting at the first section cursor moves its point away from its start.

	// Strict resource checks must see JSON inside compression, before typed resource
	// parsing could discard duplicate keys. This uses the public-envelope preflight.

	// Bound this fuzz target's allocations independently of production limits.

	// This publication exercises combined source bindings from an independent PDF
	// producer. Its structural oracle remains in TestPDFPublishedInDesignExport.

	for i, a := range wire.Body.CustomRanges {
		if a.Type != 3 {
			continue
		}
		p, err := readVDOCSBookmark(a)
		if err != nil {
			t.Fatal(err)
		}
		if p.BookmarkID == "table" {
			wire.Body.CustomRanges[i].StartIndex = 2
			wire.Body.CustomRanges[i].EndIndex = 2
			break
		}
	}
	got, err := decodeVDOCSDocument(ctx, wire)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Blocks[1].Anchors, []string{"table-alias"}) || len(got.Blocks[0].Runs) != 3 || got.Blocks[0].Runs[1].Anchor != "table" {
		t.Fatal("stale container metadata overrode current point")
	}
	found := false
	for _, d := range got.Diagnostics {
		found = found || d.Code == "vdocs_stale_bookmark_placement"
	}
	if !found {
		t.Fatal("stale placement not diagnosed")
	}
}
func TestVDOCSContainerNavigationMetadataAndBindings(t *testing.T) {
	for _, role := range []string{"table", "picture"} {
		for _, hint := range []map[string]any{{"version": 1, "role": role}, {"version": 1, "role": role, "ownerId": "o", "paragraphId": "p"}, {"version": 1, "role": role, "OwnerId": "o"}} {
			_, err := readVDOCSBookmark(vdocsRangeRecord{ID: "a", Properties: sfAttrs(map[string]any{"bookmarkId": "a", "manydocNavigation": hint})})
			if err == nil {
				t.Fatal("malformed owner accepted", hint)
			}
		}
	}
	ctx := context.Background()
	wire, err := encodeVDOCSDocument(ctx, vdocsContainerNavigationExample(t))
	if err != nil {
		t.Fatal(err)
	}
	for i, a := range wire.Body.CustomRanges {
		if a.Type != 3 {
			continue
		}
		p, err := readVDOCSBookmark(a)
		if err != nil {
			t.Fatal(err)
		}
		if p.BookmarkID == "table" {
			p.Hint.OwnerID = "changed"
			props := map[string]any{"bookmarkId": p.BookmarkID, "manydocNavigation": p.Hint, "foreign": "extra"}
			wire.Body.CustomRanges[i].Properties = sfAttrs(props)
			wire.Body.CustomRanges[i].EndIndex = a.StartIndex + 1
			break
		}
	}
	got, err := decodeVDOCSDocument(ctx, wire)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Blocks[1].Anchors, []string{"table", "table-alias"}) {
		t.Fatal("current container point lost")
	}
	for _, code := range []string{"vdocs_stale_bookmark_placement", "vdocs_bookmark_metadata", "vdocs_bookmark_span"} {
		found := false
		for _, d := range got.Diagnostics {
			found = found || d.Code == code
		}
		if !found {
			t.Fatal("loss not diagnosed", code)
		}
	}
}

func TestVDOCSPublicDrawingRegistration(t *testing.T) {
	ctx := context.Background()
	want := vdocsImageExample(t)
	want.Headers = []Block{want.Blocks[0]}
	var output bytes.Buffer
	if err := WriteVDOCS(ctx, &output, want); err != nil {
		t.Fatal(err)
	}
	var envelope vdocsEnvelope
	if err := json.Unmarshal(output.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	wire := envelope.Document
	var mirror struct {
		Data  map[string]json.RawMessage `json:"data"`
		Order []string                   `json:"order"`
	}
	found := 0
	for _, r := range wire.Resources {
		if r.Name == "DOC_DRAWING_PLUGIN" {
			found++
			if err := json.Unmarshal([]byte(r.Data), &mirror); err != nil {
				t.Fatal(err)
			}
		}
	}
	if found != 1 || len(mirror.Data) != 5 || len(mirror.Order) != 4 {
		t.Fatalf("missing root/header registration or incorrect body order: %d/%d/%d", found, len(mirror.Data), len(mirror.Order))
	}
	for id, n := range wire.Notes {
		for drawing := range n.Drawings {
			if _, ok := mirror.Data[drawing]; ok {
				t.Fatal("note source duplicated into root registry", id, drawing)
			}
		}
	}
	if err := checkVDOCSDrawingMirror(wire, vdocsResource{Name: "DOC_DRAWING_PLUGIN", Data: string(sfAttrs(mirror))}); err != nil {
		t.Fatal(err)
	}
	got, err := ReadVDOCS(ctx, bytes.NewReader(output.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatal("native registration changed document semantics", got.Diagnostics)
	}

	delete(mirror.Data, mirror.Order[0])
	wire.Resources = vdocsReplaceResource(wire.Resources, "DOC_DRAWING_PLUGIN", mirror)
	if _, err = decodeVDOCSDocument(ctx, wire); err == nil {
		t.Fatal("mirror order without its source accepted")
	}
}
func vdocsFlowNavigationExample() *Document {
	p := func(s string) Block { return Block{Kind: Paragraph, Runs: []Run{{Text: s}}} }
	list := func(id string) Block {
		return Block{Kind: ListBlock, List: "bullet", Anchors: []string{id}, Items: []ListItem{{Blocks: []Block{p("Item")}}}}
	}
	main := list("list")
	main.Anchors = append(main.Anchors, "list-alias")
	main.Items[0].Blocks[0].Anchors = []string{"item"}
	main.Items[0].Blocks = append(main.Items[0].Blocks, list("nested-list"), Block{Kind: PageBreak, Anchors: []string{"list-break"}}, p("Tail"))
	links := Block{Kind: Paragraph, Runs: []Run{{Text: "S1", Destination: "section-1"}, {Text: "List", Destination: "list"}, {Text: "Alias", Destination: "list-alias"}, {Text: "Item", Destination: "item"}, {Text: "Nested", Destination: "nested-list"}, {Text: "Break", Destination: "list-break"}, {Text: "S2", Destination: "section-2"}, {Text: "B2", Destination: "break-2"}, {Text: "Empty", Destination: "empty-section"}, {Note: &NoteReference{Kind: Footnote, ID: "n"}}}}
	return &Document{Blocks: []Block{
		{Kind: SectionBlock, Anchors: []string{"section-1"}, Section: &Section{Blocks: []Block{p("Lead 😀"), main, links}, Headers: RunningStories{Default: &RunningStory{Blocks: []Block{list("header-list")}}}}},
		{Kind: SectionBlock, Anchors: []string{"section-2"}, Section: &Section{Start: SectionNextPage, Blocks: []Block{{Kind: PageBreak, Anchors: []string{"break-2"}}, p("Second")}}},
		{Kind: SectionBlock, Anchors: []string{"empty-section"}, Section: &Section{Start: SectionContinuous}},
	}, Notes: []Note{{Kind: Footnote, ID: "n", Blocks: []Block{list("note-list")}}}}
}
func TestVDOCSFlowNavigationRoundTrip(t *testing.T) {
	ctx := context.Background()
	want := vdocsFlowNavigationExample()
	wire, err := encodeVDOCSDocument(ctx, want)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(wire)
	got, err := decodeVDOCSDocument(ctx, wire)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(wire)
	if !bytes.Equal(before, after) {
		t.Fatal("source mutated")
	}
	if !reflect.DeepEqual(got, want) {
		a, _ := json.Marshal(got)
		b, _ := json.Marshal(want)
		t.Fatalf("flow point semantics changed\ngot%s\nwant%s", a, b)
	}
	sf, err := ToStructured(got)
	if err != nil {
		t.Fatal(err)
	}
	model, err := FromStructured(sf)
	if err != nil {
		t.Fatal(err)
	}
	again, err := encodeVDOCSDocument(ctx, model)
	if err != nil {
		t.Fatal(err)
	}
	round, err := decodeVDOCSDocument(ctx, again)
	if err != nil {
		t.Fatal(err)
	}
	roundSF, err := ToStructured(round)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(sf, roundSF) {
		t.Fatal("SF graph changed")
	}
	if file := os.Getenv("MANYDOC_VDOCS_FLOW_QA"); file != "" {
		if err = os.WriteFile(file, before, 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, spec := range []struct {
		env    string
		edited bool
	}{{"MANYDOC_VDOCS_FLOW_SAVED", false}, {"MANYDOC_VDOCS_FLOW_EDITED", true}} {
		file := os.Getenv(spec.env)
		if file == "" {
			continue
		}
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var native vdocsNoteSnapshot
		if err = json.Unmarshal(raw, &native); err != nil {
			t.Fatal(err)
		}
		envelope, err := json.Marshal(vdocsEnvelope{vdocsEnvelopeType, 3, "univer", vdocsEngineVersion, native})
		if err != nil {
			t.Fatal(err)
		}
		actual, err := readVDOCS(ctx, bytes.NewReader(envelope))
		if err != nil {
			t.Fatal(err)
		}
		expected, err := FromStructured(sf)
		if err != nil {
			t.Fatal(err)
		}
		if spec.edited {
			expected.Blocks[0].Section.Blocks[0].Runs = append([]Run{{Text: "Q"}}, expected.Blocks[0].Section.Blocks[0].Runs...)
		}

		if spec.edited {
			expected.Blocks[0].Anchors = nil
			expected.Blocks[0].Section.Blocks[0].Runs = append([]Run{{Text: "Q"}, {Anchor: "section-1"}}, expected.Blocks[0].Section.Blocks[0].Runs[1:]...)
			expected.Diagnostics = []Diagnostic{{Code: "vdocs_stale_bookmark_placement", Message: "Edited native bookmark position is retained as a current inline point rather than old block placement"}}
		}
		expectedSF, err := ToStructured(expected)
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
			t.Fatalf("actual flow save/edit changed %s\ngot%s\nwant%s", spec.env, a, b)
		}
		if spec.edited {
			found := false
			for _, d := range actual.Diagnostics {
				found = found || d.Code == "vdocs_stale_bookmark_placement"
			}
			if !found {
				t.Fatal("moved section ownership not disclosed")
			}
		}
	}
}
func TestVDOCSFlowNavigationMovedListPoint(t *testing.T) {
	ctx := context.Background()
	wire, err := encodeVDOCSDocument(ctx, vdocsFlowNavigationExample())
	if err != nil {
		t.Fatal(err)
	}
	for i, a := range wire.Body.CustomRanges {
		if a.Type != 3 {
			continue
		}
		p, err := readVDOCSBookmark(a)
		if err != nil {
			t.Fatal(err)
		}
		if p.BookmarkID == "list" {
			wire.Body.CustomRanges[i].StartIndex++
			wire.Body.CustomRanges[i].EndIndex++
			break
		}
	}
	got, err := decodeVDOCSDocument(ctx, wire)
	if err != nil {
		t.Fatal(err)
	}
	list := got.Blocks[0].Section.Blocks[1]
	if !reflect.DeepEqual(list.Anchors, []string{"list-alias"}) || list.Items[0].Blocks[0].Runs[1].Anchor != "list" {
		t.Fatal("moved list point restored stale block ownership")
	}
	found := false
	for _, d := range got.Diagnostics {
		found = found || d.Code == "vdocs_stale_bookmark_placement"
	}
	if !found {
		t.Fatal("list ownership loss not disclosed")
	}
}

func TestVDOCSFlowNavigationSoftBreakKeepsPoint(t *testing.T) {
	ctx := context.Background()
	wire, err := encodeVDOCSDocument(ctx, vdocsFlowNavigationExample())
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range wire.Body.CustomRanges {
		if a.Type != 3 {
			continue
		}
		p, err := readVDOCSBookmark(a)
		if err != nil {
			t.Fatal(err)
		}
		if p.BookmarkID == "list-break" {
			wire.Body.RenderedPageBreaks = []int{a.StartIndex}
			break
		}
	}
	got, err := decodeVDOCSDocument(ctx, wire)
	if err != nil {
		t.Fatal(err)
	}
	bs := got.Blocks[0].Section.Blocks[1].Items[0].Blocks
	if len(bs) != 4 || bs[2].Kind != AnchorPoint || !reflect.DeepEqual(bs[2].Anchors, []string{"list-break"}) {
		t.Fatal("soft layout boundary swallowed point")
	}
	found := false
	for _, d := range got.Diagnostics {
		found = found || d.Code == "vdocs_soft_page_break"
	}
	if !found {
		t.Fatal("soft boundary loss not disclosed")
	}
}
func compressedIntentForTest(t *testing.T, raw string) vdocsIntentCarrier {
	t.Helper()
	var buf bytes.Buffer
	z := gzip.NewWriter(&buf)
	if _, err := z.Write([]byte(raw)); err != nil {
		t.Fatal(err)
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	return vdocsIntentCarrier{Profile: vdocsIntentProfile, Version: 2, GzipRecords: buf.Bytes(), RecordsBytes: len(raw)}
}
func TestVDOCSIntentCompressionAndValidation(t *testing.T) {
	ctx := context.Background()
	records := []vdocsResource{{Name: vdocsDocumentResource, Data: strings.Repeat("source 😀", 10000)}}
	carrier, err := makeVDOCSIntentCarrier(records)
	if err != nil {
		t.Fatal(err)
	}
	if carrier.Version != 2 || carrier.Records != nil {
		t.Fatal("large metadata not compressed")
	}
	got, err := carrier.records(ctx)
	if err != nil || !reflect.DeepEqual(records, got) {
		t.Fatal("metadata changed", err)
	}
	again, err := makeVDOCSIntentCarrier(records)
	if err != nil || !reflect.DeepEqual(carrier, again) {
		t.Fatal("nondeterministic compression", err)
	}
	for name, mutate := range map[string]func(*vdocsIntentCarrier){
		"checksum":    func(c *vdocsIntentCarrier) { c.GzipRecords[len(c.GzipRecords)-8] ^= 1 },
		"truncated":   func(c *vdocsIntentCarrier) { c.GzipRecords = c.GzipRecords[:len(c.GzipRecords)-1] },
		"trailing":    func(c *vdocsIntentCarrier) { c.GzipRecords = append(c.GzipRecords, 0) },
		"multistream": func(c *vdocsIntentCarrier) { c.GzipRecords = append(c.GzipRecords, c.GzipRecords...) },
		"short":       func(c *vdocsIntentCarrier) { c.RecordsBytes-- },
		"long":        func(c *vdocsIntentCarrier) { c.RecordsBytes++ },
		"bound":       func(c *vdocsIntentCarrier) { c.RecordsBytes = maxVDOCSIntentBytes + 1 },
		"mixed":       func(c *vdocsIntentCarrier) { c.Records = records },
		"profile":     func(c *vdocsIntentCarrier) { c.Profile = "other" },
		"version":     func(c *vdocsIntentCarrier) { c.Version = 3 },
	} {
		t.Run(name, func(t *testing.T) {
			bad := carrier
			bad.GzipRecords = bytes.Clone(carrier.GzipRecords)
			mutate(&bad)
			if _, err := bad.records(ctx); err == nil {
				t.Fatal("malformed compression accepted")
			}
		})
	}
	for _, raw := range []string{`null`, `{}`, `[{"name":"x","name":"y","data":"{}"}]`, `[{"name":"x","data":"{}","unknown":0}]`, `[`, `[null]`} {
		if _, err := compressedIntentForTest(t, raw).records(ctx); err == nil {
			t.Fatalf("invalid records accepted: %s", raw)
		}
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := carrier.records(cancelled); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation ignored", err)
	}
	legacy := vdocsIntentCarrier{Profile: vdocsIntentProfile, Version: 1, Records: records}
	legacy.GzipRecords = []byte{1}
	if _, err := legacy.records(ctx); err == nil {
		t.Fatal("legacy mixed encoding accepted")
	}

	c := compressedIntentForTest(t, `[{"name":"`+vdocsDocumentResource+`","data":"{\"version\":1,\"version\":1}"}]`)
	native := vdocsNoteSnapshot{Body: vdocsBody{CustomRanges: []vdocsRangeRecord{{Properties: sfAttrs(c)}}}}
	if err := checkVDOCSResourceJSON(ctx, native); err == nil {
		t.Fatal("duplicate resource JSON hidden by compression")
	}
}

func FuzzVDOCSCompressedIntent(f *testing.F) {
	f.Add([]byte("not gzip"), 100)
	c := compressedIntentForTestForFuzz()
	f.Add(c.GzipRecords, c.RecordsBytes)
	f.Fuzz(func(t *testing.T, data []byte, n int) {

		if len(data) > 65536 || n > 65536 {
			return
		}
		c := vdocsIntentCarrier{Profile: vdocsIntentProfile, Version: 2, GzipRecords: data, RecordsBytes: n}
		records, err := c.records(context.Background())
		if err == nil {
			if _, err := json.Marshal(records); err != nil {
				t.Fatal(err)
			}
		}
	})
}
func compressedIntentForTestForFuzz() vdocsIntentCarrier {
	raw := []byte(`[{"name":"manydoc.document-properties.v1","data":"{}"}]`)
	var buf bytes.Buffer
	z := gzip.NewWriter(&buf)
	_, _ = z.Write(raw)
	_ = z.Close()
	return vdocsIntentCarrier{Profile: vdocsIntentProfile, Version: 2, GzipRecords: buf.Bytes(), RecordsBytes: len(raw)}
}

func TestVDOCSPublishedPDFSourceGraph(t *testing.T) {
	ctx := context.Background()
	raw, err := os.ReadFile("testdata/real/copyright-office-circular-1-2021.pdf")
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprintf("%x", sha256.Sum256(raw)) != "a37450360b4b440cb10ccfd9ce37cb92ea8193884c5700948df942aa1c62920f" {
		t.Fatal("original publication changed")
	}
	want, err := ReadPDF(ctx, bytes.NewReader(raw), ReadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	original, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	current := want
	for pass := 0; pass < 2; pass++ {
		native, err := encodeVDOCSDocument(ctx, current)
		if err != nil {
			t.Fatal(err)
		}
		wire, err := json.Marshal(native)
		if err != nil {
			t.Fatal(err)
		}
		if pass == 0 {
			if file := os.Getenv("MANYDOC_VDOCS_REAL_QA"); file != "" {
				if err := os.WriteFile(file, wire, 0600); err != nil {
					t.Fatal(err)
				}
			}
		}
		got, err := decodeVDOCSDocument(ctx, native)
		if err != nil {
			t.Fatal(err)
		}
		after, _ := json.Marshal(native)
		if !bytes.Equal(wire, after) {
			t.Fatal("native snapshot mutated")
		}
		assertEvidenceSF(t, want, got)
		current = got
		unchanged, _ := json.Marshal(want)
		if !bytes.Equal(original, unchanged) {
			t.Fatal("original source document mutated")
		}
	}
	if file := os.Getenv("MANYDOC_VDOCS_REAL_SAVED"); file != "" {
		wire, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var native vdocsNoteSnapshot
		if err := json.Unmarshal(wire, &native); err != nil {
			t.Fatal(err)
		}
		envelope, err := json.Marshal(vdocsEnvelope{vdocsEnvelopeType, 3, "univer", vdocsEngineVersion, native})
		if err != nil {
			t.Fatal(err)
		}
		got, err := readVDOCS(ctx, bytes.NewReader(envelope))
		if err != nil {
			t.Fatal(err)
		}
		assertEvidenceSF(t, want, got)
		again, err := encodeVDOCSDocument(ctx, got)
		if err != nil {
			t.Fatal(err)
		}
		reopened, err := decodeVDOCSDocument(ctx, again)
		if err != nil {
			t.Fatal(err)
		}
		assertEvidenceSF(t, want, reopened)
		sf, err := ToStructured(got)
		if err != nil {
			t.Fatal(err)
		}
		data, err := json.Marshal(sf)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file+".sf", data, 0600); err != nil {
			t.Fatal(err)
		}
	}
}
