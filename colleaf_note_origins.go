package manydoc

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// Note source records bind to checked native identity metadata, never to a note
// array position or to a cached copy of the editable body or labels.
type colleafNoteOrigin struct {
	Version          int         `json:"version"`
	NativeID         string      `json:"nativeId"`
	Kind             string      `json:"kind"`
	ID               string      `json:"id"`
	Source           *SourceInfo `json:"source,omitempty"`
	LabelSource      *SourceInfo `json:"labelSource,omitempty"`
	KindInferred     bool        `json:"kindInferred,omitempty"`
	BodyFingerprint  string      `json:"bodyFingerprint"`
	LabelFingerprint string      `json:"labelFingerprint"`
}

// Fingerprints describe current SF semantics, excluding provenance. This runs
// only on independently encoded SF, not arbitrary native plugin objects.
func colleafSourceFreeHash(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	var tree any
	if err = json.Unmarshal(raw, &tree); err != nil {
		return "", err
	}
	var strip func(any)
	strip = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			delete(x, "source")
			delete(x, "label_source")
			if marks, ok := x["marks"].([]any); ok {
				kept := marks[:0]
				for _, mark := range marks {
					m, object := mark.(map[string]any)
					if !object || m["type"] != "source" {
						kept = append(kept, mark)
					}
				}
				if len(kept) == 0 {
					delete(x, "marks")
				} else {
					x["marks"] = kept
				}
			}
			for _, child := range x {
				strip(child)
			}
			if attrs, ok := x["attrs"].(map[string]any); ok && len(attrs) == 0 {
				delete(x, "attrs")
			}
		case []any:
			for _, child := range x {
				strip(child)
			}
		}
	}
	strip(tree)
	raw, err = json.Marshal(tree)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}
func colleafNoteFingerprints(n StructuredNote) (string, string, error) {
	body, err := colleafSourceFreeHash(n.Content)
	if err != nil {
		return "", "", err
	}
	label, err := colleafSourceFreeHash(StructuredNote{Label: n.Label, BodyLabel: n.BodyLabel, BodyLabelMarks: n.BodyLabelMarks, BodyLabelContent: n.BodyLabelContent})
	return body, label, err
}
func collectColleafNoteOrigins(d *Document, e **colleafDocumentEvidence) error {
	var sf *StructuredDocument
	for i, n := range d.Notes {
		if n.Source == nil && n.LabelSource == nil && !n.KindInferred {
			continue
		}
		if sf == nil {
			var err error
			sf, err = ToStructured(d)
			if err != nil {
				return err
			}
		}
		body, label, err := colleafNoteFingerprints(sf.Attrs.Notes[i])
		if err != nil {
			return err
		}
		if *e == nil {
			*e = &colleafDocumentEvidence{Version: 1}
		}
		(*e).NoteOrigins = append((*e).NoteOrigins, colleafNoteOrigin{Version: 1, NativeID: fmt.Sprintf("manydoc-note-%d", i+1), Kind: n.Kind, ID: n.ID, Source: cloneSourceInfo(n.Source), LabelSource: cloneSourceInfo(n.LabelSource), KindInferred: n.KindInferred, BodyFingerprint: body, LabelFingerprint: label})
	}
	return nil
}
func (o colleafNoteOrigin) validate(pages []PDFPage) error {
	if o.Version != 1 || o.NativeID == "" || o.ID == "" || (o.Kind != Footnote && o.Kind != Endnote) || (o.Source == nil && o.LabelSource == nil && !o.KindInferred) {
		return fmt.Errorf("invalid native note source record")
	}
	for _, s := range []string{o.NativeID, o.ID} {
		if err := validateString(s); err != nil {
			return err
		}
	}
	for _, hash := range []string{o.BodyFingerprint, o.LabelFingerprint} {
		if len(hash) != 64 {
			return fmt.Errorf("invalid native note semantic fingerprint")
		}
		if _, err := hex.DecodeString(hash); err != nil {
			return err
		}
	}
	for _, s := range []*SourceInfo{o.Source, o.LabelSource} {
		if s != nil {
			if err := s.validate(pages); err != nil {
				return err
			}
		}
	}
	return nil
}
func restoreColleafNoteOrigins(d *Document, e *colleafDocumentEvidence, input colleafNoteSnapshot, ids colleafNoteIdentities) error {
	if e == nil || len(e.NoteOrigins) == 0 {
		return nil
	}
	sf, err := ToStructured(d)
	if err != nil {
		return err
	}
	current := map[noteKey]int{}
	for i, n := range d.Notes {
		current[noteIdentity(n)] = i
	}
	bound := map[string]noteKey{}
	for _, b := range ids.Bindings {
		if n, ok := input.Notes[b.NativeID]; ok && n.ID == b.NativeID && n.Kind == b.Kind {
			bound[b.NativeID] = noteKey{b.Kind, b.ID}
		}
	}
	archive := map[string]bool{}
	for _, s := range d.UnboundSources {
		archive[string(sfAttrs(s))] = true
	}
	for _, o := range e.NoteOrigins {
		if err := o.validate(d.Pages); err != nil {
			return err
		}
		key := noteKey{o.Kind, o.ID}
		i, exists := current[key]
		if !exists || bound[o.NativeID] != key {
			for _, s := range []*SourceInfo{o.Source, o.LabelSource} {
				if s == nil {
					continue
				}
				raw := string(sfAttrs(s))
				if !archive[raw] {
					d.UnboundSources = append(d.UnboundSources, *cloneSourceInfo(s))
					archive[raw] = true
				}
			}
			d.warn("colleaf_note_source_unbound", fmt.Sprintf("Original %s %q source facts are retained unbound after its native identity binding disappeared", o.Kind, o.ID))
			if o.KindInferred {
				d.warn("colleaf_note_kind_history", fmt.Sprintf("Original %s %q placement was inferred; no current note-kind assertion is made", o.Kind, o.ID))
			}
			continue
		}
		body, label, err := colleafNoteFingerprints(sf.Attrs.Notes[i])
		if err != nil {
			return err
		}
		n := &d.Notes[i]
		n.Source = cloneSourceInfo(o.Source)
		n.LabelSource = cloneSourceInfo(o.LabelSource)
		n.KindInferred = o.KindInferred
		if n.Source != nil && (body != o.BodyFingerprint || label != o.LabelFingerprint) {
			n.Source.Historical = true
			d.warn("colleaf_note_source_historical", "Original note source facts are retained as historical after current body or marker semantics changed")
		}
		if n.LabelSource != nil && label != o.LabelFingerprint {
			n.LabelSource.Historical = true
			d.warn("colleaf_note_label_source_historical", "Original note-label source facts are retained as historical after current marker semantics changed")
		}
	}
	return nil
}
