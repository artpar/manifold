package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
)

type sfNodeWriter func(io.Writer) error

// Called after Document.Validate. Check every remaining representation-only
// restriction before touching output, without constructing transient SF nodes.
func preflightSFBlocks(ctx context.Context, bs []Block) error {
	for _, b := range bs {
		if err := ctx.Err(); err != nil {
			return err
		}
		if b.Kind != Heading && b.Level != 0 {
			return fmt.Errorf("%w: heading level on %s", ErrUnsupported, b.Kind)
		}
		if b.Kind != Paragraph && b.Kind != Heading && b.Kind != ListBlock && b.List != "" {
			return fmt.Errorf("%w: list attribute on %s", ErrUnsupported, b.Kind)
		}
		if b.Kind == Paragraph || b.Kind == Heading {
			for _, run := range b.Runs {
				r := canonicalCitationRun(run)
				if r.Text == "" && r.Note == nil && r.Anchor == "" && r.Source != nil {
					return fmt.Errorf("%w: empty text has no SF source owner; retain detached facts in UnboundSources", ErrUnsupported)
				}
				if r.Note != nil && r.Note.LabelContent != nil {
					if err := preflightSFBlocks(ctx, []Block{{Kind: Paragraph, Runs: r.Note.LabelContent}}); err != nil {
						return err
					}
				}
			}
		}
		if b.Section != nil {
			if err := preflightSFBlocks(ctx, b.Section.Blocks); err != nil {
				return err
			}
			for _, story := range sectionStories(b.Section) {
				if story != nil {
					if err := preflightSFBlocks(ctx, story.Blocks); err != nil {
						return err
					}
				}
			}
		}
		for _, item := range b.Items {
			if err := preflightSFBlocks(ctx, item.Blocks); err != nil {
				return err
			}
			if item.Marker != nil {
				if err := preflightSFBlocks(ctx, item.Marker.Blocks); err != nil {
					return err
				}
			}
		}
		for _, row := range b.Rows {
			for _, cell := range row.Cells {
				if err := preflightSFBlocks(ctx, cell.Blocks); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func writeSFNodeChildren(w io.Writer, n StructuredNode, has bool, children func() error) error {
	n.Content = nil
	p, err := json.Marshal(n)
	if err != nil {
		return err
	}
	if !has {
		_, err = w.Write(p)
		return err
	}
	if _, err = w.Write(p[:len(p)-1]); err != nil {
		return err
	}
	if _, err = io.WriteString(w, `,"content":[`); err != nil {
		return err
	}
	if err = children(); err != nil {
		return err
	}
	_, err = io.WriteString(w, `]}`)
	return err
}
func writeStructuredNode(w io.Writer, n StructuredNode) error {
	return writeSFNodeChildren(w, n, len(n.Content) > 0, func() error {
		for i, child := range n.Content {
			if i > 0 {
				if _, e := io.WriteString(w, ","); e != nil {
					return e
				}
			}
			if e := writeStructuredNode(w, child); e != nil {
				return e
			}
		}
		return nil
	})
}
func flowSFBlocks(bs []Block) []Block {
	for _, b := range bs {
		if (b.Kind == Paragraph || b.Kind == Heading) && b.List != "" {
			return legacyLists(bs)
		}
	}
	return bs
}
func writeFlowSFBlocks(w io.Writer, bs []Block) error {
	for i, b := range flowSFBlocks(bs) {
		if i > 0 {
			if _, e := io.WriteString(w, ","); e != nil {
				return e
			}
		}
		if e := writeFlowSFBlock(w, b); e != nil {
			return e
		}
	}
	return nil
}
func writeFlowSFBlock(w io.Writer, b Block) error {
	shell := b
	shell.Runs, shell.Items, shell.Rows = nil, nil, nil
	if b.Section != nil {
		section := *b.Section
		section.Blocks = nil
		shell.Section = &section
	}
	nodes, err := encodeSFBlocks([]Block{shell})
	if err != nil {
		return err
	}
	n := nodes[0]
	switch b.Kind {
	case Paragraph, Heading:
		has := false
		for _, r := range b.Runs {
			if r.Text != "" || r.Note != nil || r.Anchor != "" {
				has = true
				break
			}
		}
		return writeSFNodeChildren(w, n, has, func() error {
			first := true
			for _, run := range b.Runs {
				r := canonicalCitationRun(run)
				if r.Text == "" && r.Note == nil && r.Anchor == "" {
					continue
				}
				if !first {
					if _, e := io.WriteString(w, ","); e != nil {
						return e
					}
				}
				first = false
				if e := writeFlowSFRun(w, r); e != nil {
					return e
				}
			}
			return nil
		})
	case Table:
		return writeSFNodeChildren(w, n, len(b.Rows) > 0, func() error {
			for i, row := range b.Rows {
				if i > 0 {
					if _, e := io.WriteString(w, ","); e != nil {
						return e
					}
				}
				rn := StructuredNode{Type: "table_row"}
				if row.RepeatHeader {
					rn.Attrs = sfAttrs(struct {
						RepeatHeader bool `json:"repeatHeader"`
					}{true})
				}
				if e := writeSFNodeChildren(w, rn, len(row.Cells) > 0, func() error {
					for j, c := range row.Cells {
						if j > 0 {
							if _, e := io.WriteString(w, ","); e != nil {
								return e
							}
						}
						attrs := sfAttrs(struct {
							ColSpan int `json:"colspan"`
							RowSpan int `json:"rowspan"`
						}{max(1, c.ColSpan), max(1, c.RowSpan)})
						cn := StructuredNode{Type: "table_cell", Attrs: encodeSourceAttrs(attrs, c.Source)}
						if e := writeSFNodeChildren(w, cn, len(c.Blocks) > 0, func() error { return writeFlowSFBlocks(w, c.Blocks) }); e != nil {
							return e
						}
					}
					return nil
				}); e != nil {
					return e
				}
			}
			return nil
		})
	case ListBlock:
		return writeSFNodeChildren(w, n, len(b.Items) > 0, func() error {
			for i, item := range b.Items {
				if i > 0 {
					if _, e := io.WriteString(w, ","); e != nil {
						return e
					}
				}
				in := StructuredNode{Type: "list_item", Attrs: encodeSourceAttrs(nil, item.Source)}
				if e := writeSFNodeChildren(w, in, len(item.Blocks) > 0 || item.Marker != nil, func() error {
					if item.Marker != nil {
						mn := StructuredNode{Type: "list_marker", Attrs: encodeSourceAttrs(nil, item.Marker.Source)}
						if e := writeSFNodeChildren(w, mn, len(item.Marker.Blocks) > 0, func() error { return writeFlowSFBlocks(w, item.Marker.Blocks) }); e != nil {
							return e
						}
						if len(item.Blocks) > 0 {
							if _, e := io.WriteString(w, ","); e != nil {
								return e
							}
						}
					}
					return writeFlowSFBlocks(w, item.Blocks)
				}); e != nil {
					return e
				}
			}
			return nil
		})
	case SectionBlock:
		return writeSFNodeChildren(w, n, len(b.Section.Blocks) > 0, func() error { return writeFlowSFBlocks(w, b.Section.Blocks) })
	default:
		return writeStructuredNode(w, n)
	}
}

type sfInlineMark struct {
	Type  string `json:"type"`
	Attrs any    `json:"attrs,omitempty"`
}
type sfInlineNode struct {
	Type  string         `json:"type"`
	Attrs any            `json:"attrs,omitempty"`
	Marks []sfInlineMark `json:"marks,omitempty"`
	Text  string         `json:"text,omitempty"`
}

func writeFlowSFRun(w io.Writer, r Run) error {
	var marks [7]sfInlineMark
	n := sfInlineNode{Type: "text", Text: r.Text, Marks: marks[:0]}
	if r.Anchor != "" {
		n.Type = "anchor"
		n.Attrs = struct {
			Name string `json:"name"`
		}{r.Anchor}
	}
	if r.Note != nil {
		n.Type = "note_reference"
		attrs, err := encodeSFNoteReference(r.Note)
		if err != nil {
			return err
		}
		n.Attrs = attrs
	}
	for _, m := range []struct {
		name string
		set  bool
	}{{"strong", r.Bold}, {"em", r.Italic}, {"underline", r.Underline}, {"strike", r.Strike}} {
		if m.set {
			n.Marks = append(n.Marks, sfInlineMark{Type: m.name})
		}
	}
	if r.FontFamily != "" || r.FontSize != 0 || r.Color != "" || r.Language != "" || r.EastAsiaLanguage != "" || r.ComplexLanguage != "" || r.Direction != "" || r.ComplexScript {
		attrs := struct {
			FontFamily       string  `json:"fontFamily,omitempty"`
			FontSize         float64 `json:"fontSize,omitempty"`
			Color            string  `json:"color,omitempty"`
			Language         string  `json:"lang,omitempty"`
			EastAsiaLanguage string  `json:"eastAsiaLanguage,omitempty"`
			ComplexLanguage  string  `json:"complexLanguage,omitempty"`
			Direction        string  `json:"dir,omitempty"`
			ComplexScript    bool    `json:"complexScript,omitempty"`
		}{r.FontFamily, r.FontSize, canonicalTextColor(r.Color), r.Language, r.EastAsiaLanguage, r.ComplexLanguage, r.Direction, r.ComplexScript}
		n.Marks = append(n.Marks, sfInlineMark{"textStyle", attrs})
	}
	if r.Link != "" || r.Destination != "" {
		n.Marks = append(n.Marks, sfInlineMark{"link", struct {
			Href        string `json:"href"`
			Destination string `json:"destination,omitempty"`
		}{destinationHref(r), r.Destination}})
	}
	if r.Source != nil {
		n.Marks = append(n.Marks, sfInlineMark{"source", r.Source})
	}
	p, err := json.Marshal(n)
	if err != nil {
		return err
	}
	_, err = w.Write(p)
	return err
}
