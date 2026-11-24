package manydoc

import (
	"bytes"
	"compress/zlib"
	"context"
	"encoding/ascii85"
	"errors"
	"fmt"
	"io"

	"github.com/hhrutter/lzw"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// Internal decoded-work/allocation bounds, independent of application quotas.
const maxPDFStreamBytes = 64 << 20
const maxPDFDecodedBytes = 256 << 20
const maxPDFStreamFilters = 16

var errPDFDecodedLimit = errors.New("PDF decoded stream limit exceeded")

type pdfStreamBudget struct {
	total int
	cache map[types.IndirectRef][]byte
}

func readPDFDecoded(ctx context.Context, src io.Reader, limit int) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(contextReader{ctx, src}, int64(limit)+1))
	if err != nil {
		return nil, err
	}
	if len(data) > limit {
		return nil, fmt.Errorf("%w: %d bytes", errPDFDecodedLimit, limit)
	}
	return data, nil
}

func (r *pdfReader) decodePDFStream(object types.Object, sd *types.StreamDict) ([]byte, error) {
	if err := r.ctx.Err(); err != nil {
		return nil, err
	}
	if r.streamBudget == nil {
		r.streamBudget = &pdfStreamBudget{cache: map[types.IndirectRef][]byte{}}
	}
	ref, indirect := object.(types.IndirectRef)
	if data, ok := r.streamBudget.cache[ref]; indirect && ok {
		return data, nil
	}
	charge := func(data []byte) error {
		if len(data) > maxPDFStreamBytes {
			return fmt.Errorf("PDF decoded stream exceeds %d bytes", maxPDFStreamBytes)
		}
		if len(data) > maxPDFDecodedBytes-r.streamBudget.total {
			return fmt.Errorf("PDF decoded streams exceed %d bytes", maxPDFDecodedBytes)
		}
		r.streamBudget.total += len(data)
		return nil
	}
	if len(sd.FilterPipeline) > maxPDFStreamFilters {
		return nil, fmt.Errorf("PDF stream exceeds %d filters", maxPDFStreamFilters)
	}
	data := sd.Content
	if data == nil {
		data = sd.Raw
		if len(sd.FilterPipeline) == 0 {
			if err := charge(data); err != nil {
				return nil, err
			}
		}
		for _, f := range sd.FilterPipeline {
			limit := min(maxPDFStreamBytes, maxPDFDecodedBytes-r.streamBudget.total)
			if err := r.ctx.Err(); err != nil {
				return nil, err
			}
			var err error
			switch f.Name {
			case "FlateDecode":
				var reader io.ReadCloser
				reader, err = zlib.NewReader(contextReader{r.ctx, bytes.NewReader(data)})
				if err == nil {
					data, err = readPDFDecoded(r.ctx, reader, limit)
					closeErr := reader.Close()
					if err == nil {
						err = closeErr
					}
				}
			case "LZWDecode":
				early, e := r.pdfStreamParam(f.DecodeParms, "EarlyChange", 1)
				if e != nil {
					return nil, e
				}
				if early != 0 && early != 1 {
					return nil, fmt.Errorf("invalid PDF LZW EarlyChange")
				}
				reader := lzw.NewReader(contextReader{r.ctx, bytes.NewReader(data)}, early == 1)
				data, err = readPDFDecoded(r.ctx, reader, limit)
				closeErr := reader.Close()
				if err == nil {
					err = closeErr
				}
			case "ASCII85Decode":
				end := bytes.Index(data, []byte("~>"))
				if end < 0 {
					return nil, fmt.Errorf("PDF ASCII85 stream lacks EOD")
				}
				data, err = readPDFDecoded(r.ctx, ascii85.NewDecoder(bytes.NewReader(data[:end])), limit)
			case "ASCIIHexDecode":
				data, err = decodePDFHex(r.ctx, data, limit)
			case "RunLengthDecode":
				data, err = decodePDFRunLength(r.ctx, data, limit)
			default:
				return nil, fmt.Errorf("%w: PDF syntax stream filter %q", ErrUnsupported, f.Name)
			}
			if err != nil {
				if errors.Is(err, errPDFDecodedLimit) && limit < maxPDFStreamBytes {
					return nil, fmt.Errorf("PDF decoded streams exceed %d bytes: %w", maxPDFDecodedBytes, err)
				}
				return nil, fmt.Errorf("PDF stream %s: %w", f.Name, err)
			}
			if err = charge(data); err != nil {
				return nil, err
			}
			if f.Name == "FlateDecode" || f.Name == "LZWDecode" {
				predictor, e := r.pdfStreamParam(f.DecodeParms, "Predictor", 1)
				if e != nil {
					return nil, e
				}
				if predictor != 1 {
					colors, e := r.pdfStreamParam(f.DecodeParms, "Colors", 1)
					if e != nil {
						return nil, e
					}
					columns, e := r.pdfStreamParam(f.DecodeParms, "Columns", 1)
					if e != nil {
						return nil, e
					}
					bits, e := r.pdfStreamParam(f.DecodeParms, "BitsPerComponent", 8)
					if e != nil {
						return nil, e
					}
					data, err = unpredictPDF(r.ctx, data, predictor, colors, columns, bits)
					if err != nil {
						return nil, err
					}
				}
			}
		}
	} else if err := charge(data); err != nil {
		return nil, err
	}
	if indirect {
		r.streamBudget.cache[ref] = data
	}
	return data, nil
}

