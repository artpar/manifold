package manifold

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"reflect"
	"strings"
	"testing"
)

func listItem(text string) ListItem { return ListItem{Blocks: []Block{mergeParagraph(text)}} }
func listDocument() *Document {
	return &Document{Blocks: []Block{
		{Kind: ListBlock, List: "number", Start: listStartPointer(3), Items: []ListItem{
			{Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "Parent three", Bold: true, Link: "https://example.invalid/list"}}}, mergeParagraph("Continuation paragraph"),
				{Kind: ListBlock, List: "bullet", Items: []ListItem{listItem("Nested bullet one"), {Blocks: []Block{mergeParagraph("Nested bullet two"), {Kind: ListBlock, List: "number", Start: listStartPointer(7), Items: []ListItem{listItem("Deep seven")}}}}}},
				mergeParagraph("After nested list")}},
			listItem("Parent four"),
		}},
		{Kind: ListBlock, List: "number", Start: listStartPointer(0), Items: []ListItem{listItem("Restart zero"), listItem("Then one")}},
		{Kind: ListBlock, List: "number", Start: listStartPointer(7), Items: []ListItem{listItem("Separate seven")}},
	}}
}
func TestListTreeSemanticsAndWriters(t *testing.T) {
	ctx := context.Background()
	d := listDocument()
	s, err := ToStructured(d)
	if err != nil {
		t.Fatal(err)
	}
	shared, err := FromStructured(s)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(d, shared) {
		t.Fatal("SF list hierarchy, starts, formatting or item blocks changed")
	}
	for _, format := range []Format{SF, JSON, DOCX, PDF} {
		t.Run(string(format), func(t *testing.T) {
			var out bytes.Buffer
			if err := Write(ctx, &out, shared, format, WriteOptions{}); err != nil {
				t.Fatal(err)
			}
			actual, err := Read(ctx, &out, format, ReadOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if format != PDF {
				got, err := ToStructured(actual)
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(s.Content, got.Content) {
					a, _ := json.Marshal(s.Content)
					b, _ := json.Marshal(got.Content)
					t.Fatalf("list tree changed\nwant %s\ngot %s", a, b)
				}
				if len(actual.Diagnostics) > 0 {
					t.Fatalf("own list output has spurious losses: %#v", actual.Diagnostics)
				}
			} else {
				for _, marker := range []string{"Parent three", "Continuation paragraph", "Nested bullet one", "Nested bullet two", "Deep seven", "After nested list", "Parent four", "Restart zero", "Then one", "Separate seven"} {
					if strings.Count(actual.Text(), marker) != 1 {
						t.Fatalf("missing/repeated %s", marker)
					}
				}
				// Inferred list markers are structural labels, not duplicate
				// body text. Retain the original marker oracle across both.
				labels := actual.Text()
				var walkLabels func([]Block)
				walkLabels = func(bs []Block) {
					for _, b := range bs {
						for _, item := range b.Items {
							if item.Source != nil {
								labels += "\n" + item.Source.Label
							}
							walkLabels(item.Blocks)
						}
					}
				}
				walkLabels(actual.Blocks)
				for _, marker := range []string{"3.", "4.", "0.", "1.", "7."} {
					if !strings.Contains(labels, marker) {
						t.Fatalf("missing list value %s", marker)
					}
				}
			}
		})
	}
}
func TestLegacyListAdapter(t *testing.T) {
	legacy := &Document{Blocks: []Block{{Kind: Paragraph, List: "number", Runs: []Run{{Text: "one"}}}, {Kind: Paragraph, List: "number", Runs: []Run{{Text: "two"}}}, mergeParagraph("gap"), {Kind: Paragraph, List: "number", Runs: []Run{{Text: "restart"}}}}}
	original, _ := json.Marshal(legacy)
	s, err := ToStructured(legacy)
	if err != nil {
		t.Fatal(err)
	}
	d, err := FromStructured(s)
	if err != nil {
		t.Fatal(err)
	}
	expected := []Block{{Kind: ListBlock, List: "number", Items: []ListItem{listItem("one"), listItem("two")}}, mergeParagraph("gap"), {Kind: ListBlock, List: "number", Items: []ListItem{listItem("restart")}}}
	if !reflect.DeepEqual(d.Blocks, expected) {
		t.Fatal("legacy list grouping changed")
	}
	after, _ := json.Marshal(legacy)
	if !bytes.Equal(original, after) {
		t.Fatal("adapter mutated caller")
	}
}

func numberedDOCX(t testing.TB, body, numbering, styles string) []byte {
	t.Helper()
	source := docxWithBody(t, body)
	zr, err := zip.NewReader(bytes.NewReader(source), int64(len(source)))
	if err != nil {
		t.Fatal(err)
	}
	var result bytes.Buffer
	z := zip.NewWriter(&result)
	for _, f := range zr.File {
		rc, _ := f.Open()
		data, _ := io.ReadAll(rc)
		rc.Close()
		w, _ := z.Create(f.Name)
		w.Write(data)
	}
	for name, data := range map[string]string{
		"word/numbering.xml":           `<w:numbering xmlns:w="` + wordNS + `">` + numbering + `</w:numbering>`,
		"word/styles.xml":              `<w:styles xmlns:w="` + wordNS + `">` + styles + `</w:styles>`,
		"word/_rels/document.xml.rels": `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="num" Type="` + relNS + `/numbering" Target="numbering.xml"/><Relationship Id="style" Type="` + relNS + `/styles" Target="styles.xml"/></Relationships>`,
	} {
		w, err := z.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = w.Write([]byte(data)); err != nil {
			t.Fatal(err)
		}
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	return result.Bytes()
}

const testListNumbering = `<w:abstractNum w:abstractNumId="9"><w:multiLevelType w:val="multilevel"/>
<w:lvl w:ilvl="0"><w:start w:val="1"/><w:numFmt w:val="decimal"/><w:lvlText w:val="%1."/></w:lvl>
<w:lvl w:ilvl="1"><w:start w:val="4"/><w:numFmt w:val="decimal"/><w:lvlText w:val="%2."/></w:lvl>
</w:abstractNum><w:num w:numId="10"><w:abstractNumId w:val="9"/><w:lvlOverride w:ilvl="0"><w:startOverride w:val="3"/></w:lvlOverride></w:num>
<w:num w:numId="11"><w:abstractNumId w:val="9"/></w:num>`

func numberedParagraph(id, level, text string) string {
	return `<w:p><w:pPr><w:numPr><w:ilvl w:val="` + level + `"/><w:numId w:val="` + id + `"/></w:numPr></w:pPr><w:r><w:t>` + text + `</w:t></w:r></w:p>`
}
func TestDOCXNumberingHierarchyOverridesAndContinuation(t *testing.T) {
	body := numberedParagraph("10", "0", "Parent3") + numberedParagraph("10", "1", "Child4") + numberedParagraph("10", "1", "Child5") + numberedParagraph("10", "0", "Parent4") + numberedParagraph("10", "1", "ChildAgain") + numberedParagraph("10", "0", "Parent5") + `<w:p><w:r><w:t>gap</w:t></w:r></w:p>` + numberedParagraph("10", "0", "Parent6") + numberedParagraph("11", "0", "Restart1")
	for _, neverRestart := range []bool{false, true} {
		numbering := testListNumbering
		if neverRestart {
			numbering = strings.Replace(numbering, `<w:start w:val="4"/>`, `<w:start w:val="4"/><w:lvlRestart w:val="0"/>`, 1)
		}
		d, err := ReadDOCX(context.Background(), bytes.NewReader(numberedDOCX(t, body, numbering, "")))
		if err != nil {
			t.Fatal(err)
		}
		childStart := 4
		if neverRestart {
			childStart = 6
		}
		expected := []Block{
			{Kind: ListBlock, List: "number", Start: listStartPointer(3), Items: []ListItem{
				{Blocks: []Block{mergeParagraph("Parent3"), {Kind: ListBlock, List: "number", Start: listStartPointer(4), Items: []ListItem{listItem("Child4"), listItem("Child5")}}}},
				{Blocks: []Block{mergeParagraph("Parent4"), {Kind: ListBlock, List: "number", Start: listStartPointer(childStart), Items: []ListItem{listItem("ChildAgain")}}}}, listItem("Parent5"),
			}}, mergeParagraph("gap"), {Kind: ListBlock, List: "number", Start: listStartPointer(6), Items: []ListItem{listItem("Parent6")}},
			{Kind: ListBlock, List: "number", Items: []ListItem{listItem("Restart1")}},
		}
		// Preserve the original hierarchy/start/restart/text oracle and add the
		// literal definitions/counter contexts from the input OOXML above.
		for _, i := range []int{0, 2, 3} {
			expected[i].Numbering = &ListNumbering{Level: 0, Template: "%1.", Formats: []string{"decimal"}}
		}
		for index, parent := range []int{3, 4} {
			expected[0].Items[index].Blocks[1].Numbering = &ListNumbering{Level: 1, Template: "%2.", Formats: []string{"decimal", "decimal"}, Ancestors: []int{parent}}
		}
		if !reflect.DeepEqual(d.Blocks, expected) {
			a, _ := json.Marshal(d.Blocks)
			b, _ := json.Marshal(expected)
			t.Fatalf("numbering changed (never restart %v)\nwant %s\ngot %s", neverRestart, b, a)
		}
	}
}
func TestDOCXNumberingInheritanceCancellationAndLosses(t *testing.T) {
	styles := `<w:style w:styleId="Base"><w:pPr><w:numPr><w:numId w:val="10"/></w:numPr></w:pPr></w:style><w:style w:styleId="Derived"><w:basedOn w:val="Base"/></w:style>`
	body := `<w:p><w:pPr><w:pStyle w:val="Derived"/></w:pPr><w:r><w:t>Inherited</w:t></w:r></w:p><w:p><w:pPr><w:pStyle w:val="Derived"/><w:numPr><w:numId w:val="0"/></w:numPr></w:pPr><w:r><w:t>Cancelled</w:t></w:r></w:p>`
	d, err := ReadDOCX(context.Background(), bytes.NewReader(numberedDOCX(t, body, testListNumbering, styles)))
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Blocks) != 2 || d.Blocks[0].Kind != ListBlock || listStart(d.Blocks[0]) != 3 || d.Blocks[1].Kind != Paragraph {
		t.Fatal("style inheritance or numId cancellation lost")
	}
	bad := strings.Replace(testListNumbering, `w:val="decimal"`, `w:val="ordinal"`, 1)
	d, err = ReadDOCX(context.Background(), bytes.NewReader(numberedDOCX(t, body, bad, styles)))
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Diagnostics) != 1 || d.Diagnostics[0].Code != "numbering_format" {
		t.Fatal("numbering representation loss hidden")
	}
	for _, body := range []string{numberedParagraph("999", "0", "Missing"), numberedParagraph("10", "9", "BadLevel")} {
		if _, err = ReadDOCX(context.Background(), bytes.NewReader(numberedDOCX(t, body, testListNumbering, ""))); err == nil {
			t.Fatal("bad numbering accepted")
		}
	}
}
func TestListValidation(t *testing.T) {
	for _, b := range []Block{
		{Kind: ListBlock, List: "number"},
		{Kind: ListBlock, List: "bullet", Start: listStartPointer(0), Items: []ListItem{listItem("bad")}},
		{Kind: ListBlock, List: "number", Start: listStartPointer(-1), Items: []ListItem{listItem("bad")}},
		{Kind: ListBlock, List: "number", Start: listStartPointer(2147483647), Items: []ListItem{listItem("bad"), listItem("overflow")}},
		{Kind: ListBlock, List: "number", Items: []ListItem{{Blocks: []Block{{Kind: Picture, Image: &Image{Data: []byte{1}}}}}}},
		{Kind: Paragraph, Items: []ListItem{listItem("bad")}},
	} {
		if err := (&Document{Blocks: []Block{b}}).Validate(); err == nil {
			t.Fatal("invalid list accepted")
		}
	}
}

