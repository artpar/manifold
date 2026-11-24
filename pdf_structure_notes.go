package manydoc

import (
	"fmt"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// Relationships are resolved against the completed tree, not paint order or
// coincident label text. Only a single text mark with an unambiguous target can
// become an inline note atom. Other explicit Ref edges remain source evidence.
type pdfRecoveredNote struct {
	definition Note
	recovered  bool
}

func pdfNoteRole(n *pdfStructureNode) bool { return n.role == "Note" || n.role == "FENote" }

func (s *pdfStructureRecovery) prepareNotes(roots []*pdfStructureNode) error {
	s.notes = map[*pdfStructureNode]*pdfRecoveredNote{}
	s.noteRefs = map[*pdfStructureNode]*pdfRecoveredNote{}
	s.cachedRefs = map[*pdfStructureNode][]Block{}
	s.noteLabels = map[noteKey]string{}
	s.noteOrder = map[noteKey]int{}
	byID := map[string]*pdfStructureNode{}
	inNote, underActual := map[*pdfStructureNode]bool{}, map[*pdfStructureNode]bool{}
	parents := map[*pdfStructureNode]*pdfStructureNode{}
	var nodes []*pdfStructureNode
	var collect func([]*pdfStructureNode, *pdfStructureNode, bool, bool)
	collect = func(ns []*pdfStructureNode, parent *pdfStructureNode, note, actual bool) {
		for _, n := range ns {
			if n.kind != "StructElem" {
				continue
			}
			nodes = append(nodes, n)
			if n.id != "" {
				byID[n.id] = n
			}
			parents[n], inNote[n], underActual[n] = parent, note, actual
			collect(n.kids, n, note || pdfNoteRole(n), actual || n.actual != nil)
		}
	}
	collect(roots, nil, false, false)
	if err := s.prepareActualNoteBoundaries(roots, nodes, parents, underActual); err != nil {
		return err
	}
	targets := map[*pdfStructureNode]map[*pdfStructureNode]bool{}
	add := func(ref, note *pdfStructureNode) {
		if targets[ref] == nil {
			targets[ref] = map[*pdfStructureNode]bool{}
		}
		targets[ref][note] = true
	}
	for _, n := range nodes {
		if err := s.reader.ctx.Err(); err != nil {
			return err
		}
		for _, id := range n.refs {
			target := byID[id]
			if target == nil {
				s.warn("pdf_structure_ref_unresolved", "An explicit structure Ref target is outside the recovered tree; its object identity is retained")
				continue
			}
			if pdfNoteRole(target) && !pdfNoteRole(n) {
				add(n, target)
			}
			// WTPDF also permits identifying citations from the note's reverse Ref.
			if pdfNoteRole(n) && !pdfNoteRole(target) {
				add(target, n)
			}
		}
	}
	inferredRefs, err := s.inferNoteRelationships(roots, nodes, targets, inNote, underActual)
	if err != nil {
		return err
	}
	// Prefer a more specific explicit referring element to its enclosing wrapper.
	// A wrapper spanning multiple independently linked marks cannot be one atom.
	hasInner := map[*pdfStructureNode]bool{}
	for n := range targets {
		for p := parents[n]; p != nil; p = parents[p] {
			hasInner[p] = true
		}
	}
	grouped := map[*pdfStructureNode][]*pdfStructureNode{}
	redundantOwners := map[*pdfStructureNode][]SourceStructureReference{}
	for _, n := range nodes {
		if len(targets[n]) == 0 {
			continue
		}
		if hasInner[n] && !inNote[n] && !underActual[n] {
			if owner := s.redundantNoteOwner(n, targets); owner != nil {
				// Attach actual wrapper edges to their one descendant atom so
				// paragraph joining cannot discard an otherwise flattened owner.
				if len(n.refs) > 0 {
					redundantOwners[owner] = append(redundantOwners[owner], SourceStructureReference{Element: n.id, Targets: append([]string(nil), n.refs...)})
				}
				continue
			}
		}
		if len(targets[n]) != 1 || hasInner[n] || inNote[n] || underActual[n] {
			s.warn("pdf_note_relationship", "Ambiguous, nested or enclosing-ActualText note relationship remains explicit source evidence; visible content is retained")
			continue
		}
		for note := range targets[n] {
			if underActual[note] {
				s.warn("pdf_note_relationship", "A note definition inside opaque ActualText remains in flow without moving it outside the replacement extent")
				continue
			}
			// A citation that encloses its own note body is not a single text mark.
			encloses := false
			for p := parents[note]; p != nil; p = parents[p] {
				if p == n {
					encloses = true
					break
				}
			}
			if encloses {
				s.warn("pdf_note_relationship", "Ref container includes the note body and is retained without replacing visible content")
				continue
			}
			grouped[note] = append(grouped[note], n)
			s.noteRefs[n] = nil
		}
	}
	// Definition order follows the original logical tree, independent of maps.
	ids := map[noteKey]bool{}
	for index, n := range nodes {
		refs := grouped[n]
		if len(refs) == 0 {
			continue
		}
		note := &pdfRecoveredNote{}
		label := ""
		valid := true
		for _, ref := range refs {
			blocks, err := s.node(ref)
			if err != nil {
				return err
			}
			for i := range blocks {
				blocks[i].Source = sourceFromBlocks([]Block{blocks[i]}, ref)
			}
			s.cachedRefs[ref] = blocks
			text, ok := pdfNoteMark(blocks)
			if !ok || (text == "" && ref.actual == nil && pdfNoteHasContentReference(ref)) {
				valid = false
			}
			if label == "" {
				label = text
			}

		}
		if !valid {
			s.warn("pdf_note_label", "Note references do not have recoverable literal text marks; exact content and Ref identities are retained in flow")
			continue
		}
		kind, inferred, err := s.pdfNoteKind(n)
		if err != nil {
			return err
		}
		id := string(n.originalID)
		if id == "" || validateString(id) != nil || ids[noteKey{kind, id}] {
			id = fmt.Sprintf("pdf-note-%d", index+1)
		}
		for ids[noteKey{kind, id}] {
			id += "-"
		}
		ids[noteKey{kind, id}] = true
		note.definition = Note{ID: id, Kind: kind, KindInferred: inferred, Label: label}
		s.noteLabels[noteKey{kind, id}] = label
		s.noteOrder[noteKey{kind, id}] = index
		if inNote[n] {
			s.warn("pdf_note_nested_definition", "An independently cited nested source definition becomes a separate editable note; parent_element retains its original containment")
		}
		if label == "" {
			value := ""
			note.definition.BodyLabel = &value
			s.warn("pdf_note_markerless", "Explicit empty citation marks are preserved; empty body display is the default unless a leading label exists, without inventing visible numbering")
		}
		s.notes[n] = note
		for _, ref := range refs {
			blocks := s.cachedRefs[ref]
			source := sourceFromBlocks(blocks, ref)
			source.References = structureReferences(source.References, redundantOwners[ref])
			source.Evidence = "pdf_note_explicit_ref"
			citation, _ := pdfNoteMark(blocks)
			var run Run
			if len(blocks) > 0 && pdfNoteLabelNeedsRichContent(blocks[0].Runs) {
				content := append([]Run(nil), blocks[0].Runs...)
				for i := range content {
					if content[i].Link != "" || content[i].Destination != "" {
						s.warn("pdf_note_citation_navigation", "Citation annotation navigation remains physical evidence; the editable reference targets its note body")
					}
					content[i].Link = ""
					content[i].Destination = ""
				}
				run = Run{Source: source, Note: &NoteReference{ID: id, Kind: kind, Inferred: inferredRefs[ref] != "", LabelContent: content}}
			} else {
				run = s.structureReplacement(blocks, citation, source)
				run.Text, run.Link, run.Destination = "", "", ""
				run.Note = &NoteReference{ID: id, Kind: kind, Inferred: inferredRefs[ref] != ""}
			}
			if inferredRefs[ref] != "" {
				source.Evidence = "pdf_note_adjacent_reference"
				source.Confidence = .85
				if inferredRefs[ref] == "link_view" {
					source.Evidence = "pdf_note_link_view"
					source.Confidence = .95
				}
			}
			if run.Note.LabelContent == nil && (citation != label || label == "") {
				value := citation
				run.Note.Label = &value
			}
			s.cachedRefs[ref] = []Block{{Kind: Paragraph, Runs: []Run{run}, Source: source}}
			s.noteRefs[ref] = note
		}
	}
	return nil
}

func pdfNoteMark(blocks []Block) (string, bool) {
	if len(blocks) == 0 {
		return "", true
	}
	if len(blocks) != 1 || (blocks[0].Kind != Paragraph && blocks[0].Kind != Heading) {
		return "", false
	}
	var out strings.Builder
	for _, run := range blocks[0].Runs {
		if run.Note != nil || run.Anchor != "" {
			return "", false
		}
		out.WriteString(run.Text)
	}
	text := out.String()
	return text, validateNoteLabel(text) == nil
}

func (s *pdfStructureRecovery) pdfNoteKind(n *pdfStructureNode) (string, bool, error) {
	if value := n.attrs["NoteType"]; value != nil {
		v, err := s.reader.pdf.Dereference(value)
		if err != nil {
			return "", false, err
		}
		name, ok := v.(types.Name)
		if !ok {
			return "", false, fmt.Errorf("FENote NoteType must be a name")
		}
		switch string(name) {
		case "Footnote":
			return Footnote, false, nil
		case "Endnote":
			return Endnote, false, nil
		case "None":
		default:
			return "", false, fmt.Errorf("invalid FENote NoteType %q", name)
		}
	}
	// Choosing an endnote avoids claiming that physical placement proved a
	// reusable authorial bottom-of-page layout. This editorial default is explicit.
	s.warn("pdf_note_kind_inferred", "Note has no explicit foot/endnote kind; endnote reflow is selected with kind_inferred=true; physical source regions remain authoritative")
	return Endnote, true, nil
}

func (s *pdfStructureRecovery) recoverNote(n *pdfStructureNode, note *pdfRecoveredNote) ([]Block, error) {
	if note.recovered {
		return nil, fmt.Errorf("note definition recovered more than once")
	}
	note.recovered = true
	var blocks, labelBlocks []Block
	if n.actual != nil {
		// Apply the same structural replacement semantics as other containers.
		delete(s.notes, n)
		var err error
		blocks, err = s.node(n)
		s.notes[n] = note
		if err != nil {
			return nil, err
		}
	} else {
		for i, kid := range n.kids {
			bs, err := s.node(kid)
			if err != nil {
				return nil, err
			}
			if i == 0 && kid.kind == "StructElem" && kid.role == "Lbl" {
				if label, ok := pdfNoteMark(bs); ok {
					if label != note.definition.Label {
						value := label
						note.definition.BodyLabel = &value
					}
					labelBlocks = bs
					if len(bs) > 0 && pdfNoteLabelNeedsRichContent(bs[0].Runs) {
						note.definition.BodyLabelContent = append([]Run(nil), bs[0].Runs...)
						note.definition.BodyLabel = nil
						note.definition.LabelSource = sourceFromBlocks(bs, kid)
					} else {
						labelRun := s.structureReplacement(bs, label, sourceFromBlocks(bs, kid))
						note.definition.LabelSource = labelRun.Source
						labelRun.Text = ""
						labelRun.Source = nil
						note.definition.BodyLabelStyle = &labelRun
						s.warn("pdf_note_label_presentation", "The explicit leading note label becomes the literal semantic mark; body label formatting/navigation is retained separately from its text, with original geometry/font/paint in label_source")
					}
					continue
				}
				s.warn("pdf_note_body_label", "A distinct or complex explicit note-body label remains visible in the rich body; it is not discarded as an automatic mark")
			}
			blocks = append(blocks, bs...)
		}
	}
	note.definition.Blocks = blocks
	note.definition.Source = sourceFromBlocks(append(append([]Block(nil), labelBlocks...), blocks...), n)
	note.definition.Source.Evidence = "pdf_note_definition"
	s.reader.doc.Notes = append(s.reader.doc.Notes, note.definition)
	s.warn("pdf_note_reflow", "Explicit PDF note relationships are editable notes; source regions retain physical placement while output applies foot/endnote layout")
	return nil, nil
}

func pdfNoteHasContentReference(n *pdfStructureNode) bool {
	if n.kind != "StructElem" {
		return true
	}
	for _, kid := range n.kids {
		if pdfNoteHasContentReference(kid) {
			return true
		}
	}
	return false
}

// Recover the published inline Reference/Note pattern without pretending that
// adjacency or equal labels constitute an explicit PDF Ref edge.
func (s *pdfStructureRecovery) inferNoteRelationships(roots, nodes []*pdfStructureNode, targets map[*pdfStructureNode]map[*pdfStructureNode]bool, inNote, underActual map[*pdfStructureNode]bool) (map[*pdfStructureNode]string, error) {
	inferred := map[*pdfStructureNode]string{}
	incoming := map[*pdfStructureNode]bool{}
	byID := map[string]*pdfStructureNode{}
	for _, n := range nodes {
		if n.id != "" {
			byID[n.id] = n
		}
	}
	for _, n := range nodes {
		for _, id := range n.refs {
			if target := byID[id]; target != nil {
				incoming[target] = true
			}
		}
	}
	var hasRefs func(*pdfStructureNode) bool
	hasRefs = func(n *pdfStructureNode) bool {
		if len(n.refs) > 0 {
			return true
		}
		for _, kid := range n.kids {
			if hasRefs(kid) {
				return true
			}
		}
		return false
	}
	cache := func(n *pdfStructureNode) ([]Block, error) {
		if blocks, ok := s.cachedRefs[n]; ok {
			return blocks, nil
		}
		blocks, err := s.node(n)
		if err != nil {
			return nil, err
		}
		s.cachedRefs[n] = blocks
		return blocks, nil
	}
	var visit func([]*pdfStructureNode) error
	visit = func(siblings []*pdfStructureNode) error {
		for i, ref := range siblings {
			if err := s.reader.ctx.Err(); err != nil {
				return err
			}
			if i+1 < len(siblings) && ref.kind == "StructElem" && ref.role == "Reference" {
				note := siblings[i+1]
				if note.kind == "StructElem" && pdfNoteRole(note) && !inNote[ref] && !underActual[ref] && !inNote[note] && !underActual[note] && note.actual == nil && len(note.kids) > 0 && note.kids[0].kind == "StructElem" && note.kids[0].role == "Lbl" && !incoming[note] && len(targets[ref]) == 0 && !hasRefs(ref) && !hasRefs(note) {
					// Cache only eligible candidate text. Rejected candidates are returned
					// unchanged during flow materialization, so no paint units disappear.
					s.noteRefs[ref] = nil
					a, err := cache(ref)
					if err != nil {
						return err
					}
					b, err := cache(note.kids[0])
					if err != nil {
						return err
					}
					left, okA := pdfNoteMark(a)
					right, okB := pdfNoteMark(b)
					if okA && okB && left != "" && left == right {
						targets[ref] = map[*pdfStructureNode]bool{note: true}
						inferred[ref] = "adjacent"
						s.warn("pdf_note_relationship_inferred", "A Reference immediately preceding a Note with the same nonempty leading label is linked by inferred logical adjacency; no explicit Ref edge is claimed")
					} else {
						delete(s.noteRefs, ref)
						s.warn("pdf_note_relationship_uncertain", "Adjacent Reference/Note labels are empty, different or complex; their visible content is retained without inventing a relationship")
					}
				}
			}
			if err := visit(ref.kids); err != nil {
				return err
			}
		}
		return nil
	}
	if err := visit(roots); err != nil {
		return nil, err
	}
	if err := s.inferLinkedNotes(nodes, targets, inferred, incoming, inNote, underActual, hasRefs, cache); err != nil {
		return nil, err
	}
	return inferred, nil
}

// A physical GoTo view is explicit evidence; associating that point with a
// uniquely labelled Note container is still separately identified inference.
func (s *pdfStructureRecovery) inferLinkedNotes(nodes []*pdfStructureNode, targets map[*pdfStructureNode]map[*pdfStructureNode]bool, inferred map[*pdfStructureNode]string, incoming, inNote, underActual map[*pdfStructureNode]bool, hasRefs func(*pdfStructureNode) bool, cache func(*pdfStructureNode) ([]Block, error)) error {
	nav := s.reader.doc.PDFNavigation
	if nav == nil {
		return nil
	}
	type candidate struct {
		node   *pdfStructureNode
		label  string
		source *SourceInfo
	}
	var notes []candidate
	for _, note := range nodes {
		if !pdfNoteRole(note) || inNote[note] || underActual[note] || note.actual != nil || incoming[note] || hasRefs(note) || len(note.kids) == 0 || note.kids[0].role != "Lbl" {
			continue
		}
		blocks, err := cache(note.kids[0])
		if err != nil {
			return err
		}
		label, ok := pdfNoteMark(blocks)
		source := sourceFromBlocks(blocks, note.kids[0])
		if ok && label != "" && source.Page > 0 && source.Quad != nil {
			notes = append(notes, candidate{note, label, source})
		}
	}
	if len(notes) == 0 {
		return nil
	}
	views := map[string]PDFDestination{}
	for _, view := range nav.Destinations {
		views[view.ID] = view
	}
	comparisons := 0
	for _, ref := range nodes {
		if err := s.reader.ctx.Err(); err != nil {
			return err
		}
		if ref.role != "Link" || inNote[ref] || underActual[ref] || len(targets[ref]) > 0 || hasRefs(ref) {
			continue
		}
		link, conflict := s.logicalLink(ref.kids)
		view, ok := views[link.destination]
		if conflict || link.destination == "" || !ok || view.Unresolved || view.Point == nil {
			continue
		}
		var matches []candidate
		for _, note := range notes {
			comparisons++
			if comparisons > 200000 {
				s.warn("pdf_note_inference_budget", "Further physical note-link matching is retained as source navigation after the bounded inference work limit")
				return nil
			}
			if note.source.Page != view.Page {
				continue
			}
			x0, y0, x1, y1 := quadBounds(*note.source.Quad)
			// Two physical points permit viewer targets just above the font-metric
			// label rectangle, as in the published guide. No page/nearest-note guess.
			if view.Point.X >= x0-2 && view.Point.X <= x1+2 && view.Point.Y >= y0-2 && view.Point.Y <= y1+2 {
				matches = append(matches, note)
			}
		}
		if len(matches) != 1 {
			if len(matches) > 1 {
				s.warn("pdf_note_relationship_uncertain", "A physical Link view overlaps multiple Note label regions; exact navigation remains without invented ownership")
			}
			continue
		}
		blocks, err := cache(ref)
		if err != nil {
			return err
		}
		label, valid := pdfNoteMark(blocks)
		if !valid || label != matches[0].label {
			s.warn("pdf_note_relationship_uncertain", "Physical Link view and Note label do not establish a unique matching literal citation; content/navigation remain in flow")
			continue
		}
		targets[ref] = map[*pdfStructureNode]bool{matches[0].node: true}
		inferred[ref] = "link_view"
		s.noteRefs[ref] = nil
		s.warn("pdf_note_relationship_inferred", "A logically owned GoTo Link targets one matching Note label region within two physical points; note ownership is inferred, while original annotation/view geometry remains explicit")
	}
	return nil
}

func pdfNoteLabelNeedsRichContent(runs []Run) bool {
	if len(runs) < 2 {
		return false
	}
	for _, run := range runs {
		if run.Text == "" {
			return false
		}
	}
	for _, run := range runs[1:] {
		if !samePDFStyle(runs[0], run) {
			return true
		}
	}
	return false
}
