// Extract an original tagged PDF and emit a small provenance audit and SF sample.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"os"
	"path/filepath"

	"github.com/artpar/manydoc"
)

type cell struct {
	Row     int                 `json:"row"`
	ColSpan int                 `json:"colspan"`
	RowSpan int                 `json:"rowspan"`
	Text    string              `json:"text"`
	Source  *manydoc.SourceInfo `json:"source"`
}
type table struct {
	Source *manydoc.SourceInfo `json:"source"`
	Rows   int                 `json:"rows"`
	Cells  []cell              `json:"cells"`
}

func main() {
	src := flag.String("source", "testdata/real/mqtt-v5.0-os.pdf", "original PDF")
	out := flag.String("out", "output/tagged-spans", "QA directory")
	flag.Parse()
	f, err := os.Open(*src)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	d, err := manydoc.ReadPDF(context.Background(), f, manydoc.ReadOptions{PDFSkipInference: true})
	if err != nil {
		log.Fatal(err)
	}
	var tables []table
	sample := &manydoc.Document{Pages: d.Pages, Diagnostics: d.Diagnostics}
	var walk func([]manydoc.Block)
	walk = func(bs []manydoc.Block) {
		for _, b := range bs {
			if b.Kind == manydoc.Table {
				t := table{Source: b.Source, Rows: len(b.Rows)}
				for ri, r := range b.Rows {
					for _, c := range r.Cells {
						t.Cells = append(t.Cells, cell{ri, c.ColSpan, c.RowSpan, (&manydoc.Document{Blocks: c.Blocks}).Text(), c.Source})
					}
				}
				tables = append(tables, t)
				if b.Source != nil && (b.Source.Element == "(4887 0 R)" || b.Source.Element == "(5936 0 R)" || b.Source.Element == "(12539 0 R)") {
					sample.Blocks = append(sample.Blocks, b)
				}
			}
			for _, r := range b.Rows {
				for _, c := range r.Cells {
					walk(c.Blocks)
				}
			}
			for _, item := range b.Items {
				walk(item.Blocks)
			}
		}
	}
	walk(d.Blocks)
	if err := os.MkdirAll(*out, 0755); err != nil {
		log.Fatal(err)
	}
	report := struct {
		Pages       int                  `json:"pages"`
		Tables      []table              `json:"tables"`
		Diagnostics []manydoc.Diagnostic `json:"diagnostics"`
	}{len(d.Pages), tables, d.Diagnostics}
	raw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(*out, "tables.json"), raw, 0644); err != nil {
		log.Fatal(err)
	}
	sf, err := manydoc.ToStructured(sample)
	if err != nil {
		log.Fatal(err)
	}
	fout, err := os.Create(filepath.Join(*out, "sample.sf"))
	if err != nil {
		log.Fatal(err)
	}
	if err := manydoc.WriteStructured(context.Background(), fout, sf); err != nil {
		log.Fatal(err)
	}
	if err := fout.Close(); err != nil {
		log.Fatal(err)
	}
}
