package manydoc

import (
	"strings"
	"testing"
)

func paragraphText(b Block) string {
	var out strings.Builder
	for _, r := range b.Runs {
		out.WriteString(r.Text)
	}
	return out.String()
}

func TestPDFTaggedImpliedParagraphAndBlockBarriers(t *testing.T) {
	content := markedText(0, 20, 350, "owner")
	content += markedText(1, 50, 350, "-")
	content += markedText(2, 20, 330, "ship") + markedText(3, 20, 300, "Explicit")
	content += markedText(4, 20, 280, "Wrapped") + markedText(5, 20, 260, "last")
	content += markedText(6, 20, 240, "Separate") + markedText(7, 20, 220, "tail")
	d := readRecovery(t, taggedTestPDF(content, "<< /Type /StructTreeRoot /K 8 0 R >>",
		"<< /Type /StructElem /S /LBody /P 7 0 R /Pg 3 0 R /K [0 9 0 R 2 10 0 R 11 0 R 5 13 0 R 7] >>",
		"<< /Type /StructElem /S /Span /P 8 0 R /ActualText <FEFF00AD> /K 1 >>",
		"<< /Type /StructElem /S /P /P 8 0 R /K 3 >>",
		"<< /Type /StructElem /S /Span /P 8 0 R /K 12 0 R >>",
		"<< /Type /StructElem /S /P /P 11 0 R /K 4 >>",
		"<< /Type /StructElem /S /Div /P 8 0 R /K 6 >>"))
	want := []string{"owner\u00adship", "Explicit", "Wrapped", "last", "Separate", "tail"}
	if len(d.Blocks) != len(want) {
		t.Fatalf("block boundaries: %+v", d.Blocks)
	}
	for i, b := range d.Blocks {
		if b.Kind != Paragraph || paragraphText(b) != want[i] {
			t.Fatalf("paragraph %d: %+v", i, b)
		}
		structure := "inferred"
		if i == 1 || i == 2 {
			structure = "explicit"
		}
		if b.Source == nil || b.Source.Structure != structure || b.Source.Quad == nil {
			t.Fatalf("paragraph %d provenance: %+v", i, b.Source)
		}
	}
	if len(d.Blocks[0].Runs) != 3 || d.Blocks[0].Runs[1].Source.Role != "Span" || d.Blocks[0].Runs[0].Source.MCID == nil || *d.Blocks[0].Runs[0].Source.MCID != 0 {
		t.Fatal("inline owners or original runs lost")
	}
	assertRecoverySF(t, d)
}

func TestPDFTaggedCellImpliedAndExplicitParagraphs(t *testing.T) {
	content := markedText(0, 20, 350, "First") + markedText(1, 20, 330, "line")
	content += markedText(2, 20, 310, "Second") + markedText(3, 20, 290, "Third")
	d := readRecovery(t, taggedTestPDF(content, "<< /Type /StructTreeRoot /K 8 0 R >>",
		"<< /Type /StructElem /S /Table /P 7 0 R /Pg 3 0 R /K 9 0 R >>",
		"<< /Type /StructElem /S /TR /P 8 0 R /K 10 0 R >>",
		"<< /Type /StructElem /S /TD /P 9 0 R /K [0 1 11 0 R 12 0 R] >>",
		"<< /Type /StructElem /S /P /P 10 0 R /K 2 >>",
		"<< /Type /StructElem /S /P /P 10 0 R /K 3 >>"))
	bs := d.Blocks[0].Rows[0].Cells[0].Blocks
	if len(bs) != 3 || paragraphText(bs[0]) != "First line" || paragraphText(bs[1]) != "Second" || paragraphText(bs[2]) != "Third" || bs[0].Source.Structure != "inferred" || bs[1].Source.Structure != "explicit" {
		t.Fatalf("cell paragraph boundaries: %+v", bs)
	}
	if bs[0].Runs[1].Text != " " || bs[0].Runs[1].Source.Precision != "synthetic_separator" {
		t.Fatal("inferred inter-line word boundary not disclosed")
	}
	assertRecoverySF(t, d)
}

