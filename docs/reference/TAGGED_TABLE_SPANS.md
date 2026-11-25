# Tagged-table span recovery

Updated 2026-11-24. The full rich-document goal remains incomplete. No external
blocker prevents continued development. This reader checkpoint does not establish
Word-equivalent reflow or complete interpretation of every source PDF construct.

## Current behavior

`internal/engine/pdf_structure.go` reconciles page-local explicit rows/cells with a uniquely
matching valid painted grid. Membership remains explicit; only absent span
attributes are inferred. Present unit attributes remain authoritative. Visible
content must fit the matched physical boundary on the same page. Multiple grid
matches or row alignments refuse inference. A row alignment budget of 100,000
states refuses the entire candidate with a diagnostic when exhausted; context
cancellation propagates. Candidates use copies and per-page cached grids.

Empty vertical continuations coalesce only when they have paragraph containers
with no runs, no other visible blocks and no explicit row-span attribute. Their
identity/role/tag/page/original zero-based row survive in
`SourceInfo.TableSpans.Continuations`. If arity forces all eligible empties to be
continuations, match continuation and new-origin classes in their respective
orders; otherwise require one ordered alignment. No visible cell is discarded.

`SourceInfo.Structure/Evidence/Confidence` describe explicit membership.
`TableSpans.ColSpan/RowSpan` are `explicit` or `inferred`; separate evidence
`painted_grid_adjacency`, heuristic confidence .7 and painted bounds disclose span
inference. Content quads/regions retain their original meaning. SF and legacy
JSON retain and validate this provenance. DOCX/PDF reflow retains its existing
explicit geometry-loss policy. No schema or public API change was needed for the
short-connector fix. Runtime remains pure Go with no new dependency.

Ruling joins include crossing-stroke half-width plus the existing .25pt
coordinate tolerance, with a 1e-5pt arithmetic guard for separately rounded path
operands. Distinct axes keep the .25pt clustering rule; they are not snapped
more broadly to force a table match. A logical table with unequal trailing widths
and no unique painted solution retains its original cells/positions and emits
`pdf_structure_table_grid_uncertain`. Invalid explicit spans/overlaps still reject.
This preservation policy remains exercised by a genuinely missing-connector test.

## Short painted connector pieces

`internal/engine/pdf_graphics.go` captures filled rectangles up to 3pt in each displayed dimension
as supporting patches. They respect known rectangular clips, page bounds,
transparency and uncertain graphics-effect exclusion. Patches never introduce
row/column axes or connect independent ruling components. `rulingPatchLines`
projects their actual filled extents onto already established axes, merges them
with longitudinal rulings, and runs the same rectangular grid validation used
for tagged and untagged tables. Missing paint is not filled from row tags.

Lines and patches share the 100,000 per-page capture budget; overflow clears both
so no partial grid is presented as complete evidence. Projection caps at 100,000
segments and 4,096 merged segments per candidate, with explicit limit diagnostics
and cancellation checks. Existing limits of 1,000 axis intervals and 100,000 grid
slots remain. Broader occlusion, double-border and rotated-table work is unfinished.

The handwritten fixture has axes only one point apart and a one-point short
filled connector. Both tagged and untagged readers recover the four distinct
primitive columns, seven origins and merged spans. Removing the connector keeps
the tagged cells uncertain. Clipped, transparent and uncertain-blend connectors
cannot establish coverage. Isolated filled corner squares cannot invent axes.
Tests also preserve explicit continuation references and exact SF semantics.

## Original source and independent evidence

The original OASIS DOCX/PDF files and committed oracles are unchanged. Native
PDF import retains 137 pages, 90 page-local tables, 756 rows and all 3,020 logical
cell identities/roles/original rows. All 42 omitted-span grids recover using
painted evidence: 3,009 editable origins plus eleven continuation references.
Table 4887 page 17 retains its three independently proven eight-column merges.
Page-89 table 12539 retains 17 distinct columns and all 79 origins; every origin's
row/column, row/column spans and normalized text hash matches Word table 51 in the
unchanged independently authored source oracle. Its description origin 12635
spans four rows and keeps continuation IDs 12654/12674/12694 on rows 7/8/9.

The previous trace correctly found a nonrectangular L-shaped component (rows
6–7, primitive columns 14–16), but incorrectly assumed the evidence gap was an
unpainted source gap. Independent MuPDF paths prove three continuous filled
junctions on y=301.249995pt: x=433.14999..433.98999,
459.310005..460.030005, and 511.540005..512.50002. Short connector rectangles,
including pieces only .24/.36/.48pt wide, had been excluded by the longitudinal
line-length threshold. Capturing those pieces repairs the root cause. A proposed
row-constrained border-completion rule was investigated but is not implemented.

`scripts/qa/layout/verify_tagged_spans.py` independently decodes original pypdf membership
and confirms each coalesced tag has no MCID/object-reference content. MuPDF
checks all 5,852 sides of 1,463 recovered cell bounds against painted paths with
.3pt tolerance; this is a geometric tolerance, not an ink-outline equality claim.
A separate .001pt interval-union check proves the three junctions have paint
continuity. All 79 page-89 origin coordinates/spans/text hashes also match the
Word oracle. The three-table SF sample passes actual ProseMirror (958 nodes and
all marks/attributes). The original page-89 table crop was visually inspected;
prior five-page observations remain historical evidence, not reflow equivalence.

## Reproduce and continue

```sh
GOWORK=off go run ./examples/tagged-spans -out output/tagged-rows
output/qa-venv/bin/python scripts/qa/layout/verify_tagged_spans.py --out output/tagged-rows
NODE_PATH=output/sf-qa/node_modules node scripts/qa/structured/verify_structured.cjs output/tagged-rows/sample.sf
GOWORK=off go test -run 'TestPDF.*Patch|TestRealPDFMQTT' -v ./internal/engine
```

Full tests/race/vet, malformed-PDF and valid-grid fuzz, module verification and
host/Linux CGO-disabled builds are recorded under `short_painted_ruling_patches`
in verification.json. Generated reports/SF/images remain ignored; none of the QA
tools are required at runtime. Complete-source pretty SF is about 451 MB; the
example deliberately emits a small three-table sample and full table audit.
Provenance/codec performance remains required work.

The supplementary source character now has a qualified explicitly supplied static
font and efficient glyph embedding; see FONT_SUBSETTING.md for immutable source
hashes, licensing provenance and independent glyph/raster evidence. No external
font is vendored or linked into the runtime. The full Word source now renders
207 native pages, but a new independent audit rejects three corrupted Unicode
URI fragments. Fix that exact failure, then source widths/numbering and broader
layout/content/link/assets and PDF→SF provenance/codec performance under the
original full goal. This table checkpoint is not complete-document acceptance.
