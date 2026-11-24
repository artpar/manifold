// Numbering formats, nested edits, external contexts and rich stories via SF.
package main

import (
	"bytes"
	"context"
	"log"
	"os"
	"path/filepath"

	"github.com/artpar/manydoc"
)

func main() {
	const out = "output/numbering"
	if err := os.MkdirAll(out, 0755); err != nil {
		log.Fatal(err)
	}
	p := func(s string) manydoc.Block {
		return manydoc.Block{Kind: manydoc.Paragraph, Runs: []manydoc.Run{{Text: s}}}
	}
	list := func(format, template, text string, start int) manydoc.Block {
		return manydoc.Block{Kind: manydoc.ListBlock, List: "number", Start: &start, Numbering: &manydoc.ListNumbering{Level: 0, Template: template, Formats: []string{format}}, Items: []manydoc.ListItem{{Blocks: []manydoc.Block{p(text)}}}}
	}
	parent := list("upperLetter", "Appendix %1.", "PARENT", 27)
	child := list("lowerRoman", "%1.%2)", "CHILD", 4)
	child.Numbering.Level = 1
	child.Numbering.Formats = []string{"decimal", "lowerRoman"}
	child.Numbering.Ancestors = []int{99}
	parent.Items[0].Blocks = append(parent.Items[0].Blocks, child)
	parent.Items = append(parent.Items, manydoc.ListItem{Blocks: []manydoc.Block{p("NEXT PARENT"), child}})
	external := list("decimalZero", "%1.%2.%3", "LEGAL EXTERNAL", 7)
	external.Numbering = &manydoc.ListNumbering{Level: 2, Template: "%1.%2.%3", Formats: []string{"upperLetter", "lowerRoman", "decimalZero"}, Ancestors: []int{2, 4}, Legal: true}
	note := list("lowerLetter", "(%1)", "NOTE LIST", 28)
	d := &manydoc.Document{Layout: manydoc.Layout{Width: 400, Height: 500, Margin: 40}, Headers: []manydoc.Block{list("decimalZero", "H%1:", "HEADER", 1)}, Footers: []manydoc.Block{list("lowerRoman", "F%1:", "FOOTER", 9)},
		Notes: []manydoc.Note{{Kind: "footnote", ID: "n", Blocks: []manydoc.Block{note}}},
		Blocks: []manydoc.Block{p("NUMBERING FORMATS"), parent, external, list("upperRoman", "[%1]", "ROMAN 4000", 4000),
			{Kind: manydoc.Table, ColumnWidths: []float64{150, 150}, Rows: []manydoc.Row{{Cells: []manydoc.Cell{{Blocks: []manydoc.Block{list("lowerLetter", "(%1)", "CELL LIST", 53)}}, {Blocks: []manydoc.Block{p("CELL VALUE")}}}}}},
			{Kind: manydoc.Paragraph, Runs: []manydoc.Run{{Text: "REFERENCE "}, {Note: &manydoc.NoteReference{Kind: "footnote", ID: "n"}}}},
		}}
	ctx := context.Background()
	write := func(name string, d *manydoc.Document, f manydoc.Format) []byte {
		var b bytes.Buffer
		if err := manydoc.Write(ctx, &b, d, f, manydoc.WriteOptions{AllowLossy: true}); err != nil {
			log.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(out, name), b.Bytes(), 0644); err != nil {
			log.Fatal(err)
		}
		return b.Bytes()
	}
	source := write("source.sf", d, manydoc.SF)
	var err error
	d, err = manydoc.Read(ctx, bytes.NewReader(source), manydoc.SF, manydoc.ReadOptions{})
	if err != nil {
		log.Fatal(err)
	}
	for _, f := range []manydoc.Format{manydoc.DOCX, manydoc.PDF} {
		raw := write("numbering."+string(f), d, f)
		back, err := manydoc.Read(ctx, bytes.NewReader(raw), f, manydoc.ReadOptions{PDFSkipInference: true})
		if err != nil {
			log.Fatal(err)
		}
		write("from-"+string(f)+".sf", back, manydoc.SF)
	}
}
