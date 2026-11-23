package manydoc

import (
	"fmt"
	"strconv"
)

const (
	Footnote = "footnote"
	Endnote  = "endnote"
)

// NoteReference identifies a note independently of its displayed automatic number.
// IDs are scoped by Kind and may be application identifiers rather than integers.
type NoteReference struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
}
type noteKey struct{ kind, id string }

func referenceKey(n *NoteReference) noteKey { return noteKey{n.Kind, n.ID} }
func noteIdentity(n Note) noteKey           { return noteKey{n.Kind, n.ID} }
func walkNoteRuns(bs []Block, visit func(Run) error) error {
	for _, b := range bs {
		if b.Section != nil {
			if err := walkNoteRuns(b.Section.Blocks, visit); err != nil {
				return err
			}
		}
		for _, r := range b.Runs {
			if err := visit(r); err != nil {
				return err
			}
		}
		for _, item := range b.Items {
			if err := walkNoteRuns(item.Blocks, visit); err != nil {
				return err
			}
		}
		for _, row := range b.Rows {
			for _, cell := range row.Cells {
				if err := walkNoteRuns(cell.Blocks, visit); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
func validateNotes(d *Document) error {
	notes := map[noteKey]bool{}
	for _, n := range d.Notes {
		if n.Kind != Footnote && n.Kind != Endnote {
			return fmt.Errorf("invalid note kind %q", n.Kind)
		}
		if n.ID == "" {
			return fmt.Errorf("note ID is required")
		}
		if err := validateString(n.ID); err != nil {
			return err
		}
		key := noteIdentity(n)
		if notes[key] {
			return fmt.Errorf("duplicate %s note ID %q", n.Kind, n.ID)
		}
		notes[key] = true
		if err := walkNoteRuns(n.Blocks, func(r Run) error {
			if r.Note != nil {
				return fmt.Errorf("note bodies cannot contain note references")
			}
			return nil
		}); err != nil {
			return err
		}
	}
	var running [][]Block
	for _, b := range d.Blocks {
		if b.Section != nil {
			for _, story := range sectionStories(b.Section) {
				if story != nil {
					running = append(running, story.Blocks)
				}
			}
		}
	}
	running = append(running, d.Headers, d.Footers)
	for _, bs := range running {
		if err := walkNoteRuns(bs, func(r Run) error {
			if r.Note != nil {
				return fmt.Errorf("note references cannot occur in running headers/footers")
			}
			return nil
		}); err != nil {
			return err
		}
	}
	return walkNoteRuns(d.Blocks, func(r Run) error {
		if r.Note == nil {
			return nil
		}
		if r.Text != "" || r.Link != "" {
			return fmt.Errorf("note reference cannot also contain text or an external link")
		}
		if !notes[referenceKey(r.Note)] {
			return fmt.Errorf("dangling %s note reference %q", r.Note.Kind, r.Note.ID)
		}
		return nil
	})
}

type noteCatalog struct {
	definitions map[noteKey]Note
	labels      map[noteKey]string
	order       []noteKey
}

func catalogNotes(d *Document, requireReferences bool) (*noteCatalog, error) {
	c := &noteCatalog{definitions: map[noteKey]Note{}, labels: map[noteKey]string{}}
	for _, n := range d.Notes {
		c.definitions[noteIdentity(n)] = n
	}
	counters := map[string]int{}
	err := walkNoteRuns(d.Blocks, func(r Run) error {
		if r.Note == nil {
			return nil
		}
		key := referenceKey(r.Note)
		if _, ok := c.labels[key]; !ok {
			counters[key.kind]++
			c.labels[key] = strconv.Itoa(counters[key.kind])
			c.order = append(c.order, key)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if requireReferences {
		for _, n := range d.Notes {
			if _, ok := c.labels[noteIdentity(n)]; !ok {
				return nil, fmt.Errorf("%w: unreferenced %s note %q must be anchored before rendering", ErrUnsupported, n.Kind, n.ID)
			}
		}
	}
	return c, nil
}
