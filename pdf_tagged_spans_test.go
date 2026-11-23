package manydoc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strings"
	"testing"
)

func taggedSpanPDF() []byte {
	path := "20 220 240 120 re S 20 300 m 260 300 l S 100 260 m 260 260 l S 100 220 m 100 300 l S 180 220 m 180 340 l S\n"
	return recoveryPDF(taggedSpanObjects(path)...)
}
func taggedSpanObjects(path string) []string {
	content := path + markedText(0, 30, 320, "TOP") + markedText(1, 190, 320, "RIGHT") + markedText(2, 30, 280, "VERT") + markedText(3, 110, 280, "MID") + markedText(4, 190, 280, "R1") + markedText(5, 110, 240, "LOW") + markedText(6, 190, 240, "R2")
	return taggedTestObjects(content, "<< /Type /StructTreeRoot /K 8 0 R >>",
		"<< /Type /StructElem /S /Table /P 7 0 R /Pg 3 0 R /K [9 0 R 10 0 R 11 0 R] >>",
		"<< /Type /StructElem /S /TR /P 8 0 R /K [12 0 R 13 0 R] >>",
		"<< /Type /StructElem /S /TR /P 8 0 R /K [14 0 R 15 0 R 16 0 R] >>",
		"<< /Type /StructElem /S /TR /P 8 0 R /K [17 0 R 18 0 R 19 0 R] >>",
		"<< /Type /StructElem /S /TH /P 9 0 R /K 0 >>",
		"<< /Type /StructElem /S /TH /P 9 0 R /K 1 >>",
		"<< /Type /StructElem /S /TD /P 10 0 R /K 2 >>",
		"<< /Type /StructElem /S /TD /P 10 0 R /K 3 >>",
		"<< /Type /StructElem /S /TD /P 10 0 R /K 4 >>",
		"<< /Type /StructElem /S /TD /P 11 0 R /K [] >>",
		"<< /Type /StructElem /S /TD /P 11 0 R /K 5 >>",
		"<< /Type /StructElem /S /TD /P 11 0 R /K 6 >>")
}

func TestRulingJoinIncludesPaintedCrossingStroke(t *testing.T) {
	lines := []pdfRuling{
		{horizontal: true, pos: 10, start: .258, end: 99.742, width: .48},
		{pos: 0, start: 0, end: 20, width: .504},
		{pos: 100, start: 0, end: 20, width: .504},
	}
	if got := rulingCoverage(lines, true, 10, 0, 100); got != 1 {
		t.Fatal("painted crossing plus coordinate tolerance did not join", got)
	}
	lines[0].start = .503
	if got := rulingCoverage(lines, true, 10, 0, 100); got != -1 {
		t.Fatal("gap beyond stroke and tolerance was bridged", got)
	}
}

func TestPDFTaggedSpanSourceValidation(t *testing.T) {
	for name, mutate := range map[string]func(*SourceInfo){
		"inferred membership": func(s *SourceInfo) { s.Structure = "inferred" },
		"unknown span":        func(s *SourceInfo) { s.TableSpans.ColSpan = "unknown" },
		"nonfinite":           func(s *SourceInfo) { s.TableSpans.Bounds[0].X = math.NaN() },
		"empty bounds":        func(s *SourceInfo) { s.TableSpans.Bounds = Quad{} },
		"confidence":          func(s *SourceInfo) { s.TableSpans.Confidence = 1.1 },
		"reference row":       func(s *SourceInfo) { s.TableSpans.Continuations[0].Row = -1 },
		"duplicate origin":    func(s *SourceInfo) { s.TableSpans.Continuations[0].Element = s.Element },
	} {
		t.Run(name, func(t *testing.T) {
			d := readRecovery(t, taggedSpanPDF())
			mutate(d.Blocks[0].Rows[1].Cells[0].Source)
			if err := d.Validate(); err == nil {
				t.Fatal("invalid provenance accepted")
			}
		})
	}
}

