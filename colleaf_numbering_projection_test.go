package manydoc

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

func colleafZeroExample(t testing.TB) *Document {
	p := func(text string) Block { return Block{Kind: Paragraph, Runs: []Run{{Text: text, Bold: true}}} }
	list := func(format string, legal bool) Block {
		return Block{Kind: ListBlock, List: "number", Start: listStartPointer(0), Numbering: &ListNumbering{Template: "%1%", Formats: []string{format}, Legal: legal}, Items: []ListItem{{Blocks: []Block{p(format + " zero"), p("Continuation")}}, {Blocks: []Block{p(format + " one")}}, {Blocks: []Block{p(format + " two")}}}}
	}
	d := &Document{Metadata: Metadata{Title: "Native zero and legal"}, Headers: []Block{list("lowerRoman", false)}}
	for _, format := range []string{"decimalZero", "upperLetter", "lowerLetter", "upperRoman", "lowerRoman"} {
		d.Blocks = append(d.Blocks, list(format, false))
	}
	legal := list("decimalZero", true)
	legal.Numbering.Template = "%1."
	d.Blocks = append(d.Blocks, legal)
	parent := list("upperLetter", false)
	parent.Numbering.Template = "%1."
	child := list("lowerRoman", false)
	child.Numbering = &ListNumbering{Level: 1, Template: "%1.%2", Formats: []string{"upperLetter", "lowerRoman"}, Ancestors: []int{99}}
	parent.Items[0].Blocks = append(parent.Items[0].Blocks, child)
	d.Blocks = append(d.Blocks, parent)
	compound := list("lowerRoman", true)
	compound.Numbering = &ListNumbering{Level: 1, Template: "%1.%2", Formats: []string{"decimalZero", "lowerRoman"}, Ancestors: []int{0}, Legal: true, Independent: true}
	d.Blocks = append(d.Blocks, compound)
	d.Blocks = append(d.Blocks, Block{Kind: Table, Rows: []Row{{Cells: []Cell{{Blocks: []Block{list("decimalZero", false)}}}}}}, Block{Kind: Paragraph, Runs: []Run{{Text: "Note "}, {Note: &NoteReference{Kind: Footnote, ID: "zero"}}}})
	d.Notes = []Note{{Kind: Footnote, ID: "zero", Blocks: []Block{list("upperRoman", false)}}}
	return d
}
func TestColleafZeroLegalSFAndActualSave(t *testing.T) {
	ctx := context.Background()
	want := resolveNumberingDocument(colleafZeroExample(t))
	wire, err := encodeColleafDocument(ctx, want)
	if err != nil {
		t.Fatal(err)
	}
	got, err := decodeColleafDocument(ctx, wire)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		a, _ := json.Marshal(got)
		b, _ := json.Marshal(want)
		t.Fatalf("zero/legal semantics differ\ngot%s\nwant%s", a, b)
	}
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
	if !reflect.DeepEqual(again, wire) {
		t.Fatal("SF changed native projection/types/ownership")
	}
	if file := os.Getenv("MANYDOC_COLLEAF_ZERO_QA"); file != "" {
		data, err := json.Marshal(wire)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(file, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if file := os.Getenv("MANYDOC_COLLEAF_ZERO_SAVED"); file != "" {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var saved colleafNoteSnapshot
		if err = json.Unmarshal(data, &saved); err != nil {
			t.Fatal(err)
		}
		got, err = decodeColleafDocument(ctx, saved)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatal("actual facade changed zero/legal semantics", got.Diagnostics)
		}
	}
	if file := os.Getenv("MANYDOC_COLLEAF_ZERO_EDITED"); file != "" {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var edited colleafNoteSnapshot
		if err = json.Unmarshal(data, &edited); err != nil {
			t.Fatal(err)
		}
		got, err = decodeColleafDocument(ctx, edited)
		if err != nil {
			t.Fatal(err)
		}
		if got.Blocks[0].Numbering.Formats[0] != "decimal" || listStart(got.Blocks[0]) != 5 {
			t.Fatal("stale zero intent hid native edit", got.Blocks[0])
		}
		sf, err := ToStructured(got)
		if err != nil {
			t.Fatal(err)
		}
		model, err := FromStructured(sf)
		if err != nil {
			t.Fatal(err)
		}
		labels := colleafNumberLabels(t, model.Blocks)
		expected := []string{"5%", "06%", "07%", "0%", "A%", "B%", "0%", "a%", "b%", "0%", "I%", "II%", "0%", "i%", "ii%", "00.", "01.", "02.", "5.", "5.0", "5.i", "5.ii", "F.", "G.", "0.0", "0.1", "0.2", "0%", "01%", "02%"}
		if !reflect.DeepEqual(labels, expected) {
			t.Fatal("SF lost edited actual markers", labels)
		}
		found := false
		for _, diag := range got.Diagnostics {
			found = found || diag.Code == "colleaf_stale_numbering_projection"
		}
		if !found {
			t.Fatal("stale zero projection not diagnosed")
		}
	}
}
func TestColleafForeignZeroAndLegalPreserveMarkers(t *testing.T) {
	for _, format := range []string{"decimalZero", "upperLetter", "lowerLetter", "upperRoman", "lowerRoman"} {
		for _, legal := range []bool{false, true} {
			glyph, _ := colleafNumberGlyph(format)
			source := colleafNativeList{Type: "t", Levels: []colleafListLevel{{Format: "%1.", Glyph: &glyph, Start: -1, Align: 1, Legal: legal}}}
			body := colleafBody{DataStream: "X\rY\rZ\r\n", Paragraphs: []colleafParagraphRecord{{StartIndex: 1}, {StartIndex: 3}, {StartIndex: 5}}}
			for i := range body.Paragraphs {
				body.Paragraphs[i].Bullet = sfAttrs(colleafBullet{Type: "t", ID: "l"})
			}
			d, err := decodeColleafBody(context.Background(), body, colleafTextContext{Lists: map[string]json.RawMessage{"t": sfAttrs(source)}}, nil)
			if err != nil {
				t.Fatal(err)
			}
			var labels []string
			for _, b := range d.Blocks {
				if b.Kind != ListBlock {
					t.Fatal("lost list item grouping")
				}
				for i := range b.Items {
					label, err := numberedLabel(b, i)
					if err != nil {
						t.Fatal(err)
					}
					labels = append(labels, label)
				}
			}
			var expected []string
			switch {
			case format == "decimalZero":
				expected = []string{"00.", "01.", "02."}
			case legal:
				expected = []string{"0.", "1.", "2."}
			default:
				a, _ := numberText(format, 1)
				b, _ := numberText(format, 2)
				expected = []string{".", a + ".", b + "."}
			}
			if !reflect.DeepEqual(labels, expected) {
				t.Fatal(format, legal, labels, expected)
			}
			if _, err = ToStructured(d); err != nil {
				t.Fatal(err)
			}
		}
	}
}
func TestColleafMalformedNumberingIntent(t *testing.T) {
	for _, p := range []colleafNumberProjection{
		{}, {Formats: map[int]string{-1: "upperLetter"}, ZeroLevels: []int{-1}},
		{Formats: map[int]string{0: "decimal"}, ZeroLevels: []int{0}},
		{Formats: map[int]string{0: "upperLetter"}, ZeroLevels: []int{0, 0}},
		{Formats: map[int]string{0: "upperLetter"}},
		{Formats: map[int]string{0: "upperRoman"}, Legal: true},
		{Formats: map[int]string{0: "decimalZero"}, ZeroLevels: []int{0}, Legal: true},
	} {
		if err := validateColleafNumberProjection(p, 0); err == nil {
			t.Fatal("malformed projection accepted", p)
		}
	}
	if _, err := colleafListBindingTypes(colleafListBinding{Type: "t", Projections: map[string]colleafNumberProjection{"unknown": {Formats: map[int]string{0: "upperLetter"}, ZeroLevels: []int{0}}}}); err == nil {
		t.Fatal("unowned type projection accepted")
	}
}

