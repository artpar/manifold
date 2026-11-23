package manydoc

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

type corpusCase struct {
	Name    string   `json:"name"`
	Kind    string   `json:"kind"`
	Expect  string   `json:"expect"`
	Markers []string `json:"markers"`
	Pages   int      `json:"pages"`
	SHA256  string   `json:"sha256"`
}

func TestCorpusHashes(t *testing.T) {
	for _, c := range corpus(t) {
		actual := fmt.Sprintf("%x", sha256.Sum256(fixture(t, c.Name)))
		if actual != c.SHA256 {
			t.Errorf("fixture %s hash mismatch: %s", c.Name, actual)
		}
	}
}

func TestInvalidModelAndIO(t *testing.T) {
	for _, d := range []*Document{nil, {Blocks: []Block{{Kind: "unknown"}}}, {Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "bad\x00text"}}}}}, {Layout: Layout{Width: 10, Height: 10, Margin: 20}}} {
		if e := Write(context.Background(), &bytes.Buffer{}, d, DOCX, WriteOptions{}); e == nil {
			t.Fatal("invalid document accepted")
		}
	}
	if _, e := Read(context.Background(), nil, DOCX, ReadOptions{}); e == nil {
		t.Fatal("nil reader accepted")
	}
	if e := Write(context.Background(), nil, &Document{}, DOCX, WriteOptions{}); e == nil {
		t.Fatal("nil writer accepted")
	}
	if _, e := Read(context.Background(), strings.NewReader(`{"blocks":[],"typo":true}`), JSON, ReadOptions{}); e == nil {
		t.Fatal("unknown JSON field accepted")
	}
	d, e := Read(context.Background(), strings.NewReader("first\fsecond"), Text, ReadOptions{})
	if e != nil {
		t.Fatal(e)
	}
	if len(d.Blocks) != 3 || d.Blocks[1].Kind != PageBreak {
		t.Fatal("text page boundary lost")
	}
}

