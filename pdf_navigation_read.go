package manydoc

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"sort"
	"unicode/utf8"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

type pdfNamedView struct {
	kind  string
	name  []byte
	value types.Object
}
type pdfViewRegistry struct {
	r      *pdfReader
	names  map[string]pdfNamedView
	ids    map[string]string
	owners map[string]string
	views  map[string]int
	visits int
}

func pdfViewKey(kind string, raw []byte) string { return kind + ":" + string(raw) }
func (v *pdfViewRegistry) named(kind string, raw []byte, obj types.Object) error {
	key := pdfViewKey(kind, raw)
	if len(raw) > 65536 || len(v.names) >= 100000 {
		return fmt.Errorf("PDF destination name budget exceeded")
	}
	if _, ok := v.names[key]; ok {
		return fmt.Errorf("duplicate PDF destination name")
	}
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
			v.r.doc.warn("pdf_destination_structure", "Physical destination retained; additional structure-destination navigation semantics are not yet represented")
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
			v.r.doc.warn("pdf_destination_missing", "Named PDF link target is absent; its identity is retained explicitly unresolved")
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
		if len(r.doc.PDFNavigation.Destinations)+len(r.doc.PDFNavigation.Links) > 100000 {
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
				if s != nil && s.Page > 0 && s.Quad != nil && b.Runs[j].Text != "" {
					candidates[s.Page] = append(candidates[s.Page], pdfFlowCandidate{b, j, s})
					found = true
				}
			}
			if !found && b.Source != nil && b.Source.Page > 0 && b.Source.Quad != nil && (b.Kind == Paragraph || b.Kind == Heading || b.Kind == Picture) {
				candidates[b.Source.Page] = append(candidates[b.Source.Page], pdfFlowCandidate{b, 0, b.Source})
			}
			for j := range b.Items {
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
	inserts := map[*Block]map[int][]Run{}
	pageAnchors := map[int][]string{}
	for i := range r.doc.PDFNavigation.Destinations {
		v := &r.doc.PDFNavigation.Destinations[i]
		if v.Unresolved {
			continue
		}
		best := -1
		score := math.Inf(1)
		for j, c := range candidates[v.Page] {
			if v.Point == nil && v.Region == nil {
				break
			}
			x0, y0, x1, y1 := quadBounds(*c.source.Quad)
			distance := 0.0
			if v.Point != nil {
				p := v.Point
				distance = math.Hypot(max(0, x0-p.X, p.X-x1), max(0, y0-p.Y, p.Y-y1))
			} else {
				a, b, c, d := quadBounds(*v.Region)
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
		var out []Run
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
		remaining := make([]int, 0, len(pageAnchors))
		for p := range pageAnchors {
			remaining = append(remaining, p)
		}
		sort.Ints(remaining)
		for _, p := range remaining {
			out = append(out, Block{Kind: AnchorPoint, Anchors: pageAnchors[p], Source: &SourceInfo{Format: PDF, Page: p, Structure: "inferred", Evidence: "destination_missing_flow_page", Confidence: 0}})
			r.doc.warn("pdf_destination_page_fallback", "A source page without recovered content/control has no exact flow boundary; its physical view is retained and editable anchor is placed at the document tail")
		}
		r.doc.Blocks = out
	}
	if len(r.doc.PDFNavigation.Destinations) > 0 {
		r.doc.warn("pdf_destination_flow_inferred", "Explicit PDF views retained; editable anchor ownership is inferred from nearby content or a page boundary, not authorial bookmark structure")
	}
}
