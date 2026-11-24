package manydoc

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func colleafRunOriginExample(t *testing.T) *Document {
	d := colleafOriginExample(t)
	src := func(id string) *SourceInfo { s := cloneSourceInfo(d.Blocks[0].Source); s.Element = id; return s }
	d.Metadata.Title = "Run origins"
	d.Blocks[0].Runs[0].Source = src("first-run")
	d.Blocks[1].Runs[0].Source = src("second-run")
	d.Blocks[1].Runs[0].Link = "https://example.org/second"
	d.Blocks[2].Items[0].Blocks[0].Runs[0].Source = src("item-run")
	d.Blocks[3].Rows[0].Cells[0].Blocks[0].Runs[0].Source = src("cell-run")
	d.Headers[0].Runs[0].Source = src("header-run")
	d.Footers[0].Runs[0].Source = src("footer-run")
	d.Notes[0].Blocks[0].Runs[0].Source = src("note-run")
	d.Notes[0].BodyLabelContent = []Run{{Text: "α", Bold: true, FontSize: 13, Source: src("body-mark-run")}}
	d.Notes[0].Source = src("note-definition")
	d.Notes[0].LabelSource = src("note-label")
	cite := &d.Blocks[4].Runs[1]
	cite.Source = src("citation-run")
	cite.Note.LabelContent = []Run{{Text: "*", Italic: true, Source: src("citation-label-run")}}
	d.Blocks[4].Runs = append(d.Blocks[4].Runs, Run{Text: " / "}, Run{Source: src("default-citation-run"), Note: &NoteReference{Kind: Footnote, ID: "n"}})

	return d
}
func TestColleafRunOriginsAndActualSave(t *testing.T) {
	ctx := context.Background()
	want := colleafRunOriginExample(t)
	native, err := encodeColleafDocument(ctx, want)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(native)
	got, err := decodeColleafDocument(ctx, native)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(native)
	if !bytes.Equal(before, after) {
		t.Fatal("native source mutated")
	}
	assertEvidenceSF(t, want, got)
	if file := os.Getenv("MANYDOC_COLLEAF_RUN_ORIGIN_QA"); file != "" {
		if err = os.WriteFile(file, before, 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, spec := range []struct{ env, mode string }{{"MANYDOC_COLLEAF_RUN_ORIGIN_SAVED", "saved"}, {"MANYDOC_COLLEAF_RUN_ORIGIN_EDITED", "edited"}, {"MANYDOC_COLLEAF_RUN_ORIGIN_CITATION", "citation"}, {"MANYDOC_COLLEAF_RUN_ORIGIN_DETACHED", "detached"}} {
		t.Run(spec.mode, func(t *testing.T) {
			file := os.Getenv(spec.env)
			if file == "" {
				t.Skip("optional actual reader artifact")
			}
			raw, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			var wire colleafNoteSnapshot
			if err = json.Unmarshal(raw, &wire); err != nil {
				t.Fatal(err)
			}
			envelope, _ := json.Marshal(colleafEnvelope{colleafEnvelopeType, 3, "univer", colleafEngineVersion, wire})
			actual, err := readColleaf(ctx, bytes.NewReader(envelope))
			if err != nil {
				t.Fatal(err)
			}
			expected := colleafRunOriginExample(t)
			switch spec.mode {
			case "edited":
				left, right := expected.Blocks[0].Runs[0], expected.Blocks[0].Runs[0]
				left.Text = "😀 "
				right.Text = "First"
				left.Source = cloneSourceInfo(left.Source)
				right.Source = cloneSourceInfo(right.Source)
				left.Source.Historical = true
				right.Source.Historical = true
				expected.Blocks[0].Runs = []Run{left, {Text: "Q"}, right}
				expected.Blocks[0].Source.Historical = true
				expected.Diagnostics = []Diagnostic{{Code: "colleaf_run_source_historical", Message: "Original PDF source facts are retained as historical after current run content or presentation changed"}, {Code: "colleaf_source_historical", Message: "Original PDF source facts are retained as historical after current paragraph content or presentation changed"}}
			case "citation":
				expected.Blocks[4].Runs[1].Note.LabelContent[0].Text = "X"
				expected.Blocks[4].Runs[1].Note.LabelContent[0].Source.Historical = true
				expected.Blocks[4].Runs[1].Source.Historical = true
				expected.Diagnostics = []Diagnostic{{Code: "colleaf_run_source_historical", Message: "Original PDF source facts are retained as historical after current run content or presentation changed"}}
			case "detached":
				expected.UnboundSources = []SourceInfo{*cloneSourceInfo(expected.Blocks[0].Runs[0].Source)}
				expected.Blocks[0].Runs[0].Source = nil
				expected.Diagnostics = []Diagnostic{{Code: "colleaf_run_source_unbound", Message: "Original PDF source facts are retained unbound after their native run binding disappeared"}}
			}
			assertEvidenceSF(t, expected, actual)
			sf, err := ToStructured(actual)
			if err != nil {
				t.Fatal(err)
			}
			model, err := FromStructured(sf)
			if err != nil {
				t.Fatal(err)
			}
			next, err := encodeColleafDocument(ctx, model)
			if err != nil {
				t.Fatal(err)
			}
			stable, err := decodeColleafDocument(ctx, next)
			if err != nil {
				t.Fatal(err)
			}
			assertEvidenceSF(t, actual, stable)
			sfRaw, _ := json.Marshal(sf)
			if err = os.WriteFile(file+".sf", sfRaw, 0600); err != nil {
				t.Fatal(err)
			}
		})
	}
}
func TestColleafRunOriginMalformedAndUnsupportedSlots(t *testing.T) {
	ctx := context.Background()
	d := colleafRunOriginExample(t)
	native, err := encodeColleafDocument(ctx, d)
	if err != nil {
		t.Fatal(err)
	}
	opened, err := openColleafIntent(ctx, native)
	if err != nil {
		t.Fatal(err)
	}
	var intent colleafDocumentIntent
	if err = colleafResourceMap(opened.Resources, colleafDocumentResource, &intent); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"version", "digest", "duplicate", "missing"} {
		t.Run(mode, func(t *testing.T) {
			raw, _ := json.Marshal(intent)
			var copy colleafDocumentIntent
			_ = json.Unmarshal(raw, &copy)
			switch mode {
			case "version":
				copy.Evidence.RunOrigins[0].Version = 2
			case "digest":
				copy.Evidence.RunOrigins[0].Source.Element = "changed"
			case "duplicate":
				copy.Evidence.RunOrigins = append(copy.Evidence.RunOrigins, copy.Evidence.RunOrigins[0])
			case "missing":
				copy.Evidence.RunOrigins = nil
			}
			wire := opened
			wire.Resources = colleafReplaceResource(wire.Resources, colleafDocumentResource, copy)
			if _, err = decodeColleafDocument(ctx, wire); err == nil {
				t.Fatal("invalid run source accepted")
			}
		})
	}
	// Source-bearing empty text without a point identity is still explicitly rejected.
	source := cloneSourceInfo(d.Blocks[0].Runs[0].Source)
	for _, r := range []Run{{Source: source}} {
		if _, err = encodeColleafTextBody(ctx, []Block{{Kind: Paragraph, Runs: []Run{r}}}); err == nil {
			t.Fatal("unmapped empty source silently discarded")
		}
	}
}

func TestColleafRunOriginDirectOwnership(t *testing.T) {
	ctx := context.Background()
	for _, mode := range []string{"inherited", "overwritten", "contradictory"} {
		t.Run(mode, func(t *testing.T) {
			d := colleafRunOriginExample(t)
			wire, err := encodeColleafDocument(ctx, d)
			if err != nil {
				t.Fatal(err)
			}
			props, err := colleafStyleObject(wire.Body.TextRuns[0].TS)
			if err != nil {
				t.Fatal(err)
			}
			ref := props["manydocOrigin"]
			switch mode {
			case "inherited":
				delete(props, "manydocOrigin")
				wire.Body.TextRuns[0].TS = sfAttrs(props)
				style, _ := colleafStyleObject(wire.DocumentStyle)
				style["textStyle"] = sfAttrs(map[string]json.RawMessage{"manydocOrigin": ref})
				wire.DocumentStyle = sfAttrs(style)
			case "overwritten":
				delete(props, "manydocOrigin")
				last := wire.Body.TextRuns[0]
				last.TS = sfAttrs(props)
				wire.Body.TextRuns = append(wire.Body.TextRuns, last)
			case "contradictory":
				origin, err := readColleafOriginProfile(ref, colleafRunOriginProfile)
				if err != nil {
					t.Fatal(err)
				}
				origin.Fingerprint = strings.Repeat("0", 64)
				props["manydocOrigin"] = sfAttrs(origin)
				wire.Body.TextRuns[0].TS = sfAttrs(props)
			}
			got, err := decodeColleafDocument(ctx, wire)
			if mode == "contradictory" {
				if err == nil {
					t.Fatal("contradictory reference metadata accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.Blocks[0].Runs[0].Source != nil || len(got.UnboundSources) != 1 || got.UnboundSources[0].Element != "first-run" {
				t.Fatal("unowned source incorrectly attributed")
			}
			if got.Blocks[0].Source.Historical {
				t.Fatal("source-only editing staled paragraph provenance")
			}
			if mode == "inherited" {
				found := false
				for _, diagnostic := range got.Diagnostics {
					found = found || diagnostic.Code == "colleaf_run_origin_inheritance"
				}
				if !found {
					t.Fatal("missing inherited ownership warning")
				}
			}
		})
	}
}
func FuzzColleafRunOrigin(f *testing.F) {
	f.Add([]byte(sfAttrs(colleafOrigin{Profile: colleafRunOriginProfile, Version: 1, Ref: strings.Repeat("0", 64)})))
	f.Add([]byte(`{"profile":"manydoc.run-origin","version":2}`))
	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) > 32768 {
			return
		}
		_, _ = readColleafOriginProfile(raw, colleafRunOriginProfile)
	})
}
