// Package manydoc reads, writes and converts documents in-process in pure Go.
// Its structured model describes supported content; it is not a Word layout engine.
package manydoc

import (
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
	SF   Format = "sf"
	Text Format = "txt"
)

// Document is manydoc's interchange model. Diagnostics describe input features
// whose structure cannot be represented. Converters reject lossy output unless
// the caller explicitly acknowledges those diagnostics.
type Document struct {
	Metadata    Metadata     `json:"metadata,omitempty"`
	Layout      Layout       `json:"layout,omitempty"`
	Blocks      []Block      `json:"blocks"`
	Headers     []Block      `json:"headers,omitempty"`
	Footers     []Block      `json:"footers,omitempty"`
	Notes       []Note       `json:"notes,omitempty"`
	Diagnostics []Diagnostic `json:"diagnostics,omitempty"`
	Pages       []PDFPage    `json:"pages,omitempty"`
}
type Metadata struct {
	Title     string `json:"title,omitempty"`
	Author    string `json:"author,omitempty"`
	Subject   string `json:"subject,omitempty"`
	PageCount int    `json:"page_count,omitempty"`
}

// Layout dimensions are points, 72 per inch. Zero values use A4 and 54pt margins.
type Layout struct {
	Width  float64 `json:"width,omitempty"`
	Height float64 `json:"height,omitempty"`
	Margin float64 `json:"margin,omitempty"`
}
type Kind string

const (
	Paragraph Kind = "paragraph"
	Heading   Kind = "heading"
	Table     Kind = "table"
	ListBlock Kind = "list"
	Picture   Kind = "image"
	PageBreak Kind = "page_break"
)

type Block struct {
	Kind  Kind       `json:"kind"`
	Runs  []Run      `json:"runs,omitempty"`
	Level int        `json:"level,omitempty"`
	List  string     `json:"list,omitempty"` // bullet or number
	Rows  []Row      `json:"rows,omitempty"`
	Image *Image     `json:"image,omitempty"`
	Items []ListItem `json:"items,omitempty"`
	// Start is the first ordered-list value; nil means one. Zero is valid.
	Start           *int        `json:"start,omitempty"`
	Source          *SourceInfo `json:"source,omitempty"`
	docxList        *docxListRef
	docxNumberID    int
	docxNumberLevel int
	docxIndent      int
}

// ListItem owns its ordered blocks, including continuation paragraphs and nested lists.
type ListItem struct {
	Blocks []Block     `json:"blocks"`
	Source *SourceInfo `json:"source,omitempty"`
}

type Run struct {
	Text      string         `json:"text"`
	Bold      bool           `json:"bold,omitempty"`
	Italic    bool           `json:"italic,omitempty"`
	Underline bool           `json:"underline,omitempty"`
	Strike    bool           `json:"strike,omitempty"`
	Link      string         `json:"link,omitempty"`
	Note      *NoteReference `json:"note,omitempty"`
	Source    *SourceInfo    `json:"source,omitempty"`
}
type Row struct {
	Cells []Cell `json:"cells"`
}
type Cell struct {
	Blocks  []Block `json:"blocks"`
	ColSpan int     `json:"col_span,omitempty"`
	// RowSpan includes the origin row; zero means one. Covered cells are
	// omitted from subsequent rows, rather than stored as duplicate content.
	RowSpan int         `json:"row_span,omitempty"`
	Source  *SourceInfo `json:"source,omitempty"`
}
type Image struct {
	Data      []byte  `json:"data"` // JSON represents bytes as base64.
	MediaType string  `json:"media_type"`
	Alt       string  `json:"alt,omitempty"`
	Width     float64 `json:"width,omitempty"`
	Height    float64 `json:"height,omitempty"`
}
type Note struct {
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
}
type WriteOptions struct {
	// AllowLossy acknowledges diagnostics on an imported document. Unsupported
	// writer features still return errors rather than silently producing bad output.
	AllowLossy bool
}

func FormatFromName(name string) (Format, error) {
	f := Format(strings.ToLower(strings.TrimPrefix(filepath.Ext(name), ".")))
	switch f {
	case DOCX, PDF, JSON, SF, Text:
		return f, nil
	}
	return "", fmt.Errorf("%w: format %q", ErrUnsupported, f)
}
func Read(ctx context.Context, r io.Reader, format Format, opts ReadOptions) (*Document, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	switch format {
	case DOCX:
		return ReadDOCX(ctx, r)
	case PDF:
		return ReadPDF(ctx, r, opts)
	case SF:
		s, err := ReadStructured(ctx, r)
		if err != nil {
			return nil, err
		}
		return FromStructured(s)
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
	if format != JSON && format != SF && format != Text && len(d.Diagnostics) > 0 && !opts.AllowLossy {
		return fmt.Errorf("%w: %s", ErrLossy, d.Diagnostics[0].Message)
	}
	if (format == DOCX || format == PDF) && d.hasPDFSource() && !opts.AllowLossy {
		return fmt.Errorf("%w: positioned PDF source content will be reflowed and provenance omitted", ErrLossy)
	}
	switch format {
	case DOCX:
		return writeDOCX(ctx, w, d)
	case PDF:
		return writePDF(ctx, w, d)
	case SF:
		s, err := ToStructured(d)
		if err != nil {
			return err
		}
		return WriteStructured(ctx, w, s)
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
	for _, v := range blocks {
		switch v.Kind {
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
	for _, s := range []string{d.Metadata.Title, d.Metadata.Author, d.Metadata.Subject} {
		if err := validateString(s); err != nil {
			return err
		}
	}
	for _, n := range []float64{d.Layout.Width, d.Layout.Height, d.Layout.Margin} {
		if n < 0 || math.IsNaN(n) || math.IsInf(n, 0) {
			return errors.New("layout dimensions must be finite and nonnegative")
		}
	}
	w, h, m := layout(d.Layout)
	if w <= 2*m || h <= 2*m {
		return errors.New("page margins leave no content area")
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
			if b.Kind != ListBlock && (len(b.Items) > 0 || b.Start != nil) {
				return errors.New("list items/start require a list block")
			}
			for _, r := range b.Runs {
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
			switch b.Kind {
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
				if _, err := tableGrid(b.Rows); err != nil {
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
		if err := walk(n.Blocks, 0); err != nil {
			return err
		}
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
