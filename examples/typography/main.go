// Typography exercises proportional Unicode spacing and line breaking in Go.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/artpar/manydoc"
)

func main() {
	out := flag.String("out", "output/typography", "output directory")
	flag.Parse()
	if err := generate(*out); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func generate(out string) error {
	d := &manydoc.Document{Metadata: manydoc.Metadata{Title: "Unicode typography"}, Layout: manydoc.Layout{Width: 360, Height: 620, Margin: 30}, Blocks: []manydoc.Block{{Kind: manydoc.Heading, Level: 2, Runs: []manydoc.Run{{Text: "Unicode typography"}}}}}
	spaces := []struct{ label, text string }{{"S01", "\u2002"}, {"S02", "\u2003"}, {"S03", "\u2004"}, {"S04", "\u2005"}, {"S05", "\u2006"}, {"S06", "\u2007"}, {"S07", "\u2008"}, {"S08", "\u2009"}, {"S09", "\u200a"}, {"S10", "\u202f"}, {"S11", "\u205f"}, {"S12", "\u3000"}, {"S13", "\u00a0"}, {"S14", "\u200b"}, {"S15", "\u2060"}}
	for _, s := range spaces {
		d.Blocks = append(d.Blocks, manydoc.Block{Kind: manydoc.Paragraph, Runs: []manydoc.Run{{Text: s.label + " A" + s.text + "B"}}})
	}
	d.Blocks = append(d.Blocks,
		manydoc.Block{Kind: manydoc.Paragraph, Runs: []manydoc.Run{{Text: "Composed café / decomposed cafe\u0301", Underline: true}}},
		manydoc.Block{Kind: manydoc.Paragraph, Runs: []manydoc.Run{{Text: "Decorated A\u2003B", Underline: true, Strike: true, Link: "https://example.invalid/typography"}}},
		manydoc.Block{Kind: manydoc.Paragraph, Runs: []manydoc.Run{{Text: strings.Repeat("prefix ", 6)}, {Text: "inter", Bold: true}, {Text: "operability remains one word"}}},
		manydoc.Block{Kind: manydoc.Paragraph, Runs: []manydoc.Run{{Text: strings.Repeat("abc ", 10) + "hy\u00adphenation uses an optional hyphen"}}},
		manydoc.Block{Kind: manydoc.Paragraph, Runs: []manydoc.Run{{Text: "Anchor\u2003text"}, {Note: &manydoc.NoteReference{Kind: manydoc.Footnote, ID: "spacing"}}}},
	)
	d.Notes = []manydoc.Note{{Kind: manydoc.Footnote, ID: "spacing", Blocks: []manydoc.Block{{Kind: manydoc.Paragraph, Runs: []manydoc.Run{{Text: "Note\u2002spacing and cafe\u0301."}}}}}}
	if err := os.MkdirAll(out, 0755); err != nil {
		return err
	}
	for _, ext := range []manydoc.Format{manydoc.SF, manydoc.PDF, manydoc.DOCX} {
		f, err := os.Create(filepath.Join(out, "typography."+string(ext)))
		if err != nil {
			return err
		}
		writeErr := manydoc.Write(context.Background(), f, d, ext, manydoc.WriteOptions{})
		closeErr := f.Close()
		if writeErr != nil {
			return writeErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}
