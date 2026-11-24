# Active goal acceptance audit

Audited after acknowledged DOCX precision, sectioned note fix and original-guide Word qualification, 2026-11-24.
**The full goal remains incomplete.** This is the current requirement audit;
older milestone narratives are preserved in Git history and verification.json.
The objective remains all four rich-document paths around one shared SF, pure Go,
with independent structural/content/asset and visual evidence. Word-equivalent
layout and full UniOffice/UniPDF parity are excluded by the original objective.

## Requirement-by-requirement evidence

| Objective requirement | Current authoritative evidence | Assessment / unfinished work |
| --- | --- | --- |
| Establish an existing rich SF first, document alternatives/tradeoffs | STRUCTURED_FORMAT.md; structured.go; schema/prosemirror-v1.json; original corpus and full 205,918-node actual ProseMirror check in SF_CODEC.md | Established ProseMirror profile exists and is qualified; new actual Univer/Colleaf comparison is in COLLEAF_FORMAT.md |
| Inspect Colleaf's actual representation when available | Read-only GitHub inspection pinned to Colleaf 5ce2ef5; actual validator/editor/save source and matching npm schema inspected | Inspection fulfilled; earlier local-only unavailability claim corrected. Native .vdocs interoperability is missing, not implied by SF |
| Rich manipulation, persistence and Colleaf integration | Typed Go tree/bridge, strict profile, plain/gzip SF, reference/asset validation; current native application contract now known | Core manipulation/persistence implemented. A checked native snapshot adapter/actual reader qualification is still needed for direct Colleaf integration |
| DOCX→SF | docx_read.go and scoped package/list/table/note/section/font code; unchanged original Word source/oracles; real_docx_test.go | Common required rich content preserved with explicit losses. Original PDF-guide→SF→Word now preserves exact editable structure/styles/assets within declared physical image precision (DOCX_PRECISION.md). Broader representative DOCX qualification remains needed; no universal OOXML claim |
| SF→DOCX | docx_write.go and related modules; exact supported source SF round trips; python-docx/standard XML/LibreOffice checks and visual examples in verification.json | Implemented rich path; broaden original-export visual/corpus coverage beyond current synthetic combinations and single original Word publication |
| SF→PDF wrapping/pagination/tables/fonts/Unicode | pdf_write/text/shaping/bidi/glyph/table/note modules; independent font/GID/geometry/pixel/continuation checks; 189-page real-source render | Implemented declared support contract. Unsupported shaping/missing glyphs explicitly error; no automatic font substitution. Do not turn this into an unstated requirement for every script or Word pagination |
| PDF→SF text/images/pages/positions/structure | pdf_read/geometry/images/marked/structure/infer/rulings modules; original 90 tables/3,009 origins/eleven continuation refs; PDF_NAVIGATION.md | Rich extraction exists. Explicit artifact properties/membership now recover in physical page/source records (PDF_ARTIFACTS.md). Explicit forward/reverse/repeated Note/Ref relationships now recover typed rich notes (PDF_NOTES.md); rich multi-node/internal mixed-direction labels, nested graphs and broader untagged inference remain required; common editable label backlinks are qualified in PDF_NOTES.md |
| Distinguish explicit/inferred/uncertain PDF meanings | SourceInfo; explicit logical roles separate from geometric/table/span/width/destination inference; original source identity/geometry oracles | Implemented for current recovery. Artifact page roles/properties now remain explicit, separate from authoring templates. Notes now separate explicit roles/edges from KindInferred reflow defaults. Heterogeneous/nested note graphs and inverse ParentTree consistency remain unqualified |
| Paragraphs/headings/formatting/links/lists | Shared model/profile, DOCX style/theme/slot/direction and numbering code; NUMBERING.md, URI_TARGETS.md, PDF_NAVIGATION.md; tests/independent source/render checks | Implemented for declared subset. Audit found ActualText dropped typed internal targets; current fix and regression tests close that concrete failure, with mixed ownership still diagnosed |
| Tables including merged cells and widths | table_grid, DOCX merges/grids, native recursive continuation/repetition, tagged/painted PDF recovery; TABLE_WIDTHS.md and TAGGED_TABLE_SPANS.md | Declared semantics implemented and independently qualified, including actual Word/PDF cells. Complex ambiguous grids must retain content and disclose uncertainty |
| Images/assets/metadata/page settings | Image decode/bounds/fitting, JPEG preservation, metadata, margins/sections/story ownership, multiple independent raster and relationship checks | Implemented current contract. Preserve practical source/asset qualification in final representative corpus; unsupported image variants have explicit outcomes |
| Headers/footers/notes | Shared SF, DOCX parts/references, native per-section variants and rich note continuation; page/control/image/note tests and visual evidence | Generation and DOCX round trips exist. Explicit running-artifact facts now recover separately from templates; explicit unambiguous note graphs now recover. Remaining note variants/inference and broader source qualification remain open; anonymous links alone do not fulfill them |
| Revisions/comments/fields/unsupported constructs have explicit behavior | DOCX final-visible-view and cached-field extraction, revision/comment/control loss diagnostics; COMPATIBILITY.md and fixed corpus tests | Explicit policy exists. Editing history/comment editing/field execution are not claimed; verify policy on the added real rich-export inputs rather than requiring arbitrary Office API parity |
| Never silently lose visible content; reject unsupported output | Diagnostics, AllowLossy gate, strict SF/JSON schema/graphs, staged DOCX/PDF output, explicit missing glyph/shape failures | Existing tests and source checks qualify current paths; every new adapter/recovery feature must preserve this invariant. Physical unsupported constructs cannot be called complete merely because flattened text survives |
| Supported SF round trips preserve declared semantics | Original 51 accepted corpus cases, rich feature tests and exact original Word normalization; gzip all-value hash and source identity proofs | Implemented checked normalized contract. Keep structural/source/asset oracles unchanged; test new native adapter against its actual snapshot semantics |
| Stable public Go API and all-four-path CLI/examples | document.go Read/Write/options/errors, structured tree APIs; report/sections/notes/destinations examples; atomic CLI and format tests | Existing API/CLI implemented. Document the native integration boundary and repeat end-to-end examples after missing semantics land |
| Everything runs in-process Go, no required CGO/executables/services/license server/uploads | go.mod/THIRD_PARTY.md; host CGO-disabled execution and Linux builds; no Daptin imports/service calls | Verified current production boundary. Optional Python/Node/LibreOffice are QA only; new adapter must preserve it |
| Independent readers plus appropriate visual inspection | Optional verifiers, actual ProseMirror, pypdf/MuPDF/fontTools/python-docx/XML, many rendered synthetic pages and real native samples | Strong targeted evidence, not a blanket fidelity claim. Representative original DOCX output visual coverage and new semantic/native integration qualification remain open |
| Representative corpus includes actual exports and targeted fixtures | testdata/corpus manifest; original Microsoft Word MQTT DOCX/PDF, published InDesign PDF, W3C PDF; pinned hashes/rights/oracles | Genuine originals present. Broaden real DOCX editor/authoring-feature coverage, especially notes/headers/revisions/fields; retain originals rather than generate an oracle from manydoc output |
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
   heterogeneous/mixed-direction label layout, nested/ActualText graphs and broader untagged recovery. Artifact page roles and
   membership remain independently qualified without inventing running templates.
3. **Actual Colleaf boundary:** implement/qualify a standalone native Univer 1.0.4
   snapshot adapter through the shared SF, including rich structures and UTF-16
   ranges. Do not merely wrap plain text or rename SF. No Daptin or Colleaf edits
   are authorized by this standalone project goal; their access/quotas/save guards
   remain application responsibilities.
4. **Corpus/visual and final audit:** add pinned, legally reusable original DOCX
   sources covering contrasting producer/notes/running/review/field use; qualify
   current supported structures/assets and generated output through independent
   readers and meaningful visual samples. Inspect/fix real failures, retain their
   original oracles, then repeat every requirement and complete check.

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
