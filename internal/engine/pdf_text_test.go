package engine

import (
	"bytes"
	"compress/zlib"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/artpar/manifold/internal/bidilevels"
	"github.com/benoitkugler/textlayout/language"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
	"github.com/phpdave11/gofpdf"
	"golang.org/x/image/font/gofont/goregular"
	"math"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"
)

func TestPDFOriginalBidiRangesAndP1(t *testing.T) {
	r := textTestRenderer()
	text := "A\u030A office\r\n\nB\u2028C\u2029D"
	atoms, err := r.textAtoms([]Run{{Text: text}}, 20)
	if err != nil {
		t.Fatal(err)
	}
	position := 0
	var first *pdfBidiParagraph
	for _, a := range atoms {
		for _, s := range a.spans {
			if s.bidi.start != position || s.bidi.end != position+originalScalarLength(s.text) {
				t.Fatalf("original range %+v for %q", s.bidi, s.text)
			}
			position = s.bidi.end
			if s.text == "A\u030A" {
				first = s.bidi.paragraph
				if s.bidi.end-s.bidi.start != 2 || len(s.glyphs.positions) != 1 {
					t.Fatal("NFC erased original range")
				}
			}
		}
	}
	if position != originalScalarLength(text) || first == nil {
		t.Fatal("original coverage")
	}
	p, err := pdfBidiParagraphs(context.Background(), text, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(p) != 4 || p[0].end != 11 || p[1].resolved.Len() != 0 || p[2].resolved.Len() != 3 {
		t.Fatalf("P1 boundaries %+v", p)
	}
	if _, err := p[0].sourceRange(0, p[0].end+1); err == nil {
		t.Fatal("cross-paragraph grapheme accepted")
	}
	lines, err := r.lines([]Run{{Text: text}}, 100, 20)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range lines {
		for i, s := range line {
			if i > 0 && s.bidi.paragraph != line[0].bidi.paragraph {
				t.Fatal("line mixed paragraph contexts")
			}
		}
	}
}
func TestPDFBidiLineL1BeforeCoalescing(t *testing.T) {
	p, err := pdfBidiParagraphs(context.Background(), "אב x  y ז", "rtl")
	if err != nil {
		t.Fatal(err)
	}
	var line pdfLine
	for i, ch := range []rune("אב x  y ז") {
		br, err := p[0].sourceRange(i, i+1)
		if err != nil {
			t.Fatal(err)
		}
		line = append(line, pdfSpan{text: string(ch), bidi: br})
	}
	before := line[3].bidi.level
	if before != 2 {
		t.Fatal("Latin glyph must have even embedded level")
	}
	if err := applyPDFLineLevels(line[3:6]); err != nil {
		t.Fatal(err)
	}
	if line[3].bidi.level != 2 || line[4].bidi.level != 1 || line[5].bidi.level != 1 {
		t.Fatalf("L1 %+v", line)
	}
	// CRLF is one grapheme and one P1 boundary; its own scalar range survives.
	p, err = pdfBidiParagraphs(context.Background(), "A\r\nB", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(p) != 2 || p[0].end != 3 || p[1].start != 3 {
		t.Fatal("CRLF P1 split")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := pdfBidiParagraphs(ctx, "abc", ""); err != context.Canceled {
		t.Fatal(err)
	}
}
func TestPDFDirectionalGlyphNativeOracle(t *testing.T) {
	data, err := os.ReadFile("testdata/rtl-shaping.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		FontSHA256 string `json:"font_sha256"`
		UPEM       float64
		Cases      []struct {
			Text, Script, Language string
			Level                  uint8
			Advance                float64
			Glyphs                 []struct {
				GID           uint16
				Cluster       int
				X, Y, Advance float64
			}
		}
	}
	if err = json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	program, err := os.ReadFile("testdata/assets/DejaVuSans.ttf")
	if err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256(program)
	if hex.EncodeToString(h[:]) != fixture.FontSHA256 {
		t.Fatal("oracle font program changed")
	}
	r := textTestRenderer()
	if err := r.addFonts([]FontFace{{Family: "DejaVu", Data: program}}); err != nil {
		t.Fatal(err)
	}
	face, err := r.resolveFont(Run{FontFamily: "DejaVu"})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range fixture.Cases {
		t.Run(tc.Text, func(t *testing.T) {
			script, err := language.ParseScript(strings.ToLower(tc.Script))
			if err != nil {
				t.Fatal(err)
			}
			c, err := r.shapeGlyphTextLevel(face, tc.Text, Run{Language: tc.Language}, 65536, script, tc.Level)
			if err != nil {
				t.Fatal(err)
			}
			if len(c.positions) != len(tc.Glyphs) || math.Abs(c.width-tc.Advance/fixture.UPEM) > 1e-9 || c.rtl != (tc.Level%2 != 0) {
				t.Fatalf("native glyph count/advance/direction: got %+v width %v want %+v width %v", c.positions, c.width, tc.Glyphs, tc.Advance/fixture.UPEM)
			}
			for i, g := range c.positions {
				want := tc.Glyphs[i]
				if g.glyph != want.GID || g.cluster != want.Cluster || math.Abs(g.x-want.X/fixture.UPEM) > 1e-9 || math.Abs(g.y-want.Y/fixture.UPEM) > 1e-9 || math.Abs(g.advance-want.Advance/fixture.UPEM) > 1e-9 {
					t.Fatalf("glyph %d %+v want %+v", i, g, want)
				}
			}
			positions := append([]pdfGlyphPosition(nil), c.positions...)
			sort.SliceStable(positions, func(i, j int) bool { return positions[i].cluster < positions[j].cluster })
			var logical strings.Builder
			end := 0
			for _, g := range positions {
				if g.logical != "" {
					if g.textStart != end || g.textEnd < g.textStart || tc.Text[g.textStart:g.textEnd] != g.logical {
						t.Fatal("original logical cluster interval")
					}
					logical.WriteString(g.logical)
					end = g.textEnd
				}
			}
			if logical.String() != tc.Text || end != len(tc.Text) {
				t.Fatalf("logical clusters %q want %q", logical.String(), tc.Text)
			}
		})
	}
	if len(fixture.Cases) != 11 {
		t.Fatal("native oracle row count")
	}
}
func TestPDFRTLPartialLigaturePaint(t *testing.T) {
	program, err := os.ReadFile("testdata/assets/DejaVuSans.ttf")
	if err != nil {
		t.Fatal(err)
	}
	r := textTestRenderer()
	if err := r.addFonts([]FontFace{{Family: "DejaVu", Data: program}}); err != nil {
		t.Fatal(err)
	}
	face, err := r.resolveFont(Run{FontFamily: "DejaVu"})
	if err != nil {
		t.Fatal(err)
	}
	c, err := r.shapeGlyphTextLevel(face, "لا", Run{Language: "ar"}, 65536, language.Arabic, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.positions) != 1 {
		t.Fatal("native lam-alef ligature not formed")
	}
	spans, err := splitShapedPaint(pdfLine{{text: "ل", run: Run{Color: "#FF0000"}}, {text: "ا", run: Run{Color: "#0000FF"}}}, "لا", c, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(spans) != 2 || !spans[0].glyphs.rtl || spans[0].glyphs.clipRight || !spans[0].glyphs.clipLeft || !spans[1].glyphs.clipRight || spans[1].glyphs.clipLeft {
		t.Fatal("RTL clip sides")
	}
	if math.Abs(spans[0].glyphs.origin-c.width/2) > 1e-9 || spans[1].glyphs.origin != 0 || math.Abs(spans[0].width+spans[1].width-c.width*20) > 1e-9 {
		t.Fatal("RTL physical paint bands")
	}
	for _, s := range spans {
		if !s.glyphs.logicalCarrier || s.glyphs.positions[0].glyph != c.positions[0].glyph || s.glyphs.positions[0].logical != s.text {
			t.Fatal("RTL paint changed outline/logical mapping")
		}
	}
}

func TestPDFBidiLogicalTextPublic(t *testing.T) {
	program, err := os.ReadFile("testdata/assets/DejaVuSans.ttf")
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"שלום עולם (123) test", "سلام بالعالم ١٢٣", "A\u2067אבג (12)\u2069 Z", "אבְג سلام", "سَلَام", "\u05b0"} {
		d := &Document{Blocks: []Block{{Kind: Paragraph, Direction: "rtl", Runs: []Run{{Text: text, FontFamily: "DejaVu", FontSize: 18}}}}}
		var out bytes.Buffer
		if err := Write(context.Background(), &out, d, PDF, WriteOptions{PDFFonts: []FontFace{{Family: "DejaVu", Data: program}}}); err != nil {
			t.Fatalf("%q: %v", text, err)
		}
		back, err := ReadPDF(context.Background(), bytes.NewReader(out.Bytes()), ReadOptions{PDFSkipInference: true})
		if err != nil {
			t.Fatal(err)
		}
		if back.Text() != text {
			t.Fatalf("bidi logical text %q want %q", back.Text(), text)
		}
		assertRecoverySF(t, back)
	}
}
func TestPDFMissingBidiGlyphAllStoriesAtomic(t *testing.T) {
	bad := Block{Kind: Paragraph, Direction: "rtl", Runs: []Run{{Text: "中文"}}}
	cases := []*Document{{Blocks: []Block{bad}}, {Headers: []Block{bad}}, {Footers: []Block{bad}}, {Blocks: []Block{{Kind: Table, Rows: []Row{{Cells: []Cell{{Blocks: []Block{bad}}}}}}}}, {Blocks: []Block{{Kind: ListBlock, List: "bullet", Items: []ListItem{{Blocks: []Block{bad}}}}}}, {Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Note: &NoteReference{Kind: Footnote, ID: "n"}}}}}, Notes: []Note{{Kind: Footnote, ID: "n", Blocks: []Block{bad}}}}}
	for _, d := range cases {
		var b bytes.Buffer
		if err := Write(context.Background(), &b, d, PDF, WriteOptions{}); !errors.Is(err, ErrUnsupported) || b.Len() != 0 {
			t.Fatalf("unsupported bidi glyph must fail atomically: %v bytes %d", err, b.Len())
		}
	}
}
func TestPDFBidiL2AndAtomicNotePlacement(t *testing.T) {
	p, err := pdfBidiParagraphs(context.Background(), "אב 12 ג", "rtl")
	if err != nil {
		t.Fatal(err)
	}
	var line pdfLine
	for i, ch := range []rune("אב 12 ג") {
		br, err := p[0].sourceRange(i, i+1)
		if err != nil {
			t.Fatal(err)
		}
		line = append(line, pdfSpan{text: string(ch), width: 1, bidi: br})
	}
	if err := applyPDFLineLevels(line); err != nil {
		t.Fatal(err)
	}
	placePDFLine(line, 20)
	oracle, err := p[0].resolved.Line(0, p[0].resolved.Len())
	if err != nil {
		t.Fatal(err)
	}
	for visual, logical := range oracle.Order {
		if line[logical].visualX != float64(20-len(line)+visual) {
			t.Fatalf("L2 logical %d at %v want %d", logical, line[logical].visualX, 20-len(line)+visual)
		}
	}
	anchor := line[0]
	anchor.width = 2
	note := pdfSpan{run: Run{Note: &NoteReference{Kind: Footnote, ID: "n"}}, text: "1", width: 1, bidi: line[0].bidi}
	note.bidi.level = 0
	attached := pdfLine{anchor, note}
	placePDFLine(attached, 20)
	if attached[1].visualX != 17 || attached[0].visualX != 18 {
		t.Fatalf("RTL marker detached from anchor: %+v", attached)
	}
}
func TestPDFRunRTLCharacteristics(t *testing.T) {
	text := "first second 12+34"
	runs := []pdfRunRange{{start: 0, end: len(text), run: Run{Direction: "rtl", Language: "he"}}}
	types := pdfRunBidiTypes(text, runs)
	for i, ch := range []rune(text) {
		if ch == ' ' && types[i] != bidilevels.R {
			t.Fatal("rtl space was not overridden")
		}
		if ch == '+' && types[i] != bidilevels.ES {
			t.Fatal("Hebrew number separator was overridden")
		}
		if ch == 'f' && types[i] != bidilevels.L {
			t.Fatal("strong Latin was overridden")
		}
	}
	runs[0].run.Language = "ar"
	if pdfRunBidiTypes(text, runs)[15] != bidilevels.R {
		t.Fatal("non-Hebrew ES override missing")
	}
}

