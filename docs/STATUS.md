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
into that native format. A checked standalone adapter through SF remains open.
Read COLLEAF_FORMAT.md for exact primary sources, comparison and mapping queue.
No Colleaf or Daptin code was modified; app persistence/authorization is not part
of the adapter. Native snapshot budgets do not become library quota gates.

## Latest progress: section, break and standalone-point sources

Private section metadata, single-hard-break paragraph properties and the first
bookmark of a standalone-point group now carry manydoc.flow-origin/version1 refs.
A deterministic evidence.flowOrigins pool stores original SourceInfo and source-free
SF semantic hashes, without section/body/group caches. Current layout/content and
point groups stay authoritative. Changed semantics retain historical facts; missing
owners archive facts unbound. All native references validate even if current
composition does not consume an owner. Soft rendered boundaries do not acquire
hard-break provenance, and break metadata in mixed text does not assert ownership.

Pinned actual plugins/validator/skeleton check nine sources across two sections,
hard break, main/running/note standalone groups and aliases; two saves/reopens,
styled emoji, TextX edit, alias rename, section-binding removal and hard-to-soft
boundary snapshots. The fixture has3 current native pages with a hard break and2
with the soft boundary; original source pages never dictate that geometry. Strict
.vdocs/Go/SF compares current structures/layout/source states/diagnostics and stable
repeated round trips. ProseMirror checks117 nodes/marks/attributes. This is synthetic
headless component evidence, not browser or full imported-PDF qualification.

SF encoding now explicitly rejects source facts attached to identityless empty text
rather than silently normalizing them away. Detached facts can be retained through
Document.UnboundSources / SF unboundSources; named points and empty paragraphs have
separate supported owners. A regression checks rejection, input immutability and
the explicit unbound alternative. This closes the previously undocumented empty-run
source-loss case without inventing text or point identities.

Final tests16.196s/CLI0.694s, vet/modules, CGO-disabled focused tests0.525s, host
CLI/--help and Linux amd64/arm64 cross-builds pass. Flow-scanner fuzz passes112133
executions/11.005s, a short safety sample. Full race passes172.597s/CLI1.754s.
verification.json records scopes and limitations. Goal active/incomplete:
remaining appearance/nonvisual intent, public native API/CLI, full-preset/browser,
nonmain lookup limits, per-page/decorated notes and broader PDF/source/resource/final
acceptance audits remain required. Other source bindings still need combined real-
source qualification. Production stays pure Go; no application/backend change.

```sh
MANYDOC_COLLEAF_FLOW_ORIGIN_QA=output/colleaf-qa/flow-origins.json GOWORK=off go test -run 'TestColleafFlowOrigins|TestStructuredIdentityless' -count=1 .
node scripts/verify_colleaf_flow_origins.cjs
MANYDOC_COLLEAF_FLOW_ORIGIN_SAVED=output/colleaf-qa/flow-origins-saved.json MANYDOC_COLLEAF_FLOW_ORIGIN_EDITED=output/colleaf-qa/flow-origins-edited.json MANYDOC_COLLEAF_FLOW_ORIGIN_RENAMED=output/colleaf-qa/flow-origins-renamed.json MANYDOC_COLLEAF_FLOW_ORIGIN_DETACHED=output/colleaf-qa/flow-origins-detached.json MANYDOC_COLLEAF_FLOW_ORIGIN_SOFT=output/colleaf-qa/flow-origins-soft.json GOWORK=off go test -run TestColleafFlowOrigins -count=1 .
NODE_PATH=output/sf-qa/node_modules node scripts/verify_structured.cjs output/colleaf-qa/flow-origins-saved.json.sf output/colleaf-qa/flow-origins-edited.json.sf output/colleaf-qa/flow-origins-renamed.json.sf output/colleaf-qa/flow-origins-detached.json.sf output/colleaf-qa/flow-origins-soft.json.sf
rg -n 'ErrUnsupported|requires mapping|annotations|EastAsiaLanguage' colleaf_*.go
```

## Earlier progress: sourced inline anchors

Private native bookmark properties now carry manydoc.anchor-origin/version1 source
references. evidence.anchorOrigins stores original SourceInfo and source-free hashes
of anchor identity plus paragraph context, with no text/context cache. Current
point names/positions/relationships stay authoritative. Paragraph context is a
conservative qualification: surrounding content edits can mark an unchanged anchor
historical; no original geometry is asserted as current native positioning.
Missing inline owners archive source facts unbound, including a marker changed to
block placement. Every native reference validates even if not consumed as an
inline owner; formatted anchor atoms still reject.

Note-body marker projection captures anchor context before its temporary prefix,
so unchanged rich body marks do not stale anchor provenance. Source-only binding
removal does not stale other anchors. Seven synthetic sources across main/list/
cell/header/footer/note paths pass actual pinned save/reopen, typed point lookup,
emoji style, TextX shift and moved/renamed/detached snapshot checks. Strict .vdocs/
Go/SF compares positions, names, source states and repeated stability; ProseMirror
checks157 nodes/marks/attributes. Negative tests cover bad pools, contradictory
inline metadata and changed-owner references. The old anchor-source rejection
regression is narrowed to still-unmapped identityless empty text, because named
sourced anchors now have an implemented positive round-trip contract.

Final tests16.536s/CLI0.355s, vet/modules, CGO-disabled focused tests0.495s, host
CLI/--help and Linux amd64/arm64 cross-builds pass. Bookmark scanner fuzz passes
102382 executions/11.128s, a short safety sample. Full race passes171.870s/
CLI2.132s. verification.json records scopes and limitations. Goal active/incomplete: remaining container
sources (sections/breaks/standalone points), identityless empty-text source policy,
appearance/nonvisual intent, public native API/CLI, full-preset/browser and broader
PDF/source/resource/final acceptance audits remain. No external blocker exists.

```sh
MANYDOC_COLLEAF_ANCHOR_ORIGIN_QA=output/colleaf-qa/anchor-origins.json GOWORK=off go test -run TestColleafAnchorOrigins -count=1 .
node scripts/verify_colleaf_anchor_origins.cjs
MANYDOC_COLLEAF_ANCHOR_ORIGIN_SAVED=output/colleaf-qa/anchor-origins-saved.json MANYDOC_COLLEAF_ANCHOR_ORIGIN_EDITED=output/colleaf-qa/anchor-origins-edited.json MANYDOC_COLLEAF_ANCHOR_ORIGIN_MOVED=output/colleaf-qa/anchor-origins-moved.json MANYDOC_COLLEAF_ANCHOR_ORIGIN_RENAMED=output/colleaf-qa/anchor-origins-renamed.json MANYDOC_COLLEAF_ANCHOR_ORIGIN_DETACHED=output/colleaf-qa/anchor-origins-detached.json GOWORK=off go test -run TestColleafAnchorOrigins -count=1 .
NODE_PATH=output/sf-qa/node_modules node scripts/verify_structured.cjs output/colleaf-qa/anchor-origins-saved.json.sf output/colleaf-qa/anchor-origins-edited.json.sf output/colleaf-qa/anchor-origins-moved.json.sf output/colleaf-qa/anchor-origins-renamed.json.sf output/colleaf-qa/anchor-origins-detached.json.sf
rg -n 'Source != nil|Source:' colleaf_tables.go colleaf_sections.go colleaf_standalone_points.go
```

## Earlier progress: list and item source bindings

Private direct leading-paragraph properties now carry manydoc.list-origin and
manydoc.item-origin/version1 references in manydocListOrigin / manydocItemOrigin.
A deterministic evidence.listOrigins pool stores original SourceInfo and source-
free SF fingerprints, without list/item bodies. Ownership follows current list
composition, not old offsets or a cached tree. List semantics include the whole
current list; item semantics include its subtree, parent numbering attrs and ordinal
index. Changed content/numbering/ownership qualifies source facts historical;
missing current owners archive facts unbound. Original physical facts remain
independent of current native geometry.

Sources on unmarked paragraphs do not become list ownership; inherited style
metadata is diagnosed without ownership. All native references validate before
restoration, even if current composition does not consume their markers. Private
markers clear before return. Source-only edits do not falsely stale parent/item
semantics. Item hashes encode only the selected subtree plus small parent context,
not all sibling bodies for every item.

Pinned actual plugins/validator/skeleton check24 source records across10 nested,
independent, restarted, bullet, zero, cell, header/footer and note lists; styled
emoji; two saves/reopens; TextX edits; native marker restart2/3 to9/10; list/item
reference removal. Strict .vdocs/Go/SF compares exact content, numbering, sources,
diagnostics and repeated stability. ProseMirror checks326 nodes/marks/attributes.
Go also covers missing/inherited/contradictory ownership, bullet removal, malformed
pools and source-free bullet-item context. This is synthetic headless component
qualification, not full imported-PDF/native/browser fidelity.

Final tests16.697s/CLI0.758s, vet/modules, CGO-disabled focused tests0.544s, host
CLI/--help and Linux amd64/arm64 cross-builds pass. Metadata-scanner fuzz passes
115916 executions/10.795s, a short safety sample. Full race passes171.728s/
CLI1.804s. verification.json records scopes and limitations. The full goal remains
active/incomplete. Next implement other container and zero-width anchor sources,
then remaining appearance/nonvisual intent, public native API/CLI, full-preset/
browser and broader PDF/source/resource/final acceptance audits. Production stays
pure Go, with no application/backend change or external blocker.

