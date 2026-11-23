package main

import (
	"bytes"
	"context"
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
