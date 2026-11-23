package manydoc

import (
	"bytes"
	"fmt"
	"image"
	"math"

	"github.com/phpdave11/gofpdf"
)

// Text/image fragments are indivisible. A table fragment can supply a smaller
// measured page through recursive continuation. Text lines remain 14pt in the
// compact cell/note style. Measurement has no drawing or page-admission effects.
type pdfFragment struct {
	Line        pdfLine
	Indent      float64
	Prefix      pdfLine
	PrefixX     float64
	Height      float64
	Image       *pdfImageLayout
	Table       *pdfTableSlice
	Break       bool
	Conditional bool
}
type pdfImageLayout struct {
	image         *Image
	kind          string
	width, height float64
}
type pdfTableLayout struct {
	width, height, unit float64
	offsets             []float64
	cells               []pdfMeasuredCell
	groups              []pdfTableGroup
}
type pdfMeasuredCell struct {
	position *tableCellPosition
	flow     []pdfFragment
	height   float64
}
type pdfTableGroup struct {
	start, end, first, last int
	height                  float64
	keys                    []noteKey
	breaks                  bool
}
type pdfTableSlice struct {
	table  *pdfTableLayout
	group  pdfTableGroup
	cursor *pdfTableCursor // nil for an original, unsplit group
	page   *pdfTablePage   // an immutable, measured placement selected for drawing
	gap    float64         // original trailing spacing, retained by the last part
}

func flowHeight(flow []pdfFragment) float64 {
	h := 0.0
	for _, f := range flow {
		h += f.Height
	}
	return h
}

func flowHasBreak(flow []pdfFragment) bool {
	for _, f := range flow {
		if f.Break || f.Table != nil && f.Table.group.breaks {
			return true
		}
	}
	return false
}
func flowNoteKeys(flow []pdfFragment) []noteKey {
	var keys []noteKey
	for _, f := range flow {
		keys = append(keys, lineNoteKeys(f.Line)...)
		if f.Table != nil {
			if f.Table.page != nil {
				keys = append(keys, f.Table.page.keys...)
			} else {
				keys = append(keys, f.Table.group.keys...)
			}
		}
	}
	return keys
}
func (r *pdfRenderer) measureImage(im *Image, width float64) (*pdfImageLayout, error) {
	if err := r.ctx.Err(); err != nil {
		return nil, err
	}
	if im == nil || !finite(width) || width <= 0 {
		return nil, fmt.Errorf("%w: no width for PDF image", ErrUnsupported)
	}
	config, kind, err := image.DecodeConfig(bytes.NewReader(im.Data))
	if err != nil {
		return nil, err
	}
	if kind != "png" && kind != "jpeg" {
		return nil, fmt.Errorf("%w: PDF image %s", ErrUnsupported, kind)
	}
	iw, ih, err := fitImageSize(im, config.Width, config.Height, width)
	if err != nil {
		return nil, err
	}
	return &pdfImageLayout{im, kind, iw, ih}, nil
}
func (r *pdfRenderer) drawImage(im *pdfImageLayout, x, y float64) error {
	if err := r.ctx.Err(); err != nil {
		return err
	}
	r.images++
	name := fmt.Sprintf("image%d", r.images)
	r.pdf.RegisterImageOptionsReader(name, gofpdf.ImageOptions{ImageType: im.kind}, bytes.NewReader(im.image.Data))
	r.pdf.ImageOptions(name, x, y, im.width, im.height, false, gofpdf.ImageOptions{ImageType: im.kind}, 0, "")
	return r.pdf.Error()
}

func (r *pdfRenderer) cellFlow(bs []Block, width, indent float64) ([]pdfFragment, error) {
	if !finite(width) || width <= indent {
		return nil, fmt.Errorf("%w: PDF cell indentation/padding leaves no content width", ErrUnsupported)
	}
	var flow []pdfFragment
	for _, b := range legacyLists(bs) {
		if err := r.ctx.Err(); err != nil {
			return nil, err
		}
		switch b.Kind {
		case PageBreak:
			flow = append(flow, pdfFragment{Break: true})
		case Paragraph, Heading:
			if b.PageBreakBefore {
				flow = append(flow, pdfFragment{Break: true, Conditional: true})
			}
			runs := append([]Run(nil), b.Runs...)
			if b.Kind == Heading {
				for i := range runs {
					runs[i].Bold = true
				}
			}
			lines, err := r.lines(runs, width-indent, 10)
			if err != nil {
				return nil, err
			}
			for _, line := range lines {
				flow = append(flow, pdfFragment{Line: line, Indent: indent, Height: 14})
			}
		case ListBlock:
			for index, item := range b.Items {
				marker, step, err := r.listMarker(b, index, width-indent, 10)
				if err != nil {
					return nil, err
				}
				children, err := r.cellFlow(item.Blocks, width, indent+step)
				if err != nil {
					return nil, err
				}
				children[0].Prefix, children[0].PrefixX = marker, indent
				flow = append(flow, children...)
			}
		case Picture:
			im, err := r.measureImage(b.Image, width-indent)
			if err != nil {
				return nil, err
			}
			flow = append(flow, pdfFragment{Image: im, Indent: indent, Height: im.height + 8})
		case Table:
			plan, err := r.measureTable(b, width-indent)
			if err != nil {
				return nil, err
			}
			for i, g := range plan.groups {
				height := g.height
				if i == len(plan.groups)-1 {
					height += 8
				}
				flow = append(flow, pdfFragment{Table: &pdfTableSlice{table: plan, group: g, gap: height - g.height}, Indent: indent, Height: height})
			}
		default:
			return nil, fmt.Errorf("%w: unknown PDF cell/footnote block %s", ErrUnsupported, b.Kind)
		}
	}
	if len(flow) == 0 {
		flow = append(flow, pdfFragment{Height: 14})
	}
	return flow, nil
}

