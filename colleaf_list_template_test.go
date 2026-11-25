package manifold

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestColleafLiteralTemplateProjection(t *testing.T) {
	for _, tc := range []struct{ input, format, symbol string }{
		{"%%1.", "%10%1.", "%"}, {"%10", "%1%10", "0"}, {"%1%", "%1%10", "%"},
		{"50% literal", "%10", "50% literal"}, {"%9%%1.", "%10%1.", "%"}, {"%1%90", "%1%10", "0"},
	} {
		source := colleafNativeList{Levels: []colleafListLevel{{Glyph: colleafInt(2)}}}
		format := projectColleafListTemplate(tc.input, 0, &source)
		source.Levels[0].Format = format
		if format != tc.format || len(source.Levels) != 10 || source.Levels[9].Symbol != tc.symbol {
			t.Fatalf("%q: %q %+v", tc.input, format, source.Levels)
		}
		binding := colleafListBinding{OriginalTemplate: &tc.input}
		got, err := readColleafListTemplate(source, 0, binding)
		if err != nil || got != tc.input {
			t.Fatal(got, err)
		}
		// Lost intent can still derive the actually represented logical template.
		got, err = readColleafListTemplate(source, 0, colleafListBinding{})
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(colleafSFListParts(got, 0), colleafSFListParts(tc.input, 0)) {
			t.Fatal("native helper semantics lost", got, tc.input)
		}
	}
	// Ignore a deeper placeholder without turning its surrounding literal text into
	// a false active placeholder. Only checked original intent expresses this case.
	original := "%%91"
	source := colleafNativeList{Levels: []colleafListLevel{{Glyph: colleafInt(2)}}}
	source.Levels[0].Format = projectColleafListTemplate(original, 0, &source)
	if got, err := readColleafListTemplate(source, 0, colleafListBinding{OriginalTemplate: &original}); err != nil || got != original {
		t.Fatal(got, err)
	}
	if _, err := readColleafListTemplate(source, 0, colleafListBinding{}); err == nil {
		t.Fatal("ambiguous literal percent-digit silently became a counter")
	}
}

func colleafLiteralListExample(t testing.TB) *Document {
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
func TestColleafLiteralListsSFAndActualSave(t *testing.T) {
	ctx := context.Background()
	want := resolveNumberingDocument(colleafLiteralListExample(t))
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
	again, err := encodeColleafDocument(ctx, model)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(again, wire) {
		t.Fatal("SF changed native helper/ownership/asset semantics")
	}
	if file := os.Getenv("MANYDOC_COLLEAF_LITERAL_LIST_QA"); file != "" {
		data, err := json.Marshal(wire)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(file, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if file := os.Getenv("MANYDOC_COLLEAF_LITERAL_LIST_SAVED"); file != "" {
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
			t.Fatal("actual facade changed literal list semantics", got.Diagnostics)
		}
	}
	if file := os.Getenv("MANYDOC_COLLEAF_LITERAL_LIST_EDITED"); file != "" {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var edited colleafNoteSnapshot
		if err = json.Unmarshal(data, &edited); err != nil {
			t.Fatal(err)
		}
		got, err = decodeColleafDocument(ctx, edited)
		if err != nil {
			t.Fatal(err)
		}
		// SF resolves enclosing counters over the stored fallback ancestor values.
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
func TestColleafNativeHelperEditsAndContradictions(t *testing.T) {
	ctx := context.Background()
	d := colleafLiteralListExample(t)
	for _, mode := range []string{"symbol", "missing", "badref", "activehelper", "constant"} {
		wire, err := encodeColleafDocument(ctx, d)
		if err != nil {
			t.Fatal(err)
		}
		var bullet colleafBullet
		_ = json.Unmarshal(wire.Body.Paragraphs[0].Bullet, &bullet)
		var source colleafNativeList
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
		got, err := decodeColleafDocument(ctx, wire)
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
func FuzzColleafListTemplate(f *testing.F) {
	for _, s := range []string{"%%1%", "%10", "%9%%1", "%%91", strings.Repeat("%1a%", 40)} {
		f.Add(s, uint8(0))
	}
	f.Fuzz(func(t *testing.T, template string, l uint8) {
		level := int(l % 9)
		if len(template) > 512 || validateString(template) != nil {
			return
		}
		source := colleafNativeList{}
		for range level + 1 {
			source.Levels = append(source.Levels, colleafListLevel{Glyph: colleafInt(2)})
		}
		source.Levels[level].Format = projectColleafListTemplate(template, level, &source)
		got, err := readColleafListTemplate(source, level, colleafListBinding{OriginalTemplate: &template})
		if err != nil || got != template {
			t.Fatal(got, err)
		}
		if len(source.Levels) > colleafMaxListEntries {
			t.Fatal("unbounded helpers")
		}
	})
}
