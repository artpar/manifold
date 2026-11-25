package engine

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
	"io"
	"os"
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
	back, err := ReadDOCX(context.Background(), bytes.NewReader(

		// Independent hand-authored references and implicit part relationships.

		edited))
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

		// OOXML requires a trailing paragraph after a table inside a cell, but these
		// note-level blocks have no added model text or duplicate image data.

	}
	a, b := rich.LabelContent[0], rich.LabelContent[1]
	if a.Text != "*" || b.Text != "1" || a.FontSize != 10 || b.FontSize != 8 || a.Color != "#000000" || b.Color != "#FF0000" || a.Source == nil || b.Source == nil || a.Source.Quad == nil || b.Source.Quad == nil {
		t.Fatal("individual citation paint or positions lost", rich)
	}
	assertRecoverySF(t, d)
}
func TestNoteBodyLabelStyleAllPaths(t *testing.T) {
	for _, link := range []string{"", "https://example.org/label"} {
		for _, destination := range []string{"", "citation"} {
			if link != "" && destination != "" {
				continue
			}
			d := variantNoteDocument()
			d.Blocks[0].Anchors = []string{"citation"}
			d.Notes[0].BodyLabelStyle = &Run{Bold: true, Italic: true, Underline: true, Strike: true, FontFamily: "Go", FontSize: 9.5, Color: "#203040", Language: "en", Link: link, Destination: destination}
			if destination != "" {
				d.Notes[0].BodyLabelStyle.Link = "#" + destination
			}
			d.Notes[1].BodyLabelStyle = &Run{}
			for _, format := range []Format{SF, SFGzip, JSON, DOCX} {
				t.Run(string(format)+link+destination, func(t *testing.T) {
					var buf bytes.Buffer
					if err := Write(context.Background(), &buf, d, format, WriteOptions{AllowLossy: true}); err != nil {
						t.Fatal(err)
					}
					got, err := Read(context.Background(), &buf, format, ReadOptions{PDFSkipStructure: true})
					if err != nil {
						t.Fatal(err)
					}
					for i, n := range d.Notes {
						if !reflect.DeepEqual(n.BodyLabelStyle, got.Notes[i].BodyLabelStyle) {
							t.Fatalf("style %d: want %+v, got %+v", i, n.BodyLabelStyle, got.Notes[i].BodyLabelStyle)
						}
					}
				})
			}
			var pdf bytes.Buffer
			if err := Write(context.Background(), &pdf, d, PDF, WriteOptions{}); err != nil {
				t.Fatal(err)
			}
			got, err := ReadPDF(context.Background(), &pdf, ReadOptions{PDFSkipStructure: true})
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(got.Text(), "§E") {
				t.Fatal("missing styled body mark")
			}
		}
	}
}

func TestNoteBodyLabelStyleValidation(t *testing.T) {
	for _, style := range []Run{{Text: "hidden"}, {Anchor: "hidden"}, {Note: &NoteReference{ID: "a", Kind: Footnote}}, {Source: &SourceInfo{}}, {FontSize: -1}, {Destination: "missing"}} {
		d := variantNoteDocument()
		d.Notes[0].BodyLabelStyle = &style
		if err := d.Validate(); err == nil {
			t.Fatalf("accepted invalid style %+v", style)
		}
	}
}

func TestMarkerlessStyledNoteBody(t *testing.T) {
	d := variantNoteDocument()
	d.Notes[0].BodyLabel = noteLabel("")
	d.Notes[0].BodyLabelStyle = &Run{Bold: true, FontSize: 9}
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, DOCX, WriteOptions{AllowLossy: true}); err != nil {
		t.Fatal(err)
	}
	got, err := ReadDOCX(context.Background(), &out)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(d.Notes, got.Notes) {
		t.Fatalf("markerless style changed: %+v", got.Notes)
	}
}

func TestStyledNoteBodyRejectsHiddenContent(t *testing.T) {
	d := variantNoteDocument()
	d.Notes[0].BodyLabelStyle = &Run{Bold: true, FontSize: 9, Link: "https://example.org"}
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, DOCX, WriteOptions{AllowLossy: true}); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(string) string{
		func(s string) string { return strings.Replace(s, ">§E </w:t>", ">WRONG </w:t>", 1) },
		func(s string) string { return strings.Replace(s, ">§E </w:t>", ">§E </w:t><w:drawing/>", 1) },
		func(s string) string { return strings.Replace(s, "<w:b/>", "<w:vanish/>", 1) },
		func(s string) string { return strings.Replace(s, `md:noteBodyStyle="1"`, `md:noteBodyStyle="0"`, 1) },
		func(s string) string {
			return strings.Replace(s, `md:noteBodySeparator="1"`, `md:noteBodySeparator="0"`, 1)
		},
		func(s string) string { return strings.Replace(s, `<w:tab/>`, `<w:t>HIDDEN</w:t>`, 1) },
		func(s string) string { return strings.Replace(s, `<w:b/>`, `<w:b><w:t>HIDDEN</w:t></w:b>`, 1) },
	} {
		zr, err := zip.NewReader(bytes.NewReader(out.Bytes()), int64(out.Len()))
		if err != nil {
			t.Fatal(err)
		}
		var bad bytes.Buffer
		zw := zip.NewWriter(&bad)
		changed := false
		for _, f := range zr.File {
			rc, err := f.Open()
			if err != nil {
				t.Fatal(err)
			}
			data, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				t.Fatal(err)
			}
			if f.Name == "word/endnotes.xml" {
				next := []byte(mutate(string(data)))
				changed = !bytes.Equal(next, data)
				data = next
			}
			w, err := zw.Create(f.Name)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = w.Write(data); err != nil {
				t.Fatal(err)
			}
		}
		if err := zw.Close(); err != nil {
			t.Fatal(err)
		}
		if !changed {
			t.Fatal("mutation missed styled body")
		}
		if _, err := ReadDOCX(context.Background(), &bad); err == nil {
			t.Fatal("metadata hid contradictory content")
		}
	}
}

