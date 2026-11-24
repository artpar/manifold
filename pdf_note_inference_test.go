package manydoc

import (
	"bytes"
	"context"
	"reflect"
	"strings"
	"testing"
)

func adjacentNoteObjects() []string {
	o := explicitNoteObjects("Note")
	o[6] = strings.Replace(o[6], "/K [8 0 R 10 0 R]", "/K 8 0 R", 1)
	o[7] = strings.Replace(o[7], "/K [0 9 0 R 6 19 0 R 8]", "/K [0 9 0 R 10 0 R 6 19 0 R 8]", 1)
	o[8] = strings.Replace(o[8], "/Ref [10 0 R] ", "", 1)
	o[9] = strings.Replace(o[9], "/Ref [9 0 R 19 0 R] ", "", 1)
	o[9] = strings.Replace(o[9], "/P 7 0 R", "/P 8 0 R", 1)
	return o
}
func collectedNoteRefs(d *Document) []Run {
	var refs []Run
	_ = walkNoteRuns(d.Blocks, func(run Run) error {
		if run.Note != nil {
			refs = append(refs, run)
		}
		return nil
	})
	return refs
}
func TestPDFAdjacentNoteInferenceRichGraph(t *testing.T) {
	d := readRecovery(t, recoveryPDF(adjacentNoteObjects()...))
	refs := collectedNoteRefs(d)
	if len(d.Notes) != 1 || len(refs) != 1 || !refs[0].Note.Inferred {
		t.Fatal("published adjacent pattern flattened", d.Notes, refs, d.Diagnostics)
	}
	if refs[0].Source.Structure != "explicit" || refs[0].Source.Evidence != "pdf_note_adjacent_reference" || refs[0].Source.Confidence != .85 || len(refs[0].Source.References) != 0 {
		t.Fatal("inferred relationship confused with explicit source edges", refs[0])
	}
	note := d.Notes[0]
	if note.Kind != Endnote || !note.KindInferred || !hasDiagnostic(d, "pdf_note_relationship_inferred") || len(note.Source.References) != 0 || !hasDiagnostic(d, "pdf_note_kind_inferred") {
		t.Fatal("kind and relationship inference conflated", note, d.Diagnostics)
	}
	if len(note.Blocks) != 4 || note.Blocks[1].Kind != ListBlock || note.Blocks[2].Kind != Table || note.Blocks[3].Kind != Picture || note.Blocks[3].Image.Alt != "RED BLUE" || note.LabelSource == nil {
		t.Fatal("rich note body/source lost", note)
	}
	// The other Span is not adjacent and has no Ref. It remains literal text.
	if strings.Count(d.Text(), "*1") != 1 || strings.Count(d.Text(), "NOTE BODY") != 1 {
		t.Fatal("inference consumed unrelated repeated label or duplicated body", d.Text())
	}
	assertRecoverySF(t, d)
	for _, format := range []Format{SF, SFGzip, JSON, DOCX} {
		var out bytes.Buffer
		if err := Write(context.Background(), &out, d, format, WriteOptions{AllowLossy: true}); err != nil {
			t.Fatal(format, err)
		}
		got, err := Read(context.Background(), &out, format, ReadOptions{})
		if err != nil {
			t.Fatal(format, err)
		}
		rs := collectedNoteRefs(got)
		if len(rs) != 1 || !rs[0].Note.Inferred || rs[0].Note.ID != note.ID {
			t.Fatal("relationship inference lost through", format, rs)
		}
	}
}
func TestPDFAdjacentNotesConservativeBoundaries(t *testing.T) {
	tests := []struct {
		name   string
		mutate func([]string)
	}{
		{"different labels", func(o []string) {
			content := strings.SplitN(strings.SplitN(o[4], "stream\n", 2)[1], "\nendstream", 2)[0]
			o[4] = recoveryStream("", strings.Replace(content, "20 80 Tm (*1)", "20 80 Tm (OTHER)", 1))
		}},
		{"intervening content", func(o []string) { o[7] = strings.Replace(o[7], "9 0 R 10 0 R 6", "9 0 R 6 10 0 R", 1) }},
		{"unresolved explicit ref", func(o []string) { o[8] = strings.Replace(o[8], "/S /Reference", "/S /Reference /Ref [999 0 R]", 1) }},
		{"body actualtext", func(o []string) {
			o[9] = strings.Replace(o[9], "/S /Note", "/S /Note /ActualText (NOTE REPLACEMENT)", 1)
		}},
		{"enclosing actualtext", func(o []string) { o[7] = strings.Replace(o[7], "/S /P", "/S /P /ActualText (WHOLE REPLACEMENT)", 1) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			objects := adjacentNoteObjects()
			test.mutate(objects)
			d := readRecovery(t, recoveryPDF(objects...))
			if len(d.Notes) != 0 || len(collectedNoteRefs(d)) != 0 {
				t.Fatal("unproven adjacent relationship invented", d.Notes, d.Diagnostics)
			}
			assertRecoverySF(t, d)
		})
	}
	explicit := readRecovery(t, recoveryPDF(explicitNoteObjects("Note")...))
	for _, run := range collectedNoteRefs(explicit) {
		if run.Note.Inferred {
			t.Fatal("explicit Ref marked inferred")
		}
	}
}
func TestDOCXNoteRelationshipInferenceSites(t *testing.T) {
	for _, variant := range []bool{false, true} {
		d := noteDocument()
		if variant {
			d = variantNoteDocument()
		}
		// First and repeated references may differ in provenance despite one identity.
		d.Blocks[0].Runs[1].Note.Inferred = true
		d.Blocks[2].Runs[1].Note.Inferred = true
		var out bytes.Buffer
		if err := Write(context.Background(), &out, d, DOCX, WriteOptions{AllowLossy: true}); err != nil {
			t.Fatal(err)
		}
		got, err := ReadDOCX(context.Background(), &out)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(collectedNoteRefs(d), collectedNoteRefs(got)) {
			t.Fatal("inference sites changed in Word", variant, collectedNoteRefs(got))
		}
	}
}

