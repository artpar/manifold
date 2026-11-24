package manydoc

import (
	"bufio"
	"encoding/json"
	"io"
)

// Stream the body tree rather than buffering the JSON for every paragraph/table
// at once. Root attributes and each node's own attributes/marks remain ordinary
// JSON marshals, so assets and running/note trees are still materialized values.
func encodeStructuredJSON(w io.Writer, s *StructuredDocument) error {
	out := bufio.NewWriter(w)
	write := func(p []byte) error { _, e := out.Write(p); return e }
	value := func(v any) error {
		p, e := json.Marshal(v)
		if e != nil {
			return e
		}
		return write(p)
	}
	if e := write([]byte(`{"type":`)); e != nil {
		return e
	}
	if e := value(s.Type); e != nil {
		return e
	}
	if e := write([]byte(`,"attrs":`)); e != nil {
		return e
	}
	if e := value(s.Attrs); e != nil {
		return e
	}
	var node func(StructuredNode) error
	node = func(n StructuredNode) error {
		children := n.Content
		n.Content = nil
		p, e := json.Marshal(n)
		if e != nil {
			return e
		}
		if len(children) == 0 {
			return write(p)
		}
		if e = write(p[:len(p)-1]); e != nil {
			return e
		}
		if e = write([]byte(`,"content":[`)); e != nil {
			return e
		}
		for i, child := range children {
			if i > 0 {
				if e = out.WriteByte(','); e != nil {
					return e
				}
			}
			if e = node(child); e != nil {
				return e
			}
		}
		return write([]byte(`]}`))
	}
	if len(s.Content) > 0 {
		if e := write([]byte(`,"content":[`)); e != nil {
			return e
		}
		for i, n := range s.Content {
			if i > 0 {
				if e := out.WriteByte(','); e != nil {
					return e
				}
			}
			if e := node(n); e != nil {
				return e
			}
		}
		if e := out.WriteByte(']'); e != nil {
			return e
		}
	}
	if e := write([]byte("}\n")); e != nil {
		return e
	}
	return out.Flush()
}
