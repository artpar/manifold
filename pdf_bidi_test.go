package manydoc

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/artpar/manydoc/internal/bidilevels"
	"math"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/benoitkugler/textlayout/language"
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
