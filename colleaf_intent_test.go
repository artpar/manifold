package manydoc

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

func TestColleafNativeIntentCarrierAndLegacyRead(t *testing.T) {
	ctx := context.Background()
	wire, err := encodeColleafNotes(ctx, colleafImageExample(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(wire.Resources) != 0 {
		t.Fatal("unregistered resources would be removed by FDocument.save")
	}
	carrier := wire.Body.CustomRanges[len(wire.Body.CustomRanges)-1]
	if carrier.Type != 5 || carrier.StartIndex != colleafUTF16Len(wire.Body.DataStream)-1 || carrier.EndIndex != carrier.StartIndex {
		t.Fatal("carrier is not on terminal LF", carrier)
	}
	before, err := json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := openColleafIntent(ctx, wire)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(wire)
	if string(before) != string(after) {
		t.Fatal("opening intent mutated caller")
	}
	if len(legacy.Resources) != 3 || len(legacy.Body.CustomRanges) != 0 {
		t.Fatal("incorrect intent transport extraction")
	}
	doc, err := decodeColleafNotes(ctx, legacy, colleafTextContext{})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(doc, colleafImageExample(t)) {
		t.Fatal("legacy direct-resource input changed semantics")
	}
	for _, mutate := range []func(*colleafNoteSnapshot){
		func(w *colleafNoteSnapshot) {
			a := w.Body.CustomRanges[0]
			a.ID = "duplicate"
			w.Body.CustomRanges = append(w.Body.CustomRanges, a)
		},
		func(w *colleafNoteSnapshot) {
			w.Body.CustomRanges[0].Properties = sfAttrs(colleafIntentCarrier{Profile: colleafIntentProfile, Version: 2})
		},
		func(w *colleafNoteSnapshot) {
			w.Body.CustomRanges[0].Properties = sfAttrs(colleafIntentCarrier{Profile: colleafIntentProfile, Version: 1, Records: []colleafResource{{Name: "unknown", Data: "{}"}}})
		},
		func(w *colleafNoteSnapshot) { w.Body.CustomRanges[0].EndIndex = colleafUTF16Len(w.Body.DataStream) },
	} {
		input, err := encodeColleafNotes(ctx, colleafImageExample(t))
		if err != nil {
			t.Fatal(err)
		}
		mutate(&input)
		if _, err = decodeColleafNotes(ctx, input, colleafTextContext{}); err == nil {
			t.Fatal("invalid carrier accepted")
		}
	}
}
func TestColleafFacadeSavedIntentSemantics(t *testing.T) {
	originals := map[string]*Document{"notes": colleafNoteExample(), "lists": resolveNumberingDocument(colleafListExample()), "images": colleafImageExample(t)}
	snapshots := map[string]colleafNoteSnapshot{}
	for name, doc := range originals {
		wire, err := encodeColleafNotes(context.Background(), doc)
		if err != nil {
			t.Fatal(err)
		}
		snapshots[name] = wire
	}
	if file := os.Getenv("MANYDOC_COLLEAF_FACADE_QA"); file != "" {
		data, err := json.MarshalIndent(snapshots, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(file, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if file := os.Getenv("MANYDOC_COLLEAF_FACADE_SAVED"); file != "" {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var saved map[string]colleafNoteSnapshot
		if err = json.Unmarshal(data, &saved); err != nil {
			t.Fatal(err)
		}
		for name, original := range originals {
			wire, ok := saved[name]
			if !ok {
				t.Fatal("missing facade-saved case", name)
			}
			doc, err := decodeColleafNotes(context.Background(), wire, colleafTextContext{})
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
	if file := os.Getenv("MANYDOC_COLLEAF_FACADE_EDITED"); file != "" {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var wire colleafNoteSnapshot
		if err = json.Unmarshal(data, &wire); err != nil {
			t.Fatal(err)
		}
		doc, err := decodeColleafNotes(context.Background(), wire, colleafTextContext{})
		if err != nil {
			t.Fatal(err)
		}
		want := resolveNumberingDocument(colleafListExample())
		want.Blocks[0].Items[0].Blocks[0].Runs[0].Text = "XParent one"
		// TextX may introduce a style interval boundary without changing typography.
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
func TestColleafRegisteredDrawingMirrorMustMatchPersistedSource(t *testing.T) {
	wire, err := encodeColleafNotes(context.Background(), colleafImageExample(t))
	if err != nil {
		t.Fatal(err)
	}
	resource := colleafResource{Name: "DOC_DRAWING_PLUGIN", Data: string(sfAttrs(map[string]any{"data": wire.Drawings, "order": wire.DrawingOrder}))}
	if err = checkColleafDrawingMirror(wire, resource); err != nil {
		t.Fatal(err)
	}
	resource.Data = string(sfAttrs(map[string]any{"data": map[string]json.RawMessage{"orphan": wire.Drawings[wire.DrawingOrder[0]]}, "order": []string{"orphan"}}))
	if err = checkColleafDrawingMirror(wire, resource); err == nil {
		t.Fatal("orphan resource source silently discarded")
	}
}

func TestColleafEditorPermissionResourceIsOutsideDocumentInterpretation(t *testing.T) {
	want := colleafNoteExample()
	wire, err := encodeColleafNotes(context.Background(), want)
	if err != nil {
		t.Fatal(err)
	}
	wire.Resources = append(wire.Resources, colleafResource{Name: "DOC_OBJECT_PERMISSION_PLUGIN", Data: `{"actor":"editor-local","editable":false}`})
	got, err := decodeColleafNotes(context.Background(), wire, colleafTextContext{})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatal("SDK permission state changed document interpretation or admission", got.Diagnostics)
	}
}
func FuzzColleafIntent(f *testing.F) {
	wire, err := encodeColleafNotes(context.Background(), colleafNoteExample())
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
		var input colleafNoteSnapshot
		if json.Unmarshal(data, &input) != nil {
			return
		}
		doc, err := decodeColleafNotes(context.Background(), input, colleafTextContext{})
		if err == nil {
			if err = doc.Validate(); err != nil {
				t.Fatal("accepted invalid intent/document graph", err)
			}
		}
	})
}
