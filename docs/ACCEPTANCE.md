# Active goal acceptance audit

Audited after native transport, painted PDF structure and counter recovery, 2026-11-24.
**The full goal remains incomplete.** This is the current requirement audit;
older milestone narratives are preserved in Git history and verification.json.
The objective remains all four rich-document paths around one shared SF, pure Go,
with independent structural/content/asset and visual evidence. Word-equivalent
layout and full UniOffice/UniPDF parity are excluded by the original objective.

## Requirement-by-requirement evidence

| Objective requirement | Current authoritative evidence | Assessment / unfinished work |
| --- | --- | --- |
| Establish an existing rich SF first, document alternatives/tradeoffs | STRUCTURED_FORMAT.md; structured.go; schema/prosemirror-v1.json; original corpus and full 205,918-node actual ProseMirror check in SF_CODEC.md | Established ProseMirror profile exists and is qualified; new actual Univer/Colleaf comparison is in COLLEAF_FORMAT.md |
| Inspect Colleaf's actual representation when available | Read-only GitHub inspection pinned to Colleaf 5ce2ef5; actual validator/editor/save source and matching npm schema inspected | Inspection fulfilled; executable pinned Colleaf/Univer validator/core/view/tree probes now qualify an internal UTF-16/container foundation. Private rich text/table/list/image/note/section/story mappings and actual facade save now have pinned reader evidence; corrected U+FFFC note ranges have glyph/placed-body evidence. Foreign automatic counters now have literal SF mapping and actual glyph/body/save evidence; Independent/rich body marks now have real source/glyph/edit/save proof; Per-citation literal/rich/default/empty marks now have actual glyph/edit/save proof; strict native envelopes and per-owner sources are checked. Five original DOCX language graphs and one original PDF full-source graph now pass actual plugin saves. The offline VDOCS API/CLI is now public with external-package/CLI/example tests. Remaining native rendering/intent, full preset/browser and broader corpus qualification remain unfinished |
| Rich manipulation, persistence and Colleaf integration | Typed Go tree/bridge, strict profile, plain/gzip SF, reference/asset validation; current native application contract now known | Core manipulation/persistence implemented. Private native workers now have actual reader/save and frame/note-placement checks; actual source graphs and language metadata now persist through pinned saves/edits. The checked offline public adapter exists; remaining rendering/intent/settings and browser qualification remain unfinished |
| DOCX→SF | docx_read.go and scoped package/list/table/note/section/font code; unchanged original Word source/oracles; real_docx_test.go | Common required rich content preserved with explicit losses. Original PDF-guide→SF→Word now preserves exact editable structure/styles/assets within declared physical image precision (DOCX_PRECISION.md). Eight additional original editor exports now qualify notes/stories/review/fields in REAL_DOCX_CORPUS.md; no universal OOXML claim |
| SF→DOCX | docx_write.go and related modules; exact supported source SF round trips; python-docx/standard XML/LibreOffice checks and visual examples in verification.json | Implemented rich path; eight additional original exports and33 original/native/Word pages now qualify note/story/review/field output; producer limits remain explicit |
| SF→PDF wrapping/pagination/tables/fonts/Unicode | pdf_write/text/shaping/bidi/glyph/table/note modules; independent font/GID/geometry/pixel/continuation checks; 189-page real-source render | Implemented declared support contract. Unsupported shaping/missing glyphs explicitly error; no automatic font substitution. Do not turn this into an unstated requirement for every script or Word pagination |
| PDF→SF text/images/pages/positions/structure | pdf_read/geometry/images/marked/structure/infer/rulings modules; original 90 tables/3,009 origins/eleven continuation refs; PDF_NAVIGATION.md | Rich extraction exists. Explicit artifact properties/membership now recover in physical page/source records (PDF_ARTIFACTS.md). Explicit forward/reverse/repeated Note/Ref relationships now recover typed rich notes (PDF_NOTES.md); independently cited nested definitions and same-target ownership wrappers now qualify in PDF_NOTES.md; reciprocal untagged graphs and rich same-page continuation now qualify in PDF_UNTAGGED_NOTES.md and PDF_NOTE_CONTINUATION.md; the named cross-page/long-note case now passes in PDF_NOTE_PAGES.md; broader source/kind and full acceptance audit remain; rich body marks and atomic rich/mixed-direction citations are qualified in RICH_NOTE_LABELS.md and RICH_CITATIONS.md; common editable label backlinks are qualified in PDF_NOTES.md |
| Distinguish explicit/inferred/uncertain PDF meanings | SourceInfo; explicit logical roles separate from geometric/table/span/width/destination inference; original source identity/geometry oracles | Implemented for current recovery. Artifact page roles/properties now remain explicit, separate from authoring templates. Notes now separate explicit roles/edges from KindInferred reflow defaults. Rich/nested explicit note graphs are qualified; present reciprocal page/Form/annotation ownership is now checked with original-source and malformed-input evidence (PDF_PARENT_TREE.md); broader untagged/resource/source qualification remains open |
| Paragraphs/headings/formatting/links/lists | Shared model/profile, DOCX style/theme/slot/direction and numbering code; NUMBERING.md, URI_TARGETS.md, PDF_NAVIGATION.md; tests/independent source/render checks | Implemented for declared subset. Audit found ActualText dropped typed internal targets; current fix and regression tests close that concrete failure, with mixed ownership still diagnosed |
| Tables including merged cells and widths | table_grid, DOCX merges/grids, native recursive continuation/repetition, tagged/painted PDF recovery; TABLE_WIDTHS.md and TAGGED_TABLE_SPANS.md | Declared semantics implemented and independently qualified, including actual Word/PDF cells. Complex ambiguous grids must retain content and disclose uncertainty |
| Images/assets/metadata/page settings | Image decode/bounds/fitting, JPEG preservation, metadata, margins/sections/story ownership, multiple independent raster and relationship checks | Implemented current contract. Preserve practical source/asset qualification in final representative corpus; unsupported image variants have explicit outcomes |
| Headers/footers/notes | Shared SF, DOCX parts/references, native per-section variants and rich note continuation; page/control/image/note tests and visual evidence | Generation and DOCX round trips exist. Explicit running-artifact facts now recover separately from templates; explicit unambiguous note graphs now recover. Remaining note variants/inference and broader source qualification remain open; anonymous links alone do not fulfill them |
| Revisions/comments/fields/unsupported constructs have explicit behavior | DOCX final-visible-view and cached-field extraction, revision/comment/control loss diagnostics; COMPATIBILITY.md and fixed corpus tests | Explicit policy exists. Editing history/comment editing/field execution are not claimed; policies now verified on original Word/LibreOffice exports in REAL_DOCX_CORPUS.md; no arbitrary Office API parity claim |
| Never silently lose visible content; reject unsupported output | Diagnostics, AllowLossy gate, strict SF/JSON schema/graphs, staged DOCX/PDF output, explicit missing glyph/shape failures | Existing tests and source checks qualify current paths; every new adapter/recovery feature must preserve this invariant. Physical unsupported constructs cannot be called complete merely because flattened text survives |
| Supported SF round trips preserve declared semantics | Original 51 accepted corpus cases, rich feature tests and exact original Word normalization; gzip all-value hash and source identity proofs | Implemented checked normalized contract. Keep structural/source/asset oracles unchanged; new eight-source normalized round trips also pass; native adapter must be tested against actual snapshot semantics |
| Stable public Go API and all-four-path CLI/examples | document.go Read/Write/options/errors, structured tree APIs; report/sections/notes/destinations examples; atomic CLI and format tests | Existing API/CLI implemented. VDOCS public API/CLI/example now exposes the checked offline native profile; browser and remaining mappings stay open. The generated-PDF three-note recovery check now passes unconditionally with painted logical structure; broader final acceptance remains open |
| Everything runs in-process Go, no required CGO/executables/services/license server/uploads | go.mod/THIRD_PARTY.md; host CGO-disabled execution and Linux builds; no Daptin imports/service calls | Verified current production boundary. Optional Python/Node/LibreOffice are QA only; new adapter must preserve it |
| Independent readers plus appropriate visual inspection | Optional verifiers, actual ProseMirror, pypdf/MuPDF/fontTools/python-docx/XML, many rendered synthetic pages and real native samples | Strong targeted evidence, not a blanket fidelity claim. Representative original DOCX coverage now includes33 inspected producer/native pages; broader semantic/native integration qualification remains open |
| Representative corpus includes actual exports and targeted fixtures | testdata/corpus manifest; original Microsoft Word MQTT DOCX/PDF, published InDesign PDF, W3C PDF; pinned hashes/rights/oracles | Genuine originals present. Eight pinned Word/Mac Word/LibreOffice originals now cover notes/headers/revisions/fields with source-only oracles; retain their measured limits and original bytes |
| Tests/race/vet/malformed-input/CGO-disabled builds | verification.json and newest raw logs; strict archive/XML/PDF/SF/gzip cases, fuzz campaigns and source tests | Latest checked milestones pass. Required complete checks must run again after the remaining implementation, not substitute for missing feature evidence |
| Accurate recovery docs, limitations, commands, ongoing commits | STATUS, ARCHITECTURE, DECISIONS, COMPATIBILITY, feature contracts and verification.json; user-requested milestone commits | Keep current. Historical pending statements are not current evidence; this audit and STATUS are the current queue |

