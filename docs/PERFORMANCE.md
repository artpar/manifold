# Load and stress testing

## Plan and scope

Baseline: the user's MacBook Pro (MacBookPro18,2), Apple M1 Max, 10 CPU cores,
32 GiB RAM, Darwin arm64, Go 1.25.0. This is an active desktop, not an isolated
production server. Record background CPU contention and run-to-run variation.
The test process uses GOMAXPROCS=10 and a 4096 MiB Go soft memory limit; that
limit is not an RSS cap. Race-instrumented executions are correctness checks,
never throughput results.

Use `tools/loadtest`, an independent Go development command calling public APIs:

1. Scout small/large real DOCX/PDF inputs, mixed-content SF conversions, and
   10/100-slide synthetic PPTX inspection, edits and PNG rendering at 72/144 DPI.
2. Run repeated concurrency sweeps through saturation; report successful jobs/s,
   p50/p95/p99 latency, completed/error/partial counts, allocations, GC and heap.
3. Hold selected workloads under sustained load and overdrive worker counts.
4. Apply open-loop arrival rates below/near/above measured capacity with a bounded
   queue; report rejects and end-to-end latency including queue/scheduler delay.
5. Collect separate CPU and heap/allocation profiles for identified expensive paths.
6. Exercise concurrent operations with the race detector, independently of timing.

A job includes input parsing and the named operation/output serialization.
Document input bytes are cached in RAM; PPTX APIs include file reads/writes on a
warm local filesystem. PPTX inspect returns its native object result without JSON
encoding. Conversion output is materialized, not sent to io.Discard. Each request
owns its mutable document/output. Input fixture hashes/sizes appear in raw output.
Original source documents and expectations remain unchanged. Synthetic fixtures
are reproducible and temporary; they do not establish real-deck visual fidelity.

One untimed warmup per phase populates process caches. Closed-loop workers submit
as soon as their previous job completes; this measures saturated API capacity,
not HTTP service capacity. Open-loop arrivals use scheduled absolute timestamps,
a queue of twice the worker count and immediate rejection when full. Queue delay
and scheduler lateness are included; no hidden unbounded backlog is allowed.
In-flight operations drain before throughput is calculated, and drain time is
reported. Memory is sampled every 20ms; short spikes can be missed. Output latency
percentiles use fixed logarithmic buckets with at most 2.19% upper-bound error.
Fast/full-GC instrumentation and the harness have some overhead; no correction is
subtracted. Forced GCs occur outside each timed interval for comparable starting
state and retained-heap checks; ordinary allocation/GC inside the interval counts.

No HTTP server, network/storage upload, authentication, durable queue or CLI
process startup is included. Jobs/s can be read as embedded operation TPS; API
capacity is not a measured end-to-end HTTP RPS or production latency guarantee.
Partial renderer responses are counted and explicitly reported separately from
fully supported results. Conversion diagnostics are explicitly acknowledged with
AllowLossy; throughput does not imply lossless or visual-equivalent conversion.

## Reproduction

Run from the repository root. Build once, outside measurement:

```sh
GOWORK=off go test ./tools/loadtest
CGO_ENABLED=0 GOWORK=off go build -o bin/manifold-loadtest ./tools/loadtest
bin/manifold-loadtest -list
bin/manifold-loadtest -duration 3s -repeat 3 -concurrency 1,2,4,8,16 > output/loadtest-matrix.jsonl
```

`-workloads` selects comma-separated names; `-rate` enables open-loop jobs/s;
`-procs`/`-memory-mib` set the process limits. `-cpuprofile` and `-memprofile` collect
separate profiling runs. Use `go tool pprof -top` and `-alloc_space` to inspect them.
Generated raw JSONL, temporary documents, binaries and profiles stay untracked in
output/ or bin/. Curated measured results and limitations belong in this document.

## Pipeline completion and preliminary memory finding

Capacity qualification paused while PPTX was connected to the central SF API.
The final matrix includes PPTX→SF, slide-SF→PPTX/PDF and PDF→PPTX. Slide fixtures
now contain explicit visible frames rather than minimal native test skeletons.
A mixed workload specifies its weights in the raw metadata; a varied-title edit/
render workload probes cache growth under distinct document content.

