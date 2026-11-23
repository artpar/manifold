package manydoc

import (
	"fmt"
	"math"
	"strings"

	"github.com/benoitkugler/textlayout/fonts"
	"github.com/benoitkugler/textlayout/harfbuzz"
	"github.com/benoitkugler/textlayout/language"
	"github.com/rivo/uniseg"
)

// Paint and annotation changes do not change the font, size or language used
// for shaping. Compare resolved faces, including explicit family mappings.
func sameShapingContext(a, b pdfSpan, size float64) bool {
	if a.glyphs == nil || b.glyphs == nil || a.run.Note != nil || b.run.Note != nil {
		return false
	}
	if a.glyphs.face != b.glyphs.face || textSize(a.run, size) != textSize(b.run, size) || a.run.ComplexScript != b.run.ComplexScript {
		return false
	}
	tag := func(s string) language.Language {
		if s == "" {
			s = "und"
		}
		return language.NewLanguage(s)
	}
	return tag(a.run.Language) == tag(b.run.Language) && tag(a.run.EastAsiaLanguage) == tag(b.run.EastAsiaLanguage) && tag(a.run.ComplexLanguage) == tag(b.run.ComplexLanguage)
}

type pdfPaintCluster struct {
	first, last int
	start, end  int // original UTF-8 byte interval
	pen, width  float64
	edges       map[int]float64 // original grapheme boundaries -> shaping-group em position
}

// GDEF carets describe the internal component boundaries of a ligature. Fonts
// without a matching usable list divide the cluster advance equally among its
// original graphemes. This is a declared paint allocation, not a glyph split.
func paintClusters(text string, c *pdfGlyphCluster) (out []pdfPaintCluster, err error) {
	defer func() {
		if p := recover(); p != nil {
			out = nil
			err = fmt.Errorf("%w: ligature caret lookup failed: %v", ErrUnsupported, p)
		}
	}()
	pen, consumed := 0.0, 0
	for first := 0; first < len(c.positions); {
		last := first + 1
		for last < len(c.positions) && c.positions[last].cluster == c.positions[first].cluster {
			last++
		}
		end := consumed + len(c.positions[first].logical)
		width := 0.0
		for _, g := range c.positions[first:last] {
			width += g.advance
		}
		if !finite(width) || width < 0 {
			return nil, fmt.Errorf("%w: invalid paint cluster advance", ErrUnsupported)
		}
		cluster := pdfPaintCluster{first: first, last: last, start: consumed, end: end, pen: pen, width: width, edges: map[int]float64{consumed: pen, end: pen + width}}
		var ends []int
		graphemes := uniseg.NewGraphemes(text[consumed:end])
		for graphemes.Next() {
			_, b := graphemes.Positions()
			ends = append(ends, consumed+b)
		}
		points := make([]float64, max(0, len(ends)-1))
		for i := range points {
			points[i] = pen + width*float64(i+1)/float64(len(ends))
		}
		if last == first+1 && len(points) > 0 {
			g := c.positions[first]
			carets := c.face.shaper.GetOTLigatureCarets(harfbuzz.LeftToRight, fonts.GID(g.glyph))
			if len(carets) == len(points) {
				candidate := make([]float64, len(points))
				previous := pen
				valid := true
				for i, v := range carets {
					x := g.x + float64(v)/float64(c.face.original.parsed.UnitsPerEm())
					if !finite(x) || x <= previous || x >= pen+width {
						valid = false
						break
					}
					candidate[i] = x
					previous = x
				}
				if valid {
					points = candidate
				}
			}
		}
		for i, p := range points {
			cluster.edges[ends[i]] = p
		}
		out = append(out, cluster)
		pen += width
		consumed = end
		first = last
	}
	if consumed != len(text) {
		return nil, fmt.Errorf("%w: paint clusters omit logical text", ErrUnsupported)
	}
	return out, nil
}

func splitShapedPaint(input pdfLine, text string, c *pdfGlyphCluster, size float64) (pdfLine, error) {
	// Keep the compact single-span representation for the common uniform case.
	uniform := true
	for _, s := range input[1:] {
		uniform = uniform && samePDFStyle(input[0].run, s.run)
	}
	if uniform {
		span := input[0]
		span.text = text
		span.glyphs = c
		span.width = c.width * textSize(span.run, size)
		for _, s := range input {
			span.ascent = math.Max(span.ascent, s.ascent)
			span.descent = math.Max(span.descent, s.descent)
		}
		span.ascent = math.Max(span.ascent, c.ascent*textSize(span.run, size))
		span.descent = math.Max(span.descent, c.descent*textSize(span.run, size))
		return pdfLine{span}, nil
	}
	clusters, err := paintClusters(text, c)
	if err != nil {
		return nil, err
	}
	edge := func(index int) (float64, error) {
		for _, cluster := range clusters {
			if p, ok := cluster.edges[index]; ok {
				return p, nil
			}
		}
		return 0, fmt.Errorf("%w: paint boundary inside an indivisible grapheme", ErrUnsupported)
	}
	var result pdfLine
	group := &pdfPaintActualGroup{text: text}
	offset := 0
	for start := 0; start < len(input); {
		end := start + 1
		var content strings.Builder
		content.WriteString(input[start].text)
		for end < len(input) && samePDFStyle(input[start].run, input[end].run) {
			content.WriteString(input[end].text)
			end++
		}
		finish := offset + content.Len()
		left, err := edge(offset)
		if err != nil {
			return nil, err
		}
		right, err := edge(finish)
		if err != nil {
			return nil, err
		}
		part := &pdfGlyphCluster{actualGroup: group, script: c.script, face: c.face, width: right - left, ascent: c.ascent, descent: c.descent, origin: left}
		if part.width < 0 {
			return nil, fmt.Errorf("%w: invalid glyph paint interval", ErrUnsupported)
		}
		for _, cluster := range clusters {
			if cluster.start == cluster.end {
				// Continuation clusters inside the same original grapheme belong to its
				// existing paint run. No separate logical text may be manufactured.
				if cluster.start <= offset || cluster.start > finish {
					continue
				}
			} else if cluster.start >= finish || cluster.end <= offset {
				continue
			}
			a, b := max(offset, cluster.start), min(finish, cluster.end)
			part.clipLeft = part.clipLeft || offset > cluster.start && offset < cluster.end
			part.clipRight = part.clipRight || finish > cluster.start && finish < cluster.end
			for i, g := range c.positions[cluster.first:cluster.last] {
				g.x -= left
				g.logical = ""
				if i == 0 && b > a {
					g.logical = text[a:b]
				}
				part.positions = append(part.positions, g)
			}
		}
		span := input[start]
		span.text = content.String()
		span.glyphs = part
		span.width = part.width * textSize(span.run, size)
		for _, s := range input[start:end] {
			span.ascent = math.Max(span.ascent, s.ascent)
			span.descent = math.Max(span.descent, s.descent)
		}
		span.ascent = math.Max(span.ascent, c.ascent*textSize(span.run, size))
		span.descent = math.Max(span.descent, c.descent*textSize(span.run, size))
		result = append(result, span)
		offset = finish
		start = end
	}
	needsCarrier := false
	for _, span := range result {
		needsCarrier = needsCarrier || span.glyphs.clipLeft || span.glyphs.clipRight
	}
	if needsCarrier {
		for i := range result {
			c := result[i].glyphs
			c.actualGroup = nil
			c.logicalCarrier = c.clipLeft || c.clipRight
		}
	}
	return result, nil
}
