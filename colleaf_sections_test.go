package manydoc

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

func colleafSectionExample(t testing.TB) *Document {
	p := func(s string) Block { return Block{Kind: Paragraph, Runs: []Run{{Text: s}}} }
	story := func(s string) *RunningStory { return &RunningStory{Blocks: []Block{p(s)}} }
	picture := Block{Kind: Picture, Image: &Image{Data: runningPNG(t, false), MediaType: "image/png", Alt: "Header logo", Width: 24, Height: 12}}
	return &Document{Metadata: Metadata{Title: "Native sections", Author: "Manydoc", Subject: "Page and running story ownership"}, Layout: Layout{Width: 450.125, Height: 600.5, Margins: &PageMargins{Top: 40.125, Right: 30, Bottom: 40, Left: 30}, HeaderDistance: colleafFloat(12), FooterDistance: colleafFloat(9)}, EvenAndOddHeaders: true, Blocks: []Block{
		{Kind: SectionBlock, Section: &Section{FirstPage: true, Headers: RunningStories{Default: &RunningStory{Blocks: []Block{p("Odd header"), picture, {Kind: Table, ColumnWidths: []float64{72, 72}, Rows: []Row{{Cells: []Cell{{ColSpan: 2, Blocks: []Block{p("Header table")}}}}}}, {Kind: ListBlock, List: "bullet", Items: []ListItem{{Blocks: []Block{p("Header bullet")}}}}}}, First: story("First header"), Even: story("Even header")}, Footers: RunningStories{Default: story("Footer")}, Blocks: []Block{p("First body 😀"), {Kind: PageBreak}, p("First even body"), {Kind: PageBreak}, p("First odd body")}}},
		{Kind: SectionBlock, Section: &Section{Start: SectionNextPage, Headers: RunningStories{Default: &RunningStory{}}, Blocks: []Block{p("Second body"), {Kind: PageBreak}, p("Second next page")}}},
		{Kind: SectionBlock, Section: &Section{Start: SectionOddPage, Layout: &Layout{Width: 400, Height: 550, Margins: &PageMargins{Top: 0, Right: 0, Bottom: 0, Left: 0}, HeaderDistance: colleafFloat(0), FooterDistance: colleafFloat(0)}, Headers: RunningStories{Default: story("Third header")}, Footers: RunningStories{Default: &RunningStory{}}, Blocks: []Block{p("Third body")}}},
	}}
}
func TestColleafDocumentSectionsStoriesAndSFRoundTrip(t *testing.T) {
	ctx := context.Background()
	want := colleafSectionExample(t)
	wire, err := encodeColleafDocument(ctx, want)
	if err != nil {
		t.Fatal(err)
	}
	got, err := decodeColleafDocument(ctx, wire)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		a, _ := json.Marshal(got)
		b, _ := json.Marshal(want)
		t.Fatalf("section semantics lost\ngot%s\nwant%s", a, b)
	}
	sf, err := ToStructured(got)
	if err != nil {
		t.Fatal(err)
	}
	model, err := FromStructured(sf)
	if err != nil {
		t.Fatal(err)
	}
	again, err := encodeColleafDocument(ctx, model)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(wire, again) {
		t.Fatal("SF changed native section/story sources")
	}
	if file := os.Getenv("MANYDOC_COLLEAF_SECTION_QA"); file != "" {
		data, err := json.MarshalIndent(wire, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(file, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if file := os.Getenv("MANYDOC_COLLEAF_SECTION_SAVED"); file != "" {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var saved colleafNoteSnapshot
		if err = json.Unmarshal(data, &saved); err != nil {
			t.Fatal(err)
		}
		got, err = decodeColleafDocument(ctx, saved)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatal("facade changed sections/stories", got.Diagnostics)
		}
	}
}
func TestColleafPlainDocumentLayoutMetadataRunningStories(t *testing.T) {
	d := &Document{Metadata: Metadata{Author: "Author", Subject: "Subject"}, Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "Body"}}}}, Headers: []Block{{Kind: Paragraph, Runs: []Run{{Text: "Header"}}}}, Footers: []Block{{Kind: Paragraph, Runs: []Run{{Text: "Footer"}}}}}
	wire, err := encodeColleafDocument(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	got, err := decodeColleafDocument(context.Background(), wire)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, d) {
		a, _ := json.Marshal(got)
		b, _ := json.Marshal(d)
		t.Fatalf("plain doc changed\ngot%s\nwant%s", a, b)
	}
}
func TestColleafNativeLayoutEditsWinAndStoryReferencesValidate(t *testing.T) {
	d := colleafSectionExample(t)
	wire, err := encodeColleafDocument(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	var style colleafPageProperties
	_ = json.Unmarshal(wire.DocumentStyle, &style)
	style.PageSize.Width = 800
	wire.DocumentStyle = sfAttrs(style)
	got, err := decodeColleafDocument(context.Background(), wire)
	if err != nil {
		t.Fatal(err)
	}
	if got.Layout.Width != 600 || len(got.Diagnostics) != 1 || got.Diagnostics[0].Code != "colleaf_stale_layout" {
		t.Fatal("native paper edit lost", got.Layout, got.Diagnostics)
	}
	wire, err = encodeColleafDocument(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	delete(wire.Headers, *wire.Body.SectionBreaks[0].DefaultHeader)
	if _, err = decodeColleafDocument(context.Background(), wire); err == nil {
		t.Fatal("missing running body accepted")
	}
}

func TestColleafNativeSectionPolicies(t *testing.T) {
	ctx := context.Background()
	makeWire := func(t *testing.T) colleafNoteSnapshot {
		t.Helper()
		wire, err := encodeColleafDocument(ctx, colleafSectionExample(t))
		if err != nil {
			t.Fatal(err)
		}
		return wire
	}
	for name, mutate := range map[string]func(*colleafNoteSnapshot){
		"orphan local table": func(w *colleafNoteSnapshot) {
			id := *w.Body.SectionBreaks[0].DefaultHeader
			h := w.Headers[id]
			h.Tables = map[string]json.RawMessage{"lost": json.RawMessage(`{}`)}
			w.Headers[id] = h
		},
		"orphan local drawing": func(w *colleafNoteSnapshot) {
			id := *w.Body.SectionBreaks[0].DefaultHeader
			h := w.Headers[id]
			h.Drawings = map[string]json.RawMessage{"lost": json.RawMessage(`{}`)}
			w.Headers[id] = h
		},
		"orphan header": func(w *colleafNoteSnapshot) {
			w.Headers["orphan"] = colleafRunningStory{HeaderID: "orphan", Body: colleafBody{DataStream: "Lost\r\n"}}
		},
		"wrong story identity": func(w *colleafNoteSnapshot) {
			id := *w.Body.SectionBreaks[0].DefaultHeader
			h := w.Headers[id]
			h.HeaderID = "wrong"
			w.Headers[id] = h
		},
		"next column":        func(w *colleafNoteSnapshot) { w.Body.SectionBreaks[0].Start = colleafInt(2) },
		"section even mode":  func(w *colleafNoteSnapshot) { w.Body.SectionBreaks[0].EvenOdd = colleafInt(0) },
		"invalid first flag": func(w *colleafNoteSnapshot) { w.Body.SectionBreaks[0].FirstPage = colleafInt(2) },
		"negative page": func(w *colleafNoteSnapshot) {
			w.Body.SectionBreaks[0].PageSize = &colleafImageSize{Width: -1, Height: 100}
		},
		"note settings": func(w *colleafNoteSnapshot) { w.NoteSettings = json.RawMessage(`{"footnoteNumberStart":7}`) },
		"section note settings": func(w *colleafNoteSnapshot) {
			w.Body.SectionBreaks[0].Extra = map[string]json.RawMessage{"noteProperties": json.RawMessage(`{"numberStart":7}`)}
		},
		"unpaginated canvas": func(w *colleafNoteSnapshot) { w.DocumentStyle = json.RawMessage(`{}`) },
	} {
		t.Run(name, func(t *testing.T) {
			wire := makeWire(t)
			mutate(&wire)
			if _, err := decodeColleafDocument(ctx, wire); err == nil {
				t.Fatal("unmapped/contradictory source accepted")
			}
		})
	}
	wire := makeWire(t)
	opened, err := openColleafIntent(ctx, wire)
	if err != nil {
		t.Fatal(err)
	}
	opened.Resources = nil // A genuine foreign document has no manydoc layout intent.
	got, err := decodeColleafDocument(ctx, opened)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Blocks) != 3 || got.Blocks[0].Kind != SectionBlock || got.Blocks[0].Section.Layout == nil || !got.Blocks[0].Section.FirstPage {
		t.Fatal("foreign section properties flattened")
	}
	wire = makeWire(t)
	wire.Body.SectionBreaks[0].Extra = map[string]json.RawMessage{"columnProperties": json.RawMessage(`{"columnCount":2}`)}
	got, err = decodeColleafDocument(ctx, wire)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Diagnostics) != 1 || got.Diagnostics[0].Code != "colleaf_section_style" {
		t.Fatal("unmapped column layout not disclosed", got.Diagnostics)
	}
}

