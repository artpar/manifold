package manydoc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
)

func TestColleafRealDOCXLanguageRoundTrips(t *testing.T) {
	ctx := context.Background()
	snapshots := map[string]colleafNoteSnapshot{}
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
			native, err := encodeColleafDocument(ctx, want)
			if err != nil {
				t.Fatal(err)
			}
			got, err := decodeColleafDocument(ctx, native)
			if err != nil {
				t.Fatal(err)
			}
			assertEvidenceSF(t, want, got)
			snapshots[name] = native
			originals[name] = want
		})
	}
	originals["synthetic"] = colleafLanguageExample(t)
	synthetic, err := encodeColleafDocument(ctx, originals["synthetic"])
	if err != nil {
		t.Fatal(err)
	}
	snapshots["synthetic"] = synthetic
	got, err := decodeColleafDocument(ctx, synthetic)
	if err != nil {
		t.Fatal(err)
	}
	assertEvidenceSF(t, originals["synthetic"], got)
	for _, spec := range []struct{ env, mode string }{{"MANYDOC_COLLEAF_LANGUAGES_SAVED", "saved"}, {"MANYDOC_COLLEAF_LANGUAGES_EDITED", "edited"}, {"MANYDOC_COLLEAF_LANGUAGES_CLEARED", "cleared"}} {
		if file := os.Getenv(spec.env); file != "" {
			raw, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			var saved map[string]colleafNoteSnapshot
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
					want = colleafLanguageExample(t)
					if spec.mode == "edited" {
						want.Blocks[0].Runs[0].EastAsiaLanguage = "zh-Hant"
						want.Blocks[0].Runs[0].ComplexLanguage = "he-IL"
					}
					if spec.mode == "cleared" {
						want.Blocks[0].Runs[0].EastAsiaLanguage = ""
					}
				}
				envelope, err := json.Marshal(colleafEnvelope{colleafEnvelopeType, 3, "univer", colleafEngineVersion, native})
				if err != nil {
					t.Fatal(err)
				}
				got, err := readColleaf(ctx, bytes.NewReader(envelope))
				if err != nil {
					t.Fatalf("%s/%s: %v", spec.mode, name, err)
				}
				assertEvidenceSF(t, want, got)
				next, err := encodeColleafDocument(ctx, got)
				if err != nil {
					t.Fatal(err)
				}
				again, err := decodeColleafDocument(ctx, next)
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

	if file := os.Getenv("MANYDOC_COLLEAF_LANGUAGES_QA"); file != "" {
		raw, err := json.Marshal(snapshots)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(file, raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestColleafLanguageStyles(t *testing.T) {
	ctx := context.Background()
	want := &Document{Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "日本語", Language: "en-US", EastAsiaLanguage: "ja-JP", ComplexLanguage: "ar-SA"}}}}}
	native, err := encodeColleafDocument(ctx, want)
	if err != nil {
		t.Fatal(err)
	}
	got, err := decodeColleafDocument(ctx, native)
	if err != nil {
		t.Fatal(err)
	}
	assertEvidenceSF(t, want, got)
	for _, run := range []Run{{Text: "x", Direction: "rtl"}, {Text: "x", ComplexScript: true}} {
		if _, err := colleafRunStyle(run); !errors.Is(err, ErrUnsupported) {
			t.Fatal("unqualified rendering accepted", err)
		}
	}
}

func colleafLanguageExample(t *testing.T) *Document {
	d := colleafBodyMarksExample(t)
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

func TestColleafLanguageInheritanceAndValidation(t *testing.T) {
	ctx := context.Background()
	marker := sfAttrs(colleafLanguages{Profile: colleafLanguageProfile, Version: 1, AlternateKind: "eastAsia", ComplexLanguage: "ar-SA"})
	body := colleafBody{DataStream: "ab\r\n", TextRuns: []colleafTextRun{{St: 0, Ed: 1, StyleID: "child"}, {St: 1, Ed: 2, StyleID: "child", TS: sfAttrs(map[string]any{"altLang": "zh-Hant", "manydocLanguages": nil})}}}
	settings := colleafTextContext{DefaultText: sfAttrs(map[string]any{"lang": "en-US"}), Styles: map[string]colleafNamedStyle{"parent": {TextStyle: sfAttrs(map[string]any{"altLang": "ja-JP", "manydocLanguages": marker})}, "child": {BasedOn: "parent"}}}
	doc, err := decodeColleafTextBody(ctx, body, settings)
	if err != nil {
		t.Fatal(err)
	}
	runs := doc.Blocks[0].Runs
	if len(runs) != 2 || runs[0].Language != "en-US" || runs[0].EastAsiaLanguage != "ja-JP" || runs[0].ComplexLanguage != "ar-SA" || runs[1].EastAsiaLanguage != "zh-Hant" || runs[1].ComplexLanguage != "" {
		t.Fatalf("language inheritance/clearing: %+v", runs)
	}
	if !hasDiagnostic(doc, "colleaf_alternate_language_inferred") {
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
		style, err := colleafStyleObject(json.RawMessage(raw))
		if err != nil {
			t.Fatal(err)
		}
		if _, err = colleafStyleRun(style, &Document{}); err == nil {
			t.Fatalf("malformed language accepted: %s", raw)
		}
	}
}

func TestColleafLanguageEditsQualifySources(t *testing.T) {
	ctx := context.Background()
	base := colleafOriginExample(t)
	want := &Document{Pages: base.Pages, Blocks: []Block{{Kind: Paragraph, Source: cloneSourceInfo(base.Blocks[0].Source), Runs: []Run{{Text: "日本語", EastAsiaLanguage: "ja-JP", ComplexLanguage: "ar-SA", Source: cloneSourceInfo(base.Blocks[0].Source)}}}}}
	native, err := encodeColleafDocument(ctx, want)
	if err != nil {
		t.Fatal(err)
	}
	got, err := decodeColleafDocument(ctx, native)
	if err != nil {
		t.Fatal(err)
	}
	assertEvidenceSF(t, want, got)
	props, err := colleafStyleObject(native.Body.TextRuns[0].TS)
	if err != nil {
		t.Fatal(err)
	}
	props["altLang"] = sfAttrs("zh-Hant")
	native.Body.TextRuns[0].TS = sfAttrs(props)
	got, err = decodeColleafDocument(ctx, native)
	if err != nil {
		t.Fatal(err)
	}
	b := got.Blocks[0]
	if b.Source == nil || !b.Source.Historical || b.Runs[0].Source == nil || !b.Runs[0].Source.Historical || b.Runs[0].EastAsiaLanguage != "zh-Hant" || b.Runs[0].ComplexLanguage != "ar-SA" {
		t.Fatal("edited language/source authority lost")
	}
	if !hasDiagnostic(got, "colleaf_run_source_historical") || !hasDiagnostic(got, "colleaf_source_historical") {
		t.Fatal("source uncertainty omitted", got.Diagnostics)
	}
}

func FuzzColleafRunLanguages(f *testing.F) {
	f.Add([]byte(`{"profile":"manydoc.run-languages","version":1,"alternateKind":"eastAsia","complexLanguage":"ar-SA"}`))
	f.Add([]byte(`null`))
	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) > 65536 {
			return
		}
		value, err := readColleafLanguages(raw)
		if err == nil && value != nil {
			if err := validateLanguage(value.ComplexLanguage); err != nil {
				t.Fatal(err)
			}
		}
	})
}
