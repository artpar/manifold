package manydoc

import (
	"bytes"
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"unicode"
)

func TestPDFUntaggedCorpusCoverage(t *testing.T) {
	for _, c := range corpus(t) {
		if c.Kind != "pdf" || c.Expect == "reject" {
			continue
		}
		t.Run(c.Name, func(t *testing.T) {
			data := fixture(t, c.Name)
			raw, e := ReadPDF(context.Background(), bytes.NewReader(data), ReadOptions{Password: "secret", PDFSkipInference: true})
			if e != nil {
				t.Fatal(e)
			}
			got, e := ReadPDF(context.Background(), bytes.NewReader(data), ReadOptions{Password: "secret"})
			if e != nil {
				t.Fatal(e)
			}
			coverage := func(d *Document) (map[rune]int, map[int]Block, map[int]Run) {
				chars := map[rune]int{}
				assets := map[int]Block{}
				spans := map[int]Run{}
				add := func(s string) {
					for _, ch := range s {
						if !unicode.IsSpace(ch) {
							chars[ch]++
						}
					}
				}
				var walk func([]Block)
				walk = func(bs []Block) {
					for _, b := range bs {
						if b.Kind == Picture {
							assets[b.Source.Order] = b
						}
						for _, run := range b.Runs {
							if run.Source != nil && run.Source.Precision == "synthetic_separator" {
								continue
							}
							add(run.Text)
							if run.Source != nil {
								spans[run.Source.Order] = run
							}
						}
						for _, item := range b.Items {
							if item.Source != nil {
								add(item.Source.Label)
							}
							walk(item.Blocks)
						}
						for _, row := range b.Rows {
							for _, cell := range row.Cells {
								walk(cell.Blocks)
							}
						}
					}
				}
				walk(d.Blocks)
				return chars, assets, spans
			}
			x, a, spans := coverage(raw)
			y, b, gotSpans := coverage(got)
			if !reflect.DeepEqual(x, y) || !reflect.DeepEqual(a, b) || !reflect.DeepEqual(raw.Pages, got.Pages) {
				t.Fatal("inference lost/duplicated literal content, assets, placements or page geometry")
			}
			if !hasDiagnostic(got, "pdf_list_inferred") && !reflect.DeepEqual(spans, gotSpans) {
				t.Fatal("inference changed original formatting, relationships or source spans")
			}
		})
	}
}