func TestListItemAssetsAndTableMembership(t *testing.T) {
	imported := readFixture(t, "docx_image.docx")
	var picture Block
	for _, b := range imported.Blocks {
		if b.Kind == Picture {
			picture = b
			break
		}
	}
	if picture.Image == nil {
		t.Fatal("image fixture missing")
	}
	table := Block{Kind: Table, Rows: []Row{{Cells: []Cell{{ColSpan: 1, Blocks: []Block{mergeParagraph("Item cell")}}}}}}
	d := &Document{Blocks: []Block{{Kind: ListBlock, List: "number", Start: listStartPointer(5), Items: []ListItem{{Blocks: []Block{mergeParagraph("Item assets"), picture, table, mergeParagraph("Still same item")}}, listItem("Next item")}}}}
	expected, err := ToStructured(d)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, DOCX, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	got, err := ReadDOCX(context.Background(), &out)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := ToStructured(got)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(expected.Content, actual.Content) {
		t.Fatal("list-item image, table or membership changed")
	}
}
func rewriteListDOCX(t *testing.T, source []byte, change func(string) string) []byte {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(source), int64(len(source)))
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	z := zip.NewWriter(&out)
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		if f.Name == "word/document.xml" {
			data = []byte(change(string(data)))
		}
		w, err := z.Create(f.Name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = w.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}
