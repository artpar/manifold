package manydoc

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
)

// Match the reader's allocation bound before a decoder expands untrusted data.
const maxOutputImagePixels = 25_000_000

type outputImageInfo struct {
	config image.Config
	kind   string
}
type outputImageCache map[[32]byte]outputImageInfo

func decodeOutputImage(ctx context.Context, data []byte, cache outputImageCache) (image.Config, string, error) {
	if err := ctx.Err(); err != nil {
		return image.Config{}, "", err
	}
	digest := sha256.Sum256(data)
	if known, ok := cache[digest]; ok {
		return known.config, known.kind, nil
	}
	cfg, kind, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return cfg, kind, fmt.Errorf("image configuration: %w", err)
	}
	if kind != "png" && kind != "jpeg" {
		return cfg, kind, fmt.Errorf("%w: output image %s", ErrUnsupported, kind)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width) > maxOutputImagePixels/int64(cfg.Height) {
		return cfg, kind, fmt.Errorf("%w: output image exceeds raster allocation bound", ErrUnsupported)
	}
	decoded, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return cfg, kind, fmt.Errorf("image pixels: %w", err)
	}
	if decoded.Bounds().Dx() != cfg.Width || decoded.Bounds().Dy() != cfg.Height {
		return cfg, kind, fmt.Errorf("image configuration contradicts pixel dimensions")
	}
	if err := ctx.Err(); err != nil {
		return cfg, kind, err
	}
	if cache != nil {
		cache[digest] = outputImageInfo{cfg, kind}
	}
	return cfg, kind, nil
}

// DOCX is not laid out here. Reserve a default paragraph line per enclosing
// cell, plus a conservative reference/separator/marker area in note stories.
const docxImageCellHeightAllowance = 14.0
const docxImageNoteHeightAllowance = 56.0

// Dimensions are preferred physical sizes. A single dimension preserves the
// intrinsic aspect ratio; both dimensions explicitly specify that ratio. Fit
// to the container before inferring a missing dimension to avoid overflow.
func fitImageSize(im *Image, pixelWidth, pixelHeight int, available float64) (float64, float64, error) {
	if im == nil || pixelWidth <= 0 || pixelHeight <= 0 || !finite(available) || available <= 0 {
		return 0, 0, fmt.Errorf("%w: no space for image", ErrUnsupported)
	}
	w, h := im.Width, im.Height
	ratio := float64(pixelHeight) / float64(pixelWidth)
	if w == 0 && h == 0 {
		w = float64(pixelWidth) * .75
	}
	if w == 0 {
		if h > available*ratio {
			w, h = available, available*ratio
		} else {
			w = h / ratio
		}
	} else if h == 0 {
		w = min(w, available)
		h = w * ratio
	} else if w > available {
		h *= available / w
		w = available
	}
	if !finite(w) || !finite(h) || w <= 0 || h <= 0 {
		return 0, 0, fmt.Errorf("%w: image dimensions cannot be represented", ErrUnsupported)
	}
	return w, h, nil
}

// Fit preferred sizes to a full container without enlarging or changing the
// preferred rectangle's aspect. Both explicit dimensions may intentionally differ
// from the intrinsic aspect. Writers check coordinate serialization separately.
func fitImageBox(im *Image, pixelWidth, pixelHeight int, width, height float64) (float64, float64, error) {
	if !finite(height) || height <= 0 {
		return 0, 0, fmt.Errorf("%w: no height for image", ErrUnsupported)
	}
	w, h, err := fitImageSize(im, pixelWidth, pixelHeight, width)
	if err != nil {
		return 0, 0, err
	}
	if h > height {
		w *= height / h
		h = height
	}
	if !finite(w) || !finite(h) || w <= 0 || h <= 0 {
		return 0, 0, fmt.Errorf("%w: fitted image dimensions cannot be represented", ErrUnsupported)
	}
	return w, h, nil
}
