package manydoc

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

// Native glyphFormat reads whole decimal placeholder indices and has no percent
// escape. SF/OOXML uses single-digit %1..%9. Non-active fixed-symbol entries bridge
// literal percent and digit suffixes while leaving actual counters automatic.
const colleafMaxListEntries = 9 + 512

type colleafListPart struct {
	Literal string
	Level   int
	Dynamic bool
}

func appendColleafListLiteral(parts []colleafListPart, text string) []colleafListPart {
	if text == "" {
		return parts
	}
	if len(parts) > 0 && !parts[len(parts)-1].Dynamic {
		parts[len(parts)-1].Literal += text
		return parts
	}
	return append(parts, colleafListPart{Literal: text})
}
func colleafSFListParts(template string, level int) []colleafListPart {
	var parts []colleafListPart
	for i := 0; i < len(template); i++ {
		if template[i] == '%' && i+1 < len(template) && template[i+1] >= '1' && template[i+1] <= '9' {
			n := int(template[i+1] - '1')
			i++
			if n <= level {
				parts = append(parts, colleafListPart{Level: n, Dynamic: true})
			}
		} else {
			parts = appendColleafListLiteral(parts, template[i:i+1])
		}
	}
	return parts
}
func projectColleafListTemplate(template string, level int, source *colleafNativeList) string {
	var out strings.Builder
	helpers := map[string]int{}
	for _, part := range colleafSFListParts(template, level) {
		if part.Dynamic {
			out.WriteByte('%')
			out.WriteString(strconv.Itoa(part.Level + 1))
			continue
		}
		text := part.Literal
		digitSuffix := out.Len() > 0 && len(text) > 0 && text[0] >= '0' && text[0] <= '9'
		if !strings.Contains(text, "%") && !digitSuffix {
			out.WriteString(text)
			continue
		}
		index, ok := helpers[text]
		if !ok {
			// Keep all nine real outline levels available; helpers are never an active
			// paragraph nestingLevel or a newly invented SF numbering level.
			for len(source.Levels) < 9 {
				n := len(source.Levels)
				glyph := 2
				source.Levels = append(source.Levels, colleafListLevel{Format: "%" + strconv.Itoa(n+1) + ".", Glyph: &glyph, Align: 1})
			}
			index = len(source.Levels)
			helpers[text] = index
			glyph := 2
			source.Levels = append(source.Levels, colleafListLevel{Symbol: text, Glyph: &glyph, Align: 1})
		}
		out.WriteByte('%')
		out.WriteString(strconv.Itoa(index + 1))
	}
	return out.String()
}

func colleafTemplateMatchesOriginal(template string, level int, source colleafNativeList) bool {
	if len(template) > 512 {
		return false
	}
	// Only the active template and helper symbols affect its literal projection.
	// Current formats, counters, styles and ancestry continue to come from native.
	projected := colleafNativeList{Levels: append([]colleafListLevel(nil), source.Levels[:level+1]...)}
	format := projectColleafListTemplate(template, level, &projected)
	if format != source.Levels[level].Format || source.Levels[level].Symbol != "" {
		return false
	}
	for i := 9; i < len(projected.Levels); i++ {
		if i >= len(source.Levels) || source.Levels[i].Symbol != projected.Levels[i].Symbol {
			return false
		}
	}
	return true
}
func readColleafListTemplate(source colleafNativeList, level int, binding colleafListBinding) (string, error) {
	if binding.OriginalTemplate != nil && colleafTemplateMatchesOriginal(*binding.OriginalTemplate, level, source) {
		return *binding.OriginalTemplate, nil
	}
	var parts []colleafListPart
	logicalBytes := 0
	literal := func(text string) error {
		if logicalBytes+len(text) > 512 {
			return fmt.Errorf("%w: native list template exceeds SF representation", ErrUnsupported)
		}
		logicalBytes += len(text)
		parts = appendColleafListLiteral(parts, text)
		return nil
	}
	if source.Levels[level].Symbol != "" {
		if err := literal(source.Levels[level].Symbol); err != nil {
			return "", err
		}
	} else {
		format := source.Levels[level].Format
		if len(format) > 3072 {
			return "", fmt.Errorf("%w: native list format exceeds SF projection", ErrUnsupported)
		}
		for i := 0; i < len(format); {
			if format[i] != '%' {
				start := i
				for i < len(format) && format[i] != '%' {
					i++
				}
				if err := literal(format[start:i]); err != nil {
					return "", err
				}
				continue
			}
			i++
			start := i
			for i < len(format) && format[i] >= '0' && format[i] <= '9' {
				i++
			}
			if i == start {
				return "", fmt.Errorf("%w: native percent without a numeric placeholder", ErrUnsupported)
			}
			index, err := strconv.Atoi(format[start:i])
			if err != nil || index < 1 || index > len(source.Levels) {
				return "", fmt.Errorf("invalid native list placeholder")
			}
			index--
			spec := source.Levels[index]
			if spec.Symbol != "" {
				if err := literal(spec.Symbol); err != nil {
					return "", err
				}
			} else if index <= level {
				logicalBytes += 2
				if logicalBytes > 512 {
					return "", fmt.Errorf("%w: native list template exceeds SF representation", ErrUnsupported)
				}
				parts = append(parts, colleafListPart{Level: index, Dynamic: true})
			} else {
				return "", fmt.Errorf("%w: native numbering refers to a dynamic absent level", ErrUnsupported)
			}
		}
	}
	var out strings.Builder
	for _, part := range parts {
		if part.Dynamic {
			out.WriteByte('%')
			out.WriteString(strconv.Itoa(part.Level + 1))
		} else {
			out.WriteString(part.Literal)
		}
	}
	candidate := out.String()
	if !reflect.DeepEqual(parts, colleafSFListParts(candidate, level)) {
		return "", fmt.Errorf("%w: native literal marker cannot be expressed by the SF numbering template", ErrUnsupported)
	}
	if len(candidate) > 512 {
		return "", fmt.Errorf("%w: native list template exceeds SF representation", ErrUnsupported)
	}
	return candidate, nil
}
