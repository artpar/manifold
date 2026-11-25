package engine

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
)

func outputNoteText(n Note) string {
	return strings.Join(strings.Fields((&Document{Blocks: n.Blocks}).Text()), "")
}
func outputImages(bs []Block) []*Image {
	var out []*Image
	for _, b := range bs {
		if b.Image != nil {
			out = append(out, b.Image)
		}
		for _, it := range b.Items {
			out = append(out, outputImages(it.Blocks)...)
		}
		for _, row := range b.Rows {
			for _, c := range row.Cells {
				out = append(out, outputImages(c.Blocks)...)
			}
		}
		if b.Section != nil {
			out = append(out, outputImages(b.Section.Blocks)...)
		}
	}
	return out
}
func TestGeneratedPDFTypedNoteGraphs(t *testing.T) {
	for _, spec := range []struct {
		name string
		doc  *Document
	}{{"notes", noteDocument()}, {"rich-cells", richCellDocument(t)}, {"recursive", recursiveTableDocument(t)}, {"body-marks", vdocsBodyMarksExample(t)}} {
		t.Run(spec.name, func(t *testing.T) {
			d := spec.doc
			before := canonicalSF(t, d)
			var buf bytes.Buffer
			if err := Write(context.Background(), &buf, d, PDF, WriteOptions{}); err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(before, canonicalSF(t, d)) {
				t.Fatal("writer mutated caller's model")
			}
			if !bytes.HasPrefix(buf.Bytes(), []byte("%PDF-2.0")) {
				t.Fatal("PDF2 namespace without PDF2 header")
			}
			got, err := ReadPDF(context.Background(), bytes.NewReader(buf.Bytes()), ReadOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if len(got.Notes) != len(d.Notes) {
				t.Fatal("typed definitions lost", len(got.Notes), len(d.Notes))
			}
			ids := map[noteKey]noteKey{}
			for _, want := range d.Notes {
				var matches []Note
				for _, n := range got.Notes {
					if n.Kind == want.Kind && outputNoteText(n) == outputNoteText(want) {
						matches = append(matches, n)
					}
				}
				if len(matches) != 1 {
					t.Fatalf("note content/ownership changed: %s %q matches=%d", want.Kind, outputNoteText(want), len(matches))
				}
				n := matches[0]
				ids[noteIdentity(want)] = noteIdentity(n)
				if n.KindInferred || n.Source == nil || n.Source.Structure != "explicit" || n.Source.Role != "FENote" {
					t.Fatal("explicit note type/source lost", n)
				}
				if len(outputImages(n.Blocks)) != len(outputImages(want.Blocks)) {
					t.Fatal("note images lost")
				}
				for j, im := range outputImages(n.Blocks) {
					sameImagePixels(t, outputImages(want.Blocks)[j].Data, im.Data)
					if im.Alt != outputImages(want.Blocks)[j].Alt {
						t.Fatal("image alternative text lost")
					}
					if len(im.Data) == 0 {
						t.Fatal("note image bytes lost")
					}
				}
			}
			a, b := collectedNoteRefs(d), collectedNoteRefs(got)
			if len(a) != len(b) {
				t.Fatal("citation sites lost", len(a), len(b))
			}
			for i, ref := range b {
				if ref.Note.Inferred || referenceKey(ref.Note) != ids[referenceKey(a[i].Note)] || ref.Source == nil || ref.Source.Structure != "explicit" {
					t.Fatal("citation relationship/source changed", i, ref)
				}
			}
			// Physical inspection retains every painted figure/glyph, independently of
			// relocation into editable note definitions. These are different oracles.
			physical, err := ReadPDF(context.Background(), bytes.NewReader(buf.Bytes()), ReadOptions{PDFSkipStructure: true, PDFSkipInference: true})
			if err != nil {
				t.Fatal(err)
			}
			if len(physical.Notes) != 0 || !hasDiagnostic(physical, "pdf_structure_skipped") {
				t.Fatal("physical reader still used logical note ownership")
			}
			expectedImages := len(outputImages(d.Blocks))
			for _, n := range d.Notes {
				expectedImages += len(outputImages(n.Blocks))
			}
			if len(outputImages(physical.Blocks)) != expectedImages {
				t.Fatal("physical assets lost")
			}
			assertRecoverySF(t, got)
		})
	}
}
func TestGeneratedPDFNoteContinuationRegions(t *testing.T) {
	d := &Document{Layout: Layout{Width: 260, Height: 160, Margin: 20}, Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "Cite"}, noteRun(Footnote, "long")}}}, Notes: []Note{{Kind: Footnote, ID: "long", Blocks: []Block{mergeParagraph(strings.Repeat("continued words ", 90) + "LAST_NOTE_MARKER")}}}}
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, PDF, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	got, err := ReadPDF(context.Background(), &out, ReadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Notes) != 1 || outputNoteText(got.Notes[0]) != outputNoteText(d.Notes[0]) {
		t.Fatal("continuation duplicated/lost note body")
	}
	pages := map[int]bool{}
	for _, region := range got.Notes[0].Source.Regions {
		pages[region.Page] = true
	}
	if len(pages) < 2 {
		t.Fatal("continued definition lost physical regions", pages)
	}
}
func TestGeneratedPDFLogicalContainersAndFormatting(t *testing.T) {
	d := richCellDocument(t)
	d.Blocks = append(d.Blocks, Block{Kind: ListBlock, List: "number", Start: listStartPointer(3), Items: []ListItem{{Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "LOGICAL_LIST_ITEM", Bold: true, Color: "#123456", Link: "https://example.org/structure"}}}}}}})
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, PDF, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	got, err := ReadPDF(context.Background(), &out, ReadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	tables, lists, pictures := 0, 0, 0
	styled := ""
	var walk func([]Block)
	walk = func(bs []Block) {
		for _, b := range bs {
			for _, run := range b.Runs {
				if run.Bold && run.Color == "#123456" && run.Link == "https://example.org/structure" {
					styled += run.Text
				}
			}
			if b.Kind == ListBlock && b.List == "number" && (b.Start == nil || *b.Start != 3) {
				t.Fatal("explicit list start lost")
			}
			if b.Kind == Table {
				tables++
				if b.Source == nil || b.Source.Structure != "explicit" {
					t.Fatal("table role inferred")
				}
			}
			if b.Kind == ListBlock {
				lists++
			}
			if b.Kind == Picture {
				pictures++
			}
			for _, it := range b.Items {
				walk(it.Blocks)
			}
			for _, row := range b.Rows {
				for _, c := range row.Cells {
					walk(c.Blocks)
				}
			}
		}
	}
	walk(got.Blocks)
	for _, n := range got.Notes {
		walk(n.Blocks)
	}
	if styled != "LOGICAL_LIST_ITEM" {
		t.Fatal("formatting/URI relationship lost", styled)
	}
	if tables < 2 || lists < 1 || pictures != 3 {
		t.Fatal("rich structure/assets lost", tables, lists, pictures)
	}
}

