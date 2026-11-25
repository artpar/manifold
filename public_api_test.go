package manifold_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"reflect"
	"testing"

	"github.com/artpar/manifold"
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
			format, err := manifold.FormatFromName(path)
			if err != nil {
				t.Fatal(err)
			}
			want, err := manifold.Read(ctx, bytes.NewReader(raw), format, manifold.ReadOptions{})
			if err != nil {
				t.Fatal(err)
			}
			before, err := manifold.ToStructured(want)
			if err != nil {
				t.Fatal(err)
			}
			var native bytes.Buffer
			// Native transport retains imported diagnostics/source history; no lossy
			// acknowledgement is needed just to transport that same information.
			if err = manifold.WriteVDOCS(ctx, &native, want); err != nil {
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
			if envelope.Type != "manifold-document" || envelope.Version != 3 || envelope.Engine != "univer" || envelope.EngineVersion != "1.0.4" {
				t.Fatal("incorrect envelope", envelope)
			}
			got, err := manifold.ReadVDOCS(ctx, bytes.NewReader(native.Bytes()))
			if err != nil {
				t.Fatal(err)
			}
			after, err := manifold.ToStructured(got)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before, after) {
				t.Fatal("public native conversion changed SF graph")
			}
			unchanged, err := manifold.ToStructured(want)
			if err != nil || !reflect.DeepEqual(before, unchanged) {
				t.Fatal("input mutated", err)
			}
			native.Reset()
			if err = manifold.Write(ctx, &native, got, manifold.VDOCS, manifold.WriteOptions{}); err != nil {
				t.Fatal(err)
			}
			reopened, err := manifold.Read(ctx, bytes.NewReader(native.Bytes()), manifold.VDOCS, manifold.ReadOptions{})
			if err != nil {
				t.Fatal(err)
			}
			after, err = manifold.ToStructured(reopened)
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
	if manifold.VDOCSMIME != "application/vnd.manifold.docs+json;version=3" {
		t.Fatal("incorrect native MIME")
	}
	for _, name := range []string{"document.vdocs", "DOCUMENT.VDOCS"} {
		format, err := manifold.FormatFromName(name)
		if err != nil || format != manifold.VDOCS {
			t.Fatal("native extension dispatch", name, format, err)
		}
	}
	d := &manifold.Document{Blocks: []manifold.Block{{Kind: manifold.Paragraph, Runs: []manifold.Run{{Text: "Hello"}}}}}
	for _, path := range []string{"example.vdocs", "EXAMPLE.VDOCS"} {
		format, err := manifold.FormatFromName(path)
		if err != nil || format != manifold.VDOCS {
			t.Fatal("extension dispatch", format, err)
		}
	}
	if manifold.VDOCSMIME != "application/vnd.manifold.docs+json;version=3" {
		t.Fatal("incorrect application MIME")
	}
	if _, err := manifold.ReadVDOCS(ctx, nil); err == nil {
		t.Fatal("nil reader accepted")
	}
	if err := manifold.WriteVDOCS(ctx, nil, d); err == nil {
		t.Fatal("nil writer accepted")
	}
	if err := manifold.WriteVDOCS(ctx, io.Discard, nil); err == nil {
		t.Fatal("nil document accepted")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := manifold.ReadVDOCS(cancelled, bytes.NewReader(nil)); !errors.Is(err, context.Canceled) {
		t.Fatal("read cancellation", err)
	}
	if err := manifold.WriteVDOCS(cancelled, io.Discard, d); !errors.Is(err, context.Canceled) {
		t.Fatal("write cancellation", err)
	}
	if _, err := manifold.ReadVDOCS(ctx, nativeFailIO{}); !errors.Is(err, nativeIOFailure) {
		t.Fatal("reader error lost", err)
	}
	if err := manifold.WriteVDOCS(ctx, nativeFailIO{}, d); !errors.Is(err, nativeIOFailure) {
		t.Fatal("writer error lost", err)
	}
	for _, raw := range []string{`{}`, `{"body":{"dataStream":"x\r\n"}}`, `{"type":"doc","content":[]}`, `{"type":"manifold-document","version":2,"engine":"univer","engineVersion":"1.0.4","document":{}}`} {
		if _, err := manifold.ReadVDOCS(ctx, bytes.NewBufferString(raw)); err == nil {
			t.Fatal("wrong envelope accepted", raw)
		}
	}
	d.Blocks[0].Runs[0].Direction = "rtl"
	for _, lossy := range []bool{false, true} {
		var b bytes.Buffer
		if err := manifold.Write(ctx, &b, d, manifold.VDOCS, manifold.WriteOptions{AllowLossy: lossy}); !errors.Is(err, manifold.ErrUnsupported) || b.Len() != 0 {
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
	want, err := manifold.Read(context.Background(), bytes.NewReader(raw), manifold.SF, manifold.ReadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	saved, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	got, err := manifold.ReadVDOCS(context.Background(), bytes.NewReader(saved))
	if err != nil {
		t.Fatal(err)
	}
	a, err := manifold.ToStructured(want)
	if err != nil {
		t.Fatal(err)
	}
	b, err := manifold.ToStructured(got)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a, b) {
		t.Fatal("actual editor save changed public native SF graph")
	}
}
