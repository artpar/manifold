package engine

import (
	"fmt"
	"strconv"
	"strings"
)

func listStart(b Block) int {
	if b.Start != nil {
		return *b.Start
	}
	return 1
}
func listStartPointer(value int) *int {
	if value == 1 {
		return nil
	}
	return &value
}

// legacyLists converts consecutive legacy paragraph list runs to explicit list
// blocks. Explicit list blocks are never coalesced: their boundaries matter.
// It does not mutate the caller's values or backing slices.
func legacyLists(bs []Block) []Block {
	var result []Block
	for _, b := range bs {
		if (b.Kind == Paragraph || b.Kind == Heading) && b.List != "" {
			kind := b.List
			b.List = ""
			item := ListItem{Blocks: []Block{b}}
			if len(result) > 0 && result[len(result)-1].Kind == ListBlock && result[len(result)-1].docxNumberID == -1 && result[len(result)-1].List == kind {
				last := &result[len(result)-1]
				last.Items = append(last.Items, item)
			} else {
				result = append(result, Block{Kind: ListBlock, List: kind, Items: []ListItem{item}, docxNumberID: -1})
			}
		} else {
			result = append(result, b)
		}
	}
	// The marker distinguishes legacy runs only during this call.

	// Markers are ordinary visible flow content, not detached source provenance.
	// Page/section controls have no defined parallel label/body pagination semantics.

	// ListNumbering describes an ordered-list label using OOXML's %1..%9
	// placeholders. Level is zero-based. Formats contains one format per level;
	// Ancestors contains fallback values for levels before Level. Enclosing lists
	// override matching ancestor levels when serialized, so nested edits renumber.
	// Legal renders every referenced level in decimal, retaining its original format.

	for i := range result {
		if result[i].docxNumberID == -1 {
			result[i].docxNumberID = 0
		}
	}
	return result
}

func validateLiteralMarker(bs []Block, depth int) error {
	if depth > 64 {
		return fmt.Errorf("document nesting exceeds64 levels")
	}
	for _, b := range bs {
		if b.Kind == SectionBlock || b.Kind == PageBreak || b.Kind == AnchorPoint || b.PageBreakBefore {
			return fmt.Errorf("%w: literal list markers cannot contain page/section controls or standalone anchors", ErrUnsupported)
		}
		for _, row := range b.Rows {
			for _, cell := range row.Cells {
				if err := validateLiteralMarker(cell.Blocks, depth+1); err != nil {
					return err
				}
			}
		}
		for _, item := range b.Items {
			if item.Marker != nil {
				if err := validateLiteralMarker(item.Marker.Blocks, depth+1); err != nil {
					return err
				}
			}
			if err := validateLiteralMarker(item.Blocks, depth+1); err != nil {
				return err
			}
		}
	}
	return nil
}

func (d *Document) hasLiteralListMarkers() bool {
	var walk func([]Block) bool
	walk = func(bs []Block) bool {
		for _, b := range bs {
			if b.Section != nil {
				if walk(b.Section.Blocks) {
					return true
				}
				for _, story := range sectionStories(b.Section) {
					if story != nil && walk(story.Blocks) {
						return true
					}
				}
			}
			for _, item := range b.Items {
				if item.Marker != nil || walk(item.Blocks) {
					return true
				}
			}
			for _, row := range b.Rows {
				for _, cell := range row.Cells {
					if walk(cell.Blocks) {
						return true
					}
				}
			}
		}
		return false
	}
	if walk(d.Blocks) || walk(d.Headers) || walk(d.Footers) {
		return true
	}
	for _, note := range d.Notes {
		if walk(note.Blocks) {
			return true
		}
	}
	return false
}

type ListNumbering struct {
	Level       int      `json:"level"`
	Template    string   `json:"template"`
	Formats     []string `json:"formats"`
	Ancestors   []int    `json:"ancestors,omitempty"`
	Independent bool     `json:"independent,omitempty"`
	Legal       bool     `json:"legal,omitempty"`
}

func supportedNumberFormat(format string) bool {
	switch format {
	case "decimal", "decimalZero", "upperLetter", "lowerLetter", "upperRoman", "lowerRoman":
		return true
	}
	return false
}

func numberText(format string, value int) (string, error) {
	if value < 0 || value > 2147483647 || !supportedNumberFormat(format) {
		return "", fmt.Errorf("invalid numbering format/value")
	}
	if value == 0 {
		return "0", nil
	}
	switch format {
	case "decimal":
		return strconv.Itoa(value), nil
	case "decimalZero":
		if value < 10 {
			return "0" + strconv.Itoa(value), nil
		}
		return strconv.Itoa(value), nil
	case "upperLetter", "lowerLetter":
		count := (value-1)/26 + 1
		if count > 4096 {
			return "", fmt.Errorf("%w: alphabetic numbering label exceeds 4096 bytes", ErrUnsupported)
		}
		base := byte('A')
		if format == "lowerLetter" {
			base = 'a'
		}
		return strings.Repeat(string([]byte{base + byte((value-1)%26)}), count), nil
	default:
		if value/1000 > 4096 {
			return "", fmt.Errorf("%w: Roman numbering label exceeds 4096 bytes", ErrUnsupported)
		}
		var b strings.Builder
		for _, part := range []struct {
			n int
			s string
		}{{1000, "M"}, {900, "CM"}, {500, "D"}, {400, "CD"}, {100, "C"}, {90, "XC"}, {50, "L"}, {40, "XL"}, {10, "X"}, {9, "IX"}, {5, "V"}, {4, "IV"}, {1, "I"}} {
			count := value / part.n
			value %= part.n
			b.WriteString(strings.Repeat(part.s, count))
		}
		text := b.String()
		if len(text) > 4096 {
			return "", fmt.Errorf("%w: Roman numbering label exceeds 4096 bytes", ErrUnsupported)
		}
		if format == "lowerRoman" {
			text = strings.ToLower(text)
		}
		return text, nil
	}
}

