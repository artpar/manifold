package engine

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"path"
	"strings"
)

// Presentation is the positioned-slide extension of the shared document model.
// SourceParts preserve native package features; authored content lives in Slides
// and Resources and takes precedence when a supported object is edited.
type Presentation struct {
	Version     int                    `json:"version"`
	Width       float64                `json:"width"`
	Height      float64                `json:"height"`
	Slides      []PresentationSlide    `json:"slides"`
	Resources   []PresentationResource `json:"resources,omitempty"`
	SourceParts map[string][]byte      `json:"source_parts,omitempty"`
}
type PresentationSlide struct {
	Hidden      bool          `json:"hidden,omitempty"`
	ID          string        `json:"id"`
	Part        string        `json:"part,omitempty"`
	LayoutPart  string        `json:"layout_part,omitempty"`
	MasterPart  string        `json:"master_part,omitempty"`
	Width       float64       `json:"width,omitempty"`
	Height      float64       `json:"height,omitempty"`
	Background  string        `json:"background,omitempty"`
	Objects     []SlideObject `json:"objects,omitempty"`
	Notes       []Block       `json:"notes,omitempty"`
	Unsupported []string      `json:"unsupported,omitempty"`
}
type PresentationResource struct {
	Part        string        `json:"part"`
	Kind        string        `json:"kind"`
	MasterPart  string        `json:"master_part,omitempty"`
	Background  string        `json:"background,omitempty"`
	Objects     []SlideObject `json:"objects,omitempty"`
	Unsupported []string      `json:"unsupported,omitempty"`
}
type SlideObject struct {
	Hidden           bool          `json:"hidden,omitempty"`
	Insets           *PageMargins  `json:"insets,omitempty"`
	ParagraphAlign   []string      `json:"paragraph_align,omitempty"`
	VerticalAlign    string        `json:"vertical_align,omitempty"`
	NoWrap           bool          `json:"no_wrap,omitempty"`
	ID               string        `json:"id"`
	Kind             string        `json:"kind"` // shape, image, group, native
	Name             string        `json:"name,omitempty"`
	X                float64       `json:"x"`
	Y                float64       `json:"y"`
	Width            float64       `json:"width"`
	Height           float64       `json:"height"`
	Rotation         float64       `json:"rotation,omitempty"` // clockwise degrees
	FlipH            bool          `json:"flip_h,omitempty"`
	FlipV            bool          `json:"flip_v,omitempty"`
	Geometry         string        `json:"geometry,omitempty"` // rect, ellipse, line, roundRect
	Fill             string        `json:"fill,omitempty"`     // #RRGGBB, empty means none
	Stroke           string        `json:"stroke,omitempty"`
	StrokeWidth      float64       `json:"stroke_width,omitempty"`
	Placeholder      string        `json:"placeholder,omitempty"`
	PlaceholderIndex string        `json:"placeholder_index,omitempty"`
	Content          []Block       `json:"content,omitempty"`
	Image            *Image        `json:"image,omitempty"`
	Children         []SlideObject `json:"children,omitempty"`
	Unsupported      []string      `json:"unsupported,omitempty"`
}

type StructuredPresentation struct {
	Version     int                       `json:"version"`
	Width       float64                   `json:"width"`
	Height      float64                   `json:"height"`
	Slides      []StructuredSlide         `json:"slides"`
	Resources   []StructuredSlideResource `json:"resources,omitempty"`
	SourceParts map[string][]byte         `json:"source_parts,omitempty"`
}
type StructuredSlide struct {
	Attrs   PresentationSlide       `json:"attrs"`
	Objects []StructuredSlideObject `json:"objects,omitempty"`
	Notes   []StructuredNode        `json:"notes,omitempty"`
}
type StructuredSlideResource struct {
	Attrs   PresentationResource    `json:"attrs"`
	Objects []StructuredSlideObject `json:"objects,omitempty"`
}
type StructuredSlideObject struct {
	Attrs    SlideObject             `json:"attrs"`
	Content  []StructuredNode        `json:"content,omitempty"`
	Children []StructuredSlideObject `json:"children,omitempty"`
}