```sh
MANYDOC_COLLEAF_LIST_ORIGIN_QA=output/colleaf-qa/list-origins.json GOWORK=off go test -run TestColleafListOrigins -count=1 .
node scripts/verify_colleaf_list_origins.cjs
MANYDOC_COLLEAF_LIST_ORIGIN_SAVED=output/colleaf-qa/list-origins-saved.json MANYDOC_COLLEAF_LIST_ORIGIN_EDITED=output/colleaf-qa/list-origins-edited.json MANYDOC_COLLEAF_LIST_ORIGIN_RESTARTED=output/colleaf-qa/list-origins-restarted.json MANYDOC_COLLEAF_LIST_ORIGIN_DETACHED=output/colleaf-qa/list-origins-detached.json MANYDOC_COLLEAF_LIST_ORIGIN_ITEM_DETACHED=output/colleaf-qa/list-origins-item-detached.json GOWORK=off go test -run TestColleafListOrigins -count=1 .
NODE_PATH=output/sf-qa/node_modules node scripts/verify_structured.cjs output/colleaf-qa/list-origins-saved.json.sf output/colleaf-qa/list-origins-edited.json.sf output/colleaf-qa/list-origins-restarted.json.sf output/colleaf-qa/list-origins-detached.json.sf output/colleaf-qa/list-origins-item-detached.json.sf
rg -n 'Source != nil|manydocOrigin' colleaf_navigation.go colleaf_tables.go colleaf_sections.go
```

## Earlier progress: image source bindings and mirror consistency

Private direct drawing descriptors now carry manydoc.image-origin/version1 source
references; evidence.imageOrigins stores deduplicated original SourceInfo and
source-free SF fingerprints, without an asset/body cache. Ordinary main/cell/list/
header/footer/note images restore source ownership after current asset and fitted
geometry composition. Changed assets, descriptions or presentation retain source
facts as historical with a diagnostic. Missing current bindings archive sources
unbound. Current bytes/description/geometry stay authoritative; source evidence
never restores old images or claims current PDF coordinates.

The registered DOC_DRAWING_PLUGIN mirror must agree with direct source descriptors,
including provenance references. The new SDK probe edits both representations before
actual plugin saves, checks mirror equality and unchanged source pools, and Go rejects
an intentionally altered mirror reference. Seven synthetic sources span embedded
PNG and original JPEG assets, main drawing frames and styled emoji, two saves/
reopens, resized/replaced assets and binding detachment. Strict .vdocs/Go/SF verifies
full structures/assets/source states and repeated stability; ProseMirror checks96
nodes/marks/attributes. Go also qualifies fitted explicit-section/running/note image
sources and malformed records. This is headless component evidence, not browser
bitmap paint or full imported-PDF source/native qualification.

Final tests16.832s/CLI0.799s, vet/modules, CGO-disabled focused tests0.684s,
host CLI/--help and Linux amd64/arm64 cross-builds pass. Metadata-scanner fuzz
passes102291 executions/10.482s, a short safety sample. Full race passes172.525s/
CLI2.142s. verification.json records scopes and limitations. The full goal remains
active/incomplete. Next implement list/item/other container and zero-width anchor
sources, then remaining appearance/nonvisual intent, public native API/CLI, full-
preset/browser and broader PDF/source/resource/final acceptance audits. Production
stays pure Go, with no external blocker or application/backend change.

```sh
MANYDOC_COLLEAF_IMAGE_ORIGIN_QA=output/colleaf-qa/image-origins.json GOWORK=off go test -run TestColleafImageOrigins -count=1 .
node scripts/verify_colleaf_image_origins.cjs
MANYDOC_COLLEAF_IMAGE_ORIGIN_SAVED=output/colleaf-qa/image-origins-saved.json MANYDOC_COLLEAF_IMAGE_ORIGIN_RESIZED=output/colleaf-qa/image-origins-resized.json MANYDOC_COLLEAF_IMAGE_ORIGIN_ASSET=output/colleaf-qa/image-origins-asset.json MANYDOC_COLLEAF_IMAGE_ORIGIN_DETACHED=output/colleaf-qa/image-origins-detached.json GOWORK=off go test -run TestColleafImageOrigins -count=1 .
NODE_PATH=output/sf-qa/node_modules node scripts/verify_structured.cjs output/colleaf-qa/image-origins-saved.json.sf output/colleaf-qa/image-origins-resized.json.sf output/colleaf-qa/image-origins-asset.json.sf output/colleaf-qa/image-origins-detached.json.sf
rg -n 'Source != nil|manydocOrigin' colleaf_lists.go colleaf_navigation.go colleaf_tables.go
```

## Earlier progress: table and cell source bindings

Private native tableSource descriptors and origin tableCells now carry version1
manydoc.table-origin / manydoc.cell-origin references. evidence.tableOrigins stores
a deterministic deduplicated pool of SourceInfo and source-free SF fingerprints,
without a body/grid cache. Table and cell fingerprints use different semantic
node shapes, including when both owners share identical original source facts.
Restoration follows final native composition; unchanged owners retain sources,
content/grid/presentation changes mark source facts historical with a diagnostic,
and missing current owners archive facts unbound. Original geometry and table
width/span evidence remain source facts, never current native geometry.

Every descriptor reference is validated, including covered cells with no SF owner.
A valid marker moved to a covered cell archives its original source; a missing or
contradictory record rejects. Native covered bodies must still be empty. Dropping
only a cell binding does not stale the table's semantic source fingerprint. Existing
paragraph/run/note provenance remains independent; input snapshots stay immutable.

Actual pinned plugins/validator/skeleton check thirteen sources across five merged/
nested/main/header/footer/note tables, reference-only properties, styled emoji,
two saves/reopens, actual cell TextX insertion, detached cell binding and covered-
cell placement. Strict .vdocs/Go/SF compares full structures, widths/spans, source
states and repeated stability; ProseMirror checks165 nodes/marks/attributes. Go
also checks malformed records, missing covered references and shared source facts.
This is synthetic headless component evidence; imported-PDF/browser/final native
qualification remains incomplete.

Final all-package tests16.175s/CLI0.719s, vet/modules, CGO-disabled focused tests
0.487s, host CLI/--help and Linux amd64/arm64 cross-builds pass. Descriptor-scanner
fuzz passes101024 executions/10.487s, a short safety sample. Final full race passes
170.568s/CLI1.749s. verification.json records scopes and limitations. The full goal remains active/
incomplete; image/list/item/other container/anchor sources, remaining appearance/
nonvisual intent, public native API/CLI, full-preset/browser and broader final audits
remain required. No external blocker exists; production stays pure Go.

```sh
MANYDOC_COLLEAF_TABLE_ORIGIN_QA=output/colleaf-qa/table-origins.json GOWORK=off go test -run TestColleafTableOrigins -count=1 .
node scripts/verify_colleaf_table_origins.cjs
MANYDOC_COLLEAF_TABLE_ORIGIN_SAVED=output/colleaf-qa/table-origins-saved.json MANYDOC_COLLEAF_TABLE_ORIGIN_EDITED=output/colleaf-qa/table-origins-edited.json MANYDOC_COLLEAF_TABLE_ORIGIN_DETACHED=output/colleaf-qa/table-origins-detached.json MANYDOC_COLLEAF_TABLE_ORIGIN_COVERED=output/colleaf-qa/table-origins-covered.json GOWORK=off go test -run TestColleafTableOrigins -count=1 .
NODE_PATH=output/sf-qa/node_modules node scripts/verify_structured.cjs output/colleaf-qa/table-origins-saved.json.sf output/colleaf-qa/table-origins-edited.json.sf output/colleaf-qa/table-origins-detached.json.sf output/colleaf-qa/table-origins-covered.json.sf
rg -n 'Source != nil|manydocOrigin' colleaf_images.go colleaf_lists.go colleaf_navigation.go
```

## Earlier progress: run and citation source bindings

Private whole-document native composition now carries source references in direct
winning text-style intervals, using manydoc.run-origin/version1 and a deterministic
source/fingerprint pool in evidence.runOrigins. Ordinary nonempty text, links,
main/list/cell/header/footer/note bodies, rich body marks, rich citation label runs
and rich/default citation atoms retain their SourceInfo after final composition.
Current text/styles/relationships stay authoritative. Changed or split fragments
retain original facts as historical with an explicit diagnostic; disappearance of
all current owners archives facts unbound. Inherited defaults and overwritten text
records do not assert ownership. Contradictory inline metadata and invalid profile/
version/digest/missing/duplicate records reject.

Paragraph fingerprints now exclude nested run sources, so a source-only edit does
not falsely stale paragraph semantics. Projected default citation labels do not
acquire a duplicate source owner, and internal origin markers do not turn uniform
citations into rich labels. Actual ProseMirror exposed a source-mark declaration
omission: historical plus optional table_widths/table_spans are now declared, with
a regression comparing every public SourceInfo field against the schema.