func corpus(t testing.TB) []corpusCase {
	t.Helper()
	b, e := os.ReadFile("testdata/corpus/manifest.json")
	if e != nil {
		t.Fatal(e)
	}
	var c []corpusCase
	if e = json.Unmarshal(b, &c); e != nil {
		t.Fatal(e)
	}
	return c
}
func fixture(t testing.TB, name string) []byte {
	t.Helper()
	b, e := os.ReadFile(filepath.Join("testdata/corpus", name))
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func readFixture(t testing.TB, name string) *Document {
	t.Helper()
	f, e := FormatFromName(name)
	if e != nil {
		t.Fatal(e)
	}
	d, e := Read(context.Background(), bytes.NewReader(fixture(t, name)), f, ReadOptions{})
	if e != nil {
		t.Fatal(e)
	}
	return d
}

func TestDOCXCorpus(t *testing.T) {
	for _, c := range corpus(t) {
		if c.Kind != "docx" {
			continue
		}
		t.Run(c.Name, func(t *testing.T) {
			d, e := ReadDOCX(context.Background(), bytes.NewReader(fixture(t, c.Name)))
			if c.Expect == "reject" {
				if e == nil {
					t.Fatal("invalid DOCX accepted")
				}
				return
			}
			if e != nil {
				t.Fatal(e)
			}
			if d == nil {
				t.Fatal("nil document")
			}
		})
	}
}
func TestDOCXVisibleContent(t *testing.T) {
	cases := map[string][]string{
		"docx_minimal.docx": {"MINIMAL_MARKER"}, "docx_hyperlink.docx": {"Before Linked text after"},
		"docx_tracked.docx": {"INSERTED_MARKER"}, "docx_content_control.docx": {"CONTROL_MARKER"},
		"docx_textbox.docx": {"TEXTBOX_MARKER"}, "docx_field.docx": {"FIELD_RESULT_MARKER"},
		"docx_bookmark.docx": {"BOOKMARK_MARKER"}, "docx_unicode.docx": {"Ελληνικά", "Привет", "中文", "हिन्दी", "😀", "A&B <tag>"},
		"docx_nested_table.docx": {"Merged heading", "Outer cell", "Nested cell"}, "docx_table.docx": {"Before table", "CELL-0-0", "CELL-2-1", "After table"},
	}
	for name, markers := range cases {
		t.Run(name, func(t *testing.T) {
			d := readFixture(t, name)
			for _, m := range markers {
				if !strings.Contains(d.Text(), m) {
					t.Fatalf("missing %q in %q", m, d.Text())
				}
			}
			if name == "docx_tracked.docx" && strings.Contains(d.Text(), "DELETED_MARKER") {
				t.Fatal("deleted text in final visible view")
			}
		})
	}
}
func TestPDFCorpus(t *testing.T) {
	for _, c := range corpus(t) {
		if c.Kind != "pdf" {
			continue
		}
		t.Run(c.Name, func(t *testing.T) {
			opts := ReadOptions{}
			if c.Expect == "password" {
				opts.Password = "secret"
			}
			d, e := ReadPDF(context.Background(), bytes.NewReader(fixture(t, c.Name)), opts)
			if c.Expect == "reject" {
				if e == nil {
					t.Fatal("invalid PDF accepted")
				}
				return
			}
			if e != nil {
				t.Fatal(e)
			}
			text := d.Text()
			for _, m := range c.Markers {
				if !strings.Contains(text, m) {
					t.Fatalf("missing %q in %.400q", m, text)
				}
			}
			if c.Pages > 0 && d.Metadata.PageCount != c.Pages {
				t.Fatalf("pages=%d, want %d", d.Metadata.PageCount, c.Pages)
			}
			if strings.ContainsRune(text, 0) {
				t.Fatal("NUL in extracted text")
			}
			if c.Name == "pdf_unicode.pdf" && strings.Contains(text, "ПриветП") {
				t.Fatal("spurious font-mapped newline character")
			}
		})
	}
}
func TestPDFWidgetAppearance(t *testing.T) {
	d := readFixture(t, "pdf_form_field.pdf")
	if !strings.Contains(d.Text(), "FIELD_VALUE_MARKER") {
		t.Fatalf("missing field appearance: %q", d.Text())
	}
}
func TestPDFPasswords(t *testing.T) {
	for _, name := range []string{"pdf_rc4_40.pdf", "pdf_rc4_128.pdf", "pdf_aes-128.pdf", "pdf_aes-256.pdf"} {
		for _, password := range []string{"", "wrong"} {
			t.Run(name+password, func(t *testing.T) {
				if _, e := ReadPDF(context.Background(), bytes.NewReader(fixture(t, name)), ReadOptions{Password: password}); e == nil {
					t.Fatal("wrong password accepted")
				}
			})
		}
	}
}
func TestGenerationRoundTrip(t *testing.T) {
	d := &Document{Metadata: Metadata{Title: "A&B report", Author: "Fixture"}, Headers: []Block{{Kind: Paragraph, Runs: []Run{{Text: "Header", Link: "https://example.invalid/"}}}}, Footers: []Block{{Kind: Paragraph, Runs: []Run{{Text: "Footer"}}}}, Blocks: []Block{
		{Kind: Heading, Level: 1, Runs: []Run{{Text: "Report title"}}},
		{Kind: Paragraph, Runs: []Run{{Text: "Bold ", Bold: true}, {Text: "italic ", Italic: true}, {Text: "link", Link: "https://example.invalid/"}}},
		{Kind: Paragraph, List: "bullet", Runs: []Run{{Text: "café Ελληνικά Привет"}}},
		{Kind: Table, Rows: []Row{{Cells: []Cell{{Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "Product"}}}}}, {Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "Quantity"}}}}}}}, {Cells: []Cell{{Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "Daptin"}}}}}, {Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "42"}}}}}}}}},
		{Kind: PageBreak}, {Kind: Paragraph, Runs: []Run{{Text: "Second page"}}},
	}}
	for _, format := range []Format{DOCX, PDF, JSON, Text} {
		t.Run(string(format), func(t *testing.T) {
			var out bytes.Buffer
			if e := Write(context.Background(), &out, d, format, WriteOptions{}); e != nil {
				t.Fatal(e)
			}
			decoded, e := Read(context.Background(), &out, format, ReadOptions{})
			if e != nil {
				t.Fatal(e)
			}
			for _, m := range []string{"Report title", "Bold", "italic", "link", "Ελληνικά", "Привет", "Product", "Daptin", "42", "Second page"} {
				if !strings.Contains(decoded.Text(), m) {
					t.Fatalf("missing %q: %q", m, decoded.Text())
				}
			}
			if format == DOCX {
				if decoded.Metadata.Title != d.Metadata.Title {
					t.Fatal("metadata lost")
				}
				if len(decoded.Headers) != 1 || decoded.Headers[0].Runs[0].Link != "https://example.invalid/" {
					t.Fatal("header link lost")
				}
			}
		})
	}
}
func TestPDFOutputRejectsMissingGlyph(t *testing.T) {
	d := &Document{Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "中文"}}}}}
	var out bytes.Buffer
	if e := Write(context.Background(), &out, d, PDF, WriteOptions{}); !errors.Is(e, ErrUnsupported) {
		t.Fatalf("want unsupported glyph, got %v", e)
	}
	if out.Len() != 0 {
		t.Fatal("partial PDF emitted")
	}
}
func TestDiagnosticsRequireAcknowledgement(t *testing.T) {
	d := readFixture(t, "docx_content_control.docx")
	if len(d.Diagnostics) == 0 {
		t.Fatal("lost control semantics not reported")
	}
	var out bytes.Buffer
	if e := Write(context.Background(), &out, d, DOCX, WriteOptions{}); !errors.Is(e, ErrLossy) {
		t.Fatalf("got %v", e)
	}
	if e := Write(context.Background(), &out, d, DOCX, WriteOptions{AllowLossy: true}); e != nil {
		t.Fatal(e)
	}
}
func TestCancelledOperations(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := Read(ctx, strings.NewReader("x"), Text, ReadOptions{}); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if e := Write(ctx, &bytes.Buffer{}, &Document{}, DOCX, WriteOptions{}); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
}
func TestConcurrentOperations(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 3; j++ {
				d := readFixture(t, "docx_basic.docx")
				for _, f := range []Format{DOCX, PDF} {
					var out bytes.Buffer
					if e := Write(context.Background(), &out, d, f, WriteOptions{AllowLossy: true}); e != nil {
						t.Error(e)
					}
				}
				readFixture(t, "pdf_unicode.pdf")
			}
		}()
	}
	wg.Wait()
}
