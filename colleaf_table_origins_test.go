package manydoc

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func colleafTableOriginExample(t *testing.T) *Document {
	base := colleafOriginExample(t)
	src := func(id, role string) *SourceInfo {
		s := cloneSourceInfo(base.Blocks[0].Source)
		s.Element = id
		s.Role = role
		return s
	}
	p := func(text string) Block { return Block{Kind: Paragraph, Runs: []Run{{Text: text}}} }
	cell := func(id, text string) Cell { return Cell{Source: src(id, "TD"), Blocks: []Block{p(text)}} }
	table := func(id, text string) Block {
		return Block{Kind: Table, Source: src(id, "Table"), Rows: []Row{{Cells: []Cell{cell(id+"-cell", text)}}}}
	}
	nested := table("nested", "Nested")
	wide := cell("wide", "Wide")
	wide.ColSpan = 2
	wide.Source.TableSpans = &TableSpanSource{ColSpan: "explicit", RowSpan: "explicit", Evidence: "painted_grid_adjacency", Confidence: .8, Bounds: *wide.Source.Quad}
	tall := cell("tall", "Tall")
	tall.RowSpan = 2
	tall.Blocks = append(tall.Blocks, nested)
	outer := Block{Kind: Table, Source: src("outer", "Table"), Rows: []Row{{RepeatHeader: true, Cells: []Cell{wide}}, {Cells: []Cell{tall, cell("right", "Right")}}, {Cells: []Cell{cell("bottom", "Bottom")}}}}
	outer.Source.TableWidths = &TableWidthSource{Evidence: "painted_grid_adjacency", Confidence: .75, Bounds: *outer.Source.Quad}
	return &Document{Pages: base.Pages, Metadata: Metadata{Title: "Table origins"}, Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "😀 Source tables", Bold: true, FontSize: 13}}}, outer, {Kind: Paragraph, Runs: []Run{{Note: &NoteReference{Kind: Footnote, ID: "n"}}}}}, Headers: []Block{table("header", "Header")}, Footers: []Block{table("footer", "Footer")}, Notes: []Note{{Kind: Footnote, ID: "n", Blocks: []Block{table("note", "Note")}}}}
}
func TestColleafTableOriginsAndActualSave(t *testing.T) {
	ctx := context.Background()
	want := colleafTableOriginExample(t)
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
		t.Fatal("native source mutated")
	}
	assertEvidenceSF(t, want, got)
	if file := os.Getenv("MANYDOC_COLLEAF_TABLE_ORIGIN_QA"); file != "" {
		if err = os.WriteFile(file, before, 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, spec := range []struct{ env, mode string }{{"MANYDOC_COLLEAF_TABLE_ORIGIN_SAVED", "saved"}, {"MANYDOC_COLLEAF_TABLE_ORIGIN_EDITED", "edited"}, {"MANYDOC_COLLEAF_TABLE_ORIGIN_DETACHED", "detached"}, {"MANYDOC_COLLEAF_TABLE_ORIGIN_COVERED", "covered"}} {
		t.Run(spec.mode, func(t *testing.T) {
			file := os.Getenv(spec.env)
			if file == "" {
				t.Skip("optional independent reader artifact")
			}
			raw, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			var native colleafNoteSnapshot
			if err = json.Unmarshal(raw, &native); err != nil {
				t.Fatal(err)
			}
			envelope, _ := json.Marshal(colleafEnvelope{colleafEnvelopeType, 3, "univer", colleafEngineVersion, native})
			actual, err := readColleaf(ctx, bytes.NewReader(envelope))
			if err != nil {
				t.Fatal(err)
			}
			expected := colleafTableOriginExample(t)
			switch spec.mode {
			case "edited":
				expected.Blocks[1].Source.Historical = true
				expected.Blocks[1].Rows[0].Cells[0].Source.Historical = true
				expected.Blocks[1].Rows[0].Cells[0].Blocks[0].Runs = []Run{{Text: "Q"}, {Text: "Wide"}}
				expected.Diagnostics = []Diagnostic{{Code: "colleaf_table_source_historical", Message: "Original table or cell source facts are retained as historical after current content, grid or presentation changed"}}
			case "detached":
				expected.UnboundSources = []SourceInfo{*cloneSourceInfo(expected.Blocks[1].Rows[0].Cells[0].Source)}
				expected.Blocks[1].Rows[0].Cells[0].Source = nil
				expected.Diagnostics = []Diagnostic{{Code: "colleaf_table_source_unbound", Message: "Original table or cell source facts are retained unbound after their native ownership disappeared"}}
			case "covered":
				expected.UnboundSources = []SourceInfo{*cloneSourceInfo(expected.Blocks[1].Rows[1].Cells[0].Source)}
				expected.Blocks[1].Rows[1].Cells[0].Source = nil
				expected.Diagnostics = []Diagnostic{{Code: "colleaf_table_source_unbound", Message: "Original table or cell source facts are retained unbound after their native ownership disappeared"}}
			}
			assertEvidenceSF(t, expected, actual)
			sf, err := ToStructured(actual)
			if err != nil {
				t.Fatal(err)
			}
			rawSF, _ := json.Marshal(sf)
			if err = os.WriteFile(file+".sf", rawSF, 0600); err != nil {
				t.Fatal(err)
			}
			model, err := FromStructured(sf)
			if err != nil {
				t.Fatal(err)
			}
			next, err := encodeColleafDocument(ctx, model)
			if err != nil {
				t.Fatal(err)
			}
			stable, err := decodeColleafDocument(ctx, next)
			if err != nil {
				t.Fatal(err)
			}
			assertEvidenceSF(t, actual, stable)
		})
	}
}
func TestColleafTableOriginsMalformed(t *testing.T) {
	ctx := context.Background()
	wire, err := encodeColleafDocument(ctx, colleafTableOriginExample(t))
	if err != nil {
		t.Fatal(err)
	}
	opened, err := openColleafIntent(ctx, wire)
	if err != nil {
		t.Fatal(err)
	}
	var intent colleafDocumentIntent
	if err = colleafResourceMap(opened.Resources, colleafDocumentResource, &intent); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"version", "digest", "profile", "duplicate", "missing"} {
		t.Run(mode, func(t *testing.T) {
			raw, _ := json.Marshal(intent)
			var copy colleafDocumentIntent
			_ = json.Unmarshal(raw, &copy)
			switch mode {
			case "version":
				copy.Evidence.TableOrigins[0].Version = 2
			case "digest":
				copy.Evidence.TableOrigins[0].Source.Element = "changed"
			case "profile":
				copy.Evidence.TableOrigins[0].Profile = colleafRunOriginProfile
			case "duplicate":
				copy.Evidence.TableOrigins = append(copy.Evidence.TableOrigins, copy.Evidence.TableOrigins[0])
			case "missing":
				copy.Evidence.TableOrigins = nil
			}
			native := opened
			native.Resources = colleafReplaceResource(native.Resources, colleafDocumentResource, copy)
			if _, err = decodeColleafDocument(ctx, native); err == nil {
				t.Fatal("invalid table source accepted")
			}
		})
	}
}

