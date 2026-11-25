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

	"github.com/artpar/manifold"
)

func main() {
	ctx := context.Background()
	dir := "output/continuous-geometry"
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

	oldAnchor := p("OLD BODY")
	oldAnchor.Runs = append(oldAnchor.Runs, manifold.Run{Note: &manifold.NoteReference{ID: "old", Kind: manifold.Footnote}})
	newAnchor := p("NEW BODY")
	newAnchor.Runs[0].Link = "https://example.com/frame"
	newAnchor.Runs = append(newAnchor.Runs, manifold.Run{Note: &manifold.NoteReference{ID: "new", Kind: manifold.Footnote}})
	a := &manifold.Section{Headers: manifold.RunningStories{Default: story("OLD HEADER")}, Footers: manifold.RunningStories{Default: story("OLD FOOT")}, Blocks: []manifold.Block{oldAnchor}}
	point := func(v float64) *float64 { return &v }
	same := manifold.Layout{Width: 360, Height: 440, Margins: &manifold.PageMargins{Top: 70, Right: 20, Bottom: 60, Left: 100}, HeaderDistance: point(12), FooterDistance: point(15)}
	b := &manifold.Section{Start: manifold.SectionContinuous, Layout: &same, Headers: manifold.RunningStories{Default: story("NEW HEADER")}, Footers: manifold.RunningStories{Default: story("NEW FOOT")}, Blocks: []manifold.Block{newAnchor}}
	table := manifold.Block{Kind: manifold.Table}
	for i := 0; i < 4; i++ {
		table.Rows = append(table.Rows, manifold.Row{Cells: []manifold.Cell{{Blocks: []manifold.Block{p(fmt.Sprintf("ROW %d", i))}}, {Blocks: []manifold.Block{p(fmt.Sprintf("VALUE %d", i))}}}})
	}
	b.Blocks = append(b.Blocks, table, manifold.Block{Kind: manifold.PageBreak}, p("NEW SECOND"))
	paper := manifold.Layout{Width: 480, Height: 360, Margins: &manifold.PageMargins{Top: 50, Right: 60, Bottom: 40, Left: 30}, HeaderDistance: point(10), FooterDistance: point(12)}
	wideAnchor := p("WIDE BODY")
	wideAnchor.Runs = append(wideAnchor.Runs, manifold.Run{Note: &manifold.NoteReference{ID: "wide", Kind: manifold.Footnote}})
	c := &manifold.Section{Start: manifold.SectionContinuous, Layout: &paper, Headers: manifold.RunningStories{Default: story("WIDE HEADER")}, Footers: manifold.RunningStories{Default: story("WIDE FOOT")}, Blocks: []manifold.Block{wideAnchor, {Kind: manifold.PageBreak}, p("WIDE SECOND")}}
	changed := paper
	changed.Margins = &manifold.PageMargins{Top: 60, Right: 30, Bottom: 50, Left: 80}
	changed.HeaderDistance = point(4)
	changed.FooterDistance = point(6)
	last := &manifold.Section{Start: manifold.SectionContinuous, Layout: &changed, Headers: manifold.RunningStories{Default: story("LAST HEADER")}, Footers: manifold.RunningStories{Default: story("LAST FOOT")}, Blocks: []manifold.Block{p("LAST BODY"), {Kind: manifold.PageBreak}, p("LAST SECOND")}}
	picture := func(w, h float64) manifold.Block {
		v := *figure.Image
		v.Width, v.Height = w, h
		return manifold.Block{Kind: manifold.Picture, Image: &v}
	}
	newNote := p("NEW NOTE")
	newNote.Direction = "rtl"
	d := &manifold.Document{Metadata: manifold.Metadata{Title: "Continuous body frames and paper"}, Layout: manifold.Layout{Width: 360, Height: 440, Margin: 40}, Blocks: []manifold.Block{{Kind: manifold.SectionBlock, Section: a}, {Kind: manifold.SectionBlock, Section: b}, {Kind: manifold.SectionBlock, Section: c}, {Kind: manifold.SectionBlock, Section: last}}, Notes: []manifold.Note{{ID: "old", Kind: manifold.Footnote, Blocks: []manifold.Block{p("OLD NOTE"), picture(40, 8)}}, {ID: "new", Kind: manifold.Footnote, Blocks: []manifold.Block{newNote, picture(20, 8)}}, {ID: "wide", Kind: manifold.Footnote, Blocks: []manifold.Block{p("WIDE NOTE"), picture(30, 10)}}}}
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
		log.Fatal("continuous geometry SF semantics changed")
	}
	fmt.Println("Wrote continuous geometry SF/DOCX/PDF and reimported SF to", dir)
}
func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
