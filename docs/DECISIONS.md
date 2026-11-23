# Decisions and requirements

## 2026-11-23 Project creation

The user requested a new project at `~/workspace/code/github.com/artpar/manydoc`
after discussing native Go document performers in Daptin for Colleaf. The user
explicitly requires durable knowledge transfer so another agent/human can
recover progress without the original conversation.

### Required boundaries

- Embed in a Go binary; Pandoc, LibreOffice, Poppler and remote converters are
  excluded as production requirements.
- Begin with DOCX/PDF extraction and generation. A complete office-suite clone
  or exact Word pagination is not an established capability of this project.
- Keep Daptin integration out of this standalone project. Action permission
  admission authorizes backend workflows; do not add performer-level permission
  or quota gates.
- Use independent implementation and permissively reusable components. Public
  proprietary implementation repositories do not become project source.

### Prior investigation

The initial candidates godocx v0.1.5 (also its newer main revision) and
ledongthuc/pdf 5e46805f718e were evaluated in Daptin's separate test harness:
`/Users/artpar/workspace/code/github.com/daptin/daptin/scripts/testing/document-library-evaluation`.

There were 64 named cases, mostly synthetic, and 200 deterministic damaged-file
mutations. Those mutation counts do not represent real-world compatibility.
The important failures included silently lost DOCX hyperlinks/revisions/content
controls/text boxes/fields, inaccessible table-reading internals, valid minimal
DOCX rejection, and PDF font corruption, ignored XObjects, CJK decoding errors,
spacing/operator errors and panics after opening malformed PDFs.

Use that harness's fixtures as regression inputs, but keep all permanent tests
self-contained in manydoc. Do not require the Daptin directory to run tests.

### Initial implementation choices

- Author DOCX parsing/serialization directly against OOXML using Go's ZIP/XML
  support, rather than inherit the known content-loss behavior.
- Reuse pdfcpu's PDF object parser; do not reimplement encryption and xrefs just
  to extract text. v0.11.1 supports the project's toolchain, whereas the latest
  resolved v0.16.4 requires Go 1.26.
- Reuse phpDave11/gofpdf for PDF primitives/font embedding/layout support.
- Put a small shared model in the public root package. Avoid a plugin registry,
  service, job queue, private storage authority or Daptin-specific types.
- Diagnostics and explicit lossy-conversion acknowledgement concern document
  fidelity, not authorization.

### Fixture classification correction

The inherited manifest classified `docx_missing_rels.docx` as an invalid file.
The main document has no relationship-dependent content, so its document-level
relationships part is optional; unreferenced styles/numbering ZIP members do
not make the package invalid. manydoc deliberately accepts it, like the valid
minimal DOCX. The fixture generator/manifest now reflect this interpretation.
This is distinct from a missing required document part or a dangling declared
styles relationship, which remain errors.

### PDF font buffer ownership

The first race run found that gofpdf's font-subsetting checksum generation
mutates the input TTF byte slice. Each renderer now passes its own copy to
AddUTF8FontFromBytes. Do not remove those copies or solve this by serializing
all document generation with a global lock. sfnt glyph inspection uses the
unchanged embedded fonts; its per-call work buffers are local.
