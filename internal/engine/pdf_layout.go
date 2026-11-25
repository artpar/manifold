package engine

import (
	"errors"
	"fmt"
	"github.com/phpdave11/gofpdf"
	"math"
	"strconv"
)

const footnoteSeparatorHeight = 12.0

type pdfNoteChunk struct {
	left, width float64
	rtl         bool
	key         noteKey
	lines       []pdfFragment
	first       bool
	indent      float64
	notice      []pdfFragment
	deferred    bool
}

func lineNoteKeys(line pdfLine) []noteKey {
	var keys []noteKey
	for _, s := range line {
		if s.run.Note != nil {
			keys = append(keys, referenceKey(s.run.Note))
		}
	}
	return keys
}

// Measure a note without admitting it or changing page state. Table continuation
// uses this to keep each reference with at least the first note fragment.
func (r *pdfRenderer) measureFootnote(key noteKey) (pdfNoteChunk, error) {
	definition, ok := r.notes.definitions[key]
	if !ok {
		return pdfNoteChunk{}, fmt.Errorf("missing PDF note")
	}
	r.pdf.SetFont("Go", "", 8)
	r.pdf.SetTextColor(0, 0, 0)
	label, labelWidth, err := r.footnoteLabel(key, r.bodyWidth())
	if err != nil {
		return pdfNoteChunk{}, err
	}
	indent := math.Max(18, labelWidth+6)
	if indent >= r.bodyWidth() {
		return pdfNoteChunk{}, fmt.Errorf("%w: footnote label leaves no body width", ErrUnsupported)
	}
	lines, err := r.cellFlow(definition.Blocks, r.bodyWidth()-indent, 0, r.bodyHeight()-footnoteSeparatorHeight-10)
	if err != nil {
		return pdfNoteChunk{}, fmt.Errorf("footnote %q: %w", key.id, err)
	}
	// A tall independently styled marker owns a measured line before the body.
	// This also keeps it atomic when a rich/table body continues onto another page.
	if pdfLineHeight(label, 8, 1.4) > 14 {
		lines = append([]pdfFragment{{Line: label, TextSize: 8, Height: pdfLineHeight(label, 8, 1.4), Indent: -indent}}, lines...)
	}
	if len(definition.Blocks) > 0 && definition.Blocks[0].Direction == "rtl" && pdfLineHeight(label, 8, 1.4) > 14 {
		lines[0].Indent = r.bodyWidth() - labelWidth
	}
	chunk := pdfNoteChunk{left: r.bodyLeft(), width: r.bodyWidth(), key: key, lines: lines, first: true, indent: indent, rtl: len(definition.Blocks) > 0 && definition.Blocks[0].Direction == "rtl"}
	if imageBoundStart(lines[0]) {
		chunk.notice, err = r.cellFlow([]Block{{Kind: Paragraph, Runs: []Run{{Text: "Continued on a later page"}}}}, r.bodyWidth()-indent, 0, r.bodyHeight()-footnoteSeparatorHeight-10)
		if err != nil {
			return pdfNoteChunk{}, err
		}
	}
	return chunk, nil
}

