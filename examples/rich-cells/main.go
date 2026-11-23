// Run with: GOWORK=off go run ./examples/rich-cells -out output/rich-cells
package main

import (
	"bytes"
	"context"
	_ "embed"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/artpar/manydoc"
)

//go:embed document.sf
var source []byte

func main() {
	out := flag.String("out", "output/rich-cells", "output directory")
	continued := flag.Bool("continued", false, "demonstrate a rich footnote continuing across small pages")
	flag.Parse()
	ctx := context.Background()
	sf, err := manydoc.ReadStructured(ctx, bytes.NewReader(source))
	if err != nil {
		log.Fatal(err)
	}
	d, err := manydoc.FromStructured(sf)
	if err != nil {
		log.Fatal(err)
	}
	if *continued {
		im := *d.Notes[1].Blocks[1].Image
		im.Width, im.Height = 40, 20
		rows := []manydoc.Row{}
		for i := 0; i < 12; i++ {
			rows = append(rows, manydoc.Row{Cells: []manydoc.Cell{{Blocks: []manydoc.Block{{Kind: manydoc.Paragraph, Runs: []manydoc.Run{{Text: fmt.Sprintf("NOTE_ROW_%02d", i)}}}}}}})
		}
		d.Layout = manydoc.Layout{Width: 300, Height: 200, Margin: 20}
		d.Blocks = []manydoc.Block{{Kind: manydoc.Paragraph, Runs: []manydoc.Run{{Text: "REFERENCE"}, {Note: &manydoc.NoteReference{Kind: manydoc.Footnote, ID: "rich"}}}}, {Kind: manydoc.Paragraph, Runs: []manydoc.Run{{Text: "BODY_AFTER"}}}}
		d.Notes = []manydoc.Note{{Kind: manydoc.Footnote, ID: "rich", Blocks: []manydoc.Block{{Kind: manydoc.Paragraph, Runs: []manydoc.Run{{Text: "NOTE_INTRO"}}}, {Kind: manydoc.Picture, Image: &im}, {Kind: manydoc.Table, Rows: rows}}}}
	}
	if err = os.MkdirAll(*out, 0755); err != nil {
		log.Fatal(err)
	}
	for _, format := range []manydoc.Format{manydoc.SF, manydoc.DOCX, manydoc.PDF} {
		var buf bytes.Buffer
		if err = manydoc.Write(ctx, &buf, d, format, manydoc.WriteOptions{}); err != nil {
			log.Fatal(err)
		}
		path := filepath.Join(*out, "rich-cells."+string(format))
		if err = os.WriteFile(path, buf.Bytes(), 0644); err != nil {
			log.Fatal(err)
		}
		log.Println(path)
	}
}
