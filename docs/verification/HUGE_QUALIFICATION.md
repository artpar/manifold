# Huge original-file qualification

**28/28 complete original → SF conversions passed** under the explicit large-file policy: two solo runs and a two-child batch for each of seven pinned Internet originals. No original bytes were generated, padded, resized or edited. Total: **65,052 physical PDF pages**, **537,620 source DOCX XML paragraphs**, and **45,244,410,520 bytes of fully materialized SF**. Every repetition/worker produced the same SF SHA-256 for its source.

This supersedes the historical zero-success baseline for this scope, not for arbitrary document fidelity. See [pinned originals](HUGE_CORPUS.json), [baseline](HUGE_INPUTS.md), [repair history](HUGE_FIXES.md), and [machine-readable evidence](HUGE_RESULTS.json). Publisher binaries and converted documents remain untracked.

## Protocol

Frozen Go implementation: `711187193428751653b037a517fe0d421f2ab74a`; measurement checkout `487f28f` has identical tracked Go/module/schema files. Go 1.25.0, CGO disabled, M1 Max 10 cores / 32 GiB / macOS 15.4, active desktop. No other QA ran during timed phases. Source SHA-256/size checks precede timing and warm the OS file cache. Each conversion runs in a fresh process, uses a seekable input, materializes SF to a real buffered file, hashes all output, then closes/renames or removes it. No fsync.

