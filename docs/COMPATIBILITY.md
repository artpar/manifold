# Compatibility and fidelity

This initial implementation targets explicit, testable document operations.
Passing the included corpus is not a guarantee for arbitrary customer files.

The active completion goal is broader than this initial support table. Required
work such as PDF structured recovery, broader DOCX visual qualification and richer footnote layout is unfinished;
current rejection behavior must not be read as an exemption from that goal.

## Format support

| Operation | Implemented behavior | Important limits |
| --- | --- | --- |
| DOCX read | Paragraphs/headings, family/size/RGB/auto plus bold/italic/underline/strike, links, lists, tables/nested tables/row and column spans/repeating header prefixes, embedded images, metadata, headers/footers, notes | Model is smaller than OOXML; diagnostics describe known representation losses |
| DOCX write | Paragraphs/headings/runs/links with named families/half-point sizes/RGB/auto, lists, tables/nesting/row and column spans/repeating header prefixes, PNG/JPEG images, metadata, section boundaries, four-edge layouts/distances and default/first/even header/footer variants, typed footnotes/endnotes with rich bodies | Unanchored draft notes reject; font embedding and Word layout are not provided |
| PDF read | Positioned text/raster assets, page boxes/rotation/UserUnit, ToUnicode/common encodings, Form/annotation transforms, ActualText/MCID provenance | Tagged forward logical trees recover common paragraphs/headings/lists/tables/merges/images/URI links; untagged geometric inference also recovers common paragraphs/headings/lists/numeric or ruled tables/merges/empty cells; more complex inference, internal destinations and OCR unfinished; unsupported codecs/encodings/operators reject |
| PDF write | Flowing text/headings/lists, run formatting/links, merged/nested tables with rich cells, PNG/JPEG images, page breaks, metadata, rich measured headers/footers, linked rich footnotes/endnotes with continuation | Built-in Go coverage plus explicit supplied TTFs; qualified Hebrew/Arabic bidi below, broader scripts unfinished; body/nested/footnote row groups continue recursively; explicit cell/note breaks propagate through nested tables and retain blank pages; images fit full width/height containers; malformed, excessive or unrepresentable rasters reject |
| SF / ProseMirror JSON | Versioned profile with body, marks/links, nested lists with starts and multi-block items, nested tables/row and column spans, embedded assets, metadata/layout, headers/footers, notes and diagnostics | See STRUCTURED_FORMAT.md; bridge rejects unrepresented semantics; richer required features remain unfinished |
| Legacy JSON | Public model with base64 image data and diagnostics | Unknown JSON fields rejected; this is manydoc's model, not Colleaf's .vdocs schema |
| Text | Body/table/note text; form feeds represent page breaks | Formatting and binary image data are not represented |

PDF input was tested with RC4-40, RC4-128, AES-128 and AES-256 passwords, object
streams, incremental updates, and a PDF 2.0 header. Unsupported security handlers
and arbitrary newer PDF features are not implied by those results.

## DOCX interpretation

Inserted revision content is retained and deleted content excluded, producing
the final visible view. Revision history is not preserved. Content-control text,
cached field results and text-box text are retained, with diagnostics for their
unrepresented control/evaluation/layout semantics.

Bookmark destinations, comment annotations and advanced page binding rules remain
unfinished. Section-specific layouts and conditional running bindings are now represented. Static asymmetric edges and running distances are retained. Notes are
extracted with typed inline identities and separate rich definitions. Automatic
reference labels normalize to continuous decimal values; nondecimal/custom note
marks, numbering/restart/placement changes and custom separator content have
explicit diagnostics. Unknown body blocks are reported as unsupported, rather than claimed
as supported. The reader is not a lossless OOXML editor. Resolved font family, point size and
RGB/auto now persist through defaults/styles/direct overrides alongside boolean
marks and links. Theme bindings/transforms, script-specific faces, paragraph
positioning and arbitrary style properties remain incomplete with diagnostics.

External relationships are never fetched. DOCX/PDF output never executes macros,
scripts or field programs. DOCX font substitution depends on the reader's fonts;
the writer does not embed fonts. Word visual pagination remains unverified.

## PDF interpretation and output

PDF extraction retains displayed-point text/image placements and page geometry,
with top-left origin and y down. The forward tagged logical tree reconstructs
paragraphs/headings, lists/items/starts, tables/spans, figures/Alt and URI links.
Page/Form MCIDs are separate scopes. Logical order follows `/K`; physical pages
remain available through page records and source regions, including cross-page
paragraphs. Unclaimed painted content is retained after tagged content with a
diagnostic. Cycles, contradictory parents/owners and invalid spans reject.

Baseline groups/separators remain inferred; unassociated painted content remains
unstructured. Source Stream/Role/Element distinguish paint scope from logical
ownership. Tag presence alone is insufficient. Classes/local attributes and
ListNumbering inheritance are handled; unsupported labels retain literal text
with a list-semantics diagnostic. Label punctuation/shapes normalize explicitly.
Unknown roles/attributes, namespaces/revisions and extra accessibility properties
have diagnostics. Inverse ParentTree validation is not implemented.

