// Package manifold reads, writes and converts documents in-process in pure Go.
// Its structured model describes supported content; it is not a Word layout engine.
package manifold

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

type Format string

const (
	DOCX Format = "docx"
	PDF  Format = "pdf"
	JSON Format = "json"
	// SF is versioned ProseMirror JSON, stored with the .sf extension.
	SF Format = "sf"
	// SFGzip is the same ProseMirror JSON in an RFC 1952 gzip stream (.sf.gz).
	SFGzip Format = "sf.gz"
	// VDOCS is the Colleaf version-3 envelope for Univer 1.0.4 snapshots.
	VDOCS Format = "vdocs"
	Text  Format = "txt"
)

// Document is manifold's interchange model. Diagnostics describe input features
// whose structure cannot be represented. Converters reject lossy output unless
// the caller explicitly acknowledges those diagnostics.
type Document struct {
	// UnboundSources retains original source facts without a current content owner.
	UnboundSources    []SourceInfo   `json:"unbound_sources,omitempty"`
	PDFNavigation     *PDFNavigation `json:"pdf_navigation,omitempty"`
	Metadata          Metadata       `json:"metadata,omitempty"`
	Layout            Layout         `json:"layout,omitempty"`
	Blocks            []Block        `json:"blocks"`
	Headers           []Block        `json:"headers,omitempty"`
	Footers           []Block        `json:"footers,omitempty"`
	Notes             []Note         `json:"notes,omitempty"`
	Diagnostics       []Diagnostic   `json:"diagnostics,omitempty"`
	Pages             []PDFPage      `json:"pages,omitempty"`
	EvenAndOddHeaders bool           `json:"even_and_odd_headers,omitempty"`
	// UnresolvedDestinations explicitly retains source links whose target is absent.
	UnresolvedDestinations []string `json:"unresolved_destinations,omitempty"`
}
type Metadata struct {
	Title     string `json:"title,omitempty"`
	Author    string `json:"author,omitempty"`
	Subject   string `json:"subject,omitempty"`
	PageCount int    `json:"page_count,omitempty"`
}

// Layout dimensions are points, 72 per inch. Zero Width/Height/Margin use A4
// and the legacy 54pt fallback. Margins and distance pointers retain explicit zeros.
type Layout struct {
	Width  float64 `json:"width,omitempty"`
	Height float64 `json:"height,omitempty"`
	Margin float64 `json:"margin,omitempty"`
	// Margins overrides Margin; its zero edges are explicit, including all zero.
	Margins *PageMargins `json:"margins,omitempty"`
	// Distances are page-edge to header top/footer bottom, in points. Nil retains
	// writer defaults; a nonnil zero selects the actual edge.
	HeaderDistance *float64 `json:"header_distance,omitempty"`
	FooterDistance *float64 `json:"footer_distance,omitempty"`
}
type Kind string

const (
	Paragraph    Kind = "paragraph"
	Heading      Kind = "heading"
	Table        Kind = "table"
	ListBlock    Kind = "list"
	Picture      Kind = "image"
	PageBreak    Kind = "page_break"
	SectionBlock Kind = "section"
	AnchorPoint  Kind = "anchor_point"
)

type Block struct {
	// PageBreakBefore starts a paragraph/heading on a new page unless already
	// at the page's first flow position. PageBreak blocks always advance a page.
	PageBreakBefore bool `json:"page_break_before,omitempty"`
	// Anchors binds names to the start of this block; AnchorPoint has zero height.
	Anchors []string `json:"anchors,omitempty"`
	// Direction is empty (unspecified), ltr or rtl; it describes paragraph flow.
	Direction string `json:"direction,omitempty"`
	Kind      Kind   `json:"kind"`
	Runs      []Run  `json:"runs,omitempty"`
	Level     int    `json:"level,omitempty"`
	List      string `json:"list,omitempty"` // bullet or number
	Rows      []Row  `json:"rows,omitempty"`
	// ColumnWidths are preferred logical grid-column widths in points. Empty uses
	// equal available widths. Renderers shrink proportionally to fit a container.
	ColumnWidths []float64  `json:"column_widths,omitempty"`
	Image        *Image     `json:"image,omitempty"`
	Items        []ListItem `json:"items,omitempty"`
	// Start is the first ordered-list value; nil means one. Zero is valid.
	Numbering               *ListNumbering `json:"numbering,omitempty"`
	Start                   *int           `json:"start,omitempty"`
	Source                  *SourceInfo    `json:"source,omitempty"`
	Section                 *Section       `json:"section,omitempty"`
	colleafAutoLabel        bool
	colleafLiteral          *colleafLiteralItem
	colleafFlowOrigin       *colleafOrigin
	colleafListOrigin       *colleafOrigin
	colleafItemOrigin       *colleafOrigin
	colleafMarkerOrigin     *colleafOrigin
	colleafImageOrigin      *colleafOrigin
	colleafTableOrigin      *colleafOrigin
	colleafOrigin           *colleafOrigin
	docxSection             *xmlNode
	docxAnchorGroup         string
	docxSectionTail         bool
	docxList                *docxListRef
	docxNumberID            int
	docxNumberLevel         int
	pdfOutputLabels         []*pdfOutputNode
	pdfListMarker           *string
	pdfListMarkerStyle      *Run
	pdfListMarkerContent    []Run
	docxIndent              int
	docxExplicitBreakBefore bool
	docxBeforeList          bool
}

