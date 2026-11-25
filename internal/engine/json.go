package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"unicode/utf8"
)

// Check before encoding/json can replace malformed Unicode or collapse duplicate
// keys. Only decoded object keys are allocated; large image/string values are
// scanned in place. The depth bound protects this parser, not application quotas.
func validateStrictJSON(ctx context.Context, data []byte) error {
	return validateTypedJSON(ctx, data, nil)
}

func validateTypedJSON(ctx context.Context, data []byte, shape reflect.Type) error {
	p := strictJSONParser{ctx: ctx, data: data}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := p.value(0, shape); err != nil {
		return err
	}
	if err := p.space(); err != nil {
		return err
	}
	if p.at != len(data) {
		return p.invalid("trailing data")
	}
	return ctx.Err()
}

type strictJSONParser struct {
	ctx         context.Context
	data        []byte
	at, checked int
	fields      map[reflect.Type]map[string]reflect.Type
}

func (p *strictJSONParser) invalid(s string) error { return fmt.Errorf("JSON byte %d: %s", p.at, s) }
func (p *strictJSONParser) check() error {
	if p.at-p.checked >= 4096 {
		p.checked = p.at
		return p.ctx.Err()
	}
	return nil
}
func (p *strictJSONParser) space() error {
	for p.at < len(p.data) {
		switch p.data[p.at] {
		case ' ', '\t', '\r', '\n':
			p.at++
		default:
			return p.check()
		}
		if err := p.check(); err != nil {
			return err
		}
	}
	return nil
}
func (p *strictJSONParser) consume(c byte) bool {
	if p.at < len(p.data) && p.data[p.at] == c {
		p.at++
		return true
	}
	return false
}
func (p *strictJSONParser) value(depth int, typ reflect.Type) error {
	// Match native field names exactly. encoding/json accepts case aliases, whereas
	// the editor reads case-sensitive JavaScript properties. Do not reinterpret one
	// as the other. Raw property maps retain their existing semantic diagnostics.
	nullable := typ == nil
	if typ != nil {
		nullable = typ.Kind() == reflect.Pointer || typ.Kind() == reflect.Map || typ.Kind() == reflect.Slice
		for typ.Kind() == reflect.Pointer {
			typ = typ.Elem()
		}
		if typ == reflect.TypeFor[json.RawMessage]() {
			typ = nil
			nullable = true
		}
	}
	var child reflect.Type
	if typ != nil && (typ.Kind() == reflect.Map || typ.Kind() == reflect.Slice || typ.Kind() == reflect.Array) {
		child = typ.Elem()
	}

	if err := p.space(); err != nil {
		return err
	}
	if depth > 128 {
		return p.invalid("nesting exceeds 128")
	}
	if p.at == len(p.data) {
		return p.invalid("missing value")
	}
	if p.data[p.at] == 'n' && !nullable {
		return p.invalid("null for nonnullable native field")
	}
	switch p.data[p.at] {
	case '{':
		p.at++
		if err := p.space(); err != nil {
			return err
		}
		if p.consume('}') {
			return nil
		}
		keys := make(map[string]bool)
		for {
			start := p.at
			if err := p.string(); err != nil {
				return err
			}
			var key string
			if err := json.Unmarshal(p.data[start:p.at], &key); err != nil {
				return err
			}
			if keys[key] {
				return p.invalid("duplicate object member")
			}
			keys[key] = true
			if err := p.space(); err != nil {
				return err
			}
			if !p.consume(':') {
				return p.invalid("missing colon")
			}
			next := child
			if typ != nil && typ.Kind() == reflect.Struct {
				fields := p.recordFields(typ)
				next = fields[key]
				if next == nil {
					for known := range fields {
						if strings.EqualFold(key, known) {
							return p.invalid("noncanonical native field name")
						}
					}
				}
			}
			if err := p.value(depth+1, next); err != nil {
				return err
			}
			if err := p.space(); err != nil {
				return err
			}
			if p.consume('}') {
				return nil
			}
			if !p.consume(',') {
				return p.invalid("missing object comma")
			}
			if err := p.space(); err != nil {
				return err
			}
		}
	case '[':
		p.at++
		if err := p.space(); err != nil {
			return err
		}
		if p.consume(']') {
			return nil
		}
		for {
			if err := p.value(depth+1, child); err != nil {
				return err
			}
			if err := p.space(); err != nil {
				return err
			}
			if p.consume(']') {
				return nil
			}
			if !p.consume(',') {
				return p.invalid("missing array comma")
			}
		}
	case '"':
		return p.string()
	case 't':
		return p.literal("true")
	case 'f':
		return p.literal("false")
	case 'n':
		return p.literal("null")
	default:
		return p.number()
	}
}
func (p *strictJSONParser) literal(s string) error {
	if len(p.data)-p.at < len(s) || string(p.data[p.at:p.at+len(s)]) != s {
		return p.invalid("invalid literal")
	}
	p.at += len(s)
	return nil
}
func (p *strictJSONParser) digit() bool {
	return p.at < len(p.data) && p.data[p.at] >= '0' && p.data[p.at] <= '9'
}
func (p *strictJSONParser) digits() error {
	if !p.digit() {
		return p.invalid("missing number digits")
	}
	for p.digit() {
		p.at++
		if err := p.check(); err != nil {
			return err
		}
	}
	return nil
}
func (p *strictJSONParser) number() error {
	p.consume('-')
	if !p.consume('0') {
		if err := p.digits(); err != nil {
			return err
		}
	}
	if p.consume('.') {
		if err := p.digits(); err != nil {
			return err
		}
	}
	if p.consume('e') || p.consume('E') {
		if !p.consume('+') {
			p.consume('-')
		}
		return p.digits()
	}
	return nil
}
func (p *strictJSONParser) hex4() (uint16, error) {
	var n uint16
	for range 4 {
		if p.at == len(p.data) {
			return 0, p.invalid("short Unicode escape")
		}
		c := p.data[p.at]
		p.at++
		var d byte
		switch {
		case c >= '0' && c <= '9':
			d = c - '0'
		case c >= 'a' && c <= 'f':
			d = c - 'a' + 10
		case c >= 'A' && c <= 'F':
			d = c - 'A' + 10
		default:
			return 0, p.invalid("invalid Unicode escape")
		}
		n = n*16 + uint16(d)
	}
	return n, nil
}
func (p *strictJSONParser) string() error {
	if !p.consume('"') {
		return p.invalid("expected string")
	}
	for p.at < len(p.data) {
		if err := p.check(); err != nil {
			return err
		}
		c := p.data[p.at]
		p.at++
		switch {
		case c == '"':
			return nil
		case c < 0x20:
			return p.invalid("unescaped control in string")
		case c == '\\':
			if p.at == len(p.data) {
				return p.invalid("unfinished string escape")
			}
			esc := p.data[p.at]
			p.at++
			switch esc {
			case '"', '\\', '/', 'b', 'f', 'n', 'r', 't':
			case 'u':
				n, err := p.hex4()
				if err != nil {
					return err
				}
				if n >= 0xdc00 && n <= 0xdfff {
					return p.invalid("unpaired low surrogate")
				}
				if n >= 0xd800 && n <= 0xdbff {
					if !p.consume('\\') || !p.consume('u') {
						return p.invalid("unpaired high surrogate")
					}
					low, err := p.hex4()
					if err != nil {
						return err
					}
					if low < 0xdc00 || low > 0xdfff {
						return p.invalid("unpaired high surrogate")
					}
				}
			default:
				return p.invalid("invalid string escape")
			}
		case c >= utf8.RuneSelf:
			_, n := utf8.DecodeRune(p.data[p.at-1:])
			if n == 1 {
				return p.invalid("invalid UTF-8")
			}
			p.at += n - 1
		}
	}
	return p.invalid("unfinished string")
}

func (p *strictJSONParser) recordFields(typ reflect.Type) map[string]reflect.Type {
	if p.fields == nil {
		p.fields = make(map[reflect.Type]map[string]reflect.Type)
	}
	if fields, ok := p.fields[typ]; ok {
		return fields
	}
	fields := make(map[string]reflect.Type)
	p.fields[typ] = fields
	for i := range typ.NumField() {
		field := typ.Field(i)
		tag := strings.Split(field.Tag.Get("json"), ",")[0]
		if tag == "-" {
			continue
		}
		if field.Anonymous && tag == "" && field.Type.Kind() == reflect.Struct {
			for name, value := range p.recordFields(field.Type) {
				fields[name] = value
			}
		} else if field.PkgPath == "" {
			if tag == "" {
				tag = field.Name
			}
			fields[tag] = field.Type
		}
	}
	return fields
}

func validateVDOCSJSON(ctx context.Context, data []byte) error {
	return validateTypedJSON(ctx, data, reflect.TypeFor[vdocsEnvelope]())
}