func linkedNoteObjects() []string {
	o := explicitNoteObjects("Note")
	o[8] = strings.Replace(o[8], "/S /Reference", "/S /Link", 1)
	o[8] = strings.Replace(o[8], "/Ref [10 0 R] ", "", 1)
	o[9] = strings.Replace(o[9], "/Ref [9 0 R 19 0 R] ", "", 1)
	o[8] = strings.Replace(o[8], "/K 1 >>", "/K [1 << /Type /OBJR /Obj 22 0 R /Pg 3 0 R >>] >>", 1)
	o[2] = strings.Replace(o[2], "/Contents 5 0 R", "/Annots [22 0 R] /Contents 5 0 R", 1)
	o = append(o, "<< /Type /Annot /Subtype /Link /Rect [58 296 75 312] /Dest [3 0 R /XYZ 20 80 null] >>")
	return o
}
func TestPDFPhysicalLinkNoteInference(t *testing.T) {
	d := readRecovery(t, recoveryPDF(linkedNoteObjects()...))
	refs := collectedNoteRefs(d)
	if len(d.Notes) != 1 || len(refs) != 1 || !refs[0].Note.Inferred || refs[0].Source.Evidence != "pdf_note_link_view" || refs[0].Source.Confidence != .95 || len(refs[0].Source.References) != 0 {
		t.Fatal("physical note link not identified separately", refs, d.Diagnostics)
	}
	if d.PDFNavigation == nil || len(d.PDFNavigation.Links) != 1 || d.PDFNavigation.Links[0].Destination == "" {
		t.Fatal("physical navigation disappeared")
	}
	assertRecoverySF(t, d)
	for _, test := range []struct{ name, from, to string }{
		{"far target", "/XYZ 20 80 null", "/XYZ 20 100 null"},
		{"partial view", "/XYZ 20 80 null", "/XYZ null 80 null"},
		{"wrong page region", "/XYZ 20 80 null", "/XYZ 150 80 null"},
	} {
		t.Run(test.name, func(t *testing.T) {
			o := linkedNoteObjects()
			o[len(o)-1] = strings.Replace(o[len(o)-1], test.from, test.to, 1)
			got := readRecovery(t, recoveryPDF(o...))
			if len(got.Notes) != 0 {
				t.Fatal("unproven Link target became note", got.Notes)
			}
			if !strings.Contains(got.Text(), "NOTE BODY") {
				t.Fatal("fallback lost visible content")
			}
		})
	}
	o := linkedNoteObjects()
	content := strings.SplitN(strings.SplitN(o[4], "stream\n", 2)[1], "\nendstream", 2)[0]
	o[4] = recoveryStream("", strings.Replace(content, "60 300 Tm (*1)", "60 300 Tm (OTHER)", 1))
	got := readRecovery(t, recoveryPDF(o...))
	if len(got.Notes) != 0 || !hasDiagnostic(got, "pdf_note_relationship_uncertain") {
		t.Fatal("mismatched text coerced into note")
	}
}

func TestPDFNoteLinkAmbiguityAndExplicitPriority(t *testing.T) {
	o := linkedNoteObjects()
	o[6] = strings.Replace(o[6], "/K [8 0 R 10 0 R]", "/K [8 0 R 10 0 R 23 0 R]", 1)
	content := strings.SplitN(strings.SplitN(o[4], "stream\n", 2)[1], "\nendstream", 2)[0]
	o[4] = recoveryStream("", content+markedText(11, 20, 80, "*1")+markedText(12, 40, 80, "SECOND NOTE"))
	o = append(o, "<< /Type /StructElem /S /Note /P 7 0 R /Pg 3 0 R /ID (second) /K [24 0 R 25 0 R] >>", "<< /Type /StructElem /S /Lbl /P 23 0 R /K 11 >>", "<< /Type /StructElem /S /P /P 23 0 R /K 12 >>")
	d := readRecovery(t, recoveryPDF(o...))
	if len(d.Notes) != 0 || !hasDiagnostic(d, "pdf_note_relationship_uncertain") || !strings.Contains(d.Text(), "SECOND NOTE") {
		t.Fatal("overlapping note regions guessed or content lost", d.Notes, d.Diagnostics)
	}
	explicit := linkedNoteObjects()
	explicit[8] = strings.Replace(explicit[8], "/S /Link", "/S /Link /Ref [10 0 R]", 1)
	d = readRecovery(t, recoveryPDF(explicit...))
	refs := collectedNoteRefs(d)
	if len(refs) != 1 || refs[0].Note.Inferred || refs[0].Source.Evidence != "pdf_note_explicit_ref" {
		t.Fatal("explicit structure edge lost priority", refs)
	}
}
