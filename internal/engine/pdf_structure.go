package engine

import (
	"bytes"
	"fmt"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	// Exact agreement between decoded inner strings and an outer ActualText string
	// establishes style intervals. Never align merely by rune count or guess a map
	// for a genuinely different replacement string.
	"math"
	"regexp"
	"sort"
	"strconv"

	// PDFArtifact records an explicit marked-content artifact, independently of any
	// inferred reusable header/footer template. BBox uses default user-space units (not the current content CTM); Bounds
	// uses Matrix to map it into displayed physical points. Content refers back here
	// through SourceInfo.Artifacts, including nested marked-content and images.
	"strings"
	"unicode"
	"unicode/utf8"
)

func alignedActualTextRuns(fragments []Run) []Run {
	var out []Run
	for start := 0; start < len(fragments); {
		end := start + 1
		for end < len(fragments) && samePDFStyle(fragments[start], fragments[end]) && fragments[start].Source.Font == fragments[end].Source.Font {
			end++
		}
		run := fragments[start]
		source := *run.Source
		source.Evidence = "pdf_actualtext_aligned"
		source.Regions = nil
		source.Matrix = nil
		var text []byte
		var bounds Quad
		var baseline [2]Point
		noncollinear := false
		clipState := uint8(0)
		for i, fragment := range fragments[start:end] {
			text = append(text, fragment.Text...)
			s := fragment.Source
			clipState |= replacementClipState(s.Precision)
			source.Artifacts = artifactIDs(&source, s.Artifacts...)
			source.Confidence = min(source.Confidence, s.Confidence)
			q := *s.Quad
			source.Regions = append(source.Regions, SourceRegion{Page: s.Page, Quad: q})
			x0, y0, x1, y1 := quadBounds(q)
			if i > 0 {
				a, b, c, d := quadBounds(bounds)
				x0 = min(x0, a)
				y0 = min(y0, b)
				x1 = max(x1, c)
				y1 = max(y1, d)
			}
			bounds = Quad{{x0, y0}, {x1, y0}, {x1, y1}, {x0, y1}}
			if i == 0 {
				baseline = *s.Baseline
			} else {
				a, b := baseline[0], baseline[1]
				dx, dy := b.X-a.X, b.Y-a.Y
				length := math.Hypot(dx, dy)
				if length > 1e-8 {
					for _, p := range *s.Baseline {
						noncollinear = noncollinear || math.Abs((p.X-a.X)*dy-(p.Y-a.Y)*dx)/length > .01
					}
				} else {
					noncollinear = noncollinear || math.Hypot(s.Baseline[0].X-a.X, s.Baseline[0].Y-a.Y) > .01
				}
				baseline[1] = s.Baseline[1]
			}
		}
		run.Text = string(text)
		source.Quad = &bounds
		source.Baseline = nil
		if !noncollinear {
			source.Baseline = &baseline
		}
		source.Precision = clippedReplacementPrecision(clipState)
		run.Source = &source
		if run.Text != "" {
			out = append(out, run)
		}
		start = end
	}
	return out
}

type PDFArtifact struct {
	ID       string      `json:"id"`
	Stream   string      `json:"stream"`
	Type     string      `json:"type,omitempty"`
	Subtype  string      `json:"subtype,omitempty"`
	BBox     *[4]float64 `json:"bbox,omitempty"`
	Matrix   Matrix      `json:"matrix"`
	Bounds   *Quad       `json:"bounds,omitempty"`
	Attached []string    `json:"attached,omitempty"`
}

func (r *pdfReader) artifact(d types.Dict, m Matrix) (string, error) {
	page := &r.doc.Pages[r.page-1]
	a := PDFArtifact{ID: fmt.Sprintf("page:%d:artifact:%d", r.page, len(page.Artifacts)+1), Stream: r.stream, Matrix: m}
	if r.artifactTotal >= 100000 {
		return "", fmt.Errorf("PDF artifact budget exceeded")
	}
	for _, v := range []struct {
		key string
		out *string
	}{{"Type", &a.Type}, {"Subtype", &a.Subtype}} {
		if d[v.key] == nil {
			continue
		}
		o, e := r.pdf.Dereference(d[v.key])
		if e != nil {
			return "", e
		}
		n, ok := o.(types.Name)
		if !ok {
			return "", fmt.Errorf("artifact %s requires a name", v.key)
		}
		*v.out = string(n)
	}
	if d["BBox"] != nil {
		b, e := r.pdf.DereferenceArray(d["BBox"])
		if e != nil {
			return "", e
		}
		if len(b) != 4 {
			return "", fmt.Errorf("artifact BBox requires four coordinates")
		}
		var box [4]float64
		for i, p := range b {
			o, e := r.pdf.Dereference(p)
			if e != nil {
				return "", e
			}
			x, e := pdfNumber(o)
			if e != nil || !finite(x) {
				return "", fmt.Errorf("invalid artifact coordinate")
			}
			box[i] = x
		}
		a.BBox = &box
		q := Quad{m.point(box[0], box[1]), m.point(box[2], box[1]), m.point(box[2], box[3]), m.point(box[0], box[3])}
		a.Bounds = &q
	}
	if d["Attached"] != nil {
		ps, e := r.pdf.DereferenceArray(d["Attached"])
		if e != nil {
			return "", e
		}
		seen := map[string]bool{}
		for _, v := range ps {
			o, e := r.pdf.Dereference(v)
			if e != nil {
				return "", e
			}
			n, ok := o.(types.Name)
			if !ok {
				return "", fmt.Errorf("artifact Attached requires edge names")
			}
			edge := string(n)
			if (edge != "Top" && edge != "Bottom" && edge != "Left" && edge != "Right") || seen[edge] {
				return "", fmt.Errorf("invalid/duplicate artifact attachment")

				// An empty replacement span has no painted geometry, but logical text still persists.
			}
			seen[edge] = true
			a.Attached = append(a.Attached, edge)
		}
	}
	for key := range d {
		if key != "Type" && key != "Subtype" && key != "BBox" && key != "Attached" && key != "MCID" && key != "ActualText" {
			r.warnPage("pdf_artifact_properties", "Additional artifact properties are not represented")
		}
	}
	r.artifactTotal++
	page.Artifacts = append(page.Artifacts, a)
	return a.ID, nil
}
func clonePDFPages(pages []PDFPage) []PDFPage {
	if pages == nil {
		return nil
	}
	out := append([]PDFPage(nil), pages...)
	for i := range out {
		out[i].Artifacts = append([]PDFArtifact(nil), pages[i].Artifacts...)
		for j := range out[i].Artifacts {
			a := &out[i].Artifacts[j]
			a.Attached = append([]string(nil), a.Attached...)
			if a.BBox != nil {
				b := *a.BBox
				a.BBox = &b
			}
			if a.Bounds != nil {
				b := *a.Bounds
				a.Bounds = &b
			}
		}
	}
	return out
}
func artifactIDs(s *SourceInfo, ids ...string) []string {
	var out []string
	if s != nil {
		out = append(out, s.Artifacts...)
	}
	for _, id := range ids {
		found := false
		for _, known := range out {
			found = found || known == id
		}
		if !found {
			out = append(out, id)
		}
	}
	return out
}

