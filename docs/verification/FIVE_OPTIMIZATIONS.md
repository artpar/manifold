# Five performance changes: before and after

All five implementation changes are complete at **4f32d78**. Capacity qualification
is **28/28 successful original → SF jobs**, every output byte count and SHA-256
unchanged from v0.4.0. No work limits, schema, source facts or format fidelity
requirements were weakened. Diagnostic CPU/live-heap comparisons are still running.

[Complete counters and source/binary/check evidence](FIVE_OPTIMIZATIONS_RESULTS.json)
complement this report. Before: [frozen huge qualification](HUGE_QUALIFICATION.md),
implementation 7111871; [baseline pprof study](HUGE_PPROF.md).

## Solo latency and allocation

Means of two observations; allocation is cumulative per conversion, not live RAM.

| Original | Before seconds | After seconds | Throughput factor | Allocated GiB before → after | Allocation reduction |
| --- | ---: | ---: | ---: | ---: | ---: |
| IPCC AR6 | 140.60 | 124.99 | 1.12× | 129.26 → 90.40 | 30.1% |
| Intel SDM | 95.22 | 51.65 | 1.84× | 99.88 → 34.99 | 65.0% |
| ECMA-376 | 144.27 | 80.14 | 1.80× | 215.09 → 50.17 | 76.7% |
| 3GPP i00 DOCX | 3.19 | 2.93 | 1.09× | 2.56 → 2.19 | 14.7% |
| 3GPP i10 DOCX | 3.29 | 3.04 | 1.08× | 2.72 → 2.29 | 15.7% |
| ETSI 38.331 | 29.45 | 16.19 | 1.82× | 53.73 → 11.78 | 78.1% |
| C++ N4950 | 38.23 | 21.51 | 1.78× | 72.38 → 14.64 | 79.8% |

The results differ by workload. IPCC's mean read time barely changes (~111.83s
to 110.27s), while its write time drops ~28.61s to 14.45s. Intel reads ~47.03s
→ 28.36s and writes ~47.99s → 22.96s; ECMA reads ~80.80s → 51.15s and writes
~63.22s → 28.60s. Lower allocation does not translate directly into latency gains.

## Two-worker throughput

One isolated two-child batch per source; these are observed conversion jobs/s,
not HTTP RPS, sustained maximum throughput or latency percentiles.

| Original | Batch seconds before → after | Jobs/s before → after | Throughput factor |
| --- | ---: | ---: | ---: |
| IPCC AR6 | 176.00 → 137.13 | 0.01136 → 0.01458 | 1.28× |
| Intel SDM | 132.30 → 64.13 | 0.01512 → 0.03119 | 2.06× |
| ECMA-376 | 312.30 → 129.25 | 0.00640 → 0.01547 | 2.42× |
| 3GPP i00 DOCX | 3.35 → 3.14 | 0.59729 → 0.63611 | 1.06× |
| 3GPP i10 DOCX | 3.42 → 3.18 | 0.58489 → 0.62836 | 1.07× |
| ETSI 38.331 | 31.85 → 17.35 | 0.06279 → 0.11530 | 1.84× |
| C++ N4950 | 43.13 → 23.05 | 0.04638 → 0.08677 | 1.87× |

ECMA now completes two concurrent jobs in 129.25s, versus 160.27s summed across
its two new solo observations: ~1.24× the solo throughput. Previously its paired
batch took 312.30s, slower than two serial observations totaling 288.53s. Higher
concurrency and long-lived same-process service capacity remain unqualified.

## Peak RSS

Max of two solo observations and combined sampled paired RSS, in MiB.

| Original | Solo before → after | Pair before → after |
| --- | ---: | ---: |
| IPCC AR6 | 8305 → 8052 | 12014 → 12429 |
| Intel SDM | 8198 → 7996 | 14000 → 13624 |
| ECMA-376 | 8776 → 8265 | 13956 → 13299 |
| 3GPP i00 DOCX | 886 → 926 | 1813 → 1830 |
| 3GPP i10 DOCX | 1009 → 999 | 1940 → 1931 |
| ETSI 38.331 | 2956 → 2843 | 6112 → 5678 |
| C++ N4950 | 3845 → 3790 | 7790 → 7411 |