## Current implementation and qualification queue

- The ActualText typed-navigation defect is fixed: matching internal targets
  persist and mixed URI/internal/different-target ownership is diagnosed.
- Generated PDF now contains painted structure and reciprocal typed notes. The
  unchanged three-note CLI acceptance check runs unconditionally. Explicit source
  note graphs, rich labels/citations and running-artifact facts are qualified;
  broader untagged note/source/reflow cases remain in PDF_NOTES.md.
- Present inverse ParentTree validation now checks page/Form/annotation ownership.
  Original-source reciprocal graphs and wrong-owner mutations qualify this gap;
  expanded XObject visits now reject document-wide amplification while preserving
  valid repeated placements (PDF_RESOURCES.md); broader source/resource audit
  remains required. See PDF_PARENT_TREE.md.
- The public offline native adapter, rich graph transport, language/source metadata
  and registered-plugin saves already exist. Remaining native settings/rendering,
  marker appearance and full-preset/browser behavior stay unqualified. Read
  COLLEAF_FORMAT.md for exact mappings and source probes; do not reimplement the
  existing adapter or modify Daptin/Colleaf.
- Eight additional original Word/Mac Word/LibreOffice exports qualify notes,
  running content, reviews and fields through source-only oracles, full semantic
  round trips and 33 visually inspected pages (REAL_DOCX_CORPUS.md). Preserve their
  actual source bytes, positional-tab fix and measured producer/font/field limits.