Untagged whitespace/geometry-based reading order, aligned paragraph grouping,
relative-size headings, consecutive marker lists/nesting/starts/continuations
and aligned numeric tables now exist. Results are explicitly inferred, not
semantic facts. `ReadOptions.PDFSkipInference` / `-pdf-skip-inference` retains
untagged paint order. More complex ruling/occlusion/double-border/rotated-table recovery, complex
layouts, cross-page inferred membership and internal destinations remain required
unfinished work. Whole-span URI geometry can be recovered;
partial/overlapping text spans and conflicting multi-target Link groups diagnose
uncertainty. ActualText replaces glyph strings once with contributing source
regions. Quads use font advances/metrics, not ink; estimates remain disclosed.
Simple-font padded CMaps are repaired only when numeric one-byte mappings are
unambiguous. PDF Info title/author/subject persist; extra fields and XMP are diagnosed.

Raster XObjects and common inline images become embedded PNG assets. Soft masks,
stencils and RGB/Gray JPEG Decode are handled. Inline raw/Flate/ASCIIHex/ASCII85/
RunLength/JPEG boundaries are supported; filter decoding must also succeed.
Explicit/color-key masks, Matte, JPX, inline LZW/CCITT framing, vertical text and
broader color/font cases remain unfinished and reject explicitly. Stencils whose
fill depends on unresolved general color-space operators also reject. Image dimensions
are bounded before decoding; this is not a claim of exhaustive resource safety.
Vector painting, complex/image clipping, general color state, extended
graphics effects, alternate color-profile
conversion, mask resampling and out-of-crop bounds have explicit diagnostics.
Scanned pages preserve their images; they still produce no OCR text.

PDF output lays out the model anew. It does not preserve imported DOCX line/page
positions. The built-in Go fonts support the tested Latin/Greek/Cyrillic output;
missing glyphs return errors before output. CJK can be extracted from supported
PDF encodings but cannot currently be generated with these built-in fonts.
Hebrew/Arabic directional output now works with suitable caller-supplied TTFs
(qualified below). Built-in Go faces lack that coverage; missing glyphs still reject.
Devanagari and other guarded scripts remain required unfinished work. The output is not claimed to be PDF/A or tagged PDF. Common typographic spaces
now use measured font/em advances with standard ActualText, so the published
InDesign source renders to nine bounded pages. The earlier implied-paragraph
fragmentation in tagged list-body import is fixed, with original /K membership
checks and visible word continuity. Text multiset/URI checks alone do not
establish full semantic or original-layout fidelity.

## Conversion acknowledgement

Readers return `Document.Diagnostics`. Writing imported content to DOCX/PDF with
diagnostics returns `ErrLossy` unless `WriteOptions.AllowLossy` is true. This is a
fidelity acknowledgement, not authorization. Positioned source provenance/pages
also require acknowledgement if diagnostics were removed, since writers reflow
and omit that provenance. The option does not suppress
unsupported-feature or missing-glyph errors.

Applications such as Daptin/Colleaf retain authority over identity, permission,
storage, actions and quotas. No Daptin integration or .vdocs adapter is included
in this project yet.

## Merged table cells

DOCX import/export and SF persistence retain horizontal/vertical spans without
duplicating cell content. Orphan, mismatched or reopened OOXML continuations
are errors. Nonempty continuation content is retained at the origin with a
placement diagnostic. DOCX table/cell grid widths are constrained to their page
or parent-cell content width, including nested tables.

PDF tables render combined row/column spans, images and nested tables. Measured
fragments determine cell heights; groups that fit with their note starts retain
exact geometry. Oversized body groups continue as page bands with per-cell flow
cursors, original logical-row ownership and page-local note reservations. Only
ending cells draw interior horizontal borders; continuation pages close their
outer borders. Each band has 6pt top/bottom padding. Nested cell/footnote tables
now supply smaller bands recursively, retaining original flow/merge ownership
and one enclosing frame per physical page. An oversized table can be the first
footnote block. Explicit breaks cut the whole table page at the first cell
boundary; sibling content remains unconsumed after that point. Nested breaks
propagate to parents. Aligned sibling events share a page boundary; repeated
breaks in one flow retain separate pages. Images now fit the full container's
width and height, including ancestor cell padding and nested table gaps; a
partial page moves an ordinary figure instead of shrinking it further. A single
dimension preserves intrinsic ratio, neither means 96 DPI. Two dimensions retain
the explicitly requested rectangle aspect. Raster pixels are never resampled or
cropped by fitting. DOCX/PDF padding differs, so physical fitted sizes may differ. SF retains
requested sizes; rendered-format readers recover physical drawing extents.
A dedicated mixed-span example passed independent ProseMirror, python-docx and
pypdf checks. Its native PDF and its LibreOffice 7.6 DOCX rendering were visually
inspected. This qualifies that targeted synthetic example, not arbitrary Word
layout or a representative real-export corpus.

## Lists

SF and the engine preserve separate list boundaries, ordered starts including
zero, nesting and multi-block item membership. Consecutive legacy paragraph-list
runs are still accepted as implicit lists. DOCX rendering uses ordinary numbering
with invisible bookmark ranges to retain exact exported membership; decimal/bullet
output supports nine numbering levels. Contradictory/malformed ranges reject.

DOCX import resolves numbering by instance/level, inherited styles, start/level
overrides, counter continuation and restart rules. Unsupported numbering formats
or label patterns produce explicit normalization diagnostics; style-name guesses
and skipped levels are also diagnosed. Ordinary unmarked continuation paragraphs
remain separate blocks because OOXML does not give them explicit list membership.

