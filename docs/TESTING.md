# Testing and verification

All ordinary tests are self-contained Go tests. Python and Poppler are optional
independent verification tools, never production dependencies.

## Required checks

```sh
GOWORK=off go test ./... -count=1
GOWORK=off go test -race ./... -count=1
GOWORK=off go vet ./...
GOWORK=off go mod verify
CGO_ENABLED=0 GOWORK=off go build -o bin/manydoc ./cmd/manydoc
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GOWORK=off go build -o bin/manydoc-linux-amd64 ./cmd/manydoc
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 GOWORK=off go build -o bin/manydoc-linux-arm64 ./cmd/manydoc
git diff --check
```

Cross-builds establish compilation, not execution on Linux. Keep production
dependencies separate from fixture tooling and avoid global configuration.

## Corpus and regression tests

`testdata/corpus/manifest.json` describes 31 DOCX and 33 PDF named cases, including
malformed inputs expected to reject. `TestCorpusHashes` verifies their SHA-256
hashes. Fixtures are mostly synthetic; one PDF is the published W3C dummy file.

Tests prove preservation of previously lost DOCX visible content, valid minimal
packages, page-specific PDF fonts, glyph-positioned spaces, Unicode/CJK,
XObjects/widget appearances, valid quote operators, encryption, generation
round trips, concurrent independent operations and cancellation. CLI tests also
verify that failure preserves an existing output and removes its temporary file.

`TestMalformedInputsNeverPanic` applies 100 deterministic mutations per input
format. These are robustness checks, not 200 real-world compatibility cases.

## Fuzzing

```sh
GOWORK=off go test -run '^$' -fuzz '^FuzzReadDOCX$' -fuzztime=15s -fuzzminimizetime=1s -parallel=2 -timeout=35s .
GOWORK=off go test -run '^$' -fuzz '^FuzzReadPDF$' -fuzztime=15s -fuzzminimizetime=1s -parallel=2 -timeout=35s .
GOWORK=off go test -run '^$' -fuzz '^FuzzUnicodeMap$' -fuzztime=15s -fuzzminimizetime=1s -parallel=2 -timeout=35s .
```

Fuzzing is nondeterministic and budget-limited. Preserve any discovered failure
as a permanent regression fixture. A short passing campaign is not evidence of
exhaustive safety or full standards conformance.

## Independent generated-output checks

```sh
GOWORK=off go run ./examples/report -out output
uv venv output/qa-venv
uv pip install --python output/qa-venv/bin/python -r scripts/requirements.txt
output/qa-venv/bin/python scripts/verify_outputs.py output
output/qa-venv/bin/python scripts/compare_pdf_text.py
pdftoppm -f 1 -l 2 -scale-to 1200 -png output/report.pdf output/report-page
```

The first script reopens DOCX with python-docx and checks metadata, hyperlinks,
formatting, a table, image, headers/footers and text. It reopens PDF with pypdf,
checks two pages and compares expected content with Poppler. The second compares
all 28 non-malformed named PDF fixtures with Poppler after whitespace
normalization, including password cases. Inspect every generated PDF page PNG.

DOCX structure checks do not establish visual Word rendering. No Microsoft Word
or LibreOffice rendering acceptance has been completed. The generated PDF
example has been visually inspected, including Unicode, table layout and both
pages; that only qualifies this example.

## Regenerating fixtures

```sh
output/qa-venv/bin/python scripts/generate_fixtures.py
GOWORK=off go test ./... -count=1
```

The generator uses python-docx, ReportLab, pypdf and PyMuPDF with local assets.
Regeneration updates hashes because ZIP timestamps/package versions may change.
Review changed fixture bytes and expectations; do not blindly update oracles.
See THIRD_PARTY.md for asset provenance and notices.

## Recording a new handoff milestone

Preserve actual command output before regenerating the compact verification
summary. The summary script asserts that the captured checks passed; it does
not run checks or infer success from absent logs.

```sh
mkdir -p output
GOWORK=off go test -json -cover ./... -count=1 > output/tests-final.json
GOWORK=off go test -race ./... -count=1 > output/race-final.log 2>&1
GOWORK=off go vet ./... > output/vet-final.log 2>&1
GOWORK=off go mod verify > output/modules-final.log 2>&1
```

Capture the three fuzz commands above to output/fuzz-docx-final.log,
output/fuzz-pdf-final.log and output/fuzz-cmap.log respectively. Complete the
CGO-disabled builds and independent example/PDF comparisons above, then run:

```sh
python3 scripts/record_verification.py
```

Review docs/verification.json, update STATUS.md with changed limitations and
outcomes, and retain a local Git checkpoint. Do not publish remotely unless
requested. Never reset or clean an unfamiliar working tree to recover progress.

## Structured format milestone

Go tests include SF structure/attribute validation, preservation of all fields
in every accepted corpus model, diagnostic propagation, CLI four-path wiring,
malformed SF and cancellation. These tests establish the existing model's
preservation, not the missing rich features in ACCEPTANCE.md.

```sh
GOWORK=off go test -run '^$' -fuzz '^FuzzReadStructured$' -fuzztime=10s -fuzzminimizetime=1s -parallel=2 -timeout=25s .
CGO_ENABLED=0 GOWORK=off go build -o bin/manydoc ./cmd/manydoc
npm install --prefix output/sf-qa --no-audit --no-fund prosemirror-model@1.25.13
python3 scripts/verify_structured_corpus.py
GOWORK=off go run ./examples/report -out output
NODE_PATH=output/sf-qa/node_modules node scripts/verify_structured.cjs output/report.sf
mkdir -p output/sf-generated
bin/manydoc convert -o output/sf-generated/report.docx output/report.sf
bin/manydoc convert -o output/sf-generated/report.pdf output/report.sf
output/qa-venv/bin/python scripts/verify_outputs.py output/sf-generated
pdftoppm -f 1 -l 2 -scale-to 1000 -png output/sf-generated/report.pdf output/sf-generated/page
```

Inspect both PDF pages. ProseMirror validates the committed schema via its own
reader, including tree shapes, text, marks and attributes. It is optional QA;
neither npm nor JavaScript is needed to build, run or test the Go library.
The original `record_verification.py` summarizes initial-baseline log names;
do not rerun it over stale logs and call the result current SF evidence.

