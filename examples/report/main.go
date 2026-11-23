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

	"github.com/artpar/manydoc"
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
	d := &manydoc.Document{Metadata: manydoc.Metadata{Title: "Quarterly report", Author: "manydoc example"}, Headers: []manydoc.Block{{Kind: manydoc.Paragraph, Runs: []manydoc.Run{{Text: "Example organization"}}}}, Footers: []manydoc.Block{{Kind: manydoc.Paragraph, Runs: []manydoc.Run{{Text: "Prepared with manydoc"}}}}, Blocks: []manydoc.Block{
		{Kind: manydoc.Heading, Level: 1, Runs: []manydoc.Run{{Text: "Quarterly report"}}},
		{Kind: manydoc.Paragraph, Runs: []manydoc.Run{{Text: "This report demonstrates "}, {Text: "formatted text", Bold: true}, {Text: ", "}, {Text: "links", Link: "https://example.com"}, {Text: ", tables, images, and explicit page breaks."}}},
		{Kind: manydoc.Paragraph, Runs: []manydoc.Run{{Text: "Unicode text: café Ελληνικά Привет"}}},
		{Kind: manydoc.Picture, Image: &manydoc.Image{Data: pic.Bytes(), MediaType: "image/png", Width: 240, Height: 45, Alt: "Blue gradient banner"}},
		{Kind: manydoc.Table, Rows: []manydoc.Row{{Cells: []manydoc.Cell{{Blocks: []manydoc.Block{{Kind: manydoc.Paragraph, Runs: []manydoc.Run{{Text: "Product", Bold: true}}}}}, {Blocks: []manydoc.Block{{Kind: manydoc.Paragraph, Runs: []manydoc.Run{{Text: "Quantity", Bold: true}}}}}}}, {Cells: []manydoc.Cell{{Blocks: []manydoc.Block{{Kind: manydoc.Paragraph, Runs: []manydoc.Run{{Text: "Colleaf"}}}}}, {Blocks: []manydoc.Block{{Kind: manydoc.Paragraph, Runs: []manydoc.Run{{Text: "42"}}}}}}}}},
		{Kind: manydoc.Paragraph, List: "bullet", Runs: []manydoc.Run{{Text: "One shared model for DOCX and PDF output."}}},
		{Kind: manydoc.PageBreak}, {Kind: manydoc.Heading, Level: 2, Runs: []manydoc.Run{{Text: "Second page"}}},
		{Kind: manydoc.Paragraph, Runs: []manydoc.Run{{Text: "End of the example report."}}},
	}}
	for _, format := range []manydoc.Format{manydoc.DOCX, manydoc.PDF, manydoc.SF, manydoc.JSON} {
		name := filepath.Join(*out, "report."+string(format))
		f, e := os.Create(name)
		if e != nil {
			log.Fatal(e)
		}
		e = manydoc.Write(context.Background(), f, d, format, manydoc.WriteOptions{})
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
