package manydoc

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"unicode/utf8"
)

const pdfNoteInferenceLimit = 4_000_000

// Inference exhaustion keeps original flow and physical evidence, with a diagnostic.
func (r *pdfReader) noteInferenceStep() bool {
	if r.ctx.Err() != nil {
		return false
	}
	r.noteInferenceWork++
	if r.noteInferenceWork > pdfNoteInferenceLimit {
		if r.noteInferenceWork > pdfNoteInferenceLimit+1 {
			r.noteInferenceWork = pdfNoteInferenceLimit + 1
			return false
		}
		r.doc.warn("pdf_note_inference_uncertain", "Note-link inference reached its comparison bound; unproven content remains in flow with original physical navigation")
		return false
	}
	return true
}

func pdfCompactNoteLabel(text string) bool {
	if text == "" || strings.TrimSpace(text) != text || utf8.RuneCountInString(text) > 8 {
		return false
	}
	for _, c := range text {
		if !strings.ContainsRune("0123456789ivxlcdmIVXLCDMabcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ*†‡§¶", c) {
			return false
		}
	}
	return true
}
func pdfQuadOverlap(a, b Quad) float64 {
	ax, ay, ar, ab := quadBounds(a)
	bx, by, br, bb := quadBounds(b)
	area := (ar - ax) * (ab - ay)
	if area <= 0 {
		return 0
	}
	return max(0, min(ar, br)-max(ax, bx)) * max(0, min(ab, bb)-max(ay, by)) / area
}
func pdfPointInQuad(p Point, q Quad) bool {
	x, y, right, bottom := quadBounds(q)
	return p.X >= x-1 && p.X <= right+1 && p.Y >= y-1 && p.Y <= bottom+1
}

// Only layout boxes move: physical glyph quads/baselines remain immutable facts.
// Link-owned raised labels align with adjacent larger text for ordering/grouping.
func (r *pdfReader) alignPDFLinkedLabels(boxes []pdfLayoutBox, page int) {
	if r.doc.PDFNavigation == nil {
		return
	}
	var regions []Quad
	for _, link := range r.doc.PDFNavigation.Links {
		if link.Page == page && link.Kind == "GoTo" {
			regions = append(regions, link.Regions...)
		}
	}
	type correction struct {
		index        int
		base, y0, y1 float64
	}
	var corrections []correction
	for i := range boxes {
		b := &boxes[i]
		if b.block.Kind != Paragraph || len(b.block.Runs) != 1 || !pdfCompactNoteLabel(b.block.Runs[0].Text) || b.block.Source == nil || b.block.Source.Quad == nil {
			continue
		}
		owned := false
		for _, q := range regions {
			if !r.noteInferenceStep() {
				return
			}
			if pdfQuadOverlap(*b.block.Source.Quad, q) >= .6 {
				owned = true
				break
			}
		}
		if !owned {
			continue
		}
		chosen := -1
		ambiguous := false
		for j, other := range boxes {
			if !r.noteInferenceStep() {
				return
			}
			if i == j || other.block.Kind != Paragraph || other.size < b.size*1.25 || other.base-b.base < other.size*.1 || other.base-b.base > other.size*.6 {
				continue
			}
			gap := math.Inf(1)
			if other.x1 <= b.x0+.5 {
				gap = b.x0 - other.x1
			} else if b.x1 <= other.x0+.5 {
				gap = other.x0 - b.x1
			}
			if gap > other.size*.5 {
				continue
			}
			if chosen >= 0 && math.Abs(boxes[chosen].base-other.base) > 1 {
				ambiguous = true
				break
			}
			chosen = j
		}
		if chosen >= 0 && !ambiguous {
			other := boxes[chosen]
			corrections = append(corrections, correction{i, other.base, other.y0, other.y1})
		}
	}
	for _, c := range corrections {
		boxes[c.index].base, boxes[c.index].y0, boxes[c.index].y1 = c.base, c.y0, c.y1
	}
}

type pdfLinkedNoteMark struct {
	block *Block
	index int
	label string
	top   bool
}

