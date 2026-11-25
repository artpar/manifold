package manifold

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestPDFListNumberingCanonicalPatterns(t *testing.T) {
	for _, tc := range []struct {
		attribute        string
		labels           []string
		start            int
		format, template string
	}{
		{"LowerRoman", []string{"iv.", "v.", "vi."}, 4, "lowerRoman", "%1."},
		{"UpperRoman", []string{"(IV)", "(V)"}, 4, "upperRoman", "(%1)"},
		{"UpperRoman", []string{"MMMCMXCIX.", "MMMM."}, 3999, "upperRoman", "%1."},
		{"UpperAlpha", []string{"Z)", "AA)", "BB)"}, 26, "upperLetter", "%1)"},
		{"LowerAlpha", []string{"Annex aa:", "Annex bb:"}, 27, "lowerLetter", "Annex %1:"},
		{"Decimal", []string{"Section 03)", "Section 04)"}, 3, "decimalZero", "Section %1)"},
		{"Decimal", []string{"0.", "1."}, 0, "decimal", "%1."},
		{"Decimal", []string{"2147483646.", "2147483647."}, 2147483646, "decimal", "%1."},
		{"None", []string{"Step 2", "Step 3"}, 2, "decimal", "Step %1"},
	} {
		t.Run(tc.attribute+tc.labels[0], func(t *testing.T) {
			pattern, ok := pdfListNumberingPattern(tc.attribute, tc.labels)
			if !ok || pattern.start != tc.start {
				t.Fatal("canonical sequence not recovered", pattern, ok)
			}
			if pattern.numbering == nil {
				if tc.format != "decimal" || tc.template != "%1." {
					t.Fatal("format/template lost")
				}
			} else if pattern.numbering.Template != tc.template || !reflect.DeepEqual(pattern.numbering.Formats, []string{tc.format}) {
				t.Fatal("incorrect editable numbering", pattern.numbering)
			}
			b := Block{Kind: ListBlock, List: "number", Start: listStartPointer(pattern.start), Numbering: pattern.numbering}
			for i, want := range tc.labels {
				got, err := numberedLabel(b, i)
				if err != nil || got != want {
					t.Fatal("literal marker changed", i, got, want, err)
				}
			}
		})
	}
	for _, tc := range []struct {
		attribute string
		labels    []string
	}{
		{"LowerRoman", []string{"iiv.", "iv."}}, {"UpperRoman", []string{"iv.", "v."}},
		{"UpperAlpha", []string{"AA.", "AB."}}, {"LowerAlpha", []string{"α.", "β."}},
		{"LowerRoman", []string{"iv.", "vi."}}, {"UpperRoman", []string{"I II"}},
		{"Decimal", []string{"3.", "4)"}}, {"Decimal", []string{"2147483647.", "2147483648."}},
		{"None", []string{"iv.", "v."}}, {"Unknown", []string{"1.", "2."}},
		{"Decimal", []string{strings.Repeat("x", 513) + "1."}},
		{"Decimal", []string{"%1 3.", "%1 4."}},
	} {
		if _, ok := pdfListNumberingPattern(tc.attribute, tc.labels); ok {
			t.Fatal("unproven/unsupported template accepted", tc)
		}
	}
}

func TestPDFListNumberingConflictingFamilyStaysLiteral(t *testing.T) {
	for _, family := range []string{"LowerRoman", "UpperRoman", "LowerAlpha", "UpperAlpha"} {
		t.Run(family, func(t *testing.T) {
			raw := taggedTestPDF(markedText(0, 20, 300, "3.")+markedText(1, 40, 300, "Body"), "<< /Type /StructTreeRoot /K 8 0 R >>",
				"<< /Type /StructElem /S /L /P 7 0 R /Pg 3 0 R /A << /O /List /ListNumbering /"+family+" >> /K 9 0 R >>",
				"<< /Type /StructElem /S /LI /P 8 0 R /K [10 0 R 11 0 R] >>",
				"<< /Type /StructElem /S /Lbl /P 9 0 R /K 0 >>",
				"<< /Type /StructElem /S /LBody /P 9 0 R /K 1 >>")
			d := readRecovery(t, raw)
			assertPDFNumberedLabels(t, d, 1, []string{"3."})
			if d.Blocks[0].Numbering == nil || d.Blocks[0].Numbering.Template != "3." || d.Blocks[0].Items[0].Source.Label != "3." || !hasDiagnostic(d, "pdf_list_constant_inferred") || strings.Contains(d.Blocks[0].Source.Evidence, "start_from_label") {
				t.Fatal("conflicting family became a false dynamic counter")
			}
			physical, err := ReadPDF(context.Background(), bytes.NewReader(raw), ReadOptions{PDFSkipStructure: true, PDFSkipInference: true})
			if err != nil || physical.Text() != "3. Body" {
				t.Fatal("original literal paint oracle changed", err)
			}
			assertRecoverySF(t, d)
		})
	}
}

