package manydoc

import "fmt"

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
