package manifold

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"math"
	"reflect"
	"strings"
	"testing"
)

func runningPNG(t testing.TB, tall bool) []byte {
	t.Helper()
	w, h := 32, 16
	if tall {
		w, h = 16, 128
	}
	im := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := color.NRGBA{200, 30, 50, 255}
			if x >= w/2 {
				c = color.NRGBA{20, 80, 220, 128}
			}
			im.SetNRGBA(x, y, c)
		}
	}
	var out bytes.Buffer
	if err := png.Encode(&out, im); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}
func runningRenderer() *pdfRenderer {
	r := textTestRenderer()
	r.width, r.height, r.margin = 300, 400, 30
	return r
}
func TestPDFRichRunningMeasuredAndOriginalModel(t *testing.T) {
	data := runningPNG(t, false)
	p := func(text string) Block { return Block{Kind: Paragraph, Runs: []Run{{Text: text}}} }
	imageBlock := Block{Kind: Picture, Image: &Image{MediaType: "image/png", Data: data, Width: 80, Height: 40}}
	list := Block{Kind: ListBlock, List: "bullet", Items: []ListItem{{Blocks: []Block{p("ITEM")}}}}
	header := Block{Kind: Table, Rows: []Row{
		{RepeatHeader: true, Cells: []Cell{{ColSpan: 2, Blocks: []Block{p("HEADER")}}}},
		{Cells: []Cell{{Blocks: []Block{imageBlock}}, {Blocks: []Block{list}}}},
	}}
	footer := []Block{p("FOOTER"), {Kind: Picture, Image: &Image{MediaType: "image/png", Data: data, Width: 40, Height: 20}}}
	doc := &Document{Layout: Layout{Width: 300, Height: 400, Margin: 30}, Headers: []Block{header}, Footers: footer, Blocks: []Block{p("BODY"), {Kind: PageBreak}, p("BODY2")}}
	original, err := ToStructured(doc)
	if err != nil {
		t.Fatal(err)
	}
	r := runningRenderer()
	if err := r.prepareRunning(doc.Headers, doc.Footers, true); err != nil {
		t.Fatal(err)
	}
	if r.bodyTop() <= 30 || r.bodyBottom() >= 370 || r.header == nil || r.footer == nil {
		t.Fatalf("running bounds %+v", r.content)
	}
	if math.Abs(r.header.y+r.header.height-r.bodyTop()) > 1e-7 || math.Abs(r.footer.y-r.bodyBottom()) > 1e-7 {
		t.Fatal("running/body overlap")
	}
	after, err := ToStructured(doc)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(original, after) || !doc.Headers[0].Rows[0].RepeatHeader {
		t.Fatal("running measurement mutated canonical content")
	}
	var pdf bytes.Buffer
	if err := Write(context.Background(), &pdf, doc, PDF, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	back, err := ReadPDF(context.Background(), bytes.NewReader(pdf.Bytes()), ReadOptions{PDFSkipInference: true, PDFSkipStructure: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(back.Pages) != 2 || strings.Count(back.Text(), "HEADER") != 2 || strings.Count(back.Text(), "FOOTER") != 2 || strings.Count(back.Text(), "ITEM") != 2 {
		t.Fatalf("running repetition %q", back.Text())
	}
	images := 0
	for _, b := range back.Blocks {
		if b.Image != nil {
			images++
			sameImagePixels(t, data, b.Image.Data)
		}
	}
	if images != 4 {
		t.Fatalf("running image placements %d", images)
	}
	assertRecoverySF(t, back)
	var word bytes.Buffer
	if err := Write(context.Background(), &word, doc, DOCX, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	recovered, err := ReadDOCX(context.Background(), bytes.NewReader(word.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	sf, err := ToStructured(recovered)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(original, sf) {
		t.Fatal("rich running SF/DOCX semantics changed")
	}
}
func TestPDFRunningImagesShareAvailableHeight(t *testing.T) {
	data := runningPNG(t, true)
	imageBlock := Block{Kind: Picture, Image: &Image{MediaType: "image/png", Data: data, Height: 1000}}
	d := &Document{Layout: Layout{Width: 220, Height: 220, Margin: 20}, Headers: []Block{imageBlock}, Footers: []Block{imageBlock}, Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "ONLY BODY"}}}}}
	r := runningRenderer()
	r.width, r.height, r.margin = 220, 220, 20
	if err := r.prepareRunning(d.Headers, d.Footers, true); err != nil {
		t.Fatal(err)
	}
	if r.bodyHeight() < 15.4-1e-7 || r.header.height >= 180 || r.footer.height >= 180 {
		t.Fatalf("image budgets %+v %+v", r.header, r.footer)
	}
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, PDF, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	back, err := ReadPDF(context.Background(), bytes.NewReader(out.Bytes()), ReadOptions{PDFSkipInference: true, PDFSkipStructure: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(back.Pages) != 1 || strings.Count(back.Text(), "ONLY BODY") != 1 {
		t.Fatalf("fitted body %q", back.Text())
	}
	for _, b := range back.Blocks {
		if b.Image != nil {
			sameImagePixels(t, data, b.Image.Data)
		}
	}
	assertRecoverySF(t, back)
}
func TestPDFRunningBreakAndOversizeAtomic(t *testing.T) {
	for _, blocks := range [][]Block{{{Kind: PageBreak}}, {{Kind: Paragraph, PageBreakBefore: true}}, {{Kind: Table, Rows: []Row{{Cells: []Cell{{Blocks: []Block{{Kind: PageBreak}}}}}}}}, {{Kind: Paragraph, Runs: []Run{{Text: strings.Repeat("LINE\n", 100)}}}}} {
		for _, footer := range []bool{false, true} {
			d := &Document{Layout: Layout{Width: 200, Height: 200, Margin: 20}, Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "BODY"}}}}}
			if footer {
				d.Footers = blocks
			} else {
				d.Headers = blocks
			}
			var out bytes.Buffer
			if err := Write(context.Background(), &out, d, PDF, WriteOptions{}); !errors.Is(err, ErrUnsupported) || out.Len() != 0 {
				t.Fatalf("running unsupported not atomic: %v bytes %d", err, out.Len())
			}
		}
	}
}
func TestPDFRunningLegacySmallTextPositions(t *testing.T) {
	r := runningRenderer()
	p := Block{Kind: Paragraph, Runs: []Run{{Text: "SMALL"}}}
	if err := r.prepareRunning([]Block{p}, []Block{p}, true); err != nil {
		t.Fatal(err)
	}
	if r.header.y != 15 || r.footer.y != 385 || r.bodyTop() != 30 || r.bodyBottom() != 370 || r.header.flow[0].TextSize != 8 {
		t.Fatal("legacy small running geometry changed")
	}
}
func FuzzPDFRunningRegions(f *testing.F) {
	f.Add(uint16(80), uint16(60), byte(1))
	f.Add(uint16(1000), uint16(1000), byte(2))
	data := runningPNG(f, true)
	f.Fuzz(func(t *testing.T, header, footer uint16, n byte) {
		if n > 4 {
			t.Skip()
		}
		picture := func(h uint16) Block {
			return Block{Kind: Picture, Image: &Image{MediaType: "image/png", Data: data, Height: float64(h%2000 + 1)}}
		}
		d := &Document{Layout: Layout{Width: 240, Height: 300, Margin: 25}, Headers: []Block{picture(header)}, Footers: []Block{picture(footer)}}
		for range int(n) + 1 {
			d.Blocks = append(d.Blocks, Block{Kind: Paragraph, Runs: []Run{{Text: "BODY"}}})
		}
		var out bytes.Buffer
		if err := Write(context.Background(), &out, d, PDF, WriteOptions{}); err != nil {
			t.Fatalf("valid running images/body must fit: %v (bytes %d)", err, out.Len())
		}
		back, err := ReadPDF(context.Background(), bytes.NewReader(out.Bytes()), ReadOptions{PDFSkipInference: true, PDFSkipStructure: true})
		if err != nil {
			t.Fatal(err)
		}
		if strings.Count(back.Text(), "BODY") != int(n)+1 {
			t.Fatal("running lost body content")
		}
		assertRecoverySF(t, back)
	})
}

