// Exercise all four SF conversion paths with an explicitly supplied TTF.
package main

import (
	"bytes"
	"context"
	"flag"
	"log"
	"os"
	"path/filepath"

	"github.com/artpar/manydoc"
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
	d := &manydoc.Document{Layout: manydoc.Layout{Width: 400, Height: 200, Margin: 24}, Blocks: []manydoc.Block{{Kind: manydoc.Paragraph, Runs: []manydoc.Run{{Text: *text, FontFamily: *family, FontSize: 60}}}}}
	if err := os.MkdirAll(filepath.Dir(*out), 0755); err != nil {
		log.Fatal(err)
	}
	opts := manydoc.WriteOptions{PDFFonts: []manydoc.FontFace{{Family: *family, Data: data}}}
	write := func(suffix string, d *manydoc.Document, format manydoc.Format) []byte {
		var b bytes.Buffer
		if err := manydoc.Write(ctx, &b, d, format, opts); err != nil {
			log.Fatal(err)
		}
		if err := os.WriteFile(*out+suffix, b.Bytes(), 0644); err != nil {
			log.Fatal(err)
		}
		return b.Bytes()
	}
	raw := write(".sf", d, manydoc.SF)
	sf, err := manydoc.ReadStructured(ctx, bytes.NewReader(raw))
	if err != nil {
		log.Fatal(err)
	}
	d, err = manydoc.FromStructured(sf)
	if err != nil {
		log.Fatal(err)
	}
	word := write(".docx", d, manydoc.DOCX)
	pdf := write(".pdf", d, manydoc.PDF)
	back, err := manydoc.ReadDOCX(ctx, bytes.NewReader(word))
	if err != nil {
		log.Fatal(err)
	}
	write(".from-docx.sf", back, manydoc.SF)
	back, err = manydoc.ReadPDF(ctx, bytes.NewReader(pdf), manydoc.ReadOptions{PDFSkipInference: true})
	if err != nil {
		log.Fatal(err)
	}
	write(".from-pdf.sf", back, manydoc.SF)
}
