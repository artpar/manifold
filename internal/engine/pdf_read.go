package engine

import (
	"bytes"
	"compress/zlib"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/benoitkugler/textlayout/fonts/glyphsnames"
	"github.com/benoitkugler/textlayout/fonts/simpleencodings"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
	"io"
	"math"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"
)

func matrixPDF(a types.Array) (Matrix, error) {
	if len(a) != 6 {
		return Matrix{}, fmt.Errorf("PDF matrix requires six numbers")
	}
	var m Matrix
	for i, v := range a {
		n, err := pdfNumber(v)
		if err != nil || !finite(n) {
			return Matrix{}, fmt.Errorf("invalid PDF matrix")
		}
		m[i] = n
	}
	return m, nil
}
func pageTransform(media, crop [4]float64, rotate int, unit float64) (Matrix, float64, float64, error) {
	x0, y0, x1, y1 := math.Max(media[0], crop[0]), math.Max(media[1], crop[1]), math.Min(media[2], crop[2]), math.Min(media[3], crop[3])
	w, h := (x1-x0)*unit, (y1-y0)*unit
	if !finite(w) || !finite(h) || w <= 0 || h <= 0 || !finite(unit) || unit <= 0 {
		return Matrix{}, 0, 0, fmt.Errorf("invalid PDF page box or user unit")
	}
	switch rotate {
	case 0:
		return Matrix{unit, 0, 0, -unit, -unit * x0, unit * y1}, w, h, nil
	case 90:
		return Matrix{0, unit, unit, 0, -unit * y0, -unit * x0}, h, w, nil
	case 180:
		return Matrix{-unit, 0, 0, unit, unit * x1, -unit * y0}, w, h, nil
	case 270:
		return Matrix{0, -unit, -unit, 0, unit * y1, unit * x1}, h, w, nil
	}
	return Matrix{}, 0, 0, fmt.Errorf("unsupported PDF page rotation %d", rotate)
}
func (r *pdfReader) pageGeometry(number int, d types.Dict, attrs *model.InheritedPageAttrs) (PDFPage, error) {
	if attrs.MediaBox == nil {
		return PDFPage{}, fmt.Errorf("PDF page missing MediaBox")
	}
	rect := func(b *types.Rectangle) [4]float64 { return [4]float64{b.LL.X, b.LL.Y, b.UR.X, b.UR.Y} }
	p := PDFPage{Number: number, MediaBox: rect(attrs.MediaBox), Rotate: ((attrs.Rotate % 360) + 360) % 360, UserUnit: 1}
	p.CropBox = p.MediaBox
	if attrs.CropBox != nil {
		p.CropBox = rect(attrs.CropBox)
	}
	if value := d["UserUnit"]; value != nil {
		n, err := r.pdf.DereferenceNumber(value)
		if err != nil {
			return p, err
		}
		p.UserUnit = n
	}
	var err error
	p.Transform, p.Width, p.Height, err = pageTransform(p.MediaBox, p.CropBox, p.Rotate, p.UserUnit)
	return p, err
}
func initialPDFState() pdfTextState {
	return pdfTextState{ctm: identityMatrix(), tm: identityMatrix(), lm: identityMatrix(), hscale: 1, rulingWidth: 1, rulingStrokeAlpha: 1, rulingFillAlpha: 1, fillSpace: "DeviceGray", strokeSpace: "DeviceGray"}
}
func (s *pdfTextState) advanceText(dx float64) { s.tm = s.tm.compose(translation(dx, 0)) }
func (s *pdfTextState) moveLine(dx, dy float64) {
	s.lm = s.lm.compose(translation(dx, dy))
	s.tm = s.lm
	s.metricEstimate = false
}
func (r *pdfReader) sourceFor(s *pdfTextState, advance float64, estimated bool) *SourceInfo {
	base := r.pageInfo.Transform.compose(s.ctm).compose(s.tm)
	q := Quad{base.point(0, s.rise+s.font.ascent*s.size/1000), base.point(advance, s.rise+s.font.ascent*s.size/1000), base.point(advance, s.rise+s.font.descent*s.size/1000), base.point(0, s.rise+s.font.descent*s.size/1000)}
	baseline := [2]Point{base.point(0, s.rise), base.point(advance, s.rise)}
	matrix := base.compose(Matrix{s.size * s.hscale, 0, 0, s.size, 0, s.rise})
	precision := "font_metrics"
	confidence := 1.0
	if s.font.boundsEstimate {
		precision = "estimated_font_bounds"
		confidence = .8
	}
	if estimated || s.metricEstimate {
		precision = "estimated_advance"
		confidence = .5
	}
	r.order++
	source := &SourceInfo{Format: PDF, Page: r.page, Order: r.order, Object: r.stream, Stream: r.stream, Quad: &q, Baseline: &baseline, Matrix: &matrix, Font: s.font.name, FontSize: math.Hypot(matrix[2], matrix[3]), Structure: "unstructured", Evidence: "pdf_text_operators", Precision: precision, Confidence: confidence, RenderMode: s.renderMode}
	r.clipTextSource(source, s)
	r.applyMarked(source)
	return source
}
func (r *pdfReader) warnPage(code, message string) {
	diagnostic := Diagnostic{code, "PDF page " + strconv.Itoa(r.page) + ": " + message}
	if r.pageWarnings == nil {
		r.pageWarnings = map[Diagnostic]bool{}
		for _, d := range r.doc.Diagnostics {
			r.pageWarnings[d] = true
		}
	}
	if !r.pageWarnings[diagnostic] {
		r.pageWarnings[diagnostic] = true
		r.doc.Diagnostics = append(r.doc.Diagnostics, diagnostic)
	}
}
func sourceForRuns(runs []Run) *SourceInfo {
	var first *SourceInfo
	var bounds Quad
	have := false
	for _, run := range runs {
		s := run.Source
		if s == nil || s.Quad == nil {
			continue
		}
		if first == nil {
			copy := *s
			copy.Matrix = nil
			copy.Baseline = nil
			copy.Font = ""
			copy.FontSize = 0
			copy.FontFamilyEvidence = ""
			copy.ColorSpace = ""
			copy.ColorComponents = nil
			copy.MCID = nil
			copy.Tag = ""
			copy.Structure = "inferred"
			copy.Evidence = "baseline_line_group"
			copy.Confidence = .7
			copy.Precision = "union_of_run_bounds"
			first = &copy
		}
		first.Artifacts = artifactIDs(first, s.Artifacts...)
		x0, y0, x1, y1 := quadBounds(*s.Quad)
		if have {
			a, b, c, d := quadBounds(bounds)
			x0 = math.Min(x0, a)
			y0 = math.Min(y0, b)
			x1 = math.Max(x1, c)
			y1 = math.Max(y1, d)
		}
		bounds = Quad{{x0, y0}, {x1, y0}, {x1, y1}, {x0, y1}}
		have = true
	}
	if first != nil {
		first.Quad = &bounds
	}
	return first
}

