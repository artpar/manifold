# Status and recovery

Updated 2026-11-25. Read this, then ARCHITECTURE.md and DECISIONS.md.

Manifold is a standalone pure-Go document library and CLI, module
`github.com/artpar/manifold`, package `manifold`, command `cmd/manifold`.
The local checkout remains in the original `artpar/manydoc` directory.

## Current implementation

The v0.4.0 baseline and completed performance changes qualify seven pinned huge Internet originals for
DOCX/PDF → canonical SF: **28/28 complete conversions**, including a 423 MB /
2,409-page PDF and 5,057/5,039-page manuals. Two solo runs and one two-worker batch
per source materialized 45.24 GB of SF with identical hashes across every repeat.
Four originals also complete under default limits; the other three require the
explicit large work policy. See [qualification](verification/HUGE_QUALIFICATION.md)
and [full observations](verification/HUGE_RESULTS.json), including poor ECMA
concurrency scaling, memory use and remaining fidelity boundaries. The completed
five-change source 4f32d78 preserves all hashes and improves mean solo latency
by 8–46% across these originals; see [before/after results](verification/FIVE_OPTIMIZATIONS.md).

Both original Word files retain 73 preview placements / 71 byte-identical WMF/EMF
assets and all 15/21 native equation structures through SF and DOCX. Independent
XML/ProseMirror audits have zero missing normalized source characters. All five
PDF-derived SF artifacts pass central Go Read/Write with identical SHA-256,
including final corrected IPCC and 4.13 GB ECMA SF. Known representation losses
remain explicit; this is not Word-equivalent rendering or arbitrary-file support.

SF remains the single canonical ProseMirror JSON profile,
`manydoc-prosemirror/1`; SF.gz is that same data compressed. DOCX, PDF, PPTX,
JSON/text and optional VDOCS use the shared typed model. Native Office Math is an
additive atomic SF node, not another interchange format. The PPTX bridge includes
root Read/Write and supported native PDF paths; see [bridge qualification](verification/PPTX_SF_BRIDGE.md).

The optional VDOCS adapter targets a pinned Univer snapshot profile and requires
no editor service/host application. See [its contract](reference/VDOCS_FORMAT.md).
Public API lives at the root; cohesive codec/model implementations are under
`internal/engine`; presentation internals are under `internal/presentation`.
No production subprocess, service, CGO or application permission gate is added.

## Verification and delivery

Final performance source `4f32d78` passes complete hosted Windows/macOS/Linux
checks plus Linux race at [run 45653303125](https://github.com/artpar/manifold/actions/runs/45653303125).
Local full normal/race checks pass on 7290f2d (engine 19.694s/242.273s), followed
by focused normal/race navigation checks on the final follow-up. Final vet and
CGO-disabled builds pass. Independent original image/glyph and Word asset/math
checks pass; all 28 capacity and 10 diagnostic outputs preserve qualified hashes.
Default acceptance stays 4/7; the other three still reject explicit work budgets.

GitHub Actions owns publication from VERSION, gating the tag, six platform
archives, licenses, checksums and release on the hosted matrix. The v0.4.0 source
uses that pipeline; [CI.md](CI.md) and the
[release page](https://github.com/artpar/manifold/releases) are delivery authorities.
Do not create/upload a workstation release manually.

Earlier small/medium load qualification remains workload-specific: 801,104
successful jobs and ~107 mixed jobs/s at ten workers do not describe these huge
originals. The historical huge baseline was 0/46 completed attempts; its facts
remain in [HUGE_INPUTS.md](verification/HUGE_INPUTS.md) and PERFORMANCE.md.
The final repaired results supersede those input blockers, not the baseline record.

## Completed performance work

The [baseline pprof investigation](verification/HUGE_PPROF.md) identified layout
copying, repeated SF JSON decoding/encoding, PNG compressor churn and provenance
retention. Those observations led to all five completed changes: stable page-local
index ordering; strict key/direct typed decoding; bounded request-owned PNG reuse;
allocation-light preflight/direct streaming SF emission; owned names, packed
geometry, bounded immutable metadata sharing, exact run capacity and navigation
bounds reuse. Public mutable source ownership and canonical SF bytes remain intact.

[The comparison report](verification/FIVE_OPTIMIZATIONS.md) and
[complete evidence](verification/FIVE_OPTIMIZATIONS_RESULTS.json) record 28/28
large-policy capacity jobs plus 10/10 diagnostic jobs. Intel solo mean drops
95.22s → 51.65s; ECMA 144.27s → 80.14s; IPCC 140.60s → 124.99s. ECMA paired
wall drops 312.30s → 129.25s. Allocations fall 30–80% across the PDFs, while peak
RSS changes remain modest and some increase. These are conversion jobs/s, not
HTTP RPS or sustained maximum service capacity.

The 4.13 GB derived ECMA SF read drops 167.20s → 142.39s and allocation
95.46 GiB → 31.38 GiB. Its live model drops 5,781 MiB / 80.8M objects to
4,678 MiB / 38.6M objects; IPCC live model 2,727 MiB → 1,928 MiB. ECMA PDF
live bytes stay near 4,940 MiB despite fewer objects. All observed released heaps
remain below 1 MiB; this is not an RSS-return or long-lived service leak guarantee.

Source is frozen at 4f32d78; subsequent report/KT/version commits do not change Go code.
VERSION is 0.4.1. [The published release](https://github.com/artpar/manifold/releases/tag/v0.4.1)
was produced by [run 45660838689](https://github.com/artpar/manifold/actions/runs/45660838689)
at bdb8ed3 after all platform/race gates passed. Its tag resolves to that commit;
all six archive checksums match GitHub asset SHA-256 digests. Existing tags stay unchanged.

## Known boundaries and next work

- Large jobs retain a complete typed model; observed optimized solo RSS still reaches ~8 GiB.
  Extra workers can reduce throughput. Use caller admission and explicit limits;
  GOMEMLIMIT is a soft target, sampled RSS is not total OS memory.
- Native OMML and WMF/EMF preserve through SF/DOCX; their PDF rendering remains
  unsupported. Math conversion through PPTX/VDOCS rejects rather than dropping it.
- General PDF vector/shading/clipping/transparency reconstruction, OCR, source
  pagination and color-managed/Word-equivalent visual fidelity are not qualified.
  Unmapped CFF glyphs may remain diagnosed span images, not editable Unicode.
- Huge HTTP/same-process service capacity, higher concurrency and whole-document
  browser editor capacity remain unmeasured. See COMPATIBILITY.md for the contract.

Concrete next commands for changes or reproduction:

```sh
GOWORK=off go test ./...
GOWORK=off go test -race -timeout 20m ./...
GOWORK=off go vet ./...
CGO_ENABLED=0 GOWORK=off go build -o bin/manifold ./cmd/manifold
CGO_ENABLED=0 GOWORK=off go build -o bin/manifold-hugetest ./tools/hugetest
bin/manifold-hugetest -large -memory-mib 8192 -rss-mib 12288 \
  -storage file -repeat 2 -timeout 10m
git diff --check
```

The huge command requires the exact downloaded originals. Acquisition is in
verification/HUGE_INPUTS.md; full protocol and independent checks are in TESTING.md.
If extending scope, qualify native math/metafile rendering, smaller provenance
retention and deployment-specific same-process admission/capacity separately.
Keep STATUS/ARCHITECTURE/DECISIONS current, dependencies/notices aligned and original
oracles intact. Keep Daptin/application code unchanged; publisher/customer binaries
and generated artifacts remain untracked. Earlier milestones are in verification
reports and [history](history/README.md).