// Only a minimum start whose height is imposed by a figure can use a notice.
// Ordinary text/table starts keep their established reference-page behavior.
func imageBoundStart(f pdfFragment) bool {
	if f.Image != nil {
		return true
	}
	if f.Table == nil || f.Table.page != nil {
		return false
	}
	cursor := tableFragmentCursor(f)
	if plan := f.Table.table; plan.header != nil {
		for _, c := range plan.cells[:plan.headerLast] {
			if flowHasImage(c.flow) {
				return true
			}
		}
	}
	if plan := f.Table.table; plan.header != nil && cursor.row < plan.headerRows {
		cursor = enterPDFTableRow(plan, f.Table.group, pdfTableCursor{row: plan.headerRows, next: plan.headerLast})
	}
	maximum := 22.0
	bound := false
	for _, c := range cursor.cells {
		p := f.Table.table.cells[c.cell].position
		if p.Row+p.RowSpan != cursor.row+1 || len(c.flow) == 0 {
			continue
		}
		h := minimumPDFFragmentHeight(c.flow[0]) + 12
		if h > maximum {
			maximum = h
			bound = imageBoundStart(c.flow[0])
		} else if h == maximum {
			bound = bound || imageBoundStart(c.flow[0])
		}
	}
	return bound
}
func (r *pdfRenderer) noteStartHeights(chunks []pdfNoteChunk, referenceHeight float64) ([]float64, float64) {
	starts := make([]float64, len(chunks))
	total := 0.0
	for i, c := range chunks {
		starts[i] = minimumPDFFragmentHeight(c.lines[0])
		total += starts[i]
	}
	budget := r.bodyHeight() - referenceHeight - footnoteSeparatorHeight
	// Prefer retaining earlier complete starts when several figures compete.
	for i := len(chunks) - 1; i >= 0 && total > budget+1e-7; i-- {
		if len(chunks[i].notice) == 0 {
			continue
		}
		h := flowHeight(chunks[i].notice)
		if h < starts[i] {
			total -= starts[i] - h
			starts[i] = h
		}
	}
	return starts, total
}
func (r *pdfRenderer) minimumNewFootnotes(keys []noteKey, fresh bool, referenceHeight float64) (float64, error) {
	seen := map[noteKey]bool{}
	var chunks []pdfNoteChunk
	for _, key := range keys {
		if key.kind != Footnote || r.seenFootnotes[key] || seen[key] {
			continue
		}
		chunk, err := r.measureFootnote(key)
		if err != nil {
			return 0, err
		}
		seen[key] = true
		chunks = append(chunks, chunk)
	}
	_, height := r.noteStartHeights(chunks, referenceHeight)
	if height > 0 && (fresh || len(r.pageNotes) == 0) {
		height += footnoteSeparatorHeight
	}
	return height, nil
}
func (r *pdfRenderer) ensureWithNotes(height float64, keys []noteKey) error {
	if err := r.ctx.Err(); err != nil {
		return err
	}
	contentHeight := r.bodyHeight()
	if height > contentHeight {
		return fmt.Errorf("%w: PDF block is taller than a page", ErrUnsupported)
	}
	var additions []pdfNoteChunk
	seen := map[noteKey]bool{}
	desired := 0.0
	together := true
	for _, key := range keys {
		if key.kind != Footnote || r.seenFootnotes[key] || seen[key] {
			continue
		}
		seen[key] = true
		chunk, err := r.measureFootnote(key)
		if err != nil {
			return err
		}
		additions = append(additions, chunk)
		desired += flowHeight(chunk.lines)
		together = together && !flowHasBreak(chunk.lines)
	}
	starts, minimumStarts := r.noteStartHeights(additions, height)
	extraSeparator := 0.0
	if minimumStarts > 0 && len(r.pageNotes) == 0 {
		extraSeparator = footnoteSeparatorHeight
	}
	required := height + r.noteReserve + extraSeparator + desired
	// Prefer a fresh page to an avoidable note split. Existing notes are flushed
	// before moving the reference, so no already-painted body content overlaps.
	if r.y+required > r.bodyBottom() && r.y > r.bodyTop() && (together && height+footnoteSeparatorHeight+desired <= contentHeight || len(additions) == 0) {
		if err := r.nextPage(true); err != nil {
			return err
		}
		extraSeparator = 0
		if minimumStarts > 0 {
			extraSeparator = footnoteSeparatorHeight
		}
	}
	if r.y+height+r.noteReserve > r.bodyBottom() {
		if err := r.nextPage(true); err != nil {
			return err
		}
		extraSeparator = 0
		if minimumStarts > 0 {
			extraSeparator = footnoteSeparatorHeight
		}
	}
	capacity := r.bodyBottom() - r.y - height - r.noteReserve - extraSeparator
	if len(additions) > 0 && capacity+1e-7 < minimumStarts {
		if r.y > r.bodyTop() || r.noteReserve > 0 {
			if err := r.nextPage(true); err != nil {
				return err
			}
			extraSeparator = 0
			if minimumStarts > 0 {
				extraSeparator = footnoteSeparatorHeight
			}
			capacity = contentHeight - height - extraSeparator
		}
		if capacity+1e-7 < minimumStarts {
			return fmt.Errorf("%w: reference block leaves no room for all footnote starts", ErrUnsupported)
		}
	}
	if len(additions) > 0 {
		r.noteReserve += extraSeparator
		remainingStarts := minimumStarts
		for i, chunk := range additions {
			remainingStarts -= starts[i]
			head, tail, used, forced, err := r.takeFragments(chunk.lines, capacity-remainingStarts, false)
			if err != nil {
				return err
			}
			deferred := false
			if len(head) == 0 && !forced && len(chunk.notice) > 0 && starts[i] < minimumPDFFragmentHeight(chunk.lines[0]) {
				head, tail, used = chunk.notice, chunk.lines, flowHeight(chunk.notice)
				deferred = true
			}
			if len(head) == 0 && !forced {
				return fmt.Errorf("%w: no room for footnote start", ErrUnsupported)
			}
			if len(head) > 0 {
				r.pageNotes = append(r.pageNotes, pdfNoteChunk{left: chunk.left, width: chunk.width, rtl: chunk.rtl, key: chunk.key, lines: head, first: !deferred, indent: chunk.indent, deferred: deferred})
			}
			r.noteReserve += used
			capacity -= used
			r.seenFootnotes[chunk.key] = true
			if len(tail) > 0 || forced {
				r.pendingNotes = append(r.pendingNotes, pdfNoteChunk{left: chunk.left, width: chunk.width, rtl: chunk.rtl, key: chunk.key, lines: tail, first: deferred || len(head) == 0, indent: chunk.indent})
			}
		}
		if len(r.pageNotes) == 0 {
			r.noteReserve -= extraSeparator
		}
	}
	return r.ctx.Err()
}
func (r *pdfRenderer) paintFootnotes(chunks []pdfNoteChunk, continued bool) error {
	if len(chunks) == 0 {
		return nil
	}
	height := 0.0
	for _, c := range chunks {
		height += flowHeight(c.lines)
	}
	top := r.bodyBottom() - footnoteSeparatorHeight - height
	r.pdf.SetDrawColor(90, 90, 90)
	left, right := chunks[0].left, chunks[0].left+chunks[0].width
	for _, chunk := range chunks[1:] {
		left = math.Min(left, chunk.left)
		right = math.Max(right, chunk.left+chunk.width)
	}
	separatorWidth := math.Min(90, right-left)
	separatorX := left
	if chunks[0].rtl {
		separatorX = right - separatorWidth
	}
	r.pdf.Line(separatorX, top, separatorX+separatorWidth, top)
	if continued {
		r.pdf.SetFont("Go", "", 7)
		r.pdf.SetTextColor(0, 0, 0)
		marked := r.beginOutputMark(nil)
		r.pdf.Text(left, top-3, "Footnotes continued")
		if marked {
			r.pdf.RawWriteStr("EMC")
		}
	}
	y := top + footnoteSeparatorHeight
	for _, chunk := range chunks {
		if chunk.first {
			r.pdf.SetLink(r.noteLinks[chunk.key], y, -1)
		}
		r.pdf.SetFont("Go", "", 8)
		r.pdf.SetTextColor(0, 0, 0)
		label, labelWidth, err := r.footnoteLabel(chunk.key, chunk.width)
		if err != nil {
			return err
		}
		labelX, bodyX := chunk.left, chunk.left+chunk.indent
		if chunk.rtl {
			labelX = chunk.left + chunk.width - labelWidth
			bodyX = chunk.left
		}
		if pdfLineHeight(label, 8, 1.4) <= 14 {
			before := r.suppressOutputStructure
			r.suppressOutputStructure = before || !chunk.first
			r.drawLine(label, labelX, y, 8)
			r.suppressOutputStructure = before
		}
		noticeTop := y
		for _, fragment := range chunk.lines {
			if err := r.drawFragment(fragment, bodyX, y); err != nil {
				return err
			}
			y += fragment.Height
		}
		if chunk.deferred {
			r.pdf.Link(bodyX, noticeTop, chunk.width-chunk.indent, y-noticeTop, r.noteLinks[chunk.key])
		}
	}
	return r.pdf.Error()
}

