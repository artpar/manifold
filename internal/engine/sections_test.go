package engine

import (
	"archive/zip"
	"bytes"
	"compress/zlib"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"image"
	"image/jpeg"
	"io"
	"math"
	"os"
	"reflect"
	"strings"
	"testing"
)

func imageFitDocument(t testing.TB) *Document {
	t.Helper()
	data, err := os.ReadFile("examples/image-fit/document.sf")
	if err != nil {
		t.Fatal(err)
	}
	sf, err := ReadStructured(context.Background(), bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	d, err := FromStructured(sf)
	if err != nil {
		t.Fatal(err)
	}
	return d
}
func assertImageBounds(t testing.TB, d *Document, width, height, margin float64) int {
	t.Helper()
	count := 0
	for _, b := range d.Blocks {
		if b.Kind != Picture {
			continue
		}
		count++
		x0, y0, x1, y1 := quadBounds(*b.Source.Quad)
		if x0 < margin-.001 || y0 < margin-.001 || x1 > width-margin+.001 || y1 > height-margin+.001 {
			t.Fatalf("image outside container page: %+v", *b.Source.Quad)
		}
	}
	return count
}
func TestImagesFitBodyNestedCellsAndFootnotes(t *testing.T) {
	d := imageFitDocument(t)
	before := canonicalSF(t, d)
	got, data := renderPageBreakDocument(t, d)
	if got.Metadata.PageCount != 7 {
		t.Fatalf("image-fit pages%d", got.Metadata.PageCount)
	}
	if assertImageBounds(t, got, 300, 200, 20) != 4 {
		t.Fatal("lost/duplicated fitted image")
	}
	expected := map[int][4]float64{1: {20, 20, 15.2, 152}, 2: {32, 32, 12, 120}, 4: {38, 42, 13, 130}, 6: {50, 54, 9, 90}}
	for _, b := range got.Blocks {
		if b.Kind == Picture {
			want, ok := expected[b.Source.Page]
			if !ok {
				t.Fatal("figure moved to wrong page")
			}
			x, y, x1, y1 := quadBounds(*b.Source.Quad)
			for i, v := range []float64{x, y, x1 - x, y1 - y} {
				if math.Abs(v-want[i]) > .001 {
					t.Fatalf("image bounds %v want%v", []float64{x, y, x1 - x, y1 - y}, want)
				}
			}
			sameImagePixels(t, d.Blocks[0].Image.Data, b.Image.Data)
		}
	}
	for token, page := range map[string]int{"SIDE_TOP": 2, "SIDE_BOTTOM": 2, "IMAGE_NOTE_REF": 3, "NESTED_NOTE_REF": 5, "FINISH": 7} {
		checkTokenPage(t, got, token, page)
	}
	if strings.Count(got.Text(), "Continued on a later page") != 2 {
		t.Fatal("deferred figures need visible notices")
	}
	if !bytes.Equal(before, canonicalSF(t, d)) {
		t.Fatal("fitting changed SF preferences")
	}
	// Re-read of SF retains original preferred sizes, not the fitted rectangle.
	var sf bytes.Buffer
	if err := Write(context.Background(), &sf, d, SF, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	back, err := Read(context.Background(), &sf, SF, ReadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, canonicalSF(t, back)) {
		t.Fatal("SF changed preferred image dimensions")
	}
	_ = data // independent annotation destinations are checked by the QA verifier.
	var docx bytes.Buffer
	if err := Write(context.Background(), &docx, d, DOCX, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	word, err := ReadDOCX(context.Background(), &docx)
	if err != nil {
		t.Fatal(err)
	}
	dims := [][2]float64{{word.Blocks[0].Image.Width, word.Blocks[0].Image.Height}, {word.Blocks[2].Rows[0].Cells[0].Blocks[0].Rows[0].Cells[0].Blocks[0].Image.Width, word.Blocks[2].Rows[0].Cells[0].Blocks[0].Rows[0].Cells[0].Blocks[0].Image.Height}, {word.Notes[0].Blocks[0].Image.Width, word.Notes[0].Blocks[0].Image.Height}, {word.Notes[1].Blocks[0].Rows[0].Cells[0].Blocks[0].Rows[0].Cells[0].Blocks[0].Image.Width, word.Notes[1].Blocks[0].Rows[0].Cells[0].Blocks[0].Rows[0].Cells[0].Blocks[0].Image.Height}}
	for i, want := range [][2]float64{{15.2, 152}, {12.4, 124}, {9.6, 96}, {6.8, 68}} {
		for j := range want {
			if math.Abs(dims[i][j]-want[j]) > .001 {
				t.Fatalf("DOCX fitted dimensions%v want%v", dims[i], want)
			}
		}
	}
}
func TestFirstImageNotesUseStableActualDestinations(t *testing.T) {
	base := imageFitDocument(t).Blocks[0].Image
	a, b := *base, *base
	a.Height = 2000
	b.Height = 1000
	for _, many := range []bool{false, true} {
		t.Run(fmt.Sprint(many), func(t *testing.T) {
			d := &Document{Layout: Layout{Width: 300, Height: 200, Margin: 20}, Notes: []Note{{Kind: Footnote, ID: "a", Blocks: []Block{{Kind: Picture, Image: &a}}}}, Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "REFERENCE"}, noteRun(Footnote, "a")}}}}
			if many {
				d.Notes = append(d.Notes, Note{Kind: Footnote, ID: "b", Blocks: []Block{{Kind: Picture, Image: &b}}})
				d.Blocks[0].Runs = append(d.Blocks[0].Runs, noteRun(Footnote, "b"))
			} else {
				d.Blocks[0].Runs = append(d.Blocks[0].Runs, noteRun(Footnote, "a"))
			}
			got, data := renderPageBreakDocument(t, d)
			if many {
				if got.Metadata.PageCount != 3 {
					t.Fatal("multiple figures must continue separately")
				}
				checkNoteTarget(t, data, 2, 3, 2, 3)
			} else {
				if got.Metadata.PageCount != 2 {
					t.Fatal("repeated reference duplicated figure")
				}
				checkNoteTarget(t, data, 2, 2, 2)
			}
		})
	}
}
func TestPreviouslyOversizedImagesPreserveContent(t *testing.T) {
	for _, kind := range []string{"cell", "note", "recursive"} {
		t.Run(kind, func(t *testing.T) {
			d := richCellDocument(t)
			if kind == "cell" {
				d.Layout.Height = 150
			}
			if kind == "note" {
				d.Notes[1].Blocks[1].Image.Width = 100
				d.Notes[1].Blocks[1].Image.Height = 1000
			}
			if kind == "recursive" {
				d = recursiveTableDocument(t)
				d.Notes[1].Blocks[1].Rows[0].Cells[0].Blocks[15].Image.Height = 2000
			}
			got, _ := renderPageBreakDocument(t, d)
			tokens := []string{"CELL_START", "CELL_END", "INNER_A", "INNER_B", "INNER_C", "TOP_RIGHT", "BOTTOM_RIGHT", "AFTER_TABLE", "NOTE_CELL", "NOTE_NESTED"}
			images := 3
			if kind == "recursive" {
				tokens = recursiveTokens()
				images = 2
			}
			for _, token := range tokens {
				if strings.Count(strings.Join(strings.Fields(got.Text()), ""), token) != 1 {
					t.Fatalf("lost/repeated original %s", token)
				}
			}
			if assertImageBounds(t, got, d.Layout.Width, d.Layout.Height, d.Layout.Margin) != images {
				t.Fatal("figure appearance lost or repeated")
			}
		})
	}
}
func TestImageFittingRejectsRoundedZeroDimensions(t *testing.T) {
	base := *imageFitDocument(t).Blocks[0].Image
	for _, size := range [][2]float64{{1, 1e308}, {1e308, 1}} {
		for _, format := range []Format{PDF, DOCX} {
			base.Width, base.Height = size[0], size[1]
			var out bytes.Buffer
			err := Write(context.Background(), &out, &Document{Blocks: []Block{{Kind: Picture, Image: &base}}}, format, WriteOptions{})
			if err == nil || out.Len() != 0 {
				t.Fatalf("unrepresentable %s rectangle emitted: %v", format, err)
			}
		}
	}
}
func FuzzImageContainerFitting(f *testing.F) {
	f.Add(uint16(0), uint16(0), uint16(0), uint16(0), byte(0), byte(0))
	f.Add(uint16(127), uint16(151), uint16(65535), uint16(1), byte(3), byte(3))
	f.Fuzz(func(t *testing.T, w, h, pw, ph uint16, kind, depth byte) {
		im := *imageFitDocument(t).Blocks[0].Image
		switch kind % 4 {
		case 0:
			im.Width, im.Height = 0, 0
		case 1:
			im.Width, im.Height = 1+float64(pw), 0
		case 2:
			im.Width, im.Height = 0, 1+float64(ph)
		case 3:
			im.Width, im.Height = 1+float64(pw), 1+float64(ph)
		}
		flow := []Block{{Kind: Picture, Image: &im}}
		for i := 0; i < int(depth%4); i++ {
			flow = []Block{cellTable(flow)}
		}
		d := &Document{Layout: Layout{Width: 300 + float64(w%250), Height: 240 + float64(h%300), Margin: 20}}
		note := kind&4 != 0
		if note {
			d.Blocks = []Block{{Kind: Paragraph, Runs: []Run{{Text: "REF"}, noteRun(Footnote, "n")}}}
			d.Notes = []Note{{Kind: Footnote, ID: "n", Blocks: flow}}
		} else {
			d.Blocks = flow
		}
		before, _ := json.Marshal(d)
		got, data := renderPageBreakDocument(t, d)
		after, _ := json.Marshal(d)
		if !bytes.Equal(before, after) {
			t.Fatal("fit mutated source")
		}
		if assertImageBounds(t, got, d.Layout.Width, d.Layout.Height, d.Layout.Margin) != 1 {
			t.Fatal("figure lost or repeated")
		}
		page := 0
		for _, b := range got.Blocks {
			if b.Kind == Picture {
				page = b.Source.Page
				sameImagePixels(t, im.Data, b.Image.Data)
				width, height := 64.0, 640.0
				if im.Width > 0 && im.Height > 0 {
					width, height = im.Width, im.Height
				}
				// Bound the cross-product error by the PDF matrix's five-decimal
				// coordinate precision, including very narrow preferred rectangles.
				if math.Abs(b.Image.Height*width-b.Image.Width*height) > .000011*(width+height) {
					t.Fatal("fitting changed the requested aspect ratio")
				}
			}
		}
		if note {
			if strings.Contains(got.Text(), "Continued on a later page") {
				checkNoteTarget(t, data, page, page)
			} else {
				checkNoteTarget(t, data, 1)
			}
		}
	})
}

func TestOutputImageMalformedIDATDoesNotPanic(t *testing.T) {
	var data bytes.Buffer
	data.WriteString("\x89PNG\r\n\x1a\n")
	chunk := func(name string, payload []byte) {
		binary.Write(&data, binary.BigEndian, uint32(len(payload)))
		data.WriteString(name)
		data.Write(payload)
		crc := crc32.NewIEEE()
		crc.Write([]byte(name))
		crc.Write(payload)
		binary.Write(&data, binary.BigEndian, crc.Sum32())
	}
	header := make([]byte, 13)
	binary.BigEndian.PutUint32(header, 64)
	binary.BigEndian.PutUint32(header[4:], 640)
	header[8], header[9] = 8, 6
	chunk("IHDR", header)
	var compressed bytes.Buffer
	zw := zlib.NewWriter(&compressed)
	zw.Write(make([]byte, 1+64*4))
	zw.Close()
	chunk("IDAT", compressed.Bytes())
	chunk("IEND", nil)
	// The complete PNG framing and zlib stream contain only one of 640 rows.
	if _, _, err := image.DecodeConfig(bytes.NewReader(data.Bytes())); err != nil {
		t.Fatal(err)
	}
	for _, format := range []Format{PDF, DOCX} {
		t.Run(string(format), func(t *testing.T) {
			defer func() {
				if p := recover(); p != nil {
					t.Fatalf("malformed raster panicked: %v", p)
				}
			}()
			var out bytes.Buffer
			d := &Document{Blocks: []Block{{Kind: Picture, Image: &Image{Data: data.Bytes(), MediaType: "image/png"}}}}
			if err := Write(context.Background(), &out, d, format, WriteOptions{}); err == nil || out.Len() != 0 {
				t.Fatal("malformed raster emitted output")
			}
		})
	}
}

