package manydoc

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/benoitkugler/textlayout/fonts/glyphsnames"
	"github.com/benoitkugler/textlayout/fonts/simpleencodings"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// ReadPDF recovers positioned text/images and supported tagged logical structure.
// Untagged structure is inferred from geometry with explicit uncertainty. OCR is
// not provided by this reader.
func ReadPDF(ctx context.Context, src io.Reader, opts ReadOptions) (doc *Document, err error) {
	// A dependency parser's panic must never escape this public boundary or
	// yield partially extracted content as a successful document.
	defer func() {
		if p := recover(); p != nil {
			doc = nil
			err = fmt.Errorf("malformed PDF: %v", p)
		}
	}()
	b, err := readContext(ctx, src)
	if err != nil {
		return nil, err
	}
	conf := &model.Configuration{Reader15: true, ValidationMode: model.ValidationRelaxed, UserPW: opts.Password, OwnerPW: opts.Password, Offline: true}
	p, err := pdfcpu.ReadWithContext(ctx, bytes.NewReader(b), conf)
	if err != nil {
		return nil, fmt.Errorf("PDF read: %w", err)
	}
	// Accept version-extension conventions during object parsing, but never
	// accept partially decoded corrupt page streams as successful extraction.
	p.XRefTable.ValidationMode = model.ValidationStrict
	r := &pdfReader{ctx: ctx, pdf: p}
	if err := r.validateParserStreams(); err != nil {
		return nil, err
	}
	if err = p.EnsurePageCount(); err != nil {
		return nil, err
	}
	if p.PageCount < 0 {
		return nil, errors.New("negative PDF page count")
	}
	doc = &Document{Metadata: Metadata{Title: p.Title, Author: p.Author, Subject: p.Subject, PageCount: p.PageCount}}
	if p.Info != nil {
		info, e := p.DereferenceDict(*p.Info)
		if e != nil {
			return nil, e
		}
		for _, entry := range []struct {
			key    string
			target *string
		}{{"Title", &doc.Metadata.Title}, {"Author", &doc.Metadata.Author}, {"Subject", &doc.Metadata.Subject}} {
			if value := info[entry.key]; value != nil {
				v, e := p.Dereference(value)
				if e != nil {
					return nil, e
				}
				text, e := pdfTextString(v)
				if e != nil {
					return nil, fmt.Errorf("PDF metadata %s: %w", entry.key, e)
				}
				*entry.target = text
			}
		}
		for _, key := range []string{"Keywords", "Creator", "Producer", "CreationDate", "ModDate"} {
			if info[key] != nil {
				doc.warn("pdf_metadata_extra", "Additional PDF producer/date/keyword metadata is not yet represented")
			}
		}
	}
	doc.warn("pdf_layout", "PDF source geometry is retained; recovered semantic structure and reflowed output do not reproduce the original visual layout")
	catalog, e := p.Catalog()
	if e != nil {
		return nil, e
	}
	if catalog["Metadata"] != nil {
		doc.warn("pdf_xmp_metadata", "XMP metadata is not merged into the editable metadata model")
	}
	r.doc = doc
	r.pageRefs = map[string]int{}
	r.objects = map[pdfObjectKey][]pdfPaintRange{}
	r.links = map[pdfObjectKey]pdfRecoveredLink{}
	r.partialLinks = map[int]bool{}
	for page := 1; page <= p.PageCount; page++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		d, pageRef, attrs, e := p.PageDict(page, false)
		if e != nil {
			return nil, fmt.Errorf("PDF page %d: %w", page, e)
		}
		if pageRef != nil {
			r.pageRefs[fmt.Sprint(*pageRef)] = page
		}
		geometry, e := r.pageGeometry(page, d, attrs)
		if e != nil {
			return nil, e
		}
		doc.Pages = append(doc.Pages, geometry)
		r.page = page
		r.pageInfo = geometry
		r.stream = fmt.Sprintf("page:%d", page)
		data, e := r.pageContent(d)
		if e != nil {
			return nil, e
		}
		r.lines = nil
		r.current = nil
		r.fonts = map[string]*pdfFont{}
		if e = r.content(data, attrs.Resources, 0, initialPDFState()); e != nil {
			return nil, fmt.Errorf("PDF page %d: %w", page, e)
		}
		if ann, e := p.DereferenceArray(d["Annots"]); e != nil {
			return nil, e
		} else {
			for _, a := range ann {
				ad, e := p.DereferenceDict(a)
				if e != nil {
					return nil, e
				}
				if ad == nil {
					continue
				}
				if e := r.readLink(a, ad); e != nil {
					return nil, e
				}
				ap, e := p.DereferenceDict(ad["AP"])
				if e != nil {
					return nil, e
				}
				if ap == nil {
					continue
				}
				appearanceObject := ap["N"]
				o, e := p.Dereference(appearanceObject)
				if e != nil {
					return nil, e
				}
				if states, ok := o.(types.Dict); ok {
					state := ad.NameEntry("AS")
					if state != nil {
						o = states[*state]
						appearanceObject = o
					} else {
						r.doc.warn("annotation_state", "Annotation has multiple appearances without a selected state")
						continue
					}
				}
				if o != nil {
					start := r.order
					if e = r.appearance(appearanceObject, ad, attrs.Resources); e != nil {
						return nil, e
					}
					r.recordObject(a, start, r.order)
				}
			}
		}
		r.newline()
		if page > 1 {
			doc.Blocks = append(doc.Blocks, Block{Kind: PageBreak})
		}
		doc.Blocks = append(doc.Blocks, r.lines...)
	}
	if e := r.recoverPDFNavigation(catalog); e != nil {
		return nil, fmt.Errorf("PDF navigation: %w", e)
	}
	r.geometricLinks()
	if (catalog["StructTreeRoot"] == nil || opts.PDFSkipStructure) && !opts.PDFSkipInference {
		if e := r.inferStructure(); e != nil {
			return nil, fmt.Errorf("PDF inferred structure: %w", e)
		}
	}
	if !opts.PDFSkipStructure {
		if e := r.recoverStructure(catalog["StructTreeRoot"]); e != nil {
			return nil, fmt.Errorf("PDF logical structure: %w", e)
		}
	} else if catalog["StructTreeRoot"] != nil {
		doc.warn("pdf_structure_skipped", "Logical PDF structure was explicitly skipped; positioned paint content is retained")
	}
	r.diagnosePartialLinks()
	r.bindPDFNavigation()
	if strings.TrimSpace(strings.ReplaceAll(doc.Text(), "\f", "")) == "" {
		doc.warn("no_text", "No extractable text was found; scanned/image-only documents require OCR")
	}
	if err := doc.Validate(); err != nil {
		return nil, fmt.Errorf("PDF recovered model: %w", err)
	}
	return doc, nil
}

