package manifold

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
)

func colleafFlowOriginExample(t *testing.T) *Document {
	base := colleafOriginExample(t)
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
	return &Document{Metadata: Metadata{Title: "Flow origins"}, Pages: base.Pages, Layout: Layout{Width: 450, Height: 600, Margin: 40, HeaderDistance: colleafFloat(10), FooterDistance: colleafFloat(10)}, Blocks: []Block{
		{Kind: SectionBlock, Source: src("section-one", "Sect"), Section: &Section{Headers: RunningStories{Default: &RunningStory{Blocks: []Block{point("header"), p("Header")}}}, Footers: RunningStories{Default: &RunningStory{Blocks: []Block{p("Footer"), point("footer")}}}, Blocks: []Block{point("start", "alias"), {Kind: Paragraph, Runs: []Run{{Text: "😀 First", Bold: true, FontSize: 13}}}, {Kind: PageBreak, Anchors: []string{"break"}, Source: src("hard-break", "Artifact")}, {Kind: Paragraph, Runs: []Run{{Text: "After"}, {Note: &NoteReference{Kind: Footnote, ID: "n"}}}}, point("end")}}},
		{Kind: SectionBlock, Source: src("section-two", "Sect"), Section: &Section{Start: SectionNextPage, Layout: &Layout{Width: 360, Height: 500, Margin: 30}, Blocks: []Block{p("Second"), point("last")}}},
	}, Notes: []Note{{Kind: Footnote, ID: "n", Blocks: []Block{point("note-start"), p("Note")}}}}
}
func TestColleafFlowOriginsAndActualSave(t *testing.T) {
	ctx := context.Background()
	want := colleafFlowOriginExample(t)
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
		t.Fatal("native input mutated")
	}
	assertEvidenceSF(t, want, got)
	if file := os.Getenv("MANYDOC_COLLEAF_FLOW_ORIGIN_QA"); file != "" {
		if err = os.WriteFile(file, before, 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, spec := range []struct{ env, mode string }{{"MANYDOC_COLLEAF_FLOW_ORIGIN_SAVED", "saved"}, {"MANYDOC_COLLEAF_FLOW_ORIGIN_EDITED", "edited"}, {"MANYDOC_COLLEAF_FLOW_ORIGIN_RENAMED", "renamed"}, {"MANYDOC_COLLEAF_FLOW_ORIGIN_DETACHED", "detached"}, {"MANYDOC_COLLEAF_FLOW_ORIGIN_SOFT", "soft"}} {
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
			expected := colleafFlowOriginExample(t)
			switch spec.mode {
			case "edited":
				expected.Blocks[0].Source.Historical = true
				b := &expected.Blocks[0].Section.Blocks[1]
				first := b.Runs[0]
				first.Text = "😀 "
				last := b.Runs[0]
				last.Text = "First"
				b.Runs = []Run{first, {Text: "Q"}, last}
				expected.Diagnostics = []Diagnostic{{Code: "colleaf_flow_source_historical", Message: "Original section, break or point source facts are retained as historical after current semantics changed"}}
			case "renamed":
				expected.Blocks[0].Source.Historical = true
				expected.Blocks[0].Section.Blocks[0].Anchors[1] = "alias-renamed"
				expected.Blocks[0].Section.Blocks[0].Source.Historical = true
				expected.Diagnostics = []Diagnostic{{Code: "colleaf_flow_source_historical", Message: "Original section, break or point source facts are retained as historical after current semantics changed"}}
			case "detached":
				expected.UnboundSources = []SourceInfo{*cloneSourceInfo(expected.Blocks[0].Source)}
				expected.Blocks[0].Source = nil
				expected.Diagnostics = []Diagnostic{{Code: "colleaf_flow_source_unbound", Message: "Original section, break or point source facts are retained unbound after native ownership disappeared"}}
			case "soft":
				expected.Blocks[0].Source.Historical = true
				expected.UnboundSources = []SourceInfo{*cloneSourceInfo(expected.Blocks[0].Section.Blocks[2].Source)}
				expected.Blocks[0].Section.Blocks[2] = Block{Kind: AnchorPoint, Anchors: []string{"break"}}
				expected.Diagnostics = []Diagnostic{{Code: "colleaf_soft_page_break", Message: "Native rendered soft page boundary omitted from logical flow; literal content is retained"}, {Code: "colleaf_flow_source_historical", Message: "Original section, break or point source facts are retained as historical after current semantics changed"}, {Code: "colleaf_flow_source_unbound", Message: "Original section, break or point source facts are retained unbound after native ownership disappeared"}}
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
func TestColleafFlowOriginsMalformed(t *testing.T) {
	ctx := context.Background()
	wire, err := encodeColleafDocument(ctx, colleafFlowOriginExample(t))
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
				copy.Evidence.FlowOrigins[0].Version = 2
			case "digest":
				copy.Evidence.FlowOrigins[0].Source.Element = "changed"
			case "duplicate":
				copy.Evidence.FlowOrigins = append(copy.Evidence.FlowOrigins, copy.Evidence.FlowOrigins[0])
			case "missing":
				copy.Evidence.FlowOrigins = nil
			}
			candidate := opened
			candidate.Resources = colleafReplaceResource(candidate.Resources, colleafDocumentResource, copy)
			if _, err = decodeColleafDocument(ctx, candidate); err == nil {
				t.Fatal("malformed flow source accepted")
			}
		})
	}
}
func FuzzColleafFlowOrigins(f *testing.F) {
	f.Add([]byte(`{"manydocFlowOrigin":{"version":2}}`))
	f.Add([]byte(sfAttrs(map[string]any{"manydocFlowOrigin": colleafOrigin{Profile: colleafFlowOriginProfile, Version: 1, Ref: strings.Repeat("0", 64)}})))
	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) > 32768 {
			return
		}
		snapshot := colleafNoteSnapshot{Body: colleafBody{Paragraphs: []colleafParagraphRecord{{Style: raw}}}}
		_ = visitColleafFlowOrigins(&snapshot, false, func(*colleafOrigin) error { return nil })
	})
}

func TestStructuredIdentitylessEmptySourcePolicy(t *testing.T) {
	base := colleafOriginExample(t)
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
