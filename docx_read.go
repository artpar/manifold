package manydoc

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"path"
	"strconv"
	"strings"
)

const wordNS = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"
const strictWordNS = "http://purl.oclc.org/ooxml/wordprocessingml/main"
const relNS = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"

type xmlNode struct {
	Name     xml.Name
	Attr     []xml.Attr
	Text     string
	Children []*xmlNode
}

func parseXML(ctx context.Context, b []byte) (*xmlNode, error) {
	dec := xml.NewDecoder(bytes.NewReader(b))
	var root *xmlNode
	stack := []*xmlNode{}
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		t, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch v := t.(type) {
		case xml.StartElement:
			if len(stack) > 256 {
				return nil, errors.New("XML nesting exceeds 256 levels")
			}
			n := &xmlNode{Name: v.Name, Attr: v.Attr}
			if len(stack) == 0 {
				if root != nil {
					return nil, errors.New("multiple XML roots")
				}
				root = n
			} else {
				p := stack[len(stack)-1]
				p.Children = append(p.Children, n)
			}
			stack = append(stack, n)
		case xml.CharData:
			if len(stack) > 0 {
				stack[len(stack)-1].Text += string(v)
			} else if strings.TrimSpace(string(v)) != "" {
				return nil, errors.New("text outside XML root")
			}
		case xml.EndElement:
			if len(stack) == 0 {
				return nil, errors.New("unexpected XML closing element")
			}
			stack = stack[:len(stack)-1]
		case xml.Directive:
			return nil, errors.New("XML directives are not supported")
		}
	}
	if root == nil || len(stack) > 0 {
		return nil, errors.New("missing or incomplete XML root")
	}
	return root, nil
}
func (n *xmlNode) attr(k string) string {
	if n == nil {
		return ""
	}
	for _, a := range n.Attr {
		if a.Name.Local == k {
			return a.Value
		}
	}
	return ""
}
func (n *xmlNode) child(k string) *xmlNode {
	if n == nil {
		return nil
	}
	for _, c := range n.Children {
		if c.Name.Local == k {
			return c
		}
	}
	return nil
}
func (n *xmlNode) all(k string) []*xmlNode {
	var out []*xmlNode
	if n == nil {
		return out
	}
	if n.Name.Local == k {
		out = append(out, n)
	}
	for _, c := range n.Children {
		out = append(out, c.all(k)...)
	}
	return out
}
func (n *xmlNode) content() string {
	if n == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString(n.Text)
	for _, c := range n.Children {
		b.WriteString(c.content())
	}
	return b.String()
}

type docxRelation struct {
	Target, Type string
	External     bool
}
type docxReader struct {
	ctx    context.Context
	files  map[string]*zip.File
	doc    *Document
	styles map[string]*xmlNode
	num    map[string]string
	types  map[string]string
}

func (r *docxReader) part(name string) ([]byte, error) {
	f := r.files[name]
	if f == nil {
		return nil, fmt.Errorf("missing DOCX part %q", name)
	}
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return readContext(r.ctx, rc)
}
func (r *docxReader) xml(name string) (*xmlNode, error) {
	b, err := r.part(name)
	if err != nil {
		return nil, err
	}
	n, err := parseXML(r.ctx, b)
	if err != nil {
		return nil, fmt.Errorf("DOCX %s: %w", name, err)
	}
	return n, nil
}
func (r *docxReader) rels(part string) (map[string]docxRelation, error) {
	name := "_rels/.rels"
	if part != "" {
		name = path.Join(path.Dir(part), "_rels", path.Base(part)+".rels")
	}
	out := map[string]docxRelation{}
	if r.files[name] == nil {
		return out, nil
	}
	n, err := r.xml(name)
	if err != nil {
		return nil, err
	}
	if n.Name.Local != "Relationships" {
		return nil, errors.New("invalid DOCX relationships root")
	}
	for _, c := range n.Children {
		if c.Name.Local != "Relationship" {
			continue
		}
		id, target := c.attr("Id"), c.attr("Target")
		if id == "" || target == "" {
			return nil, errors.New("invalid DOCX relationship")
		}
		if _, ok := out[id]; ok {
			return nil, errors.New("duplicate DOCX relationship ID")
		}
		external := c.attr("TargetMode") == "External"
		if !external {
			if strings.HasPrefix(target, "/") {
				target = strings.TrimPrefix(target, "/")
			} else {
				target = path.Join(path.Dir(part), target)
			}
			if target == ".." || strings.HasPrefix(target, "../") {
				return nil, errors.New("DOCX relationship escapes package")
			}
		}
		out[id] = docxRelation{target, c.attr("Type"), external}
	}
	return out, nil
}

