package manydoc

import (
	"bytes"
	"fmt"
	"unicode/utf8"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

func (r *pdfReader) actualFrame() *pdfMarkedContent {
	for i := range r.marked {
		if r.marked[i].actual != nil {
			return &r.marked[i]
		}
	}
	return nil
}
func (r *pdfReader) applyMarked(source *SourceInfo) {
	if len(r.marked) > 0 {
		source.Tag = r.marked[len(r.marked)-1].tag
	}
	for i := len(r.marked) - 1; i >= 0; i-- {
		if r.marked[i].mcid != nil {
			n := *r.marked[i].mcid
			source.MCID = &n
			source.Stream = r.marked[i].stream
			break
		}
	}
}
func (r *pdfReader) beginMarked(tag string, value any, resources types.Dict) error {
	frame := pdfMarkedContent{tag: tag, stream: r.stream}
	if value != nil {
		var d types.Dict
		if name, ok := value.(types.Name); ok {
			props, e := r.pdf.DereferenceDict(resources["Properties"])
			if e != nil {
				return e
			}
			if props[string(name)] == nil {
				return fmt.Errorf("missing marked-content property %s", name)
			}
			var err error
			d, err = r.pdf.DereferenceDict(props[string(name)])
			if err != nil {
				return err
			}
		} else {
			var ok bool
			d, ok = value.(types.Dict)
			if !ok {
				return fmt.Errorf("invalid marked-content properties")
			}
		}
		if v := d["MCID"]; v != nil {
			n, e := r.pdf.DereferenceInteger(v)
			if e != nil || n == nil || *n < 0 {
				return fmt.Errorf("invalid MCID")
			}
			i := int(*n)
			frame.mcid = &i
		}
		if v := d["ActualText"]; v != nil {
			text, e := pdfTextString(v)
			if e != nil {
				return e
			}
			frame.actual = &text
		}
	}
	r.marked = append(r.marked, frame)
	return nil
}
func (r *pdfReader) endMarked() {
	frame := r.marked[len(r.marked)-1]
	r.marked = r.marked[:len(r.marked)-1]
	if frame.actual == nil || r.actualFrame() != nil {
		return
	}
	if frame.source == nil {
		// An empty replacement span has no painted geometry, but logical text still persists.
		r.order++
		frame.source = &SourceInfo{Format: PDF, Page: r.page, Object: r.stream, Order: r.order, Structure: "unstructured", Evidence: "pdf_actualtext", Confidence: 1}
		r.applyMarked(frame.source)
		if frame.mcid != nil {
			frame.source.MCID = frame.mcid
			frame.source.Stream = frame.stream
		}
		frame.source.Tag = frame.tag
	}
	if len(frame.source.Regions) > 0 {
		var bounds Quad
		for i, region := range frame.source.Regions {
			x0, y0, x1, y1 := quadBounds(region.Quad)
			if i > 0 {
				a, b, c, d := quadBounds(bounds)
				x0 = min(x0, a)
				y0 = min(y0, b)
				x1 = max(x1, c)
				y1 = max(y1, d)
			}
			bounds = Quad{{x0, y0}, {x1, y0}, {x1, y1}, {x0, y1}}
		}
		frame.source.Quad = &bounds
		frame.source.Matrix = nil
		frame.source.Baseline = nil
		if !frame.noncollinear && frame.baseline != nil {
			frame.source.Baseline = frame.baseline
		}
		if frame.mixedFont {
			frame.source.Font = ""
			frame.source.FontSize = 0
		}
		frame.source.Precision = "replacement_span_bounds"
	}
	if *frame.actual != "" {
		r.current = append(r.current, Run{Text: *frame.actual, Source: frame.source, Bold: frame.bold, Italic: frame.italic})
	}
}

// PDF text strings use PDFDocEncoding unless a Unicode BOM selects UTF-16BE
// (or UTF-8 in PDF 2.0). This is distinct from font-encoded content strings.
// Published reference: ISO 32000-1, 7.9.2.2 and Annex D, Table D.2.
func pdfTextString(object any) (string, error) {
	b, err := pdfString(object)
	if err != nil {
		return "", err
	}
	if bytes.HasPrefix(b, []byte{0xfe, 0xff}) {
		return utf16Text(b[2:])
	}
	if bytes.HasPrefix(b, []byte{0xef, 0xbb, 0xbf}) {
		if !utf8.Valid(b[3:]) {
			return "", fmt.Errorf("invalid UTF-8 PDF text string")
		}
		return string(b[3:]), nil
	}
	accents := []rune("˘ˇˆ˙˝˛˚˜")
	symbols := []rune("•†‡…—–ƒ⁄‹›−‰„“”‘’‚™ﬁﬂŁŒŠŸŽıłœšž")
	out := make([]rune, 0, len(b))
	for _, c := range b {
		v := rune(c)
		switch {
		case c >= 24 && c <= 31:
			v = accents[int(c)-24]
		case c >= 128 && c <= 158:
			v = symbols[int(c)-128]
		case c == 160:
			v = '€'
		case c == 127 || c == 159 || c == 173:
			return "", fmt.Errorf("undefined PDFDocEncoding byte %02x", c)
		}
		out = append(out, v)
	}
	return string(out), nil
}
