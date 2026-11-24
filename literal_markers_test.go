package manydoc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func literalMarkerDocument() *Document {
	return &Document{Blocks: []Block{{Kind: ListBlock, List: "number", Items: []ListItem{
		{Marker: &ListMarker{Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "α)", Bold: true, Link: "https://example.org/alpha"}}}}}, Blocks: []Block{mergeParagraph("FIRST")}},
		{Marker: &ListMarker{Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Anchor: "marker-point"}, {Text: "β)", Italic: true}, {Note: &NoteReference{Kind: Footnote, ID: "label-note"}}}}}}, Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "SECOND", Destination: "marker-point"}}}}},
		{Marker: &ListMarker{Blocks: []Block{{Kind: Paragraph}}}, Blocks: []Block{mergeParagraph("EMPTY")}},
	}}}, Notes: []Note{{Kind: Footnote, ID: "label-note", Blocks: []Block{mergeParagraph("Marker note")}}}}
}

func TestLiteralMarkerSFJSONGzipAndIdentityGraphs(t *testing.T) {
	ctx := context.Background()
	d := literalMarkerDocument()
	before := canonicalSF(t, d)
	for _, format := range []Format{SF, SFGzip, JSON} {
		var out bytes.Buffer
		if err := Write(ctx, &out, d, format, WriteOptions{}); err != nil {
			t.Fatal(format, err)
		}
		if dir := os.Getenv("MANYDOC_LITERAL_MARKER_QA"); dir != "" && format == SF {
			if err := os.MkdirAll(dir, 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "literal-markers.sf"), out.Bytes(), 0600); err != nil {
				t.Fatal(err)
			}
		}
		back, err := Read(ctx, &out, format, ReadOptions{})
		if err != nil {
			t.Fatal(format, err)
		}
		if !bytes.Equal(before, canonicalSF(t, back)) {
			t.Fatal(format, "literal marker graph changed")
		}
	}
	if !bytes.Equal(before, canonicalSF(t, d)) {
		t.Fatal("caller mutated")
	}
	// Marker references must participate in the same identity validation as bodies.
	d.Notes = nil
	if err := d.Validate(); err == nil {
		t.Fatal("dangling marker note accepted")
	}
	d = literalMarkerDocument()
	d.Headers = d.Blocks
	d.Blocks = nil
	if err := d.Validate(); err == nil {
		t.Fatal("marker note in running story accepted")
	}
	d = literalMarkerDocument()
	d.Notes[0].Blocks = d.Blocks
	if err := d.Validate(); err == nil {
		t.Fatal("marker note in note body accepted")
	}

	d = literalMarkerDocument()
	d.Blocks[0].Items[0].Marker.Blocks[0].Runs = append(d.Blocks[0].Items[0].Marker.Blocks[0].Runs, Run{Anchor: "marker-point"})
	if err := d.Validate(); err == nil {
		t.Fatal("duplicate marker/body anchor accepted")
	}
}

func TestLiteralMarkerGenerationRejectsBeforeOutput(t *testing.T) {
	// This is an explicit unfinished renderer capability, not permission to drop
	// new marker content or narrow the all-four-path completion criteria.
	for _, format := range []Format{VDOCS} {
		var out bytes.Buffer
		err := Write(context.Background(), &out, literalMarkerDocument(), format, WriteOptions{AllowLossy: true})
		if !errors.Is(err, ErrUnsupported) || out.Len() != 0 {
			t.Fatal(format, "marker silently discarded or partial output", err)
		}
	}
}

func TestLiteralMarkerAdmission(t *testing.T) {
	for _, marker := range [][]Block{{}, {{Kind: PageBreak}}, {{Kind: Paragraph, PageBreakBefore: true}}, {{Kind: AnchorPoint, Anchors: []string{"a"}}}} {
		d := literalMarkerDocument()
		d.Blocks[0].Items[0].Marker = &ListMarker{Blocks: marker}
		if err := d.Validate(); err == nil {
			t.Fatal("invalid literal marker accepted", marker)
		}
	}
	d := literalMarkerDocument()
	source := &SourceInfo{Format: PDF, Structure: "explicit", Role: "Lbl", Element: "marker", Confidence: 1, Evidence: "test_marker"}
	d.Blocks[0].Items[0].Marker.Source = source
	if !d.hasPDFSource() {
		t.Fatal("marker source not admitted to fidelity gate")
	}
	sf, err := ToStructured(d)
	if err != nil {
		t.Fatal(err)
	}
	back, err := FromStructured(sf)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(back.Blocks[0].Items[0].Marker.Source, source) {
		t.Fatal("marker source changed")
	}
}

