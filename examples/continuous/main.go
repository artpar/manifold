// Four conversion paths with shared-page story changes and section-local parity.
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
	dir := "output/continuous"
	must(os.MkdirAll(dir, 0755))
	p := func(s string) manifold.Block {
		return manifold.Block{Kind: manifold.Paragraph, Runs: []manifold.Run{{Text: s}}}
	}
	story := func(s string) *manifold.RunningStory { return &manifold.RunningStory{Blocks: []manifold.Block{p(s)}} }
	canvas := image.NewNRGBA(image.Rect(0, 0, 24, 12))
	for y := 0; y < 12; y++ {
		for x := 0; x < 24; x++ {
			c := color.NRGBA{20, 100, 180, 255}
			if x >= 12 {
				c = color.NRGBA{190, 60, 20, 128}
			}
			canvas.SetNRGBA(x, y, c)
		}
	}
	var data bytes.Buffer
	must(png.Encode(&data, canvas))
	figure := manifold.Block{Kind: manifold.Picture, Image: &manifold.Image{MediaType: "image/png", Data: data.Bytes(), Width: 24, Height: 12, Alt: "Continuous story logo"}}
	a := &manifold.Section{Headers: manifold.RunningStories{Default: story("A DEFAULT"), Even: story("A EVEN")}, Footers: manifold.RunningStories{Default: story("A FOOT"), Even: story("A FOOT")}, Blocks: []manifold.Block{p("A ONE"), {Kind: manifold.PageBreak}, p("A SHARED PAGE")}}
	headerTable := manifold.Block{Kind: manifold.Table}
	headerTable.Rows = append(headerTable.Rows, manifold.Row{Cells: []manifold.Cell{{ColSpan: 2, Blocks: []manifold.Block{p("B EVEN")}}}})
	headerTable.Rows = append(headerTable.Rows, manifold.Row{Cells: []manifold.Cell{{Blocks: []manifold.Block{figure}}, {Blocks: []manifold.Block{p("B HEADER RIGHT")}}}})
	rich := &manifold.RunningStory{Blocks: []manifold.Block{headerTable}}
	b := &manifold.Section{Start: manifold.SectionContinuous, FirstPage: true, Headers: manifold.RunningStories{Default: story("B DEFAULT"), Even: rich, First: story("B UNUSED FIRST")}, Footers: manifold.RunningStories{Default: story("B FOOT"), Even: story("B FOOT")}, Blocks: []manifold.Block{p("B SHARED"), {Kind: manifold.PageBreak}, p("B TWO")}}
	b.Blocks[0].Runs[0].Link = "https://example.com/continuous"
	b.Blocks[0].Runs = append(b.Blocks[0].Runs, manifold.Run{Note: &manifold.NoteReference{ID: "continuous-note", Kind: manifold.Footnote}})
	table := manifold.Block{Kind: manifold.Table}
	for i := 0; i < 6; i++ {
		table.Rows = append(table.Rows, manifold.Row{Cells: []manifold.Cell{{Blocks: []manifold.Block{p(fmt.Sprintf("ROW %d", i))}}, {Blocks: []manifold.Block{p(fmt.Sprintf("VALUE %d", i))}}}})
	}
	b.Blocks = append(b.Blocks, table, manifold.Block{Kind: manifold.PageBreak}, p("B THREE"))
	c := &manifold.Section{Start: manifold.SectionContinuous, Headers: manifold.RunningStories{Default: story("C DEFAULT"), Even: story("C EVEN")}, Footers: manifold.RunningStories{Default: story("C FOOT"), Even: story("C FOOT")}, Blocks: []manifold.Block{p("C SHARED"), {Kind: manifold.PageBreak}, p("C TWO")}}
	d := &manifold.Document{Metadata: manifold.Metadata{Title: "Continuous section stories"}, Layout: manifold.Layout{Width: 360, Height: 440, Margin: 40}, EvenAndOddHeaders: true, Blocks: []manifold.Block{{Kind: manifold.SectionBlock, Section: a}, {Kind: manifold.SectionBlock, Section: b}, {Kind: manifold.SectionBlock, Section: c}}, Notes: []manifold.Note{{ID: "continuous-note", Kind: manifold.Footnote, Blocks: []manifold.Block{p("SHARED NOTE"), figure}}}}
	for _, it := range []struct {
		name   string
		format manifold.Format
	}{{"document.sf", manifold.SF}, {"document.docx", manifold.DOCX}, {"document.pdf", manifold.PDF}} {
		var buf bytes.Buffer
		must(manifold.Write(ctx, &buf, d, it.format, manifold.WriteOptions{}))
		must(os.WriteFile(filepath.Join(dir, it.name), buf.Bytes(), 0644))
	}
	for _, it := range []struct {
		input, output string
		format        manifold.Format
	}{{"document.docx", "docx.sf", manifold.DOCX}, {"document.pdf", "pdf.sf", manifold.PDF}} {
		data, err := os.ReadFile(filepath.Join(dir, it.input))
		must(err)
		back, err := manifold.Read(ctx, bytes.NewReader(data), it.format, manifold.ReadOptions{PDFSkipInference: true})
		must(err)
		var buf bytes.Buffer
		must(manifold.Write(ctx, &buf, back, manifold.SF, manifold.WriteOptions{}))
		must(os.WriteFile(filepath.Join(dir, it.output), buf.Bytes(), 0644))
	}
	source, err := os.ReadFile(filepath.Join(dir, "document.sf"))
	must(err)
	back, err := os.ReadFile(filepath.Join(dir, "docx.sf"))
	must(err)
	if !bytes.Equal(source, back) {
		log.Fatal("continuous section SF semantics changed")
	}
	fmt.Println("Wrote continuous SF/DOCX/PDF and reimported SF to", dir)
}
func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
