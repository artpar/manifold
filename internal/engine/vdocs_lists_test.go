package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestVDOCSLiteralTemplateProjection(t *testing.T) {
	for _, tc := range []struct{ input, format, symbol string }{
		{"%%1.", "%10%1.", "%"}, {"%10", "%1%10", "0"}, {"%1%", "%1%10", "%"},
		{"50% literal", "%10", "50% literal"}, {"%9%%1.", "%10%1.", "%"}, {"%1%90", "%1%10", "0"},
	} {
		source := vdocsNativeList{Levels: []vdocsListLevel{{Glyph: vdocsInt(2)}}}
		format := projectVDOCSListTemplate(tc.input, 0, &source)
		source.Levels[0].Format = format
		if format != tc.format || len(source.Levels) != 10 || source.Levels[9].Symbol != tc.symbol {
			t.Fatalf("%q: %q %+v", tc.input, format, source.Levels)
		}
		binding := vdocsListBinding{OriginalTemplate: &tc.input}
		got, err := readVDOCSListTemplate(source, 0, binding)
		if err != nil || got != tc.input {
			t.Fatal(got, err)
		}
		// Lost intent can still derive the actually represented logical template.
		got, err = readVDOCSListTemplate(source, 0, vdocsListBinding{})
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(vdocsSFListParts(got, 0), vdocsSFListParts(tc.input, 0)) {
			t.Fatal("native helper semantics lost", got, tc.input)
		}
	}
	// Ignore a deeper placeholder without turning its surrounding literal text into
	// a false active placeholder. Only checked original intent expresses this case.
	original := "%%91"
	source := vdocsNativeList{Levels: []vdocsListLevel{{Glyph: vdocsInt(2)}}}
	source.Levels[0].Format = projectVDOCSListTemplate(original, 0, &source)
	if got, err := readVDOCSListTemplate(source, 0, vdocsListBinding{OriginalTemplate: &original}); err != nil || got != original {
		t.Fatal(got, err)
	}
	if _, err := readVDOCSListTemplate(source, 0, vdocsListBinding{}); err == nil {
		t.Fatal("ambiguous literal percent-digit silently became a counter")
	}
}

