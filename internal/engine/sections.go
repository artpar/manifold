package engine

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"math"
)

// Match the reader's allocation bound before a decoder expands untrusted data.
const maxOutputImagePixels = 25_000_000

type outputImageInfo struct {
	config image.Config
	kind   string
}
type outputImageCache map[[32]byte]outputImageInfo

func decodeOutputImage(ctx context.Context, data []byte, cache outputImageCache) (image.Config, string, error) {
	if err := ctx.Err(); err != nil {
		return image.Config{}, "", err
	}
	digest := sha256.Sum256(data)
	if known, ok := cache[digest]; ok {
		return known.config, known.kind, nil
	}
	cfg, kind, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return cfg, kind, fmt.Errorf("image configuration: %w", err)
	}
	if kind != "png" && kind != "jpeg" {
		return cfg, kind, fmt.Errorf("%w: output image %s", ErrUnsupported, kind)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width) > maxOutputImagePixels/int64(cfg.Height) {
		return cfg, kind, fmt.Errorf("%w: output image exceeds raster allocation bound", ErrUnsupported)
	}
	decoded, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return cfg, kind, fmt.Errorf("image pixels: %w", err)
	}
	if decoded.Bounds().Dx() != cfg.Width || decoded.Bounds().Dy() != cfg.Height {
		return cfg, kind, fmt.Errorf("image configuration contradicts pixel dimensions")
	}
	if err := ctx.Err(); err != nil {
		return cfg, kind, err
	}
	if cache != nil {
		cache[digest] = outputImageInfo{cfg, kind}
	}
	return cfg, kind, nil
}

// DOCX is not laid out here. Reserve a default paragraph line per enclosing
// cell, plus a conservative reference/separator/marker area in note stories.
const docxImageCellHeightAllowance = 14.0
const docxImageNoteHeightAllowance = 56.0

// Dimensions are preferred physical sizes. A single dimension preserves the
// intrinsic aspect ratio; both dimensions explicitly specify that ratio. Fit
// to the container before inferring a missing dimension to avoid overflow.
func fitImageSize(im *Image, pixelWidth, pixelHeight int, available float64) (float64, float64, error) {
	if im == nil || pixelWidth <= 0 || pixelHeight <= 0 || !finite(available) || available <= 0 {
		return 0, 0, fmt.Errorf("%w: no space for image", ErrUnsupported)
	}
	w, h := im.Width, im.Height
	ratio := float64(pixelHeight) / float64(pixelWidth)
	if w == 0 && h == 0 {
		w = float64(pixelWidth) * .75
	}
	if w == 0 {
		if h > available*ratio {
			w, h = available, available*ratio
		} else {
			w = h / ratio
		}
	} else if h == 0 {
		w = min(w, available)
		h = w * ratio
	} else if w > available {
		h *= available / w
		w = available
	}
	if !finite(w) || !finite(h) || w <= 0 || h <= 0 {
		return 0, 0, fmt.Errorf("%w: image dimensions cannot be represented", ErrUnsupported)
	}
	return w, h, nil
}

// Fit preferred sizes to a full container without enlarging or changing the
// preferred rectangle's aspect. Both explicit dimensions may intentionally differ
// from the intrinsic aspect. Writers check coordinate serialization separately.

// PageMargins overrides all four edges of Layout.Margin. Zero is an explicit
// zero margin. Top/Bottom may be negative for OOXML fixed body offsets that
// permit running-content overlap; Left/Right are nonnegative.

// New edge/distance bindings budget running images against the page around their
// anchors, independently from the already reserved body margins. Legacy inputs
// keep their previous full-body image preference container.

// Fixed running stories may overlap the body and the opposite story. Their
// containers end at their own page anchor, without paying the opposite distance.

// SectionStart controls the section start relative to the preceding section.
// An empty value means nextPage; the first section starts the document.

// RunningStories distinguishes an inherited nil story from an explicit empty one.
// Default serves odd pages (and all ordinary pages without EvenAndOddHeaders).

// RunningStory is an independently inherited header or footer variant.
// A non-nil story with no blocks explicitly suppresses that variant.

// Section owns body content. Layout nil inherits the previous effective layout
// (the document layout for the first section). Stories inherit independently.
// Native PDF keeps the stories of the section that opened a physical page.
// A continuous section counts a shared page as local page one, even when its
// body is empty. Later pages select its default/even stories using that count.

