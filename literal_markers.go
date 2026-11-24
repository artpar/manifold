package manydoc

import "fmt"

// Markers are ordinary visible flow content, not detached source provenance.
// Page/section controls have no defined parallel label/body pagination semantics.
func validateLiteralMarker(bs []Block, depth int) error {
	if depth > 64 {
		return fmt.Errorf("document nesting exceeds64 levels")
	}
	for _, b := range bs {
		if b.Kind == SectionBlock || b.Kind == PageBreak || b.Kind == AnchorPoint || b.PageBreakBefore {
			return fmt.Errorf("%w: literal list markers cannot contain page/section controls or standalone anchors", ErrUnsupported)
		}
		for _, row := range b.Rows {
			for _, cell := range row.Cells {
				if err := validateLiteralMarker(cell.Blocks, depth+1); err != nil {
					return err
				}
			}
		}
		for _, item := range b.Items {
			if item.Marker != nil {
				if err := validateLiteralMarker(item.Marker.Blocks, depth+1); err != nil {
					return err
				}
			}
			if err := validateLiteralMarker(item.Blocks, depth+1); err != nil {
				return err
			}
		}
	}
	return nil
}

func (d *Document) hasLiteralListMarkers() bool {
	var walk func([]Block) bool
	walk = func(bs []Block) bool {
		for _, b := range bs {
			if b.Section != nil {
				if walk(b.Section.Blocks) {
					return true
				}
				for _, story := range sectionStories(b.Section) {
					if story != nil && walk(story.Blocks) {
						return true
					}
				}
			}
			for _, item := range b.Items {
				if item.Marker != nil || walk(item.Blocks) {
					return true
				}
			}
			for _, row := range b.Rows {
				for _, cell := range row.Cells {
					if walk(cell.Blocks) {
						return true
					}
				}
			}
		}
		return false
	}
	if walk(d.Blocks) || walk(d.Headers) || walk(d.Footers) {
		return true
	}
	for _, note := range d.Notes {
		if walk(note.Blocks) {
			return true
		}
	}
	return false
}
