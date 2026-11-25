package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"unicode"
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

		// Move the leading root into the upper page region; keep its paragraph
		// extension low enough to provide physical end-of-page wrap evidence.
		"<< /Type /Font /Subtype /Type1 /BaseFont /Courier-Bold >>",
	}
	if variant == "endnote" {
		cut := strings.Index(content, separator)
		main, body := content[:cut], content[cut+len(separator):]
		objects[1] = "<< /Type /Pages /Kids [3 0 R 11 0 R] /Count 2 >>"

		// Fill the paragraph's initial gap with independent painted lines.
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
				t.

					// XObject belongs inside Resources, outside the Font dictionary.

					Fatal("rich continuation text/style/URI lost", bs[1])
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

					// Reconstruct the pre-inference flow to exercise exhaustion before assignment.

					// Paint order alternates columns and reverses both pairs of lines. The
					// independent oracle is the hand-authored placement, not a writer round trip.
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
func pagedUntaggedNoteSource(variant string) []byte {
	main := "BT /F 11 Tf 20 300 Td (Main text) Tj /F 6 Tf 1 0 0 1 80 304 Tm (1) Tj /F 11 Tf 1 0 0 1 84 300 Tm ( more) Tj ET "
	lines := func(top, bottom int, first, last string) string {
		var out strings.Builder
		for y := top; y >= bottom; y -= 12 {
			text := "these words remain in the same paragraph"
			if y == top {
				text = first
			}
			if y-12 < bottom {
				text = last
			}
			fmt.Fprintf(&out, "BT /F 10 Tf 20 %d Td (%s) Tj ET ", y, text)
		}
		return out.String()
	}
	last := "these last words continue with the same"
	first2 := "italic words continue on this next page"
	if variant == "new-paragraph" || variant == "wide-new-paragraph" {
		last = "This paragraph ends at this page."
		first2 = "A new paragraph starts on this next page"
	}
	root := "BT /F 6 Tf 20 243 Td (1) Tj /F 10 Tf 1 0 0 1 24 240 Tm ( body note) Tj ET "
	separator := "20 255 m 120 255 l S "
	part1 := lines(205, 25, "A long paragraph begins with these words", last)
	part2 := lines(230, 26, first2, "these last words continue with the same")
	part3 := lines(230, 194, "relationship finishes on this final page", "This is the final sentence.")
	page1, page2, page3 := main+separator+root+part1, "BT /F 11 Tf 20 350 Td (Ordinary second-page content) Tj ET "+separator+part2, separator+part3
	if variant == "blank-page" {
		page2 = ""
	}
	if variant == "high-without-separator" {
		page1 = strings.Replace(page1, separator, "", 1)
	}
	if variant == "wide-new-paragraph" {
		page2 = strings.Replace(page2, separator, "20 255 m 260 255 l S ", 1)
	}
	if variant == "missing-separator" {
		page2 = strings.Replace(page2, separator, "", 1)
	}
	if variant == "endnote" {
		page1 = main
		page2 = root + part1

		page2 = strings.Replace(page2, "20 243 Td", "20 293 Td", 1)
		page2 = strings.Replace(page2, "24 240 Tm", "24 290 Tm", 1)
		page2 = strings.Replace(page2, "20 205 Td", "20 255 Td", 1)

		page2 = strings.Replace(page2, "ET BT /F 10 Tf 20 193", "ET "+lines(243, 207, "these words remain in the same paragraph", "these words remain in the same paragraph")+"BT /F 10 Tf 20 193", 1)
		page3 = lines(350, 290, "italic words continue on this next page", "This is the final sentence.")
	}
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>", "<< /Type /Pages /Kids [3 0 R 8 0 R 10 0 R] /Count 3 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 400] /Resources << /Font << /F 4 0 R >> >> /Contents 5 0 R /Annots [6 0 R 7 0 R] >>",
		"<< /Type /Font /Subtype /Type1 /BaseFont /Courier >>", recoveryStream("", page1),
		"<< /Type /Annot /Subtype /Link /Rect [80 299 84 309] /Dest [3 0 R /XYZ 23 246 null] >>",
		"<< /Type /Annot /Subtype /Link /Rect [20 236 24 247] /Dest [3 0 R /XYZ 80 308 null] >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 400] /Resources << /Font << /F 4 0 R >> >> /Contents 9 0 R >>", recoveryStream("", page2),
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 400] /Resources << /Font << /F 4 0 R >> >> /Contents 11 0 R >>", recoveryStream("", page3),
	}
	if variant == "endnote" {
		objects[2] = strings.Replace(objects[2], "[6 0 R 7 0 R]", "[6 0 R]", 1)
		objects[5] = "<< /Type /Annot /Subtype /Link /Rect [80 299 84 309] /Dest [8 0 R /XYZ 23 296 null] >>"
		objects[6] = "<< /Type /Annot /Subtype /Link /Rect [20 286 24 297] /Dest [3 0 R /XYZ 80 308 null] >>"
		objects[7] = strings.Replace(objects[7], "/Contents 9 0 R", "/Contents 9 0 R /Annots [7 0 R]", 1)
	}
	if variant == "competing-note" {
		main2 := "BT /F 11 Tf 20 350 Td (Ordinary second-page content) Tj /F 6 Tf 1 0 0 1 210 354 Tm (2) Tj /F 11 Tf 1 0 0 1 214 350 Tm ( next) Tj ET "
		root2 := "BT /F 6 Tf 20 233 Td (2) Tj /F 10 Tf 1 0 0 1 24 230 Tm ( OTHER NOTE) Tj ET "
		page2 = main2 + separator + root2 + lines(205, 25, "another note starts under its own marker", "these last words continue with the same")
		objects[8] = recoveryStream("", page2)
		objects[7] = strings.Replace(objects[7], "/Contents 9 0 R", "/Contents 9 0 R /Annots [12 0 R 13 0 R]", 1)
		objects = append(objects, "<< /Type /Annot /Subtype /Link /Rect [210 349 214 359] /Dest [8 0 R /XYZ 23 236 null] >>", "<< /Type /Annot /Subtype /Link /Rect [20 226 24 237] /Dest [8 0 R /XYZ 210 358 null] >>")
	}
	if variant == "rich-page" {
		page3 = separator + "q 80 0 0 30 20 210 cm /Img Do Q 20 130 m 160 130 l 160 190 l 20 190 l h 90 130 m 90 190 l 20 160 m 160 160 l S " +
			"BT /F 10 Tf 25 175 Td (A) Tj 70 0 Td (B) Tj ET BT /F 10 Tf 25 145 Td (C) Tj 70 0 Td (D) Tj ET BT /F 10 Tf 20 110 Td (LINKED TAIL) Tj ET"
		objects[9] = strings.Replace(objects[9], "/Contents 11 0 R", "/XObject << /Img 12 0 R >> /Contents 11 0 R /Annots [13 0 R]", 1)

		objects[9] = strings.Replace(objects[9], "/Resources << /Font << /F 4 0 R >> >> /XObject << /Img 12 0 R >>", "/Resources << /Font << /F 4 0 R >> /XObject << /Img 12 0 R >> >>", 1)
		objects[10] = recoveryStream("", page3)
		objects = append(objects, recoveryStream("/Type /XObject /Subtype /Image /Width 2 /Height 1 /ColorSpace /DeviceRGB /BitsPerComponent 8", string([]byte{255, 0, 0, 0, 0, 255})), "<< /Type /Annot /Subtype /Link /Rect [19 106 90 122] /A << /S /URI /URI (https://example.test/cross-page) >> >>")
	}

	return recoveryPDF(objects...)
}

