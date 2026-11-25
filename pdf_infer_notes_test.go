package manydoc

import (
	"bytes"
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func untaggedNoteSource(options string) []byte {
	label := "1"
	back := "/Dest [3 0 R /XYZ 80 308 null]"
	switch options {
	case "mismatch":
		label = "2"
	case "oneway":
		back = ""
	case "partial":
		back = "/Dest [3 0 R /XYZ null 308 null]"
	case "distant":
		back = "/Dest [3 0 R /XYZ 160 308 null]"
	}
	content := "BT /F 11 Tf 20 300 Td (Main text) Tj /F 6 Tf 1 0 0 1 80 304 Tm (1) Tj /F 11 Tf 1 0 0 1 84 300 Tm ( more) Tj ET BT /F 6 Tf 20 53 Td (" + label + ") Tj /F 10 Tf 1 0 0 1 24 50 Tm ( body note) Tj ET"
	if options == "unraised" {
		content = strings.Replace(content, "/F 6 Tf 20 53 Td", "/F 10 Tf 20 50 Td", 1)
	}
	annots := "/Annots [6 0 R 7 0 R]"
	extra := []string{}
	if options == "ambiguous" {
		annots = "/Annots [6 0 R 7 0 R 8 0 R]"
		extra = append(extra, "<< /Type /Annot /Subtype /Link /Rect [20 46 24 57] "+back+" >>")
	}
	if options == "repeated" {
		content += " BT /F 11 Tf 20 250 Td (Again) Tj /F 6 Tf 1 0 0 1 80 254 Tm (1) Tj /F 11 Tf 1 0 0 1 84 250 Tm ( here) Tj ET"
		annots = "/Annots [6 0 R 7 0 R 8 0 R]"
		extra = append(extra, "<< /Type /Annot /Subtype /Link /Rect [80 249 84 259] /Dest [3 0 R /XYZ 23 56 null] >>")
	}
	objects := []string{
		"<< /Type /Annot /Subtype /Link /Rect [80 299 84 309] /Dest [3 0 R /XYZ 23 56 null] >>",
		"<< /Type /Annot /Subtype /Link /Rect [20 46 24 57] " + back + " >>"}
	return recoveryPage(content, annots, append(objects, extra...)...)
}
func TestPDFUntaggedReciprocalNote(t *testing.T) {
	source := untaggedNoteSource("")
	d, err := ReadPDF(context.Background(), bytes.NewReader(source), ReadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	refs := collectedNoteRefs(d)
	if len(d.Notes) != 1 || len(refs) != 1 || !refs[0].Note.Inferred || !d.Notes[0].KindInferred || d.Notes[0].Kind != Footnote {
		t.Fatalf("note graph: notes=%+v refs=%+v diagnostics=%+v", d.Notes, refs, d.Diagnostics)
	}
	if d.Notes[0].Label != "1" || len(d.Notes[0].Blocks) != 1 || layoutText(d.Notes[0].Blocks[0]) != " body note" || len(d.Blocks) != 1 || !strings.Contains(d.Text(), "Main text") {
		t.Fatalf("note/body structure: %+v", d)
	}
	if refs[0].Source.Structure != "inferred" || refs[0].Source.Evidence != "pdf_note_reciprocal_views" || d.Notes[0].LabelSource == nil || len(d.PDFNavigation.Links) != 2 {
		t.Fatal("provenance/navigation lost")
	}
	assertRecoverySF(t, d)
	physical := readRecovery(t, source)
	if len(physical.Notes) != 0 {
		t.Fatal("skip-inference invented a note")
	}
	for _, run := range func() []Run {
		var rr []Run
		_ = walkNoteRuns(physical.Blocks, func(r Run) error { rr = append(rr, r); return nil })
		return rr
	}() {
		if run.Source == nil {
			continue
		}
		if run.Source.Order == refs[0].Source.Order {
			if !reflect.DeepEqual(run.Source.Quad, refs[0].Source.Quad) || !reflect.DeepEqual(run.Source.Baseline, refs[0].Source.Baseline) || run.FontSize != refs[0].FontSize {
				t.Fatal("citation glyph facts changed")
			}
		}
		if run.Source.Order == d.Notes[0].LabelSource.Order && !reflect.DeepEqual(run.Source.Quad, d.Notes[0].LabelSource.Quad) {
			t.Fatal("body label glyph geometry changed")
		}
	}
	if !reflect.DeepEqual(physical.PDFNavigation.Links, d.PDFNavigation.Links) {
		t.Fatal("physical annotation geometry changed")
	}
	for i, run := range d.Blocks[0].Runs {
		if run.Note != nil && (i == 0 || i == len(d.Blocks[0].Runs)-1) {
			t.Fatal("raised citation moved out of text order")
		}
	}

}
func TestPDFUntaggedNoteAmbiguityKeepsContent(t *testing.T) {
	for _, variant := range []string{"mismatch", "oneway", "partial", "distant", "unraised", "ambiguous"} {
		t.Run(variant, func(t *testing.T) {
			d, err := ReadPDF(context.Background(), bytes.NewReader(untaggedNoteSource(variant)), ReadOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if len(d.Notes) != 0 || len(collectedNoteRefs(d)) != 0 || !strings.Contains(d.Text(), "body note") {
				t.Fatal("unproven note graph or lost content", d.Text(), d.Notes)
			}
			assertRecoverySF(t, d)
		})
	}
}

func TestPDFUntaggedRepeatedNoteAndRoundTrips(t *testing.T) {
	d, err := ReadPDF(context.Background(), bytes.NewReader(untaggedNoteSource("repeated")), ReadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	refs := collectedNoteRefs(d)
	if len(d.Notes) != 1 || len(refs) != 2 || refs[0].Note.ID != refs[1].Note.ID {
		t.Fatal("repeated references lost", d.Notes, refs)
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
		if len(got.Notes) != 1 || len(rs) != 2 || !rs[0].Note.Inferred || !got.Notes[0].KindInferred || got.Notes[0].Label != "1" || layoutText(got.Notes[0].Blocks[0]) != " body note" {
			t.Fatal(format, "note semantics lost", got.Notes, rs)
		}
	}
}

func TestPDFUntaggedEndnoteOnlyPage(t *testing.T) {
	source := recoveryPDF(
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R 8 0 R] /Count 2 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 400] /Resources << /Font << /F 4 0 R >> >> /Contents 5 0 R /Annots [6 0 R] >>",
		"<< /Type /Font /Subtype /Type1 /BaseFont /Courier >>",
		recoveryStream("", "BT /F 11 Tf 20 300 Td (Main text) Tj /F 6 Tf 1 0 0 1 80 304 Tm (1) Tj /F 11 Tf 1 0 0 1 84 300 Tm ( more) Tj ET"),
		"<< /Type /Annot /Subtype /Link /Rect [80 299 84 309] /Dest [8 0 R /XYZ 23 356 null] >>",
		"<< /Type /Annot /Subtype /Link /Rect [20 346 24 357] /Dest [3 0 R /XYZ 80 308 null] >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 400] /Resources << /Font << /F 4 0 R >> >> /Contents 9 0 R /Annots [7 0 R] >>",
		recoveryStream("", "BT /F 6 Tf 20 353 Td (1) Tj /F 10 Tf 1 0 0 1 24 350 Tm ( body note) Tj ET"),
	)
	d, err := ReadPDF(context.Background(), bytes.NewReader(source), ReadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Notes) != 1 || d.Notes[0].Kind != Endnote || len(d.Pages) != 2 || len(d.Blocks) != 1 || !hasDiagnostic(d, "pdf_note_flow_boundary_inferred") {
		t.Fatal("endnote/physical pages/reflow boundary", d)
	}
	assertRecoverySF(t, d)
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, PDF, WriteOptions{AllowLossy: true, PDFFontMap: map[string]string{"Courier": "Go Mono"}}); err != nil {
		t.Fatal(err)
	}
	got := readRecovery(t, out.Bytes())
	if len(got.Pages) != 2 {
		t.Fatal("note-only source page produced blank reflow page", len(got.Pages))
	}
}
func TestPDFUntaggedNoteBudgetPreservesFlow(t *testing.T) {
	for _, remaining := range []int{0, 5, 12, 14, 18} {
		t.Run(fmt.Sprint(remaining), func(t *testing.T) {
			source := untaggedNoteSource("")
			d, err := ReadPDF(context.Background(), bytes.NewReader(source), ReadOptions{})
			if err != nil {
				t.Fatal(err)
			}
			// Reconstruct the pre-inference flow to exercise exhaustion before assignment.
			ref := collectedNoteRefs(d)[0]
			ref.Text, ref.Note = "1", nil
			for i := range d.Blocks[0].Runs {
				if d.Blocks[0].Runs[i].Note != nil {
					d.Blocks[0].Runs[i] = ref
				}
			}
			body := d.Notes[0].Blocks[0]
			label := *d.Notes[0].BodyLabelStyle
			label.Text, label.Source = "1", d.Notes[0].LabelSource
			body.Runs = append([]Run{label}, body.Runs...)
			d.Blocks = append(d.Blocks, body)
			d.Notes = nil
			before := append([]Block(nil), d.Blocks...)
			r := pdfReader{ctx: context.Background(), doc: d, noteInferenceWork: pdfNoteInferenceLimit - remaining}
			if err := r.inferLinkedPDFNotes(); err != nil {
				t.Fatal(err)
			}
			if len(d.Notes) != 0 || !reflect.DeepEqual(before, d.Blocks) || !hasDiagnostic(d, "pdf_note_inference_uncertain") {
				t.Fatal("exhaustion mutated source flow", d)
			}
			if r.noteInferenceWork != pdfNoteInferenceLimit+1 {
				t.Fatal("unbounded work", r.noteInferenceWork)
			}
		})
	}
}
