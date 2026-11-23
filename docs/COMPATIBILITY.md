# Compatibility and fidelity

This initial implementation targets explicit, testable document operations.
Passing the included corpus is not a guarantee for arbitrary customer files.

The active completion goal is broader than this initial support table. Required
work such as PDF structured recovery, broader DOCX visual qualification and richer footnote layout is unfinished;
current rejection behavior must not be read as an exemption from that goal.

## Format support

| Operation | Implemented behavior | Important limits |
| --- | --- | --- |
| DOCX read | Paragraphs/headings, bold/italic/underline/strike, links, lists, tables/nested tables/row and column spans/repeating header prefixes, embedded images, metadata, headers/footers, notes | Model is smaller than OOXML; diagnostics describe known representation losses |
| DOCX write | Paragraphs/headings/runs/links, lists, tables/nesting/row and column spans/repeating header prefixes, PNG/JPEG images, metadata, one page layout and header/footer, typed footnotes/endnotes with rich bodies | Unanchored draft notes reject; font embedding and Word layout are not provided |
| PDF read | Positioned text/raster assets, page boxes/rotation/UserUnit, ToUnicode/common encodings, Form/annotation transforms, ActualText/MCID provenance | Tagged forward logical trees recover common paragraphs/headings/lists/tables/merges/images/URI links; untagged geometric inference also recovers common paragraphs/headings/lists/numeric or ruled tables/merges/empty cells; more complex inference, internal destinations and OCR unfinished; unsupported codecs/encodings/operators reject |
| PDF write | Flowing text/headings/lists, run formatting/links, merged/nested tables with rich cells, PNG/JPEG images, page breaks, metadata, text headers/footers, linked rich footnotes/endnotes with continuation | Embedded Go font coverage; complex shaping/bidi rejected; body/nested/footnote row groups continue recursively; explicit cell/note breaks propagate through nested tables and retain blank pages; images fit full width/height containers; malformed, excessive or unrepresentable rasters reject |
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

Bookmark destinations, comment annotations, multiple
section layouts and asymmetric margins are not fully represented. Notes are
extracted with typed inline identities and separate rich definitions. Automatic
reference labels normalize to continuous decimal values; nondecimal/custom note
marks, numbering/restart/placement changes and custom separator content have
explicit diagnostics. Unknown body blocks are reported as unsupported, rather than claimed
as supported. The reader is not a lossless OOXML editor: fonts, sizes, colors,
themes, paragraph positioning and arbitrary style properties are outside the
current model. The declared formatting subset is bold/italic/underline/strike,
headings, lists and links.

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
Vector painting, clipping (including Form BBox), general color state, extended
graphics effects, alternate color-profile
conversion, mask resampling and out-of-crop bounds have explicit diagnostics.
Scanned pages preserve their images; they still produce no OCR text.

PDF output lays out the model anew. It does not preserve imported DOCX line/page
positions. The built-in Go fonts support the tested Latin/Greek/Cyrillic output;
missing glyphs return errors before output. CJK can be extracted from supported
PDF encodings but cannot currently be generated with these built-in fonts.
Arabic/Hebrew/Devanagari output needs a shaping/bidi implementation and is
explicitly rejected. The output is not claimed to be PDF/A or tagged PDF. Common typographic spaces
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
original decomposed Unicode. Remaining combining marks or mixed formatting inside
one cluster reject until proper mark positioning is implemented. Missing glyphs
and shaping/bidi needs still reject, with no partial PDF output. Spaces/controls
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
