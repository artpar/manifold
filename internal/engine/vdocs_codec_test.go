package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestVDOCSEnvelopeRichSFAndIndependentSave(t *testing.T) {
	ctx := context.Background()
	want := vdocsSectionExample(t)
	var out bytes.Buffer
	if err := writeVDOCS(ctx, &out, want); err != nil {
		t.Fatal(err)
	}
	var wire map[string]json.RawMessage
	if err := json.Unmarshal(out.Bytes(), &wire); err != nil {
		t.Fatal(err)
	}
	if string(wire["type"]) != `"manifold-document"` || string(wire["version"]) != `3` || string(wire["engineVersion"]) != `"1.0.4"` {
		t.Fatal("wrong native identity")
	}
	for _, data := range [][]byte{out.Bytes(), bytes.ReplaceAll(out.Bytes(), []byte("😀"), []byte(`\ud83d\ude00`))} {
		got, err := readVDOCS(ctx, bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatal("envelope changed rich section/asset/metadata semantics", got.Diagnostics)
		}
		sf, err := ToStructured(got)
		if err != nil {
			t.Fatal(err)
		}
		model, err := FromStructured(sf)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(model, want) {
			t.Fatal("SF changed rich semantics")
		}
	}
	if file := os.Getenv("MANYDOC_VDOCS_ENVELOPE_SAVED"); file != "" {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		got, err := readVDOCS(ctx, bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatal("actual facade changed rich native semantics", got.Diagnostics)
		}
	}
	if file := os.Getenv("MANYDOC_VDOCS_ENVELOPE_QA"); file != "" {
		if err := os.WriteFile(file, out.Bytes(), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if file := os.Getenv("MANYDOC_VDOCS_ENVELOPE_SAVED"); file != "" {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		got, err := readVDOCS(ctx, bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatal("real saved envelope changed semantics", got.Diagnostics)
		}
	}
}

func TestVDOCSEnvelopeRejectsMalformedAmbiguousAndUnknown(t *testing.T) {
	var out bytes.Buffer
	if err := writeVDOCS(context.Background(), &out, vdocsSectionExample(t)); err != nil {
		t.Fatal(err)
	}
	original := out.String()
	for _, pair := range [][2]string{
		{`"version":3`, `"version":2`}, {`"engineVersion":"1.0.4"`, `"engineVersion":"2.0"`}, {`"engine":"univer"`, `"engine":"other"`},
		{`"type":"manifold-document"`, `"type":"sf"`}, {`"type":`, `"Type":`},
		{`"version":3`, `"version":3,"\u0076ersion":3`}, {`"locale":"enUS"`, `"locale":"enUS","unknownVisibleStory":{}`},
		{`"dataStream":`, `"unknownBody":{},"dataStream":`},
		{`"dataStream":`, `"DataStream":`}, {`"startIndex":0`, `"StartIndex":0`},
		{`"startIndex":0`, `"startIndex":null`}, {`"startIndex":0`, `"startIndex":0,"StartIndex":5`},
		{`😀`, `\ud83d`}, {`😀`, string([]byte{0xff})},
		{`"title":"Native sections"`, `"title":"Native sections","title":"Different"`},
	} {
		if !strings.Contains(original, pair[0]) {
			t.Fatalf("mutation missing %q", pair[0])
		}
		data := strings.Replace(original, pair[0], pair[1], 1)
		if _, err := readVDOCS(context.Background(), strings.NewReader(data)); err == nil {
			t.Fatalf("accepted %s", pair[1])
		}
	}
	for _, data := range []string{original + `{}`, `null`, `[]`, `{}`, `{"type":"manifold-document","version":3,"engine":"univer","engineVersion":"1.0.4","document":null}`} {
		if _, err := readVDOCS(context.Background(), strings.NewReader(data)); err == nil {
			t.Fatal("accepted malformed envelope")
		}
	}
	// Duplicate keys inside JSON-in-a-string cannot be seen by an outer decoder.

	// Returned pointer-bearing evidence must not alias the saved/native payload.

	wire, err := encodeVDOCSDocument(context.Background(), vdocsSectionExample(t))
	if err != nil {
		t.Fatal(err)
	}
	wire.Resources = append(wire.Resources, vdocsResource{Name: vdocsDocumentResource, Data: `{"author":"first","\u0061uthor":"second"}`})
	raw, err := json.Marshal(vdocsEnvelope{vdocsEnvelopeType, 3, "univer", vdocsEngineVersion, wire})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = readVDOCS(context.Background(), bytes.NewReader(raw)); err == nil || !strings.Contains(err.Error(), "duplicate object member") {
		t.Fatal("embedded resource was collapsed", err)
	}
}

func TestVDOCSEnvelopeIOAndCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := readVDOCS(ctx, strings.NewReader(`{}`)); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err := writeVDOCS(ctx, io.Discard, vdocsSectionExample(t)); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := readVDOCS(context.Background(), nil); err == nil {
		t.Fatal("nil reader accepted")
	}
	if err := writeVDOCS(context.Background(), nil, vdocsSectionExample(t)); err == nil {
		t.Fatal("nil writer accepted")
	}
}

func FuzzVDOCSEnvelope(f *testing.F) {
	var out bytes.Buffer
	if err := writeVDOCS(context.Background(), &out, vdocsSectionExample(f)); err != nil {
		f.Fatal(err)
	}
	f.Add(out.Bytes())
	f.Add([]byte(`null`))
	f.Add([]byte(`{"version":3,"version":2}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		d, err := readVDOCS(context.Background(), bytes.NewReader(data))
		if err != nil {
			return
		}
		if err = d.Validate(); err != nil {
			t.Fatal(err)
		}
		if _, err = ToStructured(d); err != nil {
			t.Fatal(err)
		}
	})
}

func TestVDOCSEnvelopeViewSettingsAndSourceValidation(t *testing.T) {
	ctx := context.Background()
	var out bytes.Buffer
	if err := writeVDOCS(ctx, &out, vdocsSectionExample(t)); err != nil {
		t.Fatal(err)
	}
	original := out.String()
	for _, settings := range []string{`null`, `{"unknown":true}`, `{"zoomRatio":0}`, `{"zoomRatio":-1}`, `{"zoomRatio":1e400}`} {
		data := strings.Replace(original, `"locale":"enUS"`, `"locale":"enUS","settings":`+settings, 1)
		if _, err := readVDOCS(ctx, strings.NewReader(data)); err == nil {
			t.Fatal("accepted invalid settings", settings)
		}
	}
	data := strings.Replace(original, `"locale":"enUS"`, `"locale":"frFR","settings":{"zoomRatio":1.25}`, 1)
	got, err := readVDOCS(ctx, strings.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	codes := map[string]bool{}
	for _, d := range got.Diagnostics {
		codes[d.Code] = true
	}
	if !codes["vdocs_view_zoom"] ||

		// New diagnostics become history on export, without repeated reopen growth.

		!codes["vdocs_editor_locale"] {
		t.Fatal("unrepresented editor view settings not disclosed", got.Diagnostics)
	}
	for _, d := range []*Document{{}, {Blocks: []Block{{Kind: Paragraph}}}, {Metadata: Metadata{Title: "Unicode 😀 �"}, Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "Text 😀 �"}}}}}} {
		out.Reset()
		if err := writeVDOCS(ctx, &out, d); err != nil {
			t.Fatal(err)
		}
		if _, err := readVDOCS(ctx, bytes.NewReader(out.Bytes())); err != nil {
			t.Fatal(err)
		}
	}
	invalid := vdocsSectionExample(t)
	invalid.Metadata.Title = string([]byte{0xff})
	out.Reset()
	if err := writeVDOCS(ctx, &out, invalid); err == nil || out.Len() != 0 {
		t.Fatal("writer silently replaced source Unicode", err)
	}
}
func vdocsEvidenceExample(t *testing.T) *Document {
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
func TestVDOCSDocumentEvidenceRoundTripAndActualSave(t *testing.T) {
	ctx := context.Background()
	want := vdocsEvidenceExample(t)
	if len(want.Pages) != 2 || want.Metadata.PageCount != 2 {
		t.Fatal("physical reference changed")
	}
	wire, err := encodeVDOCSDocument(ctx, want)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(wire)
	got, err := decodeVDOCSDocument(ctx, wire)
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
	again, err := encodeVDOCSDocument(ctx, model)
	if err != nil {
		t.Fatal(err)
	}
	round, err := decodeVDOCSDocument(ctx, again)
	if err != nil {
		t.Fatal(err)
	}
	assertEvidenceSF(t, want, round)

	got.Pages[0].MediaBox[0] = 99
	got.PDFNavigation.Destinations[0].NameBytes[0] ^= 1
	round, err = decodeVDOCSDocument(ctx, wire)
	if err != nil {
		t.Fatal(err)
	}
	assertEvidenceSF(t, want, round)
	if file := os.Getenv("MANYDOC_VDOCS_EVIDENCE_QA"); file != "" {
		if err = os.WriteFile(file, before, 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, spec := range []struct{ env, mode string }{{"MANYDOC_VDOCS_EVIDENCE_SAVED", "saved"}, {"MANYDOC_VDOCS_EVIDENCE_EDITED", "edited"}, {"MANYDOC_VDOCS_EVIDENCE_DELETED", "deleted"}, {"MANYDOC_VDOCS_EVIDENCE_CREATED", "created"}} {
		file := os.Getenv(spec.env)
		if file == "" {
			continue
		}
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var native vdocsNoteSnapshot
		if err = json.Unmarshal(raw, &native); err != nil {
			t.Fatal(err)
		}
		envelope, err := json.Marshal(vdocsEnvelope{vdocsEnvelopeType, 3, "univer", vdocsEngineVersion, native})
		if err != nil {
			t.Fatal(err)
		}
		actual, err := readVDOCS(ctx, bytes.NewReader(envelope))
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
			expected.Diagnostics = append(expected.Diagnostics, Diagnostic{Code: "vdocs_stale_bookmark_placement", Message: "Edited native bookmark position is retained as a current inline point rather than old block placement"})
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
			expected.Diagnostics = append(expected.Diagnostics, Diagnostic{Code: "vdocs_unresolved_navigation", Message: "Native internal link has no current target; its explicit unresolved identity is retained"}, Diagnostic{Code: "vdocs_pdf_flow_binding", Message: "Original PDF navigation evidence for " + target + " is retained unbound after the editable destination graph changed"})
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
			expected.Diagnostics = append(expected.Diagnostics, Diagnostic{Code: "vdocs_pdf_flow_binding", Message: "Original PDF navigation evidence for " + missing + " is retained unbound after the editable destination graph changed"})
		}
		assertEvidenceSF(t, expected, actual)

		next, err := encodeVDOCSDocument(ctx, actual)
		if err != nil {
			t.Fatal(err)
		}
		stable, err := decodeVDOCSDocument(ctx, next)
		if err != nil {
			t.Fatal(err)
		}
		assertEvidenceSF(t, actual, stable)
	}
}
func TestVDOCSDocumentEvidenceCurrentGraphEdits(t *testing.T) {
	ctx := context.Background()
	want := vdocsEvidenceExample(t)
	wire, err := encodeVDOCSDocument(ctx, want)
	if err != nil {
		t.Fatal(err)
	}
	target := evidenceTarget(want)
	var ranges []vdocsRangeRecord
	for _, a := range wire.Body.CustomRanges {
		p, _ := readVDOCSBookmark(a)
		if a.Type == 3 && p.BookmarkID == target {
			continue
		}
		ranges = append(ranges, a)
	}
	wire.Body.CustomRanges = ranges
	got, err := decodeVDOCSDocument(ctx, wire)
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
func TestVDOCSDocumentEvidenceMalformed(t *testing.T) {
	ctx := context.Background()
	wire, err := encodeVDOCSDocument(ctx, vdocsEvidenceExample(t))
	if err != nil {
		t.Fatal(err)
	}
	opened, err := openVDOCSIntent(ctx, wire)
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{`{"version":2}`, `{"version":1,"diagnostics":[{"Code":"x","message":"y"}]}`, `{"version":1,"diagnostics":[{"code":null,"message":"y"}]}`, `{"version":1,"pages":[{"number":1}]}`} {
		bad := opened
		bad.Resources = append([]vdocsResource(nil), opened.Resources...)
		for i, r := range bad.Resources {
			if r.Name == vdocsDocumentResource {
				var fields map[string]json.RawMessage
				if err = json.Unmarshal([]byte(r.Data), &fields); err != nil {
					t.Fatal(err)
				}
				fields["evidence"] = json.RawMessage(raw)
				encoded, _ := json.Marshal(fields)
				bad.Resources[i].Data = string(encoded)
			}
		}
		if _, err = decodeVDOCSDocument(ctx, bad); err == nil {
			t.Fatal("invalid evidence accepted", raw)
		}
	}
	// A changed graph must not repair an unknown historical binding enum.

	// TextX may introduce a style interval boundary without changing typography.

	invalid := opened
	invalid.Body.CustomRanges = append([]vdocsRangeRecord(nil), opened.Body.CustomRanges...)
	target := evidenceTarget(vdocsEvidenceExample(t))
	var ranges []vdocsRangeRecord
	for _, a := range invalid.Body.CustomRanges {
		p, _ := readVDOCSBookmark(a)
		if a.Type != 3 || p.BookmarkID != target {
			ranges = append(ranges, a)
		}
	}
	invalid.Body.CustomRanges = ranges
	invalid.Resources = append([]vdocsResource(nil), opened.Resources...)
	for i, r := range invalid.Resources {
		if r.Name == vdocsDocumentResource {
			var intent vdocsDocumentIntent
			if err = vdocsResourceMap(opened.Resources, vdocsDocumentResource, &intent); err != nil {
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
	if _, err = decodeVDOCSDocument(ctx, invalid); err == nil {
		t.Fatal("edited graph repaired malformed physical evidence")
	}
	bad := vdocsEvidenceExample(t)
	bad.Diagnostics = []Diagnostic{{Code: "bad", Message: string([]byte{0xff})}}
	if _, err = encodeVDOCSDocument(ctx, bad); err == nil {
		t.Fatal("invalid diagnostic text replaced silently")
	}
}
func TestPDFNavigationUnboundSourceHistory(t *testing.T) {
	d := vdocsEvidenceExample(t)
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

func FuzzVDOCSDocumentEvidence(f *testing.F) {
	ctx := context.Background()
	page := PDFPage{Number: 1, MediaBox: [4]float64{0, 0, 200, 300}, CropBox: [4]float64{0, 0, 200, 300}, Width: 200, Height: 300, UserUnit: 1, Transform: Matrix{1, 0, 0, -1, 0, 300}}
	d := &Document{Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "Body"}}}}, Pages: []PDFPage{page}, PDFNavigation: &PDFNavigation{Destinations: []PDFDestination{{ID: "view", Page: 1, View: "Fit", FlowBinding: "unbound"}}}, Diagnostics: []Diagnostic{{Code: "source", Message: "history"}}}
	wire, err := encodeVDOCSDocument(ctx, d)
	if err != nil {
		f.Fatal(err)
	}
	base, err := openVDOCSIntent(ctx, wire)
	if err != nil {
		f.Fatal(err)
	}
	e, err := vdocsEvidence(d)
	if err != nil {
		f.Fatal(err)
	}
	raw, _ := json.Marshal(e)
	f.Add(raw)
	f.Add([]byte(`{"version":1,"diagnostics":[{"code":"x","message":"😀"}]}`))
	f.Add([]byte(`{"version":2}`))
	f.Fuzz(func(t *testing.T, raw []byte) {
		input := base
		input.Resources = append([]vdocsResource(nil), base.Resources...)
		for i, r := range input.Resources {
			if r.Name == vdocsDocumentResource {
				input.Resources[i].Data = `{"layouts":[],"evidence":` + string(raw) + `}`
			}
		}
		got, err := decodeVDOCSDocument(ctx, input)
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
func TestVDOCSNativeIntentCarrierAndLegacyRead(t *testing.T) {
	ctx := context.Background()
	wire, err := encodeVDOCSNotes(ctx, vdocsImageExample(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(wire.Resources) != 0 {
		t.Fatal("unregistered resources would be removed by FDocument.save")
	}
	carrier := wire.Body.CustomRanges[len(wire.Body.CustomRanges)-1]
	if carrier.Type != 5 || carrier.StartIndex != vdocsUTF16Len(wire.Body.DataStream)-1 || carrier.EndIndex != carrier.StartIndex {
		t.Fatal("carrier is not on terminal LF", carrier)
	}
	before, err := json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := openVDOCSIntent(ctx, wire)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(wire)
	if string(before) != string(after) {
		t.Fatal("opening intent mutated caller")
	}
	if len(legacy.Resources) != 3 || len(legacy.Body.CustomRanges) != 1 {
		t.Fatal("incorrect intent transport extraction")
	}
	doc, err := decodeVDOCSNotes(ctx, legacy, vdocsTextContext{})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(doc, vdocsImageExample(t)) {
		t.Fatal("legacy direct-resource input changed semantics")
	}
	for _, mutate := range []func(*vdocsNoteSnapshot){
		func(w *vdocsNoteSnapshot) {
			a := w.Body.CustomRanges[len(w.Body.CustomRanges)-1]
			a.ID = "duplicate"
			w.Body.CustomRanges = append(w.Body.CustomRanges, a)
		},
		func(w *vdocsNoteSnapshot) {
			w.Body.CustomRanges[len(w.Body.CustomRanges)-1].Properties = sfAttrs(vdocsIntentCarrier{Profile: vdocsIntentProfile, Version: 2})
		},
		func(w *vdocsNoteSnapshot) {
			w.Body.CustomRanges[len(w.Body.CustomRanges)-1].Properties = sfAttrs(vdocsIntentCarrier{Profile: vdocsIntentProfile, Version: 1, Records: []vdocsResource{{Name: "unknown", Data: "{}"}}})
		},
		func(w *vdocsNoteSnapshot) {
			w.Body.CustomRanges[len(w.Body.CustomRanges)-1].EndIndex = vdocsUTF16Len(w.Body.DataStream)
		},
	} {
		input, err := encodeVDOCSNotes(ctx, vdocsImageExample(t))
		if err != nil {
			t.Fatal(err)
		}
		mutate(&input)
		if _, err = decodeVDOCSNotes(ctx, input, vdocsTextContext{}); err == nil {
			t.Fatal("invalid carrier accepted")
		}
	}
}
func TestVDOCSFacadeSavedIntentSemantics(t *testing.T) {
	originals := map[string]*Document{"notes": vdocsNoteExample(), "lists": resolveNumberingDocument(vdocsListExample()), "images": vdocsImageExample(t)}
	snapshots := map[string]vdocsNoteSnapshot{}
	for name, doc := range originals {
		wire, err := encodeVDOCSNotes(context.Background(), doc)
		if err != nil {
			t.Fatal(err)
		}
		snapshots[name] = wire
	}
	if file := os.Getenv("MANYDOC_VDOCS_FACADE_QA"); file != "" {
		data, err := json.MarshalIndent(snapshots, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(file, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if file := os.Getenv("MANYDOC_VDOCS_FACADE_SAVED"); file != "" {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var saved map[string]vdocsNoteSnapshot
		if err = json.Unmarshal(data, &saved); err != nil {
			t.Fatal(err)
		}
		for name, original := range originals {
			wire, ok := saved[name]
			if !ok {
				t.Fatal("missing facade-saved case", name)
			}
			doc, err := decodeVDOCSNotes(context.Background(), wire, vdocsTextContext{})
			if err != nil {
				t.Fatal(name, err)
			}
			if !reflect.DeepEqual(doc, original) {
				a, _ := json.Marshal(doc)
				b, _ := json.Marshal(original)
				t.Fatalf("facade changed %s\ngot%s\nwant%s", name, a, b)
			}
		}
	}
	if file := os.Getenv("MANYDOC_VDOCS_FACADE_EDITED"); file != "" {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var wire vdocsNoteSnapshot
		if err = json.Unmarshal(data, &wire); err != nil {
			t.Fatal(err)
		}
		doc, err := decodeVDOCSNotes(context.Background(), wire, vdocsTextContext{})
		if err != nil {
			t.Fatal(err)
		}
		want := resolveNumberingDocument(vdocsListExample())
		want.Blocks[0].Items[0].Blocks[0].Runs[0].Text = "XParent one"

		gotText := noteLabelContentText(doc.Blocks[0].Items[0].Blocks[0].Runs)
		if gotText != "XParent one" {
			t.Fatal("facade edit lost", gotText)
		}
		doc.Blocks[0].Items[0].Blocks[0].Runs = []Run{{Text: gotText}}
		if !reflect.DeepEqual(doc, want) {
			t.Fatal("facade edit lost graph/ownership", doc.Diagnostics)
		}
	}
}
func TestVDOCSRegisteredDrawingMirrorMustMatchPersistedSource(t *testing.T) {
	wire, err := encodeVDOCSNotes(context.Background(), vdocsImageExample(t))
	if err != nil {
		t.Fatal(err)
	}
	resource := vdocsResource{Name: "DOC_DRAWING_PLUGIN", Data: string(sfAttrs(map[string]any{"data": wire.Drawings, "order": wire.DrawingOrder}))}
	if err = checkVDOCSDrawingMirror(wire, resource); err != nil {
		t.Fatal(err)
	}
	resource.Data = string(sfAttrs(map[string]any{"data": map[string]json.RawMessage{"orphan": wire.Drawings[wire.DrawingOrder[0]]}, "order": []string{"orphan"}}))
	if err = checkVDOCSDrawingMirror(wire, resource); err == nil {
		t.Fatal("orphan resource source silently discarded")
	}
}

func TestVDOCSEditorPermissionResourceIsOutsideDocumentInterpretation(t *testing.T) {
	want := vdocsNoteExample()
	wire, err := encodeVDOCSNotes(context.Background(), want)
	if err != nil {
		t.Fatal(err)
	}
	wire.Resources = append(wire.Resources, vdocsResource{Name: "DOC_OBJECT_PERMISSION_PLUGIN", Data: `{"actor":"editor-local","editable":false}`})
	got, err := decodeVDOCSNotes(context.Background(), wire, vdocsTextContext{})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatal("SDK permission state changed document interpretation or admission", got.Diagnostics)
	}
}
func FuzzVDOCSIntent(f *testing.F) {
	wire, err := encodeVDOCSNotes(context.Background(), vdocsNoteExample())
	if err != nil {
		f.Fatal(err)
	}
	data, err := json.Marshal(wire)
	if err != nil {
		f.Fatal(err)
	}
	f.Add(data)
	f.Add([]byte(`{"body":{"dataStream":"\r\n","customRanges":[{"startIndex":1,"endIndex":1,"rangeId":"manydoc-intent-v1","rangeType":5,"properties":{"profile":"manydoc.native-intent","version":1,"records":[]}}]}}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		var input vdocsNoteSnapshot
		if json.Unmarshal(data, &input) != nil {
			return
		}
		doc, err := decodeVDOCSNotes(context.Background(), input, vdocsTextContext{})
		if err == nil {
			if err = doc.Validate(); err != nil {
				t.Fatal("accepted invalid intent/document graph", err)
			}
		}
	})
}
func TestVDOCSUTF16Boundaries(t *testing.T) {
	index, err := newVDOCSTextIndex(context.Background(), "A😀B\r\n")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(index.byteAt, []int{0, 1, -1, 5, 6, 7, 8}) {
		t.Fatal("UTF-16 offsets confused with bytes/runes", index.byteAt)
	}
	if text, err := index.exclusive(1, 3); err != nil || text != "😀" {
		t.Fatal(text, err)
	}
	if text, err := index.inclusive(1, 2); err != nil || text != "😀" {
		t.Fatal(text, err)
	}
	for _, rangeTest := range [][2]int{{-1, 1}, {1, 2}, {2, 3}, {5, 4}, {0, 7}} {
		if _, err := index.exclusive(rangeTest[0], rangeTest[1]); err == nil {
			t.Fatal("invalid boundary accepted", rangeTest)
		}
	}
	if _, err := index.inclusive(1, 1); err == nil {
		t.Fatal("inclusive range split a surrogate pair")
	}
	if _, err := newVDOCSTextIndex(context.Background(), string([]byte{0xff})); err == nil {
		t.Fatal("invalid UTF-8 accepted")
	}
}
func vdocsWireFixture(t *testing.T) vdocsBody {
	t.Helper()
	data, err := os.ReadFile("testdata/univer/native-stream.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Document struct {
			Body vdocsBody `json:"body"`
		} `json:"document"`
	}
	if err = json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	return fixture.Document.Body
}
func TestVDOCSRichNativeTokenGrammarAndMetadata(t *testing.T) {
	body := vdocsWireFixture(t)
	index, err := indexVDOCSBody(context.Background(), body)
	if err != nil {
		t.Fatal(err)
	}
	if len(index.sections) != 1 || len(index.tables) != 2 || len(index.blocks) != 1 {
		t.Fatal("rich native structure flattened", index)
	}
	for _, table := range body.Tables {
		if text, err := index.text.exclusive(table.StartIndex, table.EndIndex); err != nil || text[0] != vdocsTableStart || text[len(text)-1] != vdocsTableEnd {
			t.Fatal("table extent changed", table, err)
		}
	}
	if text, err := index.text.inclusive(body.CustomRanges[0].StartIndex, body.CustomRanges[0].EndIndex); err != nil || text != "😀" {
		t.Fatal("native inclusive annotation extent", text, err)
	}
	if output := os.Getenv("MANYDOC_VDOCS_QA_TREE"); output != "" {
		var proof func([]vdocsStreamNode) []map[string]any
		proof = func(nodes []vdocsStreamNode) []map[string]any {
			out := make([]map[string]any, 0, len(nodes))
			for _, node := range nodes {
				out = append(out, map[string]any{"kind": node.kind, "start": node.start, "end": node.end, "children": proof(node.children)})
			}
			return out
		}
		data, err := json.MarshalIndent(proof(index.sections), "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(output, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
}
func TestVDOCSMalformedNativeBoundsAndTokens(t *testing.T) {
	for _, change := range []func(*vdocsBody){
		func(b *vdocsBody) { b.TextRuns[0].Ed = 2 },
		func(b *vdocsBody) { b.TextRuns[0].St = 4; b.TextRuns[0].Ed = 3 },
		func(b *vdocsBody) { b.Paragraphs[0].StartIndex = 2 },
		func(b *vdocsBody) { b.SectionBreaks[0].StartIndex = 0 },
		func(b *vdocsBody) { b.Tables[0].EndIndex-- },
		func(b *vdocsBody) { b.Tables = b.Tables[:1] },
		func(b *vdocsBody) { b.CustomBlocks = nil },
		func(b *vdocsBody) { b.CustomRanges[0].EndIndex = 1 },
		func(b *vdocsBody) { b.RenderedPageBreaks = []int{0} },
		func(b *vdocsBody) { b.DataStream = strings.Replace(b.DataStream, "\x0e", "\x1e", 1) },
		func(b *vdocsBody) { b.DataStream = strings.Replace(b.DataStream, "\r\n\x1d", "\r\x1d", 1) },
	} {
		body := vdocsWireFixture(t)
		change(&body)
		if _, err := indexVDOCSBody(context.Background(), body); err == nil {
			t.Fatal("contradictory native offsets/structure accepted")
		}
	}
	for _, text := range []string{"", "plain", "x\n", "\x1cA\r\n\x1d\r\n", "\x1a\x1b\x0e\x0f\r\n", "\x1fA\r\n"} {
		if _, err := parseVDOCSStream(context.Background(), text); err == nil {
			t.Fatal("malformed native grammar accepted", text)
		}
	}
}

func TestVDOCSStreamCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := indexVDOCSBody(ctx, vdocsWireFixture(t)); err != context.Canceled {
		t.Fatal("native validation ignored cancellation", err)
	}
}

func TestVDOCSCrossParagraphRangesAndSoftBreakPositions(t *testing.T) {
	body := vdocsBody{DataStream: "\x1fA\rB\x1e\fC\r\n", RenderedPageBreaks: []int{5}}
	if _, err := indexVDOCSBody(context.Background(), body); err != nil {
		t.Fatal("valid spanning markers/soft break rejected", err)
	}
	index, err := newVDOCSTextIndex(context.Background(), "a\u0301😀")
	if err != nil || index.length() != 4 {
		t.Fatal("input normalized before offset interpretation", index, err)
	}
	if text, err := index.exclusive(0, 2); err != nil || text != "a\u0301" {
		t.Fatal("combining scalars rewritten", text, err)
	}
}

func FuzzVDOCSStream(f *testing.F) {
	raw, err := os.ReadFile("testdata/univer/native-stream.json")
	if err != nil {
		f.Fatal(err)
	}
	var fixture struct {
		Document struct {
			Body vdocsBody `json:"body"`
		} `json:"document"`
	}
	if err := json.Unmarshal(raw, &fixture); err != nil {
		f.Fatal(err)
	}
	for _, seed := range []string{"\r\n", "A😀B\r\n", fixture.Document.Body.DataStream, "\x1a\x1b\x1cA\r\n\x1d\x1e\x1f\r\n"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, text string) {
		if len(text) > 1<<20 {
			t.Skip()
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_, _ = parseVDOCSStream(ctx, text)
	})
}

func TestVDOCSNativeOverlappingStyleIntervals(t *testing.T) {
	body := vdocsBody{DataStream: "ABCD\r\n", TextRuns: []vdocsTextRun{{St: 1, Ed: 4, TS: json.RawMessage(`{"it":1}`)}, {St: 0, Ed: 3, TS: json.RawMessage(`{"bl":1}`)}, {St: 4, Ed: 4}}}
	if _, err := indexVDOCSBody(context.Background(), body); err != nil {
		t.Fatal("valid native overlapping/unsorted/empty insertion styles rejected", err)
	}
}
