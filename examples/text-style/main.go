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

	"github.com/artpar/manifold"
	"golang.org/x/image/font/gofont/goregular"
)

//go:embed document.sf
var source []byte

func main() {
	out := flag.String("out", "output/text-style", "output directory")
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
	if err = os.MkdirAll(*out, 0755); err != nil {
		log.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(*out, "app-sans.ttf"), goregular.TTF, 0644); err != nil {
		log.Fatal(err)
	}
	for _, format := range []manifold.Format{manifold.SF, manifold.DOCX, manifold.PDF} {
		var buf bytes.Buffer
		if err = manifold.Write(ctx, &buf, d, format, manifold.WriteOptions{PDFFonts: []manifold.FontFace{{Family: "App Sans", Data: goregular.TTF}}}); err != nil {
			log.Fatal(err)
		}
		path := filepath.Join(*out, "text-style."+string(format))
		if err = os.WriteFile(path, buf.Bytes(), 0644); err != nil {
			log.Fatal(err)
		}
		log.Println(path)
	}
}
