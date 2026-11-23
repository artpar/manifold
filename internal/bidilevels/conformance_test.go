package bidilevels

import (
	"bufio"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
)

func corpusLines(t *testing.T, name string, fn func(int, string)) {
	t.Helper()
	f, err := os.Open("testdata/" + name + ".gz")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	z, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	var manifest map[string]struct {
		SHA256 string `json:"sha256"`
		Bytes  int    `json:"bytes"`
	}
	data, err := os.ReadFile("testdata/manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	original, err := io.ReadAll(z)
	if err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256(original)
	if hex.EncodeToString(h[:]) != manifest[name].SHA256 || len(original) != manifest[name].Bytes {
		t.Fatal("Unicode fixture hash/size mismatch")
	}
	scan := bufio.NewScanner(strings.NewReader(string(original)))
	scan.Buffer(make([]byte, 4096), 1<<20)
	for n := 1; scan.Scan(); n++ {
		line := strings.TrimSpace(strings.SplitN(scan.Text(), "#", 2)[0])
		if line != "" {
			fn(n, line)
		}
	}
	if err := scan.Err(); err != nil {
		t.Fatal(err)
	}
}
func integers(t *testing.T, s string, base int) []int {
	t.Helper()
	var out []int
	for _, field := range strings.Fields(s) {
		if field == "x" {
			out = append(out, -1)
			continue
		}
		n, err := strconv.ParseInt(field, base, 32)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, int(n))
	}
	return out
}
func checkLine(t *testing.T, where string, line Line, wantLevels, wantOrder []int) {
	t.Helper()
	if len(line.Levels) != len(wantLevels) {
		t.Fatalf("%s level count %d want %d", where, len(line.Levels), len(wantLevels))
	}
	for i, want := range wantLevels {
		if want >= 0 && int(line.Levels[i]) != want {
			t.Fatalf("%s levels %+v want %+v", where, line.Levels, wantLevels)
		}
	}
	var order []int
	for _, i := range line.Order {
		if wantLevels[i] >= 0 {
			order = append(order, i)
		}
	}
	if !slices.Equal(order, wantOrder) {
		t.Fatalf("%s order %+v want %+v", where, order, wantOrder)
	}
}
func TestUnicode15BidiCharacterConformance(t *testing.T) {
	count := 0
	corpusLines(t, "BidiCharacterTest.txt", func(n int, line string) {
		fields := strings.Split(line, ";")
		if len(fields) != 5 {
			t.Fatalf("line %d malformed", n)
		}
		var text []rune
		for _, v := range integers(t, fields[0], 16) {
			text = append(text, rune(v))
		}
		base := integers(t, fields[1], 10)[0]
		if base == 2 {
			base = -1
		}
		p, err := Resolve(context.Background(), string(text), base)
		if err != nil {
			t.Fatalf("line %d: %v", n, err)
		}
		wantBase := integers(t, fields[2], 10)[0]
		if int(p.Base()) != wantBase {
			t.Fatalf("line %d base %d want %d", n, p.Base(), wantBase)
		}
		result, err := p.Line(0, p.Len())
		if err != nil {
			t.Fatal(err)
		}
		checkLine(t, fmt.Sprintf("character line %d", n), result, integers(t, fields[3], 10), integers(t, fields[4], 10))
		count++
	})
	if count != 91707 {
		t.Fatalf("character cases %d", count)
	}
	t.Logf("%d original Unicode character cases", count)
}
func TestUnicode15BidiClassConformance(t *testing.T) {
	names := map[string]Class{"L": L, "R": R, "EN": EN, "ES": ES, "ET": ET, "AN": AN, "CS": CS, "B": B, "S": S, "WS": WS, "ON": ON, "BN": BN, "NSM": NSM, "AL": AL, "LRO": LRO, "RLO": RLO, "LRE": LRE, "RLE": RLE, "PDF": PDF, "LRI": LRI, "RLI": RLI, "FSI": FSI, "PDI": PDI}
	var levels, order []int
	count := 0
	corpusLines(t, "BidiTest.txt", func(n int, line string) {
		if strings.HasPrefix(line, "@Levels:") {
			levels = integers(t, strings.TrimPrefix(line, "@Levels:"), 10)
			return
		}
		if strings.HasPrefix(line, "@Reorder:") {
			order = integers(t, strings.TrimPrefix(line, "@Reorder:"), 10)
			return
		}
		fields := strings.Split(line, ";")
		if len(fields) != 2 {
			t.Fatalf("line %d malformed", n)
		}
		var types []Class
		for _, s := range strings.Fields(fields[0]) {
			v, ok := names[s]
			if !ok {
				t.Fatal(s)
			}
			types = append(types, v)
		}
		mask := integers(t, fields[1], 16)[0]
		for i, base := range []level{-1, 0, 1} {
			if mask&(1<<i) == 0 {
				continue
			}
			p, err := newParagraph(types, make([]bracketType, len(types)), make([]rune, len(types)), base)
			if err != nil {
				t.Fatal(err)
			}
			result, err := (&Paragraph{p: p, base: uint8(p.embeddingLevel)}).Line(0, len(types))
			if err != nil {
				t.Fatal(err)
			}
			checkLine(t, fmt.Sprintf("class line %d base %d", n, base), result, levels, order)
			count++
		}
	})
	if count != 770241 {
		t.Fatalf("class cases %d", count)
	}
	t.Logf("%d original Unicode class/direction cases", count)
}
