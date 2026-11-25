package engine

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"github.com/rivo/uniseg"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
)

func docxPageMargins(l Layout) string {
	g := pageLayout(l)
	m := g.margins
	metadata := ""
	if l.Margins != nil {
		metadata += ` md:layoutMargins="1" md:layoutMargin="` + strconv.FormatFloat(l.Margin, 'g', -1, 64) + `"`
	}
	implicit := ""
	if l.HeaderDistance == nil {
		implicit += "h"
	}
	if l.FooterDistance == nil {
		implicit += "f"
	}
	if implicit != "" {
		metadata += ` md:layoutImplicit="` + implicit + `"`
	}
	return fmt.Sprintf(`<w:pgMar w:top="%.0f" w:right="%.0f" w:bottom="%.0f" w:left="%.0f" w:header="%.0f" w:footer="%.0f"%s/>`, m.Top*20, m.Right*20, m.Bottom*20, m.Left*20, g.headerDistance()*20, g.footerDistance()*20, metadata)
}
func docxPageNumber(n *xmlNode, key string, fallback float64) (float64, error) {
	v := n.attr(key)
	if v == "" {
		return fallback, nil
	}
	v = strings.TrimSpace(v)
	factor := 0.0
	for _, unit := range []struct {
		suffix string
		points float64
	}{{"mm", 72 / 25.4}, {"cm", 72 / 2.54}, {"in", 72}, {"pt", 1}, {"pc", 12}, {"pi", 12}} {
		if strings.HasSuffix(v, unit.suffix) {
			v = strings.TrimSuffix(v, unit.suffix)
			factor = unit.points
			break
		}
	}
	x, err := strconv.ParseFloat(v, 64)
	if err != nil || !finite(x) || !finite(x*factor) {
		return 0, fmt.Errorf("invalid DOCX page %s", key)
	}
	if factor == 0 {
		return x / 20, nil
	}
	return x * factor, nil
}
func (r *docxReader) pageMargins(n *xmlNode) error {
	left, err := docxPageNumber(n, "left", 54)
	if err != nil {
		return err
	}
	m := PageMargins{Left: left}
	for _, item := range []struct {
		key string
		p   *float64
	}{{"top", &m.Top}, {"right", &m.Right}, {"bottom", &m.Bottom}} {
		v, err := docxPageNumber(n, item.key, left)
		if err != nil {
			return err
		}
		*item.p = v
	}
	marker := n.attributeNS(docxNotesNS, "layoutMargins")
	if marker != "" && marker != "1" {
		return fmt.Errorf("invalid DOCX layoutMargins metadata")
	}
	r.doc.Layout.Margin = left
	r.doc.Layout.Margins = nil
	if marker == "1" || left == 0 || m.Top != left || m.Right != left || m.Bottom != left {
		r.doc.Layout.Margins = &m
	}
	if legacy := n.attributeNS(docxNotesNS, "layoutMargin"); legacy != "" {
		v, err := strconv.ParseFloat(legacy, 64)
		if marker != "1" || err != nil || !finite(v) || v < 0 {
			return fmt.Errorf("invalid DOCX legacy margin metadata")
		}
		r.doc.Layout.Margin = v
	}
	flags := n.attributeNS(docxNotesNS, "layoutImplicit")
	if flags != "" && flags != "h" && flags != "f" && flags != "hf" {
		return fmt.Errorf("invalid DOCX layoutImplicit metadata")
	}
	for _, item := range []struct {
		key, flag string
		p         **float64
	}{{"header", "h", &r.doc.Layout.HeaderDistance}, {"footer", "f", &r.doc.Layout.FooterDistance}} {
		*item.p = nil
		value := n.attr(item.key)
		if strings.Contains(flags, item.flag) {
			v, err := docxPageNumber(n, item.key, math.NaN())
			if err != nil || v != docxRunningDistance {
				return fmt.Errorf("DOCX implicit %s distance contradicts physical value", item.key)
			}
			continue
		}
		if value != "" {
			v, err := docxPageNumber(n, item.key, 0)
			if err != nil {
				return err
			}
			*item.p = &v
		}
	}
	if v := n.attr("gutter"); v != "" && v != "0" {
		r.doc.warn("docx_page_gutter", "Page gutter/binding offsets are not yet represented")
	}
	return nil
}

