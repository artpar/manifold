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

## Latest progress: rich body labels and measured bidi gutters

Note.BodyLabelContent now holds literal rich label runs; SF uses ordinary text
nodes/marks in body_label_content. This alternative excludes BodyLabel and
BodyLabelStyle, so there is one body-text authority. Fonts/sizes/colors/emphasis,
per-fragment links and source positions survive SF/JSON/gzip. PDF recovery retains
differently styled leading-Lbl runs instead of their common intersection.
Uniform labels retain the previous template representation.

Ordinary and sectioned Word export uses checked matching bookmark ranges around
actual visible runs/hyperlinks. The range binds to the real note; metadata does
not duplicate body text. Invalid roles, hidden/nontext content, conflicting
sources of authority and private-prefix destination collisions are tested. Inserted
revisions retain final-visible text; deleted history/comments diagnose, and an
empty final label becomes explicit markerless display without invented numbering. Native
PDF measures rich foot/endnote marks through its normal font/glyph/bidi/layout
paths. Marker bidi offsets now use the intrinsic width, preventing alignment in
an entire body column from escaping the gutter.

Independent source MCID operators establish a 10pt black/8pt red mark; source/SF/
Word/native paint, geometry, list/table/image pixels and source evidence pass.
A sectioned example preserves all three rich marks and native fragment URI/named
backlinks. Supplied-DejaVu Arabic/Latin marks qualify both gutter sides. Actual
ProseMirror checks 138 nodes/marks/attributes; eight generated pages were inspected.
Read RICH_NOTE_LABELS.md for exact semantics, independent checks and reproduction.

Independent QA rejected an inline content-control prototype that moved/restyled
labels; the final ordinary bookmark-range output passes. LibreOffice still uses
installed fonts, omits custom label PDF hyperlinks, and chooses a dedicated Letter
endnote page in the example (native pages 420pt square, Word main ~419.556pt).
No source-layout or Word-equivalent print-fidelity claim is made. This producer
qualification remains open, separately from verified standard settings/SF meaning.

Current check results are in verification.json; ignored QA is output/rich-note-labels.
Previous precision/sectioned-note parity is 2b791c0; common body-label templates
580411d; identified note relationships 894ee74. No runtime/module/app/access/quota
boundary changed. The full goal remains active/incomplete; remaining work follows.

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

1. Complete heterogeneous/internal mixed-direction citation marks and remaining
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
# Next: retain per-fragment citation marks and shape an atomic mixed-direction
# label in its own box; preserve relationship/preceding-anchor line admission.
rg -n 'noteMarkLevel|structureReplacement|citation|run.Note' pdf_structure_notes.go pdf_text.go pdf_bidi.go
GOWORK=off go run ./examples/notes -rich-body-labels -sectioned -out output/rich-note-labels/example
# Full source, reader and renderer commands: RICH_NOTE_LABELS.md
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
