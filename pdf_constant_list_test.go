package manydoc

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPDFConstantListPattern(t *testing.T) {
	for _, label := range []string{"", "§ fixed", "Case A", "100% done"} {
		for _, family := range []string{"Decimal", "UpperRoman", "LowerRoman", "UpperAlpha", "LowerAlpha"} {
			p, ok := pdfConstantListNumbering(family, []string{label, label})
			if !ok || p.start != 1 || p.numbering.Template != label {
				t.Fatal("constant labels not preserved", family, label)
			}
		}
	}
	for _, tc := range []struct {
		family string
		labels []string
	}{
		{"None", []string{"§", "§"}}, {"Disc", []string{"x", "x"}},
		{"Unknown", []string{"x"}}, {"Decimal", []string{"A", "B"}},
		{"Decimal", []string{"%1"}}, {"Decimal", []string{"x%2"}},
		{"Decimal", []string{strings.Repeat("x", 513)}}, {"Decimal", nil},
	} {
		if _, ok := pdfConstantListNumbering(tc.family, tc.labels); ok {
			t.Fatal("unproven literal template accepted", tc.family, tc.labels)
		}
	}
}

func TestGeneratedPDFConstantListFourPaths(t *testing.T) {
	ctx := context.Background()
	for index, label := range []string{"§ fixed", "Case A", "100% done", ""} {
		t.Run(fmt.Sprint(index), func(t *testing.T) {
			original := &Document{Blocks: []Block{{Kind: ListBlock, List: "number", Start: listStartPointer(42), Numbering: &ListNumbering{Template: label, Formats: []string{"lowerRoman"}}, Items: []ListItem{
				{Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "FIRST_BODY", Bold: true, Link: "https://example.org/first"}}}}},
				{Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "SECOND_BODY", Italic: true}}}}},
			}}}}
			sf, err := ToStructured(original)
			if err != nil {
				t.Fatal(err)
			}
			original, err = FromStructured(sf)
			if err != nil {
				t.Fatal(err)
			}
			var directWord bytes.Buffer
			if err = Write(ctx, &directWord, original, DOCX, WriteOptions{}); err != nil {
				t.Fatal(err)
			}
			direct, err := ReadDOCX(ctx, &directWord)
			if err != nil {
				t.Fatal(err)
			}
			assertPDFNumberedLabels(t, direct, 42, []string{label, label})
			if direct.Blocks[0].Numbering == nil || direct.Blocks[0].Numbering.Template != label || direct.Blocks[0].Numbering.Formats[0] != "lowerRoman" {
				t.Fatal("direct DOCX changed the authored hidden counter/template")
			}
			var pdf bytes.Buffer
			if err = Write(ctx, &pdf, original, PDF, WriteOptions{}); err != nil {
				t.Fatal(err)
			}
			rawPDF := bytes.Clone(pdf.Bytes())
			got, err := ReadPDF(ctx, &pdf, ReadOptions{})
			if err != nil {
				t.Fatal(err)
			}
			assertPDFNumberedLabels(t, got, 1, []string{label, label})
			if got.Blocks[0].Numbering == nil || got.Blocks[0].Numbering.Template != label || got.Blocks[0].Numbering.Formats[0] != "lowerRoman" || !hasDiagnostic(got, "pdf_list_constant_inferred") || strings.Contains(got.Blocks[0].Source.Evidence, "start_from_label") || !strings.Contains(got.Blocks[0].Source.Evidence, "start_unobserved") {
				t.Fatal("hidden start falsely recovered", got.Diagnostics)
			}
			for i, item := range got.Blocks[0].Items {
				if item.Source.Label != label || item.Source.Role != "LI" {
					t.Fatal("original explicit item/label evidence lost")
				}
				runs := item.Blocks[0].Runs
				if len(runs) != 1 || runs[0].Text != []string{"FIRST_BODY", "SECOND_BODY"}[i] || i == 0 && (!runs[0].Bold || runs[0].Link != "https://example.org/first") || i == 1 && !runs[0].Italic {
					t.Fatal("body formatting/relationship lost")
				}
			}
			assertRecoverySF(t, got)
			// Source PDF cannot expose the hidden counter start42. Once imported, the
			// explicit editorial default must survive SF/DOCX without further changes.
			var word bytes.Buffer
			if err = Write(ctx, &word, got, DOCX, WriteOptions{AllowLossy: true}); err != nil {
				t.Fatal(err)
			}
			rawWord := bytes.Clone(word.Bytes())
			back, err := ReadDOCX(ctx, &word)
			if err != nil {
				t.Fatal(err)
			}
			assertPDFNumberedLabels(t, back, 1, []string{label, label})
			if back.Blocks[0].Numbering == nil || back.Blocks[0].Numbering.Template != label || back.Blocks[0].Numbering.Formats[0] != "lowerRoman" {
				t.Fatal("DOCX literal format/template changed")
			}
			if dir := os.Getenv("MANYDOC_PDF_CONSTANT_LIST_QA"); dir != "" {
				if err = os.MkdirAll(dir, 0755); err != nil {
					t.Fatal(err)
				}
				var out bytes.Buffer
				tree, e := ToStructured(got)
				if e != nil {
					t.Fatal(e)
				}
				if err = WriteStructured(ctx, &out, tree); err != nil {
					t.Fatal(err)
				}
				for ext, data := range map[string][]byte{"pdf": rawPDF, "docx": rawWord, "sf": out.Bytes()} {
					if err = os.WriteFile(filepath.Join(dir, fmt.Sprintf("case-%d.%s", index, ext)), data, 0600); err != nil {
						t.Fatal(err)
					}
				}
			}
		})
	}
}

func FuzzPDFConstantListNumbering(f *testing.F) {
	f.Add("LowerRoman", "§ fixed", "§ fixed")
	f.Add("Decimal", "", "")
	f.Add("UpperAlpha", "A", "B")
	f.Add("Decimal", "%1", "%1")
	f.Fuzz(func(t *testing.T, family, first, second string) {
		if len(family) > 32 || len(first) > 1024 || len(second) > 1024 {
			t.Skip()
		}
		pattern, ok := pdfConstantListNumbering(family, []string{first, second})
		if !ok {
			return
		}
		if first != second || pattern.start != 1 || validateNumbering(pattern.numbering) != nil {
			t.Fatal("invalid literal pattern returned")
		}
		b := Block{Kind: ListBlock, List: "number", Numbering: pattern.numbering}
		for i := 0; i < 3; i++ {
			actual, err := numberedLabel(b, i)
			if err != nil || actual != first {
				t.Fatal("literal template became a counter", actual, err)
			}
		}
	})
}
