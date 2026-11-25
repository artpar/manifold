package manifold

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
	"reflect"
	"strings"
	"testing"
)

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
	// Independent hand-authored references and implicit part relationships.
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
	// OOXML requires a trailing paragraph after a table inside a cell, but these
	// note-level blocks have no added model text or duplicate image data.
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
	if _, err := ReadDOCX(context.Background(), bytes.NewReader(docxWithBody(t, body))); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("unmapped symbol glyph silently lost: %v", err)
	}
}