The preliminary 4-worker PDF profiling run found process-global font retention in
the old textlayout shaping path. After migration to request-owned go-text buffers,
three 8-second phases retained about 3.0/3.2/3.4 MiB post-GC instead of about
146/292/447 MiB. Existing shaping expectations were preserved. These were separate
profiled diagnostic runs, not final uninstrumented capacity results. The native
renderer result cache is now bounded to 4096 entries, with oversized keys excluded.

## Measured results — 2026-11-25

Production source: `45bebca02251d2b6d8de5edbd744b6fcb0d66837` (v0.3.0).
Harness matrix source: the same commit; `4db6a63` adds only the original large-PDF
workload. The tracked source was clean at build; environment metadata reports
modified=true because the unrelated, untracked local `.idea/` directory exists.
Baseline OS: macOS 15.4; M1 Max has eight performance and two efficiency cores.
Default GOGC=100 unless explicitly identified below. No other load-test process
ran concurrently; this was still an active desktop with background contention.

Across 260 unprofiled phases: **801,104 successful jobs, zero conversion errors,
zero partial responses**, and 1,251 intentional queue rejections in overload tests.
Profile runs are excluded from those totals and from capacity tables. Tests are
finite, workload-specific evidence, not proof of unlimited-duration stability.

### Conversion capacity

The main matrix used 16 workloads × 1/4/10/20 workers × three repetitions, two
seconds of admission per phase plus drain: 378,726 successes across 192 phases.
The table reports **10-worker median**, full three-run throughput range, and
median p95; it does not select the fastest concurrency or repetition.

| Embedded operation / input | Jobs/s median (range) | p95 ms | Allocated MiB/job |
| --- | ---: | ---: | ---: |
| Small real DOCX → SF | 475 (454–522) | 29.5 | 6.60 |
| One-page real PDF → SF | 8,049 (4,357–9,020) | 2.8 | 0.62 |
| Medium flow SF → DOCX | 1,746 (1,466–2,103) | 9.6 | 2.82 |
| Medium flow SF → PDF | 27.0 (23.2–28.0) | 396 | 261.70 |
| Medium DOCX → PDF | 26.5 (26.4–28.4) | 396 | 262.41 |
| Medium SF → VDOCS | 638 (543–775) | 23.2 | 5.88 |
| Medium VDOCS → SF | 467 (404–542) | 30.1 | 8.96 |
| 10-slide PPTX → SF | 2,041 (1,629–2,628) | 8.8 | 1.47 |
| 10-slide SF → PPTX | 1,709 (1,514–1,973) | 10.4 | 1.34 |
| 10-slide SF → vector PDF | 300 (224–314) | 42.6 | 35.16 |
| 10-page slide PDF → PPTX | 318 (267–337) | 41.7 | 12.68 |
| Native 10-slide PPTX inspect | 3,427 (2,594–4,297) | 4.8 | 0.56 |
| Native 100-slide PPTX inspect | 464 (415–518) | 31.4 | 5.21 |
| Native 10-slide PPTX text edit/verify | 1,006 (934–1,047) | 14.4 | 1.68 |
| Render **one slide** from 10-slide PPTX, 72 DPI | 896 (833–946) | 17.9 | 3.20 |
| Render **one slide** from 10-slide PPTX, 144 DPI | 279 (268–309) | 49.6 | 9.16 |

The medium flow fixture has 100 paragraphs, ten headings and five 6×2 tables;
its SF is 56,438 bytes. Slide fixtures have positioned text and a PNG on every
third slide. Rendering numbers are per selected slide, never whole-deck rates.
These simple synthetic decks do not represent arbitrary real-world PowerPoint
complexity. Source fixture names, bytes and hashes are emitted with every run.

