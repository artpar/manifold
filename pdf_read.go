package manydoc

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"

	"github.com/benoitkugler/textlayout/fonts/glyphsnames"
	"github.com/benoitkugler/textlayout/fonts/simpleencodings"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// ReadPDF extracts page content and painted form appearances. It does not
// reconstruct original paragraph/table structure or perform OCR.
func ReadPDF(ctx context.Context, src io.Reader, opts ReadOptions) (doc *Document, err error) {
	// A dependency parser's panic must never escape this public boundary or
	// yield partially extracted content as a successful document.
	defer func() {
		if p := recover(); p != nil {
			doc = nil
			err = fmt.Errorf("malformed PDF: %v", p)
		}
	}()
	b, err := readContext(ctx, src)
	if err != nil {
		return nil, err
	}
	conf := &model.Configuration{Reader15: true, ValidationMode: model.ValidationRelaxed, UserPW: opts.Password, OwnerPW: opts.Password, Offline: true}
	p, err := pdfcpu.ReadWithContext(ctx, bytes.NewReader(b), conf)
	if err != nil {
		return nil, fmt.Errorf("PDF read: %w", err)
	}
	// Accept version-extension conventions during object parsing, but never
	// accept partially decoded corrupt page streams as successful extraction.
	p.XRefTable.ValidationMode = model.ValidationStrict
	if err = p.EnsurePageCount(); err != nil {
		return nil, err
	}
	if p.PageCount < 0 {
		return nil, errors.New("negative PDF page count")
	}
	doc = &Document{Metadata: Metadata{Title: p.Title, Author: p.Author, Subject: p.Subject, PageCount: p.PageCount}}
	doc.warn("pdf_layout", "PDF text is extracted in content-stream order; editable document layout is not reconstructed")
	r := &pdfReader{ctx: ctx, pdf: p, doc: doc}
	for page := 1; page <= p.PageCount; page++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		d, _, attrs, e := p.PageDict(page, false)
		if e != nil {
			return nil, fmt.Errorf("PDF page %d: %w", page, e)
		}
		data, e := p.PageContent(d, page)
		if e != nil && !errors.Is(e, model.ErrNoContent) {
			return nil, e
		}
		r.lines = nil
		r.current = nil
		r.fonts = map[string]*pdfFont{}
		if e = r.content(data, attrs.Resources, 0, pdfTextState{}); e != nil {
			return nil, fmt.Errorf("PDF page %d: %w", page, e)
		}
		if ann, e := p.DereferenceArray(d["Annots"]); e != nil {
			return nil, e
		} else {
			for _, a := range ann {
				ad, e := p.DereferenceDict(a)
				if e != nil {
					return nil, e
				}
				if ad == nil {
					continue
				}
				ap, e := p.DereferenceDict(ad["AP"])
				if e != nil {
					return nil, e
				}
				if ap == nil {
					continue
				}
				o, e := p.Dereference(ap["N"])
				if e != nil {
					return nil, e
				}
				if states, ok := o.(types.Dict); ok {
					state := ad.NameEntry("AS")
					if state != nil {
						o = states[*state]
					} else {
						r.doc.warn("annotation_state", "Annotation has multiple appearances without a selected state")
						continue
					}
				}
				if o != nil {
					if e = r.form(o, attrs.Resources, 0); e != nil {
						return nil, e
					}
				}
			}
		}
		r.newline()
		if page > 1 {
			doc.Blocks = append(doc.Blocks, Block{Kind: PageBreak})
		}
		doc.Blocks = append(doc.Blocks, r.lines...)
	}
	if strings.TrimSpace(strings.ReplaceAll(doc.Text(), "\f", "")) == "" {
		doc.warn("no_text", "No extractable text was found; scanned/image-only documents require OCR")
	}
	return doc, nil
}

type pdfReader struct {
	ctx            context.Context
	pdf            *model.Context
	doc            *Document
	fonts          map[string]*pdfFont
	lines          []Block
	current        []Run
	lastEnd, lastY float64
	havePosition   bool
}
type pdfTextState struct {
	font                                               *pdfFont
	size, leading, wordSpace, charSpace, x, y, cursorX float64
}