type pdfMarkedContent struct {
	artifact                         string
	decoded                          []byte
	fragments                        []Run
	alignmentUnavailable             bool
	clipState                        uint8
	tag                              string
	stream                           string
	mcid                             *int
	actual                           *string
	source                           *SourceInfo
	haveStyle, mixedFont, styleMixed bool
	style                            Run
	baseline                         *[2]Point
	noncollinear                     bool
}

func matrixOperands(a []any) (Matrix, error) {
	var m Matrix
	if len(a) != 6 {
		return m, fmt.Errorf("PDF matrix requires six operands")
	}
	for i, v := range a {
		n, err := pdfNumber(v)
		if err != nil || !finite(n) {
			return m, fmt.Errorf("invalid PDF matrix")
		}
		m[i] = n
	}
	return m, nil
}

// Annotation appearances map their transformed bounding box into the widget Rect.

// JPEG entropy data may contain escaped FF bytes and restart markers. Segment
// lengths, rather than an EI byte search, identify the actual image boundary.

// ReadPDF recovers positioned text/images and supported tagged logical structure.
// Untagged structure is inferred from geometry with explicit uncertainty. OCR is
// not provided by this reader.

// A dependency parser's panic must never escape this public boundary or
// yield partially extracted content as a successful document.

// Accept version-extension conventions during object parsing, but never
// accept partially decoded corrupt page streams as successful extraction.

func (r *pdfReader) appearance(o types.Object, annotation, resources types.Dict) error {
	sd, _, err := r.pdf.DereferenceStreamDict(o)
	if err != nil {
		return err
	}
	if sd == nil {
		return fmt.Errorf("annotation appearance is not a stream")
	}
	rect, err := r.pdf.DereferenceArray(annotation["Rect"])
	if err != nil {
		return err
	}
	box, err := r.pdf.DereferenceArray(sd.Dict["BBox"])
	if err != nil {
		return err
	}
	if len(rect) != 4 || len(box) != 4 {
		return fmt.Errorf("annotation appearance requires Rect and BBox")
	}
	var rr, bb [4]float64
	for i := range rr {
		rr[i], err = pdfNumber(rect[i])
		if err != nil {
			return err
		}
		bb[i], err = pdfNumber(box[i])
		if err != nil {
			return err
		}
	}
	m := identityMatrix()
	if v := sd.Dict["Matrix"]; v != nil {
		a, e := r.pdf.DereferenceArray(v)
		if e != nil {
			return e
		}
		m, err = matrixPDF(a)
		if err != nil {
			return err
		}
	}
	q := Quad{m.point(bb[0], bb[1]), m.point(bb[2], bb[1]), m.point(bb[2], bb[3]), m.point(bb[0], bb[3])}
	x0, y0, x1, y1 := quadBounds(q)
	if x1 <= x0 || y1 <= y0 {
		return fmt.Errorf("empty annotation appearance box")
	}
	if rr[2] <= rr[0] || rr[3] <= rr[1] {
		return fmt.Errorf("empty annotation rectangle")
	}
	sx, sy := (rr[2]-rr[0])/(x1-x0), (rr[3]-rr[1])/(y1-y0)
	state := initialPDFState()
	state.ctm = Matrix{sx, 0, 0, sy, rr[0] - sx*x0, rr[1] - sy*y0}
	return r.form(o, resources, 0, state)
}

func (r *pdfReader) checkPageBounds(q Quad) {
	x0, y0, x1, y1 := quadBounds(q)
	if x0 < -.01 || y0 < -.01 || x1 > r.pageInfo.Width+.01 || y1 > r.pageInfo.Height+.01 {
		r.warnPage("pdf_page_crop", "Recovered font/image bounds extend outside the displayed crop box; geometry/content are retained without clipping")
	}
}

func (r *pdfReader) objectID(object types.Object, kind string) string {
	switch v := object.(type) {
	case types.IndirectRef:
		return fmt.Sprint(v)
	case types.Name:
		return string(v)
	}
	return fmt.Sprintf("%s:page:%d:paint:%d", kind, r.page, r.order+1)
}

var inlineKeys = map[string]string{"W": "Width", "H": "Height", "BPC": "BitsPerComponent", "CS": "ColorSpace", "D": "Decode", "DP": "DecodeParms", "F": "Filter", "IM": "ImageMask", "I": "Interpolate"}
var inlineNames = map[string]string{"G": "DeviceGray", "RGB": "DeviceRGB", "CMYK": "DeviceCMYK", "I": "Indexed", "AHx": "ASCIIHexDecode", "A85": "ASCII85Decode", "LZW": "LZWDecode", "Fl": "FlateDecode", "RL": "RunLengthDecode", "CCF": "CCITTFaxDecode", "DCT": "DCTDecode"}

