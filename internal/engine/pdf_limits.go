package engine

import "fmt"

func resolvePDFReadLimits(opts *PDFReadLimits) (PDFReadLimits, error) {
	out := PDFReadLimits{MaxDecodedBytes: maxPDFDecodedBytes, MaxRasterPixels: maxPDFRasterPixels, MaxNavigationEntries: 100000, MaxStructureEntries: 200000}
	if opts == nil {
		return out, nil
	}
	if opts.MaxStructureEntries < 0 || opts.MaxStructureEntries > 10_000_000 || opts.MaxDecodedBytes < 0 || opts.MaxRasterPixels < 0 || opts.MaxNavigationEntries < 0 || opts.MaxNavigationEntries > maxPDFNavigationEntries {
		return out, fmt.Errorf("invalid PDF read limits")
	}
	if opts.MaxDecodedBytes > 0 {
		out.MaxDecodedBytes = opts.MaxDecodedBytes
	}
	if opts.MaxRasterPixels > 0 {
		out.MaxRasterPixels = opts.MaxRasterPixels
	}
	if opts.MaxNavigationEntries > 0 {
		out.MaxNavigationEntries = opts.MaxNavigationEntries
	}
	if opts.MaxStructureEntries > 0 {
		out.MaxStructureEntries = opts.MaxStructureEntries
	}
	return out, nil
}
func (r *pdfReader) decodedLimit() int {
	if r.limits.MaxDecodedBytes > 0 {
		return r.limits.MaxDecodedBytes
	}
	return maxPDFDecodedBytes
}
func (r *pdfReader) rasterLimit() int64 {
	if r.limits.MaxRasterPixels > 0 {
		return r.limits.MaxRasterPixels
	}
	return maxPDFRasterPixels
}
func (r *pdfReader) navigationLimit() int {
	if r.limits.MaxNavigationEntries > 0 {
		return r.limits.MaxNavigationEntries
	}
	return 100000
}

func (r *pdfReader) structureLimit() int {
	if r.limits.MaxStructureEntries > 0 {
		return r.limits.MaxStructureEntries
	}
	return 200000
}
