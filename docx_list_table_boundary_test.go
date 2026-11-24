package manydoc

import (
	"bytes"
	"context"
	"reflect"
	"strings"
	"testing"
)

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
