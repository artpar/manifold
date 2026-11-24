# Colleaf native representation: inspected source and integration boundary

## Current persistence correction

The current transport uses a native CUSTOM range, not unregistered manydoc.*
plugin resources. See the final facade-save section below. Earlier sections
record the original private worker milestones; their raw-model save evidence did
not qualify the actual FDocument.save API. The current output retains their
checked intent records inside one carrier and reads the old transport as input.

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


## Private text/style semantic mapping and native edit/save qualification

The next private layer (colleaf_styles.go/colleaf_text.go) covers paragraphs,
heading1–6 (outlineLevel5 for heading6), direction/page-break-before, authored
language, font family/point size, bold/italic/underline/strike/color, hyperlinks
and VT soft line breaks. The pinned ITextStyle declares lang and altLang; lang
maps to the general SF binding, while alternate/script-specific intent remains
unmapped. Empty paragraphs and astral UTF-16 bounds survive the SF hop. A native
last-whole-record interval sweep is checked against a deterministic naive oracle
on300 interval sets; named basedOn resolution rejects undefined/cyclic styles.
Named/default/direct inheritance has Go semantic tests; full renderer-effective
inheritance is not visually qualified yet.

Optional SDK QA independently reads generated native records, uses published
TextX operations to replace the astral heading with Edited😀/italic, resets and
saves a DocumentDataModel, and Go re-imports that snapshot. The new text/style
wins; subsequent link metadata survives. This is a real reader/mutation/save
check, not browser editor or rendered-layout qualification. No full-SF text cache
is present. The test also checks native→SF→native records, including normalized
internal SF href plus matching destination. The missing internal target in this
isolated text fixture is explicitly declared for the SF validation hop.

```sh
MANYDOC_COLLEAF_TEXT_QA=output/colleaf-qa/text.json GOWORK=off go test -run TestColleafTextSemanticRoundTripAndNativeEdits -count=1 .
node scripts/verify_colleaf_text.cjs
MANYDOC_COLLEAF_TEXT_EDIT=output/colleaf-qa/text-edited.json GOWORK=off go test -run TestColleaf -count=1 .
```

Unknown presentation properties report explicit diagnostics. Hidden text rejects
until visibility semantics are mapped; unsupported containers/annotations or
multiple sections reject before partial content can escape. These are internal
workers, not a new public format contract. Lists/merges/images/resources/notes,
stories/section settings, provenance, checked sparse intent metadata, strict
JSON/envelope/surrogate decoding and broader native edit/render checks remain
required. Next extend body/container ownership and validate native list/table/note
sources using actual SDK APIs before exposing any .vdocs API/CLI.


## Private merged/nested table mapping and executable geometry

colleaf_tables.go now composes paragraphs and tables through shared SF. Native
source row/cell ordinals must exactly match stream tokens. SF omits covered
origins; native vertical continuations instead need rowSpan0/positive-columnSpan
records plus empty CR/LF cell bodies. Combined2×2 merges therefore have an empty
rowSpan0/columnSpan2 continuation in the next row. Omitting this record shifted
its following cell to x0 rather than x240 in the pinned reader. Optional empty
horizontal zero/zero records are checked against an owning span. Any covered
cell carrying visible text or a nested container rejects; it is never discarded.
A bounded occupancy grid catches overlaps, out-of-bounds/unowned spans, partial
rows, token/source count disagreements, identity mismatches and orphan sources.
Repeated headers retain their source flag and SF contiguous-prefix rules.

Nested cells retain full stream/source/style/link offsets and globally distinct
IDs. The reader reuses a global effective-style interval partition and indexed
range intersections for paragraph fragments. Bare anchoring CRs are structural;
authored text before/after a native table becomes ordered SF blocks with a loss
diagnostic for the single-paragraph grouping. Extra table/row/cell presentation
has explicit diagnostics. Fixed widths preserve point intent via native pixels.
Equal full-width SF defaults use specified percentage columns/100% table width:
zero unspecified native widths yielded zero/invisible table geometry.

The pinned table skeleton treats both legacy unit0 (declared POINT) and unit3
PIXEL dimension values as pixels. A unit0 table with the same numeric widths as
unit3 retained width432px; reducing numbers by3/4 instead shrank it to324px. Import
therefore maps both to SF points by3/4. This field-specific measured behavior must
not change fs, which remains points. Percentages resolve against available width.
Unequal relative/mixed auto/fixed dimensions still need the full layout-context
mapping; private workers reject these rather than silently inventing dimensions.

scripts/verify_colleaf_tables.cjs uses actual parseDataStreamToTree,
DocumentDataModel/DocumentViewModel and DocumentSkeleton APIs. The hash-checked
unmodified pinned Colleaf validator accepts generated and edited envelopes.
The script verifies every
row/cell count and exclusive extent, combined merge sources, following-cell x240,
432px outer width, nested skeTables, visible equal-default table widths and legacy
unit behavior. It then replaces cell C with Z via TextX, saves, and Go re-imports
the change while retaining merge ownership. skeTables is a Map; the earlier plain
JSON dump of it concealed existing table entries. These checks establish record/
geometry/mutation semantics, not canvas font metrics, browser editor usability or
visual layout fidelity. No SDK implementation code is copied or translated.

```sh
MANYDOC_COLLEAF_TABLE_QA=output/colleaf-qa/tables.json GOWORK=off go test -run TestColleafMergedNestedTableSFRoundTrip -count=1 .
node scripts/verify_colleaf_tables.cjs
MANYDOC_COLLEAF_TABLE_EDIT=output/colleaf-qa/tables-edited.json GOWORK=off go test -run TestColleaf -count=1 .
GOWORK=off go test -run '^$' -fuzz '^FuzzColleafTables$' -fuzztime=10s -parallel=2 .
```

The full adapter remains private/incomplete. Required lists, notes/images/resources,
section/story settings, source/intent mapping, strict envelope/JSON/surrogates and
broad native rich editing/rendering still need completion. Private unsupported
containers/relative sizing/ragged missing slots are unfinished work, not new public
support exceptions. Public API/CLI and production dependencies remain unchanged.


## Private foot/endnote graph mapping and sparse identity resource

colleaf_notes.go now composes main and note body mappings around actual native
customBlocks/notes/tableSource. Styled citations occupy one UTF-16 BACKSPACE;
following text, astral scalars, repeated citations and table-cell references keep
exact offsets. Real note bodies support the text/heading/link/table workers,
including merged/nested cells. Shared native customMark maps to SF Note.Label;
showBodyReference=false maps to an explicit empty BodyLabel. A matching leading
native body marker maps to BodyLabelStyle; nonleading/self/foreign references
still reject as recursive/contradictory graph content.

SF foot/endnote identities are kind-scoped and definition order is meaningful.
The internal resource manydoc.note-identities.v1 contains only a bindings array of
nativeId/kind/id records in definition order. It stores no visible text, marks or
body cache. Bindings are checked against actual native identity/kind. Native
identity edits invalidate the stale binding with a diagnostic and keep the actual
native ID/kind; native body/mark edits remain authoritative without consulting the
resource. Missing resources leave native IDs usable in deterministic order.
Duplicate resources/bindings, definition aliases, dangling/mismatched references
and contradictory metadata reject before partial content escapes.

Go tests compare the full basic note document and native records through SF,
including one identical SF ID used by a footnote and endnote, repeated citations,
point-size/style/language on citation atoms, custom § mark, markerless body and a
merged table inside the definition. scripts/verify_colleaf_notes.cjs checks exact
pinned package versions and source hash, executes the unmodified Colleaf validator
on generated/saved envelopes, reads the real note views/references/styles/table
sources and uses TextX to change Foot body→Edit body plus §→†. Go re-imports the
saved native fields and retained identities. The edit creates a legitimate text-run
boundary; the check compares combined text and retained graph rather than assuming
one original run after a native edit.

An exploratory bare headless DocumentSkeleton probe produced no generated note
placement/marker glyph evidence. It does not qualify native note rendering; actual
browser/plugin/font/layout integration remains a required later check. No source
oracle was changed or native layout fidelity claimed from metadata alone.

```sh
MANYDOC_COLLEAF_NOTE_QA=output/colleaf-qa/notes.json GOWORK=off go test -run TestColleafNoteIdentityReferenceBodyAndSFRoundTrip -count=1 .
node scripts/verify_colleaf_notes.cjs
MANYDOC_COLLEAF_NOTE_EDIT=output/colleaf-qa/notes-edited.json GOWORK=off go test -run TestColleaf -count=1 .
GOWORK=off go test -run '^$' -fuzz '^FuzzColleafNotes$' -fuzztime=10s -parallel=2 .
```

These workers remain private/incomplete. Independent literal/rich citation and body
marks/styles, explicit empty native custom marks, inferred/provenance/navigation
intent, marker-style inheritance and note numbering/placement settings still need
checked mappings. Lists/drawings/assets in notes depend on the remaining shared
body work. These are unfinished work, not new public support exceptions. The
public format/API/CLI, production dependencies and application boundaries remain
unchanged. Next complete rich label/intent and list/asset/section/story mappings,
strict native envelope/JSON/surrogate decoding and native browser qualification.


