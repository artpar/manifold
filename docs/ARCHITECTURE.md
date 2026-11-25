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
`docx_breaks.go` resolves conditional paragraph page-start properties through
pPr defaults/styles/direct overrides. The public bool/SF attribute remains
separate from unconditional block controls. Exported controls use standard
pageBreakBefore plus ignorable checked roles: before, beforeList, control and
anchor. A list-boundary control restores outside the explicit list range.
Required empty-cell/table-tail paragraphs carry checked structural markers;
heading bold metadata preserves original mark presence despite default style
appearance. Metadata never authorizes dropping actual runs/content.

The final-visible DOCX text walker preserves positional-tab controls as normal
tabs and diagnoses alignment/positioning-base/leader loss with position_tab. It
rejects nonempty ptab nodes rather than silently dropping child text. This fixes a
real first/default-story export; it does not add Word tab-position layout semantics.
Original POI editor exports and source-only oracles qualify note/story/review/field
paths separately from synthetic cases; see REAL_DOCX_CORPUS.md.

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
readers check destinations, not only labels/text. Forced note breaks move their
remaining flow to continuation pages. Leading breaks delay the first destination;
empty break-only definitions target the final requested blank page. Trailing
breaks leave a fresh page that resumed body can reuse. Empty notes reserve no
separator height. A figure-bound minimum that cannot fit with its reference can
reserve a measured linked continuation notice instead. Its first actual body
remains pending with its destination unset until drawn; repeated references reuse
that destination. Larger figures fit the full continuation container, not leftover
reference-page space. Richer numbering/separator/layout behavior remains required
expansion.

Rich citation content lives on NoteReference.LabelContent, using ordinary SF text
nodes/marks. Canonicalization materializes shared outer marks into the fragments.
`pdf_citation.go` measures inner text/bidi at superscript sizes and keeps a single
outer note atom for pagination, preceding-anchor binding, reservation and navigation.
DOCX uses literal reference-styled hyperlink runs; `docx_citation_bidi.go` emits
checked LRI/PDI boundary runs where independent reader isolation is needed, restoring
only exact generated controls. Ordinary editable label text stays authoritative.
PDF structure recovery retains heterogeneous fragments and per-run source evidence;
physical alternate navigation remains separate from the editable note target.

`pdf_structure_note_actual.go` conditionally probes structural ActualText with copied
unit-consumption/reader-diagnostic state before note grouping. Exact one-paragraph
character agreement (excluding only synthetic geometry separators) permits note
references through enclosing/nested spans. The main traversal preserves literal
runs/styles/source evidence and checks correspondence again using resolved citation
labels. Opaque, nontext/multiple-paragraph or definition-containing ranges retain
replacement semantics and uncertainty. No equal-label substring mapping is inferred;
prepared atoms cannot be silently discarded into an incompatible replacement.

Independent source-nested definitions with main-flow citations become separate notes;
`noteOrder` restores logical preorder after child-first materialization. SourceInfo.
ParentElement records validated K/P containment independently of Ref targets.
`pdf_note_wrappers.go` recognizes only same-target single-inline-child ownership
chains. Their actual owner/target edges attach to the most-specific atom. Paragraph
joining aggregates both block and run source records, preserving even unconverted
wrapper Ref evidence. Conflicting/extra-content/opaque ranges retain uncertainty;
recursive editable citations remain excluded by the existing note-body contract.

## PDF running stories and body rectangle

`pdf_running.go` measures immutable header/footer flows once through the shared
cell/table planner with an explicit running style (8pt/1.25 default). Tables,
spans, nested lists, links and validated images share normal text/font handling.
The full story repeats, so table RepeatHeader flags are cleared only on temporary
measurement copies. Page controls reject; note graph validation already forbids
running anchors. No body cursor or pagination state changes during painting.

Legacy nil bindings retain header start margin/2 and footer start height-margin/2
(moved upward when necessary). Explicit bindings instead anchor header top and
footer bottom to page-edge distances. bodyTop/bodyBottom combine the effective
edge margins with measured story exclusions. Body
lines, images, headings, table continuations, note reservations/separators and
note-only continuation pages all use these bounds. There is one effective page geometry with four edges and optional running
anchors; section-specific stories are not yet modeled.

When running images compete, binary-search their measured budgets including each
ancestor's padding (up to forty probes per story); freeze plans that reach a real
image-space floor and continue with the other story. Never shrink text or mutate source assets/preferences.
A measurement-only body copy minimizes body-image preferences, materializes text
defaults and finds indivisible line/table/header/note starts, including later
note fragments. Running fitting reserves that floor plus .001pt for serialized
coordinate rounding. Initial validation errors/cancellation propagate; only the
specific too-small-image sentinel establishes the invalid low-budget bound.
Serial/nested images amplify budget reductions, so direct subtraction of the
whole story-height excess is incorrect. The bounded solver rejects nonmonotone/
nonconvergent or impossible layouts atomically.
Running-image SHA256 names reuse resources across callbacks; body image semantics
remain unchanged. The PDF backend can also deduplicate identical byte assets.

DOCX now has a separate conservative running-image planner; see below. PDF
recovery retains placed content/positions/inferred structures; generated PDFs do
not encode authored header/footer roles.

## DOCX running-image budget

`docx_running.go` builds immutable envelopes for both running stories, then selects
one maximum raster height using up to forty monotone probes. Images below the cap
retain their preferences; larger rectangles scale proportionally. Shared
`docxTableColumns` supplies the same integer widths as emission, and end-indexed
cell constraints solve merged rows without summing parallel columns. Per-cell
work is linear per probe; nested tables/required tail paragraphs and list indents
are retained. Text uses em-column/grapheme width estimates, hard lines and effective
sizes; it does not load or substitute PDF fonts. Foreign-font/Unicode DOCX export
therefore remains independent of PDF coverage. Cancellation is checked during
text/envelope traversal and fitting.

Existing image padding/note allowances and a producer compatibility floor reserve
body capacity; DECISIONS.md records its evidence. Body/note rasters consume the
resulting full container. Running parts temporarily use their original container
and the selected cap. No caller tree or asset is mutated. Geometry is validated
before package bytes reach the caller. Source SF persists preferences; DOCX
reimport returns physical fitted sizes, which remain stable on a second round trip.
Text-only stories retain existing serialization. Public geometry now supports four edges/distances through the shared page
resolver; section-specific body/stories still require model expansion.

This is conservative raster budgeting, not a Word layout/shaping engine. Rejecting
an exhausted envelope is not proof that every producer/font could never place it.
Broader font/producer qualification remains required. Native PDF uses its separate
actual font/glyph planner. The tested LibreOffice exporter emits no URI annotations
from an independent python-docx control either; source DOCX relationships pass,
while producer PDF URI preservation remains explicitly unqualified.

## Page geometry and provenance

`page_layout.go` resolves legacy Width/Height/Margin plus optional PageMargins and
HeaderDistance/FooterDistance. Group edges are explicit (zero included), override
the scalar fallback, and retain that fallback as persistent data. Top/bottom may
be signed OOXML fixed offsets; left/right and distances are nonnegative. Validation
requires finite settings and positive physical body area. Unused far distances can
persist; an actual out-of-page running story rejects at output. SF bridges clone
all new pointers. This extends the existing ProseMirror layout attribute rather
than creating another canonical representation.

`pdf_page_layout.go` centralizes body sides/width and anchor selection. Native body,
rich stories, continuation, note gutters/separators/targets share those bounds.
New groups use 18pt defaults and footer-bottom semantics; nil legacy bindings
retain established output. Signed offsets now also allow contained running/body
overlap, with fixed body edges and running artwork behind foreground content. Running images use the
page around their anchors, not an already reduced body region. DOCX emission uses
the same source edges and its producer envelope, with standard twip geometry and
landscape orientation. Format serialization checks reject empty rounded containers.

`docx_page_layout.go` reads standard numeric/universal measures and current section
properties. It ignores history/deleted/text-box snapshots as active sections and
reports editing losses. Advanced page/conditional-story properties diagnose their
missing representation. Checked md:layoutMargins/layoutMargin/layoutImplicit
attributes preserve binding presence and the unused fallback in own files; actual
standard edge/distance values remain authoritative. Foreign physical distances
are explicit. Optional metadata is never permission or authority over content.

PDF page/crop/source quads remain physical evidence. They do not establish original
margin preferences, header/footer roles or running-anchor intent; recovery must not
invent those settings. Asymmetric example bounds/rasters/targets are independently
verified, while the sparse native first page and producer pagination/separator
choices remain separate observations. This is not a Word-compatible layout engine.

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
combine into one page plan. A zero-height Break fragment stops selection and
returns a forced-boundary signal. Every cell is reselected at the first cut,
preventing siblings from painting beyond it. The flag propagates through nested
table heads; groups with controls bypass the ordinary whole-group fast path.
Aligned sibling breaks coalesce, while repeated controls in one flow retain
separate pages. Remainder measurement traverses all forced segments. A conditional
control is skipped only at the first flow position of the current page; page-start
state propagates through nested table selection and note continuation. Body/list
paragraphs handle it before admission, keeping markers and note destinations with
the paragraph. Images now fit the full ancestor-constrained height; broader
continuation qualification remains unfinished. Repeating headers are described below.

`image_layout.go` fits preferred image dimensions to full container width/height,
without cropping/resampling or enlarging that preferred rectangle. One dimension
preserves intrinsic ratio; no dimensions means 96 DPI; both dimensions prescribe
an aspect that may intentionally differ from intrinsic. Width fitting precedes
missing-dimension derivation, then height fitting preserves the resulting ratio.
PDF propagates ancestor padding/table gaps and note separator/heading budgets.
DOCX reserves 8pt for the image paragraph, 14pt per enclosing cell and
min(56pt, half the content height) in a note story; these are conservative image
budgets, not Word row layout. SF retains preferences; readers recover fitted
physical extents. Format-specific checks match actual coordinate serialization,
including zero-rounding and EMU bounds.

Before either writer registers/embeds a raster, configuration and full PNG/JPEG
pixels are validated with a 25-million-pixel allocation bound (also used by PDF
import). Context is checked before/after decode. Each write caches successful
config/kind by SHA-256 of bytes; decoded buffers are not retained and caches are
not shared. This prevents repeated decode during layout retries and a confirmed
malformed-alpha PNG dependency panic. Invalid assets never reach caller output.
The bound is technical resource protection; identity/admission/quotas stay outside
this library.

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
The reciprocal ParentTree pass now checks present inverse ownership before
materialization; missing information is diagnosed and contradictions reject.
PDF_PARENT_TREE.md describes bounds and recovery policy. Full PDF conformance is
not certified.

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


## Repeating table prefixes

`Row.RepeatHeader` / SF `table_row.attrs.repeatHeader` mark a contiguous prefix,
independently of semantic header-cell roles. Shared grid validation rejects gaps
and merges crossing the prefix/body boundary. DOCX uses standard tblHeader on/off
properties; noncontiguous flags and boundary-crossing merges import with explicit
normalization diagnostics, retaining visible content and merges.

For PDF, a table with a repeating prefix and body has one combined continuation
group. The measured header has original row offsets and cell fragments; copies
paint using those offsets rather than splitting spanning cells into row bands.
The pure planner selects a body band from an immutable cursor after reserving
header height, then includes header keys in page-local note admission. The
minimum nested fragment includes a header and a legal body band. Remainder
measurement includes one prefix for the next selected page. Body image fitting
subtracts measured header height before applying cell/ancestor padding. Header
images initially reserve a compact body line, then measured fragment minima can
reduce their budget and remeasure the table. Each retry requires a decreasing
header height and positive budget; unshrinkable text fails explicitly. Figure-
bearing prefixes qualify a first note for the linked deferral notice. No
repeated content is inserted into persisted SF or the caller's tree. Header note
markers retain their authored identity and link to one actual note body.

