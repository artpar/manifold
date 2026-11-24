package manydoc

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

func (s *pdfStructureRecovery) node(n *pdfStructureNode) ([]Block, error) {
	if err := s.reader.ctx.Err(); err != nil {
		return nil, err
	}
	if n.kind != "StructElem" {
		return s.content(n)
	}

	if cached, ok := s.cachedRefs[n]; ok {
		return cached, nil
	}
	if note := s.notes[n]; note != nil {
		return s.recoverNote(n, note)
	}
	switch n.role {
	case "Table":
		return s.table(n)
	case "L":
		return s.list(n)
	}
	blocks, err := s.paragraphChildren(n)
	if err != nil {
		return nil, err
	}
	if n.actual != nil {
		aligned, exact := s.alignActualNoteText(blocks, *n.actual, n)
		if s.alignedActual != nil {
			s.alignedActual[n] = exact
		}
		if exact {
			blocks = aligned
		} else {
			// A prepared note atom must never disappear into a replacement.
			// The preparation pass normally keeps opaque ancestors in flow.
			if err := walkNoteRuns(blocks, func(run Run) error {
				if run.Note != nil {
					return fmt.Errorf("ActualText citation correspondence changed during recovery")
				}
				return nil
			}); err != nil {
				return nil, err
			}
			source := sourceFromBlocks(blocks, n)
			source.Evidence = "pdf_structure_actualtext"
			source.Precision = "replacement_span_bounds"
			replacement := s.structureReplacement(blocks, *n.actual, source)
			var out []Block
			inserted := false
			for _, b := range blocks {
				if b.Kind == Paragraph || b.Kind == Heading {
					if !inserted {
						out = append(out, Block{Kind: Paragraph, Runs: []Run{replacement}, Source: source})
						inserted = true
					}
				} else {
					out = append(out, b)
				}
			}
			if !inserted && *n.actual != "" {
				out = append([]Block{{Kind: Paragraph, Runs: []Run{replacement}, Source: source}}, out...)
			}
			blocks = out
		}
	}
	if n.role == "Link" {
		target, conflict := s.logicalLink(n.kids)
		if conflict {
			s.warn("pdf_link_ownership", "Logical Link element references different URI/internal targets; exact ownership is unresolved")
		} else if !target.empty() {
			for i := range blocks {
				for j := range blocks[i].Runs {
					run := &blocks[i].Runs[j]
					if (run.Link != "" || run.Destination != "") && ((run.Destination != "" && run.Destination != target.destination) || (run.Destination == "" && (run.Link != target.uri || target.destination != ""))) {
						s.warn("pdf_link_ownership", "Logical link ownership contradicts geometric annotation association; logical ownership is used")
					}
					target.apply(run)
					if run.Source != nil {
						copy := *run.Source
						copy.Evidence += ";link_from_structure"
						run.Source = &copy
					}
				}
			}
		}
	}
	if n.role == "Figure" || n.role == "Formula" || n.role == "Form" {
		count := 0
		for i := range blocks {
			if blocks[i].Kind == Picture {
				copy := *blocks[i].Image
				blocks[i].Image = &copy
				if count == 0 && n.alt != nil {
					copy.Alt = *n.alt
					blocks[i].Image = &copy
				}
				if blocks[i].Source != nil {
					src := *blocks[i].Source
					src.Element = n.id
					src.ParentElement = n.parent
					src.Role = n.role
					src.Structure = "explicit"
					src.Evidence = "pdf_structure_tree"
					blocks[i].Source = &src
				}
				count++
			}
		}
		if count > 1 {
			s.warn("pdf_structure_figure_group", "Multi-asset figure grouping is flattened; alternative text is attached to the first image")
		}
		if n.role != "Figure" {
			s.warn("pdf_structure_specialized", "Formula/form semantics are retained as roles but do not have dedicated editable nodes")
		}
		return blocks, nil
	}
	paragraph := pdfParagraphRole(n.role)
	if paragraph {
		if len(blocks) == 0 {
			blocks = []Block{{Kind: Paragraph}}
		}
		blocks = s.mergeParagraphs(blocks)
		for i := range blocks {
			if blocks[i].Kind == Paragraph || blocks[i].Kind == Heading {
				if strings.HasPrefix(n.role, "H") {
					blocks[i].Kind = Heading
					blocks[i].Level = 1
					if len(n.role) == 2 {
						blocks[i].Level = int(n.role[1] - '0')
					} else {
						s.warn("pdf_heading_level", "Generic H heading has no explicit rank; level one is used")
					}
				}
				blocks[i].Source = sourceFromBlocks([]Block{blocks[i]}, n)
			}
		}
	} else if isInlineRole(n.role) {
		if _, preparing := s.noteRefs[n]; n.role != "Span" && n.role != "Link" && !preparing {
			s.warn("pdf_structure_inline", "Specialized inline role is retained in source provenance without dedicated editing semantics")
		}
	} else {
		switch n.role {
		case "Document", "Part", "Sect", "Div", "Art", "LBody":
		default:
			s.warn("pdf_structure_role", "Unrepresented logical container/role is flattened while its content is retained")
		}
	}
	if len(n.refs) > 0 || pdfNoteRole(n) {
		for i := range blocks {
			blocks[i].Source = sourceFromBlocks([]Block{blocks[i]}, n)
		}
	}
	for i := range blocks {
		for j := range blocks[i].Runs {
			run := &blocks[i].Runs[j]
			if run.Source != nil && run.Source.Role == "" {
				copy := *run.Source
				copy.Role = n.role
				copy.Element = n.id
				copy.ParentElement = n.parent
				run.Source = &copy
			}
		}
	}
	return blocks, nil
}

