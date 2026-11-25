package manifold

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
)

func colleafTableExample() []Block {
	p := func(s string) Block { return Block{Kind: Paragraph, Runs: []Run{{Text: s}}} }
	nested := Block{Kind: Table, ColumnWidths: []float64{72}, Rows: []Row{{Cells: []Cell{{Blocks: []Block{p("Inner 😀")}}}}}}
	return []Block{p("Before"), {Kind: Table, ColumnWidths: []float64{72, 108, 144}, Rows: []Row{
		{Cells: []Cell{{RowSpan: 2, ColSpan: 2, Blocks: []Block{p("Merge 😀"), nested}}, {Blocks: []Block{p("B")}}}},
		{Cells: []Cell{{Blocks: []Block{p("C")}}}},
		{Cells: []Cell{{ColSpan: 3, Blocks: []Block{p("Tail")}}}},
	}}, p("After"), {Kind: Table, Rows: []Row{
		{RepeatHeader: true, Cells: []Cell{{Blocks: []Block{p("Header")}}}},
		{Cells: []Cell{{Blocks: []Block{p("Body")}}}},
	}}}
}
func TestColleafMergedNestedTableSFRoundTrip(t *testing.T) {
	ctx := context.Background()
	blocks := colleafTableExample()
	body, tables, err := encodeColleafBody(ctx, blocks)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := decodeColleafBody(ctx, body, colleafTextContext{}, tables)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(blocks, doc.Blocks) {
		a, _ := json.Marshal(blocks)
		b, _ := json.Marshal(doc.Blocks)
		t.Fatalf("table semantics differ\nwant%s\ngot%s", a, b)
	}
	if len(doc.Diagnostics) != 0 {
		t.Fatal(doc.Diagnostics)
	}
	if !reflect.DeepEqual(doc.Blocks, blocks) {
		t.Fatalf("native table changed SF content/spans/styles\ngot%+v\nwant%+v", doc.Blocks, blocks)
	}
	if len(doc.Diagnostics) > 0 {
		t.Fatal("supported native tables emitted losses", doc.Diagnostics)
	}
	sf, err := ToStructured(doc)
	if err != nil {
		t.Fatal(err)
	}
	throughSF, err := FromStructured(sf)
	if err != nil {
		t.Fatal(err)
	}
	body2, tables2, err := encodeColleafBody(ctx, throughSF.Blocks)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(body, body2) || !reflect.DeepEqual(tables, tables2) {
		t.Fatal("canonical SF changed native cells/relations/styles")
	}
	if path := os.Getenv("MANYDOC_COLLEAF_TABLE_QA"); path != "" {
		data, err := json.MarshalIndent(map[string]any{"id": "manydoc-table-qa", "body": body, "tableSource": tables, "documentStyle": map[string]any{"documentFlavor": 1, "pageSize": map[string]int{"width": 800, "height": 1000}, "marginLeft": 40, "marginRight": 40}}, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if path := os.Getenv("MANYDOC_COLLEAF_TABLE_EDIT"); path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var saved struct {
			Body   colleafBody                `json:"body"`
			Tables map[string]json.RawMessage `json:"tableSource"`
		}
		if err = json.Unmarshal(data, &saved); err != nil {
			t.Fatal(err)
		}
		edited, err := decodeColleafBody(ctx, saved.Body, colleafTextContext{}, saved.Tables)
		if err != nil {
			t.Fatal(err)
		}
		got := edited.Blocks[1].Rows[1].Cells[0].Blocks[0].Runs[0].Text
		if got != "Z" {
			t.Fatal("native cell edit did not win", got)
		}
		if edited.Blocks[1].Rows[0].Cells[0].RowSpan != 2 || edited.Blocks[1].Rows[0].Cells[0].ColSpan != 2 {
			t.Fatal("native save changed merge ownership")
		}
	}
}

// Independently authored native row/cell records, not produced by the exporter.
func nativeTableFixture(values [][]string, spans [][][2]int) (colleafBody, map[string]json.RawMessage) {
	stream := "\x1a"
	rows := []any{}
	for ri, row := range values {
		stream += "\x1b"
		cells := []any{}
		for ci, text := range row {
			stream += "\x1c" + text + "\r\n\x1d"
			cells = append(cells, map[string]int{"rowSpan": spans[ri][ci][0], "columnSpan": spans[ri][ci][1]})
		}
		stream += "\x0e"
		rows = append(rows, map[string]any{"tableCells": cells})
	}
	stream += "\x0f"
	end := colleafUTF16Len(stream)
	stream += "\r\n"
	width := func() any { return map[string]any{"size": colleafTableWidth{1, colleafNumberUnit{96, 3}}} }
	return colleafBody{DataStream: stream, Tables: []colleafTableRecord{{StartIndex: 0, EndIndex: end, ID: "native"}}}, map[string]json.RawMessage{"native": sfAttrs(map[string]any{"tableId": "native", "tableRows": rows, "tableColumns": []any{width(), width(), width()}})}
}
func TestColleafNativeMergeOriginsContinuationsAndContent(t *testing.T) {
	values := [][]string{{"A", "", "B"}, {"", "", "C"}}
	spans := [][][2]int{{{2, 2}, {0, 0}, {1, 1}}, {{0, 2}, {0, 0}, {1, 1}}}
	body, tables := nativeTableFixture(values, spans)
	doc, err := decodeColleafBody(context.Background(), body, colleafTextContext{}, tables)
	if err != nil {
		t.Fatal(err)
	}
	table := doc.Blocks[0]
	if len(table.Rows) != 2 || len(table.Rows[0].Cells) != 2 || len(table.Rows[1].Cells) != 1 || table.Rows[0].Cells[0].ColSpan != 2 || table.Rows[0].Cells[0].RowSpan != 2 || table.Rows[1].Cells[0].Blocks[0].Runs[0].Text != "C" {
		t.Fatal(table)
	}
	// Covered native cells must never hide visible text or nested structures.
	values[1][0] = "Do not lose me"
	body, tables = nativeTableFixture(values, spans)
	if _, err = decodeColleafBody(context.Background(), body, colleafTextContext{}, tables); err == nil || !strings.Contains(err.Error(), "contains content") {
		t.Fatal("covered content discarded", err)
	}
}
func TestColleafTableRejectsContradictorySources(t *testing.T) {
	cases := []struct {
		name   string
		values [][]string
		spans  [][][2]int
	}{
		{"overlap", [][]string{{"A", "B"}, {"C", "D"}}, [][][2]int{{{2, 2}, {1, 1}}, {{1, 2}, {1, 1}}}},
		{"unowned vertical", [][]string{{"A", "B"}}, [][][2]int{{{0, 2}, {1, 1}}}},
		{"unowned horizontal", [][]string{{"A", "B"}}, [][][2]int{{{0, 0}, {1, 3}}}},
		{"out of bounds", [][]string{{"A"}}, [][][2]int{{{2, 3}}}},
		{"incomplete grid", [][]string{{"A"}}, [][][2]int{{{1, 2}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body, tables := nativeTableFixture(tc.values, tc.spans)
			if _, err := decodeColleafBody(context.Background(), body, colleafTextContext{}, tables); err == nil {
				t.Fatal("invalid span accepted")
			}
		})
	}
	body, tables := nativeTableFixture([][]string{{"A"}}, [][][2]int{{{1, 3}}})
	if _, err := decodeColleafBody(context.Background(), body, colleafTextContext{}, nil); err == nil {
		t.Fatal("missing source accepted")
	}
	tables["orphan"] = tables["native"]
	if _, err := decodeColleafBody(context.Background(), body, colleafTextContext{}, tables); err == nil {
		t.Fatal("orphan source accepted")
	}
}

func TestColleafNativeTableLegacyGeometryUnits(t *testing.T) {
	body, tables := nativeTableFixture([][]string{{"A"}}, [][][2]int{{{1, 3}}})
	var source map[string]json.RawMessage
	if err := json.Unmarshal(tables["native"], &source); err != nil {
		t.Fatal(err)
	}
	source["tableColumns"] = sfAttrs([]any{
		map[string]any{"size": colleafTableWidth{1, colleafNumberUnit{96, 0}}},
		map[string]any{"size": colleafTableWidth{1, colleafNumberUnit{96, 3}}},
		map[string]any{"size": colleafTableWidth{1, colleafNumberUnit{96, 0}}},
	})
	source["size"] = sfAttrs(colleafTableWidth{1, colleafNumberUnit{288, 0}})
	tables["native"] = sfAttrs(source)
	doc, err := decodeColleafBody(context.Background(), body, colleafTextContext{}, tables)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(doc.Blocks[0].ColumnWidths, []float64{72, 72, 72}) || len(doc.Diagnostics) > 0 {
		t.Fatal(doc.Blocks, doc.Diagnostics)
	}
}

func TestColleafMixedTableParagraphKeepsTextOrderAndStyles(t *testing.T) {
	body, tables := nativeTableFixture([][]string{{"Cell"}}, [][][2]int{{{1, 3}}})
	end := body.Tables[0].EndIndex
	body.DataStream = "Prefix" + strings.TrimSuffix(body.DataStream, "\r\n") + "Suffix\r\n"
	body.Tables[0].StartIndex += 6
	body.Tables[0].EndIndex += 6
	body.Paragraphs = []colleafParagraphRecord{{StartIndex: 6 + end + 6, Style: sfAttrs(map[string]int{"direction": 2})}}
	body.TextRuns = []colleafTextRun{{St: 0, Ed: 6, TS: sfAttrs(map[string]int{"bl": 1})}}
	doc, err := decodeColleafBody(context.Background(), body, colleafTextContext{}, tables)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Blocks) != 3 || doc.Blocks[0].Runs[0].Text != "Prefix" || !doc.Blocks[0].Runs[0].Bold || doc.Blocks[1].Kind != Table || doc.Blocks[2].Runs[0].Text != "Suffix" || doc.Blocks[2].Direction != "rtl" {
		t.Fatal(doc.Blocks)
	}
	if len(doc.Diagnostics) != 1 || doc.Diagnostics[0].Code != "colleaf_table_paragraph" {
		t.Fatal(doc.Diagnostics)
	}
}

func FuzzColleafTables(f *testing.F) {
	type wire struct {
		Body   colleafBody                `json:"body"`
		Tables map[string]json.RawMessage `json:"tableSource"`
	}
	body, tables, err := encodeColleafBody(context.Background(), colleafTableExample())
	if err != nil {
		f.Fatal(err)
	}
	seed, err := json.Marshal(wire{body, tables})
	if err != nil {
		f.Fatal(err)
	}
	f.Add(seed)
	body, tables = nativeTableFixture([][]string{{"A", "B"}, {"", "C"}}, [][][2]int{{{2, 2}, {1, 1}}, {{0, 2}, {1, 1}}})
	seed, err = json.Marshal(wire{body, tables})
	if err != nil {
		f.Fatal(err)
	}
	f.Add(seed)
	f.Fuzz(func(t *testing.T, data []byte) {
		var input wire
		if json.Unmarshal(data, &input) != nil {
			return
		}
		doc, err := decodeColleafBody(context.Background(), input.Body, colleafTextContext{}, input.Tables)
		if err == nil {
			if len(doc.Blocks) == 0 {
				t.Fatal("accepted empty native document")
			}
			// A successful import must retain valid table ownership and dimensions.
			var walk func([]Block)
			walk = func(blocks []Block) {
				for _, b := range blocks {
					if b.Kind == Table {
						grid, err := tableGrid(b.Rows)
						if err != nil {
							t.Fatal(err)
						}
						if err = validateColumnWidths(b.ColumnWidths, grid.Columns); err != nil {
							t.Fatal(err)
						}
						for _, row := range b.Rows {
							for _, cell := range row.Cells {
								walk(cell.Blocks)
							}
						}
					}
				}
			}
			walk(doc.Blocks)
		}
	})
}
