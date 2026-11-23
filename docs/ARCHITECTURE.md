# Architecture

## Application boundary

`Read(context.Context, io.Reader, Format, ReadOptions)` produces a `Document`.
`Write(context.Context, io.Writer, *Document, Format, WriteOptions)` serializes it.
The model and format dispatch live in the root `manydoc` Go package so callers
can embed the library without importing a service framework or Daptin packages.

The initial document model contains paragraphs/headings with formatted runs,
links, lists, tables with nested blocks and column spans, embedded images,
explicit page breaks, metadata, headers/footers, notes and diagnostics.
Dimensions use points (72 per inch). JSON uses standard base64 for image bytes.

## Interpretation and fidelity

Extraction must preserve visible text of supported constructs, including text
inside hyperlinks, content controls, inserted revisions, text boxes and cached
fields. Deleted revision text is excluded from the final visible view.
Diagnostics disclose lost editing/presentation semantics. Formatting conversion
with outstanding diagnostics requires explicit `WriteOptions.AllowLossy`.
Unsupported writer features still produce errors; this option does not convert
unsupported content into silently missing output.

Plain text is an explicitly reduced representation. PDF extraction produces
page-separated text rather than inventing an editable Word layout. PDF output
renders the structured model; imported Word pagination is not authoritative.

## DOCX

`docx_read.go` uses Go archive/zip and encoding/xml. It resolves package
relationships inside the archive, supports optional package parts, rejects
duplicate/malformed paths and invalid document roots, and never fetches external
relationships. Table cells are public model values rather than opaque objects.

`docx_write.go` creates an OOXML package with deterministic member ordering,
styles, numbering, metadata, relationships, links, tables, embedded PNG/JPEG
images and page setup. The output is assembled before writing to the caller.

## PDF

The parser foundation is pdfcpu for xrefs, streams, object resolution,
encryption and PDF versions. manydoc owns a content-stream interpreter and font
decoding layer to preserve text from page-local font resources, ToUnicode maps,
Form XObjects, annotations and supported predefined encodings.

The output foundation is gofpdf with embedded, licensed Go fonts.
Layout is explicit and limited; fonts/glyphs and unsupported layout features
must be checked rather than silently rendering corrupt text.

Font resources are keyed by their actual object identity inside a page, rather
than their local resource names. ToUnicode maps and common Adobe glyph-name
encodings are interpreted separately from PDF syntax parsing. Public PDF reads
turn dependency panics into errors and discard partial results.

pdfcpu receives a per-call configuration, never its default user-config loader.
Object parsing accepts common PDF version/extension conventions; page-stream
decoding is strict so corrupt streams cannot silently return partial text.
gofpdf receives per-renderer copies of font buffers because its subset generation
mutates them. Glyph inspection uses unchanged embedded bytes and local buffers.

The CLI uses these same Read/Write paths. It creates a temporary destination and
renames it only after successful conversion. It does not introduce a separate
conversion engine or format-specific behavior.

## Tests

Unit and composition tests should cover read→model→write→read, external-reader
compatibility, the previously discovered library defects, malformed documents,
context cancellation and independent concurrent operations. Independent Poppler
or Python tools may be used for test oracles; they are not runtime dependencies.
