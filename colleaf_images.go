package manydoc

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"sort"
	"strings"
)

const colleafImageResource = "manydoc.image-geometry.v1"

type colleafImageSize struct {
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}
type colleafDrawingTransform struct {
	Size      colleafImageSize `json:"size"`
	PositionH json.RawMessage  `json:"positionH"`
	PositionV json.RawMessage  `json:"positionV"`
	Angle     float64          `json:"angle"`
	FlipX     bool             `json:"flipX,omitempty"`
	FlipY     bool             `json:"flipY,omitempty"`
}
type colleafNativeImage struct {
	ID          string                   `json:"drawingId"`
	Type        *int                     `json:"drawingType"`
	Layout      *int                     `json:"layoutType"`
	UnitID      string                   `json:"unitId"`
	SubUnitID   string                   `json:"subUnitId"`
	SourceType  string                   `json:"imageSourceType"`
	Source      string                   `json:"source"`
	Cache       string                   `json:"base64Cache,omitempty"`
	Description string                   `json:"description,omitempty"`
	Title       string                   `json:"title,omitempty"`
	Transform   *colleafDrawingTransform `json:"docTransform"`
	Hidden      bool                     `json:"hidden,omitempty"`
	Opacity     *float64                 `json:"opacity,omitempty"`
	Crop        json.RawMessage          `json:"srcRect,omitempty"`
	Geometry    *string                  `json:"prstGeom,omitempty"`
}
type colleafImageBinding struct {
	Container    *colleafImageSize `json:"container,omitempty"`
	ID           string            `json:"drawingId"`
	Hash         string            `json:"sha256"`
	Width        float64           `json:"width"`
	Height       float64           `json:"height"`
	NativeWidth  float64           `json:"nativeWidth"`
	NativeHeight float64           `json:"nativeHeight"`
}
type colleafImageGeometry struct {
	Stories map[string][]colleafImageBinding `json:"stories"`
}
type colleafEncodedBody struct {
	Body                    colleafBody
	Tables, Lists, Drawings map[string]json.RawMessage
	DrawingOrder            []string
	ListsOwnership          []colleafListBinding
	ImageGeometry           []colleafImageBinding
}

