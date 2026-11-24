package manydoc

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
)

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
	if err != nil {
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
