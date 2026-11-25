# Bounded PDF syntax-stream decoding

The page content, Form/annotation appearance and ToUnicode paths now use a shared
in-process decoder instead of unbounded dependency stream decoding. It supports
raw, Flate, ASCIIHex, ASCII85, RunLength and LZW (EarlyChange0/1), including filter
pipelines and TIFF/PNG predictors. Image pipelines now reuse this decoder with
separate raster allowances (PDF_IMAGE_DECODING.md). The dependency's initial
xref/object-stream parser remains separate; this is not exhaustive PDF memory protection.

## Work bounds and integrity

- Each decoded filter stage and unfiltered/cached syntax stream is limited to64MiB.
  A shrinking later stage cannot excuse a large intermediate expansion.
- A document-wide256MiB decoded-work budget charges stages before predictor
  reconstruction. Predictors do not increase output size. A stage reads at most
  its remaining allowance plus one detection byte before rejecting.
- Page content arrays also have a64MiB joined-size bound and are concatenated
  exactly; no separator is invented inside split tokens/strings.
- At most16 filters are allowed per syntax stream. Repeated indirect source streams
  reuse decoded bytes and are not decoded/charged twice. Each actual Form placement
  and its transform/source identity is still interpreted independently.
- Actual interpreted syntax across pages/Forms/appearances has a separate256MiB
  work bound, including cache hits. Cancellation is checked while reading decoded
  bytes, interpreting filters/predictors, joining page streams and reusing caches.

Exceeding a bound or encountering corrupt data rejects the read, without a partial
Document or committed CLI output. These are internal parser work/allocation bounds,
not Daptin identity, quotas, storage or action admission. A valid larger source can
also reject explicitly. No content is silently skipped to fit a bound. Buffer
capacity, temporary predictor buffers, original input and other parser objects
still contribute to process memory; these numbers are not an RSS guarantee.

The old Flate path accepted a deliberately bad zlib checksum and returned FILTERED
as a successful document. The exact pre-change Git-archive CLI reproduces that
success. The new zlib path checks stream completion/checksum and rejects the same
unchanged source. Truncated streams and incomplete RunLength records/EOD also
reject. A compact source expanding beyond64MiB, including a pipeline whose later
hex stage would shrink the result, now rejects before successful partial recovery.

## Predictors and reuse

The predictor implementation is independently authored from published TIFF/PDF
and PNG rules. TIFF horizontal differencing reconstructs component samples at
1/2/4/8/16 bits, including packed and wraparound values. PNG reconstruction uses
actual row filter bytes, correct bytes-per-pixel, first-row/column boundaries and
filters0..4. Row/sample arithmetic is validated before allocation; truncated rows,
invalid filters or excessive row dimensions reject. Fixed independent vectors
exercise all PNG filters and packed/16-bit TIFF cases.

Primary references: [PDF32000-1](https://opensource.adobe.com/dc-acrobat-sdk-docs/pdfstandards/PDF32000_2008.pdf)
section7.4.4.4 predictor parameters, section7.4.5 RunLength records and the
[W3C PNG filter reconstruction specification](https://www.w3.org/TR/png/#9Filters).
No proprietary implementation was copied. Go's zlib/ascii85 and the already-pinned
BSD-licensed hhrutter/lzw supply compression primitives. lzw is now a direct runtime
module; its existing license notice is retained and THIRD_PARTY.md is updated.
No dependency version changes. go mod tidy also records existing upstream
textlayout-testdata module checksums; that module is not a new runtime component.

## Independent source and reader checks

Twelve valid encodings, including two long-comment LZW dictionary-transition
sources, retain the exact complete raw-source SF graph: FILTERED content, actual
Courier baselines20..68pt at y100, typography/page/stream identities and metadata.
MuPDF independently decodes every source byte and verifies all eight glyph origins.
pypdf agrees on the ordinary sources and EarlyChange1 transition case. Its pinned
LZW decoder ignores DecodeParms and fails on the long EarlyChange0 source; this
is a consumer implementation limitation, not ambiguous source geometry. MuPDF and
manydoc retain that source exactly. The strict pypdf-only oracle remains executable
and deliberately fails; it was not replaced by the short no-transition control.

Python zlib independently rejects the corrupt checksum/truncated fixtures. Bounded
Python decompression verifies both large intermediate sources exceed the actual
public limit, and the checker requires atomic CLI rejection for all eight negative
cases. The Go tests also cover split page-stream tokens, repeated Forms/appearances,
compressed/corrupt ToUnicode, short cached streams, cumulative allowances, cache
cancellation and real LZW dictionary growth.

The original InDesign publication checker still passes10lists/57items/exact K
membership/16URI targets/20,531 normalized characters/DOCX and nine reflowed PDF
pages. Complete publication SF is byte-identical to a CLI built from exact
pre-change commit7727679. This change adds no output-layout/fidelity claim.

```sh
MANYDOC_PDF_STREAM_QA=output/pdf-stream-qa GOWORK=off go test -run '^TestPDF(Syntax|Decoded|Predictor)' -count=1 ./internal/engine
CGO_ENABLED=0 GOWORK=off go build -o bin/manifold ./cmd/manifold
output/qa-venv/bin/python scripts/qa/pdf/verify_pdf_syntax_streams.py output/pdf-stream-qa
# Expected FAIL on long-lzw0; retained independent consumer oracle:
output/qa-venv/bin/python scripts/qa/pdf/verify_pdf_syntax_streams.py output/pdf-stream-qa --require-pypdf-all
GOWORK=off go test -race -run '^TestPDF(Syntax|Decoded|Predictor)|FuzzPDFSyntax' -count=1 ./internal/engine
GOWORK=off go test -run '^$' -fuzz '^FuzzPDFSyntaxFilters$' -fuzztime=10s -parallel=2 ./internal/engine
output/qa-venv/bin/python scripts/qa/layout/verify_published_pdf.py output/pdf-stream-published
```

The independent negative CLI probes require a fresh output directory with no
*-rejected.sf files. Optional Python/MuPDF remain QA only; runtime decoding stays
inside Go with no CGO, executables, services or document uploads. The remaining
initial-parser resource audit and full original acceptance audit remain open.
The subsequent image-stage/pixel audit is recorded in PDF_IMAGE_DECODING.md.
