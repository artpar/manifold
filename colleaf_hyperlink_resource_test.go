package manydoc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"testing"
)

func colleafHyperlinkExample(t testing.TB) *Document {
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
func colleafHyperlinkRecord(t testing.TB, body *colleafBody) *colleafRangeRecord {
	t.Helper()
	for i := range body.CustomRanges {
		if body.CustomRanges[i].Type == 0 {
			return &body.CustomRanges[i]
		}
	}
	t.Fatal("fixture lacks hyperlink")
	return nil
}
func TestColleafHyperlinkPluginSaveAndSF(t *testing.T) {
	ctx := context.Background()
	want := colleafHyperlinkExample(t)
	wire, err := encodeColleafDocument(ctx, want)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	index := func(body colleafBody) {
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
	if file := os.Getenv("MANYDOC_COLLEAF_HYPERLINK_QA"); file != "" {
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
	}{{"MANYDOC_COLLEAF_HYPERLINK_SAVED", false, false}, {"MANYDOC_COLLEAF_HYPERLINK_RESOURCE_EDITED", true, true}, {"MANYDOC_COLLEAF_HYPERLINK_EDITED_SAVED", true, false}} {
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
		before, _ := json.Marshal(native)
		got, err := decodeColleafDocument(ctx, native)
		if err != nil {
			t.Fatal(spec.env, err)
		}
		envelope, err := json.Marshal(colleafEnvelope{colleafEnvelopeType, 3, "univer", colleafEngineVersion, native})
		if err != nil {
			t.Fatal(err)
		}
		fromBytes, err := readColleaf(ctx, bytes.NewReader(envelope))
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
		expected := colleafHyperlinkExample(t)
		if spec.edited {
			expected.Headers[0].Runs[0].Link = "https://example.org/header-edited"
		}
		if spec.diagnostic {
			if len(got.Diagnostics) != 1 || got.Diagnostics[0].Code != "colleaf_hyperlink_resource_target" {
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
func TestColleafHyperlinkResourceScopePrecedenceAndImmutability(t *testing.T) {
	ctx := context.Background()
	original := colleafHyperlinkExample(t)
	wire, err := encodeColleafDocument(ctx, original)
	if err != nil {
		t.Fatal(err)
	}
	var headerID string
	for _, s := range wire.Headers {
		headerID = colleafHyperlinkRecord(t, &s.Body).ID
	}
	wire.Resources = append(wire.Resources, colleafResource{Name: colleafHyperlinkResource, Data: string(sfAttrs(map[string]any{"links": []any{map[string]string{"id": headerID, "payload": "https://example.org/header-edited"}}}))})
	before, _ := json.Marshal(wire)
	got, err := decodeColleafDocument(ctx, wire)
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
	if len(got.Diagnostics) != 1 || got.Diagnostics[0].Code != "colleaf_hyperlink_resource_target" {
		t.Fatal(got.Diagnostics)
	}
	// A resource-only URL is a real load input, not merely a redundant mirror.
	main := colleafHyperlinkRecord(t, &wire.Body)
	main.Properties = sfAttrs(map[string]string{"textStyleMode": "text"})
	wire.Resources[len(wire.Resources)-1].Data = string(sfAttrs(map[string]any{"links": []any{map[string]string{"id": main.ID, "payload": "https://example.org/recovered"}}}))
	recovered, err := decodeColleafDocument(ctx, wire)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Blocks[0].Runs[0].Link != "https://example.org/recovered" {
		t.Fatal("resource-only URL not recovered")
	}
	main.Properties = sfAttrs(map[string]string{"url": "https://example.org/main", "textStyleMode": "text"})
	wire.Resources[len(wire.Resources)-1].Data = `{"links":[{"id":"deleted","payload":"https://example.org/deleted"}]}`
	got, err = decodeColleafDocument(ctx, wire)
	if err != nil {
		t.Fatal(err)
	}
	if got.Blocks[0].Runs[0].Link != original.Blocks[0].Runs[0].Link || len(got.Diagnostics) != 1 || got.Diagnostics[0].Code != "colleaf_hyperlink_resource_orphan" {
		t.Fatal("inactive resource lost visible links", got.Diagnostics)
	}
}
func TestColleafHyperlinkResourceMalformedAndConflicting(t *testing.T) {
	ctx := context.Background()
	for _, data := range []string{``, `null`, `{}`, `{"links":null}`, `{"links":[null]}`, `{"links":[{"id":"x"}]}`, `{"links":[{"id":"","payload":""}]}`, `{"Links":[]}`, `{"links":[{"ID":"x","payload":""}]}`, `{"links":[{"id":"x","payload":3}]}`, `{"links":[{"id":"x","payload":"\ud800"}]}`, `{"links":[{"id":"x","payload":"a","\u0070ayload":"b"}]}`, `{"links":[{"id":"x","payload":"a"},{"id":"x","payload":"b"}]}`} {
		wire, err := encodeColleafDocument(ctx, colleafHyperlinkExample(t))
		if err != nil {
			t.Fatal(err)
		}
		wire.Resources = append(wire.Resources, colleafResource{Name: colleafHyperlinkResource, Data: data})
		if _, err = decodeColleafDocument(ctx, wire); err == nil {
			t.Fatal("malformed native hyperlink resource accepted", data)
		}
	}
	wire, err := encodeColleafDocument(ctx, colleafHyperlinkExample(t))
	if err != nil {
		t.Fatal(err)
	}
	wire.Resources = append(wire.Resources, colleafResource{Name: colleafHyperlinkResource, Data: `{"links":[]}`}, colleafResource{Name: colleafHyperlinkResource, Data: `{"links":[]}`})
	if _, err = decodeColleafDocument(ctx, wire); err == nil {
		t.Fatal("duplicate native resource accepted")
	}
	wire.Resources = wire.Resources[:len(wire.Resources)-1]
	main := colleafHyperlinkRecord(t, &wire.Body)
	for id, s := range wire.Headers {
		colleafHyperlinkRecord(t, &s.Body).ID = main.ID
		wire.Headers[id] = s
	}
	if _, err = decodeColleafDocument(ctx, wire); err == nil {
		t.Fatal("ambiguous cross-story link ID accepted")
	}
	ctx, cancel := context.WithCancel(ctx)
	cancel()
	if _, _, err = applyColleafHyperlinkResource(ctx, wire); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
func TestColleafHyperlinkResourceInternalPlaceholder(t *testing.T) {
	wire, err := encodeColleafDocument(context.Background(), colleafHyperlinkExample(t))
	if err != nil {
		t.Fatal(err)
	}
	a := colleafHyperlinkRecord(t, &wire.Body)
	a.Properties = sfAttrs(map[string]string{"bookmarkId": "target", "textStyleMode": "text"})
	wire.Resources = append(wire.Resources, colleafResource{Name: colleafHyperlinkResource, Data: string(sfAttrs(map[string]any{"links": []any{map[string]string{"id": a.ID, "payload": ""}}}))})
	if _, _, err = applyColleafHyperlinkResource(context.Background(), wire); err != nil {
		t.Fatal(err)
	}
	wire.Resources[len(wire.Resources)-1].Data = string(sfAttrs(map[string]any{"links": []any{map[string]string{"id": a.ID, "payload": "https://example.org/conflict"}}}))
	if _, _, err = applyColleafHyperlinkResource(context.Background(), wire); !errors.Is(err, ErrUnsupported) {
		t.Fatal("conflicting internal/external targets accepted", err)
	}
}
func FuzzColleafHyperlinkResource(f *testing.F) {
	for _, data := range []string{`{"links":[]}`, `{"links":[{"id":"missing","payload":"https://example.org/"}]}`, `{"links":[{"id":"missing","payload":"\ud800"}]}`} {
		f.Add(data)
	}
	f.Fuzz(func(t *testing.T, data string) {
		wire, err := encodeColleafDocument(context.Background(), colleafHyperlinkExample(t))
		if err != nil {
			t.Fatal(err)
		}
		wire.Resources = append(wire.Resources, colleafResource{Name: colleafHyperlinkResource, Data: data})
		got, err := decodeColleafDocument(context.Background(), wire)
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
