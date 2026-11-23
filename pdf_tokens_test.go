package manydoc

import (
	"context"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
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

func TestContentNumbersDoNotConsumeStrokeColorOperator(t *testing.T) {
	input := "0 1 RG +2 -.5 3. Tr"
	want := []any{types.Integer(0), types.Integer(1), pdfOperator("RG"), types.Integer(2), types.Float(-.5), types.Float(3), pdfOperator("Tr")}
	for _, expected := range want {
		value, err := nextPDFToken(context.Background(), &input)
		if err != nil || value != expected {
			t.Fatalf("token %v, %v; want %v", value, err, expected)
		}
	}
	for _, malformed := range []string{".", "+", "1e3", "1.2.3", "0RG"} {
		if _, err := nextPDFToken(context.Background(), &malformed); err == nil {
			t.Fatalf("invalid numeric operand accepted: %s", malformed)
		}
	}
}