func TestGeneratedPDFNumberedListSemantics(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		format, template string
		start            int
		labels           []string
		legal            bool
	}{
		{"lowerRoman", "%1.", 4, []string{"iv.", "v.", "vi."}, false},
		{"upperRoman", "(%1)", 4, []string{"(IV)", "(V)", "(VI)"}, false},
		{"upperLetter", "Annex %1)", 26, []string{"Annex Z)", "Annex AA)", "Annex BB)"}, false},
		{"lowerLetter", "%1:", 27, []string{"aa:", "bb:", "cc:"}, false},
		{"decimalZero", "%1)", 8, []string{"08)", "09)", "10)"}, false},
		{"upperRoman", "%1.", 4, []string{"4.", "5.", "6."}, true},
	} {
		t.Run(tc.format+tc.template+fmt.Sprint(tc.legal), func(t *testing.T) {
			list := Block{Kind: ListBlock, List: "number", Start: listStartPointer(tc.start), Numbering: &ListNumbering{Template: tc.template, Formats: []string{tc.format}, Legal: tc.legal}}
			for i := range tc.labels {
				list.Items = append(list.Items, ListItem{Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: fmt.Sprintf("BODY_%d", i), Bold: true, Link: "https://example.org/list"}}}}})
			}
			want := &Document{Blocks: []Block{list}}
			var pdf bytes.Buffer
			if err := Write(ctx, &pdf, want, PDF, WriteOptions{}); err != nil {
				t.Fatal(err)
			}
			rawPDF := bytes.Clone(pdf.Bytes())
			got, err := ReadPDF(ctx, &pdf, ReadOptions{})
			if err != nil {
				t.Fatal(err)
			}
			assertPDFNumberedLabels(t, got, tc.start, tc.labels)
			if got.Blocks[0].Source.Structure != "explicit" || got.Blocks[0].Source.Role != "L" {
				t.Fatal("explicit hierarchy lost")
			}
			for i, item := range got.Blocks[0].Items {
				if item.Source.Label != tc.labels[i] || !item.Blocks[0].Runs[0].Bold || item.Blocks[0].Runs[0].Link != "https://example.org/list" {
					t.Fatal("item content/style/source relationship lost")
				}
			}
			assertRecoverySF(t, got)
			sf, err := ToStructured(got)
			if err != nil {
				t.Fatal(err)
			}
			through, err := FromStructured(sf)
			if err != nil {
				t.Fatal(err)
			}
			assertPDFNumberedLabels(t, through, tc.start, tc.labels)
			var word bytes.Buffer
			if err = Write(ctx, &word, got, DOCX, WriteOptions{AllowLossy: true}); err != nil {
				t.Fatal(err)
			}
			rawWord := bytes.Clone(word.Bytes())
			reopened, err := ReadDOCX(ctx, &word)
			if err != nil {
				t.Fatal(err)
			}
			assertPDFNumberedLabels(t, reopened, tc.start, tc.labels)
			if dir := os.Getenv("MANYDOC_PDF_LIST_NUMBERING_QA"); dir != "" {
				if err = os.MkdirAll(dir, 0755); err != nil {
					t.Fatal(err)
				}
				rawSF, err := json.Marshal(sf)
				if err != nil {
					t.Fatal(err)
				}
				stem := fmt.Sprintf("%s-%t", tc.format, tc.legal)
				for ext, data := range map[string][]byte{"pdf": rawPDF, "docx": rawWord, "sf": rawSF} {
					if err = os.WriteFile(filepath.Join(dir, stem+"."+ext), data, 0600); err != nil {
						t.Fatal(err)
					}
				}
			}
		})
	}
}

