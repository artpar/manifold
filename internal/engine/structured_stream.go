package engine

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
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

// Read directly into the typed model one top-level body node at a time. Public
// ReadStructured still returns a complete SF tree; Read need not retain that tree
// alongside the model for multi-gigabyte source/provenance-heavy documents.
func readFlowStructured(ctx context.Context, input io.Reader) (*Document, error) {
	if input == nil {
		return nil, fmt.Errorf("nil document reader")
	}
	dec := json.NewDecoder(contextReader{ctx, input})
	dec.DisallowUnknownFields()
	opening, err := dec.Token()
	if err != nil {
		return nil, err
	}
	if opening != json.Delim('{') {
		return nil, fmt.Errorf("structured JSON requires object root")
	}
	root := &StructuredDocument{}
	seen := map[string]bool{}
	var blocks []Block
	rawValue := func(target any) error {
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return err
		}
		validator := strictJSONParser{ctx: ctx, data: raw, rejectUnknown: true}
		if err := validator.validate(reflect.TypeOf(target).Elem()); err != nil {
			return err
		}
		return json.Unmarshal(raw, target)
	}
	for dec.More() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		key, err := dec.Token()
		if err != nil {
			return nil, err
		}
		name, ok := key.(string)
		if !ok || seen[name] {
			return nil, fmt.Errorf("duplicate or invalid structured root key")
		}
		seen[name] = true
		switch name {
		case "type":
			if err := rawValue(&root.Type); err != nil {
				return nil, err
			}
		case "attrs":
			if err := rawValue(&root.Attrs); err != nil {
				return nil, err
			}
		case "content":
			token, err := dec.Token()
			if err != nil {
				return nil, err
			}
			if token == nil {
				continue
			}
			if token != json.Delim('[') {
				return nil, fmt.Errorf("structured content requires array")
			}
			for dec.More() {
				var node StructuredNode
				if err := rawValue(&node); err != nil {
					return nil, err
				}
				if node.Type == "presentation" {
					root.Content = append(root.Content, node)
					continue
				}
				bs, err := decodeSFBlocks([]StructuredNode{node}, 0)
				if err != nil {
					return nil, err
				}
				blocks = append(blocks, bs...)
			}
			if _, err := dec.Token(); err != nil {
				return nil, err
			}
		default:
			return nil, fmt.Errorf("unknown structured root key %q", name)
		}
	}
	if _, err := dec.Token(); err != nil {
		return nil, err
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		if err != nil {
			return nil, fmt.Errorf("structured JSON trailing data: %w", err)
		}
		return nil, fmt.Errorf("structured JSON must contain one value")
	}
	if root.Attrs.Presentation == nil && len(root.Content) > 0 {
		return nil, sfUnsupported("presentation")
	}
	if root.Attrs.Presentation != nil && len(blocks) > 0 {
		return nil, fmt.Errorf("presentation and flow SF cannot coexist")
	}
	d, err := fromStructured(root, false)
	if err != nil {
		return nil, err
	}
	d.Blocks = blocks
	if err := d.Validate(); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return d, nil
}
