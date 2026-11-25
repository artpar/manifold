package presentation_test

import (
	"context"
	"encoding/json"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/artpar/manifold/internal/presentation/fixtures"
	"github.com/artpar/manifold/presentation"
)

func TestPublicDeckWorkflow(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	input, output := filepath.Join(dir, "input.pptx"), filepath.Join(dir, "output.pptx")
	if err := fixtures.WritePPTX(input, fixtures.PPTXOptions{Slides: []fixtures.Slide{{PartName: "ppt/slides/slide1.xml", Text: "Original title"}}}); err != nil {
		t.Fatal(err)
	}
	result, err := presentation.Inspect(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if result.SchemaVersion != presentation.SchemaVersion || result.Inspection.SlideCount != 1 {
		t.Fatalf("inspection: %+v", result)
	}
	spec := presentation.EditSpec{Operation: "replace_text", Target: presentation.TargetSpec{Type: "object_id", ObjectID: result.Inspection.Slides[0].VisibleText[0].ObjectID}, Replacement: "Updated title"}
	data, err := json.Marshal(spec)
	if err != nil {
		t.Fatal(err)
	}
	specPath := filepath.Join(dir, "edit.json")
	if err := os.WriteFile(specPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	plan, err := presentation.Plan(ctx, input, specPath)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Status != "ok" {
		t.Fatalf("plan: %+v", plan)
	}
	edited, err := presentation.Edit(ctx, input, specPath, output)
	if err != nil {
		t.Fatal(err)
	}
	if edited.Status != "ok" || edited.Validation == nil || !edited.Validation.Valid {
		t.Fatalf("edit: %+v", edited)
	}
	after, err := presentation.Inspect(ctx, output)
	if err != nil {
		t.Fatal(err)
	}
	if after.Inspection.Slides[0].VisibleText[0].Text != "Updated title" {
		t.Fatal("edit missing")
	}
	original, err := presentation.Inspect(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if original.Inspection.Slides[0].VisibleText[0].Text != "Original title" {
		t.Fatal("source changed")
	}
	valid, err := presentation.Validate(ctx, output)
	if err != nil || !valid.Validation.Valid {
		t.Fatalf("validate: %+v, %v", valid, err)
	}
	count, err := presentation.SlideCount(ctx, output)
	if err != nil || count != 1 {
		t.Fatalf("count: %d, %v", count, err)
	}
	pngPath := filepath.Join(dir, "slide.png")
	rendered, err := presentation.Render(ctx, output, presentation.RenderOptions{SlideNumber: 1, OutputPath: pngPath})
	if err != nil {
		t.Fatal(err)
	}
	if rendered.Status == "error" {
		t.Fatalf("render: %+v", rendered)
	}
	f, err := os.Open(pngPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	cfg, err := png.DecodeConfig(f)
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 {
		t.Fatalf("PNG: %+v %v", cfg, err)
	}
}
