package engine

import (
	"bytes"
	"compress/zlib"
	"context"
	"crypto/md5"
	"crypto/rc4"
	"encoding/ascii85"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/hhrutter/lzw"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"

	// Independently authored standard-security R2 fixture, ISO32000-1 7.6.3.
	// The fixed source includes compressed dictionaries/metadata, URI and raster
	// streams. It requires no external encoder, generated fixture or production key API.
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func directEncryptionSource(t *testing.T, encrypted, streamXref bool) []byte {
	t.Helper()
	padding, _ := hex.DecodeString("28bf4e5e4e758a4164004e56fffa01082e2e00b6d0683e802f0ca9fe6453697a")
	pad := func(s string) []byte {
		b := append([]byte(nil), []byte(s)...)
		b = append(b, padding...)
		return b[:32]
	}
	crypt := func(key, data []byte) []byte {
		c, err := rc4.NewCipher(key)
		if err != nil {
			t.Fatal(err)
		}
		out := make([]byte, len(data))
		c.XORKeyStream(out, data)
		return out
	}
	ownerKey := md5.Sum(pad("owner"))
	owner := crypt(ownerKey[:5], pad("reader"))
	fileID, _ := hex.DecodeString("00112233445566778899aabbccddeeff")
	passwordInput := append(pad("reader"), owner...)
	permissions := make([]byte, 4)
	binary.LittleEndian.PutUint32(permissions, 0xffffffc0)
	passwordInput = append(passwordInput, permissions...)
	passwordInput = append(passwordInput, fileID...)
	sum := md5.Sum(passwordInput)
	key := sum[:5]
	user := crypt(key, padding)
	objectCrypt := func(number int, data []byte) []byte {
		if !encrypted {
			return data
		}
		input := append(append([]byte(nil), key...), byte(number), byte(number>>8), byte(number>>16), 0, 0)
		sum := md5.Sum(input)
		return crypt(sum[:10], data)
	}
	catalog := "<< /Type /Catalog /Pages 2 0 R >>"
	font := "<< /Type /Font /Subtype /Type1 /BaseFont /Courier >>"
	bold := "<< /Type /Font /Subtype /Type1 /BaseFont /Courier-Bold >>"
	info := "<< /Title (OWNED METADATA) >>"
	members := []string{catalog, font, bold, info}
	ids := []int{1, 4, 9, 8}
	var header, body bytes.Buffer
	for i, member := range members {
		fmt.Fprintf(&header, "%d %d ", ids[i], body.Len())
		body.WriteString(member + " ")
	}
	var out bytes.Buffer
	out.WriteString("%PDF-1.7\n")
	offsets := map[int]int{}
	add := func(id int, value string) {
		offsets[id] = out.Len()
		fmt.Fprintf(&out, "%d 0 obj\n%s\nendobj\n", id, value)
	}
	add(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>")
	add(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 400] /Resources << /Font << /F 4 0 R /FB 9 0 R >> /XObject << /Im 10 0 R >> >> /Contents 5 0 R /Annots [11 0 R] >>")
	paint := []byte("BT /F 10 Tf 20 300 Td (COMPRESSED) Tj /FB 10 Tf (BOLD) Tj ET q 2 0 0 1 20 250 cm /Im Do Q")
	add(5, recoveryStream("", string(objectCrypt(5, paint))))
	add(6, recoveryStream(fmt.Sprintf("/Type /ObjStm /N 4 /First %d", header.Len()), string(objectCrypt(6, append(header.Bytes(), body.Bytes()...)))))
	add(10, recoveryStream("/Type /XObject /Subtype /Image /Width 2 /Height 1 /ColorSpace /DeviceRGB /BitsPerComponent 8", string(objectCrypt(10, []byte{255, 0, 0, 0, 255, 0}))))
	add(11, fmt.Sprintf("<< /Type /Annot /Subtype /Link /Rect [20 295 80 310] /A << /S /URI /URI <%x> >> >>", objectCrypt(11, []byte("https://example.test/owned"))))
	trailer := fmt.Sprintf("/Size 12 /Root 1 0 R /Info 8 0 R /ID [<%x> <%x>]", fileID, fileID)
	if encrypted {
		trailer += fmt.Sprintf(" /Encrypt << /Filter /Standard /V 1 /R 2 /O <%x> /U <%x> /P -64 >>", owner, user)
	}
	offsets[7] = out.Len()
	// Compressed members require an xref stream. The classic variant is a hybrid
	// reference file, with the same compressed map supplied through XRefStm.
	var entries bytes.Buffer
	for id := 0; id < 12; id++ {
		typ := byte(1)
		a, b := uint32(offsets[id]), uint16(0)
		if id == 0 {
			typ = 0
			a = 0
			b = 65535
		}
		for index, member := range ids {
			if id == member {
				typ = 2
				a = 6
				b = uint16(index)
			}
		}
		entries.WriteByte(typ)
		_ = binary.Write(&entries, binary.BigEndian, a)
		_ = binary.Write(&entries, binary.BigEndian, b)
	}
	add(7, recoveryStream("/Type /XRef /W [1 4 2] "+trailer, string(entries.Bytes())))
	xref := offsets[7]
	if !streamXref {
		xref = out.Len()
		out.WriteString("xref\n0 1\n0000000000 65535 f \n")
		for _, id := range []int{2, 3, 5, 6, 7, 10, 11} {
			fmt.Fprintf(&out, "%d 1\n%010d 00000 n \n", id, offsets[id])
		}
		fmt.Fprintf(&out, "trailer\n<< %s /XRefStm %d >>\n", trailer, offsets[7])
	}
	fmt.Fprintf(&out, "startxref\n%d\n%%%%EOF\n", xref)
	return out.Bytes()
}

func exportDirectEncryption(t *testing.T, name string, data []byte) {
	t.Helper()
	dir := os.Getenv("MANYDOC_DIRECT_ENCRYPT_QA")
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name+".pdf"), data, 0644); err != nil {
		t.Fatal(err)
	}
}
func TestPDFDirectEncryptionRichSource(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprintf("stream-%v", stream), func(t *testing.T) {
			clear := directEncryptionSource(t, false, stream)
			original := directEncryptionSource(t, true, stream)
			control := readRecovery(t, clear)
			if control.Metadata.Title != "OWNED METADATA" || len(control.Blocks) != 2 || len(control.Blocks[0].Runs) != 2 || !control.Blocks[0].Runs[1].Bold || control.Blocks[0].Runs[0].Link != "https://example.test/owned" {
				t.Fatalf("source structure/relationships: %+v", control)
			}
			im, _, err := image.Decode(bytes.NewReader(control.Blocks[1].Image.Data))
			if err != nil {
				t.Fatal(err)
			}
			if color.NRGBAModel.Convert(im.At(0, 0)) != (color.NRGBA{255, 0, 0, 255}) || color.NRGBAModel.Convert(im.At(1, 0)) != (color.NRGBA{0, 255, 0, 255}) {
				t.Fatal("source pixels")
			}
			for _, password := range []string{"reader", "owner"} {
				got, err := ReadPDF(context.Background(), bytes.NewReader(original), ReadOptions{Password: password, PDFSkipInference: true})
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(control, got) {
					t.Fatal("decryption changed full source document")
				}
				assertRecoverySF(t, got)
			}
			for _, password := range []string{"", "wrong"} {
				if got, err := ReadPDF(context.Background(), bytes.NewReader(original), ReadOptions{Password: password}); err == nil || got != nil {
					t.Fatalf("wrong password returned document: %v", err)
				}
			}
			snapshot := bytes.Clone(original)
			adapted, err := adaptDirectPDFEncryption(context.Background(), original)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(original, snapshot) || !bytes.HasPrefix(adapted, original) || len(adapted) <= len(original) {
				t.Fatal("original ciphertext/offsets changed")
			}
			again, err := adaptDirectPDFEncryption(context.Background(), adapted)
			if err != nil || !bytes.Equal(again, adapted) {
				t.Fatal("adapter not idempotent", err)
			}
			name := fmt.Sprintf("rich-%v", stream)
			exportDirectEncryption(t, name+"-clear", clear)
			exportDirectEncryption(t, name+"-encrypted", original)
			exportDirectEncryption(t, name+"-adapted", adapted)
		})
	}
}

