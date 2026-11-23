package manydoc

import (
	"bytes"
	"compress/zlib"
	"context"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
	"github.com/phpdave11/gofpdf"
	"golang.org/x/image/font/gofont/goregular"
)

func TestPDFRecoveredEditableDeviceStyles(t *testing.T) {
	for _, tc := range []struct {
		paint, color, space string
		components          []float64
	}{
		{"", "#000000", "DeviceGray", []float64{0}},
		{"1 0 0 rg", "#FF0000", "DeviceRGB", []float64{1, 0, 0}},
		{"0.5 g", "#808080", "DeviceGray", []float64{.5}},
		{"0 1 1 0 k", "#FF0000", "DeviceCMYK", []float64{0, 1, 1, 0}},
		{"1 0 0 rg 0 0 1 RG 1 Tr", "#0000FF", "DeviceRGB", []float64{0, 0, 1}},
	} {
		t.Run(tc.paint, func(t *testing.T) {
			d := readRecovery(t, recoveryPage(tc.paint+" BT /F 14 Tf 10 100 Td (Style) Tj ET", ""))
			rr := recoveryRuns(d)
			if len(rr) != 1 {
				t.Fatalf("runs %+v", rr)
			}
			r := rr[0]
			if r.FontFamily != "Courier" || r.FontSize != 14 || r.Color != tc.color || r.Source.Font != "Courier" || r.Source.FontFamilyEvidence != "core_font" || r.Source.ColorSpace != tc.space || !reflect.DeepEqual(r.Source.ColorComponents, tc.components) {
				t.Fatalf("style %+v source %+v", r, r.Source)
			}
			if (tc.space == "DeviceCMYK") != hasDiagnostic(d, "pdf_text_color_approximation") {
				t.Fatal("CMYK approximation not disclosed")
			}
			if (tc.paint == "0.5 g") != hasDiagnostic(d, "pdf_text_color_quantized") {
				t.Fatal("quantization not disclosed")
			}
			assertRecoverySF(t, d)
		})
	}
	d := readRecovery(t, recoveryPage("q 2 0 0 3 0 0 cm BT /F 10 Tf 10 50 Td (Scaled) Tj ET Q", ""))
	if recoveryRuns(d)[0].FontSize != 30 || !hasDiagnostic(d, "pdf_text_transform") {
		t.Fatal("physical size/nonuniform scale evidence lost")
	}
}

func TestPDFRecoveredPaintStateAndAliases(t *testing.T) {
	objects := taggedTestObjects("/Alias cs 1 0 0 sc BT /F 12 Tf 10 300 Td (Red) Tj ET q 0 1 0 rg /X Do Q BT /F 12 Tf 10 200 Td (RedAgain) Tj ET", "<< /Type /StructTreeRoot /K [] >>")
	objects[2] = strings.Replace(objects[2], "/Font <<", "/ColorSpace << /Alias /Alias2 /Alias2 /DeviceRGB >> /Font <<", 1)
	objects[5] = recoveryStream("/Type /XObject /Subtype /Form /BBox [0 0 300 400]", "BT /F 12 Tf 10 250 Td (GreenForm) Tj ET")
	d := readRecovery(t, recoveryPDF(objects...))
	rr := recoveryRuns(d)
	if len(rr) != 3 || rr[0].Color != "#FF0000" || rr[1].Color != "#00FF00" || rr[2].Color != "#FF0000" {
		t.Fatalf("paint state %+v", rr)
	}
	assertRecoverySF(t, d)
	objects[2] = strings.Replace(objects[2], "/Alias2 /DeviceRGB", "/Alias2 /Alias", 1)
	if _, err := ReadPDF(context.Background(), bytes.NewReader(recoveryPDF(objects...)), ReadOptions{}); err == nil {
		t.Fatal("color alias cycle accepted")
	}
	for _, paint := range []string{"1 0 rg", "/DeviceGray cs 1 0 sc", "7 cs"} {
		if _, err := ReadPDF(context.Background(), bytes.NewReader(recoveryPage(paint+" BT /F 12 Tf (X) Tj ET", "")), ReadOptions{}); err == nil {
			t.Fatalf("malformed color accepted: %s", paint)
		}
	}
	d = readRecovery(t, recoveryPage("/Pattern cs /P scn BT /F 12 Tf (Unknown) Tj ET", ""))
	if recoveryRuns(d)[0].Color != "" || recoveryRuns(d)[0].Source.ColorSpace != "" || !hasDiagnostic(d, "pdf_text_color") {
		t.Fatal("unresolved paint falsely recovered")
	}
}

