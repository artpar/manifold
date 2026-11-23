package manydoc

import (
	"errors"
	"fmt"
	"math"
)

type pdfBodyBounds struct{ top, bottom float64 }
type pdfRunningStory struct {
	flow              []pdfFragment
	y, height, budget float64
	imageBudget       float64
	shrink            bool
}

func (r *pdfRenderer) bodyTop() float64 {
	if r.content != nil {
		return r.content.top
	}
	return r.baseTop()
}
func (r *pdfRenderer) bodyBottom() float64 {
	if r.content != nil {
		return r.content.bottom
	}
	return r.baseBottom()
}
func (r *pdfRenderer) bodyHeight() float64 { return r.bodyBottom() - r.bodyTop() }

func validatePDFRunning(bs []Block) error {
	for _, b := range bs {
		if b.Kind == PageBreak || b.PageBreakBefore {
			return fmt.Errorf("%w: PDF running content cannot contain page controls", ErrUnsupported)
		}
		for _, item := range b.Items {
			if err := validatePDFRunning(item.Blocks); err != nil {
				return err
			}
		}
		for _, row := range b.Rows {
			for _, cell := range row.Cells {
				if err := validatePDFRunning(cell.Blocks); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// A measurement budget is passed through each table (8pt trailing gap and 12pt
// cell padding) to an image (8pt trailing gap). Tightening to this measured bound
// makes progress even when preferred widths, not the initial page budget, limit
// the image. No text/font size is reduced.
func runningImageBudget(flow []pdfFragment) float64 {
	limit := 0.0
	for _, f := range flow {
		if f.Image != nil {
			limit = math.Max(limit, f.Image.height+8)
		}
		if f.Table != nil {
			for _, cell := range f.Table.table.cells {
				if budget := runningImageBudget(cell.flow); budget > 0 {
					limit = math.Max(limit, budget+20)
				}
			}
		}
	}
	return limit
}
func (r *pdfRenderer) measureRunning(bs []Block, budget float64) (*pdfRunningStory, error) {
	if len(bs) == 0 {
		return nil, nil
	}
	if err := validatePDFRunning(bs); err != nil {
		return nil, err
	}
	flow, err := r.cellFlowStyle(bs, r.bodyWidth(), 0, budget, pdfFlowStyle{size: 8, leading: 1.25, running: true})
	if err != nil {
		return nil, err
	}
	height := flowHeight(flow)
	if !finite(height) {
		return nil, fmt.Errorf("%w: non-finite PDF running content height", ErrUnsupported)
	}
	imageBudget := runningImageBudget(flow)
	return &pdfRunningStory{flow: flow, height: height, budget: budget, imageBudget: imageBudget, shrink: imageBudget > 0}, nil
}
func (r *pdfRenderer) prepareRunning(headers, footers []Block, body bool) error {
	budget := r.baseBottom() - r.baseTop()
	if r.page != nil && (r.page.explicitMargins || r.page.header != nil || r.page.footer != nil) {
		budget = r.height
		if len(headers) > 0 {
			budget -= r.headerStart()
		}
		if end, explicit := r.footerEnd(); explicit && len(footers) > 0 {
			budget -= r.height - end
		}
	}
	header, err := r.measureRunning(headers, budget)
	if err != nil {
		return err
	}
	footer, err := r.measureRunning(footers, budget)
	if err != nil {
		return err
	}
	minimum := 1.0
	if body {
		// Keep a sub-millipoint guard above the measured floor. Final PDF
		// coordinates round to five decimals; cancellation at two fitted story
		// edges must not turn a legal body line into an over-height block.
		minimum = math.Max(11*1.4, r.minimumBody) + .001
	}
	for attempt := 0; attempt < 128; attempt++ {
		top, bottom := r.baseTop(), r.baseBottom()
		if header != nil {
			header.y = r.headerStart()
			top = math.Max(top, header.y+header.height)
		}
		if footer != nil {
			if end, explicit := r.footerEnd(); explicit {
				footer.y = end - footer.height
			} else {
				footer.y = math.Min(r.height-(r.height-r.baseBottom())/2, r.height-footer.height)
			}
			bottom = math.Min(bottom, footer.y)
		}
		// Signed OOXML offsets keep the body fixed. Non-overlapping stories use
		// the ordinary geometry; actual overlap needs separate paint-order/layout
		// qualification and must not be silently replaced by enlarged margins.
		edges := r.pageEdges()
		if header != nil && edges.Top < 0 {
			if header.y+header.height > r.baseTop()+1e-7 {
				return fmt.Errorf("%w: PDF negative top-margin running overlap requires further layout qualification", ErrUnsupported)
			}
			header.shrink = false
		}
		if footer != nil && edges.Bottom < 0 {
			if footer.y < r.baseBottom()-1e-7 {
				return fmt.Errorf("%w: PDF negative bottom-margin running overlap requires further layout qualification", ErrUnsupported)
			}
			footer.shrink = false
		}
		active := 0
		for _, story := range []*pdfRunningStory{header, footer} {
			if story != nil && story.shrink {
				active++
			}
		}
		if bottom-top >= minimum-1e-7 || active == 0 && bottom > top {
			r.header, r.footer = header, footer
			r.content = &pdfBodyBounds{top: top, bottom: bottom}
			return nil
		}
		if active == 0 {
			return fmt.Errorf("%w: running content leaves no space for the PDF body", ErrUnsupported)
		}
		excess := math.Max(.01, minimum-(bottom-top)) / float64(active)
		for i, story := range []*pdfRunningStory{header, footer} {
			if story == nil || !story.shrink {
				continue
			}
			blocks := headers
			if i == 1 {
				blocks = footers
			}
			next, err := r.shrinkRunning(blocks, story, story.height-excess)
			if err != nil {
				return err
			}
			if i == 0 {
				header = next
			} else {
				footer = next
			}
		}
	}
	return fmt.Errorf("%w: PDF running content fitting did not converge", ErrUnsupported)
}

// Search a monotone image budget, rather than subtracting story-height excess
// directly: several serial images can amplify a one-point budget reduction.
// Invalid low image budgets form a lower bound; they never discard content.
func (r *pdfRenderer) shrinkRunning(bs []Block, story *pdfRunningStory, target float64) (*pdfRunningStory, error) {
	low, high := 0.0, math.Min(story.budget, story.imageBudget)
	closest := story
	var fitting *pdfRunningStory
	for attempt := 0; attempt < 40 && high-low > 1e-7; attempt++ {
		budget := (low + high) / 2
		next, err := r.measureRunning(bs, budget)
		if err != nil {
			if r.ctx.Err() != nil {
				return nil, r.ctx.Err()
			}
			if !errors.Is(err, errPDFImageSpace) {
				return nil, err
			}
			low = budget
			continue
		}
		if next.height > story.height+1e-7 {
			return nil, fmt.Errorf("%w: nonmonotone running content measurement", ErrUnsupported)
		}
		if next.height < closest.height-1e-7 {
			closest = next
		}
		if next.height <= target+1e-7 {
			fitting = next
			low = budget // preserve the largest feasible image budget
		} else {
			high = budget
		}
	}
	if fitting != nil {
		return fitting, nil
	}
	// The target can lie below fixed text/padding or image coordinate precision.
	// Keep the best valid plan, then let the other story supply any remainder.
	copy := *closest
	copy.shrink = false
	return &copy, nil
}

func (r *pdfRenderer) paintRunning(story *pdfRunningStory) {
	if story == nil {
		return
	}
	before := r.paintingRunning
	r.paintingRunning = true
	defer func() { r.paintingRunning = before }()
	y := story.y
	for _, fragment := range story.flow {
		if err := r.drawFragment(fragment, r.bodyLeft(), y); err != nil {
			r.pdf.SetError(err)
			return
		}
		y += fragment.Height
	}
}

// Materialize body/cell defaults for a measurement-only flow, and minimize image
// preferences. This finds indivisible text/header/note needs without reserving a
// whole preferred-size body figure ahead of running-content fitting.
func (r *pdfRenderer) minimumRunningBlocks(bs []Block, cell bool) ([]Block, error) {
	out := append([]Block(nil), bs...)
	for i := range out {
		b := &out[i]
		size := 11.0
		if cell {
			size = 10
		} else if b.Kind == Heading {
			size = 26 - float64(b.Level)*2
		}
		b.Runs = append([]Run(nil), b.Runs...)
		for j := range b.Runs {
			if b.Runs[j].FontSize == 0 {
				b.Runs[j].FontSize = size
			}
		}
		if b.Image != nil {
			im := *b.Image
			cfg, _, err := decodeOutputImage(r.ctx, im.Data, r.validatedImages)
			if err != nil {
				return nil, err
			}
			im.Width, im.Height = 0, math.Max(.001, float64(cfg.Height)/float64(cfg.Width)*.000011)
			b.Image = &im
		}
		b.Items = append([]ListItem(nil), b.Items...)
		for j := range b.Items {
			children, err := r.minimumRunningBlocks(b.Items[j].Blocks, cell)
			if err != nil {
				return nil, err
			}
			b.Items[j].Blocks = children
		}
		b.Rows = append([]Row(nil), b.Rows...)
		for j := range b.Rows {
			b.Rows[j].Cells = append([]Cell(nil), b.Rows[j].Cells...)
			for k := range b.Rows[j].Cells {
				children, err := r.minimumRunningBlocks(b.Rows[j].Cells[k].Blocks, true)
				if err != nil {
					return nil, err
				}
				b.Rows[j].Cells[k].Blocks = children
			}
		}
	}
	return out, nil
}
func (r *pdfRenderer) minimumRunningBody(blocks []Block) (float64, error) {
	compact, err := r.minimumRunningBlocks(blocks, false)
	if err != nil {
		return 0, err
	}
	flow, err := r.cellFlow(compact, r.bodyWidth(), 0, r.bodyHeight())
	if err != nil {
		return 0, err
	}
	minimum := 0.0
	// The body painter reserves its paragraph/heading fallback size even for
	// empty or explicitly smaller runs. Cell/note style has its own 10pt floor.
	var bodyDefaults func([]Block)
	bodyDefaults = func(bs []Block) {
		for _, b := range bs {
			if b.Kind == Paragraph {
				minimum = math.Max(minimum, 11*1.4)
			}
			if b.Kind == Heading {
				minimum = math.Max(minimum, (26-float64(b.Level)*2)*1.4)
			}
			for _, item := range b.Items {
				bodyDefaults(item.Blocks)
			}
		}
	}
	bodyDefaults(blocks)
	var walk func([]pdfFragment, float64) error
	walk = func(flow []pdfFragment, padding float64) error {
		for _, f := range flow {
			minimum = math.Max(minimum, minimumPDFFragmentHeight(f)+padding)
			if len(f.Line) > 0 {
				h := f.Height + padding
				note, err := r.minimumNewFootnotes(lineNoteKeys(f.Line), true, h)
				if err != nil {
					return err
				}
				minimum = math.Max(minimum, h+note)
			}
			if f.Table != nil {
				extra := 12.0
				if f.Table.table.header != nil {
					extra += f.Table.table.header.height
				}
				for _, c := range f.Table.table.cells {
					if err := walk(c.flow, padding+extra); err != nil {
						return err
					}
				}
			}
		}
		return nil
	}
	if err := walk(flow, 0); err != nil {
		return 0, err
	}
	for _, key := range r.notes.order {
		note := r.notes.definitions[key]
		bs, err := r.minimumRunningBlocks(note.Blocks, key.kind == Footnote)
		if err != nil {
			return 0, err
		}
		noteFlow, err := r.cellFlow(bs, r.bodyWidth()-18, 0, r.bodyHeight())
		if err != nil {
			return 0, err
		}
		for _, f := range noteFlow {
			extra := 0.0
			if key.kind == Footnote {
				extra = footnoteSeparatorHeight + 10
			}
			minimum = math.Max(minimum, minimumPDFFragmentHeight(f)+extra)
		}
		if key.kind == Endnote {
			minimum = math.Max(minimum, 24*1.4)
		}
	}
	return minimum, nil
}
