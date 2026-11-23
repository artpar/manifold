// Run with: GOWORK=off go run ./examples/merged-tables -out output/merged-tables
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
	out := flag.String("out", "output/merged-tables", "output directory")
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
	if err := os.MkdirAll(*out, 0755); err != nil {
		log.Fatal(err)
	}
	for _, format := range []manydoc.Format{manydoc.SF, manydoc.DOCX, manydoc.PDF} {
		name := filepath.Join(*out, "merged-table."+string(format))
		var buf bytes.Buffer
		if err := manydoc.Write(ctx, &buf, d, format, manydoc.WriteOptions{}); err != nil {
			log.Fatal(err)
		}
		if err := os.WriteFile(name, buf.Bytes(), 0644); err != nil {
			log.Fatal(err)
		}
		log.Println(name)
	}
}
