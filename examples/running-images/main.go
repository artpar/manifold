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

	"github.com/artpar/manifold"
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
	picture := func(jpg bool, height float64) manifold.Block {
		data, media := pngData, "image/png"
		if jpg {
			data, media = jpegData, "image/jpeg"
		}
		return manifold.Block{Kind: manifold.Picture, Image: &manifold.Image{Data: data, MediaType: media, Height: height, Alt: "Original red and blue running picture"}}
	}
	para := func(text string, size float64) manifold.Block {
		return manifold.Block{Kind: manifold.Paragraph, Runs: []manifold.Run{{Text: text, FontSize: size}}}
	}
	cell := func(bs []manifold.Block) manifold.Block {
		return manifold.Block{Kind: manifold.Table, Rows: []manifold.Row{{Cells: []manifold.Cell{{Blocks: bs}}}}}
	}
	tall := picture(false, 1000)
	cases := map[string]*manifold.Document{}
	cases["serial"] = &manifold.Document{Layout: manifold.Layout{Width: 420, Height: 560, Margin: 36}, Headers: []manifold.Block{tall, tall, tall}, Footers: []manifold.Block{tall, tall, tall}, Blocks: []manifold.Block{para("SERIAL BODY", 11)}}
	headerLink := para("HEADER LINK", 10)
	headerLink.Runs[0].Link = "https://example.invalid/header"
	headerLink.Runs[0].Color = "#224466"
	nested := cell([]manifold.Block{{Kind: manifold.Table, Rows: []manifold.Row{
		{RepeatHeader: true, Cells: []manifold.Cell{{ColSpan: 2, Blocks: []manifold.Block{headerLink, tall, tall, tall}}}},
		{Cells: []manifold.Cell{{Blocks: []manifold.Block{para("LEFT", 8)}}, {Blocks: []manifold.Block{para("RIGHT", 8)}}}},
	}}})
	cases["nested"] = &manifold.Document{Layout: manifold.Layout{Width: 420, Height: 560, Margin: 36}, Headers: []manifold.Block{nested}, Footers: []manifold.Block{nested}, Blocks: []manifold.Block{para("NESTED BODY", 24)}}
	logo := picture(false, 5)
	logo.Image.Width = 10
	cases["body"] = &manifold.Document{Layout: manifold.Layout{Width: 300, Height: 400, Margin: 30}, Headers: []manifold.Block{tall}, Footers: []manifold.Block{logo}, Blocks: []manifold.Block{cell([]manifold.Block{cell([]manifold.Block{tall})}), para("BODY END", 11)}}
	jpg := picture(true, 1000)
	cases["jpeg"] = &manifold.Document{Layout: manifold.Layout{Width: 220, Height: 220, Margin: 20}, Headers: []manifold.Block{jpg}, Footers: []manifold.Block{jpg}, Blocks: []manifold.Block{para("JPEG BODY", 11)}}
	for _, name := range []string{"serial", "nested", "body", "jpeg"} {
		doc := cases[name]
		doc.Metadata.Title = "Running images: " + name
		for _, format := range []manifold.Format{manifold.SF, manifold.DOCX} {
			var buf bytes.Buffer
			if err := manifold.Write(context.Background(), &buf, doc, format, manifold.WriteOptions{}); err != nil {
				log.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(out, name+"."+string(format)), buf.Bytes(), 0644); err != nil {
				log.Fatal(err)
			}
			if format == manifold.DOCX {
				back, err := manifold.ReadDOCX(context.Background(), bytes.NewReader(buf.Bytes()))
				if err != nil {
					log.Fatal(err)
				}
				var sf bytes.Buffer
				if err := manifold.Write(context.Background(), &sf, back, manifold.SF, manifold.WriteOptions{}); err != nil {
					log.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(out, name+"-docx.sf"), sf.Bytes(), 0644); err != nil {
					log.Fatal(err)
				}
			}
		}
	}
}
