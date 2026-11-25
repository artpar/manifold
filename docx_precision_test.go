package manifold

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestDOCXSectionedStyledNoteSemantics(t *testing.T) {
	d := variantNoteDocument()
	d.Blocks[0].Anchors = []string{"return"}
	d.Notes[0].BodyLabelStyle = &Run{FontSize: 9.5, Bold: true, Link: "#return", Destination: "return"}
	d.Blocks = []Block{{Kind: SectionBlock, Section: &Section{Blocks: d.Blocks}}}
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, DOCX, WriteOptions{AllowLossy: true}); err != nil {
		t.Fatal(err)
	}
	got, err := ReadDOCX(context.Background(), &out)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := ToStructured(d)
	after, _ := ToStructured(got)
	if !reflect.DeepEqual(before.Content, after.Content) || !reflect.DeepEqual(before.Attrs.Notes, after.Attrs.Notes) {
		t.Fatal("sectioned note semantics changed")
	}
}

func precisionDocument(sectioned, variants bool) *Document {
	d := noteDocument()
	if variants {
		d = variantNoteDocument()
		d.Blocks[0].Anchors = []string{"return"}
		d.Notes[0].BodyLabelStyle = &Run{FontSize: 6.959646, Bold: true, Destination: "return", Link: "#return"}
	}
	for i := range d.Blocks {
		for j := range d.Blocks[i].Runs {
			d.Blocks[i].Runs[j].FontSize = 10.24999
		}
	}
	for i := range d.Blocks[1].Items {
		for j := range d.Blocks[1].Items[i].Blocks {
			for k := range d.Blocks[1].Items[i].Blocks[j].Runs {
				d.Blocks[1].Items[i].Blocks[j].Runs[k].FontSize = 10.24999
			}
		}
	}
	d.Blocks = append(d.Blocks, Block{Kind: Heading, Level: 2, Runs: []Run{{Text: "FRACTIONAL_HEADING", FontSize: 18.49}}})
	d.Notes[0].Blocks = []Block{{Kind: Paragraph, Runs: []Run{{Text: "FRACTIONAL_NOTE", FontSize: 9.959491}}}}
	d.Blocks = append(d.Blocks, Block{Kind: Table, Rows: []Row{{Cells: []Cell{{Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "FRACTIONAL_CELL", FontSize: 10.25, Bold: true}}}}}}}}})
	d.Headers = []Block{{Kind: Paragraph, Runs: []Run{{Text: "FRACTIONAL_HEADER", FontSize: 8.51}}}}
	d.Footers = []Block{{Kind: Paragraph, Runs: []Run{{Text: "FRACTIONAL_FOOTER", FontSize: 8.49}}}}
	if sectioned {
		d.Blocks = []Block{{Kind: SectionBlock, Section: &Section{Blocks: d.Blocks, Headers: RunningStories{Default: &RunningStory{Blocks: d.Headers}}, Footers: RunningStories{Default: &RunningStory{Blocks: d.Footers}}}}}
		d.Headers = nil
		d.Footers = nil
		d.EvenAndOddHeaders = true
		s := d.Blocks[0].Section
		s.FirstPage = true
		story := func(text string, size float64) *RunningStory {
			return &RunningStory{Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: text, FontSize: size}}}}}
		}
		s.Headers.First = story("FIRST_HEADER", 8.25)
		s.Headers.Even = story("EVEN_HEADER", 8.75)
		s.Footers.First = story("FIRST_FOOTER", 9.25)
		s.Footers.Even = story("EVEN_FOOTER", 9.75)
	}
	return d
}

func TestDOCXFractionalSizeIntentAllStories(t *testing.T) {
	for _, sectioned := range []bool{false, true} {
		for _, variants := range []bool{false, true} {
			d := precisionDocument(sectioned, variants)
			original, _ := json.Marshal(d)
			var out bytes.Buffer
			if err := Write(context.Background(), &out, d, DOCX, WriteOptions{AllowLossy: true}); err != nil {
				t.Fatal(sectioned, variants, err)
			}
			got, err := ReadDOCX(context.Background(), &out)
			if err != nil {
				t.Fatal(sectioned, variants, err)
			}
			before, _ := ToStructured(d)
			after, err := ToStructured(got)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before.Content, after.Content) || !reflect.DeepEqual(before.Attrs.Notes, after.Attrs.Notes) || !reflect.DeepEqual(before.Attrs.Headers, after.Attrs.Headers) || !reflect.DeepEqual(before.Attrs.Footers, after.Attrs.Footers) || before.Attrs.EvenAndOddHeaders != after.Attrs.EvenAndOddHeaders {
				t.Fatalf("fractional semantics changed: sectioned %v variants %v", sectioned, variants)
			}
			if !hasDiagnostic(got, "docx_font_size_precision") {
				t.Fatal("precision projection not disclosed")
			}
			current, _ := json.Marshal(d)
			if !bytes.Equal(original, current) {
				t.Fatal("writer mutated original precision")
			}
			var sf bytes.Buffer
			if err := Write(context.Background(), &sf, got, SFGzip, WriteOptions{}); err != nil {
				t.Fatal(err)
			}
			exact, err := Read(context.Background(), &sf, SFGzip, ReadOptions{})
			if err != nil {
				t.Fatal(err)
			}
			exactSF, _ := ToStructured(exact)
			if !reflect.DeepEqual(exactSF, after) {
				t.Fatal("restored precision changed through gzip SF")
			}
		}
	}
}

