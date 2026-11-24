package manydoc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
)

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
				// These originals contain ṭ. Built-in Go lacks this glyph; the
				// optional independent QA supplies real DejaVu style faces.
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