func (m pdfLinkedNoteMark) run() *Run { return &m.block.Runs[m.index] }
func pdfRaisedNoteMark(runs []Run, index int) bool {
	mark := runs[index].Source
	if mark == nil || mark.Baseline == nil {
		return false
	}
	for _, neighbor := range []int{index - 1, index + 1} {
		if neighbor < 0 || neighbor >= len(runs) {
			continue
		}
		other := runs[neighbor].Source
		if other == nil || other.Baseline == nil || other.Page != mark.Page || other.FontSize < mark.FontSize*1.25 {
			continue
		}
		rise := other.Baseline[0].Y - mark.Baseline[0].Y
		if rise >= other.FontSize*.1 && rise <= other.FontSize*.6 {
			return true
		}
	}
	return false
}
func (m pdfLinkedNoteMark) leadingBody() bool {
	if !m.top || m.block.Kind != Paragraph || m.block.Direction == "rtl" {
		return false
	}
	for _, run := range m.block.Runs {
		if run.Note != nil {
			return false
		}
	}
	for i, run := range m.block.Runs {
		if run.Text == "" {
			continue
		}
		if i < m.index {
			return false
		}
		if i > m.index && strings.TrimSpace(run.Text) != "" && run.Source != nil && run.Source.FontSize >= m.run().Source.FontSize*1.25 {
			return true
		}
	}
	return false
}

