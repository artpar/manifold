// Run from the repository root:
// GOWORK=off go run ./examples/colleaf -out output/colleaf-example
package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"reflect"

	"github.com/artpar/manifold"
)

func main() {
	input := flag.String("in", "examples/notes/document.sf", "input SF or other supported document")
	output := flag.String("out", "output/colleaf-example", "output directory")
	lossy := flag.Bool("allow-lossy", false, "acknowledge imported diagnostics for DOCX/PDF reflow")
	flag.Parse()
	ctx := context.Background()
	raw, err := os.ReadFile(*input)
	if err != nil {
		log.Fatal(err)
	}
	format, err := manifold.FormatFromName(*input)
	if err != nil {
		log.Fatal(err)
	}
	doc, err := manifold.Read(ctx, bytes.NewReader(raw), format, manifold.ReadOptions{})
	if err != nil {
		log.Fatal(err)
	}
	var native bytes.Buffer
	if err = manifold.WriteVDOCS(ctx, &native, doc); err != nil {
		log.Fatal(err)
	}
	reopened, err := manifold.ReadVDOCS(ctx, bytes.NewReader(native.Bytes()))
	if err != nil {
		log.Fatal(err)
	}
	before, err := manifold.ToStructured(doc)
	if err != nil {
		log.Fatal(err)
	}
	after, err := manifold.ToStructured(reopened)
	if err != nil {
		log.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		log.Fatal("native round trip changed the SF graph")
	}
	outputs := map[string][]byte{"document.vdocs": bytes.Clone(native.Bytes())}
	for _, format := range []manifold.Format{manifold.SF, manifold.DOCX, manifold.PDF} {
		var buf bytes.Buffer
		if err = manifold.Write(ctx, &buf, reopened, format, manifold.WriteOptions{AllowLossy: *lossy}); err != nil {
			log.Fatal(err)
		}
		outputs["document."+string(format)] = bytes.Clone(buf.Bytes())
	}
	if err = os.MkdirAll(*output, 0755); err != nil {
		log.Fatal(err)
	}
	for name, data := range outputs {
		if err = os.WriteFile(filepath.Join(*output, name), data, 0600); err != nil {
			log.Fatal(err)
		}
	}
	fmt.Printf("Wrote native/SF/DOCX/PDF to %s; exact native/SF graph verified.\nNative MIME: %s\n", *output, manifold.VDOCSMIME)
}