Pinned actual plugins/validator/skeleton qualify eleven synthetic run sources,
reference-only properties/no text cache, styled emoji, two saves/reopens, TextX
interval splitting and rich-citation replacement, and reference removal. Strict
.vdocs/Go/SF compares exact fragments/styles/source states and stable repeated
round trips. ProseMirror checks110 nodes/marks/attributes. Negative tests also
cover inherited/overwritten ownership, contradictions and unsupported zero-width
empty-text/anchor sources. BodyLabelStyle.Source remains invalid in the shared
model; use Note.LabelSource for that provenance. This is headless component
qualification, not full imported-PDF source transport or browser fidelity.

Final tests16.228s (CLI/internal cached), vet/modules, CGO-disabled focused tests
0.535s, host CLI/--help and Linux amd64/arm64 cross-builds pass. The unchanged
origin-profile parser passed108862 fuzz executions/11.456s, a short safety sample.
Final full race passes170.284s (CLI/internal cached). verification.json records
check scopes and limitations.
The goal remains active/incomplete. Next implement zero-width anchor and table/
image/container/cell/item source bindings, then remaining appearance/nonvisual
intent, public native API/CLI, full-preset/browser and the broader final audit.

```sh
MANYDOC_COLLEAF_RUN_ORIGIN_QA=output/colleaf-qa/run-origins.json GOWORK=off go test -run TestColleafRunOrigin -count=1 .
node scripts/verify_colleaf_run_origins.cjs
MANYDOC_COLLEAF_RUN_ORIGIN_SAVED=output/colleaf-qa/run-origins-saved.json MANYDOC_COLLEAF_RUN_ORIGIN_EDITED=output/colleaf-qa/run-origins-edited.json MANYDOC_COLLEAF_RUN_ORIGIN_CITATION=output/colleaf-qa/run-origins-citation.json MANYDOC_COLLEAF_RUN_ORIGIN_DETACHED=output/colleaf-qa/run-origins-detached.json GOWORK=off go test -run TestColleafRunOrigin -count=1 .
NODE_PATH=output/sf-qa/node_modules node scripts/verify_structured.cjs output/colleaf-qa/run-origins-saved.json.sf output/colleaf-qa/run-origins-edited.json.sf output/colleaf-qa/run-origins-citation.json.sf output/colleaf-qa/run-origins-detached.json.sf
rg -n 'Source != nil|manydocOrigin' colleaf_navigation.go colleaf_tables.go colleaf_lists.go colleaf_images.go
```

## Earlier progress: note definition and label source bindings

Whole-document native composition now carries note Source, LabelSource and
KindInferred in version1 evidence.noteOrigins records, checked against native
identity bindings and original physical pages. Separate source-free SF body/marker
fingerprints retain unchanged provenance, mark edited definition/label sources
historical with diagnostics, and never overwrite current content. Missing or changed
identity bindings archive available source facts in unboundSources; an explicit
history diagnostic retains an orphaned inferred-kind fact without asserting the
current kind. Footnote/endnote identities remain separately scoped.

Visible body-mark projection now fingerprints a sourced first paragraph before
prefix insertion, so an unchanged body mark does not falsely stale paragraph
provenance. Actual pinned plugins/validator/skeleton check three note records,
scoped identities, no body/label cache, styled emoji and rich body marks, two saves/
reopens, TextX body and marker edits, and detached inference-only identity. Strict
.vdocs/Go/SF comparisons and repeated round trips pass; ProseMirror checks61 nodes/
marks/attributes. Go also covers detached sourced-note archives and malformed
version/hash/page/duplicate records. Source-only mark/attribute removal leaves
semantic fingerprints unchanged. This is synthetic, headless component evidence,
not full imported-PDF or browser qualification.

Final checks pass: tests16.865s/CLI0.776s, race172.304s/CLI2.146s, vet/modules,
CGO-disabled focused tests0.445s, host CLI/--help and Linux amd64/arm64 cross-builds.
The final note-origin payload fuzz sample passes118517 executions/10.563s; it is
not exhaustive coverage. Source marks/empty source-only attrs normalize out of
semantic hashes; source-only differences and visible edits have distinct regression
checks. verification.json records scopes and limitations. The goal stays
active/incomplete. Next implement run/citation and table/image/container/cell/item
source mappings, then remaining marker appearance/nonvisual intent, public native
API/CLI and full-preset/browser qualification. Nonmain lookup limits, per-page/
decorated notes and broader PDF/source/resource/final acceptance audits remain.
No external blocker exists. Production stays pure Go; no app/backend change.

```sh
MANYDOC_COLLEAF_NOTE_ORIGIN_QA=output/colleaf-qa/note-origins.json GOWORK=off go test -run TestColleafNoteOrigins -count=1 .
node scripts/verify_colleaf_note_origins.cjs
MANYDOC_COLLEAF_NOTE_ORIGIN_SAVED=output/colleaf-qa/note-origins-saved.json MANYDOC_COLLEAF_NOTE_ORIGIN_BODY_EDITED=output/colleaf-qa/note-origins-edited.json MANYDOC_COLLEAF_NOTE_ORIGIN_MARK_EDITED=output/colleaf-qa/note-origins-marked.json MANYDOC_COLLEAF_NOTE_ORIGIN_DETACHED=output/colleaf-qa/note-origins-detached.json GOWORK=off go test -run TestColleafNoteOrigins -count=1 .
NODE_PATH=output/sf-qa/node_modules node scripts/verify_structured.cjs output/colleaf-qa/note-origins-saved.json.sf output/colleaf-qa/note-origins-edited.json.sf output/colleaf-qa/note-origins-marked.json.sf output/colleaf-qa/note-origins-detached.json.sf
rg -n 'Source != nil|manydocOrigin' colleaf_styles.go colleaf_tables.go colleaf_lists.go colleaf_images.go
```

## Earlier progress: paragraph origin bindings and unbound source facts

Private native paragraph/heading output now places a version1 source reference
in direct paragraph properties. A deterministic deduplicated origin pool in the
existing evidence carrier stores validated SourceInfo and source-free SF semantic
fingerprints, without text/body caches. References follow actual paragraph records
through save/edit; inherited style properties do not assert direct ownership.
Final restoration runs after citation/flow composition, across main/list/cell/
header/footer/note bodies and explicit section stories. Unchanged paragraph
semantics restore original source facts; changed content/presentation sets the
new SourceInfo.Historical flag and emits a diagnostic instead of overwriting edits.

Unreferenced origin facts are retained in Document.UnboundSources / SF root
attrs.unboundSources, with explicit loss of current ownership. Physical provenance
and existing qualifiers remain intact. Source clones isolate all nested pointer/
byte/slice fields. Formatting output still requires AllowLossy for provenance,
including unbound facts. Digest/version/missing-reference contradictions reject.
No SourceInfo geometry is treated as current native page geometry.

Pinned actual plugins/validator/renderer check seven source records and references
across stories, no body caches, styled emoji, two saves/reopens, TextX insertion
and reference removal. Strict .vdocs/Go/SF comparisons qualify unchanged/historical/
unbound semantics and stable repeated round trips. Actual ProseMirror checks70
nodes/marks/attributes across baseline, edited and detached SF. These are synthetic
paragraph-source fixtures, not full imported-PDF per-node provenance qualification.
Run, table/image/container, cell/item, note-definition/label and other source slots
remain required; unsupported output is still explicit.

Final checks pass: full tests16.187s/CLI0.705s, race170.104s/CLI1.838s,
vet/modules, CGO-disabled focused tests0.376s and host/Linux builds. The short
fuzz run passed85 executions/10.912s, a weak safety sample. verification.json
records scopes and limits. The goal remains active/incomplete. Next extend these bindings to run/citation source ranges and
container/cell/item/note provenance, then marker appearance, remaining nonvisual
intent, public native API/CLI and full-preset/browser qualification. Nonmain lookup
limits, per-page/decorated notes and broader PDF/source/resource/final acceptance
audits remain required. No external blocker exists; production stays pure Go.

```sh
MANYDOC_COLLEAF_ORIGIN_QA=output/colleaf-qa/paragraph-origins.json GOWORK=off go test -run 'TestColleafParagraphOrigins|TestColleafUnbound' -count=1 .
node scripts/verify_colleaf_origins.cjs
MANYDOC_COLLEAF_ORIGIN_SAVED=output/colleaf-qa/paragraph-origins-saved.json MANYDOC_COLLEAF_ORIGIN_EDITED=output/colleaf-qa/paragraph-origins-edited.json MANYDOC_COLLEAF_ORIGIN_DETACHED=output/colleaf-qa/paragraph-origins-detached.json GOWORK=off go test -run 'TestColleafParagraphOrigins|TestColleafUnbound' -count=1 .
NODE_PATH=output/sf-qa/node_modules node scripts/verify_structured.cjs output/colleaf-qa/paragraph-origins.json.sf output/colleaf-qa/paragraph-origins-edited.json.sf output/colleaf-qa/paragraph-origins-detached.json.sf
```

## Earlier progress: document history and PDF evidence in native intent

Private native composition now persists diagnostic history, metadata PageCount,
original PDF pages and PDFNavigation in version1 document evidence, carried by
the existing checked native CUSTOM annotation. This is nonvisual evidence, not a
cached body or native pagination instruction. Historical warning order/duplicates
remain exact; newly generated warnings append without reopen growth. Native
Title/body/styles stay authoritative. Typed intent parsing now checks canonical
keys, duplicates, Unicode and scalar shapes before semantic decoding.

