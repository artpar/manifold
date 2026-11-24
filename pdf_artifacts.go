package manydoc

import (
	"fmt"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// PDFArtifact records an explicit marked-content artifact, independently of any
// inferred reusable header/footer template. BBox uses default user-space units (not the current content CTM); Bounds
// uses Matrix to map it into displayed physical points. Content refers back here
// through SourceInfo.Artifacts, including nested marked-content and images.
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
				out = append(out, id)
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
