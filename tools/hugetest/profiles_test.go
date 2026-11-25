package main

import (
	"bytes"
	"errors"
	"io"
	"testing"
)

type failingProfileSink struct{}

func (failingProfileSink) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestDiagnosticOutputBufferPreservesBytesAndFlushError(t *testing.T) {
	payload := bytes.Repeat([]byte("source provenance;"), 1000)
	for _, size := range []int{0, 65536} {
		var sink bytes.Buffer
		w, flush, counts := profileOutput(&sink, profileConfig{Directory: t.TempDir(), OutputBuffer: size})
		if _, err := w.Write(payload[:4000]); err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(payload[4000:]); err != nil {
			t.Fatal(err)
		}
		if err := flush(); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(sink.Bytes(), payload) || counts.Bytes != int64(len(payload)) {
			t.Fatal("changed output")
		}
		expected := int64(2)
		if size > 0 {
			expected = 1
		}
		if counts.Calls != expected {
			t.Fatalf("calls %d/%d", counts.Calls, expected)
		}
	}
	w, flush, _ := profileOutput(failingProfileSink{}, profileConfig{Directory: t.TempDir(), OutputBuffer: 65536})
	if _, err := w.Write([]byte("pending")); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(flush(), io.ErrClosedPipe) {
		t.Fatal("lost buffered write error")
	}
}
