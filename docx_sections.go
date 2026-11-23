package manydoc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strings"
)

func writeSectionedDOCX(ctx context.Context, out io.Writer, d *Document) error {
	notes, err := catalogNotes(d, true)
	if err != nil {
		return err
	}
	w := &docxWriter{ctx: ctx, parts: map[string][]byte{}, next: 1, validatedImages: make(outputImageCache), noteIDs: map[noteKey]int{}, noteLabels: notes.labels, seenNoteRefs: map[noteKey]bool{}}
	counts := map[string]int{}
	for _, note := range d.Notes {
		counts[note.Kind]++
		w.noteIDs[noteIdentity(note)] = counts[note.Kind]
	}
	w.addRel("styles", "styles.xml", false)
	w.addRel("numbering", "numbering.xml", false)
	resolved := documentSections(d)
	var body strings.Builder
	storyIndex := map[string]int{}
	noteWidth, noteHeight := math.Inf(1), math.Inf(1)
	for index, block := range d.Blocks {
		s := block.Section
		r := resolved[index]
		g := pageLayout(r.layout)
		if err := validateDOCXPageGeometry(g); err != nil {
			return err
		}
		w.contentWidth, w.contentHeight = g.contentWidth(), g.contentHeight()
		cap, bodyHeight, err := w.fitSectionStories(d, r)
		if err != nil {
			return err
		}
		w.contentHeight = bodyHeight
		noteWidth, noteHeight = math.Min(noteWidth, w.contentWidth), math.Min(noteHeight, bodyHeight)
		part, err := w.blocks(s.Blocks)
		if err != nil {
			return err
		}
		var props strings.Builder
		props.WriteString(`<w:sectPr md:sectionModel="1"`)
		if index == 0 {
			base, _ := json.Marshal(d.Layout)
			props.WriteString(` md:sectionBase="` + escape(string(base)) + `"`)
		}
		if s.Layout == nil {
			props.WriteString(` md:sectionLayout="inherit"`)
		} else {
			raw, _ := json.Marshal(s.Layout)
			props.WriteString(` md:sectionLayout="` + escape(string(raw)) + `"`)
		}
		if s.Start == "" {
			props.WriteString(` md:sectionStartImplicit="1"`)
		}
		props.WriteString(`>`)
		for _, role := range []struct {
			kind, root string
			stories    RunningStories
		}{{"header", "hdr", s.Headers}, {"footer", "ftr", s.Footers}} {
			for _, variant := range []struct {
				name  string
				story *RunningStory
			}{{"default", role.stories.Default}, {"first", role.stories.First}, {"even", role.stories.Even}} {
				if variant.story == nil {
					continue
				}
				storyIndex[role.kind]++
				name := fmt.Sprintf("%s%d.xml", role.kind, storyIndex[role.kind])
				main := w.rels
				w.rels = nil
				storyCap := cap
				header := role.kind == "header"
				if header && g.margins.Top < 0 || !header && g.margins.Bottom < 0 {
					storyCap, err = w.fitFixedRunning(variant.story.Blocks, g, header)
					if err != nil {
						return err
					}
				}
				w.contentHeight, w.runningImageHeight = g.runningStoryHeight(header, true, true), storyCap
				xml, err := w.blocks(variant.story.Blocks)
				w.contentHeight, w.runningImageHeight = bodyHeight, 0
				if err != nil {
					return err
				}
				if len(variant.story.Blocks) == 0 {
					xml = `<w:p md:emptyStory="1"/>`
				}
				w.parts["word/"+name] = []byte(xmlHeader + `<w:` + role.root + wordNamespaces + docxMetadataNamespaces + `>` + xml + `</w:` + role.root + `>`)
				if len(w.rels) > 0 {
					w.parts["word/_rels/"+name+".rels"] = []byte(xmlHeader + `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` + strings.Join(w.rels, "") + `</Relationships>`)
				}
				w.rels = main
				id := w.addRel(role.kind, name, false)
				fmt.Fprintf(&props, `<w:%sReference w:type="%s" r:id="%s"/>`, role.kind, variant.name, id)
			}
		}
		start := s.Start
		if start == "" {
			start = SectionNextPage
		}
		fmt.Fprintf(&props, `<w:type w:val="%s"/>`, start)
		orientation := ""
		if g.width > g.height {
			orientation = ` w:orient="landscape"`
		}
		fmt.Fprintf(&props, `<w:pgSz w:w="%.0f" w:h="%.0f"%s/>`, g.width*20, g.height*20, orientation)
		props.WriteString(docxPageMargins(r.layout))
		if s.FirstPage {
			props.WriteString(`<w:titlePg/>`)
		}
		props.WriteString(`</w:sectPr>`)
		if index == len(d.Blocks)-1 {
			body.WriteString(part)
			body.WriteString(props.String())
		} else {
			// Attach to a real final paragraph when possible. Tables/lists require
			// a structural paragraph; its checked marker never hides visible content.
			if len(s.Blocks) > 0 && (s.Blocks[len(s.Blocks)-1].Kind == Paragraph || s.Blocks[len(s.Blocks)-1].Kind == Heading) {
				last := strings.LastIndex(part, "<w:pPr>")
				if last < 0 {
					return fmt.Errorf("missing DOCX section paragraph")
				}
				end := strings.Index(part[last:], "</w:pPr>")
				if end < 0 {
					return fmt.Errorf("missing section paragraph properties")
				}
				at := last + end
				part = part[:at] + props.String() + part[at:]
				body.WriteString(part)
			} else {
				body.WriteString(part)
				body.WriteString(`<w:p md:sectionTail="1"><w:pPr>` + props.String() + `</w:pPr></w:p>`)
			}
		}
	}
	w.contentWidth, w.contentHeight = noteWidth, noteHeight
	if err := w.writeNoteParts(d.Notes); err != nil {
		return err
	}
	if d.EvenAndOddHeaders {
		w.parts["word/settings.xml"] = []byte(xmlHeader + `<w:settings xmlns:w="` + wordNS + `"><w:evenAndOddHeaders/></w:settings>`)
		w.addRel("settings", "settings.xml", false)
	}
	w.parts["word/document.xml"] = []byte(xmlHeader + `<w:document` + wordNamespaces + docxMetadataNamespaces + `><w:body>` + body.String() + `</w:body></w:document>`)
	return w.output(out, d)
}
func (w *docxWriter) fitSectionStories(d *Document, r resolvedSection) (float64, float64, error) {
	g := pageLayout(r.layout)
	cap, body := 0.0, g.contentHeight()
	kinds := []string{"default"}
	if r.firstPage {
		kinds = append(kinds, "first")
	}
	if d.EvenAndOddHeaders {
		kinds = append(kinds, "even")
	}
	for _, kind := range kinds {
		pick := func(s RunningStories) []Block {
			p := s.Default
			if kind == "first" {
				p = s.First
			}
			if kind == "even" {
				p = s.Even
			}
			if p == nil {
				return nil
			}
			return p.Blocks
		}
		doc := &Document{Layout: r.layout, Blocks: r.blocks, Headers: pick(r.headers), Footers: pick(r.footers), Notes: d.Notes}
		x, h, err := w.fitRunning(doc, g)
		if err != nil {
			return 0, 0, err
		}
		if x > 0 && (cap == 0 || x < cap) {
			cap = x
		}
		body = math.Min(body, h)
	}
	return cap, body, nil
}

func decodeSectionLayout(raw string) (Layout, error) {
	var l Layout
	dec := json.NewDecoder(bytes.NewBufferString(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&l); err != nil {
		return l, err
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		return l, fmt.Errorf("section layout metadata must contain one value")
	}
	return l, validateLayout(l)
}
