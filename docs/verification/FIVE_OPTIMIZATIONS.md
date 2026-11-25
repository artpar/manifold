# Five performance changes: implementation and comparison

Implementation checkpoint: 7290f2d plus navigation bounds follow-up. Measurement in progress; do not read intermediate
results as final capacity. Baseline is v0.4.0 implementation 7111871, using
[huge qualification](HUGE_QUALIFICATION.md) and [pprof evidence](HUGE_PPROF.md).

## Implemented changes

1. a513642: page-local layout index partitions and cycle permutations; preserve
   the frozen algorithm's stable ties, geometric decisions and column labels.
2. 3bd9f87: borrowed unescaped JSON key views, stack-local duplicate checks,
   direct typed body decoding and strict SourceInfo decoding without fresh
   Decoder buffers. Unknown/Unicode/null/casing/reference checks remain.
3. d818b71: bounded, request-owned PNG encoder buffer reuse for raster/glyph
   images, released before structural recovery; unchanged compression and bytes.
4. d7a6e57: allocation-light complete preflight and typed inline emission through
   streamed tables/lists/sections; public full-tree encoder remains the byte oracle.
5. 7290f2d: owned short PDF names, independently packed run geometry, bounded
   immutable metadata sharing during SF import, exact SF/navigation run capacity, plus lazy candidate/region bounds reuse with
   unchanged global destination iteration and source-order ties.

No production dependency, alternate canonical format, global document/image cache,
work-limit relaxation, source-fact omission or mutable-source sharing is added.
SourceInfo stays separate from packed geometry so tagged clones do not retain an
obsolete SourceInfo record through an interior geometry pointer.

## Verification protocol

Use this same M1 Max / 32 GiB active desktop, Go 1.25.0, seekable file input and
fully materialized buffered file output without fsync. Original URLs/bytes/SHA-256
remain pinned in HUGE_CORPUS.json. Each case hashes source input before children
start, warming OS file data; children are fresh processes. Explicit large policy,
8 GiB soft target, 12 GiB solo / 20 GiB paired sampled RSS guards, and 10/15-minute
deadlines match the qualification. Run two solo observations and one two-worker
batch per original. No other heavy QA runs during measurements.

Repeat CPU and heap campaigns separately for IPCC, Intel, ECMA, the existing
4.13 GB derived ECMA SF artifact, and a real Word control. Heap snapshots include
forced two-GC live/released checkpoints; diagnostic timing is not capacity.
Original/SF output hashes must equal the frozen baseline, not newly accepted oracles.
Compare exact MemStats counters and sampled allocation sites separately from
RSS/latency. Record source/binary/check metadata and preserve raw evidence locally.

## Intermediate evidence

Layout-only ECMA run: read 53.881s, total 117.710s, allocation 65,261.35 MiB
(63.732 GiB), exact qualified SF SHA-256 unchanged. Earlier full conversion
allocation was ~215 GiB. This scout overlapped small development checks and is
not a substitute for the frozen final campaign.

Full normal suite passes fresh (engine 19.694s, facade 5.165s, CLI 0.491s).
Full race suite passes fresh: engine 242.273s, facade 75.356s, CLI 2.898s.
Vet, CGO-disabled CLI/harness builds and diff checks pass. Independent original
image/glyph checks pass in 7.450s, including 48 preserved unmapped glyph spans.
Final capacity/profiling measurements are underway.

First frozen IPCC solo observation retains the original SF hash: read 110.32s,
write 14.85s, total 125.20s, allocation 90.40 GiB. Read latency is nearly unchanged
from baseline despite lower allocation; do not equate allocation reduction with
latency reduction. Two-run/pair/profile qualification is still pending.

The initial 7290f2d capacity scout was stopped to include the promised navigation
bounds follow-up. Its records remain intermediate evidence only. The final
qualification uses fresh opt-five-final-* logs and a separately frozen binary.
