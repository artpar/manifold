// Unicode IRI targets retain their URI semantics through the four SF paths.
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
	const out = "output/uri"
	if err := os.MkdirAll(out, 0755); err != nil {
		log.Fatal(err)
	}
	p := func(label, target string) manifold.Block {
		return manifold.Block{Kind: manifold.Paragraph, Runs: []manifold.Run{{Text: label, Link: target}}}
	}
	d := &manifold.Document{Metadata: manifold.Metadata{Title: "Unicode URI targets"}, Layout: manifold.Layout{Width: 400, Height: 400, Margin: 40},
		Headers: []manifold.Block{p("HEADER", "https://example.invalid/é?q=雪#章")},
		Footers: []manifold.Block{p("FOOTER", "mailto:δοκιμή@example.invalid?subject=😀")},
		Notes:   []manifold.Note{{Kind: "footnote", ID: "n", Blocks: []manifold.Block{p("NOTE", "#_CONNECT_–_Connection")}}},
		Blocks: []manifold.Block{
			p("RFC3987", "http://www.example.org/red%09rosé#red"),
			{Kind: manifold.Table, Rows: []manifold.Row{{Cells: []manifold.Cell{
				{Blocks: []manifold.Block{p("TABLE", "https://例え.テスト/道?q=é#章")}},
				{Blocks: []manifold.Block{p("FRAGMENT", "#_Figure_4.3_–")}},
			}}}},
			{Kind: manifold.ListBlock, List: "bullet", Items: []manifold.ListItem{{Blocks: []manifold.Block{p("LIST", "https://example.invalid/cafe\u0301")}}}},
			{Kind: manifold.Paragraph, Runs: []manifold.Run{{Text: "ANCHOR "}, {Note: &manifold.NoteReference{Kind: "footnote", ID: "n"}}}},
			{Kind: manifold.PageBreak}, p("END", "https://example.invalid/😀#é"),
		}}

	ctx := context.Background()
	write := func(name string, d *manifold.Document, format manifold.Format) []byte {
		var b bytes.Buffer
		if err := manifold.Write(ctx, &b, d, format, manifold.WriteOptions{}); err != nil {
			log.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(out, name), b.Bytes(), 0644); err != nil {
			log.Fatal(err)
		}
		return b.Bytes()
	}
	raw := write("source.sf", d, manifold.SF)
	sf, err := manifold.ReadStructured(ctx, bytes.NewReader(raw))
	if err != nil {
		log.Fatal(err)
	}
	d, err = manifold.FromStructured(sf)
	if err != nil {
		log.Fatal(err)
	}
	word := write("uri.docx", d, manifold.DOCX)
	pdf := write("uri.pdf", d, manifold.PDF)
	back, err := manifold.ReadDOCX(ctx, bytes.NewReader(word))
	if err != nil {
		log.Fatal(err)
	}
	write("from-docx.sf", back, manifold.SF)
	back, err = manifold.ReadPDF(ctx, bytes.NewReader(pdf), manifold.ReadOptions{PDFSkipInference: true})
	if err != nil {
		log.Fatal(err)
	}
	write("from-pdf.sf", back, manifold.SF)
}
