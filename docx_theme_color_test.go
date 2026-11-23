package manydoc

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"
)

const themeColorPalette = `<a:dk1><a:sysClr val="windowText" lastClr="102030"/></a:dk1><a:lt1><a:sysClr val="window" lastClr="F0E0D0"/></a:lt1><a:dk2><a:srgbClr val="204060"/></a:dk2><a:lt2><a:srgbClr val="E0C0A0"/></a:lt2><a:accent1><a:srgbClr val="4F81BD"/></a:accent1><a:accent2><a:srgbClr val="C0504D"/></a:accent2><a:accent3><a:srgbClr val="FF0000"/></a:accent3><a:accent4><a:srgbClr val="808080"/></a:accent4><a:accent5><a:srgbClr val="FFFFFF"/></a:accent5><a:accent6><a:srgbClr val="000000"/></a:accent6><a:hlink><a:srgbClr val="0000FF"/></a:hlink><a:folHlink><a:srgbClr val="800080"/></a:folHlink>`

// Independently authored OPC parts. No manydoc writer supplies source colors.
func themeColorDOCX(t testing.TB, body, palette, settings, styles string) []byte {
	t.Helper()
	parts := map[string]string{
		"[Content_Types].xml":          `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/></Types>`,
		"_rels/.rels":                  `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="main" Type="` + relNS + `/officeDocument" Target="word/document.xml"/></Relationships>`,
		"word/document.xml":            `<w:document xmlns:w="` + wordNS + `"><w:body>` + body + `</w:body></w:document>`,
		"word/_rels/document.xml.rels": `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="theme" Type="` + relNS + `/theme" Target="theme/theme1.xml"/><Relationship Id="settings" Type="` + relNS + `/settings" Target="settings.xml"/><Relationship Id="styles" Type="` + relNS + `/styles" Target="styles.xml"/></Relationships>`,
		"word/theme/theme1.xml":        `<a:theme xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"><a:themeElements><a:clrScheme name="Independent">` + palette + `</a:clrScheme></a:themeElements></a:theme>`,
		"word/settings.xml":            `<w:settings xmlns:w="` + wordNS + `">` + settings + `</w:settings>`,
		"word/styles.xml":              `<w:styles xmlns:w="` + wordNS + `">` + styles + `</w:styles>`,
	}
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	for name, value := range parts {
		w, err := z.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = w.Write([]byte(value)); err != nil {
			t.Fatal(err)
		}
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
func themeColorRun(attrs string) string {
	return `<w:p><w:r><w:rPr><w:color ` + attrs + `/></w:rPr><w:t>VISIBLE</w:t></w:r></w:p>`
}
func readThemeColor(t *testing.T, attrs, palette, mapping string) *Document {
	t.Helper()
	d, err := ReadDOCX(context.Background(), bytes.NewReader(themeColorDOCX(t, themeColorRun(attrs), palette, mapping, "")))
	if err != nil {
		t.Fatal(err)
	}
	return d
}
func TestDOCXThemeColorCalculationAndMapping(t *testing.T) {
	for _, tc := range []struct{ name, attrs, mapping, want string }{
		{"theme beats stale cache", `w:themeColor="accent1" w:val="FF0000"`, "", "#4F81BD"},
		{"tint without cache", `w:themeColor="accent1" w:themeTint="99"`, "", "#95B3D7"},
		{"shade red", `w:themeColor="accent3" w:themeShade="80"`, "", "#800000"},
		{"tint red", `w:themeColor="accent3" w:themeTint="80"`, "", "#FF7F7F"},
		{"tint wins", `w:themeColor="accent3" w:themeTint="80" w:themeShade="00"`, "", "#FF7F7F"},
		{"shade zero", `w:themeColor="accent1" w:themeShade="00"`, "", "#000000"},
		{"tint zero", `w:themeColor="accent1" w:themeTint="00"`, "", "#FFFFFF"},
		{"shade identity", `w:themeColor="accent1" w:themeShade="ff"`, "", "#4F81BD"},
		{"tint white", `w:themeColor="accent5" w:themeTint="80"`, "", "#FFFFFF"},
		{"shade black", `w:themeColor="accent6" w:themeShade="80"`, "", "#000000"},
		{"gray shade", `w:themeColor="accent4" w:themeShade="80"`, "", "#404040"},
		{"dark alias", `w:themeColor="dark2"`, "", "#204060"},
		{"light alias", `w:themeColor="light2"`, "", "#E0C0A0"},
		{"text mapping", `w:themeColor="text1"`, `<w:clrSchemeMapping w:t1="accent3"/>`, "#FF0000"},
		{"background mapping", `w:themeColor="background2"`, `<w:clrSchemeMapping w:bg2="accent4"/>`, "#808080"},
		{"direct dark unaffected", `w:themeColor="dark1"`, `<w:clrSchemeMapping w:t1="accent3"/>`, "#102030"},
		{"swap not recursive", `w:themeColor="accent1"`, `<w:clrSchemeMapping w:accent1="accent3" w:accent3="accent1"/>`, "#FF0000"},
		{"hyperlink slot", `w:themeColor="hyperlink"`, "", "#0000FF"},
		{"followed slot", `w:themeColor="followedHyperlink"`, "", "#800080"},
		{"inactive none", `w:themeColor="none" w:themeTint="00" w:val="123456"`, "", "#123456"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := readThemeColor(t, tc.attrs, themeColorPalette, tc.mapping)
			if got := d.Blocks[0].Runs[0].Color; got != tc.want {
				t.Fatalf("color %s, want %s", got, tc.want)
			}
			if strings.Contains(tc.attrs, `="none"`) {
				if !hasDiagnostic(d, "color_theme_unused") {
					t.Fatal("unused modifier not disclosed")
				}
			} else if !hasDiagnostic(d, "color_theme") || hasDiagnostic(d, "color_theme_cache") {
				t.Fatal("resolved theme evidence incorrect")
			}
			if tc.name == "theme beats stale cache" && !hasDiagnostic(d, "color_theme_cache_mismatch") {
				t.Fatal("stale RGB cache replacement not disclosed")
			}
			assertRecoverySF(t, d)
			var out bytes.Buffer
			if err := Write(context.Background(), &out, d, DOCX, WriteOptions{AllowLossy: true}); err != nil {
				t.Fatal(err)
			}
			back, err := ReadDOCX(context.Background(), &out)
			if err != nil || back.Blocks[0].Runs[0].Color != tc.want {
				t.Fatalf("DOCX resolved color round trip %v", err)
			}
		})
	}
}
func TestDOCXThemeColorInheritanceAndCache(t *testing.T) {
	styles := `<w:docDefaults><w:rPrDefault><w:rPr><w:color w:themeColor="accent3" w:themeShade="80"/></w:rPr></w:rPrDefault></w:docDefaults><w:style w:type="paragraph" w:styleId="P"><w:rPr><w:color w:themeColor="accent1" w:themeTint="99"/></w:rPr></w:style><w:style w:type="character" w:styleId="C"><w:rPr><w:color w:themeColor="hyperlink"/></w:rPr></w:style>`
	body := `<w:p><w:r><w:t>DEFAULT</w:t></w:r></w:p><w:p><w:pPr><w:pStyle w:val="P"/></w:pPr><w:r><w:t>PARAGRAPH</w:t></w:r><w:r><w:rPr><w:rStyle w:val="C"/></w:rPr><w:t>CHARACTER</w:t></w:r><w:r><w:rPr><w:rStyle w:val="C"/><w:color w:val="auto"/></w:rPr><w:t>DIRECT</w:t></w:r></w:p>`
	d, err := ReadDOCX(context.Background(), bytes.NewReader(themeColorDOCX(t, body, themeColorPalette, "", styles)))
	if err != nil {
		t.Fatal(err)
	}
	rr := recoveryRuns(d)
	for i, want := range []string{"#800000", "#95B3D7", "#0000FF", "auto"} {
		if rr[i].Color != want {
			t.Fatalf("inheritance %+v", rr)
		}
	}
	for _, palette := range []string{"", `<a:accent1><a:prstClr val="red"/></a:accent1>`, `<a:accent1><a:srgbClr val="FF0000"><a:lumMod val="50000"/></a:srgbClr></a:accent1>`} {
		d = readThemeColor(t, `w:themeColor="accent1" w:themeTint="00" w:val="123456"`, palette, "")
		if d.Blocks[0].Runs[0].Color != "#123456" || !hasDiagnostic(d, "color_theme_cache") {
			t.Fatal("cache transformed twice or unsupported theme hidden")
		}
	}
	d = readThemeColor(t, `w:themeColor="text1"`, themeColorPalette, "")
	if d.Blocks[0].Runs[0].Color != "#102030" || !hasDiagnostic(d, "color_system") {
		t.Fatal("system lastClr not disclosed")
	}
}
func TestDOCXThemeColorInvalid(t *testing.T) {
	for _, attrs := range []string{`w:themeColor="unknown" w:val="123456"`, `w:themeColor="accent1" w:themeTint="100"`, `w:themeColor="accent1" w:themeShade="-1"`, `w:themeColor="accent1" w:themeTint="GG"`, `w:themeColor="accent1" w:themeTint="80" w:themeShade="x"`, `w:val="12"`, `w:themeColor="accent1" w:val="GGGGGG"`, `w:themeColor="accent1" w:themeTint="+1"`} {
		if _, err := ReadDOCX(context.Background(), bytes.NewReader(themeColorDOCX(t, themeColorRun(attrs), themeColorPalette, "", ""))); err == nil {
			t.Fatalf("invalid color accepted %s", attrs)
		}
	}
	for _, mapping := range []string{`<w:clrSchemeMapping w:t1="text1"/>`, `<w:clrSchemeMapping w:t1="not-a-slot"/>`, `<w:clrSchemeMapping/><w:clrSchemeMapping/>`} {
		if _, err := ReadDOCX(context.Background(), bytes.NewReader(themeColorDOCX(t, themeColorRun(`w:themeColor="text1"`), themeColorPalette, mapping, ""))); err == nil {
			t.Fatal("invalid mapping accepted")
		}
	}
	for _, cache := range []string{"", `w:val="auto"`} {
		if _, err := ReadDOCX(context.Background(), bytes.NewReader(themeColorDOCX(t, themeColorRun(`w:themeColor="accent1" `+cache), "", "", ""))); !errors.Is(err, ErrUnsupported) {
			t.Fatalf("missing theme/cache %v", err)
		}
	}
}
func FuzzDOCXThemeLuminance(f *testing.F) {
	f.Add(uint32(0x4F81BD), byte(0x99), true)
	f.Add(uint32(0xC0504D), byte(0xBF), false)
	f.Fuzz(func(t *testing.T, color uint32, modifier byte, tint bool) {
		color &= 0xFFFFFF
		input := fmt.Sprintf("%06X", color)
		rgb, err := docxRGB(input)
		if err != nil {
			t.Fatal(err)
		}
		transformed := docxLuminance(rgb, float64(modifier)/255, tint)
		for _, v := range transformed {
			if !finite(v) || v < 0 || v > 1 {
				t.Fatal("invalid color")
			}
		}
		factor := float64(modifier) / 255
		luminance := func(c [3]float64) float64 { return (min(c[0], c[1], c[2]) + max(c[0], c[1], c[2])) / 2 }
		want := luminance(rgb) * factor
		if tint {
			want += 1 - factor
		}
		if math.Abs(luminance(transformed)-want) > 1e-12 {
			t.Fatal("HSL luminance mismatch")
		}
		if modifier == 255 && docxHex(transformed) != input {
			t.Fatal("identity altered RGB")
		}
	})
}

func FuzzDOCXThemeColorPackage(f *testing.F) {
	f.Add(uint32(0x4F81BD), byte(0x99), true)
	f.Add(uint32(0xC0504D), byte(0xBF), false)
	f.Fuzz(func(t *testing.T, color uint32, modifier byte, tint bool) {
		color &= 0xFFFFFF
		palette := fmt.Sprintf(`<a:accent1><a:srgbClr val="%06X"/></a:accent1>`, color)
		attr := "themeShade"
		if tint {
			attr = "themeTint"
		}
		props := fmt.Sprintf(`w:themeColor="accent1" w:%s="%02X" w:val="123456"`, attr, modifier)
		d := readThemeColor(t, props, palette, "")
		if d.Text() != "VISIBLE" || !hasDiagnostic(d, "color_theme") {
			t.Fatal("visible text/binding diagnostics lost")
		}
		assertRecoverySF(t, d)
		var output bytes.Buffer
		if err := Write(context.Background(), &output, d, DOCX, WriteOptions{AllowLossy: true}); err != nil {
			t.Fatal(err)
		}
		back, err := ReadDOCX(context.Background(), &output)
		if err != nil || back.Blocks[0].Runs[0].Color != d.Blocks[0].Runs[0].Color {
			t.Fatalf("resolved color round trip changed: %v", err)
		}
	})
}
