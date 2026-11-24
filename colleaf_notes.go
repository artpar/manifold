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
	Drawings          map[string]json.RawMessage `json:"drawings,omitempty"`
	DrawingOrder      []string                   `json:"drawingsOrder,omitempty"`
	Lists             map[string]json.RawMessage `json:"lists,omitempty"`
}
type colleafNoteBinding struct {
	LiteralCitations *string `json:"literalCitations,omitempty"`
	BodyMarkRange    string  `json:"bodyMarkRange,omitempty"`
	AutoLabel        *string `json:"autoLabel,omitempty"`
	NativeID         string  `json:"nativeId"`
	Kind             string  `json:"kind"`
	ID               string  `json:"id"`
}
type colleafNoteIdentities struct {
	Bindings []colleafNoteBinding `json:"bindings"`
}
type colleafNoteSnapshot struct {
	Settings                 json.RawMessage                `json:"settings,omitempty"`
	Locale                   string                         `json:"locale,omitempty"`
	Body                     colleafBody                    `json:"body"`
	Tables                   map[string]json.RawMessage     `json:"tableSource,omitempty"`
	Notes                    map[string]colleafNativeNote   `json:"notes,omitempty"`
	Resources                []colleafResource              `json:"resources,omitempty"`
	Lists                    map[string]json.RawMessage     `json:"lists,omitempty"`
	ID                       string                         `json:"id,omitempty"`
	Drawings                 map[string]json.RawMessage     `json:"drawings,omitempty"`
	DrawingOrder             []string                       `json:"drawingsOrder,omitempty"`
	Title                    string                         `json:"title,omitempty"`
	DocumentStyle            json.RawMessage                `json:"documentStyle,omitempty"`
	Headers                  map[string]colleafRunningStory `json:"headers,omitempty"`
	Footers                  map[string]colleafRunningStory `json:"footers,omitempty"`
	Styles                   map[string]colleafNamedStyle   `json:"styles,omitempty"`
	HeaderFooterDrawingOrder []string                       `json:"headerFooterDrawingsOrder,omitempty"`
	NoteSettings             json.RawMessage                `json:"noteSettings,omitempty"`
}

