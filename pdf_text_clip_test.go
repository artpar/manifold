package manydoc

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestPDFRectangularTextClipBoundsAndRestore(t *testing.T) {
	d := readRecovery(t, recoveryPage("q 10 20 10 20 re W n BT /F 10 Tf 15 25 Td (ABCD) Tj ET Q BT /F 10 Tf 30 25 Td (END) Tj ET", ""))
	runs := recoveryRuns(d)
	if len(runs) != 3 || runs[0].Text != "ABCD" || runs[1].Text != " " || runs[1].Source.Precision != "synthetic_separator" || runs[2].Text != "END" {
		t.Fatalf("logical clipping content %+v", runs)
	}
	a, _, b, _ := quadBounds(*runs[0].Source.Quad)
	if a != 15 || b != 20 || runs[0].Source.Precision != "rect_clipped_font_metrics" {
		t.Fatalf("clip %+v", runs[0].Source)
	}
	if strings.Contains(runs[2].Source.Precision, "clip") {
		t.Fatal("clip escaped Q restore")
	}
	if hasDiagnostic(d, "pdf_clipping") {
		t.Fatal("known text clip falsely unresolved")
	}
	assertRecoverySF(t, d)
}
func TestPDFComplexAndFullyClippedTextAreExplicit(t *testing.T) {
	for _, tc := range []struct{ prefix, precision, diagnostic string }{
		{"0 0 10 10 re W n ", "fully_clipped_font_metrics", "pdf_clipped_text"},
		{"0 0 m 100 0 l 100 100 l h W n ", "unresolved_clip_font_metrics", "pdf_clipping"},
	} {
		d := readRecovery(t, recoveryPage(tc.prefix+"BT /F 10 Tf 30 25 Td (VISIBLE-OPERATOR) Tj ET", ""))
		run := recoveryRuns(d)[0]
		if run.Text != "VISIBLE-OPERATOR" || run.Source.Precision != tc.precision || !hasDiagnostic(d, tc.diagnostic) {
			t.Fatalf("clip %+v %+v", run, d.Diagnostics)
		}
		assertRecoverySF(t, d)
	}
}
func FuzzPDFTextRectClip(f *testing.F) {
	f.Add(byte(10), byte(20), byte(15), byte(25))
	f.Add(byte(100), byte(50), byte(5), byte(80))
	f.Fuzz(func(t *testing.T, x, y, tx, ty byte) {
		content := fmt.Sprintf("%d %d 20 30 re W n BT /F 10 Tf %d %d Td (CLIPPED-TEXT) Tj ET", x, y, tx, ty)
		data := recoveryPage(content, "")
		d, err := ReadPDF(context.Background(), bytes.NewReader(data), ReadOptions{PDFSkipInference: true})
		if err != nil {
			t.Fatal(err)
		}
		if d.Text() != "CLIPPED-TEXT" {
			t.Fatalf("operator text discarded %q", d.Text())
		}
		assertRecoverySF(t, d)
	})
}
