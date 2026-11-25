package manifold_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"path"
	"path/filepath"
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
	if f == manifold.PPTX {
		assertPackageRelationships(t, b.Bytes())
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

func TestOriginalPowerPointPackagesSurviveCentralSF(t *testing.T) {
	for _, name := range []string{"poi-SampleShow.pptx", "poi-smartart-simple.pptx"} {
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("testdata", "real", name))
			if err != nil {
				t.Fatal(err)
			}
			d := readFormat(t, data, manifold.PPTX)
			sf := readFormat(t, writeFormat(t, d, manifold.SF), manifold.SF)
			var output bytes.Buffer
			if err := manifold.Write(context.Background(), &output, sf, manifold.PPTX, manifold.WriteOptions{}); err != nil {
				t.Fatal(err)
			}
			before, after := zipParts(t, data), zipParts(t, output.Bytes())
			if len(before) != len(after) {
				t.Fatal("native package parts added/discarded")
			}
			for part, source := range before {
				if !bytes.Equal(source, after[part]) {
					t.Fatalf("native part changed: %s", part)
				}
			}
		})
	}
}
func TestPresentationRejectsAmbiguousOrUnsupportedOutput(t *testing.T) {
	ctx := context.Background()
	t.Run("flow and slides cannot coexist", func(t *testing.T) {
		d := presentationFixture(t)
		d.Blocks = []manifold.Block{{Kind: manifold.Paragraph, Runs: []manifold.Run{{Text: "ambiguous second content owner"}}}}
		if err := d.Validate(); err == nil {
			t.Fatal("ambiguous owners accepted")
		}
	})
	t.Run("overflow leaves output empty", func(t *testing.T) {
		d := presentationFixture(t)
		d.Presentation.Slides[0].Objects[0].Height = 1
		var out bytes.Buffer
		if err := manifold.Write(ctx, &out, d, manifold.PDF, manifold.WriteOptions{AllowLossy: true}); !errors.Is(err, manifold.ErrUnsupported) {
			t.Fatalf("overflow: %v", err)
		}
		if out.Len() != 0 {
			t.Fatal("partial PDF escaped")
		}
	})
	t.Run("unsupported output cannot ignore a shape", func(t *testing.T) {
		d := presentationFixture(t)
		d.Presentation.Slides[0].Objects[0].Unsupported = []string{"unsupported native effect"}
		var out bytes.Buffer
		if err := manifold.Write(ctx, &out, d, manifold.PDF, manifold.WriteOptions{AllowLossy: true}); !errors.Is(err, manifold.ErrUnsupported) {
			t.Fatalf("unsupported feature: %v", err)
		}
	})
	t.Run("speaker notes can be deleted", func(t *testing.T) {
		d := readFormat(t, writeFormat(t, presentationFixture(t), manifold.PPTX), manifold.PPTX)
		d.Presentation.Slides[0].Notes = nil
		back := readFormat(t, writeFormat(t, d, manifold.PPTX), manifold.PPTX)
		if strings.Contains(back.Text(), "Speaker note retained") {
			t.Fatal("deleted notes reappeared")
		}
	})
	t.Run("flow-to-slide mapping requires acknowledgement", func(t *testing.T) {
		d := &manifold.Document{Blocks: []manifold.Block{{Kind: manifold.Paragraph, Runs: []manifold.Run{{Text: "new slide"}}}}}
		var out bytes.Buffer
		if err := manifold.Write(ctx, &out, d, manifold.PPTX, manifold.WriteOptions{}); !errors.Is(err, manifold.ErrLossy) {
			t.Fatal(err)
		}
	})
}

func TestCentralSFEditsRealNativeTextAndAddsSlide(t *testing.T) {
	source, err := os.ReadFile("testdata/real/poi-SampleShow.pptx")
	if err != nil {
		t.Fatal(err)
	}
	d := readFormat(t, source, manifold.PPTX)
	sf := readFormat(t, writeFormat(t, d, manifold.SF), manifold.SF)
	changed := false
	for si := range sf.Presentation.Slides {
		for oi := range sf.Presentation.Slides[si].Objects {
			o := &sf.Presentation.Slides[si].Objects[oi]
			if o.Kind == "shape" && len(o.Content) > 0 && len(o.Content[0].Runs) > 0 && o.Content[0].Runs[0].Text != "" {
				o.Content[0].Runs[0].Text = "Edited through central SF"
				changed = true
				break
			}
		}
		if changed {
			break
		}
	}
	if !changed {
		t.Fatal("source has no editable native text")
	}
	encoded := writeFormat(t, sf, manifold.PPTX)
	edited := readFormat(t, encoded, manifold.PPTX)
	if !strings.Contains(edited.Text(), "Edited through central SF") {
		t.Fatal("source-native edit ignored")
	}
	originalParts, after := zipParts(t, source), zipParts(t, encoded)
	for part, data := range originalParts {
		if strings.Contains(part, "/slides/") && !strings.Contains(part, "/_rels/") {
			continue
		}
		if !bytes.Equal(data, after[part]) {
			t.Fatalf("text edit modified unrelated source part %s", part)
		}
	}
	sf.Presentation.Slides = append(sf.Presentation.Slides, manifold.PresentationSlide{ID: "9999", Objects: []manifold.SlideObject{{ID: "2", Kind: "shape", Geometry: "rect", X: 30, Y: 30, Width: 400, Height: 100, Content: []manifold.Block{{Kind: manifold.Paragraph, Runs: []manifold.Run{{Text: "Added canonical slide"}}}}}}})
	added := readFormat(t, writeFormat(t, sf, manifold.PPTX), manifold.PPTX)
	if len(added.Presentation.Slides) != len(d.Presentation.Slides)+1 || !strings.Contains(added.Text(), "Added canonical slide") {
		t.Fatal("canonical slide addition failed")
	}
}

