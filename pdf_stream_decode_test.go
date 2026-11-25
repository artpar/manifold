package manifold

import (
	"bytes"
	"compress/zlib"
	"context"
	"encoding/ascii85"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hhrutter/lzw"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

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
	// Tiny encoded input expands beyond the real public bound. A following hex
	// decoder would shrink it; the intermediate allocation must still be bounded.
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
	// Two actual syntax invocations share one document-wide work allowance.
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