PDF lists use measured hanging indents, retain markers while wrapping and
paginating, and support nested text lists within table cells. A dedicated SF
example passed independent DOCX numbering/bookmark/link checks, native PDF
position/link checks, and LibreOffice visual inspection. Its compact version
passed independent content/indentation checks across three rendered PDF pages.
These synthetic cases do not establish arbitrary exported-document fidelity.

## Notes

SF/JSON and DOCX retain note identities scoped by footnote/endnote kind, inline
reference positions and formatting, definition order, rich bodies and part-local
links/assets. Opaque application IDs are retained through ignorable OOXML metadata;
normal consumers see standard integer IDs. First references use standard note
markers; repeated references use cached NOTEREF fields with checked bookmark
bindings. Generic field programs are not evaluated. Duplicate/dangling identities,
invalid story references and contradictory metadata reject. Draft unanchored
notes can persist but reject rendered output even with AllowLossy.

PDF draws superscript reference links, reserves text/list/image/table footnotes at the bottom
of the reference page, and uses labelled continuation-only pages for long bodies
before resuming body text. It renders endnotes in their own section with the normal
block renderer. Repeated references link to the same first note body. Native
examples passed independent target-page, line bounds, separation, link, content
and formatting checks, plus visual inspection of two-page and five-page cases.
The normal DOCX example also passed LibreOffice 7.6 rendering/inspection.

Rich footnotes use actual fragment heights and continue between text lines or
connected table-row groups. A twelve-row image/table note passes independent
3/5/4 row-page, destination, pixel, bounds and content checks across four pages.
Oversized merged note tables and their nested groups now split recursively;
minimum legal bands reserve note starts. A three-level body/two-level note
fixture independently checks native columns, borders, pixels and destinations
across seven pages. LibreOffice 7.6 reads its DOCX with all content/images but
flattens note tables; native PDF and XML semantics pass, while note-table visual
interoperability needs broader Word/producer qualification. Figure-bound first
starts that cannot coexist with a reference now use a visible linked continuation
notice; the actual body/figure starts on a later page at its full fitted size.
References/notices point to that actual body, including multiple/repeated notes.
Ordinary starts retain their reference-page policy. Richer note numbering/
placement and custom separators remain required expansion. Explicit note breaks now move remaining content to continuation pages.
Leading breaks delay the first destination; a break-only note targets its final
blank page and needs no separator height. Trailing breaks create a reusable fresh
page. Before the paragraph-property codec, the mixed cell/note-break fixture
failed LibreOffice 7.6 visual qualification: pagination differs and two note side
texts disappear despite their presence in OOXML/native PDF. A QA paragraph
pageBreakBefore variant recovered all texts/images. Production now uses the
standard property with checked ignorable control metadata, and the actual mixed
DOCX retains both side texts and both images in LibreOffice. Paragraph property
import/export preserves defaults/inheritance/false overrides separately from
unconditional controls. Leading/repeated/trailing controls and list boundaries
round trip; conditional requests already at page start add no blank page in
native PDF. LibreOffice still differs in cell/note pagination. This is a content
mitigation, not Word-equivalent layout or broad real-export qualification.
The reader diagnoses Word's lowerRoman-vs-OOXML decimal ambiguity when endnote
format is omitted. DOCX customMarkFollows retains marks encoded as literal text but
normalizes the automatic reference, with a diagnostic; it is not a custom-label
round-trip claim. Symbol-font glyph runs reject until an explicit mapping is
available, rather than silently discarding their visible glyphs. Visual checks qualify these synthetic examples only.

## Unicode typography and wrapping

Paragraph-wide Unicode 15 segmentation respects grapheme/line boundaries across
formatting runs. Text with GL/WJ controls stays unbroken; an overwide protected
group rejects. Ordinary long words can wrap at grapheme boundaries. Note markers
stay with the preceding grapheme. Soft hyphens are invisible unless their break
is selected. CR/LF/CRLF, NEL and Unicode line/paragraph separators create hard
visual lines while their logical values remain encoded. Paragraph semantics are
still supplied by the SF block tree; this is not full tagged-PDF output.

Declared advances: en/quad = 1/2 em; em/quad/ideographic = 1 em; three/four/six-per-em
= 1/3, 1/4, 1/6 em; figure = current font digit zero; punctuation = current font
period; thin/narrow = 1/5 em; hair = 1/10 em; mathematical = 2/9 em; NBSP = ordinary
font space; ZWSP, WJ and legacy ZWNBSP = zero. Tabs currently use four ordinary
space advances. NB hyphen uses the font's hyphen glyph with its original logical
code point preserved. Widths are deterministic defaults, not source-font fidelity.

Supported NFC composition renders the composed font glyph while ActualText retains
original decomposed Unicode. Remaining marks use positioned LTR glyph clusters
when supported by the chosen font. Mixed formatting inside one cluster, missing
glyphs still reject atomically. Directional Hebrew/Arabic now use the paragraph
path below; broader script/context needs remain unfinished. Spaces/controls
at soft line boundaries retain logical text at zero advance. Independent readers
may normalize whitespace or add extraction newlines around zero-width controls;
actual geometry and standard ActualText remain available. PDF reads retain
homogeneous replacement style/font/baseline evidence and diagnose mixed styles.

