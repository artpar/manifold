package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/artpar/manifold"
)

func TestIndependentFixturesPreservePageMarkersAndImages(t *testing.T) {
	for _, format := range []string{"pdf", "docx"} {
		t.Run(format, func(t *testing.T) {
			f := fixture{Name: "test", Pages: 3, Lines: 3, Format: format}
			var input bytes.Buffer
			if format == "pdf" {
				f.ImageEdge = 8
				if e := writePDF(&input, f); e != nil {
					t.Fatal(e)
				}
			} else {
				if e := writeDOCX(&input, f); e != nil {
					t.Fatal(e)
				}
			}
			d, e := manifold.Read(context.Background(), bytes.NewReader(input.Bytes()), manifold.Format(format), manifold.ReadOptions{})
			if e != nil {
				t.Fatal(e)
			}
			var text strings.Builder
			images, breaks := 0, 0
			for _, b := range d.Blocks {
				for _, r := range b.Runs {
					text.WriteString(r.Text)
				}
				if b.Kind == manifold.Picture {
					images++
				}
				if b.Kind == manifold.PageBreak {
					breaks++
				}
			}
			for p := 1; p <= 3; p++ {
				if !strings.Contains(text.String(), token(p)) {
					t.Fatalf("missing page %d", p)
				}
			}
			if format == "pdf" && (len(d.Pages) != 3 || images != 3) {
				t.Fatalf("pages/images %d/%d", len(d.Pages), images)
			}
			if format == "docx" && breaks != 2 {
				t.Fatalf("breaks %d", breaks)
			}
		})
	}
}
