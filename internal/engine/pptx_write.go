package engine

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"math"
	"path"
	"reflect"
	"sort"
	"strconv"
	"strings"

	opc "github.com/artpar/manifold/internal/presentation/pptx"
)

const pptxNS = `xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"`
const pptxRelsNS = "http://schemas.openxmlformats.org/package/2006/relationships"
const pptxRelBase = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/"
const pptxContentBase = "application/vnd.openxmlformats-officedocument.presentationml."

func writePPTX(ctx context.Context, out io.Writer, d *Document, opts WriteOptions) error {
	p := d.Presentation
	if p == nil {
		if !opts.AllowLossy {
			return fmt.Errorf("%w: flowing/PDF content is mapped to positioned slides; pass AllowLossy after reviewing source diagnostics", ErrLossy)
		}
		var err error
		p, err = flowToPresentation(d)
		if err != nil {
			return err
		}
	}
	if err := p.validate(); err != nil {
		return err
	}
	var original *Document
	var pkg *opc.Package
	var err error
	parts := map[string][]byte{}
	for n, data := range p.SourceParts {
		parts[n] = append([]byte(nil), data...)
	}
	if len(parts) > 0 {
		pkg, err = opc.FromParts(ctx, parts, "")
		if err != nil {
			return fmt.Errorf("PPTX source parts: %w", err)
		}
		original, err = presentationFromPackage(ctx, pkg)
		if err != nil {
			return err
		}
	} else {
		parts = blankPPTXParts(p, d.Metadata)
		pkg, err = opc.FromParts(ctx, parts, "")
		if err != nil {
			return err
		}
	}
	if original != nil && reflect.DeepEqual(p, original.Presentation) && d.Metadata.Title == original.Metadata.Title && d.Metadata.Author == original.Metadata.Author && d.Metadata.Subject == original.Metadata.Subject {
		return emitPPTXArchive(ctx, out, parts)
	}
	sameOrder := original != nil && p.Width == original.Presentation.Width && p.Height == original.Presentation.Height && len(p.Slides) == len(original.Presentation.Slides)
	if sameOrder {
		for i, slide := range p.Slides {
			old := original.Presentation.Slides[i]
			if slide.ID != old.ID || slide.Part != old.Part {
				sameOrder = false
				break
			}
		}
	}
	presentationPart := pkg.PresentationPath
	var slideRefs strings.Builder
	var slideRels []opc.Relationship
	for _, rel := range pkg.PresentationRelationships {
		if rel.Type != pptxRelBase+"slide" {
			slideRels = append(slideRels, rel)
		}
	}
	reservedRelIDs := map[string]bool{}
	originalSlideIDs := map[string]string{}
	for _, rel := range pkg.PresentationRelationships {
		reservedRelIDs[rel.ID] = true
		if rel.Type == pptxRelBase+"slide" {
			originalSlideIDs[opc.ResolveTargetPart(presentationPart, rel.Target)] = rel.ID
		}
	}
	oldSlides := map[string]PresentationSlide{}
	oldResources := map[string]PresentationResource{}
	if original != nil {
		for _, s := range original.Presentation.Slides {
			oldSlides[s.Part] = s
		}
		for _, r := range original.Presentation.Resources {
			oldResources[r.Part] = r
		}
	}
	needsBlank := false
	for _, slide := range p.Slides {
		if _, exists := oldSlides[slide.Part]; !exists && slide.LayoutPart == "" {
			needsBlank = true
		}
	}
	if needsBlank && parts["ppt/slideLayouts/manifold-blank.xml"] == nil {
		blank := blankPPTXParts(p, d.Metadata)
		for name, data := range blank {
			if strings.HasPrefix(name, "ppt/slideLayouts/") || strings.HasPrefix(name, "ppt/slideMasters/") {
				parts[name] = data
			}
		}
		ensurePPTXContentType(parts, "ppt/slideLayouts/manifold-blank.xml", pptxContentBase+"slideLayout+xml")
		ensurePPTXContentType(parts, "ppt/slideMasters/manifold-blank.xml", pptxContentBase+"slideMaster+xml")
		relID := ""
		for n := 1; ; n++ {
			candidate := fmt.Sprintf("rId%d", n)
			if !reservedRelIDs[candidate] {
				relID = candidate
				reservedRelIDs[relID] = true
				break
			}
		}
		slideRels = append(slideRels, opc.Relationship{ID: relID, Type: opc.SlideMasterRelType, Target: relativePart(presentationPart, "ppt/slideMasters/manifold-blank.xml")})
		root, e := parseXML(ctx, parts[presentationPart])
		if e != nil {
			return e
		}
		list := root.child("sldMasterIdLst")
		id := uint64(2147483648)
		if list != nil {
			for _, node := range list.Children {
				v, _ := strconv.ParseUint(node.attr("id"), 10, 32)
				if v >= id {
					id = v + 1
				}
			}
		}
		entry := fmt.Sprintf(`<p:sldMasterId id="%d" r:id="%s"/>`, id, escape(relID))
		data := parts[presentationPart]
		if list != nil {
			at := int(list.rawEnd) - len(`</p:sldMasterIdLst>`)
			closeAt := bytes.LastIndex(data[list.rawStart:list.rawEnd], []byte(`</`))
			if closeAt < 0 {
				return fmt.Errorf("invalid master list")
			}
			at = int(list.rawStart) + closeAt
			data = append(append(append([]byte{}, data[:at]...), []byte(entry)...), data[at:]...)
		} else {
			first := bytes.IndexByte(data[root.rawStart:], '>') + int(root.rawStart) + 1
			data = append(append(append([]byte{}, data[:first]...), []byte(`<p:sldMasterIdLst>`+entry+`</p:sldMasterIdLst>`)...), data[first:]...)
		}
		parts[presentationPart] = ensurePPTXNamespaces(data)
	}
	for i, s := range p.Slides {
		if err := ctx.Err(); err != nil {
			return err
		}
		w, h := slideDimensions(p, s)
		if math.Abs(w-p.Width) > .01 || math.Abs(h-p.Height) > .01 {
			return fmt.Errorf("%w: PPTX requires a common slide canvas", ErrUnsupported)
		}
		part := s.Part
		if part == "" {
			part = fmt.Sprintf("ppt/slides/manifold-slide-%d.xml", i+1)
		}
		if path.Clean(part) != part || !strings.HasSuffix(part, ".xml") || strings.HasPrefix(part, "../") || strings.HasPrefix(part, "/") {
			return fmt.Errorf("invalid slide part")
		}
		relID := originalSlideIDs[part]
		if relID == "" {
			for n := 1; ; n++ {
				candidate := fmt.Sprintf("rId%d", n)
				if !reservedRelIDs[candidate] {
					relID = candidate
					reservedRelIDs[relID] = true
					break
				}
			}
		}
		slideRels = append(slideRels, opc.Relationship{ID: relID, Type: pptxRelBase + "slide", Target: relativePart(presentationPart, part)})
		nativeID, parseErr := strconv.ParseUint(s.ID, 10, 32)
		if parseErr != nil || nativeID < 256 {
			nativeID = uint64(256 + i)
		}
		fmt.Fprintf(&slideRefs, `<p:sldId id="%d" r:id="%s"/>`, nativeID, escape(relID))
		old, hasOld := oldSlides[s.Part]
		if hasOld && (s.LayoutPart != old.LayoutPart || s.MasterPart != old.MasterPart) {
			return fmt.Errorf("%w: changing native layout/master bindings requires an explicit new resource mapping", ErrUnsupported)
		}

		if !hasOld || !reflect.DeepEqual(s.Objects, old.Objects) || s.Background != old.Background || s.Hidden != old.Hidden {
			if hasOld && len(old.Unsupported) > 0 {
				return fmt.Errorf("%w: edited native slide has unmodeled features: %s", ErrUnsupported, old.Unsupported[0])
			}
			var raw []byte
			if hasOld {
				raw = parts[part]
			}
			rels, _ := pkg.RelationshipsForPart(part)
			if !hasOld {
				layout := s.LayoutPart
				if layout == "" {
					layout = "ppt/slideLayouts/manifold-blank.xml"
				}
				rels = append(rels, opc.Relationship{ID: nextPresentationRelID(rels), Type: opc.SlideLayoutRelType, Target: relativePart(part, layout)})
			}
			content, updated, e := buildPPTXObjects(ctx, parts, part, s.Objects, old.Objects, raw, rels)
			if e != nil {
				return e
			}
			if hasOld {
				parts[part], e = patchPPTXTree(ctx, raw, content, s.Background, old.Background)
				if e == nil && s.Hidden != old.Hidden {
					parts[part], e = patchSlideVisibility(ctx, parts[part], s.Hidden)
				}
			} else {
				parts[part] = []byte(`<?xml version="1.0" encoding="UTF-8"?><p:sld show="` + strconv.FormatBool(!s.Hidden) + `" ` + pptxNS + `><p:cSld>` + pptxBackground(s.Background) + `<p:spTree>` + pptxGroupRoot() + content + `</p:spTree></p:cSld><p:clrMapOvr><a:masterClrMapping/></p:clrMapOvr></p:sld>`)
			}
			if e != nil {
				return e
			}
			if parts[opc.RelationshipsPartFor(part)] == nil || !reflect.DeepEqual(rels, updated) {
				parts[opc.RelationshipsPartFor(part)] = pptxRelationships(updated)
			}
		}
		if !hasOld || !reflect.DeepEqual(s.Notes, old.Notes) {
			if err := writePPTXNotes(ctx, parts, part, s.Notes); err != nil {
				return err
			}
		}
		ensurePPTXContentType(parts, part, pptxContentBase+"slide+xml")
	}
	for _, resource := range p.Resources {
		old, ok := oldResources[resource.Part]
		if !ok {
			return fmt.Errorf("%w: authored master/layout resources need a native source part", ErrUnsupported)
		}
		if resource.Kind != old.Kind || resource.MasterPart != old.MasterPart {
			return fmt.Errorf("%w: native resource binding edits", ErrUnsupported)
		}
		if reflect.DeepEqual(resource, old) {
			continue
		}
		if len(old.Unsupported) > 0 {
			return fmt.Errorf("%w: edited resource has native features", ErrUnsupported)
		}
		raw := parts[resource.Part]
		rels, _ := pkg.RelationshipsForPart(resource.Part)
		content, updated, e := buildPPTXObjects(ctx, parts, resource.Part, resource.Objects, old.Objects, raw, rels)
		if e != nil {
			return e
		}
		parts[resource.Part], e = patchPPTXTree(ctx, raw, content, resource.Background, old.Background)
		if e != nil {
			return e
		}
		if !reflect.DeepEqual(rels, updated) {
			parts[opc.RelationshipsPartFor(resource.Part)] = pptxRelationships(updated)
		}
	}
	if !sameOrder {
		root, err := parseXML(ctx, parts[presentationPart])
		if err != nil {
			return err
		}
		data := parts[presentationPart]
		sizeXML := fmt.Sprintf(`<p:sldSz cx="%d" cy="%d"/>`, int64(math.Round(p.Width*pptxEMU)), int64(math.Round(p.Height*pptxEMU)))
		// Replace later source ranges first, using fresh parsing after each patch.
		data, err = replacePPTXNode(ctx, data, "sldIdLst", `<p:sldIdLst>`+slideRefs.String()+`</p:sldIdLst>`)
		if err != nil {
			return err
		}
		if root.child("sldSz") != nil {
			data, err = replacePPTXNode(ctx, data, "sldSz", sizeXML)
			if err != nil {
				return err
			}
		}
		data = ensurePPTXNamespaces(data)
		parts[presentationPart] = data
		parts[opc.RelationshipsPartFor(presentationPart)] = pptxRelationships(slideRels)
	}

	if original == nil || d.Metadata.Title != original.Metadata.Title || d.Metadata.Author != original.Metadata.Author || d.Metadata.Subject != original.Metadata.Subject {
		writePPTXCore(parts, d.Metadata)
	}
	// Validate final structure before emitting any caller-visible bytes.
	if _, err := opc.FromParts(ctx, parts, ""); err != nil {
		return err
	}
	return emitPPTXArchive(ctx, out, parts)
}
func emitPPTXArchive(ctx context.Context, out io.Writer, parts map[string][]byte) error {
	var staged bytes.Buffer
	zw := zip.NewWriter(&staged)
	for _, name := range sortedParts(parts) {
		if err := ctx.Err(); err != nil {
			zw.Close()
			return err
		}
		w, err := zw.Create(name)
		if err != nil {
			return err
		}
		if _, err = w.Write(parts[name]); err != nil {
			return err
		}
	}
	if err := zw.Close(); err != nil {
		return err
	}
	_, err := io.Copy(contextWriter{ctx, out}, &staged)
	return err
}
func relativePart(from, to string) string {
	from = strings.TrimSuffix(path.Dir(from), ".")
	if strings.HasPrefix(to, from+"/") {
		return strings.TrimPrefix(to, from+"/")
	}
	depth := strings.Count(strings.Trim(from, "/"), "/") + 1
	if from == "" {
		depth = 0
	}
	return strings.Repeat("../", depth) + to
}
func nextPresentationRelID(rels []opc.Relationship) string {
	used := map[string]bool{}
	for _, r := range rels {
		used[r.ID] = true
	}
	for i := 1; ; i++ {
		id := fmt.Sprintf("rId%d", i)
		if !used[id] {
			return id
		}
	}
}
func pptxRelationships(rels []opc.Relationship) []byte {
	var b strings.Builder
	b.WriteString(`<Relationships xmlns="` + pptxRelsNS + `">`)
	for _, r := range rels {
		fmt.Fprintf(&b, `<Relationship Id="%s" Type="%s" Target="%s"`, escape(r.ID), escape(r.Type), escape(r.Target))
		if r.TargetMode != "" {
			fmt.Fprintf(&b, ` TargetMode="%s"`, escape(r.TargetMode))
		}
		b.WriteString(`/>`)
	}
	b.WriteString(`</Relationships>`)
	return []byte(b.String())
}
func pptxGroupRoot() string {
	return `<p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr><p:grpSpPr/>`
}
func pptxBackground(c string) string {
	if c == "" || c == "none" {
		return ""
	}
	return `<p:bg><p:bgPr><a:solidFill><a:srgbClr val="` + escape(strings.TrimPrefix(c, "#")) + `"/></a:solidFill><a:effectLst/></p:bgPr></p:bg>`
}
func ensurePPTXNamespaces(data []byte) []byte {
	root, err := parseXML(context.Background(), data)
	if err != nil {
		return data
	}
	text := string(data)
	end := strings.IndexByte(text[int(root.rawStart):], '>') + int(root.rawStart)
	if end < 0 {
		return data
	}
	add := ""
	for prefix, ns := range map[string]string{"p": "http://schemas.openxmlformats.org/presentationml/2006/main", "a": "http://schemas.openxmlformats.org/drawingml/2006/main", "r": pptxRelBase[:len(pptxRelBase)-1]} {
		found := false
		for _, a := range root.Attr {
			if a.Name.Space == "xmlns" && a.Name.Local == prefix {
				found = true
			}
		}
		if !found {
			add += ` xmlns:` + prefix + `="` + ns + `"`
		}
	}
	return []byte(text[:end] + add + text[end:])
}
func replacePPTXNode(ctx context.Context, data []byte, name, replacement string) ([]byte, error) {
	root, err := parseXML(ctx, data)
	if err != nil {
		return nil, err
	}
	nodes := root.all(name)
	if len(nodes) != 1 {
		return nil, fmt.Errorf("expected exactly one native %s", name)
	}
	n := nodes[0]
	return append(append(append([]byte{}, data[:n.rawStart]...), []byte(replacement)...), data[n.rawEnd:]...), nil
}
func patchPPTXTree(ctx context.Context, raw []byte, content, bg, oldBG string) ([]byte, error) {
	root, err := parseXML(ctx, raw)
	if err != nil {
		return nil, err
	}
	tree := root.child("cSld").child("spTree")
	if tree == nil {
		return nil, fmt.Errorf("native slide has no spTree")
	}
	var header strings.Builder
	for _, n := range tree.Children {
		if n.Name.Local == "nvGrpSpPr" || n.Name.Local == "grpSpPr" {
			header.Write(raw[n.rawStart:n.rawEnd])
		}
	}
	replacement := `<p:spTree>` + header.String() + content + `</p:spTree>`
	data, err := replacePPTXNode(ctx, raw, "spTree", replacement)
	if err != nil {
		return nil, err
	}
	if bg != oldBG {
		root, err = parseXML(ctx, data)
		if err != nil {
			return nil, err
		}
		old := root.child("cSld").child("bg")
		if old != nil {
			data = append(append(append([]byte{}, data[:old.rawStart]...), []byte(pptxBackground(bg))...), data[old.rawEnd:]...)
		} else {
			cs := root.child("cSld")
			start := int(cs.rawStart) + strings.IndexByte(string(data[cs.rawStart:]), '>') + 1
			data = append(append(append([]byte{}, data[:start]...), []byte(pptxBackground(bg))...), data[start:]...)
		}
	}
	return ensurePPTXNamespaces(data), nil
}
func buildPPTXObjects(ctx context.Context, parts map[string][]byte, part string, objects, old []SlideObject, raw []byte, rels []opc.Relationship) (string, []opc.Relationship, error) {
	native := map[string][]byte{}
	oldMap := map[string]SlideObject{}
	for _, o := range old {
		oldMap[o.ID] = o
	}
	if len(raw) > 0 {
		root, err := parseXML(ctx, raw)
		if err != nil {
			return "", nil, err
		}
		tree := root.child("cSld").child("spTree")
		if tree != nil {
			for _, node := range tree.Children {
				ids := node.all("cNvPr")
				if len(ids) > 0 {
					native[ids[0].attr("id")] = raw[node.rawStart:node.rawEnd]
				}
			}
		}
	}
	var b strings.Builder
	for _, o := range objects {
		if previous, ok := oldMap[o.ID]; ok && sameSlideObject(o, previous) {
			b.Write(native[o.ID])
			continue
		}
		if previous, ok := oldMap[o.ID]; ok && native[o.ID] != nil {
			if patched, ok, err := patchNativeTextFrame(ctx, native[o.ID], o, previous); err != nil {
				return "", nil, err
			} else if ok {
				b.Write(patched)
				continue
			}
		}
		if previous, ok := oldMap[o.ID]; ok && len(previous.Unsupported) > 0 {
			return "", nil, fmt.Errorf("%w: edited object %s contains unmodeled native features: %s", ErrUnsupported, o.ID, previous.Unsupported[0])
		}
		if len(o.Unsupported) > 0 {
			return "", nil, fmt.Errorf("%w: cannot regenerate unsupported object %s", ErrUnsupported, o.ID)
		}
		xml, updated, err := pptxObjectXML(parts, part, o, rels)
		if err != nil {
			return "", nil, err
		}
		rels = updated
		b.WriteString(xml)
	}
	return b.String(), rels, nil
}
func pptxObjectXML(parts map[string][]byte, part string, o SlideObject, rels []opc.Relationship) (string, []opc.Relationship, error) {
	id, e := strconv.ParseUint(o.ID, 10, 32)
	if e != nil || id < 2 {
		return "", nil, fmt.Errorf("PPTX object ID must be numeric and at least 2")
	}
	if o.Width <= 0 || o.Height <= 0 {
		return "", nil, fmt.Errorf("%w: object %s has no resolved frame", ErrUnsupported, o.ID)
	}
	nv := fmt.Sprintf(`<p:cNvPr id="%d" name="%s" hidden="%t"/>`, id, escape(o.Name), o.Hidden)
	xf := fmt.Sprintf(`<a:xfrm rot="%d" flipH="%t" flipV="%t"><a:off x="%d" y="%d"/><a:ext cx="%d" cy="%d"/></a:xfrm>`, int64(math.Round(o.Rotation*60000)), o.FlipH, o.FlipV, int64(math.Round(o.X*pptxEMU)), int64(math.Round(o.Y*pptxEMU)), int64(math.Round(o.Width*pptxEMU)), int64(math.Round(o.Height*pptxEMU)))
	if o.Kind == "image" {
		if (o.Fill != "" && o.Fill != "none") || (o.Stroke != "" && o.Stroke != "none") {
			return "", nil, fmt.Errorf("%w: image fill/border regeneration", ErrUnsupported)
		}
		ext := ""
		switch o.Image.MediaType {
		case "image/png":
			ext = "png"
		case "image/jpeg":
			ext = "jpg"
		case "image/gif":
			ext = "gif"
		default:
			return "", nil, fmt.Errorf("%w: PPTX image media type %s", ErrUnsupported, o.Image.MediaType)
		}
		target := ""
		for i := 1; ; i++ {
			target = fmt.Sprintf("ppt/media/manifold-%d.%s", i, ext)
			if _, ok := parts[target]; !ok {
				break
			}
		}
		parts[target] = append([]byte(nil), o.Image.Data...)
		rid := nextPresentationRelID(rels)
		rels = append(rels, opc.Relationship{ID: rid, Type: opc.ImageRelType, Target: relativePart(part, target)})
		ensurePPTXContentType(parts, target, o.Image.MediaType)
		return `<p:pic><p:nvPicPr>` + strings.Replace(nv, `/>`, ` descr="`+escape(o.Image.Alt)+`"/>`, 1) + `<p:cNvPicPr/><p:nvPr/></p:nvPicPr><p:blipFill><a:blip r:embed="` + rid + `"/><a:stretch><a:fillRect/></a:stretch></p:blipFill><p:spPr>` + xf + `<a:prstGeom prst="rect"><a:avLst/></a:prstGeom></p:spPr></p:pic>`, rels, nil
	}
	if o.Kind != "shape" {
		return "", nil, fmt.Errorf("%w: regenerating %s objects", ErrUnsupported, o.Kind)
	}
	geometry := o.Geometry
	if geometry == "" {
		geometry = "rect"
	}
	switch geometry {
	case "rect", "ellipse", "roundRect", "line":
	default:
		return "", nil, fmt.Errorf("%w: native geometry %s", ErrUnsupported, geometry)
	}
	fill := `<a:noFill/>`
	if o.Fill != "" && o.Fill != "none" {
		fill = `<a:solidFill><a:srgbClr val="` + escape(strings.TrimPrefix(o.Fill, "#")) + `"/></a:solidFill>`
	}
	stroke := `<a:ln><a:noFill/></a:ln>`
	if o.Stroke != "" && o.Stroke != "none" {
		stroke = fmt.Sprintf(`<a:ln w="%d"><a:solidFill><a:srgbClr val="%s"/></a:solidFill></a:ln>`, int64(o.StrokeWidth*pptxEMU), escape(strings.TrimPrefix(o.Stroke, "#")))
	}
	var body strings.Builder
	for paragraphIndex, block := range o.Content {
		if block.Kind != Paragraph && block.Kind != Heading {
			return "", nil, fmt.Errorf("%w: PPTX text box content %s", ErrUnsupported, block.Kind)
		}
		if block.Kind == Heading || block.PageBreakBefore || len(block.Anchors) > 0 || block.List != "" {
			return "", nil, fmt.Errorf("%w: slide text headings/breaks/anchors must be represented by explicit slide objects", ErrUnsupported)
		}
		body.WriteString(`<a:p>`)
		align := ""
		if len(o.ParagraphAlign) > 0 {
			align = o.ParagraphAlign[paragraphIndex]
		}
		if align != "" || block.Direction != "" {
			body.WriteString(`<a:pPr`)
			if align != "" {
				body.WriteString(` algn="` + align + `"`)
			}
			if block.Direction != "" {
				fmt.Fprintf(&body, ` rtl="%t"`, block.Direction == "rtl")
			}
			body.WriteString(`/>`)
		}
		for _, run := range block.Runs {
			if run.Note != nil || run.Anchor != "" || run.Destination != "" {
				return "", nil, fmt.Errorf("%w: PPTX inline note/internal anchor", ErrUnsupported)
			}
			if run.ComplexScript || run.ComplexLanguage != "" || run.EastAsiaLanguage != "" {
				return "", nil, fmt.Errorf("%w: script-specific slide run metadata requires an explicit native mapping", ErrUnsupported)
			}
			if run.FontSize > 0 && (run.FontSize < 1 || run.FontSize > 4000 || math.Abs(run.FontSize*100-math.Round(run.FontSize*100)) > 1e-7) {
				return "", nil, fmt.Errorf("%w: PPTX font sizes require 1..4000pt in 0.01pt units", ErrUnsupported)
			}
			var props strings.Builder
			fmt.Fprintf(&props, `<a:rPr b="%t" i="%t"`, run.Bold, run.Italic)
			if run.FontSize > 0 {
				fmt.Fprintf(&props, ` sz="%d"`, int64(math.Round(run.FontSize*100)))
			}
			if run.Language != "" {
				fmt.Fprintf(&props, ` lang="%s"`, escape(run.Language))
			}
			if run.Underline {
				props.WriteString(` u="sng"`)
			}
			if run.Strike {
				props.WriteString(` strike="sngStrike"`)
			}
			props.WriteString(`>`)
			if run.Color != "" && run.Color != "auto" {
				props.WriteString(`<a:solidFill><a:srgbClr val="` + escape(strings.TrimPrefix(run.Color, "#")) + `"/></a:solidFill>`)
			}
			if run.FontFamily != "" {
				props.WriteString(`<a:latin typeface="` + escape(run.FontFamily) + `"/>`)
			}
			if run.Link != "" {
				rid := nextPresentationRelID(rels)
				rels = append(rels, opc.Relationship{ID: rid, Type: pptxRelBase + "hyperlink", Target: run.Link, TargetMode: "External"})
				props.WriteString(`<a:hlinkClick r:id="` + rid + `"/>`)
			}
			if run.Direction != "" {
				fmt.Fprintf(&props, `<a:rtl val="%t"/>`, run.Direction == "rtl")
			}
			props.WriteString(`</a:rPr>`)
			for i, line := range strings.Split(run.Text, "\n") {
				if i > 0 {
					body.WriteString(`<a:br/>`)
				}
				body.WriteString(`<a:r>` + props.String() + `<a:t xml:space="preserve">` + escape(line) + `</a:t></a:r>`)
			}
		}
		body.WriteString(`<a:endParaRPr/></a:p>`)
	}
	if len(o.Content) == 0 {
		body.WriteString(`<a:p/>`)
	}
	nvProps := `<p:nvPr/>`
	if o.Placeholder != "" {
		nvProps = `<p:nvPr><p:ph type="` + escape(o.Placeholder) + `"`
		if o.PlaceholderIndex != "" {
			nvProps += ` idx="` + escape(o.PlaceholderIndex) + `"`
		}
		nvProps += `/></p:nvPr>`
	}
	return `<p:sp><p:nvSpPr>` + nv + `<p:cNvSpPr txBox="1"/>` + nvProps + `</p:nvSpPr><p:spPr>` + xf + `<a:prstGeom prst="` + geometry + `"><a:avLst/></a:prstGeom>` + fill + stroke + `</p:spPr><p:txBody>` + pptxBodyProperties(o) + `<a:lstStyle/>` + body.String() + `</p:txBody></p:sp>`, rels, nil
}
func ensurePPTXContentType(parts map[string][]byte, part, ct string) {
	data := parts["[Content_Types].xml"]
	root, err := parseXML(context.Background(), data)
	if err != nil {
		return
	}
	for _, n := range root.all("Override") {
		if strings.TrimPrefix(n.attr("PartName"), "/") == part {
			return
		}
	}
	insert := `<Override xmlns="http://schemas.openxmlformats.org/package/2006/content-types" PartName="/` + escape(part) + `" ContentType="` + escape(ct) + `"/>`
	at := bytes.LastIndex(data, []byte(`</`))
	if at < 0 {
		return
	}
	parts["[Content_Types].xml"] = append(append(append([]byte{}, data[:at]...), []byte(insert)...), data[at:]...)
}