## Merged tables and DOCX visual check

Tests include hand-authored OOXML origins/continuations, combined row/column
spans, fully covered rows, preserved continuation content/diagnostics, orphan and
mismatched regions, malformed model grids, formatting/relationship round trips,
exact PDF cell rectangles, merged-group pagination and nested DOCX width bounds.
The DOCX and SF fuzzers include merged-table seeds.

```sh
GOWORK=off go run ./examples/merged-tables -out output/merged-tables
output/qa-venv/bin/python scripts/verify_merged_tables.py output/merged-tables
NODE_PATH=output/sf-qa/node_modules node scripts/verify_structured.cjs output/merged-tables/merged-table.sf
pdftoppm -scale-to 1200 -png output/merged-tables/merged-table.pdf output/merged-tables/page
```

On the current macOS QA host, LibreOffice 7.6 is available. Use an isolated
profile so QA does not modify the user's normal LibreOffice session/config:

```sh
/Applications/LibreOffice.app/Contents/MacOS/soffice -env:UserInstallation=file:///Users/artpar/workspace/code/github.com/artpar/manydoc/output/lo-merge-profile --headless --convert-to pdf --outdir output/merged-tables/docx-render output/merged-tables/merged-table.docx
output/qa-venv/bin/python scripts/verify_merged_tables.py output/merged-tables --docx-render output/merged-tables/docx-render/merged-table.pdf
pdftoppm -scale-to 1200 -png output/merged-tables/docx-render/merged-table.pdf output/merged-tables/docx-render/page
```

Inspect both the native PDF and DOCX-rendered page PNG. This caught fixed-width
DOCX grids overflowing a small page; the corrected example retains every cell.
This is a synthetic example visual check, not Word equivalence. LibreOffice and
Python/Poppler remain optional QA tools, never production runtime dependencies.
Adapt the isolated profile path if reproducing on another host.

## Explicit lists

The Go suite checks SF starts/boundaries/items, legacy adaptation, independent
OOXML multilevel/startOverride/lvlRestart oracles, inherited styles and numId=0,
continuation after non-list text, item images/tables, inline image normalization,
invalid range metadata, and PDF marker/content preservation across pages.
DOCX/SF fuzz seeds include both ordinary multilevel numbering and bookmarked
list documents. Dedicated list tests compare structure and relationships; they
are not only text-presence checks.

```sh
GOWORK=off go run ./examples/lists -out output/lists
GOWORK=off go run ./examples/lists -out output/lists/paginated -page-height 220
NODE_PATH=output/sf-qa/node_modules node scripts/verify_structured.cjs output/lists/lists.sf output/lists/paginated/lists.sf
output/qa-venv/bin/python scripts/verify_lists.py output/lists
output/qa-venv/bin/python scripts/verify_lists.py output/lists/paginated --expected-pages 3
pdftoppm -scale-to 1000 -png output/lists/lists.pdf output/lists/native
pdftoppm -scale-to 1000 -png output/lists/paginated/lists.pdf output/lists/paginated/page
/Applications/LibreOffice.app/Contents/MacOS/soffice -env:UserInstallation=file:///Users/artpar/workspace/code/github.com/artpar/manydoc/output/lo-list-profile --headless --convert-to pdf --outdir output/lists/docx-render output/lists/lists.docx
output/qa-venv/bin/python scripts/verify_lists.py output/lists --docx-render output/lists/docx-render/lists.pdf
pdftoppm -scale-to 1000 -png output/lists/docx-render/lists.pdf output/lists/docx-render/page
```

The verifier independently checks six numbering identities/starts, ten numbered
items, invisible boundary names, link/bold, native PDF indentation at three
levels, continuation/cell indentation, content and PDF annotations. It rejects
visible HYPERLINK instructions in an independent DOCX render. Inspect all five
native/DOCX-rendered pages from these commands. Rendering is targeted synthetic
QA, not a claim of general Word equivalence or an application runtime dependency.

## Typed notes and reference-aware layout

The Go suite verifies inline atom identities, graph validation, draft persistence
vs rendered-anchor requirements, note-definition order and cross-kind IDs,
independent OOXML note-part/reference oracles, rich note-local assets/relationships,
repeat NOTEREF bindings/cached marks, settings/section overrides, merge-continuation
atoms, malformed metadata, PDF note page-object destinations, separate adjacent
annotations and continuation content. DOCX/SF fuzzers include note seeds.

```sh
GOWORK=off go run ./examples/notes -out output/notes
GOWORK=off go run ./examples/notes -out output/notes/continued -page-height 220 -long-note
NODE_PATH=output/sf-qa/node_modules node scripts/verify_structured.cjs output/notes/notes.sf output/notes/continued/notes.sf
output/qa-venv/bin/python scripts/verify_notes.py output/notes
output/qa-venv/bin/python scripts/verify_notes.py output/notes/continued --continued
pdftoppm -scale-to 1000 -png output/notes/notes.pdf output/notes/native
pdftoppm -scale-to 1000 -png output/notes/continued/notes.pdf output/notes/continued/page
/Applications/LibreOffice.app/Contents/MacOS/soffice -env:UserInstallation=file:///Users/artpar/workspace/code/github.com/artpar/manydoc/output/lo-notes-profile --headless --convert-to pdf --outdir output/notes/docx-render output/notes/notes.docx
output/qa-venv/bin/python scripts/verify_notes.py output/notes --docx-render output/notes/docx-render/notes.pdf
pdftoppm -scale-to 1000 -png output/notes/docx-render/notes.pdf output/notes/docx-render/page
```

Inspect all seven native PDF pages and both normal DOCX-rendered pages. The
verifier checks three definitions/four reference sites, preserved public IDs/order,
OOXML content types and relationship targets, cached repeat fields, note formatting,
actual PDF destination pages, superscripts, line bounds/spacing, separator/body
separation and continuation text/footer counts. It caught the wrong SetLink page
argument despite correct text. These are synthetic examples. Nontext PDF footnotes,
custom numbering/separator behavior and representative real exported notes remain
unfinished qualification/layout work.

## Positioned PDF recovery

