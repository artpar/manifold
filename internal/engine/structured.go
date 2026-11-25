package engine

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

const StructuredSchema = "manydoc-prosemirror/1"

// StructuredDocument is a ProseMirror document using the versioned schema in
// docs/reference/STRUCTURED_FORMAT.md. It needs no JavaScript or ProseMirror runtime.
type StructuredDocument struct {
	Type    string               `json:"type"`
	Attrs   StructuredAttributes `json:"attrs"`
	Content []StructuredNode     `json:"content,omitempty"`
}
type StructuredAttributes struct {
	Presentation           *StructuredPresentation `json:"presentation,omitempty"`
	UnboundSources         []SourceInfo            `json:"unboundSources,omitempty"`
	PDFNavigation          *PDFNavigation          `json:"pdfNavigation,omitempty"`
	Schema                 string                  `json:"schema"`
	Metadata               Metadata                `json:"metadata,omitempty"`
	Layout                 Layout                  `json:"layout,omitempty"`
	Headers                []StructuredNode        `json:"headers,omitempty"`
	Footers                []StructuredNode        `json:"footers,omitempty"`
	Notes                  []StructuredNote        `json:"notes,omitempty"`
	Diagnostics            []Diagnostic            `json:"diagnostics,omitempty"`
	Pages                  []PDFPage               `json:"pages,omitempty"`
	EvenAndOddHeaders      bool                    `json:"evenAndOddHeaders,omitempty"`
	UnresolvedDestinations []string                `json:"unresolvedDestinations,omitempty"`
}
type StructuredNote struct {
	BodyLabelContent []StructuredNode  `json:"body_label_content,omitempty"`
	BodyLabelMarks   *[]StructuredMark `json:"body_label_marks,omitempty"`
	BodyLabel        *string           `json:"body_label,omitempty"`
	LabelSource      *SourceInfo       `json:"label_source,omitempty"`
	KindInferred     bool              `json:"kind_inferred,omitempty"`
	Source           *SourceInfo       `json:"source,omitempty"`
	Label            string            `json:"label,omitempty"`
	ID               string            `json:"id"`
	Kind             string            `json:"kind"`
	Content          []StructuredNode  `json:"content"`
}

// Attrs contains the attributes defined for this node type, encoded as an object.
// Unknown nodes, marks and attributes are rejected, never silently flattened.
type StructuredNode struct {
	Type    string           `json:"type"`
	Attrs   json.RawMessage  `json:"attrs,omitempty"`
	Content []StructuredNode `json:"content,omitempty"`
	Marks   []StructuredMark `json:"marks,omitempty"`
	Text    string           `json:"text,omitempty"`
}
type StructuredMark struct {
	Type  string          `json:"type"`
	Attrs json.RawMessage `json:"attrs,omitempty"`
}

func sfAttrs(v any) json.RawMessage { b, _ := json.Marshal(v); return b }

// ToStructured maps the declared Document semantics to ProseMirror JSON.
// Empty text runs and the equivalent zero/one column spans are normalized.
func ToStructured(d *Document) (*StructuredDocument, error) {
	if err := d.Validate(); err != nil {
		return nil, err
	}
	d = resolveNumberingDocument(d)
	s := &StructuredDocument{Type: "doc", Attrs: StructuredAttributes{
		UnboundSources: cloneUnboundSources(d.UnboundSources), PDFNavigation: clonePDFNavigation(d.PDFNavigation), Schema: StructuredSchema, Metadata: d.Metadata, Layout: cloneLayout(d.Layout), EvenAndOddHeaders: d.EvenAndOddHeaders,
		Diagnostics: append([]Diagnostic(nil), d.Diagnostics...), Pages: clonePDFPages(d.Pages),
		UnresolvedDestinations: append([]string(nil), d.UnresolvedDestinations...),
	}}
	var presErr error
	s.Attrs.Presentation, presErr = presentationToSF(d.Presentation)
	if presErr != nil {
		return nil, presErr
	}
	sort.Strings(s.Attrs.UnresolvedDestinations)
	var err error
	if s.Content, err = encodeSFBlocks(d.Blocks); err != nil {
		return nil, err
	}
	if d.Presentation != nil {
		s.Content = []StructuredNode{{Type: "presentation", Attrs: sfAttrs(struct {
			Version int `json:"version"`
		}{1})}}
	}
	if s.Attrs.Headers, err = encodeSFBlocks(d.Headers); err != nil {
		return nil, err
	}
	if s.Attrs.Footers, err = encodeSFBlocks(d.Footers); err != nil {
		return nil, err
	}
	for _, n := range d.Notes {
		bs, err := encodeSFBlocks(n.Blocks)
		if err != nil {
			return nil, err
		}
		var marks *[]StructuredMark
		if n.BodyLabelStyle != nil {
			style := *n.BodyLabelStyle
			style.Text = "label"
			encoded, err := encodeSFBlocks([]Block{{Kind: Paragraph, Runs: []Run{style}}})
			if err != nil {
				return nil, err
			}
			value := encoded[0].Content[0].Marks
			if value == nil {
				value = []StructuredMark{}
			}
			marks = &value
		}
		var rich []StructuredNode
		if n.BodyLabelContent != nil {
			encoded, err := encodeSFBlocks([]Block{{Kind: Paragraph, Runs: n.BodyLabelContent}})
			if err != nil {
				return nil, err
			}
			rich = encoded[0].Content
		}
		s.Attrs.Notes = append(s.Attrs.Notes, StructuredNote{BodyLabelContent: rich, BodyLabelMarks: marks, ID: n.ID, Kind: n.Kind, Content: bs, Label: n.Label, Source: n.Source, KindInferred: n.KindInferred, LabelSource: n.LabelSource, BodyLabel: n.BodyLabel})
	}
	if _, err := FromStructured(s); err != nil {
		return nil, err
	}
	return s, nil
}

