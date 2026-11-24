package manydoc

import (
	"fmt"
	"math"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// Rulings are painted centerlines in displayed points. They are evidence for
// inference, not a second editable representation of the PDF graphics program.
type pdfRuling struct {
	horizontal             bool
	pos, start, end, width float64
}
type pdfRulingRect struct{ x0, y0, x1, y1 float64 }
type pdfSubpath struct {
	points []Point
	curved bool
}
type pdfCurrentPath struct {
	parts []pdfSubpath
	clip  bool
}

func (p *pdfCurrentPath) move(point Point) {
	p.parts = append(p.parts, pdfSubpath{points: []Point{point}})
}
func (p *pdfCurrentPath) line(point Point) error {
	if len(p.parts) == 0 {
		return fmt.Errorf("PDF line/curve has no current point")
	}
	i := len(p.parts) - 1
	p.parts[i].points = append(p.parts[i].points, point)
	return nil
}
func (p *pdfCurrentPath) close() {
	if len(p.parts) == 0 {
		return
	}
	i := len(p.parts) - 1
	points := p.parts[i].points
	if len(points) > 1 && points[len(points)-1] != points[0] {
		p.parts[i].points = append(points, points[0])
	}
}
func rulingRect(points []Point) (pdfRulingRect, bool) {
	if len(points) == 4 {
		points = append(append([]Point{}, points...), points[0])
	}
	if len(points) != 5 || math.Hypot(points[0].X-points[4].X, points[0].Y-points[4].Y) > 1e-6 {
		return pdfRulingRect{}, false
	}
	x0, y0, x1, y1 := quadBounds(Quad{points[0], points[1], points[2], points[3]})
	if x1 <= x0 || y1 <= y0 {
		return pdfRulingRect{}, false
	}
	seen := 0
	for _, p := range points[:4] {
		corner := 0
		if math.Abs(p.X-x1) < 1e-6 {
			corner |= 1
		} else if math.Abs(p.X-x0) >= 1e-6 {
			return pdfRulingRect{}, false
		}
		if math.Abs(p.Y-y1) < 1e-6 {
			corner |= 2
		} else if math.Abs(p.Y-y0) >= 1e-6 {
			return pdfRulingRect{}, false
		}
		if seen&(1<<corner) != 0 {
			return pdfRulingRect{}, false
		}
		seen |= 1 << corner
	}
	for i := 1; i < 5; i++ {
		a, b := points[i-1], points[i]
		if math.Abs(a.X-b.X) > 1e-6 && math.Abs(a.Y-b.Y) > 1e-6 {
			return pdfRulingRect{}, false
		}
	}
	return pdfRulingRect{x0, y0, x1, y1}, true
}
func intersectRulingClip(a, b *pdfRulingRect) *pdfRulingRect {
	if a == nil {
		c := *b
		return &c
	}
	if b == nil {
		c := *a
		return &c
	}
	return &pdfRulingRect{max(a.x0, b.x0), max(a.y0, b.y0), min(a.x1, b.x1), min(a.y1, b.y1)}
}
func (r *pdfReader) addRuling(a, b Point, width float64, state pdfTextState) {
	if !finite(a.X) || !finite(a.Y) || !finite(b.X) || !finite(b.Y) || !finite(width) {
		r.warnPage("pdf_ruling_geometry", "Nonfinite transformed ruling geometry is not used for inference")
		return
	}
	if state.rulingEffects || state.rulingClipUnknown {
		return
	}
	line := pdfRuling{width: width}
	switch {
	case math.Abs(a.Y-b.Y) < 1e-6 && math.Abs(a.X-b.X) > 1e-6:
		line.horizontal = true
		line.pos = (a.Y + b.Y) / 2
		line.start = min(a.X, b.X)
		line.end = max(a.X, b.X)
	case math.Abs(a.X-b.X) < 1e-6 && math.Abs(a.Y-b.Y) > 1e-6:
		line.pos = (a.X + b.X) / 2
		line.start = min(a.Y, b.Y)
		line.end = max(a.Y, b.Y)
	default:
		return
	}
	clip := intersectRulingClip(state.rulingClip, &pdfRulingRect{0, 0, r.pageInfo.Width, r.pageInfo.Height})
	if clip.x1 <= clip.x0 || clip.y1 <= clip.y0 {
		return
	}
	if line.horizontal {
		if line.pos < clip.y0 || line.pos > clip.y1 {
			return
		}
		line.start = max(line.start, clip.x0)
		line.end = min(line.end, clip.x1)
	} else {
		if line.pos < clip.x0 || line.pos > clip.x1 {
			return
		}
		line.start = max(line.start, clip.y0)
		line.end = min(line.end, clip.y1)
	}
	if line.end <= line.start {
		return
	}
	if !r.admitRuling() {
		return
	}
	r.rulings[r.page] = append(r.rulings[r.page], line)
}

// Small filled corner/join pieces support coverage along established ruling
// axes. They never introduce axes or connect independent ruling components.
func (r *pdfReader) addRulingPatch(rect pdfRulingRect, state pdfTextState) {
	if state.rulingEffects || state.rulingClipUnknown {
		return
	}
	if !finite(rect.x0) || !finite(rect.y0) || !finite(rect.x1) || !finite(rect.y1) {
		r.warnPage("pdf_ruling_geometry", "Nonfinite transformed ruling geometry is not used for inference")
		return
	}
	clip := intersectRulingClip(state.rulingClip, &pdfRulingRect{0, 0, r.pageInfo.Width, r.pageInfo.Height})
	rect = pdfRulingRect{max(rect.x0, clip.x0), max(rect.y0, clip.y0), min(rect.x1, clip.x1), min(rect.y1, clip.y1)}
	if rect.x1 <= rect.x0 || rect.y1 <= rect.y0 || !r.admitRuling() {
		return
	}
	r.rulingPatches[r.page] = append(r.rulingPatches[r.page], rect)
}

func (r *pdfReader) admitRuling() bool {
	if r.rulingOverflow == nil {
		r.rulingOverflow = map[int]bool{}
		r.rulings = map[int][]pdfRuling{}
		r.rulingPatches = map[int][]pdfRulingRect{}
	}
	if r.rulingOverflow[r.page] {
		return false
	}
	if len(r.rulings[r.page])+len(r.rulingPatches[r.page]) >= 100_000 {
		r.rulingOverflow[r.page] = true
		r.rulings[r.page] = nil
		r.rulingPatches[r.page] = nil
		r.warnPage("pdf_ruling_limit", "Painted line and patch count exceeds the per-page inference budget; ruled-table inference is skipped while text/images persist")
		return false
	}
	return true
}

func (r *pdfReader) pathOperator(op string, args []any, path *pdfCurrentPath, state *pdfTextState) error {
	need := func(n int) error {
		if len(args) != n {
			return fmt.Errorf("PDF operator %s expects %d operands, got %d", op, n, len(args))
		}
		return nil
	}
	nums := func(n int) ([]float64, error) {
		if err := need(n); err != nil {
			return nil, err
		}
		out := make([]float64, n)
		for i, arg := range args {
			v, e := pdfNumber(arg)
			if e != nil || !finite(v) {
				return nil, fmt.Errorf("invalid PDF path operand")
			}
			out[i] = v
		}
		return out, nil
	}
	m := r.pageInfo.Transform.compose(state.ctm)
	switch op {
	case "m", "l":
		a, e := nums(2)
		if e != nil {
			return e
		}
		point := m.point(a[0], a[1])
		if op == "m" {
			path.move(point)
		} else {
			return path.line(point)
		}
	case "c", "v", "y":
		n := 4
		if op == "c" {
			n = 6
		}
		a, e := nums(n)
		if e != nil {
			return e
		}
		if err := path.line(m.point(a[n-2], a[n-1])); err != nil {
			return err
		}
		path.parts[len(path.parts)-1].curved = true
	case "re":
		a, e := nums(4)
		if e != nil {
			return e
		}
		x, y, w, h := a[0], a[1], a[2], a[3]
		path.move(m.point(x, y))
		for _, p := range []Point{m.point(x+w, y), m.point(x+w, y+h), m.point(x, y+h)} {
			if err := path.line(p); err != nil {
				return err
			}
		}
		path.close()
	case "h":
		if err := need(0); err != nil {
			return err
		}
		path.close()
	case "W", "W*":
		if err := need(0); err != nil {
			return err
		}
		path.clip = true
	case "w":
		a, e := nums(1)
		if e != nil {
			return e
		}
		if a[0] < 0 {
			return fmt.Errorf("negative PDF line width")
		}
		state.rulingWidth = a[0]
	default:
		if err := need(0); err != nil {
			return err
		}
		stroke := op == "S" || op == "s" || op == "B" || op == "B*" || op == "b" || op == "b*"
		fill := op == "f" || op == "F" || op == "f*" || op == "B" || op == "B*" || op == "b" || op == "b*"
		if op == "s" || op == "b" || op == "b*" {
			path.close()
		}
		for _, part := range path.parts {
			if part.curved {
				continue
			}
			if stroke && state.rulingStrokeAlpha > 0 {
				for i := 1; i < len(part.points); i++ {
					a, b := part.points[i-1], part.points[i]
					scale := math.Hypot(m[2], m[3])
					if math.Abs(a.X-b.X) < 1e-6 {
						scale = math.Hypot(m[0], m[1])
					}
					r.addRuling(a, b, state.rulingWidth*scale, *state)
				}
			}
			if fill && state.rulingFillAlpha > 0 {
				points := part.points
				if len(points) == 4 {
					points = append(append([]Point{}, points...), points[0])
				}
				if rect, ok := rulingRect(points); ok {
					w, h := rect.x1-rect.x0, rect.y1-rect.y0
					if w <= 3 && h <= 3 {
						r.addRulingPatch(rect, *state)
					}
					if h <= 3 && w >= max(4, h*5) {
						r.addRuling(Point{rect.x0, (rect.y0 + rect.y1) / 2}, Point{rect.x1, (rect.y0 + rect.y1) / 2}, h, *state)
					}
					if w <= 3 && h >= max(4, w*5) {
						r.addRuling(Point{(rect.x0 + rect.x1) / 2, rect.y0}, Point{(rect.x0 + rect.x1) / 2, rect.y1}, w, *state)
					}
				}
			}
		}
		if path.clip {
			if len(path.parts) == 1 && !path.parts[0].curved {
				if rect, ok := rulingRect(path.parts[0].points); ok {
					state.rulingClip = intersectRulingClip(state.rulingClip, &rect)
				} else {
					state.rulingClipUnknown = true
				}
			} else {
				state.rulingClipUnknown = true
			}
		}
		*path = pdfCurrentPath{}
		if op != "n" {
			r.warnPage("pdf_vector_graphics", "Painted vector appearance is not preserved as editable content; eligible rulings may inform inferred tables")
		}
	}
	return nil
}

// Only state relevant to whether a ruling is painted is used here. Other
// extended-state interpretation retains the existing explicit loss diagnostic.
func (r *pdfReader) rulingGraphicsState(args []any, resources types.Dict, state *pdfTextState) error {
	if len(args) != 1 {
		return fmt.Errorf("PDF gs expects one name")
	}
	name, err := pdfName(args[0])
	if err != nil {
		return err
	}
	states, err := r.pdf.DereferenceDict(resources["ExtGState"])
	if err != nil {
		return err
	}
	if states[name] == nil {
		return fmt.Errorf("missing PDF ExtGState %q", name)
	}
	d, err := r.pdf.DereferenceDict(states[name])
	if err != nil || d == nil {
		return fmt.Errorf("invalid PDF ExtGState")
	}
	for _, entry := range []struct {
		key    string
		target *float64
	}{{"CA", &state.rulingStrokeAlpha}, {"ca", &state.rulingFillAlpha}, {"LW", &state.rulingWidth}} {
		if value := d[entry.key]; value != nil {
			n, e := r.pdf.DereferenceNumber(value)
			if e != nil || !finite(n) || n < 0 || (entry.key != "LW" && n > 1) {
				return fmt.Errorf("invalid PDF ExtGState %s", entry.key)
			}
			*entry.target = n
		}
	}
	for _, key := range []string{"SMask", "BM", "TR", "TR2"} {
		if value := d[key]; value != nil {
			v, e := r.pdf.Dereference(value)
			if e != nil {
				return e
			}
			n, ok := v.(types.Name)
			safe := ok && ((key == "SMask" && n == "None") || (key == "BM" && (n == "Normal" || n == "Compatible")) || ((key == "TR" || key == "TR2") && (n == "Identity" || n == "Default")))
			switch key {
			case "SMask":
				state.rulingMask = !safe
			case "BM":
				state.rulingBlend = !safe
			default:
				state.rulingTransfer = !safe
			}
		}
	}
	state.rulingEffects = state.rulingMask || state.rulingBlend || state.rulingTransfer
	return nil
}