## Private list sources, counter semantics and checked item ownership

colleaf_lists.go now composes real native list sources and paragraph bullets for
main/note/table-cell stories. Native startNumber is zero-based (0→1,4→5,-1→0),
while SF stores actual starts. Parent groups share counters for nested compound
formats; independent lists get separate groups and fallback ancestors. Canonical
parent contexts resolve exactly as ToStructured on copied containers. Adjacent
explicit SF lists retain their boundaries. Note definitions can now contain lists.

Native bullet records do not own continuation paragraphs/tables. The resource
manydoc.list-ownership.v1 contains per-story list type/group and first/last paragraph
IDs, item bounds and implicit/independent flags. It contains no visible item text,
styles or complete SF payload. A binding applies only while actual paragraph IDs,
bullet type/group/level, item coverage and counter progression agree. Native text
edits remain real fields; native restart/group edits invalidate old grouping. Stale
or absent bindings produce disclosed bullet/nesting inference; unmarked text/tables
remain visible in native order. Ambiguous continuation ownership is not invented.
Maps/interval interpretation are shared across cells rather than rescanning every
source/binding per paragraph. The narrower old body helper rejects list content
if the caller would fail to transport its sources/ownership resource.

Optional scripts/verify_colleaf_lists.cjs checks the pinned app validator and actual
renderer LIST glyphs for Appendix AA./BB., AA.iv)/AA.v), independent8.1, restarted
2./3., bullet• and decimal0., plus0./1. in a note story. A native isLegal edit
changes compound Roman labels to27.4)/27.5), independently checked too. It then changes Parent one
to Edited one via TextX and saves; paragraph IDs, resources, marker glyphs and owned
continuation/table structure survive Go re-import. Counter-cache-only inspection
initially omitted older nested markers because that history resets; actual glyphs
provide the independent marker evidence. Go also checks SF/native record equality,
stale native restarts, foreign inference, templates and malformed source handling.

The original list milestone identified two additional differences. A literal percent
in glyphFormat does not follow Go/OOXML template semantics (%%1. displayed11.
instead of%1.). An undefined deeper placeholder such as%9 at level0 throws while
SF ignores it. Export projects ignored placeholders out, stores sparse original-
template intent only for that projection, and restores it only while projection
matches actual glyphFormat. Native format edits remain authoritative. Literal
percent/digit-suffix output is now implemented and qualified in the current list
projection section below. Native zero in
DECIMAL_ZERO displayed00; letter/Roman zero displayed an empty symbol, while SF
currently displays0. The original private milestone rejected those mismatches. The current zero/legal
projection section below now replaces that rejection and qualifies both directions
with explicit foreign-policy diagnostics. Decimal zero continues to work.

```sh
MANYDOC_COLLEAF_LIST_QA=output/colleaf-qa/lists.json GOWORK=off go test -run TestColleafRichListOwnershipNumberingAndSF -count=1 .
node scripts/verify_colleaf_lists.cjs
MANYDOC_COLLEAF_LIST_EDIT=output/colleaf-qa/lists-edited.json GOWORK=off go test -run TestColleaf -count=1 .
GOWORK=off go test -run '^$' -fuzz '^FuzzColleafLists$' -fuzztime=10s -parallel=2 .
```

No full browser/font/visual layout or unknown-resource persistence through the
actual full editor save is qualified yet. Remaining native work includes those
list policies/marker presentation/picture bullets, rich note label/style intent,
drawings/assets/resources, section/story/page metadata and strict envelope/JSON/
surrogate handling. No public native format/API/CLI or runtime dependency was added.


## Private embedded PNG/JPEG assets and checked geometry intent

colleaf_images.go now composes actual blockType0 drawing atoms, drawingType0 image
sources with imageSourceType:BASE64, data URIs, descriptions, drawingsOrder and
inline docTransform boxes. Sizes are pixels, independently confirmed by the
published FDocumentImage.getSize contract and executable skeleton. SF preferred
point widths/heights project by4/3; omitted dimensions use the intrinsic ratio.
The shared typed body composition transports all maps/intent together. Old narrow
helpers reject image content rather than losing drawing sources.

Pinned published @univerjs/docs-drawing and @univerjs/drawing1.0.4 declarations
establish IDocImage/IImageData fields and note-aware persisted source ownership.
The former's tarball is retrieved from the primary npm registry; SHA256
ca7bbeb97e1888296371b323f27017411e88618b16e0d2fb9950e0c0c19df0a3.
Optional QA installs both beside the existing readers (no scripts/runtime modules
in Go). IDs include the story scope: collectDocDrawings has a flat registry, while
findDocDrawing must still return notes/<id>/drawings paths for note sources.
The private composition supplies a native document ID for its real drawing-space
relationships; this is document format identity, not application authorization.

manydoc.image-geometry.v1 stores per-story drawing ID, asset digest and original/
projected dimensions only. It duplicates no bytes/alternative text or complete SF.
A hint applies only if current asset hash/box and recomputed projection all match.
This retains omitted dimensions and fractional point precision. Native asset,
resize and alt-text edits remain authoritative; stale geometry has diagnostics.
Removing hints still recovers actual explicit frame dimensions and native bytes.

Shared Go PNG/JPEG validation rejects corrupt data, media/pixel contradictions and
excessive raster expansion before pixel allocation. Source/cache contradictions,
missing/orphan sources and duplicate order IDs reject. Runtime never fetches URLs,
uses image hosting or applies Colleaf's app5MiB/2MiB quotas. Mixed inline text/images
become ordered blocks with grouping diagnostics; surrounding styles/text and
picture-leading paragraph break-before survive. Inline fragments of one explicitly
marked native paragraph stay in the same list item, even without ownership hints. Hidden image anchors/drawings,
linked images and unrepresented floating/crop/rotation/flip/opacity/shape appearance
reject in these private workers rather than becoming incorrect visible pictures.
Those appearance policies remain unfinished, not new public support exceptions.

Tests cover main/cell/list/note ownership, PNG alpha, the unchanged original MQTT
JPEG (SHA256 fe9737d0cb4f905917054c7938fc3ccf4b83f0623df9cd5d62192dbaae26b510),
all supported implicit/explicit dimension forms, fractional precision through SF,
native asset replacement, malformed relationships/bytes/cache/appearance and
hidden-anchor/break policies. scripts/verify_colleaf_images.cjs executes the
hash-checked pinned app validator and actual collectDocDrawings/findDocDrawing/
DocumentSkeleton APIs. All five occurrences have correct scopes; main/nested-cell/
list/note boxes match actual drawing sizes. A native64×32px resize and description
edit save/re-import as48×24pt/Resized PNG, overriding stale size intent. Optional
Pillow independently decodes three32×16 alpha PNGs with exact color/alpha samples
and two original JPEG byte-identical occurrences. No source fixture/oracle changed.

```sh
npm install --prefix output/colleaf-qa --ignore-scripts --no-audit --no-fund @univerjs/docs-drawing@1.0.4
MANYDOC_COLLEAF_IMAGE_QA=output/colleaf-qa/images.json GOWORK=off go test -run TestColleafEmbeddedImagesAllStoriesAndSF -count=1 .
node scripts/verify_colleaf_images.cjs
output/qa-venv/bin/python scripts/verify_colleaf_images.py
MANYDOC_COLLEAF_IMAGE_EDIT=output/colleaf-qa/images-edited.json GOWORK=off go test -run TestColleaf -count=1 .
GOWORK=off go test -run '^$' -fuzz '^FuzzColleafImages$' -fuzztime=10s -parallel=2 .
```

Actual browser bitmap painting/font/layout and unknown-resource persistence through
full editor save remain unqualified. This layer transports preferred image boxes;
page/cell/note contextual fitting needs the remaining native section/layout context.
Header/footer stories, rich label/marker/list-policy intent and strict native
JSON/envelope/surrogate handling remain required before the public native adapter.
Production modules/API/CLI and application/backend boundaries are unchanged.


## Actual FDocument.save/reopen: corrected intent transport

Read-only application source explicitly calls document.save() (colleaf-1.txt:117).
A pinned headless Univer instance with real Docs/Drawing model plugins demonstrated
that FDocument.save regenerates registered resources and discards unregistered
manydoc.note-identities.v1/list-ownership.v1/image-geometry.v1 entries. The existing
model getSnapshot tests had not exercised this API. No original oracle was changed;
the new regression reproduces the failure before testing the fix.

colleaf_intent.go now seals those same checked intent records in one body.customRanges
entry: rangeType5 (CUSTOM), rangeId manydoc-intent-v1, properties.profile
manydoc.native-intent, version1 and records. The range initially covers the final
LF, outside visible text. It adds no control characters, text, assets or complete
SF cache. Native TextX updates its scalar range during edits; individual records
still apply only after their existing ID/geometry/counter checks. The reader strips
only the recognized carrier, validates version/ranges/record names, rejects
multiple carriers and still accepts the old private direct-resource input.

