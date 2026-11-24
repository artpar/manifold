package manydoc

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// Wire records follow the pinned IDocumentBody schema. Text styles/properties
// remain raw until the semantic bridge resolves defaults and diagnoses losses;
// accepting their offsets alone does not claim supported style conversion.
type colleafBody struct {
	DataStream         string                   `json:"dataStream"`
	TextRuns           []colleafTextRun         `json:"textRuns"`
	Paragraphs         []colleafParagraphRecord `json:"paragraphs,omitempty"`
	SectionBreaks      []colleafSectionRecord   `json:"sectionBreaks,omitempty"`
	Tables             []colleafTableRecord     `json:"tables,omitempty"`
	CustomBlocks       []colleafBlockRecord     `json:"customBlocks,omitempty"`
	CustomRanges       []colleafRangeRecord     `json:"customRanges,omitempty"`
	RenderedPageBreaks []int                    `json:"renderedPageBreaks,omitempty"`
}
type colleafTextRun struct {
	St      int             `json:"st"`
	Ed      int             `json:"ed"`
	StyleID string          `json:"sId,omitempty"`
	TS      json.RawMessage `json:"ts,omitempty"`
}
type colleafParagraphRecord struct {
	StartIndex int             `json:"startIndex"`
	ID         string          `json:"paragraphId,omitempty"`
	StyleID    string          `json:"styleId,omitempty"`
	Style      json.RawMessage `json:"paragraphStyle,omitempty"`
	Bullet     json.RawMessage `json:"bullet,omitempty"`
}
type colleafSectionRecord struct {
	Extra map[string]json.RawMessage `json:"-"`
	colleafSectionProperties
	StartIndex int    `json:"startIndex"`
	ID         string `json:"sectionId,omitempty"`
}
type colleafTableRecord struct {
	StartIndex int    `json:"startIndex"`
	EndIndex   int    `json:"endIndex"`
	ID         string `json:"tableId"`
}
type colleafBlockRecord struct {
	StartIndex int    `json:"startIndex"`
	Type       *int   `json:"blockType,omitempty"`
	ID         string `json:"blockId"`
	NoteID     string `json:"noteId,omitempty"`
	NoteType   string `json:"noteType,omitempty"`
}
type colleafRangeRecord struct {
	Active     *bool           `json:"active,omitempty"`
	StartIndex int             `json:"startIndex"`
	EndIndex   int             `json:"endIndex"`
	ID         string          `json:"rangeId"`
	Type       int             `json:"rangeType"`
	Whole      bool            `json:"wholeEntity,omitempty"`
	Properties json.RawMessage `json:"properties,omitempty"`
}

type colleafBodyIndex struct {
	text     colleafTextIndex
	sections []colleafStreamNode
	tables   map[int]colleafTableRecord
	blocks   map[int]colleafBlockRecord
}

func indexColleafBody(ctx context.Context, body colleafBody) (*colleafBodyIndex, error) {
	text, err := newColleafTextIndex(ctx, body.DataStream)
	if err != nil {
		return nil, err
	}
	sections, err := parseColleafIndexedStream(ctx, text)
	if err != nil {
		return nil, err
	}
	index := &colleafBodyIndex{text: text, sections: sections, tables: map[int]colleafTableRecord{}, blocks: map[int]colleafBlockRecord{}}
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
		if err := token(paragraph.StartIndex, colleafParagraph); err != nil {
			return nil, err
		}
		positions[paragraph.StartIndex] = true
	}
	positions = map[int]bool{}
	for _, section := range body.SectionBreaks {
		if positions[section.StartIndex] {
			return nil, fmt.Errorf("duplicate native section position")
		}
		if err := token(section.StartIndex, colleafSection); err != nil {
			return nil, err
		}
		positions[section.StartIndex] = true
	}
	structuralTables := map[int]int{}
	var collect func([]colleafStreamNode)
	collect = func(nodes []colleafStreamNode) {
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
		if err := token(block.StartIndex, colleafCustomBlock); err != nil {
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
			index.blocks[annotation.StartIndex] = colleafBlockRecord{StartIndex: annotation.StartIndex, Type: &one, ID: annotation.ID, NoteID: props.NoteID, NoteType: kind}
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

func (s *colleafSectionRecord) UnmarshalJSON(data []byte) error {
	type alias colleafSectionRecord
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
	*s = colleafSectionRecord(value)
	return nil
}

// Keep uncaptured native section properties when transporting a parsed snapshot
// to an independent reader. Semantic import still diagnoses or rejects them.
func (s colleafSectionRecord) MarshalJSON() ([]byte, error) {
	type alias colleafSectionRecord
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
