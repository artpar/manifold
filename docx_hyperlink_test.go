package manifold

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"testing"
)

func hyperlinkDOCX(t *testing.T, body, target string) []byte {
	t.Helper()
	base := docxWithBody(t, body)
	z, err := zip.NewReader(bytes.NewReader(base), int64(len(base)))
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	w := zip.NewWriter(&out)
	for _, f := range z.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		part, err := w.Create(f.Name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = part.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	part, err := w.Create("word/_rels/document.xml.rels")
	if err != nil {
		t.Fatal(err)
	}
	_, err = part.Write([]byte(`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="link" Type="` + relNS + `/hyperlink" Target="` + escape(target) + `" TargetMode="External"/></Relationships>`))
	if err != nil {
		t.Fatal(err)
	}
	if err = w.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func TestDOCXRelationshipAndAnchorRetainWordTarget(t *testing.T) {
	for _, c := range []struct{ base, anchor, want string }{
		{"https://example.invalid/path?q=yes", "section", "https://example.invalid/path?q=yes#section"},
		{"https://example.invalid/é?x=1&y=2", "章节–é", "https://example.invalid/é?x=1&y=2#章节–é"},
		{"https://example.invalid/#existing", "next", "https://example.invalid/#existing#next"},
		{"https://example.invalid/%C3%A9", "%2Falready", "https://example.invalid/%C3%A9#%2Falready"},
		{"https://example.invalid/plain", "", "https://example.invalid/plain"},
	} {
		body := `<w:p><w:hyperlink xmlns:r="` + relNS + `" r:id="link" w:anchor="` + escape(c.anchor) + `"><w:r><w:rPr><w:b/></w:rPr><w:t>LABEL</w:t></w:r></w:hyperlink></w:p>`
		d, err := ReadDOCX(context.Background(), bytes.NewReader(hyperlinkDOCX(t, body, c.base)))
		if err != nil {
			t.Fatal(err)
		}
		run := d.Blocks[0].Runs[0]
		if run.Link != c.want || run.Text != "LABEL" || !run.Bold {
			t.Fatal("link identity, content or formatting changed", run, c)
		}
		if hasDiagnostic(d, "hyperlink_word_anchor") != (c.anchor != "") {
			t.Fatal(d.Diagnostics)
		}
		for _, format := range []Format{SF, DOCX, PDF} {
			var out bytes.Buffer
			if err := Write(context.Background(), &out, d, format, WriteOptions{AllowLossy: true}); err != nil {
				t.Fatal(format, err)
			}
			back, err := Read(context.Background(), &out, format, ReadOptions{PDFSkipInference: true})
			if err != nil {
				t.Fatal(format, err)
			}
			want := c.want
			if format == PDF {
				want = pdfASCIIURI(want)
			}
			if back.Blocks[0].Runs[0].Link != want {
				t.Fatal(format, want, back.Blocks[0].Runs)
			}
		}
	}
}
