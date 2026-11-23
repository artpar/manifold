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
	p := func(text, family string, size float64) manydoc.Block {
		return manydoc.Block{Kind: manydoc.Paragraph, Runs: []manydoc.Run{{Text: text, FontFamily: family, FontSize: size}}}
	}
	ref := &manydoc.NoteReference{Kind: "footnote", ID: "glyph-note"}
	marked := p("B\u0315", "Noto Sans", 22)
	marked.Runs[0].Color = "#006633"
	marked.Runs[0].Link = "https://example.invalid/glyph"
	marked.Runs[0].Underline = true
	marked.Runs = append(marked.Runs, manydoc.Run{Text: " note", Note: nil}, manydoc.Run{Note: ref})
	d := &manydoc.Document{Metadata: manydoc.Metadata{Title: "Positioned glyphs", Author: "manydoc"}, Layout: manydoc.Layout{Width: 360, Height: 320, Margin: 40}, Headers: []manydoc.Block{p("HEADER B\u0315", "Noto Sans", 10)}, Footers: []manydoc.Block{p("FOOTER 😀", "DejaVu Sans", 10)}, Notes: []manydoc.Note{{ID: "glyph-note", Kind: "footnote", Blocks: []manydoc.Block{p("NOTE Z\u035c", "Noto Sans", 12)}}}, Blocks: []manydoc.Block{marked, p("SUPPLEMENTARY A😀B", "DejaVu Sans", 20), {Kind: manydoc.PageBreak}, {Kind: manydoc.Table, Rows: []manydoc.Row{{RepeatHeader: true, Cells: []manydoc.Cell{{Blocks: []manydoc.Block{p("TABLE B\u0315", "Noto Sans", 16)}}, {Blocks: []manydoc.Block{p("😀", "DejaVu Sans", 20)}}}}, {Cells: []manydoc.Cell{{Blocks: []manydoc.Block{p("BODY Z\u035c", "Noto Sans", 16)}}, {Blocks: []manydoc.Block{p("END 😀", "DejaVu Sans", 20)}}}}}}}}
	opts := manydoc.WriteOptions{PDFFonts: []manydoc.FontFace{{Family: "Noto Sans", Data: noto}, {Family: "DejaVu Sans", Data: dejavu}}}
	for _, format := range []manydoc.Format{manydoc.SF, manydoc.DOCX, manydoc.PDF} {
		var b bytes.Buffer
		if err = manydoc.Write(context.Background(), &b, d, format, opts); err != nil {
			log.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(out, "glyphs."+string(format)), b.Bytes(), 0644); err != nil {
			log.Fatal(err)
		}
	}
}