// Current visible section properties exclude revision-history snapshots, deleted
// stories and text-box content. A sectPr's children are not additional sections.

// DOCX delegates font selection and pagination to its consumer. These envelopes
// budget inline rasters, not glyph-exact Word layout. Text uses a conservative em
// column estimate; tables retain row-span constraints rather than summing columns.
// Reserve picture descent space and a producer body floor so independently
// fitted header and footer pictures cannot each consume the complete body height.
// matches the existing 360-twip pgMar values

// headingStyles, not PDF heading sizes

func docxActiveSections(n *xmlNode) []*xmlNode {
	if n == nil {
		return nil
	}
	var sections []*xmlNode
	for _, c := range n.Children {
		switch c.Name.Local {
		case "sectPr":
			sections = append(sections, c)
		case "sectPrChange", "pPrChange", "del", "moveFrom", "txbxContent":
		default:
			sections = append(sections, docxActiveSections(c)...)
		}
	}
	return sections
}
func (r *docxReader) pageSettings(n *xmlNode, section bool) error {
	for _, c := range n.Children {
		switch c.Name.Local {
		case "evenAndOddHeaders":
			on, err := docxOnOff(c)
			if err != nil {
				return err
			}
			r.doc.EvenAndOddHeaders = on
		case "titlePg":
			if _, err := docxOnOff(c); err != nil {
				return err
			}
		case "mirrorMargins", "gutterAtTop", "bookFoldPrinting", "bookFoldRevPrinting", "printTwoOnOne":
			on, err := docxOnOff(c)
			if err != nil {
				return err
			}
			if on {
				r.doc.warn("docx_page_settings", "Conditional page/running settings are not yet represented: "+c.Name.Local)
			}
		case "cols":
			if section && (c.attr("num") != "" && c.attr("num") != "1" || len(c.Children) > 0) {
				r.doc.warn("docx_page_columns", "Multiple page columns/column widths are not represented by the single flow")
			}
		case "pgBorders", "textDirection", "paperSrc", "vAlign", "lnNumType", "pgNumType", "rtlGutter", "docGrid":
			if section {
				r.doc.warn("docx_page_settings", "Section page presentation is not represented: "+c.Name.Local)
			}
		case "sectPrChange":
			r.doc.warn("revisions", "Section revision history is omitted; current section properties are retained")
		}
	}
	return nil
}

func validateDOCXPageGeometry(g pageGeometry) error {
	values := []float64{g.width, g.height, g.margins.Top, g.margins.Right, g.margins.Bottom, g.margins.Left, g.headerDistance(), g.footerDistance()}
	for _, v := range values {
		if !finite(v * 20) {
			return fmt.Errorf("%w: DOCX page dimensions exceed twip serialization", ErrUnsupported)
		}
	}
	round := func(v float64) float64 {
		return math.RoundToEven(v*

			// Page controls remain in emitted XML. This is a picture-size
			// budget, not a replacement for consumer pagination.
			20) / 20
	}
	w, h := round(g.width), round(g.height)
	m := g.margins
	if w <= 0 || h <= 0 || w-round(m.Left)-round(m.Right) <= 0 || h-math.Abs(round(m.Top))-math.Abs(round(m.Bottom)) <= 0 {
		return fmt.Errorf("%w: DOCX page/margins round to an empty physical container", ErrUnsupported)
	}
	if m.Top < 0 && round(m.Top) == 0 || m.Bottom < 0 && round(m.Bottom) == 0 {
		return fmt.Errorf("%w: DOCX signed page offset rounds to zero", ErrUnsupported)
	}
	return nil
}

const docxRunningDistance = 18.0
const docxRunningImageGap = 8.0

type docxRunningFlow []docxRunningFragment
type docxRunningFragment struct {
	fixed, image float64
	table        *docxRunningTable
}
type docxRunningTable struct {
	rows int
	ends [][]docxRunningCell
}
type docxRunningCell struct {
	start, end int
	flow       docxRunningFlow
}

