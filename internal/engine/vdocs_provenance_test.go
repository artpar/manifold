package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func vdocsAnchorOriginExample(t *testing.T) *Document {
	base := vdocsOriginExample(t)
	point := func(id string) Run {
		s := cloneSourceInfo(base.Blocks[0].Source)
		s.Element = id
		s.Role = "Reference"
		return Run{Anchor: id, Source: s}
	}
	p := func(text, id string) Block { return Block{Kind: Paragraph, Runs: []Run{{Text: text}, point(id)}} }
	return &Document{Pages: base.Pages, Metadata: Metadata{Title: "Anchor origins"}, Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "😀A", Bold: true, FontSize: 13}, point("middle"), {Text: " tail"}, point("tail")}}, {Kind: Paragraph, Runs: []Run{{Text: "Jump", Destination: "middle"}}}, {Kind: ListBlock, List: "bullet", Items: []ListItem{{Blocks: []Block{p("Item", "item")}}}}, {Kind: Table, Rows: []Row{{Cells: []Cell{{Blocks: []Block{p("Cell", "cell")}}}}}}, {Kind: Paragraph, Runs: []Run{{Note: &NoteReference{Kind: Footnote, ID: "n"}}}}}, Headers: []Block{p("Header", "header")}, Footers: []Block{p("Footer", "footer")}, Notes: []Note{{Kind: Footnote, ID: "n", BodyLabelContent: []Run{{Text: "*", Bold: true}}, Blocks: []Block{p("Body", "note")}}}}
}
func TestVDOCSAnchorOriginsAndActualSave(t *testing.T) {
	ctx := context.Background()
	want := vdocsAnchorOriginExample(t)
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
		t.Fatal("native source mutated")
	}
	assertEvidenceSF(t, want, got)
	if file := os.Getenv("MANYDOC_VDOCS_ANCHOR_ORIGIN_QA"); file != "" {
		if err = os.WriteFile(file, before, 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, spec := range []struct{ env, mode string }{{"MANYDOC_VDOCS_ANCHOR_ORIGIN_SAVED", "saved"}, {"MANYDOC_VDOCS_ANCHOR_ORIGIN_EDITED", "edited"}, {"MANYDOC_VDOCS_ANCHOR_ORIGIN_MOVED", "moved"}, {"MANYDOC_VDOCS_ANCHOR_ORIGIN_RENAMED", "renamed"}, {"MANYDOC_VDOCS_ANCHOR_ORIGIN_DETACHED", "detached"}} {
		t.Run(spec.mode, func(t *testing.T) {
			file := os.Getenv(spec.env)
			if file == "" {
				t.Skip("optional actual reader artifact")
			}
			raw, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			var native vdocsNoteSnapshot
			if err = json.Unmarshal(raw, &native); err != nil {
				t.Fatal(err)
			}
			envelope, _ := json.Marshal(vdocsEnvelope{vdocsEnvelopeType, 3, "univer", vdocsEngineVersion, native})
			actual, err := readVDOCS(ctx, bytes.NewReader(envelope))
			if err != nil {
				t.Fatal(err)
			}
			expected := vdocsAnchorOriginExample(t)
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
				expected.Diagnostics = []Diagnostic{{Code: "vdocs_anchor_source_historical", Message: "Original anchor source facts are retained as historical after current point identity or paragraph context changed"}}
			}
			if spec.mode == "detached" {
				expected.Diagnostics = []Diagnostic{{Code: "vdocs_anchor_source_unbound", Message: "Original anchor source facts are retained unbound after their native point ownership disappeared"}}
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
			next, err := encodeVDOCSDocument(ctx, model)
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
func TestVDOCSAnchorOriginsMalformed(t *testing.T) {
	ctx := context.Background()
	wire, err := encodeVDOCSDocument(ctx, vdocsAnchorOriginExample(t))
	if err != nil {
		t.Fatal(err)
	}
	opened, err := openVDOCSIntent(ctx, wire)
	if err != nil {
		t.Fatal(err)
	}
	var intent vdocsDocumentIntent
	if err = vdocsResourceMap(opened.Resources, vdocsDocumentResource, &intent); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"version", "digest", "duplicate", "missing"} {
		t.Run(mode, func(t *testing.T) {
			raw, _ := json.Marshal(intent)
			var copy vdocsDocumentIntent
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
			candidate.Resources = vdocsReplaceResource(candidate.Resources, vdocsDocumentResource, copy)
			if _, err = decodeVDOCSDocument(ctx, candidate); err == nil {
				t.Fatal("malformed anchor source accepted")
			}
		})
	}
	for i, a := range wire.Body.CustomRanges {
		if a.Type != 3 {
			continue
		}
		p, err := readVDOCSBookmark(a)
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
	if _, err = decodeVDOCSDocument(ctx, wire); err == nil {
		t.Fatal("contradictory inline anchor source accepted")
	}
}
func FuzzVDOCSAnchorOrigins(f *testing.F) {
	f.Add([]byte(`{"bookmarkId":"point"}`))
	f.Add([]byte(`{"bookmarkId":"point","manydocOrigin":{"version":2}}`))
	f.Add([]byte(sfAttrs(vdocsBookmarkProperties{BookmarkID: "point", Origin: &vdocsOrigin{Profile: vdocsAnchorOriginProfile, Version: 1, Ref: strings.Repeat("0", 64)}})))
	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) > 32768 {
			return
		}
		snapshot := vdocsNoteSnapshot{Body: vdocsBody{CustomRanges: []vdocsRangeRecord{{Type: 3, Properties: raw}}}}
		_ = visitVDOCSAnchorOrigins(&snapshot, false, func(*vdocsOrigin) error { return nil })
	})
}

func TestVDOCSAnchorOriginsChangedOwnerAndFormatting(t *testing.T) {
	ctx := context.Background()
	wire, err := encodeVDOCSDocument(ctx, vdocsAnchorOriginExample(t))
	if err != nil {
		t.Fatal(

			// A native range ID is also a valid bookmark alias, even when its primary
			// bookmarkId differs. Normalize the relationship, not the visible text.

			// Source-bearing empty text without a point identity is still explicitly rejected.

			// Exercise references to point groups and terminal/empty native cursors.

			// Declaring an unrelated empty paragraph as the carrier cannot remove it.

			// The expected projection is authored independently of the native cursor walker.

			// Diagnostics follow interpretation order; compare their exact set independently.

			// This new document represents the disclosed normalized semantics, including
			// its original diagnostic history through checked native intent.

			err)
	}
	for _, missing := range []bool{false, true} {
		t.Run("changed-owner-"+map[bool]string{false: "valid", true: "missing"}[missing], func(t *testing.T) {
			raw, _ := json.Marshal(wire)
			var candidate vdocsNoteSnapshot
			_ = json.Unmarshal(raw, &candidate)
			for i := range candidate.Body.CustomRanges {
				a := &candidate.Body.CustomRanges[i]
				if a.Type != 3 {
					continue
				}
				p, err := readVDOCSBookmark(*a)
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
			got, err := decodeVDOCSDocument(ctx, candidate)
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
			next, err := encodeVDOCSDocument(ctx, got)
			if err != nil {
				t.Fatal(err)
			}
			stable, err := decodeVDOCSDocument(ctx, next)
			if err != nil {
				t.Fatal(err)
			}
			assertEvidenceSF(t, got, stable)
		})
	}
	d := vdocsAnchorOriginExample(t)
	d.Blocks[0].Runs[1].Bold = true
	if _, err = encodeVDOCSDocument(ctx, d); err == nil {
		t.Fatal("source support admitted formatted anchor")
	}
}
func vdocsFlowOriginExample(t *testing.T) *Document {
	base := vdocsOriginExample(t)
	src := func(id, role string) *SourceInfo {
		s := cloneSourceInfo(base.Blocks[0].Source)
		s.Element = id
		s.Role = role
		return s
	}
	p := func(text string) Block { return Block{Kind: Paragraph, Runs: []Run{{Text: text}}} }
	point := func(id string, aliases ...string) Block {
		return Block{Kind: AnchorPoint, Anchors: append([]string{id}, aliases...), Source: src(id, "Reference")}
	}
	return &Document{Metadata: Metadata{Title: "Flow origins"}, Pages: base.Pages, Layout: Layout{Width: 450, Height: 600, Margin: 40, HeaderDistance: vdocsFloat(10), FooterDistance: vdocsFloat(10)}, Blocks: []Block{
		{Kind: SectionBlock, Source: src("section-one", "Sect"), Section: &Section{Headers: RunningStories{Default: &RunningStory{Blocks: []Block{point("header"), p("Header")}}}, Footers: RunningStories{Default: &RunningStory{Blocks: []Block{p("Footer"), point("footer")}}}, Blocks: []Block{point("start", "alias"), {Kind: Paragraph, Runs: []Run{{Text: "😀 First", Bold: true, FontSize: 13}}}, {Kind: PageBreak, Anchors: []string{"break"}, Source: src("hard-break", "Artifact")}, {Kind: Paragraph, Runs: []Run{{Text: "After"}, {Note: &NoteReference{Kind: Footnote, ID: "n"}}}}, point("end")}}},
		{Kind: SectionBlock, Source: src("section-two", "Sect"), Section: &Section{Start: SectionNextPage, Layout: &Layout{Width: 360, Height: 500, Margin: 30}, Blocks: []Block{p("Second"), point("last")}}},
	}, Notes: []Note{{Kind: Footnote, ID: "n", Blocks: []Block{point("note-start"), p("Note")}}}}
}
func TestVDOCSFlowOriginsAndActualSave(t *testing.T) {
	ctx := context.Background()
	want := vdocsFlowOriginExample(t)
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
		t.Fatal("native input mutated")
	}
	assertEvidenceSF(t, want, got)
	if file := os.Getenv("MANYDOC_VDOCS_FLOW_ORIGIN_QA"); file != "" {
		if err = os.WriteFile(file, before, 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, spec := range []struct{ env, mode string }{{"MANYDOC_VDOCS_FLOW_ORIGIN_SAVED", "saved"}, {"MANYDOC_VDOCS_FLOW_ORIGIN_EDITED", "edited"}, {"MANYDOC_VDOCS_FLOW_ORIGIN_RENAMED", "renamed"}, {"MANYDOC_VDOCS_FLOW_ORIGIN_DETACHED", "detached"}, {"MANYDOC_VDOCS_FLOW_ORIGIN_SOFT", "soft"}} {
		t.Run(spec.mode, func(t *testing.T) {
			file := os.Getenv(spec.env)
			if file == "" {
				t.Skip("optional actual reader artifact")
			}
			raw, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			var native vdocsNoteSnapshot
			if err = json.Unmarshal(raw, &native); err != nil {
				t.Fatal(err)
			}
			envelope, _ := json.Marshal(vdocsEnvelope{vdocsEnvelopeType, 3, "univer", vdocsEngineVersion, native})
			actual, err := readVDOCS(ctx, bytes.NewReader(envelope))
			if err != nil {
				t.Fatal(err)
			}
			expected := vdocsFlowOriginExample(t)
			switch spec.mode {
			case "edited":
				expected.Blocks[0].Source.Historical = true
				b := &expected.Blocks[0].Section.Blocks[1]
				first := b.Runs[0]
				first.Text = "😀 "
				last := b.Runs[0]
				last.Text = "First"
				b.Runs = []Run{first, {Text: "Q"}, last}
				expected.Diagnostics = []Diagnostic{{Code: "vdocs_flow_source_historical", Message: "Original section, break or point source facts are retained as historical after current semantics changed"}}
			case "renamed":
				expected.Blocks[0].Source.Historical = true
				expected.Blocks[0].Section.Blocks[0].Anchors[1] = "alias-renamed"
				expected.Blocks[0].Section.Blocks[0].Source.Historical = true
				expected.Diagnostics = []Diagnostic{{Code: "vdocs_flow_source_historical", Message: "Original section, break or point source facts are retained as historical after current semantics changed"}}
			case "detached":
				expected.UnboundSources = []SourceInfo{*cloneSourceInfo(expected.Blocks[0].Source)}
				expected.Blocks[0].Source = nil
				expected.Diagnostics = []Diagnostic{{Code: "vdocs_flow_source_unbound", Message: "Original section, break or point source facts are retained unbound after native ownership disappeared"}}
			case "soft":
				expected.Blocks[0].Source.Historical = true
				expected.UnboundSources = []SourceInfo{*cloneSourceInfo(expected.Blocks[0].Section.Blocks[2].Source)}
				expected.Blocks[0].Section.Blocks[2] = Block{Kind: AnchorPoint, Anchors: []string{"break"}}
				expected.Diagnostics = []Diagnostic{{Code: "vdocs_soft_page_break", Message: "Native rendered soft page boundary omitted from logical flow; literal content is retained"}, {Code: "vdocs_flow_source_historical", Message: "Original section, break or point source facts are retained as historical after current semantics changed"}, {Code: "vdocs_flow_source_unbound", Message: "Original section, break or point source facts are retained unbound after native ownership disappeared"}}
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
			model, err := FromStructured(sf)
			if err != nil {
				t.Fatal(err)
			}
			next, err := encodeVDOCSDocument(ctx, model)
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
func TestVDOCSFlowOriginsMalformed(t *testing.T) {
	ctx := context.Background()
	wire, err := encodeVDOCSDocument(ctx, vdocsFlowOriginExample(t))
	if err != nil {
		t.Fatal(err)
	}
	opened, err := openVDOCSIntent(ctx, wire)
	if err != nil {
		t.Fatal(err)
	}
	var intent vdocsDocumentIntent
	if err = vdocsResourceMap(opened.Resources, vdocsDocumentResource, &intent); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"version", "digest", "duplicate", "missing"} {
		t.Run(mode, func(t *testing.T) {
			raw, _ := json.Marshal(intent)
			var copy vdocsDocumentIntent
			_ = json.Unmarshal(raw, &copy)
			switch mode {
			case "version":
				copy.Evidence.FlowOrigins[0].Version = 2
			case "digest":
				copy.Evidence.FlowOrigins[0].Source.Element = "changed"
			case "duplicate":
				copy.Evidence.FlowOrigins = append(copy.Evidence.FlowOrigins, copy.Evidence.FlowOrigins[0])
			case "missing":
				copy.Evidence.FlowOrigins = nil
			}
			candidate := opened
			candidate.Resources = vdocsReplaceResource(candidate.Resources, vdocsDocumentResource, copy)
			if _, err = decodeVDOCSDocument(ctx, candidate); err == nil {
				t.Fatal("malformed flow source accepted")
			}
		})
	}
}
func FuzzVDOCSFlowOrigins(f *testing.F) {
	f.Add([]byte(`{"manydocFlowOrigin":{"version":2}}`))
	f.Add([]byte(sfAttrs(map[string]any{"manydocFlowOrigin": vdocsOrigin{Profile: vdocsFlowOriginProfile, Version: 1, Ref: strings.Repeat("0", 64)}})))
	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) > 32768 {
			return
		}
		snapshot := vdocsNoteSnapshot{Body: vdocsBody{Paragraphs: []vdocsParagraphRecord{{Style: raw}}}}
		_ = visitVDOCSFlowOrigins(&snapshot, false, func(*vdocsOrigin) error { return nil })
	})
}

func TestStructuredIdentitylessEmptySourcePolicy(t *testing.T) {
	base := vdocsOriginExample(t)
	source := cloneSourceInfo(base.Blocks[0].Source)
	d := &Document{Pages: base.Pages, Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Source: source}}}}}
	before, _ := json.Marshal(d)
	if _, err := ToStructured(d); !errors.Is(err, ErrUnsupported) {
		t.Fatal("empty text silently discarded source facts", err)
	}
	after, _ := json.Marshal(d)
	if !bytes.Equal(before, after) {
		t.Fatal("failed conversion mutated input")
	}
	d.Blocks[0].Runs = nil
	d.UnboundSources = []SourceInfo{*source}
	sf, err := ToStructured(d)
	if err != nil {
		t.Fatal(err)
	}
	got, err := FromStructured(sf)
	if err != nil {
		t.Fatal(err)
	}
	assertEvidenceSF(t, d, got)
}
func vdocsImageOriginExample(t *testing.T) *Document {
	d := vdocsImageExample(t)
	base := vdocsOriginExample(t)
	d.Pages = base.Pages
	d.Metadata.Title = "Image origins"
	src := func(id string) *SourceInfo {
		s := cloneSourceInfo(base.Blocks[0].Source)
		s.Element = id
		s.Role = "Figure"
		return s
	}
	d.Blocks[0].Source = src("main-image")
	d.Blocks[0].Image.Width = 24
	d.Blocks[0].Image.Height = 12
	d.Blocks[2].Rows[0].Cells[0].Blocks[1].Source = src("cell-image")
	d.Blocks[3].Items[0].Blocks[1].Source = src("list-image")
	d.Blocks[4].Source = src("jpeg-image")
	d.Notes[0].Blocks[0].Source = src("note-image")
	d.Notes[0].Source = cloneSourceInfo(base.Blocks[0].Source)
	d.Notes[0].Source.Element = "note-definition"
	d.Headers = []Block{{Kind: Picture, Image: &Image{Data: runningPNG(t, false), MediaType: "image/png", Alt: "Header PNG", Width: 7, Height: 9}, Source: src("header-image")}}
	d.Footers = []Block{{Kind: Picture, Image: &Image{Data: runningPNG(t, false), MediaType: "image/png", Alt: "Footer PNG", Width: 7, Height: 9}, Source: src("footer-image")}}
	d.Blocks = append([]Block{{Kind: Paragraph, Runs: []Run{{Text: "😀 Image origins", Bold: true, FontSize: 13}}}}, d.Blocks...)
	return d
}
func TestVDOCSImageOriginsAndActualSave(t *testing.T) {
	ctx := context.Background()
	want := vdocsImageOriginExample(t)
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
		t.Fatal("source snapshot mutated")
	}
	assertEvidenceSF(t, want, got)
	if file := os.Getenv("MANYDOC_VDOCS_IMAGE_ORIGIN_QA"); file != "" {
		if err = os.WriteFile(file, before, 0600); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(file+".replacement.png", runningPNG(t, true), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, spec := range []struct{ env, mode string }{{"MANYDOC_VDOCS_IMAGE_ORIGIN_SAVED", "saved"}, {"MANYDOC_VDOCS_IMAGE_ORIGIN_RESIZED", "resized"}, {"MANYDOC_VDOCS_IMAGE_ORIGIN_ASSET", "asset"}, {"MANYDOC_VDOCS_IMAGE_ORIGIN_DETACHED", "detached"}} {
		t.Run(spec.mode, func(t *testing.T) {
			file := os.Getenv(spec.env)
			if file == "" {
				t.Skip("optional actual reader artifact")
			}
			raw, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			var native vdocsNoteSnapshot
			if err = json.Unmarshal(raw, &native); err != nil {
				t.Fatal(err)
			}
			envelope, _ := json.Marshal(vdocsEnvelope{vdocsEnvelopeType, 3, "univer", vdocsEngineVersion, native})
			actual, err := readVDOCS(ctx, bytes.NewReader(envelope))
			if err != nil {
				t.Fatal(err)
			}
			expected := vdocsImageOriginExample(t)
			switch spec.mode {
			case "resized", "asset":
				expected.Blocks[1].Source.Historical = true
				if spec.mode == "resized" {
					expected.Blocks[1].Image.Width = 48
					expected.Blocks[1].Image.Height = 24
					expected.Blocks[1].Image.Alt = "Resized PNG"
				} else {
					expected.Blocks[1].Image.Data = runningPNG(t, true)
				}
				expected.Diagnostics = []Diagnostic{{Code: "vdocs_stale_image_geometry", Message: "Stale image size intent ignored; actual native asset and resized geometry are retained"}, {Code: "vdocs_image_source_historical", Message: "Original image source facts are retained as historical after current asset, description or presentation changed"}}
			case "detached":
				expected.UnboundSources = []SourceInfo{*cloneSourceInfo(expected.Blocks[1].Source)}
				expected.Blocks[1].Source = nil
				expected.Diagnostics = []Diagnostic{{Code: "vdocs_image_source_unbound", Message: "Original image source facts are retained unbound after their native ownership disappeared"}}
			}

			if spec.mode == "saved" {
				for j, resource := range native.Resources {
					if resource.Name != "DOC_DRAWING_PLUGIN" {
						continue
					}
					var mirror struct {
						Data  map[string]json.RawMessage `json:"data"`
						Order []string                   `json:"order"`
					}
					if err = json.Unmarshal([]byte(resource.Data), &mirror); err != nil {
						t.Fatal(err)
					}
					id := native.Body.CustomBlocks[0].ID
					props, err := vdocsStyleObject(mirror.Data[id])
					if err != nil {
						t.Fatal(err)
					}
					origin, err := readVDOCSOriginProfile(props["manydocOrigin"], vdocsImageOriginProfile)
					if err != nil {
						t.Fatal(err)
					}
					origin.Ref = strings.Repeat("0", 64)
					props["manydocOrigin"] = sfAttrs(origin)
					mirror.Data[id] = sfAttrs(props)
					candidate := native
					candidate.Resources = append([]vdocsResource(nil), native.Resources...)
					candidate.Resources[j].Data = string(sfAttrs(mirror))
					if _, err = decodeVDOCSDocument(ctx, candidate); err == nil || !strings.Contains(err.Error(), "contradictory native drawing resource/source") {
						t.Fatal("mirror source-binding disagreement was not rejected", err)
					}
				}
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
			model, err := FromStructured(sf)
			if err != nil {
				t.Fatal(err)
			}
			next, err := encodeVDOCSDocument(ctx, model)
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
func TestVDOCSImageOriginsFittedSectionsAndMalformed(t *testing.T) {
	ctx := context.Background()
	d := vdocsFittedImageExample(t)
	base := vdocsOriginExample(t)
	d.Pages = base.Pages
	var walk func([]Block)
	serial := 0
	walk = func(bs []Block) {
		for i := range bs {
			b := &bs[i]
			if b.Kind == Picture {
				serial++
				b.Source = cloneSourceInfo(base.Blocks[0].Source)
				b.Source.Element = "fitted-" + strconv.Itoa(serial)
				b.Source.Role = "Figure"
			}
			for j := range b.Rows {
				for k := range b.Rows[j].Cells {
					walk(b.Rows[j].Cells[k].Blocks)
				}
			}
			for j := range b.Items {
				walk(b.Items[j].Blocks)
			}
			if b.Section != nil {
				walk(b.Section.Blocks)
				for _, s := range sectionStories(b.Section) {
					if s != nil {
						walk(s.Blocks)
					}
				}
			}
		}
	}
	walk(d.Blocks)
	for i := range d.Notes {
		walk(d.Notes[i].Blocks)
	}
	wire, err := encodeVDOCSDocument(ctx, d)
	if err != nil {
		t.Fatal(err)
	}
	got, err := decodeVDOCSDocument(ctx, wire)
	if err != nil {
		t.Fatal(err)
	}
	assertEvidenceSF(t, d, got)
	native, err := encodeVDOCSDocument(ctx, vdocsImageOriginExample(t))
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
	for _, mode := range []string{"version", "digest", "duplicate", "missing"} {
		t.Run(mode, func(t *testing.T) {
			raw, _ := json.Marshal(intent)
			var copy vdocsDocumentIntent
			_ = json.Unmarshal(raw, &copy)
			switch mode {
			case "version":
				copy.Evidence.ImageOrigins[0].Version = 2
			case "digest":
				copy.Evidence.ImageOrigins[0].Source.Element = "changed"
			case "duplicate":
				copy.Evidence.ImageOrigins = append(copy.Evidence.ImageOrigins, copy.Evidence.ImageOrigins[0])
			case "missing":
				copy.Evidence.ImageOrigins = nil
			}
			candidate := opened
			candidate.Resources = vdocsReplaceResource(candidate.Resources, vdocsDocumentResource, copy)
			if _, err = decodeVDOCSDocument(ctx, candidate); err == nil {
				t.Fatal("malformed image source accepted")
			}
		})
	}
}
func FuzzVDOCSImageOrigins(f *testing.F) {
	f.Add([]byte(`{"drawingId":"image"}`))
	f.Add([]byte(sfAttrs(map[string]any{"manydocOrigin": vdocsOrigin{Profile: vdocsImageOriginProfile, Version: 1, Ref: strings.Repeat("0", 64)}})))
	f.Add([]byte(`{"manydocOrigin":{"profile":"manydoc.image-origin","version":2}}`))
	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) > 32768 {
			return
		}
		snapshot := vdocsNoteSnapshot{Drawings: map[string]json.RawMessage{"image": raw}}
		_ = visitVDOCSImageOrigins(&snapshot, false, func(*vdocsOrigin) error { return nil })
	})
}
func vdocsListOriginExample(t *testing.T) *Document {
	d := vdocsListExample()
	base := vdocsOriginExample(t)
	d.Pages = base.Pages
	d.Metadata.Title = "List origins"
	p := func(text string) Block { return Block{Kind: Paragraph, Runs: []Run{{Text: text}}} }
	small := func(text string) Block {
		return Block{Kind: ListBlock, List: "bullet", Items: []ListItem{{Blocks: []Block{p(text)}}}}
	}
	d.Headers = []Block{small("Header item")}
	d.Footers = []Block{small("Footer item")}
	d.Blocks[0].Items[0].Blocks[3].Rows[0].Cells[0].Blocks = append(d.Blocks[0].Items[0].Blocks[3].Rows[0].Cells[0].Blocks, small("Cell item"))
	serial := 0
	src := func(role string) *SourceInfo {
		serial++
		s := cloneSourceInfo(base.Blocks[0].Source)
		s.Element = "origin-" + strconv.Itoa(serial)
		s.Role = role
		return s
	}
	var walk func([]Block)
	walk = func(bs []Block) {
		for i := range bs {
			b := &bs[i]
			if b.Kind == ListBlock {
				b.Source = src("L")
				for j := range b.Items {
					b.Items[j].Source = src("LI")
				}
			}
			for j := range b.Items {
				walk(b.Items[j].Blocks)
			}
			for j := range b.Rows {
				for k := range b.Rows[j].Cells {
					walk(b.Rows[j].Cells[k].Blocks)
				}
			}
		}
	}
	walk(d.Blocks)
	walk(d.Headers)
	walk(d.Footers)
	for i := range d.Notes {
		walk(d.Notes[i].Blocks)
	}
	d.Blocks[0].Source.Element = "outer-list"
	d.Blocks[0].Items[0].Source.Element = "outer-item"
	d.Blocks[1].Source.Element = "restart-list"
	d.Blocks[1].Items[0].Source.Element = "restart-item-1"
	d.Blocks[1].Items[1].Source.Element = "restart-item-2"
	d.Blocks = append([]Block{{Kind: Paragraph, Runs: []Run{{Text: "😀 List origins", Bold: true, FontSize: 13}}}}, d.Blocks...)
	return d
}
func TestVDOCSListOriginsAndActualSave(t *testing.T) {
	ctx := context.Background()
	want := vdocsListOriginExample(t)
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
		t.Fatal("native input mutated")
	}
	assertEvidenceSF(t, want, got)
	if file := os.Getenv("MANYDOC_VDOCS_LIST_ORIGIN_QA"); file != "" {
		if err = os.WriteFile(file, before, 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, spec := range []struct{ env, mode string }{{"MANYDOC_VDOCS_LIST_ORIGIN_SAVED", "saved"}, {"MANYDOC_VDOCS_LIST_ORIGIN_EDITED", "edited"}, {"MANYDOC_VDOCS_LIST_ORIGIN_RESTARTED", "restarted"}, {"MANYDOC_VDOCS_LIST_ORIGIN_DETACHED", "detached"}, {"MANYDOC_VDOCS_LIST_ORIGIN_ITEM_DETACHED", "item-detached"}} {
		t.Run(spec.mode, func(t *testing.T) {
			file := os.Getenv(spec.env)
			if file == "" {
				t.Skip("optional actual reader artifact")
			}
			raw, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			var native vdocsNoteSnapshot
			if err = json.Unmarshal(raw, &native); err != nil {
				t.Fatal(err)
			}
			envelope, _ := json.Marshal(vdocsEnvelope{vdocsEnvelopeType, 3, "univer", vdocsEngineVersion, native})
			actual, err := readVDOCS(ctx, bytes.NewReader(envelope))
			if err != nil {
				t.Fatal(err)
			}
			expected := vdocsListOriginExample(t)
			switch spec.mode {
			case "edited":
				expected.Blocks[1].Source.Historical = true
				expected.Blocks[1].Items[0].Source.Historical = true
				expected.Blocks[1].Items[0].Blocks[0].Runs = []Run{{Text: "Q"}, {Text: "Parent one"}}
				expected.Diagnostics = []Diagnostic{{Code: "vdocs_list_source_historical", Message: "Original list or item source facts are retained as historical after current content, numbering or ownership changed"}}
			case "restarted":
				expected.Blocks[2].Start = listStartPointer(9)
				expected.Blocks[2].Source.Historical = true
				for i := range expected.Blocks[2].Items {
					expected.Blocks[2].Items[i].Source.Historical = true
				}
				expected.Diagnostics = []Diagnostic{{Code: "vdocs_list_source_historical", Message: "Original list or item source facts are retained as historical after current content, numbering or ownership changed"}}
			case "detached":
				expected.UnboundSources = []SourceInfo{*cloneSourceInfo(expected.Blocks[1].Source)}
				expected.Blocks[1].Source = nil
				expected.Diagnostics = []Diagnostic{{Code: "vdocs_list_source_unbound", Message: "Original list or item source facts are retained unbound after their native list ownership disappeared"}}
			case "item-detached":
				expected.UnboundSources = []SourceInfo{*cloneSourceInfo(expected.Blocks[1].Items[0].Source)}
				expected.Blocks[1].Items[0].Source = nil
				expected.Diagnostics = []Diagnostic{{Code: "vdocs_list_source_unbound", Message: "Original list or item source facts are retained unbound after their native list ownership disappeared"}}
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
			model, err := FromStructured(sf)
			if err != nil {
				t.Fatal(err)
			}
			next, err := encodeVDOCSDocument(ctx, model)
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
func TestVDOCSListOriginsMalformedAndSourceFreeContext(t *testing.T) {
	ctx := context.Background()
	d := vdocsListOriginExample(t)
	wire, err := encodeVDOCSDocument(ctx, d)
	if err != nil {
		t.Fatal(err)
	}
	opened, err := openVDOCSIntent(ctx, wire)
	if err != nil {
		t.Fatal(err)
	}
	var intent vdocsDocumentIntent
	if err = vdocsResourceMap(opened.Resources, vdocsDocumentResource, &intent); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"version", "digest", "duplicate", "missing"} {
		t.Run(mode, func(t *testing.T) {
			raw, _ := json.Marshal(intent)
			var copy vdocsDocumentIntent
			_ = json.Unmarshal(raw, &copy)
			switch mode {
			case "version":
				copy.Evidence.ListOrigins[0].Version = 2
			case "digest":
				copy.Evidence.ListOrigins[0].Source.Element = "changed"
			case "duplicate":
				copy.Evidence.ListOrigins = append(copy.Evidence.ListOrigins, copy.Evidence.ListOrigins[0])
			case "missing":
				copy.Evidence.ListOrigins = nil
			}
			candidate := opened
			candidate.Resources = vdocsReplaceResource(candidate.Resources, vdocsDocumentResource, copy)
			if _, err = decodeVDOCSDocument(ctx, candidate); err == nil {
				t.Fatal("malformed list source accepted")
			}
		})
	}
	bullet := d.Headers[0]
	hash, err := vdocsItemFingerprint(bullet, 0)
	if err != nil {
		t.Fatal(err)
	}
	bullet.Source = nil
	bullet.Items = append([]ListItem(nil), bullet.Items...)
	bullet.Items[0].Source = nil
	clean, err := vdocsItemFingerprint(bullet, 0)
	if err != nil || hash != clean {
		t.Fatal("source-only edit changed item semantics", err)
	}
}
func FuzzVDOCSListOrigins(f *testing.F) {
	f.Add([]byte(`{"manydocListOrigin":{"profile":"manydoc.list-origin","version":2}}`))
	f.Add([]byte(sfAttrs(map[string]any{"manydocItemOrigin": vdocsOrigin{Profile: vdocsItemOriginProfile, Version: 1, Ref: strings.Repeat("0", 64)}})))
	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) > 32768 {
			return
		}
		snapshot := vdocsNoteSnapshot{Body: vdocsBody{Paragraphs: []vdocsParagraphRecord{{Style: raw}}}}
		_ = visitVDOCSListOrigins(&snapshot, false, func(*vdocsOrigin) error { return nil })
	})
}

func TestVDOCSListOriginsLostAndInheritedOwnership(t *testing.T) {
	ctx := context.Background()
	for _, mode := range []string{"bullet-removed", "inherited", "contradictory"} {
		t.Run(mode, func(t *testing.T) {
			source := vdocsListOriginExample(t)
			d := &Document{Pages: source.Pages, Blocks: []Block{source.Headers[0]}}
			wire, err := encodeVDOCSDocument(ctx, d)
			if err != nil {
				t.Fatal(err)
			}
			props, err := vdocsStyleObject(wire.Body.Paragraphs[0].Style)
			if err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "bullet-removed":
				wire.Body.Paragraphs[0].Bullet = nil
			case "inherited":
				marker := props["manydocListOrigin"]
				delete(props, "manydocListOrigin")
				wire.Body.Paragraphs[0].Style = sfAttrs(props)
				style, _ := vdocsStyleObject(wire.DocumentStyle)
				style["defaultParagraphStyle"] = sfAttrs(map[string]json.RawMessage{"manydocListOrigin": marker})
				wire.DocumentStyle = sfAttrs(style)
			case "contradictory":
				origin, err := readVDOCSOriginProfile(props["manydocListOrigin"], vdocsListOriginProfile)
				if err != nil {
					t.Fatal(err)
				}
				origin.Fingerprint = strings.Repeat("0", 64)
				props["manydocListOrigin"] = sfAttrs(origin)
				wire.Body.Paragraphs[0].Style = sfAttrs(props)
			}
			got, err := decodeVDOCSDocument(ctx, wire)
			if mode == "contradictory" {
				if err == nil {
					t.Fatal("contradictory list source accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if mode == "bullet-removed" {
				if len(got.UnboundSources) != 2 || got.Blocks[0].Kind != Paragraph || got.Blocks[0].Source != nil || got.Blocks[0].vdocsListOrigin != nil || got.Blocks[0].vdocsItemOrigin != nil {
					t.Fatal("lost list asserted current ownership")
				}
			}
			if mode == "inherited" {
				if len(got.UnboundSources) != 1 || got.Blocks[0].Source != nil || got.Blocks[0].Items[0].Source.Historical {
					t.Fatal("inherited list source acquired ownership or staled item")
				}
				found := false
				for _, diagnostic := range got.Diagnostics {
					found = found || diagnostic.Code == "vdocs_list_origin_inheritance"
				}
				if !found {
					t.Fatal("missing inherited ownership warning")
				}
			}
			sf, err := ToStructured(got)
			if err != nil {
				t.Fatal(err)
			}
			model, err := FromStructured(sf)
			if err != nil {
				t.Fatal(err)
			}
			next, err := encodeVDOCSDocument(ctx, model)
			if err != nil {
				t.Fatal(err)
			}
			stable, err := decodeVDOCSDocument(ctx, next)
			if err != nil {
				t.Fatal(err)
			}
			assertEvidenceSF(t, got, stable)
		})
	}
}
func vdocsNavigationExample(t testing.TB) *Document {
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
func TestVDOCSNavigationPointsSFAndActualSave(t *testing.T) {
	ctx := context.Background()
	want := vdocsNavigationExample(t)
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
	again, err := encodeVDOCSDocument(ctx, model)
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := decodeVDOCSDocument(ctx, again)
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
	if file := os.Getenv("MANYDOC_VDOCS_NAVIGATION_QA"); file != "" {
		raw, err := json.Marshal(wire)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(file, raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if file := os.Getenv("MANYDOC_VDOCS_NAVIGATION_HEADING"); file != "" {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var native vdocsNoteSnapshot
		if err = json.Unmarshal(raw, &native); err != nil {
			t.Fatal(err)
		}
		foreign, err := decodeVDOCSDocument(ctx, native)
		if err != nil {
			t.Fatal(err)
		}
		id := vdocsHeadingID("", "foreign-heading")
		if len(foreign.Blocks[1].Anchors) != 2 || foreign.Blocks[1].Anchors[0] != id || foreign.Blocks[5].Runs[4].Destination != id || foreign.Footers[0].Runs[0].Destination != id {
			t.Fatal("native heading target relationship lost")
		}
		found := false
		for _, diag := range foreign.Diagnostics {
			found = found || diag.Code == "vdocs_heading_navigation"
		}
		if !found {
			t.Fatal("heading namespace normalization not disclosed")
		}
	}
	for _, spec := range []struct {
		env    string
		edited bool
	}{{"MANYDOC_VDOCS_NAVIGATION_SAVED", false}, {"MANYDOC_VDOCS_NAVIGATION_EDITED", true}} {
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
		got, err = decodeVDOCSDocument(ctx, native)
		if err != nil {
			t.Fatal(spec.env, err)
		}
		after, _ := json.Marshal(native)
		if !bytes.Equal(before, after) {
			t.Fatal("navigation import mutated native snapshot")
		}
		envelope, err := json.Marshal(vdocsEnvelope{vdocsEnvelopeType, 3, "univer", vdocsEngineVersion, native})
		if err != nil {
			t.Fatal(err)
		}
		strict, err := readVDOCS(ctx, bytes.NewReader(envelope))
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
				found = found || diag.Code == "vdocs_stale_bookmark_placement"
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
		offset += vdocsUTF16Len(r.Text)
		if r.Note != nil {
			offset++
		}
	}
	return positions
}
func TestVDOCSForeignHeadingBookmarkAliasAndMissingTargets(t *testing.T) {
	ctx := context.Background()
	base := &Document{Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "Jump"}}}, {Kind: Heading, Level: 1, Runs: []Run{{Text: "😀 heading"}}}}}
	wire, err := encodeVDOCSDocument(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	props, _ := vdocsStyleObject(wire.Body.Paragraphs[1].Style)
	props["headingId"] = sfAttrs("foreign-heading")
	wire.Body.Paragraphs[1].Style = sfAttrs(props)
	wire.Body.CustomRanges = append(wire.Body.CustomRanges, vdocsRangeRecord{ID: "heading-link", Type: 0, StartIndex: 0, EndIndex: 3, Properties: sfAttrs(map[string]string{"headingId": "foreign-heading", "url": "#foreign-heading", "textStyleMode": "text"})})
	got, err := decodeVDOCSDocument(ctx, wire)
	if err != nil {
		t.Fatal(err)
	}
	id := vdocsHeadingID("", "foreign-heading")
	if got.Blocks[0].Runs[0].Destination != id || !reflect.DeepEqual(got.Blocks[1].Anchors, []string{id}) {
		t.Fatal("heading target did not resolve to its explicit point")
	}

	wire, err = encodeVDOCSDocument(ctx, vdocsNavigationExample(t))
	if err != nil {
		t.Fatal(err)
	}
	var rangeID string
	for _, r := range wire.Body.CustomRanges {
		if r.Type == 3 {
			p, _ := readVDOCSBookmark(r)
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
	got, err = decodeVDOCSDocument(ctx, wire)
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
	got, err = decodeVDOCSDocument(ctx, wire)
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
func TestVDOCSNavigationMalformedAndStructuralPoints(t *testing.T) {
	ctx := context.Background()
	wire, err := encodeVDOCSDocument(ctx, vdocsNavigationExample(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, offset := range []int{-1, 1, vdocsUTF16Len(wire.Body.DataStream)} {
		bad := wire
		bad.Body.CustomRanges = append([]vdocsRangeRecord(nil), wire.Body.CustomRanges...)
		bad.Body.CustomRanges = append(bad.Body.CustomRanges, vdocsRangeRecord{ID: "bad-point", Type: 3, StartIndex: offset, EndIndex: offset, Properties: sfAttrs(map[string]string{"bookmarkId": "bad"})})
		if _, err = decodeVDOCSDocument(ctx, bad); err == nil {
			t.Fatal("invalid/surrogate-split bookmark accepted", offset)
		}
	}
	bad := wire
	bad.Body.CustomRanges = append([]vdocsRangeRecord(nil), wire.Body.CustomRanges...)
	bad.Body.CustomRanges = append(bad.Body.CustomRanges, vdocsRangeRecord{ID: "structural", Type: 3, StartIndex: wire.Body.Tables[0].StartIndex, EndIndex: wire.Body.Tables[0].StartIndex, Properties: sfAttrs(map[string]string{"bookmarkId": "structural"})})
	recovered, err := decodeVDOCSDocument(ctx, bad)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(recovered.Blocks[4].Anchors, []string{"structural"}) {
		t.Fatal("native table-start point lost")
	}
	for _, raw := range []string{`{"BookmarkID":"x"}`, `{"bookmarkId":"x","manydocNavigation":{"Version":1,"role":"run","paragraphId":"p"}}`} {
		if _, err := readVDOCSBookmark(vdocsRangeRecord{ID: "a", Properties: json.RawMessage(raw)}); err == nil {
			t.Fatal("noncanonical native point field accepted")
		}
	}
	for _, hint := range []string{`{"version":2,"role":"run","paragraphId":"p"}`, `{"version":1,"role":"unknown","paragraphId":"p"}`, `{"version":1,"role":"run","paragraphId":"p","hidden":true}`} {
		_, err := readVDOCSBookmark(vdocsRangeRecord{ID:

		// The last main table is a list continuation. Its new terminal point must
		// remain in that item without invalidating paragraph-ID ownership metadata.
		"a", Properties: json.RawMessage(`{"bookmarkId":"a","manydocNavigation":` + hint + `}`)})
		if err == nil {
			t.Fatal("malformed point hint accepted")
		}
	}
}
func FuzzVDOCSNavigation(f *testing.F) {
	wire, err := encodeVDOCSDocument(context.Background(), vdocsNavigationExample(f))
	if err != nil {
		f.Fatal(err)
	}
	raw, _ := json.Marshal(wire)
	f.Add(raw)
	f.Fuzz(func(t *testing.T, raw []byte) {
		var input vdocsNoteSnapshot
		if json.Unmarshal(raw, &input) != nil {
			return
		}
		d, err := decodeVDOCSDocument(context.Background(), input)
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

func TestVDOCSNavigationGeneratedAliasesAndNonmainData(t *testing.T) {
	ctx := context.Background()
	want := vdocsNavigationExample(t)
	wire, err := encodeVDOCSDocument(ctx, want)
	if err != nil {
		t.Fatal(err)
	}
	var accidental string
	for _, a := range wire.Body.CustomRanges {
		if a.Type == 3 {
			p, _ := readVDOCSBookmark(a)
			if p.BookmarkID == "before" {
				accidental = a.ID
			}
		}
	}
	want.UnresolvedDestinations = append(want.UnresolvedDestinations, accidental)
	want.Blocks = append(want.Blocks, Block{Kind: Paragraph, Runs: []Run{{Text: "must remain missing", Destination: accidental}}})
	reserved, err := encodeVDOCSDocument(ctx, want)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range reserved.Body.CustomRanges {
		if a.Type == 3 && a.ID == accidental {
			t.Fatal("transport ID accidentally resolves a declared missing target")
		}
	}
	if _, err = decodeVDOCSDocument(ctx, reserved); err != nil {
		t.Fatal(err)
	}
	want = vdocsNavigationExample(t)
	want.Blocks = append(want.Blocks, Block{Kind: Paragraph, Runs: []Run{{Text: "nonmain target", Destination: "note-point"}}})
	wire, err = encodeVDOCSDocument(ctx, want)
	if err != nil {
		t.Fatal(err)
	}
	got, err := decodeVDOCSDocument(ctx, wire)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, diag := range got.Diagnostics {
		found = found || diag.Code == "vdocs_nonmain_navigation"
	}
	if !found {
		t.Fatal("SDK main-story target limitation was not disclosed")
	}
}

func TestVDOCSNavigationDenseParagraphGraph(t *testing.T) {
	d := &Document{}
	for i := 0; i < 1024; i++ {
		d.Blocks = append(d.Blocks, Block{Kind: Paragraph, Anchors: []string{fmt.Sprintf("point-%d", i)}, Runs: []Run{{Text: "😀x"}}})
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
		t.Fatal("dense point graph changed structure/content/identities")
	}
}
func TestVDOCSNativeBookmarkSpanLossIsExplicit(t *testing.T) {
	wire, err := encodeVDOCSDocument(context.Background(), vdocsNavigationExample(t))
	if err != nil {
		t.Fatal(err)
	}
	for i, a := range wire.Body.CustomRanges {
		if a.Type == 3 {
			wire.Body.CustomRanges[i].EndIndex = 2
			break
		}
	}
	d, err := decodeVDOCSDocument(context.Background(), wire)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, diag := range d.Diagnostics {
		found = found || diag.Code == "vdocs_bookmark_span"
	}
	if !found {
		t.Fatal("native bookmark editing extent was silently lost")
	}
	if navigationParagraphPositions(d.Blocks[0])["paragraph"] != 0 {
		t.Fatal("bookmark start moved")
	}
}

func FuzzVDOCSNavigationPoint(f *testing.F) {
	ctx := context.Background()
	base, err := encodeVDOCSDocument(ctx, &Document{Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "😀A"}}}}})
	if err != nil {
		f.Fatal(err)
	}
	f.Add([]byte(`{"bookmarkId":"a"}`), 0, 0)
	f.Add([]byte(`{"bookmarkId":"a","manydocNavigation":{"version":1,"role":"block","paragraphId":"p1"}}`), 0, 2)
	f.Fuzz(func(t *testing.T, props []byte, start, end int) {
		input := base
		input.Body.CustomRanges = append([]vdocsRangeRecord(nil), base.Body.CustomRanges...)
		input.Body.CustomRanges = append(input.Body.CustomRanges, vdocsRangeRecord{ID: "point", Type: 3, StartIndex: start, EndIndex: end, Properties: props})
		d, err := decodeVDOCSDocument(ctx, input)
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

func TestVDOCSStaleBookmarkStillReportsUnknownMetadata(t *testing.T) {
	wire, err := encodeVDOCSDocument(context.Background(), vdocsNavigationExample(t))
	if err != nil {
		t.Fatal(err)
	}
	for i, a := range wire.Body.CustomRanges {
		if a.Type == 3 {
			p, _ := readVDOCSBookmark(a)
			if p.BookmarkID == "paragraph" {
				props, _ := vdocsStyleObject(a.Properties)
				props["title"] = sfAttrs("unsupported bookmark metadata")
				wire.Body.CustomRanges[i].Properties = sfAttrs(props)
				wire.Body.CustomRanges[i].StartIndex = 3
				wire.Body.CustomRanges[i].EndIndex = 3
				break
			}
		}
	}
	d, err := decodeVDOCSDocument(context.Background(), wire)
	if err != nil {
		t.Fatal(err)
	}
	codes := map[string]bool{}
	for _, diag := range d.Diagnostics {
		codes[diag.Code] = true
	}
	if !codes["vdocs_bookmark_metadata"] || !codes["vdocs_stale_bookmark_placement"] {
		t.Fatal("stale placement hid metadata loss", d.Diagnostics)
	}
}
func vdocsOriginExample(t *testing.T) *Document {
	t.Helper()
	history := vdocsEvidenceExample(t)
	source := func(id string, order int) *SourceInfo {
		q := Quad{{10, 20}, {100, 20}, {100, 35}, {10, 35}}
		return &SourceInfo{Format: PDF, Page: 1, Element: id, Order: order, Quad: &q, Structure: "explicit", Evidence: "tagged_structure", Confidence: 1, Precision: "source_bounds", StructureID: []byte{0xff, 0, byte(order)}}
	}
	p := func(s, id string, n int) Block {
		return Block{Kind: Paragraph, Source: source(id, n), Runs: []Run{{Text: s, Bold: true, FontSize: 13}}}
	}
	return &Document{Pages: history.Pages, Metadata: Metadata{Title: "Paragraph origins", PageCount: 2}, Blocks: []Block{p("😀 First", "first", 1), p("Second", "second", 2), {Kind: ListBlock, List: "bullet", Items: []ListItem{{Blocks: []Block{p("Item", "item", 3)}}}}, {Kind: Table, Rows: []Row{{Cells: []Cell{{Blocks: []Block{p("Cell", "cell", 4)}}}}}}, {Kind: Paragraph, Runs: []Run{{Text: "Note"}, {Note: &NoteReference{Kind: Footnote, ID: "n"}}}}}, Headers: []Block{p("Header", "header", 5)}, Footers: []Block{p("Footer", "footer", 6)}, Notes: []Note{{Kind: Footnote, ID: "n", Blocks: []Block{p("Note body", "note", 7)}}}}
}
func TestVDOCSParagraphOriginsAndNativeSave(t *testing.T) {
	ctx := context.Background()
	want := vdocsOriginExample(t)
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
	again, err := encodeVDOCSDocument(ctx, model)
	if err != nil {
		t.Fatal(err)
	}
	round, err := decodeVDOCSDocument(ctx, again)
	if err != nil {
		t.Fatal(err)
	}
	assertEvidenceSF(t, want, round)
	got.Blocks[0].Source.StructureID[0] = 0
	againModel, err := decodeVDOCSDocument(ctx, wire)
	if err != nil {
		t.Fatal(err)
	}
	assertEvidenceSF(t, want, againModel)
	if file := os.Getenv("MANYDOC_VDOCS_ORIGIN_QA"); file != "" {
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
	for _, spec := range []struct{ env, mode string }{{"MANYDOC_VDOCS_ORIGIN_SAVED", "saved"}, {"MANYDOC_VDOCS_ORIGIN_EDITED", "edited"}, {"MANYDOC_VDOCS_ORIGIN_DETACHED", "detached"}} {
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
		switch spec.mode {
		case "edited":
			expected.Blocks[0].Runs = append([]Run{{Text: "Q"}}, expected.Blocks[0].Runs...)
			expected.Blocks[0].Source.Historical = true
			expected.Diagnostics = append(expected.Diagnostics, Diagnostic{Code: "vdocs_source_historical", Message: "Original PDF source facts are retained as historical after current paragraph content or presentation changed"})
		case "detached":
			expected.UnboundSources = []SourceInfo{*cloneSourceInfo(expected.Blocks[0].Source)}
			expected.Blocks[0].Source = nil
			expected.Diagnostics = append(expected.Diagnostics, Diagnostic{Code: "vdocs_source_unbound", Message: "Original PDF source facts are retained unbound after their native paragraph binding disappeared"})
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
		next, err := encodeVDOCSDocument(ctx, actual)
		if err != nil {
			t.Fatal(err)
		}
		stable, err := decodeVDOCSDocument(ctx, next)
		if err != nil {
			t.Fatal(err)
		}
		assertEvidenceSF(t, actual, stable)
	}
}
func TestVDOCSUnboundSourceSFAndFormattingAdmission(t *testing.T) {
	d := vdocsOriginExample(t)
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

func TestVDOCSParagraphOriginsExplicitSectionAndMalformed(t *testing.T) {
	ctx := context.Background()
	d := vdocsOriginExample(t)
	header := &RunningStory{Blocks: d.Headers}
	footer := &RunningStory{Blocks: d.Footers}
	d.Headers = nil
	d.Footers = nil
	d.Blocks = []Block{{Kind: SectionBlock, Section: &Section{Headers: RunningStories{Default: header}, Footers: RunningStories{Default: footer}, Blocks: d.Blocks}}}
	d.Blocks[0].Section.Blocks[4].Source = cloneSourceInfo(d.Blocks[0].Section.Blocks[0].Source)
	wire, err := encodeVDOCSDocument(ctx, d)
	if err != nil {
		t.Fatal(err)
	}
	got, err := decodeVDOCSDocument(ctx, wire)
	if err != nil {
		t.Fatal(err)
	}
	assertEvidenceSF(t, d, got)
	opened, err := openVDOCSIntent(ctx, wire)
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"digest", "version", "missing"} {
		bad := opened
		bad.Resources = append([]vdocsResource(nil), opened.Resources...)
		var intent vdocsDocumentIntent
		if err = vdocsResourceMap(opened.Resources, vdocsDocumentResource, &intent); err != nil {
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
			if r.Name == vdocsDocumentResource {
				bad.Resources[i].Data = string(sfAttrs(intent))
			}
		}
		if _, err = decodeVDOCSDocument(ctx, bad); err == nil {
			t.Fatal("invalid origin record accepted", mode)
		}
	}
}

func FuzzVDOCSParagraphOrigins(f *testing.F) {
	ctx := context.Background()
	page := PDFPage{Number: 1, MediaBox: [4]float64{0, 0, 200, 300}, CropBox: [4]float64{0, 0, 200, 300}, Width: 200, Height: 300, UserUnit: 1, Transform: Matrix{1, 0, 0, -1, 0, 300}}
	d := &Document{Pages: []PDFPage{page}, Blocks: []Block{{Kind: Paragraph, Source: &SourceInfo{Format: PDF, Page: 1, Element: "p", Structure: "explicit", Evidence: "synthetic", Confidence: 1}, Runs: []Run{{Text: "Body"}}}}}
	wire, err := encodeVDOCSDocument(ctx, d)
	if err != nil {
		f.Fatal(err)
	}
	base, err := openVDOCSIntent(ctx, wire)
	if err != nil {
		f.Fatal(err)
	}
	var intent vdocsDocumentIntent
	if err = vdocsResourceMap(base.Resources, vdocsDocumentResource, &intent); err != nil {
		f.Fatal(err)
	}
	seed, _ := json.Marshal(intent.Evidence.Origins[0])
	f.Add(seed)
	f.Add([]byte(`{"version":2}`))
	pageJSON, _ := json.Marshal(d.Pages)
	f.Fuzz(func(t *testing.T, raw []byte) {
		input := base
		input.Resources = append([]vdocsResource(nil), base.Resources...)
		for i, r := range input.Resources {
			if r.Name == vdocsDocumentResource {
				input.Resources[i].Data = `{"layouts":[],"evidence":{"version":1,"pages":` + string(pageJSON) + `,"origins":[` + string(raw) + `]}}`
			}
		}
		got, err := decodeVDOCSDocument(ctx, input)
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
func vdocsRunOriginExample(t *testing.T) *Document {
	d := vdocsOriginExample(t)
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
func TestVDOCSRunOriginsAndActualSave(t *testing.T) {
	ctx := context.Background()
	want := vdocsRunOriginExample(t)
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
		t.Fatal("native source mutated")
	}
	assertEvidenceSF(t, want, got)
	if file := os.Getenv("MANYDOC_VDOCS_RUN_ORIGIN_QA"); file != "" {
		if err = os.WriteFile(file, before, 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, spec := range []struct{ env, mode string }{{"MANYDOC_VDOCS_RUN_ORIGIN_SAVED", "saved"}, {"MANYDOC_VDOCS_RUN_ORIGIN_EDITED", "edited"}, {"MANYDOC_VDOCS_RUN_ORIGIN_CITATION", "citation"}, {"MANYDOC_VDOCS_RUN_ORIGIN_DETACHED", "detached"}} {
		t.Run(spec.mode, func(t *testing.T) {
			file := os.Getenv(spec.env)
			if file == "" {
				t.Skip("optional actual reader artifact")
			}
			raw, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			var wire vdocsNoteSnapshot
			if err = json.Unmarshal(raw, &wire); err != nil {
				t.Fatal(err)
			}
			envelope, _ := json.Marshal(vdocsEnvelope{vdocsEnvelopeType, 3, "univer", vdocsEngineVersion, wire})
			actual, err := readVDOCS(ctx, bytes.NewReader(envelope))
			if err != nil {
				t.Fatal(err)
			}
			expected := vdocsRunOriginExample(t)
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
				expected.Diagnostics = []Diagnostic{{Code: "vdocs_run_source_historical", Message: "Original PDF source facts are retained as historical after current run content or presentation changed"}, {Code: "vdocs_source_historical", Message: "Original PDF source facts are retained as historical after current paragraph content or presentation changed"}}
			case "citation":
				expected.Blocks[4].Runs[1].Note.LabelContent[0].Text = "X"
				expected.Blocks[4].Runs[1].Note.LabelContent[0].Source.Historical = true
				expected.Blocks[4].Runs[1].Source.Historical = true
				expected.Diagnostics = []Diagnostic{{Code: "vdocs_run_source_historical", Message: "Original PDF source facts are retained as historical after current run content or presentation changed"}}
			case "detached":
				expected.UnboundSources = []SourceInfo{*cloneSourceInfo(expected.Blocks[0].Runs[0].Source)}
				expected.Blocks[0].Runs[0].Source = nil
				expected.Diagnostics = []Diagnostic{{Code: "vdocs_run_source_unbound", Message: "Original PDF source facts are retained unbound after their native run binding disappeared"}}
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
			next, err := encodeVDOCSDocument(ctx, model)
			if err != nil {
				t.Fatal(err)
			}
			stable, err := decodeVDOCSDocument(ctx, next)
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
func TestVDOCSRunOriginMalformedAndUnsupportedSlots(t *testing.T) {
	ctx := context.Background()
	d := vdocsRunOriginExample(t)
	native, err := encodeVDOCSDocument(ctx, d)
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
	for _, mode := range []string{"version", "digest", "duplicate", "missing"} {
		t.Run(mode, func(t *testing.T) {
			raw, _ := json.Marshal(intent)
			var copy vdocsDocumentIntent
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
			wire.Resources = vdocsReplaceResource(wire.Resources, vdocsDocumentResource, copy)
			if _, err = decodeVDOCSDocument(ctx, wire); err == nil {
				t.Fatal("invalid run source accepted")
			}
		})
	}

	source := cloneSourceInfo(d.Blocks[0].Runs[0].Source)
	for _, r := range []Run{{Source: source}} {
		if _, err = encodeVDOCSTextBody(ctx, []Block{{Kind: Paragraph, Runs: []Run{r}}}); err == nil {
			t.Fatal("unmapped empty source silently discarded")
		}
	}
}

func TestVDOCSRunOriginDirectOwnership(t *testing.T) {
	ctx := context.Background()
	for _, mode := range []string{"inherited", "overwritten", "contradictory"} {
		t.Run(mode, func(t *testing.T) {
			d := vdocsRunOriginExample(t)
			wire, err := encodeVDOCSDocument(ctx, d)
			if err != nil {
				t.Fatal(err)
			}
			props, err := vdocsStyleObject(wire.Body.TextRuns[0].TS)
			if err != nil {
				t.Fatal(err)
			}
			ref := props["manydocOrigin"]
			switch mode {
			case "inherited":
				delete(props, "manydocOrigin")
				wire.Body.TextRuns[0].TS = sfAttrs(props)
				style, _ := vdocsStyleObject(wire.DocumentStyle)
				style["textStyle"] = sfAttrs(map[string]json.RawMessage{"manydocOrigin": ref})
				wire.DocumentStyle = sfAttrs(style)
			case "overwritten":
				delete(props, "manydocOrigin")
				last := wire.Body.TextRuns[0]
				last.TS = sfAttrs(props)
				wire.Body.TextRuns = append(wire.Body.TextRuns, last)
			case "contradictory":
				origin, err := readVDOCSOriginProfile(ref, vdocsRunOriginProfile)
				if err != nil {
					t.Fatal(err)
				}
				origin.Fingerprint = strings.Repeat("0", 64)
				props["manydocOrigin"] = sfAttrs(origin)
				wire.Body.TextRuns[0].TS = sfAttrs(props)
			}
			got, err := decodeVDOCSDocument(ctx, wire)
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
					found = found || diagnostic.Code == "vdocs_run_origin_inheritance"
				}
				if !found {
					t.Fatal("missing inherited ownership warning")
				}
			}
		})
	}
}
func FuzzVDOCSRunOrigin(f *testing.F) {
	f.Add([]byte(sfAttrs(vdocsOrigin{Profile: vdocsRunOriginProfile, Version: 1, Ref: strings.Repeat("0", 64)})))
	f.Add([]byte(`{"profile":"manydoc.run-origin","version":2}`))
	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) > 32768 {
			return
		}
		_, _ = readVDOCSOriginProfile(raw, vdocsRunOriginProfile)
	})
}
func vdocsStandaloneExample(t testing.TB) *Document {
	t.Helper()
	point := func(ids ...string) Block { return Block{Kind: AnchorPoint, Anchors: ids} }
	p := func(s string) Block { return Block{Kind: Paragraph, Runs: []Run{{Text: s}}} }
	list := Block{Kind: ListBlock, List: "bullet", Anchors: []string{"list"}, Items: []ListItem{{Blocks: []Block{p("Item"), point("item-first"), point("item-mid"), p("Tail"), point("item-end")}}, {Blocks: []Block{p("Two"), point("item-two-end")}}}}
	return &Document{Blocks: []Block{point("a", "alias"), point("b"), p("Lead 😀"), point("before-list"), list, point("after-list"), {Kind: Table, Rows: []Row{{Cells: []Cell{{Blocks: []Block{point("cell-start"), p("Cell"), point("cell-end")}}, {Blocks: []Block{point("only-cell")}}}}}}, point("before-image"), {Kind: Picture, Image: &Image{Data: runningPNG(t, false), MediaType: "image/png", Width: 12, Height: 9}}, point("before-break"), {Kind: PageBreak}, p("After"), point("end")}, Headers: []Block{point("header-start"), p("Header"), point("header-end")}, Notes: []Note{{Kind: Footnote, ID: "n", Blocks: []Block{point("only-note")}}}}
}
func TestVDOCSStandalonePointsRoundTrip(t *testing.T) {
	ctx := context.Background()
	want := vdocsStandaloneExample(t)

	var links []Run
	for _, id := range []string{"a", "alias", "b", "before-list", "item-first", "item-mid", "item-end", "item-two-end", "after-list", "cell-start", "cell-end", "only-cell", "before-image", "before-break", "end"} {
		links = append(links, Run{Text: id, Destination: id})
	}
	links = append(links, Run{Note: &NoteReference{Kind: Footnote, ID: "n"}})
	want.Blocks[11].Runs = links
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
		t.Fatalf("point structure changed\ngot%s\nwant%s", a, b)
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
		t.Fatal("SF point grouping/flow ownership changed")
	}
	if file := os.Getenv("MANYDOC_VDOCS_STANDALONE_QA"); file != "" {
		if err = os.WriteFile(file, before, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if file := os.Getenv("MANYDOC_VDOCS_STANDALONE_SAVED"); file != "" {
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
		saved, err := readVDOCS(ctx, bytes.NewReader(envelope))
		if err != nil {
			t.Fatal(err)
		}
		actual, err := ToStructured(saved)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(actual, sf) {
			a, _ := json.Marshal(actual)
			b, _ := json.Marshal(sf)
			t.Fatalf("actual native save changed\ngot%s\nwant%s", a, b)
		}
	}
	if file := os.Getenv("MANYDOC_VDOCS_STANDALONE_EDITED"); file != "" {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var native vdocsNoteSnapshot
		if err = json.Unmarshal(raw, &native); err != nil {
			t.Fatal(err)
		}
		actual, err := decodeVDOCSDocument(ctx, native)
		if err != nil {
			t.Fatal(err)
		}
		expected, err := FromStructured(sf)
		if err != nil {
			t.Fatal(err)
		}
		expected.Blocks = expected.Blocks[2:]
		expected.Blocks[0].Runs = append([]Run{{Text: "Q"}, {Anchor: "a"}, {Anchor: "alias"}, {Anchor: "b"}}, expected.Blocks[0].Runs...)
		expected.Diagnostics = []Diagnostic{{Code: "vdocs_stale_point_ownership", Message: "Standalone point retains its current native location because its owning flow or boundary changed"}}
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
			t.Fatalf("actual moved points changed graph\ngot%s\nwant%s", a, b)
		}
	}
}
func TestVDOCSStandaloneDenseItems(t *testing.T) {
	d := &Document{Blocks: []Block{{Kind: ListBlock, List: "bullet"}}}
	for i := 0; i < 256; i++ {
		d.Blocks[0].Items = append(d.Blocks[0].Items, ListItem{Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: fmt.Sprint(i)}}}, {Kind: AnchorPoint, Anchors: []string{fmt.Sprint("end-", i)}}}})
	}
	wire, err := encodeVDOCSDocument(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	got, err := decodeVDOCSDocument(context.Background(), wire)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(d, got) {
		t.Fatal("dense item-end points changed grouping")
	}
}

func TestVDOCSStandaloneCurrentNativeEdits(t *testing.T) {
	ctx := context.Background()
	d := &Document{Blocks: []Block{{Kind: AnchorPoint, Anchors: []string{"point"}}, {Kind: Paragraph, Runs: []Run{{Text: "Text"}}}}}
	wire, err := encodeVDOCSDocument(ctx, d)
	if err != nil {
		t.Fatal(err)
	}
	for i, a := range wire.Body.CustomRanges {
		if a.Type == 3 {
			wire.Body.CustomRanges[i].StartIndex = 2
			wire.Body.CustomRanges[i].EndIndex = 2
			break
		}
	}
	got, err := decodeVDOCSDocument(ctx, wire)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Blocks) != 1 || len(got.Blocks[0].Runs) != 3 || got.Blocks[0].Runs[1].Anchor != "point" {
		t.Fatal("moved point restored obsolete boundary")
	}
	if len(got.Diagnostics) != 1 || got.Diagnostics[0].Code != "vdocs_stale_point_ownership" {
		t.Fatal("ownership change not disclosed")
	}

	wire, err = encodeVDOCSDocument(ctx, &Document{Blocks: []Block{{Kind: AnchorPoint, Anchors: []string{"point"}}, {Kind: Paragraph, Runs: []Run{{Text: "Text"}}}, {Kind: Paragraph}}})
	if err != nil {
		t.Fatal(err)
	}
	for i, a := range wire.Body.CustomRanges {
		if a.Type == 3 {
			p, err := readVDOCSBookmark(a)
			if err != nil {
				t.Fatal(err)
			}
			p.Hint.PointCarrier = wire.Body.Paragraphs[1].ID
			wire.Body.CustomRanges[i].Properties = sfAttrs(p)
		}
	}
	got, err = decodeVDOCSDocument(ctx, wire)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Blocks) != 3 || got.Blocks[2].Kind != Paragraph {
		t.Fatal("unrelated authored empty paragraph discarded")
	}
	found := false
	for _, diag := range got.Diagnostics {
		found = found || diag.Code == "vdocs_stale_point_carrier"
	}
	if !found {
		t.Fatal("invalid carrier binding not disclosed")
	}
}
func TestVDOCSStandalonePointOnlyAndMalformed(t *testing.T) {
	ctx := context.Background()
	d := &Document{Blocks: []Block{{Kind: AnchorPoint, Anchors: []string{"a"}}, {Kind: AnchorPoint, Anchors: []string{"b", "c"}}}}
	wire, err := encodeVDOCSDocument(ctx, d)
	if err != nil {
		t.Fatal(err)
	}
	if wire.Body.DataStream != "\r\n" {
		t.Fatal("point-only body inserted extra visible structure")
	}
	got, err := decodeVDOCSDocument(ctx, wire)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(d, got) {
		t.Fatal("point-only groups changed")
	}
	for _, field := range []string{"pointScope", "pointOrder"} {
		bad := wire
		bad.Body.CustomRanges = append([]vdocsRangeRecord(nil), wire.Body.CustomRanges...)
		p, err := readVDOCSBookmark(bad.Body.CustomRanges[0])
		if err != nil {
			t.Fatal(err)
		}
		if field == "pointScope" {
			p.Hint.PointScope = "unknown"
		} else {
			p.Hint.PointOrder = 0
		}
		bad.Body.CustomRanges[0].Properties = sfAttrs(p)
		if _, err = decodeVDOCSDocument(ctx, bad); err == nil {
			t.Fatal("malformed standalone intent accepted", field)
		}
	}
	bad := wire
	bad.Body.CustomRanges = append([]vdocsRangeRecord(nil), wire.Body.CustomRanges...)
	p, err := readVDOCSBookmark(bad.Body.CustomRanges[2])
	if err != nil {
		t.Fatal(err)
	}
	p.Hint.PointIndex = 0
	bad.Body.CustomRanges[2].Properties = sfAttrs(p)
	if _, err = decodeVDOCSDocument(ctx, bad); err == nil {
		t.Fatal("duplicate alias order accepted")
	}
}

func TestVDOCSStandaloneExplicitSections(t *testing.T) {
	d := &Document{Blocks: []Block{{Kind: SectionBlock, Anchors: []string{"section"}, Section: &Section{Blocks: []Block{{Kind: AnchorPoint, Anchors: []string{"only"}}}}}, {Kind: SectionBlock, Section: &Section{Start: SectionNextPage, Blocks: []Block{{Kind: AnchorPoint, Anchors: []string{"start"}}, {Kind: Paragraph, Runs: []Run{{Text: "Back", Destination: "only"}, {Text: "Section", Destination: "section"}}}, {Kind: AnchorPoint, Anchors: []string{"end"}}}}}}}
	wire, err := encodeVDOCSDocument(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	got, err := decodeVDOCSDocument(context.Background(), wire)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(d, got) {
		a, _ := json.Marshal(got)
		b, _ := json.Marshal(d)
		t.Fatalf("section point graph changed\ngot%s\nwant%s", a, b)
	}
}

type nativeCursorCase struct {
	id             string
	offset         int
	kind, boundary string
}

func vdocsStructuralCases() []nativeCursorCase {
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
func vdocsForeignCursorExample(t testing.TB) (vdocsNoteSnapshot, *Document) {
	t.Helper()
	p := func(s string) Block { return Block{Kind: Paragraph, Runs: []Run{{Text: s}}} }
	table := Block{Kind: Table, ColumnWidths: []float64{72, 72}, Rows: []Row{{Cells: []Cell{{RowSpan: 2, Blocks: []Block{p("Origin")}}, {Blocks: []Block{p("B")}}}}, {Cells: []Cell{{Blocks: []Block{p("C")}}}}}}
	cases := vdocsStructuralCases()
	var links []Run
	var missing []string
	for _, c := range cases {
		links = append(links, Run{Text: "x", Destination: c.id})
		missing = append(missing, c.id)
	}
	d := &Document{Blocks: []Block{p("Lead 😀"), table, {Kind: Picture, Image: &Image{Data: runningPNG(t, false), MediaType: "image/png", Width: 12, Height: 9}}, {Kind: Paragraph, Runs: links}}, UnresolvedDestinations: missing}
	wire, err := encodeVDOCSDocument(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	if wire.Body.DataStream[:len("Lead 😀\r")] != "Lead 😀\r" {
		t.Fatal("unexpected leading stream")
	}
	if vdocsUTF16Len(wire.Body.DataStream) != 64 {
		t.Fatalf("independent cursor fixture length changed: %d", vdocsUTF16Len(wire.Body.DataStream))
	}
	for _, c := range cases {
		wire.Body.CustomRanges = append(wire.Body.CustomRanges, vdocsRangeRecord{ID: "native-" + c.id, Type: 3, StartIndex: c.offset, EndIndex: c.offset, Properties: sfAttrs(map[string]string{"bookmarkId": c.id})})
	}

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
		expected.Diagnostics = append(expected.Diagnostics, Diagnostic{Code: "vdocs_structural_cursor", Message: fmt.Sprintf("Native bookmark at UTF-16 offset %d (%s) projects to %s; exact native control cursor identity is not retained", c.offset, c.kind, c.boundary)})
	}
	return wire, expected
}
func TestVDOCSForeignStructuralCursorProjection(t *testing.T) {
	ctx := context.Background()
	wire, expected := vdocsForeignCursorExample(t)
	before, _ := json.Marshal(wire)
	got, err := decodeVDOCSDocument(ctx, wire)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(wire)
	if !bytes.Equal(before, after) {
		t.Fatal("source snapshot mutated")
	}

	wantDiagnostics := map[string]bool{}
	for _, d := range expected.Diagnostics {
		wantDiagnostics[d.Message] = true
	}
	if len(got.Diagnostics) != len(wantDiagnostics) {
		t.Fatalf("wrong diagnostic count: %+v", got.Diagnostics)
	}
	for _, d := range got.Diagnostics {
		if d.Code != "vdocs_structural_cursor" || !wantDiagnostics[d.Message] {
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

	normalizedInput, err := FromStructured(sf)
	if err != nil {
		t.Fatal(err)
	}
	normalized, err := encodeVDOCSDocument(ctx, normalizedInput)
	if err != nil {
		t.Fatal(err)
	}
	normalizedModel, err := decodeVDOCSDocument(ctx, normalized)
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
	if file := os.Getenv("MANYDOC_VDOCS_CURSOR_NORMALIZED_QA"); file != "" {
		raw, err := json.Marshal(normalized)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(file, raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if file := os.Getenv("MANYDOC_VDOCS_CURSOR_NORMALIZED_SAVED"); file != "" {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var saved vdocsNoteSnapshot
		if err = json.Unmarshal(raw, &saved); err != nil {
			t.Fatal(err)
		}
		envelope, err := json.Marshal(vdocsEnvelope{vdocsEnvelopeType, 3, "univer", vdocsEngineVersion, saved})
		if err != nil {
			t.Fatal(err)
		}
		actual, err := readVDOCS(ctx, bytes.NewReader(envelope))
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
	if file := os.Getenv("MANYDOC_VDOCS_CURSOR_QA"); file != "" {
		if err = os.WriteFile(file, before, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if file := os.Getenv("MANYDOC_VDOCS_CURSOR_SAVED"); file != "" {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var saved vdocsNoteSnapshot
		if err = json.Unmarshal(raw, &saved); err != nil {
			t.Fatal(err)
		}
		envelope, err := json.Marshal(vdocsEnvelope{vdocsEnvelopeType, 3, "univer", vdocsEngineVersion, saved})
		if err != nil {
			t.Fatal(err)
		}
		actual, err := readVDOCS(ctx, bytes.NewReader(envelope))
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

	if file := os.Getenv("MANYDOC_VDOCS_CURSOR_EDITED"); file != "" {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var edited vdocsNoteSnapshot
		if err = json.Unmarshal(raw, &edited); err != nil {
			t.Fatal(err)
		}
		envelope, err := json.Marshal(vdocsEnvelope{vdocsEnvelopeType, 3, "univer", vdocsEngineVersion, edited})
		if err != nil {
			t.Fatal(err)
		}
		actual, err := readVDOCS(ctx, bytes.NewReader(envelope))
		if err != nil {
			t.Fatal(err)
		}
		want, err := FromStructured(sf)
		if err != nil {
			t.Fatal(err)
		}
		want.Blocks[0].Runs = append([]Run{{Text: "Q"}}, want.Blocks[0].Runs...)
		for i, d := range want.Diagnostics {
			for _, c := range vdocsStructuralCases() {
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
func TestVDOCSForeignStructuralCursorListOwnership(t *testing.T) {
	ctx := context.Background()
	base := vdocsContainerNavigationExample(t)
	wire, err := encodeVDOCSDocument(ctx, base)
	if err != nil {
		t.Fatal(err)
	}

	table := wire.Body.Tables[len(wire.Body.Tables)-1]
	wire.Body.CustomRanges = append(wire.Body.CustomRanges, vdocsRangeRecord{ID: "end", Type: 3, StartIndex: table.EndIndex - 1, EndIndex: table.EndIndex - 1})
	got, err := decodeVDOCSDocument(ctx, wire)
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

func TestVDOCSForeignStructuralHorizontalContinuations(t *testing.T) {
	body, tables := nativeTableFixture([][]string{{"A", "", ""}}, [][][2]int{{{1, 3}, {0, 0}, {0, 0}}})
	for i, position := range []int{7, 9, 14} {
		body.CustomRanges = append(body.CustomRanges, vdocsRangeRecord{ID: fmt.Sprintf("horizontal-%d", i), Type: 3, StartIndex: position, EndIndex: position})
	}
	d, err := decodeVDOCSBody(context.Background(), body, vdocsTextContext{}, tables)
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
func TestVDOCSForeignStructuralRunningAndNoteStories(t *testing.T) {
	p := func(s string) Block { return Block{Kind: Paragraph, Runs: []Run{{Text: s}}} }
	table := func(s string) Block { return Block{Kind: Table, Rows: []Row{{Cells: []Cell{{Blocks: []Block{p(s)}}}}}} }
	wire, err := encodeVDOCSDocument(context.Background(), &Document{Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "Body"}, {Note: &NoteReference{Kind: Footnote, ID: "n"}}}}}, Headers: []Block{table("Header")}, Footers: []Block{table("Footer")}, Notes: []Note{{Kind: Footnote, ID: "n", Blocks: []Block{table("Note")}}}})
	if err != nil {
		t.Fatal(err)
	}
	for id, story := range wire.Headers {
		story.Body.CustomRanges = append(story.Body.CustomRanges, vdocsRangeRecord{ID: "header-row", Type: 3, StartIndex: 1, EndIndex: 1})
		wire.Headers[id] = story
	}
	for id, story := range wire.Footers {
		story.Body.CustomRanges = append(story.Body.CustomRanges, vdocsRangeRecord{ID: "footer-row", Type: 3, StartIndex: 1, EndIndex: 1})
		wire.Footers[id] = story
	}
	for id, note := range wire.Notes {
		note.Body.CustomRanges = append(note.Body.CustomRanges, vdocsRangeRecord{ID: "note-lf", Type: 3, StartIndex: 8, EndIndex: 8})
		wire.Notes[id] = note
	}
	got, err := decodeVDOCSDocument(context.Background(), wire)
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
func FuzzVDOCSForeignStructuralCursor(f *testing.F) {
	ctx := context.Background()
	base, err := encodeVDOCSDocument(ctx, &Document{Blocks: []Block{{Kind: Table, Rows: []Row{{Cells: []Cell{{RowSpan: 2, Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "😀"}}}}}, {Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "B"}}}}}}}, {Cells: []Cell{{Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "C"}}}}}}}}}}})
	if err != nil {
		f.Fatal(err)
	}
	for _, position := range []int{1, 2, 6, 9, 12, 17, 21, 26} {
		f.Add(position, []byte(`{"bookmarkId":"point"}`))
	}
	f.Fuzz(func(t *testing.T, position int, props []byte) {
		wire := base
		wire.Body.CustomRanges = append([]vdocsRangeRecord(nil), base.Body.CustomRanges...)
		wire.Body.CustomRanges = append(wire.Body.CustomRanges, vdocsRangeRecord{ID: "fuzz-point", Type: 3, StartIndex: position, EndIndex: position, Properties: props})
		d, err := decodeVDOCSDocument(ctx, wire)
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

func TestVDOCSForeignStructuralEveryScalarBoundary(t *testing.T) {
	ctx := context.Background()
	base, _ := vdocsForeignCursorExample(t)
	index, err := newVDOCSTextIndex(ctx, base.Body.DataStream)
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
		wire.Body.CustomRanges = append([]vdocsRangeRecord(nil), base.Body.CustomRanges...)
		wire.Body.CustomRanges = append(wire.Body.CustomRanges, vdocsRangeRecord{ID: "probe", Type: 3, StartIndex: position, EndIndex: position})
		d, err := decodeVDOCSDocument(ctx, wire)
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
func TestVDOCSForeignStructuralPageBreakParagraph(t *testing.T) {
	ctx := context.Background()
	wire, err := encodeVDOCSDocument(ctx, &Document{Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "Before"}}}, {Kind: PageBreak}, {Kind: Paragraph, Runs: []Run{{Text: "After"}}}}})
	if err != nil {
		t.Fatal(err)
	}
	wire.Body.CustomRanges = append(wire.Body.CustomRanges, vdocsRangeRecord{ID: "break-end", Type: 3, StartIndex: 8, EndIndex: 8})
	got, err := decodeVDOCSDocument(ctx, wire)
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
func vdocsTableOriginExample(t *testing.T) *Document {
	base := vdocsOriginExample(t)
	src := func(id, role string) *SourceInfo {
		s := cloneSourceInfo(base.Blocks[0].Source)
		s.Element = id
		s.Role = role
		return s
	}
	p := func(text string) Block { return Block{Kind: Paragraph, Runs: []Run{{Text: text}}} }
	cell := func(id, text string) Cell { return Cell{Source: src(id, "TD"), Blocks: []Block{p(text)}} }
	table := func(id, text string) Block {
		return Block{Kind: Table, Source: src(id, "Table"), Rows: []Row{{Cells: []Cell{cell(id+"-cell", text)}}}}
	}
	nested := table("nested", "Nested")
	wide := cell("wide", "Wide")
	wide.ColSpan = 2
	wide.Source.TableSpans = &TableSpanSource{ColSpan: "explicit", RowSpan: "explicit", Evidence: "painted_grid_adjacency", Confidence: .8, Bounds: *wide.Source.Quad}
	tall := cell("tall", "Tall")
	tall.RowSpan = 2
	tall.Blocks = append(tall.Blocks, nested)
	outer := Block{Kind: Table, Source: src("outer", "Table"), Rows: []Row{{RepeatHeader: true, Cells: []Cell{wide}}, {Cells: []Cell{tall, cell("right", "Right")}}, {Cells: []Cell{cell("bottom", "Bottom")}}}}
	outer.Source.TableWidths = &TableWidthSource{Evidence: "painted_grid_adjacency", Confidence: .75, Bounds: *outer.Source.Quad}
	return &Document{Pages: base.Pages, Metadata: Metadata{Title: "Table origins"}, Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "😀 Source tables", Bold: true, FontSize: 13}}}, outer, {Kind: Paragraph, Runs: []Run{{Note: &NoteReference{Kind: Footnote, ID: "n"}}}}}, Headers: []Block{table("header", "Header")}, Footers: []Block{table("footer", "Footer")}, Notes: []Note{{Kind: Footnote, ID: "n", Blocks: []Block{table("note", "Note")}}}}
}
func TestVDOCSTableOriginsAndActualSave(t *testing.T) {
	ctx := context.Background()
	want := vdocsTableOriginExample(t)
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
		t.Fatal("native source mutated")
	}
	assertEvidenceSF(t, want, got)
	if file := os.Getenv("MANYDOC_VDOCS_TABLE_ORIGIN_QA"); file != "" {
		if err = os.WriteFile(file, before, 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, spec := range []struct{ env, mode string }{{"MANYDOC_VDOCS_TABLE_ORIGIN_SAVED", "saved"}, {"MANYDOC_VDOCS_TABLE_ORIGIN_EDITED", "edited"}, {"MANYDOC_VDOCS_TABLE_ORIGIN_DETACHED", "detached"}, {"MANYDOC_VDOCS_TABLE_ORIGIN_COVERED", "covered"}} {
		t.Run(spec.mode, func(t *testing.T) {
			file := os.Getenv(spec.env)
			if file == "" {
				t.Skip("optional independent reader artifact")
			}
			raw, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			var native vdocsNoteSnapshot
			if err = json.Unmarshal(raw, &native); err != nil {
				t.Fatal(err)
			}
			envelope, _ := json.Marshal(vdocsEnvelope{vdocsEnvelopeType, 3, "univer", vdocsEngineVersion, native})
			actual, err := readVDOCS(ctx, bytes.NewReader(envelope))
			if err != nil {
				t.Fatal(err)
			}
			expected := vdocsTableOriginExample(t)
			switch spec.mode {
			case "edited":
				expected.Blocks[1].Source.Historical = true
				expected.Blocks[1].Rows[0].Cells[0].Source.Historical = true
				expected.Blocks[1].Rows[0].Cells[0].Blocks[0].Runs = []Run{{Text: "Q"}, {Text: "Wide"}}
				expected.Diagnostics = []Diagnostic{{Code: "vdocs_table_source_historical", Message: "Original table or cell source facts are retained as historical after current content, grid or presentation changed"}}
			case "detached":
				expected.UnboundSources = []SourceInfo{*cloneSourceInfo(expected.Blocks[1].Rows[0].Cells[0].Source)}
				expected.Blocks[1].Rows[0].Cells[0].Source = nil
				expected.Diagnostics = []Diagnostic{{Code: "vdocs_table_source_unbound", Message: "Original table or cell source facts are retained unbound after their native ownership disappeared"}}
			case "covered":
				expected.UnboundSources = []SourceInfo{*cloneSourceInfo(expected.Blocks[1].Rows[1].Cells[0].Source)}
				expected.Blocks[1].Rows[1].Cells[0].Source = nil
				expected.Diagnostics = []Diagnostic{{Code: "vdocs_table_source_unbound", Message: "Original table or cell source facts are retained unbound after their native ownership disappeared"}}
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
			next, err := encodeVDOCSDocument(ctx, model)
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
func TestVDOCSTableOriginsMalformed(t *testing.T) {
	ctx := context.Background()
	wire, err := encodeVDOCSDocument(ctx, vdocsTableOriginExample(t))
	if err != nil {
		t.Fatal(err)
	}
	opened, err := openVDOCSIntent(ctx, wire)
	if err != nil {
		t.Fatal(err)
	}
	var intent vdocsDocumentIntent
	if err = vdocsResourceMap(opened.Resources, vdocsDocumentResource, &intent); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"version", "digest", "profile", "duplicate", "missing"} {
		t.Run(mode, func(t *testing.T) {
			raw, _ := json.Marshal(intent)
			var copy vdocsDocumentIntent
			_ = json.Unmarshal(raw, &copy)
			switch mode {
			case "version":
				copy.Evidence.TableOrigins[0].Version = 2
			case "digest":
				copy.Evidence.TableOrigins[0].Source.Element = "changed"
			case "profile":
				copy.Evidence.TableOrigins[0].Profile = vdocsRunOriginProfile
			case "duplicate":
				copy.Evidence.TableOrigins = append(copy.Evidence.TableOrigins, copy.Evidence.TableOrigins[0])
			case "missing":
				copy.Evidence.TableOrigins = nil
			}
			native := opened
			native.Resources = vdocsReplaceResource(native.Resources, vdocsDocumentResource, copy)
			if _, err = decodeVDOCSDocument(ctx, native); err == nil {
				t.Fatal("invalid table source accepted")
			}
		})
	}
}

func TestVDOCSTableOriginsSharedFactsAndCoveredReference(t *testing.T) {
	ctx := context.Background()
	d := vdocsTableOriginExample(t)
	source := cloneSourceInfo(d.Blocks[1].Source)
	source.TableWidths = nil
	source.Role = ""
	d.Blocks[1].Rows = d.Blocks[1].Rows[:1]
	d.Blocks[1].Rows[0].Cells[0].ColSpan = 0
	d.Blocks[1].Rows[0].Cells[0].Source = source
	d.Blocks[1].Source = source
	native, err := encodeVDOCSDocument(ctx, d)
	if err != nil {
		t.Fatal(err)
	}
	got, err := decodeVDOCSDocument(ctx, native)
	if err != nil {
		t.Fatal(err)
	}
	assertEvidenceSF(t, d, got)
	wire, err := encodeVDOCSDocument(ctx, vdocsTableOriginExample(t))
	if err != nil {
		t.Fatal(err)
	}
	for id, raw := range wire.Tables {
		props, _ := vdocsStyleObject(raw)
		var rows []json.RawMessage
		_ = json.Unmarshal(props["tableRows"], &rows)
		if len(rows) != 3 {
			continue
		}
		row, _ := vdocsStyleObject(rows[2])
		var cells []json.RawMessage
		_ = json.Unmarshal(row["tableCells"], &cells)
		cell, _ := vdocsStyleObject(cells[0])
		cell["manydocOrigin"] = sfAttrs(vdocsOrigin{Profile: vdocsCellOriginProfile, Version: 1, Ref: strings.Repeat("0", 64)})
		cells[0] = sfAttrs(cell)
		row["tableCells"] = sfAttrs(cells)
		rows[2] = sfAttrs(row)
		props["tableRows"] = sfAttrs(rows)
		wire.Tables[id] = sfAttrs(props)
	}
	if _, err = decodeVDOCSDocument(ctx, wire); err == nil {
		t.Fatal("missing reference on covered cell accepted")
	}
}
func FuzzVDOCSTableOrigins(f *testing.F) {
	f.Add([]byte(`{"tableId":"probe","tableRows":[{"tableCells":[{"rowSpan":1,"columnSpan":1}]}]}`))
	f.Add([]byte(`{"tableRows":null}`))
	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) > 32768 {
			return
		}
		snapshot := vdocsNoteSnapshot{Tables: map[string]json.RawMessage{"probe": raw}}
		_ = visitVDOCSTableOrigins(&snapshot, false, func(*vdocsOrigin) error { return nil })
	})
}
