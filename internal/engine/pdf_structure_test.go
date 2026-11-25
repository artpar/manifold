package engine

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestPDFPaginationArtifactPropertiesAndMembership(t *testing.T) {
	data := recoveryPage(`/Artifact << /Type /Pagination /Subtype /Header /BBox [10 350 220 390] /Attached [/Top /Left] >> BDC /Span << /MCID 0 >> BDC BT /F 10 Tf 20 370 Td (HEADER) Tj ET EMC EMC BT /F 10 Tf 20 300 Td (BODY) Tj ET`, "")
	d := readRecovery(t, data)
	if len(d.Pages[0].Artifacts) != 1 {
		t.Fatal("missing explicit artifact")
	}
	a := d.Pages[0].Artifacts[0]
	if a.Type != "Pagination" || a.Subtype != "Header" || a.Stream != "page:1" || !reflect.DeepEqual(a.Attached, []string{"Top", "Left"}) || a.BBox == nil || *a.BBox != ([4]float64{10, 350, 220, 390}) {
		t.Fatal("artifact properties lost", a)
	}
	closePoint(t, a.Bounds[0], 10, 50)
	closePoint(t, a.Bounds[2], 220, 10)
	header, body := false, false
	for _, b := range d.Blocks {
		for _, r := range b.Runs {
			if r.Text == "HEADER" {
				header = reflect.DeepEqual(r.Source.Artifacts, []string{a.ID}) && r.Source.Tag == "Span"
			}
			if r.Text == "BODY" {
				body = len(r.Source.Artifacts) == 0
			}
		}
	}
	if !header || !body {
		t.Fatal("nested marked content confused membership", d.Blocks)
	}
	if len(d.Headers) != 0 {
		t.Fatal("invented reusable authorial story from one physical page")
	}
	assertRecoverySF(t, d)
	for _, f := range []Format{SF, SFGzip, JSON} {
		var b bytes.Buffer
		if e := Write(context.Background(), &b, d, f, WriteOptions{}); e != nil {
			t.Fatal(e)
		}
		back, e := Read(context.Background(), &b, f, ReadOptions{})
		if e != nil {
			t.Fatal(e)
		}
		if !bytes.Equal(canonicalSF(t, d), canonicalSF(t, back)) {
			t.Fatal("physical story evidence changed", f)
		}
	}
	sf, e := ToStructured(d)
	if e != nil {
		t.Fatal(e)
	}
	*sf.Attrs.Pages[0].Artifacts[0].BBox = ([4]float64{1, 2, 3, 4})
	if *d.Pages[0].Artifacts[0].BBox != ([4]float64{10, 350, 220, 390}) {
		t.Fatal("bridge aliases caller artifact data")
	}
}
func TestPDFArtifactBBoxUsesDefaultSpace(t *testing.T) {
	// Artifact BBox is default user space (ISO 32000-1 Table 330), while actual
	// glyphs still obey cm. Crop/rotation/UserUnit then map both to display pt.
	d := readRecovery(t, recoveryPage(`q 2 0 0 2 5 7 cm /Artifact << /Type /Pagination /Subtype /Footer /BBox [20 40 120 60] >> BDC BT /F 10 Tf 30 20 Td (FOOTER) Tj ET EMC Q`, `/CropBox [20 20 220 320] /Rotate 90 /UserUnit 2`))
	a := d.Pages[0].Artifacts[0]
	if a.Matrix != d.Pages[0].Transform {
		t.Fatal("content CTM applied to default-space box")
	}
	closePoint(t, a.Bounds[0], 40, 0)
	closePoint(t, a.Bounds[2], 80, 200)
}
func TestPDFArtifactNestedAndActualTextMembership(t *testing.T) {
	for _, actual := range []string{"AB", "replacement"} {
		content := `/Span << /ActualText (` + actual + `) >> BDC /Artifact << /Type /Pagination /Subtype /Header >> BDC BT /F 10 Tf 20 300 Td (A) Tj ET EMC /Artifact << /Type /Layout >> BDC BT /F 10 Tf 26 300 Td (B) Tj ET EMC EMC`
		d := readRecovery(t, recoveryPage(content, ""))
		if len(d.Pages[0].Artifacts) != 2 {
			t.Fatal("lost artifact groups")
		}
		ids := []string{d.Pages[0].Artifacts[0].ID, d.Pages[0].Artifacts[1].ID}
		found := false
		for _, b := range d.Blocks {
			for _, r := range b.Runs {
				if r.Text == actual {
					found = reflect.DeepEqual(r.Source.Artifacts, ids)
				}
			}
		}
		if !found {
			t.Fatal("ActualText union lost members", actual, d.Blocks)
		}
		assertRecoverySF(t, d)
	}
	content := `/Artifact << /Type /Pagination /Subtype /Header >> BDC /Artifact << /Type /Layout >> BDC BT /F 10 Tf 20 300 Td (A) Tj ET EMC BT /F 10 Tf 35 300 Td (B) Tj ET EMC BT /F 10 Tf 60 300 Td (C) Tj ET`
	d := readRecovery(t, recoveryPage(content, ""))
	outer := d.Pages[0].Artifacts[0].ID
	for _, r := range d.Blocks[0].Runs {
		if r.Source.Precision == "synthetic_separator" && len(r.Source.Artifacts) > 0 && !reflect.DeepEqual(r.Source.Artifacts, []string{outer}) {
			t.Fatal("gap claims unrelated artifact ownership")
		}
	}
}
func TestPDFArtifactMalformedAndGraph(t *testing.T) {
	for _, props := range []string{`/BBox [1 2 3]`, `/BBox [(x) 2 3 4]`, `/Type (Pagination)`, `/Attached [/Sideways]`, `/Attached [/Top /Top]`, `/Attached [1]`} {
		d, e := ReadPDF(context.Background(), bytes.NewReader(recoveryPage(`/Artifact << `+props+` >> BDC BT /F 10 Tf 20 300 Td (A) Tj ET EMC`, "")), ReadOptions{})
		if e == nil || d != nil {
			t.Fatal("invalid artifact accepted", props, e)
		}
	}
	d := readRecovery(t, recoveryPage(`/Artifact BMC BT /F 10 Tf 20 300 Td (A) Tj ET EMC`, ""))
	if len(d.Pages[0].Artifacts) != 1 || d.Pages[0].Artifacts[0].Type != "" {
		t.Fatal("generic BMC artifact invented properties")
	}
	for _, mutate := range []func(*StructuredDocument){
		func(s *StructuredDocument) {
			s.Attrs.Pages[0].Artifacts = append(s.Attrs.Pages[0].Artifacts, s.Attrs.Pages[0].Artifacts[0])
		},
		func(s *StructuredDocument) { s.Attrs.Pages[0].Artifacts[0].ID = "missing" },
	} {
		s, e := ToStructured(d)
		if e != nil {
			t.Fatal(e)
		}
		mutate(s)
		if _, e = FromStructured(s); e == nil {
			t.Fatal("invalid artifact reference graph accepted")
		}
	}
	if !strings.Contains(d.Text(), "A") {
		t.Fatal("artifact visible text dropped")
	}
}

