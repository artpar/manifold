package engine

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"reflect"
	"strings"

	// Wire records follow the pinned IDocumentBody schema. Text styles/properties
	// remain raw until the semantic bridge resolves defaults and diagnoses losses;
	// accepting their offsets alone does not claim supported style conversion.
	"unicode/utf8"
)

type vdocsBody struct {
	DataStream         string                 `json:"dataStream"`
	TextRuns           []vdocsTextRun         `json:"textRuns"`
	Paragraphs         []vdocsParagraphRecord `json:"paragraphs,omitempty"`
	SectionBreaks      []vdocsSectionRecord   `json:"sectionBreaks,omitempty"`
	Tables             []vdocsTableRecord     `json:"tables,omitempty"`
	CustomBlocks       []vdocsBlockRecord     `json:"customBlocks,omitempty"`
	CustomRanges       []vdocsRangeRecord     `json:"customRanges,omitempty"`
	RenderedPageBreaks []int                  `json:"renderedPageBreaks,omitempty"`
}
type vdocsTextRun struct {
	St      int             `json:"st"`
	Ed      int             `json:"ed"`
	StyleID string          `json:"sId,omitempty"`
	TS      json.RawMessage `json:"ts,omitempty"`
}
type vdocsParagraphRecord struct {
	StartIndex int             `json:"startIndex"`
	ID         string          `json:"paragraphId,omitempty"`
	StyleID    string          `json:"styleId,omitempty"`
	Style      json.RawMessage `json:"paragraphStyle,omitempty"`
	Bullet     json.RawMessage `json:"bullet,omitempty"`
}
type vdocsSectionRecord struct {
	Extra map[string]json.RawMessage `json:"-"`
	vdocsSectionProperties
	StartIndex int    `json:"startIndex"`
	ID         string `json:"sectionId,omitempty"`
}
type vdocsTableRecord struct {
	StartIndex int    `json:"startIndex"`
	EndIndex   int    `json:"endIndex"`
	ID         string `json:"tableId"`
}
type vdocsBlockRecord struct {
	StartIndex int    `json:"startIndex"`
	Type       *int   `json:"blockType,omitempty"`
	ID         string `json:"blockId"`
	NoteID     string `json:"noteId,omitempty"`
	NoteType   string `json:"noteType,omitempty"`
}
type vdocsRangeRecord struct {
	Active     *bool           `json:"active,omitempty"`
	StartIndex int             `json:"startIndex"`
	EndIndex   int             `json:"endIndex"`
	ID         string          `json:"rangeId"`
	Type       int             `json:"rangeType"`
	Whole      bool            `json:"wholeEntity,omitempty"`
	Properties json.RawMessage `json:"properties,omitempty"`
}

type vdocsBodyIndex struct {
	text     vdocsTextIndex
	sections []vdocsStreamNode
	tables   map[int]vdocsTableRecord
	blocks   map[int]vdocsBlockRecord
}

