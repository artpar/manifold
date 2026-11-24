package manydoc

import (
	"fmt"
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
	_, labelWidth, err := r.footnoteLabel(key, r.bodyWidth())
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
		r.pdf.Text(left, top-3, "Footnotes continued")
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
		r.drawLine(label, labelX, y, 8)
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
		list := Block{Kind: ListBlock, List: "number", Start: listStartPointer(start), Items: []ListItem{{Blocks: blocks}}}
		if note.Label != "" || note.BodyLabel != nil {
			label := noteBodyLabel(r.notes, key)
			list.pdfListMarker = &label
		}
		if err := r.list(list, r.bodyWidth(), r.bodyLeft()); err != nil {
			return err
		}
	}
	return nil
}

// Body marks use the normal font/glyph/shaping path too. A distinct body label
// must never bypass validation merely because the citation used other glyphs.
func (r *pdfRenderer) footnoteLabel(key noteKey, width float64) (pdfLine, float64, error) {
	lines, err := r.lines([]Run{{Text: noteBodyLabel(r.notes, key)}}, width, 8)
	if err != nil {
		return nil, 0, err
	}
	if len(lines) != 1 {
		return nil, 0, fmt.Errorf("%w: footnote label exceeds its content width", ErrUnsupported)
	}
	return lines[0], pdfAdvance(lines[0]), nil
}