func TestGeneratedPDFContinuedNoteHasOneLogicalOwner(t *testing.T) {
	d := &Document{Layout: Layout{Width: 260, Height: 160, Margin: 20}, Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "Anchor"}, noteRun(Footnote, "long")}}}, Notes: []Note{{Kind: Footnote, ID: "long", Blocks: []Block{mergeParagraph(strings.Repeat("continued words ", 80) + "LAST_NOTE_MARKER")}}}}
	var buf bytes.Buffer
	if err := Write(context.Background(), &buf, d, PDF, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	got, err := ReadPDF(context.Background(), &buf, ReadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Notes) != 1 || outputNoteText(got.Notes[0]) != outputNoteText(d.Notes[0]) || len(collectedNoteRefs(got)) != 1 {
		t.Fatal("continued note split or duplicated")
	}
	pages := map[int]bool{}
	for _, region := range got.Notes[0].Source.Regions {
		pages[region.Page] = true
	}
	if len(pages) < 2 {
		t.Fatal("cross-page note provenance lost")
	}
}

func TestGeneratedPDFRepeatedHeaderHasOneLogicalCopy(t *testing.T) {
	d := repeatedHeaderDocument(t)
	var buf bytes.Buffer
	if err := Write(context.Background(), &buf, d, PDF, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	got, err := ReadPDF(context.Background(), bytes.NewReader(buf.Bytes()), ReadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Blocks) == 0 || got.Blocks[0].Kind != Table {
		t.Fatal("explicit table missing")
	}
	if len(got.Blocks[0].Rows) != len(d.Blocks[0].Rows) {
		t.Fatal("repeated header duplicated logical rows")
	}
	wantText := outputNoteText(Note{Blocks: []Block{d.Blocks[0]}})
	if outputNoteText(Note{Blocks: []Block{got.Blocks[0]}}) != wantText {
		t.Fatal("table content lost or repeated")
	}

	physical, err := ReadPDF(context.Background(), bytes.NewReader(buf.Bytes()), ReadOptions{PDFSkipInference: true, PDFSkipStructure: true})
	if err != nil {
		t.Fatal(err)
	}
	if physical.Metadata.PageCount < 2 {
		t.Fatal("fixture did not exercise actual header repetition")
	}
}

func FuzzGeneratedPDFNoteGraph(f *testing.F) {
	f.Add(byte(0), byte(5))
	f.Add(byte(1), byte(20))
	f.Add(byte(2), byte(35))
	f.Fuzz(func(t *testing.T, mode, amount byte) {
		d := &Document{Layout: Layout{Width: 260, Height: float64(150 + int(amount%80)), Margin: 20}, Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "Anchor "}}}}}
		for i := 0; i < 3; i++ {
			kind := Footnote
			if i == 2 {
				kind = Endnote
			}
			id := fmt.Sprintf("id/%d-é😀", i)
			note := Note{Kind: kind, ID: id, Blocks: []Block{mergeParagraph(strings.Repeat("Words ", int(amount%24)+1) + fmt.Sprint(i))}}
			ref := noteRun(kind, id)
			if mode%3 == 1 {
				empty := ""
				note.BodyLabel = &empty
				ref.Note.Label = &empty
			}
			if mode%3 == 2 {
				note.BodyLabelContent = []Run{{Text: "§", Bold: true}, {Text: fmt.Sprint(i + 1), Italic: true}}
				ref.Note.LabelContent = []Run{{Text: "*", Bold: true}, {Text: fmt.Sprint(i + 1), Italic: true}}
			}
			d.Notes = append(d.Notes, note)
			d.Blocks[0].Runs = append(d.Blocks[0].Runs, ref, Run{Text: " / "})
		}
		d.Blocks[0].Runs = append(d.Blocks[0].Runs, noteRun(Footnote, d.Notes[0].ID))
		var b bytes.Buffer
		if err := Write(context.Background(), &b, d, PDF, WriteOptions{}); err != nil {
			t.Fatal(err)
		}
		got, err := ReadPDF(context.Background(), &b, ReadOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if len(got.Notes) != 3 || len(collectedNoteRefs(got)) != 4 {
			t.Fatalf("definitions/citations lost: notes=%d refs=%d text=%q diagnostics=%v", len(got.Notes), len(collectedNoteRefs(got)), got.Text(), got.Diagnostics)
		}
		for i, n := range got.Notes {
			if n.ID != d.Notes[i].ID || n.Kind != d.Notes[i].Kind || n.KindInferred || outputNoteText(n) != outputNoteText(d.Notes[i]) {
				t.Fatal("painted note ownership/content changed")
			}
		}
	})
}