PDFDestination.FlowBinding now also permits unbound: original physical view facts
remain even if native editing removes its current anchor or defines a formerly
missing source name. Resolved/missing source-PDF facts stay distinct from current
editable-target availability. No anchor/page is invented. Physical syntax and
geometry validate before graph reconciliation; unknown bindings cannot be repaired
into an accepted value by native edits. Normal bound values retain graph checks.

Pinned actual plugins/validator/renderer qualify exact history (including Unicode/
newlines/duplicates), two source PDF pages versus one current native skeleton
page, physical nullable/FitR views and annotations, styled emoji, two saves/reopens
and TextX/delete/create edits. Go strict .vdocs/SF comparisons check complete root
history/current graph, clone isolation, orphan reconciliation and stable repeated
round trips. The structural-control fixed-point test now preserves diagnostics;
its earlier clearing workaround is removed. Source PDF evidence comes from the
published-format synthetic PDF reader fixture with independently authored current
body content; this is not full imported-PDF body provenance qualification.

Final checks pass: full tests16.949s/CLI0.411s, full race171.301s/CLI2.180s,
vet/modules, CGO-disabled focused tests/host CLI execution and Linux amd64/arm64
cross-builds. Final payload fuzz passed105,180 executions; it is not exhaustive
proof. Pinned readers and diff checks pass; verification.json records exact scope.
Cross-builds are not Linux execution evidence. The full goal stays active/
incomplete. Next bind per-block/run/cell/item/note provenance through native
edits and complete marker appearance, remaining nonvisual intent, public native
API/CLI and full-preset/browser qualification. Nonmain lookup limits, per-page/
decorated notes and broader PDF/source/resource/final acceptance audits remain
required. No external blocker exists; production stays pure Go and apps are untouched.

```sh
MANYDOC_COLLEAF_EVIDENCE_QA=output/colleaf-qa/document-evidence.json GOWORK=off go test -run TestColleafDocumentEvidence -count=1 .
node scripts/verify_colleaf_evidence.cjs
MANYDOC_COLLEAF_EVIDENCE_SAVED=output/colleaf-qa/document-evidence-saved.json MANYDOC_COLLEAF_EVIDENCE_EDITED=output/colleaf-qa/document-evidence-edited.json MANYDOC_COLLEAF_EVIDENCE_DELETED=output/colleaf-qa/document-evidence-deleted.json MANYDOC_COLLEAF_EVIDENCE_CREATED=output/colleaf-qa/document-evidence-created.json GOWORK=off go test -run 'TestColleafDocumentEvidence|TestPDFNavigationUnbound' -count=1 .
```

## Earlier progress: foreign native control-cursor recovery

Private native import now projects bookmarks on row/cell boundaries and section,
table, image-paragraph and page-break-paragraph terminators into SF AnchorPoint
boundaries. Row edges follow first/last logical cells. Vertical/horizontal covered
cell cursors retain identities at the spanning origin's start/end; source positions
and normalized targets are disclosed with colleaf_structural_cursor diagnostics.
Exact native control cursor identity is not claimed. This changes previously
unrepresented points from rejection to explicit logical recovery without dropping
relationships or changing visible text, assets, merge grids or ordinary styles.

Boundary-only blocks travel with their native paragraph's flow ownership, so new
table/image-end points do not invalidate valid list continuation bounds. Deferred
cell projections apply after origins exist, including earlier-row merges. Loss
warnings include source UTF-16 position and story context; a projection-local set
avoids repeatedly scanning all prior position-specific diagnostics.

Pinned actual app/plugins/skeleton/two saves/reopens/TextX checks qualify 21 exact
original targets and their deliberately changed normalized targets. Native text,
image bytes and covered-cell records remain unchanged; each original/normalized/
edited probe issues 21 exact internal selection-manager requests. Go strict
.vdocs/SF checks compare original/saved/edited full structures, relationships,
assets and exact diagnostic sets. The normalized native fixed point is checked
separately after explicitly clearing historical diagnostics: their transport in
native nonvisual intent remains unfinished, while SF itself preserves them.
Horizontal continuations, header/footer/note contexts, list ownership, hard-break
terminators and all 63 scalar boundaries of the rich reference have regressions.
Browser caret/scroll/paint and universal native cursor fidelity are not claimed.

Final checks pass: full tests15.867s, full race169.965s/CLI2.147s, vet/modules,
CGO-disabled focused tests/host CLI execution and Linux amd64/arm64 cross-builds.
Final structural property/offset fuzz passed72,173 executions; short campaigns
are not exhaustive coverage. Pinned readers and diff checks pass; verification.json
records exact scope. Cross-builds are not Linux execution evidence. The full goal
stays active/incomplete. Next complete native marker appearance and nonvisual/provenance
intent (especially historical diagnostics), public native API/CLI and actual
full-preset/browser qualification. Nonmain lookup limits, per-page/decorated notes
and broader PDF/source/resource/final acceptance audits remain required. No
external blocker exists; runtime stays pure Go and applications are untouched.

```sh
MANYDOC_COLLEAF_CURSOR_QA=output/colleaf-qa/structural-cursors.json MANYDOC_COLLEAF_CURSOR_NORMALIZED_QA=output/colleaf-qa/structural-cursors-normalized.json GOWORK=off go test -run TestColleafForeignStructural -count=1 .
node scripts/verify_colleaf_structural_cursors.cjs
MANYDOC_COLLEAF_CURSOR_SAVED=output/colleaf-qa/structural-cursors-saved.json MANYDOC_COLLEAF_CURSOR_EDITED=output/colleaf-qa/structural-cursors-edited.json MANYDOC_COLLEAF_CURSOR_NORMALIZED_SAVED=output/colleaf-qa/structural-cursors-normalized-saved.json GOWORK=off go test -run TestColleafForeignStructural -count=1 .
```

## Earlier progress: standalone zero-height navigation blocks

Private native workers now preserve standalone SF AnchorPoint blocks at real
UTF-16 cursors without adding visible characters or extra paragraphs to ordinary
flows. Sparse version1 point intent records the native owning section/item ID,
point-group order and alias index. Section and item owners distinguish the same
cursor inside a list item from the point outside the list. Consecutive point
blocks, alias order, item/section ends, cells, images/breaks, headers and notes
retain their SF structure. Scope rebinding visits only current flow ranges; point
and temporary scope counters do not change ordinary native content IDs.

A point-only flow uses the same required empty native paragraph as an empty flow.
The sparse carrier ID authorizes removing it only while its current position,
owner, empty content and presentation still agree. Unrelated/edited authored
paragraphs remain. Native edits that move points off a current flow boundary keep
current inline/container locations with explicit ownership diagnostics. Terminal
LF points remain represented. Final-tree carrier cleanup follows actual grouping;
tentative list assembly cannot authorize removing a point from discarded output.

Pinned actual plugins/app validation/skeleton/two saves/reopens/TextX movement
qualify point groups/aliases, item/root shared cursors, cell and terminal points,
point-only carriers and fifteen exact internal selection-manager requests. Native
text stays unchanged; real insertion adds only Q and moves points. Go strict
.vdocs/SF checks compare complete saved/edited structures/assets/relationships,
exact expected ownership diagnostics and source immutability. Dense256-item,
point-only, explicit-section, malformed-order and unrelated-carrier regressions
pass. Browser caret/scroll/paint remains unqualified.

Final checks pass: full tests 16.913s/CLI0.380s, full race170.046s/CLI2.315s,
focused actual standalone race1.867s, vet/modules, CGO-disabled focused tests/host
CLI execution and Linux amd64/arm64 cross-builds. The short property/offset fuzz
run passed79,713 executions; it is not exhaustive ownership/carrier coverage.
Pinned readers and diff checks pass; verification.json records exact scope.
Cross-builds are not Linux execution evidence. The full goal stays active/
incomplete. Next recover remaining foreign structural row/cell/section
cursors, then finish marker appearance/nonvisual/provenance intent, public native
API/CLI and actual full-preset/browser qualification. Nonmain lookup limits,
per-page/decorated notes and broader PDF/source/resource/final acceptance audits
remain required. No external blocker exists; production stays pure Go and
applications/backends are untouched.

```sh
MANYDOC_COLLEAF_STANDALONE_QA=output/colleaf-qa/standalone-points.json GOWORK=off go test -run TestColleafStandalone -count=1 .
node scripts/verify_colleaf_standalone_points.cjs
MANYDOC_COLLEAF_STANDALONE_SAVED=output/colleaf-qa/standalone-points-saved.json MANYDOC_COLLEAF_STANDALONE_EDITED=output/colleaf-qa/standalone-points-edited.json GOWORK=off go test -run TestColleafStandalone -count=1 .
```

## Earlier progress: list, page-break and section navigation

Private native workers now map list container anchors, hard-page-break anchors
and explicit section anchors through real BOOKMARK3 points. List ownership hints
bind the first paragraph ID and current start; promotion to a list occurs only
when current grouping agrees. Moved/stale points stay at their current inline
position with diagnostics. Page breaks use the real form-feed token; omitting a
rendered soft break retains its point as a zero-height SF AnchorPoint. Sections
bind actual section IDs and starts, including empty sections and starts shared
with a paragraph/break point. No extra text or cached body trees are introduced.

