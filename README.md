# manydoc

Pure-Go document extraction, generation, and conversion for embedded applications.

**Initial implementation.** See [current status](docs/STATUS.md) for verified
behavior and remaining work. This is not a drop-in UniOffice/UniPDF replacement
or a Word-compatible layout engine.

The library reads and writes DOCX, extracts positioned text, raster images and supported tagged structure or disclosed geometric inference from PDF, generates PDFs from
structured content, and exchanges documents as ProseMirror JSON (`.sf`), legacy JSON and text.
The [structured-format decision](docs/STRUCTURED_FORMAT.md) records the schema,
tradeoffs and required unfinished expansion.
Applications retain ownership of identity, authorization, persistence and jobs.
No external Office installation or conversion service is required.

## Recovering the project

- [Status and handoff](docs/STATUS.md): implemented work, verification, open issues,
  and the exact next steps.
- [Architecture](docs/ARCHITECTURE.md): data model, format boundaries and layout.
- [Decisions](docs/DECISIONS.md): requirements, design rationale and exclusions.
- [Contributor instructions](AGENTS.md): engineering rules and completion checks.
- [Compatibility](docs/COMPATIBILITY.md): supported features and fidelity limits.
- [Testing](docs/TESTING.md): regression corpus and independent verification.

## Build and use

Go 1.25 or newer is required. There are no production executable/service/CGO
dependencies. PDF output embeds licensed Go fonts.

```sh
CGO_ENABLED=0 GOWORK=off go build -o bin/manydoc ./cmd/manydoc
bin/manydoc extract input.docx
bin/manydoc extract -format sf -o document.sf input.docx
bin/manydoc convert -allow-lossy -o report.docx document.sf
bin/manydoc convert -allow-lossy -o report.pdf document.sf
bin/manydoc extract -format sf -o recovered.sf report.pdf
# Inspect untagged physical paint order instead of geometric structure:
bin/manydoc extract -pdf-skip-inference -format sf -o physical.sf report.pdf
```

Flags precede the input filename. SF uses the versioned ProseMirror profile;
`ReadStructured`/`WriteStructured` expose the tree directly, while `ToStructured`
and `FromStructured` bridge the Go conversion engine. Legacy JSON follows the public `Document`
model in `document.go`; unknown fields are rejected. Review diagnostics before
using `-allow-lossy` to acknowledge them. Text output includes page
boundaries as form-feed characters. `extract -format json` retains diagnostics.

For an imported document whose editing/layout semantics cannot be preserved,
conversion returns `ErrLossy`. Review its diagnostics, then explicitly allow
that reduction if appropriate:

```sh
bin/manydoc convert -allow-lossy -o imported.docx input.pdf
```

Unsupported output features still return errors. Conversion writes to a
temporary file and replaces the destination only after success. A failed
conversion leaves an existing output file unchanged.

For encrypted PDFs, supply the password through an environment variable:

```sh
bin/manydoc extract -password-env DOCUMENT_PASSWORD protected.pdf
```

The variable must already be set; the CLI does not print its value.

## Embed the library

```go
doc := &manydoc.Document{
    Metadata: manydoc.Metadata{Title: "Report"},
    Blocks: []manydoc.Block{
        {Kind: manydoc.Heading, Level: 1,
            Runs: []manydoc.Run{{Text: "Quarterly report"}}},
        {Kind: manydoc.Paragraph,
            Runs: []manydoc.Run{{Text: "Generated inside the Go process."}}},
    },
}
err := manydoc.Write(ctx, destination, doc, manydoc.DOCX, manydoc.WriteOptions{})
```

Read with `manydoc.Read(ctx, source, manydoc.DOCX, manydoc.ReadOptions{})` or
`manydoc.ReadPDF(ctx, source, manydoc.ReadOptions{Password: password})`.
Inputs/outputs are `io.Reader`/`io.Writer`; callers control persistence and
authorization. Documents must not be concurrently mutated by the caller.

Generate a two-page example with links, formatting, a table, image and Unicode:

```sh
GOWORK=off go run ./examples/report -out output
```

The SF-first examples exercise merged/nested cells, images, lists and rich notes:

```sh
GOWORK=off go run ./examples/image-fit
GOWORK=off go run ./examples/paragraph-breaks
GOWORK=off go run ./examples/page-breaks
GOWORK=off go run ./examples/page-breaks -empty-note -out output/page-breaks/empty
GOWORK=off go run ./examples/recursive-tables
GOWORK=off go run ./examples/table-continuation
GOWORK=off go run ./examples/rich-cells -out output/rich-cells
GOWORK=off go run ./examples/rich-cells -out output/rich-cells/continued -continued
GOWORK=off go run ./examples/merged-tables -out output/merged-tables
GOWORK=off go run ./examples/lists -out output/lists
GOWORK=off go run ./examples/lists -out output/lists/paginated -page-height 220
GOWORK=off go run ./examples/notes -out output/notes
GOWORK=off go run ./examples/notes -out output/notes/continued -page-height 220 -long-note
```

Generated files are deliberately ignored by Git. See the independent
verification commands in [Testing](docs/TESTING.md).

## Development

```sh
GOWORK=off go mod download
GOWORK=off go test ./...
GOWORK=off go test -race ./...
GOWORK=off go vet ./...
```

Explicit lists use `Block{Kind: ListBlock, List: "number"}` with `Items` containing
`ListItem{Blocks: ...}`. `Start` is an optional `*int` (nil means one, zero is
valid); item blocks retain continuation paragraphs, nested lists and assets.
Legacy paragraph `List` fields remain accepted as flat list runs.

