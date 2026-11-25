package manifold

import (
	"bytes"
	"context"
	"encoding/hex"
	"math"
	"reflect"
	"strings"
	"testing"
)

func navigationFixture(tree, legacy, target string) []byte {
	return recoveryPDF(
		`<< /Type /Catalog /Pages 2 0 R /Names << /Dests 8 0 R >> /Dests << `+legacy+` >> >>`,
		`<< /Type /Pages /Kids [3 0 R 6 0 R] /Count 2 >>`,
		`<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 400] /Resources << /Font << /F 4 0 R >> >> /Contents 5 0 R /Annots [9 0 R 10 0 R 11 0 R] >>`,
		`<< /Type /Font /Subtype /Type1 /BaseFont /Courier >>`, recoveryStream("", `BT /F 10 Tf 20 300 Td (LINK) Tj ET BT /F 10 Tf 150 300 Td (ARRAY) Tj ET`),
		`<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 400] /CropBox [20 20 220 320] /Rotate 90 /UserUnit 2 /Resources << /Font << /F 4 0 R >> >> /Contents 7 0 R >>`,
		recoveryStream("", `BT /F 10 Tf 30 280 Td (TARGET) Tj ET`), tree,
		`<< /Type /Annot /Subtype /Link /Rect [15 290 60 315] /Dest `+target+` >>`,
		`<< /Type /Annot /Subtype /Link /Rect [145 290 190 315] /A << /S /GoTo /D [6 0 R /FitR 20 100 80 300] >> >>`,
		`<< /Type /Annot /Subtype /Link /Rect [0 0 1 1] /A << /S /GoTo /D (missing) >> >>`)
}
func fullNavigationTree() string {
	return `<< /Names [
 (b) [6 0 R /FitB]
 (bh) [6 0 R /FitBH null]
 (bv) [6 0 R /FitBV 50]
 (fit) [6 0 R /Fit]
 (foo) << /D [6 0 R /XYZ 20 300 null] >>
 (h) [6 0 R /FitH 300]
 (null) [6 0 R /XYZ null null 0]
 (partial) [6 0 R /XYZ null 300 null]
 (r) [6 0 R /FitR 20 100 80 300]
 (v) [6 0 R /FitV 50]
 <feff0066006f006f> [6 0 R /FitBV 80]
 <ff00> [6 0 R /FitH null]
 ] >>`
}
func TestPDFNavigationViewsIdentityAndFlow(t *testing.T) {
	data := navigationFixture(fullNavigationTree(), `/foo [6 0 R /FitH 100]`, `(foo)`)
	d := readRecovery(t, data)
	n := d.PDFNavigation
	if n == nil || len(n.Destinations) != 15 || len(n.Links) != 3 {
		t.Fatalf("navigation missing: %+v", n)
	}
	byName := map[string]PDFDestination{}
	ids := map[string]bool{}
	for _, v := range n.Destinations {
		if ids[v.ID] {
			t.Fatal("identity collision")
		}
		ids[v.ID] = true
		byName[v.NameKind+":"+hex.EncodeToString(v.NameBytes)] = v
	}
	foo := byName["string:666f6f"]
	legacy := byName["name:666f6f"]
	unicode := byName["string:feff0066006f006f"]
	if foo.ID == legacy.ID || foo.ID == unicode.ID || foo.ID == "" || foo.Page != 2 || foo.View != "XYZ" || foo.Point == nil {
		t.Fatal("byte/namespace identities collapsed")
	}
	closePoint(t, *foo.Point, 560, 0)
	partial := byName["string:7061727469616c"]
	if partial.Point != nil || partial.Region == nil {
		t.Fatal("nullable x invented")
	}
	closePoint(t, partial.Region[0], 560, 0)
	closePoint(t, partial.Region[1], 560, 400)
	null := byName["string:6e756c6c"]
	if null.Point != nil || null.Region != nil || null.Parameters[0] != nil || null.Parameters[1] != nil || *null.Parameters[2] != 0 || null.FlowBinding != "page_boundary" {
		t.Fatal("nullable viewer settings changed")
	}
	if n.Links[0].Destination == "" || !hasDiagnostic(d, "pdf_destination_missing") || !hasDiagnostic(d, "pdf_destination_flow_inferred") {
		t.Fatal("target evidence/uncertainty missing")
	}
	linked := false
	for _, b := range d.Blocks {
		for _, run := range b.Runs {
			if run.Text == "LINK" {
				linked = run.Destination == foo.ID && strings.Contains(run.Source.Evidence, "link_from_rectangle")
			}
		}
	}
	if !linked {
		t.Fatal("explicit annotation target not associated geometrically")
	}
	c, e := catalogDestinations(d, true)
	if e != nil {
		t.Fatal(e)
	}
	if len(c.anchors) != 14 || len(c.missing) != 1 {
		t.Fatal("view graph omitted", len(c.anchors), len(c.missing))
	}
	assertRecoverySF(t, d)
	sf, e := ToStructured(d)
	if e != nil {
		t.Fatal(e)
	}
	// Test alias isolation using the known XYZ view, including nullable pointers.
	for i := range sf.Attrs.PDFNavigation.Destinations {
		if sf.Attrs.PDFNavigation.Destinations[i].ID == foo.ID {
			*sf.Attrs.PDFNavigation.Destinations[i].Parameters[0] = 999
		}
	}
	if *foo.Parameters[0] != 20 {
		t.Fatal("SF mutation changed caller provenance")
	}
}
func TestPDFNavigationMalformed(t *testing.T) {
	for _, tree := range []string{
		`<< /Kids [8 0 R] >>`,
		`<< /Names [(foo) [6 0 R /Fit] (foo) [6 0 R /Fit]] >>`,
		`<< /Names [(z) [6 0 R /Fit] (a) [6 0 R /Fit]] >>`,
		`<< /Names [(foo) [99 0 R /Fit]] >>`,
		`<< /Names [(foo) [6 0 R /FitR null 1 2 3]] >>`,
		`<< /Names [(foo) [6 0 R /XYZ 20 300 -1]] >>`,
		`<< /Names [(foo) [0 /Fit]] >>`,
		`<< /Names [(foo) [6 0 R /FitH 1 2]] >>`,
	} {
		d, e := ReadPDF(context.Background(), bytes.NewReader(navigationFixture(tree, "", `(foo)`)), ReadOptions{})
		if e == nil || d != nil {
			t.Fatalf("invalid view accepted: %s: %v", tree, e)
		}
	}
}
func TestPDFNavigationNativeRecovery(t *testing.T) {
	var b bytes.Buffer
	if e := Write(context.Background(), &b, destinationDocument(), PDF, WriteOptions{}); e != nil {
		t.Fatal(e)
	}
	d := readRecovery(t, b.Bytes())
	n := d.PDFNavigation
	if n == nil {
		t.Fatal("no physical navigation")
	}
	destinations := map[string]PDFDestination{}
	for _, v := range n.Destinations {
		destinations[v.ID] = v
	}
	if len(destinations) != 6 || destinations["late"].Page != 2 || !destinations["missing"].Unresolved || destinations["missing"].Point != nil {
		t.Fatal("named targets or unresolved identity lost", destinations)
	}
	if destinations["late"].Point == nil || math.Abs(destinations["late"].Point.Y-20) > 1e-6 {
		t.Fatal("target position changed")
	}
	uri := false
	for _, l := range n.Links {
		if l.Kind == "URI" && l.URI == "#literal" && l.Destination == "" {
			uri = true
		}
	}
	if !uri {
		t.Fatal("generic fragment converted into internal destination")
	}
	assertRecoverySF(t, d)
	// Pure navigation provenance must not change on an SF persistence round trip.
	sf, e := ToStructured(d)
	if e != nil {
		t.Fatal(e)
	}
	back, e := FromStructured(sf)
	if e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(n, back.PDFNavigation) {
		t.Fatal("view/source navigation changed")
	}
}
func TestPDFTaggedInternalLinkOwnership(t *testing.T) {
	objects := taggedTestObjects(markedText(0, 20, 300, "click here"), `<< /Type /StructTreeRoot /K 8 0 R >>`, `<< /Type /StructElem /S /P /P 7 0 R /Pg 3 0 R /K 9 0 R >>`, `<< /Type /StructElem /S /Link /P 8 0 R /K [0 << /Type /OBJR /Obj 10 0 R >>] >>`, `<< /Type /Annot /Subtype /Link /Rect [20 310 25 296] /A << /S /GoTo /D [3 0 R /XYZ null 300 null] >> >>`)
	objects[2] = strings.Replace(objects[2], "/Contents 5 0 R", "/Annots [10 0 R] /Contents 5 0 R", 1)
	d := readRecovery(t, recoveryPDF(objects...))
	run := d.Blocks[0].Runs[0]
	if run.Destination == "" || !strings.Contains(run.Source.Evidence, "link_from_structure") || hasDiagnostic(d, "pdf_link_ownership") {
		t.Fatal("logical GoTo ownership not recovered", run, d.Diagnostics)
	}
}

func TestPDFNavigationMetadataAdmission(t *testing.T) {
	d := readRecovery(t, navigationFixture(fullNavigationTree(), "", `(foo)`))
	for _, mutate := range []func(*PDFNavigation){
		func(n *PDFNavigation) { n.Destinations = append(n.Destinations, n.Destinations[0]) },
		func(n *PDFNavigation) { n.Destinations[0].View = "Other" },
		func(n *PDFNavigation) {
			n.Destinations[0].NameKind = n.Destinations[1].NameKind
			n.Destinations[0].NameBytes = append([]byte(nil), n.Destinations[1].NameBytes...)
		},
		func(n *PDFNavigation) { n.Links[0].Destination = "unknown"; n.Links[0].Kind = "GoTo" },
		func(n *PDFNavigation) { n.Links[0].Regions[0][0].X = math.Inf(1) },
	} {
		sf, e := ToStructured(d)
		if e != nil {
			t.Fatal(e)
		}
		mutate(sf.Attrs.PDFNavigation)
		if back, e := FromStructured(sf); e == nil || back != nil {
			t.Fatal("invalid navigation accepted")
		}
	}
}
