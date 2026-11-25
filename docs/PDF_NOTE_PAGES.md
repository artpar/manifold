# Long and cross-page inferred PDF notes

The three-page source and strict one-footnote/two-paragraph oracle from
PDF_NOTE_CONTINUATION.md now pass unchanged. All original glyph/source facts and
three physical page records remain; one inferred body owns the continuation and
its second paragraph retains three source regions. No cached SF body is read.

## Two separate decisions

The base note graph still requires matching raised labels and reciprocal physical
XYZ views. A same-page body above the prior60% height cutoff can now qualify when
a nearby painted separator independently supports footnote placement. The40pt
citation/body separation remains. This is an inferred reflow kind, not proof of an
authorial FENote NoteType. Existing explicit kind facts remain authoritative.

A qualified body can continue into the next physical note region. Its preceding
block must reach the lower20% of the page and span at least eight body ems. The
next footnote region needs a coherent font/column and nearby separator. Matching
separator lengths tolerate max(2em,20%); a separator expanded to column width can
qualify with matching boundary text style. A changed separator may also qualify
with paragraph-wrap evidence. Leading endnote pages without separators require
wrap evidence in the upper35%. Ordinary content above a footnote region stays in
flow. Known competing note/citation owners and new small leading labels stop
cross-page assignment; blank-page gaps are not skipped.

Paragraph continuity is separate from ownership. Only inferred paragraphs with
compatible run/paragraph direction, language, font/size/style/URI can join. Their
left edges must agree and the previous final line must reach within three ems of
its right bound. A sentence-ending or uppercase new-start boundary stays separate.
Explicit paragraph roles and PageBreakBefore stay authoritative. These rules are
heuristics with confidence .65, not a claim to uniquely recover arbitrary authorial
paragraphs. Rich blocks can move into a note without being flattened or joined.

Plans retain original block indices and independent paragraph-join decisions.
The existing shared inference budget covers scans, region/font checks and wrap
checks; exhaustion/cancellation cannot leave a partial graph. Joining copies run
slices and preserves original glyph sources. Synthetic inter-page spaces remain
identified as such. Note.Source adds cross_page_note_zone; joined paragraph source
uses pdf_note_page_paragraph_continuation, with original per-page source regions.

A synthetic flow boundary is removed only when that physical page's entire flow
moved into notes. Mixed ordinary/note pages and actual blank pages keep their
boundaries. Physical Pages and annotation/view records remain independently intact.

## Generated continuation captions

The native writer's visible Footnotes continued caption now carries explicit
/Artifact membership, like its repeated label. It retains its appearance and
physical page/content membership, separately from note-owned MCIDs. Imported
artifact text remains available in flow with source membership according to
PDF_ARTIFACTS.md; it is not silently removed or invented as an authoring template.

The independent verifier follows actual note-owned structure/MCIDs and separately
recognizes artifact scopes. MuPDF painted-character baselines are correlated with
those memberships. A raw whole-page text stream can interleave ordinary content,
notes and pagination decoration; dropping strings by content would not prove the
note body. The caption regression checks explicit membership on every continuation
page and exclusion from the typed note body.

## Evidence

Independent raw-object profiles cover mixed ordinary/note pages, new paragraph
boundaries, widened continuation separators, leading endnote continuation and
cross-page rich raster/table/URI content. All preserve complete physical glyph/
source inventory, physical links and exact SF normalization; supported SF/gzip/
JSON/DOCX cycles retain note identity/graph/kind/paragraph semantics. Competing
owners, missing separators, original blank pages and an unsupported high body
without a separator retain unassigned content. Cancellation/budget tests assert
that unfinished page plans do not mutate flow.

The original synthetic POI extension is unchanged:120 authored segments in one
italic paragraph, independently owned by the original footnote's OOXML ID. Its
Word/LibreOffice PDF spans three pages. The prior1ef16ef geometric result had zero
notes; current recovery has one footnote, two body paragraphs, complete text/style
and all three paragraph regions. The pinned actual Word fixture stays unchanged.
This is a targeted synthetic extension, not an unchanged real-export claim.

scripts/verify_pdf_note_pages.py checks every original painted character origin in
five raw profiles with MuPDF, source PDF dictionaries with strict pypdf, actual
DOCX note XML and public CLI rereads. Native PDF rereads recover the typed note
body through actual structure. Both readers independently check note-owned text;
LibreOffice opens/renders all six DOCX outputs. Actual ProseMirror checks all
source/Word/native SF nodes/marks/attributes. Runtime remains pure Go; these
independent QA tools are never runtime dependencies.

The ten-page original/native/Word long-note comparison was visually inspected.
All120 segments remain readable, italic and separated from rules/captions, with
no observed clipping. Explicit Go Mono mapping gives four native pages versus
three source/Word pages, including a short final native continuation. Font,
spacing and pagination differ; Word/source print fidelity is not claimed.

```sh
GOWORK=off MANYDOC_PDF_NOTE_PAGES_QA=output/pdf-note-pages go test -run 'TestPDFUntagged|TestPDFNotePagePlanning|TestPDFFootnoteContinuationCaption' -count=1 .
GOWORK=off CGO_ENABLED=0 go build -o bin/manydoc ./cmd/manydoc
# Generate/render the unchanged --cross-page source as in PDF_NOTE_CONTINUATION.md.
output/qa-venv/bin/python scripts/verify_pdf_note_pages.py
# For each mixed/new-paragraph/wide-new-paragraph/endnote/rich-page/authored-long:
# independently render its model.docx to word-render/<name>/model.pdf with LibreOffice.
output/qa-venv/bin/python scripts/verify_pdf_note_pages.py --word-render output/pdf-note-pages/word-render
NODE_PATH=output/sf-qa/node_modules node scripts/verify_structured.cjs output/pdf-note-pages/{mixed,new-paragraph,wide-new-paragraph,endnote,rich-page,authored-long}/{model,docx,native}.sf
GOWORK=off go test ./...
GOWORK=off go test -race -timeout 20m ./...
GOWORK=off go vet ./...
git diff --check
```

The exact checks/scopes are in verification.json. This closes the named long-note
failure, not the overall goal. Return to the full original acceptance audit,
including typed fallback-kind policy, initial parser boundaries, representative
source/resource/reflow/native/API evidence and remaining required capabilities.
Ambiguous geometry still stays visible with uncertainty; it is not a completion
waiver for a concrete recoverable source failure.
