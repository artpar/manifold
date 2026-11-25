# Direct PDF encryption dictionaries

The four preserved MuPDF exports that failed before62badc0 now decrypt through
pure Go with the supplied password. Their original ciphertext hashes and strict
complete-SF oracle remain unchanged. This closes that concrete compatibility defect;
it does not complete the broader rich-document acceptance goal.

## Parser adaptation

pdfcpu v0.11.1 records trailer Encrypt only when it is an indirect reference.
MuPDF may write the dictionary directly in the trailer/xref-stream dictionary.
Previously the reader attempted to interpret ciphertext as compressed object data
and failed with zlib: invalid header before document extraction.

`internal/engine/pdf_resources.go` inspects unencrypted trailer structure and adds a private,
in-memory incremental section containing an indirect copy of that dictionary.
Original PDF objects/ciphertext/file IDs retain their identities and contents.
The original source slice/file is never modified. Object offsets remain relative
to the PDF header; non-PDF pre-header noise is omitted only from the parser view
because the dependency's repair path otherwise misreads retained stream bytes.
No adapted PDF is written by ReadPDF or used as a cached SF substitute.

The fresh object number accounts for inspected Size, classical xref subsections,
xref-stream Index ranges and previous sections. A low declared Size cannot overwrite
an active member in the qualified Index case. Root/Info/ID inheritance and hybrid
xref processing remain intact. The adapter removes stream-only dictionary keys
from its new conventional trailer; Prev still reaches the original sections.
Applying it again to its own result is a no-op. Unrelated/indirect-encryption or
undiscovered inputs retain the original parser path and existing repair behavior.

Discovery inspects at most64MiB per trailer tail and256MiB across visited tails,
checks cancellation and avoids repeated offsets. These are private inspection
bounds, not application quotas or a complete initial-parser allocation guard.
Earlier xref/object-stream expansion in the dependency still has the boundary
recorded in PDF_PARSER_STREAMS.md. No module-cache/vendor/application patch, CGO,
external executable/service, new public API or dependency version is introduced.

Password authentication and decryption remain in the existing Go security handler.
Wrong/empty passwords reject without a Document or committed CLI output. A valid
reader password works when the document denies copying; no Daptin identity,
action-admission or performer permission logic is added.

Published basis: ISO32000-1 sections7.5.5/7.5.6/7.6 and
[PDF Association encryption clarifications](https://pdf-issues.pdfa.org/32000-2-2020/clause07.html).
The file IDs and encryption-dictionary strings are not re-encrypted. The code is
independently authored around existing attributed open-source parsers/security
handling; no proprietary UniDoc implementation was copied or translated.

## Source and independent evidence

Default Go tests independently construct deterministic standard-security R2 source
syntax with ordinary/hybrid and stream xrefs, compressed metadata/dictionaries,
regular/bold Courier, URI relationships and exact red/green raster samples. They
compare full Document values against unencrypted controls and serialize through SF.
Reader and owner passwords agree; wrong/empty passwords reject. Source bytes are
immutable; dictionary adaptation is idempotent. Additional cases cover pre-header
noise, inherited trailer entries, low Size/Index collision avoidance, malformed
security metadata, no-op discovery and cancellation.

The exact original four MuPDF RC4-40/128 and AES-128/256 ciphertext files retain the
SHA256 values recorded in the earlier ledger. An exact62badc0 Git-archive CLI still
fails each original file; the current CLI passes the unchanged full control SF.
The existing parser verifier now requires success whenever encrypted sources are
present; --require-encrypted remains a compatibility alias, not a test gate.

The richer independent corpus includes four further actual MuPDF encrypted saves
from owned synthetic content, the two Go R2 sources and five repair/incremental
variants. pypdf7.1.1 and MuPDF1.28.2 check decrypted metadata, fonts, URI, original
raster bytes, displayed positions and page bounds. Reader/owner CLI SF must equal
the complete clear control; wrong/empty CLI attempts preserve an existing output.
Both independent readers also inspect the adapted trailer, unchanged encryption
values/file IDs and decrypted source objects5/6/10. Fresh MuPDF saves preserve
existing QA files rather than regenerating a failing ciphertext during a fix.

The clear, Go R2 and MuPDF AES256 complete physical source renders are pixel-identical
in MuPDF. Enlarged text/asset swatches were visually inspected: exact regular/bold
text and both raster colors agree. There is no new general layout-fidelity claim.

Two original strict repair-shape comparisons remain separate reader discrepancies:

- pypdf cannot resolve the original header-relative, pre-header-noise variant.
  MuPDF reads the original; pypdf verifies its canonical header-relative view.
  --require-pypdf-prefix retains the original strict failing oracle.
- MuPDF does not decrypt the original incremental variant whose latest trailer
  omits inherited Encrypt/ID. pypdf reads the original; MuPDF verifies the adapted
  view. --require-mupdf-repairs retains that original strict failing oracle.

These repair variants do not establish PDF conformance or universal reader agreement.
They do not replace the four original-producer success requirements.

Original publication and MQTT complete SF remain byte-identical to62badc0:
`2d630c8ff72ec594b95b67542d66bf2df13886380672f6a213e5713b8095a7b2` and
`14742c55e8295feb0058a74cc33afe394f1ac340fa82aa422af3ad325524f526`.
The publication structure/content/DOCX and nine-page PDF reflow verifier passes.

## Reproduce and continue

```sh
GOWORK=off MANYDOC_DIRECT_ENCRYPT_QA=output/pdf-direct-encrypt-qa go test -run TestPDFDirectEncryption -count=1 ./internal/engine
GOWORK=off CGO_ENABLED=0 go build -o bin/manifold ./cmd/manifold
output/qa-venv/bin/python scripts/qa/pdf/verify_pdf_direct_encryption.py output/pdf-direct-encrypt-qa --generate-mupdf
output/qa-venv/bin/python scripts/qa/pdf/verify_pdf_parser_streams.py output/pdf-parser-qa --generate-encrypted --require-encrypted
# Retained reader discrepancies on original repair-shaped sources; currently fail:
output/qa-venv/bin/python scripts/qa/pdf/verify_pdf_direct_encryption.py output/pdf-direct-encrypt-qa --require-pypdf-prefix
output/qa-venv/bin/python scripts/qa/pdf/verify_pdf_direct_encryption.py output/pdf-direct-encrypt-qa --require-mupdf-repairs
GOWORK=off go test ./...
GOWORK=off go test -race ./...
GOWORK=off go vet ./...
GOWORK=off CGO_ENABLED=0 go build -o output/manydoc ./cmd/manifold
git diff --check
```

Python/MuPDF are optional local QA only. Current required-check results and scopes
are in verification.json. The initial parser allocation concern and the full
source/semantic/reflow/native/API/visual acceptance matrix remain open. Return to
ACCEPTANCE.md and inspect actual untagged note sources alongside their original
DOCX note graphs; do not declare structure recovery complete from matching text.


Current checks pass: full tests45.412s (CLI cached), full final-production race
458.796s/CLI2.796s and focused current race1.936s including later test-only
exports/malformed metadata cases; CGO-disabled focused tests0.875s, vet/module
verification, host and Linux amd64/arm64 cross-builds. Cross-builds are not Linux
execution. The earlier timed header-fuzz deadline failure remains recorded at its
original milestone; no new complete-parser safety claim follows from this fix.
