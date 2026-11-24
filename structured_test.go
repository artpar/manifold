package manydoc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestStructuredSemantics(t *testing.T) {
	p := func(s string) Block { return Block{Kind: Paragraph, Runs: []Run{{Text: s}}} }
	d := &Document{Metadata: Metadata{Title: "Tree", Author: "Author", Subject: "Subject", PageCount: 2}, Layout: Layout{Width: 612, Height: 792, Margin: 40},
		Headers: []Block{p("header")}, Footers: []Block{p("footer")}, Notes: []Note{{ID: "42", Kind: "footnote", Blocks: []Block{p("note")}}},
		Diagnostics: []Diagnostic{{Code: "source", Message: "source limitation"}},
		Blocks: []Block{
			{Kind: Heading, Level: 2, Runs: []Run{{Text: "Title"}}},
			{Kind: Paragraph, Runs: []Run{{Text: "all marks", Bold: true, Italic: true, Underline: true, Strike: true, Link: "https://example.invalid/?x=1&y=2"}}},
			{Kind: ListBlock, List: "number", Items: []ListItem{{Blocks: []Block{p("one")}}, {Blocks: []Block{p("two")}}}},
			{Kind: ListBlock, List: "bullet", Items: []ListItem{{Blocks: []Block{p("bullet")}}}},
			{Kind: Table, Rows: []Row{{Cells: []Cell{{ColSpan: 2, Blocks: []Block{p("merged"), {Kind: Table, Rows: []Row{{Cells: []Cell{{ColSpan: 1, Blocks: []Block{p("nested")}}}}}}}}}}}},
			{Kind: Picture, Image: &Image{Data: []byte{0, 1, 2, 255}, MediaType: "image/png", Alt: "asset", Width: 30, Height: 20}},
			{Kind: PageBreak}, p("second page"),
		}}
	s, err := ToStructured(d)
	if err != nil {
		t.Fatal(err)
	}
	if s.Content[1].Content[0].Type != "text" || len(s.Content[1].Content[0].Marks) != 5 {
		t.Fatal("formatted text lost")
	}
	if s.Content[2].Type != "ordered_list" || len(s.Content[2].Content) != 2 {
		t.Fatal("list grouping lost")
	}
	if s.Content[4].Content[0].Content[0].Type != "table_cell" {
		t.Fatal("table structure lost")
	}
	var encoded bytes.Buffer
	if err = WriteStructured(context.Background(), &encoded, s); err != nil {
		t.Fatal(err)
	}
	parsed, err := ReadStructured(context.Background(), &encoded)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := FromStructured(parsed)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(d, actual) {
		t.Fatalf("semantics changed\nwant %#v\ngot %#v", d, actual)
	}
	// Unknown source diagnostics survive SF, and continue to gate rendered output.
	if err = Write(context.Background(), &bytes.Buffer{}, actual, DOCX, WriteOptions{}); !errors.Is(err, ErrLossy) {
		t.Fatalf("diagnostics bypassed: %v", err)
	}
}

func TestStructuredCorpusRoundTrips(t *testing.T) {
	// The corpus is independently fixed. Compare every model field, including
	// table nesting, links, images, notes, headers, metadata and diagnostics.
	for _, c := range corpus(t) {
		if c.Expect == "reject" {
			continue
		}
		t.Run(c.Name, func(t *testing.T) {
			f, err := FormatFromName(c.Name)
			if err != nil {
				t.Fatal(err)
			}
			d, err := Read(context.Background(), bytes.NewReader(fixture(t, c.Name)), f, ReadOptions{Password: "secret"})
			if err != nil {
				t.Fatal(err)
			}
			s, err := ToStructured(d)
			if err != nil {
				t.Fatal(err)
			}
			got, err := FromStructured(s)
			if err != nil {
				t.Fatal(err)
			}
			// Span zero and span one have identical semantics; empty runs have none.
			var normalize func([]Block)
			normalize = func(bs []Block) {
				for i := range bs {
					b := &bs[i]
					runs := b.Runs[:0]
					for _, r := range b.Runs {
						if r.Text != "" || r.Note != nil {
							runs = append(runs, r)
						}
					}
					b.Runs = runs
					if len(b.Runs) == 0 {
						b.Runs = nil
					}
					if len(b.Rows) == 0 {
						b.Rows = nil
					}
					for ri := range b.Rows {
						for ci := range b.Rows[ri].Cells {
							c := &b.Rows[ri].Cells[ci]
							if c.ColSpan == 0 {
								c.ColSpan = 1
							}
							normalize(c.Blocks)
							if len(c.Blocks) == 0 {
								c.Blocks = nil
							}
						}
					}
				}
			}
			for _, v := range []*Document{d, got} {
				normalize(v.Blocks)
				normalize(v.Headers)
				normalize(v.Footers)
				for i := range v.Notes {
					normalize(v.Notes[i].Blocks)
				}
			}
			// JSON comparison ignores nil versus empty slices under omitempty only.
			wantJSON, _ := json.Marshal(d)
			gotJSON, _ := json.Marshal(got)
			if !bytes.Equal(wantJSON, gotJSON) {
				t.Fatalf("SF lost model semantics\nwant %s\ngot %s", wantJSON, gotJSON)
			}
		})
	}
}

