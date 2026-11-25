package engine

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/validate"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func cidWidthPDF(widths string, extra ...string) []byte {
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>", "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 400] /Resources << /Font << /F 4 0 R >> >> /Contents 5 0 R >>",
		"<< /Type /Font /Subtype /Type0 /BaseFont /Demo /Encoding /Identity-H /DescendantFonts [6 0 R] /ToUnicode 7 0 R >>",
		recoveryStream("", "BT /F 10 Tf 1 0 0 1 10 300 Tm <000100020003ffff> Tj <0001> Tj ET"),
		"<< /Type /Font /Subtype /CIDFontType2 /BaseFont /Demo /CIDSystemInfo << /Registry (Adobe) /Ordering (Identity) /Supplement 0 >> /DW 900 /W [" + widths + "] /FontDescriptor 8 0 R >>",
		recoveryStream("", "1 begincodespacerange <0000> <ffff> endcodespacerange 4 beginbfchar <0001> <0041> <0002> <0042> <0003> <0043> <ffff> <005A> endbfchar"),
		"<< /Type /FontDescriptor /FontName /Demo /Ascent 900 /Descent -100 >>",
	}
	return recoveryPDF(append(objects, extra...)...)
}

func TestPDFCIDWidthOverridesAndSources(t *testing.T) {
	for i, w := range []string{
		"0 65535 500 1 [400 600] 3 3 800 65535 [1000]",
		strings.Repeat("0 65535 500 ", 128) + "1 [400 600] 3 3 800 65535 [1000]",
		"0 65535 500 1 9 0 R 3 3 800 65535 [1000]",
		"0 [500 400 600 800] 4 65534 500 65535 [1000]",
	} {
		raw := cidWidthPDF(w, "[400 600]")
		writeCIDWidthQA(t, fmt.Sprintf("case-%d", i), raw)
		d := readRecovery(t, raw)
		runs := recoveryRuns(d)
		if len(runs) != 2 || runs[0].Text != "ABCZ" || runs[1].Text != "A" {
			t.Fatal("CID content lost", d.Text())
		}
		precision := "estimated_advance"
		if i == 3 {
			precision = "font_metrics"
		}
		for j, r := range runs {
			if r.Source == nil || r.Source.Precision != precision || r.Source.Stream != "page:1" || r.Source.Page != 1 {
				t.Fatal("CID source lost", j, r)
			}
		}
		closePoint(t, runs[0].Source.Baseline[0], 10, 100)
		closePoint(t, runs[0].Source.Baseline[1], 38, 100)
		closePoint(t, runs[1].Source.Baseline[0], 38, 100)
		closePoint(t, runs[1].Source.Baseline[1], 42, 100)
		if hasDiagnostic(d, "pdf_cid_width_overlap") != (i != 3) {
			t.Fatal("overlap uncertainty was not explicit", i, d.Diagnostics)
		}
		assertRecoverySF(t, d)
	}
}

func writeCIDWidthQA(t testing.TB, name string, data []byte) {
	t.Helper()
	if dir := os.Getenv("MANYDOC_PDF_CID_QA"); dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name+".pdf"), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestPDFCIDSharedWidthArraysAndFullInventory(t *testing.T) {
	// One indirect numeric array is reused128 times. Source validation and final
	// CID ownership must not depend on whether entries are direct or shared.
	widths := strings.Repeat("0 9 0 R ", 128) + "1 [400 600] 3 3 800 65535 [1000]"
	raw := cidWidthPDF(widths, "["+strings.Repeat("500 ", 65536)+"]")
	writeCIDWidthQA(t, "shared-array", raw)
	p, err := pdfcpu.ReadWithContext(context.Background(), bytes.NewReader(raw), &model.Configuration{Reader15: true, ValidationMode: model.ValidationRelaxed, Offline: true})
	if err != nil {
		t.Fatal(err)
	}
	font, err := p.DereferenceDict(*types.NewIndirectRef(4, 0))
	if err != nil {
		t.Fatal(err)
	}
	f := &pdfFont{widths: map[uint32]float64{}}
	r := &pdfReader{ctx: context.Background(), pdf: p, doc: &Document{}}
	if err = r.fontMetrics(f, font); err != nil {
		t.Fatal(err)
	}
	if len(f.widths) != 65536 {
		t.Fatal("CID inventory lost", len(f.widths))
	}
	for code := uint32(0); code <= 65535; code++ {
		want := 500.0
		switch code {
		case 1:
			want = 400
		case 2:
			want = 600
		case 3:
			want = 800
		case 65535:
			want = 1000
		}
		if f.widths[code] != want {
			t.Fatal("ordered CID override changed", code, f.widths[code], want)
		}
	}
	d := readRecovery(t, raw)
	runs := recoveryRuns(d)
	if len(runs) != 2 || runs[0].Text != "ABCZ" || runs[1].Text != "A" {
		t.Fatal("shared array changed public recovery", d.Text())
	}
	closePoint(t, runs[1].Source.Baseline[1], 42, 100)
	assertRecoverySF(t, d)
}

