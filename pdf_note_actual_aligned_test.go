package manydoc

import (
	"bytes"
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func alignedActualNoteObjects(nested bool) []string {
	o := explicitNoteObjects("Note")
	o[7] = strings.Replace(o[7], "/S /P", "/S /P /ActualText (BODY*1MIDDLE*1AFTER)", 1)
	if nested {
		id := len(o) + 1
		o[7] = strings.Replace(o[7], "/K [0 9 0 R", fmt.Sprintf("/K [0 %d 0 R", id), 1)
		o[8] = strings.Replace(o[8], "/P 8 0 R", fmt.Sprintf("/P %d 0 R", id), 1)
		o = append(o, "<< /Type /StructElem /S /Span /P 8 0 R /ActualText (*1) /K 9 0 R >>")
	}
	return o
}

func TestPDFNoteAlignedEnclosingActualText(t *testing.T) {
	for _, nested := range []bool{false, true} {
		d := readRecovery(t, recoveryPDF(alignedActualNoteObjects(nested)...))
		refs := collectedNoteRefs(d)
		if len(d.Notes) != 1 || len(refs) != 2 || hasDiagnostic(d, "pdf_note_relationship") {
			t.Fatal("exact ActualText correspondence lost note graph", d.Notes, refs, d.Diagnostics)
		}
		if strings.Count(d.Text(), "NOTE BODY") != 1 || strings.Contains(d.Text(), "*1") || !strings.Contains(d.Text(), "BODY[endnote:note-1]MIDDLE[endnote:note-1]AFTER") {
			t.Fatal("ActualText characters/body duplicated or dropped", d.Text())
		}
		for _, ref := range refs {
			if ref.Note.Inferred || ref.Source.Structure != "explicit" || !strings.Contains(ref.Source.Evidence, "pdf_structure_actualtext_aligned") {
				t.Fatal("explicit Ref/ActualText provenance lost", ref)
			}
		}
		if len(d.Notes[0].Blocks) != 4 || d.Notes[0].Blocks[3].Image.Alt != "RED BLUE" {
			t.Fatal("rich body affected by enclosing citation replacement")
		}
		assertRecoverySF(t, d)
		before, _ := ToStructured(d)
		for _, format := range []Format{SF, SFGzip, JSON, DOCX} {
			var out bytes.Buffer
			if err := Write(context.Background(), &out, d, format, WriteOptions{AllowLossy: true}); err != nil {
				t.Fatal(format, err)
			}
			got, err := Read(context.Background(), &out, format, ReadOptions{})
			if err != nil {
				t.Fatal(format, err)
			}
			after, _ := ToStructured(got)
			if format != DOCX && !reflect.DeepEqual(before, after) {
				t.Fatal("exact source graph changed through", format)
			}
			if len(got.Notes) != 1 || len(collectedNoteRefs(got)) != 2 {
				t.Fatal("note graph changed through", format)
			}
		}
	}
}

func TestPDFNoteActualTextRequiresFullExactCorrespondence(t *testing.T) {
	for _, replacement := range []string{"BEFORE*1MIDDLE*1AFTER", "BODY*1MIDDLE*1AFTER*1", "BODY *1MIDDLE*1AFTER"} {
		o := alignedActualNoteObjects(false)
		o[7] = strings.Replace(o[7], "BODY*1MIDDLE*1AFTER", replacement, 1)
		d := readRecovery(t, recoveryPDF(o...))
		if len(d.Notes) != 0 || len(collectedNoteRefs(d)) != 0 || !hasDiagnostic(d, "pdf_note_relationship") || !strings.Contains(d.Text(), replacement) {
			t.Fatal("replacement substring was guessed into an editable citation", d)
		}
		assertRecoverySF(t, d)
	}
}

func TestPDFNoteAlignedActualTextRetainsFragmentStyles(t *testing.T) {
	o := alignedActualNoteObjects(true)
	content := strings.SplitN(strings.SplitN(o[4], "stream\n", 2)[1], "\nendstream", 2)[0]
	mixed := "q BT /F 10 Tf /P << /MCID 1 >> BDC 1 0 0 1 60 300 Tm (*) Tj /F 8 Tf 1 0 0 rg (1) Tj EMC ET Q\n"
	o[4] = recoveryStream("", strings.Replace(content, markedText(1, 60, 300, "*1"), mixed, 1))
	d := readRecovery(t, recoveryPDF(o...))
	refs := collectedNoteRefs(d)
	if len(refs) != 2 || len(refs[0].Note.LabelContent) != 2 {
		t.Fatal("aligned structural replacement flattened rich citation")
	}
	a, b := refs[0].Note.LabelContent[0], refs[0].Note.LabelContent[1]
	if a.Text != "*" || a.FontSize != 10 || a.Color != "#000000" || b.Text != "1" || b.FontSize != 8 || b.Color != "#FF0000" || a.Source.Quad == nil || b.Source.Quad == nil || hasDiagnostic(d, "pdf_actualtext_style") {
		t.Fatal("exact intervals lost individual paint/style/geometry", a, b, d.Diagnostics)
	}
	assertRecoverySF(t, d)
}

func TestPDFNoteActualTextContainingDefinitionStaysOpaque(t *testing.T) {
	o := explicitNoteObjects("Note")
	id := len(o) + 1
	o[6] = strings.Replace(o[6], "10 0 R]", fmt.Sprintf("%d 0 R]", id), 1)
	o[9] = strings.Replace(o[9], "/P 7 0 R", fmt.Sprintf("/P %d 0 R", id), 1)
	o[9] = strings.Replace(o[9], "/K [11 0 R 12 0 R 13 0 R 16 0 R 21 0 R]", "/K 3", 1)
	o = append(o, "<< /Type /StructElem /S /Span /P 7 0 R /ActualText (NOTE BODY) /K 10 0 R >>")
	d := readRecovery(t, recoveryPDF(o...))
	if len(d.Notes) != 0 || len(collectedNoteRefs(d)) != 0 || strings.Count(d.Text(), "NOTE BODY") != 1 || !hasDiagnostic(d, "pdf_note_relationship") {
		t.Fatal("definition moved outside its replacement range or duplicated", d.Text(), d.Diagnostics)
	}
	assertRecoverySF(t, d)
}
