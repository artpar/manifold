package engine

import (
	"bytes"
	"compress/zlib"
	"context"
	"encoding/ascii85"
	"encoding/binary"
	"encoding/hex"

	// Parent ActualText is outside the Link element. Child link ownership must
	// survive replacement; re-applying the child's Link role later cannot fix it.
	"encoding/json"
	"errors"
	"fmt"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestPDFActualTextRetainsInternalNavigation(t *testing.T) {
	for _, content := range []string{markedText(0, 20, 300, "XY"), markedText(0, 20, 300, "X") + markedText(1, 26, 300, "Y")} {
		objects := taggedTestObjects(content,
			`<< /Type /StructTreeRoot /K 8 0 R >>`,
			`<< /Type /StructElem /S /P /P 7 0 R /Pg 3 0 R /ActualText (replacement) /K 9 0 R >>`,
			`<< /Type /StructElem /S /Link /P 8 0 R /K [0 << /Type /OBJR /Obj 10 0 R >>] >>`,
			`<< /Type /Annot /Subtype /Link /Rect [15 290 65 315] /A << /S /GoTo /D [3 0 R /XYZ 20 300 null] >> >>`)
		if strings.Count(content, "MCID") > 1 {
			objects[8] = strings.Replace(objects[8], "/K [0 ", "/K [0 1 ", 1)
		}
		objects[2] = strings.Replace(objects[2], "/Contents 5 0 R", "/Annots [10 0 R] /Contents 5 0 R", 1)
		d := readRecovery(t, recoveryPDF(objects...))
		var replacement *Run
		for i := range d.Blocks {
			for j := range d.Blocks[i].Runs {
				if d.Blocks[i].Runs[j].Text == "replacement" {
					replacement = &d.Blocks[i].Runs[j]
				}
			}
		}
		if replacement == nil || replacement.Destination == "" || replacement.Link == "" {
			t.Fatal("ActualText lost internal target", replacement, d.Diagnostics)
		}
		if d.PDFNavigation == nil || len(d.PDFNavigation.Links) != 1 || replacement.Destination != d.PDFNavigation.Links[0].Destination {
			t.Fatal("replacement target contradicts source annotation")
		}
		assertRecoverySF(t, d)
		// Source physical fields still require explicit retypesetting for output.
		// Exercise SF/DOCX while retaining the target's typed identity.

		// An acyclic shared-object graph amplifies visits without exceeding depth16.
		// At fanout8/depth6 it expands262144 leaves from about1KiB of source arrays.

		replacement.FontFamily = "Go"
		replacement.FontSize = 10
		var word bytes.Buffer
		if e := Write(context.Background(), &word, d, DOCX, WriteOptions{AllowLossy: true}); e != nil {
			t.

				// The same class supplies repeated Decimal attributes. Local LowerRoman wins.
				Fatal(e)
		}
		back, e := ReadDOCX(context.Background(), &word)
		if e != nil {
			t.Fatal(e)
		}
		found := false
		_ = documentDestinationRuns(back, func(r Run) error {
			if r.Text == "replacement" && r.Destination == replacement.Destination {
				found = true
			}
			return nil
		})
		if !found {
			t.Fatal("internal navigation changed to URI during DOCX output")
		}
	}
}
func TestPDFActualTextLinkIntersection(t *testing.T) {
	for _, pair := range []struct {
		a, b     Run
		mixed    bool
		id, href string
	}{
		{Run{Link: "#target", Destination: "target"}, Run{Link: "#target", Destination: "target"}, false, "target", "#target"},
		{Run{Link: "#%74arget", Destination: "target"}, Run{Link: "#target", Destination: "target"}, false, "target", "#%74arget"},
		{Run{Link: "#target", Destination: "target"}, Run{Link: "#target"}, true, "", ""},
		{Run{Link: "#left", Destination: "left"}, Run{Link: "#right", Destination: "right"}, true, "", ""},
		{Run{Link: "https://example.invalid"},

			// The context transitions to cancelled during attribute visits, without a timer
			// or scheduler-dependent race. It otherwise delegates the Context contract.
			Run{Link: "https://example.invalid"}, false, "", "https://example.invalid"},
	} {
		got, mixed := commonTextStyle(pair.a, pair.b)
		if mixed != pair.mixed || got.Destination != pair.id || got.Link != pair.href {
			t.Fatal("incorrect common target", pair, got, mixed)
		}
	}
}

func pdfAttributeDAG(levels, fanout int, content string, owners int) []byte {
	leaf := 8 + owners
	refs := make([]string, owners)
	contentStream := ""
	for i := range refs {
		refs[i] = fmt.Sprintf("%d 0 R", 8+i)
		contentStream += markedText(i, 20, 300-i*20, fmt.Sprintf("BODY%d", i))
	}
	root := fmt.Sprintf("<< /Type /StructTreeRoot /K [%s] /ClassMap << /Shared %d 0 R >> >>", strings.Join(refs, " "), leaf+levels)
	elements := []string{}
	for i := 0; i < owners; i++ {
		elements = append(elements, fmt.Sprintf("<< /Type /StructElem /S /P /P 7 0 R /Pg 3 0 R /C /Shared /K %d >>", i))
	}
	elements = append(elements, content)
	for i := 0; i < levels; i++ {
		elements = append(elements, "["+strings.Repeat(fmt.Sprintf("%d 0 R ", leaf+i), fanout)+"]")
	}
	return taggedTestPDF(contentStream, root, elements...)
}

func TestPDFAttributeExpansionBudget(t *testing.T) {
	for _, tc := range []struct {
		name           string
		levels, owners int
	}{
		{"shared-array", 6, 1},
		{"shared-across-owners", 5, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := pdfAttributeDAG(tc.levels, 8, "<< /O /List /ListNumbering /Decimal >>", tc.owners)
			if len(data) > 5000 {
				t.Fatal("fixture unexpectedly materializes expanded graph")
			}
			d, err := ReadPDF(context.Background(), bytes.NewReader(data), ReadOptions{})
			if err == nil || d != nil || !strings.Contains(err.Error(), "structure attributes exceed") {
				t.Fatalf("small shared graph was not bounded: doc=%v err=%v", d != nil, err)
			}
		})
	}
}

func TestPDFAttributeSharedGraphPreservesOverrides(t *testing.T) {

	objects := taggedTestObjects(markedText(0, 20, 300, "iv.")+markedText(1, 40, 300, "Body"),
		"<< /Type /StructTreeRoot /K 8 0 R /ClassMap << /Shared 15 0 R >> >>",
		"<< /Type /StructElem /S /L /P 7 0 R /Pg 3 0 R /C /Shared /A << /O /List /ListNumbering /LowerRoman >> /K 9 0 R >>",
		"<< /Type /StructElem /S /LI /P 8 0 R /K [10 0 R 11 0 R] >>",
		"<< /Type /StructElem /S /Lbl /P 9 0 R /K 0 >>",
		"<< /Type /StructElem /S /LBody /P 9 0 R /K 1 >>",
		"<< /O /List /ListNumbering /Decimal >>")
	for i := 12; i < 15; i++ {
		objects = append(objects, "["+strings.Repeat(fmt.Sprintf("%d 0 R ", i), 8)+"]")
	}
	d := readRecovery(t, recoveryPDF(objects...))
	assertPDFNumberedLabels(t, d, 4, []string{"iv."})
	if d.Blocks[0].Items[0].Source.Label != "iv." || d.Blocks[0].Items[0].Blocks[0].Runs[0].Text != "Body" {
		t.Fatal("shared class or local override changed content")
	}
	assertRecoverySF(t, d)
}

type pdfAttributeCancelContext struct {
	context.Context
	cancel context.CancelFunc
	calls  int
}

func (c *pdfAttributeCancelContext) Err() error {
	c.calls++
	if c.calls == 4 {
		c.cancel()
	}
	return c.Context.Err()
}
func TestPDFAttributeTraversalCancellation(t *testing.T) {
	data := pdfAttributeDAG(3, 8, "<< /O /List /ListNumbering /Decimal >>", 1)
	p, err := pdfcpu.ReadWithContext(context.Background(), bytes.

		// Source PDF cannot expose the hidden counter start42. Once imported, the
		// explicit editorial default must survive SF/DOCX without further changes.

		// Native HarfBuzz oracle: default Noto office uses the ffi ligature.

		NewReader(data), &model.Configuration{Reader15: true, ValidationMode: model.ValidationRelaxed, Offline: true})
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := p.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	root, err := p.DereferenceDict(catalog["StructTreeRoot"])
	if err != nil {
		t.Fatal(err)
	}
	classes, err := p.DereferenceDict(root["ClassMap"])
	if err !=

		// Physical wrapping adds newlines; authored hard breaks are also logical
		// newlines. Every other original scalar, including marks and SHY, must survive.

		// Every painted line is within the page, in its independent parallel column.
		nil {
		t.Fatal(err)
	}
	owner, err := p.DereferenceDict(*types.NewIndirectRef(8, 0))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c := &pdfAttributeCancelContext{Context: ctx, cancel: cancel}
	recovery := pdfStructureRecovery{reader: &pdfReader{ctx: c, pdf: p, doc: &Document{}}, classes: classes}
	n := pdfStructureNode{attrs: map[string]types.Object{}}
	if err := recovery.attributes(&n, owner); !errors.Is(err, context.Canceled) {
		t.Fatalf("attribute walk ignored cancellation: %v", err)
	}
}

func FuzzPDFAttributeGraph(f *testing.F) {
	f.Add(uint8(6), uint8(7), uint8(0))
	f.Add(uint8(5), uint8(7), uint8(1))
	f.Add(uint8(3), uint8(1), uint8(2))
	f.Fuzz(func(t *testing.T, depth, branches, count uint8) {
		levels, fanout, owners := int(depth)%13, int(branches)%16+1, int(count)%4+1
		data := pdfAttributeDAG(levels, fanout, "<< /O /List /ListNumbering /Decimal >>", owners)
		d, err := ReadPDF(context.Background(), bytes.NewReader(data), ReadOptions{})
		if err != nil {
			if !strings.Contains(err.Error(), "structure attributes exceed") {
				t.Fatal("unexpected rejection", err)
			}
			return
		}
		if len(d.Blocks) != owners {
			t.Fatal("attribute graph changed forward membership")
		}
		for i, b := range d.Blocks {
			if b.Kind != Paragraph || paragraphText(b) != fmt.Sprintf("BODY%d", i) || b.Source == nil || b.Source.Structure != "explicit" {
				t.Fatal("attribute graph changed content/order/source")
			}
		}
		assertRecoverySF(t, d)
	})
}
func TestPDFConstantListPattern(t *testing.T) {
	for _, label := range []string{"", "§ fixed", "Case A", "100% done"} {
		for _, family := range []string{"Decimal", "UpperRoman", "LowerRoman", "UpperAlpha", "LowerAlpha"} {
			p, ok := pdfConstantListNumbering(family, []string{label, label})
			if !ok || p.start != 1 || p.numbering.Template != label {
				t.Fatal("constant labels not preserved", family, label)
			}
		}
	}
	for _, tc := range []struct {
		family string
		labels []string
	}{
		{"None", []string{"§", "§"}}, {"Disc", []string{"x", "x"}},
		{"Unknown", []string{"x"}}, {"Decimal", []string{"A", "B"}},
		{"Decimal", []string{"%1"}}, {"Decimal", []string{"x%2"}},
		{"Decimal", []string{strings.Repeat("x", 513)}}, {"Decimal", nil},
	} {
		if _, ok := pdfConstantListNumbering(tc.family, tc.labels); ok {
			t.Fatal("unproven literal template accepted", tc.family, tc.labels)
		}
	}
}

func TestGeneratedPDFConstantListFourPaths(t *testing.T) {
	ctx := context.Background()
	for index, label := range []string{"§ fixed", "Case A", "100% done", ""} {
		t.Run(fmt.Sprint(index), func(t *testing.T) {
			original := &Document{Blocks: []Block{{Kind: ListBlock, List: "number", Start: listStartPointer(42), Numbering: &ListNumbering{Template: label, Formats: []string{"lowerRoman"}}, Items: []ListItem{
				{Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "FIRST_BODY", Bold: true, Link: "https://example.org/first"}}}}},
				{Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "SECOND_BODY", Italic: true}}}}},
			}}}}
			sf, err := ToStructured(original)
			if err != nil {
				t.Fatal(err)
			}
			original, err = FromStructured(sf)
			if err != nil {
				t.Fatal(err)
			}
			var directWord bytes.Buffer
			if err = Write(ctx, &directWord, original, DOCX, WriteOptions{}); err != nil {
				t.Fatal(err)
			}
			direct, err := ReadDOCX(ctx, &directWord)
			if err != nil {
				t.Fatal(err)
			}
			assertPDFNumberedLabels(t, direct, 42, []string{label, label})
			if direct.Blocks[0].Numbering == nil || direct.Blocks[0].Numbering.Template != label || direct.Blocks[0].Numbering.Formats[0] != "lowerRoman" {
				t.Fatal("direct DOCX changed the authored hidden counter/template")
			}
			var pdf bytes.Buffer
			if err = Write(ctx, &pdf, original, PDF, WriteOptions{}); err != nil {
				t.Fatal(err)
			}
			rawPDF := bytes.Clone(pdf.Bytes())
			got, err := ReadPDF(ctx, &pdf, ReadOptions{})
			if err != nil {
				t.Fatal(err)
			}
			assertPDFNumberedLabels(t, got, 1, []string{label, label})
			if got.Blocks[0].Numbering == nil || got.Blocks[0].Numbering.Template != label || got.Blocks[0].Numbering.Formats[0] != "lowerRoman" || !hasDiagnostic(got, "pdf_list_constant_inferred") || strings.Contains(got.Blocks[0].Source.Evidence, "start_from_label") || !strings.Contains(got.Blocks[0].Source.Evidence, "start_unobserved") {
				t.Fatal("hidden start falsely recovered", got.Diagnostics)
			}
			for i, item := range got.Blocks[0].Items {
				if item.Source.Label != label || item.Source.Role != "LI" {
					t.Fatal("original explicit item/label evidence lost")
				}
				runs := item.Blocks[0].Runs
				if len(runs) != 1 || runs[0].Text != []string{"FIRST_BODY", "SECOND_BODY"}[i] || i == 0 && (!runs[0].Bold || runs[0].Link != "https://example.org/first") || i == 1 && !runs[0].Italic {
					t.Fatal("body formatting/relationship lost")
				}
			}
			assertRecoverySF(t, got)

			var word bytes.Buffer
			if err = Write(ctx, &word, got, DOCX, WriteOptions{AllowLossy: true}); err != nil {
				t.Fatal(err)
			}
			rawWord := bytes.Clone(word.Bytes())
			back, err := ReadDOCX(ctx, &word)
			if err != nil {
				t.Fatal(err)
			}
			assertPDFNumberedLabels(t, back, 1, []string{label, label})
			if back.Blocks[0].Numbering == nil || back.Blocks[0].Numbering.Template != label || back.Blocks[0].Numbering.Formats[0] != "lowerRoman" {
				t.Fatal("DOCX literal format/template changed")
			}
			if dir := os.Getenv("MANYDOC_PDF_CONSTANT_LIST_QA"); dir != "" {
				if err = os.MkdirAll(dir, 0755); err != nil {
					t.Fatal(err)
				}
				var out bytes.Buffer
				tree, e := ToStructured(got)
				if e != nil {
					t.Fatal(e)
				}
				if err = WriteStructured(ctx, &out, tree); err != nil {
					t.Fatal(err)
				}
				for ext, data := range map[string][]byte{"pdf": rawPDF, "docx": rawWord, "sf": out.Bytes()} {
					if err = os.WriteFile(filepath.Join(dir, fmt.Sprintf("case-%d.%s", index, ext)), data, 0600); err != nil {
						t.Fatal(err)
					}
				}
			}
		})
	}
}