func writePPTXCore(parts map[string][]byte, m Metadata) {
	key := "docProps/core.xml"
	if root, err := parseXML(context.Background(), parts["_rels/.rels"]); err == nil {
		for _, n := range root.all("Relationship") {
			if n.attr("Type") == opc.CorePropertiesRelType {
				key = opc.ResolveTargetPart("", n.attr("Target"))
			}
		}
	}
	data := parts[key]
	if len(data) == 0 {
		data = []byte(`<?xml version="1.0"?><cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties" xmlns:dc="http://purl.org/dc/elements/1.1/"></cp:coreProperties>`)
	}
	for _, field := range []struct{ name, value string }{{"title", m.Title}, {"creator", m.Author}, {"subject", m.Subject}} {
		replacement := `<dc:` + field.name + ` xmlns:dc="http://purl.org/dc/elements/1.1/">` + escape(field.value) + `</dc:` + field.name + `>`
		root, err := parseXML(context.Background(), data)
		if err != nil {
			continue
		}
		nodes := root.all(field.name)
		if len(nodes) == 1 {
			node := nodes[0]
			data = append(append(append([]byte{}, data[:node.rawStart]...), []byte(replacement)...), data[node.rawEnd:]...)
		} else if len(nodes) == 0 {
			at := bytes.LastIndex(data, []byte(`</`))
			if at >= 0 {
				data = append(append(append([]byte{}, data[:at]...), []byte(replacement)...), data[at:]...)
			}
		}
	}
	parts[key] = data
	ensurePPTXContentType(parts, key, "application/vnd.openxmlformats-package.core-properties+xml")
}
func pptxBodyProperties(o SlideObject) string {
	v := PageMargins{}
	if o.Insets != nil {
		v = *o.Insets
	}
	wrap := "square"
	if o.NoWrap {
		wrap = "none"
	}
	anchor := o.VerticalAlign
	if anchor == "" {
		anchor = "t"
	}
	return fmt.Sprintf(`<a:bodyPr lIns="%d" tIns="%d" rIns="%d" bIns="%d" wrap="%s" anchor="%s"/>`, int64(math.Round(v.Left*pptxEMU)), int64(math.Round(v.Top*pptxEMU)), int64(math.Round(v.Right*pptxEMU)), int64(math.Round(v.Bottom*pptxEMU)), wrap, anchor)
}

