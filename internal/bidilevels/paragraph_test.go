package bidilevels

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"

	"golang.org/x/text/unicode/bidi"
)

func TestNativeFriBidiWrappedLines(t *testing.T) {
	data, err := os.ReadFile("testdata/lines.json")
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(data)
	if hex.EncodeToString(hash[:]) != "275b41110085e739bd51e752b5c1389ea4e36abbbc46d7a987d7162a5ef074da" {
		t.Fatal("native line observations changed")
	}
	var fixture struct {
		Cases []struct {
			Name, Text   string
			Base         int
			ResolvedBase uint8 `json:"resolved_base"`
			Lines        []struct {
				Start, End    int
				Levels, Order []int
				ICU           struct{ Levels, Order []int } `json:"icu"`
			}
		}
	}
	if err = json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	total := 0
	for _, tc := range fixture.Cases {
		t.Run(tc.Name, func(t *testing.T) {
			p, err := Resolve(context.Background(), tc.Text, tc.Base)
			if err != nil {
				t.Fatal(err)
			}
			if p.Base() != tc.ResolvedBase {
				t.Fatalf("base %d want %d", p.Base(), tc.ResolvedBase)
			}
			for _, want := range tc.Lines {
				line, err := p.Line(want.Start, want.End)
				if err != nil {
					t.Fatal(err)
				}
				local := Line{Levels: line.Levels, Order: slices.Clone(line.Order)}
				expected := slices.Clone(want.Order)
				for i := range local.Order {
					local.Order[i] -= want.Start
				}
				for i := range expected {
					expected[i] -= want.Start
				}
				if tc.Name == "nested-isolates" && want.Start == 6 && want.End == 10 {
					// Preserve the observed native FriBidi values. Its trailing
					// PDI keeps level 1; UAX#9 L1 resets it to paragraph level 0.
					// ICU independently agrees with this reset and visual order.
					if !slices.Equal(want.Levels, []int{1, 2, 2, 1}) || !slices.Equal(want.Order, []int{9, 7, 8, 6}) {
						t.Fatal("FriBidi observation changed")
					}
					checkLine(t, tc.Name, local, []int{1, 2, 2, 0}, []int{1, 2, 0, 3})
				} else {
					checkLine(t, tc.Name, local, want.Levels, expected)
				}
				// ICU may lower even levels of a unidirectional line. UAX#9
				// conformance requires direction parity and visual order, not
				// identical numeric levels in this optimization.
				if len(want.ICU.Levels) != len(line.Levels) {
					t.Fatal("ICU line cardinality")
				}
				for i, l := range line.Levels {
					if want.ICU.Levels[i] >= 0 && int(l)%2 != want.ICU.Levels[i]%2 {
						t.Fatalf("ICU parity %v want %v", line.Levels, want.ICU.Levels)
					}
				}
				var visible []int
				for _, i := range line.Order {
					if want.ICU.Levels[i-want.Start] >= 0 {
						visible = append(visible, i)
					}
				}
				if !slices.Equal(visible, want.ICU.Order) {
					t.Fatalf("ICU visual order %v want %v", visible, want.ICU.Order)
				}
				total++
			}
		})
	}
	if len(fixture.Cases) != 12 || total != 34 {
		t.Fatalf("fixture coverage %d cases/%d lines", len(fixture.Cases), total)
	}
}
func TestParagraphValidationAndOwnership(t *testing.T) {
	for _, tc := range []struct {
		text string
		base int
	}{{"abc", -2}, {"abc", 2}, {"\xff", 0}, {"a\nb", 0}, {strings.Repeat("a", 65537), 0}} {
		if _, err := Resolve(context.Background(), tc.text, tc.base); err == nil {
			t.Fatalf("accepted %q base %d", tc.text[:min(8, len(tc.text))], tc.base)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Resolve(ctx, "אב", -1); err != context.Canceled {
		t.Fatal(err)
	}
	p, err := Resolve(context.Background(), "", -1)
	if err != nil || p.Base() != 0 || p.Len() != 0 {
		t.Fatalf("empty %+v %v", p, err)
	}
	if l, err := p.Line(0, 0); err != nil || len(l.Levels) != 0 || len(l.Order) != 0 {
		t.Fatal(l, err)
	}
	p, err = Resolve(context.Background(), "אב 123 test", 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, range_ := range [][2]int{{-1, 1}, {2, 1}, {0, p.Len() + 1}} {
		if _, err := p.Line(range_[0], range_[1]); err == nil {
			t.Fatal("accepted invalid range")
		}
	}
	original := p.Levels()
	levels := p.Levels()
	levels[0] = 100
	l, err := p.Line(0, p.Len())
	if err != nil {
		t.Fatal(err)
	}
	l.Levels[0] = 100
	l.Order[0] = 100
	if !slices.Equal(original, p.Levels()) {
		t.Fatal("caller mutated paragraph")
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 20 {
				_, err := p.Line(1, 5)
				if err != nil {
					t.Error(err)
				}
			}
		}()
	}
	wg.Wait()
}

func TestLineRetainsParagraphContext(t *testing.T) {
	p, err := Resolve(context.Background(), "א - xyz", -1)
	if err != nil {
		t.Fatal(err)
	}
	line, err := p.Line(2, 4)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(line.Levels, []uint8{1, 1}) || !slices.Equal(line.Order, []int{3, 2}) {
		t.Fatalf("lost paragraph context: %+v", line)
	}
	sliced, err := Resolve(context.Background(), "- ", -1)
	if err != nil {
		t.Fatal(err)
	}
	if sliced.Base() == p.Base() {
		t.Fatal("fixture must distinguish paragraph context from re-resolved line")
	}
}

func TestLongUnmatchedIsolates(t *testing.T) {
	p, err := Resolve(context.Background(), strings.Repeat("\u2068", 65536), 0)
	if err != nil || p.Len() != 65536 {
		t.Fatalf("long isolates %v", err)
	}
}
func TestPublishedBracketData(t *testing.T) {
	if bidi.UnicodeVersion != "15.0.0" {
		t.Fatal("requalify bracket/class/test versions together")
	}
	h := sha256.Sum256([]byte(bracketData))
	if hex.EncodeToString(h[:]) != "333ae1e99db0504ca8a046a07dc45b5e7aa91869c685e6bf955ebe674804827a" {
		t.Fatal("bracket data changed")
	}
	if len(pairedBracketValues) != 128 {
		t.Fatalf("bracket entries %d", len(pairedBracketValues))
	}
	for _, r := range []rune{0x2329, 0x232a, 0x3008, 0x3009} {
		if pairedBracketValues[r] != 0x3008 {
			t.Fatalf("canonical bracket %U", r)
		}
	}
}
func FuzzParagraphLines(f *testing.F) {
	f.Add("אב abc 123", byte(1), uint16(2), uint16(7))
	f.Add("A\u2067אבג\u2069 B", byte(2), uint16(0), uint16(5))
	f.Add("א 〈abc〉 ב", byte(0), uint16(1), uint16(4))
	f.Fuzz(func(t *testing.T, text string, b byte, a, z uint16) {
		if len(text) > 4096 {
			t.Skip()
		}
		p, err := Resolve(context.Background(), text, int(b%3)-1)
		if err != nil {
			return
		}
		start, end := int(a)%(p.Len()+1), int(z)%(p.Len()+1)
		if start > end {
			start, end = end, start
		}
		original := p.Levels()
		l, err := p.Line(start, end)
		if err != nil {
			t.Fatal(err)
		}
		if len(l.Levels) != end-start || len(l.Order) != end-start {
			t.Fatal("line cardinality")
		}
		seen := make(map[int]bool)
		for _, i := range l.Order {
			if i < start || i >= end || seen[i] {
				t.Fatal("invalid line permutation")
			}
			seen[i] = true
		}
		if !slices.Equal(original, p.Levels()) {
			t.Fatal("line mutated paragraph")
		}
		if end > start {
			breaks := []int{}
			if start > 0 {
				breaks = append(breaks, start)
			}
			breaks = append(breaks, end)
			if end < p.Len() {
				breaks = append(breaks, p.Len())
			}
			reference := p.p.getLevels(breaks)
			for i, v := range l.Levels {
				if v != uint8(reference[start+i]) {
					t.Fatal("line L1 mismatch")
				}
			}
		}
	})
}