// Reciprocal physical GoTo views plus matching raised marks support an inferred
// note graph; neither a number alone nor a destination alone establishes a note.
func (r *pdfReader) inferLinkedPDFNotes() error {
	nav := r.doc.PDFNavigation
	if nav == nil {
		return nil
	}
	var marks []pdfLinkedNoteMark
	var walk func([]Block, bool)
	walk = func(bs []Block, top bool) {
		for i := range bs {
			b := &bs[i]
			largest := 0.0
			for _, run := range b.Runs {
				if run.Source != nil {
					largest = max(largest, run.Source.FontSize)
				}
			}
			for j, run := range b.Runs {
				if run.Note == nil && pdfCompactNoteLabel(run.Text) && run.Source != nil && run.Source.Quad != nil && run.Source.Baseline != nil && largest >= run.Source.FontSize*1.25 && pdfRaisedNoteMark(b.Runs, j) {
					marks = append(marks, pdfLinkedNoteMark{b, j, run.Text, top})
				}
			}
			for j := range b.Items {
				walk(b.Items[j].Blocks, false)
			}
			for j := range b.Rows {
				for k := range b.Rows[j].Cells {
					walk(b.Rows[j].Cells[k].Blocks, false)
				}
			}
		}
	}
	walk(r.doc.Blocks, true)
	if len(marks) == 0 {
		return nil
	}
	destinations := map[string]PDFDestination{}
	for _, d := range nav.Destinations {
		destinations[d.ID] = d
	}
	owner := make([]int, len(nav.Links))
	for i := range owner {
		owner[i] = -1
	}
	for i, link := range nav.Links {
		if err := r.ctx.Err(); err != nil {
			return err
		}
		if link.Kind != "GoTo" {
			continue
		}
		for j, m := range marks {
			if !r.noteInferenceStep() {
				return r.ctx.Err()
			}
			s := m.run().Source
			if s.Page != link.Page {
				continue
			}
			for _, q := range link.Regions {
				if !r.noteInferenceStep() {
					return r.ctx.Err()
				}
				if pdfQuadOverlap(*s.Quad, q) >= .6 {
					if owner[i] >= 0 && owner[i] != j {
						owner[i] = -2
						break
					}
					owner[i] = j
				}
			}
			if owner[i] == -2 {
				break
			}
		}
	}
	target := make([]int, len(nav.Links))
	for i := range target {
		target[i] = -1
	}
	for i, link := range nav.Links {
		if err := r.ctx.Err(); err != nil {
			return err
		}
		d, ok := destinations[link.Destination]
		if !ok || owner[i] < 0 || d.View != "XYZ" || d.Point == nil || len(d.Parameters) != 3 || d.Parameters[0] == nil || d.Parameters[1] == nil {
			continue
		}
		for j, other := range nav.Links {
			if !r.noteInferenceStep() {
				return r.ctx.Err()
			}
			if i == j || owner[j] < 0 || other.Page != d.Page {
				continue
			}
			for _, q := range other.Regions {
				if !r.noteInferenceStep() {
					return r.ctx.Err()
				}
				if pdfPointInQuad(*d.Point, q) {
					if target[i] >= 0 && target[i] != j {
						target[i] = -2
						break
					}
					target[i] = j
				}
			}
			if target[i] == -2 {
				break
			}
		}
	}
	bodies := map[int]int{}
	kinds := map[int]string{}
	for i, j := range target {
		if j < 0 || target[j] != i {
			continue
		}
		citation, body := marks[owner[i]], marks[owner[j]]
		if citation.label != body.label || !body.leadingBody() {
			continue
		}
		cs, bs := citation.run().Source, body.run().Source
		kind := Endnote
		if bs.Page < cs.Page {
			continue
		}
		if bs.Page == cs.Page {
			left, y, _, _ := quadBounds(*bs.Quad)
			if bs.Page < 1 || bs.Page > len(r.doc.Pages) || bs.Baseline[0].Y-cs.Baseline[0].Y < 40 {
				continue
			}
			if y < r.doc.Pages[bs.Page-1].Height*.6 {
				em := bs.FontSize
				for _, run := range body.block.Runs {
					if !r.noteInferenceStep() {
						return r.ctx.Err()
					}
					em = max(em, run.FontSize)
				}
				if !r.pdfNoteSeparator(bs.Page, left, y, em) {
					continue
				}
			}
			kind = Footnote
		}
		bodies[j] = owner[j]
		kinds[j] = kind
	}
	// Resolve all assignments before mutating: one mark cannot name two bodies.
	markTarget := map[int]int{}
	for i, targetLink := range target {
		if targetLink < 0 || owner[i] < 0 {
			continue
		}
		mark, destinationMark := owner[i], owner[targetLink]
		if previous, ok := markTarget[mark]; ok && previous != destinationMark {
			markTarget[mark] = -2
		} else if !ok {
			markTarget[mark] = destinationMark
		}
	}
	bodyBlocks := map[*Block]bool{}
	for _, mark := range bodies {
		bodyBlocks[marks[mark].block] = true
	}
	citationsByBody := map[int][]int{}
	seen := map[[2]int]bool{}
	for i, bodyLink := range target {
		markIndex, ok := bodies[bodyLink]
		if !ok || owner[i] < 0 || owner[i] == markIndex {
			continue
		}
		citation := owner[i]
		if markTarget[citation] != markIndex || markTarget[markIndex] < 0 || bodyBlocks[marks[citation].block] || marks[citation].label != marks[markIndex].label {
			continue
		}
		key := [2]int{bodyLink, citation}
		if !seen[key] {
			citationsByBody[bodyLink] = append(citationsByBody[bodyLink], citation)
			seen[key] = true
		}
	}
	if err := r.ctx.Err(); err != nil {
		return err
	}
	extents, ok := r.planPDFNoteExtents(marks, bodies, kinds, citationsByBody)
	if !ok {
		return r.ctx.Err()
	}
	removed := map[*Block]bool{}
	continuations := 0
	pageContinuations, paragraphJoins := 0, 0
	bodyLinks := make([]int, 0, len(bodies))
	for bodyLink := range bodies {
		bodyLinks = append(bodyLinks, bodyLink)
	}
	sort.Ints(bodyLinks)
	for _, bodyLink := range bodyLinks {
		markIndex := bodies[bodyLink]
		body := marks[markIndex]
		if removed[body.block] {
			continue
		}
		citations := citationsByBody[bodyLink]
		if len(citations) == 0 {
			continue
		}
		labelRun := *body.run()
		labelSource := cloneSourceInfo(labelRun.Source)
		id := fmt.Sprintf("pdf:inferred-note:%d:%d", labelSource.Page, labelSource.Order)
		originalBody := []Block{*body.block}
		for _, index := range extents[bodyLink].blocks {
			originalBody = append(originalBody, r.doc.Blocks[index])
		}
		source := inferredPDFSource(originalBody, "pdf_note_reciprocal_views", .8)
		noteBody := *body.block
		noteBody.Runs = append(append([]Run(nil), body.block.Runs[:body.index]...), body.block.Runs[body.index+1:]...)
		noteBody.Source = inferredPDFSource([]Block{noteBody}, "pdf_note_reciprocal_body", .8)
		noteBlocks := []Block{noteBody}
		for _, index := range extents[bodyLink].blocks {
			next := r.doc.Blocks[index]
			if extents[bodyLink].joins[index] {
				paragraphJoins++
				previous := &noteBlocks[len(noteBlocks)-1]
				source := inferredPDFSource([]Block{*previous, next}, "pdf_note_page_paragraph_continuation", .65)
				previous.Runs = joinPDFLayoutRuns(append([]Run(nil), previous.Runs...), next.Runs)
				previous.Anchors = append(append([]string(nil), previous.Anchors...), next.Anchors...)
				previous.Source = source
			} else {
				noteBlocks = append(noteBlocks, next)
			}
		}
		if len(noteBlocks) > 1 {
			source.Evidence = "pdf_note_reciprocal_views;separated_note_zone_continuation"
			source.Confidence = .7
			continuations += len(noteBlocks) - 1
		}
		if extents[bodyLink].pages > 0 {
			pageContinuations += extents[bodyLink].pages
			source.Evidence += ";cross_page_note_zone"
			source.Confidence = .65
		}
		labelRun.Text, labelRun.Source, labelRun.Anchor = "", nil, ""
		r.doc.Notes = append(r.doc.Notes, Note{ID: id, Kind: kinds[bodyLink], KindInferred: true, Label: body.label, LabelSource: labelSource, BodyLabelStyle: &labelRun, Source: source, Blocks: noteBlocks})
		for _, index := range citations {
			run := marks[index].run()
			run.Source = cloneSourceInfo(run.Source)
			run.Source.Structure, run.Source.Evidence, run.Source.Confidence = "inferred", "pdf_note_reciprocal_views", .8
			run.Text, run.Link, run.Destination = "", "", ""
			run.Note = &NoteReference{ID: id, Kind: kinds[bodyLink], Inferred: true}
		}
		removed[body.block] = true
		for _, index := range extents[bodyLink].blocks {
			removed[&r.doc.Blocks[index]] = true
		}
	}
	if len(removed) > 0 {
		r.removePDFNoteFlow(removed)
		r.doc.warn("pdf_note_relationship_inferred", "Matching raised marks and reciprocal physical GoTo views infer note relationships; original annotations, views and glyph geometry remain source evidence")
		r.doc.warn("pdf_note_body_extent_inferred", "Note body extent is inferred from its linked leading paragraph and available note-zone evidence; unqualified or ambiguous continuation remains in flow")
		if continuations > 0 {
			r.doc.warn("pdf_note_continuation_inferred", "Consecutive coherent rich blocks in a separator-backed footnote zone or leading endnote page are grouped as inferred continuation; physical bounds remain source facts")
		}
		if pageContinuations > 0 {
			r.doc.warn("pdf_note_page_continuation_inferred", "Matching physical note zones infer ownership across pages; ordinary page content and original physical boundaries remain independently represented")
		}
		if paragraphJoins > 0 {
			r.doc.warn("pdf_note_paragraph_continuation_inferred", "Compatible wrap geometry and boundary text infer one note paragraph across physical pages; explicit or ambiguous paragraph boundaries remain separate")
		}
		r.doc.warn("pdf_note_kind_inferred", "Foot/endnote reflow kind is inferred from page position, not an explicit authorial note policy")
		r.doc.warn("pdf_note_label_presentation", "Inferred body marks retain original style and label_source; reflow may normalize their independent placement")
	}
	return nil
}
