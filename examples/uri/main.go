// Unicode IRI targets retain their URI semantics through the four SF paths.
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
	const out = "output/uri"
	if err := os.MkdirAll(out, 0755); err != nil {
		log.Fatal(err)
	}
	p := func(label, target string) manydoc.Block {
		return manydoc.Block{Kind: manydoc.Paragraph, Runs: []manydoc.Run{{Text: label, Link: target}}}
	}
	d := &manydoc.Document{Metadata: manydoc.Metadata{Title: "Unicode URI targets"}, Layout: manydoc.Layout{Width: 400, Height: 400, Margin: 40},
		Headers: []manydoc.Block{p("HEADER", "https://example.invalid/é?q=雪#章")},
		Footers: []manydoc.Block{p("FOOTER", "mailto:δοκιμή@example.invalid?subject=😀")},
		Notes:   []manydoc.Note{{Kind: "footnote", ID: "n", Blocks: []manydoc.Block{p("NOTE", "#_CONNECT_–_Connection")}}},
		Blocks: []manydoc.Block{
			p("RFC3987", "http://www.example.org/red%09rosé#red"),
			{Kind: manydoc.Table, Rows: []manydoc.Row{{Cells: []manydoc.Cell{
				{Blocks: []manydoc.Block{p("TABLE", "https://例え.テスト/道?q=é#章")}},
				{Blocks: []manydoc.Block{p("FRAGMENT", "#_Figure_4.3_–")}},
			}}}},
			{Kind: manydoc.ListBlock, List: "bullet", Items: []manydoc.ListItem{{Blocks: []manydoc.Block{p("LIST", "https://example.invalid/cafe\u0301")}}}},
			{Kind: manydoc.Paragraph, Runs: []manydoc.Run{{Text: "ANCHOR "}, {Note: &manydoc.NoteReference{Kind: "footnote", ID: "n"}}}},
			{Kind: manydoc.PageBreak}, p("END", "https://example.invalid/😀#é"),
		}}

	ctx := context.Background()
	write := func(name string, d *manydoc.Document, format manydoc.Format) []byte {
		var b bytes.Buffer
		if err := manydoc.Write(ctx, &b, d, format, manydoc.WriteOptions{}); err != nil {
			log.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(out, name), b.Bytes(), 0644); err != nil {
			log.Fatal(err)
		}
		return b.Bytes()
	}
	raw := write("source.sf", d, manydoc.SF)
	sf, err := manydoc.ReadStructured(ctx, bytes.NewReader(raw))
	if err != nil {
		log.Fatal(err)
	}
	d, err = manydoc.FromStructured(sf)
	if err != nil {
		log.Fatal(err)
	}
	word := write("uri.docx", d, manydoc.DOCX)
	pdf := write("uri.pdf", d, manydoc.PDF)
	back, err := manydoc.ReadDOCX(ctx, bytes.NewReader(word))
	if err != nil {
		log.Fatal(err)
	}
	write("from-docx.sf", back, manydoc.SF)
	back, err = manydoc.ReadPDF(ctx, bytes.NewReader(pdf), manydoc.ReadOptions{PDFSkipInference: true})
	if err != nil {
		log.Fatal(err)
	}
	write("from-pdf.sf", back, manydoc.SF)
}
