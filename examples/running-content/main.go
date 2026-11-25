// Rich running stories are measured once and repeated without body overlap.
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
	out := "output/running-content"
	if err := os.MkdirAll(out, 0755); err != nil {
		log.Fatal(err)
	}
	imageData := func(w, h int) []byte {
		im := image.NewNRGBA(image.Rect(0, 0, w, h))
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				v := color.NRGBA{200, 30, 50, 255}
				if x >= w/2 {
					v = color.NRGBA{20, 80, 220, 128}
				}
				im.SetNRGBA(x, y, v)
			}
		}
		var b bytes.Buffer
		if err := png.Encode(&b, im); err != nil {
			log.Fatal(err)
		}
		return b.Bytes()
	}
	logo := imageData(32, 16)
	picture := func(data []byte, w, h float64) manifold.Block {
		return manifold.Block{Kind: manifold.Picture, Image: &manifold.Image{Data: data, MediaType: "image/png", Alt: "Original red and blue logo", Width: w, Height: h}}
	}
	para := func(text string, size float64) manifold.Block {
		return manifold.Block{Kind: manifold.Paragraph, Runs: []manifold.Run{{Text: text, FontSize: size}}}
	}
	headerLink := para("HEADER LINK", 9)
	headerLink.Runs[0].Link = "https://example.invalid/header"
	headerLink.Runs[0].Color = "#004499"
	title := manifold.Block{Kind: manifold.Heading, Level: 2, Runs: []manifold.Run{{Text: "LETTERHEAD", FontFamily: "Go Mono", FontSize: 10}}}
	nested := manifold.Block{Kind: manifold.Table, Rows: []manifold.Row{{Cells: []manifold.Cell{{ColSpan: 2, Blocks: []manifold.Block{para("DEPARTMENT", 8)}}}}, {Cells: []manifold.Cell{{Blocks: []manifold.Block{para("ENGINEERING", 8)}}, {Blocks: []manifold.Block{para("REFERENCE", 8)}}}}}}
	header := manifold.Block{Kind: manifold.Table, Rows: []manifold.Row{
		{RepeatHeader: true, Cells: []manifold.Cell{{RowSpan: 2, Blocks: []manifold.Block{picture(logo, 80, 40)}}, {Blocks: []manifold.Block{title}}}},
		{RepeatHeader: true, Cells: []manifold.Cell{{Blocks: []manifold.Block{headerLink}}}},
		{Cells: []manifold.Cell{{ColSpan: 2, Blocks: []manifold.Block{nested}}}},
	}}
	footerLink := para("FOOTER LINK", 9)
	footerLink.Runs[0].Link = "https://example.invalid/footer"
	footerList := manifold.Block{Kind: manifold.ListBlock, List: "bullet", Items: []manifold.ListItem{{Blocks: []manifold.Block{para("FOOTER ITEM", 8)}}}}
	doc := &manifold.Document{Metadata: manifold.Metadata{Title: "Rich running content"}, Layout: manifold.Layout{Width: 420, Height: 560, Margin: 48}, Headers: []manifold.Block{header}, Footers: []manifold.Block{footerLink, picture(logo, 40, 20), footerList}, Blocks: []manifold.Block{para("BODY BEGIN", 11)}, Notes: []manifold.Note{{Kind: manifold.Footnote, ID: "running-note", Blocks: []manifold.Block{para("NOTE STAYS ABOVE THE FOOTER", 8), picture(logo, 24, 12), para("NOTE END", 8)}}}}
	table := manifold.Block{Kind: manifold.Table, Rows: []manifold.Row{{RepeatHeader: true, Cells: []manifold.Cell{{Blocks: []manifold.Block{para("ROW", 9)}}, {Blocks: []manifold.Block{para("VALUE", 9)}}}}}}
	for i := 0; i < 30; i++ {
		p := para(fmt.Sprintf("VALUE-%02d", i), 10)
		if i == 8 {
			p.Runs = append(p.Runs, manifold.Run{Note: &manifold.NoteReference{Kind: manifold.Footnote, ID: "running-note"}, FontSize: 10})
		}
		if i == 20 {
			p.PageBreakBefore = true
		}
		table.Rows = append(table.Rows, manifold.Row{Cells: []manifold.Cell{{Blocks: []manifold.Block{para(fmt.Sprintf("ROW-%02d", i), 10)}}, {Blocks: []manifold.Block{p}}}})
	}
	doc.Blocks = append(doc.Blocks, table, para("BODY END", 11))
	write := func(name string, d *manifold.Document) {
		for _, format := range []manifold.Format{manifold.SF, manifold.DOCX, manifold.PDF} {
			var data bytes.Buffer
			if err := manifold.Write(context.Background(), &data, d, format, manifold.WriteOptions{}); err != nil {
				log.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(out, name+"."+string(format)), data.Bytes(), 0644); err != nil {
				log.Fatal(err)
			}
			if format != manifold.SF {
				back, err := manifold.Read(context.Background(), bytes.NewReader(data.Bytes()), format, manifold.ReadOptions{})
				if err != nil {
					log.Fatal(err)
				}
				var sf bytes.Buffer
				if err := manifold.Write(context.Background(), &sf, back, manifold.SF, manifold.WriteOptions{}); err != nil {
					log.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(out, name+"-"+string(format)+".sf"), sf.Bytes(), 0644); err != nil {
					log.Fatal(err)
				}
			}
		}
	}
	write("letterhead", doc)
	tall := picture(imageData(16, 128), 0, 1000)
	write("tight", &manifold.Document{Layout: manifold.Layout{Width: 220, Height: 220, Margin: 20}, Headers: []manifold.Block{tall}, Footers: []manifold.Block{tall}, Blocks: []manifold.Block{para("ONLY BODY", 11)}})
}