func expandInlineName(v types.Object) types.Object {
	if n, ok := v.(types.Name); ok {
		if full, yes := inlineNames[string(n)]; yes {
			return types.Name(full)
		}
	}
	return v
}
func (r *pdfReader) inlineImage(input *string, resources types.Dict, state pdfTextState) error {
	d := types.Dict{"Type": types.Name("XObject"), "Subtype": types.Name("Image")}
	for {
		key, e := nextPDFToken(r.ctx, input)
		if e != nil {
			return e
		}
		if key == pdfOperator("ID") {
			break
		}
		name, e := pdfName(key)
		if e != nil {
			return fmt.Errorf("invalid inline-image key: %w", e)
		}
		if full, ok := inlineKeys[name]; ok {
			name = full
		}
		if _, exists := d[name]; exists {
			return fmt.Errorf("duplicate inline-image key %s", name)
		}
		value, e := nextPDFToken(r.ctx, input)
		if e != nil {
			return e
		}
		obj, ok := value.(types.Object)
		if !ok || obj == nil {
			return fmt.Errorf("invalid inline-image value")
		}
		if name == "ColorSpace" || name == "Filter" {
			if arr, ok := obj.(types.Array); ok {
				for i, v := range arr {
					arr[i] = expandInlineName(v)
				}
			} else {
				obj = expandInlineName(obj)
			}
		}
		d[name] = obj
	}
	if len(*input) == 0 || !pdfSpace((*input)[0]) {
		return fmt.Errorf("inline-image ID requires whitespace")
	}
	if strings.HasPrefix(*input, "\r\n") {
		*input = (*input)[2:]
	} else {
		*input = (*input)[1:]
	}
	sd := types.StreamDict{Dict: d}
	w, h, e := r.imageDimensions(&sd)
	if e != nil {
		return e
	}
	filters := types.Array{}
	if f := d["Filter"]; f != nil {
		if a, ok := f.(types.Array); ok {
			filters = a
		} else {
			filters = append(filters, f)
		}
	}
	parms := types.Array{}
	if p := d["DecodeParms"]; p != nil {
		if a, ok := p.(types.Array); ok {
			parms = a
		} else {
			parms = append(parms, p)
		}
	}
	if len(parms) > 0 && len(parms) != len(filters) {
		return fmt.Errorf("inline-image DecodeParms length mismatch")
	}
	for i, f := range filters {
		name, e := pdfName(f)
		if e != nil {
			return e
		}
		var dp types.Dict
		if len(parms) > 0 && parms[i] != nil {
			var ok bool
			dp, ok = parms[i].(types.Dict)
			if !ok {
				return fmt.Errorf("invalid inline-image DecodeParms")
			}
		}
		sd.FilterPipeline = append(sd.FilterPipeline, types.PDFFilter{Name: name, DecodeParms: dp})
	}
	n := 0
	data := []byte(*input)
	if len(filters) == 0 {
		components := 0
		switch d["ColorSpace"] {
		case types.Name("DeviceGray"):
			components = 1
		case types.Name("DeviceRGB"):
			components = 3
		case types.Name("DeviceCMYK"):
			components = 4
		}
		if a, ok := d["ColorSpace"].(types.Array); ok && len(a) > 0 && a[0] == types.Name("Indexed") {
			components = 1
		}
		bpc, e := r.pdf.DereferenceInteger(d["BitsPerComponent"])
		if e != nil {
			return e
		}
		bits := 0
		if bpc != nil {
			bits = int(*bpc)
		}
		if mask := d.BooleanEntry("ImageMask"); mask != nil && *mask {
			components = 1
			bits = 1
		}
		if components == 0 || (bits != 1 && bits != 2 && bits != 4 && bits != 8 && bits != 16) {
			return fmt.Errorf("%w: inline-image raw sample format", ErrUnsupported)
		}
		n = ((w*components*bits + 7) / 8) * h
	} else {
		switch sd.FilterPipeline[0].Name {
		case "FlateDecode":
			reader := bytes.NewReader(data)
			z, e := zlib.NewReader(reader)
			if e != nil {
				return e
			}
			count, e := io.Copy(io.Discard, io.LimitReader(z, 200_000_001))
			closeErr := z.Close()
			if e != nil {
				return e
			}
			if closeErr != nil {
				return closeErr
			}
			if count > 200_000_000 {
				return fmt.Errorf("inline-image decoded size exceeds limit")
			}
			n = len(data) - reader.Len()
		case "ASCIIHexDecode":
			n = strings.IndexByte(*input, '>') + 1
		case "ASCII85Decode":
			if end := strings.Index(*input, "~>"); end >= 0 {
				n = end + 2
			}
		case "RunLengthDecode":
			terminated := false
			for n < len(data) {
				b := data[n]
				n++
				if b == 128 {
					terminated = true
					break
				}
				if b < 128 {
					n += int(b) + 1
				} else {
					n++
				}
				if n > len(data) {
					return fmt.Errorf("truncated inline RLE image")
				}
			}
			if !terminated {
				return fmt.Errorf("unterminated inline RLE image")
			}
		case "DCTDecode":
			var e error
			n, e = inlineJPEGLength(data)
			if e != nil {
				return e
			}
		default:
			return fmt.Errorf("%w: inline-image boundary for filter %s", ErrUnsupported, sd.FilterPipeline[0].Name)
		}
	}
	if n <= 0 || n > len(data) {
		return fmt.Errorf("truncated inline-image data")
	}
	sd.Raw = data[:n]
	length := int64(n)
	sd.StreamLength = &length
	sd.Dict["Length"] = types.Integer(n)
	*input = (*input)[n:]
	if len(*input) == 0 || !pdfSpace((*input)[0]) {
		return fmt.Errorf("inline image missing EI delimiter")
	}
	end, e := nextPDFToken(r.ctx, input)
	if e != nil {
		return e
	}
	if end != pdfOperator("EI") {
		return fmt.Errorf("inline image missing EI")
	}
	return r.imageXObject(&sd, types.Name(fmt.Sprintf("inline:%s:%d", r.stream, r.order+1)), resources, state)
}

func inlineJPEGLength(data []byte) (int, error) {
	if len(data) < 2 || data[0] != 255 || data[1] != 216 {
		return 0, fmt.Errorf("invalid inline JPEG SOI")
	}
	for i := 2; i < len(data); {
		if data[i] != 255 {
			i++
			continue
		}
		i++
		for i < len(data) && data[i] == 255 {
			i++
		}
		if i >= len(data) {
			break
		}
		marker := data[i]
		i++
		if marker == 0 || marker == 1 || (marker >= 208 && marker <= 215) {
			continue
		}
		if marker == 217 {
			return i, nil
		}
		if i+2 > len(data) {
			break
		}
		length := int(data[i])<<8 | int(data[i+1])
		if length < 2 || length > len(data)-i {
			return 0, fmt.Errorf("invalid inline JPEG segment length")
		}
		i += length
	}
	return 0, fmt.Errorf("inline JPEG missing EOI")
}