Tagged LBody/cell direct content and inline spans now form implied paragraphs;
explicit P/block boundaries remain separate. Provenance distinguishes those
inferred paragraphs from explicit list ownership and paragraph tags. Structural
ActualText retains homogeneous formatting/links/font metrics, with diagnostics
for ambiguous mixed formatting. Source soft hyphens remain logical characters
at joined lines. This qualifies tested structures, not arbitrary semantic
reconstruction or original-layout fidelity.

Untagged inference uses horizontal left-to-right geometry. Missing positions or
nonhorizontal/bidirectional text falls back to paint order with a diagnostic;
overlapping image/content order remains uncertain. Heading rank is a relative
font-size guess within each page. Tables currently need repeated aligned rows
and at least two numeric values in one column; row/column spans/header roles are
not invented. List labels normalize visibly with a diagnostic and keep original
labels in source provenance. Text interchange is reduced and does not duplicate
structural list markers into body text. More general recovery is required work,
not an exemption from the active objective.

Core-font advance fallback resolves WinAnsi space/hyphen/bullet aliases using
canonical CP1252 metric slots; explicit font Widths retain precedence. Spanning
wide edge bands are separated before column cuts. More complex spanning content,
sidebars, running matter, inline baseline shifts and typed PDF note recovery
remain required qualification/recovery work.


Painted rectilinear grids now recover rectangular row/column merges, empty cells,
multiline cell text, nested grids and cell images. Geometry, original text styles/
links and assets persist; cell bounds and ownership stay inferred. Supported
boundary evidence includes stroked axis lines, thin filled rectangles, rectangular
path/Form clipping, relevant alpha/neutral graphics state and mixed-width stroke
intersections. Partially covered/nonrectangular grids and content crossing cells
retain original content with ambiguity diagnostics. Coordinate/line/slot budgets
bound inference. Border appearance, authorial/header semantics, arbitrary curves,
occlusion, double borders and complex orientations are not established by this
milestone. Native PDF output now supports measured image/nested cells and
recursive oversized body/nested/footnote group continuation and explicit
cell/note page boundaries, conditional paragraph starts and image height fitting.
Broader DOCX visual/Word qualification still needs work. DOCX/SF preserve imported
cell content, with rendered image sizes fitted to the available width.


## Conditional paragraph starts

`Block.PageBreakBefore` (SF attrs.pageBreakBefore) applies only to paragraphs and
headings. It requests a new page unless already at the first flow position;
page_break always advances. DOCX resolves docDefaults, default/named styles and
direct on/off overrides. PDF propagates page-start state through body, lists,
cells and notes. Page-start properties in PDF running content reject rather than
changing pagination recursively. Other paragraph/page properties remain required
expansion. Unmarked heading bold defaults and structural empty-cell paragraphs
now round trip without inventing explicit SF marks/content; external documents
without our metadata still import their resolved styles/content normally.


## Raster output containment and validation

Supported PNG/JPEG assets are validated through a full decode before output,
with a 25-million-pixel allocation bound matching PDF import. This is a decoder
resource bound, not an application quota/permission check. Invalid pixel streams
reject even with a valid header. Fitting retains original raster data; PDF's
extracted PNG normalization preserves decoded pixels, while embedded JPEG streams
and DOCX media retain original bytes. SF preferences persist; rendered formats
return physical fitted dimensions. Geometries rounding to zero in PDF's matrix
or DOCX's EMU serialization reject atomically. Ancestor padding must leave actual
image space. Codec variants/color qualification and the wider acceptance rows
remain open; these containment checks do not establish professional fidelity.


## Repeating table headers

SF/Go retain a contiguous prefix using row repeatHeader/RepeatHeader, distinct
from semantic header cells. DOCX reads/writes ordinary tblHeader with standard
boolean forms. Noncontiguous imported flags have no OOXML effect and diagnose
normalization; merges crossing into body rows retain content/span and diagnose
disabled repetition. Invalid booleans/duplicate properties reject. Authored
SF/Go reject non-prefix flags and crossing merges.

Native PDF repeats original merged header geometry, text, links and rasters in
body/nested/footnote continuation. It admits the prefix with a body band, reserves
its height before fitting body figures, and reuses note identities/destinations.
Header image budgets adapt to measured legal body starts, including nested tables
and other prefix rows; no text or pixels are removed. Figure-bearing note prefixes
can defer to a later page using the linked notice.
Header-only tables render once. Repeating prefixes are indivisible; page controls
inside them or a prefix exceeding full container height reject atomically. Native
explicit body breaks also repeat headers on resumed table pages; Word suppresses
repetition for manual breaks. PDF import does not infer authorial repetition from
similar painted rows on different pages. Broader role/recovery/layout qualification
remains required.

The targeted example passes independent native five-page geometry, exact pixels,
unique body content and note/URI checks. DOCX retains original assets/merge/header
flags. LibreOffice 7.6 renders four inspected pages with all body text/images and
both header rows, but reduces the small header raster to 5×50 from 64×640. This
is a failed producer exact-pixel check recorded in office-observation.json;
containment does not imply exact pixel/Word-layout qualification.


## Font family, point size and color

