package manydoc

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// Parent ActualText is outside the Link element. Child link ownership must
// survive replacement; re-applying the child's Link role later cannot fix it.
func TestPDFActualTextRetainsInternalNavigation(t *testing.T) {
	for _, content := range []string{markedText(0, 20, 300, "XY"), markedText(0, 20, 300, "X") + markedText(1, 26, 300, "Y")} {
		objects := taggedTestObjects(content,
			`<< /Type /StructTreeRoot /K 8 0 R >>`,
			`<< /Type /StructElem /S /P /P 7 0 R /Pg 3 0 R /ActualText (replacement) /K 9 0 R >>`,
			`<< /Type /StructElem /S /Link /P 8 0 R /K [0 << /Type /OBJR /Obj 10 0 R >>] >>`,
			`<< /Type /Annot /Subtype /Link /Rect [15 290 65 315] /A << /S /GoTo /D [3 0 R /XYZ 20 300 null] >> >>`)
		if strings.Count(content, "MCID") > 1 {
			objects[8] = strings.Replace(objects[8], "/K [0 ", "/K [0 1 ", 1)
		}
		objects[2] = strings.Replace(objects[2], "/Contents 5 0 R", "/Annots [10 0 R] /Contents 5 0 R", 1)
		d := readRecovery(t, recoveryPDF(objects...))
		var replacement *Run
		for i := range d.Blocks {
			for j := range d.Blocks[i].Runs {
				if d.Blocks[i].Runs[j].Text == "replacement" {
					replacement = &d.Blocks[i].Runs[j]
				}
			}
		}
		if replacement == nil || replacement.Destination == "" || replacement.Link == "" {
			t.Fatal("ActualText lost internal target", replacement, d.Diagnostics)
		}
		if d.PDFNavigation == nil || len(d.PDFNavigation.Links) != 1 || replacement.Destination != d.PDFNavigation.Links[0].Destination {
			t.Fatal("replacement target contradicts source annotation")
		}
		assertRecoverySF(t, d)
		// Source physical fields still require explicit retypesetting for output.
		// Exercise SF/DOCX while retaining the target's typed identity.
		replacement.FontFamily = "Go"
		replacement.FontSize = 10
		var word bytes.Buffer
		if e := Write(context.Background(), &word, d, DOCX, WriteOptions{AllowLossy: true}); e != nil {
			t.Fatal(e)
		}
		back, e := ReadDOCX(context.Background(), &word)
		if e != nil {
			t.Fatal(e)
		}
		found := false
		_ = documentDestinationRuns(back, func(r Run) error {
			if r.Text == "replacement" && r.Destination == replacement.Destination {
				found = true
			}
			return nil
		})
		if !found {
			t.Fatal("internal navigation changed to URI during DOCX output")
		}
	}
}
func TestPDFActualTextLinkIntersection(t *testing.T) {
	for _, pair := range []struct {
		a, b     Run
		mixed    bool
		id, href string
	}{
		{Run{Link: "#target", Destination: "target"}, Run{Link: "#target", Destination: "target"}, false, "target", "#target"},
		{Run{Link: "#%74arget", Destination: "target"}, Run{Link: "#target", Destination: "target"}, false, "target", "#%74arget"},
		{Run{Link: "#target", Destination: "target"}, Run{Link: "#target"}, true, "", ""},
		{Run{Link: "#left", Destination: "left"}, Run{Link: "#right", Destination: "right"}, true, "", ""},
		{Run{Link: "https://example.invalid"}, Run{Link: "https://example.invalid"}, false, "", "https://example.invalid"},
	} {
		got, mixed := commonTextStyle(pair.a, pair.b)
		if mixed != pair.mixed || got.Destination != pair.id || got.Link != pair.href {
			t.Fatal("incorrect common target", pair, got, mixed)
		}
	}
}
