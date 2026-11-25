# Huge original-file repair ledger

Baseline failures and pinned unchanged originals: HUGE_INPUTS.md / HUGE_CORPUS.json.
This is a chronological repair ledger. Final input/persistence qualification is
complete in [HUGE_QUALIFICATION.md](HUGE_QUALIFICATION.md); historical pending
checkpoints below describe earlier states, not current outstanding work.

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

Independent MuPDF inspection of the unchanged IPCC ParentTree counted **22,003,851
owner slots**, of which only **447,733** are nonnull (3,147 arrays, largest 54,815
slots). This explains why the previous coupled owner-slot cap could not represent
the original. Add `MaxParentTreeSlots` / `-pdf-max-parent-slots` as a distinct bounded
scan-work policy: default 400,000, explicit maximum 100M, huge profile 32M. Logical
entries and owner slots are distinct units; unused null slots do not consume the
logical identity/reference budget. Every listed nonnull owner is still checked.

The C++ and ETSI independent text inventories initially excluded original list
labels held in `list_item.attrs.source.label` after diagnosed list normalization.
The audit now counts those source labels separately from text nodes; it does not
pretend original marker styling is rendered unchanged. Both audits are rerunning.

## Further measured bottleneck repair (in progress)

A native stack sample of original IPCC recovery located repeated work in OBJR
content lookup: every tagged object reference scanned every painted unit in the
entire document. Index units by physical page, preserving range/owner checks and
adding cancellation checks. Unknown-page references retain the original global
lookup. The obsolete pre-index scout was explicitly cancelled, not counted as a
success or a deadline failure.

SF `Read` now decodes top-level body chunks directly into the typed model, keeping
strict chunk validation and final global reference/model checks. It avoids holding
a complete SF tree alongside the recovered model; public `ReadStructured` still
returns a complete tree. Root property order is unrestricted; duplicate/unknown
keys, malformed suffixes and dangling cross-chunk references reject. Gzip checksum
errors remain wrapped with their original identity. Focused SF checks pass. This
is required because original PDF outputs are multi-gigabyte SF files; compressed
persistence and memory limits must remain explicit capacity considerations.

A subsequent original IPCC sample located the same document-wide rescan pattern
in tagged logical-link ownership. Reuse authoritative page/object annotation keys;
unknown-page ownership uses a lazy object index and preserves target-conflict
checks. Unpainted annotation OBJRs skip page content scans. This is an indexing
repair, not a decision to omit tagged links. Focused navigation/structure checks
are being rerun before final measurements. Earlier scout cancellations stay
explicitly classified as cancelled.

## Unrepresentable logical replacement metadata

The original IPCC page 21 has marked-content ActualText `FEFF000900090007`
(two tabs and BEL) over painted spaces; MuPDF reports the BEL too, while its paint
trace does not contain a BEL glyph. Applying it caused final XML-compatible model
validation to fail. If ActualText contains unrepresentable controls, retain painted
content and diagnose the unapplied logical replacement with its exact original PDF
string bytes in hexadecimal. Do not invent replacement Unicode or drop painted
content. Structural Alt controls use literal escapes, also retaining exact original
bytes in diagnostics. Oversized unrepresentable metadata rejects at 64 KiB rather
than creating unbounded diagnostics. The focused ActualText preservation/SF checks
pass. This is a disclosed logical/accessibility representation loss.

The derived 1,186,617,547-byte C++ SF was read back through the central API and
written with **identical SHA-256** (96bae814…): read 50.90s, read+write 70.37s during
concurrent QA. This is persistence QA, not a quiet capacity measurement. The opt-in
`TestHugeOriginalDerivedSFRead` keeps a Go soft limit and validates the entire graph.

The next full IPCC attempt exposed U+0008 from decoded glyph text after the
ActualText repair. Font decoding now rejects XML-forbidden declared codepoints
before they enter the model, allowing the already-qualified embedded-CFF outline
preservation path instead of a late model failure. No source Unicode is guessed;
fonts without supported outline preservation still reject. Focused font/control
checks pass; the original full-document rerun is pending. Run-text validation errors
now include source page/font context to make any remaining source failure actionable.

