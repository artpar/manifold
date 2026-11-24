package manydoc

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"testing"
)

func codecGzip(t testing.TB, raw []byte) []byte {
	t.Helper()
	var b bytes.Buffer
	g := gzip.NewWriter(&b)
	if _, e := g.Write(raw); e != nil {
		t.Fatal(e)
	}
	if e := g.Close(); e != nil {
		t.Fatal(e)
	}
	return b.Bytes()
}
func TestStructuredCompressedSemantics(t *testing.T) {
	ctx := context.Background()
	for _, d := range []*Document{destinationDocument(), noteDocument(), readRecovery(t, navigationFixture(fullNavigationTree(), `/foo [6 0 R /FitH 100]`, `(foo)`))} {
		before := canonicalSF(t, d)
		var plain, compressed, repeated bytes.Buffer
		for _, o := range []struct {
			f Format
			b *bytes.Buffer
		}{{SF, &plain}, {SFGzip, &compressed}, {SFGzip, &repeated}} {
			if e := Write(ctx, o.b, d, o.f, WriteOptions{}); e != nil {
				t.Fatal(e)
			}
		}
		if !bytes.Equal(compressed.Bytes(), repeated.Bytes()) {
			t.Fatal("nondeterministic gzip metadata")
		}
		var compact bytes.Buffer
		if e := json.Compact(&compact, bytes.TrimSpace(plain.Bytes())); e != nil {
			t.Fatal(e)
		}
		if !bytes.Equal(compact.Bytes(), bytes.TrimSpace(plain.Bytes())) {
			t.Fatal("SF not compact JSON")
		}
		z, e := gzip.NewReader(bytes.NewReader(compressed.Bytes()))
		if e != nil {
			t.Fatal(e)
		}
		raw, e := io.ReadAll(z)
		if e != nil {
			t.Fatal(e)
		}
		z.Close()
		if !bytes.Equal(raw, plain.Bytes()) {
			t.Fatal("gzip changed canonical JSON payload")
		}
		back, e := Read(ctx, bytes.NewReader(compressed.Bytes()), SFGzip, ReadOptions{})
		if e != nil {
			t.Fatal(e)
		}
		if !bytes.Equal(before, canonicalSF(t, back)) {
			t.Fatal("compressed SF graph/provenance/asset semantics changed")
		}
		if !bytes.Equal(before, canonicalSF(t, d)) {
			t.Fatal("caller mutated")
		}
	}
	for name, want := range map[string]Format{"REPORT.SF.GZ": SFGzip, "report.sf": SF, "archive.json.gz": ""} {
		got, e := FormatFromName(name)
		if want == "" {
			if e == nil {
				t.Fatal("unrecognized compression accepted")
			}
		} else if e != nil || got != want {
			t.Fatal(name, got, e)
		}
	}
}
func TestStructuredCompressedMalformed(t *testing.T) {
	raw := canonicalSF(t, destinationDocument())
	valid := codecGzip(t, raw)
	badCRC := append([]byte(nil), valid...)
	badCRC[len(badCRC)-8] ^= 1
	cases := [][]byte{nil, raw, valid[:4], valid[:len(valid)-1], badCRC, append(append([]byte(nil), valid...), valid...), append(append([]byte(nil), valid...), []byte("junk")...), codecGzip(t, []byte(`{"type":"doc","attrs":{"schema":"manydoc-prosemirror/1"},"content":[{"type":"unsupported"}]}`))}
	for i, b := range cases {
		d, e := Read(context.Background(), bytes.NewReader(b), SFGzip, ReadOptions{})
		if e == nil || d != nil {
			t.Fatalf("invalid gzip/SF accepted: %d: %v", i, e)
		}
	}
	_, e := Read(context.Background(), bytes.NewReader(badCRC), SFGzip, ReadOptions{})
	if !errors.Is(e, gzip.ErrChecksum) {
		t.Fatal("checksum error lost", e)
	}
	if _, e := Read(context.Background(), nil, SFGzip, ReadOptions{}); e == nil {
		t.Fatal("nil compressed reader accepted")
	}
}

var errCodecOutput = errors.New("codec output failure")

type codecFailWriter struct{}

func (codecFailWriter) Write([]byte) (int, error) { return 0, errCodecOutput }

type codecShortWriter struct{}

func (codecShortWriter) Write(p []byte) (int, error) { return len(p) - 1, nil }

type codecCancelWriter struct{ cancel context.CancelFunc }

func (w codecCancelWriter) Write(p []byte) (int, error) { w.cancel(); return len(p), nil }
func TestStructuredCodecOutputAndAdmission(t *testing.T) {
	for _, format := range []Format{SF, SFGzip} {
		if e := Write(context.Background(), codecFailWriter{}, destinationDocument(), format, WriteOptions{}); !errors.Is(e, errCodecOutput) {
			t.Fatal(format, e)
		}
		if e := Write(context.Background(), codecShortWriter{}, destinationDocument(), format, WriteOptions{}); !errors.Is(e, io.ErrShortWrite) {
			t.Fatal(format, e)
		}
		d := destinationDocument()
		d.UnresolvedDestinations = nil
		var out bytes.Buffer
		if e := Write(context.Background(), &out, d, format, WriteOptions{}); e == nil || out.Len() != 0 {
			t.Fatal("invalid graph emitted a transport header")
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if e := Write(ctx, &out, destinationDocument(), format, WriteOptions{}); !errors.Is(e, context.Canceled) || out.Len() != 0 {
			t.Fatal("cancelled output emitted", e)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if e := Write(ctx, codecCancelWriter{cancel}, destinationDocument(), SFGzip, WriteOptions{}); !errors.Is(e, context.Canceled) {
		t.Fatal("gzip finalization ignored cancellation", e)
	}
}
func TestStructuredCompressedCorpus(t *testing.T) {
	// The original corpus and its oracles are unchanged. Compression must preserve
	// every declared normalized SF field, including source, notes and asset bytes.
	for _, c := range corpus(t) {
		if c.Expect == "reject" {
			continue
		}
		t.Run(c.Name, func(t *testing.T) {
			f, e := FormatFromName(c.Name)
			if e != nil {
				t.Fatal(e)
			}
			d, e := Read(context.Background(), bytes.NewReader(fixture(t, c.Name)), f, ReadOptions{Password: "secret"})
			if e != nil {
				t.Fatal(e)
			}
			var b bytes.Buffer
			if e = Write(context.Background(), &b, d, SFGzip, WriteOptions{}); e != nil {
				t.Fatal(e)
			}
			got, e := Read(context.Background(), &b, SFGzip, ReadOptions{})
			if e != nil {
				t.Fatal(e)
			}
			if !reflect.DeepEqual(canonicalSF(t, d), canonicalSF(t, got)) {
				t.Fatal("compressed corpus semantics changed")
			}
		})
	}
}
func FuzzReadStructuredGzip(f *testing.F) {
	raw := []byte(`{"type":"doc","attrs":{"schema":"manydoc-prosemirror/1"},"content":[{"type":"paragraph","content":[{"type":"text","text":"document"}]}]}`)
	f.Add(codecGzip(f, raw))
	f.Add(raw)
	f.Add([]byte{0x1f, 0x8b})
	f.Fuzz(func(t *testing.T, b []byte) {
		d, e := Read(context.Background(), bytes.NewReader(b), SFGzip, ReadOptions{})
		if e != nil {
			return
		}
		if e = d.Validate(); e != nil {
			t.Fatal(e)
		}
		var out bytes.Buffer
		if e = Write(context.Background(), &out, d, SFGzip, WriteOptions{}); e != nil {
			t.Fatal(e)
		}
	})
}
