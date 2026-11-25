# Presentation integration verification

Source imported from artpar/puppt at
`7f8d6d0b2dab54296fde444af7f9ac6a5816a2a7`, at the user's request.
The public `presentation` package and `manifold presentation` commands share
Manifold's module, CGO-disabled binary, CI matrix and automated release pipeline.

## Scope

Inspect, Plan, Edit, Validate, Review, SlideCount and diagnostic PNG Render are
available as Go functions. The CLI retains inspect/plan/edit/validate/review/render
and version commands. Existing SF/DOCX/PDF/VDOCS paths are unchanged. PPTX/SF,
PPTX/PDF, general new-deck creation and legacy .ppt are outside this integration.

Original package-preservation, editing, validation and renderer tests were retained.
The public facade test inspects a synthetic deck, plans/applies a text edit,
validates it, verifies the source text is unchanged and decodes the generated PNG.
The CLI dispatch test checks actual JSON inspection output and namespaced help.
These are structural/textual and PNG-validity tests, not proof of visual parity.

## Local checks

- `GOWORK=off go test -timeout 20m ./...`: pass.
- `GOWORK=off go test -race -timeout 20m ./...`: pass on the final Go source.
- `GOWORK=off go vet ./...`: pass.
- `GOWORK=off go mod verify`: pass.
- `CGO_ENABLED=0 GOWORK=off go build -o bin/manifold ./cmd/manifold`: pass;
  actual root/presentation/inspect help execution passes.
- `git diff --check`: pass.
- Pinned actionlint v1.7.12 validates the workflow.

An initial imported test built image-edit JSON by concatenating a filesystem path;
Windows rejected its unescaped backslashes. It now marshals the typed EditSpec
with encoding/json. The original replacement-byte and slide-text assertions are
unchanged. Repo-relative test artifact paths moved one level with the subsystem;
the inspection golden and coverage metadata are unchanged.

## Hosted verification

[Integration run](https://github.com/artpar/manifold/actions/runs/45583976955)
checks source `7bb3aae4fb3328929f8f97f424b48ab2d84354a8`.
[Hosted checks](https://github.com/artpar/manifold/actions/runs/45583976955)
passed on Linux, macOS and Windows, including Linux race checks and native
CGO-disabled presentation help execution. The release job passed cross-builds,
Linux CLI/version smoke execution, packaging and checksum verification.
[Manifold v0.2.0](https://github.com/artpar/manifold/releases/tag/v0.2.0) was
published automatically with six platform archives and checksums.txt. The tag
points to the checked source commit above; it is neither a draft nor prerelease.
Cross-built variants are not all executed individually.

The initial Windows run exposed an inherited test that interpolated a Windows
filename directly into JSON. The fixture now uses json.Marshal; image/text
preservation assertions are unchanged. Repository-relative path expectations were
adapted to the extra directory level. The original inspection golden bytes and
renderer coverage metadata were retained.

## Limits

The external PowerPoint pixel-oracle/real-deck corpus was ignored upstream and is
not present here. Existing optional PUPPT_* qualification tests remain opt-in and
skip without their assets/configuration. This import does not requalify the
historical real-world visual results. Unsupported content, partial rendering and
notes-creation limits remain documented in docs/presentation.

Dependencies are pinned in go.mod/go.sum. Upstream dependency, FreeType and
embedded Carlito notices are retained in THIRD_PARTY.md and third_party/licenses.
The report envelope puppt.v1 remains compatible; it describes command results,
not a new canonical document SF.