func TestPDFRecoveredFontFamilyEvidence(t *testing.T) {
	for _, tc := range []struct{ face, descriptor, want, evidence string }{
		{"ABCDEF+Custom-Bold", "/FontFamily (Editorial Family)", "Editorial Family", "descriptor"},
		{"ABCDEF+Custom-Bold", "", "Custom-Bold", "postscript_fallback"},
		{"Courier-BoldOblique", "", "Courier", "core_font"},
	} {
		objects := taggedTestObjects("BT /F 12 Tf 10 200 Td (Family) Tj ET", "<< /Type /StructTreeRoot /K [] >>")
		objects[3] = "<< /Type /Font /Subtype /Type1 /BaseFont /" + tc.face + " /FontDescriptor << /Type /FontDescriptor /FontName /" + tc.face + " " + tc.descriptor + " >> >>"
		d := readRecovery(t, recoveryPDF(objects...))
		r := recoveryRuns(d)[0]
		if r.FontFamily != tc.want || r.Source.FontFamilyEvidence != tc.evidence || r.Source.Font != tc.face {
			t.Fatalf("font %+v source %+v", r, r.Source)
		}
		if (tc.evidence == "postscript_fallback") != hasDiagnostic(d, "pdf_font_family_inferred") {
			t.Fatal("uncertain grouping not disclosed")
		}
		assertRecoverySF(t, d)
	}
	// The caller's lookup alias is not part of the embedded Go font program.
	d := &Document{Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "Embedded", FontFamily: "Caller Alias", FontSize: 18, Color: "#CC6633"}}}}}
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, PDF, WriteOptions{PDFFonts: []FontFace{{Family: "Caller Alias", Data: goregular.TTF}}}); err != nil {
		t.Fatal(err)
	}
	generated := append([]byte(nil), out.Bytes()...)
	// Independently authored primitive-writer fixture omits FontFamily, keeping
	// the embedded-name fallback oracle independent of manydoc's new descriptor.
	primitive := gofpdf.New("P", "pt", "A4", "")
	primitive.AddUTF8FontFromBytes("Caller Alias", "", append([]byte(nil), goregular.TTF...))
	primitive.AddPage()
	primitive.SetFont("Caller Alias", "", 18)
	primitive.SetTextColor(204, 102, 51)
	primitive.Text(54, 72, "Embedded")
	out.Reset()
	if err := primitive.Output(&out); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		input    []byte
		evidence string
	}{{generated, "descriptor"}, {out.Bytes(), "embedded_name"}} {
		back := readRecovery(t, tc.input)
		r := recoveryRuns(back)[0]
		if r.FontFamily != "Go" || r.Source.FontFamilyEvidence != tc.evidence || r.FontSize != 18 || r.Color != "#CC6633" {
			t.Fatalf("embedded style %+v source %+v", r, r.Source)
		}
		assertRecoverySF(t, back)
	}
}

