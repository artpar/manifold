# Tagged-table span recovery checkpoint

Updated 2026-11-24. The full rich-document goal remains incomplete. No external
blocker prevents continued development. This checkpoint concerns PDF input, not
Word-equivalent reflow or every omitted PDF construct.

## Implemented behavior

`pdf_tagged_spans.go` reconciles page-local explicit rows/cells with a uniquely
matching valid painted grid. It uses the existing bounded ruling collector and
per-page cached grids. Membership remains explicit; only absent span attributes
are inferred. Present unit attributes are authoritative too. All visible cell
content must fit the matched physical boundary on the same page. Multiple grid
matches or row alignments refuse inference. A row dynamic-programming budget of
100,000 states refuses the entire candidate when exhausted, with a diagnostic;
cancellation propagates. Existing 4,096 merged-line, 1,000-axis-interval and
100,000-slot grid budgets remain in force.

Empty vertical continuations can be coalesced only if they contain paragraph
containers with no runs, no other visible blocks and no explicit row-span
attribute. Their identity/role/tag/page and original zero-based row remain in
`SourceInfo.TableSpans.Continuations`. When arity forces all eligible empties to
be continuations, match continuation and new-origin classes in their respective
orders. This handles Word exports listing empty continuation tags before visible
origins in a different physical column. Otherwise require one ordered alignment.
Candidate copies never mutate the original table on refusal.

`SourceInfo.Structure/Evidence/Confidence` describe logical membership.
`TableSpans.ColSpan/RowSpan` are `explicit` or `inferred`; separate evidence
`painted_grid_adjacency`, heuristic confidence .7 and full painted bounds disclose
inference. The content quad/regions keep their existing meaning. SF and legacy
JSON retain this additive provenance; validation checks geometry, references,
finite metrics and supported enums. DOCX/PDF reflow retains its existing explicit
source-geometry-loss policy. The public runtime remains pure Go.

Ruling joins now include the crossing stroke's half-width plus the existing
.25pt coordinate tolerance. A 1e-5pt arithmetic guard handles separately rounded
path operands; tests reject larger unsupported gaps. This repairs actual .48/
.504pt border-piece joins without widening the axis-clustering tolerance.

A logical table with unequal trailing row widths and no unique painted solution
is retained as explicit, with `pdf_structure_tree_ragged_grid` evidence and
`pdf_structure_table_grid_uncertain`. No synthetic cells, dropped content or
unproven merges are introduced. Invalid explicit spans/overlaps still reject.
This preservation policy is not a completed merged-grid recovery claim.

## Real source and independent evidence

The original OASIS source files and independently committed oracles are unchanged.
The PDF contains 137 pages, 90 page-local tables, 756 rows and 3,020 logical cells.
Import retains every table/cell identity, role and original row membership.
Forty-one tables have recovered spans: 3,012 origins and eight retained empty
continuation identities account for all 3,020 cells. Table 4887 page 17 recovers
its three independently proven eight-column merges. Tables 5936/6359/7503 recover
three/two/three explicit empty continuation tags respectively.

`scripts/verify_tagged_spans.py` uses pypdf to check logical membership and that
coalesced tags contain no marked content/object references, and MuPDF to decode
painted paths independently and check all 5,536 sides of 1,384 recovered cell
bounds. It does not use manydoc's geometry as a source oracle. Default MuPDF table
snapping differs on several closely spaced axes; no claim of exact whole-corpus
MuPDF grid equality is made. Five original pages (17/25/53/69/89) and the detailed
page-89 table crop were visually inspected. The three-table SF sample passes the
actual ProseMirror schema with 964 nodes and all marks/attributes. Generated QA
reports, SF and images remain ignored; runtime uses none of the QA tools.

## Open work and next commands

Page 89 Table 12539 has shifted column boundaries (including .96pt shifts), so
its global primitive grid currently fails rectangular-component validation. Its
source cells and bounds remain explicit with uncertainty. The visible description
cell for byte 4 spans bytes 5–7; those three continuation tags remain explicit
cells pending a row-dependent partition solution. Do not change the original
oracle or casually snap these distinct axes to make this pass.

The previous DOCX oracle's eleven raw continuation cells and the currently
coalesced eight PDF tags are different counts. This checkpoint is not evidence
that all source merges, styles, links, text runs or images are fully recovered.
Other original extraction diagnostics remain. Full-source pretty SF has large
per-glyph provenance overhead (~451 MB); the example deliberately emits a small,
labelled three-table SF sample and a whole-table audit rather than repeating that
large artifact. Codec/provenance performance remains required work.

After this grid, supply/qualify a reusable font covering original U+2A6D4, then
continue real numbering templates, shading, widths, source-font/scripts and the
other acceptance requirements. Preserve missing-glyph errors; do not delete the
character or silently substitute fonts.

```sh
GOWORK=off go run ./examples/tagged-spans
output/qa-venv/bin/python scripts/verify_tagged_spans.py
NODE_PATH=output/sf-qa/node_modules node scripts/verify_structured.cjs output/tagged-spans/sample.sf
GOWORK=off go test -run 'TestPDFTagged|TestRealPDFMQTT|TestRulingJoin' -v .
GOWORK=off go test ./...
GOWORK=off go test -race ./...
GOWORK=off go vet ./...
CGO_ENABLED=0 GOWORK=off go build -o bin/manydoc ./cmd/manydoc
git diff --check
```

Actual final check results/log locations are in docs/verification.json; historical
results from earlier checkpoints are not substitutes for this feature's checks.
