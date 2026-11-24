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

Two additional native differences remain precise unfinished work. A literal percent
in glyphFormat does not follow Go/OOXML template semantics (%%1. displayed11.
instead of%1.). An undefined deeper placeholder such as%9 at level0 throws while
SF ignores it. Export projects ignored placeholders out, stores sparse original-
template intent only for that projection, and restores it only while projection
matches actual glyphFormat. Native format edits remain authoritative. Literal
percent cases reject pending their checked representation. Native zero in
DECIMAL_ZERO displayed00; letter/Roman zero displayed an empty symbol, while SF
currently displays0. Both import/export reject those known nondecimal-zero
mismatches until a format-specific zero policy is represented. Decimal zero works.
These are private unfinished mappings, not new public support exceptions.

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
