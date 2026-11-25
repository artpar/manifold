package manifold

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestPDFTaggedNotePlacementKind(t *testing.T) {
	for _, tc := range []struct {
		name, role, line string
		skip             bool
		want             string
		inferred         bool
	}{
		{"separator", "Note", "20 100 m 120 100 l S\n", false, Footnote, true},
		{"no-separator", "Note", "", false, Endnote, true},
		{"short-line", "Note", "20 100 m 35 100 l S\n", false, Endnote, true},
		{"distant-line", "Note", "20 180 m 120 180 l S\n", false, Endnote, true},
		{"wrong-column", "Note", "150 100 m 250 100 l S\n", false, Endnote, true},
		{"skip", "Note", "20 100 m 120 100 l S\n", true, Endnote, true},
		{"explicit-endnote", "FENote /A << /O /FENote /NoteType /Endnote >>", "20 100 m 120 100 l S\n", false, Endnote, false},
		{"explicit-footnote", "FENote /A << /O /FENote /NoteType /Footnote >>", "", false, Footnote, false},
		{"none", "FENote /A << /O /FENote /NoteType /None >>", "20 100 m 120 100 l S\n", false, Footnote, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			objects := explicitNoteObjects(tc.role)
			content := strings.SplitN(strings.SplitN(objects[4], "stream\n", 2)[1], "\nendstream", 2)[0]
			objects[4] = recoveryStream("", tc.line+content)
			raw := recoveryPDF(objects...)
			d, err := ReadPDF(context.Background(), bytes.NewReader(raw), ReadOptions{PDFSkipInference: tc.skip})
			if err != nil {
				t.Fatal(err)
			}
			if len(d.Notes) != 1 || d.Notes[0].Kind != tc.want || d.Notes[0].KindInferred != tc.inferred {
				t.Fatalf("placement %+v", d.Notes)
			}
			if len(d.Notes[0].Blocks) != 4 || len(d.Blocks) != 1 {
				t.Fatal("rich ownership changed")
			}
			refs := 0
			for _, run := range d.Blocks[0].Runs {
				if run.Note != nil {
					refs++
					if run.Note.Kind != tc.want || run.Note.Inferred {
						t.Fatal("explicit edge changed", run.Note)
					}
				}
			}
			if refs != 2 {
				t.Fatal("reference inventory", refs)
			}
			baseline := readRecovery(t, raw)
			assertPDFNoteGlyphInventory(t, baseline, d)
			assertRecoverySF(t, d)
			for _, format := range []Format{SF, SFGzip, JSON, DOCX} {
				var buf bytes.Buffer
				if err := Write(context.Background(), &buf, d, format, WriteOptions{AllowLossy: true}); err != nil {
					t.Fatal(format, err)
				}
				back, err := Read(context.Background(), &buf, format, ReadOptions{})
				if err != nil {
					t.Fatal(format, err)
				}
				if len(back.Notes) != 1 || back.Notes[0].Kind != tc.want || back.Notes[0].KindInferred != tc.inferred {
					t.Fatal(format, "kind lost", back.Notes)
				}
				if format != DOCX && !reflect.DeepEqual(d.Notes, back.Notes) {
					t.Fatal(format, "note semantics changed")
				}
			}
			var rendered bytes.Buffer
			if err := Write(context.Background(), &rendered, d, PDF, WriteOptions{AllowLossy: true, PDFFontMap: map[string]string{"Courier": "Go Mono"}}); err != nil {
				t.Fatal("native placement", err)
			}
			if folder := os.Getenv("MANYDOC_PDF_NOTE_KIND_QA"); folder != "" && tc.name == "separator" {
				if err := os.MkdirAll(folder, 0755); err != nil {
					t.Fatal(err)
				}
				for name, data := range map[string][]byte{"source.pdf": raw, "native.pdf": rendered.Bytes()} {
					if err := os.WriteFile(filepath.Join(folder, name), data, 0600); err != nil {
						t.Fatal(err)
					}
				}
				for _, format := range []Format{SF, DOCX} {
					var output bytes.Buffer
					if err := Write(context.Background(), &output, d, format, WriteOptions{AllowLossy: true}); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(filepath.Join(folder, "model."+string(format)), output.Bytes(), 0600); err != nil {
						t.Fatal(err)
					}
				}
			}
			back := readRecovery(t, rendered.Bytes())
			if len(back.Notes) != 1 || back.Notes[0].Kind != tc.want || back.Notes[0].KindInferred {
				t.Fatal("rendered NoteType must declare the chosen reflow kind", back.Notes)
			}
		})
	}
}