func encodeSFBlocks(bs []Block) ([]StructuredNode, error) {
	var ns []StructuredNode
	for _, b := range legacyLists(bs) {
		if b.Kind != Heading && b.Level != 0 {
			return nil, fmt.Errorf("%w: heading level on %s", ErrUnsupported, b.Kind)
		}
		if b.Kind != Paragraph && b.Kind != Heading && b.Kind != ListBlock && b.List != "" {
			return nil, fmt.Errorf("%w: list attribute on %s", ErrUnsupported, b.Kind)
		}
		n := StructuredNode{Type: string(b.Kind)}
		switch b.Kind {
		case SectionBlock:
			var err error
			n, err = encodeSFSection(b.Section)
			if err != nil {
				return nil, err
			}
		case Paragraph, Heading:
			if b.Kind == Heading || b.PageBreakBefore || b.Direction != "" {
				level := 0
				if b.Kind == Heading {
					level = b.Level
				}
				n.Attrs = sfAttrs(struct {
					Level           int    `json:"level,omitempty"`
					PageBreakBefore bool   `json:"pageBreakBefore,omitempty"`
					Direction       string `json:"dir,omitempty"`
				}{level, b.PageBreakBefore, b.Direction})
			}
			for _, r := range b.Runs {
				r = canonicalCitationRun(r)
				if r.Text == "" && r.Note == nil && r.Anchor == "" {
					if r.Source != nil {
						return nil, fmt.Errorf("%w: empty text has no SF source owner; retain detached facts in UnboundSources", ErrUnsupported)
					}
					continue
				}
				t := StructuredNode{Type: "text", Text: r.Text}
				if r.Anchor != "" {
					t.Type = "anchor"
					t.Attrs = sfAttrs(struct {
						Name string `json:"name"`
					}{r.Anchor})
				}
				if r.Note != nil {
					t.Type = "note_reference"
					var err error
					t.Attrs, err = encodeSFNoteReference(r.Note)
					if err != nil {
						return nil, err
					}
				}
				for _, m := range []struct {
					name string
					set  bool
				}{{"strong", r.Bold}, {"em", r.Italic}, {"underline", r.Underline}, {"strike", r.Strike}} {
					if m.set {
						t.Marks = append(t.Marks, StructuredMark{Type: m.name})
					}
				}
				if r.FontFamily != "" || r.FontSize != 0 || r.Color != "" || r.Language != "" || r.EastAsiaLanguage != "" || r.ComplexLanguage != "" || r.Direction != "" || r.ComplexScript {
					t.Marks = append(t.Marks, StructuredMark{Type: "textStyle", Attrs: sfAttrs(struct {
						FontFamily       string  `json:"fontFamily,omitempty"`
						FontSize         float64 `json:"fontSize,omitempty"`
						Color            string  `json:"color,omitempty"`
						Language         string  `json:"lang,omitempty"`
						EastAsiaLanguage string  `json:"eastAsiaLanguage,omitempty"`
						ComplexLanguage  string  `json:"complexLanguage,omitempty"`
						Direction        string  `json:"dir,omitempty"`
						ComplexScript    bool    `json:"complexScript,omitempty"`
					}{r.FontFamily, r.FontSize, canonicalTextColor(r.Color), r.Language, r.EastAsiaLanguage, r.ComplexLanguage, r.Direction, r.ComplexScript})})

				}
				if r.Link != "" || r.Destination != "" {
					t.Marks = append(t.Marks, StructuredMark{Type: "link", Attrs: sfAttrs(struct {
						Href        string `json:"href"`
						Destination string `json:"destination,omitempty"`
					}{destinationHref(r), r.Destination})})
				}
				if r.Source != nil {
					t.Marks = append(t.Marks, StructuredMark{Type: "source", Attrs: sfAttrs(r.Source)})
				}
				n.Content = append(n.Content, t)
			}
		case ListBlock:
			n.Type = "bullet_list"
			if b.List == "number" {
				n.Type = "ordered_list"
				n.Attrs = sfAttrs(struct {
					Order     int            `json:"order"`
					Numbering *ListNumbering `json:"numbering,omitempty"`
				}{listStart(b), b.Numbering})
			}
			for _, item := range b.Items {
				content, err := encodeSFBlocks(item.Blocks)
				if err != nil {
					return nil, err
				}
				if item.Marker != nil {
					label, err := encodeSFBlocks(item.Marker.Blocks)
					if err != nil {
						return nil, err
					}
					content = append([]StructuredNode{{Type: "list_marker", Attrs: encodeSourceAttrs(nil, item.Marker.Source), Content: label}}, content...)
				}
				n.Content = append(n.Content, StructuredNode{Type: "list_item", Attrs: encodeSourceAttrs(nil, item.Source), Content: content})
			}
		case Table:
			if len(b.ColumnWidths) > 0 {
				n.Attrs = sfAttrs(struct {
					ColumnWidths []float64 `json:"columnWidths"`
				}{b.ColumnWidths})
			}
			for _, row := range b.Rows {
				rn := StructuredNode{Type: "table_row"}
				if row.RepeatHeader {
					rn.Attrs = sfAttrs(struct {
						RepeatHeader bool `json:"repeatHeader"`
					}{true})
				}
				for _, c := range row.Cells {
					content, err := encodeSFBlocks(c.Blocks)
					if err != nil {
						return nil, err
					}
					span := c.ColSpan
					if span == 0 {
						span = 1
					}
					rn.Content = append(rn.Content, StructuredNode{Type: "table_cell", Attrs: sfAttrs(struct {
						ColSpan int `json:"colspan"`
						RowSpan int `json:"rowspan"`
					}{span, max(1, c.RowSpan)}), Content: content})
					last := &rn.Content[len(rn.Content)-1]
					last.Attrs = encodeSourceAttrs(last.Attrs, c.Source)
				}
				n.Content = append(n.Content, rn)
			}
		case Picture:
			n.Attrs = sfAttrs(struct {
				Src    string  `json:"src"`
				Alt    string  `json:"alt"`
				Width  float64 `json:"width"`
				Height float64 `json:"height"`
			}{"data:" + b.Image.MediaType + ";base64," + base64.StdEncoding.EncodeToString(b.Image.Data), b.Image.Alt, b.Image.Width, b.Image.Height})
		case PageBreak:
		}
		n.Attrs = encodeSourceAttrs(n.Attrs, b.Source)
		if len(b.Anchors) > 0 {
			var attrs map[string]json.RawMessage
			if len(n.Attrs) > 0 {
				json.Unmarshal(n.Attrs, &attrs)
			}
			if attrs == nil {
				attrs = map[string]json.RawMessage{}
			}
			attrs["anchors"] = sfAttrs(b.Anchors)
			n.Attrs = sfAttrs(attrs)
		}
		ns = append(ns, n)
	}
	return ns, nil
}