func (r *pdfRenderer) drawFragment(f pdfFragment, x, y float64) error {
	if err := r.ctx.Err(); err != nil {
		return err
	}
	if f.Image != nil {
		if err := r.drawImage(f.Image, x+f.Indent, y); err != nil {
			return err
		}
	}
	if f.Table != nil {
		var err error
		if f.Table.page != nil {
			err = r.paintTablePage(f.Table.table, *f.Table.page, x+f.Indent, y)
		} else {
			err = r.paintTableGroup(f.Table.table, f.Table.group, x+f.Indent, y)
		}
		if err != nil {
			return err
		}
	}
	r.drawLine(f.Line, x+f.Indent, y, 10)
	if len(f.Prefix) > 0 {
		r.drawLine(f.Prefix, x+f.PrefixX, y, 10)
	}
	return r.pdf.Error()
}

func (r *pdfRenderer) measureTable(b Block, width float64) (*pdfTableLayout, error) {
	g, err := tableGrid(b.Rows)
	if err != nil {
		return nil, err
	}
	plan := &pdfTableLayout{width: width}
	if g.Columns == 0 {
		return plan, nil
	}
	if !finite(width) || width <= 0 {
		return nil, fmt.Errorf("%w: no width for PDF table", ErrUnsupported)
	}
	plan.unit = width / float64(g.Columns)
	heights := make([]float64, len(b.Rows))
	for i := range heights {
		heights[i] = 22
	}
	for _, p := range g.Origins {
		if err := r.ctx.Err(); err != nil {
			return nil, err
		}
		flow, err := r.cellFlow(b.Rows[p.Row].Cells[p.Cell].Blocks, plan.unit*float64(p.ColSpan)-12, 0)
		if err != nil {
			return nil, err
		}
		h := math.Max(22, flowHeight(flow)+12)
		if !finite(h) {
			return nil, fmt.Errorf("invalid measured PDF cell height")
		}
		plan.cells = append(plan.cells, pdfMeasuredCell{p, flow, h})
		if p.RowSpan == 1 {
			heights[p.Row] = math.Max(heights[p.Row], h)
		}
	}
	for _, c := range plan.cells {
		p := c.position
		if p.RowSpan == 1 {
			continue
		}
		total := 0.0
		for _, h := range heights[p.Row : p.Row+p.RowSpan] {
			total += h
		}
		if total < c.height {
			heights[p.Row+p.RowSpan-1] += c.height - total
		}
	}
	plan.offsets = make([]float64, len(heights)+1)
	for i, h := range heights {
		plan.offsets[i+1] = plan.offsets[i] + h
		if !finite(plan.offsets[i+1]) {
			return nil, fmt.Errorf("%w: PDF table height exceeds the coordinate range", ErrUnsupported)
		}
	}
	plan.height = plan.offsets[len(heights)]
	cursor := 0
	for start := 0; start < len(heights); {
		end, last := start+1, cursor
		for last < len(plan.cells) && plan.cells[last].position.Row < end {
			p := plan.cells[last].position
			end = max(end, p.Row+p.RowSpan)
			last++
		}
		g := pdfTableGroup{start: start, end: end, first: cursor, last: last, height: plan.offsets[end] - plan.offsets[start]}
		for _, cell := range plan.cells[cursor:last] {
			g.keys = append(g.keys, flowNoteKeys(cell.flow)...)
			g.breaks = g.breaks || flowHasBreak(cell.flow)
		}
		plan.groups = append(plan.groups, g)
		cursor, start = last, end
	}
	return plan, nil
}
func (r *pdfRenderer) paintTableGroup(plan *pdfTableLayout, g pdfTableGroup, x, top float64) error {
	for _, c := range plan.cells[g.first:g.last] {
		if err := r.ctx.Err(); err != nil {
			return err
		}
		p := c.position
		cx := x + float64(p.Column)*plan.unit
		cy := top + plan.offsets[p.Row] - plan.offsets[g.start]
		ch := plan.offsets[p.Row+p.RowSpan] - plan.offsets[p.Row]
		r.pdf.SetDrawColor(120, 120, 120)
		r.pdf.Rect(cx, cy, float64(p.ColSpan)*plan.unit, ch, "D")
		y := cy + 6
		for _, f := range c.flow {
			if err := r.drawFragment(f, cx+6, y); err != nil {
				return err
			}
			y += f.Height
		}
	}
	return r.pdf.Error()
}
func (r *pdfRenderer) table(b Block, width, x float64) error {
	plan, err := r.measureTable(b, width)
	if err != nil {
		return err
	}
	for _, g := range plan.groups {
		minimum, err := r.minimumNewFootnotes(g.keys, true)
		if err != nil {
			return err
		}
		if g.breaks || g.height+minimum > r.height-2*r.margin {
			if err := r.continueTableGroup(plan, g, x); err != nil {
				return fmt.Errorf("PDF table row group %d-%d: %w", g.start+1, g.end, err)
			}
			continue
		}
		if err := r.ensureWithNotes(g.height, g.keys); err != nil {
			return fmt.Errorf("PDF table row group %d-%d: %w", g.start+1, g.end, err)
		}
		if err := r.paintTableGroup(plan, g, x, r.y); err != nil {
			return err
		}
		r.y += g.height
	}
	return r.pdf.Error()
}