func TestListRangesRejectInvalidMetadata(t *testing.T) {
	var out bytes.Buffer
	if err := Write(context.Background(), &out, listDocument(), DOCX, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(string) string{
		func(s string) string { return strings.Replace(s, `mdlist1_number_3_1`, `mdlist1_number_9_1`, 1) },
		func(s string) string { return strings.Replace(s, `<w:bookmarkEnd w:id="1"/>`, "", 1) },
		func(s string) string { return strings.Replace(s, `mditem1_2`, `mditem1_999`, 1) },
		func(s string) string {
			return strings.Replace(s, `<w:bookmarkEnd w:id="2"/>`, `<w:bookmarkEnd w:id="1"/>`, 1)
		},
	} {
		data := rewriteListDOCX(t, out.Bytes(), change)
		if _, err := ReadDOCX(context.Background(), bytes.NewReader(data)); err == nil {
			t.Fatal("ambiguous or contradictory list ranges accepted")
		}
	}
}
func TestPDFListPaginationAndDepth(t *testing.T) {
	d := listDocument()
	d.Layout = Layout{Width: 250, Height: 130, Margin: 20}
	d.Blocks[0].Items[0].Blocks[0].Runs[0].Text = "Parent three " + strings.Repeat("wrapping words ", 24)
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, PDF, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	got, err := ReadPDF(context.Background(), &out, ReadOptions{PDFSkipStructure: true})
	if err != nil {
		t.Fatal(err)
	}
	if got.Metadata.PageCount < 2 {
		t.Fatal("long list did not paginate")
	}
	if strings.Count(got.Text(), "3.") != 1 || strings.Count(got.Text(), "Parent four") != 1 || strings.Count(got.Text(), "wrapping words") != 24 {
		t.Fatal("list pagination lost/repeated marker or content")
	}
	// DOCX has a published nine-level numbering limit. SF/PDF retain deeper
	// trees until their own geometry/validation bounds; DOCX rejects explicitly.
	deep := Block{Kind: ListBlock, List: "bullet", Items: []ListItem{listItem("leaf")}}
	for i := 0; i < 9; i++ {
		deep = Block{Kind: ListBlock, List: "bullet", Items: []ListItem{{Blocks: []Block{mergeParagraph("level"), deep}}}}
	}
	if err := Write(context.Background(), &bytes.Buffer{}, &Document{Blocks: []Block{deep}}, DOCX, WriteOptions{}); err == nil {
		t.Fatal("DOCX numbering depth overflow accepted")
	}
}

func TestNumberedParagraphInlineImagePreserved(t *testing.T) {
	source := readFixture(t, "docx_image.docx")
	var picture Block
	for _, b := range source.Blocks {
		if b.Kind == Picture {
			picture = b
			break
		}
	}
	d := &Document{Blocks: []Block{{Kind: ListBlock, List: "number", Items: []ListItem{{Blocks: []Block{mergeParagraph("Image item"), picture}}}}}}
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, DOCX, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	// Independent editing of OOXML puts the drawing in the numbered paragraph.
	// The model must retain both its number and asset, with a placement diagnostic.
	boundary := `</w:p><w:p><w:pPr><w:ind w:left="360"/></w:pPr>`
	changed := false
	data := rewriteListDOCX(t, out.Bytes(), func(s string) string {
		changed = strings.Contains(s, boundary)
		return strings.Replace(s, boundary, "", 1)
	})
	if !changed {
		t.Fatal("drawing paragraph boundary not found")
	}
	imported, err := ReadDOCX(context.Background(), bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	expected, _ := ToStructured(d)
	actual, err := ToStructured(imported)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(expected.Content, actual.Content) {
		t.Fatal("numbered paragraph image asset or item membership lost")
	}
	if len(imported.Diagnostics) != 1 || imported.Diagnostics[0].Code != "list_inline_layout" {
		t.Fatal("inline placement normalization not disclosed")
	}
}