func FuzzPDFConstantListNumbering(f *testing.F) {
	f.Add("LowerRoman", "§ fixed", "§ fixed")
	f.Add("Decimal", "", "")
	f.Add("UpperAlpha", "A", "B")
	f.Add("Decimal", "%1", "%1")
	f.Fuzz(func(t *testing.T, family, first, second string) {
		if len(family) > 32 || len(first) > 1024 || len(second) > 1024 {
			t.Skip()
		}
		pattern, ok := pdfConstantListNumbering(family, []string{first, second})
		if !ok {
			return
		}
		if first != second || pattern.start != 1 || validateNumbering(pattern.numbering) != nil {
			t.Fatal("invalid literal pattern returned")
		}
		b := Block{Kind: ListBlock, List: "number", Numbering: pattern.numbering}
		for i := 0; i < 3; i++ {
			actual, err := numberedLabel(b, i)
			if err != nil || actual != first {
				t.Fatal("literal template became a counter", actual, err)
			}
		}
	})
}
func TestDefaultTextShapingMatchesUndAndKnownGlyphs(t *testing.T) {
	data, err := os.ReadFile("testdata/assets/NotoSans-Regular.ttf")
	if err != nil {
		t.Fatal(err)
	}
	renderer := func() *pdfRenderer {
		r := textTestRenderer()
		if err := r.addFonts([]FontFace{{Family: "Noto Sans", Data: data}}); err != nil {
			t.Fatal(err)
		}
		return r
	}
	plain := Run{Text: "office AV To", FontFamily: "Noto Sans", FontSize: 20}
	tagged := plain
	tagged.Language = "und"
	a, err := renderer().lines([]Run{plain}, 80, 20)
	if err != nil {
		t.Fatal(err)
	}
	b, err := renderer().lines([]Run{tagged}, 80, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(a) != len(b) {
		t.Fatal("language tag still changes wrapping")
	}
	for i := range a {
		if lineText(a[i]) != lineText(b[i]) || pdfAdvance(a[i]) != pdfAdvance(b[i]) || len(a[i]) != len(b[i]) {
			t.Fatalf("tag-dependent layout %d", i)
		}
		for j := range a[i] {
			if !reflect.DeepEqual(a[i][j].glyphs.positions, b[i][j].glyphs.positions) {
				t.Fatal("tag-dependent glyphs/positions")
			}
		}
	}

	r := renderer()
	first, second := plain, plain
	first.Text = "of"
	second.Text = "fice"
	lines, err := r.lines([]Run{first, second}, 200, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 1 || len(lines[0]) != 1 {
		t.Fatalf("equal default runs did not coalesce: %+v", lines)
	}
	for i, want := range []uint16{82, 1654, 70, 72} {
		if len(lines[0][0].glyphs.positions) != 4 || lines[0][0].glyphs.positions[i].glyph != want {
			t.Fatal("default ligature missing")
		}
	}
	plain.Text = "ffi"
	lines, err = r.lines([]Run{plain}, 8, 20)
	if err != nil || len(lines) != 3 {
		t.Fatalf("default emergency re-shaping %+v %v", lines, err)
	}
	for i, want := range []uint16{73, 73, 76} {
		if len(lines[i][0].glyphs.positions) != 1 || lines[i][0].glyphs.positions[0].glyph != want {
			t.Fatal("default ligature sliced at emergency boundary")
		}
	}
}

func TestPDFControlOnlyRetainsActualTextVersionAndContent(t *testing.T) {
	d := &Document{Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "\t\n"}}}}}
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, PDF, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(out.Bytes(), []byte("%PDF-2.0")) {
		t.Fatal("tagged control-only ActualText output needs PDF 2.0")
	}
	back, err := ReadPDF(context.Background(), &out, ReadOptions{PDFSkipInference: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(back.Text(), "\t") {
		t.Fatalf("lost control %q", back.Text())
	}
}

func FuzzDefaultPDFText(f *testing.F) {
	data, err := os.ReadFile("testdata/assets/NotoSans-Regular.ttf")
	if err != nil {
		f.Fatal(err)
	}
	f.Add([]byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, byte(20))
	f.Add([]byte{6, 8, 7, 0, 0, 1, 5, 4}, byte(100))
	f.Fuzz(func(t *testing.T, input []byte, width byte) {
		if len(input) > 24 {
			return
		}
		alphabet := []string{"office", "AV", "To", "б", "Ω", "Ж", "B\u0315", "e\u0301", " ", "\n", "\u00ad"}
		var text strings.Builder
		for _, b := range input {
			text.WriteString(alphabet[int(b)%len(alphabet)])
		}
		original := text.String()
		d := &Document{Layout: Layout{Width: 180 + float64(width), Height: 320, Margin: 20}, Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: original, FontFamily: "Noto Sans", FontSize: 14}}}}}
		var out bytes.Buffer
		if err := Write(context.Background(), &out, d, PDF, WriteOptions{PDFFonts: []FontFace{{Family: "Noto Sans", Data: data}}}); err != nil {
			if !errors.Is(err, ErrUnsupported) || out.Len() != 0 {
				t.Fatalf("non-atomic default shaping %v", err)
			}
			return
		}
		back, err := ReadPDF(context.Background(), &out, ReadOptions{PDFSkipInference: true})
		if err != nil {
			t.Fatal(err)
		}

		if strings.ReplaceAll(back.Text(), "\n", "") != strings.ReplaceAll(original, "\n", "") {
			t.Fatalf("default logical text %q -> %q", original, back.Text())
		}
		assertRecoverySF(t, back)
	})
}

func TestSpecialSpaceCombiningMarkKeepsPositionedGlyphs(t *testing.T) {
	data, err := os.ReadFile("testdata/assets/NotoSans-Regular.ttf")
	if err != nil {
		t.Fatal(err)
	}
	text := "A\u2007\u0315B"
	r := textTestRenderer()
	if err := r.addFonts([]FontFace{{Family: "Noto Sans", Data: data}}); err != nil {
		t.Fatal(err)
	}
	lines, err := r.lines([]Run{{Text: text, FontFamily: "Noto Sans", FontSize: 20}}, 200, 20)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, span := range lines[0] {
		if strings.Contains(span.text, "\u0315") {
			found = true
			if span.glyphs == nil || len(span.glyphs.positions) != 2 {
				t.Fatal("marked special space fell back to unpositioned primitives")
			}
		}
	}
	if !found {
		t.Fatal("lost mark")
	}
	d := &Document{Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: text, FontFamily: "Noto Sans", FontSize: 20}}}}}
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, PDF, WriteOptions{PDFFonts: []FontFace{{Family: "Noto Sans", Data: data}}}); err != nil {
		t.Fatal(err)
	}
	back, err := ReadPDF(context.Background(), &out, ReadOptions{PDFSkipInference: true})
	if err != nil {
		t.Fatal(err)
	}
	if back.Text() != text {
		t.Fatalf("marked special space changed: %q", back.Text())
	}
}
func TestPDFLiteralMarkersRichOwnership(t *testing.T) {
	d := literalMarkerDocument()
	before := canonicalSF(t, d)
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, PDF, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, canonicalSF(t, d)) {
		t.Fatal("caller mutated")
	}
	got, err := ReadPDF(context.Background(), bytes.NewReader(out.Bytes()), ReadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Blocks) != 1 || got.Blocks[0].Kind != ListBlock || len(got.Blocks[0].Items) != 3 {
		t.Fatalf("list hierarchy: %#v", got.Blocks)
	}
	items := got.Blocks[0].Items
	for i, item := range items {
		if item.Marker == nil || item.Marker.Source.Role != "Lbl" || item.Marker.Source.ParentElement != item.Source.Element {
			t.Fatalf("marker %d ownership lost", i)
		}
		if !strings.Contains(layoutText(item.Blocks[0]), []string{"FIRST", "SECOND", "EMPTY"}[i]) {
			t.Fatal("body correspondence")
		}
	}
	first := items[0].Marker.Blocks[0].Runs
	if len(first) == 0 || first[0].Text != "α)" || !first[0].Bold || first[0].Link != "https://example.org/alpha" {
		t.Fatalf("label formatting/URI lost: %+v", first)
	}
	noteFound := false
	for _, b := range items[1].Marker.Blocks {
		for _, r := range b.Runs {
			if r.Note != nil {
				noteFound = true
			}
		}
	}
	if !noteFound || len(got.Notes) != 1 || !strings.Contains(got.Text(), "Marker note") {
		t.Fatalf("marker citation graph lost: %+v", got.Notes)
	}
	assertRecoverySF(t, got)
	exportLiteralOutput(t, "rich", out.Bytes(), got)
}

func exportLiteralOutput(t *testing.T, name string, raw []byte, d *Document) {
	t.Helper()
	dir := os.Getenv("MANYDOC_LITERAL_OUTPUT_QA")
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name+".pdf"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	var sf bytes.Buffer
	if err := Write(context.Background(), &sf, d, SF, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name+".sf"), sf.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestPDFLiteralMarkersParallelPagination(t *testing.T) {
	d := literalMarkerDocument()
	d.Layout = Layout{Width: 300, Height: 240, Margin: 24}
	item := &d.Blocks[0].Items[0]
	for i := 0; i < 28; i++ {
		item.Marker.Blocks = append(item.Marker.Blocks, mergeParagraph(fmt.Sprintf("LBL_%02d", i)))
		item.Blocks = append(item.Blocks, mergeParagraph(fmt.Sprintf("BODY_%02d", i)))
	}
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, PDF, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	got, err := ReadPDF(context.Background(), bytes.NewReader(out.Bytes()), ReadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Pages) < 3 || len(got.Blocks) != 1 || got.Blocks[0].Kind != ListBlock {
		t.Fatal("missing continued logical list")
	}
	for _, prefix := range []string{"LBL_", "BODY_"} {
		for i := 0; i < 28; i++ {
			token := fmt.Sprintf("%s%02d", prefix, i)
			if strings.Count(got.Text(), token) != 1 {
				t.Fatal("lost/duplicated", token)
			}
		}
	}

	labelPages := map[int]bool{}
	bodyPages := map[int]bool{}
	for _, pair := range []struct {
		blocks []Block
		label  bool
	}{{got.Blocks[0].Items[0].Marker.Blocks, true}, {got.Blocks[0].Items[0].Blocks, false}} {
		for _, b := range pair.blocks {
			for _, r := range b.Runs {
				if r.Source == nil || r.Source.Quad == nil {
					continue
				}
				x0, y0, x1, y1 := quadBounds(*r.Source.Quad)
				if x0 < 24 || x1 > 276 || y0 < 24 || y1 > 216 {
					t.Fatalf("escaped page: %+v", r.Source)
				}
				if strings.HasPrefix(r.Text, "LBL_") {
					if x1 > 87 {
						t.Fatal("label invaded body")
					}
					labelPages[r.Source.Page] = true
				}
				if strings.HasPrefix(r.Text, "BODY_") {
					if x0 < 93 {
						t.Fatal("body invaded label")
					}
					bodyPages[r.Source.Page] = true
				}
			}
		}
	}
	if len(labelPages) < 3 || len(bodyPages) < 3 {
		t.Fatal("independent columns did not continue")
	}
	exportLiteralOutput(t, "continued", out.Bytes(), got)
}

func TestPDFLiteralMarkersNestedAssetsAndStories(t *testing.T) {
	raster := image.NewRGBA(image.Rect(0, 0, 1, 1))
	raster.Set(0, 0, color.RGBA{R: 255, A: 255})
	var asset bytes.Buffer
	if err := png.Encode(&asset, raster); err != nil {
		t.Fatal(err)
	}
	label := []Block{mergeParagraph("RICH_LABEL"), {Kind: Picture, Image: &Image{Data: asset.Bytes(), Width: 16, Height: 16, Alt: "RED_MARKER"}}, {Kind: Table, Rows: []Row{{Cells: []Cell{{Blocks: []Block{mergeParagraph("INNER_LABEL")}}}}}}}
	list := Block{Kind: ListBlock, List: "bullet", Items: []ListItem{{Marker: &ListMarker{Blocks: label}, Blocks: []Block{mergeParagraph("RICH_BODY")}}}}
	d := &Document{Blocks: []Block{{Kind: Table, Rows: []Row{{Cells: []Cell{{Blocks: []Block{list}}}}}}}, Notes: []Note{{Kind: Footnote, ID: "n", Blocks: []Block{list}}}}
	d.Blocks = append(d.Blocks, Block{Kind: Paragraph, Runs: []Run{{Text: "CALL"}, {Note: &NoteReference{Kind: Footnote, ID: "n"}}}})
	before := canonicalSF(t, d)
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, PDF, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, canonicalSF(t, d)) {
		t.Fatal("nested caller mutated")
	}
	got, err := ReadPDF(context.Background(), bytes.NewReader(out.Bytes()), ReadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Blocks[0].Kind != Table {
		t.Fatal("outer table lost")
	}
	child := got.Blocks[0].Rows[0].Cells[0].Blocks[0]
	if child.Kind != ListBlock || child.Items[0].Marker == nil {
		t.Fatal("nested list lost")
	}
	marker := child.Items[0].Marker.Blocks
	if len(marker) != 3 || marker[1].Kind != Picture || marker[2].Kind != Table {
		t.Fatalf("marker rich structure lost: %+v", marker)
	}
	sameImagePixels(t, asset.Bytes(), marker[1].Image.Data)
	if marker[1].Image.Alt != "RED_MARKER" || len(got.Notes) != 1 {
		t.Fatal("alt or note lost")
	}
	noteList := got.Notes[0].Blocks[0]
	if noteList.Kind != ListBlock || noteList.Items[0].Marker == nil || noteList.Items[0].Marker.Blocks[1].Kind != Picture {
		t.Fatal("note marker lost")
	}
	exportLiteralOutput(t, "nested", out.Bytes(), got)
	// Running-story markers use the same immutable planner and artifact painting.

	// The other Span is not adjacent and has no Ref. It remains literal text.

	// First and repeated references may differ in provenance despite one identity.

	// Geometry/paint oracles inspect the physical view, independently of logical
	// note recovery. Expected page, glyph and asset values remain unchanged.

	d = &Document{Blocks: []Block{mergeParagraph("BODY_PAGE_ONE"), {Kind: PageBreak}, mergeParagraph("BODY_PAGE_TWO")}, Headers: []Block{list}}
	before = canonicalSF(t, d)
	out.Reset()
	if err := Write(context.Background(), &out, d, PDF, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, canonicalSF(t, d)) {
		t.Fatal("running caller mutated")
	}
	physical, err := ReadPDF(context.Background(), bytes.NewReader(out.Bytes()), ReadOptions{PDFSkipStructure: true, PDFSkipInference: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(physical.Pages) != 2 || strings.Count(physical.Text(), "RICH_LABEL") != 2 || strings.Count(physical.Text(), "INNER_LABEL") != 2 {
		t.Fatal("running marker missing")
	}
	exportLiteralOutput(t, "running", out.Bytes(), physical)
}

func TestPDFLiteralMarkerUnsupportedGlyphAtomic(t *testing.T) {
	d := literalMarkerDocument()
	d.Blocks[0].Items[0].Marker.Blocks[0].Runs[0].Text = "😀"
	var out bytes.Buffer
	err := Write(context.Background(), &out, d, PDF, WriteOptions{})
	if !errors.Is(err, ErrUnsupported) || out.Len() != 0 {
		t.Fatal("missing marker glyph did not reject atomically", err)
	}
}

func TestPDFLiteralNumericLabelsRemainEditableBlocks(t *testing.T) {
	for _, label := range []string{"1.", "•", ""} {
		item := ListItem{Marker: &ListMarker{Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: label, Bold: true}}}}}, Blocks: []Block{mergeParagraph("BODY")}}
		d := &Document{Blocks: []Block{{Kind: ListBlock, List: "number", Items: []ListItem{item}}}}
		var out bytes.Buffer
		if err := Write(context.Background(), &out, d, PDF, WriteOptions{}); err != nil {
			t.Fatal(err)
		}
		got, err := ReadPDF(context.Background(), &out, ReadOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if len(got.Blocks) != 1 || got.Blocks[0].Kind != ListBlock || got.Blocks[0].Items[0].Marker == nil {
			t.Fatal("literal block mistaken for automatic counter", label)
		}
		marker := got.Blocks[0].Items[0].Marker
		if paragraphText(marker.Blocks[0]) != label || label != "" && !marker.Blocks[0].Runs[0].Bold {
			t.Fatal("literal text/style changed", label)
		}
	}
}

