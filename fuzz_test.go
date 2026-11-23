package manydoc

import (
	"bytes"
	"context"
	"math/rand"
	"testing"
	"time"
)

func TestMalformedInputsNeverPanic(t *testing.T) {
	rng := rand.New(rand.NewSource(20261123))
	for _, v := range []struct {
		name   string
		format Format
	}{{"docx_basic.docx", DOCX}, {"pdf_basic.pdf", PDF}} {
		original := fixture(t, v.name)
		for i := 0; i < 100; i++ {
			b := append([]byte(nil), original...)
			if i < 25 {
				b = b[:rng.Intn(len(b))]
			} else {
				for j := 0; j < 1+i%5; j++ {
					b[rng.Intn(len(b))] = byte(rng.Intn(256))
				}
			}
			func() {
				defer func() {
					if p := recover(); p != nil {
						t.Errorf("%s mutation %d panicked: %v", v.name, i, p)
					}
				}()
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				_, _ = Read(ctx, bytes.NewReader(b), v.format, ReadOptions{})
			}()
		}
	}
}
func FuzzReadDOCX(f *testing.F) {
	f.Add(fixture(f, "docx_basic.docx"))
	f.Add(fixture(f, "docx_wrong_root.docx"))
	f.Add([]byte(""))
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) > 2<<20 {
			t.Skip()
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_, _ = ReadDOCX(ctx, bytes.NewReader(b))
	})
}
func FuzzReadPDF(f *testing.F) {
	f.Add(fixture(f, "pdf_basic.pdf"))
	f.Add(fixture(f, "pdf_unicode.pdf"))
	f.Add([]byte(""))
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) > 2<<20 {
			t.Skip()
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_, _ = ReadPDF(ctx, bytes.NewReader(b), ReadOptions{})
	})
}
func FuzzUnicodeMap(f *testing.F) {
	f.Add([]byte(`1 beginbfchar <01> <0041> endbfchar`))
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) > 1<<16 {
			t.Skip()
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		m, e := parseUnicodeMap(ctx, b)
		if e == nil {
			_, _ = m.decode([]byte{0, 1, 2, 3})
		}
	})
}
