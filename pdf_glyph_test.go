package manifold

import (
	"bytes"
	"context"
	"errors"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/validate"
	"os"
	"strings"
	"testing"
)

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
