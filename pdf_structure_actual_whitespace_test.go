package manydoc

import (
	"bytes"
	"context"
	"testing"
)

func TestPDFActualWhitespaceAlignsOnlyMissingWhitespace(t *testing.T) {
	n := &pdfStructureNode{role: "Span"}
	bs := []Block{{Kind: Paragraph, Runs: []Run{{Text: "AB", Bold: true, Link: "https://example.org"}, {Text: "CD", Italic: true}}}}
	got, ok := alignPDFActualWhitespace(bs, "A B C D ", n)
	if !ok || paragraphText(got[0]) != "A B C D " {
		t.Fatal("missing whitespace not restored")
	}
	for _, r := range got[0].Runs {
		if r.Text == "A" || r.Text == "B" {
			if !r.Bold || r.Link != "https://example.org" {
				t.Fatal("original style/link lost")
			}
		}
		if r.Text == "C" || r.Text == "D" {
			if !r.Italic {
				t.Fatal("original style lost")
			}
		}
	}
	for _, text := range []string{"ABXCD", "ACD", "ABDC"} {
		if _, ok := alignPDFActualWhitespace(bs, text, n); ok {
			t.Fatal("non-whitespace mismatch accepted", text)
		}
	}
	painted := []Block{{Kind: Paragraph, Runs: []Run{{Text: "A B"}}}}
	if _, ok := alignPDFActualWhitespace(painted, "AB", n); ok {
		t.Fatal("painted whitespace silently discarded")
	}
}

func TestPDFWrappedReplacementRetainsMixedStylesAndWordBoundaries(t *testing.T) {
	d := &Document{Layout: Layout{Width: 130, Height: 180, Margin: 24}, Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "SUPERCALIFRAGILISTIC ", Bold: true, Link: "https://example.org/word"}, {Text: "hello wrapped world", Italic: true}}}}}
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, PDF, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	got, err := ReadPDF(context.Background(), &out, ReadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Blocks) != 1 || paragraphText(got.Blocks[0]) != "SUPERCALIFRAGILISTIC hello wrapped world" {
		t.Fatal("word boundaries changed", got.Text())
	}
	bold, italic := false, false
	for _, r := range got.Blocks[0].Runs {
		if r.Bold && r.Link == "https://example.org/word" {
			bold = true
		}
		if r.Italic {
			italic = true
		}
	}
	if !bold || !italic {
		t.Fatal("mixed styles/link lost during whitespace alignment")
	}
}