func TestPDFLiteralMarkerNestedListAndRTLColumns(t *testing.T) {
	nested := Block{Kind: ListBlock, List: "number", Items: []ListItem{{Blocks: []Block{mergeParagraph("CHILD")}}}}
	marker := &ListMarker{Blocks: []Block{mergeParagraph("LABEL"), nested}}
	item := ListItem{Marker: marker, Blocks: []Block{{Kind: Paragraph, Direction: "rtl", Runs: []Run{{Text: "BODY"}}}}}
	d := &Document{Layout: Layout{Width: 300, Height: 240, Margin: 24}, Blocks: []Block{{Kind: ListBlock, List: "bullet", Items: []ListItem{item}}}}
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, PDF, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	got, err := ReadPDF(context.Background(), bytes.NewReader(out.Bytes()), ReadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	exportLiteralOutput(t, "narrow-rtl", out.Bytes(), got)
	label := got.Blocks[0].Items[0].Marker.Blocks
	if len(label) != 2 || label[1].Kind != ListBlock || paragraphText(label[1].Items[0].Blocks[0]) != "CHILD" {
		t.Fatalf("nested marker list lost: %s", canonicalSF(t, got))
	}
	labelX, _, _, _ := quadBounds(*label[0].Runs[0].Source.Quad)
	bodyX, _, _, _ := quadBounds(*got.Blocks[0].Items[0].Blocks[0].Runs[0].Source.Quad)
	if labelX <= bodyX || labelX < 219 {
		t.Fatal("RTL marker column not on right", labelX, bodyX)
	}
}
func nestedDefinitionObjects() []string {
	o := explicitNoteObjects("Note")
	child := len(o) + 1
	o[9] = strings.Replace(o[9], "/Ref [9 0 R 19 0 R]", "/Ref [9 0 R]", 1)
	o[9] = strings.Replace(o[9], "21 0 R]", fmt.Sprintf("21 0 R %d 0 R]", child), 1)
	o[18] = strings.Replace(o[18], "/K 7", fmt.Sprintf("/Ref [%d 0 R] /K 7", child), 1)
	content := strings.SplitN(strings.SplitN(o[4], "stream\n", 2)[1], "\nendstream", 2)[0]
	content = strings.Replace(content, "125 300 Tm (*1)", "125 300 Tm (*2)", 1)
	content += markedText(11, 20, 15, "*2") + markedText(12, 40, 15, "CHILD BODY")
	o[4] = recoveryStream("", content)
	o = append(o, fmt.Sprintf("<< /Type /StructElem /S /Note /P 10 0 R /Pg 3 0 R /ID (note-2) /Ref [19 0 R] /K [%d 0 R %d 0 R] >>", child+1, child+2), fmt.Sprintf("<< /Type /StructElem /S /Lbl /P %d 0 R /K 11 >>", child), fmt.Sprintf("<< /Type /StructElem /S /P /P %d 0 R /K 12 >>", child))
	return o
}
func redundantNoteWrapperObjects() []string {
	o := explicitNoteObjects("Note")
	wrapper := len(o) + 1
	o[7] = strings.Replace(o[7], "/K [0 9 0 R", fmt.Sprintf("/K [0 %d 0 R", wrapper), 1)
	o[8] = strings.Replace(o[8], "/P 8 0 R", fmt.Sprintf("/P %d 0 R", wrapper), 1)
	o = append(o, "<< /Type /StructElem /S /Span /P 8 0 R /Ref [10 0 R] /K 9 0 R >>")
	return o
}
func TestPDFNestedIndependentNoteDefinitions(t *testing.T) {
	d := readRecovery(t, recoveryPDF(nestedDefinitionObjects()...))
	refs := collectedNoteRefs(d)
	if len(d.Notes) != 2 || len(refs) != 2 {
		t.Fatal("independently referenced nested definition flattened", len(d.Notes), len(refs), d.Diagnostics)
	}
	if d.Notes[0].ID != "note-1" || d.Notes[1].ID != "note-2" || refs[0].Note.ID != "note-1" || refs[1].Note.ID != "note-2" {
		t.Fatal("logical order/relationships changed", d.Notes, refs)
	}
	if len(d.Notes[0].Blocks) != 4 || len(d.Notes[1].Blocks) != 1 || strings.Count(d.Text(), "CHILD BODY") != 1 || strings.Count(d.Text(), "NOTE BODY") != 1 {
		t.Fatal("nested bodies duplicated/dropped", d.Text())
	}
	if !hasDiagnostic(d, "pdf_note_nested_definition") {
		t.Fatal("flattened definition containment not diagnosed")
	}
	if d.Notes[0].Source.ParentElement != "(7 0 R)" || d.Notes[1].Source.ParentElement != d.Notes[0].Source.Element {
		t.Fatal("original containment lost or confused with a citation", d.Notes[0].Source, d.Notes[1].Source)
	}
	assertRecoverySF(t, d)
	before, _ := ToStructured(d)
	for _, format := range []Format{SF, SFGzip, JSON, DOCX} {
		var out bytes.Buffer
		if err := Write(context.Background(), &out, d, format, WriteOptions{AllowLossy: true}); err != nil {
			t.Fatal(format, err)
		}
		back, err := Read(context.Background(), &out, format, ReadOptions{})
		if err != nil {
			t.Fatal(format, err)
		}
		after, _ := ToStructured(back)
		if format != DOCX && !reflect.DeepEqual(before, after) {
			t.Fatal("source graph changed", format)
		}
		if len(back.Notes) != 2 || len(collectedNoteRefs(back)) != 2 || back.Notes[0].ID != "note-1" || back.Notes[1].ID != "note-2" {
			t.Fatal("note graph/order changed", format)
		}
	}
}
func TestPDFRedundantNoteWrapper(t *testing.T) {
	d := readRecovery(t, recoveryPDF(redundantNoteWrapperObjects()...))
	if len(d.Notes) != 1 || len(collectedNoteRefs(d)) != 2 || hasDiagnostic(d, "pdf_note_relationship") {
		t.Fatal("same-target wrapper introduced ambiguity or duplicated atom", d.Diagnostics)
	}
	edges := d.Blocks[0].Source.References
	found := false
	for _, edge := range edges {
		if edge.Element == "(22 0 R)" && len(edge.Targets) == 1 && edge.Targets[0] == "(10 0 R)" {
			found = true
		}
	}
	if !found {
		t.Fatal("redundant explicit Ref edge discarded", edges)
	}
	assertRecoverySF(t, d)
}

func TestPDFNoteWrapperBoundariesRetainRefEvidence(t *testing.T) {
	for _, opaque := range []bool{false, true} {
		o := redundantNoteWrapperObjects()
		if opaque {
			o[len(o)-1] = strings.Replace(o[len(o)-1], "/S /Span", "/S /Span /ActualText (DIFFERENTMARK)", 1)
		} else {
			o[7] = strings.Replace(o[7], "/K [0 22 0 R", "/K [22 0 R", 1)
			o[len(o)-1] = strings.Replace(o[len(o)-1], "/K 9 0 R", "/K [0 9 0 R]", 1)
		}
		d := readRecovery(t, recoveryPDF(o...))
		refs := collectedNoteRefs(d)
		expected := 2
		if opaque {
			expected = 1
		}
		if len(d.Notes) != 1 || len(refs) != expected || !hasDiagnostic(d, "pdf_note_relationship") {
			t.Fatal("wrapper widened into an invented citation", d.Notes, refs, d.Diagnostics)
		}
		if opaque && !strings.Contains(d.Text(), "DIFFERENTMARK") {
			t.Fatal("replacement text discarded")
		}
		edges := d.Blocks[0].Source.References
		for _, owner := range []string{"(9 0 R)", "(22 0 R)"} {
			found := false
			for _, edge := range edges {
				if edge.Element == owner && len(edge.Targets) == 1 && edge.Targets[0] == "(10 0 R)" {
					found = true
				}
			}
			if !found {
				t.Fatal("unrepresented original Ref owner lost during paragraph merge", owner, edges)
			}
		}
		assertRecoverySF(t, d)
	}
}
func TestPDFNestedNoteContainmentValidation(t *testing.T) {
	d := readRecovery(t, recoveryPDF(nestedDefinitionObjects()...))
	d.Notes[1].Source.ParentElement = d.Notes[1].Source.Element
	if err := d.Validate(); err == nil {
		t.Fatal("self-containing source element accepted")
	}
}

func TestPDFNoteWrapperChainsAndConflicts(t *testing.T) {
	t.Run("same target chain", func(t *testing.T) {
		o := redundantNoteWrapperObjects()
		inner := len(o) + 1
		o[8] = strings.Replace(o[8], "/P 22 0 R", fmt.Sprintf("/P %d 0 R", inner), 1)
		o[len(o)-1] = strings.Replace(o[len(o)-1], "/K 9 0 R", fmt.Sprintf("/K %d 0 R", inner), 1)
		o = append(o, "<< /Type /StructElem /S /Span /P 22 0 R /Ref [10 0 R] /K 9 0 R >>")
		d := readRecovery(t, recoveryPDF(o...))
		refs := collectedNoteRefs(d)
		if len(refs) != 2 || hasDiagnostic(d, "pdf_note_relationship") || len(refs[0].Source.References) != 3 || refs[0].Source.ParentElement != "(23 0 R)" {
			t.Fatal("chain aliases split or lost original owners", refs, d.Diagnostics)
		}
		for _, owner := range []string{"(9 0 R)", "(22 0 R)", "(23 0 R)"} {
			found := false
			for _, edge := range refs[0].Source.References {
				found = found || (edge.Element == owner && len(edge.Targets) == 1 && edge.Targets[0] == "(10 0 R)")
			}
			if !found {
				t.Fatal("lost original wrapper Ref", owner, refs[0].Source.References)
			}
		}
		assertRecoverySF(t, d)
	})
	t.Run("different target", func(t *testing.T) {
		o := nestedDefinitionObjects()
		wrapper := len(o) + 1
		o[7] = strings.Replace(o[7], "/K [0 9 0 R", fmt.Sprintf("/K [0 %d 0 R", wrapper), 1)
		o[8] = strings.Replace(o[8], "/P 8 0 R", fmt.Sprintf("/P %d 0 R", wrapper), 1)
		o = append(o, "<< /Type /StructElem /S /Span /P 8 0 R /Ref [22 0 R] /K 9 0 R >>")
		d := readRecovery(t, recoveryPDF(o...))
		refs := collectedNoteRefs(d)
		if len(refs) != 2 || len(d.Notes) != 2 || !hasDiagnostic(d, "pdf_note_relationship") || refs[0].Note.ID != "note-1" || refs[1].Note.ID != "note-2" {
			t.Fatal("conflicting wrapper selected instead of specific original sites", refs, d.Diagnostics)
		}
		found := false
		for _, edge := range d.Blocks[0].Source.References {
			found = found || (edge.Element == "(25 0 R)" && len(edge.Targets) == 1 && edge.Targets[0] == "(22 0 R)")
		}
		if !found {
			t.Fatal("conflicting original edge lost", d.Blocks[0].Source.References)
		}
		assertRecoverySF(t, d)
	})
}
func alignedActualNoteObjects(nested bool) []string {
	o := explicitNoteObjects("Note")
	o[7] = strings.Replace(o[7], "/S /P", "/S /P /ActualText (BODY*1MIDDLE*1AFTER)", 1)
	if nested {
		id := len(o) + 1
		o[7] = strings.Replace(o[7], "/K [0 9 0 R", fmt.Sprintf("/K [0 %d 0 R", id), 1)
		o[8] = strings.Replace(o[8], "/P 8 0 R", fmt.Sprintf("/P %d 0 R", id), 1)
		o = append(o, "<< /Type /StructElem /S /Span /P 8 0 R /ActualText (*1) /K 9 0 R >>")
	}
	return o
}

func TestPDFNoteAlignedEnclosingActualText(t *testing.T) {
	for _, nested := range []bool{false, true} {
		d := readRecovery(t, recoveryPDF(alignedActualNoteObjects(nested)...))
		refs := collectedNoteRefs(d)
		if len(d.Notes) != 1 || len(refs) != 2 || hasDiagnostic(d, "pdf_note_relationship") {
			t.Fatal("exact ActualText correspondence lost note graph", d.Notes, refs, d.Diagnostics)
		}
		if strings.Count(d.Text(), "NOTE BODY") != 1 || strings.Contains(d.Text(), "*1") || !strings.Contains(d.Text(), "BODY[endnote:note-1]MIDDLE[endnote:note-1]AFTER") {
			t.Fatal("ActualText characters/body duplicated or dropped", d.Text())
		}
		for _, ref := range refs {
			if ref.Note.Inferred || ref.Source.Structure != "explicit" || !strings.Contains(ref.Source.Evidence, "pdf_structure_actualtext_aligned") {
				t.Fatal("explicit Ref/ActualText provenance lost", ref)
			}
		}
		if len(d.Notes[0].Blocks) != 4 || d.Notes[0].Blocks[3].Image.Alt != "RED BLUE" {
			t.Fatal("rich body affected by enclosing citation replacement")
		}
		assertRecoverySF(t, d)
		before, _ := ToStructured(d)
		for _, format := range []Format{SF, SFGzip, JSON, DOCX} {
			var out bytes.Buffer
			if err := Write(context.Background(), &out, d, format, WriteOptions{AllowLossy: true}); err != nil {
				t.Fatal(format, err)
			}
			got, err := Read(context.Background(), &out, format, ReadOptions{})
			if err != nil {
				t.Fatal(format, err)
			}
			after, _ := ToStructured(got)
			if format != DOCX && !reflect.DeepEqual(before, after) {
				t.Fatal("exact source graph changed through", format)
			}
			if len(got.Notes) != 1 || len(collectedNoteRefs(got)) != 2 {
				t.Fatal("note graph changed through", format)
			}
		}
	}
}

func TestPDFNoteActualTextRequiresFullExactCorrespondence(t *testing.T) {
	for _, replacement := range []string{"BEFORE*1MIDDLE*1AFTER", "BODY*1MIDDLE*1AFTER*1", "BODY *1MIDDLE*1AFTER"} {
		o := alignedActualNoteObjects(false)
		o[7] = strings.Replace(o[7], "BODY*1MIDDLE*1AFTER", replacement, 1)
		d := readRecovery(t, recoveryPDF(o...))
		if len(d.Notes) != 0 || len(collectedNoteRefs(d)) != 0 || !hasDiagnostic(d, "pdf_note_relationship") || !strings.Contains(d.Text(), replacement) {
			t.Fatal("replacement substring was guessed into an editable citation", d)
		}
		assertRecoverySF(t, d)
	}
}

func TestPDFNoteAlignedActualTextRetainsFragmentStyles(t *testing.T) {
	o := alignedActualNoteObjects(true)
	content := strings.SplitN(strings.SplitN(o[4], "stream\n", 2)[1], "\nendstream", 2)[0]
	mixed := "q BT /F 10 Tf /P << /MCID 1 >> BDC 1 0 0 1 60 300 Tm (*) Tj /F 8 Tf 1 0 0 rg (1) Tj EMC ET Q\n"
	o[4] = recoveryStream("", strings.Replace(content, markedText(1, 60, 300, "*1"), mixed, 1))
	d := readRecovery(t, recoveryPDF(o...))
	refs := collectedNoteRefs(d)
	if len(refs) != 2 || len(refs[0].Note.LabelContent) != 2 {
		t.Fatal("aligned structural replacement flattened rich citation")
	}
	a, b := refs[0].Note.LabelContent[0], refs[0].Note.LabelContent[1]
	if a.Text != "*" || a.FontSize != 10 || a.Color != "#000000" || b.Text != "1" || b.FontSize != 8 || b.Color != "#FF0000" || a.Source.Quad == nil || b.Source.Quad == nil || hasDiagnostic(d, "pdf_actualtext_style") {
		t.Fatal("exact intervals lost individual paint/style/geometry", a, b, d.Diagnostics)
	}
	assertRecoverySF(t, d)
}

func TestPDFNoteActualTextContainingDefinitionStaysOpaque(t *testing.T) {
	o := explicitNoteObjects("Note")
	id := len(o) + 1
	o[6] = strings.Replace(o[6], "10 0 R]", fmt.Sprintf("%d 0 R]", id), 1)
	o[9] = strings.Replace(o[9], "/P 7 0 R", fmt.Sprintf("/P %d 0 R", id), 1)
	o[9] = strings.Replace(o[9], "/K [11 0 R 12 0 R 13 0 R 16 0 R 21 0 R]", "/K 3", 1)
	o = append(o, "<< /Type /StructElem /S /Span /P 7 0 R /ActualText (NOTE BODY) /K 10 0 R >>")
	d := readRecovery(t, recoveryPDF(o...))
	if len(d.Notes) != 0 || len(collectedNoteRefs(d)) != 0 || strings.Count(d.Text(), "NOTE BODY") != 1 || !hasDiagnostic(d, "pdf_note_relationship") {
		t.Fatal("definition moved outside its replacement range or duplicated", d.Text(), d.Diagnostics)
	}
	assertRecoverySF(t, d)
}
func adjacentNoteObjects() []string {
	o := explicitNoteObjects("Note")
	o[6] = strings.Replace(o[6], "/K [8 0 R 10 0 R]", "/K 8 0 R", 1)
	o[7] = strings.Replace(o[7], "/K [0 9 0 R 6 19 0 R 8]", "/K [0 9 0 R 10 0 R 6 19 0 R 8]", 1)
	o[8] = strings.Replace(o[8], "/Ref [10 0 R] ", "", 1)
	o[9] = strings.Replace(o[9], "/Ref [9 0 R 19 0 R] ", "", 1)
	o[9] = strings.Replace(o[9], "/P 7 0 R", "/P 8 0 R", 1)
	return o
}
func collectedNoteRefs(d *Document) []Run {
	var refs []Run
	_ = walkNoteRuns(d.Blocks, func(run Run) error {
		if run.Note != nil {
			refs = append(refs, run)
		}
		return nil
	})
	return refs
}
func TestPDFAdjacentNoteInferenceRichGraph(t *testing.T) {
	d := readRecovery(t, recoveryPDF(adjacentNoteObjects()...))
	refs := collectedNoteRefs(d)
	if len(d.Notes) != 1 || len(refs) != 1 || !refs[0].Note.Inferred {
		t.Fatal("published adjacent pattern flattened", d.Notes, refs, d.Diagnostics)
	}
	if refs[0].Source.Structure != "explicit" || refs[0].Source.Evidence != "pdf_note_adjacent_reference" || refs[0].Source.Confidence != .85 || len(refs[0].Source.References) != 0 {
		t.Fatal("inferred relationship confused with explicit source edges", refs[0])
	}
	note := d.Notes[0]
	if note.Kind != Endnote || !note.KindInferred || !hasDiagnostic(d, "pdf_note_relationship_inferred") || len(note.Source.References) != 0 || !hasDiagnostic(d, "pdf_note_kind_inferred") {
		t.Fatal("kind and relationship inference conflated", note, d.Diagnostics)
	}
	if len(note.Blocks) != 4 || note.Blocks[1].Kind != ListBlock || note.Blocks[2].Kind != Table || note.Blocks[3].Kind != Picture || note.Blocks[3].Image.Alt != "RED BLUE" || note.LabelSource == nil {
		t.Fatal("rich note body/source lost", note)
	}

	if strings.Count(d.Text(), "*1") != 1 || strings.Count(d.Text(), "NOTE BODY") != 1 {
		t.Fatal("inference consumed unrelated repeated label or duplicated body", d.Text())
	}
	assertRecoverySF(t, d)
	for _, format := range []Format{SF, SFGzip, JSON, DOCX} {
		var out bytes.Buffer
		if err := Write(context.Background(), &out, d, format, WriteOptions{AllowLossy: true}); err != nil {
			t.Fatal(format, err)
		}
		got, err := Read(context.Background(), &out, format, ReadOptions{})
		if err != nil {
			t.Fatal(format, err)
		}
		rs := collectedNoteRefs(got)
		if len(rs) != 1 || !rs[0].Note.Inferred || rs[0].Note.ID != note.ID {
			t.Fatal("relationship inference lost through", format, rs)
		}
	}
}
func TestPDFAdjacentNotesConservativeBoundaries(t *testing.T) {
	tests := []struct {
		name   string
		mutate func([]string)
	}{
		{"different labels", func(o []string) {
			content := strings.SplitN(strings.SplitN(o[4], "stream\n", 2)[1], "\nendstream", 2)[0]
			o[4] = recoveryStream("", strings.Replace(content, "20 80 Tm (*1)", "20 80 Tm (OTHER)", 1))
		}},
		{"intervening content", func(o []string) { o[7] = strings.Replace(o[7], "9 0 R 10 0 R 6", "9 0 R 6 10 0 R", 1) }},
		{"unresolved explicit ref", func(o []string) { o[8] = strings.Replace(o[8], "/S /Reference", "/S /Reference /Ref [999 0 R]", 1) }},
		{"body actualtext", func(o []string) {
			o[9] = strings.Replace(o[9], "/S /Note", "/S /Note /ActualText (NOTE REPLACEMENT)", 1)
		}},
		{"enclosing actualtext", func(o []string) { o[7] = strings.Replace(o[7], "/S /P", "/S /P /ActualText (WHOLE REPLACEMENT)", 1) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			objects := adjacentNoteObjects()
			test.mutate(objects)
			d := readRecovery(t, recoveryPDF(objects...))
			if len(d.Notes) != 0 || len(collectedNoteRefs(d)) != 0 {
				t.Fatal("unproven adjacent relationship invented", d.Notes, d.Diagnostics)
			}
			assertRecoverySF(t, d)
		})
	}
	explicit := readRecovery(t, recoveryPDF(explicitNoteObjects("Note")...))
	for _, run := range collectedNoteRefs(explicit) {
		if run.Note.Inferred {
			t.Fatal("explicit Ref marked inferred")
		}
	}
}
func TestDOCXNoteRelationshipInferenceSites(t *testing.T) {
	for _, variant := range []bool{false, true} {
		d := noteDocument()
		if variant {
			d = variantNoteDocument()
		}

		d.Blocks[0].Runs[1].Note.Inferred = true
		d.Blocks[2].Runs[1].Note.Inferred = true
		var out bytes.Buffer
		if err := Write(context.Background(), &out, d, DOCX, WriteOptions{AllowLossy: true}); err != nil {
			t.Fatal(err)
		}
		got, err := ReadDOCX(context.Background(), &out)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(collectedNoteRefs(d), collectedNoteRefs(got)) {
			t.Fatal("inference sites changed in Word", variant, collectedNoteRefs(got))
		}
	}
}

func linkedNoteObjects() []string {
	o := explicitNoteObjects("Note")
	o[8] = strings.Replace(o[8], "/S /Reference", "/S /Link", 1)
	o[8] = strings.Replace(o[8], "/Ref [10 0 R] ", "", 1)
	o[9] = strings.Replace(o[9], "/Ref [9 0 R 19 0 R] ", "", 1)
	o[8] = strings.Replace(o[8], "/K 1 >>", "/K [1 << /Type /OBJR /Obj 22 0 R /Pg 3 0 R >>] >>", 1)
	o[2] = strings.Replace(o[2], "/Contents 5 0 R", "/Annots [22 0 R] /Contents 5 0 R", 1)
	o = append(o, "<< /Type /Annot /Subtype /Link /Rect [58 296 75 312] /Dest [3 0 R /XYZ 20 80 null] >>")
	return o
}
func TestPDFPhysicalLinkNoteInference(t *testing.T) {
	d := readRecovery(t, recoveryPDF(linkedNoteObjects()...))
	refs := collectedNoteRefs(d)
	if len(d.Notes) != 1 || len(refs) != 1 || !refs[0].Note.Inferred || refs[0].Source.Evidence != "pdf_note_link_view" || refs[0].Source.Confidence != .95 || len(refs[0].Source.References) != 0 {
		t.Fatal("physical note link not identified separately", refs, d.Diagnostics)
	}
	if d.PDFNavigation == nil || len(d.PDFNavigation.Links) != 1 || d.PDFNavigation.Links[0].Destination == "" {
		t.Fatal("physical navigation disappeared")
	}
	assertRecoverySF(t, d)
	for _, test := range []struct{ name, from, to string }{
		{"far target", "/XYZ 20 80 null", "/XYZ 20 100 null"},
		{"partial view", "/XYZ 20 80 null", "/XYZ null 80 null"},
		{"wrong page region", "/XYZ 20 80 null", "/XYZ 150 80 null"},
	} {
		t.Run(test.name, func(t *testing.T) {
			o := linkedNoteObjects()
			o[len(o)-1] = strings.Replace(o[len(o)-1], test.from, test.to, 1)
			got := readRecovery(t, recoveryPDF(o...))
			if len(got.Notes) != 0 {
				t.Fatal("unproven Link target became note", got.Notes)
			}
			if !strings.Contains(got.Text(), "NOTE BODY") {
				t.Fatal("fallback lost visible content")
			}
		})
	}
	o := linkedNoteObjects()
	content := strings.SplitN(strings.SplitN(o[4], "stream\n", 2)[1], "\nendstream", 2)[0]
	o[4] = recoveryStream("", strings.Replace(content, "60 300 Tm (*1)", "60 300 Tm (OTHER)", 1))
	got := readRecovery(t, recoveryPDF(o...))
	if len(got.Notes) != 0 || !hasDiagnostic(got, "pdf_note_relationship_uncertain") {
		t.Fatal("mismatched text coerced into note")
	}
}

func TestPDFNoteLinkAmbiguityAndExplicitPriority(t *testing.T) {
	o := linkedNoteObjects()
	o[6] = strings.Replace(o[6], "/K [8 0 R 10 0 R]", "/K [8 0 R 10 0 R 23 0 R]", 1)
	content := strings.SplitN(strings.SplitN(o[4], "stream\n", 2)[1], "\nendstream", 2)[0]
	o[4] = recoveryStream("", content+markedText(11, 20, 80, "*1")+markedText(12, 40, 80, "SECOND NOTE"))
	o = append(o, "<< /Type /StructElem /S /Note /P 7 0 R /Pg 3 0 R /ID (second) /K [24 0 R 25 0 R] >>", "<< /Type /StructElem /S /Lbl /P 23 0 R /K 11 >>", "<< /Type /StructElem /S /P /P 23 0 R /K 12 >>")
	d := readRecovery(t, recoveryPDF(o...))
	if len(d.Notes) != 0 || !hasDiagnostic(d, "pdf_note_relationship_uncertain") || !strings.Contains(d.Text(), "SECOND NOTE") {
		t.Fatal("overlapping note regions guessed or content lost", d.Notes, d.Diagnostics)
	}
	explicit := linkedNoteObjects()
	explicit[8] = strings.Replace(explicit[8], "/S /Link", "/S /Link /Ref [10 0 R]", 1)
	d = readRecovery(t, recoveryPDF(explicit...))
	refs := collectedNoteRefs(d)
	if len(refs) != 1 || refs[0].Note.Inferred || refs[0].Source.Evidence != "pdf_note_explicit_ref" {
		t.Fatal("explicit structure edge lost priority", refs)
	}
}
func TestPDFTaggedNotePlacementKind(t *testing.T) {
	for _, tc := range []struct {
		name, role, line string
		skip             bool
		want             string
		inferred         bool
	}{
		{"separator", "Note", "20 100 m 120 100 l S\n", false, Footnote, true},
		{"no-separator", "Note", "", false, Endnote, true},
		{"short-line", "Note", "20 100 m 35 100 l S\n", false, Endnote, true},
		{"distant-line", "Note", "20 180 m 120 180 l S\n", false, Endnote, true},
		{"wrong-column", "Note", "150 100 m 250 100 l S\n", false, Endnote, true},
		{"skip", "Note", "20 100 m 120 100 l S\n", true, Endnote, true},
		{"explicit-endnote", "FENote /A << /O /FENote /NoteType /Endnote >>", "20 100 m 120 100 l S\n", false, Endnote, false},
		{"explicit-footnote", "FENote /A << /O /FENote /NoteType /Footnote >>", "", false, Footnote, false},
		{"none", "FENote /A << /O /FENote /NoteType /None >>", "20 100 m 120 100 l S\n", false, Footnote, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			objects := explicitNoteObjects(tc.role)
			content := strings.SplitN(strings.SplitN(objects[4], "stream\n", 2)[1], "\nendstream", 2)[0]
			objects[4] = recoveryStream("", tc.line+content)
			raw := recoveryPDF(objects...)
			d, err := ReadPDF(context.Background(), bytes.NewReader(raw), ReadOptions{PDFSkipInference: tc.skip})
			if err != nil {
				t.Fatal(err)
			}
			if len(d.Notes) != 1 || d.Notes[0].Kind != tc.want || d.Notes[0].KindInferred != tc.inferred {
				t.Fatalf("placement %+v", d.Notes)
			}
			if len(d.Notes[0].Blocks) != 4 || len(d.Blocks) != 1 {
				t.Fatal("rich ownership changed")
			}
			refs := 0
			for _, run := range d.Blocks[0].Runs {
				if run.Note != nil {
					refs++
					if run.Note.Kind != tc.want || run.Note.Inferred {
						t.Fatal("explicit edge changed", run.Note)
					}
				}
			}
			if refs != 2 {
				t.Fatal("reference inventory", refs)
			}
			baseline := readRecovery(t, raw)
			assertPDFNoteGlyphInventory(t, baseline, d)
			assertRecoverySF(t, d)
			for _, format := range []Format{SF, SFGzip, JSON, DOCX} {
				var buf bytes.Buffer
				if err := Write(context.Background(), &buf, d, format, WriteOptions{AllowLossy: true}); err != nil {
					t.Fatal(format, err)
				}
				back, err := Read(context.Background(), &buf, format, ReadOptions{})
				if err != nil {
					t.Fatal(format, err)
				}
				if len(back.Notes) != 1 || back.Notes[0].Kind != tc.want || back.Notes[0].KindInferred != tc.inferred {
					t.Fatal(format, "kind lost", back.Notes)
				}
				if format != DOCX && !reflect.DeepEqual(d.Notes, back.Notes) {
					t.Fatal(format, "note semantics changed")
				}
			}
			var rendered bytes.Buffer
			if err := Write(context.Background(), &rendered, d, PDF, WriteOptions{AllowLossy: true, PDFFontMap: map[string]string{"Courier": "Go Mono"}}); err != nil {
				t.Fatal("native placement", err)
			}
			if folder := os.Getenv("MANYDOC_PDF_NOTE_KIND_QA"); folder != "" && tc.name == "separator" {
				if err := os.MkdirAll(folder, 0755); err != nil {
					t.Fatal(err)
				}
				for name, data := range map[string][]byte{"source.pdf": raw, "native.pdf": rendered.Bytes()} {
					if err := os.WriteFile(filepath.Join(folder, name), data, 0600); err != nil {
						t.Fatal(err)
					}
				}
				for _, format := range []Format{SF, DOCX} {
					var output bytes.Buffer
					if err := Write(context.Background(), &output, d, format, WriteOptions{AllowLossy: true}); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(filepath.Join(folder, "model."+string(format)), output.Bytes(), 0600); err != nil {
						t.Fatal(err)
					}
				}
			}
			back := readRecovery(t, rendered.Bytes())
			if len(back.Notes) != 1 || back.Notes[0].Kind != tc.want || back.Notes[0].KindInferred {
				t.Fatal("rendered NoteType must declare the chosen reflow kind", back.Notes)
			}
		})
	}
}
func pageBreakDocument(t testing.TB) *Document {
	t.Helper()
	data, err := os.ReadFile("examples/page-breaks/document.sf")
	if err != nil {
		t.Fatal(err)
	}
	s, err := ReadStructured(context.Background(), bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	d, err := FromStructured(s)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func renderPageBreakDocument(t testing.TB, d *Document) (*Document, []byte) {
	t.Helper()
	before, _ := json.Marshal(d)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var out bytes.Buffer
	if err := Write(ctx, &out, d, PDF, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(d)
	if !bytes.Equal(before, after) {
		t.Fatal("break planning mutated source")
	}
	got, err := ReadPDF(ctx, bytes.NewReader(out.Bytes()), ReadOptions{PDFSkipInference: true, PDFSkipStructure: true})
	if err != nil {
		t.Fatal(err)
	}
	return got, out.Bytes()
}
func checkTokenPage(t testing.TB, d *Document, token string, want int) {
	t.Helper()
	found := 0
	for _, b := range d.Blocks {
		for _, run := range b.Runs {
			if run.Text == token {
				found++
				if run.Source.Page != want {
					t.Fatalf("%s on page %d, want %d", token, run.Source.Page, want)
				}
			}
		}
	}
	if found != 1 {
		t.Fatalf("%s occurrences=%d", token, found)
	}
}
func cellTable(blocks ...[]Block) Block {
	row := Row{}
	for _, flow := range blocks {
		row.Cells = append(row.Cells, Cell{Blocks: flow})
	}
	return Block{Kind: Table, Rows: []Row{row}}
}

func TestPDFParallelCellPageBreaks(t *testing.T) {
	for _, kind := range []string{"single", "simultaneous", "staggered"} {
		t.Run(kind, func(t *testing.T) {
			left := []Block{mergeParagraph("LEFT_BEFORE"), {Kind: PageBreak}, mergeParagraph("LEFT_AFTER")}
			right := []Block{mergeParagraph("RIGHT_ZERO"), mergeParagraph("RIGHT_ONE"), mergeParagraph("RIGHT_TWO")}
			wantPages := 2
			if kind == "simultaneous" {
				right = []Block{right[0], {Kind: PageBreak}, right[1], right[2]}
			}
			if kind == "staggered" {
				right = []Block{right[0], right[1], {Kind: PageBreak}, right[2]}
				wantPages = 3
			}
			d := &Document{Layout: Layout{Width: 480, Height: 600, Margin: 20}, Blocks: []Block{cellTable(left, right)}}
			got, _ := renderPageBreakDocument(t, d)
			if got.Metadata.PageCount != wantPages {
				t.Fatalf("pages=%d want%d", got.Metadata.PageCount, wantPages)
			}
			checkTokenPage(t, got, "LEFT_BEFORE", 1)
			checkTokenPage(t, got, "LEFT_AFTER", 2)
			checkTokenPage(t, got, "RIGHT_ZERO", 1)
			checkTokenPage(t, got, "RIGHT_ONE", 2)
			checkTokenPage(t, got, "RIGHT_TWO", wantPages)
		})
	}
}

func TestPDFCellLeadingConsecutiveTrailingBreaks(t *testing.T) {
	for depth := 0; depth < 4; depth++ {
		t.Run(fmt.Sprint(depth), func(t *testing.T) {
			b := cellTable([]Block{{Kind: PageBreak}, {Kind: PageBreak}, mergeParagraph("CELL_VISIBLE"), {Kind: PageBreak}})
			for n := 0; n < depth; n++ {
				b = cellTable([]Block{b})
			}
			d := &Document{Layout: Layout{Width: 480, Height: 600, Margin: 20}, Blocks: []Block{b}}
			got, _ := renderPageBreakDocument(t, d)
			if got.Metadata.PageCount != 4 {
				t.Fatalf("break page count depth%d got%d", depth, got.Metadata.PageCount)
			}
			checkTokenPage(t, got, "CELL_VISIBLE", 3)
		})
	}
}

func checkNoteTarget(t testing.TB, data []byte, wants ...int) {
	t.Helper()
	ctx := context.Background()
	p, err := pdfcpu.ReadWithContext(ctx, bytes.NewReader(data), &model.Configuration{Reader15: true, ValidationMode: model.ValidationStrict, Offline: true})
	if err != nil {
		t.Fatal(err)
	}
	if err = p.EnsurePageCount(); err != nil {
		t.Fatal(err)
	}
	first, _, _, err := p.PageDict(1, false)
	if err != nil {
		t.Fatal(err)
	}
	targets := []types.IndirectRef{}
	for _, want := range wants {
		_, ref, _, err := p.PageDict(want, false)
		if err != nil {
			t.Fatal(err)
		}
		targets = append(targets, *ref)
	}
	annots, err := p.DereferenceArray(first["Annots"])
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	for _, a := range annots {
		dict, err := p.DereferenceDict(a)
		if err != nil {
			t.Fatal(err)
		}
		if dict["Dest"] == nil {
			continue
		}
		dest, err := p.DereferenceArray(dict["Dest"])
		if err != nil {
			t.Fatal(err)
		}
		ref, ok := dest[0].(types.IndirectRef)
		if !ok || found >= len(targets) || ref != targets[found] {
			t.Fatalf("wrong note destination %v; wanted pages%v", dest, wants)
		}
		found++
	}
	if found != len(wants) {
		t.Fatal("missing note destination")
	}
}

func TestPDFFootnoteLeadingConsecutiveTrailingBreaks(t *testing.T) {
	for _, empty := range []bool{false, true} {
		for _, resume := range []bool{false, true} {
			t.Run(fmt.Sprintf("empty%t_resume%t", empty, resume), func(t *testing.T) {
				flow := []Block{{Kind: PageBreak}, {Kind: PageBreak}}
				wantPages := 3
				if !empty {
					flow = append(flow, mergeParagraph("NOTE_VISIBLE"), Block{Kind: PageBreak})
					wantPages = 4
				}
				d := &Document{Layout: Layout{Width: 480, Height: 600, Margin: 20}, Notes: []Note{{Kind: Footnote, ID: "n", Blocks: flow}}, Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "REFERENCE"}, noteRun(Footnote, "n")}}}}
				if resume {
					d.Blocks = append(d.Blocks, Block{Kind: PageBreak}, mergeParagraph("BODY_RESUMED"))
				}
				got, data := renderPageBreakDocument(t, d)
				if got.Metadata.PageCount != wantPages {
					t.Fatalf("footnote pages=%d want%d", got.Metadata.PageCount, wantPages)
				}
				if !empty {
					checkTokenPage(t, got, "NOTE_VISIBLE", 3)
				}
				if resume {
					checkTokenPage(t, got, "BODY_RESUMED", wantPages)
				}
				checkNoteTarget(t, data, 3)
			})
		}
	}
}

