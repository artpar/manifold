package manydoc

import "math"

func (r *pdfRenderer) pageEdges() PageMargins {
	if r.page != nil {
		return r.page.margins
	}
	return PageMargins{r.margin, r.margin, r.margin, r.margin}
}
func (r *pdfRenderer) bodyLeft() float64   { return r.pageEdges().Left }
func (r *pdfRenderer) bodyRight() float64  { return r.width - r.pageEdges().Right }
func (r *pdfRenderer) bodyWidth() float64  { return r.bodyRight() - r.bodyLeft() }
func (r *pdfRenderer) baseTop() float64    { return math.Abs(r.pageEdges().Top) }
func (r *pdfRenderer) baseBottom() float64 { return r.height - math.Abs(r.pageEdges().Bottom) }
func (r *pdfRenderer) headerStart() float64 {
	if r.page != nil && (r.page.header != nil || r.page.explicitMargins) {
		return r.page.headerDistance()
	}
	return r.baseTop() / 2
}
func (r *pdfRenderer) footerEnd() (float64, bool) {
	if r.page != nil && (r.page.footer != nil || r.page.explicitMargins) {
		return r.height - r.page.footerDistance(), true
	}
	return r.height, false
}
