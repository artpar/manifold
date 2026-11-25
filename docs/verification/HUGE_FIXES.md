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