func vdocsLiteralListExample(t testing.TB) *Document {
	p := func(s string) Block { return Block{Kind: Paragraph, Runs: []Run{{Text: s}}} }
	numbered := func(template string, start int, level int, formats []string, ancestors []int, label string) Block {
		return Block{Kind: ListBlock, List: "number", Start: listStartPointer(start), Numbering: &ListNumbering{Template: template, Level: level, Formats: formats, Ancestors: ancestors}, Items: []ListItem{{Blocks: []Block{p(label + " one")}}, {Blocks: []Block{p(label + " two")}}}}
	}
	outer := numbered("%%1%", 2, 0, []string{"decimal"}, nil, "Parent")
	outer.Items[0].Blocks = append(outer.Items[0].Blocks, p("Continuation"), numbered("%1/%2%0", 4, 1, []string{"decimal", "lowerRoman"}, []int{99}, "Child"))
	return &Document{Metadata: Metadata{Title: "Literal native markers"}, Headers: []Block{numbered("header% %1", 1, 0, []string{"decimal"}, nil, "Header")}, Blocks: []Block{
		outer, numbered("%10", 1, 0, []string{"decimal"}, nil, "Digits"), numbered("50% literal", 7, 0, []string{"decimal"}, nil, "Constant"),
		numbered("%%1.%2.%3.%4.%5.%6.%7.%8.%9%", 9, 8, []string{"decimal", "decimal", "decimal", "decimal", "decimal", "decimal", "decimal", "decimal", "decimal"}, []int{1, 2, 3, 4, 5, 6, 7, 8}, "Nine"),
		{Kind: Table, Rows: []Row{{Cells: []Cell{{Blocks: []Block{numbered("%12", 1, 0, []string{"decimal"}, nil, "Cell")}}}}}},
		{Kind: Paragraph, Runs: []Run{{Text: "Note "}, {Note: &NoteReference{Kind: Footnote, ID: "literal"}}}},
	}, Notes: []Note{{Kind: Footnote, ID: "literal", Blocks: []Block{numbered("note% %1%", 0, 0, []string{"decimal"}, nil, "Note")}}}}
}
func TestVDOCSLiteralListsSFAndActualSave(t *testing.T) {
	ctx := context.Background()
	want := resolveNumberingDocument(vdocsLiteralListExample(t))
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
		t.Fatalf("literal list semantics differ\ngot %s\nwant %s", a, b)
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
	if !reflect.DeepEqual(again, wire) {
		t.Fatal("SF changed native helper/ownership/asset semantics")
	}
	if file := os.Getenv("MANYDOC_VDOCS_LITERAL_LIST_QA"); file != "" {
		data, err := json.Marshal(wire)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(file, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if file := os.Getenv("MANYDOC_VDOCS_LITERAL_LIST_SAVED"); file != "" {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var saved vdocsNoteSnapshot
		if err = json.Unmarshal(data, &saved); err != nil {
			t.Fatal(err)
		}
		got, err = decodeVDOCSDocument(ctx, saved)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatal("actual facade changed literal list semantics", got.Diagnostics)
		}
	}
	if file := os.Getenv("MANYDOC_VDOCS_LITERAL_LIST_EDITED"); file != "" {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var edited vdocsNoteSnapshot
		if err = json.Unmarshal(data, &edited); err != nil {
			t.Fatal(err)
		}
		got, err = decodeVDOCSDocument(ctx, edited)
		if err != nil {
			t.Fatal(err)
		}
		// SF resolves enclosing counters over the stored fallback ancestor values.

		// This custom indent cannot be represented by logical SF nesting alone.

		sf, err := ToStructured(got)
		if err != nil {
			t.Fatal(err)
		}
		got, err = FromStructured(sf)
		if err != nil {
			t.Fatal(err)
		}
		outer := got.Blocks[0]
		if outer.Numbering.Template != "‰%1‰" || listStart(outer) != 5 || outer.Items[0].Blocks[2].Numbering.Ancestors[0] != 5 {
			t.Fatal("current native helper/counter edits lost", outer, got.Diagnostics)
		}
	}
}
func TestVDOCSNativeHelperEditsAndContradictions(

	// Change the first native list's second item to an explicit restart.

	// Reuse rich marker content in notes and running stories without note refs.

	t *testing.T) {
	ctx := context.Background()
	d := vdocsLiteralListExample(t)
	for _, mode := range []string{"symbol", "missing", "badref", "activehelper", "constant"} {
		wire, err := encodeVDOCSDocument(ctx, d)
		if err != nil {
			t.Fatal(err)
		}
		var bullet vdocsBullet
		_ = json.Unmarshal(wire.Body.Paragraphs[0].Bullet, &bullet)
		var source vdocsNativeList
		_ = json.Unmarshal(wire.Lists[bullet.Type], &source)
		switch mode {
		case "symbol":
			source.Levels[9].Symbol = "‰"
		case "missing":
			source.Levels = source.Levels[:9]
		case "badref":
			source.Levels[0].Format = "%999"
		case "activehelper":
			bullet.Level = 9
			wire.Body.Paragraphs[0].Bullet = sfAttrs(bullet)
		case "constant":
			source.Levels[0].Symbol = "Fixed"
		}
		wire.Lists[bullet.Type] = sfAttrs(source)
		got, err := decodeVDOCSDocument(ctx, wire)
		if mode == "symbol" {
			if err != nil || got.Blocks[0].Numbering.Template != "‰%1‰" {
				t.Fatal(mode, err)
			}
		} else if mode == "constant" {
			if err != nil || got.Blocks[0].Numbering.Template != "Fixed" {
				t.Fatal(mode, err)
			}
		} else if err == nil {
			t.Fatal("invalid helper accepted", mode)
		}
	}
}
func FuzzVDOCSListTemplate(f *testing.F) {
	for _, s := range []string{"%%1%", "%10", "%9%%1", "%%91", strings.Repeat("%1a%", 40)} {
		f.Add(s, uint8(0))
	}
	f.Fuzz(func(t *testing.T, template string, l uint8) {
		level := int(l % 9)
		if len(template) > 512 || validateString(template) != nil {
			return
		}
		source := vdocsNativeList{}
		for range level + 1 {
			source.Levels = append(source.Levels, vdocsListLevel{Glyph: vdocsInt(2)})
		}
		source.Levels[level].Format = projectVDOCSListTemplate(template, level, &source)
		got, err := readVDOCSListTemplate(source, level, vdocsListBinding{OriginalTemplate: &template})
		if err != nil || got != template {
			t.Fatal(got, err)
		}
		if len(source.Levels) > vdocsMaxListEntries {
			t.Fatal("unbounded helpers")
		}
	})
}
func vdocsListExample() *Document {
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
func TestVDOCSRichListOwnershipNumberingAndSF(t *testing.T) {
	ctx := context.Background()
	original := vdocsListExample()
	want := resolveNumberingDocument(original)
	wire, err := encodeVDOCSNotes(ctx, want)
	if err != nil {
		t.Fatal(err)
	}
	got, err := decodeVDOCSNotes(ctx, wire, vdocsTextContext{})
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
	wire2, err := encodeVDOCSNotes(ctx, model)
	if err != nil {
		t.Fatal(err)
	}
	if original.Blocks[0].Items[0].Blocks[2].Numbering.Ancestors[0] != 99 {
		t.Fatal("caller numbering mutated")
	}
	if !reflect.DeepEqual(wire, wire2) {
		t.Fatal("SF changed native list source/ownership records")
	}
	if file := os.Getenv("MANYDOC_VDOCS_LIST_QA"); file != "" {
		data, err := json.MarshalIndent(wire, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(file, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if file := os.Getenv("MANYDOC_VDOCS_LIST_EDIT"); file != "" {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var edited vdocsNoteSnapshot
		if err = json.Unmarshal(data, &edited); err != nil {
			t.Fatal(err)
		}
		doc, err := decodeVDOCSNotes(ctx, edited, vdocsTextContext{})
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

func TestVDOCSCustomListIndentStillReportsPresentationLoss(t *testing.T) {
	ctx := context.Background()
	want := resolveNumberingDocument(vdocsListExample())
	wire, err := encodeVDOCSNotes(ctx, want)
	if err != nil {
		t.Fatal(err)
	}
	for id, raw := range wire.Lists {
		var list vdocsNativeList
		if err := json.Unmarshal(raw, &list); err != nil {
			t.Fatal(err)
		}

		for i := range list.Levels {
			list.Levels[i].Properties = json.RawMessage(`{"hanging":{"v":21},"indentStart":{"v":99}}`)
		}
		wire.Lists[id] = sfAttrs(list)
	}
	got, err := decodeVDOCSNotes(ctx, wire, vdocsTextContext{})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, d := range got.Diagnostics {
		found = found || d.Code == "vdocs_list_presentation"
	}
	if !found {
		t.Fatal("custom list indentation loss was silent")
	}
	got.Diagnostics = nil
	if !reflect.DeepEqual(got, want) {
		t.Fatal("custom indent normalization changed list content or counters")
	}
}
func TestVDOCSNativeListsWithoutOwnershipKeepActualBullets(t *testing.T) {
	ctx := context.Background()
	wire, err := encodeVDOCSNotes(ctx, vdocsListExample())
	if err != nil {
		t.Fatal(err)
	}
	wire, err = openVDOCSIntent(ctx, wire)
	if err != nil {
		t.Fatal(err)
	}
	var keep []vdocsResource
	for _, r := range wire.Resources {
		if r.Name != vdocsListResource {
			keep = append(keep, r)
		}
	}
	wire.Resources = keep
	doc, err := decodeVDOCSNotes(ctx, wire, vdocsTextContext{})
	if err != nil {
		t.Fatal(err)
	}
	if doc.Blocks[0].Kind != ListBlock || doc.Blocks[0].Start == nil || *doc.Blocks[0].Start != 27 || doc.Blocks[1].Runs[0].Text != "Continuation" {
		t.Fatal(doc.Blocks)
	}
	found := false
	for _, d := range doc.Diagnostics {
		if d.Code == "vdocs_list_ownership" {
			found = true
		}
	}
	if !found {
		t.Fatal("unrepresented continuation ownership was not diagnosed")
	}
}
func TestVDOCSStaleOwnershipDoesNotOverrideNativeRestart(t *testing.T) {
	wire, err := encodeVDOCSNotes(context.Background(), vdocsListExample())
	if err != nil {
		t.Fatal(err)
	}

	for i, p := range wire.Body.Paragraphs {
		var b vdocsBullet
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
	doc, err := decodeVDOCSNotes(context.Background(), wire, vdocsTextContext{})
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
		if d.Code == "vdocs_stale_list_ownership" {
			foundLoss = true
		}
	}
	if !foundRestart || !foundLoss {
		t.Fatal("stale list metadata won over native restart", doc.Blocks, doc.Diagnostics)
	}
}

func TestVDOCSListTokenProjectionAndNativeEdits(t *testing.T) {
	d := &Document{Blocks: []Block{{Kind: ListBlock, List: "number", Numbering: &ListNumbering{Template: "%1.%9", Formats: []string{"decimal"}}, Items: []ListItem{{Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "Kept"}}}}}}}}}
	wire, err := encodeVDOCSNotes(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range wire.Lists {
		var source vdocsNativeList
		if err = json.Unmarshal(raw, &source); err != nil {
			t.Fatal(err)
		}
		if source.Levels[0].Format != "%1." {
			t.Fatal("absent native level left dereferenceable", source)
		}
	}
	got, err := decodeVDOCSNotes(context.Background(), wire, vdocsTextContext{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Blocks[0].Numbering.Template != "%1.%9" {
		t.Fatal("ignored-placeholder intent lost")
	}
	for key, raw := range wire.Lists {
		var source vdocsNativeList
		_ = json.Unmarshal(raw, &source)
		source.Levels[0].Format = "Edited %1"
		wire.Lists[key] = sfAttrs(source)
	}
	got, err = decodeVDOCSNotes(context.Background(), wire, vdocsTextContext{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Blocks[0].Numbering.Template != "Edited %1" {
		t.Fatal("stale intent overrode native edit")
	}
	d.Blocks[0].Numbering.Template = "%%1."
	if _, err = encodeVDOCSNotes(context.Background(), d); err != nil {
		t.Fatal("literal percent projection rejected", err)
	}
}
func FuzzVDOCSLists(f *testing.F) {
	wire, err := encodeVDOCSNotes(context.Background(), vdocsListExample())
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
		var input vdocsNoteSnapshot
		if json.Unmarshal(data, &input) != nil {
			return
		}
		doc, err := decodeVDOCSNotes(context.Background(), input, vdocsTextContext{})
		if err == nil {
			if err = doc.Validate(); err != nil {
				t.Fatal("accepted invalid list/note graph", err)
			}
		}
	})
}

func TestVDOCSNondecimalZeroNeverSilentlyChangesMarker(t *testing.T) {
	for _, format := range []string{"decimalZero", "upperLetter", "lowerLetter", "upperRoman", "lowerRoman"} {
		d := &Document{Blocks: []Block{{Kind: ListBlock, List: "number", Start:

		// Losing a complete group must retain actual cells, rather than invent a
		// replacement list or silently dropping its visible labels/relationships.

		listStartPointer(0), Numbering: &ListNumbering{Template: "%1.", Formats: []string{format}}, Items: []ListItem{{Blocks: []Block{{Kind: Paragraph}}}}}}}
		wire, err := encodeVDOCSNotes(context.Background(), d)
		if err != nil {
			t.Fatal(err)
		}
		got, err := decodeVDOCSNotes(context.Background(), wire, vdocsTextContext{})
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, d) {
			t.Fatal("authored zero format intent lost", format, got)
		}
		glyph, _ := vdocsNumberGlyph(format)
		body := vdocsBody{DataStream: "X\r\n", Paragraphs: []vdocsParagraphRecord{{StartIndex: 1, Bullet: sfAttrs(vdocsBullet{Type: "t", ID: "l", Level: 0})}}}
		lists := map[string]json.RawMessage{"t": sfAttrs(vdocsNativeList{Type: "t", Levels: []vdocsListLevel{{Format: "%1.", Glyph: &glyph, Start: -1, Align: 1}}})}
		foreign, err := decodeVDOCSBody(context.Background(), body, vdocsTextContext{Lists: lists}, nil)
		if err != nil {
			t.Fatal(err)
		}
		want := "."
		if format == "decimalZero" {
			want = "00."
		}
		label, err := numberedLabel(foreign.Blocks[0], 0)
		if err != nil || label != want {
			t.Fatal("native actual zero marker lost", format, label, err)
		}
		found := false
		for _, diag := range foreign.Diagnostics {
			found = found || diag.Code == "vdocs_numbering_policy"
		}
		if !found {
			t.Fatal("native policy normalization not disclosed")
		}
	}
}
func TestVDOCSLiteralMarkerInternalRoundtrip(t *testing.T) {
	d := literalMarkerDocument()
	wire, err := encodeVDOCSDocument(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	got, err := decodeVDOCSDocument(context.Background(), wire)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(canonicalSF(t, d), canonicalSF(t, got)) {
		t.Fatalf("literal graph changed\n%s\n%s", canonicalSF(t, d), canonicalSF(t, got))
	}
	if path := os.Getenv("MANYDOC_NATIVE_LITERAL_QA"); path != "" {
		raw, _ := json.MarshalIndent(wire, "", "  ")
		if err := os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestVDOCSLiteralMarkerSourcesAndAssets(t *testing.T) {
	d := literalMarkerDocument()
	d.Blocks[0].Source = &SourceInfo{Format: PDF, Role: "L", Element: "list", Structure: "explicit", Confidence: 1, Evidence: "source_list"}
	d.Blocks[0].Items[0].Source = &SourceInfo{Format: PDF, Role: "LI", Element: "item", ParentElement: "list", Structure: "explicit", Confidence: 1, Evidence: "source_item"}
	d.Blocks[0].Items[0].Marker.Source = &SourceInfo{Format: PDF, Role: "Lbl", Element: "label", ParentElement: "item", Structure: "explicit", Confidence: 1, Evidence: "source_label"}
	before := canonicalSF(t, d)
	wire, err := encodeVDOCSDocument(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	got, err := decodeVDOCSDocument(context.Background(), wire)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, canonicalSF(t, got)) {
		t.Fatalf("source owners changed\n%s\n%s", before, canonicalSF(t, got))
	}
	checkNativeLiteral(t, "sources", d)
}

func checkNativeLiteral(t *testing.T, name string, d *Document) vdocsNoteSnapshot {
	t.Helper()
	d = resolveNumberingDocument(d)
	before := canonicalSF(t, d)
	wire, err := encodeVDOCSDocument(context.Background(), d)
	if err != nil {
		t.Fatal(name, err)
	}
	got, err := decodeVDOCSDocument(context.Background(), wire)
	if err != nil {
		t.Fatal(name, err)
	}
	if !bytes.Equal(before, canonicalSF(t, got)) {
		t.Fatalf("%s changed\n%s\n%s", name, before, canonicalSF(t, got))
	}
	if !bytes.Equal(before, canonicalSF(t, d)) {
		t.Fatal("caller mutated")
	}
	if dir := os.Getenv("MANYDOC_NATIVE_LITERAL_QA_DIR"); dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		raw, _ := json.MarshalIndent(wire, "", "  ")
		if err := os.WriteFile(filepath.Join(dir, name+".json"), raw, 0600); err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		if err := WriteVDOCS(context.Background(), &out, d); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name+".vdocs"), out.Bytes(), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return wire
}

func nativeLiteralAsset(t *testing.T) Block {
	t.Helper()
	im := image.NewRGBA(image.Rect(0, 0, 1, 1))
	im.Set(0, 0, color.RGBA{R: 255, A: 255})
	var data bytes.Buffer
	if err := png.Encode(&data, im); err != nil {
		t.Fatal(err)
	}
	return Block{Kind: Picture, Image: &Image{Data: data.Bytes(), MediaType: "image/png", Width: 16, Height: 16, Alt: "RED_LABEL"}}
}

func TestVDOCSLiteralMarkerRichCases(t *testing.T) {
	checkNativeLiteral(t, "rich", literalMarkerDocument())
	checkNativeLiteral(t, "image-control", &Document{Blocks: []Block{nativeLiteralAsset(t), mergeParagraph("IMAGE_CONTROL")}})
	mixed := &Document{Blocks: []Block{{Kind: ListBlock, List: "number", Start: listStartPointer(7), Numbering: &ListNumbering{Template: "Item %1)", Formats: []string{"decimal"}}, Items: []ListItem{
		{Blocks: []Block{mergeParagraph("AUTO_FIRST")}},
		{Marker: &ListMarker{Blocks: []Block{mergeParagraph("[done]")}}, Blocks: []Block{mergeParagraph("DONE_BODY")}},
		{Blocks: []Block{mergeParagraph("AUTO_RESUMED")}},
	}}}}
	checkNativeLiteral(t, "mixed", mixed)
	image := nativeLiteralAsset(t)
	nested := Block{Kind: ListBlock, List: "bullet", Anchors: []string{"list-point"}, Items: []ListItem{{Marker: &ListMarker{Blocks: []Block{{Kind: Heading, Level: 2, Runs: []Run{{Text: "RICH_LABEL", Bold: true}}}, image, cellTable([]Block{mergeParagraph("INNER_TABLE")}), {Kind: ListBlock, List: "number", Items: []ListItem{{Blocks: []Block{mergeParagraph("INNER_LIST")}}}}}}, Blocks: []Block{mergeParagraph("RICH_BODY"), {Kind: AnchorPoint, Anchors: []string{"item-end"}}}}}}
	d := &Document{Blocks: []Block{cellTable([]Block{nested})}}
	checkNativeLiteral(t, "nested", d)

	nested.Anchors = nil
	nested.Items[0].Blocks = []Block{mergeParagraph("RICH_BODY")}
	d = &Document{Headers: []Block{nested}, Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "CALL"}, {Note: &NoteReference{Kind: Footnote, ID: "n"}}}}}, Notes: []Note{{Kind: Footnote, ID: "n", Blocks: []Block{nested}}}}
	checkNativeLiteral(t, "stories", d)
	rtl := Block{Kind: ListBlock, List: "bullet", Items: []ListItem{{Marker: &ListMarker{Blocks: []Block{mergeParagraph("RIGHT_LABEL")}}, Blocks: []Block{{Kind: Paragraph, Direction: "rtl", Runs: []Run{{Text: "RTL_BODY"}}}}}}}
	checkNativeLiteral(t, "rtl", &Document{Blocks: []Block{rtl}})
	continued := &Document{Layout: Layout{Width: 300, Height: 400, Margin: 30}}
	item := ListItem{Marker: &ListMarker{}}
	for i := 0; i < 48; i++ {
		item.Marker.Blocks = append(item.Marker.Blocks, mergeParagraph(fmt.Sprintf("LABEL_LINE_%02d wraps across this narrow label column", i)))
		item.Blocks = append(item.Blocks, mergeParagraph(fmt.Sprintf("BODY_LINE_%02d wraps across this narrow body column", i)))
	}
	continued.Blocks = []Block{{Kind: ListBlock, List: "bullet", Items: []ListItem{item}}}
	checkNativeLiteral(t, "continued", continued)
}

func TestVDOCSLiteralMarkerActualSavedEdit(t *testing.T) {
	path := os.Getenv("MANYDOC_NATIVE_LITERAL_SAVED")
	if path == "" {
		t.Skip("optional pinned facade-save input")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var wire vdocsNoteSnapshot
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatal(err)
	}
	got, err := decodeVDOCSDocument(context.Background(), wire)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got.Text(), "Z)") {
		t.Fatal("native label edit hidden", got.Text())
	}
	if got.Blocks[0].Items[0].Marker == nil || got.Blocks[0].Items[1].Marker.Blocks[0].Runs[2].Note == nil {
		t.Fatal("native marker/note hierarchy lost")
	}
}

func TestVDOCSLiteralMarkerActualSavedCases(t *testing.T) {
	dir := os.Getenv("MANYDOC_NATIVE_LITERAL_SAVED_DIR")
	if dir == "" {
		t.Skip("optional pinned facade-save cases")
	}
	for _, name := range []string{"mixed", "nested", "stories", "rtl", "sources", "continued"} {
		decode := func(path string) *Document {
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var wire vdocsNoteSnapshot
			if err := json.Unmarshal(raw, &wire); err != nil {
				t.Fatal(err)
			}
			d, err := decodeVDOCSDocument(context.Background(), wire)
			if err != nil {
				t.Fatal(name, err)
			}
			return d
		}
		original := decode(filepath.Join(dir, name+".json"))
		saved := decode(filepath.Join(dir, name+"-saved.json"))
		if !bytes.Equal(canonicalSF(t, original), canonicalSF(t, saved)) {
			t.Fatalf("%s native facade changed semantic graph", name)
		}
	}
}

func TestVDOCSLiteralMarkerBrowserSaved(t *testing.T) {
	dir := os.Getenv("MANYDOC_NATIVE_LIST_BROWSER_DIR")
	if dir == "" {
		t.Skip("optional actual desktop/mobile browser saves")
	}
	for _, name := range []string{"nested", "mixed", "continued"} {
		read := func(suffix string) *Document {
			raw, err := os.ReadFile(filepath.Join(dir, name+suffix+".vdocs"))
			if err != nil {
				t.Fatal(err)
			}
			d, err := ReadVDOCS(context.Background(), bytes.NewReader(raw))
			if err != nil {
				t.Fatal(name, suffix, err)
			}
			return d
		}
		original := canonicalSF(t, read(""))
		for _, suffix := range []string{"-browser", "-mobile-browser"} {
			if !bytes.Equal(original, canonicalSF(t, read(suffix))) {
				t.Fatal(name, suffix, "browser save changed semantic graph")
			}
		}
	}
}

func TestVDOCSLiteralMarkerMalformedAndStaleOwnership(t *testing.T) {
	ctx := context.Background()
	original := literalMarkerDocument()
	base, err := encodeVDOCSDocument(ctx, original)
	if err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(map[string]json.RawMessage){
		func(m map[string]json.RawMessage) { m["version"] = json.RawMessage(`2`) },
		func(m map[string]json.RawMessage) { m["count"] = json.RawMessage(`0`) },
		func(m map[string]json.RawMessage) { m["index"] = json.RawMessage(`1000000`) },
		func(m map[string]json.RawMessage) { m["content"] = json.RawMessage(`"forbidden cache"`) },
	} {
		raw, _ := json.Marshal(base)
		var wire vdocsNoteSnapshot
		_ = json.Unmarshal(raw, &wire)
		id := wire.Body.Tables[0].ID
		props, _ := vdocsStyleObject(wire.Tables[id])
		var meta map[string]json.RawMessage
		_ = json.Unmarshal(props["manydocLiteralItem"], &meta)
		change(meta)
		props["manydocLiteralItem"] = sfAttrs(meta)
		wire.Tables[id] = sfAttrs(props)
		if _, err := decodeVDOCSDocument(ctx, wire); err == nil {
			t.Fatal("malformed private literal intent accepted")
		}
	}

	raw, _ := json.Marshal(base)
	var wire vdocsNoteSnapshot
	_ = json.Unmarshal(raw, &wire)
	id := wire.Body.Tables[1].ID
	props, _ := vdocsStyleObject(wire.Tables[id])
	delete(props, "manydocLiteralItem")
	wire.Tables[id] = sfAttrs(props)
	got, err := decodeVDOCSDocument(ctx, wire)
	if err != nil {
		t.Fatal(err)
	}
	if got.Blocks[0].Kind != Table || !hasDiagnostic(got, "vdocs_stale_literal_ownership") {
		t.Fatal("stale ownership did not retain actual table")
	}
	for _, token := range []string{"α)", "β)", "FIRST", "SECOND", "EMPTY", "Marker note"} {
		if !strings.Contains(got.Text(), token) {
			t.Fatal("stale ownership lost", token)
		}
	}
}
func vdocsZeroExample(t testing.TB) *Document {
	p := func(text string) Block { return Block{Kind: Paragraph, Runs: []Run{{Text: text, Bold: true}}} }
	list := func(format string, legal bool) Block {
		return Block{Kind: ListBlock, List: "number", Start: listStartPointer(0), Numbering: &ListNumbering{Template: "%1%", Formats: []string{format}, Legal: legal}, Items: []ListItem{{Blocks: []Block{p(format + " zero"), p("Continuation")}}, {Blocks: []Block{p(format + " one")}}, {Blocks: []Block{p(format + " two")}}}}
	}
	d := &Document{Metadata: Metadata{Title: "Native zero and legal"}, Headers: []Block{list("lowerRoman", false)}}
	for _, format := range []string{"decimalZero", "upperLetter", "lowerLetter", "upperRoman", "lowerRoman"} {
		d.Blocks = append(d.Blocks, list(format, false))
	}
	legal := list("decimalZero", true)
	legal.Numbering.Template = "%1."
	d.Blocks = append(d.Blocks, legal)
	parent := list("upperLetter", false)
	parent.Numbering.Template = "%1."
	child := list("lowerRoman", false)
	child.Numbering = &ListNumbering{Level: 1, Template: "%1.%2", Formats: []string{"upperLetter", "lowerRoman"}, Ancestors: []int{99}}
	parent.Items[0].Blocks = append(parent.Items[0].Blocks, child)
	d.Blocks = append(d.Blocks, parent)
	compound := list("lowerRoman", true)
	compound.Numbering = &ListNumbering{Level: 1, Template: "%1.%2", Formats: []string{"decimalZero", "lowerRoman"}, Ancestors: []int{0}, Legal: true, Independent: true}
	d.Blocks = append(d.Blocks, compound)
	d.Blocks = append(d.Blocks, Block{Kind: Table, Rows: []Row{{Cells: []Cell{{Blocks: []Block{list("decimalZero", false)}}}}}}, Block{Kind: Paragraph, Runs: []Run{{Text: "Note "}, {Note: &NoteReference{Kind: Footnote, ID: "zero"}}}})
	d.Notes = []Note{{Kind: Footnote, ID: "zero", Blocks: []Block{list("upperRoman", false)}}}
	return d
}
func TestVDOCSZeroLegalSFAndActualSave(t *testing.T) {
	ctx := context.Background()
	want := resolveNumberingDocument(vdocsZeroExample(t))
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
		t.Fatalf("zero/legal semantics differ\ngot%s\nwant%s", a, b)
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
	if !reflect.DeepEqual(again, wire) {
		t.Fatal("SF changed native projection/types/ownership")
	}
	if file := os.Getenv("MANYDOC_VDOCS_ZERO_QA"); file != "" {
		data, err := json.Marshal(wire)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(file, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if file := os.Getenv("MANYDOC_VDOCS_ZERO_SAVED"); file != "" {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var saved vdocsNoteSnapshot
		if err = json.Unmarshal(data, &saved); err != nil {
			t.Fatal(err)
		}
		got, err = decodeVDOCSDocument(ctx, saved)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatal("actual facade changed zero/legal semantics", got.Diagnostics)
		}
	}
	if file := os.Getenv("MANYDOC_VDOCS_ZERO_EDITED"); file != "" {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var edited vdocsNoteSnapshot
		if err = json.Unmarshal(data, &edited); err != nil {
			t.Fatal(err)
		}
		got, err = decodeVDOCSDocument(ctx, edited)
		if err != nil {
			t.Fatal(err)
		}
		if got.Blocks[0].Numbering.Formats[0] != "decimal" || listStart(got.Blocks[0]) != 5 {
			t.Fatal("stale zero intent hid native edit", got.Blocks[0])
		}
		sf, err := ToStructured(got)
		if err != nil {
			t.Fatal(err)
		}
		model, err := FromStructured(sf)
		if err != nil {
			t.Fatal(err)
		}
		labels := vdocsNumberLabels(t, model.Blocks)
		expected := []string{"5%", "06%", "07%", "0%", "A%", "B%", "0%", "a%", "b%", "0%", "I%", "II%", "0%", "i%", "ii%", "00.", "01.", "02.", "5.", "5.0", "5.i", "5.ii", "F.", "G.", "0.0", "0.1", "0.2", "0%", "01%", "02%"}
		if !reflect.DeepEqual(labels, expected) {
			t.Fatal("SF lost edited actual markers", labels)
		}
		found := false
		for _, diag := range got.Diagnostics {
			found = found || diag.Code == "vdocs_stale_numbering_projection"
		}
		if !found {
			t.Fatal("stale zero projection not diagnosed")
		}
	}
}
func TestVDOCSForeignZeroAndLegalPreserveMarkers(t *testing.T) {
	for _, format := range []string{"decimalZero", "upperLetter", "lowerLetter", "upperRoman", "lowerRoman"} {
		for _, legal := range []bool{false, true} {
			glyph, _ := vdocsNumberGlyph(format)
			source := vdocsNativeList{Type: "t", Levels: []vdocsListLevel{{Format: "%1.", Glyph: &glyph, Start: -1, Align: 1, Legal: legal}}}
			body := vdocsBody{DataStream: "X\rY\rZ\r\n", Paragraphs: []vdocsParagraphRecord{{StartIndex: 1}, {StartIndex: 3}, {StartIndex: 5}}}
			for i := range body.Paragraphs {
				body.Paragraphs[i].Bullet = sfAttrs(vdocsBullet{Type: "t", ID: "l"})
			}
			d, err := decodeVDOCSBody(context.Background(), body, vdocsTextContext{Lists: map[string]json.RawMessage{"t": sfAttrs(source)}}, nil)
			if err != nil {
				t.Fatal(err)
			}
			var labels []string
			for _, b := range d.Blocks {
				if b.Kind != ListBlock {
					t.Fatal("lost list item grouping")
				}
				for i := range b.Items {
					label, err := numberedLabel(b, i)
					if err != nil {
						t.Fatal(err)
					}
					labels = append(labels, label)
				}
			}
			var expected []string
			switch {
			case format == "decimalZero":
				expected = []string{"00.", "01.", "02."}
			case legal:
				expected = []string{"0.", "1.", "2."}
			default:
				a, _ := numberText(format, 1)
				b, _ := numberText(format, 2)
				expected = []string{".", a + ".", b + "."}
			}
			if !reflect.DeepEqual(labels, expected) {
				t.Fatal(format, legal, labels, expected)
			}
			if _, err = ToStructured(d); err != nil {
				t.Fatal(err)
			}
		}
	}
}
func TestVDOCSMalformedNumberingIntent(t *testing.T) {
	for _, p := range []vdocsNumberProjection{
		{}, {Formats: map[int]string{-1: "upperLetter"}, ZeroLevels: []int{-1}},
		{Formats: map[int]string{0: "decimal"}, ZeroLevels: []int{0}},
		{Formats: map[int]string{0: "upperLetter"}, ZeroLevels: []int{0, 0}},
		{Formats: map[int]string{0: "upperLetter"}},
		{Formats: map[int]string{0: "upperRoman"}, Legal: true},
		{Formats: map[int]string{0: "decimalZero"}, ZeroLevels: []int{0}, Legal: true},
	} {
		if err := validateVDOCSNumberProjection(p, 0); err == nil {
			t.Fatal("malformed projection accepted", p)
		}
	}
	if _, err := vdocsListBindingTypes(vdocsListBinding{Type: "t", Projections: map[string]vdocsNumberProjection{"unknown": {Formats: map[int]string{0: "upperLetter"}, ZeroLevels: []int{0}}}}); err == nil {
		t.Fatal("unowned type projection accepted")
	}
}

func vdocsNumberLabels(t testing.TB, blocks []Block) []string {
	t.Helper()
	var labels []string
	var walk func([]Block)
	walk = func(blocks []Block) {
		for _, b := range blocks {
			switch b.Kind {
			case ListBlock:
				for i, item := range b.Items {
					label, err := numberedLabel(b, i)
					if err != nil {
						t.Fatal(err)
					}
					labels = append(labels, label)
					walk(item.Blocks)
				}
			case Table:
				for _, r := range b.Rows {
					for _, c := range r.Cells {
						walk(c.Blocks)
					}
				}
			case SectionBlock:
				walk(b.Section.Blocks)
			}
		}
	}
	walk(blocks)
	return labels
}
func TestVDOCSNumberProjectionEmbeddedContradictions(t *testing.T) {
	for _, mode := range []string{"negative", "unowned", "collision"} {
		ctx := context.Background()
		wire, err := encodeVDOCSDocument(ctx, vdocsZeroExample(t))
		if err != nil {
			t.Fatal(err)
		}
		wire, err = openVDOCSIntent(ctx, wire)
		if err != nil {
			t.Fatal(err)
		}
		var ownership vdocsListOwnership
		if err = vdocsResourceMap(wire.Resources, vdocsListResource, &ownership); err != nil {
			t.Fatal(err)
		}
		b := &ownership.Stories[""][0]
		switch mode {
		case "negative":
			name := b.Items[0].Type
			b.Projections[name] = vdocsNumberProjection{Formats: map[int]string{-1: "upperLetter"}, ZeroLevels: []int{-1}}
		case "unowned":
			b.Projections["unowned"] = vdocsNumberProjection{Formats: map[int]string{0: "upperLetter"}, ZeroLevels: []int{0}}
		case "collision":
			b.Items[0].Type = ownership.Stories[""][1].Type
		}
		wire.Resources = vdocsReplaceResource(wire.Resources, vdocsListResource, ownership)
		if err = sealVDOCSIntent(&wire); err != nil {
			t.Fatal(err)
		}
		if _, err = decodeVDOCSDocument(ctx, wire); err == nil {
			t.Fatal("malformed native intent accepted", mode)
		}
	}
}
func FuzzVDOCSNumberProjection(f *testing.F) {
	f.Add(uint8(0), uint8(0), uint8(0), false)
	f.Add(uint8(5), uint8(8), uint8(0), true)
	f.Fuzz(func(t *testing.T, format, rawLevel, rawValue uint8, legal bool) {
		level, value := int(rawLevel%9), int(rawValue%3)
		n := &ListNumbering{Level: level, Template: "%1.", Legal: legal}
		source := vdocsNativeList{}
		for l := 0; l <= level; l++ {
			spec := vdocsNumberFormats[(int(format)+l)%len(vdocsNumberFormats)]
			n.Formats = append(n.Formats, spec)
			glyph, _ := vdocsNumberGlyph(spec)
			source.Levels = append(source.Levels, vdocsListLevel{Glyph: &glyph})
			if l < level {
				n.Ancestors = append(n.Ancestors, (value+l)%3)
			}
		}
		source.Levels[level].Legal = legal
		projected, p := projectVDOCSNumbering(source, n, value)
		if p == nil {
			return
		}
		if err := validateVDOCSNumberProjection(*p, level); err != nil {
			t.Fatal(err)
		}
		imported := *n
		imported.Formats = nil
		for _, spec := range projected.Levels {
			imported.Formats = append(imported.Formats, vdocsNumberFormats[*spec.Glyph-2])
		}
		if !restoreVDOCSNumberProjection(&imported, *p, value, n.Ancestors, projected) || !reflect.DeepEqual(&imported, n) {
			t.Fatal("format projection failed to retain source semantics")
		}
	})
}

func TestVDOCSProjectedFormatCanonicalMemberNames(t *testing.T) {
	for _, raw := range []string{`null`, `[]`, `{"01":"upperLetter"}`, `{"-1":"upperLetter"}`, `{"9":"upperLetter"}`, `{"0":null}`, `{"0":3}`, `{"+1":"upperLetter","1":"lowerRoman"}`} {
		var formats vdocsProjectedFormats
		if err := json.Unmarshal([]byte(raw), &formats); err == nil {
			t.Fatal("ambiguous/malformed native format keys accepted", raw)
		}
	}
}