```sh
GOWORK=off go test -run 'TestPDF(Positioned|PageCoordinate|TextMatrix|FormAnd|Composite|Marked|Raster|Inline|Source|ActualText)' -count=1
CGO_ENABLED=0 GOWORK=off go build -o bin/manydoc ./cmd/manydoc
output/qa-venv/bin/python scripts/verify_pdf_recovery.py
NODE_PATH=output/sf-qa/node_modules node scripts/verify_structured.cjs output/pdf-recovery/positioned.sf
```

The Go fixtures are hand-authored object streams independent of manydoc's writer.
They assert actual coordinates/matrices, rotation/crop/UserUnit, q/Q and quote
operators, CID/ligature advances, Form/annotation placement, MCID/ActualText,
PDFDocEncoding, raster pixels/alpha, inline filter framing and strict SF provenance.
Malformed scopes, undefined codes, truncated samples and contradictory source
geometry reject. PDF fuzzing includes transformed, marked, inline and masked-image
seeds. Short campaigns qualify only their recorded budgets.

The independent script authors a two-page source, checks pypdf structure/assets,
MuPDF text/image geometry and annotation pixels, raster mask samples and ActualText
SF semantics. It exercises recovered SF through DOCX/PDF with acknowledged reflow
and checks content/assets using independent readers. It writes source/reflow page
PNGs for inspection. The source is a targeted synthetic fixture, not a real-export
corpus or evidence of tagged/inferred semantic recovery or original-layout fidelity.
The source's XY paints intentionally extract as ActualText `fi`.

Do not compose PyMuPDF's unscaled rotation_matrix translation with UserUnit-scaled
raw origins in the tested build. Use physical page dimensions and confirm pixels.
Likewise, count unique pypdf resources separately from actual image paint instances
because gofpdf shares its resource dictionary across pages.

## Tagged structure and original published export

```sh
GOWORK=off go test -run 'TestPDF(Tagged|Published|SimpleFont)' -count=1
CGO_ENABLED=0 GOWORK=off go build -o bin/manydoc ./cmd/manydoc
output/qa-venv/bin/python scripts/verify_pdf_structure.py
output/qa-venv/bin/python scripts/verify_published_pdf.py
NODE_PATH=output/sf-qa/node_modules node scripts/verify_structured.cjs output/pdf-structure/tagged.sf output/published-pdf/publication.sf
```

`pdf_structure_test.go` contains independent hand-authored tree/oracle cases for
logical order, paragraphs spanning pages, role maps, class/local merged-cell
attributes, list labels/starts/item provenance, Form-vs-page MCID scope, Figure
MCR/OBJR references/Alt, ActualText, empty paragraphs, URI ownership and malformed
cycles/parents/owners/page references. Padded CMap conflicts reject. Unsupported
labels retain literal content. The original published file/hash and its structural
counts are verified independently of the production writer.

The optional structure script creates a two-page targeted source with a real
forward tree and ParentTree, checks its SF, then checks generated DOCX/PDF using
pypdf, MuPDF, python-docx and lxml. Its three native PDF pages and a one-page LibreOffice 7.6 DOCX rendering were
inspected.
The published-export script compares original `/K`/RoleMap/annotation structure,
all 20,531 normalized nonspace character counts, SF relationships and generated
DOCX content/metadata. The source's pages 1/8 were inspected. Its 10 lists/57 items
and 16 unique URI targets do not establish general tagged-PDF conformance.
The separate 64-case corpus is unchanged; testdata/real/ adds one original export.

The publication has styled headings role-mapped to P, one unresolved internal
GoTo and a conflicting multi-target Link group; preserve those facts. Its earlier SF -> PDF U+2003 rejection is fixed by measured Unicode typography;
see the later sections for output and corrected implied-paragraph checks.

To reproduce the targeted DOCX visual check (optional QA executable only):

```sh
mkdir -p output/pdf-structure/lo
/Applications/LibreOffice.app/Contents/MacOS/soffice '-env:UserInstallation=file:///Users/artpar/workspace/code/github.com/artpar/manydoc/output/lo-pdf-structure-profile' --headless --convert-to pdf --outdir output/pdf-structure/lo output/pdf-structure/reflow.docx
pdftoppm -scale-to 1200 -png output/pdf-structure/lo/reflow.pdf output/pdf-structure/lo/reflow
```

Adapt the isolated profile path to your checkout. This is development verification,
not a production executable dependency or a claim of original-PDF visual fidelity.

## Unicode typography and paragraph wrapping

```sh
GOWORK=off go test -run 'TestPDF(Text|Discretionary|Typography|LineBoundary|PublishedExportNow|RejectsOversized)' -count=1
GOWORK=off go test -run '^$' -fuzz '^FuzzPDFTextLayout$' -fuzztime=20s -fuzzminimizetime=1s -parallel=2 -timeout=60s
GOWORK=off go run ./examples/typography
output/qa-venv/bin/python scripts/verify_typography.py
CGO_ENABLED=0 GOWORK=off go build -o bin/manydoc ./cmd/manydoc
output/qa-venv/bin/python scripts/verify_published_pdf.py
NODE_PATH=output/sf-qa/node_modules node scripts/verify_structured.cjs output/typography/typography.sf
```

Layout tests assert word continuity across formatting runs, glue/joiner behavior,
grapheme-safe emergencies, hard-break preservation, conditional hyphens, note
anchors, exact logical Unicode/style and physical em advances. Overflow, remaining
combining/shaping needs and oversized default-unit pages reject with no partial
PDF. The saved fuzz case `07e1d9222943762c` covers consecutive soft-hyphen overflow.
Fuzz invariants compare original logical text and measured line bounds, including
run boundaries inside clusters and whitespace at wrapped line ends.

The independent typography script checks 15 space/control advances against em
fractions and the embedded CID metrics, original Unicode/NFC in ActualText,
content bounds, underline/strike lengths and URI relationships. MuPDF may insert
extraction newlines around zero-width carriers; test physical baselines separately
from normalized extraction and inspect standard ActualText directly. The native
one-page PDF and LibreOffice 7.6 DOCX rendering were visually inspected.

