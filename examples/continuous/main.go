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

	"github.com/artpar/manydoc"
)

func main() {
	ctx := context.Background()
	dir := "output/continuous"
	must(os.MkdirAll(dir, 0755))
	p := func(s string) manydoc.Block {
		return manydoc.Block{Kind: manydoc.Paragraph, Runs: []manydoc.Run{{Text: s}}}
	}
	story := func(s string) *manydoc.RunningStory { return &manydoc.RunningStory{Blocks: []manydoc.Block{p(s)}} }
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
	figure := manydoc.Block{Kind: manydoc.Picture, Image: &manydoc.Image{MediaType: "image/png", Data: data.Bytes(), Width: 24, Height: 12, Alt: "Continuous story logo"}}
	a := &manydoc.Section{Headers: manydoc.RunningStories{Default: story("A DEFAULT"), Even: story("A EVEN")}, Footers: manydoc.RunningStories{Default: story("A FOOT"), Even: story("A FOOT")}, Blocks: []manydoc.Block{p("A ONE"), {Kind: manydoc.PageBreak}, p("A SHARED PAGE")}}
	headerTable := manydoc.Block{Kind: manydoc.Table}
	headerTable.Rows = append(headerTable.Rows, manydoc.Row{Cells: []manydoc.Cell{{ColSpan: 2, Blocks: []manydoc.Block{p("B EVEN")}}}})
	headerTable.Rows = append(headerTable.Rows, manydoc.Row{Cells: []manydoc.Cell{{Blocks: []manydoc.Block{figure}}, {Blocks: []manydoc.Block{p("B HEADER RIGHT")}}}})
	rich := &manydoc.RunningStory{Blocks: []manydoc.Block{headerTable}}
	b := &manydoc.Section{Start: manydoc.SectionContinuous, FirstPage: true, Headers: manydoc.RunningStories{Default: story("B DEFAULT"), Even: rich, First: story("B UNUSED FIRST")}, Footers: manydoc.RunningStories{Default: story("B FOOT"), Even: story("B FOOT")}, Blocks: []manydoc.Block{p("B SHARED"), {Kind: manydoc.PageBreak}, p("B TWO")}}
	b.Blocks[0].Runs[0].Link = "https://example.com/continuous"
	b.Blocks[0].Runs = append(b.Blocks[0].Runs, manydoc.Run{Note: &manydoc.NoteReference{ID: "continuous-note", Kind: manydoc.Footnote}})
	table := manydoc.Block{Kind: manydoc.Table}
	for i := 0; i < 6; i++ {
		table.Rows = append(table.Rows, manydoc.Row{Cells: []manydoc.Cell{{Blocks: []manydoc.Block{p(fmt.Sprintf("ROW %d", i))}}, {Blocks: []manydoc.Block{p(fmt.Sprintf("VALUE %d", i))}}}})
	}
	b.Blocks = append(b.Blocks, table, manydoc.Block{Kind: manydoc.PageBreak}, p("B THREE"))
	c := &manydoc.Section{Start: manydoc.SectionContinuous, Headers: manydoc.RunningStories{Default: story("C DEFAULT"), Even: story("C EVEN")}, Footers: manydoc.RunningStories{Default: story("C FOOT"), Even: story("C FOOT")}, Blocks: []manydoc.Block{p("C SHARED"), {Kind: manydoc.PageBreak}, p("C TWO")}}
	d := &manydoc.Document{Metadata: manydoc.Metadata{Title: "Continuous section stories"}, Layout: manydoc.Layout{Width: 360, Height: 440, Margin: 40}, EvenAndOddHeaders: true, Blocks: []manydoc.Block{{Kind: manydoc.SectionBlock, Section: a}, {Kind: manydoc.SectionBlock, Section: b}, {Kind: manydoc.SectionBlock, Section: c}}, Notes: []manydoc.Note{{ID: "continuous-note", Kind: manydoc.Footnote, Blocks: []manydoc.Block{p("SHARED NOTE"), figure}}}}
	for _, it := range []struct {
		name   string
		format manydoc.Format
	}{{"document.sf", manydoc.SF}, {"document.docx", manydoc.DOCX}, {"document.pdf", manydoc.PDF}} {
		var buf bytes.Buffer
		must(manydoc.Write(ctx, &buf, d, it.format, manydoc.WriteOptions{}))
		must(os.WriteFile(filepath.Join(dir, it.name), buf.Bytes(), 0644))
	}
	for _, it := range []struct {
		input, output string
		format        manydoc.Format
	}{{"document.docx", "docx.sf", manydoc.DOCX}, {"document.pdf", "pdf.sf", manydoc.PDF}} {
		data, err := os.ReadFile(filepath.Join(dir, it.input))
		must(err)
		back, err := manydoc.Read(ctx, bytes.NewReader(data), it.format, manydoc.ReadOptions{PDFSkipInference: true})
		must(err)
		var buf bytes.Buffer
		must(manydoc.Write(ctx, &buf, back, manydoc.SF, manydoc.WriteOptions{}))
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