func TestPDFVariableLiteralLabelsKeepExplicitHierarchy(t *testing.T) {
	for _, family := range []string{"None", "UpperAlpha"} {
		t.Run(family, func(t *testing.T) {
			raw := taggedTestPDF(markedText(0, 20, 300, "[done]")+markedText(1, 80, 300, "FIRST")+markedText(2, 20, 280, "[later]")+markedText(3, 80, 280, "SECOND"),
				"<< /Type /StructTreeRoot /K 8 0 R >>",
				"<< /Type /StructElem /S /L /P 7 0 R /Pg 3 0 R /A << /O /List /ListNumbering /"+family+" >> /K [9 0 R 10 0 R] >>",
				"<< /Type /StructElem /S /LI /P 8 0 R /K [11 0 R 12 0 R] >>",
				"<< /Type /StructElem /S /LI /P 8 0 R /K [13 0 R 14 0 R] >>",
				"<< /Type /StructElem /S /Lbl /P 9 0 R /K 0 >>",
				"<< /Type /StructElem /S /LBody /P 9 0 R /K 1 >>",
				"<< /Type /StructElem /S /Lbl /P 10 0 R /K 2 >>",
				"<< /Type /StructElem /S /LBody /P 10 0 R /K 3 >>")
			d := readRecovery(t, raw)
			if len(d.Blocks) != 1 || d.Blocks[0].Kind != ListBlock || len(d.Blocks[0].Items) != 2 || d.Blocks[0].Source.Role != "L" || !hasDiagnostic(d, "pdf_list_literal") {
				t.Fatal("explicit list discarded")
			}
			for i, item := range d.Blocks[0].Items {
				if item.Marker == nil || item.Marker.Source == nil || item.Marker.Source.Role != "Lbl" || item.Marker.Source.ParentElement != item.Source.Element || item.Source.ParentElement != d.Blocks[0].Source.Element {
					t.Fatal("literal label/item/list source ownership lost")
				}
				if paragraphText(item.Marker.Blocks[0]) != []string{"[done]", "[later]"}[i] || paragraphText(item.Blocks[0]) != []string{"FIRST", "SECOND"}[i] {
					t.Fatal("literal label/body correspondence changed")
				}
			}
			if family == "None" && (d.Blocks[0].List != "bullet" || !hasDiagnostic(d, "pdf_list_kind_unobserved")) {
				t.Fatal("unproven automatic counter invented")
			}
			if family == "UpperAlpha" && (d.Blocks[0].List != "number" || !strings.Contains(d.Blocks[0].Source.Evidence, "start_unobserved")) {
				t.Fatal("explicit ordered family or counter uncertainty lost")
			}
			assertRecoverySF(t, d)
			exportLiteralMarkerQA(t, "variable-"+family, raw, d)
		})
	}
}

func TestLiteralMarkerSchemaAdmission(t *testing.T) {
	sf, err := ToStructured(literalMarkerDocument())
	if err != nil {
		t.Fatal(err)
	}
	label := sf.Content[0].Content[0].Content[0]
	if label.Type != "list_marker" {
		t.Fatal("literal marker not encoded as its own node")
	}
	for _, mutate := range []func(*StructuredDocument){
		func(d *StructuredDocument) { d.Content = append(d.Content, label) },
		func(d *StructuredDocument) {
			d.Content[0].Content[0].Content = append(d.Content[0].Content[0].Content, label)
		},
		func(d *StructuredDocument) { d.Content[0].Content[0].Content[0].Content = nil },
		func(d *StructuredDocument) { d.Content[0].Content[0].Content[0].Text = "hidden" },
	} {
		// JSON gives each negative case independent ownership of its node slices.
		raw, _ := json.Marshal(sf)
		var candidate StructuredDocument
		if err = json.Unmarshal(raw, &candidate); err != nil {
			t.Fatal(err)
		}
		mutate(&candidate)
		if _, err = FromStructured(&candidate); err == nil {
			t.Fatal("invalid marker placement/payload accepted")
		}
	}
}

func exportLiteralMarkerQA(t *testing.T, name string, raw []byte, d *Document) {
	t.Helper()
	dir := os.Getenv("MANYDOC_LITERAL_MARKER_QA")
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	tree, err := ToStructured(d)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err = WriteStructured(context.Background(), &out, tree); err != nil {
		t.Fatal(err)
	}
	for ext, data := range map[string][]byte{"pdf": raw, "sf": out.Bytes()} {
		if err = os.WriteFile(filepath.Join(dir, name+"."+ext), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func FuzzLiteralMarkerSF(f *testing.F) {
	f.Add("α)", "First 😀")
	f.Add("", "")
	f.Add("[done]\ncontinued", "Second")
	f.Fuzz(func(t *testing.T, label, body string) {
		if len(label) > 2048 || len(body) > 2048 {
			t.Skip()
		}
		d := &Document{Blocks: []Block{{Kind: ListBlock, List: "bullet", Items: []ListItem{{Marker: &ListMarker{Blocks: []Block{{Kind: Paragraph, Anchors: []string{"marker"}, Runs: []Run{{Text: label, Bold: true, Link: "https://example.org/marker"}}}}}, Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: body, Destination: "marker"}}}}}}}}}
		if err := d.Validate(); err != nil {
			return
		}
		before := canonicalSF(t, d)
		var out bytes.Buffer
		if err := Write(context.Background(), &out, d, SF, WriteOptions{}); err != nil {
			t.Fatal(err)
		}
		got, err := Read(context.Background(), &out, SF, ReadOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(before, canonicalSF(t, got)) {
			t.Fatal("literal marker text/style/anchor/URI graph changed")
		}
	})
}
