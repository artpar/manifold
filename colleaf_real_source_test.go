package manifold

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"testing"
)

// This publication exercises combined source bindings from an independent PDF
// producer. Its structural oracle remains in TestPDFPublishedInDesignExport.
func TestColleafPublishedPDFSourceGraph(t *testing.T) {
	ctx := context.Background()
	raw, err := os.ReadFile("testdata/real/copyright-office-circular-1-2021.pdf")
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprintf("%x", sha256.Sum256(raw)) != "a37450360b4b440cb10ccfd9ce37cb92ea8193884c5700948df942aa1c62920f" {
		t.Fatal("original publication changed")
	}
	want, err := ReadPDF(ctx, bytes.NewReader(raw), ReadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	original, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	current := want
	for pass := 0; pass < 2; pass++ {
		native, err := encodeColleafDocument(ctx, current)
		if err != nil {
			t.Fatal(err)
		}
		wire, err := json.Marshal(native)
		if err != nil {
			t.Fatal(err)
		}
		if pass == 0 {
			if file := os.Getenv("MANYDOC_COLLEAF_REAL_QA"); file != "" {
				if err := os.WriteFile(file, wire, 0600); err != nil {
					t.Fatal(err)
				}
			}
		}
		got, err := decodeColleafDocument(ctx, native)
		if err != nil {
			t.Fatal(err)
		}
		after, _ := json.Marshal(native)
		if !bytes.Equal(wire, after) {
			t.Fatal("native snapshot mutated")
		}
		assertEvidenceSF(t, want, got)
		current = got
		unchanged, _ := json.Marshal(want)
		if !bytes.Equal(original, unchanged) {
			t.Fatal("original source document mutated")
		}
	}
	if file := os.Getenv("MANYDOC_COLLEAF_REAL_SAVED"); file != "" {
		wire, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var native colleafNoteSnapshot
		if err := json.Unmarshal(wire, &native); err != nil {
			t.Fatal(err)
		}
		envelope, err := json.Marshal(colleafEnvelope{colleafEnvelopeType, 3, "univer", colleafEngineVersion, native})
		if err != nil {
			t.Fatal(err)
		}
		got, err := readColleaf(ctx, bytes.NewReader(envelope))
		if err != nil {
			t.Fatal(err)
		}
		assertEvidenceSF(t, want, got)
		again, err := encodeColleafDocument(ctx, got)
		if err != nil {
			t.Fatal(err)
		}
		reopened, err := decodeColleafDocument(ctx, again)
		if err != nil {
			t.Fatal(err)
		}
		assertEvidenceSF(t, want, reopened)
		sf, err := ToStructured(got)
		if err != nil {
			t.Fatal(err)
		}
		data, err := json.Marshal(sf)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file+".sf", data, 0600); err != nil {
			t.Fatal(err)
		}
	}
}