The published-script PDF stage now passes nine reflowed pages: metadata, all
16 URI targets, all 20,531 normalized nonspace characters, exact em/hair/punctuation
space counts and bounds. All nine pages were inspected. The earlier tagged-list-body fragmentation is
fixed and is checked structurally as described below; character/URI counts alone
do not qualify original layout or full structural correctness.
The 51 existing corpus SF exports still validate in ProseMirror (12,417 nodes).
Report/merged-table and normal/five-page note independent checks pass after layout
changes. New dependency notices are in THIRD_PARTY.md and third_party/licenses/.

## Tagged implied paragraphs

```sh
GOWORK=off go test -run 'TestPDF(Tagged|CustomList|Published)' -count=1
CGO_ENABLED=0 GOWORK=off go build -o bin/manydoc ./cmd/manydoc
output/qa-venv/bin/python scripts/verify_published_pdf.py
output/qa-venv/bin/python scripts/verify_pdf_structure.py
```

`pdf_paragraph_test.go` checks inline/explicit block boundaries, structural
ActualText, mixed/homogeneous formatting, links, discretionary word continuity,
cell paragraphs and custom-label fallback. The cross-page regression checks each
region against its original run bounds. The published checker derives LI block
membership and 57 inline-only LBody element identities directly from the original
/K tree, compares those against SF and asserts selected joined soft-hyphen words.
Original source bytes, character counts, metadata, URI relationships and generated
DOCX/PDF bounds/content remain independently checked.

## Untagged geometric structure

```sh
GOWORK=off go test -run 'TestPDFUntagged' -count=1
GOWORK=off go test -run '^$' -fuzz '^FuzzPDFInferenceCoverage$' -fuzztime=15s -parallel=2 -timeout=40s
CGO_ENABLED=0 GOWORK=off go build -o bin/manydoc ./cmd/manydoc
output/qa-venv/bin/python scripts/verify_pdf_inference.py
NODE_PATH=output/sf-qa/node_modules node scripts/verify_structured.cjs output/pdf-inference/recovered.sf output/pdf-inference/paint-order.sf
python3 scripts/verify_structured_corpus.py
output/qa-venv/bin/python scripts/compare_pdf_text.py
```

The targeted source deliberately interleaves/reverses paint order beneath a
full-width title. Independent MuPDF coordinates and hand-placed intent establish
column/paragraph/heading structure. A second page supplies aligned numeric cells,
numbered/nested marker lists, continuation text and an image. Check exact SF
membership/starts/relationships/geometry and literal coverage including original
labels; then independently reopen generated DOCX/PDF for cells, links, image,
page boundaries, content and bounds. Both source pages and both native reflow
pages were inspected. `-pdf-skip-inference` separately checks original paint
order. It is a physical-inspection option, not a semantic inference claim.

Corpus coverage compares all original nonspace characters, source spans/styles/
links, image bytes/placements and page records before/after inference for all
28 accepted PDF inputs. All 28 Poppler comparisons pass. All 51 SF outputs pass
ProseMirror at 9,027 nodes after grouping replaces spurious baseline wrappers and
synthetic gaps; fixtures and source oracles are unchanged. Primitive positioning/
inline-image tests use `PDFSkipInference` to keep their paint-order oracles;
default-mode tests independently verify the changed reading-order behavior.

The new coverage fuzz asserts that every original painted token appears exactly
once with identical style/source geometry through rearrangement, including SF
persistence. A 15-second/two-worker campaign passed 3,313 executions. Parser
fuzz now seeds inferred lists/tables; its subsequent 10-second campaign passed
29,060 executions. These campaigns do not establish exhaustive safety or general
layout reconstruction. Ruling/merged/empty/multiline tables and complex layouts
remain required work.

Optional DOCX visual reproduction (QA executable only):

```sh
mkdir -p output/pdf-inference/lo
/Applications/LibreOffice.app/Contents/MacOS/soffice '-env:UserInstallation=file:///Users/artpar/workspace/code/github.com/artpar/manydoc/output/lo-pdf-inference-profile' --headless --convert-to pdf --outdir output/pdf-inference/lo output/pdf-inference/reflow.docx
pdftoppm -scale-to 1200 -png output/pdf-inference/lo/reflow.pdf output/pdf-inference/lo/page
```

Adapt the isolated profile path to the checkout. LibreOffice 7.6 produced two
pages; headings, table/list text were independently checked and both pages were
visually inspected. This qualifies this generated example, not all DOCX output.

The spanning-edge regression uses a full-width title/footer and a large gap
between upper/lower column paragraphs, asserting whole-column order. It exposed
incorrect core-font alias advances; `TestPDFCoreFontCanonicalAliasAdvances`
checks space/hyphen/bullet slots including WinAnsi aliases. The independent
inference script now compares intact source Helvetica/Helvetica-Bold string
advances against MuPDF, in addition to bounds and coordinates. All checks above
were repeated after the whitespace-cut/metric corrections.

## Painted-grid table recovery

```sh
GOWORK=off go test -run 'TestPDFRuled|FuzzPDFRuledGrid' -count=1
GOWORK=off go test -run '^$' -fuzz '^FuzzPDFRuledGrid$' -fuzztime=15s -fuzzminimizetime=1s -parallel=2 -timeout=40s
CGO_ENABLED=0 GOWORK=off go build -o bin/manydoc ./cmd/manydoc
output/qa-venv/bin/python scripts/verify_pdf_rulings.py
output/qa-venv/bin/python scripts/verify_pdf_rulings_office.py
NODE_PATH=output/sf-qa/node_modules node scripts/verify_structured.cjs output/pdf-rulings/ruled.sf output/pdf-rulings/office/recovered.sf
```

The first independent source has two pages: one stroked, one using thin fills,
with rectangular clipping and explicit opaque ExtGState. MuPDF's own table finder
supplies the seven exact cell rectangles, including colspan/rowspan/empty cells.
SF checks compare each rectangle, membership, text, marks, links and physical
pages. Generated DOCX checks actual merged XML identity and relationships; native
PDF checks both table grids and content/bounds. Both source and native PDF pages
were visually inspected. The combined SF outputs passed ProseMirror at 83 nodes.

