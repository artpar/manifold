// Package presentation inspects, edits, validates and renders editable PPTX decks.
// It preserves the Puppt v1 result schema. Slide geometry is independent of the
// flowing document SF model; PPTX/SF conversion is not implemented.
package presentation

import (
	"context"
	"github.com/artpar/manifold/internal/presentation/edit"
	"github.com/artpar/manifold/internal/presentation/inspect"
	"github.com/artpar/manifold/internal/presentation/model"
	"github.com/artpar/manifold/internal/presentation/render"
	"github.com/artpar/manifold/internal/presentation/review"
	"github.com/artpar/manifold/internal/presentation/validate"
)

const SchemaVersion = model.SchemaVersion

type EditSpec = model.EditSpec
type TargetSpec = model.TargetSpec
type EditPlan = model.EditPlan
type TargetMatch = model.TargetMatch
type CommandResult = model.CommandResult
type Summary = model.Summary
type Warning = model.Warning
type ErrorItem = model.ErrorItem
type ChangeItem = model.ChangeItem
type SkipItem = model.SkipItem
type Validation = model.Validation
type RenderResult = model.Render
type Inspection = model.Inspection
type Metadata = model.Metadata
type Slide = model.Slide
type TextBlock = model.TextBlock
type MediaRef = model.MediaRef
type RepeatedText = model.RepeatedText

// RenderOptions selects a slide, destination PNG path and resolution.
// SlideNumber is 1-based; DPI defaults to 72.
type RenderOptions struct {
	SlideNumber int
	OutputPath  string
	DPI         int
}

// Inspect returns slide text, notes, media relationships and package facts.
func Inspect(ctx context.Context, path string) (*CommandResult, error) {
	return inspect.Inspect(ctx, path)
}

// Plan resolves a JSON edit specification without writing the deck.
func Plan(ctx context.Context, path, specPath string) (*CommandResult, error) {
	return edit.Plan(ctx, path, specPath)
}

// Edit applies a supported JSON edit specification and validates the result.
// Check the returned Status and Validation as well as the error.
func Edit(ctx context.Context, path, specPath, outputPath string) (*CommandResult, error) {
	return edit.Apply(ctx, path, specPath, outputPath)
}

// Validate checks core package structure and relationship reachability.
func Validate(ctx context.Context, path string) (*CommandResult, error) {
	return validate.Validate(ctx, path)
}

// Review combines inspection, validation and a prior JSON change report.
func Review(ctx context.Context, path, changesPath string) (*CommandResult, error) {
	return review.Review(ctx, path, changesPath)
}

// SlideCount returns the number of slides in presentation order.
func SlideCount(ctx context.Context, path string) (int, error) {
	return render.SlideCount(ctx, path)
}

// Render writes a diagnostic PNG. Unsupported visible content is reported in
// the returned result; rendering does not imply PowerPoint visual equivalence.
func Render(ctx context.Context, path string, opts RenderOptions) (CommandResult, error) {
	return render.Render(ctx, path, render.Options{SlideNumber: opts.SlideNumber, OutputPath: opts.OutputPath, DPI: opts.DPI})
}
