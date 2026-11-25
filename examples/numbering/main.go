// Numbering formats, nested edits, external contexts and rich stories via SF.
package main

import (
	"bytes"
	"context"
	"log"
	"os"
	"path/filepath"

	"github.com/artpar/manifold"
)

func main() {
	const out = "output/numbering"
	if err := os.MkdirAll(out, 0755); err != nil {
		log.Fatal(err)
	}
	p := func(s string) manifold.Block {
		return manifold.Block{Kind: manifold.Paragraph, Runs: []manifold.Run{{Text: s}}}
	}
	list := func(format, template, text string, start int) manifold.Block {
		return manifold.Block{Kind: manifold.ListBlock, List: "number", Start: &start, Numbering: &manifold.ListNumbering{Level: 0, Template: template, Formats: []string{format}}, Items: []manifold.ListItem{{Blocks: []manifold.Block{p(text)}}}}
	}
	parent := list("upperLetter", "Appendix %1.", "PARENT", 27)
	child := list("lowerRoman", "%1.%2)", "CHILD", 4)
	child.Numbering.Level = 1
	child.Numbering.Formats = []string{"decimal", "lowerRoman"}
	child.Numbering.Ancestors = []int{99}
	parent.Items[0].Blocks = append(parent.Items[0].Blocks, child)
	parent.Items = append(parent.Items, manifold.ListItem{Blocks: []manifold.Block{p("NEXT PARENT"), child}})
	external := list("decimalZero", "%1.%2.%3", "LEGAL EXTERNAL", 7)
	external.Numbering = &manifold.ListNumbering{Level: 2, Template: "%1.%2.%3", Formats: []string{"upperLetter", "lowerRoman", "decimalZero"}, Ancestors: []int{2, 4}, Legal: true}
	note := list("lowerLetter", "(%1)", "NOTE LIST", 28)
	d := &manifold.Document{Layout: manifold.Layout{Width: 400, Height: 500, Margin: 40}, Headers: []manifold.Block{list("decimalZero", "H%1:", "HEADER", 1)}, Footers: []manifold.Block{list("lowerRoman", "F%1:", "FOOTER", 9)},
		Notes: []manifold.Note{{Kind: "footnote", ID: "n", Blocks: []manifold.Block{note}}},
		Blocks: []manifold.Block{p("NUMBERING FORMATS"), parent, external, list("upperRoman", "[%1]", "ROMAN 4000", 4000),
			{Kind: manifold.Table, ColumnWidths: []float64{150, 150}, Rows: []manifold.Row{{Cells: []manifold.Cell{{Blocks: []manifold.Block{list("lowerLetter", "(%1)", "CELL LIST", 53)}}, {Blocks: []manifold.Block{p("CELL VALUE")}}}}}},
			{Kind: manifold.Paragraph, Runs: []manifold.Run{{Text: "REFERENCE "}, {Note: &manifold.NoteReference{Kind: "footnote", ID: "n"}}}},
		}}
	ctx := context.Background()
	write := func(name string, d *manifold.Document, f manifold.Format) []byte {
		var b bytes.Buffer
		if err := manifold.Write(ctx, &b, d, f, manifold.WriteOptions{AllowLossy: true}); err != nil {
			log.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(out, name), b.Bytes(), 0644); err != nil {
			log.Fatal(err)
		}
		return b.Bytes()
	}
	source := write("source.sf", d, manifold.SF)
	var err error
	d, err = manifold.Read(ctx, bytes.NewReader(source), manifold.SF, manifold.ReadOptions{})
	if err != nil {
		log.Fatal(err)
	}
	for _, f := range []manifold.Format{manifold.DOCX, manifold.PDF} {
		raw := write("numbering."+string(f), d, f)
		back, err := manifold.Read(ctx, bytes.NewReader(raw), f, manifold.ReadOptions{PDFSkipInference: true})
		if err != nil {
			log.Fatal(err)
		}
		write("from-"+string(f)+".sf", back, manifold.SF)
	}
}
