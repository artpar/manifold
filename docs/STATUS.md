# Current status and recovery

Updated 2026-11-24. Read this first, then AGENTS.md, ARCHITECTURE.md and DECISIONS.md.
Commit verified milestones with corresponding KT/evidence updates, as requested
by the user. Generated documents, binaries, source downloads and QA environments
remain ignored. Git history and verification.json retain the historical milestones;
this file and ACCEPTANCE.md are the current authoritative queue.

## Outcome and current contract

The full rich-document goal is **active/incomplete**. The standalone Go library
and CLI implement DOCX→SF, SF→DOCX, SF→PDF and PDF→SF around a versioned established
ProseMirror JSON profile, manydoc-prosemirror/1. Legacy model JSON/text remain
available; .sf.gz is standard gzip around the same SF, not another schema.

Required paragraphs/headings/formatting, links/internal destinations, lists,
merged/nested/continued/repeated tables with widths, embedded images, metadata,
section/page settings, conditional running content and rich foot/endnotes exist in
the shared model and generation paths. PDF import retains text/images/physical
pages/positions, forward logical structure and separately identified geometric
inference, source styles and physical navigation views. Read COMPATIBILITY.md for
the declared support contract, explicit losses and output rejection. No Word-
equivalent layout or full UniOffice/UniPDF API parity is claimed or required.

Production is pure Go, with no CGO/executables/services/license servers/customer
uploads. Daptin identity/access/quotas/storage/actions remain outside the library.
Proprietary UniDoc implementations have not been copied or translated; no formal
clean-room claim is made. go.mod/go.sum and THIRD_PARTY.md track reused components.

## Actual Colleaf inspection changes the integration evidence

The earlier local-only search missed the remotely available application. Read-only
GitHub inspection now pins artpar/colleaf at
5ce2ef507b28b92f5b03443ade87bc0e17f3d8b6 and matching published Univer 1.0.4 types.
Its .vdocs is a version-3 wrapper around IDocumentData with indexed streams,
source tables/drawings/stories/notes and plugin resources. SF cannot be renamed
into that native format. A checked standalone adapter through SF is now public; broader native rendering
and browser qualification remain open.
Read COLLEAF_FORMAT.md for exact primary sources, comparison and mapping queue.
No Colleaf or Daptin code was modified; app persistence/authorization is not part
of the adapter. Native snapshot budgets do not become library quota gates.

## Latest progress: actual native browser probe exposes note-order failure

The full pinned Colleaf/Univer desktop mount now builds in an isolated optional
QA harness. The unmodified actual mount/validator are hash-checked and all vendor
presets are pinned at 1.0.4. Browser readiness reaches lifecycle 3 with a canvas and
no reported errors. No application or production dependency is changed.

This is not a passing full-browser qualification: the unchanged public notes
example displays footnote 2 above footnote 1. Main indices first cite note-2 at 36,
note-3 at 127 and repeat note-2 at 147. The renderer retains the last index per
note, so its actual placement order is reversed. The browser page and an actual
headless body-order checker both reproduce it. A separate no-repeat control has
the correct order. The original example and expected first-reference order remain.

```sh
node scripts/prepare_colleaf_browser.cjs
python3 -m http.server 8768 --bind 127.0.0.1 --directory output/colleaf-browser
# Current native acceptance failure, not a passing smoke test:
node scripts/verify_colleaf_note_order.cjs
```

COLLEAF_BROWSER.md records source pins, controls and remaining work. Native browser
edit/save/reopen/mobile and wider paint/layout remain unqualified. No safe data-level
ordering fix is established yet; do not patch the applications, weaken the oracle,
or replace live citations with cached bodies. Other goal work can still progress.
The full Go checks below remain passing, but do not execute this native checker.
Goal active/incomplete; no full completion or blocked-state claim is made.

## Earlier progress: bounded PDF attribute graph traversal

A small acyclic attribute-array graph could amplify work through shared indirect
references despite the existing depth-16 guard. The new document-wide 200,000-work-
entry budget counts expanded value/class/member visits across all owners and
checks cancellation while traversing. Overflow rejects without a partial document.
Class/local override order, supported attribute semantics and revision diagnostics
are unchanged. No public API/SF schema, output layout or dependency changes.

