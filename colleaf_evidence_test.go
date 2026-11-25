package manifold

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func colleafEvidenceExample(t *testing.T) *Document {
	t.Helper()
	physical := readRecovery(t, navigationFixture(fullNavigationViewsForEvidence(), `/foo [6 0 R /FitH 100]`, `(foo)`))
	d := &Document{Metadata: Metadata{Title: "Source history", Author: "Author", Subject: "Subject", PageCount: physical.Metadata.PageCount}, Pages: clonePDFPages(physical.Pages), PDFNavigation: clonePDFNavigation(physical.PDFNavigation), UnresolvedDestinations: append([]string(nil), physical.UnresolvedDestinations...), Diagnostics: append([]Diagnostic(nil), physical.Diagnostics...)}
	duplicate := Diagnostic{Code: "source/历史", Message: "Line one\nLine two 😀"}
	d.Diagnostics = append(d.Diagnostics, duplicate, duplicate)
	b := Block{Kind: Paragraph, Runs: []Run{{Text: "Current 😀", Bold: true, FontSize: 13, Color: "#123456"}}}
	for _, v := range d.PDFNavigation.Destinations {
		if !v.Unresolved {
			b.Anchors = append(b.Anchors, v.ID)
		}
		b.Runs = append(b.Runs, Run{Text: "v", Destination: v.ID})
	}
	d.Blocks = []Block{b}
	return d
}
func fullNavigationViewsForEvidence() string {
	return `<< /Names [(box) [6 0 R /FitR 10 20 30 40] (view) [3 0 R /XYZ null null 0]] >>`
}
func evidenceTarget(d *Document) string {
	for _, v := range d.PDFNavigation.Destinations {
		if !v.Unresolved {
			return v.ID
		}
	}
	return ""
}
func assertEvidenceSF(t *testing.T, want, got *Document) {
	t.Helper()
	a, err := ToStructured(want)
	if err != nil {
		t.Fatal(err)
	}
	b, err := ToStructured(got)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a, b) {
		x, _ := json.Marshal(b)
		y, _ := json.Marshal(a)
		t.Fatalf("evidence/SF graph changed\ngot%s\nwant%s", x, y)
	}
}
func TestColleafDocumentEvidenceRoundTripAndActualSave(t *testing.T) {
	ctx := context.Background()
	want := colleafEvidenceExample(t)
	if len(want.Pages) != 2 || want.Metadata.PageCount != 2 {
		t.Fatal("physical reference changed")
	}
	wire, err := encodeColleafDocument(ctx, want)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(wire)
	got, err := decodeColleafDocument(ctx, wire)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(wire)
	if !bytes.Equal(before, after) {
		t.Fatal("source snapshot mutated")
	}
	if !reflect.DeepEqual(want, got) {
		t.Fatal("document-level metadata/history changed")
	}
	assertEvidenceSF(t, want, got)
	sf, err := ToStructured(got)
	if err != nil {
		t.Fatal(err)
	}
	model, err := FromStructured(sf)
	if err != nil {
		t.Fatal(err)
	}
	again, err := encodeColleafDocument(ctx, model)
	if err != nil {
		t.Fatal(err)
	}
	round, err := decodeColleafDocument(ctx, again)
	if err != nil {
		t.Fatal(err)
	}
	assertEvidenceSF(t, want, round)
	// Returned pointer-bearing evidence must not alias the saved/native payload.
	got.Pages[0].MediaBox[0] = 99
	got.PDFNavigation.Destinations[0].NameBytes[0] ^= 1
	round, err = decodeColleafDocument(ctx, wire)
	if err != nil {
		t.Fatal(err)
	}
	assertEvidenceSF(t, want, round)
	if file := os.Getenv("MANYDOC_COLLEAF_EVIDENCE_QA"); file != "" {
		if err = os.WriteFile(file, before, 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, spec := range []struct{ env, mode string }{{"MANYDOC_COLLEAF_EVIDENCE_SAVED", "saved"}, {"MANYDOC_COLLEAF_EVIDENCE_EDITED", "edited"}, {"MANYDOC_COLLEAF_EVIDENCE_DELETED", "deleted"}, {"MANYDOC_COLLEAF_EVIDENCE_CREATED", "created"}} {
		file := os.Getenv(spec.env)
		if file == "" {
			continue
		}
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var native colleafNoteSnapshot
		if err = json.Unmarshal(raw, &native); err != nil {
			t.Fatal(err)
		}
		envelope, err := json.Marshal(colleafEnvelope{colleafEnvelopeType, 3, "univer", colleafEngineVersion, native})
		if err != nil {
			t.Fatal(err)
		}
		actual, err := readColleaf(ctx, bytes.NewReader(envelope))
		if err != nil {
			t.Fatal(err)
		}
		expected, err := FromStructured(sf)
		if err != nil {
			t.Fatal(err)
		}
		switch spec.mode {
		case "edited":
			anchors := expected.Blocks[0].Anchors
			expected.Blocks[0].Anchors = nil
			runs := []Run{{Text: "Q"}}
			for _, id := range anchors {
				runs = append(runs, Run{Anchor: id})
			}
			expected.Blocks[0].Runs = append(runs, expected.Blocks[0].Runs...)
			expected.Diagnostics = append(expected.Diagnostics, Diagnostic{Code: "colleaf_stale_bookmark_placement", Message: "Edited native bookmark position is retained as a current inline point rather than old block placement"})
		case "deleted":
			target := evidenceTarget(expected)
			var anchors []string
			for _, id := range expected.Blocks[0].Anchors {
				if id != target {
					anchors = append(anchors, id)
				}
			}
			expected.Blocks[0].Anchors = anchors
			expected.UnresolvedDestinations = append(expected.UnresolvedDestinations, target)
			sort.Strings(expected.UnresolvedDestinations)
			expected.Diagnostics = append(expected.Diagnostics, Diagnostic{Code: "colleaf_unresolved_navigation", Message: "Native internal link has no current target; its explicit unresolved identity is retained"}, Diagnostic{Code: "colleaf_pdf_flow_binding", Message: "Original PDF navigation evidence for " + target + " is retained unbound after the editable destination graph changed"})
			for i := range expected.PDFNavigation.Destinations {
				if expected.PDFNavigation.Destinations[i].ID == target {
					expected.PDFNavigation.Destinations[i].FlowBinding = "unbound"
				}
			}
		case "created":
			missing := ""
			for i := range expected.PDFNavigation.Destinations {
				v := &expected.PDFNavigation.Destinations[i]
				if v.Unresolved {
					missing = v.ID
					v.FlowBinding = "unbound"
					break
				}
			}
			expected.Blocks[0].Runs = append([]Run{{Anchor: missing}}, expected.Blocks[0].Runs...)
			var unresolved []string
			for _, id := range expected.UnresolvedDestinations {
				if id != missing {
					unresolved = append(unresolved, id)
				}
			}
			expected.UnresolvedDestinations = unresolved
			expected.Diagnostics = append(expected.Diagnostics, Diagnostic{Code: "colleaf_pdf_flow_binding", Message: "Original PDF navigation evidence for " + missing + " is retained unbound after the editable destination graph changed"})
		}
		assertEvidenceSF(t, expected, actual)
		// New diagnostics become history on export, without repeated reopen growth.
		next, err := encodeColleafDocument(ctx, actual)
		if err != nil {
			t.Fatal(err)
		}
		stable, err := decodeColleafDocument(ctx, next)
		if err != nil {
			t.Fatal(err)
		}
		assertEvidenceSF(t, actual, stable)
	}
}
func TestColleafDocumentEvidenceCurrentGraphEdits(t *testing.T) {
	ctx := context.Background()
	want := colleafEvidenceExample(t)
	wire, err := encodeColleafDocument(ctx, want)
	if err != nil {
		t.Fatal(err)
	}
	target := evidenceTarget(want)
	var ranges []colleafRangeRecord
	for _, a := range wire.Body.CustomRanges {
		p, _ := readColleafBookmark(a)
		if a.Type == 3 && p.BookmarkID == target {
			continue
		}
		ranges = append(ranges, a)
	}
	wire.Body.CustomRanges = ranges
	got, err := decodeColleafDocument(ctx, wire)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, v := range got.PDFNavigation.Destinations {
		if v.ID == target {
			found = v.FlowBinding == "unbound" && !v.Unresolved && v.Page > 0 && v.View != ""
		}
	}
	if !found {
		t.Fatal("physical evidence was erased or a flow anchor invented")
	}
	for _, id := range got.Blocks[0].Anchors {
		if id == target {
			t.Fatal("deleted anchor restored from provenance")
		}
	}
	if !reflect.DeepEqual(got.Diagnostics[:len(want.Diagnostics)], want.Diagnostics) {
		t.Fatal("historical diagnostic order/duplicates lost")
	}
}
func TestColleafDocumentEvidenceMalformed(t *testing.T) {
	ctx := context.Background()
	wire, err := encodeColleafDocument(ctx, colleafEvidenceExample(t))
	if err != nil {
		t.Fatal(err)
	}
	opened, err := openColleafIntent(ctx, wire)
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{`{"version":2}`, `{"version":1,"diagnostics":[{"Code":"x","message":"y"}]}`, `{"version":1,"diagnostics":[{"code":null,"message":"y"}]}`, `{"version":1,"pages":[{"number":1}]}`} {
		bad := opened
		bad.Resources = append([]colleafResource(nil), opened.Resources...)
		for i, r := range bad.Resources {
			if r.Name == colleafDocumentResource {
				var fields map[string]json.RawMessage
				if err = json.Unmarshal([]byte(r.Data), &fields); err != nil {
					t.Fatal(err)
				}
				fields["evidence"] = json.RawMessage(raw)
				encoded, _ := json.Marshal(fields)
				bad.Resources[i].Data = string(encoded)
			}
		}
		if _, err = decodeColleafDocument(ctx, bad); err == nil {
			t.Fatal("invalid evidence accepted", raw)
		}
	}
	// A changed graph must not repair an unknown historical binding enum.
	invalid := opened
	invalid.Body.CustomRanges = append([]colleafRangeRecord(nil), opened.Body.CustomRanges...)
	target := evidenceTarget(colleafEvidenceExample(t))
	var ranges []colleafRangeRecord
	for _, a := range invalid.Body.CustomRanges {
		p, _ := readColleafBookmark(a)
		if a.Type != 3 || p.BookmarkID != target {
			ranges = append(ranges, a)
		}
	}
	invalid.Body.CustomRanges = ranges
	invalid.Resources = append([]colleafResource(nil), opened.Resources...)
	for i, r := range invalid.Resources {
		if r.Name == colleafDocumentResource {
			var intent colleafDocumentIntent
			if err = colleafResourceMap(opened.Resources, colleafDocumentResource, &intent); err != nil {
				t.Fatal(err)
			}
			for j := range intent.Evidence.PDFNavigation.Destinations {
				if intent.Evidence.PDFNavigation.Destinations[j].ID == target {
					intent.Evidence.PDFNavigation.Destinations[j].FlowBinding = "unknown"
				}
			}
			invalid.Resources[i].Data = string(sfAttrs(intent))
		}
	}
	if _, err = decodeColleafDocument(ctx, invalid); err == nil {
		t.Fatal("edited graph repaired malformed physical evidence")
	}
	bad := colleafEvidenceExample(t)
	bad.Diagnostics = []Diagnostic{{Code: "bad", Message: string([]byte{0xff})}}
	if _, err = encodeColleafDocument(ctx, bad); err == nil {
		t.Fatal("invalid diagnostic text replaced silently")
	}
}
func TestPDFNavigationUnboundSourceHistory(t *testing.T) {
	d := colleafEvidenceExample(t)
	target := evidenceTarget(d)
	d.Blocks[0].Anchors = nil
	for i := range d.PDFNavigation.Destinations {
		v := &d.PDFNavigation.Destinations[i]
		if !v.Unresolved {
			v.FlowBinding = "unbound"
			d.UnresolvedDestinations = append(d.UnresolvedDestinations, v.ID)
		}
	}
	if err := d.Validate(); err != nil {
		t.Fatal(err)
	}
	sf, err := ToStructured(d)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := FromStructured(sf)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(d.PDFNavigation, restored.PDFNavigation) {
		t.Fatal("unbound physical views changed")
	}
	for i := range d.PDFNavigation.Destinations {
		if d.PDFNavigation.Destinations[i].ID == target {
			d.PDFNavigation.Destinations[i].FlowBinding = "nearest_content"
		}
	}
	if err = d.Validate(); err == nil || !strings.Contains(err.Error(), "view or flow") {
		t.Fatal("missing anchor claimed a current binding", err)
	}
}

func FuzzColleafDocumentEvidence(f *testing.F) {
	ctx := context.Background()
	page := PDFPage{Number: 1, MediaBox: [4]float64{0, 0, 200, 300}, CropBox: [4]float64{0, 0, 200, 300}, Width: 200, Height: 300, UserUnit: 1, Transform: Matrix{1, 0, 0, -1, 0, 300}}
	d := &Document{Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "Body"}}}}, Pages: []PDFPage{page}, PDFNavigation: &PDFNavigation{Destinations: []PDFDestination{{ID: "view", Page: 1, View: "Fit", FlowBinding: "unbound"}}}, Diagnostics: []Diagnostic{{Code: "source", Message: "history"}}}
	wire, err := encodeColleafDocument(ctx, d)
	if err != nil {
		f.Fatal(err)
	}
	base, err := openColleafIntent(ctx, wire)
	if err != nil {
		f.Fatal(err)
	}
	e, err := colleafEvidence(d)
	if err != nil {
		f.Fatal(err)
	}
	raw, _ := json.Marshal(e)
	f.Add(raw)
	f.Add([]byte(`{"version":1,"diagnostics":[{"code":"x","message":"😀"}]}`))
	f.Add([]byte(`{"version":2}`))
	f.Fuzz(func(t *testing.T, raw []byte) {
		input := base
		input.Resources = append([]colleafResource(nil), base.Resources...)
		for i, r := range input.Resources {
			if r.Name == colleafDocumentResource {
				input.Resources[i].Data = `{"layouts":[],"evidence":` + string(raw) + `}`
			}
		}
		got, err := decodeColleafDocument(ctx, input)
		if err != nil {
			return
		}
		if err = got.Validate(); err != nil {
			t.Fatal(err)
		}
		if _, err = ToStructured(got); err != nil {
			t.Fatal(err)
		}
	})
}
