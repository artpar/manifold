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
fragmented file organization. The repository is public at https://github.com/artpar/manifold. Main contains
the rename and qualified organization cleanup. A clean clone of source commit
249414d333e6e2179f06161feeebf4ae7e7b0b06 builds with CGO disabled and passes
SF→DOCX→SF / SF→PDF→SF with three typed notes. The root now contains the public API and public API tests; implementation
and unit tests live in cohesive modules under `internal/engine`. QA tools are
grouped by concern under `scripts/qa`. Current guides, detailed contracts and
historical evidence are separate. The engine deliberately shares one document
model; it does not claim independent codec packages that the implementation lacks.

The optional VDOCS adapter handles a pinned Univer snapshot profile. It requires
no editor service or host application. See the [adapter contract](reference/VDOCS_FORMAT.md).

## Verification

Rename checks pass: normal suite 31.281s/CLI0.564s; race suite
351.804s/CLI2.868s; vet/modules; CGO-disabled host execution and Linux amd64/arm64
cross-builds; all 51 accepted corpus documents through actual ProseMirror; the
section example; and a clean-checkout all-four-path note conversion.

Organization normal/race/vet/build checks, complete-SF corpus equality, moved
QA tools, original-source readers and clean published-clone checks pass. Exact scopes/results are recorded in
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

For future changes, run affected checks and the required matrix, commit aligned
code/docs/evidence and verify the pushed commit. No publication work remains.
Keep Daptin/application code unchanged.
For earlier milestones and old filenames, use [history](history/README.md).


## Reference cleanup

Application-specific names/source links were removed while VDOCS was retained at
user request. SF remains canonical; DOCX/PDF/native snapshot paths share its model.
VDOCS now uses `manifold-document` and Manifold-owned MIME; retired headers need
explicit migration. The native fixture's document payload is unchanged. Current
normal/race/vet/build checks, all 33 retained native SDK probes, fresh save/re-import
and complete-SF equality for all 51 accepted core fixtures pass. See
[the report](verification/REFERENCE_CLEANUP.md) and [native contract](reference/VDOCS_FORMAT.md).
Published source commit `8536dc0` passes a fresh clone/build and full tracked-reference
scan. SF→VDOCS→SF retains the complete normalized graph; native→DOCX/PDF→SF
recovers the expected typed notes. No follow-up publication work remains.


## Hosted CI/CD

`.github/workflows/ci.yml` owns checks and releases. Pull requests/main/tag pushes
run native Linux/macOS/Windows Go checks, Linux race checks and CGO-disabled smoke
builds. New VERSION values on main automatically create a gated tag/release with
six platform archives, licenses and verified checksums. Hosted runs/releases are
the current result authority; see [CI/CD](CI.md). Local qualification records below
remain historical evidence, not a substitute for the hosted result.

## Presentation integration (in progress)

Puppt source commit 7f8d6d0b2dab54296fde444af7f9ac6a5816a2a7 is integrated
under internal/presentation, with public presentation API and one namespaced CLI.
Original synthetic/structural tests and PNG renderer tests are retained. New public
API tests cover inspect/plan/edit/validate/render and unchanged source text.
Repository-relative QA paths were relocated; external pixel-oracle corpora are not
present. Combined normal tests, vet, module verification, CGO-disabled build, public API
workflow and CLI dispatch/help checks pass. Final race checks and hosted CI are
pending at this checkpoint. VERSION 0.2.0 requests publication through Actions. See presentation/README.md for scope and limitations.

Next commands: GOWORK=off go test -timeout 20m ./...;
GOWORK=off go test -race -timeout 20m ./...; GOWORK=off go vet ./...;
CGO_ENABLED=0 GOWORK=off go build -o bin/manifold ./cmd/manifold;
git diff --check. Do not claim PPTX/SF or PPTX/PDF conversion.

Hosted integration verification found an upstream test constructing JSON by
concatenating a Windows filesystem path. The test now uses encoding/json to
encode the same image-replacement specification; assertions/expected bytes are
unchanged. All production paths and imported renderer oracles remain unchanged.
Release builds now stamp the presentation version from VERSION. Hosted checks
must pass on the resulting commit before automatic v0.2.0 publication.