func TestPDFNestedCellAndNotePageBreaks(t *testing.T) {
	d := pageBreakDocument(t)
	got, _ := renderPageBreakDocument(t, d)
	if got.Metadata.PageCount != 5 {
		t.Fatalf("nested breaks: pages%d", got.Metadata.PageCount)
	}
	for token, page := range map[string]int{"OUTER_BEFORE": 1, "INNER_BEFORE": 1, "INNER_AFTER": 2, "OUTER_AFTER": 2, "INNER_SIDE_B": 2, "NOTE_BEFORE": 2, "NOTE_AFTER": 3, "NOTE_INNER_BEFORE": 3, "NOTE_INNER_AFTER": 4, "NOTE_END": 4, "BODY_AFTER": 5} {
		checkTokenPage(t, got, token, page)
	}
	images := 0
	for _, b := range got.Blocks {
		if b.Kind == Picture {
			images++
			if b.Source.Page != 2 && b.Source.Page != 4 {
				t.Fatal("break misplaced image")
			}
		}
	}
	if images != 2 {
		t.Fatal("break lost or repeated asset")
	}
}

func countBlockBreaks(bs []Block) int {
	n := 0
	for _, b := range bs {
		if b.Kind == PageBreak {
			n++
		}
		for _, row := range b.Rows {
			for _, c := range row.Cells {
				n += countBlockBreaks(c.Blocks)
			}
		}
		for _, item := range b.Items {
			n += countBlockBreaks(item.Blocks)
		}
	}
	return n
}
func TestPageBreakSFDOCXPreservesStoryPlacement(t *testing.T) {
	d := pageBreakDocument(t)
	for _, format := range []Format{SF, DOCX} {
		var out bytes.Buffer
		if err := Write(context.Background(), &out, d, format, WriteOptions{}); err != nil {
			t.Fatal(err)
		}
		got, err := Read(context.Background(), &out, format, ReadOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if countBlockBreaks(got.Blocks) != 2 || countBlockBreaks(got.Notes[0].Blocks) != 2 {
			t.Fatalf("lost break ownership in%s", format)
		}
		inner := got.Blocks[0].Rows[0].Cells[0].Blocks[1].Rows[0].Cells[0]
		if inner.RowSpan != 2 || inner.Blocks[1].Kind != PageBreak || inner.Blocks[2].Runs[1].Note.ID != "broken" {
			t.Fatal("cell break/note/span placement changed")
		}
	}
}

func TestPDFRichLeadingCellBreakPreservesContent(t *testing.T) {
	d := richCellDocument(t)
	d.Blocks[1].Rows[0].Cells[0].Blocks = append([]Block{{Kind: PageBreak}}, d.Blocks[1].Rows[0].Cells[0].Blocks...)
	got, _ := renderPageBreakDocument(t, d)
	for _, token := range []string{"CELL_START", "CELL_END", "INNER_A", "INNER_B", "INNER_C", "TOP_RIGHT", "BOTTOM_RIGHT", "AFTER_TABLE", "NOTE_CELL", "NOTE_NESTED"} {
		if strings.Count(strings.Join(strings.Fields(got.Text()), ""), token) != 1 {
			t.Fatalf("lost rich content after break: %s", token)
		}
	}
	pages := map[string]int{}
	for _, b := range got.Blocks {
		for _, run := range b.Runs {
			for _, token := range []string{"PREFACE", "CELL_START"} {
				if strings.Contains(run.Text, token) {
					pages[token] = run.Source.Page
				}
			}
		}
	}
	if pages["CELL_START"] <= pages["PREFACE"] {
		t.Fatal("leading cell break not honored")
	}
}

func tokenLocations(d *Document) map[string]pdfLocation {
	locations := map[string]pdfLocation{}
	for _, b := range d.Blocks {
		for _, run := range b.Runs {
			if run.Source != nil && run.Source.Quad != nil {
				x, y, _, _ := quadBounds(*run.Source.Quad)
				locations[run.Text] = pdfLocation{run.Source.Page, x, y}
			}
		}
	}
	return locations
}

func FuzzPDFCellPageBreaks(f *testing.F) {
	f.Add(byte(0), byte(2), byte(1), byte(2), byte(2), uint16(0))
	f.Add(byte(6), byte(6), byte(0), byte(0), byte(0), uint16(150))
	f.Fuzz(func(t *testing.T, lp, rp, lc, rc, depth byte, h uint16) {
		positions := []int{int(lp % 7), int(rp % 7)}
		counts := []int{1 + int(lc%3), 1 + int(rc%3)}
		flows := [][]Block{}
		for side, prefix := range []string{"LF_", "RX_"} {
			flow := []Block{}
			for i := 0; i <= 6; i++ {
				if i == positions[side] {
					for n := 0; n < counts[side]; n++ {
						flow = append(flow, Block{Kind: PageBreak})
					}
				}
				if i < 6 {
					flow = append(flow, mergeParagraph(fmt.Sprintf("%s%02d", prefix, i)))
				}
			}
			flows = append(flows, flow)
		}
		b := cellTable(flows...)
		for i := 0; i < int(depth%3); i++ {
			b = cellTable([]Block{b})
		}
		d := &Document{Layout: Layout{Width: 480, Height: 300 + float64(h%300), Margin: 20}, Blocks: []Block{b}}
		got, _ := renderPageBreakDocument(t, d)
		locations := tokenLocations(got)
		for side, prefix := range []string{"LF_", "RX_"} {
			var previous pdfLocation
			for i := 0; i < 6; i++ {
				token := fmt.Sprintf("%s%02d", prefix, i)
				loc, ok := locations[token]
				if !ok || strings.Count(got.Text(), token) != 1 {
					t.Fatalf("break lost/duplicated %s", token)
				}
				if loc.page < previous.page || loc.page == previous.page && loc.y <= previous.y {
					t.Fatal("break reordered cell")
				}
				if i == positions[side] && loc.page-previous.page < counts[side] {
					t.Fatalf("coalesced consecutive breaks in one cell: %v %v count%d", previous, loc, counts[side])
				}
				if positions[side] == 0 && i == 0 && loc.page < 1+counts[side] {
					t.Fatal("leading breaks lost")
				}
				if i >= min(positions[0], positions[1]) && loc.page < 2 {
					t.Fatal("sibling crossed first forced boundary")
				}
				previous = loc
			}
			if positions[side] == 6 && got.Metadata.PageCount < previous.page+counts[side] {
				t.Fatal("trailing breaks lost")
			}
		}
	})
}

func FuzzPDFFootnotePageBreaks(f *testing.F) {
	f.Add(byte(0), byte(2), byte(0), byte(0), true)
	f.Add(byte(2), byte(0), byte(2), byte(2), false)
	f.Fuzz(func(t *testing.T, leading, position, consecutive, trailing byte, resume bool) {
		lead, pos, count, trail := int(leading%3), int(position%7), 1+int(consecutive%3), int(trailing%3)
		flow := []Block{}
		for n := 0; n < lead; n++ {
			flow = append(flow, Block{Kind: PageBreak})
		}
		for i := 0; i <= 6; i++ {
			if i == pos {
				for n := 0; n < count; n++ {
					flow = append(flow, Block{Kind: PageBreak})
				}
			}
			if i < 6 {
				flow = append(flow, mergeParagraph(fmt.Sprintf("FN_%02d", i)))
			}
		}
		for n := 0; n < trail; n++ {
			flow = append(flow, Block{Kind: PageBreak})
		}
		d := &Document{Layout: Layout{Width: 480, Height: 600, Margin: 20}, Notes: []Note{{Kind: Footnote, ID: "n", Blocks: flow}}, Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "REF"}, noteRun(Footnote, "n")}}}}
		if resume {
			d.Blocks = append(d.Blocks, Block{Kind: PageBreak}, mergeParagraph("RESUMED"))
		}
		got, data := renderPageBreakDocument(t, d)
		wantPages := 1 + lead + count + trail
		// A final break already creates the fresh page on which body resumes.

		// Independently authored xref/object-stream syntax. Object identities, offsets,
		// compressed indexes and expected text are fixed source oracles, not writer output.

		// Hand-authored object streams are a structural oracle independent of the writer.

		// These primitive extraction oracles inspect paint order and exact runs;
		// geometric structure has separate default-mode integration tests.

		// Published AFM widths in glyph-space units: Helvetica space 278,
		// hyphen 333, bullet 350; Courier uses 600 for each. Alias slots in
		// WinAnsi must resolve to these glyphs, not a missing-metric fallback.

		// Two spaces, two hyphens and one bullet.

		// Undefined Unicode codes reject, regardless of available width defaults.

		// The first encoded glyph maps to two Unicode runes; Tc is applied once.

		// Embedded EI and EOI bytes in a JPEG comment must not terminate the image.

		// Every branch invokes the same child repeatedly: the source stays linear while
		// actual Form execution is exponential. These are hand-authored PDF objects,
		// independent of the production writer and below the existing depth32 guard.

		if resume && trail == 0 && pos < 6 {
			wantPages++
		}
		if got.Metadata.PageCount != wantPages {
			t.Fatalf("footnote page breaks got%d want%d", got.Metadata.PageCount, wantPages)
		}
		for i := 0; i < 6; i++ {
			want := 1 + lead
			if i >= pos {
				want += count
			}
			checkTokenPage(t, got, fmt.Sprintf("FN_%02d", i), want)
		}
		target := 1 + lead
		if pos == 0 {
			target += count
		}
		checkNoteTarget(t, data, target)
		if resume {
			checkTokenPage(t, got, "RESUMED", wantPages)
		}
	})
}

