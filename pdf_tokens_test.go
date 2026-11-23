package manydoc

import (
	"context"
	"testing"
)

func TestUnicodeMap(t *testing.T) {
	s := `1 begincodespacerange <00> <FF> endcodespacerange
2 beginbfchar <01> <D83DDE00> <02> <00660069> endbfchar
2 beginbfrange <10> <12> <0041> <20> <21> [<03A9> <0416>] endbfrange`
	m, e := parseUnicodeMap(context.Background(), []byte(s))
	if e != nil {
		t.Fatal(e)
	}
	got, e := m.decode([]byte{1, 2, 0x10, 0x11, 0x12, 0x20, 0x21})
	if e != nil {
		t.Fatal(e)
	}
	if got != "😀fiABCΩЖ" {
		t.Fatalf("got %q", got)
	}
	if _, e = m.decode([]byte{3}); e == nil {
		t.Fatal("unmapped code silently accepted")
	}
}
func TestMalformedUnicodeMap(t *testing.T) {
	for _, s := range []string{`1 beginbfchar <01> endbfchar`, `1 beginbfchar <01> <D800> endbfchar`, `1 beginbfrange <01> <02> [<0041>] endbfrange`} {
		if _, e := parseUnicodeMap(context.Background(), []byte(s)); e == nil {
			t.Fatalf("accepted %s", s)
		}
	}
}
