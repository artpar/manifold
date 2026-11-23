# Architecture

## Canonical structured representation

ProseMirror JSON with the versioned `manydoc-prosemirror/1` profile is the chosen
persistence/editor interchange format. See STRUCTURED_FORMAT.md for rationale,
primary references and the actual supported subset; `schema/prosemirror-v1.json`
is its ProseMirror schema declaration. `structured.go` exposes the public tree,
strict codec and checked bridges to the existing Go engine. `.sf` and the `SF`
format use that representation; `.json` remains the legacy custom model.
The existing model is an implementation bridge, not the final scope of the SF.
Untagged geometric inference now exists; broader reconstruction and rich model
expansion remain required.

## Application boundary

`Read(context.Context, io.Reader, Format, ReadOptions)` produces a `Document`.
`Write(context.Context, io.Writer, *Document, Format, WriteOptions)` serializes it.
The model and format dispatch live in the root `manydoc` Go package so callers
can embed the library without importing a service framework or Daptin packages.

The initial document model contains paragraphs/headings with formatted runs,
links, lists, tables with nested blocks and row/column spans, embedded images,
explicit page breaks, metadata, headers/footers, notes and diagnostics.
Dimensions use points (72 per inch). JSON uses standard base64 for image bytes.

## Interpretation and fidelity

Extraction must preserve visible text of supported constructs, including text
inside hyperlinks, content controls, inserted revisions, text boxes and cached
fields. Deleted revision text is excluded from the final visible view.
Diagnostics disclose lost editing/presentation semantics. Formatting conversion
with outstanding diagnostics requires explicit `WriteOptions.AllowLossy`.
Unsupported writer features still produce errors; this option does not convert
unsupported content into silently missing output.

Plain text is an explicitly reduced representation. PDF extraction preserves
positioned text/raster assets and page geometry, without claiming an editable
Word layout. PDF output
renders the structured model; imported Word pagination is not authoritative.

## DOCX

`docx_read.go` uses Go archive/zip and encoding/xml. It resolves package
relationships inside the archive, supports optional package parts, rejects
duplicate/malformed paths and invalid document roots, and never fetches external
relationships. Table cells are public model values rather than opaque objects.

`docx_write.go` creates an OOXML package with deterministic member ordering,
styles, numbering, metadata, relationships, links, tables, embedded PNG/JPEG
images and page setup. The output is assembled before writing to the caller.

## Notes

`notes.go` validates the identity/reference graph and assigns per-kind display
labels by first reference order. Definitions are unique; inline `Run.Note` atoms
preserve reference sites independently of those labels. Draft definitions can
persist, but rendering requires anchors. Note bodies/running content cannot
contain note references, avoiding invalid DOCX stories and recursive note references.

`docx_notes.go` writes/reads standard footnote/endnote parts and per-part asset/link
relationships. Ignorable metadata attributes retain application IDs and definition
order without changing portable w:id values. First references get bookmark ranges;
repeats use cached standard NOTEREF fields. Bindings and metadata are checked,
then resolved to the same public note identity. Settings and section overrides are
combined before reporting numbering/layout losses. Generic fields retain their
existing cached-text/diagnostic policy.

`pdf_notes.go` reserves measured rich footnote fragments before painting
body/list/table references, including references inside nested cells. It draws
separators and labelled text/list/image/table bodies, draining overlong notes onto
continuation-only pages before resuming body flow. Text lines/images remain
atomic; connected table groups can supply smaller bands recursively. Endnotes use the body renderer
in a separate section. Internal links target actual note pages; independent
readers check destinations, not only labels/text. Oversized atomic fragments,
explicit cell/footnote page breaks and richer numbering/separator/layout behavior
remain required expansion.

## Lists

