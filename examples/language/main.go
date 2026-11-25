// Language bindings and paragraph/run direction persist through SF and DOCX.
// PDF demonstrates qualified LTR localized glyphs; RTL PDF remains unfinished.
package main

import (
	"bytes"
	"context"
	"log"
	"os"
	"path/filepath"

	"github.com/artpar/manifold"
)

func main() {
	out := "output/language"
	if err := os.MkdirAll(out, 0755); err != nil {
		log.Fatal(err)
	}
	p := manifold.Block{Kind: manifold.Paragraph, Direction: "rtl", Runs: []manifold.Run{{Text: "Paragraph and run directions are independent", ComplexScript: true, Direction: "ltr", Language: "en-GB", EastAsiaLanguage: "ja-JP", ComplexLanguage: "ar-SA", Link: "https://example.invalid/language"}}}
	d := &manifold.Document{Metadata: manifold.Metadata{Title: "Language and direction"}, Layout: manifold.Layout{Width: 595.3, Height: 841.9, Margin: 54}, Blocks: []manifold.Block{p, {Kind: manifold.Table, Rows: []manifold.Row{{Cells: []manifold.Cell{{Blocks: []manifold.Block{p}}}}}}, {Kind: manifold.Paragraph, Direction: "ltr", Runs: []manifold.Run{{Note: &manifold.NoteReference{Kind: manifold.Footnote, ID: "n"}, Language: "fr-CA", Direction: "ltr"}}}}, Headers: []manifold.Block{p}, Footers: []manifold.Block{p}, Notes: []manifold.Note{{Kind: manifold.Footnote, ID: "n", Blocks: []manifold.Block{p}}}}
	write := func(name string, doc *manifold.Document, format manifold.Format, opts manifold.WriteOptions) {
		var b bytes.Buffer
		if err := manifold.Write(context.Background(), &b, doc, format, opts); err != nil {
			log.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(out, name), b.Bytes(), 0644); err != nil {
			log.Fatal(err)
		}
	}
	write("direction.sf", d, manifold.SF, manifold.WriteOptions{})
	write("direction.docx", d, manifold.DOCX, manifold.WriteOptions{})
	font, err := os.ReadFile("testdata/assets/NotoSans-Regular.ttf")
	if err != nil {
		log.Fatal(err)
	}
	localized := &manifold.Document{Metadata: manifold.Metadata{Title: "Localized Cyrillic glyphs"}, Layout: manifold.Layout{Width: 360, Height: 240, Margin: 30}}
	for _, locale := range []string{"ru-RU", "sr-RS", "mk-MK"} {
		localized.Blocks = append(localized.Blocks, manifold.Block{Kind: manifold.Paragraph, Direction: "ltr", Runs: []manifold.Run{{Text: "бгдпт", Language: locale, FontFamily: "Noto Sans", FontSize: 28}}})
	}
	localized.Blocks = append(localized.Blocks, manifold.Block{Kind: manifold.Paragraph, Direction: "ltr", Runs: []manifold.Run{{Text: "бгдпт", Language: "ru-RU", ComplexLanguage: "sr-RS", ComplexScript: true, FontFamily: "Noto Sans", FontSize: 28}}})
	write("localized.sf", localized, manifold.SF, manifold.WriteOptions{})
	write("localized.pdf", localized, manifold.PDF, manifold.WriteOptions{PDFFonts: []manifold.FontFace{{Family: "Noto Sans", Data: font}}})
	write("localized.docx", localized, manifold.DOCX, manifold.WriteOptions{})
	for name, spec := range map[string]struct {
		width float64
		text  string
	}{"wrap": {80, "office AV To office AV To ffi ffi"}, "emergency": {28, "ffi"}} {
		doc := &manifold.Document{Layout: manifold.Layout{Width: spec.width, Height: 400, Margin: 10}, Blocks: []manifold.Block{{Kind: manifold.Paragraph, Runs: []manifold.Run{{Text: spec.text, Language: "en-US", FontFamily: "Noto Sans", FontSize: 20}}}}}
		write(name+".sf", doc, manifold.SF, manifold.WriteOptions{})
		write(name+".pdf", doc, manifold.PDF, manifold.WriteOptions{PDFFonts: []manifold.FontFace{{Family: "Noto Sans", Data: font}}})
	}

}
