# Active goal acceptance audit

Audited after rich citations, exact ActualText, original editor exports and nested note/wrapper qualification, 2026-11-24.
**The full goal remains incomplete.** This is the current requirement audit;
older milestone narratives are preserved in Git history and verification.json.
The objective remains all four rich-document paths around one shared SF, pure Go,
with independent structural/content/asset and visual evidence. Word-equivalent
layout and full UniOffice/UniPDF parity are excluded by the original objective.

## Requirement-by-requirement evidence

| Objective requirement | Current authoritative evidence | Assessment / unfinished work |
| --- | --- | --- |
| Establish an existing rich SF first, document alternatives/tradeoffs | STRUCTURED_FORMAT.md; structured.go; schema/prosemirror-v1.json; original corpus and full 205,918-node actual ProseMirror check in SF_CODEC.md | Established ProseMirror profile exists and is qualified; new actual Univer/Colleaf comparison is in COLLEAF_FORMAT.md |
| Inspect Colleaf's actual representation when available | Read-only GitHub inspection pinned to Colleaf 5ce2ef5; actual validator/editor/save source and matching npm schema inspected | Inspection fulfilled; executable pinned Colleaf/Univer validator/core/view/tree probes now qualify an internal UTF-16/container foundation. Private rich text/table/list/image/note/section/story mappings and actual facade save now have pinned reader evidence; corrected U+FFFC note ranges have glyph/placed-body evidence. Foreign automatic counters now have literal SF mapping and actual glyph/body/save evidence; Independent/rich body marks now have real source/glyph/edit/save proof; Per-citation literal/rich/default/empty marks now have actual glyph/edit/save proof; remaining intent, strict envelope/public API and full browser qualification remain unfinished |
| Rich manipulation, persistence and Colleaf integration | Typed Go tree/bridge, strict profile, plain/gzip SF, reference/asset validation; current native application contract now known | Core manipulation/persistence implemented. Private native workers now have actual reader/save and frame/note-placement checks; public adapter, remaining intent/settings and browser qualification remain unfinished |
| DOCX→SF | docx_read.go and scoped package/list/table/note/section/font code; unchanged original Word source/oracles; real_docx_test.go | Common required rich content preserved with explicit losses. Original PDF-guide→SF→Word now preserves exact editable structure/styles/assets within declared physical image precision (DOCX_PRECISION.md). Eight additional original editor exports now qualify notes/stories/review/fields in REAL_DOCX_CORPUS.md; no universal OOXML claim |
| SF→DOCX | docx_write.go and related modules; exact supported source SF round trips; python-docx/standard XML/LibreOffice checks and visual examples in verification.json | Implemented rich path; eight additional original exports and33 original/native/Word pages now qualify note/story/review/field output; producer limits remain explicit |
| SF→PDF wrapping/pagination/tables/fonts/Unicode | pdf_write/text/shaping/bidi/glyph/table/note modules; independent font/GID/geometry/pixel/continuation checks; 189-page real-source render | Implemented declared support contract. Unsupported shaping/missing glyphs explicitly error; no automatic font substitution. Do not turn this into an unstated requirement for every script or Word pagination |
| PDF→SF text/images/pages/positions/structure | pdf_read/geometry/images/marked/structure/infer/rulings modules; original 90 tables/3,009 origins/eleven continuation refs; PDF_NAVIGATION.md | Rich extraction exists. Explicit artifact properties/membership now recover in physical page/source records (PDF_ARTIFACTS.md). Explicit forward/reverse/repeated Note/Ref relationships now recover typed rich notes (PDF_NOTES.md); independently cited nested definitions and same-target ownership wrappers now qualify in PDF_NOTES.md; broader untagged inference remains required; rich body marks and atomic rich/mixed-direction citations are qualified in RICH_NOTE_LABELS.md and RICH_CITATIONS.md; common editable label backlinks are qualified in PDF_NOTES.md |
| Distinguish explicit/inferred/uncertain PDF meanings | SourceInfo; explicit logical roles separate from geometric/table/span/width/destination inference; original source identity/geometry oracles | Implemented for current recovery. Artifact page roles/properties now remain explicit, separate from authoring templates. Notes now separate explicit roles/edges from KindInferred reflow defaults. Rich/nested explicit note graphs are qualified; broader untagged recovery and inverse ParentTree consistency remain unqualified |
| Paragraphs/headings/formatting/links/lists | Shared model/profile, DOCX style/theme/slot/direction and numbering code; NUMBERING.md, URI_TARGETS.md, PDF_NAVIGATION.md; tests/independent source/render checks | Implemented for declared subset. Audit found ActualText dropped typed internal targets; current fix and regression tests close that concrete failure, with mixed ownership still diagnosed |
| Tables including merged cells and widths | table_grid, DOCX merges/grids, native recursive continuation/repetition, tagged/painted PDF recovery; TABLE_WIDTHS.md and TAGGED_TABLE_SPANS.md | Declared semantics implemented and independently qualified, including actual Word/PDF cells. Complex ambiguous grids must retain content and disclose uncertainty |
| Images/assets/metadata/page settings | Image decode/bounds/fitting, JPEG preservation, metadata, margins/sections/story ownership, multiple independent raster and relationship checks | Implemented current contract. Preserve practical source/asset qualification in final representative corpus; unsupported image variants have explicit outcomes |
| Headers/footers/notes | Shared SF, DOCX parts/references, native per-section variants and rich note continuation; page/control/image/note tests and visual evidence | Generation and DOCX round trips exist. Explicit running-artifact facts now recover separately from templates; explicit unambiguous note graphs now recover. Remaining note variants/inference and broader source qualification remain open; anonymous links alone do not fulfill them |
| Revisions/comments/fields/unsupported constructs have explicit behavior | DOCX final-visible-view and cached-field extraction, revision/comment/control loss diagnostics; COMPATIBILITY.md and fixed corpus tests | Explicit policy exists. Editing history/comment editing/field execution are not claimed; policies now verified on original Word/LibreOffice exports in REAL_DOCX_CORPUS.md; no arbitrary Office API parity claim |
| Never silently lose visible content; reject unsupported output | Diagnostics, AllowLossy gate, strict SF/JSON schema/graphs, staged DOCX/PDF output, explicit missing glyph/shape failures | Existing tests and source checks qualify current paths; every new adapter/recovery feature must preserve this invariant. Physical unsupported constructs cannot be called complete merely because flattened text survives |
| Supported SF round trips preserve declared semantics | Original 51 accepted corpus cases, rich feature tests and exact original Word normalization; gzip all-value hash and source identity proofs | Implemented checked normalized contract. Keep structural/source/asset oracles unchanged; new eight-source normalized round trips also pass; native adapter must be tested against actual snapshot semantics |
| Stable public Go API and all-four-path CLI/examples | document.go Read/Write/options/errors, structured tree APIs; report/sections/notes/destinations examples; atomic CLI and format tests | Existing API/CLI implemented. Document the native integration boundary and repeat end-to-end examples after missing semantics land |
| Everything runs in-process Go, no required CGO/executables/services/license server/uploads | go.mod/THIRD_PARTY.md; host CGO-disabled execution and Linux builds; no Daptin imports/service calls | Verified current production boundary. Optional Python/Node/LibreOffice are QA only; new adapter must preserve it |
| Independent readers plus appropriate visual inspection | Optional verifiers, actual ProseMirror, pypdf/MuPDF/fontTools/python-docx/XML, many rendered synthetic pages and real native samples | Strong targeted evidence, not a blanket fidelity claim. Representative original DOCX coverage now includes33 inspected producer/native pages; broader semantic/native integration qualification remains open |
| Representative corpus includes actual exports and targeted fixtures | testdata/corpus manifest; original Microsoft Word MQTT DOCX/PDF, published InDesign PDF, W3C PDF; pinned hashes/rights/oracles | Genuine originals present. Eight pinned Word/Mac Word/LibreOffice originals now cover notes/headers/revisions/fields with source-only oracles; retain their measured limits and original bytes |
| Tests/race/vet/malformed-input/CGO-disabled builds | verification.json and newest raw logs; strict archive/XML/PDF/SF/gzip cases, fuzz campaigns and source tests | Latest checked milestones pass. Required complete checks must run again after the remaining implementation, not substitute for missing feature evidence |
| Accurate recovery docs, limitations, commands, ongoing commits | STATUS, ARCHITECTURE, DECISIONS, COMPATIBILITY, feature contracts and verification.json; user-requested milestone commits | Keep current. Historical pending statements are not current evidence; this audit and STATUS are the current queue |

