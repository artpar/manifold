package engine

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func TestPDFPNGReusePreservesEncodedPixelsAndBytes(t *testing.T) {
	rgba := image.NewNRGBA(image.Rect(0, 0, 23, 31))
	gray := image.NewGray16(image.Rect(0, 0, 13, 17))
	for y := 0; y < 31; y++ {
		for x := 0; x < 23; x++ {
			rgba.SetNRGBA(x, y, color.NRGBA{uint8(x * 9), uint8(y * 7), 33, uint8((x + y) * 3)})
		}
	}
	for y := 0; y < 17; y++ {
		for x := 0; x < 13; x++ {
			gray.SetGray16(x, y, color.Gray16{uint16(x*123 + y*177)})
		}
	}
	palette := image.NewPaletted(image.Rect(0, 0, 7, 5), color.Palette{color.Transparent, color.Black, color.White})
	reader := &pdfReader{}
	for _, im := range []image.Image{rgba, gray, palette, rgba, gray} {
		var want, got bytes.Buffer
		if err := png.Encode(&want, im); err != nil {
			t.Fatal(err)
		}
		if err := reader.encodePNG(&got, im); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got.Bytes(), want.Bytes()) {
			t.Fatal("reuse changed PNG pixels or bytes")
		}
	}
	prior := reader.pngBuffers.buffer
	var large bytes.Buffer
	if err := reader.encodePNG(&large, image.NewGray(image.Rect(0, 0, 8193, 1))); err != nil {
		t.Fatal(err)
	}
	if reader.pngBuffers.buffer != prior {
		t.Fatal("oversized image entered retained pool")
	}
}