func ReadPDF(ctx context.Context, src io.Reader, opts ReadOptions) (doc *Document, err error) {

	defer func() {
		if p := recover(); p != nil {
			doc = nil
			err = fmt.Errorf("malformed PDF: %v", p)
		}
	}()

	limits, err := resolvePDFReadLimits(opts.PDFLimits)
	if err != nil {
		return nil, err
	}
	p, err := readPDFSource(ctx, src, opts.Password)
	if err != nil {
		return nil, err
	}

	p.XRefTable.ValidationMode = model.ValidationStrict
	r := &pdfReader{ctx: ctx, pdf: p, skipNoteKindInference: opts.PDFSkipInference, limits: limits}
	if err := r.validateParserStreams(); err != nil {
		return nil, err
	}
	if err = p.EnsurePageCount(); err != nil {
		return nil, err
	}
	if p.PageCount < 0 {
		return nil, errors.New("negative PDF page count")
	}
	doc = &Document{Metadata: Metadata{Title: p.Title, Author: p.Author, Subject: p.Subject, PageCount: p.PageCount}}
	if p.Info != nil {
		info, e := p.DereferenceDict(*p.Info)
		if e != nil {
			return nil, e
		}
		for _, entry := range []struct {
			key    string
			target *string
		}{{"Title", &doc.Metadata.Title}, {"Author", &doc.Metadata.Author}, {"Subject", &doc.Metadata.Subject}} {
			if value := info[entry.key]; value != nil {
				v, e := p.Dereference(value)
				if e != nil {
					return nil, e
				}
				text, e := pdfTextString(v)
				if e != nil {
					return nil, fmt.Errorf("PDF metadata %s: %w", entry.key, e)
				}
				*entry.target = text
			}
		}
		for _, key := range []string{"Keywords", "Creator", "Producer", "CreationDate", "ModDate"} {
			if info[key] != nil {
				doc.warn("pdf_metadata_extra", "Additional PDF producer/date/keyword metadata is not yet represented")
			}
		}
	}
	doc.warn("pdf_layout", "PDF source geometry is retained; recovered semantic structure and reflowed output do not reproduce the original visual layout")
	catalog, e := p.Catalog()
	if e != nil {
		return nil, e
	}
	if catalog["Metadata"] != nil {
		doc.warn("pdf_xmp_metadata", "XMP metadata is not merged into the editable metadata model")
	}
	r.doc = doc
	r.pageRefs = map[string]int{}
	r.objects = map[pdfObjectKey][]pdfPaintRange{}
	r.links = map[pdfObjectKey]pdfRecoveredLink{}
	r.partialLinks = map[int]bool{}
	for page := 1; page <= p.PageCount; page++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		d, pageRef, attrs, e := p.PageDict(page, false)
		if e != nil {
			return nil, fmt.Errorf("PDF page %d: %w", page, e)
		}
		if pageRef != nil {
			r.pageRefs[fmt.Sprint(*pageRef)] = page
		}
		geometry, e := r.pageGeometry(page, d, attrs)
		if e != nil {
			return nil, e
		}
		doc.Pages = append(doc.Pages, geometry)
		r.page = page
		r.pageInfo = geometry
		r.stream = fmt.Sprintf("page:%d", page)
		data, e := r.pageContent(d)
		if e != nil {
			return nil, e
		}
		r.lines = nil
		r.current = nil
		r.fonts = map[string]*pdfFont{}
		if e = r.content(data, attrs.Resources, 0, initialPDFState()); e != nil {
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
				if e := r.readLink(a, ad); e != nil {
					return nil, e
				}
				ap, e := p.DereferenceDict(ad["AP"])
				if e != nil {
					return nil, e
				}
				if ap == nil {
					continue
				}
				appearanceObject := ap["N"]
				o, e := p.Dereference(appearanceObject)
				if e != nil {
					return nil, e
				}
				if states, ok := o.(types.Dict); ok {
					state := ad.NameEntry("AS")
					if state != nil {
						o = states[*state]
						appearanceObject = o
					} else {
						r.doc.warn("annotation_state", "Annotation has multiple appearances without a selected state")
						continue
					}
				}
				if o != nil {
					start := r.order
					if e = r.appearance(appearanceObject, ad, attrs.Resources); e != nil {
						return nil, e
					}
					r.recordObject(a, start, r.order)
				}
			}
		}
		r.newline()
		if page > 1 {
			doc.Blocks = append(doc.Blocks, Block{Kind: PageBreak})
		}
		doc.Blocks = append(doc.Blocks, r.lines...)
	}
	if e := r.recoverPDFNavigation(catalog); e != nil {
		return nil, fmt.Errorf("PDF navigation: %w", e)
	}
	// No parser objects are needed for geometric inference of an untagged
	// document. Drop them before building the editable model, not afterwards.
	if catalog["StructTreeRoot"] == nil || opts.PDFSkipStructure {
		r.pdf = nil
		p = nil
		r.fonts = nil
		r.streamBudget = nil
		r.objects = nil
		r.lines = nil
		r.current = nil
	}
	if e := r.geometricLinks(); e != nil {
		return nil, e
	}
	if (catalog["StructTreeRoot"] == nil || opts.PDFSkipStructure) && !opts.PDFSkipInference {
		if e := r.inferStructure(); e != nil {
			return nil, fmt.Errorf("PDF inferred structure: %w", e)
		}
	}
	if !opts.PDFSkipStructure {
		if e := r.recoverStructure(catalog["StructTreeRoot"]); e != nil {
			return nil, fmt.Errorf("PDF logical structure: %w", e)
		}
	} else if catalog["StructTreeRoot"] != nil {
		doc.warn("pdf_structure_skipped", "Logical PDF structure was explicitly skipped; positioned paint content is retained")
	}
	r.pdf = nil
	p = nil
	r.fonts = nil
	r.streamBudget = nil
	r.objects = nil
	r.lines = nil
	r.current = nil
	r.diagnosePartialLinks()
	if !opts.PDFSkipInference {
		if err := r.inferLinkedPDFNotes(); err != nil {
			return nil, err
		}
	}
	r.bindPDFNavigation()
	if strings.TrimSpace(strings.ReplaceAll(doc.Text(), "\f", "")) == "" {
		doc.warn("no_text", "No extractable text was found; scanned/image-only documents require OCR")
	}
	if err := doc.Validate(); err != nil {
		return nil, fmt.Errorf("PDF recovered model: %w", err)
	}
	return doc, nil
}

type pdfReader struct {
	pageWarnings             map[Diagnostic]bool
	limits                   PDFReadLimits
	skipNoteKindInference    bool
	noteInferenceWork        int
	streamBudget             *pdfStreamBudget
	rasterPixels             int64
	contentBytes             int
	xobjectTotal             int
	artifactTotal            int
	logicalBidiLine          bool
	ctx                      context.Context
	pdf                      *model.Context
	doc                      *Document
	fonts                    map[string]*pdfFont
	lines                    []Block
	current                  []Run
	havePosition             bool
	page                     int
	pageInfo                 PDFPage
	stream                   string
	order                    int
	marked                   []pdfMarkedContent
	lastPoint, lastDirection Point
	lastSize                 float64
	pageRefs                 map[string]int
	objects                  map[pdfObjectKey][]pdfPaintRange
	links                    map[pdfObjectKey]pdfRecoveredLink
	partialLinks             map[int]bool
	rulings                  map[int][]pdfRuling
	rulingPatches            map[int][]pdfRulingRect
	rulingOverflow           map[int]bool
}
type pdfTextState struct {
	rulingMask, rulingBlend, rulingTransfer           bool
	font                                              *pdfFont
	size, leading, wordSpace, charSpace, hscale, rise float64
	ctm, tm, lm                                       Matrix
	renderMode                                        int
	metricEstimate                                    bool
	fill                                              [3]float64
	fillUnknown                                       bool
	fillSpace, strokeSpace                            string
	fillValues, strokeValues                          [4]float64
	strokeUnknown                                     bool
	rulingWidth                                       float64
	rulingStrokeAlpha, rulingFillAlpha                float64
	rulingClip                                        *pdfRulingRect
	rulingClipUnknown, rulingEffects                  bool
}