func FuzzPDFBidiRoundTrip(f *testing.F) {
	f.Add([]byte{0, 4, 7, 2, 9, 1, 3, 13}, true)
	f.Add([]byte{16, 2, 4, 1, 17, 20, 15}, false)
	program, err := os.ReadFile("testdata/assets/DejaVuSans.ttf")
	if err != nil {
		f.Fatal(err)
	}
	alphabet := []rune("ab 12אבגسلام١٢٣\u05b0\u064e\u200c\u200d\u2066\u2067\u2069\n()")
	f.Fuzz(func(t *testing.T, data []byte, rtl bool) {
		if len(data) > 32 {
			t.Skip()
		}
		var text strings.Builder
		for _, b := range data {
			text.WriteRune(alphabet[int(b)%len(alphabet)])
		}
		direction := "ltr"
		if rtl {
			direction = "rtl"
		}
		doc := &Document{Layout: Layout{Width: 300, Height: 500, Margin: 30}, Blocks: []Block{{Kind: Paragraph, Direction: direction, Runs: []Run{{Text: text.String(), FontFamily: "DejaVu", FontSize: 12}}}}}
		var out bytes.Buffer
		err := Write(context.Background(), &out, doc, PDF, WriteOptions{PDFFonts: []FontFace{{Family: "DejaVu", Data: program}}})
		if err != nil {
			if !errors.Is(err, ErrUnsupported) || out.Len() != 0 {
				t.Fatalf("non-atomic bidi write %v bytes %d", err, out.Len())
			}
			return
		}
		back, err := ReadPDF(context.Background(), bytes.NewReader(out.Bytes()), ReadOptions{PDFSkipInference: true})
		if err != nil {
			t.Fatal(err)
		}
		strip := func(s string) string {
			return strings.Map(func(r rune) rune {
				if r == '\n' || r == '\r' {
					return -1
				}
				return r
			}, s)
		}
		if strip(back.Text()) != strip(text.String()) {
			t.Fatalf("logical bidi scalars %q want %q", back.Text(), text.String())
		}
		assertRecoverySF(t, back)
	})
}

