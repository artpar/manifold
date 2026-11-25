# Geometric PDF notes from reciprocal navigation

This records the initial reciprocal-note milestone. PDF_NOTE_CONTINUATION.md
contains the current rich-body extension, verification and next long-note failure.

`pdf_infer_notes.go` extends geometric reconstruction without requiring Note/Ref
structure tags. A compact raised text mark must have a uniquely owning physical
GoTo annotation. Its explicit XYZ view must identify another uniquely owned mark
with the same literal label, and the second link must point back. A leading raised
body label in one recovered top-level paragraph qualifies the body. Same-page
bodies below60% of page height and at least40pt below the citation infer footnotes;
a later-page body infers an endnote. These are editorial heuristics, never explicit
authorial policy. References carry Inferred=true; kind_inferred stays true. Source
structure/evidence/confidence identify the rule and heuristic .8 confidence.

Additional matching citations into a reciprocal-qualified body reuse one note.
Conflicting targets, overlapping ambiguous ownership, partial XYZ views, mismatched
labels, absent backlinks and unraised labels remain literal. Marks do not establish
notes on text equality alone. Explicit tagged note recovery still runs first.
`PDFSkipInference` disables this geometric relationship recovery; explicit tag
semantics still follow the separate existing tag policy. `PDFSkipStructure` alone
selects geometric reading of the unchanged original source.

Only ordering boxes for link-owned labels align to adjacent larger baselines.
Original glyph quads/baselines/matrices/fonts/paint order never move. The label's
source is retained in label_source and its style/navigation in body_label_style;
the visible citation becomes a note_reference atom. Physical annotation rectangles,
destinations, nullable view parameters and pages remain intact, independently of
inferred editable flow anchors. Citation link semantics become note semantics with
an explicit relationship diagnostic. Original physical navigation remains available.

Moving a final note-only page removes its now-empty trailing editable flow boundary,
with a diagnostic; physical Pages remain recorded. This prevents an extra blank
page before generated endnotes. A two-page independent source and public PDF
write/read regression check the behavior.

At eb4573e, body extent used one recovered paragraph. PDF_NOTE_CONTINUATION.md
now qualifies rich same-page continuation and shared note zones. The prior limit
and its original failure are retained below as milestone history. Adjacent continuation ownership is
unproven, stays visible in ordinary flow, and is explicitly diagnosed. Multi-block,
nested, cross-page or bidirectional geometric note bodies and more ambiguous
physical policies remain acceptance work, not completed rich-note capability.
The existing explicit note graph supports rich bodies separately. Reflow may
normalize independent label presentation; that is diagnosed. There is no general
claim to reconstruct authorial note semantics from arbitrary PDF page appearance.

A shared4,000,000-comparison bound covers alignment, mark ownership and target
searches, including annotation regions. Exhaustion leaves unassigned content in
flow and diagnoses uncertainty. Assignment is planned before graph mutation;
conflicting multi-annotation targets cannot overwrite one reference. Bodies are
committed in stable navigation order, with repeated citation ownership deduplicated.
These bounds do not claim to bound all earlier parser allocations.

## Default and original-source qualification

`TestPDFUntagged*` uses independently authored PDF objects, not writer round trips.
It checks graph identity, repeated references, footnote kind/label/body, inline
citation order, source geometry, annotations, exact SF round trips, SF/gzip/JSON/
DOCX semantic round trips and three inference-budget exhaustion stages. Six
negative shapes preserve literal content: mismatched labels, absent backlinks,
partial views, distant targets, unraised labels and ambiguous duplicate ownership.

`scripts/verify_pdf_untagged_notes.py` uses the actual Apache POI Word exports
already qualified in REAL_DOCX_CORPUS.md. Original DOCX ZIP/XML independently owns
the expected body text/reference count. The original LibreOffice PDF bytes are
unchanged. Their custom Footnote/Endnote→P tags already use the separate tagged
note rule by default; `-pdf-skip-structure` exercises geometric recovery on these
originals. A separately named pypdf derivative removes only StructTreeRoot for a
true untagged profile; it is not described as an unchanged original PDF. The
original tagged footnote lacks a published FENote NoteType attribute: its default
logical read still uses the existing diagnosed endnote reflow default with
kind_inferred=true. The geometric profile infers footnote kind from position.
These are distinct, disclosed editorial policies, not identical kind evidence.
Unifying stronger positional kind evidence with tagged fallback policies remains
part of the source/note-policy audit.

The exact f74d1b5 Git-archive CLI leaves both geometric original-source cases
literal with no notes. The current CLI recovers one source-owned note in each;
this is a before/after structure improvement, not just matching plain text.

Original PDFs used in this run:

| File | SHA-256 | Pages / source note |
| --- | --- | --- |
| footnotes.pdf | d70b3438a87896c889b07325eac4bee338076a95a979ee2b49365da11e080747 |1 / snoska |
| endnotes.pdf |337daa2ba7445ee822f42cca6ca79fdbe255e339ab68fddec148e22ac394063e |2 / XXX |

