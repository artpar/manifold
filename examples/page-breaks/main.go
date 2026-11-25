// Run with: GOWORK=off go run ./examples/page-breaks -out output/page-breaks
package main

import (
	"bytes"
	"context"
	_ "embed"
	"flag"
	"log"
	"os"
	"path/filepath"

	"github.com/artpar/manifold"
)

//go:embed document.sf
var source []byte

func main() {
	out := flag.String("out", "output/page-breaks", "output directory")
	empty := flag.Bool("empty-note", false, "demonstrate two breaks in an otherwise empty footnote")
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
	if *empty {
		d = &manifold.Document{Metadata: manifold.Metadata{Title: "Empty note break destinations"}, Layout: manifold.Layout{Width: 200, Height: 60, Margin: 20},
			Blocks: []manifold.Block{{Kind: manifold.Paragraph, Runs: []manifold.Run{{Text: "REF"}, {Note: &manifold.NoteReference{Kind: manifold.Footnote, ID: "empty"}}}}},
			Notes:  []manifold.Note{{Kind: manifold.Footnote, ID: "empty", Blocks: []manifold.Block{{Kind: manifold.PageBreak}, {Kind: manifold.PageBreak}}}}}
	}
	if err = os.MkdirAll(*out, 0755); err != nil {
		log.Fatal(err)
	}
	for _, format := range []manifold.Format{manifold.SF, manifold.DOCX, manifold.PDF} {
		var buf bytes.Buffer
		if err = manifold.Write(ctx, &buf, d, format, manifold.WriteOptions{}); err != nil {
			log.Fatal(err)
		}
		path := filepath.Join(*out, "page-breaks."+string(format))
		if err = os.WriteFile(path, buf.Bytes(), 0644); err != nil {
			log.Fatal(err)
		}
		log.Println(path)
	}
}