func TestPDFTaggedSpanRequiresEvidenceAndHonorsExplicitAttributes(t *testing.T) {
	path := "20 220 240 120 re S 20 300 m 260 300 l S 100 260 m 260 260 l S 100 220 m 100 300 l S 180 220 m 180 340 l S\n"
	for _, name := range []string{"no borders", "explicit unit colspan", "visible continuation", "continuation last"} {
		t.Run(name, func(t *testing.T) {
			objects := taggedSpanObjects(path)
			switch name {
			case "no borders":
				objects = taggedSpanObjects("")
			case "explicit unit colspan":
				objects[11] = strings.Replace(objects[11], "/K 0", "/K 0 /A << /O /Table /ColSpan 1 >>", 1)
			case "visible continuation":
				objects[4] = recoveryStream("", path+markedText(0, 30, 320, "TOP")+markedText(1, 190, 320, "RIGHT")+markedText(2, 30, 280, "VERT")+markedText(3, 110, 280, "MID")+markedText(4, 190, 280, "R1")+markedText(5, 110, 240, "LOW")+markedText(6, 190, 240, "R2")+markedText(7, 30, 240, "VISIBLE"))
				objects[16] = strings.Replace(objects[16], "/K []", "/K 7", 1)
			case "continuation last":
				objects[10] = strings.Replace(objects[10], "17 0 R 18 0 R 19 0 R", "18 0 R 19 0 R 17 0 R", 1)
			}
			d := readRecovery(t, recoveryPDF(objects...))
			if name == "continuation last" {
				if d.Blocks[0].Rows[1].Cells[0].RowSpan != 2 || len(d.Blocks[0].Rows[2].Cells) != 2 {
					t.Fatal("forced continuation interleaving not recovered")
				}
			} else {
				if !hasDiagnostic(d, "pdf_structure_table_grid_uncertain") || hasDiagnostic(d, "pdf_structure_spans_inferred") || d.Blocks[0].Rows[0].Cells[0].ColSpan != 1 || len(d.Blocks[0].Rows[2].Cells) != 3 {
					t.Fatal("unproven spans or cell loss")
				}
				if name == "visible continuation" && strings.Count(d.Text(), "VISIBLE") != 1 {
					t.Fatal("visible cell discarded")
				}
			}
			assertRecoverySF(t, d)
		})
	}
}

