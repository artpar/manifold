package manifold

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

func mergeParagraph(text string) Block { return Block{Kind: Paragraph, Runs: []Run{{Text: text}}} }
func mergedTableExample() Block {
	b := Block{Kind: Table, Rows: []Row{
		{Cells: []Cell{
			{ColSpan: 2, RowSpan: 2, Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "Merged A", Bold: true, Link: "https://example.invalid/merge"}}}}},
			{RowSpan: 3, Blocks: []Block{mergeParagraph("Merged B")}},
			{Blocks: []Block{mergeParagraph("Right C")}},
		}},
		{Cells: []Cell{{Blocks: []Block{mergeParagraph("Right D")}}}},
		{Cells: []Cell{{ColSpan: 2, Blocks: []Block{mergeParagraph("Bottom E")}}, {Blocks: []Block{mergeParagraph("Right F")}}}},
		{Cells: []Cell{{ColSpan: 4, Blocks: []Block{mergeParagraph("Full G")}}}},
	}}
	for ri := range b.Rows {
		for ci := range b.Rows[ri].Cells {
			c := &b.Rows[ri].Cells[ci]
			c.ColSpan = max(1, c.ColSpan)
		}
	}
	return b
}

// Independent, hand-authored OOXML. In particular, continuation cells are
// present here although they must be omitted from the SF rows.
const mergedTableOOXML = `<w:tbl><w:tblGrid><w:gridCol/><w:gridCol/><w:gridCol/><w:gridCol/></w:tblGrid>
<w:tr><w:tc><w:tcPr><w:gridSpan w:val="2"/><w:vMerge w:val="restart"/></w:tcPr><w:p><w:r><w:t>Merged A</w:t></w:r></w:p></w:tc>
<w:tc><w:tcPr><w:vMerge w:val="restart"/></w:tcPr><w:p><w:r><w:t>Merged B</w:t></w:r></w:p></w:tc>
<w:tc><w:p><w:r><w:t>Right C</w:t></w:r></w:p></w:tc></w:tr>
<w:tr><w:tc><w:tcPr><w:gridSpan w:val="2"/><w:vMerge/></w:tcPr><w:p/></w:tc>
<w:tc><w:tcPr><w:vMerge w:val="continue"/></w:tcPr><w:p/></w:tc>
<w:tc><w:p><w:r><w:t>Right D</w:t></w:r></w:p></w:tc></w:tr>
<w:tr><w:tc><w:tcPr><w:gridSpan w:val="2"/></w:tcPr><w:p><w:r><w:t>Bottom E</w:t></w:r></w:p></w:tc>
<w:tc><w:tcPr><w:vMerge/></w:tcPr><w:p/></w:tc>
<w:tc><w:p><w:r><w:t>Right F</w:t></w:r></w:p></w:tc></w:tr>
<w:tr><w:tc><w:tcPr><w:gridSpan w:val="4"/></w:tcPr><w:p><w:r><w:t>Full G</w:t></w:r></w:p></w:tc></w:tr></w:tbl>`

