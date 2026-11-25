package engine

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// Recover only a canonical, representable sequence. The PDF attribute establishes
// the format family; rendered labels establish the start and literal template.
// A unique maximal token must reproduce every current label, not just the first.
var pdfListCounterTokens = regexp.MustCompile(`[A-Za-z]+|[0-9]+`)

type pdfListPattern struct {
	start     int
	numbering *ListNumbering
}

// A declared ordered family can also display a fixed template with no counter.
// PDF does not reveal its hidden start. Keep the current literal labels and
// hierarchy, using an explicitly diagnosed editorial default of one.
func pdfConstantListNumbering(attribute string, labels []string) (*pdfListPattern, bool) {
	if len(labels) == 0 || len(labels[0]) > 512 {
		return nil, false
	}
	format := map[string]string{"Decimal": "decimal", "UpperRoman": "upperRoman", "LowerRoman": "lowerRoman", "UpperAlpha": "upperLetter", "LowerAlpha": "lowerLetter"}[attribute]
	if format == "" {
		return nil, false
	}
	for _, label := range labels {
		if label != labels[0] {
			return nil, false
		}
	}
	n := &ListNumbering{Template: labels[0], Formats: []string{format}}
	if validateNumbering(n) != nil {
		return nil, false
	}
	list := Block{Kind: ListBlock, List: "number", Numbering: n}
	// Check two distinct values even for one item: a literal %1..%9 token
	// cannot be treated as a fixed template if rendering would interpret it.
	for i := 0; i < 2; i++ {
		actual, err := numberedLabel(list, i)
		if err != nil || actual != labels[0] {
			return nil, false
		}
	}
	return &pdfListPattern{start: 1, numbering: n}, true
}

func pdfCounterValue(format, text string) (int, bool) {
	if len(text) == 0 || len(text) > 4096 {
		return 0, false
	}
	value := 0
	switch format {
	case "decimal", "decimalZero":
		n, err := strconv.ParseInt(text, 10, 32)
		if err != nil || n < 0 {
			return 0, false
		}
		value = int(n)
	case "upperLetter", "lowerLetter":
		if text == "0" {
			return 0, true
		}
		base := byte('A')
		if format == "lowerLetter" {
			base = 'a'
		}
		if text[0] < base || text[0] > base+25 {
			return 0, false
		}
		for i := 1; i < len(text); i++ {
			if text[i] != text[0] {
				return 0, false
			}
		}
		value = (len(text)-1)*26 + int(text[0]-base) + 1
	case "upperRoman", "lowerRoman":
		if text == "0" {
			return 0, true
		}
		roman := map[byte]int{'I': 1, 'V': 5, 'X': 10, 'L': 50, 'C': 100, 'D': 500, 'M': 1000}
		token := strings.ToUpper(text)
		for i := range token {
			n, ok := roman[token[i]]
			if !ok {
				return 0, false
			}
			if i+1 < len(token) && n < roman[token[i+1]] {
				value -= n
			} else {
				value += n
			}
		}
	default:
		return 0, false
	}
	canonical, err := numberText(format, value)
	return value, err == nil && canonical == text
}

