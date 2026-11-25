package manifold

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func richUntaggedNoteSource(variant string) []byte {
	separator := "20 215 m 120 215 l S "
	continuation := "BT /B 10 Tf 20 175 Td 0 0 1 rg (CONTINUATION) Tj ET 0 g "
	switch variant {
	case "no-separator":
		separator = ""
	case "column":
		continuation = strings.Replace(continuation, "20 175 Td", "180 175 Td", 1)
	case "gap":
		continuation = strings.Replace(continuation, "20 175 Td", "20 140 Td", 1)
	case "footer":
		continuation = strings.Replace(continuation, "/B 10 Tf", "/B 8 Tf", 1)
	}
	content := "BT /F 11 Tf 20 500 Td (Main text) Tj /F 6 Tf 1 0 0 1 80 504 Tm (1) Tj /F 11 Tf 1 0 0 1 84 500 Tm ( more) Tj ET " + separator +
		"BT /F 6 Tf 20 203 Td (1) Tj /F 10 Tf 1 0 0 1 24 200 Tm ( body note) Tj ET " + continuation +
		"20 100 m 160 100 l 160 150 l 20 150 l h 90 100 m 90 150 l 20 125 m 160 125 l S " +
		"BT /F 10 Tf 25 135 Td (A) Tj 70 0 Td (B) Tj ET BT /F 10 Tf 25 110 Td (C) Tj 70 0 Td (D) Tj ET " +
		"q 30 0 0 20 70 65 cm /Img Do Q BT /F 10 Tf 20 40 Td (TAIL) Tj ET"
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 600] /Resources << /Font << /F 4 0 R /B 10 0 R >> /XObject << /Img 8 0 R >> >> /Contents 5 0 R /Annots [6 0 R 7 0 R 9 0 R] >>",
		"<< /Type /Font /Subtype /Type1 /BaseFont /Courier >>", recoveryStream("", content),
		"<< /Type /Annot /Subtype /Link /Rect [80 499 84 509] /Dest [3 0 R /XYZ 23 206 null] >>",
		"<< /Type /Annot /Subtype /Link /Rect [20 196 24 207] /Dest [3 0 R /XYZ 80 508 null] >>",
		recoveryStream("/Type /XObject /Subtype /Image /Width 2 /Height 1 /ColorSpace /DeviceRGB /BitsPerComponent 8", string([]byte{255, 0, 0, 0, 0, 255})),
		"<< /Type /Annot /Subtype /Link /Rect [19 170 100 186] /A << /S /URI /URI (https://example.test/note-continuation) >> >>",
		"<< /Type /Font /Subtype /Type1 /BaseFont /Courier-Bold >>",
	}
	if variant == "endnote" {
		cut := strings.Index(content, separator)
		main, body := content[:cut], content[cut+len(separator):]
		objects[1] = "<< /Type /Pages /Kids [3 0 R 11 0 R] /Count 2 >>"
		objects[2] = strings.Replace(objects[2], "/Annots [6 0 R 7 0 R 9 0 R]", "/Annots [6 0 R]", 1)
		objects[4] = recoveryStream("", main)
		objects[5] = "<< /Type /Annot /Subtype /Link /Rect [80 499 84 509] /Dest [11 0 R /XYZ 23 506 null] >>"
		objects[6] = "<< /Type /Annot /Subtype /Link /Rect [20 496 24 507] /Dest [3 0 R /XYZ 80 508 null] >>"
		objects[8] = strings.Replace(objects[8], "[19 170 100 186]", "[19 470 100 486]", 1)
		objects = append(objects, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 600] /Resources << /Font << /F 4 0 R /B 10 0 R >> /XObject << /Img 8 0 R >> >> /Contents 12 0 R /Annots [7 0 R 9 0 R] >>", recoveryStream("", "q 1 0 0 1 0 300 cm "+body+" Q"))
	}
	return recoveryPDF(objects...)
}