func TestPDFDirectEncryptionTrailerVariants(t *testing.T) {
	for _, stream := range []bool{false, true} {
		plain := directEncryptionSource(t, false, stream)
		encrypted := directEncryptionSource(t, true, stream)
		control := readRecovery(t, plain)
		for _, variant := range []string{"prefix", "inherited", "low-size"} {
			t.Run(fmt.Sprintf("%v/%s", stream, variant), func(t *testing.T) {
				source := bytes.Clone(encrypted)
				switch variant {
				case "prefix":
					source = append([]byte("compat-prefix\n"), source...)
				case "inherited":
					marker := bytes.LastIndex(source, []byte("startxref"))
					position := marker + len("startxref")
					probe := &pdfReader{ctx: context.Background()}
					prev, err := probe.headerInteger(source, &position)
					if err != nil {
						t.Fatal(err)
					}
					var suffix bytes.Buffer
					suffix.WriteByte('\n')
					xref := len(source) + suffix.Len()
					fmt.Fprintf(&suffix, "xref\n0 1\n0000000000 65535 f \ntrailer\n<< /Size 12 /Root 1 0 R /Prev %d >>\nstartxref\n%d\n%%%%EOF\n", prev, xref)
					source = append(source, suffix.Bytes()...)
				case "low-size":
					if !stream {
						t.Skip("Index bound belongs to the xref stream variant")
					}
					source = bytes.Replace(source, []byte("/Size 12"), []byte("/Size 11 /Index [0 12]"), 1)
				}
				got, err := ReadPDF(context.Background(), bytes.NewReader(source), ReadOptions{Password: "reader", PDFSkipInference: true})
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(control, got) {
					t.Fatal("trailer adaptation changed original semantics/geometry/assets")
				}
				assertRecoverySF(t, got)
				exportDirectEncryption(t, fmt.Sprintf("rich-%v-%s-encrypted", stream, variant), source)
				adapted, err := adaptDirectPDFEncryption(context.Background(), source)
				if err != nil {
					t.Fatal(err)
				}
				exportDirectEncryption(t, fmt.Sprintf("rich-%v-%s-adapted", stream, variant), adapted)
			})
		}
	}
}

