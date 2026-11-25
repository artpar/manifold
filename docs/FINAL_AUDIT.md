# Original goal acceptance audit

Audited 2026-11-25 against the original objective, rather than UniOffice API parity
or Word pagination identity. The original acceptance criteria are satisfied for the declared support
contract. No required capability remains unimplemented in this audit. STATUS.md remains the
current recovery entry point; verification.json records command scopes and history.

## Required capability and evidence

| Requirement | Current implementation and independent evidence |
| --- | --- |
| Established shared rich representation, selected first | manydoc-prosemirror/1, standard ProseMirror JSON with an explicit schema profile; alternatives, extensions and actual pinned Colleaf/Univer inspection are documented in STRUCTURED_FORMAT.md and COLLEAF_FORMAT.md. Actual prosemirror-model validates the wire documents. No JavaScript engine is required at runtime. |
| DOCX → SF and SF → DOCX | Original Word MQTT source tests verify headings, formatting, tables/merges, relationships and original JPEG. Eight additional original Word/Mac Word/LibreOffice exports have source-only ZIP/XML oracles and normalized full-SF round trips. REAL_DOCX_CORPUS.md and DOCX_PRECISION.md record independent readers and visual checks. |
| SF → PDF | Native measured layout handles wrapping, pagination, merged/nested/continued/repeated tables, images, section frames, running stories and rich notes. Independent geometry, font programs/GIDs, links, raster pixels and page renders qualify the declared contract. See TABLE_WIDTHS.md, FONT_SUBSETTING.md, PDF_NOTE_PAGES.md and the corresponding verification ledger entries. |
| PDF → SF | Explicit logical ownership, physical pages/positions/navigation and embedded raster assets survive; untagged geometric paragraphs/headings/lists/tables/notes disclose inference and uncertainty. Original137-page Word export has 90 page-local tables/3,009 origins and 11 continuation references. The original InDesign publication and original 71-page tagging guide provide independent producer evidence. See TAGGED_TABLE_SPANS.md, PDF_NAVIGATION.md, PDF_NOTES.md and PDF_NOTE_KIND.md. |
| Practical rich features | The shared model/profile and both writers carry paragraphs, headings, formatted text, links/internal navigation, lists, merged tables, images, metadata, page/section settings, conditional headers/footers and rich foot/endnotes. Dedicated feature tests compare structure, marks, relationships and assets. Sections, notes, lists and merged-table examples provide usable public-API entry points. |
| Revisions, comments, fields and unsupported constructs | DOCX uses final-visible revisions, cached field results and diagnosed loss of comment/history/control/evaluation semantics. Original exports qualify this behavior. Writers gate known representation losses with AllowLossy and still reject unrenderable content. COMPATIBILITY.md and REAL_DOCX_CORPUS.md define the policy. |
| PDF facts distinct from inference | SourceInfo records explicit/inferred ownership, evidence, confidence, geometry, original IDs/Ref edges and physical navigation. Artifact page facts do not become invented header templates; note kind inference is separate from relationship inference. Skip controls expose physical paint. Missing/contradictory/ambiguous structure has explicit outcomes. |
| Unicode and font behavior | Embedded Go faces and explicit caller-supplied TTFs, measured glyph positioning, contextual shaping, qualified Arabic/Hebrew bidi and original supplementary-plane source checks exist. Missing glyphs, guarded unsupported shaping and incompatible graphemes reject before output. No silent font fallback. COMPATIBILITY.md declares script/font limits. |
| Supported semantic round trips | Exact SF/gzip/model JSON and normalized DOCX comparisons cover rich source graphs, styles, assets, note identity, sections and lists; the independent editor validates actual wire trees. Physical PDF provenance has an explicit reflow-loss acknowledgement, rather than a false editable round-trip claim. |
| Stable embedding API, CLI and examples | Public Read/Write with context, readers/writers, options and typed formats/errors; structured tree codecs/bridges; CLI tests exercise every required path, gzip and atomic failure. examples/sections freshly exercises all four operations and exact editable DOCX/SF equality. |
| Pure Go runtime and application boundary | go.mod/THIRD_PARTY.md track reusable open-source components. CGO-disabled host execution and Linux cross-builds qualify runtime compilation. Production invokes no Office/Python/Node process, conversion service or license server; identity, authorization, storage, quotas and action execution remain outside the library. |
| Independent readers, visual inspection and representative originals | ProseMirror, standard XML/python-docx, pypdf, MuPDF, fontTools and LibreOffice are optional QA. Actual exported source bytes/hashes/oracles are retained; source and output checks are separate. Representative rendered pages and scoped visual evidence are documented, including 33 original/native/Word corpus pages and the latest native/Word footnote pages. |
| Tests, malformed input, race, vet and CGO builds | Full normal tests pass 75.053s/CLI cached; full final-production race passes 770.544s/CLI cached with -timeout20m. Later test-only additions pass current focused race 7.016s. Targeted race, malformed owner/index tests, a 10,000-execution ParentTree fuzz campaign, vet, module verification and CGO-disabled builds pass. Exact scopes and later test-only additions must remain explicit in verification.json. |
| Recoverable knowledge and ongoing commits | STATUS/ARCHITECTURE/DECISIONS/COMPATIBILITY, feature contracts, source manifests and the verification ledger are committed with implementation milestones. Generated documents, environments and binaries remain ignored. |

