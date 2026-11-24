package manydoc

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"
)

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
