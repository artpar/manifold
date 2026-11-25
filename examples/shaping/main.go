// Default text shaping needs no language tag or external runtime font service.
package main

import (
	"bytes"
	"context"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/artpar/manifold"
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
	p := func(text, family string, size float64) manifold.Block {
		return manifold.Block{Kind: manifold.Paragraph, Runs: []manifold.Run{{Text: text, FontFamily: family, FontSize: size}}}
	}
	linked := p("office AV To", "Noto Sans", 22)
	linked.Runs[0].Link = "https://example.invalid/shaping"
	linked.Runs[0].Underline = true
	linked.Runs[0].Color = "#225588"
	linked.Runs = append(linked.Runs, manifold.Run{Note: &manifold.NoteReference{Kind: manifold.Footnote, ID: "n"}, FontFamily: "Noto Sans", FontSize: 22})
	doc := &manifold.Document{Metadata: manifold.Metadata{Title: "Default contextual shaping"}, Layout: manifold.Layout{Width: 420, Height: 320, Margin: 30}, Headers: []manifold.Block{p("DEFAULT HEADER", "", 8)}, Footers: []manifold.Block{p("DEFAULT FOOTER", "", 8)}, Notes: []manifold.Note{{ID: "n", Kind: manifold.Footnote, Blocks: []manifold.Block{p("Default note office", "", 10)}}}, Blocks: []manifold.Block{linked, p("Default AV To office", "", 19), p("Latin ΩЖ", "", 14), {Kind: manifold.Table, Rows: []manifold.Row{{Cells: []manifold.Cell{{Blocks: []manifold.Block{p("office AV", "Noto Sans", 16)}}, {Blocks: []manifold.Block{p("Mono office", "Go Mono", 14)}}}}, {Cells: []manifold.Cell{{Blocks: []manifold.Block{p("ffi café", "Noto Sans", 16)}}, {Blocks: []manifold.Block{p("TEXT", "Go Mono", 14)}}}}}}, {Kind: manifold.PageBreak}, p(strings.Repeat("office AV To ", 7), "Noto Sans", 22)}}
	opts := manifold.WriteOptions{PDFFonts: []manifold.FontFace{{Family: "Noto Sans", Data: noto}}}
	write := func(name string, d *manifold.Document, f manifold.Format) {
		var b bytes.Buffer
		if err := manifold.Write(context.Background(), &b, d, f, opts); err != nil {
			log.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(out, name), b.Bytes(), 0644); err != nil {
			log.Fatal(err)
		}
	}
	for _, f := range []manifold.Format{manifold.SF, manifold.DOCX, manifold.PDF} {
		write("default."+string(f), doc, f)
	}
	emergency := &manifold.Document{Layout: manifold.Layout{Width: 28, Height: 180, Margin: 10}, Blocks: []manifold.Block{p("ffi", "Noto Sans", 20)}}
	marked := &manifold.Document{Layout: manifold.Layout{Width: 240, Height: 140, Margin: 20}, Blocks: []manifold.Block{p("A\u2007\u0315B", "Noto Sans", 20)}}
	write("marked-space.sf", marked, manifold.SF)
	write("marked-space.pdf", marked, manifold.PDF)
	write("emergency.sf", emergency, manifold.SF)
	write("emergency.pdf", emergency, manifold.PDF)
}
