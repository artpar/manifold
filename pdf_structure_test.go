package manydoc

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
)

// Objects 1-7 are stable; the tests own every structural expectation rather
// than using manydoc's output as an oracle for its input.
func taggedTestPDF(content, root string, elements ...string) []byte {
	return recoveryPDF(taggedTestObjects(content, root, elements...)...)
}
func taggedTestObjects(content, root string, elements ...string) []string {
	objects := []string{"<< /Type /Catalog /Pages 2 0 R /StructTreeRoot 7 0 R /MarkInfo << /Marked true >> >>", "<< /Type /Pages /Kids [3 0 R] /Count 1 >>", "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 400] /Resources << /Font << /F 4 0 R >> /XObject << /X 6 0 R >> >> /StructParents 0 /Contents 5 0 R >>", "<< /Type /Font /Subtype /Type1 /BaseFont /Courier >>", recoveryStream("", content), recoveryStream("/Type /XObject /Subtype /Form /BBox [0 0 100 100] /StructParents 1", "BT /F 10 Tf /P << /MCID 0 >> BDC 10 20 Td (FORM) Tj EMC ET"), root}
	return append(objects, elements...)
}
func markedText(id, x, y int, text string) string {
	text = strings.NewReplacer(`\`, `\\`, `(`, `\(`, `)`, `\)`).Replace(text)
	return fmt.Sprintf("BT /F 10 Tf /P << /MCID %d >> BDC 1 0 0 1 %d %d Tm (%s) Tj EMC ET\n", id, x, y, text)
}
func hasDiagnostic(d *Document, code string) bool {
	for _, v := range d.Diagnostics {
		if v.Code == code {
			return true
		}
	}
	return false
}
func TestPDFTaggedParagraphsReadingOrderAndForms(t *testing.T) {
	content := markedText(1, 20, 300, "First") + markedText(3, 20, 280, "second") + markedText(0, 20, 350, "Heading") + "/X Do\n" + markedText(9, 20, 50, "UNCLAIMED")
	root := "<< /Type /StructTreeRoot /K 8 0 R /RoleMap << /Chapter /H2 >> /ParentTree 12 0 R >>"
	d := readRecovery(t, taggedTestPDF(content, root,
		"<< /Type /StructElem /S /Document /P 7 0 R /K [9 0 R 10 0 R 11 0 R] >>",
		"<< /Type /StructElem /S /Chapter /P 8 0 R /Pg 3 0 R /K 0 >>",
		"<< /Type /StructElem /S /P /P 8 0 R /Pg 3 0 R /K [1 3] >>",
		"<< /Type /StructElem /S /P /P 8 0 R /Pg 3 0 R /K << /Type /MCR /Stm 6 0 R /MCID 0 >> >>",
		"<< /Nums [0 [9 0 R 10 0 R null 10 0 R] 1 [11 0 R]] >>"))
	if len(d.Blocks) != 4 || d.Blocks[0].Kind != Heading || d.Blocks[0].Level != 2 || d.Blocks[0].Runs[0].Text != "Heading" || d.Blocks[1].Kind != Paragraph || d.Blocks[2].Runs[0].Text != "FORM" || d.Blocks[3].Runs[0].Text != "UNCLAIMED" {
		t.Fatalf("logical order %+v", d.Blocks)
	}
	if d.Text() != "Heading\nFirst second\nFORM\nUNCLAIMED" {
		t.Fatalf("text order %q", d.Text())
	}
	if d.Blocks[0].Source.Structure != "explicit" || d.Blocks[0].Source.Tag != "Chapter" || d.Blocks[0].Source.Role != "H2" || d.Blocks[1].Source.Structure != "explicit" || d.Blocks[1].Runs[0].Source.Stream != "page:1" || d.Blocks[1].Runs[0].Source.Structure != "explicit" || d.Blocks[1].Runs[1].Source.Structure != "inferred" || d.Blocks[3].Runs[0].Source.Structure != "unstructured" {
		t.Fatal("explicit/inferred/painted provenance collapsed")
	}
	if d.Blocks[2].Runs[0].Source.Stream == d.Blocks[0].Runs[0].Source.Stream {
		t.Fatal("page/form MCID zero conflated")
	}
	if !hasDiagnostic(d, "pdf_structure_unclaimed") || hasDiagnostic(d, "pdf_structure_pending") {
		t.Fatal("unclaimed diagnostic missing")
	}
	assertRecoverySF(t, d)
}
func TestPDFTaggedTablesMergedCellsAndClasses(t *testing.T) {
	content := markedText(0, 20, 300, "MERGED") + markedText(1, 150, 300, "TOP") + markedText(2, 150, 280, "BOTTOM")
	d := readRecovery(t, taggedTestPDF(content, "<< /Type /StructTreeRoot /K 8 0 R /ClassMap << /Merge << /O /Table /RowSpan 2 /ColSpan 1 >> >> >>",
		"<< /Type /StructElem /S /Table /P 7 0 R /Pg 3 0 R /K [9 0 R 10 0 R] >>",
		"<< /Type /StructElem /S /TR /P 8 0 R /K [11 0 R 12 0 R] >>",
		"<< /Type /StructElem /S /TR /P 8 0 R /K 13 0 R >>",
		"<< /Type /StructElem /S /TH /P 9 0 R /C /Merge /A << /O /Table /ColSpan 2 >> /K 0 >>",
		"<< /Type /StructElem /S /TD /P 9 0 R /K 1 >>",
		"<< /Type /StructElem /S /TD /P 10 0 R /K 2 >>"))
	if len(d.Blocks) != 1 || d.Blocks[0].Kind != Table {
		t.Fatalf("table %+v", d.Blocks)
	}
	b := d.Blocks[0]
	if len(b.Rows) != 2 || len(b.Rows[0].Cells) != 2 || len(b.Rows[1].Cells) != 1 || b.Rows[0].Cells[0].RowSpan != 2 || b.Rows[0].Cells[0].ColSpan != 2 || b.Rows[0].Cells[0].Source.Role != "TH" {
		t.Fatalf("merged cells %+v", b.Rows)
	}
	if d.Text() != "MERGED\tTOP\nBOTTOM" {
		t.Fatalf("table text %q", d.Text())
	}
	g, e := tableGrid(b.Rows)
	if e != nil || g.Columns != 3 {
		t.Fatalf("grid %+v %v", g, e)
	}
	assertRecoverySF(t, d)
}
func TestPDFTaggedListsLabelsAndNestedBodies(t *testing.T) {
	content := markedText(0, 20, 300, "3.") + markedText(1, 40, 300, "First") + markedText(2, 20, 280, "4)") + markedText(3, 40, 280, "Second")
	d := readRecovery(t, taggedTestPDF(content, "<< /Type /StructTreeRoot /K 8 0 R >>",
		"<< /Type /StructElem /S /L /P 7 0 R /Pg 3 0 R /A << /O /List /ListNumbering /Decimal >> /K [9 0 R 10 0 R] >>",
		"<< /Type /StructElem /S /LI /P 8 0 R /K [11 0 R 12 0 R] >>",
		"<< /Type /StructElem /S /LI /P 8 0 R /K [13 0 R 14 0 R] >>",
		"<< /Type /StructElem /S /Lbl /P 9 0 R /K 0 >>",
		"<< /Type /StructElem /S /LBody /P 9 0 R /K 1 >>",
		"<< /Type /StructElem /S /Lbl /P 10 0 R /K 2 >>",
		"<< /Type /StructElem /S /LBody /P 10 0 R /K 3 >>"))
	if len(d.Blocks) != 1 || d.Blocks[0].Kind != ListBlock {
		t.Fatalf("list %+v", d.Blocks)
	}
	b := d.Blocks[0]
	if b.List != "number" || b.Start == nil || *b.Start != 3 || len(b.Items) != 2 || b.Items[0].Blocks[0].Runs[0].Text != "First" || b.Items[1].Source.Label != "4)" || !hasDiagnostic(d, "pdf_list_label_normalized") {
		t.Fatalf("list semantics %+v", b)
	}
	if strings.Contains(d.Text(), "3.") || strings.Contains(d.Text(), "4)") {
		t.Fatal("label duplicated into body")
	}
	assertRecoverySF(t, d)
}
func TestPDFTaggedCrossPageParagraph(t *testing.T) {
	objects := []string{"<< /Type /Catalog /Pages 2 0 R /StructTreeRoot 7 0 R >>", "<< /Type /Pages /Kids [3 0 R 6 0 R] /Count 2 >>", "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 400] /Resources << /Font << /F 4 0 R >> >> /Contents 5 0 R >>", "<< /Type /Font /Subtype /Type1 /BaseFont /Courier >>", recoveryStream("", markedText(0, 20, 50, "Before")), "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 400] /Resources << /Font << /F 4 0 R >> >> /Contents 9 0 R >>", "<< /Type /StructTreeRoot /K 8 0 R >>", "<< /Type /StructElem /S /P /P 7 0 R /K [<< /Type /MCR /Pg 3 0 R /MCID 0 >> << /Type /MCR /Pg 6 0 R /MCID 0 >>] >>", recoveryStream("", markedText(0, 20, 350, "after"))}
	d := readRecovery(t, recoveryPDF(objects...))
	if len(d.Blocks) != 1 || d.Text() != "Before after" || len(d.Pages) != 2 || len(d.Blocks[0].Source.Regions) != 2 || d.Blocks[0].Source.Page != 0 || d.Blocks[0].Source.Quad != nil {
		t.Fatalf("cross-page paragraph %+v", d.Blocks)
	}
	if d.Blocks[0].Runs[0].Source.Page != 1 || d.Blocks[0].Runs[len(d.Blocks[0].Runs)-1].Source.Page != 2 {
		t.Fatal("page provenance lost")
	}
	assertRecoverySF(t, d)
	// List bodies imply a paragraph from the same inline content, but do not
	// claim the explicit P semantics of the first input.
	objects[7] = strings.Replace(objects[7], "/S /P", "/S /LBody", 1)
	d = readRecovery(t, recoveryPDF(objects...))
	first, last := d.Blocks[0].Runs[0].Source, d.Blocks[0].Runs[len(d.Blocks[0].Runs)-1].Source
	want := []SourceRegion{{Page: 1, Quad: *first.Quad}, {Page: 2, Quad: *last.Quad}}
	if len(d.Blocks) != 1 || d.Text() != "Before after" || d.Blocks[0].Source.Structure != "inferred" || !reflect.DeepEqual(d.Blocks[0].Source.Regions, want) {
		t.Fatalf("implied cross-page paragraph/bounds: %+v", d.Blocks)
	}
	assertRecoverySF(t, d)
}
func TestPDFTaggedMalformedGraphsReject(t *testing.T) {
	cases := []struct {
		name, root string
		elements   []string
	}{
		{"cycle", "<< /Type /StructTreeRoot /K 8 0 R >>", []string{"<< /Type /StructElem /S /P /P 7 0 R /Pg 3 0 R /K 8 0 R >>"}},
		{"duplicate owner", "<< /Type /StructTreeRoot /K [8 0 R 9 0 R] >>", []string{"<< /Type /StructElem /S /P /P 7 0 R /Pg 3 0 R /K 0 >>", "<< /Type /StructElem /S /P /P 7 0 R /Pg 3 0 R /K 0 >>"}},
		{"bad parent", "<< /Type /StructTreeRoot /K 8 0 R >>", []string{"<< /Type /StructElem /S /P /P 6 0 R /Pg 3 0 R /K 0 >>"}},
		{"bad page", "<< /Type /StructTreeRoot /K 8 0 R >>", []string{"<< /Type /StructElem /S /P /P 7 0 R /Pg 4 0 R /K 0 >>"}},
		{"role cycle", "<< /Type /StructTreeRoot /K 8 0 R /RoleMap << /A /B /B /A >> >>", []string{"<< /Type /StructElem /S /A /P 7 0 R /Pg 3 0 R /K 0 >>"}},
		{"bad attribute class", "<< /Type /StructTreeRoot /K 8 0 R >>", []string{"<< /Type /StructElem /S /P /P 7 0 R /Pg 3 0 R /C /Absent /K 0 >>"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, e := ReadPDF(context.Background(), bytes.NewReader(taggedTestPDF(markedText(0, 20, 300, "CONTENT"), tc.root, tc.elements...)), ReadOptions{}); e == nil {
				t.Fatal("malformed logical graph accepted")
			}
		})
	}
}

func TestPDFTaggedFigureReferencesAndOuterMCID(t *testing.T) {
	root := "<< /Type /StructTreeRoot /K 8 0 R >>"
	objects := taggedTestObjects("q 40 0 0 20 20 200 cm /Figure << /MCID 2 >> BDC /X Do EMC Q", root, "<< /Type /StructElem /S /Figure /P 7 0 R /Pg 3 0 R /Alt (Red pixel) /K 2 >>")
	objects[5] = recoveryStream("/Type /XObject /Subtype /Image /Width 1 /Height 1 /BitsPerComponent 8 /ColorSpace /DeviceRGB", string([]byte{255, 0, 0}))
	for _, objectRef := range []bool{false, true} {
		if objectRef {
			objects[7] = "<< /Type /StructElem /S /Figure /P 7 0 R /Pg 3 0 R /Alt (Red pixel) /K << /Type /OBJR /Obj 6 0 R >> >>"
		}
		d := readRecovery(t, recoveryPDF(objects...))
		if len(d.Blocks) != 1 || d.Blocks[0].Kind != Picture || d.Blocks[0].Image.Alt != "Red pixel" || d.Blocks[0].Source.Stream != "page:1" || d.Blocks[0].Source.Role != "Figure" || d.Blocks[0].Source.Element == "" || !strings.Contains(d.Blocks[0].Source.Object, "6 0 R") {
			t.Fatalf("figure binding %+v", d.Blocks)
		}
		assertRecoverySF(t, d)
	}
	// A page MCID wrapping a Form invocation belongs to the outer page stream,
	// even though the text is physically painted from the Form's stream.
	objects = taggedTestObjects("/P << /MCID 2 >> BDC /X Do EMC", root, "<< /Type /StructElem /S /P /P 7 0 R /Pg 3 0 R /K 2 >>")
	objects[5] = recoveryStream("/Type /XObject /Subtype /Form /BBox [0 0 100 100]", "BT /F 10 Tf 10 20 Td (OUTER) Tj ET")
	d := readRecovery(t, recoveryPDF(objects...))
	if d.Blocks[0].Runs[0].Text != "OUTER" || d.Blocks[0].Runs[0].Source.Stream != "page:1" {
		t.Fatal("outer page marked owner lost")
	}
}
func TestPDFTaggedActualTextAndEmptyParagraph(t *testing.T) {
	d := readRecovery(t, taggedTestPDF(markedText(0, 20, 300, "XY"), "<< /Type /StructTreeRoot /K [8 0 R 9 0 R] >>", "<< /Type /StructElem /S /P /P 7 0 R /Pg 3 0 R /ActualText <FEFF00660069> /K 0 >>", "<< /Type /StructElem /S /P /P 7 0 R >>"))
	if len(d.Blocks) != 2 || d.Blocks[0].Runs[0].Text != "fi" || len(d.Blocks[1].Runs) != 0 || len(d.Blocks[0].Source.Regions) != 1 || d.Blocks[0].Source.Structure != "explicit" {
		t.Fatalf("logical replacement/empty paragraph %+v", d.Blocks)
	}
	assertRecoverySF(t, d)
}
func TestPDFTaggedUnsupportedLabelPreservesLiteralText(t *testing.T) {
	d := readRecovery(t, taggedTestPDF(markedText(0, 20, 300, "iv.")+markedText(1, 40, 300, "Body"), "<< /Type /StructTreeRoot /K 8 0 R >>", "<< /Type /StructElem /S /L /P 7 0 R /Pg 3 0 R /A << /O /List /ListNumbering /LowerRoman >> /K 9 0 R >>", "<< /Type /StructElem /S /LI /P 8 0 R /K [10 0 R 11 0 R] >>", "<< /Type /StructElem /S /Lbl /P 9 0 R /K 0 >>", "<< /Type /StructElem /S /LBody /P 9 0 R /K 1 >>"))
	if !strings.Contains(d.Text(), "iv.") || !strings.Contains(d.Text(), "Body") || !hasDiagnostic(d, "pdf_structure_list_labels") {
		t.Fatal("unsupported label silently dropped")
	}
	assertRecoverySF(t, d)
}

func TestPDFTaggedURILinksAndGeometry(t *testing.T) {
	objects := taggedTestObjects(markedText(0, 20, 300, "click here"), "<< /Type /StructTreeRoot /K 8 0 R >>", "<< /Type /StructElem /S /P /P 7 0 R /Pg 3 0 R /K 9 0 R >>", "<< /Type /StructElem /S /Link /P 8 0 R /K [0 << /Type /OBJR /Obj 10 0 R >>] >>", "<< /Type /Annot /Subtype /Link /Rect [20 310 80 296] /A << /S /URI /URI (https://example.invalid/path) >> >>")
	objects[2] = strings.Replace(objects[2], "/Contents 5 0 R", "/Annots [10 0 R] /Contents 5 0 R", 1)
	d := readRecovery(t, recoveryPDF(objects...))
	if len(d.Blocks) != 1 || d.Blocks[0].Runs[0].Link != "https://example.invalid/path" || !strings.Contains(d.Blocks[0].Runs[0].Source.Evidence, "link_from_structure") || hasDiagnostic(d, "pdf_structure_missing_content") {
		t.Fatalf("logical URI binding %+v", d.Blocks)
	}
	assertRecoverySF(t, d)
	// Without a logical tree a whole-span geometric association remains available.
	objects[0] = "<< /Type /Catalog /Pages 2 0 R >>"
	d = readRecovery(t, recoveryPDF(objects...))
	if d.Blocks[0].Runs[0].Link != "https://example.invalid/path" || !strings.Contains(d.Blocks[0].Runs[0].Source.Evidence, "link_from_rectangle") {
		t.Fatal("unstructured URI lost")
	}
	// A smaller rectangle must not silently apply its target to the entire span.
	objects[9] = strings.Replace(objects[9], "80 296", "30 296", 1)
	d = readRecovery(t, recoveryPDF(objects...))
	if d.Blocks[0].Runs[0].Link != "" || !hasDiagnostic(d, "pdf_link_partial_span") {
		t.Fatal("partial link coverage overclaimed")
	}
}
func TestPDFSimpleFontPaddedCMap(t *testing.T) {
	objects := taggedTestObjects("BT /F 10 Tf 20 300 Td (ABCD) Tj ET", "<< /Type /StructTreeRoot >>")
	objects[0] = "<< /Type /Catalog /Pages 2 0 R >>"
	objects[3] = "<< /Type /Font /Subtype /Type1 /BaseFont /Courier /ToUnicode 6 0 R >>"
	objects[5] = recoveryStream("", "1 begincodespacerange <0000> <FFFF> endcodespacerange 2 beginbfchar <0041> <0041> <42> <0042> endbfchar 1 beginbfrange <0043> <0044> <0043> endbfrange")
	d := readRecovery(t, recoveryPDF(objects...))
	if d.Text() != "ABCD" || !hasDiagnostic(d, "pdf_cmap_codespace") {
		t.Fatalf("padded simple-font map %q", d.Text())
	}
	closePoint(t, d.Blocks[0].Runs[0].Source.Baseline[1], 44, 100)
	assertRecoverySF(t, d)
	objects[5] = recoveryStream("", "2 beginbfchar <0041> <0051> <41> <0041> endbfchar")
	if _, e := ReadPDF(context.Background(), bytes.NewReader(recoveryPDF(objects...)), ReadOptions{}); e == nil {
		t.Fatal("ambiguous padded mappings accepted")
	}
}

func TestPDFPublishedInDesignExport(t *testing.T) {
	data, e := os.ReadFile("testdata/real/copyright-office-circular-1-2021.pdf")
	if e != nil {
		t.Fatal(e)
	}
	manifest, e := os.ReadFile("testdata/real/manifest.json")
	if e != nil {
		t.Fatal(e)
	}
	var metadata struct {
		Files []struct {
			SHA256 string `json:"sha256"`
		}
	}
	if e = json.Unmarshal(manifest, &metadata); e != nil {
		t.Fatal(e)
	}
	if len(metadata.Files) != 1 || fmt.Sprintf("%x", sha256.Sum256(data)) != metadata.Files[0].SHA256 {
		t.Fatal("published fixture bytes changed")
	}
	d := readRecovery(t, data)
	if len(d.Pages) != 10 || d.Metadata.Title != "Circular 1 Copyright Basics" {
		t.Fatalf("published metadata %+v", d.Metadata)
	}
	lists, items, headings := 0, 0, 0
	links := map[string]bool{}
	var walk func([]Block)
	walk = func(bs []Block) {
		for _, b := range bs {
			if b.Kind == ListBlock {
				lists++
				items += len(b.Items)
				for _, item := range b.Items {
					// Each original LBody contains only direct content/Span
					// children. It supplies one implied paragraph; a nested L
					// on one LI is an independent sibling block.
					paragraphs := 0
					for _, child := range item.Blocks {
						if child.Kind == Paragraph {
							paragraphs++
							if child.Source == nil || child.Source.Structure != "inferred" || child.Source.Role != "LBody" {
								t.Fatal("published implied-paragraph provenance lost")
							}
						}
					}
					if paragraphs != 1 {
						t.Fatalf("published LBody fragmented into %d paragraphs", paragraphs)
					}
				}
			}
			if b.Kind == Heading {
				headings++
			}
			for _, r := range b.Runs {
				if r.Link != "" {
					links[r.Link] = true
				}
			}
			for _, item := range b.Items {
				walk(item.Blocks)
			}
			for _, row := range b.Rows {
				for _, cell := range row.Cells {
					walk(cell.Blocks)
				}
			}
		}
	}
	walk(d.Blocks)
	// These values come from independently reading the publication's original
	// /K tree, role map, annotations and labels, not from a generated SF oracle.
	if lists != 10 || items != 57 || headings != 0 || len(links) != 16 {
		t.Fatalf("real-export structure: lists=%d items=%d headings=%d URI targets=%d", lists, items, headings, len(links))
	}
	if !hasDiagnostic(d, "pdf_cmap_codespace") || !hasDiagnostic(d, "pdf_link_action") || !hasDiagnostic(d, "pdf_structure_unclaimed") {
		t.Fatal("known publication uncertainty disappeared")
	}
	if !strings.Contains(d.Text(), "Copyright Basics") || !strings.Contains(d.Text(), "Literary works") {
		t.Fatal("published content missing")
	}
	for _, word := range []string{"owner\u00adship", "panto\u00admime", "pan\u00adtomime", "depend\u00ading"} {
		if !strings.Contains(d.Text(), word) {
			t.Fatalf("published word fragmented: %q", word)
		}
	}
	assertRecoverySF(t, d)
}