Repeating prefixes are indivisible; PDF rejects page controls inside them or a
prefix taller than the full container. Header-only tables paint once. Native
explicit body breaks repeat prefixes on resumed table pages, while Word suppresses
repetition for manual breaks. Real-producer/pagination qualification remains
required. Runtime dependencies and application admission boundaries are unchanged.


## Rich text style values and font resources

Run.FontFamily/FontSize/Color are primary style values; PDF Source.Font/FontSize
remain separate source evidence; PDF imports now also recover editable values. SF textStyle attributes use
one family name, numeric point size and #RRGGBB/auto. Existing defaults remain
unset; no global font substitution or Daptin admission rule is introduced.

DOCX resolves rich properties through docDefaults, paragraph basedOn chains,
character chains and direct properties. Export uses standard rFonts/sz/color.
Checked textImplicit flags f/s/c retain unset values on generated runs; readers
verify canonical inherited defaults and absence of contradictory direct properties
before restoring them. Word still sees normal styles/properties. Theme binding,
script-specific choices and embedded font omissions diagnose remaining losses.

Each PDF renderer owns a face registry and copied TTF buffers. Font programs
register/embed only on first use; generated note labels register their fixed Go
face when notes exist. All supplied programs still validate before layout. Go and Go Mono
provide regular/bold/italic/bold-italic; application data adds named faces through
WriteOptions.PDFFonts. PDFFontMap is explicit family selection, not an automatic
fallback. Missing actual styles/glyphs fail before caller output. sfnt validation
checks custom table bounds/name strings/cmap agreement and all glyph outlines
before the existing subsetter consumes data. No font files/services are fetched.

Text atoms use selected face advances and actual effective sizes. sfnt ascent/
descent can enlarge baseline/line reserves beyond normal em defaults. Mixed spans
share a line baseline; wrapping retains original logical Unicode. Paragraph/list/
cell/note/running-content placement admits measured heights. Decorations inherit
run color, then restore surrounding drawing state. PDF sizes match the engine's
0.01pt serialization; derived note sizes are rounded to that layout precision.
DOCX requires exact half-points. Header/table/image planners consume the same
fragment heights, including rich text inside repeated prefixes.


## PDF editable style recovery

`pdf_text_style.go` resolves descriptor FontFamily, then bounded embedded sfnt
name-table family metadata, then standard core families or a diagnosed PostScript
lookup identifier. This identifies a request, not an available output font. No
source program is silently reused as a configured face. Metadata decoding accepts
unfiltered/single-Flate streams within 32MiB, observes cancellation, and falls back
with diagnostics for malformed/unsupported metadata without discarding text.

Graphics state carries independent device fill/stroke spaces/components. Standard
operators and resource aliases resolve Gray/RGB/CMYK; saves/Forms preserve scope.
SourceInfo records family evidence, color space and effective components. Editable
RGB is 8-bit; quantization/CMYK/default-space approximation and unrepresented paint
or transform effects diagnose. Unknown general spaces leave editable color unset.
Physical vertical scale supplies point size; source matrices retain other geometry.

Content and structure-tree ActualText intersect styles independently rather than
assigning first-character formatting to a replacement span. Only homogeneous
paint receives shared source components. Synthetic gaps intersect neighboring
editable styles, without claiming original painted evidence. PDF numeric operands
are parsed as direct numbers; PDF object parsing of adjacent integers can otherwise
consume an RG operator prefix as an indirect reference. SF source marks declare
all new fields with optional defaults; prior field meanings/profile version persist.


## DOCX theme text color calculation

`docx_theme_color.go` separates Word run tint/shade from DrawingML palette
representation. Settings clrSchemeMapping targets actual palette slots once,
including logical text/background and accent/link mappings; swaps are valid.
Plain sRGB or sysClr lastClr supplies the base. Tint/shade modifies HSL luminance
while retaining hue/saturation, then rounds final RGB channels to nearest 8-bit.
Tint wins when both modifiers exist. Checked final RGB persists through existing
Run.Color/SF textStyle and standard DOCX RGB output; schema/API are unchanged.

Theme binding, system-dependent color choice and unused modifiers diagnose.
Calculated values take precedence over mismatched cached colors, with a warning.
Missing/unsupported palette constructs can retain a valid stored final RGB cache
with explicit uncertainty; they never apply theme modifiers twice. Without usable
cache, unsupported resolution rejects. Malformed RGB/modifiers/mapping values and
duplicate mapping nodes/attributes reject. DrawingML choices/transforms beyond
plain sRGB/sysClr, theme editing and script/font selection remain required work.


## DOCX font-selection import state

`docxRunStyle` wraps the public Run only inside the reader. Four independent
family/theme pairs, hint/language and forced complex-format properties inherit
without discarding unmentioned slots. Text leaves select effective families and
become ordinary public spans; identical families coalesce within the original
leaf. Links and nested block ownership remain intact. No new canonical font-map
format is introduced. Later language/direction model fields preserve those
bindings; unused selector/theme/hint losses still have diagnostics.

`docx_fonts.go` uses the published MS-OI29500 character/hint table as a declared
selection policy. Theme enumeration determines latin/ea/cs region; settings
BCP47 languages can select supplemental font script entries, with explicit
low-confidence/fallback diagnostics. Font-table charset participates in the
specific Chinese hint exception. Missing used theme faces reject without a usable
stored family; unknown application-default slots diagnose unresolved defaults.
Only used text needs a resolvable theme slot. Explicit cs/rtl selects complex
bold/italic/size; source RTL direction now persists through the later language/direction model;
native bidi now has targeted qualification below; broader coverage is unfinished.
DOCX output writes b/bCs, i/iCs and sz/szCs twins for explicit public values.

This is not a complete Word font-selection/layout engine: modern Unicode range
extensions, automatic complex-script assignment, ambiguous cs hints and producer
variants remain required expansion/qualification. Native PDF still rejects
unsupported shaping, supplementary glyphs and cross-style graphemes atomically;
source normalized spans survive SF/DOCX independently of that output limitation.


## Glyph-ID PDF transport

`pdf_glyph.go` provides the glyph resource/paint path that Unicode-codepoint
primitives could not express. Supported LTR graphemes retaining marks after NFC,
or containing supplementary codepoints, use pinned textlayout HarfBuzz. Positions
are em-normalized; advance and actual glyph extents feed the shared layout engine.
Fonts and buffers are renderer-owned, default shaping language is und, and no
process-global configuration is changed. Other unsupported paragraph scripts stay guarded; Hebrew/Arabic directional
shaping is now qualified below. A 1,024-scalar grapheme and 65,535-CID
per-face inventory are explicit resource bounds; offset coordinates are checked.

Painting uses explicit glyph origins under a shared quantized baseline, preserving
relative GPOS offsets. ActualText spans retain original logical text; CID keys
include glyph ID and logical mapping. The first component maps the grapheme's
UTF-16 string and continuation components map empty strings. Both ActualText-aware
and ToUnicode-fallback readers are independently checked. Full original TTF,
CIDToGIDMap, nominal widths, descriptors and ToUnicode streams are installed into
page resources by the existing in-process pdfcpu writer. Shared programs/resources
are not duplicated per page. Existing font admission/embedding restrictions remain
conservative; color/CFF/collection/font-variation expansion is still required.

Ordinary authored text now uses the glyph graph. Virtual controls and internal
fixed-Go note labels/captions retain primitives; control-only files skip rewriting.
Renderer font entries retain their catalog key to avoid linear lookup while choosing
between paths. Errors, cancellation and recovered dependency panics occur before
writing caller output. Dependency shaping observes bounded graphemes and context
checks at phase boundaries, not a claim of exhaustive adversarial-font safety.

ActualText component movement is not a logical line break. Noncollinear replacements
lack an established baseline; untagged inference retains paint order with uncertainty.
General PDF note/paragraph/reading-order recovery remains separate and incomplete.


## Language, direction and contextual LTR shaping

Block.Direction and Run.Direction are separate effective flow/RTL-characteristic
values; Run.Language/EastAsiaLanguage/ComplexLanguage retain all language slots,
and Run.ComplexScript keeps forced cs selection. The existing SF textStyle adds
these scalars; paragraph/heading attrs.dir carries paragraph flow. Checked standard
DOCX properties flatten defaults/style/direct inheritance; explicit false values
become ltr, independently from absent properties. Do not treat ltr as a bidi
override. Retained flags no longer independently trigger selector-loss diagnostics.

pdf_bidi.go now resolves paragraph levels in every story. The
path in pdf_shaping.go resolves adjacent weak scripts, shapes
compatible-face/size/language/script segments and measures actual HB clusters. Ligature advances are
apportioned among source graphemes only to estimate emergency breaks. Final lines
are freshly shaped and width checked, backing off on overflow; stale sliced glyphs
never paint. Notes remain attached to their preceding atomic anchor. Distinct
faces/sizes/language bindings/scripts/controls bound segments. pdf_glyph.go maps each resulting cluster
to its original logical text for ToUnicode while retaining complete-span ActualText.
General/EA/forced-CS language reaches the shaping buffer; empty language uses
deterministic und, with the same contextual shaping for ordinary authored text. Directional shaping and L1/L2 placement now exist as described below; context
across different faces/sizes/languages and broader scripts remain unfinished.


Decoration painting is independent of glyph/control segmentation: adjacent spans
with the same authored style share one continuous underline/strike stroke. Marked
special-space graphemes retain their previous glyph-positioned path. Original full
TTF overhead now applies to ordinary output; do not bypass shaping to reduce bytes.
A future glyph subsetter must preserve real GIDs/positions/logical mappings.


## Contextual shaping and paint intervals

pdf_paint_context.go compares resolved faces/effective sizes/canonical language
bindings independently of paint/annotations. It partitions shaped cluster geometry
back into authored paint intervals. Matching monotone GDEF ligature carets define
internal edges; otherwise equal original-grapheme advance is an explicit allocation
policy. A style boundary inside a grapheme still rejects. Shared group origins are
rounded once, preserving relative GPOS positions. Line fitting and final shaping
use the same context rule; emergency breaks never reuse sliced ligatures.

pdf_glyph.go clips repeated original glyph outlines at partial-ligature paint
boundaries. Such outlines are artifacts with empty ActualText/ToUnicode; unclipped
invisible space-glyph carriers supply each interval's logical text with a CID width
matching its allocated advance. This avoids MuPDF dropping clipped replacement
characters. Color/size/link geometry remains authored; recovered source exposes
render mode 3. Groups without split ligatures use outer ActualText. No glyph subset
or original font-program modification is introduced.

pdf_text_clip.go intersects known rectangular clips and axis-aligned font-advance
quads, retaining precision prefixes and baseline intersections. It diagnoses
complex/non-axis-aligned text, image clipping and fully clipped operator strings.
pdf_actualtext_align.go preserves mixed styles only on exact decoded/replacement
agreement; otherwise the conservative replacement path retains loss diagnostics.


## Paragraph bidi foundation and renderer integration

internal/bidilevels adapts the attributed x/text v0.30.0 BSD reference core to
retain resolved paragraph levels and expose line L1/L2 maps. The existing public
bidi API hides full levels and its Line method re-resolves sliced text, so it
cannot directly supply this layout contract. Existing x/text property lookup and
published Unicode 15 bracket data supply classes/canonical pair IDs. A stack
replaces quadratic isolate matching. Results use original scalar indices, are
owned copies, and allow concurrent queries. The adapter performs no normalization.