// Finish current notes and drain overlong bodies onto continuation-only pages.
// When bodyNeeded is true, start a fresh page after those continuations.
func (r *pdfRenderer) nextPage(bodyNeeded bool) error {
	if err := r.paintFootnotes(r.pageNotes, false); err != nil {
		return err
	}
	r.pageNotes = nil
	r.noteReserve = 0
	pending := r.pendingNotes
	r.pendingNotes = nil
	capacity := r.bodyHeight() - footnoteSeparatorHeight - 10
	if capacity <= 0 {
		for _, chunk := range pending {
			for _, f := range chunk.lines {
				if minimumPDFFragmentHeight(f) > 0 {
					return fmt.Errorf("%w: page too small for footnote continuation", ErrUnsupported)
				}
			}
		}
		capacity = 0
	}
	lastEmpty := false
	for len(pending) > 0 {
		if err := r.ctx.Err(); err != nil {
			return err
		}
		r.addPDFPage()
		r.y = r.bodyTop()
		left := capacity
		var chunks []pdfNoteChunk
		for len(pending) > 0 && (left > 0 || len(pending[0].lines) == 0 || pending[0].lines[0].Break) {
			chunk := pending[0]
			// A trailing break with no remaining body still requests a page.
			if len(chunk.lines) == 0 {
				if chunk.first {
					r.pdf.SetLink(r.noteLinks[chunk.key], r.bodyTop(), -1)
				}
				pending = pending[1:]
				continue
			}
			head, tail, used, forced, err := r.takeFragments(chunk.lines, left, len(chunks) == 0)
			if err != nil {
				return err
			}
			if len(head) == 0 && !forced {
				if len(chunks) == 0 {
					return fmt.Errorf("%w: footnote fragment is taller than a continuation page", ErrUnsupported)
				}
				break
			}
			if len(head) > 0 {
				chunks = append(chunks, pdfNoteChunk{left: chunk.left, width: chunk.width, rtl: chunk.rtl, key: chunk.key, lines: head, first: chunk.first, indent: chunk.indent})
			}
			left -= used
			if len(tail) == 0 {
				pending = pending[1:]
			} else {
				pending[0].lines = tail
				pending[0].first = chunk.first && len(head) == 0
			}
			if forced {
				if len(tail) == 0 {
					pending = append([]pdfNoteChunk{{left: chunk.left, width: chunk.width, rtl: chunk.rtl, key: chunk.key, first: chunk.first && len(head) == 0}}, pending...)
				}
				break
			}
		}
		lastEmpty = len(chunks) == 0
		if err := r.paintFootnotes(chunks, true); err != nil {
			return err
		}
	}
	if bodyNeeded && !lastEmpty {
		r.addPDFPage()
		r.y = r.bodyTop()
	}
	return r.pdf.Error()
}
func (r *pdfRenderer) finishNotes() error {
	if err := r.nextPage(false); err != nil {
		return err
	}
	hasEndnotes := false
	for _, key := range r.notes.order {
		if key.kind == Endnote {
			hasEndnotes = true
		}
	}
	if !hasEndnotes {
		return nil
	}
	if err := r.nextPage(true); err != nil {
		return err
	}
	if err := r.blocks([]Block{{Kind: Heading, Level: 1, Runs: []Run{{Text: "Endnotes"}}}}, r.bodyWidth(), r.bodyLeft()); err != nil {
		return err
	}
	for _, key := range r.notes.order {
		if key.kind != Endnote {
			continue
		}
		note := r.notes.definitions[key]
		blocks := append([]Block(nil), note.Blocks...)
		if len(blocks) == 0 || (blocks[0].Kind != Paragraph && blocks[0].Kind != Heading) {
			blocks = append([]Block{{Kind: Paragraph}}, blocks...)
		}
		size := 11.0
		minimum := size * 1.4
		if blocks[0].Kind == Heading {
			size = 26 - float64(blocks[0].Level)*2
			minimum = size * 1.4 * 2
		}
		if err := r.paragraphBreakBefore(blocks[0]); err != nil {
			return err
		}
		if err := r.ensure(minimum); err != nil {
			return err
		}
		r.pdf.SetLink(r.noteLinks[key], r.y, -1)
		start, _ := strconv.Atoi(noteBodyLabel(r.notes, key))
		list := Block{pdfOutputLabels: []*pdfOutputNode{note.pdfOutputLabel}, Kind: ListBlock, List: "number", Start: listStartPointer(start), Items: []ListItem{{Blocks: blocks}}}
		if note.Label != "" || note.BodyLabel != nil || note.BodyLabelStyle != nil || note.BodyLabelContent != nil {
			label := noteBodyLabel(r.notes, key)
			list.pdfListMarker = &label
			list.pdfListMarkerStyle = note.BodyLabelStyle
			list.pdfListMarkerContent = note.BodyLabelContent
		}
		if err := r.list(list, r.bodyWidth(), r.bodyLeft()); err != nil {
			return err
		}
	}
	return nil
}