func TestTallNoteBodyLabelReservesLayout(t *testing.T) {
	for _, kind := range []string{Footnote, Endnote} {
		d := &Document{Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "CITATION"}, {Note: &NoteReference{ID: "large", Kind: kind}}}}}, Notes: []Note{{ID: "large", Kind: kind, Label: "BIG", BodyLabelStyle: &Run{FontSize: 42}, Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "BODY_AFTER_LABEL"}}}}}}}
		d.Blocks[0].Runs = append(d.Blocks[0].Runs, Run{Note: &NoteReference{ID: "second", Kind: kind}})
		d.Notes = append(d.Notes, Note{ID: "second", Kind: kind, Label: "SMALL", Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "SIBLING_NOTE_BODY"}}}}})
		var out bytes.Buffer
		if err := Write(context.Background(), &out, d, PDF, WriteOptions{}); err != nil {
			t.Fatal(err)
		}
		got, err := ReadPDF(context.Background(), &out, ReadOptions{PDFSkipStructure: true})
		if err != nil {
			t.Fatal(err)
		}
		if strings.Count(got.Text(), "BIG") != 2 || !strings.Contains(got.Text(), "BODY_AFTER_LABEL") {
			t.Fatalf("large %s marker lost: %q", kind, got.Text())
		}
		var label, body, sibling *SourceInfo
		_ = walkNoteRuns(got.Blocks, func(run Run) error {
			if run.Text == "BIG" && run.FontSize >= 40 {
				label = run.Source
			}
			if strings.Contains(run.Text, "BODY_AFTER_LABEL") {
				body = run.Source
			}
			if strings.Contains(run.Text, "SIBLING_NOTE_BODY") {
				sibling = run.Source
			}
			return nil
		})
		if label == nil || body == nil || sibling == nil || label.Quad == nil || body.Quad == nil || sibling.Quad == nil {
			t.Fatal("missing independently recoverable label/body/sibling geometry")
		}
		_, ly0, lx1, ly1 := quadBounds(*label.Quad)
		bx0, by0, _, by1 := quadBounds(*body.Quad)
		_, sy0, _, _ := quadBounds(*sibling.Quad)
		if label.Page == body.Page && lx1 > bx0 && ly0 < by1 && by0 < ly1 {
			t.Fatal("tall label overlaps its body")
		}
		if label.Page == sibling.Page && sy0 < ly1 {
			t.Fatal("sibling note starts before tall marker ends")
		}

	}
}
func TestCustomNoteLabelsAcrossFormats(t *testing.T) {
	d := noteDocument()
	d.Notes[0].Label = "§E"
	d.Notes[1].Label = "*α"
	catalog, err := catalogNotes(d, true)
	if err != nil {
		t.Fatal(err)
	}
	if catalog.labels[noteKey{Footnote, "shared"}] != "1" || catalog.labels[noteKey{Footnote, "alpha &<42>"}] != "*α" {
		t.Fatal("custom note advanced automatic counter", catalog.labels)
	}
	for _, format := range []Format{SF, SFGzip, JSON, DOCX} {
		t.Run(string(format), func(t *testing.T) {
			var out bytes.Buffer
			if err := Write(context.Background(), &out, d, format, WriteOptions{}); err != nil {
				t.Fatal(err)
			}
			got, err := Read(context.Background(), &out, format, ReadOptions{PDFSkipStructure: true})
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(d.Notes, got.Notes) || !reflect.DeepEqual(d.Blocks, got.Blocks) || len(got.Diagnostics) != 0 {
				t.Fatalf("custom labels changed: %#v %#v %v", got.Notes, got.Blocks, got.Diagnostics)
			}
		})
	}
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, PDF, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	got, err := ReadPDF(context.Background(), &out, ReadOptions{PDFSkipStructure: true})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(got.Text(), "*α") != 3 || strings.Count(got.Text(), "§E") != 2 {
		t.Fatalf("custom body/reference marks missing or duplicated: %q", got.Text())
	}
	for _, label := range []string{"\t", "\n", "\r", " ", "a\nb", "\x00"} {
		d.Notes[0].Label = label
		if err := d.Validate(); err == nil {
			t.Fatalf("invalid label %q accepted", label)
		}
	}
}

func TestDOCXCustomNoteMarkBoundary(t *testing.T) {
	body := `<w:p><w:r><w:t>Before</w:t></w:r><w:r><w:footnoteReference w:id="7" w:customMarkFollows="1"/><w:t>*</w:t></w:r><w:r><w:t>After</w:t></w:r></w:p>`
	d, err := ReadDOCX(context.Background(), bytes.NewReader(notesDOCX(t, body, rawFootnotes, "")))
	if err != nil {
		t.Fatal(err)
	}
	if d.Notes[0].Label != "*" || !reflect.DeepEqual(d.Blocks[0].Runs, []Run{{Text: "Before"}, noteRun(Footnote, "7"), {Text: "After"}}) {
		t.Fatal("custom mark flattened or surrounding text changed", d)
	}
	for _, bad := range []string{strings.Replace(body, `<w:t>*</w:t>`, "", 1), strings.Replace(body, `<w:t>*</w:t>`, `<w:t></w:t>`, 1)} {
		if _, err := ReadDOCX(context.Background(), bytes.NewReader(notesDOCX(t, bad, rawFootnotes, ""))); err == nil {
			t.Fatal("missing custom mark accepted")
		}
	}
}

