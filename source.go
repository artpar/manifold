package manydoc

import (
	"fmt"
	"math"
)

// Point is in physical points in displayed page coordinates: top-left origin,
// x right and y down. PDFPage.Transform maps original PDF user coordinates here.
type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}
type Quad [4]Point

// Matrix is [a b c d e f]: x'=a*x+c*y+e, y'=b*x+d*y+f.
type Matrix [6]float64

func identityMatrix() Matrix { return Matrix{1, 0, 0, 1, 0, 0} }
func (a Matrix) point(x, y float64) Point {
	return Point{a[0]*x + a[2]*y + a[4], a[1]*x + a[3]*y + a[5]}
}

// compose applies b first, then a.
func (a Matrix) compose(b Matrix) Matrix {
	return Matrix{a[0]*b[0] + a[2]*b[1], a[1]*b[0] + a[3]*b[1], a[0]*b[2] + a[2]*b[3], a[1]*b[2] + a[3]*b[3], a[0]*b[4] + a[2]*b[5] + a[4], a[1]*b[4] + a[3]*b[5] + a[5]}
}
func translation(x, y float64) Matrix { return Matrix{1, 0, 0, 1, x, y} }

type SourceRegion struct {
	Page int  `json:"page"`
	Quad Quad `json:"quad"`
}

// SourceElementReference retains an explicit logical cell that was an empty
// continuation of a geometrically recovered merged cell.
type SourceElementReference struct {
	Row     int    `json:"row"` // zero-based row in the explicit table
	Element string `json:"element"`
	Role    string `json:"role"`
	Tag     string `json:"tag"`
	Page    int    `json:"page,omitempty"`
}

// TableSpanSource separates explicit logical membership from recovered spans.
// ColSpan and RowSpan are "explicit" or "inferred". Bounds records the painted
// cell boundary; SourceInfo.Quad continues to record its content bounds.
type TableSpanSource struct {
	ColSpan       string                   `json:"colspan"`
	RowSpan       string                   `json:"rowspan"`
	Evidence      string                   `json:"evidence"`
	Confidence    float64                  `json:"confidence"`
	Bounds        Quad                     `json:"bounds"`
	Continuations []SourceElementReference `json:"continuations,omitempty"`
}

// TableWidthSource records measured painted-grid widths independently from
// explicit PDF logical table membership. Confidence is not a probability.
type TableWidthSource struct {
	Evidence   string  `json:"evidence"`
	Confidence float64 `json:"confidence"`
	Bounds     Quad    `json:"bounds"`
}

// SourceInfo records provenance, not an alternative copy of content. Quads use
// advance/font metrics, not glyph ink outlines. Precision discloses estimates.
// Confidence is a heuristic ranking, not a calibrated probability.
type SourceInfo struct {
	Artifacts          []string          `json:"artifacts,omitempty"`
	Format             Format            `json:"format"`
	Page               int               `json:"page,omitempty"`
	Object             string            `json:"object,omitempty"`
	Stream             string            `json:"stream,omitempty"`
	Role               string            `json:"role,omitempty"`
	Element            string            `json:"element,omitempty"`
	Order              int               `json:"order,omitempty"`
	Quad               *Quad             `json:"quad,omitempty"`
	Baseline           *[2]Point         `json:"baseline,omitempty"`
	Matrix             *Matrix           `json:"matrix,omitempty"`
	Regions            []SourceRegion    `json:"regions,omitempty"`
	Font               string            `json:"font,omitempty"`
	FontFamilyEvidence string            `json:"font_family_evidence,omitempty"`
	ColorSpace         string            `json:"color_space,omitempty"`
	ColorComponents    []float64         `json:"color_components,omitempty"`
	FontSize           float64           `json:"font_size,omitempty"`
	MCID               *int              `json:"mcid,omitempty"`
	Tag                string            `json:"tag,omitempty"`
	Structure          string            `json:"structure"` // explicit, inferred or unstructured
	Evidence           string            `json:"evidence"`
	Confidence         float64           `json:"confidence"`
	Precision          string            `json:"precision,omitempty"`
	Label              string            `json:"label,omitempty"`
	RenderMode         int               `json:"render_mode,omitempty"`
	TableWidths        *TableWidthSource `json:"table_widths,omitempty"`
	TableSpans         *TableSpanSource  `json:"table_spans,omitempty"`
}