func FuzzPDFBidiFont(f *testing.F) {
	f.Add(uint32(0), byte(0))
	f.Add(uint32(400), byte(64))
	original, err := os.ReadFile("testdata/assets/DejaVuSans.ttf")
	if err != nil {
		f.Fatal(err)
	}
	f.Fuzz(func(t *testing.T, offset uint32, change byte) {
		program := append([]byte(nil), original...)
		program[int(offset)%len(program)] ^= change
		doc := &Document{Blocks: []Block{{Kind: Paragraph, Direction: "rtl", Runs: []Run{{Text: "سَلَام ", FontFamily: "DejaVu"}, {Text: "ل", FontFamily: "DejaVu", Color: "#FF0000"}, {Text: "ا", FontFamily: "DejaVu", Color: "#0000FF"}}}}}
		var out bytes.Buffer
		err := Write(context.Background(), &out, doc, PDF, WriteOptions{PDFFonts: []FontFace{{Family: "DejaVu", Data: program}}})
		if err != nil {
			if out.Len() != 0 {
				t.Fatal("partial output on mutated bidi font")
			}
			return
		}
		back, err := ReadPDF(context.Background(), bytes.NewReader(out.Bytes()), ReadOptions{PDFSkipInference: true})
		if err != nil {
			t.Fatal(err)
		}
		if back.Text() != "سَلَام لا" {
			t.Fatalf("mutated bidi logical text %q", back.Text())
		}
		assertRecoverySF(t, back)
	})
}

func TestPDFNativeItemContext(t *testing.T) {
	data, err := os.ReadFile("testdata/rtl-item-context.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Fonts map[string]struct {
			Path, SHA256 string
			UPEM         float64
		}
		Items []struct {
			Row, Context, Font, Language string
			Start, End                   int
			Size, Advance                float64
			Glyphs                       []struct {
				GID           uint16
				Cluster       int
				X, Y, Advance float64
			}
		}
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	r := textTestRenderer()
	var faces []FontFace
	for style, f := range fixture.Fonts {
		program, err := os.ReadFile(f.Path)
		if err != nil {
			t.Fatal(err)
		}
		h := sha256.Sum256(program)
		if hex.EncodeToString(h[:]) != f.SHA256 {
			t.Fatal("context oracle font changed")
		}
		faces = append(faces, FontFace{Family: "DejaVu", Bold: style == "bold", Data: program})
	}
	if err := r.addFonts(faces); err != nil {
		t.Fatal(err)
	}
	for _, tc := range fixture.Items {
		text := string([]rune(tc.Context)[tc.Start:tc.End])
		run := Run{Text: text, FontFamily: "DejaVu", Bold: tc.Font == "bold", FontSize: tc.Size, Language: tc.Language}
		face, err := r.resolveFont(run)
		if err != nil {
			t.Fatal(err)
		}
		shaped, err := r.shapeGlyphTextContext(face, text, run, 65536, language.Arabic, 1, []rune(tc.Context), tc.Start)
		if err != nil {
			t.Fatal(err)
		}
		upem := fixture.Fonts[tc.Font].UPEM
		if len(shaped.positions) != len(tc.Glyphs) || math.Abs(shaped.width-tc.Advance/upem) > 1e-9 {
			t.Fatalf("context %s cardinality/advance", tc.Row)
		}
		for i, g := range shaped.positions {
			w := tc.Glyphs[i]
			if g.glyph != w.GID || g.cluster != w.Cluster || math.Abs(g.x-w.X/upem) > 1e-9 || math.Abs(g.y-w.Y/upem) > 1e-9 || math.Abs(g.advance-w.Advance/upem) > 1e-9 {
				t.Fatalf("native item %s %+v want %+v", tc.Row, g, w)
			}
		}
	}
	// The line pipeline must actually pass item context, not only expose the API.

	// The caller's lookup alias is not part of the embedded Go font program.

	// Independently authored primitive-writer fixture omits FontFamily, keeping
	// the embedded-name fallback oracle independent of manifold's new descriptor.

	// Font/model limitations remain explicit and never yield partial PDF bytes.

	// Source programs are not licensed output fonts. A caller must choose
	// substitutions and explicitly resolve sizes finer than PDF writer precision.

	row := 0
	for row < len(fixture.Items) {
		first, second := fixture.Items[row], fixture.Items[row+1]
		var runs []Run
		if first.Start == 0 && second.Start == 1 {
			for _, tc := range fixture.Items[row : row+2] {
				runs = append(runs, Run{Text: "ب", FontFamily: "DejaVu", Bold: tc.Font == "bold", FontSize: tc.Size, Language: tc.Language})
			}
		} else {
			runs = []Run{{Text: first.Context, FontFamily: "DejaVu", FontSize: 18}}
		}
		lines, err := r.linesDirection(runs, 300, 18, "rtl")
		if err != nil {
			t.Fatal(err)
		}
		var glyphs []uint16
		for _, s := range lines[0] {
			if s.glyphs != nil {
				for _, g := range s.glyphs.positions {
					glyphs = append(glyphs, g.glyph)
				}
			}
		}
		if len(glyphs) != 2 || glyphs[0] != first.Glyphs[0].GID || glyphs[1] != second.Glyphs[0].GID {
			t.Fatalf("line lost %s joining context: %v", first.Row, glyphs)
		}
		row += 2
	}
	if len(fixture.Items) != 14 {
		t.Fatal("context oracle item count")
	}
}

