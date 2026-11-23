package manydoc

import (
	"fmt"
	"math"
	"strconv"
)

const footnoteSeparatorHeight = 12.0

type pdfNoteChunk struct {
	key    noteKey
	lines  []pdfFragment
	first  bool
	indent float64
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
	indent := math.Max(18, r.pdf.GetStringWidth(r.notes.labels[key])+6)
	lines, err := r.cellFlow(definition.Blocks, r.width-2*r.margin-indent, 0)
	if err != nil {
		return pdfNoteChunk{}, fmt.Errorf("footnote %q: %w", key.id, err)
	}
	return pdfNoteChunk{key: key, lines: lines, first: true, indent: indent}, nil
}

func (r *pdfRenderer) minimumNewFootnotes(keys []noteKey, fresh bool) (float64, error) {
	seen := map[noteKey]bool{}
	height := 0.0
	for _, key := range keys {
		if key.kind != Footnote || r.seenFootnotes[key] || seen[key] {
			continue
		}
		chunk, err := r.measureFootnote(key)
		if err != nil {
			return 0, err
		}
		if len(seen) == 0 && (fresh || len(r.pageNotes) == 0) {
			height += footnoteSeparatorHeight
		}
		seen[key] = true
		height += minimumPDFFragmentHeight(chunk.lines[0])
	}
	return height, nil
}
func (r *pdfRenderer) ensureWithNotes(height float64, keys []noteKey) error {
	if err := r.ctx.Err(); err != nil {
		return err
	}
	contentHeight := r.height - 2*r.margin
	if height > contentHeight {
		return fmt.Errorf("%w: PDF block is taller than a page", ErrUnsupported)
	}
	var additions []pdfNoteChunk
	seen := map[noteKey]bool{}
	desired, minimumStarts := 0.0, 0.0
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
		minimumStarts += minimumPDFFragmentHeight(chunk.lines[0])
	}
	extraSeparator := 0.0
	if len(additions) > 0 && len(r.pageNotes) == 0 {
		extraSeparator = footnoteSeparatorHeight
	}
	required := height + r.noteReserve + extraSeparator + desired
	// Prefer a fresh page to an avoidable note split. Existing notes are flushed
	// before moving the reference, so no already-painted body content overlaps.
	if r.y+required > r.height-r.margin && r.y > r.margin && (height+footnoteSeparatorHeight+desired <= contentHeight || len(additions) == 0) {
		if err := r.nextPage(true); err != nil {
			return err
		}
		extraSeparator = 0
		if len(additions) > 0 {
			extraSeparator = footnoteSeparatorHeight
		}
	}
	if r.y+height+r.noteReserve > r.height-r.margin {
		if err := r.nextPage(true); err != nil {
			return err
		}
		extraSeparator = 0
		if len(additions) > 0 {
			extraSeparator = footnoteSeparatorHeight
		}
	}
	capacity := r.height - r.margin - r.y - height - r.noteReserve - extraSeparator
	if len(additions) > 0 && capacity+1e-7 < minimumStarts {
		if r.y > r.margin || r.noteReserve > 0 {
			if err := r.nextPage(true); err != nil {
				return err
			}
			extraSeparator = footnoteSeparatorHeight
			capacity = contentHeight - height - extraSeparator
		}
		if capacity+1e-7 < minimumStarts {
			return fmt.Errorf("%w: reference block leaves no room for all footnote starts", ErrUnsupported)
		}
	}
	if len(additions) > 0 {
		r.noteReserve += extraSeparator
		remainingStarts := minimumStarts
		for _, chunk := range additions {
			remainingStarts -= minimumPDFFragmentHeight(chunk.lines[0])
			head, tail, used, err := r.takeFragments(chunk.lines, capacity-remainingStarts)
			if err != nil {
				return err
			}
			if len(head) == 0 {
				return fmt.Errorf("%w: no room for footnote start", ErrUnsupported)
			}
			r.pageNotes = append(r.pageNotes, pdfNoteChunk{key: chunk.key, lines: head, first: true, indent: chunk.indent})
			r.noteReserve += used
			capacity -= used
			r.seenFootnotes[chunk.key] = true
			if len(tail) > 0 {
				r.pendingNotes = append(r.pendingNotes, pdfNoteChunk{key: chunk.key, lines: tail, indent: chunk.indent})
			}
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
	top := r.height - r.margin - footnoteSeparatorHeight - height
	r.pdf.SetDrawColor(90, 90, 90)
	r.pdf.Line(r.margin, top, r.margin+math.Min(90, r.width-2*r.margin), top)
	if continued {
		r.pdf.SetFont("Go", "", 7)
		r.pdf.Text(r.margin, top-3, "Footnotes continued")
	}
	y := top + footnoteSeparatorHeight
	for _, chunk := range chunks {
		if chunk.first {
			r.pdf.SetLink(r.noteLinks[chunk.key], y, -1)
		}
		r.pdf.SetFont("Go", "", 8)
		r.pdf.Text(r.margin, y+8, r.notes.labels[chunk.key])
		for _, fragment := range chunk.lines {
			if err := r.drawFragment(fragment, r.margin+chunk.indent, y); err != nil {
				return err
			}
			y += fragment.Height
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
	capacity := r.height - 2*r.margin - footnoteSeparatorHeight - 10
	if len(pending) > 0 && capacity <= 0 {
		return fmt.Errorf("%w: page too small for footnote continuation", ErrUnsupported)
	}
	for len(pending) > 0 {
		if err := r.ctx.Err(); err != nil {
			return err
		}
		r.pdf.AddPage()
		r.y = r.margin
		left := capacity
		var chunks []pdfNoteChunk
		for len(pending) > 0 && left > 0 {
			chunk := pending[0]
			head, tail, used, err := r.takeFragments(chunk.lines, left)
			if err != nil {
				return err
			}
			if len(head) == 0 {
				if len(chunks) == 0 {
					return fmt.Errorf("%w: footnote fragment is taller than a continuation page", ErrUnsupported)
				}
				break
			}
			chunks = append(chunks, pdfNoteChunk{key: chunk.key, lines: head, first: chunk.first, indent: chunk.indent})
			left -= used
			if len(tail) == 0 {
				pending = pending[1:]
			} else {
				pending[0].lines = tail
				pending[0].first = false
			}
		}
		if err := r.paintFootnotes(chunks, true); err != nil {
			return err
		}
	}
	if bodyNeeded {
		r.pdf.AddPage()
		r.y = r.margin
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
	if err := r.blocks([]Block{{Kind: Heading, Level: 1, Runs: []Run{{Text: "Endnotes"}}}}, r.width-2*r.margin, r.margin); err != nil {
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
		if err := r.ensure(minimum); err != nil {
			return err
		}
		r.pdf.SetLink(r.noteLinks[key], r.y, -1)
		start, _ := strconv.Atoi(r.notes.labels[key])
		list := Block{Kind: ListBlock, List: "number", Start: listStartPointer(start), Items: []ListItem{{Blocks: blocks}}}
		if err := r.list(list, r.width-2*r.margin, r.margin); err != nil {
			return err
		}
	}
	return nil
}