The registered DOC_DRAWING_PLUGIN contains data/order mirrors. Every mirrored
descriptor must exactly match a real persisted main/note drawing source; IDs/order
must be valid and unique. A checked mirror does not become another authority.
Contradictions or resource-only/orphan sources reject pending their explicit
mapping, rather than silently choosing bytes. Empty DOC_WORD_STYLES_PLUGIN and
DOC_NOTE_PLUGIN resources do not imply content loss. SDK object permission state
is deliberately not interpreted or enforced by this standalone document library.

scripts/verify_colleaf_facade.cjs uses actual Univer, Docs, Drawing and DocsDrawing
plugins plus the public createDocument/FDocument.save APIs, then fresh instances
for reopen/save. Notes/lists/images retain the carrier and all mapped semantics
on Go re-import. The script also reproduces the old drop, executes the pinned
unmodified app validator and inserts text with TextX before saving/reopening;
IDs, continuation ownership and carrier placement survive. The prior reader
scripts now locate intent records in the carrier, with unchanged semantic oracles.

An additional image.setSize facade probe requires an unregistered render-manager
service in this bare headless setup. No fake service was substituted and no image
facade mutation/browser painting qualification is claimed. Those need the actual
render/preset/browser environment. The successful facade save proof closes the
API-level persistence gap; it does not close all editor/UI/rendering work.

```sh
MANYDOC_COLLEAF_FACADE_QA=output/colleaf-qa/facade-cases.json GOWORK=off go test -run TestColleafFacadeSavedIntentSemantics -count=1 .
node scripts/verify_colleaf_facade.cjs
MANYDOC_COLLEAF_FACADE_SAVED=output/colleaf-qa/facade-saved.json MANYDOC_COLLEAF_FACADE_EDITED=output/colleaf-qa/facade-edited.json GOWORK=off go test -run TestColleaf -count=1 .
GOWORK=off go test -run '^$' -fuzz '^FuzzColleafIntent$' -fuzztime=10s -parallel=2 .
```

Next implement native page/section/story metadata and contextual fitting, complete
remaining rich label/list/appearance intent and strict envelope/JSON/surrogates,
then qualify actual full-preset/browser editing/rendering. The adapter remains
private/incomplete. Production dependencies/API/CLI and application/backend code
are unchanged; Node remains optional QA, with no services/uploads/permission gates.


## Private document composition: sections, geometry and running content

colleaf_sections.go now composes the private snapshot through the shared SF. Main
body sections use one scalar index, with closing LF section records, sectionType
continuous/next/even/odd and sectionTypeNext precedence on import. Page dimensions,
per-side margins and header/footer distances project points to pixels. Explicit
zero/fractional values, optional layouts and nil inheritance survive guarded intent;
actual native geometry edits invalidate stale original intent and emit diagnostics.
Traditional native defaults measured 794×1124px with 72px margins; an unpaginated
canvas is not silently turned into a finite page. Unmapped note settings still
reject pending required mapping, rather than being claimed as completed support.

First/even/default headers and footers have distinct IDs and bodies. Nil stories
inherit; explicit empty bodies override inheritance. Original author/subject and
empty title intent persist without a second visible text authority. Native unknown
section/document layout properties produce explicit diagnostics. Contradictory
references, malformed flags/geometry, missing bodies and orphan sources reject.
Authored form feeds map to PageBreak; renderedPageBreaks are soft/inferred layout
boundaries, removed from logical text with colleaf_soft_page_break diagnostics.

The executable renderer ignored an image present only in a header-local catalog.
Header/footer tables, list sources/groups and images now use scoped identities in
the document catalogs, with headerFooterDrawingsOrder. Source selection checks
all relationships and local/global orphan coverage. Notes keep their earlier local
catalog behavior. Automatic hyphenation is explicitly disabled on output because
the facade otherwise inserts enabled defaults; nonzero input remains diagnosed.

scripts/verify_colleaf_sections.cjs checks the unmodified hash-pinned application
validator, Univer 1.0.4 skeleton and actual FDocument.save/fresh reopen. Its fixed
example independently checks seven pages (including odd-page blank insertion),
page sizes/margins, each page's header/footer ID, empty overrides, inherited even
headers, a merged header table, bullet marker and 32×16px drawing box. Go re-import
compares full model semantics and SF/native sources, including fractional sizes,
metadata and nil/empty variants. Separate tests cover foreign/no-intent sections,
native edits, supplementary characters/page breaks and notes in later sections.
This is structural/geometry/save evidence; browser paint/fonts are unqualified.

```sh
MANYDOC_COLLEAF_SECTION_QA=output/colleaf-qa/sections.json GOWORK=off go test -run TestColleaf -count=1 .
node scripts/verify_colleaf_sections.cjs
MANYDOC_COLLEAF_SECTION_SAVED=output/colleaf-qa/sections-saved.json GOWORK=off go test -run TestColleaf -count=1 .
GOWORK=off go test -run '^$' -fuzz '^FuzzColleafDocument$' -fuzztime=10s -parallel=2 .
```

Next: contextual page/cell/note/running-story image fitting; rich note/marker/list/
appearance intent; strict native envelope/JSON/surrogates; then full-preset/browser
editing and rendering. The adapter remains private/incomplete; native API/CLI and
the full goal are not complete. No runtime dependencies or application/backend
changes were introduced. Existing source/oracle files are unchanged.


## Individual frame fitting and actual foot/endnote placement correction

Executable overflow regression: native docTransform.size 1000×2000px stayed that
size on a 320px-wide body/600px-high page. Source dimensions alone do not qualify
placement. colleaf_image_fit.go now projects frame bounds through main section
layouts, fixed/relative merged/nested table grids (native 10px horizontal cell
margins), list continuations, actual cited note sections and inherited running
variants. Shared stories use the narrowest applicable context. Conservative page/
cell/note paragraph/separator allowances bound individual frames. Running bands
use declared margin/distance space when available, otherwise a physical-page
fallback. Complete native story text height/font layout is not measured here.

Original preferred dimensions remain unchanged through SF. Only fitted frames add
a container to the existing checked image geometry record. Import recomputes that
projection and checks original asset digest plus actual box. Native frame/asset
edits win and disclose stale intent. No second image/whole-SF payload is stored.

The actual note layout probe found the earlier custom-block references were only
transported metadata: no native note bodies were placed. Native resolveNoteReferences
reads CustomRangeType.FOOTNOTE8/ENDNOTE9 and createNoteGlyph consumes U+FFFC. The
writer now emits those exact single-atom ranges; indexing rejects bad bounds,
missing definitions, conflicting ownership and unsupported range properties. The
reader separates note ranges from hyperlink ranges and preserves styled references
in cell/paragraph fragments. Earlier private custom-block input remains readable.

Pinned note renderer assertions now inspect real citation glyphs and placed
foot/endnote pages, including a cell reference, instead of only getCustomBlock.
Generated automatic notes use customMark strings with guarded implicit intent:
native repeat counters and lowerRoman endnote defaults would otherwise change SF
shared decimal labels. When native citation order changes, stale automatic intent
is not restored; actual marks and disclosed numbering changes remain. Foreign
implicit note numbering/settings and richer citation/body-label policies still
need their required mappings before exposing the public adapter.

scripts/verify_colleaf_image_fit.cjs independently checks body, nested merged-cell,
list, shared note and inherited running boxes, actual note-image placement,
facade save/fresh reopen and the original overflowing-frame regression. Go checks
full SF equality/preferred sizes and native resize precedence. This is geometry/
structure/save evidence; browser font/paint and total running-story layout remain
unfinished. Production dependencies and application/backend code are unchanged.

```sh
MANYDOC_COLLEAF_NOTE_QA=output/colleaf-qa/notes.json MANYDOC_COLLEAF_FIT_QA=output/colleaf-qa/fitted-images.json GOWORK=off go test -run TestColleaf -count=1 .
node scripts/verify_colleaf_notes.cjs
node scripts/verify_colleaf_image_fit.cjs
MANYDOC_COLLEAF_FIT_SAVED=output/colleaf-qa/fitted-images-saved.json GOWORK=off go test -run TestColleaf -count=1 .
GOWORK=off go test -run '^$' -fuzz '^FuzzColleafNotes$' -fuzztime=10s -parallel=2 .
```


## Foreign automatic numbering: current marks, not invented live policy

colleaf_note_numbering.go now reads typed root noteSettings and root-section
noteProperties. Root defaults merge with each section's explicit properties.
Per-kind counters honor active starts/continuous/eachSect restarts, and custom
marks do not advance them. Default native endnotes are lowerRoman. Modern note
ranges must follow scalar source order; unknown/nullable/malformed settings reject
rather than silently changing counter behavior. Table-cell section sentinels do
not own document note settings.

Current labels map into existing SF fields: first Note.Label, differing per-range
NoteReference.Label, and independent final BodyLabel when the repeated native
note body uses a different mark. Empty labels remain markerless at both sites.
The native sample displays III/AA/IV/V/§/VI, then 00/BB/01, then an empty mark;
the shared body's mark is 01. Generated checked autoLabel behavior stays intact.
colleaf_note_numbering/colleaf_note_settings diagnostics state that live counters,
placement and column policy have been normalized. Source label/body/citation
identities and text survive SF; the policies are not claimed as retained.