Run.FontFamily/FontSize/Color use one family name, points (zero means renderer
default) and #RRGGBB/auto (empty means default). SF textStyle carries the same
semantics. RGB hex spelling canonicalizes; actual scalar values persist. This
profile uses numeric points, not arbitrary Tiptap CSS strings. Imported PDF
Source.Font/FontSize remain physical provenance; editable family/size/device-color
recovery now also exists with evidence and explicit approximation diagnostics. No Colleaf/Tiptap drop-in schema compatibility is asserted.

DOCX resolves docDefaults, paragraph/character basedOn chains and direct fields.
Bold/italic/strike in style definitions toggle, whereas direct values are absolute.
Export uses rFonts, exact positive half-point sz and RGB/auto color. Checked
textImplicit metadata preserves our unset defaults and rejects contradictions.
Unrepresented style properties diagnose. Four font slots and theme language/script
choices now resolve to explicit text spans with binding-loss/uncertainty diagnostics. Theme sRGB/system-lastClr run colors now resolve settings mappings
and HSL tint/shade; tint wins over shade. Calculated current RGB overrides a
mismatched final cache with a diagnostic. Unsupported/missing palette constructs
may retain a valid final RGB cache with uncertainty; without one they reject.
Broader DrawingML transformations, font-selection qualification and source font-program
recovery, highlighting/shading and additional
paragraph properties remain required work. Page backgrounds/embedded fonts now
diagnose loss. Font selection in a DOCX viewer still depends on installed faces;
output does not embed fonts there.

PDF has real Go/Go Mono regular/bold/italic/bold-italic faces. PDFFonts supplies
TTF programs in memory; PDFFontMap explicitly maps requested family names to
configured/built-in names. Family lookup is case-insensitive. The CLI exposes
repeatable -pdf-font 'Family[/B|I|BI]=path.ttf' and -pdf-font-map 'Requested=Target'.
No font is fetched or substituted implicitly; missing families/styles/glyphs
reject atomically. Supplied data is copied before subset operations. Required
TrueType table/name/cmap bounds, BMP mapping agreement and all outlines validate
before registration; font flags must permit this embedding/subsetting mode.
CFF/collections/color/bitmap fonts and name-table format 1 are currently
unsupported. Supplementary outline glyphs use original-font CID transport. This is unfinished coverage, not an exemption from the active goal.

Selected advances, effective sizes and font metrics drive wrapping, shared
baselines and paragraph/list/cell/note/running-content line reserves. Explicit
small fonts and their trailing empty lines retain their metrics. Decorations use
run colors. PDF sizes require 0.01pt precision and fit the page coordinate range;
derived 70% note sizes round to 0.01pt. DOCX requires exact half-points; unsupported
precision returns an error. NFC/Unicode ActualText behavior remains; bidi/joining controls and Hebrew/Arabic
now use the directional path below. Other common complex scripts still need shaping/itemization qualification.
Supported LTR/RTL marks use HarfBuzz positioning. Broader scripts/font formats remain required.

The four-page authored native example passes independent size/color/baseline/
advance/font-program/link/bounds tests; its four pages were inspected. A supplied
Noto face also passes Unicode/metrics/ownership checks. LibreOffice's two inspected
pages retain all tokens but use installed substitutes and different pagination;
that observation does not establish matching fonts or Word-equivalent layout.


### PDF imported text styles

Editable FontFamily resolves descriptor, embedded TrueType family metadata or
known standard core-face grouping; a stripped subset PostScript identifier is a
last-resort lookup request with uncertain grouping diagnosed. SourceInfo has
font_family_evidence (descriptor/embedded_name/core_font/postscript_fallback/
unresolved), color_space and color_components. Color components represent effective
Gray/RGB/CMYK values; Source.Font preserves the original face identifier. Embedded
font data is read only for metadata, not exported or made an available output font.
Unsupported font metadata decoders/oversized names fall back with diagnostics.

FontSize uses physical vertical scale. Other transforms remain source geometry;
nonuniform/skew effects diagnose. Opaque RGB retains 8-bit values; more precise
Gray/RGB components persist with a quantization warning. CMYK is an approximation,
not ICC-managed conversion. Unresolved general spaces omit editable color and warn;
default device-space overrides, render/opacity/blend/clip effects warn. Pattern,
ICC, spot and other complex text colors remain required expansion. Source render
modes/geometry persist. Mixed ActualText keeps each common style independently and
warns for ambiguous ones; homogeneous replacements retain their paint evidence.

Missing imported family/style still rejects PDF output unless callers supply fonts
or explicit mappings. Imported point sizes need not match writer precision; users
must choose and disclose retypesetting when necessary. Neither AllowLossy nor PDF
import silently substitutes output fonts or rounds source styles. Full rich-document
acceptance remains unfinished, including broader source-font/color/shaping coverage.


### DOCX theme color details

Run.Color remains resolved #RRGGBB/auto, not a live theme binding. clrSchemeMapping
maps logical text/background/accent/link references once to actual slots; accent
swaps are valid and direct dark/light references retain slot meaning. Theme bytes
are exactly two hex digits, RGB exactly six; invalid/duplicate mappings reject.
Plain sRGB and system lastClr resolve; system/platform binding loss is explicit.
Tint/shade modifies HSL luminance and rounds nearest final RGB channels. Inactive
modifiers diagnose; stored final caches are never transformed twice. Floating
calculations and producer quantization may differ, disclosed when cache mismatches.