func TestPDFActualTextRetainsOnlyCommonEditableStyles(t *testing.T) {
	for _, structural := range []bool{false, true} {
		for _, mixed := range []bool{false, true} {
			t.Run(strings.Join([]string{map[bool]string{true: "structural", false: "stream"}[structural], map[bool]string{true: "mixed", false: "same"}[mixed]}, "/"), func(t *testing.T) {
				second := ""
				if mixed {
					second = "0 0 1 rg /F 16 Tf"
				}
				content := "1 0 0 rg BT /F 12 Tf 10 250 Td /P << /MCID 0 >> BDC (A) Tj " + second + " (B) Tj EMC ET"
				var data []byte
				if structural {
					data = taggedTestPDF(content, "<< /Type /StructTreeRoot /K 8 0 R >>", "<< /Type /StructElem /S /P /P 7 0 R /Pg 3 0 R /ActualText (Replacement) /K 0 >>")
				} else {
					data = recoveryPage(strings.Replace(content, "/MCID 0", "/ActualText (Replacement)", 1), "")
				}
				d := readRecovery(t, data)
				rr := recoveryRuns(d)
				if len(rr) != 1 || rr[0].Text != "Replacement" {
					t.Fatalf("replacement %+v", rr)
				}
				r := rr[0]
				if r.FontFamily != "Courier" {
					t.Fatal("common family lost")
				}
				if mixed {
					if r.FontSize != 0 || r.Color != "" || r.Source.ColorSpace != "" || !hasDiagnostic(d, "pdf_actualtext_style") {
						t.Fatalf("ambiguous style %+v source %+v", r, r.Source)
					}
				} else {
					if r.FontSize != 12 || r.Color != "#FF0000" || r.Source.ColorSpace != "DeviceRGB" || !reflect.DeepEqual(r.Source.ColorComponents, []float64{1, 0, 0}) {
						t.Fatalf("common style %+v source %+v", r, r.Source)
					}
				}
				assertRecoverySF(t, d)
			})
		}
	}
}

func TestPDFSourceStyleValidationAndBoundedMetadata(t *testing.T) {
	valid := SourceInfo{Format: PDF, Structure: "unstructured", Evidence: "test", FontFamilyEvidence: "descriptor", ColorSpace: "DeviceRGB", ColorComponents: []float64{1, 0, .5}}
	for _, bad := range []SourceInfo{
		{FontFamilyEvidence: "guessed"}, {ColorSpace: "DeviceRGB", ColorComponents: []float64{1, 0}}, {ColorSpace: "DeviceGray", ColorComponents: []float64{math.NaN()}}, {ColorSpace: "DeviceGray", ColorComponents: []float64{2}}, {ColorComponents: []float64{0}},
	} {
		bad.Format = PDF
		bad.Structure = "unstructured"
		bad.Evidence = "test"
		if bad.validate(nil) == nil {
			t.Fatalf("invalid evidence accepted %+v", bad)
		}
	}
	if err := valid.validate(nil); err != nil {
		t.Fatal(err)
	}
	var compressed bytes.Buffer
	z := zlib.NewWriter(&compressed)
	_, _ = z.Write(goregular.TTF)
	_ = z.Close()
	s := &types.StreamDict{Raw: compressed.Bytes(), FilterPipeline: []types.PDFFilter{{Name: "FlateDecode"}}}
	decoded, err := boundedFontMetadata(context.Background(), s)
	if err != nil || !bytes.Equal(decoded, goregular.TTF) {
		t.Fatalf("metadata decode %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := boundedFontMetadata(ctx, s); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation ignored")
	}
	s.FilterPipeline[0].Name = "DCTDecode"
	if _, err := boundedFontMetadata(context.Background(), s); err == nil {
		t.Fatal("unsupported font filter accepted")
	}
	compressed.Reset()
	z = zlib.NewWriter(&compressed)
	_, _ = z.Write(make([]byte, maxFontMetadataBytes+1))
	_ = z.Close()
	s.Raw = compressed.Bytes()
	s.FilterPipeline[0].Name = "FlateDecode"
	if _, err := boundedFontMetadata(context.Background(), s); err == nil {
		t.Fatal("expanded metadata exceeds limit")
	}
}

