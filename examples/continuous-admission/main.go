// Four conversion paths for individually valid continuous stories needing new pages.
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
	dir := "output/continuous-admission"
	must(os.MkdirAll(dir, 0755))
	canvas := image.NewNRGBA(image.Rect(0, 0, 32, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 32; x++ {
			canvas.SetNRGBA(x, y, color.NRGBA{R: 20, G: 100, B: 180, A: 128})
		}
	}
	var raster bytes.Buffer
	must(png.Encode(&raster, canvas))
	p := func(s string) manifold.Block {
		return manifold.Block{Kind: manifold.Paragraph, Runs: []manifold.Run{{Text: s}}}
	}
	story := func(s string) *manifold.RunningStory { return &manifold.RunningStory{Blocks: []manifold.Block{p(s)}} }
	point := func(v float64) *float64 { return &v }
	fixed := manifold.Layout{Width: 300, Height: 400, Margins: &manifold.PageMargins{Top: -24, Right: 30, Bottom: -24, Left: 30}, HeaderDistance: point(12), FooterDistance: point(12)}
	minimum := fixed
	minimum.Margins = &manifold.PageMargins{Top: 24, Right: 30, Bottom: 24, Left: 30}
	for _, name := range []string{"fixed-first", "minimum-first", "long-note", "body-floor"} {
		picture := manifold.Block{Kind: manifold.Picture, Image: &manifold.Image{MediaType: "image/png", Data: raster.Bytes(), Width: 100, Height: 370, Alt: "Fixed running background"}}
		blank := manifold.RunningStories{Default: &manifold.RunningStory{}}
		a := &manifold.Section{Layout: &fixed, Headers: manifold.RunningStories{Default: &manifold.RunningStory{Blocks: []manifold.Block{picture}}}, Footers: manifold.RunningStories{Default: &manifold.RunningStory{Blocks: []manifold.Block{picture}}}, Blocks: []manifold.Block{p("FIXED BODY")}}
		b := &manifold.Section{Layout: &minimum, FirstPage: true, Headers: manifold.RunningStories{Default: story("MIN DEFAULT"), First: story("MIN FIRST")}, Footers: blank, Blocks: []manifold.Block{p("MIN BODY")}}
		if name == "body-floor" {
			picture.Image.Height = 340
			a.Footers = blank
			b.Blocks[0] = p("BIG")
			b.Blocks[0].Runs[0].FontSize = 40
		}
		first, second := a, b
		if name == "minimum-first" {
			first, second = b, a
		}
		first.Blocks[0].Runs = append(first.Blocks[0].Runs, manifold.Run{Note: &manifold.NoteReference{Kind: manifold.Footnote, ID: "old"}})
		second.Blocks[0].Runs[0].Link = "https://example.com/admission"
		second.Start = manifold.SectionContinuous
		third := &manifold.Section{Start: manifold.SectionContinuous, Layout: second.Layout, FirstPage: true, Headers: manifold.RunningStories{Default: story("THIRD HEADER"), First: story("THIRD FIRST")}, Footers: blank, Blocks: []manifold.Block{p("SHARED BODY"), {Kind: manifold.PageBreak}, p("THIRD NEXT")}}
		var note []manifold.Block
		count := 1
		if name == "long-note" {
			count = 32
		}
		for i := 0; i < count; i++ {
			note = append(note, p(fmt.Sprintf("OLD NOTE %02d", i)))
		}
		d := &manifold.Document{Metadata: manifold.Metadata{Title: "Continuous admission: " + name}, Blocks: []manifold.Block{{Kind: manifold.SectionBlock, Section: first}, {Kind: manifold.SectionBlock, Section: second}, {Kind: manifold.SectionBlock, Section: third}}, Notes: []manifold.Note{{Kind: manifold.Footnote, ID: "old", Blocks: note}}}
		for _, f := range []struct {
			suffix string
			format manifold.Format
		}{{".sf", manifold.SF}, {".docx", manifold.DOCX}, {".pdf", manifold.PDF}} {
			var buf bytes.Buffer
			must(manifold.Write(ctx, &buf, d, f.format, manifold.WriteOptions{}))
			must(os.WriteFile(filepath.Join(dir, name+f.suffix), buf.Bytes(), 0644))
		}
		for _, f := range []struct {
			suffix, out string
			format      manifold.Format
		}{{".docx", "-docx.sf", manifold.DOCX}, {".pdf", "-pdf.sf", manifold.PDF}} {
			data, err := os.ReadFile(filepath.Join(dir, name+f.suffix))
			must(err)
			back, err := manifold.Read(ctx, bytes.NewReader(data), f.format, manifold.ReadOptions{PDFSkipInference: true})
			must(err)
			var buf bytes.Buffer
			must(manifold.Write(ctx, &buf, back, manifold.SF, manifold.WriteOptions{}))
			must(os.WriteFile(filepath.Join(dir, name+f.out), buf.Bytes(), 0644))
		}
		source, err := os.ReadFile(filepath.Join(dir, name+".sf"))
		must(err)
		back, err := os.ReadFile(filepath.Join(dir, name+"-docx.sf"))
		must(err)
		if !bytes.Equal(source, back) {
			log.Fatal("continuous start/story semantics changed: ", name)
		}
	}
	fmt.Println("Wrote four continuous-admission SF/DOCX/PDF cases and recovered SF to", dir)
}
func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