func TestDOCXCustomNoteMetadataChecked(t *testing.T) {
	d := noteDocument()
	d.Notes[1].Label = "*α"
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, DOCX, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	bad := rewriteListDOCX(t, out.Bytes(), func(s string) string {
		return strings.ReplaceAll(s, `<w:t xml:space="preserve">*</w:t>`, `<w:t xml:space="preserve">wrong</w:t>`)
	})
	if _, err := ReadDOCX(context.Background(), bytes.NewReader(bad)); err == nil {
		t.Fatal("contradictory literal label metadata accepted")
	}
	body := `<w:p><w:r><w:footnoteReference w:id="7" w:customMarkFollows="1"/><w:t>*</w:t><w:t>α</w:t></w:r><w:r><w:t>BODY</w:t></w:r></w:p>`
	got, err := ReadDOCX(context.Background(), bytes.NewReader(notesDOCX(t, body, rawFootnotes, "")))
	if err != nil || got.Notes[0].Label != "*α" || got.Blocks[0].Runs[1].Text != "BODY" {
		t.Fatal("split text nodes lost", got, err)
	}
	for _, bad := range []string{strings.Replace(body, `customMarkFollows="1"`, `customMarkFollows="invalid"`, 1), strings.Replace(body, `<w:t>α</w:t>`, `<w:tab/>`, 1)} {
		if _, err := ReadDOCX(context.Background(), bytes.NewReader(notesDOCX(t, bad, rawFootnotes, ""))); err == nil {
			t.Fatal("malformed custom mark accepted")
		}
	}
}
func richBodyNoteDocument(sectioned bool) *Document {
	d := variantNoteDocument()
	d.Blocks[0].Anchors = []string{"return"}
	for i := range d.Notes {
		d.Notes[i].BodyLabel = nil
		d.Notes[i].BodyLabelContent = []Run{{Text: "N", Bold: true, FontFamily: "Go", FontSize: 9, Color: "#203040", Destination: "return", Link: "#return"}, {Text: "-", Italic: true, FontFamily: "Go", FontSize: 8}, {Text: "one", Underline: true, FontFamily: "Go", FontSize: 10, Link: "https://example.org/note"}}
	}
	if sectioned {
		d.Blocks = []Block{{Kind: SectionBlock, Section: &Section{Blocks: d.Blocks}}}
	}
	return d
}
func TestRichBodyLabelsAllPaths(t *testing.T) {
	for _, sectioned := range []bool{false, true} {
		d := richBodyNoteDocument(sectioned)
		before, err := ToStructured(d)
		if err != nil {
			t.Fatal(err)
		}
		for _, format := range []Format{SF, SFGzip, JSON, DOCX} {
			var out bytes.Buffer
			if err := Write(context.Background(), &out, d, format, WriteOptions{AllowLossy: true}); err != nil {
				t.Fatal(sectioned, format, err)
			}
			got, err := Read(context.Background(), &out, format, ReadOptions{PDFSkipStructure: true})
			if err != nil {
				t.Fatal(sectioned, format, err)
			}
			after, err := ToStructured(got)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before.Content, after.Content) || !reflect.DeepEqual(before.Attrs.Notes, after.Attrs.Notes) {
				t.Fatalf("rich body labels changed through %s, sectioned %v\nwant %+v\ngot %+v", format, sectioned, d.Notes, got.Notes)
			}
		}
		var out bytes.Buffer
		if err := Write(context.Background(), &out, d, PDF, WriteOptions{}); err != nil {
			t.Fatal(err)
		}
		got, err := ReadPDF(context.Background(), &out, ReadOptions{PDFSkipStructure: true})
		if err != nil {
			t.Fatal(err)
		}
		if strings.Count(got.Text(), "N-one") != 3 {
			t.Fatal("lost or duplicated rich labels", got.Text())
		}
	}
}
func TestRichBodyLabelValidation(t *testing.T) {
	for _, change := range []func(*Note){
		func(n *Note) { n.BodyLabel = noteLabel("duplicate") },
		func(n *Note) { n.BodyLabelStyle = &Run{Bold: true} },
		func(n *Note) { n.BodyLabelContent = []Run{} },
		func(n *Note) { n.BodyLabelContent = []Run{{Text: ""}} },
		func(n *Note) { n.BodyLabelContent = []Run{{Text: "bad\nlabel"}} },
		func(n *Note) { n.BodyLabelContent = []Run{{Text: " "}} },
		func(n *Note) { n.BodyLabelContent = []Run{{Text: "label", Anchor: "bad"}} },
		func(n *Note) {
			n.BodyLabelContent = []Run{{Text: "label", Note: &NoteReference{ID: "shared", Kind: Endnote}}}
		},
		func(n *Note) { n.BodyLabelContent = []Run{{Text: "label", Destination: "missing"}} },
	} {
		d := richBodyNoteDocument(false)
		change(&d.Notes[0])
		if err := d.Validate(); err == nil {
			t.Fatal("invalid rich body label accepted")
		}
	}
}

func richLabelNoteObjects() []string {
	o := explicitNoteObjects("FENote /A << /O /FENote /NoteType /Footnote >>")
	content := strings.SplitN(strings.SplitN(o[4], "stream\n", 2)[1], "\nendstream", 2)[0]
	mixed := "q BT /F 10 Tf /P << /MCID 2 >> BDC 1 0 0 1 20 80 Tm (*) Tj /F 8 Tf 1 0 0 rg (1) Tj EMC ET Q\n"
	o[4] = recoveryStream("", strings.Replace(content, markedText(2, 20, 80, "*1"), mixed, 1))
	return o
}
func TestPDFRichBodyLabelRetainsIndividualStyles(t *testing.T) {
	d := readRecovery(t, recoveryPDF(richLabelNoteObjects()...))
	if len(d.Notes) != 1 {
		t.Fatal("missing note")
	}
	n := d.Notes[0]
	runs := n.BodyLabelContent
	if n.BodyLabel != nil || n.BodyLabelStyle != nil || len(runs) != 2 || noteLabelContentText(runs) != "*1" {
		t.Fatalf("body label flattened or duplicated: %+v", n)
	}
	if runs[0].FontSize != 10 || runs[1].FontSize != 8 || runs[0].Color != "#000000" || runs[1].Color != "#FF0000" || !runs[0].Bold || !runs[1].Bold {
		t.Fatalf("individual label formatting lost: %+v", runs)
	}
	if n.LabelSource == nil || runs[0].Source == nil || runs[1].Source == nil || runs[0].Source.Quad == nil || runs[1].Source.Quad == nil || runs[0].Source.Element != "(11 0 R)" {
		t.Fatal("independent label-run evidence lost", runs)
	}
	assertRecoverySF(t, d)
	for _, format := range []Format{SF, SFGzip, JSON} {
		var out bytes.Buffer
		if err := Write(context.Background(), &out, d, format, WriteOptions{}); err != nil {
			t.Fatal(err)
		}
		got, err := Read(context.Background(), &out, format, ReadOptions{PDFSkipStructure: true})
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(d.Notes, got.Notes) {
			t.Fatal("rich label provenance lost", format)
		}
	}
}