func TestPDFRTLDecimalNoteGlyphDirection(t *testing.T) {
	data, err := os.ReadFile("testdata/assets/DejaVuSans.ttf")
	if err != nil {
		t.Fatal(err)
	}
	r := textTestRenderer()
	if err := r.addFonts([]FontFace{{Family: "DejaVu", Data: data}}); err != nil {
		t.Fatal(err)
	}
	r.notes.labels[noteKey{Footnote, "n"}] = "10"
	lines, err := r.linesDirection([]Run{{Text: "שלום", FontFamily: "DejaVu"}, {Note: &NoteReference{Kind: Footnote, ID: "n"}, FontFamily: "DejaVu"}}, 200, 18, "rtl")
	if err != nil {
		t.Fatal(err)
	}
	label := lines[0][len(lines[0])-1]
	if label.text != "10" || label.bidi.level != 1 || label.glyphs.rtl || len(label.glyphs.positions) != 2 {
		t.Fatalf("decimal note direction %+v", label)
	}
	if label.glyphs.positions[0].glyph != 20 || label.glyphs.positions[1].glyph != 19 {
		t.Fatal("10 note label reversed to 01")
	}
	if label.visualX >= lines[0][0].visualX {
		t.Fatal("LTR decimal label detached from RTL anchor")
	}
}

func TestPDFBidiInventedSpaceRegressions(t *testing.T) {
	program, err := os.ReadFile("testdata/assets/DejaVuSans.ttf")
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"א(2((((", "َא٢(((("} {
		doc := &Document{Layout: Layout{Width: 300, Height: 500, Margin: 30}, Blocks: []Block{{Kind: Paragraph, Direction: "ltr", Runs: []Run{{Text: text, FontFamily: "DejaVu", FontSize: 12}}}}}
		var out bytes.Buffer
		if err := Write(context.Background(), &out, doc, PDF, WriteOptions{PDFFonts: []FontFace{{Family: "DejaVu", Data: program}}}); err != nil {
			t.Fatal(err)
		}
		back, err := ReadPDF(context.Background(), bytes.NewReader(out.Bytes()), ReadOptions{PDFSkipInference: true})
		if err != nil {
			t.Fatal(err)
		}
		if back.Text() != text {
			t.Fatalf("invented bidi space %q want %q", back.Text(), text)
		}
		assertRecoverySF(t, back)
	}
}
func TestPDFRectangularTextClipBoundsAndRestore(t *testing.T) {
	d := readRecovery(t, recoveryPage("q 10 20 10 20 re W n BT /F 10 Tf 15 25 Td (ABCD) Tj ET Q BT /F 10 Tf 30 25 Td (END) Tj ET", ""))
	runs := recoveryRuns(d)
	if len(runs) != 3 || runs[0].Text != "ABCD" || runs[1].Text != " " || runs[1].Source.Precision != "synthetic_separator" || runs[2].Text != "END" {
		t.Fatalf("logical clipping content %+v", runs)
	}
	a, _, b, _ := quadBounds(*runs[0].Source.Quad)
	if a != 15 || b != 20 || runs[0].Source.Precision != "rect_clipped_font_metrics" {
		t.Fatalf("clip %+v", runs[0].Source)
	}
	if strings.Contains(runs[2].Source.Precision, "clip") {
		t.Fatal("clip escaped Q restore")
	}
	if hasDiagnostic(d, "pdf_clipping") {
		t.Fatal("known text clip falsely unresolved")
	}
	assertRecoverySF(t, d)
}
func TestPDFComplexAndFullyClippedTextAreExplicit(t *testing.T) {
	for _, tc := range []struct{ prefix, precision, diagnostic string }{
		{"0 0 10 10 re W n ", "fully_clipped_font_metrics", "pdf_clipped_text"},
		{"0 0 m 100 0 l 100 100 l h W n ", "unresolved_clip_font_metrics", "pdf_clipping"},
	} {
		d := readRecovery(t, recoveryPage(tc.prefix+"BT /F 10 Tf 30 25 Td (VISIBLE-OPERATOR) Tj ET", ""))
		run := recoveryRuns(d)[0]
		if run.Text != "VISIBLE-OPERATOR" || run.Source.Precision != tc.precision || !hasDiagnostic(d, tc.diagnostic) {
			t.Fatalf("clip %+v %+v", run, d.Diagnostics)
		}
		assertRecoverySF(t, d)
	}
}
func FuzzPDFTextRectClip(f *testing.F) {
	f.Add(byte(10), byte(20), byte(15), byte(25))
	f.Add(byte(100), byte(50), byte(5), byte(80))
	f.Fuzz(func(t *testing.T, x, y, tx, ty byte) {
		content := fmt.Sprintf("%d %d 20 30 re W n BT /F 10 Tf %d %d Td (CLIPPED-TEXT) Tj ET", x, y, tx, ty)
		data := recoveryPage(content, "")
		d, err := ReadPDF(context.Background(), bytes.NewReader(data), ReadOptions{PDFSkipInference: true})
		if err != nil {
			t.Fatal(err)
		}
		if d.Text() != "CLIPPED-TEXT" {
			t.Fatalf("operator text discarded %q", d.Text())
		}
		assertRecoverySF(t, d)
	})
}
func TestPDFRecoveredEditableDeviceStyles(t *testing.T) {
	for _, tc := range []struct {
		paint, color, space string
		components          []float64
	}{
		{"", "#000000", "DeviceGray", []float64{0}},
		{"1 0 0 rg", "#FF0000", "DeviceRGB", []float64{1, 0, 0}},
		{"0.5 g", "#808080", "DeviceGray", []float64{.5}},
		{"0 1 1 0 k", "#FF0000", "DeviceCMYK", []float64{0, 1, 1, 0}},
		{"1 0 0 rg 0 0 1 RG 1 Tr", "#0000FF", "DeviceRGB", []float64{0, 0, 1}},
	} {
		t.Run(tc.paint, func(t *testing.T) {
			d := readRecovery(t, recoveryPage(tc.paint+" BT /F 14 Tf 10 100 Td (Style) Tj ET", ""))
			rr := recoveryRuns(d)
			if len(rr) != 1 {
				t.Fatalf("runs %+v", rr)
			}
			r := rr[0]
			if r.FontFamily != "Courier" || r.FontSize != 14 || r.Color != tc.color || r.Source.Font != "Courier" || r.Source.FontFamilyEvidence != "core_font" || r.Source.ColorSpace != tc.space || !reflect.DeepEqual(r.Source.ColorComponents, tc.components) {
				t.Fatalf("style %+v source %+v", r, r.Source)
			}
			if (tc.space == "DeviceCMYK") != hasDiagnostic(d, "pdf_text_color_approximation") {
				t.Fatal("CMYK approximation not disclosed")
			}
			if (tc.paint == "0.5 g") != hasDiagnostic(d, "pdf_text_color_quantized") {
				t.Fatal("quantization not disclosed")
			}
			assertRecoverySF(t, d)
		})
	}
	d := readRecovery(t, recoveryPage("q 2 0 0 3 0 0 cm BT /F 10 Tf 10 50 Td (Scaled) Tj ET Q", ""))
	if recoveryRuns(d)[0].FontSize != 30 || !hasDiagnostic(d, "pdf_text_transform") {
		t.Fatal("physical size/nonuniform scale evidence lost")
	}
}

