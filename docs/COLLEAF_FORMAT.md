# Colleaf native representation: inspected source and integration boundary

## Authoritative source inspected

Colleaf is available through GitHub even though no local checkout was found.
The earlier local-only search did not establish that its schema was unavailable.
Read-only inspection on 2026-11-24 pinned
`artpar/colleaf@5ce2ef507b28b92f5b03443ade87bc0e17f3d8b6`:

- [Native format validator](https://github.com/artpar/colleaf/blob/5ce2ef507b28b92f5b03443ade87bc0e17f3d8b6/src/shared/files/docsFormat.ts).
- [Actual editor mount/save integration](https://github.com/artpar/colleaf/blob/5ce2ef507b28b92f5b03443ade87bc0e17f3d8b6/src/integrations/univer/document.ts).
- [Native-document contract](https://github.com/artpar/colleaf/blob/5ce2ef507b28b92f5b03443ade87bc0e17f3d8b6/docs/native-document-contract.md).
- [Format tests](https://github.com/artpar/colleaf/blob/5ce2ef507b28b92f5b03443ade87bc0e17f3d8b6/src/features/docs/format.test.ts),
  package.json/package-lock.json and src/daptin/documents.ts.

No Colleaf or Daptin code was modified, no application/backend calls or messages were sent,
and no app/customer documents were uploaded. Retrieved source is ignored QA
material under output/codec. The library did not acquire an editor dependency.

## Actual native contract

The extension is `.vdocs`; the exact MIME is
`application/vnd.vibedada.docs+json;version=3`. The envelope identifies
`type: vibedada-document`, `version: 3`, `engine: univer`,
`engineVersion: 1.0.4`; its document property is the complete IDocumentData
snapshot returned by Univer's document save API. The editor deep-clones that
snapshot into createDocument and persists the corresponding save snapshot.
This is an office-engine document snapshot, not ProseMirror JSON or inferred HTML.

Blank documents initialize an id/title/locale, body.dataStream ending in CR/LF,
textRuns, paragraphs, sectionBreaks and traditional page dimensions/margins.
The validator retains headers/footers, tableSource, drawings and plugin resources;
recognition and loading also require the exact MIME/version/extension. Raw SF
cannot be renamed to .vdocs and accepted as an editable Colleaf document.

The actual app validates a 5 MiB full native snapshot and 2 MiB individual embedded
images. Its image policy accepts embedded PNG/JPEG/GIF/WebP and rejects transient
blob URLs and remote/vendor-only sources. Those are Colleaf deployment/application
rules, not new manydoc quotas or performer permission gates. Its current conditional
Daptin save/conflict/recovery protocol remains owned by the application. Standard
SF gzip transport does not bypass or implement that native save contract.

## Matching published schema examined

Inspected the Apache-2.0 `@univerjs/core@1.0.4` published declaration
`lib/types/types/interfaces/i-document-data.d.ts`. The npm distribution's SHA-512
integrity was verified before reading it:
`ofbW7Pt/+YW1IOnhjWao4CHz36AMlaWG24fQZOk0Lc+UT3vyefWvHL1U3wiup/daVibut6nVYwJkNtoxgoxB5w==`.
The declaration SHA-256 is
`1bc2354b7aa5114ccf6b5df90a22aca743ba9f034d4aef5f3e5aac11f2299be5`.
Source: [published npm release](https://www.npmjs.com/package/@univerjs/core/v/1.0.4).
No implementation was translated or incorporated into production.

The representation has a flat body data stream with indexed paragraphs, text
runs and section boundaries; tables, custom ranges/blocks and related source maps
supply richer structure. Text-run st/ed boundaries are inclusive/exclusive;
custom-range endIndex is inclusive. Layout offsets are UTF-16 positions, which
must not be confused with Go byte/rune offsets or inclusive range ends. Snapshot
source tables cover lists, drawings, headers, footers and notes. Published types
include editable note bodies/settings and OOXML-compatible styles/raw-preservation
fields; this is a substantially richer contract than the small blank initializer.

## SF choice and actual interoperability work

ProseMirror remains manydoc's selected canonical logical document format. Its
explicit child trees and marks support structural Go manipulation and rich table/
list ownership without maintaining a parallel flat-offset/control-token protocol.
The library profile also represents extracted PDF provenance and uncertainty.
Univer is an established, stronger candidate than the earlier incomplete local
search could assess; its native snapshot closely matches Colleaf's editing needs.
Its flat offsets, engine/plugin resources and version-specific office fields are
important integration costs and must be handled by an explicit checked adapter.
Using either representation does not require executing its associated editor in
production; the choice is about the data contract, not a runtime restriction.

The actual Colleaf boundary is therefore **not implemented or qualified yet**.
A standalone manydoc native adapter should map IDocumentData through the shared
SF, covering actual rich fields rather than only wrapping plain text:

| Native component | Required adapter evidence |
| --- | --- |
| dataStream/textRuns/paragraphs | UTF-16 positions, terminators, formatting, headings, breaks, Unicode boundary tests |
| lists/tables/tableSource | item/cell ownership, nested structures, widths, merged cells and source identities |
| drawings/customBlocks/resources | embedded bytes, sizes/placement and durable relationships |
| documentStyle/sectionBreaks/headers/footers | geometry, inheritance, first/even variants, story ownership |
| notes/customRanges | identity, body/reference membership, markers and hyperlinks with correct range-end rules |
| styles/settings/raw office fields/plugins | preserve supported meanings; explicitly diagnose or reject unrepresented content |
| native envelope | exact pinned identity and actual Colleaf-validator/Univer-reader checks |

No adapter, .vdocs API, app save/load or deployment compatibility is claimed by
the existing four SF conversion paths. Do not change Colleaf/Daptin for this
standalone work. Application authorization, quotas and persistence remain outside
manydoc. This gap stays visible in the acceptance audit; source inspection alone
is not interoperability completion.

## Executable reader and internal stream foundation (2026-11-24)

The semantic `.vdocs`↔SF adapter and public CLI format are **still incomplete**.
The first implementation layer is private: colleaf_stream.go and colleaf_body.go
validate UTF-16 scalar boundaries, native paragraph/section/table grammar and the
positions of styles, annotations, custom blocks and soft page-break records. This
is a prerequisite, not a plain-text adapter or a claim of rich native conversion.
Styles/properties remain raw until the bridge resolves defaults and diagnostics;
root/header/footer/note/source/drawing/resource relationships still need mapping.

The actual pinned runtime corrected a material declaration-comment error:

| Native control | Executable DataStreamTreeTokenType 1.0.4 |
| --- | --- |
| Table start/row start/cell start/cell end |0x1A /0x1B /0x1C /0x1D |
| Table row end/table end |0x0E /0x0F |
| Custom annotation start/end |0x1F /0x1E |
| Paragraph/section/custom block |CR /LF /BACKSPACE |

The prose comments in i-document-data.d.ts incorrectly list0x1E/0x1F for table
row/end; those bytes are annotation controls in the actual runtime. Using the
comments as the implementation would corrupt table ownership. Each table cell
contains its own paragraph/section CR/LF terminator. Omitting that LF produced
negative/empty cell bounds in the independent parser; it is not a valid profile.
A table's endIndex is exclusive after TABLE_END. Runtime tree-node endIndex is
inclusive, so the QA normalizer adds one before comparing with Go's internal tree.
Paragraph metadata startIndex points at CR, section metadata at LF; these names
must not be interpreted as the first text character's offset.

The executable reader also accepts unsorted/overlapping text-run intervals and
empty insertion-style records. The last matching whole record wins, without a
union of earlier marks. The foundation checks scalar/range bounds without falsely
rejecting those valid inputs; semantic mapping still needs to project the actual
winning intervals and diagnose unsupported insertion-style editing intent.

Text-run st/ed remains inclusive/exclusive; custom-range startIndex/endIndex is
inclusive/inclusive. In A😀B the emoji occupies UTF-16 units1 and2, even though it
is one scalar/four UTF-8 bytes. The mapper rejects partial surrogate slices,
invalid UTF-8, reversed/out-of-bounds extents, conflicting style intervals and
metadata pointing at the wrong control. Original combining strings are not NFC-
normalized before interpreting positions. The index is shared by token/metadata
validation; it is not rebuilt for every range. Context cancellation and the
library's existing64-level nesting boundary apply; app document/image quotas do
not become library permission or admission gates.

Layout geometry uses96-DPI pixels (72pt=96px), while native text style fs is already
points. The executable engine conversion APIs and declaration both confirm that
difference. It must not be implemented as one global scaling of every number.

The hand-authored testdata/colleaf/native-stream.json is a protocol reference
fixture, not output from manydoc. It contains astral styled/link text, a real note
anchor/body and nested tables with actual source records. The Go tree is compared
node-for-node/extent-for-extent with @univerjs/engine-render 1.0.4's
parseDataStreamToTree. Actual DocumentDataModel/DocumentViewModel readers verify
style, inclusive link range, note body/anchor, table lookup/source cell counts and
unchanged stream text. Colleaf's unmodified pinned validator accepts the envelope.
Its source SHA256 is
c54b27b8c303713f5d5d5dfdb7cd54ec9b8410f5045613150861855b11c899c0.
The reference fixture SHA256 is recorded in verification.json.

This proves scalar/token/metadata prerequisites, not merged-cell rendering, asset
loading, source-map fidelity, list/section/story ownership or native edit/save
semantics. Those remain necessary before the adapter can be called complete.
Do not expose a partial CLI conversion or store the whole SF as an opaque private
payload and call that interoperability. Native visible text/styles/relationships/
assets must be real, independently editable fields. Any extension for provenance
or otherwise unrepresented semantics must avoid duplicated visible-text authority
and must be checked against actual native records/edits.

Optional QA reproduction (no runtime JavaScript or editor service):

```sh
npm install --prefix output/colleaf-qa --ignore-scripts --no-audit --no-fund @univerjs/core@1.0.4 @univerjs/docs@1.0.4 rxjs@7.8.2
# Retain the unmodified pinned docsFormat.ts as output/codec/colleaf-format-0.txt:
curl -fL -o output/codec/colleaf-format-0.txt https://raw.githubusercontent.com/artpar/colleaf/5ce2ef507b28b92f5b03443ade87bc0e17f3d8b6/src/shared/files/docsFormat.ts
MANYDOC_COLLEAF_QA_TREE=output/colleaf-qa/go-tree.json GOWORK=off go test -run 'TestColleaf' -count=1 .
node scripts/verify_colleaf_stream.cjs
GOWORK=off go test -race -run 'TestColleaf' -count=1 .
GOWORK=off go test -run '^$' -fuzz '^FuzzColleafStream$' -fuzztime=10s -parallel=2 .
```

The QA script checks exact package versions and source hash before executing the
actual validator. Node's TypeScript stripping emits its experimental warning on
this host. An investigative @univerjs/docs-exchange 1.0.4 install returned404; that
package is not required for the independent core/renderer readers or Go bridge,
and no external blocker exists. No implementation code was copied/translated from
Univer; the adapter is authored from format declarations and executable API probes.
Next implement both semantic mappings through SF, with rich fixtures and actual
reader/edit/save qualification, keeping the application/backend untouched.