func TestPDFRunningImagesReserveIndivisibleBody(t *testing.T) {
	data := runningPNG(t, true)
	picture := Block{Kind: Picture, Image: &Image{MediaType: "image/png", Data: data, Height: 1000}}
	for _, name := range []string{"large-text", "heading", "table", "footnote"} {
		t.Run(name, func(t *testing.T) {
			p := Block{Kind: Paragraph, Runs: []Run{{Text: "BODY", FontSize: 24}}}
			d := &Document{Layout: Layout{Width: 240, Height: 300, Margin: 25}, Headers: []Block{picture}, Footers: []Block{picture}, Blocks: []Block{p}}
			switch name {
			case "heading":
				d.Blocks[0].Kind, d.Blocks[0].Level, d.Blocks[0].Runs[0].FontSize = Heading, 1, 0
			case "table":
				d.Blocks = []Block{{Kind: Table, Rows: []Row{{RepeatHeader: true, Cells: []Cell{{Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "HEAD"}}}}}}}, {Cells: []Cell{{Blocks: []Block{p}}}}}}}
			case "footnote":
				d.Blocks[0].Runs = append(d.Blocks[0].Runs, Run{Note: &NoteReference{Kind: Footnote, ID: "note"}})
				d.Notes = []Note{{Kind: Footnote, ID: "note", Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "NOTE", FontSize: 20}}}}}}
			}
			before, err := ToStructured(d)
			if err != nil {
				t.Fatal(err)
			}
			var out bytes.Buffer
			if err := Write(context.Background(), &out, d, PDF, WriteOptions{}); err != nil {
				t.Fatal(err)
			}
			back, err := ReadPDF(context.Background(), bytes.NewReader(out.Bytes()), ReadOptions{PDFSkipInference: true, PDFSkipStructure: true})
			if err != nil {
				t.Fatal(err)
			}
			if len(back.Pages) != 1 || strings.Count(back.Text(), "BODY") != 1 {
				t.Fatalf("body admission %q (%d pages)", back.Text(), len(back.Pages))
			}
			if name == "footnote" && strings.Count(back.Text(), "NOTE") != 1 {
				t.Fatal("lost note")
			}
			after, err := ToStructured(d)
			if err != nil || !reflect.DeepEqual(before, after) {
				t.Fatalf("measurement mutated source: %v", err)
			}
			assertRecoverySF(t, back)
		})
	}
}