// Structural ActualText replaces characters, not homogeneous formatting. A
// mixed replacement has no exact character-to-style mapping; retain common
// marks and disclose the ambiguity, as for content-stream ActualText.
func (s *pdfStructureRecovery) structureReplacement(blocks []Block, text string, source *SourceInfo) Run {
	out := Run{Text: text, Source: source}
	count := 0
	var first *SourceInfo
	mixedFont := false
	mixedPaint := false
	for _, b := range blocks {
		if b.Kind != Paragraph && b.Kind != Heading {
			continue
		}
		for _, r := range b.Runs {
			if r.Source != nil && r.Source.Precision == "synthetic_separator" {
				continue
			}
			if count == 0 {
				out.Bold, out.Italic, out.Underline, out.Strike, out.Link = r.Bold, r.Italic, r.Underline, r.Strike, r.Link
				out.Destination = r.Destination
				out.FontFamily, out.FontSize, out.Color = r.FontFamily, r.FontSize, r.Color
				first = r.Source
			} else {
				var styleMixed bool
				out, styleMixed = commonTextStyle(out, r)
				if styleMixed {
					s.warn("pdf_actualtext_style", "ActualText covers mixed formatting that cannot be mapped exactly to replacement characters")
				}
				mixedFont = mixedFont || first == nil || r.Source == nil || first.Font != r.Source.Font || math.Abs(first.FontSize-r.Source.FontSize) > 1e-7
				mixedPaint = mixedPaint || !sameSourcePaint(first, r.Source)
			}
			if r.Source != nil {
				source.Confidence = min(source.Confidence, r.Source.Confidence)
			}
			count++
		}
	}
	if first != nil && !mixedFont {
		source.Font, source.FontSize = first.Font, first.FontSize
		source.FontFamilyEvidence = first.FontFamilyEvidence
		if count == 1 && source.Page == first.Page && first.Baseline != nil {
			baseline := *first.Baseline
			source.Baseline = &baseline
		}
	}
	if first != nil && !mixedPaint {
		source.ColorSpace = first.ColorSpace
		source.ColorComponents = append([]float64(nil), first.ColorComponents...)
	}
	if mixedFont {
		s.warn("pdf_actualtext_style", "ActualText covers different fonts or sizes; a single font metric is not assigned to the replacement")
	}
	return out
}

func (s *pdfStructureRecovery) mergeParagraphs(blocks []Block) []Block {
	var out []Block
	for _, b := range blocks {
		if len(out) == 0 || b.Kind != Paragraph || out[len(out)-1].Kind != Paragraph || explicitPDFParagraph(b) || explicitPDFParagraph(out[len(out)-1]) {
			out = append(out, b)
			continue
		}
		s.joinParagraph(&out[len(out)-1], b)
	}
	return out
}

