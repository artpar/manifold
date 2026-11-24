package manydoc

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"testing"
)

func colleafListOriginExample(t *testing.T) *Document {
	d := colleafListExample()
	base := colleafOriginExample(t)
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
func TestColleafListOriginsAndActualSave(t *testing.T) {
	ctx := context.Background()
	want := colleafListOriginExample(t)
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
	if file := os.Getenv("MANYDOC_COLLEAF_LIST_ORIGIN_QA"); file != "" {
		if err = os.WriteFile(file, before, 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, spec := range []struct{ env, mode string }{{"MANYDOC_COLLEAF_LIST_ORIGIN_SAVED", "saved"}, {"MANYDOC_COLLEAF_LIST_ORIGIN_EDITED", "edited"}, {"MANYDOC_COLLEAF_LIST_ORIGIN_RESTARTED", "restarted"}, {"MANYDOC_COLLEAF_LIST_ORIGIN_DETACHED", "detached"}, {"MANYDOC_COLLEAF_LIST_ORIGIN_ITEM_DETACHED", "item-detached"}} {
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
			expected := colleafListOriginExample(t)
			switch spec.mode {
			case "edited":
				expected.Blocks[1].Source.Historical = true
				expected.Blocks[1].Items[0].Source.Historical = true
				expected.Blocks[1].Items[0].Blocks[0].Runs = []Run{{Text: "Q"}, {Text: "Parent one"}}
				expected.Diagnostics = []Diagnostic{{Code: "colleaf_list_source_historical", Message: "Original list or item source facts are retained as historical after current content, numbering or ownership changed"}}
			case "restarted":
				expected.Blocks[2].Start = listStartPointer(9)
				expected.Blocks[2].Source.Historical = true
				for i := range expected.Blocks[2].Items {
					expected.Blocks[2].Items[i].Source.Historical = true
				}
				expected.Diagnostics = []Diagnostic{{Code: "colleaf_list_source_historical", Message: "Original list or item source facts are retained as historical after current content, numbering or ownership changed"}}
			case "detached":
				expected.UnboundSources = []SourceInfo{*cloneSourceInfo(expected.Blocks[1].Source)}
				expected.Blocks[1].Source = nil
				expected.Diagnostics = []Diagnostic{{Code: "colleaf_list_source_unbound", Message: "Original list or item source facts are retained unbound after their native list ownership disappeared"}}
			case "item-detached":
				expected.UnboundSources = []SourceInfo{*cloneSourceInfo(expected.Blocks[1].Items[0].Source)}
				expected.Blocks[1].Items[0].Source = nil
				expected.Diagnostics = []Diagnostic{{Code: "colleaf_list_source_unbound", Message: "Original list or item source facts are retained unbound after their native list ownership disappeared"}}
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
func TestColleafListOriginsMalformedAndSourceFreeContext(t *testing.T) {
	ctx := context.Background()
	d := colleafListOriginExample(t)
	wire, err := encodeColleafDocument(ctx, d)
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
				copy.Evidence.ListOrigins[0].Version = 2
			case "digest":
				copy.Evidence.ListOrigins[0].Source.Element = "changed"
			case "duplicate":
				copy.Evidence.ListOrigins = append(copy.Evidence.ListOrigins, copy.Evidence.ListOrigins[0])
			case "missing":
				copy.Evidence.ListOrigins = nil
			}
			candidate := opened
			candidate.Resources = colleafReplaceResource(candidate.Resources, colleafDocumentResource, copy)
			if _, err = decodeColleafDocument(ctx, candidate); err == nil {
				t.Fatal("malformed list source accepted")
			}
		})
	}
	bullet := d.Headers[0]
	hash, err := colleafItemFingerprint(bullet, 0)
	if err != nil {
		t.Fatal(err)
	}
	bullet.Source = nil
	bullet.Items = append([]ListItem(nil), bullet.Items...)
	bullet.Items[0].Source = nil
	clean, err := colleafItemFingerprint(bullet, 0)
	if err != nil || hash != clean {
		t.Fatal("source-only edit changed item semantics", err)
	}
}
func FuzzColleafListOrigins(f *testing.F) {
	f.Add([]byte(`{"manydocListOrigin":{"profile":"manydoc.list-origin","version":2}}`))
	f.Add([]byte(sfAttrs(map[string]any{"manydocItemOrigin": colleafOrigin{Profile: colleafItemOriginProfile, Version: 1, Ref: strings.Repeat("0", 64)}})))
	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) > 32768 {
			return
		}
		snapshot := colleafNoteSnapshot{Body: colleafBody{Paragraphs: []colleafParagraphRecord{{Style: raw}}}}
		_ = visitColleafListOrigins(&snapshot, false, func(*colleafOrigin) error { return nil })
	})
}

func TestColleafListOriginsLostAndInheritedOwnership(t *testing.T) {
	ctx := context.Background()
	for _, mode := range []string{"bullet-removed", "inherited", "contradictory"} {
		t.Run(mode, func(t *testing.T) {
			source := colleafListOriginExample(t)
			d := &Document{Pages: source.Pages, Blocks: []Block{source.Headers[0]}}
			wire, err := encodeColleafDocument(ctx, d)
			if err != nil {
				t.Fatal(err)
			}
			props, err := colleafStyleObject(wire.Body.Paragraphs[0].Style)
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
				style, _ := colleafStyleObject(wire.DocumentStyle)
				style["defaultParagraphStyle"] = sfAttrs(map[string]json.RawMessage{"manydocListOrigin": marker})
				wire.DocumentStyle = sfAttrs(style)
			case "contradictory":
				origin, err := readColleafOriginProfile(props["manydocListOrigin"], colleafListOriginProfile)
				if err != nil {
					t.Fatal(err)
				}
				origin.Fingerprint = strings.Repeat("0", 64)
				props["manydocListOrigin"] = sfAttrs(origin)
				wire.Body.Paragraphs[0].Style = sfAttrs(props)
			}
			got, err := decodeColleafDocument(ctx, wire)
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
				if len(got.UnboundSources) != 2 || got.Blocks[0].Kind != Paragraph || got.Blocks[0].Source != nil || got.Blocks[0].colleafListOrigin != nil || got.Blocks[0].colleafItemOrigin != nil {
					t.Fatal("lost list asserted current ownership")
				}
			}
			if mode == "inherited" {
				if len(got.UnboundSources) != 1 || got.Blocks[0].Source != nil || got.Blocks[0].Items[0].Source.Historical {
					t.Fatal("inherited list source acquired ownership or staled item")
				}
				found := false
				for _, diagnostic := range got.Diagnostics {
					found = found || diagnostic.Code == "colleaf_list_origin_inheritance"
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
			next, err := encodeColleafDocument(ctx, model)
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
}
