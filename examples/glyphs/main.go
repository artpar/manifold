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
	out := "output/glyphs"
	if err := os.MkdirAll(out, 0755); err != nil {
		log.Fatal(err)
	}
	noto, err := os.ReadFile("testdata/assets/NotoSans-Regular.ttf")
	if err != nil {
		log.Fatal(err)
	}
	dejavu, err := os.ReadFile("testdata/assets/DejaVuSans.ttf")
	if err != nil {
		log.Fatal(err)
	}
	p := func(text, family string, size float64) manifold.Block {
		return manifold.Block{Kind: manifold.Paragraph, Runs: []manifold.Run{{Text: text, FontFamily: family, FontSize: size}}}
	}
	ref := &manifold.NoteReference{Kind: "footnote", ID: "glyph-note"}
	marked := p("B\u0315", "Noto Sans", 22)
	marked.Runs[0].Color = "#006633"
	marked.Runs[0].Link = "https://example.invalid/glyph"
	marked.Runs[0].Underline = true
	marked.Runs = append(marked.Runs, manifold.Run{Text: " note", Note: nil}, manifold.Run{Note: ref})
	d := &manifold.Document{Metadata: manifold.Metadata{Title: "Positioned glyphs", Author: "manydoc"}, Layout: manifold.Layout{Width: 360, Height: 320, Margin: 40}, Headers: []manifold.Block{p("HEADER B\u0315", "Noto Sans", 10)}, Footers: []manifold.Block{p("FOOTER 😀", "DejaVu Sans", 10)}, Notes: []manifold.Note{{ID: "glyph-note", Kind: "footnote", Blocks: []manifold.Block{p("NOTE Z\u035c", "Noto Sans", 12)}}}, Blocks: []manifold.Block{marked, p("SUPPLEMENTARY A😀B", "DejaVu Sans", 20), {Kind: manifold.PageBreak}, {Kind: manifold.Table, Rows: []manifold.Row{{RepeatHeader: true, Cells: []manifold.Cell{{Blocks: []manifold.Block{p("TABLE B\u0315", "Noto Sans", 16)}}, {Blocks: []manifold.Block{p("😀", "DejaVu Sans", 20)}}}}, {Cells: []manifold.Cell{{Blocks: []manifold.Block{p("BODY Z\u035c", "Noto Sans", 16)}}, {Blocks: []manifold.Block{p("END 😀", "DejaVu Sans", 20)}}}}}}}}
	opts := manifold.WriteOptions{PDFFonts: []manifold.FontFace{{Family: "Noto Sans", Data: noto}, {Family: "DejaVu Sans", Data: dejavu}}}
	for _, format := range []manifold.Format{manifold.SF, manifold.DOCX, manifold.PDF} {
		var b bytes.Buffer
		if err = manifold.Write(context.Background(), &b, d, format, opts); err != nil {
			log.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(out, "glyphs."+string(format)), b.Bytes(), 0644); err != nil {
			log.Fatal(err)
		}
	}
}
