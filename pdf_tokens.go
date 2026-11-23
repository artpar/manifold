package manydoc

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf16"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

type pdfOperator string

func pdfSpace(c byte) bool     { return c == 0 || c == 9 || c == 10 || c == 12 || c == 13 || c == 32 }
func pdfDelimiter(c byte) bool { return pdfSpace(c) || strings.ContainsRune("()<>[]{}/%", rune(c)) }
func nextPDFToken(ctx context.Context, s *string) (any, error) {
	for len(*s) > 0 {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if pdfSpace((*s)[0]) {
			*s = (*s)[1:]
			continue
		}
		if (*s)[0] == '%' {
			if i := strings.IndexAny(*s, "\r\n"); i >= 0 {
				*s = (*s)[i:]
				continue
			}
			*s = ""
			return nil, nil
		}
		break
	}
	if *s == "" {
		return nil, nil
	}
	c := (*s)[0]
	// Content-stream operands are direct objects. Parsing adjacent numbers as
	// a general object can consume the R prefix of RG as an indirect reference
	// (for example, "0 1 RG"), losing both paint operands and the operator.
	if strings.ContainsRune("+-.0123456789", rune(c)) {
		i := 0
		for i < len(*s) && !pdfDelimiter((*s)[i]) {
			i++
		}
		token := (*s)[:i]
		*s = (*s)[i:]
		for j, ch := range token {
			if !(ch >= '0' && ch <= '9' || ch == '.' || j == 0 && (ch == '+' || ch == '-')) {
				return nil, fmt.Errorf("invalid PDF number %q", token)
			}
		}
		if !strings.Contains(token, ".") {
			n, err := strconv.ParseInt(token, 10, 0)
			return types.Integer(n), err
		}
		n, err := strconv.ParseFloat(token, 64)
		if err != nil || !finite(n) {
			return nil, fmt.Errorf("invalid PDF number %q", token)
		}
		return types.Float(n), nil
	}
	if strings.ContainsRune("(/[<", rune(c)) || strings.HasPrefix(*s, "true") || strings.HasPrefix(*s, "false") || strings.HasPrefix(*s, "null") {
		before := len(*s)
		v, err := model.ParseObjectContext(ctx, s)
		if err != nil {
			return nil, err
		}
		if len(*s) >= before {
			return nil, errors.New("PDF operand parser made no progress")
		}
		return v, nil
	}
	i := 0
	for i < len(*s) && !pdfDelimiter((*s)[i]) {
		i++
	}
	if i == 0 {
		return nil, fmt.Errorf("unexpected PDF token %q", (*s)[:1])
	}
	v := pdfOperator((*s)[:i])
	*s = (*s)[i:]
	return v, nil
}
func pdfString(o any) ([]byte, error) {
	switch v := o.(type) {
	case types.StringLiteral:
		return types.Unescape(string(v))
	case types.HexLiteral:
		return v.Bytes()
	}
	return nil, errors.New("expected PDF string")
}
func pdfNumber(o any) (float64, error) {
	switch v := o.(type) {
	case types.Integer:
		return float64(v), nil
	case types.Float:
		return float64(v), nil
	}
	return 0, errors.New("expected PDF number")
}
func pdfName(o any) (string, error) {
	if n, ok := o.(types.Name); ok {
		return string(n), nil
	}
	return "", errors.New("expected PDF name")
}
func utf16Text(b []byte) (string, error) {
	if len(b)%2 != 0 {
		return "", errors.New("odd-length UTF-16 text")
	}
	u := make([]uint16, len(b)/2)
	for i := range u {
		u[i] = binary.BigEndian.Uint16(b[2*i:])
	}
	for i := 0; i < len(u); i++ {
		if u[i] >= 0xD800 && u[i] <= 0xDBFF {
			if i+1 >= len(u) || u[i+1] < 0xDC00 || u[i+1] > 0xDFFF {
				return "", errors.New("unpaired UTF-16 surrogate")
			}
			i++
		} else if u[i] >= 0xDC00 && u[i] <= 0xDFFF {
			return "", errors.New("unpaired UTF-16 surrogate")
		}
	}
	return string(utf16.Decode(u)), nil
}

type codeRange struct {
	low, high uint32
	size      int
	dest      []byte
	values    [][]byte
}
type unicodeMap struct {
	chars  map[string]string
	ranges []codeRange
	spaces []codeRange
}