// ListItem owns its ordered blocks, including continuation paragraphs and nested lists.
type ListItem struct {
	// Marker owns an explicit literal label instead of an automatic counter or
	// bullet. Nil selects the list's automatic rule. An empty paragraph represents
	// a deliberately empty label. Marker content is independent of item body text.
	Marker *ListMarker `json:"marker,omitempty"`
	Blocks []Block     `json:"blocks"`
	Source *SourceInfo `json:"source,omitempty"`
}

// ListMarker retains literal visible label content and its independent source owner.
// It does not define an automatic numbering or restart rule.
type ListMarker struct {
	Blocks []Block     `json:"blocks"`
	Source *SourceInfo `json:"source,omitempty"`
}

type Run struct {
	// Language tags preserve the general, East Asian and complex-script bindings.
	Language         string `json:"language,omitempty"`
	EastAsiaLanguage string `json:"east_asia_language,omitempty"`
	ComplexLanguage  string `json:"complex_language,omitempty"`
	// ComplexScript forces complex-script formatting/language selection (OOXML cs).
	ComplexScript bool `json:"complex_script,omitempty"`
	// Direction enables/disables RTL run characteristics, independently of
	// paragraph flow. Empty is unspecified; ltr does not override strong RTL text.
	Direction string `json:"direction,omitempty"`

	// Empty family/color and zero size use the renderer default. Sizes are points.
	FontFamily string  `json:"font_family,omitempty"`
	FontSize   float64 `json:"font_size,omitempty"`
	Color      string  `json:"color,omitempty"` // #RRGGBB or auto

	Text      string `json:"text"`
	Bold      bool   `json:"bold,omitempty"`
	Italic    bool   `json:"italic,omitempty"`
	Underline bool   `json:"underline,omitempty"`
	Strike    bool   `json:"strike,omitempty"`
	Link      string `json:"link,omitempty"`
	// Destination is an internal name, separate from generic URI Link references.
	Destination string `json:"destination,omitempty"`
	// Anchor declares a zero-width named navigation point between text runs.
	Anchor              string         `json:"anchor,omitempty"`
	Note                *NoteReference `json:"note,omitempty"`
	Source              *SourceInfo    `json:"source,omitempty"`
	colleafAnchorOrigin *colleafOrigin
	colleafRunOrigin    *colleafOrigin
	pdfOutput           *pdfOutputNode
}
type Row struct {
	// RepeatHeader repeats a contiguous prefix of rows on table continuation pages.
	// It is a pagination property, separate from semantic header-cell roles.
	RepeatHeader bool   `json:"repeat_header,omitempty"`
	Cells        []Cell `json:"cells"`
}
type Cell struct {
	Blocks  []Block `json:"blocks"`
	ColSpan int     `json:"col_span,omitempty"`
	// RowSpan includes the origin row; zero means one. Covered cells are
	// omitted from subsequent rows, rather than stored as duplicate content.
	RowSpan       int         `json:"row_span,omitempty"`
	Source        *SourceInfo `json:"source,omitempty"`
	colleafOrigin *colleafOrigin
}
type Image struct {
	pdfOutput *pdfOutputNode
	Data      []byte  `json:"data"` // JSON represents bytes as base64.
	MediaType string  `json:"media_type"`
	Alt       string  `json:"alt,omitempty"`
	Width     float64 `json:"width,omitempty"`
	Height    float64 `json:"height,omitempty"`
}
type Note struct {
	pdfOutputLabel *pdfOutputNode
	// BodyLabelContent owns a literal rich body mark. It excludes BodyLabel and
	// BodyLabelStyle; only nonempty text runs are allowed, without nested atoms.
	BodyLabelContent []Run `json:"body_label_content,omitempty"`
	// BodyLabelStyle supplies formatting/navigation for the resolved body mark, without text or atoms.
	BodyLabelStyle *Run `json:"body_label_style,omitempty"`
	// BodyLabel overrides the body mark, independently of citation marks.
	BodyLabel *string `json:"body_label,omitempty"`
	// KindInferred distinguishes an editorial placement choice from an explicit source kind.
	KindInferred bool        `json:"kind_inferred,omitempty"`
	LabelSource  *SourceInfo `json:"label_source,omitempty"`
	Source       *SourceInfo `json:"source,omitempty"`
	// Label is a literal display mark. Empty uses automatic per-kind numbering.
	// Custom labels do not advance the automatic counter.
	Label  string  `json:"label,omitempty"`
	ID     string  `json:"id"`
	Kind   string  `json:"kind"`
	Blocks []Block `json:"blocks"`
}
type Diagnostic struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

