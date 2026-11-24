package manydoc

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

func colleafListExample() *Document {
	p := func(s string) Block { return Block{Kind: Paragraph, Runs: []Run{{Text: s}}} }
	nested := Block{Kind: ListBlock, List: "number", Start: listStartPointer(4), Numbering: &ListNumbering{Level: 1, Template: "%1.%2)", Formats: []string{"upperLetter", "lowerRoman"}, Ancestors: []int{99}}, Items: []ListItem{{Blocks: []Block{p("Child four 😀")}}, {Blocks: []Block{p("Child five")}}}}
	independent := Block{Kind: ListBlock, List: "number", Numbering: &ListNumbering{Level: 1, Template: "%1.%2", Formats: []string{"decimal", "decimal"}, Ancestors: []int{8}, Independent: true}, Items: []ListItem{{Blocks: []Block{p("Independent")}}}}
	table := Block{Kind: Table, Rows: []Row{{Cells: []Cell{{ColSpan: 2, Blocks: []Block{p("Owned merged table")}}}}}}
	return &Document{Blocks: []Block{
		{Kind: ListBlock, List: "number", Start: listStartPointer(27), Numbering: &ListNumbering{Template: "Appendix %1.", Formats: []string{"upperLetter"}}, Items: []ListItem{{Blocks: []Block{p("Parent one"), p("Continuation"), nested, table}}, {Blocks: []Block{p("Parent two"), independent}}}},
		{Kind: ListBlock, List: "number", Start: listStartPointer(2), Items: []ListItem{{Blocks: []Block{p("Separate two")}}, {Blocks: []Block{p("Separate three")}}}},
		{Kind: ListBlock, List: "bullet", Items: []ListItem{{Blocks: []Block{p("Bullet"), {Kind: ListBlock, List: "number", Start: listStartPointer(0), Items: []ListItem{{Blocks: []Block{p("Zero")}}}}}}}},
		{Kind: Paragraph, Runs: []Run{{Text: "Note"}, {Note: &NoteReference{Kind: Footnote, ID: "n"}}}},
	}, Notes: []Note{{Kind: Footnote, ID: "n", Blocks: []Block{{Kind: ListBlock, List: "number", Start: listStartPointer(0), Items: []ListItem{{Blocks: []Block{p("Note zero")}}, {Blocks: []Block{p("Note one")}}}}}}}}
}
func TestColleafRichListOwnershipNumberingAndSF(t *testing.T) {
	ctx := context.Background()
	original := colleafListExample()
	want := resolveNumberingDocument(original)
	wire, err := encodeColleafNotes(ctx, want)
	if err != nil {
		t.Fatal(err)
	}
	got, err := decodeColleafNotes(ctx, wire, colleafTextContext{})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		a, _ := json.Marshal(want)
		b, _ := json.Marshal(got)
		t.Fatalf("list semantics differ\nwant%s\ngot%s", a, b)
	}
	sf, err := ToStructured(got)
	if err != nil {
		t.Fatal(err)
	}
	model, err := FromStructured(sf)
	if err != nil {
		t.Fatal(err)
	}
	wire2, err := encodeColleafNotes(ctx, model)
	if err != nil {
		t.Fatal(err)
	}
	if original.Blocks[0].Items[0].Blocks[2].Numbering.Ancestors[0] != 99 {
		t.Fatal("caller numbering mutated")
	}
	if !reflect.DeepEqual(wire, wire2) {
		t.Fatal("SF changed native list source/ownership records")
	}
	if file := os.Getenv("MANYDOC_COLLEAF_LIST_QA"); file != "" {
		data, err := json.MarshalIndent(wire, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(file, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if file := os.Getenv("MANYDOC_COLLEAF_LIST_EDIT"); file != "" {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var edited colleafNoteSnapshot
		if err = json.Unmarshal(data, &edited); err != nil {
			t.Fatal(err)
		}
		doc, err := decodeColleafNotes(ctx, edited, colleafTextContext{})
		if err != nil {
			t.Fatal(err)
		}
		if noteLabelContentText(doc.Blocks[0].Items[0].Blocks[0].Runs) != "Edited one" {
			t.Fatal("native text edit lost", doc.Blocks[0])
		}
		if len(doc.Blocks[0].Items[0].Blocks) != 4 || doc.Blocks[0].Items[0].Blocks[3].Kind != Table {
			t.Fatal("native save lost continuation ownership")
		}
	}
}
func TestColleafNativeListsWithoutOwnershipKeepActualBullets(t *testing.T) {
	ctx := context.Background()
	wire, err := encodeColleafNotes(ctx, colleafListExample())
	if err != nil {
		t.Fatal(err)
	}
	wire, err = openColleafIntent(ctx, wire)
	if err != nil {
		t.Fatal(err)
	}
	var keep []colleafResource
	for _, r := range wire.Resources {
		if r.Name != colleafListResource {
			keep = append(keep, r)
		}
	}
	wire.Resources = keep
	doc, err := decodeColleafNotes(ctx, wire, colleafTextContext{})
	if err != nil {
		t.Fatal(err)
	}
	if doc.Blocks[0].Kind != ListBlock || doc.Blocks[0].Start == nil || *doc.Blocks[0].Start != 27 || doc.Blocks[1].Runs[0].Text != "Continuation" {
		t.Fatal(doc.Blocks)
	}
	found := false
	for _, d := range doc.Diagnostics {
		if d.Code == "colleaf_list_ownership" {
			found = true
		}
	}
	if !found {
		t.Fatal("unrepresented continuation ownership was not diagnosed")
	}
}
func TestColleafStaleOwnershipDoesNotOverrideNativeRestart(t *testing.T) {
	wire, err := encodeColleafNotes(context.Background(), colleafListExample())
	if err != nil {
		t.Fatal(err)
	}
	// Change the first native list's second item to an explicit restart.
	for i, p := range wire.Body.Paragraphs {
		var b colleafBullet
		if len(p.Bullet) == 0 {
			continue
		}
		if err = json.Unmarshal(p.Bullet, &b); err != nil {
			t.Fatal(err)
		}
		if b.Level == 0 && b.Start == nil {
			restart := 8
			b.Start = &restart
			wire.Body.Paragraphs[i].Bullet = sfAttrs(b)
			break
		}
	}
	doc, err := decodeColleafNotes(context.Background(), wire, colleafTextContext{})
	if err != nil {
		t.Fatal(err)
	}
	foundRestart, foundLoss := false, false
	for _, b := range doc.Blocks {
		if b.Kind == ListBlock && b.Start != nil && *b.Start == 9 {
			foundRestart = true
		}
	}
	for _, d := range doc.Diagnostics {
		if d.Code == "colleaf_stale_list_ownership" {
			foundLoss = true
		}
	}
	if !foundRestart || !foundLoss {
		t.Fatal("stale list metadata won over native restart", doc.Blocks, doc.Diagnostics)
	}
}

func TestColleafListTokenProjectionAndNativeEdits(t *testing.T) {
	d := &Document{Blocks: []Block{{Kind: ListBlock, List: "number", Numbering: &ListNumbering{Template: "%1.%9", Formats: []string{"decimal"}}, Items: []ListItem{{Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "Kept"}}}}}}}}}
	wire, err := encodeColleafNotes(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range wire.Lists {
		var source colleafNativeList
		if err = json.Unmarshal(raw, &source); err != nil {
			t.Fatal(err)
		}
		if source.Levels[0].Format != "%1." {
			t.Fatal("absent native level left dereferenceable", source)
		}
	}
	got, err := decodeColleafNotes(context.Background(), wire, colleafTextContext{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Blocks[0].Numbering.Template != "%1.%9" {
		t.Fatal("ignored-placeholder intent lost")
	}
	for key, raw := range wire.Lists {
		var source colleafNativeList
		_ = json.Unmarshal(raw, &source)
		source.Levels[0].Format = "Edited %1"
		wire.Lists[key] = sfAttrs(source)
	}
	got, err = decodeColleafNotes(context.Background(), wire, colleafTextContext{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Blocks[0].Numbering.Template != "Edited %1" {
		t.Fatal("stale intent overrode native edit")
	}
	d.Blocks[0].Numbering.Template = "%%1."
	if _, err = encodeColleafNotes(context.Background(), d); err != nil {
		t.Fatal("literal percent projection rejected", err)
	}
}
func FuzzColleafLists(f *testing.F) {
	wire, err := encodeColleafNotes(context.Background(), colleafListExample())
	if err != nil {
		f.Fatal(err)
	}
	data, err := json.Marshal(wire)
	if err != nil {
		f.Fatal(err)
	}
	f.Add(data)
	f.Add([]byte(`{"body":{"dataStream":"X\r\n","paragraphs":[{"startIndex":1,"bullet":{"listType":"t","listId":"l","nestingLevel":0}}]},"lists":{"t":{"listType":"t","nestingLevel":[{"glyphFormat":"%1.","glyphType":2,"startNumber":0,"bulletAlignment":1}]}}}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		var input colleafNoteSnapshot
		if json.Unmarshal(data, &input) != nil {
			return
		}
		doc, err := decodeColleafNotes(context.Background(), input, colleafTextContext{})
		if err == nil {
			if err = doc.Validate(); err != nil {
				t.Fatal("accepted invalid list/note graph", err)
			}
		}
	})
}

func TestColleafNondecimalZeroNeverSilentlyChangesMarker(t *testing.T) {
	for _, format := range []string{"decimalZero", "upperLetter", "lowerLetter", "upperRoman", "lowerRoman"} {
		d := &Document{Blocks: []Block{{Kind: ListBlock, List: "number", Start: listStartPointer(0), Numbering: &ListNumbering{Template: "%1.", Formats: []string{format}}, Items: []ListItem{{Blocks: []Block{{Kind: Paragraph}}}}}}}
		if _, err := encodeColleafNotes(context.Background(), d); err == nil {
			t.Fatal("unmapped native zero marker emitted", format)
		}
		glyph, _ := colleafNumberGlyph(format)
		body := colleafBody{DataStream: "X\r\n", Paragraphs: []colleafParagraphRecord{{StartIndex: 1, Bullet: sfAttrs(colleafBullet{Type: "t", ID: "l", Level: 0})}}}
		lists := map[string]json.RawMessage{"t": sfAttrs(colleafNativeList{Type: "t", Levels: []colleafListLevel{{Format: "%1.", Glyph: &glyph, Start: -1, Align: 1}}})}
		if _, err := decodeColleafBody(context.Background(), body, colleafTextContext{Lists: lists}, nil); err == nil {
			t.Fatal("native zero marker silently changed", format)
		}
	}
}