func TestOutputImageAllocationBoundBeforeExpansion(t *testing.T) {
	data := append([]byte(nil), imageFitDocument(t).Blocks[0].Image.Data...)
	binary.BigEndian.PutUint32(data[16:], 50000)
	binary.BigEndian.PutUint32(data[20:], 50000)
	binary.BigEndian.PutUint32(data[29:], crc32.ChecksumIEEE(data[12:29]))
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || cfg.Width != 50000 {
		t.Fatal("invalid test header", err)
	}
	for _, format := range []Format{PDF, DOCX} {
		var out bytes.Buffer
		err := Write(context.Background(), &out, &Document{Blocks: []Block{{Kind: Picture, Image: &Image{Data: data, MediaType: "image/png"}}}}, format, WriteOptions{})
		if !errors.Is(err, ErrUnsupported) || out.Len() != 0 {
			t.Fatalf("unbounded image expansion accepted: %v", err)
		}
	}
}
func TestJPEGHeightFittingKeepsOriginalAsset(t *testing.T) {
	src := imageFitDocument(t).Blocks[0].Image
	im, _, err := image.Decode(bytes.NewReader(src.Data))
	if err != nil {
		t.Fatal(err)
	}
	var encoded bytes.Buffer
	if err = jpeg.Encode(&encoded, im, &jpeg.Options{Quality: 85}); err != nil {
		t.Fatal(err)
	}
	asset := &Image{Data: encoded.Bytes(), MediaType: "image/jpeg", Width: 80}
	for _, format := range []Format{PDF, DOCX} {
		var out bytes.Buffer
		d := &Document{Layout: Layout{Width: 300, Height: 200, Margin: 20}, Blocks: []Block{{Kind: Picture, Image: asset}}}
		if err := Write(context.Background(), &out, d, format, WriteOptions{}); err != nil {
			t.Fatal(err)
		}
		if format == PDF && !bytes.Contains(out.Bytes(), asset.Data) {
			t.Fatal("PDF did not embed the original JPEG stream")
		}
		got, err := Read(context.Background(), &out, format, ReadOptions{PDFSkipInference: true, PDFSkipStructure: true})
		if err != nil {
			t.Fatal(err)
		}
		image := got.Blocks[0].Image
		if math.Abs(image.Width-15.2) > .001 || math.Abs(image.Height-152) > .001 {
			t.Fatal("JPEG aspect/fitting changed")
		}
		if format == PDF {
			// PDF extraction normalizes raster assets to PNG; compare actual
			// decoded pixels as well as the embedded original DCT stream above.
			sameImagePixels(t, asset.Data, image.Data)
		} else if !bytes.Equal(image.Data, asset.Data) {
			t.Fatal("DOCX JPEG source was resampled or transcoded")
		}
	}
}

func TestImageUsesFullContainerAfterPreface(t *testing.T) {
	im := imageFitDocument(t).Blocks[0].Image
	d := &Document{Layout: Layout{Width: 300, Height: 200, Margin: 20}, Blocks: []Block{mergeParagraph("PREFACE"), {Kind: Picture, Image: im}}}
	got, _ := renderPageBreakDocument(t, d)
	if got.Metadata.PageCount != 2 {
		t.Fatal("large figure did not move to fresh page")
	}
	checkTokenPage(t, got, "PREFACE", 1)
	for _, b := range got.Blocks {
		if b.Kind == Picture {
			if b.Source.Page != 2 || math.Abs(b.Image.Height-152) > .001 {
				t.Fatal("figure shrank to current page remainder")
			}
		}
	}
}
func TestLargeFirstImageNoteAfterHeadingReference(t *testing.T) {
	im := *imageFitDocument(t).Blocks[0].Image
	im.Height = 2000
	d := &Document{Layout: Layout{Width: 300, Height: 200, Margin: 20}, Blocks: []Block{{Kind: Heading, Level: 1, Runs: []Run{{Text: "HEADING"}, noteRun(Footnote, "n")}}}, Notes: []Note{{Kind: Footnote, ID: "n", Blocks: []Block{{Kind: Picture, Image: &im}}}}}
	got, data := renderPageBreakDocument(t, d)
	if got.Metadata.PageCount != 2 || !strings.Contains(got.Text(), "Continued on a later page") {
		t.Fatal("heading reference cannot place figure note")
	}
	checkNoteTarget(t, data, 2, 2)
}

