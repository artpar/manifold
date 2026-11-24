# Active goal acceptance audit

Audited 2026-11-24 after the SF foundation merged-table and explicit-list and typed-note, positioned-PDF and forward-tagged-PDF and Unicode-typography/implied-paragraph/untagged-inference/painted-grid/rich-cell/body-table-continuation/recursive-table/explicit-page-break/paragraph-property/image-height/repeating-header/text-style/PDF-style-recovery/theme-color/font-slot/glyph-transport/language-LTR-shaping/default-shaping/paint-context milestones. **Not complete.** Passing initial
checks does not satisfy the expanded user goal. No external blocker currently
prevents continuing implementation.

| Required outcome | Current evidence | Remaining work |
| --- | --- | --- |
| Established shared SF with documented tradeoffs | ProseMirror JSON profile, primary-source comparison, executable NodeSpec/MarkSpec, strict Go codec; actual ProseMirror independently reads 51 corpus outputs (9,031 nodes after font-slot recovery) | Expand profile/engine together; current bridge subset is insufficient |
| Inspect Colleaf representation if available | Workspace search and read-only Daptin source search found no editor schema | Inspect real Colleaf source if it becomes available; do not claim .vdocs compatibility |
| DOCX → SF | CLI/API, accepted corpus semantics including links/assets preserved through checked bridge | Resolved font family/size/color and style/default/character overrides now persist; theme tint/shade/settings mappings calculate RGB and font slots/theme languages resolve styled spans; remaining DrawingML/theme-script/shading/embedded-font details, broader numbering formats/qualification, richer page/section settings; broader note numbering/separator qualification |
| SF → DOCX | CLI/API and independent python-docx checks for report output | Remaining rich-model gaps and broader note layout qualification; representative exported DOCX and visual checks. Production paragraph-property encoding now retains both previously missing note side texts in LibreOffice; cell/note pagination differs and broader Word/real-export qualification remains |
| SF → PDF | CLI/API, independent pypdf/Poppler checks, two SF-generated report pages visually inspected | Measured Unicode spaces, paragraph-wide grapheme/line wrapping, nine-page explicitly normalized real-export reflow with content/URI/spacing/bounds checks now pass. Rich image/nested cells and image/table footnotes now have independent geometry/pixel/link/continuation checks. Oversized body rows/groups now continue with independent cell-order/border/image/note checks. Recursive nested/note groups now have seven-page independent column/border/pixel/destination checks. Explicit cell/note boundaries now have five-page and blank-note independent checks. Full-container image fitting and figure-note deferral now have exact pixel/extent/destination checks. Repeating prefixes now have five-page independent merge/pixel/body/note checks. Actual Go/Go Mono and supplied TTF faces, explicit family mappings, measured mixed-size lines and RGB/decorations now have four-page independent checks. Glyph-ID transport, supported LTR marks/supplementary outlines now have independent two-page GID/GPOS/font-hash checks. Remaining: broader script/itemization/context, fonts/layout and qualification |
| PDF → SF with geometry/assets/structure | Positioned runs/raster assets and page geometry/transforms; MCIDs/ActualText; editable family/physical size/device-color fields with provenance and ten independent span checks; strict SF provenance round trips; independent two-page baselines/pixels/placement checks | Checked forward tagged trees now recover common blocks/merges/URI ownership, implied LBody/cell paragraphs and cross-page regions; original /K proves all 57 published body memberships. Untagged whitespace-cut reading order, aligned paragraphs, relative headings, marker lists and numeric tables now have independent geometry/membership/coverage checks. Painted rectilinear grids now have exact independent cell/merge/empty bounds plus targeted actual Writer-export qualification. Remaining: broader untagged/occlusion/double-border/complex-layout recovery, internal/partial-span links, broader roles/attributes/images/fonts and inverse ParentTree validation; current path still does not fulfill the full requirement |
| Paragraphs/headings/text formatting/links | Current declared boolean marks and link destinations covered structurally | Font size/family/color now persist through textStyle and DOCX defaults/style/character/direct resolution; PDF measures actual selected faces/sizes/colors. DOCX theme text tint/shade/mappings now calculate RGB with independent eleven-binding checks. Font-slot/theme-language normalization now preserves used span choices, with independent eight-span checks. Remaining selection qualification, DrawingML/theme binding/shading/paragraph properties and broader PDF style/color/source-font coverage; conditional paragraph starts now preserve inheritance/cancellation and SF/DOCX semantics |
| Lists/tables including merged cells | Explicit nested lists, starts, multi-block membership, ordinary DOCX numbering/restarts and PDF body/cell lists; nested tables; row/column spans across SF, DOCX import/export and PDF rich-table layout, with exact geometry/pixel/nested-note tests | Broader numbering formats and external continuation inference/qualification; broader rich-cell/header-role qualification; repeating prefixes now preserve SF/DOCX semantics and native continuation |
| Images/metadata/page settings/running content/notes | Images, metadata, section bodies, four-edge page layouts, running distances and independent default/first/even bindings, rich measured PDF running content; typed references and rich note definitions survive SF/DOCX, PDF rich footnotes/endnotes render with links/continuations, including images and tables | Fourteen targeted control/empty-owner cases now qualify native policy; broader producer parity/inheritance and DOCX font/containment qualification remain; typed PDF note recovery, broader image variants and note numbering/layout; LibreOffice flattened complex note tables in the inspected DOCX, requiring broader visual/Word qualification |
| Revisions/comments/fields/unsupported behavior | Final-visible-view extraction and explicit loss diagnostics; strict output rejection | Qualify/document complete policy against real exports; do not disguise required gaps as unsupported exclusions |
| Supported SF round-trip semantics | Current engine fields compared across 51 accepted inputs, targeted rich tree regression, unsafe separate-list collapse rejected | Expand preservation coverage as required semantics are implemented |
| Stable public Go API and CLI | Existing Read/Write, new public structured tree/codecs, CLI atomic output and four-path tests | Reassess/finalize rich API contract after missing semantics land |
| Pure Go in-process, no service/CGO/executable requirement | Go tests/race/vet/module verification and host CGO-disabled build pass; new SF codec adds no dependency | Repeat final cross-builds and audit once feature work is finished |
| Representative corpus, independent readers and visual QA | Targeted synthetic corpus, original Word MQTT 5.0 DOCX/PDF pair with all cell/merge/text/link/asset oracles, one W3C PDF plus an original ten-page published InDesign government export (57 list items/16 URI targets), current report checked independently and PDF rendered; merged-table example checked structurally, native PDF and LibreOffice-rendered DOCX visually inspected | Real exported documents from representative editors/producers, licensing/provenance, structural oracles and broader DOCX visual acceptance |
| Robustness/final checks | Existing deterministic damaged-input tests pass; SF fuzz 177,117 executions passed; full tests/race/vet/build pass | Repeat relevant malformed-input/fuzz and final complete checks after remaining implementation |
| Recovery documentation | STATUS/ARCHITECTURE/DECISIONS/COMPATIBILITY/STRUCTURED_FORMAT/TESTING plus verification summary | Keep current each milestone; no completion claim until every row is proven |

