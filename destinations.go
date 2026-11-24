package manydoc

import (
	"fmt"
	"net/url"
)

type destinationCatalog struct {
	anchors map[string]bool
	missing map[string]bool
	links   map[string]bool
}

func destinationID(id string) error {
	if id == "" || len(id) > 1024 {
		return fmt.Errorf("destination identity must contain 1..1024 UTF-8 bytes")
	}
	return validateString(id)
}

func walkDestinationRuns(bs []Block, visit func(Run) error) error {
	for _, b := range bs {
		for _, id := range b.Anchors {
			if err := visit(Run{Anchor: id}); err != nil {
				return err
			}
		}
		for _, r := range b.Runs {
			if err := visit(r); err != nil {
				return err
			}
		}
		if b.Section != nil {
			if err := walkDestinationRuns(b.Section.Blocks, visit); err != nil {
				return err
			}
			for _, s := range sectionStories(b.Section) {
				if s != nil {
					if err := walkDestinationRuns(s.Blocks, visit); err != nil {
						return err
					}
				}
			}
		}
		for _, i := range b.Items {
			if err := walkDestinationRuns(i.Blocks, visit); err != nil {
				return err
			}
		}
		for _, row := range b.Rows {
			for _, c := range row.Cells {
				if err := walkDestinationRuns(c.Blocks, visit); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func documentDestinationRuns(d *Document, visit func(Run) error) error {
	for _, bs := range [][]Block{d.Blocks, d.Headers, d.Footers} {
		if err := walkDestinationRuns(bs, visit); err != nil {
			return err
		}
	}
	for _, n := range d.Notes {
		if err := walkDestinationRuns(n.Blocks, visit); err != nil {
			return err
		}
	}
	return nil
}

func catalogDestinations(d *Document, checkLinks bool) (*destinationCatalog, error) {
	c := &destinationCatalog{anchors: map[string]bool{}, missing: map[string]bool{}, links: map[string]bool{}}
	for _, id := range d.UnresolvedDestinations {
		if err := destinationID(id); err != nil {
			return nil, err
		}
		if c.missing[id] {
			return nil, fmt.Errorf("duplicate unresolved destination %q", id)
		}
		c.missing[id] = true
	}
	err := documentDestinationRuns(d, func(r Run) error {
		if r.Anchor != "" {
			if err := destinationID(r.Anchor); err != nil {
				return err
			}
			if c.anchors[r.Anchor] || c.missing[r.Anchor] {
				return fmt.Errorf("duplicate/conflicting destination %q", r.Anchor)
			}
			if r.Text != "" || r.Note != nil || r.Link != "" || r.Destination != "" || r.Bold || r.Italic || r.Underline || r.Strike || r.FontFamily != "" || r.FontSize != 0 || r.Color != "" || r.Language != "" || r.EastAsiaLanguage != "" || r.ComplexLanguage != "" || r.Direction != "" || r.ComplexScript {
				return fmt.Errorf("anchor is a zero-width unformatted inline atom")
			}
			c.anchors[r.Anchor] = true
		}
		if r.Destination != "" {
			if err := destinationID(r.Destination); err != nil {
				return err
			}
			if r.Note != nil {
				return fmt.Errorf("note references own their navigation target")
			}
			if r.Link != "" && r.Link != "#"+r.Destination {
				u, err := url.Parse(r.Link)
				if err != nil || u.Scheme != "" || u.Host != "" || u.Path != "" || u.RawQuery != "" || u.Opaque != "" || u.Fragment != r.Destination {
					return fmt.Errorf("internal link href contradicts destination %q", r.Destination)
				}
			}
			c.links[r.Destination] = true
		}
		if len(c.anchors)+len(c.missing)+len(c.links) > 100000 {
			return fmt.Errorf("destination graph exceeds 100000 identities/references")
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if checkLinks {
		for id := range c.links {
			if !c.anchors[id] && !c.missing[id] {
				return nil, fmt.Errorf("internal link has undeclared destination %q", id)
			}
		}
	}
	return c, nil
}

func destinationHref(r Run) string {
	if r.Link != "" {
		return r.Link
	}
	return (&url.URL{Fragment: r.Destination}).String()
}
