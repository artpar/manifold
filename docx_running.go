package manifold

import (
	"fmt"
	"math"
	"strings"

	"github.com/rivo/uniseg"
)

// DOCX delegates font selection and pagination to its consumer. These envelopes
// budget inline rasters, not glyph-exact Word layout. Text uses a conservative em
// column estimate; tables retain row-span constraints rather than summing columns.
// Reserve picture descent space and a producer body floor so independently
// fitted header and footer pictures cannot each consume the complete body height.
const docxRunningDistance = 18.0 // matches the existing 360-twip pgMar values
const docxRunningImageGap = 8.0

type docxRunningFlow []docxRunningFragment
type docxRunningFragment struct {
	fixed, image float64
	table        *docxRunningTable
}
type docxRunningTable struct {
	rows int
	ends [][]docxRunningCell
}
type docxRunningCell struct {
	start, end int
	flow       docxRunningFlow
}

func (f docxRunningFlow) height(limit float64) float64 {
	h := 0.0
	for _, fragment := range f {
		h += fragment.fixed + math.Min(fragment.image, limit)
		if table := fragment.table; table != nil {
			offsets := make([]float64, table.rows+1)
			for end := 1; end <= table.rows; end++ {
				offsets[end] = offsets[end-1] + 11*1.4
				for _, cell := range table.ends[end] {
					offsets[end] = math.Max(offsets[end], offsets[cell.start]+cell.flow.height(limit)+docxImageCellHeightAllowance)
				}
			}
			h += offsets[table.rows]
		}
	}
	return h
}
func (f docxRunningFlow) largestImage() float64 {
	largest := 0.0
	for _, fragment := range f {
		largest = math.Max(largest, fragment.image)
		if fragment.table != nil {
			for _, cells := range fragment.table.ends {
				for _, cell := range cells {
					largest = math.Max(largest, cell.flow.largestImage())
				}
			}
		}
	}
	return largest
}
func docxParagraphSize(b Block) float64 {
	fallback := 11.0
	if b.Kind == Heading {
		fallback = float64(44-b.Level*3) / 2 // headingStyles, not PDF heading sizes
	}
	size := 0.0
	for _, run := range b.Runs {
		if run.Text == "" && run.Note == nil {
			continue
		}
		effective := run.FontSize
		if effective == 0 {
			effective = fallback
		}
		size = math.Max(size, effective)
	}
	if size == 0 {
		size = fallback
	}
	return size
}
func (w *docxWriter) runningParagraph(b Block, width float64) (float64, error) {
	size := docxParagraphSize(b)
	var text strings.Builder
	for _, run := range b.Runs {
		text.WriteString(run.Text)
	}
	lines, advance := 1, 0.0
	g := uniseg.NewGraphemes(text.String())
	for g.Next() {
		if err := w.ctx.Err(); err != nil {
			return 0, err
		}
		s := g.Str()
		switch s {
		case "\r\n", "\r", "\n", "\u0085", "\u2028", "\u2029":
			lines++
			advance = 0
			continue
		}
		em := float64(uniseg.StringWidth(s))
		if s == "\t" {
			em = 4
		}
		w := em * size
		if advance > 0 && advance+w > width {
			lines++
			advance = 0
		}
		advance += w
	}
	return float64(lines)*size*1.4 + size*.4, nil
}
func (w *docxWriter) runningFlow(bs []Block, width, height float64, depth int, indent float64) (docxRunningFlow, int, error) {
	var flow docxRunningFlow
	images := 0
	for _, b := range legacyLists(bs) {
		if err := w.ctx.Err(); err != nil {
			return nil, 0, err
		}
		available := width - indent
		if available <= 0 {
			return nil, 0, fmt.Errorf("%w: no width for DOCX running content", ErrUnsupported)
		}
		switch b.Kind {
		case Paragraph, Heading:
			fixed, err := w.runningParagraph(b, available)
			if err != nil {
				return nil, 0, err
			}
			flow = append(flow, docxRunningFragment{fixed: fixed})
		case ListBlock:
			if depth > 8 {
				return nil, 0, fmt.Errorf("%w: DOCX numbering supports nine levels", ErrUnsupported)
			}
			for _, item := range b.Items {
				if item.Marker != nil {
					table, _ := docxLiteralTable(item, available)
					child, n, err := w.runningFlow([]Block{table}, available, height, depth, 0)
					if err != nil {
						return nil, 0, err
					}
					flow = append(flow, child...)
					images += n
					continue
				}
				child, n, err := w.runningFlow(item.Blocks, width, height, depth+1, float64(depth+1)*18)
				if err != nil {
					return nil, 0, err
				}
				flow = append(flow, child...)
				images += n
			}
		case Picture:
			cfg, _, err := decodeOutputImage(w.ctx, b.Image.Data, w.validatedImages)
			if err != nil {
				return nil, 0, err
			}
			_, h, err := fitImageBox(b.Image, cfg.Width, cfg.Height, available, height-docxRunningImageGap)
			if err != nil {
				return nil, 0, err
			}
			flow = append(flow, docxRunningFragment{fixed: docxRunningImageGap, image: h})
			images++
		case Table:
			grid, widths, err := docxTableColumns(b, available)
			if err != nil {
				return nil, 0, err
			}
			table := &docxRunningTable{rows: len(b.Rows), ends: make([][]docxRunningCell, len(b.Rows)+1)}
			for _, cell := range grid.Origins {
				cellWidth := 0
				for _, width := range widths[cell.Column : cell.Column+cell.ColSpan] {
					cellWidth += width
				}
				blocks := b.Rows[cell.Row].Cells[cell.Cell].Blocks
				child, n, err := w.runningFlow(blocks, float64(cellWidth)/20-8, height-docxImageCellHeightAllowance, 0, 0)
				if err != nil {
					return nil, 0, err
				}
				if len(blocks) == 0 || blocks[len(blocks)-1].Kind == Table {
					child = append(child, docxRunningFragment{fixed: 11 * 1.4})
				}
				end := cell.Row + cell.RowSpan
				table.ends[end] = append(table.ends[end], docxRunningCell{cell.Row, end, child})
				images += n
			}
			flow = append(flow, docxRunningFragment{table: table})
		case PageBreak:
			// Page controls remain in emitted XML. This is a picture-size
			// budget, not a replacement for consumer pagination.
		}
	}
	return flow, images, nil
}

