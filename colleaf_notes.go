package manydoc

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
)

const colleafNoteIdentityResource = "manydoc.note-identities.v1"

type colleafResource struct {
	Name string `json:"name"`
	Data string `json:"data"`
}
type colleafNativeNote struct {
	ID                string                     `json:"noteId"`
	Kind              string                     `json:"type"`
	Body              colleafBody                `json:"body"`
	Tables            map[string]json.RawMessage `json:"tableSource,omitempty"`
	Mark              *string                    `json:"customMark,omitempty"`
	ReferenceStyle    json.RawMessage            `json:"referenceTextStyle,omitempty"`
	ShowBodyReference *bool                      `json:"showBodyReference,omitempty"`
	Drawings          json.RawMessage            `json:"drawings,omitempty"`
	DrawingOrder      []string                   `json:"drawingsOrder,omitempty"`
	Lists             json.RawMessage            `json:"lists,omitempty"`
}
type colleafNoteBinding struct {
	NativeID string `json:"nativeId"`
	Kind     string `json:"kind"`
	ID       string `json:"id"`
}
type colleafNoteIdentities struct {
	Bindings []colleafNoteBinding `json:"bindings"`
}
type colleafNoteSnapshot struct {
	Body      colleafBody                  `json:"body"`
	Tables    map[string]json.RawMessage   `json:"tableSource,omitempty"`
	Notes     map[string]colleafNativeNote `json:"notes,omitempty"`
	Resources []colleafResource            `json:"resources,omitempty"`
}

// Native note IDs share one namespace; SF identities are scoped by kind. This
// sparse resource binds identities/order only, never another copy of note text.
func encodeColleafNotes(ctx context.Context, d *Document) (colleafNoteSnapshot, error) {
	var out colleafNoteSnapshot
	if err := d.Validate(); err != nil {
		return out, err
	}
	ids := map[noteKey]string{}
	bindings := colleafNoteIdentities{}
	out.Notes = map[string]colleafNativeNote{}
	for i, n := range d.Notes {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		if n.Source != nil || n.LabelSource != nil || n.KindInferred || n.BodyLabelContent != nil || n.BodyLabelStyle != nil {
			return out, fmt.Errorf("%w: native note body mark/provenance requires checked intent mapping", ErrUnsupported)
		}
		if n.BodyLabel != nil && *n.BodyLabel != "" {
			return out, fmt.Errorf("%w: native independent note body mark requires its literal mapping", ErrUnsupported)
		}
		id := fmt.Sprintf("manydoc-note-%d", i+1)
		ids[noteIdentity(n)] = id
		bindings.Bindings = append(bindings.Bindings, colleafNoteBinding{NativeID: id, Kind: n.Kind, ID: n.ID})
		body, tables, err := encodeColleafBody(ctx, n.Blocks)
		if err != nil {
			return out, err
		}
		note := colleafNativeNote{ID: id, Kind: n.Kind, Body: body, Tables: tables}
		if n.Label != "" {
			mark := n.Label
			note.Mark = &mark
		}
		if n.BodyLabel != nil {
			show := false
			note.ShowBodyReference = &show
		}
		out.Notes[id] = note
	}
	body, tables, err := encodeColleafBodyWithNotes(ctx, d.Blocks, ids)
	if err != nil {
		return out, err
	}
	out.Body = body
	out.Tables = tables
	if len(bindings.Bindings) > 0 {
		data, err := json.Marshal(bindings)
		if err != nil {
			return out, err
		}
		out.Resources = []colleafResource{{Name: colleafNoteIdentityResource, Data: string(data)}}
	}
	return out, nil
}