func assertPDFNumberedLabels(t *testing.T, d *Document, start int, labels []string) {
	t.Helper()
	if len(d.Blocks) != 1 || d.Blocks[0].Kind != ListBlock || d.Blocks[0].List != "number" || len(d.Blocks[0].Items) != len(labels) || listStart(d.Blocks[0]) != start {
		t.Fatal("numbered hierarchy/start lost", d.Text(), d.Diagnostics)
	}
	for i, want := range labels {
		got, err := numberedLabel(d.Blocks[0], i)
		if err != nil || got != want {
			t.Fatal("numbered marker changed", i, got, want, err)
		}
	}
}

func TestPDFCounterAnnotationsStayLiteral(t *testing.T) {
	for _, href := range []string{"https://example.org/marker"} {
		objects := taggedTestObjects(markedText(0, 20, 300, "3.")+markedText(1, 50, 300, "Body"),
			`<< /Type /StructTreeRoot /K 8 0 R >>`,
			`<< /Type /StructElem /S /L /P 7 0 R /Pg 3 0 R /A << /O /List /ListNumbering /Decimal >> /K 9 0 R >>`,
			`<< /Type /StructElem /S /LI /P 8 0 R /K [10 0 R 11 0 R] >>`,
			`<< /Type /StructElem /S /Lbl /P 9 0 R /K 12 0 R >>`,
			`<< /Type /StructElem /S /LBody /P 9 0 R /K 1 >>`,
			`<< /Type /StructElem /S /Link /P 10 0 R /K [0 << /Type /OBJR /Obj 13 0 R >>] >>`,
			`<< /Type /Annot /Subtype /Link /Rect [20 310 32 296] /A << /S /URI /URI (`+href+`) >> >>`)
		objects[2] = strings.Replace(objects[2], "/Contents 5 0 R", "/Contents 5 0 R /Annots [13 0 R]", 1)
		raw := recoveryPDF(objects...)
		d := readRecovery(t, raw)
		if len(d.Blocks) != 1 || d.Blocks[0].Kind != ListBlock || len(d.Blocks[0].Items) != 1 || !hasDiagnostic(d, "pdf_list_literal") {
			t.Fatal("explicit list hierarchy lost")
		}
		marker := d.Blocks[0].Items[0].Marker
		if len(marker.Blocks) != 1 || len(marker.Blocks[0].Runs) != 1 || marker.Blocks[0].Runs[0].Text != "3." || marker.Blocks[0].Runs[0].Link != href || d.Text() != "3. Body" {
			t.Fatal("literal label/relationship/body oracle changed")
		}
		assertRecoverySF(t, d)
		exportLiteralMarkerQA(t, "annotation", raw, d)
	}
}

func FuzzPDFListNumberingPattern(f *testing.F) {
	f.Add("UpperRoman", "IV)", "V)")
	f.Add("LowerAlpha", "aa.", "bb.")
	f.Add("Decimal", "03)", "04)")
	f.Fuzz(func(t *testing.T, attribute, first, second string) {
		if len(first) > 4096 || len(second) > 4096 || len(attribute) > 32 {
			return
		}
		pattern, ok := pdfListNumberingPattern(attribute, []string{first, second})
		if !ok {
			return
		}
		b := Block{Kind: ListBlock, List: "number", Start: listStartPointer(pattern.start), Numbering: pattern.numbering}
		if validateNumbering(b.Numbering) != nil {
			t.Fatal("invalid numbering inferred")
		}
		for i, want := range []string{first, second} {
			got, err := numberedLabel(b, i)
			if err != nil || got != want {
				t.Fatal("inferred marker changed source label", err)
			}
		}
	})
}

