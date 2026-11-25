package manifold

import (
	"bytes"
	"context"
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
