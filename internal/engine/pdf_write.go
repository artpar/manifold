package engine

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
	"github.com/phpdave11/gofpdf"
	"golang.org/x/image/font/gofont/gobold"

	// This tree contains role/relationship intent and references to actual paint,
	// with standard inline replacement text, never a cached SF/body representation.
	// MCIDs are contiguous within a page.
	"golang.org/x/image/font/gofont/gobolditalic"
	"golang.org/x/image/font/gofont/goitalic"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/text/unicode/norm"
	"io"
	"math"
	"sort"
	"strings"
	"sync"
	"unicode"
	"unicode/utf16"
)

type pdfOutputNode struct {
	role     string
	parent   *pdfOutputNode
	children []*pdfOutputNode
	marks    []pdfOutputMark
	refs     []*pdfOutputNode
	attrs    types.Dict
	id, alt  string
	actual   *strings.Builder
}
type pdfOutputMark struct{ page, mcid int }
type pdfOutputStructure struct {
	root   *pdfOutputNode
	nodes  []*pdfOutputNode
	pages  map[int][]*pdfOutputNode
	notes  map[noteKey]*pdfOutputNode
	labels map[noteKey]*pdfOutputNode
}

func (s *pdfOutputStructure) node(role string, parent *pdfOutputNode) *pdfOutputNode {
	n := &pdfOutputNode{role: role, parent: parent}
	s.nodes = append(s.nodes, n)
	if parent != nil {
		parent.children = append(parent.children, n)
	}
	return n
}
func preparePDFOutputStructure(d *Document) (*Document, *pdfOutputStructure) {
	s := &pdfOutputStructure{pages: map[int][]*pdfOutputNode{}, notes: map[noteKey]*pdfOutputNode{}, labels: map[noteKey]*pdfOutputNode{}}
	s.root = s.node("Document", nil)
	// Reserve already-global IDs. Qualify scoped collisions once without growing
	// prefixes on subsequent PDF reflow cycles.
	counts := map[string]int{}
	usedIDs := map[string]bool{}
	for _, n := range d.Notes {
		counts[n.ID]++
	}
	for id, count := range counts {
		if count == 1 {
			usedIDs[id] = true
		}
	}
	// Allocate definitions before citations, but attach them after main content.
	for _, note := range d.Notes {
		n := s.node("FENote", nil)
		n.id = note.ID
		if counts[note.ID] > 1 {
			n.id = note.Kind + "/" + note.ID
			for usedIDs[n.id] {
				n.id += "-"
			}
			usedIDs[n.id] = true
		}
		kind := "Footnote"
		if note.Kind == Endnote {
			kind = "Endnote"
		}
		n.attrs = types.Dict{"O": types.Name("FENote"), "NoteType": types.Name(kind)}
		s.notes[noteIdentity(note)] = n
		s.labels[noteIdentity(note)] = s.node("Lbl", n)
	}
	var runs func([]Run, *pdfOutputNode) []Run
	runs = func(input []Run, parent *pdfOutputNode) []Run {
		out := append([]Run(nil), input...)
		var ordinary *pdfOutputNode
		for i := range out {
			r := &out[i]
			if r.Note != nil {
				note := *r.Note
				r.Note = &note
				n := s.node("Reference", parent)
				n.refs = []*pdfOutputNode{s.notes[referenceKey(r.Note)]}
				if n.refs[0] != nil {
					n.refs[0].refs = append(n.refs[0].refs, n)
				}
				r.pdfOutput = n
				// Label fragments share one citation owner and preserve shaping contexts.
				r.Note.LabelContent = append([]Run(nil), r.Note.LabelContent...)
				for j := range r.Note.LabelContent {
					r.Note.LabelContent[j].pdfOutput = n
				}
				ordinary = nil
			} else if r.Anchor == "" && r.Text != "" {
				// Contiguous ordinary runs share an owner so a shaped cluster
				// crossing a paint boundary remains one replacement range.
				if ordinary == nil {
					ordinary = s.node("Span", parent)
					ordinary.actual = new(strings.Builder)
				}
				ordinary.actual.WriteString(r.Text)
				r.pdfOutput = ordinary
			}
		}
		return out
	}
	var blocks func([]Block, *pdfOutputNode) []Block
	blocks = func(input []Block, parent *pdfOutputNode) []Block {
		out := append([]Block(nil), input...)
		for i := range out {
			b := &out[i]
			switch b.Kind {
			case Paragraph, Heading:
				role := "P"
				if b.Kind == Heading {
					role = fmt.Sprintf("H%d", b.Level)
				}
				n := s.node(role, parent)
				b.Runs = runs(b.Runs, n)
			case SectionBlock:
				sec := *b.Section
				b.Section = &sec
				sec.Blocks = blocks(sec.Blocks, s.node("Sect", parent))
			case ListBlock:
				n := s.node("L", parent)
				format := types.Name("Disc")
				if b.List == "number" {
					format = "Decimal"
					if b.Numbering != nil && !b.Numbering.Legal && b.Numbering.Level < len(b.Numbering.Formats) {
						switch b.Numbering.Formats[b.Numbering.Level] {
						case "decimal", "decimalZero":
							format = "Decimal"
						case "lowerRoman":
							format = "LowerRoman"
						case "upperRoman":
							format = "UpperRoman"
						case "lowerLetter":
							format = "LowerAlpha"
						case "upperLetter":
							format = "UpperAlpha"
						default:
							format = "None"
						}
					}
				}
				n.attrs = types.Dict{"O": types.Name("List"), "ListNumbering": format}
				b.Items = append([]ListItem(nil), b.Items...)
				b.pdfOutputLabels = nil
				for j := range b.Items {
					item := s.node("LI", n)
					b.pdfOutputLabels = append(b.pdfOutputLabels, s.node("Lbl", item))
					if marker := b.Items[j].Marker; marker != nil {
						m := *marker
						m.Blocks = blocks(m.Blocks, b.pdfOutputLabels[j])
						b.Items[j].Marker = &m
					}
					b.Items[j].Blocks = blocks(b.Items[j].Blocks, s.node("LBody", item))
				}
			case Table:
				n := s.node("Table", parent)
				b.Rows = append([]Row(nil), b.Rows...)
				for j := range b.Rows {
					row := s.node("TR", n)
					b.Rows[j].Cells = append([]Cell(nil), b.Rows[j].Cells...)
					for k := range b.Rows[j].Cells {
						c := &b.Rows[j].Cells[k]
						role := "TD"
						cell := s.node(role, row)
						cell.attrs = types.Dict{"O": types.Name("Table"), "RowSpan": types.Integer(max(1, c.RowSpan)), "ColSpan": types.Integer(max(1, c.ColSpan))}
						c.Blocks = blocks(c.Blocks, cell)
					}
				}
			case Picture:
				im := *b.Image
				b.Image = &im
				im.pdfOutput = s.node("Figure", parent)
				im.pdfOutput.alt = im.Alt
			}
		}
		return out
	}
	out := *d
	out.Blocks = blocks(d.Blocks, s.root)
	out.Notes = append([]Note(nil), d.Notes...)
	for i := range out.Notes {
		note := &out.Notes[i]
		key := noteIdentity(*note)
		node := s.notes[key]
		node.parent = s.root
		s.root.children = append(s.root.children, node)
		note.pdfOutputLabel = s.labels[key]
		note.Blocks = blocks(note.Blocks, node)
		if note.BodyLabelStyle != nil {
			v := *note.BodyLabelStyle
			note.BodyLabelStyle = &v
			v.pdfOutput = note.pdfOutputLabel
		}
		note.BodyLabelContent = append([]Run(nil), note.BodyLabelContent...)
		for j := range note.BodyLabelContent {
			note.BodyLabelContent[j].pdfOutput = note.pdfOutputLabel
		}
	}
	return &out, s
}
func (r *pdfRenderer) beginOutputMark(n *pdfOutputNode) bool {
	if r.outputStructure == nil {
		return false
	}
	if n == nil || r.paintingRunning || r.suppressOutputStructure {
		r.pdf.RawWriteStr("/Artifact BMC")
		return true
	}
	page := r.pdf.PageNo()
	mcid := len(r.outputStructure.pages[page])
	r.outputStructure.pages[page] = append(r.outputStructure.pages[page], n)
	n.marks = append(n.marks, pdfOutputMark{page, mcid})
	r.pdf.RawWriteStr(fmt.Sprintf("/%s << /MCID %d >> BDC", n.role, mcid))
	return true
}
func pdfOutputString(text string) types.HexLiteral {
	data := []byte{0xfe, 0xff}
	for _, c := range utf16.Encode([]rune(text)) {
		data = append(data, byte(c>>8), byte(c))
	}
	return types.HexLiteral(hex.EncodeToString(data))
}
func (r *pdfRenderer) finishOutputStructure(p *model.Context) error {
	s := r.outputStructure
	if s == nil {
		return nil
	}
	catalog, err := p.Catalog()
	if err != nil {
		return err
	}
	tree := types.Dict{"Type": types.Name("StructTreeRoot")}
	treeRef, err := p.IndRefForNewObject(tree)
	if err != nil {
		return err
	}
	ns, err := p.IndRefForNewObject(types.Dict{"Type": types.Name("Namespace"), "NS": types.StringLiteral("http://iso.org/pdf2/ssn")})
	if err != nil {
		return err
	}
	refs := map[*pdfOutputNode]types.IndirectRef{}
	dicts := map[*pdfOutputNode]types.Dict{}
	for _, n := range s.nodes {
		d := types.Dict{"Type": types.Name("StructElem"), "S": types.Name(n.role), "NS": *ns}
		ref, e := p.IndRefForNewObject(d)
		if e != nil {
			return e
		}
		refs[n] = *ref
		dicts[n] = d
	}
	pageRefs := map[int]types.IndirectRef{}
	nums := types.Array{}
	for page := 1; page <= p.PageCount; page++ {
		d, ref, _, e := p.PageDict(page, false)
		if e != nil {
			return e
		}
		pageRefs[page] = *ref
		owners := types.Array{}
		for _, n := range s.pages[page] {
			owners = append(owners, refs[n])
		}
		d["StructParents"] = types.Integer(page - 1)
		nums = append(nums, types.Integer(page-1), owners)
	}
	for _, n := range s.nodes {
		if err := r.ctx.Err(); err != nil {
			return err
		}
		d := dicts[n]
		parent := *treeRef
		if n.parent != nil {
			parent = refs[n.parent]
		}
		d["P"] = parent
		kids := types.Array{}
		for _, c := range n.children {
			kids = append(kids, refs[c])
		}
		for _, m := range n.marks {
			kids = append(kids, types.Dict{"Type": types.Name("MCR"), "Pg": pageRefs[m.page], "MCID": types.Integer(m.mcid)})
		}
		d["K"] = kids
		if n.attrs != nil {
			d["A"] = n.attrs
		}
		if n.id != "" {
			d["ID"] = pdfOutputID(n.id)
		}
		if n.alt != "" {
			d["Alt"] = pdfOutputString(n.alt)
		}
		if n.actual != nil && len(n.marks) > 0 {
			d["ActualText"] = pdfOutputString(n.actual.String())
		}
		if len(n.refs) > 0 {
			targets := types.Array{}
			for _, target := range n.refs {
				if target == nil {
					return fmt.Errorf("missing PDF output structure reference target")
				}
				targets = append(targets, refs[target])
			}
			d["Ref"] = targets
		}
	}
	ids := make([]*pdfOutputNode, 0)
	for _, n := range s.nodes {
		if n.id != "" {
			ids = append(ids, n)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return string(pdfOutputID(ids[i].id)) < string(pdfOutputID(ids[j].id)) })
	names := types.Array{}
	for _, n := range ids {
		names = append(names, pdfOutputID(n.id), refs[n])
	}
	idTree, err := p.IndRefForNewObject(types.Dict{"Names": names})
	if err != nil {
		return err
	}
	tree["IDTree"] = *idTree
	tree["K"] = refs[s.root]
	tree["ParentTree"] = types.Dict{"Nums": nums}
	tree["ParentTreeNextKey"] = types.Integer(p.PageCount)
	tree["Namespaces"] = types.Array{*ns}
	catalog["StructTreeRoot"] = *treeRef
	catalog["MarkInfo"] = types.Dict{"Marked": types.Boolean(true)}
	v := model.V20
	p.HeaderVersion = &v
	p.RootVersion = &v
	catalog["Version"] = types.Name("2.0")
	return nil
}