func TestPDFPictorialCounterStaysVisible(t *testing.T) {
	content := markedText(0, 20, 300, "3.") + markedText(1, 50, 300, "Body") + `/Figure << /MCID 2 >> BDC q 10 0 0 10 35 295 cm /X Do Q EMC`
	objects := taggedTestObjects(content,
		`<< /Type /StructTreeRoot /K 8 0 R >>`,
		`<< /Type /StructElem /S /L /P 7 0 R /Pg 3 0 R /A << /O /List /ListNumbering /Decimal >> /K 9 0 R >>`,
		`<< /Type /StructElem /S /LI /P 8 0 R /K [10 0 R 11 0 R] >>`,
		`<< /Type /StructElem /S /Lbl /P 9 0 R /K [0 12 0 R] >>`,
		`<< /Type /StructElem /S /LBody /P 9 0 R /K 1 >>`,
		`<< /Type /StructElem /S /Figure /P 10 0 R /Alt (MARKER_ICON) /K 2 >>`)
	objects[5] = recoveryStream(`/Type /XObject /Subtype /Image /Width 1 /Height 1 /ColorSpace /DeviceRGB /BitsPerComponent 8`, string([]byte{255, 0, 0}))
	raw := recoveryPDF(objects...)
	d := readRecovery(t, raw)
	if len(d.Blocks) != 1 || d.Blocks[0].Kind != ListBlock || len(d.Blocks[0].Items) != 1 || !hasDiagnostic(d, "pdf_list_literal") {
		t.Fatal("pictorial label lost list ownership")
	}
	item := d.Blocks[0].Items[0]
	if len(item.Marker.Blocks) != 2 || item.Marker.Blocks[0].Kind != Paragraph || item.Marker.Blocks[1].Kind != Picture || item.Marker.Blocks[1].Image.Alt != "MARKER_ICON" || paragraphText(item.Marker.Blocks[0]) != "3." || paragraphText(item.Blocks[0]) != "Body" {
		t.Fatal("literal marker/body order or asset lost")
	}
	decoded, err := png.Decode(bytes.NewReader(item.Marker.Blocks[1].Image.Data))
	if err != nil {
		t.Fatal(err)
	}
	red, green, blue, alpha := decoded.At(0, 0).RGBA()
	if red != 65535 || green != 0 || blue != 0 || alpha != 65535 {
		t.Fatal("original red marker pixel changed")
	}
	if d.Text() != "3.\nMARKER_ICON\nBody" {
		t.Fatal("original literal text oracle changed", d.Text())
	}
	assertRecoverySF(t, d)
	exportLiteralMarkerQA(t, "picture", raw, d)
}

func TestPDFNestedCounterKeepsLiteralObservedContext(t *testing.T) {
	parent := Block{Kind: ListBlock, List: "number", Start: listStartPointer(27), Numbering: &ListNumbering{Template: "Appendix %1.", Formats: []string{"upperLetter"}}, Items: []ListItem{{Blocks: []Block{mergeParagraph("PARENT"), {Kind: ListBlock, List: "number", Start: listStartPointer(4), Numbering: &ListNumbering{Level: 1, Template: "%1.%2)", Formats: []string{"upperLetter", "lowerRoman"}, Ancestors: []int{99}}, Items: []ListItem{{Blocks: []Block{mergeParagraph("FIRST")}}, {Blocks: []Block{mergeParagraph("SECOND")}}}}}}}}
	var out bytes.Buffer
	if err := Write(context.Background(), &out, &Document{Blocks: []Block{parent}}, PDF, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	got, err := ReadPDF(context.Background(), &out, ReadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	assertPDFNumberedLabels(t, got, 27, []string{"Appendix AA."})
	children := got.Blocks[0].Items[0].Blocks
	if len(children) != 2 || children[1].Kind != ListBlock || children[1].Numbering == nil || children[1].Numbering.Template != "AA.%1)" {
		t.Fatal("nested displayed context lost or unproved ancestry invented")
	}
	for i, want := range []string{"AA.iv)", "AA.v)"} {
		label, err := numberedLabel(children[1], i)
		if err != nil || label != want {
			t.Fatal("nested label changed", label, err)
		}
	}
	if !hasDiagnostic(got, "pdf_list_numbering_inferred") {
		t.Fatal("observed literal context not diagnosed")
	}
}