// PDFPage records physical page boundaries even when a logical tagged paragraph
// spans pages. Boxes retain PDF user units; Width/Height and source quads use pt.
type PDFPage struct {
	Artifacts []PDFArtifact `json:"artifacts,omitempty"`
	Number    int           `json:"number"`
	MediaBox  [4]float64    `json:"media_box"`
	CropBox   [4]float64    `json:"crop_box"`
	Rotate    int           `json:"rotate"`
	UserUnit  float64       `json:"user_unit"`
	Width     float64       `json:"width"`
	Height    float64       `json:"height"`
	Transform Matrix        `json:"transform"`
}

func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
func (s *SourceInfo) validate(pages []PDFPage) error {
	if s == nil {
		return nil
	}
	if s.Format != PDF {
		return fmt.Errorf("source format must be pdf")
	}
	seenArtifacts := map[string]bool{}
	for _, id := range s.Artifacts {
		if seenArtifacts[id] {
			return fmt.Errorf("duplicate source artifact membership")
		}
		seenArtifacts[id] = true
		found := false
		for _, page := range pages {
			if s.Page != 0 && s.Page != page.Number {
				continue
			}
			for _, a := range page.Artifacts {
				if a.ID == id {
					found = true
					break
				}
			}
		}
		if !found {
			return fmt.Errorf("source artifact membership is undefined or belongs to another page")
		}
	}
	if s.Page < 0 || s.Page > len(pages) {
		return fmt.Errorf("source page out of range")
	}
	if (s.Quad != nil || s.Baseline != nil) && s.Page == 0 {
		return fmt.Errorf("source geometry requires a page")
	}
	if s.Order < 0 || !finite(s.FontSize) || s.FontSize < 0 || !finite(s.Confidence) || s.Confidence < 0 || s.Confidence > 1 {
		return fmt.Errorf("invalid source metrics/confidence")
	}
	switch s.FontFamilyEvidence {
	case "", "descriptor", "embedded_name", "core_font", "postscript_fallback", "unresolved":
	default:
		return fmt.Errorf("invalid font family evidence")
	}
	count := map[string]int{"": 0, "DeviceGray": 1, "DeviceRGB": 3, "DeviceCMYK": 4}
	n, ok := count[s.ColorSpace]
	if !ok || len(s.ColorComponents) != n {
		return fmt.Errorf("invalid source color components")
	}
	for _, v := range s.ColorComponents {
		if !finite(v) || v < 0 || v > 1 {
			return fmt.Errorf("invalid source color value")
		}
	}
	if s.MCID != nil && *s.MCID < 0 {
		return fmt.Errorf("invalid marked-content ID")
	}
	if s.Structure != "explicit" && s.Structure != "inferred" && s.Structure != "unstructured" {
		return fmt.Errorf("invalid source structure provenance")
	}
	if s.Evidence == "" {
		return fmt.Errorf("source evidence is required")
	}
	if s.RenderMode < 0 || s.RenderMode > 7 {
		return fmt.Errorf("invalid text render mode")
	}
	for _, v := range []string{s.Object, s.Stream, s.Role, s.Element, s.Font, s.Tag, s.Evidence, s.Precision, s.Label} {
		if err := validateString(v); err != nil {
			return err
		}
	}
	check := func(q Quad) error {
		for _, p := range q {
			if !finite(p.X) || !finite(p.Y) {
				return fmt.Errorf("source coordinates must be finite")
			}
		}
		return nil
	}
	if s.Quad != nil {
		if err := check(*s.Quad); err != nil {
			return err
		}
	}
	if s.Baseline != nil {
		for _, p := range *s.Baseline {
			if !finite(p.X) || !finite(p.Y) {
				return fmt.Errorf("source baseline must be finite")
			}
		}
	}
	if s.Matrix != nil {
		for _, v := range *s.Matrix {
			if !finite(v) {
				return fmt.Errorf("source matrix must be finite")
			}
		}
	}
	for _, region := range s.Regions {
		if region.Page < 1 || region.Page > len(pages) {
			return fmt.Errorf("source region page out of range")
		}
		if err := check(region.Quad); err != nil {
			return err
		}
	}
	if width := s.TableWidths; width != nil {
		if s.Page < 1 || s.Role != "Table" || width.Evidence != "painted_grid_adjacency" || !finite(width.Confidence) || width.Confidence < 0 || width.Confidence > 1 {
			return fmt.Errorf("invalid table width provenance")
		}
		if err := check(width.Bounds); err != nil {
			return err
		}
		x0, y0, x1, y1 := quadBounds(width.Bounds)
		if x1 <= x0 || y1 <= y0 {
			return fmt.Errorf("invalid table width bounds")
		}
	}
	if span := s.TableSpans; span != nil {
		if s.Page < 1 || s.Structure != "explicit" || (s.Role != "TD" && s.Role != "TH") {
			return fmt.Errorf("table span provenance requires a page-local explicit cell")
		}
		if (span.ColSpan != "explicit" && span.ColSpan != "inferred") || (span.RowSpan != "explicit" && span.RowSpan != "inferred") || span.Evidence != "painted_grid_adjacency" || !finite(span.Confidence) || span.Confidence < 0 || span.Confidence > 1 {
			return fmt.Errorf("invalid table span provenance")
		}
		if err := check(span.Bounds); err != nil {
			return err
		}
		x0, y0, x1, y1 := quadBounds(span.Bounds)
		if x1 <= x0 || y1 <= y0 || len(span.Continuations) > 1000 {
			return fmt.Errorf("invalid table span bounds or continuation count")
		}
		seen := map[string]bool{s.Element: true}
		for _, ref := range span.Continuations {
			if ref.Row < 0 || ref.Row > 1000 || ref.Element == "" || seen[ref.Element] || (ref.Role != "TD" && ref.Role != "TH") || ref.Tag == "" || ref.Page < 0 || (ref.Page != 0 && ref.Page != s.Page) {
				return fmt.Errorf("invalid or duplicate table continuation reference")
			}
			seen[ref.Element] = true
			for _, value := range []string{ref.Element, ref.Role, ref.Tag} {
				if err := validateString(value); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
func validatePDFPages(pages []PDFPage) error {
	artifacts := map[string]bool{}
	for i, p := range pages {
		for _, a := range p.Artifacts {
			if artifacts[a.ID] {
				return fmt.Errorf("duplicate PDF artifact identity")
			}
			artifacts[a.ID] = true
			if len(artifacts) > 100000 {
				return fmt.Errorf("PDF artifact budget exceeded")
			}
			if a.Matrix != p.Transform {
				return fmt.Errorf("artifact coordinate matrix contradicts its page default space")
			}
			if e := a.validate(); e != nil {
				return e
			}
		}
		if p.Number != i+1 || p.Rotate < 0 || p.Rotate >= 360 || p.Rotate%90 != 0 || !finite(p.UserUnit) || p.UserUnit <= 0 || !finite(p.Width) || !finite(p.Height) || p.Width <= 0 || p.Height <= 0 {
			return fmt.Errorf("invalid PDF page geometry")
		}
		for _, box := range [][4]float64{p.MediaBox, p.CropBox} {
			for _, v := range box {
				if !finite(v) {
					return fmt.Errorf("page boxes must be finite")
				}
			}
			if box[2] <= box[0] || box[3] <= box[1] {
				return fmt.Errorf("empty PDF page box")
			}
		}
		expected, w, h, err := pageTransform(p.MediaBox, p.CropBox, p.Rotate, p.UserUnit)
		if err != nil {
			return err
		}
		same := func(a, b float64) bool { return math.Abs(a-b) <= 1e-8*max(1, math.Abs(b)) }
		if !same(p.Width, w) || !same(p.Height, h) {
			return fmt.Errorf("page dimensions contradict boxes/rotation/user unit")
		}
		for j, v := range expected {
			if !same(p.Transform[j], v) {
				return fmt.Errorf("page transform contradicts boxes/rotation/user unit")
			}
		}
		for _, v := range p.Transform {
			if !finite(v) {
				return fmt.Errorf("page transform must be finite")
			}
		}
	}
	return nil
}
func quadBounds(q Quad) (x0, y0, x1, y1 float64) {
	x0, y0, x1, y1 = q[0].X, q[0].Y, q[0].X, q[0].Y
	for _, p := range q[1:] {
		x0 = math.Min(x0, p.X)
		y0 = math.Min(y0, p.Y)
		x1 = math.Max(x1, p.X)
		y1 = math.Max(y1, p.Y)
	}
	return
}

// Renderers currently reflow content; provenance must not disappear without acknowledgment.
func (d *Document) hasPDFSource() bool {
	if len(d.Pages) > 0 {
		return true
	}
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
			if b.Source != nil {
				return true
			}
			for _, r := range b.Runs {
				if r.Source != nil {
					return true
				}
			}
			for _, row := range b.Rows {
				for _, c := range row.Cells {
					if c.Source != nil || walk(c.Blocks) {
						return true
					}
				}
			}
			for _, item := range b.Items {
				if item.Source != nil || walk(item.Blocks) {
					return true
				}
			}
		}
		return false
	}
	if walk(d.Blocks) || walk(d.Headers) || walk(d.Footers) {
		return true
	}
	for _, n := range d.Notes {
		if walk(n.Blocks) {
			return true
		}
	}
	return false
}
