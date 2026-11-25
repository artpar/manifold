// Create, edit and convert slides through the central SF reader/writer API.
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
	dir := "output/presentation"
	must(os.MkdirAll(dir, 0755))
	pixels := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := range 64 {
		for x := range 64 {
			pixels.SetRGBA(x, y, color.RGBA{R: 220, G: uint8(x * 3), B: uint8(y * 3), A: 255})
		}
	}
	var picture bytes.Buffer
	must(png.Encode(&picture, pixels))
	d := &manifold.Document{Metadata: manifold.Metadata{Title: "Central SF slides", Author: "Manifold example"}, Presentation: &manifold.Presentation{Version: 1, Width: 720, Height: 540, Slides: []manifold.PresentationSlide{{ID: "256", Background: "#F4F6FA", Objects: []manifold.SlideObject{
		{ID: "2", Kind: "shape", Name: "Title", X: 40, Y: 40, Width: 600, Height: 70, Geometry: "rect", Content: []manifold.Block{{Kind: manifold.Paragraph, Runs: []manifold.Run{{Text: "Slides through central SF", Bold: true, FontSize: 24, FontFamily: "Go", Color: "#2255AA"}}}}},
		{ID: "3", Kind: "image", Name: "Pixels", X: 40, Y: 180, Width: 120, Height: 120, Image: &manifold.Image{Data: picture.Bytes(), MediaType: "image/png", Alt: "Source pixel image"}},
		{ID: "4", Kind: "shape", Name: "Vector ellipse", X: 300, Y: 180, Width: 150, Height: 100, Geometry: "ellipse", Fill: "#00AA88", Stroke: "#112233", StrokeWidth: 2},
	}, Notes: []manifold.Block{{Kind: manifold.Paragraph, Runs: []manifold.Run{{Text: "Speaker notes stay in SF and PPTX."}}}}}}}}
	write := func(name string, d *manifold.Document, f manifold.Format) []byte {
		var out bytes.Buffer
		must(manifold.Write(ctx, &out, d, f, manifold.WriteOptions{AllowLossy: true}))
		must(os.WriteFile(filepath.Join(dir, name), out.Bytes(), 0644))
		return out.Bytes()
	}
	read := func(data []byte, f manifold.Format) *manifold.Document {
		d, err := manifold.Read(ctx, bytes.NewReader(data), f, manifold.ReadOptions{})
		must(err)
		for _, diagnostic := range d.Diagnostics {
			fmt.Fprintf(os.Stderr, "%s: %s\n", diagnostic.Code, diagnostic.Message)
		}
		return d
	}
	pptx := write("source.pptx", d, manifold.PPTX)
	sf := write("source.sf", read(pptx, manifold.PPTX), manifold.SF)
	edited := read(sf, manifold.SF)
	edited.Presentation.Slides[0].Objects[0].Content[0].Runs[0].Text = "Edited through central SF"
	edited.Presentation.Slides[0].Objects[0].X = 55
	write("edited.pptx", edited, manifold.PPTX)
	write("edited.sf", edited, manifold.SF)
	pdf := write("edited.pdf", edited, manifold.PDF)
	write("pdf-recovered.pptx", read(pdf, manifold.PDF), manifold.PPTX)
	fmt.Println("Wrote PPTX/SF/edited PPTX/vector PDF/PDF-recovered PPTX to", dir)
}
func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
