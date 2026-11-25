package engine

import (
	"bytes"
	"context"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
	"os"
	"testing"
)

func TestFlowStructuredStreamingPreservesExactPersistentBytes(t *testing.T) {
	for _, name := range []string{"docx_basic.docx", "docx_hyperlink.docx", "docx_image.docx", "docx_sections.docx", "docx_header_footer.docx", "pdf_w3c_dummy.pdf", "pdf_unicode.pdf", "pdf_form_xobject.pdf"} {
		t.Run(name, func(t *testing.T) {
			data, e := os.ReadFile("testdata/corpus/" + name)
			if e != nil {
				t.Fatal(e)
			}
			format, e := FormatFromName(name)
			if e != nil {
				t.Fatal(e)
			}
			d, e := Read(context.Background(), bytes.NewReader(data), format, ReadOptions{})
			if e != nil {
				t.Fatal(e)
			}
			s, e := ToStructured(d)
			if e != nil {
				t.Fatal(e)
			}
			var expected, actual bytes.Buffer
			if e = encodeStructured(context.Background(), &expected, s, false); e != nil {
				t.Fatal(e)
			}
			if e = Write(context.Background(), &actual, d, SF, WriteOptions{}); e != nil {
				t.Fatal(e)
			}
			if !bytes.Equal(expected.Bytes(), actual.Bytes()) {
				t.Fatalf("stream changed SF bytes: expected %d got %d", expected.Len(), actual.Len())
			}
		})
	}
}
func TestFlowStructuredPreflightRejectsBeforeWriting(t *testing.T) {
	d := &Document{Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "valid prefix"}}}, {Kind: Paragraph, Runs: []Run{{Source: &SourceInfo{Format: PDF, Page: 1}}}}}}
	var out bytes.Buffer
	if e := Write(context.Background(), &out, d, SF, WriteOptions{}); e == nil || out.Len() != 0 {
		t.Fatalf("unrepresentable suffix leaked partial output: %v / %d bytes", e, out.Len())
	}
}
func TestPDFReadLimitsAndCacheRemainBounded(t *testing.T) {
	limits, e := resolvePDFReadLimits(nil)
	if e != nil || limits.MaxDecodedBytes != maxPDFDecodedBytes || limits.MaxNavigationEntries != 100000 || limits.MaxStructureEntries != 200000 {
		t.Fatal("default policy changed")
	}
	if _, e = resolvePDFReadLimits(&PDFReadLimits{MaxDecodedBytes: -1}); e == nil {
		t.Fatal("negative work budget accepted")
	}
	if _, e = resolvePDFReadLimits(&PDFReadLimits{MaxNavigationEntries: maxPDFNavigationEntries + 1}); e == nil {
		t.Fatal("model navigation bound bypassed")
	}
	if _, e = resolvePDFReadLimits(&PDFReadLimits{MaxStructureEntries: 10_000_001}); e == nil {
		t.Fatal("structure ceiling bypassed")
	}
	limits, e = resolvePDFReadLimits(&PDFReadLimits{MaxStructureEntries: 2_000_000})
	if e != nil || limits.MaxStructureEntries != 2_000_000 {
		t.Fatal("explicit structure policy not applied")
	}
	reader := &pdfReader{ctx: context.Background(), limits: PDFReadLimits{MaxStructureEntries: 1}}
	recovery := &pdfStructureRecovery{reader: reader}
	if e = recovery.attributeWork(); e != nil {
		t.Fatal(e)
	}
	if e = recovery.attributeWork(); e == nil {
		t.Fatal("attribute policy not enforced")
	}
	b := &pdfStreamBudget{}
	data := make([]byte, 1<<20)
	for i := 1; i <= 80; i++ {
		b.store(types.IndirectRef{ObjectNumber: types.Integer(i)}, data)
	}
	if b.cacheBytes > 32<<20 || len(b.cache) > 32 || len(b.order) > 4096 {
		t.Fatalf("cache not bounded: %d/%d/%d", b.cacheBytes, len(b.cache), len(b.order))
	}
	if _, old := b.cache[types.IndirectRef{ObjectNumber: 1}]; old {
		t.Fatal("old source stream never evicted")
	}
	if _, recent := b.cache[types.IndirectRef{ObjectNumber: 80}]; !recent {
		t.Fatal("recent stream not cached")
	}
}
