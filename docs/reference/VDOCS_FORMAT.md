# VDOCS: optional Univer snapshot interchange

SF is Manifold's canonical rich-document representation. All DOCX/PDF paths share
that model. VDOCS is an optional interchange container for a pinned Univer native
snapshot; it is not a second canonical document model or a required editor service.

## Container and API

The `.vdocs` JSON container is owned by Manifold:

```json
{
  "type": "manifold-document",
  "version": 3,
  "engine": "univer",
  "engineVersion": "1.0.4",
  "document": {"id": "example", "body": {"dataStream": "Hello\r\n"}}
}
```

MIME: `application/vnd.manifold.docs+json;version=3`.
`ReadVDOCS` / `WriteVDOCS`, `Read` / `Write` with `VDOCS`, and the CLI extension
map the checked native snapshot through the shared typed model and SF profile.
The reader validates the complete supported container/native graph. Renaming SF
JSON to `.vdocs` or supplying an unwrapped snapshot does not satisfy that contract.

This container is a Manifold convention, not an official Univer file standard.
The document payload follows the published Univer 1.0.4 IDocumentData family.
Primary references are the [Univer source](https://github.com/dream-num/univer),
[published core package](https://www.npmjs.com/package/@univerjs/core/v/1.0.4) and
[published document package](https://www.npmjs.com/package/@univerjs/docs/v/1.0.4).
Production parsing and conversion are independently authored Go; running Univer
or JavaScript is not required.

## Checked semantics

The adapter interprets UTF-16 offsets/control-token structure, text styles and
languages, paragraphs, lists/counters/literal labels, tables/merged cells, embedded
images/drawing ownership, internal/external navigation, sections/stories and rich
foot/endnotes. Indexed records and owner identities are validated together;
nonvisual intent metadata cannot replace the actual native content graph.

Snapshot edits/saves can change current style, positions or relationships. Stale
or contradictory preservation metadata diagnoses or rejects; it does not override
actual native content. Supported native/SF round trips retain their declared
structure, relationships, assets and source history. Unknown or unrepresentable
mappings have explicit outcomes. No application admission/permission gates exist.

## Limits

This is a checked subset of a pinned snapshot schema. It does not promise complete
editor UI, browser layout, every plugin/resource or native rendering parity.
Forced complex-script/direction mappings and unsupported native output reject
explicitly. Documented SDK rendering differences do not alter SF/PDF support.

Optional QA uses the actual pinned Univer core/view/tree/plugins/facade APIs for
snapshot inspection, edits, glyph/geometry and saves. Host-application validators,
source downloads and mounts have been removed. Their historical acceptance claims
are retired; current qualification is against the native SDK and Go conversion
contract. SDK packages remain ignored QA tools, never runtime dependencies.

## Branding change and migration

Application-specific container labels and diagnostics were removed at the user's
request. The strict reader now expects `manifold-document`; its MIME and native
loss codes use Manifold/VDOCS names. The SF schema and embedded `manydoc.*` metadata
profiles remain unchanged. A file with a retired container label requires an
explicit header migration; the original `document` payload must remain unchanged
and will still be fully validated. No legacy-name aliases are emitted or accepted.

The authored fixture in `testdata/univer/native-stream.json` changes only its
container label; its native `document` payload is unchanged. This is an intentional
format identity change, not a weakened content/structure oracle.

```sh
GOWORK=off go run ./examples/vdocs -out output/vdocs-example
bin/manifold extract -format sf -o output/vdocs-example/reopened.sf output/vdocs-example/document.vdocs
GOWORK=off go test -run 'TestVDOCS|FuzzVDOCS' ./internal/engine
```