func TestColleafTableOriginsSharedFactsAndCoveredReference(t *testing.T) {
	ctx := context.Background()
	d := colleafTableOriginExample(t)
	source := cloneSourceInfo(d.Blocks[1].Source)
	source.TableWidths = nil
	source.Role = ""
	d.Blocks[1].Rows = d.Blocks[1].Rows[:1]
	d.Blocks[1].Rows[0].Cells[0].ColSpan = 0
	d.Blocks[1].Rows[0].Cells[0].Source = source
	d.Blocks[1].Source = source
	native, err := encodeColleafDocument(ctx, d)
	if err != nil {
		t.Fatal(err)
	}
	got, err := decodeColleafDocument(ctx, native)
	if err != nil {
		t.Fatal(err)
	}
	assertEvidenceSF(t, d, got)
	wire, err := encodeColleafDocument(ctx, colleafTableOriginExample(t))
	if err != nil {
		t.Fatal(err)
	}
	for id, raw := range wire.Tables {
		props, _ := colleafStyleObject(raw)
		var rows []json.RawMessage
		_ = json.Unmarshal(props["tableRows"], &rows)
		if len(rows) != 3 {
			continue
		}
		row, _ := colleafStyleObject(rows[2])
		var cells []json.RawMessage
		_ = json.Unmarshal(row["tableCells"], &cells)
		cell, _ := colleafStyleObject(cells[0])
		cell["manydocOrigin"] = sfAttrs(colleafOrigin{Profile: colleafCellOriginProfile, Version: 1, Ref: strings.Repeat("0", 64)})
		cells[0] = sfAttrs(cell)
		row["tableCells"] = sfAttrs(cells)
		rows[2] = sfAttrs(row)
		props["tableRows"] = sfAttrs(rows)
		wire.Tables[id] = sfAttrs(props)
	}
	if _, err = decodeColleafDocument(ctx, wire); err == nil {
		t.Fatal("missing reference on covered cell accepted")
	}
}
func FuzzColleafTableOrigins(f *testing.F) {
	f.Add([]byte(`{"tableId":"probe","tableRows":[{"tableCells":[{"rowSpan":1,"columnSpan":1}]}]}`))
	f.Add([]byte(`{"tableRows":null}`))
	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) > 32768 {
			return
		}
		snapshot := colleafNoteSnapshot{Tables: map[string]json.RawMessage{"probe": raw}}
		_ = visitColleafTableOrigins(&snapshot, false, func(*colleafOrigin) error { return nil })
	})
}