func validateNumbering(n *ListNumbering) error {
	if n == nil {
		return nil
	}
	if n.Level < 0 || n.Level > 8 || len(n.Formats) != n.Level+1 || len(n.Ancestors) != n.Level || len(n.Template) > 512 {
		return fmt.Errorf("invalid numbering level/template/context")
	}
	if err := validateString(n.Template); err != nil {
		return err
	}
	for _, f := range n.Formats {
		if !supportedNumberFormat(f) {
			return fmt.Errorf("%w: numbering format %s", ErrUnsupported, f)
		}
	}
	for _, v := range n.Ancestors {
		if v < 0 || v > 2147483647 {
			return fmt.Errorf("invalid numbering ancestor")
		}
	}
	return nil
}

func numberedLabel(b Block, index int) (string, error) {
	value := listStart(b) + index
	if b.Numbering == nil {
		return strconv.Itoa(value) + ".", nil
	}
	n := b.Numbering
	if err := validateNumbering(n); err != nil {
		return "", err
	}
	var out strings.Builder
	for i := 0; i < len(n.Template); i++ {
		piece := n.Template[i : i+1]
		if n.Template[i] == '%' && i+1 < len(n.Template) && n.Template[i+1] >= '1' && n.Template[i+1] <= '9' {
			level := int(n.Template[i+1] - '1')
			i++
			if level > n.Level {
				continue
			} // OOXML ignores deeper-level placeholders.

			// Resolve nested counter contexts on owned containers; source runs, assets and
			// provenance remain shared read-only. A list's own start remains authoritative.

			v := value
			if level < n.Level {
				v = n.Ancestors[level]
			}
			format := n.Formats[level]
			if n.Legal {
				format = "decimal"
			}
			var err error
			piece, err = numberText(format, v)
			if err != nil {
				return "", err
			}
		}
		if out.Len()+len(piece) > 4096 {
			return "", fmt.Errorf("%w: numbering label exceeds 4096 bytes", ErrUnsupported)
		}
		out.WriteString(piece)
	}
	return out.String(), nil
}

func resolveNumberingDocument(d *Document) *Document {
	type counter struct {
		value  int
		format string
	}
	var walk func([]Block, map[int]counter, int) []Block
	var stories func(RunningStories) RunningStories
	stories = func(s RunningStories) RunningStories {
		clone := func(v *RunningStory) *RunningStory {
			if v == nil {
				return nil
			}
			return &RunningStory{Blocks: walk(v.Blocks, nil, 0)}
		}
		return RunningStories{Default: clone(s.Default), First: clone(s.First), Even: clone(s.Even)}
	}
	walk = func(bs []Block, parents map[int]counter, depth int) []Block {
		if bs == nil {
			return nil
		}
		out := append([]Block(nil), bs...)
		for i := range out {
			b := &out[i]
			if b.Section != nil {
				s := *b.Section
				s.Blocks = walk(s.Blocks, nil, 0)
				s.Headers = stories(s.Headers)
				s.Footers = stories(s.Footers)
				b.Section = &s
			}
			if b.Numbering != nil {
				n := *b.Numbering
				n.Formats = append([]string(nil), n.Formats...)
				n.Ancestors = append([]int(nil), n.Ancestors...)
				for level, p := range parents {
					if !n.Independent && level < n.Level {
						n.Ancestors[level] = p.value
						n.Formats[level] = p.format
					}
				}
				b.Numbering = &n
			}
			if b.Rows != nil {
				b.Rows = append([]Row(nil), b.Rows...)
			}
			for ri := range b.Rows {
				row := &b.Rows[ri]
				row.Cells = append([]Cell(nil), row.Cells...)
				for ci := range row.Cells {
					row.Cells[ci].Blocks = walk(row.Cells[ci].Blocks, parents, depth)
				}
			}
			if b.Items != nil {
				b.Items = append([]ListItem(nil), b.Items...)
			}
			for ii := range b.Items {
				next := make(map[int]counter, len(parents)+1)
				for k, v := range parents {
					next[k] = v
				}
				level, format := depth, "decimal"
				if b.Numbering != nil {
					level = b.Numbering.Level
					format = b.Numbering.Formats[level]
					for k, v := range b.Numbering.Ancestors {
						next[k] = counter{v, b.Numbering.Formats[k]}
					}
				}
				if b.List == "number" {
					for k := range next {
						if k >= level {
							delete(next, k)
						}
					}
					next[level] = counter{listStart(*b) + ii, format}
				}
				if marker := b.Items[ii].Marker; marker != nil {
					m := *marker
					m.Blocks = walk(m.Blocks, next, level+1)
					b.Items[ii].Marker = &m
				}
				b.Items[ii].Blocks = walk(b.Items[ii].Blocks, next, level+1)
			}
		}
		return out
	}
	copy := *d
	copy.Blocks = walk(d.Blocks, nil, 0)
	copy.Headers = walk(d.Headers, nil, 0)
	copy.Footers = walk(d.Footers, nil, 0)
	copy.Notes = append([]Note(nil), d.Notes...)
	for i := range copy.Notes {
		copy.Notes[i].Blocks = walk(copy.Notes[i].Blocks, nil, 0)
	}
	return &copy
}
