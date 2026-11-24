package manydoc

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

type parserStreamFixture struct {
	objectCodec, xrefCodec             string
	header                             string
	count                              int
	badObjectChecksum, badXRefChecksum bool
}

// Independently authored xref/object-stream syntax. Object identities, offsets,
// compressed indexes and expected text are fixed source oracles, not writer output.
func parserStreamPDF(t *testing.T, options parserStreamFixture) []byte {
	t.Helper()
	catalog := "<< /Type /Catalog /Pages 2 0 R >>"
	font := "<< /Type /Font /Subtype /Type1 /BaseFont /Courier >>"
	header := fmt.Sprintf("1 0 4 %d ", len(catalog)+1)
	if options.header != "" {
		header = options.header
	}
	count := 2
	if options.count != 0 {
		count = options.count
	}
	encode := func(data []byte, codec string, bad bool) ([]byte, string) {
		switch codec {
		case "", "raw":
			return data, ""
		case "flate":
			b := syntaxZlib(t, data)
			if bad {
				b[len(b)-1] ^= 1
			}
			return b, "/Filter /FlateDecode"
		case "png", "tiff":
			samples := append([]byte(nil), data...)
			for i := len(samples) - 1; i > 0; i-- {
				samples[i] -= samples[i-1]
			}
			predictor := 2
			if codec == "png" {
				samples = append([]byte{1}, samples...)
				predictor = 15
			}
			return syntaxZlib(t, samples), fmt.Sprintf("/Filter /FlateDecode /DecodeParms << /Predictor %d /Colors 1 /BitsPerComponent 8 /Columns %d >>", predictor, len(data))
		case "ascii85":
			return syntax85(data), "/Filter /ASCII85Decode"
		case "lzw0":
			return syntaxLZW(t, data, false), "/Filter /LZWDecode /DecodeParms << /EarlyChange 0 >>"
		case "lzw1":
			return syntaxLZW(t, data, true), "/Filter /LZWDecode"
		default:
			t.Fatalf("unknown codec %s", codec)
			return nil, ""
		}
	}
	var out bytes.Buffer
	out.WriteString("%PDF-1.7\n")
	offsets := map[int]int{}
	add := func(id int, body string) {
		offsets[id] = out.Len()
		fmt.Fprintf(&out, "%d 0 obj\n%s\nendobj\n", id, body)
	}
	add(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>")
	add(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 400] /Resources << /Font << /F 4 0 R >> >> /Contents 5 0 R >>")
	add(5, recoveryStream("", "BT /F 10 Tf 20 300 Td (COMPRESSED) Tj ET"))
	body, filters := encode([]byte(header+catalog+" "+font), options.objectCodec, options.badObjectChecksum)
	add(6, recoveryStream(fmt.Sprintf("/Type /ObjStm /N %d /First %d %s", count, len(header), filters), string(body)))
	offsets[7] = out.Len()
	var entries bytes.Buffer
	for id := 0; id < 8; id++ {
		typ := byte(1)
		a, b := uint32(offsets[id]), uint16(0)
		switch id {
		case 0:
			typ = 0
			a = 0
			b = 65535
		case 1:
			typ = 2
			a = 6
			b = 0
		case 4:
			typ = 2
			a = 6
			b = 1
		}
		entries.WriteByte(typ)
		_ = binary.Write(&entries, binary.BigEndian, a)
		_ = binary.Write(&entries, binary.BigEndian, b)
	}
	data, filters := encode(entries.Bytes(), options.xrefCodec, options.badXRefChecksum)
	add(7, recoveryStream("/Type /XRef /Size 8 /Root 1 0 R /W [1 4 2] "+filters, string(data)))
	fmt.Fprintf(&out, "startxref\n%d\n%%%%EOF\n", offsets[7])
	return out.Bytes()
}
func parserExport(t *testing.T, name string, source []byte) {
	t.Helper()
	if dir := os.Getenv("MANYDOC_PDF_PARSER_QA"); dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name+".pdf"), source, 0644); err != nil {
			t.Fatal(err)
		}
	}
}
func TestPDFParserStreamSources(t *testing.T) {
	var reference *Document
	for _, codec := range []string{"raw", "flate", "ascii85", "lzw0", "lzw1", "png", "tiff"} {
		t.Run(codec, func(t *testing.T) {
			source := parserStreamPDF(t, parserStreamFixture{objectCodec: codec, xrefCodec: codec})
			parserExport(t, codec, source)
			d := readRecovery(t, source)
			if len(d.Blocks) != 1 || d.Blocks[0].Kind != Paragraph || len(d.Blocks[0].Runs) != 1 || d.Blocks[0].Runs[0].Text != "COMPRESSED" {
				t.Fatalf("source semantics: %+v", d.Blocks)
			}
			closePoint(t, d.Blocks[0].Runs[0].Source.Baseline[0], 20, 100)
			assertRecoverySF(t, d)
			if reference == nil {
				reference = d
			} else if !reflect.DeepEqual(reference, d) {
				t.Fatal("codec changed full source document")
			}
		})
	}
}
func TestPDFParserStreamInvalidSources(t *testing.T) {
	for _, tc := range []struct {
		name    string
		options parserStreamFixture
	}{
		{"object-checksum", parserStreamFixture{objectCodec: "flate", badObjectChecksum: true}},
		{"xref-checksum", parserStreamFixture{xrefCodec: "flate", badXRefChecksum: true}},
		{"wrong-header-owner", parserStreamFixture{header: "9 0 4 33 "}},
		{"wrong-count", parserStreamFixture{count: 1}},
		{"count-too-large", parserStreamFixture{count: 3}},
		{"duplicate-member", parserStreamFixture{header: "1 0 1 34 "}},
		{"vertical-tab", parserStreamFixture{header: "1\v0 4 34 "}},
		{"zero-member", parserStreamFixture{header: "0 0 4 34 "}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := parserStreamPDF(t, tc.options)
			parserExport(t, "reject-"+tc.name, source)
			d, err := ReadPDF(context.Background(), bytes.NewReader(source), ReadOptions{})
			if err == nil || d != nil {
				t.Fatalf("accepted invalid initial parser stream: %v", d)
			}
		})
	}
}