func rewriteRichNoteParts(t *testing.T, source []byte, change func(string) string) []byte {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(source), int64(len(source)))
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	zw := zip.NewWriter(&out)
	changed := false
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		if f.Name == "word/endnotes.xml" || f.Name == "word/footnotes.xml" {
			next := []byte(change(string(data)))
			changed = changed || !bytes.Equal(next, data)
			data = next
		}
		w, err := zw.Create(f.Name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = w.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("rich label mutation missed")
	}
	return out.Bytes()
}
func TestRichBodyLabelWordRolesAndEditing(t *testing.T) {
	d := richBodyNoteDocument(false)
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, DOCX, WriteOptions{AllowLossy: true}); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(string) string{
		func(s string) string {
			return strings.Replace(s, `md:noteBodyContent="1"`, `md:noteBodyContent="2"`, 1)
		},
		func(s string) string { return strings.Replace(s, `mdnoterich1_endnote_1`, `mdnoterich1_footnote_1`, 1) },
		func(s string) string {
			return strings.Replace(s, `md:noteBodyContent="1"/>`, `md:noteBodyContent="1"/><w:r><w:drawing/></w:r>`, 1)
		},
		func(s string) string { return strings.Replace(s, `<w:b/>`, `<w:vanish/>`, 1) },
		func(s string) string {
			return strings.Replace(s, `md:noteBodyContent="1"/>`, `md:noteBodyContent="1"/><w:r md:noteBodyStyle="1"><w:t>OTHER</w:t></w:r>`, 1)
		},
		func(s string) string {
			return strings.Replace(s, `md:noteBodyContent="1"/>`, `md:noteBodyContent="1"/><w:sdt><w:sdtContent><w:r><w:t>OTHER</w:t></w:r></w:sdtContent></w:sdt>`, 1)
		},
		func(s string) string {
			return strings.Replace(s, `<w:t xml:space="preserve">N</w:t>`, `<w:t xml:space="preserve">N</w:t><w:br/>`, 1)
		},
	} {
		bad := rewriteRichNoteParts(t, out.Bytes(), change)
		if _, err := ReadDOCX(context.Background(), bytes.NewReader(bad)); err == nil {
			t.Fatal("metadata swallowed unsupported label content")
		}
	}
	edited := rewriteRichNoteParts(t, out.Bytes(), func(s string) string {
		return strings.ReplaceAll(s, `<w:t xml:space="preserve">N</w:t>`, `<w:t xml:space="preserve">RICH</w:t>`)
	})
	got, err := ReadDOCX(context.Background(), bytes.NewReader(edited))
	if err != nil {
		t.Fatal(err)
	}
	if noteLabelContentText(got.Notes[0].BodyLabelContent) != "RICH-one" || got.Notes[0].Label != d.Notes[0].Label {
		t.Fatal("actual editable text was overridden by duplicate metadata")
	}
}

func TestRichBodyLabelMixedDirectionFitsOwnGutter(t *testing.T) {
	data, err := os.ReadFile("testdata/assets/DejaVuSans.ttf")
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{Footnote, Endnote} {
		for _, rtlBody := range []bool{false, true} {
			d := &Document{Layout: Layout{Width: 420, Height: 420, Margin: 36}, Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "Citation"}, {Note: &NoteReference{Kind: kind, ID: "mixed"}}}}}, Notes: []Note{{ID: "mixed", Kind: kind, Label: "1", BodyLabelContent: []Run{{Text: "ب", FontFamily: "DejaVu", FontSize: 10}, {Text: "A", FontFamily: "DejaVu", FontSize: 9, Color: "#FF0000"}}, Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "BODY_TEXT"}}}}}}}
			if rtlBody {
				d.Notes[0].Blocks[0].Direction = "rtl"
			}
			var out bytes.Buffer
			if err := Write(context.Background(), &out, d, PDF, WriteOptions{PDFFonts: []FontFace{{Family: "DejaVu", Data: data}}}); err != nil {
				t.Fatal(err)
			}
			got, err := ReadPDF(context.Background(), &out, ReadOptions{PDFSkipStructure: true})
			if err != nil {
				t.Fatal(err)
			}
			found := 0
			_ = walkNoteRuns(got.Blocks, func(run Run) error {
				if strings.Contains(run.FontFamily, "DejaVu") && run.Source != nil && run.Source.Quad != nil {
					found++
					x0, _, x1, _ := quadBounds(*run.Source.Quad)
					if (!rtlBody && (x0 < 35.99 || x1 > 70)) || (rtlBody && (x0 < 350 || x1 > 384.01)) {
						t.Fatalf("label escaped its own gutter: %s rtl %v bounds %v %v", kind, rtlBody, x0, x1)
					}
				}
				return nil
			})
			if found == 0 {
				t.Fatal("mixed label did not produce independently recoverable glyphs")
			}
		}
	}
}

func TestRichBodyLabelReservedDestinationAndSFNodes(t *testing.T) {
	d := richBodyNoteDocument(false)
	id := "mdnoterich1_endnote_1"
	d.Blocks[0].Anchors = append(d.Blocks[0].Anchors, id)
	d.Notes[0].BodyLabelContent[0].Destination = id
	d.Notes[0].BodyLabelContent[0].Link = "#" + id
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, DOCX, WriteOptions{AllowLossy: true}); err != nil {
		t.Fatal(err)
	}
	got, err := ReadDOCX(context.Background(), &out)
	if err != nil {
		t.Fatal(err)
	}
	if got.Notes[0].BodyLabelContent[0].Destination != id || !reflect.DeepEqual(got.Blocks, d.Blocks) {
		t.Fatal("private range stole an authored destination")
	}
	sf, err := ToStructured(d)
	if err != nil {
		t.Fatal(err)
	}
	sf.Attrs.Notes[0].BodyLabelContent = []StructuredNode{{Type: "note_reference", Attrs: sfAttrs(map[string]any{"kind": Endnote, "id": "shared"})}}
	if _, err := FromStructured(sf); err == nil {
		t.Fatal("nested SF atom accepted in rich label")
	}
	sf.Attrs.Notes[0].BodyLabelContent = []StructuredNode{}
	if _, err := FromStructured(sf); err == nil {
		t.Fatal("empty rich SF label accepted")
	}
}