func TestPDFDirectEncryptionNoopAndCancellation(t *testing.T) {
	for _, source := range [][]byte{fixture(t, "pdf_basic.pdf"), parserStreamPDF(t, parserStreamFixture{}), []byte("startxref\nnot-a-number"), []byte("%PDF-1.7\nstartxref\n999999999999999999999999999999")} {
		got, err := adaptDirectPDFEncryption(context.Background(), source)
		if err != nil || !bytes.Equal(source, got) {
			t.Fatal("unrelated/undiscovered PDF changed", err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got, err := adaptDirectPDFEncryption(ctx, []byte("%PDF-1.7")); got != nil || err != context.Canceled {
		t.Fatal("cancel", err)
	}
}

func TestPDFDirectEncryptionMalformedTrailer(t *testing.T) {
	build := func(dictionary string) []byte {
		var b bytes.Buffer
		b.WriteString("%PDF-1.7\n")
		xref := b.Len()
		fmt.Fprintf(&b, "xref\n0 1\n0000000000 65535 f \ntrailer\n%s\nstartxref\n%d\n%%%%EOF\n", dictionary, xref)
		return b.Bytes()
	}
	for _, dictionary := range []string{
		"<< /Size 0 /Root 1 0 R /ID [<00><00>] /Encrypt << /Filter /Standard >> >>",
		"<< /Size 8 /Root 8 0 R /ID [<00><00>] /Encrypt << /Filter /Standard >> >>",
		"<< /Size 8 /Root 1 0 R /ID [<00>] /Encrypt << /Filter /Standard >> >>",
		"<< /Size 8 /Root 1 0 R /ID [42 <00>] /Encrypt << /Filter /Standard >> >>",
	} {
		got, err := adaptDirectPDFEncryption(context.

			// Optional full public-limit probe; ordinary tests exercise the same remaining-
			// allowance path without materializing a 200 MB decoder result under the race tool.

			// Tiny encoded input expands beyond the real public bound. A following hex
			// decoder would shrink it; the intermediate allocation must still be bounded.

			// Two actual syntax invocations share one document-wide work allowance.
			Background(), build(dictionary))
		if err == nil || got != nil {
			t.Fatalf("invalid encryption metadata accepted: %s", dictionary)
		}
	}
}
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

const syntaxStreamBody = "BT /F 10 Tf 20 300 Td (FILTERED) Tj ET"

func syntaxZlib(t testing.TB, b []byte) []byte {
	t.Helper()
	var out bytes.Buffer
	w := zlib.NewWriter(&out)
	if _, err := w.Write(b); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}
func syntax85(b []byte) []byte {
	out := make([]byte, ascii85.MaxEncodedLen(len(b)))
	n := ascii85.Encode(out, b)
	return append(out[:n], '~', '>')
}
func syntaxLZW(t testing.TB, b []byte, early bool) []byte {
	t.Helper()
	var out bytes.Buffer
	w := lzw.NewWriter(&out, early)
	if _, err := w.Write(b); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}
func syntaxLiteralRL(b []byte) []byte {
	var out []byte
	for len(b) > 0 {
		n := min(len(b), 128)
		out = append(out, byte(n-1))
		out = append(out, b[:n]...)
		b = b[n:]
	}
	return append(out, 128)
}
func syntaxExport(t testing.TB, name string, b []byte) {
	t.Helper()
	if dir := os.Getenv("MANYDOC_PDF_STREAM_QA"); dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name+".pdf"), b, 0600); err != nil {
			t.Fatal(err)
		}
	}
}
func syntaxPage(data []byte, filters string) []byte {
	objects := []string{"<< /Type /Catalog /Pages 2 0 R >>", "<< /Type /Pages /Kids [3 0 R] /Count 1 >>", "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 400] /Resources << /Font << /F 4 0 R >> >> /Contents 5 0 R >>", "<< /Type /Font /Subtype /Type1 /BaseFont /Courier >>", recoveryStream(filters, string(data))}
	return recoveryPDF(objects...)
}

