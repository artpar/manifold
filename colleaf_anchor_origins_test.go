package manifold

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func colleafAnchorOriginExample(t *testing.T) *Document {
	base := colleafOriginExample(t)
	point := func(id string) Run {
		s := cloneSourceInfo(base.Blocks[0].Source)
		s.Element = id
		s.Role = "Reference"
		return Run{Anchor: id, Source: s}
	}
	p := func(text, id string) Block { return Block{Kind: Paragraph, Runs: []Run{{Text: text}, point(id)}} }
	return &Document{Pages: base.Pages, Metadata: Metadata{Title: "Anchor origins"}, Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "😀A", Bold: true, FontSize: 13}, point("middle"), {Text: " tail"}, point("tail")}}, {Kind: Paragraph, Runs: []Run{{Text: "Jump", Destination: "middle"}}}, {Kind: ListBlock, List: "bullet", Items: []ListItem{{Blocks: []Block{p("Item", "item")}}}}, {Kind: Table, Rows: []Row{{Cells: []Cell{{Blocks: []Block{p("Cell", "cell")}}}}}}, {Kind: Paragraph, Runs: []Run{{Note: &NoteReference{Kind: Footnote, ID: "n"}}}}}, Headers: []Block{p("Header", "header")}, Footers: []Block{p("Footer", "footer")}, Notes: []Note{{Kind: Footnote, ID: "n", BodyLabelContent: []Run{{Text: "*", Bold: true}}, Blocks: []Block{p("Body", "note")}}}}
}
func TestColleafAnchorOriginsAndActualSave(t *testing.T) {
	ctx := context.Background()
	want := colleafAnchorOriginExample(t)
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
	if file := os.Getenv("MANYDOC_COLLEAF_ANCHOR_ORIGIN_QA"); file != "" {
		if err = os.WriteFile(file, before, 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, spec := range []struct{ env, mode string }{{"MANYDOC_COLLEAF_ANCHOR_ORIGIN_SAVED", "saved"}, {"MANYDOC_COLLEAF_ANCHOR_ORIGIN_EDITED", "edited"}, {"MANYDOC_COLLEAF_ANCHOR_ORIGIN_MOVED", "moved"}, {"MANYDOC_COLLEAF_ANCHOR_ORIGIN_RENAMED", "renamed"}, {"MANYDOC_COLLEAF_ANCHOR_ORIGIN_DETACHED", "detached"}} {
		t.Run(spec.mode, func(t *testing.T) {
			file := os.Getenv(spec.env)
			if file == "" {
				t.Skip("optional actual reader artifact")
			}
			raw, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			var native colleafNoteSnapshot
			if err = json.Unmarshal(raw, &native); err != nil {
				t.Fatal(err)
			}
			envelope, _ := json.Marshal(colleafEnvelope{colleafEnvelopeType, 3, "univer", colleafEngineVersion, native})
			actual, err := readColleaf(ctx, bytes.NewReader(envelope))
			if err != nil {
				t.Fatal(err)
			}
			expected := colleafAnchorOriginExample(t)
			switch spec.mode {
			case "edited":
				expected.Blocks[0].Runs = append([]Run{{Text: "Q"}}, expected.Blocks[0].Runs...)
				expected.Blocks[0].Runs[2].Source.Historical = true
				expected.Blocks[0].Runs[4].Source.Historical = true
			case "moved":
				middle, tail := expected.Blocks[0].Runs[1], expected.Blocks[0].Runs[3]
				middle.Source.Historical = true
				tail.Source.Historical = true
				expected.Blocks[0].Runs = []Run{expected.Blocks[0].Runs[0], middle, {Text: " "}, tail, {Text: "tail"}}
			case "renamed":
				expected.Blocks[0].Runs[3].Anchor = "tail-renamed"
				expected.Blocks[0].Runs[1].Source.Historical = true
				expected.Blocks[0].Runs[3].Source.Historical = true
			case "detached":
				expected.UnboundSources = []SourceInfo{*cloneSourceInfo(expected.Blocks[0].Runs[1].Source)}
				expected.Blocks[0].Runs[1].Source = nil
			}
			if spec.mode == "edited" || spec.mode == "moved" || spec.mode == "renamed" {
				expected.Diagnostics = []Diagnostic{{Code: "colleaf_anchor_source_historical", Message: "Original anchor source facts are retained as historical after current point identity or paragraph context changed"}}
			}
			if spec.mode == "detached" {
				expected.Diagnostics = []Diagnostic{{Code: "colleaf_anchor_source_unbound", Message: "Original anchor source facts are retained unbound after their native point ownership disappeared"}}
			}
			assertEvidenceSF(t, expected, actual)
			sf, err := ToStructured(actual)
			if err != nil {
				t.Fatal(err)
			}
			rawSF, _ := json.Marshal(sf)
			if err = os.WriteFile(file+".sf", rawSF, 0600); err != nil {
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
		})
	}
}
func TestColleafAnchorOriginsMalformed(t *testing.T) {
	ctx := context.Background()
	wire, err := encodeColleafDocument(ctx, colleafAnchorOriginExample(t))
	if err != nil {
		t.Fatal(err)
	}
	opened, err := openColleafIntent(ctx, wire)
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
				copy.Evidence.AnchorOrigins[0].Version = 2
			case "digest":
				copy.Evidence.AnchorOrigins[0].Source.Element = "changed"
			case "duplicate":
				copy.Evidence.AnchorOrigins = append(copy.Evidence.AnchorOrigins, copy.Evidence.AnchorOrigins[0])
			case "missing":
				copy.Evidence.AnchorOrigins = nil
			}
			candidate := opened
			candidate.Resources = colleafReplaceResource(candidate.Resources, colleafDocumentResource, copy)
			if _, err = decodeColleafDocument(ctx, candidate); err == nil {
				t.Fatal("malformed anchor source accepted")
			}
		})
	}
	for i, a := range wire.Body.CustomRanges {
		if a.Type != 3 {
			continue
		}
		p, err := readColleafBookmark(a)
		if err != nil {
			t.Fatal(err)
		}
		if p.Origin == nil {
			continue
		}
		p.Origin.Fingerprint = strings.Repeat("0", 64)
		wire.Body.CustomRanges[i].Properties = sfAttrs(p)
		break
	}
	if _, err = decodeColleafDocument(ctx, wire); err == nil {
		t.Fatal("contradictory inline anchor source accepted")
	}
}
func FuzzColleafAnchorOrigins(f *testing.F) {
	f.Add([]byte(`{"bookmarkId":"point"}`))
	f.Add([]byte(`{"bookmarkId":"point","manydocOrigin":{"version":2}}`))
	f.Add([]byte(sfAttrs(colleafBookmarkProperties{BookmarkID: "point", Origin: &colleafOrigin{Profile: colleafAnchorOriginProfile, Version: 1, Ref: strings.Repeat("0", 64)}})))
	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) > 32768 {
			return
		}
		snapshot := colleafNoteSnapshot{Body: colleafBody{CustomRanges: []colleafRangeRecord{{Type: 3, Properties: raw}}}}
		_ = visitColleafAnchorOrigins(&snapshot, false, func(*colleafOrigin) error { return nil })
	})
}

