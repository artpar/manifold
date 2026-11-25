package manifold

import (
	"bytes"
	"compress/zlib"
	"context"
	"encoding/ascii85"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
	"image"
	"image/color"
	"image/jpeg"
	"math"
	"reflect"
	"strings"
	"testing"
)

// Hand-authored object streams are a structural oracle independent of the writer.
func recoveryPDF(objects ...string) []byte {
	var out bytes.Buffer
	out.WriteString("%PDF-1.7\n")
	offsets := []int{0}
	for i, o := range objects {
		offsets = append(offsets, out.Len())
		fmt.Fprintf(&out, "%d 0 obj\n%s\nendobj\n", i+1, o)
	}
	xref := out.Len()
	fmt.Fprintf(&out, "xref\n0 %d\n0000000000 65535 f \n", len(offsets))
	for _, offset := range offsets[1:] {
		fmt.Fprintf(&out, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&out, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offsets), xref)
	return out.Bytes()
}
func recoveryStream(dict, content string) string {
	return fmt.Sprintf("<< %s /Length %d >>\nstream\n%s\nendstream", dict, len(content), content)
}
func recoveryPage(content, attrs string, extra ...string) []byte {
	objects := []string{"<< /Type /Catalog /Pages 2 0 R >>", "<< /Type /Pages /Kids [3 0 R] /Count 1 >>", "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 400] /Resources << /Font << /F 4 0 R >> /XObject << /X 6 0 R /Y 7 0 R >> >> /Contents 5 0 R " + attrs + " >>", "<< /Type /Font /Subtype /Type1 /BaseFont /Courier >>", recoveryStream("", content)}
	return recoveryPDF(append(objects, extra...)...)
}
func readRecovery(t *testing.T, data []byte) *Document {
	t.Helper()
	// These primitive extraction oracles inspect paint order and exact runs;
	// geometric structure has separate default-mode integration tests.
	d, e := ReadPDF(context.Background(), bytes.NewReader(data), ReadOptions{PDFSkipInference: true})
	if e != nil {
		t.Fatal(e)
	}
	return d
}
func closePoint(t *testing.T, actual Point, x, y float64) {
	t.Helper()
	if math.Abs(actual.X-x) > 1e-7 || math.Abs(actual.Y-y) > 1e-7 {
		t.Fatalf("point %+v, want (%g,%g)", actual, x, y)
	}
}
func recoveryRuns(d *Document) []Run {
	var runs []Run
	for _, b := range d.Blocks {
		runs = append(runs, b.Runs...)
	}
	return runs
}
func assertRecoverySF(t *testing.T, d *Document) {
	t.Helper()
	sf, e := ToStructured(d)
	if e != nil {
		t.Fatal(e)
	}
	var out bytes.Buffer
	if e = WriteStructured(context.Background(), &out, sf); e != nil {
		t.Fatal(e)
	}
	sf, e = ReadStructured(context.Background(), &out)
	if e != nil {
		t.Fatal(e)
	}
	got, e := FromStructured(sf)
	if e != nil {
		t.Fatal(e)
	}
	a, _ := json.Marshal(d)
	b, _ := json.Marshal(got)
	if !bytes.Equal(a, b) {
		t.Fatalf("SF changed recovery semantics\n%s\n%s", a, b)
	}
}

func TestPDFPositionedTextTransforms(t *testing.T) {
	d := readRecovery(t, recoveryPage("q 2 0 0 3 10 20 cm BT /F 10 Tf 50 Tz 2 Ts 1 0 0 1 5 7 Tm (AB) Tj (C) Tj ET Q", ""))
	runs := recoveryRuns(d)
	if len(runs) != 2 || runs[0].Text != "AB" || runs[1].Text != "C" {
		t.Fatalf("runs: %+v", runs)
	}
	a, b := runs[0].Source, runs[1].Source
	closePoint(t, a.Baseline[0], 20, 353)
	closePoint(t, a.Baseline[1], 32, 353)
	closePoint(t, b.Baseline[0], 32, 353)
	closePoint(t, b.Baseline[1], 38, 353)
	if *a.Matrix != (Matrix{10, 0, 0, -30, 20, 353}) || a.FontSize != 30 || a.Precision != "font_metrics" || a.Structure != "unstructured" || d.Blocks[0].Source.Structure != "inferred" {
		t.Fatalf("source %+v", a)
	}
	if len(d.Pages) != 1 || d.Pages[0].Width != 300 || d.Pages[0].Height != 400 {
		t.Fatalf("pages %+v", d.Pages)
	}
	assertRecoverySF(t, d)
}