LibreOffice renders the deliberate stale source cache in the synthetic fixture;
our resolved current-theme RGB differs there. Ten other source colors and all
11 exported RGB-only DOCX colors match independent reader checks. No exact Word
color-engine equivalence is claimed. Unsupported DrawingML palette constructs,
editable theme bindings, theme/script font selection and broader color/real-export
qualification remain required expansion. This is not a complete theme engine.


### DOCX per-script font slots

ASCII/high-ANSI/East-Asian/complex slot/theme pairs inherit independently through
all supported run style levels. Resolved families become textStyle spans; equal
effective families do not create extra spans. The deterministic policy follows
the cited MS-OI29500 character/hint table, including Times New Roman, Chinese
hint/charset and UTF-16 surrogate rules. themeFontLang BCP47 scripts select matching
supplemental major/minor entries; absent languages use the appropriate default
region. Cached theme fallback, low-confidence scripts, unknown application defaults
and unavailable charset decisions diagnose. A used unresolved theme with no family
cache rejects. Unused unresolved theme slots do not discard unrelated content.

Explicit cs/rtl selects complex font and bCs/iCs/szCs. Style bCs/iCs toggle;
direct values override. DOCX output writes complex twins for public explicit
bold/italic/size. Latent font selectors and theme bindings
are not retained, with diagnostics. Checked implicit-default metadata cannot
conceal contradictory script families/complex size. Normalized SF/DOCX spans
preserve current declared family/mark/size/links and nested block ownership.

Modern Unicode updates, automatic complex-script assignment without explicit
cs/rtl, cs hints, font substitution and broader real-export/Word qualification
remain required. The published deterministic policy is not a Word layout engine.
Native PDF missing glyph/unsupported shaping/cross-font-grapheme rejection stays
atomic. CID glyph transport and supported LTR graphemes now exist; paragraph-wide
joining/bidi and broader Unicode qualification remain incomplete.


### Native glyph rendering support

Admitted TTF faces can now render LTR supplementary outline glyphs and combining
marks remaining after NFC via glyph IDs/GPOS. No fallback face is selected implicitly.
The original full font is embedded, once per used glyph face; ordinary authored text also uses the glyph path; virtual controls and internal
fixed-Go note labels/captions retain primitives. PDFs using this graph serialize as PDF 1.7. CIDToGIDMap,
nominal widths, original descriptors/TTF and ActualText/ToUnicode preserve visual
and logical glyph relationships. Baseline placement matches the primitive writer's
0.01pt precision; relative offsets retain finer precision. Ink extents reserve
vertical layout space. Graphemes over 1,024 scalars, per-face inventories over 65,535
CIDs, invalid glyphs or offsets outside PDF coordinate bounds reject atomically.
Font admission remains conservative, including the existing BMP cmap/embedding-flag
requirements even though glyph transport itself uses full programs.

The original path supports LTR graphemes. Language-tagged text now also uses
contextual LTR runs and line re-shaping, as detailed below. Bidi, arbitrary script/
language support, color fonts and supplementary fallback remain unfinished. Existing explicit
broader complex-script/cross-style-grapheme errors remain until implemented.
These are required expansion work, not completion exemptions. Independent evidence
uses Noto Sans marks and DejaVu Sans monochrome U+1F600, full program hashes and
MuPDF reference GIDs/offsets; it does not qualify all fonts or Word-equivalent layout.
PDF recovery preserves actual span positions and logical text with source precision;
ambiguous noncollinear replacement paragraphs retain paint order and diagnostics.


### Language/direction bindings and LTR run shaping

SF/JSON/Go represent paragraph direction and independent run RTL characteristics,
general/East Asian/complex language tags and forced complex-script selection.
DOCX imports per-property defaults/style/direct inheritance and standard output
preserves effective values. Explicit `ltr` maps to disabled w:bidi/w:rtl; run
direction is not a Unicode embedding override. Unsupported values and malformed/
duplicate properties reject. Unused font choices/hints remain diagnosed, but
retained direction/language/cs alone no longer produces a false loss warning.

For all ordinary authored PDF text, compatible-face/size/language LTR text shapes across graphemes and
paint/annotation boundaries. Distinct strong scripts stay separate; Common/Inherited
graphemes use adjacent script context inside the shaping segment. General language
is used normally, East Asian language for the resolved East Asian script, and
forced cs selects complex language when set. Native HarfBuzz independently
qualifies Russian/Serbian/Macedonian Cyrillic locl, Latin kerning and ffi ligatures.
Line selection uses shaped cluster advances; emergency estimates never paint
partial ligatures. Each final line re-shapes, rechecks its width and backs off if
necessary. Per-cluster ToUnicode retains original logical text and ligatures;
ActualText covers each final shaped span. Terminal overflowing whitespace retains
logical text at zero advance. Shaping segments have a 65,536-scalar resource bound;
individual graphemes retain the 1,024-scalar bound. Full original TTF cost remains.

RTL paragraph/run handling now exists in every story, as detailed below. Broader
script joining,
context across differing fonts/sizes/languages and within-grapheme styles, full script itemization and broader fonts remain required implementation and
qualification. Empty language now selects und without changing the shaping path.
This milestone does not satisfy the broader Unicode acceptance criterion.


