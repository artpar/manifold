package engine

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"
)

func hyperlinkDOCX(t *testing.T, body, target string) []byte {
	t.Helper()
	base := docxWithBody(t, body)
	z, err := zip.NewReader(bytes.NewReader(base), int64(len(base)))
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	w := zip.NewWriter(&out)
	for _, f := range z.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		part, err := w.Create(f.Name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = part.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	part, err := w.Create("word/_rels/document.xml.rels")
	if err != nil {
		t.Fatal(err)
	}
	_, err = part.Write([]byte(`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="link" Type="` + relNS + `/hyperlink" Target="` + escape(target) + `" TargetMode="External"/></Relationships>`))
	if err != nil {
		t.Fatal(err)
	}
	if err = w.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func TestDOCXRelationshipAndAnchorRetainWordTarget(t *testing.T) {
	for _, c := range []struct{ base, anchor, want string }{
		{"https://example.invalid/path?q=yes", "section", "https://example.invalid/path?q=yes#section"},
		{"https://example.invalid/é?x=1&y=2", "章节–é", "https://example.invalid/é?x=1&y=2#章节–é"},
		{"https://example.invalid/#existing", "next", "https://example.invalid/#existing#next"},
		{"https://example.invalid/%C3%A9", "%2Falready", "https://example.invalid/%C3%A9#%2Falready"},
		{"https://example.invalid/plain", "", "https://example.invalid/plain"},
	} {
		body := `<w:p><w:hyperlink xmlns:r="` + relNS + `" r:id="link" w:anchor="` + escape(c.anchor) + `"><w:r><w:rPr><w:b/></w:rPr><w:t>LABEL</w:t></w:r></w:hyperlink></w:p>`
		d, err := ReadDOCX(context.Background(), bytes.NewReader(hyperlinkDOCX(t, body, c.base)))
		if err != nil {
			t.Fatal(err)
		}
		run := d.Blocks[0].Runs[0]
		if run.Link != c.want || run.Text != "LABEL" || !run.Bold {
			t.Fatal("link identity, content or formatting changed", run, c)
		}
		if hasDiagnostic(d, "hyperlink_word_anchor") != (c.anchor != "") {
			t.Fatal(d.Diagnostics)
		}
		for _, format := range []Format{SF, DOCX, PDF} {
			var out bytes.Buffer
			if err := Write(context.Background(), &out, d, format, WriteOptions{AllowLossy: true}); err != nil {
				t.Fatal(format, err)
			}
			back, err := Read(context.Background(), &out, format, ReadOptions{PDFSkipInference: true})
			if err != nil {
				t.Fatal(format, err)
			}
			want := c.want
			if format == PDF {
				want = pdfASCIIURI(want)
			}
			if back.Blocks[0].Runs[0].Link != want {
				t.Fatal(format, want, back.Blocks[0].Runs)
			}
		}
	}
}
func TestDOCXListTableBoundaryPreservesBlocks(t *testing.T) {
	d := &Document{Blocks: []Block{{Kind: ListBlock, List: "number", Start: listStartPointer(1), Items: []ListItem{listItem("ONLY LIST ITEM")}}, {Kind: Table, Rows: []Row{{Cells: []Cell{{ColSpan: 1, Blocks: []Block{mergeParagraph("TABLE CELL")}}}}}}}}
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, DOCX, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	got, err := ReadDOCX(context.Background(), bytes.NewReader(out.Bytes()))
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
	if !reflect.DeepEqual(after.Content, before.Content) {
		t.Fatal("layout boundary changed authored blocks", got.Blocks)
	}
	for _, change := range []func(string) string{
		func(s string) string {
			return strings.Replace(s, docxListTableBoundary, strings.Replace(docxListTableBoundary, `</w:p>`, `<w:r><w:t>VISIBLE</w:t></w:r></w:p>`, 1), 1)
		},
		func(s string) string {
			return strings.Replace(s, docxListTableBoundary, `<w:p md:listTableBoundary="1"/>`, 1)
		},
		func(s string) string {
			return strings.Replace(s, docxListTableBoundary, strings.Replace(docxListTableBoundary, `w:numId w:val="0"`, `w:numId w:val="1"`, 1), 1)
		},
		func(s string) string {
			return strings.Replace(s, docxListTableBoundary, strings.Replace(docxListTableBoundary, `<w:pPr>`, `<w:wrong>`, 1), 1)
		},
	} {
		bad := rewriteListDOCX(t, out.Bytes(), change)
		if _, err := ReadDOCX(context.Background(), bytes.NewReader(bad)); err == nil {
			t.Fatal("contradictory boundary metadata accepted")
		}
	}
}
func TestDOCXPositionalTabContentAndLoss(t *testing.T) {
	body := `<w:p><w:r><w:rPr><w:b/></w:rPr><w:t>Left</w:t></w:r><w:r><w:ptab w:alignment="center" w:relativeTo="margin" w:leader="none"/></w:r><w:r><w:t>Middle</w:t></w:r><w:r><w:ptab w:alignment="right" w:relativeTo="margin" w:leader="dot"/></w:r><w:r><w:t>Right</w:t></w:r></w:p>`
	d, err := ReadDOCX(context.Background(), bytes.NewReader(docxWithBody(t, body)))
	if err != nil {
		t.Fatal(err)
	}
	if d.Text() != "Left\tMiddle\tRight" || !hasDiagnostic(d, "position_tab") {
		t.Fatal("visible positional separation discarded or loss hidden", d.Text(), d.Diagnostics)
	}
	for _, format := range []Format{SF, DOCX} {
		var out bytes.Buffer
		if err := Write(context.Background(), &out, d, format, WriteOptions{AllowLossy: true}); err != nil {
			t.Fatal(err)
		}
		back, err := Read(context.Background(), &out, format, ReadOptions{})
		if err != nil || back.Text() != d.Text() {
			t.Fatal("normalized separator roundtrip", format, err)
		}
	}
	bad := `<w:p><w:r><w:ptab w:alignment="center" w:relativeTo="margin" w:leader="none"><w:t>Must not disappear</w:t></w:ptab></w:r></w:p>`
	if _, err := ReadDOCX(context.Background(), bytes.NewReader(docxWithBody(t, bad))); err == nil {
		t.Fatal("nonempty positional tab silently swallowed content")
	}
}

type realCellOracle struct {
	Row, Column, Colspan, Rowspan int
	TextSHA256                    string `json:"text_sha256"`
}
type realTableOracle struct {
	Rows       int
	HeaderRows int `json:"header_rows"`
	Cells      []realCellOracle
}
type mqttOracle struct {
	SourceSHA256       string `json:"source_sha256"`
	SourceBytes        int    `json:"source_bytes"`
	Paragraphs         int    `json:"nonempty_body_paragraphs"`
	ParagraphHash      string `json:"body_paragraph_multiset_sha256"`
	Headings, Sections int
	Tables             []realTableOracle
	Hyperlinks         map[string]string `json:"hyperlink_text_sha256"`
	Media              map[string]struct {
		Bytes  int
		SHA256 string
	}
}

func realTextDigest(text string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(text))) }
func realBodyParagraphs(bs []Block) []string {
	var texts []string
	for _, b := range bs {
		if b.Kind == Paragraph || b.Kind == Heading {
			var text strings.Builder
			for _, r := range b.Runs {
				text.WriteString(r.Text)
			}
			if s := strings.Join(strings.Fields(text.String()), " "); s != "" {
				texts = append(texts, s)
			}
		}
		if b.Section != nil {
			texts = append(texts, realBodyParagraphs(b.Section.Blocks)...)
		}
		for _, item := range b.Items {
			texts = append(texts, realBodyParagraphs(item.Blocks)...)
		}
		for _, row := range b.Rows {
			for _, cell := range row.Cells {
				texts = append(texts, realBodyParagraphs(cell.Blocks)...)
			}
		}
	}
	return texts
}
func TestRealWordMQTTStructureAssetsAndNormalizedRoundtrip(t *testing.T) {
	raw, err := os.ReadFile("testdata/real/mqtt-v5.0-os.docx")
	if err != nil {
		t.Fatal(err)
	}
	oracleBytes, err := os.ReadFile("testdata/real/mqtt-v5.0-oracle.json")
	if err != nil {
		t.Fatal(err)
	}
	var oracle mqttOracle
	if err := json.Unmarshal(oracleBytes, &oracle); err != nil {
		t.Fatal(err)
	}
	if len(raw) != oracle.SourceBytes || fmt.Sprintf("%x", sha256.Sum256(raw)) != oracle.SourceSHA256 {
		t.Fatal("original published DOCX bytes changed")
	}
	d, err := ReadDOCX(context.Background(), bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if d.Metadata.Title != "MQTT Version 5.0" || d.Metadata.Author != "OASIS Message Queuing Telemetry Transport (MQTT) TC" {
		t.Fatal("publisher metadata lost", d.Metadata)
	}
	paragraphs := realBodyParagraphs(d.Blocks)
	sort.Strings(paragraphs)
	if len(paragraphs) != oracle.Paragraphs || realTextDigest(strings.Join(paragraphs, "\n")) != oracle.ParagraphHash {
		t.Fatal("original nonempty body paragraph multiset changed", len(paragraphs))
	}
	if !strings.Contains(d.Text(), "𪛔") {
		t.Fatal("supplementary-plane example lost")
	}
	var tables []Block
	linkedText := map[string]string{}
	headings, sections, images := 0, 0, 0
	var walk func([]Block)
	walk = func(bs []Block) {
		for _, b := range bs {
			for _, run := range b.Runs {
				if run.Link != "" {
					linkedText[run.Link] += strings.Join(strings.Fields(run.Text), "")
				}
			}
			switch b.Kind {
			case Heading:
				headings++
			case Table:
				tables = append(tables, b)
			case SectionBlock:
				sections++
			case Picture:
				images++
				expected := oracle.Media["word/media/image1.jpg"]
				if b.Image == nil || len(b.Image.Data) != expected.Bytes || fmt.Sprintf("%x", sha256.Sum256(b.Image.Data)) != expected.SHA256 {
					t.Fatal("original JPEG relationship/bytes changed")
				}
			}
			if b.Section != nil {
				walk(b.Section.Blocks)
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
	walk(d.Blocks)
	for target, text := range linkedText {
		if text == "" {
			delete(linkedText, target)
		}
	}
	// The original oracle remains the normative raw-attribute projection.
	// Word's four split-attribute links have a separately verified publisher
	// oracle; do not alter the original package facts to fit import behavior.
	wordLinksRaw, err := os.ReadFile("testdata/real/mqtt-v5.0-word-links.json")
	if err != nil {
		t.Fatal(err)
	}
	var wordLinks mqttOracle
	if err := json.Unmarshal(wordLinksRaw, &wordLinks); err != nil {
		t.Fatal(err)
	}
	if wordLinks.SourceSHA256 != oracle.SourceSHA256 {
		t.Fatal("Word-link oracle source identity changed")
	}
	if len(linkedText) != len(wordLinks.Hyperlinks) {
		t.Fatal("hyperlink target ownership changed", len(linkedText), len(wordLinks.Hyperlinks))
	}
	for target, want := range wordLinks.Hyperlinks {
		if realTextDigest(linkedText[target]) != want {
			t.Fatal("hyperlink visible text/destination ownership changed", target)
		}
	}
	if headings != oracle.Headings || sections != oracle.Sections || len(tables) != len(oracle.Tables) || images != 1 {
		t.Fatal("rich structure lost", headings, sections, len(tables), images)
	}
	for ti, b := range tables {
		expected := oracle.Tables[ti]
		g, err := tableGrid(b.Rows)
		if err != nil {
			t.Fatal(err)
		}
		headers, err := tableHeaderRows(b.Rows)
		if err != nil {
			t.Fatal(err)
		}
		if len(b.Rows) != expected.Rows || headers != expected.HeaderRows || len(g.Origins) != len(expected.Cells) {
			t.Fatal("table membership/header structure changed", ti, len(b.Rows), len(g.Origins))
		}
		for ci, p := range g.Origins {
			want := expected.Cells[ci]
			if p.Row != want.Row || p.Column != want.Column || p.ColSpan != want.Colspan || p.RowSpan != want.Rowspan {
				t.Fatal("original merged-cell grid changed", ti, ci, *p, want)
			}
			cell := b.Rows[p.Row].Cells[p.Cell]
			if realTextDigest(strings.Join(realBodyParagraphs(cell.Blocks), "\n")) != want.TextSHA256 {
				t.Fatal("original cell content ownership changed", ti, ci)
			}
		}
	}
	if len(d.Diagnostics) == 0 {
		t.Fatal("known source formatting/numbering losses became silent")
	}
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, DOCX, WriteOptions{}); !errors.Is(err, ErrLossy) || out.Len() != 0 {
		t.Fatal("unacknowledged losses must fail atomically", err)
	}
	before := canonicalSF(t, d)
	if err := Write(context.Background(), &out, d, DOCX, WriteOptions{AllowLossy: true}); err != nil {
		t.Fatal(err)
	}
	back, err := ReadDOCX(context.Background(), &out)
	if err != nil {
		t.Fatal(err)
	}
	normalized := *d
	normalized.Diagnostics = nil // acknowledged losses; source report stays in the original SF.
	if !bytes.Equal(canonicalSF(t, &normalized), canonicalSF(t, back)) {
		t.Fatal("normalized rich SF semantics changed through DOCX")
	}
	var sf bytes.Buffer
	if err := Write(context.Background(), &sf, d, SF, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	sfBack, err := Read(context.Background(), &sf, SF, ReadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, canonicalSF(t, sfBack)) || !bytes.Equal(before, canonicalSF(t, d)) {
		t.Fatal("SF/source diagnostics or document semantics changed")
	}
}

func TestRealExportFixtureHashes(t *testing.T) {
	raw, err := os.ReadFile("testdata/real/manifest.json")
	if err !=

		// These originals contain ṭ. Built-in Go lacks this glyph; the
		// optional independent QA supplies real DejaVu style faces.
		nil {
		t.Fatal(err)
	}
	var m struct {
		Files []struct {
			Path, SHA256 string
			Size         int
		}
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, f := range m.Files {
		if f.Path == "" || seen[f.Path] {
			t.Fatal("missing/duplicate real fixture identity", f.Path)
		}
		seen[f.Path] = true
		b, err := os.ReadFile("testdata/real/" + f.Path)
		if err != nil {
			t.Fatal(err)
		}
		if len(b) != f.Size || fmt.Sprintf("%x", sha256.Sum256(b)) != f.SHA256 {
			t.Fatal("original real fixture changed", f.Path)
		}
	}
}

type poiStyleOracle struct {
	Paragraph, Start, End int
	Marks                 map[string]any
}
type poiNoteOracle struct {
	ID, Kind   string
	Paragraphs []string
	Styles     []poiStyleOracle
}
type poiSectionOracle struct {
	Stories       map[string][]string
	Page, Margins map[string]float64
	FirstPage     bool `json:"first_page"`
}
type poiSourceOracle struct {
	Metadata          map[string]string
	SHA256            string
	Bytes             int
	Application       string
	Body              []string
	BodyStyles        []poiStyleOracle `json:"body_styles"`
	Notes             []poiNoteOracle
	Sections          []poiSectionOracle
	DeletedText       string   `json:"deleted_text"`
	FieldInstructions []string `json:"field_instructions"`
	Comments          []string
	Review            map[string]int
	Hyperlinks        map[string]string
}

func poiParagraphText(b Block) string {
	var text strings.Builder
	for _, r := range b.Runs {
		if r.Note != nil {
			fmt.Fprintf(&text, "\ufffc%s:%s\ufffc", r.Note.Kind, r.Note.ID)
		} else {
			text.WriteString(r.Text)
		}
	}
	return text.String()
}
func poiParagraphs(bs []Block) []Block {
	var out []Block
	for _, b := range bs {
		if (b.Kind == Paragraph || b.Kind == Heading) && poiParagraphText(b) != "" {
			out = append(out, b)
		}
		if b.Section != nil {
			out = append(out, poiParagraphs(b.Section.Blocks)...)
		}
		for _, item := range b.Items {
			out = append(out, poiParagraphs(item.Blocks)...)
		}
		for _, row := range b.Rows {
			for _, cell := range row.Cells {
				out = append(out, poiParagraphs(cell.Blocks)...)
			}
		}
	}
	return out
}
func poiCheckParagraphs(t *testing.T, bs []Block, want []string, styles []poiStyleOracle) {
	t.Helper()
	ps := poiParagraphs(bs)
	var got []string
	for _, p := range ps {
		got = append(got, poiParagraphText(p))
	}
	if !slices.Equal(want, got) {
		t.Fatalf("original paragraph order/text/reference sites changed\nwant %#v\ngot %#v", want, got)
	}
	for _, style := range styles {
		if style.Paragraph >= len(ps) {
			t.Fatal("missing original styled paragraph")
		}
		offset := 0
		for _, run := range ps[style.Paragraph].Runs {
			n := len(run.Text)
			if run.Note != nil {
				n = len("\ufffc" + run.Note.Kind + ":" + run.Note.ID + "\ufffc")
			}
			start, end := offset, offset+n
			offset = end
			if end <= style.Start || start >= style.End {
				continue
			}
			for mark, value := range style.Marks {
				var actual any
				switch mark {
				case "bold":
					actual = run.Bold
				case "italic":
					actual = run.Italic
				case "underline":
					actual = run.Underline
				case "strike":
					actual = run.Strike
				case "size":
					actual = run.FontSize
				case "color":
					actual = run.Color
				default:
					t.Fatal("unknown source oracle mark", mark)
				}
				if actual != value {
					t.Fatalf("original paragraph%d range%d:%d %s: want %v got %v (%q)", style.Paragraph, style.Start, style.End, mark, value, actual, run.Text)
				}
			}
		}
	}
}
func poiCheckSections(t *testing.T, d *Document, want []poiSectionOracle) {
	t.Helper()
	var sections []*Section
	for _, b := range d.Blocks {
		if b.Section != nil {
			sections = append(sections, b.Section)
		}
	}
	if len(sections) == 0 {
		sections = []*Section{{Layout: &d.Layout}}
	}
	if len(sections) != len(want) {
		t.Fatal("section count", len(sections), len(want))
	}
	for i, s := range sections {
		o := want[i]
		if s.FirstPage != o.FirstPage {
			t.Fatal("first-page story activation", i)
		}
		layout := d.Layout
		if s.Layout != nil {
			layout = *s.Layout
		}
		if layout.Width != o.Page["w"] || layout.Height != o.Page["h"] {
			t.Fatal("original page geometry", layout, o.Page)
		}
		margins := pageLayout(layout).margins
		for _, pair := range []struct {
			key   string
			value float64
		}{{"top", margins.Top}, {"bottom", margins.Bottom}, {"left", margins.Left}, {"right", margins.Right}} {
			if pair.value != o.Margins[pair.key] {
				t.Fatal("original page margin", pair.key, pair.value, o.Margins)
			}
		}
		for _, story := range []struct {
			key   string
			value *RunningStory
		}{{"header_default", s.Headers.Default}, {"header_first", s.Headers.First}, {"header_even", s.Headers.Even}, {"footer_default", s.Footers.Default}, {"footer_first", s.Footers.First}, {"footer_even", s.Footers.Even}} {
			expected, exists := o.Stories[story.key]
			if exists != (story.value != nil) {
				t.Fatal("original story binding", story.key)
			}
			if exists {
				poiCheckParagraphs(t, story.value.Blocks, expected, nil)
			}
		}
	}
}
func TestRealPOIOriginalEditorExports(t *testing.T) {
	raw, err := os.ReadFile("testdata/real/poi-docx-oracle.json")
	if err != nil {
		t.Fatal(err)
	}
	var sources map[string]poiSourceOracle
	if err = json.Unmarshal(raw, &sources); err != nil {
		t.Fatal(err)
	}
	if len(sources) != 8 {
		t.Fatal("expected eight pinned source exports")
	}
	for name, o := range sources {
		t.Run(name, func(t *testing.T) {
			source, err := os.ReadFile("testdata/real/" + name)
			if err != nil {
				t.Fatal(err)
			}
			if len(source) != o.Bytes || realTextDigest(string(source)) != o.SHA256 {
				t.Fatal("original fixture bytes changed")
			}
			d, err := ReadDOCX(context.Background(), bytes.NewReader(source))
			if err != nil {
				t.Fatal(err)
			}
			if d.Metadata.Author != o.Metadata["author"] || d.Metadata.Title != o.Metadata["title"] || d.Metadata.Subject != o.Metadata["subject"] {
				t.Fatal("original document metadata changed", d.Metadata, o.Metadata)
			}
			poiCheckParagraphs(t, d.Blocks, o.Body, o.BodyStyles)
			poiCheckSections(t, d, o.Sections)
			links := map[string]string{}
			_ = walkNoteRuns(d.Blocks, func(run Run) error {
				if run.Link != "" {
					links[run.Link] += run.Text
				}
				return nil
			})
			for target, text := range o.Hyperlinks {
				if links[target] != text {
					t.Fatal("original hyperlink relationship/text changed", target, text, links[target])
				}
			}

			if name == "poi-DiffFirstPageHeadFoot.docx" && !hasDiagnostic(d, "position_tab") {
				t.Fatal("original positional-tab presentation loss not diagnosed")
			}
			if len(d.Notes) != len(o.Notes) {
				t.Fatal("original note count")
			}
			for _, note := range o.Notes {
				found := false
				for _, got := range d.Notes {
					if got.ID == note.ID && got.Kind == note.Kind {
						found = true
						poiCheckParagraphs(t, got.Blocks, note.Paragraphs, note.Styles)
					}
				}
				if !found {
					t.Fatal("original note relationship missing", note.ID, note.Kind)
				}
			}
			if (o.Review["ins"] > 0 || o.Review["del"] > 0) && !hasDiagnostic(d, "revision_history") {
				t.Fatal("review history loss not diagnosed")
			}
			if len(o.FieldInstructions) > 0 && !hasDiagnostic(d, "field") {
				t.Fatal("field evaluation loss not diagnosed")
			}
			if len(o.Comments) > 0 && !hasDiagnostic(d, "comments") {
				t.Fatal("comment loss not diagnosed")
			}
			if name == "poi-delins.docx" || name == "poi-endnotes.docx" {

				var out bytes.Buffer
				err := Write(context.Background(), &out, d, PDF, WriteOptions{AllowLossy: true, PDFFontMap: map[string]string{"Times New Roman": "Go", "Calibri": "Go", "Cambria": "Go"}})
				if !errors.Is(err, ErrUnsupported) || !strings.Contains(err.Error(), "U+1E6D") || out.Len() != 0 {
					t.Fatal("original missing glyph was corrupted or partly emitted", err, out.Len())
				}
			}
			before, err := ToStructured(d)
			if err != nil {
				t.Fatal(err)
			}
			for _, format := range []Format{SF, SFGzip, JSON, DOCX} {
				var out bytes.Buffer
				if err := Write(context.Background(), &out, d, format, WriteOptions{AllowLossy: true}); err != nil {
					t.Fatal(format, err)
				}
				back, err := Read(context.Background(), &out, format, ReadOptions{})
				if err != nil {
					t.Fatal(format, err)
				}
				after, err := ToStructured(back)
				if err != nil {
					t.Fatal(format, err)
				}
				if format == DOCX {
					before.Attrs.Diagnostics = nil
					after.Attrs.Diagnostics = nil
				}
				if !reflect.DeepEqual(before, after) {
					t.Fatal("normalized SF semantics changed through", format)
				}
			}
		})
	}
}