func TestPDFCoreFontCanonicalAliasAdvances(t *testing.T) {
	// Published AFM widths in glyph-space units: Helvetica space 278,
	// hyphen 333, bullet 350; Courier uses 600 for each. Alias slots in
	// WinAnsi must resolve to these glyphs, not a missing-metric fallback.
	for _, tc := range []struct {
		name string
		want float64
	}{{"Helvetica", 15.72}, {"Helvetica-Bold", 15.72}, {"Courier", 30}} {
		t.Run(tc.name, func(t *testing.T) {
			objects := inferenceObjects("BT /F 10 Tf 20 300 Td <20A02DAD95> Tj ET")
			objects[3] = "<< /Type /Font /Subtype /Type1 /BaseFont /" + tc.name + " /Encoding /WinAnsiEncoding >>"
			d := readRecovery(t, recoveryPDF(objects...))
			run := d.Blocks[0].Runs[0]
			// Two spaces, two hyphens and one bullet.
			closePoint(t, run.Source.Baseline[1], 20+tc.want, 500)
			if run.Source.Precision != "font_metrics" {
				t.Fatal("exact core metrics not used")
			}
		})
	}
}
func TestPDFPageCoordinateSystems(t *testing.T) {
	tests := []struct {
		rotate     int
		start, end Point
		w, h       float64
	}{{0, Point{20, 540}, Point{32, 540}, 400, 600}, {90, Point{60, 20}, Point{60, 32}, 600, 400}, {180, Point{380, 60}, Point{368, 60}, 400, 600}, {270, Point{540, 380}, Point{540, 368}, 600, 400}}
	for _, tc := range tests {
		t.Run(fmt.Sprint(tc.rotate), func(t *testing.T) {
			d := readRecovery(t, recoveryPage("BT /F 10 Tf 1 0 0 1 30 50 Tm (A) Tj ET", fmt.Sprintf("/CropBox [20 20 220 320] /Rotate %d /UserUnit 2", tc.rotate)))
			s := recoveryRuns(d)[0].Source
			closePoint(t, s.Baseline[0], tc.start.X, tc.start.Y)
			closePoint(t, s.Baseline[1], tc.end.X, tc.end.Y)
			if d.Pages[0].Width != tc.w || d.Pages[0].Height != tc.h {
				t.Fatalf("page %+v", d.Pages[0])
			}
			assertRecoverySF(t, d)
		})
	}
}
func TestPDFTextMatrixStateAndQuotes(t *testing.T) {
	d := readRecovery(t, recoveryPage("BT /F 10 Tf 10 100 Td 12 TL (A) Tj q 2 0 0 2 0 0 cm (B) Tj Q (C) Tj (D) ' 0 0 (E) \" ET", ""))
	rr := recoveryRuns(d)
	var positions = map[string]Point{"A": {10, 300}, "B": {32, 200}, "C": {22, 300}, "D": {10, 312}, "E": {10, 324}}
	for _, r := range rr {
		want, ok := positions[strings.TrimSpace(r.Text)]
		if !ok {
			t.Fatalf("unexpected run %q", r.Text)
		}
		closePoint(t, r.Source.Baseline[0], want.X, want.Y)
		delete(positions, strings.TrimSpace(r.Text))
	}
	if len(positions) > 0 {
		t.Fatalf("missing %v", positions)
	}
}
func TestPDFFormAndAppearancePlacement(t *testing.T) {
	form := recoveryStream("/Type /XObject /Subtype /Form /BBox [0 0 20 20] /Matrix [2 0 0 2 3 4]", "BT /F 10 Tf 1 0 0 1 5 6 Tm (X) Tj ET")
	d := readRecovery(t, recoveryPage("q 1 0 0 1 10 20 cm /X Do Q", "", form))
	closePoint(t, recoveryRuns(d)[0].Source.Baseline[0], 23, 364)
	d = readRecovery(t, recoveryPage("", "/Annots [7 0 R]", form, "<< /Type /Annot /Subtype /Widget /Rect [100 200 200 300] /AP << /N 6 0 R >> >>"))
	closePoint(t, recoveryRuns(d)[0].Source.Baseline[0], 125, 170)
	assertRecoverySF(t, d)
}
func TestPDFCompositeMetricsAndLigatureSpacing(t *testing.T) {
	cmap := "begincmap 1 begincodespacerange <0000> <ffff> endcodespacerange 2 beginbfchar <0001> <00660069> <0002> <0058> endbfchar endcmap"
	objects := []string{"<< /Type /Catalog /Pages 2 0 R >>", "<< /Type /Pages /Kids [3 0 R] /Count 1 >>", "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 400] /Resources << /Font << /F 4 0 R >> >> /Contents 5 0 R >>", "<< /Type /Font /Subtype /Type0 /BaseFont /Demo /Encoding /Identity-H /DescendantFonts [6 0 R] /ToUnicode 7 0 R >>", recoveryStream("", "BT /F 10 Tf 2 Tc 1 0 0 1 10 100 Tm <00010002> Tj <0003> Tj ET"), "<< /Type /Font /Subtype /CIDFontType2 /BaseFont /Demo /CIDSystemInfo << /Registry (Adobe) /Ordering (Identity) /Supplement 0 >> /DW 800 /W [1 [400] 2 2 600] /FontDescriptor 8 0 R >>", recoveryStream("", cmap), "<< /Type /FontDescriptor /FontName /Demo /Ascent 900 /Descent -100 >>"}
	// Undefined Unicode codes reject, regardless of available width defaults.
	if _, e := ReadPDF(context.Background(), bytes.NewReader(recoveryPDF(objects...)), ReadOptions{}); e == nil {
		t.Fatal("undefined Unicode code accepted")
	}
	objects[4] = recoveryStream("", "BT /F 10 Tf 2 Tc 1 0 0 1 10 100 Tm <00010002> Tj <0001> Tj ET")
	d := readRecovery(t, recoveryPDF(objects...))
	rr := recoveryRuns(d)
	if rr[0].Text != "fiX" {
		t.Fatalf("ligature %q", rr[0].Text)
	}
	closePoint(t, rr[0].Source.Baseline[1], 24, 300)
	closePoint(t, rr[1].Source.Baseline[0], 24, 300)
	// The first encoded glyph maps to two Unicode runes; Tc is applied once.
	assertRecoverySF(t, d)
}
func TestPDFMarkedContentAndActualText(t *testing.T) {
	d := readRecovery(t, recoveryPage("BT /F 10 Tf 10 100 Td /P << /MCID 5 >> BDC /Span << /ActualText <FEFF00660069> >> BDC (X) Tj (Y) Tj EMC EMC ET", ""))
	rr := recoveryRuns(d)
	if len(rr) != 1 || rr[0].Text != "fi" || rr[0].Source.MCID == nil || *rr[0].Source.MCID != 5 || rr[0].Source.Tag != "Span" || rr[0].Source.Evidence != "pdf_actualtext" || len(rr[0].Source.Regions) != 2 {
		t.Fatalf("ActualText %+v", rr)
	}
	closePoint(t, rr[0].Source.Regions[1].Quad[0], 16, 291.95)
	assertRecoverySF(t, d)
	for _, content := range []string{"EMC", "/P BMC", "q", "Q", "/P << /MCID -1 >> BDC EMC"} {
		if _, e := ReadPDF(context.Background(), bytes.NewReader(recoveryPage(content, "")), ReadOptions{}); e == nil {
			t.Fatalf("accepted unbalanced/invalid %q", content)
		}
	}
}
func TestPDFRasterAssetsAndMasks(t *testing.T) {
	raw := string([]byte{255, 0, 0, 0, 255, 0})
	imageObject := recoveryStream("/Type /XObject /Subtype /Image /Width 2 /Height 1 /BitsPerComponent 8 /ColorSpace /DeviceRGB /SMask 7 0 R", raw)
	mask := recoveryStream("/Type /XObject /Subtype /Image /Width 2 /Height 1 /BitsPerComponent 8 /ColorSpace /DeviceGray", string([]byte{255, 0}))
	d := readRecovery(t, recoveryPage("q 40 0 0 20 10 30 cm /X Do Q", "", imageObject, mask))
	b := d.Blocks[0]
	if b.Kind != Picture || b.Image.Width != 40 || b.Image.Height != 20 {
		t.Fatalf("picture %+v", b)
	}
	im, _, e := image.Decode(bytes.NewReader(b.Image.Data))
	if e != nil {
		t.Fatal(e)
	}
	if got := color.NRGBAModel.Convert(im.At(0, 0)); got != (color.NRGBA{255, 0, 0, 255}) {
		t.Fatalf("first pixel %v", got)
	}
	_, _, _, a := im.At(1, 0).RGBA()
	if a != 0 {
		t.Fatalf("mask alpha %d", a)
	}
	closePoint(t, b.Source.Quad[0], 10, 350)
	closePoint(t, b.Source.Quad[2], 50, 370)
	assertRecoverySF(t, d)
	stencil := recoveryStream("/Type /XObject /Subtype /Image /Width 2 /Height 1 /ImageMask true /BitsPerComponent 1", string([]byte{0x40}))
	d = readRecovery(t, recoveryPage("0 0 1 rg q 40 0 0 20 10 30 cm /X Do Q", "", stencil))
	im, _, e = image.Decode(bytes.NewReader(d.Blocks[0].Image.Data))
	if e != nil {
		t.Fatal(e)
	}
	if got := color.NRGBAModel.Convert(im.At(0, 0)); got != (color.NRGBA{0, 0, 255, 255}) {
		t.Fatalf("stencil %v", got)
	}
	_, _, _, a = im.At(1, 0).RGBA()
	if a != 0 {
		t.Fatal("unpainted stencil pixel opaque")
	}
}
func TestPDFSourceValidationAndLossGate(t *testing.T) {
	d := readRecovery(t, recoveryPage("BT /F 10 Tf 10 100 Td (A) Tj ET", ""))
	d.Diagnostics = nil
	for _, f := range []Format{PDF, DOCX} {
		if e := Write(context.Background(), &bytes.Buffer{}, d, f, WriteOptions{}); !errors.Is(e, ErrLossy) {
			t.Fatalf("source provenance bypassed loss acknowledgment: %v", e)
		}
	}
	original := d.Pages[0]
	d.Pages[0].Width++
	if e := d.Validate(); e == nil {
		t.Fatal("inconsistent page geometry accepted")
	}
	d.Pages[0] = original
	sf, e := ToStructured(d)
	if e != nil {
		t.Fatal(e)
	}
	s := sf.Content[0].Content[0].Marks[0].Attrs
	var attrs map[string]any
	_ = json.Unmarshal(s, &attrs)
	attrs["unexpected"] = 1
	sf.Content[0].Content[0].Marks[0].Attrs, _ = json.Marshal(attrs)
	if _, e = FromStructured(sf); e == nil {
		t.Fatal("unknown provenance field accepted")
	}
	if reflect.DeepEqual(s, nil) {
		t.Fatal("source mark missing")
	}
}

