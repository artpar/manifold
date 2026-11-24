package manydoc

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"reflect"
	"strings"
	"testing"
)

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