Qualified formats: decimal, decimalZero, upper/lowerLetter, upper/lowerRoman,
none, chicago, decimalFullWidth/2, ideographDigital/taiwaneseDigital,
decimalEnclosedCircle, ideographEnclosedCircle, ideographTraditional and
ideographZodiac. The pinned implementation uses parenthesized ideographs for the
name ideographEnclosedCircle; the adapter preserves that actual outcome. Go and
actual renderer checks include zero/overflow-to-decimal symbol boundaries.
Remaining number formats, authoritative automatic eachPage numbering, and editable
separator/continuation/notice bodies remain unfinished and reject explicitly.
All-custom-mark per-page settings can import because displayed labels do not
depend on inferred page counters; policy loss is still diagnosed.

Uncaptured section properties now survive raw marshaling for independent reader
transport. A save/reopen equality failure also corrected foreign default header/
footer distances to native 30px/22.5pt; own generated values are already explicit.
No oracle or source document was changed to accept the earlier wrong default.

scripts/verify_colleaf_note_numbering.cjs checks the pinned app validator, actual
skeleton citation glyphs/placed body markers, fifteen formatter outcomes, native
30px defaults and real facade save/reopen. Go checks full imported SF semantics,
source order/validation, markerless variants and DOCX label round trips. Optional
Word/PDF artifacts use explicit regular/bold DejaVu for the unchanged supplementary
character. scripts/verify_colleaf_note_outputs.py verifies eight note bodies,
variant and markerless Word anchor relationships, nine PDF citation/link boxes,
shared destinations and Unicode through python-docx/XML/pypdf/MuPDF. Four Poppler
pages were visually checked for overlap, clipping and missing glyphs. No native
browser paint, live counter editing or general Word visual parity is claimed.

```sh
MANYDOC_COLLEAF_NUMBER_QA=output/colleaf-qa/note-numbering.json MANYDOC_COLLEAF_NUMBER_OUTPUT=output/colleaf-qa/note-numbering GOWORK=off go test -run TestColleaf -count=1 .
node scripts/verify_colleaf_note_numbering.cjs
MANYDOC_COLLEAF_NUMBER_SAVED=output/colleaf-qa/note-numbering-saved.json GOWORK=off go test -run TestColleaf -count=1 .
output/qa-venv/bin/python scripts/verify_colleaf_note_outputs.py
pdftoppm -r 100 -png output/colleaf-qa/note-numbering.pdf output/colleaf-qa/note-numbering-page
```

Next implement literal/rich citation and independent body-mark output with checked
native ownership, then remaining mappings/strict envelope and public API/CLI;
qualify actual browser/preset behavior. The full adapter and goal stay incomplete.


## Editable independent/rich note-body marks

Private SF→native output now handles BodyLabel literals, BodyLabelContent and
BodyLabelStyle using actual native runs/styles/hyperlinks. The mark precedes a tab
in the first paragraph/heading; table/list/image/empty bodies get a separate leading
paragraph. showBodyReference=false prevents a duplicate generated marker. Empty
styled intent uses native referenceTextStyle without adding a visible tab.

The note story carries one rangeType5 manydoc.note-body-mark annotation, version1,
containing mode/native note ID/paragraph ID/separate/default/empty flags and neutral
render properties. It contains no label text, duplicated styles or body/SF cache.
The root checked identity record names the expected body-mark range, so deleting
that annotation cannot silently change semantics. Literal/rich contents are derived
from real source text intervals; default omission is restored only while actual
label/default-marker values still agree.

The pinned renderer applies blue.600 and underline to ordinary CUSTOM ranges by
default. The new ownership range explicitly uses active=false and textColorMode=text;
scripts/verify_colleaf_body_marks.cjs reproduces the original visible override and
checks actual corrected glyph color/size/bold/italic/underline and hyperlink lookup.
A nonneutral/unknown ownership version rejects rather than silently dropping its
appearance. This remains subject to the required full browser/interaction audit.

Actual TextX replacement moved the annotation start to the tab while inserting a
new mark before it. The stable first-paragraph ID and owned terminal tab define the
current leading marker; recovery uses those bounds and actual runs, not a cached
old value. Rich edits stay rich. Formatting a former literal becomes rich label
content with diagnostics. Enabling the generated marker or removing/changing
ownership preserves visible prefix/body text and discloses stale intent. Separate
paragraphs are removed only while no new ordinary body text remains there.

The fixed six-note fixture covers independent 9, rich linked *2 with different
styles, styled automatic 3, table-first 9, otherwise empty 9, and empty styled intent.
Actual skeleton bodies/citations, neutral annotation rendering, facade save/fresh
reopen, native text/style edits and exact Go/SF/native round trips pass. Browser
paint/font metrics/wrapping and rich per-citation output are not claimed.

```sh
MANYDOC_COLLEAF_BODY_MARK_QA=output/colleaf-qa/body-marks.json GOWORK=off go test -run TestColleaf -count=1 .
node scripts/verify_colleaf_body_marks.cjs
MANYDOC_COLLEAF_BODY_MARK_SAVED=output/colleaf-qa/body-marks-saved.json MANYDOC_COLLEAF_BODY_MARK_EDITED=output/colleaf-qa/body-marks-edited.json GOWORK=off go test -run TestColleaf -count=1 .
GOWORK=off go test -run '^$' -fuzz '^FuzzColleafBodyMarks$' -fuzztime=10s -parallel=2 .
```

Next implement native literal/rich per-citation markers with checked ownership,
then remaining intent/strict envelope and public adapter/browser qualification.
Body provenance/nonvisual style policies remain on the wider mapping queue. No
production dependency, app/backend change or runtime external tool was added.


## Per-citation literal/rich/default/markerless output

colleaf_citation.go now handles NoteReference.Label, LabelContent and Inferred in
native output. Every citation in an affected note group keeps a real U+FFFC note
atom with customMark empty. Real superscript text runs follow that atom, then a
U+2060 delimiter. A neutral manydoc.note-citation version1 CUSTOM annotation binds
profile/token/native note ID/mode/inferred flags to the current source interval.
It contains no label text/style/body copy. Root checked identity data retains the
original shared/automatic label intent; independent/default body marks use real
body-prefix runs through the existing worker. Notes remain shared definitions.

The actual native BaselineOffset.SUPERSCRIPT is 3, checked against pinned glyphs.
Rich atom-wide style is canonicalized into its fragments using the existing SF
rule. Authoritative native text/style edits derive new per-reference values.
Default omission restores only while actual labels still match the current shared
catalog; otherwise explicit values and diagnostics preserve displayed marks.

Actual TextX replaces II with III by splitting its ownership annotation into anchor
and delimiter fragments. Stable tokens and identical properties join that known
case without inventing text. Original fragment IDs/scalar bounds validate first.
Only guarded superscript label intervals normalize their baseline during semantic
recovery. Neutral baseline 0 maps to ordinary text; unsupported nonzero baselines
outside ownership still emit diagnostics. Missing/changed ownership retains real
text and anchors. Invalid versions, appearance, bounds, contradictions or ownership
crossing another atom reject instead of hiding content.

Style-span binary searches preserve last-record-wins precedence without scanning
every source record per citation. Paragraph folding consumes runs once. The fixed
2,048-reference case checks all order/default/ownership semantics; ordinary cases
cover nested cells/lists/sections and foot/endnotes sharing a public ID.

scripts/verify_colleaf_citations.cjs checks the unmodified hash-pinned app validator,
actual seven markerless anchors, visible literal/rich/default/empty labels, red 12pt
bold/green 8pt italic/superscript fields, and two placed note bodies. Real facade
save/fresh reopen plus TextX/style edits pass exact Go/SF semantics. It preserves
the actual split-range edit, not a synthetic direct-text substitute. This is
headless structure/glyph/style/save evidence; full browser paint/font/wrapping,
atomic-label selection/click/navigation and interaction remain unqualified.

```sh
MANYDOC_COLLEAF_CITATION_QA=output/colleaf-qa/citations.json GOWORK=off go test -run TestColleaf -count=1 .
node scripts/verify_colleaf_citations.cjs
MANYDOC_COLLEAF_CITATION_SAVED=output/colleaf-qa/citations-saved.json MANYDOC_COLLEAF_CITATION_EDITED=output/colleaf-qa/citations-edited.json GOWORK=off go test -run TestColleaf -count=1 .
GOWORK=off go test -run '^$' -fuzz '^FuzzColleafCitations$' -fuzztime=1000x -parallel=2 .
```

Next complete remaining list/appearance/navigation/provenance/nonvisual intent,
strict native envelope and public API/CLI, then actual full-preset/browser QA.
Per-page/decorated-note policies and final broader goal audits remain explicit.
No production dependency, external runtime tool or application/backend edit added.


## Current private .vdocs envelope codec and strict JSON qualification

