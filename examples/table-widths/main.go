// Preferred grid widths, merged cells, nested layout and continued headers.
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
	const out = "output/table-widths"
	if err := os.MkdirAll(out, 0755); err != nil {
		log.Fatal(err)
	}
	p := func(s string) manifold.Block {
		return manifold.Block{Kind: manifold.Paragraph, Runs: []manifold.Run{{Text: s}}}
	}
	c := func(s string) manifold.Cell { return manifold.Cell{Blocks: []manifold.Block{p(s)}} }
	table := manifold.Block{Kind: manifold.Table, ColumnWidths: []float64{45, 180, 75}, Rows: []manifold.Row{
		{RepeatHeader: true, Cells: []manifold.Cell{{ColSpan: 2, Blocks: []manifold.Block{p("COMBINED HEADER")}}, c("VALUE")}},
		{Cells: []manifold.Cell{{RowSpan: 2, Blocks: []manifold.Block{p("MERGED")}}, c("Session Expiry Interval (0)"), c("FIRST")}},
		{Cells: []manifold.Cell{c("Description stays readable"), c("SECOND")}},
	}}
	for i := 0; i < 22; i++ {
		table.Rows = append(table.Rows, manifold.Row{Cells: []manifold.Cell{c("ITEM"), c("A wide description column retains its useful reading width."), c("DATA")}})
	}
	nested := manifold.Block{Kind: manifold.Table, ColumnWidths: []float64{90, 210}, Rows: []manifold.Row{{Cells: []manifold.Cell{c("INNER"), c("FITTED INNER")}}}}
	table.Rows = append(table.Rows, manifold.Row{Cells: []manifold.Cell{c("NEST"), {ColSpan: 2, Blocks: []manifold.Block{nested}}}})
	d := &manifold.Document{Layout: manifold.Layout{Width: 400, Height: 400, Margin: 40}, Blocks: []manifold.Block{p("PREFERRED COLUMN WIDTHS"), table}}
	ctx := context.Background()
	write := func(name string, d *manifold.Document, f manifold.Format) []byte {
		var b bytes.Buffer
		if err := manifold.Write(ctx, &b, d, f, manifold.WriteOptions{AllowLossy: true}); err != nil {
			log.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(out, name), b.Bytes(), 0644); err != nil {
			log.Fatal(err)
		}
		return b.Bytes()
	}
	write("source.sf", d, manifold.SF)
	word := write("widths.docx", d, manifold.DOCX)
	pdf := write("widths.pdf", d, manifold.PDF)
	for _, item := range []struct {
		name   string
		data   []byte
		format manifold.Format
	}{{"from-docx.sf", word, manifold.DOCX}, {"from-pdf.sf", pdf, manifold.PDF}} {
		back, err := manifold.Read(ctx, bytes.NewReader(item.data), item.format, manifold.ReadOptions{})
		if err != nil {
			log.Fatal(err)
		}
		write(item.name, back, manifold.SF)
	}
}