var ErrUnsupported = errors.New("unsupported document feature")
var ErrLossy = errors.New("conversion would lose document features")

type ReadOptions struct {
	Password string
	// PDFSkipInference retains untagged PDF paint order and baseline groups for
	// callers inspecting physical extraction. Tagged logical trees still apply.
	// The default recovers geometric structure with disclosed uncertainty.
	PDFSkipInference bool
	// PDFSkipStructure ignores the logical tree and retains positioned content.
	// Combine with PDFSkipInference for a physical paint-order inspection.
	PDFSkipStructure bool
}
type WriteOptions struct {
	// AllowLossy acknowledges import/reflow diagnostics and documented output
	// normalizations, including DOCX half-point font-size rounding with checked
	// original intent. Unsupported writer features still return errors.
	AllowLossy bool
	// PDFFonts supplies in-memory TTF faces; missing families/styles never substitute.
	PDFFonts []FontFace
	// PDFFontMap explicitly maps requested families to supplied/built-in families.
	PDFFontMap map[string]string
}

func FormatFromName(name string) (Format, error) {
	if strings.HasSuffix(strings.ToLower(name), ".sf.gz") {
		return SFGzip, nil
	}
	f := Format(strings.ToLower(strings.TrimPrefix(filepath.Ext(name), ".")))
	switch f {
	case DOCX, PDF, JSON, SF, Text, VDOCS:
		return f, nil
	}
	return "", fmt.Errorf("%w: format %q", ErrUnsupported, f)
}
func Read(ctx context.Context, r io.Reader, format Format, opts ReadOptions) (*Document, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	switch format {
	case VDOCS:
		return ReadVDOCS(ctx, r)
	case DOCX:
		return ReadDOCX(ctx, r)
	case PDF:
		return ReadPDF(ctx, r, opts)
	case SF:
		_, d, err := readStructured(ctx, r)
		return d, err
	case SFGzip:
		if r == nil {
			return nil, errors.New("nil document reader")
		}
		compressed, err := gzip.NewReader(contextReader{ctx, r})
		if err != nil {
			return nil, fmt.Errorf("structured gzip: %w", err)
		}
		defer compressed.Close()
		_, d, err := readStructured(ctx, compressed)
		return d, err
	case JSON:
		var d Document
		dec := json.NewDecoder(r)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&d); err != nil {
			return nil, fmt.Errorf("document JSON: %w", err)
		}
		var extra any
		if err := dec.Decode(&extra); err != io.EOF {
			return nil, errors.New("document JSON must contain one value")
		}
		return &d, d.Validate()
	case Text:
		b, err := readContext(ctx, r)
		if err != nil {
			return nil, err
		}
		d := &Document{}
		for i, page := range strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\f") {
			if i > 0 {
				d.Blocks = append(d.Blocks, Block{Kind: PageBreak})
			}
			for _, p := range strings.Split(page, "\n") {
				d.Blocks = append(d.Blocks, Block{Kind: Paragraph, Runs: []Run{{Text: p}}})
			}
		}
		return d, d.Validate()
	}
	return nil, fmt.Errorf("%w: format %q", ErrUnsupported, format)
}
func Write(ctx context.Context, w io.Writer, d *Document, format Format, opts WriteOptions) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if w == nil {
		return errors.New("nil document writer")
	}
	if err := d.Validate(); err != nil {
		return err
	}
	if format != JSON && format != SF && format != SFGzip && format != VDOCS && format != Text && len(d.Diagnostics) > 0 && !opts.AllowLossy {
		return fmt.Errorf("%w: %s", ErrLossy, d.Diagnostics[0].Message)
	}
	if (format == DOCX || format == PDF) && d.hasPDFSource() && !opts.AllowLossy {
		return fmt.Errorf("%w: positioned PDF source content will be reflowed and provenance omitted", ErrLossy)
	}
	if format == DOCX && !opts.AllowLossy && len(variantNoteKeys(d)) > 0 {
		for _, n := range d.Notes {
			if n.Label == "" {
				return fmt.Errorf("%w: DOCX variant note marks require static portable labels; automatic intent is retained in checked metadata", ErrLossy)
			}
		}
	}
	if format == DOCX || format == PDF {
		d = resolveNumberingDocument(d)
	}
	switch format {
	case VDOCS:
		return writeColleaf(ctx, w, d)
	case DOCX:
		return writeDOCX(ctx, w, d, opts.AllowLossy)
	case PDF:
		return writePDFWithFonts(ctx, w, d, opts.PDFFonts, opts.PDFFontMap)
	case SF, SFGzip:
		s, err := ToStructured(d)
		if err != nil {
			return err
		}
		return encodeStructured(ctx, w, s, format == SFGzip)
	case JSON:
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(d)
	case Text:
		_, err := io.Copy(w, strings.NewReader(d.Text()))
		return err
	}
	return fmt.Errorf("%w: format %q", ErrUnsupported, format)
}