func fitImageBox(im *Image, pixelWidth, pixelHeight int, width, height float64) (float64, float64, error) {
	if !finite(height) || height <= 0 {
		return 0, 0, fmt.Errorf("%w: no height for image", ErrUnsupported)
	}
	w, h, err := fitImageSize(im, pixelWidth, pixelHeight, width)
	if err != nil {
		return 0, 0, err
	}
	if h > height {
		w *= height / h
		h = height
	}
	if !finite(w) || !finite(h) || w <= 0 || h <= 0 {
		return 0, 0, fmt.Errorf("%w: fitted image dimensions cannot be represented", ErrUnsupported)
	}
	return w, h, nil
}

type PageMargins struct {
	Top    float64 `json:"top"`
	Right  float64 `json:"right"`
	Bottom float64 `json:"bottom"`
	Left   float64 `json:"left"`
}

type pageGeometry struct {
	width, height   float64
	margins         PageMargins
	header, footer  *float64
	explicitMargins bool
}

func cloneLayout(l Layout) Layout {
	if l.Margins != nil {
		v := *l.Margins
		l.Margins = &v
	}
	if l.HeaderDistance != nil {
		v := *l.HeaderDistance
		l.HeaderDistance = &v
	}
	if l.FooterDistance != nil {
		v := *l.FooterDistance
		l.FooterDistance = &v
	}
	return l
}
func pageLayout(l Layout) pageGeometry {
	w, h, m := layout(l)
	g := pageGeometry{width: w, height: h, margins: PageMargins{m, m, m, m}, header: l.HeaderDistance, footer: l.FooterDistance, explicitMargins: l.Margins != nil}
	if l.Margins != nil {
		g.margins = *l.Margins
	}
	return g
}
func (g pageGeometry) contentWidth() float64 { return g.width - g.margins.Left - g.margins.Right }
func (g pageGeometry) contentHeight() float64 {
	return g.height - math.Abs(g.margins.Top) - math.Abs(g.margins.Bottom)
}
func (g pageGeometry) headerDistance() float64 {
	if g.header != nil {
		return *g.header
	}
	return docxRunningDistance
}
func (g pageGeometry) footerDistance() float64 {
	if g.footer != nil {
		return *g.footer
	}
	return docxRunningDistance
}
func validateLayout(l Layout) error {
	for _, n := range []float64{l.Width, l.Height, l.Margin} {
		if !finite(n) || n < 0 {
			return fmt.Errorf("layout dimensions must be finite and nonnegative")
		}
	}
	g := pageLayout(l)
	m := g.margins
	for _, n := range []float64{m.Top, m.Right, m.Bottom, m.Left} {
		if !finite(n) {
			return fmt.Errorf("page margins must be finite")
		}
	}
	if m.Left < 0 || m.Right < 0 {
		return fmt.Errorf("left/right page margins must be nonnegative")
	}
	if g.contentWidth() <= 0 || g.contentHeight() <= 0 {
		return fmt.Errorf("page margins leave no content area")
	}
	for _, p := range []*float64{l.HeaderDistance, l.FooterDistance} {
		if p != nil && (!finite(*p) || *p < 0) {
			return fmt.Errorf("running distance must be finite and nonnegative")
		}
	}
	return nil
}

func (g pageGeometry) runningHeight(header, footer bool) float64 {
	if !g.explicitMargins && g.header == nil && g.footer == nil {
		return g.contentHeight()
	}
	h := g.height
	if header {
		h -= g.headerDistance()
	}
	if footer {
		h -= g.footerDistance()
	}
	return h
}

func (g pageGeometry) runningStoryHeight(header, hasHeader, hasFooter bool) float64 {
	if header && g.margins.Top < 0 {
		return g.height - g.headerDistance()
	}
	if !header && g.margins.Bottom < 0 {
		return g.height - g.footerDistance()
	}
	return g.runningHeight(hasHeader, hasFooter)
}

type SectionStart string

const (
	SectionNextPage   SectionStart = "nextPage"
	SectionContinuous SectionStart = "continuous"
	SectionEvenPage   SectionStart = "evenPage"
	SectionOddPage    SectionStart = "oddPage"
)

type RunningStories struct {
	Default *RunningStory `json:"default,omitempty"`
	First   *RunningStory `json:"first,omitempty"`
	Even    *RunningStory `json:"even,omitempty"`
}

type RunningStory struct {
	Blocks []Block `json:"blocks"`
}

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
