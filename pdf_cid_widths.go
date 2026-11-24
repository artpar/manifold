package manydoc

import (
	"fmt"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

type pdfCIDWidthAssignment struct {
	first, last uint32
	width       float64
	values      []float64
}

// Parse source assignments once. Applying them from the end can skip CIDs already
// owned by a later assignment, preserving existing last-assignment semantics
// without expanding every overlapping range repeatedly.
func (r *pdfReader) readCIDWidths(f *pdfFont, widths types.Array) error {
	target := f.widths
	var assignments []pdfCIDWidthAssignment
	arrays := map[types.IndirectRef][]float64{}
	expanded, maxCode := 0, uint32(0)
	for i := 0; i < len(widths); {
		if err := r.ctx.Err(); err != nil {
			return err
		}
		start, err := r.pdf.DereferenceInteger(widths[i])
		if err != nil || start == nil || *start < 0 || *start > 65535 {
			return fmt.Errorf("invalid CID width start")
		}
		i++
		if i >= len(widths) {
			return fmt.Errorf("incomplete CID widths")
		}
		source := widths[i]
		value, err := r.pdf.Dereference(source)
		if err != nil {
			return err
		}
		i++
		a := pdfCIDWidthAssignment{first: uint32(*start)}
		if list, ok := value.(types.Array); ok {
			if int(*start)+len(list) > 65536 {
				return fmt.Errorf("CID widths outside range")
			}
			ref, indirect := source.(types.IndirectRef)
			values, cached := arrays[ref]
			if !indirect || !cached {
				values = make([]float64, len(list))
				for j, w := range list {
					if j%1024 == 0 {
						if err := r.ctx.Err(); err != nil {
							return err
						}
					}
					values[j], err = r.pdf.DereferenceNumber(w)
					if err != nil {
						return err
					}
					if !finite(values[j]) {
						return fmt.Errorf("nonfinite glyph width")
					}
				}
				if indirect {
					arrays[ref] = values
				}
			}
			if len(values) == 0 {
				continue
			}
			a.values = values
			a.last = a.first + uint32(len(values)) - 1
		} else {
			end, ok := value.(types.Integer)
			if !ok || int(end) < int(*start) || int(end) > 65535 || i >= len(widths) {
				return fmt.Errorf("invalid CID width range")
			}
			a.width, err = r.pdf.DereferenceNumber(widths[i])
			if err != nil {
				return err
			}
			i++
			if !finite(a.width) {
				return fmt.Errorf("nonfinite glyph width")
			}
			a.last = uint32(end)
		}
		assignments = append(assignments, a)
		maxCode = max(maxCode, a.last)
		// Saturation only selects the bounded-application strategy; it never rejects.
		expanded = min(65537, expanded+int(a.last-a.first)+1)
	}
	f.cidWidthOverlap = expanded > 65536
	write := func(a pdfCIDWidthAssignment, code uint32) {
		if _, exists := target[code]; exists {
			f.cidWidthOverlap = true
		}
		width := a.width
		if a.values != nil {
			width = a.values[code-a.first]
		}
		target[code] = width
	}
	if expanded <= 65536 {
		for _, a := range assignments {
			if err := r.ctx.Err(); err != nil {
				return err
			}
			for code := a.first; code <= a.last; code++ {
				if code%1024 == 0 {
					if err := r.ctx.Err(); err != nil {
						return err
					}
				}
				write(a, code)
			}
		}
		return nil
	}
	// Successor links skip codes already assigned by later source entries. At most
	// 65536 codes are materialized, plus linear parsing of the actual source.
	next := make([]uint32, int(maxCode)+2)
	for i := range next {
		next[i] = uint32(i)
	}
	find := func(code uint32) uint32 {
		root := code
		for next[root] != root {
			root = next[root]
		}
		for next[code] != code {
			parent := next[code]
			next[code] = root
			code = parent
		}
		return root
	}
	for i := len(assignments) - 1; i >= 0; i-- {
		if err := r.ctx.Err(); err != nil {
			return err
		}
		a := assignments[i]
		for code := find(a.first); code <= a.last; code = find(code) {
			if code%1024 == 0 {
				if err := r.ctx.Err(); err != nil {
					return err
				}
			}
			write(a, code)
			next[code] = find(code + 1)
		}
	}
	return nil
}