// FromStructured checks the schema and converts without discarding unrepresented
// semantics. Valid ProseMirror constructs outside the current engine return
// ErrUnsupported; AllowLossy cannot override these errors.
func FromStructured(s *StructuredDocument) (*Document, error) {
	if s == nil || s.Type != "doc" {
		return nil, fmt.Errorf("structured document requires doc root")
	}
	if s.Attrs.Schema != StructuredSchema {
		return nil, fmt.Errorf("%w: structured schema %q", ErrUnsupported, s.Attrs.Schema)
	}
	d := &Document{UnboundSources: cloneUnboundSources(s.Attrs.UnboundSources), PDFNavigation: clonePDFNavigation(s.Attrs.PDFNavigation), EvenAndOddHeaders: s.Attrs.EvenAndOddHeaders, Metadata: s.Attrs.Metadata, Layout: cloneLayout(s.Attrs.Layout), Diagnostics: append([]Diagnostic(nil), s.Attrs.Diagnostics...), Pages: clonePDFPages(s.Attrs.Pages), UnresolvedDestinations: append([]string(nil), s.Attrs.UnresolvedDestinations...)}
	var err error
	d.Presentation, err = presentationFromSF(s.Attrs.Presentation)
	if err != nil {
		return nil, err
	}
	body := s.Content
	if d.Presentation != nil {
		if len(body) != 1 || body[0].Type != "presentation" || len(body[0].Content) > 0 || len(body[0].Marks) > 0 || body[0].Text != "" {
			return nil, fmt.Errorf("presentation SF requires its sole feature-marker node")
		}
		var marker struct {
			Version int `json:"version"`
		}
		if err := strictSFAttrs(body[0].Attrs, &marker); err != nil {
			return nil, err
		}
		if marker.Version != 1 {
			return nil, fmt.Errorf("unsupported presentation marker version")
		}
		body = nil
	}
	if d.Blocks, err = decodeSFBlocks(body, 0); err != nil {
		return nil, err
	}
	if d.Headers, err = decodeSFBlocks(s.Attrs.Headers, 0); err != nil {
		return nil, err
	}
	if d.Footers, err = decodeSFBlocks(s.Attrs.Footers, 0); err != nil {
		return nil, err
	}
	for _, n := range s.Attrs.Notes {
		if err := validateString(n.ID); err != nil {
			return nil, err
		}
		if err := validateString(n.Kind); err != nil {
			return nil, err
		}
		bs, err := decodeSFBlocks(n.Content, 0)
		if err != nil {
			return nil, err
		}
		var style *Run
		if n.BodyLabelMarks != nil {
			decoded, err := decodeSFBlocks([]StructuredNode{{Type: "paragraph", Content: []StructuredNode{{Type: "text", Text: "label", Marks: *n.BodyLabelMarks}}}}, 0)
			if err != nil {
				return nil, err
			}
			run := decoded[0].Runs[0]
			run.Text = ""
			style = &run
		}
		var rich []Run
		if n.BodyLabelContent != nil {
			if len(n.BodyLabelContent) == 0 {
				return nil, fmt.Errorf("empty rich body label")
			}
			for _, node := range n.BodyLabelContent {
				if node.Type != "text" {
					return nil, fmt.Errorf("rich body labels require text nodes")
				}
			}
			decoded, err := decodeSFBlocks([]StructuredNode{{Type: "paragraph", Content: n.BodyLabelContent}}, 0)
			if err != nil {
				return nil, err
			}
			rich = decoded[0].Runs
		}
		d.Notes = append(d.Notes, Note{BodyLabelContent: rich, BodyLabelStyle: style, ID: n.ID, Kind: n.Kind, Blocks: bs, Label: n.Label, Source: n.Source, KindInferred: n.KindInferred, LabelSource: n.LabelSource, BodyLabel: n.BodyLabel})
	}
	for _, diag := range d.Diagnostics {
		if err := validateString(diag.Code); err != nil {
			return nil, err
		}
		if err := validateString(diag.Message); err != nil {
			return nil, err
		}
	}
	if err := d.Validate(); err != nil {
		return nil, err
	}
	return d, nil
}

