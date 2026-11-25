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
	"strconv"
	"strings"

	"github.com/artpar/manifold"
)

//go:embed document.sf
var source []byte

func main() {
	out := flag.String("out", "output/notes", "output directory")
	longNote := flag.Bool("long-note", false, "expand the first footnote to exercise continuation pages")
	pageHeight := flag.Float64("page-height", 0, "override page height in points to exercise pagination")
	customLabels := flag.Bool("custom-labels", false, "use literal foot/endnote marks without advancing automatic numbering")
	variantLabels := flag.Bool("variant-labels", false, "use independent citation/body marks; acknowledge static Word numbering")
	styledLabels := flag.Bool("styled-labels", false, "preserve independent body mark typography and backlinks; acknowledge static Word numbering")
	fractionalSizes := flag.Bool("fractional-sizes", false, "export SF/DOCX with fractional authoring sizes (native PDF requires 0.01pt)")
	sectioned := flag.Bool("sectioned", false, "wrap content and running stories in an explicit section")
	richBodyLabels := flag.Bool("rich-body-labels", false, "use multiple independently styled/linkable body-label text runs")
	richCitations := flag.Bool("rich-citations", false, "use individually styled literal citation fragments")
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
				if run.Note != nil && run.Note.Kind == manifold.Footnote && run.Note.ID == d.Notes[1].ID {
					seen++
					if seen > 1 {
						label := "see α"
						run.Note.Label = &label
					}
				}
			}
		}
	}
	if *styledLabels {
		d.Blocks[0].Anchors = append(d.Blocks[0].Anchors, "citation-return")
		for i := range d.Notes {
			d.Notes[i].BodyLabelStyle = &manifold.Run{FontFamily: "Go", FontSize: 9.5, Bold: true, Italic: true, Color: "#203040", Underline: true, Destination: "citation-return", Link: "#citation-return", Language: "en"}
		}
	}
	if *richBodyLabels {
		d.Blocks[0].Anchors = append(d.Blocks[0].Anchors, "rich-return")
		for i := range d.Notes {
			d.Notes[i].BodyLabel = nil
			d.Notes[i].BodyLabelStyle = nil
			d.Notes[i].BodyLabelContent = []manifold.Run{{Text: "N", Bold: true, FontFamily: "Go", FontSize: 9, Color: "#203040", Link: "#rich-return", Destination: "rich-return"}, {Text: "-" + strconv.Itoa(i+1), Italic: true, FontFamily: "Go", FontSize: 8, Link: "https://example.org/label"}}
		}
	}
	if *fractionalSizes {
		for i := range d.Blocks {
			for j := range d.Blocks[i].Runs {
				d.Blocks[i].Runs[j].FontSize = 10.24999
			}
		}
		d.Blocks[0].Runs[0].FontSize = 19.24999
		for i := range d.Notes {
			if d.Notes[i].BodyLabelContent != nil {
				for j := range d.Notes[i].BodyLabelContent {
					d.Notes[i].BodyLabelContent[j].FontSize = 6.959646 + float64(j)
				}
			} else {
				if d.Notes[i].BodyLabelStyle == nil {
					d.Notes[i].BodyLabelStyle = &manifold.Run{}
				}
				d.Notes[i].BodyLabelStyle.FontSize = 6.959646
			}
			if len(d.Notes[i].Blocks) > 0 && len(d.Notes[i].Blocks[0].Runs) > 0 {
				d.Notes[i].Blocks[0].Runs[0].FontSize = 9.959491
			}
		}
		for i := range d.Headers {
			for j := range d.Headers[i].Runs {
				d.Headers[i].Runs[j].FontSize = 8.51
			}
		}
		for i := range d.Footers {
			for j := range d.Footers[i].Runs {
				d.Footers[i].Runs[j].FontSize = 8.49
			}
		}
	}
	if *richCitations {
		styleExampleCitations(d.Blocks, map[string]string{})
	}
	if *sectioned {
		d.Blocks = []manifold.Block{{Kind: manifold.SectionBlock, Section: &manifold.Section{Blocks: d.Blocks, Headers: manifold.RunningStories{Default: &manifold.RunningStory{Blocks: d.Headers}}, Footers: manifold.RunningStories{Default: &manifold.RunningStory{Blocks: d.Footers}}}}}
		d.Headers = nil
		d.Footers = nil
	}
	if *longNote {
		for i := range d.Notes {
			if d.Notes[i].Kind == manifold.Footnote {
				d.Notes[i].Blocks = []manifold.Block{{Kind: manifold.Paragraph, Runs: []manifold.Run{{Text: strings.Repeat("Continued citation text. ", 50) + "LAST_NOTE_MARKER"}}}}
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
	formats := []manifold.Format{manifold.SF, manifold.DOCX, manifold.PDF}
	if *fractionalSizes {
		formats = []manifold.Format{manifold.SF, manifold.DOCX}
	}
	for _, format := range formats {
		name := filepath.Join(*out, "notes."+string(format))
		var buf bytes.Buffer
		if err := manifold.Write(ctx, &buf, d, format, manifold.WriteOptions{AllowLossy: *variantLabels || *styledLabels || *fractionalSizes || *richBodyLabels || *richCitations}); err != nil {
			log.Fatal(err)
		}
		if err := os.WriteFile(name, buf.Bytes(), 0644); err != nil {
			log.Fatal(err)
		}
		log.Println(name)
	}
}

func styleExampleCitations(blocks []manifold.Block, labels map[string]string) {
	for i := range blocks {
		b := &blocks[i]
		for j := range b.Runs {
			ref := b.Runs[j].Note
			if ref == nil {
				continue
			}
			key := ref.Kind + ":" + ref.ID
			suffix := labels[key]
			if suffix == "" {
				suffix = strconv.Itoa(len(labels) + 1)
				labels[key] = suffix
			}
			ref.Label = nil
			ref.LabelContent = []manifold.Run{{Text: "C", FontFamily: "Go", FontSize: 9, Bold: true, Color: "#203040"}, {Text: "-" + suffix, FontFamily: "Go", FontSize: 8, Italic: true, Color: "#FF0000"}}
		}
		for j := range b.Items {
			styleExampleCitations(b.Items[j].Blocks, labels)
		}
		for j := range b.Rows {
			for k := range b.Rows[j].Cells {
				styleExampleCitations(b.Rows[j].Cells[k].Blocks, labels)
			}
		}
	}
}
