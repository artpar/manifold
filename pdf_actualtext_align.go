package manydoc

import "math"

// Exact agreement between decoded inner strings and an outer ActualText string
// establishes style intervals. Never align merely by rune count or guess a map
// for a genuinely different replacement string.
func alignedActualTextRuns(fragments []Run) []Run {
	var out []Run
	for start := 0; start < len(fragments); {
		end := start + 1
		for end < len(fragments) && samePDFStyle(fragments[start], fragments[end]) && fragments[start].Source.Font == fragments[end].Source.Font {
			end++
		}
		run := fragments[start]
		source := *run.Source
		source.Evidence = "pdf_actualtext_aligned"
		source.Regions = nil
		source.Matrix = nil
		var text []byte
		var bounds Quad
		var baseline [2]Point
		noncollinear := false
		clipState := uint8(0)
		for i, fragment := range fragments[start:end] {
			text = append(text, fragment.Text...)
			s := fragment.Source
			clipState |= replacementClipState(s.Precision)
			source.Artifacts = artifactIDs(&source, s.Artifacts...)
			source.Confidence = min(source.Confidence, s.Confidence)
			q := *s.Quad
			source.Regions = append(source.Regions, SourceRegion{Page: s.Page, Quad: q})
			x0, y0, x1, y1 := quadBounds(q)
			if i > 0 {
				a, b, c, d := quadBounds(bounds)
				x0 = min(x0, a)
				y0 = min(y0, b)
				x1 = max(x1, c)
				y1 = max(y1, d)
			}
			bounds = Quad{{x0, y0}, {x1, y0}, {x1, y1}, {x0, y1}}
			if i == 0 {
				baseline = *s.Baseline
			} else {
				a, b := baseline[0], baseline[1]
				dx, dy := b.X-a.X, b.Y-a.Y
				length := math.Hypot(dx, dy)
				if length > 1e-8 {
					for _, p := range *s.Baseline {
						noncollinear = noncollinear || math.Abs((p.X-a.X)*dy-(p.Y-a.Y)*dx)/length > .01
					}
				} else {
					noncollinear = noncollinear || math.Hypot(s.Baseline[0].X-a.X, s.Baseline[0].Y-a.Y) > .01
				}
				baseline[1] = s.Baseline[1]
			}
		}
		run.Text = string(text)
		source.Quad = &bounds
		source.Baseline = nil
		if !noncollinear {
			source.Baseline = &baseline
		}
		source.Precision = clippedReplacementPrecision(clipState)
		run.Source = &source
		if run.Text != "" {
			out = append(out, run)
		}
		start = end
	}
	return out
}
