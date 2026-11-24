package manydoc

import (
	"bytes"
	"context"
	"fmt"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// pdfTrailerAt only inspects unencrypted file structure. A failed discovery leaves
// the original input to the ordinary parser, including its existing repair rules.
func pdfTrailerAt(ctx context.Context, b []byte, offset int) (types.Dict, int, error) {
	if offset < 0 || offset >= len(b) {
		return nil, 0, nil
	}
	end := offset + min(maxPDFStreamBytes, len(b)-offset)
	text := string(b[offset:end])
	next := func() (any, error) { return nextPDFToken(ctx, &text) }
	first, err := next()
	if err != nil {
		return nil, 0, err
	}
	high := 0
	finish := func(d types.Dict) (types.Dict, int, error) {
		if d == nil {
			return nil, 0, nil
		}
		if size, ok := d["Size"].(types.Integer); ok && size > 0 {
			high = max(high, int(size))
		}
		if index, ok := d["Index"].(types.Array); ok {
			if len(index)%2 != 0 {
				return nil, 0, nil
			}
			for i := 0; i < len(index); i += 2 {
				start, a := index[i].(types.Integer)
				count, c := index[i+1].(types.Integer)
				if !a || !c || start < 0 || count < 0 || int(start) > int(^uint(0)>>1)-int(count) {
					return nil, 0, nil
				}
				high = max(high, int(start)+int(count))
			}
		}
		return d, high, nil
	}
	if first == pdfOperator("xref") {
		for text != "" {
			token, err := next()
			if err != nil {
				return nil, 0, err
			}
			if token == pdfOperator("trailer") {
				object, err := model.ParseObjectContext(ctx, &text)
				if err != nil {
					return nil, 0, err
				}
				d, _ := object.(types.Dict)
				return finish(d)
			}
			start, ok := token.(types.Integer)
			if !ok || start < 0 {
				return nil, 0, nil
			}
			token, err = next()
			if err != nil {
				return nil, 0, err
			}
			count, ok := token.(types.Integer)
			if !ok || count < 0 || int(start) > int(^uint(0)>>1)-int(count) {
				return nil, 0, nil
			}
			high = max(high, int(start)+int(count))
			for i := 0; i < int(count); i++ {
				for field := 0; field < 3; field++ {
					token, err := next()
					if err != nil {
						return nil, 0, err
					}
					if token == nil {
						return nil, 0, nil
					}
				}
			}
		}
		return nil, 0, nil
	}
	if _, ok := first.(types.Integer); !ok {
		return nil, 0, nil
	}
	gen, err := next()
	if err != nil {
		return nil, 0, err
	}
	if _, ok := gen.(types.Integer); !ok {
		return nil, 0, nil
	}
	keyword, err := next()
	if err != nil {
		return nil, 0, err
	}
	if keyword != pdfOperator("obj") {
		return nil, 0, nil
	}
	object, err := model.ParseObjectContext(ctx, &text)
	if err != nil {
		return nil, 0, err
	}
	d, _ := object.(types.Dict)
	if d == nil || d["Type"] != types.Name("XRef") {
		return nil, 0, nil
	}
	return finish(d)
}

// adaptDirectPDFEncryption adds an in-memory incremental trailer which gives a
// direct encryption dictionary an indirect identity understood by pdfcpu. Original
// objects, ciphertext, relative byte offsets and file IDs remain intact. A
// non-PDF header prefix is excluded from this parser view. Password checking
// and decryption still run through the existing Go security handler.
func adaptDirectPDFEncryption(ctx context.Context, b []byte) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	marker := bytes.LastIndex(b, []byte("startxref"))
	if marker < 0 {
		return b, nil
	}
	position := marker + len("startxref")
	probe := &pdfReader{ctx: ctx}
	start, err := probe.headerInteger(b, &position)
	if err != nil || start < 0 || start >= len(b) {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return b, nil
	}
	header := bytes.Index(b, []byte("%PDF-"))
	if header < 0 || header > 1024 {
		return b, nil
	}
	visited := map[int]bool{}
	var latest types.Dict
	var encryption types.Dict
	offset := start
	work := 0
	fresh := 0
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if visited[offset] || offset < 0 || offset >= len(b)-header {
			return b, nil
		}
		visited[offset] = true
		work += min(maxPDFStreamBytes, len(b)-offset-header)
		if work > maxPDFDecodedBytes {
			return b, nil
		}
		d, high, e := pdfTrailerAt(ctx, b, offset+header)
		if e != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return b, nil
		}
		if d == nil {
			return b, nil
		}
		fresh = max(fresh, high)
		if latest == nil {
			latest = types.Dict{}
			for k, v := range d {
				latest[k] = v
			}
		}
		for _, key := range []string{"Root", "ID", "Info"} {
			if latest[key] == nil && d[key] != nil {
				latest[key] = d[key]
			}
		}
		if encryption == nil && d["Encrypt"] != nil {
			encryption, _ = d["Encrypt"].(types.Dict)
			if encryption == nil {
				return b, nil
			}
		}

		prev, ok := d["Prev"].(types.Integer)
		if !ok {
			break
		}
		offset = int(prev)
	}
	if encryption == nil {
		return b, nil
	}
	size, ok := latest["Size"].(types.Integer)
	if ok {
		size = types.Integer(max(int(size), fresh))
	}
	root, rootOK := latest["Root"].(types.IndirectRef)
	id, idOK := latest["ID"].(types.Array)
	if !ok || size <= 0 || int(size) == int(^uint(0)>>1) || !rootOK || root.ObjectNumber <= 0 || root.ObjectNumber >= size || !idOK || len(id) != 2 {
		return nil, fmt.Errorf("invalid direct PDF encryption trailer")
	}
	for _, value := range id {
		if _, err := pdfString(value); err != nil {
			return nil, fmt.Errorf("invalid direct PDF encryption file ID")
		}
	}
	for _, key := range []string{"Type", "W", "Index", "Length", "Filter", "DecodeParms", "DL", "XRefStm"} {
		delete(latest, key)
	}
	latest["Size"] = size + 1
	latest["Encrypt"] = *types.NewIndirectRef(int(size), 0)
	latest["Prev"] = types.Integer(start)
	var extra bytes.Buffer
	extra.WriteByte('\n')
	objectOffset := len(b) + extra.Len() - header
	if int64(objectOffset) > 9_999_999_999 {
		return nil, fmt.Errorf("PDF encryption adapter xref offset exceeds supported range")
	}
	fmt.Fprintf(&extra, "%d 0 obj\n%s\nendobj\n", size, encryption.PDFString())
	xrefOffset := len(b) + extra.Len() - header
	fmt.Fprintf(&extra, "xref\n%d 1\n%010d 00000 n \ntrailer\n%s\nstartxref\n%d\n%%%%EOF\n", size, objectOffset, latest.PDFString(), xrefOffset)
	out := make([]byte, 0, len(b)-header+extra.Len())
	out = append(out, b[header:]...)
	out = append(out, extra.Bytes()...)
	return out, nil
}
