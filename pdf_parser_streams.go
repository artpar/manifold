package manydoc

import (
	"bytes"
	"fmt"
	"sort"
	"strconv"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// validateParserStreams verifies the original decoded source, not the dependency's
// possibly lenient cached Content. It runs before page access. The dependency has
// already read xref streams and object-stream prologs: this is not a guard against
// allocations made inside ReadWithContext itself.
func (r *pdfReader) validateParserStreams() error {
	ids := make([]int, 0, len(r.pdf.Table))
	owners := map[int]map[int]int{}
	for id, entry := range r.pdf.Table {
		if err := r.ctx.Err(); err != nil {
			return err
		}
		if entry == nil || entry.Free {
			continue
		}
		ids = append(ids, id)
		if entry.ObjectStream != nil && entry.ObjectStreamInd != nil {
			stream, index := *entry.ObjectStream, *entry.ObjectStreamInd
			if owners[stream] == nil {
				owners[stream] = map[int]int{}
			}
			if previous, found := owners[stream][index]; found && previous != id {
				return fmt.Errorf("PDF object stream %d index %d has conflicting owners", stream, index)
			}
			owners[stream][index] = id
		}
	}
	sort.Ints(ids)
	for _, id := range ids {
		var sd types.StreamDict
		objectStream := false
		switch object := r.pdf.Table[id].Object.(type) {
		case types.ObjectStreamDict:
			sd = object.StreamDict
			objectStream = true
		case types.XRefStreamDict:
			sd = object.StreamDict
		case types.StreamDict:
			if object.IsObjStm() {
				sd = object
				objectStream = true
			} else if typ := object.NameEntry("Type"); typ != nil && *typ == "XRef" {
				sd = object
			} else {
				continue
			}
		default:
			continue
		}
		cached := sd.Content
		sd.Content = nil
		decoded, err := r.decodePDFStream(nil, &sd)
		if err != nil {
			return fmt.Errorf("PDF parser stream %d: %w", id, err)
		}
		if cached != nil && !bytes.Equal(cached, decoded) {
			return fmt.Errorf("PDF parser stream %d decoded source disagrees with parser content", id)
		}
		if objectStream {
			if err := r.validateObjectStreamHeader(id, &sd, decoded, owners[id]); err != nil {
				return err
			}
			delete(owners, id)
		}
	}
	if len(owners) > 0 {
		return fmt.Errorf("PDF compressed objects have no validated object stream")
	}
	return nil
}

func pdfHeaderSpace(c byte) bool {
	return c == 0 || c == 9 || c == 10 || c == 12 || c == 13 || c == ' '
}

// Object-stream prologs contain integer pairs, with PDF whitespace and comments.
// Scan in place instead of allocating a field for every whitespace separator.
func (r *pdfReader) skipHeaderSpace(header []byte, pos *int) error {
	for *pos < len(header) {
		if *pos%8192 == 0 {
			if err := r.ctx.Err(); err != nil {
				return err
			}
		}
		c := header[*pos]
		if pdfHeaderSpace(c) {
			*pos++
			continue
		}
		if c != '%' {
			break
		}
		for *pos < len(header) && header[*pos] != '\r' && header[*pos] != '\n' {
			if *pos%8192 == 0 {
				if err := r.ctx.Err(); err != nil {
					return err
				}
			}
			*pos++
		}
	}
	return nil
}
func (r *pdfReader) headerInteger(header []byte, pos *int) (int, error) {
	if err := r.skipHeaderSpace(header, pos); err != nil {
		return 0, err
	}
	start := *pos
	for *pos < len(header) && !pdfHeaderSpace(header[*pos]) && header[*pos] != '%' {
		if *pos%8192 == 0 {
			if err := r.ctx.Err(); err != nil {
				return 0, err
			}
		}
		*pos++
	}
	if start == *pos {
		return 0, fmt.Errorf("missing object-stream header integer")
	}
	n, err := strconv.Atoi(string(header[start:*pos]))
	if err != nil {
		return 0, fmt.Errorf("invalid object-stream header integer")
	}
	return n, nil
}
func (r *pdfReader) validateObjectStreamHeader(id int, sd *types.StreamDict, data []byte, owners map[int]int) error {
	n, err := r.pdfStreamParam(sd.Dict, "N", -1)
	if err != nil {
		return err
	}
	first, err := r.pdfStreamParam(sd.Dict, "First", -1)
	if err != nil {
		return err
	}
	if n < 0 || first < 0 || first > len(data) {
		return fmt.Errorf("PDF object stream %d has invalid N/First", id)
	}
	for index := range owners {
		if index < 0 || index >= n {
			return fmt.Errorf("PDF object stream %d compressed index is outside N", id)
		}
	}
	header := data[:first]
	pos, previous := 0, -1
	seen := map[int]bool{}
	for index := 0; index < n; index++ {
		if err := r.ctx.Err(); err != nil {
			return err
		}
		member, err := r.headerInteger(header, &pos)
		if err != nil {
			return fmt.Errorf("PDF object stream %d: %w", id, err)
		}
		offset, err := r.headerInteger(header, &pos)
		if err != nil {
			return fmt.Errorf("PDF object stream %d: %w", id, err)
		}
		if member <= 0 || seen[member] || offset < 0 || offset <= previous || offset >= len(data)-first {
			return fmt.Errorf("PDF object stream %d has invalid member identity/offset", id)
		}
		seen[member] = true
		previous = offset
		if owner, found := owners[index]; found && owner != member {
			return fmt.Errorf("PDF object stream %d index %d header owner %d contradicts xref owner %d", id, index, member, owner)
		}
	}
	if err := r.skipHeaderSpace(header, &pos); err != nil {
		return err
	}
	if pos != len(header) {
		return fmt.Errorf("PDF object stream %d header count contradicts N", id)
	}
	return nil
}