func inferenceText(font string, size, x, y int, text string) string {
	text = strings.NewReplacer(`\`, `\\`, `(`, `\(`, `)`, `\)`).Replace(text)
	return fmt.Sprintf("BT /%s %d Tf 1 0 0 1 %d %d Tm (%s) Tj ET\n", font, size, x, y, text)
}

func FuzzPDFInferenceCoverage(f *testing.F) {
	f.Add([]byte{30, 40, 10, 50, 60, 70, 80, 90})
	f.Add([]byte{0, 0, 0, 0, 255, 255, 255, 255})
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 64 {
			t.Skip()
		}
		n := max(1, len(data))
		content := ""
		for i := 0; i < n; i++ {
			v := byte(0)
			if len(data) > 0 {
				v = data[i]
			}
			font := "F"
			if v&1 != 0 {
				font = "B"
			}
			content += inferenceText(font, 6+int(v%15), 20+int(v)*2, 50+int(v)*2, fmt.Sprintf("TOKEN%03d", i))
		}
		pdf := recoveryPDF(inferenceObjects(content)...)
		raw, e := ReadPDF(context.Background(), bytes.NewReader(pdf), ReadOptions{PDFSkipInference: true})
		if e != nil {
			t.Fatal(e)
		}
		got, e := ReadPDF(context.Background(), bytes.NewReader(pdf), ReadOptions{})
		if e != nil {
			t.Fatal(e)
		}
		original := map[int]Run{}
		for _, b := range raw.Blocks {
			for _, run := range b.Runs {
				if run.Source.Precision != "synthetic_separator" {
					original[run.Source.Order] = run
				}
			}
		}
		seen := map[int]bool{}
		var walk func([]Block)
		walk = func(bs []Block) {
			for _, b := range bs {
				for _, run := range b.Runs {
					if run.Source.Precision == "synthetic_separator" {
						continue
					}
					if seen[run.Source.Order] || !reflect.DeepEqual(run, original[run.Source.Order]) {
						t.Fatal("inference duplicated, changed or invented painted content/geometry")
					}
					seen[run.Source.Order] = true
				}
				for _, item := range b.Items {
					walk(item.Blocks)
				}
				for _, row := range b.Rows {
					for _, cell := range row.Cells {
						walk(cell.Blocks)
					}
				}
			}
		}
		walk(got.Blocks)
		if len(seen) != n {
			t.Fatalf("lost source spans: got %d want %d", len(seen), n)
		}
		assertRecoverySF(t, got)
	})
}
func inferenceObjects(content string) []string {
	return []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 600 800] /Resources << /Font << /F 4 0 R /B 5 0 R >> /XObject << /Im 7 0 R >> >> /Contents 6 0 R >>",
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica-Bold >>",
		recoveryStream("", content),
		recoveryStream("/Type /XObject /Subtype /Image /Width 1 /Height 1 /BitsPerComponent 8 /ColorSpace /DeviceRGB", string([]byte{255, 0, 0})),
	}
}
func readInferred(t *testing.T, objects []string) *Document {
	t.Helper()
	d, e := ReadPDF(context.Background(), bytes.NewReader(recoveryPDF(objects...)), ReadOptions{})
	if e != nil {
		t.Fatal(e)
	}
	if !hasDiagnostic(d, "pdf_structure_inferred") {
		t.Fatal("inference not disclosed")
	}
	assertRecoverySF(t, d)
	return d
}

func TestPDFUntaggedColumnsParagraphsHeadingAndLinks(t *testing.T) {
	// Paint order alternates columns and reverses both pairs of lines. The
	// independent oracle is the hand-authored placement, not a writer round trip.
	content := inferenceText("F", 10, 330, 676, "RIGHT continues in the same paragraph.")
	content += inferenceText("F", 10, 40, 640, "LEFT second paragraph is separated.")
	content += inferenceText("B", 22, 40, 750, "Quarterly review and supporting measurements")
	content += inferenceText("F", 10, 40, 676, "LEFT continues in the same paragraph.")
	content += inferenceText("F", 10, 330, 690, "RIGHT begins here; then continues")
	content += inferenceText("F", 10, 40, 690, "LEFT starts with the first paragraph")
	content += inferenceText("F", 10, 330, 640, "RIGHT second paragraph is separated.")
	objects := inferenceObjects(content)
	objects[2] = strings.Replace(objects[2], "/Contents 6 0 R", "/Annots [8 0 R] /Contents 6 0 R", 1)
	objects = append(objects, "<< /Type /Annot /Subtype /Link /Rect [329 687 590 700] /A << /S /URI /URI (https://example.invalid/right) >> >>")
	d := readInferred(t, objects)
	want := []string{"Quarterly review and supporting measurements", "LEFT starts with the first paragraph LEFT continues in the same paragraph.", "LEFT second paragraph is separated.", "RIGHT begins here; then continues RIGHT continues in the same paragraph.", "RIGHT second paragraph is separated."}
	if len(d.Blocks) != len(want) {
		t.Fatalf("column boundaries: %+v", d.Blocks)
	}
	seen := map[int]bool{}
	for i, b := range d.Blocks {
		if layoutText(b) != want[i] || b.Source.Structure != "inferred" {
			t.Fatalf("block %d: %+v", i, b)
		}
		for _, run := range b.Runs {
			if run.Source.Precision == "synthetic_separator" {
				continue
			}
			if run.Source.Structure != "unstructured" || seen[run.Source.Order] {
				t.Fatal("paint ownership duplicated or promoted to explicit")
			}
			seen[run.Source.Order] = true
		}
	}
	if len(seen) != 7 || d.Blocks[0].Kind != Heading || d.Blocks[0].Level != 1 || !d.Blocks[0].Runs[0].Bold || d.Blocks[3].Runs[0].Link != "https://example.invalid/right" {
		t.Fatal("heading/style/URI or complete original spans lost")
	}
	if d.Blocks[1].Source.Quad[1].X >= 300 || d.Blocks[3].Source.Quad[0].X < 329 {
		t.Fatal("columns merged spatially")
	}
	for _, b := range d.Blocks {
		if b.Kind == Table || b.Kind == ListBlock {
			t.Fatal("prose columns misclassified")
		}
	}
}

func TestPDFUntaggedTableListContinuationAndAssets(t *testing.T) {
	content := ""
	for row, values := range [][]string{{"Item", "Count", "Price"}, {"Apples", "12", "$5.00"}, {"Pears", "34", "$9.25"}} {
		for col := 2; col >= 0; col-- {
			content += inferenceText("F", 10, []int{40, 220, 340}[col], 650-row*14, values[col])
		}
	}
	content += inferenceText("F", 10, 40, 550, "3. First numbered item")
	content += inferenceText("F", 10, 54, 536, "continued at the hanging indent.")
	content += inferenceText("F", 10, 40, 522, "4. Second numbered item")
	content += inferenceText("F", 10, 58, 508, "- Nested one")
	content += inferenceText("F", 10, 58, 494, "- Nested two")
	content += "q 40 0 0 30 400 210 cm /Im Do Q\n"
	d := readInferred(t, inferenceObjects(content))
	if len(d.Blocks) != 3 || d.Blocks[0].Kind != Table || d.Blocks[1].Kind != ListBlock || d.Blocks[2].Kind != Picture {
		t.Fatalf("table/list/image membership: %+v", d.Blocks)
	}
	table := d.Blocks[0]
	if len(table.Rows) != 3 || len(table.Rows[0].Cells) != 3 || table.Source.Evidence != "aligned_rows_numeric_columns" {
		t.Fatalf("table dimensions: %+v", table)
	}
	for row, values := range [][]string{{"Item", "Count", "Price"}, {"Apples", "12", "$5.00"}, {"Pears", "34", "$9.25"}} {
		for col, want := range values {
			cell := table.Rows[row].Cells[col]
			if len(cell.Blocks) != 1 || layoutText(cell.Blocks[0]) != want || cell.Source.Structure != "inferred" || cell.Source.Quad == nil {
				t.Fatalf("cell %d/%d: %+v", row, col, cell)
			}
		}
	}
	list := d.Blocks[1]
	if list.List != "number" || list.Start == nil || *list.Start != 3 || len(list.Items) != 2 || list.Items[1].Source.Label != "4." {
		t.Fatalf("numbered list: %+v", list)
	}
	if layoutText(list.Items[0].Blocks[0]) != "First numbered item continued at the hanging indent." {
		t.Fatalf("continuation: %+v", list.Items[0])
	}
	if len(list.Items[1].Blocks) != 2 || list.Items[1].Blocks[1].Kind != ListBlock || len(list.Items[1].Blocks[1].Items) != 2 || list.Items[1].Blocks[1].Items[0].Source.Label != "-" {
		t.Fatalf("nested list: %+v", list.Items[1])
	}
	if list.Items[0].Blocks[0].Runs[0].Source.Precision != "partial_text_span_bounds" {
		t.Fatal("prefix slicing claimed exact bounds")
	}
	if d.Blocks[2].Image == nil || len(d.Blocks[2].Image.Data) == 0 || d.Blocks[2].Source.Quad == nil || !hasDiagnostic(d, "pdf_list_inferred") || !hasDiagnostic(d, "pdf_table_inferred") {
		t.Fatal("asset or inference evidence missing")
	}
}

func TestPDFUntaggedNumberedValuesAreNotTableCells(t *testing.T) {
	content := inferenceText("F", 10, 40, 600, "3.") + inferenceText("F", 10, 100, 600, "2021")
	content += inferenceText("F", 10, 40, 586, "4.") + inferenceText("F", 10, 100, 586, "2022")
	d := readInferred(t, inferenceObjects(content))
	if len(d.Blocks) != 1 || d.Blocks[0].Kind != ListBlock || d.Blocks[0].Start == nil || *d.Blocks[0].Start != 3 || len(d.Blocks[0].Items) != 2 || layoutText(d.Blocks[0].Items[0].Blocks[0]) != "2021" || hasDiagnostic(d, "pdf_table_inferred") {
		t.Fatalf("numbered values misclassified: %+v", d.Blocks)
	}
}

func TestPDFUntaggedSpanningEdgesDoNotInterleaveColumns(t *testing.T) {
	content := inferenceText("F", 10, 330, 560, "RIGHT lower paragraph has a large preceding gap.")
	content += inferenceText("B", 20, 40, 750, "Quarterly review and supporting measurements")
	content += inferenceText("F", 10, 40, 710, "LEFT upper paragraph starts here.")
	content += inferenceText("F", 10, 330, 710, "RIGHT upper paragraph starts here.")
	content += inferenceText("F", 10, 40, 560, "LEFT lower paragraph has a large preceding gap.")
	content += inferenceText("F", 12, 40, 480, "FULL WIDTH concluding text spans both of the columns before the page ends.")
	d := readInferred(t, inferenceObjects(content))
	want := []string{"Quarterly review and supporting measurements", "LEFT upper paragraph starts here.", "LEFT lower paragraph has a large preceding gap.", "RIGHT upper paragraph starts here.", "RIGHT lower paragraph has a large preceding gap.", "FULL WIDTH concluding text spans both of the columns before the page ends."}
	if len(d.Blocks) != len(want) {
		t.Fatalf("spanning edge boundaries: %+v", d.Blocks)
	}
	for i, b := range d.Blocks {
		if layoutText(b) != want[i] {
			t.Fatalf("block %d: %q want %q", i, layoutText(b), want[i])
		}
	}
}

func TestPDFUntaggedUncertainGeometryAndOptOut(t *testing.T) {
	for name, content := range map[string]string{
		"vertical":              "BT /F 10 Tf 0 1 -1 0 20 100 Tm (vertical) Tj ET",
		"logical_without_paint": "/Span << /ActualText <FEFF006C006F0067006900630061006C> >> BDC EMC",
	} {
		t.Run(name, func(t *testing.T) {
			d := readInferred(t, inferenceObjects(content))
			if !hasDiagnostic(d, "pdf_inference_uncertain") || !strings.Contains(d.Text(), map[string]string{"vertical": "vertical", "logical_without_paint": "logical"}[name]) {
				t.Fatal("uncertain content omitted or assigned silent confidence")
			}
		})
	}
	content := "q 40 0 0 20 10 30 cm /Im Do Q\n" + inferenceText("F", 10, 10, 100, "ABOVE")
	objects := inferenceObjects(content)
	d := readInferred(t, objects)
	if len(d.Blocks) != 2 || d.Blocks[0].Kind != Paragraph || d.Blocks[1].Kind != Picture {
		t.Fatal("displayed text above image was left in paint order")
	}
	raw, e := ReadPDF(context.Background(), bytes.NewReader(recoveryPDF(objects...)), ReadOptions{PDFSkipInference: true})
	if e != nil || raw.Blocks[0].Kind != Picture || raw.Blocks[1].Runs[0].Text != "ABOVE" || hasDiagnostic(raw, "pdf_structure_inferred") {
		t.Fatalf("physical extraction opt-out: %+v, %v", raw, e)
	}
	overlap := "q 100 0 0 40 10 80 cm /Im Do Q\n" + inferenceText("F", 10, 20, 100, "OVERLAY")
	d = readInferred(t, inferenceObjects(overlap))
	if len(d.Blocks) != 2 || !strings.Contains(d.Text(), "OVERLAY") || !hasDiagnostic(d, "pdf_inference_uncertain") {
		t.Fatal("overlapping image/text omitted or ordering certainty invented")
	}
}
