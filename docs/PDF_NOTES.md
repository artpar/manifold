# Explicit PDF notes and source relationships

The shared model now recovers explicit Note/FENote definitions and referring
structure elements joined by forward or reverse Ref entries. Resolution runs
after the whole logical tree is parsed, so forward references and paint order
cannot change note identity. Matching label text alone never establishes a link.
Repeated citations become separate note_reference atoms linked to one rich note.
Paragraphs, actual text styles, lists, tables, raster pixels/alternative text and
source navigation into the moved note body remain present.

## Source facts and editorial choices

Note.Source / SF note source retains explicit definition role, object identity,
physical bounds/regions and original binary ID bytes. Optional Note.LabelSource /
label_source retains the removed leading Lbl's own geometry, font/metrics and paint,
so converting that visible mark into a semantic label does not erase its position. SourceInfo.StructureID is
base64 JSON for those bytes, not a PDF text-string/Unicode reinterpretation.
SourceInfo.References contains owner/target object identities; aggregate source
bounds retain contributing explicit edges too. Unresolved targets remain recorded.
Reference atoms keep their own explicit source positions, styles and relationships.

FENote attributes with owner FENote and NoteType Footnote/Endnote select the
corresponding kind explicitly. None/absent and older Note tags select an editorial
endnote reflow default with Note.KindInferred / kind_inferred true and an explicit
diagnostic. This does not claim that physical placement proves the original
foot/endnote policy. Original positions remain source facts. KindInferred persists
through JSON/SF/gzip and checked DOCX metadata. Source provenance itself retains
the existing declared DOCX/native reflow-loss policy.

The first nonempty literal citation mark becomes Note.Label, independently of
public identity and automatic numbering. An explicit single-text leading Lbl is
removed from the rich body and represented by that semantic label. A presentation
diagnostic discloses that reflow normalizes independent label typography;
label_source retains the physical font/paint evidence. A differing body mark is
retained as BodyLabel; a complex label remains visible with a diagnostic.
Definition ActualText follows the existing structural replacement policy. Enclosing
ActualText, nested/ambiguous relationships or complex nontext citations remain in
flow with diagnostics and explicit source Ref edges instead of invented note atoms.
Malformed Ref shapes/targets or known invalid NoteType values are rejected; total
Ref targets are bounded at 200,000 per document.

## Published semantics

