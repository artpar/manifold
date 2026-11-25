package engine

import (
	"context"
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"

	opc "github.com/artpar/manifold/internal/presentation/pptx"
)

const pptxEMU = 12700.0

// ReadPPTX maps a bounded native package to the central, editable SF model.
func ReadPPTX(ctx context.Context, input io.Reader) (*Document, error) {
	if input == nil {
		return nil, fmt.Errorf("nil PPTX reader")
	}
	pkg, err := opc.OpenReader(ctx, contextReader{ctx, input})
	if err != nil {
		return nil, err
	}
	d, err := presentationFromPackage(ctx, pkg)
	if err != nil {
		return nil, err
	}
	return d, d.Validate()
}
func presentationFromPackage(ctx context.Context, pkg *opc.Package) (*Document, error) {
	root, err := parseXML(ctx, pkg.Parts[pkg.PresentationPath])
	if err != nil {
		return nil, err
	}
	size := root.child("sldSz")
	w, h := pptxNumber(size, "cx")/pptxEMU, pptxNumber(size, "cy")/pptxEMU
	if w <= 0 || h <= 0 {
		w, h = 720, 540
	}
	p := &Presentation{Version: 1, Width: w, Height: h, SourceParts: pkg.Parts}
	d := &Document{Presentation: p, Layout: Layout{Width: w, Height: h}}
	for _, rel := range pkg.RootRelationships {
		if rel.Type == opc.CorePropertiesRelType {
			if n, e := parseXML(ctx, pkg.Parts[opc.ResolveTargetPart("", rel.Target)]); e == nil {
				if t := n.child("title"); t != nil {
					d.Metadata.Title = t.content()
				}
				if t := n.child("creator"); t != nil {
					d.Metadata.Author = t.content()
				}
				if t := n.child("subject"); t != nil {
					d.Metadata.Subject = t.content()
				}
			}
		}
	}
	ids := root.child("sldIdLst").all("sldId")
	for i, part := range pkg.SlideParts {
		n, e := parseXML(ctx, pkg.Parts[part])
		if e != nil {
			return nil, e
		}
		slide := PresentationSlide{ID: strconv.Itoa(256 + i), Part: part}
		if i < len(ids) && ids[i].attr("id") != "" {
			slide.ID = ids[i].attr("id")
		}
		rels, e := pkg.RelationshipsForPart(part)
		if e != nil {
			return nil, e
		}
		for _, rel := range rels {
			target := opc.ResolveTargetPart(part, rel.Target)
			switch rel.Type {
			case opc.SlideLayoutRelType:
				slide.LayoutPart = target
				lr, e := pkg.RelationshipsForPart(target)
				if e != nil {
					return nil, e
				}
				for _, r := range lr {
					if r.Type == opc.SlideMasterRelType {
						slide.MasterPart = opc.ResolveTargetPart(target, r.Target)
					}
				}
			case opc.NotesSlideRelType:
				if notes, e := parseXML(ctx, pkg.Parts[target]); e == nil {
					for _, shape := range notes.all("sp") {
						ph := shape.child("nvSpPr").child("nvPr").child("ph")
						if ph.attr("type") == "sldNum" || ph.attr("type") == "hdr" || ph.attr("type") == "ftr" {
							continue
						}
						slide.Notes = append(slide.Notes, pptxParagraphs(shape.child("txBody"), nil, "", nil)...)
					}
				} else {
					return nil, e
				}
			}
		}
		parser := pptxSlideParser{ctx: ctx, pkg: pkg, part: part, width: w, height: h}
		slide.Objects, e = parser.objects(n.child("cSld").child("spTree"), 0)
		if e != nil {
			return nil, e
		}
		slide.Background = parser.background(n.child("cSld").child("bg"))
		slide.Unsupported = append(slide.Unsupported, parser.unsupported...)
		for _, feature := range []string{"timing", "transition"} {
			if n.child(feature) != nil {
				slide.Unsupported = append(slide.Unsupported, "native "+feature+" is preserved but not an editable static SF object")
			}
		}
		p.Slides = append(p.Slides, slide)
	}
	names := pkg.PartNames()
	for _, part := range names {
		kind := ""
		ct := pkg.ContentTypes.ForPart(part)
		if strings.Contains(ct, "presentationml.slideMaster+xml") {
			kind = "master"
		}
		if strings.Contains(ct, "presentationml.slideLayout+xml") {
			kind = "layout"
		}
		if kind == "" {
			continue
		}
		n, e := parseXML(ctx, pkg.Parts[part])
		if e != nil {
			return nil, e
		}
		parser := pptxSlideParser{ctx: ctx, pkg: pkg, part: part, width: w, height: h}
		objects, e := parser.objects(n.child("cSld").child("spTree"), 0)
		if e != nil {
			return nil, e
		}
		r := PresentationResource{Part: part, Kind: kind, Objects: objects, Background: parser.background(n.child("cSld").child("bg")), Unsupported: parser.unsupported}
		rels, e := pkg.RelationshipsForPart(part)
		if e != nil {
			return nil, e
		}
		for _, rel := range rels {
			if rel.Type == opc.SlideMasterRelType {
				r.MasterPart = opc.ResolveTargetPart(part, rel.Target)
			}
		}
		p.Resources = append(p.Resources, r)
	}
	// Resolve placeholder frames from explicit layout/master objects without copying
	// inherited content into a second editable owner.
	for si := range p.Slides {
		s := &p.Slides[si]
		for oi := range s.Objects {
			o := &s.Objects[oi]
			if o.Placeholder == "" {
				continue
			}
			for _, part := range []string{s.LayoutPart, s.MasterPart} {
				for _, r := range p.Resources {
					if r.Part != part {
						continue
					}
					for _, base := range r.Objects {
						if base.Placeholder != "" && ((o.PlaceholderIndex != "" && o.PlaceholderIndex == base.PlaceholderIndex) || (o.PlaceholderIndex == "" && o.Placeholder == base.Placeholder)) {
							if o.Width == 0 || o.Height == 0 {
								o.X, o.Y, o.Width, o.Height = base.X, base.Y, base.Width, base.Height
							}
						}
					}
				}
			}
		}
	}
	for si := range p.Slides {
		var scan func([]SlideObject)
		scan = func(objects []SlideObject) {
			for _, o := range objects {
				for _, u := range o.Unsupported {
					d.warn("pptx_representation", u)
				}
				scan(o.Children)
			}
		}
		scan(p.Slides[si].Objects)
		for _, u := range p.Slides[si].Unsupported {
			d.warn("pptx_representation", u)
		}
	}
	d.Metadata.PageCount = len(p.Slides)
	return d, nil
}

