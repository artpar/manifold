package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	"github.com/artpar/manifold"
	"github.com/artpar/manifold/presentation"
)

func makeWorkloads(ctx context.Context, dir string) ([]workload, error) {
	var all []workload
	// Each conversion reparses immutable input and uses request-owned state. No
	// parsed Document or mutable output buffer is shared between callers.
	conversion := func(name, description string, data []byte, from, to manifold.Format) {
		data = bytes.Clone(data)
		all = append(all, workload{Name: name, Description: description, InputBytes: len(data), InputSHA256: digest(data), prepare: func(int) operation {
			return func(ctx context.Context) (observation, error) {
				d, err := manifold.Read(ctx, bytes.NewReader(data), from, manifold.ReadOptions{})
				if err != nil {
					return observation{}, err
				}
				var out bytes.Buffer
				if err := manifold.Write(ctx, &out, d, to, manifold.WriteOptions{AllowLossy: true}); err != nil {
					return observation{}, err
				}
				if out.Len() == 0 {
					return observation{}, fmt.Errorf("empty conversion output")
				}
				return observation{bytes: int64(out.Len())}, nil
			}
		}})
	}
	for _, spec := range []struct {
		name, path, description string
		from, to                manifold.Format
	}{
		{"docx-small-sf", "testdata/corpus/docx_basic.docx", "Original corpus basic DOCX to serialized SF", manifold.DOCX, manifold.SF},
		{"docx-mqtt-sf", "testdata/real/mqtt-v5.0-os.docx", "Original OASIS MQTT Word document: 4998 nonempty body paragraphs and 59 tables to SF", manifold.DOCX, manifold.SF},
		{"pdf-small-sf", "testdata/assets/w3c-dummy.pdf", "Original one-page W3C PDF to SF, default structure/inference enabled", manifold.PDF, manifold.SF},
		{"pdf-circular-sf", "testdata/real/copyright-office-circular-1-2021.pdf", "Original ten-page Copyright Office PDF to SF, default structure/inference enabled", manifold.PDF, manifold.SF},
	} {
		data, err := os.ReadFile(spec.path)
		if err != nil {
			return nil, err
		}
		conversion(spec.name, spec.description, data, spec.from, spec.to)
	}
	doc := &manifold.Document{Metadata: manifold.Metadata{Title: "Manifold load-test fixture"}}
	for i := range 100 {
		if i%10 == 0 {
			doc.Blocks = append(doc.Blocks, manifold.Block{Kind: manifold.Heading, Level: 1, Runs: []manifold.Run{{Text: fmt.Sprintf("Chapter %d", i/10+1)}}})
		}
		doc.Blocks = append(doc.Blocks, manifold.Block{Kind: manifold.Paragraph, Runs: []manifold.Run{{Text: fmt.Sprintf("Paragraph %03d. ", i)}, {Text: strings.Repeat("Deterministic document conversion preserves text and supported structure. ", 3)}, {Text: "Bold emphasis. ", Bold: true}, {Text: "Italic emphasis.", Italic: true}}})
		if i%20 == 0 {
			table := manifold.Block{Kind: manifold.Table}
			for r := range 6 {
				table.Rows = append(table.Rows, manifold.Row{Cells: []manifold.Cell{{Blocks: []manifold.Block{{Kind: manifold.Paragraph, Runs: []manifold.Run{{Text: fmt.Sprintf("Row %d", r)}}}}}, {Blocks: []manifold.Block{{Kind: manifold.Paragraph, Runs: []manifold.Run{{Text: "Supported table cell content"}}}}}}})
			}
			doc.Blocks = append(doc.Blocks, table)
		}
	}
	var sf, vdocs, docx bytes.Buffer
	for _, target := range []struct {
		format manifold.Format
		buffer *bytes.Buffer
	}{{manifold.SF, &sf}, {manifold.VDOCS, &vdocs}, {manifold.DOCX, &docx}} {
		if err := manifold.Write(ctx, target.buffer, doc, target.format, manifold.WriteOptions{AllowLossy: true}); err != nil {
			return nil, err
		}
	}
	for _, to := range []manifold.Format{manifold.DOCX, manifold.PDF, manifold.VDOCS} {
		conversion("sf-medium-"+string(to), "Synthetic mixed document: 100 paragraphs, 10 headings, five 6x2 tables, bold/italic runs; parse SF and serialize "+string(to), sf.Bytes(), manifold.SF, to)
	}
	conversion("vdocs-medium-sf", "Synthetic mixed document VDOCS back to serialized SF", vdocs.Bytes(), manifold.VDOCS, manifold.SF)
	conversion("docx-medium-pdf", "Synthetic mixed document DOCX parse and complete PDF generation", docx.Bytes(), manifold.DOCX, manifold.PDF)

	img := image.NewRGBA(image.Rect(0, 0, 128, 128))
	for y := 0; y < 128; y++ {
		for x := 0; x < 128; x++ {
			img.SetRGBA(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: uint8(x ^ y), A: 255})
		}
	}
	var picture bytes.Buffer
	if err := png.Encode(&picture, img); err != nil {
		return nil, err
	}
	for _, size := range []int{10, 100} {
		deck := filepath.Join(dir, fmt.Sprintf("deck-%d.pptx", size))
		fixture := &manifold.Document{Metadata: manifold.Metadata{Title: "Visible load-test slides"}, Presentation: &manifold.Presentation{Version: 1, Width: 960, Height: 540}}
		for i := range size {
			slide := manifold.PresentationSlide{ID: fmt.Sprint(256 + i), Part: fmt.Sprintf("ppt/slides/slide%d.xml", i+1), Objects: []manifold.SlideObject{
				{ID: "2", Kind: "shape", Geometry: "rect", X: 40, Y: 30, Width: 850, Height: 80, Content: []manifold.Block{{Kind: manifold.Paragraph, Runs: []manifold.Run{{Text: fmt.Sprintf("Manifold visible slide %d", i+1), FontSize: 24, Bold: true}}}}},
				{ID: "3", Kind: "shape", Geometry: "rect", X: 40, Y: 130, Width: 850, Height: 120, Content: []manifold.Block{{Kind: manifold.Paragraph, Runs: []manifold.Run{{Text: strings.Repeat("Measured visible body content with supported text. ", 3), FontSize: 16}}}}},
			}}
			if i%3 == 0 {
				slide.Objects = append(slide.Objects, manifold.SlideObject{ID: "4", Kind: "image", X: 40, Y: 290, Width: 128, Height: 128, Image: &manifold.Image{Data: picture.Bytes(), MediaType: "image/png", Alt: "Deterministic colored pixels"}})
			}
			fixture.Presentation.Slides = append(fixture.Presentation.Slides, slide)
		}
		var encoded bytes.Buffer
		if err := manifold.Write(ctx, &encoded, fixture, manifold.PPTX, manifold.WriteOptions{}); err != nil {
			return nil, err
		}
		if err := os.WriteFile(deck, encoded.Bytes(), 0600); err != nil {
			return nil, err
		}
		data, err := os.ReadFile(deck)
		if err != nil {
			return nil, err
		}
		all = append(all, workload{Name: fmt.Sprintf("pptx-%d-inspect", size), Description: fmt.Sprintf("Synthetic %d-slide PPTX with explicit visible text/image frames; read/inspect complete package", size), InputBytes: len(data), InputSHA256: digest(data), prepare: func(int) operation {
			return func(ctx context.Context) (observation, error) {
				r, err := presentation.Inspect(ctx, deck)
				if err != nil {
					return observation{}, err
				}
				if r.Status != "ok" || r.Inspection == nil || r.Inspection.SlideCount != size {
					return observation{}, fmt.Errorf("invalid inspection: %+v", r)
				}
				return observation{}, nil
			}
		}})
		if size == 10 {
			canonical, err := manifold.Read(ctx, bytes.NewReader(data), manifold.PPTX, manifold.ReadOptions{})
			if err != nil {
				return nil, err
			}
			var sfSlides, pdfSlides bytes.Buffer
			if err := manifold.Write(ctx, &sfSlides, canonical, manifold.SF, manifold.WriteOptions{}); err != nil {
				return nil, err
			}
			if err := manifold.Write(ctx, &pdfSlides, canonical, manifold.PDF, manifold.WriteOptions{AllowLossy: true}); err != nil {
				return nil, err
			}
			conversion("pptx-central-sf", "Visible 10-slide PPTX through root Read and serialized central SF, including native preservation parts", data, manifold.PPTX, manifold.SF)
			conversion("sf-slides-pptx", "Parse central slide SF and serialize PPTX, preserving native parts", sfSlides.Bytes(), manifold.SF, manifold.PPTX)
			conversion("sf-slides-pdf", "Parse central slide SF and generate ten PDF pages with searchable/vector text and images", sfSlides.Bytes(), manifold.SF, manifold.PDF)
			conversion("pdf-slides-pptx", "Recover positioned content from ten-page PDF and create PPTX through root Read/Write", pdfSlides.Bytes(), manifold.PDF, manifold.PPTX)
			var sequence atomic.Uint64
			all = append(all, workload{Name: "pptx-varied-edit-render", InputBytes: len(data), InputSHA256: digest(data), Description: "10-slide visible deck; uniquely vary title every request, apply native edit, render slide 1 at 144 DPI; probes high-cardinality shaping-cache retention", prepare: func(worker int) operation {
				spec := filepath.Join(dir, fmt.Sprintf("vary-spec-%d.json", worker))
				edited := filepath.Join(dir, fmt.Sprintf("vary-deck-%d.pptx", worker))
				output := filepath.Join(dir, fmt.Sprintf("vary-render-%d.png", worker))
				return func(ctx context.Context) (observation, error) {
					title := fmt.Sprintf("Unique load-test title %d", sequence.Add(1))
					b, err := json.Marshal(presentation.EditSpec{Operation: "replace_text", Target: presentation.TargetSpec{Type: "object_id", ObjectID: "ppt/slides/slide1.xml#shape-2"}, Replacement: title})
					if err != nil {
						return observation{}, err
					}
					if err = os.WriteFile(spec, b, 0600); err != nil {
						return observation{}, err
					}
					edit, err := presentation.Edit(ctx, deck, spec, edited)
					if err != nil {
						return observation{}, err
					}
					if edit.Status != "ok" {
						return observation{}, fmt.Errorf("varied edit: %s", edit.Status)
					}
					rendered, err := presentation.Render(ctx, edited, presentation.RenderOptions{SlideNumber: 1, OutputPath: output, DPI: 144})
					if err != nil {
						return observation{}, err
					}
					if rendered.Status != "ok" && rendered.Status != "partial" {
						return observation{}, fmt.Errorf("varied render: %s", rendered.Status)
					}
					stat, err := os.Stat(output)
					if err != nil {
						return observation{}, err
					}
					return observation{bytes: stat.Size(), partial: rendered.Status == "partial"}, nil
				}
			}})
			spec := filepath.Join(dir, "edit.json")
			if err := os.WriteFile(spec, []byte(`{"operation":"replace_text","target":{"type":"object_id","object_id":"ppt/slides/slide1.xml#shape-2"},"replacement":"Edited load-test title"}`), 0600); err != nil {
				return nil, err
			}
			all = append(all, workload{Name: "pptx-10-edit", Description: "Synthetic 10-slide deck; plan, replace text, write PPTX, validate and verify edit", InputBytes: len(data), InputSHA256: digest(data), prepare: func(worker int) operation {
				output := filepath.Join(dir, fmt.Sprintf("edit-worker-%d.pptx", worker))
				return func(ctx context.Context) (observation, error) {
					r, err := presentation.Edit(ctx, deck, spec, output)
					if err != nil {
						return observation{}, err
					}
					if r.Status != "ok" || r.Validation == nil || !r.Validation.Valid {
						return observation{}, fmt.Errorf("invalid edit status %s", r.Status)
					}
					s, err := os.Stat(output)
					if err != nil {
						return observation{}, err
					}
					return observation{bytes: s.Size()}, nil
				}
			}})
		}
		dpis := []int{144}
		if size == 10 {
			dpis = []int{72, 144}
		}
		for _, dpi := range dpis {
			all = append(all, workload{Name: fmt.Sprintf("pptx-%d-render-%d", size, dpi), Description: fmt.Sprintf("Synthetic %d-slide deck; render slide 1 (text and PNG picture) at %d DPI; partial responses counted and reported", size, dpi), InputBytes: len(data), InputSHA256: digest(data), prepare: func(worker int) operation {
				output := filepath.Join(dir, fmt.Sprintf("render-%d-%d-worker-%d.png", size, dpi, worker))
				return func(ctx context.Context) (observation, error) {
					r, err := presentation.Render(ctx, deck, presentation.RenderOptions{SlideNumber: 1, OutputPath: output, DPI: dpi})
					if err != nil {
						return observation{}, err
					}
					if r.Status != "ok" && r.Status != "partial" {
						return observation{}, fmt.Errorf("render status %s", r.Status)
					}
					s, err := os.Stat(output)
					if err != nil {
						return observation{}, err
					}
					diagnostic := ""
					if len(r.Unsupported) > 0 {
						diagnostic = r.Unsupported[0].Message
					}
					return observation{bytes: s.Size(), partial: r.Status == "partial", diagnostic: diagnostic}, nil
				}
			}})
		}
	}
	schedule := []string{"docx-small-sf", "docx-small-sf", "docx-small-sf", "pdf-small-sf", "pdf-small-sf", "sf-medium-docx", "sf-medium-pdf", "sf-medium-pdf", "pptx-central-sf", "sf-slides-pdf"}
	byName := map[string]workload{}
	for _, w := range all {
		byName[w.Name] = w
	}
	fingerprint := ""
	avgBytes := 0
	for _, name := range schedule {
		fingerprint += name + ":" + byName[name].InputSHA256 + ";"
		avgBytes += byName[name].InputBytes
	}
	all = append(all, workload{Name: "mixed-api", InputBytes: avgBytes / len(schedule), InputSHA256: digest([]byte(fingerprint)), Description: "Weighted embedded jobs: DOCX-small->SF 30%, PDF-small->SF 20%, SF-medium->DOCX 10%, SF-medium->PDF 20%, PPTX-central->SF 10%, SF-slides->PDF 10%; hash identifies ordered workload schedule", prepare: func(worker int) operation {
		ops := make([]operation, len(schedule))
		for i, name := range schedule {
			ops[i] = byName[name].prepare(worker)
		}
		n := worker % len(ops)
		return func(ctx context.Context) (observation, error) { op := ops[n]; n = (n + 1) % len(ops); return op(ctx) }
	}})
	return all, nil
}
