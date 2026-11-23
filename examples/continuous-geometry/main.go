// Four conversion paths with changing continuous body frames and paper canvases.
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
	dir := "output/continuous-geometry"
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

	oldAnchor := p("OLD BODY")
	oldAnchor.Runs = append(oldAnchor.Runs, manydoc.Run{Note: &manydoc.NoteReference{ID: "old", Kind: manydoc.Footnote}})
	newAnchor := p("NEW BODY")
	newAnchor.Runs[0].Link = "https://example.com/frame"
	newAnchor.Runs = append(newAnchor.Runs, manydoc.Run{Note: &manydoc.NoteReference{ID: "new", Kind: manydoc.Footnote}})
	a := &manydoc.Section{Headers: manydoc.RunningStories{Default: story("OLD HEADER")}, Footers: manydoc.RunningStories{Default: story("OLD FOOT")}, Blocks: []manydoc.Block{oldAnchor}}
	point := func(v float64) *float64 { return &v }
	same := manydoc.Layout{Width: 360, Height: 440, Margins: &manydoc.PageMargins{Top: 70, Right: 20, Bottom: 60, Left: 100}, HeaderDistance: point(12), FooterDistance: point(15)}
	b := &manydoc.Section{Start: manydoc.SectionContinuous, Layout: &same, Headers: manydoc.RunningStories{Default: story("NEW HEADER")}, Footers: manydoc.RunningStories{Default: story("NEW FOOT")}, Blocks: []manydoc.Block{newAnchor}}
	table := manydoc.Block{Kind: manydoc.Table}
	for i := 0; i < 4; i++ {
		table.Rows = append(table.Rows, manydoc.Row{Cells: []manydoc.Cell{{Blocks: []manydoc.Block{p(fmt.Sprintf("ROW %d", i))}}, {Blocks: []manydoc.Block{p(fmt.Sprintf("VALUE %d", i))}}}})
	}
	b.Blocks = append(b.Blocks, table, manydoc.Block{Kind: manydoc.PageBreak}, p("NEW SECOND"))
	paper := manydoc.Layout{Width: 480, Height: 360, Margins: &manydoc.PageMargins{Top: 50, Right: 60, Bottom: 40, Left: 30}, HeaderDistance: point(10), FooterDistance: point(12)}
	wideAnchor := p("WIDE BODY")
	wideAnchor.Runs = append(wideAnchor.Runs, manydoc.Run{Note: &manydoc.NoteReference{ID: "wide", Kind: manydoc.Footnote}})
	c := &manydoc.Section{Start: manydoc.SectionContinuous, Layout: &paper, Headers: manydoc.RunningStories{Default: story("WIDE HEADER")}, Footers: manydoc.RunningStories{Default: story("WIDE FOOT")}, Blocks: []manydoc.Block{wideAnchor, {Kind: manydoc.PageBreak}, p("WIDE SECOND")}}
	changed := paper
	changed.Margins = &manydoc.PageMargins{Top: 60, Right: 30, Bottom: 50, Left: 80}
	changed.HeaderDistance = point(4)
	changed.FooterDistance = point(6)
	last := &manydoc.Section{Start: manydoc.SectionContinuous, Layout: &changed, Headers: manydoc.RunningStories{Default: story("LAST HEADER")}, Footers: manydoc.RunningStories{Default: story("LAST FOOT")}, Blocks: []manydoc.Block{p("LAST BODY"), {Kind: manydoc.PageBreak}, p("LAST SECOND")}}
	picture := func(w, h float64) manydoc.Block {
		v := *figure.Image
		v.Width, v.Height = w, h
		return manydoc.Block{Kind: manydoc.Picture, Image: &v}
	}
	newNote := p("NEW NOTE")
	newNote.Direction = "rtl"
	d := &manydoc.Document{Metadata: manydoc.Metadata{Title: "Continuous body frames and paper"}, Layout: manydoc.Layout{Width: 360, Height: 440, Margin: 40}, Blocks: []manydoc.Block{{Kind: manydoc.SectionBlock, Section: a}, {Kind: manydoc.SectionBlock, Section: b}, {Kind: manydoc.SectionBlock, Section: c}, {Kind: manydoc.SectionBlock, Section: last}}, Notes: []manydoc.Note{{ID: "old", Kind: manydoc.Footnote, Blocks: []manydoc.Block{p("OLD NOTE"), picture(40, 8)}}, {ID: "new", Kind: manydoc.Footnote, Blocks: []manydoc.Block{newNote, picture(20, 8)}}, {ID: "wide", Kind: manydoc.Footnote, Blocks: []manydoc.Block{p("WIDE NOTE"), picture(30, 10)}}}}
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
		log.Fatal("continuous geometry SF semantics changed")
	}
	fmt.Println("Wrote continuous geometry SF/DOCX/PDF and reimported SF to", dir)
}
func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
