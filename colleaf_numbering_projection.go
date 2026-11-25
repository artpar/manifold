package manifold

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strconv"
)

// Sparse format intent only: actual templates, marker text, counters, item bodies
// and styles remain native. A zero-only type shares the normal list counter group.
type colleafNumberProjection struct {
	Formats    colleafProjectedFormats `json:"formats"`
	ZeroLevels []int                   `json:"zeroLevels,omitempty"`
	Legal      bool                    `json:"legal,omitempty"`
}

// Integer member names must be canonical: encoding/json otherwise aliases "01"
// and "1" to the same Go map key before intent validation can see ambiguity.
type colleafProjectedFormats map[int]string

func (f *colleafProjectedFormats) UnmarshalJSON(data []byte) error {
	if !bytes.HasPrefix(bytes.TrimSpace(data), []byte("{")) {
		return fmt.Errorf("native projected formats must be an object")
	}
	var entries map[string]json.RawMessage
	if err := json.Unmarshal(data, &entries); err != nil {
		return err
	}
	if len(entries) > 9 {
		return fmt.Errorf("too many native projected formats")
	}
	result := colleafProjectedFormats{}
	for key, raw := range entries {
		level, err := strconv.Atoi(key)
		if err != nil || level < 0 || level > 8 || strconv.Itoa(level) != key {
			return fmt.Errorf("noncanonical native projected format level")
		}
		var format *string
		if err = json.Unmarshal(raw, &format); err != nil || format == nil {
			return fmt.Errorf("native projected format must be a string")
		}
		result[level] = *format
	}
	*f = result
	return nil
}

func projectColleafNumbering(source colleafNativeList, n *ListNumbering, value int) (colleafNativeList, *colleafNumberProjection) {
	if n == nil {
		return source, nil
	}
	source.Levels = append([]colleafListLevel(nil), source.Levels...)
	p := &colleafNumberProjection{Formats: map[int]string{}, Legal: n.Legal}
	for l, format := range n.Formats {
		v := value
		if l < n.Level {
			v = n.Ancestors[l]
		}
		zero := !n.Legal && v == 0 && format != "decimal"
		if zero || n.Legal && format == "decimalZero" {
			source.Levels[l].Glyph = colleafInt(2)
			p.Formats[l] = format
			if zero {
				p.ZeroLevels = append(p.ZeroLevels, l)
			}
		}
	}
	if len(p.Formats) == 0 {
		return source, nil
	}
	return source, p
}
func validateColleafNumberProjection(p colleafNumberProjection, level int) error {
	if len(p.Formats) == 0 || len(p.Formats) > 9 || len(p.ZeroLevels) > 9 {
		return fmt.Errorf("invalid native numbering projection")
	}
	zeros := map[int]bool{}
	for _, l := range p.ZeroLevels {
		if l < 0 || l > level || zeros[l] || p.Formats[l] == "" {
			return fmt.Errorf("invalid native zero-level intent")
		}
		zeros[l] = true
	}
	for l, format := range p.Formats {
		if l < 0 || l > level || !supportedNumberFormat(format) || format == "decimal" || (p.Legal && (format != "decimalZero" || len(zeros) != 0)) || (!p.Legal && !zeros[l]) {
			return fmt.Errorf("invalid native format intent")
		}
	}
	return nil
}
func restoreColleafNumberProjection(n *ListNumbering, p colleafNumberProjection, value int, ancestors []int, source colleafNativeList) bool {
	if n.Legal != p.Legal || source.Levels[n.Level].Symbol != "" {
		return false
	}
	for l := range p.Formats {
		if n.Formats[l] != "decimal" || source.Levels[l].Symbol != "" {
			return false
		}
	}
	for _, l := range p.ZeroLevels {
		v := value
		if l < n.Level {
			v = ancestors[l]
		}
		if v != 0 {
			return false
		}
	}
	for l, format := range p.Formats {
		n.Formats[l] = format
	}
	return true
}

// Recover the actual foreign native presentation without changing SF's existing
// zero/legal rules. A foreign zero can need a distinct first-item template; group
// assembly checks semantic changes instead of hiding them under one list type.
func normalizeColleafNativeNumbering(n *ListNumbering, value int, ancestors []int) (bool, error) {
	loss := false
	if n.Legal {
		padded := false
		for _, f := range n.Formats {
			padded = padded || f == "decimalZero"
		}
		if padded {
			n.Legal = false
			loss = true
			for i, f := range n.Formats {
				if f != "decimalZero" {
					n.Formats[i] = "decimal"
				}
			}
		}
	}
	if n.Legal {
		return loss, nil
	}
	parts := colleafSFListParts(n.Template, n.Level)
	var actual []colleafListPart
	for _, part := range parts {
		if part.Dynamic {
			v := value
			if part.Level < n.Level {
				v = ancestors[part.Level]
			}
			if v == 0 && n.Formats[part.Level] != "decimal" {
				zero := ""
				if n.Formats[part.Level] == "decimalZero" {
					zero = "00"
				}
				actual = appendColleafListLiteral(actual, zero)
				loss = true
				continue
			}
			actual = append(actual, part)
		} else {
			actual = appendColleafListLiteral(actual, part.Literal)
		}
	}
	if !loss {
		return false, nil
	}
	candidate := colleafListPartsText(actual)
	if len(candidate) > 512 || !reflect.DeepEqual(actual, colleafSFListParts(candidate, n.Level)) {
		return false, fmt.Errorf("%w: native zero marker cannot be expressed in the SF numbering template", ErrUnsupported)
	}
	n.Template = candidate
	return true, nil
}
func colleafListPartsText(parts []colleafListPart) string {
	var out string
	for _, part := range parts {
		if part.Dynamic {
			out += "%" + string(rune('1'+part.Level))
		} else {
			out += part.Literal
		}
	}
	return out
}
func sameColleafNumbering(a, b colleafListParagraph) bool {
	return a.Kind == b.Kind && reflect.DeepEqual(a.Numbering, b.Numbering)
}
func colleafListBindingTypes(b colleafListBinding) ([]string, error) {
	if b.Type == "" {
		return nil, fmt.Errorf("invalid native list binding type")
	}
	seen := map[string]bool{b.Type: true}
	for _, item := range b.Items {
		if item.Type != "" {
			seen[item.Type] = true
		}
	}
	for name := range b.Projections {
		if !seen[name] {
			return nil, fmt.Errorf("unowned native numbering projection")
		}
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}
