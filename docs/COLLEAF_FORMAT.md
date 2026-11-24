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