Policy: 4 GiB decoded/interpreted work, 2B raster pixels, 1M navigation entries, 2M logical-structure entries, 32M ParentTree slots. Go soft memory target 8 GiB per child. Solo sampled child RSS guard 12 GiB / deadline 10m; two-worker aggregate guard 20 GiB / deadline 15m. This opts into work; it is not a memory reservation. See [API/CLI recipe](../reference/PDF_RESOURCES.md#explicit-large-document-work-policy).

## Measured complete-job throughput

A job includes import, content checks, full SF output and its hash. Jobs/s are conversion TPS, **not HTTP RPS**. Ranges contain only two solo observations; two-worker values are one batch, not sustained maximum capacity or latency percentiles.

| Original | Source scale | Solo wall s | Solo peak RSS MiB | Solo jobs/s | Two-worker wall s | Two-worker jobs/s | Combined peak RSS MiB |
| --- | --- | --- | ---: | --- | ---: | ---: | ---: |
| ipcc-ar6-wgi | 423.35 MB; 2,409 PDF pages | 139.92–141.28 | 8,305 | 0.0071–0.0071 | 176.00 | 0.0114 | 12,014 |
| intel-sdm-084 | 25.17 MB; 5,057 PDF pages | 94.59–95.85 | 8,198 | 0.0104–0.0106 | 132.30 | 0.0151 | 14,000 |
| ecma376-part1 | 35.30 MB; 5,039 PDF pages | 143.58–144.95 | 8,776 | 0.0069–0.0070 | 312.30 | 0.0064 | 13,956 |
| 3gpp-38331-i00 | 3.89 MB; 66,185 XML paragraphs | 3.19–3.20 | 886 | 0.3125–0.3138 | 3.35 | 0.5973 | 1,813 |
| 3gpp-38331-i10 | 4.00 MB; 68,220 XML paragraphs | 3.29–3.29 | 1,009 | 0.3042–0.3044 | 3.42 | 0.5849 | 1,940 |
| etsi-38331-v180100 | 7.24 MB; 1,624 PDF pages | 29.05–29.85 | 2,956 | 0.0335–0.0344 | 31.85 | 0.0628 | 6,112 |
| cpp-n4950 | 8.04 MB; 2,134 PDF pages | 38.18–38.27 | 3,845 | 0.0261–0.0262 | 43.13 | 0.0464 | 7,790 |

MB is decimal; RSS MiB is binary. These are isolated-process workers, not concurrent requests inside one globally heap-limited Go service. Word cached Pages=1 is stale; the corresponding ETSI V18.1.0 PDF has 1,624 pages, while exact Word pagination was not rendered. The 423 MB original is not a resized or dummy 100 MB substitute.

## Defaults and controls

**4/7 originals also complete with default work limits.** The other three reject explicitly:

| Original | Default result |
| --- | --- |
| ipcc-ar6-wgi | PDF decoded streams exceed 268435456 bytes: PDF decoded stream limit exceeded: 2763826 bytes |
| intel-sdm-084 | PDF navigation: PDF destination name budget exceeded |
| ecma376-part1 | PDF page 2842: PDF decoded streams exceed 268435456 bytes: PDF decoded stream limit exceeded: 1186932 bytes |
| 3gpp-38331-i00 | Complete; same SF hash |
| 3gpp-38331-i10 | Complete; same SF hash |
| etsi-38331-v180100 | Complete; same SF hash |
| cpp-n4950 | Complete; same SF hash |

Those are resource-policy rejections, not unresolved font/image feature failures. The production defaults were not weakened to obtain a benchmark result. Separate deliberately inadmissible controls succeed as controls: 1 MiB aggregate RSS terminates the child (`rss_guard`, ~0.209s); 50ms deadline terminates it (`deadline`, ~0.052s). Neither is counted as successful capacity.

## Content and persistence evidence

| Original | Independent ProseMirror body nodes | Missing normalized characters vs independent reader | Other evidence |
| --- | ---: | --- | --- |
| ipcc-ar6-wgi | 94,658 | 3,037 / 11,467,581 | 2,409 physical pages; 42,422 SF image blocks |
| intel-sdm-084 | 115,637 | 0 / 10,272,447 | 5,057 physical pages; 6 SF image blocks |
| ecma376-part1 | 97,484 | 15 / 7,418,356 | 5,039 physical pages; 3,033 SF image blocks |
| 3gpp-38331-i00 | 12 | 0 / 3,248,134 | 73 preview placements / 71 exact original hashes; 15 exact native equation structures |
| 3gpp-38331-i10 | 12 | 0 / 3,360,695 | 73 preview placements / 71 exact original hashes; 21 exact native equation structures |
| etsi-38331-v180100 | 30,512 | 138 / 3,644,064 | 1,624 physical pages; 1,743 SF image blocks |
| cpp-n4950 | 89,839 | 109 / 4,933,357 | 2,134 physical pages; 0 SF image blocks |

All audits bind to the exact final SF hashes in HUGE_RESULTS.json. Audits from earlier equivalent artifacts were reused only after hashing those files and matching the final output byte-for-byte; both Word audits were rerun on final outputs. Character inventories use NFKC/nonwhitespace counts; original list labels retained in provenance are counted separately. MuPDF U+FFFD values are excluded and reported, not guessed into characters. Counts do not prove reading order, pagination or visual fidelity. Most remaining PDF differences are control-code/Unicode representation differences; they are not relabelled exact parity.

Independent checks found and drove real repairs: skipped Word legacy previews and OMML, missing symbol mappings and table-grid columns, invalid ActualText controls, and TOC U+FFFD placeholders. Both original Word files now have no missing normalized source characters, no missing media hashes, and no missing native equation structures. Their 20 explicit no-break-hyphen elements are counted too. Final Go SF/DOCX round trips retain the original previews and equations.

All five PDF-derived SF artifacts pass central Go Read → Write with identical SHA-256, including the 4.13 GB ECMA file and final corrected 1.75 GB IPCC SF. Final IPCC persistence: read 65.124s; read+write 91.091s during separate QA, not a capacity timing.

Original ETSI explicit-mask and ECMA Matte alpha/white composites match independent MuPDF/Pillow pixels exactly in the inspected assets. IPCC CMYK conversion remains approximate (mean RGB channel differences 7.36/15.01/8.60 of 255 for the inspected image). Original CFF glyph-outline spans were independently rendered and visually compared; missing usable Unicode stays diagnosed rather than invented.

MuPDF image-device counts include its own rasterized vector shading: original IPCC page 62 has one embedded CMYK image plus 12 `sh` paints reported as tiny RGB xref=0 tiles. Direct source-image placement checks match on pages 1/6/61/508/1436: 1/2/8/769/5. Do not mislabel all image-device occurrences embedded assets or claim shading reconstruction.

## Bottlenecks and practical boundaries

1. **Whole typed-model memory remains material.** Peak solo child RSS reaches ~8.6 GiB even for a 35 MB compressed manual. File bytes alone are a poor predictor; structure, spans, navigation and assets dominate. Large jobs need caller admission and explicit work policy.
2. **SF provenance and serialization are substantial.** ECMA produces 4.13 GB SF; its first solo read is 80.75s and write 62.58s. Intel produces 3.07 GB SF; read 47.00s and write 47.39s. SF Read/Write stream top-level chunks, but retain a complete typed model; full-tree APIs and browsers have no huge-document memory qualification. SF.gz is the same compressed SF; its throughput was not measured here.
3. **More workers are not always faster.** ECMA two-worker throughput is lower than solo throughput (312.30s for two versus 288.53s across the two solo runs). About 4.55 GiB host swap use was observed just afterward, without before/after or per-process attribution. Memory/GC pressure is a likely contributor, not a measured CPU-percentage conclusion. Prefer one such heavy job at a time on this baseline; higher concurrency is unqualified.
4. **Coverage limits remain explicit.** Native OMML equations and WMF/EMF previews preserve through SF/DOCX; their PDF rendering is unsupported. Unsupported math output through PPTX/VDOCS rejects. General PDF vector/shading/clipping/transparency reconstruction and color-managed/Word-equivalent visual parity are not qualified; representation losses are diagnosed. CFF spans without usable Unicode may be preserved as transparent images, not searchable/editable text.
5. **This is bounded batch evidence.** No HTTP server, higher worker count, sustained saturation/p99, exact 100 MB fixture, rendered 1000-page DOCX layout or arbitrary-format large-file guarantee is claimed. Earlier small-file throughput applies only to those earlier workloads.

## Verification and reproduction

Full frozen normal/race/vet/CGO-disabled CLI build/diff checks pass. Engine normal 20.582s; engine race 236.147s. Facade normal/race 6.039s/78.847s; CLI 0.508s/2.983s. Final original Word preservation rerun passes in 18.93s; both final independent Word audits pass. Clean hosted Linux/macOS/Windows and Linux race checks passed on the same implementation at [run 45621690181](https://github.com/artpar/manifold/actions/runs/45621690181). The final publication commit is independently gated by the same hosted matrix.

Download the exact originals using [HUGE_INPUTS.md](HUGE_INPUTS.md), then:

```sh
CGO_ENABLED=0 GOWORK=off go build -o bin/manifold-hugetest ./tools/hugetest
bin/manifold-hugetest -large -memory-mib 8192 -rss-mib 12288 \
  -storage file -output-dir output/huge-final-output -repeat 2 -timeout 10m
bin/manifold-hugetest -large -memory-mib 8192 -rss-mib 20480 \
  -storage file -repeat 1 -workers 2 -timeout 15m
bin/manifold-hugetest -memory-mib 8192 -rss-mib 12288 \
  -storage file -repeat 1 -timeout 10m
```

Raw local phase records: `output/huge-final-{solo,two-worker,default,rss-control,deadline-control}.jsonl`. Independent hashes, observations and complete phase records are curated in [HUGE_RESULTS.json](HUGE_RESULTS.json). Optional source/ProseMirror/Go persistence commands are in [TESTING.md](../TESTING.md). Next work, if extending scope, is a separately qualified native math/metafile renderer, smaller provenance/model retention, and same-process/service-specific capacity testing.