func TestPDFInlineImageBoundaries(t *testing.T) {
	raw := []byte{'E', 'I', ' ', 0, 255, 1}
	var compressed bytes.Buffer
	z := zlib.NewWriter(&compressed)
	_, _ = z.Write(raw)
	_ = z.Close()
	a85 := make([]byte, ascii85.MaxEncodedLen(len(raw)))
	n85 := ascii85.Encode(a85, raw)
	a85 = append(a85[:n85], []byte("~>")...)
	source := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	source.SetNRGBA(0, 0, color.NRGBA{255, 0, 0, 255})
	source.SetNRGBA(1, 0, color.NRGBA{0, 0, 255, 255})
	var jpg bytes.Buffer
	if e := jpeg.Encode(&jpg, source, nil); e != nil {
		t.Fatal(e)
	}
	// Embedded EI and EOI bytes in a JPEG comment must not terminate the image.
	jpegData := append([]byte{}, jpg.Bytes()[:2]...)
	jpegData = append(jpegData, 255, 254, 0, 9, ' ', 'E', 'I', ' ', 255, 217, 0)
	jpegData = append(jpegData, jpg.Bytes()[2:]...)
	for _, tc := range []struct {
		name, filter string
		data         []byte
	}{{"raw", "", raw}, {"flate", "/F /Fl", compressed.Bytes()}, {"hex", "/F /AHx", []byte(hex.EncodeToString(raw) + ">")}, {"ascii85", "/F /A85", a85}, {"rle", "/F /RL", append(append([]byte{5}, raw...), 128)}, {"jpeg", "/F /DCT", jpegData}} {
		t.Run(tc.name, func(t *testing.T) {
			content := "q 40 0 0 20 10 30 cm BI /W 2 /H 1 /BPC 8 /CS /RGB " + tc.filter + " ID " + string(tc.data) + " EI Q BT /F 10 Tf 10 100 Td (AFTER) Tj ET"
			d := readRecovery(t, recoveryPage(content, ""))
			if len(d.Blocks) != 2 || d.Blocks[0].Kind != Picture || d.Blocks[1].Runs[0].Text != "AFTER" {
				t.Fatalf("inline recovery %+v", d.Blocks)
			}
			if tc.name != "jpeg" {
				im, _, e := image.Decode(bytes.NewReader(d.Blocks[0].Image.Data))
				if e != nil {
					t.Fatal(e)
				}
				if got := color.NRGBAModel.Convert(im.At(0, 0)); got != (color.NRGBA{'E', 'I', ' ', 255}) {
					t.Fatalf("raw EI pixel changed: %v", got)
				}
			}
			assertRecoverySF(t, d)
		})
	}
	for _, content := range []string{"BI /W 2 /H 1 /BPC 8 /CS /RGB ID abc EI", "BI /W 1 /H 1 /BPC 8 /CS /G /F /RL ID \x00\x80 EI", "BI /W 1 /H 1 /BPC 8 /CS /G ID Z EX"} {
		if _, e := ReadPDF(context.Background(), bytes.NewReader(recoveryPage(content, "")), ReadOptions{}); e == nil {
			t.Fatal("malformed inline image accepted")
		}
	}
}

func TestPDFActualTextStringEncoding(t *testing.T) {
	for _, v := range []any{types.StringLiteral(`\200\223\240\030`), types.HexLiteral("8093a018")} {
		got, e := pdfTextString(v)
		if e != nil || got != "•ﬁ€˘" {
			t.Fatalf("PDFDocEncoding %q %v", got, e)
		}
	}
	got, e := pdfTextString(types.HexLiteral("5c323030"))
	if e != nil || got != `\200` {
		t.Fatalf("hex bytes unescaped a second time: %q %v", got, e)
	}
	for _, v := range []any{types.HexLiteral("feffd800"), types.HexLiteral("7f"), types.HexLiteral("efbbbf80")} {
		if _, e = pdfTextString(v); e == nil {
			t.Fatal("invalid text string accepted")
		}
	}
}
