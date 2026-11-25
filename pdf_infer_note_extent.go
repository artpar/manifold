package manydoc

import (
	"math"
	"sort"
)

// Extent is an editorial grouping, separate from the immutable physical bounds.
// A separator-backed footnote zone or a leading endnote page supports following
// coherent blocks. Missing/conflicting evidence leaves those blocks in flow.
type pdfNoteExtentFacts struct {
	page               int
	x0, y0, x1, y1, em float64
	ok                 bool
}

func (r *pdfReader) pdfNoteExtentFacts(b *Block) pdfNoteExtentFacts {
	f := pdfNoteExtentFacts{x0: math.Inf(1), y0: math.Inf(1), x1: math.Inf(-1), y1: math.Inf(-1), ok: true}
	var source func(*SourceInfo)
	source = func(s *SourceInfo) {
		if s == nil || !f.ok {
			return
		}
		if !r.noteInferenceStep() {
			f.ok = false
			return
		}
		if len(s.References) != 0 || len(s.Artifacts) != 0 {
			f.ok = false
			return
		}
		add := func(page int, q Quad) {
			if page < 1 || f.page != 0 && page != f.page {
				f.ok = false
				return
			}
			f.page = page
			x0, y0, x1, y1 := quadBounds(q)
			f.x0, f.y0, f.x1, f.y1 = min(f.x0, x0), min(f.y0, y0), max(f.x1, x1), max(f.y1, y1)
		}
		if s.Quad != nil {
			add(s.Page, *s.Quad)
		}
		for _, region := range s.Regions {
			if !r.noteInferenceStep() {
				f.ok = false
				return
			}
			add(region.Page, region.Quad)
		}
	}
	var walk func(*Block)
	walk = func(block *Block) {
		if !f.ok {
			return
		}
		if !r.noteInferenceStep() {
			f.ok = false
			return
		}
		switch block.Kind {
		case Paragraph, Heading, ListBlock, Table, Picture:
		default:
			f.ok = false
			return
		}
		source(block.Source)
		for _, run := range block.Runs {
			if !r.noteInferenceStep() {
				f.ok = false
				return
			}
			if run.Note != nil {
				f.ok = false
				return
			}
			source(run.Source)
			f.em = max(f.em, run.FontSize)
		}
		for i := range block.Items {
			item := &block.Items[i]
			source(item.Source)
			if item.Marker != nil {
				source(item.Marker.Source)
				for j := range item.Marker.Blocks {
					walk(&item.Marker.Blocks[j])
				}
			}
			for j := range item.Blocks {
				walk(&item.Blocks[j])
			}
		}
		for i := range block.Rows {
			for j := range block.Rows[i].Cells {
				for k := range block.Rows[i].Cells[j].Blocks {
					walk(&block.Rows[i].Cells[j].Blocks[k])
				}
			}
		}
	}
	walk(b)
	f.ok = f.ok && f.page > 0 && finite(f.x0) && finite(f.y0) && finite(f.x1) && finite(f.y1)
	return f
}

func (r *pdfReader) pdfNoteSeparator(page int, left, top, em float64) bool {
	return r.pdfNoteSeparatorWidth(page, left, top, em) > 0
}

func (r *pdfReader) pdfNoteSeparatorWidth(page int, left, top, em float64) float64 {
	for _, line := range r.rulings[page] {
		if !r.noteInferenceStep() {
			return 0
		}
		if line.horizontal && line.width <= max(3, em*.3) && line.pos <= top+1 && line.pos >= top-4*em && line.start <= left+em && line.end >= left+4*em {
			return line.end - line.start
		}
	}
	for _, patch := range r.rulingPatches[page] {
		if !r.noteInferenceStep() {
			return 0
		}
		if patch.y1-patch.y0 <= max(3, em*.3) && patch.y1 <= top+1 && patch.y0 >= top-4*em && patch.x0 <= left+em && patch.x1 >= left+4*em {
			return patch.x1 - patch.x0
		}
	}
	return 0
}