Peak RSS changes are modest, with some increases (notably IPCC's paired RSS).
The complete typed model remains large and the 8 GiB soft Go target influences
heap growth/GC. A 77% allocation reduction is not a 77% peak-memory reduction.
Forced-GC live-model measurements are separate and will be added below.

## What changed

1. **Layout ordering — a513642.** Recursively partition integer indices into a
   page-owned box array; apply stable permutation cycles once. No repeated wide
   box copies during sorting. A frozen old-algorithm oracle checks stable ties
   and column labels on 100 deterministic cases; original SF hashes stay exact.
2. **SF decoding — 3bd9f87.** Borrow validated unescaped key byte views, keep
   small duplicate-key sets on the stack, decode typed chunks directly after
   strict checks, and avoid fresh SourceInfo Decoder buffers. Only immutable
   metadata for a fixed schema type graph is globally cached.
3. **PNG encoding — d818b71.** Reuse one request-owned EncoderBuffer slot for
   raster/glyph images up to 8,192 pixels wide and 1M total pixels. Larger images
   bypass reuse; release the opaque buffer before recovery. Compression settings
   and PNG bytes remain unchanged. Upstream intermediate conversions remain.
4. **SF writing — d7a6e57.** Preflight representation restrictions without
   constructing nodes; emit typed inline marks through streamed list/table/section
   containers. Full-tree APIs retain the old encoding as a byte oracle.
5. **Retained memory/navigation — 7290f2d, 4f32d78.** Own short retained PDF names;
   pack per-run geometry independently of mutable SourceInfo; share common SF
   metadata strings under request-local limits (4,096 entries, 256 bytes/string,
   1 MiB string payload plus map overhead); reserve run capacities. Cache candidate
   bounds by page, region bounds by view, preserving global destination iteration
   and distance/tolerance/source-order ties. Release page bounds after their last use.

No new production dependency, runtime executable/service, alternate canonical
format, document upload, global document/image pool, limit relaxation or mutable
SourceInfo sharing is introduced. Source objects remain distinct and editable.

## Method and controls

Same MacBookPro18,2 / M1 Max 10 cores / 32 GiB / macOS 15.4 / Go 1.25.0 active
desktop as before. GOMAXPROCS=10; fresh isolated children, seekable file input and
fully materialized buffered file output without fsync. Source hashing warms OS
input data before each case. Original URLs/bytes/SHA-256 remain pinned in
[HUGE_CORPUS.json](HUGE_CORPUS.json); no padded/generated large input is used.

Explicit large work policy and 8 GiB soft target match the baseline. Solo RSS
guard 12 GiB / 10m deadline; pair combined guard 20 GiB / 15m. No heavy QA overlaps
the final campaign. Same sampled heap/RSS intervals and output-hash method.
Baseline data was recorded earlier on the same active machine, not randomized
interleaved trials; only two solo observations and one paired batch per case exist.

A fresh frozen-old-binary ECMA control takes **144.26s** (read 82.36s, write 61.52s),
matching the historical 143.58–144.95s range. It retains the exact original hash
and allocates ~215.09 GiB. That supports the comparison; it is not a refreshed
control for every workload or a causal attribution of each individual change.

Defaults still pass **4/7** and reject the same three: IPCC/ECMA decoded-stream
work budgets, Intel destination-name budget. These are expected bounded rejections,
not successful capacity. Large-policy success does not imply default acceptance.

Host-wide VM/swap snapshots before/after final capacity are retained in the results
JSON. They are not per-process measurements or proof of the cause of the old
ECMA concurrency slowdown. Earlier single post-run swap evidence stays scoped.

Initial layout-only and pre-navigation scouts are excluded from final capacity.
Final logs use opt-five-final-*; source is frozen at 4f32d78. Raw files, binaries,
profiles and originals remain ignored locally. Source/binary hashes identify the
actual artifacts independently of their embedded pre-commit VCS metadata.

## Correctness and checks

- Local complete normal/race suites pass at 7290f2d: engine 19.694s / 242.273s;
  facade 5.165s / 75.356s; CLI 0.491s / 2.898s.
- Final navigation follow-up passes focused normal/race checks, including
  interleaved-page and exact tie ordering. Final vet and CGO-disabled builds pass.
- Exact final source 4f32d78 passes complete hosted Windows/macOS/Linux checks
  and full Linux race at [run 45653303125](https://github.com/artpar/manifold/actions/runs/45653303125).
- Independent original image/glyph tests pass (7.450s), including 48 preserved
  unmapped glyph spans; byte-identical PNG reuse tests cover gray16/palette/alpha.
- Original Word SF/DOCX asset/equation checks pass (18.065s): both versions retain
  all 73 placements / 71 media hashes and 15/21 native equations.
- All 28 large-policy outputs and four default-policy successes match original
  qualified SF bytes/hashes; references, native math/previews and source facts stay.

VERSION remains unchanged. Automated CI publication skips new assets; no existing
release/tag is moved and no workstation release is performed.

## Diagnostic comparison pending

Separate CPU and two-GC heap campaigns are running on IPCC/Intel/ECMA, real Word
i00 and the existing 4.13 GB ECMA-derived SF artifact. The latter is derived from
the pinned publisher original, not a fabricated input. SF output uses the same
explicit 1 MiB diagnostic caller buffer as its baseline. Instrumented timings
will not replace the capacity figures above.
