package manifold

import (
	"fmt"
	"reflect"
)

// PDFNavigation retains explicit PDF annotation/view evidence independently of
// inferred editable-flow anchors. Coordinates in Point/Region use displayed pt;
// Parameters retain original PDF user units and nullable viewer settings.
type PDFNavigation struct {
	Destinations []PDFDestination    `json:"destinations,omitempty"`
	Links        []PDFNavigationLink `json:"links,omitempty"`
}
type PDFDestination struct {
	ID         string     `json:"id"`
	NameKind   string     `json:"name_kind,omitempty"`  // name (legacy dictionary), string (name tree), or empty for anonymous
	NameBytes  []byte     `json:"name_bytes,omitempty"` // identity bytes, not a Unicode-equivalence key
	Page       int        `json:"page,omitempty"`
	PageObject string     `json:"page_object,omitempty"`
	View       string     `json:"view,omitempty"`
	Parameters []*float64 `json:"parameters,omitempty"`
	Point      *Point     `json:"point,omitempty"`
	Region     *Quad      `json:"region,omitempty"`
	Unresolved bool       `json:"unresolved,omitempty"`
	// FlowBinding records inference, never authorial bookmark ownership.
	FlowBinding string `json:"flow_binding,omitempty"` // nearest_content, page_boundary, or unbound (source evidence without a current flow assertion)
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

// A nil catalog validates physical/history syntax independently of an edited
// flow graph. Document.Validate always supplies its current complete catalog.
func (n *PDFNavigation) validate(d *Document, c *destinationCatalog) error {
	if n == nil {
		return nil
	}
	if len(n.Destinations)+len(n.Links) > 100000 {
		return fmt.Errorf("PDF navigation exceeds 100000 entries")
	}
	ids := map[string]bool{}
	names := map[string]bool{}
	for _, v := range n.Destinations {
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