func indexVDOCSBody(ctx context.Context, body vdocsBody) (*vdocsBodyIndex, error) {
	text, err := newVDOCSTextIndex(ctx, body.DataStream)
	if err != nil {
		return nil, err
	}
	sections, err := parseVDOCSIndexedStream(ctx, text)
	if err != nil {
		return nil, err
	}
	index := &vdocsBodyIndex{text: text, sections: sections, tables: map[int]vdocsTableRecord{}, blocks: map[int]vdocsBlockRecord{}}
	token := func(offset int, ch rune) error {
		b, err := text.byteBoundary(offset)
		if err != nil {
			return err
		}
		if b == len(body.DataStream) || !strings.HasPrefix(body.DataStream[b:], string(ch)) {
			return fmt.Errorf("native offset %d must point at U+%04X", offset, ch)
		}
		return nil
	}
	for _, run := range body.TextRuns {
		// The actual reader accepts unsorted/overlapping intervals; the last
		// matching record wins as a whole. Semantic mapping must preserve that
		// rule instead of unioning marks or rejecting valid native input.
		if _, err := text.exclusive(run.St, run.Ed); err != nil {
			return nil, err
		}
	}
	positions := map[int]bool{}
	for _, paragraph := range body.Paragraphs {
		if positions[paragraph.StartIndex] {
			return nil, fmt.Errorf("duplicate native paragraph position")
		}
		if err := token(paragraph.StartIndex, vdocsParagraph); err != nil {
			return nil, err
		}
		positions[paragraph.StartIndex] = true
	}
	positions = map[int]bool{}
	for _, section := range body.SectionBreaks {
		if positions[section.StartIndex] {
			return nil, fmt.Errorf("duplicate native section position")
		}
		if err := token(section.StartIndex, vdocsSection); err != nil {
			return nil, err
		}
		positions[section.StartIndex] = true
	}
	structuralTables := map[int]int{}
	var collect func([]vdocsStreamNode)
	collect = func(nodes []vdocsStreamNode) {
		for _, node := range nodes {
			if node.kind == "table" {
				structuralTables[node.start] = node.end
			}
			collect(node.children)
		}
	}
	collect(sections)
	ids := map[string]bool{}
	for _, table := range body.Tables {
		if table.ID == "" || ids[table.ID] {
			return nil, fmt.Errorf("native table identity must be nonempty and unique")
		}
		if _, exists := index.tables[table.StartIndex]; exists {
			return nil, fmt.Errorf("duplicate native table position")
		}
		if end, exists := structuralTables[table.StartIndex]; !exists || end != table.EndIndex {
			return nil, fmt.Errorf("native table metadata must match a complete exclusive token extent")
		}
		if _, err := text.exclusive(table.StartIndex, table.EndIndex); err != nil {
			return nil, err
		}
		ids[table.ID] = true
		index.tables[table.StartIndex] = table
	}
	if len(index.tables) != len(structuralTables) {
		return nil, fmt.Errorf("native table token lacks its metadata record")
	}
	for _, block := range body.CustomBlocks {
		if block.ID == "" {
			return nil, fmt.Errorf("native custom block requires identity")
		}
		if err := token(block.StartIndex, vdocsCustomBlock); err != nil {
			return nil, err
		}
		if _, exists := index.blocks[block.StartIndex]; exists {
			return nil, fmt.Errorf("duplicate native custom block position")
		}
		if block.Type != nil && *block.Type != 0 && *block.Type != 1 {
			return nil, fmt.Errorf("%w: native custom block type %d", ErrUnsupported, *block.Type)
		}
		index.blocks[block.StartIndex] = block
	}
	for offset, b := range text.byteAt {
		if b >= 0 && b < len(body.DataStream) && body.DataStream[b] == '\b' {
			if _, exists := index.blocks[offset]; !exists {
				return nil, fmt.Errorf("native custom-block token lacks its metadata record")
			}
		}
	}
	ids = map[string]bool{}
	for _, annotation := range body.CustomRanges {
		if annotation.ID == "" || ids[annotation.ID] {
			return nil, fmt.Errorf("native custom range identity must be nonempty and unique")
		}
		if annotation.Type == 3 && annotation.StartIndex == annotation.EndIndex {
			if annotation.StartIndex >= text.length() {
				return nil, fmt.Errorf("native bookmark is outside the stream")
			}
			if _, err := text.byteBoundary(annotation.StartIndex); err != nil {
				return nil, err
			}
		} else if _, err := text.inclusive(annotation.StartIndex, annotation.EndIndex); err != nil {
			return nil, err
		}
		ids[annotation.ID] = true
		if annotation.Type == 8 || annotation.Type == 9 {
			if annotation.StartIndex != annotation.EndIndex {
				return nil, fmt.Errorf("native note range must own exactly one atom")
			}
			if err := token(annotation.StartIndex, '\ufffc'); err != nil {
				return nil, err
			}
			var props struct {
				NoteID string `json:"noteId"`
			}
			if err := strictSFAttrs(annotation.Properties, &props); err != nil {
				return nil, err
			}
			if props.NoteID == "" {
				return nil, fmt.Errorf("native note range lacks definition identity")
			}
			if _, exists := index.blocks[annotation.StartIndex]; exists {
				return nil, fmt.Errorf("conflicting native note atom ownership")
			}
			kind := Footnote
			if annotation.Type == 9 {
				kind = Endnote
			}
			one := 1
			index.blocks[annotation.StartIndex] = vdocsBlockRecord{StartIndex: annotation.StartIndex, Type: &one, ID: annotation.ID, NoteID: props.NoteID, NoteType: kind}
		}
	}
	positions = map[int]bool{}
	for _, offset := range body.RenderedPageBreaks {
		if positions[offset] {
			return nil, fmt.Errorf("native rendered page-break positions must be unique")
		}
		if err := token(offset, '\f'); err != nil {
			return nil, err
		}
		positions[offset] = true
	}
	return index, nil
}

func (s *vdocsSectionRecord) UnmarshalJSON(data []byte) error {
	type alias vdocsSectionRecord
	var value alias
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	known := map[string]bool{}
	for _, key := range []string{"startIndex", "sectionId", "sectionType", "sectionTypeNext", "pageSize", "pageOrient", "documentFlavor", "marginTop", "marginRight", "marginBottom", "marginLeft", "marginHeader", "marginFooter", "defaultHeaderId", "defaultFooterId", "firstPageHeaderId", "firstPageFooterId", "evenPageHeaderId", "evenPageFooterId", "useFirstPageHeaderFooter", "evenAndOddHeaders"} {
		known[key] = true
	}
	for key, raw := range fields {
		if !known[key] {
			if value.Extra == nil {
				value.Extra = map[string]json.RawMessage{}
			}
			value.Extra[key] = raw
		}
	}
	*s = vdocsSectionRecord(value)
	return nil
}

// Keep uncaptured native section properties when transporting a parsed snapshot
// to an independent reader. Semantic import still diagnoses or rejects them.

