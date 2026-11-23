package manydoc

import (
	"context"
	"fmt"
)

// Check every story before layout, including empty paragraphs and table/note
// content. Direction must not be silently ignored merely because text is empty.
func validatePDFDirections(ctx context.Context, d *Document) error {
	var walk func([]Block) error
	walk = func(blocks []Block) error {
		for _, b := range blocks {
			if err := ctx.Err(); err != nil {
				return err
			}
			if b.Direction == "rtl" {
				return fmt.Errorf("%w: PDF paragraph direction needs bidirectional layout", ErrUnsupported)
			}
			for _, run := range b.Runs {
				if run.Direction == "rtl" {
					return fmt.Errorf("%w: PDF run direction needs bidirectional layout", ErrUnsupported)
				}
			}
			for _, item := range b.Items {
				if err := walk(item.Blocks); err != nil {
					return err
				}
			}
			for _, row := range b.Rows {
				for _, cell := range row.Cells {
					if err := walk(cell.Blocks); err != nil {
						return err
					}
				}
			}
		}
		return nil
	}
	for _, blocks := range [][]Block{d.Blocks, d.Headers, d.Footers} {
		if err := walk(blocks); err != nil {
			return err
		}
	}
	for _, note := range d.Notes {
		if err := walk(note.Blocks); err != nil {
			return err
		}
	}
	return nil
}
