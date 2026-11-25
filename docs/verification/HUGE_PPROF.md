# Huge-original pprof investigation

Work in progress, 2026-11-25. Production codec implementation remains v0.4.0
(`7111871`); this investigation changes development diagnostics only.

## Method

M1 Max, 10 cores, 32 GiB, macOS 15.4, Go 1.25.0; active desktop. Three pinned
unmodified publisher PDFs: IPCC AR6 WGI (423 MB / 2,409 pages), Intel SDM
(25 MB / 5,057 pages), ECMA-376 Part 1 (35 MB / 5,039 pages). Source hashes
and URLs remain in [HUGE_CORPUS.json](HUGE_CORPUS.json). Use cold isolated child
processes, seekable file input, buffered file output without fsync, full output
SHA-256, explicit large work policy, 8 GiB soft Go memory limit, 12 GiB sampled
RSS guard and 15-minute hard timeout. Sources are read once for hash verification
before each case, warming the OS input cache. No production subprocess is added.

CPU and heap runs are separate. CPU profiles sample each read/write stage at the
Go default 100 Hz. Heap sampling is 512 KiB; natural stage-end snapshots differ
from explicit live snapshots after two GC cycles with `runtime.KeepAlive(d)`.
A final two-GC snapshot follows dropping document/input/output-buffer references.
[Go heap profile semantics](https://pkg.go.dev/runtime/pprof) distinguish cumulative
allocations from live objects; [runtime profiling source](https://go.dev/src/runtime/mprof.go)
explains the GC lag. These instrumented timings are diagnostics, **not replacement
capacity figures** for [the frozen qualification](HUGE_QUALIFICATION.md).

CPU seconds sum samples across goroutines/cores and can exceed wall seconds.
Cumulative call-path percentages overlap; do not add them or convert them into
latency speedup estimates. Background GC includes idle workers. Sampled heap
bytes differ from exact MemStats and RSS. Post-release live heap does not imply
all reserved pages have returned to the OS.

## Initial findings

- IPCC read: 158.85 sampled CPU seconds / 114.94 profile wall seconds.
  `runtime.gcDrain` 32.98%; imageXObject 29.30%; PNG Encode 17.58%.
- Intel read: 77.89 sampled CPU seconds / 47.84 profile wall seconds.
  `runtime.gcDrain` 40.49%; inferStructure 14.80%; bindPDFNavigation 12.58%.
  Candidate quad bounds at pdf_navigation.go:1073 account for 7.42 sampled CPU
  seconds within navigation's 9.80 seconds. The loop is page-indexed but still
  scans every candidate for every destination on that page.
- ECMA read: 233.57 sampled CPU seconds / 83.15 profile wall seconds.
  `runtime.gcDrain` 60.71%; layoutOrder 5.03%; cumulative read allocation
  211,435,714,160 bytes (196.92 GiB), 202,412,321 objects, 137 GC cycles.
- SF writes: underlying file-write CPU paths cover 48.86% IPCC, 52.64% Intel,
  46.40% ECMA. The serializer uses a default 4 KiB bufio.Writer. A diagnostic
  counter and caller-buffer comparison will test attribution, preserving hashes.

## Allocation and live-heap evidence (initial)

IPCC allocates 119.476 GiB during read, retains 2,727 MiB after two GC cycles,
and drops to 0.70 MiB after document release. The sampled heap attributes 61.89 GiB
(51.72%) of allocation to flate.NewWriter, 65.84 GiB / 55.03% inclusive to PNG
encoding. Image extraction at pdf_images.go:168 accounts for 37.65 GiB and the
final PNG encode at :328 another 34.10 GiB. A bounded request-owned encoder buffer
pool is a concrete candidate; avoiding intermediate encoded-image conversion
requires preserving all color/Decode/mask semantics.

Intel layoutOrder accounts for 56.36 GiB / 65.79% of sampled read allocations;
ECMA accounts for 152.81 GiB / 77.57%. The recursive routine copies wide
pdfLayoutBox slices for x/y partitions (pdf_inference.go:225,234), while
sortPDFLayoutBoxes allocates a full ordered slice at :319. Its existing index
sort avoids expensive swaps but still copies the full payload each call.
Use stable permutations of lightweight indices into page-owned immutable boxes,
with bounded reusable scratch, and preserve inference ordering/hash expectations.

Intel retains 3,649.65 MiB after read; sourceFor owns 1,772.58 sampled MiB (48.95%),
including SourceInfo at pdf_read.go:101 and separate Quad/Baseline/Matrix objects
at :87-89. Navigation insertion retains 571.75 MiB, including 482.69 MiB in
rebuilt Run slices at pdf_navigation.go:1116. After document release exact live
heap falls to 0.69 MiB. ECMA's sampled live graph is 4,902.51 MiB, dominated by
sourceFor (2,793.93 MiB / 56.99%) and joinPDFLayoutRuns (1,522.39 MiB / 31.05%).

IPCC also retains 791.23 MiB allocated by content's string(data) at pdf_read.go:1000.
Source inspection exposes a concrete retention route: unescaped names returned
by the pinned pdfcpu parser remain substrings of that input; BMC/BDC names flow
through beginMarked and applyMarked into SourceInfo.Tag. These short retained
strings can pin full decoded content streams. A targeted ownership-boundary
clone/intern A/B is needed to quantify each field's contribution; pprof identifies
the retained allocation site, not every owning pointer.

SF-reader coverage and buffer controls are pending.

