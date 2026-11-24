package manydoc

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	Footnote = "footnote"
	Endnote  = "endnote"
)

// NoteReference identifies a note independently of its displayed automatic number.
// IDs are scoped by Kind and may be application identifiers rather than integers.
type NoteReference struct {
	// Inferred records a recovered relationship that was not an explicit source link.
	Inferred bool `json:"inferred,omitempty"`
	// Label overrides this citation only. Nil inherits the note label; empty is markerless.
	Label *string `json:"label,omitempty"`
	ID    string  `json:"id"`
	Kind  string  `json:"kind"`
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
		if n.BodyLabel != nil {
			if err := validateNoteLabel(*n.BodyLabel); err != nil {
				return err
			}
		}
		if err := validateNoteLabel(n.Label); err != nil {
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
		if r.Note.Label != nil {
			if err := validateNoteLabel(*r.Note.Label); err != nil {
				return err
			}
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
			if label := c.definitions[key].Label; label != "" {
				c.labels[key] = label
			} else {
				counters[key.kind]++
				c.labels[key] = strconv.Itoa(counters[key.kind])
			}
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

func validateNoteLabel(label string) error {
	if err := validateString(label); err != nil {
		return err
	}
	if strings.ContainsAny(label, "\t\r\n\u0085\u2028\u2029") || (label != "" && strings.TrimSpace(label) == "") {
		return fmt.Errorf("note label must be a nonblank single-line mark")
	}
	return nil
}

func noteReferenceLabel(c *noteCatalog, ref *NoteReference) string {
	if ref.Label != nil {
		return *ref.Label
	}
	return c.labels[referenceKey(ref)]
}
func noteBodyLabel(c *noteCatalog, key noteKey) string {
	if value := c.definitions[key].BodyLabel; value != nil {
		return *value
	}
	return c.labels[key]
}
func variantNoteKeys(d *Document) map[noteKey]bool {
	keys := map[noteKey]bool{}
	for _, n := range d.Notes {
		if n.BodyLabel != nil || n.BodyLabelStyle != nil {
			keys[noteIdentity(n)] = true
		}
	}
	_ = walkNoteRuns(d.Blocks, func(r Run) error {
		if r.Note != nil && r.Note.Label != nil {
			keys[referenceKey(r.Note)] = true
		}
		return nil
	})
	return keys
}

func noteBodyLabelRun(c *noteCatalog, key noteKey) Run {
	var run Run
	if style := c.definitions[key].BodyLabelStyle; style != nil {
		run = *style
	}
	run.Text = noteBodyLabel(c, key)
	return run
}