// Body marks use the normal font/glyph/shaping path too. A distinct body label
// must never bypass validation merely because the citation used other glyphs.

// A measurement budget is passed through each table (8pt trailing gap and 12pt
// cell padding) to an image (8pt trailing gap). Tightening to this measured bound
// makes progress even when preferred widths, not the initial page budget, limit
// the image. No text/font size is reduced.

// Keep a sub-millipoint guard above the measured floor. Final PDF
// coordinates round to five decimals; cancellation at two fitted story
// edges must not turn a legal body line into an over-height block.

// Search a monotone image budget, rather than subtracting story-height excess
// directly: several serial images can amplify a one-point budget reduction.
// Invalid low image budgets form a lower bound; they never discard content.

// preserve the largest feasible image budget

// The target can lie below fixed text/padding or image coordinate precision.
// Keep the best valid plan, then let the other story supply any remainder.

// Materialize body/cell defaults for a measurement-only flow, and minimize image
// preferences. This finds indivisible text/header/note needs without reserving a
// whole preferred-size body figure ahead of running-content fitting.

// The body painter reserves its paragraph/heading fallback size even for
// empty or explicitly smaller runs. Cell/note style has its own 10pt floor.

// default, first, even

// A physical PDF page has one canvas; a changed paper size starts a new page.

// Definitions remain globally addressable, but unrelated notes must not impose
// their dimensions on a different section. Endnotes render in the final frame.

// Plan a common body for each chain of continuous sections before painting any
// of it. Already-painted content cannot be moved when a later story is taller.

// Preserve established output for a compatible whole chain. If sharing
// is impossible, greedily admit compatible prefixes to fresh pages.

