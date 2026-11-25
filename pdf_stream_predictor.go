package manifold

import (
	"context"
	"fmt"
)

func unpredictPDF(ctx context.Context, data []byte, predictor, colors, columns, bits int) ([]byte, error) {
	return unpredictPDFLimit(ctx, data, predictor, colors, columns, bits, maxPDFStreamBytes)
}

func unpredictPDFLimit(ctx context.Context, data []byte, predictor, colors, columns, bits, limit int) ([]byte, error) {
	if predictor != 2 && (predictor < 10 || predictor > 15) {
		return nil, fmt.Errorf("%w: PDF stream predictor %d", ErrUnsupported, predictor)
	}
	if colors < 1 || columns < 1 || (bits != 1 && bits != 2 && bits != 4 && bits != 8 && bits != 16) {
		return nil, fmt.Errorf("invalid PDF predictor dimensions/sample depth")
	}
	if int64(colors) > int64(limit)*8/int64(bits)/int64(columns) {
		return nil, fmt.Errorf("PDF predictor row exceeds decoded stream limit")
	}
	samples := colors * columns
	rowBytes := (samples*bits + 7) / 8
	pixelBytes := (colors*bits + 7) / 8
	if predictor == 2 {
		if len(data)%rowBytes != 0 {
			return nil, fmt.Errorf("truncated PDF TIFF predictor row")
		}
		mask := uint32(1<<bits) - 1
		sample := func(row []byte, index int) uint32 {
			var v uint32
			for j := 0; j < bits; j++ {
				bit := index*bits + j
				v = v<<1 | uint32(row[bit/8]>>uint(7-bit%8)&1)
			}
			return v
		}
		for offset := 0; offset < len(data); offset += rowBytes {
			row := data[offset : offset+rowBytes]
			for i := colors; i < samples; i++ {
				if i%4096 == 0 {
					if err := ctx.Err(); err != nil {
						return nil, err
					}
				}
				value := (sample(row, i) + sample(row, i-colors)) & mask
				for j := 0; j < bits; j++ {
					bit := i*bits + j
					shift := uint(7 - bit%8)
					row[bit/8] = row[bit/8]&^(1<<shift) | byte(value>>uint(bits-j-1)&1)<<shift
				}
			}
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		return data, nil
	}
	stride := rowBytes + 1
	if len(data)%stride != 0 {
		return nil, fmt.Errorf("truncated PDF PNG predictor row")
	}
	out := make([]byte, len(data)/stride*rowBytes)
	for src, dst := 0, 0; src < len(data); src, dst = src+stride, dst+rowBytes {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		filter := data[src]
		if filter > 4 {
			return nil, fmt.Errorf("invalid PDF PNG predictor filter")
		}
		for i := 0; i < rowBytes; i++ {
			if i%8192 == 0 {
				if err := ctx.Err(); err != nil {
					return nil, err
				}
			}
			var left, up, upperLeft byte
			if i >= pixelBytes {
				left = out[dst+i-pixelBytes]
			}
			if dst > 0 {
				up = out[dst+i-rowBytes]
				if i >= pixelBytes {
					upperLeft = out[dst+i-rowBytes-pixelBytes]
				}
			}
			v := data[src+1+i]
			switch filter {
			case 1:
				v += left
			case 2:
				v += up
			case 3:
				v += byte((int(left) + int(up)) / 2)
			case 4:
				v += pdfPaeth(left, up, upperLeft)
			}
			out[dst+i] = v
		}
	}
	return out, nil
}
func pdfPaeth(a, b, c byte) byte {
	p := int(a) + int(b) - int(c)
	abs := func(v int) int {
		if v < 0 {
			return -v
		}
		return v
	}
	pa, pb, pc := abs(p-int(a)), abs(p-int(b)), abs(p-int(c))
	if pa <= pb && pa <= pc {
		return a
	}
	if pb <= pc {
		return b
	}
	return c
}
