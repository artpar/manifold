# Tagged note placement and original-source recovery

A PDF Note/Ref graph establishes definition/citation ownership. It does not
establish whether the author intended footnote or endnote reflow. Explicit
FENote NoteType Footnote/Endnote remains authoritative. An absent/None kind now
supports inferred Footnote when all of the following hold:

- The first child is a recoverable nonempty Lbl, outside opaque ActualText.
- Its single-page bounds start in the lower 40% of the physical page.
- A thin source ruling sits within four label ems above it and spans at least
  four ems to its right, using the existing note separator checks.
- At least one recovered citation lies on that same page, above the label by
  more than two label ems.

Note.KindInferred remains true. The original logical graph and physical source
facts remain explicit; no source NoteType is invented. Unqualified placement
retains the diagnosed editorial Endnote default. PDFSkipInference disables this
new geometric policy. Comparison/cancellation bounds reuse the existing note
inference counter. Cached label recovery prevents duplicate content consumption.

## Original-source regression found by the acceptance audit

Rechecking the unchanged 71-page PDF Association guide exposed a regression in
reciprocal-index validation: its ParentTree contains direct child dictionaries.
An exact f171ab4 CLI rejects it with `ParentTree Kids must be indirect`, before
recovering either note. Direct dictionaries now emit
`pdf_parent_tree_direct_child`; recovery still checks the full index, forward
owners, scalar/array shapes, child ranges, Limits, cycles and existing work bounds.
This accepts a disclosed number-tree conformance deviation, not contradictory
ownership. Direct *owner* dictionaries and non-dictionary children still reject.
The source bytes were not rewritten to qualify recovery.

The source is the same pinned original documented in PDF_NOTES.md, SHA256
8185a30793869379ea4c2d82f214a930368b3fa7520faba61df38c52a502335e.
Its NOTE_1/NOTE_2 labels lie on pages 8/22 below actual thin painted separators.
Independent pypdf source decoding verifies original IDs, label/body MCIDs,
styles, citation sites and GoTo/backlink parameters. MuPDF verifies physical
separator/label/page geometry. Both now recover as inferred Footnote.

The new verifier compares complete current SF with current PDFSkipInference SF:
only kind values and the placement diagnostic text may differ. It does not weaken
the old guide-styled paint comparison. The older artifact differs by exactly 56 tagged_paragraph_word_gap records with
synthetic_separator precision. Literal list-marker recovery now separates label
and body ownership. The strict historical comparison still fails; the additional
inventory oracle proves every omitted record is synthesized spacing, every
original painted record remains and no new painted record appears. This is an
explained historical semantic difference, not a waived painted-content loss.

## Reproduction and scope

```sh
GOWORK=off go build -o bin/manifold ./cmd/manifold
MANYDOC_PDF_NOTE_KIND_QA=output/pdf-note-kind GOWORK=off go test -run TestPDFTaggedNotePlacementKind -count=1 ./internal/engine
bin/manifold extract -format sf.gz -o output/note-inference/guide-kind.sf.gz output/note-inference/tagged-guide.pdf
bin/manifold extract -pdf-skip-inference -format sf.gz -o output/note-inference/guide-kind-uninferred.sf.gz output/note-inference/tagged-guide.pdf
output/qa-venv/bin/python scripts/qa/pdf/verify_pdf_note_kind.py --render-dir output/pdf-note-kind --historical-sf output/note-inference/guide-styled.sf.gz
output/qa-venv/bin/python scripts/qa/layout/verify_note_inference.py --guide output/note-inference/tagged-guide.pdf --guide-sf output/note-inference/guide-kind.sf.gz
output/qa-venv/bin/python scripts/qa/pdf/verify_pdf_note_kind.py
GOWORK=off go test -run 'TestPDFTaggedNotePlacementKind|TestPDFParentTree' -count=1 ./internal/engine
```

Raw authored rich-note sources check inferred, explicit and skipped placement;
missing/short/misaligned separators; all rich body blocks, both references,
physical glyph inventories, complete SF/gzip/JSON, DOCX kind persistence and
native PDF NoteType. Native and actual LibreOffice-rendered DOCX each keep the
rich footnote on one page, with two working citations and exact2x1 raster pixels;
both pages were visually inspected. ParentTree tests cover direct children and unchanged
contradictory/malformed ownership. Exact check scopes/results are in
verification.json. This resolves one original-source compatibility/reflow issue;
it does not establish general geometric note recognition or finish the original
acceptance audit. Earlier parser allocation boundaries and native browser reader
discrepancies remain documented separately.

Current-code regression checks also regenerate all four paths for eight original
Word/Mac Word/LibreOffice exports, then pass verify_poi_corpus.py against unchanged
source oracles and existing independent producer renders (33 pages). The sectioned
rich example and independent section verifier pass. Actual ProseMirror validates
all 51 accepted synthetic corpus files/9,034nodes and marks. These scopes are
separate from the newly inspected two one-page placement outputs.
