package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math/rand"
	"os"
	"reflect"
	"testing"
)

func vdocsHyperlinkExample(t testing.TB) *Document {
	p := func(label, url string) Block {
		return Block{Kind: Paragraph, Runs: []Run{{Text: label, Link: url, Language: "en-US", Bold: true, FontSize: 13, Color: "#123456"}}}
	}
	return &Document{Metadata: Metadata{Title: "Native hyperlink persistence"}, Blocks: []Block{
		p("Main 😀", "https://example.org/main"),
		{Kind: Table, Rows: []Row{{Cells: []Cell{{Blocks: []Block{p("Cell", "https://example.org/cell")}}}}}},
		{Kind: ListBlock, List: "number", Items: []ListItem{{Blocks: []Block{p("List", "https://example.org/list")}}}},
		{Kind: Paragraph, Runs: []Run{{Text: "Note "}, {Note: &NoteReference{Kind: Footnote, ID: "linked"}}}},
	}, Headers: []Block{p("Header", "https://example.org/header")}, Footers: []Block{p("Footer", "https://example.org/footer")}, Notes: []Note{{Kind: Footnote, ID: "linked", Blocks: []Block{p("Note link", "https://example.org/note")}}}}
}
func vdocsHyperlinkRecord(t testing.TB, body *vdocsBody) *vdocsRangeRecord {
	t.Helper()
	for i := range body.CustomRanges {
		if body.CustomRanges[i].Type == 0 {
			return &body.CustomRanges[i]
		}
	}
	t.Fatal("fixture lacks hyperlink")
	return nil
}
func TestVDOCSHyperlinkPluginSaveAndSF(t *testing.T) {
	ctx := context.Background()
	want := vdocsHyperlinkExample(t)
	wire, err := encodeVDOCSDocument(ctx, want)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	index := func(body vdocsBody) {
		for _, r := range body.CustomRanges {
			if r.Type == 0 {
				if seen[r.ID] {
					t.Fatal("hyperlink IDs collide across plugin stories", r.ID)
				}
				seen[r.ID] = true
			}
		}
	}
	index(wire.Body)
	for _, s := range wire.Headers {
		index(s.Body)
	}
	for _, s := range wire.Footers {
		index(s.Body)
	}
	if len(seen) != 5 {
		t.Fatal("missing hyperlink relationships", seen)
	}
	if file := os.Getenv("MANYDOC_VDOCS_HYPERLINK_QA"); file != "" {
		raw, err := json.Marshal(wire)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(file, raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, spec := range []struct {
		env        string
		edited     bool
		diagnostic bool
	}{{"MANYDOC_VDOCS_HYPERLINK_SAVED", false, false}, {"MANYDOC_VDOCS_HYPERLINK_RESOURCE_EDITED", true, true}, {"MANYDOC_VDOCS_HYPERLINK_EDITED_SAVED", true, false}} {
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
		before, _ := json.Marshal(native)
		got, err := decodeVDOCSDocument(ctx, native)
		if err != nil {
			t.Fatal(spec.env, err)
		}
		envelope, err := json.Marshal(vdocsEnvelope{vdocsEnvelopeType, 3, "univer", vdocsEngineVersion, native})
		if err != nil {
			t.Fatal(err)
		}
		fromBytes, err := readVDOCS(ctx, bytes.NewReader(envelope))
		if err != nil {
			t.Fatal(spec.env, "native file boundary", err)
		}
		if !reflect.DeepEqual(fromBytes, got) {
			t.Fatal("strict native file boundary changed hyperlink semantics")
		}
		after, _ := json.Marshal(native)
		if !bytes.Equal(before, after) {
			t.Fatal("native import mutated caller snapshot")
		}
		expected := vdocsHyperlinkExample(t)
		if spec.edited {
			expected.Headers[0].Runs[0].Link = "https://example.org/header-edited"
		}
		if spec.diagnostic {
			if len(got.Diagnostics) != 1 || got.Diagnostics[0].Code != "vdocs_hyperlink_resource_target" {
				t.Fatal("resource precedence not disclosed", got.Diagnostics)
			}
			expected.Diagnostics = got.Diagnostics
		}
		if !reflect.DeepEqual(got, expected) {
			a, _ := json.Marshal(got)
			b, _ := json.Marshal(expected)
			t.Fatalf("%s changed semantics\ngot%s\nwant%s", spec.env, a, b)
		}
		sf, err := ToStructured(got)
		if err != nil {
			t.Fatal(err)
		}
		model, err := FromStructured(sf)
		if err != nil {
			t.Fatal(err)
		}
		// The documented SF bridge normalizes equivalent zero/unit cell spans.
		// Keep every other field, including all link/style/story semantics, exact.
		expected.Blocks[1].Rows[0].Cells[0].ColSpan = 1
		if !reflect.DeepEqual(model, expected) {
			a, _ := json.Marshal(model)
			b, _ := json.Marshal(expected)
			t.Fatalf("SF lost current hyperlink semantics\ngot%s\nwant%s", a, b)
		}
	}
}
func TestVDOCSHyperlinkResourceScopePrecedenceAndImmutability(t *testing.T) {
	ctx := context.Background()
	original := vdocsHyperlinkExample(t)
	wire, err := encodeVDOCSDocument(ctx, original)
	if err != nil {
		t.Fatal(err)
	}
	var headerID string
	for _, s := range wire.Headers {
		headerID = vdocsHyperlinkRecord(t, &s.Body).ID
	}
	wire.Resources = append(wire.Resources, vdocsResource{Name: vdocsHyperlinkResource, Data: string(sfAttrs(map[string]any{"links": []any{map[string]string{"id": headerID, "payload": "https://example.org/header-edited"}}}))})
	before, _ := json.Marshal(wire)
	got, err := decodeVDOCSDocument(ctx, wire)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(wire)
	if !bytes.Equal(before, after) {
		t.Fatal("resource application mutated input")
	}
	if got.Headers[0].Runs[0].Link != "https://example.org/header-edited" || got.Blocks[0].Runs[0].Link != original.Blocks[0].Runs[0].Link || got.Notes[0].Blocks[0].Runs[0].Link != original.Notes[0].Blocks[0].Runs[0].Link {
		t.Fatal("resource updated the wrong story")
	}
	if len(got.Diagnostics) != 1 || got.Diagnostics[0].Code != "vdocs_hyperlink_resource_target" {
		t.Fatal(got.Diagnostics)
	}
	// A resource-only URL is a real load input, not merely a redundant mirror.

	// Cross the actual canonical SF, with the missing internal target declared.

	// Native visible text and the last native style record remain authoritative.

	// A decoration is an object, not a BooleanNumber: malformed input must fail.

	main := vdocsHyperlinkRecord(t, &wire.Body)
	main.Properties = sfAttrs(map[string]string{"textStyleMode": "text"})
	wire.Resources[len(wire.Resources)-1].Data = string(sfAttrs(map[string]any{"links": []any{map[string]string{"id": main.ID, "payload": "https://example.org/recovered"}}}))
	recovered, err := decodeVDOCSDocument(ctx, wire)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Blocks[0].Runs[0].Link != "https://example.org/recovered" {
		t.Fatal("resource-only URL not recovered")
	}
	main.Properties = sfAttrs(map[string]string{"url": "https://example.org/main", "textStyleMode": "text"})
	wire.Resources[len(wire.Resources)-1].Data = `{"links":[{"id":"deleted","payload":"https://example.org/deleted"}]}`
	got, err = decodeVDOCSDocument(ctx, wire)
	if err != nil {
		t.Fatal(err)
	}
	if got.Blocks[0].Runs[0].Link != original.Blocks[0].Runs[0].Link || len(got.Diagnostics) != 1 || got.Diagnostics[0].Code != "vdocs_hyperlink_resource_orphan" {
		t.Fatal("inactive resource lost visible links", got.Diagnostics)
	}
}
func TestVDOCSHyperlinkResourceMalformedAndConflicting(t *testing.T) {
	ctx := context.Background()
	for _, data := range []string{``, `null`, `{}`, `{"links":null}`, `{"links":[null]}`, `{"links":[{"id":"x"}]}`, `{"links":[{"id":"","payload":""}]}`, `{"Links":[]}`, `{"links":[{"ID":"x","payload":""}]}`, `{"links":[{"id":"x","payload":3}]}`, `{"links":[{"id":"x","payload":"\ud800"}]}`, `{"links":[{"id":"x","payload":"a","\u0070ayload":"b"}]}`, `{"links":[{"id":"x","payload":"a"},{"id":"x","payload":"b"}]}`} {
		wire, err := encodeVDOCSDocument(ctx, vdocsHyperlinkExample(t))
		if err != nil {
			t.Fatal(err)
		}
		wire.Resources = append(wire.Resources, vdocsResource{Name: vdocsHyperlinkResource, Data: data})
		if _, err = decodeVDOCSDocument(ctx, wire); err == nil {
			t.Fatal("malformed native hyperlink resource accepted", data)
		}
	}
	wire, err := encodeVDOCSDocument(ctx, vdocsHyperlinkExample(t))
	if err != nil {
		t.Fatal(err)
	}
	wire.Resources = append(wire.Resources, vdocsResource{Name: vdocsHyperlinkResource, Data: `{"links":[]}`}, vdocsResource{Name: vdocsHyperlinkResource, Data: `{"links":[]}`})
	if _, err = decodeVDOCSDocument(ctx, wire); err == nil {
		t.Fatal("duplicate native resource accepted")
	}
	wire.Resources = wire.Resources[:len(wire.Resources)-1]
	main := vdocsHyperlinkRecord(t, &wire.Body)
	for id, s := range wire.Headers {
		vdocsHyperlinkRecord(t, &s.Body).ID = main.ID
		wire.Headers[id] = s
	}
	if _, err = decodeVDOCSDocument(ctx, wire); err == nil {
		t.Fatal("ambiguous cross-story link ID accepted")
	}
	ctx, cancel := context.WithCancel(ctx)
	cancel()
	if _, _, err = applyVDOCSHyperlinkResource(ctx, wire); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
func TestVDOCSHyperlinkResourceInternalPlaceholder(t *testing.T) {
	wire, err := encodeVDOCSDocument(context.Background(), vdocsHyperlinkExample(t))
	if err != nil {
		t.Fatal(err)
	}
	a := vdocsHyperlinkRecord(t, &wire.Body)
	a.Properties = sfAttrs(map[string]string{"bookmarkId": "target", "textStyleMode": "text"})
	wire.Resources = append(wire.Resources, vdocsResource{Name: vdocsHyperlinkResource, Data: string(sfAttrs(map[string]any{"links": []any{map[string]string{"id": a.ID, "payload": ""}}}))})
	if _, _, err = applyVDOCSHyperlinkResource(context.Background(), wire); err != nil {
		t.Fatal(err)
	}
	wire.Resources[len(wire.Resources)-1].Data = string(sfAttrs(map[string]any{"links": []any{map[string]string{"id": a.ID, "payload": "https://example.org/conflict"}}}))
	if _, _, err = applyVDOCSHyperlinkResource(context.Background(), wire); !errors.Is(err, ErrUnsupported) {
		t.Fatal("conflicting internal/external targets accepted", err)
	}
}
func FuzzVDOCSHyperlinkResource(f *testing.F) {
	for _, data := range []string{`{"links":[]}`, `{"links":[{"id":"missing","payload":"https://example.org/"}]}`, `{"links":[{"id":"missing","payload":"\ud800"}]}`} {
		f.Add(data)
	}
	f.Fuzz(func(t *testing.T, data string) {
		wire, err := encodeVDOCSDocument(context.Background(), vdocsHyperlinkExample(t))
		if err != nil {
			t.Fatal(err)
		}
		wire.Resources = append(wire.Resources, vdocsResource{Name: vdocsHyperlinkResource, Data: data})
		got, err := decodeVDOCSDocument(context.Background(), wire)
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
func TestVDOCSRealDOCXLanguageRoundTrips(t *testing.T) {
	ctx := context.Background()
	snapshots := map[string]vdocsNoteSnapshot{}
	originals := map[string]*Document{}
	for _, name := range []string{"poi-footnotes.docx", "poi-endnotes.docx", "poi-headerFooter.docx", "poi-HeaderFooterUnicode.docx", "poi-DiffFirstPageHeadFoot.docx"} {
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile("testdata/real/" + name)
			if err != nil {
				t.Fatal(err)
			}
			want, err := ReadDOCX(ctx, bytes.NewReader(raw))
			if err != nil {
				t.Fatal(err)
			}
			native, err := encodeVDOCSDocument(ctx, want)
			if err != nil {
				t.Fatal(err)
			}
			got, err := decodeVDOCSDocument(ctx, native)
			if err != nil {
				t.Fatal(err)
			}
			assertEvidenceSF(t, want, got)
			snapshots[name] = native
			originals[name] = want
		})
	}
	originals["synthetic"] = vdocsLanguageExample(t)
	synthetic, err := encodeVDOCSDocument(ctx, originals["synthetic"])
	if err != nil {
		t.Fatal(err)
	}
	snapshots["synthetic"] = synthetic
	got, err := decodeVDOCSDocument(ctx, synthetic)
	if err != nil {
		t.Fatal(err)
	}
	assertEvidenceSF(t, originals["synthetic"], got)
	for _, spec := range []struct{ env, mode string }{{"MANYDOC_VDOCS_LANGUAGES_SAVED", "saved"}, {"MANYDOC_VDOCS_LANGUAGES_EDITED", "edited"}, {"MANYDOC_VDOCS_LANGUAGES_CLEARED", "cleared"}} {
		if file := os.Getenv(spec.env); file != "" {
			raw, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			var saved map[string]vdocsNoteSnapshot
			if err = json.Unmarshal(raw, &saved); err != nil {
				t.Fatal(err)
			}
			if len(saved) != len(originals) {
				t.Fatal("missing actual reader case")
			}
			for name, native := range saved {
				want := originals[name]
				if want == nil {
					t.Fatal("unexpected reader case", name)
				}
				if name == "synthetic" {
					want = vdocsLanguageExample(t)
					if spec.mode == "edited" {
						want.Blocks[0].Runs[0].EastAsiaLanguage = "zh-Hant"
						want.Blocks[0].Runs[0].ComplexLanguage = "he-IL"
					}
					if spec.mode == "cleared" {
						want.Blocks[0].Runs[0].EastAsiaLanguage = ""
					}
				}
				envelope, err := json.Marshal(vdocsEnvelope{vdocsEnvelopeType, 3, "univer", vdocsEngineVersion, native})
				if err != nil {
					t.Fatal(err)
				}
				got, err := readVDOCS(ctx, bytes.NewReader(envelope))
				if err != nil {
					t.Fatalf("%s/%s: %v", spec.mode, name, err)
				}
				assertEvidenceSF(t, want, got)
				next, err := encodeVDOCSDocument(ctx, got)
				if err != nil {
					t.Fatal(err)
				}
				again, err := decodeVDOCSDocument(ctx, next)
				if err != nil {
					t.Fatal(err)
				}
				assertEvidenceSF(t, want, again)
				sf, err := ToStructured(got)
				if err != nil {
					t.Fatal(err)
				}
				data, err := json.Marshal(sf)
				if err != nil {
					t.Fatal(err)
				}
				if err = os.WriteFile(file+"-"+name+".sf", data, 0600); err != nil {
					t.Fatal(err)
				}
			}
		}
	}

	if file := os.Getenv("MANYDOC_VDOCS_LANGUAGES_QA"); file != "" {
		raw, err := json.Marshal(snapshots)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(file, raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestVDOCSLanguageStyles(t *testing.T) {
	ctx := context.Background()
	want := &Document{Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "日本語", Language: "en-US", EastAsiaLanguage: "ja-JP", ComplexLanguage: "ar-SA"}}}}}
	native, err := encodeVDOCSDocument(ctx, want)
	if err != nil {
		t.Fatal(err)
	}
	got, err := decodeVDOCSDocument(ctx, native)
	if err != nil {
		t.Fatal(err)
	}
	assertEvidenceSF(t, want, got)
	for _, run := range []Run{{Text: "x", Direction: "rtl"}, {Text: "x", ComplexScript: true}} {
		if _, err := vdocsRunStyle(run); !errors.Is(err, ErrUnsupported) {
			t.Fatal("unqualified rendering accepted", err)
		}
	}
}

func vdocsLanguageExample(t *testing.T) *Document {
	d := vdocsBodyMarksExample(t)
	d.Blocks[0].Runs[0] = Run{Text: "日本語😀", Bold: true, FontSize: 13}
	d.Headers = []Block{{Kind: Paragraph, Runs: []Run{{Text: "Header"}}}}
	d.Footers = []Block{{Kind: Paragraph, Runs: []Run{{Text: "Footer"}}}}
	d.Blocks = append(d.Blocks, Block{Kind: ListBlock, List: "bullet", Items: []ListItem{{Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "Item"}}}}}}}, Block{Kind: Table, Rows: []Row{{Cells: []Cell{{Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "Cell"}}}}}}}}})
	var runs func([]Run)
	runs = func(rs []Run) {
		for i := range rs {
			r := &rs[i]
			r.Language = "en-US"
			r.EastAsiaLanguage = "ja-JP"
			r.ComplexLanguage = "ar-SA"
			if r.Note != nil {
				runs(r.Note.LabelContent)
			}
		}
	}
	var blocks func([]Block)
	blocks = func(bs []Block) {
		for i := range bs {
			b := &bs[i]
			runs(b.Runs)
			for j := range b.Items {
				blocks(b.Items[j].Blocks)
			}
			for j := range b.Rows {
				for k := range b.Rows[j].Cells {
					blocks(b.Rows[j].Cells[k].Blocks)
				}
			}
		}
	}
	blocks(d.Blocks)
	blocks(d.Headers)
	blocks(d.Footers)
	for i := range d.Notes {
		n := &d.Notes[i]
		blocks(n.Blocks)
		runs(n.BodyLabelContent)
		if n.BodyLabelStyle != nil {
			n.BodyLabelStyle.Language = "en-US"
			n.BodyLabelStyle.EastAsiaLanguage = "ja-JP"
			n.BodyLabelStyle.ComplexLanguage = "ar-SA"
		}
	}
	return d
}

