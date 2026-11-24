package manydoc

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

var pdfURICases = []struct{ iri, uri string }{
	{"https://example.invalid/a%2Fb?q=a+b&x=%25#part", "https://example.invalid/a%2Fb?q=a+b&x=%25#part"},
	{"http://www.example.org/red%09rosé#red", "http://www.example.org/red%09ros%C3%A9#red"}, // RFC 3987 §3.1
	{"#_CONNECT_–_Connection", "#_CONNECT_%E2%80%93_Connection"},
	{"#_CONNACK_–_Connect", "#_CONNACK_%E2%80%93_Connect"},
	{"#_Figure_4.3_–", "#_Figure_4.3_%E2%80%93"},
	{"https://例え.テスト/道?q=é#章", "https://%E4%BE%8B%E3%81%88.%E3%83%86%E3%82%B9%E3%83%88/%E9%81%93?q=%C3%A9#%E7%AB%A0"},
	{"mailto:δοκιμή@example.invalid?subject=😀", "mailto:%CE%B4%CE%BF%CE%BA%CE%B9%CE%BC%CE%AE@example.invalid?subject=%F0%9F%98%80"},
	{"https://example.invalid/cafe\u0301", "https://example.invalid/cafe%CC%81"},
	{"https://example.invalid/café", "https://example.invalid/caf%C3%A9"},
	{"https://example.invalid/a b<\"{x}|\\^`>\t\n", "https://example.invalid/a%20b%3C%22%7Bx%7D%7C%5C%5E%60%3E%09%0A"},
	{"https://[2001:db8::1]/a(b)?x=!$&y='*+,;=:@/#end", "https://[2001:db8::1]/a(b)?x=!$&y='*+,;=:@/#end"},
}

func TestPDFIRIToASCIIURI(t *testing.T) {
	for _, c := range pdfURICases {
		if got := pdfASCIIURI(c.iri); got != c.uri {
			t.Fatalf("IRI %q: got %q want %q", c.iri, got, c.uri)
		}
		if got := pdfASCIIURI(c.uri); got != c.uri {
			t.Fatal("URI encoding not idempotent", got)
		}
	}
}

func TestPDFUnicodeURITargetsAndSourceOwnership(t *testing.T) {
	d := &Document{}
	want := map[string]bool{}
	for _, c := range pdfURICases {
		d.Blocks = append(d.Blocks, Block{Kind: Paragraph, Runs: []Run{{Text: "LINK", Link: c.iri}}})
		want[c.uri] = true
	}
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, PDF, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	for i, c := range pdfURICases {
		if d.Blocks[i].Runs[0].Link != c.iri {
			t.Fatal("source target mutated")
		}
	}
	back, err := ReadPDF(context.Background(), bytes.NewReader(out.Bytes()), ReadOptions{PDFSkipInference: true})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, b := range back.Blocks {
		for _, r := range b.Runs {
			if r.Link != "" {
				got[r.Link] = true
			}
		}
	}
	if len(got) != len(want) {
		t.Fatal("target count changed", got)
	}
	for uri := range want {
		if !got[uri] {
			t.Fatal("target changed", uri, got)
		}
	}
	if strings.Count(back.Text(), "LINK") != len(pdfURICases) {
		t.Fatal("display text lost")
	}
}

func FuzzPDFASCIIURI(f *testing.F) {
	for _, c := range pdfURICases {
		f.Add(c.iri)
	}
	f.Fuzz(func(t *testing.T, iri string) {
		if len(iri) > 4096 {
			t.Skip()
		}
		got := pdfASCIIURI(iri)
		if got != pdfASCIIURI(got) {
			t.Fatal("non-idempotent URI")
		}
		for _, b := range []byte(got) {
			if b < 0x21 || b >= 0x7f || strings.ContainsRune("<>\"{}|\\^`", rune(b)) {
				t.Fatalf("non-URI byte %x", b)
			}
		}
	})
}
