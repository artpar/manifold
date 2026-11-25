# Status and recovery

Updated 2026-11-25. Read this, then ARCHITECTURE.md and DECISIONS.md.

Manifold is a standalone pure-Go document library and CLI. Its module is
`github.com/artpar/manifold`, package `manifold`, executable `cmd/manifold`.
The local checkout remains in the original `artpar/manydoc` directory.

## Current work

The original rich-conversion goal is qualified: DOCX → SF, SF → DOCX, SF → PDF
and PDF → SF. The established SF is ProseMirror JSON with the compatibility
identifier `manydoc-prosemirror/1`. See [acceptance](verification/ACCEPTANCE.md) and
[the audit](verification/FINAL_AUDIT.md) for source, reader and visual evidence.

The user requested renaming/publishing as `artpar/manifold`, then flagged the
fragmented file organization. The repository is created at
https://github.com/artpar/manifold but its first push is pending the organization
checks. The root now contains the public API and public API tests; implementation
and unit tests live in cohesive modules under `internal/engine`. QA tools are
grouped by concern under `scripts/qa`. Current guides, detailed contracts and
historical evidence are separate. The engine deliberately shares one document
model; it does not claim independent codec packages that the implementation lacks.

The optional VDOCS adapter handles a Univer snapshot envelope observed in Colleaf.
No Colleaf service, application code or runtime dependency is required. Its source
provenance stays in the [adapter contract](reference/VDOCS_FORMAT.md); it is not
Manifold's product identity. Persisted metadata identifiers and QA environment
variables retain their old spelling for compatibility.

## Verification

Rename checks pass: normal suite 31.281s/CLI0.564s; race suite
351.804s/CLI2.868s; vet/modules; CGO-disabled host execution and Linux amd64/arm64
cross-builds; all 51 accepted corpus documents through actual ProseMirror; the
section example; and a clean-checkout all-four-path note conversion.

Organization normal/race/vet/build checks, complete-SF corpus equality, moved
QA tools and original-source reader checks pass. Commit/push and clean-clone
verification remain. Exact scopes/results are recorded in
[verification.json](verification.json). No source fixture or independent oracle
was rewritten. Generated output, binaries and QA environments remain ignored.

## Next commands

```sh
GOWORK=off go test ./...
GOWORK=off go test -race -timeout 20m ./...
GOWORK=off go vet ./...
CGO_ENABLED=0 GOWORK=off go build -o bin/manifold ./cmd/manifold
GOWORK=off go run ./examples/sections
output/qa-venv/bin/python scripts/qa/structured/verify_structured_corpus.py
git diff --check
```

Finish checks, commit aligned code/docs/evidence, push `main` to `origin`, and
verify the remote commit and a clean clone. Keep Daptin/application code unchanged.
For earlier milestones and old filenames, use [history](history/README.md).