// Plan a candidate shared-page group without changing its input plans. Refit
// positive stories to the group's body floor; fixed stories keep their own
// physical anchors. An incompatible group may still consist of valid sections.

// Body frames may change on a shared page. Its running stories keep
// the geometry of the section that opened this physical page.

// Already measured note fragments must finish in their old vertical
// interval when a new fixed frame cannot contain them.

// The opened page keeps its painted header and queued footer. It is
// page one of the new section, so its next page selects variant two.

// gofpdf runs the old footer before invoking the new header. Only the
// header installs the new geometry, so the old footer keeps its frame.

func (r *pdfRenderer) footnoteLabel(key noteKey, width float64) (pdfLine, float64, error) {
	lines, err := r.lines(noteBodyLabelRuns(r.notes, key), width, 8)
	if err != nil {
		return nil, 0, err
	}
	if len(lines) != 1 {
		return nil, 0, fmt.Errorf("%w: footnote label exceeds its content width", ErrUnsupported)
	}
	placePDFLine(lines[0], pdfAdvance(lines[0]))
	return lines[0], pdfAdvance(lines[0]), nil
}
func (r *pdfRenderer) pageEdges() PageMargins {
	if r.page != nil {
		return r.page.margins
	}
	return PageMargins{r.margin, r.margin, r.margin, r.margin}
}
func (r *pdfRenderer) bodyLeft() float64   { return r.pageEdges().Left }
func (r *pdfRenderer) bodyRight() float64  { return r.width - r.pageEdges().Right }
func (r *pdfRenderer) bodyWidth() float64  { return r.bodyRight() - r.bodyLeft() }
func (r *pdfRenderer) baseTop() float64    { return math.Abs(r.pageEdges().Top) }
func (r *pdfRenderer) baseBottom() float64 { return r.height - math.Abs(r.pageEdges().Bottom) }
func (r *pdfRenderer) headerStart() float64 {
	if r.page != nil && (r.page.header != nil || r.page.explicitMargins) {
		return r.page.headerDistance()
	}
	return r.baseTop() / 2
}
func (r *pdfRenderer) footerEnd() (float64, bool) {
	if r.page != nil && (r.page.footer != nil || r.page.explicitMargins) {
		return r.height - r.page.footerDistance(), true
	}
	return r.height, false
}

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
			if item.Marker != nil {
				if err := validatePDFRunning(item.Marker.Blocks); err != nil {
					return err
				}
			}
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
	headerBudget, footerBudget := budget, budget
	edges := r.pageEdges()
	if edges.Top < 0 {
		headerBudget = r.height - r.headerStart()
	}
	if edges.Bottom < 0 {
		if end, explicit := r.footerEnd(); explicit {
			footerBudget = end
		} else {
			footerBudget = r.height
		}
	}
	header, err := r.measureRunning(headers, headerBudget)
	if err != nil {
		return err
	}
	footer, err := r.measureRunning(footers, footerBudget)
	if err != nil {
		return err
	}
	minimum := 1.0
	if body {

		minimum = math.Max(11*1.4, r.minimumBody) + .001
	}
	for attempt := 0; attempt < 128; attempt++ {
		top, bottom := r.baseTop(), r.baseBottom()
		edges := r.pageEdges()
		headerOverflow, footerOverflow := 0.0, 0.0
		if header != nil {
			header.y = r.headerStart()
			if header.y < 0 || header.y >= r.height {
				return fmt.Errorf("%w: PDF header anchor lies outside page", ErrUnsupported)
			}
			headerOverflow = math.Max(0, header.y+header.height-r.height)
			if edges.Top >= 0 {
				top = math.Max(top, header.y+header.height)
			}
		}
		if footer != nil {
			if end, explicit := r.footerEnd(); explicit {
				footer.y = end - footer.height
			} else {
				footer.y = math.Min(r.height-(r.height-r.baseBottom())/2, r.height-footer.height)
			}
			if footer.y+footer.height <= 0 || footer.y+footer.height > r.height+1e-7 {
				return fmt.Errorf("%w: PDF footer anchor lies outside page", ErrUnsupported)
			}
			footerOverflow = math.Max(0, -footer.y)
			if edges.Bottom >= 0 {
				bottom = math.Min(bottom, footer.y)
			}
		}
		bodyActive := 0
		if header != nil && header.shrink && edges.Top >= 0 {
			bodyActive++
		}
		if footer != nil && footer.shrink && edges.Bottom >= 0 {
			bodyActive++
		}
		if headerOverflow <= 1e-7 && footerOverflow <= 1e-7 && (bottom-top >= minimum-1e-7 || bodyActive == 0 && bottom > top) {
			r.header, r.footer = header, footer
			r.content = &pdfBodyBounds{top: top, bottom: bottom}
			return nil
		}
		if headerOverflow > 1e-7 && !header.shrink || footerOverflow > 1e-7 && !footer.shrink {
			return fmt.Errorf("%w: PDF running text exceeds its physical page", ErrUnsupported)
		}
		deficit := math.Max(0, minimum-(bottom-top))
		if deficit > 1e-7 && bodyActive == 0 {
			return fmt.Errorf("%w: running content leaves no space for the PDF body", ErrUnsupported)
		}
		excess := 0.0
		if bodyActive > 0 && deficit > 0 {
			excess = math.Max(.01, deficit) / float64(bodyActive)
		}
		for i, story := range []*pdfRunningStory{header, footer} {
			if story == nil || !story.shrink {
				continue
			}
			overflow, bodySide, blocks := headerOverflow, edges.Top >= 0, headers
			if i == 1 {
				overflow, bodySide, blocks = footerOverflow, edges.Bottom >= 0, footers
			}
			reduction := overflow
			if bodySide {
				reduction = excess
			}
			if reduction <= 1e-7 {
				continue
			}
			next, err := r.shrinkRunning(blocks, story, story.height-reduction)
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
			low = budget
		} else {
			high = budget
		}
	}
	if fitting != nil {
		return fitting, nil
	}

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
			if marker := b.Items[j].Marker; marker != nil {
				m := *marker
				var err error
				m.Blocks, err = r.minimumRunningBlocks(m.Blocks, true)
				if err != nil {
					return nil, err
				}
				b.Items[j].Marker = &m
			}
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
				if item.Marker != nil {
					bodyDefaults(item.Marker.Blocks)
				}
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