colleaf_codec.go now connects the rich workers to the exact pinned envelope.
readColleaf/writeColleaf remain private; public Read/Write/CLI native dispatch is
still unfinished. The supported envelope is exactly vibedada-document version3,
engine univer, engineVersion1.0.4. Bare snapshots, renamed SF, wrong identity/version,
unknown typed fields, missing id/textRuns/documentStyle, ambiguous decoded keys,
malformed UTF-8 and unpaired surrogate escapes reject before interpretation.
Known typed record case aliases and null scalar offsets also reject. Valid escaped
surrogate pairs and intentional U+FFFD remain actual scalar text. Resource JSON
inside strings, including manydoc carrier records, gets a separate strict pass.
Raw native property objects retain their existing semantic diagnostics/rejection.
The parser depth bound protects its stack; it is not the app's file/image budget.
Input is buffered and container/key-set allocation remains on the resource audit.

The actual FDocument.save output contains settings:{} even when omitted on input.
Known settings support empty/finite positive zoomRatio only; zoom and nondefault
locale disclose editor-view state absent from logical SF. Unknown settings reject.
The writer initializes enUS locale; it does not introduce application persistence,
collaboration, identity, authorization or quota behavior.

scripts/verify_colleaf_envelope.cjs verifies the pinned unmodified validator hash,
the generated .vdocs envelope and two actual facade save/reopens. Go reimports that
unchanged saved envelope with exact rich section/story/table/list/image/metadata
semantics and checks its shared SF bridge. This qualifies structural acceptance
and save transport only. Existing section glyph/page probes remain separate;
actual browser paint/wrapping/selection/navigation, remaining list/appearance/
nonvisual/provenance intent and per-page/decorated-note policies are unfinished.
No full native adapter or overall goal completion is claimed.

```sh
MANYDOC_COLLEAF_ENVELOPE_QA=output/colleaf-qa/sections.vdocs GOWORK=off go test -run 'TestColleafEnvelope|TestStrictJSON' -count=1 .
node scripts/verify_colleaf_envelope.cjs
MANYDOC_COLLEAF_ENVELOPE_SAVED=output/colleaf-qa/sections-saved.vdocs GOWORK=off go test -run 'TestColleafEnvelope|TestStrictJSON' -count=1 .
GOWORK=off go test -run '^$' -fuzz '^FuzzStrictJSON$' -fuzztime=10s -parallel=2 .
GOWORK=off go test -run '^$' -fuzz '^FuzzColleafEnvelope$' -fuzztime=10s -parallel=2 .
```


## Current literal-percent/digit-suffix list projection

The former literal-percent output rejection is replaced by a checked dynamic
projection in colleaf_list_template.go. SF uses single-digit %1..%9 placeholders;
native glyphFormat consumes whole decimal indices and has no literal-percent
escape. Fixed-symbol source entries at index9 onward represent literal fragments,
while real paragraph nestingLevel stays0..8 and counter placeholders stay dynamic.
This is also valid with all nine real levels occupied. Entries are deduplicated
per template and bounded by the existing SF512-byte template contract. Native
format parsing has a bounded expansion allowance; these are representation/stack
protections, not Colleaf's snapshot/image budgets.

Current native format/helpers must agree before original-template intent restores
ignored deeper placeholders or a normalized spelling. Otherwise import derives
current symbolic parts and checks that their SF spelling retains the same literal/
dynamic meaning. Unbound representable helper projections work too. Malformed,
overlong or intrinsically ambiguous templates reject. Native numeric glyphs with
fixed glyphSymbol now import the real constant marker rather than silently
substituting a dynamic number. The current zero/legal section below now qualifies nondecimal-zero and native
legal-decimalZero mapping; broader marker appearance/browser qualification remains open.

scripts/verify_colleaf_literal_lists.cjs checks actual pinned glyphs for twelve
main/cell labels, two note labels and two header labels (using the correct global
running-story list catalog), including percent, digit suffix, static percent text,
compound levels and all-nine-level templates. Actual facade save/reopen twice and
Go/SF import preserve exact supported semantics. A real helper edit from% to‰ and
a parent start edit from2 to5 change parent/child glyphs; SF recovers the edited
template and resolves actual enclosing counters. Header marker content is checked
as an isolated story with its actual global catalog; broader browser paint/preset
controls/interactive list editing are not established by these probes.

```sh
MANYDOC_COLLEAF_LITERAL_LIST_QA=output/colleaf-qa/literal-lists.json GOWORK=off go test -run TestColleafLiteral -count=1 .
node scripts/verify_colleaf_literal_lists.cjs
MANYDOC_COLLEAF_LITERAL_LIST_SAVED=output/colleaf-qa/literal-lists-saved.json MANYDOC_COLLEAF_LITERAL_LIST_EDITED=output/colleaf-qa/literal-lists-edited.json GOWORK=off go test -run TestColleafLiteral -count=1 .
GOWORK=off go test -run '^$' -fuzz '^FuzzColleafListTemplate$' -fuzztime=10s -parallel=2 .
```


## Current checked zero/legal list mapping

colleaf_numbering_projection.go replaces the former nondecimal-zero rejection.
SF's zero/legal convention stays unchanged. Native first-zero items use a decimal
variant, followed by the ordinary type in the same automatic counter group; zero
ancestor fields and legal decimalZero fields also project to decimal. Sparse format/
zero-level intent records only the missing distinction, while item-type aliases
retain one owned SF list with continuations. Running-story global promotion also
renames those aliases and projection keys. No marker, counter or body cache exists.

Current glyph fields, Legal flags and actual counters must agree before intent
restores source formats. Changed native fields win and stale projection/ownership
is diagnosed. Assembly compares every item's effective numbering, so type/counter
agreement cannot hide a changed format. Foreign native00/empty zero labels and
mixed padded legal rules derive explicit current SF templates/effective formats;
groups split where semantics change, with diagnostics for lost live numbering
policy. Intrinsically ambiguous unrepresentable templates still reject explicitly.
Canonical sparse integer member names, bounded format/zero indices and ownership
are checked; stable diagnostics and primary-binding deduplication keep output
order deterministic and avoid alias-driven quadratic validation.

scripts/verify_colleaf_zero_lists.cjs qualifies30 actual main/cell markers,3 note
markers,3 isolated header markers using the correct global catalog, and10 foreign
zero/legal policies against pinned Univer1.0.4. Covered formats are decimalZero,
upper/lowerLetter and upper/lowerRoman, zero ancestors and independent compound
legal lists. Two real facade saves/reopens preserve exact Go/SF/native semantics.
Native counter0-to5 edits, including a zero ancestor, and a legal glyph edit back
to padded format update real glyphs. Go/SF reimport verifies all30 edited main/cell
markers and diagnoses stale intent without overriding current native data.
Old literal and original list readers remain regression checks. These are headless
content/counter/save checks, not browser paint, preset controls or all live-policy
editing parity. Appearance/navigation/nonvisual/provenance intent, API/CLI and
actual browser qualification remain required before the native adapter is complete.

```sh
MANYDOC_COLLEAF_ZERO_QA=output/colleaf-qa/zero-lists.json GOWORK=off go test -run TestColleafZero -count=1 .
node scripts/verify_colleaf_zero_lists.cjs
MANYDOC_COLLEAF_ZERO_SAVED=output/colleaf-qa/zero-lists-saved.json MANYDOC_COLLEAF_ZERO_EDITED=output/colleaf-qa/zero-lists-edited.json GOWORK=off go test -run TestColleafZero -count=1 .
GOWORK=off go test -run '^$' -fuzz '^FuzzColleafNumberProjection$' -fuzztime=10s -parallel=2 .
```


## Current registered hyperlink persistence and next navigation evidence

The earlier core/facade probes did not register the real hyperlink plugin. Current
private import now handles DOC_HYPER_LINK_PLUGIN from @univerjs/docs-hyper-link1.0.4.
Its onLoad applies links[{id,payload}] by global rangeId to main/header/footer
HYPERLINK0 records. It excludes note bodies. Export prefixes running-story hyperlink
IDs; import copies modified containers and applies current payloads over missing/
different body URLs with explicit diagnostics. Unused identities are disclosed.
Duplicate resources/entries/global rangeIds, malformed shape/Unicode/typed member
aliases and conflicting internal/external targets reject. Permission resources
remain application-owned and introduce no document-library permission gate.

Primary packages/source: Apache-2.0 npm releases
https://registry.npmjs.org/@univerjs/docs-hyper-link/-/docs-hyper-link-1.0.4.tgz and
https://registry.npmjs.org/@univerjs/docs-hyper-link-ui/-/docs-hyper-link-ui-1.0.4.tgz.
The model tar's SHA512 is0d610e27f1e5eb7a0cab5ee50c2c3b123889d6206537ccb1ab45af70954d823c55a0015c54c4339e05808bd9eade5c02a0ae71d24aabd675557ce8df891534ad;
its lib/cjs/index.js SHA256 is28566260eb368abfc45fad97045943dc23bbaa54654cd9f955e51926168a4da4.
The UI tar's SHA512 isf942ab398775ff511e3cac04419e39d71ed0ee38af68e2ae0f07cd0fedf5ed47c0735434e90be79548f22ed4b2f75908b1637778e5c5bded775955ef2d32a39f;
its lib/cjs/index.js SHA256 isa5055ae57c52a0f593e2e10e10b3ae39612f9df3bcf84ae4c9f8fc77ba7ac6fb.
Downloads, npm installs/locks and source-only UI extraction remain ignored QA data.

