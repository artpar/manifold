# Huge-original pprof investigation

Completed 2026-11-25: **12/12 diagnostic conversions**, all output hashes matching
qualification. Captured 14 stage CPU and 30 heap profiles; four additional heap
profiles are derived write-allocation deltas. Production codec implementation
remains v0.4.0 (`7111871`); this investigation changes development diagnostics only.
[Measured counters, profile summaries and artifact hashes](HUGE_PPROF_RESULTS.json)
retain the evidence. Raw profiles/binaries stay ignored under output/ and bin/.

The largest measured problems are recursive layout copies, repeated strict SF
JSON work, PNG/DEFLATE allocation and the complete pointer-heavy model. File
buffering is a modest result despite its initially large syscall sample share.

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
latency speedup estimates. Background GC includes idle workers; the runtime
lifetime GCCPUFraction is a different metric and excludes idle mark work. Darwin profiling also has reported
[sampling anomalies](https://github.com/golang/go/issues/68477); syscall-stack
samples are not a measurement of kernel CPU seconds or disk wait. Cross-check
with exact counters, OS user/system totals and the buffer wall-time comparison.
Sampled heap bytes differ from exact MemStats and RSS. Post-release live heap does not imply
all reserved pages have returned to the OS.

## PDF CPU profiles

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
- SF writes: underlying file-write sample paths cover 48.86% IPCC, 52.64% Intel,
  46.40% ECMA. The serializer uses a default 4 KiB bufio.Writer. The controlled
  counter/buffer comparison below shows why this is not a speedup prediction.

## Allocation and live-heap evidence

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

## Stage memory summary

Exact MemStats from the memory runs, GiB unless stated otherwise:

| Original | Read allocated | Read allocations (millions) | Read GC cycles | Write allocated | Live model after read | Live heap after release (MiB) |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| IPCC | 119.476 | 582.033 | 80 | 9.758 | 2.663 | 0.698 |
| Intel | 85.828 | 110.386 | 55 | 14.050 | 3.564 | 0.688 |
| ECMA | 196.915 | 202.412 | 139 | 18.170 | 4.831 | 0.703 |

Live model sizes stay essentially unchanged after writing. These jobs show no
large document graph remaining reachable after release; they do not establish
unlimited same-process lifetime behavior. RSS and heap reservation can remain
large despite tiny live heap. MemStats allocation counts include tiny allocations
that heap-profile block counts do not count identically (Go 1.25 runtime/mstats.go);
use pprof object counts as sampled allocation-block attribution, not exact counts.

On this arm64 build Block/Run/SourceInfo are 440/224/440 bytes, Quad is 64 bytes,
Matrix 48 bytes. pdfLayoutBox contains Block plus geometry/column fields and is
512 bytes. Copying these records through recursive partitions accounts for large
byte traffic even when the final text is only ~10 MB.

## Buffer control: a hypothesis demoted by measurement

Same ECMA original, binary, work limits, source hash and SF hash; separate
cold children, one measured pair. Caller buffering only; no codec changes.

| Observation | Normal output | Caller buffer 1 MiB |
| --- | ---: | ---: |
| Underlying file writes | 1,002,226 | 3,935 |
| Writes at most 4 KiB | 1,001,357 | 0 |
| Read seconds | 83.154 | 84.000 |
| Write seconds, including flush | 63.374 | 61.175 |
| Total conversion seconds | 146.909 | 145.570 |
| Child OS user / system CPU seconds | 352.892 / 9.849 | 351.480 / 8.500 |
| Sampled write CPU seconds | 71.64 | 67.59 |
| File-write stack sample share | 49.46% | 9.63% |

Buffering cuts write calls by 254.7× but write wall time only 3.5%, total time
0.9%. One pair does not establish statistical significance. The SF hash stays
`ec7b59e7042bb47614bcc8ee10b297d3e4253c6b1dd1efb0537804ca832c3982`.
**Do not read the original ~50% syscall sample share as a potential 50% speedup.**
The buffered profile attributes 40.76% inclusive to json.Marshal, 32.68% to
encodeSFBlocks and 20.43% to gcDrain; these overlap. Buffering does not remove
the ~18.18 GiB write allocation churn.

In write-only allocation deltas (write_live minus read_live), direct json.Marshal
allocations account for 55.91% IPCC, 61.35% Intel, 62.40% ECMA; encodeSFBlocks
adds 21.76%, 27.54%, 29.95% direct allocation respectively. These flat shares
are disjoint; their inclusive shares are not. The SF stream writer encodes every
body block once during preflight and again for emission (structured_stream.go:34,52).
Each stage builds transient nodes, RawMessages and marshaled bytes. Source attrs
also pass through map/JSON decode/reencode. Replace the preflight construction
with allocation-light validation and emit typed attrs directly, while retaining
reject-before-output semantics, canonical bytes, source facts and global references.

## Central SF reader

The existing 4,132,810,817-byte ECMA SF artifact is derived from the pinned
publisher original, not an additional original or generated dummy. Input and
round-trip output both hash to the qualified ECMA SF hash. The diagnostic
manifest is `output/pprof-huge/derived-manifest.json`. Both SF profiling runs use
the 1 MiB diagnostic caller buffer for output, using identical aggregation
without changing the reader or SF bytes.

CPU run: read 167.198s, 188.89 sampled CPU seconds; 102,505,050,920 allocated bytes
(95.465 GiB), 990,930,363 allocations, 111 GC cycles. Inclusive sample shares:
json.Decoder.Decode 36.22%, validateTypedJSON 17.33%, strictSFAttrs 17.23%,
decodeSFBlocks 21.71%, gcDrain 18.67%. These overlap. Memory clearing alone
appears as 12.95% flat samples. Rewriting the same SF takes 62.941s and preserves
its complete hash.

The path repeatedly materializes/scans each body chunk: outer Decoder to
RawMessage, strict typed scan, inner decode to StructuredNode, attrs map decoding
and strict SourceInfo/typed attrs decoding, then model/global reference validation.
A direct strict typed decoder can combine these passes, but must still reject
duplicate keys, malformed Unicode, noncanonical field casing, unknown typed attrs,
invalid nulls and unresolved cross-references. Removing validation is not an
optimization acceptable under the format contract. Optimize key decoding and
repeated per-attribute decoders first; type-field
metadata caching is a smaller secondary candidate.

SF heap attribution is more specific than a generic JSON bottleneck:

- Direct json.Unmarshal allocations: 29,748.58 MiB (30.49%); the strict parser's
  per-key json.Unmarshal call at json.go:120 accounts for 29.52 GiB inclusive.
- Decoder.refill: 27,226.79 MiB (27.91%), and fresh NewDecoder objects another
  5,097.56 MiB (5.23%). Repeated attrs decoders are a substantial target.
- strictJSONParser.value direct allocations: 9,948.61 MiB (10.20%), including
  duplicate-key maps (6.66 GiB) and key variables (3.06 GiB). Its inclusive
  allocation is 41.27%, including the per-key Unmarshal work.
- RawMessage copies: 8,159.44 MiB (8.36%).
- Type-field metadata at recordFields is only 91.02 MiB cumulative here: caching
  it is not the principal fix.

The sampled SF live model is 5.65 GiB, larger than PDF-import ECMA's 4.79 sampled
GiB. SourceInfo mark records allocated at structured.go:588 account for 2.14 GiB;
Run slices at :621 another 1.53 GiB, with separately decoded strings/geometry
adding more. Byte-equivalent SF persistence does not guarantee identical allocation
sharing or slice capacity. The memory run reads in 166.013s, allocates 95.464 GiB
and retains exactly 5,781.23 MiB / 80,763,533 heap objects after two GC cycles.
After writing it retains 5,781.23 MiB; after release, 0.695 MiB / 3,491 objects.
For comparison, PDF-import ECMA retains 4,946.93 MiB / 33,143,805 objects.
Request-bounded interning of repeated immutable metadata strings is a concrete
candidate; shared mutable SourceInfo pointers would alter API behavior.

## Real Word control

The pinned 66,185-XML-paragraph 3GPP i00 DOCX also has separate CPU/heap runs.
CPU run: read 2.127s, write 0.986s; qualified SF hash unchanged. Memory run:
read allocates 1.744 GiB, write 0.822 GiB; live model 100.80 MiB and released
heap 0.683 MiB. Sampled read allocation: parseXML 526.80 MiB / 29.36% inclusive;
language.Parse 395.54 MiB / 22.04%; io.ReadAll 270.85 MiB / 15.09% direct.
Language validation repeatedly parses tags, offering a bounded request-local
validation-cache candidate. DOCX is much cheaper than these huge PDFs/SF; its
short CPU profile has fewer samples and lower ranking confidence.

## Prioritized implementation work

| Priority | Change and evidence | Required preservation check |
| --- | --- | --- |
| 1 | Replace recursive 512-byte layout copies with page-local index permutations / bounded scratch: 65.79–77.57% of Intel/ECMA read allocations. [Code](../../internal/engine/pdf_inference.go#L175). | Exact original SF hashes, geometry/order/table/list/column oracles; lower alloc_space and GC work. |
| 2 | Fuse strict SF decode passes; fast-path unescaped keys and avoid a fresh decoder per attrs object: ~991M allocations / 95.46 GiB per ECMA SF read. [Keys](../../internal/engine/json.go#L119), [attrs](../../internal/engine/structured.go#L417). | Existing malformed Unicode, duplicate/escaped-key aliases, casing/null/unknown attrs and EOF global-reference tests; exact 4.13 GB round trip. |
| 3 | Request-owned bounded PNG encoder buffer reuse; avoid supported intermediate encode/decode paths: PNG 55.03% of IPCC read allocation. [Code](../../internal/engine/pdf_images.go#L168). | Unchanged pixels, color/Decode/mask/high-bit-depth behavior, independent image audits; preserve media/hash contract where promised. |
| 4 | Allocation-light SF preflight plus direct typed emission: writer allocates 9.76–18.17 GiB; direct Marshal + encodeSFBlocks own 78–92% of write allocation deltas. [Code](../../internal/engine/structured_stream.go#L16). | Reject-before-output, cancellation/short-write tests, exact hashes and complete provenance/global references. |
| 5 | Clone/intern short source names at ownership boundaries; compact per-run provenance allocation and reserve exact run capacities: live models 2.66–5.65 GiB, IPCC content strings 791 MiB. [Source](../../internal/engine/pdf_read.go#L85), [marked tags](../../internal/engine/pdf_structure.go#L294). | Keep exported mutable model semantics, distinct ownership, all source facts; post-release heap and measured live bytes/objects. |
| 6 | Cache candidate bounds, then qualify page-local nearest-content indexing: Intel navigation 9.80 sampled CPU seconds, bounds 7.42s. [Code](../../internal/engine/pdf_navigation.go#L1069). | Preserve nearest/tolerance/order ties, physical views, inferred anchors and exact navigation hash. |
| 7 | Caller buffering / language validation caching: useful secondary work; buffering's measured total improvement is only 0.9% in one pair. | Repeat controlled A/Bs; no claimed universal benefit from current data. |

These are implementation candidates with evidence, not delivered codec speedups.
Do not skip source facts, disable strict validation, weaken work limits, remove
preflight atomicity, use lossy image shortcuts, or add global unbounded pools.
SF remains the canonical format. Reducing temporary graphs is compatible with
that contract; a bounded/page-streaming public model would require separate
API/lifetime design.

## Limits, checks and next commands

This is a solo-process bottleneck study, not a new RPS/TPS qualification. The
earlier ECMA two-worker slowdown remains measured, but this campaign did not
profile that concurrent window or causally apportion CPU contention, host swap
and memory pressure. The earlier single after-run swap observation cannot do so.
No mutex/block/trace/hardware-counter campaign, huge PPTX or PDF/DOCX generation
profiling, non-Mac deployment, or long-lived service leak claim is added here.
The 8 GiB soft target affects GC behavior; natural snapshots are stage ends, not
precise peaks. Modest pprof analysis/build activity and other desktop work can
affect timings. One buffer pair is not a latency distribution.

Focused diagnostic tests/vet/build passed before runs. Final GOWORK=off normal
and race suites, vet, CGO-disabled CLI build and git diff --check pass; production
package tests were cached, while the new diagnostic buffering/error-propagation
race test ran fresh (1.364s). Production Go files are unchanged from 7111871.
No VERSION bump, manual tag or workstation release is part of this investigation.

[Reproduction commands](../TESTING.md#huge-original-cpu-and-heap-diagnosis) build
the current runner. Actual runs used source-equivalent tooling checkpoints
2f67b5c (initial CPU/memory) and b6f8a12 (counter/A-B/SF/Word CPU); binaries were
built before those commits, so embedded VCS metadata correctly records the prior
base plus modified=true. Their exact hashes/build metadata are in the results JSON.

Inspect the strongest read allocation path locally:

```sh
go tool pprof -top -sample_index=alloc_space \
  output/pprof-huge/memory/ecma376-part1-r1-w0/read_live.heap.pprof
go tool pprof -list=layoutOrder -sample_index=alloc_space \
  output/pprof-huge/memory/ecma376-part1-r1-w0/read_live.heap.pprof
go tool pprof -top -sample_index=alloc_space \
  output/pprof-huge/sf-memory/ecma-derived-sf-r1-w0/read_live.heap.pprof
```

Implement priority 1 first, run the full existing regression/race/vet/build checks,
then repeat the same pinned-original CPU/heap jobs with a fresh profile root and
require exact SF hashes before claiming allocation or capacity improvements.