func TestPDFRecoveredPaintStateAndAliases(t *testing.T) {
	objects := taggedTestObjects("/Alias cs 1 0 0 sc BT /F 12 Tf 10 300 Td (Red) Tj ET q 0 1 0 rg /X Do Q BT /F 12 Tf 10 200 Td (RedAgain) Tj ET", "<< /Type /StructTreeRoot /K [] >>")
	objects[2] = strings.Replace(objects[2], "/Font <<", "/ColorSpace << /Alias /Alias2 /Alias2 /DeviceRGB >> /Font <<", 1)
	objects[5] = recoveryStream("/Type /XObject /Subtype /Form /BBox [0 0 300 400]", "BT /F 12 Tf 10 250 Td (GreenForm) Tj ET")
	d := readRecovery(t, recoveryPDF(objects...))
	rr := recoveryRuns(d)
	if len(rr) != 3 || rr[0].Color != "#FF0000" || rr[1].Color != "#00FF00" || rr[2].Color != "#FF0000" {
		t.Fatalf("paint state %+v", rr)
	}
	assertRecoverySF(t, d)
	objects[2] = strings.Replace(objects[2], "/Alias2 /DeviceRGB", "/Alias2 /Alias", 1)
	if _, err := ReadPDF(context.Background(), bytes.NewReader(recoveryPDF(objects...)), ReadOptions{}); err == nil {
		t.Fatal("color alias cycle accepted")
	}
	for _, paint := range []string{"1 0 rg", "/DeviceGray cs 1 0 sc", "7 cs"} {
		if _, err := ReadPDF(context.Background(), bytes.NewReader(recoveryPage(paint+" BT /F 12 Tf (X) Tj ET", "")), ReadOptions{}); err == nil {
			t.Fatalf("malformed color accepted: %s", paint)
		}
	}
	d = readRecovery(t, recoveryPage("/Pattern cs /P scn BT /F 12 Tf (Unknown) Tj ET", ""))
	if recoveryRuns(d)[0].Color != "" || recoveryRuns(d)[0].Source.ColorSpace != "" || !hasDiagnostic(d, "pdf_text_color") {
		t.Fatal("unresolved paint falsely recovered")
	}
}

func TestPDFRecoveredFontFamilyEvidence(t *testing.T) {
	for _, tc := range []struct{ face, descriptor, want, evidence string }{
		{"ABCDEF+Custom-Bold", "/FontFamily (Editorial Family)", "Editorial Family", "descriptor"},
		{"ABCDEF+Custom-Bold", "", "Custom-Bold", "postscript_fallback"},
		{"Courier-BoldOblique", "", "Courier", "core_font"},
	} {
		objects := taggedTestObjects("BT /F 12 Tf 10 200 Td (Family) Tj ET", "<< /Type /StructTreeRoot /K [] >>")
		objects[3] = "<< /Type /Font /Subtype /Type1 /BaseFont /" + tc.face + " /FontDescriptor << /Type /FontDescriptor /FontName /" + tc.face + " " + tc.descriptor + " >> >>"
		d := readRecovery(t, recoveryPDF(objects...))
		r := recoveryRuns(d)[0]
		if r.FontFamily != tc.want || r.Source.FontFamilyEvidence != tc.evidence || r.Source.Font != tc.face {
			t.Fatalf("font %+v source %+v", r, r.Source)
		}
		if (tc.evidence == "postscript_fallback") != hasDiagnostic(d, "pdf_font_family_inferred") {
			t.Fatal("uncertain grouping not disclosed")
		}
		assertRecoverySF(t, d)
	}

	d := &Document{Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "Embedded", FontFamily: "Caller Alias", FontSize: 18, Color: "#CC6633"}}}}}
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, PDF, WriteOptions{PDFFonts: []FontFace{{Family: "Caller Alias", Data: goregular.TTF}}}); err != nil {
		t.Fatal(err)
	}
	generated := append([]byte(nil), out.Bytes()...)

	primitive := gofpdf.New("P", "pt", "A4", "")
	primitive.AddUTF8FontFromBytes("Caller Alias", "", append([]byte(nil), goregular.TTF...))
	primitive.AddPage()
	primitive.SetFont("Caller Alias", "", 18)
	primitive.SetTextColor(204, 102, 51)
	primitive.Text(54, 72, "Embedded")
	out.Reset()
	if err := primitive.Output(&out); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		input    []byte
		evidence string
	}{{generated, "descriptor"}, {out.Bytes(), "embedded_name"}} {
		back := readRecovery(t, tc.input)
		r := recoveryRuns(back)[0]
		if r.FontFamily != "Go" || r.Source.FontFamilyEvidence != tc.evidence || r.FontSize != 18 || r.Color != "#CC6633" {
			t.Fatalf("embedded style %+v source %+v", r, r.Source)
		}
		assertRecoverySF(t, back)
	}
}

func TestPDFActualTextRetainsOnlyCommonEditableStyles(t *testing.T) {
	for _, structural := range []bool{false, true} {
		for _, mixed := range []bool{false, true} {
			t.Run(strings.Join([]string{map[bool]string{true: "structural", false: "stream"}[structural], map[bool]string{true: "mixed", false: "same"}[mixed]}, "/"), func(t *testing.T) {
				second := ""
				if mixed {
					second = "0 0 1 rg /F 16 Tf"
				}
				content := "1 0 0 rg BT /F 12 Tf 10 250 Td /P << /MCID 0 >> BDC (A) Tj " + second + " (B) Tj EMC ET"
				var data []byte
				if structural {
					data = taggedTestPDF(content, "<< /Type /StructTreeRoot /K 8 0 R >>", "<< /Type /StructElem /S /P /P 7 0 R /Pg 3 0 R /ActualText (Replacement) /K 0 >>")
				} else {
					data = recoveryPage(strings.Replace(content, "/MCID 0", "/ActualText (Replacement)", 1), "")
				}
				d := readRecovery(t, data)
				rr := recoveryRuns(d)
				if len(rr) != 1 || rr[0].Text != "Replacement" {
					t.Fatalf("replacement %+v", rr)
				}
				r := rr[0]
				if r.FontFamily != "Courier" {
					t.Fatal("common family lost")
				}
				if mixed {
					if r.FontSize != 0 || r.Color != "" || r.Source.ColorSpace != "" || !hasDiagnostic(d, "pdf_actualtext_style") {
						t.Fatalf("ambiguous style %+v source %+v", r, r.Source)
					}
				} else {
					if r.FontSize != 12 || r.Color != "#FF0000" || r.Source.ColorSpace != "DeviceRGB" || !reflect.DeepEqual(r.Source.ColorComponents, []float64{1, 0, 0}) {
						t.Fatalf("common style %+v source %+v", r, r.Source)
					}
				}
				assertRecoverySF(t, d)
			})
		}
	}
}

