package manifold

import (
	"bytes"
	"compress/zlib"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

func TestPDFImageSampleDepths(t *testing.T) {
	for _, tc := range []struct {
		name, cs    string
		bits, width int
		data        []byte
		decode      string
		want        []color.NRGBA64
	}{
		{"gray1", "DeviceGray", 1, 3, []byte{0xa0}, "", []color.NRGBA64{{65535, 65535, 65535, 65535}, {0, 0, 0, 65535}, {65535, 65535, 65535, 65535}}},
		{"gray2", "DeviceGray", 2, 3, []byte{0x1c}, "", []color.NRGBA64{{0, 0, 0, 65535}, {21845, 21845, 21845, 65535}, {65535, 65535, 65535, 65535}}},
		{"rgb4", "DeviceRGB", 4, 2, []byte{0xf0, 0x00, 0xf0}, "", []color.NRGBA64{{65535, 0, 0, 65535}, {0, 65535, 0, 65535}}},
		{"rgb8decode", "DeviceRGB", 8, 1, []byte{255, 0, 255}, "/Decode [1 0 1 0 1 0]", []color.NRGBA64{{0, 65535, 0, 65535}}},
		{"rgb16", "DeviceRGB", 16, 1, []byte{0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc}, "", []color.NRGBA64{{0x1234, 0x5678, 0x9abc, 65535}}},
		{"gray16decode", "DeviceGray", 16, 1, []byte{0x12, 0x34}, "/Decode [1 0]", []color.NRGBA64{{0xedcb, 0xedcb, 0xedcb, 65535}}},
		{"cmyk4", "DeviceCMYK", 4, 1, []byte{0x0f, 0xf0}, "", []color.NRGBA64{{65535, 0, 0, 65535}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, compressed := range []bool{false, true} {
				data := tc.data
				filter := ""
				suffix := "raw"
				if compressed {
					data = syntaxZlib(t, data)
					filter = "/Filter /FlateDecode"
					suffix = "flate"
				}
				object := recoveryStream(fmt.Sprintf("/Type /XObject /Subtype /Image /Width %d /Height 1 /ColorSpace /%s /BitsPerComponent %d %s %s", tc.width, tc.cs, tc.bits, tc.decode, filter), string(data))
				source := recoveryPage("q 40 0 0 20 10 30 cm /X Do Q", "", object)
				d := readRecovery(t, source)
				if len(d.Blocks) != 1 || d.Blocks[0].Kind != Picture {
					t.Fatalf("picture structure: %+v", d.Blocks)
				}
				im, _, err := image.Decode(bytes.NewReader(d.Blocks[0].Image.Data))
				if err != nil {
					t.Fatal(err)
				}
				if im.Bounds().Dx() != tc.width || im.Bounds().Dy() != 1 {
					t.Fatal(im.Bounds())
				}
				for x, want := range tc.want {
					if got := color.NRGBA64Model.Convert(im.At(x, 0)).(color.NRGBA64); got != want {
						t.Fatalf("%s pixel%d = %#v want %#v", suffix, x, got, want)
					}
				}
				closePoint(t, d.Blocks[0].Source.Quad[0], 10, 350)
				closePoint(t, d.Blocks[0].Source.Quad[2], 50, 370)
				assertRecoverySF(t, d)
				exportImageQA(t, tc.name+"-"+suffix, source, d.Blocks[0].Image.Data)
			}
		})
	}
}

func exportImageQA(t *testing.T, name string, source, pixels []byte) {
	t.Helper()
	dir := os.Getenv("MANYDOC_PDF_IMAGE_QA")
	if dir == "" {
		return
	}
	if e := os.MkdirAll(dir, 0755); e != nil {
		t.Fatal(e)
	}
	for ext, data := range map[string][]byte{".pdf": source, ".png": pixels} {
		if e := os.WriteFile(filepath.Join(dir, name+ext), data, 0644); e != nil {
			t.Fatal(e)
		}
	}
}

func TestPDFImage16BitSoftMask(t *testing.T) {
	obj := recoveryStream("/Type /XObject /Subtype /Image /Width 1 /Height 1 /ColorSpace /DeviceRGB /BitsPerComponent 16 /SMask 7 0 R", string([]byte{0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc}))
	mask := recoveryStream("/Type /XObject /Subtype /Image /Width 1 /Height 1 /ColorSpace /DeviceGray /BitsPerComponent 16", string([]byte{0x80, 0x12}))
	source := recoveryPage("q 40 0 0 20 10 30 cm /X Do Q", "", obj, mask)
	d := readRecovery(t, source)
	im, err := png.Decode(bytes.NewReader(d.Blocks[0].Image.Data))
	if err != nil {
		t.Fatal(err)
	}
	got := color.NRGBA64Model.Convert(im.At(0, 0)).(color.NRGBA64)
	if got != (color.NRGBA64{0x1234, 0x5678, 0x9abc, 0x8012}) {
		t.Fatalf("lost sample precision: %#v", got)
	}
	assertRecoverySF(t, d)
	exportImageQA(t, "rgb16-mask", source, d.Blocks[0].Image.Data)
}

func TestPDFImageFax(t *testing.T) {
	for _, tc := range []struct {
		name, params string
		data         []byte
		white        uint16
	}{
		{"fax-g3", "/K 0 /Columns 8 /Rows 1 /EndOfLine true", []byte{0x00, 0x19, 0x80, 0x08, 0x00, 0x80, 0x08, 0x00, 0x80, 0x08, 0x00, 0x80}, 65535},
		{"fax-g4", "/K -1 /Columns 8 /Rows 1", []byte{0x80, 0x08, 0x00, 0x80}, 65535},
		{"fax-g4-invert", "/K -1 /Columns 8 /Rows 0 /BlackIs1 true", []byte{0x80, 0x08, 0x00, 0x80}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			obj := recoveryStream("/Type /XObject /Subtype /Image /Width 8 /Height 1 /ColorSpace /DeviceGray /BitsPerComponent 1 /Filter /CCITTFaxDecode /DecodeParms << "+tc.params+" >>", string(tc.data))
			source := recoveryPage("q 40 0 0 20 10 30 cm /X Do Q", "", obj)
			d := readRecovery(t, source)
			im, _, err := image.Decode(bytes.NewReader(d.Blocks[0].Image.Data))
			if err != nil {
				t.Fatal(err)
			}
			for x := 0; x < 8; x++ {
				r, g, b, a := im.At(x, 0).RGBA()
				if r != uint32(tc.white) || g != uint32(tc.white) || b != uint32(tc.white) || a != 65535 {
					t.Fatalf("pixel %d: %v", x, im.At(x, 0))
				}
			}
			assertRecoverySF(t, d)
			exportImageQA(t, tc.name, source, d.Blocks[0].Image.Data)
		})
	}
}