func (r *pdfReader) pdfStreamParam(d types.Dict, name string, fallback int) (int, error) {
	value := d[name]
	if value == nil {
		return fallback, nil
	}
	if n, ok := value.(types.Integer); ok {
		return int(n), nil
	}
	if r.pdf != nil {
		n, err := r.pdf.DereferenceInteger(value)
		if err == nil && n != nil {
			return int(*n), nil
		}
	}
	return 0, fmt.Errorf("invalid PDF stream parameter %s", name)
}

func decodePDFHex(ctx context.Context, data []byte, limit int) ([]byte, error) {
	var out []byte
	pending := -1
	for i, c := range data {
		if i%8192 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		if c == '>' {
			break
		}
		if pdfSpace(c) {
			continue
		}
		value := -1
		switch {
		case c >= '0' && c <= '9':
			value = int(c - '0')
		case c >= 'a' && c <= 'f':
			value = int(c-'a') + 10
		case c >= 'A' && c <= 'F':
			value = int(c-'A') + 10
		}
		if value < 0 {
			return nil, fmt.Errorf("invalid PDF ASCIIHex digit")
		}
		if pending < 0 {
			pending = value
			continue
		}
		if len(out) >= limit {
			return nil, fmt.Errorf("%w: %d bytes", errPDFDecodedLimit, limit)
		}
		out = append(out, byte(pending<<4|value))
		pending = -1
	}
	if pending >= 0 {
		if len(out) >= limit {
			return nil, fmt.Errorf("%w: %d bytes", errPDFDecodedLimit, limit)
		}
		out = append(out, byte(pending<<4))
	}
	return out, nil
}

func decodePDFRunLength(ctx context.Context, data []byte, limit int) ([]byte, error) {
	var out []byte
	for i := 0; i < len(data); {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		n := int(data[i])
		i++
		if n == 128 {
			return out, nil
		}
		size := n + 1
		if n > 128 {
			size = 257 - n
		}
		if size > limit-len(out) {
			return nil, fmt.Errorf("%w: %d bytes", errPDFDecodedLimit, limit)
		}
		if n < 128 {
			if size > len(data)-i {
				return nil, fmt.Errorf("truncated PDF RunLength literal")
			}
			out = append(out, data[i:i+size]...)
			i += size
		} else {
			if i >= len(data) {
				return nil, fmt.Errorf("truncated PDF RunLength repeat")
			}
			for j := 0; j < size; j++ {
				out = append(out, data[i])
			}
			i++
		}
	}
	return nil, fmt.Errorf("PDF RunLength stream lacks EOD")
}

func (r *pdfReader) pageContent(d types.Dict) ([]byte, error) {
	object := d["Contents"]
	if object == nil {
		return nil, nil
	}
	resolved, err := r.pdf.Dereference(object)
	if err != nil {
		return nil, err
	}
	if resolved == nil {
		return nil, nil
	}
	objects := types.Array{object}
	if a, ok := resolved.(types.Array); ok {
		objects = a
	}
	var result []byte
	for _, o := range objects {
		if err := r.ctx.Err(); err != nil {
			return nil, err
		}
		if o == nil {
			continue
		}
		sd, _, err := r.pdf.DereferenceStreamDict(o)
		if err != nil {
			return nil, err
		}
		if sd == nil {
			continue
		}
		data, err := r.decodePDFStream(o, sd)
		if err != nil {
			return nil, err
		}
		if len(data) > maxPDFStreamBytes-len(result) {
			return nil, fmt.Errorf("PDF page content exceeds %d bytes", maxPDFStreamBytes)
		}
		// PDF content arrays concatenate bytes; do not invent token separators.
		result = append(result, data...)
	}
	return result, nil
}
