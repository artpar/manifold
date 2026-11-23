// Budgeted DOCX running rasters, including serial, nested and body figures.
package main

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"log"
	"os"
	"path/filepath"

	"github.com/artpar/manydoc"
)

func main() {
	out := "output/running-images"
	if err := os.MkdirAll(out, 0755); err != nil {
		log.Fatal(err)
	}
	makeImage := func(jpg bool) []byte {
		im := image.NewNRGBA(image.Rect(0, 0, 16, 128))
		for y := 0; y < 128; y++ {
			for x := 0; x < 16; x++ {
				v := color.NRGBA{200, 30, 50, 255}
				if x >= 8 {
					v = color.NRGBA{20, 80, 220, 128}
				}
				im.SetNRGBA(x, y, v)
			}
		}
		var buf bytes.Buffer
		var err error
		if jpg {
			err = jpeg.Encode(&buf, im, &jpeg.Options{Quality: 85})
		} else {
			err = png.Encode(&buf, im)
		}
		if err != nil {
			log.Fatal(err)
		}
		return buf.Bytes()
	}
	pngData, jpegData := makeImage(false), makeImage(true)
	picture := func(jpg bool, height float64) manydoc.Block {
		data, media := pngData, "image/png"
		if jpg {
			data, media = jpegData, "image/jpeg"
		}
		return manydoc.Block{Kind: manydoc.Picture, Image: &manydoc.Image{Data: data, MediaType: media, Height: height, Alt: "Original red and blue running picture"}}
	}
	para := func(text string, size float64) manydoc.Block {
		return manydoc.Block{Kind: manydoc.Paragraph, Runs: []manydoc.Run{{Text: text, FontSize: size}}}
	}
	cell := func(bs []manydoc.Block) manydoc.Block {
		return manydoc.Block{Kind: manydoc.Table, Rows: []manydoc.Row{{Cells: []manydoc.Cell{{Blocks: bs}}}}}
	}
	tall := picture(false, 1000)
	cases := map[string]*manydoc.Document{}
	cases["serial"] = &manydoc.Document{Layout: manydoc.Layout{Width: 420, Height: 560, Margin: 36}, Headers: []manydoc.Block{tall, tall, tall}, Footers: []manydoc.Block{tall, tall, tall}, Blocks: []manydoc.Block{para("SERIAL BODY", 11)}}
	headerLink := para("HEADER LINK", 10)
	headerLink.Runs[0].Link = "https://example.invalid/header"
	headerLink.Runs[0].Color = "#224466"
	nested := cell([]manydoc.Block{{Kind: manydoc.Table, Rows: []manydoc.Row{
		{RepeatHeader: true, Cells: []manydoc.Cell{{ColSpan: 2, Blocks: []manydoc.Block{headerLink, tall, tall, tall}}}},
		{Cells: []manydoc.Cell{{Blocks: []manydoc.Block{para("LEFT", 8)}}, {Blocks: []manydoc.Block{para("RIGHT", 8)}}}},
	}}})
	cases["nested"] = &manydoc.Document{Layout: manydoc.Layout{Width: 420, Height: 560, Margin: 36}, Headers: []manydoc.Block{nested}, Footers: []manydoc.Block{nested}, Blocks: []manydoc.Block{para("NESTED BODY", 24)}}
	logo := picture(false, 5)
	logo.Image.Width = 10
	cases["body"] = &manydoc.Document{Layout: manydoc.Layout{Width: 300, Height: 400, Margin: 30}, Headers: []manydoc.Block{tall}, Footers: []manydoc.Block{logo}, Blocks: []manydoc.Block{cell([]manydoc.Block{cell([]manydoc.Block{tall})}), para("BODY END", 11)}}
	jpg := picture(true, 1000)
	cases["jpeg"] = &manydoc.Document{Layout: manydoc.Layout{Width: 220, Height: 220, Margin: 20}, Headers: []manydoc.Block{jpg}, Footers: []manydoc.Block{jpg}, Blocks: []manydoc.Block{para("JPEG BODY", 11)}}
	for _, name := range []string{"serial", "nested", "body", "jpeg"} {
		doc := cases[name]
		doc.Metadata.Title = "Running images: " + name
		for _, format := range []manydoc.Format{manydoc.SF, manydoc.DOCX} {
			var buf bytes.Buffer
			if err := manydoc.Write(context.Background(), &buf, doc, format, manydoc.WriteOptions{}); err != nil {
				log.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(out, name+"."+string(format)), buf.Bytes(), 0644); err != nil {
				log.Fatal(err)
			}
			if format == manydoc.DOCX {
				back, err := manydoc.ReadDOCX(context.Background(), bytes.NewReader(buf.Bytes()))
				if err != nil {
					log.Fatal(err)
				}
				var sf bytes.Buffer
				if err := manydoc.Write(context.Background(), &sf, back, manydoc.SF, manydoc.WriteOptions{}); err != nil {
					log.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(out, name+"-docx.sf"), sf.Bytes(), 0644); err != nil {
					log.Fatal(err)
				}
			}
		}
	}
}
