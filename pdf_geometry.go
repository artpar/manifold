package manydoc

import (
	"fmt"
	"math"
	"strconv"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
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
	r.doc.warn(code, "PDF page "+strconv.Itoa(r.page)+": "+message)
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