// strictSFAttrs also rejects null and non-object attribute values.
func strictSFAttrs(raw json.RawMessage, v any) error {
	if len(raw) == 0 {
		raw = json.RawMessage(`{}`)
	}
	if !bytes.HasPrefix(bytes.TrimSpace(raw), []byte("{")) {
		return fmt.Errorf("structured attrs must be an object")
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("structured attrs: %w", err)
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		return fmt.Errorf("structured attrs must contain one object")
	}
	return nil
}
func emptySFAttrs(raw json.RawMessage) error { return strictSFAttrs(raw, &struct{}{}) }
func sfUnsupported(typ string) error {
	return fmt.Errorf("%w: structured node/mark %q", ErrUnsupported, typ)
}

func decodeSFBlocks(ns []StructuredNode, depth int) ([]Block, error) {
	if depth > 64 {
		return nil, fmt.Errorf("structured nesting exceeds 64 levels")
	}
	var bs []Block
	for _, n := range ns {
		attrs, source, err := decodeSourceAttrs(n.Attrs)
		if err != nil {
			return nil, err
		}
		var anchors []string
		if len(attrs) > 0 {
			var common map[string]json.RawMessage
			if err := json.Unmarshal(attrs, &common); err != nil {
				return nil, err
			}
			if a, ok := common["anchors"]; ok {
				if err := json.Unmarshal(a, &anchors); err != nil {
					return nil, err
				}
				delete(common, "anchors")
				attrs = sfAttrs(common)
			}
		}
		n.Attrs = attrs
		if n.Text != "" || len(n.Marks) > 0 {
			return nil, fmt.Errorf("block %s cannot have text or marks", n.Type)
		}
		b := Block{Kind: Kind(n.Type), Source: source, Anchors: anchors}
		switch n.Type {
		case "section":
			b.Section, err = decodeSFSection(n, depth)
			if err != nil {
				return nil, err
			}
		case "paragraph", "heading":
			var paragraphAttrs map[string]json.RawMessage
			if len(n.Attrs) > 0 {
				if err := json.Unmarshal(n.Attrs, &paragraphAttrs); err != nil {
					return nil, err
				}
				if v, present := paragraphAttrs["dir"]; present && bytes.Equal(bytes.TrimSpace(v), []byte("null")) {
					return nil, fmt.Errorf("paragraph direction cannot be null")
				}
			}
			a := struct {
				Level           int    `json:"level,omitempty"`
				PageBreakBefore bool   `json:"pageBreakBefore,omitempty"`
				Direction       string `json:"dir,omitempty"`
			}{}
			if n.Type == "heading" {
				a.Level = 1
			}
			if err := strictSFAttrs(n.Attrs, &a); err != nil {
				return nil, err
			}
			if n.Type == "paragraph" {
				paragraph := struct {
					PageBreakBefore bool   `json:"pageBreakBefore,omitempty"`
					Direction       string `json:"dir,omitempty"`
				}{}
				if err := strictSFAttrs(n.Attrs, &paragraph); err != nil {
					return nil, err
				}
			}
			b.Level = a.Level
			b.PageBreakBefore = a.PageBreakBefore
			b.Direction = a.Direction
			for _, t := range n.Content {
				r := Run{}
				switch t.Type {
				case "text":
					if t.Text == "" || len(t.Content) > 0 {
						return nil, fmt.Errorf("text nodes require nonempty text and no content")
					}
					if err := emptySFAttrs(t.Attrs); err != nil {
						return nil, err
					}
					r.Text = t.Text
				case "anchor":
					if t.Text != "" || len(t.Content) > 0 {
						return nil, fmt.Errorf("anchor is an inline atom")
					}
					var a struct {
						Name string `json:"name"`
					}
					if err := strictSFAttrs(t.Attrs, &a); err != nil {
						return nil, err
					}
					if err := destinationID(a.Name); err != nil {
						return nil, err
					}
					r.Anchor = a.Name
				case "note_reference":
					if t.Text != "" || len(t.Content) > 0 {
						return nil, fmt.Errorf("note reference is an inline atom")
					}
					ref, err := decodeSFNoteReference(t.Attrs, depth)
					if err != nil {
						return nil, err
					}
					r.Note = ref
				default:
					return nil, sfUnsupported(t.Type)
				}
				seen := map[string]bool{}
				for _, m := range t.Marks {
					if seen[m.Type] {
						return nil, fmt.Errorf("duplicate mark %q", m.Type)
					}
					seen[m.Type] = true
					if m.Type != "link" && m.Type != "source" && m.Type != "textStyle" {
						if err := emptySFAttrs(m.Attrs); err != nil {
							return nil, err
						}
					}
					switch m.Type {
					case "textStyle":
						var values map[string]json.RawMessage
						if len(m.Attrs) > 0 {
							if err := json.Unmarshal(m.Attrs, &values); err != nil {
								return nil, err
							}
							for _, v := range values {
								if bytes.Equal(bytes.TrimSpace(v), []byte("null")) {
									return nil, fmt.Errorf("textStyle attributes cannot be null")
								}
							}
						}
						a := struct {
							FontFamily       string  `json:"fontFamily"`
							FontSize         float64 `json:"fontSize"`
							Color            string  `json:"color"`
							Language         string  `json:"lang"`
							EastAsiaLanguage string  `json:"eastAsiaLanguage"`
							ComplexLanguage  string  `json:"complexLanguage"`
							Direction        string  `json:"dir"`
							ComplexScript    bool    `json:"complexScript"`
						}{}
						if err := strictSFAttrs(m.Attrs, &a); err != nil {
							return nil, err
						}
						r.FontFamily, r.FontSize, r.Color = a.FontFamily, a.FontSize, a.Color
						r.Language, r.EastAsiaLanguage, r.ComplexLanguage, r.Direction = a.Language, a.EastAsiaLanguage, a.ComplexLanguage, a.Direction
						r.ComplexScript = a.ComplexScript
						if err := validateTextStyle(r); err != nil {
							return nil, err
						}
					case "source":
						src := SourceInfo{}
						if err := strictSFAttrs(m.Attrs, &src); err != nil {
							return nil, err
						}
						r.Source = &src
					case "strong":
						r.Bold = true
					case "em":
						r.Italic = true
					case "underline":
						r.Underline = true
					case "strike":
						r.Strike = true
					case "link":
						a := struct {
							Href        string `json:"href"`
							Destination string `json:"destination,omitempty"`
						}{}
						if err := strictSFAttrs(m.Attrs, &a); err != nil {
							return nil, err
						}
						if a.Href == "" {
							return nil, fmt.Errorf("link requires href")
						}
						r.Link = a.Href
						r.Destination = a.Destination
					default:
						return nil, sfUnsupported(m.Type)
					}
				}
				if err := validateTextStyle(r); err != nil {
					return nil, err
				}
				b.Runs = append(b.Runs, canonicalCitationRun(r))
			}
		case "bullet_list", "ordered_list":
			b.Kind = ListBlock
			b.List = "bullet"
			if n.Type == "ordered_list" {
				a := struct {
					Order     int            `json:"order"`
					Numbering *ListNumbering `json:"numbering,omitempty"`
				}{Order: 1}
				if err := strictSFAttrs(n.Attrs, &a); err != nil {
					return nil, err
				}
				b.List = "number"
				b.Start = listStartPointer(a.Order)
				b.Numbering = a.Numbering
			} else if err := emptySFAttrs(n.Attrs); err != nil {
				return nil, err
			}
			for _, item := range n.Content {
				if item.Type != "list_item" || item.Text != "" || len(item.Marks) > 0 {
					return nil, fmt.Errorf("list requires list_item children")
				}
				attrs, itemSource, err := decodeSourceAttrs(item.Attrs)
				if err != nil {
					return nil, err
				}
				if err := emptySFAttrs(attrs); err != nil {
					return nil, err
				}
				children := item.Content
				var marker *ListMarker
				if len(children) > 0 && children[0].Type == "list_marker" {
					label := children[0]
					markerAttrs, markerSource, e := decodeSourceAttrs(label.Attrs)
					if e != nil {
						return nil, e
					}
					if label.Text != "" || len(label.Marks) > 0 || emptySFAttrs(markerAttrs) != nil || len(label.Content) == 0 {
						return nil, fmt.Errorf("invalid literal list marker")
					}
					markerBlocks, err := decodeSFBlocks(label.Content, depth+1)
					if err != nil {
						return nil, err
					}
					marker = &ListMarker{Blocks: markerBlocks, Source: markerSource}
					children = children[1:]
				}
				blocks, err := decodeSFBlocks(children, depth+1)
				if err != nil {
					return nil, err
				}
				b.Items = append(b.Items, ListItem{Marker: marker, Blocks: blocks, Source: itemSource})
			}

		case "table":
			a := struct {
				ColumnWidths []float64 `json:"columnWidths"`
			}{}
			if err := strictSFAttrs(n.Attrs, &a); err != nil {
				return nil, err
			}
			b.ColumnWidths = append([]float64(nil), a.ColumnWidths...)
			for _, row := range n.Content {
				if row.Type != "table_row" || row.Text != "" || len(row.Marks) > 0 {
					return nil, fmt.Errorf("table requires table_row children")
				}
				a := struct {
					RepeatHeader json.RawMessage `json:"repeatHeader"`
				}{}
				if err := strictSFAttrs(row.Attrs, &a); err != nil {
					return nil, err
				}
				repeat := false
				if len(a.RepeatHeader) > 0 {
					switch string(bytes.TrimSpace(a.RepeatHeader)) {
					case "true":
						repeat = true
					case "false":
					default:
						return nil, fmt.Errorf("repeatHeader must be a boolean")
					}
				}
				r := Row{RepeatHeader: repeat}
				for _, cell := range row.Content {
					cellAttrs, cellSource, err := decodeSourceAttrs(cell.Attrs)
					if err != nil {
						return nil, err
					}
					cell.Attrs = cellAttrs
					if cell.Type != "table_cell" || cell.Text != "" || len(cell.Marks) > 0 {
						return nil, fmt.Errorf("row requires table_cell children")
					}
					a := struct {
						ColSpan int `json:"colspan"`
						RowSpan int `json:"rowspan"`
					}{1, 1}
					if err := strictSFAttrs(cell.Attrs, &a); err != nil {
						return nil, err
					}
					if a.ColSpan < 1 || a.ColSpan > 1000 {
						return nil, fmt.Errorf("invalid colspan")
					}
					if a.RowSpan < 1 {
						return nil, fmt.Errorf("invalid rowspan")
					}
					blocks, err := decodeSFBlocks(cell.Content, depth+1)
					if err != nil {
						return nil, err
					}
					rowSpan := a.RowSpan
					if rowSpan == 1 {
						rowSpan = 0
					}
					r.Cells = append(r.Cells, Cell{Blocks: blocks, ColSpan: a.ColSpan, RowSpan: rowSpan, Source: cellSource})
				}
				b.Rows = append(b.Rows, r)
			}
		case "anchor_point":
			if len(n.Content) > 0 {
				return nil, fmt.Errorf("anchor point cannot have content")
			}
			if err := emptySFAttrs(n.Attrs); err != nil {
				return nil, err
			}
		case "image":
			if len(n.Content) > 0 {
				return nil, fmt.Errorf("image cannot have content")
			}
			a := struct {
				Src    string  `json:"src"`
				Alt    string  `json:"alt"`
				Width  float64 `json:"width"`
				Height float64 `json:"height"`
			}{}
			if err := strictSFAttrs(n.Attrs, &a); err != nil {
				return nil, err
			}
			head, data, ok := strings.Cut(a.Src, ",")
			if !ok || !strings.HasPrefix(head, "data:") || !strings.HasSuffix(head, ";base64") {
				return nil, fmt.Errorf("%w: image src must be embedded base64 data URI", ErrUnsupported)
			}
			raw, err := base64.StdEncoding.Strict().DecodeString(data)
			if err != nil {
				return nil, fmt.Errorf("image data: %w", err)
			}
			b.Image = &Image{Data: raw, MediaType: strings.TrimSuffix(strings.TrimPrefix(head, "data:"), ";base64"), Alt: a.Alt, Width: a.Width, Height: a.Height}
		case "page_break":
			if err := emptySFAttrs(n.Attrs); err != nil {
				return nil, err
			}
			if len(n.Content) > 0 {
				return nil, fmt.Errorf("page break cannot have content")
			}
		default:
			return nil, sfUnsupported(n.Type)
		}
		bs = append(bs, b)
	}
	return bs, nil
}

