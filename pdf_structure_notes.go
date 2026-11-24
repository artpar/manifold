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
	// Prefer a more specific explicit referring element to its enclosing wrapper.
	// A wrapper spanning multiple independently linked marks cannot be one atom.
	hasInner := map[*pdfStructureNode]bool{}
	for n := range targets {
		for p := parents[n]; p != nil; p = parents[p] {
			hasInner[p] = true
		}
	}
	grouped := map[*pdfStructureNode][]*pdfStructureNode{}
	for _, n := range nodes {
		if len(targets[n]) == 0 {
			continue
		}
		if len(targets[n]) != 1 || hasInner[n] || inNote[n] || underActual[n] {
			s.warn("pdf_note_relationship", "Ambiguous, nested or enclosing-ActualText note relationship remains explicit source evidence; visible content is retained")
			continue
		}
		for note := range targets[n] {
			if inNote[note] || underActual[note] {
				s.warn("pdf_note_relationship", "Nested note definitions are retained in flow without inventing recursive notes")
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
		if label == "" {
			value := ""
			note.definition.BodyLabel = &value
			s.warn("pdf_note_markerless", "Explicit empty citation marks are preserved; empty body display is the default unless a leading label exists, without inventing visible numbering")
		}
		s.notes[n] = note
		for _, ref := range refs {
			blocks := s.cachedRefs[ref]
			source := sourceFromBlocks(blocks, ref)
			source.Evidence = "pdf_note_explicit_ref"
			citation, _ := pdfNoteMark(blocks)
			run := s.structureReplacement(blocks, citation, source)
			run.Text, run.Link, run.Destination = "", "", ""
			run.Note = &NoteReference{ID: id, Kind: kind}
			if citation != label || label == "" {
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
					labelRun := s.structureReplacement(bs, label, sourceFromBlocks(bs, kid))
					note.definition.LabelSource = labelRun.Source
					s.warn("pdf_note_label_presentation", "The explicit leading note label becomes the literal semantic mark; independent label typography is normalized on reflow, with original geometry/font/paint retained in label_source")
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