Across all four profiles, the reconstructed visible glyph inventory (including
citation atoms and body labels) equals the physical control exactly, with fonts,
quads, matrices, baselines and original order; physical navigation equals the
control after excluding editable flow-binding fields. pypdf and MuPDF independently
check text and two actual reciprocal annotations; MuPDF additionally checks the
citation/body label positions against original glyph origins. Generated DOCX XML
and a public-API reread retain editable relationships and inference flags.
LibreOffice independently opens/renders all four DOCX outputs; pypdf/MuPDF require
the actual note text and one/two-page counts in those Word-rendered PDFs. The
last-page comparison sheet was visually inspected with no observed missing notes
or clipping. Actual ProseMirror validates118 nodes and their marks/attributes. Native
PDFs pass both independent readers' note-text checks and retain navigation.
Reflow has one footnote page/two endnote pages; citations target the actual note
page, with no extra blank page.
Explicit QA font mappings use four original DejaVu Sans faces; no silent fallback.

Visual inspection of original and both recovered/reflowed first/last pages found readable
citation/body labels and note text with safe separation and no observed clipping.
The source endnote has no generated Endnotes heading; native output includes its
normal heading. Font/placement differences remain visible. This is a layout check,
not source/Word-equivalent print fidelity. Generated sources/renders/reports remain
ignored under output, with reproducible commands below. Original publication/MQTT
full SF remains unchanged against the prior direct-encryption milestone.

```sh
GOWORK=off CGO_ENABLED=0 go build -o bin/manydoc ./cmd/manydoc
GOWORK=off go test -run TestPDFUntagged -count=1 .
# Prepare original-render PDFs and QA fonts as in REAL_DOCX_CORPUS.md.
output/qa-venv/bin/python scripts/verify_pdf_untagged_notes.py
mkdir -p output/pdf-untagged-notes/word-render
/Applications/LibreOffice.app/Contents/MacOS/soffice -env:UserInstallation=file:///tmp/manydoc-untagged-note-word-qa --headless --convert-to pdf --outdir output/pdf-untagged-notes/word-render output/pdf-untagged-notes/{footnotes,endnotes}-{original-geometric,untagged}.docx
output/qa-venv/bin/python scripts/verify_pdf_untagged_notes.py --word-render output/pdf-untagged-notes/word-render
NODE_PATH=output/sf-qa/node_modules node scripts/verify_structured.cjs output/pdf-untagged-notes/{footnotes,endnotes}-{original-geometric,untagged}.sf
GOWORK=off go test ./...
GOWORK=off go test -race ./...
GOWORK=off go vet ./...
git diff --check
```

Verification scopes/timings are recorded in verification.json. The default full
race run hit its ten-minute deadline in an existing custom-font seed; final
focused race passes and the unchanged full suite with -timeout20m passes at
eb4573e: library677.584s/CLI4.442s. The original timeout remains recorded. This
qualification predates the rich-extent extension in PDF_NOTE_CONTINUATION.md. The overall goal
remains active/incomplete. Next: broaden actual multi-paragraph/page and conflicting
source note cases while retaining the existing physical/relationship oracles, then
continue the source/resource/reflow/native and final acceptance audit.

## Next concrete continuation failure

`scripts/generate_pdf_note_continuation_probe.py` authors a synthetic extension of
poi-footnotes.docx without changing that pinned fixture. Its original footnote
owns a second italic paragraph, with24pt paragraph spacing to make the physical
separation recoverable. The expected body comes directly from authored OOXML;
this is a targeted derivative, not an unchanged real-export fixture.

The independent LibreOffice PDF visibly retains both paragraphs. At eb4573e, geometric
recovery links only ` snoska` into the footnote; `Continuation paragraph belongs to
this footnote, with italic text.` remains visible and italic in ordinary flow.
The body-extent diagnostic is emitted, but the required two-paragraph note graph
is not complete. The strict source oracle failed at that milestone and must not be weakened.
PDF_NOTE_CONTINUATION.md records the new rich-extent implementation passing the
same authored oracle, with structural/source/asset and independent-reader proof. This is
an implementation task, not an external blocker or a completion waiver.

```sh
python3 scripts/generate_pdf_note_continuation_probe.py
/Applications/LibreOffice.app/Contents/MacOS/soffice -env:UserInstallation=file:///tmp/manydoc-note-continuation-qa --headless --convert-to pdf --outdir output/pdf-note-continuation output/pdf-note-continuation/continuation-source.docx
bin/manydoc extract -pdf-skip-structure -format json -o output/pdf-note-continuation/geometric.json output/pdf-note-continuation/continuation-source.pdf
# Failed at eb4573e; the current rich-extent implementation passes this exact oracle.
python3 scripts/generate_pdf_note_continuation_probe.py --check output/pdf-note-continuation/geometric.json
```

Continue from that precise original-owned graph expectation, retaining its text,
style, physical geometry/navigation and independently authored source relationship.
