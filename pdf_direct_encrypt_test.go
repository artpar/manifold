package manydoc

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/rc4"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// Independently authored standard-security R2 fixture, ISO32000-1 7.6.3.
// The fixed source includes compressed dictionaries/metadata, URI and raster
// streams. It requires no external encoder, generated fixture or production key API.
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
		got, err := adaptDirectPDFEncryption(context.Background(), build(dictionary))
		if err == nil || got != nil {
			t.Fatalf("invalid encryption metadata accepted: %s", dictionary)
		}
	}
}
