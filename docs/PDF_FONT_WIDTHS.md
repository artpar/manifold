# CID width resources and geometry uncertainty

The reader supports the two CIDFont W forms: a starting CID plus an array of
successive widths, or first/last CIDs plus one width for that range. DW supplies
missing widths. See ISO32000-1 section9.7.4.3 and Adobe's published
[PDF reference, Glyph Metrics in CIDFonts](https://opensource.adobe.com/dc-acrobat-sdk-docs/pdfstandards/pdfreference1.6.pdf).
No universal precedence for the conflicting overlapping declarations in these
robustness fixtures is assumed by this reader contract.

## Work and cancellation

The old reader materialized every range on every occurrence. A width table with
1000 repeated full-CID ranges performed65,536,000 assignments, even though the
final inventory has only65,536 CIDs. It also did not observe context cancellation
during width expansion; the new deterministic cancellation regression returned
nil before the fix.

`readCIDWidths` now parses and validates source assignments once. Shared indirect
numeric arrays are converted once per font. Small expansions retain a direct path
bounded by65,536 assignments. Larger expansions process source entries in reverse,
using successor links to skip already-owned codes. Each final CID is then written
at most once. This preserves the previous last-assignment values without rejecting
otherwise accepted complex width tables or adding an application quota. The helper
checks cancellation while parsing arrays/entries and applying widths. Invalid
width values reject even if a later source entry would overwrite them.

The auxiliary successor inventory is bounded by the16-bit CID domain (at most
65,537 uint32 links). Source record/array storage remains proportional to input;
this does not bound arbitrary document input or decoded-stream memory. It is not
an exhaustive PDF resource-safety claim.

One-shot Apple M1 Max/darwin-arm64 benchmark samples for1000 full ranges changed
from506.612ms/5,203,264B to5.927ms/5,563,760B. Extra bounded bookkeeping increases
this sample's allocation while removing repeated expansion. These are single
samples, not a universal speedup or statistical performance guarantee.

## Overlap is not exact geometry

The independent MuPDF check exposed a preexisting source interpretation discrepancy.
For a full-CID range followed by narrow overrides, manydoc's legacy values place
ABCZA origins at10,14,20,28,38pt. MuPDF observes10,15,21,29,39pt; heavily repeated
ranges produce10,15,20,25,30pt. Text is retained, but these are not interchangeable
physical positions. The original consumer-agreement oracle remains executable and
still fails for those overlapping inputs.

The reader retains its legacy last-assignment values and now emits
`pdf_cid_width_overlap`. Affected advances use `estimated_advance` precision and
0.5 run confidence, with the existing general metric-estimation diagnostic.
Detection is conservative: even redundant overlaps are flagged. No arbitrary
consumer is claimed to be the normative winner. Source content, relationships,
legacy baseline coordinates and other fields remain unchanged; uncertainty is
now explicit. The unambiguous non-overlapping control retains font_metrics
precision and matches MuPDF's original expected origins exactly.

## Evidence and reproduction

Tests verify the complete65,536-CID inventory after repeated shared-array/range
assignments, exact public text/baselines/page/stream/SF, invalid source entries
including overwritten entries, and deterministic cancellation before expansion
finishes. The short fuzz campaign varies ranges, widths and repetition while
retaining fixed final content/geometry overrides and SF semantics.

Optional pypdf independently resolves actual W source entries and computes every
legacy CID value. PyMuPDF supplies independent character origins. The checker
requires exact source graphs and uncertainty for overlaps, plus actual geometric
agreement for the non-overlapping control. Its strict extra switch retains the
original disagreement expectation instead of changing the geometry oracle.

```sh
MANYDOC_PDF_CID_QA=output/pdf-cid-qa GOWORK=off go test -run 'TestPDFCID' -count=1 .
CGO_ENABLED=0 GOWORK=off go build -o bin/manifold ./cmd/manifold
output/qa-venv/bin/python scripts/verify_pdf_cid_widths.py output/pdf-cid-qa
# Expected FAIL on overlapping input; original consumer-agreement oracle:
output/qa-venv/bin/python scripts/verify_pdf_cid_widths.py output/pdf-cid-qa --require-mupdf-agreement
GOWORK=off go test -race -run 'TestPDFCID|FuzzPDFCID' -count=1 .
GOWORK=off go test -run '^$' -fuzz '^FuzzPDFCIDWidthOverrides$' -fuzztime=10s -parallel=2 .
GOWORK=off go test -run '^$' -bench '^BenchmarkPDFCIDRepeatedRanges$' -benchtime=1x -count=1 .
output/qa-venv/bin/python scripts/verify_published_pdf.py output/pdf-cid-published-final
```

The published InDesign source still passes10lists/57items/exact K membership,
16URIs/20,531 normalized characters/DOCX and nine reflowed PDF pages. A CLI built
from exact pre-change Git archive2b4b4d0 produces byte-identical full publication
SF. Its source has none of the newly diagnosed overlapping widths. Synthetic
non-overlapping control SF also remains identical; overlapping synthetic SF changes
only diagnostics and precision/confidence, preserving the other original fields.
No new output-layout or visual-fidelity claim follows from this reader/resource fix.

Decoded streams and broader font/source resources remain the next audit work;
see PDF_RESOURCES.md and STATUS.md. The full goal remains active.

Next decoded-stream audit: pinned pdfcpu0.11.1 StreamDict.DecodeLength is not a
safe drop-in bounded-read API. Cached/raw paths slice directly to maxLen, and the
filtered path limits only the final filter while intermediate Decode calls remain
unbounded; it also slices the result to the requested length. Investigate bounded
per-filter decoding/cancellation and short-stream behavior before changing calls.
This source inspection is a next-step finding, not a shipped stream-memory fix.