func assertPDFNoteGlyphInventory(t *testing.T, raw, got *Document) {
	t.Helper()
	type glyph struct {
		text   string
		source SourceInfo
	}
	collect := func(d *Document) map[int]glyph {
		out := map[int]glyph{}
		add := func(text string, s *SourceInfo) {
			if text == "" || s == nil || s.Precision == "synthetic_separator" {
				return
			}
			if _, exists := out[s.Order]; exists {
				t.Fatalf("duplicate painted glyph order %d", s.Order)
			}
			copy := *s
			copy.Structure, copy.Evidence, copy.Confidence = "", "", 0
			out[s.Order] = glyph{text, copy}
		}
		visit := func(run Run) error {
			text := run.Text
			if run.Note != nil {
				for _, note := range d.Notes {
					if note.ID == run.Note.ID {
						text = note.Label
					}
				}
			}
			add(text, run.Source)
			return nil
		}
		_ = walkNoteRuns(d.Blocks, visit)
		for _, note := range d.Notes {
			add(note.Label, note.LabelSource)
			_ = walkNoteRuns(note.Blocks, visit)
		}
		return out
	}
	if !reflect.DeepEqual(collect(raw), collect(got)) {
		t.Fatal("painted text/source inventory changed, duplicated or omitted")
	}
	if !reflect.DeepEqual(raw.PDFNavigation.Links, got.PDFNavigation.Links) {
		t.Fatal("physical links changed")
	}
}

func TestPDFUntaggedRichNoteExtent(t *testing.T) {
	for _, variant := range []string{"", "endnote"} {
		kind := Footnote
		if variant == "endnote" {
			kind = Endnote
		}
		t.Run(kind, func(t *testing.T) {
			source := richUntaggedNoteSource(variant)
			d, err := ReadPDF(context.Background(), bytes.NewReader(source), ReadOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if len(d.Notes) != 1 || len(d.Blocks) != 1 || len(d.Notes[0].Blocks) != 5 || d.Notes[0].Kind != kind {
				t.Fatalf("rich note not grouped: blocks=%+v notes=%+v", d.Blocks, d.Notes)
			}
			bs := d.Notes[0].Blocks
			if bs[0].Kind != Paragraph || layoutText(bs[0]) != " body note" || bs[1].Kind != Paragraph || layoutText(bs[1]) != "CONTINUATION" || !bs[1].Runs[0].Bold || bs[1].Runs[0].Color != "#0000FF" || bs[1].Runs[0].Link != "https://example.test/note-continuation" {
				t.Fatal("rich continuation text/style/URI lost", bs[1])
			}
			if bs[2].Kind != Table || len(bs[2].Rows) != 2 || len(bs[2].Rows[0].Cells) != 2 || layoutText(bs[2].Rows[1].Cells[1].Blocks[0]) != "D" || bs[3].Kind != Picture || bs[4].Kind != Paragraph || layoutText(bs[4]) != "TAIL" {
				t.Fatal("rich body structure lost", bs)
			}
			im, _, err := image.Decode(bytes.NewReader(bs[3].Image.Data))
			if err != nil {
				t.Fatal(err)
			}
			if color.NRGBAModel.Convert(im.At(0, 0)) != (color.NRGBA{255, 0, 0, 255}) || color.NRGBAModel.Convert(im.At(1, 0)) != (color.NRGBA{0, 0, 255, 255}) {
				t.Fatal("source raster pixels changed")
			}
			if d.Notes[0].Source.Evidence != "pdf_note_reciprocal_views;separated_note_zone_continuation" || !hasDiagnostic(d, "pdf_note_continuation_inferred") {
				t.Fatal("continuation provenance missing")
			}
			assertPDFNoteGlyphInventory(t, readRecovery(t, source), d)
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
				if len(got.Notes) != 1 || len(got.Notes[0].Blocks) != 5 || got.Notes[0].Blocks[2].Kind != Table || got.Notes[0].Blocks[3].Kind != Picture || !got.Notes[0].Blocks[1].Runs[0].Bold || got.Notes[0].Blocks[1].Runs[0].Link != "https://example.test/note-continuation" {
					t.Fatal(format, "rich note roundtrip changed", got.Notes)
				}
			}
			if root := os.Getenv("MANYDOC_PDF_NOTE_EXTENT_QA"); root != "" {
				root = filepath.Join(root, kind)
				if err := os.MkdirAll(root, 0755); err != nil {
					t.Fatal(err)
				}
				artifacts := map[string][]byte{"rich.pdf": source}
				for name, doc := range map[string]*Document{"rich": d, "rich-physical": readRecovery(t, source)} {
					data, err := json.MarshalIndent(doc, "", "  ")
					if err != nil {
						t.Fatal(err)
					}
					artifacts[name+".json"] = data
				}
				var sf bytes.Buffer
				if err := Write(context.Background(), &sf, d, SF, WriteOptions{}); err != nil {
					t.Fatal(err)
				}
				artifacts["rich.sf"] = sf.Bytes()
				for name, data := range artifacts {
					if err := os.WriteFile(filepath.Join(root, name), data, 0644); err != nil {
						t.Fatal(err)
					}
				}
			}
		})
	}
}

