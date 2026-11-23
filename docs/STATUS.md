# Current status and handoff

Updated 2026-11-23. Read this first, then the root AGENTS.md and the architecture,
decision, compatibility and testing documents in this directory.

## Outcome

The initial standalone library and CLI are working: DOCX reading/writing, PDF
extraction/generation, and JSON/text interchange. This is a defined initial
feature set, not a complete UniOffice/UniPDF clone or Word-compatible renderer.
No Daptin integration, GitHub publication, remote push or deployment was done.

## Code map

| File | Authority |
| --- | --- |
| document.go | Public model, validation, Read/Write dispatch, JSON/text, diagnostics |
| docx_read.go | OOXML package/XML reader, relationships and visible content |
| docx_write.go | OOXML generation, styles/numbering, images and part-local relationships |
| pdf_tokens.go | PDF operands and ToUnicode code maps |
| pdf_read.go | pdfcpu adapter, page/form/widget text, fonts, encodings and spacing |
| pdf_write.go | Flow layout, embedded fonts, text tables/images and PDF generation |
| cmd/manydoc | CLI, cancellation, diagnostics and atomic output replacement |
| examples/report | Reproducible two-page DOCX/PDF/JSON example |
| testdata/corpus | Self-contained 64-case corpus and SHA-256 manifest |
| scripts | Independent fixture generation and verification tools |

## Verified in this session

- Ordinary Go tests passed: 21 top-level tests, including named corpus cases,
  content regressions, generation round trips, invalid models/IO, passwords,
  cancellation, concurrent operations, hashes and CLI atomic-output tests.
- Coverage: library 76.4%, CLI 75.0%. The example program is exercised by
  independent verification, rather than covered as a unit test.
- Race tests passed after fixing font-buffer ownership. The initial failure is
  in ignored output/race.log; output/race-final.log is the successful check.
- go vet passed; go mod verify reported all modules verified.
- CGO-disabled CLI builds passed for darwin/arm64, linux/amd64 and linux/arm64.
  Linux binaries were cross-compiled, not executed on Linux.
- All 28 non-malformed named PDF inputs matched Poppler after whitespace
  normalization, including supported password cases.
- python-docx independently reopened the generated DOCX and verified metadata,
  text, links, formatting, a table/image and header/footer. pypdf/Poppler verified
  two-page PDF output; both PDF pages were rendered and visually inspected.
  No Word/LibreOffice visual acceptance is claimed for DOCX.
- 200 deterministic damaged-file mutations completed without escaping panics.
  This is a robustness count, not 200 real-world compatibility cases.
- Short DOCX/PDF/CMap fuzz campaigns passed. The durable counts and budgets are
  in verification.json; they do not establish exhaustive safety.

Ignored output/ contains raw test logs, comparisons, generated examples, page
PNGs and the optional Python environment. bin/ contains local/cross executables.
Both are regenerable. docs/verification.json preserves a compact source summary.

## Recover and reproduce

```sh
cd ~/workspace/code/github.com/artpar/manydoc
git status --short
GOWORK=off go test ./... -count=1
GOWORK=off go test -race ./... -count=1
GOWORK=off go vet ./...
CGO_ENABLED=0 GOWORK=off go build -o bin/manydoc ./cmd/manydoc
GOWORK=off go run ./examples/report -out output
```

TESTING.md contains independent checks and optional Python/Poppler setup.
Ordinary Go tests need no Python, Poppler, Daptin checkout, customer files,
license server or external converter. Initial module download uses normal Go
module access. The go.mod selects Go 1.25; do not force an older local toolchain.

## Important implementation details

- Never use pdfcpu NewDefaultConfiguration here: it may create user files or
  exit the process. Configure each read explicitly; do not mutate global state.
- PDF objects accept common version/extension conventions; page-stream decoding
  is subsequently strict so corrupt streams cannot yield successful partial text.
- Keep per-renderer gofpdf font byte copies. Subsetting mutates its input;
  sharing embedded slices caused the race detected and fixed in this session.
- PDF font resource names are page-local. Key fonts by actual objects, not a
  document-wide cache keyed by names such as /F2+0.
- The missing document-relationships fixture was corrected because that part
  is optional for its contents. DECISIONS.md explains this; required/dangling
  parts remain errors.
- Notes, output font coverage/shaping, inline PDF images and advanced layouts
  have explicit limitations in COMPATIBILITY.md.

## Next work

There are no known failing required checks. ROADMAP.md describes capability
expansion, especially Word visual acceptance and the actual Colleaf .vdocs
adapter. The public JSON model is not a .vdocs adapter, and no Daptin performers
have been registered. Do not claim broad professional fidelity from this corpus.

All source and fixtures are local to this repository. The old Daptin evaluation
is historical context only; normal tests have no absolute-path dependency on it.
Keep this handoff, decisions and verification summary current after changes.
