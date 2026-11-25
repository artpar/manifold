package engine

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"testing"
)

func TestOfficeMathNativePreservation(t *testing.T) {
	raw := `<m:oMath xmlns:m="` + officeMathNS + `"><m:f><m:num><m:r><m:t>a</m:t></m:r></m:num><m:den><m:r><m:t>b</m:t></m:r></m:den></m:f></m:oMath>`
	d := &Document{Blocks: []Block{{Kind: OfficeMathBlock, Math: &OfficeMath{OMML: raw}}}}
	for _, format := range []Format{SF, DOCX} {
		var out bytes.Buffer
		if err := Write(context.Background(), &out, d, format, WriteOptions{AllowLossy: true}); err != nil {
			t.Fatal(err)
		}
		got, err := Read(context.Background(), bytes.NewReader(out.Bytes()), format, ReadOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if got.Text() != "ab" || !documentHasOfficeMath(got) {
			t.Fatalf("native math lost in %s", format)
		}
	}
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, PDF, WriteOptions{AllowLossy: true}); !errors.Is(err, ErrUnsupported) || out.Len() != 0 {
		t.Fatalf("unsupported math output: %v bytes %d", err, out.Len())
	}
	for _, raw := range []string{`<oMath/>`, `<!DOCTYPE x><oMath/>`, `<oMath xmlns="` + officeMathNS + `"><r xmlns:r="` + relNS + `" r:id="x"/></oMath>`} {
		if _, err := parseOfficeMath(raw); err == nil {
			t.Fatal("accepted unsafe or unbound math")
		}
	}
}
func TestHugeOriginalWordAssets(t *testing.T) {
	if os.Getenv("MANIFOLD_HUGE_WORD") == "" {
		t.Skip("downloaded original corpus only")
	}
	for _, version := range []string{"i00", "i10"} {
		t.Run(version, func(t *testing.T) {
			path := "output/hugetest-real/38331-" + version + ".docx"
			z, err := zip.OpenReader(path)
			if err != nil {
				t.Fatal(err)
			}
			defer z.Close()
			originals := map[[32]byte]bool{}
			for _, f := range z.File {
				if len(f.Name) > 11 && f.Name[:11] == "word/media/" {
					r, e := f.Open()
					if e != nil {
						t.Fatal(e)
					}
					var b bytes.Buffer
					b.ReadFrom(r)
					r.Close()
					originals[sha256.Sum256(b.Bytes())] = true
				}
			}
			f, err := os.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			d, err := Read(context.Background(), f, DOCX, ReadOptions{})
			f.Close()
			if err != nil {
				t.Fatal(err)
			}
			for _, format := range []Format{SF, DOCX} {
				var out bytes.Buffer
				if err := Write(context.Background(), &out, d, format, WriteOptions{AllowLossy: true}); err != nil {
					t.Fatal(err)
				}
				got, e := Read(context.Background(), bytes.NewReader(out.Bytes()), format, ReadOptions{})
				if e != nil {
					t.Fatal(e)
				}
				count, mathCount := 0, 0
				seen := map[[32]byte]bool{}
				var walk func([]Block)
				walk = func(bs []Block) {
					for _, b := range bs {
						if b.Image != nil {
							count++
							seen[sha256.Sum256(b.Image.Data)] = true
						}
						if b.Math != nil {
							mathCount++
						}
						if b.Section != nil {
							walk(b.Section.Blocks)
						}
						for _, r := range b.Rows {
							for _, c := range r.Cells {
								walk(c.Blocks)
							}
						}
						for _, i := range b.Items {
							walk(i.Blocks)
						}
					}
				}
				walk(got.Blocks)
				expectedMath := 15
				if version == "i10" {
					expectedMath = 21
				}
				if count != 73 || len(seen) != 71 || mathCount != expectedMath {
					t.Fatalf("%s images=%d unique=%d math=%d", format, count, len(seen), mathCount)
				}
				for h := range originals {
					if !seen[h] {
						t.Fatalf("original asset missing %x", h)
					}
				}
				if format == SF {
					os.MkdirAll("output/huge-word-fixed", 0700)
					if err := os.WriteFile(fmt.Sprintf("output/huge-word-fixed/38331-%s.sf", version), out.Bytes(), 0600); err != nil {
						t.Fatal(err)
					}
				}
				t.Logf("%s: %d exact asset placements / %d unique assets, %d native equations", format, count, len(seen), mathCount)
			}
		})
	}
}