func TestVDOCSLanguageInheritanceAndValidation(t *testing.T) {
	ctx := context.Background()
	marker := sfAttrs(vdocsLanguages{Profile: vdocsLanguageProfile, Version: 1, AlternateKind: "eastAsia", ComplexLanguage: "ar-SA"})
	body := vdocsBody{DataStream: "ab\r\n", TextRuns: []vdocsTextRun{{St: 0, Ed: 1, StyleID: "child"}, {St: 1, Ed: 2, StyleID: "child", TS: sfAttrs(map[string]any{"altLang": "zh-Hant", "manydocLanguages": nil})}}}
	settings := vdocsTextContext{DefaultText: sfAttrs(map[string]any{"lang": "en-US"}), Styles: map[string]vdocsNamedStyle{"parent": {TextStyle: sfAttrs(map[string]any{"altLang": "ja-JP", "manydocLanguages": marker})}, "child": {BasedOn: "parent"}}}
	doc, err := decodeVDOCSTextBody(ctx, body, settings)
	if err != nil {
		t.Fatal(err)
	}
	runs := doc.Blocks[0].Runs
	if len(runs) != 2 || runs[0].Language != "en-US" || runs[0].EastAsiaLanguage != "ja-JP" || runs[0].ComplexLanguage != "ar-SA" || runs[1].EastAsiaLanguage != "zh-Hant" || runs[1].ComplexLanguage != "" {
		t.Fatalf("language inheritance/clearing: %+v", runs)
	}
	if !hasDiagnostic(doc, "vdocs_alternate_language_inferred") {
		t.Fatal("foreign slot inference not reported")
	}
	for _, raw := range []string{
		`{"altLang":"bad_tag"}`,
		`{"altLang":123}`,
		`{"manydocLanguages":{"profile":"manydoc.run-languages","version":1,"alternateKind":"eastAsia","complexLanguage":"bad_tag"}}`,
		`{"manydocLanguages":{"profile":"manydoc.run-languages","version":2,"alternateKind":"eastAsia"}}`,
		`{"manydocLanguages":{"profile":"manydoc.run-languages","version":1,"alternateKind":"complex"}}`,
		`{"manydocLanguages":{"profile":"manydoc.run-languages","version":1,"alternateKind":"eastAsia","unknown":1}}`,
		`{"manydocLanguages":{"profile":"manydoc.run-languages","version":1,"alternateKind":"eastAsia","ComplexLanguage":"ar"}}`,
		`{"manydocLanguages":{"profile":"manydoc.run-languages","version":1,"version":1,"alternateKind":"eastAsia"}}`,
		`{"manydocLanguages":{"profile":"manydoc.run-languages","version":1,"alternateKind":"eastAsia","complexLanguage":null}}`,
	} {
		style, err := vdocsStyleObject(json.RawMessage(raw))
		if err != nil {
			t.Fatal(err)
		}
		if _, err = vdocsStyleRun(style, &Document{}); err == nil {
			t.Fatalf("malformed language accepted: %s", raw)
		}
	}
}