func TestPDFEmptyBreakNoteNeedsNoSeparatorHeight(t *testing.T) {
	d := &Document{Layout: Layout{Width: 200, Height: 60, Margin: 20}, Notes: []Note{{Kind: Footnote, ID: "n", Blocks: []Block{{Kind: PageBreak}, {Kind: PageBreak}}}}, Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "REF"}, noteRun(Footnote, "n")}}}}
	got, data := renderPageBreakDocument(t, d)
	if got.Metadata.PageCount != 3 {
		t.Fatalf("empty note break pages%d", got.Metadata.PageCount)
	}
	checkNoteTarget(t, data, 3)
}

func TestPDFLeadingBreakNoteAlongsideVisibleNote(t *testing.T) {
	d := &Document{Layout: Layout{Width: 480, Height: 240, Margin: 20}, Notes: []Note{{Kind: Footnote, ID: "moved", Blocks: []Block{{Kind: PageBreak}, mergeParagraph("NOTE_MOVED")}}, {Kind: Footnote, ID: "same", Blocks: []Block{mergeParagraph("NOTE_STAYS")}}}, Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "REF"}, noteRun(Footnote, "moved"), noteRun(Footnote, "same")}}}}
	got, data := renderPageBreakDocument(t, d)
	if got.Metadata.PageCount != 2 {
		t.Fatalf("mixed note pages%d", got.Metadata.PageCount)
	}
	checkTokenPage(t, got, "NOTE_MOVED", 2)
	checkTokenPage(t, got, "NOTE_STAYS", 1)
	checkNoteTarget(t, data, 2, 1)
}
func paragraphText(b Block) string {
	var out strings.Builder
	for _, r := range b.Runs {
		out.WriteString(r.Text)
	}
	return out.String()
}