func TestDOCXSizeProjectionBoundsAndStrictAdmission(t *testing.T) {
	for _, size := range []float64{.25, .24999, .1, 10.24999, 10.25, 10.25001, 6.959646, 2147483647.49, 2147483648} {
		d := &Document{Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "EXACT", FontSize: size}}}}}
		for _, allow := range []bool{false, true} {
			var out bytes.Buffer
			err := Write(context.Background(), &out, d, DOCX, WriteOptions{AllowLossy: allow})
			valid := allow && size >= .25 && size < 2147483647.75
			if !valid {
				if !errors.Is(err, ErrUnsupported) || out.Len() != 0 {
					t.Fatal("unrepresentable/strict size accepted", size, allow, err)
				}
				continue
			}
			if err != nil {
				t.Fatal(size, err)
			}
			got, err := ReadDOCX(context.Background(), &out)
			if err != nil {
				t.Fatal(size, err)
			}
			if got.Blocks[0].Runs[0].FontSize != size {
				t.Fatal("original precision changed", size, got.Blocks[0].Runs[0].FontSize)
			}
		}
	}
}

func TestDOCXSizePrecisionRejectsContradictions(t *testing.T) {
	d := &Document{Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "VISIBLE", FontSize: 10.25}}}}}
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, DOCX, WriteOptions{AllowLossy: true}); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(string) string{
		func(s string) string { return strings.Replace(s, `md:fontSize="10.25"`, `md:fontSize="NaN"`, 1) },
		func(s string) string { return strings.Replace(s, `md:fontSize="10.25"`, `md:fontSize=""`, 1) },
		func(s string) string {
			return strings.Replace(s, `<w:sz w:val="21"/>`, `<x:sz xmlns:x="urn:lookalike" w:val="21"/>`, 1)
		},
		func(s string) string {
			return strings.Replace(s, `<w:sz w:val="21"/>`, `<w:sz xmlns:x="urn:lookalike" x:val="21"/>`, 1)
		},
		func(s string) string { return strings.Replace(s, `md:fontSize="10.25"`, `md:fontSize="10"`, 1) },
		func(s string) string { return strings.Replace(s, `md:fontSize="10.25"`, `md:fontSize="9.25"`, 1) },
		func(s string) string { return strings.Replace(s, `md:fontSize="10.25"`, `md:fontSize="0.1"`, 1) },
		func(s string) string { return strings.Replace(s, `<w:sz w:val="21"/>`, `<w:sz w:val="20"/>`, 1) },
		func(s string) string { return strings.Replace(s, `<w:szCs w:val="21"/>`, ``, 1) },
		func(s string) string {
			return strings.Replace(s, `md:fontSize="10.25"`, `md:fontSize="10.25" md:textImplicit="s"`, 1)
		},
	} {
		bad := rewriteListDOCX(t, out.Bytes(), change)
		if _, err := ReadDOCX(context.Background(), bytes.NewReader(bad)); err == nil {
			t.Fatal("contradictory precision accepted")
		}
	}
	// Stripping ignorable intent exposes the ordinary portable half-point size.
	plain := rewriteListDOCX(t, out.Bytes(), func(s string) string { return strings.Replace(s, ` md:fontSize="10.25"`, "", 1) })
	got, err := ReadDOCX(context.Background(), bytes.NewReader(plain))
	if err != nil || got.Blocks[0].Runs[0].FontSize != 10.5 {
		t.Fatal("standard output size is not authoritative", err)
	}
}

func TestXMLDuplicateExpandedAttributesReject(t *testing.T) {
	for _, input := range []string{`<r a="1" a="2"/>`, `<r xmlns:m="urn:x" xmlns:n="urn:x" m:a="1" n:a="2"/>`} {
		if _, err := parseXML(context.Background(), []byte(input)); err == nil {
			t.Fatal("duplicate expanded XML attribute accepted")
		}
	}
	if _, err := parseXML(context.Background(), []byte(`<r xmlns:m="urn:x" xmlns:n="urn:y" m:a="1" n:a="2"/>`)); err != nil {
		t.Fatal("distinct namespaces conflated", err)
	}
}

func TestDOCXFractionalComplexScriptSize(t *testing.T) {
	d := &Document{Blocks: []Block{{Kind: Paragraph, Direction: "rtl", Runs: []Run{{Text: "مرحبا", FontSize: 13.333, ComplexScript: true, Direction: "rtl", Language: "ar", FontFamily: "DejaVu Sans", Bold: true}}}}}
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, DOCX, WriteOptions{AllowLossy: true}); err != nil {
		t.Fatal(err)
	}
	got, err := ReadDOCX(context.Background(), &out)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(d.Blocks, got.Blocks) {
		t.Fatalf("complex size intent changed: %+v", got.Blocks)
	}
}