func blankPPTXParts(p *Presentation, m Metadata) map[string][]byte {
	parts := map[string][]byte{"[Content_Types].xml": []byte(`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/></Types>`)}
	parts["_rels/.rels"] = pptxRelationships([]opc.Relationship{{ID: "rId1", Type: pptxRelBase + "officeDocument", Target: "ppt/presentation.xml"}, {ID: "rId2", Type: opc.CorePropertiesRelType, Target: "docProps/core.xml"}})
	parts["ppt/presentation.xml"] = []byte(fmt.Sprintf(`<p:presentation %s><p:sldMasterIdLst><p:sldMasterId id="2147483648" r:id="rIdMaster"/></p:sldMasterIdLst><p:sldIdLst></p:sldIdLst><p:sldSz cx="%d" cy="%d"/><p:notesSz cx="6858000" cy="9144000"/></p:presentation>`, pptxNS, int64(p.Width*pptxEMU), int64(p.Height*pptxEMU)))
	parts["ppt/_rels/presentation.xml.rels"] = pptxRelationships([]opc.Relationship{{ID: "rIdMaster", Type: opc.SlideMasterRelType, Target: "slideMasters/manifold-blank.xml"}})
	parts["ppt/slideMasters/manifold-blank.xml"] = []byte(`<p:sldMaster ` + pptxNS + `><p:cSld><p:spTree>` + pptxGroupRoot() + `</p:spTree></p:cSld><p:clrMap accent1="accent1" accent2="accent2" accent3="accent3" accent4="accent4" accent5="accent5" accent6="accent6" bg1="lt1" bg2="lt2" folHlink="folHlink" hlink="hlink" tx1="dk1" tx2="dk2"/><p:sldLayoutIdLst><p:sldLayoutId id="2147483649" r:id="rId1"/></p:sldLayoutIdLst><p:txStyles><p:titleStyle/><p:bodyStyle/><p:otherStyle/></p:txStyles></p:sldMaster>`)
	parts["ppt/slideMasters/_rels/manifold-blank.xml.rels"] = pptxRelationships([]opc.Relationship{{ID: "rId1", Type: opc.SlideLayoutRelType, Target: "../slideLayouts/manifold-blank.xml"}})
	parts["ppt/slideLayouts/manifold-blank.xml"] = []byte(`<p:sldLayout ` + pptxNS + ` type="blank" preserve="1"><p:cSld name="Blank"><p:spTree>` + pptxGroupRoot() + `</p:spTree></p:cSld><p:clrMapOvr><a:masterClrMapping/></p:clrMapOvr></p:sldLayout>`)
	parts["ppt/slideLayouts/_rels/manifold-blank.xml.rels"] = pptxRelationships([]opc.Relationship{{ID: "rId1", Type: opc.SlideMasterRelType, Target: "../slideMasters/manifold-blank.xml"}})
	for _, item := range []struct{ part, kind string }{{"ppt/presentation.xml", "presentation.main"}, {"ppt/slideMasters/manifold-blank.xml", "slideMaster"}, {"ppt/slideLayouts/manifold-blank.xml", "slideLayout"}} {
		ensurePPTXContentType(parts, item.part, pptxContentBase+item.kind+"+xml")
	}
	writePPTXCore(parts, m)
	return parts
}
func writePPTXNotes(ctx context.Context, parts map[string][]byte, slidePart string, notes []Block) error {
	rels := []opc.Relationship{}
	if pkg, err := opc.FromParts(ctx, parts, ""); err == nil {
		rels, _ = pkg.RelationshipsForPart(slidePart)
	}
	target := ""
	for _, rel := range rels {
		if rel.Type == opc.NotesSlideRelType {
			target = opc.ResolveTargetPart(slidePart, rel.Target)
		}
	}
	if target == "" && len(notes) == 0 {
		return nil
	}
	if target == "" {
		target = "ppt/notesSlides/notes-" + strings.TrimSuffix(path.Base(slidePart), ".xml") + ".xml"
		rels = append(rels, opc.Relationship{ID: nextPresentationRelID(rels), Type: opc.NotesSlideRelType, Target: relativePart(slidePart, target)})
	}
	obj := SlideObject{ID: "2", Kind: "shape", Name: "Notes", Placeholder: "body", Width: 600, Height: 700, Content: notes}
	body, _, err := pptxObjectXML(parts, target, obj, nil)
	if err != nil {
		return err
	}
	parts[target] = []byte(`<p:notes ` + pptxNS + `><p:cSld><p:spTree>` + pptxGroupRoot() + body + `</p:spTree></p:cSld><p:clrMapOvr><a:masterClrMapping/></p:clrMapOvr></p:notes>`)
	parts[opc.RelationshipsPartFor(slidePart)] = pptxRelationships(rels)
	parts[opc.RelationshipsPartFor(target)] = pptxRelationships([]opc.Relationship{{ID: "rId1", Type: pptxRelBase + "slide", Target: relativePart(target, slidePart)}})
	ensurePPTXContentType(parts, target, pptxContentBase+"notesSlide+xml")
	return nil
}