func TestPDFSyntaxFilterSourceEquivalence(t *testing.T) {
	raw := []byte(syntaxStreamBody)
	base := canonicalSF(t, readRecovery(t, syntaxPage(raw, "")))
	tiff := append([]byte(nil), raw...)
	for i := len(tiff) - 1; i > 0; i-- {
		tiff[i] -= tiff[i-1]
	}
	for _, tc := range []struct {
		name, dict string
		data       []byte
	}{
		{"raw", "", raw}, {"flate", "/Filter /FlateDecode", syntaxZlib(t, raw)},
		{"asciihex", "/Filter /ASCIIHexDecode", []byte(hex.EncodeToString(raw) + ">")},
		{"ascii85", "/Filter /ASCII85Decode", syntax85(raw)},
		{"runlength", "/Filter /RunLengthDecode", syntaxLiteralRL(raw)},
		{"lzw0", "/Filter /LZWDecode /DecodeParms << /EarlyChange 0 >>", syntaxLZW(t, raw, false)},
		{"lzw1", "/Filter /LZWDecode", syntaxLZW(t, raw, true)},
		{"chain", "/Filter [/ASCII85Decode /FlateDecode]", syntax85(syntaxZlib(t, raw))},
		{"png", "/Filter /FlateDecode /DecodeParms << /Predictor 15 /Columns 38 >>", nil},
		{"tiff", fmt.Sprintf("/Filter /FlateDecode /DecodeParms << /Predictor 2 /Columns %d >>", len(raw)), syntaxZlib(t, tiff)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.name == "png" {
				tc.dict = fmt.Sprintf("/Filter /FlateDecode /DecodeParms << /Predictor 15 /Columns %d >>", len(raw))
				tc.data = syntaxZlib(t, append([]byte{0}, raw...))
			}
			source := syntaxPage(tc.data, tc.dict)
			syntaxExport(t, tc.name, source)
			d := readRecovery(t, source)
			if !bytes.Equal(base, canonicalSF(t, d)) {
				t.Fatal("encoding changed full source/SF graph", tc.name)
			}
			runs := recoveryRuns(d)
			if len(runs) != 1 || runs[0].Text != "FILTERED" {
				t.Fatal("decoded text lost")
			}
			closePoint(t, runs[0].Source.Baseline[0], 20, 100)
			closePoint(t, runs[0].Source.Baseline[1], 68, 100)
		})
	}
}