The PDF pipeline now carries levels/source ranges through textAtoms, shaping
segments, final-line re-shaping and placement. Glyph direction/L3/L4, logical
extraction and paint/link/note geometry remain separate.
The internal resolver's complete Unicode 15 ordering tests do not qualify glyph
rendering, script coverage or end-to-end bidi output. See internal/bidilevels/
README.md for source provenance, bounds, cancellation granularity and native-oracle
disagreements. No module/schema/public API change. Renderer behavior is qualified below.


## Directional PDF shaping and logical ownership

pdf_bidi.go records original scalar intervals, with separate P1 paragraph objects.
CRLF belongs to one preceding boundary; line separator U+2028 does not restart
paragraph resolution. NFC occurs only inside the shaper; normalized HB clusters
map back to original UTF-8 intervals. Notes occupy one U+FFFC source scalar before
label substitution. L1 is applied while spans are individual graphemes; complete
numeric levels bound shaping contexts. Final-line shaping remains fresh, including
emergency breaks. L2 computes physical span offsets, while content operators stay
in original logical order. Decoration runs sort by physical offsets; links use
the same placement. RTL paragraphs align within their admitted content width.

Run rtl supplies selected weak/neutral class overrides through the internal
ResolveWithTypes API; strong letters/numeric exceptions keep intrinsic classes.
It does not become a strong-letter override. Source run rtl also selects the
complex-language binding. Arabic/Hebrew glyph/mark/ligature/mirror output reuses
the pinned shaper and original TTF/CID transport; other script guards remain
required work. Script resolution is still adjacent-strong resolution within
compatible level/face/size/language contexts, not a complete itemizer.

For nonzero levels, visible glyphs are empty-logical artifacts and unclipped
render-mode-3 carriers hold complete logical text with allocated CID widths
(including zero). This avoids MuPDF losing words/reversing numerals under a
positioned multi-glyph ActualText. Source spans are logical advance bounds, not
per-character visible ink. RTL ligature paint allocation reverses clip sides and
logical caret order. Font programs are unmodified; no new dependency is introduced.

A list item's first paragraph chooses its marker side; an RTL footnote's first
paragraph chooses label/gutter/separator placement. Continuations retain that
choice. Inline reference labels stay atomic with the preceding anchor. Fixed-Go
note captions and broader note/layout/producer behavior remain separately limited.

Reader logical line state tracks bidi text. Adjacent ActualText spans on such a
line keep encoded strings without invented gap spaces, with pdf_bidi_spacing
uncertainty. Empty ActualText painting does not create logical gaps or line breaks.
Two exact fuzz regressions pin these rules. Untagged bidi structure still falls
back to paint-order groups with explicit uncertainty; general structure/note/table
recovery is not supplied by logical carriers.


Shaping items now pass the complete normalized paragraph/actual-line text and
item offset to HarfBuzz AddRunes. HB pre/post context selects joining forms across
face/size/language and transparent-control boundaries without forming cross-face
ligatures. Returned clusters subtract the item offset before original-byte mapping.
Notes remain U+FFFC in context; generated decimal label glyphs are always LTR while
placement follows the anchor level. Fourteen native vectors and seven two-font
PDF rows qualify these paths; cross-item positioning, full itemization and broader
font/script qualification remain unfinished. The existing LTR spacing/control
policy stays intact. The original DejaVu Sans Bold test-only asset has a pinned
hash and the existing license, not a new runtime dependency.


## Section bodies and running-story selection

`sections.go` adds root-only section containers, leaving flat documents compatible.
`documentSections` resolves nil layouts and independent same-variant story links.
`structured_sections.go` bridges them into the existing ProseMirror schema, whose
root accepts either ordinary blocks or sections. Running content remains node
arrays in section attributes, validated separately as editor trees. No parallel
canonical representation is introduced. Global stories cannot accompany sections.

DOCX properties at paragraph ends delimit the preceding body; their type controls
that section's start. `docx_sections_read.go` partitions before recursive list and
note resolution. The writer emits per-story parts/relationships and final or
paragraph section properties; checked structural tails restore exact SF membership.
Physical page geometry validates ignorable source-layout metadata. Reused source
story parts decode once. Shared note parts use minimum section image containers.

`pdf_sections.go` measures active story variants into a conservative common body
frame, starts physical parity pages, and selects stories by section-local count.
Pending plans install in the new page header callback after the old footer runs.
Automatic body/note pages retain current size/orientation. Footnotes finish before paged
section transitions; endnotes use the final section. Continuous sections
share a page when paper dimensions match and may change body margins/story policy. Chains
premeasure a common frame and shared body/note floor before painting. Opened pages
keep their owner; pending stories install on the next page with the shared page
counted as local page one. Changed paper dimensions drain notes and start a new physical canvas; horizontal
body frames update immediately on shared pages. Producer story selection/pagination and broader ownership/packing remain
qualification work; signed running overlap is implemented as described below.


Section planning clears stale body bounds while measuring a new geometry and
filters measurement note order by reference scope. Prior section anchors mark
cross-references as already materialized; unrelated note definitions cannot impose
another section's minimum. Endnotes contribute floors only to the final section.
Definitions/labels stay global, and temporary measurement state restores on error.
Continuous-chain body floors refit earlier pictures before unioning story bounds.
This remains conservative: every active variant contributes to the common frame.
Changed paper/body frames now have separate ownership as described below.
Control/empty-section edge cases still need qualification, without a Word-fidelity claim.

Section-note collection follows local reference order and a single first-anchor
index, avoiding a scan of every global footnote for each section.

Reference collection preserves encounter order and scopes measurement notes without
rescanning every global footnote for every section; the global endnote order is
needed only in the final scope. Measurement maps and geometry restore on errors.


## Physical page ownership and body frames

`sameSectionCanvas` groups continuous chains by width/height, not by all margin
or running bindings. Group bounds reserve vertical stories/body floors; horizontal
body coordinates remain section-specific and stable across a section's paragraph/
table continuations. Changed paper dimensions begin a physical page after old
notes drain. The native renderer never paints two page canvases into one page.

Each opened page captures its running-story owner. Footer painting temporarily
restores that plan's geometry before restoring the current body frame, so a later
shared-page margin cannot move an earlier footer. `pdfNoteChunk` carries measured
left/width through every split/deferred/continuation path. Notes use those values
for labels, content and links; their separator spans the participating frames.
The old single-frame path remains coordinate-identical. Common vertical packing
is deliberately conservative and still needs broader qualification, as do control/
empty-section owners and signed overlap. Native policies are independently tested
without claiming a Word-compatible layout engine.


## Signed edges and overlap paint phases

`prepareRunning` separates fixed signed edges from positive minimum reservations.
Fixed stories fit their own physical anchor interval; they do not shrink merely
to leave body space or avoid the opposite story. Out-of-canvas text still rejects.
Positive fitting keeps its previous allocation behavior. DOCX uses independent
fixed-story containers/caps in both flat and per-section writers; checked source
settings and original assets remain unchanged.

In continuous chains a signed edge retains its own absolute offset. Positive
edges reserve actual possible running envelopes. Incompatible pending note
intervals drain before a new frame binds, rather than moving already-measured note
fragments into a smaller interval. Incompatible common envelopes now partition
into feasible groups before painting; see continuous admission below.

Header backgrounds already precede body painting. A chain with a fixed footer
also paints its opened-page footer in that background phase, preserving body/note
foregrounds. Owner geometry is retained across continuous frame changes. Positive-
only footer behavior is unchanged. Raw paint order may change for signed footers;
physical source quads remain evidence, not authored running-story reconstruction.
Raster QA checks foreground ink and alpha samples alongside source/relationship/
asset and page checks. Producer compositing and pagination remain qualified.


## Continuous running-story admission

`planContinuousGroup` calculates candidate bounds on copies, refits positive
stories against the candidate body floor, restores signed edges and requires
space for that floor. A failed candidate leaves original independently feasible
plans unchanged. `planSections` first tries the whole same-paper continuous chain
so established compatible output stays exact. On incompatibility it greedily
extends feasible prefixes, marking each later group's first plan `freshPage`.
Errors from individual sections and cancellation still propagate.

Rendering uses the existing transition that flushes/drains old notes before
binding a pending page owner. A group boundary starts with the new section's
first-page story and local page one; compatible successors may share again.
`freshPage` is renderer state, never a mutation of canonical continuous-start
intent or DOCX section properties. Running assets retain their source sizes.
The frame strategy remains conservative, not globally minimal packing; fallback
prefix refitting may repeat measurements (quadratic work in an adverse chain).
Targeted control/empty ownership is qualified below; broader real-export
qualification remains open.


## Page controls, empty owners and explicit note numbering

Page controls and section starts are distinct requests. A body `PageBreak` always
advances; a paragraph `PageBreakBefore` skips at the first body position. Native
section starts are relative to a preceding section, so the first section opens
the document regardless of its start hint. Later next/parity starts open pages;
parity fillers use the preceding physical canvas and suppress both stories.
Explicit trailing controls may therefore leave a running-only page before a
next-page section. Source controls and conditional intent remain separate in SF
and DOCX (standard pageBreakBefore plus checked ignorable control metadata).

An empty continuous section can update the pending plan without repainting the
opened page. Each shared start counts as local page one; the next opened page
selects that section's local default/even variant. It retains the opened page's
header/footer owner until then. Footnote continuations drain before a subsequent
explicit/section boundary. Endnotes bind the final effective body frame even if
its section contains no body. Tests and independent physical page/annotation/
baseline checks qualify these specific combinations; this is not Word-equivalent
pagination. More general packing, note bindings and real-export coverage remain.

`docxNoteProperties` is shared by flat and sectioned writers. For document note
kinds present, every section emits decimal numFmt and continuous numRestart.
Previously the sectioned writer omitted these, introducing a default-numbering
ambiguity diagnostic on its own endnote reimport. The helper preserves flat output
exactly and changes only sectioned note properties. No canonical schema change or
reader diagnostic suppression was used to obtain the exact source round trip.


## Original Word corpus and independent oracles

The original MQTT 5.0 DOCX/PDF pair lives under testdata/real, with immutable hashes
and retained publisher Notices. `audit_mqtt_source.py` derives Word paragraph and
per-cell content hashes, merged origin coordinates, header prefixes, hyperlink
ownership and media identity directly from ZIP/XML. The Go regression checks all
59 tables/756 rows/3,009 origins plus complete normalized SF preservation through
DOCX. These are structural/textual checks, not Word-layout equivalence.

`audit_mqtt_pdf.py` independently records original forward tagged-table membership
and default spans, not inferred geometry. Its 90 tables/3,020 cells include empty
vertical continuation cells omitted from Word origin lists. Forty-two tables lack
span attributes, making current strict rectangular-grid recovery fail. Existing
painted-grid inference demonstrates recovery on an untagged local page-17 control;
merging that geometric evidence with authoritative tagged membership is now
implemented for unique page-local grids (see TAGGED_TABLE_SPANS.md). Missing
span attributes must not be confused with
invalid PDF objects, nor should the whole logical table be discarded to obtain a
passing plain-text extraction. Cell/attribute provenance and empty continuations now have explicit treatment;
short filled connector pieces now supply coverage along existing axes; no
row-constrained border completion is implemented. Native output also needs a font covering actual U+2A6D4;
missing-glyph rejection currently preserves atomicity, not full-goal completion.


## Tagged membership and painted span inference

