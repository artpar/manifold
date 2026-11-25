package manifold

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
)

// This projection bounds individual frames, not total story height. The native
// editor still owns text metrics and pagination. Preferred SF sizes stay in the
// checked geometry record; bytes and source relationships do not move.
func colleafImageBoxes(ctx context.Context, blocks []Block, box colleafImageSize) ([]colleafImageSize, error) {
	var boxes []colleafImageSize
	var visit func([]Block, colleafImageSize, int) error
	visit = func(bs []Block, available colleafImageSize, depth int) error {
		if depth > 64 {
			return fmt.Errorf("native image container nesting exceeds64")
		}
		for _, b := range bs {
			if err := ctx.Err(); err != nil {
				return err
			}
			switch b.Kind {
			case Picture:
				if !finite(available.Width) || !finite(available.Height) || available.Width <= 0 || available.Height <= 0 {
					return fmt.Errorf("%w: native image container has no space", ErrUnsupported)
				}
				boxes = append(boxes, available)
			case ListBlock:
				for i, item := range b.Items {
					if hasColleafLiteralItems(b) {
						if err := visit([]Block{colleafLiteralTable(b, item, i)}, available, depth+1); err != nil {
							return err
						}
						continue
					}
					if err := visit(item.Blocks, available, depth+1); err != nil {
						return err
					}
				}
			case Table:
				grid, err := tableGrid(b.Rows)
				if err != nil {
					return err
				}
				widths := make([]float64, grid.Columns)
				if grid.Columns == 0 {
					continue
				}
				total := 0.0
				for i := range widths {
					widths[i] = available.Width / float64(grid.Columns)
					if len(b.ColumnWidths) > 0 {
						widths[i] = b.ColumnWidths[i]
					}
					total += widths[i]
				}
				scale := 1.0
				if total > available.Width {
					scale = available.Width / total
				}
				for _, origin := range grid.Origins {
					width := 0.0
					for _, w := range widths[origin.Column : origin.Column+origin.ColSpan] {
						width += w * scale
					}
					// Published native defaults, checked against executable cell pages:
					// 10px each horizontal edge, 5px vertical; reserve a paragraph as well.
					inner := colleafImageSize{Width: math.Min(width, math.Max(.75, width-15)), Height: available.Height - 14}
					if err := visit(b.Rows[origin.Row].Cells[origin.Cell].Blocks, inner, depth+1); err != nil {
						return err
					}
				}
			}
		}
		return nil
	}
	if err := visit(blocks, box, 0); err != nil {
		return nil, err
	}
	return boxes, nil
}
func colleafBoundImages(ctx context.Context, blocks []Block, box colleafImageSize, bindings []colleafImageBinding, drawings map[string]json.RawMessage, cache outputImageCache) error {
	boxes, err := colleafImageBoxes(ctx, blocks, box)
	if err != nil {
		return err
	}
	if len(boxes) != len(bindings) {
		return fmt.Errorf("native image container/source count mismatch")
	}
	for i, bound := range boxes {
		b := &bindings[i]
		var drawing colleafNativeImage
		if err = json.Unmarshal(drawings[b.ID], &drawing); err != nil {
			return err
		}
		if drawing.Transform == nil {
			return fmt.Errorf("native fitted image lacks frame")
		}
		data, _, err := colleafEmbeddedImage(drawing.Source)
		if err != nil {
			return err
		}
		cfg, _, err := decodeOutputImage(ctx, data, cache)
		if err != nil {
			return err
		}
		im := &Image{Width: b.Width, Height: b.Height}
		width, height, err := fitImageBox(im, cfg.Width, cfg.Height, bound.Width, bound.Height)
		if err != nil {
			return err
		}
		// Do not attach context when it did not change the frame; existing private
		// workers remain wire-compatible and sparse preferred-size intent stays small.
		size := colleafImageSize{Width: width * 4 / 3, Height: height * 4 / 3}
		if size == drawing.Transform.Size {
			continue
		}
		drawing.Transform.Size = size
		b.NativeWidth = size.Width
		b.NativeHeight = size.Height
		b.Container = &bound
		drawings[b.ID] = sfAttrs(drawing)
	}
	return nil
}