The new regressions reproduced successful over-budget imports and ignored
cancellation before the fix. They now pass; a valid shared graph still preserves
local Roman numbering over class Decimal, its literal/source label, start/body
and SF structure. Both amplification fixtures stay below 5,000 source bytes;
fanout eight/depth six expands to 262,144 leaf visits. See PDF_RESOURCES.md.

```sh
GOWORK=off go test -run 'TestPDFAttribute|TestPDFPublished' -count=1 .
GOWORK=off go test -run '^$' -fuzz '^FuzzPDFAttributeGraph$' -fuzztime=10s -parallel=2 .
```

Final evidence is being recorded. The goal remains active/incomplete; this closes
the concrete unbounded attribute-walk gap, not general PDF resource safety or the
remaining native/semantic/reflow/API/visual acceptance audits.

Full tests pass (library 27.044s/CLI 0.534s), full race passes (317.245s/3.178s),
vet/modules and CGO-disabled focused tests (1.969s), host CLI build/help and Linux
amd64/arm64 cross-builds pass. Bounded graph fuzz passes 4,062 executions/10.537s/
two workers (short safety sample). The original InDesign SF is byte-identical to
its preceding-milestone result; independent source/DOCX/PDF checks pass. Details
and hashes are in verification.json and PDF_RESOURCES.md. Goal active/incomplete.

## Earlier progress: reciprocal PDF ParentTree audit

PDF import now cross-checks present inverse ParentTree entries against forward
page/Form MCIDs and whole-object annotation ownership. Number-tree keys/ranges,
optional Limits, repeated/cyclic nodes, owner types, container-key uniqueness and
array/scalar entry shape are validated. Contradictory ownership rejects; missing
index/container/slot information and inverse-only entries have distinct diagnostics
while preserving forward content/order. The index never invents additional content.
Absent indexes do not prevent forward recovery or certify reciprocal ownership.

Bounds are explicit: depth 64, 200,000 index nodes/key entries and 400,000 combined
index/owner slots. This is scoped defensive validation, not exhaustive PDF resource
safety or conformance certification. Reused Form/page MCID zero values remain
separate; annotation StructParent uses a scalar owner. No public SF/API shape,
production dependency or output-layout behavior changes.

Independent pypdf walks the unchanged original Copyright Office InDesign export:
413 forward elements and 550 reciprocal content owners agree. Generated notes
independently yield 40 elements and 24 reciprocal owners. Mutations substitute
another existing owner while preserving all decoded page content streams; CLI
imports must reject their contradictions. The original real-source SF validates
8,872 actual ProseMirror nodes. Existing source/oracle bytes are unchanged.

```sh
output/qa-venv/bin/python scripts/verify_pdf_parent_tree.py testdata/real/copyright-office-circular-1-2021.pdf
output/qa-venv/bin/python scripts/verify_pdf_parent_tree.py output/colleaf-tagged/document.pdf --out output/pdf-parent-tree/generated-mutated.pdf
GOWORK=off go test -run 'TestPDFParentTree|TestGeneratedPDF|TestPDFTagged' -count=1 .
```

Full tests pass (library 26.800s/CLI cached; preceding CLI 0.863s), full race
passes (317.351s/3.184s), vet/modules and CGO-disabled focused tests (0.656s),
host CLI execution and Linux amd64/arm64 cross-builds pass. Independent combined
tagged-structure and original-publication conversion verifiers pass. The latter's
original 16-URI oracle is unchanged; typed internal destinations now have a separate
source GoTo/name assertion. No output-layout change or new visual claim is made.
Final focused race passes (1.825s); bounded index fuzz passes 24,063 executions
in 11.459s/two workers (short safety sample). Object/xref injection is excluded
from this fixed-forward-fixture target. verification.json records commands/hashes.
The full goal remains
active/incomplete: broader source/resource/untagged recovery, custom/literal-list
representation, native rendering/settings/browser, note policies and final
semantic/reflow/API/visual acceptance remain open.

## Earlier progress: editable PDF list numbering

Tagged PDF Roman and Latin alphabetic lists now recover as editable ordered lists
when one canonical sequence reproduces every displayed label. Starting values,
case, repeated-letter alphabetic passes, two-digit decimal padding and consistent
literal prefixes/suffixes persist through SF and DOCX. Standard explicit format
families select parsing; start/pattern inference is separately diagnosed and
recorded in source evidence. Absent/None format attributes retain conservative
decimal/bullet inference. Observed nested prefixes remain literal; no hidden
ancestor-counter relationship is invented.