func TestPDFArtifactFormAndResourceProperties(t *testing.T) {
	objects := []string{
		`<< /Type /Catalog /Pages 2 0 R >>`,
		`<< /Type /Pages /Kids [3 0 R] /Count 1 >>`,
		`<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 400] /Resources << /Font << /F 4 0 R >> /XObject << /X 6 0 R >> /Properties << /Header << /Type /Pagination /Subtype /Header /BBox [10 350 220 390] /Attached [/Top] >> >> >> /Contents 5 0 R >>`,
		`<< /Type /Font /Subtype /Type1 /BaseFont /Courier >>`,
		recoveryStream("", `/Artifact /Header BDC /X Do EMC /X Do`),
		recoveryStream(`/Type /XObject /Subtype /Form /BBox [0 0 100 100] /Matrix [1 0 0 1 10 10] /Resources << /Font << /F 4 0 R >> >>`, `BT /F 10 Tf 10 20 Td (FORM) Tj ET`),
	}
	d := readRecovery(t, recoveryPDF(objects...))
	if len(d.Pages[0].Artifacts) != 1 {
		t.Fatal("property resource not recovered")
	}
	var memberships []int
	for _, b := range d.Blocks {
		for _, r := range b.Runs {
			if r.Text == "FORM" {
				memberships = append(memberships, len(r.Source.Artifacts))
			}
		}
	}
	if !reflect.DeepEqual(memberships, []int{1, 0}) {
		t.Fatal("artifact context leaked across form calls", memberships)
	}
	assertRecoverySF(t, d)
}
func TestPDFAlignedActualTextUnionsArtifacts(t *testing.T) {
	content := `/Span << /ActualText (ABC) >> BDC /Artifact << /Type /Pagination /Subtype /Header >> BDC BT /F 10 Tf 20 300 Td (A) Tj ET EMC /Artifact << /Type /Layout >> BDC BT /F 10 Tf 26 300 Td (B) Tj ET EMC 1 0 0 rg BT /F 10 Tf 32 300 Td (C) Tj ET EMC`
	d := readRecovery(t, recoveryPage(content, ""))
	found := false
	for _, b := range d.Blocks {
		for _, r := range b.Runs {
			if r.Text == "AB" {
				found = len(r.Source.Artifacts) == 2 && r.Source.Evidence == "pdf_actualtext_aligned"
			}
		}
	}
	if !found {
		t.Fatal("aligned replacement lost its contributing artifact identities")
	}
}
func parentTreeFixture(tree string, extra ...string) []byte {
	objects := taggedTestObjects(markedText(0, 20, 300, "FIRST")+markedText(1, 20, 280, "SECOND"),
		"<< /Type /StructTreeRoot /K [8 0 R 9 0 R] /ParentTree 10 0 R >>",
		"<< /Type /StructElem /S /P /P 7 0 R /Pg 3 0 R /K 0 >>",
		"<< /Type /StructElem /S /P /P 7 0 R /Pg 3 0 R /K 1 >>", tree)
	return recoveryPDF(append(objects, extra...)...)
}

func TestPDFParentTreeReciprocalOwnership(t *testing.T) {
	for _, tc := range []struct {
		name, tree string
		extra      []string
		diagnostic string
	}{
		{name: "flat", tree: "<< /Nums [0 [8 0 R 9 0 R]] >>"},
		{name: "direct-child", tree: "<< /Kids [<< /Limits [0 0] /Nums [0 [8 0 R 9 0 R]] >>] >>", diagnostic: "pdf_parent_tree_direct_child"},
		{name: "branches", tree: "<< /Kids [11 0 R 12 0 R] >>", extra: []string{"<< /Limits [0 0] /Nums [0 [8 0 R 9 0 R]] >>", "<< /Limits [3 3] /Nums [3 [null]] >>"}},
		{name: "missing-key", tree: "<< /Nums [] >>", diagnostic: "pdf_parent_tree_incomplete"},
		{name: "null-gap", tree: "<< /Nums [0 [8 0 R null]] >>", diagnostic: "pdf_parent_tree_incomplete"},
		{name: "extra-known-owner", tree: "<< /Nums [0 [8 0 R 9 0 R 8 0 R]] >>", diagnostic: "pdf_parent_tree_unclaimed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := readRecovery(t, parentTreeFixture(tc.tree, tc.extra...))
			if len(d.Blocks) != 2 || d.Text() != "FIRST\nSECOND" || d.Blocks[0].Source.Element != "(8 0 R)" || d.Blocks[1].Source.Element != "(9 0 R)" {
				t.Fatalf("forward content/order/ownership changed: %+v", d.Blocks)
			}
			if tc.diagnostic != "" && !hasDiagnostic(d, tc.diagnostic) {
				t.Fatal("missing reciprocal uncertainty diagnostic")
			}
			if tc.diagnostic == "" && (hasDiagnostic(d, "pdf_parent_tree_incomplete") || hasDiagnostic(d, "pdf_parent_tree_unclaimed")) {
				t.Fatal("valid reciprocal map reported uncertain")
			}
			assertRecoverySF(t, d)
		})
	}
}

