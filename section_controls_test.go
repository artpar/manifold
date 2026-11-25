package manifold

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
	"testing"
)

func controlSection(id string, start SectionStart, blocks ...Block) *Section {
	stories := func(role string) RunningStories {
		return RunningStories{Default: sectionStory(id + "_" + role + "_DEFAULT"), First: sectionStory(id + "_" + role + "_FIRST"), Even: sectionStory(id + "_" + role + "_EVEN")}
	}
	return &Section{Start: start, FirstPage: true, Headers: stories("HEAD"), Footers: stories("FOOT"), Blocks: blocks}
}

type controlCase struct {
	name     string
	sections []*Section
	pages    []string
}

func sectionControlCases() []controlCase {
	p := sectionParagraph
	br := Block{Kind: PageBreak}
	before := func(s string) Block { b := p(s); b.PageBreakBefore = true; return b }
	return []controlCase{
		{"leading", []*Section{controlSection("A", SectionEvenPage, br, br, p("A_BODY"))}, []string{"A_FIRST", "A_EVEN", "A_DEFAULT A_BODY"}},
		{"adjacent", []*Section{controlSection("A", "", p("A_BODY"), br), controlSection("B", SectionContinuous, br, p("B_BODY"))}, []string{"A_FIRST A_BODY", "A_EVEN", "B_EVEN B_BODY"}},
		{"trailing-next", []*Section{controlSection("A", "", p("A_BODY"), br), controlSection("B", SectionNextPage, p("B_BODY"))}, []string{"A_FIRST A_BODY", "A_EVEN", "B_FIRST B_BODY"}},
		{"empty-continuous", []*Section{controlSection("A", ""), controlSection("B", SectionContinuous), controlSection("C", SectionContinuous, p("C_BODY"), br, p("C_NEXT"))}, []string{"A_FIRST C_BODY", "C_EVEN C_NEXT"}},
		{"empty-next", []*Section{controlSection("A", "", p("A_BODY")), controlSection("B", SectionNextPage), controlSection("C", SectionContinuous, p("C_BODY"), br, p("C_NEXT"))}, []string{"A_FIRST A_BODY", "B_FIRST C_BODY", "C_EVEN C_NEXT"}},
		{"empty-parity", []*Section{controlSection("A", "", p("A_BODY")), controlSection("B", SectionOddPage), controlSection("C", SectionEvenPage, p("C_BODY"))}, []string{"A_FIRST A_BODY", "", "B_FIRST", "C_FIRST C_BODY"}},
		{"ending-empty", []*Section{controlSection("A", "", p("A_BODY"), br, p("A_NEXT")), controlSection("B", SectionContinuous), controlSection("C", SectionContinuous)}, []string{"A_FIRST A_BODY", "A_EVEN A_NEXT"}},
		{"control-only", []*Section{controlSection("A", "", p("A_BODY")), controlSection("B", SectionContinuous, br, br), controlSection("C", SectionContinuous, p("C_BODY"), br, p("C_NEXT"))}, []string{"A_FIRST A_BODY", "B_EVEN", "B_DEFAULT C_BODY", "C_EVEN C_NEXT"}},
		{"conditional", []*Section{controlSection("A", ""), controlSection("B", SectionContinuous, before("B_BODY"), before("B_NEXT")), controlSection("C", SectionContinuous, before("C_BODY"))}, []string{"A_FIRST B_BODY", "B_EVEN B_NEXT", "C_EVEN C_BODY"}},
	}
}
func controlDocument(sections []*Section) *Document {
	d := &Document{Layout: Layout{Width: 300, Height: 400, Margin: 30}, EvenAndOddHeaders: true}
	for _, s := range sections {
		d.Blocks = append(d.Blocks, Block{Kind: SectionBlock, Section: s})
	}
	return d
}
func assertControlPages(t *testing.T, d *Document, want []string) *Document {
	t.Helper()
	before := canonicalSF(t, d)
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, PDF, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	back, err := ReadPDF(context.Background(), &out, ReadOptions{PDFSkipInference: true, PDFSkipStructure: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(back.Pages) != len(want) {
		t.Fatalf("pages=%d want=%d; text=%q", len(back.Pages), len(want), back.Text())
	}
	texts := make([]string, len(want))
	for _, b := range back.Blocks {
		if b.Source != nil && len(b.Source.Regions) > 0 {
			texts[b.Source.Regions[0].Page-1] += (&Document{Blocks: []Block{b}}).Text() + "\n"
		}
	}
	for page, expected := range want {
		tokens := strings.Fields(expected)
		var bodyTokens []string
		if len(tokens) > 0 {
			bodyTokens = tokens[1:]
		}
		head, foot := "", ""
		if len(tokens) > 0 {
			owners := strings.Split(tokens[0], "/")
			if len(owners) == 1 {
				owners = append(owners, owners[0])
			}
			for i, role := range []string{"HEAD", "FOOT"} {
				if owners[i] == "-" {
					continue
				}
				bits := strings.SplitN(owners[i], "_", 2)
				if i == 0 {
					head = bits[0] + "_" + role + "_" + bits[1]
				} else {
					foot = bits[0] + "_" + role + "_" + bits[1]
				}
			}
		}
		for _, id := range []string{"A", "B", "C"} {
			for _, role := range []string{"HEAD", "FOOT"} {
				for _, variant := range []string{"FIRST", "EVEN", "DEFAULT"} {
					token := id + "_" + role + "_" + variant
					count := 0
					if token == head || token == foot {
						count = 1
					}
					if strings.Count(texts[page], token) != count {
						t.Fatalf("page %d owner=%s want=%s; text=%q", page+1, token, expected, texts[page])
					}
				}
			}
		}
		for _, s := range d.Blocks {
			for _, block := range s.Section.Blocks {
				for _, run := range block.Runs {
					if strings.Contains(run.Text, "_BODY") || strings.Contains(run.Text, "_NEXT") {
						expectedCount := 0
						for _, token := range bodyTokens {
							if token == run.Text {
								expectedCount++
							}
						}
						if strings.Count(texts[page], run.Text) != expectedCount {
							t.Fatalf("page %d body %s: %q", page+1, run.Text, texts[page])
						}
					}
				}
			}
		}
		if expected == "" && strings.TrimSpace(texts[page]) != "" {
			t.Fatal("parity filler must be completely blank", texts[page])
		}
	}
	if !bytes.Equal(before, canonicalSF(t, d)) {
		t.Fatal("source controls/section settings mutated")
	}
	checkParagraphRoundtrip(t, d)
	return back
}
func TestPDFSectionControlOwnership(t *testing.T) {
	for _, c := range sectionControlCases() {
		t.Run(c.name, func(t *testing.T) { assertControlPages(t, controlDocument(c.sections), c.pages) })
	}
}
func TestPDFSectionControlsDrainNotesBeforeBoundary(t *testing.T) {
	br := Block{Kind: PageBreak}
	a := controlSection("A", "", sectionParagraph("A_BODY"), br)
	a.Blocks[0].Runs = append(a.Blocks[0].Runs, Run{Note: &NoteReference{ID: "old", Kind: Footnote}})
	b := controlSection("B", SectionContinuous, br, sectionParagraph("B_BODY"))
	c := controlSection("C", SectionOddPage, sectionParagraph("C_BODY"))
	d := controlDocument([]*Section{a, b, c})
	var lines []Block
	for i := 0; i < 32; i++ {
		lines = append(lines, sectionParagraph(fmt.Sprintf("NOTE_%02d", i)))
	}
	d.Notes = []Note{{ID: "old", Kind: Footnote, Blocks: lines}}
	back := assertControlPages(t, d, []string{"A_FIRST A_BODY", "A_EVEN", "A_DEFAULT", "B_EVEN B_BODY", "C_FIRST C_BODY"})
	for i := 0; i < 32; i++ {
		if strings.Count(back.Text(), fmt.Sprintf("NOTE_%02d", i)) != 1 {
			t.Fatal("note content lost/repeated", back.Text())
		}
	}
	for _, block := range back.Blocks {
		for _, run := range block.Runs {
			if strings.HasPrefix(run.Text, "NOTE_") && run.Source.Page > 2 {
				t.Fatal("notes crossed explicit control boundary", run.Source)
			}
		}
	}
}

func TestPDFSectionControlIndependentInheritance(t *testing.T) {
	a := controlSection("A", "", sectionParagraph("A_BODY"))
	b := &Section{Start: SectionContinuous, FirstPage: true, Footers: RunningStories{Even: &RunningStory{}}, Blocks: []Block{{Kind: PageBreak}, sectionParagraph("B_BODY")}}
	c := &Section{Start: SectionNextPage, FirstPage: true, Headers: RunningStories{First: &RunningStory{}}, Blocks: []Block{sectionParagraph("C_BODY")}}
	assertControlPages(t, controlDocument([]*Section{a, b, c}), []string{"A_FIRST A_BODY", "A_EVEN/- B_BODY", "-/A_FIRST C_BODY"})
}
func TestPDFEmptyPendingSectionKeepsParityCanvasAndFooterOwner(t *testing.T) {
	a := controlSection("A", "", sectionParagraph("A_BODY"))
	b := controlSection("B", SectionContinuous)
	b.Layout = &Layout{Width: 300, Height: 400, Margins: &PageMargins{Top: 40, Right: 20, Bottom: 45, Left: 90}}
	c := controlSection("C", SectionOddPage, sectionParagraph("C_BODY"), Block{Kind: PageBreak}, sectionParagraph("C_NEXT"))
	c.Layout = &Layout{Width: 400, Height: 300, Margin: 20}
	back := assertControlPages(t, controlDocument([]*Section{a, b, c}), []string{"A_FIRST A_BODY", "", "C_FIRST C_BODY", "C_EVEN C_NEXT"})
	for i, p := range back.Pages {
		w, h := 300.0, 400.0
		if i >= 2 {
			w, h = 400, 300
		}
		if p.Width != w || p.Height != h {
			t.Fatal("parity/empty pending section changed canvas owner", i+1, p)
		}
	}
	for _, b := range back.Blocks {
		for _, run := range b.Runs {
			if run.Text == "A_FOOT_FIRST" && (run.Source.Baseline == nil || run.Source.Baseline[0].X != 30) {
				t.Fatal("empty pending section moved old footer", run.Source)
			}
		}
	}
}
func TestPDFEmptyContinuousFinalOwnerBindsEndnotes(t *testing.T) {
	a := controlSection("A", "", sectionParagraph("A_BODY"), Block{Kind: PageBreak}, sectionParagraph("A_NEXT"))
	a.Blocks[0].Runs = append(a.Blocks[0].Runs, Run{Note: &NoteReference{Kind: Endnote, ID: "ending"}})
	b := controlSection("B", SectionContinuous)
	c := controlSection("C", SectionContinuous)
	c.Layout = &Layout{Width: 300, Height: 400, Margins: &PageMargins{Top: 60, Right: 20, Bottom: 50, Left: 70}}
	d := controlDocument([]*Section{a, b, c})
	d.Notes = []Note{{Kind: Endnote, ID: "ending", Blocks: []Block{sectionParagraph("ENDNOTE_CONTENT")}}}
	back := assertControlPages(t, d, []string{"A_FIRST A_BODY", "A_EVEN A_NEXT", "C_EVEN"})
	found := false
	for _, b := range back.Blocks {
		for _, run := range b.Runs {
			if run.Text == "ENDNOTE_CONTENT" {
				found = true
				if run.Source.Page != 3 || run.Source.Baseline == nil || run.Source.Baseline[0].X < 70 || run.Source.Baseline[0].Y < 60 {
					t.Fatal("endnote left final empty continuous frame", run.Source)
				}
			}
		}
	}
	if !found {
		t.Fatal("empty final owner discarded endnote", back.Text())
	}
}

func TestSectionedDOCXDeclaresGlobalNoteNumberingInEverySection(t *testing.T) {
	for _, even := range []bool{false, true} {
		t.Run(fmt.Sprint(even), func(t *testing.T) {
			a, b := controlSection("A", "", sectionParagraph("A_BODY")), controlSection("B", SectionNextPage, sectionParagraph("B_BODY"))
			d := controlDocument([]*Section{a, b, controlSection("C", SectionContinuous)})
			d.EvenAndOddHeaders = even
			for i, s := range []*Section{a, b} {
				for _, kind := range []string{Footnote, Endnote} {
					id := fmt.Sprintf("%s-%d", kind, i)
					s.Blocks[0].Runs = append(s.Blocks[0].Runs, Run{Note: &NoteReference{Kind: kind, ID: id}})
					d.Notes = append(d.Notes, Note{Kind: kind, ID: id, Blocks: []Block{sectionParagraph(id + "_CONTENT")}})
				}
			}
			checkParagraphRoundtrip(t, d)
			var out bytes.Buffer
			if err := Write(context.Background(), &out, d, DOCX, WriteOptions{}); err != nil {
				t.Fatal(err)
			}
			archive, err := zip.NewReader(bytes.NewReader(out.Bytes()), int64(out.Len()))
			if err != nil {
				t.Fatal(err)
			}
			var body []byte
			for _, file := range archive.File {
				if file.Name == "word/document.xml" {
					r, err := file.Open()
					if err != nil {
						t.Fatal(err)
					}
					body, err = io.ReadAll(r)
					r.Close()
					if err != nil {
						t.Fatal(err)
					}
				}
			}
			var document struct {
				Body struct {
					Paragraphs []struct {
						Properties struct {
							Section *numberedSection `xml:"sectPr"`
						} `xml:"pPr"`
					} `xml:"p"`
					Final *numberedSection `xml:"sectPr"`
				} `xml:"body"`
			}
			if err := xml.Unmarshal(body, &document); err != nil {
				t.Fatal(err)
			}
			var sections []*numberedSection
			for _, p := range document.Body.Paragraphs {
				if p.Properties.Section != nil {
					sections = append(sections, p.Properties.Section)
				}
			}
			sections = append(sections, document.Body.Final)
			if len(sections) != 3 {
				t.Fatal("section properties lost", len(sections))
			}
			for _, s := range sections {
				if s == nil {
					t.Fatal("missing final section")
				}
				for _, p := range []numberedNote{s.Footnote, s.Endnote} {
					if p.Format.Value != "decimal" || p.Restart.Value != "continuous" {
						t.Fatal("section falls back to producer note policy", p)
					}
				}
			}
		})
	}
}

type numberedNote struct {
	Format struct {
		Value string `xml:"val,attr"`
	} `xml:"numFmt"`
	Restart struct {
		Value string `xml:"val,attr"`
	} `xml:"numRestart"`
}
type numberedSection struct {
	Footnote numberedNote `xml:"footnotePr"`
	Endnote  numberedNote `xml:"endnotePr"`
}
