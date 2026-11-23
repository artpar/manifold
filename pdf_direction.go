package manydoc

import "golang.org/x/text/unicode/bidi"

// Bidi format/mark controls affect paragraph levels but have no visible advance.
// Keep their original logical text using the existing zero-width ActualText path.
func pdfBidiControl(ch rune) bool {
	return ch == 0x061c || ch == 0x200e || ch == 0x200f || ch >= 0x202a && ch <= 0x202e || ch >= 0x2066 && ch <= 0x2069
}

func pdfHasBidiText(text string) bool {
	for _, ch := range text {
		p, _ := bidi.LookupRune(ch)
		if p.Class() == bidi.R || p.Class() == bidi.AL || p.Class() == bidi.AN || pdfBidiControl(ch) {
			return true
		}
	}
	return false
}
