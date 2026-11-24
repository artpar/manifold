package manydoc

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestColleafUTF16Boundaries(t *testing.T) {
	index, err := newColleafTextIndex(context.Background(), "A😀B\r\n")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(index.byteAt, []int{0, 1, -1, 5, 6, 7, 8}) {
		t.Fatal("UTF-16 offsets confused with bytes/runes", index.byteAt)
	}
	if text, err := index.exclusive(1, 3); err != nil || text != "😀" {
		t.Fatal(text, err)
	}
	if text, err := index.inclusive(1, 2); err != nil || text != "😀" {
		t.Fatal(text, err)
	}
	for _, rangeTest := range [][2]int{{-1, 1}, {1, 2}, {2, 3}, {5, 4}, {0, 7}} {
		if _, err := index.exclusive(rangeTest[0], rangeTest[1]); err == nil {
			t.Fatal("invalid boundary accepted", rangeTest)
		}
	}
	if _, err := index.inclusive(1, 1); err == nil {
		t.Fatal("inclusive range split a surrogate pair")
	}
	if _, err := newColleafTextIndex(context.Background(), string([]byte{0xff})); err == nil {
		t.Fatal("invalid UTF-8 accepted")
	}
}
func colleafWireFixture(t *testing.T) colleafBody {
	t.Helper()
	data, err := os.ReadFile("testdata/colleaf/native-stream.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Document struct {
			Body colleafBody `json:"body"`
		} `json:"document"`
	}
	if err = json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	return fixture.Document.Body
}
func TestColleafRichNativeTokenGrammarAndMetadata(t *testing.T) {
	body := colleafWireFixture(t)
	index, err := indexColleafBody(context.Background(), body)
	if err != nil {
		t.Fatal(err)
	}
	if len(index.sections) != 1 || len(index.tables) != 2 || len(index.blocks) != 1 {
		t.Fatal("rich native structure flattened", index)
	}
	for _, table := range body.Tables {
		if text, err := index.text.exclusive(table.StartIndex, table.EndIndex); err != nil || text[0] != colleafTableStart || text[len(text)-1] != colleafTableEnd {
			t.Fatal("table extent changed", table, err)
		}
	}
	if text, err := index.text.inclusive(body.CustomRanges[0].StartIndex, body.CustomRanges[0].EndIndex); err != nil || text != "😀" {
		t.Fatal("native inclusive annotation extent", text, err)
	}
	if output := os.Getenv("MANYDOC_COLLEAF_QA_TREE"); output != "" {
		var proof func([]colleafStreamNode) []map[string]any
		proof = func(nodes []colleafStreamNode) []map[string]any {
			out := make([]map[string]any, 0, len(nodes))
			for _, node := range nodes {
				out = append(out, map[string]any{"kind": node.kind, "start": node.start, "end": node.end, "children": proof(node.children)})
			}
			return out
		}
		data, err := json.MarshalIndent(proof(index.sections), "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(output, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
}
func TestColleafMalformedNativeBoundsAndTokens(t *testing.T) {
	for _, change := range []func(*colleafBody){
		func(b *colleafBody) { b.TextRuns[0].Ed = 2 },
		func(b *colleafBody) { b.TextRuns[0].St = 4; b.TextRuns[0].Ed = 3 },
		func(b *colleafBody) { b.Paragraphs[0].StartIndex = 2 },
		func(b *colleafBody) { b.SectionBreaks[0].StartIndex = 0 },
		func(b *colleafBody) { b.Tables[0].EndIndex-- },
		func(b *colleafBody) { b.Tables = b.Tables[:1] },
		func(b *colleafBody) { b.CustomBlocks = nil },
		func(b *colleafBody) { b.CustomRanges[0].EndIndex = 1 },
		func(b *colleafBody) { b.RenderedPageBreaks = []int{0} },
		func(b *colleafBody) { b.DataStream = strings.Replace(b.DataStream, "\x0e", "\x1e", 1) },
		func(b *colleafBody) { b.DataStream = strings.Replace(b.DataStream, "\r\n\x1d", "\r\x1d", 1) },
	} {
		body := colleafWireFixture(t)
		change(&body)
		if _, err := indexColleafBody(context.Background(), body); err == nil {
			t.Fatal("contradictory native offsets/structure accepted")
		}
	}
	for _, text := range []string{"", "plain", "x\n", "\x1cA\r\n\x1d\r\n", "\x1a\x1b\x0e\x0f\r\n", "\x1fA\r\n"} {
		if _, err := parseColleafStream(context.Background(), text); err == nil {
			t.Fatal("malformed native grammar accepted", text)
		}
	}
}

func TestColleafStreamCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := indexColleafBody(ctx, colleafWireFixture(t)); err != context.Canceled {
		t.Fatal("native validation ignored cancellation", err)
	}
}

func TestColleafCrossParagraphRangesAndSoftBreakPositions(t *testing.T) {
	body := colleafBody{DataStream: "\x1fA\rB\x1e\fC\r\n", RenderedPageBreaks: []int{5}}
	if _, err := indexColleafBody(context.Background(), body); err != nil {
		t.Fatal("valid spanning markers/soft break rejected", err)
	}
	index, err := newColleafTextIndex(context.Background(), "a\u0301😀")
	if err != nil || index.length() != 4 {
		t.Fatal("input normalized before offset interpretation", index, err)
	}
	if text, err := index.exclusive(0, 2); err != nil || text != "a\u0301" {
		t.Fatal("combining scalars rewritten", text, err)
	}
}

func FuzzColleafStream(f *testing.F) {
	raw, err := os.ReadFile("testdata/colleaf/native-stream.json")
	if err != nil {
		f.Fatal(err)
	}
	var fixture struct {
		Document struct {
			Body colleafBody `json:"body"`
		} `json:"document"`
	}
	if err := json.Unmarshal(raw, &fixture); err != nil {
		f.Fatal(err)
	}
	for _, seed := range []string{"\r\n", "A😀B\r\n", fixture.Document.Body.DataStream, "\x1a\x1b\x1cA\r\n\x1d\x1e\x1f\r\n"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, text string) {
		if len(text) > 1<<20 {
			t.Skip()
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_, _ = parseColleafStream(ctx, text)
	})
}

func TestColleafNativeOverlappingStyleIntervals(t *testing.T) {
	body := colleafBody{DataStream: "ABCD\r\n", TextRuns: []colleafTextRun{{St: 1, Ed: 4, TS: json.RawMessage(`{"it":1}`)}, {St: 0, Ed: 3, TS: json.RawMessage(`{"bl":1}`)}, {St: 4, Ed: 4}}}
	if _, err := indexColleafBody(context.Background(), body); err != nil {
		t.Fatal("valid native overlapping/unsorted/empty insertion styles rejected", err)
	}
}
