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
	d = resolveNumberingDocument(d)
	root, e := toStructured(d, false)
	if e != nil {
		return e
	}
	blocks := legacyLists(d.Blocks)
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