func (r *pdfReader) newline() {
	r.logicalBidiLine = false
	r.havePosition = false
	if len(r.current) > 0 {
		r.lines = append(r.lines, Block{Kind: Paragraph, Runs: r.current, Source: sourceForRuns(r.current)})
		r.current = nil
	}
}
func (r *pdfReader) space(next Run) {
	if len(r.current) == 0 {
		return
	}
	last := &r.current[len(r.current)-1]
	lastRune, _ := utf8.DecodeLastRuneInString(last.Text)
	if last.Text != "" && !unicode.IsSpace(lastRune) {
		copyRun, _ := commonTextStyle(*last, next)
		copyRun.Text = " "
		copyRun.Note = nil
		if last.Source != nil {
			source := *last.Source
			source.Artifacts = commonArtifacts(last.Source, next.Source)
			source.Font = ""
			source.FontSize = 0
			source.RenderMode = 0
			source.FontFamilyEvidence = ""
			source.ColorSpace = ""
			source.ColorComponents = nil
			source.Quad = nil
			source.Matrix = nil
			source.Baseline = nil
			source.Regions = nil
			source.MCID = nil
			source.Tag = ""
			source.Structure = "inferred"
			source.Evidence = "positioned_word_gap"
			source.Confidence = .6
			source.Precision = "synthetic_separator"
			copyRun.Source = &source
		}
		r.current = append(r.current, copyRun)
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
	if s.font.vertical {
		return fmt.Errorf("%w: vertical PDF font positioning", ErrUnsupported)
	}
	text := ""
	actual := r.actualFrame()
	decoded, decodeErr := s.font.decode(b)
	if s.font.fallbackUsed {
		r.warnPage("pdf_cmap_encoding_fallback", "A partial simple-font ToUnicode map is completed using explicit source Encoding glyph names")
		s.font.fallbackUsed = false
	}
	if actual == nil {
		if decodeErr != nil {
			return decodeErr
		}
		text = decoded
	} else if decodeErr != nil {
		actual.alignmentUnavailable = true
	}
	advance, estimated, err := s.font.displacement(b, s.size, s.charSpace, s.wordSpace, s.hscale)
	if err != nil {
		return err
	}
	source := r.sourceFor(s, advance, estimated)
	style := r.textStyle(s, source)
	r.checkPageBounds(*source.Quad)
	if s.renderMode == 3 || s.renderMode == 7 {
		r.warnPage("pdf_invisible_text", "Invisible/clipping-only text is retained with its render mode in source provenance")
	}
	if estimated {
		r.warnPage("pdf_font_metrics", "Text advance estimated because font metrics/code-to-CID mapping are incomplete or ambiguous")
	}
	if s.font.boundsEstimate {
		r.warnPage("pdf_font_bounds", "Font ascent/descent bounds are estimated")
	}
	begin, end := source.Baseline[0], source.Baseline[1]
	direction := Point{end.X - begin.X, end.Y - begin.Y}
	norm := math.Hypot(direction.X, direction.Y)
	if norm == 0 {
		base := r.pageInfo.Transform.compose(s.ctm).compose(s.tm)
		direction = Point{base[0], base[1]}
		norm = math.Hypot(direction.X, direction.Y)
	}
	if norm > 0 {
		direction.X /= norm
		direction.Y /= norm
	}
	// Component glyph backtracking/mark offsets do not establish logical line
	// breaks inside one ActualText replacement.
	logicalEmpty := actual != nil && *actual.actual == ""
	if !logicalEmpty && r.havePosition && (actual == nil || actual.source == nil) {
		cross := r.lastDirection.X*direction.Y - r.lastDirection.Y*direction.X
		dx, dy := begin.X-r.lastPoint.X, begin.Y-r.lastPoint.Y
		perpendicular := math.Abs(dx*r.lastDirection.Y - dy*r.lastDirection.X)
		if r.lastDirection.X*direction.X+r.lastDirection.Y*direction.Y < .95 || math.Abs(cross) > .02 || perpendicular > max(1, source.FontSize*.4) {
			r.newline()
		} else if dx*r.lastDirection.X+dy*r.lastDirection.Y > max(.5, source.FontSize*.15) {
			uncertain := false
			if actual != nil {
				uncertain = pdfHasBidiText(*actual.actual) || r.logicalBidiLine
			}
			if uncertain {
				r.warnPage("pdf_bidi_spacing", "Whitespace is not inferred between adjacent ActualText bidi spans; original logical strings are retained")
			} else {
				r.space(style)
			}
		}
	}
	if actual != nil {
		actual.clipState |= replacementClipState(source.Precision)
		if !actual.haveStyle {
			actual.style = style
			actual.style.Source = nil
			actual.haveStyle = true
			baseline := *source.Baseline
			actual.baseline = &baseline
		} else {
			var mixed bool
			actual.style, mixed = commonTextStyle(actual.style, style)
			actual.styleMixed = actual.styleMixed || mixed
			if !sameSourcePaint(actual.source, source) {
				actual.source.ColorSpace = ""
				actual.source.ColorComponents = nil
			}
			if actual.style.FontFamily == "" {
				actual.source.FontFamilyEvidence = ""
			}
			first, last := actual.baseline[0], actual.baseline[1]
			dx, dy := last.X-first.X, last.Y-first.Y
			length := math.Hypot(dx, dy)
			if length > 1e-8 {
				actual.noncollinear = actual.noncollinear || (math.Abs((begin.X-first.X)*dy-(begin.Y-first.Y)*dx)/length > .01 || math.Abs((end.X-first.X)*dy-(end.Y-first.Y)*dx)/length > .01)
			} else {
				actual.noncollinear = actual.noncollinear || math.Hypot(begin.X-first.X, begin.Y-first.Y) > .01
			}
			actual.baseline[1] = end
			actual.mixedFont = actual.mixedFont || actual.source.Font != source.Font || math.Abs(actual.source.FontSize-source.FontSize) > 1e-7
		}
		if decodeErr == nil {
			part := style
			part.Text = decoded
			part.Source = source
			actual.fragments = append(actual.fragments, part)
			actual.decoded = append(actual.decoded, decoded...)
		}
		if actual.source == nil {
			copy := *source
			copy.Evidence = "pdf_actualtext"
			copy.Regions = nil
			actual.source = &copy
		}
		actual.source.Artifacts = artifactIDs(actual.source, source.Artifacts...)
		actual.source.Confidence = min(actual.source.Confidence, source.Confidence)
		actual.source.Regions = append(actual.source.Regions, SourceRegion{Page: r.page, Quad: *source.Quad})
	}
	if text != "" {
		r.logicalBidiLine = r.logicalBidiLine || pdfHasBidiText(text)
		r.current = append(r.current, Run{Text: text, Bold: style.Bold, Italic: style.Italic, FontFamily: style.FontFamily, FontSize: style.FontSize, Color: style.Color, Source: source})
	}
	s.advanceText(advance)
	s.metricEstimate = s.metricEstimate || estimated
	if !logicalEmpty {
		r.lastPoint = end
		r.lastDirection = direction
		r.lastSize = source.FontSize
		r.havePosition = true
	}
	return nil
}

func (r *pdfReader) content(data []byte, resources types.Dict, depth int, state pdfTextState) error {
	if err := r.ctx.Err(); err != nil {
		return err
	}
	if len(data) > maxPDFDecodedBytes-r.contentBytes {
		return fmt.Errorf("PDF interpreted syntax exceeds %d bytes", maxPDFDecodedBytes)
	}
	r.contentBytes += len(data)
	spaces, err := r.pdf.DereferenceDict(resources["ColorSpace"])
	if err != nil {
		return err
	}
	for _, key := range []string{"DefaultGray", "DefaultRGB", "DefaultCMYK"} {
		if spaces[key] != nil {
			r.warnPage("pdf_default_color_space", "Default device color-space overrides are not resolved; editable RGB uses base device components as an approximation")
		}
	}
	if depth > 32 {
		return errors.New("cyclic or excessively nested PDF Form XObject")
	}
	input := string(data)
	var args []any
	var saved []pdfTextState
	var path pdfCurrentPath
	markedBase := len(r.marked)
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
			state.tm = identityMatrix()
			state.lm = identityMatrix()
			state.metricEstimate = false
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
					state.advanceText(-n * state.size * state.hscale / 1000)
				}
			}
		case "'":
			if err := need(1); err != nil {
				return err
			}
			r.newline()
			state.moveLine(0, -state.leading)
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
			state.moveLine(0, -state.leading)
			if err := r.show(args[2], &state); err != nil {
				return err
			}
		case "T*":
			if err := need(0); err != nil {
				return err
			}
			r.newline()
			state.moveLine(0, -state.leading)
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
			// A Td immediately after BT positions the first span; its y
			// operand is not proof of a new visual line. show compares the
			// transformed baselines, including across separate text objects.
			state.moveLine(dx, dy)
			if op == "TD" {
				state.leading = -dy
			}
		case "Tm":
			if err := need(6); err != nil {
				return err
			}
			matrix, e := matrixOperands(args)
			if e != nil {
				return e
			}
			state.tm = matrix
			state.lm = matrix
			state.metricEstimate = false
		case "TL", "Tw", "Tc", "Ts", "Tz", "Tr":
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
			case "Ts":
				state.rise = n
			case "Tz":
				state.hscale = n / 100
			case "Tr":
				if n != math.Trunc(n) || n < 0 || n > 7 {
					return fmt.Errorf("invalid text render mode")
				}
				state.renderMode = int(n)

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
			tm, lm, estimate := state.tm, state.lm, state.metricEstimate
			state = saved[len(saved)-1]
			state.tm, state.lm, state.metricEstimate = tm, lm, estimate
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
			if e = r.form(o, resources, depth+1, state); e != nil {
				return e
			}
		case "BI":
			if err := need(0); err != nil {
				return err
			}
			if err := r.inlineImage(&input, resources, state); err != nil {
				return err
			}
		case "cm":
			if err := need(6); err != nil {
				return err
			}
			matrix, err := matrixOperands(args)
			if err != nil {
				return err
			}
			state.ctm = state.ctm.compose(matrix)
		case "BMC", "BDC":
			n := 1
			if op == "BDC" {
				n = 2
			}
			if err := need(n); err != nil {
				return err
			}
			tag, err := pdfName(args[0])
			if err != nil {
				return err
			}
			var props any
			if n == 2 {
				props = args[1]
			}
			if err := r.beginMarked(tag, props, resources); err != nil {
				return err
			}
		case "EMC":
			if err := need(0); err != nil {
				return err
			}
			if len(r.marked) <= markedBase {
				return fmt.Errorf("unbalanced marked-content end")
			}
			r.endMarked()

		case "cs", "CS":
			if err := r.selectTextColorSpace(args, resources, &state, op == "CS"); err != nil {
				return err
			}
		case "sc", "scn", "SC", "SCN":
			if err := setTextColorValues(args, &state, op == "SC" || op == "SCN"); err != nil {
				return err
			}
		case "g", "rg", "k", "G", "RG", "K":
			stroke := op == "G" || op == "RG" || op == "K"
			space := "DeviceGray"
			if op == "rg" || op == "RG" {
				space = "DeviceRGB"
			}
			if op == "k" || op == "K" {
				space = "DeviceCMYK"
			}
			if stroke {
				state.strokeSpace = space
			} else {
				state.fillSpace = space
			}
			if err := setTextColorValues(args, &state, stroke); err != nil {
				return fmt.Errorf("%s: %w", op, err)
			}

		case "m", "l", "c", "v", "y", "h", "re", "n", "S", "s", "f", "F", "f*", "B", "B*", "b", "b*", "W", "W*", "w":
			if err := r.pathOperator(string(op), args, &path, &state); err != nil {
				return err
			}
		case "sh":
			r.warnPage("pdf_vector_graphics", "Painted vector paths/shading are not represented as editable content")
		case "gs":
			if err := r.rulingGraphicsState(args, resources, &state); err != nil {
				return err
			}
			r.warnPage("pdf_graphics_effects", "Extended graphics-state effects are not reconstructed")
		case "J", "j", "M", "d", "ri", "i", "MP", "DP", "BX", "EX", "d0", "d1":
		default:
			return fmt.Errorf("%w: PDF content operator %q", ErrUnsupported, op)
		}
		args = nil
	}
	if len(saved) > 0 {
		return errors.New("unbalanced PDF graphics save")
	}
	if len(r.marked) != markedBase {
		return errors.New("unbalanced PDF marked content")
	}
	if len(args) > 0 {
		return errors.New("PDF content ends with operands without an operator")
	}
	return nil
}