func (r *pdfReader) newline() {
	r.havePosition = false
	if len(r.current) > 0 {
		r.lines = append(r.lines, Block{Kind: Paragraph, Runs: r.current})
		r.current = nil
	}
}
func (r *pdfReader) space() {
	if len(r.current) == 0 {
		return
	}
	last := &r.current[len(r.current)-1]
	if last.Text != "" && !strings.HasSuffix(last.Text, " ") && !strings.HasSuffix(last.Text, "\t") {
		last.Text += " "
	}
}
func (r *pdfReader) show(v any, s *pdfTextState) error {
	b, err := pdfString(v)
	if err != nil {
		return err
	}
	if len(b) == 0 {
		return nil
	}
	if s.font == nil {
		return errors.New("PDF text has no selected font")
	}
	text, err := s.font.decode(b)
	if err != nil {
		return err
	}
	if text != "" {
		if r.havePosition && math.Abs(s.y-r.lastY) < max(1, s.size*.4) && s.cursorX-r.lastEnd > max(.5, s.size*.15) {
			r.space()
		}
		r.current = append(r.current, Run{Text: text, Bold: s.font.bold, Italic: s.font.italic})
		s.cursorX += s.font.advance(b)*s.size/1000 + float64(len([]rune(text)))*s.charSpace + float64(strings.Count(text, " "))*s.wordSpace
		r.lastEnd, r.lastY, r.havePosition = s.cursorX, s.y, true
	}
	return nil
}
func (r *pdfReader) content(data []byte, resources types.Dict, depth int, state pdfTextState) error {
	if depth > 32 {
		return errors.New("cyclic or excessively nested PDF Form XObject")
	}
	input := string(data)
	var args []any
	var saved []pdfTextState
	for input != "" {
		v, err := nextPDFToken(r.ctx, &input)
		if err != nil {
			return err
		}
		if v == nil {
			break
		}
		op, isOp := v.(pdfOperator)
		if !isOp {
			args = append(args, v)
			continue
		}
		need := func(n int) error {
			if len(args) != n {
				return fmt.Errorf("PDF operator %s expects %d operands, got %d", op, n, len(args))
			}
			return nil
		}
		switch string(op) {
		case "BT":
			if err := need(0); err != nil {
				return err
			}
			r.newline()
			state.x, state.y, state.cursorX = 0, 0, 0
		case "ET":
			if err := need(0); err != nil {
				return err
			}
		case "Tf":
			if err := need(2); err != nil {
				return err
			}
			name, err := pdfName(args[0])
			if err != nil {
				return err
			}
			state.font, err = r.font(resources, name)
			if err != nil {
				return err
			}
			state.size, err = pdfNumber(args[1])
			if err != nil {
				return err
			}
		case "Tj":
			if err := need(1); err != nil {
				return err
			}
			if err := r.show(args[0], &state); err != nil {
				return err
			}
		case "TJ":
			if err := need(1); err != nil {
				return err
			}
			arr, ok := args[0].(types.Array)
			if !ok {
				return errors.New("TJ operand is not an array")
			}
			for _, a := range arr {
				switch a.(type) {
				case types.StringLiteral, types.HexLiteral:
					if err := r.show(a, &state); err != nil {
						return err
					}
				default:
					n, e := pdfNumber(a)
					if e != nil {
						return e
					}
					if n <= -150 {
						r.space()
					}
					state.cursorX -= n * state.size / 1000
				}
			}
		case "'":
			if err := need(1); err != nil {
				return err
			}
			r.newline()
			state.cursorX = state.x
			if err := r.show(args[0], &state); err != nil {
				return err
			}
		case "\"":
			if err := need(3); err != nil {
				return err
			}
			state.wordSpace, err = pdfNumber(args[0])
			if err != nil {
				return err
			}
			state.charSpace, err = pdfNumber(args[1])
			if err != nil {
				return err
			}
			r.newline()
			state.cursorX = state.x
			if err := r.show(args[2], &state); err != nil {
				return err
			}
		case "T*":
			if err := need(0); err != nil {
				return err
			}
			r.newline()
			state.y -= state.leading
			state.cursorX = state.x
		case "Td", "TD":
			if err := need(2); err != nil {
				return err
			}
			dx, e := pdfNumber(args[0])
			if e != nil {
				return e
			}
			dy, e := pdfNumber(args[1])
			if e != nil {
				return e
			}
			if math.Abs(dy) > .1 {
				r.newline()
			}
			state.x += dx
			state.y += dy
			state.cursorX = state.x
			if op == "TD" {
				state.leading = -dy
			}
		case "Tm":
			if err := need(6); err != nil {
				return err
			}
			x, e := pdfNumber(args[4])
			if e != nil {
				return e
			}
			y, e := pdfNumber(args[5])
			if e != nil {
				return e
			}
			if math.Abs(y-state.y) > max(1, state.size*.4) {
				r.newline()
			}
			state.x = x
			state.y = y
			state.cursorX = x
		case "TL", "Tw", "Tc":
			if err := need(1); err != nil {
				return err
			}
			n, e := pdfNumber(args[0])
			if e != nil {
				return e
			}
			switch op {
			case "TL":
				state.leading = n
			case "Tw":
				state.wordSpace = n
			case "Tc":
				state.charSpace = n
			}
		case "q":
			if err := need(0); err != nil {
				return err
			}
			saved = append(saved, state)
		case "Q":
			if err := need(0); err != nil {
				return err
			}
			if len(saved) == 0 {
				return errors.New("unbalanced PDF graphics state")
			}
			state = saved[len(saved)-1]
			saved = saved[:len(saved)-1]
		case "Do":
			if err := need(1); err != nil {
				return err
			}
			name, e := pdfName(args[0])
			if e != nil {
				return e
			}
			objects, e := r.pdf.DereferenceDict(resources["XObject"])
			if e != nil {
				return e
			}
			o := objects[name]
			if o == nil {
				return fmt.Errorf("missing PDF XObject %q", name)
			}
			if e = r.form(o, resources, depth+1); e != nil {
				return e
			}
		case "BI":
			return fmt.Errorf("%w: inline PDF images; external image XObjects are supported", ErrUnsupported)
		case "cm", "w", "J", "j", "M", "d", "ri", "i", "gs", "m", "l", "c", "v", "y", "h", "re", "S", "s", "f", "F", "f*", "B", "B*", "b", "b*", "n", "W", "W*", "CS", "cs", "SC", "SCN", "sc", "scn", "G", "g", "RG", "rg", "K", "k", "sh", "MP", "DP", "BMC", "BDC", "EMC", "BX", "EX", "d0", "d1", "Tr", "Ts", "Tz":
		default:
			return fmt.Errorf("%w: PDF content operator %q", ErrUnsupported, op)
		}
		args = nil
	}
	if len(args) > 0 {
		return errors.New("PDF content ends with operands without an operator")
	}
	return nil
}
func (r *pdfReader) form(o types.Object, parent types.Dict, depth int) error {
	sd, _, err := r.pdf.DereferenceStreamDict(o)
	if err != nil {
		return err
	}
	if sd == nil {
		return errors.New("PDF XObject is not a stream")
	}
	sub := sd.Dict.NameEntry("Subtype")
	if sub == nil {
		return errors.New("PDF XObject subtype missing")
	}
	if *sub == "Image" {
		return nil
	}
	if *sub != "Form" {
		return fmt.Errorf("%w: PDF XObject subtype %s", ErrUnsupported, *sub)
	}
	if err := sd.Decode(); err != nil {
		return err
	}
	resources, err := r.pdf.DereferenceDict(sd.Dict["Resources"])
	if err != nil {
		return err
	}
	if resources == nil {
		resources = parent
	}
	r.newline()
	return r.content(sd.Content, resources, depth+1, pdfTextState{})
}