func decodeColleafNotes(ctx context.Context, input colleafNoteSnapshot, settings colleafTextContext) (*Document, error) {
	doc := &Document{}
	ids := map[string]noteKey{}
	keys := make([]string, 0, len(input.Notes))
	for id, n := range input.Notes {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if id == "" || id != n.ID || (n.Kind != Footnote && n.Kind != Endnote) {
			return nil, fmt.Errorf("native note identity/kind mismatch")
		}
		if err := validateString(id); err != nil {
			return nil, err
		}
		ids[id] = noteKey{n.Kind, id}
		keys = append(keys, id)
	}
	sort.Strings(keys)
	// A stale identity record cannot override an edited native ID or kind. It
	// simply ceases to apply; actual native notes/references remain authoritative.
	var order []string
	seenResource := false
	for _, resource := range input.Resources {
		if resource.Name != colleafNoteIdentityResource {
			doc.warn("colleaf_resource", "Native plugin resource "+resource.Name+" is not represented; supported visible content is retained")
			continue
		}
		if seenResource {
			return nil, fmt.Errorf("duplicate native note identity resource")
		}
		seenResource = true
		var bindings colleafNoteIdentities
		if err := strictSFAttrs(json.RawMessage(resource.Data), &bindings); err != nil {
			return nil, err
		}
		seen := map[string]bool{}
		for _, binding := range bindings.Bindings {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if seen[binding.NativeID] || binding.NativeID == "" || binding.ID == "" || (binding.Kind != Footnote && binding.Kind != Endnote) {
				return nil, fmt.Errorf("invalid/duplicate native note identity binding")
			}
			seen[binding.NativeID] = true
			if err := validateString(binding.ID); err != nil {
				return nil, err
			}
			n, ok := input.Notes[binding.NativeID]
			if !ok || n.ID != binding.NativeID || n.Kind != binding.Kind {
				doc.warn("colleaf_stale_note_identity", "Stale note identity metadata ignored; actual native note identity/kind is retained")
				continue
			}
			ids[binding.NativeID] = noteKey{binding.Kind, binding.ID}
			order = append(order, binding.NativeID)
		}
	}
	ordered := map[string]bool{}
	originals := map[noteKey]bool{}
	for _, id := range order {
		ordered[id] = true
	}
	for _, id := range keys {
		if !ordered[id] {
			order = append(order, id)
		}
	}
	for _, id := range order {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		n := input.Notes[id]
		key := ids[id]
		if originals[key] {
			return nil, fmt.Errorf("native note identity bindings alias distinct definitions")
		}
		originals[key] = true
		if (len(n.Drawings) > 0 && string(n.Drawings) != "{}") || len(n.DrawingOrder) > 0 || (len(n.Lists) > 0 && string(n.Lists) != "{}") {
			return nil, fmt.Errorf("%w: native note drawings/lists require their rich mapping", ErrUnsupported)
		}
		bodySettings := settings
		bodySettings.NoteIDs = map[string]noteKey{id: ids[id]}
		body, err := decodeColleafBody(ctx, n.Body, bodySettings, n.Tables)
		if err != nil {
			return nil, err
		}
		note := Note{ID: key.id, Kind: key.kind, Blocks: body.Blocks}
		// The schema permits the matching body-number atom at offset zero.
		// It is the note's body marker, not a recursive citation to itself.
		var bodyMarker *Run
		if len(n.Body.CustomBlocks) > 0 {
			for _, atom := range n.Body.CustomBlocks {
				if atom.StartIndex != 0 {
					continue
				}
				if len(note.Blocks) == 0 || (note.Blocks[0].Kind != Paragraph && note.Blocks[0].Kind != Heading) || len(note.Blocks[0].Runs) == 0 || note.Blocks[0].Runs[0].Note == nil {
					return nil, fmt.Errorf("invalid native note body marker")
				}
				if n.ShowBodyReference != nil && !*n.ShowBodyReference {
					return nil, fmt.Errorf("%w: contradictory native hidden/explicit body marker", ErrUnsupported)
				}
				style := note.Blocks[0].Runs[0]
				style.Note = nil
				bodyMarker = &style
				note.Blocks[0].Runs = note.Blocks[0].Runs[1:]
				if len(note.Blocks[0].Runs) == 0 {
					note.Blocks[0].Runs = nil
				}
			}
		}

		doc.Diagnostics = append(doc.Diagnostics, body.Diagnostics...)
		if n.Mark != nil {
			if *n.Mark == "" {
				return nil, fmt.Errorf("%w: explicit empty native custom mark requires markerless semantics mapping", ErrUnsupported)
			}
			if err := validateNoteLabel(*n.Mark); err != nil {
				return nil, err
			}
			note.Label = *n.Mark
		}
		if n.ShowBodyReference != nil && !*n.ShowBodyReference {
			empty := ""
			note.BodyLabel = &empty
		}
		if len(n.ReferenceStyle) > 0 {
			style, err := colleafStyleObject(n.ReferenceStyle)
			if err != nil {
				return nil, err
			}
			run, err := colleafStyleRun(style, doc)
			if err != nil {
				return nil, err
			}
			note.BodyLabelStyle = &run
			doc.warn("colleaf_note_marker_style", "Native generated marker style is retained on the note body; citation style comes from its native text interval")
		}
		if bodyMarker != nil {
			note.BodyLabelStyle = bodyMarker
		}
		doc.Notes = append(doc.Notes, note)
	}
	settings.NoteIDs = ids
	body, err := decodeColleafBody(ctx, input.Body, settings, input.Tables)
	if err != nil {
		return nil, err
	}
	doc.Blocks = body.Blocks
	doc.Diagnostics = append(doc.Diagnostics, body.Diagnostics...)
	if err = validateNotes(doc); err != nil {
		return nil, err
	}
	return doc, nil
}
