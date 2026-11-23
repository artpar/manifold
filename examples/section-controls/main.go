// Four conversion paths for page controls, empty sections and note ownership.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/artpar/manydoc"
	"log"
	"os"
	"path/filepath"
	"sort"
)

func sectionParagraph(s string) manydoc.Block {
	return manydoc.Block{Kind: manydoc.Paragraph, Runs: []manydoc.Run{{Text: s}}}
}
func sectionStory(s string) *manydoc.RunningStory {
	return &manydoc.RunningStory{Blocks: []manydoc.Block{sectionParagraph(s)}}
}
func controlSection(id string, start manydoc.SectionStart, blocks ...manydoc.Block) *manydoc.Section {
	stories := func(role string) manydoc.RunningStories {
		return manydoc.RunningStories{Default: sectionStory(id + "_" + role + "_DEFAULT"), First: sectionStory(id + "_" + role + "_FIRST"), Even: sectionStory(id + "_" + role + "_EVEN")}
	}
	return &manydoc.Section{Start: start, FirstPage: true, Headers: stories("HEAD"), Footers: stories("FOOT"), Blocks: blocks}
}

type controlCase struct {
	name     string
	sections []*manydoc.Section
	pages    []string
}

func sectionControlCases() []controlCase {
	p := sectionParagraph
	br := manydoc.Block{Kind: manydoc.PageBreak}
	before := func(s string) manydoc.Block { b := p(s); b.PageBreakBefore = true; return b }
	return []controlCase{
		{"leading", []*manydoc.Section{controlSection("A", manydoc.SectionEvenPage, br, br, p("A_BODY"))}, []string{"A_FIRST", "A_EVEN", "A_DEFAULT A_BODY"}},
		{"adjacent", []*manydoc.Section{controlSection("A", "", p("A_BODY"), br), controlSection("B", manydoc.SectionContinuous, br, p("B_BODY"))}, []string{"A_FIRST A_BODY", "A_EVEN", "B_EVEN B_BODY"}},
		{"trailing-next", []*manydoc.Section{controlSection("A", "", p("A_BODY"), br), controlSection("B", manydoc.SectionNextPage, p("B_BODY"))}, []string{"A_FIRST A_BODY", "A_EVEN", "B_FIRST B_BODY"}},
		{"empty-continuous", []*manydoc.Section{controlSection("A", ""), controlSection("B", manydoc.SectionContinuous), controlSection("C", manydoc.SectionContinuous, p("C_BODY"), br, p("C_NEXT"))}, []string{"A_FIRST C_BODY", "C_EVEN C_NEXT"}},
		{"empty-next", []*manydoc.Section{controlSection("A", "", p("A_BODY")), controlSection("B", manydoc.SectionNextPage), controlSection("C", manydoc.SectionContinuous, p("C_BODY"), br, p("C_NEXT"))}, []string{"A_FIRST A_BODY", "B_FIRST C_BODY", "C_EVEN C_NEXT"}},
		{"empty-parity", []*manydoc.Section{controlSection("A", "", p("A_BODY")), controlSection("B", manydoc.SectionOddPage), controlSection("C", manydoc.SectionEvenPage, p("C_BODY"))}, []string{"A_FIRST A_BODY", "", "B_FIRST", "C_FIRST C_BODY"}},
		{"ending-empty", []*manydoc.Section{controlSection("A", "", p("A_BODY"), br, p("A_NEXT")), controlSection("B", manydoc.SectionContinuous), controlSection("C", manydoc.SectionContinuous)}, []string{"A_FIRST A_BODY", "A_EVEN A_NEXT"}},
		{"control-only", []*manydoc.Section{controlSection("A", "", p("A_BODY")), controlSection("B", manydoc.SectionContinuous, br, br), controlSection("C", manydoc.SectionContinuous, p("C_BODY"), br, p("C_NEXT"))}, []string{"A_FIRST A_BODY", "B_EVEN", "B_DEFAULT C_BODY", "C_EVEN C_NEXT"}},
		{"conditional", []*manydoc.Section{controlSection("A", ""), controlSection("B", manydoc.SectionContinuous, before("B_BODY"), before("B_NEXT")), controlSection("C", manydoc.SectionContinuous, before("C_BODY"))}, []string{"A_FIRST B_BODY", "B_EVEN B_NEXT", "C_EVEN C_BODY"}},
	}
}
func controlDocument(sections []*manydoc.Section) *manydoc.Document {
	d := &manydoc.Document{Layout: manydoc.Layout{Width: 300, Height: 400, Margin: 30}, EvenAndOddHeaders: true}
	for _, s := range sections {
		d.Blocks = append(d.Blocks, manydoc.Block{Kind: manydoc.SectionBlock, Section: s})
	}
	return d
}

