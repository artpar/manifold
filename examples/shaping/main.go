// Default text shaping needs no language tag or external runtime font service.
package main

import (
	"bytes"
	"context"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/artpar/manydoc"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/goregular"
)

func main() {
	out := "output/default-shaping"
	if err := os.MkdirAll(out, 0755); err != nil {
		log.Fatal(err)
	}
	noto, err := os.ReadFile("testdata/assets/NotoSans-Regular.ttf")
	if err != nil {
		log.Fatal(err)
	}
	for name, data := range map[string][]byte{"Go-Regular.ttf": goregular.TTF, "Go-Mono.ttf": gomono.TTF} {
		if err := os.WriteFile(filepath.Join(out, name), data, 0644); err != nil {
			log.Fatal(err)
		}
	}
	p := func(text, family string, size float64) manydoc.Block {
		return manydoc.Block{Kind: manydoc.Paragraph, Runs: []manydoc.Run{{Text: text, FontFamily: family, FontSize: size}}}
	}
	linked := p("office AV To", "Noto Sans", 22)
	linked.Runs[0].Link = "https://example.invalid/shaping"
	linked.Runs[0].Underline = true
	linked.Runs[0].Color = "#225588"
	linked.Runs = append(linked.Runs, manydoc.Run{Note: &manydoc.NoteReference{Kind: manydoc.Footnote, ID: "n"}, FontFamily: "Noto Sans", FontSize: 22})
	doc := &manydoc.Document{Metadata: manydoc.Metadata{Title: "Default contextual shaping"}, Layout: manydoc.Layout{Width: 420, Height: 320, Margin: 30}, Headers: []manydoc.Block{p("DEFAULT HEADER", "", 8)}, Footers: []manydoc.Block{p("DEFAULT FOOTER", "", 8)}, Notes: []manydoc.Note{{ID: "n", Kind: manydoc.Footnote, Blocks: []manydoc.Block{p("Default note office", "", 10)}}}, Blocks: []manydoc.Block{linked, p("Default AV To office", "", 19), p("Latin ΩЖ", "", 14), {Kind: manydoc.Table, Rows: []manydoc.Row{{Cells: []manydoc.Cell{{Blocks: []manydoc.Block{p("office AV", "Noto Sans", 16)}}, {Blocks: []manydoc.Block{p("Mono office", "Go Mono", 14)}}}}, {Cells: []manydoc.Cell{{Blocks: []manydoc.Block{p("ffi café", "Noto Sans", 16)}}, {Blocks: []manydoc.Block{p("TEXT", "Go Mono", 14)}}}}}}, {Kind: manydoc.PageBreak}, p(strings.Repeat("office AV To ", 7), "Noto Sans", 22)}}
	opts := manydoc.WriteOptions{PDFFonts: []manydoc.FontFace{{Family: "Noto Sans", Data: noto}}}
	write := func(name string, d *manydoc.Document, f manydoc.Format) {
		var b bytes.Buffer
		if err := manydoc.Write(context.Background(), &b, d, f, opts); err != nil {
			log.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(out, name), b.Bytes(), 0644); err != nil {
			log.Fatal(err)
		}
	}
	for _, f := range []manydoc.Format{manydoc.SF, manydoc.DOCX, manydoc.PDF} {
		write("default."+string(f), doc, f)
	}
	emergency := &manydoc.Document{Layout: manydoc.Layout{Width: 28, Height: 180, Margin: 10}, Blocks: []manydoc.Block{p("ffi", "Noto Sans", 20)}}
	marked := &manydoc.Document{Layout: manydoc.Layout{Width: 240, Height: 140, Margin: 20}, Blocks: []manydoc.Block{p("A\u2007\u0315B", "Noto Sans", 20)}}
	write("marked-space.sf", marked, manydoc.SF)
	write("marked-space.pdf", marked, manydoc.PDF)
	write("emergency.sf", emergency, manydoc.SF)
	write("emergency.pdf", emergency, manydoc.PDF)
}
