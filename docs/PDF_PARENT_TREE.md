# Reciprocal PDF ownership

The forward logical tree supplies reading order. The inverse ParentTree supplies
reciprocal content ownership; it is not a cached document representation.
`pdf_parent_tree.go` checks a present index before recovering notes/content.

The [PDF 2.0 specification corrections, clause 14.7.5.4](https://pdf-issues.pdfa.org/32000-2-2020/clause14.html#14754-finding-structure-elements-from-content-items)
distinguish owner arrays indexed by page/Form MCIDs from scalar owners for whole
objects. MCIDs are local to their page/stream; reciprocal keys identify distinct
containers. Null array slots may represent unused or unowned sequences.

## Behavior

- Valid reciprocal entries preserve the existing forward structure/content/order.
- A missing ParentTree emits `pdf_parent_tree_missing`; missing container keys,
  indirect owner identity or reciprocal slots emit `pdf_parent_tree_incomplete`.
  Recoverable forward content remains, without reciprocal certification.
- Inverse entries absent from the forward graph emit `pdf_parent_tree_unclaimed`.
  They never supply extra content or invented order.
- Conflicting owners, owner references outside the forward structure, invalid
  owner types, shared keys between different containers, duplicate forward MCIDs,
  wrong array/scalar entry shape or simultaneous StructParent/StructParents reject.
- Number trees validate increasing nonnegative signed-32-bit keys, disjoint child
  ranges, optional Limits when present, indirect Kids, paired Nums and node shape.
  Cyclic/repeated nodes reject. Missing nonroot Limits are tolerated; full PDF
  number-tree conformance is not claimed.

Bounds: 64 levels; 200,000 combined index nodes/key entries; 400,000 combined
index entries/owner slots. Context cancellation is checked while traversing nodes
and forward owners. This is not exhaustive resource safety; source parsing and
other PDF operators have separate limits. A source without an index can still
recover forward structure, with an explicit diagnostic. Physical inspection via
PDFSkipStructure deliberately bypasses logical and reciprocal interpretation.

## Evidence and reproduction

Go tests check page/Form MCID zero isolation, annotation URI/scalar ownership,
flat/branched indexes, null/missing/extra entries, unknown/direct/conflicting owners,
ordering/range/shape errors, cycles/repeated branches, depth and sparse-slot bounds.
All existing original source bytes and independent structural expectations remain
unchanged. Generated notes/lists retain their current typed graph tests.

Independent pypdf checks find 413 forward elements/550 reciprocal content owners
in the original Copyright Office InDesign export and 40/24 in generated notes.
The verifier changes an existing slot to another existing forward owner, leaves
all decoded page content streams unchanged and saves a distinct malformed variant.
CLI logical import rejects that ownership contradiction without committing output;
unchanged sources pass. The combined tagged-structure verifier also passes table,
image, Form, URI and DOCX/PDF reflow checks. The original-publication verifier
passes 10 lists/57 items/exact K membership/16 URI targets/20,531 normalized
characters, DOCX metadata and nine reflowed PDF pages/space counts/bounds. Its
stale link classifier now separates typed internal destinations from external
URIs and additionally checks the source GoTo/name identity; the original 16-URI
oracle and source bytes remain unchanged.
This is graph evidence, with no new output-layout or visual fidelity claim.

```sh
GOWORK=off go test -run 'TestPDFParentTree|TestPDFTagged|TestGeneratedPDF' -count=1 .
GOWORK=off go test -run '^$' -fuzz '^FuzzPDFParentTree$' -fuzztime=10s -parallel=2 .
output/qa-venv/bin/python scripts/verify_pdf_parent_tree.py testdata/real/copyright-office-circular-1-2021.pdf
output/qa-venv/bin/python scripts/verify_pdf_parent_tree.py output/colleaf-tagged/document.pdf --out output/pdf-parent-tree/generated-mutated.pdf
CGO_ENABLED=0 GOWORK=off go build -o output/manydoc-parent-tree ./cmd/manydoc
# Expected failure, with ParentTree/forward-owner contradiction:
output/manydoc-parent-tree extract -format sf -o output/pdf-parent-tree/rejected.sf output/pdf-parent-tree/mutated.pdf
```

No PDF/UA/WTPDF/full PDF conformance, browser/Word fidelity or general malformed-
input safety follows from this scoped validation. Broader source/resource/untagged
and final goal audits remain required.