func explicitPDFParagraph(b Block) bool {
	return b.Source != nil && b.Source.Structure == "explicit" && pdfParagraphRole(b.Source.Role)
}

func pdfParagraphRole(role string) bool {
	return role == "P" || role == "H" || (len(role) == 2 && role[0] == 'H' && role[1] >= '1' && role[1] <= '6') || role == "Caption" || role == "Lbl"
}

// Painted lines and inline structure elements do not define paragraph breaks.
// A container's consecutive inline content forms an implied paragraph, while
// explicit paragraphs and nontext blocks remain barriers, even through wrappers.
func (s *pdfStructureRecovery) paragraphChildren(n *pdfStructureNode) ([]Block, error) {
	var out, pending []Block
	flush := func() {
		blocks := s.mergeParagraphs(pending)
		for i := range blocks {
			if blocks[i].Kind != Paragraph || explicitPDFParagraph(blocks[i]) {
				continue
			}
			src := sourceFromBlocks([]Block{blocks[i]}, n)
			src.Structure = "inferred"
			src.Evidence = "pdf_structure_inline_paragraph"
			src.Confidence = .75
			blocks[i].Source = src
		}
		out = append(out, blocks...)
		pending = nil
	}
	for _, child := range n.kids {
		blocks, err := s.node(child)
		if err != nil {
			return nil, err
		}
		if child.kind != "StructElem" || isInlineRole(child.role) {
			pending = append(pending, blocks...)
		} else {
			flush()
			out = append(out, blocks...)
		}
	}
	flush()
	return out, nil
}