Next implementation slices: expand directional shaping with remaining font/size/language and within-grapheme context on the unified default/tagged LTR shaper, then source-font handling, then complex color/selection details
and broader font qualification. Editable PDF device-style recovery now has ten
independently authored span/style/baseline checks through both output formats. Family/size/
color and configured font handling now exist with targeted checks. Repeating
header prefixes now work through SF/DOCX and native body/nested/footnote continuation.
Broader layout/Word and real-export qualification remain open. Full-container image fitting,
figure-note deferral and bounded raster validation now exist. Conditional
paragraph starts and the production DOCX content mitigation now have whole-SF
round trips and independent native/producer checks. Measured image/nested cells, rich footnotes and
recursive body/nested/note table continuation now exist. Typed notes, explicit list
boundaries, starts, hierarchy and multi-block items now exist.
Merged row spans and recursive group splitting exist; forced native breaks exist, while broader DOCX visual/Word and continuation qualification remain unfinished. Use the concrete recovery commands in STATUS.md. None of these
required capabilities may be removed from this audit simply because it is hard
to implement or currently rejected.


Language/paragraph/run direction/forced-CS bindings now preserve effective SF and
DOCX semantics, with independent inherited XML and all-story checks. Language-
tagged native LTR shaping qualifies localized GIDs, kerning, ligatures and actual
re-shaped wrapped lines against C HarfBuzz. This narrows the implementation gap
without completing bidi, default/style-context shaping, broader Unicode or the
full rich-document goal. RTL rejection remains a limitation, not acceptance proof.

Default authored text now shares contextual LTR glyph/line shaping without language
metadata. Native GIDs/origins, program hashes, default font/locale equivalence,
marked spaces and decoration continuity are qualified; bidi/style context and the
other incomplete rows still prevent completion.


Paint-context qualification now covers kerning and ligatures across color,
decoration and URI boundaries with original-font/GID/GPOS/GDEF/clip/advance checks,
both-reader complete logical words, source provenance and DOCX/visual checks.
This narrows the style-context gap. It does not complete paragraph bidi/joining,
within-grapheme style changes, cross-face/size/language context or the other rows.
Known rectangular text/font-box clip recovery and exact mixed ActualText alignment
also exist; complex/image clips and broader source/structure recovery remain open.