func TestPDFSourceStyleValidationAndBoundedMetadata(t *testing.T) {
	valid := SourceInfo{Format: PDF, Structure: "unstructured", Evidence: "test", FontFamilyEvidence: "descriptor", ColorSpace: "DeviceRGB", ColorComponents: []float64{1, 0, .5}}
	for _, bad := range []SourceInfo{
		{FontFamilyEvidence: "guessed"}, {ColorSpace: "DeviceRGB", ColorComponents: []float64{1, 0}}, {ColorSpace: "DeviceGray", ColorComponents: []float64{math.NaN()}}, {ColorSpace: "DeviceGray", ColorComponents: []float64{2}}, {ColorComponents: []float64{0}},
	} {
		bad.Format = PDF
		bad.Structure = "unstructured"
		bad.Evidence = "test"
		if bad.validate(nil) == nil {
			t.Fatalf("invalid evidence accepted %+v", bad)
		}
	}
	if err := valid.validate(nil); err != nil {
		t.Fatal(err)
	}
	var compressed bytes.Buffer
	z := zlib.NewWriter(&compressed)
	_, _ = z.Write(goregular.TTF)
	_ = z.Close()
	s := &types.StreamDict{Raw: compressed.Bytes(), FilterPipeline: []types.PDFFilter{{Name: "FlateDecode"}}}
	decoded, err := boundedFontMetadata(context.Background(), s)
	if err != nil || !bytes.Equal(decoded, goregular.TTF) {
		t.Fatalf("metadata decode %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := boundedFontMetadata(ctx, s); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation ignored")
	}
	s.FilterPipeline[0].Name = "DCTDecode"
	if _, err := boundedFontMetadata(context.Background(), s); err == nil {
		t.Fatal("unsupported font filter accepted")
	}
	compressed.Reset()
	z = zlib.NewWriter(&compressed)
	_, _ = z.Write(make([]byte, maxFontMetadataBytes+1))
	_ = z.Close()
	s.Raw = compressed.Bytes()
	s.FilterPipeline[0].Name = "FlateDecode"
	if _, err := boundedFontMetadata(context.Background(), s); err == nil {
		t.Fatal("expanded metadata exceeds limit")
	}
}

func TestPDFRecoveredStyleSeparatorsAndMetadataFallback(t *testing.T) {
	for _, mixed := range []bool{false, true} {
		paint := ""
		if mixed {
			paint = "0 0 1 rg /F 16 Tf"
		}
		d := readRecovery(t, recoveryPage("1 0 0 rg BT /F 12 Tf 10 200 Td (A) Tj "+paint+" 30 0 Td (B) Tj ET", ""))
		rr := recoveryRuns(d)
		if len(rr) != 3 || rr[1].Text != " " || rr[1].Source.Precision != "synthetic_separator" {
			t.Fatalf("gap %+v", rr)
		}
		gap := rr[1]
		if gap.FontFamily != "Courier" || gap.Source.ColorSpace != "" || gap.Source.FontFamilyEvidence != "" {
			t.Fatalf("synthetic source %+v", gap)
		}
		if mixed {
			if gap.FontSize != 0 || gap.Color != "" {
				t.Fatal("invented mixed gap style")
			}
		} else if gap.FontSize != 12 || gap.Color != "#FF0000" {
			t.Fatal("common gap style lost")
		}
		assertRecoverySF(t, d)
	}
	objects := taggedTestObjects("BT /F 12 Tf 10 200 Td (Retained) Tj ET", "<< /Type /StructTreeRoot /K [] >>")
	objects[3] = "<< /Type /Font /Subtype /Type1 /BaseFont /CustomFace /FontDescriptor << /Type /FontDescriptor /FontName /CustomFace /FontFile2 8 0 R >> >>"
	objects = append(objects, recoveryStream("", "not a font"))
	d := readRecovery(t, recoveryPDF(objects...))
	r := recoveryRuns(d)[0]
	if r.Text != "Retained" || r.FontFamily != "CustomFace" || r.Source.FontFamilyEvidence != "postscript_fallback" || !hasDiagnostic(d, "pdf_font_family") {
		t.Fatal("invalid embedded metadata lost text or fabricated family")
	}
}

func FuzzPDFEditableStyles(f *testing.F) {
	f.Add(12.0, 1.0, 0.0, 0.0, false)
	f.Add(18.5, .2, .4, .8, true)
	f.Fuzz(func(t *testing.T, size, red, green, blue float64, actual bool) {
		if !finite(size) || !finite(red) || !finite(green) || !finite(blue) {
			t.Skip()
		}
		size = math.Mod(math.Abs(size), 40) + 1
		red = math.Mod(math.Abs(red), 1)
		green = math.Mod(math.Abs(green), 1)
		blue = math.Mod(math.Abs(blue), 1)
		content := fmt.Sprintf("%.8f %.8f %.8f rg BT /F %.8f Tf 10 200 Td (Style) Tj ET", red, green, blue, size)
		if actual {
			content = strings.Replace(content, "(Style) Tj", "/Span << /ActualText (Replaced) >> BDC (Style) Tj EMC", 1)
		}
		d := readRecovery(t, recoveryPage(content, ""))
		r := recoveryRuns(d)[0]
		if r.FontFamily != "Courier" || math.Abs(r.FontSize-size) > 1e-7 || len(r.Color) != 7 || r.Source.ColorSpace != "DeviceRGB" {
			t.Fatalf("style %+v", r)
		}
		assertRecoverySF(t, d)
	})
}

