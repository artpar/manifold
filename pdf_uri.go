package manifold

import "strings"

// URI actions use 7-bit ASCII (ISO 32000-1, Table 206). Map UTF-8 IRI octets
// using RFC 3987 section 3.1 without normalization or re-encoding existing
// escapes/reserved delimiters. Also escape excluded ASCII characters. Resolution
// and scheme-specific host processing belong to the consuming application.
func pdfASCIIURI(iri string) string {
	unsafe := func(c byte) bool {
		return c <= 0x20 || c >= 0x7f || strings.IndexByte("<>\"{}|\\^`", c) >= 0
	}
	first := 0
	for first < len(iri) && !unsafe(iri[first]) {
		first++
	}
	if first == len(iri) {
		return iri
	}
	const hex = "0123456789ABCDEF"
	var out strings.Builder
	out.Grow(len(iri))
	out.WriteString(iri[:first])
	for i := first; i < len(iri); i++ {
		c := iri[i]
		if unsafe(c) {
			out.WriteByte('%')
			out.WriteByte(hex[c>>4])
			out.WriteByte(hex[c&15])
		} else {
			out.WriteByte(c)
		}
	}
	return out.String()
}