Default text now has the same kerning/ligature/line re-shaping behavior as an
explicit und language. Native original-font hashes and HarfBuzz GIDs/origins qualify
Go/Go Mono/Noto Sans examples. Adjacent same-style decorations remain continuous
across script/spacing boundaries. Marked special spaces retain real glyph positioning.
Ordinary outputs serialize as PDF 1.7; control-only ActualText uses 1.4. Full-font
embedding increases small PDF size (the short regular-face example is 71,869 bytes);
no compact-subset promise or complete bidi/style-context support is made.


### Paint boundaries and clipping

Color/underline/strike/link changes preserve shared contextual kerning/ligatures
when resolved face, effective size and canonical language bindings match. Explicit
font aliases resolving to the same face qualify. GDEF ligature carets split paint
bands; missing/mismatched carets use equal original-grapheme advance. Font/size/
language changes and paint changes inside a grapheme remain unqualified.

Split ligatures repeat the original outline under rectangular bands and use
invisible logical carriers with exact band advances. These have render mode 3 and
pdf_invisible_text diagnostics on recovery. Visible glyphs are artifacts with empty
logical mappings; supported readers recover the original text once. Unsplit mixed
paint groups use exact outer ActualText alignment, evidence pdf_actualtext_aligned.
Different replacement strings still use conservative common formatting and warnings.

Known rectangular clipping intersects axis-aligned text font-advance bounds and
baseline segments, including applicable Form BBox. Precision gains rect_clipped_
or replacement-span prefixes; these are not ink-outline bounds. Fully clipped text
retains operator strings/original bounds with fully_clipped_ precision and warning;
raw partial clips warn of hidden character ink. Complex/non-axis-aligned text and
raster image clipping remain unresolved with explicit diagnostics.


### Paragraph bidi preparation (2026-11-24)

An internal pure-Go resolver now passes the complete original Unicode 15.0.0
level/order corpus and independent wrapped-line checks. It retains paragraph
context and exposes L1/L2 maps. It is now connected to PDF layout, with the targeted support below. No API/SF
schema change is added. Directional shaping/itemization, L3/L4, paint/link/note placement and logical
text recovery still require implementation and independent glyph/layout evidence.
The internal README records the native FriBidi/ICU difference without discarding
either observation. Qualification is Unicode 15, not a claim of Unicode 18 coverage.


### Directional PDF layout (2026-11-24)

Explicit rtl/ltr paragraph flow reaches body, cell, list, running and note text;
unset flow defaults LTR. Original scalar ranges survive normalization and note
substitution. Line L1 resets retain paragraph context, final lines re-shape, and
L2 places level-uniform spans. Suitable supplied DejaVu Sans qualifies Hebrew and
Arabic marks, joining, brackets and ligatures; the font remains caller-supplied,
with no automatic fallback or synthesized styles. Run rtl applies selected
weak/neutral overrides with numeric exceptions and complex-language selection.
Embeddings/isolates/directional marks retain logical zero-advance text; ZWJ/ZWNJ
reach shaping. Other guarded scripts, full script itemization, cross-item positioning and
styles inside a grapheme remain unfinished requirements.

RTL list item gutter/marker placement follows its first paragraph; RTL footnotes
choose their label/gutter/separator side from their first paragraph, preserving
continuation choice. Inline labels stay atomic with their anchor. Physical offsets
also determine URI/note rectangles and decorations. Richer direction/producer/
section/note qualification remains open.

Nonzero-level spans retain original visible glyph outlines as empty-logical
artifacts plus invisible logical carriers with allocated widths. Source exposes
render mode 3 and logical span bounds; it is not per-character ink evidence.
This avoids observed MuPDF dropped-word/numeral-reversal behavior. Both native
readers recover the tested words; invisible controls/physical newlines may differ
in their text APIs. ActualText/ToUnicode and checked Go SF retain original scalars.
Reader gaps on bidi ActualText lines are not guessed; pdf_bidi_spacing reports
uncertainty. Empty replacements do not establish logical spacing/line boundaries.
Untagged bidi reading order/table/note recovery still uses diagnosed fallback.

The native example has independent original-program/GID/GPOS/advance, C FriBidi L2
order/continuity, RTL color clips and link checks. Two native and two LibreOffice
DOCX pages were visually inspected. LibreOffice differs on the authored RTL-space
example; this evidence does not establish Word-equivalent behavior or complete
Unicode coverage. The full acceptance audit remains incomplete.


Item context now crosses font/size/language and transparent-control boundaries
using full paragraph/final-line text with explicit offsets. Fourteen native
vectors and seven PDF rows qualify regular/bold DejaVu faces, differing sizes,
Arabic/Persian language bindings and RLM/ZWSP/WJ/FEFF. This supplies contextual
forms without cross-face ligatures; broader font/script/cross-item positioning
qualification remains open. Decimal note labels keep LTR glyph direction even
when RTL placement puts them left of the anchor. Existing explicit LTR spacing
and controls retain their prior policy.


## Rich PDF running content

Native PDF supports formatted paragraphs/headings, links, nested explicit lists,
PNG/JPEG images and merged/nested tables in its single header/footer pair. Running
default text is 8pt with 1.25 leading; explicit styles/font/shaping rules apply,
and headings are bold. The whole story repeats on body, blank and note-only
pages. Row-prefix flags remain in SF/DOCX but do not cause extra repetition inside
an already repeating story. No schema or public API change is needed.

