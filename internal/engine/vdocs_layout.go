package engine

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"

	// This projection bounds individual frames, not total story height. The native
	// editor still owns text metrics and pagination. Preferred SF sizes stay in the
	// checked geometry record; bytes and source relationships do not move.
	"image"
	"math"
	"reflect"
	"sort"
	"strings"
)

func vdocsImageBoxes(ctx context.Context, blocks []Block, box vdocsImageSize) ([]vdocsImageSize, error) {
	var boxes []vdocsImageSize
	var visit func([]Block, vdocsImageSize, int) error
	visit = func(bs []Block, available vdocsImageSize, depth int) error {
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
						if err := visit([]Block{vdocsLiteralTable(b, item, i)}, available, depth+1); err != nil {
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
					inner := vdocsImageSize{Width: math.Min(width, math.Max(.75, width-15)), Height: available.Height - 14}
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
func vdocsBoundImages(ctx context.Context, blocks []Block, box vdocsImageSize, bindings []vdocsImageBinding, drawings map[string]json.RawMessage, cache outputImageCache) error {
	boxes, err := vdocsImageBoxes(ctx, blocks, box)
	if err != nil {
		return err
	}
	if len(boxes) != len(bindings) {
		return fmt.Errorf("native image container/source count mismatch")
	}
	for i, bound := range boxes {
		b := &bindings[i]
		var drawing vdocsNativeImage
		if err = json.Unmarshal(drawings[b.ID], &drawing); err != nil {
			return err
		}
		if drawing.Transform == nil {
			return fmt.Errorf("native fitted image lacks frame")
		}
		data, _, err := vdocsEmbeddedImage(drawing.Source)
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
		size := vdocsImageSize{Width: width * 4 / 3, Height: height * 4 / 3}
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

func fitColleafDocumentImages(ctx context.Context, d *Document, out *vdocsNoteSnapshot, geometry *vdocsImageGeometry, running map[string][]Block, cache outputImageCache) error {
	sections := d.Blocks
	if !hasSectionBlocks(sections) {
		sections = []Block{{Kind: SectionBlock, Section: &Section{Blocks: d.Blocks}}}
	}
	layout := d.Layout
	noteBoxes := map[noteKey]vdocsImageSize{}
	storyBoxes := map[string]vdocsImageSize{}
	narrow := func(old vdocsImageSize, ok bool, b vdocsImageSize) vdocsImageSize {
		if ok {
			b.Width = min(old.Width, b.Width)
			b.Height = min(old.Height, b.Height)
		}
		return b
	}
	root := vdocsPageProperties{}
	if err := json.Unmarshal(out.DocumentStyle, &root); err != nil {
		return err
	}
	refs := []*string{root.DefaultHeader, root.FirstHeader, root.EvenHeader, root.DefaultFooter, root.FirstFooter, root.EvenFooter}
	sectionProperties := map[string]vdocsPageProperties{}
	for _, record := range out.Body.SectionBreaks {
		sectionProperties[record.ID] = record.vdocsPageProperties
	}
	offset := 0
	minimum := vdocsImageSize{Width: math.Inf(1), Height: math.Inf(1)}
	for i, block := range sections {
		s := block.Section
		if s.Layout != nil {
			layout = *s.Layout
		}
		g := pageLayout(layout)
		body := vdocsImageSize{Width: g.width - g.margins.Left - g.margins.Right, Height: g.height - g.margins.Top - g.margins.Bottom - 8}
		minimum = narrow(minimum, true, body)
		boxes, err := vdocsImageBoxes(ctx, s.Blocks, body)
		if err != nil {
			return err
		}
		bindings := geometry.Stories[""]
		if offset+len(boxes) > len(bindings) {
			return fmt.Errorf("native section image count mismatch")
		}
		if err = vdocsBoundImages(ctx, s.Blocks, body, bindings[offset:offset+len(boxes)], out.Drawings, cache); err != nil {
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
		var p vdocsPageProperties
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
			b := vdocsImageSize{Width: body.Width, Height: height}
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
		if err := vdocsBoundImages(ctx, blocks, box, geometry.Stories[id], out.Drawings, cache); err != nil {
			return err
		}
	}
	var identities vdocsNoteIdentities
	if err := vdocsResourceMap(out.Resources, vdocsNoteIdentityResource, &identities); err != nil {
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
		if err := vdocsBoundImages(ctx, note.Blocks, box, geometry.Stories[id], native.Drawings, cache); err != nil {
			return err
		}
	}
	return nil
}

const vdocsImageResource = "manydoc.image-geometry.v1"

type vdocsImageSize struct {
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}
type vdocsDrawingTransform struct {
	Size      vdocsImageSize  `json:"size"`
	PositionH json.RawMessage `json:"positionH"`
	PositionV json.RawMessage `json:"positionV"`
	Angle     float64         `json:"angle"`
	FlipX     bool            `json:"flipX,omitempty"`
	FlipY     bool            `json:"flipY,omitempty"`
}
type vdocsNativeImage struct {
	Origin      json.RawMessage        `json:"manydocOrigin,omitempty"`
	ID          string                 `json:"drawingId"`
	Type        *int                   `json:"drawingType"`
	Layout      *int                   `json:"layoutType"`
	UnitID      string                 `json:"unitId"`
	SubUnitID   string                 `json:"subUnitId"`
	SourceType  string                 `json:"imageSourceType"`
	Source      string                 `json:"source"`
	Cache       string                 `json:"base64Cache,omitempty"`
	Description string                 `json:"description,omitempty"`
	Title       string                 `json:"title,omitempty"`
	Transform   *vdocsDrawingTransform `json:"docTransform"`
	Hidden      bool                   `json:"hidden,omitempty"`
	Opacity     *float64               `json:"opacity,omitempty"`
	Crop        json.RawMessage        `json:"srcRect,omitempty"`
	Geometry    *string                `json:"prstGeom,omitempty"`
}
type vdocsImageBinding struct {
	Container    *vdocsImageSize `json:"container,omitempty"`
	ID           string          `json:"drawingId"`
	Hash         string          `json:"sha256"`
	Width        float64         `json:"width"`
	Height       float64         `json:"height"`
	NativeWidth  float64         `json:"nativeWidth"`
	NativeHeight float64         `json:"nativeHeight"`
}
type vdocsImageGeometry struct {
	Stories map[string][]vdocsImageBinding `json:"stories"`
}
type vdocsEncodedBody struct {
	Body                    vdocsBody
	Tables, Lists, Drawings map[string]json.RawMessage
	DrawingOrder            []string
	ListsOwnership          []vdocsListBinding
	ImageGeometry           []vdocsImageBinding
}

func vdocsIntrinsicImageSize(im *Image, cfg image.Config) (vdocsImageSize, error) {
	w, h := im.Width, im.Height
	if !finite(w) || !finite(h) || w < 0 || h < 0 {
		return vdocsImageSize{}, fmt.Errorf("invalid image geometry")
	}
	if w == 0 && h == 0 {
		w = float64(cfg.Width) * .75
	}
	if w == 0 {
		w = h * float64(cfg.Width) / float64(cfg.Height)
	}
	if h == 0 {
		h = w * float64(cfg.Height) / float64(cfg.Width)
	}
	if w <= 0 || h <= 0 || w > 1e7 || h > 1e7 {
		return vdocsImageSize{}, fmt.Errorf("image geometry outside supported coordinates")
	}
	return vdocsImageSize{w * 4 / 3, h * 4 / 3}, nil
}
func vdocsImageHash(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}
func (w *vdocsBodyWriter) picture(block Block) error {
	if block.Image == nil || block.Direction != "" || block.PageBreakBefore || len(block.Runs) > 0 || len(block.Rows) > 0 || len(block.Items) > 0 || block.Section != nil || block.List != "" || block.Numbering != nil || block.Start != nil || block.Level != 0 {
		return fmt.Errorf("%w: invalid/unmapped native picture annotations", ErrUnsupported)
	}
	cfg, kind, err := decodeOutputImage(w.ctx, block.Image.Data, w.imageCache)
	if err != nil {
		return err
	}
	if block.Image.MediaType != "image/"+kind {
		return fmt.Errorf("image media type contradicts embedded pixels")
	}
	if err = validateString(block.Image.Alt); err != nil {
		return err
	}
	size, err := vdocsIntrinsicImageSize(block.Image, cfg)
	if err != nil {
		return err
	}
	story := base64.RawURLEncoding.EncodeToString([]byte(w.storyID))
	if story == "" {
		story = "main"
	}
	id := "manydoc-image-" + story + "-" + w.id("i")
	if err := w.containerAnchors(block.Anchors, w.offset, "picture", id); err != nil {
		return err
	}
	zero := 0
	drawing := vdocsNativeImage{ID: id, Type: &zero, Layout: &zero, UnitID: w.documentID, SubUnitID: w.documentID, SourceType: "BASE64", Source: "data:" + block.Image.MediaType + ";base64," + base64.StdEncoding.EncodeToString(block.Image.Data), Description: block.Image.Alt, Transform: &vdocsDrawingTransform{Size: size, PositionH: sfAttrs(map[string]int{"relativeFrom": 2, "posOffset": 0}), PositionV: sfAttrs(map[string]int{"relativeFrom": 2, "posOffset": 0})}}
	if block.Source != nil {
		hash, err := vdocsImageFingerprint(block)
		if err != nil {
			return err
		}
		origin, err := newColleafSourceOrigin(block.Source, hash, vdocsImageOriginProfile)
		if err != nil {
			return err
		}
		drawing.Origin = sfAttrs(origin)
	}
	w.drawings[id] = sfAttrs(drawing)
	w.drawingOrder = append(w.drawingOrder, id)
	w.imageGeometry = append(w.imageGeometry, vdocsImageBinding{ID: id, Hash: vdocsImageHash(block.Image.Data), Width: block.Image.Width, Height: block.Image.Height, NativeWidth: size.Width, NativeHeight: size.Height})
	w.body.CustomBlocks = append(w.body.CustomBlocks, vdocsBlockRecord{StartIndex: w.offset, Type: &zero, ID: id})
	w.text("\b")
	w.body.Paragraphs = append(w.body.Paragraphs, vdocsParagraphRecord{StartIndex: w.offset, ID: w.id("p")})
	w.text("\r")
	return nil
}
func encodeColleafAssetBody(ctx context.Context, blocks []Block, noteIDs map[noteKey]string, documentID, storyID string, cache outputImageCache, projection ...map[noteKey]string) (vdocsEncodedBody, error) {
	if cache == nil {
		cache = outputImageCache{}
	}
	w := vdocsBodyWriter{ctx: ctx, noteIDs: noteIDs, documentID: documentID, storyID: storyID, imageCache: cache, tables: map[string]json.RawMessage{}, lists: map[string]json.RawMessage{}, drawings: map[string]json.RawMessage{}, body: vdocsBody{TextRuns: []vdocsTextRun{}}}
	if len(projection) > 0 {
		w.citationProjection = projection[0]
	}
	if err := w.blocks(blocks, 0); err != nil {
		return vdocsEncodedBody{}, err
	}
	w.body.DataStream = w.stream.String()
	if _, err := indexColleafBody(ctx, w.body); err != nil {
		return vdocsEncodedBody{}, err
	}
	return vdocsEncodedBody{Body: w.body, Tables: w.tables, Lists: w.lists, Drawings: w.drawings, DrawingOrder: w.drawingOrder, ListsOwnership: w.listBindings, ImageGeometry: w.imageGeometry}, nil
}
func vdocsEmbeddedImage(source string) ([]byte, string, error) {
	comma := strings.IndexByte(source, ',')
	if comma < 0 {
		return nil, "", fmt.Errorf("%w: image must contain embedded PNG/JPEG bytes", ErrUnsupported)
	}
	header := strings.ToLower(source[:comma])
	kind := ""
	switch header {
	case "data:image/png;base64":
		kind = "png"
	case "data:image/jpeg;base64":
		kind = "jpeg"
	default:
		return nil, "", fmt.Errorf("%w: native image source must be embedded PNG/JPEG", ErrUnsupported)
	}
	payload := source[comma+1:]
	if strings.ContainsAny(payload, " \r\n\t") {
		return nil, "", fmt.Errorf("invalid embedded image base64")
	}
	data, err := base64.StdEncoding.Strict().DecodeString(payload)
	if err != nil {
		return nil, "", fmt.Errorf("invalid embedded image base64: %w", err)
	}
	return data, kind, nil
}
func (r *vdocsBodyReader) nativeImage(id string) (*Image, error) {
	if cached := r.images[id]; cached != nil {
		copy := *cached
		return &copy, nil
	}
	raw, ok := r.settings.Drawings[id]
	if !ok {
		return nil, fmt.Errorf("native image atom lacks drawing source %q", id)
	}
	var drawing vdocsNativeImage
	if err := json.Unmarshal(raw, &drawing); err != nil {
		return nil, err
	}
	if drawing.ID != id || drawing.Type == nil || *drawing.Type != 0 || drawing.Transform == nil {
		return nil, fmt.Errorf("%w: native drawing is not a bound image", ErrUnsupported)
	}
	if drawing.Layout == nil || *drawing.Layout != 0 || drawing.Transform.Angle != 0 || drawing.Transform.FlipX || drawing.Transform.FlipY || drawing.Hidden || (drawing.Opacity != nil && *drawing.Opacity != 1) || (len(drawing.Crop) > 0 && string(drawing.Crop) != "null") || (drawing.Geometry != nil && *drawing.Geometry != "rect") {
		return nil, fmt.Errorf("%w: native image transforms/floating/visibility need their appearance mapping", ErrUnsupported)
	}
	if drawing.SourceType != "BASE64" {
		return nil, fmt.Errorf("%w: native external/hosted image source", ErrUnsupported)
	}
	data, kind, err := vdocsEmbeddedImage(drawing.Source)
	if err != nil {
		return nil, err
	}
	if drawing.Cache != "" {
		cached, cacheKind, err := vdocsEmbeddedImage(drawing.Cache)
		if err != nil || cacheKind != kind || vdocsImageHash(cached) != vdocsImageHash(data) {
			return nil, fmt.Errorf("contradictory native image source/cache")
		}
	}
	cfg, decodedKind, err := decodeOutputImage(r.ctx, data, r.settings.ImageCache)
	if err != nil {
		return nil, err
	}
	if decodedKind != kind {
		return nil, fmt.Errorf("native image media type contradicts pixels")
	}
	size := drawing.Transform.Size
	if !finite(size.Width) || !finite(size.Height) || size.Width <= 0 || size.Height <= 0 || size.Width > 1e7*4/3 || size.Height > 1e7*4/3 {
		return nil, fmt.Errorf("invalid native image layout size")
	}
	if err := validateString(drawing.Description); err != nil {
		return nil, err
	}
	if err := validateString(drawing.Title); err != nil {
		return nil, err
	}
	im := &Image{Data: data, MediaType: "image/" + kind, Alt: drawing.Description, Width: size.Width * 3 / 4, Height: size.Height * 3 / 4}
	if binding, ok := r.imageBindings[id]; ok {
		candidate := *im
		candidate.Width = binding.Width
		candidate.Height = binding.Height
		projected, err := vdocsIntrinsicImageSize(&candidate, cfg)
		if err == nil && binding.Container != nil {
			width, height, fitErr := fitImageBox(&candidate, cfg.Width, cfg.Height, binding.Container.Width, binding.Container.Height)
			err = fitErr
			projected = vdocsImageSize{Width: width * 4 / 3, Height: height * 4 / 3}
		}
		if err == nil && binding.Hash == vdocsImageHash(data) && binding.NativeWidth == size.Width && binding.NativeHeight == size.Height && projected == size {
			im.Width = binding.Width
			im.Height = binding.Height
		} else {
			r.doc.warn("colleaf_stale_image_geometry", "Stale image size intent ignored; actual native asset and resized geometry are retained")
		}
	}
	r.presentationLosses(raw, []string{"drawingId", "drawingType", "layoutType", "unitId", "subUnitId", "imageSourceType", "source", "base64Cache", "description", "docTransform", "opacity", "hidden", "srcRect", "prstGeom", "title", "manydocOrigin"}, "colleaf_image_presentation")
	if drawing.Title != "" {
		r.doc.warn("colleaf_image_title", "Native image title is not represented separately; description/alternative text and image bytes are retained")
	}
	r.images[id] = im
	copy := *im
	return &copy, nil
}
func (r *vdocsBodyReader) imageAnchorVisible(position int, p vdocsParagraphRecord) error {
	resolver := vdocsStyleResolver{named: r.settings.Styles}
	named, err := resolver.resolve(p.StyleID, 0)
	if err != nil {
		return err
	}
	base, err := vdocsStyleObject(r.settings.DefaultText)
	if err != nil {
		return err
	}
	namedText, err := vdocsStyleObject(named.TextStyle)
	if err != nil {
		return err
	}
	props, err := vdocsStyleObject(r.settings.DefaultParagraph)
	if err != nil {
		return err
	}
	inherited, err := vdocsStyleObject(named.ParagraphStyle)
	if err != nil {
		return err
	}
	direct, err := vdocsStyleObject(p.Style)
	if err != nil {
		return err
	}
	props = mergeColleafStyle(mergeColleafStyle(props, inherited), direct)
	paragraphText, err := vdocsStyleObject(props["textStyle"])
	if err != nil {
		return err
	}
	style := mergeColleafStyle(mergeColleafStyle(base, namedText), paragraphText)
	i := sort.Search(len(r.spans), func(i int) bool { return r.spans[i].end > position })
	if i < len(r.spans) && r.spans[i].record >= 0 {
		run := r.body.TextRuns[r.spans[i].record]
		character, err := resolver.resolve(run.StyleID, 0)
		if err != nil {
			return err
		}
		inherited, err := vdocsStyleObject(character.TextStyle)
		if err != nil {
			return err
		}
		direct, err := vdocsStyleObject(run.TS)
		if err != nil {
			return err
		}
		style = mergeColleafStyle(mergeColleafStyle(style, inherited), direct)
		if run.StyleID != "" || len(direct) > 0 {
			r.doc.warn("colleaf_image_anchor_style", "Native inline drawing text style is normalized to standalone image flow; image bytes and geometry are retained")
		}
	}
	if raw, ok := style["hidden"]; ok && string(raw) != "null" {
		var hidden bool
		if err := json.Unmarshal(raw, &hidden); err != nil {
			return err
		}
		if hidden {
			return fmt.Errorf("%w: hidden native image anchor needs visibility mapping", ErrUnsupported)
		}
	}
	return nil
}

func (r *vdocsBodyReader) imageParagraph(start, end int, p vdocsParagraphRecord) ([]Block, error) {
	authoredPresentation := len(p.Style) > 0 || p.StyleID != ""
	var positions []int
	first := sort.SearchInts(r.atomPositions, start)
	// atomPositions is sorted and shared with note/style fragment translation.
	for _, pos := range r.atomPositions[first:] {
		if pos >= end {
			break
		}
		atom := r.index.blocks[pos]
		if atom.Type != nil && *atom.Type == 0 {
			positions = append(positions, pos)
		}
	}
	if len(positions) == 0 {
		return r.textParagraph(start, end, p)
	}
	var blocks []Block
	if positions[0] == start {
		empty, err := r.textParagraph(start, start, p)
		if err != nil {
			return nil, err
		}
		if len(empty) > 0 && (empty[0].PageBreakBefore || len(p.Bullet) > 0) {
			blocks = append(blocks, empty...)
		}
	}
	cursor := start
	for _, pos := range positions {
		if cursor < pos {
			part, err := r.textParagraph(cursor, pos, p)
			if err != nil {
				return nil, err
			}
			blocks = append(blocks, part...)
		}
		atom := r.index.blocks[pos]
		a := sort.Search(len(r.ranges), func(i int) bool { return r.ranges[i].EndIndex >= pos })
		if a < len(r.ranges) && r.ranges[a].StartIndex <= pos {
			return nil, fmt.Errorf("%w: native linked/annotated image needs relationship mapping", ErrUnsupported)
		}
		if err := r.imageAnchorVisible(pos, p); err != nil {
			return nil, err
		}
		image, err := r.nativeImage(atom.ID)
		if err != nil {
			return nil, err
		}
		props, err := vdocsStyleObject(r.settings.Drawings[atom.ID])
		if err != nil {
			return nil, err
		}
		origin, err := readColleafOriginProfile(props["manydocOrigin"], vdocsImageOriginProfile)
		if err != nil {
			return nil, err
		}
		blocks = append(blocks, Block{Kind: Picture, Image: image, Anchors: r.containerPoints[pos], vdocsImageOrigin: origin})
		cursor = pos + 1
		properties, err := vdocsStyleObject(p.Style)
		if err != nil {
			return nil, err
		}
		properties["pageBreakBefore"] = json.RawMessage("0")
		p.Style = sfAttrs(properties)
	}
	if cursor < end {
		part, err := r.textParagraph(cursor, end, p)
		if err != nil {
			return nil, err
		}
		blocks = append(blocks, part...)
	}
	if len(positions) > 1 || start != positions[0] || end != positions[0]+1 || len(p.Bullet) > 0 {
		r.doc.warn("colleaf_inline_image_reflow", "Native inline images and text sharing one paragraph become ordered SF blocks; image bytes, dimensions and visible text are retained")
	}
	after, err := r.cursorBlocks(end, "boundary after image paragraph", false)
	if err != nil {
		return nil, err
	}
	blocks = append(blocks, after...)
	if authoredPresentation {
		r.doc.warn("colleaf_image_paragraph", "Native picture paragraph presentation is normalized to SF image block flow; image and supported surrounding text remain")
	}
	return blocks, nil
}

const vdocsDocumentResource = "manydoc.document-properties.v1"

type vdocsPageProperties struct {
	PageSize       *vdocsImageSize `json:"pageSize,omitempty"`
	Top            *float64        `json:"marginTop,omitempty"`
	Right          *float64        `json:"marginRight,omitempty"`
	Bottom         *float64        `json:"marginBottom,omitempty"`
	Left           *float64        `json:"marginLeft,omitempty"`
	HeaderDistance *float64        `json:"marginHeader,omitempty"`
	FooterDistance *float64        `json:"marginFooter,omitempty"`
	Orient         *int            `json:"pageOrient,omitempty"`
	Flavor         *int            `json:"documentFlavor,omitempty"`
	DefaultHeader  *string         `json:"defaultHeaderId,omitempty"`
	DefaultFooter  *string         `json:"defaultFooterId,omitempty"`
	FirstHeader    *string         `json:"firstPageHeaderId,omitempty"`
	FirstFooter    *string         `json:"firstPageFooterId,omitempty"`
	EvenHeader     *string         `json:"evenPageHeaderId,omitempty"`
	EvenFooter     *string         `json:"evenPageFooterId,omitempty"`
	FirstPage      *int            `json:"useFirstPageHeaderFooter,omitempty"`
	EvenOdd        *int            `json:"evenAndOddHeaders,omitempty"`
}
type vdocsSectionProperties struct {
	vdocsPageProperties
	Start *int `json:"sectionType,omitempty"`
	Next  *int `json:"sectionTypeNext,omitempty"`
}
type vdocsRunningStory struct {
	HeaderID     string                     `json:"headerId,omitempty"`
	FooterID     string                     `json:"footerId,omitempty"`
	Body         vdocsBody                  `json:"body"`
	Tables       map[string]json.RawMessage `json:"tableSource,omitempty"`
	Lists        map[string]json.RawMessage `json:"lists,omitempty"`
	Drawings     map[string]json.RawMessage `json:"drawings,omitempty"`
	DrawingOrder []string                   `json:"drawingsOrder,omitempty"`
}
type vdocsLayoutBinding struct {
	ID            string              `json:"id"`
	Original      *Layout             `json:"original"`
	Native        vdocsPageProperties `json:"native"`
	ImplicitStart bool                `json:"implicitStart,omitempty"`
	Empty         bool                `json:"empty,omitempty"`
}
type vdocsDocumentIntent struct {
	Evidence     *vdocsDocumentEvidence `json:"evidence,omitempty"`
	Unresolved   []string               `json:"unresolved,omitempty"`
	Author       string                 `json:"author,omitempty"`
	Subject      string                 `json:"subject,omitempty"`
	EmptyTitle   bool                   `json:"emptyTitle,omitempty"`
	Sectioned    bool                   `json:"sectioned,omitempty"`
	Layouts      []vdocsLayoutBinding   `json:"layouts"`
	EmptyStories map[string]bool        `json:"emptyStories,omitempty"`
}

func vdocsFloat(v float64) *float64 { return &v }
func vdocsInt(v int) *int           { return &v }
func vdocsString(v string) *string  { return &v }
func vdocsPageGeometry(p vdocsPageProperties) vdocsPageProperties {
	p.DefaultHeader = nil
	p.DefaultFooter = nil
	p.FirstHeader = nil
	p.FirstFooter = nil
	p.EvenHeader = nil
	p.EvenFooter = nil
	p.FirstPage = nil
	p.EvenOdd = nil
	p.Flavor = nil
	return p
}
func vdocsLayoutProjection(l Layout) vdocsPageProperties {
	g := pageLayout(l)
	return vdocsPageProperties{PageSize: &vdocsImageSize{g.width * 4 / 3, g.height * 4 / 3}, Top: vdocsFloat(g.margins.Top * 4 / 3), Right: vdocsFloat(g.margins.Right * 4 / 3), Bottom: vdocsFloat(g.margins.Bottom * 4 / 3), Left: vdocsFloat(g.margins.Left * 4 / 3), HeaderDistance: vdocsFloat(g.headerDistance() * 4 / 3), FooterDistance: vdocsFloat(g.footerDistance() * 4 / 3), Orient: vdocsInt(0)}
}
func vdocsLayoutFromProperties(p vdocsPageProperties, previous Layout) (Layout, error) {
	g := pageLayout(previous)
	l := Layout{Width: g.width, Height: g.height, Margins: &PageMargins{g.margins.Top, g.margins.Right, g.margins.Bottom, g.margins.Left}, HeaderDistance: vdocsFloat(g.headerDistance()), FooterDistance: vdocsFloat(g.footerDistance())}
	if p.PageSize != nil {
		l.Width = p.PageSize.Width * .75
		l.Height = p.PageSize.Height * .75
		if l.Width <= 0 || l.Height <= 0 {
			return Layout{}, fmt.Errorf("native page dimensions must be positive")
		}
	}
	for _, pair := range []struct {
		source *float64
		target *float64
	}{{p.Top, &l.Margins.Top}, {p.Right, &l.Margins.Right}, {p.Bottom, &l.Margins.Bottom}, {p.Left, &l.Margins.Left}, {p.HeaderDistance, l.HeaderDistance}, {p.FooterDistance, l.FooterDistance}} {
		if pair.source != nil {
			*pair.target = *pair.source * .75
		}
	}
	if p.Orient != nil && *p.Orient != 0 {
		if *p.Orient != 1 {
			return Layout{}, fmt.Errorf("unsupported native page orientation")
		}
		l.Width, l.Height = l.Height, l.Width
	}
	if err := validateLayout(l); err != nil {
		return Layout{}, err
	}
	return l, nil
}
func vdocsSectionStart(start SectionStart) (int, error) {
	switch start {
	case "", SectionNextPage:
		return 3, nil
	case SectionContinuous:
		return 1, nil
	case SectionEvenPage:
		return 4, nil
	case SectionOddPage:
		return 5, nil
	}
	return 0, fmt.Errorf("unsupported section start")
}
func vdocsStartFromNative(value int) (SectionStart, error) {
	switch value {
	case 0, 3:
		return SectionNextPage, nil
	case 1:
		return SectionContinuous, nil
	case 4:
		return SectionEvenPage, nil
	case 5:
		return SectionOddPage, nil
	}
	return "", fmt.Errorf("%w: native section start%d", ErrUnsupported, value)
}
func vdocsEmptyBlocks(blocks []Block) bool {
	return len(blocks) == 1 && reflect.DeepEqual(blocks[0], Block{Kind: Paragraph})
}

func vdocsResourceMap(records []vdocsResource, name string, target any) error {
	found := false
	for _, r := range records {
		if r.Name != name {
			continue
		}
		if found {
			return fmt.Errorf("duplicate native intent record %q", name)
		}
		found = true
		if err := validateTypedJSON(context.Background(), []byte(r.Data), reflect.TypeOf(target)); err != nil {
			return err
		}
		if err := strictSFAttrs(json.RawMessage(r.Data), target); err != nil {
			return err
		}
	}
	return nil
}

func vdocsReplaceResource(records []vdocsResource, name string, value any) []vdocsResource {
	var out []vdocsResource
	for _, r := range records {
		if r.Name != name {
			out = append(out, r)
		}
	}
	return append(out, vdocsResource{Name: name, Data: string(sfAttrs(value))})
}

func encodeColleafDocument(ctx context.Context, d *Document) (vdocsNoteSnapshot, error) {
	if err := d.Validate(); err != nil {
		return vdocsNoteSnapshot{}, err
	}
	resolved := resolveNumberingDocument(d)
	plain := *resolved
	plain.Notes = append([]Note(nil), resolved.Notes...)
	for i := range plain.Notes {
		plain.Notes[i].Source = nil
		plain.Notes[i].LabelSource = nil
		plain.Notes[i].KindInferred = false
	}
	plain.PDFNavigation = nil
	if hasSectionBlocks(d.Blocks) {
		plain.Blocks = resolved.Blocks[0].Section.Blocks
		plain.Headers = nil
		plain.Footers = nil
		plain.EvenAndOddHeaders = false
	}

	partial, err := catalogDestinations(&plain, false)
	if err != nil {
		return vdocsNoteSnapshot{}, err
	}
	plain.UnresolvedDestinations = append([]string(nil), plain.UnresolvedDestinations...)
	var omitted []string
	for id := range partial.links {
		if !partial.anchors[id] && !partial.missing[id] {
			omitted = append(omitted, id)
		}
	}
	sort.Strings(omitted)
	plain.UnresolvedDestinations = append(plain.UnresolvedDestinations, omitted...)
	catalog, err := catalogNotes(resolved, false)
	if err != nil {
		return vdocsNoteSnapshot{}, err
	}
	out, err := encodeColleafNotesWithCatalog(ctx, &plain, catalog, vdocsCitationProjection(resolved, catalog))
	if err != nil {
		return out, err
	}
	out, err = openColleafIntent(ctx, out)
	if err != nil {
		return out, err
	}
	evidence, err := vdocsEvidence(d)
	if err != nil {
		return out, err
	}
	if err = collectColleafNoteOrigins(resolved, &evidence); err != nil {
		return out, err
	}
	intent := vdocsDocumentIntent{Evidence: evidence, Unresolved: append([]string(nil), d.UnresolvedDestinations...), Author: d.Metadata.Author, Subject: d.Metadata.Subject, EmptyTitle: d.Metadata.Title == "", Sectioned: hasSectionBlocks(d.Blocks), EmptyStories: map[string]bool{}}
	out.Title = d.Metadata.Title
	if out.Title == "" {
		out.Title = "Untitled"
	}
	root := vdocsLayoutProjection(d.Layout)
	root.Flavor = vdocsInt(1)
	root.FirstPage = vdocsInt(0)
	root.EvenOdd = vdocsInt(0)
	if d.EvenAndOddHeaders {
		root.EvenOdd = vdocsInt(1)
	}
	rootOriginal := cloneLayout(d.Layout)
	intent.Layouts = append(intent.Layouts, vdocsLayoutBinding{ID: "document", Original: &rootOriginal, Native: vdocsPageGeometry(root)})
	ownership := vdocsListOwnership{}
	if err = vdocsResourceMap(out.Resources, vdocsListResource, &ownership); err != nil {
		return out, err
	}
	if ownership.Stories == nil {
		ownership.Stories = map[string][]vdocsListBinding{}
	}
	geometry := vdocsImageGeometry{}
	if err = vdocsResourceMap(out.Resources, vdocsImageResource, &geometry); err != nil {
		return out, err
	}
	if geometry.Stories == nil {
		geometry.Stories = map[string][]vdocsImageBinding{}
	}
	out.Headers = map[string]vdocsRunningStory{}
	out.Footers = map[string]vdocsRunningStory{}
	cache := outputImageCache{}
	runningBlocks := map[string][]Block{}
	storySerial := 0
	encodeStory := func(story *RunningStory, header bool) (*string, error) {
		if story == nil {
			return nil, nil
		}
		storySerial++
		prefix := "footer"
		if header {
			prefix = "header"
		}
		id := fmt.Sprintf("manydoc-%s-%d", prefix, storySerial)
		runningBlocks[id] = story.Blocks
		e, err := encodeColleafAssetBody(ctx, story.Blocks, nil, out.ID, id, cache)
		if err != nil {
			return nil, err
		}
		n := vdocsRunningStory{Body: e.Body, Tables: e.Tables, Lists: e.Lists, Drawings: e.Drawings, DrawingOrder: e.DrawingOrder}
		if header {
			n.HeaderID = id
			out.Headers[id] = n
		} else {
			n.FooterID = id
			out.Footers[id] = n
		}
		ownership.Stories[id] = e.ListsOwnership
		geometry.Stories[id] = e.ImageGeometry
		if len(story.Blocks) == 0 {
			intent.EmptyStories[id] = true
		}
		return vdocsString(id), nil
	}
	encodeStories := func(headers, footers RunningStories, p *vdocsPageProperties) error {
		targets := []**string{&p.DefaultHeader, &p.FirstHeader, &p.EvenHeader, &p.DefaultFooter, &p.FirstFooter, &p.EvenFooter}
		sources := []*RunningStory{headers.Default, headers.First, headers.Even, footers.Default, footers.First, footers.Even}
		for i, story := range sources {
			id, err := encodeStory(story, i < 3)
			if err != nil {
				return err
			}
			*targets[i] = id
		}
		return nil
	}
	if !intent.Sectioned {
		if len(d.Headers) > 0 {
			root.DefaultHeader, err = encodeStory(&RunningStory{Blocks: d.Headers}, true)
			if err != nil {
				return out, err
			}
		}
		if len(d.Footers) > 0 {
			root.DefaultFooter, err = encodeStory(&RunningStory{Blocks: d.Footers}, false)
			if err != nil {
				return out, err
			}
		}
	}
	ids := map[noteKey]string{}
	var identities vdocsNoteIdentities
	if err = vdocsResourceMap(out.Resources, vdocsNoteIdentityResource, &identities); err != nil {
		return out, err
	}
	for _, b := range identities.Bindings {
		ids[noteKey{b.Kind, b.ID}] = b.NativeID
	}
	w := vdocsBodyWriter{citationProjection: vdocsCitationProjection(resolved, catalog), ctx: ctx, noteIDs: ids, documentID: out.ID, imageCache: cache, tables: map[string]json.RawMessage{}, lists: map[string]json.RawMessage{}, drawings: map[string]json.RawMessage{}, body: vdocsBody{TextRuns: []vdocsTextRun{}}}
	sectionBlocks := resolved.Blocks
	if !intent.Sectioned {
		sectionBlocks = []Block{{Kind: SectionBlock, Section: &Section{Blocks: resolved.Blocks}}}
	}
	for i, b := range sectionBlocks {
		section := b.Section
		sectionOffset := w.offset
		firstRange := len(w.body.CustomRanges)
		if err = w.blocks(section.Blocks, 0); err != nil {
			return out, err
		}
		record := &w.body.SectionBreaks[len(w.body.SectionBreaks)-1]
		if b.Source != nil {
			origin, err := newColleafFlowOrigin(b)
			if err != nil {
				return out, err
			}
			if record.Extra == nil {
				record.Extra = map[string]json.RawMessage{}
			}
			record.Extra["manydocFlowOrigin"] = sfAttrs(origin)
		}
		oldSectionID := record.ID
		record.ID = fmt.Sprintf("manydoc-section-%d", i+1)
		if err = rebindColleafPointScope(&w.body, oldSectionID, record.ID, firstRange); err != nil {
			return out, err
		}
		if err = w.containerAnchors(b.Anchors, sectionOffset, "section", record.ID); err != nil {
			return out, err
		}
		start, err := vdocsSectionStart(section.Start)
		if err != nil {
			return out, err
		}
		record.Start = &start
		record.FirstPage = vdocsInt(0)
		if section.FirstPage {
			record.FirstPage = vdocsInt(1)
		}
		layoutBinding := vdocsLayoutBinding{ID: record.ID, ImplicitStart: section.Start == "", Empty: len(section.Blocks) == 0}
		if section.Layout != nil {
			l := cloneLayout(*section.Layout)
			record.vdocsPageProperties = vdocsLayoutProjection(l)
			layoutBinding.Original = &l
			layoutBinding.Native = vdocsPageGeometry(record.vdocsPageProperties)
			record.FirstPage = vdocsInt(0)
			if section.FirstPage {
				record.FirstPage = vdocsInt(1)
			}
		}
		if err = encodeStories(section.Headers, section.Footers, &record.vdocsPageProperties); err != nil {
			return out, err
		}
		intent.Layouts = append(intent.Layouts, layoutBinding)
		w.listDepth = 0
		w.listGroup = ""
	}
	w.body.DataStream = w.stream.String()
	if _, err = indexColleafBody(ctx, w.body); err != nil {
		return out, err
	}
	out.Body = w.body
	out.Tables = w.tables
	out.Lists = w.lists
	out.Drawings = w.drawings
	out.DrawingOrder = w.drawingOrder
	ownership.Stories[""] = w.listBindings
	geometry.Stories[""] = w.imageGeometry

	storyIDs := make([]string, 0, len(out.Headers)+len(out.Footers))
	for id := range out.Headers {
		storyIDs = append(storyIDs, id)
	}
	for id := range out.Footers {
		storyIDs = append(storyIDs, id)
	}
	sort.Strings(storyIDs)
	for _, id := range storyIDs {
		story, header := out.Headers[id]
		if !header {
			story = out.Footers[id]
		}
		tableNames := map[string]string{}
		for old, raw := range story.Tables {
			scoped := id + "-" + old
			tableNames[old] = scoped
			var data map[string]json.RawMessage
			if err := json.Unmarshal(raw, &data); err != nil {
				return out, err
			}
			data["tableId"] = sfAttrs(scoped)
			out.Tables[scoped] = sfAttrs(data)
		}
		for i := range story.Body.Tables {
			story.Body.Tables[i].ID = tableNames[story.Body.Tables[i].ID]
		}
		for i := range story.Body.CustomRanges {
			a := &story.Body.CustomRanges[i]
			if a.Type != 3 {
				continue
			}
			p, err := readColleafBookmark(*a)
			if err != nil {
				return out, err
			}
			if p.Hint != nil && p.Hint.Role == "table" {
				p.Hint.OwnerID = tableNames[p.Hint.OwnerID]
				a.Properties = sfAttrs(p)
			}
		}
		listNames := map[string]string{}
		for old, raw := range story.

			// Keep the geometry arithmetic explicit: native drawing boxes use pixels while
			// SF dimensions use points. No application image/file quotas are introduced.

			// Whole native document composition adds real page/section/story relationships
			// around the already-qualified main/note text, table, list and image workers.

			// preliminary note composition has no physical navigation authority

			// The preliminary note composition uses only the first section body. Its
			// omitted root/other-section anchors are not unresolved in the final document.
			// Admit those links only in this temporary validated projection.

			// Executable SDK readers use the document catalogs for running stories;
			// local header/footer catalogs alone are not enough for rendering.

			Lists {
			scoped := id + "-" + old
			listNames[old] = scoped
			var data vdocsNativeList
			if err := json.Unmarshal(raw, &data); err != nil {
				return out, err
			}
			data.Type = scoped
			out.Lists[scoped] = sfAttrs(data)
		}
		for j := range story.Body.CustomRanges {
			if story.Body.CustomRanges[j].Type == 0 {
				story.Body.CustomRanges[j].ID = id + "-" + story.Body.CustomRanges[j].ID
			}
		}
		for i, p := range story.Body.Paragraphs {
			if len(p.Bullet) > 0 {
				var bullet vdocsBullet
				if err := json.Unmarshal(p.Bullet, &bullet); err != nil {
					return out, err
				}
				bullet.Type = listNames[bullet.Type]
				bullet.ID = id + "-" + bullet.ID
				story.Body.Paragraphs[i].Bullet = sfAttrs(bullet)
			}
		}
		for i, b := range ownership.Stories[id] {
			for j := range b.Items {
				if b.Items[j].Type != "" {
					b.Items[j].Type = listNames[b.Items[j].Type]
				}
			}
			if len(b.Projections) > 0 {
				mapped := map[string]vdocsNumberProjection{}
				for name, p := range b.Projections {
					mapped[listNames[name]] = p
				}
				b.Projections = mapped
			}
			b.Type = listNames[b.Type]
			b.Group = id + "-" + b.Group
			ownership.Stories[id][i] = b
		}
		for drawing, raw := range story.Drawings {
			if _, ok := out.Drawings[drawing]; ok {
				return out, fmt.Errorf("duplicate native global drawing identity")
			}
			out.Drawings[drawing] = raw
		}
		out.HeaderFooterDrawingOrder = append(out.HeaderFooterDrawingOrder, story.DrawingOrder...)
		story.Tables = nil
		story.Lists = nil
		story.Drawings = nil
		story.DrawingOrder = nil
		if header {
			out.Headers[id] = story
		} else {
			out.Footers[id] = story
		}
	}
	styleOutput, _ := vdocsStyleObject(sfAttrs(root))
	styleOutput["autoHyphenation"] = json.RawMessage("0")
	styleOutput["consecutiveHyphenLimit"] = json.RawMessage("0")
	styleOutput["doNotHyphenateCaps"] = json.RawMessage("0")
	out.DocumentStyle = sfAttrs(styleOutput)
	out.Resources = vdocsReplaceResource(out.Resources, vdocsListResource, ownership)
	if err = fitColleafDocumentImages(ctx, resolved, &out, &geometry, runningBlocks, cache); err != nil {
		return out, err
	}
	out.Resources = vdocsReplaceResource(out.Resources, vdocsImageResource, geometry)
	if err = collectColleafFlowOrigins(&out, &intent.Evidence); err != nil {
		return out, err
	}
	if err = collectColleafAnchorOrigins(&out, &intent.Evidence); err != nil {
		return out, err
	}
	if err = collectColleafListOrigins(&out, &intent.Evidence); err != nil {
		return out, err
	}
	if err = collectColleafImageOrigins(&out, &intent.Evidence); err != nil {
		return out, err
	}
	if err = collectColleafTableOrigins(&out, &intent.Evidence); err != nil {
		return out, err
	}
	if err = collectColleafRunOrigins(&out, &intent.Evidence); err != nil {
		return out, err
	}
	if err = collectColleafOrigins(&out, &intent.Evidence); err != nil {
		return out, err
	}
	out.Resources = vdocsReplaceResource(out.Resources, vdocsDocumentResource, intent)
	if err = scopeColleafBookmarkIDs(ctx, &out, d); err != nil {
		return out, err
	}
	if err = sealColleafIntent(&out); err != nil {
		return out, err
	}
	// Univer's drawing controller initializes its registry from this built-in
	// resource, not the persisted source map alone. Mirror only root sources:
	// notes retain their own sources and are collected by the native controller.
	if len(out.Drawings) > 0 || len(out.Notes) > 0 {
		data := out.Drawings
		if data == nil {
			data = map[string]json.RawMessage{}
		}
		order := append([]string{}, out.DrawingOrder...)
		out.Resources = vdocsReplaceResource(out.Resources, "DOC_DRAWING_PLUGIN", struct {
			Data  map[string]json.RawMessage `json:"data"`
			Order []string                   `json:"order"`
		}{data, order})
	}
	return out, nil
}

func decodeColleafDocument(ctx context.Context, input vdocsNoteSnapshot) (*Document, error) {
	opened, err := openColleafIntent(ctx, input)
	if err != nil {
		return nil, err
	}
	opened, hyperlinkDiagnostics, err := applyColleafHyperlinkResource(ctx, opened)
	if err != nil {
		return nil, err
	}
	for _, r := range opened.Resources {
		if r.Name == "DOC_DRAWING_PLUGIN" {
			if err := checkColleafDrawingMirror(opened, r); err != nil {
				return nil, err
			}
		}
	}
	var root vdocsPageProperties
	if len(opened.DocumentStyle) > 0 {
		if err = json.Unmarshal(opened.DocumentStyle, &root); err != nil {
			return nil, err
		}
	}
	if (root.Flavor == nil || *root.Flavor == 0) && root.PageSize == nil {
		return nil, fmt.Errorf("%w: native unpaginated canvas needs layout mapping", ErrUnsupported)
	}
	if root.Flavor != nil && *root.Flavor != 0 && *root.Flavor != 1 {
		return nil, fmt.Errorf("%w: native nontraditional document flavor", ErrUnsupported)
	}
	opened, citations, err := openColleafCitations(ctx, opened)
	if err != nil {
		return nil, err
	}
	numberPlan, err := planColleafNoteNumbers(ctx, opened)
	if err != nil {
		return nil, err
	}
	citations.adaptPlan(&numberPlan)
	var intent vdocsDocumentIntent
	if err = vdocsResourceMap(opened.Resources, vdocsDocumentResource, &intent); err != nil {
		return nil, err
	}
	hints := map[string]vdocsLayoutBinding{}
	for _, h := range intent.Layouts {
		if _, ok := hints[h.ID]; ok {
			return nil, fmt.Errorf("duplicate native layout intent")
		}
		hints[h.ID] = h
	}
	// Traditional native defaults are measured against the pinned executable reader.
	baseline := Layout{Width: 794 * .75, Height: 1124 * .75, Margin: 54, HeaderDistance: vdocsFloat(22.5), FooterDistance: vdocsFloat(22.5)}
	layout, err := vdocsLayoutFromProperties(root, baseline)
	if err != nil {
		return nil, err
	}
	var diagnostics []Diagnostic
	restoreLayout := func(id string, p vdocsPageProperties, actual Layout) *Layout {
		if h, ok := hints[id]; ok {
			if reflect.DeepEqual(h.Native, vdocsPageGeometry(p)) {
				if h.Original == nil {
					return nil
				}
				l := cloneLayout(*h.Original)
				return &l
			}
			diagnostics = append(diagnostics, Diagnostic{Code: "colleaf_stale_layout", Message: "Stale page size intent ignored; actual native layout is retained"})
		}
		l := cloneLayout(actual)
		return &l
	}
	rootLayout := restoreLayout("document", root, layout)
	if rootLayout == nil {
		rootLayout = &layout
	}
	ownership := vdocsListOwnership{}
	if err = vdocsResourceMap(opened.Resources, vdocsListResource, &ownership); err != nil {
		return nil, err
	}
	geometry := vdocsImageGeometry{}
	if err = vdocsResourceMap(opened.Resources, vdocsImageResource, &geometry); err != nil {
		return nil, err
	}
	aliases, err := vdocsBookmarkAliases(opened.Body)
	if err != nil {
		return nil, err
	}
	settings := vdocsTextContext{NavigationAliases: aliases, ImageCache: outputImageCache{}, Styles: opened.Styles, NoteLabels: numberPlan.Overrides, Citations: citations}
	styleFields, err := vdocsStyleObject(opened.DocumentStyle)
	if err != nil {
		return nil, err
	}
	settings.DefaultText = styleFields["textStyle"]
	settings.DefaultParagraph = styleFields["defaultParagraphStyle"]
	// Notes are decoded once by the existing composition. Main root sections are
	// then visited against the same validated global stream/source indexes.
	stub := opened
	stub.Body = vdocsBody{DataStream: "\r\n"}
	stub.Tables = nil
	stub.Lists = nil
	stub.Drawings = nil
	stub.DrawingOrder = nil
	var records []vdocsResource
	for _, r := range opened.Resources {
		if r.Name != vdocsDocumentResource && r.Name != "DOC_DRAWING_PLUGIN" {
			records = append(records, r)
		}
	}
	stub.Resources = records
	stubOwnership := vdocsListOwnership{Stories: map[string][]vdocsListBinding{}}
	stubGeometry := vdocsImageGeometry{Stories: map[string][]vdocsImageBinding{}}
	for id := range opened.Notes {
		stubOwnership.Stories[id] = ownership.Stories[id]
		stubGeometry.Stories[id] = geometry.Stories[id]
	}
	stub.Resources = vdocsReplaceResource(stub.Resources, vdocsListResource, stubOwnership)
	stub.Resources = vdocsReplaceResource(stub.Resources, vdocsImageResource, stubGeometry)
	noteSettings := settings
	noteSettings.DeferNoteNumbering = true
	doc, err := decodeColleafNotes(ctx, stub, noteSettings)
	if err != nil {
		return nil, err
	}
	styleKnown := map[string]bool{}
	for _, key := range []string{"pageSize", "pageOrient", "documentFlavor", "marginTop", "marginRight", "marginBottom", "marginLeft", "marginHeader", "marginFooter", "defaultHeaderId", "defaultFooterId", "firstPageHeaderId", "firstPageFooterId", "evenPageHeaderId", "evenPageFooterId", "useFirstPageHeaderFooter", "evenAndOddHeaders", "textStyle", "defaultParagraphStyle"} {
		styleKnown[key] = true
	}
	if raw, ok := styleFields["autoHyphenation"]; ok {
		enabled, err := vdocsBool(raw)
		if err != nil {
			return nil, err
		}
		if !enabled {
			styleKnown["autoHyphenation"] = true
			styleKnown["consecutiveHyphenLimit"] = true
			styleKnown["doNotHyphenateCaps"] = true
		}
	}
	var styleKeys []string
	for key := range styleFields {
		if !styleKnown[key] {
			styleKeys = append(styleKeys, key)
		}
	}
	sort.Strings(styleKeys)
	for _, key := range styleKeys {
		doc.warn("colleaf_document_style", "Native document property "+key+" is not represented; supported page geometry and content are retained")
	}
	doc.Blocks = nil
	doc.Layout = *rootLayout
	doc.Metadata = Metadata{Title: input.Title, Author: intent.Author, Subject: intent.Subject}
	if intent.EmptyTitle && input.Title == "Untitled" {
		doc.Metadata.Title = ""
	}
	if root.EvenOdd != nil {
		if *root.EvenOdd < 0 || *root.EvenOdd > 1 {
			return nil, fmt.Errorf("invalid native even/odd flag")
		}
		doc.EvenAndOddHeaders = *root.EvenOdd == 1
	}
	noteIDs := map[string]noteKey{}
	var identities vdocsNoteIdentities
	_ = vdocsResourceMap(opened.Resources, vdocsNoteIdentityResource, &identities)
	for id, n := range opened.Notes {
		noteIDs[id] = noteKey{n.Kind, id}
	}
	for _, binding := range identities.Bindings {
		if n, ok := opened.Notes[binding.NativeID]; ok && n.Kind == binding.Kind {
			noteIDs[binding.NativeID] = noteKey{binding.Kind, binding.ID}
		}
	}
	usedTables := map[string]bool{}
	usedLists := map[string]bool{}
	usedDrawings := map[string]bool{}
	selectSources := func(body vdocsBody, localTables, localLists, localDrawings map[string]json.RawMessage) (map[string]json.RawMessage, map[string]json.RawMessage, map[string]json.RawMessage, error) {
		tables, lists, drawings := map[string]json.RawMessage{}, map[string]json.RawMessage{}, map[string]json.RawMessage{}
		for _, table := range body.Tables {
			raw, ok := localTables[table.ID]
			if !ok {
				raw, ok = opened.Tables[table.ID]
				usedTables[table.ID] = true
			}
			if !ok {
				return nil, nil, nil, fmt.Errorf("native story table lacks source")
			}
			tables[table.ID] = raw
		}
		for _, p := range body.Paragraphs {
			if len(p.Bullet) > 0 && string(p.Bullet) != "null" {
				var bullet vdocsBullet
				if err := json.Unmarshal(p.Bullet, &bullet); err != nil {
					return nil, nil, nil, err
				}
				raw, ok := localLists[bullet.Type]
				if !ok {
					raw, ok = opened.Lists[bullet.Type]
					usedLists[bullet.Type] = true
				}
				if !ok {
					return nil, nil, nil, fmt.Errorf("native story list lacks source")
				}
				lists[bullet.Type] = raw
			}
		}
		for _, atom := range body.CustomBlocks {
			if atom.Type != nil && *atom.Type == 0 {
				raw, ok := localDrawings[atom.ID]
				if !ok {
					raw, ok = opened.Drawings[atom.ID]
					usedDrawings[atom.ID] = true
				}
				if !ok {
					return nil, nil, nil, fmt.Errorf("native story drawing lacks source")
				}
				drawings[atom.ID] = raw
			}
		}
		// Catalog entries without a stream relationship may contain visible content.
		// Do not hide them by passing only the selected subset to the body reader.

		// Native tables retain a real stream body plus ordinal source records. SF omits
		// vertically covered cells; native rows require zero-row-span continuation tokens.

		// The shared SF grid tolerates trailing uncovered slots in ragged rows;
		// native rectangular rows must never invent a missing visible cell.

		// Native tables are children of an anchoring paragraph, whose CR is structural.

		// Decode a paragraph fragment using original effective intervals and hyperlink
		// identities. Offsets are translated only after intersecting scalar-safe ranges.

		// Preserve authored suffix text/paragraph properties. An otherwise empty
		// bare anchoring CR is not an extra SF paragraph after every table.

		// A point at a terminal LF has no text carrier. Retain a changed scope at
		// this actual current boundary rather than dropping it.

		// The pinned table layout treats legacy POINT(0) values as geometry
		// pixels too; executable reader probes establish this field-specific rule.

		// Occupancy names the single origin covering each logical slot. Vertical
		// continuation bodies are validated as empty before they can be omitted in SF.

		for id := range localTables {
			if _, ok := tables[id]; !ok {
				return nil, nil, nil, fmt.Errorf("native local table source is orphaned")
			}
		}
		for id := range localDrawings {
			if _, ok := drawings[id]; !ok {
				return nil, nil, nil, fmt.Errorf("native local drawing source is orphaned")
			}
		}
		return tables, lists, drawings, nil
	}
	mainTables, mainLists, mainDrawings, err := selectSources(opened.Body, nil, nil, nil)
	if err != nil {
		return nil, err
	}
	settings.NoteIDs = noteIDs
	settings.Lists = mainLists
	settings.ListBindings = ownership.Stories[""]
	settings.Drawings = mainDrawings
	settings.DrawingOrder = opened.DrawingOrder
	settings.ImageBindings = geometry.Stories[""]
	index, err := indexColleafBody(ctx, opened.Body)
	if err != nil {
		return nil, err
	}
	reader, err := newColleafBodyReader(ctx, opened.Body, settings, mainTables)
	if err != nil {
		return nil, err
	}
	usedStories := map[string]bool{}
	stories := map[string]*RunningStory{}
	readStory := func(id *string, header bool) (*RunningStory, error) {
		if id == nil || *id == "" {
			return nil, nil
		}
		if story, ok := stories[*id]; ok {
			return story, nil
		}
		native, ok := opened.Footers[*id]
		if header {
			native, ok = opened.Headers[*id]
		}
		if !ok {
			return nil, fmt.Errorf("native running story reference lacks body %q", *id)
		}
		if (header && native.HeaderID != *id) || (!header && native.FooterID != *id) {
			return nil, fmt.Errorf("native running story identity mismatch")
		}
		tables, lists, drawings, err := selectSources(native.Body, native.Tables, native.Lists, native.Drawings)
		if err != nil {
			return nil, err
		}
		child := settings
		child.NavigationScope = "footer:" + *id
		if header {
			child.NavigationScope = "header:" + *id
		}
		child.NoteIDs = nil
		child.Lists = lists
		child.ListBindings = ownership.Stories[*id]
		child.Drawings = drawings
		child.DrawingOrder = native.DrawingOrder
		if len(child.DrawingOrder) == 0 {
			for _, id := range opened.HeaderFooterDrawingOrder {
				if _, ok := drawings[id]; ok {
					child.DrawingOrder = append(child.DrawingOrder, id)
				}
			}
		}
		child.ImageBindings = geometry.Stories[*id]
		parsed, err := decodeColleafBody(ctx, native.Body, child, tables)
		if err != nil {
			return nil, err
		}
		doc.Diagnostics = append(doc.Diagnostics, parsed.Diagnostics...)
		blocks := parsed.Blocks
		if intent.EmptyStories[*id] && vdocsEmptyBlocks(blocks) {
			blocks = nil
		}
		story := &RunningStory{Blocks: blocks}
		stories[*id] = story
		usedStories[*id] = true
		return story, nil
	}
	readStories := func(p vdocsPageProperties) (RunningStories, RunningStories, error) {
		var h, f RunningStories
		targets := []**RunningStory{&h.Default, &h.First, &h.Even, &f.Default, &f.First, &f.Even}
		ids := []*string{p.DefaultHeader, p.FirstHeader, p.EvenHeader, p.DefaultFooter, p.FirstFooter, p.EvenFooter}
		for i, id := range ids {
			story, err := readStory(id, i < 3)
			if err != nil {
				return h, f, err
			}
			*targets[i] = story
		}
		return h, f, nil
	}
	rootHeaders,

		// Apply all projections after origin cells exist, including earlier-row
		// origins of covered native cells. Preserve source order within each edge.
		rootFooters, err := readStories(root)
	if err != nil {
		return nil, err
	}
	sectionRecords := map[int]vdocsSectionRecord{}
	for _, record := range opened.Body.SectionBreaks {
		sectionRecords[record.StartIndex] = record
	}
	previous := layout
	for i, node := range index.sections {
		record := sectionRecords[node.end-1]
		var extras []string
		for key := range record.Extra {
			if key == "noteProperties" || key == "manydocFlowOrigin" {
				continue
			}
			extras = append(extras, key)
		}
		sort.Strings(extras)
		for _, key := range extras {
			doc.warn("colleaf_section_style", "Native section property "+key+" is not represented; supported body order and page geometry are retained")
		}
		if record.EvenOdd != nil && ((*record.EvenOdd != 0 && *record.EvenOdd != 1) || (*record.EvenOdd == 1) != doc.EvenAndOddHeaders) {
			return nil, fmt.Errorf("%w: per-section even/odd mode cannot use a single document-wide flag",

				// Neutral flow defaults are represented by SF's ordinary table flow.
				ErrUnsupported)
		}
		actual, err := vdocsLayoutFromProperties(record.vdocsPageProperties, previous)
		if err != nil {
			return nil, err
		}
		start := 3
		if record.Start != nil {
			start = *record.Start
		}
		if i > 0 {
			before := sectionRecords[index.sections[i-1].end-1]
			if before.Next != nil {
				start = *before.Next
			}
		}
		mode, err := vdocsStartFromNative(start)
		if err != nil {
			return nil, err
		}
		blocks, err := reader.sections([]vdocsStreamNode{node}, 0)
		if err != nil {
			return nil, err
		}
		if hint, ok := hints[record.ID]; ok {
			if hint.Empty && vdocsEmptyBlocks(blocks) {
				blocks = nil
			}
			if hint.ImplicitStart && start == 3 {
				mode = ""
			}
		}
		h, f, err := readStories(record.vdocsPageProperties)
		if err != nil {
			return nil, err
		}
		if i == 0 {
			h = inheritStories(h, rootHeaders)
			f = inheritStories(f, rootFooters)
		}
		section := &Section{Layout: restoreLayout(record.ID, record.vdocsPageProperties, actual), Start: mode, Headers: h, Footers: f, Blocks: blocks}
		if record.FirstPage != nil {
			if *record.FirstPage < 0 || *record.FirstPage > 1 {
				return nil, fmt.Errorf("invalid native first-page flag")
			}
			section.FirstPage = *record.FirstPage == 1
		}
		origin, err := readColleafOriginProfile(record.Extra["manydocFlowOrigin"], vdocsFlowOriginProfile)
		if err != nil {
			return nil, err
		}
		doc.Blocks = append(doc.Blocks, Block{Kind: SectionBlock, Section: section, Anchors: reader.sectionPoints[node.start], vdocsFlowOrigin: origin})
		previous = actual
	}
	if len(intent.Layouts) > 0 && !intent.Sectioned && len(doc.Blocks) == 1 && len(doc.Blocks[0].Anchors) == 0 && !doc.EvenAndOddHeaders && !doc.Blocks[0].Section.FirstPage && doc.Blocks[0].Section.Headers.First == nil && doc.Blocks[0].Section.Headers.Even == nil && doc.Blocks[0].Section.Footers.First == nil && doc.Blocks[0].Section.Footers.Even == nil {
		section := doc.Blocks[0].Section
		doc.Blocks = section.Blocks
		if section.Layout != nil {
			doc.Layout = *section.Layout
		}
		if section.Headers.Default != nil {
			doc.Headers = section.Headers.Default.Blocks
		}
		if section.Footers.Default != nil {
			doc.Footers = section.Footers.Default.Blocks
		}
		doc.EvenAndOddHeaders = false
	}
	if len(usedStories) != len(opened.Headers)+len(opened.Footers) {
		return nil, fmt.Errorf("native running source is orphaned")
	}
	if len(usedTables) != len(opened.Tables) || len(usedDrawings) != len(opened.Drawings) {
		return nil, fmt.Errorf("native main source is orphaned")
	}
	autoLabels := map[noteKey]string{}
	for _, binding := range identities.Bindings {
		if n, ok := opened.Notes[binding.NativeID]; ok && n.Kind == binding.Kind && binding.AutoLabel != nil && n.Mark != nil && *n.Mark == *binding.AutoLabel {
			autoLabels[noteKey{binding.Kind, binding.ID}] = *binding.AutoLabel
		}
	}
	applyColleafNoteNumbers(doc, numberPlan, noteIDs)
	citations.restoreDefinitions(doc, noteIDs)
	if err = finishColleafNoteNumbering(doc, autoLabels); err != nil {
		return nil, err
	}
	doc.Diagnostics = append(doc.Diagnostics, hyperlinkDiagnostics...)
	reader.finishStandalone(doc.Blocks)
	reader.finishListPoints()
	doc.Diagnostics = append(doc.Diagnostics, reader.doc.Diagnostics...)
	doc.Diagnostics = append(doc.Diagnostics, diagnostics...)
	if err = citations.collapse(ctx, doc); err != nil {
		return nil, err
	}
	if err = finishColleafNavigation(ctx, opened, doc, intent.Unresolved); err != nil {
		return nil, err
	}
	if err = restoreColleafEvidence(doc, intent.Evidence); err != nil {
		return nil, err
	}
	if err = restoreColleafRunOrigins(doc, intent.Evidence); err != nil {
		return nil, err
	}
	if err = restoreColleafOrigins(doc, intent.Evidence); err != nil {
		return nil, err
	}
	if err = restoreColleafNoteOrigins(doc, intent.Evidence, opened, identities); err != nil {
		return nil, err
	}
	if err = restoreColleafAnchorOrigins(doc, intent.Evidence, opened); err != nil {
		return nil, err
	}
	if err = restoreColleafListOrigins(doc, intent.Evidence, opened); err != nil {
		return nil, err
	}
	if err = restoreColleafImageOrigins(doc, intent.Evidence, opened); err != nil {
		return nil, err
	}
	if err = restoreColleafTableOrigins(doc, intent.Evidence, opened); err != nil {
		return nil, err
	}
	if err = restoreColleafFlowOrigins(doc, intent.Evidence, opened); err != nil {
		return nil, err
	}
	if err = doc.Validate(); err != nil {
		return nil, err
	}
	return doc, nil
}

func (r *vdocsBodyReader) breakParagraph(start, end int, p vdocsParagraphRecord) ([]Block, error) {
	text, err := r.index.text.exclusive(start, end)
	if err != nil {
		return nil, err
	}
	var breaks []int
	offset := start
	for _, ch := range text {
		if ch == '\f' {
			breaks = append(breaks, offset)
		}
		offset++
		if ch > 0xffff {
			offset++
		}
	}
	if len(breaks) == 0 {
		return r.imageParagraph(start, end, p)
	}
	soft := map[int]bool{}
	for _, position := range r.body.RenderedPageBreaks {
		soft[position] = true
	}
	var flowOrigin *vdocsOrigin
	if text == "\f" {
		props, err := vdocsStyleObject(p.Style)
		if err != nil {
			return nil, err
		}
		flowOrigin, err = readColleafOriginProfile(props["manydocFlowOrigin"], vdocsFlowOriginProfile)
		if err != nil {
			return nil, err
		}
	}
	var blocks []Block
	cursor := start
	join := false
	appendPart := func(part []Block) {
		for _, block := range part {
			if join && len(blocks) > 0 && (block.Kind == Paragraph || block.Kind == Heading) && blocks[len(blocks)-1].Kind == block.Kind && blocks[len(blocks)-1].Level == block.Level && blocks[len(blocks)-1].Direction == block.Direction {
				blocks[len(blocks)-1].Runs = append(blocks[len(blocks)-1].Runs, block.Runs...)
			} else {
				blocks = append(blocks, block)
			}
			join = false
		}
	}
	for _, position := range breaks {
		if cursor < position {
			part, err := r.imageParagraph(cursor, position, p)
			if err != nil {
				return nil, err
			}
			appendPart(part)
		}
		if soft[position] {
			if ids := r.containerPoints[position]; len(ids) > 0 {
				blocks = append(blocks, Block{Kind: AnchorPoint, Anchors: ids})
			}
			r.doc.warn("colleaf_soft_page_break", "Native rendered soft page boundary omitted from logical flow; literal content is retained")
			join = true
		} else {
			blocks = append(blocks, Block{Kind: PageBreak, Anchors: r.containerPoints[position], vdocsFlowOrigin: flowOrigin})
			join = false
		}
		cursor = position + 1
		props, err := vdocsStyleObject(p.Style)
		if err != nil {
			return nil, err
		}
		props["pageBreakBefore"] = json.RawMessage("0")
		p.Style = sfAttrs(props)
	}
	if cursor < end {
		part, err := r.imageParagraph(cursor, end, p)
		if err != nil {
			return nil, err
		}
		appendPart(part)
	}
	after, err := r.cursorBlocks(end, "boundary after page-break paragraph", false)
	if err != nil {
		return nil, err
	}
	blocks = append(blocks, after...)
	return blocks, nil
}

type vdocsNumberUnit struct {
	Value float64 `json:"v"`
	Unit  int     `json:"u"`
}
type vdocsTableWidth struct {
	Type  int             `json:"type"`
	Width vdocsNumberUnit `json:"width"`
}
type vdocsNativeRow struct {
	Cells      []json.RawMessage `json:"tableCells"`
	Repeat     json.RawMessage   `json:"repeatHeaderRow,omitempty"`
	GridBefore int               `json:"gridBefore,omitempty"`
	GridAfter  int               `json:"gridAfter,omitempty"`
}
type vdocsNativeTable struct {
	Literal *vdocsLiteralItem `json:"manydocLiteralItem,omitempty"`
	Origin  json.RawMessage   `json:"manydocOrigin,omitempty"`
	ID      string            `json:"tableId"`
	Rows    []json.RawMessage `json:"tableRows"`
	Columns []struct {
		Size vdocsTableWidth `json:"size"`
	} `json:"tableColumns"`
	Size vdocsTableWidth `json:"size"`
}

type vdocsBodyWriter struct {
	pointScopeSerial       int
	pointScope, pointOwner string
	pointSerial            int
	citationProjection     map[noteKey]string
	ctx                    context.Context
	stream                 strings.Builder
	body                   vdocsBody
	tables                 map[string]json.RawMessage
	offset, serial         int
	noteIDs                map[noteKey]string
	lists                  map[string]json.RawMessage
	listBindings           []vdocsListBinding
	listDepth              int
	listGroup              string
	documentID, storyID    string
	drawings               map[string]json.RawMessage
	drawingOrder           []string
	imageGeometry          []vdocsImageBinding
	imageCache             outputImageCache
}

func (w *vdocsBodyWriter) text(s string) { w.stream.WriteString(s); w.offset += vdocsUTF16Len(s) }
func (w *vdocsBodyWriter) id(prefix string) string {
	w.serial++
	return fmt.Sprintf("%s%d", prefix, w.serial)
}
func (w *vdocsBodyWriter) paragraph(block Block) error {
	sourceBlock := block
	block.Source = nil
	fragment, err := encodeColleafTextBodyWithNotes(w.ctx, []Block{block}, w.noteIDs, w.citationProjection)
	if err != nil {
		return err
	}
	origin := sourceBlock.vdocsOrigin
	if origin == nil {
		origin, err = newColleafOrigin(sourceBlock)
		if err != nil {
			return err
		}
	}
	if origin != nil {
		for i := range fragment.Paragraphs {
			props, err := vdocsStyleObject(fragment.Paragraphs[i].Style)
			if err != nil {
				return err
			}
			props["manydocOrigin"] = sfAttrs(origin)
			fragment.Paragraphs[i].Style = sfAttrs(props)
		}
	}
	for _, binding := range []struct {
		key    string
		origin *vdocsOrigin
	}{{"manydocListOrigin", sourceBlock.vdocsListOrigin}, {"manydocItemOrigin", sourceBlock.vdocsItemOrigin}, {"manydocMarkerOrigin", sourceBlock.vdocsMarkerOrigin}} {
		if binding.origin != nil {
			for i := range fragment.Paragraphs {
				props, err := vdocsStyleObject(fragment.Paragraphs[i].Style)
				if err != nil {
					return err
				}
				props[binding.key] = sfAttrs(binding.origin)
				fragment.Paragraphs[i].Style = sfAttrs(props)
			}
		}
	}
	if sourceBlock.vdocsAutoLabel {
		for i := range fragment.Paragraphs {
			props, err := vdocsStyleObject(fragment.Paragraphs[i].Style)
			if err != nil {
				return err
			}
			props["horizontalAlign"] = sfAttrs(3)
			fragment.Paragraphs[i].Style = sfAttrs(props)
		}
	}
	paragraphIDs := map[string]string{}
	shift := w.offset
	w.text(strings.TrimSuffix(fragment.DataStream, "\n"))
	for _, p := range fragment.Paragraphs {
		p.StartIndex += shift
		oldID := p.ID
		p.ID = w.id("p")
		paragraphIDs[oldID] = p.ID
		w.body.Paragraphs = append(w.body.Paragraphs, p)
	}
	for _, r := range fragment.TextRuns {
		r.St += shift
		r.Ed += shift
		w.body.TextRuns = append(w.body.TextRuns, r)
	}
	for _, r := range fragment.CustomRanges {
		r.StartIndex += shift
		r.EndIndex += shift
		if r.Type != 3 {
			r.ID = w.id("link")
		}
		if err := rebindColleafBookmark(&r, paragraphIDs); err != nil {
			return err
		}
		nameColleafCitationRange(&r)
		w.body.CustomRanges = append(w.body.CustomRanges, r)
	}
	for _, atom := range fragment.CustomBlocks {
		atom.StartIndex += shift
		atom.ID = w.id("note")
		w.body.CustomBlocks = append(w.body.CustomBlocks, atom)
	}
	return nil
}
func (w *vdocsBodyWriter) blocks(blocks []Block, depth int) error {
	if depth > 64 {
		return fmt.Errorf("native body nesting exceeds64")
	}
	allPoints := len(blocks) > 0
	for _, b := range blocks {
		allPoints = allPoints && b.Kind == AnchorPoint
	}
	if len(blocks) == 0 {
		if err := w.paragraph(Block{Kind: Paragraph}); err != nil {
			return err
		}
	}
	scope := w.pointScopeID()
	firstRange := len(w.body.CustomRanges)
	if err := w.pointFlow(blocks, depth, "section", scope); err != nil {
		return err
	}
	if allPoints {
		if err := w.paragraph(Block{Kind: Paragraph}); err != nil {
			return err
		}
		carrier := w.body.Paragraphs[len(w.body.Paragraphs)-1].ID
		for i := firstRange; i < len(w.body.CustomRanges); i++ {
			a := &w.body.CustomRanges[i]
			if a.Type != 3 {
				continue
			}
			p, err := readColleafBookmark(*a)
			if err != nil {
				return err
			}
			if p.Hint != nil && p.Hint.Role == "point" && p.Hint.OwnerID == scope {
				p.Hint.PointCarrier = carrier
				a.Properties = sfAttrs(p)
			}
		}
	}
	sectionID := w.id("s")
	if err := rebindColleafPointScope(&w.body, scope, sectionID, firstRange); err != nil {
		return err
	}
	w.body.SectionBreaks = append(w.body.SectionBreaks, vdocsSectionRecord{StartIndex: w.offset, ID: sectionID})
	w.text("\n")
	return nil
}
func (w *vdocsBodyWriter) flow(blocks []Block, depth int) error {
	if depth > 64 {
		return fmt.Errorf("native body nesting exceeds64")
	}
	for _, b := range blocks {
		if err := w.ctx.Err(); err != nil {
			return err
		}
		if b.Kind == AnchorPoint {
			if err := w.standalonePoint(b); err != nil {
				return err
			}
		} else if b.Kind == PageBreak {
			if err := w.containerAnchors(b.Anchors, w.offset, "break", "page-break"); err != nil {
				return err
			}
			w.text("\f")
			origin, err := newColleafFlowOrigin(b)
			if err != nil {
				return err
			}
			var props json.RawMessage
			if origin != nil {
				props = sfAttrs(map[string]any{"manydocFlowOrigin": origin})
			}
			w.body.Paragraphs = append(w.body.Paragraphs, vdocsParagraphRecord{StartIndex: w.offset, ID: w.id("p"), Style: props})
			w.text("\r")
		} else if b.Kind == Picture {
			if err := w.picture(b); err != nil {
				return err
			}
		} else if b.Kind == ListBlock {
			if err := w.list(b, depth+1); err != nil {
				return err
			}
		} else if b.Kind == Table {
			if err := w.table(b, depth+1); err != nil {
				return err
			}
		} else {
			if err := w.paragraph(b); err != nil {
				return err
			}
		}
	}
	return nil
}

func (w *vdocsBodyWriter) table(block Block, depth int) error {
	if len(block.Runs) > 0 || block.Image != nil || len(block.Items) > 0 || block.Section != nil || block.List != "" || block.Level != 0 || block.Start != nil || block.Numbering != nil {
		return fmt.Errorf("invalid native table block")
	}
	if block.PageBreakBefore || block.Direction != "" {
		return fmt.Errorf("%w: native table annotations require mapping", ErrUnsupported)
	}
	grid, err := tableGrid(block.Rows)
	if err != nil {
		return err
	}
	if grid.Columns == 0 || len(block.Rows) == 0 {
		return fmt.Errorf("native table needs a nonempty grid")
	}
	if err = validateColumnWidths(block.ColumnWidths, grid.Columns); err != nil {
		return err
	}
	id := w.id("table")
	start := w.offset
	if err := w.containerAnchors(block.Anchors, start, "table", id); err != nil {
		return err
	}
	w.text("\x1a")
	rows := make([]any, 0, len(block.Rows))
	for ri, row := range block.Rows {
		w.text("\x1b")
		cells := []any{}
		for col := 0; col < grid.Columns; {

			if col >= len(grid.Slots[ri]) || grid.Slots[ri][col] == nil {
				return fmt.Errorf("%w: ragged native table grid", ErrUnsupported)
			}
			origin := grid.Slots[ri][col]
			if origin.Column != col {
				return fmt.Errorf("invalid native span origin")
			}
			w.text("\x1c")
			rs := 0
			var sourceOrigin *vdocsOrigin
			if origin.Row == ri {
				cell := row.Cells[origin.Cell]
				if cell.Source != nil {
					hash, err := vdocsCellFingerprint(cell)
					if err != nil {
						return err
					}
					sourceOrigin, err = newColleafSourceOrigin(cell.Source, hash, vdocsCellOriginProfile)
					if err != nil {
						return err
					}
				}
				rs = origin.RowSpan
				if err = w.blocks(cell.Blocks, depth); err != nil {
					return err
				}
			} else {
				if err = w.blocks(nil, depth); err != nil {
					return err
				}
			}
			w.text("\x1d")
			cp := map[string]any{"rowSpan": rs, "columnSpan": origin.ColSpan}
			if sourceOrigin != nil {
				cp["manydocOrigin"] = sourceOrigin
			}
			cells = append(cells, cp)
			col += origin.ColSpan
		}
		w.text("\x0e")
		nativeRow := map[string]any{"tableCells": cells, "trHeight": map[string]any{"val": vdocsNumberUnit{0, 3}, "hRule": 0}}
		if row.RepeatHeader {
			nativeRow["repeatHeaderRow"] = 1
		}
		rows = append(rows, nativeRow)
	}
	w.text("\x0f")
	w.body.Tables = append(w.body.Tables, vdocsTableRecord{StartIndex: start, EndIndex: w.offset, ID: id})
	columns := make([]any, grid.Columns)
	total := 0.0
	unit := 3
	for i := range columns {
		width := 100.0 / float64(grid.Columns)
		if len(block.ColumnWidths) > 0 {
			width = block.ColumnWidths[i] * 4 / 3
		} else {
			unit = 4
		}
		columns[i] = map[string]any{"size": vdocsTableWidth{1, vdocsNumberUnit{width, unit}}}
		total += width
	}
	if unit == 4 {
		total = 100
	}
	w.tables[id] = sfAttrs(map[string]any{"tableId": id, "tableRows": rows, "tableColumns": columns, "align": 0, "indent": vdocsNumberUnit{0, 3}, "textWrap": 0, "position": map[string]any{"positionH": map[string]int{"relativeFrom": 0}, "positionV": map[string]int{"relativeFrom": 0}}, "dist": map[string]int{"distT": 0, "distB": 0, "distL": 0, "distR": 0}, "size": vdocsTableWidth{1, vdocsNumberUnit{total, unit}}, "layout": 1})
	if block.Source != nil {
		hash, err := vdocsTableFingerprint(block)
		if err != nil {
			return err
		}
		props, err := vdocsStyleObject(w.tables[id])
		if err != nil {
			return err
		}
		sourceOrigin, err := newColleafSourceOrigin(block.Source, hash, vdocsTableOriginProfile)
		if err != nil {
			return err
		}
		props["manydocOrigin"] = sfAttrs(sourceOrigin)
		w.tables[id] = sfAttrs(props)
	}

	w.body.Paragraphs = append(w.body.Paragraphs, vdocsParagraphRecord{StartIndex: w.offset, ID: w.id("p")})
	w.text("\r")
	return nil
}
func encodeColleafBody(ctx context.Context, blocks []Block) (vdocsBody, map[string]json.RawMessage, error) {
	return encodeColleafBodyWithNotes(ctx, blocks, nil)
}
func encodeColleafBodyWithNotes(ctx context.Context, blocks []Block, noteIDs map[noteKey]string) (vdocsBody, map[string]json.RawMessage, error) {
	body, tables, lists, bindings, err := encodeColleafListBody(ctx, blocks, noteIDs)
	if err != nil {
		return vdocsBody{}, nil, err
	}
	if len(lists) > 0 || len(bindings) > 0 {
		return vdocsBody{}, nil, fmt.Errorf("%w: native list body requires list source/ownership transport", ErrUnsupported)
	}
	return body, tables, nil
}

func encodeColleafListBody(ctx context.Context, blocks []Block, noteIDs map[noteKey]string) (vdocsBody, map[string]json.RawMessage, map[string]json.RawMessage, []vdocsListBinding, error) {
	encoded, err := encodeColleafAssetBody(ctx, blocks, noteIDs, "manydoc", "", nil)
	if err != nil {
		return vdocsBody{}, nil, nil, nil, err
	}
	if len(encoded.Drawings) > 0 {
		return vdocsBody{}, nil, nil, nil, fmt.Errorf("%w: native images require drawing source/geometry transport", ErrUnsupported)
	}
	return encoded.Body, encoded.Tables, encoded.Lists, encoded.ListsOwnership, nil
}

func (r *vdocsBodyReader) paragraphFragment(start, end int, p vdocsParagraphRecord) (vdocsBody, error) {
	text, err := r.index.text.exclusive(start, end)
	if err != nil {
		return vdocsBody{}, err
	}
	out := vdocsBody{DataStream: text + "\r\n"}
	originalEnd := p.StartIndex
	if start != r.paragraphStarts[originalEnd] && len(p.Style) > 0 {
		props, err := vdocsStyleObject(p.Style)
		if err != nil {
			return out, err
		}
		delete(props, "headingId")
		p.Style = sfAttrs(props)
	}
	p.StartIndex = end - start
	out.Paragraphs = []vdocsParagraphRecord{p}
	first := sort.Search(len(r.spans), func(i int) bool { return r.spans[i].end > start })
	for _, span := range r.spans[first:] {
		if span.start >= end {
			break
		}
		if span.record < 0 {
			continue
		}
		run := r.body.TextRuns[span.record]
		run.St = max(start, span.start) - start
		run.Ed = min(end, span.end) - start
		out.TextRuns = append(out.TextRuns, run)
	}
	first = sort.Search(len(r.ranges), func(i int) bool { return r.ranges[i].EndIndex >= start })
	for _, a := range r.ranges[first:] {
		if a.StartIndex >= end {
			break
		}
		a.StartIndex = max(start, a.StartIndex) - start
		a.EndIndex = min(end-1, a.EndIndex) - start
		out.CustomRanges = append(out.CustomRanges, a)
	}
	first = sort.Search(len(r.bookmarks), func(i int) bool { return r.bookmarks[i].StartIndex >= start })
	for _, a := range r.bookmarks[first:] {
		if a.StartIndex > end {
			break
		}
		if r.bookmarkUsed[a.ID] {
			continue
		}
		bp, err := readColleafBookmark(a)
		if err != nil {
			return out, err
		}
		if bp.Hint != nil && bp.Hint.Role == "block" && (a.StartIndex != r.paragraphStarts[originalEnd] || bp.Hint.ParagraphID != p.ID) {
			hint := *bp.Hint
			hint.Role = "run"
			fields, err := vdocsStyleObject(a.Properties)
			if err != nil {
				return out, err
			}
			fields["manydocNavigation"] = sfAttrs(hint)
			a.Properties = sfAttrs(fields)
			r.doc.warn("colleaf_stale_bookmark_placement", "Edited native bookmark position is retained as a current inline point rather than old block placement")
		}
		if a.EndIndex != a.StartIndex {
			r.doc.warn("colleaf_bookmark_span", "Native bookmark range editing extent becomes an SF point at its current start")
		}
		a.StartIndex -= start
		a.EndIndex = a.StartIndex
		out.CustomRanges = append(out.CustomRanges, a)
		r.bookmarkUsed[a.ID] = true
	}
	first = sort.SearchInts(r.atomPositions, start)
	for _, position := range r.atomPositions[first:] {
		if position >= end {
			break
		}
		atom := r.index.blocks[position]
		raw, err := r.index.text.exclusive(position, position+1)
		if err != nil {
			return out, err
		}
		atom.StartIndex -= start
		if raw == "\ufffc" && atom.NoteID != "" {
			kind := 8
			if atom.NoteType == Endnote {
				kind = 9
			}
			out.CustomRanges = append(out.CustomRanges, vdocsRangeRecord{StartIndex: atom.StartIndex, EndIndex: atom.StartIndex, ID: atom.ID, Type: kind, Whole: true, Properties: sfAttrs(map[string]string{"noteId": atom.NoteID})})
		} else {
			out.CustomBlocks = append(out.CustomBlocks, atom)
		}
	}

	return out, nil
}

type vdocsBodyReader struct {
	literalAutomaticCell bool
	cursorWarningSet     map[string]bool
	cursorKinds          map[int]string
	cursorBookmarks      map[int][]vdocsRangeRecord
	pointSlots           map[vdocsPointSlot]bool
	standalone           []vdocsStandalonePoint
	standaloneIDs        map[string]bool
	pointsAt             map[int][]vdocsStandalonePoint
	pointCarriers        map[string]bool
	sectionIDs           map[int]string
	sectionPoints        map[int][]string
	listPointPositions   map[string]int
	listPointOwners      map[string]string
	promotedListPoints   map[string]bool
	containerPoints      map[int][]string
	ctx                  context.Context
	body                 vdocsBody
	index                *vdocsBodyIndex
	settings             vdocsTextContext
	tables               map[string]json.RawMessage
	doc                  *Document
	paragraphs           map[int]vdocsParagraphRecord
	used                 map[string]bool
	spans                []vdocsStyleSpan
	ranges               []vdocsRangeRecord
	bookmarks            []vdocsRangeRecord
	bookmarkUsed         map[string]bool
	paragraphStarts      map[int]int
	atomPositions        []int
	listParagraphs       map[int]vdocsListParagraph
	listBindings         map[string]vdocsListBinding
	images               map[string]*Image
	imageBindings        map[string]vdocsImageBinding
}

func (r *vdocsBodyReader) paragraph(start, end int, p vdocsParagraphRecord) ([]Block, error) {
	return r.breakParagraph(start, end, p)
}
func (r *vdocsBodyReader) textParagraph(start, end int, p vdocsParagraphRecord) ([]Block, error) {
	p.Bullet = nil
	autoLayout := false
	if r.literalAutomaticCell && start == end {
		props, err := vdocsStyleObject(p.Style)
		if err != nil {
			return nil, err
		}
		if string(props["horizontalAlign"]) == "3" {
			delete(props, "horizontalAlign")
			p.Style = sfAttrs(props)
			autoLayout = true
		}
	}
	fragment, err := r.paragraphFragment(start, end, p)
	if err != nil {
		return nil, err
	}
	doc, err := decodeColleafTextBody(r.ctx, fragment, r.settings)
	if err != nil {
		return nil, err
	}
	r.doc.Diagnostics = append(r.doc.Diagnostics, doc.Diagnostics...)
	if autoLayout && len(doc.Blocks) == 1 {
		doc.Blocks[0].vdocsAutoLabel = true
	}
	return doc.Blocks, nil
}
func (r *vdocsBodyReader) sections(sections []vdocsStreamNode, depth int) ([]Block, error) {
	if depth > 64 {
		return nil, fmt.Errorf("native body nesting exceeds64")
	}
	if len(sections) != 1 {
		return nil, fmt.Errorf("%w: native sections require settings mapping", ErrUnsupported)
	}
	var flow []vdocsFlowBlock
	push := func(bs []Block, p vdocsParagraphRecord, start int) {
		for _, b := range bs {
			if b.Kind == AnchorPoint && len(flow) > 0 && flow[len(flow)-1].Position == p.StartIndex {
				flow[len(flow)-1].After = append(flow[len(flow)-1].After, b)
				continue
			}
			if len(flow) > 0 && flow[len(flow)-1].Position == p.StartIndex {
				p.Bullet = nil
			}
			flow = append(flow, vdocsFlowBlock{Block: b, ID: p.ID, Bullet: p.Bullet, Position: p.StartIndex, Start: start})
		}
	}
	for _, node := range sections[0].children {
		if err := r.ctx.Err(); err != nil {
			return nil, err
		}
		if node.kind != "paragraph" {
			return nil, fmt.Errorf("invalid native body child")
		}
		p := r.paragraphs[node.end-1]
		p.StartIndex = node.end - 1
		if len(node.children) == 0 {
			part, err := r.paragraph(node.start, node.end-1, p)
			if err != nil {
				return nil, err
			}
			if !r.emptyPointCarrier(p, node.start, node.end-1, r.sectionIDs[sections[0].end-1], part) {
				push(part, p, node.start)
			}
			continue
		}
		cursor := node.start
		mixed := false
		if len(p.Bullet) > 0 && node.children[0].start == node.start {
			empty, err := r.paragraph(node.start, node.start, p)
			if err != nil {
				return nil, err
			}
			push(empty, p, node.start)
		}
		for _, child := range node.children {
			if cursor < child.start {
				part, err := r.paragraph(cursor, child.start, p)
				if err != nil {
					return nil, err
				}
				push(part, p, cursor)
				mixed = true
			}
			if child.kind != "table" {
				return nil, fmt.Errorf("%w: native container", ErrUnsupported)
			}
			table, err := r.table(child, depth+1)
			if err != nil {
				return nil, err
			}
			push([]Block{table}, p, child.start)
			after, err := r.cursorBlocks(child.end-1, "boundary after table", false)
			if err != nil {
				return nil, err
			}
			flow[len(flow)-1].After = append(flow[len(flow)-1].After, after...)
			if child.end == node.end-1 {
				after, err = r.cursorBlocks(node.end-1, "boundary after table", false)
				if err != nil {
					return nil, err
				}
				flow[len(flow)-1].After = append(flow[len(flow)-1].After, after...)
			}
			cursor = child.end
		}

		if cursor < node.end-1 || len(p.Style) > 0 || p.StyleID != "" || len(p.Bullet) > 0 {
			part, err := r.paragraph(cursor, node.end-1, p)
			if err != nil {
				return nil, err
			}
			push(part, p, cursor)
			mixed = true
		}
		if mixed {
			r.doc.warn("colleaf_table_paragraph", "Native table and text sharing one paragraph become ordered SF blocks; visible content and supported paragraph styles are retained")
		}
	}
	blocks, err := r.groupLists(flow, depth, r.sectionIDs[sections[0].end-1], sections[0].start, sections[0].end-1)
	if err != nil {
		return nil, err
	}
	blocks, err = r.literalLists(blocks, depth)
	if err != nil {
		return nil, err
	}

	scope := r.sectionIDs[sections[0].end-1]
	represented := vdocsStandaloneBlockIDs(blocks)
	for _, p := range r.pointsAt[sections[0].end-1] {
		if !represented[p.id] && (p.scope != "section" || p.owner != scope) {
			blocks = append(blocks, Block{Kind: AnchorPoint, Anchors: []string{p.id}})
			r.doc.warn("colleaf_stale_point_ownership", "Standalone point retains its current native location because its owning flow or boundary changed")
		}
	}
	terminal, err := r.cursorBlocks(sections[0].end-1, "end of current section flow", false)
	if err != nil {
		return nil, err
	}
	blocks = append(blocks, terminal...)
	return blocks, nil
}
func vdocsUnitPoints(unit vdocsNumberUnit) (float64, error) {
	if math.IsNaN(unit.Value) || math.IsInf(unit.Value, 0) || unit.Value < 0 {
		return 0, fmt.Errorf("invalid native table dimension")
	}
	switch unit.Unit {

	case 0, 3:
		return unit.Value * 3 / 4, nil
	default:
		return 0, fmt.Errorf("%w: relative native table dimension unit%d", ErrUnsupported, unit.Unit)
	}
}
func (r *vdocsBodyReader) table(node vdocsStreamNode, depth int) (Block, error) {
	block := Block{Kind: Table, Anchors: r.containerPoints[node.start]}
	record := r.index.tables[node.start]
	raw, ok := r.tables[record.ID]
	if !ok {
		return block, fmt.Errorf("native table %q lacks source", record.ID)
	}
	if r.used[record.ID] {
		return block, fmt.Errorf("duplicate native table source ownership")
	}
	r.used[record.ID] = true
	var table vdocsNativeTable
	if err := json.Unmarshal(raw, &table); err != nil {
		return block, err
	}
	var err error
	if table.Literal != nil {
		var props map[string]json.RawMessage
		if err := json.Unmarshal(raw, &props); err != nil {
			return block, err
		}
		if err := strictSFAttrs(props["manydocLiteralItem"], table.Literal); err != nil {
			return block, err
		}
		if err := table.Literal.validate(); err != nil {
			return block, err
		}
		block.vdocsLiteral = table.Literal
	}
	block.vdocsTableOrigin, err = readColleafOriginProfile(table.Origin, vdocsTableOriginProfile)
	if err != nil {
		return block, err
	}
	if table.ID != record.ID || len(table.Rows) != len(node.children) || len(table.Columns) == 0 {
		return block, fmt.Errorf("native table source/stream identity or grid mismatch")
	}
	columns := len(table.Columns)
	if columns > 10000 || len(table.Rows) > 1_000_000/columns {
		return block, fmt.Errorf("native table grid exceeds supported bounds")
	}
	relativeEqual := table.Size.Type == 1 && table.Size.Width.Unit == 4 && math.Abs(table.Size.Width.Value-100) < 1e-9
	for _, col := range table.Columns {
		relativeEqual = relativeEqual && col.Size.Type == 1 && col.Size.Width.Unit == 4 && math.Abs(col.Size.Width.Value-100/float64(columns)) < 1e-9
	}
	explicit := false
	if !relativeEqual {
		for _, col := range table.Columns {
			if col.Size.Type == 0 && col.Size.Width.Value == 0 {
				block.ColumnWidths = append(block.ColumnWidths, 0)
				continue
			}
			if col.Size.Type != 1 {
				return block, fmt.Errorf("%w: native table column sizing", ErrUnsupported)
			}
			width, err := vdocsUnitPoints(col.Size.Width)
			if err != nil {
				return block, err
			}
			if width <= 0 {
				return block, fmt.Errorf("native table column must be positive")
			}
			explicit = true
			block.ColumnWidths = append(block.ColumnWidths, width)
		}
		if !explicit {
			block.ColumnWidths = nil
			r.doc.warn("colleaf_table_width", "Native unspecified zero widths become SF equal available widths; content and grid are retained")
		} else {
			for _, width := range block.ColumnWidths {
				if width == 0 {
					return block, fmt.Errorf("%w: mixed automatic/fixed native columns", ErrUnsupported)
				}
			}
		}
	}
	if err := validateColumnWidths(block.ColumnWidths, columns); err != nil {
		return block, err
	}

	var cursorProjections []vdocsCellCursorProjection
	occupied := make([][]*tableCellPosition, len(table.Rows))
	for i := range occupied {
		occupied[i] = make([]*tableCellPosition, columns)
	}
	for ri, rowRaw := range table.Rows {
		var row vdocsNativeRow
		if err := json.Unmarshal(rowRaw, &row); err != nil {
			return block, err
		}
		rowNode := node.children[ri]
		if len(row.Cells) != len(rowNode.children) || row.GridBefore != 0 || row.GridAfter != 0 {
			return block, fmt.Errorf("%w: native row token/source counts or omitted grid columns", ErrUnsupported)
		}
		outRow := Row{}
		if len(row.Repeat) > 0 {
			repeat, err := vdocsBool(row.Repeat)
			if err != nil {
				return block, err
			}
			outRow.RepeatHeader = repeat
		}
		col := 0
		previousSpan := 0
		var previousOrigin *tableCellPosition
		for ci, cellRaw := range row.Cells {
			properties, err := vdocsStyleObject(cellRaw)
			if err != nil {
				return block, err
			}
			rs, hasRS, err := vdocsRawInt(properties, "rowSpan")
			if err != nil {
				return block, err
			}
			cs, hasCS, err := vdocsRawInt(properties, "columnSpan")
			if err != nil {
				return block, err
			}
			if !hasRS {
				rs = 1
			}
			if !hasCS {
				cs = 1
			}
			r.presentationLosses(cellRaw, []string{"rowSpan", "columnSpan", "manydocOrigin"}, "colleaf_cell_presentation")
			if rs < 0 || cs < 0 || rs > len(table.Rows)-ri || cs > columns {
				return block, fmt.Errorf("invalid native cell span")
			}
			cellNode := rowNode.children[ci]
			if rs == 0 {
				content, err := r.index.text.exclusive(cellNode.start+1, cellNode.end-1)
				if err != nil {
					return block, err
				}
				if content != "\r\n" {
					return block, fmt.Errorf("native covered cell contains content; refusing to discard it")
				}
				if cs == 0 {
					if previousSpan <= 1 {
						return block, fmt.Errorf("unowned native horizontal continuation")
					}
					projection, err := r.cellCursorProjection(rowNode, cellNode, ci, len(row.Cells), previousOrigin, true)
					if err != nil {
						return block, err
					}
					if len(projection.before)+len(projection.after) > 0 {
						cursorProjections = append(cursorProjections, projection)
					}
					previousSpan--
					continue
				}
				if col+cs > columns || occupied[ri][col] == nil || occupied[ri][col].Row >= ri || occupied[ri][col].Column != col || occupied[ri][col].ColSpan != cs {
					return block, fmt.Errorf("unowned native vertical continuation")
				}
				for cc := col; cc < col+cs; cc++ {
					if occupied[ri][cc] != occupied[ri][col] {
						return block, fmt.Errorf("contradictory native continuation")
					}
				}
				previousOrigin = occupied[ri][col]
				projection, err := r.cellCursorProjection(rowNode, cellNode, ci, len(row.Cells), previousOrigin, true)
				if err != nil {
					return block, err
				}
				if len(projection.before)+len(projection.after) > 0 {
					cursorProjections = append(cursorProjections, projection)
				}
				col += cs
				previousSpan = cs
				continue
			}
			if cs == 0 || col+cs > columns {
				return block, fmt.Errorf("invalid native cell column span")
			}
			origin := &tableCellPosition{Row: ri, Cell: len(outRow.Cells), Column: col, ColSpan: cs, RowSpan: rs}
			for rr := ri; rr < ri+rs; rr++ {
				for cc := col; cc < col+cs; cc++ {
					if occupied[rr][cc] != nil {
						return block, fmt.Errorf("overlapping native cell span")
					}
					occupied[rr][cc] = origin
				}
			}
			previousAutoCell := r.literalAutomaticCell
			r.literalAutomaticCell = table.Literal != nil && table.Literal.Automatic && ri == 0 && ((ci == 0 && !table.Literal.Right) || (ci == 1 && table.Literal.Right))
			content, err := r.sections(cellNode.children, depth)
			r.literalAutomaticCell = previousAutoCell
			if err != nil {
				return block, err
			}
			projection, err := r.cellCursorProjection(rowNode, cellNode, ci, len(row.Cells), origin, false)
			if err != nil {
				return block, err
			}
			if len(projection.before)+len(projection.after) > 0 {
				cursorProjections = append(cursorProjections, projection)
			}
			previousOrigin = origin
			cellOrigin, err := readColleafOriginProfile(properties["manydocOrigin"], vdocsCellOriginProfile)
			if err != nil {
				return block, err
			}
			sfCell := Cell{Blocks: content, vdocsOrigin: cellOrigin}
			if rs > 1 {
				sfCell.RowSpan = rs
			}
			if cs > 1 {
				sfCell.ColSpan = cs
			}
			outRow.Cells = append(outRow.Cells, sfCell)
			col += cs
			previousSpan = cs
		}
		if col != columns {
			return block, fmt.Errorf("native table row does not cover its declared grid")
		}
		r.presentationLosses(rowRaw, []string{"tableCells", "repeatHeaderRow", "gridBefore", "gridAfter", "trHeight"}, "colleaf_row_presentation")
		var props map[string]json.RawMessage
		_ = json.Unmarshal(rowRaw, &props)
		if height := props["trHeight"]; len(height) > 0 {
			var h struct {
				Val  vdocsNumberUnit `json:"val"`
				Rule int             `json:"hRule"`
			}
			if err := json.Unmarshal(height, &h); err != nil {
				return block, err
			}
			if h.Val.Value != 0 || h.Rule != 0 {
				r.doc.warn("colleaf_row_height", "Native authored row height is not represented; row content and merge ownership are retained")
			}
		}
		block.Rows = append(block.Rows, outRow)
	}

	beforeByOrigin := map[*tableCellPosition][]Block{}
	afterByOrigin := map[*tableCellPosition][]Block{}
	var origins []*tableCellPosition
	seenOrigin := map[*tableCellPosition]bool{}
	for _, p := range cursorProjections {
		if !seenOrigin[p.origin] {
			seenOrigin[p.origin] = true
			origins = append(origins, p.origin)
		}
		beforeByOrigin[p.origin] = append(beforeByOrigin[p.origin], p.before...)
		afterByOrigin[p.origin] = append(afterByOrigin[p.origin], p.after...)
	}
	for _, origin := range origins {
		cell := &block.Rows[origin.Row].Cells[origin.Cell]
		cell.Blocks = append(append(beforeByOrigin[origin], cell.Blocks...), afterByOrigin[origin]...)
	}
	if _, err := tableGrid(block.Rows); err != nil {
		return block, err
	}

	var props map[string]json.RawMessage
	if err := json.Unmarshal(raw, &props); err != nil {
		return block, err
	}
	total := 0.0
	for _, col := range table.Columns {
		total += col.Size.Width.Value
	}
	typeValue, layout, unit := 0, 0, 3
	if explicit || relativeEqual {
		typeValue, layout = 1, 1
	}
	if relativeEqual {
		unit = 4
		total = 100
	} else if explicit && table.Size.Width.Unit == 0 {
		unit = 0
	}
	neutral := map[string]any{
		"align": 0, "indent": vdocsNumberUnit{0, 3}, "textWrap": 0,
		"position": map[string]any{"positionH": map[string]int{"relativeFrom": 0}, "positionV": map[string]int{"relativeFrom": 0}},
		"dist":     map[string]int{"distT": 0, "distB": 0, "distL": 0, "distR": 0},
		"size":     vdocsTableWidth{typeValue, vdocsNumberUnit{total, unit}}, "layout": layout,
	}
	allowed := []string{"tableId", "tableRows", "tableColumns", "manydocOrigin", "manydocLiteralItem"}
	for key, expected := range neutral {
		var actual, canonical any
		_ = json.Unmarshal(props[key], &actual)
		_ = json.Unmarshal(sfAttrs(expected), &canonical)
		if reflect.DeepEqual(actual, canonical) {
			allowed = append(allowed, key)
		}
	}
	r.presentationLosses(raw, allowed, "colleaf_table_presentation")
	return block, nil
}
func (r *vdocsBodyReader) presentationLosses(raw json.RawMessage, allowed []string, code string) {
	var props map[string]json.RawMessage
	if json.Unmarshal(raw, &props) != nil {
		return
	}
	keys := []string{}
	for key := range props {
		found := false
		for _, a := range allowed {
			if a == key {
				found = true
				break
			}
		}
		if !found {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		r.doc.warn(code, "Native property "+key+" is not represented in SF; table content and supported geometry are retained")
	}
}
func newColleafBodyReader(ctx context.Context, body vdocsBody, settings vdocsTextContext, tables map[string]json.RawMessage) (*vdocsBodyReader, error) {
	index, err := indexColleafBody(ctx, body)
	if err != nil {
		return nil, err
	}
	for _, a := range body.CustomRanges {
		if a.Type != 0 && a.Type != 3 && a.Type != 8 && a.Type != 9 {
			return nil, fmt.Errorf("%w: native annotation type %d requires mapping", ErrUnsupported, a.Type)
		}
	}
	for _, atom := range index.blocks {
		if atom.Type != nil && *atom.Type == 0 {
			if _, ok := settings.Drawings[atom.ID]; !ok {
				return nil, fmt.Errorf("native drawing atom lacks source")
			}
			if atom.NoteID != "" || atom.NoteType != "" {
				return nil, fmt.Errorf("contradictory native image/note atom")
			}
			continue
		}
		key, ok := settings.NoteIDs[atom.NoteID]
		if !ok || (atom.Type != nil && *atom.Type != 1) || (atom.NoteType != "" && key.kind != atom.NoteType) {
			return nil, fmt.Errorf("%w: unmapped/contradictory native atom", ErrUnsupported)
		}
	}
	r := vdocsBodyReader{ctx: ctx, body: body, index: index, settings: settings, tables: tables, doc: &Document{}, paragraphs: map[int]vdocsParagraphRecord{}, used: map[string]bool{}}
	r.images = map[string]*Image{}
	r.imageBindings = map[string]vdocsImageBinding{}
	for _, binding := range settings.ImageBindings {
		if binding.ID == "" {
			return nil, fmt.Errorf("invalid image geometry binding")
		}
		if _, ok := r.imageBindings[binding.ID]; ok {
			return nil, fmt.Errorf("duplicate image geometry binding")
		}
		r.imageBindings[binding.ID] = binding
	}
	order := map[string]bool{}
	for _, id := range settings.DrawingOrder {
		if order[id] {
			return nil, fmt.Errorf("duplicate native drawing order ID")
		}
		if _, ok := settings.Drawings[id]; !ok {
			return nil, fmt.Errorf("native drawing order lacks source")
		}
		order[id] = true
	}
	for position := range index.blocks {
		r.atomPositions = append(r.atomPositions, position)
	}
	sort.Ints(r.atomPositions)
	r.spans, err = vdocsStyleSpans(ctx, index.text, body.TextRuns)
	if err != nil {
		return nil, err
	}
	if err := r.indexStructuralCursors(); err != nil {
		return nil, err
	}
	r.bookmarkUsed = map[string]bool{}
	r.containerPoints = map[int][]string{}
	r.sectionPoints = map[int][]string{}
	r.standaloneIDs = map[string]bool{}
	r.pointSlots = map[vdocsPointSlot]bool{}
	r.pointsAt = map[int][]vdocsStandalonePoint{}
	r.pointCarriers = map[string]bool{}
	r.sectionIDs = map[int]string{}
	sectionEndIDs := map[int]string{}
	for _, s := range body.SectionBreaks {
		sectionEndIDs[s.StartIndex] = s.ID
		r.sectionIDs[s.StartIndex] = s.ID
	}
	sectionStartIDs := map[int]string{}
	for _, s := range index.sections {
		sectionStartIDs[s.start] = sectionEndIDs[s.end-1]
	}
	r.listPointOwners = map[string]string{}
	r.listPointPositions = map[string]int{}
	r.promotedListPoints = map[string]bool{}
	r.paragraphStarts = vdocsParagraphStarts(index)
	for _, a := range body.CustomRanges {
		if a.Type == 3 {
			p, err := readColleafBookmark(a)
			if err != nil {
				return nil, err
			}
			if p.Hint != nil && p.Hint.Role == "point" {
				if err := r.registerStandalone(a, p); err != nil {
					return nil, err
				}
			}
			if p.Hint != nil && p.Hint.Role == "section" && p.Hint.OwnerID == sectionStartIDs[a.StartIndex] {
				if err = vdocsBookmarkLosses(r.doc, a); err != nil {
					return nil, err
				}
				r.sectionPoints[a.StartIndex] = append(r.sectionPoints[a.StartIndex], p.BookmarkID)
				continue
			}
			claimed, err := r.containerBookmark(a)
			if err != nil {
				return nil, err
			}
			if claimed {
				continue
			}
			if _, ok := r.cursorKinds[a.StartIndex]; ok && !(p.Hint != nil && p.Hint.Role == "point" && r.cursorKinds[a.StartIndex] == "section terminator") {
				r.cursorBookmarks[a.StartIndex] = append(r.cursorBookmarks[a.StartIndex], a)
				continue
			}
			if p.Hint != nil && p.Hint.Role == "list" {
				r.listPointOwners[p.BookmarkID] = p.Hint.OwnerID
				r.listPointPositions[p.BookmarkID] = a.StartIndex
			}
			r.bookmarks = append(r.bookmarks, a)
			continue
		}
		if a.Type != 8 && a.Type != 9 {
			r.ranges = append(r.ranges, a)
		}
	}
	for _, p := range r.standalone {
		r.pointsAt[p.position] = append(r.pointsAt[p.position], p)
	}
	sort.SliceStable(r.bookmarks, func(i, j int) bool { return r.bookmarks[i].StartIndex < r.bookmarks[j].StartIndex })
	sort.Slice(r.ranges, func(i, j int) bool { return r.ranges[i].StartIndex < r.ranges[j].StartIndex })
	for i := 1; i < len(r.ranges); i++ {
		if r.ranges[i-1].EndIndex >= r.ranges[i].StartIndex {
			return nil, fmt.Errorf("conflicting native hyperlink extents")
		}
	}
	r.listBindings = map[string]vdocsListBinding{}
	for _, binding := range settings.ListBindings {
		names, err := vdocsListBindingTypes(binding)
		if err != nil {
			return nil, err
		}
		for _, name := range names {
			if _, ok := r.listBindings[name]; ok {
				return nil, fmt.Errorf("duplicate native list binding alias")
			}
			r.listBindings[name] = binding
		}
	}

	r.listParagraphs, err = vdocsListParagraphs(ctx, body, settings.Lists, r.listBindings)
	if err != nil {
		return nil, err
	}
	policyLoss, staleProjection := false, false
	for _, info := range r.listParagraphs {
		policyLoss = policyLoss || info.PolicyLoss
		staleProjection = staleProjection || info.StaleProjection
	}
	if policyLoss {
		r.doc.warn("colleaf_numbering_policy", "Native zero/legal marker presentation is retained with explicit SF templates/formats; live native numbering policy is not retained")
	}
	if staleProjection {
		r.doc.warn("colleaf_stale_numbering_projection", "Changed native numbering projection retained; old source format intent was not applied")
	}

	for _, p := range body.Paragraphs {
		r.paragraphs[p.StartIndex] = p
	}
	return &r, nil
}

func decodeColleafBody(ctx context.Context, body vdocsBody, settings vdocsTextContext, tables map[string]json.RawMessage) (*Document, error) {
	r, err := newColleafBodyReader(ctx, body, settings, tables)
	if err != nil {
		return nil, err
	}
	blocks, err := r.sections(r.index.sections, 0)
	if err != nil {
		return nil, err
	}
	r.doc.Blocks = blocks
	r.finishStandalone(blocks)
	r.finishListPoints()
	if len(r.used) != len(tables) {
		return nil, fmt.Errorf("native table source is orphaned")
	}
	if len(r.images) != len(settings.Drawings) {
		return nil, fmt.Errorf("native drawing source is unbound/orphaned")
	}
	return r.doc, nil
}
