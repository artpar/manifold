package engine

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
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

	// Marker references must participate in the same identity validation as bodies.

	// JSON gives each negative case independent ownership of its node slices.

	// A valid symbolic SF value need not allocate its enormous alphabetic label.

	// This assertion checks painted marker strings. Logical list semantics
	// are checked separately by TestGeneratedPDFNumberedListSemantics.

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
func literalMarkerDocument() *Document {
	return &Document{Blocks: []Block{{Kind: ListBlock, List: "number", Items: []ListItem{
		{Marker: &ListMarker{Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "α)", Bold: true, Link: "https://example.org/alpha"}}}}}, Blocks: []Block{mergeParagraph("FIRST")}},
		{Marker: &ListMarker{Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Anchor: "marker-point"}, {Text: "β)", Italic: true}, {Note: &NoteReference{Kind: Footnote, ID: "label-note"}}}}}}, Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "SECOND", Destination: "marker-point"}}}}},
		{Marker: &ListMarker{Blocks: []Block{{Kind: Paragraph}}}, Blocks: []Block{mergeParagraph("EMPTY")}},
	}}}, Notes: []Note{{Kind: Footnote, ID: "label-note", Blocks: []Block{mergeParagraph("Marker note")}}}}
}

func TestLiteralMarkerSFJSONGzipAndIdentityGraphs(t *testing.T) {
	ctx := context.Background()
	d := literalMarkerDocument()
	before := canonicalSF(t, d)
	for _, format := range []Format{SF, SFGzip, JSON} {
		var out bytes.Buffer
		if err := Write(ctx, &out, d, format, WriteOptions{}); err != nil {
			t.Fatal(format, err)
		}
		if dir := os.Getenv("MANYDOC_LITERAL_MARKER_QA"); dir != "" && format == SF {
			if err := os.MkdirAll(dir, 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "literal-markers.sf"), out.Bytes(), 0600); err != nil {
				t.Fatal(err)
			}
		}
		back, err := Read(ctx, &out, format, ReadOptions{})
		if err != nil {
			t.Fatal(format, err)
		}
		if !bytes.Equal(before, canonicalSF(t, back)) {
			t.Fatal(format, "literal marker graph changed")
		}
	}
	if !bytes.Equal(before, canonicalSF(t, d)) {
		t.Fatal("caller mutated")
	}

	d.Notes = nil
	if err := d.Validate(); err == nil {
		t.Fatal("dangling marker note accepted")
	}
	d = literalMarkerDocument()
	d.Headers = d.Blocks
	d.Blocks = nil
	if err := d.Validate(); err == nil {
		t.Fatal("marker note in running story accepted")
	}
	d = literalMarkerDocument()
	d.Notes[0].Blocks = d.Blocks
	if err := d.Validate(); err == nil {
		t.Fatal("marker note in note body accepted")
	}

	d = literalMarkerDocument()
	d.Blocks[0].Items[0].Marker.Blocks[0].Runs = append(d.Blocks[0].Items[0].Marker.Blocks[0].Runs, Run{Anchor: "marker-point"})
	if err := d.Validate(); err == nil {
		t.Fatal("duplicate marker/body anchor accepted")
	}
}

func TestLiteralMarkerNativePublicGeneration(t *testing.T) {
	want := resolveNumberingDocument(literalMarkerDocument())
	var out bytes.Buffer
	if err := WriteVDOCS(context.Background(), &out, want); err != nil {
		t.Fatal(err)
	}
	got, err := ReadVDOCS(context.Background(), &out)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(canonicalSF(t, want), canonicalSF(t, got)) {
		t.Fatal("public native writer changed actual label content/relationships")
	}
}

func TestLiteralMarkerAdmission(t *testing.T) {
	for _, marker := range [][]Block{{}, {{Kind: PageBreak}}, {{Kind: Paragraph, PageBreakBefore: true}}, {{Kind: AnchorPoint, Anchors: []string{"a"}}}} {
		d := literalMarkerDocument()
		d.Blocks[0].Items[0].Marker = &ListMarker{Blocks: marker}
		if err := d.Validate(); err == nil {
			t.Fatal("invalid literal marker accepted", marker)
		}
	}
	d := literalMarkerDocument()
	source := &SourceInfo{Format: PDF, Structure: "explicit", Role: "Lbl", Element: "marker", Confidence: 1, Evidence: "test_marker"}
	d.Blocks[0].Items[0].Marker.Source = source
	if !d.hasPDFSource() {
		t.Fatal("marker source not admitted to fidelity gate")
	}
	sf, err := ToStructured(d)
	if err != nil {
		t.Fatal(err)
	}
	back, err := FromStructured(sf)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(back.Blocks[0].Items[0].Marker.Source, source) {
		t.Fatal("marker source changed")
	}
}

