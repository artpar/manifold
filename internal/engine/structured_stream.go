package engine

import (
	"compress/gzip"
	"context"
	"io"
)

// Preflight the complete document and every generated body chunk before any
// output. Keep cross-reference validation global, but avoid simultaneously
// retaining a second SF tree and a third decoded model for source-heavy PDFs.
func writeFlowStructured(ctx context.Context, w io.Writer, d *Document, compressed bool) error {
	if flowNeedsNumbering(d) {
		d = resolveNumberingDocument(d)
	}
	root, e := toStructured(d, false)
	if e != nil {
		return e
	}
	blocks := d.Blocks
	for _, b := range blocks {
		if (b.Kind == Paragraph || b.Kind == Heading) && b.List != "" {
			blocks = legacyLists(blocks)
			break
		}
	}
	for _, b := range blocks {
		if e := ctx.Err(); e != nil {
			return e
		}
		if _, e := encodeSFBlocks([]Block{b}); e != nil {
			return e
		}
	}

	target := io.Writer(contextWriter{ctx, w})
	var gz *gzip.Writer
	if compressed {
		gz = gzip.NewWriter(target)
		target = gz
	}
	produce := func(yield func(StructuredNode) error) error {
		for _, b := range blocks {
			if e := ctx.Err(); e != nil {
				return e
			}
			ns, e := encodeSFBlocks([]Block{b})
			if e != nil {
				return e
			}
			for _, n := range ns {
				if e := yield(n); e != nil {
					return e
				}
			}
		}
		return nil
	}
	if e = encodeStructuredJSONStream(target, root, produce); e != nil {
		return e
	}
	if gz != nil {
		return gz.Close()
	}
	return nil
}

func flowNeedsNumbering(d *Document) bool {
	var blocks func([]Block) bool
	blocks = func(bs []Block) bool {
		for _, b := range bs {
			if b.Numbering != nil {
				return true
			}
			if b.Section != nil {
				if blocks(b.Section.Blocks) {
					return true
				}
				for _, s := range sectionStories(b.Section) {
					if s != nil && blocks(s.Blocks) {
						return true
					}
				}
			}
			for _, i := range b.Items {
				if blocks(i.Blocks) || (i.Marker != nil && blocks(i.Marker.Blocks)) {
					return true
				}
			}
			for _, row := range b.Rows {
				for _, c := range row.Cells {
					if blocks(c.Blocks) {
						return true
					}
				}
			}
		}
		return false
	}
	if blocks(d.Blocks) || blocks(d.Headers) || blocks(d.Footers) {
		return true
	}
	for _, n := range d.Notes {
		if blocks(n.Blocks) {
			return true
		}
	}
	return false
}
