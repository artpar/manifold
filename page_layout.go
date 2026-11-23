package manydoc

import (
	"fmt"
	"math"
)

// PageMargins overrides all four edges of Layout.Margin. Zero is an explicit
// zero margin. Top/Bottom may be negative for OOXML fixed body offsets that
// permit running-content overlap; Left/Right are nonnegative.
type PageMargins struct {
	Top    float64 `json:"top"`
	Right  float64 `json:"right"`
	Bottom float64 `json:"bottom"`
	Left   float64 `json:"left"`
}

type pageGeometry struct {
	width, height   float64
	margins         PageMargins
	header, footer  *float64
	explicitMargins bool
}

func cloneLayout(l Layout) Layout {
	if l.Margins != nil {
		v := *l.Margins
		l.Margins = &v
	}
	if l.HeaderDistance != nil {
		v := *l.HeaderDistance
		l.HeaderDistance = &v
	}
	if l.FooterDistance != nil {
		v := *l.FooterDistance
		l.FooterDistance = &v
	}
	return l
}
func pageLayout(l Layout) pageGeometry {
	w, h, m := layout(l)
	g := pageGeometry{width: w, height: h, margins: PageMargins{m, m, m, m}, header: l.HeaderDistance, footer: l.FooterDistance, explicitMargins: l.Margins != nil}
	if l.Margins != nil {
		g.margins = *l.Margins
	}
	return g
}
func (g pageGeometry) contentWidth() float64 { return g.width - g.margins.Left - g.margins.Right }
func (g pageGeometry) contentHeight() float64 {
	return g.height - math.Abs(g.margins.Top) - math.Abs(g.margins.Bottom)
}
func (g pageGeometry) headerDistance() float64 {
	if g.header != nil {
		return *g.header
	}
	return docxRunningDistance
}
func (g pageGeometry) footerDistance() float64 {
	if g.footer != nil {
		return *g.footer
	}
	return docxRunningDistance
}
func validateLayout(l Layout) error {
	for _, n := range []float64{l.Width, l.Height, l.Margin} {
		if !finite(n) || n < 0 {
			return fmt.Errorf("layout dimensions must be finite and nonnegative")
		}
	}
	g := pageLayout(l)
	m := g.margins
	for _, n := range []float64{m.Top, m.Right, m.Bottom, m.Left} {
		if !finite(n) {
			return fmt.Errorf("page margins must be finite")
		}
	}
	if m.Left < 0 || m.Right < 0 {
		return fmt.Errorf("left/right page margins must be nonnegative")
	}
	if g.contentWidth() <= 0 || g.contentHeight() <= 0 {
		return fmt.Errorf("page margins leave no content area")
	}
	for _, p := range []*float64{l.HeaderDistance, l.FooterDistance} {
		if p != nil && (!finite(*p) || *p < 0) {
			return fmt.Errorf("running distance must be finite and nonnegative")
		}
	}
	return nil
}

// New edge/distance bindings budget running images against the page around their
// anchors, independently from the already reserved body margins. Legacy inputs
// keep their previous full-body image preference container.
func (g pageGeometry) runningHeight(header, footer bool) float64 {
	if !g.explicitMargins && g.header == nil && g.footer == nil {
		return g.contentHeight()
	}
	h := g.height
	if header {
		h -= g.headerDistance()
	}
	if footer {
		h -= g.footerDistance()
	}
	return h
}
