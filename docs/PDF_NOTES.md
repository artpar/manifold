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

One consistent literal text citation mark becomes Note.Label, independently of
public identity and automatic numbering. Only a matching explicit leading Lbl is
removed from the rich body and represented by that semantic label. A presentation
diagnostic discloses that reflow normalizes independent label typography;
label_source retains the physical font/paint evidence. A different or
complex label remains visible with a diagnostic; it is never silently deleted.
Definition ActualText follows the existing structural replacement policy. Enclosing
ActualText, nested/ambiguous relationships or conflicting citation marks remain in
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
still needs differing citation versus body marks, differently labelled citations,
complex or markerless references, and carefully identified adjacency/geometric
inference where no Ref exists. Enclosing ActualText/nested cases retain content and
source evidence but do not yet expose a full editable note graph. Do not relabel
these required follow-ups as a completed note contract. Original rich producer
inputs, native Colleaf interoperability and final full-goal acceptance remain open.