// ReadStructured reads and validates the supported ProseMirror schema.
func ReadStructured(ctx context.Context, r io.Reader) (*StructuredDocument, error) {
	s, _, err := readStructured(ctx, r)
	return s, err
}

// Return the already-validated bridge result to Read(SF), avoiding a second
// decoding/allocation of every source mark and asset in the same operation.
func readStructured(ctx context.Context, r io.Reader) (*StructuredDocument, *Document, error) {
	if r == nil {
		return nil, nil, fmt.Errorf("nil document reader")
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	var s StructuredDocument
	dec := json.NewDecoder(contextReader{ctx, r})
	dec.DisallowUnknownFields()
	if err := dec.Decode(&s); err != nil {
		return nil, nil, fmt.Errorf("structured JSON: %w", err)
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		if err != nil {
			return nil, nil, fmt.Errorf("structured JSON trailing data: %w", err)
		}
		return nil, nil, fmt.Errorf("structured JSON must contain one value")
	}
	d, err := FromStructured(&s)
	if err != nil {
		return nil, nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	return &s, d, nil
}

// WriteStructured validates before emitting the persistent representation.
func WriteStructured(ctx context.Context, w io.Writer, s *StructuredDocument) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if w == nil {
		return fmt.Errorf("nil document writer")
	}
	if _, err := FromStructured(s); err != nil {
		return err
	}
	return encodeStructured(ctx, w, s, false)
}

// The caller has already checked the complete SF graph. Keep JSON compact:
// source-heavy documents otherwise spend most persisted bytes on indentation.

// Stream the body tree rather than buffering the JSON for every paragraph/table
// at once. Root attributes and each node's own attributes/marks remain ordinary
// JSON marshals, so assets and running/note trees are still materialized values.

func encodeStructured(ctx context.Context, w io.Writer, s *StructuredDocument, compressed bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !compressed {
		return encodeStructuredJSON(contextWriter{ctx, w}, s)
	}
	gz := gzip.NewWriter(contextWriter{ctx, w})
	if err := encodeStructuredJSON(contextWriter{ctx, gz}, s); err != nil {
		return err
	}
	return gz.Close()
}

type contextWriter struct {
	ctx context.Context
	w   io.Writer
}

func (w contextWriter) Write(p []byte) (int, error) {
	if err := w.ctx.Err(); err != nil {
		return 0, err
	}
	n, err := w.w.Write(p)
	if err == nil && n < len(p) {
		err = io.ErrShortWrite
	}
	return n, err
}

func encodeSourceAttrs(raw json.RawMessage, source *SourceInfo) json.RawMessage {
	if source == nil {
		return raw
	}
	attrs := map[string]json.RawMessage{}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &attrs)
	}
	attrs["source"] = sfAttrs(source)
	return sfAttrs(attrs)
}
func decodeSourceAttrs(raw json.RawMessage) (json.RawMessage, *SourceInfo, error) {
	if len(raw) == 0 {
		return raw, nil, nil
	}
	var attrs map[string]json.RawMessage
	if !bytes.HasPrefix(bytes.TrimSpace(raw), []byte("{")) {
		return nil, nil, fmt.Errorf("structured attrs must be an object")
	}
	if err := json.Unmarshal(raw, &attrs); err != nil {
		return nil, nil, err
	}
	value, exists := attrs["source"]
	if !exists {
		return raw, nil, nil
	}
	delete(attrs, "source")
	var source *SourceInfo
	if string(bytes.TrimSpace(value)) != "null" {
		source = &SourceInfo{}
		if err := strictSFAttrs(value, source); err != nil {
			return nil, nil, err
		}
	}
	return sfAttrs(attrs), source, nil
}