// VDOCSMIME is the MIME type required for VDOCS version-3 native documents.

// ReadVDOCS reads a VDOCS version-3 envelope for Univer 1.0.4. It validates
// native structure, relationships and interpreted metadata before returning a
// logical document. Unknown or unrepresentable constructs reject or produce
// explicit diagnostics. Bare editor snapshots and renamed SF are not accepted.

// WriteVDOCS writes a VDOCS version-3 envelope for Univer 1.0.4. It preserves
// supported logical content, assets, diagnostic history and source facts. It
// rejects unsupported mappings; it does not validate application admission limits
// or guarantee native browser rendering. See docs/VDOCS_FORMAT.md for scope.

// The internal worker reads the envelope rather than a bare editor snapshot.

// Native resources encode JSON inside strings. Validate the interpreted payloads
// as well as the outer JSON before any of those maps can collapse duplicates.

// Original source evidence and diagnostic history are nonvisual intent, not a
// cached body and not a claim about the native editor's current page geometry.

// Preserve historical order and duplicates exactly. Deduplicate only newly
// generated warnings against history so repeated save/reopen does not grow it.

// Unregistered resources are removed by the actual FDocument.save API. A native
// CUSTOM annotation on the terminal LF survives that API and normal text edits.
// Its payload contains checked missing intent only, never a whole SF/text cache.

// Copy slices before filtering: caller-owned native records remain untouched.

// Verify the scalar range before treating the annotation as metadata. TextX
// may expand/move an annotation during editing; its bounds do not own text.

// The registered drawing resource may mirror a subset of persisted story
// drawings. Ignore it only after checking every mirrored descriptor and order ID.

// This is a parser allocation bound, independent of application admission quotas.

// Include base64 expansion before choosing the compact representation.

// Native Univer snapshots index UTF-16 code units. Go strings remain UTF-8;
// byteAt marks the interior of an astral scalar with -1 so no range operation
// can slice one surrogate half or silently replace visible text.

// Published interface prose has stale table-end token comments. These values
// agree with @univerjs/core 1.0.4 DataStreamTreeTokenType and executable parser.

// UTF-16, inclusive/exclusive, including structural sentinels

// Sections, including those inside cells, end in LF. Every section ends in a
// paragraph CR immediately before its LF. Tables live inside paragraph extents;
// cell bodies therefore need their own CR/LF, not just CR. No document content
// is inferred from unbalanced/missing tokens.

// CR/LF cases have already advanced through their structural token.

func (s vdocsSectionRecord) MarshalJSON() ([]byte, error) {
	type alias vdocsSectionRecord
	data, err := json.Marshal(alias(s))
	if err != nil {
		return nil, err
	}
	if len(s.Extra) == 0 {
		return data, nil
	}
	var fields map[string]json.RawMessage
	if err = json.Unmarshal(data, &fields); err != nil {
		return nil, err
	}
	for key, raw := range s.Extra {
		if _, exists := fields[key]; exists {
			return nil, fmt.Errorf("conflicting native section property %q", key)
		}
		fields[key] = raw
	}
	return json.Marshal(fields)
}

const VDOCSMIME = "application/vnd.manifold.docs+json;version=3"

const vdocsEnvelopeType = "manifold-document"
const vdocsEngineVersion = "1.0.4"

type vdocsEnvelope struct {
	Type          string            `json:"type"`
	Version       int               `json:"version"`
	Engine        string            `json:"engine"`
	EngineVersion string            `json:"engineVersion"`
	Document      vdocsNoteSnapshot `json:"document"`
}

func ReadVDOCS(ctx context.Context, r io.Reader) (*Document, error) {
	return readVDOCS(ctx, r)
}

func WriteVDOCS(ctx context.Context, w io.Writer, d *Document) error {
	return Write(ctx, w, d, VDOCS, WriteOptions{})
}