func TestPDFParentTreeMalformedOwnership(t *testing.T) {
	for _, tc := range []struct {
		name, tree string
		extra      []string
	}{
		{name: "conflicting-owner", tree: "<< /Nums [0 [9 0 R 8 0 R]] >>"},
		{name: "unknown-owner", tree: "<< /Nums [0 [7 0 R 9 0 R]] >>"},
		{name: "direct-owner", tree: "<< /Nums [0 [<< /S /P >> 9 0 R]] >>"},
		{name: "wrong-entry-shape", tree: "<< /Nums [0 8 0 R] >>"},
		{name: "duplicate-key", tree: "<< /Nums [0 [8 0 R] 0 [9 0 R]] >>"},
		{name: "descending-keys", tree: "<< /Nums [3 [] 0 [8 0 R 9 0 R]] >>"},
		{name: "negative-key", tree: "<< /Nums [-1 []] >>"},
		{name: "direct-child-wrong-owner", tree: "<< /Kids [<< /Nums [0 [9 0 R 8 0 R]] >>] >>"},
		{name: "direct-child-bad-type", tree: "<< /Kids [17] >>"},
		{name: "noninteger-key", tree: "<< /Nums [(0) []] >>"},
		{name: "odd-pairs", tree: "<< /Nums [0 [8 0 R] 2] >>"},
		{name: "inconsistent-limits", tree: "<< /Limits [0 2] /Nums [0 [8 0 R 9 0 R]] >>"},
		{name: "both-node-kinds", tree: "<< /Kids [11 0 R] /Nums [0 []] >>", extra: []string{"<< /Nums [0 []] >>"}},
		{name: "self-cycle", tree: "<< /Kids [10 0 R] >>"},
		{name: "repeated-child", tree: "<< /Kids [11 0 R 11 0 R] >>", extra: []string{"<< /Nums [0 [8 0 R 9 0 R]] >>"}},
		{name: "overlapping-branches", tree: "<< /Kids [11 0 R 12 0 R] >>", extra: []string{"<< /Nums [0 [8 0 R 9 0 R]] >>", "<< /Nums [0 [8 0 R 9 0 R]] >>"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d, err := ReadPDF(context.Background(), bytes.NewReader(parentTreeFixture(tc.tree, tc.extra...)), ReadOptions{})
			if err == nil || d != nil {
				t.Fatalf("contradictory/malformed inverse ownership accepted: %v", err)
			}
		})
	}
}

func TestPDFParentTreeAnnotationOwnership(t *testing.T) {
	objects := taggedTestObjects(markedText(0, 20, 300, "TARGET"),
		"<< /Type /StructTreeRoot /K 8 0 R /ParentTree 9 0 R >>",
		"<< /Type /StructElem /S /Link /P 7 0 R /Pg 3 0 R /K [0 << /Type /OBJR /Obj 10 0 R >>] >>",
		"<< /Nums [0 [8 0 R] 1 8 0 R] >>",
		"<< /Type /Annot /Subtype /Link /StructParent 1 /Rect [20 310 80 296] /A << /S /URI /URI (https://example.org/target) >> >>")
	objects[2] = strings.Replace(objects[2], "/Contents 5 0 R", "/Annots [10 0 R] /Contents 5 0 R", 1)
	d := readRecovery(t, recoveryPDF(objects...))
	if d.Text() != "TARGET" || d.Blocks[0].Runs[0].Link != "https://example.org/target" || hasDiagnostic(d, "pdf_parent_tree_incomplete") || hasDiagnostic(d, "pdf_parent_tree_unclaimed") {
		t.Fatalf("annotation reciprocal edge/URI lost: %+v", d)
	}
	assertRecoverySF(t, d)
	objects[9] = strings.Replace(objects[9], "/StructParent 1", "/StructParent 1 /StructParents 2", 1)
	if _, err := ReadPDF(context.Background(), bytes.NewReader(recoveryPDF(objects...)), ReadOptions{}); err == nil {
		t.Fatal("object/content-container conflict accepted")
	}
}

func TestPDFParentTreeAbsentRemainsExplicitForward(t *testing.T) {
	d := readRecovery(t, taggedTestPDF(markedText(0, 20, 300, "VISIBLE"), "<< /Type /StructTreeRoot /K 8 0 R >>", "<< /Type /StructElem /S /P /P 7 0 R /Pg 3 0 R /K 0 >>"))
	if d.Text() != "VISIBLE" || d.Blocks[0].Source.Structure != "explicit" || !hasDiagnostic(d, "pdf_parent_tree_missing") {
		t.Fatal("missing index silently certified or content lost")
	}
	assertRecoverySF(t, d)
}

