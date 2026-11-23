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

	"github.com/artpar/manydoc"
)

func main() {
	out := "output/page-layout"
	if err := os.MkdirAll(out, 0755); err != nil {
		log.Fatal(err)
	}
	point := func(n float64) *float64 { return &n }
	para := func(text string, size float64) manydoc.Block {
		return manydoc.Block{Kind: manydoc.Paragraph, Runs: []manydoc.Run{{Text: text, FontSize: size}}}
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
	picture := func(w, h float64) manydoc.Block {
		return manydoc.Block{Kind: manydoc.Picture, Image: &manydoc.Image{MediaType: "image/png", Data: pngData.Bytes(), Alt: "Original page-layout logo", Width: w, Height: h}}
	}
	headerLink := para("ASYMMETRIC HEADER", 8)
	headerLink.Runs[0].Link = "https://example.invalid/page-header"
	headerLink.Runs[0].Color = "#224466"
	headerLink.Runs[0].Underline = true
	header := manydoc.Block{Kind: manydoc.Table, Rows: []manydoc.Row{{Cells: []manydoc.Cell{{ColSpan: 2, Blocks: []manydoc.Block{headerLink}}}}, {Cells: []manydoc.Cell{{Blocks: []manydoc.Block{picture(30, 10)}}, {Blocks: []manydoc.Block{para("RIGHT HEADER", 8)}}}}}}
	note := para("NOTE ABOVE FOOTER", 8)
	note.Direction = "rtl"
	doc := &manydoc.Document{Metadata: manydoc.Metadata{Title: "Asymmetric landscape"}, Layout: manydoc.Layout{Width: 480, Height: 420, Margins: &manydoc.PageMargins{Top: 70, Right: 60, Bottom: 55, Left: 30}, HeaderDistance: point(12), FooterDistance: point(16)}, Headers: []manydoc.Block{header}, Footers: []manydoc.Block{para("ASYMMETRIC FOOTER", 8), picture(20, 6)}, Blocks: []manydoc.Block{para("BODY START", 11)}, Notes: []manydoc.Note{{ID: "page-note", Kind: manydoc.Footnote, Blocks: []manydoc.Block{note, picture(24, 8), para("NOTE END", 8)}}}}
	table := manydoc.Block{Kind: manydoc.Table, Rows: []manydoc.Row{{RepeatHeader: true, Cells: []manydoc.Cell{{Blocks: []manydoc.Block{para("ROW", 9)}}, {Blocks: []manydoc.Block{para("VALUE", 9)}}}}}}
	for i := 0; i < 24; i++ {
		p := para(fmt.Sprintf("VALUE-%02d", i), 10)
		if i == 4 {
			p.Runs = append(p.Runs, manydoc.Run{FontSize: 10, Note: &manydoc.NoteReference{Kind: manydoc.Footnote, ID: "page-note"}})
		}
		if i == 12 {
			p.PageBreakBefore = true
		}
		table.Rows = append(table.Rows, manydoc.Row{Cells: []manydoc.Cell{{Blocks: []manydoc.Block{para(fmt.Sprintf("ROW-%02d", i), 10)}}, {Blocks: []manydoc.Block{p}}}})
	}
	doc.Blocks = append(doc.Blocks, table, manydoc.Block{Kind: manydoc.PageBreak}, para("BODY END", 11))
	zero := &manydoc.Document{Metadata: manydoc.Metadata{Title: "Explicit zero edges"}, Layout: manydoc.Layout{Width: 240, Height: 300, Margin: 41, Margins: &manydoc.PageMargins{}, HeaderDistance: point(0), FooterDistance: point(0)}, Headers: []manydoc.Block{para("EDGE HEADER", 8)}, Footers: []manydoc.Block{para("EDGE FOOTER", 8)}, Blocks: []manydoc.Block{para("ZERO MARGINS", 11), picture(50, 12), {Kind: manydoc.PageBreak}, para("SECOND PAGE", 11)}}
	offsets := &manydoc.Document{Metadata: manydoc.Metadata{Title: "Independent distances"}, Layout: manydoc.Layout{Width: 300, Height: 400, Margin: 33, HeaderDistance: point(0), FooterDistance: point(10)}, Headers: []manydoc.Block{para("TOP EDGE", 8)}, Footers: []manydoc.Block{para("BOTTOM EDGE", 8)}, Blocks: []manydoc.Block{para("OFFSET BODY", 11)}}
	for _, item := range []struct {
		name string
		doc  *manydoc.Document
	}{{"asymmetric", doc}, {"zero", zero}, {"offsets", offsets}} {
		for _, format := range []manydoc.Format{manydoc.SF, manydoc.DOCX, manydoc.PDF} {
			var buf bytes.Buffer
			if err := manydoc.Write(context.Background(), &buf, item.doc, format, manydoc.WriteOptions{}); err != nil {
				log.Fatal(item.name, ": ", err)
			}
			if err := os.WriteFile(filepath.Join(out, item.name+"."+string(format)), buf.Bytes(), 0644); err != nil {
				log.Fatal(err)
			}
			if format != manydoc.SF {
				back, err := manydoc.Read(context.Background(), bytes.NewReader(buf.Bytes()), format, manydoc.ReadOptions{})
				if err != nil {
					log.Fatal(err)
				}
				var sf bytes.Buffer
				if err := manydoc.Write(context.Background(), &sf, back, manydoc.SF, manydoc.WriteOptions{}); err != nil {
					log.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(out, item.name+"-"+string(format)+".sf"), sf.Bytes(), 0644); err != nil {
					log.Fatal(err)
				}
			}
		}
	}
}
