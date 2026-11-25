// Four conversion paths with explicit asymmetric/zero margins and running offsets.
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
	out := "output/page-layout"
	if err := os.MkdirAll(out, 0755); err != nil {
		log.Fatal(err)
	}
	point := func(n float64) *float64 { return &n }
	para := func(text string, size float64) manifold.Block {
		return manifold.Block{Kind: manifold.Paragraph, Runs: []manifold.Run{{Text: text, FontSize: size}}}
	}
	im := image.NewNRGBA(image.Rect(0, 0, 32, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 32; x++ {
			c := color.NRGBA{200, 30, 50, 255}
			if x >= 16 {
				c = color.NRGBA{20, 80, 220, 128}
			}
			im.SetNRGBA(x, y, c)
		}
	}
	var pngData bytes.Buffer
	if err := png.Encode(&pngData, im); err != nil {
		log.Fatal(err)
	}
	picture := func(w, h float64) manifold.Block {
		return manifold.Block{Kind: manifold.Picture, Image: &manifold.Image{MediaType: "image/png", Data: pngData.Bytes(), Alt: "Original page-layout logo", Width: w, Height: h}}
	}
	headerLink := para("ASYMMETRIC HEADER", 8)
	headerLink.Runs[0].Link = "https://example.invalid/page-header"
	headerLink.Runs[0].Color = "#224466"
	headerLink.Runs[0].Underline = true
	header := manifold.Block{Kind: manifold.Table, Rows: []manifold.Row{{Cells: []manifold.Cell{{ColSpan: 2, Blocks: []manifold.Block{headerLink}}}}, {Cells: []manifold.Cell{{Blocks: []manifold.Block{picture(30, 10)}}, {Blocks: []manifold.Block{para("RIGHT HEADER", 8)}}}}}}
	note := para("NOTE ABOVE FOOTER", 8)
	note.Direction = "rtl"
	doc := &manifold.Document{Metadata: manifold.Metadata{Title: "Asymmetric landscape"}, Layout: manifold.Layout{Width: 480, Height: 420, Margins: &manifold.PageMargins{Top: 70, Right: 60, Bottom: 55, Left: 30}, HeaderDistance: point(12), FooterDistance: point(16)}, Headers: []manifold.Block{header}, Footers: []manifold.Block{para("ASYMMETRIC FOOTER", 8), picture(20, 6)}, Blocks: []manifold.Block{para("BODY START", 11)}, Notes: []manifold.Note{{ID: "page-note", Kind: manifold.Footnote, Blocks: []manifold.Block{note, picture(24, 8), para("NOTE END", 8)}}}}
	table := manifold.Block{Kind: manifold.Table, Rows: []manifold.Row{{RepeatHeader: true, Cells: []manifold.Cell{{Blocks: []manifold.Block{para("ROW", 9)}}, {Blocks: []manifold.Block{para("VALUE", 9)}}}}}}
	for i := 0; i < 24; i++ {
		p := para(fmt.Sprintf("VALUE-%02d", i), 10)
		if i == 4 {
			p.Runs = append(p.Runs, manifold.Run{FontSize: 10, Note: &manifold.NoteReference{Kind: manifold.Footnote, ID: "page-note"}})
		}
		if i == 12 {
			p.PageBreakBefore = true
		}
		table.Rows = append(table.Rows, manifold.Row{Cells: []manifold.Cell{{Blocks: []manifold.Block{para(fmt.Sprintf("ROW-%02d", i), 10)}}, {Blocks: []manifold.Block{p}}}})
	}
	doc.Blocks = append(doc.Blocks, table, manifold.Block{Kind: manifold.PageBreak}, para("BODY END", 11))
	zero := &manifold.Document{Metadata: manifold.Metadata{Title: "Explicit zero edges"}, Layout: manifold.Layout{Width: 240, Height: 300, Margin: 41, Margins: &manifold.PageMargins{}, HeaderDistance: point(0), FooterDistance: point(0)}, Headers: []manifold.Block{para("EDGE HEADER", 8)}, Footers: []manifold.Block{para("EDGE FOOTER", 8)}, Blocks: []manifold.Block{para("ZERO MARGINS", 11), picture(50, 12), {Kind: manifold.PageBreak}, para("SECOND PAGE", 11)}}
	offsets := &manifold.Document{Metadata: manifold.Metadata{Title: "Independent distances"}, Layout: manifold.Layout{Width: 300, Height: 400, Margin: 33, HeaderDistance: point(0), FooterDistance: point(10)}, Headers: []manifold.Block{para("TOP EDGE", 8)}, Footers: []manifold.Block{para("BOTTOM EDGE", 8)}, Blocks: []manifold.Block{para("OFFSET BODY", 11)}}
	for _, item := range []struct {
		name string
		doc  *manifold.Document
	}{{"asymmetric", doc}, {"zero", zero}, {"offsets", offsets}} {
		for _, format := range []manifold.Format{manifold.SF, manifold.DOCX, manifold.PDF} {
			var buf bytes.Buffer
			if err := manifold.Write(context.Background(), &buf, item.doc, format, manifold.WriteOptions{}); err != nil {
				log.Fatal(item.name, ": ", err)
			}
			if err := os.WriteFile(filepath.Join(out, item.name+"."+string(format)), buf.Bytes(), 0644); err != nil {
				log.Fatal(err)
			}
			if format != manifold.SF {
				back, err := manifold.Read(context.Background(), bytes.NewReader(buf.Bytes()), format, manifold.ReadOptions{})
				if err != nil {
					log.Fatal(err)
				}
				var sf bytes.Buffer
				if err := manifold.Write(context.Background(), &sf, back, manifold.SF, manifold.WriteOptions{}); err != nil {
					log.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(out, item.name+"-"+string(format)+".sf"), sf.Bytes(), 0644); err != nil {
					log.Fatal(err)
				}
			}
		}
	}
}