func TestPDFCustomListLabelKeepsExplicitBodyParagraphs(t *testing.T) {
	content := markedText(0, 20, 350, "iv.") + markedText(1, 50, 350, "First") + markedText(2, 50, 330, "Second")
	d := readRecovery(t, taggedTestPDF(content, "<< /Type /StructTreeRoot /K 8 0 R >>",
		"<< /Type /StructElem /S /L /P 7 0 R /Pg 3 0 R /K 9 0 R >>",
		"<< /Type /StructElem /S /LI /P 8 0 R /K [10 0 R 11 0 R] >>",
		"<< /Type /StructElem /S /Lbl /P 9 0 R /K 0 >>",
		"<< /Type /StructElem /S /LBody /P 9 0 R /K [12 0 R 13 0 R] >>",
		"<< /Type /StructElem /S /P /P 11 0 R /K 1 >>",
		"<< /Type /StructElem /S /P /P 11 0 R /K 2 >>"))
	if len(d.Blocks) != 1 || d.Blocks[0].Kind != ListBlock || len(d.Blocks[0].Items) != 1 {
		t.Fatal("explicit custom list hierarchy lost")
	}
	item := d.Blocks[0].Items[0]
	if item.Marker == nil || paragraphText(item.Marker.Blocks[0]) != "iv." || len(item.Blocks) != 2 || paragraphText(item.Blocks[0]) != "First" || paragraphText(item.Blocks[1]) != "Second" || item.Blocks[0].Source.Structure != "explicit" || item.Blocks[1].Source.Structure != "explicit" || !hasDiagnostic(d, "pdf_list_literal") || !hasDiagnostic(d, "pdf_list_kind_unobserved") {
		t.Fatal("original marker/body/paragraph/source boundaries changed")
	}
	if d.Text() != "iv. First\nSecond" {
		t.Fatal("original literal text oracle changed", d.Text())
	}
	assertRecoverySF(t, d)
}

func TestPDFTaggedInlineReplacementFormattingAndLink(t *testing.T) {
	objects := taggedTestObjects(markedText(0, 20, 350, "XY")+markedText(1, 20, 330, "linked"), "<< /Type /StructTreeRoot /K 8 0 R >>",
		"<< /Type /StructElem /S /LBody /P 7 0 R /Pg 3 0 R /K [9 0 R 10 0 R] >>",
		"<< /Type /StructElem /S /Span /P 8 0 R /ActualText <FEFF00660069> /K 0 >>",
		"<< /Type /StructElem /S /Link /P 8 0 R /K [1 << /Type /OBJR /Obj 11 0 R >>] >>",
		"<< /Type /Annot /Subtype /Link /Rect [20 325 56 340] /A << /S /URI /URI (https://example.invalid/inline) >> >>")
	objects[2] = strings.Replace(objects[2], "/Contents 5 0 R", "/Annots [11 0 R] /Contents 5 0 R", 1)
	objects[3] = strings.Replace(objects[3], "/Courier", "/Courier-BoldOblique", 1)
	d := readRecovery(t, recoveryPDF(objects...))
	if len(d.Blocks) != 1 || paragraphText(d.Blocks[0]) != "fi linked" || d.Blocks[0].Source.Structure != "inferred" {
		t.Fatalf("inline paragraph: %+v", d.Blocks)
	}
	rs := d.Blocks[0].Runs
	if len(rs) != 3 || !rs[0].Bold || !rs[0].Italic || rs[0].Source.FontSize != 10 || rs[0].Source.Baseline == nil || !rs[2].Bold || !rs[2].Italic || rs[2].Link != "https://example.invalid/inline" || rs[2].Source.Role != "Link" {
		t.Fatalf("inline formatting/relationship/provenance: %+v", rs)
	}
	assertRecoverySF(t, d)
	objects[7] = strings.Replace(objects[7], "/K [9 0 R 10 0 R]", "/K 9 0 R", 1)
	objects[8] = strings.Replace(objects[8], "/K 0", "/K [0 1]", 1)
	objects[2] = strings.Replace(objects[2], "/F 4 0 R", "/F 4 0 R /N 12 0 R", 1)
	objects = append(objects, "<< /Type /Font /Subtype /Type1 /BaseFont /Courier >>")
	objects[4] = recoveryStream("", markedText(0, 20, 350, "X")+strings.Replace(markedText(1, 26, 350, "Y"), "/F 10", "/N 10", 1))
	d = readRecovery(t, recoveryPDF(objects...))
	if d.Text() != "fi" || len(d.Blocks) != 1 || len(d.Blocks[0].Runs) != 1 || d.Blocks[0].Runs[0].Bold || d.Blocks[0].Runs[0].Italic || d.Blocks[0].Runs[0].Source.Font != "" || !hasDiagnostic(d, "pdf_actualtext_style") {
		t.Fatalf("mixed replacement ambiguity: %+v", d.Blocks)
	}
	assertRecoverySF(t, d)
}