func TestRichBodyLabelWordRevisionsAndComments(t *testing.T) {
	d := richBodyNoteDocument(false)
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, DOCX, WriteOptions{AllowLossy: true}); err != nil {
		t.Fatal(err)
	}
	edited := rewriteRichNoteParts(t, out.Bytes(), func(s string) string {
		start := strings.Index(s, `md:noteBodyContent="1"/>`) + len(`md:noteBodyContent="1"/>`)
		if start < len(`md:noteBodyContent="1"/>`) {
			return s
		}
		end := start + strings.Index(s[start:], `</w:hyperlink>`) + len(`</w:hyperlink>`)
		fragment := s[start:end]
		history := `<w:del w:id="101"><w:r><w:delText>DELETED</w:delText></w:r></w:del><w:commentRangeStart w:id="200"/><w:ins w:id="102">` + fragment + `</w:ins><w:commentRangeEnd w:id="200"/><w:r><w:commentReference w:id="200"/></w:r>`
		return s[:start] + history + s[end:]
	})
	got, err := ReadDOCX(context.Background(), bytes.NewReader(edited))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(d.Notes, got.Notes) || !hasDiagnostic(got, "revision_history") || !hasDiagnostic(got, "comments") {
		t.Fatal("final-visible rich label revision/comment policy changed")
	}
	deleted := rewriteRichNoteParts(t, out.Bytes(), func(s string) string {
		start := strings.Index(s, `md:noteBodyContent="1"/>`) + len(`md:noteBodyContent="1"/>`)
		if start < len(`md:noteBodyContent="1"/>`) {
			return s
		}
		end := start + strings.Index(s[start:], `<w:bookmarkEnd`)
		history := strings.ReplaceAll(strings.ReplaceAll(s[start:end], `<w:t `, `<w:delText `), `</w:t>`, `</w:delText>`)
		return s[:start] + `<w:del w:id="301">` + history + `</w:del>` + s[end:]
	})
	empty, err := ReadDOCX(context.Background(), bytes.NewReader(deleted))
	if err != nil {
		t.Fatal(err)
	}
	for i, note := range empty.Notes {
		if i < 2 && (note.BodyLabel == nil || *note.BodyLabel != "" || note.BodyLabelContent != nil) {
			t.Fatal("deleted mark became inherited/invented numbering")
		}
		if i == 2 && !reflect.DeepEqual(note.BodyLabelContent, d.Notes[i].BodyLabelContent) {
			t.Fatal("markerless state leaked into next note")
		}
	}
	if !hasDiagnostic(empty, "docx_rich_label_empty") {
		t.Fatal("empty rich mark normalization not disclosed")
	}
	blank := rewriteRichNoteParts(t, out.Bytes(), func(s string) string {
		start := strings.Index(s, `md:noteBodyContent="1"/>`) + len(`md:noteBodyContent="1"/>`)
		if start < len(`md:noteBodyContent="1"/>`) {
			return s
		}
		end := start + strings.Index(s[start:], `<w:bookmarkEnd`)
		return s[:start] + `<w:r><w:rPr><w:b/></w:rPr><w:t/></w:r>` + s[end:]
	})
	none, err := ReadDOCX(context.Background(), bytes.NewReader(blank))
	if err != nil {
		t.Fatal(err)
	}
	if none.Notes[0].BodyLabel == nil || *none.Notes[0].BodyLabel != "" {
		t.Fatal("empty Word text became inherited numbering")
	}
}
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
			got, err := Read(context.Background(), &buf, format, ReadOptions{PDFSkipStructure: true})
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
	got, err := ReadPDF(context.Background(), &pdf, ReadOptions{PDFSkipStructure: true})
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
			got, err := ReadPDF(context.Background(), &buf, ReadOptions{PDFSkipStructure: true})
			if err != nil || !strings.Contains(got.Text(), "FIRST_NOTE_BODY") {
				t.Fatal("markerless site lost body", err)
			}
			continue
		}
		got, err := Read(context.Background(), &buf, format, ReadOptions{PDFSkipStructure: true})
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
func noteRun(kind, id string) Run { return Run{Note: &NoteReference{Kind: kind, ID: id}} }
func noteDocument() *Document {
	return &Document{Layout: Layout{Width: 420, Height: 420, Margin: 36}, Notes: []Note{
		{Kind: Endnote, ID: "shared", Blocks: []Block{mergeParagraph("END_NOTE_BODY")}},
		{Kind: Footnote, ID: "alpha &<42>", Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "FIRST_NOTE_BODY ", Bold: true}, {Text: "note link", Link: "https://example.invalid/note"}}}, {Kind: ListBlock, List: "bullet", Items: []ListItem{listItem("Note list item")}}}},
		{Kind: Footnote, ID: "shared", Blocks: []Block{mergeParagraph("SECOND_NOTE_BODY")}},
	}, Blocks: []Block{
		{Kind: Paragraph, Runs: []Run{{Text: "First reference"}, noteRun(Footnote, "alpha &<42>"), {Text: " and an endnote"}, noteRun(Endnote, "shared"), {Text: "."}}},
		{Kind: ListBlock, List: "number", Start: listStartPointer(3), Items: []ListItem{{Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "List reference"}, noteRun(Footnote, "shared")}}}}}},
		{Kind: Paragraph, Runs: []Run{{Text: "Repeated reference"}, noteRun(Footnote, "alpha &<42>")}},
	}}
}
func TestNoteIdentitySFAndDOCX(t *testing.T) {
	ctx := context.Background()
	d := noteDocument()
	s, err := ToStructured(d)
	if err != nil {
		t.Fatal(err)
	}
	if s.Content[0].Content[1].Type != "note_reference" || s.Content[0].Content[1].Text != "" {
		t.Fatal("reference flattened into text")
	}
	shared, err := FromStructured(s)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(d, shared) {
		t.Fatal("SF note identities, ordering, reference sites or body semantics changed")
	}
	for _, format := range []Format{SF, JSON, DOCX} {
		t.Run(string(format), func(t *testing.T) {
			var out bytes.Buffer
			if err := Write(ctx, &out, shared, format, WriteOptions{}); err != nil {
				t.Fatal(err)
			}
			decoded, err := Read(ctx, &out, format, ReadOptions{})
			if err != nil {
				t.Fatal(err)
			}
			got, err := ToStructured(decoded)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(s.Content, got.Content) || !reflect.DeepEqual(s.Attrs.Notes, got.Attrs.Notes) {
				a, _ := json.Marshal(s)
				b, _ := json.Marshal(got)
				t.Fatalf("note round trip changed\nwant %s\ngot %s", a, b)
			}
			if len(decoded.Diagnostics) != 0 {
				t.Fatalf("own note output diagnosed as lossy: %#v", decoded.Diagnostics)
			}
		})
	}
}
func TestNoteValidationAndOrphans(t *testing.T) {
	for _, d := range []*Document{
		{Blocks: []Block{{Kind: Paragraph, Runs: []Run{noteRun(Footnote, "missing")}}}},
		{Notes: []Note{{Kind: Footnote, ID: "same"}, {Kind: Footnote, ID: "same"}}},
		{Notes: []Note{{Kind: "unknown", ID: "x"}}},
		{Notes: []Note{{Kind: Footnote}}},
		{Notes: []Note{{Kind: Footnote, ID: "x"}}, Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "ambiguous", Note: &NoteReference{ID: "x", Kind: Footnote}}}}}},
		{Notes: []Note{{Kind: Footnote, ID: "x", Blocks: []Block{{Kind: Paragraph, Runs: []Run{noteRun(Footnote, "x")}}}}}},
		{Notes: []Note{{Kind: Footnote, ID: "x"}}, Headers: []Block{{Kind: Paragraph, Runs: []Run{noteRun(Footnote, "x")}}}},
	} {
		if err := d.Validate(); err == nil {
			t.Fatal("invalid note graph accepted")
		}
	}
	orphan := &Document{Notes: []Note{{Kind: Footnote, ID: "draft", Blocks: []Block{mergeParagraph("draft body")}}}}
	if _, err := ToStructured(orphan); err != nil {
		t.Fatal("draft notes must persist without an anchor", err)
	}
	for _, format := range []Format{DOCX, PDF} {
		if err := Write(context.Background(), &bytes.Buffer{}, orphan, format, WriteOptions{AllowLossy: true}); !errors.Is(err, ErrUnsupported) {
			t.Fatalf("unanchored note silently hidden in %s: %v", format, err)
		}
	}
}
func notesDOCX(t testing.TB, body, footnotes, endnotes string) []byte {
	t.Helper()
	source := docxWithBody(t, body)
	zr, err := zip.NewReader(bytes.NewReader(source), int64(len(source)))
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	z := zip.NewWriter(&out)
	for _, f := range zr.File {
		rc, _ := f.Open()
		b, _ := io.ReadAll(rc)
		rc.Close()
		w, _ := z.Create(f.Name)
		w.Write(b)
	}
	parts := map[string]string{"word/_rels/document.xml.rels": `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="fn" Type="` + relNS + `/footnotes" Target="footnotes.xml"/><Relationship Id="en" Type="` + relNS + `/endnotes" Target="endnotes.xml"/></Relationships>`, "word/footnotes.xml": `<w:footnotes xmlns:w="` + wordNS + `">` + footnotes + `</w:footnotes>`, "word/endnotes.xml": `<w:endnotes xmlns:w="` + wordNS + `">` + endnotes + `</w:endnotes>`}
	for name, data := range parts {
		w, err := z.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = w.Write([]byte(data)); err != nil {
			t.Fatal(err)
		}
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

const rawNoteBody = `<w:p><w:r><w:t>Before</w:t></w:r><w:r><w:footnoteReference w:id="7"/></w:r><w:r><w:t>Between</w:t></w:r><w:r><w:endnoteReference w:id="7"/></w:r><w:r><w:t>After</w:t></w:r></w:p>`
const rawFootnotes = `<w:footnote w:type="separator" w:id="-1"><w:p><w:r><w:separator/></w:r></w:p></w:footnote><w:footnote w:id="7"><w:p><w:r><w:footnoteRef/><w:tab/></w:r><w:r><w:rPr><w:b/></w:rPr><w:t>Footnote text</w:t></w:r></w:p></w:footnote>`
const rawEndnotes = `<w:endnote w:id="7"><w:p><w:r><w:endnoteRef/><w:tab/></w:r><w:r><w:t>Endnote text</w:t></w:r></w:p></w:endnote>`

func TestDOCXNotesPublishedStructure(t *testing.T) {

	d, err := ReadDOCX(context.Background(), bytes.NewReader(notesDOCX(t, rawNoteBody, rawFootnotes, rawEndnotes)))
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Notes) != 2 || d.Notes[0].ID != "7" || d.Notes[0].Kind != Footnote || d.Notes[1].Kind != Endnote {
		t.Fatal("note identities or separators misread")
	}
	want := []Run{{Text: "Before"}, noteRun(Footnote, "7"), {Text: "Between"}, noteRun(Endnote, "7"), {Text: "After"}}
	if !reflect.DeepEqual(d.Blocks[0].Runs, want) {
		t.Fatal("reference sites or surrounding content changed")
	}
	if d.Notes[0].Blocks[0].Runs[0].Text != "Footnote text" || d.Notes[1].Blocks[0].Runs[0].Text != "Endnote text" {
		t.Fatal("note body lost or automatic marker duplicated")
	}
	for _, bad := range []struct{ body, foot string }{{rawNoteBody, ""}, {strings.Replace(rawNoteBody, `w:id="7"`, `w:id="999"`, 1), rawFootnotes}, {rawNoteBody, rawFootnotes + rawFootnotes}, {rawNoteBody, strings.Replace(rawFootnotes, `w:id="7"`, `w:id="-2"`, 1)}} {
		if _, err := ReadDOCX(context.Background(), bytes.NewReader(notesDOCX(t, bad.body, bad.foot, rawEndnotes))); err == nil {
			t.Fatal("malformed note relationship/identity accepted")
		}
	}
}
func TestDOCXRichNoteAssetsAndPartRelationships(t *testing.T) {
	d := noteDocument()
	imageDoc := readFixture(t, "docx_image.docx")
	for _, b := range imageDoc.Blocks {
		if b.Kind == Picture {
			d.Notes[0].Blocks = append([]Block{b}, d.Notes[0].Blocks...)
			break
		}
	}
	d.Notes[0].Blocks = append(d.Notes[0].Blocks, Block{Kind: Table, Rows: []Row{{Cells: []Cell{{ColSpan: 1, Blocks: []Block{mergeParagraph("Note cell")}}}}}})
	expected, err := ToStructured(d)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, DOCX, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	decoded, err := ReadDOCX(context.Background(), &out)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := ToStructured(decoded)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(expected.Attrs.Notes, actual.Attrs.Notes) {
		a, _ := json.Marshal(expected.Attrs.Notes)
		b, _ := json.Marshal(actual.Attrs.Notes)
		t.Fatalf("rich note assets/structure changed\n%s\n%s", a, b)
	}
}
func TestPDFNotesPlacementAndContinuation(t *testing.T) {
	ctx := context.Background()
	d := noteDocument()
	var out bytes.Buffer
	if err := Write(ctx, &out, d, PDF, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	got, err := ReadPDF(ctx, bytes.NewReader(out.Bytes()), ReadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{"FIRST_NOTE_BODY", "SECOND_NOTE_BODY", "END_NOTE_BODY", "Note list item", "note link"} {
		if strings.Count(got.Text(), marker) != 1 {
			t.Fatalf("missing or repeated note body %s: %s", marker, got.Text())
		}
	}
	if got.Metadata.PageCount != 2 {
		t.Fatalf("endnotes should occupy their section, got %d pages", got.Metadata.PageCount)
	}
	long := &Document{Layout: Layout{Width: 260, Height: 160, Margin: 20}, Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "Long note anchor"}, noteRun(Footnote, "long")}}}, Notes: []Note{{Kind: Footnote, ID: "long", Blocks: []Block{mergeParagraph(strings.Repeat("continued words ", 60) + "LAST_NOTE_MARKER")}}}}
	out.Reset()
	if err := Write(ctx, &out, long, PDF, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	got, err = ReadPDF(ctx, &out, ReadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for _, word := range strings.Fields(strings.ReplaceAll(got.Text(), "Footnotes continued", "")) {
		counts[word]++
	}
	if got.Metadata.PageCount < 2 || counts["continued"] != 60 || counts["words"] != 60 || strings.Count(got.Text(), "LAST_NOTE_MARKER") != 1 {
		t.Fatalf("footnote continuation lost/repeated text: %s", got.Text())
	}
	if !strings.Contains(got.Text(), "Footnotes continued") {
		t.Fatal("footnote continuation not disclosed on continuation pages")
	}
}

func TestNoteReferenceInMergeContinuationSurvives(t *testing.T) {
	body := `<w:tbl><w:tr><w:tc><w:tcPr><w:vMerge w:val="restart"/></w:tcPr><w:p><w:r><w:t>Span origin</w:t></w:r></w:p></w:tc></w:tr><w:tr><w:tc><w:tcPr><w:vMerge/></w:tcPr><w:p><w:r><w:footnoteReference w:id="7"/></w:r></w:p></w:tc></w:tr></w:tbl>`
	d, err := ReadDOCX(context.Background(), bytes.NewReader(notesDOCX(t, body, rawFootnotes, "")))
	if err != nil {
		t.Fatal(err)
	}
	blocks := d.Blocks[0].Rows[0].Cells[0].Blocks
	if len(blocks) != 2 || len(blocks[1].Runs) != 1 || blocks[1].Runs[0].Note == nil || blocks[1].Runs[0].Note.ID != "7" {
		t.Fatal("zero-text reference atom discarded in continuation")
	}
	found := false
	for _, diag := range d.Diagnostics {
		if diag.Code == "merge_continuation_content" {
			found = true
		}
	}
	if !found {
		t.Fatal("reference placement normalization not diagnosed")
	}
}
func TestRepeatedNoteReferenceUsesCachedNOTEREF(t *testing.T) {
	var out bytes.Buffer
	if err := Write(context.Background(), &out, noteDocument(), DOCX, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	z, err := zip.NewReader(bytes.NewReader(out.Bytes()), int64(out.Len()))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range z.File {
		if f.Name != "word/document.xml" {
			continue
		}
		rc, _ := f.Open()
		raw, _ := io.ReadAll(rc)
		rc.Close()
		xml := string(raw)
		if strings.Count(xml, `<w:footnoteReference `) != 2 || strings.Count(xml, `NOTEREF mdnote1_footnote_1`) != 1 {
			t.Fatal("repeat reference generated a duplicate note body")
		}
	}
	invalid := rewriteListDOCX(t, out.Bytes(), func(s string) string {
		return strings.Replace(s, `NOTEREF mdnote1_footnote_1`, `NOTEREF mdnote1_footnote_99`, 1)
	})
	if _, err := ReadDOCX(context.Background(), bytes.NewReader(invalid)); err == nil {
		t.Fatal("dangling note cross-reference accepted")
	}
}

func TestPDFNoteDestinationsResolveToNotePages(t *testing.T) {
	ctx := context.Background()
	var out bytes.Buffer
	if err := Write(ctx, &out, noteDocument(), PDF, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	p, err := pdfcpu.ReadWithContext(ctx, bytes.NewReader(out.Bytes()), &model.Configuration{Reader15: true, ValidationMode: model.ValidationStrict, Offline: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := p.EnsurePageCount(); err != nil {
		t.Fatal(err)
	}
	first, firstRef, _, err := p.PageDict(1, false)
	if err != nil {
		t.Fatal(err)
	}
	_, endRef, _, err := p.PageDict(2, false)
	if err != nil {
		t.Fatal(err)
	}
	annotations, err := p.DereferenceArray(first["Annots"])
	if err != nil {
		t.Fatal(err)
	}
	var targets []types.IndirectRef
	for _, a := range annotations {
		dict, err := p.DereferenceDict(a)
		if err != nil {
			t.Fatal(err)
		}
		if dict["Dest"] == nil {
			continue
		}
		dest, err := p.DereferenceArray(dict["Dest"])
		if err != nil {
			t.Fatal(err)
		}
		ref, ok := dest[0].(types.IndirectRef)
		if !ok {
			t.Fatal("note target is not a page reference")
		}
		targets = append(targets, ref)
	}
	expected := []types.IndirectRef{*firstRef, *endRef, *firstRef, *firstRef}
	if !reflect.DeepEqual(targets, expected) {
		t.Fatalf("note links target wrong pages: got %v, want %v", targets, expected)
	}
}

func TestNoteBookmarkBindingAndCachedDisplayDiagnostics(t *testing.T) {
	var out bytes.Buffer
	if err := Write(context.Background(), &out, noteDocument(), DOCX, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	wrong := rewriteListDOCX(t, out.Bytes(), func(s string) string {
		return strings.Replace(s, `w:name="mdnote1_footnote_1"`, `w:name="mdnote1_footnote_2"`, 1)
	})
	if _, err := ReadDOCX(context.Background(), bytes.NewReader(wrong)); err == nil {
		t.Fatal("note bookmark target identity mismatch accepted")
	}
	changed := false
	stale := rewriteListDOCX(t, out.Bytes(), func(s string) string {
		start := strings.Index(s, "<w:fldSimple")
		end := start + strings.Index(s[start:], "</w:fldSimple>")
		field := s[start:end]
		changed = strings.Contains(field, `<w:t>1</w:t>`)
		return s[:start] + strings.Replace(field, `<w:t>1</w:t>`, `<w:t>99</w:t>`, 1) + s[end:]
	})
	if !changed {
		t.Fatal("cached mark not found")
	}
	d, err := ReadDOCX(context.Background(), bytes.NewReader(stale))
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Diagnostics) != 1 || d.Diagnostics[0].Code != "note_cached_mark" {
		t.Fatal("stale cached note mark normalized silently")
	}
	if d.Blocks[2].Runs[1].Note == nil || d.Blocks[2].Runs[1].Note.ID != "alpha &<42>" {
		t.Fatal("cached cross-reference flattened instead of preserving identity")
	}
}

func TestEffectiveNoteSettingsDiagnostics(t *testing.T) {
	defaults, err := parseXML(context.Background(), []byte(`<w:settings xmlns:w="`+wordNS+`"><w:endnotePr><w:numFmt w:val="lowerRoman"/></w:endnotePr></w:settings>`))
	if err != nil {
		t.Fatal(err)
	}
	section, err := parseXML(context.Background(), []byte(`<w:sectPr xmlns:w="`+wordNS+`"><w:endnotePr><w:numFmt w:val="decimal"/></w:endnotePr></w:sectPr>`))
	if err != nil {
		t.Fatal(err)
	}
	r := &docxReader{doc: &Document{Notes: []Note{{Kind: Endnote, ID: "1"}}}, notePropertySources: []*xmlNode{defaults, section}}
	r.checkEffectiveNoteProperties()
	if len(r.doc.Diagnostics) != 0 {
		t.Fatal("overridden note settings diagnosed as a loss")
	}
	r.doc.Diagnostics = nil
	r.notePropertySources = []*xmlNode{defaults}
	r.checkEffectiveNoteProperties()
	if len(r.doc.Diagnostics) != 1 || r.doc.Diagnostics[0].Code != "note_numbering" {
		t.Fatal("inherited note numbering loss not diagnosed")
	}
	r.doc.Diagnostics = nil
	r.notePropertySources = nil
	r.checkEffectiveNoteProperties()
	if len(r.doc.Diagnostics) != 1 || r.doc.Diagnostics[0].Code != "note_default_numbering" {
		t.Fatal("Word/default endnote ambiguity not diagnosed")
	}
	r.doc = &Document{}
	r.notePropertySources = []*xmlNode{defaults}
	r.checkEffectiveNoteProperties()
	if len(r.doc.Diagnostics) != 0 {
		t.Fatal("unused note settings diagnosed as a content loss")
	}
}

func TestCustomNoteSeparatorOmissionIsExplicit(t *testing.T) {
	custom := strings.Replace(rawFootnotes, `<w:separator/>`, `<w:separator/><w:t>Custom separator words</w:t>`, 1)
	d, err := ReadDOCX(context.Background(), bytes.NewReader(notesDOCX(t, rawNoteBody, custom, rawEndnotes)))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, diag := range d.Diagnostics {
		if diag.Code == "note_separator" {
			found = true
		}
	}
	if !found {
		t.Fatal("custom separator visible content silently omitted")
	}
	if err := Write(context.Background(), &bytes.Buffer{}, d, DOCX, WriteOptions{}); !errors.Is(err, ErrLossy) {
		t.Fatal("separator representation loss did not gate output")
	}
}

func TestAdjacentReferencesRemainSeparatePDFAnnotations(t *testing.T) {
	ref := &NoteReference{Kind: Footnote, ID: "same"}
	d := &Document{Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Note: ref}, {Note: ref}}}}, Notes: []Note{{Kind: Footnote, ID: "same", Blocks: []Block{mergeParagraph("one definition")}}}}
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, PDF, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	p, err := pdfcpu.ReadWithContext(context.Background(), bytes.NewReader(out.Bytes()), &model.Configuration{Reader15: true, ValidationMode: model.ValidationStrict, Offline: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := p.EnsurePageCount(); err != nil {
		t.Fatal(err)
	}
	page, _, _, err := p.PageDict(1, false)
	if err != nil {
		t.Fatal(err)
	}
	annotations, err := p.DereferenceArray(page["Annots"])
	if err != nil {
		t.Fatal(err)
	}
	if len(annotations) != 2 {
		t.Fatal("adjacent reference atoms merged into one annotation")
	}
}

func TestDOCXSymbolGlyphIsNotSilentlyDiscarded(t *testing.T) {
	body := `<w:p><w:r><w:t>Before</w:t></w:r><w:r><w:sym w:font="Symbol" w:char="F0B7"/></w:r><w:r><w:t>After</w:t></w:r></w:p>`
	d, err := ReadDOCX(context.Background(), bytes.NewReader(docxWithBody(t, body)))
	if err != nil {
		t.Fatal(err)
	}
	var text strings.Builder
	for _, run := range d.Blocks[0].Runs {
		text.WriteString(run.Text)
	}
	if text.String() != "Before•After" {
		t.Fatalf("mapped symbol glyph lost: %q", text.String())
	}
	unknown := strings.Replace(body, `w:font="Symbol"`, `w:font="UnknownSymbolFont"`, 1)
	if _, err := ReadDOCX(context.Background(), bytes.NewReader(docxWithBody(t, unknown))); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("unknown symbol font guessed: %v", err)
	}
}