type cidWidthCancelContext struct {
	context.Context
	cancel context.CancelFunc
	widths map[uint32]float64
}

func (c *cidWidthCancelContext) Err() error {
	if len(c.widths) >= 1024 {
		c.cancel()
	}
	return c.Context.Err()
}
func TestPDFCIDWidthExpansionCancellation(t *testing.T) {
	p, err := pdfcpu.ReadWithContext(context.Background(), bytes.NewReader(cidWidthPDF("0 65535 500")), &model.Configuration{Reader15: true, ValidationMode: model.ValidationRelaxed, Offline: true})
	if err != nil {
		t.Fatal(err)
	}
	font, err := p.DereferenceDict(*types.NewIndirectRef(4, 0))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f := &pdfFont{widths: map[uint32]float64{}}
	c := &cidWidthCancelContext{Context: ctx, cancel: cancel, widths: f.widths}
	r := &pdfReader{ctx: c, pdf: p, doc: &Document{}}
	if err = r.fontMetrics(f, font); !errors.Is(err, context.Canceled) {
		t.Fatalf("CID expansion ignored deterministic cancellation: %v", err)
	}
	if len(f.widths) >= 65536 {
		t.Fatal("cancellation checked only after full CID expansion")
	}
}

func BenchmarkPDFCIDRepeatedRanges(b *testing.B) {
	raw := cidWidthPDF(strings.Repeat("0 65535 500 ", 1000) + "1 [400 600] 3 3 800 65535 [1000]")
	b.ReportAllocs()
	for b.Loop() {
		if _, err := ReadPDF(context.Background(), bytes.NewReader(raw), ReadOptions{PDFSkipInference: true}); err != nil {
			b.Fatal(err)
		}
	}
}

func TestPDFCIDWidthsMalformed(t *testing.T) {
	for _, w := range []string{"-1 2 500", "1 65536 500", "65535 [500 600]", "1 2", "1 [500 /Bad] 1 [400 600]", "1 9 0 R 1 [400 600]"} {
		d, err := ReadPDF(context.Background(), bytes.NewReader(cidWidthPDF(w, "[500 /Bad]")), ReadOptions{})
		if err == nil || d != nil {
			t.Fatal("invalid overwritten/source width accepted", w)
		}
	}
}