type pdfFont struct {
	mapping      *unicodeMap
	encoding     simpleencodings.Encoding
	unicode      bool
	bold, italic bool
	widths       map[uint32]float64
}

func (f *pdfFont) advance(b []byte) float64 {
	var width float64
	step := 1
	if f.unicode {
		step = 2
	}
	for len(b) > 0 {
		size := step
		if f.mapping != nil && len(f.mapping.spaces) > 0 {
			for _, s := range f.mapping.spaces {
				if len(b) >= s.size {
					v := codeNumber(b[:s.size])
					if v >= s.low && v <= s.high {
						size = s.size
						break
					}
				}
			}
		}
		if size > len(b) {
			break
		}
		code := codeNumber(b[:size])
		b = b[size:]
		w, ok := f.widths[code]
		if !ok {
			w = 500
		}
		width += w
	}
	return width
}

func (f *pdfFont) decode(b []byte) (string, error) {
	if f.mapping != nil {
		return f.mapping.decode(b)
	}
	if f.unicode {
		return utf16Text(b)
	}
	var s strings.Builder
	for _, c := range b {
		name := f.encoding[c]
		if name == "" {
			return "", fmt.Errorf("%w: font has no glyph mapping for byte %02x", ErrUnsupported, c)
		}
		v, ok := glyphsnames.GlyphToRune(name)
		if !ok {
			return "", fmt.Errorf("%w: glyph %q", ErrUnsupported, name)
		}
		s.WriteRune(v)
	}
	return s.String(), nil
}
func (r *pdfReader) font(resources types.Dict, name string) (*pdfFont, error) {
	fonts, err := r.pdf.DereferenceDict(resources["Font"])
	if err != nil {
		return nil, err
	}
	obj := fonts[name]
	if obj == nil {
		return nil, fmt.Errorf("missing PDF font %q", name)
	}
	// The indirect font object is authoritative, not the page-local resource name.
	key := fmt.Sprint(obj)
	if cached := r.fonts[key]; cached != nil {
		return cached, nil
	}
	d, err := r.pdf.DereferenceDict(obj)
	if err != nil || d == nil {
		return nil, errors.New("invalid PDF font dictionary")
	}
	f := &pdfFont{encoding: simpleencodings.AdobeStandard, widths: map[uint32]float64{}}
	if widths, e := r.pdf.DereferenceArray(d["Widths"]); e != nil {
		return nil, e
	} else {
		first := 0
		if v := d.IntEntry("FirstChar"); v != nil {
			first = *v
		}
		for i, w := range widths {
			n, e := pdfNumber(w)
			if e != nil {
				return nil, e
			}
			f.widths[uint32(first+i)] = n
		}
	}
	base := d.NameEntry("BaseFont")
	if base != nil {
		lower := strings.ToLower(*base)
		f.bold = strings.Contains(lower, "bold")
		f.italic = strings.Contains(lower, "italic") || strings.Contains(lower, "oblique")
		if *base == "Symbol" {
			f.encoding = simpleencodings.Symbol
		}
		if *base == "ZapfDingbats" {
			f.encoding = simpleencodings.ZapfDingbats
		}
	}
	if o := d["ToUnicode"]; o != nil {
		sd, _, err := r.pdf.DereferenceStreamDict(o)
		if err != nil {
			return nil, err
		}
		if sd == nil {
			return nil, errors.New("invalid ToUnicode stream")
		}
		if err := sd.Decode(); err != nil {
			return nil, err
		}
		f.mapping, err = parseUnicodeMap(r.ctx, sd.Content)
		if err != nil {
			return nil, fmt.Errorf("ToUnicode: %w", err)
		}
		r.fonts[key] = f
		return f, nil
	}
	enc, err := r.pdf.Dereference(d["Encoding"])
	if err != nil {
		return nil, err
	}
	selectEncoding := func(name string) error {
		switch name {
		case "WinAnsiEncoding":
			f.encoding = simpleencodings.WinAnsi
		case "MacRomanEncoding":
			f.encoding = simpleencodings.MacRoman
		case "MacExpertEncoding":
			f.encoding = simpleencodings.MacExpert
		case "StandardEncoding":
			f.encoding = simpleencodings.AdobeStandard
		case "UniGB-UCS2-H", "UniGB-UCS2-V", "UniJIS-UCS2-H", "UniJIS-UCS2-V", "UniCNS-UCS2-H", "UniKS-UCS2-H":
			f.unicode = true
		default:
			return fmt.Errorf("%w: PDF font encoding %q without ToUnicode", ErrUnsupported, name)
		}
		return nil
	}
	switch e := enc.(type) {
	case types.Name:
		if err := selectEncoding(string(e)); err != nil {
			return nil, err
		}
	case types.Dict:
		if b := e.NameEntry("BaseEncoding"); b != nil {
			if err := selectEncoding(*b); err != nil {
				return nil, err
			}
		}
		if arr, err := r.pdf.DereferenceArray(e["Differences"]); err != nil {
			return nil, err
		} else {
			code := 0
			for _, o := range arr {
				switch v := o.(type) {
				case types.Integer:
					code = int(v)
				case types.Name:
					if code < 0 || code > 255 {
						return nil, errors.New("font difference outside byte range")
					}
					f.encoding[code] = string(v)
					code++
				default:
					return nil, errors.New("invalid font difference")
				}
			}
		}
	case nil:
		if subtype := d.NameEntry("Subtype"); subtype != nil && *subtype == "Type0" {
			return nil, fmt.Errorf("%w: composite PDF font without Unicode mapping", ErrUnsupported)
		}
	default:
		return nil, errors.New("invalid PDF font encoding")
	}
	r.fonts[key] = f
	return f, nil
}