`pdf_tagged_spans.go` reconciles explicit page-local logical cells with a unique painted
grid. Span provenance is separate from membership and keeps empty continuation
identities/original rows. Candidates use copies, bounded alignment and per-page grid
caching. Ambiguous grids retain explicit cells with uncertainty, not guessed padding.
Small filled rectangles provide coverage along established axes without creating
axes or connecting components. See TAGGED_TABLE_SPANS.md for contracts and limits.


## Glyph embedding subsets remain in process

font_subset.go builds a GID-preserving TrueType embedding program after shaping.
Iterative component closure/cycle checks retain used outlines and hinting, with original
metrics/cmap/name/license tables. Rebuilt loca/directory/checksums and deterministic PDF
subset tags replace full-program glyph embedding without changing CID mappings, text,
positions or caller bytes. Optional fontTools QA independently compares outlines and
metadata. No SF/public API/runtime dependency changes. See FONT_SUBSETTING.md for
source-font qualification and the newly reproduced three-target Unicode URI failure.


## PDF URI actions use ASCII references

pdf_uri.go maps Unicode IRI UTF-8 octets and excluded ASCII to percent-encoded URI
references at the existing LinkString call. Source models/codecs and DOCX retain the
IRI; PDF recovery keeps the stored equivalent URI. Existing escapes/delimiters and
distinct resource normalization forms are preserved. No URI resolution, destination
inference or public schema/API change is added. See URI_TARGETS.md for source-oracle and
reader evidence.


## Preferred physical table grids

table_widths.go validates and fits point preferences shared by DOCX containers and
PDF measurement/paint/continuation. SF table columnWidths extends the established
profile; docx_table_widths.go imports original grid preferences and checks metadata
against actual serialized twips. PDF source.table_widths separates measured grid
evidence from explicit membership. See TABLE_WIDTHS.md.


## Numbering definitions and owned counter contexts

numbering.go implements six standard formats and %1..%9 templates.
numbering_resolve.go resolves enclosing counters on owned containers for SF/DOCX/PDF
without modifying caller data. DOCX preserves logical levels and explicit contexts,
with independent sequences, Word empty-override disclosure, checked implicit markers
and legal-format metadata validated against visible decimal definitions. See NUMBERING.md.


## Word relationship/anchor URI composition

DOCX paragraph hyperlink reading now combines r:id Target plus #anchor using
Word behavior from MS-OI29500 2.1.521, with an explicit compatibility diagnostic.
The shared model holds the actual URI; canonical DOCX export uses that URI directly.
That URI milestone changed no runtime dependency/API/schema. Typed native
destinations and PDF recovery now follow below; see INTERNAL_DESTINATIONS.md
and PDF_NAVIGATION.md for current scope.

## Named navigation points (2026-11-24)

`destinations.go` owns the shared graph: block-start names, standalone zero-height
boundaries, inline atoms, typed internal references and explicit missing names.
`docx_destinations.go` pairs part-local numeric bookmarks, maps checked portable
names and restores points across structural paragraphs without creating content.
`pdf_destinations.go` binds during paint after admission and finalizes a standard
UTF-16 name tree/GoTo actions in the glyph-font rewrite stage. Measurement never
binds targets, and inline atoms never enter shaping. Running/header repeats use
first paint. Generic URI semantics and note identities stay separate. PDF import now retains typed physical views and separately inferred binding; see PDF_NAVIGATION.md.

## Physical PDF navigation recovery (2026-11-24)

`pdf_navigation.go` defines/clones/validates PDFNavigation provenance, independently
of the editable destination graph. `pdf_navigation_read.go` resolves byte-identity
names and direct arrays after pages exist, retains explicit views/actions/regions,
and binds points only after tagged/geometric recovery, with separate inference.
`pdf_links.go` uses typed URI/internal targets throughout both association paths.
SF root pdfNavigation and legacy JSON preserve physical evidence; formatting
output uses editable flow points rather than reproducing source viewer state.
See PDF_NAVIGATION.md for exact source checks and the remaining completion audit.

## SF persistence and compression (2026-11-24)

Plain SF writes compact JSON. SFGzip (.sf.gz) is the same checked JSON in standard
Go gzip, not a new canonical schema. The private reader returns both tree and checked
bridge so Read does not decode provenance twice; Write reuses ToStructured's checked
result. Public tree APIs still validate their inputs. structured_encode.go streams
body nodes; root attributes and individual node/asset values remain materialized.
Context/error checks cover encoding and gzip finalization. See SF_CODEC.md for
full original-value/provenance proof, independent ProseMirror checks and memory costs.

## Current audit and Colleaf boundary (2026-11-24)

ACCEPTANCE.md is now the current requirement audit rather than a cumulative list
of obsolete pending milestones. The actual Colleaf contract was inspected read-only
at pinned GitHub source and matching Univer 1.0.4 published declarations. Its .vdocs
snapshot requires an explicit native adapter through SF; it is not ProseMirror.
See COLLEAF_FORMAT.md. Application access/quotas/storage/conflicts stay outside
manydoc, and neither Colleaf nor Daptin was modified.

The audit's ActualText regression fix copies typed internal destinations alongside
hrefs and intersects URI/internal ownership as separate target kinds. Matching
internal IDs can share semantically equivalent href spellings; mixed targets clear
editable ownership with the existing ambiguity diagnostic. Physical annotation
records remain intact. Explicit PDF Note/Reference and pagination artifact semantic
recovery still remain next work, with reproducible independent source probes.

## PDF artifact provenance (2026-11-24)

pdf_artifacts.go records page/stream-scoped explicit marked groups. SourceInfo
membership links text/images and aggregates to PDFPage.Artifacts without copying
visible content. Default-user-space BBoxes use the page transform, independently
of content cm. Nested marked/ActualText/Form operations retain contributing IDs;
synthetic gaps retain only shared memberships. SF pages clone these pointer-bearing
records and source marks include artifacts. Header/Footer here is physical evidence,
not an inferred reusable authoring template. See PDF_ARTIFACTS.md.

## Literal note display labels

Note.Label / StructuredNote.label is an optional literal mark; empty selects the
existing continuous per-kind counter. A custom mark does not advance that counter.
Repeated references retain the same identity/label. The native footnote area and
endnote section both measure the literal mark through normal glyph/shaping checks.
DOCX uses standard customMarkFollows with same-run text nodes and a standard
footnoteRef/endnoteRef in the note body. One Unicode scalar per text node avoids
LibreOffice duplicating multi-character marks. Checked md:label metadata agrees
with the actual main-story mark, preserving identity independently of appearance.

## Explicit PDF note graph recovery

pdf_structure_notes.go resolves forward and reverse structure Ref edges against
the completed tree before flow materialization. Unambiguous consistent text marks
become reference atoms, rich definitions move once into Document.Notes, and only
matching leading explicit Lbl content becomes the semantic label. Definitions keep
Note.Source and independent leading-label geometry/font/paint in Note.LabelSource;
all source records can retain binary StructureID and owner/target Ref
edges, including unresolved relationships and contributing aggregate bounds.
Explicit FENote NoteType is separate from KindInferred editorial reflow defaults.
Navigation candidate traversal includes relocated note bodies. Read PDF_NOTES.md
for current evidence, precise fallbacks and remaining required note work.


## Citation and body label variants

NoteReference.Label and Note.BodyLabel are optional literal display overrides;
nil inherits and empty means markerless. Catalog identity/counters remain separate.
PDF recovery joins all explicit sites even with distinct marks, retaining the body
label/source independently. Unknown paint is not evidence of an empty mark.
Native body labels use normal font/glyph/shaping/width checks. docx_note_variants.go
writes standard note anchors/body bookmarks/literal hyperlinks with checked roles
and style/namespace/content validation. DOCX mixed automatic/variant export requires
AllowLossy for literal portable numbering, while checked metadata retains automatic
SF intent. Ordinary note representation stays unchanged. PDF_NOTES.md is the full
current behavior/qualification contract.

## Inferred note relationships

PDF note preparation now also recognizes matching immediate Reference/Note
siblings and logically owned physical GoTo views into a unique matching Note-label
region. NoteReference.Inferred persists epistemic ownership independently of
Note.KindInferred and explicit role/annotation/geometry evidence. The source record
carries rule-specific evidence/confidence; no synthetic Ref edge is manufactured.
Cache candidate/label blocks once so rejected candidates remain intact in ordinary
flow. Explicit edges outrank inference, ambiguous/unknown cases retain flow, and
physical matching has a cancellable work bound. The original published guide's
two real notes qualify the nonadjacent physical-link rule; PDF_NOTES.md records
source hashes, independent MCID/object/view checks and the editable-label-backlink
gap exposed by that source.


## Independent body-label template

Note.BodyLabelStyle is a text-free validated Run template; label resolution stays
in notes.go. structured.go uses existing ProseMirror marks for body_label_marks,
not a new inline schema. documentDestinationRuns includes template links. PDF
recovery splits LabelSource from the recovered common template before binding
navigation; citation atoms participate in candidate flow anchors. Native footnote
and endnote marker paths use normal fonts/shaping/measurement, with atomic tall
footnote labels and measured endnote minimum height. docx_note_style.go reuses
ordinary visible run/hyperlink encoding and parsing, with checked label/separator
roles, no second text authority, and a conservative derived gutter. See
PDF_NOTES.md for empty labels, exact checks and independent-reader limitations.


## Rich literal body marks

Note.BodyLabelContent / SF body_label_content is an alternative literal sequence
of ordinary text runs/nodes with existing marks, excluding string/template body
fields. Shared validation/catalog/source traversal includes each fragment.
PDF recovery retains heterogeneous Lbl runs and aggregate LabelSource independently;
normal font/glyph/bidi and marker measurement render them. Marker placement is
normalized to intrinsic advance before gutter painting. docx_note_rich.go groups
checked direct inline bookmark ranges, validates actual runs, and reuses ordinary
paragraph decoding without duplicate label text. Final-visible revisions/comments
use the ordinary paragraph policy; empty final marks normalize to explicit
markerless display with diagnostics. Both Word paths share the note
catalog; reserved names cannot steal user destinations. Ordinary paragraphs avoid
rich-range allocation. Read RICH_NOTE_LABELS.md for evidence and remaining scope.

## DOCX portable font-size precision

The dispatcher passes AllowLossy to both DOCX writers. docx_precision.go projects
font sizes to standard half-points, stores exact original intent on the actual run,
and restores it only after standard ordinary/complex size validation. The common
run encoder/parser covers all stories and checked note roles. No model clone is
mutated for quantization; original styles still drive the conservative envelope.
Both writers now initializeNotes through one catalog; the sectioned path previously
omitted variant/custom/style setup. parseXML rejects duplicate expanded attribute
names before interpretation. DOCX_PRECISION.md records the original-guide and
independent-reader qualification, precise bounds and remaining PDF precision policy.


## Colleaf native adapter foundation (internal, incomplete)

colleaf_stream.go maps UTF-16 positions to validated UTF-8 scalar boundaries and
parses actual Univer 1.0.4 paragraph/section/table controls. It preserves original
scalar sequences rather than normalizing before indexing. colleaf_body.go shares
that index to validate style/range/control metadata ownership. The internal AST
uses exclusive ends; native annotation/tree ends have distinct inclusive rules.
Context cancellation and existing nesting bounds apply. These helpers do not
replace the canonical SF or expose a partial native format/API.

Actual pinned Colleaf validator and DocumentDataModel/DocumentViewModel/table
parser probes qualify token values, control ownership, astral style/link/note
records and nested table bounds. They exposed stale table-token prose and the
need for CR/LF inside cells. Geometry is pixels; text fs is points. Full semantic
mapping, inherited styles/sections/stories, merged tables/lists/assets/notes and
native edit/save checks remain open. See COLLEAF_FORMAT.md.