func encodeStructuredJSON(w io.Writer, s *StructuredDocument) error {
	out := bufio.NewWriter(w)
	write := func(p []byte) error { _, e := out.Write(p); return e }
	value := func(v any) error {
		p, e := json.Marshal(v)
		if e != nil {
			return e
		}
		return write(p)
	}
	if e := write([]byte(`{"type":`)); e != nil {
		return e
	}
	if e := value(s.Type); e != nil {
		return e
	}
	if e := write([]byte(`,"attrs":`)); e != nil {
		return e
	}
	if e := value(s.Attrs); e != nil {
		return e
	}
	var node func(StructuredNode) error
	node = func(n StructuredNode) error {
		children := n.Content
		n.Content = nil
		p, e := json.Marshal(n)
		if e != nil {
			return e
		}
		if len(children) == 0 {
			return write(p)
		}
		if e = write(p[:len(p)-1]); e != nil {
			return e
		}
		if e = write([]byte(`,"content":[`)); e != nil {
			return e
		}
		for i, child := range children {
			if i > 0 {
				if e = out.WriteByte(','); e != nil {
					return e
				}
			}
			if e = node(child); e != nil {
				return e
			}
		}
		return write([]byte(`]}`))
	}
	if len(s.Content) > 0 {
		if e := write([]byte(`,"content":[`)); e != nil {
			return e
		}
		for i, n := range s.Content {
			if i > 0 {
				if e := out.WriteByte(','); e != nil {
					return e
				}
			}
			if e := node(n); e != nil {
				return e
			}
		}
		if e := out.WriteByte(']'); e != nil {
			return e
		}
	}
	if e := write([]byte("}\n")); e != nil {
		return e
	}
	return out.Flush()
}

