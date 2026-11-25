package manifold

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Every branch invokes the same child repeatedly: the source stays linear while
// actual Form execution is exponential. These are hand-authored PDF objects,
// independent of the production writer and below the existing depth32 guard.
func pdfXObjectDAG(levels, fanout, pages int, painted, appearances bool) []byte {
	font, content, first := 3+pages, 4+pages, 5+pages
	kids := make([]string, pages)
	for i := range kids {
		kids[i] = fmt.Sprintf("%d 0 R", 3+i)
	}
	objects := []string{"<< /Type /Catalog /Pages 2 0 R >>", fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), pages)}
	for i := 0; i < pages; i++ {
		attrs := ""
		if appearances {
			refs := make([]string, 6)
			for j := range refs {
				refs[j] = fmt.Sprintf("%d 0 R", first+levels+1+j)
			}
			attrs = "/Annots [" + strings.Join(refs, " ") + "]"
		}
		objects = append(objects, fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 400] /Resources << /Font << /F %d 0 R >> /XObject << /X %d 0 R >> >> /Contents %d 0 R %s >>", font, first, content, attrs))
	}
	objects = append(objects, "<< /Type /Font /Subtype /Type1 /BaseFont /Courier >>")
	stream := "BT /F 10 Tf 1 0 0 1 0 350 Tm (KEEP) Tj ET"
	if !appearances {
		stream += " /X Do"
	}
	objects = append(objects, recoveryStream("", stream))
	for i := 0; i <= levels; i++ {
		body := ""
		resources := ""
		if i < levels {
			resources = fmt.Sprintf("/Resources << /XObject << /Next %d 0 R >> /Font << /F %d 0 R >> >>", first+i+1, font)
			step := 1
			for j := i + 1; j < levels; j++ {
				step *= fanout
			}
			for branch := 0; branch < fanout; branch++ {
				shift := 0
				if painted {
					shift = 10 * branch * step
				}
				body += fmt.Sprintf("q 1 0 0 1 %d 0 cm /Next Do Q ", shift)
			}
		} else if painted {
			body = "BT /F 10 Tf 1 0 0 1 0 300 Tm (LEAF) Tj ET"
		}
		objects = append(objects, recoveryStream("/Type /XObject /Subtype /Form /BBox [0 0 300 400] "+resources, body))
	}
	if appearances {
		for i := 0; i < 6; i++ {
			objects = append(objects, fmt.Sprintf("<< /Type /Annot /Subtype /Widget /Rect [0 0 300 400] /AP << /N %d 0 R >> >>", first))
		}
	}
	return recoveryPDF(objects...)
}

func TestPDFXObjectExpansionBudget(t *testing.T) {
	for _, tc := range []struct {
		name          string
		levels, pages int
		appearances   bool
	}{
		{"acyclic-shared-form", 6, 1, false},
		{"shared-across-pages", 5, 6, false},
		{"shared-appearances", 5, 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := pdfXObjectDAG(tc.levels, 8, tc.pages, false, tc.appearances)
			if dir := os.Getenv("MANYDOC_PDF_XOBJECT_QA"); dir != "" {
				if err := os.MkdirAll(dir, 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, tc.name+".pdf"), data, 0600); err != nil {
					t.Fatal(err)
				}
			}
			if len(data) > 6000 {
				t.Fatal("fixture materialized the expanded graph", len(data))
			}
			d, err := ReadPDF(context.Background(), bytes.NewReader(data), ReadOptions{})
			if err == nil || d != nil || !strings.Contains(err.Error(), "XObject expansion exceeds") {
				t.Fatalf("compact repeated Form graph was not bounded: doc=%v err=%v", d != nil, err)
			}
		})
	}
}

func TestPDFXObjectRepeatedPaintPreservesPlacements(t *testing.T) {
	d := readRecovery(t, pdfXObjectDAG(3, 2, 1, true, false))
	runs := recoveryRuns(d)
	if len(runs) != 9 || runs[0].Text != "KEEP" {
		t.Fatal("missing actual Form occurrences", len(runs), d.Text())
	}
	stream := ""
	for i, run := range runs[1:] {
		if run.Text != "LEAF" || run.Source == nil || run.Source.Page != 1 || run.Source.Stream == "" || run.Source.Structure != "unstructured" {
			t.Fatal("Form paint/source identity changed", run)
		}
		if i == 0 {
			stream = run.Source.Stream
		}
		if run.Source.Stream != stream {
			t.Fatal("shared source was replaced by duplicated definitions")
		}
		closePoint(t, run.Source.Baseline[0], float64(i*10), 100)
		closePoint(t, run.Source.Baseline[1], float64(i*10+24), 100)
	}
	assertRecoverySF(t, d)
}

func FuzzPDFXObjectGraph(f *testing.F) {
	for _, s := range [][3]uint8{{6, 7, 0}, {5, 7, 5}, {3, 1, 0}, {0, 0, 1}} {
		f.Add(s[0], s[1], s[2])
	}
	f.Fuzz(func(t *testing.T, depth, branches, count uint8) {
		levels, fanout, pages := int(depth)%7, int(branches)%8+1, int(count)%6+1
		visits, atLevel := 0, 1
		for i := 0; i <= levels; i++ {
			visits += atLevel
			atLevel *= fanout
		}
		data := pdfXObjectDAG(levels, fanout, pages, false, false)
		d, err := ReadPDF(context.Background(), bytes.NewReader(data), ReadOptions{PDFSkipInference: true})
		if visits*pages > maxPDFXObjectVisits {
			if err == nil || d != nil || !strings.Contains(err.Error(), "XObject expansion exceeds") {
				t.Fatalf("expanded graph did not reject: %d visits, %v", visits*pages, err)
			}
			return
		}
		if err != nil {
			t.Fatal(err)
		}
		if len(d.Pages) != pages || strings.Count(d.Text(), "KEEP") != pages {
			t.Fatal("per-page source content lost", d.Text())
		}
		for _, run := range recoveryRuns(d) {
			if run.Text != "KEEP" || run.Source == nil || run.Source.Page < 1 || run.Source.Page > pages {
				t.Fatal("source identity lost", run)
			}
		}
		assertRecoverySF(t, d)
	})
}