The preliminary first-section note composition now admits omitted section/root
anchor references only in its internal temporary graph. Final destination
validation still checks the entire real document and retains only actual declared
unresolved targets. This fixes valid cross-section links without inventing final
missing-target declarations. Section/list ownership and metadata losses remain
explicit; no application admission/permission policy is introduced.

Pinned actual plugins/validator/skeleton/save/reopen/TextX checks qualify eleven
points across nested main lists, three sections (one empty), hard breaks, header
and note lists, plus nine exact internal selection-manager refresh requests. Go
strict .vdocs/SF checks compare the entire saved/edited structure and relationships,
including exact stale-section placement diagnostics and actual text intervals.
Moved-in-paragraph list points and soft-break point retention have regressions.
No browser caret/scroll/paint fidelity is claimed.

Final checks pass: post-cleanup full tests 16.491s and all Colleaf race tests
5.077s; preceding full race 174.130s/CLI1.820s passed before removing one duplicate
identical control lookup. Final vet/CGO-disabled host build pass; modules, focused
CGO-disabled tests, CLI execution and Linux amd64/arm64 builds also pass. The short
property/offset fuzz run passed75,187 executions before the isolated cleanup; it
is not exhaustive flow-owner coverage. Pinned readers and diff checks pass. See
verification.json for exact scopes; cross-builds are not Linux execution evidence.
The full goal stays active/incomplete. Next implement standalone AnchorPoint export and remaining
structural cursors, then marker appearance/nonvisual/provenance intent, public
native API/CLI and actual full-preset/browser qualification. Nonmain target lookup
limits, per-page/decorated notes and broader PDF/source/resource/final acceptance
work remain required. No external blocker exists; production stays pure Go.

```sh
MANYDOC_COLLEAF_FLOW_QA=output/colleaf-qa/flow-navigation.json GOWORK=off go test -run TestColleafFlowNavigation -count=1 .
node scripts/verify_colleaf_flow_navigation.cjs
MANYDOC_COLLEAF_FLOW_SAVED=output/colleaf-qa/flow-navigation-saved.json MANYDOC_COLLEAF_FLOW_EDITED=output/colleaf-qa/flow-navigation-edited.json GOWORK=off go test -run TestColleafFlowNavigation -count=1 .
```

## Earlier progress: real table and image navigation points

Private native workers now export/import anchors at real table-start controls and
image atoms, including nested cells, list continuations, headers, footers and
notes. Native source IDs bind sparse version1 table/picture owner hints; running
story table promotion rebinds those IDs. No synthetic paragraph, invisible text,
asset copy or block tree is used to transport navigation. Current native token
positions win over stale hints. Foreign hint-free points at these starts bind to
the current SF table/image; moved container points in text become inline anchors
with explicit diagnostics. Unknown metadata and span editing extent remain
explicit losses. Unrepresented row/cell/section control positions still reject.

Pinned actual plugins, app validation, two saves/reopens, skeleton construction
and TextX insertion qualify 16 points across main/nested/cell/list/header/footer/
note bodies, seven exact internal selection-manager refresh requests, hint-free
native table/image points and current source-ID relationships. Go strict .vdocs/SF
reimport checks the entire saved/edited tree, assets, formatting and relationships.
TextX's independently inserted Q interval is checked separately from the original
text interval. No browser caret/scroll/paint qualification is claimed.

Final checks pass: all-package tests 18.163s, full race 174.809s, combined actual
saved/edited navigation regression race 2.447s, vet/modules, CGO-disabled focused
tests/host CLI execution and Linux amd64/arm64 cross-builds. A short navigation
property/offset fuzz campaign passed 81,482 executions; it is not exhaustive
container coverage. Pinned readers and diff checks pass; verification.json records
exact scope. Cross-builds are not Linux execution evidence. The goal remains
active/incomplete. Next implement standalone AnchorPoint, anchors on lists,
sections and page breaks, and remaining structural cursor projection; then finish
marker appearance/nonvisual/provenance intent, public native API/CLI and actual
full-preset/browser qualification. Nonmain editor target lookup limits, per-page/
decorated notes and broader PDF/source/resource/final acceptance audits remain
required. No external blocker exists; runtime is pure Go and apps are untouched.

```sh
MANYDOC_COLLEAF_CONTAINER_QA=output/colleaf-qa/container-navigation.json GOWORK=off go test -run TestColleafContainerNavigation -count=1 .
node scripts/verify_colleaf_container_navigation.cjs
MANYDOC_COLLEAF_CONTAINER_SAVED=output/colleaf-qa/container-navigation-saved.json MANYDOC_COLLEAF_CONTAINER_EDITED=output/colleaf-qa/container-navigation-edited.json GOWORK=off go test -run TestColleafContainerNavigation -count=1 .
```

## Earlier progress: bookmark points and typed internal navigation

Private native output/import now maps paragraph/heading block anchors and inline
anchor atoms to real BOOKMARK3 point records without inserting visible characters.
UTF-16 point boundaries include astral starts and paragraph ends. Small version1
placement hints bind block ownership to the current paragraph ID/position; native
edits win and stale block placement becomes an inline point with diagnostics.
Stable transport IDs avoid all logical/missing names, preventing accidental SDK
rangeId aliases. No text, styles or body trees are cached in point metadata.

Main bookmark range-ID aliases resolve to canonical SF identities. Native headingId
bindings become scoped SF point identities and typed links; redundant native
fragment URLs normalize to the existing typed-destination semantics. Explicit
missing targets persist; new missing references are diagnosed. Foreign bookmark
editing ranges collapse to current start points with disclosed loss of range
extent. A final graph check rejects unrepresented structural cursors. Main-story
lookup limits for targets in other stories are disclosed; data is retained in SF.
These limits remain required qualification work, not completion exceptions.

Pinned real save/reopen and TextX insertion verify nine main points, twelve
internal-link selection-refresh requests from main/cell/list/header/footer/note
stories, foreign headingId lookup, missing-target false behavior and intact emoji/
13pt/bold/color glyph fields. The unmodified hash-pinned click handler uses the
real selection manager; the render manager is stubbed, so actual caret display,
scrolling, browser events and UI controls remain unqualified. Strict .vdocs/Go/SF
checks verify saved and edited graphs, source immutability, and current point
movement over stale ownership. A 1,024-paragraph case retains every named point.

Final tests pass: library15.527s/CLI0.369s/internal2.264s, all-package race
(cache-confirmed after the fresh run), actual saved/edited/heading focused race
2.310s, vet/modules, CGO-disabled focused tests/host CLI execution and Linux amd64/
arm64 cross-builds. Point fuzz84,704 executions passed before the isolated metadata
passthrough adjustment; the four-execution whole-snapshot fuzz sample is weak
evidence. Pinned readers and diff checks pass; verification.json records scope
and recovery details. Linux builds are not Linux execution evidence. The full goal
stays active/incomplete. Next finish standalone/container anchor and structural-
cursor projection, remaining marker appearance/nonvisual/provenance intent,
public native API/CLI and actual full-preset/browser qualification. Per-page/
decorated notes and broader PDF/source/resource/final acceptance audits remain
required. No external blocker exists. Runtime stays pure Go; apps/backends are
untouched and no permission/quota gates were introduced.

```sh
MANYDOC_COLLEAF_NAVIGATION_QA=output/colleaf-qa/navigation.json GOWORK=off go test -run TestColleafNavigation -count=1 .
node scripts/verify_colleaf_navigation.cjs
MANYDOC_COLLEAF_NAVIGATION_SAVED=output/colleaf-qa/navigation-saved.json MANYDOC_COLLEAF_NAVIGATION_EDITED=output/colleaf-qa/navigation-edited.json MANYDOC_COLLEAF_NAVIGATION_HEADING=output/colleaf-qa/navigation-heading.json GOWORK=off go test -run 'TestColleafNavigation|TestColleafForeignHeading' -count=1 .
```

## Earlier progress: registered hyperlink persistence and scoped identities

The actual pinned hyperlink plugin now participates in native save/reopen QA.
It emits DOC_HYPER_LINK_PLUGIN URL resources and applies them by global rangeId
across main/header/footer models on load; it does not visit note bodies. Private
import now applies that precedence before interpreting text and preserves direct
note URLs. Current resource targets win over missing/different body URLs with an
explicit diagnostic; inactive entries are disclosed. Duplicate resources/entries,
ambiguous cross-story identities, malformed JSON and conflicting external/internal
targets reject. Input snapshots stay unchanged. Export scopes running-story link
IDs so independently authored main/header/footer links cannot collide in the plugin.

Actual two-save/reopen checks preserve five links in main/cell/list/header/footer
stories, a note-local link, astral text and authored13pt/bold/color/no-underline
fields. A header resource-only edit updates exactly that target; Go/SF reimport
matches both before and after actual plugin load/save. The existing unit-span SF
normalization is explicit in the comparison, not an altered corpus/oracle. The
hash-pinned unmodified UI click handler runs against real loaded models and selects
the five expected external targets; window.open is stubbed. This proves handler
logic/save transport, not browser popup/event/paint or internal bookmark navigation.