func TestGeneratedPDFStandaloneMergedTable(t *testing.T) {
	d := &Document{Blocks: []Block{{Kind: Heading, Level: 2, Runs: []Run{{Text: "Standalone table"}}}, {Kind: Table, Rows: []Row{{Cells: []Cell{{ColSpan: 2, RowSpan: 2, Blocks: []Block{mergeParagraph("MERGED")}}, {Blocks: []Block{mergeParagraph("RIGHT_TOP")}}}}, {Cells: []Cell{{Blocks: []Block{mergeParagraph("RIGHT_BOTTOM")}}}}}}}}
	var b bytes.Buffer
	if err := Write(context.Background(), &b, d, PDF, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	got, err := ReadPDF(context.Background(), &b, ReadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Blocks) != 2 || got.Blocks[0].Kind != Heading || got.Blocks[0].Level != 2 || got.Blocks[1].Kind != Table {
		t.Fatal("standalone semantic blocks lost")
	}
	table := got.Blocks[1]
	if len(table.Rows) != 2 || len(table.Rows[0].Cells) != 2 || len(table.Rows[1].Cells) != 1 || table.Rows[0].Cells[0].ColSpan != 2 || table.Rows[0].Cells[0].RowSpan != 2 {
		t.Fatal("explicit merged grid lost")
	}
	if table.Source.Structure != "explicit" || table.Source.Role != "Table" || outputNoteText(Note{Blocks: []Block{table}}) != "MERGEDRIGHT_TOPRIGHT_BOTTOM" {
		t.Fatal("table content/source lost")
	}
}

func TestGeneratedPDFNoteIDsRemainStable(t *testing.T) {
	d := noteDocument()
	for pass := 0; pass < 2; pass++ {
		var b bytes.Buffer
		if err := Write(context.Background(), &b, d, PDF, WriteOptions{AllowLossy: true}); err != nil {
			t.Fatal(err)
		}
		got, err := ReadPDF(context.Background(), &b, ReadOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if len(got.Notes) != 3 || got.Notes[0].ID != "endnote/shared" || got.Notes[1].ID != "alpha &<42>" || got.Notes[2].ID != "footnote/shared" {
			t.Fatal("scoped/global IDs changed", pass)
		}
		d = got
	}
}