func TestPDFTaggedImpliedParagraphAndBlockBarriers(t *testing.T) {
	content := markedText(0, 20, 350, "owner")
	content += markedText(1, 50, 350, "-")
	content += markedText(2, 20, 330, "ship") + markedText(3, 20, 300, "Explicit")
	content += markedText(4, 20, 280, "Wrapped") + markedText(5, 20, 260, "last")
	content += markedText(6, 20, 240, "Separate") + markedText(7, 20, 220, "tail")
	d := readRecovery(t, taggedTestPDF(content, "<< /Type /StructTreeRoot /K 8 0 R >>",
		"<< /Type /StructElem /S /LBody /P 7 0 R /Pg 3 0 R /K [0 9 0 R 2 10 0 R 11 0 R 5 13 0 R 7] >>",
		"<< /Type /StructElem /S /Span /P 8 0 R /ActualText <FEFF00AD> /K 1 >>",
		"<< /Type /StructElem /S /P /P 8 0 R /K 3 >>",
		"<< /Type /StructElem /S /Span /P 8 0 R /K 12 0 R >>",
		"<< /Type /StructElem /S /P /P 11 0 R /K 4 >>",
		"<< /Type /StructElem /S /Div /P 8 0 R /K 6 >>"))
	want := []string{"owner\u00adship", "Explicit", "Wrapped", "last", "Separate", "tail"}
	if len(d.Blocks) != len(want) {
		t.Fatalf("block boundaries: %+v", d.Blocks)
	}
	for i, b := range d.Blocks {
		if b.Kind != Paragraph || paragraphText(b) != want[i] {
			t.Fatalf("paragraph %d: %+v", i, b)
		}
		structure := "inferred"
		if i == 1 || i == 2 {
			structure = "explicit"
		}
		if b.Source == nil || b.Source.Structure != structure || b.Source.Quad == nil {
			t.Fatalf("paragraph %d provenance: %+v", i, b.Source)
		}
	}
	if len(d.Blocks[0].Runs) != 3 || d.Blocks[0].Runs[1].Source.Role != "Span" || d.Blocks[0].Runs[0].Source.MCID == nil || *d.Blocks[0].Runs[0].Source.MCID != 0 {
		t.Fatal("inline owners or original runs lost")
	}
	assertRecoverySF(t, d)
}

func TestPDFTaggedCellImpliedAndExplicitParagraphs(t *testing.T) {
	content := markedText(0, 20, 350, "First") + markedText(1, 20, 330, "line")
	content += markedText(2, 20, 310, "Second") + markedText(3, 20, 290, "Third")
	d := readRecovery(t, taggedTestPDF(content, "<< /Type /StructTreeRoot /K 8 0 R >>",
		"<< /Type /StructElem /S /Table /P 7 0 R /Pg 3 0 R /K 9 0 R >>",
		"<< /Type /StructElem /S /TR /P 8 0 R /K 10 0 R >>",
		"<< /Type /StructElem /S /TD /P 9 0 R /K [0 1 11 0 R 12 0 R] >>",
		"<< /Type /StructElem /S /P /P 10 0 R /K 2 >>",
		"<< /Type /StructElem /S /P /P 10 0 R /K 3 >>"))
	bs := d.Blocks[0].Rows[0].Cells[0].Blocks
	if len(bs) != 3 || paragraphText(bs[0]) != "First line" || paragraphText(bs[1]) != "Second" || paragraphText(bs[2]) != "Third" || bs[0].Source.Structure != "inferred" || bs[1].Source.Structure != "explicit" {
		t.Fatalf("cell paragraph boundaries: %+v", bs)
	}
	if bs[0].Runs[1].Text != " " || bs[0].Runs[1].Source.Precision != "synthetic_separator" {
		t.Fatal("inferred inter-line word boundary not disclosed")
	}
	assertRecoverySF(t, d)
}

func TestPDFCustomListLabelKeepsExplicitBodyParagraphs(t *testing.T) {
	content := markedText(0, 20, 350, "iv.") + markedText(1, 50, 350, "First") + markedText(2, 50, 330, "Second")
	d := readRecovery(t, taggedTestPDF(content, "<< /Type /StructTreeRoot /K 8 0 R >>",
		"<< /Type /StructElem /S /L /P 7 0 R /Pg 3 0 R /K 9 0 R >>",
		"<< /Type /StructElem /S /LI /P 8 0 R /K [10 0 R 11 0 R] >>",
		"<< /Type /StructElem /S /Lbl /P 9 0 R /K 0 >>",
		"<< /Type /StructElem /S /LBody /P 9 0 R /K [12 0 R 13 0 R] >>",
		"<< /Type /StructElem /S /P /P 11 0 R /K 1 >>",
		"<< /Type /StructElem /S /P /P 11 0 R /K 2 >>"))
	if len(d.Blocks) != 1 || d.Blocks[0].Kind != ListBlock || len(d.Blocks[0].Items) != 1 {
		t.Fatal("explicit custom list hierarchy lost")
	}
	item := d.Blocks[0].Items[0]
	if item.Marker == nil || paragraphText(item.Marker.Blocks[0]) != "iv." || len(item.Blocks) != 2 || paragraphText(item.Blocks[0]) != "First" || paragraphText(item.Blocks[1]) != "Second" || item.Blocks[0].Source.Structure != "explicit" || item.Blocks[1].Source.Structure != "explicit" || !hasDiagnostic(d, "pdf_list_literal") || !hasDiagnostic(d, "pdf_list_kind_unobserved") {
		t.Fatal("original marker/body/paragraph/source boundaries changed")
	}
	if d.Text() != "iv. First\nSecond" {
		t.Fatal("original literal text oracle changed", d.Text())
	}
	assertRecoverySF(t, d)
}

func TestPDFTaggedInlineReplacementFormattingAndLink(t *testing.T) {
	objects := taggedTestObjects(markedText(0, 20, 350, "XY")+markedText(1, 20, 330, "linked"), "<< /Type /StructTreeRoot /K 8 0 R >>",
		"<< /Type /StructElem /S /LBody /P 7 0 R /Pg 3 0 R /K [9 0 R 10 0 R] >>",
		"<< /Type /StructElem /S /Span /P 8 0 R /ActualText <FEFF00660069> /K 0 >>",
		"<< /Type /StructElem /S /Link /P 8 0 R /K [1 << /Type /OBJR /Obj 11 0 R >>] >>",
		"<< /Type /Annot /Subtype /Link /Rect [20 325 56 340] /A << /S /URI /URI (https://example.invalid/inline) >> >>")
	objects[2] = strings.Replace(objects[2], "/Contents 5 0 R", "/Annots [11 0 R] /Contents 5 0 R", 1)
	objects[3] = strings.Replace(objects[3], "/Courier", "/Courier-BoldOblique", 1)
	d := readRecovery(t, recoveryPDF(objects...))
	if len(d.Blocks) != 1 || paragraphText(d.Blocks[0]) != "fi linked" || d.Blocks[0].Source.Structure != "inferred" {
		t.Fatalf("inline paragraph: %+v", d.Blocks)
	}
	rs := d.Blocks[0].Runs
	if len(rs) != 3 || !rs[0].Bold || !rs[0].Italic || rs[0].Source.FontSize != 10 || rs[0].Source.Baseline == nil || !rs[2].Bold || !rs[2].Italic || rs[2].Link != "https://example.invalid/inline" || rs[2].Source.Role != "Link" {
		t.Fatalf("inline formatting/relationship/provenance: %+v", rs)
	}
	assertRecoverySF(t, d)
	objects[7] = strings.Replace(objects[7], "/K [9 0 R 10 0 R]", "/K 9 0 R", 1)
	objects[8] = strings.Replace(objects[8], "/K 0", "/K [0 1]", 1)
	objects[2] = strings.Replace(objects[2], "/F 4 0 R", "/F 4 0 R /N 12 0 R", 1)
	objects = append(objects, "<< /Type /Font /Subtype /Type1 /BaseFont /Courier >>")
	objects[4] = recoveryStream("", markedText(0, 20, 350, "X")+strings.Replace(markedText(1, 26, 350, "Y"), "/F 10", "/N 10", 1))
	d = readRecovery(t, recoveryPDF(objects...))
	if d.Text() != "fi" || len(d.Blocks) != 1 || len(d.Blocks[0].Runs) != 1 || d.Blocks[0].Runs[0].Bold || d.Blocks[0].Runs[0].Italic || d.Blocks[0].Runs[0].Source.Font != "" || !hasDiagnostic(d, "pdf_actualtext_style") {
		t.Fatalf("mixed replacement ambiguity: %+v", d.Blocks)
	}
	assertRecoverySF(t, d)
}

type parserStreamFixture struct {
	objectCodec, xrefCodec             string
	header                             string
	count                              int
	badObjectChecksum, badXRefChecksum bool
}

