package manifold

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
		// Move the leading root into the upper page region; keep its paragraph
		// extension low enough to provide physical end-of-page wrap evidence.
		page2 = strings.Replace(page2, "20 243 Td", "20 293 Td", 1)
		page2 = strings.Replace(page2, "24 240 Tm", "24 290 Tm", 1)
		page2 = strings.Replace(page2, "20 205 Td", "20 255 Td", 1)
		// Fill the paragraph's initial gap with independent painted lines.
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
		// XObject belongs inside Resources, outside the Font dictionary.
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