func TestPDFSyntaxCorruptStreamsReject(t *testing.T) {
	good := syntaxZlib(t, []byte(syntaxStreamBody))
	bad := append([]byte(nil), good...)
	bad[len(bad)-1] ^= 1
	for _, tc := range []struct {
		name, dict string
		data       []byte
	}{
		{"checksum", "/Filter /FlateDecode", bad}, {"truncated-flate", "/Filter /FlateDecode", good[:len(good)-5]},
		{"truncated-runlength", "/Filter /RunLengthDecode", []byte{20, 'A'}},
		{"runlength-no-eod", "/Filter /RunLengthDecode", []byte{0, 'A'}},
		{"ascii85-no-eod", "/Filter /ASCII85Decode", []byte("!!!!!")},
		{"asciihex-invalid", "/Filter /ASCIIHexDecode", []byte("GG>")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := syntaxPage(tc.data, tc.dict)
			syntaxExport(t, tc.name, b)
			d, err := ReadPDF(context.Background(), bytes.NewReader(b), ReadOptions{})
			if err == nil || d != nil {
				t.Fatal("corrupt syntax accepted with partial content", err)
			}
		})
	}
}

func TestPDFSyntaxOversizedIntermediateReject(t *testing.T) {

	var encoded bytes.Buffer
	w := zlib.NewWriter(&encoded)
	block := bytes.Repeat([]byte{' '}, 64<<10)
	for i := 0; i < 1025; i++ {
		if _, err := w.Write(block); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	for _, filter := range []string{"/Filter /FlateDecode", "/Filter [/FlateDecode /ASCIIHexDecode]"} {
		b := syntaxPage(encoded.Bytes(), filter)
		name := "oversized"
		if strings.Contains(filter, "[") {
			name = "oversized-intermediate"
		}
		syntaxExport(t, name, b)
		d, err := ReadPDF(context.Background(), bytes.NewReader(b), ReadOptions{})
		if err == nil || d != nil || !errors.Is(err, errPDFDecodedLimit) {
			t.Fatal("decoded allocation was not bounded", err)
		}
	}
}

func TestPDFSyntaxArrayAndFormStreams(t *testing.T) {
	a, b := syntaxStreamBody[:31], syntaxStreamBody[31:]
	objects := []string{"<< /Type /Catalog /Pages 2 0 R >>", "<< /Type /Pages /Kids [3 0 R] /Count 1 >>", "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 400] /Resources << /Font << /F 4 0 R >> >> /Contents [5 0 R 6 0 R] >>", "<< /Type /Font /Subtype /Type1 /BaseFont /Courier >>", recoveryStream("/Filter /FlateDecode", string(syntaxZlib(t, []byte(a)))), recoveryStream("/Filter /ASCIIHexDecode", hex.EncodeToString([]byte(b))+">")}
	d := readRecovery(t, recoveryPDF(objects...))
	if d.Text() != "FILTERED" {
		t.Fatal("array fragments were not concatenated exactly", d.Text())
	}
	assertRecoverySF(t, d)
	for _, appearance := range []bool{false, true} {
		form := recoveryStream("/Type /XObject /Subtype /Form /BBox [0 0 300 400] /Filter /FlateDecode", string(syntaxZlib(t, []byte(syntaxStreamBody))))
		var source []byte
		if appearance {
			source = recoveryPage("", "/Annots [7 0 R]", form, "<< /Type /Annot /Subtype /Widget /Rect [0 0 300 400] /AP << /N 6 0 R >> >>")
		} else {
			source = recoveryPage("/X Do /X Do", "", form)
		}
		d := readRecovery(t, source)
		runs := recoveryRuns(d)
		count := 2
		if appearance {
			count = 1
		}
		if len(runs) != count {
			t.Fatal("Form/cache occurrences lost", appearance, len(runs))
		}
		for _, r := range runs {
			if r.Text != "FILTERED" || r.Source.Page != 1 || r.Source.Stream == "page:1" {
				t.Fatal("Form source lost", r)
			}
			closePoint(t, r.Source.Baseline[0], 20, 100)
			closePoint(t, r.Source.Baseline[1], 68, 100)
		}
		assertRecoverySF(t, d)
	}
}

func TestPDFSyntaxCodecDictionaryGrowth(t *testing.T) {
	raw := make([]byte, 20000)
	for i := range raw {
		raw[i] = byte(i*i + i/3)
	}
	for _, early := range []bool{false, true} {
		v := 0
		if early {
			v = 1
		}
		r := &pdfReader{ctx: context.Background()}
		sd := &types.StreamDict{Raw: syntaxLZW(t, raw, early), FilterPipeline: []types.PDFFilter{{Name: "LZWDecode", DecodeParms: types.Dict{"EarlyChange": types.Integer(v)}}}}
		got, err := r.decodePDFStream(*types.NewIndirectRef(5, 0), sd)
		if err != nil || !bytes.Equal(got, raw) {
			t.Fatal("LZW dictionary transition lost", early, err)
		}
		if r.streamBudget.total != len(raw) {
			t.Fatal("unexpected stage charge")
		}
		got, err = r.decodePDFStream(*types.NewIndirectRef(5, 0), sd)
		if err != nil || !bytes.Equal(got, raw) || r.streamBudget.total != len(raw) {
			t.Fatal("cached content was charged/decoded twice", err)
		}
	}
}

func TestPDFSyntaxBudgetAndCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	r := &pdfReader{ctx: ctx}
	ref := *types.NewIndirectRef(5, 0)
	sd := &types.StreamDict{Raw: []byte("q Q")}
	if _, err := r.decodePDFStream(ref, sd); err != nil {
		t.Fatal(err)
	}
	cancel()
	if _, err := r.decodePDFStream(ref, sd); !errors.Is(err, context.Canceled) {
		t.Fatal("cache hit ignored cancellation", err)
	}
	r = &pdfReader{ctx: context.Background(), streamBudget: &pdfStreamBudget{total: maxPDFDecodedBytes - 2, cache: map[types.IndirectRef][]byte{}}}
	if _, err := r.decodePDFStream(ref, sd); err == nil || !strings.Contains(err.Error(), "decoded streams exceed") {
		t.Fatal("document stream budget ignored", err)
	}
	compressed := &types.StreamDict{Raw: syntaxZlib(t, []byte("q Q")), FilterPipeline: []types.PDFFilter{{Name: "FlateDecode"}}}
	r = &pdfReader{ctx: context.Background(), streamBudget: &pdfStreamBudget{total: maxPDFDecodedBytes - 2, cache: map[types.IndirectRef][]byte{}}}
	if _, err := r.decodePDFStream(ref, compressed); err == nil || !strings.Contains(err.Error(), "decoded streams exceed") {
		t.Fatal("filtered stage ignored remaining document allowance", err)
	}
	r = &pdfReader{ctx: context.Background()}
	if data, err := r.decodePDFStream(ref, &types.StreamDict{Content: []byte("q Q")}); err != nil || string(data) != "q Q" {
		t.Fatal("short predecoded stream failed", err)
	}
	r = &pdfReader{ctx: context.Background()}
	if _, err := r.decodePDFStream(ref, &types.StreamDict{FilterPipeline: make([]types.PDFFilter, maxPDFStreamFilters+1)}); err == nil {
		t.Fatal("excessive filter pipeline accepted")
	}

	p, err := pdfcpu.ReadWithContext(context.Background(), bytes.NewReader(syntaxPage(nil, "")), &model.Configuration{Reader15: true, ValidationMode: model.ValidationRelaxed, Offline: true})
	if err != nil {
		t.Fatal(err)
	}
	r = &pdfReader{ctx: context.Background(), pdf: p, doc: &Document{}, contentBytes: maxPDFDecodedBytes - 4}
	if err = r.content([]byte("q Q"), nil, 0, initialPDFState()); err != nil {
		t.Fatal(err)
	}
	if err = r.content([]byte("q Q"), nil, 0, initialPDFState()); err == nil || !strings.Contains(err.Error(), "interpreted syntax exceeds") {
		t.Fatal("repeated syntax budget ignored", err)
	}
}