// Patch only source-bound text/frame fields when styles and unknown native
// properties are unchanged. This preserves native effects/theme data instead of
// regenerating an unsupported object merely to change its text.
func patchNativeTextFrame(ctx context.Context, raw []byte, current, old SlideObject) ([]byte, bool, error) {
	a, b := current, old
	a.Content = nil
	b.Content = nil
	a.X, a.Y, a.Width, a.Height, a.Rotation = 0, 0, 0, 0, 0
	b.X, b.Y, b.Width, b.Height, b.Rotation = 0, 0, 0, 0, 0
	if !reflect.DeepEqual(a, b) || len(current.Content) != len(old.Content) {
		return nil, false, nil
	}
	var oldText, newText []string
	for i, c := range current.Content {
		o := old.Content[i]
		if len(c.Runs) != len(o.Runs) {
			return nil, false, nil
		}
		cr, or := c.Runs, o.Runs
		c.Runs = nil
		o.Runs = nil
		if !reflect.DeepEqual(c, o) {
			return nil, false, nil
		}
		for j, r := range cr {
			before := or[j]
			oldText = append(oldText, before.Text)
			newText = append(newText, r.Text)
			r.Text = ""
			before.Text = ""
			if !reflect.DeepEqual(r, before) {
				return nil, false, nil
			}
		}
	}
	wrapped := append(append([]byte(`<p:sp `+pptxNS+`>`), raw...), []byte(`</p:sp>`)...)
	root, err := parseXML(ctx, wrapped)
	if err != nil {
		return nil, false, nil
	}
	if len(root.Children) != 1 {
		return nil, false, nil
	}
	object := root.Children[0]
	if len(object.all("fld")) > 0 || len(object.all("br")) > 0 {
		return nil, false, nil
	}
	texts := object.all("t")
	if len(texts) != len(oldText) {
		return nil, false, nil
	}
	prefix := int64(len(`<p:sp ` + pptxNS + `>`))
	type patch struct {
		start, end int64
		text       []byte
	}
	var patches []patch
	for i, n := range texts {
		if n.content() != oldText[i] {
			return nil, false, nil
		}
		if newText[i] != oldText[i] {
			patches = append(patches, patch{n.rawStart - prefix, n.rawEnd - prefix, []byte(`<a:t xml:space="preserve">` + escape(newText[i]) + `</a:t>`)})
		}
	}
	if current.X != old.X || current.Y != old.Y || current.Width != old.Width || current.Height != old.Height || current.Rotation != old.Rotation {
		frames := object.all("xfrm")
		if len(frames) != 1 || frames[0].child("off") == nil || frames[0].child("ext") == nil {
			return nil, false, nil
		}
		frame := frames[0]
		// Child coordinate transforms and unmodeled xfrm attributes are not rewritten.
		if len(frame.Children) != 2 {
			return nil, false, nil
		}
		for _, attr := range frame.Attr {
			if attr.Name.Local != "rot" && attr.Name.Local != "flipH" && attr.Name.Local != "flipV" {
				return nil, false, nil
			}
		}
		text := fmt.Sprintf(`<a:xfrm rot="%d" flipH="%t" flipV="%t"><a:off x="%d" y="%d"/><a:ext cx="%d" cy="%d"/></a:xfrm>`, int64(math.Round(current.Rotation*60000)), current.FlipH, current.FlipV, int64(math.Round(current.X*pptxEMU)), int64(math.Round(current.Y*pptxEMU)), int64(math.Round(current.Width*pptxEMU)), int64(math.Round(current.Height*pptxEMU)))
		if object.Name.Local == "graphicFrame" {
			text = strings.ReplaceAll(text, "a:xfrm", "p:xfrm")
		}
		patches = append(patches, patch{frame.rawStart - prefix, frame.rawEnd - prefix, []byte(text)})
	}
	sort.Slice(patches, func(i, j int) bool { return patches[i].start > patches[j].start })
	result := append([]byte(nil), raw...)
	for _, p := range patches {
		if p.start < 0 || p.end > int64(len(result)) {
			return nil, false, fmt.Errorf("invalid source-bound XML range")
		}
		result = append(append(append([]byte{}, result[:p.start]...), p.text...), result[p.end:]...)
	}
	return result, true, nil
}