The second script independently authors a targeted python-docx table and calls
actual LibreOffice 7.6 with UseTaggedPDF=false/PDFUACompliance=false. It asserts
the exported PDF really lacks StructTreeRoot, compares all seven recovered cell
bounds to MuPDF (three decimal points), then checks generated DOCX/PDF and a
LibreOffice rendering of the generated DOCX. Source/native/LibreOffice pages were
inspected. The isolated profile lives under output; the executable path can be
changed with `--soffice`. These generated documents qualify a producer on targeted
content; they are not a representative published real-world corpus. Neither
Python nor LibreOffice is used by library/CLI runtime or ordinary Go tests.

Go tests additionally cover current-path lifetime across q/Q, Form/clip transforms,
implicit rectangle closure, degenerate fills, half-stroke joins/overhangs and
mixed widths, opacity and unknown/reset mask state, partial/nonrectangular grids,
crossing text, empty/nested grids and preserved cell images. Text-grid spans,
empty cells and content survive SF -> DOCX and SF -> PDF -> SF; original runs/
styles/positions are compared directly. Native image-cell layout still rejects
with no partial output; SF/DOCX retain it. This remains required implementation.

Fuzz input authors known rectangular partitions before drawing per-cell borders,
then asserts exact recovered origins/spans/text, translated table bounds and SF
persistence. It varies merge/empty choices, positions, stroke widths and paint
mode. The latest 15-second/two-worker run passed 4,122 executions. A malformed
PDF campaign with ruled-grid seeds passed 67,941 executions in 10 seconds.
Full tests/race/vet/module checks and host CGO-disabled build pass; all 28 Poppler
comparisons and 51 ProseMirror corpus files (9,027 nodes) still pass. Finite
campaigns/budgets do not establish exhaustive safety or general layout fidelity.


## Rich PDF cells and footnote continuation

```sh
GOWORK=off go test -run 'TestPDFRich|TestImage' -count=1
GOWORK=off go test -run '^$' -fuzz '^FuzzPDFRichLayout$' -fuzztime=15s -parallel=2 -timeout=40s
GOWORK=off go run ./examples/rich-cells -out output/rich-cells
GOWORK=off go run ./examples/rich-cells -out output/rich-cells/continued -continued
output/qa-venv/bin/python scripts/verify_rich_cells.py output/rich-cells
output/qa-venv/bin/python scripts/verify_rich_cells.py output/rich-cells/continued --continued
NODE_PATH=output/sf-qa/node_modules node scripts/verify_structured.cjs output/rich-cells/rich-cells.sf output/rich-cells/continued/rich-cells.sf
```

The normal hand-authored SF fixture has combined outer spans, a nested rowspan,
three authored checkerboard PNGs, two nested-cell footnotes and a URI. Independent
pypdf/MuPDF/python-docx checks compare all six cell rectangles, image pixels and
placement, real DOCX merge identities/media/relationships/extents, content/marks,
URI and both actual note destinations. Native output has two pages; preferred
400×200 image size fits to 228×114pt in PDF and 232×116pt in DOCX. ProseMirror
accepts both example trees (98 combined nodes); SF round trips keep preferences.
Invisible logical column subdivisions cannot be inferred from painted boundaries
alone, so this fixture does not claim full logical-span recovery through PDF.

The continued example contains a 12-row table and image in one footnote. Its
four native pages have row counts 3/5/4/0, with each original row exactly once,
image only on page one and body flow resuming on page four. The internal reference
targets page one. All six native pages across both examples were visually inspected.
LibreOffice 7.6 rendered the normal DOCX to one visually inspected page, with all
content/images; this independently checks readability, not matching pagination.
Existing two/five-page text-note checks and seven-rectangle merged-table checks
pass unchanged. Earlier image-cell rejection statements above describe the
previous milestone; native rich-cell output now works.

Tests also check cancellation, invalid assets, oversized groups/fragments,
explicit cell/note breaks and extreme image preferences with atomic output. Rich
layout fuzz varies page dimensions, image sizes and nested row counts while
checking input immutability, exact content/asset occurrence and page bounds. A
15-second/two-worker run passed 1,498 executions. Full tests/race/vet/module
verification, CGO-disabled build, 28/28 Poppler comparisons and the existing
51-file/9,027-node ProseMirror corpus pass. These finite checks do not establish
exhaustive robustness or representative real-document fidelity.


## Oversized body-table groups

```sh
GOWORK=off go test -run 'TestPDFTableContinuation|TestTableContinuation|TestPDFMerged' -count=1
GOWORK=off go run ./examples/table-continuation
output/qa-venv/bin/python scripts/verify_table_continuation.py
NODE_PATH=output/sf-qa/node_modules node scripts/verify_structured.cjs output/table-continuation/table-continuation.sf
GOWORK=off go test -run '^$' -fuzz '^FuzzPDFTableContinuation$' -fuzztime=15s -parallel=2 -timeout=40s
```

The independent script checks a three-row/colspan-two cell beside a long adjoining
cell. Original labelled paragraphs, order, formatting, URI, image pixels/position,
DOCX merge identity and page-local note targets are asserted separately. Native
PDF has four pages: table continuations on pages one to three, early/late notes
on pages one/three and resumed body on page four. The image is at (26,110) with
size 30×15pt on page three. Columns remain at x=20/206.667/300; page-fragment
bottoms are 154/154/153pt. Only the right column has interior rules at y=46 on
page one and y=116 on page two. No such rule divides the spanning cell.
All four native pages were visually inspected. LibreOffice 7.6 reads/renders the
DOCX in three visually inspected pages with every original token and one image;
this checks independent readability, not matching pagination. The SF passes
ProseMirror at 88 nodes; SF/DOCX tests retain spans, note/URI ownership and pixels.

Continuation fuzz varies page width/height and checks immutable input, exact-once
content/assets, physical page bounds, styles/relationships and reference/body
ownership. An initial 15-second run passed 1,220 executions after correcting an
invalid strict page-order assertion; taller pages may put both notes together.
The valid input is retained under testdata/fuzz/FuzzPDFTableContinuation. Tests
also cover a preexisting body note before the table reference. A separate rich
layout campaign passed 1,037 executions in 15 seconds. Both used two workers.
Full tests (library 4.558s), race (library 47.500s), vet, module verification,
CGO-disabled host build/execution and diff checks pass. All 28 Poppler comparisons
and 51-file/9,027-node ProseMirror corpus still pass. Regenerated rich-cell,
normal/five-page note and seven-rectangle merged-table examples pass independent
checks. Oversized nested groups in cells/footnotes, image atoms, repeated headers
and explicit cell/note page breaks remain unfinished work, not accepted omissions.


