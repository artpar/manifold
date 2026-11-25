# Huge original-file qualification

Status: solo phase complete; two-worker/default-policy phases still running.

Frozen Go implementation: `711187193428751653b037a517fe0d421f2ab74a`.
All seven sources are unchanged Internet originals pinned in [the manifest](HUGE_CORPUS.json).
Publisher files and converted output remain untracked. The historical zero-success baseline remains in [HUGE_INPUTS.md](HUGE_INPUTS.md).

## Scope and protocol

This measures complete original DOCX/PDF → canonical SF, including real buffered file output and SHA-256. It does not claim Word-equivalent rendering, arbitrary-document support, browser editor capacity or HTTP RPS. Same-format SF persistence and independent content audits are separate checks.

Baseline: M1 Max / 10 cores / 32 GiB, macOS 15.4, Go 1.25.0, active desktop. Cold child processes; OS input data warmed by integrity checks; file writes without fsync. No other QA runs during the timed phases.

Large work policy: 4 GiB decoded/interpreted bytes, 2B raster pixels, 1M navigation entries, 2M structure work entries, 32M ParentTree slots. Go soft memory target 8 GiB per child; solo sampled RSS guard 12 GiB and 10-minute deadline; two-worker aggregate guard 20 GiB and 15-minute deadline. Defaults remain conservative.

## Completed solo measurements

**14/14 conversions succeeded**; both repetitions produce identical full output hashes for every source. Ranges below contain only two observations, not percentile or sustained-service capacity estimates.

| Original | Source scale | Complete wall seconds (two runs) | Peak sampled RSS MiB | Observed jobs/s | SF bytes |
| --- | --- | --- | ---: | --- | ---: |
| ipcc-ar6-wgi | 423.35 MB; 2,409 PDF pages | 139.92–141.28 | 8,305 | 0.0071–0.0071 | 1,745,648,991 |
| intel-sdm-084 | 25.17 MB; 5,057 PDF pages | 94.59–95.85 | 8,198 | 0.0104–0.0106 | 3,069,352,846 |
| ecma376-part1 | 35.30 MB; 5,039 PDF pages | 143.58–144.95 | 8,776 | 0.0069–0.0070 | 4,132,810,817 |
| 3gpp-38331-i00 | 3.89 MB; 66,185 XML paragraphs | 3.19–3.20 | 886 | 0.3125–0.3138 | 46,390,537 |
| 3gpp-38331-i10 | 4.00 MB; 68,220 XML paragraphs | 3.29–3.29 | 1,009 | 0.3042–0.3044 | 48,307,780 |
| etsi-38331-v180100 | 7.24 MB; 1,624 PDF pages | 29.05–29.85 | 2,956 | 0.0335–0.0344 | 1,081,974,112 |
| cpp-n4950 | 8.04 MB; 2,134 PDF pages | 38.18–38.27 | 3,845 | 0.0261–0.0262 | 1,186,617,547 |

MB is decimal; RSS MiB is binary. The Word producer caches Pages=1, which is stale. Corresponding ETSI V18.1.0 PDF has 1,624 pages; exact Word pagination was not rendered or claimed. A 423 MB original was tested; no synthetic or resized 100 MB file was substituted.

The full two-worker results, default-policy outcomes, audited artifacts and final checks will be appended after those phases finish. See [the repair ledger](HUGE_FIXES.md) for source-backed fixes and observed limitations.
