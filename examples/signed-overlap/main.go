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

	"github.com/artpar/manydoc"
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
	picture := func(data []byte, height float64) manydoc.Block {
		return manydoc.Block{Kind: manydoc.Picture, Image: &manydoc.Image{MediaType: "image/png", Data: data, Alt: "Half-transparent running overlay", Width: 200, Height: height}}
	}
	point := func(v float64) *float64 { return &v }
	for _, spec := range []struct {
		name        string
		top, bottom float64
	}{{"fixed", -24, -24}, {"minimum", 24, 24}, {"fixed-top", -24, 24}, {"fixed-bottom", 24, -24}, {"large-fixed", -24, -24}} {
		d := &manydoc.Document{Metadata: manydoc.Metadata{Title: "Signed running overlap: " + spec.name}, Layout: manydoc.Layout{Width: 300, Height: 400, Margins: &manydoc.PageMargins{Top: spec.top, Right: 30, Bottom: spec.bottom, Left: 30}, HeaderDistance: point(12), FooterDistance: point(12)}, Headers: []manydoc.Block{picture(blue, 110)}, Footers: []manydoc.Block{picture(red, 120)}}
		if spec.name == "large-fixed" {
			for _, story := range [][]manydoc.Block{d.Headers, d.Footers} {
				story[0].Image.Width, story[0].Image.Height = 100, 370
			}
		}
		for i := 0; i < 12; i++ {
			b := manydoc.Block{Kind: manydoc.Paragraph, Runs: []manydoc.Run{{Text: fmt.Sprintf("BODY %02d", i), FontSize: 14}}}
			if i == 9 {
				b.Runs = append(b.Runs, manydoc.Run{Note: &manydoc.NoteReference{ID: "note", Kind: manydoc.Footnote}})
			}
			if i == 10 {
				b.Runs[0].Link = "https://example.com/overlap"
			}
			d.Blocks = append(d.Blocks, b)
		}
		d.Notes = []manydoc.Note{{ID: "note", Kind: manydoc.Footnote, Blocks: []manydoc.Block{{Kind: manydoc.Paragraph, Runs: []manydoc.Run{{Text: "NOTE IN BODY FRAME", FontSize: 10}}}}}}
		for _, f := range []struct {
			suffix string
			format manydoc.Format
		}{{".sf", manydoc.SF}, {".docx", manydoc.DOCX}, {".pdf", manydoc.PDF}} {
			var buf bytes.Buffer
			must(manydoc.Write(ctx, &buf, d, f.format, manydoc.WriteOptions{}))
			must(os.WriteFile(filepath.Join(dir, spec.name+f.suffix), buf.Bytes(), 0644))
		}
		for _, f := range []struct {
			suffix, out string
			format      manydoc.Format
		}{{".docx", "-docx.sf", manydoc.DOCX}, {".pdf", "-pdf.sf", manydoc.PDF}} {
			data, err := os.ReadFile(filepath.Join(dir, spec.name+f.suffix))
			must(err)
			back, err := manydoc.Read(ctx, bytes.NewReader(data), f.format, manydoc.ReadOptions{PDFSkipInference: true})
			must(err)
			var buf bytes.Buffer
			must(manydoc.Write(ctx, &buf, back, manydoc.SF, manydoc.WriteOptions{}))
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
