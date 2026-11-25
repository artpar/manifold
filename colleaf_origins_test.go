package manifold

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

func colleafOriginExample(t *testing.T) *Document {
	t.Helper()
	history := colleafEvidenceExample(t)
	source := func(id string, order int) *SourceInfo {
		q := Quad{{10, 20}, {100, 20}, {100, 35}, {10, 35}}
		return &SourceInfo{Format: PDF, Page: 1, Element: id, Order: order, Quad: &q, Structure: "explicit", Evidence: "tagged_structure", Confidence: 1, Precision: "source_bounds", StructureID: []byte{0xff, 0, byte(order)}}
	}
	p := func(s, id string, n int) Block {
		return Block{Kind: Paragraph, Source: source(id, n), Runs: []Run{{Text: s, Bold: true, FontSize: 13}}}
	}
	return &Document{Pages: history.Pages, Metadata: Metadata{Title: "Paragraph origins", PageCount: 2}, Blocks: []Block{p("😀 First", "first", 1), p("Second", "second", 2), {Kind: ListBlock, List: "bullet", Items: []ListItem{{Blocks: []Block{p("Item", "item", 3)}}}}, {Kind: Table, Rows: []Row{{Cells: []Cell{{Blocks: []Block{p("Cell", "cell", 4)}}}}}}, {Kind: Paragraph, Runs: []Run{{Text: "Note"}, {Note: &NoteReference{Kind: Footnote, ID: "n"}}}}}, Headers: []Block{p("Header", "header", 5)}, Footers: []Block{p("Footer", "footer", 6)}, Notes: []Note{{Kind: Footnote, ID: "n", Blocks: []Block{p("Note body", "note", 7)}}}}
}
func TestColleafParagraphOriginsAndNativeSave(t *testing.T) {
	ctx := context.Background()
	want := colleafOriginExample(t)
	wire, err := encodeColleafDocument(ctx, want)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(wire)
	got, err := decodeColleafDocument(ctx, wire)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(wire)
	if !bytes.Equal(before, after) {
		t.Fatal("native source mutated")
	}
	assertEvidenceSF(t, want, got)
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
	round, err := decodeColleafDocument(ctx, again)
	if err != nil {
		t.Fatal(err)
	}
	assertEvidenceSF(t, want, round)
	got.Blocks[0].Source.StructureID[0] = 0
	againModel, err := decodeColleafDocument(ctx, wire)
	if err != nil {
		t.Fatal(err)
	}
	assertEvidenceSF(t, want, againModel)
	if file := os.Getenv("MANYDOC_COLLEAF_ORIGIN_QA"); file != "" {
		rawSF, err := json.Marshal(sf)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(file+".sf", rawSF, 0600); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(file, before, 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, spec := range []struct{ env, mode string }{{"MANYDOC_COLLEAF_ORIGIN_SAVED", "saved"}, {"MANYDOC_COLLEAF_ORIGIN_EDITED", "edited"}, {"MANYDOC_COLLEAF_ORIGIN_DETACHED", "detached"}} {
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
		envelope, err := json.Marshal(colleafEnvelope{colleafEnvelopeType, 3, "univer", colleafEngineVersion, native})
		if err != nil {
			t.Fatal(err)
		}
		actual, err := readColleaf(ctx, bytes.NewReader(envelope))
		if err != nil {
			t.Fatal(err)
		}
		expected, err := FromStructured(sf)
		if err != nil {
			t.Fatal(err)
		}
		switch spec.mode {
		case "edited":
			expected.Blocks[0].Runs = append([]Run{{Text: "Q"}}, expected.Blocks[0].Runs...)
			expected.Blocks[0].Source.Historical = true
			expected.Diagnostics = append(expected.Diagnostics, Diagnostic{Code: "colleaf_source_historical", Message: "Original PDF source facts are retained as historical after current paragraph content or presentation changed"})
		case "detached":
			expected.UnboundSources = []SourceInfo{*cloneSourceInfo(expected.Blocks[0].Source)}
			expected.Blocks[0].Source = nil
			expected.Diagnostics = append(expected.Diagnostics, Diagnostic{Code: "colleaf_source_unbound", Message: "Original PDF source facts are retained unbound after their native paragraph binding disappeared"})
		}
		aSF, err := ToStructured(actual)
		if err != nil {
			t.Fatal(err)
		}
		rawSF, err := json.Marshal(aSF)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(file+".sf", rawSF, 0600); err != nil {
			t.Fatal(err)
		}
		assertEvidenceSF(t, expected, actual)
		next, err := encodeColleafDocument(ctx, actual)
		if err != nil {
			t.Fatal(err)
		}
		stable, err := decodeColleafDocument(ctx, next)
		if err != nil {
			t.Fatal(err)
		}
		assertEvidenceSF(t, actual, stable)
	}
}
func TestColleafUnboundSourceSFAndFormattingAdmission(t *testing.T) {
	d := colleafOriginExample(t)
	source := cloneSourceInfo(d.Blocks[0].Source)
	d.Blocks = []Block{{Kind: Paragraph, Runs: []Run{{Text: "Current"}}}}
	d.Headers = nil
	d.Footers = nil
	d.Notes = nil
	d.UnboundSources = []SourceInfo{*source}
	sf, err := ToStructured(d)
	if err != nil {
		t.Fatal(err)
	}
	got, err := FromStructured(sf)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.UnboundSources, d.UnboundSources) {
		t.Fatal("unbound source changed")
	}
	got.UnboundSources[0].StructureID[0] = 0
	if d.UnboundSources[0].StructureID[0] != 0xff {
		t.Fatal("SF aliases original source")
	}
	for _, format := range []Format{DOCX, PDF} {
		var out bytes.Buffer
		if err = Write(context.Background(), &out, d, format, WriteOptions{}); err == nil || out.Len() != 0 {
			t.Fatal("formatting silently discarded unbound provenance", format, err)
		}
	}
}

func TestColleafParagraphOriginsExplicitSectionAndMalformed(t *testing.T) {
	ctx := context.Background()
	d := colleafOriginExample(t)
	header := &RunningStory{Blocks: d.Headers}
	footer := &RunningStory{Blocks: d.Footers}
	d.Headers = nil
	d.Footers = nil
	d.Blocks = []Block{{Kind: SectionBlock, Section: &Section{Headers: RunningStories{Default: header}, Footers: RunningStories{Default: footer}, Blocks: d.Blocks}}}
	d.Blocks[0].Section.Blocks[4].Source = cloneSourceInfo(d.Blocks[0].Section.Blocks[0].Source)
	wire, err := encodeColleafDocument(ctx, d)
	if err != nil {
		t.Fatal(err)
	}
	got, err := decodeColleafDocument(ctx, wire)
	if err != nil {
		t.Fatal(err)
	}
	assertEvidenceSF(t, d, got)
	opened, err := openColleafIntent(ctx, wire)
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"digest", "version", "missing"} {
		bad := opened
		bad.Resources = append([]colleafResource(nil), opened.Resources...)
		var intent colleafDocumentIntent
		if err = colleafResourceMap(opened.Resources, colleafDocumentResource, &intent); err != nil {
			t.Fatal(err)
		}
		switch mode {
		case "digest":
			intent.Evidence.Origins[0].Fingerprint = "0000000000000000000000000000000000000000000000000000000000000000"
		case "version":
			intent.Evidence.Origins[0].Version = 2
		case "missing":
			intent.Evidence.Origins = nil
		}
		for i, r := range bad.Resources {
			if r.Name == colleafDocumentResource {
				bad.Resources[i].Data = string(sfAttrs(intent))
			}
		}
		if _, err = decodeColleafDocument(ctx, bad); err == nil {
			t.Fatal("invalid origin record accepted", mode)
		}
	}
}