// Structure IDs and IDTree keys are byte strings, unlike Alt text strings.

// ActualText was introduced in PDF 1.4. gofpdf has no public minimum
// version setter and otherwise emits 1.3 for opaque documents. This
// equal-length header promotion leaves every object/xref offset unchanged.

func pdfOutputID(id string) types.HexLiteral { return types.HexLiteral(hex.EncodeToString([]byte(id))) }

var pdfFonts = []struct {
	style string
	data  []byte
}{{"", goregular.TTF}, {"B", gobold.TTF}, {"I", goitalic.TTF}, {"BI", gobolditalic.TTF}}
var pdfFontOnce sync.Once
var parsedPDFFonts map[string]*sfnt.Font
var pdfFontError error

func runStyle(r Run) string {
	s := ""
	if r.Bold {
		s += "B"
	}
	if r.Italic {
		s += "I"
	}
	return s
}
func validatePDFText(r Run) error {
	pdfFontOnce.Do(func() {
		parsedPDFFonts = map[string]*sfnt.Font{}
		for _, f := range pdfFonts {
			p, err := sfnt.Parse(f.data)
			if err != nil {
				pdfFontError = err
				return
			}
			parsedPDFFonts[f.style] = p
		}
	})
	if pdfFontError != nil {
		return pdfFontError
	}
	return validateFontText(r, parsedPDFFonts[runStyle(r)])
}
func validateFontText(r Run, font *sfnt.Font) error {

	var buf sfnt.Buffer
	for _, ch := range norm.NFC.String(r.Text) {
		if ch == 0x200c || ch == 0x200d {
			continue
		}
		if pdfSpecial(ch) {
			required := []rune{' '}
			switch ch {
			case 0xad, 0x2010, 0x2011:
				required = append(required, '-')
			case 0x2007:
				required = append(required, '0')
			case 0x2008:
				required = append(required, '.')
			}
			for _, value := range required {
				glyph, err := font.GlyphIndex(&buf, value)
				if err != nil {
					return err
				}
				if glyph == 0 {
					return fmt.Errorf("%w: font lacks replacement/advance glyph %U for %U", ErrUnsupported, value, ch)
				}
			}
			continue
		}
		if unicode.In(ch, unicode.Devanagari, unicode.Bengali, unicode.Gurmukhi, unicode.Gujarati, unicode.Oriya, unicode.Tamil, unicode.Telugu, unicode.Kannada, unicode.Malayalam, unicode.Sinhala, unicode.Myanmar, unicode.Khmer, unicode.Thai, unicode.Lao, unicode.Tibetan, unicode.Mongolian) {
			return fmt.Errorf("%w: PDF output needs broader script shaping for %U", ErrUnsupported, ch)
		}

		g, err := font.GlyphIndex(&buf, ch)
		if err != nil {
			return err
		}
		if g == 0 {
			return fmt.Errorf("%w: embedded PDF font has no glyph for %U", ErrUnsupported, ch)
		}
	}
	return nil
}