func (s *pdfStructureRecovery) joinParagraph(last *Block, b Block) {
	if len(last.Runs) > 0 && len(b.Runs) > 0 && needsPDFSeparator(last.Runs[len(last.Runs)-1], b.Runs[0]) {
		src := sourceFromBlocks([]Block{*last, b}, &pdfStructureNode{})
		src.Structure = "inferred"
		src.Evidence = "tagged_paragraph_word_gap"
		src.Artifacts = commonArtifacts(last.Runs[len(last.Runs)-1].Source, b.Runs[0].Source)
		src.Confidence = .6
		src.Quad = nil
		src.Regions = nil
		src.Precision = "synthetic_separator"
		gap, _ := commonTextStyle(last.Runs[len(last.Runs)-1], b.Runs[0])
		gap.Text = " "
		gap.Source = src
		last.Runs = append(last.Runs, gap)
	}
	// Container Ref edges live on block sources as well as runs. Retain both
	// original sources before flattening a wrapper into the merged paragraph.
	source := sourceFromBlocks([]Block{*last, b}, &pdfStructureNode{})
	last.Runs = append(last.Runs, b.Runs...)
	// Do not union different pages into the first page's coordinate system.
	last.Source = source
	last.Source.Structure = "inferred"
	last.Source.Evidence = "pdf_structure_inline_paragraph"
	last.Source.Confidence = .75
}
func needsPDFSeparator(a, b Run) bool {
	if a.Text == "" || b.Text == "" || unicode.IsSpace([]rune(a.Text)[len([]rune(a.Text))-1]) || unicode.IsSpace([]rune(b.Text)[0]) {
		return false
	}
	// Discretionary hyphens and explicit word-boundary controls already convey
	// the logical join. A change of painted baseline must not invent a space.
	left, right := []rune(a.Text), []rune(b.Text)
	if strings.ContainsRune("\u00ad\u200b\u2060\ufeff", left[len(left)-1]) || strings.ContainsRune("\u00ad\u200b\u2060\ufeff", right[0]) {
		return false
	}
	x, y := a.Source, b.Source
	if x == nil || y == nil {
		return false
	}
	if x.Page != y.Page {
		return true
	}
	if x.Baseline != nil && y.Baseline != nil {
		end, start := x.Baseline[1], y.Baseline[0]
		dir := Point{x.Baseline[1].X - x.Baseline[0].X, x.Baseline[1].Y - x.Baseline[0].Y}
		length := math.Hypot(dir.X, dir.Y)
		if length > 0 {
			dir.X /= length
			dir.Y /= length
			dx, dy := start.X-end.X, start.Y-end.Y
			return math.Abs(dx*dir.Y-dy*dir.X) > max(1, x.FontSize*.4) || dx*dir.X+dy*dir.Y > max(.5, x.FontSize*.15)
		}
	}
	if x.Quad != nil && y.Quad != nil {
		a0, b0, a1, b1 := quadBounds(*x.Quad)
		c0, d0, _, d1 := quadBounds(*y.Quad)
		return d0 > b1 || d1 < b0 || (c0 > a1+max(.5, (b1-b0)*.15)) || c0 < a0-1
	}
	return false
}
func (s *pdfStructureRecovery) table(n *pdfStructureNode) ([]Block, error) {
	var rows []*pdfStructureNode
	valid := true
	var gather func([]*pdfStructureNode)
	gather = func(nodes []*pdfStructureNode) {
		for _, child := range nodes {
			switch child.role {
			case "TR":
				rows = append(rows, child)
			case "THead", "TBody", "TFoot":
				gather(child.kids)
			default:
				valid = false
			}
		}
	}
	gather(n.kids)
	for _, row := range rows {
		for _, cell := range row.kids {
			if cell.role != "TD" && cell.role != "TH" {
				valid = false
			}
		}
	}
	if !valid {
		s.warn("pdf_structure_table", "Irregular table structure is flattened without discarding content")
		return s.children(n.kids)
	}
	table := Block{Kind: Table}
	for _, row := range rows {
		rr := Row{}
		for _, cell := range row.kids {
			blocks, err := s.paragraphChildren(cell)
			if err != nil {
				return nil, err
			}
			if len(blocks) == 0 {
				blocks = []Block{{Kind: Paragraph}}
			}
			span := func(key string) (int, error) {
				value := cell.attrs[key]
				if value == nil {
					return 1, nil
				}
				v, e := s.reader.pdf.DereferenceInteger(value)
				if e != nil || v == nil || *v < 1 || *v > 1000 {
					return 0, fmt.Errorf("invalid tagged table %s", key)
				}
				return int(*v), nil
			}
			cs, err := span("ColSpan")
			if err != nil {
				return nil, err
			}
			rs, err := span("RowSpan")
			if err != nil {
				return nil, err
			}
			if rs == 1 {
				rs = 0
			}
			rr.Cells = append(rr.Cells, Cell{ColSpan: cs, RowSpan: rs, Blocks: blocks, Source: sourceFromBlocks(blocks, cell)})
		}
		table.Rows = append(table.Rows, rr)
	}
	table.Source = sourceFromBlocks([]Block{table}, n)
	if recovered, ok := s.recoverTableSpans(table, rows); ok {
		table = recovered
	}
	if err := s.reader.ctx.Err(); err != nil {
		return nil, err
	}
	grid, err := tableGrid(table.Rows)
	if err != nil {
		return nil, fmt.Errorf("tagged table grid: %w", err)
	}
	for _, slots := range grid.Slots {
		if len(slots) != grid.Columns {
			// PDF logical rows may omit merge attributes. Keep their explicit
			// membership if painted evidence cannot uniquely recover the grid;
			// do not invent empty cells or discard the table's structure/content.
			table.Source.Evidence = "pdf_structure_tree_ragged_grid"
			s.warn("pdf_structure_table_grid_uncertain", fmt.Sprintf("Tagged table %s has unequal default-span row widths; explicit cells and positions are retained without inventing unproven merged spans", n.id))
			break
		}
	}
	return []Block{table}, nil
}

var pdfDecimalLabel = regexp.MustCompile(`^([0-9]{1,10})([.)]?)$`)

