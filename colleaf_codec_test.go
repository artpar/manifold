package manifold

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestColleafEnvelopeRichSFAndIndependentSave(t *testing.T) {
	ctx := context.Background()
	want := colleafSectionExample(t)
	var out bytes.Buffer
	if err := writeColleaf(ctx, &out, want); err != nil {
		t.Fatal(err)
	}
	var wire map[string]json.RawMessage
	if err := json.Unmarshal(out.Bytes(), &wire); err != nil {
		t.Fatal(err)
	}
	if string(wire["type"]) != `"vibedada-document"` || string(wire["version"]) != `3` || string(wire["engineVersion"]) != `"1.0.4"` {
		t.Fatal("wrong native identity")
	}
	for _, data := range [][]byte{out.Bytes(), bytes.ReplaceAll(out.Bytes(), []byte("😀"), []byte(`\ud83d\ude00`))} {
		got, err := readColleaf(ctx, bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatal("envelope changed rich section/asset/metadata semantics", got.Diagnostics)
		}
		sf, err := ToStructured(got)
		if err != nil {
			t.Fatal(err)
		}
		model, err := FromStructured(sf)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(model, want) {
			t.Fatal("SF changed rich semantics")
		}
	}
	if file := os.Getenv("MANYDOC_COLLEAF_ENVELOPE_SAVED"); file != "" {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		got, err := readColleaf(ctx, bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatal("actual facade changed rich native semantics", got.Diagnostics)
		}
	}
	if file := os.Getenv("MANYDOC_COLLEAF_ENVELOPE_QA"); file != "" {
		if err := os.WriteFile(file, out.Bytes(), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if file := os.Getenv("MANYDOC_COLLEAF_ENVELOPE_SAVED"); file != "" {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		got, err := readColleaf(ctx, bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatal("real saved envelope changed semantics", got.Diagnostics)
		}
	}
}

func TestColleafEnvelopeRejectsMalformedAmbiguousAndUnknown(t *testing.T) {
	var out bytes.Buffer
	if err := writeColleaf(context.Background(), &out, colleafSectionExample(t)); err != nil {
		t.Fatal(err)
	}
	original := out.String()
	for _, pair := range [][2]string{
		{`"version":3`, `"version":2`}, {`"engineVersion":"1.0.4"`, `"engineVersion":"2.0"`}, {`"engine":"univer"`, `"engine":"other"`},
		{`"type":"vibedada-document"`, `"type":"sf"`}, {`"type":`, `"Type":`},
		{`"version":3`, `"version":3,"\u0076ersion":3`}, {`"locale":"enUS"`, `"locale":"enUS","unknownVisibleStory":{}`},
		{`"dataStream":`, `"unknownBody":{},"dataStream":`},
		{`"dataStream":`, `"DataStream":`}, {`"startIndex":0`, `"StartIndex":0`},
		{`"startIndex":0`, `"startIndex":null`}, {`"startIndex":0`, `"startIndex":0,"StartIndex":5`},
		{`😀`, `\ud83d`}, {`😀`, string([]byte{0xff})},
		{`"title":"Native sections"`, `"title":"Native sections","title":"Different"`},
	} {
		if !strings.Contains(original, pair[0]) {
			t.Fatalf("mutation missing %q", pair[0])
		}
		data := strings.Replace(original, pair[0], pair[1], 1)
		if _, err := readColleaf(context.Background(), strings.NewReader(data)); err == nil {
			t.Fatalf("accepted %s", pair[1])
		}
	}
	for _, data := range []string{original + `{}`, `null`, `[]`, `{}`, `{"type":"vibedada-document","version":3,"engine":"univer","engineVersion":"1.0.4","document":null}`} {
		if _, err := readColleaf(context.Background(), strings.NewReader(data)); err == nil {
			t.Fatal("accepted malformed envelope")
		}
	}
	// Duplicate keys inside JSON-in-a-string cannot be seen by an outer decoder.
	wire, err := encodeColleafDocument(context.Background(), colleafSectionExample(t))
	if err != nil {
		t.Fatal(err)
	}
	wire.Resources = append(wire.Resources, colleafResource{Name: colleafDocumentResource, Data: `{"author":"first","\u0061uthor":"second"}`})
	raw, err := json.Marshal(colleafEnvelope{colleafEnvelopeType, 3, "univer", colleafEngineVersion, wire})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = readColleaf(context.Background(), bytes.NewReader(raw)); err == nil || !strings.Contains(err.Error(), "duplicate object member") {
		t.Fatal("embedded resource was collapsed", err)
	}
}

func TestColleafEnvelopeIOAndCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := readColleaf(ctx, strings.NewReader(`{}`)); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err := writeColleaf(ctx, io.Discard, colleafSectionExample(t)); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := readColleaf(context.Background(), nil); err == nil {
		t.Fatal("nil reader accepted")
	}
	if err := writeColleaf(context.Background(), nil, colleafSectionExample(t)); err == nil {
		t.Fatal("nil writer accepted")
	}
}

func FuzzColleafEnvelope(f *testing.F) {
	var out bytes.Buffer
	if err := writeColleaf(context.Background(), &out, colleafSectionExample(f)); err != nil {
		f.Fatal(err)
	}
	f.Add(out.Bytes())
	f.Add([]byte(`null`))
	f.Add([]byte(`{"version":3,"version":2}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		d, err := readColleaf(context.Background(), bytes.NewReader(data))
		if err != nil {
			return
		}
		if err = d.Validate(); err != nil {
			t.Fatal(err)
		}
		if _, err = ToStructured(d); err != nil {
			t.Fatal(err)
		}
	})
}

func TestColleafEnvelopeViewSettingsAndSourceValidation(t *testing.T) {
	ctx := context.Background()
	var out bytes.Buffer
	if err := writeColleaf(ctx, &out, colleafSectionExample(t)); err != nil {
		t.Fatal(err)
	}
	original := out.String()
	for _, settings := range []string{`null`, `{"unknown":true}`, `{"zoomRatio":0}`, `{"zoomRatio":-1}`, `{"zoomRatio":1e400}`} {
		data := strings.Replace(original, `"locale":"enUS"`, `"locale":"enUS","settings":`+settings, 1)
		if _, err := readColleaf(ctx, strings.NewReader(data)); err == nil {
			t.Fatal("accepted invalid settings", settings)
		}
	}
	data := strings.Replace(original, `"locale":"enUS"`, `"locale":"frFR","settings":{"zoomRatio":1.25}`, 1)
	got, err := readColleaf(ctx, strings.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	codes := map[string]bool{}
	for _, d := range got.Diagnostics {
		codes[d.Code] = true
	}
	if !codes["colleaf_view_zoom"] || !codes["colleaf_editor_locale"] {
		t.Fatal("unrepresented editor view settings not disclosed", got.Diagnostics)
	}
	for _, d := range []*Document{{}, {Blocks: []Block{{Kind: Paragraph}}}, {Metadata: Metadata{Title: "Unicode 😀 �"}, Blocks: []Block{{Kind: Paragraph, Runs: []Run{{Text: "Text 😀 �"}}}}}} {
		out.Reset()
		if err := writeColleaf(ctx, &out, d); err != nil {
			t.Fatal(err)
		}
		if _, err := readColleaf(ctx, bytes.NewReader(out.Bytes())); err != nil {
			t.Fatal(err)
		}
	}
	invalid := colleafSectionExample(t)
	invalid.Metadata.Title = string([]byte{0xff})
	out.Reset()
	if err := writeColleaf(ctx, &out, invalid); err == nil || out.Len() != 0 {
		t.Fatal("writer silently replaced source Unicode", err)
	}
}