func TestPDFUntaggedNoteExtentBarriers(t *testing.T) {
	for _, variant := range []string{"no-separator", "column", "gap", "footer"} {
		t.Run(variant, func(t *testing.T) {
			source := richUntaggedNoteSource(variant)
			d, err := ReadPDF(context.Background(), bytes.NewReader(source), ReadOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if len(d.Notes) != 1 || len(d.Notes[0].Blocks) != 1 || !strings.Contains(d.Text(), "CONTINUATION") || len(d.Blocks) <= 1 {
				t.Fatal("unqualified region was consumed", d.Notes, d.Blocks)
			}
			assertPDFNoteGlyphInventory(t, readRecovery(t, source), d)
			assertRecoverySF(t, d)
		})
	}
}

func TestPDFUntaggedMultipleNoteExtents(t *testing.T) {
	content := "BT /F 11 Tf 20 500 Td (Main text) Tj /F 6 Tf 1 0 0 1 80 504 Tm (1) Tj /F 11 Tf 1 0 0 1 84 500 Tm ( more) Tj /F 6 Tf 1 0 0 1 140 504 Tm (2) Tj /F 11 Tf 1 0 0 1 144 500 Tm ( end) Tj ET 20 215 m 120 215 l S " +
		"BT /F 6 Tf 20 203 Td (1) Tj /F 10 Tf 1 0 0 1 24 200 Tm ( FIRST) Tj ET BT /F 10 Tf 20 175 Td (FIRST CONTINUATION) Tj ET " +
		"BT /F 6 Tf 20 143 Td (2) Tj /F 10 Tf 1 0 0 1 24 140 Tm ( SECOND) Tj ET BT /F 10 Tf 20 115 Td (SECOND CONTINUATION) Tj ET"
	source := recoveryPDF("<< /Type /Catalog /Pages 2 0 R >>", "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 600] /Resources << /Font << /F 4 0 R >> >> /Contents 5 0 R /Annots [6 0 R 7 0 R 8 0 R 9 0 R] >>",
		"<< /Type /Font /Subtype /Type1 /BaseFont /Courier >>", recoveryStream("", content),
		"<< /Type /Annot /Subtype /Link /Rect [80 499 84 509] /Dest [3 0 R /XYZ 23 206 null] >>",
		"<< /Type /Annot /Subtype /Link /Rect [20 196 24 207] /Dest [3 0 R /XYZ 80 508 null] >>",
		"<< /Type /Annot /Subtype /Link /Rect [140 499 144 509] /Dest [3 0 R /XYZ 23 146 null] >>",
		"<< /Type /Annot /Subtype /Link /Rect [20 136 24 147] /Dest [3 0 R /XYZ 140 508 null] >>")
	var reference []byte
	for i := 0; i < 5; i++ {
		d, err := ReadPDF(context.Background(), bytes.NewReader(source), ReadOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if len(d.Notes) != 2 || len(d.Blocks) != 1 || len(collectedNoteRefs(d)) != 2 {
			t.Fatal("multiple-note graph", d.Notes, d.Blocks)
		}
		for j, note := range d.Notes {
			want := []string{"FIRST CONTINUATION", "SECOND CONTINUATION"}[j]
			if len(note.Blocks) != 2 || layoutText(note.Blocks[1]) != want {
				t.Fatal("body boundary or shared separator ownership changed", note)
			}
		}
		assertPDFNoteGlyphInventory(t, readRecovery(t, source), d)
		assertRecoverySF(t, d)
		encoded, err := json.Marshal(d)
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			reference = encoded
		} else if !bytes.Equal(reference, encoded) {
			t.Fatal("note inference is nondeterministic")
		}
	}
}
