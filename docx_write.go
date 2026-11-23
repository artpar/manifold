package manydoc

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"sort"
	"strconv"
	"strings"
)

func escape(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

const xmlHeader = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`
const wordNamespaces = ` xmlns:w="` + wordNS + `" xmlns:r="` + relNS + `" xmlns:wp="http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:pic="http://schemas.openxmlformats.org/drawingml/2006/picture"`

type docxWriter struct {
	ctx    context.Context
	parts  map[string][]byte
	rels   []string
	next   int
	images int
}

func (w *docxWriter) addRel(kind, target string, external bool) string {
	id := "rId" + strconv.Itoa(w.next)
	w.next++
	mode := ""
	if external {
		mode = ` TargetMode="External"`
	}
	w.rels = append(w.rels, `<Relationship Id="`+id+`" Type="`+relNS+`/`+kind+`" Target="`+escape(target)+`"`+mode+`/>`)
	return id
}

func writeDOCX(ctx context.Context, out io.Writer, d *Document) error {
	if len(d.Notes) > 0 {
		return fmt.Errorf("%w: DOCX writing note references is not implemented", ErrUnsupported)
	}
	w := &docxWriter{ctx: ctx, parts: map[string][]byte{}, next: 1}
	w.addRel("styles", "styles.xml", false)
	w.addRel("numbering", "numbering.xml", false)
	body, err := w.blocks(d.Blocks)
	if err != nil {
		return err
	}
	wid, hei, mar := layout(d.Layout)
	section := `<w:sectPr>`
	for _, v := range []struct {
		name, root, ref string
		blocks          []Block
	}{{"header1.xml", "hdr", "headerReference", d.Headers}, {"footer1.xml", "ftr", "footerReference", d.Footers}} {
		if len(v.blocks) == 0 {
			continue
		}
		mainRels := w.rels
		w.rels = nil
		part, err := w.blocks(v.blocks)
		if err != nil {
			return err
		}
		if len(w.rels) > 0 {
			w.parts["word/_rels/"+v.name+".rels"] = []byte(xmlHeader + `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` + strings.Join(w.rels, "") + `</Relationships>`)
		}
		w.rels = mainRels
		id := w.addRel(strings.TrimSuffix(v.name, "1.xml"), v.name, false)
		w.parts["word/"+v.name] = []byte(xmlHeader + `<w:` + v.root + wordNamespaces + `>` + part + `</w:` + v.root + `>`)
		section += `<w:` + v.ref + ` w:type="default" r:id="` + id + `"/>`
	}
	section += fmt.Sprintf(`<w:pgSz w:w="%.0f" w:h="%.0f"/><w:pgMar w:top="%.0f" w:right="%.0f" w:bottom="%.0f" w:left="%.0f" w:header="360" w:footer="360"/></w:sectPr>`, wid*20, hei*20, mar*20, mar*20, mar*20, mar*20)
	w.parts["word/document.xml"] = []byte(xmlHeader + `<w:document` + wordNamespaces + `><w:body>` + body + section + `</w:body></w:document>`)
	w.parts["word/styles.xml"] = []byte(xmlHeader + `<w:styles xmlns:w="` + wordNS + `"><w:docDefaults><w:rPrDefault><w:rPr><w:rFonts w:ascii="Noto Sans" w:hAnsi="Noto Sans"/><w:sz w:val="22"/></w:rPr></w:rPrDefault></w:docDefaults><w:style w:type="paragraph" w:default="1" w:styleId="Normal"><w:name w:val="Normal"/></w:style>` + headingStyles() + `</w:styles>`)
	w.parts["word/numbering.xml"] = []byte(xmlHeader + `<w:numbering xmlns:w="` + wordNS + `"><w:abstractNum w:abstractNumId="0"><w:multiLevelType w:val="singleLevel"/><w:lvl w:ilvl="0"><w:start w:val="1"/><w:numFmt w:val="bullet"/><w:lvlText w:val="•"/><w:pPr><w:ind w:left="360" w:hanging="180"/></w:pPr></w:lvl></w:abstractNum><w:abstractNum w:abstractNumId="1"><w:multiLevelType w:val="singleLevel"/><w:lvl w:ilvl="0"><w:start w:val="1"/><w:numFmt w:val="decimal"/><w:lvlText w:val="%1."/><w:pPr><w:ind w:left="360" w:hanging="180"/></w:pPr></w:lvl></w:abstractNum><w:num w:numId="1"><w:abstractNumId w:val="0"/></w:num><w:num w:numId="2"><w:abstractNumId w:val="1"/></w:num></w:numbering>`)
	relsHead := xmlHeader + `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`
	w.parts["word/_rels/document.xml.rels"] = []byte(relsHead + strings.Join(w.rels, "") + `</Relationships>`)
	w.parts["_rels/.rels"] = []byte(relsHead + `<Relationship Id="rId1" Type="` + relNS + `/officeDocument" Target="word/document.xml"/><Relationship Id="rId2" Type="http://schemas.openxmlformats.org/package/2006/relationships/metadata/core-properties" Target="docProps/core.xml"/></Relationships>`)
	w.parts["docProps/core.xml"] = []byte(xmlHeader + `<cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties" xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:title>` + escape(d.Metadata.Title) + `</dc:title><dc:creator>` + escape(d.Metadata.Author) + `</dc:creator><dc:subject>` + escape(d.Metadata.Subject) + `</dc:subject></cp:coreProperties>`)
	ct := xmlHeader + `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Default Extension="png" ContentType="image/png"/><Default Extension="jpg" ContentType="image/jpeg"/><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/><Override PartName="/word/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.styles+xml"/><Override PartName="/word/numbering.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.numbering+xml"/><Override PartName="/docProps/core.xml" ContentType="application/vnd.openxmlformats-package.core-properties+xml"/>`
	for _, v := range []struct{ name, kind string }{{"header1.xml", "header"}, {"footer1.xml", "footer"}} {
		if w.parts["word/"+v.name] != nil {
			ct += `<Override PartName="/word/` + v.name + `" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.` + v.kind + `+xml"/>`
		}
	}
	w.parts["[Content_Types].xml"] = []byte(ct + `</Types>`)
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	names := make([]string, 0, len(w.parts))
	for n := range w.parts {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		if err := ctx.Err(); err != nil {
			return err
		}
		f, err := z.Create(n)
		if err != nil {
			return err
		}
		if _, err = f.Write(w.parts[n]); err != nil {
			return err
		}
	}
	if err := z.Close(); err != nil {
		return err
	}
	_, err = io.Copy(out, &buf)
	return err
}
func headingStyles() string {
	var b strings.Builder
	for i := 1; i <= 6; i++ {
		fmt.Fprintf(&b, `<w:style w:type="paragraph" w:styleId="Heading%d"><w:name w:val="heading %d"/><w:basedOn w:val="Normal"/><w:pPr><w:keepNext/><w:outlineLvl w:val="%d"/></w:pPr><w:rPr><w:b/><w:sz w:val="%d"/></w:rPr></w:style>`, i, i, i-1, 44-i*3)
	}
	return b.String()
}
func (w *docxWriter) blocks(blocks []Block) (string, error) {
	var out strings.Builder
	for _, b := range blocks {
		if err := w.ctx.Err(); err != nil {
			return "", err
		}
		switch b.Kind {
		case PageBreak:
			out.WriteString(`<w:p><w:r><w:br w:type="page"/></w:r></w:p>`)
		case Paragraph, Heading:
			out.WriteString(`<w:p><w:pPr>`)
			if b.Kind == Heading {
				fmt.Fprintf(&out, `<w:pStyle w:val="Heading%d"/>`, b.Level)
			}
			if b.List != "" {
				id := 1
				if b.List == "number" {
					id = 2
				}
				fmt.Fprintf(&out, `<w:numPr><w:ilvl w:val="0"/><w:numId w:val="%d"/></w:numPr>`, id)
			}
			out.WriteString(`</w:pPr>`)
			for _, r := range b.Runs {
				if r.Link != "" {
					if strings.HasPrefix(r.Link, "#") {
						out.WriteString(`<w:hyperlink w:anchor="` + escape(strings.TrimPrefix(r.Link, "#")) + `">`)
					} else {
						id := w.addRel("hyperlink", r.Link, true)
						out.WriteString(`<w:hyperlink r:id="` + id + `">`)
					}
				}
				out.WriteString(`<w:r><w:rPr>`)
				if r.Bold {
					out.WriteString(`<w:b/>`)
				}
				if r.Italic {
					out.WriteString(`<w:i/>`)
				}
				if r.Underline {
					out.WriteString(`<w:u w:val="single"/>`)
				}
				if r.Strike {
					out.WriteString(`<w:strike/>`)
				}
				out.WriteString(`</w:rPr>`)
				var text strings.Builder
				flush := func() {
					if text.Len() > 0 {
						out.WriteString(`<w:t xml:space="preserve">` + escape(text.String()) + `</w:t>`)
						text.Reset()
					}
				}
				for _, ch := range r.Text {
					switch ch {
					case '\t':
						flush()
						out.WriteString(`<w:tab/>`)
					case '\n':
						flush()
						out.WriteString(`<w:br/>`)
					case '\r':
					default:
						text.WriteRune(ch)
					}
				}
				flush()
				out.WriteString(`</w:r>`)
				if r.Link != "" {
					out.WriteString(`</w:hyperlink>`)
				}
			}
			out.WriteString(`</w:p>`)
		case Table:
			columns := 0
			for _, r := range b.Rows {
				n := 0
				for _, c := range r.Cells {
					n += max(1, c.ColSpan)
				}
				columns = max(columns, n)
			}
			out.WriteString(`<w:tbl><w:tblPr><w:tblW w:w="0" w:type="auto"/><w:tblBorders><w:top w:val="single" w:sz="4"/><w:left w:val="single" w:sz="4"/><w:bottom w:val="single" w:sz="4"/><w:right w:val="single" w:sz="4"/><w:insideH w:val="single" w:sz="4"/><w:insideV w:val="single" w:sz="4"/></w:tblBorders></w:tblPr><w:tblGrid>`)
			for i := 0; i < columns; i++ {
				out.WriteString(`<w:gridCol w:w="2400"/>`)
			}
			out.WriteString(`</w:tblGrid>`)
			for _, row := range b.Rows {
				out.WriteString(`<w:tr>`)
				for _, c := range row.Cells {
					out.WriteString(`<w:tc><w:tcPr>`)
					if c.ColSpan > 1 {
						fmt.Fprintf(&out, `<w:gridSpan w:val="%d"/>`, c.ColSpan)
					}
					out.WriteString(`</w:tcPr>`)
					v, err := w.blocks(c.Blocks)
					if err != nil {
						return "", err
					}
					out.WriteString(v)
					if len(c.Blocks) == 0 || c.Blocks[len(c.Blocks)-1].Kind == Table {
						out.WriteString(`<w:p/>`)
					}
					out.WriteString(`</w:tc>`)
				}
				out.WriteString(`</w:tr>`)
			}
			out.WriteString(`</w:tbl>`)
		case Picture:
			im := b.Image
			config, kind, err := image.DecodeConfig(bytes.NewReader(im.Data))
			if err != nil {
				return "", fmt.Errorf("image: %w", err)
			}
			ext := "png"
			if kind == "jpeg" {
				ext = "jpg"
			} else if kind != "png" {
				return "", fmt.Errorf("%w: DOCX image %s", ErrUnsupported, kind)
			}
			w.images++
			name := fmt.Sprintf("media/image%d.%s", w.images, ext)
			w.parts["word/"+name] = im.Data
			id := w.addRel("image", name, false)
			width, height := im.Width, im.Height
			if width == 0 {
				width = float64(config.Width) * 0.75
			}
			if height == 0 {
				height = width * float64(config.Height) / float64(config.Width)
			}
			fmt.Fprintf(&out, `<w:p><w:r><w:drawing><wp:inline><wp:extent cx="%.0f" cy="%.0f"/><wp:docPr id="%d" name="Image %d" descr="%s"/><a:graphic><a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/picture"><pic:pic><pic:nvPicPr><pic:cNvPr id="0" name="Image"/><pic:cNvPicPr/></pic:nvPicPr><pic:blipFill><a:blip r:embed="%s"/><a:stretch><a:fillRect/></a:stretch></pic:blipFill><pic:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="%.0f" cy="%.0f"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></pic:spPr></pic:pic></a:graphicData></a:graphic></wp:inline></w:drawing></w:r></w:p>`, width*12700, height*12700, w.images, w.images, escape(im.Alt), id, width*12700, height*12700)
		}
	}
	return out.String(), nil
}
