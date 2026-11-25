package engine

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

type pdfContextReadSeeker struct {
	ctx context.Context
	io.ReadSeeker
}

func (s pdfContextReadSeeker) Read(b []byte) (int, error) {
	if e := s.ctx.Err(); e != nil {
		return 0, e
	}
	return s.ReadSeeker.Read(b)
}
func (s pdfContextReadSeeker) Seek(n int64, w int) (int64, error) {
	if e := s.ctx.Err(); e != nil {
		return 0, e
	}
	return s.ReadSeeker.Seek(n, w)
}

// Use an existing seekable source directly. The parser already has random access;
// another io.ReadAll copy (including its growth allocations) adds no information.
// Preserve the direct-encryption compatibility adapter on the exceptional path.
func readPDFSource(ctx context.Context, src io.Reader, password string) (*model.Context, error) {
	conf := func() *model.Configuration {
		return &model.Configuration{Reader15: true, ValidationMode: model.ValidationRelaxed, UserPW: password, OwnerPW: password, Offline: true}
	}
	if rs, ok := src.(io.ReadSeeker); ok {
		if pos, e := rs.Seek(0, io.SeekCurrent); e == nil && pos == 0 {
			p, firstError := pdfcpu.ReadWithContext(ctx, pdfContextReadSeeker{ctx, rs}, conf())
			if firstError == nil {
				return p, nil
			}
			if e := ctx.Err(); e != nil {
				return nil, e
			}
			if _, e = rs.Seek(0, io.SeekStart); e != nil {
				return nil, e
			}
			original, e := readContext(ctx, rs)
			if e != nil {
				return nil, e
			}
			adapted, e := adaptDirectPDFEncryption(ctx, original)
			if e != nil {
				return nil, e
			}
			if bytes.Equal(original, adapted) {
				return nil, fmt.Errorf("PDF read: %w", firstError)
			}
			p, e = pdfcpu.ReadWithContext(ctx, bytes.NewReader(adapted), conf())
			if e != nil {
				return nil, fmt.Errorf("PDF read: %w", e)
			}
			return p, nil
		}
	}
	b, e := readContext(ctx, src)
	if e != nil {
		return nil, e
	}
	b, e = adaptDirectPDFEncryption(ctx, b)
	if e != nil {
		return nil, e
	}
	p, e := pdfcpu.ReadWithContext(ctx, bytes.NewReader(b), conf())
	if e != nil {
		return nil, fmt.Errorf("PDF read: %w", e)
	}
	return p, nil
}