func pdfListNumberingPattern(attribute string, labels []string) (*pdfListPattern, bool) {
	if len(labels) == 0 || len(labels[0]) > 4096 {
		return nil, false
	}
	formats := []string{}
	switch attribute {
	case "None", "Decimal":
		formats = []string{"decimal", "decimalZero"}
	case "UpperRoman":
		formats = []string{"upperRoman"}
	case "LowerRoman":
		formats = []string{"lowerRoman"}
	case "UpperAlpha":
		formats = []string{"upperLetter"}
	case "LowerAlpha":
		formats = []string{"lowerLetter"}
	default:
		return nil, false
	}
	for _, format := range formats {
		var found *pdfListPattern
		for _, bounds := range pdfListCounterTokens.FindAllStringIndex(labels[0], -1) {
			start, ok := pdfCounterValue(format, labels[0][bounds[0]:bounds[1]])
			if !ok || int64(start)+int64(len(labels))-1 > 2147483647 {
				continue
			}
			n := &ListNumbering{Template: labels[0][:bounds[0]] + "%1" + labels[0][bounds[1]:], Formats: []string{format}}
			if validateNumbering(n) != nil {
				continue
			}
			list := Block{Kind: ListBlock, List: "number", Start: listStartPointer(start), Numbering: n}
			matches := true
			for i, label := range labels {
				if len(label) > 4096 {
					matches = false
					break
				}
				actual, err := numberedLabel(list, i)
				if err != nil || actual != label {
					matches = false
					break
				}
			}
			if !matches {
				continue
			}
			// Multiple distinct counter positions do not establish an editable pattern.

			// Rich labels and bodies use parallel independent flows. The synthetic table is
			// geometry only: original content already owns Lbl/LBody structure nodes. The
			// column policy is deliberately deterministic, not a recovered authoring indent.

			if found != nil {
				return nil, false
			}
			found = &pdfListPattern{start, n}
		}
		if found != nil {
			if format == "decimal" && found.numbering.Template == "%1." {
				found.numbering = nil
			}
			return found, true
		}
	}
	return nil, false
}
func (r *pdfRenderer) listMarker(b Block, index int, width, size float64) (pdfLine, float64, error) {
	text := "•"
	if b.List == "number" {
		var err error
		text, err = numberedLabel(b, index)
		if err != nil {
			return nil, 0, err
		}
	}
	if b.pdfListMarker != nil {
		text = *b.pdfListMarker
	}
	if text == "" {
		return nil, 0, nil
	}
	run := Run{Text: text}
	if b.pdfListMarkerStyle != nil {
		run = *b.pdfListMarkerStyle
		run.Text = text
	}
	runs := []Run{run}
	if b.pdfListMarkerContent != nil {
		runs = b.pdfListMarkerContent
	}
	if index < len(b.pdfOutputLabels) {
		runs = append([]Run(nil), runs...)
		for i := range runs {
			runs[i].pdfOutput = b.pdfOutputLabels[index]
		}
	}
	lines, err := r.lines(runs, width, size)
	if err != nil {
		return nil, 0, err
	}
	if len(lines) != 1 {
		return nil, 0, fmt.Errorf("%w: list marker exceeds content width", ErrUnsupported)
	}
	markerWidth := 0.0
	for _, s := range lines[0] {
		markerWidth += s.width
	}
	indent := math.Max(18, markerWidth+6)
	if width <= indent {
		return nil, 0, fmt.Errorf("%w: list indentation leaves no text width", ErrUnsupported)
	}
	placePDFLine(lines[0], markerWidth)
	return lines[0], indent, nil
}
func (r *pdfRenderer) list(b Block, width, x float64) error {
	for index, item := range b.Items {
		if item.Marker != nil {
			if err := r.paragraphBreakBefore(item.Blocks[0]); err != nil {
				return err
			}
			plan, err := r.measureLiteralItem(item, width, r.bodyHeight(), pdfFlowStyle{size: 11, leading: 1.4})
			if err != nil {
				return err
			}
			if index == 0 {
				plan.anchors = b.Anchors
			}
			if err := r.renderTablePlan(plan, x); err != nil {
				return err
			}
			continue
		}
		size := 11.0
		first := item.Blocks[0]
		if err := r.paragraphBreakBefore(first); err != nil {
			return err
		}
		if first.Kind == Heading {
			size = 26 - float64(first.Level)*2
		}
		marker, indent, err := r.listMarker(b, index, width, size)
		if err != nil {
			return err
		}
		minimum := size * 1.4
		if first.Kind == Heading {
			minimum *= 2
		}
		firstLines, err := r.linesDirection(first.Runs, width-indent, size, first.Direction)
		if err != nil {
			return err
		}
		minimum = pdfLineHeight(firstLines[0], size, 1.4)
		if first.Kind == Heading {
			minimum = math.Min(minimum*2, r.bodyHeight())
		}
		markerHeight := pdfLineHeight(marker, size, 1.4)
		minimum = math.Max(minimum, markerHeight)
		if err := r.ensureWithNotes(minimum, lineNoteKeys(firstLines[0])); err != nil {
			return err
		}
		if index == 0 {
			r.bindDestinations(b.Anchors, r.y)
		}
		markerX, bodyX := x, x+indent
		if first.Direction == "rtl" {
			markerX = x + width - pdfAdvance(marker)
			bodyX = x
		}
		markerTop, markerPage := r.y, r.pdf.PageNo()
		markerY := markerTop + pdfLineSize(firstLines[0], size) - size
		if b.pdfListMarkerStyle != nil || b.pdfListMarkerContent != nil {
			markerY = markerTop
		}
		r.drawLine(marker, markerX, markerY, size)
		if err := r.blocks(item.Blocks, width-indent, bodyX); err != nil {
			return err
		}
		if r.pdf.PageNo() == markerPage {
			r.y = math.Max(r.y, markerTop+markerHeight)
		}
	}
	return r.pdf.Error()
}

func (r *pdfRenderer) measureLiteralItem(item ListItem, width, height float64, style pdfFlowStyle) (*pdfTableLayout, error) {
	labelWidth := math.Min(120, width/4)
	cells := []Cell{{Blocks: item.Marker.Blocks}, {Blocks: item.Blocks}}
	widths := []float64{labelWidth, width - labelWidth}
	if item.Blocks[0].Direction == "rtl" {
		cells[0], cells[1] = cells[1], cells[0]
		widths[0], widths[1] = widths[1], widths[0]
	}
	b := Block{Kind: Table, ColumnWidths: widths, Rows: []Row{{Cells: cells}}}
	plan, err := r.measureTableStyle(b, width, height, style)
	if err != nil {
		return nil, err
	}
	plan.borderless = true
	return plan, nil
}
