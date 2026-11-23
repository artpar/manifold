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

	"github.com/artpar/manydoc"
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
	p := func(s string) manydoc.Block {
		return manydoc.Block{Kind: manydoc.Paragraph, Runs: []manydoc.Run{{Text: s}}}
	}
	story := func(s string) *manydoc.RunningStory { return &manydoc.RunningStory{Blocks: []manydoc.Block{p(s)}} }
	point := func(v float64) *float64 { return &v }
	fixed := manydoc.Layout{Width: 300, Height: 400, Margins: &manydoc.PageMargins{Top: -24, Right: 30, Bottom: -24, Left: 30}, HeaderDistance: point(12), FooterDistance: point(12)}
	minimum := fixed
	minimum.Margins = &manydoc.PageMargins{Top: 24, Right: 30, Bottom: 24, Left: 30}
	for _, name := range []string{"fixed-first", "minimum-first", "long-note", "body-floor"} {
		picture := manydoc.Block{Kind: manydoc.Picture, Image: &manydoc.Image{MediaType: "image/png", Data: raster.Bytes(), Width: 100, Height: 370, Alt: "Fixed running background"}}
		blank := manydoc.RunningStories{Default: &manydoc.RunningStory{}}
		a := &manydoc.Section{Layout: &fixed, Headers: manydoc.RunningStories{Default: &manydoc.RunningStory{Blocks: []manydoc.Block{picture}}}, Footers: manydoc.RunningStories{Default: &manydoc.RunningStory{Blocks: []manydoc.Block{picture}}}, Blocks: []manydoc.Block{p("FIXED BODY")}}
		b := &manydoc.Section{Layout: &minimum, FirstPage: true, Headers: manydoc.RunningStories{Default: story("MIN DEFAULT"), First: story("MIN FIRST")}, Footers: blank, Blocks: []manydoc.Block{p("MIN BODY")}}
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
		first.Blocks[0].Runs = append(first.Blocks[0].Runs, manydoc.Run{Note: &manydoc.NoteReference{Kind: manydoc.Footnote, ID: "old"}})
		second.Blocks[0].Runs[0].Link = "https://example.com/admission"
		second.Start = manydoc.SectionContinuous
		third := &manydoc.Section{Start: manydoc.SectionContinuous, Layout: second.Layout, FirstPage: true, Headers: manydoc.RunningStories{Default: story("THIRD HEADER"), First: story("THIRD FIRST")}, Footers: blank, Blocks: []manydoc.Block{p("SHARED BODY"), {Kind: manydoc.PageBreak}, p("THIRD NEXT")}}
		var note []manydoc.Block
		count := 1
		if name == "long-note" {
			count = 32
		}
		for i := 0; i < count; i++ {
			note = append(note, p(fmt.Sprintf("OLD NOTE %02d", i)))
		}
		d := &manydoc.Document{Metadata: manydoc.Metadata{Title: "Continuous admission: " + name}, Blocks: []manydoc.Block{{Kind: manydoc.SectionBlock, Section: first}, {Kind: manydoc.SectionBlock, Section: second}, {Kind: manydoc.SectionBlock, Section: third}}, Notes: []manydoc.Note{{Kind: manydoc.Footnote, ID: "old", Blocks: note}}}
		for _, f := range []struct {
			suffix string
			format manydoc.Format
		}{{".sf", manydoc.SF}, {".docx", manydoc.DOCX}, {".pdf", manydoc.PDF}} {
			var buf bytes.Buffer
			must(manydoc.Write(ctx, &buf, d, f.format, manydoc.WriteOptions{}))
			must(os.WriteFile(filepath.Join(dir, name+f.suffix), buf.Bytes(), 0644))
		}
		for _, f := range []struct {
			suffix, out string
			format      manydoc.Format
		}{{".docx", "-docx.sf", manydoc.DOCX}, {".pdf", "-pdf.sf", manydoc.PDF}} {
			data, err := os.ReadFile(filepath.Join(dir, name+f.suffix))
			must(err)
			back, err := manydoc.Read(ctx, bytes.NewReader(data), f.format, manydoc.ReadOptions{PDFSkipInference: true})
			must(err)
			var buf bytes.Buffer
			must(manydoc.Write(ctx, &buf, back, manydoc.SF, manydoc.WriteOptions{}))
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