Padded decimal output now declares Decimal and legal numbering declares its actual
rendered decimal family. Markers containing links, bookmarks, notes or non-text
content stay literal paragraphs with diagnostics, preserving their content and
relationships. Marker typography may normalize on reflow and is diagnosed. Mixed
punctuation retains the existing decimal normalization policy; unsupported,
noncanonical, ambiguous and out-of-range sequences are not assigned false counters.
This is explicit partial recovery, not arbitrary label/locale/template fidelity.

Six PDF→SF→DOCX cases preserve 18 editable items, body style/URI relationships,
source labels and starts/templates/formats. Independent pypdf/SF/OOXML checks
confirm actual PDF attributes/rendered labels and emitted Word numbering. The
original iv./Body source fixture is unchanged: the former unsupported-stage test
now requires a typed Roman list, original label and inference disclosure. Existing
physical nested-marker strings are unchanged and inspected using the explicit
physical view; separate logical tests verify nested item structure/context.

```sh
MANYDOC_PDF_LIST_NUMBERING_QA=output/pdf-list-numbering GOWORK=off go test -run TestGeneratedPDFNumberedListSemantics -count=1 .
output/qa-venv/bin/python scripts/verify_pdf_list_numbering.py
NODE_PATH=output/sf-qa/node_modules node scripts/verify_structured.cjs output/pdf-list-numbering/*.sf
GOWORK=off go test -run 'TestPDFListNumbering|TestGeneratedPDFNumbered|TestPDFNestedCounter|TestPDFCounterAnnotations|TestPDFPictorialCounter' -count=1 .
```

Full tests pass (library 26.969s/CLI 0.477s), vet/modules and CGO-disabled
host/Linux amd64/arm64 builds pass. Final full race passes (318.303s/3.164s); the final guarded paths also pass
focused race (2.018s) and CGO-disabled tests (0.423s).
Canonical-pattern fuzz passes 53,792 executions/11.466s/two workers (short safety
sample). Actual ProseMirror validates 66 nodes across six SFs; diff check passes.
verification.json records commands/scope. Goal active/incomplete:
remaining literal/custom list representation, PDF inverse/source/resource and
reflow audits, native rendering/intent/browser, note policies and final acceptance
remain open. No new production dependency, backend or permission gate is added.

## Earlier progress: painted PDF structure and typed-note recovery

PDF output now emits a PDF 2.0 logical tree tied to actual page-scoped MCIDs,
MCRs, reciprocal parent-tree entries and named structure IDs. Paragraphs/headings,
list/item/label/body groups, table rows/cells/spans, image Alt and foot/endnote
roles describe current rendered content. Typed note definitions and citation Ref
edges are reciprocal. Global note IDs are retained; scoped collisions are qualified
once, with an IDTree, without growing prefixes on reflow. No SF/body/text cache is
embedded. Blank citations have empty structural owners rather than dangling paint
references. Running content and repeated headers/continued labels are artifacts.

The previously failing three-note CLI oracle is unchanged and now passes in every
standard run; its environment gate is removed. New Go tests check rich/default/
empty labels, repeated/continued/scoped citations, exact body content and ownership,
merged tables including documents without notes, list starts, formatting/URI,
image pixels/Alt, source roles/regions and caller immutability. Original physical
geometry/glyph/image/header oracles keep their expected values and explicitly use
PDFSkipStructure (plus PDFSkipInference where needed). Header-version checks now
require 2.0 for the emitted PDF 2.0 namespace; this is an intentional format upgrade.

ReadOptions.PDFSkipStructure and CLI -pdf-skip-structure expose the positioned
view without applying the logical tree. Combine with PDFSkipInference / the CLI
flag for paint-order inspection; pdf_structure_skipped discloses the choice.
Default tagged recovery remains logical; older tagged/untagged inputs still read.

