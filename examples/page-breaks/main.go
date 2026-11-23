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

	"github.com/artpar/manydoc"
)

//go:embed document.sf
var source []byte

func main() {
	out := flag.String("out", "output/page-breaks", "output directory")
	empty := flag.Bool("empty-note", false, "demonstrate two breaks in an otherwise empty footnote")
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
	if *empty {
		d = &manydoc.Document{Metadata: manydoc.Metadata{Title: "Empty note break destinations"}, Layout: manydoc.Layout{Width: 200, Height: 60, Margin: 20},
			Blocks: []manydoc.Block{{Kind: manydoc.Paragraph, Runs: []manydoc.Run{{Text: "REF"}, {Note: &manydoc.NoteReference{Kind: manydoc.Footnote, ID: "empty"}}}}},
			Notes:  []manydoc.Note{{Kind: manydoc.Footnote, ID: "empty", Blocks: []manydoc.Block{{Kind: manydoc.PageBreak}, {Kind: manydoc.PageBreak}}}}}
	}
	if err = os.MkdirAll(*out, 0755); err != nil {
		log.Fatal(err)
	}
	for _, format := range []manydoc.Format{manydoc.SF, manydoc.DOCX, manydoc.PDF} {
		var buf bytes.Buffer
		if err = manydoc.Write(ctx, &buf, d, format, manydoc.WriteOptions{}); err != nil {
			log.Fatal(err)
		}
		path := filepath.Join(*out, "page-breaks."+string(format))
		if err = os.WriteFile(path, buf.Bytes(), 0644); err != nil {
			log.Fatal(err)
		}
		log.Println(path)
	}
}
