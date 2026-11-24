// Run with: GOWORK=off go run ./examples/notes -out output/notes
package main

import (
	"bytes"
	"context"
	_ "embed"
	"flag"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/artpar/manydoc"
)

//go:embed document.sf
var source []byte

func main() {
	out := flag.String("out", "output/notes", "output directory")
	longNote := flag.Bool("long-note", false, "expand the first footnote to exercise continuation pages")
	pageHeight := flag.Float64("page-height", 0, "override page height in points to exercise pagination")
	customLabels := flag.Bool("custom-labels", false, "use literal foot/endnote marks without advancing automatic numbering")
	variantLabels := flag.Bool("variant-labels", false, "use independent citation/body marks; acknowledge static Word numbering")
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
	if *customLabels {
		d.Notes[0].Label = "§E"
		d.Notes[1].Label = "*α"
	}
	if *variantLabels {
		d.Notes[0].Label = "E"
		end := "§E"
		d.Notes[0].BodyLabel = &end
		d.Notes[1].Label = "*α"
		body := "N-1"
		d.Notes[1].BodyLabel = &body
		seen := 0
		for bi := range d.Blocks {
			for ri := range d.Blocks[bi].Runs {
				run := &d.Blocks[bi].Runs[ri]
				if run.Note != nil && run.Note.Kind == manydoc.Footnote && run.Note.ID == d.Notes[1].ID {
					seen++
					if seen > 1 {
						label := "see α"
						run.Note.Label = &label
					}
				}
			}
		}
	}
	if *longNote {
		for i := range d.Notes {
			if d.Notes[i].Kind == manydoc.Footnote {
				d.Notes[i].Blocks = []manydoc.Block{{Kind: manydoc.Paragraph, Runs: []manydoc.Run{{Text: strings.Repeat("Continued citation text. ", 50) + "LAST_NOTE_MARKER"}}}}
				break
			}
		}
	}
	if *pageHeight > 0 {
		d.Layout.Height = *pageHeight
	}
	if err := os.MkdirAll(*out, 0755); err != nil {
		log.Fatal(err)
	}
	for _, format := range []manydoc.Format{manydoc.SF, manydoc.DOCX, manydoc.PDF} {
		name := filepath.Join(*out, "notes."+string(format))
		var buf bytes.Buffer
		if err := manydoc.Write(ctx, &buf, d, format, manydoc.WriteOptions{AllowLossy: *variantLabels}); err != nil {
			log.Fatal(err)
		}
		if err := os.WriteFile(name, buf.Bytes(), 0644); err != nil {
			log.Fatal(err)
		}
		log.Println(name)
	}
}
