package manydoc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
)

const colleafEnvelopeType = "vibedada-document"
const colleafEngineVersion = "1.0.4"

type colleafEnvelope struct {
	Type          string              `json:"type"`
	Version       int                 `json:"version"`
	Engine        string              `json:"engine"`
	EngineVersion string              `json:"engineVersion"`
	Document      colleafNoteSnapshot `json:"document"`
}

// Private until remaining native semantics and interaction are qualified. This
// reads the actual .vdocs envelope, never a renamed SF or bare Univer snapshot.
func readColleaf(ctx context.Context, r io.Reader) (*Document, error) {
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
	if err = validateColleafJSON(ctx, data); err != nil {
		return nil, fmt.Errorf("native JSON: %w", err)
	}
	var wire colleafEnvelope
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err = dec.Decode(&wire); err != nil {
		return nil, fmt.Errorf("native envelope: %w", err)
	}
	if wire.Type != colleafEnvelopeType || wire.Version != 3 || wire.Engine != "univer" || wire.EngineVersion != colleafEngineVersion {
		return nil, fmt.Errorf("%w: native envelope identity/version/engine", ErrUnsupported)
	}
	if wire.Document.ID == "" || wire.Document.Body.TextRuns == nil || !bytes.HasPrefix(bytes.TrimSpace(wire.Document.DocumentStyle), []byte("{")) {
		return nil, fmt.Errorf("native snapshot requires id, textRuns array and documentStyle object")
	}
	if err = checkColleafResourceJSON(ctx, wire.Document); err != nil {
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
	doc, err := decodeColleafDocument(ctx, wire.Document)
	if err != nil {
		return nil, err
	}
	if settings.Zoom != nil {
		doc.warn("colleaf_view_zoom", "Native editor view zoom is not represented in logical SF; document geometry and content are retained")
	}
	if wire.Document.Locale != "" && wire.Document.Locale != "enUS" {
		doc.warn("colleaf_editor_locale", "Native editor locale is not represented in logical SF; explicit source language and content are retained")
	}
	return doc, nil
}
func writeColleaf(ctx context.Context, w io.Writer, d *Document) error {
	if w == nil {
		return fmt.Errorf("nil native document writer")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	native, err := encodeColleafDocument(ctx, d)
	if err != nil {
		return err
	}
	native.Locale = "enUS"
	wire := colleafEnvelope{colleafEnvelopeType, 3, "univer", colleafEngineVersion, native}
	return json.NewEncoder(contextWriter{ctx, w}).Encode(wire)
}

// Native resources encode JSON inside strings. Validate the interpreted payloads
// as well as the outer JSON before any of those maps can collapse duplicates.
func checkColleafResourceJSON(ctx context.Context, native colleafNoteSnapshot) error {
	records := append([]colleafResource(nil), native.Resources...)
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
		if hint.Profile == colleafIntentProfile {
			var carrier colleafIntentCarrier
			if err := json.Unmarshal(r.Properties, &carrier); err != nil {
				return err
			}
			records = append(records, carrier.Records...)
		}
	}
	for _, r := range records {
		if colleafOwnIntent(r.Name) || (r.Name == "DOC_DRAWING_PLUGIN" || r.Name == colleafHyperlinkResource) {
			if err := validateStrictJSON(ctx, []byte(r.Data)); err != nil {
				return fmt.Errorf("native resource JSON: %w", err)
			}
		}
	}
	return ctx.Err()
}