// Text includes body content and notes. Page boundaries are form-feed characters.
func (d *Document) Text() string {
	if d == nil {
		return ""
	}
	var b strings.Builder
	textBlocks(&b, d.Blocks)
	for _, n := range d.Notes {
		textBlocks(&b, n.Blocks)
	}
	return strings.TrimRight(b.String(), "\n")
}
func textBlocks(b *strings.Builder, blocks []Block) {
	seenSection := false
	for _, v := range blocks {
		switch v.Kind {
		case SectionBlock:
			if seenSection && v.Section.Start != SectionContinuous {
				b.WriteString("\f")
			}
			seenSection = true
			textBlocks(b, v.Section.Blocks)
		case AnchorPoint:
			continue
		case PageBreak:
			b.WriteString("\f")
		case Table:
			for _, row := range v.Rows {
				for i, c := range row.Cells {
					if i > 0 {
						b.WriteByte('\t')
					}
					var cell strings.Builder
					textBlocks(&cell, c.Blocks)
					b.WriteString(strings.TrimRight(cell.String(), "\n"))
				}
				b.WriteByte('\n')
			}
		case ListBlock:
			for _, item := range v.Items {
				if item.Marker != nil {
					var marker strings.Builder
					textBlocks(&marker, item.Marker.Blocks)
					text := strings.TrimRight(marker.String(), "\n")
					if text != "" {
						b.WriteString(text)
						last := item.Marker.Blocks[len(item.Marker.Blocks)-1].Kind
						if last == Paragraph || last == Heading {
							b.WriteByte(' ')
						} else {
							b.WriteByte('\n')
						}
					}
				}
				textBlocks(b, item.Blocks)
			}
		case Picture:
			if v.Image != nil && v.Image.Alt != "" {
				b.WriteString(v.Image.Alt)
				b.WriteByte('\n')
			}
		default:
			for _, r := range v.Runs {
				if r.Note != nil {
					b.WriteString("[" + r.Note.Kind + ":" + r.Note.ID + "]")
				} else {
					b.WriteString(r.Text)
				}
			}
			b.WriteByte('\n')
		}
	}
}
func (d *Document) Validate() error {
	if d == nil {
		return errors.New("nil document")
	}
	if err := validatePDFPages(d.Pages); err != nil {
		return err
	}
	for i := range d.UnboundSources {
		if err := d.UnboundSources[i].validate(d.Pages); err != nil {
			return err
		}
	}
	for _, s := range []string{d.Metadata.Title, d.Metadata.Author, d.Metadata.Subject} {
		if err := validateString(s); err != nil {
			return err
		}
	}
	if err := validateSectionPlacement(d); err != nil {
		return err
	}
	if err := validateLayout(d.Layout); err != nil {
		return err
	}
	var walk func([]Block, int) error
	walk = func(bs []Block, depth int) error {
		if depth > 64 {
			return errors.New("document nesting exceeds 64 levels")
		}
		for _, b := range bs {
			if err := b.Source.validate(d.Pages); err != nil {
				return err
			}
			if err := validateDirection(b.Direction); err != nil {
				return err
			}
			if b.Direction != "" && b.Kind != Paragraph && b.Kind != Heading {
				return errors.New("direction requires paragraph or heading")
			}
			if b.PageBreakBefore && b.Kind != Paragraph && b.Kind != Heading {
				return errors.New("page break before requires paragraph or heading")
			}
			if b.Kind != ListBlock && (len(b.Items) > 0 || b.Start != nil) {
				return errors.New("list items/start require a list block")
			}
			for _, r := range b.Runs {
				if r.Note != nil && r.Note.LabelContent != nil {
					if err := validateCitationContent(r.Note, d.Pages); err != nil {
						return err
					}
				}
				if err := validateTextStyle(r); err != nil {
					return err
				}
				if err := r.Source.validate(d.Pages); err != nil {
					return err
				}
				if err := validateString(r.Text); err != nil {
					return err
				}
				if err := validateString(r.Link); err != nil {
					return err
				}
			}
			if b.Numbering != nil && (b.Kind != ListBlock || b.List != "number") {
				return errors.New("numbering requires an ordered list")
			}
			if err := validateNumbering(b.Numbering); err != nil {
				return err
			}
			if b.Kind != Table && len(b.ColumnWidths) > 0 {
				return errors.New("column widths require a table")
			}
			switch b.Kind {
			case SectionBlock:
				if b.Section == nil || len(b.Runs) > 0 || len(b.Rows) > 0 || b.Image != nil || len(b.Items) > 0 || b.List != "" || b.Level != 0 {
					return errors.New("section content belongs in its section container")
				}
				if err := walk(b.Section.Blocks, depth+1); err != nil {
					return err
				}
				for _, story := range sectionStories(b.Section) {
					if story != nil {
						if err := walk(story.Blocks, depth+1); err != nil {
							return err
						}
					}
				}
			case Paragraph, Heading:
				if len(b.Rows) > 0 || b.Image != nil {
					return errors.New("paragraph cannot contain rows or an image")
				}
				if b.Kind == Heading && (b.Level < 1 || b.Level > 6) {
					return errors.New("heading level must be 1 through 6")
				}
				if b.List != "" && b.List != "bullet" && b.List != "number" {
					return errors.New("list must be bullet or number")
				}
			case ListBlock:
				if b.List != "bullet" && b.List != "number" {
					return errors.New("list kind must be bullet or number")
				}
				if b.Level != 0 || len(b.Runs) > 0 || len(b.Rows) > 0 || b.Image != nil {
					return errors.New("list content belongs in items")
				}
				if len(b.Items) == 0 {
					return errors.New("list requires items")
				}
				if b.Start != nil && (b.List != "number" || *b.Start < 0 || int64(*b.Start)+int64(len(b.Items))-1 > 2147483647) {
					return errors.New("invalid ordered-list start/range")
				}

				for _, item := range b.Items {
					if item.Marker != nil {
						if err := item.Marker.Source.validate(d.Pages); err != nil {
							return err
						}
						if len(item.Marker.Blocks) == 0 {
							return errors.New("literal list marker requires content; use an empty paragraph for an empty label")
						}
						if err := validateLiteralMarker(item.Marker.Blocks, depth+1); err != nil {
							return err
						}
						if err := walk(item.Marker.Blocks, depth+1); err != nil {
							return err
						}
					}
					if err := item.Source.validate(d.Pages); err != nil {
						return err
					}
					if len(item.Blocks) == 0 || (item.Blocks[0].Kind != Paragraph && item.Blocks[0].Kind != Heading) {
						return errors.New("list item must start with paragraph or heading")
					}
					for _, child := range item.Blocks {
						if child.Kind != ListBlock && child.List != "" {
							return errors.New("explicit list item cannot contain legacy list paragraphs")
						}
					}
					if err := walk(item.Blocks, depth+1); err != nil {
						return err
					}
				}
			case Table:
				if len(b.Runs) > 0 || b.Image != nil {
					return errors.New("table cannot contain direct runs or an image")
				}
				g, err := tableGrid(b.Rows)
				if err != nil {
					return err
				}
				if err := validateColumnWidths(b.ColumnWidths, g.Columns); err != nil {
					return err
				}
				for _, r := range b.Rows {
					for _, c := range r.Cells {
						if err := c.Source.validate(d.Pages); err != nil {
							return err
						}
						if c.ColSpan < 0 || c.ColSpan > 1000 {
							return errors.New("invalid column span")
						}
						if err := walk(c.Blocks, depth+1); err != nil {
							return err
						}
					}
				}
			case Picture:
				if b.Image == nil || len(b.Image.Data) == 0 {
					return errors.New("image data is required")
				}
				if err := validateString(b.Image.Alt); err != nil {
					return err
				}
				if b.Image.Width < 0 || b.Image.Height < 0 || math.IsNaN(b.Image.Width) || math.IsNaN(b.Image.Height) || math.IsInf(b.Image.Width, 0) || math.IsInf(b.Image.Height, 0) {
					return errors.New("invalid image dimensions")
				}
			case AnchorPoint:
				if len(b.Anchors) == 0 || len(b.Runs) > 0 || len(b.Rows) > 0 || b.Image != nil || b.List != "" || len(b.Items) > 0 || b.Section != nil || b.Level != 0 || b.PageBreakBefore || b.Direction != "" || len(b.ColumnWidths) != 0 || b.Start != nil || b.Numbering != nil {
					return errors.New("anchor point requires only named anchors")
				}
			case PageBreak:
				if len(b.Runs) > 0 || len(b.Rows) > 0 || b.Image != nil {
					return errors.New("page break cannot contain content")
				}
			default:
				return fmt.Errorf("unknown block kind %q", b.Kind)
			}
		}
		return nil
	}
	for _, bs := range [][]Block{d.Blocks, d.Headers, d.Footers} {
		if err := walk(bs, 0); err != nil {
			return err
		}
	}
	for _, n := range d.Notes {
		if n.BodyLabelContent != nil {
			if n.BodyLabel != nil || n.BodyLabelStyle != nil || len(n.BodyLabelContent) == 0 {
				return fmt.Errorf("rich body label requires one content authority")
			}
			for _, run := range n.BodyLabelContent {
				if run.Text == "" || run.Note != nil || run.Anchor != "" {
					return fmt.Errorf("rich body label requires nonempty text runs without atoms")
				}
			}
			if err := validateNoteLabel(noteLabelContentText(n.BodyLabelContent)); err != nil {
				return err
			}
			if err := walk([]Block{{Kind: Paragraph, Runs: n.BodyLabelContent}}, 0); err != nil {
				return err
			}
		}
		if n.BodyLabelStyle != nil {
			style := *n.BodyLabelStyle
			if style.Text != "" || style.Note != nil || style.Anchor != "" || style.Source != nil {
				return fmt.Errorf("body label style cannot contain text, atoms or source; use LabelSource for provenance")
			}
			style.Text = "label"
			if err := walk([]Block{{Kind: Paragraph, Runs: []Run{style}}}, 0); err != nil {
				return err
			}
		}
		if err := n.LabelSource.validate(d.Pages); err != nil {
			return err
		}
		if err := n.Source.validate(d.Pages); err != nil {
			return err
		}
		if err := walk(n.Blocks, 0); err != nil {
			return err
		}
	}
	if c, err := catalogDestinations(d, true); err != nil {
		return err
	} else if err := d.PDFNavigation.validate(d, c); err != nil {
		return err
	}
	return validateNotes(d)
}
func layout(l Layout) (float64, float64, float64) {
	w, h, m := l.Width, l.Height, l.Margin
	if w == 0 {
		w = 595.28
	}
	if h == 0 {
		h = 841.89
	}
	if m == 0 {
		m = 54
	}
	return w, h, m
}
func (d *Document) warn(code, message string) {
	for _, v := range d.Diagnostics {
		if v.Code == code && v.Message == message {
			return
		}
	}
	d.Diagnostics = append(d.Diagnostics, Diagnostic{code, message})
}

type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(p)
}
func readContext(ctx context.Context, r io.Reader) ([]byte, error) {
	if r == nil {
		return nil, errors.New("nil document reader")
	}
	return io.ReadAll(contextReader{ctx, r})
}

func validateString(s string) error {
	if !utf8.ValidString(s) {
		return errors.New("document strings must be valid UTF-8")
	}
	for _, r := range s {
		if (r < 0x20 && r != '\n' && r != '\r' && r != '\t') || r == 0xFFFE || r == 0xFFFF {
			return fmt.Errorf("document string contains invalid XML character %U", r)
		}
	}
	return nil
}
