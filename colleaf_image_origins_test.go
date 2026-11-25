package manifold

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"testing"
)

func colleafImageOriginExample(t *testing.T) *Document {
	d := colleafImageExample(t)
	base := colleafOriginExample(t)
	d.Pages = base.Pages
	d.Metadata.Title = "Image origins"
	src := func(id string) *SourceInfo {
		s := cloneSourceInfo(base.Blocks[0].Source)
		s.Element = id
		s.Role = "Figure"
		return s
	}
	d.Blocks[0].Source = src("main-image")
	d.Blocks[0].Image.Width = 24
	d.Blocks[0].Image.Height = 12
	d.Blocks[2].Rows[0].Cells[0].Blocks[1].Source = src("cell-image")
	d.Blocks[3].Items[0].Blocks[1].Source = src("list-image")
	d.Blocks[4].Source = src("jpeg-image")
	d.Notes[0].Blocks[0].Source = src("note-image")
	d.Notes[0].Source = cloneSourceInfo(base.Blocks[0].Source)
	d.Notes[0].Source.Element = "note-definition"
	d.Headers = []Block{{Kind: Picture, Image: &Image{Data: runningPNG(t, false), MediaType: "image/png", Alt: "Header PNG", Width: 7, Height: 9}, Source: src("header-image")}}
	d.Footers = []Block{{Kind: Picture, Image: &Image{Data: runningPNG(t, false), MediaType: "image/png", Alt: "Footer PNG", Width: 7, Height: 9}, Source: src("footer-image")}}
	d.Blocks = append([]Block{{Kind: Paragraph, Runs: []Run{{Text: "😀 Image origins", Bold: true, FontSize: 13}}}}, d.Blocks...)
	return d
}
func TestColleafImageOriginsAndActualSave(t *testing.T) {
	ctx := context.Background()
	want := colleafImageOriginExample(t)
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
		t.Fatal("source snapshot mutated")
	}
	assertEvidenceSF(t, want, got)
	if file := os.Getenv("MANYDOC_COLLEAF_IMAGE_ORIGIN_QA"); file != "" {
		if err = os.WriteFile(file, before, 0600); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(file+".replacement.png", runningPNG(t, true), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, spec := range []struct{ env, mode string }{{"MANYDOC_COLLEAF_IMAGE_ORIGIN_SAVED", "saved"}, {"MANYDOC_COLLEAF_IMAGE_ORIGIN_RESIZED", "resized"}, {"MANYDOC_COLLEAF_IMAGE_ORIGIN_ASSET", "asset"}, {"MANYDOC_COLLEAF_IMAGE_ORIGIN_DETACHED", "detached"}} {
		t.Run(spec.mode, func(t *testing.T) {
			file := os.Getenv(spec.env)
			if file == "" {
				t.Skip("optional actual reader artifact")
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
			expected := colleafImageOriginExample(t)
			switch spec.mode {
			case "resized", "asset":
				expected.Blocks[1].Source.Historical = true
				if spec.mode == "resized" {
					expected.Blocks[1].Image.Width = 48
					expected.Blocks[1].Image.Height = 24
					expected.Blocks[1].Image.Alt = "Resized PNG"
				} else {
					expected.Blocks[1].Image.Data = runningPNG(t, true)
				}
				expected.Diagnostics = []Diagnostic{{Code: "colleaf_stale_image_geometry", Message: "Stale image size intent ignored; actual native asset and resized geometry are retained"}, {Code: "colleaf_image_source_historical", Message: "Original image source facts are retained as historical after current asset, description or presentation changed"}}
			case "detached":
				expected.UnboundSources = []SourceInfo{*cloneSourceInfo(expected.Blocks[1].Source)}
				expected.Blocks[1].Source = nil
				expected.Diagnostics = []Diagnostic{{Code: "colleaf_image_source_unbound", Message: "Original image source facts are retained unbound after their native ownership disappeared"}}
			}

			if spec.mode == "saved" {
				for j, resource := range native.Resources {
					if resource.Name != "DOC_DRAWING_PLUGIN" {
						continue
					}
					var mirror struct {
						Data  map[string]json.RawMessage `json:"data"`
						Order []string                   `json:"order"`
					}
					if err = json.Unmarshal([]byte(resource.Data), &mirror); err != nil {
						t.Fatal(err)
					}
					id := native.Body.CustomBlocks[0].ID
					props, err := colleafStyleObject(mirror.Data[id])
					if err != nil {
						t.Fatal(err)
					}
					origin, err := readColleafOriginProfile(props["manydocOrigin"], colleafImageOriginProfile)
					if err != nil {
						t.Fatal(err)
					}
					origin.Ref = strings.Repeat("0", 64)
					props["manydocOrigin"] = sfAttrs(origin)
					mirror.Data[id] = sfAttrs(props)
					candidate := native
					candidate.Resources = append([]colleafResource(nil), native.Resources...)
					candidate.Resources[j].Data = string(sfAttrs(mirror))
					if _, err = decodeColleafDocument(ctx, candidate); err == nil || !strings.Contains(err.Error(), "contradictory native drawing resource/source") {
						t.Fatal("mirror source-binding disagreement was not rejected", err)
					}
				}
			}
			assertEvidenceSF(t, expected, actual)
			sf, err := ToStructured(actual)
			if err != nil {
				t.Fatal(err)
			}
			sfRaw, _ := json.Marshal(sf)
			if err = os.WriteFile(file+".sf", sfRaw, 0600); err != nil {
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
func TestColleafImageOriginsFittedSectionsAndMalformed(t *testing.T) {
	ctx := context.Background()
	d := colleafFittedImageExample(t)
	base := colleafOriginExample(t)
	d.Pages = base.Pages
	var walk func([]Block)
	serial := 0
	walk = func(bs []Block) {
		for i := range bs {
			b := &bs[i]
			if b.Kind == Picture {
				serial++
				b.Source = cloneSourceInfo(base.Blocks[0].Source)
				b.Source.Element = "fitted-" + strconv.Itoa(serial)
				b.Source.Role = "Figure"
			}
			for j := range b.Rows {
				for k := range b.Rows[j].Cells {
					walk(b.Rows[j].Cells[k].Blocks)
				}
			}
			for j := range b.Items {
				walk(b.Items[j].Blocks)
			}
			if b.Section != nil {
				walk(b.Section.Blocks)
				for _, s := range sectionStories(b.Section) {
					if s != nil {
						walk(s.Blocks)
					}
				}
			}
		}
	}
	walk(d.Blocks)
	for i := range d.Notes {
		walk(d.Notes[i].Blocks)
	}
	wire, err := encodeColleafDocument(ctx, d)
	if err != nil {
		t.Fatal(err)
	}
	got, err := decodeColleafDocument(ctx, wire)
	if err != nil {
		t.Fatal(err)
	}
	assertEvidenceSF(t, d, got)
	native, err := encodeColleafDocument(ctx, colleafImageOriginExample(t))
	if err != nil {
		t.Fatal(err)
	}
	opened, err := openColleafIntent(ctx, native)
	if err != nil {
		t.Fatal(err)
	}
	var intent colleafDocumentIntent
	if err = colleafResourceMap(opened.Resources, colleafDocumentResource, &intent); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"version", "digest", "duplicate", "missing"} {
		t.Run(mode, func(t *testing.T) {
			raw, _ := json.Marshal(intent)
			var copy colleafDocumentIntent
			_ = json.Unmarshal(raw, &copy)
			switch mode {
			case "version":
				copy.Evidence.ImageOrigins[0].Version = 2
			case "digest":
				copy.Evidence.ImageOrigins[0].Source.Element = "changed"
			case "duplicate":
				copy.Evidence.ImageOrigins = append(copy.Evidence.ImageOrigins, copy.Evidence.ImageOrigins[0])
			case "missing":
				copy.Evidence.ImageOrigins = nil
			}
			candidate := opened
			candidate.Resources = colleafReplaceResource(candidate.Resources, colleafDocumentResource, copy)
			if _, err = decodeColleafDocument(ctx, candidate); err == nil {
				t.Fatal("malformed image source accepted")
			}
		})
	}
}
func FuzzColleafImageOrigins(f *testing.F) {
	f.Add([]byte(`{"drawingId":"image"}`))
	f.Add([]byte(sfAttrs(map[string]any{"manydocOrigin": colleafOrigin{Profile: colleafImageOriginProfile, Version: 1, Ref: strings.Repeat("0", 64)}})))
	f.Add([]byte(`{"manydocOrigin":{"profile":"manydoc.image-origin","version":2}}`))
	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) > 32768 {
			return
		}
		snapshot := colleafNoteSnapshot{Drawings: map[string]json.RawMessage{"image": raw}}
		_ = visitColleafImageOrigins(&snapshot, false, func(*colleafOrigin) error { return nil })
	})
}