func TestPDFVariableLiteralLabelsKeepExplicitHierarchy(t *testing.T) {
	for _, family := range []string{"None", "UpperAlpha"} {
		t.Run(family, func(t *testing.T) {
			raw := taggedTestPDF(markedText(0, 20, 300, "[done]")+markedText(1, 80, 300, "FIRST")+markedText(2, 20, 280, "[later]")+markedText(3, 80, 280, "SECOND"),
				"<< /Type /StructTreeRoot /K 8 0 R >>",
				"<< /Type /StructElem /S /L /P 7 0 R /Pg 3 0 R /A << /O /List /ListNumbering /"+family+" >> /K [9 0 R 10 0 R] >>",
				"<< /Type /StructElem /S /LI /P 8 0 R /K [11 0 R 12 0 R] >>",
				"<< /Type /StructElem /S /LI /P 8 0 R /K [13 0 R 14 0 R] >>",
				"<< /Type /StructElem /S /Lbl /P 9 0 R /K 0 >>",
				"<< /Type /StructElem /S /LBody /P 9 0 R /K 1 >>",
				"<< /Type /StructElem /S /Lbl /P 10 0 R /K 2 >>",
				"<< /Type /StructElem /S /LBody /P 10 0 R /K 3 >>")
			d := readRecovery(t, raw)
			if len(d.Blocks) != 1 || d.Blocks[0].Kind != ListBlock || len(d.Blocks[0].Items) != 2 || d.Blocks[0].Source.Role != "L" || !hasDiagnostic(d, "pdf_list_literal") {
				t.Fatal("explicit list discarded")
			}
			for i, item := range d.Blocks[0].Items {
				if item.Marker == nil || item.Marker.Source == nil || item.Marker.Source.Role != "Lbl" || item.Marker.Source.ParentElement != item.Source.Element || item.Source.ParentElement != d.Blocks[0].Source.Element {
					t.Fatal("literal label/item/list source ownership lost")
				}
				if paragraphText(item.Marker.Blocks[0]) != []string{"[done]", "[later]"}[i] || paragraphText(item.Blocks[0]) != []string{"FIRST", "SECOND"}[i] {
					t.Fatal("literal label/body correspondence changed")
				}
			}
			if family == "None" && (d.Blocks[0].List != "bullet" || !hasDiagnostic(d, "pdf_list_kind_unobserved")) {
				t.Fatal("unproven automatic counter invented")
			}
			if family == "UpperAlpha" && (d.Blocks[0].List != "number" || !strings.Contains(d.Blocks[0].Source.Evidence, "start_unobserved")) {
				t.Fatal("explicit ordered family or counter uncertainty lost")
			}
			assertRecoverySF(t, d)
			exportLiteralMarkerQA(t, "variable-"+family, raw, d)
		})
	}
}

func TestLiteralMarkerSchemaAdmission(t *testing.T) {
	sf, err := ToStructured(literalMarkerDocument())
	if err != nil {
		t.Fatal(err)
	}
	label := sf.Content[0].Content[0].Content[0]
	if label.Type != "list_marker" {
		t.Fatal("literal marker not encoded as its own node")
	}
	for _, mutate := range []func(*StructuredDocument){
		func(d *StructuredDocument) { d.Content = append(d.Content, label) },
		func(d *StructuredDocument) {
			d.Content[0].Content[0].Content = append(d.Content[0].Content[0].Content, label)
		},
		func(d *StructuredDocument) { d.Content[0].Content[0].Content[0].Content = nil },
		func(d *StructuredDocument) { d.Content[0].Content[0].Content[0].Text = "hidden" },
	} {

		raw, _ := json.Marshal(sf)
		var candidate StructuredDocument
		if err = json.Unmarshal(raw, &candidate); err != nil {
			t.Fatal(err)
		}
		mutate(&candidate)
		if _, err = FromStructured(&candidate); err == nil {
			t.Fatal("invalid marker placement/payload accepted")
		}
	}
}

func exportLiteralMarkerQA(t *testing.T, name string, raw []byte, d *Document) {
	t.Helper()
	dir := os.Getenv("MANYDOC_LITERAL_MARKER_QA")
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	tree, err := ToStructured(d)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err = WriteStructured(context.Background(), &out, tree); err != nil {
		t.Fatal(err)
	}
	for ext, data := range map[string][]byte{"pdf": raw, "sf": out.Bytes()} {
		if err = os.WriteFile(filepath.Join(dir, name+"."+ext), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func FuzzLiteralMarkerSF(f *testing.F) {
	f.Add("α)", "First 😀")
	f.Add("", "")
	f.Add("[done]\ncontinued", "Second")
	f.Fuzz(func(t *testing.T, label, body string) {
		if len(label) > 2048 || len(body) > 2048 {
			t.Skip()
		}
		d := &Document{Blocks: []Block{{Kind: ListBlock, List: "bullet", Items: []ListItem{{Marker: &ListMarker{Blocks: []Block{{Kind: Paragraph, Anchors: []string{"marker"}, Runs: []Run{{Text: label, Bold: true, Link: "https://example.org/marker"}}}}}, Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: body, Destination: "marker"}}}}}}}}}
		if err := d.Validate(); err != nil {
			return
		}
		before := canonicalSF(t, d)
		var out bytes.Buffer
		if err := Write(context.Background(), &out, d, SF, WriteOptions{}); err != nil {
			t.Fatal(err)
		}
		got, err := Read(context.Background(), &out, SF, ReadOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(before, canonicalSF(t, got)) {
			t.Fatal("literal marker text/style/anchor/URI graph changed")
		}
	})
}
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
