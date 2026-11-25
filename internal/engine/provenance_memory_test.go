package engine

import (
	"fmt"
	"strings"
	"testing"
	"unsafe"
)

func TestSourceMetadataSharingRetainsMutableOwnership(t *testing.T) {
	var pool sourceStringPool
	stream := strings.Repeat("Span", 1024)
	owned := pool.intern(stream[:4], true)
	if owned != "Span" || unsafe.StringData(owned) == unsafe.StringData(stream) {
		t.Fatal("short name retained full stream")
	}
	a, b := &SourceInfo{Font: strings.Clone("RepeatedFont"), Evidence: strings.Clone("pdf_text_operators")}, &SourceInfo{Font: strings.Clone("RepeatedFont"), Evidence: strings.Clone("pdf_text_operators")}
	bs := []Block{{Kind: Paragraph, Runs: []Run{{Text: "first", Source: a}, {Text: "second", Source: b}}}}
	pool.blocks(bs)
	if bs[0].Runs[0].Source == bs[0].Runs[1].Source {
		t.Fatal("mutable sources aliased")
	}
	if unsafe.StringData(a.Font) != unsafe.StringData(b.Font) {
		t.Fatal("metadata not shared")
	}
	a.Font = "Edited"
	if b.Font != "RepeatedFont" {
		t.Fatal("source edit changed another owner")
	}
	for i := 0; i < 5000; i++ {
		pool.intern(fmt.Sprintf("font-%d", i), true)
	}
	if len(pool.values) > 4096 || pool.bytes > 1<<20 {
		t.Fatal("metadata cache unbounded")
	}
	n := len(pool.values)
	pool.intern(strings.Repeat("x", 257), true)
	if len(pool.values) != n {
		t.Fatal("long value entered cache")
	}
}

func TestPDFPackedGeometryKeepsSourceAndCloneOwnership(t *testing.T) {
	reader := &pdfReader{page: 1, pageInfo: PDFPage{Transform: identityMatrix()}}
	state := initialPDFState()
	state.size = 12
	state.font = &pdfFont{name: "Example", ascent: 800, descent: -200}
	a, b := reader.sourceFor(&state, 10, false), reader.sourceFor(&state, 10, false)
	before := *b.Quad
	a.Quad[0].X += 10
	a.Matrix[0] += 1
	if *b.Quad != before || *a.Matrix == *b.Matrix {
		t.Fatal("geometry leaked between runs")
	}
	copy := cloneSourceInfo(a)
	copy.Quad[0].X++
	copy.Baseline[0].X++
	if copy.Quad[0].X == a.Quad[0].X || copy.Baseline[0].X == a.Baseline[0].X {
		t.Fatal("clone geometry aliased")
	}
}
