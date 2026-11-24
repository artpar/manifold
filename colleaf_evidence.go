package manydoc

import "fmt"

// Original source evidence and diagnostic history are nonvisual intent, not a
// cached body and not a claim about the native editor's current page geometry.
type colleafDocumentEvidence struct {
	AnchorOrigins  []colleafOrigin     `json:"anchorOrigins,omitempty"`
	ListOrigins    []colleafOrigin     `json:"listOrigins,omitempty"`
	ImageOrigins   []colleafOrigin     `json:"imageOrigins,omitempty"`
	TableOrigins   []colleafOrigin     `json:"tableOrigins,omitempty"`
	RunOrigins     []colleafOrigin     `json:"runOrigins,omitempty"`
	NoteOrigins    []colleafNoteOrigin `json:"noteOrigins,omitempty"`
	Origins        []colleafOrigin     `json:"origins,omitempty"`
	UnboundSources []SourceInfo        `json:"unboundSources,omitempty"`
	Version        int                 `json:"version"`
	Diagnostics    []Diagnostic        `json:"diagnostics,omitempty"`
	PageCount      int                 `json:"pageCount,omitempty"`
	Pages          []PDFPage           `json:"pages,omitempty"`
	PDFNavigation  *PDFNavigation      `json:"pdfNavigation,omitempty"`
}

func colleafEvidence(d *Document) (*colleafDocumentEvidence, error) {
	if len(d.UnboundSources) == 0 && len(d.Diagnostics) == 0 && d.Metadata.PageCount == 0 && len(d.Pages) == 0 && d.PDFNavigation == nil {
		return nil, nil
	}
	e := &colleafDocumentEvidence{Version: 1, UnboundSources: cloneUnboundSources(d.UnboundSources), Diagnostics: append([]Diagnostic(nil), d.Diagnostics...), PageCount: d.Metadata.PageCount, Pages: clonePDFPages(d.Pages), PDFNavigation: clonePDFNavigation(d.PDFNavigation)}
	if err := e.validate(); err != nil {
		return nil, err
	}
	return e, nil
}
func (e *colleafDocumentEvidence) validate() error {
	if e == nil {
		return nil
	}
	if e.Version != 1 {
		return fmt.Errorf("%w: native document evidence version", ErrUnsupported)
	}
	for _, d := range e.Diagnostics {
		if err := validateString(d.Code); err != nil {
			return err
		}
		if err := validateString(d.Message); err != nil {
			return err
		}
	}
	if err := validatePDFPages(e.Pages); err != nil {
		return err
	}
	for i := range e.UnboundSources {
		if err := e.UnboundSources[i].validate(e.Pages); err != nil {
			return err
		}
	}
	for _, o := range e.AnchorOrigins {
		if err := validateColleafOriginProfile(o, e.Pages, colleafAnchorOriginProfile); err != nil {
			return err
		}
	}
	for _, o := range e.ListOrigins {
		if err := validateColleafListOrigin(o, e.Pages); err != nil {
			return err
		}
	}
	for _, o := range e.ImageOrigins {
		if err := validateColleafOriginProfile(o, e.Pages, colleafImageOriginProfile); err != nil {
			return err
		}
	}
	for _, o := range e.TableOrigins {
		if err := validateColleafTableOrigin(o, e.Pages); err != nil {
			return err
		}
	}
	for _, o := range e.RunOrigins {
		if err := validateColleafOriginProfile(o, e.Pages, colleafRunOriginProfile); err != nil {
			return err
		}
	}
	seenNotes := map[string]bool{}
	for _, o := range e.NoteOrigins {
		if err := o.validate(e.Pages); err != nil {
			return err
		}
		if seenNotes[o.NativeID] {
			return fmt.Errorf("duplicate native note source record")
		}
		seenNotes[o.NativeID] = true
	}
	for _, o := range e.Origins {
		if err := validateColleafOrigin(o, e.Pages); err != nil {
			return err
		}
	}
	return e.PDFNavigation.validate(&Document{Pages: e.Pages}, nil)
}
func restoreColleafEvidence(d *Document, e *colleafDocumentEvidence) error {
	if e == nil {
		return nil
	}
	if err := e.validate(); err != nil {
		return err
	}
	d.UnboundSources = cloneUnboundSources(e.UnboundSources)
	d.Metadata.PageCount = e.PageCount
	d.Pages = clonePDFPages(e.Pages)
	d.PDFNavigation = clonePDFNavigation(e.PDFNavigation)
	// Preserve historical order and duplicates exactly. Deduplicate only newly
	// generated warnings against history so repeated save/reopen does not grow it.
	history := append([]Diagnostic(nil), e.Diagnostics...)
	seen := map[Diagnostic]bool{}
	for _, v := range history {
		seen[v] = true
	}
	for _, v := range d.Diagnostics {
		if !seen[v] {
			history = append(history, v)
			seen[v] = true
		}
	}
	d.Diagnostics = history
	if d.PDFNavigation == nil {
		return nil
	}
	graph, err := catalogDestinations(d, false)
	if err != nil {
		return err
	}
	for i := range d.PDFNavigation.Destinations {
		view := &d.PDFNavigation.Destinations[i]
		detached := !view.Unresolved && !graph.anchors[view.ID] || view.Unresolved && !graph.missing[view.ID]
		if detached && view.FlowBinding != "unbound" {
			view.FlowBinding = "unbound"
			warning := Diagnostic{Code: "colleaf_pdf_flow_binding", Message: "Original PDF navigation evidence for " + view.ID + " is retained unbound after the editable destination graph changed"}
			if !seen[warning] {
				d.Diagnostics = append(d.Diagnostics, warning)
				seen[warning] = true
			}
		}
	}
	return nil
}