The internal paragraph bidi foundation (2026-11-24) now passes complete original
Unicode 15 level/order conformance plus native wrapped-line observations. PDF
integration, directional shaping/L3/L4, logical extraction and paint/link/note
placement are unfinished. No acceptance row becomes complete from those algorithm
tests alone. Original Unicode/native observations remain reference fixtures.


Directional Hebrew/Arabic PDF output now has eleven native shaping vectors and
29 actual-output original-font/GID/GPOS/advance groups, independent C FriBidi L2
order/continuity, RTL ligature colors and URI/note bounds. Original scalars pass
checked SF recovery and fuzz regression cases. This narrows bidi/joining work;
full itemization, other scripts, cross-face/size/language and within-grapheme styles,
broader Word/real-export and PDF structure/note recovery remain unfinished. Native
and LibreOffice pages qualify only the authored example, with an explicit
RTL-space producer difference. No acceptance row becomes complete from it alone.


Native item-context qualification adds fourteen vectors/seven two-font PDF rows
across faces, sizes, languages and transparent controls. Actual HarfBuzz item
context is implemented; full itemization/cross-item positioning, broader scripts/
fonts, within-grapheme styles and the other acceptance rows remain unfinished.


Rich PDF running stories now share table/list/image/text measurement and reserve
body/note space. Independent tests verify eleven exact RGBA placements across
five native pages, merged/nested geometry, URI and real note destinations, source
immutability and declared SF/DOCX semantics. Six SF files/925 nodes pass actual
ProseMirror checks. Five native and three LibreOffice-rendered pages were inspected.
The letterhead producer preserves content on two pages; native layout uses four,
including a sparse note-only page. LibreOffice ignores the cell paragraph start
and initially omitted the oversized footer image in tight.docx. The shared DOCX
budget below now repairs that omission. Broader DOCX/pagination qualification, richer page/section settings, source-font/script/
recovery work and representative real exports remain required. No completion claim
is justified by the new native running-content feature alone.


DOCX running pictures now share a body budget, including nested/merged tables,
serial images, JPEG and nested body figures. The unchanged tight source fixture
now produces both visible contained images. Four new producer pages/seventeen
placements pass exact-pixel, geometry and source-DOCX relationship checks. Source
SF remains unchanged, and physical second round trips are exact. This narrows the
running-image gap without completing conservative font/producer/page qualification.
The tested exporter produces zero URI annotations even for an independent
python-docx control; expected targets remain intact and producer URI preservation
is explicitly unqualified. Asymmetric margins/distances/sections, remaining script/
source-font/PDF recovery/color and representative real-export evidence stay required.


Four-edge margins, explicit zero, running distances and landscape geometry now
survive source SF/DOCX and drive native body/story/table/note bounds. Nine native
and six producer pages have independent geometry/assets/relationship checks and
visual inspection; nine SF files/770 nodes pass ProseMirror. Current section
properties exclude history/deleted snapshots and diagnose unrepresented settings.
This does not complete page support: section-specific boundaries/stories, actual
signed overlap and advanced binding/page selection still require implementation/
qualification. Sparse native table/note packing and producer pagination differences
remain visible. The original full goal and all other unfinished rows remain intact.


Section containers now preserve rich source SF/DOCX boundaries and conditional
stories; native output exercises changed geometry, parity blank, inherited/blank
stories and section-context notes. Eight native and seven producer pages were
inspected; four exact RGBA placements and source relationships match. Producer
pagination/story differences, changed continuous shared-page frames and signed
overlap stay required. This does not complete the page/running-story row or the
full goal. See the latest STATUS and section evidence in verification.json.


Same-geometry continuous story changes now work, including local parity restart,
shared-page owner retention and future-story/body/note admission. Five native and
five producer pages were inspected; exact SF/DOCX, original rasters, merged cells,
all six table rows and actual URI/note destinations have independent checks.
Changed geometry and control/empty-section ownership remain required. Producer
parity/packing differs and is recorded separately. The full goal remains open.


Continuous margin/distance changes and changed paper now generate native output
with separate body frames/page ownership. Five native and five producer pages were
inspected; exact source SF/DOCX, three RGBA placements, URI/note destinations and
eight independent geometry controls pass. Paragraph/table/note continuations retain
their measured widths. Paper changes open a new native canvas; shared horizontal
margins apply immediately while vertical packing is conservative. Controls/empty
sections, signed overlap, broader bindings/packing and the rest of the original
acceptance rows remain unfinished; this is not full-goal completion.


