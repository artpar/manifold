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

	"github.com/artpar/manydoc"
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
	format, err := manydoc.FormatFromName(*input)
	if err != nil {
		log.Fatal(err)
	}
	doc, err := manydoc.Read(ctx, bytes.NewReader(raw), format, manydoc.ReadOptions{})
	if err != nil {
		log.Fatal(err)
	}
	var native bytes.Buffer
	if err = manydoc.WriteVDOCS(ctx, &native, doc); err != nil {
		log.Fatal(err)
	}
	reopened, err := manydoc.ReadVDOCS(ctx, bytes.NewReader(native.Bytes()))
	if err != nil {
		log.Fatal(err)
	}
	before, err := manydoc.ToStructured(doc)
	if err != nil {
		log.Fatal(err)
	}
	after, err := manydoc.ToStructured(reopened)
	if err != nil {
		log.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		log.Fatal("native round trip changed the SF graph")
	}
	outputs := map[string][]byte{"document.vdocs": bytes.Clone(native.Bytes())}
	for _, format := range []manydoc.Format{manydoc.SF, manydoc.DOCX, manydoc.PDF} {
		var buf bytes.Buffer
		if err = manydoc.Write(ctx, &buf, reopened, format, manydoc.WriteOptions{AllowLossy: *lossy}); err != nil {
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
	fmt.Printf("Wrote native/SF/DOCX/PDF to %s; exact native/SF graph verified.\nNative MIME: %s\n", *output, manydoc.VDOCSMIME)
}
