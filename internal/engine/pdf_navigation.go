package engine

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
	"math"
	"reflect"
	"sort"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

type pdfDestinationPoint struct {
	page int
	y    float64
}

func (r *pdfRenderer) prepareDestinations(d *Document) error {
	c, err := catalogDestinations(d, true)
	if err != nil {
		return err
	}
	if len(c.anchors)+len(c.links) == 0 {
		return nil
	}
	r.destinationPoints = map[string]pdfDestinationPoint{}
	r.destinationTokens = map[string]string{}
	r.destinationURIs = map[string]string{}
	used := map[string]bool{}
	if err := documentDestinationRuns(d, func(run Run) error {
		if run.Link != "" && run.Destination == "" {
			used[pdfASCIIURI(run.Link)] = true
		}
		return nil
	}); err != nil {
		return err
	}
	prefix := ""
	for nonce := 0; nonce <= len(used); nonce++ {
		candidate := fmt.Sprintf("urn:manydoc:internal:%d:", nonce)
		collision := false
		for uri := range used {
			if strings.HasPrefix(uri, candidate) {
				collision = true
				break
			}
		}
		if !collision {
			prefix = candidate
			break
		}
	}
	if prefix == "" {
		return fmt.Errorf("cannot allocate internal link namespace")
	}
	ids := make([]string, 0, len(c.links))
	for id := range c.links {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for i, id := range ids {
		token := fmt.Sprintf("%s%d", prefix, i)
		r.destinationTokens[token] = id
		r.destinationURIs[id] = token
	}
	return nil
}

func (r *pdfRenderer) bindDestinations(ids []string, y float64) {
	for _, id := range ids {
		if _, seen := r.destinationPoints[id]; seen {
			continue
		}
		if r.destinationPoints == nil {
			r.destinationPoints = map[string]pdfDestinationPoint{}
		}
		r.destinationPoints[id] = pdfDestinationPoint{r.pdf.PageNo(), y}
	}
}

// Bind only after shaping, line breaking and bidi placement. The anchor itself
// never enters text or changes graphemes, glyph context, advances or metrics.
// Navigation opens the containing line; it does not claim a glyph caret x.
func bindPDFLineAnchors(runs []Run, lines []pdfLine) []pdfLine {
	if len(lines) == 0 {
		lines = []pdfLine{{}}
	}
	offset := 0
	for _, run := range runs {
		if run.Anchor != "" {
			chosen := -1
			following := -1
			for li, line := range lines {
				for _, span := range line {
					if span.bidi.paragraph == nil {
						continue
					}
					if span.bidi.start <= offset && offset < span.bidi.end {
						chosen = li
						break
					}
					if following < 0 && span.bidi.start >= offset {
						following = li
					}
				}
				if chosen >= 0 {
					break
				}
			}
			if chosen < 0 {
				chosen = following
			}
			if chosen < 0 {
				chosen = len(lines) - 1
			}
			lines[chosen] = append(lines[chosen], pdfSpan{run: Run{Anchor: run.Anchor}})
		}
		if run.Note != nil {
			offset++
		} else {
			offset += utf8.RuneCountInString(run.Text)
		}
	}
	return lines
}

func pdfDestinationString(id string) types.HexLiteral {
	data := []byte{0xfe, 0xff}
	for _, unit := range utf16.Encode([]rune(id)) {
		data = append(data, byte(unit>>8), byte(unit))
	}
	return types.HexLiteral(hex.EncodeToString(data))
}

func (r *pdfRenderer) finishDestinations(p *model.Context) error {
	if len(r.destinationPoints)+len(r.destinationTokens) == 0 {
		return nil
	}
	ids := make([]string, 0, len(r.destinationPoints))
	for id := range r.destinationPoints {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return pdfDestinationString(ids[i]) < pdfDestinationString(ids[j]) })
	pairs := types.Array{}
	for _, id := range ids {
		point := r.destinationPoints[id]
		dict, ref, _, err := p.PageDict(point.page, false)
		if err != nil {
			return err
		}
		box, err := p.DereferenceArray(dict["MediaBox"])
		if err != nil {
			return err
		}
		// gofpdf may inherit the media box. Its canvas height is available in
		// the page's generated /MediaBox or through inherited attributes.

		// Geometric association only assigns whole recovered spans when the overlap is
		// unambiguous. Partial spans remain unchanged and diagnosed; tagged Link nodes
		// can recover their exact ownership independently of geometric estimates.

		// PDFNavigation retains explicit PDF annotation/view evidence independently of
		// inferred editable-flow anchors. Coordinates in Point/Region use displayed pt;
		// Parameters retain original PDF user units and nullable viewer settings.

		// name (legacy dictionary), string (name tree), or empty for anonymous
		// identity bytes, not a Unicode-equivalence key

		// FlowBinding records inference, never authorial bookmark ownership.
		// nearest_content, page_boundary, or unbound (source evidence without a current flow assertion)

		// A nil catalog validates physical/history syntax independently of an edited
		// flow graph. Document.Validate always supplies its current complete catalog.

		if len(box) != 4 {
			_, _, inherited, e := p.PageDict(point.page, false)
			if e != nil {
				return e
			}
			if inherited.MediaBox == nil {
				return fmt.Errorf("destination page lacks media box")
			}
			box = types.Array{types.Float(inherited.MediaBox.LL.X), types.Float(inherited.MediaBox.LL.Y), types.Float(inherited.MediaBox.UR.X), types.Float(inherited.MediaBox.UR.Y)}
		}
		top, err := pdfNumber(box[3])
		if err != nil {
			return err
		}
		pairs = append(pairs, pdfDestinationString(id), types.Array{*ref, types.Name("XYZ"), types.Integer(0), types.Float(top - point.y), nil})
	}
	if len(pairs) > 0 {
		catalog, err := p.Catalog()
		if err != nil {
			return err
		}
		names, err := p.DereferenceDict(catalog["Names"])
		if err != nil {
			return err
		}
		if names == nil {
			names = types.Dict{}
		}
		if names["Dests"] != nil {
			return fmt.Errorf("unexpected generated destination name tree")
		}
		names["Dests"] = types.Dict{"Names": pairs}
		catalog["Names"] = names
	}
	changed := 0
	for page := 1; page <= p.PageCount; page++ {
		if err := r.ctx.Err(); err != nil {
			return err
		}
		dict, _, _, err := p.PageDict(page, false)
		if err != nil {
			return err
		}
		annots, err := p.DereferenceArray(dict["Annots"])
		if err != nil {
			return err
		}
		for _, ref := range annots {
			a, err := p.DereferenceDict(ref)
			if err != nil {
				return err
			}
			action, err := p.DereferenceDict(a["A"])
			if err != nil {
				return err
			}
			if action == nil {
				continue
			}
			kind := action.NameEntry("S")
			if kind == nil || *kind != "URI" {
				continue
			}
			uri, err := pdfTextString(action["URI"])
			if err != nil {
				return err
			}
			id, ok := r.destinationTokens[uri]
			if !ok {
				continue
			}
			a["A"] = types.Dict{"S": types.Name("GoTo"), "D": pdfDestinationString(id)}
			changed++
		}
	}
	if len(r.destinationTokens) > 0 && changed == 0 {
		return fmt.Errorf("internal link annotations were not serialized")
	}
	return nil
}