## Limits and scope decisions

These remain explicit compatibility limits, not replacements for the capabilities
above: arbitrary OOXML editing/history, field execution, OCR, every PDF graphics
operator/image codec/font format/script, exact source visual layout and formal
PDF/A/PDF/UA/WTPDF conformance. The original objective specifically permits explicit
missing-glyph/unsupported-shaping outcomes and excludes direct Word-to-PDF visual
fidelity and full UniDoc API parity. Common supported rich content is preserved;
unsupported/uncertain source behavior is diagnosed or rejected as documented.

Known independent-reader discrepancies remain executable and recorded: ambiguous
CID width overlap, managed CMYK/JPEG interpolation differences, a pinned pypdf LZW
EarlyChange limitation, and the pinned optional Colleaf browser's repeated-note
ordering behavior. They do not establish missing core operations. The public
offline .vdocs adapter is additional interoperability work; complete browser/API
parity with that third-party editor is not an original acceptance gate.

Initial pdfcpu parsing can allocate before the later decoded-stream guards run.
The library does not promise a universal process memory cap or exhaustive hostile
PDF safety. Existing bounds, cancellation, malformed tests and fuzz checks are
qualified individually in PDF_RESOURCES.md and PDF_PARSER_STREAMS.md. Application
resource isolation is distinct from document conversion and no app quota gate is
introduced here.

The audit found one actual original-source regression, rather than simply accepting
all existing tests: f171ab4 rejected direct ParentTree child dictionaries in the
original tagging guide. The current recovery diagnoses that number-tree conformance
issue while preserving reciprocal checks. Its two real notes now infer footnote
placement from original separator/label/citation geometry. PDF_NOTE_KIND.md records
the unchanged source, exact pre-change failure, full current-SF comparison and
independent glyph/relationship/raster/visual evidence.

## Final qualification commands

```sh
GOWORK=off go test ./...
GOWORK=off go test -race -timeout 20m ./...
GOWORK=off go vet ./...
GOWORK=off go mod verify
CGO_ENABLED=0 GOWORK=off go build -o bin/manifold ./cmd/manifold
output/qa-venv/bin/python scripts/generate_poi_qa.py
output/qa-venv/bin/python scripts/verify_poi_corpus.py
output/qa-venv/bin/python scripts/verify_structured_corpus.py
GOWORK=off go run ./examples/sections
output/qa-venv/bin/python scripts/verify_sections.py
output/qa-venv/bin/python scripts/verify_pdf_note_kind.py --render-dir output/pdf-note-kind --historical-sf output/note-inference/guide-styled.sf.gz
git diff --check
```

The optional scripts need their documented local QA environment/fonts/producer
renders. They are not runtime dependencies. Do not regenerate source oracles to
match output. The closure commit reconciles this audit, STATUS.md, ACCEPTANCE.md and
verification.json. Future behavior changes should repeat affected source/reader
checks and the required Go matrix, then update these records in the same commit.


## Final verification scope

- Full normal suite on the final tree: 75.053s, CLI/internal bidi cached; all packages pass.
- Full race suite on final production code: 770.544s, CLI/internal bidi cached; no races. Later test-only QA export/fuzz-seed coverage passes focused race 7.016s.
- Final vet and module verification pass. CGO-disabled host CLI builds and executes; Linux amd64/arm64 cross-builds pass (not executed on Linux).
- ParentTree fuzz:10,000executions/two workers/9.025s passes, alongside all malformed suite cases. No exhaustive safety claim.
- Current CLI regenerates all four paths for eight original exports; independent corpus checks pass33 producer/native pages. Section example/reader and all 51 accepted SF corpus documents/9,034 ProseMirror nodes and marks pass.
- Original guide ownership/paint/views, current complete-SF placement-only comparison, native/Word footnote navigation/raster and visual checks pass. Historical56 synthesized list-gap differences are explained and checked, not painted-content omissions.

Raw logs are ignored under output/pdf-note-kind-* and output/final-goal-*;
verification.json records scopes. Source provenance, independent oracles and
regeneration commands remain in the linked feature contracts. Known compatibility
limits above remain part of the delivered contract.