Independent pypdf checks actual marked-content balance, 24 contiguous MCIDs,
forward/inverse page ownership, structure parents, IDs, three FENotes/two footnotes/
one endnote and four reciprocal citation edges. MuPDF checks both example pages
pixel-identical at 1.5x to output generated from commit 1d13d3c; Poppler renders of
both pages were visually inspected, with aligned labels, notes and footer text.
Independent recursive/repeated-header examples additionally cover 12 pages, seven
FENotes total across all three cases, and table/row/cell-span/image/marker ownership.
This is targeted structural/visual evidence, not PDF/UA/WTPDF conformance, full
Word fidelity or completion of all PDF/native/source/resource acceptance.

```sh
GOWORK=off go test -run '^TestGeneratedPDF|^FuzzGeneratedPDFNoteGraph$' -count=1 .
GOWORK=off go test -run 'TestCLIVDOCSSemantics/document.sf/pdf|TestCLIPDFPhysicalInspection' -count=1 ./cmd/manydoc
GOWORK=off go run ./examples/colleaf -out output/colleaf-tagged
# Recreate the independent pre-tagging baseline (fresh detached checkout):
git worktree add --detach output/tagged-baseline-1d13d3c 1d13d3c
MANYDOC_QA_OUT="$PWD/output/colleaf-baseline-1d13" GOWORK=off sh -c 'go -C output/tagged-baseline-1d13d3c run ./examples/colleaf -out "$MANYDOC_QA_OUT"'
git worktree remove output/tagged-baseline-1d13d3c
output/qa-venv/bin/python scripts/verify_generated_pdf_structure.py
```

Full tests pass (library 27.124s/CLI 0.838s), race passes (315.451s/3.200s),
vet/modules, CGO-disabled focused tests 0.605s, host build/help/logical and physical
CLI extraction and Linux amd64/arm64 cross-builds pass. Generated-note fuzz passes
951 executions/10.547s/two workers (short safety sample). ProseMirror checks 43
recovered nodes/marks/attributes. verification.json records exact QA and scopes.
Goal active/incomplete:
remaining native rendering/intent/browser, note policies/decorations, general
untagged recovery, broader source/resource and final acceptance audits stay open.

## Public native integration evidence to preserve

VDOCS, ReadVDOCS, WriteVDOCS, VDOCSMIME and .vdocs CLI dispatch expose the
checked offline Colleaf version-3/Univer 1.0.4 profile through SF. Public/external-
package/CLI/example tests cover rich graphs, assets, source facts, wrong envelopes,
I/O, cancellation, immutability and atomic failures. Native transport retains
source/diagnostic history without a lossy-output gate; unsupported mappings reject.

Actual pinned validator/plugins/facade save/reopen probes qualify rich containers,
notes/stories, languages, lists, links, anchors and owner-specific source metadata.
The original InDesign PDF preserves its full SF graph through two actual saves:
153 paragraph/8,651 run/67 list-item origins and 8,872 ProseMirror nodes. Checked
lossless metadata compression reduces its native envelope from 11 MiB to 3.6 MiB,
passing the unchanged app validator. Five original DOCX language graphs also
survive actual saves/edits/removal, with 516 ProseMirror nodes over 18 artifacts.
These are component/offline transport results; native browser behavior and broader
rendering/settings intent remain unqualified. See COLLEAF_FORMAT.md and the durable
verification milestones. No application code or admission/authorization was changed.

Earlier chronological progress and resolved failures remain in Git history and
verification.json, rather than contradictory current instructions here. In
particular, the old generated-PDF zero-note failure is fixed by painted structure
and enforced unconditionally; the public native adapter is already implemented.

## Existing verified evidence to preserve

- Original Word MQTT: 59 tables/756 rows/3,009 origins, all grid widths/spans/cell
  hashes, 291 headings and 4,998 nonempty body paragraphs; 3,106 bookmarks/379
  visible internal IDs and independently audited 435 Word URI/text target groups.
- Original 137-page PDF: all 90 page-local tables, 3,009 origins plus eleven merge
  continuation identities; all 42 omitted-span grids recovered from actual paint.
  All 325 anonymous views/1,245 annotation targets and transformed regions match
  independent original PDF objects. The InDesign publication's one named view/19
  annotations and native example's physical views also match. See PDF_NAVIGATION.md.
- Native full Word-source render: 189 pages, original JPEG/metadata/required Unicode,
  qualified supplied/built-in fonts and subsetting; all 291 heading labels/titles
  audited, with one original title hyphen spelling difference recorded explicitly.
  Named-navigation changes preserve all page commands and annotation rectangles.