func FuzzColleafParagraphOrigins(f *testing.F) {
	ctx := context.Background()
	page := PDFPage{Number: 1, MediaBox: [4]float64{0, 0, 200, 300}, CropBox: [4]float64{0, 0, 200, 300}, Width: 200, Height: 300, UserUnit: 1, Transform: Matrix{1, 0, 0, -1, 0, 300}}
	d := &Document{Pages: []PDFPage{page}, Blocks: []Block{{Kind: Paragraph, Source: &SourceInfo{Format: PDF, Page: 1, Element: "p", Structure: "explicit", Evidence: "synthetic", Confidence: 1}, Runs: []Run{{Text: "Body"}}}}}
	wire, err := encodeColleafDocument(ctx, d)
	if err != nil {
		f.Fatal(err)
	}
	base, err := openColleafIntent(ctx, wire)
	if err != nil {
		f.Fatal(err)
	}
	var intent colleafDocumentIntent
	if err = colleafResourceMap(base.Resources, colleafDocumentResource, &intent); err != nil {
		f.Fatal(err)
	}
	seed, _ := json.Marshal(intent.Evidence.Origins[0])
	f.Add(seed)
	f.Add([]byte(`{"version":2}`))
	pageJSON, _ := json.Marshal(d.Pages)
	f.Fuzz(func(t *testing.T, raw []byte) {
		input := base
		input.Resources = append([]colleafResource(nil), base.Resources...)
		for i, r := range input.Resources {
			if r.Name == colleafDocumentResource {
				input.Resources[i].Data = `{"layouts":[],"evidence":{"version":1,"pages":` + string(pageJSON) + `,"origins":[` + string(raw) + `]}}`
			}
		}
		got, err := decodeColleafDocument(ctx, input)
		if err != nil {
			return
		}
		if err = got.Validate(); err != nil {
			t.Fatal(err)
		}
		if _, err = ToStructured(got); err != nil {
			t.Fatal(err)
		}
	})
}
