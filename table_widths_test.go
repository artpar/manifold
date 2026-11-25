package manifold

import (
	"bytes"
	"context"
	"math"
	"math/rand"
	"os"
	"reflect"
	"testing"
)

func TestTableWidthQuantizationRoundtrip(t *testing.T) {
	rng := rand.New(rand.NewSource(729))
	for i := 0; i < 120; i++ {
		scale := 900.0
		if i%2 == 0 {
			scale = 60
		}
		d := &Document{Layout: Layout{Width: 400, Height: 500, Margin: 40}, Blocks: []Block{widthTable(10+rng.Float64()*scale, 10+rng.Float64()*scale, 10+rng.Float64()*scale)}}
		var data bytes.Buffer
		if err := Write(context.Background(), &data, d, DOCX, WriteOptions{}); err != nil {
			t.Fatal(err)
		}
		back, err := ReadDOCX(context.Background(), &data)
		if err != nil {
			t.Fatal(i, d.Blocks[0].ColumnWidths, err)
		}
		if !reflect.DeepEqual(back.Blocks[0].ColumnWidths, d.Blocks[0].ColumnWidths) {
			t.Fatal("preferred widths rounded")
		}
	}
}

func widthTable(widths ...float64) Block {
	return Block{Kind: Table, ColumnWidths: widths, Rows: []Row{
		{RepeatHeader: true, Cells: []Cell{{ColSpan: 2, Blocks: []Block{mergeParagraph("HEADER")}}, {Blocks: []Block{mergeParagraph("END")}}}},
		{Cells: []Cell{{RowSpan: 2, Blocks: []Block{mergeParagraph("LEFT")}}, {Blocks: []Block{mergeParagraph("DESCRIPTION")}}, {Blocks: []Block{mergeParagraph("VALUE")}}}},
		{Cells: []Cell{{ColSpan: 2, Blocks: []Block{mergeParagraph("BOTTOM")}}}},
	}}
}

func TestTableWidthsSFAndDOCXRoundtrip(t *testing.T) {
	for _, widths := range [][]float64{nil, {45, 180, 75}, {91.123, 360.456, 150.789}, {.8, 179.35, 27.4}} {
		d := &Document{Layout: Layout{Width: 400, Height: 500, Margin: 50}, Blocks: []Block{widthTable(widths...)}}
		for _, format := range []Format{SF, DOCX, JSON} {
			var data bytes.Buffer
			if err := Write(context.Background(), &data, d, format, WriteOptions{}); err != nil {
				t.Fatal(widths, format, err)
			}
			back, err := Read(context.Background(), &data, format, ReadOptions{})
			if err != nil {
				t.Fatal(widths, format, err)
			}
			if !reflect.DeepEqual(back.Blocks[0].ColumnWidths, widths) {
				t.Fatalf("%s preferred widths changed: %v != %v", format, back.Blocks[0].ColumnWidths, widths)
			}
			if len(back.Diagnostics) != 0 {
				t.Fatal(back.Diagnostics)
			}
		}
	}
}

func TestTableWidthAdmission(t *testing.T) {
	for _, widths := range [][]float64{{1}, {1, 0, 2}, {1, -1, 2}, {1, math.NaN(), 2}, {1, math.Inf(1), 2}, {1, 1e8, 2}} {
		d := &Document{Blocks: []Block{widthTable(widths...)}}
		var out bytes.Buffer
		if err := Write(context.Background(), &out, d, SF, WriteOptions{}); err == nil || out.Len() != 0 {
			t.Fatal("invalid widths admitted", widths, err)
		}
	}
	d := &Document{Blocks: []Block{{Kind: Paragraph, ColumnWidths: []float64{1}}}}
	if _, err := ToStructured(d); err == nil {
		t.Fatal("non-table widths admitted")
	}
	// Widths may be positive in SF yet not representable in DOCX twips.
	d.Blocks = []Block{widthTable(.001, 200, 30)}
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, DOCX, WriteOptions{}); err == nil || out.Len() != 0 {
		t.Fatal("sub-twip width silently rounded away")
	}
}

func TestDOCXWidthMetadataChecked(t *testing.T) {
	for _, attrs := range []string{`md:columnWidths="[10,20,30]"`, `md:columnWidths="[90,180,30]"`, `md:columnWidths="[0,180,75]"`} {
		xml := `<w:tbl xmlns:md="` + docxNotesNS + `" ` + attrs + ` md:columnWidthContainer="300"><w:tblGrid><w:gridCol w:w="900"/><w:gridCol w:w="3600"/><w:gridCol w:w="1500"/></w:tblGrid><w:tr><w:tc/><w:tc/><w:tc/></w:tr></w:tbl>`
		n, err := parseXML(context.Background(), []byte(xml))
		if err != nil {
			t.Fatal(err)
		}
		r := docxReader{ctx: context.Background(), doc: &Document{}}
		if _, err = r.table(n, nil, 0); err == nil {
			t.Fatal("contradictory metadata accepted", attrs)
		}
	}
}

func TestRealWordTableWidthGrid(t *testing.T) {
	raw, err := os.ReadFile("testdata/real/mqtt-v5.0-os.docx")
	if err != nil {
		t.Fatal(err)
	}
	d, err := ReadDOCX(context.Background(), bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	var tables []Block
	var walk func([]Block)
	walk = func(bs []Block) {
		for _, b := range bs {
			if b.Kind == Table {
				tables = append(tables, b)
			}
			if b.Section != nil {
				walk(b.Section.Blocks)
			}
			for _, row := range b.Rows {
				for _, c := range row.Cells {
					walk(c.Blocks)
				}
			}
		}
	}
	walk(d.Blocks)
	// Independent original OOXML grid, not an output-derived golden value.
	want := []float64{77.15, 179.35, 1, 25.05, 26.1, .95, 25.15, .9, 25.2, .85, 25.25, .8, 25.3, .75, 25.35, .95, 27.4}
	if len(tables) != 59 || !reflect.DeepEqual(tables[51].ColumnWidths, want) {
		t.Fatalf("source physical grid changed: %v", tables[51].ColumnWidths)
	}
	widths, err := fittedColumnWidths(tables[51].ColumnWidths, 17, 468)
	if err != nil {
		t.Fatal(err)
	}
	if widths[1]+widths[2] < 180 {
		t.Fatal("description cell narrowed to equal logical columns")
	}
}

func TestPDFTableWidthsCarrySeparateProvenance(t *testing.T) {
	d := readRecovery(t, taggedSpanPDF())
	b := d.Blocks[0]
	if !reflect.DeepEqual(b.ColumnWidths, []float64{80, 80, 80}) || b.Source.Structure != "explicit" || b.Source.TableWidths == nil || b.Source.TableWidths.Evidence != "painted_grid_adjacency" {
		t.Fatal("physical width evidence conflated with explicit membership", b.ColumnWidths, b.Source)
	}
	assertRecoverySF(t, d)
	// Removing the ruling paint retains explicit membership but supplies no widths.
	d = readRecovery(t, recoveryPDF(taggedSpanObjects("")...))
	if len(d.Blocks[0].ColumnWidths) != 0 || d.Blocks[0].Source.TableWidths != nil {
		t.Fatal("widths invented without painted grid")
	}
}