func TestPDFUntaggedNoteAcrossPages(t *testing.T) {
	for _, variant := range []string{"", "new-paragraph", "wide-new-paragraph", "endnote"} {
		t.Run(variant, func(t *testing.T) {
			source := pagedUntaggedNoteSource(variant)
			d, err := ReadPDF(context.Background(), bytes.NewReader(source), ReadOptions{})
			if err != nil {
				t.Fatal(err)
			}
			wantBlocks, wantKind, wantPages := 2, Footnote, 3
			if variant == "new-paragraph" || variant == "wide-new-paragraph" {
				wantBlocks = 3
				wantPages = 1
			}
			if variant == "endnote" {
				wantKind = Endnote
				wantPages = 2
			}
			if len(d.Notes) != 1 || len(d.Notes[0].Blocks) != wantBlocks || d.Notes[0].Kind != wantKind || len(d.Pages) != 3 {
				t.Fatalf("paged note graph: %+v", d.Notes)
			}
			if len(d.Notes[0].Blocks[1].Source.Regions) != wantPages {
				t.Fatal("paragraph lost physical source regions", d.Notes[0].Blocks[1].Source)
			}
			if (variant == "new-paragraph" || variant == "wide-new-paragraph") && len(d.Notes[0].Blocks[2].Source.Regions) != 2 {
				t.Fatal("new paragraph boundary or later continuity lost")
			}
			if variant != "endnote" && (!strings.Contains(d.Text(), "Ordinary second-page content") || len(d.Blocks) != 3 || d.Blocks[1].Kind != PageBreak) {
				t.Fatal("ordinary page content or boundary moved into note", d.Blocks)
			}
			if variant == "endnote" && len(d.Blocks) != 1 {
				t.Fatal("note-only pages left empty reflow pages", d.Blocks)
			}
			if !hasDiagnostic(d, "pdf_note_page_continuation_inferred") || !hasDiagnostic(d, "pdf_note_paragraph_continuation_inferred") {
				t.Fatal("cross-page uncertainty missing")
			}
			writePDFNotePageQA(t, variant, source, d)
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
				if len(got.Notes) != 1 || len(got.Notes[0].Blocks) != wantBlocks || got.Notes[0].Kind != wantKind {
					t.Fatal(format, "paged note semantics changed", got.Notes)
				}
			}
		})
	}
}

