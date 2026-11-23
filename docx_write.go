package manydoc

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"math"
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
	ctx                context.Context
	parts              map[string][]byte
	rels               []string
	next               int
	images             int
	contentWidth       float64
	contentHeight      float64
	runningImageHeight float64
	validatedImages    outputImageCache
	numberings         []docxNumbering
	bookmarks          int
	noteIDs            map[noteKey]int
	noteLabels         map[noteKey]string
	seenNoteRefs       map[noteKey]bool
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
	if hasSectionBlocks(d.Blocks) {
		return writeSectionedDOCX(ctx, out, d)
	}
	notes, err := catalogNotes(d, true)
	if err != nil {
		return err
	}
	g := pageLayout(d.Layout)
	if err := validateDOCXPageGeometry(g); err != nil {
		return err
	}
	if len(d.Headers) > 0 && g.headerDistance() >= g.height || len(d.Footers) > 0 && g.footerDistance() >= g.height {
		return fmt.Errorf("%w: DOCX running anchor lies outside the page", ErrUnsupported)
	}
	wid, hei := g.width, g.height
	w := &docxWriter{ctx: ctx, parts: map[string][]byte{}, next: 1, contentWidth: g.contentWidth(), contentHeight: g.contentHeight(), validatedImages: make(outputImageCache)}
	w.noteIDs = map[noteKey]int{}
	w.noteLabels = notes.labels
	w.seenNoteRefs = map[noteKey]bool{}
	noteCounts := map[string]int{}
	for _, n := range d.Notes {
		noteCounts[n.Kind]++
		w.noteIDs[noteIdentity(n)] = noteCounts[n.Kind]
	}
	imageHeight, bodyHeight, err := w.fitRunning(d, g)
	if err != nil {
		return err
	}
	w.contentHeight = bodyHeight
	w.runningImageHeight = 0
	w.addRel("styles", "styles.xml", false)
	w.addRel("numbering", "numbering.xml", false)
	body, err := w.blocks(d.Blocks)
	if err != nil {
		return err
	}
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
		cap := imageHeight
		header := v.root == "hdr"
		if header && g.margins.Top < 0 || !header && g.margins.Bottom < 0 {
			cap, err = w.fitFixedRunning(v.blocks, g, header)
			if err != nil {
				return err
			}
		}
		w.contentHeight, w.runningImageHeight = g.runningStoryHeight(header, len(d.Headers) > 0, len(d.Footers) > 0), cap
		part, err := w.blocks(v.blocks)
		w.contentHeight, w.runningImageHeight = bodyHeight, 0
		if err != nil {
			return err
		}
		if len(w.rels) > 0 {
			w.parts["word/_rels/"+v.name+".rels"] = []byte(xmlHeader + `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` + strings.Join(w.rels, "") + `</Relationships>`)
		}
		w.rels = mainRels
		id := w.addRel(strings.TrimSuffix(v.name, "1.xml"), v.name, false)
		w.parts["word/"+v.name] = []byte(xmlHeader + `<w:` + v.root + wordNamespaces + docxMetadataNamespaces + `>` + part + `</w:` + v.root + `>`)
		section += `<w:` + v.ref + ` w:type="default" r:id="` + id + `"/>`
	}
	if err := w.writeNoteParts(d.Notes); err != nil {
		return err
	}
	for _, kind := range []string{Footnote, Endnote} {
		if noteCounts[kind] > 0 {
			section += `<w:` + kind + `Pr><w:numFmt w:val="decimal"/><w:numRestart w:val="continuous"/></w:` + kind + `Pr>`
		}
	}
	orientation := ""
	if wid > hei {
		orientation = ` w:orient="landscape"`
	}
	section += fmt.Sprintf(`<w:pgSz w:w="%.0f" w:h="%.0f"%s/>`, wid*20, hei*20, orientation) + docxPageMargins(d.Layout) + `</w:sectPr>`
	w.parts["word/document.xml"] = []byte(xmlHeader + `<w:document` + wordNamespaces + docxMetadataNamespaces + `><w:body>` + body + section + `</w:body></w:document>`)
	return w.output(out, d)
}

