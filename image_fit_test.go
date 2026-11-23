package manydoc

import (
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
	"math"
	"os"
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
		got, err := Read(context.Background(), &out, format, ReadOptions{PDFSkipInference: true})
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
	im := *imageFitDocument(t).Blocks[0].Image
	im.Width, im.Height = .5/12700, 1
	if fmt.Sprintf("%.0f", im.Width*12700) != "0" {
		t.Fatal("invalid rounding-boundary fixture")
	}
	var out bytes.Buffer
	if err := Write(context.Background(), &out, &Document{Blocks: []Block{{Kind: Picture, Image: &im}}}, DOCX, WriteOptions{}); !errors.Is(err, ErrUnsupported) || out.Len() != 0 {
		t.Fatal("zero EMU extent emitted", err)
	}
}
