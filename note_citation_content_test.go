package manifold

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func richCitationDocument(sectioned bool) *Document {
	d := variantNoteDocument()
	_ = walkNoteRuns(d.Blocks, func(run Run) error {
		if run.Note != nil {
			run.Note.Label = nil
			run.Note.LabelContent = []Run{{Text: "N", Bold: true, FontFamily: "Go", FontSize: 9, Color: "#203040"}, {Text: "-1", Italic: true, FontFamily: "Go", FontSize: 8}}
		}
		return nil
	})
	if sectioned {
		d.Blocks = []Block{{Kind: SectionBlock, Section: &Section{Blocks: d.Blocks}}}
	}
	return d
}
func TestRichCitationAllPaths(t *testing.T) {
	for _, sectioned := range []bool{false, true} {
		d := richCitationDocument(sectioned)
		before, err := ToStructured(d)
		if err != nil {
			t.Fatal(err)
		}
		source, _ := json.Marshal(d)
		for _, format := range []Format{SF, SFGzip, JSON, DOCX} {
			var out bytes.Buffer
			if err := Write(context.Background(), &out, d, format, WriteOptions{AllowLossy: true}); err != nil {
				t.Fatal(format, err)
			}
			got, err := Read(context.Background(), &out, format, ReadOptions{})
			if err != nil {
				t.Fatal(format, err)
			}
			after, err := ToStructured(got)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before.Content, after.Content) || !reflect.DeepEqual(before.Attrs.Notes, after.Attrs.Notes) {
				t.Fatalf("rich citation semantics changed through %s", format)
			}
		}
		unchanged, _ := json.Marshal(d)
		if !bytes.Equal(source, unchanged) {
			t.Fatal("source changed")
		}
		if err := Write(context.Background(), new(bytes.Buffer), d, PDF, WriteOptions{}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRichCitationSharedMarksCanonicalizeWithoutMutation(t *testing.T) {
	d := richCitationDocument(false)
	run := &d.Blocks[0].Runs[1]
	run.Bold = true
	run.FontSize = 12
	run.Note.LabelContent[1].FontSize = 0
	raw, _ := json.Marshal(d)
	sf, err := ToStructured(d)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := FromStructured(sf)
	if err != nil {
		t.Fatal(err)
	}
	got := canonical.Blocks[0].Runs[1]
	if got.Bold || got.FontSize != 0 || !got.Note.LabelContent[1].Bold || got.Note.LabelContent[1].FontSize != 12 || got.Note.LabelContent[0].FontSize != 9 {
		t.Fatal("shared marks not materialized with explicit fragment overrides")
	}
	var word bytes.Buffer
	if err := Write(context.Background(), &word, d, DOCX, WriteOptions{AllowLossy: true}); err != nil {
		t.Fatal(err)
	}
	back, err := ReadDOCX(context.Background(), &word)
	if err != nil {
		t.Fatal(err)
	}
	after, err := ToStructured(back)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(sf.Content, after.Content) {
		t.Fatal("canonical styles changed in Word")
	}
	current, _ := json.Marshal(d)
	if !bytes.Equal(raw, current) {
		t.Fatal("canonicalization mutated caller")
	}
}

func TestRichCitationValidation(t *testing.T) {
	for _, change := range []func(*NoteReference){
		func(r *NoteReference) { r.Label = noteLabel("duplicate") },
		func(r *NoteReference) { r.LabelContent = []Run{} },
		func(r *NoteReference) { r.LabelContent = []Run{{Text: ""}} },
		func(r *NoteReference) { r.LabelContent = []Run{{Text: "\n"}} },
		func(r *NoteReference) { r.LabelContent = []Run{{Text: "x", Note: r}} },
		func(r *NoteReference) { r.LabelContent = []Run{{Text: "x", Anchor: "unexpected"}} },
		func(r *NoteReference) { r.LabelContent = []Run{{Text: "x", Link: "https://example.org"}} },
		func(r *NoteReference) { r.LabelContent = []Run{{Text: "x", Destination: "target"}} },
		func(r *NoteReference) { r.LabelContent = []Run{{Text: "x", FontSize: -1}} },
	} {
		d := richCitationDocument(false)
		change(d.Blocks[0].Runs[1].Note)
		if err := d.Validate(); err == nil {
			t.Fatal("invalid rich citation accepted")
		}
	}
	d := richCitationDocument(false)
	sf, _ := ToStructured(d)
	sf.Content[0].Content[1].Marks = []StructuredMark{{Type: "textStyle", Attrs: sfAttrs(map[string]any{"fontSize": -1})}}
	if _, err := FromStructured(sf); err == nil {
		t.Fatal("invalid outer style was hidden by canonicalization")
	}
	sf.Content[0].Content[1].Marks = nil
	sf.Content[0].Content[1].Attrs = sfAttrs(map[string]any{"id": "shared", "kind": Endnote, "label_content": []StructuredNode{{Type: "note_reference"}}})
	if _, err := FromStructured(sf); err == nil {
		t.Fatal("nested SF citation atom accepted")
	}
}

func TestDOCXCitationIsolationChecksLiteralControls(t *testing.T) {
	d := richCitationDocument(false)
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, DOCX, WriteOptions{AllowLossy: true}); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(string) string{
		func(s string) string {
			return strings.Replace(s, `md:noteIsolation="ltr"`, `md:noteIsolation="rtl"`, 1)
		},
		func(s string) string { return strings.Replace(s, "\u2066", "visible", 1) },
		func(s string) string { return strings.Replace(s, "\u2069", "\u202c", 1) },
		func(s string) string {
			return strings.Replace(s, `md:noteIsolation="start"`, `md:noteIsolation="end"`, 1)
		},
		func(s string) string { return strings.Replace(s, docxCitationIsolationEnd, "", 1) },
		func(s string) string {
			return strings.Replace(s, docxCitationIsolationStart, `<w:r md:noteIsolation="start"><w:rPr><w:vanish/></w:rPr><w:t>`+"\u2066"+`</w:t></w:r>`, 1)
		},
	} {
		bad := rewriteListDOCX(t, out.Bytes(), change)
		if _, err := ReadDOCX(context.Background(), bytes.NewReader(bad)); err == nil {
			t.Fatal("contradictory isolation metadata accepted")
		}
	}
	// A legitimate literal edit remains the single visible text authority.
	edited := rewriteListDOCX(t, out.Bytes(), func(s string) string { return strings.Replace(s, `>N</w:t>`, `>EDIT</w:t>`, 1) })
	back, err := ReadDOCX(context.Background(), bytes.NewReader(edited))
	if err != nil || back.Blocks[0].Runs[1].Note.LabelContent[0].Text != "EDIT" {
		t.Fatal("visible rich citation edit lost", err)
	}
}

func TestRichCitationAtomicPaginationBox(t *testing.T) {
	r := textTestRenderer()
	r.notes.labels[noteKey{Footnote, "n"}] = "1"
	ref := Run{Note: &NoteReference{Kind: Footnote, ID: "n", LabelContent: []Run{{Text: "N", FontSize: 10, Bold: true}, {Text: "-1", FontSize: 8, Italic: true}}}}
	atoms, err := r.textAtoms([]Run{{Text: "ABC"}, ref}, 11)
	if err != nil {
		t.Fatal(err)
	}
	if len(atoms) != 3 || len(atoms[2].spans) != 2 || atoms[2].spans[1].citation == nil || len(lineNoteKeys(atoms[2].spans)) != 1 {
		t.Fatal("citation split from its preceding anchor or lost note admission")
	}
	box := atoms[2].spans[1].citation
	if len(box.line) != 2 || box.line[0].run.FontSize != 7 || box.line[1].run.FontSize != 5.6 || box.line[0].run.Note != nil || box.line[1].run.Note != nil {
		t.Fatal("inner text not shaped independently at declared superscript size")
	}
	lines, err := r.lines([]Run{{Text: "B A"}, ref}, atoms[2].width+1, 11)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 2 || len(lineNoteKeys(lines[0])) != 0 || len(lineNoteKeys(lines[1])) != 1 {
		t.Fatal("citation wrapping/admission lost atomic ownership")
	}
}

func TestPDFRichCitationStylesAndSource(t *testing.T) {
	o := explicitNoteObjects("Note")
	content := strings.SplitN(strings.SplitN(o[4], "stream\n", 2)[1], "\nendstream", 2)[0]
	mixed := "q BT /F 10 Tf /P << /MCID 1 >> BDC 1 0 0 1 60 300 Tm (*) Tj /F 8 Tf 1 0 0 rg (1) Tj EMC ET Q\n"
	o[4] = recoveryStream("", strings.Replace(content, markedText(1, 60, 300, "*1"), mixed, 1))
	d := readRecovery(t, recoveryPDF(o...))
	if len(d.Notes) != 1 {
		t.Fatal("lost rich citation relationship")
	}
	var rich *NoteReference
	_ = walkNoteRuns(d.Blocks, func(run Run) error {
		if run.Note != nil && run.Note.LabelContent != nil {
			rich = run.Note
		}
		return nil
	})
	if rich == nil || len(rich.LabelContent) != 2 || rich.Label != nil {
		t.Fatal("citation styles collapsed to a common mark")
	}
	a, b := rich.LabelContent[0], rich.LabelContent[1]
	if a.Text != "*" || b.Text != "1" || a.FontSize != 10 || b.FontSize != 8 || a.Color != "#000000" || b.Color != "#FF0000" || a.Source == nil || b.Source == nil || a.Source.Quad == nil || b.Source.Quad == nil {
		t.Fatal("individual citation paint or positions lost", rich)
	}
	assertRecoverySF(t, d)
}
