# Manifold

[![CI and Release](https://github.com/artpar/manifold/actions/workflows/ci.yml/badge.svg)](https://github.com/artpar/manifold/actions/workflows/ci.yml)

Pure-Go rich document processing for embedded applications and the command line.

Convert **DOCX → structured format → DOCX/PDF**, and recover structured content
from PDF. The shared format is a versioned ProseMirror JSON profile, with typed Go
trees for programmatic manipulation. Runtime conversion needs no CGO, external
executables, services or license servers.

## Install

Go 1.25 or newer:

```sh
go get github.com/artpar/manifold
go install github.com/artpar/manifold/cmd/manifold@latest
```

## CLI

```sh
manifold extract -format sf -o document.sf input.docx
manifold convert -allow-lossy -o document.docx document.sf
manifold convert -allow-lossy -o document.pdf document.sf
manifold extract -format sf -o recovered.sf document.pdf
```

Flags precede the input filename. Review import diagnostics before acknowledging
losses with `-allow-lossy`. Unsupported output still rejects. `.sf.gz` compresses
the same structured format. The CLI commits an output file only after success.

## Go API

```go
import "github.com/artpar/manifold"

doc, err := manifold.Read(ctx, source, manifold.DOCX, manifold.ReadOptions{})
if err != nil {
    return err
}
return manifold.Write(ctx, destination, doc, manifold.SF, manifold.WriteOptions{})
```

The public package exposes document types, conversion functions, options and
errors. Callers own readers/writers, persistence and authorization. See
[sections](examples/sections), [rich tables](examples/merged-tables) and
[notes](examples/notes) for complete examples.

## Conversion model

SF is the canonical structured representation shared by the converters:

```text
DOCX ↔ SF ↔ PDF
       ↕
     VDOCS
```

VDOCS is optional native snapshot interchange, not another canonical model or a
second DOCX/PDF pipeline. Word support is DOCX; legacy binary DOC is not provided.
PDF import reconstructs recoverable structure and positions with diagnostics;
source-identical layout is not promised.

## Support and layout

Paragraphs/headings, formatted runs, links, lists, merged/nested tables, images,
metadata, page settings, headers/footers and rich notes share the same model.
PDF extraction distinguishes explicit structure from geometric inference and
retains physical pages, positions, navigation and assets. Missing glyphs,
unsupported shaping and representation losses have explicit outcomes.

The optional offline [VDOCS adapter](docs/reference/VDOCS_FORMAT.md) supports a
qualified native editor snapshot profile; no editor service is required.
Existing files retain `manydoc-prosemirror/1` and embedded metadata identifiers
for compatibility. This is not a Word layout engine or full UniDoc API replacement.

```text
manifold.go          public API
cmd/manifold/        CLI and file handling
internal/engine/     conversion model, codecs and layout; adjacent unit tests
internal/bidilevels/ attributed bidi algorithm and conformance data
examples/            runnable public API examples
scripts/qa/          optional independent qualification, grouped by concern
testdata/            pinned synthetic and original reference documents
docs/                current guides, reference contracts, reports and history
```

Start with [documentation](docs/README.md), [compatibility](docs/COMPATIBILITY.md),
[verification](docs/verification/FINAL_AUDIT.md) or [testing](docs/TESTING.md).
Original-source fixtures and third-party notices are documented in
[THIRD_PARTY.md](THIRD_PARTY.md). The project is MIT licensed.


## CI and releases

GitHub Actions tests pushes and pull requests on Linux, macOS and Windows, with
race checks on Linux. A new version in `VERSION` on `main` automatically creates
the tag and release after checks pass. The pipeline builds CGO-disabled binaries
for Linux/macOS/Windows on amd64/arm64, packages licenses and publishes checksums.
See [CI/CD](docs/CI.md). No workstation build/upload step is needed for releases.

## PowerPoint presentations

Manifold also includes a pure-Go presentation API and CLI, integrated from Puppt:

```sh
manifold presentation inspect deck.pptx --json
manifold presentation render deck.pptx --slide 1 --out slide.png --json
```

Import `github.com/artpar/manifold/presentation` for inspection, planned edits,
validation, review and diagnostic PNG rendering. See [presentation guide](docs/presentation/README.md)
for supported operations and limits. PPTX/SF and PPTX/PDF conversion are not yet supported.