func (f docxRunningFlow) height(limit float64) float64 {
	h := 0.0
	for _, fragment := range f {
		h += fragment.fixed + math.Min(fragment.image, limit)
		if table := fragment.table; table != nil {
			offsets := make([]float64, table.rows+1)
			for end := 1; end <= table.rows; end++ {
				offsets[end] = offsets[end-1] + 11*1.4
				for _, cell := range table.ends[end] {
					offsets[end] = math.Max(offsets[end], offsets[cell.start]+cell.flow.height(limit)+docxImageCellHeightAllowance)
				}
			}
			h += offsets[table.rows]
		}
	}
	return h
}
func (f docxRunningFlow) largestImage() float64 {
	largest := 0.0
	for _, fragment := range f {
		largest = math.Max(largest, fragment.image)
		if fragment.table != nil {
			for _, cells := range fragment.table.ends {
				for _, cell := range cells {
					largest = math.Max(largest, cell.flow.largestImage())
				}
			}
		}
	}
	return largest
}
func docxParagraphSize(b Block) float64 {
	fallback := 11.0
	if b.Kind == Heading {
		fallback = float64(44-b.Level*3) / 2
	}
	size := 0.0
	for _, run := range b.Runs {
		if run.Text == "" && run.Note == nil {
			continue
		}
		effective := run.FontSize
		if effective == 0 {
			effective = fallback
		}
		size = math.Max(size, effective)
	}
	if size == 0 {
		size = fallback
	}
	return size
}
func (w *docxWriter) runningParagraph(b Block, width float64) (float64, error) {
	size := docxParagraphSize(b)
	var text strings.Builder
	for _, run := range b.Runs {
		text.WriteString(run.Text)
	}
	lines, advance := 1, 0.0
	g := uniseg.NewGraphemes(text.String())
	for g.Next() {
		if err := w.ctx.Err(); err != nil {
			return 0, err
		}
		s := g.Str()
		switch s {
		case "\r\n", "\r", "\n", "\u0085", "\u2028", "\u2029":
			lines++
			advance = 0
			continue
		}
		em := float64(uniseg.StringWidth(s))
		if s == "\t" {
			em = 4
		}
		w := em * size
		if advance > 0 && advance+w > width {
			lines++
			advance = 0
		}
		advance += w
	}
	return float64(lines)*size*1.4 + size*.4, nil
}
func (w *docxWriter) runningFlow(bs []Block, width, height float64, depth int, indent float64) (docxRunningFlow, int, error) {
	var flow docxRunningFlow
	images := 0
	for _, b := range legacyLists(bs) {
		if err := w.ctx.Err(); err != nil {
			return nil, 0, err
		}
		available := width - indent
		if available <= 0 {
			return nil, 0, fmt.Errorf("%w: no width for DOCX running content", ErrUnsupported)
		}
		switch b.Kind {
		case Paragraph, Heading:
			fixed, err := w.runningParagraph(b, available)
			if err != nil {
				return nil, 0, err
			}
			flow = append(flow, docxRunningFragment{fixed: fixed})
		case ListBlock:
			if depth > 8 {
				return nil, 0, fmt.Errorf("%w: DOCX numbering supports nine levels", ErrUnsupported)
			}
			for _, item := range b.Items {
				if item.Marker != nil {
					table, _ := docxLiteralTable(item, available)
					child, n, err := w.runningFlow([]Block{table}, available, height, depth, 0)
					if err != nil {
						return nil, 0, err
					}
					flow = append(flow, child...)
					images += n
					continue
				}
				child, n, err := w.runningFlow(item.Blocks, width, height, depth+1, float64(depth+1)*18)
				if err != nil {
					return nil, 0, err
				}
				flow = append(flow, child...)
				images += n
			}
		case Picture:
			cfg, _, err := decodeOutputImage(w.ctx, b.Image.Data, w.validatedImages)
			if err != nil {
				return nil, 0, err
			}
			_, h, err := fitImageBox(b.Image, cfg.Width, cfg.Height, available, height-docxRunningImageGap)
			if err != nil {
				return nil, 0, err
			}
			flow = append(flow, docxRunningFragment{fixed: docxRunningImageGap, image: h})
			images++
		case Table:
			grid, widths, err := docxTableColumns(b, available)
			if err != nil {
				return nil, 0, err
			}
			table := &docxRunningTable{rows: len(b.Rows), ends: make([][]docxRunningCell, len(b.Rows)+1)}
			for _, cell := range grid.Origins {
				cellWidth := 0
				for _, width := range widths[cell.Column : cell.Column+cell.ColSpan] {
					cellWidth += width
				}
				blocks := b.Rows[cell.Row].Cells[cell.Cell].Blocks
				child, n, err := w.runningFlow(blocks, float64(cellWidth)/20-8, height-docxImageCellHeightAllowance, 0, 0)
				if err != nil {
					return nil, 0, err
				}
				if len(blocks) == 0 || blocks[len(blocks)-1].Kind == Table {
					child = append(child, docxRunningFragment{fixed: 11 * 1.4})
				}
				end := cell.Row + cell.RowSpan
				table.ends[end] = append(table.ends[end], docxRunningCell{cell.Row, end, child})
				images += n
			}
			flow = append(flow, docxRunningFragment{table: table})
		case PageBreak:

		}
	}
	return flow, images, nil
}