type pdfReader struct {
	streamBudget             *pdfStreamBudget
	rasterPixels             int64
	contentBytes             int
	xobjectTotal             int
	artifactTotal            int
	logicalBidiLine          bool
	ctx                      context.Context
	pdf                      *model.Context
	doc                      *Document
	fonts                    map[string]*pdfFont
	lines                    []Block
	current                  []Run
	havePosition             bool
	page                     int
	pageInfo                 PDFPage
	stream                   string
	order                    int
	marked                   []pdfMarkedContent
	lastPoint, lastDirection Point
	lastSize                 float64
	pageRefs                 map[string]int
	objects                  map[pdfObjectKey][]pdfPaintRange
	links                    map[pdfObjectKey]pdfRecoveredLink
	partialLinks             map[int]bool
	rulings                  map[int][]pdfRuling
	rulingPatches            map[int][]pdfRulingRect
	rulingOverflow           map[int]bool
}
type pdfTextState struct {
	rulingMask, rulingBlend, rulingTransfer           bool
	font                                              *pdfFont
	size, leading, wordSpace, charSpace, hscale, rise float64
	ctm, tm, lm                                       Matrix
	renderMode                                        int
	metricEstimate                                    bool
	fill                                              [3]float64
	fillUnknown                                       bool
	fillSpace, strokeSpace                            string
	fillValues, strokeValues                          [4]float64
	strokeUnknown                                     bool
	rulingWidth                                       float64
	rulingStrokeAlpha, rulingFillAlpha                float64
	rulingClip                                        *pdfRulingRect
	rulingClipUnknown, rulingEffects                  bool
}

