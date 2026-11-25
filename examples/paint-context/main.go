// Compatible typography keeps context across color, decorations and links.
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
	out := "output/paint-context"
	if err := os.MkdirAll(out, 0755); err != nil {
		log.Fatal(err)
	}
	font, err := os.ReadFile("testdata/assets/NotoSans-Regular.ttf")
	if err != nil {
		log.Fatal(err)
	}
	run := func(text, color string) manifold.Run {
		return manifold.Run{Text: text, FontFamily: "Noto Sans", FontSize: 32, Color: color}
	}
	red, blue, green := "#FF0000", "#0000FF", "#008800"
	a, v := run("A", red), run("V", blue)
	o, f, i, ce := run("of", red), run("f", blue), run("i", green), run("ce", "#000000")
	o.Underline = true
	f.Strike = true
	f.Link = "https://example.invalid/ligature"
	paragraph := func(runs ...manifold.Run) manifold.Block { return manifold.Block{Kind: manifold.Paragraph, Runs: runs} }
	header := paragraph(run("A", red), run("V", blue))
	for j := range header.Runs {
		header.Runs[j].FontSize = 10
	}
	doc := &manifold.Document{Metadata: manifold.Metadata{Title: "Paint preserves shaping"}, Layout: manifold.Layout{Width: 360, Height: 420, Margin: 40}, Headers: []manifold.Block{header}, Footers: []manifold.Block{header}, Blocks: []manifold.Block{paragraph(a, v), paragraph(o, f, i, ce), {Kind: manifold.Table, Rows: []manifold.Row{{Cells: []manifold.Cell{{Blocks: []manifold.Block{paragraph(o, f, i, ce)}}}}}}}}
	opts := manifold.WriteOptions{PDFFonts: []manifold.FontFace{{Family: "Noto Sans", Data: font}}}
	write := func(name string, d *manifold.Document, format manifold.Format) {
		var b bytes.Buffer
		if err := manifold.Write(context.Background(), &b, d, format, opts); err != nil {
			log.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(out, name), b.Bytes(), 0644); err != nil {
			log.Fatal(err)
		}
	}
	for _, format := range []manifold.Format{manifold.SF, manifold.DOCX, manifold.PDF} {
		write("paint."+string(format), doc, format)
	}
	emergency := &manifold.Document{Layout: manifold.Layout{Width: 32, Height: 220, Margin: 10}, Blocks: []manifold.Block{paragraph(run("f", red), run("f", blue), run("i", green))}}
	write("emergency.sf", emergency, manifold.SF)
	write("emergency.pdf", emergency, manifold.PDF)
}