// Native note IDs share one namespace; SF identities are scoped by kind. This
// sparse resource binds identities/order only, never another copy of note text.
func encodeColleafNotes(ctx context.Context, d *Document) (colleafNoteSnapshot, error) {
	return encodeColleafNotesWithCatalog(ctx, d, nil)
}
func encodeColleafNotesWithCatalog(ctx context.Context, d *Document, catalog *noteCatalog, projection ...map[noteKey]string) (colleafNoteSnapshot, error) {
	out := colleafNoteSnapshot{ID: "manydoc"}
	if err := d.Validate(); err != nil {
		return out, err
	}
	d = resolveNumberingDocument(d)
	if catalog == nil {
		var err error
		catalog, err = catalogNotes(d, false)
		if err != nil {
			return out, err
		}
	}

	citationProjection := colleafCitationProjection(d, catalog)
	if len(projection) > 0 {
		citationProjection = projection[0]
	}
	ids := map[noteKey]string{}
	bindings := colleafNoteIdentities{}
	ownership := colleafListOwnership{Stories: map[string][]colleafListBinding{}}
	imageGeometry := colleafImageGeometry{Stories: map[string][]colleafImageBinding{}}
	imageCache := outputImageCache{}
	out.Notes = map[string]colleafNativeNote{}
	for i, n := range d.Notes {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		if n.Source != nil || n.LabelSource != nil || n.KindInferred {
			return out, fmt.Errorf("%w: native note provenance requires checked intent mapping", ErrUnsupported)
		}

		id := fmt.Sprintf("manydoc-note-%d", i+1)
		ids[noteIdentity(n)] = id
		binding := colleafNoteBinding{NativeID: id, Kind: n.Kind, ID: n.ID}
		if n.Label == "" && catalog.labels[noteIdentity(n)] != "" {
			label := catalog.labels[noteIdentity(n)]
			binding.AutoLabel = &label
		}
		bodyInput := n
		if _, projected := citationProjection[noteIdentity(n)]; projected {
			label := n.Label
			binding.LiteralCitations = &label
			if n.BodyLabel == nil && n.BodyLabelContent == nil && n.BodyLabelStyle == nil {
				actual := catalog.labels[noteIdentity(n)]
				bodyInput.BodyLabel = &actual
			}
		}
		blocks, bodyMark, markLength, err := prepareColleafBodyMark(bodyInput, catalog)
		if bodyMark != nil && binding.LiteralCitations != nil && n.BodyLabel == nil && n.BodyLabelContent == nil {
			bodyMark.UseDefault = true
		}
		if err != nil {
			return out, err
		}
		if bodyMark != nil {
			binding.BodyMarkRange = "manydoc-body-mark-" + id
		}
		bindings.Bindings = append(bindings.Bindings, binding)
		encoded, err := encodeColleafAssetBody(ctx, blocks, nil, out.ID, id, imageCache)
		if err != nil {
			return out, err
		}
		note := colleafNativeNote{ID: id, Kind: n.Kind, Body: encoded.Body, Tables: encoded.Tables, Lists: encoded.Lists, Drawings: encoded.Drawings, DrawingOrder: encoded.DrawingOrder}
		if len(encoded.ListsOwnership) > 0 {
			ownership.Stories[id] = encoded.ListsOwnership
		}
		if len(encoded.ImageGeometry) > 0 {
			imageGeometry.Stories[id] = encoded.ImageGeometry
		}
		if n.Label != "" || binding.AutoLabel != nil {
			mark := n.Label
			if binding.AutoLabel != nil {
				mark = *binding.AutoLabel
			}
			note.Mark = &mark
		}
		if n.BodyLabel != nil {
			show := false
			note.ShowBodyReference = &show
		}
		if bodyMark != nil {
			if bodyMark.Empty {
				style, err := colleafRunStyle(*n.BodyLabelStyle)
				if err != nil {
					return out, err
				}
				note.ReferenceStyle = style
			}
			if err := sealColleafBodyMark(&note, *bodyMark, markLength); err != nil {
				return out, err
			}
		}
		if binding.LiteralCitations != nil {
			empty := ""
			note.Mark = &empty
		}
		out.Notes[id] = note
	}
	encoded, err := encodeColleafAssetBody(ctx, d.Blocks, ids, out.ID, "", imageCache, citationProjection)
	if err != nil {
		return out, err
	}
	out.Body = encoded.Body
	out.Tables = encoded.Tables
	out.Lists = encoded.Lists
	out.Drawings = encoded.Drawings
	out.DrawingOrder = encoded.DrawingOrder
	if len(encoded.ListsOwnership) > 0 {
		ownership.Stories[""] = encoded.ListsOwnership
	}
	if len(encoded.ImageGeometry) > 0 {
		imageGeometry.Stories[""] = encoded.ImageGeometry
	}
	if len(bindings.Bindings) > 0 {
		data, err := json.Marshal(bindings)
		if err != nil {
			return out, err
		}
		out.Resources = []colleafResource{{Name: colleafNoteIdentityResource, Data: string(data)}}
	}
	if len(ownership.Stories) > 0 {
		data, err := json.Marshal(ownership)
		if err != nil {
			return out, err
		}
		out.Resources = append(out.Resources, colleafResource{Name: colleafListResource, Data: string(data)})
	}
	if len(imageGeometry.Stories) > 0 {
		data, err := json.Marshal(imageGeometry)
		if err != nil {
			return out, err
		}
		out.Resources = append(out.Resources, colleafResource{Name: colleafImageResource, Data: string(data)})
	}
	if err := sealColleafIntent(&out); err != nil {
		return colleafNoteSnapshot{}, err
	}
	return out, nil
}