func (r *pdfReader) newline() {
	r.logicalBidiLine = false
	r.havePosition = false
	if len(r.current) > 0 {
		r.lines = append(r.lines, Block{Kind: Paragraph, Runs: r.current, Source: sourceForRuns(r.current)})
		r.current = nil
	}
}
func (r *pdfReader) space(next Run) {
	if len(r.current) == 0 {
		return
	}
	last := &r.current[len(r.current)-1]
	lastRune, _ := utf8.DecodeLastRuneInString(last.Text)
	if last.Text != "" && !unicode.IsSpace(lastRune) {
		copyRun, _ := commonTextStyle(*last, next)
		copyRun.Text = " "
		copyRun.Note = nil
		if last.Source != nil {
			source := *last.Source
			source.Artifacts = commonArtifacts(last.Source, next.Source)
			source.Font = ""
			source.FontSize = 0
			source.RenderMode = 0
			source.FontFamilyEvidence = ""
			source.ColorSpace = ""
			source.ColorComponents = nil
			source.Quad = nil
			source.Matrix = nil
			source.Baseline = nil
			source.Regions = nil
			source.MCID = nil
			source.Tag = ""
			source.Structure = "inferred"
			source.Evidence = "positioned_word_gap"
			source.Confidence = .6
			source.Precision = "synthetic_separator"
			copyRun.Source = &source
		}
		r.current = append(r.current, copyRun)
	}
}
func (r *pdfReader) show(v any, s *pdfTextState) error {
	b, err := pdfString(v)
	if err != nil {
		return err
	}
	if len(b) == 0 {
		return nil
	}
	if s.font == nil {
		return errors.New("PDF text has no selected font")
	}
	if s.font.vertical {
		return fmt.Errorf("%w: vertical PDF font positioning", ErrUnsupported)
	}
	text := ""
	actual := r.actualFrame()
	decoded, decodeErr := s.font.decode(b)
	if actual == nil {
		if decodeErr != nil {
			return decodeErr
		}
		text = decoded
	} else if decodeErr != nil {
		actual.alignmentUnavailable = true
	}
	advance, estimated, err := s.font.displacement(b, s.size, s.charSpace, s.wordSpace, s.hscale)
	if err != nil {
		return err
	}
	source := r.sourceFor(s, advance, estimated)
	style := r.textStyle(s, source)
	r.checkPageBounds(*source.Quad)
	if s.renderMode == 3 || s.renderMode == 7 {
		r.warnPage("pdf_invisible_text", "Invisible/clipping-only text is retained with its render mode in source provenance")
	}
	if estimated {
		r.warnPage("pdf_font_metrics", "Text advance estimated because font metrics/code-to-CID mapping are incomplete or ambiguous")
	}
	if s.font.boundsEstimate {
		r.warnPage("pdf_font_bounds", "Font ascent/descent bounds are estimated")
	}
	begin, end := source.Baseline[0], source.Baseline[1]
	direction := Point{end.X - begin.X, end.Y - begin.Y}
	norm := math.Hypot(direction.X, direction.Y)
	if norm == 0 {
		base := r.pageInfo.Transform.compose(s.ctm).compose(s.tm)
		direction = Point{base[0], base[1]}
		norm = math.Hypot(direction.X, direction.Y)
	}
	if norm > 0 {
		direction.X /= norm
		direction.Y /= norm
	}
	// Component glyph backtracking/mark offsets do not establish logical line
	// breaks inside one ActualText replacement.
	logicalEmpty := actual != nil && *actual.actual == ""
	if !logicalEmpty && r.havePosition && (actual == nil || actual.source == nil) {
		cross := r.lastDirection.X*direction.Y - r.lastDirection.Y*direction.X
		dx, dy := begin.X-r.lastPoint.X, begin.Y-r.lastPoint.Y
		perpendicular := math.Abs(dx*r.lastDirection.Y - dy*r.lastDirection.X)
		if r.lastDirection.X*direction.X+r.lastDirection.Y*direction.Y < .95 || math.Abs(cross) > .02 || perpendicular > max(1, source.FontSize*.4) {
			r.newline()
		} else if dx*r.lastDirection.X+dy*r.lastDirection.Y > max(.5, source.FontSize*.15) {
			uncertain := false
			if actual != nil {
				uncertain = pdfHasBidiText(*actual.actual) || r.logicalBidiLine
			}
			if uncertain {
				r.warnPage("pdf_bidi_spacing", "Whitespace is not inferred between adjacent ActualText bidi spans; original logical strings are retained")
			} else {
				r.space(style)
			}
		}
	}
	if actual != nil {
		actual.clipState |= replacementClipState(source.Precision)
		if !actual.haveStyle {
			actual.style = style
			actual.style.Source = nil
			actual.haveStyle = true
			baseline := *source.Baseline
			actual.baseline = &baseline
		} else {
			var mixed bool
			actual.style, mixed = commonTextStyle(actual.style, style)
			actual.styleMixed = actual.styleMixed || mixed
			if !sameSourcePaint(actual.source, source) {
				actual.source.ColorSpace = ""
				actual.source.ColorComponents = nil
			}
			if actual.style.FontFamily == "" {
				actual.source.FontFamilyEvidence = ""
			}
			first, last := actual.baseline[0], actual.baseline[1]
			dx, dy := last.X-first.X, last.Y-first.Y
			length := math.Hypot(dx, dy)
			if length > 1e-8 {
				actual.noncollinear = actual.noncollinear || (math.Abs((begin.X-first.X)*dy-(begin.Y-first.Y)*dx)/length > .01 || math.Abs((end.X-first.X)*dy-(end.Y-first.Y)*dx)/length > .01)
			} else {
				actual.noncollinear = actual.noncollinear || math.Hypot(begin.X-first.X, begin.Y-first.Y) > .01
			}
			actual.baseline[1] = end
			actual.mixedFont = actual.mixedFont || actual.source.Font != source.Font || math.Abs(actual.source.FontSize-source.FontSize) > 1e-7
		}
		if decodeErr == nil {
			part := style
			part.Text = decoded
			part.Source = source
			actual.fragments = append(actual.fragments, part)
			actual.decoded = append(actual.decoded, decoded...)
		}
		if actual.source == nil {
			copy := *source
			copy.Evidence = "pdf_actualtext"
			copy.Regions = nil
			actual.source = &copy
		}
		actual.source.Artifacts = artifactIDs(actual.source, source.Artifacts...)
		actual.source.Confidence = min(actual.source.Confidence, source.Confidence)
		actual.source.Regions = append(actual.source.Regions, SourceRegion{Page: r.page, Quad: *source.Quad})
	}
	if text != "" {
		r.logicalBidiLine = r.logicalBidiLine || pdfHasBidiText(text)
		r.current = append(r.current, Run{Text: text, Bold: style.Bold, Italic: style.Italic, FontFamily: style.FontFamily, FontSize: style.FontSize, Color: style.Color, Source: source})
	}
	s.advanceText(advance)
	s.metricEstimate = s.metricEstimate || estimated
	if !logicalEmpty {
		r.lastPoint = end
		r.lastDirection = direction
		r.lastSize = source.FontSize
		r.havePosition = true
	}
	return nil
}