type pdfSectionVariant struct{ header, footer *pdfRunningStory }
type pdfSectionPlan struct {
	section      resolvedSection
	geometry     pageGeometry
	bounds       pdfBodyBounds
	minimumBody  float64
	footerBehind bool
	freshPage    bool
	variants     [3]pdfSectionVariant
}

type pdfSectionScope struct {
	index, total   int
	firstFootnotes map[noteKey]int
}

func sameSectionCanvas(a, b *pdfSectionPlan) bool {
	return a.geometry.width == b.geometry.width && a.geometry.height == b.geometry.height
}

func (r *pdfRenderer) addPDFPage() {
	if r.sectioned {
		orientation := "P"
		if r.width > r.height {
			orientation = "L"
		}
		r.pdf.AddPageFormat(orientation, gofpdf.SizeType{Wd: math.Min(r.width, r.height), Ht: math.Max(r.width, r.height)})
	} else {
		r.pdf.AddPage()
	}
}
func (r *pdfRenderer) planSection(d *Document, s resolvedSection, minimumFloor float64, scope pdfSectionScope) (*pdfSectionPlan, error) {
	oldPage, oldWidth, oldHeight, oldMargin := r.page, r.width, r.height, r.margin
	oldHeader, oldFooter, oldContent, oldMinimum := r.header, r.footer, r.content, r.minimumBody
	defer func() {
		r.page, r.width, r.height, r.margin = oldPage, oldWidth, oldHeight, oldMargin
		r.header, r.footer, r.content, r.minimumBody = oldHeader, oldFooter, oldContent, oldMinimum
	}()
	g := pageLayout(s.layout)
	if g.width > 14400 || g.height > 14400 || fmt.Sprintf("%.2f", g.width) == "0.00" || fmt.Sprintf("%.2f", g.height) == "0.00" {
		return nil, fmt.Errorf("%w: invalid PDF section page dimensions", ErrUnsupported)
	}
	r.page, r.width, r.height, r.margin = &g, g.width, g.height, g.margins.Left
	r.content = nil
	minimum, err := r.minimumSectionBody(s.blocks, scope)
	if err != nil {
		return nil, err
	}
	r.minimumBody = math.Max(minimum, minimumFloor)
	bodyNeeded := len(s.blocks) > 0
	if scope.index == scope.total-1 {
		for _, key := range r.notes.order {
			if key.kind == Endnote {
				bodyNeeded = true
				break
			}
		}
	}
	plan := &pdfSectionPlan{section: s, geometry: g, minimumBody: r.minimumBody, bounds: pdfBodyBounds{top: r.baseTop(), bottom: r.baseBottom()}}
	for i := range plan.variants {
		if i == 1 && !s.firstPage || i == 2 && !d.EvenAndOddHeaders {
			continue
		}
		header := selectStory(s.headers, i == 1, i == 2, s.firstPage, d.EvenAndOddHeaders)
		footer := selectStory(s.footers, i == 1, i == 2, s.firstPage, d.EvenAndOddHeaders)
		if err := r.prepareRunning(header, footer, bodyNeeded); err != nil {
			return nil, err
		}
		plan.variants[i] = pdfSectionVariant{r.header, r.footer}
		plan.bounds.top = math.Max(plan.bounds.top, r.bodyTop())
		plan.bounds.bottom = math.Min(plan.bounds.bottom, r.bodyBottom())
	}
	if plan.bounds.bottom <= plan.bounds.top {
		return nil, fmt.Errorf("%w: running variants leave no common section body", ErrUnsupported)
	}
	return plan, nil
}