`lists.go` normalizes the legacy flat paragraph encoding without mutating callers.
Explicit `ListBlock` values own `ListItem.Blocks`; separate lists never merge.
`docx_lists.go` owns numbering definitions, style inheritance, counters/restarts,
nested-tree recovery and invisible bookmark ranges for precise exported list/item
membership. It validates ranges against numbering and rejects contradictions.
The temporary reader grouping nodes are not output content controls. DOCX limits
numbering depth to nine levels. `pdf_lists.go` measures markers, hanging indents,
body pagination and nested list lines inside table cells. SF uses the established
ordered_list/bullet_list/list_item tree with an expanded block-capable item rule.

## Shared table geometry

`table_grid.go` maps each cell origin to occupied row/column slots, with bounded
allocation and overlap/bounds checks. Covered cells are omitted from model/SF
rows. `docx_table.go` folds valid vMerge continuations into their origins and
recreates the physical continuation cells on output. Extra continuation content
is retained with a placement diagnostic. DOCX grid/cell widths use the available
page/parent-cell width rather than fixed grid widths that can overflow the page.
`pdf_cell_layout.go` measures cells into fragments containing text lines, images
or nested connected table-row groups. Measurement has no drawing/page-admission
side effects. It solves spanning-cell height constraints, then admits connected
row groups with their recursively collected note references and paints each cell
origin once. The same fragments drive footnote layout. Oversized body groups
now use `pdf_table_continue.go`: immutable active-cell cursors plan page bands,
consume each flow once and grow logical rows to finish their ending cells.
Interior borders respect rowspans; page fragments close at the page boundary.
Each band applies 6pt top/bottom padding. Before painting, page-local reference
keys determine minimum new-note reservation, including a fresh-page separator.
The original group geometry remains unchanged when it fits with note starts.
`pdf_fragment_continue.go` lets a table supply a smaller measured head and an
immutable cursor tail, recursively. Footnote starts reserve the minimum legal
band, including padding/final spacing. Only references in selected heads are
admitted. Remeasuring a remainder without a page limit accounts for added
continuation padding; a parent does not underestimate its child and close/reopen
the same merged frame several times on one physical page. Same-row bands can
combine into one page plan. Explicit cell/footnote page-break continuation and
oversized image handling are still unfinished.

`image_layout.go` fits preferred image dimensions to the current page/cell/list
container before deriving a missing dimension, preventing overflow for extreme
preferences. One dimension preserves intrinsic aspect ratio; no dimensions means
96 DPI. Explicit width and height specify an aspect ratio. DOCX and PDF use the
same fitting policy with their own padding. SF preserves preferences; readers of
rendered formats recover physical dimensions. Finite positive dimensions and
DOCX EMU conversion bounds are checked before output.

## PDF

The parser foundation is pdfcpu for xrefs, streams, object resolution,
encryption and PDF versions. manydoc owns a content-stream interpreter and font
decoding layer to preserve text from page-local font resources, ToUnicode maps,
Form XObjects, annotations and supported predefined encodings.

`pdf_geometry.go` composes affine page/graphics/text matrices. Pages retain raw
boxes/rotation/UserUnit and displayed physical dimensions. `source.go` validates
page consistency and positioned provenance on actual runs/blocks/cells. Quads use
font ascent/descent and advances, not glyph outlines. `pdf_font_metrics.go` reads
simple/CID widths and core metrics; uncertain bounds/advances are disclosed.
q/Q restore graphics/text parameters without rewinding text/line matrices.
Forms inherit their graphics state and apply Matrix; appearances additionally map
their transformed BBox to annotation Rect. Content-stream paint order persists.

`pdf_images.go` uses pdfcpu raster extraction plus Go image decoding to normalize
supported assets to PNG, applies soft masks/stencil fill and validates dimensions
before allocations. `pdf_inline.go` frames inline bytes by exact raw sample count
or the first filter's framing, never by scanning arbitrary binary data for EI.
`pdf_marked.go` records MCIDs/tags and replaces painted strings with ActualText
once, keeping contributing paint regions. A tag/MCID alone is not labelled
recovered explicit document structure.