func TestVDOCSLanguageEditsQualifySources(t *testing.T) {
	ctx := context.Background()
	base := vdocsOriginExample(t)
	want := &Document{Pages: base.Pages, Blocks: []Block{{Kind: Paragraph, Source: cloneSourceInfo(base.Blocks[0].Source), Runs: []Run{{Text: "日本語", EastAsiaLanguage: "ja-JP", ComplexLanguage: "ar-SA", Source: cloneSourceInfo(base.Blocks[0].Source)}}}}}
	native, err := encodeVDOCSDocument(ctx, want)
	if err != nil {
		t.Fatal(err)
	}
	got, err := decodeVDOCSDocument(ctx, native)
	if err != nil {
		t.Fatal(err)
	}
	assertEvidenceSF(t, want, got)
	props, err := vdocsStyleObject(native.Body.TextRuns[0].TS)
	if err != nil {
		t.Fatal(err)
	}
	props["altLang"] = sfAttrs("zh-Hant")
	native.Body.TextRuns[0].TS = sfAttrs(props)
	got, err = decodeVDOCSDocument(ctx, native)
	if err != nil {
		t.Fatal(err)
	}
	b := got.Blocks[0]
	if b.Source == nil || !b.Source.Historical || b.Runs[0].Source == nil || !b.Runs[0].Source.Historical || b.Runs[0].EastAsiaLanguage != "zh-Hant" || b.Runs[0].ComplexLanguage != "ar-SA" {
		t.Fatal("edited language/source authority lost")
	}
	if !hasDiagnostic(got, "vdocs_run_source_historical") || !hasDiagnostic(got, "vdocs_source_historical") {
		t.Fatal("source uncertainty omitted", got.Diagnostics)
	}
}