func TestDOCXHalfEMUIsRejectedBeforeRoundingToZero(t *testing.T) {
	im := *imageFitDocument(t).Blocks[0].

		// Source quads describe font bounds, whose top is below the
		// paragraph origin. Check the real 11pt baseline at 60+11.

		Image
	im.Width, im.Height = .5/12700, 1
	if fmt.Sprintf("%.0f", im.Width*12700) != "0" {
		t.Fatal("invalid rounding-boundary fixture")
	}
	var out bytes.Buffer
	if err := Write(context.Background(), &out, &Document{Blocks: []Block{{Kind: Picture, Image: &im}}}, DOCX, WriteOptions{}); !errors.Is(err, ErrUnsupported) || out.Len() != 0 {
		t.Fatal("zero EMU extent emitted", err)
	}
}
func pagePoint(v float64) *float64 { return &v }
func pageTestDoc(l Layout) *Document {
	return &Document{Layout: l, Headers: []Block{{Kind: Paragraph, Runs: []Run{{Text: "HEADER", FontSize: 8}}}}, Footers: []Block{{Kind: Paragraph, Runs: []Run{{Text: "FOOTER", FontSize: 8}}}}, Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "BODY"}}}, {Kind: PageBreak}, {Kind: Paragraph, Runs: []Run{{Text: "BODY TWO"}}}}}
}
func TestPageLayoutSFAndDOCX(t *testing.T) {
	cases := []Layout{
		{Width: 360, Height: 420, Margin: 17, Margins: &PageMargins{60, 44, 55, 22}, HeaderDistance: pagePoint(12), FooterDistance: pagePoint(14)},
		{Width: 200, Height: 240, Margin: 42, Margins: &PageMargins{}, HeaderDistance: pagePoint(0), FooterDistance: pagePoint(0)},
		{Width: 300, Height: 400, Margins: &PageMargins{30, 30, 30, 30}},
		{Width: 300, Height: 400, Margin: 30, HeaderDistance: pagePoint(0)},
		{Width: 360, Height: 420, Margins: &PageMargins{-60, 44, -55, 22}, HeaderDistance: pagePoint(12), FooterDistance: pagePoint(14)},
	}
	for i, l := range cases {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			d := pageTestDoc(l)
			before := canonicalSF(t, d)
			for _, f := range []Format{SF, JSON, DOCX} {
				var buf bytes.Buffer
				if err := Write(context.Background(), &buf, d, f, WriteOptions{}); err != nil {
					t.Fatal(f, err)
				}
				back, err := Read(context.Background(), bytes.NewReader(buf.Bytes()), f, ReadOptions{})
				if err != nil {
					t.Fatal(f, err)
				}
				if !bytes.Equal(before, canonicalSF(t, back)) {
					t.Fatalf("%s lost page/settings/content semantics", f)
				}
			}
			if !bytes.Equal(before, canonicalSF(t, d)) {
				t.Fatal("source changed")
			}
		})
	}
}
func TestPageLayoutBridgeOwnership(t *testing.T) {
	d := pageTestDoc(Layout{Width: 300, Height: 400, Margins: &PageMargins{40, 20, 30, 25}, HeaderDistance: pagePoint(10), FooterDistance: pagePoint(12)})
	s, err := ToStructured(d)
	if err != nil {
		t.Fatal(err)
	}
	back, err := FromStructured(s)
	if err != nil {
		t.Fatal(err)
	}
	s.Attrs.Layout.Margins.Left = 100
	*s.Attrs.Layout.HeaderDistance = 33
	*s.Attrs.Layout.FooterDistance = 44
	if d.Layout.Margins.Left != 25 || back.Layout.Margins.Left != 25 || *d.Layout.HeaderDistance != 10 || *back.Layout.FooterDistance != 12 {
		t.Fatal("page settings share mutable ownership")
	}
}
func TestPageLayoutPDFPositions(t *testing.T) {
	d := pageTestDoc(Layout{Width: 360, Height: 420, Margin: 17, Margins: &PageMargins{60, 44, 55, 22}, HeaderDistance: pagePoint(12), FooterDistance: pagePoint(14)})
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, PDF, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	back, err := ReadPDF(context.Background(), bytes.NewReader(out.Bytes()), ReadOptions{PDFSkipInference: true, PDFSkipStructure: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(back.Pages) != 2 {
		t.Fatal("page count")
	}
	counts := map[string]int{}
	for _, b := range back.Blocks {
		if b.Source == nil || b.Source.Quad == nil {
			continue
		}
		var text strings.Builder
		for _, r := range b.Runs {
			text.WriteString(r.Text)
		}
		s := text.String()
		if s == "" {
			continue
		}
		counts[s]++
		x, y, x1, y1 := quadBounds(*b.Source.Quad)
		if x < 21.99 || x1 > 316.01 {
			t.Fatalf("horizontal body/running bounds %q %.3f..%.3f", s, x, x1)
		}
		if strings.HasPrefix(s, "BODY") && (y < 59.99 || y1 > 365.01) {
			t.Fatalf("body bounds %q %.3f..%.3f", s, y, y1)
		}
		if s == "HEADER" && (y < 11.9 || y1 > 23) {
			t.Fatal("header edge distance")
		}
		if s == "FOOTER" && (y < 395.9 || y1 > 406.5) {
			t.Fatalf("footer bottom distance %.3f..%.3f", y, y1)
		}
	}
	if counts["HEADER"] != 2 || counts["FOOTER"] != 2 || counts["BODY"] != 1 || counts["BODY TWO"] != 1 {
		t.Fatalf("content %v", counts)
	}
	assertRecoverySF(t, back)
}
func TestPageLayoutInvalidAndNegativePDF(t *testing.T) {
	for _, l := range []Layout{{Margins: &PageMargins{0, -1, 0, 0}}, {Margins: &PageMargins{0, 0, 0, -1}}, {Margins: &PageMargins{300, 0, 600, 0}}, {Margins: &PageMargins{0, 500, 0, 500}}, {HeaderDistance: pagePoint(math.NaN())}, {FooterDistance: pagePoint(math.Inf(1))}, {HeaderDistance: pagePoint(-1)}, {Margins: &PageMargins{math.Inf(1), 0, 0, 0}}} {
		for _, f := range []Format{SF, DOCX, PDF} {
			var out bytes.Buffer
			if err := Write(context.Background(), &out, pageTestDoc(l), f, WriteOptions{}); err == nil || out.Len() != 0 {
				t.Fatal("invalid layout accepted/wrote output", l, f, err)
			}
		}
	}
	d := pageTestDoc(Layout{Width: 300, Height: 400, Margins: &PageMargins{-10, 20, 30, 25}})
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, PDF, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	back, err := ReadPDF(context.Background(), bytes.NewReader(out.Bytes()), ReadOptions{PDFSkipInference: true, PDFSkipStructure: true})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, block := range back.Blocks {
		for _, run := range block.Runs {
			if run.Text == "BODY" {
				found = true
				if run.Source.Baseline == nil || math.Abs(run.Source.Baseline[0].Y-21) > .01 {
					t.Fatal("fixed signed top was enlarged", run.Source)
				}
			}
		}
	}
	if !found {
		t.Fatal("signed body missing")
	}
	out.Reset()
	d.Headers, d.Footers = nil, nil
	if err := Write(context.Background(), &out, d, PDF, WriteOptions{}); err != nil {
		t.Fatal("signed offsets without overlapping running content", err)
	}
}
func TestDOCXPageSettingsExternalAndMetadata(t *testing.T) {
	d := pageTestDoc(Layout{Width: 360, Height: 420, Margin: 30})
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, DOCX, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	plain := rewriteListDOCX(t, out.Bytes(), func(s string) string { return strings.ReplaceAll(s, ` md:layoutImplicit="hf"`, "") })
	back, err := ReadDOCX(context.Background(), bytes.NewReader(plain))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(back.Layout.HeaderDistance, pagePoint(18)) || !reflect.DeepEqual(back.Layout.FooterDistance, pagePoint(18)) {
		t.Fatal("external physical distances not recovered")
	}
	for _, change := range []func(string) string{
		func(s string) string { return strings.Replace(s, `w:header="360"`, `w:header="361"`, 1) },
		func(s string) string { return strings.Replace(s, `layoutImplicit="hf"`, `layoutImplicit="hff"`, 1) },
		func(s string) string { return strings.Replace(s, `w:top="600"`, `w:top="NaN"`, 1) },
	} {
		if _, err := ReadDOCX(context.Background(), bytes.NewReader(rewriteListDOCX(t, out.Bytes(), change))); err == nil {
			t.Fatal("malformed/contradictory page metadata accepted")
		}
	}
	units := rewriteListDOCX(t, plain, func(s string) string {
		s = strings.ReplaceAll(s, `w:top="600"`, `w:top="25.4mm"`)
		s = strings.ReplaceAll(s, `w:left="600"`, `w:left="1in"`)
		s = strings.ReplaceAll(s, `w:right="600"`, `w:right="2.54cm"`)
		s = strings.ReplaceAll(s, `w:bottom="600"`, `w:bottom="6pc"`)
		s = strings.ReplaceAll(s, `w:header="360"`, `w:header="12pt"`)
		return strings.ReplaceAll(s, `w:footer="360"`, `w:footer="1pi"`)
	})
	back, err = ReadDOCX(context.Background(), bytes.NewReader(units))
	if err != nil {
		t.Fatal(err)
	}
	g := pageLayout(back.Layout)
	for _, v := range []float64{g.margins.Top, g.margins.Right, g.margins.Bottom, g.margins.Left} {
		if math.Abs(v-72) > 1e-8 {
			t.Fatal("unit conversion", v)
		}
	}
	if *back.Layout.HeaderDistance != 12 || *back.Layout.FooterDistance != 12 {
		t.Fatal("distance units")
	}
}

func FuzzPageLayoutRoundTrips(f *testing.F) {
	f.Add(uint16(480), uint16(420), byte(70), byte(60), byte(55), byte(30), byte(12))
	f.Add(uint16(300), uint16(400), byte(0), byte(0), byte(0), byte(0), byte(0))
	blob := runningPNG(f, false)
	f.Fuzz(func(t *testing.T, width, height uint16, top, right, bottom, left, dist byte) {
		l := Layout{Width: float64(width%301 + 300), Height: float64(height%301 + 400), Margin: float64(dist) / 20, Margins: &PageMargins{float64(top % 91), float64(right % 81), float64(bottom % 91), float64(left % 81)}}
		if dist&1 == 0 {
			l.HeaderDistance = pagePoint(float64(dist % 21))
		}
		if dist&2 == 0 {
			l.FooterDistance = pagePoint(float64(dist % 21))
		}
		d := pageTestDoc(l)
		d.Blocks = append([]Block{{Kind: Picture, Image: &Image{MediaType: "image/png", Data: blob, Width: 32, Height: 8}}}, d.Blocks...)
		before := canonicalSF(t, d)
		var word bytes.Buffer
		if err := Write(context.Background(), &word, d, DOCX, WriteOptions{}); err != nil {
			t.Fatal(err)
		}
		back, err := ReadDOCX(context.Background(), bytes.NewReader(word.Bytes()))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(before, canonicalSF(t, back)) {
			t.Fatal("page settings/content/assets changed through DOCX")
		}
		var pdf bytes.Buffer
		if err := Write(context.Background(), &pdf, d, PDF, WriteOptions{}); err != nil {
			t.Fatal(err)
		}
		back, err = ReadPDF(context.Background(), bytes.NewReader(pdf.Bytes()), ReadOptions{PDFSkipInference: true, PDFSkipStructure: true})
		if err != nil {
			t.Fatal(err)
		}
		if len(back.Pages) != 2 || strings.Count(back.Text(), "HEADER") != 2 || strings.Count(back.Text(), "FOOTER") != 2 || strings.Count(back.Text(), "BODY TWO") != 1 {
			t.Fatal("layout lost content/pages")
		}
		images := 0
		for _, b := range back.Blocks {
			if b.Image != nil {
				images++
				sameImagePixels(t, blob, b.Image.Data)
				x, y, x1, y1 := quadBounds(*b.Source.Quad)
				g := pageLayout(l)
				if x < g.margins.Left-.01 || x1 > g.width-g.margins.Right+.01 || y < g.margins.Top-.01 || y1 > g.height-g.margins.Bottom+.01 {
					t.Fatal("body figure outside explicit margins")
				}
			}
		}
		if images != 1 || !bytes.Equal(before, canonicalSF(t, d)) {
			t.Fatal("source mutation/image loss")
		}
		assertRecoverySF(t, back)
	})
}

func TestDOCXPageCurrentPropertiesAndLossDiagnostics(t *testing.T) {
	section := `<w:sectPr><w:pgSz w:w="8000" w:h="10000"/><w:pgMar w:top="1000" w:right="800" w:bottom="900" w:left="600" w:header="240" w:footer="280"/><w:cols w:num="2"/><w:pgBorders/><w:titlePg/><w:sectPrChange><w:sectPr><w:pgSz w:w="12000" w:h="14000"/><w:pgMar w:top="3000" w:right="2000" w:bottom="3000" w:left="2000"/><w:headerReference xmlns:r="` + relNS + `" r:id="missing-old" w:type="even"/></w:sectPr></w:sectPrChange></w:sectPr>`
	body := `<w:p><w:r><w:t>CURRENT</w:t></w:r></w:p><w:del><w:p><w:pPr><w:sectPr><w:pgMar w:left="9999"/></w:sectPr></w:pPr><w:r><w:delText>DELETED</w:delText></w:r></w:p></w:del>` + section
	data := themeColorDOCX(t, body, themeColorPalette, `<w:mirrorMargins/><w:evenAndOddHeaders/>`, "")
	d, err := ReadDOCX(context.Background(), bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if d.Text() != "CURRENT" || d.Layout.Width != 400 || d.Layout.Height != 500 || d.Layout.Margins == nil || *d.Layout.Margins != (PageMargins{50, 40, 45, 30}) || *d.Layout.HeaderDistance != 12 || *d.Layout.FooterDistance != 14 {
		t.Fatalf("history/deleted properties replaced current geometry: %+v %q", d.Layout, d.Text())
	}
	for _, code := range []string{"docx_page_settings", "docx_page_columns", "revisions"} {
		if !hasDiagnostic(d, code) {
			t.Fatal("missing loss diagnostic", code)
		}
	}
	if hasDiagnostic(d, "multiple_sections") {
		t.Fatal("history/deleted snapshots counted as actual sections")
	}
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, DOCX, WriteOptions{}); err == nil || out.Len() != 0 {
		t.Fatal("unacknowledged page-property loss exported")
	}
}

func TestPageMarginsDoNotShrinkReservedRunningImages(t *testing.T) {
	d := pageTestDoc(Layout{Width: 300, Height: 500, Margins: &PageMargins{200, 30, 100, 30}, HeaderDistance: pagePoint(12), FooterDistance: pagePoint(18)})
	d.Headers = []Block{{Kind: Picture, Image: &Image{MediaType: "image/png", Data: runningPNG(t, true), Width: 18.75, Height: 150}}}
	before := canonicalSF(t, d)
	var word bytes.Buffer
	if err := Write(context.Background(), &word, d, DOCX, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	back, err := ReadDOCX(context.Background(), bytes.NewReader(word.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, canonicalSF(t, back)) {
		t.Fatal("reserved header picture was shrunk to the small body container")
	}
	var pdf bytes.Buffer
	if err := Write(context.Background(), &pdf, d, PDF, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	back, err = ReadPDF(context.Background(), bytes.NewReader(pdf.Bytes()), ReadOptions{PDFSkipInference: true, PDFSkipStructure: true})
	if err != nil {
		t.Fatal(err)
	}
	images := 0
	for _, b := range back.Blocks {
		if b.Image != nil {
			images++
			x, y, x1, y1 := quadBounds(*b.Source.Quad)
			if math.Abs(x-30) > .01 || math.Abs(y-12) > .01 || math.Abs(x1-x-18.75) > .01 || math.Abs(y1-y-150) > .01 {
				t.Fatal("reserved running image bounds", b.Source.Quad)
			}
		}
	}
	if images != 2 {
		t.Fatal("running repetition")
	}
}

func TestPageLayoutSignedNonoverlapPDF(t *testing.T) {
	d := pageTestDoc(Layout{Width: 360, Height: 420, Margins: &PageMargins{-60, 44, -55, 22}, HeaderDistance: pagePoint(12), FooterDistance: pagePoint(14)})
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, PDF, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	back, err := ReadPDF(context.Background(), bytes.NewReader(out.Bytes()), ReadOptions{PDFSkipInference: true, PDFSkipStructure: true})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, b := range back.Blocks {
		for _, run := range b.Runs {
			if run.Text == "BODY" {

				if run.Source == nil || run.Source.Baseline == nil {
					t.Fatal("missing source baseline")
				}
				point := run.Source.Baseline[0]
				if math.Abs(point.X-22) > .01 || math.Abs(point.Y-71) > .01 {
					t.Fatal("signed body baseline was not fixed", point)
				}
				found = true
			}
		}
	}
	if !found {
		t.Fatal("missing body")
	}
}

func TestPageLayoutPhysicalSerializationAtomic(t *testing.T) {
	for _, item := range []struct {
		format Format
		layout Layout
	}{
		{DOCX, Layout{Width: .001, Height: 200, Margins: &PageMargins{}}},
		{PDF, Layout{Width: .001, Height: 200, Margins: &PageMargins{}}},
		{DOCX, Layout{Width: 200, Height: 200.0001, Margins: &PageMargins{100, 0, 100, 0}}},
		{DOCX, Layout{Width: 200, Height: 200, Margins: &PageMargins{-.001, 0, 0, 0}}},
		{DOCX, Layout{Width: math.MaxFloat64, Height: 200, Margins: &PageMargins{}}},
	} {
		d := &Document{Layout: item.layout}
		if err := d.Validate(); err != nil {
			t.Fatal("finite persistent layout should remain representable in SF", err)
		}
		var out bytes.Buffer
		if err := Write(context.Background(), &out, d, item.format, WriteOptions{}); !errors.Is(err, ErrUnsupported) || out.Len() != 0 {
			t.Fatalf("unrepresentable %s page emitted: %v (%d bytes)", item.format, err, out.Len())
		}
	}
}

func TestPageLayoutUnusedDistancesPersist(t *testing.T) {
	d := pageTestDoc(Layout{Width: 360, Height: 420, Margin: 30, HeaderDistance: pagePoint(900), FooterDistance: pagePoint(900)})
	d.Headers, d.Footers = nil, nil
	before := canonicalSF(t, d)
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, DOCX, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	back, err := ReadDOCX(context.Background(), bytes.NewReader(out.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, canonicalSF(t, back)) {
		t.Fatal("unused distances were dropped or normalized")
	}
	d.Headers = []Block{{Kind: Paragraph, Runs: []Run{{Text: "HEADER"}}}}
	for _, format := range []Format{DOCX, PDF} {
		out.Reset()
		if err := Write(context.Background(), &out, d, format, WriteOptions{}); !errors.Is(err, ErrUnsupported) || out.Len() != 0 {
			t.Fatal("out-of-page running story was not rejected atomically", format, err)
		}
	}
}
func sectionParagraph(text string) Block { return Block{Kind: Paragraph, Runs: []Run{{Text: text}}} }
func sectionStory(text string) *RunningStory {
	return &RunningStory{Blocks: []Block{sectionParagraph(text)}}
}
func sectionTestDocument(t testing.TB) *Document {
	blob := runningPNG(t, false)
	landscape := Layout{Width: 480, Height: 360, Margin: 29, Margins: &PageMargins{Top: 45, Right: 40, Bottom: 50, Left: 30}, HeaderDistance: pagePoint(12), FooterDistance: pagePoint(15)}
	image := Block{Kind: Picture, Image: &Image{MediaType: "image/png", Data: blob, Width: 32, Height: 16}}
	a := &Section{FirstPage: true, Headers: RunningStories{Default: sectionStory("A DEFAULT"), First: sectionStory("A FIRST"), Even: sectionStory("A EVEN")}, Footers: RunningStories{Default: sectionStory("A FOOT"), First: &RunningStory{}, Even: sectionStory("A EVEN FOOT")}, Blocks: []Block{sectionParagraph("A BODY"), {Kind: Table, Rows: []Row{{Cells: []Cell{{ColSpan: 2, Blocks: []Block{image}}}}, {Cells: []Cell{{Blocks: []Block{sectionParagraph("CELL LEFT")}}, {Blocks: []Block{sectionParagraph("CELL RIGHT")}}}}}}}}
	a.Headers.Default.Blocks = append(a.Headers.Default.Blocks, image)
	a.Headers.First.Blocks[0].Runs[0].Link = "https://example.com/first"
	b := &Section{Layout: &landscape, Start: SectionOddPage, FirstPage: true, Headers: RunningStories{First: &RunningStory{}}, Blocks: []Block{{Kind: ListBlock, List: "number", Items: []ListItem{{Blocks: []Block{sectionParagraph("B LIST")}}}}, sectionParagraph("B BODY")}}
	b.Blocks[1].Runs = append(b.Blocks[1].Runs, Run{Note: &NoteReference{ID: "section-note", Kind: Footnote}})
	c := &Section{Start: SectionNextPage, Headers: RunningStories{Default: &RunningStory{}}, Blocks: []Block{sectionParagraph("C BODY")}}
	return &Document{Layout: Layout{Width: 360, Height: 440, Margin: 40}, EvenAndOddHeaders: true, Blocks: []Block{{Kind: SectionBlock, Section: a}, {Kind: SectionBlock, Section: b}, {Kind: SectionBlock, Section: c}}, Notes: []Note{{ID: "section-note", Kind: Footnote, Blocks: []Block{sectionParagraph("SECTION NOTE"), image}}}}
}
func TestSectionSFAndDOCXSemantics(t *testing.T) {
	d := sectionTestDocument(t)
	before := canonicalSF(t, d)
	for _, f := range []Format{SF, JSON, DOCX} {
		var out bytes.Buffer
		if err := Write(context.Background(), &out, d, f, WriteOptions{}); err != nil {
			t.Fatal(f, err)
		}
		back, err := Read(context.Background(), &out, f, ReadOptions{})
		if err != nil {
			t.Fatal(f, err)
		}
		if got := canonicalSF(t, back); !bytes.Equal(before, got) {
			t.Fatalf("%s section semantics changed\nwant %s\ngot %s", f, before, got)
		}
	}
	if !bytes.Equal(before, canonicalSF(t, d)) {
		t.Fatal("source changed")
	}
	resolved := documentSections(d)
	if len(resolved) != 3 || resolved[1].headers.Default != d.Blocks[0].Section.Headers.Default || len(resolved[1].headers.First.Blocks) != 0 || len(resolved[2].headers.Default.Blocks) != 0 || resolved[2].layout.Width != 480 {
		t.Fatal("story/layout inheritance")
	}
	sf, err := ToStructured(d)
	if err != nil {
		t.Fatal(err)
	}
	sf.Content[1].Attrs = bytes.Replace(sf.Content[1].Attrs, []byte(`"width":480`), []byte(`"width":481`), 1)
	changed, err := FromStructured(sf)
	if err != nil {
		t.Fatal(err)
	}
	changed.Blocks[1].Section.Layout.Margins.Left = 31
	if d.Blocks[1].Section.Layout.Width != 480 || d.Blocks[1].Section.Layout.Margins.Left != 30 {
		t.Fatal("SF layout aliases source")
	}
}
func TestSectionValidation(t *testing.T) {
	for name, mutate := range map[string]func(*Document){
		"mixed-root":   func(d *Document) { d.Blocks = append(d.Blocks, sectionParagraph("VISIBLE")) },
		"nested":       func(d *Document) { d.Blocks[0].Section.Blocks = []Block{{Kind: SectionBlock, Section: &Section{}}} },
		"bad-start":    func(d *Document) { d.Blocks[0].Section.Start = "nextColumn" },
		"dual-stories": func(d *Document) { d.Headers = []Block{sectionParagraph("GLOBAL")} },
		"running-note": func(d *Document) {
			d.Blocks[0].Section.Headers.First.Blocks[0].Runs = []Run{{Note: &NoteReference{Kind: Footnote, ID: "section-note"}}}
		},
		"hidden-fields": func(d *Document) { d.Blocks[0].Runs = []Run{{Text: "VISIBLE"}} },
		"cyclic":        func(d *Document) { d.Blocks[0].Section.Blocks = d.Blocks },
	} {
		t.Run(name, func(t *testing.T) {
			d := sectionTestDocument(t)
			mutate(d)
			if err := d.Validate(); err == nil {
				t.Fatal("invalid sections accepted")
			}
		})
	}
	for _, attrs := range []string{`{"headers":{"unexpected":[]}}`, `{"headers":{"first":null}}`, `{"firstPage":"true"}`, `{"start":"invalid"}`} {
		sf := &StructuredDocument{Type: "doc", Attrs: StructuredAttributes{Schema: StructuredSchema}, Content: []StructuredNode{{Type: "section", Attrs: []byte(attrs)}}}
		if _, err := FromStructured(sf); err == nil {
			t.Fatal("invalid section attrs accepted", attrs)
		}
	}
}
func TestDOCXSectionMetadataCannotHideContent(t *testing.T) {
	d := sectionTestDocument(t)
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, DOCX, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	for i, change := range []func(string) string{
		func(s string) string {
			return strings.Replace(s, `md:sectionTail="1"><w:pPr>`, `md:sectionTail="1"><w:r><w:t>VISIBLE</w:t></w:r><w:pPr>`, 1)
		},
		func(s string) string { return strings.Replace(s, `w:w="7200"`, `w:w="7201"`, 1) },
		func(s string) string { return strings.Replace(s, `w:type="default"`, `w:type="unknown"`, 1) },
		func(s string) string {
			return strings.Replace(s, `md:sectionStartImplicit="1"`, `md:sectionStartImplicit="2"`, 1)
		},
	} {
		if _, err := ReadDOCX(context.Background(), bytes.NewReader(rewriteListDOCX(t, out.Bytes(), change))); err == nil {
			t.Fatal("contradictory section metadata accepted", i)
		}
	}
}

func TestPDFSectionFramesVariantsAndNotes(t *testing.T) {
	d := sectionTestDocument(t)
	a := d.Blocks[0].Section
	a.Blocks = []Block{sectionParagraph("A PAGE ONE"), {Kind: PageBreak}, sectionParagraph("A PAGE TWO"), {Kind: PageBreak}, sectionParagraph("A PAGE THREE")}
	b := d.Blocks[1].Section
	b.Blocks = append([]Block{sectionParagraph("B PAGE ONE"), {Kind: PageBreak}, sectionParagraph("B PAGE TWO"), {Kind: PageBreak}}, b.Blocks...)
	before := canonicalSF(t, d)
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, PDF, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	got, err := ReadPDF(context.Background(), bytes.NewReader(out.Bytes()), ReadOptions{PDFSkipInference: true, PDFSkipStructure: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Pages) != 8 {
		t.Fatalf("pages=%d text=%q", len(got.Pages), got.Text())
	}
	texts := make([]string, 8)
	for _, block := range got.Blocks {
		if block.Source != nil && len(block.Source.Regions) > 0 {
			n := block.Source.Regions[0].Page
			if n > 0 && n <= 8 {
				texts[n-1] += (&Document{Blocks: []Block{block}}).Text() + "\n"
			}
		}
	}
	for _, c := range []struct {
		page         int
		want, absent string
	}{{1, "A FIRST", "A DEFAULT"}, {2, "A EVEN", "A FIRST"}, {3, "A DEFAULT", "A FIRST"}, {4, "", "A FOOT"}, {5, "B PAGE ONE", "A FIRST"}, {6, "A EVEN", "A DEFAULT"}, {7, "A DEFAULT", "A FIRST"}, {8, "C BODY", "A DEFAULT"}} {
		if !strings.Contains(texts[c.page-1], c.want) || c.absent != "" && strings.Contains(texts[c.page-1], c.absent) {
			t.Fatalf("page %d text=%q", c.page, texts[c.page-1])
		}
	}
	if strings.TrimSpace(texts[3]) != "" {
		t.Fatal("parity page is not blank", texts[3])
	}
	for i, p := range got.Pages {
		wantW, wantH := 360.0, 440.0
		if i >= 4 {
			wantW, wantH = 480, 360
		}
		if p.Width != wantW || p.Height != wantH {
			t.Fatalf("page %d geometry=%+v", i+1, p)
		}
	}
	if !bytes.Equal(before, canonicalSF(t, d)) {
		t.Fatal("PDF planning mutated source")
	}
}
func TestPDFContinuousSectionContract(t *testing.T) {
	d := &Document{Layout: Layout{Width: 300, Height: 400, Margin: 30}, Blocks: []Block{{Kind: SectionBlock, Section: &Section{Blocks: []Block{sectionParagraph("ONE")}}}, {Kind: SectionBlock, Section: &Section{Start: SectionContinuous, Blocks: []Block{sectionParagraph("TWO")}}}}}
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, PDF, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	got, err := ReadPDF(context.Background(), bytes.NewReader(out.Bytes()), ReadOptions{PDFSkipInference: true, PDFSkipStructure: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Pages) != 1 || !strings.Contains(got.Text(), "TWO") {
		t.Fatal("continuous section started a page", got.Text())
	}
	d.Blocks[1].Section.Layout = &Layout{Width: 400, Height: 300, Margin: 30}
	out.Reset()
	if err := Write(context.Background(), &out, d, PDF, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	got, err = ReadPDF(context.Background(), &out, ReadOptions{PDFSkipInference: true, PDFSkipStructure: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Pages) != 2 || got.Pages[0].Width != 300 || got.Pages[0].Height != 400 || got.Pages[1].Width != 400 || got.Pages[1].Height != 300 {
		t.Fatal("changed paper canvas was discarded", got.Pages)
	}
}

func FuzzSectionRoundTrips(f *testing.F) {
	f.Add(byte(0), byte(0), uint16(300), uint16(400))
	f.Add(byte(7), byte(3), uint16(500), uint16(320))
	f.Fuzz(func(t *testing.T, flags, start byte, width, height uint16) {
		layout := Layout{Width: float64(width%401 + 300), Height: float64(height%401 + 300), Margin: 30}
		s := &Section{Layout: &layout, Start: []SectionStart{"", SectionNextPage, SectionOddPage, SectionEvenPage}[start%4], FirstPage: flags&1 != 0, Blocks: []Block{sectionParagraph("SECTION TWO")}}
		if flags&2 != 0 {
			s.Headers.Default = &RunningStory{}
		}
		if flags&4 != 0 {
			s.Headers.First = sectionStory("NEW FIRST")
		}
		d := &Document{Layout: Layout{Width: 360, Height: 440, Margin: 30}, EvenAndOddHeaders: true, Blocks: []Block{{Kind: SectionBlock, Section: &Section{Headers: RunningStories{Default: sectionStory("DEFAULT"), First: sectionStory("FIRST"), Even: sectionStory("EVEN")}, Blocks: []Block{sectionParagraph("SECTION ONE")}}}, {Kind: SectionBlock, Section: s}}}
		before := canonicalSF(t, d)
		for _, format := range []Format{SF, DOCX} {
			var out bytes.Buffer
			if err := Write(context.Background(), &out, d, format, WriteOptions{}); err != nil {
				t.Fatal(err)
			}
			back, err := Read(context.Background(), &out, format, ReadOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(before, canonicalSF(t, back)) {
				t.Fatal("section semantics changed", format)
			}
		}
		var out bytes.Buffer
		if err := Write(context.Background(), &out, d, PDF, WriteOptions{}); err != nil {
			t.Fatal(err)
		}
		back, err := ReadPDF(context.Background(), &out, ReadOptions{PDFSkipInference: true, PDFSkipStructure: true})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(back.Text(), "SECTION ONE") || !strings.Contains(back.Text(), "SECTION TWO") {
			t.Fatal("section body lost")
		}
	})
}

func FuzzSectionBindings(f *testing.F) {
	f.Add(byte(0), byte(0), uint16(300), uint16(400))
	f.Add(byte(7), byte(3), uint16(500), uint16(320))
	f.Fuzz(func(t *testing.T, flags, start byte, width, height uint16) {
		starts := []SectionStart{"", SectionNextPage, SectionEvenPage, SectionOddPage}
		d := &Document{Layout: Layout{Width: 360, Height: 440, Margin: 40}, EvenAndOddHeaders: flags&1 != 0}
		for i := 0; i < 3; i++ {
			s := &Section{Start: starts[(int(start)+i)%4], FirstPage: flags&(1<<i) != 0, Blocks: []Block{sectionParagraph("BODY")}}
			if i == 1 {
				s.Layout = &Layout{Width: float64(width%400 + 300), Height: float64(height%400 + 300), Margin: 30}
			}
			if flags&2 != 0 {
				s.Headers.Default = sectionStory("DEFAULT")
			}
			if flags&4 != 0 {
				s.Headers.First = &RunningStory{}
			}
			if flags&8 != 0 {
				s.Headers.Even = sectionStory("EVEN")
			}
			d.Blocks = append(d.Blocks, Block{Kind: SectionBlock, Section: s})
		}
		want := canonicalSF(t, d)
		var out bytes.Buffer
		if err := Write(context.Background(), &out, d, DOCX, WriteOptions{}); err != nil {
			t.Fatal(err)
		}
		back, err := ReadDOCX(context.Background(), &out)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(want, canonicalSF(t, back)) {
			t.Fatal("section layout/story/binding semantics changed")
		}
		out.Reset()
		if err := Write(context.Background(), &out, d, PDF, WriteOptions{}); err != nil {
			t.Fatal(err)
		}
		if _, err := ReadPDF(context.Background(), &out, ReadOptions{PDFSkipInference: true, PDFSkipStructure: true}); err != nil {
			t.Fatal(err)
		}
	})
}

func TestDOCXForeignSectionBoundaries(t *testing.T) {
	body := `<w:p><w:pPr><w:sectPr><w:type w:val="nextPage"/><w:pgSz w:w="7200" w:h="8800"/><w:pgMar w:top="800" w:right="800" w:bottom="800" w:left="800"/></w:sectPr></w:pPr><w:r><w:t>FIRST SECTION END</w:t></w:r></w:p><w:p><w:r><w:t>SECOND SECTION BODY</w:t></w:r></w:p><w:sectPr><w:type w:val="evenPage"/><w:pgSz w:w="9600" w:h="7200"/><w:pgMar w:top="900" w:right="1000" w:bottom="1100" w:left="1200"/><w:titlePg/></w:sectPr>`
	data := fontSelectionDOCX(t, body, "", "<w:evenAndOddHeaders/>", "", "")
	got, err := ReadDOCX(context.Background(), bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Blocks) != 2 || got.Blocks[0].Kind != SectionBlock || got.Blocks[1].Kind != SectionBlock {
		t.Fatal("foreign sections flattened")
	}
	a, b := got.Blocks[0].Section, got.Blocks[1].Section
	if a.Layout.Width != 360 || b.Layout.Width != 480 || b.Start != SectionEvenPage || !b.FirstPage || !got.EvenAndOddHeaders || len(a.Blocks) != 1 || len(b.Blocks) != 1 {
		t.Fatal("foreign section settings/content lost")
	}
	if b.Blocks[0].Runs[0].Text != "SECOND SECTION BODY" || a.Blocks[0].Runs[0].Text != "FIRST SECTION END" {
		t.Fatal("end properties shifted the body boundary")
	}
	if a.Headers.Default != nil || b.Headers.Even != nil {
		t.Fatal("missing stories fabricated")
	}
	for _, diag := range got.Diagnostics {
		if diag.Code == "multiple_sections" || diag.Code == "docx_running_variants" || strings.Contains(diag.Message, "titlePg") || strings.Contains(diag.Message, "evenAndOddHeaders") {
			t.Fatal("represented settings still diagnosed as loss", diag)
		}
	}
	checkParagraphRoundtrip(t, got)
}

func TestDOCXEmptySectionStoryCannotHideContent(t *testing.T) {
	d := sectionTestDocument(t)
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, DOCX, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(out.Bytes()), int64(out.Len()))
	if err != nil {
		t.Fatal(err)
	}
	var edited bytes.Buffer
	zw := zip.NewWriter(&edited)
	changed := false
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		if strings.HasPrefix(f.Name, "word/header") && bytes.Contains(data, []byte(`md:emptyStory="1"/>`)) {
			data = bytes.Replace(data, []byte(`md:emptyStory="1"/>`), []byte(`md:emptyStory="1"><w:r><w:t>VISIBLE CONTENT</w:t></w:r></w:p>`), 1)
			changed = true
		}
		w, err := zw.Create(f.Name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = w.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("no empty story edited")
	}
	if _, err := ReadDOCX(context.Background(), &edited); err == nil {
		t.Fatal("empty story metadata hid visible content")
	}
}

func TestPDFContinuousStoriesAndSectionCount(t *testing.T) {
	a := &Section{Headers: RunningStories{Default: sectionStory("A DEFAULT"), Even: sectionStory("A EVEN")}, Footers: RunningStories{Default: sectionStory("A FOOT"), Even: sectionStory("A FOOT")}, Blocks: []Block{sectionParagraph("A ONE"), {Kind: PageBreak}, sectionParagraph("A TWO")}}
	b := &Section{Start: SectionContinuous, FirstPage: true, Headers: RunningStories{Default: sectionStory("B DEFAULT"), Even: sectionStory("B EVEN"), First: sectionStory("B FIRST")}, Footers: RunningStories{Default: sectionStory("B FOOT"), Even: sectionStory("B FOOT")}, Blocks: []Block{sectionParagraph("B SHARED"), {Kind: PageBreak}, sectionParagraph("B TWO"), {Kind: PageBreak}, sectionParagraph("B THREE")}}
	c := &Section{Start: SectionContinuous, Headers: RunningStories{Default: sectionStory("C DEFAULT"), Even: sectionStory("C EVEN")}, Footers: RunningStories{Default: sectionStory("C FOOT"), Even: sectionStory("C FOOT")}, Blocks: []Block{sectionParagraph("C SHARED"), {Kind: PageBreak}, sectionParagraph("C TWO")}}
	d := &Document{Layout: Layout{Width: 300, Height: 400, Margin: 40}, EvenAndOddHeaders: true, Blocks: []Block{{Kind: SectionBlock, Section: a}, {Kind: SectionBlock, Section: b}, {Kind: SectionBlock, Section: c}}}
	before := canonicalSF(t, d)
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, PDF, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	back, err := ReadPDF(context.Background(), &out, ReadOptions{PDFSkipInference: true, PDFSkipStructure: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(back.Pages) != 5 {
		t.Fatalf("pages=%d text=%q", len(back.Pages), back.Text())
	}
	texts := make([]string, 5)
	for _, b := range back.Blocks {
		if b.Source != nil && len(b.Source.Regions) > 0 {
			texts[b.Source.Regions[0].Page-1] += (&Document{Blocks: []Block{b}}).Text() + "\n"
		}
	}
	for i, wants := range [][]string{{"A DEFAULT", "A ONE", "A FOOT"}, {"A EVEN", "A TWO", "B SHARED", "A FOOT"}, {"B EVEN", "B TWO", "B FOOT"}, {"B DEFAULT", "B THREE", "C SHARED", "B FOOT"}, {"C EVEN", "C TWO", "C FOOT"}} {
		for _, want := range wants {
			if !strings.Contains(texts[i], want) {
				t.Fatalf("page %d missing %q: %q", i+1, want, texts[i])
			}
		}
		for _, token := range []string{"B FIRST", "C DEFAULT"} {
			if strings.Contains(texts[i], token) {
				t.Fatal("midpage story retroactively selected", texts[i])
			}
		}
	}
	if !bytes.Equal(before, canonicalSF(t, d)) {
		t.Fatal("source mutated")
	}
	checkParagraphRoundtrip(t, d)
}

func TestPDFContinuousStoriesReserveFutureBounds(t *testing.T) {
	a := &Section{Headers: RunningStories{Default: sectionStory("SHORT HEADER")}, Footers: RunningStories{Default: sectionStory("SHORT FOOTER")}, Blocks: []Block{sectionParagraph("FIRST BODY")}}
	b := &Section{Start: SectionContinuous, Headers: RunningStories{Default: &RunningStory{Blocks: []Block{sectionParagraph("LATER ONE"), sectionParagraph("LATER TWO"), sectionParagraph("LATER THREE"), sectionParagraph("LATER FOUR")}}}, Footers: RunningStories{Default: &RunningStory{Blocks: []Block{sectionParagraph("BOTTOM ONE"), sectionParagraph("BOTTOM TWO"), sectionParagraph("BOTTOM THREE")}}}, Blocks: []Block{sectionParagraph("SHARED BODY"), {Kind: PageBreak}, sectionParagraph("LATER BODY")}}
	d := &Document{Layout: Layout{Width: 300, Height: 400, Margin: 30}, Blocks: []Block{{Kind: SectionBlock, Section: a}, {Kind: SectionBlock, Section: b}}}
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, PDF, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	back, err := ReadPDF(context.Background(), &out, ReadOptions{PDFSkipInference: true, PDFSkipStructure: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(back.Pages) != 2 {
		t.Fatal("shared story added an unnecessary page", len(back.Pages))
	}
	top, futureBottom := 0.0, 0.0
	for _, block := range back.Blocks {
		for _, run := range block.Runs {
			if run.Text == "FIRST BODY" {
				top = run.Source.Quad[0].Y
			}
			if run.Text == "LATER FOUR" {
				for _, p := range *run.Source.Quad {
					if p.Y > futureBottom {
						futureBottom = p.Y
					}
				}
			}
		}
	}
	if top <= 30 || futureBottom == 0 || top < futureBottom {
		t.Fatalf("first body at %.3f overlaps later header ending %.3f", top, futureBottom)
	}
	if strings.Count(back.Text(), "SHORT FOOTER") != 1 || strings.Count(back.Text(), "BOTTOM THREE") != 1 {
		t.Fatal("shared footer owner changed", back.Text())
	}
}

func TestPDFContinuousRunningBudgetIncludesLaterBody(t *testing.T) {
	figure := Block{Kind: Picture, Image: &Image{MediaType: "image/png", Data: runningPNG(t, false), Width: 30, Height: 190}}
	a := &Section{Headers: RunningStories{Default: &RunningStory{Blocks: []Block{figure}}}, Blocks: []Block{sectionParagraph("SMALL BODY")}}
	b := &Section{Start: SectionContinuous, Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "LARGE", FontSize: 60}}}}}
	d := &Document{Layout: Layout{Width: 300, Height: 260, Margin: 20}, Blocks: []Block{{Kind: SectionBlock, Section: a}, {Kind: SectionBlock, Section: b}}}
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, PDF, WriteOptions{}); err != nil {
		t.Fatal("later large line must participate in the shared image budget", err)
	}
	back, err := ReadPDF(context.Background(), &out, ReadOptions{PDFSkipInference: true, PDFSkipStructure: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(back.Text(), "SMALL BODY") || !strings.Contains(back.Text(), "LARGE") {
		t.Fatal("shared body content lost", back.Text())
	}
}

func TestPDFSectionNoteMeasurementUsesLocalFrame(t *testing.T) {
	narrow := Layout{Width: 120, Height: 120, Margin: 20}
	wide := Layout{Width: 480, Height: 500, Margin: 40}
	a := &Section{Layout: &narrow, Headers: RunningStories{Default: sectionStory("HEADER")}, Blocks: []Block{sectionParagraph("SMALL BODY")}}
	anchor := sectionParagraph("LARGE BODY")
	anchor.Runs = append(anchor.Runs, Run{Note: &NoteReference{ID: "wide", Kind: Footnote}})
	b := &Section{Layout: &wide, Blocks: []Block{anchor}}
	d := &Document{Blocks: []Block{{Kind: SectionBlock, Section: a}, {Kind: SectionBlock, Section: b}}, Notes: []Note{{ID: "wide", Kind: Footnote, Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "W", FontSize: 100}}}}}}}
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, PDF, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	back, err := ReadPDF(context.Background(), &out, ReadOptions{PDFSkipInference: true, PDFSkipStructure: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(back.Pages) != 2 || back.Pages[0].Width != 120 || back.Pages[1].Width != 480 || !strings.Contains(back.Text(), "W") {
		t.Fatal("unrelated note or previous body frame constrained section", len(back.Pages), back.Text())
	}
}

func TestPDFSectionRepeatedNoteDoesNotRefitInLaterFrame(t *testing.T) {
	wide := Layout{Width: 480, Height: 500, Margin: 40}
	narrow := Layout{Width: 120, Height: 120, Margin: 20}
	anchor := sectionParagraph("ANCHOR")
	anchor.Runs = append(anchor.Runs, Run{Note: &NoteReference{ID: "wide", Kind: Footnote}})
	d := &Document{Blocks: []Block{{Kind: SectionBlock, Section: &Section{Layout: &wide, Blocks: []Block{anchor}}}, {Kind: SectionBlock, Section: &Section{Layout: &narrow, Blocks: []Block{anchor}}}}, Notes: []Note{{ID: "wide", Kind: Footnote, Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "W", FontSize: 100}}}}}}}
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, PDF, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	back, err := ReadPDF(context.Background(), &out, ReadOptions{PDFSkipInference: true, PDFSkipStructure: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(back.Pages) != 2 || strings.Count(back.Text(), "W") != 1 || strings.Count(back.Text(), "ANCHOR") != 2 {
		t.Fatal("cross-reference rematerialized a note", back.Text())
	}
}

func FuzzContinuousStories(f *testing.F) {
	f.Add(byte(0), uint16(360), uint16(440))
	f.Add(byte(7), uint16(300), uint16(400))
	f.Fuzz(func(t *testing.T, flags byte, width, height uint16) {
		a := &Section{FirstPage: flags&1 != 0, Headers: RunningStories{Default: sectionStory("A DEFAULT"), Even: sectionStory("A EVEN"), First: sectionStory("A FIRST")}, Blocks: []Block{sectionParagraph("A ONE"), {Kind: PageBreak}, sectionParagraph("A TWO")}}
		b := &Section{Start: SectionContinuous, FirstPage: flags&2 != 0, Headers: RunningStories{Default: sectionStory("B DEFAULT"), Even: sectionStory("B EVEN"), First: sectionStory("B FIRST")}, Blocks: []Block{sectionParagraph("B SHARED"), {Kind: PageBreak}, sectionParagraph("B TWO")}}
		c := &Section{Start: SectionContinuous, FirstPage: flags&4 != 0, Headers: RunningStories{Default: sectionStory("C DEFAULT"), Even: sectionStory("C EVEN"), First: sectionStory("C FIRST")}, Blocks: []Block{sectionParagraph("C SHARED"), {Kind: PageBreak}, sectionParagraph("C TWO")}}
		d := &Document{Layout: Layout{Width: float64(width%201 + 300), Height: float64(height%201 + 400), Margin: 40}, EvenAndOddHeaders: true, Blocks: []Block{{Kind: SectionBlock, Section: a}, {Kind: SectionBlock, Section: b}, {Kind: SectionBlock, Section: c}}}
		before := canonicalSF(t, d)
		var out bytes.Buffer
		if err := Write(context.Background(), &out, d, PDF, WriteOptions{}); err != nil {
			t.Fatal(err)
		}
		back, err := ReadPDF(context.Background(), &out, ReadOptions{PDFSkipInference: true, PDFSkipStructure: true})
		if err != nil {
			t.Fatal(err)
		}
		if len(back.Pages) != 4 || strings.Count(back.Text(), "B EVEN") != 1 || strings.Count(back.Text(), "C EVEN") != 1 || strings.Contains(back.Text(), "B FIRST") || strings.Contains(back.Text(), "C FIRST") {
			t.Fatal("continuous section page/story ownership changed", back.Text())
		}
		if !bytes.Equal(before, canonicalSF(t, d)) {
			t.Fatal("PDF source changed")
		}
		out.Reset()
		if err := Write(context.Background(), &out, d, DOCX, WriteOptions{}); err != nil {
			t.Fatal(err)
		}
		back, err = ReadDOCX(context.Background(), &out)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(before, canonicalSF(t, back)) {
			t.Fatal("continuous DOCX source semantics changed")
		}
	})
}

func TestPDFEmptyFinalSectionReservesEndnoteBody(t *testing.T) {
	anchor := sectionParagraph("ANCHOR")
	anchor.Runs = append(anchor.Runs, Run{Note: &NoteReference{ID: "ending", Kind: Endnote}})
	figure := Block{Kind: Picture, Image: &Image{MediaType: "image/png", Data: runningPNG(t, false), Width: 30, Height: 210}}
	final := Layout{Width: 300, Height: 300, Margin: 30}
	d := &Document{Layout: Layout{Width: 300, Height: 400, Margin: 30}, Blocks: []Block{{Kind: SectionBlock, Section: &Section{Blocks: []Block{anchor}}}, {Kind: SectionBlock, Section: &Section{Layout: &final, Headers: RunningStories{Default: &RunningStory{Blocks: []Block{figure}}}}}}, Notes: []Note{{ID: "ending", Kind: Endnote, Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "W", FontSize: 70}}}}}}}
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, PDF, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	back, err := ReadPDF(context.Background(), &out, ReadOptions{PDFSkipInference: true, PDFSkipStructure: true})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(back.Text(), "W") != 1 || !strings.Contains(back.Text(), "Endnotes") {
		t.Fatal("empty final section dropped endnote", back.Text())
	}
	for _, page := range back.Pages[1:] {
		if page.Width != 300 || page.Height != 300 {
			t.Fatal("endnote left final section frame", page)
		}
	}
}

func TestPDFContinuousInheritedStoriesRestartParity(t *testing.T) {
	a := &Section{Headers: RunningStories{Default: sectionStory("DEFAULT"), Even: sectionStory("EVEN")}, Blocks: []Block{sectionParagraph("A ONE"), {Kind: PageBreak}, sectionParagraph("A TWO")}}
	b := &Section{Start: SectionContinuous, Blocks: []Block{sectionParagraph("B SHARED"), {Kind: PageBreak}, sectionParagraph("B TWO")}}
	d := &Document{Layout: Layout{Width: 300, Height: 400, Margin: 40}, EvenAndOddHeaders: true, Blocks: []Block{{Kind: SectionBlock, Section: a}, {Kind: SectionBlock, Section: b}}}
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, PDF, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	back, err := ReadPDF(context.Background(), &out, ReadOptions{PDFSkipInference: true, PDFSkipStructure: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(back.Pages) != 3 || strings.Count(back.Text(), "DEFAULT") != 1 || strings.Count(back.Text(), "EVEN") != 2 {
		t.Fatal("identical inherited policy kept the preceding local count", back.Text())
	}
}

func TestPDFContinuousFramesRetainRunningAndNoteOwners(t *testing.T) {
	first := sectionParagraph("OLD BODY")
	first.Runs = append(first.Runs, Run{Note: &NoteReference{ID: "old", Kind: Footnote}})
	second := sectionParagraph("NEW BODY")
	second.Runs = append(second.Runs, Run{Note: &NoteReference{ID: "new", Kind: Footnote}})
	old := &Section{Headers: RunningStories{Default: sectionStory("OLD HEADER")}, Footers: RunningStories{Default: sectionStory("OLD FOOT")}, Blocks: []Block{first}}
	l := Layout{Width: 360, Height: 440, Margins: &PageMargins{Top: 60, Right: 20, Bottom: 70, Left: 100}, HeaderDistance: pagePoint(10), FooterDistance: pagePoint(15)}
	next := &Section{Layout: &l, Start: SectionContinuous, Headers: RunningStories{Default: sectionStory("NEW HEADER")}, Footers: RunningStories{Default: sectionStory("NEW FOOT")}, Blocks: []Block{second, {Kind: PageBreak}, sectionParagraph("NEW SECOND")}}
	blob := runningPNG(t, false)
	figure := func(w float64) Block {
		return Block{Kind: Picture, Image: &Image{MediaType: "image/png", Data: blob, Width: w, Height: 12}}
	}
	rtl := sectionParagraph("SECOND NOTE")
	rtl.Direction = "rtl"
	d := &Document{Layout: Layout{Width: 360, Height: 440, Margin: 40}, Blocks: []Block{{Kind: SectionBlock, Section: old}, {Kind: SectionBlock, Section: next}}, Notes: []Note{{ID: "old", Kind: Footnote, Blocks: []Block{sectionParagraph("FIRST NOTE"), figure(120)}}, {ID: "new", Kind: Footnote, Blocks: []Block{rtl, figure(80)}}}}
	before := canonicalSF(t, d)
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, PDF, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	back, err := ReadPDF(context.Background(), &out, ReadOptions{PDFSkipInference: true, PDFSkipStructure: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(back.Pages) != 2 {
		t.Fatal("same canvas forced a page", len(back.Pages), back.Text())
	}
	positions := map[string]Point{}
	var imageX []float64
	for _, b := range back.Blocks {
		if b.Image != nil {
			imageX = append(imageX, b.Source.Quad[0].X)
		}
		for _, run := range b.Runs {
			if run.Source != nil && run.Source.Quad != nil {
				positions[run.Text] = run.Source.Quad[0]
			}
		}
	}
	for token, want := range map[string]float64{"OLD BODY": 40, "NEW BODY": 100, "NEW SECOND": 100, "OLD HEADER": 40, "NEW HEADER": 100, "OLD FOOT": 40, "NEW FOOT": 100} {
		if math.Abs(positions[token].X-want) > .02 {
			t.Fatal("frame owner changed", token, positions[token])
		}
	}
	if len(imageX) != 2 || math.Abs(imageX[0]-58) > .02 || math.Abs(imageX[1]-100) > .02 {
		t.Fatal("note re-used current body horizontal frame", imageX)
	}
	if !bytes.Equal(before, canonicalSF(t, d)) {
		t.Fatal("source layout changed")
	}
	checkParagraphRoundtrip(t, d)
}

func FuzzContinuousGeometry(f *testing.F) {
	f.Add(byte(0), byte(20), byte(30), uint16(360), uint16(440))
	f.Add(byte(1), byte(80), byte(50), uint16(480), uint16(360))
	image := runningPNG(f, false)
	f.Fuzz(func(t *testing.T, flags, left, right byte, width, height uint16) {
		l := Layout{Width: 360, Height: 440, Margins: &PageMargins{Top: 50, Right: float64(right % 61), Bottom: 60, Left: float64(left % 101)}, HeaderDistance: pagePoint(float64(flags % 21)), FooterDistance: pagePoint(12)}
		if flags&1 != 0 {
			l.Width = float64(width%201 + 300)
			l.Height = float64(height%201 + 350)
		}
		aBody := sectionParagraph("OLD BODY")
		aBody.Runs = append(aBody.Runs, Run{Note: &NoteReference{Kind: Footnote, ID: "old"}})
		bBody := sectionParagraph("NEW BODY")
		bBody.Runs = append(bBody.Runs, Run{Note: &NoteReference{Kind: Footnote, ID: "new"}})
		a := &Section{Headers: RunningStories{Default: sectionStory("OLD HEADER")}, Footers: RunningStories{Default: sectionStory("OLD FOOTER")}, Blocks: []Block{aBody}}
		b := &Section{Start: SectionContinuous, Layout: &l, Headers: RunningStories{Default: sectionStory("NEW HEADER")}, Footers: RunningStories{Default: sectionStory("NEW FOOTER")}, Blocks: []Block{bBody, {Kind: PageBreak}, sectionParagraph("FINAL BODY")}}
		figure := Block{Kind: Picture, Image: &Image{MediaType: "image/png", Data: image, Width: 24, Height: 12}}
		d := &Document{Layout: Layout{Width: 360, Height: 440, Margin: 40}, Blocks: []Block{{Kind: SectionBlock, Section: a}, {Kind: SectionBlock, Section: b}}, Notes: []Note{{ID: "old", Kind: Footnote, Blocks: []Block{sectionParagraph("OLD NOTE"), figure}}, {ID: "new", Kind: Footnote, Blocks: []Block{sectionParagraph("NEW NOTE"), figure}}}}
		before := canonicalSF(t, d)
		var out bytes.Buffer
		if err := Write(context.Background(), &out, d, PDF, WriteOptions{}); err != nil {
			t.Fatal(err)
		}
		back, err := ReadPDF(context.Background(), &out, ReadOptions{PDFSkipInference: true, PDFSkipStructure: true})
		if err != nil {
			t.Fatal(err)
		}
		pages := 2
		if l.Width != 360 || l.Height != 440 {
			pages = 3
		}
		if len(back.Pages) != pages {
			t.Fatalf("paper transition produced %d rather than %d pages", len(back.Pages), pages)
		}
		pictures := 0
		for _, block := range back.Blocks {
			if block.Image != nil {
				want := 58.0
				if pictures == 1 {
					want = l.Margins.Left + 18
				}
				if block.Source == nil || block.Source.Quad == nil || math.Abs(block.Source.Quad[0].X-want) > .03 {
					t.Fatal("note frame not preserved", block.Source)
				}
				pictures++
			}
		}
		if pictures != 2 || strings.Count(back.Text(), "OLD FOOTER") != 1 || !strings.Contains(back.Text(), "FINAL BODY") {
			t.Fatal("changed geometry lost content", back.Text())
		}
		for _, p := range back.Pages[1:] {
			if p.Width != l.Width || p.Height != l.Height {
				t.Fatal("paper settings lost", p)
			}
		}
		if !bytes.Equal(before, canonicalSF(t, d)) {
			t.Fatal("source mutated")
		}
		out.Reset()
		if err := Write(context.Background(), &out, d, DOCX, WriteOptions{}); err != nil {
			t.Fatal(err)
		}
		back, err = ReadDOCX(context.Background(), &out)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(before, canonicalSF(t, back)) {
			t.Fatal("DOCX section semantics changed")
		}
	})
}

func TestPDFContinuousMarginChangeKeepsNoteContinuationsInTheirFrame(t *testing.T) {
	blob := runningPNG(t, false)
	var oldNote []Block
	for i := 0; i < 18; i++ {
		oldNote = append(oldNote, sectionParagraph(fmt.Sprintf("OLD NOTE LINE %02d", i)))
		if i%3 == 0 {
			oldNote = append(oldNote, Block{Kind: Picture, Image: &Image{MediaType: "image/png", Data: blob, Width: 40, Height: 8}})
		}
	}
	aBody := sectionParagraph("OLD ANCHOR")
	aBody.Runs = append(aBody.Runs, Run{Note: &NoteReference{ID: "old", Kind: Footnote}})
	bBody := sectionParagraph("NEW ANCHOR")
	bBody.Runs = append(bBody.Runs, Run{Note: &NoteReference{ID: "new", Kind: Footnote}})
	l := Layout{Width: 360, Height: 440, Margins: &PageMargins{Top: 50, Right: 20, Bottom: 60, Left: 100}}
	d := &Document{Layout: Layout{Width: 360, Height: 440, Margin: 40}, Blocks: []Block{{Kind: SectionBlock, Section: &Section{Blocks: []Block{aBody}}}, {Kind: SectionBlock, Section: &Section{Start: SectionContinuous, Layout: &l, Blocks: []Block{bBody}}}}, Notes: []Note{{ID: "old", Kind: Footnote, Blocks: oldNote}, {ID: "new", Kind: Footnote, Blocks: []Block{sectionParagraph("NEW NOTE"), {Kind: Picture, Image: &Image{MediaType: "image/png", Data: blob, Width: 20, Height: 8}}}}}}
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, PDF, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	back, err := ReadPDF(context.Background(), &out, ReadOptions{PDFSkipInference: true, PDFSkipStructure: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(back.Pages) < 3 {
		t.Fatal("note did not exercise continuation", len(back.Pages))
	}
	oldImages, newImages := 0, 0
	for _, b := range back.Blocks {
		if b.Image == nil {
			continue
		}
		q := b.Source.Quad
		w := q[1].X - q[0].X
		if math.Abs(w-40) < .03 {
			oldImages++
			if math.Abs(q[0].X-58) > .03 {
				t.Fatal("old continuation adopted new margin", q)
			}
		} else if math.Abs(w-20) < .03 {
			newImages++
			if math.Abs(q[0].X-118) > .03 {
				t.Fatal("new note kept old margin", q)
			}
		}
	}
	if oldImages != 6 || newImages != 1 {
		t.Fatal("note images lost", oldImages, newImages)
	}
	for i := 0; i < 18; i++ {
		if strings.Count(back.Text(), fmt.Sprintf("OLD NOTE LINE %02d", i)) != 1 {
			t.Fatal("note text lost or repeated", i)
		}
	}
	if strings.Count(back.Text(), "NEW ANCHOR") != 1 || strings.Count(back.Text(), "NEW NOTE") != 1 {
		t.Fatal("new content lost", back.Text())
	}
}

func TestPDFContinuousMarginsKeepWrappedParagraphAndTableWidth(t *testing.T) {
	a := &Section{Headers: RunningStories{Default: sectionStory("OLD HEADER")}, Footers: RunningStories{Default: sectionStory("OLD FOOT")}, Blocks: []Block{sectionParagraph("OLD BODY")}}
	l := Layout{Width: 360, Height: 440, Margins: &PageMargins{Top: 50, Right: 70, Bottom: 60, Left: 110}}
	b := &Section{Start: SectionContinuous, Layout: &l, Headers: RunningStories{Default: sectionStory("NEW HEADER")}, Footers: RunningStories{Default: sectionStory("NEW FOOT")}}
	for i := 0; i < 12; i++ {
		b.Blocks = append(b.Blocks, sectionParagraph(fmt.Sprintf("PARA-%02d wrapping words must stay inside the new narrower body frame on every continuation page", i)))
	}
	table := Block{Kind: Table, Rows: []Row{{RepeatHeader: true, Cells: []Cell{{ColSpan: 2, Blocks: []Block{sectionParagraph("TABLE HEADER")}}}}}}
	for i := 0; i < 24; i++ {
		table.Rows = append(table.Rows, Row{Cells: []Cell{{Blocks: []Block{sectionParagraph(fmt.Sprintf("ROW-%02d", i))}}, {Blocks: []Block{sectionParagraph(fmt.Sprintf("VALUE-%02d", i))}}}})
	}
	b.Blocks = append(b.Blocks, table)
	d := &Document{Layout: Layout{Width: 360, Height: 440, Margin: 40}, Blocks: []Block{{Kind: SectionBlock, Section: a}, {Kind: SectionBlock, Section: b}}}
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, PDF, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	back, err := ReadPDF(context.Background(), &out, ReadOptions{PDFSkipInference: true, PDFSkipStructure: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(back.Pages) < 4 {
		t.Fatal("did not exercise multi-page paragraphs and table", len(back.Pages))
	}
	for _, block := range back.Blocks {
		for _, run := range block.Runs {
			if run.Source == nil || run.Source.Quad == nil || run.Text == "OLD BODY" || run.Source.FontSize < 10 {
				continue
			}
			for _, p := range *run.Source.Quad {
				if p.X < 109.99 || p.X > 290.01 {
					t.Fatal("continuation reused old body width", run.Text, run.Source)
				}
			}
		}
	}
	for i := 0; i < 12; i++ {
		if strings.Count(back.Text(), fmt.Sprintf("PARA-%02d", i)) != 1 {
			t.Fatal("paragraph lost or repeated", i)
		}
	}
	for i := 0; i < 24; i++ {
		if strings.Count(back.Text(), fmt.Sprintf("ROW-%02d", i)) != 1 || strings.Count(back.Text(), fmt.Sprintf("VALUE-%02d", i)) != 1 {
			t.Fatal("table row lost or repeated", i)
		}
	}
}
func TestPDFSignedRunningBounds(t *testing.T) {
	for _, top := range []float64{-24, 24} {
		for _, bottom := range []float64{-24, 24} {
			r := runningRenderer()
			g := pageLayout(Layout{Width: 300, Height: 400, Margins: &PageMargins{Top: top, Right: 30, Bottom: bottom, Left: 30}, HeaderDistance: pagePoint(12), FooterDistance: pagePoint(12)})
			r.page = &g
			story := []Block{sectionParagraph("ONE"), sectionParagraph("TWO"), sectionParagraph("THREE"), sectionParagraph("FOUR")}
			if err := r.prepareRunning(story, story, true); err != nil {
				t.Fatal(err)
			}
			wantTop, wantBottom := 52.0, 348.0
			if top < 0 {
				wantTop = 24
			}
			if bottom < 0 {
				wantBottom = 376
			}
			if math.Abs(r.bodyTop()-wantTop) > .001 || math.Abs(r.bodyBottom()-wantBottom) > .001 {
				t.Fatal("fixed and minimum edges conflated", top, bottom, r.content)
			}
		}
	}
}
func TestPDFSignedRunningImagesDoNotCompeteWithFixedBody(t *testing.T) {
	image := Block{Kind: Picture, Image: &Image{MediaType: "image/png", Data: runningPNG(t, false), Width: 100, Height: 370}}
	d := &Document{Layout: Layout{Width: 300, Height: 400, Margins: &PageMargins{Top: -24, Right: 30, Bottom: -24, Left: 30}, HeaderDistance: pagePoint(12), FooterDistance: pagePoint(12)}, Headers: []Block{image}, Footers: []Block{image}, Blocks: []Block{sectionParagraph("FIXED BODY")}}
	before := canonicalSF(t, d)
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, PDF, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	back, err := ReadPDF(context.Background(), &out, ReadOptions{PDFSkipInference: true, PDFSkipStructure: true})
	if err != nil {
		t.Fatal(err)
	}
	images := 0
	for _, b := range back.Blocks {
		if b.Image != nil {
			images++
			if math.Abs(b.Image.Height-370) > .01 {
				t.Fatal("fixed running image shrank against body/other story", b.Image.Height)
			}
		}
	}
	if images != 2 || !strings.Contains(back.Text(), "FIXED BODY") {
		t.Fatal("overlapping content discarded", back.Text(), images)
	}
	if !bytes.Equal(before, canonicalSF(t, d)) {
		t.Fatal("source preferences changed")
	}
	checkParagraphRoundtrip(t, d)
}
func TestPDFSignedRunningStillRejectsOutOfCanvasContent(t *testing.T) {
	for _, header := range []bool{true, false} {
		d := &Document{Layout: Layout{Width: 300, Height: 400, Margins: &PageMargins{Top: -24, Right: 30, Bottom: -24, Left: 30}, HeaderDistance: pagePoint(12), FooterDistance: pagePoint(12)}, Blocks: []Block{sectionParagraph("BODY")}}
		story := []Block{sectionParagraph(strings.Repeat("LINE\n", 100))}
		if header {
			d.Headers = story
		} else {
			d.Footers = story
		}
		for _, format := range []Format{DOCX, PDF} {
			var out bytes.Buffer
			if err := Write(context.Background(), &out, d, format, WriteOptions{}); !errors.Is(err, ErrUnsupported) || out.Len() != 0 {
				t.Fatal(format, "out-of-page text must reject atomically", header, err)
			}
		}
	}
}

func TestPDFSignedContinuousOriginsAndPositiveReservations(t *testing.T) {
	story := &RunningStory{Blocks: []Block{sectionParagraph("RUN ONE"), sectionParagraph("RUN TWO"), sectionParagraph("RUN THREE"), sectionParagraph("RUN FOUR")}}
	a := &Section{Headers: RunningStories{Default: story}, Footers: RunningStories{Default: story}, Blocks: []Block{sectionParagraph("FIXED FIRST"), {Kind: PageBreak}, sectionParagraph("FIXED SECOND")}}
	positive := Layout{Width: 300, Height: 400, Margins: &PageMargins{Top: 60, Right: 30, Bottom: 60, Left: 30}, HeaderDistance: pagePoint(12), FooterDistance: pagePoint(12)}
	b := &Section{Start: SectionContinuous, Layout: &positive, Headers: RunningStories{Default: sectionStory("NEW HEADER")}, Footers: RunningStories{Default: sectionStory("NEW FOOTER")}, Blocks: []Block{sectionParagraph("POSITIVE SHARED"), {Kind: PageBreak}, sectionParagraph("POSITIVE SECOND")}}
	d := &Document{Layout: Layout{Width: 300, Height: 400, Margins: &PageMargins{Top: -24, Right: 30, Bottom: -24, Left: 30}, HeaderDistance: pagePoint(12), FooterDistance: pagePoint(12)}, Blocks: []Block{{Kind: SectionBlock, Section: a}, {Kind: SectionBlock, Section: b}}}
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, PDF, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	back, err := ReadPDF(context.Background(), &out, ReadOptions{PDFSkipInference: true, PDFSkipStructure: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(back.Pages) != 3 {
		t.Fatal("shared positive/fixed frames added pages", len(back.Pages))
	}
	found := 0
	for _, block := range back.Blocks {
		for _, run := range block.Runs {
			want := 0.0
			switch run.Text {
			case "FIXED FIRST", "FIXED SECOND":
				want = 35
			case "POSITIVE SHARED", "POSITIVE SECOND":
				want = 71
			}
			if want > 0 {
				found++
				if run.Source == nil || run.Source.Baseline == nil || math.Abs(run.Source.Baseline[0].Y-want) > .01 {
					t.Fatal("shared reservation overrode fixed origin or permitted positive overlap", run.Text, run.Source)
				}
			}
		}
	}
	if found != 4 {
		t.Fatal("body lost", back.Text())
	}
	checkParagraphRoundtrip(t, d)
}
func TestPDFSignedContinuousDrainsOldNoteInterval(t *testing.T) {
	anchor := sectionParagraph("OLD ANCHOR")
	anchor.Runs = append(anchor.Runs, Run{Note: &NoteReference{ID: "old", Kind: Footnote}})
	var note []Block
	for i := 0; i < 32; i++ {
		note = append(note, sectionParagraph("OLD NOTE LINE"))
	}
	a := &Section{Blocks: []Block{anchor}}
	narrower := Layout{Width: 300, Height: 400, Margins: &PageMargins{Top: 120, Right: 30, Bottom: 120, Left: 30}}
	b := &Section{Start: SectionContinuous, Layout: &narrower, FirstPage: true, Headers: RunningStories{First: sectionStory("NEW FIRST")}, Blocks: []Block{sectionParagraph("NEW BODY")}}
	d := &Document{Layout: Layout{Width: 300, Height: 400, Margins: &PageMargins{Top: -24, Right: 30, Bottom: -24, Left: 30}}, Blocks: []Block{{Kind: SectionBlock, Section: a}, {Kind: SectionBlock, Section: b}}, Notes: []Note{{Kind: Footnote, ID: "old", Blocks: note}}}
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, PDF, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	back, err := ReadPDF(context.Background(), &out, ReadOptions{PDFSkipInference: true, PDFSkipStructure: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(back.Pages) < 3 || strings.Count(back.Text(), "OLD NOTE LINE") != 32 || strings.Count(back.Text(), "NEW FIRST") != 1 {
		t.Fatal("old interval was re-used or note content lost", back.Text())
	}
	for _, block := range back.Blocks {
		for _, run := range block.Runs {
			if run.Text == "NEW BODY" {
				if run.Source.Page < 3 || run.Source.Baseline == nil || math.Abs(run.Source.Baseline[0].Y-131) > .01 {
					t.Fatal("new frame did not own fresh page", run.Source)
				}
			}
		}
	}
}

func TestFixedBlankStoryKeepsUnusedOutsideDistance(t *testing.T) {
	l := Layout{Width: 300, Height: 400, Margins: &PageMargins{Top: -24, Right: 30, Bottom: -24, Left: 30}, HeaderDistance: pagePoint(900)}
	d := &Document{Blocks: []Block{{Kind: SectionBlock, Section: &Section{Layout: &l, FirstPage: true, Headers: RunningStories{First: &RunningStory{}}, Blocks: []Block{sectionParagraph("BODY")}}}}}
	checkParagraphRoundtrip(t, d)
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, PDF, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
}

func FuzzSignedRunning(f *testing.F) {
	f.Add(byte(3), uint16(300), uint16(400), uint16(220), uint16(180))
	f.Add(byte(0), uint16(360), uint16(440), uint16(290), uint16(280))
	blob := runningPNG(f, false)
	f.Fuzz(func(t *testing.T, flags byte, width, height, header, footer uint16) {
		w, h := float64(width%201+300), float64(height%201+350)
		top, bottom := float64(flags%41+20), float64((flags/2)%41+20)
		if flags&1 != 0 {
			top = -top
		}
		if flags&2 != 0 {
			bottom = -bottom
		}
		dist := float64(flags%13 + 8)
		head := float64(header%uint16(h-dist-64) + 40)
		foot := float64(footer%uint16(h-dist-64) + 40)
		picture := func(height float64) Block {
			return Block{Kind: Picture, Image: &Image{MediaType: "image/png", Data: blob, Width: 80, Height: height}}
		}
		d := &Document{Layout: Layout{Width: w, Height: h, Margins: &PageMargins{Top: top, Right: 30, Bottom: bottom, Left: 30}, HeaderDistance: pagePoint(dist), FooterDistance: pagePoint(dist)}, Headers: []Block{picture(head)}, Footers: []Block{picture(foot)}, Blocks: []Block{sectionParagraph("BODY")}}
		before := canonicalSF(t, d)
		var out bytes.Buffer
		if err := Write(context.Background(), &out, d, PDF, WriteOptions{}); err != nil {
			t.Fatal(err)
		}
		back, err := ReadPDF(context.Background(), &out, ReadOptions{PDFSkipInference: true, PDFSkipStructure: true})
		if err != nil {
			t.Fatal(err)
		}
		if len(back.Pages) != 1 || !strings.Contains(back.Text(), "BODY") {
			t.Fatal("body missing or paginated unnecessarily", back.Text())
		}
		var images []*Image
		for _, b := range back.Blocks {
			if b.Image != nil {
				images = append(images, b.Image)
			}
		}
		if len(images) != 2 {
			t.Fatal("running content lost", len(images))
		}
		if top < 0 && math.Abs(images[0].Height-head) > .01 || bottom < 0 && math.Abs(images[1].Height-foot) > .01 {
			t.Fatal("fixed story shrank to avoid body overlap", images[0].Height, images[1].Height, head, foot)
		}
		out.Reset()
		if err := Write(context.Background(), &out, d, DOCX, WriteOptions{}); err != nil {
			t.Fatal(err)
		}
		back, err = ReadDOCX(context.Background(), &out)
		if err != nil {
			t.Fatal(err)
		}
		if top < 0 && math.Abs(back.Headers[0].Image.Height-head) > .001 || bottom < 0 && math.Abs(back.Footers[0].Image.Height-foot) > .001 {
			t.Fatal("DOCX fixed story shrank against other anchors")
		}
		if !bytes.Equal(before, canonicalSF(t, d)) {
			t.Fatal("writer mutated source")
		}
	})
}

func TestPDFContinuousIncompatibleStoriesStartFreshPage(t *testing.T) {
	for _, side := range []string{"header", "footer", "both"} {
		for _, reverse := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/reverse=%t", side, reverse), func(t *testing.T) {
				fixed := Layout{Width: 300, Height: 400, Margins: &PageMargins{Top: -24, Right: 30, Bottom: -24, Left: 30}, HeaderDistance: pagePoint(12), FooterDistance: pagePoint(12)}
				minimum := fixed
				minimum.Margins = &PageMargins{Top: 24, Right: 30, Bottom: 24, Left: 30}
				image := Block{Kind: Picture, Image: &Image{MediaType: "image/png", Data: runningPNG(t, false), Width: 100, Height: 370}}
				blank := RunningStories{Default: &RunningStory{}}
				a := &Section{Layout: &fixed, Headers: blank, Footers: blank, Blocks: []Block{sectionParagraph("FIXED BODY")}}
				if side != "footer" {
					a.Headers = RunningStories{Default: &RunningStory{Blocks: []Block{image}}}
				}
				if side != "header" {
					a.Footers = RunningStories{Default: &RunningStory{Blocks: []Block{image}}}
				}
				b := &Section{Layout: &minimum, FirstPage: true, Headers: RunningStories{Default: sectionStory("MIN DEFAULT"), First: sectionStory("MIN FIRST")}, Footers: blank, Blocks: []Block{sectionParagraph("MIN BODY")}}
				first, second := a, b
				if reverse {
					first, second = b, a
				}
				first.Blocks[0].Runs = append(first.Blocks[0].Runs, Run{Note: &NoteReference{Kind: Footnote, ID: "old"}})
				second.Start = SectionContinuous
				third := &Section{Start: SectionContinuous, Layout: second.Layout, Headers: blank, Footers: blank, Blocks: []Block{sectionParagraph("SHARED BODY")}}
				d := &Document{Blocks: []Block{{Kind: SectionBlock, Section: first}, {Kind: SectionBlock, Section: second}, {Kind: SectionBlock, Section: third}}, Notes: []Note{{Kind: Footnote, ID: "old", Blocks: []Block{sectionParagraph("OLD NOTE")}}}}
				before := canonicalSF(t, d)
				var out bytes.Buffer
				if err := Write(context.Background(), &out, d, PDF, WriteOptions{}); err != nil {
					t.Fatal("individually feasible stories must paginate", err)
				}
				back, err := ReadPDF(context.Background(), &out, ReadOptions{PDFSkipInference: true, PDFSkipStructure: true})
				if err != nil {
					t.Fatal(err)
				}
				if len(back.Pages) != 2 {
					t.Fatal("expected fresh incompatible page and compatible successor sharing", len(back.Pages), back.Text())
				}
				for _, text := range []string{"FIXED BODY", "MIN BODY", "SHARED BODY", "OLD NOTE", "MIN FIRST"} {
					if strings.Count(back.Text(), text) != 1 {
						t.Fatal("visible content lost or repeated", text, back.Text())
					}
				}
				images := 0
				for _, block := range back.Blocks {
					if block.Image != nil {
						images++
						if math.Abs(block.Image.Height-370) > .01 {
							t.Fatal("fixed background shrank to share incompatible page", block.Image.Height)
						}
					}
					for _, run := range block.Runs {
						if run.Text == "SHARED BODY" && run.Source.Page != 2 || run.Text == "OLD NOTE" && run.Source.Page != 1 {
							t.Fatal("partition reassigned note/body ownership", run.Text, run.Source)
						}
					}
				}
				wantImages := 1
				if side == "both" {
					wantImages = 2
				}
				if images != wantImages {
					t.Fatal("running asset loss", images)
				}
				if !bytes.Equal(before, canonicalSF(t, d)) {
					t.Fatal("pagination mutated source start or image preferences")
				}
				checkParagraphRoundtrip(t, d)
			})
		}
	}
}

func TestPDFContinuousAdmissionReservesActualBodyFloor(t *testing.T) {
	fixed := Layout{Width: 300, Height: 400, Margins: &PageMargins{Top: -24, Right: 30, Bottom: -24, Left: 30}, HeaderDistance: pagePoint(12), FooterDistance: pagePoint(12)}
	positive := fixed
	positive.Margins = &PageMargins{Top: 24, Right: 30, Bottom: 24, Left: 30}
	large := sectionParagraph("BIG")
	large.Runs[0].FontSize = 40
	d := &Document{Blocks: []Block{
		{Kind: SectionBlock, Section: &Section{Layout: &fixed, Headers: RunningStories{Default: &RunningStory{Blocks: []Block{{Kind: Picture, Image: &Image{MediaType: "image/png", Data: runningPNG(t, false), Width: 100, Height: 340}}}}}, Blocks: []Block{sectionParagraph("FIXED BODY")}}},
		{Kind: SectionBlock, Section: &Section{Layout: &positive, Start: SectionContinuous, Headers: RunningStories{Default: &RunningStory{}}, Blocks: []Block{large}}},
	}}
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, PDF, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	back, err := ReadPDF(context.Background(), &out, ReadOptions{PDFSkipInference: true, PDFSkipStructure: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(back.Pages) != 2 || strings.Count(back.Text(), "BIG") != 1 {
		t.Fatal("positive but insufficient shared frame", back.Text(), len(back.Pages))
	}
}

func FuzzContinuousAdmission(f *testing.F) {
	f.Add(uint16(370), uint16(360), byte(3))
	f.Add(uint16(340), uint16(120), byte(0))
	blob := runningPNG(f, false)
	f.Fuzz(func(t *testing.T, head, foot uint16, flags byte) {
		fixed := Layout{Width: 300, Height: 400, Margins: &PageMargins{Top: -24, Right: 30, Bottom: -24, Left: 30}, HeaderDistance: pagePoint(12), FooterDistance: pagePoint(12)}
		positive := fixed
		positive.Margins = &PageMargins{Top: 24, Right: 30, Bottom: 24, Left: 30}
		picture := func(h uint16) *RunningStory {
			return &RunningStory{Blocks: []Block{{Kind: Picture, Image: &Image{MediaType: "image/png", Data: blob, Width: 100, Height: float64(h%15 + 355)}}}}
		}
		blank := RunningStories{Default: &RunningStory{}}
		a := &Section{Layout: &fixed, Headers: RunningStories{Default: picture(head)}, Footers: RunningStories{Default: picture(foot)}, Blocks: []Block{sectionParagraph("FIXED")}}
		b := &Section{Layout: &positive, Headers: blank, Footers: blank, Blocks: []Block{sectionParagraph("POSITIVE")}}
		b.Blocks[0].Runs[0].FontSize = float64(flags%30 + 11)
		first, second := a, b
		if flags&1 != 0 {
			first, second = b, a
		}
		second.Start = SectionContinuous
		first.Blocks[0].Runs = append(first.Blocks[0].Runs, Run{Note: &NoteReference{Kind: Footnote, ID: "old"}})
		d := &Document{Blocks: []Block{{Kind: SectionBlock, Section: first}, {Kind: SectionBlock, Section: second}}, Notes: []Note{{Kind: Footnote, ID: "old", Blocks: []Block{sectionParagraph("NOTE")}}}}
		before := canonicalSF(t, d)
		var out bytes.Buffer
		if err := Write(context.Background(), &out, d, PDF, WriteOptions{}); err != nil {
			t.Fatal("feasible sections must render", err)
		}
		back, err := ReadPDF(context.Background(), &out, ReadOptions{PDFSkipInference: true, PDFSkipStructure: true})
		if err != nil {
			t.Fatal(err)
		}
		if len(back.Pages) > 2 {
			t.Fatal("unnecessary extra pages", len(back.Pages))
		}
		for _, text := range []string{"FIXED", "POSITIVE", "NOTE"} {
			if strings.Count(back.Text(), text) != 1 {
				t.Fatal("content lost or repeated", text, back.Text())
			}
		}
		images := 0
		for _, b := range back.Blocks {
			if b.Image != nil {
				images++
				if math.Abs(b.Image.Height-float64(head%15+355)) > .01 && math.Abs(b.Image.Height-float64(foot%15+355)) > .01 {
					t.Fatal("fixed story refitted", b.Image.Height)
				}
			}
		}
		if images != 2 {
			t.Fatal("asset placements lost or repeated", images)
		}
		if !bytes.Equal(before, canonicalSF(t, d)) {
			t.Fatal("source mutated")
		}
	})
}
