package manydoc

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sync"
	"unicode"

	"github.com/phpdave11/gofpdf"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/gobolditalic"
	"golang.org/x/image/font/gofont/goitalic"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/text/unicode/norm"
)

var pdfFonts = []struct {
	style string
	data  []byte
}{{"", goregular.TTF}, {"B", gobold.TTF}, {"I", goitalic.TTF}, {"BI", gobolditalic.TTF}}
var pdfFontOnce sync.Once
var parsedPDFFonts map[string]*sfnt.Font
var pdfFontError error

func runStyle(r Run) string {
	s := ""
	if r.Bold {
		s += "B"
	}
	if r.Italic {
		s += "I"
	}
	return s
}
func validatePDFText(r Run) error {
	pdfFontOnce.Do(func() {
		parsedPDFFonts = map[string]*sfnt.Font{}
		for _, f := range pdfFonts {
			p, err := sfnt.Parse(f.data)
			if err != nil {
				pdfFontError = err
				return
			}
			parsedPDFFonts[f.style] = p
		}
	})
	if pdfFontError != nil {
		return pdfFontError
	}
	font := parsedPDFFonts[runStyle(r)]
	var buf sfnt.Buffer
	for _, ch := range norm.NFC.String(r.Text) {
		if pdfSpecial(ch) {
			continue
		}
		if unicode.Is(unicode.Arabic, ch) || unicode.Is(unicode.Devanagari, ch) || unicode.Is(unicode.Hebrew, ch) {
			return fmt.Errorf("%w: PDF output needs shaping/bidirectional layout for %U", ErrUnsupported, ch)
		}
		if unicode.Is(unicode.Mn, ch) || unicode.Is(unicode.Mc, ch) || unicode.Is(unicode.Me, ch) {
			return fmt.Errorf("%w: PDF text needs combining-mark positioning for %U", ErrUnsupported, ch)
		}
		g, err := font.GlyphIndex(&buf, ch)
		if err != nil {
			return err
		}
		if g == 0 {
			return fmt.Errorf("%w: embedded PDF font has no glyph for %U", ErrUnsupported, ch)
		}
	}
	return nil
}

type pdfSpan struct {
	run   Run
	text  string
	width float64
}
type pdfLine []pdfSpan
type pdfRenderer struct {
	ctx                      context.Context
	pdf                      *gofpdf.Fpdf
	width, height, margin, y float64
	images                   int
	headers, footers         []Block
	notes                    *noteCatalog
	noteLinks                map[noteKey]int
	seenFootnotes            map[noteKey]bool
	pageNotes, pendingNotes  []pdfNoteChunk
	noteReserve              float64
}

func writePDF(ctx context.Context, out io.Writer, d *Document) error {
	notes, err := catalogNotes(d, true)
	if err != nil {
		return err
	}
	w, h, m := layout(d.Layout)
	if w > 14400 || h > 14400 {
		return fmt.Errorf("%w: PDF page dimensions exceed 14400pt without UserUnit", ErrUnsupported)
	}
	p := gofpdf.NewCustom(&gofpdf.InitType{UnitStr: "pt", Size: gofpdf.SizeType{Wd: w, Ht: h}})
	p.SetMargins(m, m, m)
	p.SetAutoPageBreak(false, m)
	p.SetTitle(d.Metadata.Title, true)
	p.SetAuthor(d.Metadata.Author, true)
	p.SetSubject(d.Metadata.Subject, true)
	for _, f := range pdfFonts {
		// gofpdf's subset/checksum implementation mutates the supplied TTF bytes.
		// Each renderer owns its buffer; sharing embedded font slices races.
		p.AddUTF8FontFromBytes("Go", f.style, append([]byte(nil), f.data...))
	}
	r := &pdfRenderer{ctx: ctx, pdf: p, width: w, height: h, margin: m, y: m, headers: d.Headers, footers: d.Footers}
	r.notes = notes
	r.noteLinks = map[noteKey]int{}
	r.seenFootnotes = map[noteKey]bool{}
	for _, key := range notes.order {
		r.noteLinks[key] = p.AddLink()
	}
	// Headers and footers use an explicit small-text contract to avoid recursive
	// pagination or dropping nontext content.
	for _, blocks := range [][]Block{d.Headers, d.Footers} {
		for _, b := range blocks {
			if (b.Kind != Paragraph && b.Kind != Heading) || b.List != "" {
				return fmt.Errorf("%w: PDF header/footer must contain paragraphs", ErrUnsupported)
			}
		}
	}
	p.SetHeaderFunc(func() { r.y = m; r.headerFooter(d.Headers, m/2) })
	p.SetFooterFunc(func() { r.headerFooter(d.Footers, h-m/2) })
	p.AddPage()
	if err := r.blocks(d.Blocks, w-2*m, m); err != nil {
		return err
	}
	if err := r.finishNotes(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if p.Error() != nil {
		return p.Error()
	}
	var b bytes.Buffer
	if err := p.Output(&b); err != nil {
		return err
	}
	// ActualText was introduced in PDF 1.4. gofpdf has no public minimum
	// version setter and otherwise emits 1.3 for opaque documents. This
	// equal-length header promotion leaves every object/xref offset unchanged.
	if bytes.HasPrefix(b.Bytes(), []byte("%PDF-1.3")) {
		b.Bytes()[7] = '4'
	}
	_, err = io.Copy(out, &b)
	return err
}
func (r *pdfRenderer) headerFooter(bs []Block, y float64) {
	header := y < r.margin
	for _, b := range bs {
		lines, err := r.lines(b.Runs, r.width-2*r.margin, 8)
		if err != nil {
			r.pdf.SetError(err)
			return
		}
		for _, line := range lines {
			if (header && y+10 > r.margin) || (!header && y+10 > r.height) {
				r.pdf.SetError(fmt.Errorf("%w: PDF header/footer exceeds reserved area", ErrUnsupported))
				return
			}
			r.drawLine(line, r.margin, y, 8)
			y += 10
		}
	}
}
func (r *pdfRenderer) ensure(height float64) error { return r.ensureWithNotes(height, nil) }

func (r *pdfRenderer) blocks(bs []Block, width, x float64) error {
	for _, b := range legacyLists(bs) {
		if err := r.ctx.Err(); err != nil {
			return err
		}
		switch b.Kind {
		case PageBreak:
			if err := r.nextPage(true); err != nil {
				return err
			}
		case Paragraph, Heading:
			size := 11.0
			runs := append([]Run(nil), b.Runs...)
			if b.Kind == Heading {
				size = 26 - float64(b.Level)*2
				for i := range runs {
					runs[i].Bold = true
				}
			}

			lines, err := r.lines(runs, width, size)
			if err != nil {
				return err
			}
			leading := size * 1.4
			if b.Kind == Heading {
				if err := r.ensure(leading * 2); err != nil {
					return err
				}
			}
			for _, line := range lines {
				if err := r.ensureWithNotes(leading, lineNoteKeys(line)); err != nil {
					return err
				}
				r.drawLine(line, x, r.y, size)
				r.y += leading
			}
			r.y += size * .4
		case ListBlock:
			if err := r.list(b, width, x); err != nil {
				return err
			}
		case Table:
			if err := r.table(b, width, x); err != nil {
				return err
			}
			r.y += 8
		case Picture:
			im, err := r.measureImage(b.Image, width)
			if err != nil {
				return err
			}
			if err := r.ensure(im.height + 8); err != nil {
				return err
			}
			if err := r.drawImage(im, x, r.y); err != nil {
				return err
			}
			r.y += im.height + 8
		}
	}
	return r.pdf.Error()
}