func docxBodyFloor(bs []Block, padding float64) float64 {
	floor := 11*1.4 + padding
	for _, b := range bs {
		switch b.Kind {
		case Paragraph, Heading:
			size := docxParagraphSize(b)
			floor = math.Max(floor, size*1.4+padding)
		case ListBlock:
			for _, item := range b.Items {
				itemPadding := padding
				if item.Marker != nil {
					itemPadding += docxImageCellHeightAllowance
					floor = math.Max(floor, docxBodyFloor(item.Marker.Blocks, itemPadding))
				}
				floor = math.Max(floor, docxBodyFloor(item.Blocks, itemPadding))
			}
		case Table:
			for _, row := range b.Rows {
				for _, cell := range row.Cells {
					floor = math.Max(floor, docxBodyFloor(cell.Blocks, padding+docxImageCellHeightAllowance))
				}
			}
		}
	}
	return floor
}

// Return a common picture-height cap for growing running stories and the body
// container. Signed fixed stories use their independent page-anchor cap instead.
// Zero means no cap. Smaller preferred pictures remain unchanged.
// The producer still controls actual line/page placement.
func (w *docxWriter) fitRunning(d *Document, g pageGeometry) (float64, float64, error) {
	headerHeight := g.runningStoryHeight(true, len(d.Headers) > 0, len(d.Footers) > 0)
	footerHeight := g.runningStoryHeight(false, len(d.Headers) > 0, len(d.Footers) > 0)
	header, hn, err := w.runningFlow(d.Headers, w.contentWidth, headerHeight, 0, 0)
	if err != nil {
		return 0, 0, err
	}
	footer, fn, err := w.runningFlow(d.Footers, w.contentWidth, footerHeight, 0, 0)
	if err != nil {
		return 0, 0, err
	}
	if hn+fn == 0 {
		return 0, w.contentHeight, nil
	}
	available := func(limit float64) float64 {
		top, bottom := math.Abs(g.margins.Top), math.Abs(g.margins.Bottom)
		if len(d.Headers) > 0 && g.margins.Top >= 0 {
			top = math.Max(top, g.headerDistance()+header.height(limit))
		}
		if len(d.Footers) > 0 && g.margins.Bottom >= 0 {
			bottom = math.Max(bottom, g.footerDistance()+footer.height(limit))
		}
		return g.height - top - bottom
	}
	// LibreOffice limits header/footer growth while retaining one fifth of the
	// original page text area. Reserve that space proactively, instead of relying
	// on the producer to relocate oversized footer content beyond the page.
	// This is a compatibility allowance, not a Word-layout-equivalence claim.
	minimum := math.Max(docxBodyFloor(d.Blocks, 0), w.contentHeight/5)
	for _, note := range d.Notes {
		minimum = math.Max(minimum, docxBodyFloor(note.Blocks, docxImageNoteHeightAllowance))
	}
	largest := math.Max(header.largestImage(), footer.largestImage())
	fits := func(limit float64, guard float64) bool {
		return available(limit) >= minimum+guard && (len(d.Headers) == 0 || g.margins.Top < 0 || g.headerDistance()+header.height(limit) <= g.height-guard) && (len(d.Footers) == 0 || g.margins.Bottom < 0 || g.footerDistance()+footer.height(limit) <= g.height-guard)
	}
	if fits(largest, 0) {
		return 0, available(largest), nil
	}
	if !fits(0, 0) {
		return 0, 0, fmt.Errorf("%w: DOCX running text/padding leaves no image and body space", ErrUnsupported)
	}
	low, high := 0.0, largest
	for range 40 {
		if err := w.ctx.Err(); err != nil {
			return 0, 0, err
		}
		mid := (low + high) / 2
		if fits(mid, .001) {
			low = mid
		} else {
			high = mid
		}
	}
	if low <= 0 {
		return 0, 0, fmt.Errorf("%w: DOCX running images cannot fit", ErrUnsupported)
	}
	return low, available(low), nil
}

// Fixed stories fit independently to the canvas. A positive story's body budget
// must not shrink an otherwise fitting negative-edge story.
func (w *docxWriter) fitFixedRunning(blocks []Block, g pageGeometry, header bool) (float64, error) {
	if len(blocks) == 0 {
		return 0, nil
	}
	height := g.runningStoryHeight(header, true, true)
	flow, _, err := w.runningFlow(blocks, w.contentWidth, height, 0, 0)
	if err != nil {
		return 0, err
	}
	largest := flow.largestImage()
	if flow.height(largest) <= height {
		return 0, nil
	}
	if flow.height(0) >= height || largest <= 0 {
		return 0, fmt.Errorf("%w: DOCX fixed running text/padding exceeds page anchor", ErrUnsupported)
	}
	low, high := 0.0, largest
	for range 40 {
		if err := w.ctx.Err(); err != nil {
			return 0, err
		}
		mid := (low + high) / 2
		if flow.height(mid) <= height-.001 {
			low = mid
		} else {
			high = mid
		}
	}
	if low <= 0 {
		return 0, fmt.Errorf("%w: DOCX fixed running images cannot fit page", ErrUnsupported)
	}
	return low, nil
}
