package manifold

import (
	"strings"
	"unicode"
)

// Explicit replacement text can restore whitespace consumed at line wrapping.
// This is character alignment, not fuzzy matching: every painted non-synthetic
// character must match in order. Only missing Unicode whitespace may be inserted;
// style/relationship/source ownership stays on each original character segment.
func alignPDFActualWhitespace(blocks []Block, text string, n *pdfStructureNode) ([]Block, bool) {
	if n.role != "Span" || len(n.refs) > 0 || len(blocks) != 1 || blocks[0].Kind != Paragraph {
		return nil, false
	}
	target := []rune(text)
	at := 0
	var out []Run
	var last Run
	insert := func(space string, style Run) {
		style.Text = space
		style.Source = &SourceInfo{Format: PDF, Role: n.role, Element: n.id, ParentElement: n.parent, Structure: "explicit", Evidence: "pdf_structure_actualtext_whitespace;adjacent_style_inferred", Confidence: .75, Precision: "replacement_whitespace"}
		out = append(out, style)
	}
	for _, run := range blocks[0].Runs {
		if run.Source != nil && run.Source.Precision == "synthetic_separator" {
			continue
		}
		if run.Note != nil || run.Anchor != "" {
			return nil, false
		}
		var chunk strings.Builder
		flush := func() {
			if chunk.Len() > 0 {
				part := run
				part.Text = chunk.String()
				out = append(out, part)
				chunk.Reset()
			}
		}
		for _, ch := range run.Text {
			start := at
			for at < len(target) && target[at] != ch && unicode.IsSpace(target[at]) && !unicode.IsSpace(ch) {
				at++
			}
			if at > start {
				flush()
				insert(string(target[start:at]), run)
			}
			if at >= len(target) || target[at] != ch {
				return nil, false
			}
			chunk.WriteRune(ch)
			at++
		}
		flush()
		last = run
	}
	start := at
	for at < len(target) && unicode.IsSpace(target[at]) {
		at++
	}
	if at != len(target) {
		return nil, false
	}
	if at > start {
		insert(string(target[start:at]), last)
	}
	block := blocks[0]
	block.Runs = out
	return []Block{block}, true
}