type pdfSpan struct {
	citation        *pdfCitationBox
	bidi            pdfBidiRange
	placed          bool
	visualX         float64
	run             Run
	text            string
	width           float64
	ascent, descent float64
	glyphs          *pdfGlyphCluster
}
type pdfLine []pdfSpan
type pdfRenderer struct {
	ctx                      context.Context
	outputStructure          *pdfOutputStructure
	suppressOutputStructure  bool
	destinationPoints        map[string]pdfDestinationPoint
	destinationTokens        map[string]string
	destinationURIs          map[string]string
	pdf                      *gofpdf.Fpdf
	width, height, margin, y float64
	images                   int
	header, footer           *pdfRunningStory
	content                  *pdfBodyBounds
	page                     *pageGeometry
	paintingRunning          bool
	sectioned                bool
	minimumBody              float64
	runningImages            map[[32]byte]string
	notes                    *noteCatalog
	noteLinks                map[noteKey]int
	seenFootnotes            map[noteKey]bool
	pageNotes, pendingNotes  []pdfNoteChunk
	noteReserve              float64
	validatedImages          outputImageCache
	fonts                    map[string]pdfFontFace
	fontMap                  map[string]string
	glyphFaces               map[string]*pdfGlyphFace
}

func writePDF(ctx context.Context, out io.Writer, d *Document) error {
	return writePDFWithFonts(ctx, out, d, nil, nil)
}
func writePDFWithFonts(ctx context.Context, out io.Writer, d *Document, fonts []FontFace, fontMap map[string]string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	d, structure := preparePDFOutputStructure(d)
	notes, err := catalogNotes(d, true)
	if err != nil {
		return err
	}
	g := pageLayout(d.Layout)
	if hasSectionBlocks(d.Blocks) {
		g = pageLayout(documentSections(d)[0].layout)
	}
	w, h, m := g.width, g.height, g.margins.Left
	if fmt.Sprintf("%.2f", w) == "0.00" || fmt.Sprintf("%.2f", h) == "0.00" {
		return fmt.Errorf("%w: PDF page dimensions round to zero", ErrUnsupported)
	}
	if w > 14400 || h > 14400 {
		return fmt.Errorf("%w: PDF page dimensions exceed 14400pt without UserUnit", ErrUnsupported)
	}
	p := gofpdf.NewCustom(&gofpdf.InitType{UnitStr: "pt", Size: gofpdf.SizeType{Wd: w, Ht: h}})
	p.SetMargins(g.margins.Left, math.Abs(g.margins.Top), g.margins.Right)
	p.SetAutoPageBreak(false, math.Abs(g.margins.Bottom))
	p.SetTitle(d.Metadata.Title, true)
	p.SetAuthor(d.Metadata.Author, true)
	p.SetSubject(d.Metadata.Subject, true)
	r := &pdfRenderer{outputStructure: structure, ctx: ctx, pdf: p, width: w, height: h, margin: m, page: &g, y: math.Abs(g.margins.Top), validatedImages: make(outputImageCache)}
	if err := r.prepareDestinations(d); err != nil {
		return err
	}
	r.fontMap = map[string]string{}
	for from, to := range fontMap {
		if from == "" || to == "" {
			return fmt.Errorf("empty PDF font mapping")
		}
		if err := validateTextStyle(Run{FontFamily: from}); err != nil {
			return err
		}
		if err := validateTextStyle(Run{FontFamily: to}); err != nil {
			return err
		}
		key := fontFamilyKey(from)
		if _, ok := r.fontMap[key]; ok {
			return fmt.Errorf("duplicate PDF font mapping")
		}
		r.fontMap[key] = to
	}
	if err := r.addFonts(fonts); err != nil {
		return err
	}

	if len(notes.order) > 0 {
		if _, err := r.registerFont(fontKey("Go", "")); err != nil {
			return err
		}
	}
	r.notes = notes
	r.noteLinks = map[noteKey]int{}
	r.seenFootnotes = map[noteKey]bool{}
	for _, key := range notes.order {
		r.noteLinks[key] = p.AddLink()
	}
	if hasSectionBlocks(d.Blocks) {
		if err := r.renderSections(d); err != nil {
			return err
		}
	} else {
		if len(d.Headers) > 0 || len(d.Footers) > 0 {
			minimum, err := r.minimumRunningBody(d.Blocks)
			if err != nil {
				return err
			}
			r.minimumBody = minimum
		}
		if err := r.prepareRunning(d.Headers, d.Footers, len(d.Blocks) > 0); err != nil {
			return err
		}
		footerBehind := g.margins.Bottom < 0
		p.SetHeaderFunc(func() {
			r.y = r.bodyTop()
			r.paintRunning(r.header)
			if footerBehind {
				r.paintRunning(r.footer)
			}
		})
		p.SetFooterFunc(func() {
			if !footerBehind {
				r.paintRunning(r.footer)
			}
		})

		p.AddPage()
		if err := r.blocks(d.Blocks, r.bodyWidth(), r.bodyLeft()); err != nil {
			return err
		}
	}
	if err := r.finishNotes(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if p.Error() != nil {
		return p.Error()
	}
	var b bytes.Buffer
	if err := p.Output(&b); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	if bytes.HasPrefix(b.Bytes(), []byte("%PDF-1.3")) {
		b.Bytes()[7] = '4'
	}
	complete, err := r.finishGlyphFonts(b.Bytes())
	if err != nil {
		return err
	}
	_, err = io.Copy(out, bytes.NewReader(complete))
	return err
}
func (r *pdfRenderer) ensure(height float64) error { return r.ensureWithNotes(height, nil) }

func (r *pdfRenderer) blocks(bs []Block, width, x float64) error {
	for _, b := range legacyLists(bs) {
		if err := r.ctx.Err(); err != nil {
			return err
		}
		switch b.Kind {
		case AnchorPoint:
			r.bindDestinations(b.Anchors, r.y)
		case PageBreak:
			r.bindDestinations(b.Anchors, r.y)
			if err := r.nextPage(true); err != nil {
				return err
			}
		case Paragraph, Heading:
			if err := r.paragraphBreakBefore(b); err != nil {
				return err
			}
			size := 11.0
			runs := append([]Run(nil), b.Runs...)
			if b.Kind == Heading {
				size = 26 - float64(b.Level)*2
				for i := range runs {
					runs[i].Bold = true
				}
			}

			lines, err := r.linesDirection(runs, width, size, b.Direction)
			if err != nil {
				return err
			}
			leading := pdfLineHeight(lines[0], size, 1.4)
			if b.Kind == Heading {
				if err := r.ensure(math.Min(leading*2, r.bodyHeight())); err != nil {
					return err
				}
			}
			for lineIndex, line := range lines {
				leading = pdfLineHeight(line, size, 1.4)
				if err := r.ensureWithNotes(leading, lineNoteKeys(line)); err != nil {
					return err
				}
				if lineIndex == 0 {
					r.bindDestinations(b.Anchors, r.y)
				}
				r.drawLine(line, x, r.y, size)
				r.y += leading
			}
			r.y += size * .4
		case ListBlock:
			if err := r.list(b, width, x); err != nil {
				return err
			}
		case Table:
			if err := r.table(b, width, x); err != nil {
				return err
			}
			r.y += 8
		case Picture:
			im, err := r.measureImage(b.Image, width, r.bodyHeight()-8)
			if err != nil {
				return err
			}
			if err := r.ensure(im.height + 8); err != nil {
				return err
			}
			r.bindDestinations(b.Anchors, r.y)
			if err := r.drawImage(im, x, r.y); err != nil {
				return err
			}
			r.y += im.height + 8
		}
	}
	return r.pdf.Error()
}

func (r *pdfRenderer) paragraphBreakBefore(b Block) error {
	if b.PageBreakBefore && r.y > r.bodyTop()+1e-7 {
		return r.nextPage(true)
	}
	return nil
}
