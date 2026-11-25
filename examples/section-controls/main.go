// Four conversion paths for page controls, empty sections and note ownership.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/artpar/manifold"
	"log"
	"os"
	"path/filepath"
	"sort"
)

func sectionParagraph(s string) manifold.Block {
	return manifold.Block{Kind: manifold.Paragraph, Runs: []manifold.Run{{Text: s}}}
}
func sectionStory(s string) *manifold.RunningStory {
	return &manifold.RunningStory{Blocks: []manifold.Block{sectionParagraph(s)}}
}
func controlSection(id string, start manifold.SectionStart, blocks ...manifold.Block) *manifold.Section {
	stories := func(role string) manifold.RunningStories {
		return manifold.RunningStories{Default: sectionStory(id + "_" + role + "_DEFAULT"), First: sectionStory(id + "_" + role + "_FIRST"), Even: sectionStory(id + "_" + role + "_EVEN")}
	}
	return &manifold.Section{Start: start, FirstPage: true, Headers: stories("HEAD"), Footers: stories("FOOT"), Blocks: blocks}
}

type controlCase struct {
	name     string
	sections []*manifold.Section
	pages    []string
}

func sectionControlCases() []controlCase {
	p := sectionParagraph
	br := manifold.Block{Kind: manifold.PageBreak}
	before := func(s string) manifold.Block { b := p(s); b.PageBreakBefore = true; return b }
	return []controlCase{
		{"leading", []*manifold.Section{controlSection("A", manifold.SectionEvenPage, br, br, p("A_BODY"))}, []string{"A_FIRST", "A_EVEN", "A_DEFAULT A_BODY"}},
		{"adjacent", []*manifold.Section{controlSection("A", "", p("A_BODY"), br), controlSection("B", manifold.SectionContinuous, br, p("B_BODY"))}, []string{"A_FIRST A_BODY", "A_EVEN", "B_EVEN B_BODY"}},
		{"trailing-next", []*manifold.Section{controlSection("A", "", p("A_BODY"), br), controlSection("B", manifold.SectionNextPage, p("B_BODY"))}, []string{"A_FIRST A_BODY", "A_EVEN", "B_FIRST B_BODY"}},
		{"empty-continuous", []*manifold.Section{controlSection("A", ""), controlSection("B", manifold.SectionContinuous), controlSection("C", manifold.SectionContinuous, p("C_BODY"), br, p("C_NEXT"))}, []string{"A_FIRST C_BODY", "C_EVEN C_NEXT"}},
		{"empty-next", []*manifold.Section{controlSection("A", "", p("A_BODY")), controlSection("B", manifold.SectionNextPage), controlSection("C", manifold.SectionContinuous, p("C_BODY"), br, p("C_NEXT"))}, []string{"A_FIRST A_BODY", "B_FIRST C_BODY", "C_EVEN C_NEXT"}},
		{"empty-parity", []*manifold.Section{controlSection("A", "", p("A_BODY")), controlSection("B", manifold.SectionOddPage), controlSection("C", manifold.SectionEvenPage, p("C_BODY"))}, []string{"A_FIRST A_BODY", "", "B_FIRST", "C_FIRST C_BODY"}},
		{"ending-empty", []*manifold.Section{controlSection("A", "", p("A_BODY"), br, p("A_NEXT")), controlSection("B", manifold.SectionContinuous), controlSection("C", manifold.SectionContinuous)}, []string{"A_FIRST A_BODY", "A_EVEN A_NEXT"}},
		{"control-only", []*manifold.Section{controlSection("A", "", p("A_BODY")), controlSection("B", manifold.SectionContinuous, br, br), controlSection("C", manifold.SectionContinuous, p("C_BODY"), br, p("C_NEXT"))}, []string{"A_FIRST A_BODY", "B_EVEN", "B_DEFAULT C_BODY", "C_EVEN C_NEXT"}},
		{"conditional", []*manifold.Section{controlSection("A", ""), controlSection("B", manifold.SectionContinuous, before("B_BODY"), before("B_NEXT")), controlSection("C", manifold.SectionContinuous, before("C_BODY"))}, []string{"A_FIRST B_BODY", "B_EVEN B_NEXT", "C_EVEN C_BODY"}},
	}
}
func controlDocument(sections []*manifold.Section) *manifold.Document {
	d := &manifold.Document{Layout: manifold.Layout{Width: 300, Height: 400, Margin: 30}, EvenAndOddHeaders: true}
	for _, s := range sections {
		d.Blocks = append(d.Blocks, manifold.Block{Kind: manifold.SectionBlock, Section: s})
	}
	return d
}