func FuzzVDOCSRunLanguages(f *testing.F) {
	f.Add([]byte(`{"profile":"manydoc.run-languages","version":1,"alternateKind":"eastAsia","complexLanguage":"ar-SA"}`))
	f.Add([]byte(`null`))
	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) > 65536 {
			return
		}
		value, err := readVDOCSLanguages(raw)
		if err == nil && value != nil {
			if err := validateLanguage(value.ComplexLanguage); err != nil {
				t.Fatal(err)
			}
		}
	})
}
func TestVDOCSStyleSweepMatchesLastWholeRecord(t *testing.T) {
	ctx := context.Background()
	index, err := newVDOCSTextIndex(ctx, "abcdefghij\r\n")
	if err != nil {
		t.Fatal(err)
	}
	random := rand.New(rand.NewSource(417))
	for trial := 0; trial < 300; trial++ {
		runs := make([]vdocsTextRun, random.Intn(40))
		for i := range runs {
			a, b := random.Intn(13), random.Intn(13)
			if a > b {
				a, b = b, a
			}
			runs[i] = vdocsTextRun{St: a, Ed: b}
		}
		spans, err := vdocsStyleSpans(ctx, index, runs)
		if err != nil {
			t.Fatal(err)
		}
		covered := 0
		for _, span := range spans {
			if span.start != covered || span.end <= span.start {
				t.Fatalf("nonpartition %+v", spans)
			}
			for p := span.start; p < span.end; p++ {
				want := -1
				for j, r := range runs {
					if p >= r.St && p < r.Ed {
						want = j
					}
				}
				if span.record != want {
					t.Fatalf("trial%d offset%d got%d want%d", trial, p, span.record, want)
				}
			}
			covered = span.end
		}
		if covered != index.length() {
			t.Fatal("incomplete partition")
		}
	}
}