func TestPDFActualTextExactDecodedAlignmentKeepsMixedStyles(t *testing.T) {
	content := "BT /F 12 Tf 1 0 0 rg 10 200 Td /Span << /ActualText (AB) >> BDC (A) Tj 0 0 1 rg /F 16 Tf (B) Tj EMC ET"
	d := readRecovery(t, recoveryPage(content, ""))
	runs := recoveryRuns(d)
	if len(runs) != 2 || runs[0].Text != "A" || runs[1].Text != "B" || runs[0].Color != "#FF0000" || runs[1].Color != "#0000FF" || runs[0].FontSize != 12 || runs[1].FontSize != 16 {
		t.Fatalf("exact ActualText alignment %+v", runs)
	}
	for _, r := range runs {
		if r.Source.Evidence != "pdf_actualtext_aligned" {
			t.Fatal("alignment not distinguished in provenance")
		}
	}
	if hasDiagnostic(d, "pdf_actualtext_style") {
		t.Fatal("exact style alignment diagnosed as ambiguous")
	}
	assertRecoverySF(t, d)
}
func textTestRenderer() *pdfRenderer {
	p := gofpdf.NewCustom(&gofpdf.InitType{UnitStr: "pt", Size: gofpdf.SizeType{Wd: 300, Ht: 400}})
	for _, font := range pdfFonts {
		p.AddUTF8FontFromBytes("Go", font.style, append([]byte(nil), font.data...))
	}
	return &pdfRenderer{ctx: context.Background(), pdf: p, notes: &noteCatalog{labels: map[noteKey]string{{Footnote, "n"}: "1"}}}
}
func lineText(line pdfLine) string {
	var b strings.Builder
	for _, span := range line {
		b.WriteString(span.text)
	}
	return b.String()
}
func lineAdvance(line pdfLine) float64 {
	w := 0.0
	for _, span := range line {
		w += span.width
	}
	return w
}

func TestPDFTextWordsCrossFormattingRuns(t *testing.T) {
	r := textTestRenderer()
	lines, e := r.lines([]Run{{Text: "prefix "}, {Text: "inter", Bold: true}, {Text: "operability"}}, 80, 11)
	if e != nil {
		t.Fatal(e)
	}
	if len(lines) != 2 || strings.TrimSpace(lineText(lines[0])) != "prefix" || lineText(lines[1]) != "interoperability" || !lines[1][0].run.Bold {
		t.Fatalf("format boundary split a word: %+v", lines)
	}
	for _, line := range lines {
		if lineAdvance(line) > 80+1e-7 {
			t.Fatal("wrapped text exceeds measured width")
		}
	}
}
func TestPDFTextGlueAcrossRuns(t *testing.T) {
	for _, glue := range []string{"\u00a0", "\u2007", "\u202f", "\u2060", "\ufeff", "\u2011"} {
		t.Run(glue, func(t *testing.T) {
			r := textTestRenderer()
			r.pdf.SetFont("Go", "", 11)
			word := "A" + glue + "B"
			width := r.textAdvance(word, 11) + 1
			runs := []Run{{Text: "X A"}, {Text: glue}, {Text: "B"}}
			lines, e := r.lines(runs, width, 11)
			if e != nil {
				t.Fatal(e)
			}
			if len(lines) != 2 || strings.TrimSpace(lineText(lines[0])) != "X" || lineText(lines[1]) != word {
				t.Fatalf("glue split across runs: %+v", lines)
			}
			if _, e = r.lines([]Run{{Text: word}}, width-2, 11); !errors.Is(e, ErrUnsupported) {
				t.Fatalf("overwide glue did not reject: %v", e)
			}
		})
	}
}
func TestPDFTextGraphemesAndHardBreaks(t *testing.T) {
	r := textTestRenderer()
	original := "aaaaae\u0301bbbbbe\u0301ccc"
	lines, e := r.lines([]Run{{Text: original}}, 25, 11)
	if e != nil {
		t.Fatal(e)
	}
	var joined strings.Builder
	for _, line := range lines {
		text := lineText(line)
		first, _ := utf8.DecodeRuneInString(text)
		if unicode.IsMark(first) {
			t.Fatal("emergency wrap split a grapheme")
		}
		joined.WriteString(text)
		if lineAdvance(line) > 25+1e-7 {
			t.Fatal("grapheme overflow")
		}
	}
	if joined.String() != original {
		t.Fatal("emergency wrap changed logical Unicode")
	}
	if _, e = r.lines([]Run{{Text: "e"}, {Text: "\u0301", Bold: true}}, 100, 11); !errors.Is(e, ErrUnsupported) {
		t.Fatal("unsupported mixed-style combining cluster accepted")
	}
	lines, e = r.lines([]Run{{Text: "A\r"}, {Text: "\nB\u2028C"}}, 100, 11)
	if e != nil {
		t.Fatal(e)
	}
	if len(lines) != 3 || lineText(lines[0]) != "A\r\n" || lineText(lines[1]) != "B\u2028" || lineText(lines[2]) != "C" {
		t.Fatalf("hard break handling %+v", lines)
	}
}
func TestPDFDiscretionaryHyphenAndNoteAnchor(t *testing.T) {
	r := textTestRenderer()
	r.pdf.SetFont("Go", "", 11)
	width := r.pdf.GetStringWidth("hy-") + .01
	lines, e := r.lines([]Run{{Text: "hy\u00adphen"}}, width, 11)
	if e != nil {
		t.Fatal(e)
	}
	if lineText(lines[0]) != "hy\u00ad" || len(lines[0]) < 2 || math.Abs(lines[0][len(lines[0])-1].width-r.pdf.GetStringWidth("-")) > 1e-7 {
		t.Fatal("selected soft hyphen has no measured visible hyphen")
	}
	lines, e = r.lines([]Run{{Text: "hy\u00adphen"}}, 100, 11)
	if e != nil {
		t.Fatal(e)
	}
	if len(lines) != 1 {
		t.Fatal("unnecessary discretionary break")
	}
	for _, span := range lines[0] {
		if span.text == "\u00ad" && span.width != 0 {
			t.Fatal("unused soft hyphen painted")
		}
	}
	lines, e = r.lines([]Run{{Text: "prefix anchor"}, noteRun(Footnote, "n")}, 60, 11)
	if e != nil {
		t.Fatal(e)
	}
	if len(lines) != 2 || lineText(lines[1]) != "anchor1" || len(lineNoteKeys(lines[1])) != 1 || len(lineNoteKeys(lines[0])) != 0 {
		t.Fatal("note marker separated from anchor")
	}
}
func TestPDFTypographyLogicalTextAndGeometry(t *testing.T) {
	text := "A\u00a0B C\u2007D E\u202fF G\u2060H I\u200bJ café cafe\u0301 K\u2011L M\u2003N O\u200aP"
	doc := &Document{Layout: Layout{Width: 1000, Height: 160, Margin: 20}, Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: text, Bold: true, Italic: true, Underline: true, Strike: true, Link: "https://example.invalid/spacing"}}}}}
	var output bytes.Buffer
	if e := Write(context.Background(), &output, doc, PDF, WriteOptions{}); e != nil {
		t.Fatal(e)
	}
	if !bytes.HasPrefix(output.Bytes(), []byte("%PDF-2.0")) {
		t.Fatal("Tagged shaped output did not declare PDF 2.0")
	}
	got, e := ReadPDF(context.Background(), bytes.NewReader(output.Bytes()), ReadOptions{})
	if e != nil {
		t.Fatal(e)
	}
	if got.Text() != text {
		t.Fatalf("logical characters changed\nwant %q\ngot  %q", text, got.Text())
	}
	for _, run := range recoveryRuns(got) {
		if !run.Bold || !run.Italic {
			t.Fatal("homogeneous ActualText formatting lost")
		}
	}

	foundEm := false
	for _, run := range recoveryRuns(got) {
		if strings.Contains(run.Text, "\u2003") && run.Source != nil {
			for _, region := range run.Source.Regions {
				a, _, b, _ := quadBounds(region.Quad)
				if math.Abs(b-a-11) < .02 {
					foundEm = true
				}
			}
		}
	}
	if !foundEm {
		t.Fatal("em space has no measured 11pt source region")
	}

	for _, bad := range []string{"q\u0301", "中文", "مرحبا"} {
		doc.Blocks[0].Runs[0].Text = bad
		output.Reset()
		if e = Write(context.Background(), &output, doc, PDF, WriteOptions{}); !errors.Is(e, ErrUnsupported) || output.Len() != 0 {
			t.Fatalf("unsupported shaping/glyph emitted: %v", e)
		}
	}
}
func TestPDFLineBoundaryWhitespaceAndCancellation(t *testing.T) {
	r := textTestRenderer()
	lines, e := r.lines([]Run{{Text: "A\u2003B"}}, 12, 11)
	if e != nil {
		t.Fatal(e)
	}
	if len(lines) != 2 || lineText(lines[0]) != "A\u2003" || lineText(lines[1]) != "B" || lineAdvance(lines[0]) > 12 {
		t.Fatalf("line-edge space discarded/overflowed %+v", lines)
	}
	var cancel context.CancelFunc
	r.ctx, cancel = context.WithCancel(context.Background())
	cancel()
	if _, e = r.lines([]Run{{Text: "text"}}, 100, 11); !errors.Is(e, context.Canceled) {
		t.Fatal("layout ignored cancellation")
	}
}
func TestPDFPublishedExportNowRenders(t *testing.T) {
	input, e := os.ReadFile("testdata/real/copyright-office-circular-1-2021.pdf")
	if e != nil {
		t.Fatal(e)
	}
	imported := readRecovery(t, input)
	sf, e := ToStructured(imported)
	if e != nil {
		t.Fatal(e)
	}
	shared, e := FromStructured(sf)
	if e != nil {
		t.Fatal(e)
	}
	var out bytes.Buffer

	if e = Write(context.Background(), &out, shared, PDF, WriteOptions{AllowLossy: true}); !errors.Is(e, ErrUnsupported) || out.Len() != 0 {
		t.Fatalf("missing font substitution must reject atomically: %v", e)
	}
	mapping := map[string]string{}
	var prepare func([]Block)
	prepare = func(blocks []Block) {
		for i := range blocks {
			for j := range blocks[i].Runs {
				r := &blocks[i].Runs[j]
				mapping[r.FontFamily] = "Go"
				r.FontSize = math.Round(r.FontSize*100) / 100
			}
			for j := range blocks[i].Items {
				prepare(blocks[i].Items[j].Blocks)
			}
			for j := range blocks[i].Rows {
				for k := range blocks[i].Rows[j].Cells {
					prepare(blocks[i].Rows[j].Cells[k].Blocks)
				}
			}
		}
	}
	prepare(shared.Blocks)
	if len(mapping) != 10 {
		t.Fatalf("publication families lost: %v", mapping)
	}
	if e = Write(context.Background(), &out, shared, PDF, WriteOptions{AllowLossy: true, PDFFontMap: mapping}); e != nil {
		t.Fatal(e)
	}
	rendered, e := ReadPDF(context.Background(), bytes.NewReader(out.Bytes()), ReadOptions{})
	if e != nil {
		t.Fatal(e)
	}
	if rendered.Metadata.Title != shared.Metadata.Title || rendered.Metadata.PageCount < 2 {
		t.Fatal("published output metadata/pagination missing")
	}
	for _, space := range []string{"\u2003", "\u200a", "\u2008"} {
		if strings.Count(rendered.Text(), space) != strings.Count(shared.Text(), space) {
			t.Fatalf("publication spacing lost: %U", []rune(space)[0])
		}
	}
	for _, run := range recoveryRuns(rendered) {
		if run.Source == nil || run.Source.Quad == nil {
			continue
		}
		_, _, right, bottom := quadBounds(*run.Source.Quad)
		if right > rendered.Pages[run.Source.Page-1].Width-54+.1 || bottom > rendered.Pages[run.Source.Page-1].Height-54+.1 {
			t.Fatalf("published text outside content bounds: %+v", run.Source)
		}
	}
}