type sfSectionAttrs struct {
	Layout    *Layout                     `json:"layout,omitempty"`
	Start     SectionStart                `json:"start,omitempty"`
	FirstPage bool                        `json:"firstPage,omitempty"`
	Headers   map[string][]StructuredNode `json:"headers,omitempty"`
	Footers   map[string][]StructuredNode `json:"footers,omitempty"`
}

func encodeSFStories(stories RunningStories) (map[string][]StructuredNode, error) {
	out := map[string][]StructuredNode{}
	for _, s := range []struct {
		name  string
		story *RunningStory
	}{{"default", stories.Default}, {"first", stories.First}, {"even", stories.Even}} {
		if s.story == nil {
			continue
		}
		nodes, err := encodeSFBlocks(s.story.Blocks)
		if err != nil {
			return nil, err
		}
		if nodes == nil {
			nodes = []StructuredNode{}
		}
		out[s.name] = nodes
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}
func decodeSFStories(nodes map[string][]StructuredNode, depth int) (RunningStories, error) {
	var out RunningStories
	for key, content := range nodes {
		var target **RunningStory
		switch key {
		case "default":
			target = &out.Default
		case "first":
			target = &out.First
		case "even":
			target = &out.Even
		default:
			return out, fmt.Errorf("unknown running story %q", key)
		}
		if content == nil {
			return out, fmt.Errorf("running story must be an array, not null")
		}
		blocks, err := decodeSFBlocks(content, depth+1)
		if err != nil {
			return out, err
		}
		*target = &RunningStory{blocks}
	}
	return out, nil
}
func encodeSFSection(s *Section) (StructuredNode, error) {
	a := sfSectionAttrs{Start: s.Start, FirstPage: s.FirstPage}
	if s.Layout != nil {
		l := cloneLayout(*s.Layout)
		a.Layout = &l
	}
	var err error
	if a.Headers, err = encodeSFStories(s.Headers); err != nil {
		return StructuredNode{}, err
	}
	if a.Footers, err = encodeSFStories(s.Footers); err != nil {
		return StructuredNode{}, err
	}
	content, err := encodeSFBlocks(s.Blocks)
	if err != nil {
		return StructuredNode{}, err
	}
	return StructuredNode{Type: "section", Attrs: sfAttrs(a), Content: content}, nil
}
func decodeSFSection(n StructuredNode, depth int) (*Section, error) {
	var raw map[string]json.RawMessage
	if len(n.Attrs) > 0 {
		if err := json.Unmarshal(n.Attrs, &raw); err != nil {
			return nil, err
		}
	}
	for _, name := range []string{"headers", "footers"} {
		if v, ok := raw[name]; ok && bytes.Equal(bytes.TrimSpace(v), []byte("null")) {
			return nil, fmt.Errorf("%s must be an object", name)
		}
	}
	var a sfSectionAttrs
	if err := strictSFAttrs(n.Attrs, &a); err != nil {
		return nil, err
	}
	s := &Section{Start: a.Start, FirstPage: a.FirstPage}
	if a.Layout != nil {
		l := cloneLayout(*a.Layout)
		s.Layout = &l
	}
	var err error
	if s.Headers, err = decodeSFStories(a.Headers, depth); err != nil {
		return nil, err
	}
	if s.Footers, err = decodeSFStories(a.Footers, depth); err != nil {
		return nil, err
	}
	s.Blocks, err = decodeSFBlocks(n.Content, depth+1)
	return s, err
}