scripts/verify_colleaf_hyperlinks.cjs registers the actual model plugin and verifies
two saves/reopens, five main/cell/list/header/footer targets, a separate note URL,
astral boundaries and authored13pt/bold/#123456/no-underline glyph fields. A header
resource-only edit changes the actual loaded target; Go/SF import retains it both
before and after plugin load/save. A hash-pinned unmodified UI click-handler/helper
region executes against real loaded models with window.open stubbed. It selects
all five external targets and correct call arguments. This is source-handler and
save evidence, not browser widgets/events/popups/paint or actual external navigation.
No app/customer documents were uploaded and no external links were opened.

The same UI source has BOOKMARK3 lookup by rangeId/properties.bookmarkId, and
heading lookup through paragraphStyle.headingId, then selects a collapsed UTF-16
main-body offset. Native bookmark/heading point output/import, real selection/
scroll behavior and internal links remain unfinished. Do not treat published field
names or a headless URL handler pass as proof of their completion.

Optional preparation after the existing pinned core/drawing QA environment:

```sh
npm install --ignore-scripts --no-audit --no-fund --save-exact --prefix output/colleaf-qa @univerjs/docs-hyper-link@1.0.4
mkdir -p output/codec/docs-hyper-link-ui-1.0.4
curl -fsSL https://registry.npmjs.org/@univerjs/docs-hyper-link-ui/-/docs-hyper-link-ui-1.0.4.tgz -o output/codec/docs-hyper-link-ui-1.0.4.tgz
shasum -a 512 output/codec/docs-hyper-link-ui-1.0.4.tgz
tar -xzf output/codec/docs-hyper-link-ui-1.0.4.tgz -C output/codec/docs-hyper-link-ui-1.0.4
MANYDOC_COLLEAF_HYPERLINK_QA=output/colleaf-qa/hyperlinks.json GOWORK=off go test -run TestColleafHyperlink -count=1 .
node scripts/verify_colleaf_hyperlinks.cjs
MANYDOC_COLLEAF_HYPERLINK_SAVED=output/colleaf-qa/hyperlinks-saved.json MANYDOC_COLLEAF_HYPERLINK_RESOURCE_EDITED=output/colleaf-qa/hyperlinks-resource-edited.json MANYDOC_COLLEAF_HYPERLINK_EDITED_SAVED=output/colleaf-qa/hyperlinks-edited-saved.json GOWORK=off go test -run TestColleafHyperlink -count=1 .
```


## Current bookmark-point and typed internal navigation component

Real BOOKMARK3 custom ranges now represent SF paragraph/heading block anchors and
inline anchor atoms. startIndex=endIndex denotes a point at a UTF-16 scalar
boundary, including an astral start or paragraph end; it carries no document text.
Small manydocNavigation version1 role/paragraphId properties retain block-vs-run
ownership only while the actual paragraph and current point agree. Native TextX
moves win. Stale block placement becomes an inline point with diagnostics. IDs
used for transport avoid logical/missing target names and all other range IDs;
actual SDK lookup accepts both rangeId and properties.bookmarkId, so accidental
aliases would otherwise make a missing target appear resolved.

Main bookmark aliases canonicalize to the SF identity. Native headingId becomes a
scoped SF navigation name with diagnostics; current links target that point.
Equivalent fragment URL/bookmark or URL/heading pairs are one typed target, while
contradictory targets reject. Redundant href spelling normalizes through the
existing SF typed-destination contract. Explicit unresolved IDs persist in checked
document intent; currently missing native references become explicit SF unresolved
identities. Foreign bookmark spans retain the current start and disclose loss of
range editing extent. Unknown bookmark metadata/invalid hints and surviving graph
coverage are checked. Structural points not yet representable reject rather than
being silently dropped.

scripts/verify_colleaf_navigation.cjs uses the already hash-pinned model/UI source
from the preceding section, actual registered plugins and selection manager, with
a stub render manager and window.open. It checks nine main points, twelve internal
selection-refresh requests from main/cell/list/header/footer/note contexts, actual
headingId lookup, false outcome for missing targets, emoji integrity and authored
13pt/bold/#123456 fields. Two real saves/reopens and actual TextX insertion retain
current offsets. Go/strict .vdocs/SF reimport checks all saved/edited structures,
styles and relationships and does not mutate source snapshots. A1,024-paragraph
point graph is an additional structural regression, not a visual fidelity claim.

The handler requests a collapsed main-body selection with preserveCaret; the real
selection manager emits its refresh request. No render/controller is mounted in
this probe. Actual caret display, scrolling, widgets and browser events remain
unqualified. The pinned handler resolves targets only in the main body; links to
nonmain point definitions retain data with an explicit limitation diagnostic.
Real table/image-start points are now mapped as described below. Standalone
AnchorPoint native export and remaining structural cursor projection remain
required work; list/section/page-break owners are mapped in the flow component
below. This component does not claim
full native adapter/API/CLI or overall goal completion.

```sh
MANYDOC_COLLEAF_NAVIGATION_QA=output/colleaf-qa/navigation.json GOWORK=off go test -run TestColleafNavigation -count=1 .
node scripts/verify_colleaf_navigation.cjs
MANYDOC_COLLEAF_NAVIGATION_SAVED=output/colleaf-qa/navigation-saved.json MANYDOC_COLLEAF_NAVIGATION_EDITED=output/colleaf-qa/navigation-edited.json MANYDOC_COLLEAF_NAVIGATION_HEADING=output/colleaf-qa/navigation-heading.json GOWORK=off go test -run 'TestColleafNavigation|TestColleafForeignHeading' -count=1 .
GOWORK=off go test -run '^$' -fuzz '^FuzzColleafNavigationPoint$' -fuzztime=10s -parallel=2 .
```


## Current real table/image point component

SF table/image Block.Anchors now use native BOOKMARK3 points at the actual table
start control/image atom. Sparse version1 manydocNavigation accepts table/picture
roles with ownerId and no paragraphId. Owner IDs follow promoted running-story
table catalogs. Body interpretation classifies current token/source relationships
before paragraph fragmentation, so points on images are not consumed into an
empty temporary paragraph and points on tables are not omitted. No new text,
paragraph, copied asset or SF body cache is introduced.

Hint-free foreign points at these structural starts attach to the current table/
image. Current edits win over stale intent, including points moved into ordinary
text; diagnostics disclose ownership changes, unknown metadata and bookmark span
extent loss. SF container anchors retain order and multiple aliases. Unmapped
row/cell/section controls and other missing source graph nodes still reject.

scripts/verify_colleaf_container_navigation.cjs uses the same pinned packages,
validator and unmodified handler/actual selection manager as the previous probe.
It checks16 points across main/nested/cell/list/header/footer/note bodies and seven
exact internal selection-refresh requests, validates current token/source IDs,
removes two hints to qualify foreign native points, saves/reopens twice and uses
TextX to insert Q before all main points. Go strict .vdocs/SF reimport verifies the
complete saved/edited tree, assets, styles and relationships. The inserted text
retains TextX's separate interval. Skeleton construction passes; real browser
paint/caret/scroll/controller behavior and nonmain target lookup remain open.
Standalone AnchorPoint, list/section/page-break anchors, remaining structural
cursor projection, broader intent/API/CLI/UI and final goal audits are unfinished.

```sh
MANYDOC_COLLEAF_CONTAINER_QA=output/colleaf-qa/container-navigation.json GOWORK=off go test -run TestColleafContainerNavigation -count=1 .
node scripts/verify_colleaf_container_navigation.cjs
MANYDOC_COLLEAF_CONTAINER_SAVED=output/colleaf-qa/container-navigation-saved.json MANYDOC_COLLEAF_CONTAINER_EDITED=output/colleaf-qa/container-navigation-edited.json GOWORK=off go test -run TestColleafContainerNavigation -count=1 .
```


## Current list, break and section point component

Version1 owner hints now also accept list (first paragraph ID), break (real
form-feed control) and section (current native section ID/start). List points
become container anchors only at the reconstructed list's current first-paragraph
start. Moved points retain actual inline locations with diagnostics, including
moves within the same paragraph. Explicit section and child points can share an
offset without collapsing ownership. Empty section anchors survive save/reopen.
A point on a native rendered soft page break becomes a zero-height SF AnchorPoint
when that layout boundary is omitted, with the ordinary soft-break diagnostic.
Standalone AnchorPoint native export is now mapped as described below, including
that imported case; other foreign structural cursor recovery remains required.

The preliminary note composition sees a first-section subset. Its temporary
unresolved set admits references to omitted root/other-section anchors; the final
native intent retains the original declarations and complete graph validation.
No invented final destination or runtime permission gate is introduced.

scripts/verify_colleaf_flow_navigation.cjs reuses the same pinned validator,
plugins, unmodified handler and actual selection manager. Eleven points across
main/nested/header/note lists, breaks and three sections (one empty), nine exact
internal refresh requests, two saves/reopens and actual TextX movement pass.
Go strict .vdocs/SF reimport checks all structures, relationships, actual interval
boundaries and exact stale-section diagnostics. Render manager remains stubbed;
actual browser caret/scroll/paint/events and remaining structural/API/intent
qualification remain open.