Actual signed-margin running overlap now generates native output with fixed body
edges, independent fixed-story picture budgets and body/note foreground retention.
Five cases preserve source SF/DOCX and original RGBA through all four paths. Nine
native and seven producer pages were visually inspected; pixel probes, note/URI
relationships and recovered quads supplement textual and structural checks. Eighteen
prior nonnegative PDF streams/three DOCX packages are exact against 17c8b6c. Some
continuous fixed/minimum mixtures still lack a common envelope, and producer
compositing/pagination differs; broader ownership/packing, bindings, fonts/scripts,
PDF recovery/color and representative real-export work remain required.


Continuous story admission now handles individually valid fixed/minimum sections
without a shared envelope. Four examples preserve exact source DOCX SF and all
body/note content across thirteen native pages, with independent positions,
assets, note targets, foreground pixels and producer visual observations. Native
fresh-page groups preserve authored continuous starts; compatible output remains
exact against a pinned prior build. Control/empty ownership, conservative packing
and the remaining original acceptance requirements are still incomplete.


Page-control and empty-section ownership is now qualified for fourteen targeted
combinations: forty-five native pages, thirty-three URI annotations, six actual
note targets and 156 independently positioned recovered text spans. Standard
OOXML checks and exact source SF expose/fix the sectioned note-numbering omission;
forty-three inspected producer pages preserve body/note content with different
ownership/packing. These remain targeted synthetic controls, not representative
real-world export coverage. Broader bindings, corpus, font/script and PDF recovery
requirements remain open; the full objective is incomplete.


The original MQTT Word export now qualifies normalized DOCX/SF structure across
59 tables and all 3,009 origin cells plus 434 hyperlink ownership hashes. This
advances the real-corpus requirement without claiming four-path completion: its
paired PDF still fails tagged-span recovery, and native output still needs a
font for its supplementary CJK character. Source formatting/numbering and visual
normalization differences remain explicit. Original files/oracles are retained.


## 2026-11-24 Tagged spans: verified progress, incomplete goal

The original MQTT PDF now imports with every table/cell identity/role/row preserved.
Forty-one grids infer missing spans with separate provenance; eight empty continuation
tags survive on origins. Independent readers audit all memberships and 5,536 painted
cell sides. Page 89 remains uncertain with three visible-merge continuations still
represented as explicit cells. Broader source/font/style/link/content/asset recovery and
all remaining acceptance work stay required; see TAGGED_TABLE_SPANS.md.


## 2026-11-24 All original MQTT omitted-span grids recovered

All 42 omitted-span grids now recover from actual paint; 3,009 origins plus eleven
continuation references retain all 3,020 cell identities. Page 89 preserves seventeen
distinct axes and matches all 79 original Word-oracle span/content facts. Independent
readers check 5,852 sides and actual short-connector paint continuity. Other source
semantics, fonts/scripts, rendering and full corpus requirements remain open; see
TAGGED_TABLE_SPANS.md.


## 2026-11-24 Glyph font efficiency and real-source render evidence

The actual non-BMP Han font now produces a 464KB preview instead of 20.3MB, with exact
independent glyph/text/raster evidence. Six earlier pages retain exact commands/pixels
and all shaping/GID/GPOS oracles pass. The complete original source now renders 207
pages with explicit family/style mappings, but its URI audit fails three Unicode
fragments and widths/numbering/other source fidelity remain open. The full objective is
not complete; see FONT_SUBSETTING.md.


## 2026-11-24 Unicode URI failure resolved

The full original Word-source render now matches all 434 equivalent URI targets under
independent RFC mapping and raw ASCII checks. The old file still fails on three
targets/four invalid annotations. Eight all-story sample targets pass through all four
SF paths with visible repeated running labels and preserved note destinations. All 207
page commands/annotation rectangles are exact before/after. The goal remains incomplete:
source widths/numbering, internal destinations, broader source/recovery/corpus and codec
overhead remain required. See URI_TARGETS.md.


## 2026-11-24 Physical grid sizing milestone

Source widths now persist across SF/DOCX; PDF layout/continuation and measured-grid
recovery use them. Independent original XML proves all 59 grids, with the affected
17-axis table readable and 180.35pt description width. All four paths have width
evidence; four native and three independent DOCX-render pages were inspected.
Full-source output is 189 pages, not a Word-equivalence result. Source numbering,
internal destinations, broader corpus/recovery and codec/performance remain open.
See TABLE_WIDTHS.md.


## 2026-11-24 Numbering format/template milestone

Six formats, multilevel labels, logical counter contexts, nested edits and legal
numbering now persist through SF/DOCX and render in native rich containers. Ten
all-story markers agree with independent DOCX rendering; all 291 source heading
numbers match the publisher, with one original title hyphen spelling difference
recorded explicitly. Native output contains all 291 exact source headings.
This does not close internal destinations, broader source/recovery/corpus, rich
PDF semantic recovery or codec/performance work. See NUMBERING.md.