Final checks pass: all-package tests with actual saved/edited fixtures16.019s/
0.354s, full race168.814s/2.125s, additional actual byte-codec/SF race1.692s,
vet/modules, CGO-disabled focused tests/host CLI execution and Linux amd64/arm64
cross-builds. Resource fuzz31,987 and JSON fuzz96,466 executions pass; short campaigns
are not exhaustive. Cross-builds are not Linux execution. Diff check is clean;
verification.json records exact scope. Full goal stays active/incomplete. Next map actual bookmark/heading points and typed internal
navigation, remaining marker appearance/nonvisual/provenance intent, public native
API/CLI and real full-preset/browser interaction. Per-page/decorated notes and the
broader PDF/source/resource/final acceptance audit remain required. No external
blocker exists; runtime remains pure Go and applications/backends are untouched.

```sh
MANYDOC_COLLEAF_HYPERLINK_QA=output/colleaf-qa/hyperlinks.json GOWORK=off go test -run TestColleafHyperlink -count=1 .
node scripts/verify_colleaf_hyperlinks.cjs
MANYDOC_COLLEAF_HYPERLINK_SAVED=output/colleaf-qa/hyperlinks-saved.json MANYDOC_COLLEAF_HYPERLINK_RESOURCE_EDITED=output/colleaf-qa/hyperlinks-resource-edited.json MANYDOC_COLLEAF_HYPERLINK_EDITED_SAVED=output/colleaf-qa/hyperlinks-edited-saved.json GOWORK=off go test -run TestColleafHyperlink -count=1 .
```

## Earlier progress: native zero and legal-numbering mappings

SF keeps its existing zero/legal semantics: nondecimal zero displays0 and Legal
uses decimal for every referenced level. Private native output now uses a decimal
first-zero type followed by the normal type in the same automatic counter group.
Zero ancestors project similarly. Legal decimalZero fields project to decimal,
correcting native's padded legal rendering. Sparse checked format intent and item-
type aliases preserve exact SF formats/ownership; no marker/body cache was added.
Running-story promotion scopes all aliases/projection keys into global catalogs.

Import checks native glyphs, legal flags and actual counters before restoring
format intent. Native edits win; stale intent gets diagnostics. Owned containers
must agree on every item's effective numbering, not merely type/counter sequence.
Foreign native00 or empty letter/Roman zero markers become explicit SF templates;
foreign mixed legal-padding rules become actual effective formats. Semantic
changes split groups and live-policy loss is diagnosed, retaining current visible
markers. Projection keys/indices/ownership and canonical integer member names are
checked; diagnostic order is stable and repeated aliases do not cause quadratic
projection validation. Native format/live-policy parity is not claimed from this.

The pinned renderer checks30 main/cell markers,3 note markers,3 isolated header
markers with the actual global catalog, and10 foreign zero/legal policies. Two
actual facade saves/reopens preserve exact Go/SF/native semantics. Native first
counter edits0-to5, zero-ancestor edits0-to5 and legal glyph changes back to padded
format update actual labels. Go/SF import checks every edited main/cell marker,
including split groups. Existing literal and original list reader regressions pass.
Headless content/counter/save checks do not qualify browser/preset interactions.

Final checks are recorded in verification.json before commit. Goal remains active/
incomplete. Next complete native marker appearance and navigation/nonvisual/
provenance intent, public API/CLI and actual full-preset/browser paint/wrapping/
selection/navigation/list editing checks. Per-page/decorated-note policies and
broader PDF/source/resource/final acceptance audits remain required. No external
blocker exists; no application/backend changes or runtime services were added.

```sh
MANYDOC_COLLEAF_ZERO_QA=output/colleaf-qa/zero-lists.json GOWORK=off go test -run TestColleafZero -count=1 .
node scripts/verify_colleaf_zero_lists.cjs
MANYDOC_COLLEAF_ZERO_SAVED=output/colleaf-qa/zero-lists-saved.json MANYDOC_COLLEAF_ZERO_EDITED=output/colleaf-qa/zero-lists-edited.json GOWORK=off go test -run TestColleafZero -count=1 .
```

## Earlier progress: dynamic native literal-percent and digit-suffix markers

Native list output now compiles SF/OOXML single-digit placeholders into the pinned
native grammar, which reads whole decimal indices and has no percent escape.
Non-active fixed-symbol entries at index9 onward carry literal fragments; real
paragraph levels remain0..8 and real counters/formats stay automatic. This supports
literal %, percent-only/constant templates, digit suffixes such as SF %10, all
nine real levels, compound/continued lists, cells, notes and headers. Helpers are
native source entries, not extra SF outline levels or frozen per-item labels.

Existing sparse OriginalTemplate intent restores ignored deeper placeholders only
when the current format/helper projection agrees. Current helper/format/counter
edits win. Unbound representable helper templates derive from current native data;
ambiguous literal percent-digit cases reject rather than becoming false counters.
Known numeric glyphs with fixed glyphSymbol now import their actual constant
marker instead of silently treating it as a changing numeric format. Template and
helper bounds follow the SF512-byte representation; actual levels remain bounded.

Pinned actual glyph checks verify twelve main/cell labels, two note labels and two
header labels using the correct global running-story list catalog. Two actual
facade saves/reopens preserve exact Go/SF/native semantics. Native percent-to-per-
mille helper edits and parent2-to5 counter edits update the real child labels;
Go/SF import resolves current enclosing counters over stored ancestor fallbacks.
These checks do not qualify browser paint, preset controls or full native parity.

Final checks pass: tests16.097s/0.717s, race168.349s/2.129s, vet/modules,
CGO-disabled focused tests/host CLI execution and Linux amd64/arm64 cross-builds.
Template fuzz106,875 and list-graph fuzz37,843 executions pass; these are short
campaigns, not exhaustive proof. Pinned old/new list readers, app validator,
facade save/reopen and final edited/saved Go/SF import0.361s pass; diff check is
clean. Cross-builds are not Linux execution. verification.json records evidence.
The goal remains active/incomplete. Next investigate/implement native nondecimal-zero and legal
numbering differences (notably decimalZero), remaining marker appearance and
navigation/nonvisual/provenance intent, then public API/CLI and actual browser/
preset interaction. Per-page/decorated notes, broader PDF/source/resource audits
and final acceptance remain required. No external blocker exists.

```sh
MANYDOC_COLLEAF_LITERAL_LIST_QA=output/colleaf-qa/literal-lists.json GOWORK=off go test -run TestColleafLiteral -count=1 .
node scripts/verify_colleaf_literal_lists.cjs
MANYDOC_COLLEAF_LITERAL_LIST_SAVED=output/colleaf-qa/literal-lists-saved.json MANYDOC_COLLEAF_LITERAL_LIST_EDITED=output/colleaf-qa/literal-lists-edited.json GOWORK=off go test -run TestColleafLiteral -count=1 .
```

## Earlier progress: strict native .vdocs envelope boundary

Private readColleaf/writeColleaf workers now read/write the exact pinned v3
vibedada-document/Univer1.0.4 envelope around the rich native projection, through
shared SF semantics. Bare snapshots/renamed SF, wrong identities/versions, unknown
typed fields and missing required snapshot fields reject. A scanner validates
syntax, decoded duplicate keys, UTF-8 and paired Unicode surrogate escapes before
encoding/json can replace text or collapse maps. Known typed record aliases and
null scalar offsets reject. Large string values scan in place without string
copies; object-key sets and schema metadata allocate. Nested interpreted resource
JSON is checked independently, including carrier records. A depth bound protects
the parser; no application byte/image quota or permission gate was introduced.

Actual pinned facade save exposed an added settings object that raw-snapshot
qualification missed. The codec accepts documented empty/positive zoom settings;
zoom and nondefault editor locale emit explicit logical-SF loss diagnostics.
Unrepresented settings reject. The unmodified hash-pinned Colleaf validator accepts
the generated envelope. Two actual facade saves/reopens and exact Go/SF reimport
preserve section/page settings, first/even/default/empty stories, tables, lists,
image bytes/relationships and metadata. Existing blank/Unicode cases pass too.
This is structural/save evidence, not browser paint or interaction qualification.

Final checks pass: tests 15.722s/0.722s, race 167.447s/2.236s, vet/modules,
CGO-disabled focused native/JSON tests and host CLI execution, Linux amd64/arm64
cross-builds, strict JSON fuzz 76,406 executions and native envelope fuzz 872
executions. Fuzz campaigns are short, not exhaustive; cross-builds are not Linux
execution. Actual app-validator/facade/saved-envelope reimport and diff checks
pass. verification.json records scope/evidence. The full goal remains active/incomplete. Next finish native list/appearance/navigation and
nonvisual/provenance intent, automatic per-page/decorated-note policies, public
API/CLI and actual full-preset/browser paint/wrapping/selection/navigation checks.
The broader PDF/source/resource/final acceptance audit remains required; native
input currently buffers the envelope and large-container allocation behavior
still needs that audit. No external blocker exists.

Reproduce this milestone with:

```sh
MANYDOC_COLLEAF_ENVELOPE_QA=output/colleaf-qa/sections.vdocs GOWORK=off go test -run 'TestColleafEnvelope|TestStrictJSON' -count=1 .
node scripts/verify_colleaf_envelope.cjs
MANYDOC_COLLEAF_ENVELOPE_SAVED=output/colleaf-qa/sections-saved.vdocs GOWORK=off go test -run 'TestColleafEnvelope|TestStrictJSON' -count=1 .
```

