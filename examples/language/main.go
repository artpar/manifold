// Language bindings and paragraph/run direction persist through SF and DOCX.
// PDF demonstrates qualified LTR localized glyphs; RTL PDF remains unfinished.
package main

import (
	"bytes"
	"context"
	"log"
	"os"
	"path/filepath"

	"github.com/artpar/manydoc"
)

func main() {
	out := "output/language"
	if err := os.MkdirAll(out, 0755); err != nil {
		log.Fatal(err)
	}
	p := manydoc.Block{Kind: manydoc.Paragraph, Direction: "rtl", Runs: []manydoc.Run{{Text: "Paragraph and run directions are independent", ComplexScript: true, Direction: "ltr", Language: "en-GB", EastAsiaLanguage: "ja-JP", ComplexLanguage: "ar-SA", Link: "https://example.invalid/language"}}}
	d := &manydoc.Document{Metadata: manydoc.Metadata{Title: "Language and direction"}, Layout: manydoc.Layout{Width: 595.3, Height: 841.9, Margin: 54}, Blocks: []manydoc.Block{p, {Kind: manydoc.Table, Rows: []manydoc.Row{{Cells: []manydoc.Cell{{Blocks: []manydoc.Block{p}}}}}}, {Kind: manydoc.Paragraph, Direction: "ltr", Runs: []manydoc.Run{{Note: &manydoc.NoteReference{Kind: manydoc.Footnote, ID: "n"}, Language: "fr-CA", Direction: "ltr"}}}}, Headers: []manydoc.Block{p}, Footers: []manydoc.Block{p}, Notes: []manydoc.Note{{Kind: manydoc.Footnote, ID: "n", Blocks: []manydoc.Block{p}}}}
	write := func(name string, doc *manydoc.Document, format manydoc.Format, opts manydoc.WriteOptions) {
		var b bytes.Buffer
		if err := manydoc.Write(context.Background(), &b, doc, format, opts); err != nil {
			log.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(out, name), b.Bytes(), 0644); err != nil {
			log.Fatal(err)
		}
	}
	write("direction.sf", d, manydoc.SF, manydoc.WriteOptions{})
	write("direction.docx", d, manydoc.DOCX, manydoc.WriteOptions{})
	font, err := os.ReadFile("testdata/assets/NotoSans-Regular.ttf")
	if err != nil {
		log.Fatal(err)
	}
	localized := &manydoc.Document{Metadata: manydoc.Metadata{Title: "Localized Cyrillic glyphs"}, Layout: manydoc.Layout{Width: 360, Height: 240, Margin: 30}}
	for _, locale := range []string{"ru-RU", "sr-RS", "mk-MK"} {
		localized.Blocks = append(localized.Blocks, manydoc.Block{Kind: manydoc.Paragraph, Direction: "ltr", Runs: []manydoc.Run{{Text: "бгдпт", Language: locale, FontFamily: "Noto Sans", FontSize: 28}}})
	}
	localized.Blocks = append(localized.Blocks, manydoc.Block{Kind: manydoc.Paragraph, Direction: "ltr", Runs: []manydoc.Run{{Text: "бгдпт", Language: "ru-RU", ComplexLanguage: "sr-RS", ComplexScript: true, FontFamily: "Noto Sans", FontSize: 28}}})
	write("localized.sf", localized, manydoc.SF, manydoc.WriteOptions{})
	write("localized.pdf", localized, manydoc.PDF, manydoc.WriteOptions{PDFFonts: []manydoc.FontFace{{Family: "Noto Sans", Data: font}}})
	write("localized.docx", localized, manydoc.DOCX, manydoc.WriteOptions{})
	for name, spec := range map[string]struct {
		width float64
		text  string
	}{"wrap": {80, "office AV To office AV To ffi ffi"}, "emergency": {28, "ffi"}} {
		doc := &manydoc.Document{Layout: manydoc.Layout{Width: spec.width, Height: 400, Margin: 10}, Blocks: []manydoc.Block{{Kind: manydoc.Paragraph, Runs: []manydoc.Run{{Text: spec.text, Language: "en-US", FontFamily: "Noto Sans", FontSize: 20}}}}}
		write(name+".sf", doc, manydoc.SF, manydoc.WriteOptions{})
		write(name+".pdf", doc, manydoc.PDF, manydoc.WriteOptions{PDFFonts: []manydoc.FontFace{{Family: "Noto Sans", Data: font}}})
	}

}