## Recursive nested-table and footnote continuation

```sh
GOWORK=off go test -run 'TestPDFRecursive|TestRecursiveTable' -count=1
GOWORK=off go run ./examples/recursive-tables
output/qa-venv/bin/python scripts/verify_recursive_tables.py
NODE_PATH=output/sf-qa/node_modules node scripts/verify_structured.cjs output/recursive-tables/recursive-tables.sf
GOWORK=off go test -run '^$' -fuzz '^FuzzPDFRecursiveContinuation$' -fuzztime=20s -parallel=2 -timeout=50s
/Applications/LibreOffice.app/Contents/MacOS/soffice --headless --convert-to pdf --outdir output/recursive-tables/lo output/recursive-tables/recursive-tables.docx
output/qa-venv/bin/python scripts/verify_recursive_tables.py --office
```

Use an isolated `-env:UserInstallation=file:///absolute/output/lo-recursive-profile`
for reproducible LibreOffice automation when another instance/profile may exist.
The script verifies seven native pages, all original tokens and each cell's
order, analytical column positions, exact image pixels/placements, URI and note
destination pages, styles, DOCX merge identity and original media. For each
spanning frame it requires exactly one top/bottom pair per physical page, catching
fake interior dividers and repeated closed parts. The body image is at
(38,108)-(68,123) on page three; the note image at (44,172)-(68,184) on page six.
Early/late references and first note bodies share pages one/three. Pages four to
six contain note continuation only; body resumes on page seven. All seven were
visually inspected. The 175-node SF passes actual ProseMirror.

Unit tests also start a footnote directly with its oversized merged table,
compare SF/DOCX spans and note/URI ownership, check invalid oversized images
produce no output and verify all body content stays out of continuation-only
pages. A reference's already-admitted body fragment may contain later body text
on that same reference page; it need not wait for the entire footnote to finish.
The test invariant explicitly allows this correct case. No original fixtures or
expected content were changed. Width/height fuzz checks immutable sources,
exact-once content/assets, positions, marks/links, cell order and note starts.
A 20-second/two-worker run passed 1,100 executions.

LibreOffice 7.6 produced six inspected DOCX-rendered pages, preserving all tokens
and both images within margins. It visibly flattened the complex footnote tables.
`--office` reports this observation explicitly; its content/bounds pass is not a
note-table visual fidelity pass. XML table semantics remain checked independently,
and wider Word/producer interoperability remains required work.

Full Go tests (library 3.589s), race (50.447s), vet, module verification,
CGO-disabled host build/execution and diff checks pass. Regenerated previous
body/rich-cell/note/merged examples pass their exact independent oracles. The
28/28 Poppler comparisons and 51-file/9,027-node ProseMirror corpus pass. Older
sections' nested/footnote-group rejection statements describe prior milestones;
recursive continuation is now implemented. Explicit cell/note page breaks,
oversized images, repeated headers and the other ACCEPTANCE.md rows remain open.


## Explicit cell/note page boundaries

```sh
GOWORK=off go test -run 'TestPDF.*Break|TestPageBreak' -count=1
GOWORK=off go run ./examples/page-breaks
GOWORK=off go run ./examples/page-breaks -empty-note -out output/page-breaks/empty
output/qa-venv/bin/python scripts/verify_page_breaks.py
output/qa-venv/bin/python scripts/verify_page_breaks.py output/page-breaks/empty --empty-note
NODE_PATH=output/sf-qa/node_modules node scripts/verify_structured.cjs output/page-breaks/page-breaks.sf output/page-breaks/empty/page-breaks.sf
GOWORK=off go test -run '^$' -fuzz '^FuzzPDFCellPageBreaks$' -fuzztime=15s -parallel=2 -timeout=45s
GOWORK=off go test -run '^$' -fuzz '^FuzzPDFFootnotePageBreaks$' -fuzztime=15s -parallel=2 -timeout=45s
```

Independent pypdf/MuPDF/python-docx/XML checks assert five native mixed-story
pages, literal token ownership, real target pages, span frames/column positions,
marks/URIs and original media. The body image is on page two at
(32,46)-(62,61); the note image on page four at (44,172)-(68,184), with exact
source pixels. The cell split is between pages one/two. The note starts on page
two, continues on pages three/four, and body resumes on page five. The empty
note case has three pages, with a link from page one to blank page three; both
blank continuation pages persist. All eight native pages were visually inspected.
Both SF trees pass ProseMirror (64 nodes combined). SF/DOCX tests preserve four
mixed-story controls, note identity and merged-cell ownership.

Unit tests cover aligned/staggered sibling controls; leading/consecutive/trailing
breaks through four nesting depths; empty notes with only 20pt of content height;
visible notes alongside explicitly delayed notes; valid blank-page destinations;
no mutation and atomic error output. Fifteen-second/two-worker campaigns passed
1,825 cell-break and 1,831 note-break executions. These vary break positions,
counts, nesting and page heights, checking every original token's ownership/order,
blank page counts, synchronized sibling cuts and actual note destinations.
All earlier layout examples were regenerated and their exact independent
oracles still pass, alongside the 28 Poppler comparisons and 51-file/9,027-node
ProseMirror corpus. Full Go/race/vet/module/CGO-disabled build/diff checks pass;
latest measured timings are in verification.json.

The mixed DOCX's LibreOffice 7.6 rendering is an explicit failed visual check:
three pages, different cell/note boundaries and missing NOTE_SIDE_A/NOTE_SIDE_B.
The original OOXML includes both texts and all four controls. Reproduce the
observation with an isolated profile and record it separately from native checks:

```sh
/Applications/LibreOffice.app/Contents/MacOS/soffice -env:UserInstallation=file:///absolute/output/lo-pagebreak-profile --headless --convert-to pdf --outdir output/page-breaks/lo output/page-breaks/page-breaks.docx
output/qa-venv/bin/python scripts/verify_page_breaks.py --office
output/qa-venv/bin/python scripts/author_paragraph_break_variant.py
/Applications/LibreOffice.app/Contents/MacOS/soffice -env:UserInstallation=file:///absolute/output/lo-pagebreak-experiment-profile --headless --convert-to pdf --outdir output/page-breaks/lo-experiment output/page-breaks/paragraph-breaks.docx
```

`--office` writes libreoffice-observation.json and says explicitly that its
observation is not a pass. The variant changes four standalone break runs into
pageBreakBefore on following paragraphs. Its actual two-page rendering retains
all original tokens and both images, but cell-break pagination still differs.
This authors an experiment only, not a production codec or round-trip claim.
Next implement/qualify paragraph-property import/export while preserving the
existing forced-event SF semantics. Original fixtures remain unchanged.


## Paragraph properties and DOCX control preservation

```sh
GOWORK=off go test -run 'TestParagraph|TestDOCXParagraph|TestDOCXExplicit|TestDOCXBreakMetadata|TestDOCXHeading|TestDOCXStructural' -count=1
GOWORK=off go run ./examples/paragraph-breaks
output/qa-venv/bin/python scripts/verify_paragraph_breaks.py
NODE_PATH=output/sf-qa/node_modules node scripts/verify_structured.cjs output/paragraph-breaks/paragraph-breaks.sf output/paragraph-breaks/independent.sf
GOWORK=off go test -run '^$' -fuzz '^FuzzDOCXParagraphBreakRoundtrip$' -fuzztime=15s -parallel=2 -timeout=45s
GOWORK=off go test -run '^$' -fuzz '^FuzzPDFParagraphStarts$' -fuzztime=15s -parallel=2 -timeout=45s
GOWORK=off go test -run '^$' -fuzz '^FuzzReadDOCX$' -fuzztime=10s -parallel=2 -timeout=35s
```

The independent script checks eight native pages: FIRST on one; SECOND on two;
note bodies on three/four with a real destination from two to three; list first/
continuation on five/six; cell paragraphs on seven/eight. The first conditional
property adds no blank first page. Opposing cells stay in their source columns.
Marks/URI/Word properties/list-boundary metadata and bounds are checked.
A separate python-docx-authored source, with no manydoc metadata, exercises a
base style, derived style and explicit cancellation through DOCX -> SF -> PDF.
It yields three pages with SOURCE_BASE and SOURCE_CANCEL together on page two.
All eight example pages and three independent-source reflows were inspected.
Actual ProseMirror reads both SFs at 42 nodes combined.

The whole-SF fuzzer compares exact canonical trees, including marks, controls,
conditional properties, nested cells and note ownership, without mutating inputs.
It found the retained empty-cell seed 1968518a8e616e7b; required paragraphs now
use checked structural metadata instead of adding authored content. Heading
marks are also retained independently of default heading appearance. Malformed
metadata cannot hide runs. Tests cover defaults/styles/direct false overrides,
all six on/off lexical values, duplicate/invalid properties, cyclic inheritance,
inline split placement and paragraph-only attribute validation. The 15-second
DOCX run passed 12,060 executions; native conditional table fuzz passed 1,720 in
15 seconds. Malformed DOCX fuzz, including new property/metadata seeds, passed
336 executions in 10 seconds. All used two workers. These finite campaigns do
not establish exhaustive robustness.

Render actual DOCX output using an isolated profile, then add --office to the
independent verifier. The paragraph example renders to five inspected
LibreOffice 7.6 pages, retaining every token with no synthetic blank page. The
old mixed-break example now renders to two inspected pages with all original
tokens and both images, including NOTE_SIDE_A/NOTE_SIDE_B. The current
verify_page_breaks.py --office asserts that content mitigation. Historical failed
three-page output is recorded above; do not rewrite that evidence as a pass.
Cell/note pagination still differs, so broader Word/producer qualification stays
open. The QA-only author_paragraph_break_variant.py describes the earlier
experiment; current production output already uses paragraph properties.

Full tests/race/vet/module verification, host CGO-disabled build/execution and
diff checks pass (timings in verification.json). All earlier examples were
regenerated and their structural/geometry/pixel/destination oracles pass; only
DOCX's intentional property encoding assertion changed. All 28 Poppler text
comparisons and 51 corpus SF files/9,027 nodes pass. No production dependency
was added. Remaining acceptance rows, including oversized images, repeated
headers, richer model/fonts/shaping and a broader real corpus, remain required.


## Full-container image fitting

```sh
GOWORK=off go run ./examples/image-fit
output/qa-venv/bin/python scripts/verify_image_fit.py
NODE_PATH=output/sf-qa/node_modules node scripts/verify_structured.cjs output/image-fit/image-fit.sf
GOWORK=off go test -run 'TestImage|TestImages|TestFirstImage|TestOutputImage|TestPreviouslyOversized|TestJPEG|TestLargeFirstImage' -count=1
GOWORK=off go test -run '^$' -fuzz '^FuzzImageContainerFitting$' -fuzztime=20s -parallel=2 -timeout=50s
```

The source contains four placements of an authored 64×640 RGB checkerboard with
intrinsic, width-only, height-only and explicit preferred sizes, in body/nested
merged cells and two figure-first notes. Exact independent image rectangles are:
page one (20,20)-(35.2,172), page two (32,32)-(44,152), page four
(38,42)-(51,172), page six (50,54)-(59,144). All decoded RGB pixels match source;
no library resampling/cropping occurs. Nested table rectangles and rowspan
identity are checked. Both references and notices point from three to four and
five to six. Notice link rectangles contain their text. Page seven resumes body.
All seven native pages were visually inspected. SF retains original size values;
DOCX extents are 15.2×152, 12.4×124, 9.6×96 and 6.8×68pt, with original media bytes.
Actual ProseMirror accepts the new example (36 nodes).

Render the DOCX in LibreOffice with an isolated profile, then run the verifier
with --office. Its five inspected pages retain all four figures and original
text within margins. Pagination differs from native. This is actual producer
qualification of targeted content, not an expanded published-document corpus.
Regenerated prior examples still pass their exact independent geometry/pixel/
reference checks, along with 28 Poppler comparisons and 51 corpus SFs/9,027 nodes.