`pdf_structure.go` parses the forward `/StructTreeRoot/K` graph after all pages
are painted. Content units are indexed by page, MCID owner stream and ID, or
object paint ranges. Logical containment consumes actual units once, detecting
cycles/multiple ownership/parent contradictions. Unknown/unclaimed content is
retained with diagnostics. RoleMap/ClassMap/local attributes resolve standard
paragraph/heading/list/table/image roles; table spans use the shared checked grid.
`pdf_structure_blocks.go` builds engine/SF semantics without a duplicate content
sidecar. Cross-page logical blocks have source regions rather than invented page
breaks. Source Stream/Role/Element keep paint and semantic provenance distinct;
list items own label/source metadata independently from paragraph provenance.
This walk does not yet validate the inverse ParentTree index or full PDF conformance.

`pdf_links.go` reads URI annotations and binds tagged Link/OBJR ownership, with
whole-span geometric fallback for untagged text. Partial/overlapping geometry,
conflicting logical targets and unsupported internal actions diagnose uncertainty.
PDF Info strings use PDF text encoding; XMP/extra metadata remain diagnosed gaps.
Simple-font ToUnicode padding/codespace repairs use authoritative one-byte font
codes, reject ambiguous/out-of-range mappings and disclose repairs.

SF persists page geometry in document attributes, block/cell provenance in node
attributes, list-item provenance in node attributes, and text provenance in a source mark (ProseMirror text nodes have no
attributes). It stores each content value once. Rendered output currently omits
source provenance and reflows, so Write requires AllowLossy for any source values
or pages even without diagnostics. Text/JSON/SF remain available directly.

`pdf_text.go` builds grapheme atoms across whole paragraphs, preserving run styles
and note anchors. uniseg v0.4.7 supplies Unicode 15 break opportunities; font/em
metrics supply proportional advances. Greedy wrapping selects legal boundaries,
then grapheme-safe emergency breaks for ordinary long words. Protected glue groups
and genuinely oversized atoms reject. Trailing boundary spaces retain logical
values at zero advance. Soft hyphens reserve/display a hyphen only at a selected
break; consecutive discretionary candidates are checked before emergency fallback.

Typographic spaces use explicit advances without relying on missing font glyphs.
NFC chooses supported visual glyphs while ActualText retains the original string;
uncomposable marks needing positioning reject. Drawn replacement spaces scale via
Tz within q/Q. ActualText is emitted around the logical span and retains contributing
paint geometry on read. Homogeneous style/font/baseline information survives;
mixed replacements disclose ambiguity. Explicit PDF 1.4 promotion changes one
header byte only, preserving serialized xref offsets. Page dimensions over 14400pt
reject until UserUnit output is supported. Text decoration lengths use measured
advances. Current font/shaping/table/section limits still apply.

The output foundation is gofpdf with embedded, licensed Go fonts.
Layout is explicit and limited; fonts/glyphs and unsupported layout features
must be checked rather than silently rendering corrupt text.

Font resources are keyed by their actual object identity inside a page, rather
than their local resource names. ToUnicode maps and common Adobe glyph-name
encodings are interpreted separately from PDF syntax parsing. Public PDF reads
turn dependency panics into errors and discard partial results.

pdfcpu receives a per-call configuration, never its default user-config loader.
Object parsing accepts common PDF version/extension conventions; page-stream
decoding is strict so corrupt streams cannot silently return partial text.
gofpdf receives per-renderer copies of font buffers because its subset generation
mutates them. Glyph inspection uses unchanged embedded bytes and local buffers.

The CLI uses these same Read/Write paths. It creates a temporary destination and
renames it only after successful conversion. It does not introduce a separate
conversion engine or format-specific behavior.

## Tests

Unit and composition tests should cover read→model→write→read, external-reader
compatibility, the previously discovered library defects, malformed documents,
context cancellation and independent concurrent operations. Independent Poppler
or Python tools may be used for test oracles; they are not runtime dependencies.

## Tagged inline-to-paragraph recovery