**Cache/GC sensitivity:** the fast PDF figure varies materially with process
history. A separate fresh-process, 3×5s, ten-worker GOGC=100 control measured
6,431–6,497 jobs/s. GOGC=400 measured 10,003–10,394, with sampled peak heap up to
699 MiB versus 76 MiB. With GOGC=100, warming native PNG caches between repetitions
raised PDF throughput from 6,523 to 10,309/10,486 jobs/s, while post-GC heap rose
from 4 to 75 MiB. Retained caches increase the GC heap target: apparent speedup
is not solely a faster parser. The matrix ranges therefore matter; do not use
its fastest small-file result as a universal service capacity or tune GC blindly.
Main-matrix maximum process RSS was 950,042,624 bytes (906 MiB).

### Large real documents

Large DOCX and ten-page PDF used three repetitions of eight-second admission at
1/4/10/20 workers. Their ten-worker results:

| Original input | Jobs/s median | p95 ms | Allocated MiB/job |
| --- | ---: | ---: | ---: |
| MQTT DOCX, 355,947 bytes, 4,998 nonempty paragraphs, 59 tables | 11.45 | 1,006 | 296.76 |
| Copyright Office PDF, 111,322 bytes, ten pages | 20.53 | 561 | 152.28 |

At 20 workers these reached only 11.75 and 21.06 jobs/s, while p95 rose to 2,195
and 1,250 ms. Compressed file bytes alone are a poor admission-control estimate.

The original MQTT PDF is 1,928,173 bytes, **137 pages with 90 tagged tables**.
Three single-worker runs each measured one operation (~11.6 seconds; ~0.086 jobs/s).
Three four-worker batches measured four operations each (~14–15 seconds/batch;
~0.28 aggregate jobs/s). These are **15 measured jobs**, plus warmups: the one-second
admission window was shorter than one operation, so these are batch measurements,
not steady-state capacity or statistically robust percentile estimates. Each job
allocated ~3,633 MiB cumulatively; peak sampled live heap was 845 MiB alone and
3,376 MiB with four workers. Post-GC heap returned below 7 MiB. Allocation volume
is not simultaneously live memory; the 4 GiB soft limit is not a hard safety cap.

### Mixed traffic, overload and sustained runs

Mixed jobs: 30% small DOCX→SF, 20% small PDF→SF, 10% medium SF→DOCX,
20% medium SF→PDF, 10% PPTX→SF and 10% slide SF→PDF. The 137-page PDF is excluded.
Three six-second closed-loop repetitions measured:

| Workers | Median successful jobs/s | Median p95 ms |
| ---: | ---: | ---: |
| 1 | 25.76 | 170 |
| 4 | 79.91 | 216 |
| 10 | 106.99 | 405 |
| 20 | 111.21 | 828 |

Open-loop tests used ten workers, a 20-job queue, two eight-second repetitions,
then drained. Successful throughput includes drain time, hence it is slightly
below the offered rate even when every arrival succeeds.

| Offered jobs/s | Successful jobs/s median | Rejected / offered | Median end-to-end p95 ms |
| ---: | ---: | ---: | ---: |
| 50 | 49.11 | 0 / 800 | 188 |
| 100 | 98.03 | 0 / 1,600 | 249 |
| 200 | 115.36 | 1,251 / 3,200 (39.1%) | 586 |

Maximum generator lateness was under 12 ms in these runs. No conversion failed.
Rejected jobs are admission failures, not successes hidden from the denominator.
A single six-second overdrive at 40/80 workers reached 115.6/106.7 mixed jobs/s,
with p95 1.62/3.31 seconds. Medium PDF overdrive reached only 29.6/29.4 jobs/s,
with p95 2.06/4.39 seconds; 80 workers raised sampled heap to 1,643 MiB.

Two 30-second ten-worker medium-PDF phases measured 25.2/27.6 jobs/s. Alternating
30-second distinct-title native PPTX edit+144-DPI single-slide render phases
measured 223/234 jobs/s with p95 63/55 ms. Post-GC native-render heap rose from
76 to 82 MiB; a separate three-phase profile ended at ~84 MiB before the final
profile collection. This finite test does not establish perfectly flat native
cache/pool memory or leak freedom for every input.

### Bottlenecks and fixes

- **Fixed PDF font retention:** the old process-global shaping plan cache retained
  request font faces: ~146→292→447 MiB post-GC across matched profiled phases.
  Request-owned go-text buffers reduced this to ~3.0→3.2→3.4 MiB. Existing glyph,
  ligature, bidi and caret expectations remain unchanged. Allocations fell from
  ~312 to ~262 MiB/job; PDF layout allocation remains the major optimization target.