func TestPDFUntaggedNotePageWithoutSeparatorRemainsFlow(t *testing.T) {
	source := pagedUntaggedNoteSource("missing-separator")
	d, err := ReadPDF(context.Background(), bytes.NewReader(source), ReadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Notes) != 1 || len(d.Notes[0].Source.Regions) != 1 || len(d.Blocks) <= 3 {
		t.Fatal("unproven continuation assigned", d.Notes, d.Blocks)
	}
	assertPDFNoteGlyphInventory(t, readRecovery(t, source), d)
	assertRecoverySF(t, d)
}

func TestPDFUntaggedNotePageOwnerAndBlankBarriers(t *testing.T) {
	for _, variant := range []string{"competing-note", "blank-page"} {
		t.Run(variant, func(t *testing.T) {
			source := pagedUntaggedNoteSource(variant)
			d, err := ReadPDF(context.Background(), bytes.NewReader(source), ReadOptions{})
			if err != nil {
				t.Fatal(err)
			}
			want := 1
			if variant == "competing-note" {
				want = 2
			}
			if len(d.Notes) != want || len(d.Notes[0].Source.Regions) != 1 {
				t.Fatal("continuation crossed distinct owner or blank page", d.Notes)
			}
			if variant == "blank-page" {
				breaks := 0
				for _, b := range d.Blocks {
					if b.Kind == PageBreak {
						breaks++
					}
				}
				if breaks != 2 {
					t.Fatal("original blank page discarded", d.Blocks)
				}
			}
			assertPDFNoteGlyphInventory(t, readRecovery(t, source), d)
			assertRecoverySF(t, d)
		})
	}
}

