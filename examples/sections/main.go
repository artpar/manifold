// Four paths with section-specific geometry, inheritance and running variants.
package main

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"os"
	"path/filepath"

	"github.com/artpar/manydoc"
)

func main() {
	ctx := context.Background()
	dir := "output/sections"
	must(os.MkdirAll(dir, 0755))
	p := func(s string) manydoc.Block {
		return manydoc.Block{Kind: manydoc.Paragraph, Runs: []manydoc.Run{{Text: s}}}
	}
	story := func(s string) *manydoc.RunningStory { return &manydoc.RunningStory{Blocks: []manydoc.Block{p(s)}} }
	canvas := image.NewNRGBA(image.Rect(0, 0, 32, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 32; x++ {
			c := color.NRGBA{R: 200, G: 30, B: 50, A: 255}
			if x >= 16 {
				c = color.NRGBA{R: 20, G: 80, B: 220, A: 128}
			}
			canvas.SetNRGBA(x, y, c)
		}
	}
	var raster bytes.Buffer
	must(png.Encode(&raster, canvas))
	figure := manydoc.Block{Kind: manydoc.Picture, Image: &manydoc.Image{MediaType: "image/png", Data: raster.Bytes(), Alt: "Section logo", Width: 32, Height: 16}}
	a := &manydoc.Section{FirstPage: true, Headers: manydoc.RunningStories{Default: story("A DEFAULT"), First: story("A FIRST"), Even: story("A EVEN")}, Footers: manydoc.RunningStories{Default: story("A FOOT"), First: &manydoc.RunningStory{}, Even: story("A EVEN FOOT")}, Blocks: []manydoc.Block{p("A PAGE ONE"), {Kind: manydoc.PageBreak}, p("A PAGE TWO"), {Kind: manydoc.PageBreak}, p("A PAGE THREE")}}
	a.Headers.Default.Blocks = append(a.Headers.Default.Blocks, figure)
	a.Headers.First.Blocks[0].Runs[0].Link = "https://example.com/first"
	a.Headers.First.Blocks[0].Runs[0].Bold = true
	a.Blocks = append(a.Blocks, manydoc.Block{Kind: manydoc.Table, Rows: []manydoc.Row{{Cells: []manydoc.Cell{{ColSpan: 2, Blocks: []manydoc.Block{p("MERGED SECTION CELL"), figure}}}}, {Cells: []manydoc.Cell{{Blocks: []manydoc.Block{p("CELL LEFT")}}, {Blocks: []manydoc.Block{p("CELL RIGHT")}}}}}})
	distance := func(n float64) *float64 { return &n }
	landscape := manydoc.Layout{Width: 480, Height: 360, Margin: 29, Margins: &manydoc.PageMargins{Top: 45, Right: 40, Bottom: 50, Left: 30}, HeaderDistance: distance(12), FooterDistance: distance(15)}
	b := &manydoc.Section{Layout: &landscape, Start: manydoc.SectionOddPage, FirstPage: true, Headers: manydoc.RunningStories{First: &manydoc.RunningStory{}}, Blocks: []manydoc.Block{p("B PAGE ONE"), {Kind: manydoc.PageBreak}, p("B PAGE TWO"), {Kind: manydoc.PageBreak}, {Kind: manydoc.ListBlock, List: "number", Items: []manydoc.ListItem{{Blocks: []manydoc.Block{p("B LIST")}}}}, p("B BODY")}}
	b.Blocks[len(b.Blocks)-1].Runs = append(b.Blocks[len(b.Blocks)-1].Runs, manydoc.Run{Note: &manydoc.NoteReference{ID: "section-note", Kind: manydoc.Footnote}})
	c := &manydoc.Section{Start: manydoc.SectionNextPage, Headers: manydoc.RunningStories{Default: &manydoc.RunningStory{}}, Blocks: []manydoc.Block{p("C BODY")}}
	d := &manydoc.Document{Metadata: manydoc.Metadata{Title: "Section frames and stories"}, Layout: manydoc.Layout{Width: 360, Height: 440, Margin: 40}, EvenAndOddHeaders: true, Blocks: []manydoc.Block{{Kind: manydoc.SectionBlock, Section: a}, {Kind: manydoc.SectionBlock, Section: b}, {Kind: manydoc.SectionBlock, Section: c}}, Notes: []manydoc.Note{{ID: "section-note", Kind: manydoc.Footnote, Blocks: []manydoc.Block{p("SECTION NOTE"), figure}}}}
	for _, item := range []struct {
		name   string
		format manydoc.Format
	}{{"document.sf", manydoc.SF}, {"document.docx", manydoc.DOCX}, {"document.pdf", manydoc.PDF}} {
		var data bytes.Buffer
		must(manydoc.Write(ctx, &data, d, item.format, manydoc.WriteOptions{}))
		must(os.WriteFile(filepath.Join(dir, item.name), data.Bytes(), 0644))
	}
	for _, item := range []struct {
		input, output string
		format        manydoc.Format
	}{{"document.docx", "docx.sf", manydoc.DOCX}, {"document.pdf", "pdf.sf", manydoc.PDF}} {
		data, err := os.ReadFile(filepath.Join(dir, item.input))
		must(err)
		back, err := manydoc.Read(ctx, bytes.NewReader(data), item.format, manydoc.ReadOptions{PDFSkipInference: true})
		must(err)
		var sf bytes.Buffer
		must(manydoc.Write(ctx, &sf, back, manydoc.SF, manydoc.WriteOptions{}))
		must(os.WriteFile(filepath.Join(dir, item.output), sf.Bytes(), 0644))
	}
	source, err := os.ReadFile(filepath.Join(dir, "document.sf"))
	must(err)
	back, err := os.ReadFile(filepath.Join(dir, "docx.sf"))
	must(err)
	if !bytes.Equal(source, back) {
		log.Fatal("DOCX section semantics changed")
	}
	fmt.Println("Wrote section SF/DOCX/PDF and recovered SF to", dir)
}
func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
