// Named navigation points through SF, DOCX, native PDF and PDF view recovery.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"os"
	"path/filepath"

	"github.com/artpar/manifold"
)

func main() {
	const out = "output/destinations"
	if err := os.MkdirAll(out, 0755); err != nil {
		log.Fatal(err)
	}
	p := func(id, text, target string) manifold.Block {
		return manifold.Block{Kind: manifold.Paragraph, Anchors: []string{id}, Runs: []manifold.Run{{Text: text, Destination: target}}}
	}
	d := &manifold.Document{Metadata: manifold.Metadata{Title: "Named navigation"}, Layout: manifold.Layout{Width: 400, Height: 400, Margin: 40},
		UnresolvedDestinations: []string{"missing"},
		Headers:                []manifold.Block{p("header", "HEADER", "table")},
		Footers:                []manifold.Block{p("footer", "FOOTER", "start")},
		Notes:                  []manifold.Note{{Kind: "footnote", ID: "n", Blocks: []manifold.Block{p("note", "NOTE", "end")}}},
		Blocks: []manifold.Block{
			p("start", "FORWARD", "end"),
			{Kind: manifold.Paragraph, Runs: []manifold.Run{{Text: "of"}, {Anchor: "ligature"}, {Text: "fice"}, {Text: " URI", Link: "#literal"}, {Text: " COLLISION", Link: "urn:manydoc:internal:0:0"}, {Text: " MISSING", Destination: "missing"}}},
			{Kind: manifold.Table, Anchors: []string{"table"}, Rows: []manifold.Row{{Cells: []manifold.Cell{
				{Blocks: []manifold.Block{p("cell", "TABLE", "note")}},
				{Blocks: []manifold.Block{{Kind: manifold.AnchorPoint, Anchors: []string{"empty-cell", "empty-alias"}}}},
			}}}},
			{Kind: manifold.AnchorPoint, Anchors: []string{"between", "between-alias"}},
			{Kind: manifold.ListBlock, List: "bullet", Anchors: []string{"list"}, Items: []manifold.ListItem{{Blocks: []manifold.Block{p("item", "LIST", "cell")}}}},
			{Kind: manifold.Paragraph, Runs: []manifold.Run{{Text: "NOTE REFERENCE "}, {Note: &manifold.NoteReference{Kind: "footnote", ID: "n"}}}},
			{Kind: manifold.PageBreak}, p("end", "BACKWARD", "start"),
			{Kind: manifold.Paragraph, Runs: []manifold.Run{{Anchor: "章 space–%"}, {Text: "UNICODE NAME"}}},
			{Kind: manifold.AnchorPoint, Anchors: []string{"tail", "tail-alias"}},
		}}

	ctx := context.Background()
	write := func(name string, d *manifold.Document, format manifold.Format) []byte {
		var b bytes.Buffer
		if err := manifold.Write(ctx, &b, d, format, manifold.WriteOptions{}); err != nil {
			log.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(out, name), b.Bytes(), 0644); err != nil {
			log.Fatal(err)
		}
		return b.Bytes()
	}
	raw := write("source.sf", d, manifold.SF)
	sf, err := manifold.ReadStructured(ctx, bytes.NewReader(raw))
	if err != nil {
		log.Fatal(err)
	}
	d, err = manifold.FromStructured(sf)
	if err != nil {
		log.Fatal(err)
	}
	word := write("destinations.docx", d, manifold.DOCX)
	pdf := write("destinations.pdf", d, manifold.PDF)
	// Same visible document and link styling without navigation points, for an
	// independent byte comparison of page paint commands.
	cloned, _ := json.Marshal(d)
	var baseline manifold.Document
	if err := json.Unmarshal(cloned, &baseline); err != nil {
		log.Fatal(err)
	}
	var strip func([]manifold.Block) []manifold.Block
	strip = func(bs []manifold.Block) []manifold.Block {
		var result []manifold.Block
		for _, b := range bs {
			if b.Kind == manifold.AnchorPoint {
				continue
			}
			b.Anchors = nil
			for i := range b.Runs {
				if b.Runs[i].Anchor != "" {
					b.Runs[i] = manifold.Run{}
				}
				if b.Runs[i].Destination != "" {
					b.Runs[i].Link = "#" + b.Runs[i].Destination
					b.Runs[i].Destination = ""
				}
			}
			for i := range b.Items {
				b.Items[i].Blocks = strip(b.Items[i].Blocks)
			}
			for i := range b.Rows {
				for j := range b.Rows[i].Cells {
					b.Rows[i].Cells[j].Blocks = strip(b.Rows[i].Cells[j].Blocks)
				}
			}
			result = append(result, b)
		}
		return result
	}
	baseline.Blocks = strip(baseline.Blocks)
	baseline.Headers = strip(baseline.Headers)
	baseline.Footers = strip(baseline.Footers)
	for i := range baseline.Notes {
		baseline.Notes[i].Blocks = strip(baseline.Notes[i].Blocks)
	}
	baseline.UnresolvedDestinations = nil
	write("without-points.pdf", &baseline, manifold.PDF)

	back, err := manifold.ReadDOCX(ctx, bytes.NewReader(word))
	if err != nil {
		log.Fatal(err)
	}
	write("from-docx.sf", back, manifold.SF)
	back, err = manifold.ReadPDF(ctx, bytes.NewReader(pdf), manifold.ReadOptions{PDFSkipInference: true})
	if err != nil {
		log.Fatal(err)
	}
	write("from-pdf.sf", back, manifold.SF)
}
