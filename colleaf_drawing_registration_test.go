package manydoc

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"testing"
)

// The native controller resets its source map from this resource on load. A
// valid standalone source descriptor alone therefore does not establish paint.
func TestColleafPublicDrawingRegistration(t *testing.T) {
	ctx := context.Background()
	want := colleafImageExample(t)
	want.Headers = []Block{want.Blocks[0]}
	var output bytes.Buffer
	if err := WriteVDOCS(ctx, &output, want); err != nil {
		t.Fatal(err)
	}
	var envelope colleafEnvelope
	if err := json.Unmarshal(output.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	wire := envelope.Document
	var mirror struct {
		Data  map[string]json.RawMessage `json:"data"`
		Order []string                   `json:"order"`
	}
	found := 0
	for _, r := range wire.Resources {
		if r.Name == "DOC_DRAWING_PLUGIN" {
			found++
			if err := json.Unmarshal([]byte(r.Data), &mirror); err != nil {
				t.Fatal(err)
			}
		}
	}
	if found != 1 || len(mirror.Data) != 5 || len(mirror.Order) != 4 {
		t.Fatalf("missing root/header registration or incorrect body order: %d/%d/%d", found, len(mirror.Data), len(mirror.Order))
	}
	for id, n := range wire.Notes {
		for drawing := range n.Drawings {
			if _, ok := mirror.Data[drawing]; ok {
				t.Fatal("note source duplicated into root registry", id, drawing)
			}
		}
	}
	if err := checkColleafDrawingMirror(wire, colleafResource{Name: "DOC_DRAWING_PLUGIN", Data: string(sfAttrs(mirror))}); err != nil {
		t.Fatal(err)
	}
	got, err := ReadVDOCS(ctx, bytes.NewReader(output.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatal("native registration changed document semantics", got.Diagnostics)
	}
	// Contradictory mirrors still reject; registration cannot override content.
	delete(mirror.Data, mirror.Order[0])
	wire.Resources = colleafReplaceResource(wire.Resources, "DOC_DRAWING_PLUGIN", mirror)
	if _, err = decodeColleafDocument(ctx, wire); err == nil {
		t.Fatal("mirror order without its source accepted")
	}
}