## Concrete contradictions and next implementation order

1. **ActualText navigation:** structural replacement previously copied href while
   dropping Run.Destination; the common-style intersection also ignored typed
   destination identity. The current fix retains matching internal targets and
   clears mixed URI/internal/different-target ownership. Single/multiple-run parent
   ActualText, SF, DOCX and intersection tests establish the corrected behavior.
2. **Explicit PDF story semantics:** the original zero-note Note/Reference/Ref
   probe now recovers a typed note. Rich/repeated/reverse-linked cases preserve
   label, ID bytes, style, list/table/image hierarchy/pixels and note-body navigation.
   KindInferred explicitly separates the endnote editorial default from source
   NoteType. PDF_NOTES.md records qualification and implemented different/empty citation and body labels,
   identified logical/physical inference and original guide cases, and still-required
   broader untagged graphs; independently cited nested definitions/Ref wrappers and exact nested/enclosing ActualText correspondence
   is now qualified in PDF_NOTES.md, while opaque replacements retain uncertainty. Rich/internal mixed-direction
   citations now have the independent evidence in RICH_CITATIONS.md. Artifact page roles and
   membership remain independently qualified without inventing running templates.
3. **Actual Colleaf boundary:** implement/qualify a standalone native Univer 1.0.4
   snapshot adapter through the shared SF, including rich structures and UTF-16
   ranges. Private exact-envelope/strict-JSON/Unicode and actual facade saved-envelope
   reimport now exist (COLLEAF_FORMAT.md); public native API/CLI, remaining mappings
   and browser/preset qualification are still unfinished. Dynamic literal-percent/
   digit-suffix and zero/legal native list templates now have real glyph/save/edit
   evidence; remaining marker appearance and browser controls are still open.
   Do not merely wrap plain text or rename SF. No Daptin or Colleaf edits
   are authorized by this standalone project goal; their access/quotas/save guards
   remain application responsibilities.
   Registered hyperlink URL-resource precedence and scoped main/running identities
   now have actual plugin save/edit/Go/SF evidence. Source-only external handler
   execution with window.open stubbed does not qualify browser interaction or
   full native navigation. Current bookmark/heading point data, typed targets and
   real selection-manager refresh requests now have save/edit/Go/SF evidence.
   Real table/image-start anchors now have nested/list/story save/edit/SF and
   exact selection-request evidence. List/section/page-break anchors now also have
   nested/story/empty-section save/edit/SF and selection evidence; soft-boundary
   point retention has a Go regression. Standalone AnchorPoint projection now has
   actual save/edit/SF/group/alias/carrier/text and selection-request evidence;
   foreign row/cell/terminator controls now have explicit normalized SF targets
   and original/normalized/edited native selection evidence. Nonmain lookup limits,
   document-level diagnostic/page/view evidence now survives actual native saves
   and edited target graphs. Per-node provenance/remaining intent and rendered caret/
   scroll/browser behavior remain required. Document-level diagnostic/page/view
   history now has actual save/edit/SF evidence and explicit unbound physical
   views after editable-target changes; native pagination remains independent.
   Paragraph origins now have actual save/edit/SF/provenance-pool evidence, typed
   historical/unbound states and ProseMirror qualification; full imported-PDF per-
   node source transport remains incomplete.