func TestColleafAnchorOriginsChangedOwnerAndFormatting(t *testing.T) {
	ctx := context.Background()
	wire, err := encodeColleafDocument(ctx, colleafAnchorOriginExample(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, missing := range []bool{false, true} {
		t.Run("changed-owner-"+map[bool]string{false: "valid", true: "missing"}[missing], func(t *testing.T) {
			raw, _ := json.Marshal(wire)
			var candidate colleafNoteSnapshot
			_ = json.Unmarshal(raw, &candidate)
			for i := range candidate.Body.CustomRanges {
				a := &candidate.Body.CustomRanges[i]
				if a.Type != 3 {
					continue
				}
				p, err := readColleafBookmark(*a)
				if err != nil {
					t.Fatal(err)
				}
				if p.BookmarkID != "middle" {
					continue
				}
				a.StartIndex = 0
				a.EndIndex = 0
				p.Hint.Role = "block"
				if missing {
					p.Origin.Ref = strings.Repeat("0", 64)
				}
				a.Properties = sfAttrs(p)
			}
			got, err := decodeColleafDocument(ctx, candidate)
			if missing {
				if err == nil {
					t.Fatal("invalid unconsumed reference accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(got.UnboundSources) != 1 || got.UnboundSources[0].Element != "middle" || len(got.Blocks[0].Anchors) != 1 || got.Blocks[0].Anchors[0] != "middle" {
				t.Fatal("changed owner incorrectly asserted inline source")
			}
			next, err := encodeColleafDocument(ctx, got)
			if err != nil {
				t.Fatal(err)
			}
			stable, err := decodeColleafDocument(ctx, next)
			if err != nil {
				t.Fatal(err)
			}
			assertEvidenceSF(t, got, stable)
		})
	}
	d := colleafAnchorOriginExample(t)
	d.Blocks[0].Runs[1].Bold = true
	if _, err = encodeColleafDocument(ctx, d); err == nil {
		t.Fatal("source support admitted formatted anchor")
	}
}
