// Four conversion paths for minimum and signed fixed running/body margins.
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
	dir := "output/signed-overlap"
	must(os.MkdirAll(dir, 0755))
	raster := func(c color.NRGBA) []byte {
		canvas := image.NewNRGBA(image.Rect(0, 0, 32, 16))
		for y := 0; y < 16; y++ {
			for x := 0; x < 32; x++ {
				canvas.SetNRGBA(x, y, c)
			}
		}
		var buf bytes.Buffer
		must(png.Encode(&buf, canvas))
		return buf.Bytes()
	}
	blue, red := raster(color.NRGBA{R: 20, G: 100, B: 180, A: 128}), raster(color.NRGBA{R: 200, G: 30, B: 50, A: 128})
	picture := func(data []byte, height float64) manifold.Block {
		return manifold.Block{Kind: manifold.Picture, Image: &manifold.Image{MediaType: "image/png", Data: data, Alt: "Half-transparent running overlay", Width: 200, Height: height}}
	}
	point := func(v float64) *float64 { return &v }
	for _, spec := range []struct {
		name        string
		top, bottom float64
	}{{"fixed", -24, -24}, {"minimum", 24, 24}, {"fixed-top", -24, 24}, {"fixed-bottom", 24, -24}, {"large-fixed", -24, -24}} {
		d := &manifold.Document{Metadata: manifold.Metadata{Title: "Signed running overlap: " + spec.name}, Layout: manifold.Layout{Width: 300, Height: 400, Margins: &manifold.PageMargins{Top: spec.top, Right: 30, Bottom: spec.bottom, Left: 30}, HeaderDistance: point(12), FooterDistance: point(12)}, Headers: []manifold.Block{picture(blue, 110)}, Footers: []manifold.Block{picture(red, 120)}}
		if spec.name == "large-fixed" {
			for _, story := range [][]manifold.Block{d.Headers, d.Footers} {
				story[0].Image.Width, story[0].Image.Height = 100, 370
			}
		}
		for i := 0; i < 12; i++ {
			b := manifold.Block{Kind: manifold.Paragraph, Runs: []manifold.Run{{Text: fmt.Sprintf("BODY %02d", i), FontSize: 14}}}
			if i == 9 {
				b.Runs = append(b.Runs, manifold.Run{Note: &manifold.NoteReference{ID: "note", Kind: manifold.Footnote}})
			}
			if i == 10 {
				b.Runs[0].Link = "https://example.com/overlap"
			}
			d.Blocks = append(d.Blocks, b)
		}
		d.Notes = []manifold.Note{{ID: "note", Kind: manifold.Footnote, Blocks: []manifold.Block{{Kind: manifold.Paragraph, Runs: []manifold.Run{{Text: "NOTE IN BODY FRAME", FontSize: 10}}}}}}
		for _, f := range []struct {
			suffix string
			format manifold.Format
		}{{".sf", manifold.SF}, {".docx", manifold.DOCX}, {".pdf", manifold.PDF}} {
			var buf bytes.Buffer
			must(manifold.Write(ctx, &buf, d, f.format, manifold.WriteOptions{}))
			must(os.WriteFile(filepath.Join(dir, spec.name+f.suffix), buf.Bytes(), 0644))
		}
		for _, f := range []struct {
			suffix, out string
			format      manifold.Format
		}{{".docx", "-docx.sf", manifold.DOCX}, {".pdf", "-pdf.sf", manifold.PDF}} {
			data, err := os.ReadFile(filepath.Join(dir, spec.name+f.suffix))
			must(err)
			back, err := manifold.Read(ctx, bytes.NewReader(data), f.format, manifold.ReadOptions{PDFSkipInference: true})
			must(err)
			var buf bytes.Buffer
			must(manifold.Write(ctx, &buf, back, manifold.SF, manifold.WriteOptions{}))
			must(os.WriteFile(filepath.Join(dir, spec.name+f.out), buf.Bytes(), 0644))
		}
		source, err := os.ReadFile(filepath.Join(dir, spec.name+".sf"))
		must(err)
		back, err := os.ReadFile(filepath.Join(dir, spec.name+"-docx.sf"))
		must(err)
		if !bytes.Equal(source, back) {
			log.Fatal("source SF changed through DOCX: ", spec.name)
		}
	}
	fmt.Println("Wrote five signed/minimum SF/DOCX/PDF cases and recovered SF to", dir)
}
func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
