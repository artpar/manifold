# Testing and verification

All ordinary tests are self-contained Go tests. Python and Poppler are optional
independent verification tools, never production dependencies.

## Required checks

```sh
GOWORK=off go test ./... -count=1
GOWORK=off go test -race ./... -count=1
GOWORK=off go vet ./...
GOWORK=off go mod verify
CGO_ENABLED=0 GOWORK=off go build -o bin/manydoc ./cmd/manydoc
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GOWORK=off go build -o bin/manydoc-linux-amd64 ./cmd/manydoc
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 GOWORK=off go build -o bin/manydoc-linux-arm64 ./cmd/manydoc
git diff --check
```

Cross-builds establish compilation, not execution on Linux. Keep production
dependencies separate from fixture tooling and avoid global configuration.

## Corpus and regression tests

`testdata/corpus/manifest.json` describes 31 DOCX and 33 PDF named cases, including
malformed inputs expected to reject. `TestCorpusHashes` verifies their SHA-256
hashes. Fixtures are mostly synthetic; one PDF is the published W3C dummy file.

Tests prove preservation of previously lost DOCX visible content, valid minimal
packages, page-specific PDF fonts, glyph-positioned spaces, Unicode/CJK,
XObjects/widget appearances, valid quote operators, encryption, generation
round trips, concurrent independent operations and cancellation. CLI tests also
verify that failure preserves an existing output and removes its temporary file.

`TestMalformedInputsNeverPanic` applies 100 deterministic mutations per input
format. These are robustness checks, not 200 real-world compatibility cases.

## Fuzzing

```sh
GOWORK=off go test -run '^$' -fuzz '^FuzzReadDOCX$' -fuzztime=15s -fuzzminimizetime=1s -parallel=2 -timeout=35s .
GOWORK=off go test -run '^$' -fuzz '^FuzzReadPDF$' -fuzztime=15s -fuzzminimizetime=1s -parallel=2 -timeout=35s .
GOWORK=off go test -run '^$' -fuzz '^FuzzUnicodeMap$' -fuzztime=15s -fuzzminimizetime=1s -parallel=2 -timeout=35s .
```

Fuzzing is nondeterministic and budget-limited. Preserve any discovered failure
as a permanent regression fixture. A short passing campaign is not evidence of
exhaustive safety or full standards conformance.

## Independent generated-output checks

```sh
GOWORK=off go run ./examples/report -out output
uv venv output/qa-venv
uv pip install --python output/qa-venv/bin/python -r scripts/requirements.txt
output/qa-venv/bin/python scripts/verify_outputs.py output
output/qa-venv/bin/python scripts/compare_pdf_text.py
pdftoppm -f 1 -l 2 -scale-to 1200 -png output/report.pdf output/report-page
```

The first script reopens DOCX with python-docx and checks metadata, hyperlinks,
formatting, a table, image, headers/footers and text. It reopens PDF with pypdf,
checks two pages and compares expected content with Poppler. The second compares
all 28 non-malformed named PDF fixtures with Poppler after whitespace
normalization, including password cases. Inspect every generated PDF page PNG.

DOCX structure checks do not establish visual Word rendering. No Microsoft Word
or LibreOffice rendering acceptance has been completed. The generated PDF
example has been visually inspected, including Unicode, table layout and both
pages; that only qualifies this example.

## Regenerating fixtures

```sh
output/qa-venv/bin/python scripts/generate_fixtures.py
GOWORK=off go test ./... -count=1
```

The generator uses python-docx, ReportLab, pypdf and PyMuPDF with local assets.
Regeneration updates hashes because ZIP timestamps/package versions may change.
Review changed fixture bytes and expectations; do not blindly update oracles.
See THIRD_PARTY.md for asset provenance and notices.

## Recording a new handoff milestone

Preserve actual command output before regenerating the compact verification
summary. The summary script asserts that the captured checks passed; it does
not run checks or infer success from absent logs.

```sh
mkdir -p output
GOWORK=off go test -json -cover ./... -count=1 > output/tests-final.json
GOWORK=off go test -race ./... -count=1 > output/race-final.log 2>&1
GOWORK=off go vet ./... > output/vet-final.log 2>&1
GOWORK=off go mod verify > output/modules-final.log 2>&1
```

Capture the three fuzz commands above to output/fuzz-docx-final.log,
output/fuzz-pdf-final.log and output/fuzz-cmap.log respectively. Complete the
CGO-disabled builds and independent example/PDF comparisons above, then run:

```sh
python3 scripts/record_verification.py
```

Review docs/verification.json, update STATUS.md with changed limitations and
outcomes, and retain a local Git checkpoint. Do not publish remotely unless
requested. Never reset or clean an unfamiliar working tree to recover progress.
