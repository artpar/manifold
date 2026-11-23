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

	"github.com/artpar/manydoc"
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
	picture := func(data []byte, w, h float64) manydoc.Block {
		return manydoc.Block{Kind: manydoc.Picture, Image: &manydoc.Image{Data: data, MediaType: "image/png", Alt: "Original red and blue logo", Width: w, Height: h}}
	}
	para := func(text string, size float64) manydoc.Block {
		return manydoc.Block{Kind: manydoc.Paragraph, Runs: []manydoc.Run{{Text: text, FontSize: size}}}
	}
	headerLink := para("HEADER LINK", 9)
	headerLink.Runs[0].Link = "https://example.invalid/header"
	headerLink.Runs[0].Color = "#004499"
	title := manydoc.Block{Kind: manydoc.Heading, Level: 2, Runs: []manydoc.Run{{Text: "LETTERHEAD", FontFamily: "Go Mono", FontSize: 10}}}
	nested := manydoc.Block{Kind: manydoc.Table, Rows: []manydoc.Row{{Cells: []manydoc.Cell{{ColSpan: 2, Blocks: []manydoc.Block{para("DEPARTMENT", 8)}}}}, {Cells: []manydoc.Cell{{Blocks: []manydoc.Block{para("ENGINEERING", 8)}}, {Blocks: []manydoc.Block{para("REFERENCE", 8)}}}}}}
	header := manydoc.Block{Kind: manydoc.Table, Rows: []manydoc.Row{
		{RepeatHeader: true, Cells: []manydoc.Cell{{RowSpan: 2, Blocks: []manydoc.Block{picture(logo, 80, 40)}}, {Blocks: []manydoc.Block{title}}}},
		{RepeatHeader: true, Cells: []manydoc.Cell{{Blocks: []manydoc.Block{headerLink}}}},
		{Cells: []manydoc.Cell{{ColSpan: 2, Blocks: []manydoc.Block{nested}}}},
	}}
	footerLink := para("FOOTER LINK", 9)
	footerLink.Runs[0].Link = "https://example.invalid/footer"
	footerList := manydoc.Block{Kind: manydoc.ListBlock, List: "bullet", Items: []manydoc.ListItem{{Blocks: []manydoc.Block{para("FOOTER ITEM", 8)}}}}
	doc := &manydoc.Document{Metadata: manydoc.Metadata{Title: "Rich running content"}, Layout: manydoc.Layout{Width: 420, Height: 560, Margin: 48}, Headers: []manydoc.Block{header}, Footers: []manydoc.Block{footerLink, picture(logo, 40, 20), footerList}, Blocks: []manydoc.Block{para("BODY BEGIN", 11)}, Notes: []manydoc.Note{{Kind: manydoc.Footnote, ID: "running-note", Blocks: []manydoc.Block{para("NOTE STAYS ABOVE THE FOOTER", 8), picture(logo, 24, 12), para("NOTE END", 8)}}}}
	table := manydoc.Block{Kind: manydoc.Table, Rows: []manydoc.Row{{RepeatHeader: true, Cells: []manydoc.Cell{{Blocks: []manydoc.Block{para("ROW", 9)}}, {Blocks: []manydoc.Block{para("VALUE", 9)}}}}}}
	for i := 0; i < 30; i++ {
		p := para(fmt.Sprintf("VALUE-%02d", i), 10)
		if i == 8 {
			p.Runs = append(p.Runs, manydoc.Run{Note: &manydoc.NoteReference{Kind: manydoc.Footnote, ID: "running-note"}, FontSize: 10})
		}
		if i == 20 {
			p.PageBreakBefore = true
		}
		table.Rows = append(table.Rows, manydoc.Row{Cells: []manydoc.Cell{{Blocks: []manydoc.Block{para(fmt.Sprintf("ROW-%02d", i), 10)}}, {Blocks: []manydoc.Block{p}}}})
	}
	doc.Blocks = append(doc.Blocks, table, para("BODY END", 11))
	write := func(name string, d *manydoc.Document) {
		for _, format := range []manydoc.Format{manydoc.SF, manydoc.DOCX, manydoc.PDF} {
			var data bytes.Buffer
			if err := manydoc.Write(context.Background(), &data, d, format, manydoc.WriteOptions{}); err != nil {
				log.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(out, name+"."+string(format)), data.Bytes(), 0644); err != nil {
				log.Fatal(err)
			}
			if format != manydoc.SF {
				back, err := manydoc.Read(context.Background(), bytes.NewReader(data.Bytes()), format, manydoc.ReadOptions{})
				if err != nil {
					log.Fatal(err)
				}
				var sf bytes.Buffer
				if err := manydoc.Write(context.Background(), &sf, back, manydoc.SF, manydoc.WriteOptions{}); err != nil {
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
	write("tight", &manydoc.Document{Layout: manydoc.Layout{Width: 220, Height: 220, Margin: 20}, Headers: []manydoc.Block{tall}, Footers: []manydoc.Block{tall}, Blocks: []manydoc.Block{para("ONLY BODY", 11)}})
}