func main() {
	ctx := context.Background()
	dir := "output/section-controls"
	must(os.MkdirAll(dir, 0755))
	docs := map[string]*manifold.Document{}
	expected := map[string][]string{}
	for _, c := range sectionControlCases() {
		docs[c.name] = controlDocument(c.sections)
		expected[c.name] = c.pages
	}
	p := sectionParagraph
	br := manifold.Block{Kind: manifold.PageBreak}
	a := controlSection("A", "", p("A_BODY"), br)
	a.Blocks[0].Runs = append(a.Blocks[0].Runs, manifold.Run{Note: &manifold.NoteReference{Kind: manifold.Footnote, ID: "old"}})
	b := controlSection("B", manifold.SectionContinuous, br, p("B_BODY"))
	c := controlSection("C", manifold.SectionOddPage, p("C_BODY"))
	d := controlDocument([]*manifold.Section{a, b, c})
	var lines []manifold.Block
	for i := 0; i < 32; i++ {
		lines = append(lines, p(fmt.Sprintf("NOTE_%02d", i)))
	}
	d.Notes = []manifold.Note{{Kind: manifold.Footnote, ID: "old", Blocks: lines}}
	docs["note-controls"] = d
	expected["note-controls"] = []string{"A_FIRST A_BODY", "A_EVEN", "A_DEFAULT", "B_EVEN B_BODY", "C_FIRST C_BODY"}
	a = controlSection("A", "", p("A_BODY"))
	b = &manifold.Section{Start: manifold.SectionContinuous, FirstPage: true, Footers: manifold.RunningStories{Even: &manifold.RunningStory{}}, Blocks: []manifold.Block{br, p("B_BODY")}}
	c = &manifold.Section{Start: manifold.SectionNextPage, FirstPage: true, Headers: manifold.RunningStories{First: &manifold.RunningStory{}}, Blocks: []manifold.Block{p("C_BODY")}}
	docs["independent-inheritance"] = controlDocument([]*manifold.Section{a, b, c})
	expected["independent-inheritance"] = []string{"A_FIRST A_BODY", "A_EVEN/- B_BODY", "-/A_FIRST C_BODY"}
	a = controlSection("A", "", p("A_BODY"))
	b = controlSection("B", manifold.SectionContinuous)
	b.Layout = &manifold.Layout{Width: 300, Height: 400, Margins: &manifold.PageMargins{Top: 40, Right: 20, Bottom: 45, Left: 90}}
	c = controlSection("C", manifold.SectionOddPage, p("C_BODY"), br, p("C_NEXT"))
	c.Layout = &manifold.Layout{Width: 400, Height: 300, Margin: 20}
	docs["empty-parity-canvas"] = controlDocument([]*manifold.Section{a, b, c})
	expected["empty-parity-canvas"] = []string{"A_FIRST A_BODY", "", "C_FIRST C_BODY", "C_EVEN C_NEXT"}
	a = controlSection("A", "", p("A_BODY"), br, p("A_NEXT"))
	a.Blocks[0].Runs = append(a.Blocks[0].Runs, manifold.Run{Note: &manifold.NoteReference{Kind: manifold.Endnote, ID: "ending"}})
	b = controlSection("B", manifold.SectionContinuous)
	c = controlSection("C", manifold.SectionContinuous)
	c.Layout = &manifold.Layout{Width: 300, Height: 400, Margins: &manifold.PageMargins{Top: 60, Right: 20, Bottom: 50, Left: 70}}
	d = controlDocument([]*manifold.Section{a, b, c})
	d.Notes = []manifold.Note{{Kind: manifold.Endnote, ID: "ending", Blocks: []manifold.Block{p("ENDNOTE_CONTENT")}}}
	docs["empty-final-endnote"] = d
	expected["empty-final-endnote"] = []string{"A_FIRST A_BODY", "A_EVEN A_NEXT", "C_EVEN"}
	a = controlSection("A", "", p("A_BODY"))
	b = controlSection("B", manifold.SectionNextPage, p("B_BODY"))
	c = controlSection("C", manifold.SectionContinuous)
	d = controlDocument([]*manifold.Section{a, b, c})
	for i, s := range []*manifold.Section{a, b} {
		for _, kind := range []string{manifold.Footnote, manifold.Endnote} {
			id := fmt.Sprintf("%s-%d", kind, i)
			s.Blocks[0].Runs = append(s.Blocks[0].Runs, manifold.Run{Note: &manifold.NoteReference{Kind: kind, ID: id}})
			d.Notes = append(d.Notes, manifold.Note{Kind: kind, ID: id, Blocks: []manifold.Block{p(id + "_CONTENT")}})
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
			format manifold.Format
		}{{".sf", manifold.SF}, {".docx", manifold.DOCX}, {".pdf", manifold.PDF}} {
			var buf bytes.Buffer
			must(manifold.Write(ctx, &buf, d, f.format, manifold.WriteOptions{}))
			must(os.WriteFile(filepath.Join(dir, name+f.suffix), buf.Bytes(), 0644))
		}
		for _, f := range []struct {
			suffix, out string
			format      manifold.Format
		}{{".docx", "-docx.sf", manifold.DOCX}, {".pdf", "-pdf.sf", manifold.PDF}} {
			data, err := os.ReadFile(filepath.Join(dir, name+f.suffix))
			must(err)
			back, err := manifold.Read(ctx, bytes.NewReader(data), f.format, manifold.ReadOptions{PDFSkipInference: true})
			must(err)
			var buf bytes.Buffer
			must(manifold.Write(ctx, &buf, back, manifold.SF, manifold.WriteOptions{}))
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