Inline note references use `Run{Note: &NoteReference{Kind: Footnote, ID: "source"}}`;
define the body once in `Document.Notes`. IDs are scoped by kind and need not be
numbers. Draft unreferenced notes may persist as SF/JSON but must be anchored
before rendering. DOCX notes and PDF footnotes/endnotes accept rich blocks,
including images and nested tables. PDF footnotes continue between measured
fragments. Oversized body-table groups continue with their cell flows and note
references, including nested tables and oversized merged footnote tables.
Explicit cell/note breaks propagate through native PDF layout, including blank
pages. `Block.PageBreakBefore` requests a paragraph page start without adding a
blank page when already at page start; it is distinct from `PageBreak`. DOCX uses
paragraph properties with checked metadata to preserve explicit controls and
mitigate LibreOffice content loss. Cell/note pagination still differs between
readers. Images fit full container width/height without cropping or resampling.
A figure-first footnote can use a linked continuation notice. PNG/JPEG output is
fully validated, with a 25-million-pixel allocation bound and explicit errors for
invalid data or unrepresentable dimensions.

The Go engine model starts in `document.go`; the canonical SF tree is in
`structured.go`. `Read` and `Write` are
the application boundary. `errors.Is(err, manydoc.ErrUnsupported)` identifies
unsupported output/features; `errors.Is(err, manydoc.ErrLossy)` identifies
unacknowledged imported-document diagnostics.

## References

- [ECMA-376 Office Open XML](https://ecma-international.org/publications-and-standards/standards/ecma-376/)
- [PDF specifications](https://pdfa.org/resource/pdf-specification-archive/)

This is an independent implementation using published formats and open-source
components. No UniOffice/UniPDF implementation code is included.

The Unicode typography example exercises measured spaces, logical NFC text,
wrapping across formatting runs, decorations and note spacing:

```sh
GOWORK=off go run ./examples/typography -out output/typography
```

See the current support contract and unfinished structure/layout work in
[COMPATIBILITY.md](docs/COMPATIBILITY.md) and [STATUS.md](docs/STATUS.md).


Run `GOWORK=off go run ./examples/repeated-headers` for SF/DOCX/PDF outputs with
repeating merged table headers, original raster assets, oversized body cells and
linked footnotes. `Row.RepeatHeader` marks a contiguous row prefix; native PDF
continues body/nested/footnote tables with that prefix and preserves authored SF.
See docs/COMPATIBILITY.md for page-control and producer qualification limits.


`GOWORK=off go run ./examples/text-style` writes a rich SF/DOCX/PDF example and
an output-only TTF for the CLI example. Runs support FontFamily, FontSize in
points and Color (#RRGGBB/auto). PDF has Go and Go Mono styles built in; supply
FontFace values with WriteOptions.PDFFonts for additional TTF faces. Missing
families/styles/glyphs return errors. PDFFontMap explicitly selects alternatives.

```sh
bin/manydoc convert -pdf-font 'App Sans=output/text-style/app-sans.ttf' -o output/text-style/cli.pdf output/text-style/text-style.sf
# An explicit alternative for a retained source font, when desired:
bin/manydoc convert -allow-lossy -pdf-font-map 'Calibri=Go' -pdf-font-map 'Cambria=Go' -o output/converted.pdf input.docx
```

DOCX sizes require half-points; PDF sizes require 0.01pt precision. Theme/script
choices, shaping/bidi, broader PDF recovery and real-export qualification remain
unfinished; see docs/COMPATIBILITY.md and docs/STATUS.md.


PDF → SF also recovers editable font families, physical point sizes and device
text colors, with source evidence and explicit uncertainty/loss diagnostics.
Source font names are lookup requests; configure output faces or mappings before
reflow. Imported sizes finer than writer precision require explicit caller choices.
Optional QA: `output/qa-venv/bin/python scripts/verify_pdf_text_styles.py` checks an
independent ReportLab source through SF, DOCX and mapped-font PDF. See the current
recovery contract and remaining work in docs/COMPATIBILITY.md.


DOCX imports calculate run theme tint/shade and document color mappings into RGB.
Theme/system bindings and mismatched cached colors have explicit diagnostics;
unsupported palette choices may use a final cached RGB with uncertainty. Optional
QA: `output/qa-venv/bin/python scripts/verify_docx_theme_colors.py`.


DOCX font slots and theme languages resolve into explicit styled spans instead of
one family for an entire mixed-script run. Unused selector/theme losses and uncertain defaults have diagnostics; language
and paragraph/run direction bindings now persist in SF and DOCX. Optional QA:
`output/qa-venv/bin/python scripts/verify_docx_font_selection.py`. Broader font
selection and native shaping/bidi remain required work; see docs/STATUS.md.


`GOWORK=off go run ./examples/glyphs` exercises native positioned marks and
monochrome supplementary glyphs with caller-supplied TTFs, tables, links, notes
and running content. Optional QA: `output/qa-venv/bin/python scripts/verify_glyphs.py`.
Native joining/bidi/complex-script support is still unfinished; errors remain
explicit. See docs/COMPATIBILITY.md for the current glyph/font contract.


`GOWORK=off go run ./examples/language` demonstrates general/East Asian/complex
language bindings and independent paragraph/run direction in SF/DOCX. Native PDF
examples qualify language-tagged LTR localization, kerning, ligatures and re-shaping
after wrapping. Optional QA: `output/qa-venv/bin/python scripts/verify_language.py`.
RTL PDF and broader contextual/script/style shaping remain unfinished.


`GOWORK=off go run ./examples/shaping` demonstrates ordinary default text with
kerning, ligatures and final-line re-shaping; language tags are optional. QA:
`output/qa-venv/bin/python scripts/verify_default_shaping.py`. Full original font
embedding increases small PDF size. Bidi/joining and differing-style/font context
remain required work; this is not a Word-equivalent layout engine.