func clonePresentation(p *Presentation) *Presentation {
	if p == nil {
		return nil
	}
	data, _ := json.Marshal(p)
	var q Presentation
	_ = json.Unmarshal(data, &q)
	return &q
}
func presentationToSF(p *Presentation) (*StructuredPresentation, error) {
	if p == nil {
		return nil, nil
	}
	p = clonePresentation(p)
	s := &StructuredPresentation{Version: p.Version, Width: p.Width, Height: p.Height, SourceParts: p.SourceParts}
	var encode func([]SlideObject) ([]StructuredSlideObject, error)
	encode = func(objects []SlideObject) ([]StructuredSlideObject, error) {
		var result []StructuredSlideObject
		for _, o := range objects {
			n := StructuredSlideObject{Attrs: o}
			var err error
			n.Content, err = encodeSFBlocks(o.Content)
			if err != nil {
				return nil, err
			}
			n.Children, err = encode(o.Children)
			if err != nil {
				return nil, err
			}
			n.Attrs.Content = nil
			n.Attrs.Children = nil
			result = append(result, n)
		}
		return result, nil
	}
	for _, slide := range p.Slides {
		n := StructuredSlide{Attrs: slide}
		var err error
		n.Objects, err = encode(slide.Objects)
		if err != nil {
			return nil, err
		}
		n.Notes, err = encodeSFBlocks(slide.Notes)
		if err != nil {
			return nil, err
		}
		n.Attrs.Objects = nil
		n.Attrs.Notes = nil
		s.Slides = append(s.Slides, n)
	}
	for _, resource := range p.Resources {
		n := StructuredSlideResource{Attrs: resource}
		var err error
		n.Objects, err = encode(resource.Objects)
		if err != nil {
			return nil, err
		}
		n.Attrs.Objects = nil
		s.Resources = append(s.Resources, n)
	}
	return s, nil
}
func presentationFromSF(s *StructuredPresentation) (*Presentation, error) {
	if s == nil {
		return nil, nil
	}
	p := &Presentation{Version: s.Version, Width: s.Width, Height: s.Height, SourceParts: s.SourceParts}
	var decode func([]StructuredSlideObject, int) ([]SlideObject, error)
	decode = func(ns []StructuredSlideObject, depth int) ([]SlideObject, error) {
		if depth > 32 {
			return nil, fmt.Errorf("presentation group depth exceeds 32")
		}
		var objects []SlideObject
		for _, n := range ns {
			if len(n.Attrs.Content) > 0 || len(n.Attrs.Children) > 0 {
				return nil, fmt.Errorf("presentation attrs cannot duplicate canonical SF content")
			}
			o := n.Attrs
			var err error
			o.Content, err = decodeSFBlocks(n.Content, 0)
			if err != nil {
				return nil, err
			}
			o.Children, err = decode(n.Children, depth+1)
			if err != nil {
				return nil, err
			}
			objects = append(objects, o)
		}
		return objects, nil
	}
	for _, n := range s.Slides {
		if len(n.Attrs.Objects) > 0 || len(n.Attrs.Notes) > 0 {
			return nil, fmt.Errorf("slide attrs duplicate canonical content")
		}
		slide := n.Attrs
		var err error
		slide.Objects, err = decode(n.Objects, 0)
		if err != nil {
			return nil, err
		}
		slide.Notes, err = decodeSFBlocks(n.Notes, 0)
		if err != nil {
			return nil, err
		}
		p.Slides = append(p.Slides, slide)
	}
	for _, n := range s.Resources {
		if len(n.Attrs.Objects) > 0 {
			return nil, fmt.Errorf("resource attrs duplicate canonical objects")
		}
		resource := n.Attrs
		var err error
		resource.Objects, err = decode(n.Objects, 0)
		if err != nil {
			return nil, err
		}
		p.Resources = append(p.Resources, resource)
	}
	return clonePresentation(p), nil
}
func (p *Presentation) validate() error {
	if p.Version != 1 || !finite(p.Width) || !finite(p.Height) || p.Width <= 0 || p.Height <= 0 || p.Width > 14400 || p.Height > 14400 {
		return fmt.Errorf("invalid presentation version/canvas")
	}
	if len(p.Slides) > 10000 {
		return fmt.Errorf("presentation exceeds 10000 slides")
	}
	var bytesCount int64
	for name, data := range p.SourceParts {
		if err := validateString(name); err != nil {
			return err
		}
		bytesCount += int64(len(data))
		if path.Clean(name) != name || strings.HasPrefix(name, "/") || strings.HasPrefix(name, "../") || strings.Contains(name, "\\") || len(data) > 64<<20 {
			return fmt.Errorf("invalid presentation source part %q", name)
		}
	}
	if len(p.SourceParts) > 10000 || bytesCount > 256<<20 {
		return fmt.Errorf("presentation source resource limit")
	}
	validColor := func(c string) error {
		if c == "" || c == "none" {
			return nil
		}
		if c == "auto" {
			return fmt.Errorf("canvas/background color requires explicit RGB")
		}
		return validateTextStyle(Run{Color: c})
	}
	validateBlocks := func(bs []Block) error { return (&Document{Blocks: bs}).Validate() }
	var objects func([]SlideObject, int, map[string]bool) error
	objects = func(os []SlideObject, depth int, ids map[string]bool) error {
		if depth > 32 {
			return fmt.Errorf("presentation object depth exceeds 32")
		}
		for _, o := range os {
			if o.ID == "" || ids[o.ID] {
				return fmt.Errorf("empty/duplicate slide object ID")
			}
			ids[o.ID] = true
			if len(ids) > 10000 {
				return fmt.Errorf("slide object count exceeds 10000")
			}
			for _, v := range []float64{o.X, o.Y, o.Width, o.Height, o.Rotation, o.StrokeWidth} {
				if !finite(v) || math.Abs(v) > 14400 {
					return fmt.Errorf("invalid slide object geometry")
				}
			}
			if o.Width < 0 || o.Height < 0 || o.StrokeWidth < 0 {
				return fmt.Errorf("negative slide extent")
			}
			switch o.Kind {
			case "shape", "image", "group", "native":
			default:
				return fmt.Errorf("unsupported slide object kind %q", o.Kind)
			}
			for _, c := range []string{o.Fill, o.Stroke} {
				if c != "" && c != "none" {
					if err := validateTextStyle(Run{Color: c}); err != nil {
						return err
					}
				}
			}
			for _, s := range append([]string{o.ID, o.Name, o.Geometry, o.Placeholder, o.PlaceholderIndex}, o.Unsupported...) {
				if err := validateString(s); err != nil {
					return err
				}
			}
			if o.Insets != nil {
				for _, v := range []float64{o.Insets.Top, o.Insets.Bottom, o.Insets.Left, o.Insets.Right} {
					if !finite(v) || v < 0 || v > 14400 {
						return fmt.Errorf("invalid slide text inset")
					}
				}
			}
			if len(o.ParagraphAlign) > 0 && len(o.ParagraphAlign) != len(o.Content) {
				return fmt.Errorf("paragraph alignment count must match content")
			}
			for _, a := range o.ParagraphAlign {
				if a != "" && a != "l" && a != "ctr" && a != "r" {
					return fmt.Errorf("unsupported paragraph alignment %q", a)
				}
			}
			if o.VerticalAlign != "" && o.VerticalAlign != "t" && o.VerticalAlign != "ctr" && o.VerticalAlign != "b" {
				return fmt.Errorf("unsupported vertical text anchor")
			}
			if err := validateBlocks(o.Content); err != nil {
				return err
			}
			if o.Image != nil {
				if err := validateBlocks([]Block{{Kind: Picture, Image: o.Image}}); err != nil {
					return err
				}
			}
			if o.Image != nil && o.Kind != "image" {
				return fmt.Errorf("only image objects can own image pixels")
			}
			if o.Kind == "image" && (len(o.Content) > 0 || len(o.Children) > 0) {
				return fmt.Errorf("image objects cannot also own text/group content")
			}
			if o.Kind == "image" && o.Image == nil {
				return fmt.Errorf("image object requires pixels")
			}
			if o.Kind != "group" && len(o.Children) > 0 {
				return fmt.Errorf("children require group")
			}
			if err := objects(o.Children, depth+1, ids); err != nil {
				return err
			}
		}
		return nil
	}
	ids := map[string]bool{}
	partsUsed := map[string]bool{}
	for _, s := range p.Slides {
		if err := validColor(s.Background); err != nil {
			return err
		}
		if s.Part != "" {
			if partsUsed[s.Part] {
				return fmt.Errorf("duplicate slide part")
			}
			partsUsed[s.Part] = true
		}
		if s.ID == "" || ids[s.ID] {
			return fmt.Errorf("duplicate/empty slide ID")
		}
		ids[s.ID] = true
		if s.Width < 0 || s.Height < 0 || !finite(s.Width) || !finite(s.Height) || s.Width > 14400 || s.Height > 14400 {
			return fmt.Errorf("invalid slide size")
		}
		if err := objects(s.Objects, 0, map[string]bool{}); err != nil {
			return err
		}
		if err := validateBlocks(s.Notes); err != nil {
			return err
		}
	}
	resourceIDs := map[string]bool{}
	for _, r := range p.Resources {
		if err := validColor(r.Background); err != nil {
			return err
		}
		if r.Part == "" || resourceIDs[r.Part] || (r.Kind != "master" && r.Kind != "layout") {
			return fmt.Errorf("invalid presentation resource")
		}
		resourceIDs[r.Part] = true
		if err := objects(r.Objects, 0, map[string]bool{}); err != nil {
			return err
		}
	}
	return nil
}
func presentationText(p *Presentation) string {
	var b strings.Builder
	var walk func([]SlideObject)
	walk = func(objects []SlideObject) {
		for _, o := range objects {
			if o.Hidden {
				continue
			}
			textBlocks(&b, o.Content)
			if o.Image != nil && o.Image.Alt != "" {
				b.WriteString(o.Image.Alt + "\n")
			}
			walk(o.Children)
		}
	}
	for i, s := range p.Slides {
		if s.Hidden {
			continue
		}
		if i > 0 {
			b.WriteString("\f")
		}
		for _, part := range []string{s.MasterPart, s.LayoutPart} {
			for _, resource := range p.Resources {
				if resource.Part == part {
					for _, o := range resource.Objects {
						if o.Placeholder == "" {
							walk([]SlideObject{o})
						}
					}
				}
			}
		}
		walk(s.Objects)
		textBlocks(&b, s.Notes)
	}
	return strings.TrimRight(b.String(), "\n")
}
func sameSlideObject(a, b SlideObject) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return bytes.Equal(x, y)
}
