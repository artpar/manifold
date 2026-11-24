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

## Latest progress: independent citation/body display

NoteReference.Label and Note.BodyLabel now distinguish inherited, literal and
explicitly empty display independently from kind/ID and automatic counter order.
Explicit PDF forward/reverse references with different marks retain typed sites
and one rich body; leading Lbl values/LabelSource remain independent. Proven empty
reference containers recover markerless atoms. Missing paint/MCR associations stay
uncertain and do not establish empty marks. PDF_NOTES.md is the current contract.

DOCX variants use standard real-note anchors, literal body labels and visible
hyperlinks to body bookmarks, with checked shape/target/style/namespace metadata.
Mixed automatic/variant Word export requires AllowLossy for literal portable
numbering; SF automatic intent round-trips in checked metadata. Ordinary note
output keeps its existing representation. Native body labels now validate fonts,
glyphs/shaping and widths. Homogeneous RTL citations work; mixed-direction atomic
labels produce an explicit shaping error. Per-site run formatting is retained.

Independent original paint/empty-container, python-docx/lxml, pypdf/PyMuPDF and
actual ProseMirror checks cover distinct/empty fields, rich bodies/raster pixels,
all editable DOCX round-trip values and visible citation-to-body coordinates.
All four native/LibreOffice example pages were inspected. New fuzz seeds include
variants. Final full tests (15.737s/0.363s), full production race (181.448s/2.114s),
final focused/new-seed race (2.727s), vet/modules, host CGO-disabled execution
(0.408s), Linux amd64/arm64 cross-builds, DOCX/SF fuzz (331/57,871 executions) and
diff checks pass. Exact check scope is in verification.json. Evidence is ignored
under output/note-variants. Primary source/oracle bytes and dependencies are intact.

The full goal remains active/incomplete. Required work includes rich multi-node/
internal mixed-direction label layout, nested/enclosing-ActualText graphs,
identified adjacency/geometric note inference without Ref, native Colleaf adapter,
broader original DOCX producer/corpus/visual qualification and final acceptance.
The previous explicit PDF note graph/provenance milestone is 60bf496; literal
labels are 44fe42f and physical artifact structure is 10459dd.

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

1. Complete rich label layout and remaining note graphs/identified inference;
   use the concrete cases in PDF_NOTES.md, preserving source evidence and content.
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
GOWORK=off go test -race -run 'TestPDFExplicitNotes|TestPDFNote|TestDOCXListTable' -count=1 .
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
