# DOCX font-size precision and shared note setup

## Contract

The Go/SF model retains finite nonnegative point sizes; zero inherits. The published OOXML [ordinary size](https://learn.microsoft.com/en-us/dotnet/api/documentformat.openxml.wordprocessing.fontsize?view=openxml-3.0.1)
and [complex-script size](https://learn.microsoft.com/en-us/dotnet/api/documentformat.openxml.wordprocessing.fontsizecomplexscript?view=openxml-3.0.1)
properties express values in half-points. Strict DOCX output keeps the previous
ErrUnsupported outcome for non-half-point sizes, with an actionable message.
WriteOptions.AllowLossy now permits nearest half-point projection, ties upward,
with maximum deviation .25pt. Values rounding below one half-point or above
uint32 half-points reject atomically; acknowledgement does not clamp or overflow.
Other unsupported features still reject. No caller model, source or asset is mutated.

A projected ordinary run carries ignorable md:fontSize containing the original
round-trippable float. Both explicit standard sz and szCs carry the same projected
integer. Reimport restores original precision only if those actual properties,
inherited effective sizes and the deterministic projection agree. Missing,
nonfinite, redundant or contradictory intent rejects; text and properties are not
suppressed. Removing the optional attribute exposes ordinary portable sizes.
The docx_font_size_precision diagnostic states that checked original intent was
restored and portable appearance uses half-points. Restored SF persists that intent
through plain/gzip JSON. Independent consumers need not preserve private intent.

The rule applies through the same run encoder/decoder to ordinary and sectioned
body text, headings, tables/lists, running stories, note bodies, first/repeated
references and styled/variant body marks. Complex-script szCs intent is restored
alongside ordinary size; conflicting script properties reject. The styled mark's
derived gutter uses original intent consistently, rather than mistaking its
physical half-point projection for a foreign indentation edit.

This does not change native PDF's existing .01pt admission contract, supply output
fonts, evaluate fields or implement Word layout. The example's fractional-sizes
mode intentionally emits SF/DOCX only and labels this in flag help. Normal examples
still generate native PDFs. Missing/shaping/unsupported precision outcomes remain
explicit, and no automatic font substitution is added to native output.

## Sectioned notes regression

The sectioned writer previously initialized IDs/counts separately and omitted the
custom/variant/style catalog. A supported styled note caused a nil-pointer panic;
custom/variant display could also be lost. Both writers now use initializeNotes
for identity, ordering, labels, variants, custom marks and first/repeat state.
Relationships and page/story geometry remain scoped normally. An explicit
pre-fix regression reproduced the panic; the same rich note graph now round trips,
including after fractional projection. This closes a genuine path mismatch, not
a reduced oracle. No application/access/quota code changed.

The malformed-metadata test also exposed encoding/xml accepting duplicate expanded
attribute names. parseXML now rejects them (including namespace aliases), while
allowing the same local name in distinct namespaces. This prevents ambiguous
standard/intent values. Existing source fixtures/oracles are unchanged.

## Evidence

The unchanged original publisher guide (hash/source in PDF_NOTES.md) exports as
943 KiB DOCX. Independent lxml checks all 87,442 explicit projected runs against
original point values and standard sz/szCs; python-docx opens the package. Reimport
preserves editable structure, formatting, relationships, note definitions/sites,
backlinks and original font sizes exactly, after the declared source-provenance
omission. Seven relationship-resolved image instances retain their byte hashes;
fourteen dimensions differ within one EMU (1/12700pt). Those differences are
measured serialization outcomes, not changed source values or an updated oracle.

LibreOffice exports the reflowed guide to 39 pages; both note bodies occur once on
page 39. Pages 1, 20 and 39 were visually inspected: cover/contents, technical text/
code/list and notes remain readable. Original 71-page pagination is not preserved
or claimed. Original font names can select installed substitutes; earlier custom
note-label PDF hyperlink omissions remain a reader qualification limit.

A two-page sectioned example independently paints both original 6.959646pt body
marks at 7pt. Its 17 projected runs preserve exact SF intent, including first and
repeated references; both rendered pages were inspected. Actual ProseMirror checks
89,433 nodes/marks/attributes across the example, its Word SF and the original guide
Word SF. Tests also cover ties/bounds, strict/acknowledged admission, immutable
sources, complex-script precision, contradictory/missing properties, stripped
intent, namespace-alias duplicates and all modeled stories. See verification.json
for full command results and exact qualification scope.

## Reproduction

```sh
GOWORK=off go run ./examples/notes -variant-labels -styled-labels -fractional-sizes -sectioned -out output/note-precision/sectioned
GOWORK=off CGO_ENABLED=0 go build -o bin/manydoc ./cmd/manydoc
bin/manydoc extract -format sf -o output/note-precision/sectioned/notes-docx.sf output/note-precision/sectioned/notes.docx
/Applications/LibreOffice.app/Contents/MacOS/soffice -env:UserInstallation=file:///tmp/manydoc-note-precision-lo --headless --convert-to pdf --outdir output/note-precision/sectioned/lo output/note-precision/sectioned/notes.docx
bin/manydoc convert -allow-lossy -o output/note-inference/guide-precision.docx output/note-inference/guide-styled.sf.gz
bin/manydoc extract -format sf.gz -o output/note-precision/guide-docx.sf.gz output/note-inference/guide-precision.docx
output/qa-venv/bin/python scripts/verify_docx_precision.py --guide-sf output/note-inference/guide-styled.sf.gz --guide-docx output/note-inference/guide-precision.docx --guide-roundtrip output/note-precision/guide-docx.sf.gz
NODE_PATH=output/sf-qa/node_modules node scripts/verify_structured.cjs output/note-precision/sectioned/notes.sf output/note-precision/sectioned/notes-docx.sf output/note-precision/guide-docx.sf.gz
GOWORK=off go test -race -run 'TestDOCXFractional|TestDOCXSize|TestDOCXSectionedStyledNote|TestXMLDuplicate' -count=1 .
```

Raw guide downloads, outputs and QA environments stay ignored. Python/Node/
LibreOffice are independent development readers, never runtime requirements.
The full goal remains incomplete: heterogeneous/mixed-direction note labels,
nested/enclosing-ActualText and broader untagged graphs, the actual Colleaf native
adapter, broader original DOCX corpus and final acceptance remain required.