- **PDF layout:** the post-fix allocation profile attributes 39.8% cumulatively to
  shaping, 18.1% flat to font-text validation, 11.6% flat to font glyph loading,
  and 11.2% cumulatively to shaped-atom measurement. Cumulative categories overlap;
  they must not be added. Repeated validation, font loading and line measurement
  drive allocation/GC pressure. Next optimization: request-local reuse of parsed
  font metrics and validated/measured text, preserving existing layout oracles.
- **Large DOCX/SF processing:** a separate allocation profile attributes 24.1%
  cumulatively to language validation and 18.2% to XML tree parsing. SF conversion
  also repeatedly materializes JSON/model structures. Next candidates are bounded
  request-local language-validation reuse and fewer intermediate encode/decode passes.
- **137-page PDF:** the separate allocation profile attributes 36.9% cumulatively
  to SF block decoding and 17.7% to tagged-structure recovery; JSON decoder refill
  alone is 16.6% flat. Recovery and repeated SF materialization both matter, rather
  than compressed input size or decompression alone. Next candidate is reducing
  duplicate recovered-model/JSON representations while preserving tagged tables.
- **PPTX→SF:** allocation profiling attributes 19.0% cumulatively to XML parsing,
  16.0% to JSON marshaling and 10.9% flat to io.ReadAll. OPC preservation and strict
  SF validation copy/serialize native resources. Reducing duplicate representations
  needs round-trip/unknown-part tests; stripping them would violate preservation.
- **PNG rendering:** CPU samples show full-frame Display-P3 conversion and PNG/file
  work, not just text shaping. At 144 DPI a result is ~6.2 MB with the current fast
  PNG encoding; four times the pixel count lowers throughput substantially versus
  72 DPI. Writes are buffered/warm, not measured durable-disk capacity. Final native
  in-use profile is ~76.6 MiB, of which **64 MiB is a fixed color lookup table**;
  fonts, runtime and bounded shaping results account for much of the remainder.
  The shaping-result cache is bounded to 4,096 entries with oversized keys excluded.
- **GC/process history:** the control above confirms GC policy materially changes
  fast-operation throughput. Darwin CPU profiles also contain sleep/syscall/runtime
  samples; these are not all computation and should not be presented as parser CPU
  percentages. Profile throughput is excluded from the tables.

Practical baseline: start with 4–10 workers for the measured ordinary mix, a bounded
queue, and a separate low-concurrency lane for large PDFs. Around 50 offered mixed
jobs/s gives more headroom here; 100 succeeded in these short runs but is near the
observed saturation region. More workers mostly increase tail latency and memory.
A real HTTP service still needs its own workload/SLO and longer isolated-host soak;
there is no universal TPS/RPS number across document sizes and conversion routes.

### Commands and evidence

Raw local evidence is ignored under `output/loadtest/`: `matrix.jsonl`,
`large-inputs.jsonl`, `book-pdf.jsonl`, `mixed.jsonl`, `sustained.jsonl`,
`open-{50,100,200}.jsonl`, `overdrive.jsonl`, GC controls and separate CPU/heap
profiles. They are not shipped production dependencies or committed customer data.
Representative reproduction after building the harness as above:

```sh
bin/manifold-loadtest -workloads pptx-central-sf,sf-slides-pptx,sf-slides-pdf,pdf-slides-pptx -duration 2s -repeat 3 -concurrency 1,4,10,20
bin/manifold-loadtest -workloads mixed-api -duration 8s -repeat 2 -concurrency 10 -rate 100
bin/manifold-loadtest -workloads pdf-mqtt-sf -duration 1s -repeat 3 -concurrency 1,4
bin/manifold-loadtest -workloads sf-medium-pdf,pptx-varied-edit-render -duration 30s -repeat 2 -concurrency 10
bin/manifold-loadtest -workloads docx-mqtt-sf -duration 10s -repeat 1 -concurrency 4 -cpuprofile output/docx-cpu.pprof -memprofile output/docx-heap.pprof
```

