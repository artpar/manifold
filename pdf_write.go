package manydoc

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"io"
	"math"
	"regexp"
	"strings"
	"sync"
	"unicode"

	"github.com/phpdave11/gofpdf"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/gobolditalic"
	"golang.org/x/image/font/gofont/goitalic"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/sfnt"
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
	for _, ch := range r.Text {
		if ch == '\n' || ch == '\r' || ch == '\t' {
			continue
		}
		if unicode.Is(unicode.Arabic, ch) || unicode.Is(unicode.Devanagari, ch) || unicode.Is(unicode.Hebrew, ch) {
			return fmt.Errorf("%w: PDF output needs shaping/bidirectional layout for %U", ErrUnsupported, ch)
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
}

var wordTokens = regexp.MustCompile(`\n|\t| +|[^ \n\t\r]+`)

func writePDF(ctx context.Context, out io.Writer, d *Document) error {
	if len(d.Notes) > 0 {
		return fmt.Errorf("%w: PDF note layout is not implemented", ErrUnsupported)
	}
	w, h, m := layout(d.Layout)
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
	// Headers and footers use an explicit small-text contract to avoid recursive
	// pagination or dropping nontext content.
	for _, blocks := range [][]Block{d.Headers, d.Footers} {
		for _, b := range blocks {
			if b.Kind != Paragraph && b.Kind != Heading {
				return fmt.Errorf("%w: PDF header/footer must contain paragraphs", ErrUnsupported)
			}
			for _, run := range b.Runs {
				if err := validatePDFText(run); err != nil {
					return err
				}
			}
		}
	}
	p.SetHeaderFunc(func() { r.y = m; r.headerFooter(d.Headers, m/2) })
	p.SetFooterFunc(func() { r.headerFooter(d.Footers, h-m/2) })
	p.AddPage()
	if err := r.blocks(d.Blocks, w-2*m, m); err != nil {
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
	_, err := io.Copy(out, &b)
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
func (r *pdfRenderer) ensure(height float64) error {
	if height > r.height-2*r.margin {
		return fmt.Errorf("%w: PDF block is taller than a page", ErrUnsupported)
	}
	if r.y+height > r.height-r.margin {
		r.pdf.AddPage()
		r.y = r.margin
	}
	return r.ctx.Err()
}
func (r *pdfRenderer) lines(runs []Run, width, size float64) ([]pdfLine, error) {
	if width <= 0 {
		return nil, fmt.Errorf("invalid PDF text width")
	}
	lines := []pdfLine{{}}
	used := 0.0
	add := func(run Run, text string, w float64) {
		i := len(lines) - 1
		n := len(lines[i])
		if n > 0 && lines[i][n-1].run == run {
			lines[i][n-1].text += text
			lines[i][n-1].width += w
		} else {
			lines[i] = append(lines[i], pdfSpan{run, text, w})
		}
		used += w
	}
	newline := func() { lines = append(lines, pdfLine{}); used = 0 }
	for _, run := range runs {
		if err := validatePDFText(run); err != nil {
			return nil, err
		}
		r.pdf.SetFont("Go", runStyle(run), size)
		for _, word := range wordTokens.FindAllString(run.Text, -1) {
			if word == "\n" {
				newline()
				continue
			}
			if word == "\t" {
				word = "    "
			}
			space := strings.Trim(word, " ") == ""
			ww := r.pdf.GetStringWidth(word)
			if used+ww > width && used > 0 {
				newline()
				if space {
					continue
				}
			}
			if ww <= width {
				add(run, word, ww)
				continue
			}
			for _, ch := range word {
				t := string(ch)
				cw := r.pdf.GetStringWidth(t)
				if cw > width {
					return nil, fmt.Errorf("%w: glyph wider than PDF content area", ErrUnsupported)
				}
				if used+cw > width {
					newline()
				}
				add(run, t, cw)
			}
		}
	}
	return lines, nil
}
func (r *pdfRenderer) drawLine(line pdfLine, x, y, size float64) {
	for _, span := range line {
		style := runStyle(span.run)
		if span.run.Underline {
			style += "U"
		}
		r.pdf.SetFont("Go", style, size)
		r.pdf.Text(x, y+size, span.text)
		if span.run.Link != "" {
			r.pdf.LinkString(x, y, span.width, size*1.3, span.run.Link)
		}
		if span.run.Strike {
			r.pdf.Line(x, y+size*.6, x+span.width, y+size*.6)
		}
		x += span.width
	}
}
func (r *pdfRenderer) blocks(bs []Block, width, x float64) error {
	number := 0
	for _, b := range bs {
		if err := r.ctx.Err(); err != nil {
			return err
		}
		switch b.Kind {
		case PageBreak:
			r.pdf.AddPage()
			r.y = r.margin
		case Paragraph, Heading:
			size := 11.0
			runs := append([]Run(nil), b.Runs...)
			if b.Kind == Heading {
				size = 26 - float64(b.Level)*2
				for i := range runs {
					runs[i].Bold = true
				}
			}
			if b.List != "" {
				prefix := "• "
				if b.List == "number" {
					number++
					prefix = fmt.Sprintf("%d. ", number)
				}
				runs = append([]Run{{Text: prefix}}, runs...)
			} else {
				number = 0
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
				if err := r.ensure(leading); err != nil {
					return err
				}
				r.drawLine(line, x, r.y, size)
				r.y += leading
			}
			r.y += size * .4
		case Table:
			if err := r.table(b, width, x); err != nil {
				return err
			}
			r.y += 8
		case Picture:
			im := b.Image
			conf, kind, err := image.DecodeConfig(bytes.NewReader(im.Data))
			if err != nil {
				return err
			}
			if kind != "png" && kind != "jpeg" {
				return fmt.Errorf("%w: PDF image %s", ErrUnsupported, kind)
			}
			iw, ih := im.Width, im.Height
			if iw == 0 {
				iw = float64(conf.Width) * .75
			}
			if ih == 0 {
				ih = iw * float64(conf.Height) / float64(conf.Width)
			}
			if iw > width {
				ih *= width / iw
				iw = width
			}
			if err := r.ensure(ih + 8); err != nil {
				return err
			}
			r.images++
			name := fmt.Sprintf("image%d", r.images)
			r.pdf.RegisterImageOptionsReader(name, gofpdf.ImageOptions{ImageType: kind}, bytes.NewReader(im.Data))
			r.pdf.ImageOptions(name, x, r.y, iw, ih, false, gofpdf.ImageOptions{ImageType: kind}, 0, "")
			r.y += ih + 8
		}
	}
	return r.pdf.Error()
}
func (r *pdfRenderer) table(b Block, width, x float64) error {
	cols := 0
	for _, row := range b.Rows {
		n := 0
		for _, c := range row.Cells {
			n += max(1, c.ColSpan)
		}
		cols = max(cols, n)
	}
	if cols == 0 {
		return nil
	}
	unit := width / float64(cols)
	for _, row := range b.Rows {
		var cells [][]pdfLine
		var widths []float64
		rowHeight := 22.0
		for _, cell := range row.Cells {
			cw := unit * float64(max(1, cell.ColSpan))
			var runs []Run
			for i, b := range cell.Blocks {
				if b.Kind != Paragraph && b.Kind != Heading {
					return fmt.Errorf("%w: PDF table cells support text paragraphs", ErrUnsupported)
				}
				if i > 0 {
					runs = append(runs, Run{Text: "\n"})
				}
				runs = append(runs, b.Runs...)
			}
			lines, err := r.lines(runs, cw-12, 10)
			if err != nil {
				return err
			}
			cells = append(cells, lines)
			widths = append(widths, cw)
			rowHeight = math.Max(rowHeight, float64(len(lines))*14+12)
		}
		if err := r.ensure(rowHeight); err != nil {
			return err
		}
		cx := x
		for i, lines := range cells {
			r.pdf.SetDrawColor(120, 120, 120)
			r.pdf.Rect(cx, r.y, widths[i], rowHeight, "D")
			for j, line := range lines {
				r.drawLine(line, cx+6, r.y+6+float64(j)*14, 10)
			}
			cx += widths[i]
		}
		r.y += rowHeight
	}
	return nil
}
