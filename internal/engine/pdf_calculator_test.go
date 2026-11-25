package engine

import (
	"math"
	"testing"
)

func TestPDFCalculatorOriginalBlackTint(t *testing.T) {
	p := []byte(`{1.000000 2 1 roll 1.000000 2 1 roll 1.000000 2 1 roll 0 index 1.000000 cvr exch sub 2 1 roll 5 -1 roll 1.000000 cvr exch sub 5 1 roll 4 -1 roll 1.000000 cvr exch sub 4 1 roll 3 -1 roll 1.000000 cvr exch sub 3 1 roll 2 -1 roll 1.000000 cvr exch sub 2 1 roll pop }`)
	for _, x := range []float64{0, .25, .5, .75, 1} {
		got, e := pdfCalculatorTint(p, x, 4)
		if e != nil {
			t.Fatal(e)
		}
		for i, want := range []float64{0, 0, 0, x} {
			if math.Abs(got[i]-want) > 1e-12 {
				t.Fatalf("tint %v -> %v", x, got)
			}
		}
	}
	for _, p := range []string{`{pop pop}`, `{1 0 div}`, `{200 index}`, `{1 200 roll}`, `{exec}`} {
		if _, e := pdfCalculatorTint([]byte(p), .5, 4); e == nil {
			t.Fatalf("unbounded or unknown function accepted: %s", p)
		}
	}
}