func patchSlideVisibility(ctx context.Context, data []byte, hidden bool) ([]byte, error) {
	root, err := parseXML(ctx, data)
	if err != nil {
		return nil, err
	}
	end := bytes.IndexByte(data[root.rawStart:], '>') + int(root.rawStart)
	if end < 0 {
		return nil, fmt.Errorf("invalid slide start")
	}
	opening := string(data[root.rawStart : int64(end)+1])
	// Replace only the declared visibility attribute in the start tag.
	for _, quote := range []string{`"`, `'`} {
		needle := `show=` + quote
		if at := strings.Index(opening, needle); at >= 0 {
			valueAt := at + len(needle)
			valueEnd := strings.Index(opening[valueAt:], quote)
			if valueEnd < 0 {
				return nil, fmt.Errorf("invalid visibility attribute")
			}
			opening = opening[:valueAt] + strconv.FormatBool(!hidden) + opening[valueAt+valueEnd:]
			return append(append(append([]byte{}, data[:root.rawStart]...), []byte(opening)...), data[end+1:]...), nil
		}
	}
	opening = opening[:len(opening)-1] + ` show="` + strconv.FormatBool(!hidden) + `">`
	return append(append(append([]byte{}, data[:root.rawStart]...), []byte(opening)...), data[end+1:]...), nil
}
