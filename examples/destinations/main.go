// Named navigation points through SF, DOCX, native PDF and PDF view recovery.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"os"
	"path/filepath"

	"github.com/artpar/manydoc"
)

func main() {
	const out = "output/destinations"
	if err := os.MkdirAll(out, 0755); err != nil {
		log.Fatal(err)
	}
	p := func(id, text, target string) manydoc.Block {
		return manydoc.Block{Kind: manydoc.Paragraph, Anchors: []string{id}, Runs: []manydoc.Run{{Text: text, Destination: target}}}
	}
	d := &manydoc.Document{Metadata: manydoc.Metadata{Title: "Named navigation"}, Layout: manydoc.Layout{Width: 400, Height: 400, Margin: 40},
		UnresolvedDestinations: []string{"missing"},
		Headers:                []manydoc.Block{p("header", "HEADER", "table")},
		Footers:                []manydoc.Block{p("footer", "FOOTER", "start")},
		Notes:                  []manydoc.Note{{Kind: "footnote", ID: "n", Blocks: []manydoc.Block{p("note", "NOTE", "end")}}},
		Blocks: []manydoc.Block{
			p("start", "FORWARD", "end"),
			{Kind: manydoc.Paragraph, Runs: []manydoc.Run{{Text: "of"}, {Anchor: "ligature"}, {Text: "fice"}, {Text: " URI", Link: "#literal"}, {Text: " COLLISION", Link: "urn:manydoc:internal:0:0"}, {Text: " MISSING", Destination: "missing"}}},
			{Kind: manydoc.Table, Anchors: []string{"table"}, Rows: []manydoc.Row{{Cells: []manydoc.Cell{
				{Blocks: []manydoc.Block{p("cell", "TABLE", "note")}},
				{Blocks: []manydoc.Block{{Kind: manydoc.AnchorPoint, Anchors: []string{"empty-cell", "empty-alias"}}}},
			}}}},
			{Kind: manydoc.AnchorPoint, Anchors: []string{"between", "between-alias"}},
			{Kind: manydoc.ListBlock, List: "bullet", Anchors: []string{"list"}, Items: []manydoc.ListItem{{Blocks: []manydoc.Block{p("item", "LIST", "cell")}}}},
			{Kind: manydoc.Paragraph, Runs: []manydoc.Run{{Text: "NOTE REFERENCE "}, {Note: &manydoc.NoteReference{Kind: "footnote", ID: "n"}}}},
			{Kind: manydoc.PageBreak}, p("end", "BACKWARD", "start"),
			{Kind: manydoc.Paragraph, Runs: []manydoc.Run{{Anchor: "章 space–%"}, {Text: "UNICODE NAME"}}},
			{Kind: manydoc.AnchorPoint, Anchors: []string{"tail", "tail-alias"}},
		}}

	ctx := context.Background()
	write := func(name string, d *manydoc.Document, format manydoc.Format) []byte {
		var b bytes.Buffer
		if err := manydoc.Write(ctx, &b, d, format, manydoc.WriteOptions{}); err != nil {
			log.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(out, name), b.Bytes(), 0644); err != nil {
			log.Fatal(err)
		}
		return b.Bytes()
	}
	raw := write("source.sf", d, manydoc.SF)
	sf, err := manydoc.ReadStructured(ctx, bytes.NewReader(raw))
	if err != nil {
		log.Fatal(err)
	}
	d, err = manydoc.FromStructured(sf)
	if err != nil {
		log.Fatal(err)
	}
	word := write("destinations.docx", d, manydoc.DOCX)
	pdf := write("destinations.pdf", d, manydoc.PDF)
	// Same visible document and link styling without navigation points, for an
	// independent byte comparison of page paint commands.
	cloned, _ := json.Marshal(d)
	var baseline manydoc.Document
	if err := json.Unmarshal(cloned, &baseline); err != nil {
		log.Fatal(err)
	}
	var strip func([]manydoc.Block) []manydoc.Block
	strip = func(bs []manydoc.Block) []manydoc.Block {
		var result []manydoc.Block
		for _, b := range bs {
			if b.Kind == manydoc.AnchorPoint {
				continue
			}
			b.Anchors = nil
			for i := range b.Runs {
				if b.Runs[i].Anchor != "" {
					b.Runs[i] = manydoc.Run{}
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
	write("without-points.pdf", &baseline, manydoc.PDF)

	back, err := manydoc.ReadDOCX(ctx, bytes.NewReader(word))
	if err != nil {
		log.Fatal(err)
	}
	write("from-docx.sf", back, manydoc.SF)
	back, err = manydoc.ReadPDF(ctx, bytes.NewReader(pdf), manydoc.ReadOptions{PDFSkipInference: true})
	if err != nil {
		log.Fatal(err)
	}
	write("from-pdf.sf", back, manydoc.SF)
}