type pdfLinkTarget struct{ uri, destination string }

func (t pdfLinkTarget) empty() bool { return t.uri == "" && t.destination == "" }
func (t pdfLinkTarget) apply(run *Run) {
	run.Link = t.uri
	run.Destination = t.destination
	if t.destination != "" {
		run.Link = destinationHref(Run{Destination: t.destination})
	}
}

type pdfRecoveredLink struct {
	target  pdfLinkTarget
	view    types.Object
	hasURI  bool
	regions []Quad
}

func (r *pdfReader) readLink(object types.Object, d types.Dict) error {
	subtype := d.NameEntry("Subtype")
	if subtype == nil || *subtype != "Link" {
		return nil
	}
	key := pdfObjectKey{r.page, fmt.Sprint(object)}
	action, err := r.pdf.DereferenceDict(d["A"])
	if err != nil {
		return err
	}
	if action != nil && d["Dest"] != nil {
		return fmt.Errorf("link has both an action and a destination")
	}
	link := pdfRecoveredLink{}
	if action != nil {
		kind := action.NameEntry("S")
		if kind != nil && *kind == "URI" {
			text, err := pdfTextString(action["URI"])
			if err != nil {
				return err
			}
			link.target.uri = text
			link.hasURI = true
			if text == "" {
				r.warnPage("pdf_link_empty_uri", "Empty URI annotation retained as physical evidence; an empty href cannot own an editable text mark")
			}
		} else if kind != nil && *kind == "GoTo" {
			if action["D"] == nil {
				return fmt.Errorf("GoTo action lacks a destination")
			}
			link.view = action["D"]
		} else {
			r.warnPage("pdf_link_action", "Non-URI/non-GoTo action is not represented as an editable link")
		}
	} else if d["Dest"] != nil {
		link.view = d["Dest"]
	}
	if points, err := r.pdf.DereferenceArray(d["QuadPoints"]); err != nil {
		return err
	} else if len(points) > 0 {
		if len(points)%8 != 0 {
			return fmt.Errorf("invalid link QuadPoints")
		}
		for i := 0; i < len(points); i += 8 {
			var q Quad
			for j := 0; j < 4; j++ {
				x, e := pdfNumber(points[i+2*j])
				if e != nil {
					return e
				}
				y, e := pdfNumber(points[i+2*j+1])
				if e != nil {
					return e
				}
				q[j] = r.pageInfo.Transform.point(x, y)
			}
			link.regions = append(link.regions, q)
		}
	} else {
		a, e := r.pdf.DereferenceArray(d["Rect"])
		if e != nil {
			return e
		}
		if len(a) != 4 {
			return fmt.Errorf("link requires a rectangle")
		}
		var b [4]float64
		for i, v := range a {
			n, e := pdfNumber(v)
			if e != nil || !finite(n) {
				return fmt.Errorf("invalid link rectangle")
			}
			b[i] = n
		}
		m := r.pageInfo.Transform
		link.regions = []Quad{{m.point(b[0], b[1]), m.point(b[2], b[1]), m.point(b[2], b[3]), m.point(b[0], b[3])}}
	}
	r.links[key] = link
	return nil
}