func commonArtifacts(a, b *SourceInfo) []string {
	var out []string
	if a == nil || b == nil {
		return nil
	}
	for _, id := range a.Artifacts {
		for _, other := range b.Artifacts {
			if id == other {
				out = append(

					// PDF text strings use PDFDocEncoding unless a Unicode BOM selects UTF-16BE
					// (or UTF-8 in PDF 2.0). This is distinct from font-encoded content strings.
					// Published reference: ISO 32000-1, 7.9.2.2 and Annex D, Table D.2.

					// ParentTree is a reciprocal index, not an alternate source of document order.
					// Missing reciprocal information is disclosed; contradictory ownership rejects.

					out, id)
				break
			}
		}
	}
	return out
}
func (a PDFArtifact) validate() error {
	if destinationID(a.ID) != nil {
		return fmt.Errorf("invalid artifact ID")
	}
	for _, s := range []string{a.Stream, a.Type, a.Subtype} {
		if e := validateString(s); e != nil {
			return e
		}
	}
	for _, v := range a.Matrix {
		if !finite(v) {
			return fmt.Errorf("invalid artifact matrix")
		}
	}
	if (a.BBox == nil) != (a.Bounds == nil) {
		return fmt.Errorf("artifact BBox and display bounds must agree")
	}
	if a.BBox != nil {
		for _, v := range *a.BBox {
			if !finite(v) {
				return fmt.Errorf("invalid artifact BBox")
			}
		}
		b := a.BBox
		expected := Quad{a.Matrix.point(b[0], b[1]), a.Matrix.point(b[2], b[1]), a.Matrix.point(b[2], b[3]), a.Matrix.point(b[0], b[3])}
		for i, p := range *a.Bounds {
			if !finite(p.X) || !finite(p.Y) || p != expected[i] {
				return fmt.Errorf("artifact bounds contradict BBox/matrix")
			}
		}
	}
	seen := map[string]bool{}
	for _, s := range a.Attached {
		if seen[s] || (s != "Top" && s != "Bottom" && s != "Left" && s != "Right") {
			return fmt.Errorf("invalid artifact attachment")
		}
		seen[s] = true
	}
	return nil
}
func (r *pdfReader) actualFrame() *pdfMarkedContent {
	for i := range r.marked {
		if r.marked[i].actual != nil {
			return &r.marked[i]
		}
	}
	return nil
}
func (r *pdfReader) applyMarked(source *SourceInfo) {
	for _, frame := range r.marked {
		if frame.artifact != "" {
			source.Artifacts = artifactIDs(source, frame.artifact)
		}
	}
	if len(r.marked) > 0 {
		source.Tag = r.marked[len(r.marked)-1].tag
	}
	for i := len(r.marked) - 1; i >= 0; i-- {
		if r.marked[i].mcid != nil {
			n := *r.marked[i].mcid
			source.MCID = &n
			source.Stream = r.marked[i].stream
			break
		}
	}
}
func (r *pdfReader) beginMarked(tag string, value any, resources types.Dict) error {
	frame := pdfMarkedContent{tag: tag, stream: r.stream}
	var properties types.Dict
	if value != nil {
		var d types.Dict
		if name, ok := value.(types.Name); ok {
			props, e := r.pdf.DereferenceDict(resources["Properties"])
			if e != nil {
				return e
			}
			if props[string(name)] == nil {
				return fmt.Errorf("missing marked-content property %s", name)
			}
			var err error
			d, err = r.pdf.DereferenceDict(props[string(name)])
			if err != nil {
				return err
			}
		} else {
			var ok bool
			d, ok = value.(types.Dict)
			if !ok {
				return fmt.Errorf("invalid marked-content properties")
			}
		}
		properties = d
		if v := d["MCID"]; v != nil {
			n, e := r.pdf.DereferenceInteger(v)
			if e != nil || n == nil || *n < 0 {
				return fmt.Errorf("invalid MCID")
			}
			i := int(*n)
			frame.mcid = &i
		}
		if v := d["ActualText"]; v != nil {
			text, e := pdfTextString(v)
			if e != nil {
				return e
			}
			frame.actual = &text
		}
	}
	if tag == "Artifact" {
		id, err := r.artifact(properties, r.pageInfo.Transform)
		if err != nil {
			return err
		}
		frame.artifact = id
	}
	r.marked = append(r.marked, frame)
	return nil
}
func (r *pdfReader) endMarked() {
	frame := r.marked[len(r.marked)-1]
	r.marked = r.marked[:len(r.marked)-1]
	if frame.actual == nil || r.actualFrame() != nil {
		return
	}
	if frame.styleMixed && !frame.alignmentUnavailable && string(frame.decoded) == *frame.actual && len(frame.fragments) > 0 {
		r.logicalBidiLine = r.logicalBidiLine || pdfHasBidiText(*frame.actual)
		r.current = append(r.current, alignedActualTextRuns(frame.fragments)...)
		return
	}
	if frame.styleMixed {
		r.warnPage("pdf_actualtext_style", "ActualText covers mixed formatting that cannot be mapped exactly to replacement characters")
	}
	if frame.source == nil {

		r.order++
		frame.source = &SourceInfo{Format: PDF, Page: r.page, Object: r.stream, Order: r.order, Structure: "unstructured", Evidence: "pdf_actualtext", Confidence: 1}
		r.applyMarked(frame.source)
		if frame.mcid != nil {
			frame.source.MCID = frame.mcid
			frame.source.Stream = frame.stream
		}
		frame.source.Tag = frame.tag
		if frame.artifact != "" {
			frame.source.Artifacts = artifactIDs(frame.source, frame.artifact)
		}
	}
	if len(frame.source.Regions) > 0 {
		var bounds Quad
		for i, region := range frame.source.Regions {
			x0, y0, x1, y1 := quadBounds(region.Quad)
			if i > 0 {
				a, b, c, d := quadBounds(bounds)
				x0 = min(x0, a)
				y0 = min(y0, b)
				x1 = max(x1, c)
				y1 = max(y1, d)
			}
			bounds = Quad{{x0, y0}, {x1, y0}, {x1, y1}, {x0, y1}}
		}
		frame.source.Quad = &bounds
		frame.source.Matrix = nil
		frame.source.Baseline = nil
		if !frame.noncollinear && frame.baseline != nil {
			frame.source.Baseline = frame.baseline
		}
		if frame.mixedFont {
			frame.source.Font = ""
			frame.source.FontSize = 0
		}
		frame.source.Precision = clippedReplacementPrecision(frame.clipState)
	}
	if *frame.actual != "" {
		run := frame.style
		run.Text = *frame.actual
		run.Source = frame.source
		r.logicalBidiLine = r.logicalBidiLine || pdfHasBidiText(run.Text)
		r.current = append(r.current, run)
	}
}