func parserStreamPDF(t *testing.T, options parserStreamFixture) []byte {
	t.Helper()
	catalog := "<< /Type /Catalog /Pages 2 0 R >>"
	font := "<< /Type /Font /Subtype /Type1 /BaseFont /Courier >>"
	header := fmt.Sprintf("1 0 4 %d ", len(catalog)+1)
	if options.header != "" {
		header = options.header
	}
	count := 2
	if options.count != 0 {
		count = options.count
	}
	encode := func(data []byte, codec string, bad bool) ([]byte, string) {
		switch codec {
		case "", "raw":
			return data, ""
		case "flate":
			b := syntaxZlib(t, data)
			if bad {
				b[len(b)-1] ^= 1
			}
			return b, "/Filter /FlateDecode"
		case "png", "tiff":
			samples := append([]byte(nil), data...)
			for i := len(samples) - 1; i > 0; i-- {
				samples[i] -= samples[i-1]
			}
			predictor := 2
			if codec == "png" {
				samples = append([]byte{1}, samples...)
				predictor = 15
			}
			return syntaxZlib(t, samples), fmt.Sprintf("/Filter /FlateDecode /DecodeParms << /Predictor %d /Colors 1 /BitsPerComponent 8 /Columns %d >>", predictor, len(data))
		case "ascii85":
			return syntax85(data), "/Filter /ASCII85Decode"
		case "lzw0":
			return syntaxLZW(t, data, false), "/Filter /LZWDecode /DecodeParms << /EarlyChange 0 >>"
		case "lzw1":
			return syntaxLZW(t, data, true), "/Filter /LZWDecode"
		default:
			t.Fatalf("unknown codec %s", codec)
			return nil, ""
		}
	}
	var out bytes.Buffer
	out.WriteString("%PDF-1.7\n")
	offsets := map[int]int{}
	add := func(id int, body string) {
		offsets[id] = out.Len()
		fmt.Fprintf(&out, "%d 0 obj\n%s\nendobj\n", id, body)
	}
	add(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>")
	add(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 400] /Resources << /Font << /F 4 0 R >> >> /Contents 5 0 R >>")
	add(5, recoveryStream("", "BT /F 10 Tf 20 300 Td (COMPRESSED) Tj ET"))
	body, filters := encode([]byte(header+catalog+" "+font), options.objectCodec, options.badObjectChecksum)
	add(6, recoveryStream(fmt.Sprintf("/Type /ObjStm /N %d /First %d %s", count, len(header), filters), string(body)))
	offsets[7] = out.Len()
	var entries bytes.Buffer
	for id := 0; id < 8; id++ {
		typ := byte(1)
		a, b := uint32(offsets[id]), uint16(0)
		switch id {
		case 0:
			typ = 0
			a = 0
			b = 65535
		case 1:
			typ = 2
			a = 6
			b = 0
		case 4:
			typ = 2
			a = 6
			b = 1
		}
		entries.WriteByte(typ)
		_ = binary.Write(&entries, binary.BigEndian, a)
		_ = binary.Write(&entries, binary.BigEndian, b)
	}
	data, filters := encode(entries.Bytes(), options.xrefCodec, options.badXRefChecksum)
	add(7, recoveryStream("/Type /XRef /Size 8 /Root 1 0 R /W [1 4 2] "+filters, string(data)))
	fmt.Fprintf(&out, "startxref\n%d\n%%%%EOF\n", offsets[7])
	return out.Bytes()
}
func parserExport(t *testing.T, name string, source []byte) {
	t.Helper()
	if dir := os.Getenv("MANYDOC_PDF_PARSER_QA"); dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name+".pdf"), source, 0644); err != nil {
			t.Fatal(err)
		}
	}
}
func TestPDFParserStreamSources(t *testing.T) {
	var reference *Document
	for _, codec := range []string{"raw", "flate", "ascii85", "lzw0", "lzw1", "png", "tiff"} {
		t.Run(codec, func(t *testing.T) {
			source := parserStreamPDF(t, parserStreamFixture{objectCodec: codec, xrefCodec: codec})
			parserExport(t, codec, source)
			d := readRecovery(t, source)
			if len(d.Blocks) != 1 || d.Blocks[0].Kind != Paragraph || len(d.Blocks[0].Runs) != 1 || d.Blocks[0].Runs[0].Text != "COMPRESSED" {
				t.Fatalf("source semantics: %+v", d.Blocks)
			}
			closePoint(t, d.Blocks[0].Runs[0].Source.Baseline[0], 20, 100)
			assertRecoverySF(t, d)
			if reference == nil {
				reference = d
			} else if !reflect.DeepEqual(reference, d) {
				t.Fatal("codec changed full source document")
			}
		})
	}
}
func TestPDFParserStreamInvalidSources(t *testing.T) {
	for _, tc := range []struct {
		name    string
		options parserStreamFixture
	}{
		{"object-checksum", parserStreamFixture{objectCodec: "flate", badObjectChecksum: true}},
		{"xref-checksum", parserStreamFixture{xrefCodec: "flate", badXRefChecksum: true}},
		{"wrong-header-owner", parserStreamFixture{header: "9 0 4 33 "}},
		{"wrong-count", parserStreamFixture{count: 1}},
		{"count-too-large", parserStreamFixture{count: 3}},
		{"duplicate-member", parserStreamFixture{header: "1 0 1 34 "}},
		{"vertical-tab", parserStreamFixture{header: "1\v0 4 34 "}},
		{"zero-member", parserStreamFixture{header: "0 0 4 34 "}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := parserStreamPDF(t, tc.options)
			parserExport(t, "reject-"+tc.name, source)
			d, err := ReadPDF(context.Background(), bytes.NewReader(source), ReadOptions{})
			if err == nil || d != nil {
				t.Fatalf("accepted invalid initial parser stream: %v", d)
			}
		})
	}
}

func TestPDFObjectStreamHeaderValidation(t *testing.T) {
	for _, tc := range []struct {
		name, header string
		n            int
		owners       map[int]int
		valid        bool
	}{
		{"ordered offsets, unsorted identities", "4 0 1 3 ", 2, map[int]int{0: 4, 1: 1}, true},
		{"PDF whitespace and comments", "4\x000\t% note\r\n1 3 ", 2, map[int]int{0: 4, 1: 1}, true},
		{"duplicates", "4 0 4 3 ", 2, nil, false},
		{"descending offsets", "4 3 1 0 ", 2, nil, false},
		{"negative offset", "4 -1 1 3 ", 2, nil, false},
		{"outside body", "4 0 1 10 ", 2, nil, false},
		{"missing pair", "4 0 ", 2, nil, false},
		{"extra pair", "4 0 1 3 ", 1, nil, false},
		{"owner mismatch", "4 0 1 3 ", 2, map[int]int{1: 5}, false},
		{"huge N", "4 0 ", int(^uint(0) >> 1), nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := &pdfReader{ctx: context.Background()}
			data := []byte(tc.header + "10 20")
			sd := types.StreamDict{Dict: types.Dict{"N": types.Integer(tc.n), "First": types.Integer(len(tc.header))}}
			err := r.validateObjectStreamHeader(6, &sd, data, tc.owners)
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v error=%v", tc.valid, err)
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r := &pdfReader{ctx: ctx}
	pos := 0
	if err := r.skipHeaderSpace([]byte("%"+strings.Repeat("a", 16384)), &pos); err != context.Canceled {
		t.Fatalf("cancel: %v", err)
	}
}

func TestPDFParserIncrementalSource(t *testing.T) {
	source := parserStreamPDF(t, parserStreamFixture{objectCodec: "flate", xrefCodec: "flate"})
	parts := strings.Split(string(source), "startxref\n")
	previous, err := strconv.Atoi(strings.Fields(parts[len(parts)-1])[0])
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	out.Write(source)
	fontOffset := out.Len()
	out.WriteString("4 0 obj\n<< /Type /Font /Subtype /Type1 /BaseFont /Courier-Bold >>\nendobj\n")
	xref := out.Len()
	fmt.Fprintf(&out, "xref\n4 1\n%010d 00000 n \ntrailer\n<< /Size 8 /Root 1 0 R /Prev %d >>\nstartxref\n%d\n%%%%EOF\n", fontOffset, previous, xref)
	d := readRecovery(t, out.Bytes())
	if d.Blocks[0].Runs[0].Text != "COMPRESSED" || !d.Blocks[0].Runs[0].Bold {
		t.Fatal("incremental replacement lost", d.Blocks)
	}
	assertRecoverySF(t, d)
	parserExport(t, "incremental", out.Bytes())
}

func TestPDFParserHeaderWhitespaceSources(t *testing.T) {
	for _, tc := range []struct{ name, header string }{
		{"comment", "1 0 4 34 % retained header comment\n"},
		{"nul", "1\x000\x004\x0034\x00"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := parserStreamPDF(t, parserStreamFixture{header: tc.header})
			d := readRecovery(t, source)
			if d.Text() != "COMPRESSED" {
				t.Fatalf("body %q", d.Text())
			}
			assertRecoverySF(t, d)
			parserExport(t, tc.name, source)
		})
	}
}

func FuzzPDFObjectStreamHeader(f *testing.F) {
	f.Add([]byte("1 0 4 3 abcdef"), 2, 8)
	f.Add([]byte("4 0 % header\n42"), 1, 13)
	f.Add([]byte("4 0 "), int(^uint(0)>>1), 4)
	f.Fuzz(func(t *testing.T, data []byte, n, first int) {
		if len(data) > 4096 {
			t.Skip()
		}
		r := &pdfReader{ctx: context.Background()}
		sd := types.StreamDict{Dict: types.Dict{"N": types.Integer(n), "First": types.Integer(first)}}
		_ = r.validateObjectStreamHeader(6, &sd, data, map[int]int{0: 1, 1: 4})
	})
}

func recoveryPDF(objects ...string) []byte {
	var out bytes.Buffer
	out.WriteString("%PDF-1.7\n")
	offsets := []int{0}
	for i, o := range objects {
		offsets = append(offsets, out.Len())
		fmt.Fprintf(&out, "%d 0 obj\n%s\nendobj\n", i+1, o)
	}
	xref := out.Len()
	fmt.Fprintf(&out, "xref\n0 %d\n0000000000 65535 f \n", len(offsets))
	for _, offset := range offsets[1:] {
		fmt.Fprintf(&out, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&out, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offsets), xref)
	return out.Bytes()
}
func recoveryStream(dict, content string) string {
	return fmt.Sprintf("<< %s /Length %d >>\nstream\n%s\nendstream", dict, len(content), content)
}
func recoveryPage(content, attrs string, extra ...string) []byte {
	objects := []string{"<< /Type /Catalog /Pages 2 0 R >>", "<< /Type /Pages /Kids [3 0 R] /Count 1 >>", "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 400] /Resources << /Font << /F 4 0 R >> /XObject << /X 6 0 R /Y 7 0 R >> >> /Contents 5 0 R " + attrs + " >>", "<< /Type /Font /Subtype /Type1 /BaseFont /Courier >>", recoveryStream("", content)}
	return recoveryPDF(append(objects, extra...)...)
}
func readRecovery(t *testing.T, data []byte) *Document {
	t.Helper()

	d, e := ReadPDF(context.Background(), bytes.NewReader(data), ReadOptions{PDFSkipInference: true})
	if e != nil {
		t.Fatal(e)
	}
	return d
}
func closePoint(t *testing.T, actual Point, x, y float64) {
	t.Helper()
	if math.Abs(actual.X-x) > 1e-7 || math.Abs(actual.Y-y) > 1e-7 {
		t.Fatalf("point %+v, want (%g,%g)", actual, x, y)
	}
}
func recoveryRuns(d *Document) []Run {
	var runs []Run
	for _, b := range d.Blocks {
		runs = append(runs, b.Runs...)
	}
	return runs
}
func assertRecoverySF(t *testing.T, d *Document) {
	t.Helper()
	sf, e := ToStructured(d)
	if e != nil {
		t.Fatal(e)
	}
	var out bytes.Buffer
	if e = WriteStructured(context.Background(), &out, sf); e != nil {
		t.Fatal(e)
	}
	sf, e = ReadStructured(context.Background(), &out)
	if e != nil {
		t.Fatal(e)
	}
	got, e := FromStructured(sf)
	if e != nil {
		t.Fatal(e)
	}
	a, _ := json.Marshal(d)
	b, _ := json.Marshal(got)
	if !bytes.Equal(a, b) {
		t.Fatalf("SF changed recovery semantics\n%s\n%s", a, b)
	}
}

func TestPDFPositionedTextTransforms(t *testing.T) {
	d := readRecovery(t, recoveryPage("q 2 0 0 3 10 20 cm BT /F 10 Tf 50 Tz 2 Ts 1 0 0 1 5 7 Tm (AB) Tj (C) Tj ET Q", ""))
	runs := recoveryRuns(d)
	if len(runs) != 2 || runs[0].Text != "AB" || runs[1].Text != "C" {
		t.Fatalf("runs: %+v", runs)
	}
	a, b := runs[0].Source, runs[1].Source
	closePoint(t, a.Baseline[0], 20, 353)
	closePoint(t, a.Baseline[1], 32, 353)
	closePoint(t, b.Baseline[0], 32, 353)
	closePoint(t, b.Baseline[1], 38, 353)
	if *a.Matrix != (Matrix{10, 0, 0, -30, 20, 353}) || a.FontSize != 30 || a.Precision != "font_metrics" || a.Structure != "unstructured" || d.Blocks[0].Source.Structure != "inferred" {
		t.Fatalf("source %+v", a)
	}
	if len(d.Pages) != 1 || d.Pages[0].Width != 300 || d.Pages[0].Height != 400 {
		t.Fatalf("pages %+v", d.Pages)
	}
	assertRecoverySF(t, d)
}

func TestPDFCoreFontCanonicalAliasAdvances(t *testing.T) {

	for _, tc := range []struct {
		name string
		want float64
	}{{"Helvetica", 15.72}, {"Helvetica-Bold", 15.72}, {"Courier", 30}} {
		t.Run(tc.name, func(t *testing.T) {
			objects := inferenceObjects("BT /F 10 Tf 20 300 Td <20A02DAD95> Tj ET")
			objects[3] = "<< /Type /Font /Subtype /Type1 /BaseFont /" + tc.name + " /Encoding /WinAnsiEncoding >>"
			d := readRecovery(t, recoveryPDF(objects...))
			run := d.Blocks[0].Runs[0]

			closePoint(t, run.Source.Baseline[1], 20+tc.want, 500)
			if run.Source.Precision != "font_metrics" {
				t.Fatal("exact core metrics not used")
			}
		})
	}
}
func TestPDFPageCoordinateSystems(t *testing.T) {
	tests := []struct {
		rotate     int
		start, end Point
		w, h       float64
	}{{0, Point{20, 540}, Point{32, 540}, 400, 600}, {90, Point{60, 20}, Point{60, 32}, 600, 400}, {180, Point{380, 60}, Point{368, 60}, 400, 600}, {270, Point{540, 380}, Point{540, 368}, 600, 400}}
	for _, tc := range tests {
		t.Run(fmt.Sprint(tc.rotate), func(t *testing.T) {
			d := readRecovery(t, recoveryPage("BT /F 10 Tf 1 0 0 1 30 50 Tm (A) Tj ET", fmt.Sprintf("/CropBox [20 20 220 320] /Rotate %d /UserUnit 2", tc.rotate)))
			s := recoveryRuns(d)[0].Source
			closePoint(t, s.Baseline[0], tc.start.X, tc.start.Y)
			closePoint(t, s.Baseline[1], tc.end.X, tc.end.Y)
			if d.Pages[0].Width != tc.w || d.Pages[0].Height != tc.h {
				t.Fatalf("page %+v", d.Pages[0])
			}
			assertRecoverySF(t, d)
		})
	}
}
func TestPDFTextMatrixStateAndQuotes(t *testing.T) {
	d := readRecovery(t, recoveryPage("BT /F 10 Tf 10 100 Td 12 TL (A) Tj q 2 0 0 2 0 0 cm (B) Tj Q (C) Tj (D) ' 0 0 (E) \" ET", ""))
	rr := recoveryRuns(d)
	var positions = map[string]Point{"A": {10, 300}, "B": {32, 200}, "C": {22, 300}, "D": {10, 312}, "E": {10, 324}}
	for _, r := range rr {
		want, ok := positions[strings.TrimSpace(r.Text)]
		if !ok {
			t.Fatalf("unexpected run %q", r.Text)
		}
		closePoint(t, r.Source.Baseline[0], want.X, want.Y)
		delete(positions, strings.TrimSpace(r.Text))
	}
	if len(positions) > 0 {
		t.Fatalf("missing %v", positions)
	}
}
func TestPDFFormAndAppearancePlacement(t *testing.T) {
	form := recoveryStream("/Type /XObject /Subtype /Form /BBox [0 0 20 20] /Matrix [2 0 0 2 3 4]", "BT /F 10 Tf 1 0 0 1 5 6 Tm (X) Tj ET")
	d := readRecovery(t, recoveryPage("q 1 0 0 1 10 20 cm /X Do Q", "", form))
	closePoint(t, recoveryRuns(d)[0].Source.Baseline[0], 23, 364)
	d = readRecovery(t, recoveryPage("", "/Annots [7 0 R]", form, "<< /Type /Annot /Subtype /Widget /Rect [100 200 200 300] /AP << /N 6 0 R >> >>"))
	closePoint(t, recoveryRuns(d)[0].Source.Baseline[0], 125, 170)
	assertRecoverySF(t, d)
}
func TestPDFCompositeMetricsAndLigatureSpacing(t *testing.T) {
	cmap := "begincmap 1 begincodespacerange <0000> <ffff> endcodespacerange 2 beginbfchar <0001> <00660069> <0002> <0058> endbfchar endcmap"
	objects := []string{"<< /Type /Catalog /Pages 2 0 R >>", "<< /Type /Pages /Kids [3 0 R] /Count 1 >>", "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 400] /Resources << /Font << /F 4 0 R >> >> /Contents 5 0 R >>", "<< /Type /Font /Subtype /Type0 /BaseFont /Demo /Encoding /Identity-H /DescendantFonts [6 0 R] /ToUnicode 7 0 R >>", recoveryStream("", "BT /F 10 Tf 2 Tc 1 0 0 1 10 100 Tm <00010002> Tj <0003> Tj ET"), "<< /Type /Font /Subtype /CIDFontType2 /BaseFont /Demo /CIDSystemInfo << /Registry (Adobe) /Ordering (Identity) /Supplement 0 >> /DW 800 /W [1 [400] 2 2 600] /FontDescriptor 8 0 R >>", recoveryStream("", cmap), "<< /Type /FontDescriptor /FontName /Demo /Ascent 900 /Descent -100 >>"}

	if _, e := ReadPDF(context.Background(), bytes.NewReader(recoveryPDF(objects...)), ReadOptions{}); e == nil {
		t.Fatal("undefined Unicode code accepted")
	}
	objects[4] = recoveryStream("", "BT /F 10 Tf 2 Tc 1 0 0 1 10 100 Tm <00010002> Tj <0001> Tj ET")
	d := readRecovery(t, recoveryPDF(objects...))
	rr := recoveryRuns(d)
	if rr[0].Text != "fiX" {
		t.Fatalf("ligature %q", rr[0].Text)
	}
	closePoint(t, rr[0].Source.Baseline[1], 24, 300)
	closePoint(t, rr[1].Source.Baseline[0], 24, 300)

	assertRecoverySF(t, d)
}
func TestPDFMarkedContentAndActualText(t *testing.T) {
	d := readRecovery(t, recoveryPage("BT /F 10 Tf 10 100 Td /P << /MCID 5 >> BDC /Span << /ActualText <FEFF00660069> >> BDC (X) Tj (Y) Tj EMC EMC ET", ""))
	rr := recoveryRuns(d)
	if len(rr) != 1 || rr[0].Text != "fi" || rr[0].Source.MCID == nil || *rr[0].Source.MCID != 5 || rr[0].Source.Tag != "Span" || rr[0].Source.Evidence != "pdf_actualtext" || len(rr[0].Source.Regions) != 2 {
		t.Fatalf("ActualText %+v", rr)
	}
	closePoint(t, rr[0].Source.Regions[1].Quad[0], 16, 291.95)
	assertRecoverySF(t, d)
	for _, content := range []string{"EMC", "/P BMC", "q", "Q", "/P << /MCID -1 >> BDC EMC"} {
		if _, e := ReadPDF(context.Background(), bytes.NewReader(recoveryPage(content, "")), ReadOptions{}); e == nil {
			t.Fatalf("accepted unbalanced/invalid %q", content)
		}
	}
}
func TestPDFRasterAssetsAndMasks(t *testing.T) {
	raw := string([]byte{255, 0, 0, 0, 255, 0})
	imageObject := recoveryStream("/Type /XObject /Subtype /Image /Width 2 /Height 1 /BitsPerComponent 8 /ColorSpace /DeviceRGB /SMask 7 0 R", raw)
	mask := recoveryStream("/Type /XObject /Subtype /Image /Width 2 /Height 1 /BitsPerComponent 8 /ColorSpace /DeviceGray", string([]byte{255, 0}))
	d := readRecovery(t, recoveryPage("q 40 0 0 20 10 30 cm /X Do Q", "", imageObject, mask))
	b := d.Blocks[0]
	if b.Kind != Picture || b.Image.Width != 40 || b.Image.Height != 20 {
		t.Fatalf("picture %+v", b)
	}
	im, _, e := image.Decode(bytes.NewReader(b.Image.Data))
	if e != nil {
		t.Fatal(e)
	}
	if got := color.NRGBAModel.Convert(im.At(0, 0)); got != (color.NRGBA{255, 0, 0, 255}) {
		t.Fatalf("first pixel %v", got)
	}
	_, _, _, a := im.At(1, 0).RGBA()
	if a != 0 {
		t.Fatalf("mask alpha %d", a)
	}
	closePoint(t, b.Source.Quad[0], 10, 350)
	closePoint(t, b.Source.Quad[2], 50, 370)
	assertRecoverySF(t, d)
	stencil := recoveryStream("/Type /XObject /Subtype /Image /Width 2 /Height 1 /ImageMask true /BitsPerComponent 1", string([]byte{0x40}))
	d = readRecovery(t, recoveryPage("0 0 1 rg q 40 0 0 20 10 30 cm /X Do Q", "", stencil))
	im, _, e = image.Decode(bytes.NewReader(d.Blocks[0].Image.Data))
	if e != nil {
		t.Fatal(e)
	}
	if got := color.NRGBAModel.Convert(im.At(0, 0)); got != (color.NRGBA{0, 0, 255, 255}) {
		t.Fatalf("stencil %v", got)
	}
	_, _, _, a = im.At(1, 0).RGBA()
	if a != 0 {
		t.Fatal("unpainted stencil pixel opaque")
	}
}
func TestPDFSourceValidationAndLossGate(t *testing.T) {
	d := readRecovery(t, recoveryPage("BT /F 10 Tf 10 100 Td (A) Tj ET", ""))
	d.Diagnostics = nil
	for _, f := range []Format{PDF, DOCX} {
		if e := Write(context.Background(), &bytes.Buffer{}, d, f, WriteOptions{}); !errors.Is(e, ErrLossy) {
			t.Fatalf("source provenance bypassed loss acknowledgment: %v", e)
		}
	}
	original := d.Pages[0]
	d.Pages[0].Width++
	if e := d.Validate(); e == nil {
		t.Fatal("inconsistent page geometry accepted")
	}
	d.Pages[0] = original
	sf, e := ToStructured(d)
	if e != nil {
		t.Fatal(e)
	}
	s := sf.Content[0].Content[0].Marks[0].Attrs
	var attrs map[string]any
	_ = json.Unmarshal(s, &attrs)
	attrs["unexpected"] = 1
	sf.Content[0].Content[0].Marks[0].Attrs, _ = json.Marshal(attrs)
	if _, e = FromStructured(sf); e == nil {
		t.Fatal("unknown provenance field accepted")
	}
	if reflect.DeepEqual(s, nil) {
		t.Fatal("source mark missing")
	}
}

func TestPDFInlineImageBoundaries(t *testing.T) {
	raw := []byte{'E', 'I', ' ', 0, 255, 1}
	var compressed bytes.Buffer
	z := zlib.NewWriter(&compressed)
	_, _ = z.Write(raw)
	_ = z.Close()
	a85 := make([]byte, ascii85.MaxEncodedLen(len(raw)))
	n85 := ascii85.Encode(a85, raw)
	a85 = append(a85[:n85], []byte("~>")...)
	source := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	source.SetNRGBA(0, 0, color.NRGBA{255, 0, 0, 255})
	source.SetNRGBA(1, 0, color.NRGBA{0, 0, 255, 255})
	var jpg bytes.Buffer
	if e := jpeg.Encode(&jpg, source, nil); e != nil {
		t.Fatal(e)
	}

	jpegData := append([]byte{}, jpg.Bytes()[:2]...)
	jpegData = append(jpegData, 255, 254, 0, 9, ' ', 'E', 'I', ' ', 255, 217, 0)
	jpegData = append(jpegData, jpg.Bytes()[2:]...)
	for _, tc := range []struct {
		name, filter string
		data         []byte
	}{{"raw", "", raw}, {"flate", "/F /Fl", compressed.Bytes()}, {"hex", "/F /AHx", []byte(hex.EncodeToString(raw) + ">")}, {"ascii85", "/F /A85", a85}, {"rle", "/F /RL", append(append([]byte{5}, raw...), 128)}, {"jpeg", "/F /DCT", jpegData}} {
		t.Run(tc.name, func(t *testing.T) {
			content := "q 40 0 0 20 10 30 cm BI /W 2 /H 1 /BPC 8 /CS /RGB " + tc.filter + " ID " + string(tc.data) + " EI Q BT /F 10 Tf 10 100 Td (AFTER) Tj ET"
			d := readRecovery(t, recoveryPage(content, ""))
			if len(d.Blocks) != 2 || d.Blocks[0].Kind != Picture || d.Blocks[1].Runs[0].Text != "AFTER" {
				t.Fatalf("inline recovery %+v", d.Blocks)
			}
			if tc.name != "jpeg" {
				im, _, e := image.Decode(bytes.NewReader(d.Blocks[0].Image.Data))
				if e != nil {
					t.Fatal(e)
				}
				if got := color.NRGBAModel.Convert(im.At(0, 0)); got != (color.NRGBA{'E', 'I', ' ', 255}) {
					t.Fatalf("raw EI pixel changed: %v", got)
				}
			}
			assertRecoverySF(t, d)
		})
	}
	for _, content := range []string{"BI /W 2 /H 1 /BPC 8 /CS /RGB ID abc EI", "BI /W 1 /H 1 /BPC 8 /CS /G /F /RL ID \x00\x80 EI", "BI /W 1 /H 1 /BPC 8 /CS /G ID Z EX"} {
		if _, e := ReadPDF(context.Background(), bytes.NewReader(recoveryPage(content, "")), ReadOptions{}); e == nil {
			t.Fatal("malformed inline image accepted")
		}
	}
}

func TestPDFActualTextStringEncoding(t *testing.T) {
	for _, v := range []any{types.StringLiteral(`\200\223\240\030`), types.HexLiteral("8093a018")} {
		got, e := pdfTextString(v)
		if e != nil || got != "•ﬁ€˘" {
			t.Fatalf("PDFDocEncoding %q %v", got, e)
		}
	}
	got, e := pdfTextString(types.HexLiteral("5c323030"))
	if e != nil || got != `\200` {
		t.Fatalf("hex bytes unescaped a second time: %q %v", got, e)
	}
	for _, v := range []any{types.HexLiteral("feffd800"), types.HexLiteral("7f"), types.HexLiteral("efbbbf80")} {
		if _, e = pdfTextString(v); e == nil {
			t.Fatal("invalid text string accepted")
		}
	}
}

func pdfXObjectDAG(levels, fanout, pages int, painted, appearances bool) []byte {
	font, content, first := 3+pages, 4+pages, 5+pages
	kids := make([]string, pages)
	for i := range kids {
		kids[i] = fmt.Sprintf("%d 0 R", 3+i)
	}
	objects := []string{"<< /Type /Catalog /Pages 2 0 R >>", fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), pages)}
	for i := 0; i < pages; i++ {
		attrs := ""
		if appearances {
			refs := make([]string, 6)
			for j := range refs {
				refs[j] = fmt.Sprintf("%d 0 R", first+levels+1+j)
			}
			attrs = "/Annots [" + strings.Join(refs, " ") + "]"
		}
		objects = append(objects, fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 400] /Resources << /Font << /F %d 0 R >> /XObject << /X %d 0 R >> >> /Contents %d 0 R %s >>", font, first, content, attrs))
	}
	objects = append(objects, "<< /Type /Font /Subtype /Type1 /BaseFont /Courier >>")
	stream := "BT /F 10 Tf 1 0 0 1 0 350 Tm (KEEP) Tj ET"
	if !appearances {
		stream += " /X Do"
	}
	objects = append(objects, recoveryStream("", stream))
	for i := 0; i <= levels; i++ {
		body := ""
		resources := ""
		if i < levels {
			resources = fmt.Sprintf("/Resources << /XObject << /Next %d 0 R >> /Font << /F %d 0 R >> >>", first+i+1, font)
			step := 1
			for j := i + 1; j < levels; j++ {
				step *= fanout
			}
			for branch := 0; branch < fanout; branch++ {
				shift := 0
				if painted {
					shift = 10 * branch * step
				}
				body += fmt.Sprintf("q 1 0 0 1 %d 0 cm /Next Do Q ", shift)
			}
		} else if painted {
			body = "BT /F 10 Tf 1 0 0 1 0 300 Tm (LEAF) Tj ET"
		}
		objects = append(objects, recoveryStream("/Type /XObject /Subtype /Form /BBox [0 0 300 400] "+resources, body))
	}
	if appearances {
		for i := 0; i < 6; i++ {
			objects = append(objects, fmt.Sprintf("<< /Type /Annot /Subtype /Widget /Rect [0 0 300 400] /AP << /N %d 0 R >> >>", first))
		}
	}
	return recoveryPDF(objects...)
}

