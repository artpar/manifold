package manifold

import (
	"context"
	"fmt"
	"unicode/utf8"
)

// Native Univer snapshots index UTF-16 code units. Go strings remain UTF-8;
// byteAt marks the interior of an astral scalar with -1 so no range operation
// can slice one surrogate half or silently replace visible text.
type colleafTextIndex struct {
	text   string
	byteAt []int
}

func newColleafTextIndex(ctx context.Context, text string) (colleafTextIndex, error) {
	if err := ctx.Err(); err != nil {
		return colleafTextIndex{}, err
	}
	if !utf8.ValidString(text) {
		return colleafTextIndex{}, fmt.Errorf("native document text is not UTF-8")
	}
	index := colleafTextIndex{text: text, byteAt: make([]int, 0, len(text)+1)}
	count := 0
	for offset, ch := range text {
		if count%1024 == 0 {
			if err := ctx.Err(); err != nil {
				return colleafTextIndex{}, err
			}
		}
		count++
		index.byteAt = append(index.byteAt, offset)
		if ch > 0xffff {
			index.byteAt = append(index.byteAt, -1)
		}
	}
	index.byteAt = append(index.byteAt, len(text))
	return index, nil
}
func (s colleafTextIndex) length() int { return len(s.byteAt) - 1 }
func (s colleafTextIndex) byteBoundary(offset int) (int, error) {
	if offset < 0 || offset >= len(s.byteAt) || s.byteAt[offset] < 0 {
		return 0, fmt.Errorf("native UTF-16 offset %d is outside a scalar boundary", offset)
	}
	return s.byteAt[offset], nil
}
func (s colleafTextIndex) exclusive(start, end int) (string, error) {
	if end < start {
		return "", fmt.Errorf("native range end precedes start")
	}
	a, err := s.byteBoundary(start)
	if err != nil {
		return "", err
	}
	b, err := s.byteBoundary(end)
	if err != nil {
		return "", err
	}
	return s.text[a:b], nil
}
func (s colleafTextIndex) inclusive(start, last int) (string, error) {
	if last < start || last < 0 || last >= s.length() {
		return "", fmt.Errorf("invalid inclusive native range")
	}
	return s.exclusive(start, last+1)
}

// Published interface prose has stale table-end token comments. These values
// agree with @univerjs/core 1.0.4 DataStreamTreeTokenType and executable parser.
const (
	colleafParagraph        = '\r'
	colleafSection          = '\n'
	colleafTableStart       = '\x1a'
	colleafRowStart         = '\x1b'
	colleafCellStart        = '\x1c'
	colleafCellEnd          = '\x1d'
	colleafRowEnd           = '\x0e'
	colleafTableEnd         = '\x0f'
	colleafCustomRangeStart = '\x1f'
	colleafCustomRangeEnd   = '\x1e'
	colleafCustomBlock      = '\b'
)

type colleafStreamNode struct {
	kind       string
	start, end int // UTF-16, inclusive/exclusive, including structural sentinels
	children   []colleafStreamNode
}

// Sections, including those inside cells, end in LF. Every section ends in a
// paragraph CR immediately before its LF. Tables live inside paragraph extents;
// cell bodies therefore need their own CR/LF, not just CR. No document content
// is inferred from unbalanced/missing tokens.
func parseColleafStream(ctx context.Context, text string) ([]colleafStreamNode, error) {
	index, err := newColleafTextIndex(ctx, text)
	if err != nil {
		return nil, err
	}
	return parseColleafIndexedStream(ctx, index)
}
func parseColleafIndexedStream(ctx context.Context, index colleafTextIndex) ([]colleafStreamNode, error) {
	p := colleafStreamParser{index: index, ctx: ctx}
	nodes, err := p.sections(false, 0)
	if err != nil {
		return nil, err
	}
	if p.offset != index.length() || p.ranges != 0 || len(nodes) == 0 {
		return nil, fmt.Errorf("unconsumed native document stream")
	}
	return nodes, nil
}

type colleafStreamParser struct {
	ctx    context.Context
	index  colleafTextIndex
	offset int
	ranges int
}

