# Status and recovery

Updated 2026-11-25. Read this, then ARCHITECTURE.md and DECISIONS.md.

Manifold is a standalone pure-Go document library and CLI, module
`github.com/artpar/manifold`, package `manifold`, command `cmd/manifold`.
The local checkout remains in the original `artpar/manydoc` directory.

## Current implementation

The v0.4.0 implementation qualifies the seven pinned huge Internet originals for
DOCX/PDF → canonical SF: **28/28 complete conversions**, including a 423 MB /
2,409-page PDF and 5,057/5,039-page manuals. Two solo runs and one two-worker batch
per source materialized 45.24 GB of SF with identical hashes across every repeat.
Four originals also complete under default limits; the other three require the
explicit large work policy. See [qualification](verification/HUGE_QUALIFICATION.md)
and [full observations](verification/HUGE_RESULTS.json), including poor ECMA
concurrency scaling, memory use and remaining fidelity boundaries.

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

Frozen Go implementation `7111871` passes the full normal suite, full race suite,
vet, CGO-disabled CLI build and diff check. Engine normal/race 20.582s/236.147s;
facade 6.039s/78.847s; CLI 0.508s/2.983s. Final original Word preservation checks
pass, as do RSS/deadline enforcement controls and independent source audits.
Clean hosted Linux/macOS/Windows plus Linux race checks pass on identical code at
[run 45621690181](https://github.com/artpar/manifold/actions/runs/45621690181).

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

## Completed pprof investigation

Twelve diagnostic conversions on three huge PDF originals, a real Word control
and qualified derived ECMA SF all retain exact output hashes. Captured 14 stage
CPU and 30 heap profiles plus four allocation deltas. See [the full investigation](verification/HUGE_PPROF.md)
and [counters/profile evidence](verification/HUGE_PPROF_RESULTS.json).

Largest targets: recursive layout copies (66–78% of Intel/ECMA read allocation),
SF key decoding / repeated decoder buffers (~991M allocations per 4.13 GB SF read),
PNG/DEFLATE churn on IPCC, repeated SF encoding and full model retention. SF-loaded
ECMA retains 5.65 GiB / 80.8M heap objects; PDF-import ECMA 4.83 GiB / 33.1M.
All observed memory runs return below 1 MiB live Go heap after document release;
that is not an RSS-return or long-lived service leak guarantee.

Caller buffering cuts writes 255× but write time only 3.5% in one pair. Do not
translate syscall samples into latency gains. Production codecs, VERSION and
frozen capacity claims are unchanged. Next implement index-based page layout
scratch, preserve exact hashes and rerun the pinned original profiles. Final
normal/race suites (production packages cached), vet, CGO-disabled build and diff
check pass; the new diagnostic buffering/error-propagation race test ran fresh.

## Performance implementation underway

Change 1 replaces recursive wide layout copies with stable page-local index
partitions and in-place permutation cycles. Existing geometry/table/list tests
and 100 deterministic comparisons against the frozen previous ordering algorithm
pass. A real ECMA conversion retains the original SF hash: read 53.88s, total
117.71s, 63.73 GiB allocated (single intermediate run, not final qualification).
Change 2 removes per-key JSON decoding for unescaped keys and redundant inner
decoders; SourceInfo uses strict shape checks plus direct typed decoding. JSON/SF
and unknown/nested-field/escaped-duplicate regression tests pass. Change 3 adds a single request-owned PNG buffer slot, bypassing large images
and releasing it before structural recovery. Byte-identity/depth/palette and
existing image/mask/glyph tests pass. Change 4 preflights representation restrictions without node construction, then
emits typed inline marks directly through streaming containers. SF/citation/math
regressions and exact-byte comparisons including lists/nested tables pass.
Change 5 detaches retained PDF names, packs run geometry independently of mutable
SourceInfo, interns bounded common SF metadata and reserves SF/navigation run
capacity. Ownership/budget and targeted source/navigation/SF regressions pass.
All five changes pass the full normal/race/vet/CGO-disabled build matrix. Fresh
engine normal/race results are 19.694s/242.273s; original image/glyph checks pass
(7.450s). The navigation bounds-cache follow-up also passes focused normal/race checks,
including interleaved-page and exact tie ordering. Earlier intermediate capacity
scouts are excluded; the final frozen snapshot is being measured afresh.

## Known boundaries and next work

- Large jobs retain a complete typed model; observed solo RSS reaches ~8.6 GiB.
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