func TestPDFUntaggedNoteCrossPageRichAssets(t *testing.T) {
	source := pagedUntaggedNoteSource("rich-page")
	d, err := ReadPDF(context.Background(), bytes.NewReader(source), ReadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Notes) != 1 || len(d.Notes[0].Blocks) != 5 {
		t.Fatal("rich page body", d.Notes)
	}
	bs := d.Notes[0].Blocks
	if bs[2].Kind != Picture || bs[3].Kind != Table || len(bs[3].Rows) != 2 || bs[4].Runs[0].Link != "https://example.test/cross-page" {
		t.Fatal("cross-page assets/table/URI lost", bs)
	}
	im, _, err := image.Decode(bytes.NewReader(bs[2].Image.Data))
	if err != nil {
		t.Fatal(err)
	}
	if color.NRGBAModel.Convert(im.At(0, 0)) != (color.NRGBA{255, 0, 0, 255}) || color.NRGBAModel.Convert(im.At(1, 0)) != (color.NRGBA{0, 0, 255, 255}) {
		t.Fatal("source pixels changed")
	}
	writePDFNotePageQA(t, "rich-page", source, d)
	assertPDFNoteGlyphInventory(t, readRecovery(t, source), d)
	assertRecoverySF(t, d)
}
func TestPDFUntaggedHighBodyWithoutSeparator(t *testing.T) {
	source := pagedUntaggedNoteSource("high-without-separator")
	d, err := ReadPDF(context.Background(), bytes.NewReader(source), ReadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Notes) != 0 {
		t.Fatal("high anonymous body inferred without separator")
	}
	assertPDFNoteGlyphInventory(t, readRecovery(t, source), d)
}

func TestPDFNotePagePlanningBudgetAndCancellation(t *testing.T) {
	d := readRecovery(t, pagedUntaggedNoteSource(""))
	boundary := -1
	for i, b := range d.Blocks {
		if b.Kind == PageBreak {
			boundary = i
			break
		}
	}
	if boundary < 1 {
		t.Fatal("missing independent physical page boundary")
	}
	before, err := json.Marshal(d.Blocks)
	if err != nil {
		t.Fatal(err)
	}
	for _, cancelled := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		r := pdfReader{ctx: ctx, doc: d}
		current := r.pdfNoteExtentFacts(&d.Blocks[boundary-1])
		if cancelled {
			cancel()
		} else {
			r.noteInferenceWork = pdfNoteInferenceLimit
		}
		next, _, ok := r.pdfNoteNextPage(boundary+1, boundary-1, current, 20, 10, 100, Footnote, map[*Block]int{}, map[*Block]bool{}, func(i int) pdfNoteExtentFacts { return r.pdfNoteExtentFacts(&d.Blocks[i]) })
		cancel()
		if ok || next != -1 {
			t.Fatal("unfinished page plan accepted", cancelled, next, ok)
		}
		after, err := json.Marshal(d.Blocks)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(before, after) || len(d.Notes) != 0 {
			t.Fatal("page plan mutated partial flow")
		}
	}
}

func TestPDFFootnoteContinuationCaptionArtifact(t *testing.T) {
	d := &Document{Layout: Layout{Width: 300, Height: 300, Margin: 30}, Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "Main"}, {Note: &NoteReference{ID: "n", Kind: Footnote}}}}}, Notes: []Note{{ID: "n", Kind: Footnote, Blocks: []Block{{Kind: Paragraph, Runs: []Run{{FontFamily: "Go Mono", FontSize: 10, Text: strings.Repeat("A long note keeps all its words across pages. ", 30)}}}}}}}
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, PDF, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	got := readRecovery(t, out.Bytes())
	if len(got.Pages) < 2 || len(got.Notes) != 1 {
		t.Fatal("continuation fixture did not paginate")
	}
	seen := map[int]bool{}
	_ = walkNoteRuns(got.Blocks, func(run Run) error {
		if strings.Contains(run.Text, "Footnotes continued") {
			if run.Source == nil || len(run.Source.Artifacts) == 0 {
				t.Fatal("generated caption lacks explicit artifact membership")
			}
			seen[run.Source.Page] = true
		}
		return nil
	})
	if len(seen) != len(got.Pages)-1 {
		t.Fatal("continuation caption missing or duplicated", seen, len(got.Pages))
	}
	if strings.Contains(layoutText(got.Notes[0].Blocks[0]), "Footnotes continued") {
		t.Fatal("pagination caption became authored note text")
	}
	assertRecoverySF(t, got)
}

