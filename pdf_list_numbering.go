package manifold

import (
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