func FuzzPDFCIDWidthOverrides(f *testing.F) {
	f.Add(uint16(1), uint16(3), int16(500), uint8(2))
	f.Add(uint16(0), uint16(65535), int16(0), uint8(8))
	f.Fuzz(func(t *testing.T, a, b uint16, width int16, repeats uint8) {
		if a > b {
			a, b = b, a
		}
		raw := strings.Repeat(fmt.Sprintf("%d %d %d ", a, b, width), int(repeats)%16+1) + "1 [400 600] 3 3 800 65535 [1000]"
		d := readRecovery(t, cidWidthPDF(raw))
		runs := recoveryRuns(d)
		if len(runs) != 2 || runs[0].Text != "ABCZ" {
			t.Fatal("source content changed")
		}
		closePoint(t, runs[0].Source.Baseline[1], 38, 100)
		closePoint(t, runs[1].Source.Baseline[1], 42, 100)
		assertRecoverySF(t, d)
	})
}
func TestPDFPositionedGlyphCombiningTransport(t *testing.T) {
	data, err := os.ReadFile("testdata/assets/NotoSans-Regular.ttf")
	if err != nil {
		t.Fatal(err)
	}
	text := "B\u0315 Z\u035c"
	d := &Document{Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: text, FontFamily: "Noto", FontSize: 22, Color: "#006633"}}}}}
	var b bytes.Buffer
	if err = Write(context.Background(), &b, d, PDF, WriteOptions{PDFFonts: []FontFace{{Family: "Noto", Data: data}}}); err != nil {
		t.Fatal(err)
	}
	back, err := ReadPDF(context.Background(), bytes.NewReader(b.Bytes()), ReadOptions{PDFSkipInference: true})
	if err != nil {
		t.Fatal(err)
	}
	if back.Text() != text {
		t.Fatalf("glyph text %q want %q", back.Text(), text)
	}
	rr := recoveryRuns(back)
	found := false
	for _, r := range rr {
		if strings.Contains(r.Text, "\u0315") {
			found = true
			if r.FontFamily != "Noto Sans" || r.FontSize != 22 || r.Color != "#006633" || r.Source.Precision != "replacement_span_bounds" {
				t.Fatalf("glyph style %+v source %+v", r, r.Source)
			}
		}
	}
	if !found {
		t.Fatal("positioned combining source missing")
	}
	assertRecoverySF(t, back)
}

func TestPDFGlyphSupplementaryTransport(t *testing.T) {
	data, err := os.ReadFile("testdata/assets/DejaVuSans.ttf")
	if err != nil {
		t.Fatal(err)
	}
	d := &Document{Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "A😀B", FontFamily: "DejaVu", FontSize: 18}}}}}
	var out bytes.Buffer
	if err = Write(context.Background(), &out, d, PDF, WriteOptions{PDFFonts: []FontFace{{Family: "DejaVu", Data: data}}}); err != nil {
		t.Fatal(err)
	}
	back, err := ReadPDF(context.Background(), bytes.NewReader(out.Bytes()), ReadOptions{PDFSkipInference: true})
	if err != nil {
		t.Fatal(err)
	}
	if back.Text() != "A😀B" {
		t.Fatalf("supplementary text changed %q", back.Text())
	}
	assertRecoverySF(t, back)
}