func (r *pdfReader) geometricLinks() error {
	// Compare a span only with annotations on its physical page. Scanning every
	// link in the document for every span is quadratic in multi-thousand-page PDFs.
	byPage := map[int][]pdfRecoveredLink{}
	for key, link := range r.links {
		if !link.target.empty() {
			byPage[key.page] = append(byPage[key.page], link)
		}
	}
	for bi := range r.doc.Blocks {
		if err := r.ctx.Err(); err != nil {
			return err
		}
		for ri := range r.doc.Blocks[bi].Runs {
			run := &r.doc.Blocks[bi].Runs[ri]
			if run.Source == nil || run.Source.Quad == nil {
				continue
			}
			a, b, c, d := quadBounds(*run.Source.Quad)
			area := (c - a) * (d - b)
			if area <= 0 {
				continue
			}
			target := pdfLinkTarget{}
			ambiguous := false
			for _, link := range byPage[run.Source.Page] {
				for _, q := range link.regions {
					x0, y0, x1, y1 := quadBounds(q)
					intersection := max(0, math.Min(c, x1)-math.Max(a, x0)) * max(0, math.Min(d, y1)-math.Max(b, y0))
					fraction := intersection / area
					if fraction >= .8 {
						if !target.empty() && target != link.target {
							ambiguous = true
						}
						target = link.target
					} else if fraction > .05 {
						r.partialLinks[run.Source.Order] = true
					}
				}
			}
			if !target.empty() && !ambiguous {
				target.apply(run)
				copy := *run.Source
				copy.Evidence += ";link_from_rectangle"
				run.Source = &copy
			} else if ambiguous {
				r.warn("pdf_link_overlap", "Overlapping different link targets leave geometric association unresolved")
			}
		}
	}
	return nil
}
func (s *pdfStructureRecovery) logicalLink(nodes []*pdfStructureNode) (pdfLinkTarget, bool) {
	target := pdfLinkTarget{}
	conflict := false
	var walk func([]*pdfStructureNode)
	walk = func(nodes []*pdfStructureNode) {
		for _, n := range nodes {
			if n.kind == "OBJR" {
				for _, key := range s.logicalLinkKeys(n) {
					link := s.reader.links[key]
					if !link.target.empty() {
						if !target.empty() && target != link.target {
							conflict = true
						}
						target = link.target
					}
				}
			}
			walk(n.kids)
		}
	}
	walk(nodes)
	return target, conflict
}

func (r *pdfReader) diagnosePartialLinks() {
	var walk func([]Block)
	walk = func(blocks []Block) {
		for _, b := range blocks {
			for _, run := range b.Runs {
				if run.Source != nil && r.partialLinks[run.Source.Order] && run.Link == "" && run.Destination == "" && run.Source.Precision != "synthetic_separator" {
					r.warn("pdf_link_partial_span", "Link rectangle partially covers a text span; exact ownership requires tagged Link structure or finer glyph recovery")
				}
			}
			for _, row := range b.Rows {
				for _, cell := range row.Cells {
					walk(cell.Blocks)
				}
			}
			for _, item := range b.Items {
				if item.Marker != nil {
					walk(item.Marker.Blocks)
				}
				walk(item.Blocks)
			}
		}
	}
	walk(r.doc.Blocks)
}

type PDFNavigation struct {
	Destinations []PDFDestination    `json:"destinations,omitempty"`
	Links        []PDFNavigationLink `json:"links,omitempty"`
}
type PDFDestination struct {
	ID         string     `json:"id"`
	NameKind   string     `json:"name_kind,omitempty"`
	NameBytes  []byte     `json:"name_bytes,omitempty"`
	Page       int        `json:"page,omitempty"`
	PageObject string     `json:"page_object,omitempty"`
	View       string     `json:"view,omitempty"`
	Parameters []*float64 `json:"parameters,omitempty"`
	Point      *Point     `json:"point,omitempty"`
	Region     *Quad      `json:"region,omitempty"`
	Unresolved bool       `json:"unresolved,omitempty"`

	FlowBinding string `json:"flow_binding,omitempty"`
	FlowOrder   int    `json:"flow_order,omitempty"`
}
type PDFNavigationLink struct {
	Kind        string `json:"kind"`
	Page        int    `json:"page"`
	Object      string `json:"object"`
	URI         string `json:"uri,omitempty"`
	Destination string `json:"destination,omitempty"`
	Regions     []Quad `json:"regions"`
}

func clonePDFNavigation(n *PDFNavigation) *PDFNavigation {
	if n == nil {
		return nil
	}
	out := *n
	out.Destinations = append([]PDFDestination(nil), n.Destinations...)
	out.Links = append([]PDFNavigationLink(nil), n.Links...)
	for i := range out.Destinations {
		v := &out.Destinations[i]
		v.NameBytes = append([]byte(nil), v.NameBytes...)
		v.Parameters = append([]*float64(nil), v.Parameters...)
		for j, p := range v.Parameters {
			if p != nil {
				x := *p
				v.Parameters[j] = &x
			}
		}
		if v.Point != nil {
			x := *v.Point
			v.Point = &x
		}
		if v.Region != nil {
			x := *v.Region
			v.Region = &x
		}
	}
	for i := range out.Links {
		out.Links[i].Regions = append([]Quad(nil), out.Links[i].Regions...)
	}
	return &out
}

func pdfViewCount(view string) int {
	switch view {
	case "XYZ":
		return 3
	case "Fit", "FitB":
		return 0
	case "FitH", "FitV", "FitBH", "FitBV":
		return 1
	case "FitR":
		return 4
	}
	return -1
}

