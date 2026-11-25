# Status and recovery

Updated 2026-11-25. Read this, then ARCHITECTURE.md and DECISIONS.md.

Manifold is a standalone pure-Go document library and CLI. Its module is
`github.com/artpar/manifold`, package `manifold`, executable `cmd/manifold`.
The local checkout remains in the original `artpar/manydoc` directory.

## Current work

Current completion checkpoint: **v0.3.0 is released automatically** from
`45bebca`; PPTX↔SF and supported PPTX/PDF conversion use the central API/CLI.
Original-deck preservation, guarded SF edits, independent consumers and hosted
Linux/macOS/Windows plus full Linux race checks passed. Advanced PowerPoint
rendering/regeneration remains explicitly limited; see
[bridge qualification](verification/PPTX_SF_BRIDGE.md).

Load/stress qualification is complete on this M1 Max baseline: 260 unprofiled
phases, 801,104 successful jobs, zero conversion errors/partial responses, and
1,251 bounded-queue overload rejections. Ten-worker mixed throughput was ~107
jobs/s; medium flow SF→PDF ~27 jobs/s. Large documents differ dramatically.
The global PDF shaping retention was fixed and the native result cache bounded.
See [PERFORMANCE.md](PERFORMANCE.md) for ranges, input sizes, latency, memory,
profiles, remaining bottlenecks and reproduction commands. Further performance
work should target request-local font/measurement reuse and large-input admission;
no additional optimization is claimed by the measurements. Final harness checks passed: normal/race tool tests, tool vet, CGO-disabled CLI build and git diff --check. Historical checkpoints
below describe earlier states, not outstanding release work.

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

## Presentation integration

Puppt source commit 7f8d6d0b2dab54296fde444af7f9ac6a5816a2a7 is integrated
under internal/presentation, with public presentation API and one namespaced CLI.
Original synthetic/structural tests and PNG renderer tests are retained. New public
API tests cover inspect/plan/edit/validate/render and unchanged source text.
Repository-relative QA paths were relocated; external pixel-oracle corpora are not
present. Combined normal tests, vet, module verification, CGO-disabled build, public API
workflow and CLI dispatch/help checks pass. The final local race suite also passes.
Hosted Linux/macOS/Windows checks, Linux race checks and automated packaging/
publication pass for source commit 7bb3aae. Actions published v0.2.0 with all six
platform archives and checksums; the release tag points to that checked commit.
See [the verification record](verification/PRESENTATION_INTEGRATION.md) and
[presentation scope](presentation/README.md). No integration/release work remains
within that declared scope; external visual-oracle qualification remains opt-in.

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

## Performance qualification (in progress)

The user requested a plan and actual load/stress testing with bottlenecks and TPS.
The baseline is this M1 Max desktop. tools/loadtest implements public-API jobs,
closed-loop concurrency sweeps, bounded open-loop overload, latency/heap/GC metrics
and optional profiles. docs/PERFORMANCE.md records the plan, measurement semantics
and limits. Initial fixture/scout checks run; repeated matrix, soak/overload,
profiling and final capacity evidence remain pending. Production conversion code
and reference inputs/oracles are unchanged.

## SF presentation bridge (in progress)

The user rejected a PPTX-only facade as incomplete and required SF alignment.
PPTX now enters the root Read/Write API through a typed Presentation extension of
the existing SF root attributes. Slides retain positioned objects, rich SF text,
images, notes and master/layout resources; original OPC parts retain unmodeled
features. Native-package round trips, central-SF text/geometry edits, searchable
vector PDF output, PDF→PPTX and basic flow-SF→PPTX tests pass on synthetic fixtures.
Full compatibility/negative tests, original-source qualification and hosted checks
remain pending. This is not a claim of arbitrary PowerPoint rendering parity.
The performance matrix is paused until this pipeline is qualified, then will be
rerun including the new routes. A preliminary allocation profile identified
repeated font parsing retained by textlayout/harfbuzz's global face-keyed shaping
plan cache; that finding needs mitigation and a sustained recheck before capacity
claims. Existing raw stress/profile data remains ignored under output/.

SF bridge qualification update: synthetic end-to-end/negative tests pass; two
unmodified Apache POI PowerPoint packages survive SF with every original ZIP part
unchanged, and the actual ProseMirror reader checks the nested rich content.
Native text/frame patches preserve unrelated style/theme XML. Unsupported output,
ambiguous owners and overflowing frames reject; deleted notes do not reappear.
Full normal suite passes. Shaping was migrated to the already-pinned go-text
backend, whose plan cache is request-owned; existing glyph/ligature/bidi/caret
oracles pass unchanged. Sustained memory recheck and final race/hosted checks are
pending before the resumed performance matrix and final release.

Central-SF feature completion checkpoint: root API and CLI cover PPTX↔SF and
supported PPTX/PDF paths; original-source structural, SF marker/old-reader rejection,
notes-deletion, native text edit/slide addition and unsupported-output tests pass.
Actual ProseMirror accepts the extension. PPTX-specific frame/font/language rules
reject unrepresented output semantics. Native PNG/inspect/edit helpers remain.
Final regression/hosted checks and expanded capacity matrix are pending; all
throughput claims must use that final production pipeline. See the current
presentation/README.md and PERFORMANCE.md; older facade-only limits are historical.

Independent consumer checkpoint: python-pptx 1.0.2 reads source/edited/recovered
PPTX output and verifies title/frame/notes; PyMuPDF 1.28.2 verifies searchable
PDF text, one image, two vectors and 720x540pt geometry. The rendered example was
visually inspected. A standards check fixed the main presentation MIME and
notes-body placeholder. See verification/PPTX_SF_BRIDGE.md. This does not upgrade
unsupported native features to general PowerPoint fidelity.

## Huge-input follow-up (in progress)

The previous 137-page maximum does not establish 100 MB / 1,000-page capacity.
Added tools/hugetest with independent deterministic PDF/OOXML fixtures, complete
page-marker/image checks, isolated child processes, hard timeouts and sampled
aggregate RSS guard. Small generator/read checks pass. Huge-input measurements,
concurrency qualification and report updates are pending; see PERFORMANCE.md.
