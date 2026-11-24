// Preferred grid widths, merged cells, nested layout and continued headers.
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
	const out = "output/table-widths"
	if err := os.MkdirAll(out, 0755); err != nil {
		log.Fatal(err)
	}
	p := func(s string) manydoc.Block {
		return manydoc.Block{Kind: manydoc.Paragraph, Runs: []manydoc.Run{{Text: s}}}
	}
	c := func(s string) manydoc.Cell { return manydoc.Cell{Blocks: []manydoc.Block{p(s)}} }
	table := manydoc.Block{Kind: manydoc.Table, ColumnWidths: []float64{45, 180, 75}, Rows: []manydoc.Row{
		{RepeatHeader: true, Cells: []manydoc.Cell{{ColSpan: 2, Blocks: []manydoc.Block{p("COMBINED HEADER")}}, c("VALUE")}},
		{Cells: []manydoc.Cell{{RowSpan: 2, Blocks: []manydoc.Block{p("MERGED")}}, c("Session Expiry Interval (0)"), c("FIRST")}},
		{Cells: []manydoc.Cell{c("Description stays readable"), c("SECOND")}},
	}}
	for i := 0; i < 22; i++ {
		table.Rows = append(table.Rows, manydoc.Row{Cells: []manydoc.Cell{c("ITEM"), c("A wide description column retains its useful reading width."), c("DATA")}})
	}
	nested := manydoc.Block{Kind: manydoc.Table, ColumnWidths: []float64{90, 210}, Rows: []manydoc.Row{{Cells: []manydoc.Cell{c("INNER"), c("FITTED INNER")}}}}
	table.Rows = append(table.Rows, manydoc.Row{Cells: []manydoc.Cell{c("NEST"), {ColSpan: 2, Blocks: []manydoc.Block{nested}}}})
	d := &manydoc.Document{Layout: manydoc.Layout{Width: 400, Height: 400, Margin: 40}, Blocks: []manydoc.Block{p("PREFERRED COLUMN WIDTHS"), table}}
	ctx := context.Background()
	write := func(name string, d *manydoc.Document, f manydoc.Format) []byte {
		var b bytes.Buffer
		if err := manydoc.Write(ctx, &b, d, f, manydoc.WriteOptions{AllowLossy: true}); err != nil {
			log.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(out, name), b.Bytes(), 0644); err != nil {
			log.Fatal(err)
		}
		return b.Bytes()
	}
	write("source.sf", d, manydoc.SF)
	word := write("widths.docx", d, manydoc.DOCX)
	pdf := write("widths.pdf", d, manydoc.PDF)
	for _, item := range []struct {
		name   string
		data   []byte
		format manydoc.Format
	}{{"from-docx.sf", word, manydoc.DOCX}, {"from-pdf.sf", pdf, manydoc.PDF}} {
		back, err := manydoc.Read(ctx, bytes.NewReader(item.data), item.format, manydoc.ReadOptions{})
		if err != nil {
			log.Fatal(err)
		}
		write(item.name, back, manydoc.SF)
	}
}