## All original imports complete; final qualification pending

The 2026-11-25 IPCC scout completed all **2,409 physical pages**, **42,455 image
blocks** and **90,887 paragraphs**. Read 115.99s; SF write 30.95s; total 146.97s;
external sampled peak RSS 7,766.5 MiB. Output 1,745,726,243 bytes, SHA-256
5e932817a98bfb317aa0b611756554e333a0abce0e717a4f25d48df7bea9a252.
This was a diagnostic scout during independent QA, not a quiet throughput run.
All seven pinned originals have now completed an input → SF scout; final repeated
measurements will use one frozen implementation and explicitly recorded work policy.

Intel independent MuPDF / streaming ProseMirror audit reports **zero missing
normalized source characters** across 5,057 pages / 10,272,447 characters, including
original normalized-list labels retained in provenance. C++ and ETSI still have
109/138 inventory differences from MuPDF respectively, mostly source control-code
representations; they are not being relabelled perfect textual/visual fidelity.
Independent full IPCC/ECMA audits, original Word preservation reruns, full regression
matrix and repeated solo/two-worker stress remain pending before completion.

Earlier local race attempts overlapped source-file edits and produced mixed-build
compiler errors (new fields/imports absent from the initial package snapshot).
These are invalid verification attempts, not passing checks. The final matrix is
now running on frozen source; its result will supersede those attempts. Hosted
c3285ec full CI passed; later hosted source checks are pending.

ECMA's independent MuPDF / streaming ProseMirror audit checked all 5,039 pages,
97,484 body nodes and 3,033 image blocks. Its source character inventory differs
by 15 of 7,418,356 normalized characters. These are retained as observed reader
representation differences, not silently corrected into a perfect-match claim.
Full source audits and central Go SF read/write SHA-256 checks remain separate:
the latter verifies exact persistence of Manifold's complete recovered graph.

## Audit caught a real replacement-content loss

The full IPCC audit was not accepted merely because import completed. Its 31,713
page-inventory differences included thousands of valid TOC dot leaders: producer
ActualText combines those dots with U+0008. Ignoring the entire invalid replacement
(as the previous checkpoint did) dropped valid replacement characters. Correct the
policy: retain **all XML-compatible replacement characters**, disclose controls and
retain exact original bytes in diagnostics. When no printable replacement remains,
retain painted glyph content instead. Structural ActualText uses the same policy;
Alt controls remain literal escapes. A focused regression requires the original
control-plus-dots pattern to retain its dots. The original full rerun and audit are
pending. The earlier successful import scout is not final content qualification.