func TestPDFGlyphDefaultInferenceKeepsInlineOrder(t *testing.T) {
	data, err := os.ReadFile("testdata/assets/DejaVuSans.ttf")
	if err != nil {
		t.Fatal(err)
	}
	text := "PREFIX A😀B SUFFIX"
	d := &Document{Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: text, FontFamily: "DejaVu", FontSize: 18}}}}}
	var out bytes.Buffer
	if err = Write(context.Background(), &out, d, PDF, WriteOptions{PDFFonts: []FontFace{{Family: "DejaVu", Data: data}}}); err != nil {
		t.Fatal(err)
	}
	back, err := ReadPDF(context.Background(), bytes.NewReader(out.Bytes()), ReadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if back.Text() != text {
		t.Fatalf("inference separated glyphs from neighbors: %q", back.Text())
	}
}
func TestPDFGlyphCIDInventoryAndCancellation(t *testing.T) {
	face := pdfGlyphFace{codes: map[pdfGlyphCode]uint16{}}
	a, err := face.cid(17, "ab")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := face.cid(17, "ab")
	c, _ := face.cid(17, "xy")
	z, _ := face.cid(18, "")
	if a != b || a == c || c == z || len(face.entries) != 3 {
		t.Fatal("CID semantics collided")
	}
	face.entries = make([]pdfGlyphCode, 65535)
	if _, err = face.cid(99, "overflow"); err == nil {
		t.Fatal("CID resource bound bypassed")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r := pdfRenderer{ctx: ctx}
	if _, err = r.glyphCluster(pdfFontFace{}, "B\u0315"); !errors.Is(err, context.Canceled) {
		t.Fatalf("shaping ignored cancellation %v", err)
	}
}
func TestPDFGlyphStrictFontGraphAndContextualMarks(t *testing.T) {
	data, err := os.ReadFile("testdata/assets/NotoSans-Regular.ttf")
	if err != nil {
		t.Fatal(err)
	}
	text := "B\u0315\u0315"
	d := &Document{Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: text, FontFamily: "Noto", FontSize: 20}}}}}
	var out bytes.Buffer
	if err = Write(context.Background(), &out, d, PDF, WriteOptions{PDFFonts: []FontFace{{Family: "Noto", Data: data}}}); err != nil {
		t.Fatal(err)
	}
	conf := &model.Configuration{Reader15: true, ValidationMode: model.ValidationStrict, Offline: true, Eol: "\n"}
	parsed, e := pdfcpu.ReadWithContext(context.Background(), bytes.NewReader(out.Bytes()), conf)
	if e != nil {
		t.Fatal(e)
	}
	if err = validate.XRefTable(parsed); err != nil {
		t.Fatalf("independent strict font/file validation: %v", err)
	}
	back, err := ReadPDF(context.Background(), bytes.NewReader(out.Bytes()), ReadOptions{PDFSkipInference: true})
	if err != nil {
		t.Fatal(err)
	}
	if back.Text() != text {
		t.Fatalf("mark offsets split logical text: %q", back.Text())
	}
}
func FuzzPDFGlyphClusters(f *testing.F) {
	data, err := os.ReadFile("testdata/assets/NotoSans-Regular.ttf")
	if err != nil {
		f.Fatal(err)
	}
	f.Add(byte('B'), byte(0x15))
	f.Add(byte('Z'), byte(0x5c))
	f.Fuzz(func(t *testing.T, base, mark byte) {
		text := string(rune('A'+base%26)) + string(rune(0x300+uint16(mark)%0x70))
		d := &Document{Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: text, FontFamily: "Noto"}}}}}
		var out bytes.Buffer
		err := Write(context.Background(), &out, d, PDF, WriteOptions{PDFFonts: []FontFace{{Family: "Noto", Data: data}}})
		if err != nil {
			if !errors.Is(err, ErrUnsupported) || out.Len() != 0 {
				t.Fatalf("non-atomic glyph error %v", err)
			}
			return
		}
		back, err := ReadPDF(context.Background(), bytes.NewReader(out.Bytes()), ReadOptions{PDFSkipInference: true})
		if err != nil {
			t.Fatal(err)
		}
		if back.Text() != text {
			t.Fatalf("logical cluster changed: %q -> %q", text, back.Text())
		}
	})
}

func FuzzPDFGlyphFont(f *testing.F) {
	data, err := os.ReadFile("testdata/assets/NotoSans-Regular.ttf")
	if err != nil {
		f.Fatal(err)
	}
	f.Add(data)
	f.Add(data[:1000])
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 2<<20 {
			t.Skip()
		}
		var out bytes.Buffer
		d := &Document{Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "B\u0315", FontFamily: "Font"}}}}}
		err := Write(context.Background(), &out, d, PDF, WriteOptions{PDFFonts: []FontFace{{Family: "Font", Data: data}}})
		if err != nil {
			if out.Len() != 0 {
				t.Fatal("font failure emitted partial output")
			}
			return
		}
		back, err := ReadPDF(context.Background(), bytes.NewReader(out.Bytes()), ReadOptions{PDFSkipInference: true})
		if err != nil {
			t.Fatal(err)
		}
		if back.Text() != "B\u0315" {
			t.Fatal("successful font transport corrupted text")
		}
	})
}
func TestPDFNoncollinearReplacementDoesNotInventReadingOrder(t *testing.T) {
	content := "BT /F 10 Tf 10 100 Td (PREFIX) Tj ET /Span << /ActualText <FEFF004203150315> >> BDC BT /F 10 Tf 46 100 Td (X) Tj ET BT /F 10 Tf 48 110 Td (Y) Tj ET EMC"
	d, err := ReadPDF(context.Background(), bytes.NewReader(recoveryPage(content, "")), ReadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !hasDiagnostic(d, "pdf_inference_uncertain") {
		t.Fatal("invented logical baseline for noncollinear replacement")
	}
	rr := recoveryRuns(d)
	if len(rr) != 2 || rr[0].Text != "PREFIX" || rr[1].Text != "B\u0315\u0315" || rr[1].Source.Baseline != nil {
		t.Fatalf("replacement order/geometry %+v", rr)
	}
}