func TestPDFImageRejectBeforeRendering(t *testing.T) {
	var jpg bytes.Buffer
	if err := jpeg.Encode(&jpg, image.NewNRGBA(image.Rect(0, 0, 2, 1)), nil); err != nil {
		t.Fatal(err)
	}
	corrupt := syntaxZlib(t, []byte{255, 0, 0})
	corrupt[len(corrupt)-1] ^= 1
	for _, tc := range []struct {
		name, attrs string
		data        []byte
		want        string
	}{
		{"jpeg-header", "/Filter /DCTDecode", jpg.Bytes(), "dimensions contradict"},
		{"checksum", "/Filter /FlateDecode", corrupt, "checksum"},
		{"truncated", "", []byte{255, 0}, "truncated"},
		{"invalid-decode", "/Decode [1 0]", []byte{255, 0, 0}, "Decode array"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			obj := recoveryStream("/Type /XObject /Subtype /Image /Width 1 /Height 1 /ColorSpace /DeviceRGB /BitsPerComponent 8 "+tc.attrs, string(tc.data))
			source := recoveryPage("/X Do", "", obj)
			d, err := ReadPDF(context.Background(), bytes.NewReader(source), ReadOptions{})
			exportImageQA(t, "reject-"+tc.name, source, nil)
			if err == nil || d != nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("result=%v err=%v", d, err)
			}
		})
	}
}