func (r *pdfRenderer) minimumSectionBody(blocks []Block, scope pdfSectionScope) (float64, error) {
	references := map[noteKey]bool{}
	var order []noteKey
	if err := walkNoteRuns(blocks, func(run Run) error {
		if run.Note != nil {
			key := referenceKey(run.Note)
			if !references[key] {
				references[key] = true
				order = append(order, key)
			}
		}
		return r.ctx.Err()
	}); err != nil {
		return 0, err
	}
	original, originalSeen := r.notes, r.seenFootnotes
	seen := make(map[noteKey]bool, len(originalSeen)+len(references))
	for key, value := range originalSeen {
		seen[key] = value
	}
	for _, key := range order {
		if first, ok := scope.firstFootnotes[key]; ok && first < scope.index {
			seen[key] = true
		}
	}
	local := *original
	local.order = nil
	for _, key := range order {
		if key.kind == Footnote && !seen[key] {
			local.order = append(local.order, key)
		}
	}
	if scope.index == scope.total-1 {
		for _, key := range original.order {
			if key.kind == Endnote {
				local.order = append(local.order, key)
			}
		}
	}
	r.notes, r.seenFootnotes = &local, seen
	defer func() { r.notes, r.seenFootnotes = original, originalSeen }()
	return r.minimumRunningBody(blocks)
}

func (r *pdfRenderer) planSections(d *Document, sections []resolvedSection) ([]*pdfSectionPlan, error) {
	firstFootnotes := map[noteKey]int{}
	for i, s := range sections {
		if err := walkNoteRuns(s.blocks, func(run Run) error {
			if run.Note != nil && run.Note.Kind == Footnote {
				key := referenceKey(run.Note)
				if _, ok := firstFootnotes[key]; !ok {
					firstFootnotes[key] = i
				}
			}
			return r.ctx.Err()
		}); err != nil {
			return nil, err
		}
	}
	scope := func(i int) pdfSectionScope {
		return pdfSectionScope{index: i, total: len(sections), firstFootnotes: firstFootnotes}
	}
	plans := make([]*pdfSectionPlan, len(sections))
	for i, s := range sections {
		if err := r.ctx.Err(); err != nil {
			return nil, err
		}

		plan, err := r.planSection(d, s, 0, scope(i))
		if err != nil {
			return nil, err
		}
		plans[i] = plan
	}
	for start := 0; start < len(plans); {
		end := start + 1
		for end < len(plans) && sections[end].start == SectionContinuous && sameSectionCanvas(plans[end-1], plans[end]) {
			end++
		}

		group, err := r.planContinuousGroup(d, plans[start:end], start, scope)
		if err == nil {
			copy(plans[start:end], group)
		} else if !errors.Is(err, ErrUnsupported) {
			return nil, err
		} else {
			for part := start; part < end; {
				accepted, err := r.planContinuousGroup(d, plans[part:part+1], part, scope)
				if err != nil {
					return nil, err
				}
				next := part + 1
				for next < end {
					candidate, err := r.planContinuousGroup(d, plans[part:next+1], part, scope)
					if err != nil {
						if !errors.Is(err, ErrUnsupported) {
							return nil, err
						}
						break
					}
					accepted = candidate
					next++
				}
				accepted[0].freshPage = part > start
				copy(plans[part:next], accepted)
				part = next
			}
		}
		start = end
	}
	return plans, nil
}

func (r *pdfRenderer) planContinuousGroup(d *Document, original []*pdfSectionPlan, start int, scope func(int) pdfSectionScope) ([]*pdfSectionPlan, error) {
	minimum := 0.0
	for _, plan := range original {
		minimum = math.Max(minimum, plan.minimumBody)
	}
	plans := make([]*pdfSectionPlan, len(original))
	for i, plan := range original {
		if err := r.ctx.Err(); err != nil {
			return nil, err
		}
		copy := *plan
		plans[i] = &copy
		if plan.minimumBody < minimum {
			fitted, err := r.planSection(d, plan.section, minimum, scope(start+i))
			if err != nil {
				return nil, err
			}
			plans[i] = fitted
		}
	}
	bounds := plans[0].bounds
	footerBehind := false
	for _, plan := range plans {
		bounds.top = math.Max(bounds.top, plan.bounds.top)
		bounds.bottom = math.Min(bounds.bottom, plan.bounds.bottom)
		footerBehind = footerBehind || plan.geometry.margins.Bottom < 0
		for _, variant := range plan.variants {
			if variant.header != nil {
				bounds.top = math.Max(bounds.top, variant.header.y+variant.header.height)
			}
			if variant.footer != nil {
				bounds.bottom = math.Min(bounds.bottom, variant.footer.y)
			}
		}
	}
	for _, plan := range plans {
		own := bounds
		if plan.geometry.margins.Top < 0 {
			own.top = math.Abs(plan.geometry.margins.Top)
		}
		if plan.geometry.margins.Bottom < 0 {
			own.bottom = plan.geometry.height - math.Abs(plan.geometry.margins.Bottom)
		}
		if own.bottom <= own.top || own.bottom-own.top+1e-7 < plan.minimumBody {
			return nil, fmt.Errorf("%w: continuous running stories leave no common body", ErrUnsupported)
		}
		plan.bounds = own
		plan.footerBehind = footerBehind
	}
	return plans, nil
}