## Private Colleaf text/style mapping

colleaf_styles.go computes a UTF-16 interval partition with an event sweep and
last-whole-record priority, resolves named basedOn styles with cycle/depth checks,
and projects supported typography with explicit loss diagnostics. Font fs remains
points. Native lang maps to the authored general language, while altLang/script-
specific bindings still require a checked mapping. Hidden text cannot become visible.

colleaf_text.go shares validated indexes for paragraph/heading semantics, link
ranges and native VT↔SF newline. It does not return partial rich containers or
multiple sections. Native text and styles are authoritative; no private whole-SF
payload exists. The SF href plus matching destination normalization is accepted.
These workers remain private pending rich ownership/asset/section/story mappings.
Tests cross the canonical SF and compare native records, verify style interval
winners/inheritance, and re-import actual pinned SDK TextX edit/save output.
Optional scripts/verify_colleaf_text.cjs is reader/mutation QA, not renderer QA.


## Private Colleaf rich body/table mapping

colleaf_tables.go composes actual stream/source records recursively. Native
vertical continuations have rowSpan0, a positive columnSpan and empty CR/LF cell
bodies; SF keeps only origin cells. Optional horizontal zero/zero placeholders
are checked against an owning span. A bounded occupancy grid verifies every
source/token ordinal, span and continuation before any document escapes. Visible
continuation content rejects. IDs and UTF-16 metadata shift together across
nested cells; table anchoring CRs do not invent extra SF paragraphs. Mixed native
text/table paragraphs become ordered SF blocks with an explicit diagnostic.

The reader partitions effective text styles once and uses indexed intersections
for cell paragraphs; it does not scan every global text run for every paragraph.
Widths use actual native geometry: legacy unit0 POINT and unit3 PIXEL table
values are both consumed as pixels by the pinned executable table layout.
SF point widths multiply by4/3 for native unit3. Equal default columns are native
specified percentages totaling100, not zero automatic widths (which rendered
invisible in the pinned skeleton). Named formatting stays in the shared text
worker. Actual record/tree/geometry/edit/save probes are in
scripts/verify_colleaf_tables.cjs; font metrics/browser rendering remain open.
This internal composition is not a public partial native format support claim.


## Private Colleaf note graph composition

colleaf_notes.go composes the shared native text/table workers for main and note
bodies. Note reference atoms occupy one UTF-16 BACKSPACE control and carry real
customBlocks noteId/noteType metadata; interval styles remain native. Definition
bodies/tables live in real notes records. Repeated references share one definition.
The internal manydoc.note-identities.v1 resource stores native-ID/kind/SF-ID bindings
and original definition order only. It duplicates no visible text or labels.
Bindings apply only to matching actual native ID/kind; stale bindings diagnose
and leave actual native identity authoritative. Definition aliasing is rejected.

colleaf_text.go isolates atom boundaries alongside style/link ranges; the rich
body reader translates custom-block metadata through paragraph/cell fragments using
sorted atom positions. A matching leading body-marker atom maps to BodyLabelStyle;
nonleading/self/foreign citations still fail note graph validation. Native shared
customMark and showBodyReference=false map to existing SF label/body-label intent.
The internal worker rejects unmapped independent/rich labels, navigation/provenance
and assets/lists rather than dropping relationships. Complete note settings, marker
defaults/intent and browser/native placement qualification are still needed. The
public API/CLI and runtime dependency graph are unchanged.


## Private Colleaf list composition and ownership

colleaf_lists.go emits actual paragraph bullet records, named list sources and
nine-level native glyph/counter definitions. SF starts map to native startNumber-1;
shared group IDs bind non-independent nested counters, while separate groups keep
independent counters. Native templates/levels/styles remain the real presentation
fields. The same owned body writer handles main, note and table-cell list content.
Canonical numbering contexts resolve on copied containers before encoding, matching
ToStructured; caller fallback ancestors are not mutated.

manydoc.list-ownership.v1 stores per-story list type/group IDs, first/last paragraph
IDs, item bounds and implicit/independent intent. It stores no item text or SF body.
Bounds must match actual paragraphs and item bullets, nesting and counters before
reconstructing containers. Stale bindings cannot override native restarts/group edits.
Foreign lists infer only marked paragraphs/nesting with explicit ownership uncertainty;
unmarked content remains visible. Native list source/counter interpretation occurs
once per body; ownership maps are shared across cell sections.

Inactive deeper placeholders are omitted from native glyphFormat (the executable
formatter otherwise dereferences absent levels). Sparse original-template intent is
restored only while its projection equals the actual native format. Literal percent
syntax and nondecimal zero formats differ: private workers reject rather than emit
wrong/crashing markers. Full policy/intent mapping remains open. SDK QA inspects
actual GlyphType.LIST content, not only the mutable counter history cache. Browser
layout and full editor resource persistence remain separate required evidence.


## Private Colleaf embedded image transport

colleaf_images.go composes actual inline drawing atoms, embedded data-URI sources,
per-story drawings/order and pixel docTransform boxes. All occurrences have distinct
IDs including a story prefix: the actual drawing registry is flat across notes.
The shared body encoder now returns a typed composition containing text/tables/
lists/assets/checked intent rather than dropping extra maps through narrow tuple
helpers. Narrow helpers explicitly reject assets they cannot transport.

Point preferred sizes project to pixel boxes; omitted dimensions derive the
intrinsic ratio. manydoc.image-geometry.v1 stores only asset digest and checked
original/native size intent, not another image/text payload. Original precision/
omission is restored only when current bytes and frame agree with the computed
projection. Asset/resize edits invalidate stale hints and retain actual geometry.
Image and output validators share the existing raster expansion bound/cache.
No app-specific5MiB/2MiB quota or network/hosting dependency is introduced.

The rich paragraph decoder separates native image atoms from styled text/note
atoms. Mixed inline flow has explicit grouping diagnostics, hidden anchors reject,
and leading paragraph break-before remains an actual paragraph property. Linked,
floating/cropped/rotated/hidden/translucent appearance still requires mapping before
public native support. Node pinned reader/drawing scope/skeleton/save checks and
Pillow byte/pixel/alpha checks qualify basic asset transport. Browser paint,
contextual fitting and editor resource-manager persistence remain open.


## Native intent persistence through the actual facade

colleaf_intent.go replaces unregistered-resource output with a versioned CUSTOM5
annotation on the final LF: profile manydoc.native-intent, version1, records holding
only the existing checked identity/ownership/geometry intent. FDocument.save
regenerates registered resources and drops unknown ones, so model getSnapshot
round trips alone were insufficient. The Go reader opens the carrier before
ordinary range mapping, verifies scalar bounds/version/record names and rejects
duplicates. Direct resources remain readable for older private snapshots.

The registered DOC_DRAWING_PLUGIN resource is ignored only as a verified subset
mirror of real persisted main/note drawing sources and valid unique order IDs.
Contradictory descriptors and unbound entries reject rather than choosing a hidden
second asset authority. Empty word/note plugin resources do not create false loss
diagnostics. DOC_OBJECT_PERMISSION_PLUGIN is outside document interpretation;
its payload is never evaluated and introduces no admission/permission gate.

scripts/verify_colleaf_facade.cjs creates real Univer/Docs/Drawing model plugins,
uses actual FUniver.createDocument and FDocument.save, reopens in fresh instances,
and tests TextX edits. It deliberately reproduces the old resource-drop failure.
Go verifies full mapped model semantics after saved/reopened fixtures. Browser
painting, preset-specific UI behavior and drawing facade mutations requiring the
render-manager service remain separate, unfinished qualification.


## Native document/page/section and running-story composition

colleaf_sections.go composes the private native workers through the shared Go/SF
model. It uses one main UTF-16 body writer/index across root sections and separate
body indexes per running story. Section closing LF records carry current section
properties; native sectionTypeNext can override the following start. Points project
to pixels (4/3), and partial native geometry inherits the previous effective layout.
Checked document-properties intent retains original optional/fractional layout,
implicit starts, empty-story/section distinction and author/subject metadata only.
Actual geometry changes invalidate old layout intent; visible title stays native.

Running-story IDs distinguish nil inheritance from explicit empty bodies. Header/
footer tables/lists/drawings are promoted to document catalogs with globally scoped
identities because the executable renderer does not resolve those local catalogs.
The reader subsets global catalogs per actual body relationships, retaining orphan
checks across the union and within local catalogs. Main, running and note streams
share semantics without confusing their scalar offsets or source ownership.

Explicit form feeds become PageBreak blocks; renderedPageBreaks designate disclosed
soft pagination boundaries omitted from logical flow. Native first/even/default
references, page distances and zero margins are explicit. Output disables native
automatic hyphenation rather than accepting facade-added defaults silently. Foreign
nontraditional/unpaginated and note-setting policies remain unfinished and reject;
unknown style properties retain content with explicit representation diagnostics.
The pinned seven-page skeleton and real facade save/reopen qualify source lookup,
geometry and story selection, separately from unfinished browser/font/paint checks.


## Native frame bounds and executable note citations

colleaf_image_fit.go bounds individual frames after document composition. Shared
fitImageBox preserves the preferred rectangle's aspect without enlarging. Main
sections have distinct body boxes; table grid/span recursion accounts for native
10px horizontal cell margins, nested containers and conservative height allowances.
Shared notes use the narrowest actual citation section, with a separator/reference
allowance. Inherited running frames use the narrowest referenced section and
available running band (physical fallback for zero margins). These bounds do not
claim to measure complete native text/story height; the editor owns those metrics.

Only changed frames gain a container in manydoc.image-geometry.v1. Re-import
recomputes the fit from preferred size/container and checks source digest/native
box before restoring optional dimensions. Real asset/resize edits invalidate it.
The body/native catalog remains the sole source of image bytes/relationships.

The native renderer consumes FOOTNOTE8/ENDNOTE9 custom ranges covering one U+FFFC
atom, not the old custom-block note projection. Body indexing validates each
range/token/definition relationship and derives semantic atom entries internally;
paragraph fragments preserve ranges. Hyperlinks remain a separate range mapping.
Generated repeated/automatic notes use real customMark strings because native
repeat counters and implicit Roman endnotes differ from SF shared decimal labels.
The checked identity binding stores only the missing automatic-label intent; it is
restored after complete body assembly when actual mark and citation order agree.
Legacy private custom blocks still read. Foreign automatic settings and advanced
marks remain unfinished. Pinned skeleton glyphs/placed note pages, frame boxes and
actual facade save qualify the correction; browser font/paint is still separate.


## Native automatic numbering resolution before semantic decoding

colleaf_note_numbering.go resolves modern native note ranges against root section
ownership and document/section note settings before body mapping. Independent
per-kind counters follow scalar reference order, reset only under the active
restart policy and exclude custom marks. Default footnotes are decimal; native
endnotes are lowerRoman. Zero decimal/letter/Roman outcomes and supported symbol/
Unicode outcomes match the executable pinned reader, including its parenthesized
ideograph result for the format named ideographEnclosedCircle.

A plan binds range IDs to citation overrides and note IDs to first/final marks.
Semantic import stores current literal Note.Label, differing NoteReference.Label,
and independent/empty BodyLabel values. It emits explicit numbering/settings
normalization diagnostics because SF does not retain live native counter, column
or placement policies. Generated custom marks with checked autoLabel intent retain
their existing semantic restoration; legacy private block input still reads.
Source property nulls, malformed enums/ranges and unresolved automatic per-page
counters reject. Editable separator/notice bodies reject pending visible mapping.