func TestPDFParentTreeFormScopeAndSharedKeys(t *testing.T) {
	objects := taggedTestObjects(markedText(0, 20, 300, "PAGE")+"/X Do\n",
		"<< /Type /StructTreeRoot /K [8 0 R 9 0 R] /ParentTree 10 0 R >>",
		"<< /Type /StructElem /S /P /P 7 0 R /Pg 3 0 R /K 0 >>",
		"<< /Type /StructElem /S /P /P 7 0 R /Pg 3 0 R /K << /Type /MCR /Stm 6 0 R /MCID 0 >> >>",
		"<< /Nums [0 [8 0 R] 1 [9 0 R]] >>")
	d := readRecovery(t, recoveryPDF(objects...))
	if d.Text() != "PAGE\nFORM" || d.Blocks[0].Runs[0].Source.Stream == d.Blocks[1].Runs[0].Source.Stream || hasDiagnostic(d, "pdf_parent_tree_incomplete") {
		t.Fatal("page/Form MCID scopes conflated")
	}
	objects[5] = strings.Replace(objects[5], "/StructParents 1", "/StructParents 0", 1)
	if _, err := ReadPDF(context.Background(), bytes.NewReader(recoveryPDF(objects...)), ReadOptions{}); err == nil {
		t.Fatal("distinct page/Form share reciprocal key")
	}
}

func TestPDFParentTreeResourceBounds(t *testing.T) {
	t.Run("depth", func(t *testing.T) {
		extra := make([]string, 65)
		for i := range extra {
			extra[i] = fmt.Sprintf("<< /Kids [%d 0 R] >>", 12+i)
		}
		extra = append(extra, "<< /Nums [0 [8 0 R 9 0 R]] >>")
		if _, err := ReadPDF(context.Background(), bytes.NewReader(parentTreeFixture("<< /Kids [11 0 R] >>", extra...)), ReadOptions{}); err == nil || !strings.Contains(err.Error(), "ParentTree exceeds") {
			t.Fatalf("deep tree not bounded: %v", err)
		}
	})
	t.Run("owner-slots", func(t *testing.T) {
		data := parentTreeFixture("<< /Nums [0 [" + strings.Repeat("null ", 400001) + "]] >>")
		if _, err := ReadPDF(context.Background(), bytes.NewReader(data), ReadOptions{}); err == nil || !strings.Contains(err.Error(), "ParentTree exceeds owner slot limit") {
			t.Fatalf("sparse array not bounded: %v", err)
		}
	})
}

func FuzzPDFParentTree(f *testing.F) {
	for _, tree := range []string{"<< /Kids [<< /Nums [0 [8 0 R 9 0 R]] >>] >>", "<< /Nums [0 [8 0 R 9 0 R]] >>", "<< /Nums [0 [9 0 R 8 0 R]] >>", "<< /Kids [10 0 R] >>", "<< /Nums [0 [8 0 R null]] >>"} {
		f.Add(tree)
	}
	f.Fuzz(func(t *testing.T, tree string) {
		if len(tree) > 8192 {
			t.Skip()
		}
		// Keep this target confined to the index object: object/xref injection
		// could legitimately change the forward fixture rather than its index.

		// Objects 1-7 are stable; the tests own every structural expectation rather
		// than using manifold's output as an oracle for its input.

		// List bodies imply a paragraph from the same inline content, but do not
		// claim the explicit P semantics of the first input.

		for _, keyword := range []string{"obj", "stream", "xref", "trailer"} {
			if strings.Contains(tree, keyword) {
				t.Skip()
			}
		}
		d, err := ReadPDF(context.Background(), bytes.NewReader(parentTreeFixture(tree)), ReadOptions{})
		if err == nil {
			if d.Text() != "FIRST\nSECOND" || len(d.Blocks) != 2 {
				t.Fatal("inverse data changed forward content/order")
			}
			if e := d.Validate(); e != nil {
				t.Fatal("successful import violates model", e)
			}
		}
	})
}
func TestPDFActualWhitespaceAlignsOnlyMissingWhitespace(t *testing.T) {
	n := &pdfStructureNode{role: "Span"}
	bs := []Block{{Kind: Paragraph, Runs: []Run{{Text: "AB", Bold: true, Link: "https://example.org"}, {Text: "CD", Italic: true}}}}
	got, ok := alignPDFActualWhitespace(bs, "A B C D ", n)
	if !ok || paragraphText(got[0]) != "A B C D " {
		t.Fatal("missing whitespace not restored")
	}
	for _, r := range got[0].Runs {
		if r.Text == "A" || r.Text == "B" {
			if !r.Bold || r.Link != "https://example.org" {
				t.Fatal("original style/link lost")
			}
		}
		if r.Text == "C" || r.Text == "D" {
			if !r.Italic {
				t.Fatal("original style lost")
			}
		}
	}
	for _, text := range []string{"ABXCD", "ACD", "ABDC"} {
		if _, ok := alignPDFActualWhitespace(bs, text, n); ok {
			t.Fatal("non-whitespace mismatch accepted", text)
		}
	}
	painted := []Block{{Kind: Paragraph, Runs: []Run{{Text: "A B"}}}}
	if _, ok := alignPDFActualWhitespace(painted, "AB", n); ok {
		t.Fatal("painted whitespace silently discarded")
	}
}

