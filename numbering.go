package manydoc

import (
	"fmt"
	"strconv"
	"strings"
)

// ListNumbering describes an ordered-list label using OOXML's %1..%9
// placeholders. Level is zero-based. Formats contains one format per level;
// Ancestors contains fallback values for levels before Level. Enclosing lists
// override matching ancestor levels when serialized, so nested edits renumber.
// Legal renders every referenced level in decimal, retaining its original format.
type ListNumbering struct {
	Level       int      `json:"level"`
	Template    string   `json:"template"`
	Formats     []string `json:"formats"`
	Ancestors   []int    `json:"ancestors,omitempty"`
	Independent bool     `json:"independent,omitempty"`
	Legal       bool     `json:"legal,omitempty"`
}

func supportedNumberFormat(format string) bool {
	switch format {
	case "decimal", "decimalZero", "upperLetter", "lowerLetter", "upperRoman", "lowerRoman":
		return true
	}
	return false
}

func numberText(format string, value int) (string, error) {
	if value < 0 || value > 2147483647 || !supportedNumberFormat(format) {
		return "", fmt.Errorf("invalid numbering format/value")
	}
	if value == 0 {
		return "0", nil
	}
	switch format {
	case "decimal":
		return strconv.Itoa(value), nil
	case "decimalZero":
		if value < 10 {
			return "0" + strconv.Itoa(value), nil
		}
		return strconv.Itoa(value), nil
	case "upperLetter", "lowerLetter":
		count := (value-1)/26 + 1
		if count > 4096 {
			return "", fmt.Errorf("%w: alphabetic numbering label exceeds 4096 bytes", ErrUnsupported)
		}
		base := byte('A')
		if format == "lowerLetter" {
			base = 'a'
		}
		return strings.Repeat(string([]byte{base + byte((value-1)%26)}), count), nil
	default:
		if value/1000 > 4096 {
			return "", fmt.Errorf("%w: Roman numbering label exceeds 4096 bytes", ErrUnsupported)
		}
		var b strings.Builder
		for _, part := range []struct {
			n int
			s string
		}{{1000, "M"}, {900, "CM"}, {500, "D"}, {400, "CD"}, {100, "C"}, {90, "XC"}, {50, "L"}, {40, "XL"}, {10, "X"}, {9, "IX"}, {5, "V"}, {4, "IV"}, {1, "I"}} {
			count := value / part.n
			value %= part.n
			b.WriteString(strings.Repeat(part.s, count))
		}
		text := b.String()
		if len(text) > 4096 {
			return "", fmt.Errorf("%w: Roman numbering label exceeds 4096 bytes", ErrUnsupported)
		}
		if format == "lowerRoman" {
			text = strings.ToLower(text)
		}
		return text, nil
	}
}

func validateNumbering(n *ListNumbering) error {
	if n == nil {
		return nil
	}
	if n.Level < 0 || n.Level > 8 || len(n.Formats) != n.Level+1 || len(n.Ancestors) != n.Level || len(n.Template) > 512 {
		return fmt.Errorf("invalid numbering level/template/context")
	}
	if err := validateString(n.Template); err != nil {
		return err
	}
	for _, f := range n.Formats {
		if !supportedNumberFormat(f) {
			return fmt.Errorf("%w: numbering format %s", ErrUnsupported, f)
		}
	}
	for _, v := range n.Ancestors {
		if v < 0 || v > 2147483647 {
			return fmt.Errorf("invalid numbering ancestor")
		}
	}
	return nil
}

func numberedLabel(b Block, index int) (string, error) {
	value := listStart(b) + index
	if b.Numbering == nil {
		return strconv.Itoa(value) + ".", nil
	}
	n := b.Numbering
	if err := validateNumbering(n); err != nil {
		return "", err
	}
	var out strings.Builder
	for i := 0; i < len(n.Template); i++ {
		piece := n.Template[i : i+1]
		if n.Template[i] == '%' && i+1 < len(n.Template) && n.Template[i+1] >= '1' && n.Template[i+1] <= '9' {
			level := int(n.Template[i+1] - '1')
			i++
			if level > n.Level {
				continue
			} // OOXML ignores deeper-level placeholders.
			v := value
			if level < n.Level {
				v = n.Ancestors[level]
			}
			format := n.Formats[level]
			if n.Legal {
				format = "decimal"
			}
			var err error
			piece, err = numberText(format, v)
			if err != nil {
				return "", err
			}
		}
		if out.Len()+len(piece) > 4096 {
			return "", fmt.Errorf("%w: numbering label exceeds 4096 bytes", ErrUnsupported)
		}
		out.WriteString(piece)
	}
	return out.String(), nil
}