The same import works in whole-document composition and the private note worker;
whole-document assembly defers numbering until all sections are available. Source
range IDs survive paragraph fragments. Number/body lookup is linear in notes and
references. Uncaptured section properties survive raw snapshot marshaling for real
reader checks, while SF interpretation still diagnoses/rejects representation loss.
Foreign omitted running distances now follow native constructor defaults 30px;
explicit own projection values are unaffected.


## Native editable body-mark projection and recovery

colleaf_note_body_mark.go projects existing Note.BodyLabel/BodyLabelStyle/
BodyLabelContent into real text/style/link records at the beginning of the note
story. It prepends to the first paragraph/heading or adds a dedicated paragraph
before table/list/image/empty content. A terminal tab separates the mark from body
text. Generated native markers are suppressed with showBodyReference=false.
The local manydoc.note-body-mark version1 annotation stores only mode, native note/
paragraph IDs, separate/default/empty flags and neutral-render settings. The root
identity binding retains the expected range ID to diagnose metadata loss. There is
no duplicate label text, style cache, note body or SF payload in that annotation.

Univer CUSTOM ranges style text by default. active=false plus properties.
textColorMode=text makes this ownership annotation neutral; actual SDK glyph tests
reproduce the former blue/underline override and verify the correction. Import
rejects an unknown/nonneutral ownership version rather than silently interpreting
presentation it cannot preserve. Local annotations survive actual facade saves.

Recovery removes only guarded leading marker runs and their owned delimiter from
ordinary decoded note text, deriving actual rich marks/styles/links from those
runs. Paragraph identity plus the terminal tab survive TextX replacement even
when the annotation start moves to the delimiter. New text/styles remain current;
changed/default-style representation becomes rich label content with diagnostics.
Separate marker paragraphs disappear only if no new body text remains. Missing/
stale ownership leaves actual visible text in the body and discloses the loss.
Empty styled marks use the real referenceTextStyle and a terminal-LF annotation.
Per-citation rich output and broader nonvisual intent remain separate unfinished
workers; this body-mark layer does not expose a partial public native format.


## Native per-citation label projection

colleaf_citation.go projects a note group with per-citation overrides/rich labels
into markerless real note atoms, editable superscript label runs and U+2060 end
delimiters. A neutral CUSTOM annotation stores only profile/version/token/note ID/
mode/inferred flags. It does not cache label text, styles or a second note body.
The original shared label/automatic intent remains in the checked identity record;
real independent/default body marks use the existing body-mark worker. This avoids
duplicating note definitions when several labels point to one note.

Native TextX can split a CUSTOM range around replacement text. Identical copied
tokens/properties combine fragments; original identities/scalar bounds validate
before combining. The protected atom and end delimiter define current label text.
Effective native style spans preserve last-record-wins precedence. Only guarded
superscript 3 label spans are semantically normalized to ordinary run styles before
folding them into a NoteReference. Neutral baseline 0 is ordinary SF text; unowned
nonzero baseline offsets still diagnose representation loss. Binary searches avoid
rescanning all style records per citation, and paragraph folding consumes each
run without repeatedly copying its whole remainder.

Final semantic assembly derives labels/styles from actual source runs, restores
implicit shared/default/body labels only when current values agree, and retains
explicit overrides after edits. Missing/stale ownership preserves visible label
text and markerless anchors with diagnostics. Baseline/shape/relationship changes
do not justify hiding text. Nested cells/lists share the root source-index context;
note bodies do not accidentally reuse root citation IDs. All of this remains a
private adapter pending broader intent, strict envelope and actual browser/API QA.


## Private Colleaf native file boundary

colleaf_codec.go wraps the rich section/story/note workers in the actual pinned
v3 .vdocs envelope; it does not add a public Format or rename SF. Strict lexical
validation runs before typed decoding: duplicate decoded keys, invalid UTF-8,
unpaired surrogate escapes, case aliases in known typed records, null scalar
values and excessive parser depth reject. Raw native property objects still use
their existing semantic checks/diagnostics. Interpreted JSON resource strings are
also validated before their maps can be collapsed. Large strings scan in place;
key sets/schema maps allocate. Input is buffered and broader allocation auditing
remains unfinished. No Colleaf snapshot/image budgets are runtime library quotas.

Known native settings accept empty objects or finite positive zoomRatio; view zoom
and nondefault editor locale get explicit loss diagnostics because logical SF has
no editor-view state. Unknown settings/typed fields reject. The actual facade's
added empty settings object is a retained save/reimport regression. Exact envelope
validator and rich facade/Go/SF tests qualify structural transport only; remaining
intent mappings, public API/CLI, browser/preset interaction and final corpus audits
still gate native adapter completion.


## Native list template grammar projection

colleaf_list_template.go keeps SF's single-digit %1..%9 grammar independent from
native glyphFormat's whole-index grammar. Fixed-symbol native source entries at
index9+ carry literal percent and digit suffix fragments; real paragraph levels
stay0..8 and number counters remain native/automatic. This also works when all
nine real levels participate. Helpers deduplicate identical literal fragments.
There is no extra SF schema, per-item marker cache or application dependency.

Import checks current format/helpers before restoring sparse original template
intent; otherwise it derives the current representable SF template. It compares
symbolic parts to avoid turning a literal percent-digit fragment into an active
counter. Numeric glyph types with glyphSymbol now import their actual constant
marker. Unrepresentable/overlong/malformed helper templates reject explicitly.
SF512-byte template limits bound helper projections and expanded parsing, without
copying app quotas. Pinned renderer and facade tests qualify main/nested/cell/note/
header marker content and counter/helper edits. Browser/preset qualification and
zero/legal mappings are now described below; broader marker policies remain unfinished.


## Native zero/legal numbering projection and import

colleaf_numbering_projection.go maps SF's existing zero/legal convention without
changing canonical SF. First nondecimal-zero items use a decimal type variant;
normal items keep their number format in the same native counter group. Ancestor
zero fields and legal decimalZero fields project to decimal as needed. Sparse
format/zero-level intent and per-item type aliases preserve source SF semantics;
there is no fixed marker text/per-item body cache. Running-story promotion scopes
alias and projection keys along with the native catalogs.

Import checks current native glyphs/legal flags/actual zero counters before
restoring intent. Stale projections use current native data with diagnostics.
Container assembly compares each item's effective numbering and scoped type;
foreign native00/empty nondecimal-zero markers or mixed padded legal rules become
explicit SF templates/effective formats, splitting groups when semantics change.
This retains current presentation and discloses lost live policy. Canonical sparse
integer member names and bounded format/zero indices prevent map-key aliasing;
source/ownership contradictions reject. Policy diagnostics aggregate in stable
order. Primary-binding deduplication avoids revalidating all projections for every
alias. Native browser/preset appearance/live-edit parity and broader resource/API
qualification remain on the required queue.


## Registered native hyperlink URL resource

colleaf_hyperlink_resource.go interprets DOC_HYPER_LINK_PLUGIN before ordinary
main/running text. The pinned plugin applies URL payloads to global hyperlink
rangeIds in main/header/footer bodies, excluding notes. Export prefixes running
link IDs by story. Import follows actual resource precedence over missing/different
body URLs and discloses it; inactive identities also get diagnostics. Duplicate
resource/entry/global IDs, invalid shape/Unicode/member aliases and conflicting
external/internal targets reject. Only modified ranges/story containers are copied;
caller snapshots remain unchanged. Other plugin resources retain their existing
handling, including the application-owned permission boundary.

The strict JSON scanner's shared typed entry point validates the nested interpreted
resource before decoding can collapse keys or replace text. Actual plugin save/
reopen and a resource-only header edit prove Go/SF transport across main/cell/list/
header/footer and note-local URLs. Hash-pinned source-only execution of the actual
external-link handler, with real loaded models and a window.open stub, qualifies
target selection logic. It does not load UI widgets or qualify browser gestures,
popup behavior, paint or internal navigation. Actual bookmark/heading point mapping
and remaining intent/API/browser/resource work still gate adapter completion.


## Native bookmark points and typed destination graph

colleaf_navigation.go maps real BOOKMARK3 records to the existing SF paragraph/
heading anchors and inline anchor atoms. Zero-width native points validate their
UTF-16 start boundary independently of inclusive formatting extents, including
before astral scalars. Rich paragraph fragments carry point records separately
from hyperlink intervals, preserving overlaps and end-of-paragraph points. Sparse
version1 role/paragraph-ID hints preserve authored block ownership only at its
current valid start; moved points retain actual positions with diagnostics.

Export transport IDs avoid logical and unresolved names as well as native range
identities. Import resolves main bookmark aliases, maps headingId through scoped
SF identities and normalizes equivalent fragment URLs to typed destinations.
Document intent retains explicit unresolved IDs; final graph validation recovers
new missing targets and rejects points that rich interpretation did not represent.
Range-to-point editing extent, unknown metadata and main-only native target lookup
limits are diagnosed. Diagnostic ordering is stable.

Actual registered-plugin save/reopen and TextX mutation, strict byte-codec/SF
round trips and1,024 point-bearing paragraphs qualify this component. Hash-pinned
unmodified handler source issues twelve requests through the actual selection
manager, with a stub render manager. This proves target/event logic, not rendered
caret/scroll/browser interaction. Standalone/container anchors and structural
cursor projection remain unfinished, alongside broader intent/API/UI audits.


## Native table/image navigation points

The rich body reader classifies bookmark starts against validated native table
and image indexes before paragraph fragmentation. These zero-width points attach
to the existing SF Block.Anchors on the current table/image, preserving nested
and story ownership without consuming them into a temporary empty paragraph.
Export uses real token offsets with sparse table/picture ownerId hints; header/
footer table catalog promotion rebinds hint IDs with actual table IDs. Native
edits and hint-free foreign points remain authoritative. Stale placement, unknown
metadata and range editing extent losses are explicit. Other unrepresented
structural controls still fail final graph validation rather than losing anchors.
The optional pinned container-navigation probe qualifies actual plugin/save/edit/
selection-refresh behavior and full Go/SF tree/assets/relationships; browser UI
and remaining standalone/list/section/page-break anchors are unfinished.


## Native flow-owner points

List hints carry the first native paragraph ID. Inline points are promoted to
Block.Anchors only at the current first paragraph/start of a reconstructed list;
other points retain current inline positions and get stable stale-placement
diagnostics. Page-break points are classified before text fragmentation, using
real form-feed controls. A discarded rendered soft break retains a zero-height
SF point. Explicit sections use checked current section-ID/start bindings and
avoid flattening away section anchors. Empty sections remain independently owned.

Whole-document encoding validates the complete destination graph first. Its
preliminary first-section note projection temporarily declares only links whose
anchors were omitted by that projection; the final native intent uses original
unresolved identities and final composition checks the complete real graph.
Standalone AnchorPoint export and other structural cursors remain unfinished.


## Native standalone point projection

colleaf_standalone_points.go adds sparse point scope/group/order/carrier intent to
real BOOKMARK3 ranges. Native section IDs bind ordinary/cell flows; the first
paragraph ID binds list items. Grouping inserts zero-height blocks at current
flow boundaries after list ownership reconstruction. Existing inline/container
points remain as carriers until the final tree contains the corresponding point
blocks; only then are duplicate carriers removed. Changed ownership/boundaries
retain current source positions with diagnostics. Terminal LFs receive a current
boundary fallback rather than losing points.

