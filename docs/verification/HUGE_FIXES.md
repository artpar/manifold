# Huge original-file repair ledger

Baseline failures and pinned unchanged originals: HUGE_INPUTS.md / HUGE_CORPUS.json.
Work is in progress; a first blocker passing is not full-document qualification.

## First implementation checkpoint

- Resolve empty DOCX theme-region faces from an explicitly declared run language
  and the source theme's supplemental script face, with a diagnostic. Configured
  theme languages/default faces remain authoritative; unresolved faces still reject.
- Map declared Symbol/ZapfDingbats legacy codes through the existing attributed
  encoding/glyph-name dependency. Unknown fonts/codes still reject. The old Symbol
  rejection test now requires the actual bullet to survive and retains a separate
  unknown-font rejection; this is a newly supported feature, not an altered source.
- Retain skipped Word table grid columns as explicit empty aligned cells, with an
  open-edge geometry normalization diagnostic rather than claiming exact table XML.
- Complete partial simple-font ToUnicode maps only from explicit source Encoding
  glyph names, with diagnostics. Composite/undeclared characters are not guessed.
- Decode CMYK JPEG Decode arrays in component space through the existing decoder;
  decode raw CMYK samples directly instead of emitting unsupported intermediary TIFF.
- Undo device RGB/Gray soft-mask Matte preblending before alpha; apply explicit
  1-bit masks and color-key masks using raw pre-Decode samples. Invalid/unknown
  semantics still reject. Source-profile/device RGB approximation stays diagnosed.
- Index geometric links by physical page, preserving overlap rules and adding
  cancellation checks. A diagnostic stack located the previous >120s C++ import
  in a document-wide link scan per span. The revised original completes import.
- Read existing seekable PDF inputs directly, avoiding another full-file copy.
  Retain direct-encryption adaptation on the exceptional parser-error path;
  nonseekable inputs retain the established buffered behavior.

Focused font/theme/symbol/mask/navigation/encryption tests pass. Independent
original-image extraction is opt-in via MANIFOLD_HUGE_IMAGES=1 go test
./internal/engine -run TestHugeOriginalImagePixels. PyMuPDF/Pillow comparisons:
ETSI object 99 explicit-mask alpha and white composite match exactly; ECMA object
9020 Matte alpha and white composite match exactly. Both were visually inspected.
IPCC object 1024 CMYK alpha agrees; device/profile RGB conversion differs (mean
channel errors 7.36/15.01/8.60 of 255), explicitly diagnosed rather than claimed
color-managed equivalence. Original bytes/hashes remain unchanged.

Initial post-fix scouts completed full PDF→SF for ETSI (1,624 pages, 1,743 images)
and C++ N4950 (2,134 pages). They still allocate heavily and reach ~6.6/7.3 GiB
sampled RSS while producing ~1.08/1.19 GB SF, so these are diagnostic scouts, not
final capacity. ECMA subsequently hit the existing 256 MiB decoded-work budget.
Word imports progressed to grid omissions, now handled with explicit diagnostics.
Large-file resource policy, bounded SF serialization, independent full-content QA,
full regressions and final frozen-source stress reruns remain pending.

## Bounded processing checkpoint (in progress)

Flowing SF Write now validates the complete model/references and preflights every
encoder chunk before emitting bytes, then streams bounded body chunks. It avoids
the redundant full SF-tree → model self-round-trip in this writer path. Public
ToStructured/FromStructured retain full graph validation. Eight original small
corpus files produce byte-identical SF through both writers; malformed suffixes
still reject before any output. Existing SF regressions pass. No persisted schema
or original oracle/fixture bytes were changed.

ReadOptions.PDFLimits adds explicit aggregate decoded-work, raster-work and
navigation-entry policies. Zero fields retain the old conservative defaults;
per-stream/image/recursion guards remain. The model caps navigation at one million
entries AND 64 MiB total destination-name bytes. Decoded-stream caching is capped
at 32 MiB / 4,096 entries independently of permitted cumulative work. Duplicate
page diagnostics are indexed without changing their ordered persisted values.
Parser-only state is released before untagged geometric inference (or after tagged
recovery) so it does not remain live while the editable graph is built.