- Finish the scoped consistency/resource/semantic-reflow/API/visual audit, then
  repeat the complete matrix and required checks. Component successes alone do
  not close the full goal.

Allocation/editor costs remain explicit qualification concerns. Do not escalate
the contract into every PDF drawing, image codec, script or Word layout rule; the
required standard is the stated rich features, declared support contract,
meaningful structure recovery and explicit uncertainty/omissions. Required work
cannot be excused merely by labeling it unsupported.

## Reproduce implemented probes and remaining cases

```sh
output/qa-venv/bin/python scripts/generate_pdf_semantic_audit.py
bin/manydoc extract -format sf -o output/audit/explicit-note.sf output/audit/explicit-note.pdf
bin/manydoc extract -format sf -o output/audit/pagination-artifact.sf output/audit/pagination-artifact.pdf
rg -n 'Ref|Note|Artifact|Subtype|BBox' pdf_structure.go pdf_structure_blocks.go pdf_marked.go
```

The generated source fixtures are independent hand-authored object syntax. Current
Note/Ref probes now recover typed notes with separately identified kind uncertainty;
artifact records express Header properties and linked content. Broader note cases
remain required as listed in PDF_NOTES.md. See COLLEAF_FORMAT.md for
pinned remote schema evidence. The complete goal stays active, with no external
blocker preventing the next implementation.


Earlier chronological component evidence and resolved failures remain in Git
history and verification.json. The current table/queue above supersede their
historical unfinished-work statements.

## Current generated-PDF structure acceptance result

