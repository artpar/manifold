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

The initial workspace-only search found no Colleaf checkout. A later read-only
GitHub audit found and inspected actual `artpar/colleaf` at
`5ce2ef507b28b92f5b03443ade87bc0e17f3d8b6`. Its native .vdocs is a version-3 wrapper
around Univer 1.0.4 IDocumentData, not this ProseMirror profile. Matching published
Apache-2.0 type declarations were inspected with verified distribution integrity.
COLLEAF_FORMAT.md records exact source links, fields, range semantics and constraints.

ProseMirror remains the canonical logical format because explicit child ownership
and marks support structural manipulation/provenance without an offset/control-token
protocol. Univer snapshots remain the required application adapter boundary: rich
lists/tables/drawings/stories/notes/plugin resources must be mapped and checked.
Raw SF is not .vdocs, and no native adapter or app persistence compatibility is
currently claimed. This corrects the earlier unavailable-schema claim; it does
not require executing either editor at runtime. No Daptin/Colleaf files changed.

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
| Paragraph/heading | paragraph/heading with text children; heading.attrs.level 1–6; optional boolean attrs.pageBreakBefore (default false) |
| Formatting | strong, em, underline, strike marks; link.attrs.href; textStyle with fontFamily, numeric point fontSize, color (#RRGGBB/auto), lang/eastAsiaLanguage/complexLanguage, dir and complexScript |
| Lists | ordered_list (nonnegative order, default 1) or bullet_list → list_item → paragraph/heading followed by blocks, including nested lists |
| Tables | table → table_row → table_cell; colspan 1–1000, positive rowspan bounded by remaining rows; nested blocks; optional row attrs.repeatHeader (false default) |
| Images | block image; src is an embedded `data:<media-type>;base64,...` URI; alt, width, height |
| Page breaks | page_break block |
| PDF recovery | doc.attrs.pages; source node/cell/list-item attributes and source text/reference marks preserve positioned provenance |
| Metadata/layout | doc.attrs.metadata and layout; point width/height/margin, optional four-edge margins and header_distance/footer_distance |
| Running content | doc.attrs.headers and footers contain block arrays |
| Notes | doc.attrs.notes contains id, kind, content block arrays; inline note_reference atoms carry id/kind |
| Representation losses | doc.attrs.diagnostics contains code/message objects |

No image URLs are fetched. Image bytes are included in persistence, including
within tables, running content and notes. Output validates PNG/JPEG configuration
and complete pixels, bounded to 25 million pixels before expansion. Running-content
layout follows each writer's separate contract.
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
or recursively split table bands. Explicit cell/note page breaks propagate
through native table/note layout. Leading breaks can move the first note body
away from its reference; links target that body or a break-only note's final
blank page. Repeated breaks preserve blank pages. Figures fit full continuation
containers; a figure-bound first start that cannot coexist with the reference
uses a linked continuation notice, keeping the actual body destination on its
later page. Repeated references do not duplicate the figure. Custom separator content, nondecimal note numbering, restart/start
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
PDF running stories now use the same measurement with their compact style.

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

- Broader note numbering/separator/layout qualification and image-codec/color variants.
- Full theme/script formatting, embedded font recovery and richer page settings;
  font sizes/families/colors now persist explicitly.
- Broader PDF structure recovery, internal destinations and inverse ParentTree
  validation. Positions/assets/pages and explicit/inferred provenance now exist;
  more complex layouts and broader real exports still need qualification.
- PDF font/shaping coverage and table pagination/cell layout expansion, including
  broader image/layout qualification; repeated prefixes now exist. Oversized body/nested/footnote groups
  and explicit cell/note page breaks now continue recursively. Qualify DOCX
  richer paragraph/page properties without changing SF control semantics.
  Conditional pageBreakBefore import/export now exists separately from controls.

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
DOCX/PDF fit images to full available container width/height, so imported rendered
formats return physical extents. One requested dimension preserves intrinsic
ratio; neither means 96 DPI. Oversized body/nested/note groups now continue
recursively, including explicit cell/note boundaries. Aligned sibling breaks
coalesce; consecutive breaks in one flow preserve distinct pages. Image height
containment now exists; broader DOCX visual/Word qualification still needs work.
No SF schema change is required.


## Conditional paragraph page starts

Paragraphs/headings can carry attrs.pageBreakBefore=true. The engine field is
Block.PageBreakBefore (legacy JSON page_break_before). False is the default;
existing trees retain their meaning, so this additive profile extension does not
change the schema ID. Unknown attributes/types still reject. This is a conditional
page-start request, not an alias for an unconditional page_break node. Native
PDF skips it at page start and propagates that position through nested layout.

DOCX property inheritance resolves to the bool, including explicit cancellation.
Source style programs are not stored as a second authoritative representation.
Exported explicit controls use standard paragraph properties with checked
ignorable metadata to restore their distinct semantics. Empty structural
paragraphs and default heading bold styling no longer invent SF authored nodes
or strong marks on our exports. Exact SF comparisons cover controls/properties,
notes, headings and nested cells; third-party readers can ignore metadata while
retaining ordinary Word structures. LibreOffice content mitigation is verified,
while cell/note pagination differs and wider Word qualification remains required.


Image width/height are preferences, not an instruction to crop content outside
paper. Full-container fitting keeps their prescribed aspect (intrinsic when one
is missing), original pixels and source SF values. A page remainder does not
change the ordinary figure size. PDF and DOCX use their own padding/reference
budgets, so physical sizes can differ. A later DOCX/PDF import reports those
physical extents; exact preferred-size persistence is established through SF,
not claimed through a fitted rendering. No profile schema change was required.


## Repeating table rows

`table_row.attrs.repeatHeader` is an optional boolean with false default. True
rows form a contiguous prefix; merged cells may span within it but not into body
rows. The attribute requests repeated physical rows on continuation pages, without
inventing semantic table_header cells or duplicating authored content. Existing
profile meanings are unchanged. DOCX maps it to tblHeader. Native PDF supports
body, nested and footnote tables, reserving the measured prefix before body
selection/image fitting; header figure budgets also account for legal nested
body starts. Repeated references link to one original note body.
Header-only tables appear once. Native explicit body page boundaries repeat the
prefix on resumed table pages. PDF prefixes are indivisible; taller-than-container
prefixes and page controls inside a prefix reject, while persistence/DOCX retain
the requests. Broader page/layout qualification remains required work.


## Text style mark

`textStyle` is an optional formatting mark with `fontFamily` (string, default
empty), `fontSize` (nonnegative finite numeric points, default zero) and `color`
(empty/default, #RRGGBB or auto), plus the language/direction/complex-script
bindings described below. Null/wrongly typed attributes reject. Hex spelling
canonicalizes while semantic values persist. Font family names are single lookup
names, not CSS fallback lists. The established TextStyle editor pattern is reused,
with this profile's point-unit contract; native Tiptap/Colleaf compatibility is not
implied. Existing omitted styles and profile version meanings are unchanged.

DOCX resolves inherited/direct fields and preserves our unset values via checked
metadata. PDF uses actual configured face programs and effective metrics in all
stories; caller font data/mappings are output configuration rather than customer
identity/permissions or required services. Missing glyph/style/family and unsafe
precision reject. PDF provenance remains separate from editable fields; broader
PDF style/color/source-font coverage, theme/script handling, shading and shaping/bidi
remain required; device-style recovery now exists as described below.


PDF imports now populate textStyle from recoverable family/physical size/device
paint. Optional source-mark attributes font_family_evidence, color_space and
color_components retain origin and effective component values separately from
editable values. Evidence enumeration and component count/range validate in Go;
schema defaults are empty strings/array. Existing source marks/profile meanings
remain valid. ActualText preserves common fields and diagnoses mixed-field losses;
synthetic separators claim only common editable styles and inferred provenance.
Unknown paint omits editable color with a diagnostic; quantization/CMYK/default
space approximations and transforms diagnose. Original SF must survive any caller
retypesetting chosen to satisfy output font availability/precision restrictions.


DOCX theme text colors now calculate settings mappings/HSL tint/shade before
populating existing textStyle.color. No schema/model version changes. Editable
theme/system bindings still diagnose loss; source caches differ from calculated
current-theme RGB in some producers. Unsupported palette resolution uses a final
RGB cache only with uncertainty, otherwise rejects. Broader DrawingML/theme/font
and script features remain required expansion; resolved RGB round trips preserve
this profile's declared scalar, not an absent live theme relationship.


DOCX font-slot/theme-language import now emits ordinary textStyle spans for the
selected family and complex size/marks. Private import selectors do not enter
SF; binding/uncertainty losses diagnose. Exact source spans may split, while equal
effective families coalesce within a leaf. Public schema/version meanings remain
unchanged. Script/layout selector retention and broader qualification remain
unfinished; native unsupported shaping/glyph errors are not completion exemptions.


Native PDF glyph-ID transport expands supported output without changing SF:
LTR supplementary outlines and residual combining marks use configured/admitted
fonts and preserve original logical text through ActualText/ToUnicode. Glyph IDs
are output implementation data, not another canonical text representation. Full
source/font relationships remain in ordinary textStyle/link/note/table values.
Language/direction bindings now exist as described below. Paragraph joining/bidi
and broader Unicode support remain required; atomic unsupported-script errors must not be used as completion evidence.


### Language and text flow

Paragraph/heading `attrs.dir` is `""` (unspecified), `"ltr"` or `"rtl"`. Other
blocks reject it. Optional textStyle strings `lang`, `eastAsiaLanguage` and
`complexLanguage` preserve the general/East Asian/complex-script language bindings.
Empty strings are unspecified. Nonempty tags are at most 255 bytes and must be
accepted by the pinned BCP47 parser; underscores/whitespace reject. Original
spelling/case persists; shaping uses a canonicalized tag. TextStyle `dir` preserves
independent RTL run characteristics; `ltr` disables them without overriding strong
RTL characters. `complexScript` is a boolean (default false), retaining forced
complex-script selection independently of direction. These extend the existing
version-one profile under development, not a new canonical format.

DOCX resolves defaults, paragraph/character style inheritance and direct overrides
into these values, including explicit false direction overrides and independently
inherited language slots. Standard w:bidi/w:rtl/w:lang/w:cs exports them. Latent
font choices, theme/hint bindings and other unrepresented properties still
diagnose. PDF does not currently recover/persist these editing bindings.
Ordinary LTR PDF uses contextual run shaping and re-shapes final lines, with und
when the language binding is absent;
RTL/joining/bidi, context across differing styles and broader script/language
qualification remain required. Language attributes affect locale; they are not an activation flag for typography.


PDF paint-context shaping adds no schema fields. Source evidence may be
pdf_actualtext_aligned when decoded operator text exactly establishes mixed-style
ActualText intervals. Known text clipping uses rect_clipped_ precision prefixes;
fully_clipped_/unresolved_clip_/mixed_clip_ describe retained uncertain operator
bounds. Split-ligature logical carriers expose render_mode 3 and the existing
pdf_invisible_text diagnostic; their replacement_span_bounds are logical advance
bands, not visible glyph ink. Checked SF preserves these values and paint/link
marks. Broader bidi/font/language context and within-grapheme style changes remain
required, unfinished capabilities.


The 2026-11-24 internal paragraph bidi resolver adds no SF attributes or new
canonical representation. Existing paragraph/run direction bindings persist as
before. PDF integration is unfinished; algorithm conformance does not imply that
RTL SF now renders. Original source scalar ranges must remain separate from any
normalization or visual order during the next renderer implementation.


The directional-PDF milestone adds no SF schema. Existing paragraph dir/run
textStyle direction/language values now feed layout. Checked source/DOCX/PDF SF
examples validate 123 nodes. Original scalar ranges are internal, separate from
NFC/HB/visual indices. Nonzero-level logical carriers expose existing render_mode
3, replacement_span_bounds and invisible-text diagnostics. pdf_bidi_spacing warns
when geometric gaps are not inferred between logical bidi spans. Unsupported
scripts/contexts and PDF inferred structure/note recovery remain explicit open work.


## Running-story rendering without a second schema

Existing doc.attrs.headers/footers block arrays now render rich tables/merged
cells, images, lists and links in native PDF. Default text is 8pt/1.25; semantic
headings are bold. The renderer measures immutable stories, expands their body
exclusion area, fits competing image preferences and repeats them on every page.
Canonical nodes/assets/dimensions/row-prefix flags remain unchanged; no schema
revision or new editor representation is introduced. Page controls in running
stories reject, and existing graph rules forbid note anchors there. Page geometry now has four edges and running distances in one effective frame;
section-specific layouts/stories remain unfinished. DOCX emits standard parts.

A native PDF reimport retains placed text/assets and page geometry with existing
explicit/inferred provenance; it must not label repeated untagged content as
source-authored headers/footers. The letterhead example exactly preserves SF
through DOCX. Tight oversized image preferences return physical DOCX dimensions
per the existing rendered-format contract, and the initial LibreOffice output omitted one of its two placements. The shared
DOCX budget described below now repairs that omission. Broader DOCX font/producer
qualification remains required, separately from successful native PDF fitting.


## Shared DOCX running-image budgets

Existing header/footer block arrays now share a conservative DOCX raster cap and
body reservation. Smaller logos stay unchanged; larger rectangles fit without
altering original bytes, alt text, source preferences or structured relationships.
Body/note rasters use the remaining container. The source letterhead retains exact
SF/DOCX semantics; oversized picture preferences normalize to physical dimensions
on DOCX reimport under the existing rendered-format contract. Four new examples
have exact stable physical second round trips. No second schema or version change
is introduced. Fourteen new/refreshed SF files pass actual ProseMirror checks.

DOCX text envelopes use estimates and producer compatibility allowances, unlike
native PDF's actual glyph measurement. Broader font/producer/section qualification
remains required. Producer URI-export limitations are recorded separately from
preserved SF/DOCX links; neither fitted pixels nor plain text alone proves complete
rich-document conversion.


## Explicit page edges and anchors

The existing doc.attrs.layout object adds optional `margins` (top/right/bottom/left)
and `header_distance`/`footer_distance` point values. A present margins object
replaces all four scalar-margin edges; omitted members mean explicit zero, not
inheritance. A missing object retains the scalar fallback. Distance presence
similarly distinguishes zero from unspecified. Left/right and distances are finite
and nonnegative; top/bottom may be signed fixed body offsets. Width/height and
absolute vertical edges must leave a positive body area. Unused distances beyond
the canvas can persist, while an actual out-of-page running story rejects output.

```json
{"width":480,"height":420,"margin":17,"margins":{"top":70,"right":60,"bottom":55,"left":30},"header_distance":12,"footer_distance":16}
```

Here the retained scalar 17 is ignored for geometry. Both bridges deep-copy all
new pointers and preserve every field. New groups use 18pt default edge anchors;
legacy nil settings retain prior writer defaults. DOCX's standard page attributes
and checked ignorable metadata preserve binding presence in own round trips,
including zero and the unused fallback. External distances remain explicit. DOCX
physical coordinates normalize to twips; source SF remains precise. Native PDF
uses the effective bounds throughout body/stories/tables/notes. Signed offsets permit actual contained overlap; fixed running backgrounds paint
before body/notes. Out-of-canvas stories still reject.

This is an additive optional extension of the selected ProseMirror profile, not a
second schema or alternate canonical layout format. Current effective DOCX section
properties are read separately from revision snapshots. Unsupported advanced page/
story settings and multiple-section flattening diagnose. PDF provenance preserves
physical pages/quads, not inferred authorial margin preferences or anchor intent.


## Section extension (2026-11-24)

The same `manydoc-prosemirror/1` profile now accepts either `block*` or `section+`
at the root. Sections are not general blocks and cannot nest in bodies, tables,
lists, running stories or notes. A `section` contains ordinary block nodes. Its
attributes are `layout` (optional full Layout override; null/absent inherits),
`start` (`nextPage`, `continuous`, `evenPage`, `oddPage`; omitted means nextPage),
`firstPage` (boolean), `headers` and `footers` (objects), and optional provenance.
Each story object accepts only `default`, `first`, `even` node arrays. Missing keys
inherit that same variant; `[]` is an explicit blank. Null stories/maps reject.
Top-level `evenAndOddHeaders` controls even selection; first-page selection is per
section. Distinct story mode requires sections. Global root headers/footers cannot
be mixed with sections. Legacy flat content remains supported.

```json
{"type":"section","attrs":{"start":"oddPage","firstPage":true,"headers":{"first":[],"default":[{"type":"paragraph","content":[{"type":"text","text":"Chapter"}]}]}},"content":[{"type":"paragraph","content":[{"type":"text","text":"Body"}]}]}
```

Both bridges preserve and validate body/story nodes, assets, marks, notes and
layout pointers. Native PDF selects even stories using section pages counted from
one, while physical parity governs starts. Body packing uses a common conservative
frame across active variants. Advanced page bindings and broader packing/ownership qualification remain unfinished. DOCX preserves section intent through standard parts;
checked ignorable metadata restores implicit presence, never replaces conflicting
physical content. PDF recovery retains physical pages and source regions and does
not fabricate authorial section/layout/story bindings from untagged output.
`examples/sections` and `scripts/verify_sections.py` expose the four paths and
independent checks, with producer differences recorded separately.


Same-geometry continuous sections may now change story bindings without a new SF
field. The opened page keeps its earlier stories; subsequent pages use the new
bindings, counting the shared page as local page one. Continuous chains reserve a
common body plus their greatest body/note floor before painting. This affects
native packing, not source semantics or preference values. Changed paper dimensions now start a physical canvas, and same-paper horizontal
body frames may change within a shared page. Control/empty-section ownership and
conservative vertical packing still need qualification. `examples/continuous` preserves exact source SF through DOCX and exposes
physical PDF recovery separately; untagged PDFs do not establish authored sections.


Continuous layout overrides no longer reject for geometry alone. Native PDF uses
each section's horizontal body margins, retains common vertical reservations, and
opens a physical page when paper dimensions change. Already-opened running stories
and measured footnote chunks retain their original frames. No SF field/version
change is needed; exact source intent/settings still round-trip through DOCX.
Paper-driven native pagination is separate from logical `continuous` start intent.
Physical recovery records actual pages/quads, not inferred authorial section policy.


Signed vertical margins now implement their fixed-body meaning during actual
running overlap, without a new SF field/version. Positive margins remain minimum
reservations. Source preferences/asset bytes remain stable; native/Word page
placement and compositing are independently qualified. Fixed footer backgrounds
paint before body/notes, so physical PDF paint order differs from late-footer
paths and must not be mistaken for authored logical order. Shared-chain signed
edges retain their own absolute bound, and incompatible measured note intervals
finish before the new frame binds. Incompatible continuous story envelopes now
produce fresh native pages while preserving source continuous starts exactly.
Admission groups and page-owner state are renderer internals, not new SF fields.
Conservative packing and the full goal remain open.


Control/empty-section combinations now have targeted independent qualification:
relative starts apply after the first section, explicit page_break always
advances, and pageBreakBefore is conditional at body top. Continuous empty sections
may update pending ownership while the opened page keeps its original stories;
local section page counts include the shared page. Parity fillers are blank;
endnotes use the final effective frame. These are renderer rules, not new SF
attributes or inferred authored controls. Sectioned DOCX now declares the model's
existing decimal/continuous note numbering in every section, retaining exact
source SF including empty bodies/endnotes. Broader bindings and real exports
remain required; native and producer layout ownership can differ.


The original MQTT Word corpus independently verifies the current profile's
normalized paragraph/cell/merge/link/asset semantics. Its source diagnostics remain
in extracted SF; a caller-acknowledged normalized DOCX round trip matches the
profile after removing that source diagnostic record. This is not preservation
of all original numbering/layout/field semantics. No schema change is introduced
by the corpus checkpoint. Tagged PDF cells with missing spans still need
geometry-backed inference and per-attribute/continuation provenance; original
logical membership must remain distinguishable from inferred geometry.


## Additive table-span provenance

Cell source attributes can include `table_spans` with `colspan`/`rowspan` values
`explicit` or `inferred`, evidence, confidence, painted bounds and empty continuation
references (`element`, `role`, `tag`, `page`, zero-based `row`). Source content quads
remain unchanged. Logical membership remains explicit. These values round-trip through
SF and are validated by Go; the schema already permits the source attribute. See
TAGGED_TABLE_SPANS.md.


## Link target encoding at the PDF boundary

The existing link mark href string retains its Unicode IRI in SF and DOCX. PDF
URI-action strings use equivalent RFC 3987 ASCII/percent representations, and PDF import
retains that stored URI. Direct SF bridges do not rewrite targets. This is a
serialization rule, with no new mark/node/version; see URI_TARGETS.md.


## Physical table preferences

Optional table attrs.columnWidths is an array of positive finite point widths,
one per shared logical column; default null/empty means equal available columns.
It preserves subpixel source dimensions and avoids inconsistent per-cell widths.
PDF source.table_widths records measured grid evidence/confidence/bounds independently
from explicit membership. See TABLE_WIDTHS.md for editor adaptation and validation.


## Ordered-list numbering contexts

Optional ordered_list attrs.numbering maps to ListNumbering: level, template,
formats, ancestors, legal and independent. Fields are checked by the Go bridge;
templates use %1..%9 and preserve literals. Enclosing list counters refresh matching
ancestors during serialization, unless independent; fallback contexts cover resumed
lists. Source SF/DOCX round trips retain canonical semantics and implicit nil defaults.
See NUMBERING.md for supported formats, budgets and independent evidence.

## Named points and internal links

All blocks accept `attrs.anchors` (default []). `anchor_point` is a zero-height
block boundary. Inline `anchor` is an atom with `attrs.name`, optionally a source
mark, without text/format/link marks. `link` optionally includes `destination`,
separate from its interoperable `href`; the root optionally declares
`unresolvedDestinations`. Missing hrefs derive a fragment; present hrefs must refer
to that ID. Plain href fragments retain generic URI semantics. IDs are unique,
nonempty valid UTF-8 up to 1,024 bytes, and references require a definition or an
explicit unresolved declaration. SF/JSON/DOCX preserve the graph. Native PDF binds
standard named views after admission; PDF import retains explicit view provenance separately from inferred flow binding.
See INTERNAL_DESTINATIONS.md for range, point placement and repeat policies.

Root `attrs.pdfNavigation` (default null) stores PDFNavigation: explicit source
views and annotation links, including base64 name bytes, nullable source parameters,
display geometry and separately labeled flow binding/order. These values are
validated and deeply cloned in SF bridges. They are provenance rather than an
instruction to retain original page/zoom state after reflow; see PDF_NAVIGATION.md.

## Compact JSON and optional gzip transport

The profile and values are unchanged. WriteStructured and Write(SF) emit compact
JSON; ReadStructured/Read(SF) accept both compact and indented JSON. Write/Read with
SFGzip use standard gzip around exactly that JSON, with .sf.gz extension inference
and CLI extract -format sf.gz. ReadStructured remains a plain JSON API; callers
can wrap a standard gzip reader or convert through Read(SFGzip)/ToStructured.
Compressed and uncompressed inputs receive the same strict schema/graph checks.
See SF_CODEC.md for full-value/provenance proofs and retained allocation costs.

PDF page attrs now optionally include artifacts (PDFArtifact records); source node/
cell/list-item attrs and source marks optionally include artifacts ID arrays.
These identify explicit physical page/stream groups with their original properties
and displayed bounds. Membership survives nested marked content, Forms and ActualText;
Go checks IDs/pages/coordinates and clones records across bridges. Physical page roles
are separate from reusable header/footer templates. See PDF_ARTIFACTS.md.

Optional `attrs.notes[].label` retains a literal foot/endnote display mark. Empty
or omitted means automatic numbering by first reference order per kind. Literal
marks do not advance that automatic counter; repeated references reuse the mark.
Whitespace-only or multiline/tab/control labels are rejected. Label text is not a
note identifier; the existing kind/ID graph remains authoritative. SF/gzip/JSON
and supported DOCX round trips preserve it, including multi-character Unicode.

PDF note recovery adds optional note `source`, `label_source` and `kind_inferred`.
`label_source` retains the leading source label's own geometry, font/paint and role
when its text becomes the semantic note label; reflow typography normalization is
explicitly diagnosed. The latter
separates an editorial foot/endnote choice from explicit source NoteType; source
roles/regions remain explicit. Source marks/objects accept `structure_id` (original
binary PDF ID as base64) and `references` (array of `{element, targets}` explicit
Ref owners/object targets, including unresolved edges and aggregate contributors).
These are provenance, not a second copy of editable note bodies. Read PDF_NOTES.md.


Optional `note_reference.attrs.label` and note `body_label` distinguish inherited
(null/omitted) from literal overrides, including explicitly empty strings. These
change display only; kind/ID remains the relationship key and counter order is
unchanged. Per-reference run marks still carry citation formatting. See
PDF_NOTES.md for source recovery, Word's checked variant representation/automatic
editing loss, glyph and bidi behavior, and independent checks.