func (r *pdfReader) content(data []byte, resources types.Dict, depth int, state pdfTextState) error {
	if err := r.ctx.Err(); err != nil {
		return err
	}
	if len(data) > maxPDFDecodedBytes-r.contentBytes {
		return fmt.Errorf("PDF interpreted syntax exceeds %d bytes", maxPDFDecodedBytes)
	}
	r.contentBytes += len(data)
	spaces, err := r.pdf.DereferenceDict(resources["ColorSpace"])
	if err != nil {
		return err
	}
	for _, key := range []string{"DefaultGray", "DefaultRGB", "DefaultCMYK"} {
		if spaces[key] != nil {
			r.warnPage("pdf_default_color_space", "Default device color-space overrides are not resolved; editable RGB uses base device components as an approximation")
		}
	}
	if depth > 32 {
		return errors.New("cyclic or excessively nested PDF Form XObject")
	}
	input := string(data)
	var args []any
	var saved []pdfTextState
	var path pdfCurrentPath
	markedBase := len(r.marked)
	for input != "" {
		v, err := nextPDFToken(r.ctx, &input)
		if err != nil {
			return err
		}
		if v == nil {
			break
		}
		op, isOp := v.(pdfOperator)
		if !isOp {
			args = append(args, v)
			continue
		}
		need := func(n int) error {
			if len(args) != n {
				return fmt.Errorf("PDF operator %s expects %d operands, got %d", op, n, len(args))
			}
			return nil
		}
		switch string(op) {
		case "BT":
			if err := need(0); err != nil {
				return err
			}
			state.tm = identityMatrix()
			state.lm = identityMatrix()
			state.metricEstimate = false
		case "ET":
			if err := need(0); err != nil {
				return err
			}
		case "Tf":
			if err := need(2); err != nil {
				return err
			}
			name, err := pdfName(args[0])
			if err != nil {
				return err
			}
			state.font, err = r.font(resources, name)
			if err != nil {
				return err
			}
			state.size, err = pdfNumber(args[1])
			if err != nil {
				return err
			}
		case "Tj":
			if err := need(1); err != nil {
				return err
			}
			if err := r.show(args[0], &state); err != nil {
				return err
			}
		case "TJ":
			if err := need(1); err != nil {
				return err
			}
			arr, ok := args[0].(types.Array)
			if !ok {
				return errors.New("TJ operand is not an array")
			}
			for _, a := range arr {
				switch a.(type) {
				case types.StringLiteral, types.HexLiteral:
					if err := r.show(a, &state); err != nil {
						return err
					}
				default:
					n, e := pdfNumber(a)
					if e != nil {
						return e
					}
					state.advanceText(-n * state.size * state.hscale / 1000)
				}
			}
		case "'":
			if err := need(1); err != nil {
				return err
			}
			r.newline()
			state.moveLine(0, -state.leading)
			if err := r.show(args[0], &state); err != nil {
				return err
			}
		case "\"":
			if err := need(3); err != nil {
				return err
			}
			state.wordSpace, err = pdfNumber(args[0])
			if err != nil {
				return err
			}
			state.charSpace, err = pdfNumber(args[1])
			if err != nil {
				return err
			}
			r.newline()
			state.moveLine(0, -state.leading)
			if err := r.show(args[2], &state); err != nil {
				return err
			}
		case "T*":
			if err := need(0); err != nil {
				return err
			}
			r.newline()
			state.moveLine(0, -state.leading)
		case "Td", "TD":
			if err := need(2); err != nil {
				return err
			}
			dx, e := pdfNumber(args[0])
			if e != nil {
				return e
			}
			dy, e := pdfNumber(args[1])
			if e != nil {
				return e
			}
			// A Td immediately after BT positions the first span; its y
			// operand is not proof of a new visual line. show compares the
			// transformed baselines, including across separate text objects.
			state.moveLine(dx, dy)
			if op == "TD" {
				state.leading = -dy
			}
		case "Tm":
			if err := need(6); err != nil {
				return err
			}
			matrix, e := matrixOperands(args)
			if e != nil {
				return e
			}
			state.tm = matrix
			state.lm = matrix
			state.metricEstimate = false
		case "TL", "Tw", "Tc", "Ts", "Tz", "Tr":
			if err := need(1); err != nil {
				return err
			}
			n, e := pdfNumber(args[0])
			if e != nil {
				return e
			}
			switch op {
			case "TL":
				state.leading = n
			case "Tw":
				state.wordSpace = n
			case "Tc":
				state.charSpace = n
			case "Ts":
				state.rise = n
			case "Tz":
				state.hscale = n / 100
			case "Tr":
				if n != math.Trunc(n) || n < 0 || n > 7 {
					return fmt.Errorf("invalid text render mode")
				}
				state.renderMode = int(n)

			}
		case "q":
			if err := need(0); err != nil {
				return err
			}
			saved = append(saved, state)
		case "Q":
			if err := need(0); err != nil {
				return err
			}
			if len(saved) == 0 {
				return errors.New("unbalanced PDF graphics state")
			}
			tm, lm, estimate := state.tm, state.lm, state.metricEstimate
			state = saved[len(saved)-1]
			state.tm, state.lm, state.metricEstimate = tm, lm, estimate
			saved = saved[:len(saved)-1]
		case "Do":
			if err := need(1); err != nil {
				return err
			}
			name, e := pdfName(args[0])
			if e != nil {
				return e
			}
			objects, e := r.pdf.DereferenceDict(resources["XObject"])
			if e != nil {
				return e
			}
			o := objects[name]
			if o == nil {
				return fmt.Errorf("missing PDF XObject %q", name)
			}
			if e = r.form(o, resources, depth+1, state); e != nil {
				return e
			}
		case "BI":
			if err := need(0); err != nil {
				return err
			}
			if err := r.inlineImage(&input, resources, state); err != nil {
				return err
			}
		case "cm":
			if err := need(6); err != nil {
				return err
			}
			matrix, err := matrixOperands(args)
			if err != nil {
				return err
			}
			state.ctm = state.ctm.compose(matrix)
		case "BMC", "BDC":
			n := 1
			if op == "BDC" {
				n = 2
			}
			if err := need(n); err != nil {
				return err
			}
			tag, err := pdfName(args[0])
			if err != nil {
				return err
			}
			var props any
			if n == 2 {
				props = args[1]
			}
			if err := r.beginMarked(tag, props, resources); err != nil {
				return err
			}
		case "EMC":
			if err := need(0); err != nil {
				return err
			}
			if len(r.marked) <= markedBase {
				return fmt.Errorf("unbalanced marked-content end")
			}
			r.endMarked()

		case "cs", "CS":
			if err := r.selectTextColorSpace(args, resources, &state, op == "CS"); err != nil {
				return err
			}
		case "sc", "scn", "SC", "SCN":
			if err := setTextColorValues(args, &state, op == "SC" || op == "SCN"); err != nil {
				return err
			}
		case "g", "rg", "k", "G", "RG", "K":
			stroke := op == "G" || op == "RG" || op == "K"
			space := "DeviceGray"
			if op == "rg" || op == "RG" {
				space = "DeviceRGB"
			}
			if op == "k" || op == "K" {
				space = "DeviceCMYK"
			}
			if stroke {
				state.strokeSpace = space
			} else {
				state.fillSpace = space
			}
			if err := setTextColorValues(args, &state, stroke); err != nil {
				return fmt.Errorf("%s: %w", op, err)
			}

		case "m", "l", "c", "v", "y", "h", "re", "n", "S", "s", "f", "F", "f*", "B", "B*", "b", "b*", "W", "W*", "w":
			if err := r.pathOperator(string(op), args, &path, &state); err != nil {
				return err
			}
		case "sh":
			r.warnPage("pdf_vector_graphics", "Painted vector paths/shading are not represented as editable content")
		case "gs":
			if err := r.rulingGraphicsState(args, resources, &state); err != nil {
				return err
			}
			r.warnPage("pdf_graphics_effects", "Extended graphics-state effects are not reconstructed")
		case "J", "j", "M", "d", "ri", "i", "MP", "DP", "BX", "EX", "d0", "d1":
		default:
			return fmt.Errorf("%w: PDF content operator %q", ErrUnsupported, op)
		}
		args = nil
	}
	if len(saved) > 0 {
		return errors.New("unbalanced PDF graphics save")
	}
	if len(r.marked) != markedBase {
		return errors.New("unbalanced PDF marked content")
	}
	if len(args) > 0 {
		return errors.New("PDF content ends with operands without an operator")
	}
	return nil
}