func colleafNumberLabels(t testing.TB, blocks []Block) []string {
	t.Helper()
	var labels []string
	var walk func([]Block)
	walk = func(blocks []Block) {
		for _, b := range blocks {
			switch b.Kind {
			case ListBlock:
				for i, item := range b.Items {
					label, err := numberedLabel(b, i)
					if err != nil {
						t.Fatal(err)
					}
					labels = append(labels, label)
					walk(item.Blocks)
				}
			case Table:
				for _, r := range b.Rows {
					for _, c := range r.Cells {
						walk(c.Blocks)
					}
				}
			case SectionBlock:
				walk(b.Section.Blocks)
			}
		}
	}
	walk(blocks)
	return labels
}
func TestColleafNumberProjectionEmbeddedContradictions(t *testing.T) {
	for _, mode := range []string{"negative", "unowned", "collision"} {
		ctx := context.Background()
		wire, err := encodeColleafDocument(ctx, colleafZeroExample(t))
		if err != nil {
			t.Fatal(err)
		}
		wire, err = openColleafIntent(ctx, wire)
		if err != nil {
			t.Fatal(err)
		}
		var ownership colleafListOwnership
		if err = colleafResourceMap(wire.Resources, colleafListResource, &ownership); err != nil {
			t.Fatal(err)
		}
		b := &ownership.Stories[""][0]
		switch mode {
		case "negative":
			name := b.Items[0].Type
			b.Projections[name] = colleafNumberProjection{Formats: map[int]string{-1: "upperLetter"}, ZeroLevels: []int{-1}}
		case "unowned":
			b.Projections["unowned"] = colleafNumberProjection{Formats: map[int]string{0: "upperLetter"}, ZeroLevels: []int{0}}
		case "collision":
			b.Items[0].Type = ownership.Stories[""][1].Type
		}
		wire.Resources = colleafReplaceResource(wire.Resources, colleafListResource, ownership)
		if err = sealColleafIntent(&wire); err != nil {
			t.Fatal(err)
		}
		if _, err = decodeColleafDocument(ctx, wire); err == nil {
			t.Fatal("malformed native intent accepted", mode)
		}
	}
}
func FuzzColleafNumberProjection(f *testing.F) {
	f.Add(uint8(0), uint8(0), uint8(0), false)
	f.Add(uint8(5), uint8(8), uint8(0), true)
	f.Fuzz(func(t *testing.T, format, rawLevel, rawValue uint8, legal bool) {
		level, value := int(rawLevel%9), int(rawValue%3)
		n := &ListNumbering{Level: level, Template: "%1.", Legal: legal}
		source := colleafNativeList{}
		for l := 0; l <= level; l++ {
			spec := colleafNumberFormats[(int(format)+l)%len(colleafNumberFormats)]
			n.Formats = append(n.Formats, spec)
			glyph, _ := colleafNumberGlyph(spec)
			source.Levels = append(source.Levels, colleafListLevel{Glyph: &glyph})
			if l < level {
				n.Ancestors = append(n.Ancestors, (value+l)%3)
			}
		}
		source.Levels[level].Legal = legal
		projected, p := projectColleafNumbering(source, n, value)
		if p == nil {
			return
		}
		if err := validateColleafNumberProjection(*p, level); err != nil {
			t.Fatal(err)
		}
		imported := *n
		imported.Formats = nil
		for _, spec := range projected.Levels {
			imported.Formats = append(imported.Formats, colleafNumberFormats[*spec.Glyph-2])
		}
		if !restoreColleafNumberProjection(&imported, *p, value, n.Ancestors, projected) || !reflect.DeepEqual(&imported, n) {
			t.Fatal("format projection failed to retain source semantics")
		}
	})
}

func TestColleafProjectedFormatCanonicalMemberNames(t *testing.T) {
	for _, raw := range []string{`null`, `[]`, `{"01":"upperLetter"}`, `{"-1":"upperLetter"}`, `{"9":"upperLetter"}`, `{"0":null}`, `{"0":3}`, `{"+1":"upperLetter","1":"lowerRoman"}`} {
		var formats colleafProjectedFormats
		if err := json.Unmarshal([]byte(raw), &formats); err == nil {
			t.Fatal("ambiguous/malformed native format keys accepted", raw)
		}
	}
}