Published format references used for independently authored preservation code:
[Microsoft WMF header](https://learn.microsoft.com/en-us/openspecs/windows_protocols/ms-wmf/d169108a-e3fe-436a-bb44-bea61a46ce56),
[WMF framing](https://learn.microsoft.com/en-us/openspecs/windows_protocols/ms-wmf/ba5458c6-e885-41e6-b5d7-d54ef9e1065f),
[EMF framing](https://learn.microsoft.com/en-us/openspecs/windows_protocols/ms-emf/daaf9447-0c47-446e-b72e-ac6bd7a2e8f1),
[EMF signature](https://learn.microsoft.com/en-us/openspecs/windows_protocols/ms-emf/b55cbd24-612d-47fe-81c9-1da6a682246f), and
[Adobe CFF Technical Note 5176](https://adobe-type-tools.github.io/font-tech-notes/pdfs/5176.CFF.pdf).
No proprietary UniDoc implementation was copied or translated.

## Precise TOC root cause and source-backed recovery

Closer source inspection supersedes the earlier “control-plus-dots” explanation
for the original TOCs: their ActualText often contains **only U+0008**, while
font **KIAGLJ+FrutigerLTPro-Light**, ToUnicode object **63**, explicitly maps
`<2E>` to `<FFFD>`. The same original font explicitly declares WinAnsiEncoding,
whose glyph name is `period`. Ignoring invalid ActualText alone therefore left
replacement characters instead of dots. Retain valid replacement characters as
above, and resolve a simple-font U+FFFD placeholder **only when an explicit source
Encoding supplies a valid, different glyph Unicode value**. Diagnose this separately
as `pdf_cmap_placeholder_encoding`. An undeclared placeholder stays unchanged;
there is no family/GID/ASCII guess.

Direct original-page checks after this fix recover dot counts 1,824 / 2,145 /
2,912 / 3,155 on pages 16 / 166 / 1532 / 1947. MuPDF counts 1,824 / 2,142 / 2,909 /
3,152; the additional three correspond to source placeholder extraction differences,
not thousands of lost leaders. The source font map/Encoding was inspected directly.
This uses original bytes, not a rewritten font or altered oracle. Focused regressions
check declared period recovery and prohibit an undeclared-character fallback.

Derived SF persistence checks completed for all five PDF originals: C++, ECMA,
Intel, IPCC and ETSI re-emit byte-identical SF SHA-256 values after central Go Read.
The IPCC persistence check predates the final dot-placeholder repair; it proves
exact persistence of that recovered graph, not final visible-content fidelity.
Final original-file reruns and independent audits are still required.

## Image-count denominator and frozen regression results

MuPDF's generic image-device inventory includes images it rasterizes from vector
shadings. Original IPCC page 62 has one embedded CMYK XObject (1052), no `BI`
operators, and 12 `sh` operators; MuPDF additionally reports 12 tiny RGB tiles with
xref=0. Direct original-page production-reader checks agree on the real image
placements for pages 1 / 6 / 61 / 508 / 1436: **1 / 2 / 8 / 769 / 5**. Do not call
all 53,657 MuPDF image-device occurrences embedded assets, or claim shading
appearance recovery. Unsupported shading/vector effects retain explicit diagnostics.
The independent audit field is named `mupdf_image_device_occurrences` accordingly.

Frozen implementation **711187193428751653b037a517fe0d421f2ab74a** passes the full
normal and race suites, vet, CGO-disabled CLI build and diff check. Normal engine
20.582s / facade 6.039s / CLI 0.508s; race engine 236.147s / facade 78.847s / CLI
2.983s. These final commands ran without Go source edits during the matrix. Earlier
mixed-snapshot build failures are not substituted for these passing results.

Final repeated measurements are running with QA stopped. First two 423 MB IPCC
jobs complete at 139.916s / 141.276s batch wall; identical full SF SHA-256 df759827…;
peaks 8,305 / 8,162 MiB child RSS. Intel's first 5,057-page job completes at 94.59s.
These are preliminary rows of the frozen run, not the completed stress verdict.


## Final qualification

All 28 guarded large-policy original → SF jobs pass; all four repetitions/workers
per source have identical SF hashes. Default policy also completes four originals;
three reject only by documented work budgets. Deliberate RSS/deadline controls
terminate correctly. Final source/ProseMirror audits, original Word SF/DOCX asset
and native-equation checks, all five PDF-derived Go SF persistence checks, complete
normal/race/vet/CGO-disabled build and clean hosted cross-platform checks pass.
The final corrected IPCC SF (df759827…) read/writes identically in 91.091s during
separate QA. No Go/module/schema source changed after frozen implementation 7111871.

The independent Word audit was tightened to count its 20 explicit no-break-hyphen
nodes as well as w:t/math/symbol text; both originals now have equal normalized
source/recovered character inventories, not merely zero missing characters. No
source or expected original glyph/asset/equation was removed. The remaining native
rendering, PDF appearance, memory and service-capacity limits are explicit in the
final report; this is not an unrestricted Word-equivalent completion claim.