func vdocsTextExample() []Block {
	return []Block{
		{Kind: Heading, Level: 6, Direction: "rtl", PageBreakBefore: true, Runs: []Run{{Text: "Title 😀", Bold: true, Language: "en-US", FontFamily: "Go", FontSize: 16, Color: "#123ABC"}}},
		{Kind: Paragraph, Runs: []Run{{Text: "Link 😀", Italic: true, Underline: true, Link: "https://example.org", FontSize: 12}, {Text: "\nnext\tcell", Strike: true, Destination: "destination", Color: "auto"}}},
		{Kind: Paragraph},
	}
}
func TestVDOCSTextSemanticRoundTripAndNativeEdits(t *testing.T) {
	ctx := context.Background()
	want := vdocsTextExample()
	body, err := encodeVDOCSTextBody(ctx, want)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := decodeVDOCSTextBody(ctx, body, vdocsTextContext{})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(doc.Blocks, want) {
		t.Fatalf("semantic difference\ngot%+v\nwant%+v", doc.Blocks, want)
	}

	doc.UnresolvedDestinations = []string{"destination"}
	sf, err := ToStructured(doc)
	if err != nil {
		t.Fatal(err)
	}
	throughSF, err := FromStructured(sf)
	if err != nil {
		t.Fatal(err)
	}
	sfBody, err := encodeVDOCSTextBody(ctx, throughSF.Blocks)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(sfBody, body) {
		t.Fatal("SF bridge changed native records")
	}
	if len(doc.Diagnostics) > 0 {
		t.Fatal(doc.Diagnostics)
	}
	if body.TextRuns[0].Ed != 8 || body.CustomRanges[0].EndIndex-body.CustomRanges[0].StartIndex != 6 {
		t.Fatal("UTF-16 extents", body)
	}
	if file := os.Getenv("MANYDOC_VDOCS_TEXT_QA"); file != "" {
		data, err := json.MarshalIndent(map[string]any{"id": "manydoc-text-qa", "title": "Native text authority", "body": body, "documentStyle": map[string]any{"documentFlavor": 1}}, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(file, data, 0600); err != nil {
			t.Fatal(err)
		}
	}

	body.DataStream = "Edited😀" + body.DataStream[len("Title 😀"):]
	body.TextRuns = append(body.TextRuns, vdocsTextRun{St: 0, Ed: 8, TS: sfAttrs(map[string]any{"it": 1})})
	doc, err = decodeVDOCSTextBody(ctx, body, vdocsTextContext{})
	if err != nil {
		t.Fatal(err)
	}
	if got := doc.Blocks[0].Runs[0]; got.Text != "Edited😀" || !got.Italic || got.Bold || got.FontSize != 0 || got.Language != "" {
		t.Fatalf("stale text/style authority %+v", got)
	}
	if file := os.Getenv("MANYDOC_VDOCS_TEXT_EDIT"); file != "" {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var snapshot struct {
			Body vdocsBody `json:"body"`
		}
		if err = json.Unmarshal(data, &snapshot); err != nil {
			t.Fatal(err)
		}
		doc, err = decodeVDOCSTextBody(ctx, snapshot.Body, vdocsTextContext{})
		if err != nil {
			t.Fatal(err)
		}
		if got := doc.Blocks[0].Runs[0]; got.Text != "Edited😀" || !got.Italic || got.Bold {
			t.Fatalf("SDK saved edit lost %+v", got)
		}
	}
}

func TestVDOCSInheritedStylesAndLossDiagnostics(t *testing.T) {
	body := vdocsBody{DataStream: "ABCD\r\n", Paragraphs: []vdocsParagraphRecord{{StartIndex: 4, StyleID: "child", Style: sfAttrs(map[string]any{"direction": 2, "spaceAbove": map[string]int{"v": 12, "u": 0}, "textStyle": map[string]int{"it": 0}})}}, TextRuns: []vdocsTextRun{{St: 1, Ed: 4, TS: sfAttrs(map[string]int{"bl": 1})}, {St: 0, Ed: 3, StyleID: "character", TS: sfAttrs(map[string]int{"bl": 0})}}}
	settings := vdocsTextContext{DefaultText: sfAttrs(map[string]any{"ff": "Go", "fs": 11, "it": 1}), Styles: map[string]vdocsNamedStyle{
		"base":      {TextStyle: sfAttrs(map[string]int{"bl": 1}), ParagraphStyle: sfAttrs(map[string]int{"outlineLevel": 2})},
		"child":     {BasedOn: "base", TextStyle: sfAttrs(map[string]any{"cl": map[string]string{"rgb": "rgb(1, 2, 255)"}})},
		"character": {TextStyle: sfAttrs(map[string]int{"st": 1})},
	}}

	if _, err := decodeVDOCSTextBody(context.Background(), body, settings); err == nil {
		t.Fatal("invalid decoration accepted")
	}
	settings.Styles["character"] = vdocsNamedStyle{TextStyle: sfAttrs(map[string]any{"st": map[string]int{"s": 1}})}
	doc, err := decodeVDOCSTextBody(context.Background(), body, settings)
	if err != nil {
		t.Fatal(err)
	}
	block := doc.Blocks[0]
	if block.Kind != Heading || block.Level != 3 || block.Direction != "rtl" || len(block.Runs) != 2 {
		t.Fatal(block)
	}
	if a, b := block.Runs[0], block.Runs[1]; a.Text != "ABC" || a.Bold || a.Italic || !a.Strike || a.FontFamily != "Go" || a.FontSize != 11 || a.Color != "#0102FF" || b.Text != "D" || !b.Bold || b.Strike {
		t.Fatalf("inheritance/winner %+v", block.Runs)
	}
	if len(doc.Diagnostics) != 1 || doc.Diagnostics[0].Code != "vdocs_paragraph_style" {
		t.Fatal(doc.Diagnostics)
	}
	resolver := vdocsStyleResolver{named: map[string]vdocsNamedStyle{"a": {BasedOn: "b"}, "b": {BasedOn: "a"}}}
	if _, err = resolver.resolve("a", 0); err == nil {
		t.Fatal("style cycle accepted")
	}
	if _, err = resolver.resolve("missing", 0); err == nil {
		t.Fatal("missing style accepted")
	}
}

func TestVDOCSTextRejectsUnmappedAndMalformedContent(t *testing.T) {
	ctx := context.Background()
	for _, raw := range []string{"null", "true", "2", "1.0", "\"1\""} {
		if _, err := vdocsBool(json.RawMessage(raw)); err == nil {
			t.Fatal("invalid boolean", raw)
		}
	}
	for _, style := range []string{`{"fs":0}`, `{"ul":{"s":null}}`, `{"cl":{"th":1}}`, `{"hidden":true}`} {
		props, err := vdocsStyleObject(json.RawMessage(style))
		if err != nil {
			t.Fatal(err)
		}
		if _, err = vdocsStyleRun(props, &Document{}); err == nil {
			t.Fatal("invalid/unmapped style", style)
		}
	}
	for _, blocks := range [][]Block{{{Kind: Table}}, {{Kind: Paragraph, Runs: []Run{{Text: "A", Note: &NoteReference{}}}}}, {{Kind: Paragraph, Runs: []Run{{Text: "A\rB"}}}}} {
		if _, err := encodeVDOCSTextBody(ctx, blocks); !errors.Is(err, ErrUnsupported) {
			t.Fatal("partial encoding", err)
		}
	}
	for _, body := range []vdocsBody{
		{DataStream: "A\r\nB\r\n"},
		{DataStream: "A\r\n", Paragraphs: []vdocsParagraphRecord{{StartIndex: 1, Style: json.RawMessage(`{"outlineLevel":null}`)}}},
		{DataStream: "A\r\n", CustomRanges: []vdocsRangeRecord{{StartIndex: 0, EndIndex: 0, ID: "a", Type: 4}}},
		{DataStream: "A\r\n", CustomRanges: []vdocsRangeRecord{{StartIndex: 0, EndIndex: 0, ID: "a"}, {StartIndex: 0, EndIndex: 0, ID: "b"}}},
	} {
		if _, err := decodeVDOCSTextBody(ctx, body, vdocsTextContext{}); err == nil {
			t.Fatal("partial/malformed decoding", body)
		}
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := encodeVDOCSTextBody(canceled, vdocsTextExample()); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
