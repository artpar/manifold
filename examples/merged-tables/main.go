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

	"github.com/artpar/manifold"
)

//go:embed document.sf
var source []byte

func main() {
	out := flag.String("out", "output/merged-tables", "output directory")
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
	if err := os.MkdirAll(*out, 0755); err != nil {
		log.Fatal(err)
	}
	for _, format := range []manifold.Format{manifold.SF, manifold.DOCX, manifold.PDF} {
		name := filepath.Join(*out, "merged-table."+string(format))
		var buf bytes.Buffer
		if err := manifold.Write(ctx, &buf, d, format, manifold.WriteOptions{}); err != nil {
			log.Fatal(err)
		}
		if err := os.WriteFile(name, buf.Bytes(), 0644); err != nil {
			log.Fatal(err)
		}
		log.Println(name)
	}
}
