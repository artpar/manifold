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

	"github.com/artpar/manifold"
)

func main() {
	ctx := context.Background()
	dir := "output/sections"
	must(os.MkdirAll(dir, 0755))
	p := func(s string) manifold.Block {
		return manifold.Block{Kind: manifold.Paragraph, Runs: []manifold.Run{{Text: s}}}
	}
	story := func(s string) *manifold.RunningStory { return &manifold.RunningStory{Blocks: []manifold.Block{p(s)}} }
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
	figure := manifold.Block{Kind: manifold.Picture, Image: &manifold.Image{MediaType: "image/png", Data: raster.Bytes(), Alt: "Section logo", Width: 32, Height: 16}}
	a := &manifold.Section{FirstPage: true, Headers: manifold.RunningStories{Default: story("A DEFAULT"), First: story("A FIRST"), Even: story("A EVEN")}, Footers: manifold.RunningStories{Default: story("A FOOT"), First: &manifold.RunningStory{}, Even: story("A EVEN FOOT")}, Blocks: []manifold.Block{p("A PAGE ONE"), {Kind: manifold.PageBreak}, p("A PAGE TWO"), {Kind: manifold.PageBreak}, p("A PAGE THREE")}}
	a.Headers.Default.Blocks = append(a.Headers.Default.Blocks, figure)
	a.Headers.First.Blocks[0].Runs[0].Link = "https://example.com/first"
	a.Headers.First.Blocks[0].Runs[0].Bold = true
	a.Blocks = append(a.Blocks, manifold.Block{Kind: manifold.Table, Rows: []manifold.Row{{Cells: []manifold.Cell{{ColSpan: 2, Blocks: []manifold.Block{p("MERGED SECTION CELL"), figure}}}}, {Cells: []manifold.Cell{{Blocks: []manifold.Block{p("CELL LEFT")}}, {Blocks: []manifold.Block{p("CELL RIGHT")}}}}}})
	distance := func(n float64) *float64 { return &n }
	landscape := manifold.Layout{Width: 480, Height: 360, Margin: 29, Margins: &manifold.PageMargins{Top: 45, Right: 40, Bottom: 50, Left: 30}, HeaderDistance: distance(12), FooterDistance: distance(15)}
	b := &manifold.Section{Layout: &landscape, Start: manifold.SectionOddPage, FirstPage: true, Headers: manifold.RunningStories{First: &manifold.RunningStory{}}, Blocks: []manifold.Block{p("B PAGE ONE"), {Kind: manifold.PageBreak}, p("B PAGE TWO"), {Kind: manifold.PageBreak}, {Kind: manifold.ListBlock, List: "number", Items: []manifold.ListItem{{Blocks: []manifold.Block{p("B LIST")}}}}, p("B BODY")}}
	b.Blocks[len(b.Blocks)-1].Runs = append(b.Blocks[len(b.Blocks)-1].Runs, manifold.Run{Note: &manifold.NoteReference{ID: "section-note", Kind: manifold.Footnote}})
	c := &manifold.Section{Start: manifold.SectionNextPage, Headers: manifold.RunningStories{Default: &manifold.RunningStory{}}, Blocks: []manifold.Block{p("C BODY")}}
	d := &manifold.Document{Metadata: manifold.Metadata{Title: "Section frames and stories"}, Layout: manifold.Layout{Width: 360, Height: 440, Margin: 40}, EvenAndOddHeaders: true, Blocks: []manifold.Block{{Kind: manifold.SectionBlock, Section: a}, {Kind: manifold.SectionBlock, Section: b}, {Kind: manifold.SectionBlock, Section: c}}, Notes: []manifold.Note{{ID: "section-note", Kind: manifold.Footnote, Blocks: []manifold.Block{p("SECTION NOTE"), figure}}}}
	for _, item := range []struct {
		name   string
		format manifold.Format
	}{{"document.sf", manifold.SF}, {"document.docx", manifold.DOCX}, {"document.pdf", manifold.PDF}} {
		var data bytes.Buffer
		must(manifold.Write(ctx, &data, d, item.format, manifold.WriteOptions{}))
		must(os.WriteFile(filepath.Join(dir, item.name), data.Bytes(), 0644))
	}
	for _, item := range []struct {
		input, output string
		format        manifold.Format
	}{{"document.docx", "docx.sf", manifold.DOCX}, {"document.pdf", "pdf.sf", manifold.PDF}} {
		data, err := os.ReadFile(filepath.Join(dir, item.input))
		must(err)
		back, err := manifold.Read(ctx, bytes.NewReader(data), item.format, manifold.ReadOptions{PDFSkipInference: true})
		must(err)
		var sf bytes.Buffer
		must(manifold.Write(ctx, &sf, back, manifold.SF, manifold.WriteOptions{}))
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
