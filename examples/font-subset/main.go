// Exercise all four SF conversion paths with an explicitly supplied TTF.
package main

import (
	"bytes"
	"context"
	"flag"
	"log"
	"os"
	"path/filepath"

	"github.com/artpar/manifold"
)

func main() {
	fontPath := flag.String("font", "testdata/assets/DejaVuSans.ttf", "caller-supplied static TTF")
	family := flag.String("family", "DejaVu Sans", "font family recorded in SF and DOCX")
	text := flag.String("text", "A😀B ÀǺé", "text covered by that font")
	out := flag.String("out", "output/font-subset/example", "output path stem")
	flag.Parse()
	ctx := context.Background()
	data, err := os.ReadFile(*fontPath)
	if err != nil {
		log.Fatal(err)
	}
	d := &manifold.Document{Layout: manifold.Layout{Width: 400, Height: 200, Margin: 24}, Blocks: []manifold.Block{{Kind: manifold.Paragraph, Runs: []manifold.Run{{Text: *text, FontFamily: *family, FontSize: 60}}}}}
	if err := os.MkdirAll(filepath.Dir(*out), 0755); err != nil {
		log.Fatal(err)
	}
	opts := manifold.WriteOptions{PDFFonts: []manifold.FontFace{{Family: *family, Data: data}}}
	write := func(suffix string, d *manifold.Document, format manifold.Format) []byte {
		var b bytes.Buffer
		if err := manifold.Write(ctx, &b, d, format, opts); err != nil {
			log.Fatal(err)
		}
		if err := os.WriteFile(*out+suffix, b.Bytes(), 0644); err != nil {
			log.Fatal(err)
		}
		return b.Bytes()
	}
	raw := write(".sf", d, manifold.SF)
	sf, err := manifold.ReadStructured(ctx, bytes.NewReader(raw))
	if err != nil {
		log.Fatal(err)
	}
	d, err = manifold.FromStructured(sf)
	if err != nil {
		log.Fatal(err)
	}
	word := write(".docx", d, manifold.DOCX)
	pdf := write(".pdf", d, manifold.PDF)
	back, err := manifold.ReadDOCX(ctx, bytes.NewReader(word))
	if err != nil {
		log.Fatal(err)
	}
	write(".from-docx.sf", back, manifold.SF)
	back, err = manifold.ReadPDF(ctx, bytes.NewReader(pdf), manifold.ReadOptions{PDFSkipInference: true})
	if err != nil {
		log.Fatal(err)
	}
	write(".from-pdf.sf", back, manifold.SF)
}