## Earlier progress: editable native per-citation literal and rich marks

Private native output now preserves per-reference literal/rich/empty/default labels
and inferred-relationship flags, including repeated citations in sections, cells
and lists. Real markerless note atoms retain the note relationship/placement;
editable superscript label runs and an owned zero-width delimiter carry display.
Separate real body marks preserve the note's shared default without duplicating
note definitions or whole bodies. Existing SF label fields remain canonical.

The pinned renderer checks seven markerless anchors, actual visible labels,
superscript 3 and authored color/size/bold/italic fields, plus two placed bodies.
Actual facade save/reopen and TextX/style edits retain exact Go/SF semantics.
TextX can split a label annotation around replaced text: a stable token combines
only matching fragments between the protected anchor and delimiter. Missing/stale
ownership leaves visible text/anchors in the story and reports losses; malformed
or nonneutral ownership rejects. A 2,048-reference paragraph retains every reference
and default, using effective style spans and linear paragraph consumption.

Final milestone checks pass: tests 16.855s/0.777s, race 167.692s/2.133s,
vet/modules, CGO-disabled focused tests/host CLI execution and Linux amd64/arm64
cross-builds. Citation fuzz 1,000 and document fuzz 50,749 executions pass; all twelve
pinned reader/save scripts, independent Python regressions, final combined Go
edited/saved re-import and diff check pass. Cross-builds are not Linux execution
evidence. verification.json records exact scope, timings and remaining limits.

Next complete remaining native list/appearance/navigation and nonvisual/provenance
intent, strict envelope/JSON/surrogates, public API/CLI and actual full-preset/
browser paint/wrapping/selection/navigation/interaction qualification. Automatic
per-page/decorated-note cases and the broader PDF/source/resource/final acceptance
audit remain on the required queue. No browser click/atomic wrapping or full native
adapter completion is claimed from headless glyph/save checks. Runtime stays
pure Go, with applications/backends untouched and no external blocker.

## Earlier progress: editable independent and rich native body marks

Private native output now writes literal, rich and uniformly styled independent
note-body marks as actual editable runs, including links, table-first and otherwise
empty note bodies. Native generated body markers are suppressed. A small local
body annotation binds role/representation to the paragraph and delimiter; marker
text/styles remain solely in normal source runs. Empty styled intent uses the real
referenceTextStyle field with nonvisible ownership metadata.

Pinned renderer/facade checks confirm six placed bodies, distinct citation/body
marks, actual red/green/font-size/bold/italic/underline/link fields, table placement
and empty variants. Actual TextX replacement and style edits survive Go re-import.
Native CUSTOM annotations otherwise override authored colors/underline: output
sets active=false and textColorMode=text, with a retained failing-render regression.
TextX can detach the start boundary during replacement, so recovery uses stable
paragraph identity and the owned terminal tab, rather than stale original text.
Missing/stale ownership retains visible story text and emits diagnostics; malformed
or nonneutral ownership rejects explicitly. Native API/CLI remains private.

Final milestone checks pass: tests 17.345s/0.799s, race 167.140s/2.180s,
vet/modules, CGO-disabled focused tests/host CLI execution and Linux amd64/arm64
cross-builds. Body-mark fuzz 1,000 executions and document fuzz 51,866 executions
pass; diff check and all eleven pinned reader/save scripts plus independent Python
regressions pass. Cross-builds are not Linux execution evidence. verification.json
records exact scope, timings and the remaining UI/font/layout qualification.

Per-citation output is now qualified above. Next complete remaining list/appearance/
provenance intent, strict native envelope/JSON/surrogates and public native API/CLI.
Actual browser/preset paint/wrapping/interaction and total story qualification also
remain required. Prior foreign-numbering/import and DOCX/PDF checks stay valid;
this milestone does not claim full adapter or overall goal completion.

## Earlier progress: foreign native note numbering and output proof

colleaf_note_numbering.go now recovers native automatic citation/body marks from
root and section settings: starts/restarts, decimal/zero/letter/Roman defaults,
markerless/custom marks, repeated citations with different labels and supported
symbol/Unicode formats. Current values become explicit SF labels with diagnostics
for live-counter/placement/column policy losses. First and final body marks stay
distinct; source range IDs carry per-citation overrides through cell fragments.
The actual reader confirms all ten example citations and eight placed note bodies,
fifteen format outcomes, native default Roman endnotes and facade save/reopen.

The save test also corrected foreign omitted header/footer distances: the native
constructor supplies 30px (22.5pt), not the earlier assumed 18pt. Generated own
projections already emit their distances explicitly. Section snapshot transport now
retains uncaptured properties so independent readers see actual noteProperties.

SF→DOCX/PDF paths accept the imported marks with explicit AllowLossy acknowledgment.
Independent Word XML/python-docx and PDF pypdf/MuPDF verify eight bodies, variant/
markerless relationships, nine actual citation/link boxes and shared destinations.
PDF output uses explicitly supplied regular/bold DejaVu faces for the original
supplementary character; missing font/glyph attempts rejected explicitly. All four
Poppler-rendered PDF pages were visually inspected: marks/bodies are legible with
no clipping/overlap/missing glyphs. This is a sparse targeted fixture, not a general
Word/Colleaf-browser fidelity claim. Full check evidence is in verification.json.

Final milestone checks pass: tests 17.489s/0.746s, race 167.481s/2.265s,
vet/modules, CGO-disabled focused tests/host CLI execution and Linux amd64/arm64
cross-builds; document/note fuzz 22,371/2,551 executions and diff check. All ten
pinned reader/save scripts, Pillow, independent note-output readers and combined
Go edited/saved import pass. Cross-builds are not Linux execution evidence.

The adapter remains private/incomplete. Independent/rich body-mark output is now qualified above. Next implement native
literal/rich citation output, remaining list/appearance/provenance
intent, strict envelope/JSON/surrogates, then public native API/CLI and actual
browser/preset qualification. Automatic per-page note counters still require an
authoritative layout strategy; editable separators/notices and remaining native
number formats also remain explicit unfinished cases. They reject rather than
silently guessing or dropping visible content. Pure-Go runtime and application/
backend boundaries are unchanged; there is no external blocker to the next work.

## Earlier progress: native image bounds and real note placement

Private document output now fits individual image frames to page/cell/note/running
containers, including merged/nested cells and narrower inherited story contexts.
It retains preferred SF sizes in checked geometry intent; real native resizes win.
The executable renderer does not shrink oversized frames itself: a 1000×2000px
frame overflowed a 320px body on a 600px page. The new regression preserves that
failure, checks actual projected boxes and verifies facade save/Go re-import.

That probe exposed a substantive note representation error. Earlier custom-block
checks proved transport/source access but did not place notes. Native citation
layout consumes FOOTNOTE8/ENDNOTE9 ranges on U+FFFC, now emitted and read. The
pinned reader independently checks citation glyphs and actual placed foot/endnote
bodies, including a cell citation. Checked automatic-mark intent stabilizes shared
SF note labels across repeated native references and native endnote Roman defaults;
reordered native citations keep actual marks when automatic intent becomes stale.
Older private custom-block input remains readable, but is no longer emitted.

Final checks pass: tests 18.035s/0.815s, race 168.843s/2.995s, vet/modules,
CGO-disabled focused tests/host CLI execution and Linux amd64/arm64 cross-builds;
document/note fuzz 15,990/737 executions and diff check. Linux cross-builds are not
execution evidence. Full details and limits are in verification.json.

Nine pinned reader/save scripts plus Pillow and combined edited/saved Go re-import
pass. The latest full checks are recorded in verification.json. These are frame/
structure/placement/save checks, not browser font/paint or total-story-height
qualification. Basic foreign numbering is now qualified above. Rich citation/body
marks, per-page/decorated settings, remaining list/appearance/provenance intent, strict native envelope/JSON/
surrogates and actual full-preset/browser behavior remain work. Public native
API/CLI and the full goal remain incomplete. Production stays pure Go.

## Earlier progress: native sections, page settings and running stories

Private document composition now connects page geometry, section boundaries and
first/even/default header/footer variants to the existing text/table/list/image/
note workers. Zero/fractional margins and distances, optional/inherited layout,
explicit empty stories, authored page breaks and metadata survive SF round trips.
Native geometry edits win over checked original precision/default intent. Unknown
layout properties produce diagnostics; contradictory references/orphans reject.
Native rendered soft page breaks are disclosed and removed from logical flow.

The pinned executable reader confirms seven pages, odd-page blank insertion,
geometry transitions, story selection/inheritance and header merged table/bullet/
image boxes. Header assets must live in document catalogs: local-only sources
were ignored by the renderer. Actual FDocument.save/fresh reopen retains the same
Go model. This proves structure/geometry/save behavior, not browser paint or font
metrics. The native adapter remains private/incomplete, with no public API/CLI.

Next qualify total native story layout and complete rich note/marker/list/appearance
intent and strict native envelope/JSON/surrogate handling, then actual full-preset/
browser editing/rendering and the final acceptance audit. Broader untagged PDF
note recovery and final source/resource/API checks also remain. No external
blocker exists; production remains pure Go with applications/backends untouched.