type pptxSlideParser struct {
	ctx           context.Context
	pkg           *opc.Package
	part          string
	width, height float64
	unsupported   []string
}

func pptxNumber(n *xmlNode, key string) float64 {
	v, _ := strconv.ParseFloat(n.attr(key), 64)
	return v
}
func (r *pptxSlideParser) color(n *xmlNode) string {
	if n == nil {
		return ""
	}
	if s := n.child("srgbClr"); s != nil {
		if len(s.Children) > 0 {
			r.unsupported = append(r.unsupported, "color transforms are preserved in native source")
		}
		return "#" + strings.ToUpper(s.attr("val"))
	}
	if c := n.child("schemeClr"); c != nil {
		for _, name := range r.pkg.PartNames() {
			if !strings.Contains(r.pkg.ContentTypes.ForPart(name), "theme+xml") {
				continue
			}
			root, e := parseXML(r.ctx, r.pkg.Parts[name])
			if e != nil {
				continue
			}
			scheme := root.child("themeElements").child("clrScheme")
			key := c.attr("val")
			switch key {
			case "bg1":
				key = "lt1"
			case "tx1":
				key = "dk1"
			case "bg2":
				key = "lt2"
			case "tx2":
				key = "dk2"
			}
			entry := scheme.child(key)
			if v := entry.child("srgbClr"); v != nil {
				return "#" + strings.ToUpper(v.attr("val"))
			}
			if v := entry.child("sysClr"); v != nil {
				return "#" + strings.ToUpper(v.attr("lastClr"))
			}
		}
	}
	r.unsupported = append(r.unsupported, "unresolved native color is preserved in source parts")
	return ""
}
func (r *pptxSlideParser) background(n *xmlNode) string {
	if n == nil {
		return ""
	}
	if fill := n.child("bgPr").child("solidFill"); fill != nil {
		return r.color(fill)
	}
	r.unsupported = append(r.unsupported, "non-solid slide background is preserved in source parts")
	return ""
}
func (r *pptxSlideParser) objects(tree *xmlNode, depth int) ([]SlideObject, error) {
	if depth > 32 {
		return nil, fmt.Errorf("PPTX group depth exceeds 32")
	}
	if tree == nil {
		return nil, nil
	}
	rels, err := r.pkg.RelationshipsForPart(r.part)
	if err != nil {
		return nil, err
	}
	targets := map[string]opc.Relationship{}
	for _, rel := range rels {
		targets[rel.ID] = rel
	}
	var objects []SlideObject
	for _, n := range tree.Children {
		switch n.Name.Local {
		case "nvGrpSpPr", "grpSpPr", "extLst":
			continue
		}
		ids := n.all("cNvPr")
		if len(ids) == 0 {
			r.unsupported = append(r.unsupported, "unmodeled spTree child "+n.Name.Local+" preserved in source")
			continue
		}
		o := SlideObject{ID: ids[0].attr("id"), Name: ids[0].attr("name"), Kind: "shape"}
		if o.ID == "" {
			return nil, fmt.Errorf("PPTX object has no ID")
		}
		phs := n.all("ph")
		if len(phs) > 0 {
			o.Placeholder = phs[0].attr("type")
			if o.Placeholder == "" {
				o.Placeholder = "body"
			}
			o.PlaceholderIndex = phs[0].attr("idx")
		}
		props := n.child("spPr")
		if n.Name.Local == "grpSp" {
			props = n.child("grpSpPr")
		}
		if props == nil {
			props = &xmlNode{}
		}
		xfrm := props.child("xfrm")
		if n.Name.Local == "graphicFrame" {
			xfrm = n.child("xfrm")
		}
		off, ext := xfrm.child("off"), xfrm.child("ext")
		o.X, o.Y = pptxNumber(off, "x")/pptxEMU, pptxNumber(off, "y")/pptxEMU
		o.Width, o.Height = pptxNumber(ext, "cx")/pptxEMU, pptxNumber(ext, "cy")/pptxEMU
		o.Rotation = pptxNumber(xfrm, "rot") / 60000
		o.FlipH = xfrm.attr("flipH") == "1" || xfrm.attr("flipH") == "true"
		o.FlipV = xfrm.attr("flipV") == "1" || xfrm.attr("flipV") == "true"
		o.Geometry = props.child("prstGeom").attr("prst")
		if o.Geometry == "" {
			o.Geometry = "rect"
		}
		o.Fill = r.color(props.child("solidFill"))
		line := props.child("ln")
		o.Stroke = r.color(line.child("solidFill"))
		o.StrokeWidth = pptxNumber(line, "w") / pptxEMU
		for _, c := range props.Children {
			switch c.Name.Local {
			case "xfrm", "prstGeom", "solidFill", "noFill", "ln", "extLst":
			default:
				o.Unsupported = append(o.Unsupported, "native shape feature "+c.Name.Local+" is preserved but not editable in this profile")
			}
		}
		if av := props.child("prstGeom").child("avLst"); av != nil && len(av.Children) > 0 {
			o.Unsupported = append(o.Unsupported, "native shape adjustments are preserved")
		}
		switch n.Name.Local {
		case "sp", "cxnSp":
			if n.Name.Local == "cxnSp" {
				o.Geometry = "line"
			}
			o.Content = pptxParagraphs(n.child("txBody"), targets, r.part, &o.Unsupported)
		case "pic":
			o.Kind = "image"
			blips := n.all("blip")
			if len(blips) == 0 {
				return nil, fmt.Errorf("PPTX image has no blip")
			}
			rel := targets[blips[0].attr("embed")]
			if rel.Target == "" || rel.TargetMode != "" && rel.TargetMode != "Internal" {
				o.Kind = "native"
				o.Unsupported = append(o.Unsupported, "external or unresolved image preserved in native source")
			} else {
				target := opc.ResolveTargetPart(r.part, rel.Target)
				data, ok := r.pkg.Parts[target]
				if !ok {
					return nil, fmt.Errorf("missing PPTX media %s", target)
				}
				o.Image = &Image{Data: append([]byte(nil), data...), MediaType: r.pkg.ContentTypes.ForPart(target), Alt: ids[0].attr("descr"), Width: o.Width, Height: o.Height}
			}
			for _, feature := range []string{"srcRect", "tile"} {
				if len(n.all(feature)) > 0 {
					o.Unsupported = append(o.Unsupported, "image "+feature+" is preserved in native source")
				}
			}
		case "grpSp":
			o.Kind = "group"
			o.Children, err = r.objects(n, depth+1)
			if err != nil {
				return nil, err
			}
			o.Unsupported = append(o.Unsupported, "native group coordinate transforms require explicit supported output handling")
		case "graphicFrame":
			o.Kind = "native"
			o.Unsupported = append(o.Unsupported, "native graphic frame (table/chart/diagram) is retained in source parts")
			for _, p := range n.all("p") {
				o.Content = append(o.Content, pptxParagraphs(&xmlNode{Children: []*xmlNode{p}}, targets, r.part, nil)...)
			}
		default:
			o.Kind = "native"
			o.Unsupported = append(o.Unsupported, "native object "+n.Name.Local+" is retained in source parts")
		}
		if o.Width == 0 || o.Height == 0 {
			if o.Placeholder == "" && o.Kind != "native" {
				o.Unsupported = append(o.Unsupported, "object has no explicit frame; native inheritance/defaults are preserved")
			}
		}
		objects = append(objects, o)
	}
	return objects, nil
}
func pptxParagraphs(body *xmlNode, rels map[string]opc.Relationship, part string, unsupported *[]string) []Block {
	if body == nil {
		return nil
	}
	var result []Block
	for _, p := range body.Children {
		if p.Name.Local != "p" {
			continue
		}
		b := Block{Kind: Paragraph}
		defaults := pptxRunStyle(p.child("pPr").child("defRPr"), Run{}, unsupported)
		for _, c := range p.Children {
			switch c.Name.Local {
			case "r", "fld":
				style := pptxRunStyle(c.child("rPr"), defaults, unsupported)
				style.Text = c.child("t").content()
				link := c.child("rPr").child("hlinkClick").attr("id")
				if rel, ok := rels[link]; ok {
					if rel.TargetMode == "External" {
						style.Link = rel.Target
					} else if unsupported != nil {
						*unsupported = append(*unsupported, "internal slide hyperlink is retained in native source")
					}
				}
				b.Runs = append(b.Runs, style)
			case "br":
				b.Runs = append(b.Runs, Run{Text: "\n"})
			}
		}
		props := p.child("pPr")
		if props != nil && unsupported != nil {
			for _, c := range props.Children {
				switch c.Name.Local {
				case "defRPr", "extLst":
				default:
					*unsupported = append(*unsupported, "native paragraph property "+c.Name.Local+" is retained in source")
				}
			}
			if props.attr("algn") != "" && props.attr("algn") != "l" {
				*unsupported = append(*unsupported, "native paragraph alignment is retained in source")
			}
		}
		result = append(result, b)
	}
	return result
}
func pptxRunStyle(n *xmlNode, base Run, unsupported *[]string) Run {
	if n == nil {
		return base
	}
	if v := n.attr("b"); v != "" {
		base.Bold = v == "1" || v == "true"
	}
	if v := n.attr("i"); v != "" {
		base.Italic = v == "1" || v == "true"
	}
	if v := n.attr("u"); v != "" {
		base.Underline = v != "none"
	}
	if v := n.attr("strike"); v != "" {
		base.Strike = v != "noStrike"
	}
	if n.attr("sz") != "" {
		base.FontSize = pptxNumber(n, "sz") / 100
	}
	if v := n.attr("lang"); v != "" {
		base.Language = v
	}
	if latin := n.child("latin"); latin != nil {
		base.FontFamily = latin.attr("typeface")
	}
	if color := n.child("solidFill").child("srgbClr"); color != nil {
		base.Color = "#" + strings.ToUpper(color.attr("val"))
	}
	if unsupported != nil {
		for _, c := range n.Children {
			switch c.Name.Local {
			case "latin", "solidFill", "hlinkClick", "extLst":
			default:
				*unsupported = append(*unsupported, "native run property "+c.Name.Local+" preserved in source")
			}
		}
	}
	return base
}
func slideDimensions(p *Presentation, s PresentationSlide) (float64, float64) {
	w, h := p.Width, p.Height
	if s.Width > 0 {
		w = s.Width
	}
	if s.Height > 0 {
		h = s.Height
	}
	return w, h
}
func presentationQuadBox(q Quad) (float64, float64, float64, float64) {
	x0, y0, x1, y1 := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	for _, pt := range q {
		x0 = math.Min(x0, pt.X)
		y0 = math.Min(y0, pt.Y)
		x1 = math.Max(x1, pt.X)
		y1 = math.Max(y1, pt.Y)
	}
	return x0, y0, x1 - x0, y1 - y0
}
func sortedParts(parts map[string][]byte) []string {
	names := make([]string, 0, len(parts))
	for n := range parts {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
