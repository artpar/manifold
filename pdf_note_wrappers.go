package manydoc

// An explicit same-target wrapper around one inline descendant contributes a
// redundant relationship, not another occurrence. Additional paint/objects,
// conflicting targets, note definitions and opaque replacement ranges are not
// transparent wrappers and keep the normal uncertainty diagnostics.
func (s *pdfStructureRecovery) redundantNoteOwner(n *pdfStructureNode, targets map[*pdfStructureNode]map[*pdfStructureNode]bool) *pdfStructureNode {
	if len(targets[n]) != 1 {
		return nil
	}
	var target *pdfStructureNode
	for note := range targets[n] {
		target = note
	}
	var owner *pdfStructureNode
	for {
		if len(n.kids) != 1 || (n.actual != nil && !s.alignedActual[n]) {
			return owner
		}
		n = n.kids[0]
		if n.kind != "StructElem" || pdfNoteRole(n) || !isInlineRole(n.role) {
			return owner
		}
		if refs := targets[n]; len(refs) > 0 {
			if len(refs) != 1 || !refs[target] {
				return nil
			}
			owner = n
		}
	}
}