## Current standalone zero-height block component

Real BOOKMARK3 points now carry optional role point metadata: pointScope selects
native section or list-item ownership, ownerId binds its native section/first-
paragraph ID, positive pointOrder identifies an ordered zero-height block and nonnegative
pointIndex orders its aliases. pointCarrier optionally binds the existing required empty
paragraph for a point-only native flow. No extra characters, serialized SF body,
text, styles or assets are stored. Ordinary native content ID allocation remains
unchanged; rebinding visits only ranges from the current flow.

The rich reader inserts point groups at current reconstructed flow boundaries,
then removes duplicate inline/container carriers only where the final grouped
tree contains the standalone blocks. This preserves item-end and outside-list
points at the same cursor. Edits away from those boundaries retain actual current
locations with colleaf_stale_point_ownership. Terminal LF points remain represented.
Bare-empty carrier omission requires current owner/position/content/presentation;
unrelated or edited paragraphs stay, with colleaf_stale_point_carrier when needed.
Order-slot duplicates and malformed bindings reject. The SF paragraph-first list
item contract is unchanged.

scripts/verify_colleaf_standalone_points.cjs reuses the same pinned packages,
validator and unmodified internal handler/actual selection manager. It qualifies
ordered groups/aliases, shared cursors, cell/terminal/point-only positions, fifteen
exact main selection requests, unchanged native text after two saves/reopens and
actual TextX movement. Strict Go/.vdocs/SF comparison checks complete saved/edited
structures, assets, note/link relationships and expected diagnostics. Dense256-
item, explicit-section and invalid-carrier regressions add structural coverage.
No actual browser caret/scroll/paint/event or whole-native fidelity is claimed.
Foreign control normalization is described below. Nonmain lookup, remaining
appearance/provenance/API/browser and final acceptance remain required.


## Current foreign control-cursor projection

Foreign bookmarks at row/cell controls, section/table terminators and bare image/
page-break paragraph terminators now recover as logical SF AnchorPoint boundaries.
Row edges use first/last cells; covered horizontal/vertical cells retain their
identities at the validated spanning origin. Table/image/break terminators become
following boundaries. colleaf_structural_cursor discloses original UTF-16 offset,
story context and normalized logical target. Exact native control-cursor identity
is not represented. Known current table/image/paragraph/standalone mappings keep
their existing semantics; unsupported interpreted constructs still reject.

Flow After data retains terminal points without duplicate paragraph IDs, so list
continuations stay owned. Deferred cell projection covers earlier-row origins.
Projection-local warning deduplication avoids scanning the entire diagnostic
history per cursor. No visible text, asset or cached SF body is inserted.

scripts/verify_colleaf_structural_cursors.cjs reuses pinned packages/validator/
unmodified handler and actual selection manager. It asserts 21 original and 21
normalized targets, covered-cell sources, unchanged native text/image bytes, two
saves/reopens and actual TextX shifts; each probe issues 21 exact internal requests.
Go strict .vdocs/SF checks compare full saved/edited graphs and diagnostic evidence.
All 63 scalar boundaries of the rich reference, horizontal continuations,
header/footer/note contexts, list ownership and hard-break termini have regressions.

SF preserves the historical loss diagnostics exactly. The normalized native
semantic fixed point now preserves them through document evidence below; its
temporary clearing workaround is removed. Original source expectations stay unchanged.
Headless requests do not prove browser caret/scroll/paint/events. Nonmain lookup,
remaining appearance/nonvisual/provenance/API/preset and final acceptance work
remain required.


## Current document-level evidence component

Document properties intent optionally includes evidence version1: ordered
Diagnostics, original metadata PageCount, PDF pages and PDFNavigation. The same
checked CUSTOM carrier persists it through actual FDocument.save; it contains no
body/cache or native-pagination authority. History duplicates/order remain exact;
new identical warnings deduplicate against history. Returned pointer-bearing
source evidence is cloned. Known resource payloads now pass typed canonical JSON
validation before semantic decoding, including aliases/duplicates/Unicode/nulls.

PDFNavigation FlowBinding accepts unbound when current editable targets differ
from original physical inference. Source resolution facts, names/bytes, nullable
views, geometry and historical source order persist. Deleted anchors are not
restored from evidence; newly created editor targets do not resolve an original
source-missing name. Syntax/geometry validate before reconciliation, and final
model validation uses the complete current graph. Unknown bindings cannot be
repaired by target edits. Ordinary bound values still require actual anchors.

scripts/verify_colleaf_evidence.cjs uses pinned actual plugins, validator and
renderer to check Unicode/newline/duplicate history, two original PDF pages versus
one current native skeleton page, physical views/annotations,13pt/bold/color emoji,
two saves/reopens and actual TextX/delete/create edits. Go strict .vdocs/SF tests
check full root data/current graph, malformed evidence, isolation and repeated
stability. The structural-cursor semantic fixed point now retains its diagnostics;
the clearing workaround is removed. This is document-level history qualification,
not full imported-PDF body provenance, browser painting or adapter completion.
Full per-node Source transport, remaining appearance/intent/API/UI and final
acceptance remain required; paragraph/note components are qualified below.


## Current paragraph origin component

Direct paragraphStyle.manydocOrigin contains profile manydoc.paragraph-origin,
version1 and a source reference. Root evidence.origins stores each source/fingerprint
record once. Fingerprints contain no text and are checked against source-free SF
paragraph semantics after full native composition. Exact current bindings retain
SourceInfo. Changed semantics retain facts with historical=true and an explicit
colleaf_source_historical diagnostic; they never restore old content/presentation.
Missing references archive facts in unboundSources with colleaf_source_unbound.
Inherited source properties are not direct ownership. Malformed digests/versions,
unknown references and contradictory inline records reject.

scripts/verify_colleaf_origins.cjs reuses the pinned SDK/validator/renderer. Seven
source records cover main/list/cell/header/footer/note paragraphs, unchanged text,
authored emoji/style, two saves/reopens, TextX and reference detachment. Strict Go/
.vdocs/SF compares complete baseline/edited/detached semantics and stable repeated
round trips. Actual ProseMirror validates70 nodes/marks/attributes, including the
new root archive and historical flags. Explicit section stories/citation paragraphs,
clone isolation, malformed records and formatting admission also have regressions.
These are synthetic paragraph-level sources; note-definition/label sources are
qualified separately below. Run/citation, table/image/container, cell/item and other
provenance remain required, as do native appearance/API/preset/browser and the
broader final audit. No body cache, service,
CGO dependency, app modification or permission gate is introduced.


## Current note origin component

Whole-document evidence.noteOrigins contains version1 native/scoped identities,
Source/LabelSource, KindInferred and separate source-free SF body/marker hashes.
Source marks and empty source-only attrs are normalized out of hashes; binding
changes alone do not edit note semantics. No editable note body, literal label or
alternate style tree is stored. Restoration
follows numbering, citation, navigation and paragraph-origin composition. Ownership
requires both current native identity binding and matching current SF note identity.
Body edits qualify definition Source as historical; marker edits qualify definition
and LabelSource. Missing identity bindings archive available sources unbound and
diagnose original inferred placement without asserting a new note's kind. Native
mark state remains authoritative: loss of an automatic-label identity hint may
leave its persisted native literal mark rather than recreate old automatic intent.
Typed/version/hash-shape/source-page/duplicate checks reject malformed evidence.

The original first paragraph fingerprint is taken before a visible body-mark prefix
is projected. Removing that prefix at import therefore preserves unchanged source
binding. scripts/verify_colleaf_note_origins.cjs checks three source/inference records,
scoped foot/end IDs, no text cache, actual styled glyphs and neutral body marks,
two saves/reopens, actual TextX body/label edits and identity detachment. Strict Go/
.vdocs/SF tests check full structures, independent source historical states, archive
isolation and repeated stability; actual ProseMirror checks61 nodes/marks/attrs.
Go also checks detached sourced-note archives and malformed records. This synthetic
headless qualification does not complete full per-node provenance, marker/API/
preset/browser or the full imported-PDF/final acceptance audit. The run/citation
component is qualified separately below.


## Current run origin component

Direct textRun.ts.manydocOrigin carries only manydoc.run-origin/version1/ref.
Root evidence.runOrigins stores each original SourceInfo and source-free SF run
fingerprint once. Restoration follows current winning native text intervals and
final citation/body-mark composition, not old offsets or cached text. Exact run
semantics retain source facts. Changed or split fragments retain original geometry/
structure with historical=true and colleaf_run_source_historical; no source-free
subgeometry is invented. Missing owners archive facts with colleaf_run_source_unbound.
Inherited style metadata is diagnosed without ownership; overwritten intervals do
not own provenance. Sources outside direct interval context are explicitly diagnosed.
Profile/version/digest/reference/duplicate/inline contradictions reject.