The [PDF Association tagged-PDF guide](https://pdfa.org/wp-content/uploads/2023/07/Tagged-PDF-Best-Practice-Guide.pdf)
describes Note, Reference and Lbl, including inline notes and non-prescriptive
physical placement. [WTPDF 1.0](https://pdfa.org/wp-content/uploads/2024/02/Well-Tagged-PDF-WTPDF-1.0.pdf)
defines FENote Ref relationships and the optional FENote-owner NoteType attribute.
These are independently authored recovery rules against published formats, not
copied UniDoc implementation code or a formal clean-room claim. The small source
probes exercise individual primitives; they are not PDF/UA conformance fixtures or
a substitute for the remaining original producer corpus.

## Reproducible qualification

```sh
python3 scripts/generate_pdf_semantic_audit.py
GOWORK=off CGO_ENABLED=0 go build -o bin/manydoc ./cmd/manydoc
bin/manydoc extract -format sf -o output/audit/explicit-note.sf output/audit/explicit-note.pdf
bin/manydoc extract -format sf -o output/audit/rich-explicit-note.sf output/audit/rich-explicit-note.pdf
bin/manydoc convert -allow-lossy -pdf-font-map 'Courier=Go Mono' -o output/audit/rich-note-native.pdf output/audit/rich-explicit-note.sf
bin/manydoc convert -allow-lossy -o output/audit/rich-note.docx output/audit/rich-explicit-note.sf
bin/manydoc extract -format sf -o output/audit/rich-note-docx.sf output/audit/rich-note.docx
/Applications/LibreOffice.app/Contents/MacOS/soffice -env:UserInstallation=file:///tmp/manydoc-pdf-notes-lo --headless --convert-to pdf --outdir output/audit/lo output/audit/rich-note.docx
output/qa-venv/bin/python scripts/verify_pdf_notes.py
NODE_PATH=output/sf-qa/node_modules node scripts/verify_structured.cjs output/audit/explicit-note.sf output/audit/rich-explicit-note.sf
GOWORK=off go test -race -run 'TestPDFExplicitNotes|TestPDFNoteAmbiguity|TestPDFNoteActualText|TestPDFNoteInline|TestDOCXListTable' -count=1 .
```

The explicit font map is a caller-authorized Go Mono substitution for this probe,
not a claim to have recovered/embedded the source Courier font program. QA tools
are optional development tools; production remains pure Go with unchanged modules.

pypdf independently walks the original Ref/ID graph and checks every recovered
reference site, kind/uncertainty, source position, rich node hierarchy, strong mark,
red/blue raster pixels, alternative text and inferred note-body view binding.
All editable note values also match the DOCX→SF round trip after removing only
acknowledged source provenance and empty default spellings.
python-docx reads the generated package; pypdf/PyMuPDF read native and LibreOffice
PDF outputs: each has one page, two references plus one body mark, each body string
once, linked note targets and exact embedded pixels. Rendered pages were inspected.
Native and Word reflow differ; no source visual-layout equivalence is asserted.

Visual QA found a real independent-reader list/table boundary failure: LibreOffice
synthesized a second numbered paragraph after the list bookmark range. The writer
now emits a checked empty paragraph cancelling numbering before the following
table. Ordinary paragraph layout is deliberate: forcing a tiny exact line height
hid/clipped the table in LibreOffice. The boundary is layout-only, checked against
its exact empty shape and list-end/table siblings before restoring the original SF
blocks. Visible/malformed marker content is rejected. Tests compare canonical SF
semantics so explicit/default start=1 and colspan=1 spellings do not become false
semantic failures.

## Still required

This milestone closes the audit's zero-typed-note failure for explicit unambiguous
relationships, including rich/repeated/reverse-linked cases. Broader note recovery
still needs rich multi-node/internal mixed-direction citations, nested/enclosing-ActualText graphs, and carefully identified adjacency/
geometric inference where no Ref exists. Differing, literal and explicitly empty
citation/body marks now have their own representation, described below. Enclosing
ActualText/nested cases retain content and source evidence but do not yet expose a full editable note graph. Do not relabel
these required follow-ups as a completed note contract. Original rich producer
inputs, native Colleaf interoperability and final full-goal acceptance remain open.

## Independent display marks (current extension)

`NoteReference.Label` / `note_reference.attrs.label` is optional: nil/null/omitted
inherits the note's canonical label; a string overrides only that citation, and
an empty string explicitly displays no marker. `Note.BodyLabel` / note `body_label`
has the same inheritance/empty distinction for the body mark. These fields do not
change note identity or automatic counter order. Existing files without these
attributes keep their previous meaning. Multiline, blank-only and invalid XML/
Unicode label values are rejected; an explicitly empty override is valid.

Explicit PDF references can now have different labels, including differing leading
body Lbl text. Each site keeps its actual mark, source position, style and Ref
identity; the body Lbl becomes BodyLabel when needed, with its LabelSource intact.
An explicitly empty reference container can produce a markerless atom. A missing
paint/MCR association cannot establish an empty mark: those cases retain flow,
source relationships and uncertainty diagnostics. A source with entirely empty
citations keeps explicit empty overrides, including an empty body default if no
leading body label exists. No visible decimal is invented for that source.

Native PDF uses citation overrides independently of body overrides. Footnote body
labels now use the normal font/glyph/shaping pipeline and measured advance; a body
label with a missing glyph or excess width fails explicitly before output. Endnote
marks use the normal list-marker path with a distinct explicit-empty state. Empty
citation marks retain note admission/identity while emitting no zero-width link
rectangle. Homogeneous RTL citation labels shape in their own direction; decimal
marks remain LTR inside RTL prose. Mixed-direction atomic citation labels currently
return an explicit shaping error rather than paint incorrect glyph order. Rich
multi-node/mixed-script internal label layout remains a follow-up, not a fidelity
claim.

### DOCX representation and editing policy

For variant notes, DOCX uses a standard foot/endnote reference with a zero-width
custom mark, an explicit note-body bookmark, literal hyperlink citation runs, and
a literal body mark. This keeps an active real Word note and visible labels with
correct body navigation. Ignorable checked metadata reconstructs the identity,
nil/empty/override distinction and canonical automatic intent. Standard reference
styles make visible citations superscript. Generated body marks are 8pt with a
checked separator; their private role never permits discarding arbitrary runs,
drawings, fields, differently named targets or namespace lookalikes.

If any variant note shares a document with automatic labels, portable Word labels
are serialized literally. The library requires AllowLossy for that DOCX export:
automatic intent round-trips in checked metadata, but Word's own subsequent editing
will not recompute those literal labels. Ordinary notes without variants keep the
previous standard automatic-reference/NOTEREF representation. PDF source reflow
already retains its existing AllowLossy requirement. Daptin access/admission and
Colleaf save/quotas remain outside the library.

### Additional reproduction and evidence

```sh
GOWORK=off go run ./examples/notes -variant-labels -out output/note-variants
python3 scripts/generate_pdf_semantic_audit.py
bin/manydoc extract -format sf -o output/note-variants/variant-pdf.sf output/audit/variant-explicit-note.pdf
bin/manydoc extract -format sf -o output/note-variants/markerless-pdf.sf output/audit/markerless-explicit-note.pdf
bin/manydoc convert -allow-lossy -pdf-font-map 'Courier=Go Mono' -o output/note-variants/variant-native.pdf output/note-variants/variant-pdf.sf
bin/manydoc convert -allow-lossy -o output/note-variants/variant.docx output/note-variants/variant-pdf.sf
bin/manydoc extract -format sf -o output/note-variants/variant-docx.sf output/note-variants/variant.docx
bin/manydoc convert -allow-lossy -o output/note-variants/markerless.docx output/note-variants/markerless-pdf.sf
bin/manydoc extract -format sf -o output/note-variants/markerless-docx.sf output/note-variants/markerless.docx
/Applications/LibreOffice.app/Contents/MacOS/soffice -env:UserInstallation=file:///tmp/manydoc-note-variants-lo --headless --convert-to pdf --outdir output/note-variants/lo output/note-variants/notes.docx
output/qa-venv/bin/python scripts/verify_note_variants.py
GOWORK=off go test -race -run 'TestNoteVariant|TestPDFDifferent|TestMarkerless|TestPDFMarkerless|TestIndependentBody|TestVariantCitation|TestMissingCitation' -count=1 .
```

The independent verifier checks original paint and empty-container facts, actual
reference/body values, rich hierarchy/pixels, all editable DOCX round-trip values,
real package markup and visible citation-to-body target coordinates. All four
native/LibreOffice example pages were inspected. Note label variants no longer
flatten merely because their text differs. The old differing-label fallback test
now requires the new typed behavior while retaining the original source bytes;
malformed target and uncertainty expectations remain intact. Existing primary
source files/oracles have not been changed.

## Relationship inference and original-publisher qualification

`NoteReference.Inferred` / `note_reference.attrs.inferred` now distinguishes a
recovered relationship from an explicit source Ref. It is independent of
Note.KindInferred, which concerns editorial foot/endnote placement. Both flags
persist through JSON/SF/gzip and checked DOCX reference/citation metadata, including
repeated references. The original Reference/Link/Note roles remain explicit source
facts; no guessed Ref edge is inserted into SourceInfo.References.

Two bounded rules now recover additional relationships:

- An immediate sibling Reference then Note, with identical nonempty literal
  citation/leading Lbl text, no enclosing ActualText/nested-note scope and no
  conflicting explicit relationships. Evidence is pdf_note_adjacent_reference,
  heuristic confidence .85. This follows the inline pattern described in the
  [published tagging guide](https://pdfa.org/wp-content/uploads/2023/07/Tagged-PDF-Best-Practice-Guide.pdf).
- A logically owned GoTo Link with a resolved two-coordinate physical view into
  exactly one Note leading-label region (two physical points of tolerance) and
  identical nonempty literal text. Evidence is pdf_note_link_view, heuristic
  confidence .95. Original physical annotation/view values remain explicit;
  associating that view with a semantic Note is still identified as inference.

Confidence values are heuristic rankings, not probabilities. Explicit Ref links
win. Empty/different/complex marks, intervening content, missing paint, partial or
far views, overlapping note-label regions, unresolved explicit edges and enclosing
ActualText retain content/navigation with diagnostics instead of guessing. Link
matching is bounded at 200,000 comparisons and observes cancellation. No proximity
alone, page-bottom placement or repeated equal text establishes a note relationship.
Label blocks are cached during candidate inspection and materialized once, including
rejected candidates; inference must not consume or duplicate visible glyphs.

### Real original guide, not only a matching synthetic probe

The 71-page PDF Association Tagged PDF Best Practice Guide 1.0.1 itself contains
two Note definitions, NOTE_1/NOTE_2, on physical pages 8 and 22. Its exported logical
Note containers sit at the end of Document and its citation sites are Links, with
no explicit Ref entries or Reference roles. Adjacency alone therefore recovers
neither. The physical-link rule now recovers both as typed notes with separate
relationship/kind inference flags. Independent pypdf decoding of original font
encodings/ToUnicode and MCID paint matches both label/body strings; original ID,
GoTo parameters and source label bounds match too. The new verifier reports only
text hashes, not republished prose.

Original file: 1,233,877 bytes, SHA256
8185a30793869379ea4c2d82f214a930368b3fa7520faba61df38c52a502335e.
The publisher download returned HTTP 403 to the QA client; the public
[distribution mirror](https://www.newformat.se/documents/pdfa-org/pdfa-org-technical-papers/pdfa-org-technical-paper-eng-pdfua-tagged-pdf-best-practices-guide-syntax-v1-0-1-2023-01-14.pdf)
provided the pinned original export. It remains ignored QA input, not a vendored
fixture or a redistribution-license claim. No customer file was uploaded.

All 87,381 source paint-order text records, including semantic citation/body marks,
match the pre-inference extraction exactly: normalized SHA256
c09a4c63f5cba37c53150c3f894e0bf8910690e43f8ae04a335cbcbb2d0e068c.
Actual ProseMirror checks 89,397 nodes/marks/attributes across that original, the
adjacent probe and its DOCX round trip. Original source pages 8/22 and generated
native/LibreOffice note pages were inspected; no source-layout equivalence is
claimed. The kind default remains editorial Endnote despite observed footnote
placement, with KindInferred=true and original page/region provenance.

The original leading labels now retain editable typography and backlinks through
BodyLabelStyle / body_label_marks as well as independent physical PDFNavigation.
See the latest section below; this replaces the former pdf_note_label_link loss.
Physical geometry and inferred semantic anchor ownership remain separate facts.

```sh
python3 scripts/generate_pdf_semantic_audit.py
bin/manydoc extract -format sf -o output/note-inference/adjacent.sf output/audit/adjacent-note.pdf
bin/manydoc convert -allow-lossy -o output/note-inference/adjacent.docx output/note-inference/adjacent.sf
bin/manydoc extract -format sf -o output/note-inference/adjacent-docx.sf output/note-inference/adjacent.docx
bin/manydoc extract -format sf.gz -o output/note-inference/guide-styled.sf.gz output/note-inference/tagged-guide.pdf
output/qa-venv/bin/python scripts/verify_note_inference.py --guide output/note-inference/tagged-guide.pdf --guide-sf output/note-inference/guide-styled.sf.gz --baseline-guide-sf output/note-inference/guide-linked.sf.gz
GOWORK=off go test -race -run 'TestPDFAdjacent|TestPDFPhysicalLinkNoteInference|TestDOCXNoteRelationshipInference|TestPDFNoteLinkAmbiguity' -count=1 .
```

No required runtime component, module dependency, app integration or quota/access
boundary changed. General untagged note recognition, richer/nested/ActualText label
graphs, heterogeneous label layout, Colleaf snapshot integration and remaining
original DOCX corpus/visual qualification are still open.


## Editable body-label style and navigation

Note.BodyLabelStyle is an optional Run template without Text, Note, Anchor or
Source. Its font/size/color/language/direction/emphasis and Link/Destination apply
to the body mark resolved from BodyLabel/default numbering. LabelSource retains
physical provenance independently. SF stores optional body_label_marks using
normal ProseMirror textStyle/emphasis/link marks; an empty array retains an
explicit empty template. No second copy of visible label text is authoritative.
Invalid mark/source/atom/destination combinations reject through normal validators.

Recovery uses the leading Lbl's independently recovered common style/navigation;
heterogeneous leading body labels now use BodyLabelContent and preserve each
style (RICH_NOTE_LABELS.md). Citation intersections remain separate next work. Native
PDF uses normal measured text and font/shaping admission. A body mark taller than
the compact footnote line gets an atomic measured line before its rich body;
continuations do not reintroduce that tall marker. Styled endnote gutter and
height are measured before body admission. This is reflow, not source layout.

Word uses ordinary visible text/hyperlinks and rPr, with checked noteBodyStyle
and noteBodySeparator roles. The latter contains exactly a tab and two layout
spaces, so consumers that discard the custom tab still show a label/body gap.
The gutter budgets one em per display column plus 6pt; this is conservative
consumer layout, not Word font measurement. Exact label/structure/namespace/style
checks prevent metadata from swallowing other text, drawings or hidden runs.
Markerless labels still retain their template. Variant/automatic editing loss
continues to require AllowLossy; existing private metadata retains counter intent.

Independent original guide checks now compare label Tf/BaseFont/black paint and
backlink annotation GoTo parameters with editable marks and semantic target
bindings, in addition to existing original body/ID/forward-view oracles. Source
bytes/oracles are unchanged. A current nonempty text/reference/leading-label
projection preserves 87,442 records exactly against guide-linked.sf.gz; the
historical 87,381-record report used a different projection. Actual ProseMirror
checks 89,431 nodes/marks/attributes across the original guide, styled example
and Word round trip. Native and LibreOffice output pages were inspected.

LibreOffice renders 9.5pt bold italic colored labels but substitutes unavailable
Go with Liberation Serif and omits the custom label hyperlinks from its PDF.
Standard DOCX targets/relationships and native PDF named backlinks are separately
verified. This does not establish that every Word consumer preserves navigation.
The original guide's 6.959646pt sizes now export with acknowledged half-point
projection and checked original intent; see DOCX_PRECISION.md for full original
Word round-trip/reader evidence. Strict output still rejects inexact sizes.
Heterogeneous/mixed-direction body labels are qualified in RICH_NOTE_LABELS.md;
rich/internal mixed-direction citations and nested/ActualText graphs remain open.

```sh
GOWORK=off go run ./examples/notes -variant-labels -styled-labels -out output/note-styles
bin/manydoc extract -format sf -o output/note-styles/notes-docx.sf output/note-styles/notes.docx
/Applications/LibreOffice.app/Contents/MacOS/soffice -env:UserInstallation=file:///tmp/manydoc-note-styles-lo --headless --convert-to pdf --outdir output/note-styles/lo output/note-styles/notes.docx
output/qa-venv/bin/python scripts/verify_note_styles.py
NODE_PATH=output/sf-qa/node_modules node scripts/verify_structured.cjs output/note-styles/notes.sf output/note-styles/notes-docx.sf output/note-inference/guide-styled.sf.gz
GOWORK=off go test -race -run 'TestNoteBodyLabelStyle|TestStyledNoteBody|TestMarkerlessStyled|TestTallNote' -count=1 .
```


Rich body labels now have a literal text-node alternative, BodyLabelContent /
body_label_content, with individually retained styles/links/source evidence.
It excludes the simple body string/template and does not change note identity or
citation counters. Standard visible Word runs use checked bookmark boundaries;
source paint/Word/native style/geometry/asset and mixed-script gutter checks pass.
RICH_NOTE_LABELS.md contains complete behavior, producer limits and commands.