Short stories keep the existing half-margin anchors. Taller headers extend body
start; taller footers move up to fit the page. All body/table/note placement honors
that measured rectangle. Competing images scale without changing aspect/pixels;
text sizes remain intact. The fitter reserves indivisible body/note minima and a
.001pt rounding guard. Content that cannot fit, and page controls anywhere in a
running story, reject atomically. Running note references still reject by the
existing graph contract. Static asymmetric edges/distances now exist; multiple sections, first/even/odd
stories and fields remain unfinished required expansion, not completion exceptions.

The new native example passes independent geometry/pixel/URI/note/merge checks.
Its source letterhead survives exact SF→DOCX→SF; tight oversized image preferences
normalize to physical dimensions on DOCX reimport, per the existing image contract.
LibreOffice paints five letterhead image placements on two pages, versus four
native pages with a sparse note-only continuation page. It ignores the table-cell
paragraph start. The initial tight.docx independently fitted both running pictures too large and
LibreOffice painted only one. The shared DOCX running-image budget below now
repairs that omission while native PDF stays unchanged. Source
PDF positions/inferred grids recover, but authored running roles are not encoded
by the writer or claimed by the reader. Synthetic evidence is not Word-equivalent
fidelity or a representative real-export corpus.


## DOCX running-image containment

Both running stories now share a conservative picture-height budget before
emission. Smaller logos keep their sizes; larger images preserve aspect, media
bytes and alt text while fitting. Merged/nested table constraints, list indents,
hard lines, estimated text wrapping and default/explicit sizes participate.
Body/note/nested body rasters fit the remaining container. Text-only stories and
native PDF behavior remain unchanged. Exhausted envelopes reject atomically.
SF keeps preferences, while DOCX reimport returns physical dimensions; the tested
second physical round trips are exact. No public schema/API expansion occurs.

The repaired tight fixture paints both images in LibreOffice 7.6.0.3. Four new
producer pages preserve seventeen exact-pixel placements, nested/merged geometry,
JPEG, body content and an unchanged small logo. The compatibility allowance has
pinned evidence in DECISIONS.md. Text envelopes are estimates, not Word font/layout
metrics or proof of producer impossibility; broader fonts/producer behavior and
section-specific pages/stories, advanced binding and actual overlap remain required
work. Source page twips are
exact; the tested producer slightly normalizes paper width.

Source DOCX links pass scoped XML and independent python-docx checks. Producer
PDF URI preservation remains unqualified: the nested example exports zero of two
expected links, and an independent python-docx control exports zero of one. The
strict verifier gate intentionally still fails there. Native PDF URI checks pass.
This does not turn the full rich-document goal into a completed subset.


## Four-edge pages and running distances

Layout.Margins (*PageMargins) overrides all four scalar Margin edges, including
zeros. HeaderDistance/FooterDistance pointers are points from page edge to header
top/footer bottom. Explicit zero is retained; nil retains defaults. Explicit groups
use 18pt default anchors in both writers, while legacy nil bindings keep their
established per-writer behavior. The unused scalar value persists as data. SF/JSON
retain settings and bridge ownership is independent. No new SF version is needed.

DOCX reads/writes standard edges/anchors, numeric twips/universal-unit input and
landscape size. Own files use checked ignorable metadata for binding presence;
external physical distances remain explicit. Values normalize to rendered twip
precision. Invalid/nonfinite or unrepresentable rounded containers reject atomically.
Native PDF applies all sides to body/table/image/note/story pagination and measures
running exclusions. Reserved header pictures use the anchor region, independently
from body capacity. Signed top/bottom offsets persist in SF/DOCX; native PDF handles
non-overlap but rejects actual running/body overlap pending qualification.

Current properties override revision snapshots; deleted/moved-from/text-box sections
are not active main-story sections. History loss diagnoses. Mirror/gutter/columns/
first-even story/page presentation is not yet represented and diagnoses rather than
silently appearing supported. Multiple active sections still flatten with a loss
diagnostic. Those features, signed overlap and wider producer qualification remain
required work. PDF recovery keeps physical pages/positions, without inventing
original margin/anchor intent.

Nine native pages/fourteen exact-pixel placements and six producer pages/eight
placements verify the new common frames independently. Source SF/DOCX settings,
formatting/relationships and assets compare exactly at represented precision.
Native links/RTL note bounds and merged frames pass; native packing and producer
cell-start/separator differences remain explicit. No professional/Word equivalence
is established by this targeted evidence.


Sectioned SF/DOCX preserves body membership, full layout overrides, same-variant
inheritance and explicit blank stories. Native PDF supports next/odd/even starts
and identical-policy continuous flow, changes page size/margins, and selects first/
even/default stories. Changed continuous geometry/stories still reject explicitly;
this is a required gap, not a completed-page-support exclusion. Body frames reserve
all active variants conservatively. Footnotes drain before a section transition;
endnotes use the final section. DOCX note parts fit minimum section containers.
The eight-page native example has four exact-pixel image placements and preserves
URI/note links. LibreOffice produces seven pages, suppresses the parity blank and
differs in story selection/inheritance. A separate python-docx control demonstrates
physical versus section-local parity differences. Preserve these observations;
Word and broader real-export section behavior remain unqualified.