func (s *pdfStructureRecovery) list(n *pdfStructureNode) ([]Block, error) {
	for _, child := range n.kids {
		if child.role != "LI" {
			s.warn("pdf_structure_list", "Irregular list structure is flattened without discarding content")
			return s.children(n.kids)
		}
	}
	var labels [][]Block
	var bodies [][]Block
	for _, item := range n.kids {
		var label, body []Block
		for _, child := range item.kids {
			blocks, e := s.node(child)
			if e != nil {
				return nil, e
			}
			if child.role == "Lbl" {
				label = append(label, blocks...)
			} else {
				body = append(body, blocks...)
			}
		}
		labels = append(labels, label)
		bodies = append(bodies, body)
	}
	if len(bodies) == 0 {
		return nil, nil
	}
	numbering := "None"
	if value := n.attrs["ListNumbering"]; value != nil {
		v, e := s.reader.pdf.Dereference(value)
		if e != nil {
			return nil, e
		}
		name, ok := v.(types.Name)
		if !ok {
			return nil, fmt.Errorf("invalid ListNumbering")
		}
		numbering = string(name)
	}
	kind := ""
	switch numbering {
	case "Decimal", "UpperRoman", "LowerRoman", "UpperAlpha", "LowerAlpha":
		kind = "number"
	case "Disc", "Circle", "Square":
		kind = "bullet"
	}
	texts := make([]string, len(labels))
	allDecimal, allBullet := true, true
	textOnlyLabels, markerPresentation := true, false
	start := -1
	for i, label := range labels {
		var text strings.Builder
		for _, b := range label {
			if b.Kind != Paragraph || len(b.Anchors) != 0 {
				allDecimal = false
				allBullet = false
				textOnlyLabels = false
			}
			for _, r := range b.Runs {
				if r.Note != nil || r.Anchor != "" || r.Link != "" || r.Destination != "" {
					textOnlyLabels = false
				}
				markerPresentation = markerPresentation || r.Bold || r.Italic || r.Underline || r.Strike || r.Color != "" && r.Color != "#000000" && r.Color != "auto"
				text.WriteString(r.Text)
			}
		}
		texts[i] = strings.TrimSpace(text.String())
		match := pdfDecimalLabel.FindStringSubmatch(texts[i])
		if match == nil {
			allDecimal = false
		} else {
			v, e := strconv.ParseInt(match[1], 10, 32)
			if e != nil {
				allDecimal = false
			} else {
				if start < 0 {
					start = int(v)
				}
				if int(v) != start+i {
					allDecimal = false
				}
			}
		}
		if texts[i] != "•" && texts[i] != "◦" && texts[i] != "▪" && texts[i] != "●" {
			allBullet = false
		}
	}
	pattern, recoveredPattern := pdfListNumberingPattern(numbering, texts)
	constantPattern := false
	if !recoveredPattern {
		pattern, constantPattern = pdfConstantListNumbering(numbering, texts)
		recoveredPattern = constantPattern
	}
	// Decimal fallback only applies to its declared family or absent attributes.
	// Conflicting explicit Roman/Alpha evidence must not become decimal counters.
	if numbering != "Decimal" && numbering != "None" {
		allDecimal = false
	}
	if !textOnlyLabels {
		recoveredPattern = false
		allDecimal = false
		allBullet = false
	}
	inferred := false
	if recoveredPattern {
		kind = "number"
		start = pattern.start
		inferred = numbering == "None"
	}
	if kind == "" && numbering == "None" {
		if allDecimal {
			kind = "number"
			inferred = true
		} else if allBullet {
			kind = "bullet"
			inferred = true
		}
	}
	if (kind == "number" && (!(allDecimal || recoveredPattern) || start < 0 || int64(start)+int64(len(bodies))-1 > 2147483647)) || (kind == "bullet" && !allBullet) {
		kind = ""
	}
	if kind == "" {
		return s.literalList(n, numbering, labels, bodies, texts)
	}

	if markerPresentation {
		s.warn("pdf_list_label_presentation", "List marker typography is not an editable numbering property; original label text and physical style evidence are retained while marker appearance may normalize on reflow")
	}
	list := Block{Kind: ListBlock, List: kind}
	if kind == "number" {
		list.Start = listStartPointer(start)
		if recoveredPattern {
			list.Numbering = pattern.numbering
		}
	}
	for i, body := range bodies {
		if len(body) == 0 || (body[0].Kind != Paragraph && body[0].Kind != Heading) {
			body = append([]Block{{Kind: Paragraph}}, body...)
		}
		src := sourceFromBlocks(append(append([]Block{}, labels[i]...), body...), n.kids[i])
		src.Label = texts[i]

		if kind == "number" && list.Numbering == nil && texts[i] != strconv.Itoa(start+i)+"." {
			s.warn("pdf_list_label_normalized", "Ordered list punctuation is normalized to decimal dot markers; source labels are retained")
		}
		if kind == "bullet" && texts[i] != "•" {
			s.warn("pdf_list_label_normalized", "Bullet marker shape is normalized; source labels are retained")
		}
		list.Items = append(list.Items, ListItem{Blocks: body, Source: src})
	}
	list.Source = sourceFromBlocks([]Block{list}, n)
	if inferred {
		list.Source.Evidence = "pdf_structure_tree;list_kind_from_labels"
		s.warn("pdf_list_kind_inferred", "List kind is inferred from explicit Lbl text because ListNumbering is None/absent")
	}
	if kind == "number" {
		if constantPattern {
			list.Source.Evidence += ";constant_label_from_display;start_unobserved"
			s.warn("pdf_list_constant_inferred", "Fixed list labels are retained as a constant template; hidden counter/restart intent is unobservable, so the unused start defaults to one")
		} else {
			list.Source.Evidence += ";start_from_label"
		}
		if list.Numbering != nil {
			list.Source.Evidence += ";numbering_from_labels"
			if !constantPattern {
				s.warn("pdf_list_numbering_inferred", "Ordered list start and literal pattern are inferred from displayed labels; explicit format attributes select the family when present, and original labels are retained")
			}
		}
	}
	return []Block{list}, nil
}

