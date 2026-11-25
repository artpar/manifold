package manifold

import (
	"context"
	"encoding/json"
	"errors"
	"math/rand"
	"os"
	"reflect"
	"testing"
)

func TestColleafStyleSweepMatchesLastWholeRecord(t *testing.T) {
	ctx := context.Background()
	index, err := newColleafTextIndex(ctx, "abcdefghij\r\n")
	if err != nil {
		t.Fatal(err)
	}
	random := rand.New(rand.NewSource(417))
	for trial := 0; trial < 300; trial++ {
		runs := make([]colleafTextRun, random.Intn(40))
		for i := range runs {
			a, b := random.Intn(13), random.Intn(13)
			if a > b {
				a, b = b, a
			}
			runs[i] = colleafTextRun{St: a, Ed: b}
		}
		spans, err := colleafStyleSpans(ctx, index, runs)
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

func colleafTextExample() []Block {
	return []Block{
		{Kind: Heading, Level: 6, Direction: "rtl", PageBreakBefore: true, Runs: []Run{{Text: "Title 😀", Bold: true, Language: "en-US", FontFamily: "Go", FontSize: 16, Color: "#123ABC"}}},
		{Kind: Paragraph, Runs: []Run{{Text: "Link 😀", Italic: true, Underline: true, Link: "https://example.org", FontSize: 12}, {Text: "\nnext\tcell", Strike: true, Destination: "destination", Color: "auto"}}},
		{Kind: Paragraph},
	}
}
func TestColleafTextSemanticRoundTripAndNativeEdits(t *testing.T) {
	ctx := context.Background()
	want := colleafTextExample()
	body, err := encodeColleafTextBody(ctx, want)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := decodeColleafTextBody(ctx, body, colleafTextContext{})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(doc.Blocks, want) {
		t.Fatalf("semantic difference\ngot%+v\nwant%+v", doc.Blocks, want)
	}
	// Cross the actual canonical SF, with the missing internal target declared.
	doc.UnresolvedDestinations = []string{"destination"}
	sf, err := ToStructured(doc)
	if err != nil {
		t.Fatal(err)
	}
	throughSF, err := FromStructured(sf)
	if err != nil {
		t.Fatal(err)
	}
	sfBody, err := encodeColleafTextBody(ctx, throughSF.Blocks)
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
	if file := os.Getenv("MANYDOC_COLLEAF_TEXT_QA"); file != "" {
		data, err := json.MarshalIndent(map[string]any{"id": "manydoc-text-qa", "title": "Native text authority", "body": body, "documentStyle": map[string]any{"documentFlavor": 1}}, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(file, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	// Native visible text and the last native style record remain authoritative.
	body.DataStream = "Edited😀" + body.DataStream[len("Title 😀"):]
	body.TextRuns = append(body.TextRuns, colleafTextRun{St: 0, Ed: 8, TS: sfAttrs(map[string]any{"it": 1})})
	doc, err = decodeColleafTextBody(ctx, body, colleafTextContext{})
	if err != nil {
		t.Fatal(err)
	}
	if got := doc.Blocks[0].Runs[0]; got.Text != "Edited😀" || !got.Italic || got.Bold || got.FontSize != 0 || got.Language != "" {
		t.Fatalf("stale text/style authority %+v", got)
	}
	if file := os.Getenv("MANYDOC_COLLEAF_TEXT_EDIT"); file != "" {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var snapshot struct {
			Body colleafBody `json:"body"`
		}
		if err = json.Unmarshal(data, &snapshot); err != nil {
			t.Fatal(err)
		}
		doc, err = decodeColleafTextBody(ctx, snapshot.Body, colleafTextContext{})
		if err != nil {
			t.Fatal(err)
		}
		if got := doc.Blocks[0].Runs[0]; got.Text != "Edited😀" || !got.Italic || got.Bold {
			t.Fatalf("SDK saved edit lost %+v", got)
		}
	}
}

func TestColleafInheritedStylesAndLossDiagnostics(t *testing.T) {
	body := colleafBody{DataStream: "ABCD\r\n", Paragraphs: []colleafParagraphRecord{{StartIndex: 4, StyleID: "child", Style: sfAttrs(map[string]any{"direction": 2, "spaceAbove": map[string]int{"v": 12, "u": 0}, "textStyle": map[string]int{"it": 0}})}}, TextRuns: []colleafTextRun{{St: 1, Ed: 4, TS: sfAttrs(map[string]int{"bl": 1})}, {St: 0, Ed: 3, StyleID: "character", TS: sfAttrs(map[string]int{"bl": 0})}}}
	settings := colleafTextContext{DefaultText: sfAttrs(map[string]any{"ff": "Go", "fs": 11, "it": 1}), Styles: map[string]colleafNamedStyle{
		"base":      {TextStyle: sfAttrs(map[string]int{"bl": 1}), ParagraphStyle: sfAttrs(map[string]int{"outlineLevel": 2})},
		"child":     {BasedOn: "base", TextStyle: sfAttrs(map[string]any{"cl": map[string]string{"rgb": "rgb(1, 2, 255)"}})},
		"character": {TextStyle: sfAttrs(map[string]int{"st": 1})},
	}}
	// A decoration is an object, not a BooleanNumber: malformed input must fail.
	if _, err := decodeColleafTextBody(context.Background(), body, settings); err == nil {
		t.Fatal("invalid decoration accepted")
	}
	settings.Styles["character"] = colleafNamedStyle{TextStyle: sfAttrs(map[string]any{"st": map[string]int{"s": 1}})}
	doc, err := decodeColleafTextBody(context.Background(), body, settings)
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
	if len(doc.Diagnostics) != 1 || doc.Diagnostics[0].Code != "colleaf_paragraph_style" {
		t.Fatal(doc.Diagnostics)
	}
	resolver := colleafStyleResolver{named: map[string]colleafNamedStyle{"a": {BasedOn: "b"}, "b": {BasedOn: "a"}}}
	if _, err = resolver.resolve("a", 0); err == nil {
		t.Fatal("style cycle accepted")
	}
	if _, err = resolver.resolve("missing", 0); err == nil {
		t.Fatal("missing style accepted")
	}
}

func TestColleafTextRejectsUnmappedAndMalformedContent(t *testing.T) {
	ctx := context.Background()
	for _, raw := range []string{"null", "true", "2", "1.0", "\"1\""} {
		if _, err := colleafBool(json.RawMessage(raw)); err == nil {
			t.Fatal("invalid boolean", raw)
		}
	}
	for _, style := range []string{`{"fs":0}`, `{"ul":{"s":null}}`, `{"cl":{"th":1}}`, `{"hidden":true}`} {
		props, err := colleafStyleObject(json.RawMessage(style))
		if err != nil {
			t.Fatal(err)
		}
		if _, err = colleafStyleRun(props, &Document{}); err == nil {
			t.Fatal("invalid/unmapped style", style)
		}
	}
	for _, blocks := range [][]Block{{{Kind: Table}}, {{Kind: Paragraph, Runs: []Run{{Text: "A", Note: &NoteReference{}}}}}, {{Kind: Paragraph, Runs: []Run{{Text: "A\rB"}}}}} {
		if _, err := encodeColleafTextBody(ctx, blocks); !errors.Is(err, ErrUnsupported) {
			t.Fatal("partial encoding", err)
		}
	}
	for _, body := range []colleafBody{
		{DataStream: "A\r\nB\r\n"},
		{DataStream: "A\r\n", Paragraphs: []colleafParagraphRecord{{StartIndex: 1, Style: json.RawMessage(`{"outlineLevel":null}`)}}},
		{DataStream: "A\r\n", CustomRanges: []colleafRangeRecord{{StartIndex: 0, EndIndex: 0, ID: "a", Type: 4}}},
		{DataStream: "A\r\n", CustomRanges: []colleafRangeRecord{{StartIndex: 0, EndIndex: 0, ID: "a"}, {StartIndex: 0, EndIndex: 0, ID: "b"}}},
	} {
		if _, err := decodeColleafTextBody(ctx, body, colleafTextContext{}); err == nil {
			t.Fatal("partial/malformed decoding", body)
		}
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := encodeColleafTextBody(canceled, colleafTextExample()); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
