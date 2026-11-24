package manydoc

import (
	"bytes"
	"context"
	"reflect"
	"strings"
	"testing"
)

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
			got, err := Read(context.Background(), &out, format, ReadOptions{})
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
	got, err := ReadPDF(context.Background(), &out, ReadOptions{})
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