func colleafIntrinsicImageSize(im *Image, cfg image.Config) (colleafImageSize, error) {
	w, h := im.Width, im.Height
	if !finite(w) || !finite(h) || w < 0 || h < 0 {
		return colleafImageSize{}, fmt.Errorf("invalid image geometry")
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
		return colleafImageSize{}, fmt.Errorf("image geometry outside supported coordinates")
	}
	return colleafImageSize{w * 4 / 3, h * 4 / 3}, nil
}
func colleafImageHash(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}
func (w *colleafBodyWriter) picture(block Block) error {
	if block.Image == nil || block.Source != nil || len(block.Anchors) > 0 || block.Direction != "" || block.PageBreakBefore || len(block.Runs) > 0 || len(block.Rows) > 0 || len(block.Items) > 0 || block.Section != nil || block.List != "" || block.Numbering != nil || block.Start != nil || block.Level != 0 {
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
	size, err := colleafIntrinsicImageSize(block.Image, cfg)
	if err != nil {
		return err
	}
	story := base64.RawURLEncoding.EncodeToString([]byte(w.storyID))
	if story == "" {
		story = "main"
	}
	id := "manydoc-image-" + story + "-" + w.id("i")
	zero := 0
	drawing := colleafNativeImage{ID: id, Type: &zero, Layout: &zero, UnitID: w.documentID, SubUnitID: w.documentID, SourceType: "BASE64", Source: "data:" + block.Image.MediaType + ";base64," + base64.StdEncoding.EncodeToString(block.Image.Data), Description: block.Image.Alt, Transform: &colleafDrawingTransform{Size: size, PositionH: sfAttrs(map[string]int{"relativeFrom": 2, "posOffset": 0}), PositionV: sfAttrs(map[string]int{"relativeFrom": 2, "posOffset": 0})}}
	w.drawings[id] = sfAttrs(drawing)
	w.drawingOrder = append(w.drawingOrder, id)
	w.imageGeometry = append(w.imageGeometry, colleafImageBinding{ID: id, Hash: colleafImageHash(block.Image.Data), Width: block.Image.Width, Height: block.Image.Height, NativeWidth: size.Width, NativeHeight: size.Height})
	w.body.CustomBlocks = append(w.body.CustomBlocks, colleafBlockRecord{StartIndex: w.offset, Type: &zero, ID: id})
	w.text("\b")
	w.body.Paragraphs = append(w.body.Paragraphs, colleafParagraphRecord{StartIndex: w.offset, ID: w.id("p")})
	w.text("\r")
	return nil
}
func encodeColleafAssetBody(ctx context.Context, blocks []Block, noteIDs map[noteKey]string, documentID, storyID string, cache outputImageCache, projection ...map[noteKey]string) (colleafEncodedBody, error) {
	if cache == nil {
		cache = outputImageCache{}
	}
	w := colleafBodyWriter{ctx: ctx, noteIDs: noteIDs, documentID: documentID, storyID: storyID, imageCache: cache, tables: map[string]json.RawMessage{}, lists: map[string]json.RawMessage{}, drawings: map[string]json.RawMessage{}, body: colleafBody{TextRuns: []colleafTextRun{}}}
	if len(projection) > 0 {
		w.citationProjection = projection[0]
	}
	if err := w.blocks(blocks, 0); err != nil {
		return colleafEncodedBody{}, err
	}
	w.body.DataStream = w.stream.String()
	if _, err := indexColleafBody(ctx, w.body); err != nil {
		return colleafEncodedBody{}, err
	}
	return colleafEncodedBody{Body: w.body, Tables: w.tables, Lists: w.lists, Drawings: w.drawings, DrawingOrder: w.drawingOrder, ListsOwnership: w.listBindings, ImageGeometry: w.imageGeometry}, nil
}
func colleafEmbeddedImage(source string) ([]byte, string, error) {
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
func (r *colleafBodyReader) nativeImage(id string) (*Image, error) {
	if cached := r.images[id]; cached != nil {
		copy := *cached
		return &copy, nil
	}
	raw, ok := r.settings.Drawings[id]
	if !ok {
		return nil, fmt.Errorf("native image atom lacks drawing source %q", id)
	}
	var drawing colleafNativeImage
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
	data, kind, err := colleafEmbeddedImage(drawing.Source)
	if err != nil {
		return nil, err
	}
	if drawing.Cache != "" {
		cached, cacheKind, err := colleafEmbeddedImage(drawing.Cache)
		if err != nil || cacheKind != kind || colleafImageHash(cached) != colleafImageHash(data) {
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
		projected, err := colleafIntrinsicImageSize(&candidate, cfg)
		if err == nil && binding.Container != nil {
			width, height, fitErr := fitImageBox(&candidate, cfg.Width, cfg.Height, binding.Container.Width, binding.Container.Height)
			err = fitErr
			projected = colleafImageSize{Width: width * 4 / 3, Height: height * 4 / 3}
		}
		if err == nil && binding.Hash == colleafImageHash(data) && binding.NativeWidth == size.Width && binding.NativeHeight == size.Height && projected == size {
			im.Width = binding.Width
			im.Height = binding.Height
		} else {
			r.doc.warn("colleaf_stale_image_geometry", "Stale image size intent ignored; actual native asset and resized geometry are retained")
		}
	}
	r.presentationLosses(raw, []string{"drawingId", "drawingType", "layoutType", "unitId", "subUnitId", "imageSourceType", "source", "base64Cache", "description", "docTransform", "opacity", "hidden", "srcRect", "prstGeom", "title"}, "colleaf_image_presentation")
	if drawing.Title != "" {
		r.doc.warn("colleaf_image_title", "Native image title is not represented separately; description/alternative text and image bytes are retained")
	}
	r.images[id] = im
	copy := *im
	return &copy, nil
}
func (r *colleafBodyReader) imageAnchorVisible(position int, p colleafParagraphRecord) error {
	resolver := colleafStyleResolver{named: r.settings.Styles}
	named, err := resolver.resolve(p.StyleID, 0)
	if err != nil {
		return err
	}
	base, err := colleafStyleObject(r.settings.DefaultText)
	if err != nil {
		return err
	}
	namedText, err := colleafStyleObject(named.TextStyle)
	if err != nil {
		return err
	}
	props, err := colleafStyleObject(r.settings.DefaultParagraph)
	if err != nil {
		return err
	}
	inherited, err := colleafStyleObject(named.ParagraphStyle)
	if err != nil {
		return err
	}
	direct, err := colleafStyleObject(p.Style)
	if err != nil {
		return err
	}
	props = mergeColleafStyle(mergeColleafStyle(props, inherited), direct)
	paragraphText, err := colleafStyleObject(props["textStyle"])
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
		inherited, err := colleafStyleObject(character.TextStyle)
		if err != nil {
			return err
		}
		direct, err := colleafStyleObject(run.TS)
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

func (r *colleafBodyReader) imageParagraph(start, end int, p colleafParagraphRecord) ([]Block, error) {
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
		blocks = append(blocks, Block{Kind: Picture, Image: image})
		cursor = pos + 1
		properties, err := colleafStyleObject(p.Style)
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
	if authoredPresentation {
		r.doc.warn("colleaf_image_paragraph", "Native picture paragraph presentation is normalized to SF image block flow; image and supported surrounding text remain")
	}
	return blocks, nil
}

// Keep the geometry arithmetic explicit: native drawing boxes use pixels while
// SF dimensions use points. No application image/file quotas are introduced.