func TestRealPDFMQTTTaggedTableMembership(t *testing.T) {
	raw, err := os.ReadFile("testdata/real/mqtt-v5.0-os.pdf")
	if err != nil {
		t.Fatal(err)
	}
	oracleRaw, err := os.ReadFile("testdata/real/mqtt-v5.0-pdf-oracle.json")
	if err != nil {
		t.Fatal(err)
	}
	var oracle struct {
		Tables []struct {
			ID    int
			Rows  int
			Cells [][]struct {
				ID   int
				Role string
			}
		}
	}
	if err := json.Unmarshal(oracleRaw, &oracle); err != nil {
		t.Fatal(err)
	}
	d, err := ReadPDF(context.Background(), bytes.NewReader(raw), ReadOptions{PDFSkipInference: true})
	if err != nil {
		t.Fatal(err)
	}
	tables := map[string]Block{}
	var walk func([]Block)
	walk = func(bs []Block) {
		for _, b := range bs {
			if b.Kind == Table && b.Source != nil {
				tables[b.Source.Element] = b
			}
			for _, r := range b.Rows {
				for _, c := range r.Cells {
					walk(c.Blocks)
				}
			}
			for _, item := range b.Items {
				walk(item.Blocks)
			}
		}
	}
	walk(d.Blocks)
	if len(d.Pages) != 137 || len(tables) != len(oracle.Tables) {
		t.Fatal("page or table membership lost", len(d.Pages), len(tables))
	}
	origins, continuations, inferred, uncertain := 0, 0, 0, 0
	for _, want := range oracle.Tables {
		b, ok := tables[fmt.Sprintf("(%d 0 R)", want.ID)]
		if !ok || b.Source.Structure != "explicit" || len(b.Rows) != want.Rows {
			t.Fatal("explicit table rows lost", want.ID)
		}
		got := map[string]string{}
		inferredTable := false
		for ri, row := range b.Rows {
			for _, c := range row.Cells {
				if c.Source == nil || c.Source.Structure != "explicit" {
					t.Fatal("explicit cell lost", want.ID)
				}
				key := fmt.Sprintf("%d/%s", ri, c.Source.Element)
				if _, exists := got[key]; exists {
					t.Fatal("duplicate origin", key)
				}
				got[key] = "/" + c.Source.Role
				origins++
				if spans := c.Source.TableSpans; spans != nil {
					inferredTable = true
					for _, ref := range spans.Continuations {
						key := fmt.Sprintf("%d/%s", ref.Row, ref.Element)
						if _, exists := got[key]; exists {
							t.Fatal("duplicate continuation", key)
						}
						got[key] = "/" + ref.Role
						continuations++
					}
				}
			}
		}
		if inferredTable {
			inferred++
		}
		if b.Source.Evidence == "pdf_structure_tree_ragged_grid" {
			uncertain++
		}
		count := 0
		for ri, row := range want.Cells {
			for _, c := range row {
				key := fmt.Sprintf("%d/(%d 0 R)", ri, c.ID)
				if got[key] != c.Role {
					t.Fatal("cell identity/row/role lost", want.ID, key, got[key], c.Role)
				}
				count++
			}
		}
		if len(got) != count {
			t.Fatal("unexpected cell membership", want.ID, len(got), count)
		}
		if want.ID == 4887 {
			for _, row := range b.Rows[1:] {
				if len(row.Cells) != 2 || row.Cells[1].ColSpan != 8 {
					t.Fatal("independently proven page-17 merge lost")
				}
			}
		}
	}
	if origins+continuations != 3020 || continuations != 8 || uncertain != 1 {
		t.Fatal("corpus cell accounting/uncertainty changed", origins, continuations, uncertain)
	}
	t.Logf("tables=%d inferred=%d uncertain=%d origins=%d continuations=%d", len(tables), inferred, uncertain, origins, continuations)
}
func TestPDFTaggedMissingSpansUsePaintedGrid(t *testing.T) {
	d := readRecovery(t, taggedSpanPDF())
	if len(d.Blocks) != 1 || d.Blocks[0].Kind != Table {
		t.Fatal("explicit table lost", d.Text())
	}
	b := d.Blocks[0]
	g, err := tableGrid(b.Rows)
	if err != nil || g.Columns != 3 || len(g.Origins) != 7 || len(b.Rows[2].Cells) != 2 {
		t.Fatal("painted merged grid not recovered", g, err)
	}
	top, vert := b.Rows[0].Cells[0], b.Rows[1].Cells[0]
	if top.ColSpan != 2 || vert.RowSpan != 2 {
		t.Fatal("missing merged spans", top.ColSpan, vert.RowSpan)
	}
	if top.Source.Structure != "explicit" || top.Source.TableSpans == nil || top.Source.TableSpans.ColSpan != "inferred" || top.Source.TableSpans.Confidence >= 1 {
		t.Fatal("membership and inference conflated", top.Source)
	}
	refs := vert.Source.TableSpans.Continuations
	if len(refs) != 1 || refs[0].Element != "(17 0 R)" || refs[0].Role != "TD" || refs[0].Page != 1 {
		t.Fatal("empty explicit continuation identity lost", refs)
	}
	if !hasDiagnostic(d, "pdf_structure_spans_inferred") {
		t.Fatal("span inference not disclosed")
	}
	for _, word := range []string{"TOP", "RIGHT", "VERT", "MID", "R1", "LOW", "R2"} {
		if strings.Count(d.Text(), word) != 1 {
			t.Fatal("text loss or duplication", word, d.Text())
		}
	}
	assertRecoverySF(t, d)
}

func TestRulingJoinAllowsOperandQuantizationOnly(t *testing.T) {
	lines := []pdfRuling{{horizontal: true, pos: 10, start: .250005, end: 99.749995, width: .48}}
	if got := rulingCoverage(lines, true, 10, 0, 100); got != 1 {
		t.Fatal("rounded boundary join was treated as missing paint", got)
	}
	lines[0].start = .251
	if got := rulingCoverage(lines, true, 10, 0, 100); got != -1 {
		t.Fatal("genuine gap was hidden by precision guard", got)
	}
}