// ReadDOCX reads the final visible view: insertions are included and deletions
// excluded. Revision history and unsupported presentation are reported explicitly.
// No relationships cause network or filesystem access.
func ReadDOCX(ctx context.Context, src io.Reader) (*Document, error) {
	b, err := readContext(ctx, src)
	if err != nil {
		return nil, err
	}
	z, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		return nil, fmt.Errorf("DOCX archive: %w", err)
	}
	r := &docxReader{ctx: ctx, files: map[string]*zip.File{}, doc: &Document{}, styles: map[string]*xmlNode{}, num: map[string]string{}, types: map[string]string{}}
	for _, f := range z.File {
		if f.FileInfo().IsDir() {
			continue
		}
		if strings.Contains(f.Name, "\\") || path.Clean(f.Name) != f.Name || strings.HasPrefix(f.Name, "/") || strings.HasPrefix(f.Name, "../") {
			return nil, errors.New("invalid DOCX archive path")
		}
		if r.files[f.Name] != nil {
			return nil, errors.New("duplicate DOCX archive member")
		}
		r.files[f.Name] = f
	}
	ct, err := r.xml("[Content_Types].xml")
	if err != nil {
		return nil, err
	}
	if ct.Name.Local != "Types" {
		return nil, errors.New("invalid content types root")
	}
	for _, v := range ct.Children {
		if v.Name.Local == "Default" {
			r.types[strings.ToLower(v.attr("Extension"))] = v.attr("ContentType")
		}
	}
	rels, err := r.rels("")
	if err != nil {
		return nil, err
	}
	main := ""
	for _, v := range rels {
		if strings.HasSuffix(v.Type, "/officeDocument") {
			if main != "" {
				return nil, errors.New("multiple DOCX main document relationships")
			}
			if v.External {
				return nil, errors.New("external DOCX main document")
			}
			main = v.Target
		}
		if strings.HasSuffix(v.Type, "/metadata/core-properties") && !v.External {
			n, e := r.xml(v.Target)
			if e != nil {
				return nil, e
			}
			r.doc.Metadata.Title = n.child("title").content()
			r.doc.Metadata.Author = n.child("creator").content()
			r.doc.Metadata.Subject = n.child("subject").content()
		}
	}
	if main == "" {
		return nil, errors.New("DOCX main document relationship missing")
	}
	n, err := r.xml(main)
	if err != nil {
		return nil, err
	}
	if n.Name.Local != "document" || (n.Name.Space != wordNS && n.Name.Space != strictWordNS) || n.child("body") == nil {
		return nil, errors.New("invalid WordprocessingML document root or body")
	}
	mainRels, err := r.rels(main)
	if err != nil {
		return nil, err
	}
	for _, v := range mainRels {
		if v.External {
			continue
		}
		switch {
		case strings.HasSuffix(v.Type, "/styles"):
			s, e := r.xml(v.Target)
			if e != nil {
				return nil, e
			}
			for _, v := range s.Children {
				if v.Name.Local == "style" {
					r.styles[v.attr("styleId")] = v
				}
			}
		case strings.HasSuffix(v.Type, "/numbering"):
			s, e := r.xml(v.Target)
			if e != nil {
				return nil, e
			}
			abstract := map[string]string{}
			for _, a := range s.Children {
				if a.Name.Local == "abstractNum" {
					kind := "number"
					for _, f := range a.all("numFmt") {
						if f.attr("val") == "bullet" {
							kind = "bullet"
						}
					}
					abstract[a.attr("abstractNumId")] = kind
				}
			}
			for _, a := range s.Children {
				if a.Name.Local == "num" {
					r.num[a.attr("numId")] = abstract[a.child("abstractNumId").attr("val")]
				}
			}
		}
	}
	r.doc.Blocks, err = r.blocks(n.child("body"), mainRels, 0)
	if err != nil {
		return nil, err
	}
	for _, s := range n.all("sectPr") {
		if p := s.child("pgSz"); p != nil {
			r.doc.Layout.Width = number(p.attr("w")) / 20
			r.doc.Layout.Height = number(p.attr("h")) / 20
		}
		if m := s.child("pgMar"); m != nil {
			r.doc.Layout.Margin = number(m.attr("left")) / 20
			if m.attr("left") != m.attr("right") || m.attr("top") != m.attr("bottom") || m.attr("left") != m.attr("top") {
				r.doc.warn("asymmetric_margins", "Asymmetric page margins are represented by the left margin")
			}
		}
		for _, c := range s.Children {
			if c.Name.Local != "headerReference" && c.Name.Local != "footerReference" {
				continue
			}
			rel, ok := mainRels[c.attr("id")]
			if !ok || rel.External {
				return nil, errors.New("missing header/footer relationship")
			}
			part, e := r.xml(rel.Target)
			if e != nil {
				return nil, e
			}
			pr, e := r.rels(rel.Target)
			if e != nil {
				return nil, e
			}
			blocks, e := r.blocks(part, pr, 0)
			if e != nil {
				return nil, e
			}
			if c.Name.Local == "headerReference" {
				r.doc.Headers = append(r.doc.Headers, blocks...)
			} else {
				r.doc.Footers = append(r.doc.Footers, blocks...)
			}
		}
	}
	if len(n.all("sectPr")) > 1 {
		r.doc.warn("multiple_sections", "Multiple sections are flattened to one page layout")
	}
	for _, v := range mainRels {
		if v.External {
			continue
		}
		kind := ""
		if strings.HasSuffix(v.Type, "/footnotes") {
			kind = "footnote"
		}
		if strings.HasSuffix(v.Type, "/endnotes") {
			kind = "endnote"
		}
		if kind == "" {
			continue
		}
		notes, e := r.xml(v.Target)
		if e != nil {
			return nil, e
		}
		nr, e := r.rels(v.Target)
		if e != nil {
			return nil, e
		}
		for _, nn := range notes.Children {
			if nn.Name.Local != kind || strings.HasPrefix(nn.attr("id"), "-") || nn.attr("type") != "" {
				continue
			}
			bs, e := r.blocks(nn, nr, 0)
			if e != nil {
				return nil, e
			}
			r.doc.Notes = append(r.doc.Notes, Note{nn.attr("id"), kind, bs})
		}
	}
	return r.doc, r.doc.Validate()
}
func number(s string) float64 { v, _ := strconv.ParseFloat(s, 64); return v }
func (r *docxReader) blocks(n *xmlNode, rels map[string]docxRelation, depth int) ([]Block, error) {
	if depth > 64 {
		return nil, errors.New("DOCX block nesting exceeds 64 levels")
	}
	var out []Block
	for _, c := range n.Children {
		if err := r.ctx.Err(); err != nil {
			return nil, err
		}
		switch c.Name.Local {
		case "p":
			bs, err := r.paragraph(c, rels)
			if err != nil {
				return nil, err
			}
			out = append(out, bs...)
		case "tbl":
			b := Block{Kind: Table}
			for _, row := range c.Children {
				if row.Name.Local != "tr" {
					continue
				}
				rr := Row{}
				for _, cell := range row.Children {
					if cell.Name.Local != "tc" {
						continue
					}
					bs, err := r.blocks(cell, rels, depth+1)
					if err != nil {
						return nil, err
					}
					span := int(number(cell.child("tcPr").child("gridSpan").attr("val")))
					if cell.child("tcPr").child("vMerge") != nil {
						r.doc.warn("vertical_merge", "Vertical table merge is not represented")
					}
					rr.Cells = append(rr.Cells, Cell{bs, span})
				}
				b.Rows = append(b.Rows, rr)
			}
			out = append(out, b)
		case "sdt":
			r.doc.warn("content_control", "Content control content is retained; control properties are not represented")
			if content := c.child("sdtContent"); content != nil {
				bs, err := r.blocks(content, rels, depth+1)
				if err != nil {
					return nil, err
				}
				out = append(out, bs...)
			}
		case "ins", "moveTo":
			r.doc.warn("revision_history", "Final visible revision content is retained; revision history is not represented")
			bs, err := r.blocks(c, rels, depth+1)
			if err != nil {
				return nil, err
			}
			out = append(out, bs...)
		case "del", "moveFrom":
			r.doc.warn("revision_history", "Final visible revision content is retained; revision history is not represented")
		case "sectPr", "tcPr":
		default:
			r.doc.warn("unsupported_block", "Unsupported Word block: "+c.Name.Local)
		}
	}
	return out, nil
}
func (r *docxReader) paragraph(n *xmlNode, rels map[string]docxRelation) ([]Block, error) {
	b := Block{Kind: Paragraph}
	ppr := n.child("pPr")
	if ppr != nil {
		for _, p := range ppr.Children {
			switch p.Name.Local {
			case "pStyle", "numPr", "outlineLvl", "sectPr", "keepNext":
			default:
				r.doc.warn("paragraph_formatting", "Paragraph property not represented: "+p.Name.Local)
			}
		}
	}
	styleID := ppr.child("pStyle").attr("val")
	if strings.HasPrefix(strings.ToLower(styleID), "heading") {
		level, _ := strconv.Atoi(strings.TrimPrefix(strings.ToLower(styleID), "heading"))
		if level >= 1 && level <= 6 {
			b.Kind = Heading
			b.Level = level
		}
	}
	if level := ppr.child("outlineLvl"); level != nil {
		v := int(number(level.attr("val"))) + 1
		if v >= 1 && v <= 6 {
			b.Kind = Heading
			b.Level = v
		}
	}
	if ppr.child("numPr") != nil {
		b.List = r.num[ppr.child("numPr").child("numId").attr("val")]
		if b.List == "" {
			b.List = "number"
		}
	}
	if strings.Contains(strings.ToLower(styleID), "listbullet") {
		b.List = "bullet"
	}
	if strings.Contains(strings.ToLower(styleID), "listnumber") {
		b.List = "number"
	}
	out := []Block{}
	base := Run{}
	seen := map[string]bool{}
	var applyStyle func(string)
	applyStyle = func(id string) {
		if id == "" || seen[id] {
			return
		}
		seen[id] = true
		s := r.styles[id]
		if s == nil {
			return
		}
		applyStyle(s.child("basedOn").attr("val"))
		applyRunProps(&base, s.child("rPr"))
	}
	applyStyle(styleID)
	flush := func() { out = append(out, b); b.Runs = nil }
	var walk func(*xmlNode, Run) error
	walk = func(c *xmlNode, style Run) error {
		switch c.Name.Local {
		case "pPr", "rPr":
			return nil
		case "del", "moveFrom":
			r.doc.warn("revision_history", "Final visible revision content is retained; revision history is not represented")
			return nil
		case "ins", "moveTo":
			r.doc.warn("revision_history", "Final visible revision content is retained; revision history is not represented")
		case "r":
			if props := c.child("rPr"); props != nil {
				for _, p := range props.Children {
					switch p.Name.Local {
					case "b", "i", "u", "strike":
					default:
						r.doc.warn("run_formatting", "Run property not represented: "+p.Name.Local)
					}
				}
			}
			applyRunProps(&style, c.child("rPr"))
		case "hyperlink":
			if id := c.attr("id"); id != "" {
				rel, ok := rels[id]
				if !ok {
					return errors.New("missing hyperlink relationship")
				}
				style.Link = rel.Target
			} else if a := c.attr("anchor"); a != "" {
				style.Link = "#" + a
			}
		case "t":
			style.Text = c.content()
			b.Runs = append(b.Runs, style)
			return nil
		case "tab":
			style.Text = "\t"
			b.Runs = append(b.Runs, style)
			return nil
		case "br", "cr":
			if c.attr("type") == "page" {
				if len(b.Runs) > 0 {
					flush()
				}
				out = append(out, Block{Kind: PageBreak})
			} else {
				style.Text = "\n"
				b.Runs = append(b.Runs, style)
			}
			return nil
		case "drawing":
			blips := c.all("blip")
			for _, blip := range blips {
				id := blip.attr("embed")
				if id == "" {
					r.doc.warn("external_image", "External images are not fetched")
					continue
				}
				rel, ok := rels[id]
				if !ok || rel.External {
					return errors.New("missing embedded image relationship")
				}
				data, err := r.part(rel.Target)
				if err != nil {
					return err
				}
				media := r.types[strings.TrimPrefix(strings.ToLower(path.Ext(rel.Target)), ".")]
				im := &Image{Data: data, MediaType: media}
				if ext := c.all("extent"); len(ext) > 0 {
					im.Width = number(ext[0].attr("cx")) / 12700
					im.Height = number(ext[0].attr("cy")) / 12700
				}
				if props := c.all("docPr"); len(props) > 0 {
					im.Alt = props[0].attr("descr")
				}
				if len(b.Runs) > 0 {
					flush()
				}
				out = append(out, Block{Kind: Picture, Image: im})
			}
			if len(c.all("anchor")) > 0 {
				r.doc.warn("floating_image", "Floating image is represented inline")
			}
			return nil
		case "txbxContent":
			r.doc.warn("text_box_layout", "Text box text is retained without floating layout")
			bs, err := r.blocks(c, rels, 0)
			if err != nil {
				return err
			}
			if len(b.Runs) > 0 {
				flush()
			}
			out = append(out, bs...)
			return nil
		case "fldSimple":
			r.doc.warn("field", "Cached field text is retained; field evaluation is not supported")
		case "instrText", "fldChar":
			r.doc.warn("field", "Cached field text is retained; field evaluation is not supported")
			return nil
		case "sdt":
			r.doc.warn("content_control", "Content control content is retained; control properties are not represented")
		case "bookmarkStart", "bookmarkEnd":
			r.doc.warn("bookmarks", "Bookmark text is retained; bookmark destinations are not represented")
			return nil
		case "footnoteReference", "endnoteReference":
			style.Text = "[" + c.attr("id") + "]"
			b.Runs = append(b.Runs, style)
			r.doc.warn("note_reference", "Note text is retained separately; inline reference semantics are not represented")
			return nil
		case "commentReference", "commentRangeStart", "commentRangeEnd":
			r.doc.warn("comments", "Comment annotations are not represented")
			return nil
		case "noBreakHyphen":
			style.Text = "\u2011"
			b.Runs = append(b.Runs, style)
			return nil
		case "softHyphen":
			style.Text = "\u00ad"
			b.Runs = append(b.Runs, style)
			return nil
		}
		for _, cc := range c.Children {
			if err := walk(cc, style); err != nil {
				return err
			}
		}
		return nil
	}
	for _, c := range n.Children {
		if err := walk(c, base); err != nil {
			return nil, err
		}
	}
	if len(b.Runs) > 0 || len(out) == 0 {
		flush()
	}
	return out, nil
}
func applyRunProps(r *Run, n *xmlNode) {
	if n == nil {
		return
	}
	for _, p := range n.Children {
		v := p.attr("val")
		enabled := v != "0" && v != "false" && v != "off" && v != "none"
		switch p.Name.Local {
		case "b":
			r.Bold = enabled
		case "i":
			r.Italic = enabled
		case "u":
			r.Underline = enabled
		case "strike":
			r.Strike = enabled
		}
	}
}