func TestPDFXObjectExpansionBudget(t *testing.T) {
	for _, tc := range []struct {
		name          string
		levels, pages int
		appearances   bool
	}{
		{"acyclic-shared-form", 6, 1, false},
		{"shared-across-pages", 5, 6, false},
		{"shared-appearances", 5, 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := pdfXObjectDAG(tc.levels, 8, tc.pages, false, tc.appearances)
			if dir := os.Getenv("MANYDOC_PDF_XOBJECT_QA"); dir != "" {
				if err := os.MkdirAll(dir, 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, tc.name+".pdf"), data, 0600); err != nil {
					t.Fatal(err)
				}
			}
			if len(data) > 6000 {
				t.Fatal("fixture materialized the expanded graph", len(data))
			}
			d, err := ReadPDF(context.Background(), bytes.NewReader(data), ReadOptions{})
			if err == nil || d != nil || !strings.Contains(err.Error(), "XObject expansion exceeds") {
				t.Fatalf("compact repeated Form graph was not bounded: doc=%v err=%v", d != nil, err)
			}
		})
	}
}

func TestPDFXObjectRepeatedPaintPreservesPlacements(t *testing.T) {
	d := readRecovery(t, pdfXObjectDAG(3, 2, 1, true, false))
	runs := recoveryRuns(d)
	if len(runs) != 9 || runs[0].Text != "KEEP" {
		t.Fatal("missing actual Form occurrences", len(runs), d.Text())
	}
	stream := ""
	for i, run := range runs[1:] {
		if run.Text != "LEAF" || run.Source == nil || run.Source.Page != 1 || run.Source.Stream == "" || run.Source.Structure != "unstructured" {
			t.Fatal("Form paint/source identity changed", run)
		}
		if i == 0 {
			stream = run.Source.Stream
		}
		if run.Source.Stream != stream {
			t.Fatal("shared source was replaced by duplicated definitions")
		}
		closePoint(t, run.Source.Baseline[0], float64(i*10), 100)
		closePoint(t, run.Source.Baseline[1], float64(i*10+24), 100)
	}
	assertRecoverySF(t, d)
}

func FuzzPDFXObjectGraph(f *testing.F) {
	for _, s := range [][3]uint8{{6, 7, 0}, {5, 7, 5}, {3, 1, 0}, {0, 0, 1}} {
		f.Add(s[0], s[1], s[2])
	}
	f.Fuzz(func(t *testing.T, depth, branches, count uint8) {
		levels, fanout, pages := int(depth)%7, int(branches)%8+1, int(count)%6+1
		visits, atLevel := 0, 1
		for i := 0; i <= levels; i++ {
			visits += atLevel
			atLevel *= fanout
		}
		data := pdfXObjectDAG(levels, fanout, pages, false, false)
		d, err := ReadPDF(context.Background(), bytes.NewReader(data), ReadOptions{PDFSkipInference: true})
		if visits*pages > maxPDFXObjectVisits {
			if err == nil || d != nil || !strings.Contains(err.Error(), "XObject expansion exceeds") {
				t.Fatalf("expanded graph did not reject: %d visits, %v", visits*pages, err)
			}
			return
		}
		if err != nil {
			t.Fatal(err)
		}
		if len(d.Pages) != pages || strings.Count(d.Text(), "KEEP") != pages {
			t.Fatal("per-page source content lost", d.Text())
		}
		for _, run := range recoveryRuns(d) {
			if run.Text != "KEEP" || run.Source == nil || run.Source.Page < 1 || run.Source.Page > pages {
				t.Fatal("source identity lost", run)
			}
		}
		assertRecoverySF(t, d)
	})
}