func TestPDFWrappedReplacementRetainsMixedStylesAndWordBoundaries(t *testing.T) {
	d := &Document{Layout: Layout{Width: 130, Height: 180, Margin: 24}, Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "SUPERCALIFRAGILISTIC ", Bold: true, Link: "https://example.org/word"}, {Text: "hello wrapped world", Italic: true}}}}}
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, PDF, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	got, err := ReadPDF(context.Background(), &out, ReadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Blocks) != 1 || paragraphText(got.Blocks[0]) != "SUPERCALIFRAGILISTIC hello wrapped world" {
		t.Fatal("word boundaries changed", got.Text())
	}
	bold, italic := false, false
	for _, r := range got.Blocks[0].Runs {
		if r.Bold && r.Link == "https://example.org/word" {
			bold = true
		}
		if r.Italic {
			italic = true
		}
	}
	if !bold || !italic {
		t.Fatal("mixed styles/link lost during whitespace alignment")
	}
}

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
func TestPDFTaggedRomanLabelPreservesEditableNumbering(t *testing.T) {
	d := readRecovery(t, taggedTestPDF(markedText(0, 20, 300, "iv.")+markedText(1, 40, 300, "Body"), "<< /Type /StructTreeRoot /K 8 0 R >>", "<< /Type /StructElem /S /L /P 7 0 R /Pg 3 0 R /A << /O /List /ListNumbering /LowerRoman >> /K 9 0 R >>", "<< /Type /StructElem /S /LI /P 8 0 R /K [10 0 R 11 0 R] >>", "<< /Type /StructElem /S /Lbl /P 9 0 R /K 0 >>", "<< /Type /StructElem /S /LBody /P 9 0 R /K 1 >>"))
	assertPDFNumberedLabels(t, d, 4, []string{"iv."})
	if d.Blocks[0].Items[0].Source.Label != "iv." || !strings.Contains(d.Text(), "Body") || !hasDiagnostic(d, "pdf_list_numbering_inferred") || hasDiagnostic(d, "pdf_structure_list_labels") {
		t.Fatal("Roman content/explicit source/inference disclosure lost")
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
			Path   string `json:"path"`
			SHA256 string `json:"sha256"`
		}
	}
	if e = json.Unmarshal(manifest, &metadata); e != nil {
		t.Fatal(e)
	}
	matches := 0
	for _, file := range metadata.Files {
		if file.Path == "copyright-office-circular-1-2021.pdf" && fmt.Sprintf("%x", sha256.Sum256(data)) == file.SHA256 {
			matches++
		}
	}
	if matches != 1 {
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
				if r.Link != "" && r.Destination == "" {
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
	if !hasDiagnostic(d, "pdf_cmap_codespace") || !hasDiagnostic(d, "pdf_destination_flow_inferred") || !hasDiagnostic(d, "pdf_structure_unclaimed") {
		t.Fatal("known publication uncertainty disappeared")
	}
	// Independently checked against the original /Names tree and its one GoTo
	// annotation: page 2 links to page 6 at XYZ(95,699,null).
	if d.PDFNavigation == nil || len(d.PDFNavigation.Destinations) != 1 || len(d.PDFNavigation.Links) != 19 {
		t.Fatal("published navigation evidence omitted")
	}
	view := d.PDFNavigation.Destinations[0]
	if view.ID != "Circular 1 202109.indd:Copyright Notice:15" || view.Page != 6 || view.View != "XYZ" || *view.Parameters[0] != 95 || *view.Parameters[1] != 699 || view.Parameters[2] != nil {
		t.Fatal("published explicit destination changed", view)
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
func taggedSpanPDF() []byte {
	path := "20 220 240 120 re S 20 300 m 260 300 l S 100 260 m 260 260 l S 100 220 m 100 300 l S 180 220 m 180 340 l S\n"
	return recoveryPDF(taggedSpanObjects(path)...)
}
func taggedSpanObjects(path string) []string {
	content := path + markedText(0, 30, 320, "TOP") + markedText(1, 190, 320, "RIGHT") + markedText(2, 30, 280, "VERT") + markedText(3, 110, 280, "MID") + markedText(4, 190, 280, "R1") + markedText(5, 110, 240, "LOW") + markedText(6, 190, 240, "R2")
	return taggedTestObjects(content, "<< /Type /StructTreeRoot /K 8 0 R >>",
		"<< /Type /StructElem /S /Table /P 7 0 R /Pg 3 0 R /K [9 0 R 10 0 R 11 0 R] >>",
		"<< /Type /StructElem /S /TR /P 8 0 R /K [12 0 R 13 0 R] >>",
		"<< /Type /StructElem /S /TR /P 8 0 R /K [14 0 R 15 0 R 16 0 R] >>",
		"<< /Type /StructElem /S /TR /P 8 0 R /K [17 0 R 18 0 R 19 0 R] >>",
		"<< /Type /StructElem /S /TH /P 9 0 R /K 0 >>",
		"<< /Type /StructElem /S /TH /P 9 0 R /K 1 >>",
		"<< /Type /StructElem /S /TD /P 10 0 R /K 2 >>",
		"<< /Type /StructElem /S /TD /P 10 0 R /K 3 >>",
		"<< /Type /StructElem /S /TD /P 10 0 R /K 4 >>",
		"<< /Type /StructElem /S /TD /P 11 0 R /K [] >>",
		"<< /Type /StructElem /S /TD /P 11 0 R /K 5 >>",
		"<< /Type /StructElem /S /TD /P 11 0 R /K 6 >>")
}

const taggedShortPatchPath = "0.25 w 20 220 240 120 re S 20 300 m 260 300 l S 100 260 m 180 260 l S 181 260 m 260 260 l S 100 220 m 100 300 l S 180 260 m 180 340 l S 181 220 m 181 260 l S\n"

func taggedShortPatchObjects(patch bool) []string {
	path := taggedShortPatchPath
	if patch {
		path += "180 259.875 1 .25 re f\n"
	}
	return taggedSpanObjects(path)
}

func TestPDFRulingPatchesRespectClipAndVisibility(t *testing.T) {
	for _, mode := range []string{"clipped", "transparent", "blend"} {
		t.Run(mode, func(t *testing.T) {
			paint := "q /G gs 180 259.875 1 .25 re f Q\n"
			if mode == "clipped" {
				paint = "q 0 0 1 1 re W n 180 259.875 1 .25 re f Q\n"
			}
			objects := taggedSpanObjects(taggedShortPatchPath + paint)
			if mode != "clipped" {
				state := "/ca 0"
				if mode == "blend" {
					state = "/BM /Multiply"
				}
				objects[2] = strings.Replace(objects[2], "/Resources <<", "/Resources << /ExtGState << /G << "+state+" >> >>", 1)
			}
			d := readRecovery(t, recoveryPDF(objects...))
			if !hasDiagnostic(d, "pdf_structure_table_grid_uncertain") || hasDiagnostic(d, "pdf_structure_spans_inferred") {
				t.Fatal("unpainted or uncertain patch used as evidence", d.Diagnostics)
			}
			assertRecoverySF(t, d)
		})
	}
}

func TestPDFSmallFilledPatchesDoNotCreateAxes(t *testing.T) {
	var content strings.Builder
	for _, x := range []int{20, 100, 180, 260} {
		for _, y := range []int{220, 260, 300, 340} {
			fmt.Fprintf(&content, "%d %d 1 1 re f\n", x, y)
		}
	}
	content.WriteString(markedText(0, 30, 320, "KEPT"))
	d, err := ReadPDF(context.Background(), bytes.NewReader(recoveryPDF(inferenceObjects(content.String())...)), ReadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range d.Blocks {
		if b.Kind == Table {
			t.Fatal("isolated patches invented a grid")
		}
	}
	if strings.Count(d.Text(), "KEPT") != 1 {
		t.Fatal("text lost")
	}
}

func TestPDFTaggedShortPatchSupportsShiftedAxes(t *testing.T) {
	d := readRecovery(t, recoveryPDF(taggedShortPatchObjects(true)...))
	if len(d.Blocks) != 1 || d.Blocks[0].Kind != Table {
		t.Fatal("table lost")
	}
	b := d.Blocks[0]
	g, err := tableGrid(b.Rows)
	if err != nil || g.Columns != 4 || len(g.Origins) != 7 {
		t.Fatal("distinct shifted axes lost", g, err)
	}
	if b.Rows[0].Cells[0].ColSpan != 2 || b.Rows[0].Cells[1].ColSpan != 2 || b.Rows[1].Cells[0].RowSpan != 2 || b.Rows[1].Cells[2].ColSpan != 2 || b.Rows[2].Cells[0].ColSpan != 2 || b.Rows[2].Cells[1].ColSpan != 1 {
		t.Fatal("shifted spans incorrect", b.Rows)
	}
	refs := b.Rows[1].Cells[0].Source.TableSpans.Continuations
	if len(refs) != 1 || refs[0].Element != "(17 0 R)" || refs[0].Row != 2 {
		t.Fatal("continuation identity lost", refs)
	}
	if hasDiagnostic(d, "pdf_structure_table_grid_uncertain") || !hasDiagnostic(d, "pdf_structure_spans_inferred") {
		t.Fatal("unexpected inference result", d.Diagnostics)
	}
	for _, row := range b.Rows {
		for _, c := range row.Cells {
			if c.Source.TableSpans.Evidence != "painted_grid_adjacency" {
				t.Fatal("paint evidence lost")
			}
		}
	}
	for _, word := range []string{"TOP", "RIGHT", "VERT", "MID", "R1", "LOW", "R2"} {
		if strings.Count(d.Text(), word) != 1 {
			t.Fatal("content changed", word)
		}
	}
	assertRecoverySF(t, d)
}

func TestPDFTaggedMissingPatchRetainsUncertainCells(t *testing.T) {
	d := readRecovery(t, recoveryPDF(taggedShortPatchObjects(false)...))
	if len(d.Blocks) != 1 || d.Blocks[0].Kind != Table || len(d.Blocks[0].Rows[2].Cells) != 3 || !hasDiagnostic(d, "pdf_structure_table_grid_uncertain") || hasDiagnostic(d, "pdf_structure_spans_inferred") {
		t.Fatal("unpainted border silently completed", d.Diagnostics)
	}
	assertRecoverySF(t, d)
}

func TestPDFUntaggedShortPatchSupportsShiftedAxes(t *testing.T) {
	objects := taggedShortPatchObjects(true)
	objects[0] = strings.Replace(objects[0], "/StructTreeRoot 7 0 R", "", 1)
	d, err := ReadPDF(context.Background(), bytes.NewReader(recoveryPDF(objects...)), ReadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Blocks) != 1 || d.Blocks[0].Kind != Table {
		t.Fatal("painted grid not recovered", d.Text())
	}
	b := d.Blocks[0]
	g, err := tableGrid(b.Rows)
	if err != nil || g.Columns != 4 || len(g.Origins) != 7 || b.Rows[1].Cells[0].RowSpan != 2 {
		t.Fatal("shifted physical grid lost", g, err)
	}
	if b.Source.Structure != "inferred" || b.Source.Evidence != "painted_grid_adjacency" {
		t.Fatal("untagged inference provenance incorrect", b.Source)
	}
	for _, word := range []string{"TOP", "RIGHT", "VERT", "MID", "R1", "LOW", "R2"} {
		if strings.Count(d.Text(), word) != 1 {
			t.Fatal("content changed", word)
		}
	}
	assertRecoverySF(t, d)
}

func TestRulingJoinIncludesPaintedCrossingStroke(t *testing.T) {
	lines := []pdfRuling{
		{horizontal: true, pos: 10, start: .258, end: 99.742, width: .48},
		{pos: 0, start: 0, end: 20, width: .504},
		{pos: 100, start: 0, end: 20, width: .504},
	}
	if got := rulingCoverage(lines, true, 10, 0, 100); got != 1 {
		t.Fatal("painted crossing plus coordinate tolerance did not join", got)
	}
	lines[0].start = .503
	if got := rulingCoverage(lines, true, 10, 0, 100); got != -1 {
		t.Fatal("gap beyond stroke and tolerance was bridged", got)
	}
}

func TestPDFTaggedSpanSourceValidation(t *testing.T) {
	for name, mutate := range map[string]func(*SourceInfo){
		"inferred membership": func(s *SourceInfo) { s.Structure = "inferred" },
		"unknown span":        func(s *SourceInfo) { s.TableSpans.ColSpan = "unknown" },
		"nonfinite":           func(s *SourceInfo) { s.TableSpans.Bounds[0].X = math.NaN() },
		"empty bounds":        func(s *SourceInfo) { s.TableSpans.Bounds = Quad{} },
		"confidence":          func(s *SourceInfo) { s.TableSpans.Confidence = 1.1 },
		"reference row":       func(s *SourceInfo) { s.TableSpans.Continuations[0].Row = -1 },
		"duplicate origin":    func(s *SourceInfo) { s.TableSpans.Continuations[0].Element = s.Element },
	} {
		t.Run(name, func(t *testing.T) {
			d := readRecovery(t, taggedSpanPDF())
			mutate(d.Blocks[0].Rows[1].Cells[0].Source)
			if err := d.Validate(); err == nil {
				t.Fatal("invalid provenance accepted")
			}
		})
	}
}

func TestPDFTaggedSpanRequiresEvidenceAndHonorsExplicitAttributes(t *testing.T) {
	path := "20 220 240 120 re S 20 300 m 260 300 l S 100 260 m 260 260 l S 100 220 m 100 300 l S 180 220 m 180 340 l S\n"
	for _, name := range []string{"no borders", "explicit unit colspan", "visible continuation", "continuation last"} {
		t.Run(name, func(t *testing.T) {
			objects := taggedSpanObjects(path)
			switch name {
			case "no borders":
				objects = taggedSpanObjects("")
			case "explicit unit colspan":
				objects[11] = strings.Replace(objects[11], "/K 0", "/K 0 /A << /O /Table /ColSpan 1 >>", 1)
			case "visible continuation":
				objects[4] = recoveryStream("", path+markedText(0, 30, 320, "TOP")+markedText(1, 190, 320, "RIGHT")+markedText(2, 30, 280, "VERT")+markedText(3, 110, 280, "MID")+markedText(4, 190, 280, "R1")+markedText(5, 110, 240, "LOW")+markedText(6, 190, 240, "R2")+markedText(7, 30, 240, "VISIBLE"))
				objects[16] = strings.Replace(objects[16], "/K []", "/K 7", 1)
			case "continuation last":
				objects[10] = strings.Replace(objects[10], "17 0 R 18 0 R 19 0 R", "18 0 R 19 0 R 17 0 R", 1)
			}
			d := readRecovery(t, recoveryPDF(objects...))
			if name == "continuation last" {
				if d.Blocks[0].Rows[1].Cells[0].RowSpan != 2 || len(d.Blocks[0].Rows[2].Cells) != 2 {
					t.Fatal("forced continuation interleaving not recovered")
				}
			} else {
				if !hasDiagnostic(d, "pdf_structure_table_grid_uncertain") || hasDiagnostic(d, "pdf_structure_spans_inferred") || d.Blocks[0].Rows[0].Cells[0].ColSpan != 1 || len(d.Blocks[0].Rows[2].Cells) != 3 {
					t.Fatal("unproven spans or cell loss")
				}
				if name == "visible continuation" && strings.Count(d.Text(), "VISIBLE") != 1 {
					t.Fatal("visible cell discarded")
				}
			}
			assertRecoverySF(t, d)
		})
	}
}

func TestRealPDFMQTTTaggedTableMembership(t *testing.T) {
	raw, err := os.ReadFile("testdata/real/mqtt-v5.0-os.pdf")
	if err != nil {
		t.Fatal(err)
	}
	oracleRaw, err := os.ReadFile("testdata/real/mqtt-v5.0-pdf-oracle.json")
	if err != nil {
		t.Fatal(err)
	}
	var oracle struct {
		Tables []struct {
			ID    int
			Rows  int
			Cells [][]struct {
				ID   int
				Role string
			}
		}
	}
	if err := json.Unmarshal(oracleRaw, &oracle); err != nil {
		t.Fatal(err)
	}
	wordRaw, err := os.ReadFile("testdata/real/mqtt-v5.0-oracle.json")
	if err != nil {
		t.Fatal(err)
	}
	var word struct {
		Tables []struct {
			Cells []struct {
				Row     int
				Column  int
				ColSpan int    `json:"colspan"`
				RowSpan int    `json:"rowspan"`
				TextSHA string `json:"text_sha256"`
			}
		}
	}
	if err := json.Unmarshal(wordRaw, &word); err != nil {
		t.Fatal(err)
	}
	d, err := ReadPDF(context.Background(), bytes.NewReader(raw), ReadOptions{PDFSkipInference: true})
	if err != nil {
		t.Fatal(err)
	}
	tables := map[string]Block{}
	var walk func([]Block)
	walk = func(bs []Block) {
		for _, b := range bs {
			if b.Kind == Table && b.Source != nil {
				tables[b.Source.Element] = b
			}
			for _, r := range b.Rows {
				for _, c := range r.Cells {
					walk(c.Blocks)
				}
			}
			for _, item := range b.Items {
				walk(item.Blocks)
			}
		}
	}
	walk(d.Blocks)
	if len(d.Pages) != 137 || len(tables) != len(oracle.Tables) {
		t.Fatal("page or table membership lost", len(d.Pages), len(tables))
	}
	origins, continuations, inferred, uncertain := 0, 0, 0, 0
	for _, want := range oracle.Tables {
		b, ok := tables[fmt.Sprintf("(%d 0 R)", want.ID)]
		if !ok || b.Source.Structure != "explicit" || len(b.Rows) != want.Rows {
			t.Fatal("explicit table rows lost", want.ID)
		}
		got := map[string]string{}
		inferredTable := false
		for ri, row := range b.Rows {
			for _, c := range row.Cells {
				if c.Source == nil || c.Source.Structure != "explicit" {
					t.Fatal("explicit cell lost", want.ID)
				}
				key := fmt.Sprintf("%d/%s", ri, c.Source.Element)
				if _, exists := got[key]; exists {
					t.Fatal("duplicate origin", key)
				}
				got[key] = "/" + c.Source.Role
				origins++
				if spans := c.Source.TableSpans; spans != nil {
					inferredTable = true
					for _, ref := range spans.Continuations {
						key := fmt.Sprintf("%d/%s", ref.Row, ref.Element)
						if _, exists := got[key]; exists {
							t.Fatal("duplicate continuation", key)
						}
						got[key] = "/" + ref.Role
						continuations++
					}
				}
			}
		}
		if inferredTable {
			inferred++
		}
		if b.Source.Evidence == "pdf_structure_tree_ragged_grid" {
			uncertain++
		}
		count := 0
		for ri, row := range want.Cells {
			for _, c := range row {
				key := fmt.Sprintf("%d/(%d 0 R)", ri, c.ID)
				if got[key] != c.Role {
					t.Fatal("cell identity/row/role lost", want.ID, key, got[key], c.Role)
				}
				count++
			}
		}
		if len(got) != count {
			t.Fatal("unexpected cell membership", want.ID, len(got), count)
		}
		if want.ID == 4887 {
			for _, row := range b.Rows[1:] {
				if len(row.Cells) != 2 || row.Cells[1].ColSpan != 8 {
					t.Fatal("independently proven page-17 merge lost")
				}
			}
		}
		if want.ID == 12539 {
			g, err := tableGrid(b.Rows)
			if err != nil || g.Columns != 17 || len(g.Origins) != len(word.Tables[51].Cells) {
				t.Fatal("source shifted grid lost", err)
			}
			for i, p := range g.Origins {
				w := word.Tables[51].Cells[i]
				c := b.Rows[p.Row].Cells[p.Cell]
				hash := fmt.Sprintf("%x", sha256.Sum256([]byte(strings.Join(strings.Fields((&Document{Blocks: c.Blocks}).Text()), " "))))
				if p.Row != w.Row || p.Column != w.Column || p.ColSpan != w.ColSpan || p.RowSpan != w.RowSpan || hash != w.TextSHA {
					t.Fatalf("Word table 51 cell %d coordinate/span/content differs: %+v %+v", i, p, w)
				}
			}
		}
	}
	if origins != 3009 || origins+continuations != 3020 || continuations != 11 || uncertain != 0 || inferred != 42 {
		t.Fatal("corpus cell accounting/uncertainty changed", origins, continuations, uncertain)
	}
	t.Logf("tables=%d inferred=%d uncertain=%d origins=%d continuations=%d", len(tables), inferred, uncertain, origins, continuations)
}
func TestPDFTaggedMissingSpansUsePaintedGrid(t *testing.T) {
	d := readRecovery(t, taggedSpanPDF())
	if len(d.Blocks) != 1 || d.Blocks[0].Kind != Table {
		t.Fatal("explicit table lost", d.Text())
	}
	b := d.Blocks[0]
	g, err := tableGrid(b.Rows)
	if err != nil || g.Columns != 3 || len(g.Origins) != 7 || len(b.Rows[2].Cells) != 2 {
		t.Fatal("painted merged grid not recovered", g, err)
	}
	top, vert := b.Rows[0].Cells[0], b.Rows[1].Cells[0]
	if top.ColSpan != 2 || vert.RowSpan != 2 {
		t.Fatal("missing merged spans", top.ColSpan, vert.RowSpan)
	}
	if top.Source.Structure != "explicit" || top.Source.TableSpans == nil || top.Source.TableSpans.ColSpan != "inferred" || top.Source.TableSpans.Confidence >= 1 {
		t.Fatal("membership and inference conflated", top.Source)
	}
	refs := vert.Source.TableSpans.Continuations
	if len(refs) != 1 || refs[0].Element != "(17 0 R)" || refs[0].Role != "TD" || refs[0].Page != 1 {
		t.Fatal("empty explicit continuation identity lost", refs)
	}
	if !hasDiagnostic(d, "pdf_structure_spans_inferred") {
		t.Fatal("span inference not disclosed")
	}
	for _, word := range []string{"TOP", "RIGHT", "VERT", "MID", "R1", "LOW", "R2"} {
		if strings.Count(d.Text(), word) != 1 {
			t.Fatal("text loss or duplication", word, d.Text())
		}
	}
	assertRecoverySF(t, d)
}

func TestRulingJoinAllowsOperandQuantizationOnly(t *testing.T) {
	lines := []pdfRuling{{horizontal: true, pos: 10, start: .250005, end: 99.749995, width: .48}}
	if got := rulingCoverage(lines, true, 10, 0, 100); got != 1 {
		t.Fatal("rounded boundary join was treated as missing paint", got)
	}
	lines[0].start = .251
	if got := rulingCoverage(lines, true, 10, 0, 100); got != -1 {
		t.Fatal("genuine gap was hidden by precision guard", got)
	}
}