- Plain compact SF and explicit gzip retain every original JSON value. Original
  477,613,781-byte SF persists as 8,880,248-byte .sf.gz; normalized hash matches all
  provenance/assets/relationships. Actual ProseMirror validates all 205,918 nodes
  and marks/attributes. Old/read/optimized/gzip text hashes match; full tree memory
  and editor cost remain material. See SF_CODEC.md for exact measurements/limits.
- Fixed original corpus and rich synthetic cases qualify supported SF semantics,
  fonts/shaping/bidi, tables/pagination, images, stories/notes and losses through
  independent reader/geometry/asset/pixel/visual checks. These do not substitute
  for the remaining semantic/native/source/resource and final acceptance work.

Latest scoped check results appear above and in verification.json. Linux builds
establish compilation, not Linux execution. Optional Node/Python/LibreOffice remain
QA only. Historical codec timings are retained in the verification ledger.

## Next concrete implementation order

1. Reproduce and resolve the native repeated-footnote body-order failure in
   COLLEAF_BROWSER.md while preserving the source graph and original oracle.
   Broaden source/structure and resource audits beyond the newly qualified
   reciprocal ParentTree checks, using malformed and independently authored sources.
   Retain current physical and explicit/inferred source oracles; never recover from an embedded SF cache.
2. Qualify remaining native rendering/intent/settings and browser/preset behavior
   against the pinned Colleaf/Univer packages. The public offline VDOCS adapter,
   rich mappings, source persistence and plugin saves already exist; do not repeat
   their implementation or modify the applications.
3. Complete the remaining note-policy, custom/literal-list representation and
   broader untagged/source/reflow audits. Record exact scope and reproducible
   failures rather than treating a current rejection as completed capability.
4. Preserve the original corpus and independent expectations, refresh the full
   acceptance matrix and visual/API/resource checks, and complete the goal only
   after every original criterion is proven. No external blocker is established.

```sh
cat docs/ACCEPTANCE.md docs/COLLEAF_BROWSER.md docs/COLLEAF_FORMAT.md
rg -n 'ParentTree|StructParent|MCID|OBJR' pdf_structure*.go pdf_marked.go
GOWORK=off go test -run 'TestGeneratedPDF|TestPDFTagged|TestCLIVDOCSSemantics' -count=1 ./...
output/qa-venv/bin/python scripts/verify_pdf_parent_tree.py testdata/real/copyright-office-circular-1-2021.pdf
rg -n 'ErrUnsupported|requires mapping' colleaf_*.go
```

## Code map and recovery commands

| Area | Main authority |
| --- | --- |
| API/model/dispatch | document.go, source.go, destinations.go, notes.go, sections.go |
| SF/codec/schema | structured.go, structured_sections.go, structured_encode.go, schema/prosemirror-v1.json |
| DOCX | docx_read/write, docx_lists, docx_table/widths, docx_notes, docx_sections/running, docx_destinations |
| PDF recovery | pdf_read/geometry/images/marked/structure/links/navigation_read/infer/rulings/text_style |
| Native layout | pdf_write/text/shaping/bidi/glyph/font/notes/lists/cell_layout/table_continue |
| Source/QA | testdata/real and corpus manifests/oracles, scripts, examples, verification.json |
| Current contracts | ACCEPTANCE.md, COMPATIBILITY.md, STRUCTURED_FORMAT.md, COLLEAF_FORMAT.md, feature evidence docs |

```sh
GOWORK=off go test ./... -count=1
GOWORK=off go test -race ./... -count=1
GOWORK=off go vet ./...
GOWORK=off go mod verify
GOWORK=off CGO_ENABLED=0 go build -o bin/manydoc ./cmd/manydoc
GOWORK=off GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o bin/manydoc-linux-amd64 ./cmd/manydoc
git diff --check
```

Use extract -format sf or sf.gz explicitly; extract defaults to text irrespective
of output filename. Keep pdfcpu configuration explicit rather than using a global
constructor that creates user files/exits. Per-renderer font buffers must remain
owned because subsetting mutates them. Source font names are page/object scoped.
Do not discard visible content or revise a fixture oracle to obtain a green test.
