package engine

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math"
	"strconv"
	"unicode/utf8"

	"github.com/phpdave11/gofpdf"
)

// writePresentationPDF paints canonical SF objects as native PDF text, vector
// shapes and images. It never converts whole slides to opaque page bitmaps.
func writePresentationPDF(ctx context.Context, out io.Writer, d *Document, opts WriteOptions) error {
	pres := d.Presentation
	if len(pres.Slides) == 0 {
		return fmt.Errorf("%w: an empty deck has no PDF pages", ErrUnsupported)
	}
	p := gofpdf.NewCustom(&gofpdf.InitType{UnitStr: "pt", Size: gofpdf.SizeType{Wd: pres.Width, Ht: pres.Height}})
	p.SetAutoPageBreak(false, 0)
	p.SetMargins(0, 0, 0)
	p.SetTitle(d.Metadata.Title, true)
	p.SetAuthor(d.Metadata.Author, true)
	p.SetSubject(d.Metadata.Subject, true)
	r := &pdfRenderer{ctx: ctx, pdf: p, fontMap: map[string]string{}, validatedImages: make(outputImageCache)}
	for from, to := range opts.PDFFontMap {
		r.fontMap[fontFamilyKey(from)] = to
	}
	if err := r.addFonts(opts.PDFFonts); err != nil {
		return err
	}
	visible := false
	for _, slide := range pres.Slides {
		if !slide.Hidden {
			visible = true
		}
	}
	if !visible {
		return fmt.Errorf("%w: no visible presentation slides", ErrUnsupported)
	}
	resources := map[string]PresentationResource{}
	for _, res := range pres.Resources {
		resources[res.Part] = res
	}
	for _, slide := range pres.Slides {
		if slide.Hidden {
			continue
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if len(slide.Unsupported) > 0 {
			return fmt.Errorf("%w: PDF cannot paint native slide feature: %s", ErrUnsupported, slide.Unsupported[0])
		}
		if len(slide.Notes) > 0 && !opts.AllowLossy {
			return fmt.Errorf("%w: speaker notes remain in SF/PPTX but are not visible PDF page content", ErrLossy)
		}
		w, h := slideDimensions(pres, slide)
		g := pageLayout(Layout{Width: w, Height: h, Margins: &PageMargins{}})
		r.page = &g
		r.width = w
		r.height = h
		r.margin = 0
		p.AddPageFormat("P", gofpdf.SizeType{Wd: w, Ht: h})
		background := "#FFFFFF"
		for _, part := range []string{slide.MasterPart, slide.LayoutPart} {
			if res, ok := resources[part]; ok && res.Background != "" && res.Background != "none" {
				background = res.Background
			}
		}
		if slide.Background != "" && slide.Background != "none" {
			background = slide.Background
		}
		red, green, blue := textRGB(background)
		p.SetFillColor(red, green, blue)
		p.Rect(0, 0, w, h, "F")
		for _, part := range []string{slide.MasterPart, slide.LayoutPart} {
			if res, ok := resources[part]; ok {
				if len(res.Unsupported) > 0 {
					return fmt.Errorf("%w: master/layout feature %s", ErrUnsupported, res.Unsupported[0])
				}
				for _, o := range res.Objects {
					if o.Placeholder != "" {
						continue
					}
					if err := paintSlideObject(r, o); err != nil {
						return err
					}
				}
			}
		}
		for _, o := range slide.Objects {
			if err := paintSlideObject(r, o); err != nil {
				return err
			}
		}
	}
	if err := p.Error(); err != nil {
		return err
	}
	var raw bytes.Buffer
	if err := p.Output(&raw); err != nil {
		return err
	}
	if bytes.HasPrefix(raw.Bytes(), []byte("%PDF-1.3")) {
		raw.Bytes()[7] = '4'
	}
	complete, err := r.finishGlyphFonts(raw.Bytes())
	if err != nil {
		return err
	}
	_, err = io.Copy(contextWriter{ctx, out}, bytes.NewReader(complete))
	return err
}
func paintSlideObject(r *pdfRenderer, o SlideObject) error {
	if o.Hidden {
		return nil
	}
	if len(o.Unsupported) > 0 {
		return fmt.Errorf("%w: PDF object %s: %s", ErrUnsupported, o.ID, o.Unsupported[0])
	}
	if o.Width <= 0 || o.Height <= 0 {
		return fmt.Errorf("%w: object %s has no resolved frame", ErrUnsupported, o.ID)
	}
	p := r.pdf
	p.TransformBegin()
	defer p.TransformEnd()
	cx, cy := o.X+o.Width/2, o.Y+o.Height/2
	if o.Rotation != 0 {
		p.TransformRotate(-o.Rotation, cx, cy)
	}
	if o.FlipH {
		p.TransformScaleX(-100, cx, cy)
	}
	if o.FlipV {
		p.TransformScaleY(-100, cx, cy)
	}
	switch o.Kind {
	case "image":
		if (o.Fill != "" && o.Fill != "none") || (o.Stroke != "" && o.Stroke != "none") {
			return fmt.Errorf("%w: image fill/border painting", ErrUnsupported)
		}
		im := *o.Image
		_, kind, err := decodeOutputImage(r.ctx, im.Data, r.validatedImages)
		if err != nil {
			return err
		}
		if kind != "png" && kind != "jpeg" {
			return fmt.Errorf("%w: PDF image media type %s", ErrUnsupported, kind)
		}
		return r.drawImage(&pdfImageLayout{image: &im, kind: kind, width: o.Width, height: o.Height}, o.X, o.Y)
	case "shape":
		switch o.Geometry {
		case "", "rect", "ellipse", "roundRect", "line":
		default:
			return fmt.Errorf("%w: vector geometry %s", ErrUnsupported, o.Geometry)
		}
		style := ""
		if o.Fill != "" && o.Fill != "none" {
			red, green, blue := textRGB(o.Fill)
			p.SetFillColor(red, green, blue)
			style = "F"
		}
		if o.Stroke != "" && o.Stroke != "none" {
			red, green, blue := textRGB(o.Stroke)
			p.SetDrawColor(red, green, blue)
			p.SetLineWidth(math.Max(.1, o.StrokeWidth))
			if style != "" {
				style = "DF"
			} else {
				style = "D"
			}
		}
		if style != "" {
			switch o.Geometry {
			case "", "rect":
				p.Rect(o.X, o.Y, o.Width, o.Height, style)
			case "ellipse":
				p.Ellipse(cx, cy, o.Width/2, o.Height/2, 0, style)
			case "roundRect":
				p.RoundedRect(o.X, o.Y, o.Width, o.Height, math.Min(o.Width, o.Height)/8, "1234", style)
			case "line":
				p.Line(o.X, o.Y, o.X+o.Width, o.Y+o.Height)
			default:
				return fmt.Errorf("%w: vector geometry %s", ErrUnsupported, o.Geometry)
			}
		}
		return paintSlideText(r, o)

	case "group":
		return fmt.Errorf("%w: native group coordinate mappings are preserved but not painted into PDF", ErrUnsupported)
	default:
		return fmt.Errorf("%w: unmodeled native object cannot be painted into PDF", ErrUnsupported)
	}
}

func flowToPresentation(d *Document) (*Presentation, error) {
	if len(d.Pages) > 0 {
		return pdfFlowToPresentation(d)
	}
	w, h, _ := layout(d.Layout)
	p := &Presentation{Version: 1, Width: w, Height: h}
	margin := 36.0
	y := margin
	nextSlide := func() {
		p.Slides = append(p.Slides, PresentationSlide{ID: strconv.Itoa(256 + len(p.Slides))})
		y = margin
	}
	nextSlide()
	add := func(o SlideObject, height float64) error {
		if height > h-2*margin {
			return fmt.Errorf("%w: flow block exceeds slide height; split it before export", ErrUnsupported)
		}
		if y+height > h-margin {
			nextSlide()
		}
		o.ID = strconv.Itoa(len(p.Slides[len(p.Slides)-1].Objects) + 2)
		o.X = margin
		o.Y = y
		o.Width = w - 2*margin
		o.Height = height
		p.Slides[len(p.Slides)-1].Objects = append(p.Slides[len(p.Slides)-1].Objects, o)
		y += height + 6
		return nil
	}
	var walk func([]Block) error
	walk = func(bs []Block) error {
		for _, b := range legacyLists(bs) {
			switch b.Kind {
			case PageBreak:
				nextSlide()
			case AnchorPoint:
				return fmt.Errorf("%w: flow anchors need a slide target mapping", ErrUnsupported)
			case Paragraph, Heading:
				if b.PageBreakBefore && y > margin {
					nextSlide()
				}
				runs := append([]Run(nil), b.Runs...)
				size := 18.0
				if b.Kind == Heading {
					size = 24
				}
				count := 0
				for i := range runs {
					runs[i].Source = nil
					if runs[i].FontSize > 0 {
						runs[i].FontSize = math.Round(runs[i].FontSize*100) / 100
					}
					if runs[i].FontSize > size {
						size = runs[i].FontSize
					}
					if runs[i].Note != nil {
						runs[i].Text = "[" + runs[i].Note.Kind + ":" + runs[i].Note.ID + "]"
						runs[i].Note = nil
					}
					count += utf8.RuneCountInString(runs[i].Text)
				}
				if b.Kind == Heading {
					for i := range runs {
						if runs[i].FontSize == 0 {
							runs[i].FontSize = size
						}
						runs[i].Bold = true
					}
					b.Kind = Paragraph
					b.Level = 0
				}
				b.Runs = runs
				b.Source = nil
				b.PageBreakBefore = false
				lines := math.Max(1, math.Ceil(float64(count)*size*.65/(w-2*margin)))
				if err := add(SlideObject{Kind: "shape", Geometry: "rect", Content: []Block{b}}, lines*size*1.4+size); err != nil {
					return err
				}
			case Picture:
				im := *b.Image
				height := im.Height
				if height == 0 {
					height = 160
				}
				if err := add(SlideObject{Kind: "image", Image: &im}, height); err != nil {
					return err
				}
			case Table:
				for _, row := range b.Rows {
					var blocks []Block
					for i, cell := range row.Cells {
						if cell.ColSpan > 1 || cell.RowSpan > 1 {
							return fmt.Errorf("%w: merged flow tables require an explicit slide layout", ErrUnsupported)
						}
						if i > 0 {
							blocks = append(blocks, Block{Kind: Paragraph, Runs: []Run{{Text: " | "}}})
						}
						blocks = append(blocks, cell.Blocks...)
					}
					if err := walk(blocks); err != nil {
						return err
					}
				}
			case ListBlock:
				if b.Numbering != nil {
					return fmt.Errorf("%w: automatic numbering needs explicit literal markers for slides", ErrUnsupported)
				}
				start := 1
				if b.Start != nil {
					start = *b.Start
				}
				for i, item := range b.Items {
					if item.Marker != nil {
						if err := walk(item.Marker.Blocks); err != nil {
							return err
						}
					} else {
						label := "• "
						if b.List == "number" {
							label = fmt.Sprintf("%d. ", start+i)
						}
						if err := walk([]Block{{Kind: Paragraph, Runs: []Run{{Text: label}}}}); err != nil {
							return err
						}
					}
					if err := walk(item.Blocks); err != nil {
						return err
					}
				}
			case SectionBlock:
				if len(p.Slides[len(p.Slides)-1].Objects) > 0 {
					nextSlide()
				}
				if b.Section.Layout != nil {
					return fmt.Errorf("%w: section-specific page geometry requires an explicit slide layout", ErrUnsupported)
				}
				if err := walk(b.Section.Blocks); err != nil {
					return err
				}
			default:
				return fmt.Errorf("%w: flow block %s", ErrUnsupported, b.Kind)
			}
		}
		return nil
	}
	if len(d.Headers) > 0 || len(d.Footers) > 0 {
		return nil, fmt.Errorf("%w: running stories need an explicit slide master", ErrUnsupported)
	}
	if err := walk(d.Blocks); err != nil {
		return nil, err
	}
	for _, note := range d.Notes {
		p.Slides[0].Notes = append(p.Slides[0].Notes, note.Blocks...)
	}
	return p, p.validate()
}
func pdfFlowToPresentation(d *Document) (*Presentation, error) {
	p := &Presentation{Version: 1, Width: d.Pages[0].Width, Height: d.Pages[0].Height}
	for i, page := range d.Pages {
		p.Slides = append(p.Slides, PresentationSlide{ID: strconv.Itoa(256 + i), Width: page.Width, Height: page.Height})
	}
	add := func(page int, o SlideObject) error {
		if page < 1 || page > len(p.Slides) {
			return fmt.Errorf("%w: PDF content has no physical slide owner", ErrUnsupported)
		}
		slide := &p.Slides[page-1]
		o.ID = strconv.Itoa(len(slide.Objects) + 2)
		slide.Objects = append(slide.Objects, o)
		return nil
	}
	var walk func([]Block) error
	walk = func(bs []Block) error {
		for _, b := range bs {
			switch b.Kind {
			case Paragraph, Heading:
				for _, run := range b.Runs {
					if run.Text == "" {
						continue
					}
					source := run.Source
					if source == nil {
						source = b.Source
					}
					if source == nil || source.Quad == nil {
						return fmt.Errorf("%w: PDF run lacks a positioned source frame", ErrUnsupported)
					}
					x, y, w, h := presentationQuadBox(*source.Quad)
					run.Source = nil
					if run.FontSize == 0 {
						run.FontSize = source.FontSize
					}
					if run.FontSize == 0 {
						run.FontSize = 12
					}
					run.FontSize = math.Round(run.FontSize*100) / 100
					if w <= 0 {
						w = run.FontSize
					}
					height := math.Max(h, run.FontSize*1.4)
					o := SlideObject{Kind: "shape", Geometry: "rect", X: x, Y: y, Width: w + .5, Height: height, Content: []Block{{Kind: Paragraph, Runs: []Run{run}}}}
					if err := add(source.Page, o); err != nil {
						return err
					}
				}
			case Picture:
				if b.Source == nil || b.Source.Quad == nil {
					return fmt.Errorf("%w: PDF image lacks position", ErrUnsupported)
				}
				x, y, w, h := presentationQuadBox(*b.Source.Quad)
				if err := add(b.Source.Page, SlideObject{Kind: "image", X: x, Y: y, Width: w, Height: h, Image: b.Image}); err != nil {
					return err
				}
			case Table:
				for _, row := range b.Rows {
					for _, cell := range row.Cells {
						if err := walk(cell.Blocks); err != nil {
							return err
						}
					}
				}
			case ListBlock:
				for _, item := range b.Items {
					if item.Marker != nil {
						if err := walk(item.Marker.Blocks); err != nil {
							return err
						}
					}
					if err := walk(item.Blocks); err != nil {
						return err
					}
				}
			case PageBreak, AnchorPoint:
			case SectionBlock:
				if err := walk(b.Section.Blocks); err != nil {
					return err
				}
			default:
				return fmt.Errorf("%w: PDF recovered block %s", ErrUnsupported, b.Kind)
			}
		}
		return nil
	}
	if err := walk(d.Headers); err != nil {
		return nil, err
	}
	if err := walk(d.Blocks); err != nil {
		return nil, err
	}
	if err := walk(d.Footers); err != nil {
		return nil, err
	}
	for _, n := range d.Notes {
		if err := walk(n.Blocks); err != nil {
			return nil, err
		}
	}
	return p, p.validate()
}

func paintSlideText(r *pdfRenderer, o SlideObject) error {
	hasText := false
	for _, block := range o.Content {
		for _, run := range block.Runs {
			if run.Text != "" {
				hasText = true
			}
		}
	}
	if !hasText {
		return nil
	}
	inset := PageMargins{}
	if o.Insets != nil {
		inset = *o.Insets
	}
	width, height := o.Width-inset.Left-inset.Right, o.Height-inset.Top-inset.Bottom
	if width <= 0 || height <= 0 {
		return fmt.Errorf("%w: slide text insets consume its frame", ErrUnsupported)
	}
	type placed struct {
		line                pdfLine
		size, height, width float64
		align               string
	}
	var lines []placed
	total := 0.0
	for i, block := range o.Content {
		if block.Kind != Paragraph && block.Kind != Heading {
			return fmt.Errorf("%w: slide text block %s", ErrUnsupported, block.Kind)
		}
		size := 18.0
		if block.Kind == Heading {
			size = 24
		}
		for _, run := range block.Runs {
			if run.Note != nil || run.Anchor != "" || run.Destination != "" {
				return fmt.Errorf("%w: slide inline note/internal target", ErrUnsupported)
			}
		}
		wrapWidth := width
		if o.NoWrap {
			wrapWidth = 14400
		}
		shaped, err := r.linesDirection(block.Runs, wrapWidth, size, block.Direction)
		if err != nil {
			return err
		}
		align := ""
		if len(o.ParagraphAlign) > 0 {
			align = o.ParagraphAlign[i]
		}
		for _, line := range shaped {
			h := pdfLineHeight(line, size, 1.15)
			advance := 0.0
			for _, span := range line {
				advance += span.width
			}
			if advance > width+.1 {
				return fmt.Errorf("%w: slide text exceeds horizontal frame", ErrUnsupported)
			}
			lines = append(lines, placed{line, size, h, advance, align})
			total += h
		}
	}
	if total > height+.1 {
		return fmt.Errorf("%w: text overflows slide object %s; enlarge its frame", ErrUnsupported, o.ID)
	}
	y := o.Y + inset.Top
	switch o.VerticalAlign {
	case "ctr":
		y += (height - total) / 2
	case "b":
		y += height - total
	}
	for _, line := range lines {
		x := o.X + inset.Left
		switch line.align {
		case "ctr":
			x += (width - line.width) / 2
		case "r":
			x += width - line.width
		}
		r.drawLine(line.line, x, y, line.size)
		if err := r.pdf.Error(); err != nil {
			return err
		}
		y += line.height
	}
	return nil
}