func TestPresentationSFRequiresExplicitFeatureMarker(t *testing.T) {
	sf, err := manifold.ToStructured(presentationFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(sf.Content) != 1 || sf.Content[0].Type != "presentation" {
		t.Fatal("missing feature marker")
	}
	sf.Content = nil
	if _, err := manifold.FromStructured(sf); err == nil {
		t.Fatal("old-reader-droppable presentation admitted")
	}
}

func TestPPTXInputDoesNotAcceptWordPackageAsEmptyDeck(t *testing.T) {
	data, err := os.ReadFile("testdata/corpus/docx_basic.docx")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manifold.Read(context.Background(), bytes.NewReader(data), manifold.PPTX, manifold.ReadOptions{}); err == nil {
		t.Fatal("Word input masqueraded as an empty PPTX")
	}
}
func TestHiddenSlideContentRemainsHidden(t *testing.T) {
	d := presentationFixture(t)
	d.Presentation.Slides[0].Objects[0].Hidden = true
	d.Presentation.Slides[1].Hidden = true
	back := readFormat(t, writeFormat(t, d, manifold.PPTX), manifold.PPTX)
	if strings.Contains(back.Text(), "Hello SF.") || strings.Contains(back.Text(), "Second slide text") {
		t.Fatal("hidden content became visible")
	}
	pdf := readFormat(t, writeFormat(t, back, manifold.PDF), manifold.PDF)
	if len(pdf.Pages) != 1 || strings.Contains(pdf.Text(), "Hello SF.") {
		t.Fatal("hidden content painted into PDF")
	}
}

// Independent ZIP/XML graph check: native IDs must be unique and every internal
// relationship target must actually exist. This does not use Manifold's parser.
func assertPackageRelationships(t *testing.T, data []byte) {
	t.Helper()
	parts := zipParts(t, data)
	var types struct {
		Overrides []struct {
			Name string `xml:"PartName,attr"`
			Type string `xml:"ContentType,attr"`
		} `xml:"Override"`
	}
	if err := xml.Unmarshal(parts["[Content_Types].xml"], &types); err != nil {
		t.Fatal(err)
	}
	foundMain := false
	for _, entry := range types.Overrides {
		if entry.Name == "/ppt/presentation.xml" {
			foundMain = true
			if entry.Type != "application/vnd.openxmlformats-officedocument.presentationml.presentation.main+xml" {
				t.Fatalf("wrong presentation MIME type: %s", entry.Type)
			}
		}
	}
	if !foundMain {
		t.Fatal("missing main presentation content type")
	}
	for name, raw := range parts {
		if !strings.HasSuffix(name, ".rels") {
			continue
		}
		var root struct {
			Relationships []struct {
				ID     string `xml:"Id,attr"`
				Target string `xml:"Target,attr"`
				Mode   string `xml:"TargetMode,attr"`
			} `xml:"Relationship"`
		}
		if err := xml.Unmarshal(raw, &root); err != nil {
			t.Fatal(err)
		}
		ids := map[string]bool{}
		source := ""
		if name != "_rels/.rels" {
			source = path.Join(path.Dir(path.Dir(name)), strings.TrimSuffix(path.Base(name), ".rels"))
		}
		for _, r := range root.Relationships {
			if r.ID == "" || ids[r.ID] {
				t.Fatalf("duplicate/empty relationship in %s: %s", name, r.ID)
			}
			ids[r.ID] = true
			if r.Mode == "External" {
				continue
			}
			target := path.Clean(path.Join(path.Dir(source), r.Target))
			if strings.HasPrefix(r.Target, "/") {
				target = strings.TrimPrefix(r.Target, "/")
			}
			if _, exists := parts[target]; !exists {
				t.Fatalf("missing target %s -> %s", name, target)
			}
		}
	}
}
