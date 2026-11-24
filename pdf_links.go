package manydoc

import (
	"fmt"
	"math"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

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

// Geometric association only assigns whole recovered spans when the overlap is
// unambiguous. Partial spans remain unchanged and diagnosed; tagged Link nodes
// can recover their exact ownership independently of geometric estimates.
func (r *pdfReader) geometricLinks() {
	for bi := range r.doc.Blocks {
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
			for key, link := range r.links {
				if key.page != run.Source.Page || link.target.empty() {
					continue
				}
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
				r.doc.warn("pdf_link_overlap", "Overlapping different link targets leave geometric association unresolved")
			}
		}
	}
}
func (s *pdfStructureRecovery) logicalLink(nodes []*pdfStructureNode) (pdfLinkTarget, bool) {
	target := pdfLinkTarget{}
	conflict := false
	var walk func([]*pdfStructureNode)
	walk = func(nodes []*pdfStructureNode) {
		for _, n := range nodes {
			if n.kind == "OBJR" {
				for key, link := range s.reader.links {
					if key.object == n.object && (n.page == 0 || key.page == n.page) && !link.target.empty() {
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
					r.doc.warn("pdf_link_partial_span", "Link rectangle partially covers a text span; exact ownership requires tagged Link structure or finer glyph recovery")
				}
			}
			for _, row := range b.Rows {
				for _, cell := range row.Cells {
					walk(cell.Blocks)
				}
			}
			for _, item := range b.Items {
				walk(item.Blocks)
			}
		}
	}
	walk(r.doc.Blocks)
}