Final section milestone checks pass: tests 17.829s/0.786s, race 174.270s/1.773s,
vet/modules, CGO-disabled focused tests/host CLI execution, Linux amd64/arm64
cross-builds, short document fuzz 2,854 executions and diff check. All eight pinned
reader/save scripts, Pillow and edited/saved Go re-import pass. See verification.json
for exact scope; Linux cross-builds are not execution evidence.

The preceding facade correction remains: only checked nonvisible intent is stored
in a native CUSTOM range on the terminal LF because actual FDocument.save drops
unregistered manydoc resources. Registered drawing mirrors must match persisted
sources. SDK permission resources never introduce document admission gates.


Private image workers now map embedded PNG/JPEG bytes, alternative text, drawing
atoms/sources/order and preferred boxes through SF in main bodies, nested cells,
list continuations and notes. Drawing IDs are globally distinct across stories,
matching the native flat registry while retaining note-local source ownership.
Checked image geometry hints preserve omitted dimensions and exact fractional
point intent without duplicating bytes/text; native asset/resize edits win.

Pinned app/drawing lookup/skeleton box/save/Go re-import checks and independent
Pillow pixel/alpha checks pass, including the unchanged original MQTT JPEG hash.
Mixed inline image/text becomes ordered SF blocks with diagnostics. Malformed or
contradictory sources, orphans, remote/hosted sources and unmapped hidden/transformed
appearance reject instead of silently changing/dropping content. No public native
API/CLI exists. Browser bitmap painting/editor resource persistence, contextual
total story fitting and remaining profile mappings are open. Individual frames
now have contextual bounds; running stories
now have the section-composition qualification above.
The full adapter and overall goal remain active/incomplete.


Private list workers now map native bullets/list sources and checked item ownership
through SF, including nested lists, continuation paragraphs/tables, separate adjacent
lists, restarts, compound/legal/independent numbering and decimal zero starts.
Lists also work inside note bodies. Native paragraph IDs bind item/container bounds;
no item text is cached. Actual bullet/source edits remain authoritative. Unbound or
stale ownership falls back to disclosed native-bullet inference without dropping
unmarked text/tables. Native counter starts are zero-based, unlike SF starts.

Pinned app validator/renderer marker glyphs/TextX save/Go re-import qualify AA/BB,
compound Roman markers, independent8.1, restarts, bullets and decimal0 in main and
note stories. Counter-cache history alone was insufficient; actual LIST glyphs
retain earlier nested markers. Full browser/resource persistence and visual layout
remain unqualified. Literal percent templates and nondecimal zero policies differ
between SF and this native renderer: private workers reject these known mismatches
pending proper representation. Undefined deeper placeholders project safely with
checked intent. The full adapter and overall goal remain active/incomplete.


Private note workers now map real native foot/endnote definitions, styled UTF-16
reference atoms, repeated citations, custom shared marks, markerless body marks
and editable rich bodies including merged/nested tables. A sparse checked resource
preserves kind-scoped SF IDs and definition order only; native visible text/styles/
marks remain authoritative. Edited native identity/kind invalidates its old binding.
Actual pinned app validation/reader/TextX note edit/save/Go re-import checks pass.
No generated-note placement/browser/visual fidelity is claimed.

Basic note graph→SF→native records round-trip exactly, including colliding foot/
endnote IDs. A matching explicit native body-marker atom at offset zero maps to a
body label style, not a recursive citation. Contradictory/dangling graph records
reject. Independent rich citation/body-label variants, provenance/marker defaults,
notes with lists/assets, note numbering/placement settings, and native generated
note layout remain unfinished. No public native format/API/CLI is exposed.
Next complete remaining rich native mappings and actual native rendering evidence.


Private rich-body workers now map actual native table stream/source ownership
through SF, including combined row/column merges, nested tables, fixed widths,
equal full-width default columns and repeated headers. Vertical continuations use
empty rowSpan0 records; covered SF cells remain omitted. Contradictory source/
stream counts, overlap, missing/orphan sources and visible continuation content
reject before a partial document is returned. Mixed text/table paragraphs retain
ordered text/table blocks with an explicit normalization diagnostic.

Pinned SDK parser/view/skeleton geometry and TextX edit/save checks qualify these
records: the cell following a two-column vertical merge stays at x240px, nested
table geometry remains present, default-width tables are visible and Go re-imports
a native cell edit. This is structural/geometry evidence without canvas font
metrics or browser/visual layout qualification. No public .vdocs API/CLI exists.
Lists, assets/resources, notes, section/story settings, provenance/intent and
strict envelope/JSON decoding still require implementation. Next continue these
rich mappings, not a public wrapper around the current subset.

Private text workers now map paragraphs/headings (including level6), direction,
page-break-before, point-size typography, language, links and soft line breaks
through the actual SF. Overlapping/unsorted native text styles select the last
whole record; named/default/direct style layers resolve with cycle checks. Native
visible edits remain authoritative. Optional pinned SDK QA reads generated records,
uses TextX to replace astral heading text, saves, then Go re-imports the edit.
This is text-level interoperability, not rich adapter or layout completion.

Unknown presentation emits diagnostics; hidden text and other unmapped containers reject
rather than returning a partial document. Remaining rich containers, section/story settings,
provenance/nonvisual annotations, strict envelope decoding and full native edit/
layout qualification remain open. No public format/API/CLI was added. Next extend
these workers to lists/notes/assets and section/story ownership.

The native adapter's private first layer now validates UTF-16 scalar/range bounds,
paragraph/section/nested-table tokens and style/annotation/custom-block/soft-break
metadata positions. No public .vdocs conversion is exposed yet. Complete semantic
mapping through SF, assets/resources, lists/merges/sections/stories and native edit/
save qualification remain required; this foundation is not adapter completion.

Actual Univer 1.0.4 readers exposed stale published prose comments: table row/end
controls are0x0E/0x0F, while0x1E/0x1F belong to annotations. Cells require their own
CR/LF section terminator. Go token trees/UTF-16 extents now match the executable
parseDataStreamToTree exactly for a hand-authored rich native reference. Actual
DocumentDataModel/DocumentViewModel checks styled astral text, inclusive link range,
note anchor/body, nested table sources and unchanged stream text; the pinned
unmodified Colleaf validator accepts its envelope. Geometry is pixels; fs is pt.
See COLLEAF_FORMAT.md for exact source/hash/reader evidence and reproduction.

The codec rejects split surrogate boundaries, invalid UTF-8, malformed token
nesting, contradictory metadata positions/ranges and unbound table/custom-block
controls. Context cancellation and existing nesting bounds apply; app quotas and
permission gates are absent. All Go production dependencies and public API/CLI
remain unchanged. Runtime is pure Go; Node/Univer are ignored QA installations.
Full/focused/race/vet/modules/CGO-disabled/fuzz evidence is in verification.json.

Previous fd2fdd7 qualifies independently cited nested PDF notes, original parents/
order and wrapper Ref provenance;8061131 qualifies original Office sources and
positional-tab separation. The full goal remains active/incomplete. Next is both
rich native mappings, not another declaration-only or plain-text milestone.

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
  for the remaining representative original DOCX/semantic/native adapter work.

Last fully committed codec checks: tests 15.129s/0.371s, race 175.287s/2.119s,
vet/modules, host CGO-disabled execution, Linux amd64/arm64 cross-builds and diff
checks pass; gzip/plain SF fuzz 128,050/260 executions pass. Current audit-fix
checks pass: tests 14.736s/0.686s, race 175.468s/2.199s, vet/modules, host CGO-
disabled build/focused execution, Linux amd64/arm64 cross-builds and diff check.
Verification.json records the durable evidence. Linux builds are not
Linux execution evidence. Optional Node/Python/LibreOffice remain QA only.

## Next concrete implementation order

1. Implement/qualify the actual Colleaf native snapshot adapter through the shared
   SF. Include UTF-16 boundaries, rich source maps, notes/stories and resources;
   do not stop at plain-text wrapping. Leave applications/backends untouched.
2. Qualify remaining broader untagged note recovery and original source consistency,
   using PDF_NOTES.md and preserving source facts/content. Newly qualified nested
   definition/wrapper cases must remain regression evidence, not repeated blockers.
3. Preserve the now broadened original DOCX corpus and independent expectations;
   carry its measured source/producer/review/field limits into the final audit.
   Fix further real failures without changing original oracles.
4. Audit remaining source-structure consistency/resource and public API evidence,
   repeat the full acceptance matrix and final checks, and complete the goal only
   after every original criterion is proven. There is no external blocker now.

```sh
# Next: implement actual native integration from the pinned schema.
cat docs/COLLEAF_FORMAT.md
rg -n 'IDocumentData|dataStream|customRanges|tableSource' output/codec
GOWORK=off go test -race -run 'TestPDFNested|TestPDFRedundantNote|TestPDFNoteWrapper' -count=1 .
# Full nested source/generation/reader commands: PDF_NOTES.md
output/qa-venv/bin/python scripts/generate_pdf_semantic_audit.py
bin/manydoc extract -format sf -o output/audit/explicit-note.sf output/audit/explicit-note.pdf
bin/manydoc extract -format sf -o output/audit/pagination-artifact.sf output/audit/pagination-artifact.pdf
output/qa-venv/bin/python scripts/verify_pdf_notes.py
# Full probe/generation/reader commands: docs/PDF_NOTES.md
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
