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
	presentationPart := pkg.PresentationPath
	var slideRefs strings.Builder
	var slideRels []opc.Relationship
	for _, rel := range pkg.PresentationRelationships {
		if rel.Type != pptxRelBase+"slide" {
			slideRels = append(slideRels, rel)
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
		relID := nextPresentationRelID(slideRels)
		slideRels = append(slideRels, opc.Relationship{ID: relID, Type: pptxRelBase + "slide", Target: relativePart(presentationPart, part)})
		nativeID, parseErr := strconv.ParseUint(s.ID, 10, 32)
		if parseErr != nil || nativeID < 256 {
			nativeID = uint64(256 + i)
		}
		fmt.Fprintf(&slideRefs, `<p:sldId id="%d" r:id="%s"/>`, nativeID, escape(relID))
		old, hasOld := oldSlides[s.Part]
		if !hasOld || !reflect.DeepEqual(s.Objects, old.Objects) || s.Background != old.Background {
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
			} else {
				parts[part] = []byte(`<?xml version="1.0" encoding="UTF-8"?><p:sld ` + pptxNS + `><p:cSld>` + pptxBackground(s.Background) + `<p:spTree>` + pptxGroupRoot() + content + `</p:spTree></p:cSld><p:clrMapOvr><a:masterClrMapping/></p:clrMapOvr></p:sld>`)
			}
			if e != nil {
				return e
			}
			parts[opc.RelationshipsPartFor(part)] = pptxRelationships(updated)
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
		parts[opc.RelationshipsPartFor(resource.Part)] = pptxRelationships(updated)
	}
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
	if original == nil || d.Metadata.Title != original.Metadata.Title || d.Metadata.Author != original.Metadata.Author || d.Metadata.Subject != original.Metadata.Subject {
		writePPTXCore(parts, d.Metadata)
	}
	// Validate final structure before emitting any caller-visible bytes.
	if _, err := opc.FromParts(ctx, parts, ""); err != nil {
		return err
	}
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
	_, err = io.Copy(contextWriter{ctx, out}, &staged)
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
	nv := fmt.Sprintf(`<p:cNvPr id="%d" name="%s"/>`, id, escape(o.Name))
	xf := fmt.Sprintf(`<a:xfrm rot="%d" flipH="%t" flipV="%t"><a:off x="%d" y="%d"/><a:ext cx="%d" cy="%d"/></a:xfrm>`, int64(math.Round(o.Rotation*60000)), o.FlipH, o.FlipV, int64(math.Round(o.X*pptxEMU)), int64(math.Round(o.Y*pptxEMU)), int64(math.Round(o.Width*pptxEMU)), int64(math.Round(o.Height*pptxEMU)))
	if o.Kind == "image" {
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
	for _, block := range o.Content {
		if block.Kind != Paragraph && block.Kind != Heading {
			return "", nil, fmt.Errorf("%w: PPTX text box content %s", ErrUnsupported, block.Kind)
		}
		body.WriteString(`<a:p>`)
		for _, run := range block.Runs {
			if run.Note != nil || run.Anchor != "" || run.Destination != "" {
				return "", nil, fmt.Errorf("%w: PPTX inline note/internal anchor", ErrUnsupported)
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
			if run.FontFamily != "" {
				props.WriteString(`<a:latin typeface="` + escape(run.FontFamily) + `"/>`)
			}
			if run.Color != "" && run.Color != "auto" {
				props.WriteString(`<a:solidFill><a:srgbClr val="` + escape(strings.TrimPrefix(run.Color, "#")) + `"/></a:solidFill>`)
			}
			if run.Link != "" {
				rid := nextPresentationRelID(rels)
				rels = append(rels, opc.Relationship{ID: rid, Type: pptxRelBase + "hyperlink", Target: run.Link, TargetMode: "External"})
				props.WriteString(`<a:hlinkClick r:id="` + rid + `"/>`)
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
	return `<p:sp><p:nvSpPr>` + nv + `<p:cNvSpPr txBox="1"/><p:nvPr/></p:nvSpPr><p:spPr>` + xf + `<a:prstGeom prst="` + geometry + `"><a:avLst/></a:prstGeom>` + fill + stroke + `</p:spPr><p:txBody><a:bodyPr lIns="0" tIns="0" rIns="0" bIns="0"/><a:lstStyle/>` + body.String() + `</p:txBody></p:sp>`, rels, nil
}
func ensurePPTXContentType(parts map[string][]byte, part, ct string) {
	data := parts["[Content_Types].xml"]
	needle := `PartName="/` + part + `"`
	if bytes.Contains(data, []byte(needle)) {
		return
	}
	insert := `<Override PartName="/` + escape(part) + `" ContentType="` + escape(ct) + `"/>`
	parts["[Content_Types].xml"] = bytes.Replace(data, []byte(`</Types>`), []byte(insert+`</Types>`), 1)
}
func writePPTXCore(parts map[string][]byte, m Metadata) {
	parts["docProps/core.xml"] = []byte(`<?xml version="1.0"?><cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties" xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:title>` + escape(m.Title) + `</dc:title><dc:creator>` + escape(m.Author) + `</dc:creator><dc:subject>` + escape(m.Subject) + `</dc:subject></cp:coreProperties>`)
	ensurePPTXContentType(parts, "docProps/core.xml", "application/vnd.openxmlformats-package.core-properties+xml")
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
	for _, item := range []struct{ part, kind string }{{"ppt/presentation.xml", "presentation"}, {"ppt/slideMasters/manifold-blank.xml", "slideMaster"}, {"ppt/slideLayouts/manifold-blank.xml", "slideLayout"}} {
		ensurePPTXContentType(parts, item.part, pptxContentBase+item.kind+"+xml")
	}
	writePPTXCore(parts, m)
	return parts
}
func writePPTXNotes(ctx context.Context, parts map[string][]byte, slidePart string, notes []Block) error {
	if len(notes) == 0 {
		return nil
	}
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
	if target == "" {
		target = "ppt/notesSlides/notes-" + strings.TrimSuffix(path.Base(slidePart), ".xml") + ".xml"
		rels = append(rels, opc.Relationship{ID: nextPresentationRelID(rels), Type: opc.NotesSlideRelType, Target: relativePart(slidePart, target)})
	}
	obj := SlideObject{ID: "2", Kind: "shape", Name: "Notes", Width: 600, Height: 700, Content: notes}
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
