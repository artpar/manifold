// Compatible typography keeps context across color, decorations and links.
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
	out := "output/paint-context"
	if err := os.MkdirAll(out, 0755); err != nil {
		log.Fatal(err)
	}
	font, err := os.ReadFile("testdata/assets/NotoSans-Regular.ttf")
	if err != nil {
		log.Fatal(err)
	}
	run := func(text, color string) manydoc.Run {
		return manydoc.Run{Text: text, FontFamily: "Noto Sans", FontSize: 32, Color: color}
	}
	red, blue, green := "#FF0000", "#0000FF", "#008800"
	a, v := run("A", red), run("V", blue)
	o, f, i, ce := run("of", red), run("f", blue), run("i", green), run("ce", "#000000")
	o.Underline = true
	f.Strike = true
	f.Link = "https://example.invalid/ligature"
	paragraph := func(runs ...manydoc.Run) manydoc.Block { return manydoc.Block{Kind: manydoc.Paragraph, Runs: runs} }
	header := paragraph(run("A", red), run("V", blue))
	for j := range header.Runs {
		header.Runs[j].FontSize = 10
	}
	doc := &manydoc.Document{Metadata: manydoc.Metadata{Title: "Paint preserves shaping"}, Layout: manydoc.Layout{Width: 360, Height: 420, Margin: 40}, Headers: []manydoc.Block{header}, Footers: []manydoc.Block{header}, Blocks: []manydoc.Block{paragraph(a, v), paragraph(o, f, i, ce), {Kind: manydoc.Table, Rows: []manydoc.Row{{Cells: []manydoc.Cell{{Blocks: []manydoc.Block{paragraph(o, f, i, ce)}}}}}}}}
	opts := manydoc.WriteOptions{PDFFonts: []manydoc.FontFace{{Family: "Noto Sans", Data: font}}}
	write := func(name string, d *manydoc.Document, format manydoc.Format) {
		var b bytes.Buffer
		if err := manydoc.Write(context.Background(), &b, d, format, opts); err != nil {
			log.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(out, name), b.Bytes(), 0644); err != nil {
			log.Fatal(err)
		}
	}
	for _, format := range []manydoc.Format{manydoc.SF, manydoc.DOCX, manydoc.PDF} {
		write("paint."+string(format), doc, format)
	}
	emergency := &manydoc.Document{Layout: manydoc.Layout{Width: 32, Height: 220, Margin: 10}, Blocks: []manydoc.Block{paragraph(run("f", red), run("f", blue), run("i", green))}}
	write("emergency.sf", emergency, manydoc.SF)
	write("emergency.pdf", emergency, manydoc.PDF)
}
