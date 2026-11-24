package manydoc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestStrictJSONUnicodeKeysAndSyntax(t *testing.T) {
	for _, s := range []string{`null`, `true`, `false`, `0`, `-0`, `1.5E+2`, `-0.32e-10`, `"😀�"`, `"\ud83d\ude00"`, `"\\ud800"`, `{"é":1,"é":2}`, `{"a":1,"nested":{"a":2}}`, `{"\uD83D\uDE00":"\b\f\n\r\t\/\\\""}`, " \t[{}, [], 12]\r\n"} {
		if err := validateStrictJSON(context.Background(), []byte(s)); err != nil {
			t.Fatalf("valid %q: %v", s, err)
		}
	}
	for _, s := range []string{``, `01`, `-`, `+1`, `1.`, `1e`, `1e+`, `NaN`, `truefalse`, `{} {}`, `[1,]`, `{"a":1,}`, `{a:1}`, `{"a" 1}`, `{"a":1 "b":2}`, `"\x"`, `"\u000g"`, `"\ud800"`, `"\udc00"`, `"\ud800\u0041"`, `"\ud800\ud800"`, `"\ud800x"`, "\"line\nfeed\"", "\"\xff\"", "\"\xed\xa0\x80\"", `{"a":1,"\u0061":2}`, `{"😀":1,"\uD83D\uDE00":2}`, `[{"x":null,"x":true}]`, strings.Repeat("[", 130) + "0" + strings.Repeat("]", 130)} {
		if err := validateStrictJSON(context.Background(), []byte(s)); err == nil {
			t.Fatalf("accepted invalid %q", s)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := validateStrictJSON(ctx, []byte(`{}`)); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func FuzzStrictJSON(f *testing.F) {
	for _, s := range []string{`{"a":[1,"😀",true,null]}`, `"\ud83d\ude00"`, `{"a":1,"\u0061":2}`, `-1.2e-3`, `"\ud800"`} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		err := validateStrictJSON(context.Background(), data)
		if err == nil && !json.Valid(data) {
			t.Fatalf("accepted invalid JSON %q", data)
		}
		// Standard decoding cannot create invalid Unicode or duplicate object keys.
		// Its re-encoding must always be accepted, except for our explicit depth bound.
		var value any
		if json.Unmarshal(data, &value) == nil {
			canonical, e := json.Marshal(value)
			if e != nil {
				return
			}
			if e = validateStrictJSON(context.Background(), canonical); e != nil && !strings.Contains(e.Error(), "nesting exceeds") {
				t.Fatal(e)
			}
		}
	})
}

func TestStrictJSONValueScannerAvoidsLargeStringCopies(t *testing.T) {
	data := append([]byte{'"'}, bytes.Repeat([]byte{'a'}, 1<<20)...)
	data = append(data, '"')
	if got := testing.AllocsPerRun(3, func() {
		if err := validateStrictJSON(context.Background(), data); err != nil {
			panic(err)
		}
	}); got != 0 {
		t.Fatalf("large string scan allocated %g times", got)
	}
}
