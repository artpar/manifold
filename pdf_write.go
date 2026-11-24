package manydoc

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math"
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
	return validateFontText(r, parsedPDFFonts[runStyle(r)])
}
func validateFontText(r Run, font *sfnt.Font) error {

	var buf sfnt.Buffer
	for _, ch := range norm.NFC.String(r.Text) {
		if ch == 0x200c || ch == 0x200d {
			continue
		}
		if pdfSpecial(ch) {
			required := []rune{' '}
			switch ch {
			case 0xad, 0x2010, 0x2011:
				required = append(required, '-')
			case 0x2007:
				required = append(required, '0')
			case 0x2008:
				required = append(required, '.')
			}
			for _, value := range required {
				glyph, err := font.GlyphIndex(&buf, value)
				if err != nil {
					return err
				}
				if glyph == 0 {
					return fmt.Errorf("%w: font lacks replacement/advance glyph %U for %U", ErrUnsupported, value, ch)
				}
			}
			continue
		}
		if unicode.In(ch, unicode.Devanagari, unicode.Bengali, unicode.Gurmukhi, unicode.Gujarati, unicode.Oriya, unicode.Tamil, unicode.Telugu, unicode.Kannada, unicode.Malayalam, unicode.Sinhala, unicode.Myanmar, unicode.Khmer, unicode.Thai, unicode.Lao, unicode.Tibetan, unicode.Mongolian) {
			return fmt.Errorf("%w: PDF output needs broader script shaping for %U", ErrUnsupported, ch)
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
	citation        *pdfCitationBox
	bidi            pdfBidiRange
	placed          bool
	visualX         float64
	run             Run
	text            string
	width           float64
	ascent, descent float64
	glyphs          *pdfGlyphCluster
}
type pdfLine []pdfSpan
type pdfRenderer struct {
	ctx                      context.Context
	outputStructure          *pdfOutputStructure
	suppressOutputStructure  bool
	destinationPoints        map[string]pdfDestinationPoint
	destinationTokens        map[string]string
	destinationURIs          map[string]string
	pdf                      *gofpdf.Fpdf
	width, height, margin, y float64
	images                   int
	header, footer           *pdfRunningStory
	content                  *pdfBodyBounds
	page                     *pageGeometry
	paintingRunning          bool
	sectioned                bool
	minimumBody              float64
	runningImages            map[[32]byte]string
	notes                    *noteCatalog
	noteLinks                map[noteKey]int
	seenFootnotes            map[noteKey]bool
	pageNotes, pendingNotes  []pdfNoteChunk
	noteReserve              float64
	validatedImages          outputImageCache
	fonts                    map[string]pdfFontFace
	fontMap                  map[string]string
	glyphFaces               map[string]*pdfGlyphFace
}

func writePDF(ctx context.Context, out io.Writer, d *Document) error {
	return writePDFWithFonts(ctx, out, d, nil, nil)
}
func writePDFWithFonts(ctx context.Context, out io.Writer, d *Document, fonts []FontFace, fontMap map[string]string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	d, structure := preparePDFOutputStructure(d)
	notes, err := catalogNotes(d, true)
	if err != nil {
		return err
	}
	g := pageLayout(d.Layout)
	if hasSectionBlocks(d.Blocks) {
		g = pageLayout(documentSections(d)[0].layout)
	}
	w, h, m := g.width, g.height, g.margins.Left
	if fmt.Sprintf("%.2f", w) == "0.00" || fmt.Sprintf("%.2f", h) == "0.00" {
		return fmt.Errorf("%w: PDF page dimensions round to zero", ErrUnsupported)
	}
	if w > 14400 || h > 14400 {
		return fmt.Errorf("%w: PDF page dimensions exceed 14400pt without UserUnit", ErrUnsupported)
	}
	p := gofpdf.NewCustom(&gofpdf.InitType{UnitStr: "pt", Size: gofpdf.SizeType{Wd: w, Ht: h}})
	p.SetMargins(g.margins.Left, math.Abs(g.margins.Top), g.margins.Right)
	p.SetAutoPageBreak(false, math.Abs(g.margins.Bottom))
	p.SetTitle(d.Metadata.Title, true)
	p.SetAuthor(d.Metadata.Author, true)
	p.SetSubject(d.Metadata.Subject, true)
	r := &pdfRenderer{outputStructure: structure, ctx: ctx, pdf: p, width: w, height: h, margin: m, page: &g, y: math.Abs(g.margins.Top), validatedImages: make(outputImageCache)}
	if err := r.prepareDestinations(d); err != nil {
		return err
	}
	r.fontMap = map[string]string{}
	for from, to := range fontMap {
		if from == "" || to == "" {
			return fmt.Errorf("empty PDF font mapping")
		}
		if err := validateTextStyle(Run{FontFamily: from}); err != nil {
			return err
		}
		if err := validateTextStyle(Run{FontFamily: to}); err != nil {
			return err
		}
		key := fontFamilyKey(from)
		if _, ok := r.fontMap[key]; ok {
			return fmt.Errorf("duplicate PDF font mapping")
		}
		r.fontMap[key] = to
	}
	if err := r.addFonts(fonts); err != nil {
		return err
	}

	if len(notes.order) > 0 {
		if _, err := r.registerFont(fontKey("Go", "")); err != nil {
			return err
		}
	}
	r.notes = notes
	r.noteLinks = map[noteKey]int{}
	r.seenFootnotes = map[noteKey]bool{}
	for _, key := range notes.order {
		r.noteLinks[key] = p.AddLink()
	}
	if hasSectionBlocks(d.Blocks) {
		if err := r.renderSections(d); err != nil {
			return err
		}
	} else {
		if len(d.Headers) > 0 || len(d.Footers) > 0 {
			minimum, err := r.minimumRunningBody(d.Blocks)
			if err != nil {
				return err
			}
			r.minimumBody = minimum
		}
		if err := r.prepareRunning(d.Headers, d.Footers, len(d.Blocks) > 0); err != nil {
			return err
		}
		footerBehind := g.margins.Bottom < 0
		p.SetHeaderFunc(func() {
			r.y = r.bodyTop()
			r.paintRunning(r.header)
			if footerBehind {
				r.paintRunning(r.footer)
			}
		})
		p.SetFooterFunc(func() {
			if !footerBehind {
				r.paintRunning(r.footer)
			}
		})

		p.AddPage()
		if err := r.blocks(d.Blocks, r.bodyWidth(), r.bodyLeft()); err != nil {
			return err
		}
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
	if err := ctx.Err(); err != nil {
		return err
	}
	// ActualText was introduced in PDF 1.4. gofpdf has no public minimum
	// version setter and otherwise emits 1.3 for opaque documents. This
	// equal-length header promotion leaves every object/xref offset unchanged.
	if bytes.HasPrefix(b.Bytes(), []byte("%PDF-1.3")) {
		b.Bytes()[7] = '4'
	}
	complete, err := r.finishGlyphFonts(b.Bytes())
	if err != nil {
		return err
	}
	_, err = io.Copy(out, bytes.NewReader(complete))
	return err
}
func (r *pdfRenderer) ensure(height float64) error { return r.ensureWithNotes(height, nil) }

func (r *pdfRenderer) blocks(bs []Block, width, x float64) error {
	for _, b := range legacyLists(bs) {
		if err := r.ctx.Err(); err != nil {
			return err
		}
		switch b.Kind {
		case AnchorPoint:
			r.bindDestinations(b.Anchors, r.y)
		case PageBreak:
			r.bindDestinations(b.Anchors, r.y)
			if err := r.nextPage(true); err != nil {
				return err
			}
		case Paragraph, Heading:
			if err := r.paragraphBreakBefore(b); err != nil {
				return err
			}
			size := 11.0
			runs := append([]Run(nil), b.Runs...)
			if b.Kind == Heading {
				size = 26 - float64(b.Level)*2
				for i := range runs {
					runs[i].Bold = true
				}
			}

			lines, err := r.linesDirection(runs, width, size, b.Direction)
			if err != nil {
				return err
			}
			leading := pdfLineHeight(lines[0], size, 1.4)
			if b.Kind == Heading {
				if err := r.ensure(math.Min(leading*2, r.bodyHeight())); err != nil {
					return err
				}
			}
			for lineIndex, line := range lines {
				leading = pdfLineHeight(line, size, 1.4)
				if err := r.ensureWithNotes(leading, lineNoteKeys(line)); err != nil {
					return err
				}
				if lineIndex == 0 {
					r.bindDestinations(b.Anchors, r.y)
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
			im, err := r.measureImage(b.Image, width, r.bodyHeight()-8)
			if err != nil {
				return err
			}
			if err := r.ensure(im.height + 8); err != nil {
				return err
			}
			r.bindDestinations(b.Anchors, r.y)
			if err := r.drawImage(im, x, r.y); err != nil {
				return err
			}
			r.y += im.height + 8
		}
	}
	return r.pdf.Error()
}

func (r *pdfRenderer) paragraphBreakBefore(b Block) error {
	if b.PageBreakBefore && r.y > r.bodyTop()+1e-7 {
		return r.nextPage(true)
	}
	return nil
}
