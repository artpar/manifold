// Bidi layout uses paragraph context, directional shaping and logical text.
package main

import (
	"bytes"
	"context"
	"github.com/artpar/manifold"
	"log"
	"os"
	"path/filepath"
)

func main() {
	out := "output/bidi"
	if err := os.MkdirAll(out, 0755); err != nil {
		log.Fatal(err)
	}
	font, err := os.ReadFile("testdata/assets/DejaVuSans.ttf")
	if err != nil {
		log.Fatal(err)
	}
	run := func(s string) manifold.Run { return manifold.Run{Text: s, FontFamily: "DejaVu", FontSize: 18} }
	para := func(dir, text string) manifold.Block {
		return manifold.Block{Kind: manifold.Paragraph, Direction: dir, Runs: []manifold.Run{run(text)}}
	}
	colored := para("rtl", "")
	red, blue := run("ل"), run("ا")
	red.Color = "#CC0000"
	red.Underline = true
	blue.Color = "#0000CC"
	blue.Link = "https://example.invalid/rtl"
	colored.Runs = []manifold.Run{red, blue}
	space := run(" ")
	space.Direction = "rtl"
	override := para("rtl", "")
	override.Runs = []manifold.Run{run("first"), space, run("second")}
	anchor := para("rtl", "שלום הערה")
	anchor.Runs = append(anchor.Runs, manifold.Run{Note: &manifold.NoteReference{Kind: manifold.Footnote, ID: "rtl"}, FontFamily: "DejaVu", FontSize: 18})
	running := para("rtl", "שלום")
	running.Runs[0].FontSize = 10
	doc := &manifold.Document{Layout: manifold.Layout{Width: 360, Height: 520, Margin: 45}, Headers: []manifold.Block{running}, Footers: []manifold.Block{running}, Blocks: []manifold.Block{
		para("rtl", "שלום עולם (123) test"), para("rtl", "سلام بالعالم ١٢٣"), para("ltr", "ABC שלום DEF 123"), override, colored, para("ltr", "A\u2067אבג (12)\u2069 Z"), para("rtl", "שלום עולם שלום עולם שלום עולם שלום עולם שלום עולם"), anchor,
		{Kind: manifold.ListBlock, List: "number", Items: []manifold.ListItem{{Blocks: []manifold.Block{para("rtl", "שלום רשימה")}}}},
		{Kind: manifold.Table, Rows: []manifold.Row{{Cells: []manifold.Cell{{Blocks: []manifold.Block{para("rtl", "אבְג سلام")}}}}}},
	}, Notes: []manifold.Note{{Kind: manifold.Footnote, ID: "rtl", Blocks: []manifold.Block{para("rtl", "سلام הערה")}}}}
	opts := manifold.WriteOptions{PDFFonts: []manifold.FontFace{{Family: "DejaVu", Data: font}}}
	// Context is preserved even when shaping items use different faces/sizes/
	// languages or are separated by a transparent zero-advance control.
	boldFont, err := os.ReadFile("testdata/assets/DejaVuSans-Bold.ttf")
	if err != nil {
		log.Fatal(err)
	}
	a, b := run("ب"), run("ب")
	a.Language, b.Language = "ar", "ar"
	b.Bold = true
	fontBoundary := manifold.Block{Kind: manifold.Paragraph, Direction: "rtl", Runs: []manifold.Run{a, b}}
	b.Bold = false
	a.FontSize, b.FontSize = 12, 24
	sizeBoundary := manifold.Block{Kind: manifold.Paragraph, Direction: "rtl", Runs: []manifold.Run{a, b}}
	a.FontSize, b.FontSize = 18, 18
	b.Language = "fa"
	languageBoundary := manifold.Block{Kind: manifold.Paragraph, Direction: "rtl", Runs: []manifold.Run{a, b}}
	contextDoc := &manifold.Document{Layout: doc.Layout, Blocks: []manifold.Block{fontBoundary, sizeBoundary, languageBoundary, para("rtl", "ب\u200fب"), para("rtl", "ب\u200bب"), para("rtl", "ب\u2060ب"), para("rtl", "ب\ufeffب")}}
	contextOpts := opts
	contextOpts.PDFFonts = append(append([]manifold.FontFace(nil), opts.PDFFonts...), manifold.FontFace{Family: "DejaVu", Bold: true, Data: boldFont})
	for _, format := range []manifold.Format{manifold.SF, manifold.DOCX, manifold.PDF} {
		var data bytes.Buffer
		if err := manifold.Write(context.Background(), &data, contextDoc, format, contextOpts); err != nil {
			log.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(out, "context."+string(format)), data.Bytes(), 0644); err != nil {
			log.Fatal(err)
		}
	}

	for _, format := range []manifold.Format{manifold.SF, manifold.DOCX, manifold.PDF} {
		var b bytes.Buffer
		if err := manifold.Write(context.Background(), &b, doc, format, opts); err != nil {
			log.Fatal(err)
		}
		if format == manifold.PDF || format == manifold.DOCX {
			recovered, err := manifold.Read(context.Background(), bytes.NewReader(b.Bytes()), format, manifold.ReadOptions{})
			if err != nil {
				log.Fatal(err)
			}
			var sf bytes.Buffer
			if err := manifold.Write(context.Background(), &sf, recovered, manifold.SF, manifold.WriteOptions{}); err != nil {
				log.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(out, string(format)+"-recovered.sf"), sf.Bytes(), 0644); err != nil {
				log.Fatal(err)
			}
		}
		if err := os.WriteFile(filepath.Join(out, "bidi."+string(format)), b.Bytes(), 0644); err != nil {
			log.Fatal(err)
		}
	}
}
