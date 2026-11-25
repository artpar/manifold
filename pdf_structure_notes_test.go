package manifold

import (
	"bytes"
	"context"
	"golang.org/x/image/font/gofont/gomonobold"
	"image"
	"image/color"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func explicitNoteObjects(kind string) []string {
	content := markedText(0, 20, 300, "BODY") + markedText(1, 60, 300, "*1") + markedText(2, 20, 80, "*1") + markedText(3, 40, 80, "NOTE BODY") + markedText(4, 40, 65, "LIST ITEM") + markedText(5, 40, 50, "TABLE CELL") + markedText(6, 80, 300, "MIDDLE") + markedText(7, 125, 300, "*1") + markedText(8, 145, 300, "AFTER")
	content += markedText(9, 20, 65, "1.") + "/Figure << /MCID 10 >> BDC q 20 0 0 10 40 30 cm BI /W 2 /H 1 /BPC 8 /CS /RGB /F /AHx ID FF00000000FF> EI Q EMC\n"
	objects := taggedTestObjects(content, "<< /Type /StructTreeRoot /K [8 0 R 10 0 R] >>",
		"<< /Type /StructElem /S /P /P 7 0 R /Pg 3 0 R /K [0 9 0 R 6 19 0 R 8] >>",
		"<< /Type /StructElem /S /Reference /P 8 0 R /Pg 3 0 R /Ref [10 0 R] /K 1 >>",
		"<< /Type /StructElem /S /"+kind+" /P 7 0 R /Pg 3 0 R /ID (note-1) /Ref [9 0 R 19 0 R] /K [11 0 R 12 0 R 13 0 R 16 0 R 21 0 R] >>",
		"<< /Type /StructElem /S /Lbl /P 10 0 R /K 2 >>",
		"<< /Type /StructElem /S /P /P 10 0 R /K 3 >>",
		"<< /Type /StructElem /S /L /A << /O /List /ListNumbering /Decimal >> /P 10 0 R /K 14 0 R >>",
		"<< /Type /StructElem /S /LI /P 13 0 R /K [20 0 R 15 0 R] >>",
		"<< /Type /StructElem /S /LBody /P 14 0 R /K 4 >>",
		"<< /Type /StructElem /S /Table /P 10 0 R /K 17 0 R >>",
		"<< /Type /StructElem /S /TR /P 16 0 R /K 18 0 R >>",
		"<< /Type /StructElem /S /TD /P 17 0 R /K 5 >>",
		// No outgoing Ref: the note's explicit reverse edge identifies this citation.
		"<< /Type /StructElem /S /Span /P 8 0 R /Pg 3 0 R /K 7 >>",
		"<< /Type /StructElem /S /Lbl /P 14 0 R /K 9 >>",
		"<< /Type /StructElem /S /Figure /P 10 0 R /Alt (RED BLUE) /K 10 >>")
	objects[0] = strings.Replace(objects[0], "/MarkInfo", "/Dests << /note [3 0 R /XYZ 40 80 null] >> /MarkInfo", 1)
	objects[3] = strings.Replace(objects[3], "/Courier", "/Courier-Bold", 1)
	return objects
}

func TestPDFExplicitNotesRichBodyAndReferences(t *testing.T) {
	for _, kind := range []string{"FENote /A << /O /FENote /NoteType /Footnote >>", "Note"} {
		t.Run(kind, func(t *testing.T) {
			d := readRecovery(t, recoveryPDF(explicitNoteObjects(kind)...))
			if len(d.Notes) != 1 || len(d.Blocks) != 1 {
				t.Fatalf("note flattened/duplicated: %#v %#v", d.Notes, d.Blocks)
			}
			n := d.Notes[0]
			wantKind := Footnote
			if kind == "Note" {
				wantKind = Endnote
			}
			if n.ID != "note-1" || n.Label != "*1" || n.Kind != wantKind || n.KindInferred != (kind == "Note") {
				t.Fatalf("note identity/label/kind: %+v", n)
			}
			if hasDiagnostic(d, "pdf_note_kind_inferred") != (kind == "Note") || hasDiagnostic(d, "pdf_structure_inline") {
				t.Fatal("explicit and inferred kinds or supported references conflated", d.Diagnostics)
			}
			if n.Source == nil || n.Source.Structure != "explicit" || n.Source.Element != "(10 0 R)" || string(n.Source.StructureID) != "note-1" || len(n.Source.References) != 1 {
				t.Fatalf("definition graph/provenance %+v", n.Source)
			}
			if n.LabelSource == nil || n.LabelSource.Role != "Lbl" || n.LabelSource.Font != "Courier-Bold" || n.LabelSource.FontSize != 10 || n.LabelSource.Quad == nil || !hasDiagnostic(d, "pdf_note_label_presentation") {
				t.Fatal("removed leading label lost its independent physical provenance", n.LabelSource)
			}
			if len(n.Blocks) != 4 || n.Blocks[0].Kind != Paragraph || n.Blocks[1].Kind != ListBlock || n.Blocks[2].Kind != Table || n.Blocks[3].Kind != Picture {
				t.Fatalf("rich note body %+v", n.Blocks)
			}
			if !n.Blocks[0].Runs[len(n.Blocks[0].Runs)-1].Bold || n.Blocks[1].Items[0].Blocks[0].Runs[0].Text != "LIST ITEM" || n.Blocks[2].Rows[0].Cells[0].Blocks[0].Runs[0].Text != "TABLE CELL" {
				t.Fatal("note styles/list/table content changed", n.Blocks)
			}
			refs := 0
			for _, run := range d.Blocks[0].Runs {
				if run.Note != nil {
					refs++
					if run.Text != "" || run.Note.ID != n.ID || run.Source.Structure != "explicit" || run.Source.Evidence != "pdf_note_explicit_ref" {
						t.Fatal("reference flattened", run)
					}
				}
			}
			if refs != 2 || strings.Count(d.Text(), "NOTE BODY") != 1 || strings.Contains(d.Text(), "*1") {
				t.Fatalf("lost/duplicated content or Lbl %q", d.Text())
			}
			im, _, err := image.Decode(bytes.NewReader(n.Blocks[3].Image.Data))
			if err != nil || color.NRGBAModel.Convert(im.At(0, 0)) != (color.NRGBA{255, 0, 0, 255}) || color.NRGBAModel.Convert(im.At(1, 0)) != (color.NRGBA{0, 0, 255, 255}) || n.Blocks[3].Image.Alt != "RED BLUE" {
				t.Fatal("note raster pixels/alt lost", err)
			}
			target := d.PDFNavigation.Destinations[0].ID
			found := slices.Contains(n.Blocks[0].Anchors, target)
			for _, run := range n.Blocks[0].Runs {
				found = found || run.Anchor == target
			}
			if !found {
				t.Fatal("physical navigation did not bind inside relocated note")
			}
			assertRecoverySF(t, d)
			for _, format := range []Format{SF, SFGzip, JSON} {
				var buf bytes.Buffer
				if err := Write(context.Background(), &buf, d, format, WriteOptions{}); err != nil {
					t.Fatal(err)
				}
				got, err := Read(context.Background(), &buf, format, ReadOptions{})
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(d.Notes, got.Notes) {
					t.Fatal("note provenance graph lost through", format)
				}
			}
			var out bytes.Buffer
			if err := Write(context.Background(), &out, d, PDF, WriteOptions{AllowLossy: true, PDFFonts: []FontFace{{Family: "Courier", Bold: true, Data: gomonobold.TTF}}}); err != nil {
				t.Fatal("native rich note layout", err)
			}
		})
	}
}

func TestPDFNoteAmbiguityAndMalformedRelationships(t *testing.T) {
	objects := explicitNoteObjects("Note")
	content := strings.SplitN(strings.SplitN(objects[4], "stream\n", 2)[1], "\nendstream", 2)[0]
	objects[4] = recoveryStream("", strings.Replace(content, "125 300 Tm (*1)", "125 300 Tm (DIFFERENT)", 1))
	d := readRecovery(t, recoveryPDF(objects...))
	if len(d.Notes) != 1 || hasDiagnostic(d, "pdf_note_label") || strings.Count(d.Text(), "NOTE BODY") != 1 {
		t.Fatal("explicit differing labels did not recover", d.Text(), d.Diagnostics)
	}
	found := false
	for _, b := range d.Blocks {
		if b.Source != nil {
			found = found || len(b.Source.References) > 0
		}
	}
	if !found {
		t.Fatal("unrepresented explicit Ref edges discarded")
	}
	for _, bad := range []string{"/Ref 10 0 R", "/Ref [10]"} {
		o := explicitNoteObjects("Note")
		o[8] = strings.Replace(o[8], "/Ref [10 0 R]", bad, 1)
		if _, err := ReadPDF(context.Background(), bytes.NewReader(recoveryPDF(o...)), ReadOptions{}); err == nil {
			t.Fatal("invalid Ref accepted", bad)
		}
	}
	o := explicitNoteObjects("FENote /A << /O /FENote /NoteType /Invalid >>")
	if _, err := ReadPDF(context.Background(), bytes.NewReader(recoveryPDF(o...)), ReadOptions{}); err == nil {
		t.Fatal("invalid known note attribute accepted")
	}
}

func TestPDFNoteActualTextAndRawIDs(t *testing.T) {
	o := explicitNoteObjects("Note")
	o[9] = strings.Replace(o[9], "/ID (note-1)", "/ID <007FFF> /ActualText (REPLACEMENT NOTE)", 1)
	d := readRecovery(t, recoveryPDF(o...))
	if len(d.Notes) != 1 || !bytes.Equal(d.Notes[0].Source.StructureID, []byte{0, 127, 255}) || d.Notes[0].Blocks[0].Runs[0].Text != "REPLACEMENT NOTE" {
		t.Fatal("binary ID or note ActualText lost", d.Notes)
	}
	assertRecoverySF(t, d)
}

func TestPDFNoteInlineAndEnclosingActualText(t *testing.T) {
	o := explicitNoteObjects("Note")
	o[6] = strings.Replace(o[6], "/K [8 0 R 10 0 R]", "/K 8 0 R", 1)
	o[7] = strings.Replace(o[7], "19 0 R 8]", "19 0 R 10 0 R 8]", 1)
	o[9] = strings.Replace(o[9], "/P 7 0 R", "/P 8 0 R", 1)
	d := readRecovery(t, recoveryPDF(o...))
	if len(d.Notes) != 1 || len(d.Blocks) != 1 || strings.Count(d.Text(), "NOTE BODY") != 1 {
		t.Fatal("inline Note splits paragraph or duplicates rich body", d.Text())
	}
	o[7] = strings.Replace(o[7], "/S /P", "/S /P /ActualText (REPLACEMENT BODY)", 1)
	d = readRecovery(t, recoveryPDF(o...))
	if len(d.Notes) != 0 || !hasDiagnostic(d, "pdf_note_relationship") || !strings.Contains(d.Text(), "REPLACEMENT BODY") {
		t.Fatal("enclosing ActualText became dangling note or silently lost replacement", d)
	}
	assertRecoverySF(t, d)
}