Nonempty ordinary text, links, story/cell/list/note body runs, rich body marks,
rich citation-label runs and rich/default citation atoms have source mappings.
Default projected labels do not duplicate an atom's source. Internal markers do
not change uniform-label interpretation. Paragraph hashes exclude nested sources,
so dropping only a run binding does not stale paragraph provenance. Original
source-free paragraph hash behavior remains intact.

scripts/verify_colleaf_run_origins.cjs uses pinned SDK/validator/renderer and checks
eleven sources/reference-only fields, whole bold13pt emoji, two saves/reopens,
actual TextX split/replacement and binding removal. Strict .vdocs/Go/SF compares
exact fragments, styles, labels, sources, historical/unbound states and repeated
stability. ProseMirror checks110 nodes/marks/attrs and exposed the missing source
mark historical declaration, now fixed alongside optional table_widths/table_spans;
a Go regression checks every public SourceInfo field against source-mark attrs.
Go also qualifies overwritten/inherited ownership and malformed evidence. This is
synthetic headless qualification. Zero-width empty-text/anchor sources still reject;
BodyLabelStyle.Source is invalid in the existing shared model (use LabelSource).
Container/cell/item/anchor provenance and remaining intent/API/preset/browser/final
acceptance work remain required.


## Current table/cell origin component

Direct tableSource[id].manydocOrigin and tableRows[].tableCells[].manydocOrigin
carry manydoc.table-origin or manydoc.cell-origin, version1 and a reference only.
Root evidence.tableOrigins stores deduplicated validated SourceInfo and source-free
SF fingerprints, never editable rows/cells/text. Cell hashes cover the cell node;
table hashes cover the table tree. Shared original facts therefore do not conflate
owner types. Changes to nested source bindings alone do not edit those semantics.

Final composition restores exact owners, marks edited content/grid/presentation
historical with colleaf_table_source_historical, and archives missing owners with
colleaf_table_source_unbound. Original table-width/span and physical evidence never
claim current native geometry. Every descriptor reference validates, including
covered cells: a valid covered marker has no SF owner and archives its facts; missing
or contradictory covered records reject. Existing empty-continuation/content checks
remain in force. Reads do not rewrite caller-owned source descriptors.

scripts/verify_colleaf_table_origins.cjs reuses pinned SDK/validator/renderer for
thirteen sources across five merged/nested/main/header/footer/note tables, reference-
only metadata, styled emoji/table skeleton, two saves/reopens, actual cell TextX
edit, binding removal and covered placement. Strict .vdocs/Go/SF compares full
structures, width/span facts, source states and repeated stability. ProseMirror
checks165 nodes/marks/attributes. Go also checks shared facts, malformed pools and
missing covered references. This synthetic headless component does not complete
full per-node provenance or native/API/browser/final qualification. The image
component is qualified separately below.


## Current image origin component

Direct drawings[id].manydocOrigin carries manydoc.image-origin/version1/ref;
evidence.imageOrigins stores deduplicated SourceInfo plus source-free SF image
fingerprints. Root/running/note descriptors all participate. There is no asset/body
cache. Restoration follows current image bytes, alt text, preferred/fitted geometry
and anchors. Changed semantics retain original facts with historical=true and
colleaf_image_source_historical. Missing owners archive sources with
colleaf_image_source_unbound. Stale size intent still diagnoses independently;
source evidence cannot restore an old asset or override current geometry.

Registered DOC_DRAWING_PLUGIN mirrors retain the same source property and must
agree with persisted descriptors. The SDK probe changes coherent descriptors and
mirrors before actual plugin saves. Go rejects a provenance-reference mismatch in
the registered mirror. Profile/version/digest/missing/duplicate/contradictory direct
source records also reject, and reads do not mutate native input.

scripts/verify_colleaf_image_origins.cjs reuses pinned SDK/validator/renderer for
seven main/cell/list/header/footer/note source records, embedded PNG/original JPEG
bytes, a main drawing frame and styled emoji, two saves/reopens, resized/replaced
snapshot assets and detached references. Strict .vdocs/Go/SF compares exact assets,
structure, preferred sizes, source states, diagnostics and repeated stability.
Actual ProseMirror checks96 nodes/marks/attributes. Go separately qualifies fitted
explicit-section/running/note sources. This is synthetic headless provenance
qualification; browser bitmap paint, remaining source slots and full native/API/
preset/final acceptance remain required. The list/item component is qualified below.


## Current list/item origin component

Direct paragraphStyle.manydocListOrigin / manydocItemOrigin contain respectively
manydoc.list-origin or manydoc.item-origin, version1 and ref only. Root
evidence.listOrigins stores deduplicated SourceInfo and source-free SF fingerprints,
not list/item content. Final current list composition assigns the list source from
its first leading paragraph and each item source from its leading paragraph.
Markers left on unmarked or other non-owning paragraphs do not assert ownership.
Inherited source properties diagnose; every direct native reference still validates.
Private markers clear before public return.

List fingerprints cover current list semantics. Item fingerprints cover its subtree,
parent numbering attrs and ordinal index, without repeated sibling-body encoding.
Counter/format/position edits therefore qualify affected item sources historical,
even with unchanged text. Changes to source binding metadata alone do not stale
semantics. Changed owners use colleaf_list_source_historical; missing owners archive
facts with colleaf_list_source_unbound. Source physical coordinates remain original.

scripts/verify_colleaf_list_origins.cjs reuses pinned SDK/validator/renderer for24
records (10lists/14items) across nested/independent/restarted/bullet/zero/cell/running/
note cases, styled emoji, two saves/reopens, TextX content edits, rendered2/3-to9/10
native restart and list/item reference removal. Strict .vdocs/Go/SF compares full
structures/numbering/source states/diagnostics and repeated stability. ProseMirror
checks326 nodes/marks/attributes. Go separately qualifies inherited/contradictory/
lost ownership, unmarked paragraphs, malformed pools and source-only bullet-item
context. This synthetic headless evidence does not complete other container/anchor
source slots, remaining appearance/intent or native/API/browser/final acceptance.


## Current inline-anchor origin component

Native bookmark properties.manydocOrigin carries manydoc.anchor-origin/version1/ref.
Root evidence.anchorOrigins stores deduplicated SourceInfo and source-free hashes
of canonical anchor identity plus paragraph semantics. No context/text cache is
stored. Original paragraph context is captured before temporary rich note-body
marker prefix insertion. Only Source is admitted beyond the existing unformatted
Run.Anchor contract; formatting/text/note/link annotations remain invalid.

Current native point names and locations remain authoritative. Context or identity
changes retain historical facts with colleaf_anchor_source_historical; this is
conservative, including surrounding edits that leave the anchor name unchanged.
Missing inline owners archive facts with colleaf_anchor_source_unbound. A current
block-owned point does not silently acquire an inline source as its block source.
Every bookmark reference still validates, even when current placement does not
consume the source as inline ownership. Source-only metadata removal does not stale
other anchors, and no source geometry becomes current native coordinates.

scripts/verify_colleaf_anchor_origins.cjs checks seven main/list/cell/header/footer/
note sources, rich body-mark projection, typed point lookup and bold13pt emoji,
two actual plugin saves/reopens, TextX shift and moved/renamed/detached snapshots.
Strict .vdocs/Go/SF compares point identities/placement/source states/diagnostics and
repeated stability. ProseMirror checks157 nodes/marks/attributes. Go also checks
changed owner kind, malformed/unconsumed/contradictory references and formatting
rejection. Synthetic headless evidence does not qualify browser caret/scroll/paint,
remaining section/break/standalone source slots, identityless empty-text policy or
full native/API/final acceptance.


## Current section/break/standalone origin component

Section-record manydocFlowOrigin, direct single-break paragraphStyle.manydocFlowOrigin
and standalone bookmark properties.manydocFlowOrigin contain manydoc.flow-origin,
version1 and ref only. evidence.flowOrigins deduplicates original SourceInfo and
source-free SF hashes. Section hashes include current body/layout/running content;
break hashes include break semantics/anchors; standalone hashes include group alias
identity/order. The first group bookmark carries the group source. No body/group
cache is stored and original physical geometry never dictates native pagination.

Final composition restores owners, uses colleaf_flow_source_historical for changed
semantics and colleaf_flow_source_unbound for unused facts. Every carrier reference
validates, including non-owning metadata. A rendered soft boundary or mixed text
paragraph does not own a single-hard-break source. Caller snapshot bytes remain
unchanged. scripts/verify_colleaf_flow_origins.cjs checks nine sources across two
sections, break and main/running/note groups; two plugin saves/reopens; styled emoji;
TextX edit, alias rename, binding removal and soft-boundary snapshot edits. Current
native skeleton pages are3 with the hard break and2 after soft conversion. Strict
.vdocs/Go/SF compares full structures/layout/source states/diagnostics and repeated
stability; ProseMirror checks117 nodes/marks/attributes. This synthetic headless
component does not complete combined real-source/browser/native/API/final audits.

Source on identityless empty text is now explicitly rejected by SF encoding rather
than silently skipped. Detached facts can be represented in UnboundSources. Named
points and empty paragraph blocks retain their distinct supported semantics.