func TestColleafAuthoredAndRenderedPageBreaks(t *testing.T) {
	for _, soft := range []bool{false, true} {
		body := colleafBody{DataStream: "A😀\fB\r\n", Paragraphs: []colleafParagraphRecord{{StartIndex: 5}}, SectionBreaks: []colleafSectionRecord{{StartIndex: 6}}}
		if soft {
			body.RenderedPageBreaks = []int{3}
		}
		got, err := decodeColleafBody(context.Background(), body, colleafTextContext{}, nil)
		if err != nil {
			t.Fatal(err)
		}
		if soft {
			if len(got.Blocks) != 1 || len(got.Diagnostics) != 1 || got.Diagnostics[0].Code != "colleaf_soft_page_break" {
				t.Fatal("soft break not disclosed/joined", got)
			}
			var text string
			for _, run := range got.Blocks[0].Runs {
				text += run.Text
			}
			if text != "A😀B" {
				t.Fatal(text)
			}
		} else if len(got.Blocks) != 3 || got.Blocks[1].Kind != PageBreak || got.Blocks[0].Runs[0].Text != "A😀" || got.Blocks[2].Runs[0].Text != "B" {
			t.Fatal("authored page boundary lost", got)
		}
	}
}

func TestColleafSectionsShareNotes(t *testing.T) {
	ctx := context.Background()
	d := colleafSectionExample(t)
	notes := colleafNoteExample()
	d.Notes = notes.Notes
	d.Blocks[1].Section.Blocks = append(d.Blocks[1].Section.Blocks, notes.Blocks...)
	wire, err := encodeColleafDocument(ctx, d)
	if err != nil {
		t.Fatal(err)
	}
	if wire.Notes["manydoc-note-1"].Mark == nil || *wire.Notes["manydoc-note-1"].Mark != "1" || wire.Notes["manydoc-note-2"].Mark == nil || *wire.Notes["manydoc-note-2"].Mark != "1" {
		t.Fatal("later-section automatic labels not projected")
	}
	got, err := decodeColleafDocument(ctx, wire)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, d) {
		t.Fatal("notes in later sections lost", got.Diagnostics)
	}
	id := *wire.Body.SectionBreaks[0].DefaultHeader
	h := wire.Headers[id]
	h.Tables = map[string]json.RawMessage{"lost": json.RawMessage(`{}`)}
	wire.Headers[id] = h
	if _, err = decodeColleafDocument(ctx, wire); err == nil {
		t.Fatal("orphan local table silently dropped")
	}
}

func FuzzColleafDocument(f *testing.F) {
	wire, err := encodeColleafDocument(context.Background(), colleafSectionExample(f))
	if err != nil {
		f.Fatal(err)
	}
	data, err := json.Marshal(wire)
	if err != nil {
		f.Fatal(err)
	}
	f.Add(data)
	f.Add([]byte(`{"documentStyle":{"documentFlavor":1},"body":{"dataStream":"Text\r\n"}}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		var input colleafNoteSnapshot
		if json.Unmarshal(data, &input) != nil {
			return
		}
		doc, err := decodeColleafDocument(context.Background(), input)
		if err == nil {
			if err = doc.Validate(); err != nil {
				t.Fatal(err)
			}
		}
	})
}