func (p *colleafStreamParser) peek() rune {
	b := p.index.byteAt[p.offset]
	if b == len(p.index.text) {
		return -1
	}
	ch, _ := utf8.DecodeRuneInString(p.index.text[b:])
	return ch
}
func (p *colleafStreamParser) advance() {
	ch := p.peek()
	if ch > 0xffff {
		p.offset += 2
	} else {
		p.offset++
	}
}
func (p *colleafStreamParser) sections(cell bool, depth int) ([]colleafStreamNode, error) {
	if depth > 64 {
		return nil, fmt.Errorf("native stream nesting exceeds 64")
	}
	var sections, paragraphs []colleafStreamNode
	sectionStart, paragraphStart := p.offset, p.offset
	var tables []colleafStreamNode
	for p.offset < p.index.length() {
		if err := p.ctx.Err(); err != nil {
			return nil, err
		}
		ch := p.peek()
		switch ch {
		case colleafCellEnd:
			if !cell || p.offset != sectionStart || len(paragraphs) > 0 || len(tables) > 0 {
				return nil, fmt.Errorf("native cell closes before its section terminator")
			}
			return sections, nil
		case colleafTableStart:
			table, err := p.table(depth + 1)
			if err != nil {
				return nil, err
			}
			tables = append(tables, table)
			continue
		case colleafParagraph:
			p.advance()
			paragraphs = append(paragraphs, colleafStreamNode{kind: "paragraph", start: paragraphStart, end: p.offset, children: tables})
			tables = nil
			paragraphStart = p.offset
		case colleafSection:
			if p.offset != paragraphStart || len(paragraphs) == 0 {
				return nil, fmt.Errorf("native section requires a final paragraph terminator")
			}
			p.advance()
			sections = append(sections, colleafStreamNode{kind: "section", start: sectionStart, end: p.offset, children: paragraphs})
			paragraphs = nil
			sectionStart = p.offset
			paragraphStart = p.offset
		case colleafCustomRangeStart:
			p.ranges++
		case colleafCustomRangeEnd:
			p.ranges--
			if p.ranges < 0 {
				return nil, fmt.Errorf("unmatched native range end")
			}
		case colleafRowStart, colleafRowEnd, colleafCellStart, colleafTableEnd:
			return nil, fmt.Errorf("unexpected native table control U+%04X", ch)
		case 0, '\x10', '\x11', '\x12', '\x13', '\x14', '\x15':
			return nil, fmt.Errorf("%w: native stream control U+%04X needs its own representation", ErrUnsupported, ch)
		default:
			if ch < 32 && ch != '\t' && ch != '\b' && ch != '\f' && ch != '\v' {
				return nil, fmt.Errorf("unsupported native stream control U+%04X", ch)
			}
		}
		// CR/LF cases have already advanced through their structural token.
		if ch != colleafParagraph && ch != colleafSection {
			p.advance()
		}
	}
	if cell || sectionStart != p.offset || len(paragraphs) > 0 || len(tables) > 0 {
		return nil, fmt.Errorf("native stream lacks a complete section/cell terminator")
	}
	return sections, nil
}
func (p *colleafStreamParser) table(depth int) (colleafStreamNode, error) {
	if depth > 64 {
		return colleafStreamNode{}, fmt.Errorf("native stream nesting exceeds 64")
	}
	table := colleafStreamNode{kind: "table", start: p.offset}
	p.advance()
	for p.offset < p.index.length() && p.peek() == colleafRowStart {
		row := colleafStreamNode{kind: "row", start: p.offset}
		p.advance()
		for p.offset < p.index.length() && p.peek() == colleafCellStart {
			cell := colleafStreamNode{kind: "cell", start: p.offset}
			p.advance()
			children, err := p.sections(true, depth+1)
			if err != nil {
				return table, err
			}
			if len(children) == 0 || p.offset >= p.index.length() || p.peek() != colleafCellEnd {
				return table, fmt.Errorf("native table cell is missing content/closing token")
			}
			p.advance()
			cell.end = p.offset
			cell.children = children
			row.children = append(row.children, cell)
		}
		if len(row.children) == 0 || p.offset >= p.index.length() || p.peek() != colleafRowEnd {
			return table, fmt.Errorf("native table row is missing cells/closing token")
		}
		p.advance()
		row.end = p.offset
		table.children = append(table.children, row)
	}
	if len(table.children) == 0 || p.offset >= p.index.length() || p.peek() != colleafTableEnd {
		return table, fmt.Errorf("native table is missing rows/closing token")
	}
	p.advance()
	table.end = p.offset
	return table, nil
}
