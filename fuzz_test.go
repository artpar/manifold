package manydoc

import (
	"bytes"
	"context"
	"math/rand"
	"os"
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
	var destinations bytes.Buffer
	if err := Write(context.Background(), &destinations, destinationDocument(), DOCX, WriteOptions{}); err != nil {
		f.Fatal(err)
	}
	f.Add(destinations.Bytes())

	f.Add(fixture(f, "docx_basic.docx"))
	f.Add(fixture(f, "docx_wrong_root.docx"))
	f.Add(docxWithBody(f, mergedTableOOXML))
	f.Add(numberedDOCX(f, numberedParagraph("10", "0", "parent")+numberedParagraph("10", "1", "child"), testListNumbering, ""))
	var lists bytes.Buffer
	if err := Write(context.Background(), &lists, listDocument(), DOCX, WriteOptions{}); err != nil {
		f.Fatal(err)
	}
	f.Add(lists.Bytes())
	var notes bytes.Buffer
	if err := Write(context.Background(), &notes, noteDocument(), DOCX, WriteOptions{}); err != nil {
		f.Fatal(err)
	}
	f.Add(notes.Bytes())
	var paragraphs bytes.Buffer
	if err := Write(context.Background(), &paragraphs, paragraphDocument(f), DOCX, WriteOptions{}); err != nil {
		f.Fatal(err)
	}
	f.Add(paragraphs.Bytes())
	var structural bytes.Buffer
	if err := Write(context.Background(), &structural, &Document{Blocks: []Block{{Kind: PageBreak}, cellTable([]Block{cellTable(nil)})}}, DOCX, WriteOptions{}); err != nil {
		f.Fatal(err)
	}
	f.Add(structural.Bytes())
	var headers bytes.Buffer
	if err := Write(context.Background(), &headers, repeatedHeaderDocument(f), DOCX, WriteOptions{}); err != nil {
		f.Fatal(err)
	}
	f.Add(headers.Bytes())
	var styled bytes.Buffer
	if err := Write(context.Background(), &styled, textStyleDocument(f), DOCX, WriteOptions{}); err != nil {
		f.Fatal(err)
	}
	f.Add(styled.Bytes())
	f.Add(docxWithBody(f, `<w:tbl><w:tr><w:trPr><w:tblHeader w:val="maybe"/></w:trPr><w:tc><w:p/></w:tc></w:tr></w:tbl>`))
	f.Add(notesDOCX(f, rawNoteBody, rawFootnotes, rawEndnotes))
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
	f.Add(recoveryPDF(explicitNoteObjects("FENote /A << /O /FENote /NoteType /Footnote >>")...))
	f.Add(recoveryPage(`/Artifact << /Type /Pagination /Subtype /Header /BBox [10 350 220 390] /Attached [/Top] >> BDC /Span << /ActualText (replacement) >> BDC BT /F 10 Tf 20 370 Td (HEADER) Tj ET EMC EMC`, ""))
	f.Add(navigationFixture(fullNavigationTree(), `/foo [6 0 R /FitH 100]`, `(foo)`))
	f.Add(recoveryPDF(taggedShortPatchObjects(true)...))
	f.Add(recoveryPDF(taggedShortPatchObjects(false)...))
	f.Add(recoveryPDF(inferenceObjects(ruledPath(false) + ruledContent())...))
	f.Add(fixture(f, "pdf_basic.pdf"))
	f.Add(fixture(f, "pdf_unicode.pdf"))
	f.Add(recoveryPDF(inferenceObjects(inferenceText("F", 10, 40, 300, "3. First") + inferenceText("F", 10, 40, 286, "4. Second") + inferenceText("F", 10, 40, 250, "Apples") + inferenceText("F", 10, 140, 250, "12") + inferenceText("F", 10, 40, 236, "Pears") + inferenceText("F", 10, 140, 236, "34"))...))
	published, err := os.ReadFile("testdata/real/copyright-office-circular-1-2021.pdf")
	if err != nil {
		f.Fatal(err)
	}
	f.Add(published)
	f.Add(taggedTestPDF(markedText(0, 20, 300, "tagged"), "<< /Type /StructTreeRoot /K 8 0 R >>", "<< /Type /StructElem /S /H2 /P 7 0 R /Pg 3 0 R /K 0 >>"))
	f.Add(taggedTestPDF(markedText(0, 20, 300, "cycle"), "<< /Type /StructTreeRoot /K 8 0 R >>", "<< /Type /StructElem /S /P /P 7 0 R /Pg 3 0 R /K 8 0 R >>"))

	f.Add(recoveryPage("q 2 0 0 3 10 20 cm BT /F 10 Tf 50 Tz 2 Ts 1 0 0 1 5 7 Tm (AB) Tj ET Q", ""))
	f.Add(recoveryPage("BT /F 10 Tf /P << /MCID 0 /ActualText (replacement) >> BDC (A) Tj EMC ET", ""))
	f.Add(recoveryPage("q 20 0 0 10 10 20 cm BI /W 2 /H 1 /BPC 8 /CS /RGB ID EI \x00\xff\x01 EI Q", ""))
	f.Add(recoveryPage("q 20 0 0 10 10 20 cm /X Do Q", "", recoveryStream("/Type /XObject /Subtype /Image /Width 2 /Height 1 /BitsPerComponent 8 /ColorSpace /DeviceRGB /SMask 7 0 R", string([]byte{255, 0, 0, 0, 255, 0})), recoveryStream("/Type /XObject /Subtype /Image /Width 2 /Height 1 /BitsPerComponent 8 /ColorSpace /DeviceGray", string([]byte{255, 0}))))

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
