# Rich inferred PDF note bodies

PDF_NOTE_PAGES.md records the current cross-page extension and resolves the long-
note failure retained below as milestone history.

The strict two-paragraph oracle in PDF_UNTAGGED_NOTES.md now passes unchanged.
`pdf_infer_note_extent.go` plans continuation ownership after reciprocal mark/view
relationships qualify a note and before any references or flow blocks mutate.
It retains existing paragraphs, headings, lists, tables and pictures as rich blocks;
it does not flatten a body into concatenated text or recover an embedded SF cache.

## Evidence and uncertainty

A short horizontal stroke/filled strip immediately above a footnote's leading
body establishes an inferred note zone. A leading endnote page can establish a
zone when no earlier same-page ordinary or unlocated visible content precedes it.
Consecutive same-page blocks qualify by physical order, nearby baseline/text size,
left edge/indent and bounded gap. Centered pictures and tables retain their own
positions without requiring a text-like left edge. Different font families and
bold/italic/color/URI text remain represented. A coherent next qualified note root
inherits a shared zone, but stays a boundary between independently owned bodies.

The separator must begin within one body em of the label's left edge, extend at
least four ems rightward, lie no more than four ems above the body, and be at most
max(3pt,.3em) thick. A leading endnote root starts in the upper35% of its page.
Continuations cannot move upward more than1pt, exceed four ems of vertical gap,
start more than1.5em left of the label or extend beyond their page. Text/list left
edges cannot exceed three ems of indentation. A text-bearing block's largest size
must be between .85 and1.8 times the leading body's size. These are documented
heuristics, not calibrated authorial certainty or a universal PDF layout grammar.

Page changes, other note/citation owners, unknown/multi-page geometry, explicit
cross-reference/artifact associations, unsupported containers, large gaps,
unrelated columns and incompatible sizes stop grouping. Unqualified content stays
visible in ordinary flow, with body-extent uncertainty diagnosed. Existing rich
explicit tagged-note recovery remains separate. `PDFSkipInference` disables this
geometric relationship/extent rule; `PDFSkipStructure` alone selects geometric
reading of the original source.

Original glyph text, font/paint facts, matrices, quads, baselines, paint order,
assets and physical annotation/view records remain. Note.Source records inferred
ownership with evidence `pdf_note_reciprocal_views;separated_note_zone_continuation`
and heuristic confidence .7; reference/kind inference and original body-label
source/style remain separately represented. Child blocks retain their original
role evidence, including a table's independent structure evidence. Later body
blocks are moved once; every body boundary and URI remains independent.

The existing4,000,000-step inference budget now also covers candidate traversal,
source-region checks, rulings and nested body content. Cached per-block geometry
avoids repeated subtree scans. Stable root ordering follows source flow, while
output note order retains the existing navigation contract. Index construction
and stable sorting are separate bounded input-size work. All ownership plans
complete before mutation; exhaustion/cancellation/conflicting claims do not leave
partially assigned notes. The two new exhaustion stages enter extent planning.
There is no new dependency, public schema/API, app authorization or runtime process.

## Qualification

`TestPDFUntaggedRichNoteExtent` authors independent raw PDF objects for both foot-
and endnotes, with a bold blue URI paragraph, a2x2 A/B/C/D table, a centered raster
with original red/blue pixels, and a final paragraph. Both infer five rich body
blocks. Exact SF/gzip/JSON and normalized DOCX round trips retain the semantics.

The complete painted glyph/source inventory equals the physical control exactly
apart from the separately asserted inference attribution fields; empty editable
anchors and synthetic word separators are not painted glyphs. Physical annotation
records also equal the control. The image's two pixels are checked against its
hand-authored source, independently of output encoding. Tests cover separator,
column, gap and small-footer barriers, plus two independently linked notes sharing
one separator and five repeated reads requiring identical whole-model JSON.
Existing source/view/malformed/graph tests remain unchanged. Budget cases now
exercise both relationship resolution and extent planning.

`scripts/verify_pdf_note_extent.py` independently checks source pypdf objects,
three annotations/URI, exact original image bytes and MuPDF glyph origins for all
46 painted characters in each profile. It checks the centered image's actual
placement against model quads. Actual source spans prove bold, blue and10pt text.
Generated DOCX ZIP/XML owns the table, hyperlink and raster relationships; the
public CLI rereads it. Native tagged PDF is reread through actual structure into
five typed body blocks with table, URI, bold text and pixels; pypdf/MuPDF check
its visible content/navigation. LibreOffice independently renders both DOCX files,
with both PDF readers checking actual note text. No cached SF substitution or
text-only correctness claim is made.