Point-only flows require the same CR/LF paragraph as empty native flows. A checked
carrier ID removes that paragraph only with matching owner/position and unchanged
empty content/presentation. Metadata cannot remove an unrelated authored empty
paragraph. Point order slots are unique; typed malformed metadata rejects. Binding
rebases visit only ranges emitted by the current flow, avoiding repeated scans of
prior items; ordinary content ID allocation stays unchanged. Native list items
retain the profile's paragraph-first contract.

Pinned save/edit/selection and full Go/SF structural/asset/relationship evidence
qualify standalone projection, including256 items and point-only stories/cells.
Foreign control normalization is described below. Remaining provenance/appearance/
API/browser and the broader goal acceptance audit remain required.


## Foreign native control cursors

colleaf_structural_cursors.go indexes row/cell edges, section/table terminators
and bare image/page-break paragraph terminators in the validated stream tree.
Recognized current points retain existing exact mappings; foreign control points
become explicit logical SF boundaries with source-position/story and target-loss
diagnostics. Covered-cell positions resolve through the validated occupancy grid
to the spanning origin. Deferred projections run after origins exist. Boundary
blocks use flow After data rather than duplicate paragraph IDs, preserving list
continuation ownership. Position-specific diagnostics use a projection-local set.

Independent pinned native saves/edits/selection requests assert both original and
normalized offsets, unchanged text/assets and covered-cell source records. Exact
SF structure/loss checks and a normalized native semantic fixed point are separate:
historical diagnostic transport now uses the document evidence described below.
SF keeps those diagnostics. Every scalar boundary of the rich fixture, horizontal/vertical
continuations and story/list/break cases have structural regressions. Browser UI,
remaining appearance/provenance/API and final acceptance work remain open.


## Native document-level history/evidence

colleaf_evidence.go carries version1 diagnostics, metadata page count, original
PDF pages and view/annotation evidence in existing native document intent. It
contains no editable body. Deep clones separate returned evidence from inputs.
History order/duplicates persist; only new identical warnings are deduplicated.
Current native text/style/targets/layout never restore from source evidence.

PDFNavigation permits unbound physical evidence independently of current editable
anchor availability. History syntax/geometry validates without a current catalog;
final Document.Validate always uses its complete current catalog and requires
anchors for bound inference values. Native deletion/creation reconciles historical
bindings to unbound with diagnostics, preserving physical resolution facts and
last source order rather than inventing anchors/pages or viewer state. Known
intent resources pass typed canonical JSON validation before decoding.

Actual pinned save/edit/current-skeleton evidence and strict native/SF tests cover
root history, source geometry and target edits. Full per-node Source transport and
remaining appearance/intent/API/browser/final work are unfinished; paragraph and
note components are described below.


## Native paragraph origin bindings

colleaf_origins.go stores direct manydocOrigin references in native paragraph
properties and one deduplicated pool of SourceInfo plus source-free semantic hashes
in document evidence. No alternate content tree is stored. Final restoration follows
citation/list/section composition; semantic mismatch marks SourceInfo.Historical,
retaining original facts without asserting current correspondence. Missing direct
ownership archives facts in Document.UnboundSources / SF unboundSources. Inherited
styles do not acquire direct provenance. Invalid digests/versions/references reject.

Source clones cover nested geometry, binary IDs, references and continuation data.
Formatting admission includes unbound provenance. Canonical SF/schema exposes both
historical and unbound states independently of original explicit/inferred recovery
qualifiers. Actual pinned save/edit/skeleton and ProseMirror checks qualify synthetic
paragraph-source transport. Other per-node source slots and broader native/API/UI/
acceptance work remain required.


## Native note origin bindings

colleaf_note_origins.go stores version1 source facts, kind-inference flags and
separate source-free SF body/marker fingerprints in document evidence. Hashing
removes source marks and empty source-only attrs as well as source values, so
provenance-only changes do not change content identity. Native note
identity bindings, actual native IDs/kinds and current SF note identities must agree
before ownership is restored. Composition keeps current bodies/markers authoritative.
Edited definitions/labels retain historical provenance with diagnostics; absent
bindings archive sources and diagnose the original inferred placement. This adds
no alternate body/label cache or current layout authority. Internal subset note
encoding still rejects unsupported provenance; whole-document composition owns
its projection and final restoration. Body-mark insertion retains the first sourced
paragraph's pre-projection fingerprint. Other native source slots and full API/UI/
corpus/final qualification remain required.


## Native run origin bindings

colleaf_run_origins.go stores direct text-style source references and a separate
version1 run-origin pool. Native last-record-wins spans and final citation/body-mark
composition determine current owners. A semantic hash mismatch marks a surviving
fragment historical; no surviving reference archives original source facts unbound.
Inherited style metadata does not assert ownership. Rich citation labels retain
independent sources; projected default label text does not duplicate the citation
atom's source. Private pending markers survive canonicalization/collapse and clear
before public return. Paragraph hashing strips nested run provenance without
changing old source-free paragraph hashes. Schema source marks now enumerate all
public SourceInfo fields, including historical and optional table provenance.
Zero-width anchor/container/cell/item mappings and broader native/API/UI/final
qualification remain required; empty text source output rejects explicitly.


## Native table/cell origin bindings

colleaf_table_origins.go collects reference-only tableSource/tableCells markers
into a deterministic source/fingerprint pool. Table and cell SF fingerprint shapes
remain distinct even for shared facts; nested source attributes/marks normalize out.
All descriptor references validate, including covered cells without current SF
owners. Final composition restores current owners, qualifies changed table/cell
semantics historical, and archives orphan facts unbound. It never invents covered
content or current geometry. The read-only descriptor pass preserves input bytes.
Merged/nested/main/running/note tables have actual save/edit/skeleton and strict SF
component evidence. Other source slots and full native/API/UI/final work remain.


## Native image origin bindings

colleaf_image_origins.go stores direct drawing source references and an independent
source/fingerprint pool in document evidence. Fingerprints cover current SF image
bytes, alt text, preferred geometry and anchors; no old asset is cached. Restoration
follows current asset/geometry composition and precedes enclosing table-source
qualification. Edits retain historical source facts; absent bindings archive facts
unbound. Descriptor scanning validates every reference without mutating input.
The registered drawing mirror remains checked against all direct properties,
including provenance. A shared source-record constructor retains existing table
and run/paragraph value contracts. Actual save/edit/frame/asset/SF checks qualify
this component; list/item/other container/anchor and full native/API/UI/final work
remain required.


## Native list/item origin bindings

colleaf_list_origins.go binds sources through direct leading-paragraph metadata and
a deduplicated document-evidence pool. Final list composition determines current
owners; old offsets and cached item trees have no authority. List fingerprints
cover the current list; item fingerprints cover the selected item subtree plus
parent numbering attrs and ordinal index, with all source fields normalized out.
Only the selected subtree is encoded per item, avoiding repeated sibling traversal.
Changed semantics retain historical facts; unused records archive unbound. All
native references validate, inherited styles do not own sources and unconsumed
private markers clear before return. Actual save/edit/marker-restart/SF checks
qualify this component; remaining container/anchor/native/API/UI/final work remains.


## Native inline-anchor origin bindings

colleaf_anchor_origins.go carries source references in native bookmark properties
and a deduplicated evidence pool. Source-free paragraph context plus current point
identity supplies a conservative semantic hash; surrounding edits may qualify the
anchor historical without any claim of current physical geometry. Current point
placement remains native-authoritative. Unowned facts archive unbound, every native
reference validates and transient pointers clear before return. Note-body prefix
projection captures the original paragraph hash before temporary visible mark text.
Paragraph hashes are computed once per sourced paragraph in each encoder/restorer
pass. Actual save/edit/lookup/SF evidence qualifies this component; remaining block
sources, identityless empty-text policy and native/API/UI/final work stay open.


## Native section/break/standalone origin bindings

colleaf_flow_origins.go stores source refs on native section extras, single-hard-
break paragraph properties and the first standalone-group bookmark. Final owners
restore source facts after all content/layout/group composition. SF semantic hashes
qualify changed sources historical; unused records archive facts unbound. Soft or
mixed paragraphs cannot assert single-hard-break ownership. All carrier references
validate without rewriting caller input. No alternate body/group cache or current
geometry authority is introduced. Actual save/edit/page-count/source/SF probes
qualify the component; combined real-source and native/API/UI/final audits remain.

SF encoding rejects Source on identityless empty runs, which ProseMirror cannot
retain as text nodes. Callers can explicitly retain detached facts in UnboundSources;
named anchors and empty paragraphs retain distinct supported ownership semantics.


## Compact native intent transport

Large private intent record arrays use native-intent/version 2 with gzipRecords
(base64 JSON bytes) and recordsBytes (exact decompressed UTF-8 byte count). Version1
records remain supported and emitted below 64 KiB or when compression would not
reduce base64 payload size. The 128 MiB expansion bound protects parser allocation;
it is unrelated to application admission. Single-stream gzip checksums, exact size,
trailing bytes, cancellation and strict typed JSON/resource validation apply before
interpretation. The carrier holds only existing sparse metadata/source pools, never
an alternative editable document. Real published PDF full-graph/save checks now
qualify combined paragraph/run/list provenance; browser and broader audits remain.


## Native language-style intent

colleaf_languages.go validates a versioned manydocLanguages text-style property.
The native altLang scalar remains the authority for SF EastAsiaLanguage; the
marker declares that interpretation and preserves ComplexLanguage separately.
These are inheritable authored styles, not source ownership or cached text.
Foreign unmarked altLang maps with an inference diagnostic. Native value edits
and deletion survive; current SF language semantics participate in existing source
fingerprints. The pinned engine has no distinct complex-language layout property:
retaining that setting is persistence evidence, not native shaping equivalence.
Run direction/forced complex-script rendering remain separate required mappings.


## Public native format boundary

VDOCS dispatches through the existing checked envelope workers. ReadVDOCS and
WriteVDOCS expose that same boundary; VDOCSMIME declares the exact application
MIME without implementing app admission. Write treats native transport like SF:
existing diagnostic/source history is retained, while unsupported mappings reject
regardless of AllowLossy. The CLI reuses existing atomic replacement behavior.
The offline profile is now public independently of still-unqualified browser
interaction; this does not widen the supported native rendering contract.

The initial end-to-end note probe exposed missing PDF logical tags. The painted
output structure below now fixes that failure, and the unchanged three-note
expectation runs unconditionally. Earlier opt-in failures remain historical evidence.


## Painted PDF output structure

pdf_output_structure.go clones current logical content and assigns transient
structure owners to runs, list/body labels and images. Measurement/continuation
carry those owners into actual painting. Page MCIDs are contiguous and only
created for nonempty paint; blank citations retain empty structural owners.
Shaping/ActualText boundaries stay inside the same ordinary-span owner, without
splitting shaping contexts at paint changes. Repeated headers/running content and
continued labels are artifacts, not duplicate logical owners.

The existing in-process pdfcpu font/destination finalization pass emits a PDF2
namespace, role tree, MCR/page relationships, inverse ParentTree, reciprocal note
Refs and IDTree. IDs are byte strings; Alt is a Unicode text string. Globally
unique note IDs persist; kind-scoped collisions receive stable qualified IDs.
No alternate text/body/SF cache is stored. Default import uses that graph;
PDFSkipStructure provides a diagnosed positioned view, and PDFSkipInference also
disables geometric grouping. Physical test oracles explicitly select that view;
separate logical tests check relationships/content/assets/formatting.


## Tagged PDF numbering recovery

pdf_list_numbering.go recognizes canonical numeric tokens under explicit PDF
Decimal/Roman/Alpha families (or conservative decimal inference for None). One
maximal token position must reproduce every trimmed Lbl through the existing SF
numbering renderer. Starts and prefix/suffix templates are inferred, bounded and
validated; ambiguous, noncanonical or unrepresentable patterns retain literal
content. The source L/LI hierarchy is explicit when a counter is recovered, while
source evidence/diagnostics distinguish display-derived start/template values.

