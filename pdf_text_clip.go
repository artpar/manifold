package manifold

import (
	"math"
	"strings"
)

// Recover displayed axis-aligned font-advance bounds under known rectangular
// graphics clips. These remain font boxes, not exact glyph ink outlines.
func (r *pdfReader) clipTextSource(source *SourceInfo, s *pdfTextState) {
	if s.rulingClipUnknown {
		source.Confidence = min(source.Confidence, .8)
		source.Precision = "unresolved_clip_" + source.Precision
		r.warnPage("pdf_clipping", "Complex text clipping is not applied to recovered bounds")
		return
	}
	if s.rulingClip == nil {
		return
	}
	q := *source.Quad
	rect, ok := rulingRect([]Point{q[0], q[1], q[2], q[3], q[0]})
	if !ok {
		source.Confidence = min(source.Confidence, .8)
		source.Precision = "unresolved_clip_" + source.Precision
		r.warnPage("pdf_clipping", "Rectangular clipping of non-axis-aligned text bounds is unresolved")
		return
	}
	clipped := intersectRulingClip(&rect, s.rulingClip)
	if clipped.x1 <= clipped.x0 || clipped.y1 <= clipped.y0 {
		source.Precision = "fully_clipped_" + source.Precision
		source.Confidence = min(source.Confidence, .5)
		r.warnPage("pdf_clipped_text", "Fully clipped text is retained with original operator bounds; it does not establish visible content")
		return
	}
	if math.Abs(rect.x0-clipped.x0) < 1e-8 && math.Abs(rect.y0-clipped.y0) < 1e-8 && math.Abs(rect.x1-clipped.x1) < 1e-8 && math.Abs(rect.y1-clipped.y1) < 1e-8 {
		return
	}
	result := Quad{{clipped.x0, clipped.y0}, {clipped.x1, clipped.y0}, {clipped.x1, clipped.y1}, {clipped.x0, clipped.y1}}
	source.Quad = &result
	source.Precision = "rect_clipped_" + source.Precision
	if r.actualFrame() == nil {
		r.warnPage("pdf_text_partial_clip", "Partially clipped text operator strings are retained; clipping may hide some character ink")
	}
	if baseline, ok := clipBaseline(*source.Baseline, s.rulingClip); ok {
		source.Baseline = &baseline
	}
}

func clipBaseline(line [2]Point, rect *pdfRulingRect) ([2]Point, bool) {
	a, b := line[0], line[1]
	dx, dy := b.X-a.X, b.Y-a.Y
	low, high := 0.0, 1.0
	for _, edge := range []struct{ p, q float64 }{{-dx, a.X - rect.x0}, {dx, rect.x1 - a.X}, {-dy, a.Y - rect.y0}, {dy, rect.y1 - a.Y}} {
		if edge.p == 0 {
			if edge.q < 0 {
				return line, false
			}
			continue
		}
		t := edge.q / edge.p
		if edge.p < 0 {
			low = max(low, t)
		} else {
			high = min(high, t)
		}
		if low > high {
			return line, false
		}
	}
	return [2]Point{{a.X + low*dx, a.Y + low*dy}, {a.X + high*dx, a.Y + high*dy}}, true
}
func replacementClipState(precision string) uint8 {
	switch {
	case strings.HasPrefix(precision, "rect_clipped_"):
		return 1
	case strings.HasPrefix(precision, "unresolved_clip_"):
		return 2
	case strings.HasPrefix(precision, "fully_clipped_"):
		return 4
	}
	return 0
}
func clippedReplacementPrecision(state uint8) string {
	switch {
	case state&2 != 0:
		return "unresolved_clip_replacement_span_bounds"
	case state == 4:
		return "fully_clipped_replacement_span_bounds"
	case state&4 != 0:
		return "mixed_clip_replacement_span_bounds"
	case state&1 != 0:
		return "rect_clipped_replacement_span_bounds"
	}
	return "replacement_span_bounds"
}
