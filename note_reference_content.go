package manydoc

import (
	"encoding/json"
	"fmt"
)

func validateCitationContent(ref *NoteReference, pages []PDFPage) error {
	if ref.Label != nil || len(ref.LabelContent) == 0 {
		return fmt.Errorf("rich citation requires one nonempty text authority")
	}
	for _, r := range ref.LabelContent {
		if r.Text == "" || r.Note != nil || r.Anchor != "" || r.Link != "" || r.Destination != "" {
			return fmt.Errorf("rich citation requires text/style/source runs; its note owns navigation")
		}
		if err := validateString(r.Text); err != nil {
			return err
		}
		if err := validateTextStyle(r); err != nil {
			return err
		}
		if err := r.Source.validate(pages); err != nil {
			return err
		}
	}
	return validateNoteLabel(noteLabelContentText(ref.LabelContent))
}

// Atom-wide positive marks and unspecified text properties are materialized in
// the literal fragments. This is the canonical flat SF form, not a second style
// authority stored in Word metadata. Caller-owned runs and references are copied.
func canonicalCitationRun(r Run) Run {
	if r.Note == nil || r.Note.LabelContent == nil {
		return r
	}
	ref := *r.Note
	ref.LabelContent = append([]Run(nil), ref.LabelContent...)
	for i := range ref.LabelContent {
		c := &ref.LabelContent[i]
		c.Bold = c.Bold || r.Bold
		c.Italic = c.Italic || r.Italic
		c.Underline = c.Underline || r.Underline
		c.Strike = c.Strike || r.Strike
		c.ComplexScript = c.ComplexScript || r.ComplexScript
		for _, p := range [][2]*string{{&c.FontFamily, &r.FontFamily}, {&c.Color, &r.Color}, {&c.Language, &r.Language}, {&c.EastAsiaLanguage, &r.EastAsiaLanguage}, {&c.ComplexLanguage, &r.ComplexLanguage}, {&c.Direction, &r.Direction}} {
			if *p[0] == "" {
				*p[0] = *p[1]
			}
		}
		if c.FontSize == 0 {
			c.FontSize = r.FontSize
		}
	}
	return Run{Text: r.Text, Link: r.Link, Destination: r.Destination, Anchor: r.Anchor, Source: r.Source, Note: &ref}
}

type sfNoteReference struct {
	LabelContent []StructuredNode `json:"label_content,omitempty"`
	Label        *string          `json:"label,omitempty"`
	Inferred     bool             `json:"inferred,omitempty"`
	ID           string           `json:"id"`
	Kind         string           `json:"kind"`
}

func encodeSFNoteReference(ref *NoteReference) (json.RawMessage, error) {
	value := sfNoteReference{Label: ref.Label, Inferred: ref.Inferred, ID: ref.ID, Kind: ref.Kind}
	if ref.LabelContent != nil {
		nodes, err := encodeSFBlocks([]Block{{Kind: Paragraph, Runs: ref.LabelContent}})
		if err != nil {
			return nil, err
		}
		value.LabelContent = nodes[0].Content
	}
	return sfAttrs(value), nil
}
func decodeSFNoteReference(raw json.RawMessage, depth int) (*NoteReference, error) {
	var value sfNoteReference
	if err := strictSFAttrs(raw, &value); err != nil {
		return nil, err
	}
	ref := &NoteReference{ID: value.ID, Kind: value.Kind, Inferred: value.Inferred, Label: value.Label}
	if value.LabelContent != nil {
		if len(value.LabelContent) == 0 {
			return nil, fmt.Errorf("empty rich citation")
		}
		for _, node := range value.LabelContent {
			if node.Type != "text" {
				return nil, fmt.Errorf("rich citation requires text nodes")
			}
		}
		bs, err := decodeSFBlocks([]StructuredNode{{Type: "paragraph", Content: value.LabelContent}}, depth+1)
		if err != nil {
			return nil, err
		}
		ref.LabelContent = bs[0].Runs
	}
	return ref, nil
}