func docxBodyFloor(bs []Block, padding float64) float64 {
	floor := 11*1.4 + padding
	for _, b := range bs {
		switch b.Kind {
		case Paragraph, Heading:
			size := docxParagraphSize(b)
			floor = math.Max(floor, size*1.4+padding)
		case ListBlock:
			for _, item := range b.Items {
				itemPadding := padding
				if item.Marker != nil {
					itemPadding += docxImageCellHeightAllowance
					floor = math.Max(floor, docxBodyFloor(item.Marker.Blocks, itemPadding))
				}
				floor = math.Max(floor, docxBodyFloor(item.Blocks, itemPadding))
			}
		case Table:
			for _, row := range b.Rows {
				for _, cell := range row.Cells {
					floor = math.Max(floor, docxBodyFloor(cell.Blocks, padding+docxImageCellHeightAllowance))
				}
			}
		}
	}
	return floor
}

// Return a common picture-height cap for growing running stories and the body
// container. Signed fixed stories use their independent page-anchor cap instead.
// Zero means no cap. Smaller preferred pictures remain unchanged.
// The producer still controls actual line/page placement.
func (w *docxWriter) fitRunning(d *Document, g pageGeometry) (float64, float64, error) {
	headerHeight := g.runningStoryHeight(true, len(d.Headers) > 0, len(d.Footers) > 0)
	footerHeight := g.runningStoryHeight(false, len(d.Headers) > 0, len(d.Footers) > 0)
	header, hn, err := w.runningFlow(d.Headers, w.contentWidth, headerHeight, 0, 0)
	if err != nil {
		return 0, 0, err
	}
	footer, fn, err := w.runningFlow(d.Footers, w.contentWidth, footerHeight, 0, 0)
	if err != nil {
		return 0, 0, err
	}
	if hn+fn == 0 {
		return 0, w.contentHeight, nil
	}
	available := func(limit float64) float64 {
		top, bottom := math.Abs(g.margins.Top), math.Abs(g.margins.Bottom)
		if len(d.Headers) > 0 && g.margins.Top >= 0 {
			top = math.Max(top, g.headerDistance()+header.height(limit))
		}
		if len(d.Footers) > 0 && g.margins.Bottom >= 0 {
			bottom = math.Max(bottom, g.footerDistance()+footer.height(limit))
		}
		return g.height - top - bottom
	}
	// LibreOffice limits header/footer growth while retaining one fifth of the
	// original page text area. Reserve that space proactively, instead of relying
	// on the producer to relocate oversized footer content beyond the page.
	// This is a compatibility allowance, not a Word-layout-equivalence claim.
	minimum := math.Max(docxBodyFloor(d.Blocks, 0), w.contentHeight/5)
	for _, note := range d.Notes {
		minimum = math.Max(minimum, docxBodyFloor(note.Blocks, docxImageNoteHeightAllowance))
	}
	largest := math.Max(header.largestImage(), footer.largestImage())
	fits := func(limit float64, guard float64) bool {
		return available(limit) >= minimum+guard && (len(d.Headers) == 0 || g.margins.Top < 0 || g.headerDistance()+header.height(limit) <= g.height-guard) && (len(d.Footers) == 0 || g.margins.Bottom < 0 || g.footerDistance()+footer.height(limit) <= g.height-guard)
	}
	if fits(largest, 0) {
		return 0, available(largest), nil
	}
	if !fits(0, 0) {
		return 0, 0, fmt.Errorf("%w: DOCX running text/padding leaves no image and body space", ErrUnsupported)
	}
	low, high := 0.0, largest
	for range 40 {
		if err := w.ctx.Err(); err != nil {
			return 0, 0, err
		}
		mid := (low + high) / 2
		if fits(mid, .001) {
			low = mid
		} else {
			high = mid
		}
	}
	if low <= 0 {
		return 0, 0, fmt.Errorf("%w: DOCX running images cannot fit", ErrUnsupported)
	}
	return low, available(low), nil
}

