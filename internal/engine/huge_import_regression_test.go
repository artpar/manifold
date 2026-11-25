package engine

import (
	"bytes"
	"context"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"testing"
)

func TestDOCXEmptyThemeRegionUsesExplicitRunScript(t *testing.T) {
	scheme := `<a:majorFont><a:latin typeface="Calibri Light"/><a:cs typeface=""/><a:font script="Arab" typeface="Times New Roman"/></a:majorFont>`
	d := readFontSelection(t, `<w:p>`+fontRun(`<w:rFonts w:asciiTheme="majorBidi"/><w:lang w:val="en-GB" w:bidi="ar-SA"/>`, "if ")+`</w:p>`, "", `<w:themeFontLang w:val="sv-SE"/>`, scheme, "")
	if d.Blocks[0].Runs[0].FontFamily != "Times New Roman" || !hasDiagnostic(d, "font_theme_run_language") {
		t.Fatalf("run language resolution: %+v", d)
	}
}
func TestPDFPartialToUnicodeUsesOnlyDeclaredGlyphNames(t *testing.T) {
	// Test the codepoint layer independently: no undeclared ASCII fallbacks.
	f := pdfFont{mapping: &unicodeMap{spaces: []codeRange{{size: 1, low: 0, high: 255}}, chars: map[string]string{"A": "α"}}, explicitEncoding: map[byte]string{'7': "↦"}}
	text, e := f.decode([]byte("A7"))
	if e != nil || text != "α↦" || !f.fallbackUsed {
		t.Fatalf("partial mapping %q %v", text, e)
	}
	if _, e = f.decode([]byte("Z")); e == nil {
		t.Fatal("undeclared character guessed")
	}
	f.composite = true
	if _, e = f.decode([]byte("7")); e == nil {
		t.Fatal("CID interpreted as simple-font byte")
	}
}
func TestPDFMatteUnblendsBeforeAlpha(t *testing.T) {
	// Red composited over white at 50% alpha is pink, not opaque pink.
	imageObject := recoveryStream("/Type /XObject /Subtype /Image /Width 1 /Height 1 /BitsPerComponent 8 /ColorSpace /DeviceRGB /SMask 7 0 R", string([]byte{255, 127, 127}))
	mask := recoveryStream("/Type /XObject /Subtype /Image /Width 1 /Height 1 /BitsPerComponent 8 /ColorSpace /DeviceGray /Matte [1 1 1]", string([]byte{128}))
	d := readRecovery(t, recoveryPage("q 10 0 0 10 0 0 cm /X Do Q", "", imageObject, mask))
	im, _, e := image.Decode(bytes.NewReader(d.Blocks[0].Image.Data))
	if e != nil {
		t.Fatal(e)
	}
	c := color.NRGBA64Model.Convert(im.At(0, 0)).(color.NRGBA64)
	if c.R != 65535 || c.G > 257 || c.B > 257 || c.A != 128*257 {
		t.Fatalf("preblended edge not recovered: %+v", c)
	}
}
func TestPDFExplicitAndColorKeyMasksKeepSourceSamples(t *testing.T) {
	for _, tc := range []struct {
		name, mask string
		extra      []string
		raw        []byte
	}{
		{"explicit", "7 0 R", []string{recoveryStream("/Type /XObject /Subtype /Image /Width 2 /Height 1 /ImageMask true /BitsPerComponent 1", string([]byte{0x40}))}, []byte{255, 0, 0, 0, 255, 0}},
		// Decode inversion must not change which RAW source samples match the key.
		{"color-key", "[255 255 0 0 0 0]", nil, []byte{255, 0, 0, 0, 255, 0}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			obj := recoveryStream("/Type /XObject /Subtype /Image /Width 2 /Height 1 /BitsPerComponent 8 /ColorSpace /DeviceRGB /Mask "+tc.mask, string(tc.raw))
			objects := append([]string{obj}, tc.extra...)
			d := readRecovery(t, recoveryPage("q 20 0 0 10 0 0 cm /X Do Q", "", objects...))
			im, _, e := image.Decode(bytes.NewReader(d.Blocks[0].Image.Data))
			if e != nil {
				t.Fatal(e)
			}
			_, _, _, a := im.At(0, 0).RGBA()
			_, _, _, b := im.At(1, 0).RGBA()
			if tc.name == "explicit" {
				if a != 65535 || b != 0 {
					t.Fatalf("stencil polarity %d %d", a, b)
				}
			} else if a != 0 || b != 65535 {
				t.Fatalf("raw color-key polarity %d %d", a, b)
			}
		})
	}
}
func TestDOCXSymbolHasExactKnownEncoding(t *testing.T) {
	for code, want := range map[string]string{"F057": "Ω", "F077": "ω", "F061": "α"} {
		got, e := docxSymbolUnicode("Symbol", code)
		if e != nil || got != want {
			t.Fatalf("%s => %q %v", code, got, e)
		}
	}
}