// Literal labels retain explicit L/LI/Lbl ownership even when no automatic
// counter rule can represent their text, links, images or mixed presentation.
func (s *pdfStructureRecovery) literalList(n *pdfStructureNode, numbering string, labels, bodies [][]Block, texts []string) ([]Block, error) {
	list := Block{Kind: ListBlock, List: "bullet"}
	knownKind := true
	switch numbering {
	case "Decimal", "UpperRoman", "LowerRoman", "UpperAlpha", "LowerAlpha":
		list.List = "number"
	case "Disc", "Circle", "Square":
	default:
		knownKind = false
	}
	for i, body := range bodies {
		if len(body) == 0 || (body[0].Kind != Paragraph && body[0].Kind != Heading) {
			body = append([]Block{{Kind: Paragraph}}, body...)
		}
		marker := labels[i]
		if len(marker) == 0 {
			marker = []Block{{Kind: Paragraph}}
		}
		if err := validateLiteralMarker(marker, 0); err != nil {
			return nil, err
		}
		source := sourceFromBlocks(append(append([]Block(nil), marker...), body...), n.kids[i])
		source.Label = texts[i]
		var markerSource *SourceInfo
		for _, child := range n.kids[i].kids {
			if child.role == "Lbl" {
				if markerSource != nil {
					markerSource = nil
					s.warn("pdf_list_multiple_labels", "Multiple Lbl children retain their source-bearing content in one literal marker without assigning a single source owner")
					break
				}
				markerSource = sourceFromBlocks(marker, child)
			}
		}
		list.Items = append(list.Items, ListItem{Marker: &ListMarker{Blocks: marker, Source: markerSource}, Blocks: body, Source: source})
	}
	list.Source = sourceFromBlocks([]Block{list}, n)
	list.Source.Evidence += ";literal_labels"
	s.warn("pdf_list_literal", "Literal list labels retain their content, relationships and explicit item hierarchy; automatic counter/restart intent is not reconstructed")
	if list.List == "number" {
		list.Source.Evidence += ";start_unobserved"
	}
	if !knownKind {
		list.Source.Evidence += ";list_kind_unobserved"
		s.warn("pdf_list_kind_unobserved", "ListNumbering is absent, None or unrepresented; literal labels preserve display while the list uses an unordered editing container without inventing a counter rule")
	}
	return []Block{list}, nil
}
