package manydoc

import (
	"bytes"
	"context"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestDefaultTextShapingMatchesUndAndKnownGlyphs(t *testing.T) {
	data, err := os.ReadFile("testdata/assets/NotoSans-Regular.ttf")
	if err != nil {
		t.Fatal(err)
	}
	renderer := func() *pdfRenderer {
		r := textTestRenderer()
		if err := r.addFonts([]FontFace{{Family: "Noto Sans", Data: data}}); err != nil {
			t.Fatal(err)
		}
		return r
	}
	plain := Run{Text: "office AV To", FontFamily: "Noto Sans", FontSize: 20}
	tagged := plain
	tagged.Language = "und"
	a, err := renderer().lines([]Run{plain}, 80, 20)
	if err != nil {
		t.Fatal(err)
	}
	b, err := renderer().lines([]Run{tagged}, 80, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(a) != len(b) {
		t.Fatal("language tag still changes wrapping")
	}
	for i := range a {
		if lineText(a[i]) != lineText(b[i]) || pdfAdvance(a[i]) != pdfAdvance(b[i]) || len(a[i]) != len(b[i]) {
			t.Fatalf("tag-dependent layout %d", i)
		}
		for j := range a[i] {
			if !reflect.DeepEqual(a[i][j].glyphs.positions, b[i][j].glyphs.positions) {
				t.Fatal("tag-dependent glyphs/positions")
			}
		}
	}
	// Native HarfBuzz oracle: default Noto office uses the ffi ligature.
	r := renderer()
	first, second := plain, plain
	first.Text = "of"
	second.Text = "fice"
	lines, err := r.lines([]Run{first, second}, 200, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 1 || len(lines[0]) != 1 {
		t.Fatalf("equal default runs did not coalesce: %+v", lines)
	}
	for i, want := range []uint16{82, 1654, 70, 72} {
		if len(lines[0][0].glyphs.positions) != 4 || lines[0][0].glyphs.positions[i].glyph != want {
			t.Fatal("default ligature missing")
		}
	}
	plain.Text = "ffi"
	lines, err = r.lines([]Run{plain}, 8, 20)
	if err != nil || len(lines) != 3 {
		t.Fatalf("default emergency re-shaping %+v %v", lines, err)
	}
	for i, want := range []uint16{73, 73, 76} {
		if len(lines[i][0].glyphs.positions) != 1 || lines[i][0].glyphs.positions[0].glyph != want {
			t.Fatal("default ligature sliced at emergency boundary")
		}
	}
}

func TestPDFControlOnlyRetainsActualTextVersionAndContent(t *testing.T) {
	d := &Document{Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "\t\n"}}}}}
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, PDF, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(out.Bytes(), []byte("%PDF-1.4")) {
		t.Fatal("control-only ActualText needs PDF 1.4")
	}
	back, err := ReadPDF(context.Background(), &out, ReadOptions{PDFSkipInference: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(back.Text(), "\t") {
		t.Fatalf("lost control %q", back.Text())
	}
}

func FuzzDefaultPDFText(f *testing.F) {
	data, err := os.ReadFile("testdata/assets/NotoSans-Regular.ttf")
	if err != nil {
		f.Fatal(err)
	}
	f.Add([]byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, byte(20))
	f.Add([]byte{6, 8, 7, 0, 0, 1, 5, 4}, byte(100))
	f.Fuzz(func(t *testing.T, input []byte, width byte) {
		if len(input) > 24 {
			return
		}
		alphabet := []string{"office", "AV", "To", "б", "Ω", "Ж", "B\u0315", "e\u0301", " ", "\n", "\u00ad"}
		var text strings.Builder
		for _, b := range input {
			text.WriteString(alphabet[int(b)%len(alphabet)])
		}
		original := text.String()
		d := &Document{Layout: Layout{Width: 180 + float64(width), Height: 320, Margin: 20}, Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: original, FontFamily: "Noto Sans", FontSize: 14}}}}}
		var out bytes.Buffer
		if err := Write(context.Background(), &out, d, PDF, WriteOptions{PDFFonts: []FontFace{{Family: "Noto Sans", Data: data}}}); err != nil {
			if !errors.Is(err, ErrUnsupported) || out.Len() != 0 {
				t.Fatalf("non-atomic default shaping %v", err)
			}
			return
		}
		back, err := ReadPDF(context.Background(), &out, ReadOptions{PDFSkipInference: true})
		if err != nil {
			t.Fatal(err)
		}
		// Physical wrapping adds newlines; authored hard breaks are also logical
		// newlines. Every other original scalar, including marks and SHY, must survive.
		if strings.ReplaceAll(back.Text(), "\n", "") != strings.ReplaceAll(original, "\n", "") {
			t.Fatalf("default logical text %q -> %q", original, back.Text())
		}
		assertRecoverySF(t, back)
	})
}

func TestSpecialSpaceCombiningMarkKeepsPositionedGlyphs(t *testing.T) {
	data, err := os.ReadFile("testdata/assets/NotoSans-Regular.ttf")
	if err != nil {
		t.Fatal(err)
	}
	text := "A\u2007\u0315B"
	r := textTestRenderer()
	if err := r.addFonts([]FontFace{{Family: "Noto Sans", Data: data}}); err != nil {
		t.Fatal(err)
	}
	lines, err := r.lines([]Run{{Text: text, FontFamily: "Noto Sans", FontSize: 20}}, 200, 20)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, span := range lines[0] {
		if strings.Contains(span.text, "\u0315") {
			found = true
			if span.glyphs == nil || len(span.glyphs.positions) != 2 {
				t.Fatal("marked special space fell back to unpositioned primitives")
			}
		}
	}
	if !found {
		t.Fatal("lost mark")
	}
	d := &Document{Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: text, FontFamily: "Noto Sans", FontSize: 20}}}}}
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, PDF, WriteOptions{PDFFonts: []FontFace{{Family: "Noto Sans", Data: data}}}); err != nil {
		t.Fatal(err)
	}
	back, err := ReadPDF(context.Background(), &out, ReadOptions{PDFSkipInference: true})
	if err != nil {
		t.Fatal(err)
	}
	if back.Text() != text {
		t.Fatalf("marked special space changed: %q", back.Text())
	}
}
