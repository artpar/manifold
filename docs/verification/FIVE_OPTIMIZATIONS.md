# Five performance changes: before and after

All five implementation changes are complete at **4f32d78**. Capacity qualification
is **28/28 successful original → SF jobs**, every output byte count and SHA-256
unchanged from v0.4.0. No work limits, schema, source facts or format fidelity
requirements were weakened. All 10 diagnostic jobs also pass with unchanged hashes.

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
Forced-GC live-model measurements are separate and appear below.

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

## CPU and allocation-site comparison

Ten final CPU/memory jobs pass with original hashes. Captured 10 stage CPU and
30 heap profiles, plus five derived write-allocation deltas. Raw profiles stay
under output/pprof-opt-five; artifact digests and sampled function summaries are
in the results JSON. CPU and memory runs remain separate. Heap sampling is 512 KiB,
with natural stage ends and explicit two-GC live/released checkpoints. These
instrumented timings are not replacements for capacity numbers above.

| Read CPU profile | Sampled CPU seconds before → after | gcDrain sampled seconds before → after |
| --- | ---: | ---: |
| IPCC | 158.85 → 135.00 | 52.39 → 35.47 |
| Intel | 77.89 → 38.18 | 31.54 → 11.09 |
| ECMA | 233.57 → 75.88 | 141.81 → 25.52 |
| ECMA-derived SF | 188.89 → 139.01 | 35.27 → 10.48 |

Cumulative functions overlap; idle/background GC is included. Darwin sample
limitations from [the baseline study](HUGE_PPROF.md) remain. Samples do not equal
wall-time shares, kernel CPU accounting or predicted speedups. Fresh ECMA control
OS user+system CPU is **360.918s**; optimized solo observations are 122.370s and
122.074s (~66% less CPU). This independent whole-process counter supports the
CPU reduction for that workload.

- **Layout:** Intel layoutOrder drops 6.72s → 1.65s inclusive sampled CPU; ECMA
  11.75s → 4.10s. The earlier 56.36/152.81 GiB recursive layout-copy allocations
  disappear as dominant sites; recursive work now uses integer index slices.
  Inference/table/run construction remains substantial.
- **Navigation:** Intel binding drops 9.80s → 1.46s; whole quadBounds inclusive
  samples drop 7.83s → 0.80s. Global destination/tie order and SF hash remain exact.
- **PNG:** IPCC total PNG Encoder CPU stays ~27–28s; pooling reduces compressor
  allocation, not the underlying compression work. Fresh flate state remains
  significant through upstream image conversion. IPCC read allocation drops
  119.476 → 86.027 GiB, while read latency changes little.
- **Writing:** exact write-phase allocation drops IPCC 9.758 → 4.379 GiB, Intel
  14.050 → 5.799 GiB, ECMA 18.170 → 6.955 GiB. Typed emission removes repeated
  node/RawMessage/source-attribute materialization; preflight still rejects before
  output. The source figures differ slightly from CPU-run counters, as expected.

## SF reader and retained model

The existing 4,132,810,817-byte ECMA SF artifact comes from the pinned original,
not a generated dummy. CPU and heap runs both reproduce its exact hash. As in the
baseline, these SF diagnostic runs use an explicit 1 MiB caller buffer for output.

| SF diagnostic observation | Before | After |
| --- | ---: | ---: |
| Read seconds (CPU run) | 167.198 | 142.387 |
| Write seconds (CPU run) | 62.941 | 28.370 |
| Read allocated GiB (memory run) | 95.464 | 31.380 |
| Read allocations (memory run) | 990,930,129 | 356,785,400 |
| Read GC cycles (memory run) | 112 | 44 |
| Write allocated GiB (memory run) | 18.174 | 6.954 |

Read allocation drops ~67%, allocation count ~64%; read latency improves ~15%.
The strict parser's per-key Unmarshal is no longer a dominant allocation site.
Sampled Decoder.refill falls ~26.59 → 5.51 GiB; direct Unmarshal allocation
~29.05 → 1.49 GiB. Remaining RawMessage copies stay ~8 GiB and are now a principal
allocation site, along with remaining attrs decoders and typed tree construction.
The implementation reduces passes/allocations; it is not a fully fused zero-copy
SF decoder or a constant-memory document API.

Exact MemStats after two GC cycles with the complete model explicitly alive:

| Input | Live MiB before → after | Live heap objects before → after | Released live MiB after |
| --- | ---: | ---: | ---: |
| IPCC PDF | 2,727.15 → 1,928.20 | 13,037,477 → 9,975,480 | 0.698 |
| Intel PDF | 3,649.65 → 3,520.17 | 25,500,773 → 18,590,624 | 0.688 |
| ECMA PDF | 4,946.93 → 4,939.83 | 33,143,805 → 22,878,117 | 0.690 |
| Word i00 DOCX | 100.80 → 100.81 | 339,925 → 339,940 | 0.691 |
| ECMA-derived SF | 5,781.23 → 4,678.31 | 80,763,533 → 38,562,359 | 0.697 |

IPCC live bytes fall ~29%; its old ~791 MiB live content-string allocation no
longer appears among significant retained sites after names become owned. SF
live bytes fall ~19% and object count ~52%. ECMA PDF live bytes barely change,
while objects fall ~31%: packing geometry mainly reduces object/GC overhead, not
its numerical payload. Word import model size is unchanged.

All five observed released heaps remain under 1 MiB, but this does not establish
RSS return, arbitrary-file bounds or unlimited same-process service behavior.
Full typed models and provenance remain multi-GiB. Changing that floor further
requires carefully designed representation/lifetime work, not dropping facts.

## Remaining bottlenecks and reproduction

The remaining measured targets are upstream image encode/decode/compression,
PDF table/run construction, SF RawMessage copies and remaining attrs decoders.
No higher-concurrency, HTTP, huge-PPTX or universal visual-fidelity claim follows
from this campaign. Individual changes were not independently isolated except
the explicitly marked layout scout; report combined gains, not additive speedups.

The source snapshot is 4f32d78; later commits update reports/KT only. Binary
hashes/build metadata are recorded because the binary was compiled before the
checkpoint commit and embeds the preceding revision plus modified=true.

```sh
CGO_ENABLED=0 GOWORK=off go build -o bin/manifold-opt ./tools/hugetest
bin/manifold-opt -large -memory-mib 8192 -rss-mib 12288 \
  -storage file -repeat 2 -timeout 10m
bin/manifold-opt -large -memory-mib 8192 -rss-mib 20480 \
  -storage file -repeat 1 -workers 2 -timeout 15m
go tool pprof -top -sample_index=alloc_space \
  output/pprof-opt-five/memory/ecma376-part1-r1-w0/read_live.heap.pprof
go tool pprof -top -sample_index=inuse_space \
  output/pprof-opt-five/sf-memory/ecma-derived-sf-r1-w0/read_live.heap.pprof
```

[Profiling protocol](../TESTING.md#huge-original-cpu-and-heap-diagnosis) covers
fresh profile roots, stage allocation deltas and exact caller-buffer settings.
Raw capacity logs: output/opt-five-final-{solo,pair,default}.jsonl; the old control
is output/opt-five-baseline-control.jsonl. All source bytes remain untracked.