The original-authored POI derivative retains the source-owned two-paragraph note
and italic continuation. Its original fixture remains unchanged. The same strict
oracle that failed at eb4573e now passes; MuPDF/pypdf independently confirm source
text and actual italic spans. It remains a synthetic extension of a real fixture,
not an unchanged original-export claim. The preceding original foot/endnote
exports remain qualified separately in PDF_UNTAGGED_NOTES.md.

Actual ProseMirror validates174 nodes/marks/attributes across source, Word-reread
and native-PDF-reread SFs for both profiles. Original publication/MQTT regression
uses full SF equality against the earlier direct-encryption milestone.

The source/native/Word note-page comparison was visually inspected: text, all four
table cells, bold blue continuation, red/blue image and tail remain visible with
safe separation and no observed clipping. Native Endnotes adds its normal heading;
fonts, spacing, page size, image alignment and table-row heights differ. Source
positions remain evidence; no source/Word-equivalent print fidelity is claimed.
Generated files/renders/environments stay ignored under output.

```sh
GOWORK=off MANYDOC_PDF_NOTE_EXTENT_QA=output/pdf-note-extent go test -run TestPDFUntagged -count=1 .
GOWORK=off CGO_ENABLED=0 go build -o bin/manydoc ./cmd/manydoc
# Prepare the unchanged two-paragraph derivative as in PDF_UNTAGGED_NOTES.md.
output/qa-venv/bin/python scripts/verify_pdf_note_extent.py
mkdir -p output/pdf-note-extent/word-render/{footnote,endnote}
/Applications/LibreOffice.app/Contents/MacOS/soffice -env:UserInstallation=file:///tmp/manydoc-pdf-note-extent-word --headless --convert-to pdf --outdir output/pdf-note-extent/word-render/footnote output/pdf-note-extent/footnote/rich.docx
/Applications/LibreOffice.app/Contents/MacOS/soffice -env:UserInstallation=file:///tmp/manydoc-pdf-note-extent-word --headless --convert-to pdf --outdir output/pdf-note-extent/word-render/endnote output/pdf-note-extent/endnote/rich.docx
output/qa-venv/bin/python scripts/verify_pdf_note_extent.py --word-render output/pdf-note-extent/word-render
NODE_PATH=output/sf-qa/node_modules node scripts/verify_structured.cjs output/pdf-note-extent/{footnote,endnote}/{rich,rich-docx,rich-native}.sf
GOWORK=off go test ./...
# The full suite's existing font/history corpus exceeded the default10m race limit.
GOWORK=off go test -race -timeout 20m ./...
GOWORK=off go vet ./...
git diff --check
```

Exact check scopes/results are in verification.json. This closes the concrete
single-page continuation failure, not the overall goal. Cross-page geometric
ownership, denser/longer footnote placement, tagged fallback-kind policy and the
remaining source/resource/native/final acceptance audit remain open. Required
capabilities are not declared complete merely because ambiguous text is retained.

## Next concrete long-note/cross-page failure

The generator's separate `--cross-page` profile authors a long italic second
paragraph under the same footnote ID; it writes a distinct output directory and
leaves the original two-paragraph short oracle and pinned real fixture unchanged.
LibreOffice independently exports three pages. pypdf/MuPDF retain the complete
paragraph. Page1 has matching reciprocal citation/body links and a separator near
y209pt, with the body label near y212pt on an842pt page. Subsequent pages retain
separator-backed continuation text without another label/link pair.

At1ef16ef, geometric recovery returned zero notes: the base same-page footnote rule
requires a body below60% page height, even though this longer footnote's leading
body starts higher. Extent planning also treats physical page changes as hard
boundaries. All visible text stays in flow, but the authored one-note/two-paragraph
relationship is not recovered. The unchanged strict oracle fails. This remains
required implementation work, not an unsupported-feature exception or external
blocker. The next change must qualify stronger separator-backed long-note placement
and cross-page ownership/paragraph continuity while retaining source expectations.

```sh
python3 scripts/generate_pdf_note_continuation_probe.py --cross-page
/Applications/LibreOffice.app/Contents/MacOS/soffice -env:UserInstallation=file:///tmp/manydoc-note-crosspage-qa --headless --convert-to pdf --outdir output/pdf-note-continuation-crosspage output/pdf-note-continuation-crosspage/continuation-source.docx
bin/manydoc extract -pdf-skip-structure -format json -o output/pdf-note-continuation-crosspage/geometric.json output/pdf-note-continuation-crosspage/continuation-source.pdf
# Failed at1ef16ef; the current PDF_NOTE_PAGES.md implementation passes unchanged.
python3 scripts/generate_pdf_note_continuation_probe.py --cross-page --check output/pdf-note-continuation-crosspage/geometric.json
```