const maxPDFNavigationEntries = 1_000_000
const maxPDFDestinationNameBytes = 64 << 20

func (n *PDFNavigation) validate(d *Document, c *destinationCatalog) error {
	if n == nil {
		return nil
	}
	if len(n.Destinations)+len(n.Links) > maxPDFNavigationEntries {
		return fmt.Errorf("PDF navigation exceeds %d entries", maxPDFNavigationEntries)
	}
	ids := map[string]bool{}
	names := map[string]bool{}
	totalNameBytes := 0
	for _, v := range n.Destinations {
		totalNameBytes += len(v.NameBytes)
		if totalNameBytes > maxPDFDestinationNameBytes {
			return fmt.Errorf("PDF destination name bytes exceed limit")
		}
		if destinationID(v.ID) != nil || ids[v.ID] {
			return fmt.Errorf("invalid/duplicate PDF destination ID")
		}
		ids[v.ID] = true
		if v.NameKind != "" && v.NameKind != "name" && v.NameKind != "string" {
			return fmt.Errorf("invalid PDF destination name kind")
		}
		if v.NameKind == "" && len(v.NameBytes) > 0 {
			return fmt.Errorf("anonymous PDF destination has name bytes")
		}
		if len(v.NameBytes) > 65536 {
			return fmt.Errorf("PDF destination name exceeds limit")
		}
		if v.NameKind != "" {
			key := pdfViewKey(v.NameKind, v.NameBytes)
			if names[key] {
				return fmt.Errorf("duplicate PDF destination name bytes")
			}
			names[key] = true
		}
		if v.Unresolved {
			if v.NameKind == "" || v.PageObject != "" || (c != nil && v.FlowBinding != "unbound" && !c.missing[v.ID]) || v.Page != 0 || v.View != "" || len(v.Parameters) > 0 || v.Point != nil || v.Region != nil || (v.FlowBinding != "" && v.FlowBinding != "unbound") || v.FlowOrder != 0 {
				return fmt.Errorf("invalid unresolved PDF destination")
			}
			continue
		}
		if v.Page < 1 || v.Page > len(d.Pages) || pdfViewCount(v.View) != len(v.Parameters) || (c != nil && v.FlowBinding != "unbound" && !c.anchors[v.ID]) {
			return fmt.Errorf("invalid PDF destination view or flow anchor")
		}
		for _, p := range v.Parameters {
			if p != nil && !finite(*p) {
				return fmt.Errorf("nonfinite PDF destination parameter")
			}
			if p == nil && v.View == "FitR" {
				return fmt.Errorf("FitR requires four coordinates")
			}
		}
		if v.View == "XYZ" && v.Parameters[2] != nil && *v.Parameters[2] < 0 {
			return fmt.Errorf("negative PDF destination zoom")
		}
		expected := v
		expected.Point = nil
		expected.Region = nil
		expected.deriveGeometry(d.Pages[v.Page-1])
		if !reflect.DeepEqual(v.Point, expected.Point) || !reflect.DeepEqual(v.Region, expected.Region) {
			return fmt.Errorf("destination geometry contradicts explicit view parameters")
		}
		if v.FlowBinding != "nearest_content" && v.FlowBinding != "page_boundary" && v.FlowBinding != "unbound" {
			return fmt.Errorf("invalid destination flow binding")
		}
		if v.FlowOrder < 0 {
			return fmt.Errorf("invalid destination flow order")
		}
		for _, s := range []string{v.PageObject} {
			if err := validateString(s); err != nil {
				return err
			}
		}
		if v.Point != nil && (!finite(v.Point.X) || !finite(v.Point.Y)) {
			return fmt.Errorf("invalid destination point")
		}
		if v.Region != nil {
			for _, p := range *v.Region {
				if !finite(p.X) || !finite(p.Y) {
					return fmt.Errorf("invalid destination region")
				}
			}
		}
	}
	for _, l := range n.Links {
		if l.Page < 1 || l.Page > len(d.Pages) || len(l.Regions) == 0 || (l.Kind != "URI" && l.Kind != "GoTo") || (l.Kind == "URI" && l.Destination != "") || (l.Kind == "GoTo" && (l.Destination == "" || l.URI != "")) {
			return fmt.Errorf("invalid PDF annotation link evidence")
		}
		if l.Destination != "" && !ids[l.Destination] {
			return fmt.Errorf("PDF annotation target lacks destination evidence")
		}
		for _, s := range []string{l.URI, l.Object} {
			if err := validateString(s); err != nil {
				return err
			}
		}
		for _, q := range l.Regions {
			for _, p := range q {
				if !finite(p.X) || !finite(p.Y) {
					return fmt.Errorf("invalid annotation link region")
				}
			}
		}
	}
	return nil
}

type pdfNamedView struct {
	kind  string
	name  []byte
	value types.Object
}
type pdfViewRegistry struct {
	r         *pdfReader
	names     map[string]pdfNamedView
	ids       map[string]string
	owners    map[string]string
	views     map[string]int
	visits    int
	nameBytes int
}

