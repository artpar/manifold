# manydoc

Pure-Go document extraction, generation, and conversion for embedded applications.

**Initial implementation.** See [current status](docs/STATUS.md) for verified
behavior and remaining work. This is not a drop-in UniOffice/UniPDF replacement
or a Word-compatible layout engine.

The library reads and writes DOCX, extracts text from PDF, generates PDFs from
structured content, and exchanges documents as JSON/text.
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
bin/manydoc extract -format json -o document.json input.docx
bin/manydoc convert -o report.docx document.json
bin/manydoc convert -o report.pdf document.json
```

Flags precede the input filename. JSON input follows the public `Document`
model in `document.go`; unknown fields are rejected. Text output includes page
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

Generated files are deliberately ignored by Git. See the independent
verification commands in [Testing](docs/TESTING.md).

## Development

```sh
GOWORK=off go mod download
GOWORK=off go test ./...
GOWORK=off go test -race ./...
GOWORK=off go vet ./...
```

The public interchange model starts in `document.go`. `Read` and `Write` are
the application boundary. `errors.Is(err, manydoc.ErrUnsupported)` identifies
unsupported output/features; `errors.Is(err, manydoc.ErrLossy)` identifies
unacknowledged imported-document diagnostics.

## References

- [ECMA-376 Office Open XML](https://ecma-international.org/publications-and-standards/standards/ecma-376/)
- [PDF specifications](https://pdfa.org/resource/pdf-specification-archive/)

This is an independent implementation using published formats and open-source
components. No UniOffice/UniPDF implementation code is included.
