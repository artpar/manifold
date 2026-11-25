package manifold_test

import (
	"archive/zip"
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"io"
	"strings"
	"testing"

	"github.com/artpar/manifold"
)

func presentationFixture(t *testing.T) *manifold.Document {
	t.Helper()
	im := image.NewRGBA(image.Rect(0, 0, 16, 16))
	for y := range 16 {
		for x := range 16 {
			im.SetRGBA(x, y, color.RGBA{R: 200, G: uint8(x * 10), B: uint8(y * 10), A: 255})
		}
	}
	var pixels bytes.Buffer
	if err := png.Encode(&pixels, im); err != nil {
		t.Fatal(err)
	}
	return &manifold.Document{Metadata: manifold.Metadata{Title: "SF presentation bridge", Author: "Manifold tests"}, Presentation: &manifold.Presentation{Version: 1, Width: 720, Height: 540, Slides: []manifold.PresentationSlide{
		{ID: "256", Background: "#F0F0F0", Objects: []manifold.SlideObject{
			{ID: "2", Kind: "shape", Name: "Title", X: 40, Y: 40, Width: 500, Height: 70, Geometry: "rect", Content: []manifold.Block{{Kind: manifold.Paragraph, Runs: []manifold.Run{{Text: "Hello SF. ", FontSize: 18, Color: "#123456"}, {Text: "Bold Ελληνικά", Bold: true, Italic: true, FontSize: 18}}}}},
			{ID: "3", Kind: "image", X: 40, Y: 180, Width: 100, Height: 100, Image: &manifold.Image{Data: pixels.Bytes(), MediaType: "image/png", Alt: "colored raster", Width: 100, Height: 100}},
			{ID: "4", Kind: "shape", Geometry: "ellipse", X: 300, Y: 180, Width: 120, Height: 70, Fill: "#00AA88", Stroke: "#000000", StrokeWidth: 2},
		}, Notes: []manifold.Block{{Kind: manifold.Paragraph, Runs: []manifold.Run{{Text: "Speaker note retained"}}}}},
		{ID: "257", Objects: []manifold.SlideObject{{ID: "2", Kind: "shape", Name: "Second", X: 40, Y: 40, Width: 500, Height: 100, Geometry: "rect", Rotation: 7, Content: []manifold.Block{{Kind: manifold.Paragraph, Runs: []manifold.Run{{Text: "Second slide text", FontSize: 18}}}}}}},
	}}}
}
func writeFormat(t *testing.T, d *manifold.Document, f manifold.Format) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := manifold.Write(context.Background(), &b, d, f, manifold.WriteOptions{AllowLossy: true}); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
func readFormat(t *testing.T, data []byte, f manifold.Format) *manifold.Document {
	t.Helper()
	d, err := manifold.Read(context.Background(), bytes.NewReader(data), f, manifold.ReadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return d
}
func zipParts(t *testing.T, data []byte) map[string][]byte {
	t.Helper()
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	parts := map[string][]byte{}
	for _, f := range z.File {
		r, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		b, err := io.ReadAll(r)
		r.Close()
		if err != nil {
			t.Fatal(err)
		}
		parts[f.Name] = b
	}
	return parts
}
func TestPresentationPPTXSFEditsAndNativePreservation(t *testing.T) {
	original := writeFormat(t, presentationFixture(t), manifold.PPTX)
	d := readFormat(t, original, manifold.PPTX)
	if d.Presentation == nil || len(d.Presentation.Slides) != 2 {
		t.Fatal("PPTX did not enter shared model")
	}
	if !strings.Contains(d.Text(), "Speaker note retained") || !strings.Contains(d.Text(), "Hello SF.") {
		t.Fatal(d.Text())
	}
	for _, f := range []manifold.Format{manifold.SF, manifold.SFGzip, manifold.JSON} {
		back := readFormat(t, writeFormat(t, d, f), f)
		if back.Presentation.Slides[0].Objects[0].X != 40 {
			t.Fatal("geometry lost")
		}
		rewritten := writeFormat(t, back, manifold.PPTX)
		a, b := zipParts(t, original), zipParts(t, rewritten)
		if len(a) != len(b) {
			t.Fatalf("%s native part count changed: %d/%d", f, len(a), len(b))
		}
		for name, data := range a {
			if !bytes.Equal(data, b[name]) {
				t.Fatalf("%s changed untouched native part %s", f, name)
			}
		}
	}
	sf := readFormat(t, writeFormat(t, d, manifold.SF), manifold.SF)
	obj := &sf.Presentation.Slides[0].Objects[0]
	obj.X = 88
	obj.Content[0].Runs[0].Text = "Edited through central SF. "
	edited := readFormat(t, writeFormat(t, sf, manifold.PPTX), manifold.PPTX)
	if edited.Presentation.Slides[0].Objects[0].X != 88 || !strings.Contains(edited.Text(), "Edited through central SF.") {
		t.Fatalf("SF edits ignored: %+v", edited.Presentation.Slides[0])
	}
	if !bytes.Equal(edited.Presentation.Slides[0].Objects[1].Image.Data, d.Presentation.Slides[0].Objects[1].Image.Data) {
		t.Fatal("image payload changed")
	}
}
func TestPresentationPPTXSFPDFAndPDFPPTX(t *testing.T) {
	d := readFormat(t, writeFormat(t, presentationFixture(t), manifold.PPTX), manifold.PPTX)
	sf := readFormat(t, writeFormat(t, d, manifold.SF), manifold.SF)
	pdf := writeFormat(t, sf, manifold.PDF)
	back := readFormat(t, pdf, manifold.PDF)
	if len(back.Pages) != 2 {
		t.Fatalf("page count %d", len(back.Pages))
	}
	if !strings.Contains(back.Text(), "Hello SF.") || !strings.Contains(back.Text(), "Ελληνικά") || !strings.Contains(back.Text(), "Second slide text") {
		t.Fatal(back.Text())
	}
	restored := readFormat(t, writeFormat(t, back, manifold.PPTX), manifold.PPTX)
	if len(restored.Presentation.Slides) != 2 || !strings.Contains(restored.Text(), "Hello SF.") || !strings.Contains(restored.Text(), "Second slide text") {
		t.Fatal(restored.Text())
	}
}
func TestFlowSFCanCreatePPTX(t *testing.T) {
	d := &manifold.Document{Blocks: []manifold.Block{{Kind: manifold.Paragraph, Runs: []manifold.Run{{Text: "Flowing SF into slides"}}}, {Kind: manifold.PageBreak}, {Kind: manifold.Paragraph, Runs: []manifold.Run{{Text: "Next page"}}}}}
	back := readFormat(t, writeFormat(t, d, manifold.PPTX), manifold.PPTX)
	if len(back.Presentation.Slides) != 2 || !strings.Contains(back.Text(), "Flowing SF into slides") {
		t.Fatal(back.Text())
	}
}