// Fixed stories fit independently to the canvas. A positive story's body budget
// must not shrink an otherwise fitting negative-edge story.

// Attach to a real final paragraph when possible. Tables/lists require
// a structural paragraph; its checked marker never hides visible content.

// A block-level list bookmark ending immediately before a table makes
// LibreOffice synthesize another numbered paragraph. End numbering explicitly
// in a checked, empty layout boundary before the table.

func (w *docxWriter) fitFixedRunning(blocks []Block, g pageGeometry, header bool) (float64, error) {
	if len(blocks) == 0 {
		return 0, nil
	}
	height := g.runningStoryHeight(header, true, true)
	flow, _, err := w.runningFlow(blocks, w.contentWidth, height, 0, 0)
	if err != nil {
		return 0, err
	}
	largest := flow.largestImage()
	if flow.height(largest) <= height {
		return 0, nil
	}
	if flow.height(0) >= height || largest <= 0 {
		return 0, fmt.Errorf("%w: DOCX fixed running text/padding exceeds page anchor", ErrUnsupported)
	}
	low, high := 0.0, largest
	for range 40 {
		if err := w.ctx.Err(); err != nil {
			return 0, err
		}
		mid := (low + high) / 2
		if flow.height(mid) <=

			// An initial conditional property alone cannot request an empty
			// first page. This checked empty anchor establishes that page.

			height-.001 {
			low = mid
		} else {
			high = mid
		}
	}
	if low <= 0 {
		return 0, fmt.Errorf("%w: DOCX fixed running images cannot fit page", ErrUnsupported)
	}
	return low, nil
}
func writeSectionedDOCX(ctx context.Context, out io.Writer, d *Document, allowLossy bool) error {
	notes, err := catalogNotes(d, true)
	if err != nil {
		return err
	}
	w := &docxWriter{allowLossy: allowLossy, ctx: ctx, parts: map[string][]byte{}, next: 1, validatedImages: make(outputImageCache)}
	counts := w.initializeNotes(d, notes)
	w.addRel("styles", "styles.xml", false)
	w.addRel("numbering", "numbering.xml", false)
	resolved := documentSections(d)
	var body strings.Builder
	storyIndex := map[string]int{}
	noteWidth, noteHeight := math.Inf(1), math.Inf(1)
	for index, block := range d.Blocks {
		s := block.Section
		r := resolved[index]
		g := pageLayout(r.layout)
		if err := validateDOCXPageGeometry(g); err != nil {
			return err
		}
		w.contentWidth, w.contentHeight = g.contentWidth(), g.contentHeight()
		cap, bodyHeight, err := w.fitSectionStories(d, r)
		if err != nil {
			return err
		}
		w.contentHeight = bodyHeight
		noteWidth, noteHeight = math.Min(noteWidth, w.contentWidth), math.Min(noteHeight, bodyHeight)
		part, err := w.blocks(r.blocks)
		if err != nil {
			return err
		}
		var props strings.Builder
		props.WriteString(`<w:sectPr md:sectionModel="1"`)
		if len(block.Anchors) > 0 {
			raw, _ := json.Marshal(block.Anchors)
			props.WriteString(` md:sectionAnchors="` + escape(string(raw)) + `"`)
		}
		if index == 0 {
			base, _ := json.Marshal(d.Layout)
			props.WriteString(` md:sectionBase="` + escape(string(base)) + `"`)
		}
		if s.Layout == nil {
			props.WriteString(` md:sectionLayout="inherit"`)
		} else {
			raw, _ := json.Marshal(s.Layout)
			props.WriteString(` md:sectionLayout="` + escape(string(raw)) + `"`)
		}
		if s.Start == "" {
			props.WriteString(` md:sectionStartImplicit="1"`)
		}
		props.WriteString(`>`)
		for _, role := range []struct {
			kind, root string
			stories    RunningStories
		}{{"header", "hdr", s.Headers}, {"footer", "ftr", s.Footers}} {
			for _, variant := range []struct {
				name  string
				story *RunningStory
			}{{"default", role.stories.Default}, {"first", role.stories.First}, {"even", role.stories.Even}} {
				if variant.story == nil {
					continue
				}
				storyIndex[role.kind]++
				name := fmt.Sprintf("%s%d.xml", role.kind, storyIndex[role.kind])
				main := w.rels
				w.rels = nil
				storyCap := cap
				header := role.kind == "header"
				if header && g.margins.Top < 0 || !header && g.margins.Bottom < 0 {
					storyCap, err = w.fitFixedRunning(variant.story.Blocks, g, header)
					if err != nil {
						return err
					}
				}
				w.contentHeight, w.runningImageHeight = g.runningStoryHeight(header, true, true), storyCap
				xml, err := w.blocks(variant.story.Blocks)
				w.contentHeight, w.runningImageHeight = bodyHeight, 0
				if err != nil {
					return err
				}
				if len(variant.story.Blocks) == 0 {
					xml = `<w:p md:emptyStory="1"/>`
				}
				w.parts["word/"+name] = []byte(xmlHeader + `<w:` + role.root + wordNamespaces + docxMetadataNamespaces + `>` + xml + `</w:` + role.root + `>`)
				if len(w.rels) > 0 {
					w.parts["word/_rels/"+name+".rels"] = []byte(xmlHeader + `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` + strings.Join(w.rels, "") + `</Relationships>`)
				}
				w.rels = main
				id := w.addRel(role.kind, name, false)
				fmt.Fprintf(&props, `<w:%sReference w:type="%s" r:id="%s"/>`, role.kind, variant.name, id)
			}
		}
		props.WriteString(docxNoteProperties(counts))
		start := s.Start
		if start == "" {
			start = SectionNextPage
		}
		fmt.Fprintf(&props, `<w:type w:val="%s"/>`, start)
		orientation := ""
		if g.width > g.height {
			orientation = ` w:orient="landscape"`
		}
		fmt.Fprintf(&props, `<w:pgSz w:w="%.0f" w:h="%.0f"%s/>`, g.width*20, g.height*20, orientation)
		props.WriteString(docxPageMargins(r.layout))
		if s.FirstPage {
			props.WriteString(`<w:titlePg/>`)
		}
		props.WriteString(`</w:sectPr>`)
		if index == len(d.Blocks)-1 {
			body.WriteString(part)
			body.WriteString(props.String())
		} else {

			if len(s.Blocks) > 0 && (s.Blocks[len(s.Blocks)-1].Kind == Paragraph || s.Blocks[len(s.Blocks)-1].Kind == Heading) {
				last := strings.LastIndex(part, "<w:pPr>")
				if last < 0 {
					return fmt.Errorf("missing DOCX section paragraph")
				}
				end := strings.Index(part[last:], "</w:pPr>")
				if end < 0 {
					return fmt.Errorf("missing section paragraph properties")
				}
				at := last + end
				part = part[:at] + props.String() + part[at:]
				body.WriteString(part)
			} else {
				body.WriteString(part)
				body.WriteString(`<w:p md:sectionTail="1"><w:pPr>` + props.String() + `</w:pPr></w:p>`)
			}
		}
	}
	w.contentWidth, w.contentHeight = noteWidth, noteHeight
	if err := w.writeNoteParts(d.Notes); err != nil {
		return err
	}
	if d.EvenAndOddHeaders {
		w.parts["word/settings.xml"] = []byte(xmlHeader + `<w:settings xmlns:w="` + wordNS + `"><w:evenAndOddHeaders/></w:settings>`)
		w.addRel("settings", "settings.xml", false)
	}
	w.parts["word/document.xml"] = []byte(xmlHeader + `<w:document` + wordNamespaces + docxMetadataNamespaces + docxUnresolvedDestinations(d) + `><w:body>` + body.String() + `</w:body></w:document>`)
	return w.output(out, d)
}
func (w *docxWriter) fitSectionStories(d *Document, r resolvedSection) (float64, float64, error) {
	g := pageLayout(r.layout)
	cap, body := 0.0, g.contentHeight()
	kinds := []string{"default"}
	if r.firstPage {
		kinds = append(kinds, "first")
	}
	if d.EvenAndOddHeaders {
		kinds = append(kinds, "even")
	}
	for _, kind := range kinds {
		pick := func(s RunningStories) []Block {
			p := s.Default
			if kind == "first" {
				p = s.First
			}
			if kind == "even" {
				p = s.Even
			}
			if p == nil {
				return nil
			}
			return p.Blocks
		}
		doc := &Document{Layout: r.layout, Blocks: r.blocks, Headers: pick(r.headers), Footers: pick(r.footers), Notes: d.Notes}
		x, h, err := w.fitRunning(doc, g)
		if err != nil {
			return 0, 0, err
		}
		if x > 0 && (cap == 0 || x < cap) {
			cap = x
		}
		body = math.Min(body, h)
	}
	return cap, body, nil
}