// Count actual invocations, not unique objects or nesting. A shallow acyclic
// shared Form graph can expand exponentially, including through appearances.
// This parser work bound is independent of application admission or quotas.
const maxPDFXObjectVisits = 200000

func (r *pdfReader) form(o types.Object, parent types.Dict, depth int, state pdfTextState) error {
	if err := r.ctx.Err(); err != nil {
		return err
	}
	if r.xobjectTotal >= maxPDFXObjectVisits {
		return fmt.Errorf("PDF XObject expansion exceeds %d visits", maxPDFXObjectVisits)
	}
	r.xobjectTotal++
	sd, _, err := r.pdf.DereferenceStreamDict(o)
	if err != nil {
		return err
	}
	if sd == nil {
		return errors.New("PDF XObject is not a stream")
	}
	sub := sd.Dict.NameEntry("Subtype")
	if sub == nil {
		return errors.New("PDF XObject subtype missing")
	}
	start := r.order
	defer func() { r.recordObject(o, start, r.order) }()
	if *sub == "Image" {
		return r.imageXObject(sd, o, parent, state)
	}
	if *sub != "Form" {
		return fmt.Errorf("%w: PDF XObject subtype %s", ErrUnsupported, *sub)
	}
	decoded, err := r.decodePDFStream(o, sd)
	if err != nil {
		return err
	}
	resources, err := r.pdf.DereferenceDict(sd.Dict["Resources"])
	if err != nil {
		return err
	}
	if resources == nil {
		resources = parent
	}
	if value := sd.Dict["Matrix"]; value != nil {
		a, err := r.pdf.DereferenceArray(value)
		if err != nil {
			return err
		}
		m, err := matrixPDF(a)
		if err != nil {
			return err
		}
		state.ctm = state.ctm.compose(m)
	}
	if value := sd.Dict["BBox"]; value != nil {
		a, err := r.pdf.DereferenceArray(value)
		if err != nil {
			return err
		}
		if len(a) != 4 {
			return fmt.Errorf("invalid Form BBox")
		}
		var v [4]float64
		for i, x := range a {
			n, e := pdfNumber(x)
			if e != nil || !finite(n) {
				return fmt.Errorf("invalid Form BBox")
			}
			v[i] = n
		}
		m := r.pageInfo.Transform.compose(state.ctm)
		points := []Point{m.point(v[0], v[1]), m.point(v[2], v[1]), m.point(v[2], v[3]), m.point(v[0], v[3]), m.point(v[0], v[1])}
		if clip, ok := rulingRect(points); ok {
			state.rulingClip = intersectRulingClip(state.rulingClip, &clip)
		} else {
			state.rulingClipUnknown = true
		}
	}
	oldStream := r.stream
	r.stream = r.objectID(o, "form")
	defer func() { r.stream = oldStream }()
	r.newline()
	return r.content(decoded, resources, depth+1, state)
}