func (w *docxWriter) output(out io.Writer, d *Document) error {
	w.parts["word/styles.xml"] = []byte(xmlHeader + `<w:styles xmlns:w="` + wordNS + `"><w:docDefaults><w:rPrDefault><w:rPr><w:rFonts w:ascii="Noto Sans" w:hAnsi="Noto Sans"/><w:sz w:val="22"/></w:rPr></w:rPrDefault></w:docDefaults><w:style w:type="paragraph" w:default="1" w:styleId="Normal"><w:name w:val="Normal"/></w:style>` + headingStyles() + noteStyles() + `</w:styles>`)
	w.parts["word/numbering.xml"] = []byte(w.numberingXML())
	relsHead := xmlHeader + `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`
	w.parts["word/_rels/document.xml.rels"] = []byte(relsHead + strings.Join(w.rels, "") + `</Relationships>`)
	w.parts["_rels/.rels"] = []byte(relsHead + `<Relationship Id="rId1" Type="` + relNS + `/officeDocument" Target="word/document.xml"/><Relationship Id="rId2" Type="http://schemas.openxmlformats.org/package/2006/relationships/metadata/core-properties" Target="docProps/core.xml"/></Relationships>`)
	w.parts["docProps/core.xml"] = []byte(xmlHeader + `<cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties" xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:title>` + escape(d.Metadata.Title) + `</dc:title><dc:creator>` + escape(d.Metadata.Author) + `</dc:creator><dc:subject>` + escape(d.Metadata.Subject) + `</dc:subject></cp:coreProperties>`)
	ct := xmlHeader + `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Default Extension="png" ContentType="image/png"/><Default Extension="jpg" ContentType="image/jpeg"/><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/><Override PartName="/word/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.styles+xml"/><Override PartName="/word/numbering.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.numbering+xml"/><Override PartName="/docProps/core.xml" ContentType="application/vnd.openxmlformats-package.core-properties+xml"/>`
	storyParts := []struct{ name, kind string }{}
	var names []string
	for name := range w.parts {
		if strings.HasPrefix(name, "word/header") && strings.HasSuffix(name, ".xml") {
			names = append(names, strings.TrimPrefix(name, "word/"))
		}
	}
	sort.Strings(names)
	for _, name := range names {
		storyParts = append(storyParts, struct{ name, kind string }{name, "header"})
	}
	names = nil
	for name := range w.parts {
		if strings.HasPrefix(name, "word/footer") && strings.HasSuffix(name, ".xml") {
			names = append(names, strings.TrimPrefix(name, "word/"))
		}
	}
	sort.Strings(names)
	for _, name := range names {
		storyParts = append(storyParts, struct{ name, kind string }{name, "footer"})
	}
	storyParts = append(storyParts, struct{ name, kind string }{"footnotes.xml", "footnotes"}, struct{ name, kind string }{"endnotes.xml", "endnotes"}, struct{ name, kind string }{"settings.xml", "settings"})
	for _, v := range storyParts {
		if w.parts["word/"+v.name] != nil {
			ct += `<Override PartName="/word/` + v.name + `" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.` + v.kind + `+xml"/>`
		}
	}
	w.parts["[Content_Types].xml"] = []byte(ct + `</Types>`)
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	names = make([]string, 0, len(w.parts))
	for n := range w.parts {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		if err := w.ctx.Err(); err != nil {
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
	_, err := io.Copy(out, &buf)
	return err
}
func headingStyles() string {
	var b strings.Builder
	for i := 1; i <= 6; i++ {
		fmt.Fprintf(&b, `<w:style w:type="paragraph" w:styleId="Heading%d"><w:name w:val="heading %d"/><w:basedOn w:val="Normal"/><w:pPr><w:keepNext/><w:outlineLvl w:val="%d"/></w:pPr><w:rPr><w:b/><w:sz w:val="%d"/></w:rPr></w:style>`, i, i, i-1, 44-i*3)
	}
	return b.String()
}
func (w *docxWriter) blocks(blocks []Block) (string, error) { return w.blocksAt(blocks, 0) }
func (w *docxWriter) blocksAt(blocks []Block, depth int) (string, error) {
	var out strings.Builder
	bs := legacyLists(blocks)
	for i := 0; i < len(bs); i++ {
		b := bs[i]
		if err := w.ctx.Err(); err != nil {
			return "", err
		}
		if b.Kind == PageBreak {
			if i == 0 {
				// An initial conditional property alone cannot request an empty
				// first page. This checked empty anchor establishes that page.
				out.WriteString(`<w:p md:breakRole="anchor"/>`)
			}
			if i+1 < len(bs) && (bs[i+1].Kind == Paragraph || bs[i+1].Kind == Heading || bs[i+1].Kind == ListBlock) {
				i++
				b = bs[i]
				b.docxExplicitBreakBefore = true
				b.docxBeforeList = b.Kind == ListBlock
			} else {
				out.WriteString(`<w:p md:breakRole="control"><w:pPr><w:pageBreakBefore/></w:pPr></w:p>`)
				continue
			}
		}
		switch b.Kind {
		case Paragraph, Heading:
			out.WriteString(`<w:p`)
			if b.docxExplicitBreakBefore {
				if b.docxBeforeList {
					out.WriteString(` md:breakRole="beforeList"`)
				} else {
					out.WriteString(` md:breakRole="before"`)
				}
				if b.PageBreakBefore {
					out.WriteString(` md:conditionalBefore="1"`)
				}
			}
			out.WriteString(`><w:pPr>`)
			if b.Direction != "" {
				fmt.Fprintf(&out, `<w:bidi w:val="%t"/>`, b.Direction == "rtl")
			}
			if b.Kind == Heading {
				fmt.Fprintf(&out, `<w:pStyle w:val="Heading%d"/>`, b.Level)
			}
			if b.PageBreakBefore || b.docxExplicitBreakBefore {
				out.WriteString(`<w:pageBreakBefore/>`)
			}
			if b.docxNumberID > 0 {
				fmt.Fprintf(&out, `<w:numPr><w:ilvl w:val="%d"/><w:numId w:val="%d"/></w:numPr>`, b.docxNumberLevel, b.docxNumberID)
			}
			if b.docxIndent > 0 {
				fmt.Fprintf(&out, `<w:ind w:left="%d"/>`, b.docxIndent)
			}
			out.WriteString(`</w:pPr>`)
			for _, r := range b.Runs {
				repeated := false
				noteBookmark := 0
				noteName := ""
				if r.Note != nil {
					key := referenceKey(r.Note)
					id, ok := w.noteIDs[key]
					if !ok {
						return "", fmt.Errorf("dangling note reference")
					}
					noteName = fmt.Sprintf("mdnote1_%s_%d", key.kind, id)
					repeated = w.seenNoteRefs[key]
					if repeated {
						fmt.Fprintf(&out, `<w:fldSimple w:instr=" NOTEREF %s \h \f ">`, noteName)
					} else {
						w.seenNoteRefs[key] = true
						w.bookmarks++
						noteBookmark = w.bookmarks
						fmt.Fprintf(&out, `<w:bookmarkStart w:id="%d" w:name="%s"/>`, noteBookmark, noteName)
					}
				}
				if r.Link != "" {
					if strings.HasPrefix(r.Link, "#") {
						out.WriteString(`<w:hyperlink w:anchor="` + escape(strings.TrimPrefix(r.Link, "#")) + `">`)
					} else {
						id := w.addRel("hyperlink", r.Link, true)
						out.WriteString(`<w:hyperlink r:id="` + id + `">`)
					}
				}
				out.WriteString(`<w:r`)
				out.WriteString(docxTextImplicit(r))
				if b.Kind == Heading {
					if r.Bold {
						out.WriteString(` md:headingBold="1"`)
					} else {
						out.WriteString(` md:headingBold="0"`)
					}
				}
				out.WriteString(`><w:rPr>`)
				textStyle, err := docxTextStyle(r)
				if err != nil {
					return "", err
				}
				out.WriteString(textStyle)
				if r.Note != nil {
					out.WriteString(`<w:rStyle w:val="` + strings.Title(r.Note.Kind) + `Reference"/>`)
				}
				if r.Bold {
					out.WriteString(`<w:b/><w:bCs/>`)
				}
				if r.Italic {
					out.WriteString(`<w:i/><w:iCs/>`)
				}
				if r.Underline {
					out.WriteString(`<w:u w:val="single"/>`)
				}
				if r.Strike {
					out.WriteString(`<w:strike/>`)
				}
				out.WriteString(`</w:rPr>`)
				if r.Note != nil {
					id, ok := w.noteIDs[referenceKey(r.Note)]
					if !ok {
						return "", fmt.Errorf("dangling note reference")
					}
					if repeated {
						fmt.Fprintf(&out, `<w:t>%s</w:t></w:r></w:fldSimple>`, escape(w.noteLabels[referenceKey(r.Note)]))
					} else {
						fmt.Fprintf(&out, `<w:%sReference w:id="%d"/></w:r><w:bookmarkEnd w:id="%d"/>`, r.Note.Kind, id, noteBookmark)
					}
					continue
				}
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
		case ListBlock:
			v, err := w.list(b, depth)
			if err != nil {
				return "", err
			}
			out.WriteString(v)
		case Table:
			v, err := w.table(b)
			if err != nil {
				return "", err
			}
			out.WriteString(v)
		case Picture:
			im := b.Image
			config, kind, err := decodeOutputImage(w.ctx, im.Data, w.validatedImages)
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
			available := w.contentWidth - float64(b.docxIndent)/20
			width, height, err := fitImageBox(im, config.Width, config.Height, available, w.contentHeight-8)
			if err != nil {
				return "", err
			}
			if w.runningImageHeight > 0 && height > w.runningImageHeight {
				width *= w.runningImageHeight / height
				height = w.runningImageHeight
			}
			if width > float64(math.MaxInt64)/12700 || height > float64(math.MaxInt64)/12700 || fmt.Sprintf("%.0f", width*12700) == "0" || fmt.Sprintf("%.0f", height*12700) == "0" {
				return "", fmt.Errorf("%w: DOCX image exceeds EMU coordinate range", ErrUnsupported)
			}
			imageParagraph := "<w:p>"
			if b.docxIndent > 0 {
				imageParagraph += fmt.Sprintf(`<w:pPr><w:ind w:left="%d"/></w:pPr>`, b.docxIndent)
			}
			out.WriteString(imageParagraph)
			fmt.Fprintf(&out, `<w:r><w:drawing><wp:inline><wp:extent cx="%.0f" cy="%.0f"/><wp:docPr id="%d" name="Image %d" descr="%s"/><a:graphic><a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/picture"><pic:pic><pic:nvPicPr><pic:cNvPr id="0" name="Image"/><pic:cNvPicPr/></pic:nvPicPr><pic:blipFill><a:blip r:embed="%s"/><a:stretch><a:fillRect/></a:stretch></pic:blipFill><pic:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="%.0f" cy="%.0f"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></pic:spPr></pic:pic></a:graphicData></a:graphic></wp:inline></w:drawing></w:r></w:p>`, width*12700, height*12700, w.images, w.images, escape(im.Alt), id, width*12700, height*12700)
		}
	}
	return out.String(), nil
}
