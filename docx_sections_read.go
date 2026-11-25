package manifold

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
)

// Word stores a section's properties at its end; type controls its start.
func (r *docxReader) readDocumentSections(properties []*xmlNode, rels map[string]docxRelation) error {
	sectioned := len(properties) > 1 || r.doc.EvenAndOddHeaders
	for _, p := range properties {
		if p.attributeNS(docxNotesNS, "sectionModel") != "" || p.child("titlePg") != nil {
			sectioned = true
		}
		for _, c := range p.Children {
			if (c.Name.Local == "headerReference" || c.Name.Local == "footerReference") && c.attr("type") != "" && c.attr("type") != "default" {
				sectioned = true
			}
		}
	}
	var bodies [][]Block
	var pending []Block
	var boundaryProps []*xmlNode
	for _, b := range r.doc.Blocks {
		section := b.docxSection
		b.docxSection = nil
		if !b.docxSectionTail {
			pending = append(pending, b)
		}
		if section != nil {
			bodies = append(bodies, pending)
			pending = nil
			boundaryProps = append(boundaryProps, section)
		}
	}
	// A final section property belongs directly to body, including an empty body.
	if len(properties) > len(boundaryProps) {
		bodies = append(bodies, pending)
	} else if len(pending) > 0 || len(properties) == 0 {
		bodies = append(bodies, pending)
	}
	if len(properties) > 0 && len(bodies) != len(properties) {
		return fmt.Errorf("section properties outside root body boundaries")
	}
	if len(properties) == 0 {
		return nil
	}
	storyCache := map[string]*RunningStory{}
	var sections []Block
	previous := cloneLayout(r.doc.Layout)
	base := cloneLayout(previous)
	for i, p := range properties {
		if err := r.pageSettings(p, true); err != nil {
			return err
		}
		r.notePropertySources = append(r.notePropertySources, p)
		if i == 0 && p.attributeNS(docxNotesNS, "sectionModel") == "1" {
			raw := p.attributeNS(docxNotesNS, "sectionBase")
			var err error
			base, err = decodeSectionLayout(raw)
			if err != nil {
				return fmt.Errorf("section base: %w", err)
			}
			previous = cloneLayout(base)
		}
		r.doc.Layout = cloneLayout(previous)
		if size := p.child("pgSz"); size != nil {
			var err error
			r.doc.Layout.Width, err = docxPageNumber(size, "w", previous.Width)
			if err != nil {
				return err
			}
			r.doc.Layout.Height, err = docxPageNumber(size, "h", previous.Height)
			if err != nil {
				return err
			}
		}
		if margins := p.child("pgMar"); margins != nil {
			if err := r.pageMargins(margins); err != nil {
				return err
			}
		}
		actual := cloneLayout(r.doc.Layout)
		s := &Section{Blocks: bodies[i], Start: SectionNextPage}
		s.Layout = &actual
		if t := p.child("type"); t != nil {
			s.Start = SectionStart(t.attr("val"))
			if s.Start == "" {
				return fmt.Errorf("missing section start value")
			}
		}
		if t := p.child("titlePg"); t != nil {
			var err error
			s.FirstPage, err = docxOnOff(t)
			if err != nil {
				return err
			}
		}
		model := p.attributeNS(docxNotesNS, "sectionModel")
		if model != "" {
			if model != "1" {
				return fmt.Errorf("invalid section model marker")
			}
			raw := p.attributeNS(docxNotesNS, "sectionLayout")
			expected := previous
			if raw == "inherit" {
				s.Layout = nil
			} else {
				l, err := decodeSectionLayout(raw)
				if err != nil {
					return fmt.Errorf("section layout: %w", err)
				}
				s.Layout = &l
				expected = l
			}
			if !sameDOCXSectionGeometry(expected, actual) {
				return fmt.Errorf("section metadata contradicts physical page settings")
			}
			if flag := p.attributeNS(docxNotesNS, "sectionStartImplicit"); flag != "" {
				if flag != "1" || s.Start != SectionNextPage {
					return fmt.Errorf("invalid implicit section start")
				}
				s.Start = ""
			}
		}
		for _, c := range p.Children {
			if c.Name.Local != "headerReference" && c.Name.Local != "footerReference" {
				continue
			}
			kind, root := "header", "hdr"
			target := &s.Headers
			if c.Name.Local == "footerReference" {
				kind, root, target = "footer", "ftr", &s.Footers
			}
			variant := c.attr("type")
			if variant == "" {
				variant = "default"
			}
			var destination **RunningStory
			switch variant {
			case "default":
				destination = &target.Default
			case "first":
				destination = &target.First
			case "even":
				destination = &target.Even
			default:
				return fmt.Errorf("invalid running story variant %q", variant)
			}
			if *destination != nil {
				return fmt.Errorf("duplicate section %s %s", kind, variant)
			}
			rel, ok := rels[c.attr("id")]
			if !ok || rel.External || !strings.HasSuffix(rel.Type, "/"+kind) {
				return fmt.Errorf("missing or invalid %s relationship", kind)
			}
			if cached := storyCache[rel.Target]; cached != nil {
				*destination = cached
				continue
			}
			part, err := r.xml(rel.Target)
			if err != nil {
				return err
			}
			if part.Name.Local != root {
				return fmt.Errorf("invalid %s root", kind)
			}
			pr, err := r.rels(rel.Target)
			if err != nil {
				return err
			}
			blocks, err := r.blocks(part, pr, 0)
			if err != nil {
				return err
			}
			if len(part.Children) == 1 && part.Children[0].attributeNS(docxNotesNS, "emptyStory") != "" {
				n := part.Children[0]
				if n.Name.Local != "p" || n.attributeNS(docxNotesNS, "emptyStory") != "1" || len(n.Children) != 0 || strings.TrimSpace(n.Text) != "" {
					return fmt.Errorf("invalid empty running story marker")
				}
				blocks = nil
			}
			for _, b := range blocks {
				if b.docxSection != nil {
					return fmt.Errorf("section boundary inside running story")
				}
			}
			*destination = &RunningStory{Blocks: blocks}
			storyCache[rel.Target] = *destination
		}
		var anchors []string
		if raw := p.attributeNS(docxNotesNS, "sectionAnchors"); raw != "" {
			if model != "1" {
				return fmt.Errorf("section anchor metadata requires exported section model")
			}
			if err := json.Unmarshal([]byte(raw), &anchors); err != nil || len(anchors) == 0 {
				return fmt.Errorf("invalid section anchor metadata")
			}
			if len(s.Blocks) == 0 || len(s.Blocks[0].Anchors) < len(anchors) {
				return fmt.Errorf("section anchor metadata lacks standard bookmarks")
			}
			for j, id := range anchors {
				if s.Blocks[0].Anchors[j] != id {
					return fmt.Errorf("section anchor metadata contradicts standard bookmarks")
				}
			}
			s.Blocks[0].Anchors = s.Blocks[0].Anchors[len(anchors):]
			if s.Blocks[0].Kind == AnchorPoint && len(s.Blocks[0].Anchors) == 0 {
				s.Blocks = s.Blocks[1:]
			}
		}
		sections = append(sections, Block{Kind: SectionBlock, Section: s, Anchors: anchors})
		if s.Layout != nil {
			previous = cloneLayout(*s.Layout)
		}
	}
	if sectioned {
		r.doc.Blocks = sections
		r.doc.Layout = base
		if properties[0].attributeNS(docxNotesNS, "sectionModel") == "" {
			r.doc.Layout = cloneLayout(*sections[0].Section.Layout)
		}
	} else {
		s := sections[0].Section
		r.doc.Blocks = s.Blocks
		r.doc.Layout = cloneLayout(*s.Layout)
		if s.Headers.Default != nil {
			r.doc.Headers = s.Headers.Default.Blocks
		}
		if s.Footers.Default != nil {
			r.doc.Footers = s.Footers.Default.Blocks
		}
	}
	return nil
}
func sameDOCXSectionGeometry(a, b Layout) bool {
	x, y := pageLayout(a), pageLayout(b)
	values := func(g pageGeometry) []float64 {
		return []float64{g.width, g.height, g.margins.Left, g.margins.Right, g.margins.Top, g.margins.Bottom, g.headerDistance(), g.footerDistance()}
	}
	av, bv := values(x), values(y)
	for i, v := range av {
		if math.RoundToEven(v*20) != math.RoundToEven(bv[i]*20) {
			return false
		}
	}
	return true
}
