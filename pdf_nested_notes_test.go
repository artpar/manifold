package manifold

import (
	"bytes"
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func nestedDefinitionObjects() []string {
	o := explicitNoteObjects("Note")
	child := len(o) + 1
	o[9] = strings.Replace(o[9], "/Ref [9 0 R 19 0 R]", "/Ref [9 0 R]", 1)
	o[9] = strings.Replace(o[9], "21 0 R]", fmt.Sprintf("21 0 R %d 0 R]", child), 1)
	o[18] = strings.Replace(o[18], "/K 7", fmt.Sprintf("/Ref [%d 0 R] /K 7", child), 1)
	content := strings.SplitN(strings.SplitN(o[4], "stream\n", 2)[1], "\nendstream", 2)[0]
	content = strings.Replace(content, "125 300 Tm (*1)", "125 300 Tm (*2)", 1)
	content += markedText(11, 20, 15, "*2") + markedText(12, 40, 15, "CHILD BODY")
	o[4] = recoveryStream("", content)
	o = append(o, fmt.Sprintf("<< /Type /StructElem /S /Note /P 10 0 R /Pg 3 0 R /ID (note-2) /Ref [19 0 R] /K [%d 0 R %d 0 R] >>", child+1, child+2), fmt.Sprintf("<< /Type /StructElem /S /Lbl /P %d 0 R /K 11 >>", child), fmt.Sprintf("<< /Type /StructElem /S /P /P %d 0 R /K 12 >>", child))
	return o
}
func redundantNoteWrapperObjects() []string {
	o := explicitNoteObjects("Note")
	wrapper := len(o) + 1
	o[7] = strings.Replace(o[7], "/K [0 9 0 R", fmt.Sprintf("/K [0 %d 0 R", wrapper), 1)
	o[8] = strings.Replace(o[8], "/P 8 0 R", fmt.Sprintf("/P %d 0 R", wrapper), 1)
	o = append(o, "<< /Type /StructElem /S /Span /P 8 0 R /Ref [10 0 R] /K 9 0 R >>")
	return o
}
func TestPDFNestedIndependentNoteDefinitions(t *testing.T) {
	d := readRecovery(t, recoveryPDF(nestedDefinitionObjects()...))
	refs := collectedNoteRefs(d)
	if len(d.Notes) != 2 || len(refs) != 2 {
		t.Fatal("independently referenced nested definition flattened", len(d.Notes), len(refs), d.Diagnostics)
	}
	if d.Notes[0].ID != "note-1" || d.Notes[1].ID != "note-2" || refs[0].Note.ID != "note-1" || refs[1].Note.ID != "note-2" {
		t.Fatal("logical order/relationships changed", d.Notes, refs)
	}
	if len(d.Notes[0].Blocks) != 4 || len(d.Notes[1].Blocks) != 1 || strings.Count(d.Text(), "CHILD BODY") != 1 || strings.Count(d.Text(), "NOTE BODY") != 1 {
		t.Fatal("nested bodies duplicated/dropped", d.Text())
	}
	if !hasDiagnostic(d, "pdf_note_nested_definition") {
		t.Fatal("flattened definition containment not diagnosed")
	}
	if d.Notes[0].Source.ParentElement != "(7 0 R)" || d.Notes[1].Source.ParentElement != d.Notes[0].Source.Element {
		t.Fatal("original containment lost or confused with a citation", d.Notes[0].Source, d.Notes[1].Source)
	}
	assertRecoverySF(t, d)
	before, _ := ToStructured(d)
	for _, format := range []Format{SF, SFGzip, JSON, DOCX} {
		var out bytes.Buffer
		if err := Write(context.Background(), &out, d, format, WriteOptions{AllowLossy: true}); err != nil {
			t.Fatal(format, err)
		}
		back, err := Read(context.Background(), &out, format, ReadOptions{})
		if err != nil {
			t.Fatal(format, err)
		}
		after, _ := ToStructured(back)
		if format != DOCX && !reflect.DeepEqual(before, after) {
			t.Fatal("source graph changed", format)
		}
		if len(back.Notes) != 2 || len(collectedNoteRefs(back)) != 2 || back.Notes[0].ID != "note-1" || back.Notes[1].ID != "note-2" {
			t.Fatal("note graph/order changed", format)
		}
	}
}
func TestPDFRedundantNoteWrapper(t *testing.T) {
	d := readRecovery(t, recoveryPDF(redundantNoteWrapperObjects()...))
	if len(d.Notes) != 1 || len(collectedNoteRefs(d)) != 2 || hasDiagnostic(d, "pdf_note_relationship") {
		t.Fatal("same-target wrapper introduced ambiguity or duplicated atom", d.Diagnostics)
	}
	edges := d.Blocks[0].Source.References
	found := false
	for _, edge := range edges {
		if edge.Element == "(22 0 R)" && len(edge.Targets) == 1 && edge.Targets[0] == "(10 0 R)" {
			found = true
		}
	}
	if !found {
		t.Fatal("redundant explicit Ref edge discarded", edges)
	}
	assertRecoverySF(t, d)
}

func TestPDFNoteWrapperBoundariesRetainRefEvidence(t *testing.T) {
	for _, opaque := range []bool{false, true} {
		o := redundantNoteWrapperObjects()
		if opaque {
			o[len(o)-1] = strings.Replace(o[len(o)-1], "/S /Span", "/S /Span /ActualText (DIFFERENTMARK)", 1)
		} else {
			o[7] = strings.Replace(o[7], "/K [0 22 0 R", "/K [22 0 R", 1)
			o[len(o)-1] = strings.Replace(o[len(o)-1], "/K 9 0 R", "/K [0 9 0 R]", 1)
		}
		d := readRecovery(t, recoveryPDF(o...))
		refs := collectedNoteRefs(d)
		expected := 2
		if opaque {
			expected = 1
		}
		if len(d.Notes) != 1 || len(refs) != expected || !hasDiagnostic(d, "pdf_note_relationship") {
			t.Fatal("wrapper widened into an invented citation", d.Notes, refs, d.Diagnostics)
		}
		if opaque && !strings.Contains(d.Text(), "DIFFERENTMARK") {
			t.Fatal("replacement text discarded")
		}
		edges := d.Blocks[0].Source.References
		for _, owner := range []string{"(9 0 R)", "(22 0 R)"} {
			found := false
			for _, edge := range edges {
				if edge.Element == owner && len(edge.Targets) == 1 && edge.Targets[0] == "(10 0 R)" {
					found = true
				}
			}
			if !found {
				t.Fatal("unrepresented original Ref owner lost during paragraph merge", owner, edges)
			}
		}
		assertRecoverySF(t, d)
	}
}
func TestPDFNestedNoteContainmentValidation(t *testing.T) {
	d := readRecovery(t, recoveryPDF(nestedDefinitionObjects()...))
	d.Notes[1].Source.ParentElement = d.Notes[1].Source.Element
	if err := d.Validate(); err == nil {
		t.Fatal("self-containing source element accepted")
	}
}

func TestPDFNoteWrapperChainsAndConflicts(t *testing.T) {
	t.Run("same target chain", func(t *testing.T) {
		o := redundantNoteWrapperObjects()
		inner := len(o) + 1
		o[8] = strings.Replace(o[8], "/P 22 0 R", fmt.Sprintf("/P %d 0 R", inner), 1)
		o[len(o)-1] = strings.Replace(o[len(o)-1], "/K 9 0 R", fmt.Sprintf("/K %d 0 R", inner), 1)
		o = append(o, "<< /Type /StructElem /S /Span /P 22 0 R /Ref [10 0 R] /K 9 0 R >>")
		d := readRecovery(t, recoveryPDF(o...))
		refs := collectedNoteRefs(d)
		if len(refs) != 2 || hasDiagnostic(d, "pdf_note_relationship") || len(refs[0].Source.References) != 3 || refs[0].Source.ParentElement != "(23 0 R)" {
			t.Fatal("chain aliases split or lost original owners", refs, d.Diagnostics)
		}
		for _, owner := range []string{"(9 0 R)", "(22 0 R)", "(23 0 R)"} {
			found := false
			for _, edge := range refs[0].Source.References {
				found = found || (edge.Element == owner && len(edge.Targets) == 1 && edge.Targets[0] == "(10 0 R)")
			}
			if !found {
				t.Fatal("lost original wrapper Ref", owner, refs[0].Source.References)
			}
		}
		assertRecoverySF(t, d)
	})
	t.Run("different target", func(t *testing.T) {
		o := nestedDefinitionObjects()
		wrapper := len(o) + 1
		o[7] = strings.Replace(o[7], "/K [0 9 0 R", fmt.Sprintf("/K [0 %d 0 R", wrapper), 1)
		o[8] = strings.Replace(o[8], "/P 8 0 R", fmt.Sprintf("/P %d 0 R", wrapper), 1)
		o = append(o, "<< /Type /StructElem /S /Span /P 8 0 R /Ref [22 0 R] /K 9 0 R >>")
		d := readRecovery(t, recoveryPDF(o...))
		refs := collectedNoteRefs(d)
		if len(refs) != 2 || len(d.Notes) != 2 || !hasDiagnostic(d, "pdf_note_relationship") || refs[0].Note.ID != "note-1" || refs[1].Note.ID != "note-2" {
			t.Fatal("conflicting wrapper selected instead of specific original sites", refs, d.Diagnostics)
		}
		found := false
		for _, edge := range d.Blocks[0].Source.References {
			found = found || (edge.Element == "(25 0 R)" && len(edge.Targets) == 1 && edge.Targets[0] == "(22 0 R)")
		}
		if !found {
			t.Fatal("conflicting original edge lost", d.Blocks[0].Source.References)
		}
		assertRecoverySF(t, d)
	})
}
