package manydoc

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
)

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
	for _, tree := range []string{"<< /Nums [0 [8 0 R 9 0 R]] >>", "<< /Nums [0 [9 0 R 8 0 R]] >>", "<< /Kids [10 0 R] >>", "<< /Nums [0 [8 0 R null]] >>"} {
		f.Add(tree)
	}
	f.Fuzz(func(t *testing.T, tree string) {
		if len(tree) > 8192 {
			t.Skip()
		}
		// Keep this target confined to the index object: object/xref injection
		// could legitimately change the forward fixture rather than its index.
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
