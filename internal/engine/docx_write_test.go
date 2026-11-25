package engine

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"testing"
)

func runningDocxRoundTrip(t *testing.T, doc *Document) *Document {
	t.Helper()
	before := canonicalSF(t, doc)
	var data bytes.Buffer
	if err := Write(context.Background(), &data, doc, DOCX, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, canonicalSF(t, doc)) {
		t.Fatal("DOCX fitting mutated source SF")
	}
	back, err := ReadDOCX(context.Background(), bytes.NewReader(data.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	return back
}
func runningPictures(bs []Block) []*Image {
	var images []*Image
	for _, b := range bs {
		if b.Image != nil {
			images = append(images, b.Image)
		}
		for _, item := range b.Items {
			images = append(images, runningPictures(item.Blocks)...)
		}
		for _, row := range b.Rows {
			for _, cell := range row.Cells {
				images = append(images, runningPictures(cell.Blocks)...)
			}
		}
	}
	return images
}
func TestDOCXRunningImagesShareBodySpace(t *testing.T) {
	blob := runningPNG(t, true)
	pic := Block{Kind: Picture, Image: &Image{MediaType: "image/png", Data: blob, Height: 1000}}
	d := &Document{Layout: Layout{Width: 220, Height: 220, Margin: 20}, Headers: []Block{pic}, Footers: []Block{pic}, Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "ONLY BODY"}}}}}
	back := runningDocxRoundTrip(t, d)
	for _, im := range append(runningPictures(back.Headers), runningPictures(back.Footers)...) {
		if im.Height < 60 || im.Height > 70 || math.Abs(im.Width/im.Height-.125) > .00001 {
			t.Fatalf("uncontained/over-reduced running picture: %+v", im)
		}
		if !bytes.Equal(im.Data, blob) {
			t.Fatal("changed original media bytes")
		}
	}
	// Both stories must remain real content, not a shared first-only placement.
	if len(runningPictures(back.Headers)) != 1 || len(runningPictures(back.Footers)) != 1 || back.Text() != "ONLY BODY" {
		t.Fatal("lost story or body")
	}
}
func TestDOCXRunningSmallLogoAndBodyImages(t *testing.T) {
	blob := runningPNG(t, true)
	pic := Block{Kind: Picture, Image: &Image{MediaType: "image/png", Data: blob, Height: 1000}}
	logo := Block{Kind: Picture, Image: &Image{MediaType: "image/png", Data: blob, Width: 10, Height: 5}}
	d := &Document{Layout: Layout{Width: 300, Height: 400, Margin: 30}, Headers: []Block{pic}, Footers: []Block{logo}, Blocks: []Block{pic}}
	// An image inside two body-cell ancestors still needs positive physical
	// space after the running stories are reserved.
	d.Blocks = []Block{{Kind: Table, Rows: []Row{{Cells: []Cell{{Blocks: []Block{{Kind: Table, Rows: []Row{{Cells: []Cell{{Blocks: d.Blocks}}}}}}}}}}}}
	back := runningDocxRoundTrip(t, d)
	im := back.Footers[0].Image
	if im.Width != 10 || im.Height != 5 {
		t.Fatal("small preferred logo unnecessarily scaled")
	}
	if runningPictures(back.Blocks)[0].Height >= 100 || back.Headers[0].Image.Height >= 340 {
		t.Fatal("body/running pictures each consumed full body height")
	}
	for _, im := range append(runningPictures(back.Blocks), append(runningPictures(back.Headers), runningPictures(back.Footers)...)...) {
		if !bytes.Equal(im.Data, blob) {
			t.Fatal("changed raster")
		}
	}
}
func TestDOCXRunningMergedNestedAndForeignFont(t *testing.T) {
	blob := runningPNG(t, true)
	pic := Block{Kind: Picture, Image: &Image{MediaType: "image/png", Data: blob, Height: 1000}}
	text := Block{Kind: Paragraph, Runs: []Run{{Text: "अक्षर العربية", FontFamily: "External Display Face", FontSize: 10, Color: "#224466", Link: "https://example.invalid/running"}}}
	inner := Block{Kind: Table, Rows: []Row{{Cells: []Cell{{ColSpan: 2, Blocks: []Block{text, pic, pic, pic}}}}, {Cells: []Cell{{Blocks: []Block{text}}, {Blocks: []Block{text}}}}}}
	outer := Block{Kind: Table, Rows: []Row{{RepeatHeader: true, Cells: []Cell{{Blocks: []Block{inner}}}}}}
	d := &Document{Layout: Layout{Width: 420, Height: 560, Margin: 36}, Headers: []Block{outer}, Footers: []Block{outer}, Blocks: []Block{{Kind: Heading, Level: 1, Runs: []Run{{Text: "BODY", FontSize: 24}}}}}
	back := runningDocxRoundTrip(t, d)
	all := append(runningPictures(back.Headers), runningPictures(back.Footers)...)
	if len(all) != 6 {
		t.Fatal("lost nested picture placements")
	}
	for _, im := range all {
		if im.Height <= 0 || im.Height >= 60 || !bytes.Equal(im.Data, blob) {
			t.Fatal("nested fit/pixels")
		}
	}
	if !back.Headers[0].Rows[0].RepeatHeader || back.Headers[0].Rows[0].Cells[0].Blocks[0].Rows[0].Cells[0].ColSpan != 2 {
		t.Fatal("lost nested/header/merge geometry")
	}
	// Restore only documented physical picture dimensions to compare every other
	// SF field, including relationships/styles and non-PDF-font text, exactly.
	for _, im := range all {
		im.Width, im.Height = 0, 1000
	}
	if !bytes.Equal(canonicalSF(t, d), canonicalSF(t, back)) {
		t.Fatal("changed semantics beyond fitted image dimensions")
	}
}
func TestDOCXRunningImpossibleAtomic(t *testing.T) {
	pic := Block{Kind: Picture, Image: &Image{MediaType: "image/png", Data: runningPNG(t, true), Height: 1000}}
	d := &Document{Layout: Layout{Width: 200, Height: 200, Margin: 20}, Headers: []Block{pic}, Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "BODY", FontSize: 200}}}}}
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, DOCX, WriteOptions{}); !errors.Is(err, ErrUnsupported) || out.Len() != 0 {
		t.Fatalf("impossible fit was not atomic: %v bytes %d", err, out.Len())
	}
}
func FuzzDOCXRunningImages(f *testing.F) {
	f.Add(uint16(1000), uint16(1000), byte(2), byte(1))
	f.Add(uint16(1), uint16(800), byte(0), byte(0))
	blob := runningPNG(f, true)
	f.Fuzz(func(t *testing.T, header, footer uint16, count, nested byte) {
		picture := func(height uint16) Block {
			return Block{Kind: Picture, Image: &Image{MediaType: "image/png", Data: blob, Height: float64(height%2000 + 1)}}
		}
		story := func(height uint16) []Block {
			var bs []Block
			for range int(count%3) + 1 {
				bs = append(bs, picture(height))
			}
			for range int(nested % 3) {
				bs = []Block{{Kind: Table, Rows: []Row{{Cells: []Cell{{Blocks: bs}}}}}}
			}
			return bs
		}
		d := &Document{Layout: Layout{Width: 420, Height: 560, Margin: 36}, Headers: story(header), Footers: story(footer), Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "BODY"}}}}}
		back := runningDocxRoundTrip(t, d)
		images := append(runningPictures(back.Headers), runningPictures(back.Footers)...)
		if len(images) != 2*(int(count%3)+1) {
			t.Fatal("lost images")
		}
		for _, im := range images {
			if im.Height <= 0 || !bytes.Equal(im.Data, blob) {
				t.Fatal(fmt.Sprint("invalid fitted image", im))
			}
		}
	})
}