type pdfFont struct {
	mapping                                                        *unicodeMap
	encoding                                                       simpleencodings.Encoding
	unicode                                                        bool
	bold, italic                                                   bool
	widths                                                         map[uint32]float64
	name                                                           string
	family, familyEvidence                                         string
	ascent, descent, defaultWidth                                  float64
	boundsEstimate, defaultExact, composite, identityCID, vertical bool
	cidWidthOverlap                                                bool
}

func (f *pdfFont) decode(b []byte) (string, error) {
	if f.mapping != nil {
		return f.mapping.decode(b)
	}
	if f.unicode {
		return utf16Text(b)
	}
	var s strings.Builder
	for _, c := range b {
		name := f.encoding[c]
		if name == "" {
			return "", fmt.Errorf("%w: font has no glyph mapping for byte %02x", ErrUnsupported, c)
		}
		v, ok := glyphsnames.GlyphToRune(name)
		if !ok {
			return "", fmt.Errorf("%w: glyph %q", ErrUnsupported, name)
		}
		s.WriteRune(v)
	}
	return s.String(), nil
}
func (r *pdfReader) font(resources types.Dict, name string) (*pdfFont, error) {
	fonts, err := r.pdf.DereferenceDict(resources["Font"])
	if err != nil {
		return nil, err
	}
	obj := fonts[name]
	if obj == nil {
		return nil, fmt.Errorf("missing PDF font %q", name)
	}
	// The indirect font object is authoritative, not the page-local resource name.
	key := fmt.Sprint(obj)
	if cached := r.fonts[key]; cached != nil {
		return cached, nil
	}
	d, err := r.pdf.DereferenceDict(obj)
	if err != nil || d == nil {
		return nil, errors.New("invalid PDF font dictionary")
	}
	f := &pdfFont{encoding: simpleencodings.AdobeStandard, widths: map[uint32]float64{}}
	if widths, e := r.pdf.DereferenceArray(d["Widths"]); e != nil {
		return nil, e
	} else {
		first := 0
		if v := d.IntEntry("FirstChar"); v != nil {
			first = *v
		}
		for i, w := range widths {
			n, e := pdfNumber(w)
			if e != nil {
				return nil, e
			}
			f.widths[uint32(first+i)] = n
		}
	}
	base := d.NameEntry("BaseFont")
	f.name = "unknown"
	if base != nil {
		f.name = *base
		lower := strings.ToLower(*base)
		f.bold = strings.Contains(lower, "bold")
		f.italic = strings.Contains(lower, "italic") || strings.Contains(lower, "oblique")
		if *base == "Symbol" {
			f.encoding = simpleencodings.Symbol
		}
		if *base == "ZapfDingbats" {
			f.encoding = simpleencodings.ZapfDingbats
		}
	}
	if o := d["ToUnicode"]; o != nil {
		sd, _, err := r.pdf.DereferenceStreamDict(o)
		if err != nil {
			return nil, err
		}
		if sd == nil {
			return nil, errors.New("invalid ToUnicode stream")
		}
		decoded, err := r.decodePDFStream(o, sd)
		if err != nil {
			return nil, err
		}
		f.mapping, err = parseUnicodeMap(r.ctx, decoded)
		if err != nil {
			return nil, fmt.Errorf("ToUnicode: %w", err)
		}
		if err := r.fontMetrics(f, d); err != nil {
			return nil, err
		}
		if err := r.normalizeSimpleCMap(f); err != nil {
			return nil, err
		}
		r.fonts[key] = f
		return f, nil
	}
	enc, err := r.pdf.Dereference(d["Encoding"])
	if err != nil {
		return nil, err
	}
	selectEncoding := func(name string) error {
		switch name {
		case "WinAnsiEncoding":
			f.encoding = simpleencodings.WinAnsi
		case "MacRomanEncoding":
			f.encoding = simpleencodings.MacRoman
		case "MacExpertEncoding":
			f.encoding = simpleencodings.MacExpert
		case "StandardEncoding":
			f.encoding = simpleencodings.AdobeStandard
		case "UniGB-UCS2-H", "UniGB-UCS2-V", "UniJIS-UCS2-H", "UniJIS-UCS2-V", "UniCNS-UCS2-H", "UniKS-UCS2-H":
			f.unicode = true
		default:
			return fmt.Errorf("%w: PDF font encoding %q without ToUnicode", ErrUnsupported, name)
		}
		return nil
	}
	switch e := enc.(type) {
	case types.Name:
		if err := selectEncoding(string(e)); err != nil {
			return nil, err
		}
	case types.Dict:
		if b := e.NameEntry("BaseEncoding"); b != nil {
			if err := selectEncoding(*b); err != nil {
				return nil, err
			}
		}
		if arr, err := r.pdf.DereferenceArray(e["Differences"]); err != nil {
			return nil, err
		} else {
			code := 0
			for _, o := range arr {
				switch v := o.(type) {
				case types.Integer:
					code = int(v)
				case types.Name:
					if code < 0 || code > 255 {
						return nil, errors.New("font difference outside byte range")
					}
					f.encoding[code] = string(v)
					code++
				default:
					return nil, errors.New("invalid font difference")
				}
			}
		}
	case nil:
		if subtype := d.NameEntry("Subtype"); subtype != nil && *subtype == "Type0" {
			return nil, fmt.Errorf("%w: composite PDF font without Unicode mapping", ErrUnsupported)
		}
	default:
		return nil, errors.New("invalid PDF font encoding")
	}
	if err := r.fontMetrics(f, d); err != nil {
		return nil, err
	}
	r.fonts[key] = f
	return f, nil
}