func pdfViewKey(kind string, raw []byte) string { return kind + ":" + string(raw) }
func (v *pdfViewRegistry) named(kind string, raw []byte, obj types.Object) error {
	key := pdfViewKey(kind, raw)
	if len(raw) > 65536 || len(v.names) >= v.r.navigationLimit() || len(raw) > maxPDFDestinationNameBytes-v.nameBytes {
		return fmt.Errorf("PDF destination name budget exceeded")
	}
	if _, ok := v.names[key]; ok {
		return fmt.Errorf("duplicate PDF destination name")
	}
	v.nameBytes += len(raw)
	v.names[key] = pdfNamedView{kind, append([]byte(nil), raw...), obj}
	return nil
}
func (v *pdfViewRegistry) tree(obj types.Object, depth int, seen map[string]bool) error {
	if err := v.r.ctx.Err(); err != nil {
		return err
	}
	v.visits++
	if depth > 64 || v.visits > 100000 {
		return fmt.Errorf("PDF destination tree budget exceeded")
	}
	if ref, ok := obj.(types.IndirectRef); ok {
		key := fmt.Sprint(ref)
		if seen[key] {
			return fmt.Errorf("repeated/cyclic PDF destination tree node")
		}
		seen[key] = true
	}
	d, err := v.r.pdf.DereferenceDict(obj)
	if err != nil {
		return err
	}
	if d == nil {
		return fmt.Errorf("missing PDF destination tree node")
	}
	ns, err := v.r.pdf.DereferenceArray(d["Names"])
	if err != nil {
		return err
	}
	kids, err := v.r.pdf.DereferenceArray(d["Kids"])
	if err != nil {
		return err
	}
	if len(ns) > 0 && len(kids) > 0 || len(ns)%2 != 0 {
		return fmt.Errorf("invalid destination name tree node")
	}
	var previous []byte
	for i := 0; i < len(ns); i += 2 {
		o, e := v.r.pdf.Dereference(ns[i])
		if e != nil {
			return e
		}
		raw, e := pdfString(o)
		if e != nil {
			return e
		}
		if i > 0 && bytes.Compare(previous, raw) >= 0 {
			return fmt.Errorf("destination tree keys are not strictly sorted")
		}
		previous = raw
		if e = v.named("string", raw, ns[i+1]); e != nil {
			return e
		}
	}
	for _, k := range kids {
		if err := v.tree(k, depth+1, seen); err != nil {
			return err
		}
	}
	return nil
}
func (v *pdfViewRegistry) allocate(key, kind string, raw []byte) string {
	if id, ok := v.ids[key]; ok {
		return id
	}
	id := ""
	if kind != "" {
		if kind == "name" && utf8.Valid(raw) {
			id = string(raw)
		} else {
			id, _ = pdfTextString(types.HexLiteral(hex.EncodeToString(raw)))
		}
	}
	if destinationID(id) != nil || (v.owners[id] != "" && v.owners[id] != key) {
		id = ""
	}
	if id == "" {
		for nonce := 0; ; nonce++ {
			candidate := fmt.Sprintf("pdf:destination:%x", sha256.Sum256([]byte(fmt.Sprintf("%s:%d", key, nonce))))
			if v.owners[candidate] == "" {
				id = candidate
				break
			}
		}
	}
	v.ids[key] = id
	v.owners[id] = key
	return id
}
func (v *pdfViewRegistry) array(obj types.Object) (PDFDestination, error) {
	out := PDFDestination{}
	o, err := v.r.pdf.Dereference(obj)
	if err != nil {
		return out, err
	}
	if d, ok := o.(types.Dict); ok {
		if d["SD"] != nil {
			v.r.warn("pdf_destination_structure", "Physical destination retained; additional structure-destination navigation semantics are not yet represented")
		}
		o, err = v.r.pdf.Dereference(d["D"])
		if err != nil {
			return out, err
		}
	}
	a, ok := o.(types.Array)
	if !ok || len(a) < 2 {
		return out, fmt.Errorf("PDF destination requires a view array")
	}
	ref, ok := a[0].(types.IndirectRef)
	if !ok {
		return out, fmt.Errorf("local PDF destination requires a page reference")
	}
	out.PageObject = fmt.Sprint(ref)
	out.Page = v.r.pageRefs[out.PageObject]
	if out.Page == 0 {
		return out, fmt.Errorf("PDF destination references an unknown page")
	}
	mode, err := v.r.pdf.Dereference(a[1])
	if err != nil {
		return out, err
	}
	name, ok := mode.(types.Name)
	if !ok {
		return out, fmt.Errorf("PDF destination view requires a name")
	}
	out.View = string(name)
	n := pdfViewCount(out.View)
	if n < 0 {
		return out, fmt.Errorf("%w: PDF destination view %s", ErrUnsupported, out.View)
	}
	if len(a) != n+2 {
		return out, fmt.Errorf("invalid parameter count for PDF %s destination", out.View)
	}
	for _, p := range a[2:] {
		o, e := v.r.pdf.Dereference(p)
		if e != nil {
			return out, e
		}
		if o == nil {
			out.Parameters = append(out.Parameters, nil)
			continue
		}
		x, e := pdfNumber(o)
		if e != nil || !finite(x) {
			return out, fmt.Errorf("invalid PDF destination parameter")
		}
		out.Parameters = append(out.Parameters, &x)
	}
	if out.View == "XYZ" && out.Parameters[2] != nil && *out.Parameters[2] < 0 {
		return out, fmt.Errorf("negative destination zoom")
	}
	if out.View == "FitR" {
		for _, p := range out.Parameters {
			if p == nil {
				return out, fmt.Errorf("FitR requires coordinates")
			}
		}
	}
	out.deriveGeometry(v.r.doc.Pages[out.Page-1])
	return out, nil
}
func (v *PDFDestination) deriveGeometry(page PDFPage) {
	p := v.Parameters
	m := page.Transform
	switch v.View {
	case "XYZ":
		if p[0] != nil && p[1] != nil {
			q := m.point(*p[0], *p[1])
			v.Point = &q
		} else if p[0] != nil {
			a, b := m.point(*p[0], page.CropBox[1]), m.point(*p[0], page.CropBox[3])
			q := Quad{a, b, b, a}
			v.Region = &q
		} else if p[1] != nil {
			a, b := m.point(page.CropBox[0], *p[1]), m.point(page.CropBox[2], *p[1])
			q := Quad{a, b, b, a}
			v.Region = &q
		}
	case "FitR":
		q := Quad{m.point(*p[0], *p[1]), m.point(*p[2], *p[1]), m.point(*p[2], *p[3]), m.point(*p[0], *p[3])}
		v.Region = &q
	case "FitH", "FitBH":
		if p[0] != nil {
			a, b := m.point(page.CropBox[0], *p[0]), m.point(page.CropBox[2], *p[0])
			q := Quad{a, b, b, a}
			v.Region = &q
		}
	case "FitV", "FitBV":
		if p[0] != nil {
			a, b := m.point(*p[0], page.CropBox[1]), m.point(*p[0], page.CropBox[3])
			q := Quad{a, b, b, a}
			v.Region = &q
		}
	}
}
func (v *pdfViewRegistry) resolve(obj types.Object) (string, error) {
	o, e := v.r.pdf.Dereference(obj)
	if e != nil {
		return "", e
	}
	kind := ""
	var raw []byte
	switch n := o.(type) {
	case types.Name:
		kind = "name"
		raw = []byte(n)
	case types.StringLiteral, types.HexLiteral:
		kind = "string"
		raw, e = pdfString(n)
		if e != nil {
			return "", e
		}
	}
	if kind != "" {
		if len(raw) > 65536 {
			return "", fmt.Errorf("PDF destination name budget exceeded")
		}
		key := pdfViewKey(kind, raw)
		id := v.allocate(key, kind, raw)
		if _, ok := v.views[key]; ok {
			return id, nil
		}
		entry, ok := v.names[key]
		if !ok {
			v.r.doc.PDFNavigation.Destinations = append(v.r.doc.PDFNavigation.Destinations, PDFDestination{ID: id, NameKind: kind, NameBytes: append([]byte(nil), raw...), Unresolved: true})
			v.views[key] = len(v.r.doc.PDFNavigation.Destinations) - 1
			v.r.doc.UnresolvedDestinations = append(v.r.doc.UnresolvedDestinations, id)
			v.r.warn("pdf_destination_missing", "Named PDF link target is absent; its identity is retained explicitly unresolved")
			return id, nil
		}
		d, e := v.array(entry.value)
		if e != nil {
			return "", e
		}
		d.ID = id
		d.NameKind = kind
		d.NameBytes = append([]byte(nil), raw...)
		v.views[key] = len(v.r.doc.PDFNavigation.Destinations)
		v.r.doc.PDFNavigation.Destinations = append(v.r.doc.PDFNavigation.Destinations, d)
		return id, nil
	}
	d, e := v.array(o)
	if e != nil {
		return "", e
	}
	// Anonymous arrays with identical page/view parameters describe the same view.
	key := fmt.Sprintf("array:%s:%s", d.PageObject, d.View)
	for _, p := range d.Parameters {
		if p == nil {
			key += ":null"
		} else {
			key += fmt.Sprintf(":%.17g", *p)
		}
	}
	id := v.allocate(key, "", nil)
	if _, ok := v.views[key]; !ok {
		d.ID = id
		v.views[key] = len(v.r.doc.PDFNavigation.Destinations)
		v.r.doc.PDFNavigation.Destinations = append(v.r.doc.PDFNavigation.Destinations, d)
	}
	return id, nil
}
func (r *pdfReader) recoverPDFNavigation(catalog types.Dict) error {
	v := &pdfViewRegistry{r: r, names: map[string]pdfNamedView{}, ids: map[string]string{}, owners: map[string]string{}, views: map[string]int{}}
	r.doc.PDFNavigation = &PDFNavigation{}
	legacy, e := r.pdf.DereferenceDict(catalog["Dests"])
	if e != nil {
		return e
	}
	for name, obj := range legacy {
		if e = v.named("name", []byte(name), obj); e != nil {
			return e
		}
	}
	names, e := r.pdf.DereferenceDict(catalog["Names"])
	if e != nil {
		return e
	}
	if names["Dests"] != nil {
		if e = v.tree(names["Dests"], 0, map[string]bool{}); e != nil {
			return e
		}
	}
	keys := make([]string, 0, len(v.names))
	for key := range v.names {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	// Allocate all declared names first so anonymous views cannot capture their IDs.
	for _, key := range keys {
		n := v.names[key]
		v.allocate(key, n.kind, n.name)
	}
	for _, key := range keys {
		n := v.names[key]
		var obj types.Object = types.HexLiteral(hex.EncodeToString(n.name))
		if n.kind == "name" {
			obj = types.Name(n.name)
		}
		if _, e = v.resolve(obj); e != nil {
			return e
		}
	}
	links := make([]pdfObjectKey, 0, len(r.links))
	for key := range r.links {
		links = append(links, key)
	}
	sort.Slice(links, func(i, j int) bool {
		if links[i].page != links[j].page {
			return links[i].page < links[j].page
		}
		return links[i].object < links[j].object
	})
	for _, key := range links {
		if e = r.ctx.Err(); e != nil {
			return e
		}
		link := r.links[key]
		if link.view != nil {
			link.target.destination, e = v.resolve(link.view)
			if e != nil {
				return e
			}
			r.links[key] = link
		}
		kind := "URI"
		if link.view != nil {
			kind = "GoTo"
		}
		if link.hasURI || link.view != nil {
			r.doc.PDFNavigation.Links = append(r.doc.PDFNavigation.Links, PDFNavigationLink{Kind: kind, Page: key.page, Object: key.object, URI: link.target.uri, Destination: link.target.destination, Regions: append([]Quad(nil), link.regions...)})
		}
		if len(r.doc.PDFNavigation.Destinations)+len(r.doc.PDFNavigation.Links) > r.navigationLimit() {
			return fmt.Errorf("PDF navigation budget exceeded")
		}
	}
	if len(r.doc.PDFNavigation.Destinations)+len(r.doc.PDFNavigation.Links) == 0 {
		r.doc.PDFNavigation = nil
	} else {
		sort.Slice(r.doc.PDFNavigation.Destinations, func(i, j int) bool {
			return r.doc.PDFNavigation.Destinations[i].ID < r.doc.PDFNavigation.Destinations[j].ID
		})
	}
	sort.Strings(r.doc.UnresolvedDestinations)
	return nil
}

type pdfFlowCandidate struct {
	block  *Block
	index  int
	source *SourceInfo
}

func (r *pdfReader) bindPDFNavigation() {
	if r.doc.PDFNavigation == nil {
		return
	}
	candidates := map[int][]pdfFlowCandidate{}
	var walk func([]Block)
	walk = func(bs []Block) {
		for i := range bs {
			b := &bs[i]
			found := false
			for j := range b.Runs {
				s := b.Runs[j].Source
				if s != nil && s.Page > 0 && s.Quad != nil && (b.Runs[j].Text != "" || b.Runs[j].Note != nil) {
					candidates[s.Page] = append(candidates[s.Page], pdfFlowCandidate{b, j, s})
					found = true
				}
			}
			if !found && b.Source != nil && b.Source.Page > 0 && b.Source.Quad != nil && (b.Kind == Paragraph || b.Kind == Heading || b.Kind == Picture) {
				candidates[b.Source.Page] = append(candidates[b.Source.Page], pdfFlowCandidate{b, 0, b.Source})
			}
			for j := range b.Items {
				if b.Items[j].Marker != nil {
					walk(b.Items[j].Marker.Blocks)
				}
				walk(b.Items[j].Blocks)
			}
			for j := range b.Rows {
				for k := range b.Rows[j].Cells {
					walk(b.Rows[j].Cells[k].Blocks)
				}
			}
		}
	}
	walk(r.doc.Blocks)
	for i := range r.doc.Notes {
		walk(r.doc.Notes[i].Blocks)
	}
	inserts := map[*Block]map[int][]Run{}
	pageAnchors := map[int][]string{}
	// Preserve global destination order/ties, but compute immutable candidate
	// bounds once per used page. Release a page after its final positioned view.
	remaining := map[int]int{}
	for _, v := range r.doc.PDFNavigation.Destinations {
		if !v.Unresolved && (v.Point != nil || v.Region != nil) {
			remaining[v.Page]++
		}
	}
	pageBounds := map[int][][4]float64{}
	for i := range r.doc.PDFNavigation.Destinations {
		v := &r.doc.PDFNavigation.Destinations[i]
		if v.Unresolved {
			continue
		}
		var bounds [][4]float64
		var region [4]float64
		if v.Point != nil || v.Region != nil {
			bounds = pageBounds[v.Page]
			if bounds == nil && len(candidates[v.Page]) > 0 {
				bounds = make([][4]float64, len(candidates[v.Page]))
				for j, c := range candidates[v.Page] {
					a, b, c, d := quadBounds(*c.source.Quad)
					bounds[j] = [4]float64{a, b, c, d}
				}
				pageBounds[v.Page] = bounds
			}
			remaining[v.Page]--
			if remaining[v.Page] == 0 {
				delete(pageBounds, v.Page)
			}
			if v.Region != nil {
				a, b, c, d := quadBounds(*v.Region)
				region = [4]float64{a, b, c, d}
			}
		}
		best := -1
		score := math.Inf(1)
		for j, c := range candidates[v.Page] {
			if v.Point == nil && v.Region == nil {
				break
			}
			x0, y0, x1, y1 := bounds[j][0], bounds[j][1], bounds[j][2], bounds[j][3]
			distance := 0.0
			if v.Point != nil {
				p := v.Point
				distance = math.Hypot(max(0, x0-p.X, p.X-x1), max(0, y0-p.Y, p.Y-y1))
			} else {
				a, b, c, d := region[0], region[1], region[2], region[3]
				distance = math.Hypot(max(0, x0-c, a-x1), max(0, y0-d, b-y1))
			}
			if best < 0 || distance < score-1e-6 || (math.Abs(distance-score) < 1e-6 && c.source.Order < candidates[v.Page][best].source.Order) {
				best = j
				score = distance
			}
		}
		if best < 0 && len(candidates[v.Page]) > 0 {
			v.FlowBinding = "page_boundary"
			chosen := candidates[v.Page][0]
			chosen.block.Anchors = append(chosen.block.Anchors, v.ID)
			v.FlowOrder = chosen.source.Order
			continue
		}
		if best < 0 {
			v.FlowBinding = "page_boundary"
			pageAnchors[v.Page] = append(pageAnchors[v.Page], v.ID)
			continue
		}
		chosen := candidates[v.Page][best]
		v.FlowBinding = "nearest_content"
		v.FlowOrder = chosen.source.Order
		if chosen.index == 0 {
			chosen.block.Anchors = append(chosen.block.Anchors, v.ID)
		} else {
			if inserts[chosen.block] == nil {
				inserts[chosen.block] = map[int][]Run{}
			}
			source := &SourceInfo{Format: PDF, Page: v.Page, Order: v.FlowOrder, Structure: "inferred", Evidence: "destination_nearest_content", Confidence: .4, Precision: "navigation_flow_binding"}
			inserts[chosen.block][chosen.index] = append(inserts[chosen.block][chosen.index], Run{Anchor: v.ID, Source: source})
		}
	}
	for block, points := range inserts {
		count := len(block.Runs)
		for _, rs := range points {
			count += len(rs)
		}
		out := make([]Run, 0, count)
		for i, run := range block.Runs {
			out = append(out, points[i]...)
			out = append(out, run)
		}
		block.Runs = out
	}
	if len(pageAnchors) > 0 {
		// Page-boundary fallback is separate from authorial content ownership. Page
		// breaks survive blank pages even when no text/image can serve as an anchor.
		var out []Block
		page := 1
		add := func() {
			if ids := pageAnchors[page]; len(ids) > 0 {
				delete(pageAnchors, page)
				out = append(out, Block{Kind: AnchorPoint, Anchors: ids, Source: &SourceInfo{Format: PDF, Page: page, Structure: "inferred", Evidence: "destination_page_boundary", Confidence: .25}})
			}
		}
		add()
		for _, b := range r.doc.Blocks {
			out = append(out, b)
			if b.Kind == PageBreak {
				page++
				add()
			}
		}
		// Logical recovery may have folded physical page controls. Any remaining
		// blank-page view stays explicit; a diagnosed tail boundary is a flow
		// fallback, not a claim that the old page survived reflow.

		// URI actions use 7-bit ASCII (ISO 32000-1, Table 206). Map UTF-8 IRI octets
		// using RFC 3987 section 3.1 without normalization or re-encoding existing
		// escapes/reserved delimiters. Also escape excluded ASCII characters. Resolution
		// and scheme-specific host processing belong to the consuming application.

		remaining := make([]int, 0, len(pageAnchors))
		for p := range pageAnchors {
			remaining = append(remaining, p)
		}
		sort.Ints(remaining)
		for _, p := range remaining {
			out = append(out, Block{Kind: AnchorPoint, Anchors: pageAnchors[p], Source: &SourceInfo{Format: PDF, Page: p, Structure: "inferred", Evidence: "destination_missing_flow_page", Confidence: 0}})
			r.warn("pdf_destination_page_fallback", "A source page without recovered content/control has no exact flow boundary; its physical view is retained and editable anchor is placed at the document tail")
		}
		r.doc.Blocks = out
	}
	if len(r.doc.PDFNavigation.Destinations) > 0 {
		r.warn("pdf_destination_flow_inferred", "Explicit PDF views retained; editable anchor ownership is inferred from nearby content or a page boundary, not authorial bookmark structure")
	}
}

func pdfASCIIURI(iri string) string {
	unsafe := func(c byte) bool {
		return c <= 0x20 || c >= 0x7f || strings.IndexByte("<>\"{}|\\^`", c) >= 0
	}
	first := 0
	for first < len(iri) && !unsafe(iri[first]) {
		first++
	}
	if first == len(iri) {
		return iri
	}
	const hex = "0123456789ABCDEF"
	var out strings.Builder
	out.Grow(len(iri))
	out.WriteString(iri[:first])
	for i := first; i < len(iri); i++ {
		c := iri[i]
		if unsafe(c) {
			out.WriteByte('%')
			out.WriteByte(hex[c>>4])
			out.WriteByte(hex[c&15])
		} else {
			out.WriteByte(c)
		}
	}
	return out.String()
}

// The reader already keys annotation ownership by page/object. Preserve unknown-
// page conflict checks through a request-owned object index instead of scanning
// all annotations once per tagged Link.
func (s *pdfStructureRecovery) logicalLinkKeys(n *pdfStructureNode) []pdfObjectKey {
	if n.page != 0 {
		return []pdfObjectKey{{n.page, n.object}}
	}
	if s.linksByObject == nil {
		s.linksByObject = map[string][]pdfObjectKey{}
		for key := range s.reader.links {
			s.linksByObject[key.object] = append(s.linksByObject[key.object], key)
		}
	}
	return s.linksByObject[n.object]
}
