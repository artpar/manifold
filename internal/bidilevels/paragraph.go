// Package bidilevels retains paragraph embedding levels for line layout.
// It adapts the BSD-licensed x/text v0.30.0 reference core; see README.md.
package bidilevels

import (
	"context"
	_ "embed"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/unicode/bidi"
)

//go:embed data/BidiBrackets.txt
var bracketData string

// Canonical equivalence of U+2329/U+232A and U+3008/U+3009 is part of BD16.
// The published paired-bracket property supplies all other pairs unchanged.
var pairedBracketValues = func() map[rune]rune {
	pairs := make(map[rune]rune)
	for _, line := range strings.Split(bracketData, "\n") {
		line = strings.TrimSpace(strings.SplitN(line, "#", 2)[0])
		if line == "" {
			continue
		}
		fields := strings.Split(line, ";")
		if len(fields) != 3 {
			panic("invalid embedded Unicode bracket data")
		}
		r, err := strconv.ParseInt(strings.TrimSpace(fields[0]), 16, 32)
		if err != nil {
			panic(err)
		}
		pair, err := strconv.ParseInt(strings.TrimSpace(fields[1]), 16, 32)
		if err != nil {
			panic(err)
		}
		open := rune(r)
		if strings.TrimSpace(fields[2]) == "c" {
			open = rune(pair)
		}
		if open == 0x2329 {
			open = 0x3008
		}
		pairs[rune(r)] = open
	}
	return pairs
}()

// Paragraph is immutable after resolution. Its methods return owned slices and
// may be called concurrently. Input/output positions count Unicode scalars,
// never UTF-8 bytes or normalized scalars. No normalization is performed.
type Paragraph struct {
	p    *paragraph
	base uint8
}

// Resolve resolves one paragraph using UAX #9 through I2. base is -1 (automatic),
// 0 (explicit LTR) or 1 (explicit RTL). An explicit base is not a fallback and is
// not overridden by the first strong character. A B-class separator is accepted
// only at the end; callers must split paragraphs before resolution.
func Resolve(ctx context.Context, text string, base int) (*Paragraph, error) {
	return resolve(ctx, text, base, nil)
}

// ResolveWithTypes supplies higher-level-protocol bidi classes while retaining
// the original runes for bracket identity. Classes use original scalar indices.
func ResolveWithTypes(ctx context.Context, text string, base int, types []Class) (*Paragraph, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(types) != utf8.RuneCountInString(text) {
		return nil, fmt.Errorf("bidi class override length mismatch")
	}
	for _, c := range types {
		if c > PDI || c >= Control && c < LRO {
			return nil, fmt.Errorf("invalid bidi class override")
		}
	}
	return resolve(ctx, text, base, types)
}
func resolve(ctx context.Context, text string, base int, overrides []Class) (result *Paragraph, err error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if base < -1 || base > 1 || !utf8.ValidString(text) {
		return nil, fmt.Errorf("invalid bidi paragraph base or UTF-8")
	}
	if utf8.RuneCountInString(text) > 65536 {
		return nil, fmt.Errorf("bidi paragraph exceeds 65536-scalar resource limit")
	}
	defer func() {
		if p := recover(); p != nil {
			result, err = nil, fmt.Errorf("bidi paragraph resolution failed: %v", p)
		}
	}()
	runes := []rune(text)
	if len(runes) == 0 {
		return &Paragraph{base: uint8(max(0, base))}, nil
	}
	types := make([]Class, len(runes))
	pairs := make([]bracketType, len(runes))
	values := make([]rune, len(runes))
	for i, r := range runes {
		if i%1024 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		properties, _ := bidi.LookupRune(r)
		types[i] = Class(properties.Class())
		if overrides != nil {
			types[i] = overrides[i]
		}
		if open, ok := pairedBracketValues[r]; ok {
			values[i] = open
			pairs[i] = bpClose
			if properties.IsOpeningBracket() {
				pairs[i] = bpOpen
			}
		}
	}
	p, err := newParagraph(types, pairs, values, level(base))
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return &Paragraph{p: p, base: uint8(p.embeddingLevel)}, nil
}

func (p *Paragraph) Len() int {
	if p.p == nil {
		return 0
	}
	return p.p.Len()
}

func (p *Paragraph) Base() uint8 { return p.base }

// Levels returns resolved paragraph levels before the line-specific L1 resets.
// Their parity selects shaping direction; the complete values are needed by L2.
func (p *Paragraph) Levels() []uint8 {
	if p.p == nil {
		return nil
	}
	out := make([]uint8, p.Len())
	for i, l := range p.p.resultLevels {
		out[i] = uint8(l)
	}
	return out
}

type Line struct {
	Levels []uint8 // logical scalar order after L1
	Order  []int   // visual -> original paragraph scalar index after L2
}

// Line applies L1/L2 to [start,end) without re-resolving its weak/neutral types,
// isolates or base direction. The result includes X9 controls (whose placement
// is non-normative); the rendering caller must keep them logical but unpainted.
// L3 grapheme/mark positioning and L4 mirroring belong to the glyph shaper.
func (p *Paragraph) Line(start, end int) (Line, error) {
	if start < 0 || end < start || end > p.Len() {
		return Line{}, fmt.Errorf("invalid bidi line range [%d,%d)", start, end)
	}
	if start == end {
		return Line{}, nil
	}
	// Copy only the requested line. Paragraph resolution is never repeated.
	levels := append([]level(nil), p.p.resultLevels[start:end]...)
	for i, t := range p.p.initialTypes[start:end] {
		if t.in(B, S) {
			levels[i] = level(p.base)
			for j := i - 1; j >= 0 && isWhitespace(p.p.initialTypes[start+j]); j-- {
				levels[j] = level(p.base)
			}
		}
	}
	for j := len(levels) - 1; j >= 0 && isWhitespace(p.p.initialTypes[start+j]); j-- {
		levels[j] = level(p.base)
	}
	out := Line{Levels: make([]uint8, len(levels)), Order: computeReordering(levels)}
	for i, l := range levels {
		out.Levels[i] = uint8(l)
		out.Order[i] += start
	}
	return out, nil
}