// Count actual invocations, not unique objects or nesting. A shallow acyclic
// shared Form graph can expand exponentially, including through appearances.
// This parser work bound is independent of application admission or quotas.
const maxPDFXObjectVisits = 200000

func (r *pdfReader) form(o types.Object, parent types.Dict, depth int, state pdfTextState) error {
	if err := r.ctx.Err(); err != nil {
		return err
	}
	if r.xobjectTotal >= maxPDFXObjectVisits {
		return fmt.Errorf("PDF XObject expansion exceeds %d visits", maxPDFXObjectVisits)
	}
	r.xobjectTotal++
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
	start := r.order
	defer func() { r.recordObject(o, start, r.order) }()
	if *sub == "Image" {
		return r.imageXObject(sd, o, parent, state)
	}
	if *sub != "Form" {
		return fmt.Errorf("%w: PDF XObject subtype %s", ErrUnsupported, *sub)
	}
	decoded, err := r.decodePDFStream(o, sd)
	if err != nil {
		return err
	}
	resources, err := r.pdf.DereferenceDict(sd.Dict["Resources"])
	if err != nil {
		return err
	}
	if resources == nil {
		resources = parent
	}
	if value := sd.Dict["Matrix"]; value != nil {
		a, err := r.pdf.DereferenceArray(value)
		if err != nil {
			return err
		}
		m, err := matrixPDF(a)
		if err != nil {
			return err
		}
		state.ctm = state.ctm.compose(m)
	}
	if value := sd.Dict["BBox"]; value != nil {
		a, err := r.pdf.DereferenceArray(value)
		if err != nil {
			return err
		}
		if len(a) != 4 {
			return fmt.Errorf("invalid Form BBox")
		}
		var v [4]float64
		for i, x := range a {
			n, e := pdfNumber(x)
			if e != nil || !finite(n) {
				return fmt.Errorf("invalid Form BBox")
			}
			v[i] = n
		}
		m := r.pageInfo.Transform.compose(state.ctm)
		points := []Point{m.point(v[0], v[1]), m.point(v[2], v[1]), m.point(v[2], v[3]), m.point(v[0], v[3]), m.point(v[0], v[1])}
		if clip, ok := rulingRect(points); ok {
			state.rulingClip = intersectRulingClip(state.rulingClip, &clip)
		} else {
			state.rulingClipUnknown = true
		}
	}
	oldStream := r.stream
	r.stream = r.objectID(o, "form")
	defer func() { r.stream = oldStream }()
	r.newline()
	return r.content(decoded, resources, depth+1, state)
}

