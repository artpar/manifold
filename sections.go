package manydoc

import "fmt"

// SectionStart controls the section start relative to the preceding section.
// An empty value means nextPage; the first section starts the document.
type SectionStart string

const (
	SectionNextPage   SectionStart = "nextPage"
	SectionContinuous SectionStart = "continuous"
	SectionEvenPage   SectionStart = "evenPage"
	SectionOddPage    SectionStart = "oddPage"
)

// RunningStories distinguishes an inherited nil story from an explicit empty one.
// Default serves odd pages (and all ordinary pages without EvenAndOddHeaders).
type RunningStories struct {
	Default *RunningStory `json:"default,omitempty"`
	First   *RunningStory `json:"first,omitempty"`
	Even    *RunningStory `json:"even,omitempty"`
}

// RunningStory is an independently inherited header or footer variant.
// A non-nil story with no blocks explicitly suppresses that variant.
type RunningStory struct {
	Blocks []Block `json:"blocks"`
}

// Section owns body content. Layout nil inherits the previous effective layout
// (the document layout for the first section). Stories inherit independently.
// Native PDF keeps the stories of the section that opened a physical page.
// A continuous section counts a shared page as local page one, even when its
// body is empty. Later pages select its default/even stories using that count.
type Section struct {
	Layout    *Layout        `json:"layout,omitempty"`
	Start     SectionStart   `json:"start,omitempty"`
	FirstPage bool           `json:"first_page,omitempty"`
	Headers   RunningStories `json:"headers"`
	Footers   RunningStories `json:"footers"`
	Blocks    []Block        `json:"blocks"`
}

func sectionStories(s *Section) []*RunningStory {
	return []*RunningStory{s.Headers.Default, s.Headers.First, s.Headers.Even, s.Footers.Default, s.Footers.First, s.Footers.Even}
}
func hasSectionBlocks(bs []Block) bool {
	for _, b := range bs {
		if b.Kind == SectionBlock {
			return true
		}
	}
	return false
}
func validateSectionPlacement(d *Document) error {
	sectioned := hasSectionBlocks(d.Blocks)
	if sectioned && (len(d.Headers) > 0 || len(d.Footers) > 0) {
		return fmt.Errorf("sectioned documents keep running stories in sections")
	}
	if d.EvenAndOddHeaders && !sectioned {
		return fmt.Errorf("distinct even/odd stories require section containers")
	}
	var walk func([]Block, bool, int) error
	walk = func(bs []Block, root bool, depth int) error {
		if depth > 64 {
			return fmt.Errorf("document nesting exceeds 64 levels")
		}
		for _, b := range bs {
			if b.Kind == SectionBlock {
				if !root || b.Section == nil {
					return fmt.Errorf("sections require a top-level section container")
				}
				s := b.Section
				switch s.Start {
				case "", SectionNextPage, SectionContinuous, SectionEvenPage, SectionOddPage:
				default:
					return fmt.Errorf("invalid section start %q", s.Start)
				}
				if s.Layout != nil {
					if err := validateLayout(*s.Layout); err != nil {
						return err
					}
				}
				if err := walk(s.Blocks, false, depth+1); err != nil {
					return err
				}
				for _, story := range sectionStories(s) {
					if story != nil {
						if err := walk(story.Blocks, false, depth+1); err != nil {
							return err
						}
					}
				}
			} else {
				if b.Section != nil {
					return fmt.Errorf("section properties require section kind")
				}
				if root && sectioned {
					return fmt.Errorf("top-level sections cannot mix with ordinary blocks")
				}
				for _, item := range b.Items {
					if err := walk(item.Blocks, false, depth+1); err != nil {
						return err
					}
				}
				for _, row := range b.Rows {
					for _, cell := range row.Cells {
						if err := walk(cell.Blocks, false, depth+1); err != nil {
							return err
						}
					}
				}
			}
		}
		return nil
	}
	if err := walk(d.Blocks, true, 0); err != nil {
		return err
	}
	for _, bs := range [][]Block{d.Headers, d.Footers} {
		if err := walk(bs, false, 0); err != nil {
			return err
		}
	}
	for _, note := range d.Notes {
		if err := walk(note.Blocks, false, 0); err != nil {
			return err
		}
	}
	return nil
}

type resolvedSection struct {
	layout           Layout
	start            SectionStart
	firstPage        bool
	headers, footers RunningStories
	blocks           []Block
}

func inheritStories(current, previous RunningStories) RunningStories {
	if current.Default == nil {
		current.Default = previous.Default
	}
	if current.First == nil {
		current.First = previous.First
	}
	if current.Even == nil {
		current.Even = previous.Even
	}
	return current
}
func documentSections(d *Document) []resolvedSection {
	previous := resolvedSection{layout: cloneLayout(d.Layout), headers: RunningStories{Default: &RunningStory{d.Headers}}, footers: RunningStories{Default: &RunningStory{d.Footers}}}
	if !hasSectionBlocks(d.Blocks) {
		previous.blocks = d.Blocks
		return []resolvedSection{previous}
	}
	var out []resolvedSection
	for _, b := range d.Blocks {
		s := b.Section
		current := resolvedSection{layout: cloneLayout(previous.layout), start: s.Start, firstPage: s.FirstPage, headers: inheritStories(s.Headers, previous.headers), footers: inheritStories(s.Footers, previous.footers), blocks: s.Blocks}
		if len(b.Anchors) > 0 {
			current.blocks = append([]Block(nil), current.blocks...)
			if len(current.blocks) == 0 {
				current.blocks = []Block{{Kind: AnchorPoint, Anchors: b.Anchors}}
			} else {
				current.blocks[0].Anchors = append(append([]string(nil), b.Anchors...), current.blocks[0].Anchors...)
			}
		}
		if s.Layout != nil {
			current.layout = cloneLayout(*s.Layout)
		}
		out = append(out, current)
		previous = current
	}
	return out
}
func selectStory(stories RunningStories, first, even, distinctFirst, distinctEven bool) []Block {
	story := stories.Default
	if first && distinctFirst {
		story = stories.First
	} else if even && distinctEven {
		story = stories.Even
	}
	if story == nil {
		return nil
	}
	return story.Blocks
}