func codeNumber(b []byte) uint32 {
	var v uint32
	for _, c := range b {
		v = v<<8 | uint32(c)
	}
	return v
}
func parseUnicodeMap(ctx context.Context, b []byte) (*unicodeMap, error) {
	m := &unicodeMap{chars: map[string]string{}}
	s := string(b)
	mode := ""
	var args []any
	for s != "" {
		v, err := nextPDFToken(ctx, &s)
		if err != nil {
			return nil, err
		}
		if v == nil {
			break
		}
		if op, ok := v.(pdfOperator); ok {
			switch string(op) {
			case "begincodespacerange", "beginbfchar", "beginbfrange":
				mode = string(op)
				args = nil
				continue
			case "endcodespacerange", "endbfchar", "endbfrange":
				step := 2
				if mode == "beginbfrange" {
					step = 3
				}
				if len(args)%step != 0 {
					return nil, errors.New("malformed ToUnicode mapping")
				}
				for i := 0; i < len(args); i += step {
					lo, err := pdfString(args[i])
					if err != nil || len(lo) == 0 || len(lo) > 4 {
						return nil, errors.New("invalid ToUnicode source code")
					}
					if mode == "beginbfchar" {
						dst, err := pdfString(args[i+1])
						if err != nil {
							return nil, err
						}
						text, err := utf16Text(dst)
						if err != nil {
							return nil, err
						}
						m.chars[string(lo)] = text
						continue
					}
					hi, err := pdfString(args[i+1])
					if err != nil || len(lo) != len(hi) || codeNumber(hi) < codeNumber(lo) {
						return nil, errors.New("invalid ToUnicode code range")
					}
					r := codeRange{low: codeNumber(lo), high: codeNumber(hi), size: len(lo)}
					if mode == "begincodespacerange" {
						m.spaces = append(m.spaces, r)
						continue
					}
					if arr, ok := args[i+2].(types.Array); ok {
						if uint64(len(arr)) != uint64(r.high)-uint64(r.low)+1 {
							return nil, errors.New("ToUnicode range array length mismatch")
						}
						for _, value := range arr {
							dst, err := pdfString(value)
							if err != nil {
								return nil, err
							}
							r.values = append(r.values, dst)
						}
					} else {
						r.dest, err = pdfString(args[i+2])
						if err != nil {
							return nil, err
						}
					}
					m.ranges = append(m.ranges, r)
				}
				mode = ""
				args = nil
				continue
			}
			if mode != "" {
				return nil, fmt.Errorf("unexpected ToUnicode operator %q", op)
			}
			args = nil
			continue
		}
		if mode != "" {
			args = append(args, v)
		}
	}
	if mode != "" {
		return nil, errors.New("incomplete ToUnicode map")
	}
	if len(m.chars) == 0 && len(m.ranges) == 0 {
		return nil, errors.New("empty ToUnicode map")
	}
	return m, nil
}
func (m *unicodeMap) decode(b []byte) (string, error) {
	var out strings.Builder
	for len(b) > 0 {
		size := 0
		if len(m.spaces) > 0 {
			for _, r := range m.spaces {
				if len(b) >= r.size {
					v := codeNumber(b[:r.size])
					if v >= r.low && v <= r.high {
						size = r.size
						break
					}
				}
			}
		} else {
			for n := 1; n <= 4 && n <= len(b); n++ {
				if _, ok := m.chars[string(b[:n])]; ok {
					size = n
					break
				}
				for _, r := range m.ranges {
					if r.size == n && codeNumber(b[:n]) >= r.low && codeNumber(b[:n]) <= r.high {
						size = n
						break
					}
				}
				if size > 0 {
					break
				}
			}
		}
		if size == 0 {
			return "", errors.New("text code outside ToUnicode codespace")
		}
		code := b[:size]
		b = b[size:]
		if text, ok := m.chars[string(code)]; ok {
			out.WriteString(text)
			continue
		}
		found := false
		v := codeNumber(code)
		for _, r := range m.ranges {
			if r.size != size || v < r.low || v > r.high {
				continue
			}
			offset := v - r.low
			var dst []byte
			if r.values != nil {
				dst = r.values[offset]
			} else {
				dst = append([]byte(nil), r.dest...)
				carry := uint64(offset)
				for j := len(dst) - 1; j >= 0 && carry > 0; j-- {
					carry += uint64(dst[j])
					dst[j] = byte(carry)
					carry >>= 8
				}
				if carry > 0 {
					return "", errors.New("overflow in ToUnicode destination")
				}
			}
			text, err := utf16Text(dst)
			if err != nil {
				return "", err
			}
			out.WriteString(text)
			found = true
			break
		}
		if !found {
			return "", fmt.Errorf("unmapped ToUnicode code %s", strconv.FormatUint(uint64(v), 16))
		}
	}
	return out.String(), nil
}