type pdfFont struct {
	mapping                                                        *unicodeMap
	explicitEncoding                                               map[byte]string
	fallbackUsed                                                   bool
	encoding                                                       simpleencodings.Encoding
	unicode                                                        bool
	bold, italic                                                   bool
	widths                                                         map[uint32]float64
	name                                                           string
	family, familyEvidence                                         string
	ascent, descent, defaultWidth                                  float64
	boundsEstimate, defaultExact, composite, identityCID, vertical bool
	cidWidthOverlap                                                bool
}

func (f *pdfFont) decode(b []byte) (string, error) {
	if f.mapping != nil {
		text, err := f.mapping.decode(b)
		if err == nil || f.composite || len(f.explicitEncoding) == 0 {
			return text, err
		}
		var out strings.Builder
		for _, code := range b {
			part, e := f.mapping.decode([]byte{code})
			if e != nil {
				if !strings.HasPrefix(e.Error(), "unmapped ToUnicode code ") {
					return "", e
				}
				var ok bool
				part, ok = f.explicitEncoding[code]
				if !ok {
					return "", e
				}
				f.fallbackUsed = true
			}
			out.WriteString(part)
		}
		return out.String(), nil
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

	// Content-stream operands are direct objects. Parsing adjacent numbers as
	// a general object can consume the R prefix of RG as an indirect reference
	// (for example, "0 1 RG"), losing both paint operands and the operator.

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
	f.name = "unknown"
	if base != nil {
		f.name = *base
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
		decoded, err := r.decodePDFStream(o, sd)
		if err != nil {
			return nil, err
		}
		f.mapping, err = parseUnicodeMap(r.ctx, decoded)
		if err != nil {
			return nil, fmt.Errorf("ToUnicode: %w", err)
		}
		if err := r.fontMetrics(f, d); err != nil {
			return nil, err
		}
		if err := r.normalizeSimpleCMap(f); err != nil {
			return nil, err
		}
		if !f.composite {
			if err := r.completeSimpleFontEncoding(f, d); err != nil {
				return nil, err
			}
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
	if err := r.fontMetrics(f, d); err != nil {
		return nil, err
	}
	r.fonts[key] = f
	return f, nil
}

type pdfOperator string

func pdfSpace(c byte) bool     { return c == 0 || c == 9 || c == 10 || c == 12 || c == 13 || c == 32 }
func pdfDelimiter(c byte) bool { return pdfSpace(c) || strings.ContainsRune("()<>[]{}/%", rune(c)) }
func nextPDFToken(ctx context.Context, s *string) (any, error) {
	for len(*s) > 0 {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if pdfSpace((*s)[0]) {
			*s = (*s)[1:]
			continue
		}
		if (*s)[0] == '%' {
			if i := strings.IndexAny(*s, "\r\n"); i >= 0 {
				*s = (*s)[i:]
				continue
			}
			*s = ""
			return nil, nil
		}
		break
	}
	if *s == "" {
		return nil, nil
	}
	c := (*s)[0]

	if strings.ContainsRune("+-.0123456789", rune(c)) {
		i := 0
		for i < len(*s) && !pdfDelimiter((*s)[i]) {
			i++
		}
		token := (*s)[:i]
		*s = (*s)[i:]
		for j, ch := range token {
			if !(ch >= '0' && ch <= '9' || ch == '.' || j == 0 && (ch == '+' || ch == '-')) {
				return nil, fmt.Errorf("invalid PDF number %q", token)
			}
		}
		if !strings.Contains(token, ".") {
			n, err := strconv.ParseInt(token, 10, 0)
			return types.Integer(n), err
		}
		n, err := strconv.ParseFloat(token, 64)
		if err != nil || !finite(n) {
			return nil, fmt.Errorf("invalid PDF number %q", token)
		}
		return types.Float(n), nil
	}
	if strings.ContainsRune("(/[<", rune(c)) || strings.HasPrefix(*s, "true") || strings.HasPrefix(*s, "false") || strings.HasPrefix(*s, "null") {
		before := len(*s)
		v, err := model.ParseObjectContext(ctx, s)
		if err != nil {
			return nil, err
		}
		if len(*s) >= before {
			return nil, errors.New("PDF operand parser made no progress")
		}
		return v, nil
	}
	i := 0
	for i < len(*s) && !pdfDelimiter((*s)[i]) {
		i++
	}
	if i == 0 {
		return nil, fmt.Errorf("unexpected PDF token %q", (*s)[:1])
	}
	v := pdfOperator((*s)[:i])
	*s = (*s)[i:]
	return v, nil
}
func pdfString(o any) ([]byte, error) {
	switch v := o.(type) {
	case types.StringLiteral:
		return types.Unescape(string(v))
	case types.HexLiteral:
		return v.Bytes()
	}
	return nil, errors.New("expected PDF string")
}
func pdfNumber(o any) (float64, error) {
	switch v := o.(type) {
	case types.Integer:
		return float64(v), nil
	case types.Float:
		return float64(v), nil
	}
	return 0, errors.New("expected PDF number")
}
func pdfName(o any) (string, error) {
	if n, ok := o.(types.Name); ok {
		return string(n), nil
	}
	return "", errors.New("expected PDF name")
}
func utf16Text(b []byte) (string, error) {
	if len(b)%2 != 0 {
		return "", errors.New("odd-length UTF-16 text")
	}
	u := make([]uint16, len(b)/2)
	for i := range u {
		u[i] = binary.BigEndian.Uint16(b[2*i:])
	}
	for i := 0; i < len(u); i++ {
		if u[i] >= 0xD800 && u[i] <= 0xDBFF {
			if i+1 >= len(u) || u[i+1] < 0xDC00 || u[i+1] > 0xDFFF {
				return "", errors.New("unpaired UTF-16 surrogate")
			}
			i++
		} else if u[i] >= 0xDC00 && u[i] <= 0xDFFF {
			return "", errors.New("unpaired UTF-16 surrogate")
		}
	}
	return string(utf16.Decode(u)), nil
}

type codeRange struct {
	low, high uint32
	size      int
	dest      []byte
	values    [][]byte
}
type unicodeMap struct {
	chars  map[string]string
	ranges []codeRange
	spaces []codeRange
}

func codeNumber(b []byte) uint32 {
	var v uint32
	for _, c := range b {
		v = v<<8 | uint32(c)
	}
	return v
}
func parseUnicodeMap(ctx context.Context, b []byte) (*unicodeMap, error) {
	m := &unicodeMap{chars: map[string]string{}}
	s := string(b)
	mode := ""
	var args []any
	for s != "" {
		v, err := nextPDFToken(ctx, &s)
		if err != nil {
			return nil, err
		}
		if v == nil {
			break
		}
		if op, ok := v.(pdfOperator); ok {
			switch string(op) {
			case "begincodespacerange", "beginbfchar", "beginbfrange":
				mode = string(op)
				args = nil
				continue
			case "endcodespacerange", "endbfchar", "endbfrange":
				step := 2
				if mode == "beginbfrange" {
					step = 3
				}
				if len(args)%step != 0 {
					return nil, errors.New("malformed ToUnicode mapping")
				}
				for i := 0; i < len(args); i += step {
					lo, err := pdfString(args[i])
					if err != nil || len(lo) == 0 || len(lo) > 4 {
						return nil, errors.New("invalid ToUnicode source code")
					}
					if mode == "beginbfchar" {
						dst, err := pdfString(args[i+1])
						if err != nil {
							return nil, err
						}
						text, err := utf16Text(dst)
						if err != nil {
							return nil, err
						}
						m.chars[string(lo)] = text
						continue
					}
					hi, err := pdfString(args[i+1])
					if err != nil || len(lo) != len(hi) || codeNumber(hi) < codeNumber(lo) {
						return nil, errors.New("invalid ToUnicode code range")
					}
					r := codeRange{low: codeNumber(lo), high: codeNumber(hi), size: len(lo)}
					if mode == "begincodespacerange" {
						m.spaces = append(m.spaces, r)
						continue
					}
					if arr, ok := args[i+2].(types.Array); ok {
						if uint64(len(arr)) != uint64(r.high)-uint64(r.low)+1 {
							return nil, errors.New("ToUnicode range array length mismatch")
						}
						for _, value := range arr {
							dst, err := pdfString(value)
							if err != nil {
								return nil, err
							}
							r.values = append(r.values, dst)
						}
					} else {
						r.dest, err = pdfString(args[i+2])
						if err != nil {
							return nil, err
						}
					}
					m.ranges = append(m.ranges, r)
				}
				mode = ""
				args = nil
				continue
			}
			if mode != "" {
				return nil, fmt.Errorf("unexpected ToUnicode operator %q", op)
			}
			args = nil
			continue
		}
		if mode != "" {
			args = append(args, v)
		}
	}
	if mode != "" {
		return nil, errors.New("incomplete ToUnicode map")
	}
	if len(m.chars) == 0 && len(m.ranges) == 0 {
		return nil, errors.New("empty ToUnicode map")
	}
	return m, nil
}
func (m *unicodeMap) decode(b []byte) (string, error) {
	var out strings.Builder
	for len(b) > 0 {
		size := 0
		if len(m.spaces) > 0 {
			for _, r := range m.spaces {
				if len(b) >= r.size {
					v := codeNumber(b[:r.size])
					if v >= r.low && v <= r.high {
						size = r.size
						break
					}
				}
			}
		} else {
			for n := 1; n <= 4 && n <= len(b); n++ {
				if _, ok := m.chars[string(b[:n])]; ok {
					size = n
					break
				}
				for _, r := range m.ranges {
					if r.size == n && codeNumber(b[:n]) >= r.low && codeNumber(b[:n]) <= r.high {
						size = n
						break
					}
				}
				if size > 0 {
					break
				}
			}
		}
		if size == 0 {
			return "", errors.New("text code outside ToUnicode codespace")
		}
		code := b[:size]
		b = b[size:]
		if text, ok := m.chars[string(code)]; ok {
			out.WriteString(text)
			continue
		}
		found := false
		v := codeNumber(code)
		for _, r := range m.ranges {
			if r.size != size || v < r.low || v > r.high {
				continue
			}
			offset := v - r.low
			var dst []byte
			if r.values != nil {
				dst = r.values[offset]
			} else {
				dst = append([]byte(nil), r.dest...)
				carry := uint64(offset)
				for j := len(dst) - 1; j >= 0 && carry > 0; j-- {
					carry += uint64(dst[j])
					dst[j] = byte(carry)
					carry >>= 8
				}
				if carry > 0 {
					return "", errors.New("overflow in ToUnicode destination")
				}
			}
			text, err := utf16Text(dst)
			if err != nil {
				return "", err
			}
			out.WriteString(text)
			found = true
			break
		}
		if !found {
			return "", fmt.Errorf("unmapped ToUnicode code %s", strconv.FormatUint(uint64(v), 16))
		}
	}
	return out.String(), nil
}
