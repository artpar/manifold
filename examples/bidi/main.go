// Bidi layout uses paragraph context, directional shaping and logical text.
package main

import (
	"bytes"
	"context"
	"github.com/artpar/manydoc"
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
	run := func(s string) manydoc.Run { return manydoc.Run{Text: s, FontFamily: "DejaVu", FontSize: 18} }
	para := func(dir, text string) manydoc.Block {
		return manydoc.Block{Kind: manydoc.Paragraph, Direction: dir, Runs: []manydoc.Run{run(text)}}
	}
	colored := para("rtl", "")
	red, blue := run("ل"), run("ا")
	red.Color = "#CC0000"
	red.Underline = true
	blue.Color = "#0000CC"
	blue.Link = "https://example.invalid/rtl"
	colored.Runs = []manydoc.Run{red, blue}
	space := run(" ")
	space.Direction = "rtl"
	override := para("rtl", "")
	override.Runs = []manydoc.Run{run("first"), space, run("second")}
	anchor := para("rtl", "שלום הערה")
	anchor.Runs = append(anchor.Runs, manydoc.Run{Note: &manydoc.NoteReference{Kind: manydoc.Footnote, ID: "rtl"}, FontFamily: "DejaVu", FontSize: 18})
	running := para("rtl", "שלום")
	running.Runs[0].FontSize = 10
	doc := &manydoc.Document{Layout: manydoc.Layout{Width: 360, Height: 520, Margin: 45}, Headers: []manydoc.Block{running}, Footers: []manydoc.Block{running}, Blocks: []manydoc.Block{
		para("rtl", "שלום עולם (123) test"), para("rtl", "سلام بالعالم ١٢٣"), para("ltr", "ABC שלום DEF 123"), override, colored, para("ltr", "A\u2067אבג (12)\u2069 Z"), para("rtl", "שלום עולם שלום עולם שלום עולם שלום עולם שלום עולם"), anchor,
		{Kind: manydoc.ListBlock, List: "number", Items: []manydoc.ListItem{{Blocks: []manydoc.Block{para("rtl", "שלום רשימה")}}}},
		{Kind: manydoc.Table, Rows: []manydoc.Row{{Cells: []manydoc.Cell{{Blocks: []manydoc.Block{para("rtl", "אבְג سلام")}}}}}},
	}, Notes: []manydoc.Note{{Kind: manydoc.Footnote, ID: "rtl", Blocks: []manydoc.Block{para("rtl", "سلام הערה")}}}}
	opts := manydoc.WriteOptions{PDFFonts: []manydoc.FontFace{{Family: "DejaVu", Data: font}}}
	// Context is preserved even when shaping items use different faces/sizes/
	// languages or are separated by a transparent zero-advance control.
	boldFont, err := os.ReadFile("testdata/assets/DejaVuSans-Bold.ttf")
	if err != nil {
		log.Fatal(err)
	}
	a, b := run("ب"), run("ب")
	a.Language, b.Language = "ar", "ar"
	b.Bold = true
	fontBoundary := manydoc.Block{Kind: manydoc.Paragraph, Direction: "rtl", Runs: []manydoc.Run{a, b}}
	b.Bold = false
	a.FontSize, b.FontSize = 12, 24
	sizeBoundary := manydoc.Block{Kind: manydoc.Paragraph, Direction: "rtl", Runs: []manydoc.Run{a, b}}
	a.FontSize, b.FontSize = 18, 18
	b.Language = "fa"
	languageBoundary := manydoc.Block{Kind: manydoc.Paragraph, Direction: "rtl", Runs: []manydoc.Run{a, b}}
	contextDoc := &manydoc.Document{Layout: doc.Layout, Blocks: []manydoc.Block{fontBoundary, sizeBoundary, languageBoundary, para("rtl", "ب\u200fب"), para("rtl", "ب\u200bب"), para("rtl", "ب\u2060ب"), para("rtl", "ب\ufeffب")}}
	contextOpts := opts
	contextOpts.PDFFonts = append(append([]manydoc.FontFace(nil), opts.PDFFonts...), manydoc.FontFace{Family: "DejaVu", Bold: true, Data: boldFont})
	for _, format := range []manydoc.Format{manydoc.SF, manydoc.DOCX, manydoc.PDF} {
		var data bytes.Buffer
		if err := manydoc.Write(context.Background(), &data, contextDoc, format, contextOpts); err != nil {
			log.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(out, "context."+string(format)), data.Bytes(), 0644); err != nil {
			log.Fatal(err)
		}
	}

	for _, format := range []manydoc.Format{manydoc.SF, manydoc.DOCX, manydoc.PDF} {
		var b bytes.Buffer
		if err := manydoc.Write(context.Background(), &b, doc, format, opts); err != nil {
			log.Fatal(err)
		}
		if format == manydoc.PDF || format == manydoc.DOCX {
			recovered, err := manydoc.Read(context.Background(), bytes.NewReader(b.Bytes()), format, manydoc.ReadOptions{})
			if err != nil {
				log.Fatal(err)
			}
			var sf bytes.Buffer
			if err := manydoc.Write(context.Background(), &sf, recovered, manydoc.SF, manydoc.WriteOptions{}); err != nil {
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
