# Structured format: ProseMirror JSON

## Decision and evidence

The persistence/editor format is a versioned ProseMirror document schema,
`manydoc-prosemirror/1`, stored in `.sf` files. The machine-readable NodeSpec and
MarkSpec are in `schema/prosemirror-v1.json`. This is the established ProseMirror
JSON encoding (`type`, `attrs`, `content`, `marks`, `text`), with an explicit
manydoc schema profile; it is not a claim that arbitrary ProseMirror, Tiptap or
Colleaf documents use the same schema. All production code is Go. Using this
encoding does not require running ProseMirror or a JavaScript engine.

Sources consulted on 2026-11-23:

- [ProseMirror guide](https://prosemirror.net/docs/guide/): schema-defined trees,
  marks, attributes, JSON serialization and programmatic manipulation.
- [Basic schema](https://github.com/ProseMirror/prosemirror-schema-basic):
  paragraphs, headings, text, images, strong/emphasis and links.
- [List schema](https://github.com/ProseMirror/prosemirror-schema-list):
  ordered_list, bullet_list, list_item and the order attribute.
- [Table schema](https://github.com/ProseMirror/prosemirror-tables):
  table, table_row, table_cell and row/column spans.
- [Pandoc types](https://github.com/jgm/pandoc-types/blob/master/src/Text/Pandoc/Definition.hs):
  an alternative established document AST with blocks, inlines, notes, metadata
  and rich tables. No Pandoc implementation is included or executed.

Pandoc was considered because its document vocabulary is broader by default.
ProseMirror was selected for browser editing and direct tree manipulation, while
Pandoc's AST would also need application conventions for page layout, running
headers/footers, asset persistence and PDF extraction provenance. HTML was
considered for browser interoperability, but is ambiguous about document page
semantics and requires CSS interpretation; plain text/Markdown cannot preserve
all required rich content. The old manydoc JSON was not selected as the canonical
format because it invents a document vocabulary and is already too limited.
ProseMirror also requires explicit extensions for page layout and PDF provenance;
those extensions must be documented and tested, rather than called universal
editor compatibility.

No Colleaf checkout was found in the local workspace search. The Daptin checkout
has `.vdocs` extension references in tests, but no editor schema was found in the
searched source. This does not establish what Colleaf uses. Inspect its real
schema before adding a `.vdocs` adapter; do not infer its representation from its
file extension. A package.json search for Colleaf/vdocs across all local GitHub
checkouts also returned no matches. Daptin was read only and was not changed.

## Wire profile and current API

```json
{
  "type": "doc",
  "attrs": {
    "schema": "manydoc-prosemirror/1",
    "metadata": {"title": "Example"},
    "layout": {"width": 612, "height": 792, "margin": 54}
  },
  "content": [{"type": "paragraph", "content": [
    {"type": "text", "text": "Hello", "marks": [{"type": "strong"}]}
  ]}]
}
```

`StructuredDocument`, `StructuredNode` and `StructuredMark` are the public Go
values. Node/mark attributes use `json.RawMessage`; they can be constructed with
`json.Marshal` and changed without any JavaScript. `ReadStructured` and
`WriteStructured` validate this profile. `ToStructured`/`FromStructured` bridge
the current Go conversion engine's `Document` model. `Read`/`Write` accept `SF`
and the CLI recognizes `.sf` and `extract -format sf`. The old `.json` encoding
is retained as a legacy API for existing users, not a second canonical format.
Do not develop features only in legacy JSON; extend the SF and engine together.

The first implementation bridges the existing engine to avoid replacing all
parsers/renderers in one unverified change. The bridge must reject any construct
it cannot preserve. It is not evidence that the full requested SF is implemented.

| Semantic | Current encoding |
| --- | --- |
| Body | doc.content in order; an empty document is valid |
| Paragraph/heading | paragraph/heading with text children; heading.attrs.level 1–6 |
| Formatting | strong, em, underline, strike marks; link.attrs.href |
| Lists | ordered_list (nonnegative order, default 1) or bullet_list → list_item → paragraph/heading followed by blocks, including nested lists |
| Tables | table → table_row → table_cell; colspan 1–1000, positive rowspan bounded by remaining rows; nested blocks |
| Images | block image; src is an embedded `data:<media-type>;base64,...` URI; alt, width, height |
| Page breaks | page_break block |
| PDF recovery | doc.attrs.pages; source node/cell/list-item attributes and source text/reference marks preserve positioned provenance |
| Metadata/layout | doc.attrs.metadata and doc.attrs.layout, dimensions in points |
| Running content | doc.attrs.headers and footers contain block arrays |
| Notes | doc.attrs.notes contains id, kind, content block arrays; inline note_reference atoms carry id/kind |
| Representation losses | doc.attrs.diagnostics contains code/message objects |

No image URLs are fetched. Image bytes are included in persistence, including
within tables, running content and notes. Rendering still validates image types.
Diagnostics survive SF persistence and still require `AllowLossy` for rendered
DOCX/PDF output. Revision/comment/field interpretation currently follows the
DOCX reader's documented final-visible-view policy; their editing semantics are
not yet represented by dedicated SF nodes or marks.

The ProseMirror schema declaration checks tree shape. The Go validator additionally
checks numeric ranges, image URI encoding, strings, nesting, supported semantics,
and every node-specific attribute. Unknown JSON fields, nodes, marks and
attributes are errors. Empty legacy text runs are omitted; zero and one row/column
span mean the same thing and serialize as one. Adjacent equal-format text-node
boundaries have no document semantics. Separate adjacent lists retain separate identities, including lists of the same
type. Ordered values from zero through 2,147,483,647 are supported, bounded so
all item values fit. `Block{Kind: ListBlock, List: "number"|"bullet", Items: ...}`
is the engine representation; `Start *int` is nil for the default one. Legacy
paragraph `List` fields are still accepted and consecutive same-kind legacy runs
are normalized into a tree. Explicit lists are never coalesced.

## Note identity and layout

`Run.Note *NoteReference` and the inline `note_reference` atom carry an `(id, kind)`
identity; `kind` is footnote or endnote. Definitions occur once in doc.attrs.notes,
so repeated references do not duplicate rich bodies. Paragraphs/headings accept
text and reference atoms with the declared formatting marks. Text/link content on
a reference atom is invalid. IDs are nonempty valid document strings and are
scoped by kind, so an endnote and footnote can share an application ID.

Note bodies can contain the same block tree as the document. Duplicate identities,
dangling references, references in running content and nested note references are
invalid. Unreferenced definitions may persist in SF/legacy JSON as drafts; rendered
DOCX/PDF rejects them rather than creating invisible or silently invented anchors.
Plain text represents references with `[kind:id]` identity markers, followed by
stored note bodies. Automatic labels are continuous decimal numbers per kind,
assigned by first body-reference order independently of application IDs.

DOCX uses ordinary footnotes.xml/endnotes.xml, implicit part relationships and
standard reference elements. Markup-compatibility attributes in
`urn:manydoc:metadata:1` preserve original application IDs and definition order
while w:id remains a portable integer. Part-local links/images use part-local
relationships. Repeated references use standard cached NOTEREF fields and invisible
bookmarks, verified against the actual first reference and note. The library does
not evaluate arbitrary fields. Corrupt identities/order/bookmark bindings reject;
stale cached displayed marks produce a normalization diagnostic.

PDF references are measured superscripts with internal destinations. Rich
text/list/image/table footnotes reserve measured space below body text on the
reference page. Overlong bodies
flow onto labelled continuation-only pages before body flow resumes; repeated
references link to the single first body. Endnotes use a separate measured section
and the normal block renderer. Rich endnotes inherit its table/image/layout limits.
Footnote images/tables use shared cell fragments and continue between text lines
or recursively split table bands. Oversized images and explicit cell/note page
breaks remain required continuation work. Custom separator content, nondecimal note numbering, restart/start
or placement changes are diagnosed on DOCX import; custom marks encoded as text remain literal
text alongside the normalized reference, with an explicit diagnostic. Symbol-font
glyph runs reject until an explicit font-to-Unicode mapping is available. The omitted
endnote numbering format has a documented Word-vs-OOXML default ambiguity, also
diagnosed. None of these limitations establishes completion of the full goal.

## List import/export and rendering

The profile's `list_item` content expression is `(paragraph | heading) block*`.
Lists preserve starts, separate boundaries, hierarchy, continuation paragraphs,
and item membership for images and tables. This is a schema profile; the standard
ProseMirror list commands require paragraph-first items, so heading-first support
does not imply universal editor-command compatibility.

DOCX output renders using standard numbering definitions, level references and
start overrides. Invisible standard bookmark ranges (`mdlist1_...` / `mditem1_...`)
record list and item boundaries so structural membership survives round trips.
No duplicate text is stored. Ranges must be nested, closed and consistent with
numbering; contradictions reject. The reader constructs temporary grouping
nodes internally, not persisted content controls. Content-control wrappers were
rejected for output after LibreOffice 7.6 visibly exposed hyperlink instructions.

Ordinary DOCX numbering without these ranges is read from abstract definitions,
per-instance overrides, effective paragraph styles and `numId`/`ilvl`. Counters
continue across intervening non-list paragraphs and respect `lvlRestart`.
`numId=0` cancels inherited numbering. Missing hierarchy levels are not invented;
normalization is diagnosed. Style-name-only membership is diagnosed as inferred.
Untagged DOCX continuation paragraphs have no explicit item membership in OOXML;
this reader keeps them as ordinary blocks, with existing indentation losses
reported. Numeric formats/label patterns outside decimal and a plain bullet are
normalized with diagnostics rather than silently claimed to be preserved.

DOCX output supports its published nine numbering levels. SF can retain deeper
trees within the overall nesting bound; PDF supports them while indentation
leaves measurable text space. PDF rendering uses hanging item indentation,
wraps/paginates item text and nested content, and measures list indentation and
markers inside rich table cells. Images and nested tables use measured fragments;
richer running content remains unfinished.

## Table span semantics

Cells occur once at their origin. A cell's `rowspan` includes its origin row;
covered slots are omitted from subsequent table_row content arrays. Fully
covered rows remain present with empty content. The same occupancy grid validates
SF, DOCX serialization and PDF cell positioning. Overlap, internal uncovered
slots, spans past the final row, more than 10,000 columns or more than 1,000,000
grid slots are errors. Legacy trailing ragged rows remain accepted; arbitrary
skipped leading OOXML grid cells are rejected explicitly.

DOCX `vMerge` restart/continue markers map to this origin representation and are
recreated on export. Required empty continuation paragraphs are structural;
nonempty continuation content is appended to the origin with a diagnostic about
normalized placement. PDF rich-table layout measures spanning cells over the
sum of their row heights. Connected groups that fit with their note starts stay
on one page. Oversized body groups continue using per-cell flow cursors and
page-local reference admission. Interior borders retain logical row-span
ownership; physical page fragments close their borders and add continuation
padding. Oversized nested groups inside cells/footnotes now supply smaller bands
recursively, retaining their own immutable row/cell cursors. Footnote starts use
a minimum legal band rather than the full merged group. Original SF
rows/spans/blocks remain unchanged; no profile version change is needed.

## Required unfinished expansion

This is an initial SF milestone, not fulfillment of the active completion goal.
The following remain required work, even where the current bridge rejects them:

- Oversized PDF image atoms and richer note numbering/separator/layout options.
- Font sizes/families/colors and richer page settings.
- Broader PDF structure recovery, internal destinations and inverse ParentTree
  validation. Positions/assets/pages and explicit/inferred provenance now exist;
  more complex layouts and broader real exports still need qualification.
- PDF font/shaping coverage and table pagination/cell layout expansion, including
  explicit cell/note page breaks and repeated headers. Oversized body/nested/
  footnote groups now continue recursively.

Profile extensions must preserve existing encoded semantics. If a change alters
meaning, use a new schema version and explicit migration. Never use a new profile
number merely to hide an unimplemented required feature.

## Independent verification

`prosemirror-model` is an optional QA dependency, never a Go runtime dependency.

```sh
GOWORK=off go run ./examples/report -out output
npm install --prefix output/sf-qa --no-audit --no-fund prosemirror-model
NODE_PATH=output/sf-qa/node_modules node scripts/verify_structured.cjs output/report.sf
```

The verifier uses the actual ProseMirror reader and `Node.check()`, and checks
node/mark attributes and text, including side content. It proves schema/read
compatibility for supplied files; it does not prove a browser editor integration,
a Colleaf adapter, DOCX layout fidelity or complete rich-document support.

## Positioned PDF provenance

`doc.attrs.pages` holds `PDFPage` values: sequential number, raw media_box/crop_box,
rotate (0/90/180/270), user_unit, physical width/height and affine transform.
`SourceInfo` is stored in block/cell `attrs.source` and the inline `source` mark.
Coordinates are physical points in displayed orientation, origin top-left, y down.
`Matrix` is [a,b,c,d,e,f], x'=a*x+c*y+e and y'=b*x+d*y+f. Geometry values must be
finite and consistent with the page boxes/rotation/unit. Referenced pages must
exist. This extends the profile; it does not establish arbitrary editor support.

Sources carry page/object/order, optional quad/baseline/matrix/regions/font data,
MCID/tag/render_mode, stream/role/element, structure/evidence/confidence/precision
and optional label.
Content is not duplicated in a sidecar. Structure is explicit, inferred or
unstructured; confidence is a heuristic ranking, not a calibrated probability.
Painted text/images remain unstructured until associated with checked forward
logical structure. Recovered logical blocks/roles are explicit; paragraph line
grouping, synthetic separators and label-based list-kind inference retain their
separate evidence. List-item source attributes own original labels independently
from paragraph source data. Cross-page logical blocks use source regions and
page records rather than fake breaks inside editable paragraphs.
Quads describe advances/font metrics or transformed image squares, not glyph ink.
ActualText keeps contributing painted regions, with no claim that a replacement
string has the original glyph matrix. Raster assets are normalized PNG pixels,
not a byte-preserving copy of JPEG compression or original color profiles.

SF/JSON preserve these values. Current DOCX/PDF writers reflow and omit provenance;
rendering requires AllowLossy even if diagnostics are removed. See COMPATIBILITY.md
for unfinished extraction/graphics/image cases and ACCEPTANCE.md for goal status.

## Untagged geometric evidence

Default PDF import now supplies inferred paragraphs, relative typographic
headings, marker lists and numeric tables from positioned content. Source marks
keep original run coordinates, formatting/links and paint identity; nodes use
inferred evidence/confidence. Physical pages/assets persist. No profile schema
change is needed. Marker labels stay in item provenance, and partial prefix
bounds disclose conservative precision. `PDFSkipInference` retains physical
paint groups for inspection. More complex untagged structures and richer model
semantics remain unfinished.


Painted-grid PDF recovery also supplies inferred table/cell boundary sources for
row/column merges and empty cells. Nested tables and images use existing blocks;
original runs/assets/coordinates remain authoritative evidence. Empty paragraphs
can carry boundary provenance without invented text. Native PDF rendering of
image/nested cells now uses measured fragments. SF retains preferred image sizes;
DOCX/PDF fit images to their available container width, so imported rendered
formats return physical extents. One requested dimension preserves intrinsic
ratio; neither means 96 DPI. Oversized body/nested/note groups now continue
recursively. Explicit cell/note page breaks and oversized images still need work.
No SF schema change is required.
