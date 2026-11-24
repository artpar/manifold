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

## Latest progress: acknowledged DOCX precision and sectioned note parity

AllowLossy now permits nearest half-point DOCX font-size projection, ties upward,
with at most .25pt deviation and checked original intent in md:fontSize. Strict
output retains ErrUnsupported. Both standard sz/szCs must agree before original
precision is restored; reimport diagnoses portable precision. Values outside the
representable range reject without partial output or caller mutation. This applies
to body/heading/table/list/stories/notes and first/repeated/variant styled marks.
Native PDF's existing .01pt precision and explicit font/glyph rules are unchanged.

A new sectioned styled-note regression reproduced a nil-pointer panic: separate
writer initialization omitted custom/variant/style catalogs. Both paths now share
initializeNotes; full note semantics round trip. Duplicate expanded XML attributes,
including aliases, now reject instead of selecting an ambiguous first value.
Read DOCX_PRECISION.md for exact behavior, checks and reproduction.

The unchanged original 71-page tagging guide now exports to DOCX. Independent
standard XML checks cover all 87,442 projected runs; editable structure, all
formatting/original point sizes, note definitions/sites/backlinks and seven image
instances retain semantics/assets on reimport. Fourteen image dimensions differ
within one EMU. LibreOffice's reflowed 39 pages include both notes once on page 39;
cover/contents, technical text/code/list and notes pages were inspected. A sectioned
example's two Word pages show both 6.959646pt marks at the declared portable 7pt.
Actual ProseMirror validates 89,433 nodes/marks/attributes across those round trips.
No original pagination, font substitution or Word-equivalent fidelity is claimed.

Previous body-label templates/backlinks are 580411d; identified note relationships
894ee74; display variants 09315a1; explicit graph/provenance 60bf496. Existing native
and standard Word links remain independently checked. LibreOffice's missing custom
note-label PDF hyperlinks and installed-font substitution remain reader limits.
Current checks/results are in verification.json; ignored evidence is in
output/note-precision and output/note-inference. No production modules, runtime
services/executables/CGO/uploads, app code or access/quota boundaries changed.
The full goal remains active/incomplete; remaining work is ordered below.

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

1. Complete heterogeneous/mixed-direction note labels and remaining
   nested/ActualText/untagged graphs;
   use the original-guide evidence in PDF_NOTES.md, preserving source facts/content.
2. Implement/qualify the actual Colleaf native snapshot adapter through the shared
   SF. Include UTF-16 boundaries, rich source maps, notes/stories and resources;
   do not stop at plain-text wrapping. Leave applications/backends untouched.
3. Broaden pinned, legally reusable original DOCX producer/rich-feature inputs;
   establish independent structure/asset/reference oracles and meaningful visual
   output checks. Fix actual failures without changing the original expectations.
4. Audit remaining source-structure consistency/resource and public API evidence,
   repeat the full acceptance matrix and final checks, and complete the goal only
   after every original criterion is proven. There is no external blocker now.

```sh
# Next implementation: rich label content through established SF inline nodes,
# retaining exact marks/relationships, with one text authority and normal layout.
rg -n 'pdfNoteMark|recoverNote|structureReplacement|ActualText' pdf_structure_notes.go pdf_structure*.go
GOWORK=off go run ./examples/notes -variant-labels -styled-labels -fractional-sizes -sectioned -out output/note-precision/sectioned
bin/manydoc extract -format sf -o output/note-precision/sectioned/notes-docx.sf output/note-precision/sectioned/notes.docx
output/qa-venv/bin/python scripts/verify_docx_precision.py
GOWORK=off go test -race -run 'TestPDFAdjacent|TestPDFPhysicalLinkNoteInference|TestDOCXNoteRelationshipInference|TestPDFNoteLinkAmbiguity' -count=1 .
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