The v0.3.0 production source passed hosted Linux/macOS/Windows checks and the full
Linux race suite in [Actions run 45597860193](https://github.com/artpar/manifold/actions/runs/45597860193).
[The automated v0.3.0 release](https://github.com/artpar/manifold/releases/tag/v0.3.0)
contains six platform archives and checksums. Independent original-deck/SF/PDF
qualification is recorded in [PPTX_SF_BRIDGE.md](verification/PPTX_SF_BRIDGE.md).

## Huge real-input results — 2026-11-25

The user explicitly requires original Internet-sourced documents. The earlier
synthetic huge-input scout was stopped and excluded from real-file evidence;
fixture-generation code was removed. tools/hugetest now reads the pinned source
manifest [HUGE_CORPUS.json](verification/HUGE_CORPUS.json), verifies original-file
SHA-256/byte lengths, and uses no generated or duplicated document content.
Downloaded originals and raw evidence remain ignored under output/hugetest-real/.

The cohort includes the 423 MB / 2,409-page original IPCC WGI report, 5,057-page
Intel manual, 5,039-page ECMA OOXML reference, two original 3GPP Word specifications,
and the corresponding 1,624-page ETSI V18.1.0 PDF. Page counts were independently
read with PyMuPDF 1.28.2, not inferred from file names. DOCX XML paragraph counts
are independently counted; cached Word page metadata is disclosed and not trusted
as rendered pagination. Original publisher bytes are not committed/redistributed.

Each batch launches isolated cold Go children with no warmup. Children read the
file, parse using default production options, check the PDF physical page count
and nonempty recovered content, then serialize complete buffered output with
AllowLossy=true. All diagnostics and recovered counts are recorded; successful
execution is not proof of complete semantic/visual fidelity. Parse/write times
exclude startup; batch time includes startup and checks. Output is materialized
in memory, not written to disk or io.Discard. Unix ps samples aggregate child RSS
at 200 ms; runtime heap sampling is 20 ms. These are development-tool dependencies,
not production requirements. A 120-second hard deadline and 8 GiB sampled RSS
batch guard bound testing, alongside each child's 4 GiB Go soft memory limit.
Sampling can overshoot and the soft limit is not an OS memory sandbox. Failures,
limits and killed jobs never count as successful throughput. Start at one child,
repeat twice; only attempt higher concurrency within the guarded desktop budget.

```sh
CGO_ENABLED=0 GOWORK=off go build -o bin/manifold-hugetest ./tools/hugetest
bin/manifold-hugetest -repeat 2 > output/huge-real-single.jsonl
bin/manifold-hugetest -cases ipcc-ar6-wgi -workers 2 -repeat 2 > output/huge-real-concurrent.jsonl
```

**Result: zero successful full conversions across 46 ordinary attempts** (seven
original documents × two single-process attempts + four attempts in two two-process
batches each; plus four simultaneous IPCC attempts). Two additional forced-boundary
controls were killed by the harness as expected. All ordinary jobs failed during
input interpretation, before SF serialization. These are reproducible compatibility
and resource boundaries, **not successful throughput benchmarks**. No successful
huge-file TPS/RPS is established; this corpus cannot currently be advertised as
supported. Production feature/work limits were not changed to force a pass.

Single-child wall time includes process start, file read and failed interpretation;
RSS is the largest 200-ms sample across two attempts, not an exact OS high-water mark.

| Unmodified original | Actual scale | Time to rejection (s) | Peak sampled RSS MiB | First failure |
| --- | --- | ---: | ---: | --- |
| IPCC AR6 WGI full report | 423.35 MB; 2,409 pages | 5.44–5.86 | 3,124 | Page 61: DCT/JPEG color space unsupported |
| Intel SDM revision 084 | 25.17 MB; 5,057 pages | 19.13–19.67 | 4,040 | PDF destination-name budget exceeded |
| ECMA-376 Part 1 | 35.30 MB; 5,039 pages | 8.86–8.87 | 1,468 | Page 625: soft-mask Matte unsupported |
| 3GPP 38.331 V18.0.0 DOCX | 3.89 MB; 66,185 XML paragraphs | 1.10 | 404 | Used majorBidi theme font unresolved |
| 3GPP 38.331 V18.1.0 DOCX | 4.00 MB; 68,220 XML paragraphs | 1.17–1.25 | 431 | Used majorBidi theme font unresolved |
| ETSI 138 331 V18.1.0 PDF | 7.24 MB; 1,624 pages | 0.25 | 91 | Page 41: explicit/color-key image masks unsupported |
| C++ N4950 draft | 8.04 MB; 2,134 pages | 1.24 | 683 | Page 389: unmapped ToUnicode code 37 |

MB here is decimal input bytes / 1,000,000; RSS uses MiB. Both DOCX files' cached
Pages=1 metadata is stale. The corresponding ETSI V18.1.0 PDF has 1,624 physical
pages; exact Word pagination was not rendered or claimed. PDF counts are independent
PyMuPDF 1.28.2 observations. This is an original **423 MB** test, not a padded or
resized 100 MB test; a document of exactly 100 MB has not been qualified.

Two-child batches reproduced the same feature/budget failures for every file.
Peak combined RSS reached 6,256 MiB for IPCC and **7,763 MiB for Intel**; Intel
batches took 22.95–25.67 seconds to reject both jobs. Four simultaneous IPCC jobs
all rejected in 9.84 seconds, peaking at 7,480 MiB sampled RSS. The normal 8 GiB
batch guard and 120s deadline were not reached. Higher process counts therefore
provide no supported full-conversion capacity for this corpus; rejection rate
must not be reported as successful TPS.

Boundary controls used these same originals: a 1 GiB RSS guard killed an IPCC
child in 0.23s at a 1,300 MiB sample (visible sampling overshoot), and a one-second
hard deadline killed an Intel child in 1.01s. These are **intentional harness
kills**, not engine limits or accidental OOM crashes. They prove the supervisor
can stop work but are not hard OS memory guarantees. No ordinary run crashed or
was killed; it returned an explicit parser/unsupported/budget error.

### Remaining bottlenecks and concrete next work

1. **Feature coverage blocks capacity first.** Qualified JPEG color-space and
   soft-mask/color-key handling, used DOCX theme-font resolution and font mapping
   must be addressed with original source fixtures and independent paint/text
   checks. Do not delete images, replace fonts silently or skip unsupported pages.
2. **Navigation complexity has a bounded-work ceiling.** Intel contains **279,501 named destinations** (independently counted by
   PyMuPDF; longest name 100 UTF-8 bytes), exceeding the existing 100,000-entry
   registry ceiling despite its modest compressed file size. Review
   a scalable representation while retaining names/links and bounded memory;
   merely increasing the budget is not demonstrated safe by this study.
3. **High memory precedes rejection.** ReadPDF currently buffers all source bytes;
   this harness additionally retains its caller-owned full input. Parser/model
   state and allocation/GC also contribute. That code-path observation explains
   why file size is not an RSS ceiling, but this study has not apportioned giant
   file memory with a new allocation profile. Investigate bounded/spooled source
   handling and earlier validation before increasing concurrency. The earlier
   small-file/PDF-layout profiles are not profiles of these huge originals.
4. **Requalification is still required.** After fixing each actual feature failure,
   rerun these exact hashes at one child, then guarded concurrency. Only successful
   complete conversions can establish latency/TPS or justify a sustained huge-file
   soak. Writer/load capacity remains unmeasured because every input failed first.

Use [HUGE_INPUTS.md](verification/HUGE_INPUTS.md) for publisher URLs, download and
extraction commands, independent checks and source boundaries. The manifest pins
all original bytes. Raw local evidence: output/huge-real-single.jsonl,
huge-real-etsi-single.jsonl, huge-real-cpp-single.jsonl, huge-real-concurrent.jsonl,
huge-real-cpp-concurrent.jsonl, huge-real-guard.jsonl, huge-real-guard-control.jsonl
and huge-real-deadline.jsonl. Source implementation is unchanged v0.3.0; the real
runner is source-equivalent to 4709926, with subsequent manifest/KT additions.
The harness process exit code indicates evidence collection completed; consult
child.success, child.stderr and batch.termination for conversion outcomes.