CLI flags expose the policies: -pdf-max-decoded-mib, -pdf-max-raster-pixels and
-pdf-max-navigation. The huge runner's explicit -large profile currently chooses
2 GiB decoded work, 500M raster pixels and 1M navigation entries, still guarded by
an external process deadline/RSS budget. -storage file uses seekable input and
materializes streamed output in a real file (optional -output-dir keeps it), with
output SHA-256 included. This is a separately labelled measurement from the old
in-memory baseline, not a silent change to its denominator or retained outputs.
The runner's nonempty-content walker was corrected to descend into Sections;
the earlier empty-DOCX scout was a harness accounting bug, not missing content.

Indexed device-color images now decode directly from original palette/sample
bytes, including CMYK, instead of relying on an unsupported intermediary TIFF.
Original-file scouts and full-content checks are still in progress; do not claim
all seven originals pass from these implementation changes alone.

## Original-content preservation checkpoint (2026-11-25, in progress)

The original Word imports revealed a silent-loss defect beyond the initial blockers:
73 legacy drawing placements (71 distinct WMF/EMF assets) and 15/21 native Office
Math equations. These are now preserved through typed model → SF → DOCX. An opt-in
original-file test verifies every preview SHA-256 and placement count and the native
equation count in both outputs. Both original versions pass. Native previews use
bounded metafile framing validation, without executing embedded OLE applications.
Floating geometry is diagnosed as normalized. Native equations use an additive
`office_math` SF block with authoritative self-contained OMML, not guessed Unicode.
Plain text exposes its source tokens; PDF/PPTX/VDOCS math output rejects explicitly.
This is preservation, not a newly claimed equation/metafile rendering engine.

PDF fixes also retain unmapped embedded CFF outlines as transparent span images;
known Unicode stays text. The original IPCC page 98 has 48 such spans. Missing
Unicode is diagnosed with source font/codes/geometry; OCR/text is not fabricated.
Raster density includes the PDF frame transform. Source tint Decode is applied
before Separation/one-component DeviceN functions; bounded type-2 and numeric
calculator-function subsets support the original black-channel images. Unknown
operators/color semantics reject. A request-owned 256-entry font cache and indexed
warnings avoid reparsing fonts and rescanning thousands of diagnostics per span.
Inference consumes/relinquishes page graphs and sorts integer indices rather than
repeatedly moving large block values. Navigation validation now also budgets
explicit unresolved references against one million identities/64 MiB name bytes.

The explicit huge scout profile is now **4 GiB cumulative decoded work, 2B raster
pixels, one million navigation entries**, with **6 GiB Go soft heap policy** and
**8 GiB sampled external RSS guard** for solo jobs. These opt-in policies do not
change default import limits. The previous 4 GiB soft heap produced severe GC
thrashing on large live graphs. The runner now cancels children on parent signals.
The earlier 2 GiB profile above is historical, superseded by this scout profile.

Five of seven full original imports completed in a diagnostic scout, but that
scout predates the Word preservation repair and the latest IPCC/Intel fixes.
It is **not final qualification**. Frozen-source reruns, independent full-text and
ProseMirror audits, complete regressions and publication remain in progress.

Independent streaming ProseMirror + XML audit of i00 currently reports zero missing
NFKC/nonwhitespace source characters, all 71 original media hashes / 73 placements,
and all 15 native equations. The normal Go suite passes after this checkpoint.
The IPCC original consumes more than 500M aggregate raster pixels (guard at page
1566); the opt-in huge policy is now 2B, with the same external RSS/deadline guards.
Default policy is unchanged; this guard rejection remains recorded in scout logs.

## Logical-structure work policy (in progress)

IPCC full page interpretation now completes, then the original tagged tree exceeds
200,000 attribute-work entries. `PDFReadLimits.MaxStructureEntries` (CLI
`-pdf-max-structure`) makes the existing structure/ParentTree/reference/attribute
work limits explicit. Zero retains 200,000; the maximum accepted policy is 10M.
Depth/cycle/owner/reference validation is retained. The huge profile opts into 2M
structure entries. Unit checks verify unchanged defaults, policy ceilings and
actual attribute-work enforcement. A new diagnostic scout uses 8 GiB Go soft
memory / 12 GiB external RSS after smaller policies exposed GC pressure. Neither
that scout nor concurrently running QA is a final throughput measurement.

Both Word independent audits now verify **zero missing normalized source
characters**, zero missing native equation structures, and all original diagram
hashes. Extra 20 source discretionary-hyphen displays are retained. MuPDF-vs-SF
PDF character inventories are under review; these compare representations, not
page visual parity. Original mask composites and CFF outline contact sheets have
been independently rendered and visually checked; source/recovered glyph shapes
agree in the inspected original page-98 spans.