Rich/non-text/annotated label blocks never become automatic counters that would
drop their relationships/assets. Their current literal fallback remains explicit
loss of editable list semantics. Marker typography has a separate diagnostic.
Nested prefixes stay literal observed context, with no inferred dynamic ancestry.
The writer's standard attribute describes the rendered family, including legal
decimal and decimalZero as the Decimal family.


## Reciprocal PDF ownership validation

`pdf_parent_tree.go` independently parses the number tree and compares its
container/MCID or whole-object slots against parsed forward content owners before
notes/materialization. Typed content-object references remain private parser state;
there is no SF schema expansion. Missing indexes/keys/slots and inverse-only
ownership remain explicit uncertainty; they never change forward ordering.
Malformed/cyclic ranges, unknown/nonindirect owners, shared container keys, wrong
slot shape and contradictory owners reject. See PDF_PARENT_TREE.md.


## Bounded PDF attribute graph expansion

The forward structure parser now shares a document-wide expanded-attribute work
counter across owners. Value visits, class selection/entries and dictionary
members consume the 200,000-entry budget, with cancellation checked during
traversal. The existing depth-16 guard alone could not bound acyclic shared arrays.
Class/local override order is unchanged; overflow returns an error without partial
content. See PDF_RESOURCES.md for regression/source evidence and distinct bounds.


Actual desktop preset browser qualification now has a reproducible repeated-note
placement failure. The unchanged public notes example places label 2 above label 1;
the pinned renderer uses the later repeated citation index for the first note.
The source identity/content graph still persists, but that does not prove correct
body order. COLLEAF_BROWSER.md and scripts/verify_colleaf_note_order.cjs preserve
the original first-reference-order oracle and passing no-repeat control. Ordinary
Go tests do not run this optional native-reader check, which currently FAILS.
No application/vendor/production change or full browser/fidelity claim is made.


## Constant PDF list labels

After canonical counter recovery fails, pdfConstantListNumbering can retain a
known explicit ordered family with identical displayed labels through the existing
constant Numbering.Template. It verifies counter independence with two values and
retains empty labels without paint references. Source evidence distinguishes the
unobserved hidden start from observed counter parsing; default one is diagnosed.
This is not a new SF schema, source body cache or proof of original authoring
restart/template intent. Annotation/nontext guards still preserve literal content.


## Literal list-marker ownership

ListItem.Marker (*ListMarker) owns literal Blocks and Source independently of
automatic numbering and body blocks. SF list_marker is an optional leading child
of list_item, with ordinary source-bearing block content. Identity/source/link
walkers include this tree; no source metadata becomes editable content by itself.
PDF irregular-label fallback now retains actual L/LI/Lbl structure, assets and
relationships instead of flattening paragraphs. Unknown kind/counter intent is
explicit. PDF generation uses a borderless parallel flow plan through the existing table
continuation machinery, with real Lbl/LBody content owners and no logical layout
table. Block-bearing labels remain literal on recovery. VDOCS now uses checked
editable label/body cells with measured automatic counters. LITERAL_LIST_MARKERS.md
and NATIVE_LITERAL_MARKERS.md record scoped evidence and the broader open queue.


Generated PDF ordinary Span groups now carry standard structural ActualText tied
to actual MCIDs. Groups cross contiguous authored run boundaries so shaping/paint
fragments do not duplicate replacement text. Exact matching removes synthetic
geometry gaps without discarding styles/links. A strict missing-whitespace matcher
restores only unpainted Unicode whitespace, preserves every painted character,
and diagnoses inherited whitespace style. The narrow RTL marker regression proves
CHILD survives its CHIL / D wrap; LITERAL_LIST_MARKERS.md records source/spec/QA.


DOCX literal markers use ordinary borderless layout tables within existing list/
item ranges, linked to real standard counter definitions through checked sparse
attributes. Resumed automatic runs use actual start overrides and checked semantic
identity across numbering IDs. Foot/endnotes use sequential label bookmark ranges
instead of adding a nested layout cell that LibreOffice omitted. Empty preceding
layout paragraphs carry/restorably encode item page starts; no content can hide
inside them. Note resolution and running fitting include markers. The actual ODT
link graph passes while the separate Word-PDF annotation oracle remains a recorded
failure; LITERAL_LIST_MARKERS.md gives exact behavior and qualification commands.


An internal native literal-label prototype encodes actual equal-column label/body
tables and checked manydocLiteralItem role/counter metadata. Contiguous owners
reconstruct lists; stale ownership retains actual cells and diagnostics. Source-only
marker references join list/item origins, and all native model walks include marker
trees. Empty automatic-label paragraphs use checked right alignment inside their
cells. SDK/facade graphs, browser image/counter paint and six-page continuation
now pass scoped checks and public export is enabled. See NATIVE_LITERAL_MARKERS.md
for evidence and the broader remaining qualification queue.


Native drawing registration follow-up: the initial zero-red canvas failure is
resolved by emitting DOC_DRAWING_PLUGIN root data/order from actual native sources.
The pinned controller consumes that resource to initialize its registry and collects
notes separately. Source descriptors/SF/assets remain authoritative and mirror
contradictions reject. Actual native insertion, save/reload and unchanged red-pixel
oracles establish this fix; inactive header paint is separately pink in the editor.
See NATIVE_LITERAL_MARKERS.md. Subsequent measured-counter/pagination qualification
enables public marker export; no general native fidelity claim.


## 2026-11-25 Enable native rich labels after measured-layout qualification

Public VDOCS generation now uses the checked editable label/body graph. Ordinary
native lists use21px logical indentation steps and zero hanging so the engine
retains measured counter advances. Fixed21px hanging made long counters overlap
body text; the existing original numbering example exposed it. Right-aligned empty
automatic-label cells keep measured layout without ordinary list properties. Exact
generated layout is consumed on import; custom indents still diagnose. SDK saves,
actual desktop/mobile app saves, complete nested/mixed counters, image pixels and
six-page96-token continuation pass scoped checks. No universal layout guarantee
or completion claim follows; NATIVE_LITERAL_MARKERS.md gives the remaining queue.

## Expanded PDF XObject work

The PDF reader counts actual Form/image invocations across pages and annotation
appearances, rejecting beyond200,000 visits before decoding the next source.
Unique-object/depth checks alone do not bound shallow acyclic shared Form graphs.
Valid repeated instances are still interpreted at their individual transforms;
rendered content is not memoized in a way that discards placements or source order.
This parser work guard is separate from app quotas/admission and other PDF bounds.
PDF_RESOURCES.md records source-only DAG, placement/SF, independent reader/CLI and
original-publication checks; no general PDF safety or layout claim follows.

## CID width interpretation

Font metrics now parse CID W assignments once. Below65,536 expanded assignments,
a direct path preserves order; above that, reverse application uses bounded
successor links so each final CID is written once. Shared indirect numeric arrays
are converted once per font. Cancellation/invalid-width checks occur during source
interpretation and application. Existing last-assignment values persist, while
conservative overlap detection emits pdf_cid_width_overlap and marks advances as
estimated because independent consumers disagree. Unambiguous source geometry and
original-publication SF remain unchanged. See PDF_FONT_WIDTHS.md for evidence and
remaining decoded-stream/resource scope; no application quota or new dependency.


## 2026-11-25 Bound syntax decoding while preserving actual source content

Page contents, Forms/appearances and ToUnicode use independently authored bounded
filter/predictor interpretation around standard Go and pinned hhrutter/lzw codecs.
A64MiB stage/page bound and256MiB document decoded/interpreted-work bounds reject
before partial success. Indirect decoded bytes are cached, not rendered content;
each valid occurrence keeps its actual transform/source. Strict zlib integrity now
rejects the original checksum fixture that the previous reader accepted. Packed
TIFF/PNG rules and both LZW EarlyChange modes have explicit tests and MuPDF bytes/
position evidence; pypdf's longEarlyChange0 failure remains an executable separate
oracle. No app admission/identity/quota logic or dependency version upgrade. Images
and initial xref/object-stream decoding remain separate unfinished resource paths.
PDF_SYNTAX_STREAMS.md gives limits, source references and original-corpus evidence.


## 2026-11-25 Bound image expansion and retain sample precision

Decode image/stencil/mask byte filters before invoking raster renderers; Indexed
lookup streams also use the bounded decoder. Image stages retain the existing200MB
allowance, sharing256MiB decoded work; actual raster work is bounded100million
pixels with the existing25million individual bound. JPEG headers precede pixel
allocation; CCITT uses validated dimensions. Native packed/16-bit RGB/gray Decode
mapping and16-bit mask composition preserve sample precision. Device-color and
JPEG interpolation differences retain their original strict independent oracles.
No API/dependency-version/runtime-service/app-permission changes. Original publication
and MQTT SF remain identical. PDF_IMAGE_DECODING.md records exact fixtures, independent
readers, visual inspection, reproduction and initial-parser/full-goal remaining work.


## 2026-11-25 Validate retained parser streams and compressed identities

After initial pdfcpu loading, verify original Raw xref/object streams through the
shared64MiB/256MiB decoder, bypassing lenient cached Content. Validate N/First,
member identity/offsets and active xref-index ownership before page access, retaining
incremental replacements. This closes successful corrupt-source acceptance without
claiming to bound earlier initial-parser allocations. No new API/dependency/version,
app permission gate or vendor patch. PDF_PARSER_STREAMS.md records original sources,
independent full-SF/geometry checks, original corpus equality, and the next exact
compatibility defect: direct Encrypt dictionaries in MuPDF compressed exports.
Its strict original success oracle remains failing and is next work, not a waiver.


## 2026-11-25 Adapt direct encryption dictionaries without replacing source content

A private in-memory incremental trailer gives direct Encrypt dictionaries an
indirect identity understood by the existing Go security handler. Original objects,
ciphertext/file IDs and semantic/source/asset graphs persist; prefix noise is
removed only from the parser view. Inspect Size/xref ranges/previous sections to
avoid identity collisions. Password checks remain mandatory; valid reader passwords
work with denied copy bits, without app permission gates. No vendor/application
patch, API/dependency-version change or runtime executable/service. The original
four MuPDF failures now pass unconditionally. PDF_DIRECT_ENCRYPTION.md records
independent rich source/visual/full-SF evidence, exact prior ciphertexts, repair-shape
reader discrepancies, initial-parser boundary and remaining original-goal work.

## Reciprocal geometric note graphs

After geometric flow and explicit tag recovery, PDF note inference can join
uniquely link-owned raised labels through reciprocal XYZ views. Source geometry
and physical navigation remain separate from inferred note references/kind/body
extent. A shared finite comparison budget and pre-mutation ownership plan keep
ambiguous/exhausted content visible. PDF_UNTAGGED_NOTES.md records the exact
single-paragraph scope, original-source/independent checks and remaining rich-body
work. No new dependency, runtime process, app policy or public schema was added.

## Rich geometric note-body ownership

After reciprocal link qualification, pdf_infer_note_extent.go plans same-page rich
body ownership using separator/leading-note-page evidence and coherent physical
flow. Cached nested source facts share the inference work budget; note/citation
roots remain boundaries. Complete plans precede mutation, preserving child block
roles, glyph geometry, assets and navigation while disclosing inferred ownership.
PDF_NOTE_CONTINUATION.md records public round trips, actual source/reader/visual
checks and the remaining cross-page/long-note and final-audit work.
