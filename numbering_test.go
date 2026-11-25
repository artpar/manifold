package manifold

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestNumberFormatLiterals(t *testing.T) {
	for _, c := range []struct {
		f    string
		n    int
		want string
	}{
		{"decimalZero", 0, "0"}, {"decimalZero", 1, "01"}, {"decimalZero", 12, "12"},
		{"upperLetter", 0, "0"}, {"upperLetter", 26, "Z"}, {"upperLetter", 27, "AA"}, {"upperLetter", 28, "BB"}, {"lowerLetter", 52, "zz"}, {"lowerLetter", 53, "aaa"},
		{"upperRoman", 4, "IV"}, {"lowerRoman", 9, "ix"}, {"upperRoman", 3999, "MMMCMXCIX"}, {"lowerRoman", 4000, "mmmm"},
	} {
		got, err := numberText(c.f, c.n)
		if err != nil || got != c.want {
			t.Fatal(c, got, err)
		}
	}
}

func TestNumberingAdmissionAndPaintBudget(t *testing.T) {
	for _, n := range []*ListNumbering{
		{Level: -1}, {Level: 9}, {Template: "%1", Formats: []string{"ordinal"}},
		{Level: 1, Template: "%1.%2", Formats: []string{"decimal", "decimal"}},
		{Level: 1, Template: "%1.%2", Formats: []string{"decimal", "decimal"}, Ancestors: []int{-1}},
	} {
		d := &Document{Blocks: []Block{{Kind: ListBlock, List: "number", Numbering: n, Items: []ListItem{listItem("KEPT")}}}}
		var out bytes.Buffer
		if err := Write(context.Background(), &out, d, SF, WriteOptions{}); err == nil || out.Len() != 0 {
			t.Fatal("invalid numbering emitted", n, err)
		}
	}
	// A valid symbolic SF value need not allocate its enormous alphabetic label.
	d := &Document{Blocks: []Block{{Kind: ListBlock, List: "number", Start: listStartPointer(2147483647), Numbering: &ListNumbering{Template: "%1.", Formats: []string{"upperLetter"}}, Items: []ListItem{listItem("KEPT")}}}}
	var sf, out bytes.Buffer
	if err := Write(context.Background(), &sf, d, SF, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := Write(context.Background(), &out, d, PDF, WriteOptions{}); err == nil || out.Len() != 0 {
		t.Fatal("oversized label painted", err)
	}
	d.Blocks[0].Start = nil
	d.Blocks[0].Numbering.Template = "%1 %9 %0 %"
	if got, err := numberedLabel(d.Blocks[0], 0); err != nil || got != "A  %0 %" {
		t.Fatal(got, err)
	}
}

func TestNumberingAdmissionAndLiteralTemplate(t *testing.T) {
	for _, n := range []*ListNumbering{
		{Level: -1}, {Level: 9}, {Formats: []string{"ordinal"}, Template: "%1"},
		{Formats: []string{"decimal"}, Template: strings.Repeat("x", 513)},
		{Level: 1, Formats: []string{"decimal", "decimal"}, Template: "%1.%2"},
		{Level: 1, Formats: []string{"decimal", "decimal"}, Ancestors: []int{-1}},
	} {
		d := &Document{Blocks: []Block{{Kind: ListBlock, List: "number", Numbering: n, Items: []ListItem{listItem("KEPT")}}}}
		var out bytes.Buffer
		if err := Write(context.Background(), &out, d, SF, WriteOptions{}); err == nil || out.Len() != 0 {
			t.Fatal("invalid numbering admitted", n, err)
		}
	}
	b := Block{Kind: ListBlock, List: "number", Numbering: &ListNumbering{Formats: []string{"decimal"}, Template: "%1 %9 %0 %"}, Items: []ListItem{listItem("KEPT")}}
	if got, err := numberedLabel(b, 0); err != nil || got != "1  %0 %" {
		t.Fatal(got, err)
	}
	for _, attrs := range []string{`md:numberingImplicit="1"`, `md:numberingFormats='["lowerLetter"]'`} {
		defs := `<w:abstractNum w:abstractNumId="1"><w:lvl xmlns:md="` + docxNotesNS + `" w:ilvl="0" ` + attrs + `><w:start w:val="1"/><w:numFmt w:val="lowerLetter"/><w:lvlText w:val="%1)"/></w:lvl></w:abstractNum><w:num w:numId="1"><w:abstractNumId w:val="1"/></w:num>`
		if _, err := ReadDOCX(context.Background(), bytes.NewReader(numberedDOCX(t, numberedParagraph("1", "0", "KEPT"), defs, ""))); err == nil {
			t.Fatal("contradictory numbering metadata admitted", attrs)
		}
	}
}

func TestDOCXEmptyOverrideAndIndependentCounterContext(t *testing.T) {
	defs := testListNumbering + `<w:abstractNum w:abstractNumId="20"><w:lvl w:ilvl="0"><w:start w:val="8"/><w:numFmt w:val="decimal"/><w:lvlText w:val="%1"/></w:lvl><w:lvl w:ilvl="1"><w:start w:val="1"/><w:numFmt w:val="decimal"/><w:lvlText w:val="%1.%2"/></w:lvl></w:abstractNum><w:num w:numId="20"><w:abstractNumId w:val="20"/></w:num>`
	body := numberedParagraph("10", "0", "PARENT") + numberedParagraph("20", "1", "INDEPENDENT")
	d, err := ReadDOCX(context.Background(), bytes.NewReader(numberedDOCX(t, body, defs, "")))
	if err != nil {
		t.Fatal(err)
	}
	s, err := ToStructured(d)
	if err != nil {
		t.Fatal(err)
	}
	back, err := FromStructured(s)
	if err != nil {
		t.Fatal(err)
	}
	child := back.Blocks[0].Items[0].Blocks[1]
	if got, err := numberedLabel(child, 0); err != nil || got != "8.1" || !child.Numbering.Independent {
		t.Fatal(got, err, child.Numbering)
	}
	defs = strings.Replace(defs, `<w:abstractNumId w:val="20"/>`, `<w:abstractNumId w:val="20"/><w:lvlOverride w:ilvl="1"/>`, 1)
	d, err = ReadDOCX(context.Background(), bytes.NewReader(numberedDOCX(t, body, defs, "")))
	if err != nil {
		t.Fatal(err)
	}
	child = d.Blocks[0].Items[0].Blocks[1]
	if got, err := numberedLabel(child, 0); err != nil || got != "8.0" || !hasDiagnostic(d, "numbering_empty_override") {
		t.Fatal(got, err, d.Diagnostics)
	}
}

func numberingDocument() *Document {
	child := Block{Kind: ListBlock, List: "number", Start: listStartPointer(4), Numbering: &ListNumbering{Level: 1, Template: "%1.%2)", Formats: []string{"upperLetter", "lowerRoman"}, Ancestors: []int{99}}, Items: []ListItem{listItem("CHILD FOUR"), listItem("CHILD FIVE")}}
	return &Document{Layout: Layout{Width: 400, Height: 500, Margin: 35}, Blocks: []Block{
		{Kind: ListBlock, List: "number", Start: listStartPointer(27), Numbering: &ListNumbering{Template: "Appendix %1.", Formats: []string{"upperLetter"}}, Items: []ListItem{{Blocks: []Block{mergeParagraph("PARENT"), child}}}},
		{Kind: ListBlock, List: "number", Start: listStartPointer(7), Numbering: &ListNumbering{Level: 2, Template: "%1.%2.%3", Formats: []string{"upperLetter", "lowerRoman", "decimalZero"}, Ancestors: []int{2, 4}, Legal: true}, Items: []ListItem{listItem("EXTERNAL CONTEXT")}},
	}}
}

func TestNumberingNestedEditsAndFourPaths(t *testing.T) {
	d := numberingDocument()
	before, _ := json.Marshal(d)
	s, err := ToStructured(d)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := FromStructured(s)
	if err != nil {
		t.Fatal(err)
	}
	child := resolved.Blocks[0].Items[0].Blocks[1]
	for i, want := range []string{"AA.iv)", "AA.v)"} {
		got, err := numberedLabel(child, i)
		if err != nil || got != want {
			t.Fatal(got, err)
		}
	}
	if got, err := numberedLabel(resolved.Blocks[1], 0); err != nil || got != "2.4.7" {
		t.Fatal(got, err)
	}
	for _, format := range []Format{SF, DOCX, PDF} {
		var out bytes.Buffer
		if err := Write(context.Background(), &out, d, format, WriteOptions{}); err != nil {
			t.Fatal(format, err)
		}
		// This assertion checks painted marker strings. Logical list semantics
		// are checked separately by TestGeneratedPDFNumberedListSemantics.
		back, err := Read(context.Background(), &out, format, ReadOptions{PDFSkipStructure: true, PDFSkipInference: true})
		if err != nil {
			t.Fatal(format, err)
		}
		if format != PDF {
			actual, err := ToStructured(back)
			if err != nil || !reflect.DeepEqual(s.Content, actual.Content) {
				t.Fatal("numbering semantics changed", format, err)
			}
		} else {
			for _, label := range []string{"Appendix AA.", "AA.iv)", "AA.v)", "2.4.7"} {
				if !bytes.Contains([]byte(back.Text()), []byte(label)) {
					t.Fatal("PDF label lost", label, back.Text())
				}
			}
		}
	}
	after, _ := json.Marshal(d)
	if !bytes.Equal(before, after) {
		t.Fatal("caller context mutated")
	}
	d.Blocks[0].Start = listStartPointer(28)
	resolved = resolveNumberingDocument(d)
	if got, _ := numberedLabel(resolved.Blocks[0].Items[0].Blocks[1], 0); got != "BB.iv)" {
		t.Fatal("parent edit did not renumber", got)
	}
	d.Blocks[0].Items[0].Blocks[1].Numbering.Independent = true
	resolved = resolveNumberingDocument(d)
	if got, _ := numberedLabel(resolved.Blocks[0].Items[0].Blocks[1], 0); got != "UUUU.iv)" {
		t.Fatal("independent context overwritten", got)
	}
}

func TestDOCXSourceNumberTemplates(t *testing.T) {
	defs := `<w:abstractNum w:abstractNumId="1"><w:lvl w:ilvl="0"><w:start w:val="2"/><w:numFmt w:val="upperLetter"/><w:lvlText w:val="Appendix %1."/></w:lvl><w:lvl w:ilvl="1"><w:start w:val="4"/><w:numFmt w:val="lowerRoman"/><w:lvlText w:val="%1.%2)"/></w:lvl></w:abstractNum><w:num w:numId="1"><w:abstractNumId w:val="1"/></w:num>`
	body := numberedParagraph("1", "0", "PARENT") + numberedParagraph("1", "1", "CHILD") + numberedParagraph("1", "0", "NEXT") + numberedParagraph("1", "1", "RESET")
	d, err := ReadDOCX(context.Background(), bytes.NewReader(numberedDOCX(t, body, defs, "")))
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Diagnostics) > 0 {
		t.Fatal(d.Diagnostics)
	}
	root := d.Blocks[0]
	if got, _ := numberedLabel(root, 0); got != "Appendix B." {
		t.Fatal(got)
	}
	if got, _ := numberedLabel(root.Items[0].Blocks[1], 0); got != "B.iv)" {
		t.Fatal(got)
	}
	if got, _ := numberedLabel(root.Items[1].Blocks[1], 0); got != "C.iv)" {
		t.Fatal(got)
	}
}
