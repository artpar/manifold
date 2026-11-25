package manifold

import "strings"

// A side-effect-free recovery pass identifies ActualText boundaries whose exact
// characters correspond to their one inline paragraph. No substring search or
// equal-label heuristic maps a citation into different replacement characters.
// Only needed when an ActualText ancestor might hide a note relationship.
func (s *pdfStructureRecovery) prepareActualNoteBoundaries(roots, nodes []*pdfStructureNode, parents map[*pdfStructureNode]*pdfStructureNode, underActual map[*pdfStructureNode]bool) error {
	needed := false
	for _, n := range nodes {
		if underActual[n] && (len(n.refs) > 0 || n.role == "Reference" || n.role == "Link" || pdfNoteRole(n)) {
			needed = true
			break
		}
	}
	if !needed {
		return nil
	}
	reader := *s.reader
	doc := *reader.doc
	doc.Diagnostics = nil
	reader.doc = &doc
	shadow := &pdfStructureRecovery{
		reader: &reader, units: append([]pdfContentUnit(nil), s.units...),
		marked: s.marked, alignedActual: map[*pdfStructureNode]bool{},
	}
	if _, err := shadow.children(roots); err != nil {
		return err
	}
	s.alignedActual = shadow.alignedActual
	for _, n := range nodes {
		underActual[n] = false
		for p := parents[n]; p != nil; p = parents[p] {
			if p.actual != nil && !s.alignedActual[p] {
				underActual[n] = true
				break
			}
		}
	}
	return nil
}

// Synthetic geometric separators have no character authority under ActualText.
// Preserve exact original characters/styles/relationships only when all text is
// one paragraph and the replacement equals their complete concatenation. Images,
// tables and multiple paragraphs cannot acquire an invented inline mapping.
func (s *pdfStructureRecovery) alignActualNoteText(blocks []Block, text string, n *pdfStructureNode) ([]Block, bool) {
	if len(blocks) != 1 || (blocks[0].Kind != Paragraph && blocks[0].Kind != Heading) {
		return nil, false
	}
	// Moving a definition out of a replacement range changes its character
	// extent even when its original text matched. Keep those ranges opaque.
	var containsDefinition func([]*pdfStructureNode) bool
	containsDefinition = func(nodes []*pdfStructureNode) bool {
		for _, child := range nodes {
			if pdfNoteRole(child) || containsDefinition(child.kids) {
				return true
			}
		}
		return false
	}
	if containsDefinition(n.kids) {
		return nil, false
	}
	var literal strings.Builder
	var runs []Run
	for _, run := range blocks[0].Runs {
		if run.Source != nil && run.Source.Precision == "synthetic_separator" {
			continue
		}
		if run.Note == nil {
			literal.WriteString(run.Text)
		} else {
			label, ok := s.noteLabels[referenceKey(run.Note)]
			if !ok {
				return nil, false
			}
			if run.Note.LabelContent != nil {
				label = noteLabelContentText(run.Note.LabelContent)
			} else if run.Note.Label != nil {
				label = *run.Note.Label
			}
			literal.WriteString(label)
		}
		runs = append(runs, run)
	}
	if literal.String() != text {
		return nil, false
	}
	for i := range runs {
		if runs[i].Source != nil {
			source := *runs[i].Source
			source.Evidence += ";pdf_structure_actualtext_aligned"
			runs[i].Source = &source
		}
	}
	block := blocks[0]
	block.Runs = runs
	block.Source = sourceFromBlocks([]Block{block}, n)
	block.Source.Evidence = "pdf_structure_actualtext_aligned"
	return []Block{block}, true
}
