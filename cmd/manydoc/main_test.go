package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIConversion(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "input.txt")
	out := filepath.Join(dir, "output.pdf")
	if e := os.WriteFile(in, []byte("Hello manydoc\ncafé Ελληνικά Привет"), 0600); e != nil {
		t.Fatal(e)
	}
	var stdout, stderr bytes.Buffer
	if e := run(context.Background(), []string{"convert", "-o", out, in}, &stdout, &stderr); e != nil {
		t.Fatal(e)
	}
	stdout.Reset()
	stderr.Reset()
	if e := run(context.Background(), []string{"extract", out}, &stdout, &stderr); e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(stdout.String(), "Hello manydoc") || !strings.Contains(stdout.String(), "Привет") {
		t.Fatal(stdout.String())
	}
}
func TestCLIAtomicFailure(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "unsupported.txt")
	out := filepath.Join(dir, "existing.pdf")
	if e := os.WriteFile(in, []byte("中文"), 0600); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(out, []byte("KEEP EXISTING OUTPUT"), 0600); e != nil {
		t.Fatal(e)
	}
	if e := run(context.Background(), []string{"convert", "-o", out, in}, &bytes.Buffer{}, &bytes.Buffer{}); e == nil {
		t.Fatal("unsupported glyph accepted")
	}
	b, e := os.ReadFile(out)
	if e != nil {
		t.Fatal(e)
	}
	if string(b) != "KEEP EXISTING OUTPUT" {
		t.Fatal("existing output changed on failure")
	}
	files, e := filepath.Glob(filepath.Join(dir, ".manydoc-*"))
	if e != nil || len(files) > 0 {
		t.Fatal("temporary output leaked", files, e)
	}
}
func TestCLIUsage(t *testing.T) {
	for _, args := range [][]string{{}, {"help"}, {"convert", "-h"}} {
		if e := run(context.Background(), args, &bytes.Buffer{}, &bytes.Buffer{}); e != nil {
			t.Fatal(e)
		}
	}
	for _, args := range [][]string{{"unknown"}, {"convert"}, {"extract", "-format", "pdf", "x.docx"}} {
		if e := run(context.Background(), args, &bytes.Buffer{}, &bytes.Buffer{}); e == nil {
			t.Fatal("bad usage accepted", args)
		}
	}
}

func TestCLIStructuredPaths(t *testing.T) {
	dir := t.TempDir()
	sf := filepath.Join(dir, "document.sf")
	input := filepath.Join("..", "..", "testdata", "corpus", "docx_basic.docx")
	if err := run(context.Background(), []string{"extract", "-format", "sf", "-o", sf, input}, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	for _, ext := range []string{"docx", "pdf"} {
		rendered := filepath.Join(dir, "rendered."+ext)
		if err := run(context.Background(), []string{"convert", "-allow-lossy", "-pdf-font-map", "Calibri=Go", "-pdf-font-map", "Cambria=Go", "-o", rendered, sf}, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		if err := run(context.Background(), []string{"extract", "-format", "sf", rendered}, &out, &bytes.Buffer{}); err != nil {
			t.Fatal(err)
		}
		var tree struct {
			Attrs struct {
				Schema string `json:"schema"`
			} `json:"attrs"`
		}
		if err := json.Unmarshal(out.Bytes(), &tree); err != nil {
			t.Fatal(err)
		}
		if tree.Attrs.Schema != "manydoc-prosemirror/1" {
			t.Fatal("missing structured schema")
		}
	}
}

func TestCLIGzipStructuredPaths(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join("..", "..", "testdata", "corpus", "docx_basic.docx")
	compressed := filepath.Join(dir, "document.sf.gz")
	ctx := context.Background()
	if e := run(ctx, []string{"extract", "-format", "sf.gz", "-o", compressed, input}, &bytes.Buffer{}, &bytes.Buffer{}); e != nil {
		t.Fatal(e)
	}
	for _, ext := range []string{"sf", "docx", "pdf"} {
		out := filepath.Join(dir, "roundtrip."+ext)
		if e := run(ctx, []string{"convert", "-allow-lossy", "-pdf-font-map", "Calibri=Go", "-pdf-font-map", "Cambria=Go", "-o", out, compressed}, &bytes.Buffer{}, &bytes.Buffer{}); e != nil {
			t.Fatal(ext, e)
		}
	}
	// A corrupted trailer is detected before atomic output replacement.
	b, e := os.ReadFile(compressed)
	if e != nil {
		t.Fatal(e)
	}
	b[len(b)-8] ^= 1
	if e = os.WriteFile(compressed, b, 0600); e != nil {
		t.Fatal(e)
	}
	keep := filepath.Join(dir, "keep.sf")
	if e = os.WriteFile(keep, []byte("KEEP"), 0600); e != nil {
		t.Fatal(e)
	}
	if e = run(ctx, []string{"convert", "-o", keep, compressed}, &bytes.Buffer{}, &bytes.Buffer{}); e == nil {
		t.Fatal("bad checksum accepted")
	}
	after, e := os.ReadFile(keep)
	if e != nil || string(after) != "KEEP" {
		t.Fatal("atomic output changed", e)
	}
}