func pdfTextString(object any) (string, error) {
	b, err := pdfString(object)
	if err != nil {
		return "", err
	}
	if bytes.HasPrefix(b, []byte{0xfe, 0xff}) {
		return utf16Text(b[2:])
	}
	if bytes.HasPrefix(b, []byte{0xef, 0xbb, 0xbf}) {
		if !utf8.Valid(b[3:]) {
			return "", fmt.Errorf("invalid UTF-8 PDF text string")
		}
		return string(b[3:]), nil
	}
	accents := []rune("˘ˇˆ˙˝˛˚˜")
	symbols := []rune("•†‡…—–ƒ⁄‹›−‰„“”‘’‚™ﬁﬂŁŒŠŸŽıłœšž")
	out := make([]rune, 0, len(b))
	for _, c := range b {
		v := rune(c)
		switch {
		case c >= 24 && c <= 31:
			v = accents[int(c)-24]
		case c >= 128 && c <= 158:
			v = symbols[int(c)-128]
		case c == 160:
			v = '€'
		case c == 127 || c == 159 || c == 173:
			return "", fmt.Errorf("undefined PDFDocEncoding byte %02x", c)
		}
		out = append(out, v)
	}
	return string(out), nil
}

func (s *pdfStructureRecovery) validateParentTree(value types.Object, kids []*pdfStructureNode) error {
	if value == nil {
		s.warn("pdf_parent_tree_missing", "Logical structure has no inverse ParentTree; forward ownership is recovered without reciprocal validation")
		return nil
	}
	entries := map[int]types.Object{}
	visited := map[string]bool{}
	count := 0
	var read func(types.Object, int) (int, int, error)
	read = func(value types.Object, depth int) (int, int, error) {
		if err := s.reader.ctx.Err(); err != nil {
			return 0, 0, err
		}
		count++
		if depth > 64 || count > s.reader.structureLimit() {
			return 0, 0, fmt.Errorf("ParentTree exceeds nesting/entry limit")
		}
		if ref, ok := value.(types.IndirectRef); ok {
			id := fmt.Sprint(ref)
			if visited[id] {
				return 0, 0, fmt.Errorf("ParentTree cyclic or repeated node %s", id)
			}
			visited[id] = true
		}
		d, err := s.reader.pdf.DereferenceDict(value)
		if err != nil || d == nil {
			return 0, 0, fmt.Errorf("ParentTree node is not a dictionary")
		}
		if d["Nums"] != nil && d["Kids"] != nil {
			return 0, 0, fmt.Errorf("ParentTree node has both Nums and Kids")
		}
		min, max := -1, -1
		if d["Nums"] != nil {
			nums, e := s.reader.pdf.DereferenceArray(d["Nums"])
			if e != nil || nums == nil || len(nums)%2 != 0 || len(nums) > 2*s.reader.structureLimit() {
				return 0, 0, fmt.Errorf("invalid ParentTree Nums")
			}
			for i := 0; i < len(nums); i += 2 {
				count++
				if count > s.reader.structureLimit() {
					return 0, 0, fmt.Errorf("ParentTree exceeds entry limit")
				}
				key, e := s.reader.pdf.DereferenceInteger(nums[i])
				if e != nil || key == nil || *key < 0 || int64(*key) > 2147483647 || int(*key) <= max {
					return 0, 0, fmt.Errorf("ParentTree keys must be increasing nonnegative integers")
				}
				if _, found := entries[int(*key)]; found {
					return 0, 0, fmt.Errorf("duplicate ParentTree key")
				}
				if min < 0 {
					min = int(*key)
				}
				max = int(*key)
				entries[max] = nums[i+1]
			}
		} else if d["Kids"] != nil {
			children, e := s.reader.pdf.DereferenceArray(d["Kids"])
			if e != nil || len(children) == 0 || len(children) > s.reader.structureLimit() {
				return 0, 0, fmt.Errorf("invalid ParentTree Kids")
			}
			for _, child := range children {
				if _, ok := child.(types.IndirectRef); !ok {
					if _, direct := child.(types.Dict); !direct {
						return 0, 0, fmt.Errorf("ParentTree child is not a dictionary or indirect reference")
					}
					s.warn("pdf_parent_tree_direct_child", "ParentTree has a direct child dictionary instead of an indirect reference; reciprocal ownership is still checked, without claiming number-tree conformance")
				}
				lo, hi, e := read(child, depth+1)
				if e != nil {
					return 0, 0, e
				}
				if lo < 0 || lo <= max {
					return 0, 0, fmt.Errorf("ParentTree child ranges overlap or are empty")
				}
				if min < 0 {
					min = lo
				}
				max = hi
			}
		} else {
			return 0, 0, fmt.Errorf("ParentTree node has no Nums or Kids")
		}
		if d["Limits"] != nil {
			limits, e := s.reader.pdf.DereferenceArray(d["Limits"])
			if e != nil || len(limits) != 2 {
				return 0, 0, fmt.Errorf("invalid ParentTree Limits")
			}
			lo, e := s.reader.pdf.DereferenceInteger(limits[0])
			if e != nil || lo == nil {
				return 0, 0, fmt.Errorf("invalid ParentTree lower limit")
			}
			hi, e := s.reader.pdf.DereferenceInteger(limits[1])
			if e != nil || hi == nil || int(*lo) != min || int(*hi) != max {
				return 0, 0, fmt.Errorf("ParentTree Limits contradict keys")
			}
		}
		return min, max, nil
	}
	if _, _, err := read(value, 0); err != nil {
		return err
	}
	type slot struct{ key, mcid int }
	expected := map[slot]string{}
	containers := map[int]string{}
	forward := map[pdfContentKey]bool{}
	var walk func([]*pdfStructureNode, string) error
	walk = func(nodes []*pdfStructureNode, owner string) error {
		for _, n := range nodes {
			if err := s.reader.ctx.Err(); err != nil {
				return err
			}
			if n.kind == "StructElem" {
				if err := walk(n.kids, n.id); err != nil {
					return err
				}
				continue
			}
			var d types.Dict
			var container string
			var err error
			field := "StructParents"
			mcid := n.mcid
			if n.contentObject != nil {
				container = fmt.Sprint(n.contentObject)
				obj, e := s.reader.pdf.Dereference(n.contentObject)
				if e != nil {
					return e
				}
				switch obj := obj.(type) {
				case types.Dict:
					d = obj
				case types.StreamDict:
					d = obj.Dict
				default:
					return fmt.Errorf("ParentTree content container is not a dictionary")
				}
			} else {
				var ref *types.IndirectRef
				d, ref, _, err = s.reader.pdf.PageDict(n.page, false)
				if err != nil {
					return err
				}
				if ref != nil {
					container = fmt.Sprint(*ref)
				}
			}
			if n.kind == "OBJR" {
				field = "StructParent"
				mcid = -1
			}
			if d["StructParent"] != nil && d["StructParents"] != nil {
				return fmt.Errorf("content object has both StructParent and StructParents")
			}
			key, e := s.reader.pdf.DereferenceInteger(d[field])
			if e != nil {
				return e
			}
			if key == nil || owner == "" {
				s.warn("pdf_parent_tree_incomplete", "Forward content ownership lacks an indirect owner or reciprocal container key; forward content is retained")
				continue
			}
			if *key < 0 || int64(*key) > 2147483647 {
				return fmt.Errorf("invalid %s key", field)
			}
			if previous, ok := containers[int(*key)]; ok && previous != container {
				return fmt.Errorf("ParentTree key shared by different content containers")
			}
			containers[int(*key)] = container
			pos := slot{int(*key), mcid}
			if previous, ok := expected[pos]; ok && previous != owner {
				return fmt.Errorf("forward content has multiple logical owners")
			}
			if n.kind == "MCR" {
				physical := pdfContentKey{n.page, n.stream, n.mcid}
				if forward[physical] {
					return fmt.Errorf("marked-content sequence is referenced repeatedly")
				}
				forward[physical] = true
			}
			expected[pos] = owner
		}
		return nil
	}
	if err := walk(kids, ""); err != nil {
		return err
	}
	// Validate all listed references, including entries that have no forward owner.
	// Null array slots are allowed for unused or structurally unowned MCIDs.

	// Synthetic gaps only belong to a marked sequence when both neighboring
	// painted runs identify that same sequence. Literal spaces are never dropped.

	ownerSlots := 0
	inverse := map[slot]string{}
	arrayEntries := map[int]bool{}
	ownerRef := func(value types.Object) (string, error) {
		if value == nil {
			return "", nil
		}
		ref, ok := value.(types.IndirectRef)
		if !ok {
			return "", fmt.Errorf("ParentTree owner must be an indirect structure element")
		}
		id := fmt.Sprint(ref)
		if !s.seen[id] {
			return "", fmt.Errorf("ParentTree owner is absent from forward structure")
		}
		return id, nil
	}
	for key, value := range entries {
		obj, e := s.reader.pdf.Dereference(value)
		if e != nil {
			return e
		}
		if array, ok := obj.(types.Array); ok {
			arrayEntries[key] = true
			ownerSlots += len(array)
			if ownerSlots > s.reader.parentTreeSlotLimit() {
				return fmt.Errorf("ParentTree exceeds owner slot limit")
			}
			for mcid, v := range array {
				id, e := ownerRef(v)
				if e != nil {
					return e
				}
				if id != "" {
					inverse[slot{key, mcid}] = id
				}
			}
		} else {
			id, e := ownerRef(value)
			if e != nil {
				return e
			}
			if id == "" {
				return fmt.Errorf("ParentTree object entry is null")
			}
			inverse[slot{key, -1}] = id
		}
	}
	for pos, owner := range expected {
		if _, present := entries[pos.key]; present && arrayEntries[pos.key] != (pos.mcid >= 0) {
			return fmt.Errorf("ParentTree entry shape contradicts content container at key %d", pos.key)
		}
		actual := inverse[pos]
		if actual == "" {
			s.warn("pdf_parent_tree_incomplete", "Forward content lacks a reciprocal ParentTree entry; forward content is retained")
			continue
		}
		if actual != owner {
			return fmt.Errorf("ParentTree contradicts forward content owner at key %d MCID %d", pos.key, pos.mcid)
		}
	}
	for pos := range inverse {
		if _, ok := expected[pos]; !ok {
			s.warn("pdf_parent_tree_unclaimed", "ParentTree contains ownership absent from the forward structure; no additional content or ordering is invented")
		}
	}
	return nil
}

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
	parent                              string
	page, mcid                          int
	kids                                []*pdfStructureNode
	attrs                               map[string]types.Object
	alt, actual                         *string
	refs                                []string
	originalID                          []byte
	contentObject                       types.Object
}
type pdfStructureRecovery struct {
	reader         *pdfReader
	roles, classes types.Dict
	units          []pdfContentUnit
	unitsByPage    map[int][]int
	marked         map[pdfContentKey][]int
	active, seen   map[string]bool
	referenceTotal int
	attributeTotal int
	nodes          int
	grids          map[int][]pdfGridRegion
	notes          map[*pdfStructureNode]*pdfRecoveredNote
	noteRefs       map[*pdfStructureNode]*pdfRecoveredNote
	cachedRefs     map[*pdfStructureNode][]Block
	alignedActual  map[*pdfStructureNode]bool
	noteLabels     map[noteKey]string
	noteOrder      map[noteKey]int
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
	s.unitsByPage = map[int][]int{}
	for i, u := range s.units {
		if u.source != nil {
			s.unitsByPage[u.source.Page] = append(s.unitsByPage[u.source.Page], i)
		}
		if u.source != nil && u.source.MCID != nil {
			key := pdfContentKey{u.source.Page, u.source.Stream, *u.source.MCID}
			s.marked[key] = append(s.marked[key], i)
		}
	}

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
	if err := s.validateParentTree(d["ParentTree"], kids); err != nil {
		return err
	}
	if err := s.prepareNotes(kids); err != nil {
		return err
	}
	blocks, err := s.children(kids)
	if err != nil {
		return err
	}
	sort.SliceStable(r.doc.Notes, func(i, j int) bool {
		return s.noteOrder[noteIdentity(r.doc.Notes[i])] < s.noteOrder[noteIdentity(r.doc.Notes[j])]
	})
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
	r.warn("pdf_structure_layout", "Logical tree order replaces paint order; physical page boundaries remain in pages and source regions, without invented breaks inside logical paragraphs")
	return nil
}
func (s *pdfStructureRecovery) warn(code, message string) { s.reader.warn(code, message) }
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
	if s.nodes > s.reader.structureLimit() {
		return nil, fmt.Errorf("structure exceeds %d entries", s.reader.structureLimit())
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
			n.contentObject = ref
		}
	case "OBJR":
		ref, ok := d["Obj"].(types.IndirectRef)
		if !ok {
			return nil, fmt.Errorf("OBJR requires an indirect object")
		}
		n.object = fmt.Sprint(ref)
		n.contentObject = ref
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
		n.parent = parent
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

		if value := d["ID"]; value != nil {
			value, err = s.reader.pdf.Dereference(value)
			if err != nil {
				return nil, err
			}
			n.originalID, err = pdfString(value)
			if err != nil {
				return nil, err
			}
		}
		if value := d["Ref"]; value != nil {
			refs, e := s.reader.pdf.DereferenceArray(value)
			if e != nil || refs == nil {
				return nil, fmt.Errorf("structure Ref must be an array")
			}
			for _, value := range refs {
				s.referenceTotal++
				if s.referenceTotal > s.reader.structureLimit() {
					return nil, fmt.Errorf("structure Ref graph exceeds %d targets", s.reader.structureLimit())
				}
				ref, ok := value.(types.IndirectRef)
				if !ok {
					return nil, fmt.Errorf("structure Ref target must be indirect")
				}
				n.refs = append(n.refs, fmt.Sprint(ref))
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
		for _, key := range []string{"Lang", "E", "T", "NS", "Phoneme"} {
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
		if err := s.attributeWork(); err != nil {
			return err
		}
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
			if err := s.attributeWork(); err != nil {
				return err
			}
			if k == "O" {
				continue
			}
			if (*owner == "Table" && (k == "RowSpan" || k == "ColSpan")) || (*owner == "List" && k == "ListNumbering") || (*owner == "FENote" && k == "NoteType") {
				n.attrs[k] = v
			} else {
				s.warn("pdf_structure_attributes", "Additional structure layout/style/table/list attributes are not represented")
			}
		}
		return nil
	}
	if c := d["C"]; c != nil {
		if err := s.attributeWork(); err != nil {
			return err
		}
		v, e := s.reader.pdf.Dereference(c)
		if e != nil {
			return e
		}
		classes, ok := v.(types.Array)
		if !ok {
			classes = types.Array{v}
		}
		for i := 0; i < len(classes); i++ {
			if err := s.attributeWork(); err != nil {
				return err
			}
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
	if d["A"] != nil {
		return attrs(d["A"], 0)
	}
	return s.reader.ctx.Err()
}

// Count expanded visits, not unique objects: repeated acyclic attribute arrays
// can amplify a small PDF into exponential work despite a nesting limit.

// Explicit replacement text can restore whitespace consumed at line wrapping.
// This is character alignment, not fuzzy matching: every painted non-synthetic
// character must match in order. Only missing Unicode whitespace may be inserted;
// style/relationship/source ownership stays on each original character segment.

// A prepared note atom must never disappear into a replacement.
// The preparation pass normally keeps opaque ancestors in flow.

// Structural ActualText replaces characters, not homogeneous formatting. A
// mixed replacement has no exact character-to-style mapping; retain common
// marks and disclose the ambiguity, as for content-stream ActualText.

// Painted lines and inline structure elements do not define paragraph breaks.
// A container's consecutive inline content forms an implied paragraph, while
// explicit paragraphs and nontext blocks remain barriers, even through wrappers.

// Container Ref edges live on block sources as well as runs. Retain both
// original sources before flattening a wrapper into the merged paragraph.

// Do not union different pages into the first page's coordinate system.

// Discretionary hyphens and explicit word-boundary controls already convey
// the logical join. A change of painted baseline must not invent a space.

func (s *pdfStructureRecovery) attributeWork() error {
	if err := s.reader.ctx.Err(); err != nil {
		return err
	}
	s.attributeTotal++
	if s.attributeTotal > s.reader.structureLimit() {
		return fmt.Errorf("structure attributes exceed %d work entries", s.reader.structureLimit())
	}
	return nil
}
func (s *pdfStructureRecovery) content(n *pdfStructureNode) ([]Block, error) {
	var ids []int
	if n.kind == "MCR" {
		ids = s.marked[pdfContentKey{n.page, n.stream, n.mcid}]

		// PDF logical rows may omit merge attributes. Keep their explicit
		// membership if painted evidence cannot uniquely recover the grid;
		// do not invent empty cells or discard the table's structure/content.

	} else {
		// OBJR refers to a physical page/object. Do not rescan every painted
		// unit in a multi-thousand-page document for each object reference.
		candidates := s.unitsByPage[n.page]
		if n.page == 0 {
			candidates = make([]int, len(s.units))
			for i := range candidates {
				candidates[i] = i
			}
		}
		for j, i := range candidates {
			if j%1024 == 0 {
				if err := s.reader.ctx.Err(); err != nil {
					return nil, err
				}
			}
			u := s.units[i]
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

			// A block-bearing label owns editable structure independently of
			// its current displayed number. Do not infer away those blocks.

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
	source := &SourceInfo{Format: PDF, ParentElement: n.parent, StructureID: append([]byte(nil), n.originalID...), Element: n.id, Tag: n.tag, Role: n.role, Structure: "explicit", Evidence: "pdf_structure_tree", Confidence: 1, Precision: "union_of_content_bounds"}
	if len(n.refs) > 0 {
		source.References = []SourceStructureReference{{Element: n.id, Targets: append([]string(nil), n.refs...)}}
	}
	boxes := map[int]Quad{}
	var add func(*SourceInfo)
	add = func(src *SourceInfo) {
		if src == nil {
			return
		}
		source.References = structureReferences(source.References, src.References)
		source.Artifacts = artifactIDs(source, src.Artifacts...)
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
				if item.Marker != nil {
					add(item.Marker.Source)
					walk(item.Marker.Blocks)
				}
				walk(item.Blocks)
			}
		}
	}
	walk(blocks)

	// Decimal fallback only applies to its declared family or absent attributes.
	// Conflicting explicit Roman/Alpha evidence must not become decimal counters.

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

func alignPDFActualWhitespace(blocks []Block, text string, n *pdfStructureNode) ([]Block, bool) {
	if n.role != "Span" || len(n.refs) > 0 || len(blocks) != 1 || blocks[0].Kind != Paragraph {
		return nil, false
	}
	target := []rune(text)
	at := 0
	var out []Run
	var last Run
	insert := func(space string, style Run) {
		style.Text = space
		style.Source = &SourceInfo{Format: PDF, Role: n.role, Element: n.id, ParentElement: n.parent, Structure: "explicit", Evidence: "pdf_structure_actualtext_whitespace;adjacent_style_inferred", Confidence: .75, Precision: "replacement_whitespace"}
		out = append(out, style)
	}
	for _, run := range blocks[0].Runs {
		if run.Source != nil && run.Source.Precision == "synthetic_separator" {
			continue
		}
		if run.Note != nil || run.Anchor != "" {
			return nil, false
		}
		var chunk strings.Builder
		flush := func() {
			if chunk.Len() > 0 {
				part := run
				part.Text = chunk.String()
				out = append(out, part)
				chunk.Reset()
			}
		}
		for _, ch := range run.Text {
			start := at
			for at < len(target) && target[at] != ch && unicode.IsSpace(target[at]) && !unicode.IsSpace(ch) {
				at++
			}
			if at > start {
				flush()
				insert(string(target[start:at]), run)
			}
			if at >= len(target) || target[at] != ch {
				return nil, false
			}
			chunk.WriteRune(ch)
			at++
		}
		flush()
		last = run
	}
	start := at
	for at < len(target) && unicode.IsSpace(target[at]) {
		at++
	}
	if at != len(target) {
		return nil, false
	}
	if at > start {
		insert(string(target[start:at]), last)
	}
	block := blocks[0]
	block.Runs = out
	return []Block{block}, true
}
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
		} else if aligned, ok := alignPDFActualWhitespace(blocks, *n.actual, n); ok {
			blocks = aligned
			s.warn("pdf_actualtext_whitespace", "Explicit ActualText restores whitespace absent from paint; adjacent text style is used because that whitespace has no painted style evidence")
		} else {

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

	source := sourceFromBlocks([]Block{*last, b}, &pdfStructureNode{})
	last.Runs = append(last.Runs, b.Runs...)

	last.Source = source
	last.Source.Structure = "inferred"
	last.Source.Evidence = "pdf_structure_inline_paragraph"
	last.Source.Confidence = .75
}
func needsPDFSeparator(a, b Run) bool {
	if a.Text == "" || b.Text == "" || unicode.IsSpace([]rune(a.Text)[len([]rune(a.Text))-1]) || unicode.IsSpace([]rune(b.Text)[0]) {
		return false
	}

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
	explicitLabelBlocks := false
	for _, item := range n.kids {
		var label, body []Block
		for _, child := range item.kids {
			blocks, e := s.node(child)
			if e != nil {
				return nil, e
			}
			if child.role == "Lbl" {

				for _, content := range child.kids {
					if content.kind == "StructElem" && (pdfParagraphRole(content.role) || content.role == "Table" || content.role == "L" || content.role == "Figure") {
						explicitLabelBlocks = true
					}
				}
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
	textOnlyLabels, markerPresentation := !explicitLabelBlocks, false
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

// Reconcile a page-local painted grid with explicit row/cell membership. A
// candidate never changes the input table. Empty vertical continuations may be
// coalesced, but their explicit identities remain in the origin's provenance.

// Match each row to physical cell segments, allowing an empty continuation to
// be present or omitted in the logical tree. Count at most two alignments: an
// ambiguous sequence must never pick an arbitrary merged origin.

// Grid cells are origin-sorted; spanning cells can precede new origins.

// Some Word exports list all empty continuation tags before the new
// cells in a row. When the arity forces every eligible empty cell to be a
// continuation, match the two ordered classes independently. No visible
// cell is reordered, and inferred attribution keeps every explicit ID.

// Allocate enough space before retaining pointers into this row.

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

func (s *pdfStructureRecovery) recoverTableSpans(table Block, nodes []*pdfStructureNode) (Block, bool) {
	source := table.Source
	if source == nil || source.Page < 1 || source.Quad == nil || len(source.Regions) > 1 {
		return table, false
	}
	if s.grids == nil {
		s.grids = map[int][]pdfGridRegion{}
	}
	regions, ok := s.grids[source.Page]
	if !ok {
		regions = s.reader.rulingRegions(source.Page)
		s.grids[source.Page] = regions
	}
	var chosen Block
	matches := 0
	spansChanged := false
	x0, y0, x1, y1 := quadBounds(*source.Quad)
	for _, g := range regions {
		if s.reader.ctx.Err() != nil {
			return table, false
		}
		if !g.valid || len(g.ys)-1 != len(table.Rows) || !rulingContains(g.rect, pdfRulingRect{x0, y0, x1, y1}) {
			continue
		}
		candidate, changed, ok := s.matchTaggedGrid(table, nodes, g)
		if !ok {
			continue
		}
		if !changed {
			candidate = table
		}
		candidate.ColumnWidths = gridColumnWidths(g)
		src := *candidate.Source
		src.TableWidths = gridWidthSource(g)
		candidate.Source = &src
		chosen = candidate
		spansChanged = changed
		matches++
		if matches > 1 {
			return table, false
		}
	}
	if matches != 1 {
		return table, false
	}
	s.warn("pdf_table_widths_inferred", "Column widths are measured from a uniquely matching painted grid, independently of explicit logical membership")
	if spansChanged {
		s.warn("pdf_structure_spans_inferred", "Missing tagged-table spans are recovered from a uniquely matching painted grid; logical membership stays explicit and empty continuation identities are retained")
	}
	return chosen, true
}

func taggedCellEmpty(c Cell) bool {
	for _, b := range c.Blocks {
		if b.Kind != Paragraph || len(b.Runs) > 0 || b.PageBreakBefore {
			return false
		}
	}
	return true
}
func taggedCellInside(c Cell, rect pdfRulingRect, page int) bool {
	if c.Source != nil && c.Source.Page != 0 && c.Source.Page != page {
		return false
	}
	if taggedCellEmpty(c) {
		return true
	}
	if c.Source == nil || c.Source.Page != page || c.Source.Quad == nil {
		return false
	}
	x0, y0, x1, y1 := quadBounds(*c.Source.Quad)
	return rulingContains(rect, pdfRulingRect{x0, y0, x1, y1})
}
func taggedGridRect(g pdfGridRegion, c pdfGridCell) pdfRulingRect {
	return pdfRulingRect{g.xs[c.col], g.ys[c.row], g.xs[c.col+c.cols], g.ys[c.row+c.rows]}
}
func taggedSpanMatches(n *pdfStructureNode, c Cell, g pdfGridCell, continuation bool) bool {
	if continuation && (n.attrs["RowSpan"] != nil || n.id == "") {
		return false
	}
	if n.attrs["ColSpan"] != nil && max(1, c.ColSpan) != g.cols {
		return false
	}
	if continuation {
		return n.attrs["RowSpan"] == nil && taggedCellEmpty(c)
	}
	return n.attrs["RowSpan"] == nil || max(1, c.RowSpan) == g.rows
}

func (s *pdfStructureRecovery) matchTaggedRow(table Block, nodes []*pdfStructureNode, g pdfGridRegion, row int) ([]int, bool) {
	var segments []int
	for i, c := range g.cells {
		if c.row <= row && c.row+c.rows > row {
			segments = append(segments, i)
		}
	}

	sort.Slice(segments, func(i, j int) bool { return g.cells[segments[i]].col < g.cells[segments[j]].col })
	cells := table.Rows[row].Cells
	if len(cells) > len(segments) {
		return nil, false
	}
	required := make([]int, len(segments)+1)
	for i := len(segments) - 1; i >= 0; i-- {
		required[i] = required[i+1]
		if g.cells[segments[i]].row == row {
			required[i]++
		}
	}

	var starts, continued, empty []int
	for _, gi := range segments {
		if g.cells[gi].row == row {
			starts = append(starts, gi)
		} else {
			continued = append(continued, gi)
		}
	}
	needed := len(cells) - len(starts)
	if needed < 0 || needed > len(continued) {
		return nil, false
	}
	for ci, c := range cells {
		if taggedCellEmpty(c) && nodes[row].kids[ci].attrs["RowSpan"] == nil && nodes[row].kids[ci].id != "" {
			empty = append(empty, ci)
		}
	}
	if needed > 0 && needed == len(empty) && needed == len(continued) {
		mapping := make([]int, len(cells))
		isContinuation := make(map[int]bool, len(empty))
		for i, ci := range empty {
			gi := continued[i]
			if !taggedSpanMatches(nodes[row].kids[ci], cells[ci], g.cells[gi], true) || !taggedCellInside(cells[ci], taggedGridRect(g, g.cells[gi]), table.Source.Page) {
				return nil, false
			}
			mapping[ci], isContinuation[ci] = gi, true
		}
		next := 0
		for ci, c := range cells {
			if isContinuation[ci] {
				continue
			}
			if next >= len(starts) {
				return nil, false
			}
			gi := starts[next]
			if !taggedSpanMatches(nodes[row].kids[ci], c, g.cells[gi], false) || !taggedCellInside(c, taggedGridRect(g, g.cells[gi]), table.Source.Page) {
				return nil, false
			}
			mapping[ci] = gi
			next++
		}
		return mapping, next == len(starts)
	}
	type state struct{ segment, cell int }
	memo := map[state]int{}
	exhausted := false
	var count func(int, int) int
	count = func(si, ci int) int {
		if s.reader.ctx.Err() != nil {
			return 0
		}
		if len(cells)-ci < required[si] || len(cells)-ci > len(segments)-si {
			return 0
		}
		if exhausted || len(memo) >= 100000 {
			exhausted = true
			return 0
		}
		key := state{si, ci}
		if value, ok := memo[key]; ok {
			return value
		}
		if si == len(segments) {
			if ci == len(cells) {
				return 1
			}
			return 0
		}
		grid := g.cells[segments[si]]
		continuation := grid.row < row
		total := 0
		if continuation {
			total = count(si+1, ci)
		}
		if total > 1 {
			memo[key] = 2
			return 2
		}
		if ci < len(cells) && taggedSpanMatches(nodes[row].kids[ci], cells[ci], grid, continuation) && taggedCellInside(cells[ci], taggedGridRect(g, grid), table.Source.Page) {
			total += count(si+1, ci+1)
		}
		total = min(2, total)
		memo[key] = total
		return total
	}
	alignments := count(0, 0)
	if exhausted {
		s.warn("pdf_structure_spans_limit", "Tagged span alignment exceeds the inference budget; original logical cells remain")
	}
	if exhausted || alignments != 1 {
		return nil, false
	}
	mapping := make([]int, len(cells))
	ci := 0
	for si, gi := range segments {
		grid := g.cells[gi]
		if grid.row < row && count(si+1, ci) > 0 {
			continue
		}
		if ci >= len(cells) {
			return nil, false
		}
		mapping[ci] = gi
		ci++
	}
	return mapping, ci == len(cells)
}

func (s *pdfStructureRecovery) matchTaggedGrid(table Block, nodes []*pdfStructureNode, g pdfGridRegion) (Block, bool, bool) {
	mappings := make([][]int, len(table.Rows))
	for ri := range table.Rows {
		var ok bool
		mappings[ri], ok = s.matchTaggedRow(table, nodes, g, ri)
		if !ok {
			return Block{}, false, false
		}
	}
	result := table
	result.Rows = make([]Row, len(table.Rows))
	origins := make(map[int]*Cell)
	changed := false
	for ri, row := range table.Rows {
		result.Rows[ri].RepeatHeader = row.RepeatHeader

		result.Rows[ri].Cells = make([]Cell, 0, len(row.Cells))
		for ci, cell := range row.Cells {
			gi := mappings[ri][ci]
			grid := g.cells[gi]
			node := nodes[ri].kids[ci]
			if grid.row < ri {
				origin := origins[gi]
				if origin == nil {
					return Block{}, false, false
				}
				origin.Source.TableSpans.Continuations = append(origin.Source.TableSpans.Continuations, SourceElementReference{Row: ri, Element: node.id, Role: node.role, Tag: node.tag, Page: node.page})
				changed = true
				continue
			}
			if max(1, cell.ColSpan) != grid.cols || max(1, cell.RowSpan) != grid.rows {
				changed = true
			}
			cell.ColSpan = grid.cols
			cell.RowSpan = grid.rows
			if cell.RowSpan == 1 {
				cell.RowSpan = 0
			}
			if cell.Source == nil {
				return Block{}, false, false
			}
			src := *cell.Source
			if src.Page == 0 {
				src.Page = table.Source.Page
			}
			rect := taggedGridRect(g, grid)
			src.TableSpans = &TableSpanSource{ColSpan: "inferred", RowSpan: "inferred", Evidence: "painted_grid_adjacency", Confidence: .7, Bounds: Quad{{rect.x0, rect.y0}, {rect.x1, rect.y0}, {rect.x1, rect.y1}, {rect.x0, rect.y1}}}
			if node.attrs["ColSpan"] != nil {
				src.TableSpans.ColSpan = "explicit"
			}
			if node.attrs["RowSpan"] != nil {
				src.TableSpans.RowSpan = "explicit"
			}
			cell.Source = &src
			result.Rows[ri].Cells = append(result.Rows[ri].Cells, cell)
			origins[gi] = &result.Rows[ri].Cells[len(result.Rows[ri].Cells)-1]
		}
	}
	if _, err := tableGrid(result.Rows); err != nil {
		return Block{}, false, false
	}
	if !changed {
		return table, false, true
	}
	return result, changed, true
}