func TestPDFRecoveredStyleSeparatorsAndMetadataFallback(t *testing.T) {
	for _, mixed := range []bool{false, true} {
		paint := ""
		if mixed {
			paint = "0 0 1 rg /F 16 Tf"
		}
		d := readRecovery(t, recoveryPage("1 0 0 rg BT /F 12 Tf 10 200 Td (A) Tj "+paint+" 30 0 Td (B) Tj ET", ""))
		rr := recoveryRuns(d)
		if len(rr) != 3 || rr[1].Text != " " || rr[1].Source.Precision != "synthetic_separator" {
			t.Fatalf("gap %+v", rr)
		}
		gap := rr[1]
		if gap.FontFamily != "Courier" || gap.Source.ColorSpace != "" || gap.Source.FontFamilyEvidence != "" {
			t.Fatalf("synthetic source %+v", gap)
		}
		if mixed {
			if gap.FontSize != 0 || gap.Color != "" {
				t.Fatal("invented mixed gap style")
			}
		} else if gap.FontSize != 12 || gap.Color != "#FF0000" {
			t.Fatal("common gap style lost")
		}
		assertRecoverySF(t, d)
	}
	objects := taggedTestObjects("BT /F 12 Tf 10 200 Td (Retained) Tj ET", "<< /Type /StructTreeRoot /K [] >>")
	objects[3] = "<< /Type /Font /Subtype /Type1 /BaseFont /CustomFace /FontDescriptor << /Type /FontDescriptor /FontName /CustomFace /FontFile2 8 0 R >> >>"
	objects = append(objects, recoveryStream("", "not a font"))
	d := readRecovery(t, recoveryPDF(objects...))
	r := recoveryRuns(d)[0]
	if r.Text != "Retained" || r.FontFamily != "CustomFace" || r.Source.FontFamilyEvidence != "postscript_fallback" || !hasDiagnostic(d, "pdf_font_family") {
		t.Fatal("invalid embedded metadata lost text or fabricated family")
	}
}

func FuzzPDFEditableStyles(f *testing.F) {
	f.Add(12.0, 1.0, 0.0, 0.0, false)
	f.Add(18.5, .2, .4, .8, true)
	f.Fuzz(func(t *testing.T, size, red, green, blue float64, actual bool) {
		if !finite(size) || !finite(red) || !finite(green) || !finite(blue) {
			t.Skip()
		}
		size = math.Mod(math.Abs(size), 40) + 1
		red = math.Mod(math.Abs(red), 1)
		green = math.Mod(math.Abs(green), 1)
		blue = math.Mod(math.Abs(blue), 1)
		content := fmt.Sprintf("%.8f %.8f %.8f rg BT /F %.8f Tf 10 200 Td (Style) Tj ET", red, green, blue, size)
		if actual {
			content = strings.Replace(content, "(Style) Tj", "/Span << /ActualText (Replaced) >> BDC (Style) Tj EMC", 1)
		}
		d := readRecovery(t, recoveryPage(content, ""))
		r := recoveryRuns(d)[0]
		if r.FontFamily != "Courier" || math.Abs(r.FontSize-size) > 1e-7 || len(r.Color) != 7 || r.Source.ColorSpace != "DeviceRGB" {
			t.Fatalf("style %+v", r)
		}
		assertRecoverySF(t, d)
	})
}

func TestPDFActualTextExactDecodedAlignmentKeepsMixedStyles(t *testing.T) {
	content := "BT /F 12 Tf 1 0 0 rg 10 200 Td /Span << /ActualText (AB) >> BDC (A) Tj 0 0 1 rg /F 16 Tf (B) Tj EMC ET"
	d := readRecovery(t, recoveryPage(content, ""))
	runs := recoveryRuns(d)
	if len(runs) != 2 || runs[0].Text != "A" || runs[1].Text != "B" || runs[0].Color != "#FF0000" || runs[1].Color != "#0000FF" || runs[0].FontSize != 12 || runs[1].FontSize != 16 {
		t.Fatalf("exact ActualText alignment %+v", runs)
	}
	for _, r := range runs {
		if r.Source.Evidence != "pdf_actualtext_aligned" {
			t.Fatal("alignment not distinguished in provenance")
		}
	}
	if hasDiagnostic(d, "pdf_actualtext_style") {
		t.Fatal("exact style alignment diagnosed as ambiguous")
	}
	assertRecoverySF(t, d)
}
