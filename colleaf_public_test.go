package manydoc_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"reflect"
	"testing"

	"github.com/artpar/manydoc"
)

func TestVDOCSPublicGraphAndHistory(t *testing.T) {
	ctx := context.Background()
	for _, path := range []string{
		"testdata/real/poi-footnotes.docx", "testdata/real/poi-headerFooter.docx",
		"testdata/corpus/docx_table.docx", "testdata/corpus/docx_image.docx",
		"testdata/corpus/docx_lists.docx", "testdata/corpus/docx_unicode.docx",
		"testdata/real/copyright-office-circular-1-2021.pdf",
	} {
		t.Run(path, func(t *testing.T) {
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			format, err := manydoc.FormatFromName(path)
			if err != nil {
				t.Fatal(err)
			}
			want, err := manydoc.Read(ctx, bytes.NewReader(raw), format, manydoc.ReadOptions{})
			if err != nil {
				t.Fatal(err)
			}
			before, err := manydoc.ToStructured(want)
			if err != nil {
				t.Fatal(err)
			}
			var native bytes.Buffer
			// Native transport retains imported diagnostics/source history; no lossy
			// acknowledgement is needed just to transport that same information.
			if err = manydoc.WriteVDOCS(ctx, &native, want); err != nil {
				t.Fatal(err)
			}
			var envelope struct {
				Type                  string
				Version               int
				Engine, EngineVersion string
			}
			if err = json.Unmarshal(native.Bytes(), &envelope); err != nil {
				t.Fatal(err)
			}
			if envelope.Type != "vibedada-document" || envelope.Version != 3 || envelope.Engine != "univer" || envelope.EngineVersion != "1.0.4" {
				t.Fatal("incorrect envelope", envelope)
			}
			got, err := manydoc.ReadVDOCS(ctx, bytes.NewReader(native.Bytes()))
			if err != nil {
				t.Fatal(err)
			}
			after, err := manydoc.ToStructured(got)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before, after) {
				t.Fatal("public native conversion changed SF graph")
			}
			unchanged, err := manydoc.ToStructured(want)
			if err != nil || !reflect.DeepEqual(before, unchanged) {
				t.Fatal("input mutated", err)
			}
			native.Reset()
			if err = manydoc.Write(ctx, &native, got, manydoc.VDOCS, manydoc.WriteOptions{}); err != nil {
				t.Fatal(err)
			}
			reopened, err := manydoc.Read(ctx, bytes.NewReader(native.Bytes()), manydoc.VDOCS, manydoc.ReadOptions{})
			if err != nil {
				t.Fatal(err)
			}
			after, err = manydoc.ToStructured(reopened)
			if err != nil || !reflect.DeepEqual(before, after) {
				t.Fatal("generic format dispatch changed SF graph", err)
			}
		})
	}
}

var nativeIOFailure = errors.New("native test I/O failure")

type nativeFailIO struct{}

func (nativeFailIO) Read([]byte) (int, error)  { return 0, nativeIOFailure }
func (nativeFailIO) Write([]byte) (int, error) { return 0, nativeIOFailure }

func TestVDOCSPublicBoundaries(t *testing.T) {
	ctx := context.Background()
	if manydoc.VDOCSMIME != "application/vnd.vibedada.docs+json;version=3" {
		t.Fatal("incorrect native MIME")
	}
	for _, name := range []string{"document.vdocs", "DOCUMENT.VDOCS"} {
		format, err := manydoc.FormatFromName(name)
		if err != nil || format != manydoc.VDOCS {
			t.Fatal("native extension dispatch", name, format, err)
		}
	}
	d := &manydoc.Document{Blocks: []manydoc.Block{{Kind: manydoc.Paragraph, Runs: []manydoc.Run{{Text: "Hello"}}}}}
	for _, path := range []string{"example.vdocs", "EXAMPLE.VDOCS"} {
		format, err := manydoc.FormatFromName(path)
		if err != nil || format != manydoc.VDOCS {
			t.Fatal("extension dispatch", format, err)
		}
	}
	if manydoc.VDOCSMIME != "application/vnd.vibedada.docs+json;version=3" {
		t.Fatal("incorrect application MIME")
	}
	if _, err := manydoc.ReadVDOCS(ctx, nil); err == nil {
		t.Fatal("nil reader accepted")
	}
	if err := manydoc.WriteVDOCS(ctx, nil, d); err == nil {
		t.Fatal("nil writer accepted")
	}
	if err := manydoc.WriteVDOCS(ctx, io.Discard, nil); err == nil {
		t.Fatal("nil document accepted")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := manydoc.ReadVDOCS(cancelled, bytes.NewReader(nil)); !errors.Is(err, context.Canceled) {
		t.Fatal("read cancellation", err)
	}
	if err := manydoc.WriteVDOCS(cancelled, io.Discard, d); !errors.Is(err, context.Canceled) {
		t.Fatal("write cancellation", err)
	}
	if _, err := manydoc.ReadVDOCS(ctx, nativeFailIO{}); !errors.Is(err, nativeIOFailure) {
		t.Fatal("reader error lost", err)
	}
	if err := manydoc.WriteVDOCS(ctx, nativeFailIO{}, d); !errors.Is(err, nativeIOFailure) {
		t.Fatal("writer error lost", err)
	}
	for _, raw := range []string{`{}`, `{"body":{"dataStream":"x\r\n"}}`, `{"type":"doc","content":[]}`, `{"type":"vibedada-document","version":2,"engine":"univer","engineVersion":"1.0.4","document":{}}`} {
		if _, err := manydoc.ReadVDOCS(ctx, bytes.NewBufferString(raw)); err == nil {
			t.Fatal("wrong envelope accepted", raw)
		}
	}
	d.Blocks[0].Runs[0].Direction = "rtl"
	for _, lossy := range []bool{false, true} {
		var b bytes.Buffer
		if err := manydoc.Write(ctx, &b, d, manydoc.VDOCS, manydoc.WriteOptions{AllowLossy: lossy}); !errors.Is(err, manydoc.ErrUnsupported) || b.Len() != 0 {
			t.Fatal("unsupported output emitted or bypassed", err)
		}
	}
}

func TestVDOCSPublicActualSavedExample(t *testing.T) {
	file := os.Getenv("MANYDOC_PUBLIC_VDOCS_SAVED")
	if file == "" {
		t.Skip("optional actual reader artifact")
	}
	raw, err := os.ReadFile("examples/notes/document.sf")
	if err != nil {
		t.Fatal(err)
	}
	want, err := manydoc.Read(context.Background(), bytes.NewReader(raw), manydoc.SF, manydoc.ReadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	saved, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	got, err := manydoc.ReadVDOCS(context.Background(), bytes.NewReader(saved))
	if err != nil {
		t.Fatal(err)
	}
	a, err := manydoc.ToStructured(want)
	if err != nil {
		t.Fatal(err)
	}
	b, err := manydoc.ToStructured(got)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a, b) {
		t.Fatal("actual editor save changed public native SF graph")
	}
}