// Explicitly opt in to the downloaded originals; no large fixture is generated
// or committed. Pixel assets are extracted by the same production image path.
func TestHugeOriginalImagePixels(t *testing.T) {
	if os.Getenv("MANIFOLD_HUGE_IMAGES") != "1" {
		t.Skip("requires downloaded original corpus")
	}
	root := filepath.Join("output", "hugetest-real")
	out := filepath.Join("output", "huge-image-fixed")
	if e := os.MkdirAll(out, 0700); e != nil {
		t.Fatal(e)
	}
	for _, tc := range []struct {
		name         string
		object, page int
	}{{"ipcc-ar6-wgi", 1024, 61}, {"ecma376-part1", 9020, 625}, {"etsi-38331-v180100", 99, 41}} {
		t.Run(tc.name, func(t *testing.T) {
			file, e := os.Open(filepath.Join(root, tc.name+".pdf"))
			if e != nil {
				t.Fatal(e)
			}
			defer file.Close()
			p, e := pdfcpu.ReadWithContext(context.Background(), file, &model.Configuration{Reader15: true, ValidationMode: model.ValidationRelaxed, Offline: true})
			if e != nil {
				t.Fatal(e)
			}
			sd, _, e := p.DereferenceStreamDict(types.IndirectRef{ObjectNumber: types.Integer(tc.object)})
			if e != nil || sd == nil {
				t.Fatalf("image source: %v", e)
			}
			r := &pdfReader{ctx: context.Background(), pdf: p, doc: &Document{}, page: tc.page, pageInfo: PDFPage{Transform: Matrix{1, 0, 0, 1, 0, 0}, CropBox: [4]float64{0, 0, 10000, 10000}}}
			if e = r.imageXObject(sd, types.IndirectRef{ObjectNumber: types.Integer(tc.object)}, nil, pdfTextState{ctm: Matrix{1, 0, 0, 1, 0, 0}}); e != nil {
				t.Fatal(e)
			}
			if len(r.lines) != 1 || r.lines[0].Image == nil {
				t.Fatal("original image absent")
			}
			if e = os.WriteFile(filepath.Join(out, tc.name+".png"), r.lines[0].Image.Data, 0600); e != nil {
				t.Fatal(e)
			}
		})
	}
}

func TestDOCXSkippedTableColumnsRetainAlignmentAndDiscloseNormalization(t *testing.T) {
	body := `<w:tbl><w:tblGrid><w:gridCol w:w="1000"/><w:gridCol w:w="1000"/><w:gridCol w:w="1000"/></w:tblGrid><w:tr><w:trPr><w:gridBefore w:val="1"/><w:gridAfter w:val="1"/></w:trPr><w:tc><w:p><w:r><w:t>Middle</w:t></w:r></w:p></w:tc></w:tr></w:tbl>`
	d, e := ReadDOCX(context.Background(), bytes.NewReader(docxWithBody(t, body)))
	if e != nil {
		t.Fatal(e)
	}
	if !hasDiagnostic(d, "table_grid_gaps") {
		t.Fatal("open-edge normalization not disclosed")
	}
	cells := d.Blocks[0].Rows[0].Cells
	if len(cells) != 3 || cells[1].Blocks[0].Runs[0].Text != "Middle" {
		t.Fatalf("skipped columns shifted content: %+v", cells)
	}
	assertRecoverySF(t, d)
}
