// Run with: GOWORK=off go run ./examples/text-style -out output/text-style
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
	"golang.org/x/image/font/gofont/goregular"
)

//go:embed document.sf
var source []byte

func main() {
	out := flag.String("out", "output/text-style", "output directory")
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
	if err = os.MkdirAll(*out, 0755); err != nil {
		log.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(*out, "app-sans.ttf"), goregular.TTF, 0644); err != nil {
		log.Fatal(err)
	}
	for _, format := range []manydoc.Format{manydoc.SF, manydoc.DOCX, manydoc.PDF} {
		var buf bytes.Buffer
		if err = manydoc.Write(ctx, &buf, d, format, manydoc.WriteOptions{PDFFonts: []manydoc.FontFace{{Family: "App Sans", Data: goregular.TTF}}}); err != nil {
			log.Fatal(err)
		}
		path := filepath.Join(*out, "text-style."+string(format))
		if err = os.WriteFile(path, buf.Bytes(), 0644); err != nil {
			log.Fatal(err)
		}
		log.Println(path)
	}
}
