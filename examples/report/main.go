// Run with: go run ./examples/report -out output
package main

import (
	"bytes"
	"context"
	"flag"
	"image"
	"image/color"
	"image/png"
	"log"
	"os"
	"path/filepath"

	"github.com/artpar/manifold"
)

func main() {
	out := flag.String("out", "output", "output directory")
	flag.Parse()
	if e := os.MkdirAll(*out, 0755); e != nil {
		log.Fatal(e)
	}
	im := image.NewRGBA(image.Rect(0, 0, 320, 60))
	for y := 0; y < 60; y++ {
		for x := 0; x < 320; x++ {
			im.SetRGBA(x, y, color.RGBA{R: 32, G: uint8(80 + x/3), B: 150, A: 255})
		}
	}
	var pic bytes.Buffer
	if e := png.Encode(&pic, im); e != nil {
		log.Fatal(e)
	}
	d := &manifold.Document{Metadata: manifold.Metadata{Title: "Quarterly report", Author: "manydoc example"}, Headers: []manifold.Block{{Kind: manifold.Paragraph, Runs: []manifold.Run{{Text: "Example organization"}}}}, Footers: []manifold.Block{{Kind: manifold.Paragraph, Runs: []manifold.Run{{Text: "Prepared with manydoc"}}}}, Blocks: []manifold.Block{
		{Kind: manifold.Heading, Level: 1, Runs: []manifold.Run{{Text: "Quarterly report"}}},
		{Kind: manifold.Paragraph, Runs: []manifold.Run{{Text: "This report demonstrates "}, {Text: "formatted text", Bold: true}, {Text: ", "}, {Text: "links", Link: "https://example.com"}, {Text: ", tables, images, and explicit page breaks."}}},
		{Kind: manifold.Paragraph, Runs: []manifold.Run{{Text: "Unicode text: café Ελληνικά Привет"}}},
		{Kind: manifold.Picture, Image: &manifold.Image{Data: pic.Bytes(), MediaType: "image/png", Width: 240, Height: 45, Alt: "Blue gradient banner"}},
		{Kind: manifold.Table, Rows: []manifold.Row{{Cells: []manifold.Cell{{Blocks: []manifold.Block{{Kind: manifold.Paragraph, Runs: []manifold.Run{{Text: "Product", Bold: true}}}}}, {Blocks: []manifold.Block{{Kind: manifold.Paragraph, Runs: []manifold.Run{{Text: "Quantity", Bold: true}}}}}}}, {Cells: []manifold.Cell{{Blocks: []manifold.Block{{Kind: manifold.Paragraph, Runs: []manifold.Run{{Text: "Colleaf"}}}}}, {Blocks: []manifold.Block{{Kind: manifold.Paragraph, Runs: []manifold.Run{{Text: "42"}}}}}}}}},
		{Kind: manifold.Paragraph, List: "bullet", Runs: []manifold.Run{{Text: "One shared model for DOCX and PDF output."}}},
		{Kind: manifold.PageBreak}, {Kind: manifold.Heading, Level: 2, Runs: []manifold.Run{{Text: "Second page"}}},
		{Kind: manifold.Paragraph, Runs: []manifold.Run{{Text: "End of the example report."}}},
	}}
	for _, format := range []manifold.Format{manifold.DOCX, manifold.PDF, manifold.SF, manifold.JSON} {
		name := filepath.Join(*out, "report."+string(format))
		f, e := os.Create(name)
		if e != nil {
			log.Fatal(e)
		}
		e = manifold.Write(context.Background(), f, d, format, manifold.WriteOptions{})
		ce := f.Close()
		if e != nil {
			log.Fatal(e)
		}
		if ce != nil {
			log.Fatal(ce)
		}
		log.Println(name)
	}
}