func main() {
	ctx := context.Background()
	dir := "output/section-controls"
	must(os.MkdirAll(dir, 0755))
	docs := map[string]*manydoc.Document{}
	expected := map[string][]string{}
	for _, c := range sectionControlCases() {
		docs[c.name] = controlDocument(c.sections)
		expected[c.name] = c.pages
	}
	p := sectionParagraph
	br := manydoc.Block{Kind: manydoc.PageBreak}
	a := controlSection("A", "", p("A_BODY"), br)
	a.Blocks[0].Runs = append(a.Blocks[0].Runs, manydoc.Run{Note: &manydoc.NoteReference{Kind: manydoc.Footnote, ID: "old"}})
	b := controlSection("B", manydoc.SectionContinuous, br, p("B_BODY"))
	c := controlSection("C", manydoc.SectionOddPage, p("C_BODY"))
	d := controlDocument([]*manydoc.Section{a, b, c})
	var lines []manydoc.Block
	for i := 0; i < 32; i++ {
		lines = append(lines, p(fmt.Sprintf("NOTE_%02d", i)))
	}
	d.Notes = []manydoc.Note{{Kind: manydoc.Footnote, ID: "old", Blocks: lines}}
	docs["note-controls"] = d
	expected["note-controls"] = []string{"A_FIRST A_BODY", "A_EVEN", "A_DEFAULT", "B_EVEN B_BODY", "C_FIRST C_BODY"}
	a = controlSection("A", "", p("A_BODY"))
	b = &manydoc.Section{Start: manydoc.SectionContinuous, FirstPage: true, Footers: manydoc.RunningStories{Even: &manydoc.RunningStory{}}, Blocks: []manydoc.Block{br, p("B_BODY")}}
	c = &manydoc.Section{Start: manydoc.SectionNextPage, FirstPage: true, Headers: manydoc.RunningStories{First: &manydoc.RunningStory{}}, Blocks: []manydoc.Block{p("C_BODY")}}
	docs["independent-inheritance"] = controlDocument([]*manydoc.Section{a, b, c})
	expected["independent-inheritance"] = []string{"A_FIRST A_BODY", "A_EVEN/- B_BODY", "-/A_FIRST C_BODY"}
	a = controlSection("A", "", p("A_BODY"))
	b = controlSection("B", manydoc.SectionContinuous)
	b.Layout = &manydoc.Layout{Width: 300, Height: 400, Margins: &manydoc.PageMargins{Top: 40, Right: 20, Bottom: 45, Left: 90}}
	c = controlSection("C", manydoc.SectionOddPage, p("C_BODY"), br, p("C_NEXT"))
	c.Layout = &manydoc.Layout{Width: 400, Height: 300, Margin: 20}
	docs["empty-parity-canvas"] = controlDocument([]*manydoc.Section{a, b, c})
	expected["empty-parity-canvas"] = []string{"A_FIRST A_BODY", "", "C_FIRST C_BODY", "C_EVEN C_NEXT"}
	a = controlSection("A", "", p("A_BODY"), br, p("A_NEXT"))
	a.Blocks[0].Runs = append(a.Blocks[0].Runs, manydoc.Run{Note: &manydoc.NoteReference{Kind: manydoc.Endnote, ID: "ending"}})
	b = controlSection("B", manydoc.SectionContinuous)
	c = controlSection("C", manydoc.SectionContinuous)
	c.Layout = &manydoc.Layout{Width: 300, Height: 400, Margins: &manydoc.PageMargins{Top: 60, Right: 20, Bottom: 50, Left: 70}}
	d = controlDocument([]*manydoc.Section{a, b, c})
	d.Notes = []manydoc.Note{{Kind: manydoc.Endnote, ID: "ending", Blocks: []manydoc.Block{p("ENDNOTE_CONTENT")}}}
	docs["empty-final-endnote"] = d
	expected["empty-final-endnote"] = []string{"A_FIRST A_BODY", "A_EVEN A_NEXT", "C_EVEN"}
	a = controlSection("A", "", p("A_BODY"))
	b = controlSection("B", manydoc.SectionNextPage, p("B_BODY"))
	c = controlSection("C", manydoc.SectionContinuous)
	d = controlDocument([]*manydoc.Section{a, b, c})
	for i, s := range []*manydoc.Section{a, b} {
		for _, kind := range []string{manydoc.Footnote, manydoc.Endnote} {
			id := fmt.Sprintf("%s-%d", kind, i)
			s.Blocks[0].Runs = append(s.Blocks[0].Runs, manydoc.Run{Note: &manydoc.NoteReference{Kind: kind, ID: id}})
			d.Notes = append(d.Notes, manydoc.Note{Kind: kind, ID: id, Blocks: []manydoc.Block{p(id + "_CONTENT")}})
		}
	}
	docs["global-numbering"] = d
	expected["global-numbering"] = []string{"A_FIRST A_BODY", "B_FIRST B_BODY", "C_EVEN"}
	names := make([]string, 0, len(docs))
	for name := range docs {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		d := docs[name]
		d.Metadata.Title = "Section controls: " + name
		for _, container := range d.Blocks {
			s := container.Section
			for i := range s.Blocks {
				for j := range s.Blocks[i].Runs {
					run := &s.Blocks[i].Runs[j]
					if run.Text != "" {
						run.Link = "https://example.com/controls/" + run.Text
					}
				}
			}
		}
		for _, f := range []struct {
			suffix string
			format manydoc.Format
		}{{".sf", manydoc.SF}, {".docx", manydoc.DOCX}, {".pdf", manydoc.PDF}} {
			var buf bytes.Buffer
			must(manydoc.Write(ctx, &buf, d, f.format, manydoc.WriteOptions{}))
			must(os.WriteFile(filepath.Join(dir, name+f.suffix), buf.Bytes(), 0644))
		}
		for _, f := range []struct {
			suffix, out string
			format      manydoc.Format
		}{{".docx", "-docx.sf", manydoc.DOCX}, {".pdf", "-pdf.sf", manydoc.PDF}} {
			data, err := os.ReadFile(filepath.Join(dir, name+f.suffix))
			must(err)
			back, err := manydoc.Read(ctx, bytes.NewReader(data), f.format, manydoc.ReadOptions{PDFSkipInference: true})
			must(err)
			var buf bytes.Buffer
			must(manydoc.Write(ctx, &buf, back, manydoc.SF, manydoc.WriteOptions{}))
			must(os.WriteFile(filepath.Join(dir, name+f.out), buf.Bytes(), 0644))
		}
		source, err := os.ReadFile(filepath.Join(dir, name+".sf"))
		must(err)
		back, err := os.ReadFile(filepath.Join(dir, name+"-docx.sf"))
		must(err)
		if !bytes.Equal(source, back) {
			log.Fatal("section/control/note semantics changed: ", name)
		}
	}
	data, err := json.MarshalIndent(expected, "", "  ")
	must(err)
	must(os.WriteFile(filepath.Join(dir, "expected.json"), append(data, '\n'), 0644))
	fmt.Println("Wrote fourteen control/empty-section SF/DOCX/PDF cases and recovered SF to", dir)
}
func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
