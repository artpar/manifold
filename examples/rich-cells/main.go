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

	"github.com/artpar/manifold"
)

//go:embed document.sf
var source []byte

func main() {
	out := flag.String("out", "output/rich-cells", "output directory")
	continued := flag.Bool("continued", false, "demonstrate a rich footnote continuing across small pages")
	flag.Parse()
	ctx := context.Background()
	sf, err := manifold.ReadStructured(ctx, bytes.NewReader(source))
	if err != nil {
		log.Fatal(err)
	}
	d, err := manifold.FromStructured(sf)
	if err != nil {
		log.Fatal(err)
	}
	if *continued {
		im := *d.Notes[1].Blocks[1].Image
		im.Width, im.Height = 40, 20
		rows := []manifold.Row{}
		for i := 0; i < 12; i++ {
			rows = append(rows, manifold.Row{Cells: []manifold.Cell{{Blocks: []manifold.Block{{Kind: manifold.Paragraph, Runs: []manifold.Run{{Text: fmt.Sprintf("NOTE_ROW_%02d", i)}}}}}}})
		}
		d.Layout = manifold.Layout{Width: 300, Height: 200, Margin: 20}
		d.Blocks = []manifold.Block{{Kind: manifold.Paragraph, Runs: []manifold.Run{{Text: "REFERENCE"}, {Note: &manifold.NoteReference{Kind: manifold.Footnote, ID: "rich"}}}}, {Kind: manifold.Paragraph, Runs: []manifold.Run{{Text: "BODY_AFTER"}}}}
		d.Notes = []manifold.Note{{Kind: manifold.Footnote, ID: "rich", Blocks: []manifold.Block{{Kind: manifold.Paragraph, Runs: []manifold.Run{{Text: "NOTE_INTRO"}}}, {Kind: manifold.Picture, Image: &im}, {Kind: manifold.Table, Rows: rows}}}}
	}
	if err = os.MkdirAll(*out, 0755); err != nil {
		log.Fatal(err)
	}
	for _, format := range []manifold.Format{manifold.SF, manifold.DOCX, manifold.PDF} {
		var buf bytes.Buffer
		if err = manifold.Write(ctx, &buf, d, format, manifold.WriteOptions{}); err != nil {
			log.Fatal(err)
		}
		path := filepath.Join(*out, "rich-cells."+string(format))
		if err = os.WriteFile(path, buf.Bytes(), 0644); err != nil {
			log.Fatal(err)
		}
		log.Println(path)
	}
}