func decodeSectionLayout(raw string) (Layout, error) {
	var l Layout
	dec := json.NewDecoder(bytes.NewBufferString(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&l); err != nil {
		return l, err
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		return l, fmt.Errorf("section layout metadata must contain one value")
	}
	return l, validateLayout(l)
}
func escape(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

const xmlHeader = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`
const wordNamespaces = ` xmlns:w="` + wordNS + `" xmlns:r="` + relNS + `" xmlns:wp="http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:pic="http://schemas.openxmlformats.org/drawingml/2006/picture"`

type docxWriter struct {
	noteStory          bool
	allowLossy         bool
	destinationNames   map[string]string
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
	noteVariants       map[noteKey]bool
	noteCatalog        *noteCatalog
	noteCustomLabels   map[noteKey]string
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

func writeDOCX(ctx context.Context, out io.Writer, d *Document, allowLossy bool) error {
	if hasSectionBlocks(d.Blocks) {
		return writeSectionedDOCX(ctx, out, d, allowLossy)
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
	w := &docxWriter{allowLossy: allowLossy, ctx: ctx, parts: map[string][]byte{}, next: 1, contentWidth: g.contentWidth(), contentHeight: g.contentHeight(), validatedImages: make(outputImageCache)}
	noteCounts := w.initializeNotes(d, notes)
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
	section += docxNoteProperties(noteCounts)
	orientation := ""
	if wid > hei {
		orientation = ` w:orient="landscape"`
	}
	section += fmt.Sprintf(`<w:pgSz w:w="%.0f" w:h="%.0f"%s/>`, wid*20, hei*20, orientation) + docxPageMargins(d.Layout) + `</w:sectPr>`
	w.parts["word/document.xml"] = []byte(xmlHeader + `<w:document` + wordNamespaces + docxMetadataNamespaces + docxUnresolvedDestinations(d) + `><w:body>` + body + section + `</w:body></w:document>`)
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
	hasPhysical := false
	for i := 0; i < len(bs); i++ {
		b := bs[i]

		if i > 0 && bs[i-1].Kind == ListBlock && b.Kind == Table {
			out.WriteString(docxListTableBoundary)
		}
		for _, id := range b.Anchors {
			v, err := w.anchor(id)
			if err != nil {
				return "", err
			}
			if b.Kind == AnchorPoint {
				v = strings.Replace(v, ` md:anchor=`, ` md:standaloneAnchor="`+escape(b.Anchors[0])+`" md:anchor=`, 1)
			}
			out.WriteString(v)
		}
		if b.Kind == AnchorPoint {
			continue
		}
		if err := w.ctx.Err(); err != nil {
			return "", err
		}
		if b.Kind == PageBreak {
			if !hasPhysical {

				out.WriteString(`<w:p md:breakRole="anchor"/>`)
			}
			if i+1 < len(bs) && len(bs[i+1].Anchors) == 0 && (bs[i+1].Kind == Paragraph || bs[i+1].Kind == Heading || bs[i+1].Kind == ListBlock) {
				i++
				b = bs[i]
				b.docxExplicitBreakBefore = true
				b.docxBeforeList = b.Kind == ListBlock
			} else {
				out.WriteString(`<w:p md:breakRole="control"><w:pPr><w:pageBreakBefore/></w:pPr></w:p>`)
				hasPhysical = true
				continue
			}
		}
		hasPhysical = true
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
				if r.Anchor != "" {
					v, err := w.anchor(r.Anchor)
					if err != nil {
						return "", err
					}
					out.WriteString(v)
					continue
				}
				if r.Note != nil && w.noteVariants[referenceKey(r.Note)] {
					value, err := w.variantNoteReference(r, b)
					if err != nil {
						return "", err
					}
					out.WriteString(value)
					continue
				}
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
						fmt.Fprintf(&out, `<w:fldSimple w:instr=" NOTEREF %s \h \f "%s>`, noteName, docxNoteInference(r.Note))
					} else {
						w.seenNoteRefs[key] = true
						w.bookmarks++
						noteBookmark = w.bookmarks
						fmt.Fprintf(&out, `<w:bookmarkStart w:id="%d" w:name="%s"/>`, noteBookmark, noteName)
					}
				}
				if r.Destination != "" {
					name, err := w.destinationName(r.Destination)
					if err != nil {
						return "", err
					}
					out.WriteString(`<w:hyperlink w:anchor="` + escape(name) + `" md:destination="` + escape(r.Destination) + `" md:href="` + escape(destinationHref(r)) + `">`)
				} else if r.Link != "" {
					id := w.addRel("hyperlink", r.Link, true)
					out.WriteString(`<w:hyperlink r:id="` + id + `">`)
				}
				out.WriteString(`<w:r`)
				out.WriteString(docxTextImplicit(r))
				out.WriteString(docxFontSizeIntent(r))
				if b.Kind == Heading {
					if r.Bold {
						out.WriteString(` md:headingBold="1"`)
					} else {
						out.WriteString(` md:headingBold="0"`)
					}
				}
				out.WriteString(`><w:rPr>`)
				textStyle, err := docxTextStyle(r, w.allowLossy)
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
						if label := w.noteCustomLabels[referenceKey(r.Note)]; label != "" {
							fmt.Fprintf(&out, `<w:%sReference w:id="%d" w:customMarkFollows="1"%s/>`, r.Note.Kind, id, docxNoteInference(r.Note))
							// One Unicode scalar per text node also avoids LibreOffice duplicating
							// a multi-character custom mark on import. These are one logical mark.
							for _, scalar := range label {
								fmt.Fprintf(&out, `<w:t xml:space="preserve">%s</w:t>`, escape(string(scalar)))
							}
							fmt.Fprintf(&out, `</w:r><w:bookmarkEnd w:id="%d"/>`, noteBookmark)
						} else {
							fmt.Fprintf(&out, `<w:%sReference w:id="%d"%s/></w:r><w:bookmarkEnd w:id="%d"/>`, r.Note.Kind, id, docxNoteInference(r.Note), noteBookmark)
						}
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
				if r.Link != "" || r.Destination != "" {
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
