package manydoc

import (
	"fmt"
	"sort"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

type pdfObjectKey struct {
	page   int
	object string
}
type pdfPaintRange struct{ start, end int }

func (r *pdfReader) recordObject(object types.Object, start, end int) {
	if r.objects == nil {
		return
	}
	if _, ok := object.(types.IndirectRef); !ok {
		return
	}
	k := pdfObjectKey{r.page, fmt.Sprint(object)}
	r.objects[k] = append(r.objects[k], pdfPaintRange{start, end})
}

type pdfContentKey struct {
	page   int
	stream string
	mcid   int
}
type pdfContentUnit struct {
	block  int
	run    *Run
	image  *Block
	source *SourceInfo
	used   bool
}
type pdfStructureNode struct {
	kind, tag, role, id, stream, object string
	page, mcid                          int
	kids                                []*pdfStructureNode
	attrs                               map[string]types.Object
	alt, actual                         *string
}
type pdfStructureRecovery struct {
	reader         *pdfReader
	roles, classes types.Dict
	units          []pdfContentUnit
	marked         map[pdfContentKey][]int
	active, seen   map[string]bool
	nodes          int
}

func (r *pdfReader) recoverStructure(root types.Object) error {
	if root == nil {
		return nil
	}
	d, err := r.pdf.DereferenceDict(root)
	if err != nil {
		return err
	}
	if d == nil {
		return fmt.Errorf("structure root is not a dictionary")
	}
	s := &pdfStructureRecovery{reader: r, marked: map[pdfContentKey][]int{}, active: map[string]bool{}, seen: map[string]bool{}}
	s.roles, err = r.pdf.DereferenceDict(d["RoleMap"])
	if err != nil {
		return err
	}
	s.classes, err = r.pdf.DereferenceDict(d["ClassMap"])
	if err != nil {
		return err
	}
	for bi, b := range r.doc.Blocks {
		if b.Kind == Picture {
			copy := b
			s.units = append(s.units, pdfContentUnit{block: bi, image: &copy, source: b.Source})
		}
		for _, run := range b.Runs {
			copy := run
			s.units = append(s.units, pdfContentUnit{block: bi, run: &copy, source: copy.Source})
		}
	}
	for i, u := range s.units {
		if u.source != nil && u.source.MCID != nil {
			key := pdfContentKey{u.source.Page, u.source.Stream, *u.source.MCID}
			s.marked[key] = append(s.marked[key], i)
		}
	}
	// Synthetic gaps only belong to a marked sequence when both neighboring
	// painted runs identify that same sequence. Literal spaces are never dropped.
	for i := 1; i+1 < len(s.units); i++ {
		u := &s.units[i]
		if u.source == nil || u.source.Evidence != "positioned_word_gap" {
			continue
		}
		a, b := s.units[i-1], s.units[i+1]
		if a.source == nil || b.source == nil || a.source.MCID == nil || b.source.MCID == nil {
			continue
		}
		ka, kb := pdfContentKey{a.source.Page, a.source.Stream, *a.source.MCID}, pdfContentKey{b.source.Page, b.source.Stream, *b.source.MCID}
		if ka == kb {
			s.marked[ka] = append(s.marked[ka], i)
		}
	}
	for key, ids := range s.marked {
		sort.Ints(ids)
		s.marked[key] = ids
	}
	kids, err := s.parse(d["K"], 0, fmt.Sprint(root), 0)
	if err != nil {
		return err
	}
	blocks, err := s.children(kids)
	if err != nil {
		return err
	}
	var leftovers []int
	for i, u := range s.units {
		if !u.used && !(u.source != nil && u.source.Evidence == "positioned_word_gap") {
			leftovers = append(leftovers, i)
		}
	}
	if len(leftovers) > 0 {
		s.warn("pdf_structure_unclaimed", "Painted content absent from the logical tree is retained after tagged content in original paint order")
		blocks = append(blocks, s.materialize(leftovers, false, "")...)
	}
	r.doc.Blocks = blocks
	r.doc.warn("pdf_structure_layout", "Logical tree order replaces paint order; physical page boundaries remain in pages and source regions, without invented breaks inside logical paragraphs")
	return nil
}
func (s *pdfStructureRecovery) warn(code, message string) { s.reader.doc.warn(code, message) }
func (s *pdfStructureRecovery) page(value types.Object, inherited int) (int, error) {
	if value == nil {
		return inherited, nil
	}
	ref, ok := value.(types.IndirectRef)
	if !ok {
		return 0, fmt.Errorf("structure Pg must reference a page")
	}
	page := s.reader.pageRefs[fmt.Sprint(ref)]
	if page == 0 {
		return 0, fmt.Errorf("structure Pg references a non-page object")
	}
	return page, nil
}
func (s *pdfStructureRecovery) role(tag string) (string, error) {
	seen := map[string]bool{}
	role := tag
	for i := 0; i < 64; i++ {
		value := s.roles[role]
		if value == nil {
			return role, nil
		}
		if seen[role] {
			return "", fmt.Errorf("cyclic role mapping for %s", tag)
		}
		seen[role] = true
		v, err := s.reader.pdf.Dereference(value)
		if err != nil {
			return "", err
		}
		n, ok := v.(types.Name)
		if !ok {
			s.warn("pdf_structure_namespace", "Namespace-specific role mapping is not interpreted; custom role content is retained")
			return role, nil
		}
		role = string(n)
	}
	return "", fmt.Errorf("role mapping exceeds 64 levels")
}
func (s *pdfStructureRecovery) parse(value types.Object, page int, parent string, depth int) ([]*pdfStructureNode, error) {
	if err := s.reader.ctx.Err(); err != nil {
		return nil, err
	}
	if value == nil {
		return nil, nil
	}
	if depth > 64 {
		return nil, fmt.Errorf("structure nesting exceeds 64")
	}
	s.nodes++
	if s.nodes > 200000 {
		return nil, fmt.Errorf("structure exceeds 200000 entries")
	}
	id := ""
	if ref, ok := value.(types.IndirectRef); ok {
		id = fmt.Sprint(ref)
		if s.active[id] {
			return nil, fmt.Errorf("cyclic structure reference %s", id)
		}
		s.active[id] = true
		defer delete(s.active, id)
	}
	v, err := s.reader.pdf.Dereference(value)
	if err != nil {
		return nil, err
	}
	if arr, ok := v.(types.Array); ok {
		var result []*pdfStructureNode
		for _, child := range arr {
			ns, e := s.parse(child, page, parent, depth+1)
			if e != nil {
				return nil, e
			}
			result = append(result, ns...)
		}
		return result, nil
	}
	if n, ok := v.(types.Integer); ok {
		if n < 0 || page == 0 {
			return nil, fmt.Errorf("MCID requires nonnegative ID and page context")
		}
		return []*pdfStructureNode{{kind: "MCR", page: page, stream: fmt.Sprintf("page:%d", page), mcid: int(n)}}, nil
	}
	d, ok := v.(types.Dict)
	if !ok {
		return nil, fmt.Errorf("invalid structure child")
	}
	page, err = s.page(d["Pg"], page)
	if err != nil {
		return nil, err
	}
	kind := "StructElem"
	if n := d.NameEntry("Type"); n != nil {
		kind = *n
	}
	n := &pdfStructureNode{kind: kind, id: id, page: page, attrs: map[string]types.Object{}}
	switch kind {
	case "MCR":
		mcid, e := s.reader.pdf.DereferenceInteger(d["MCID"])
		if e != nil || mcid == nil || *mcid < 0 || page == 0 {
			return nil, fmt.Errorf("invalid marked-content reference")
		}
		n.mcid = int(*mcid)
		n.stream = fmt.Sprintf("page:%d", page)
		if stream := d["Stm"]; stream != nil {
			ref, ok := stream.(types.IndirectRef)
			if !ok {
				return nil, fmt.Errorf("MCR Stm must be an indirect stream")
			}
			sd, _, e := s.reader.pdf.DereferenceStreamDict(ref)
			if e != nil || sd == nil {
				return nil, fmt.Errorf("MCR Stm is not a stream")
			}
			n.stream = fmt.Sprint(ref)
		}
	case "OBJR":
		ref, ok := d["Obj"].(types.IndirectRef)
		if !ok {
			return nil, fmt.Errorf("OBJR requires an indirect object")
		}
		n.object = fmt.Sprint(ref)
	case "StructElem":
		if id != "" {
			if s.seen[id] {
				return nil, fmt.Errorf("structure element has multiple parents/references: %s", id)
			}
			s.seen[id] = true
		}
		tag := d.NameEntry("S")
		if tag == nil {
			return nil, fmt.Errorf("structure element missing role")
		}
		n.tag = *tag
		n.role, err = s.role(n.tag)
		if err != nil {
			return nil, err
		}
		if p := d["P"]; p != nil && fmt.Sprint(p) != parent {
			return nil, fmt.Errorf("structure parent contradicts K containment for %s", id)
		}
		for _, entry := range []struct {
			key    string
			target **string
		}{{"Alt", &n.alt}, {"ActualText", &n.actual}} {
			if value := d[entry.key]; value != nil {
				text, e := pdfTextString(value)
				if e != nil {
					return nil, e
				}
				*entry.target = &text
			}
		}
		if err = s.attributes(n, d); err != nil {
			return nil, err
		}
		n.kids, err = s.parse(d["K"], page, id, depth+1)
		if err != nil {
			return nil, err
		}
		if value := n.attrs["ListNumbering"]; value != nil {
			inheritPDFListNumbering(n.kids, value)
		}
		for _, key := range []string{"Lang", "E", "T", "NS", "Ref", "Phoneme"} {
			if d[key] != nil {
				s.warn("pdf_structure_properties", "Additional logical language/title/namespace/accessibility properties are not represented")
			}
		}
	default:
		return nil, fmt.Errorf("unexpected structure child type %s", kind)
	}
	return []*pdfStructureNode{n}, nil
}
func (s *pdfStructureRecovery) attributes(n *pdfStructureNode, d types.Dict) error {
	// Class attributes precede local A overrides. Revision suffix integers are
	// validated but historical revision selection is explicitly diagnosed.
	var attrs func(types.Object, int) error
	attrs = func(value types.Object, depth int) error {
		if value == nil {
			return nil
		}
		if depth > 16 {
			return fmt.Errorf("attribute nesting exceeds 16")
		}
		v, e := s.reader.pdf.Dereference(value)
		if e != nil {
			return e
		}
		if arr, ok := v.(types.Array); ok {
			for i := 0; i < len(arr); i++ {
				if e := attrs(arr[i], depth+1); e != nil {
					return e
				}
				if i+1 < len(arr) {
					if revision, ok := arr[i+1].(types.Integer); ok {
						if revision < 0 {
							return fmt.Errorf("negative attribute revision")
						}
						if revision != 0 {
							s.warn("pdf_structure_attribute_revision", "Attribute revision history is not reconstructed; listed attribute values are used")
						}
						i++
					}
				}
			}
			return nil
		}
		a, ok := v.(types.Dict)
		if !ok {
			if stream, ok := v.(types.StreamDict); ok {
				a = stream.Dict
			} else {
				return fmt.Errorf("invalid structure attributes")
			}
		}
		owner := a.NameEntry("O")
		if owner == nil {
			return fmt.Errorf("attribute owner missing")
		}
		for k, v := range a {
			if k == "O" {
				continue
			}
			if (*owner == "Table" && (k == "RowSpan" || k == "ColSpan")) || (*owner == "List" && k == "ListNumbering") {
				n.attrs[k] = v
			} else {
				s.warn("pdf_structure_attributes", "Additional structure layout/style/table/list attributes are not represented")
			}
		}
		return nil
	}
	if c := d["C"]; c != nil {
		v, e := s.reader.pdf.Dereference(c)
		if e != nil {
			return e
		}
		classes, ok := v.(types.Array)
		if !ok {
			classes = types.Array{v}
		}
		for i := 0; i < len(classes); i++ {
			name, ok := classes[i].(types.Name)
			if !ok {
				return fmt.Errorf("invalid attribute class name")
			}
			value := s.classes[string(name)]
			if value == nil {
				return fmt.Errorf("missing attribute class %s", name)
			}
			if e := attrs(value, 0); e != nil {
				return e
			}
			if i+1 < len(classes) {
				if revision, ok := classes[i+1].(types.Integer); ok {
					if revision < 0 {
						return fmt.Errorf("negative class revision")
					}
					if revision != 0 {
						s.warn("pdf_structure_attribute_revision", "Attribute revision history is not reconstructed; listed attribute values are used")
					}
					i++
				}
			}
		}
	}
	return attrs(d["A"], 0)
}
func (s *pdfStructureRecovery) content(n *pdfStructureNode) ([]Block, error) {
	var ids []int
	if n.kind == "MCR" {
		ids = s.marked[pdfContentKey{n.page, n.stream, n.mcid}]
	} else {
		for i, u := range s.units {
			if u.source == nil || (n.page != 0 && u.source.Page != n.page) {
				continue
			}
			ranges := s.reader.objects[pdfObjectKey{u.source.Page, n.object}]
			for _, r := range ranges {
				if u.source.Order > r.start && u.source.Order <= r.end {
					ids = append(ids, i)
					break
				}
			}
		}
	}
	if len(ids) == 0 && n.kind == "OBJR" {
		for key := range s.reader.links {
			if key.object == n.object && (n.page == 0 || key.page == n.page) {
				return nil, nil
			}
		}
	}
	if len(ids) == 0 {
		s.warn("pdf_structure_missing_content", "A logical content reference has no recoverable painted text/image; it remains unresolved")
		return nil, nil
	}
	for _, id := range ids {
		if s.units[id].used {
			return nil, fmt.Errorf("painted content has multiple logical owners")
		}
		s.units[id].used = true
	}
	return s.materialize(ids, true, ""), nil
}
func (s *pdfStructureRecovery) materialize(ids []int, explicit bool, role string) []Block {
	var blocks []Block
	previous := -1
	for _, id := range ids {
		u := s.units[id]
		source := u.source
		if source != nil {
			copy := *source
			source = &copy
			if explicit && source.Structure != "inferred" {
				source.Structure = "explicit"
				if source.Evidence == "pdf_actualtext" {
					source.Evidence = "pdf_structure_content_reference;pdf_actualtext"
				} else {
					source.Evidence = "pdf_structure_content_reference"
				}
			}
			if role != "" {
				source.Role = role
			}
		}
		if u.image != nil {
			b := *u.image
			b.Source = source
			blocks = append(blocks, b)
			previous = -1
			continue
		}
		run := *u.run
		run.Source = source
		if previous != u.block {
			blocks = append(blocks, Block{Kind: Paragraph})
			previous = u.block
		}
		b := &blocks[len(blocks)-1]
		b.Runs = append(b.Runs, run)
		b.Source = sourceForRuns(b.Runs)
	}
	return blocks
}
func sourceFromBlocks(blocks []Block, n *pdfStructureNode) *SourceInfo {
	source := &SourceInfo{Format: PDF, Element: n.id, Tag: n.tag, Role: n.role, Structure: "explicit", Evidence: "pdf_structure_tree", Confidence: 1, Precision: "union_of_content_bounds"}
	boxes := map[int]Quad{}
	var add func(*SourceInfo)
	add = func(src *SourceInfo) {
		if src == nil {
			return
		}
		if src.Order > 0 && (source.Order == 0 || src.Order < source.Order) {
			source.Order = src.Order
		}
		regions := src.Regions
		if len(regions) == 0 && src.Quad != nil {
			regions = []SourceRegion{{src.Page, *src.Quad}}
		}
		for _, region := range regions {
			x0, y0, x1, y1 := quadBounds(region.Quad)
			if q, ok := boxes[region.Page]; ok {
				a, b, c, d := quadBounds(q)
				x0 = min(a, x0)
				y0 = min(b, y0)
				x1 = max(c, x1)
				y1 = max(d, y1)
			}
			boxes[region.Page] = Quad{{x0, y0}, {x1, y0}, {x1, y1}, {x0, y1}}
		}
	}
	var walk func([]Block)
	walk = func(bs []Block) {
		for _, b := range bs {
			add(b.Source)
			for _, r := range b.Runs {
				add(r.Source)
			}
			for _, row := range b.Rows {
				for _, cell := range row.Cells {
					add(cell.Source)
					walk(cell.Blocks)
				}
			}
			for _, item := range b.Items {
				add(item.Source)
				walk(item.Blocks)
			}
		}
	}
	walk(blocks)
	pages := make([]int, 0, len(boxes))
	for page := range boxes {
		pages = append(pages, page)
	}
	sort.Ints(pages)
	for _, page := range pages {
		source.Regions = append(source.Regions, SourceRegion{page, boxes[page]})
	}
	if len(pages) == 1 {
		source.Page = pages[0]
		q := boxes[pages[0]]
		source.Quad = &q
	} else if len(pages) == 0 {
		source.Page = n.page
	}
	return source
}
func (s *pdfStructureRecovery) children(nodes []*pdfStructureNode) ([]Block, error) {
	var blocks []Block
	for _, n := range nodes {
		bs, e := s.node(n)
		if e != nil {
			return nil, e
		}
		blocks = append(blocks, bs...)
	}
	return blocks, nil
}
func isInlineRole(role string) bool {
	return strings.Contains("|Span|Quote|Reference|BibEntry|Code|Link|Annot|Ruby|RB|RT|RP|Warichu|WT|WP|", "|"+role+"|")
}

func inheritPDFListNumbering(nodes []*pdfStructureNode, value types.Object) {
	for _, n := range nodes {
		if n.kind != "StructElem" {
			continue
		}
		current := value
		if own := n.attrs["ListNumbering"]; own != nil {
			current = own
		} else {
			n.attrs["ListNumbering"] = value
		}
		inheritPDFListNumbering(n.kids, current)
	}
}