func decodeColleafNotes(ctx context.Context, input colleafNoteSnapshot, settings colleafTextContext) (*Document, error) {
	var err error
	input, err = openColleafIntent(ctx, input)
	if err != nil {
		return nil, err
	}
	input, hyperlinkDiagnostics, err := applyColleafHyperlinkResource(ctx, input)
	if err != nil {
		return nil, err
	}
	if settings.Citations == nil {
		input, settings.Citations, err = openColleafCitations(ctx, input)
		if err != nil {
			return nil, err
		}
	}
	var numberPlan colleafNoteNumberPlan
	if !settings.DeferNoteNumbering {
		numberPlan, err = planColleafNoteNumbers(ctx, input)
		if err != nil {
			return nil, err
		}
		settings.Citations.adaptPlan(&numberPlan)
		settings.NoteLabels = numberPlan.Overrides
	}
	doc := &Document{Diagnostics: hyperlinkDiagnostics}
	ids := map[string]noteKey{}
	autoLabels := map[noteKey]string{}
	expectedBodyMarks := map[string]string{}
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
	ownership := colleafListOwnership{}
	seenLists := false
	seenImages := false
	imageGeometry := colleafImageGeometry{}
	if settings.ImageCache == nil {
		settings.ImageCache = outputImageCache{}
	}
	for _, resource := range input.Resources {
		if resource.Name == "DOC_OBJECT_PERMISSION_PLUGIN" {
			continue
		}
		if resource.Name == "DOC_DRAWING_PLUGIN" {
			if err := checkColleafDrawingMirror(input, resource); err != nil {
				return nil, err
			}
			continue
		}
		if emptyColleafBuiltinResource(resource) {
			continue
		}
		if resource.Name == colleafImageResource {
			if seenImages {
				return nil, fmt.Errorf("duplicate native image geometry resource")
			}
			seenImages = true
			if err := strictSFAttrs(json.RawMessage(resource.Data), &imageGeometry); err != nil {
				return nil, err
			}
			continue
		}
		if resource.Name == colleafListResource {
			if seenLists {
				return nil, fmt.Errorf("duplicate native list ownership resource")
			}
			seenLists = true
			if err := strictSFAttrs(json.RawMessage(resource.Data), &ownership); err != nil {
				return nil, err
			}
			continue
		}
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
			expectedBodyMarks[binding.NativeID] = binding.BodyMarkRange
			if binding.AutoLabel != nil && n.Mark != nil && *n.Mark == *binding.AutoLabel {
				autoLabels[ids[binding.NativeID]] = *binding.AutoLabel
			}

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
		n, bodyMarkScope, err := openColleafBodyMark(ctx, input.Notes[id])
		if err != nil {
			return nil, err
		}
		if expected := expectedBodyMarks[id]; expected != "" && (bodyMarkScope == nil || bodyMarkScope.RangeID != expected) {
			doc.warn("colleaf_stale_note_body_mark", "Expected native body-mark ownership is absent or changed; actual visible story text is retained")
			bodyMarkScope = nil
		}
		key := ids[id]
		if originals[key] {
			return nil, fmt.Errorf("native note identity bindings alias distinct definitions")
		}
		originals[key] = true

		bodySettings := settings
		bodySettings.Citations = nil
		bodySettings.NoteIDs = map[string]noteKey{id: ids[id]}
		bodySettings.Lists = n.Lists
		bodySettings.ListBindings = ownership.Stories[id]
		bodySettings.Drawings = n.Drawings
		bodySettings.DrawingOrder = n.DrawingOrder
		bodySettings.ImageBindings = imageGeometry.Stories[id]
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
				if atom.StartIndex != 0 || atom.NoteID != id || (atom.Type != nil && *atom.Type == 0) {
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
				empty := ""
				note.BodyLabel = &empty
			}
			if err := validateNoteLabel(*n.Mark); err != nil {
				return nil, err
			}
			note.Label = *n.Mark
			if _, ok := autoLabels[key]; ok {
				note.Label = ""
			}

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
			if bodyMarkScope == nil {
				doc.warn("colleaf_note_marker_style", "Native generated marker style is retained on the note body; citation style comes from its native text interval")
			}
		}
		if bodyMarker != nil {
			note.BodyLabelStyle = bodyMarker
		}
		if err = restoreColleafBodyMark(ctx, n, bodyMarkScope, &note, doc); err != nil {
			return nil, err
		}
		if bodyMarkScope != nil && bodyMarkScope.UseDefault && bodyMarkScope.Restored {
			settings.Citations.BodyDefaults[key] = true
		}

		doc.Notes = append(doc.Notes, note)
	}
	settings.NoteIDs = ids
	settings.Lists = input.Lists
	settings.ListBindings = ownership.Stories[""]
	settings.Drawings = input.Drawings
	settings.DrawingOrder = input.DrawingOrder
	settings.ImageBindings = imageGeometry.Stories[""]
	for story := range ownership.Stories {
		if story != "" {
			if _, ok := input.Notes[story]; !ok {
				doc.warn("colleaf_stale_list_ownership", "List ownership for an absent native story ignored; actual native content is retained")
			}
		}
	}
	body, err := decodeColleafBody(ctx, input.Body, settings, input.Tables)
	if err != nil {
		return nil, err
	}
	doc.Blocks = body.Blocks
	doc.Diagnostics = append(doc.Diagnostics, body.Diagnostics...)
	if !settings.DeferNoteNumbering {
		applyColleafNoteNumbers(doc, numberPlan, ids)
		settings.Citations.restoreDefinitions(doc, ids)
		if err = finishColleafNoteNumbering(doc, autoLabels); err != nil {
			return nil, err
		}
	}
	if !settings.DeferNoteNumbering {
		if err = settings.Citations.collapse(ctx, doc); err != nil {
			return nil, err
		}
	}
	if err = validateNotes(doc); err != nil {
		return nil, err
	}
	return doc, nil
}

func finishColleafNoteNumbering(doc *Document, autoLabels map[noteKey]string) error {
	if len(autoLabels) > 0 {
		actual, err := catalogNotes(doc, false)
		if err != nil {
			return err
		}
		for i, n := range doc.Notes {
			if label, ok := autoLabels[noteIdentity(n)]; ok && actual.labels[noteIdentity(n)] != label {
				doc.Notes[i].Label = label
				doc.warn("colleaf_stale_note_numbering", "Native custom mark retained because current citation order no longer matches original automatic numbering")
			}
		}
	}

	return nil
}
