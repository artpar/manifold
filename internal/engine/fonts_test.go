package engine

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"golang.org/x/image/font"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
	"os"
	"reflect"
	"testing"
)

func TestGlyphFontSubsetKeepsOutlinesMetricsAndOwnership(t *testing.T) {
	data, err := os.ReadFile("testdata/assets/DejaVuSans.ttf")
	if err != nil {
		t.Fatal(err)
	}
	originalBytes := append([]byte{}, data...)
	f, err := sfnt.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	var buf sfnt.Buffer
	used := []uint16{0}
	for _, ch := range "ÀǺé😀" {
		gid, e := f.GlyphIndex(&buf, ch)
		if e != nil || gid == 0 {
			t.Fatal("fixture coverage", ch, e)
		}
		used = append(used, uint16(gid))
	}
	out, err := subsetGlyphTrueType(context.Background(), data, used)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) >= len(data)/2 {
		t.Fatal("unused outlines not removed", len(out), len(data))
	}
	if !bytes.Equal(data, originalBytes) {
		t.Fatal("caller font mutated")
	}
	if trueTypeChecksum(out) != 0xb1b0afba {
		t.Fatal("invalid whole-font checksum")
	}
	sub, err := sfnt.Parse(out)
	if err != nil {
		t.Fatal(err)
	}
	if sub.NumGlyphs() != f.NumGlyphs() {
		t.Fatal("GIDs renumbered")
	}
	for _, gid := range used {
		a, e := f.LoadGlyph(&buf, sfnt.GlyphIndex(gid), fixed.I(18), nil)
		if e != nil {
			t.Fatal(e)
		}
		a = append(sfnt.Segments{}, a...)
		b, e := sub.LoadGlyph(&buf, sfnt.GlyphIndex(gid), fixed.I(18), nil)
		if e != nil {
			t.Fatal(e)
		}
		if !reflect.DeepEqual(a, b) {
			t.Fatal("composite/supplementary outline changed", gid)
		}
		x, e := f.GlyphAdvance(&buf, sfnt.GlyphIndex(gid), fixed.I(18), font.HintingNone)
		if e != nil {
			t.Fatal(e)
		}
		y, e := sub.GlyphAdvance(&buf, sfnt.GlyphIndex(gid), fixed.I(18), font.HintingNone)
		if e != nil || x != y {
			t.Fatal("advance changed", gid, e)
		}
	}
	unused, _ := f.GlyphIndex(&buf, 'Z')
	segments, err := sub.LoadGlyph(&buf, unused, fixed.I(18), nil)
	if err != nil || len(segments) != 0 {
		t.Fatal("unused outline retained", err)
	}
	for _, id := range []sfnt.NameID{sfnt.NameIDCopyright, sfnt.NameIDFamily, sfnt.NameIDPostScript, sfnt.NameID(13), sfnt.NameID(14)} {
		a, ea := f.Name(&buf, id)
		b, eb := sub.Name(&buf, id)
		if a != b || (ea == nil) != (eb == nil) {
			t.Fatal("font notices/names changed", id)
		}
	}
	second, err := subsetGlyphTrueType(context.Background(), data, append(append([]uint16{}, used...), used...))
	if err != nil || !bytes.Equal(out, second) {
		t.Fatal("duplicate requests changed subset", err)
	}
}

func FuzzGlyphSubsetSelection(f *testing.F) {
	data, err := os.ReadFile("testdata/assets/DejaVuSans.ttf")
	if err != nil {
		f.Fatal(err)
	}
	original, err := sfnt.Parse(data)
	if err != nil {
		f.Fatal(err)
	}
	f.Add(uint16(171), uint16(5857))
	f.Add(uint16(444), uint16(0))
	f.Add(uint16(36), uint16(171))
	f.Fuzz(func(t *testing.T, a, b uint16) {
		ids := []uint16{uint16(int(a) % original.NumGlyphs()), uint16(int(b) % original.NumGlyphs())}
		out, err := subsetGlyphTrueType(context.Background(), data, ids)
		if err != nil {
			t.Fatal(err)
		}
		parsed, err := sfnt.Parse(out)
		if err != nil {
			t.Fatal(err)
		}
		if parsed.NumGlyphs() != original.NumGlyphs() || trueTypeChecksum(out) != 0xb1b0afba {
			t.Fatal("invalid subset inventory/checksum")
		}
		for _, id := range ids {
			var ob, sb sfnt.Buffer
			x, err := original.LoadGlyph(&ob, sfnt.GlyphIndex(id), fixed.I(24), nil)
			if err != nil {
				t.Fatal(err)
			}
			y, err := parsed.LoadGlyph(&sb, sfnt.GlyphIndex(id), fixed.I(24), nil)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(x, y) {
				t.Fatal("selected original outline changed", id)
			}
		}
	})
}

func TestGlyphSubsetRejectsBadOffsetsReferencesAndCycles(t *testing.T) {
	data, err := os.ReadFile("testdata/assets/DejaVuSans.ttf")
	if err != nil {
		t.Fatal(err)
	}
	f, err := sfnt.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	var buf sfnt.Buffer
	gid, err := f.GlyphIndex(&buf, 'é')
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"truncated", "offset", "cycle", "reference", "license"} {
		t.Run(mode, func(t *testing.T) {
			b := append([]byte{}, data...)
			var head, loca, glyf, os2 int
			for i := 0; i < int(binary.BigEndian.Uint16(b[4:])); i++ {
				r := b[12+16*i:]
				off := int(binary.BigEndian.Uint32(r[8:]))
				switch string(r[:4]) {
				case "head":
					head = off
				case "loca":
					loca = off
				case "glyf":
					glyf = off
				case "OS/2":
					os2 = off
				}
			}
			at := glyf
			if binary.BigEndian.Uint16(b[head+50:]) == 0 {
				at += 2 * int(binary.BigEndian.Uint16(b[loca+2*int(gid):]))
			} else {
				at += int(binary.BigEndian.Uint32(b[loca+4*int(gid):]))
			}
			switch mode {
			case "truncated":
				b = b[:100]
			case "offset":
				binary.BigEndian.PutUint32(b[loca:], 0xffffffff)
			case "cycle":
				binary.BigEndian.PutUint16(b[at+12:], uint16(gid))
			case "reference":
				binary.BigEndian.PutUint16(b[at+12:], 0xffff)
			case "license":
				binary.BigEndian.PutUint16(b[os2+8:], 0x100)
			}
			if _, err := subsetGlyphTrueType(context.Background(), b, []uint16{uint16(gid)}); err == nil {
				t.Fatal("bad subset accepted")
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := subsetGlyphTrueType(ctx, data, []uint16{uint16(gid)}); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation ignored", err)
	}
	if _, err := subsetGlyphTrueType(context.Background(), data, []uint16{0xffff}); err == nil {
		t.Fatal("out-of-range glyph accepted")
	}
}
