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
the old guide-styled paint comparison. That much older artifact differs from
current recovery in additional paint records; its strict historical comparison
still fails and is not claimed as a regression baseline for this milestone.

## Reproduction and scope

```sh
GOWORK=off go build -o bin/manydoc ./cmd/manydoc
bin/manydoc extract -format sf.gz -o output/note-inference/guide-kind.sf.gz output/note-inference/tagged-guide.pdf
bin/manydoc extract -pdf-skip-inference -format sf.gz -o output/note-inference/guide-kind-uninferred.sf.gz output/note-inference/tagged-guide.pdf
output/qa-venv/bin/python scripts/verify_note_inference.py --guide output/note-inference/tagged-guide.pdf --guide-sf output/note-inference/guide-kind.sf.gz
output/qa-venv/bin/python scripts/verify_pdf_note_kind.py
GOWORK=off go test -run 'TestPDFTaggedNotePlacementKind|TestPDFParentTree' -count=1 .
```

Raw authored rich-note sources check inferred, explicit and skipped placement;
missing/short/misaligned separators; all rich body blocks, both references,
physical glyph inventories, complete SF/gzip/JSON, DOCX kind persistence and
native PDF NoteType. ParentTree tests cover direct children and unchanged
contradictory/malformed ownership. Exact check scopes/results are in
verification.json. This resolves one original-source compatibility/reflow issue;
it does not establish general geometric note recognition or finish the original
acceptance audit. Earlier parser allocation boundaries and native browser reader
discrepancies remain documented separately.