func readVDOCS(ctx context.Context, r io.Reader) (*Document, error) {
	if r == nil {
		return nil, fmt.Errorf("nil native document reader")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	data, err := io.ReadAll(contextReader{ctx, r})
	if err != nil {
		return nil, err
	}
	if err = validateVDOCSJSON(ctx, data); err != nil {
		return nil, fmt.Errorf("native JSON: %w", err)
	}
	var wire vdocsEnvelope
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err = dec.Decode(&wire); err != nil {
		return nil, fmt.Errorf("native envelope: %w", err)
	}
	if wire.Type != vdocsEnvelopeType || wire.Version != 3 || wire.Engine != "univer" || wire.EngineVersion != vdocsEngineVersion {
		return nil, fmt.Errorf("%w: native envelope identity/version/engine", ErrUnsupported)
	}
	if wire.Document.ID == "" || wire.Document.Body.TextRuns == nil || !bytes.HasPrefix(bytes.TrimSpace(wire.Document.DocumentStyle), []byte("{")) {
		return nil, fmt.Errorf("native snapshot requires id, textRuns array and documentStyle object")
	}
	if err = checkVDOCSResourceJSON(ctx, wire.Document); err != nil {
		return nil, err
	}
	var settings struct {
		Zoom *float64 `json:"zoomRatio,omitempty"`
	}
	if len(wire.Document.Settings) > 0 {
		if err = strictSFAttrs(wire.Document.Settings, &settings); err != nil {
			return nil, fmt.Errorf("native settings: %w", err)
		}
		if settings.Zoom != nil && (*settings.Zoom <= 0 || math.IsInf(*settings.Zoom, 0) || math.IsNaN(*settings.Zoom)) {
			return nil, fmt.Errorf("native zoomRatio must be finite and positive")
		}
	}
	doc, err := decodeVDOCSDocument(ctx, wire.Document)
	if err != nil {
		return nil, err
	}
	if settings.Zoom != nil {
		doc.warn("vdocs_view_zoom", "Native editor view zoom is not represented in logical SF; document geometry and content are retained")
	}
	if wire.Document.Locale != "" && wire.Document.Locale != "enUS" {
		doc.warn("vdocs_editor_locale", "Native editor locale is not represented in logical SF; explicit source language and content are retained")
	}
	return doc, nil
}
func writeVDOCS(ctx context.Context, w io.Writer, d *Document) error {
	if w == nil {
		return fmt.Errorf("nil native document writer")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	native, err := encodeVDOCSDocument(ctx, d)
	if err != nil {
		return err
	}
	native.Locale = "enUS"
	wire := vdocsEnvelope{vdocsEnvelopeType, 3, "univer", vdocsEngineVersion, native}
	return json.NewEncoder(contextWriter{ctx, w}).Encode(wire)
}

func checkVDOCSResourceJSON(ctx context.Context, native vdocsNoteSnapshot) error {
	records := append([]vdocsResource(nil), native.Resources...)
	for _, r := range native.Body.CustomRanges {
		if len(r.Properties) == 0 {
			continue
		}
		var hint struct {
			Profile string `json:"profile"`
		}
		if err := json.Unmarshal(r.Properties, &hint); err != nil {
			return err
		}
		if hint.Profile == vdocsIntentProfile {
			var carrier vdocsIntentCarrier
			if err := json.Unmarshal(r.Properties, &carrier); err != nil {
				return err
			}
			expanded, err := carrier.records(ctx)
			if err != nil {
				return err
			}
			records = append(records, expanded...)
		}
	}
	for _, r := range records {
		if vdocsOwnIntent(r.Name) || (r.Name == "DOC_DRAWING_PLUGIN" || r.Name == vdocsHyperlinkResource) {
			if err := validateStrictJSON(ctx, []byte(r.Data)); err != nil {
				return fmt.Errorf("native resource JSON: %w", err)
			}
		}
	}
	return ctx.Err()
}

type vdocsDocumentEvidence struct {
	FlowOrigins    []vdocsOrigin     `json:"flowOrigins,omitempty"`
	AnchorOrigins  []vdocsOrigin     `json:"anchorOrigins,omitempty"`
	ListOrigins    []vdocsOrigin     `json:"listOrigins,omitempty"`
	ImageOrigins   []vdocsOrigin     `json:"imageOrigins,omitempty"`
	TableOrigins   []vdocsOrigin     `json:"tableOrigins,omitempty"`
	RunOrigins     []vdocsOrigin     `json:"runOrigins,omitempty"`
	NoteOrigins    []vdocsNoteOrigin `json:"noteOrigins,omitempty"`
	Origins        []vdocsOrigin     `json:"origins,omitempty"`
	UnboundSources []SourceInfo      `json:"unboundSources,omitempty"`
	Version        int               `json:"version"`
	Diagnostics    []Diagnostic      `json:"diagnostics,omitempty"`
	PageCount      int               `json:"pageCount,omitempty"`
	Pages          []PDFPage         `json:"pages,omitempty"`
	PDFNavigation  *PDFNavigation    `json:"pdfNavigation,omitempty"`
}

func vdocsEvidence(d *Document) (*vdocsDocumentEvidence, error) {
	if len(d.UnboundSources) == 0 && len(d.Diagnostics) == 0 && d.Metadata.PageCount == 0 && len(d.Pages) == 0 && d.PDFNavigation == nil {
		return nil, nil
	}
	e := &vdocsDocumentEvidence{Version: 1, UnboundSources: cloneUnboundSources(d.UnboundSources), Diagnostics: append([]Diagnostic(nil), d.Diagnostics...), PageCount: d.Metadata.PageCount, Pages: clonePDFPages(d.Pages), PDFNavigation: clonePDFNavigation(d.PDFNavigation)}
	if err := e.validate(); err != nil {
		return nil, err
	}
	return e, nil
}
func (e *vdocsDocumentEvidence) validate() error {
	if e == nil {
		return nil
	}
	if e.Version != 1 {
		return fmt.Errorf("%w: native document evidence version", ErrUnsupported)
	}
	for _, d := range e.Diagnostics {
		if err := validateString(d.Code); err != nil {
			return err
		}
		if err := validateString(d.Message); err != nil {
			return err
		}
	}
	if err := validatePDFPages(e.Pages); err != nil {
		return err
	}
	for i := range e.UnboundSources {
		if err := e.UnboundSources[i].validate(e.Pages); err != nil {
			return err
		}
	}
	for _, o := range e.FlowOrigins {
		if err := validateVDOCSOriginProfile(o, e.Pages, vdocsFlowOriginProfile); err != nil {
			return err
		}
	}
	for _, o := range e.AnchorOrigins {
		if err := validateVDOCSOriginProfile(o, e.Pages, vdocsAnchorOriginProfile); err != nil {
			return err
		}
	}
	for _, o := range e.ListOrigins {
		if err := validateVDOCSListOrigin(o, e.Pages); err != nil {
			return err
		}
	}
	for _, o := range e.ImageOrigins {
		if err := validateVDOCSOriginProfile(o, e.Pages, vdocsImageOriginProfile); err != nil {
			return err
		}
	}
	for _, o := range e.TableOrigins {
		if err := validateVDOCSTableOrigin(o, e.Pages); err != nil {
			return err
		}
	}
	for _, o := range e.RunOrigins {
		if err := validateVDOCSOriginProfile(o, e.Pages, vdocsRunOriginProfile); err != nil {
			return err
		}
	}
	seenNotes := map[string]bool{}
	for _, o := range e.NoteOrigins {
		if err := o.validate(e.Pages); err != nil {
			return err
		}
		if seenNotes[o.NativeID] {
			return fmt.Errorf("duplicate native note source record")
		}
		seenNotes[o.NativeID] = true
	}
	for _, o := range e.Origins {
		if err := validateVDOCSOrigin(o, e.Pages); err != nil {
			return err
		}
	}
	return e.PDFNavigation.validate(&Document{Pages: e.Pages}, nil)
}
func restoreVDOCSEvidence(d *Document, e *vdocsDocumentEvidence) error {
	if e == nil {
		return nil
	}
	if err := e.validate(); err != nil {
		return err
	}
	d.UnboundSources = cloneUnboundSources(e.UnboundSources)
	d.Metadata.PageCount = e.PageCount
	d.Pages = clonePDFPages(e.Pages)
	d.PDFNavigation = clonePDFNavigation(e.PDFNavigation)

	history := append([]Diagnostic(nil), e.Diagnostics...)
	seen := map[Diagnostic]bool{}
	for _, v := range history {
		seen[v] = true
	}
	for _, v := range d.Diagnostics {
		if !seen[v] {
			history = append(history, v)
			seen[v] = true
		}
	}
	d.Diagnostics = history
	if d.PDFNavigation == nil {
		return nil
	}
	graph, err := catalogDestinations(d, false)
	if err != nil {
		return err
	}
	for i := range d.PDFNavigation.Destinations {
		view := &d.PDFNavigation.Destinations[i]
		detached := !view.Unresolved && !graph.anchors[view.ID] || view.Unresolved && !graph.missing[view.ID]
		if detached && view.FlowBinding != "unbound" {
			view.FlowBinding = "unbound"
			warning := Diagnostic{Code: "vdocs_pdf_flow_binding", Message: "Original PDF navigation evidence for " + view.ID + " is retained unbound after the editable destination graph changed"}
			if !seen[warning] {
				d.Diagnostics = append(d.Diagnostics, warning)
				seen[warning] = true
			}
		}
	}
	return nil
}

const vdocsIntentProfile = "manydoc.native-intent"
const vdocsIntentRangeID = "manydoc-intent-v1"

type vdocsIntentCarrier struct {
	Profile      string          `json:"profile"`
	Version      int             `json:"version"`
	Records      []vdocsResource `json:"records,omitempty"`
	GzipRecords  []byte          `json:"gzipRecords,omitempty"`
	RecordsBytes int             `json:"recordsBytes,omitempty"`
}

func vdocsOwnIntent(name string) bool {
	switch name {
	case vdocsNoteIdentityResource, vdocsListResource, vdocsImageResource, vdocsDocumentResource:
		return true
	}
	return false
}

func sealVDOCSIntent(snapshot *vdocsNoteSnapshot) error {
	var records, remaining []vdocsResource
	for _, resource := range snapshot.Resources {
		if vdocsOwnIntent(resource.Name) {
			records = append(records, resource)
		} else {
			remaining = append(remaining, resource)
		}
	}
	if len(records) == 0 {
		return nil
	}
	position := vdocsUTF16Len(snapshot.Body.DataStream) - 1
	if position < 0 || !strings.HasSuffix(snapshot.Body.DataStream, "\r\n") {
		return fmt.Errorf("native intent carrier requires final paragraph/section terminators")
	}
	for _, a := range snapshot.Body.CustomRanges {
		if a.ID == vdocsIntentRangeID {
			return fmt.Errorf("native intent carrier identity collision")
		}
	}
	carrier, err := makeVDOCSIntentCarrier(records)
	if err != nil {
		return err
	}
	snapshot.Body.CustomRanges = append(snapshot.Body.CustomRanges, vdocsRangeRecord{StartIndex: position, EndIndex: position, ID: vdocsIntentRangeID, Type: 5, Properties: sfAttrs(carrier)})
	snapshot.Resources = remaining
	return nil
}
func openVDOCSIntent(ctx context.Context, snapshot vdocsNoteSnapshot) (vdocsNoteSnapshot, error) {

	snapshot.Resources = append([]vdocsResource(nil), snapshot.Resources...)
	var ranges []vdocsRangeRecord
	found := false
	for _, a := range snapshot.Body.CustomRanges {
		var marker struct {
			Profile string `json:"profile"`
		}
		_ = json.Unmarshal(a.Properties, &marker)
		if a.Type != 5 || marker.Profile != vdocsIntentProfile {
			ranges = append(ranges, a)
			continue
		}
		if found {
			return snapshot, fmt.Errorf("duplicate native intent carrier")
		}
		found = true
		var carrier vdocsIntentCarrier
		if err := strictSFAttrs(a.Properties, &carrier); err != nil {
			return snapshot, err
		}
		records, err := carrier.records(ctx)
		if err != nil {
			return snapshot, err
		}
		index, err := newVDOCSTextIndex(ctx, snapshot.Body.DataStream)
		if err != nil {
			return snapshot, err
		}

		if _, err = index.inclusive(a.StartIndex, a.EndIndex); err != nil {
			return snapshot, err
		}
		for _, record := range records {
			if !vdocsOwnIntent(record.Name) {
				return snapshot, fmt.Errorf("unknown record in native intent carrier %q", record.Name)
			}
			snapshot.Resources = append(snapshot.Resources, record)
		}
	}
	snapshot.Body.CustomRanges = ranges
	return snapshot, nil
}
func emptyVDOCSBuiltinResource(resource vdocsResource) bool {
	switch resource.Name {
	case "DOC_OBJECT_PERMISSION_PLUGIN", "DOC_WORD_STYLES_PLUGIN", "DOC_NOTE_PLUGIN":
		value := strings.TrimSpace(resource.Data)
		return value == "" || value == "{}" || value == "null"
	}
	return false
}

func checkVDOCSDrawingMirror(snapshot vdocsNoteSnapshot, resource vdocsResource) error {
	var mirror struct {
		Data  map[string]json.RawMessage `json:"data"`
		Order []string                   `json:"order"`
	}
	if err := strictSFAttrs(json.RawMessage(resource.Data), &mirror); err != nil {
		return err
	}
	sources := map[string]json.RawMessage{}
	add := func(drawings map[string]json.RawMessage) error {
		for id, raw := range drawings {
			if _, ok := sources[id]; ok {
				return fmt.Errorf("ambiguous native drawing ID across stories")
			}
			sources[id] = raw
		}
		return nil
	}
	if err := add(snapshot.Drawings); err != nil {
		return err
	}
	for _, note := range snapshot.Notes {
		if err := add(note.Drawings); err != nil {
			return err
		}
	}
	for _, story := range snapshot.Headers {
		if err := add(story.Drawings); err != nil {
			return err
		}
	}
	for _, story := range snapshot.Footers {
		if err := add(story.Drawings); err != nil {
			return err
		}
	}
	for id, raw := range mirror.Data {
		source, ok := sources[id]
		if !ok {
			return fmt.Errorf("native drawing resource lacks persisted source %q", id)
		}
		var a, b any
		if err := json.Unmarshal(raw, &a); err != nil {
			return err
		}
		if err := json.Unmarshal(source, &b); err != nil {
			return err
		}
		if !reflect.DeepEqual(a, b) {
			return fmt.Errorf("contradictory native drawing resource/source %q", id)
		}
	}
	seen := map[string]bool{}
	for _, id := range mirror.Order {
		if seen[id] {
			return fmt.Errorf("duplicate native drawing resource order")
		}
		if _, ok := mirror.Data[id]; !ok {
			return fmt.Errorf("native drawing resource order lacks data")
		}
		seen[id] = true
	}
	return nil
}

const maxVDOCSIntentBytes = 128 << 20

func makeVDOCSIntentCarrier(records []vdocsResource) (vdocsIntentCarrier, error) {
	out := vdocsIntentCarrier{Profile: vdocsIntentProfile, Version: 1, Records: records}
	raw, err := json.Marshal(records)
	if err != nil {
		return out, err
	}
	if len(raw) > maxVDOCSIntentBytes {
		return out, fmt.Errorf("%w: native intent exceeds parser expansion bound", ErrUnsupported)
	}
	if len(raw) < 64<<10 {
		return out, nil
	}
	var buf bytes.Buffer
	z := gzip.NewWriter(&buf)
	if _, err = z.Write(raw); err != nil {
		return out, err
	}
	if err = z.Close(); err != nil {
		return out, err
	}

	if ((buf.Len()+2)/3)*4 >= len(raw) {
		return out, nil
	}
	out.Version, out.Records, out.GzipRecords, out.RecordsBytes = 2, nil, buf.Bytes(), len(raw)
	return out, nil
}

func (c vdocsIntentCarrier) records(ctx context.Context) ([]vdocsResource, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if c.Profile != vdocsIntentProfile {
		return nil, fmt.Errorf("invalid native intent profile")
	}
	switch c.Version {
	case 1:
		if c.GzipRecords != nil || c.RecordsBytes != 0 {
			return nil, fmt.Errorf("mixed native intent encodings")
		}
		return c.Records, nil
	case 2:
		if c.Records != nil || len(c.GzipRecords) == 0 || c.RecordsBytes <= 0 || c.RecordsBytes > maxVDOCSIntentBytes {
			return nil, fmt.Errorf("invalid compressed native intent size/encoding")
		}
	default:
		return nil, fmt.Errorf("%w: native intent version%d", ErrUnsupported, c.Version)
	}
	source := bytes.NewReader(c.GzipRecords)
	z, err := gzip.NewReader(source)
	if err != nil {
		return nil, err
	}
	defer z.Close()
	z.Multistream(false)
	raw, err := io.ReadAll(io.LimitReader(contextReader{ctx, z}, int64(c.RecordsBytes)+1))
	if err != nil {
		return nil, err
	}
	if len(raw) != c.RecordsBytes || source.Len() != 0 {
		return nil, fmt.Errorf("compressed native intent length/trailing data mismatch")
	}
	var records []vdocsResource
	if err = validateTypedJSON(ctx, raw, reflect.TypeFor[[]vdocsResource]()); err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&records); err != nil {
		return nil, err
	}
	if records == nil {
		return nil, fmt.Errorf("compressed native intent requires record array")
	}
	return records, nil
}

type vdocsTextIndex struct {
	text   string
	byteAt []int
}

func newVDOCSTextIndex(ctx context.Context, text string) (vdocsTextIndex, error) {
	if err := ctx.Err(); err != nil {
		return vdocsTextIndex{}, err
	}
	if !utf8.ValidString(text) {
		return vdocsTextIndex{}, fmt.Errorf("native document text is not UTF-8")
	}
	index := vdocsTextIndex{text: text, byteAt: make([]int, 0, len(text)+1)}
	count := 0
	for offset, ch := range text {
		if count%1024 == 0 {
			if err := ctx.Err(); err != nil {
				return vdocsTextIndex{}, err
			}
		}
		count++
		index.byteAt = append(index.byteAt, offset)
		if ch > 0xffff {
			index.byteAt = append(index.byteAt, -1)
		}
	}
	index.byteAt = append(index.byteAt, len(text))
	return index, nil
}
func (s vdocsTextIndex) length() int { return len(s.byteAt) - 1 }
func (s vdocsTextIndex) byteBoundary(offset int) (int, error) {
	if offset < 0 || offset >= len(s.byteAt) || s.byteAt[offset] < 0 {
		return 0, fmt.Errorf("native UTF-16 offset %d is outside a scalar boundary", offset)
	}
	return s.byteAt[offset], nil
}
func (s vdocsTextIndex) exclusive(start, end int) (string, error) {
	if end < start {
		return "", fmt.Errorf("native range end precedes start")
	}
	a, err := s.byteBoundary(start)
	if err != nil {
		return "", err
	}
	b, err := s.byteBoundary(end)
	if err != nil {
		return "", err
	}
	return s.text[a:b], nil
}
func (s vdocsTextIndex) inclusive(start, last int) (string, error) {
	if last < start || last < 0 || last >= s.length() {
		return "", fmt.Errorf("invalid inclusive native range")
	}
	return s.exclusive(start, last+1)
}

const (
	vdocsParagraph        = '\r'
	vdocsSection          = '\n'
	vdocsTableStart       = '\x1a'
	vdocsRowStart         = '\x1b'
	vdocsCellStart        = '\x1c'
	vdocsCellEnd          = '\x1d'
	vdocsRowEnd           = '\x0e'
	vdocsTableEnd         = '\x0f'
	vdocsCustomRangeStart = '\x1f'
	vdocsCustomRangeEnd   = '\x1e'
	vdocsCustomBlock      = '\b'
)

type vdocsStreamNode struct {
	kind       string
	start, end int
	children   []vdocsStreamNode
}

func parseVDOCSStream(ctx context.Context, text string) ([]vdocsStreamNode, error) {
	index, err := newVDOCSTextIndex(ctx, text)
	if err != nil {
		return nil, err
	}
	return parseVDOCSIndexedStream(ctx, index)
}
func parseVDOCSIndexedStream(ctx context.Context, index vdocsTextIndex) ([]vdocsStreamNode, error) {
	p := vdocsStreamParser{index: index, ctx: ctx}
	nodes, err := p.sections(false, 0)
	if err != nil {
		return nil, err
	}
	if p.offset != index.length() || p.ranges != 0 || len(nodes) == 0 {
		return nil, fmt.Errorf("unconsumed native document stream")
	}
	return nodes, nil
}

type vdocsStreamParser struct {
	ctx    context.Context
	index  vdocsTextIndex
	offset int
	ranges int
}

func (p *vdocsStreamParser) peek() rune {
	b := p.index.byteAt[p.offset]
	if b == len(p.index.text) {
		return -1
	}
	ch, _ := utf8.DecodeRuneInString(p.index.text[b:])
	return ch
}
func (p *vdocsStreamParser) advance() {
	ch := p.peek()
	if ch > 0xffff {
		p.offset += 2
	} else {
		p.offset++
	}
}
func (p *vdocsStreamParser) sections(cell bool, depth int) ([]vdocsStreamNode, error) {
	if depth > 64 {
		return nil, fmt.Errorf("native stream nesting exceeds 64")
	}
	var sections, paragraphs []vdocsStreamNode
	sectionStart, paragraphStart := p.offset, p.offset
	var tables []vdocsStreamNode
	for p.offset < p.index.length() {
		if err := p.ctx.Err(); err != nil {
			return nil, err
		}
		ch := p.peek()
		switch ch {
		case vdocsCellEnd:
			if !cell || p.offset != sectionStart || len(paragraphs) > 0 || len(tables) > 0 {
				return nil, fmt.Errorf("native cell closes before its section terminator")
			}
			return sections, nil
		case vdocsTableStart:
			table, err := p.table(depth + 1)
			if err != nil {
				return nil, err
			}
			tables = append(tables, table)
			continue
		case vdocsParagraph:
			p.advance()
			paragraphs = append(paragraphs, vdocsStreamNode{kind: "paragraph", start: paragraphStart, end: p.offset, children: tables})
			tables = nil
			paragraphStart = p.offset
		case vdocsSection:
			if p.offset != paragraphStart || len(paragraphs) == 0 {
				return nil, fmt.Errorf("native section requires a final paragraph terminator")
			}
			p.advance()
			sections = append(sections, vdocsStreamNode{kind: "section", start: sectionStart, end: p.offset, children: paragraphs})
			paragraphs = nil
			sectionStart = p.offset
			paragraphStart = p.offset
		case vdocsCustomRangeStart:
			p.ranges++
		case vdocsCustomRangeEnd:
			p.ranges--
			if p.ranges < 0 {
				return nil, fmt.Errorf("unmatched native range end")
			}
		case vdocsRowStart, vdocsRowEnd, vdocsCellStart, vdocsTableEnd:
			return nil, fmt.Errorf("unexpected native table control U+%04X", ch)
		case 0, '\x10', '\x11', '\x12', '\x13', '\x14', '\x15':
			return nil, fmt.Errorf("%w: native stream control U+%04X needs its own representation", ErrUnsupported, ch)
		default:
			if ch < 32 && ch != '\t' && ch != '\b' && ch != '\f' && ch != '\v' {
				return nil, fmt.Errorf("unsupported native stream control U+%04X", ch)
			}
		}

		if ch != vdocsParagraph && ch != vdocsSection {
			p.advance()
		}
	}
	if cell || sectionStart != p.offset || len(paragraphs) > 0 || len(tables) > 0 {
		return nil, fmt.Errorf("native stream lacks a complete section/cell terminator")
	}
	return sections, nil
}
func (p *vdocsStreamParser) table(depth int) (vdocsStreamNode, error) {
	if depth > 64 {
		return vdocsStreamNode{}, fmt.Errorf("native stream nesting exceeds 64")
	}
	table := vdocsStreamNode{kind: "table", start: p.offset}
	p.advance()
	for p.offset < p.index.length() && p.peek() == vdocsRowStart {
		row := vdocsStreamNode{kind: "row", start: p.offset}
		p.advance()
		for p.offset < p.index.length() && p.peek() == vdocsCellStart {
			cell := vdocsStreamNode{kind: "cell", start: p.offset}
			p.advance()
			children, err := p.sections(true, depth+1)
			if err != nil {
				return table, err
			}
			if len(children) == 0 || p.offset >= p.index.length() || p.peek() != vdocsCellEnd {
				return table, fmt.Errorf("native table cell is missing content/closing token")
			}
			p.advance()
			cell.end = p.offset
			cell.children = children
			row.children = append(row.children, cell)
		}
		if len(row.children) == 0 || p.offset >= p.index.length() || p.peek() != vdocsRowEnd {
			return table, fmt.Errorf("native table row is missing cells/closing token")
		}
		p.advance()
		row.end = p.offset
		table.children = append(table.children, row)
	}
	if len(table.children) == 0 || p.offset >= p.index.length() || p.peek() != vdocsTableEnd {
		return table, fmt.Errorf("native table is missing rows/closing token")
	}
	p.advance()
	table.end = p.offset
	return table, nil
}