func TestPDFDecodedReaderShortAndCanceled(t *testing.T) {
	for _, raw := range [][]byte{nil, []byte("x"), []byte("short")} {
		got, err := readPDFDecoded(context.Background(), bytes.NewReader(raw), 100)
		if err != nil || !bytes.Equal(got, raw) {
			t.Fatal("short stream mishandled", err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := readPDFDecoded(ctx, strings.NewReader("body"), 100); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestPDFPredictorFixedVectors(t *testing.T) {
	for filter, encoded := range [][]byte{
		{0, 10, 20, 30, 0, 20, 30, 40},
		{1, 10, 10, 10, 1, 20, 10, 10},
		{2, 10, 20, 30, 2, 10, 10, 10},
		{3, 10, 15, 20, 3, 15, 10, 10},
		{4, 10, 10, 10, 4, 10, 10, 10},
	} {
		for _, shape := range [][3]int{{1, 3, 8}, {2, 3, 4}} {
			got, err := unpredictPDF(context.Background(), encoded, 15, shape[0], shape[1], shape[2])
			if err != nil || !bytes.Equal(got, []byte{10, 20, 30, 20, 30, 40}) {
				t.Fatal("PNG source vector failed", filter, shape, got, err)
			}
		}
	}
	for _, tc := range []struct {
		colors, columns, bits int
		encoded, want         []byte
	}{
		{1, 4, 1, []byte{0xe0}, []byte{0xb0}},
		{1, 4, 2, []byte{0x5b}, []byte{0x63}},
		{1, 4, 4, []byte{0x12, 0xfd}, []byte{0x13, 0x2f}},
		{2, 2, 8, []byte{1, 10, 2, 10}, []byte{1, 10, 3, 20}},
		{1, 2, 16, []byte{0x12, 0x34, 0x22, 0x22}, []byte{0x12, 0x34, 0x34, 0x56}},
		{1, 2, 16, []byte{0xff, 0xfa, 0, 0x10}, []byte{0xff, 0xfa, 0, 0x0a}},
	} {
		got, err := unpredictPDF(context.Background(), append([]byte(nil), tc.encoded...), 2, tc.colors, tc.columns, tc.bits)
		if err != nil || !bytes.Equal(got, tc.want) {
			t.Fatal("TIFF packed source vector failed", tc.bits, got, tc.want, err)
		}
	}
	for _, tc := range [][4]int{{15, 1, 1 << 62, 16}, {15, 0, 2, 8}, {2, 1, 1, 3}, {15, 1, 3, 8}} {
		if _, err := unpredictPDF(context.Background(), []byte{0, 1}, tc[0], tc[1], tc[2], tc[3]); err == nil {
			t.Fatal("invalid predictor accepted", tc)
		}
	}
}

func syntaxUnicodePage(data []byte, dict string) []byte {
	return recoveryPDF("<< /Type /Catalog /Pages 2 0 R >>", "<< /Type /Pages /Kids [3 0 R] /Count 1 >>", "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 400] /Resources << /Font << /F 4 0 R >> >> /Contents 5 0 R >>", "<< /Type /Font /Subtype /Type0 /BaseFont /Demo /Encoding /Identity-H /DescendantFonts [6 0 R] /ToUnicode 7 0 R >>", recoveryStream("", "BT /F 10 Tf 20 300 Td <0001> Tj ET"), "<< /Type /Font /Subtype /CIDFontType2 /BaseFont /Demo /CIDSystemInfo << /Registry (Adobe) /Ordering (Identity) /Supplement 0 >> /DW 500 >>", recoveryStream(dict, string(data)))
}
func TestPDFSyntaxToUnicodeStream(t *testing.T) {
	raw := []byte("1 beginbfchar <0001> <03A9> endbfchar")
	want := canonicalSF(t, readRecovery(t, syntaxUnicodePage(raw, "")))
	encoded := syntaxZlib(t, raw)
	d := readRecovery(t, syntaxUnicodePage(encoded, "/Filter /FlateDecode"))
	if d.Text() != "Ω" || !bytes.Equal(want, canonicalSF(t, d)) {
		t.Fatal("encoded ToUnicode changed source graph")
	}
	encoded[len(encoded)-1] ^= 1
	if d, err := ReadPDF(context.Background(), bytes.NewReader(syntaxUnicodePage(encoded, "/Filter /FlateDecode")), ReadOptions{}); err == nil || d != nil {
		t.Fatal("corrupt ToUnicode stream accepted")
	}
}

func FuzzPDFSyntaxFilters(f *testing.F) {
	f.Add([]byte("short"), uint8(0))
	f.Add([]byte("\x00\xff"), uint8(3))
	f.Add([]byte{}, uint8(5))
	f.Fuzz(func(t *testing.T, raw []byte, codec uint8) {
		if len(raw) > 16384 {
			raw = raw[:16384]
		}
		sd := &types.StreamDict{}
		filter := types.PDFFilter{}
		switch codec % 6 {
		case 0:
			filter.Name = "FlateDecode"
			sd.Raw = syntaxZlib(t, raw)
		case 1:
			filter.Name = "ASCIIHexDecode"
			sd.Raw = []byte(hex.EncodeToString(raw) + ">")
		case 2:
			filter.Name = "ASCII85Decode"
			sd.Raw = syntax85(raw)
		case 3:
			filter.Name = "RunLengthDecode"
			sd.Raw = syntaxLiteralRL(raw)
		case 4:
			filter.Name = "LZWDecode"
			filter.DecodeParms = types.Dict{"EarlyChange": types.Integer(0)}
			sd.Raw = syntaxLZW(t, raw, false)
		case 5:
			filter.Name = "LZWDecode"
			sd.Raw = syntaxLZW(t, raw, true)
		}
		sd.FilterPipeline = []types.PDFFilter{filter}
		r := &pdfReader{ctx: context.Background()}
		decoded, err := r.decodePDFStream(*types.NewIndirectRef(5, 0), sd)
		if err != nil || !bytes.Equal(decoded, raw) {
			t.Fatal("bounded codec altered source", filter.Name, err)
		}
	})
}

func TestPDFSyntaxLongLZWPublicSources(t *testing.T) {
	comment := make([]byte, 30000)
	for i := range comment {
		comment[i] = byte('!' + (i*i+i/7)%90)
	}
	raw := append([]byte{'%'}, comment...)
	raw = append(raw, '\n')
	raw = append(raw, []byte(syntaxStreamBody)...)
	base := canonicalSF(t, readRecovery(t, syntaxPage(raw, "")))
	for _, early := range []bool{false, true} {
		n := 0
		if early {
			n = 1
		}
		dict := fmt.Sprintf("/Filter /LZWDecode /DecodeParms << /EarlyChange %d >>", n)
		source := syntaxPage(syntaxLZW(t, raw, early), dict)
		syntaxExport(t, fmt.Sprintf("long-lzw%d", n), source)
		d := readRecovery(t, source)
		if !bytes.Equal(base, canonicalSF(t, d)) {
			t.Fatal("LZW dictionary growth changed public source graph", n)
		}
	}
}