func TestPDFImageWorkBudgets(t *testing.T) {
	r := &pdfReader{ctx: context.Background()}
	r.rasterPixels = maxPDFRasterPixels - 1
	if err := r.chargePDFRaster(2, 1); err == nil {
		t.Fatal("accepted excessive raster work")
	}
	r.rasterPixels = 0
	r.streamBudget = &pdfStreamBudget{cache: map[types.IndirectRef][]byte{}, total: maxPDFDecodedBytes - 8}
	sd := &types.StreamDict{Dict: types.Dict{"BitsPerComponent": types.Integer(8)}, Raw: syntaxZlib(t, make([]byte, 9)), FilterPipeline: []types.PDFFilter{{Name: "FlateDecode"}}}
	if err := r.preparePDFImage(sd, 1, 1); err == nil || !strings.Contains(err.Error(), "exceed") {
		t.Fatalf("decoded budget: %v", err)
	}
}

func TestPDFImageBoundedPreparation(t *testing.T) {
	r := &pdfReader{ctx: context.Background(), rasterPixels: maxPDFRasterPixels - 3}
	if err := r.chargePDFRaster(2, 2); err == nil {
		t.Fatal("accepted cumulative raster work")
	}
	r = &pdfReader{ctx: context.Background(), streamBudget: &pdfStreamBudget{total: maxPDFDecodedBytes - 2, cache: map[types.IndirectRef][]byte{}}}
	sd := types.StreamDict{Dict: types.Dict{}, Raw: []byte{1, 2, 3}}
	if err := r.preparePDFImage(&sd, 1, 1); err == nil || !strings.Contains(err.Error(), "decoded streams exceed") {
		t.Fatalf("budget: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r = &pdfReader{ctx: ctx}
	if err := r.chargePDFRaster(1, 1); err != context.Canceled {
		t.Fatalf("cancel: %v", err)
	}
}

func TestPDFImageIndexedLookupAndPlacements(t *testing.T) {
	for _, bad := range []bool{false, true} {
		palette := syntaxZlib(t, []byte{255, 0, 0, 0, 255, 0})
		if bad {
			palette[len(palette)-1] ^= 1
		}
		obj := recoveryStream("/Type /XObject /Subtype /Image /Width 2 /Height 1 /BitsPerComponent 1 /ColorSpace [/Indexed /DeviceRGB 1 7 0 R]", string([]byte{0x40}))
		lookup := recoveryStream("/Filter /FlateDecode", string(palette))
		source := recoveryPage("q 40 0 0 20 10 30 cm /X Do Q q 20 0 0 10 100 90 cm /X Do Q", "", obj, lookup)
		d, err := ReadPDF(context.Background(), bytes.NewReader(source), ReadOptions{PDFSkipInference: true})
		if bad {
			if err == nil || d != nil {
				t.Fatal("accepted corrupt lookup")
			}
			exportImageQA(t, "reject-lookup", source, nil)
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		if len(d.Blocks) != 2 {
			t.Fatalf("placements %d", len(d.Blocks))
		}
		for _, b := range d.Blocks {
			im, _, err := image.Decode(bytes.NewReader(b.Image.Data))
			if err != nil {
				t.Fatal(err)
			}
			if color.NRGBAModel.Convert(im.At(0, 0)) != (color.NRGBA{255, 0, 0, 255}) || color.NRGBAModel.Convert(im.At(1, 0)) != (color.NRGBA{0, 255, 0, 255}) {
				t.Fatal("palette pixels")
			}
		}
		closePoint(t, d.Blocks[0].Source.Quad[0], 10, 350)
		closePoint(t, d.Blocks[1].Source.Quad[0], 100, 300)
		if d.Blocks[0].Source.Object != d.Blocks[1].Source.Object || d.Blocks[0].Source.Order == d.Blocks[1].Source.Order {
			t.Fatal("placement source identity")
		}
		assertRecoverySF(t, d)
		exportImageQA(t, "indexed", source, d.Blocks[0].Image.Data)
	}
}

func TestPDFImageJPEGPipeline(t *testing.T) {
	original := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	original.SetNRGBA(0, 0, color.NRGBA{255, 0, 0, 255})
	original.SetNRGBA(1, 0, color.NRGBA{0, 255, 0, 255})
	var encoded bytes.Buffer
	if err := jpeg.Encode(&encoded, original, nil); err != nil {
		t.Fatal(err)
	}
	want, err := jpeg.Decode(bytes.NewReader(encoded.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	object := recoveryStream("/Type /XObject /Subtype /Image /Width 2 /Height 1 /ColorSpace /DeviceRGB /BitsPerComponent 8 /Filter [/ASCII85Decode /DCTDecode]", string(syntax85(encoded.Bytes())))
	source := recoveryPage("q 40 0 0 20 10 30 cm /X Do Q", "", object)
	d := readRecovery(t, source)
	got, _, err := image.Decode(bytes.NewReader(d.Blocks[0].Image.Data))
	if err != nil {
		t.Fatal(err)
	}
	for x := 0; x < 2; x++ {
		if color.NRGBAModel.Convert(got.At(x, 0)) != color.NRGBAModel.Convert(want.At(x, 0)) {
			t.Fatal("JPEG pipeline changed pixels")
		}
	}
	assertRecoverySF(t, d)
	exportImageQA(t, "jpeg-chain", source, d.Blocks[0].Image.Data)
}

// Optional full public-limit probe; ordinary tests exercise the same remaining-
// allowance path without materializing a 200 MB decoder result under the race tool.
func TestPDFImagePublicStageLimit(t *testing.T) {
	if os.Getenv("MANYDOC_PDF_IMAGE_LARGE") != "1" {
		t.Skip("set MANYDOC_PDF_IMAGE_LARGE=1 for full public image-stage probe")
	}
	var encoded bytes.Buffer
	writer := zlib.NewWriter(&encoded)
	chunk := make([]byte, 1<<20)
	for remaining := maxPDFImageBytes + 1; remaining > 0; {
		n := min(remaining, len(chunk))
		if _, err := writer.Write(chunk[:n]); err != nil {
			t.Fatal(err)
		}
		remaining -= n
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	object := recoveryStream("/Type /XObject /Subtype /Image /Width 1 /Height 1 /ColorSpace /DeviceRGB /BitsPerComponent 8 /Filter /FlateDecode", encoded.String())
	source := recoveryPage("/X Do", "", object)
	d, err := ReadPDF(context.Background(), bytes.NewReader(source), ReadOptions{})
	if err == nil || d != nil || !strings.Contains(err.Error(), "decoded stream limit exceeded") {
		t.Fatalf("public limit: %v", err)
	}
	exportImageQA(t, "reject-oversized-image", source, nil)
}

func TestPDFImageCodecDepthContradiction(t *testing.T) {
	var jpg bytes.Buffer
	if err := jpeg.Encode(&jpg, image.NewNRGBA(image.Rect(0, 0, 1, 1)), nil); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		codec string
		bits  int
		data  []byte
	}{
		{"DCTDecode", 16, jpg.Bytes()}, {"CCITTFaxDecode", 8, []byte{0x80, 0x08, 0, 0x80}},
	} {
		object := recoveryStream(fmt.Sprintf("/Type /XObject /Subtype /Image /Width 1 /Height 1 /BitsPerComponent %d /ColorSpace /DeviceRGB /Filter /%s", tc.bits, tc.codec), string(tc.data))
		d, err := ReadPDF(context.Background(), bytes.NewReader(recoveryPage("/X Do", "", object)), ReadOptions{})
		if d != nil || err == nil || !strings.Contains(err.Error(), "depth contradicts") {
			t.Fatalf("%s depth: %v", tc.codec, err)
		}
	}
}