func TestPDFRejectsOversizedPageWithoutPartialOutput(t *testing.T) {
	d := &Document{Layout: Layout{Width: 15000, Height: 600, Margin: 20}, Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "text"}}}}}
	var out bytes.Buffer
	if e := Write(context.Background(), &out, d, PDF, WriteOptions{}); !errors.Is(e, ErrUnsupported) || out.Len() != 0 {
		t.Fatalf("oversized default-unit PDF emitted: %v", e)
	}
}
func FuzzPDFTextLayout(f *testing.F) {
	for _, text := range []string{"prefix interoperability", "A\u2003B", "A\u00a0B", "cafe\u0301", "a\r\nb\u2028c", "hy\u00adphenation", "ab\u2060cd", "Latin Ελληνικά Привет"} {
		f.Add(text, uint16(80), uint16(5))
	}
	f.Fuzz(func(t *testing.T, text string, widthValue, splitValue uint16) {
		if len(text) > 8192 || validateString(text) != nil {
			return
		}
		runes := []rune(text)
		split := int(splitValue) % (len(runes) + 1)
		runs := []Run{{Text: string(runes[:split])}, {Text: string(runes[split:]), Bold: true}}
		width := 12 + float64(widthValue%400)
		r := textTestRenderer()
		lines, e := r.lines(runs, width, 11)
		if e != nil {
			if !errors.Is(e, ErrUnsupported) {
				t.Fatal(e)
			}
			return
		}
		var reconstructed strings.Builder
		for _, line := range lines {
			if lineAdvance(line) > width+1e-6 {
				t.Fatalf("layout exceeded content area %g > %g", lineAdvance(line), width)
			}
			reconstructed.WriteString(lineText(line))
		}
		if reconstructed.String() != text {
			t.Fatalf("layout changed logical text %q -> %q", text, reconstructed.String())
		}
	})
}
