package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"math/rand/v2"
	"os"
	"strings"
)

type fixture struct {
	Name                    string
	Pages, Lines, ImageEdge int
	Format                  string
}

var fixtures = []fixture{
	{"pdf-text-100", 100, 40, 0, "pdf"}, {"pdf-text-1000", 1000, 40, 0, "pdf"}, {"pdf-text-5000", 5000, 40, 0, "pdf"},
	{"pdf-image-100mb", 100, 1, 600, "pdf"}, {"pdf-image-300mb", 300, 1, 600, "pdf"},
	{"docx-text-1000", 1000, 20, 0, "docx"}, {"docx-text-5000", 5000, 20, 0, "docx"},
}

func token(page int) string { return fmt.Sprintf("PageToken%06d", page) }
func line(page, n int) string {
	if n == 0 {
		return token(page)
	}
	return fmt.Sprintf("Line %02d: deterministic visible text for scale, layout and extraction verification.", n)
}

// Independent minimal PDF writer: real visible text and distinct uncompressed RGB
// image streams, no padding/attachments, stable seed, correct xref byte offsets.
func writePDF(w io.Writer, f fixture) error {
	var offset int64
	var failure error
	write := func(s string) {
		if failure != nil {
			return
		}
		n, e := io.WriteString(w, s)
		offset += int64(n)
		failure = e
	}
	raw := func(b []byte) {
		if failure != nil {
			return
		}
		n, e := w.Write(b)
		offset += int64(n)
		failure = e
	}
	per := 2
	if f.ImageEdge > 0 {
		per = 3
	}
	count := 3 + f.Pages*per
	offsets := make([]int64, count+1)
	object := func(id int, s string) { offsets[id] = offset; write(fmt.Sprintf("%d 0 obj\n%s\nendobj\n", id, s)) }
	write("%PDF-1.7\n% synthetic manifold scale fixture\n")
	object(1, "<< /Type /Catalog /Pages 2 0 R >>")
	var kids strings.Builder
	for p := 0; p < f.Pages; p++ {
		fmt.Fprintf(&kids, "%d 0 R ", 4+p*per)
	}
	object(2, fmt.Sprintf("<< /Type /Pages /Count %d /Kids [%s] >>", f.Pages, kids.String()))
	object(3, "<< /Type /Font /Subtype /Type1 /BaseFont /Courier >>")
	rng := rand.New(rand.NewPCG(12345, 67890))
	for p := 0; p < f.Pages; p++ {
		id := 4 + p*per
		resources := "/Font << /F1 3 0 R >>"
		if f.ImageEdge > 0 {
			resources += fmt.Sprintf(" /XObject << /Im1 %d 0 R >>", id+2)
		}
		object(id, fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << %s >> /Contents %d 0 R >>", resources, id+1))
		var content strings.Builder
		content.WriteString("BT /F1 9 Tf 40 750 Td 14 TL\n")
		for n := 0; n < f.Lines; n++ {
			fmt.Fprintf(&content, "(%s) Tj T*\n", line(p+1, n))
		}
		content.WriteString("ET\n")
		if f.ImageEdge > 0 {
			content.WriteString("q 500 0 0 500 40 100 cm /Im1 Do Q\n")
		}
		object(id+1, fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream", content.Len(), content.String()))
		if f.ImageEdge > 0 {
			image := make([]byte, f.ImageEdge*f.ImageEdge*3)
			for i := range image {
				image[i] = byte(rng.Uint32())
			}
			offsets[id+2] = offset
			write(fmt.Sprintf("%d 0 obj\n<< /Type /XObject /Subtype /Image /Width %d /Height %d /ColorSpace /DeviceRGB /BitsPerComponent 8 /Length %d >>\nstream\n", id+2, f.ImageEdge, f.ImageEdge, len(image)))
			raw(image)
			write("\nendstream\nendobj\n")
		}
	}
	xref := offset
	write(fmt.Sprintf("xref\n0 %d\n0000000000 65535 f \n", count+1))
	for _, o := range offsets[1:] {
		write(fmt.Sprintf("%010d 00000 n \n", o))
	}
	write(fmt.Sprintf("trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", count+1, xref))
	return failure
}
func writeDOCX(w io.Writer, f fixture) error {
	z := zip.NewWriter(w)
	parts := []struct{ name, body string }{
		{"[Content_Types].xml", `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/></Types>`},
		{"_rels/.rels", `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/></Relationships>`},
	}
	for _, p := range parts {
		entry, e := z.Create(p.name)
		if e != nil {
			return e
		}
		if _, e = io.WriteString(entry, p.body); e != nil {
			return e
		}
	}
	entry, e := z.Create("word/document.xml")
	if e != nil {
		return e
	}
	var b bytes.Buffer
	b.WriteString(`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>`)
	for p := 1; p <= f.Pages; p++ {
		if p > 1 {
			b.WriteString(`<w:p><w:r><w:br w:type="page"/></w:r></w:p>`)
		}
		for n := 0; n < f.Lines; n++ {
			fmt.Fprintf(&b, `<w:p><w:pPr><w:spacing w:before="0" w:after="0"/></w:pPr><w:r><w:rPr><w:sz w:val="18"/></w:rPr><w:t>%s</w:t></w:r></w:p>`, line(p, n))
		}
	}
	b.WriteString(`<w:sectPr><w:pgSz w:w="12240" w:h="15840"/><w:pgMar w:top="720" w:right="720" w:bottom="720" w:left="720"/></w:sectPr></w:body></w:document>`)
	if _, e = entry.Write(b.Bytes()); e != nil {
		return e
	}
	return z.Close()
}
func generate(path string, f fixture) (int64, string, error) {
	out, e := os.Create(path)
	if e != nil {
		return 0, "", e
	}
	h := sha256.New()
	w := io.MultiWriter(out, h)
	if f.Format == "pdf" {
		e = writePDF(w, f)
	} else {
		e = writeDOCX(w, f)
	}
	ce := out.Close()
	if e != nil {
		return 0, "", e
	}
	if ce != nil {
		return 0, "", ce
	}
	st, e := os.Stat(path)
	if e != nil {
		return 0, "", e
	}
	return st.Size(), fmt.Sprintf("%x", h.Sum(nil)), nil
}