Fuzzing varies page dimensions, nesting, preferred sizes/aspects and body/note
contexts. It checks original pixels, format-precision-bounded requested aspect,
page bounds, no mutation, one placement and actual destinations. Final execution
count/timings are in verification.json. Unit cases include full-container sizing
after a preface; figure-first heading references; multiple/repeated notes with
separate real targets; formerly oversized cells/notes; SF preference persistence;
JPEG original stream/media bytes and decoded pixels; coordinate underflow and
a half-EMU tie. PDF extraction normalizes rasters to PNG, so JPEG checks compare
embedded DCT bytes and decoded pixels, not compressed extraction byte identity.

A well-framed RGBA PNG containing only one of 640 required rows reproduced
'index out of range [257]' in PDF output before the fix; DOCX also accepted the
bad asset. TestOutputImageMalformedIDATDoesNotPanic now gets zero-output errors
for both writers after full validation. A 50,000×50,000 header with corrected CRC
is rejected before expansion under the 25-million-pixel bound. There is no runtime
panic-recovery suppression or resampling workaround. Invalid/error cases remain
atomic. Older sections' oversized-image rejection statements describe earlier
milestones; valid tall figures now work, while no-space/precision/resource/codec
limits remain explicit. Full tests/race/vet/module/CGO-disabled build/diff checks
pass; current measured results are in verification.json.


## Repeating merged table headers

```sh
GOWORK=off go run ./examples/repeated-headers
output/qa-venv/bin/python scripts/verify_repeated_headers.py
NODE_PATH=output/sf-qa/node_modules node scripts/verify_structured.cjs output/repeated-headers/repeated-headers.sf
GOWORK=off go test -run 'TestRepeating|TestDOCX.*Header|TestInvalidRepeat' -count=1
GOWORK=off go test -run '^$' -fuzz '^FuzzRepeatingTableHeaders$' -fuzztime=20s -parallel=2 -timeout=50s
GOWORK=off go test -run '^$' -fuzz '^FuzzReadDOCX$' -fuzztime=10s -parallel=2 -timeout=40s
mkdir -p output/repeated-headers/lo
/Applications/LibreOffice.app/Contents/MacOS/soffice -env:UserInstallation=file:///Users/artpar/workspace/code/github.com/artpar/manydoc/output/lo-repeated-headers-profile --headless --convert-to pdf --outdir output/repeated-headers/lo output/repeated-headers/repeated-headers.docx
output/qa-venv/bin/python scripts/verify_repeated_headers.py --office output/repeated-headers/lo/repeated-headers.pdf --cli
```

The Go example writes SF/DOCX/PDF in-process. External readers/renderers above
are optional developer QA. The source is independently authored SF with one
header-prefix copy, two merged header rows, a 64×640 source raster requested at
12×12pt, a three-row/two-column oversized body merge, a body image, two note
references and a URI. No existing fixture or content/geometry oracle changed.

Independent native checks assert five pages; header merged rectangles
(20,20,206.6667,72), (206.6667,20,300,46), (206.6667,46,300,72); no false interior
border through the header rowspan; exact header pixels at (26,40,38,52) on each
page; unique ordered body tokens/assets; final body image at (26,92,56,107);
actual note targets 1→1 and 4→4; URI on page five; all text within margins. DOCX
checks exact original media, first-two-row tblHeader flags and both header/body
merge identities. Actual ProseMirror checks the new 100-node SF tree/attributes.
All five native pages and four LibreOffice pages were visually inspected.

LibreOffice repeats both header rows on all four pages and retains every body
token/image. Its pagination differs and it reduces the header raster to 5×50;
this is recorded separately from native exact-pixel acceptance. The independent
script's office branch checks that observed normalization and containment, not
pixel equivalence. Broader real exports and Word qualification remain required.

Whole-SF Go round trips cover body/nested/note row flags, merges, assets and
relationships. Tests include four nesting levels, all-page header note links to
one actual body, partial-page keep with the first fitted figure, explicit native
body boundaries, standard DOCX booleans, duplicate/bad flags, non-prefix imports
and boundary-crossing merge diagnostics. Indivisible over-height/control-bearing
PDF prefixes fail atomically while SF/DOCX preserve the requests. Fuzz varies
prefix length, body count, nested depth, page height and body/note placement,
checking every prefix row count, body uniqueness, bounds and no mutation plus
whole-SF DOCX round trips. Malformed DOCX seeds include a real generated header
package and an invalid on/off property. Finite campaigns are not exhaustive
safety evidence.

CGO-disabled CLI extraction of the example DOCX reproduces the entire authored
SF; converting that SF produces identical PDF text, geometry, raster hashes and links
through MuPDF (generation timestamps differ). PDF→SF independently
checks recovered physical content/geometry but diagnoses inferred rows/header
semantics and unsupported editable internal destinations; authorial repetition
is not invented from repeated paint. Existing table/recursive/image/break
examples, 28/28 Poppler comparisons and 51 corpus SFs/9,027 nodes still pass.
Exact command outcomes/timings are in verification.json. No production dependency
or runtime executable was added. The full goal remains incomplete.

The repeated-header LibreOffice exact-header-pixel check failed because its export
reduced the raster dimensions. `verify_repeated_headers.py --office ...` records
that failure separately in ignored `office-observation.json`; its containment pass
does not turn that pixel-fidelity failure into acceptance. Native PDF and DOCX
original-media checks remain exact.


`TestRepeatingHeaderFigureLeavesLegalBodyStart` reproduces two original failures
in output/repeated-header-figure-before.log (body fragment could not fit, and a
note had no room for its start). The final 24-case test varies one to three
header rows, zero to three nested body levels and body/note placement. It checks
original SF preferences, exact source pixels, real page bounds/body location and
both note/notice destinations. Header image fitting reserves a real compact line
and remeasures for deeper legal starts; figure-bearing prefixes can defer with a
notice. `TestHeaderOnlyTablePaintsEachAuthoredRowOnce` also checks a large figure
with two additional header rows. These extend rather than relax the native oracle.