func docxWithBody(t testing.TB, body string) []byte {
	t.Helper()
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	parts := map[string]string{
		"[Content_Types].xml": `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/></Types>`,
		"_rels/.rels":         `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="` + relNS + `/officeDocument" Target="word/document.xml"/></Relationships>`,
		"word/document.xml":   `<w:document xmlns:w="` + wordNS + `"><w:body>` + body + `</w:body></w:document>`,
	}
	for name, value := range parts {
		w, err := z.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = w.Write([]byte(value)); err != nil {
			t.Fatal(err)
		}
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestDOCXVerticalMergeImport(t *testing.T) {
	d, err := ReadDOCX(context.Background(), bytes.NewReader(docxWithBody(t, mergedTableOOXML)))
	if err != nil {
		t.Fatal(err)
	}
	b := d.Blocks[0]
	if len(b.Rows) != 4 || len(b.Rows[1].Cells) != 1 || len(b.Rows[2].Cells) != 2 {
		t.Fatalf("continuation cells not folded: %#v", b.Rows)
	}
	if b.Rows[0].Cells[0].RowSpan != 2 || b.Rows[0].Cells[0].ColSpan != 2 || b.Rows[0].Cells[1].RowSpan != 3 {
		t.Fatal("merge geometry lost")
	}
	for _, diag := range d.Diagnostics {
		if diag.Code == "vertical_merge" {
			t.Fatal("supported merge was diagnosed as lost")
		}
	}
	s, err := ToStructured(d)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(s.Content[0].Content[0].Content[0].Attrs), `"rowspan":2`) {
		t.Fatal("SF rowspan lost")
	}
	if len(s.Content[0].Content[1].Content) != 1 {
		t.Fatal("SF duplicated continuation cells")
	}
	for _, marker := range []string{"Merged A", "Merged B", "Right C", "Right D", "Bottom E", "Right F", "Full G"} {
		if strings.Count(d.Text(), marker) != 1 {
			t.Fatalf("lost or repeated %s", marker)
		}
	}
	// A fully covered row still exists even though its SF Cells are empty.
	full := `<w:tbl><w:tr><w:tc><w:tcPr><w:vMerge w:val="restart"/></w:tcPr><w:p><w:r><w:t>All rows</w:t></w:r></w:p></w:tc></w:tr><w:tr><w:tc><w:tcPr><w:vMerge/></w:tcPr><w:p/></w:tc></w:tr></w:tbl>`
	d, err = ReadDOCX(context.Background(), bytes.NewReader(docxWithBody(t, full)))
	if err != nil {
		t.Fatal(err)
	}
	if d.Blocks[0].Rows[0].Cells[0].RowSpan != 2 || len(d.Blocks[0].Rows[1].Cells) != 0 {
		t.Fatal("covered row lost")
	}
	for _, format := range []Format{SF, DOCX, PDF} {
		var out bytes.Buffer
		if err := Write(context.Background(), &out, d, format, WriteOptions{}); err != nil {
			t.Fatal(err)
		}
		decoded, err := Read(context.Background(), &out, format, ReadOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if strings.Count(decoded.Text(), "All rows") != 1 {
			t.Fatal("fully merged cell content duplicated or lost")
		}
		if format != PDF && (len(decoded.Blocks[0].Rows) != 2 || decoded.Blocks[0].Rows[0].Cells[0].RowSpan != 2 || len(decoded.Blocks[0].Rows[1].Cells) != 0) {
			t.Fatal("fully covered row changed on round trip")
		}
	}
}

func TestVerticalMergeRoundTrip(t *testing.T) {
	d := &Document{Blocks: []Block{mergedTableExample()}}
	s, err := ToStructured(d)
	if err != nil {
		t.Fatal(err)
	}
	shared, err := FromStructured(s)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(d, shared) {
		t.Fatal("SF merge/formatting/content changed")
	}
	var encoded bytes.Buffer
	if err = Write(context.Background(), &encoded, shared, DOCX, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	z, err := zip.NewReader(bytes.NewReader(encoded.Bytes()), int64(encoded.Len()))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range z.File {
		if f.Name == "word/document.xml" {
			rc, err := f.Open()
			if err != nil {
				t.Fatal(err)
			}
			xml, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				t.Fatal(err)
			}
			if strings.Count(string(xml), `<w:vMerge w:val="restart"/>`) != 2 || strings.Count(string(xml), `<w:vMerge/>`) != 3 {
				t.Fatalf("incorrect OOXML merge markers: %s", xml)
			}
			for _, m := range []string{"Merged A", "Merged B"} {
				if strings.Count(string(xml), m) != 1 {
					t.Fatal("origin content duplicated")
				}
			}
		}
	}
	decoded, err := ReadDOCX(context.Background(), &encoded)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := ToStructured(decoded)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(s.Content, actual.Content) {
		t.Fatal("DOCX merge/formatting/relationship round trip changed")
	}
}

func TestMergeContinuationContentAndInvalidDOCX(t *testing.T) {
	extra := strings.Replace(mergedTableOOXML, `<w:vMerge/></w:tcPr><w:p/>`, `<w:vMerge/></w:tcPr><w:p><w:r><w:t>Continuation text</w:t></w:r></w:p>`, 1)
	d, err := ReadDOCX(context.Background(), bytes.NewReader(docxWithBody(t, extra)))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(d.Blocks[0].Rows[0].Cells[0].Blocks[1].Runs[0].Text, "Continuation text") || len(d.Diagnostics) == 0 || d.Diagnostics[0].Code != "merge_continuation_content" {
		t.Fatal("continuation content silently discarded")
	}
	closed := `<w:tbl><w:tr><w:tc><w:tcPr><w:vMerge w:val="restart"/></w:tcPr><w:p/></w:tc></w:tr><w:tr><w:tc><w:p/></w:tc></w:tr><w:tr><w:tc><w:tcPr><w:vMerge/></w:tcPr><w:p/></w:tc></w:tr></w:tbl>`
	for _, bad := range []string{closed,
		strings.Replace(mergedTableOOXML, `w:val="restart"`, `w:val="continue"`, 1),
		strings.Replace(mergedTableOOXML, `w:val="restart"`, `w:val="bogus"`, 1),
		strings.Replace(mergedTableOOXML, `<w:gridSpan w:val="2"/><w:vMerge/>`, `<w:gridSpan w:val="1"/><w:vMerge/>`, 1),
		strings.Replace(mergedTableOOXML, `<w:gridSpan w:val="2"/>`, `<w:gridSpan w:val="0"/>`, 1),
	} {
		if _, err := ReadDOCX(context.Background(), bytes.NewReader(docxWithBody(t, bad))); err == nil {
			t.Fatal("invalid DOCX merge accepted")
		}
	}
}

func TestTableGridRejectsInvalidSpans(t *testing.T) {
	for _, rows := range [][]Row{
		{{Cells: []Cell{{RowSpan: 2}}}},
		{{Cells: []Cell{{RowSpan: -1}}}},
		{{Cells: []Cell{{ColSpan: 1001}}}},
		{{Cells: []Cell{{}, {RowSpan: 2}}}, {Cells: []Cell{{ColSpan: 2}}}}, // crosses occupied second column
		{{Cells: []Cell{{}, {RowSpan: 2}}}, {}},                            // missing first-column cell
	} {
		if err := (&Document{Blocks: []Block{{Kind: Table, Rows: rows}}}).Validate(); err == nil {
			t.Fatal("invalid table accepted")
		}
	}
}

func TestPDFMergedCellGeometryAndPagination(t *testing.T) {
	ctx := context.Background()
	d := &Document{Layout: Layout{Width: 360, Height: 300, Margin: 30}, Blocks: []Block{mergedTableExample()}}
	var out bytes.Buffer
	if err := Write(ctx, &out, d, PDF, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	p, err := pdfcpu.ReadWithContext(ctx, bytes.NewReader(out.Bytes()), &model.Configuration{Reader15: true, ValidationMode: model.ValidationStrict, Offline: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := p.EnsurePageCount(); err != nil {
		t.Fatal(err)
	}
	pd, _, _, err := p.PageDict(1, false)
	if err != nil {
		t.Fatal(err)
	}
	data, err := p.PageContent(pd, 1)
	if err != nil {
		t.Fatal(err)
	}
	stream := string(data)
	var operands []any
	var rects [][]float64
	for stream != "" {
		token, err := nextPDFToken(ctx, &stream)
		if err != nil {
			t.Fatal(err)
		}
		if token == nil {
			break
		}
		if op, ok := token.(pdfOperator); ok {
			if op == "re" {
				values := make([]float64, len(operands))
				for i, v := range operands {
					var err error
					values[i], err = pdfNumber(v)
					if err != nil {
						t.Fatal(err)
					}
				}
				rects = append(rects, values)
			}
			operands = nil
		} else {
			operands = append(operands, token)
		}
	}
	want := [][]float64{{30, 270, 150, -52}, {180, 270, 75, -78}, {255, 270, 75, -26}, {255, 244, 75, -26}, {30, 218, 150, -26}, {255, 218, 75, -26}, {30, 192, 300, -26}}
	if len(rects) != len(want) {
		t.Fatalf("got %d rectangles, want 7", len(rects))
	}
	for i := range want {
		for j, v := range want[i] {
			if math.Abs(rects[i][j]-v) > .01 {
				t.Fatalf("rectangle %d: got %v, want %v", i, rects[i], want[i])
			}
		}
	}
	// Move the connected 3-row merged region as a whole to the next page.
	d.Layout = Layout{Width: 360, Height: 180, Margin: 20}
	d.Blocks = append([]Block{mergeParagraph(strings.Repeat("Preface\n", 5))}, d.Blocks...)
	out.Reset()
	if err := Write(ctx, &out, d, PDF, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	decoded, err := ReadPDF(ctx, &out, ReadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Metadata.PageCount != 2 {
		t.Fatalf("unexpected pagination: %d", decoded.Metadata.PageCount)
	}
	for _, m := range []string{"Merged A", "Merged B", "Right C", "Right D", "Bottom E", "Right F", "Full G"} {
		if strings.Count(decoded.Text(), m) != 1 {
			t.Fatalf("lost/repeated text across pages: %s", m)
		}
	}
	d.Layout.Height = 90 // The connected group now continues over several pages.
	out.Reset()
	if err := Write(ctx, &out, d, PDF, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	decoded, err = ReadPDF(ctx, &out, ReadOptions{PDFSkipInference: true})
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Metadata.PageCount < 3 {
		t.Fatal("oversized group did not continue")
	}
	for _, m := range []string{"Merged A", "Merged B", "Right C", "Right D", "Bottom E", "Right F", "Full G"} {
		if strings.Count(decoded.Text(), m) != 1 {
			t.Fatalf("lost/repeated continued-cell text: %s", m)
		}
	}
}

func TestDOCXTableWidthsStayInsideContainer(t *testing.T) {
	nested := Block{Kind: Table, Rows: []Row{{Cells: []Cell{{Blocks: []Block{mergeParagraph("Nested left")}}, {Blocks: []Block{mergeParagraph("Nested right")}}}}}}
	outer := Block{Kind: Table, Rows: []Row{{Cells: []Cell{{Blocks: []Block{nested}}, {Blocks: []Block{mergeParagraph("Outer right")}}}}}}
	d := &Document{Layout: Layout{Width: 360, Height: 300, Margin: 30}, Blocks: []Block{outer}}
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, DOCX, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	z, err := zip.NewReader(bytes.NewReader(out.Bytes()), int64(out.Len()))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range z.File {
		if f.Name == "word/document.xml" {
			rc, err := f.Open()
			if err != nil {
				t.Fatal(err)
			}
			raw, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				t.Fatal(err)
			}
			root, err := parseXML(context.Background(), raw)
			if err != nil {
				t.Fatal(err)
			}
			tables := root.all("tbl")
			if len(tables) != 2 {
				t.Fatal("nested table lost")
			}
			for i, want := range []string{"6000", "2840"} {
				if tables[i].child("tblPr").child("tblW").attr("w") != want {
					t.Fatalf("table %d overflows available container: %s", i, raw)
				}
				total := 0.0
				for _, col := range tables[i].child("tblGrid").Children {
					total += number(col.attr("w"))
				}
				if total != number(want) {
					t.Fatal("grid width does not match available width")
				}
			}
		}
	}
}
