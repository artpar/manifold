package manydoc

import (
	"bytes"
	"context"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
)

func noteLabel(s string) *string { return &s }
func variantNoteDocument() *Document {
	d := noteDocument()
	d.Notes[0].Label = "E"
	d.Notes[0].BodyLabel = noteLabel("§E")
	d.Notes[1].Label = "*α"
	d.Notes[1].BodyLabel = noteLabel("N-1")
	d.Blocks[2].Runs[1].Note.Label = noteLabel("see α")
	return d
}
func TestNoteVariantLabelsAllPaths(t *testing.T) {
	d := variantNoteDocument()
	for _, format := range []Format{SF, SFGzip, JSON, DOCX} {
		t.Run(string(format), func(t *testing.T) {
			var buf bytes.Buffer
			if err := Write(context.Background(), &buf, d, format, WriteOptions{AllowLossy: true}); err != nil {
				t.Fatal(err)
			}
			got, err := Read(context.Background(), &buf, format, ReadOptions{})
			if err != nil {
				t.Fatal(err)
			}
			before, err := ToStructured(d)
			if err != nil {
				t.Fatal(err)
			}
			after, err := ToStructured(got)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before.Content, after.Content) || !reflect.DeepEqual(before.Attrs.Notes, after.Attrs.Notes) {
				t.Fatalf("variant meanings changed through %s\n%+v\n%+v", format, got.Blocks, got.Notes)
			}
		})
	}
	if err := Write(context.Background(), new(bytes.Buffer), d, DOCX, WriteOptions{}); !errors.Is(err, ErrLossy) {
		t.Fatal("portable automatic numbering loss not gated", err)
	}
	var pdf bytes.Buffer
	if err := Write(context.Background(), &pdf, d, PDF, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	got, err := ReadPDF(context.Background(), &pdf, ReadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, mark := range []string{"*α", "N-1", "see α", "§E", "FIRST_NOTE_BODY", "SECOND_NOTE_BODY"} {
		if strings.Count(got.Text(), mark) != 1 {
			t.Fatalf("lost/duplicated mark %q: %s", mark, got.Text())
		}
	}
}

func TestPDFDifferentNoteLabelsRemainTyped(t *testing.T) {
	o := explicitNoteObjects("Note")
	content := strings.SplitN(strings.SplitN(o[4], "stream\n", 2)[1], "\nendstream", 2)[0]
	content = strings.Replace(content, "125 300 Tm (*1)", "125 300 Tm (OTHER)", 1)
	content = strings.Replace(content, "20 80 Tm (*1)", "20 80 Tm (N-1)", 1)
	o[4] = recoveryStream("", content)
	d := readRecovery(t, recoveryPDF(o...))
	if len(d.Notes) != 1 || d.Notes[0].BodyLabel == nil || *d.Notes[0].BodyLabel != "N-1" {
		t.Fatal("distinct body label not recovered", d.Notes)
	}
	labels := []string{}
	c, err := catalogNotes(d, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, run := range d.Blocks[0].Runs {
		if run.Note != nil {
			labels = append(labels, noteReferenceLabel(c, run.Note))
		}
	}
	if !reflect.DeepEqual(labels, []string{"*1", "OTHER"}) || hasDiagnostic(d, "pdf_note_label") || d.Notes[0].LabelSource == nil {
		t.Fatal("distinct citations flattened", labels, d.Diagnostics)
	}
	assertRecoverySF(t, d)
}

func TestMarkerlessNoteLabels(t *testing.T) {
	d := variantNoteDocument()
	d.Notes[1].BodyLabel = noteLabel("")
	d.Blocks[2].Runs[1].Note.Label = noteLabel("")
	for _, format := range []Format{SF, DOCX, PDF} {
		var buf bytes.Buffer
		if err := Write(context.Background(), &buf, d, format, WriteOptions{AllowLossy: true}); err != nil {
			t.Fatal(format, err)
		}
		if format == PDF {
			got, err := ReadPDF(context.Background(), &buf, ReadOptions{})
			if err != nil || !strings.Contains(got.Text(), "FIRST_NOTE_BODY") {
				t.Fatal("markerless site lost body", err)
			}
			continue
		}
		got, err := Read(context.Background(), &buf, format, ReadOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(d.Notes, got.Notes) || !reflect.DeepEqual(d.Blocks, got.Blocks) {
			t.Fatal("nil/empty marks conflated", format)
		}
	}
}

func TestPDFMarkerlessExplicitReferences(t *testing.T) {
	o := explicitNoteObjects("Note")
	content := strings.SplitN(strings.SplitN(o[4], "stream\n", 2)[1], "\nendstream", 2)[0]
	for _, line := range []string{markedText(1, 60, 300, "*1"), markedText(7, 125, 300, "*1"), markedText(2, 20, 80, "*1")} {
		content = strings.Replace(content, line, "", 1)
	}
	o[4] = recoveryStream("", content)
	o[8] = strings.Replace(o[8], "/K 1 >>", "/K [] >>", 1)
	o[18] = strings.Replace(o[18], "/K 7 >>", "/K [] >>", 1)
	o[10] = strings.Replace(o[10], "/K 2 >>", "/K [] >>", 1)
	d := readRecovery(t, recoveryPDF(o...))
	if len(d.Notes) != 1 || d.Notes[0].BodyLabel == nil || *d.Notes[0].BodyLabel != "" || !hasDiagnostic(d, "pdf_note_markerless") {
		t.Fatal("markerless explicit graph not recovered", d.Notes, d.Diagnostics)
	}
	count := 0
	_ = walkNoteRuns(d.Blocks, func(run Run) error {
		if run.Note != nil {
			count++
			if run.Note.Label == nil || *run.Note.Label != "" {
				t.Fatal("empty and inherited marks conflated")
			}
		}
		return nil
	})
	if count != 2 {
		t.Fatal("missing empty reference sites", count)
	}
	assertRecoverySF(t, d)
}

func TestIndependentBodyLabelGlyphValidation(t *testing.T) {
	d := variantNoteDocument()
	d.Notes[1].BodyLabel = noteLabel("注")
	var buf bytes.Buffer
	if err := Write(context.Background(), &buf, d, PDF, WriteOptions{}); !errors.Is(err, ErrUnsupported) || buf.Len() != 0 {
		t.Fatal("body-label glyph bypassed validation", err)
	}
}

func TestNoteVariantMetadataRejectsContradictions(t *testing.T) {
	d := variantNoteDocument()
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, DOCX, WriteOptions{AllowLossy: true}); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(string) string{
		func(s string) string {
			return strings.Replace(s, `w:anchor="mdnotebody1_footnote_1"`, `w:anchor="mdnotebody1_endnote_1"`, 1)
		},
		func(s string) string {
			return strings.Replace(s, `md:noteAnchor="mdnote1_footnote_1"`, `md:noteAnchor="mdnote1_footnote_2"`, 1)
		},
		func(s string) string {
			return strings.Replace(s, `<w:t>`+docxNoteAnchorMark+`</w:t>`, `<w:t>VISIBLE</w:t>`, 1)
		},
		func(s string) string { return strings.Replace(s, `>see α</w:t>`, `>see α</w:t><w:drawing/>`, 1) },
		func(s string) string {
			return strings.Replace(s, `<w:rStyle w:val="FootnoteReference"/>`, `<w:vanish/>`, 1)
		},
	} {
		bad := rewriteListDOCX(t, out.Bytes(), change)
		if bytes.Equal(bad, out.Bytes()) {
			t.Fatal("mutation missed source")
		}
		if _, err := ReadDOCX(context.Background(), bytes.NewReader(bad)); err == nil {
			t.Fatal("contradictory/visible content hidden by metadata")
		}
	}
}

func TestVariantCitationDirectionAndShaping(t *testing.T) {
	data, err := os.ReadFile("testdata/assets/DejaVuSans.ttf")
	if err != nil {
		t.Fatal(err)
	}
	r := textTestRenderer()
	if err := r.addFonts([]FontFace{{Family: "DejaVu", Data: data}}); err != nil {
		t.Fatal(err)
	}
	r.notes.labels[noteKey{Footnote, "n"}] = "1"
	label := "بب"
	lines, err := r.lines([]Run{{Note: &NoteReference{Kind: Footnote, ID: "n", Label: &label}, FontFamily: "DejaVu"}}, 200, 18)
	if err != nil {
		t.Fatal(err)
	}
	span := lines[0][0]
	if span.text != label || span.glyphs == nil || !span.glyphs.rtl {
		t.Fatal("RTL citation shaped in LTR direction", span)
	}
	mixed := "بb"
	composite, err := r.lines([]Run{{Note: &NoteReference{Kind: Footnote, ID: "n", Label: &mixed}, FontFamily: "DejaVu"}}, 200, 18)
	if err != nil {
		t.Fatal(err)
	}
	if len(composite) != 1 || len(composite[0]) != 1 || composite[0][0].citation == nil || composite[0][0].text != mixed || len(composite[0][0].citation.line) < 2 {
		t.Fatal("mixed citation did not retain one outer atom with independently shaped inner spans")
	}

}

func TestMissingCitationPaintIsNotMarkerlessProof(t *testing.T) {
	o := explicitNoteObjects("Note")
	content := strings.SplitN(strings.SplitN(o[4], "stream\n", 2)[1], "\nendstream", 2)[0]
	content = strings.Replace(content, markedText(1, 60, 300, "*1"), "", 1)
	content = strings.Replace(content, markedText(7, 125, 300, "*1"), "", 1)
	o[4] = recoveryStream("", content)
	d := readRecovery(t, recoveryPDF(o...))
	if len(d.Notes) != 0 || !hasDiagnostic(d, "pdf_structure_missing_content") || !hasDiagnostic(d, "pdf_note_label") {
		t.Fatal("missing paint misrepresented as explicitly empty citations", d.Notes, d.Diagnostics)
	}
	if !strings.Contains(d.Text(), "NOTE BODY") {
		t.Fatal("fallback lost note body")
	}
}