func TestPDFRunningFittedFloorPagination(t *testing.T) {
	// Two fitted story edges used to leave a body height a few ulps below the
	// measured 15.4pt line. Rejecting this feasible case hid behind ErrUnsupported
	// acceptance in the initial fuzz harness; require actual output instead.
	data := runningPNG(t, true)
	picture := Block{Kind: Picture, Image: &Image{MediaType: "image/png", Data: data, Height: 1001}}
	d := &Document{Layout: Layout{Width: 240, Height: 300, Margin: 25}, Headers: []Block{picture}, Footers: []Block{picture}}
	for range 3 {
		d.Blocks = append(d.Blocks, Block{Kind: Paragraph, Runs: []Run{{Text: "BODY"}}})
	}
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, PDF, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	back, err := ReadPDF(context.Background(), bytes.NewReader(out.Bytes()), ReadOptions{PDFSkipInference: true, PDFSkipStructure: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(back.Pages) != 3 || strings.Count(back.Text(), "BODY") != 3 {
		t.Fatalf("fitted floor pagination: %d pages %q", len(back.Pages), back.Text())
	}
	images := 0
	for _, b := range back.Blocks {
		if b.Image != nil {
			images++
			sameImagePixels(t, data, b.Image.Data)
		}
	}
	if images != 6 {
		t.Fatalf("running placements: %d", images)
	}
	assertRecoverySF(t, back)
}

func TestPDFRunningSerialNestedImagesFit(t *testing.T) {
	data := runningPNG(t, true)
	for _, nested := range []bool{false, true} {
		picture := Block{Kind: Picture, Image: &Image{MediaType: "image/png", Data: data, Height: 1000}}
		story := []Block{picture, picture, picture}
		if nested {
			story = []Block{{Kind: Table, Rows: []Row{{Cells: []Cell{{Blocks: []Block{{Kind: Table, Rows: []Row{{Cells: []Cell{{Blocks: story}}}}}}}}}}}}
		}
		d := &Document{Layout: Layout{Width: 300, Height: 400, Margin: 30}, Headers: story, Footers: story, Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "BODY"}}}}}
		before, err := ToStructured(d)
		if err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		if err := Write(context.Background(), &out, d, PDF, WriteOptions{}); err != nil {
			t.Fatalf("nested=%v: %v", nested, err)
		}
		back, err := ReadPDF(context.Background(), bytes.NewReader(out.Bytes()), ReadOptions{PDFSkipInference: true, PDFSkipStructure: true})
		if err != nil {
			t.Fatal(err)
		}
		images := 0
		for _, b := range back.Blocks {
			if b.Image != nil {
				images++
				sameImagePixels(t, data, b.Image.Data)
			}
		}
		if len(back.Pages) != 1 || images != 6 || strings.Count(back.Text(), "BODY") != 1 {
			t.Fatalf("nested=%v: pages=%d images=%d text=%q", nested, len(back.Pages), images, back.Text())
		}
		after, err := ToStructured(d)
		if err != nil || !reflect.DeepEqual(before, after) {
			t.Fatal("mutated source")
		}
		assertRecoverySF(t, back)
	}
}