// Plan all moves before assigning references or removing blocks. Existing body
// roots and citation owners are barriers, as are missing geometry, unqualified page
// changes, large gaps, incompatible type sizes and unrelated text columns.
func (r *pdfReader) planPDFNoteExtents(marks []pdfLinkedNoteMark, bodies map[int]int, kinds map[int]string, citations map[int][]int) (map[int]pdfNoteExtentPlan, bool) {
	indices := map[*Block]int{}
	for i := range r.doc.Blocks {
		indices[&r.doc.Blocks[i]] = i
	}
	protected := map[*Block]bool{}
	rootLinks := map[*Block]int{}
	bodyOrder := make([]int, 0, len(bodies))
	for link, mark := range bodies {
		protected[marks[mark].block] = true
		rootLinks[marks[mark].block] = link
		bodyOrder = append(bodyOrder, link)
	}
	sort.Slice(bodyOrder, func(i, j int) bool {
		a, b := indices[marks[bodies[bodyOrder[i]]].block], indices[marks[bodies[bodyOrder[j]]].block]
		if a == b {
			return bodyOrder[i] < bodyOrder[j]
		}
		return a < b
	})
	for _, refs := range citations {
		for _, mark := range refs {
			protected[marks[mark].block] = true
		}
	}
	facts := map[int]pdfNoteExtentFacts{}
	fact := func(index int) pdfNoteExtentFacts {
		if f, ok := facts[index]; ok {
			return f
		}
		f := r.pdfNoteExtentFacts(&r.doc.Blocks[index])
		facts[index] = f
		return f
	}
	extents := map[int]pdfNoteExtentPlan{}
	claims := map[int]int{}
	sharedZone := map[int]bool{}
	sharedWidth := map[int]float64{}
	for _, bodyLink := range bodyOrder {
		markIndex := bodies[bodyLink]
		if len(citations[bodyLink]) == 0 {
			continue
		}
		mark := marks[markIndex]
		index, ok := indices[mark.block]
		if !ok {
			continue
		}
		base := fact(index)
		if !base.ok || base.em <= 0 || base.page > len(r.doc.Pages) {
			continue
		}
		left, _, _, _ := quadBounds(*mark.run().Source.Quad)
		zone := sharedZone[index]
		width := sharedWidth[index]
		if !zone && kinds[bodyLink] == Footnote {
			width = r.pdfNoteSeparatorWidth(base.page, left, base.y0, base.em)
			zone = width > 0
		} else if !zone && kinds[bodyLink] == Endnote && base.y0 < r.doc.Pages[base.page-1].Height*.35 {
			// A leading note page has no prior ordinary same-page content.
			zone = true
			for i := index - 1; i >= 0; i-- {
				if !r.noteInferenceStep() {
					return nil, false
				}
				if r.doc.Blocks[i].Kind == PageBreak {
					break
				}
				prior := fact(i)
				if prior.ok && prior.page == base.page || !prior.ok && r.doc.Blocks[i].Kind != AnchorPoint {
					zone = false
					break
				}
			}
		}
		if !zone {
			continue
		}
		bottom := base.y1
		currentPage, lastIndex := base.page, index
		plan := pdfNoteExtentPlan{joins: map[int]bool{}}
		for i := index + 1; i < len(r.doc.Blocks); i++ {
			if !r.noteInferenceStep() {
				return nil, false
			}
			b := &r.doc.Blocks[i]
			if b.Kind == PageBreak {
				next, join, ok := r.pdfNoteNextPage(i+1, lastIndex, fact(lastIndex), left, base.em, width, kinds[bodyLink], rootLinks, protected, fact)
				if !ok {
					return nil, false
				}
				if next < 0 {
					break
				}
				i = next
				b = &r.doc.Blocks[i]
				currentPage = fact(i).page
				bottom = fact(i).y0
				plan.pages++
				plan.joins[i] = join
			}
			f := fact(i)
			if !pdfNoteCoherentBlock(b, f, currentPage, left, base.em, r.doc.Pages[currentPage-1].Width) || f.y0 < bottom-1 || f.y0-bottom > 4*base.em {
				break
			}
			if protected[b] {
				if next, exists := rootLinks[b]; exists && kinds[next] == kinds[bodyLink] {
					sharedZone[i] = true
					sharedWidth[i] = width
				}
				break
			}
			if owner, exists := claims[i]; exists && owner != bodyLink {
				r.doc.warn("pdf_note_inference_uncertain", "Competing note bodies claim one continuation; the unassigned source remains in flow")
				return nil, false
			}
			claims[i] = bodyLink
			plan.blocks = append(plan.blocks, i)
			lastIndex = i
			bottom = f.y1
		}
		extents[bodyLink] = plan
	}
	if r.noteInferenceWork > pdfNoteInferenceLimit || r.ctx.Err() != nil {
		return nil, false
	}
	return extents, true
}