`paragraphChildren` groups consecutive direct content and ILSE children within
logical containers/cells. Explicit paragraph roles and child block containers
remain boundaries, including explicit blocks passed through inline wrappers.
`Span` ActualText replaces characters without asserting a paragraph boundary;
homogeneous marks/links/font metrics survive, mixed mappings diagnose ambiguity.
Implied paragraphs carry inferred provenance while actual MCID/tree ownership
remains explicit on contributing runs/items. Soft hyphens/zero-width boundaries
supply logical joins without extra geometric spaces. Region unions stay in each
physical page's coordinate system. Unsupported list-label fallback prefixes the
first body paragraph without collapsing later explicit paragraphs.

## Untagged geometric structure

When no StructTreeRoot exists, default PDF reads invoke `pdf_infer.go` after URI
association. It keeps original painted runs/assets, replaces only synthetic word
gaps, and applies bounding-box whitespace cuts before baseline grouping. Wide
vertical gutters order columns; isolated wide edge bands are separated first,
then the first horizontal whitespace separates remaining bands. This avoids a
large mid-body gap interleaving columns beneath a title/above a wide footer.
Relative font sizes suggest heading ranks; consecutive decimal/bullet markers
and indentation suggest list starts/membership/nesting/continuations. Aligned
row groups with repeated numeric-column evidence suggest table cells. Marker
columns are excluded from table inference. Wrapped-line alignment/gaps suggest
paragraphs within a column, without joining across pages.

These nodes carry inferred source evidence/confidence. Run provenance stays
unstructured, original marks/links survive, and image/page geometry is unchanged.
Sliced marker prefixes retain conservative original span bounds with disclosed
precision; original marker labels persist on items. Missing geometry or
nonhorizontal/bidirectional text keeps original line/paint groups. Overlapping
images/content and traversal limits disclose uncertainty. `PDFSkipInference`
retains untagged physical extraction; it does not disable actual logical tags.
Rectilinear ruling-based tables/spans now have a separate evidence path;
complex layouts and broad qualification remain open.

Core-font metric fallback resolves duplicate WinAnsi glyph names through canonical
CP1252 space/hyphen/bullet slots. Poppler-style aliases at NBSP/SHY/undefined byte
positions must not select absent pdfcpu metric entries. Explicit Widths continue
to take precedence. Unit advances and independent MuPDF string advances verify
this mapping; original font/advance bounds remain distinct from glyph ink.

## Painted ruling grids

`pdf_rulings.go` maintains a current path per content invocation, outside the
saved graphics state. Coordinates are transformed at construction time; q/Q
preserves the path while restoring graphics parameters. Painting consumes the
path, captures eligible centerlines and then updates pending clipping. Thin
filled axis rectangles can supply boundary centerlines; zero-area retraced
polygons cannot. Rectangular clips/Form BBoxes constrain these lines; unresolved
clip/mask/blend/transfer effects suppress uncertain boundary evidence. Opacity
zero is not a painted boundary. General vector appearance/text-image clipping
still has diagnostics rather than a false lossless-graphics claim.

`pdf_ruled_table.go` consolidates collinear segments, finds intersecting components
and checks a bounded rectangular lattice. Intersections use stroke thickness and
0.25pt geometric tolerance, ignoring only endpoint join nubs. Partially covered
edges reject the inferred grid. Missing internal boundaries connect primitive
slots; each resulting group must be rectangular with no contradictory painted
internal divider. The shared table grid validates emitted spans/coverage.

Cell content must fit a unique recovered cell; crossing spans preserve original
content with a diagnostic. Empty cells have explicit inferred boundary bounds.
Small grids resolve first, allowing nested-table ownership without duplication.
Text/list cell flow uses existing geometric grouping; images remain assets with
original positions. Failed grids block numeric reclassification of their content.
The SF profile already represents these values; no new canonical schema is added.
Border appearance, arbitrary curves/occlusion, complex grids and richer native
PDF cell layout remain required expansion/qualification.