func writePDFNotePageQA(t *testing.T, name string, source []byte, d *Document) {
	t.Helper()
	root := os.Getenv("MANYDOC_PDF_NOTE_PAGES_QA")
	if root == "" {
		return
	}
	if name == "" {
		name = "mixed"
	}
	root = filepath.Join(root, name)
	if err := os.MkdirAll(root, 0755); err != nil {
		t.Fatal(err)
	}
	artifacts := map[string][]byte{"source.pdf": source}
	for name, doc := range map[string]*Document{"model": d, "physical": readRecovery(t, source)} {
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
	artifacts["model.sf"] = sf.Bytes()
	for name, data := range artifacts {
		if err := os.WriteFile(filepath.Join(root, name), data, 0644); err != nil {
			t.Fatal(err)
		}
	}
}
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
func TestPDFUntaggedCorpusCoverage(t *testing.T) {
	for _, c := range corpus(t) {
		if c.Kind != "pdf" || c.Expect == "reject" {
			continue
		}
		t.Run(c.Name, func(t *testing.T) {
			data := fixture(t, c.Name)
			raw, e := ReadPDF(context.Background(), bytes.NewReader(data), ReadOptions{Password: "secret", PDFSkipInference: true})
			if e != nil {
				t.Fatal(e)
			}
			got, e := ReadPDF(context.Background(), bytes.NewReader(data), ReadOptions{Password: "secret"})
			if e != nil {
				t.Fatal(e)
			}
			coverage := func(d *Document) (map[rune]int, map[int]Block, map[int]Run) {
				chars := map[rune]int{}
				assets := map[int]Block{}
				spans := map[int]Run{}
				add := func(s string) {
					for _, ch := range s {
						if !unicode.IsSpace(ch) {
							chars[ch]++
						}
					}
				}
				var walk func([]Block)
				walk = func(bs []Block) {
					for _, b := range bs {
						if b.Kind == Picture {
							assets[b.Source.Order] = b
						}
						for _, run := range b.Runs {
							if run.Source != nil && run.Source.Precision == "synthetic_separator" {
								continue
							}
							add(run.Text)
							if run.Source != nil {
								spans[run.Source.Order] = run
							}
						}
						for _, item := range b.Items {
							if item.Source != nil {
								add(item.Source.Label)
							}
							walk(item.Blocks)
						}
						for _, row := range b.Rows {
							for _, cell := range row.Cells {
								walk(cell.Blocks)
							}
						}
					}
				}
				walk(d.Blocks)
				return chars, assets, spans
			}
			x, a, spans := coverage(raw)
			y, b, gotSpans := coverage(got)
			if !reflect.DeepEqual(x, y) || !reflect.DeepEqual(a, b) || !reflect.DeepEqual(raw.Pages, got.Pages) {
				t.Fatal("inference lost/duplicated literal content, assets, placements or page geometry")
			}
			if !hasDiagnostic(got, "pdf_list_inferred") && !reflect.DeepEqual(spans, gotSpans) {
				t.Fatal("inference changed original formatting, relationships or source spans")
			}
		})
	}
}

func inferenceText(font string, size, x, y int, text string) string {
	text = strings.NewReplacer(`\`, `\\`, `(`, `\(`, `)`, `\)`).Replace(text)
	return fmt.Sprintf("BT /%s %d Tf 1 0 0 1 %d %d Tm (%s) Tj ET\n", font, size, x, y, text)
}

func FuzzPDFInferenceCoverage(f *testing.F) {
	f.Add([]byte{30, 40, 10, 50, 60, 70, 80, 90})
	f.Add([]byte{0, 0, 0, 0, 255, 255, 255, 255})
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 64 {
			t.Skip()
		}
		n := max(1, len(data))
		content := ""
		for i := 0; i < n; i++ {
			v := byte(0)
			if len(data) > 0 {
				v = data[i]
			}
			font := "F"
			if v&1 != 0 {
				font = "B"
			}
			content += inferenceText(font, 6+int(v%15), 20+int(v)*2, 50+int(v)*2, fmt.Sprintf("TOKEN%03d", i))
		}
		pdf := recoveryPDF(inferenceObjects(content)...)
		raw, e := ReadPDF(context.Background(), bytes.NewReader(pdf), ReadOptions{PDFSkipInference: true})
		if e != nil {
			t.Fatal(e)
		}
		got, e := ReadPDF(context.Background(), bytes.NewReader(pdf), ReadOptions{})
		if e != nil {
			t.Fatal(e)
		}
		original := map[int]Run{}
		for _, b := range raw.Blocks {
			for _, run := range b.Runs {
				if run.Source.Precision != "synthetic_separator" {
					original[run.Source.Order] = run
				}
			}
		}
		seen := map[int]bool{}
		var walk func([]Block)
		walk = func(bs []Block) {
			for _, b := range bs {
				for _, run := range b.Runs {
					if run.Source.Precision == "synthetic_separator" {
						continue
					}
					if seen[run.Source.Order] || !reflect.DeepEqual(run, original[run.Source.Order]) {
						t.Fatal("inference duplicated, changed or invented painted content/geometry")
					}
					seen[run.Source.Order] = true
				}
				for _, item := range b.Items {
					walk(item.Blocks)
				}
				for _, row := range b.Rows {
					for _, cell := range row.Cells {
						walk(cell.Blocks)
					}
				}
			}
		}
		walk(got.Blocks)
		if len(seen) != n {
			t.Fatalf("lost source spans: got %d want %d", len(seen), n)
		}
		assertRecoverySF(t, got)
	})
}
func inferenceObjects(content string) []string {
	return []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 600 800] /Resources << /Font << /F 4 0 R /B 5 0 R >> /XObject << /Im 7 0 R >> >> /Contents 6 0 R >>",
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica-Bold >>",
		recoveryStream("", content),
		recoveryStream("/Type /XObject /Subtype /Image /Width 1 /Height 1 /BitsPerComponent 8 /ColorSpace /DeviceRGB", string([]byte{255, 0, 0})),
	}
}
func readInferred(t *testing.T, objects []string) *Document {
	t.Helper()
	d, e := ReadPDF(context.Background(), bytes.NewReader(recoveryPDF(objects...)), ReadOptions{})
	if e != nil {
		t.Fatal(e)
	}
	if !hasDiagnostic(d, "pdf_structure_inferred") {
		t.Fatal("inference not disclosed")
	}
	assertRecoverySF(t, d)
	return d
}

func TestPDFUntaggedColumnsParagraphsHeadingAndLinks(t *testing.T) {

	content := inferenceText("F", 10, 330, 676, "RIGHT continues in the same paragraph.")
	content += inferenceText("F", 10, 40, 640, "LEFT second paragraph is separated.")
	content += inferenceText("B", 22, 40, 750, "Quarterly review and supporting measurements")
	content += inferenceText("F", 10, 40, 676, "LEFT continues in the same paragraph.")
	content += inferenceText("F", 10, 330, 690, "RIGHT begins here; then continues")
	content += inferenceText("F", 10, 40, 690, "LEFT starts with the first paragraph")
	content += inferenceText("F", 10, 330, 640, "RIGHT second paragraph is separated.")
	objects := inferenceObjects(content)
	objects[2] = strings.Replace(objects[2], "/Contents 6 0 R", "/Annots [8 0 R] /Contents 6 0 R", 1)
	objects = append(objects, "<< /Type /Annot /Subtype /Link /Rect [329 687 590 700] /A << /S /URI /URI (https://example.invalid/right) >> >>")
	d := readInferred(t, objects)
	want := []string{"Quarterly review and supporting measurements", "LEFT starts with the first paragraph LEFT continues in the same paragraph.", "LEFT second paragraph is separated.", "RIGHT begins here; then continues RIGHT continues in the same paragraph.", "RIGHT second paragraph is separated."}
	if len(d.Blocks) != len(want) {
		t.Fatalf("column boundaries: %+v", d.Blocks)
	}
	seen := map[int]bool{}
	for i, b := range d.Blocks {
		if layoutText(b) != want[i] || b.Source.Structure != "inferred" {
			t.Fatalf("block %d: %+v", i, b)
		}
		for _, run := range b.Runs {
			if run.Source.Precision == "synthetic_separator" {
				continue
			}
			if run.Source.Structure != "unstructured" || seen[run.Source.Order] {
				t.Fatal("paint ownership duplicated or promoted to explicit")
			}
			seen[run.Source.Order] = true
		}
	}
	if len(seen) != 7 || d.Blocks[0].Kind != Heading || d.Blocks[0].Level != 1 || !d.Blocks[0].Runs[0].Bold || d.Blocks[3].Runs[0].Link != "https://example.invalid/right" {
		t.Fatal("heading/style/URI or complete original spans lost")
	}
	if d.Blocks[1].Source.Quad[1].X >= 300 || d.Blocks[3].Source.Quad[0].X < 329 {
		t.Fatal("columns merged spatially")
	}
	for _, b := range d.Blocks {
		if b.Kind == Table || b.Kind == ListBlock {
			t.Fatal("prose columns misclassified")
		}
	}
}

func TestPDFUntaggedTableListContinuationAndAssets(t *testing.T) {
	content := ""
	for row, values := range [][]string{{"Item", "Count", "Price"}, {"Apples", "12", "$5.00"}, {"Pears", "34", "$9.25"}} {
		for col := 2; col >= 0; col-- {
			content += inferenceText("F", 10, []int{40, 220, 340}[col], 650-row*14, values[col])
		}
	}
	content += inferenceText("F", 10, 40, 550, "3. First numbered item")
	content += inferenceText("F", 10, 54, 536, "continued at the hanging indent.")
	content += inferenceText("F", 10, 40, 522, "4. Second numbered item")
	content += inferenceText("F", 10, 58, 508, "- Nested one")
	content += inferenceText("F", 10, 58, 494, "- Nested two")
	content += "q 40 0 0 30 400 210 cm /Im Do Q\n"
	d := readInferred(t, inferenceObjects(content))
	if len(d.Blocks) != 3 || d.Blocks[0].Kind != Table || d.Blocks[1].Kind != ListBlock || d.Blocks[2].Kind != Picture {
		t.Fatalf("table/list/image membership: %+v", d.Blocks)
	}
	table := d.Blocks[0]
	if len(table.Rows) != 3 || len(table.Rows[0].Cells) != 3 || table.Source.Evidence != "aligned_rows_numeric_columns" {
		t.Fatalf("table dimensions: %+v", table)
	}
	for row, values := range [][]string{{"Item", "Count", "Price"}, {"Apples", "12", "$5.00"}, {"Pears", "34", "$9.25"}} {
		for col, want := range values {
			cell := table.Rows[row].Cells[col]
			if len(cell.Blocks) != 1 || layoutText(cell.Blocks[0]) != want || cell.Source.Structure != "inferred" || cell.Source.Quad == nil {
				t.Fatalf("cell %d/%d: %+v", row, col, cell)
			}
		}
	}
	list := d.Blocks[1]
	if list.List != "number" || list.Start == nil || *list.Start != 3 || len(list.Items) != 2 || list.Items[1].Source.Label != "4." {
		t.Fatalf("numbered list: %+v", list)
	}
	if layoutText(list.Items[0].Blocks[0]) != "First numbered item continued at the hanging indent." {
		t.Fatalf("continuation: %+v", list.Items[0])
	}
	if len(list.Items[1].Blocks) != 2 || list.Items[1].Blocks[1].Kind != ListBlock || len(list.Items[1].Blocks[1].Items) != 2 || list.Items[1].Blocks[1].Items[0].Source.Label != "-" {
		t.Fatalf("nested list: %+v", list.Items[1])
	}
	if list.Items[0].Blocks[0].Runs[0].Source.Precision != "partial_text_span_bounds" {
		t.Fatal("prefix slicing claimed exact bounds")
	}
	if d.Blocks[2].Image == nil || len(d.Blocks[2].Image.Data) == 0 || d.Blocks[2].Source.Quad == nil || !hasDiagnostic(d, "pdf_list_inferred") || !hasDiagnostic(d, "pdf_table_inferred") {
		t.Fatal("asset or inference evidence missing")
	}
}

func TestPDFUntaggedNumberedValuesAreNotTableCells(t *testing.T) {
	content := inferenceText("F", 10, 40, 600, "3.") + inferenceText("F", 10, 100, 600, "2021")
	content += inferenceText("F", 10, 40, 586, "4.") + inferenceText("F", 10, 100, 586, "2022")
	d := readInferred(t, inferenceObjects(content))
	if len(d.Blocks) != 1 || d.Blocks[0].Kind != ListBlock || d.Blocks[0].Start == nil || *d.Blocks[0].Start != 3 || len(d.Blocks[0].Items) != 2 || layoutText(d.Blocks[0].Items[0].Blocks[0]) != "2021" || hasDiagnostic(d, "pdf_table_inferred") {
		t.Fatalf("numbered values misclassified: %+v", d.Blocks)
	}
}

func TestPDFUntaggedSpanningEdgesDoNotInterleaveColumns(t *testing.T) {
	content := inferenceText("F", 10, 330, 560, "RIGHT lower paragraph has a large preceding gap.")
	content += inferenceText("B", 20, 40, 750, "Quarterly review and supporting measurements")
	content += inferenceText("F", 10, 40, 710, "LEFT upper paragraph starts here.")
	content += inferenceText("F", 10, 330, 710, "RIGHT upper paragraph starts here.")
	content += inferenceText("F", 10, 40, 560, "LEFT lower paragraph has a large preceding gap.")
	content += inferenceText("F", 12, 40, 480, "FULL WIDTH concluding text spans both of the columns before the page ends.")
	d := readInferred(t, inferenceObjects(content))
	want := []string{"Quarterly review and supporting measurements", "LEFT upper paragraph starts here.", "LEFT lower paragraph has a large preceding gap.", "RIGHT upper paragraph starts here.", "RIGHT lower paragraph has a large preceding gap.", "FULL WIDTH concluding text spans both of the columns before the page ends."}
	if len(d.Blocks) != len(want) {
		t.Fatalf("spanning edge boundaries: %+v", d.Blocks)
	}
	for i, b := range d.Blocks {
		if layoutText(b) != want[i] {
			t.Fatalf("block %d: %q want %q", i, layoutText(b), want[i])
		}
	}
}

func TestPDFUntaggedUncertainGeometryAndOptOut(t *testing.T) {
	for name, content := range map[string]string{
		"vertical":              "BT /F 10 Tf 0 1 -1 0 20 100 Tm (vertical) Tj ET",
		"logical_without_paint": "/Span << /ActualText <FEFF006C006F0067006900630061006C> >> BDC EMC",
	} {
		t.Run(name, func(t *testing.T) {
			d := readInferred(t, inferenceObjects(content))
			if !hasDiagnostic(d, "pdf_inference_uncertain") || !strings.Contains(d.Text(), map[string]string{"vertical": "vertical", "logical_without_paint": "logical"}[name]) {
				t.Fatal("uncertain content omitted or assigned silent confidence")
			}
		})
	}
	content := "q 40 0 0 20 10 30 cm /Im Do Q\n" + inferenceText("F", 10, 10, 100, "ABOVE")
	objects := inferenceObjects(content)
	d := readInferred(t, objects)
	if len(d.Blocks) != 2 || d.Blocks[0].Kind != Paragraph || d.Blocks[1].Kind != Picture {
		t.Fatal("displayed text above image was left in paint order")
	}
	raw, e := ReadPDF(context.Background(), bytes.NewReader(recoveryPDF(objects...)), ReadOptions{PDFSkipInference: true})
	if e != nil || raw.Blocks[0].Kind != Picture || raw.Blocks[1].Runs[0].Text != "ABOVE" || hasDiagnostic(raw, "pdf_structure_inferred") {
		t.Fatalf("physical extraction opt-out: %+v, %v", raw, e)
	}
	overlap := "q 100 0 0 40 10 80 cm /Im Do Q\n" + inferenceText("F", 10, 20, 100, "OVERLAY")
	d = readInferred(t, inferenceObjects(overlap))
	if len(d.Blocks) != 2 || !strings.Contains(d.Text(), "OVERLAY") || !hasDiagnostic(d, "pdf_inference_uncertain") {
		t.Fatal("overlapping image/text omitted or ordering certainty invented")
	}
}