4. **Corpus/visual and final audit:** eight pinned, legally attributed original
   Word/Mac Word/LibreOffice inputs now qualify notes/running/review/field policies
   through source-only oracles, full semantic round trips, independent readers
   and33 visually inspected pages (REAL_DOCX_CORPUS.md). Preserve the positional-tab
   fix and explicit font/field/review/producer limits. Complete consistency/resource/
   API qualification, then repeat every requirement and full check.


Further source-structure consistency and allocation/editor costs remain explicit
qualification concerns. Do not escalate the contract into every PDF drawing,
image codec, script or Word layout rule; the required standard is the stated rich
features, declared support contract, meaningful structure recovery and explicit
uncertainty/omissions. No required feature in the queue may be excused merely by
labeling it unsupported.

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


Native note-definition/label provenance and KindInferred now have synthetic actual
save/edit/identity-detachment and SF evidence. Paragraph provenance survives body-
mark prefix projection without false staling. Run/citation/container/cell/item source
bindings, marker appearance/nonvisual intent, public API/CLI and full native/browser/
corpus/final audits remain required; this milestone does not complete that queue.


Native nonempty run/link/rich-body-mark/citation-source transport now has synthetic
actual save/split/edit/reference-detachment and exact SF evidence. ProseMirror
source-mark attribute coverage now includes historical and optional table facts.
Zero-width anchor/container/cell/item source bindings and the remaining native
intent/API/UI/corpus/final audits remain required.