func (r *pdfRenderer) renderSections(d *Document) error {
	sections := documentSections(d)
	plans, err := r.planSections(d, sections)
	if err != nil {
		return err
	}
	r.sectioned = true
	var active, pending, owner *pdfSectionPlan
	var ownerFooter *pdfRunningStory
	localPage, pendingPage := 0, 0
	blank, nextBlank := false, false
	r.pdf.SetHeaderFunc(func() {
		blank = nextBlank
		nextBlank = false
		if pending != nil {
			active = pending
			pending = nil
			localPage = pendingPage
			r.page = &active.geometry
			r.width, r.height, r.margin = r.page.width, r.page.height, r.page.margins.Left
			r.content = &active.bounds
			r.pdf.SetMargins(r.bodyLeft(), r.bodyTop(), r.page.margins.Right)
			r.pdf.SetAutoPageBreak(false, r.height-r.bodyBottom())
		}
		localPage++
		variant := 0
		if localPage == 1 && active.section.firstPage {
			variant = 1
		} else if localPage%2 == 0 && d.EvenAndOddHeaders {
			variant = 2
		}
		r.header, r.footer = active.variants[variant].header, active.variants[variant].footer
		owner, ownerFooter = active, r.footer
		r.y = r.bodyTop()
		if !blank {
			r.paintRunning(r.header)
			if owner.footerBehind {
				r.paintRunning(ownerFooter)
			}
		}
	})
	r.pdf.SetFooterFunc(func() {
		if !blank && !owner.footerBehind {

			page, content, width, height, margin := r.page, r.content, r.width, r.height, r.margin
			r.page, r.content = &owner.geometry, &owner.bounds
			r.width, r.height, r.margin = owner.geometry.width, owner.geometry.height, owner.geometry.margins.Left
			r.paintRunning(ownerFooter)
			r.page, r.content, r.width, r.height, r.margin = page, content, width, height, margin
		}
	})
	for i, s := range sections {
		if err := r.ctx.Err(); err != nil {
			return err
		}
		share := i > 0 && !plans[i].freshPage && s.start == SectionContinuous && sameSectionCanvas(plans[i-1], plans[i])
		if share {
			next := plans[i].bounds

			if len(r.pendingNotes) > 0 && (next.top != r.bodyTop() || next.bottom != r.bodyBottom()) || r.noteReserve > 0 && math.Max(r.y, next.top)+r.noteReserve > next.bottom+1e-7 {
				share = false
			}
		}
		if share {

			pending, pendingPage = plans[i], 1
			plan := plans[i]
			r.page, r.content = &plan.geometry, &plan.bounds
			r.margin = plan.geometry.margins.Left
			r.y = math.Max(r.y, r.bodyTop())
			r.pdf.SetMargins(r.bodyLeft(), r.bodyTop(), plan.geometry.margins.Right)
			r.pdf.SetAutoPageBreak(false, r.height-r.bodyBottom())
			if err := r.blocks(s.blocks, r.bodyWidth(), r.bodyLeft()); err != nil {
				return err
			}
			continue
		}
		if i > 0 {
			if err := r.nextPage(false); err != nil {
				return err
			}
		}
		if i > 0 && (s.start == SectionEvenPage && (r.pdf.PageNo()+1)%2 != 0 || s.start == SectionOddPage && (r.pdf.PageNo()+1)%2 != 1) {
			nextBlank = true
			r.addPDFPage()
		}
		plan := plans[i]
		pending, pendingPage = plan, 0

		orientation := "P"
		if plan.geometry.width > plan.geometry.height {
			orientation = "L"
		}
		r.pdf.AddPageFormat(orientation, gofpdf.SizeType{Wd: math.Min(plan.geometry.width, plan.geometry.height), Ht: math.Max(plan.geometry.width, plan.geometry.height)})
		if err := r.blocks(s.blocks, r.bodyWidth(), r.bodyLeft()); err != nil {
			return err
		}
	}
	return nil
}