func TestPDFObjectStreamHeaderValidation(t *testing.T) {
	for _, tc := range []struct {
		name, header string
		n            int
		owners       map[int]int
		valid        bool
	}{
		{"ordered offsets, unsorted identities", "4 0 1 3 ", 2, map[int]int{0: 4, 1: 1}, true},
		{"PDF whitespace and comments", "4\x000\t% note\r\n1 3 ", 2, map[int]int{0: 4, 1: 1}, true},
		{"duplicates", "4 0 4 3 ", 2, nil, false},
		{"descending offsets", "4 3 1 0 ", 2, nil, false},
		{"negative offset", "4 -1 1 3 ", 2, nil, false},
		{"outside body", "4 0 1 10 ", 2, nil, false},
		{"missing pair", "4 0 ", 2, nil, false},
		{"extra pair", "4 0 1 3 ", 1, nil, false},
		{"owner mismatch", "4 0 1 3 ", 2, map[int]int{1: 5}, false},
		{"huge N", "4 0 ", int(^uint(0) >> 1), nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := &pdfReader{ctx: context.Background()}
			data := []byte(tc.header + "10 20")
			sd := types.StreamDict{Dict: types.Dict{"N": types.Integer(tc.n), "First": types.Integer(len(tc.header))}}
			err := r.validateObjectStreamHeader(6, &sd, data, tc.owners)
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v error=%v", tc.valid, err)
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r := &pdfReader{ctx: ctx}
	pos := 0
	if err := r.skipHeaderSpace([]byte("%"+strings.Repeat("a", 16384)), &pos); err != context.Canceled {
		t.Fatalf("cancel: %v", err)
	}
}

func TestPDFParserIncrementalSource(t *testing.T) {
	source := parserStreamPDF(t, parserStreamFixture{objectCodec: "flate", xrefCodec: "flate"})
	parts := strings.Split(string(source), "startxref\n")
	previous, err := strconv.Atoi(strings.Fields(parts[len(parts)-1])[0])
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	out.Write(source)
	fontOffset := out.Len()
	out.WriteString("4 0 obj\n<< /Type /Font /Subtype /Type1 /BaseFont /Courier-Bold >>\nendobj\n")
	xref := out.Len()
	fmt.Fprintf(&out, "xref\n4 1\n%010d 00000 n \ntrailer\n<< /Size 8 /Root 1 0 R /Prev %d >>\nstartxref\n%d\n%%%%EOF\n", fontOffset, previous, xref)
	d := readRecovery(t, out.Bytes())
	if d.Blocks[0].Runs[0].Text != "COMPRESSED" || !d.Blocks[0].Runs[0].Bold {
		t.Fatal("incremental replacement lost", d.Blocks)
	}
	assertRecoverySF(t, d)
	parserExport(t, "incremental", out.Bytes())
}

func TestPDFParserHeaderWhitespaceSources(t *testing.T) {
	for _, tc := range []struct{ name, header string }{
		{"comment", "1 0 4 34 % retained header comment\n"},
		{"nul", "1\x000\x004\x0034\x00"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := parserStreamPDF(t, parserStreamFixture{header: tc.header})
			d := readRecovery(t, source)
			if d.Text() != "COMPRESSED" {
				t.Fatalf("body %q", d.Text())
			}
			assertRecoverySF(t, d)
			parserExport(t, tc.name, source)
		})
	}
}

func FuzzPDFObjectStreamHeader(f *testing.F) {
	f.Add([]byte("1 0 4 3 abcdef"), 2, 8)
	f.Add([]byte("4 0 % header\n42"), 1, 13)
	f.Add([]byte("4 0 "), int(^uint(0)>>1), 4)
	f.Fuzz(func(t *testing.T, data []byte, n, first int) {
		if len(data) > 4096 {
			t.Skip()
		}
		r := &pdfReader{ctx: context.Background()}
		sd := types.StreamDict{Dict: types.Dict{"N": types.Integer(n), "First": types.Integer(first)}}
		_ = r.validateObjectStreamHeader(6, &sd, data, map[int]int{0: 1, 1: 4})
	})
}