func fitColleafDocumentImages(ctx context.Context, d *Document, out *colleafNoteSnapshot, geometry *colleafImageGeometry, running map[string][]Block, cache outputImageCache) error {
	sections := d.Blocks
	if !hasSectionBlocks(sections) {
		sections = []Block{{Kind: SectionBlock, Section: &Section{Blocks: d.Blocks}}}
	}
	layout := d.Layout
	noteBoxes := map[noteKey]colleafImageSize{}
	storyBoxes := map[string]colleafImageSize{}
	narrow := func(old colleafImageSize, ok bool, b colleafImageSize) colleafImageSize {
		if ok {
			b.Width = min(old.Width, b.Width)
			b.Height = min(old.Height, b.Height)
		}
		return b
	}
	root := colleafPageProperties{}
	if err := json.Unmarshal(out.DocumentStyle, &root); err != nil {
		return err
	}
	refs := []*string{root.DefaultHeader, root.FirstHeader, root.EvenHeader, root.DefaultFooter, root.FirstFooter, root.EvenFooter}
	sectionProperties := map[string]colleafPageProperties{}
	for _, record := range out.Body.SectionBreaks {
		sectionProperties[record.ID] = record.colleafPageProperties
	}
	offset := 0
	minimum := colleafImageSize{Width: math.Inf(1), Height: math.Inf(1)}
	for i, block := range sections {
		s := block.Section
		if s.Layout != nil {
			layout = *s.Layout
		}
		g := pageLayout(layout)
		body := colleafImageSize{Width: g.width - g.margins.Left - g.margins.Right, Height: g.height - g.margins.Top - g.margins.Bottom - 8}
		minimum = narrow(minimum, true, body)
		boxes, err := colleafImageBoxes(ctx, s.Blocks, body)
		if err != nil {
			return err
		}
		bindings := geometry.Stories[""]
		if offset+len(boxes) > len(bindings) {
			return fmt.Errorf("native section image count mismatch")
		}
		if err = colleafBoundImages(ctx, s.Blocks, body, bindings[offset:offset+len(boxes)], out.Drawings, cache); err != nil {
			return err
		}
		offset += len(boxes)
		if err = walkNoteRuns(s.Blocks, func(r Run) error {
			if r.Note != nil {
				key := referenceKey(r.Note)
				old, ok := noteBoxes[key]
				noteBoxes[key] = narrow(old, ok, body)
			}
			return nil
		}); err != nil {
			return err
		}
		var p colleafPageProperties
		// Cell section records are interspersed in the native slice. IDs identify
		// only root section records produced by the document composer.
		id := fmt.Sprintf("manydoc-section-%d", i+1)
		p = sectionProperties[id]
		next := []*string{p.DefaultHeader, p.FirstHeader, p.EvenHeader, p.DefaultFooter, p.FirstFooter, p.EvenFooter}
		for j, v := range next {
			if v != nil {
				refs[j] = v
			}
		}
		for j, ref := range refs {
			if ref == nil || *ref == "" {
				continue
			}
			height := g.height - g.headerDistance() - g.footerDistance() - 36
			// Prefer the declared running band when it exists; the native renderer may
			// expand it for text/tables. Physical fallback leaves a body line on zero-margin pages.
			band := g.margins.Top - g.headerDistance()
			if j >= 3 {
				band = g.margins.Bottom - g.footerDistance()
			}
			if band > 0 {
				height = min(height, band)
			}
			b := colleafImageSize{Width: body.Width, Height: height}
			old, ok := storyBoxes[*ref]
			storyBoxes[*ref] = narrow(old, ok, b)
		}
	}
	if offset != len(geometry.Stories[""]) {
		return fmt.Errorf("unassigned native main image")
	}
	for id, blocks := range running {
		box, ok := storyBoxes[id]
		if !ok {
			return fmt.Errorf("native running image scope is unreferenced")
		}
		if err := colleafBoundImages(ctx, blocks, box, geometry.Stories[id], out.Drawings, cache); err != nil {
			return err
		}
	}
	var identities colleafNoteIdentities
	if err := colleafResourceMap(out.Resources, colleafNoteIdentityResource, &identities); err != nil {
		return err
	}
	ids := map[noteKey]string{}
	for _, b := range identities.Bindings {
		ids[noteKey{b.Kind, b.ID}] = b.NativeID
	}
	for _, note := range d.Notes {
		box, ok := noteBoxes[noteIdentity(note)]
		if !ok {
			box = minimum
		}
		box.Height -= docxImageNoteHeightAllowance
		id := ids[noteIdentity(note)]
		native, ok := out.Notes[id]
		if !ok {
			return fmt.Errorf("native note image scope missing")
		}
		if err := colleafBoundImages(ctx, note.Blocks, box, geometry.Stories[id], native.Drawings, cache); err != nil {
			return err
		}
	}
	return nil
}