The zero-note failure discovered in the preceding milestone is fixed. Its source
and three-note/two-footnote/one-endnote expectation are unchanged, and the test now
runs unconditionally. Actual painted MCIDs/forward MCR/inverse ParentTree, reciprocal
Ref, IDs/IDTree and PDF2 FENote/NoteType describe rendered content without a cached
SF body. Standalone merged-table and richer note/body/list/image/formatting/source
tests cover more than successful parsing/text. Empty-mark fuzz seeds remain intact.

Independent pypdf validates actual paint/tree ownership and three definitions/four
citations; MuPDF confirms two pages pixel-identical to the pre-tagging commit,
with Poppler visual inspection. Physical oracles retain expected values and select
PDFSkipStructure explicitly; default reads verify the logical graph separately.
No PDF/UA/WTPDF conformance or full goal completion follows from this component
evidence. Remaining native/browser, note policies, broader untagged/source/resource
and final acceptance work stays open.

Independent source-SF-driven recursive-table and repeated-header checks also pass:
7 and5 pages, respectively, with table/row/cell-span/image/citation ownership and
original literal marker counts. Only the two-page notes example has the fresh
pre-tagging pixel baseline and visual inspection in this milestone.


Latest PDF numbering evidence: six explicit-family generated cases preserve 18
editable items, source labels, starts/templates/formats and body style/URI through
SF and DOCX. Independent pypdf/SF/OOXML checks verify actual attributes, labels
and numbered paragraphs; a nested case preserves observed literal context without
inventing ancestry. Canonical bounds/ambiguity/unsupported alphabet and annotated/
pictorial label guards retain source content with explicit losses. The original
Roman fixture remains unchanged and now has stronger positive structural/source
expectations. Literal/custom marker representation and broader inverse/source/
resource/native/reflow/final audits remain open.


Latest inverse-ownership evidence: unchanged real InDesign source has 413 forward
elements/550 reciprocal owners; generated notes have 40/24. Independent pypdf
confirms them and creates wrong-owner mutations with unchanged decoded page content
streams. CLI must reject those contradictions. Go tests cover page/Form scopes,
annotations, missing/null/extra entries, malformed shape/ranges/owners/cycles and
resource bounds. Missing reciprocal metadata is disclosed without dropping forward
content. This closes the specific ignored-ParentTree gap, not general source or
resource safety, accessibility conformance or full goal acceptance.


Latest resource evidence closes a concrete attribute-graph amplification gap:
small shared arrays and cross-owner classes now stop at a document-wide expanded
work budget, and traversal observes cancellation. A valid shared graph preserves
local Roman-over-Decimal behavior and exact body/label/source/SF evidence. Original
source bytes and oracles remain unchanged. See PDF_RESOURCES.md; broader resource
and final acceptance remain unproven.


Actual desktop preset browser qualification now has a reproducible repeated-note
placement failure. The unchanged public notes example places label 2 above label 1;
the pinned renderer uses the later repeated citation index for the first note.
The source identity/content graph still persists, but that does not prove correct
body order. COLLEAF_BROWSER.md and scripts/verify_colleaf_note_order.cjs preserve
the original first-reference-order oracle and passing no-repeat control. Ordinary
Go tests do not run this optional native-reader check, which currently FAILS.
No application/vendor/production change or full browser/fidelity claim is made.


Fixed labels under explicit ordered PDF families now retain L/LI hierarchy as
constant templates when canonical counter parsing fails. Hidden start/restart
intent remains unobserved, with a diagnosed unused start-one default and separate
source evidence. Four cases/eight items have actual PDF/SF/OOXML/Word display and
body-style/link checks, including empty markers without phantom painted content.
NUMBERING.md records commands, source/physical-oracle policy and remaining variable/
rich-label limitations. Native repeated-note browser placement remains unresolved.


