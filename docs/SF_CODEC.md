# SF codecs, gzip transport and large provenance

## Implemented behavior

The canonical representation is unchanged ProseMirror JSON with profile
`manydoc-prosemirror/1`. `.sf` output is now compact JSON; readers continue to
accept indented JSON. Field ordering and indentation are not document semantics.
`WriteStructured` still checks the complete supported graph before emitting data.

`SFGzip` (`"sf.gz"`) is that same JSON in a standard gzip stream. `FormatFromName`
recognizes `.sf.gz`, including uppercase filenames. `Read`/`Write` support the new
format; CLI `convert` infers it from the extension and `extract -format sf.gz`
selects it explicitly. Plain `ReadStructured` remains a JSON-tree API; callers can
wrap a standard gzip reader or use `Read(..., SFGzip, ...)` and `ToStructured`.
Compression adds no schema variant, opaque document object model or runtime
executable/service. It uses Go's standard compress/gzip package and no new module.

Gzip output has deterministic header metadata. The reader consumes the complete
JSON/gzip stream and rejects checksum failures, truncation, trailing junk and
multiple JSON values. Concatenated members follow standard gzip stream behavior;
they must still decode to exactly one SF value. Unknown SF content and invalid
navigation/asset/structure graphs still reject. Cancellation and output errors,
including short writes and gzip finalization failure, propagate to the caller.
Validation failure emits neither JSON nor a gzip header. CLI failures preserve an
existing destination through its existing atomic-output path.

## Allocation changes and retained costs

The internal SF reader returns its already-validated Document bridge to Read(SF),
so the same source marks/assets are not decoded into a second temporary model.
Write(SF/SFGzip) uses ToStructured's already-validated SF result without another
public WriteStructured bridge validation. Public tree APIs retain their validation.
No admission/schema check was removed from externally supplied SF or documents.

`structured_encode.go` streams body nodes rather than marshaling the whole body
into one additional output buffer. Root attributes and each node's own attributes,
marks and assets are still ordinary JSON values. Notes/running trees held in root
attributes and inline assets can therefore still need large allocations. Parsing,
bridge conversion and the complete high-fidelity tree remain in memory; this is
not a bounded-memory document model. Body streaming trades some encoder work for
avoiding an additional document-sized output buffer.

## Original real-source evidence

The unchanged original MQTT PDF yields 205,161 provenance records and 195,686 text
nodes. No record, glyph position, source order, relationship, asset, diagnostic or
nullable view parameter was dropped to reduce size.

| Representation | Bytes |
| --- | ---: |
| Previous indented SF | 477,613,781 |
| Independently compacted JSON | 168,130,798 |
| New compact JSON payload | 168,131,166 |
| New streaming `.sf.gz` | 8,880,248 |

`scripts/verify_sf_codec.py` independently parses the old JSON and standard gzip
payload using Python's standard library. All normalized JSON values and type
counts match, with SHA-256
`4b9ccc6f060fc45958494e3cfdf85ed3bad4d54076a97e79a153662e7d1d1d1b`.
The source still has 6,075 paragraphs, 294 headings, 90 page-local tables,
756 table rows, 3,009 origin cells, all 325 physical navigation views and 1,245
annotation links. Source fixtures and reference oracles are unchanged.
Actual ProseMirror read/check verified all **205,918 nodes** and their marks/
attributes directly from the compressed export. This is structural/editor-schema
validation, not a new visual-fidelity claim.

The old executable read the indented SF into text in 16.95s; the optimized reader
read the same indented input in 7.54s and compressed SF in 3.93s. All three text
outputs have SHA-256
`3562d99826de3d24b08012e684c9b3a91de382425edd633702688c8949bd3747`.
These are host observations, not latency guarantees; other checks ran during some
new measurements. Darwin's reported peak memory footprint was about 1.59GB for the
indented input and 0.755GB for compressed input. RSS measurements varied and are
retained separately in raw time logs. Full large-document memory/editor costs
remain material despite the much smaller persistence/transport size.

New tests cover compressed semantic round trips across all 51 accepted original
corpus inputs, rich notes/destinations/navigation provenance, byte-identical JSON
payloads between plain/compressed writes, deterministic metadata, strict damaged
streams, cancellation/failure propagation and CLI atomic replacement. Existing
schema/structure/asset/font/navigation oracles still run unchanged. Visual layout
code did not change; no new renderer/parity claim is made.

## Reproduce and continue

```sh
GOWORK=off CGO_ENABLED=0 go build -o bin/manifold ./cmd/manifold
bin/manifold convert -o output/codec/mqtt.sf.gz output/navigation-import/mqtt.sf
bin/manifold convert -o output/codec/recovered.txt output/codec/mqtt.sf.gz
output/qa-venv/bin/python scripts/verify_sf_codec.py output/navigation-import/mqtt.sf output/codec/mqtt.sf.gz --report output/codec/independent.json
NODE_PATH=output/sf-qa/node_modules node --max-old-space-size=8192 scripts/verify_structured.cjs output/codec/mqtt.sf.gz
GOWORK=off go test -run 'TestStructuredCompressed|TestStructuredCodec|TestCLIGzip' -count=1 ./...
GOWORK=off go test -run '^$' -fuzz '^FuzzReadStructuredGzip$' -fuzztime=10s -parallel=2 .
```

Generated files, binaries, metrics and optional QA environments remain ignored.
The full goal is incomplete. Next use ACCEPTANCE.md for a bounded current-state
requirement audit, including real-source rich semantics, producer/visual corpus
qualification and actual resource costs. Compression does not prove those items.

Full checks pass: tests (library 15.129s / CLI 0.371s), race (175.287s / 2.119s),
vet, module verification, host CGO-disabled build/execution, Linux/amd64 and arm64
CGO-disabled cross-builds and diff whitespace check. Ten-second/two-worker gzip and
plain SF fuzz campaigns passed 128,050 / 260 executions. These counts are robustness
evidence, not exhaustive malformed-input coverage. See verification.json for logs.