func TestStructuredRejectsUnrepresentedSemantics(t *testing.T) {
	for _, body := range []string{
		`{"type":"paragraph","attrs":{"align":"center"}}`,
		`{"type":"paragraph","content":[{"type":"text","text":"x","marks":[{"type":"code"}]}]}`,
		`{"type":"paragraph","content":[{"type":"text","text":"x","marks":[{"type":"strong"},{"type":"strong"}]}]}`,
		`{"type":"paragraph","content":[{"type":"text","text":"x","marks":[{"type":"link","attrs":{"href":"x","title":"lost"}}]}]}`,
		`{"type":"paragraph","content":[{"type":"image","attrs":{"src":"https://example.invalid/image"}}]}`,
		`{"type":"image","attrs":{"src":"https://example.invalid/image"}}`,
		`{"type":"image","attrs":{"src":"data:image/png;base64,%%%"}}`,
		`{"type":"image","content":[{"type":"paragraph"}]}`,
		`{"type":"ordered_list","attrs":{"order":-1},"content":[{"type":"list_item","content":[{"type":"paragraph"}]}]}`,
		`{"type":"bullet_list","content":[]}`,
		`{"type":"table","content":[{"type":"table_row","content":[{"type":"table_cell","attrs":{"rowspan":2}}]}]}`,
		`{"type":"table","content":[{"type":"table_row","content":[{"type":"table_cell","attrs":{"colspan":0}}]}]}`,
		`{"type":"page_break","text":"hidden"}`,
		`{"type":"page_break","content":[{"type":"paragraph"}]}`,
		`{"type":"paragraph","attrs":null}`,
		`{"type":"paragraph","attrs":[]}`,
		`{"type":"heading","attrs":{"level":0}}`,
		`{"type":"paragraph","content":[{"type":"text","text":""}]}`,
		`{"type":"mystery","content":[{"type":"paragraph"}]}`,
	} {
		input := `{"type":"doc","attrs":{"schema":"` + StructuredSchema + `"},"content":[` + body + `]}`
		if _, err := ReadStructured(context.Background(), strings.NewReader(input)); err == nil {
			t.Errorf("accepted %s", body)
		}
	}
	for _, input := range []string{`null`, `{}`, `{"type":"doc","attrs":{"schema":"future"}}`, `{"type":"doc","attrs":{"schema":"` + StructuredSchema + `","unknown":1}}`, `{"type":"doc","attrs":{"schema":"` + StructuredSchema + `"},"unknown":true}`} {
		if _, err := ReadStructured(context.Background(), strings.NewReader(input)); err == nil {
			t.Errorf("accepted %s", input)
		}
	}
	s, _ := ToStructured(&Document{})
	b, _ := json.Marshal(s)
	if _, err := ReadStructured(context.Background(), strings.NewReader(string(b)+"{}")); err == nil {
		t.Fatal("trailing JSON accepted")
	}
	if _, err := ReadStructured(context.Background(), nil); err == nil {
		t.Fatal("nil reader accepted")
	}
	if err := WriteStructured(context.Background(), nil, s); err == nil {
		t.Fatal("nil writer accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ReadStructured(ctx, strings.NewReader(string(b))); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err := WriteStructured(ctx, &bytes.Buffer{}, s); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestStructuredFourPaths(t *testing.T) {
	ctx := context.Background()
	d := readFixture(t, "docx_image.docx")
	var sf bytes.Buffer
	if err := Write(ctx, &sf, d, SF, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	shared, err := Read(ctx, bytes.NewReader(sf.Bytes()), SF, ReadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []Format{DOCX, PDF} {
		var out bytes.Buffer
		if err := Write(ctx, &out, shared, f, WriteOptions{AllowLossy: true, PDFFontMap: map[string]string{"Calibri": "Go", "Cambria": "Go"}}); err != nil {
			t.Fatal(err)
		}
		imported, err := Read(ctx, &out, f, ReadOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if err := Write(ctx, &bytes.Buffer{}, imported, SF, WriteOptions{}); err != nil {
			t.Fatal(err)
		}
		// DOCX must retain the binary asset, not just a placeholder or text.
		if f == DOCX {
			a, _ := ToStructured(shared)
			b, _ := ToStructured(imported)
			if !reflect.DeepEqual(a.Content, b.Content) {
				t.Fatal("DOCX structure or asset changed")
			}
		}
	}
}

func TestStructuredListBoundaryRoundTrip(t *testing.T) {
	s, err := ToStructured(&Document{Blocks: []Block{{Kind: Paragraph, List: "number", Runs: []Run{{Text: "one"}}}}})
	if err != nil {
		t.Fatal(err)
	}
	s.Content = append(s.Content, s.Content[0])
	d, err := FromStructured(s)
	if err != nil {
		t.Fatal(err)
	}
	got, err := ToStructured(d)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Content) != 2 || !reflect.DeepEqual(s.Content, got.Content) {
		t.Fatal("adjacent list boundary lost")
	}

}

func FuzzReadStructured(f *testing.F) {
	var destinations bytes.Buffer
	if err := Write(context.Background(), &destinations, destinationDocument(), SF, WriteOptions{}); err != nil {
		f.Fatal(err)
	}
	f.Add(destinations.Bytes())

	tagged, tagErr := ReadPDF(context.Background(), bytes.NewReader(taggedTestPDF(markedText(0, 20, 300, "Heading"), "<< /Type /StructTreeRoot /K 8 0 R /RoleMap << /Chapter /H2 >> >>", "<< /Type /StructElem /S /Chapter /P 7 0 R /Pg 3 0 R /K 0 >>")), ReadOptions{})
	if tagErr != nil {
		f.Fatal(tagErr)
	}
	var taggedSF bytes.Buffer
	if tagErr = Write(context.Background(), &taggedSF, tagged, SF, WriteOptions{}); tagErr != nil {
		f.Fatal(tagErr)
	}
	f.Add(taggedSF.Bytes())

	recovered, e := ReadPDF(context.Background(), bytes.NewReader(recoveryPage("BT /F 10 Tf 10 100 Td /P << /MCID 0 /ActualText (replacement) >> BDC (A) Tj EMC ET", "/CropBox [20 20 220 320] /Rotate 90 /UserUnit 2")), ReadOptions{})
	if e != nil {
		f.Fatal(e)
	}
	var positioned bytes.Buffer
	if e = Write(context.Background(), &positioned, recovered, SF, WriteOptions{}); e != nil {
		f.Fatal(e)
	}
	f.Add(positioned.Bytes())

	b, err := os.ReadFile("examples/merged-tables/document.sf")
	if err != nil {
		f.Fatal(err)
	}
	f.Add(b)
	headers, err := os.ReadFile("examples/repeated-headers/document.sf")
	if err != nil {
		f.Fatal(err)
	}
	f.Add(headers)
	styled, err := os.ReadFile("examples/text-style/document.sf")
	if err != nil {
		f.Fatal(err)
	}
	f.Add(styled)
	lists, err := os.ReadFile("examples/lists/document.sf")
	if err != nil {
		f.Fatal(err)
	}
	f.Add(lists)
	notes, err := os.ReadFile("examples/notes/document.sf")
	if err != nil {
		f.Fatal(err)
	}
	f.Add(notes)
	f.Add([]byte(`{"type":"doc","attrs":{"schema":"manydoc-prosemirror/1"},"content":[{"type":"paragraph","content":[{"type":"text","text":"hello"}]}]}`))
	f.Add([]byte(`{"type":"doc","attrs":{"schema":"manydoc-prosemirror/1"},"content":[{"type":"image","attrs":{"src":"data:image/png;base64,AA=="}}]}`))
	f.Fuzz(func(t *testing.T, b []byte) {
		s, err := ReadStructured(context.Background(), bytes.NewReader(b))
		if err != nil {
			return
		}
		if err = WriteStructured(context.Background(), &bytes.Buffer{}, s); err != nil {
			t.Fatal(err)
		}
		d, err := FromStructured(s)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = ToStructured(d); err != nil {
			t.Fatal(err)
		}
	})
}