Literal-marker list extraction/persistence now retains explicit PDF L/LI/Lbl
ownership, variable/rich labels, relationships and assets through ListItem.Marker
and the additive list_marker SF node. Unknown kind/counter intent is diagnosed.
PDF now renders literal labels through borderless parallel measured flows with
actual Lbl/LBody paint ownership, rich nested blocks, notes and pagination. Explicit
block-bearing labels remain literal on recovery, including numeric/bullet text.
DOCX now retains rich literal labels in checked borderless layout cells, with
sequential label/body ranges in notes to avoid nested-table omission. Mixed automatic
counters use real start overrides; item page starts sit before layout cells.
VDOCS now generates checked native label/body cells and recovers actual content. Existing
source/text/pixel oracles remain unchanged while structural assertions strengthen.
LITERAL_LIST_MARKERS.md records the schema, evidence and next implementation.


Public VDOCS literal-marker generation now preserves actual editable label/body
cells, source owners and real automatic counters. Native list indentation retains
measured counter advance; custom foreign indents still diagnose. SDK edits/saves,
actual desktop/mobile saves, image pixels and96 unique label/body tokens across
six native pages pass scoped checks. See NATIVE_LITERAL_MARKERS.md for exact
commands, original counter regression evidence and wider qualification limits.
The full acceptance goal remains active; no general native/Word fidelity is claimed.


Expanded PDF XObject execution now has a document-wide200,000-visit work bound.
Compact acyclic/shared-across-page/appearance sources reject atomically; valid
transformed repeated paint retains exact geometry/source/SF. Independent pypdf
counts the source graphs and original publication oracles still pass. This closes
one concrete resource gap, not the full source/resource or completion audit.
The original pinned-native repeated-footnote placement oracle still fails; no
completion waiver or overall blocked state follows. See PDF_RESOURCES.md and
COLLEAF_BROWSER.md.


CID-width resource/source qualification now avoids repeated range expansion and
converts shared numeric arrays once per font, retaining all CID values, content,
positions and SF while observing cancellation. Independent MuPDF overlap differences
are explicitly diagnosed and marked estimated; the strict original agreement oracle
remains executable. The non-overlapping control and original publication agree with
independent expectations. PDF_FONT_WIDTHS.md gives the scoped evidence. Decoded-
stream/resource and full original-goal audits remain open.


Syntax-stream resource/integrity audit now bounds every page/Form/appearance/
ToUnicode filter stage and cumulative decoded/interpreted work. Original bad-checksum
acceptance is fixed; public negative probes reject atomically. Twelve raw/filtered/
predictor/LZW-transition source graphs match exactly and MuPDF independently confirms
bytes/positions. Original publication SF remains identical. The pinned pypdf long
EarlyChange0 limitation retains a strict failing oracle; no blanket reader/fidelity
claim follows. Images/initial-parser resources and the full acceptance audit remain
open. See PDF_SYNTAX_STREAMS.md.


Image-stage/source qualification now bounds image, stencil, soft-mask and Indexed
lookup expansion, checks JPEG headers before decoding and retains packed/16-bit
RGB/gray samples and soft alpha. Exact pixels/placements/SF, public-stage rejection,
independent readers and original publication/MQTT complete-SF equality qualify the
milestone. CMYK managed color and JPEG chroma interpolation differences retain
strict oracles. See PDF_IMAGE_DECODING.md and verification.json. Initial parser
resources and the full acceptance audit remain open; the goal is active/incomplete.


Retained initial-stream integrity/ownership now has exact source and independent
full-SF/geometry evidence: ten positive sources, nine atomically rejected invalid
sources and original publication/MQTT equality. Earlier dependency allocations
remain outside these guards. The audit exposed direct Encrypt dictionaries in
MuPDF object-stream exports as a concrete library compatibility defect. At that
milestone the independent-reader password cases succeeded while the library strict
oracle failed. The following direct-encryption milestone resolves it while keeping
its original source/full-SF oracle. No overall blocked state or completion exception follows.


The direct Encrypt defect identified by the parser audit is now fixed. The same
four original ciphertexts pass exact full-SF expectations with pure-Go password
handling. Rich ordinary/hybrid/stream-xref, metadata/font/URI/raster/geometry and
reader/owner/negative-password cases have independent evidence, including actual
MuPDF saves. Strict original repair-shape reader differences remain executable.
See PDF_DIRECT_ENCRYPTION.md and verification.json. Return to the original broader
source/untagged-note/reflow/native/API/visual acceptance work; this milestone does
not complete the active goal or remove the initial-parser allocation concern.
