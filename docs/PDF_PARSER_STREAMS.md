# PDF parser-stream integrity and ownership

The library now verifies retained xref/object streams immediately after
`pdfcpu.ReadWithContext`, before page-count, metadata and content access. This
closes successful acceptance of corrupt internal compressed data and inconsistent
compressed-object identities. It does not bound allocations already made inside
the dependency's initial parser. The full goal remains active/incomplete.

## Boundary and implementation

`pdf_parser_streams.go` checks original Raw data with the shared native decoder,
clearing cached Content on a local copy so a lenient prior decode cannot bypass
checksum validation. Every retained internal stream gets the64MiB stage bound and
charges the existing shared256MiB decoded-work budget. Already cached decoded
content must agree. Syntax/image and parser validation use one budget; it is not
reset before page extraction. The validator neither rewrites input nor mutates
parsed objects, and adds no application quotas or permission checks.

Object-stream prologs must contain exactly N integer pairs within First. Identities
are positive/unique, offsets increase within the actual decoded body, and each
active compressed xref index must name that exact header identity. Incremental
updates may replace an old compressed member with an ordinary object; historical
members are not required to remain active in the latest xref map. PDF whitespace,
including NUL, and trailing header comments retain their source semantics.
Cancellation is observed while visiting streams and scanning long prologs.

Published basis: ISO32000-1 sections7.5.7/7.5.8
([Adobe specification](https://udp.adobe.io/document-services/docs/assets/35e4369068f86065372c18787171a17e/PDF_ISO_32000-1.pdf)).
Object-stream offsets increase; member object numbers need not be sorted.
This implementation is independently authored around existing open-source types
and codecs. No proprietary UniDoc code or dependency fork was introduced.

## Evidence

Hand-authored source syntax fixes the original object IDs, compressed indexes,
payload bytes, physical Courier geometry and expected SF. Seven raw/filtered
variants (raw/Flate/ASCII85/LZW EarlyChange0/1/PNG/TIFF), two header-whitespace
variants and an incremental Courier-Bold replacement pass. Full Document values
survive SF serialization; source identity/formatting/positions are checked, not
only plain text. The seven codec Documents and nine non-incremental CLI SF graphs
agree completely. This short LZW corpus does not establish all dependency-parser
LZW transitions; the separate syntax-stream transition oracle remains unchanged.

The exact pre-change73fb39e Git-archive CLI accepted all nine negative sources:
corrupt xref/object-stream zlib checksums, wrong N in both directions, wrong/zero/
duplicate member identities, vertical-tab separators, and a67,108,936-byte lazy
object stream. Current reads reject atomically. Independent Python zlib confirms
corrupt checksums and counts the oversized expansion in small chunks. The CLI
must preserve an existing destination for every negative source.

Optional pypdf7.1.1 and MuPDF1.28.2 check original payload/prolog/xref relationships,
actual text origins and font replacement, and complete CLI SF equality. Original
publication structure/content/DOCX and nine-page PDF reflow checks pass unchanged.
Complete publication SF is byte-identical to73fb39e, SHA256
`2d630c8ff72ec594b95b67542d66bf2df13886380672f6a213e5713b8095a7b2`;
original MQTT complete SF is also identical,
`14742c55e8295feb0058a74cc33afe394f1ac340fa82aa422af3ad325524f526`.
There are no output-layout changes or new general fidelity claims.

## Precise remaining initial-parser limit

The dependency's `read.go` calls `saveDecodedStreamContent(nil, ...)` while loading
xref streams, and `parseObjectStream` calls `DecodeLength(First)` for object
prologs before returning the context. Some filters/pipelines decode fully even
for DecodeLength. Root/version access may also resolve a lazy compressed object
inside ReadWithContext. Its public configuration exposes no decoder-budget hook.
Consequently this validator is an integrity/work check after initial loading, not
protection against all earlier expansion, object allocation or source-file size.
No upstream/module-cache/vendor patch or executable workaround was adopted.
This remains an audit/engineering concern, not an overall external blocker.

## Newly reproduced encrypted-export failure: next compatibility fix

MuPDF exports with object streams and a direct Encrypt dictionary fail in both
73fb39e and the current reader, before the new validator. RC4-40, RC4-128, AES-128
and AES-256 sources all decrypt/extract correctly in independent pypdf/MuPDF using
the supplied reader password. Their original source dictionaries and strict success
oracle remain in the verifier. They use only owned synthetic fixture content.

The dependency's `parseTrailer` records Encrypt only via IndirectRefEntry. For
these direct dictionaries it never initializes encryption and attempts to read
ciphertext as Flate, yielding `decodeObjectStreamObjects ... zlib: invalid header`.
The optional strict `--require-encrypted` command currently FAILS on aes128.
This is a manydoc/dependency compatibility defect, not an independent-reader
limitation or a completed unsupported-feature exception. Existing encrypted corpus
checks cover other shapes; they do not establish this missing combination.

Next investigate a pure-Go adaptation for direct encryption dictionaries that
preserves original object numbers, ciphertext, file IDs and actual document
semantics. Do not bypass passwords, introduce performer permission gates, patch
vendor/application code or substitute decrypted text from an external tool.
Keep the original strict source/relationship/position oracle during the fix. The
generator preserves existing encrypted files; exact failing source SHA256 values
are in verification.json. A successful fix must also retain the complete control
SF, not merely return a readable string or exit successfully.

## Reproduce and continue

```sh
GOWORK=off MANYDOC_PDF_PARSER_QA=output/pdf-parser-qa go test -run 'TestPDFParser|TestPDFObjectStream' -count=1 .
GOWORK=off CGO_ENABLED=0 go build -o bin/manydoc ./cmd/manydoc
output/qa-venv/bin/python scripts/verify_pdf_parser_streams.py output/pdf-parser-qa --large --generate-encrypted
# Current library compatibility failure; strict original success oracle:
output/qa-venv/bin/python scripts/verify_pdf_parser_streams.py output/pdf-parser-qa --require-encrypted
GOWORK=off go test -run '^$' -fuzz '^FuzzPDFObjectStreamHeader$' -fuzztime=100000x -parallel=2 .
GOWORK=off go test ./...
GOWORK=off go test -race ./...
GOWORK=off go vet ./...
GOWORK=off CGO_ENABLED=0 go build -o output/manydoc ./cmd/manydoc
git diff --check
```

Current production passes full tests (library28.161s, CLI cached), full race
(library344.060s, CLI2.798s), vet/module verification and CGO-disabled
host/Linux amd64/arm64 builds; cross-builds are not Linux execution. Final focused
race1.550s includes later test-only prolog/predictor/incremental/fuzz additions.
The first timed10s header-fuzz campaign reported `context deadline exceeded` after
110,806 executions with no recorded crashing input. The same unchanged fuzzer
then passed a100,000-execution campaign. Both outcomes remain in verification.json;
this is a short header-parser sample, not exhaustive PDF-parser safety.

Python/MuPDF remain optional QA only. Check STATUS/ACCEPTANCE and the ledger before
continuing; the remaining original-goal semantic/native/API/visual matrix is still
required in addition to these scoped integrity checks.
