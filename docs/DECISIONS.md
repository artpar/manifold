# Decisions and requirements

## 2026-11-23 Project creation

The user requested a new project at `~/workspace/code/github.com/artpar/manydoc`
after discussing native Go document performers in Daptin for Colleaf. The user
explicitly requires durable knowledge transfer so another agent/human can
recover progress without the original conversation.

### Required boundaries

- Embed in a Go binary; Pandoc, LibreOffice, Poppler and remote converters are
  excluded as production requirements.
- Begin with DOCX/PDF extraction and generation. A complete office-suite clone
  or exact Word pagination is not an established capability of this project.
- Keep Daptin integration out of this standalone project. Action permission
  admission authorizes backend workflows; do not add performer-level permission
  or quota gates.
- Use independent implementation and permissively reusable components. Public
  proprietary implementation repositories do not become project source.

### Prior investigation

The initial candidates godocx v0.1.5 (also its newer main revision) and
ledongthuc/pdf 5e46805f718e were evaluated in Daptin's separate test harness:
`/Users/artpar/workspace/code/github.com/daptin/daptin/scripts/testing/document-library-evaluation`.

There were 64 named cases, mostly synthetic, and 200 deterministic damaged-file
mutations. Those mutation counts do not represent real-world compatibility.
The important failures included silently lost DOCX hyperlinks/revisions/content
controls/text boxes/fields, inaccessible table-reading internals, valid minimal
DOCX rejection, and PDF font corruption, ignored XObjects, CJK decoding errors,
spacing/operator errors and panics after opening malformed PDFs.

Use that harness's fixtures as regression inputs, but keep all permanent tests
self-contained in manydoc. Do not require the Daptin directory to run tests.

### Initial implementation choices

- Author DOCX parsing/serialization directly against OOXML using Go's ZIP/XML
  support, rather than inherit the known content-loss behavior.
- Reuse pdfcpu's PDF object parser; do not reimplement encryption and xrefs just
  to extract text. v0.11.1 supports the project's toolchain, whereas the latest
  resolved v0.16.4 requires Go 1.26.
- Reuse phpDave11/gofpdf for PDF primitives/font embedding/layout support.
- Put a small shared model in the public root package. Avoid a plugin registry,
  service, job queue, private storage authority or Daptin-specific types.
- Diagnostics and explicit lossy-conversion acknowledgement concern document
  fidelity, not authorization.

### Fixture classification correction

The inherited manifest classified `docx_missing_rels.docx` as an invalid file.
The main document has no relationship-dependent content, so its document-level
relationships part is optional; unreferenced styles/numbering ZIP members do
not make the package invalid. manydoc deliberately accepts it, like the valid
minimal DOCX. The fixture generator/manifest now reflect this interpretation.
This is distinct from a missing required document part or a dangling declared
styles relationship, which remain errors.

### PDF font buffer ownership

The first race run found that gofpdf's font-subsetting checksum generation
mutates the input TTF byte slice. Each renderer now passes its own copy to
AddUTF8FontFromBytes. Do not remove those copies or solve this by serializing
all document generation with a global lock. sfnt glyph inspection uses the
unchanged embedded fonts; its per-call work buffers are local.

## 2026-11-23 Structured-format foundation

The active user goal requires an established shared structured format and a
broader feature set than the initial implementation. Select ProseMirror JSON
with an explicit versioned profile (`manydoc-prosemirror/1`); the rationale,
compared alternatives, primary sources, wire mapping and unimplemented features
are in STRUCTURED_FORMAT.md. The executable schema declaration is
`schema/prosemirror-v1.json`. No Colleaf schema has been verified yet.

Keep the existing Document conversion engine and legacy JSON API temporarily,
with checked bridges to/from the selected SF. Preserve assets, relationships,
running content, notes and diagnostics through the bridge. Reject semantics the
engine cannot represent, including separate adjacent same-kind lists, rather
than silently flattening them. These rejections record unfinished required work;
they do not narrow the user's completion criteria. The profile's current subset
must expand alongside the parsers/renderers.

The user permits consulting UniDoc for implementation help. Public API docs and
examples may inform behavior/edge cases; independently authored implementation
and the prohibition on copying proprietary source remain the project boundary.

## 2026-11-23 Row spans and constrained table widths

Support row spans in the engine (`Cell.RowSpan`) and the existing ProseMirror
`rowspan` attribute. Zero/one mean a single row; covered cells are omitted from
later model rows. No schema-version change is needed: this adds support for an
existing attribute without changing existing encoded semantics.

OOXML interpretation follows Microsoft's published
[vMerge reference](https://learn.microsoft.com/en-us/dotnet/api/documentformat.openxml.wordprocessing.verticalmerge?view=openxml-3.0.1)
and [Val reference](https://learn.microsoft.com/en-us/dotnet/api/documentformat.openxml.wordprocessing.verticalmerge.val?view=openxml-3.0.1):
restart begins a region; omitted val continues it; continuation grid spans must
match the origin; missing vMerge closes the region. Do not copy implementation
code from proprietary converters. Invalid continuations reject instead of
inventing a merge. Preserve unusual continuation content at its origin and
report the normalized placement.

Use one bounded occupancy grid for validation, DOCX writing and PDF geometry.
PDF cells impose minimum heights over their complete row spans; connected
row-span groups stay together during pagination. Oversized groups remain
required future splitting work and return an explicit error today.

Independent LibreOffice rendering exposed a real DOCX defect missed by XML
structure checks: fixed 2400-twip grid columns clipped the right column on a
small page. Constrain table/grid/cell widths to the actual page or parent-cell
content width and use fixed table layout. Add a nested-container width regression.
The corrected mixed-span example retains all seven cells in LibreOffice 7.6 and
its rendered PDF was visually inspected. This is targeted evidence only.

## 2026-11-23 Explicit list trees and OOXML boundary ranges

Add `ListBlock` with `ListItem.Blocks` and nullable `Start` (nil = one, zero is
valid). It mirrors ProseMirror lists rather than encoding hierarchy through flat
paragraph flags. Keep the old flags as a checked compatibility adapter; never
coalesce explicit adjacent list nodes. Expand item content to paragraph/heading
plus blocks. No existing serialized meaning changes, so the SF version stays 1.

OOXML behavior follows Microsoft's published references for
[startOverride](https://learn.microsoft.com/en-us/dotnet/api/documentformat.openxml.wordprocessing.startoverridenumberingvalue?view=openxml-3.0.1),
[lvlRestart](https://learn.microsoft.com/en-us/dotnet/api/documentformat.openxml.wordprocessing.levelrestart?view=openxml-3.0.1)
and [numId cancellation](https://learn.microsoft.com/en-us/dotnet/api/documentformat.openxml.wordprocessing.numberingid?view=openxml-3.0.1).
Resolve per-instance/per-level definitions and styles; retain effective starts
across non-list text, and do not synthesize nonexistent ancestors. Diagnose
unsupported number formats/labels and inferred membership.

Use standard numbering for DOCX rendering and invisible bookmark ranges for
exported list/item membership. Numeric definitions alone cannot distinguish a
continuation paragraph/table from an ordinary following block. Content-control
wrappers were tested but LibreOffice 7.6 displayed hyperlink instructions and
lost the run appearance inside them, even with explicit richText. Bookmarks
preserve ordinary hyperlink/run rendering and do not add editable controls.
Reject broken/crossed ranges, duplicate numbering identities across ranges,
start contradictions and mixed marked/unmarked uses of an exported numbering
identity. These checks protect semantics, not application authorization.

PDF body lists measure hanging indentation from marker widths, wrap/paginate
items, and retain nested starts and continuation paragraphs. Text-table cells
use the same marker/indentation rules. The one-page SF list example and its
three-page compact PDF passed independent content/position checks and visual
inspection; DOCX rendering passed LibreOffice 7.6 inspection with no exposed
field instructions. No general Word-fidelity claim follows from these examples.

## 2026-11-23 Typed notes and reference-aware pagination

Use an inline ProseMirror atom (`note_reference`) with id/kind and a unique
side definition tree. This follows ProseMirror's
[inline footnote approach](https://prosemirror.net/examples/footnote/) while
separating definition ownership to support repeated references and rich block
bodies without duplication. The Go engine uses Run.Note and Note definitions.
IDs are scoped by kind and independent of display numbers. Invalid graphs reject;
unanchored drafts persist but cannot render invisibly.

DOCX uses standard parts, implicit relationships and reference marks, following
Microsoft's [reference mark documentation](https://learn.microsoft.com/en-us/dotnet/api/documentformat.openxml.wordprocessing.footnotereferencemark?view=openxml-3.0.1)
and [custom-mark behavior](https://learn.microsoft.com/en-us/dotnet/api/documentformat.openxml.wordprocessing.footnoteendnotereferencetype.custommarkfollows?view=openxml-3.0.1).
Ignorable metadata retains application IDs and definition order. Repeated
references use cached NOTEREF fields pointing to checked first-reference
bookmarks, so ordinary readers do not create duplicate note bodies. This is a
recognized declarative reference, not arbitrary field evaluation. Preserve rich
note bodies and their per-part relationships; reject inconsistent metadata.

Combine document-wide settings with section overrides before diagnosing note
numbering/layout losses. The standard's decimal endnote default differs from
[Word's lowerRoman default](https://learn.microsoft.com/en-us/openspecs/office_standards/ms-oi29500/a910e9f7-f95f-49ad-ad23-68ea9a7922cd).
When format is omitted, choose the published OOXML default and report ambiguity.
Custom automatic marks and separators do not silently claim round-trip support.

PDF footnote space must be reserved before painting each reference line, list
marker or connected table-row group. Text/list bodies wrap into measured bottom
areas; long bodies use continuation-only pages before body flow resumes. Endnotes
use a separate normal flow section. Internal links are part of correctness:
independent readers caught a wrong endnote page caused by using gofpdf SetLink's
page=0 instead of page=-1 (current page). Fix it and retain a page-reference
regression. Two-page and five-page examples passed independent bounds/content/
reference checks and visual inspection; DOCX passed a LibreOffice render check.
Nontext footnotes and broader layout/numbering remain unfinished, not exempted.

## 2026-11-23 Positioned PDF recovery foundation

Retain source geometry on the actual content tree, using SF node attributes and
inline marks, rather than a second authoritative text/image sidecar. ProseMirror
text nodes cannot own attrs; `source` is therefore a mark. Page boxes, rotation,
UserUnit and physical dimensions persist independently from editable flow layout.
Displayed coordinates are top-left physical points. Fonts/advance quads are not
ink outlines. Source structure/evidence/confidence distinguish baseline inference
from painted content; MCIDs alone do not establish recovered logical structure.
ActualText follows PDF text-string encoding (including PDFDocEncoding, not the
content font or Windows-1252) and keeps all contributing paint regions.

Compose CTM, text matrices and Form/appearance transforms directly against the
published PDF model. Read simple/CID metrics; apply character spacing per encoded
glyph rather than Unicode rune count. Core font metrics are reused through
pdfcpu's public API, guarded by IsCoreFont: its non-core missing-font fallback
can exit the process and must never be reached from untrusted font names.

Reuse pdfcpu raster extraction and Go codecs; normalize supported image pixels to
PNG with explicit stencil/soft-mask handling and bounded dimensions. Unsupported
mask/color/codec cases reject rather than disappear. Inline-image delimiters are
binary framing, not token searching: use exact raw size or filter framing before
accepting EI. This avoids false terminators embedded in visible image pixels.

Current writers reflow and omit provenance, so imported positioned source requires
AllowLossy even if a caller clears diagnostics. This is a fidelity contract, not
an application permission gate. Logical tagged-tree and geometric semantic
recovery remain required unfinished work. No broad visual fidelity is established.

Primary format references used:

- [PDF 1.7 / ISO 32000-1](https://opensource.adobe.com/dc-acrobat-sdk-docs/standards/pdfstandards/pdf/PDF32000_2008.pdf),
  sections 7.9.2, 8.3, 8.9, 8.10, 9.3–9.4, 12.5 and 14.6, and Annex D.
- [PDF Association text errata](https://pdf-issues.pdfa.org/32000-2-2020/clause09.html),
  clarifications to text state/matrix behavior.
- Pinned pdfcpu v0.11.1 public ExtractImage/font APIs and independently licensed
  implementation were inspected for supported behavior. Production modules and
  notices remain pinned; no proprietary UniDoc implementation was used.

Independent qualification uses hand-authored streams, pypdf, MuPDF, Pillow,
python-docx and actual ProseMirror. A PyMuPDF rotation_matrix translation uses an
unscaled CropBox while raw text origins include UserUnit in the tested build;
the verifier uses physical rendered width for the clockwise rotation and checks
painted pixels too. pypdf lists shared page image resources even on pages that do
not paint them; checks distinguish unique assets from actual MuPDF paint counts.

## 2026-11-23 Forward tagged-PDF recovery

Follow the explicit `/StructTreeRoot/K` graph, not visual paint order, when a
logical tree exists. MCIDs are scoped by page and owner stream; an image's object
identity differs from the stream containing the marked invocation. A page MCID
wrapping a Form stays page-owned; an MCID inside the Form is Form-owned. Preserve
those identities separately through SourceInfo.Stream/Role/Element. Consume actual
painted text/image units once; reject cyclic/multiple ownership and contradictory
parent/page/stream references. Preserve unclaimed painted content with diagnostics.
This is forward graph interpretation, not full inverse ParentTree/conformance
validation. A bare MCID or marked tag remains insufficient evidence of structure.

Rebuild common paragraphs/headings, lists/items/labels, tables/spans and figures.
Cross-page logical paragraphs remain single editable paragraphs with multi-page
source regions. Do not inject fake paragraph/page breaks to simplify extraction.
Resolve RoleMap, ClassMap/local overrides and inherited ListNumbering. Store
list-item provenance/labels on the item, preserving paragraph provenance. Unsupported
labels remain literal content with a semantic-loss diagnostic. Header-cell role
TH survives in source metadata; broader table-header editing semantics remain open.
Bind URI Link/OBJR ownership without treating nonpainted link annotations as missing
text. Geometry supplies disclosed whole-span fallback; partial/overlapping spans,
conflicting URI groups and internal destinations remain explicit unfinished cases.

Primary reference: ISO 32000-1, 14.7–14.9, especially Tables 323–326 and 347/349;
[PDF Association clause 14 errata](https://pdf-issues.pdfa.org/32000-2-2020/clause14.html)
clarifies page context for MCR references. No proprietary implementation was used.

Qualify against a real, original published export as well as targeted streams.
U.S. Copyright Office Circular 1 (2021 InDesign/PDF Library export) is pinned in
testdata/real/ with original hash/provenance/rights notes. Its simple fonts combine
one-byte and zero-padded mapping keys with a two-byte codespace. The authoritative
simple-font code width is one byte (9.6): normalize only numeric mappings in
0..255, diagnose the repair and reject collisions/out-of-range values. Do not
change source fixtures or plain-text/structural oracles to accept bad output.
The same investigation found that pdfcpu's unvalidated read leaves convenience
Info fields empty: read/decode the actual Info dictionary, keeping extra/XMP
metadata omissions explicit.

Independent original-tree/annotation checks prove 10 lists, 57 items, 16 URI
targets and publisher role mappings; all 20,531 normalized nonspace characters
match MuPDF after including stored labels. This multiset check proves content,
not reading order; separate hierarchy/relationship tests are required. Styled
heading names map to P in the publisher's tree, so they are not labelled explicit
heading ranks. One internal destination and a conflicting multi-target Link group
remain diagnosed. Current SF -> PDF rejects this publication's U+2003 EM SPACE;
this is a required unfinished font/Unicode/layout capability, not an exemption.

## 2026-11-23 Unicode typography and paragraph-wide wrapping

Use Unicode segmentation across the entire paragraph, preserving run styles and
note atoms, rather than tokenizing each formatting run with an ASCII regexp.
Reuse uniseg v0.4.7's Unicode 15 UAX #14/#29 implementation; keep font metrics for
proportional widths instead of its terminal-cell width facility. Greedy legal
breaks precede emergency grapheme boundaries for ordinary long words. Protected
no-break groups and genuinely oversized atoms reject without partial output.

Common typographic spaces are advances, not missing-glyph substitutions. The
explicit font/em table is documented in COMPATIBILITY.md. Standard ActualText
preserves original Unicode while Tz scales a real embedded space glyph to the
specified advance. Zero-width controls keep a zero-advance painted carrier;
omitting that carrier made a tested reader duplicate nearby text. Some readers
still add extraction newlines around zero-width controls; geometric positions and
logical ActualText are separately checked. Underline/strike lengths follow the
measured span. Standard NFC composition selects a supported visual glyph while
retaining original decomposed text; unresolved marks/mixed-style clusters reject
until shaping/mark positioning is implemented. This does not claim broad shaping.

ActualText requires PDF 1.4. gofpdf v1.4.3 has no public minimum-version setter,
so promote only its serialized `%PDF-1.3` header to `%PDF-1.4` with an equal-length
byte change. This does not relocate any object/xref entry or rewrite user content.
Existing higher versions remain untouched. Current output lacks UserUnit; enforce
the default-unit 14400pt page limit rather than emitting invalid oversized pages.

Independent spacing checks exposed eager newline inference on every Td after BT.
Use transformed baselines to group adjacent positioned spans instead. Homogeneous
ActualText replacements retain justified style/font/baseline information; mixed
formatting diagnoses uncertainty. Font descriptor flags/weight/angle complement
font-name style guesses, including gofpdf's abbreviated font names.

A layout fuzz campaign discovered `000` + two soft hyphens + `0` at 22pt width:
backing off only the last discretionary candidate left an earlier overflowing
hyphen selected. Back off all overflowing candidates; keep the exact saved fuzz
input and invariant that emitted line advances never exceed available width.
The following 20-second/two-worker campaign passed 6,898 executions.

Primary references: [Unicode 17 chapter 6](https://www.unicode.org/versions/Unicode17.0.0/core-spec/chapter-6/)
for space semantics; [uniseg](https://github.com/rivo/uniseg) for its pinned UAX
implementation; ISO 32000-1 9.3/9.10 and 14.9 for text state/ActualText. Runtime
modules/notices are pinned. All production work still runs inside a Go process.

Published reflow is now possible and independently bounded/content-checked, but
visual review found existing tagged LBody fragments creating separate paragraphs,
including standalone soft-hyphen paragraphs. The 20,531-character multiset alone
cannot prove those boundaries or word flows correct. Keep this required semantic
recovery task open; do not reinterpret successful rendering as full conversion.


## Tagged inline runs imply paragraphs; painted lines do not

ISO 32000-1 14.8.3.3 and 14.8.4.3.3/14.8.4.4 distinguish inline packing,
paragraphlike blocks and list bodies. LBody may hold direct content or nested
blocks. Group consecutive direct content/inline elements, preserving explicit
paragraphs and child block containers (including blocks within inline wrappers).
Use the same rule in table cells. Mark implied paragraph boundaries inferred;
the enclosing LI and actual content ownership remain explicit. A Span with
structural ActualText is still inline. Keep homogeneous formatting/links/font
metrics on replacement; disclose mixed mappings instead of assigning arbitrary
character formatting. Preserve original discretionary hyphens and word-boundary
controls; do not invent spaces around them solely because baselines change.
Union paragraph bounds per page. Prefix custom literal list labels to the first
body paragraph only, retaining later explicit paragraphs.

The original published fixture is unchanged. Its /K independently supplies 57
inline-only LBody containers, one nested-list sibling and exact LI block order.
The independent QA now checks each implied paragraph's source element identity
and every item's block membership, plus joined discretionary words. The existing
20,531-character multiset remains a content check, not a paragraph oracle.


## Untagged inference keeps evidence separate from explicit semantics

A PDF's paint order need not be reading order. Use an independently authored
bounding-box adaptation of whitespace X-Y subdivision, not a raster/OCR service
or proprietary implementation. Wide persistent vertical gutters identify column
regions; horizontal bands separate spanning content before ordering columns.
The published [Nagy/Seth X-Y tree paper](https://sites.ecse.rpi.edu/~nagy/PDF_chrono/1984_Seth_icpr84-xy-tree.pdf)
provides the spatial-decomposition idea; our run boxes, thresholds, grouping and
semantic rules are project code. The [PDF Association tagging introduction](https://taggedpdfschool.pdfa.org/basics)
explains why explicit logical tags carry different evidence from visual guesses.

Keep inferred blocks on `SourceInfo.Structure=inferred`; original runs/assets
retain painted provenance, geometry and marks. Infer paragraphs from baseline
alignment/gaps, headings from relative font sizes, lists from repeated markers
and indentation, and numeric tables from aligned row/column evidence. Exclude
list-marker columns from tables. Do not fabricate spans or header roles from
numeric alignment alone. Prefix slicing retains conservative span geometry and
labels, with disclosed normalization/precision. Preserve ambiguous content;
missing/nonhorizontal/bidirectional geometry falls back, overlapping regions
report uncertainty. Provide an explicit physical-inspection opt-out while
keeping default structured conversion useful. Tagged trees remain authoritative.

Primitive extraction tests use the physical-inspection option so their exact
paint/run oracles remain valid. Default-mode tests independently check recovered
structure, original literal/span/asset/page coverage and source geometry. The
old generated-list marker oracle now searches structural labels as well as body
text, retaining the same expected numbers without requiring duplicate labels.
No reference bytes or expected numbers were changed. More complex recovery,
including rulings and merged/multiline/empty cells, remains required work.

A stronger spanning-edge regression exposed two interacting defects: selecting a
large mid-body horizontal gap can interleave columns, and duplicate WinAnsi names
could choose an alias byte absent from pdfcpu's CP1252 metric map, producing its
1000-unit fallback for a normal space. Separate isolated wide top/bottom bands
before column cuts and use the first remaining horizontal band. Canonicalize
space/hyphen/bullet metric lookups to 32/45/149 for Latin core fonts, retaining
explicit Widths precedence. Test the published glyph advances and independently
compare every intact Helvetica/Helvetica-Bold source string against MuPDF. No
source placement, expected reading order or reference fixture was weakened.


## Painted boundaries are evidence for a table, not authored table semantics

Recover ruled cells from independently interpreted PDF path operations and
geometry. The current path is not q/Q graphics state, coordinates are captured
under their construction CTM, fill/clip paths close implicitly, and pending clips
apply at path termination. See ISO 32000-1 sections 8.5/8.10 and the
[PDF Association path clarifications](https://pdf-issues.pdfa.org/32000-2-2020/clause08.html).
Use only eligible painted axis lines and thin rectangle fills; rectangle clipping
and relevant alpha/neutral effects are interpreted for this evidence. Retain
existing diagnostics for vector appearance and unimplemented graphics effects.

A closed grid with absent interior dividers supplies a geometric adjacency graph.
Emit only rectangular connected slot groups with consistent perimeter/interior
edges; validate against the shared grid. Preserve empty cells and source boundary
rectangles explicitly as inferred values. Process small nested grids first and
assign every content atom once to a wholly containing cell. Partial boundaries,
nonrectangular groups and crossed content do not justify silently reassigned
cells; retain the source content and disclose ambiguity instead. Block the weaker
numeric inference from overriding that rejected grid. Keep finite inference
budgets/context checks, not unbounded all-pairs allocation.

Independent MuPDF table extraction verifies seven cell rectangles/merges/empty
cells on both stroke and fill sources. A targeted python-docx document exported
by actual LibreOffice 7.6 with UseTaggedPDF=false exposed its half-stroke corner
endpoints. Account for crossing-line thickness, mixed widths and numeric epsilon;
join nubs are not interior partial dividers. Zero-area retraced fills and implicit
rectangle clips have regressions. [LibreOffice's export parameters](https://help.libreoffice.org/latest/en-US/text/shared/guide/pdf_params.html)
document that explicit option. These are targeted producer checks, not proof of
professional fidelity or a representative real-world corpus. Image/nested cells
now import, but native PDF layout remains required unfinished work.


## Measure rich fragments before page admission or painting

Use independently authored shared `pdfFragment` values for cell and footnote
content. Measure text lines, fitted images and nested connected table-row groups
without touching PDF pages/resources. Solve merged-cell heights first; collect
all nested reference identities before admitting a containing group. Painting
then uses explicit coordinates. This preserves existing text-table rectangles
and text-only note pagination while adding rich cells and notes.

Footnotes reserve actual fragment heights, including image gaps and nested cell
padding. Continuations split between measured fragments, with connected rowspan
groups and images atomic. Oversized groups and explicit cell/note page breaks
remain required unfinished continuation layout; they reject before caller output.
The old image-cell rejection regression now asserts preserved cell assets after
successful output, retaining its original source and content oracle.

Image dimensions are preferences: fit the available container width; one dimension
preserves intrinsic ratio and neither means 96 DPI. Apply fitting before deriving
a missing dimension so huge finite preferences do not overflow. DOCX and PDF share
this policy with their own cell padding (4pt and 6pt per side). SF keeps requested
sizes; rendered-format readers report physical extents. Check positive finite
sizes and DOCX integer-EMU conversion bounds. Independent pixel/bounding-box and
DOCX relationship/extent checks qualify the new self-contained fixture. Native
PDF and LibreOffice pagination differ; this is not Word-equivalent layout.


## Continue oversized body groups using per-cell ownership

Keep exact existing rectangles for groups that fit with new note starts. For an
oversized body group, plan immutable active-cell cursors row by row into page
bands, consuming only whole measured fragments. Row-spanning cells keep their
unconsumed flow across logical row boundaries; grow a row until its ending cells
finish. Apply 6pt padding per band, draw internal borders only at real cell ends,
and close page fragments. This preserves content and merge ownership without
pretending the result matches Word pagination. A fitting nested group remains
an atomic fragment; recursive oversized nested/note splitting is still required.

Collect note keys only from the planned page's fragments. Budget all new starts
and a separator before painting, accounting for both current-page notes and a
possible fresh page. Replan with a smaller budget when necessary. Admission can
then move the whole planned fragment safely. An earlier body note must not hide
the separator needed after a page move. Independent targets and a dedicated
regression cover both cases. The prior oversized-group rejection test now checks
all the same original labels exactly once across continued pages; the original
fitting seven-rectangle oracle remains unchanged.

The retained fuzz input `777cb92bf7b47ddc` is a valid tall-page case. It exposed
an overly strict new test assertion that early and late notes always occupy
different pages. The actual contract is matching reference/body page and
nondecreasing reference order; the fixed small-page fixture separately verifies
distinct target pages. No document fixture or original expected content changed.


## Select nested table bands recursively before drawing

Reuse the body table cursor/planner inside fragment selection. Return a measured
head plus an immutable continuation tail; only the head's actual references
participate in admission. Calculate a legal first-band minimum recursively so a
note can start with an oversized merged table without reserving the whole group.
Text/images stay atomic; final table spacing stays with the last part. No SF
schema extension or alternate persisted layout tree is needed.

Original offsets underestimate remaining height after continuation padding.
Remeasure the remainder without a page limit before reporting its height to a
parent. Otherwise that parent can allocate too little, producing two closed
pieces of a nested merged frame on one physical page. Let same-row bands combine
inside a single page plan. Independent checks require exactly one top/bottom
pair per spanning frame/page, not just retained text. Measurement/selection uses
context checks and no page admission or image-resource registration.

The new canonical fixture has three nested body tables and two levels in a rich
footnote. Native PDF retains frame ownership, content order, pixels and links
across seven pages. XML/python-docx retain DOCX merges/media. LibreOffice's six
rendered pages retain every token/image within bounds but visibly flatten note
tables. Record that reader interoperability limit; do not flatten valid SF/OOXML
to make a visual oracle pass. Broader Word/producer qualification remains required.
Explicit cell/note breaks and oversized image atoms remain unfinished work.


## Forced boundaries are control events, not measured height

Represent PageBreak in cell/note flow with a zero-height control and return an
explicit forced-boundary flag. A table first selects each active cell, chooses
the earliest forced position and reselects every sibling from the same immutable
cursor at that cut. Propagate the flag through parent table heads. Break-aware
groups must bypass the ordinary whole-group draw path even when their total
height fits. Coalesce aligned sibling requests; preserve repeated events in one
flow. Blank cell fragments retain the renderer's padding and page frames.

Notes consume controls before queuing remaining flow. Leading controls keep the
first-body flag until actual content is drawn; break-only definitions target the
final requested blank page. Empty trailing chunks request a page without adding
a label/separator; resumed body reuses that fresh page. Explicit note boundaries
cannot be avoided by moving the reference to another page, so ordinary avoidable
split preference does not move such references merely to fit the full note.
Other definitions may still begin on the reference page. Remainder measurement
walks all forced segments instead of treating the first one as the whole tail.

Independent XML proves DOCX retains all content and control ownership. The actual
LibreOffice 7.6 mixed-story rendering nevertheless drops NOTE_SIDE_A/NOTE_SIDE_B
and changes cell break placement. Keep this as a failed visual result. A QA-only
variant using pageBreakBefore on four following paragraphs retains all original
tokens/images in LibreOffice but still differs in cell pagination. Do not weaken
fixtures or silently flatten note tables. Qualify a production paragraph-property
codec with SF round trips, repeated/trailing controls and broader Word behavior
before changing encoding. The current native PDF behavior is independently
verified; the larger goal remains incomplete.


## Distinguish paragraph page starts from explicit page advances

[Microsoft's ISO/IEC 29500 PageBreakBefore reference](https://learn.microsoft.com/en-us/dotnet/api/documentformat.openxml.wordprocessing.pagebreakbefore?view=openxml-3.0.1)
specifies a paragraph page-start property inherited through styles unless
overridden. Keep this conditional request separate from unconditional SF controls.
Add an optional false-default paragraph/heading attribute and Go bool; no existing
profile meaning changes. Resolve defaults/styles/direct false overrides, reject
invalid booleans/duplicates/cycles, and apply the property only to the first
segment when an inline break splits an imported paragraph.

Pass page-start state through pure PDF selection. Skip a conditional control at
that position, retain unconditional controls, and move list markers/note targets
with the paragraph. Footnote continuation area counts as a fresh page-start flow;
a property on a first note body on the reference page can delay that body.

Export controls using ordinary pageBreakBefore plus checked ignorable roles.
Synthetic empty anchors establish a leading blank page; empty controls preserve
repeated/trailing requests. Collapse a control before a list onto its first
numbered paragraph and restore it outside the list range. This avoids an extra
blank page when that paragraph also has a conditional property. Keep standard
Word structures for readers ignoring metadata, and reject contradictory metadata
instead of silently discarding content.

Full-SF fuzzing exposed independent preservation defects: default heading bold
became an explicit strong mark, and required empty-cell paragraphs became authored
content. Retain original heading mark presence and identify structural empty/
table-tail paragraphs with checked metadata. Preserve the failing empty-input
seed 1968518a8e616e7b; do not normalize the comparison to hide extra nodes/marks.

Independent producer checks now retain all mixed-example tokens and both images
in LibreOffice. Its cell/note pagination still differs from native PDF. Existing
XML tests assert four paragraph properties instead of four break runs because
the encoding deliberately changed; original SF, token/page/geometry/pixel/link
oracles are unchanged. This content mitigation does not establish Word-equivalent
layout, broad real-export fidelity or completion of the active goal.


## Fit figures to full containers and validate original raster data

Treat dimensions as preferences: first fit width safely, then height while
retaining the resulting aspect. One missing dimension uses intrinsic aspect;
both explicit values can intentionally prescribe another ratio. Never crop or
resample pixels. Propagate full available height through table padding/gaps and
note areas; current partial-page space does not shrink ordinary figures. DOCX's
14pt per-cell and min(56pt, half-content) note allowances are conservative budgets
for its declared default paragraph/reference layout, not a Word layout engine.

A figure-bound first note start can be larger than the space left with its
reference even on a fresh page. Keep the fitted figure at continuation size,
render a measured linked notice and defer the original flow with first-body
identity intact. Prefer earlier complete starts when several figures compete.
Use 'a later page', since another note can precede this body's continuation.
Both notice/reference resolve to the actual body; no synthetic notice is added
to persisted SF content. Ordinary text/table starts retain prior admission rules.

Configuration-only validation allowed a well-framed alpha PNG with one of 640
scanlines to panic inside the PDF dependency and to enter a DOCX as a bad asset.
Validate a complete standard-library decode before registration/embedding. Apply
the existing PDF reader's 25-million-pixel allocation bound before expanding
pixels; cache only successful config/kind per write by digest. Keep original
encoded data. This is resource protection, not a Daptin permission/quota gate.

Check actual serialized dimensions, including half-EMU round-to-even ties, rather
than an approximate geometric threshold that can still emit zero. Preserve
atomic output for malformed, excessive and unrepresentable assets. Positive
oversized cases now assert preserved content/pixels; original fixtures and
expected source text/assets remain unchanged. Native seven-page and LibreOffice
five-page qualification is targeted synthetic evidence, not professional fidelity.


## Keep repeating prefixes separate from semantic table header roles

Use optional `Row.RepeatHeader` / SF row `attrs.repeatHeader` for pagination.
A ProseMirror table_header cell describes a semantic cell role, not necessarily
repeatable rows; do not infer one from the other. OOXML tblHeader requires a
contiguous prefix ([published property reference](https://learn.microsoft.com/en-us/dotnet/api/documentformat.openxml.wordprocessing.tableheader?view=openxml-3.0.1)).
Reject non-prefix authored flags and boundary-crossing spans. On DOCX import,
normalize ignored noncontiguous flags and ambiguous crossing spans with explicit
diagnostics, retaining their text/assets and merge geometry.

Plan the header plus a legal body band as one admission unit in all native table
contexts. Keep copies out of authored SF. Repeated note references share original
identity/destination and do not create duplicate definitions. Reserve measured
header space before fitting body images. A repeated header paints from original
row offsets: an initial row-band implementation added continuation padding inside
a spanning header, detected through independent pixel/rectangle checks and fixed
before acceptance. Copies are indivisible and cannot contain page controls in
PDF; SF/DOCX preserve such requests, while PDF rejects atomically. Native explicit
body boundaries repeat a prefix when the table resumes; this differs from
[Word's documented manual-break policy](https://support.microsoft.com/en-us/word/repeat-table-header-on-subsequent-pages).

Keep native exact-pixel/media oracles. LibreOffice's four-page rendering retains
all original text/images and both header rows, but reduces a tiny header raster
from 64×640 to 5×50. Record that failed exact-pixel producer check in office-observation.json; passing containment/visual
checks does not assert producer pixel equivalence. The five native pages and
four LibreOffice pages are synthetic qualification, not representative real-export
or Word-equivalent fidelity. Rich fonts/shaping and other acceptance work remain.


A final adversarial case found that a header figure's initial 22pt body allowance
was smaller than a real 14pt compact line plus 12pt cell padding. Reserve 26pt,
then compare every measured body fragment minimum (including nested starts)
against the remaining container. Reduce only the header image budget and remeasure
when necessary; require decreasing header height/positive budget instead of
shrinking text or looping on an unchangeable prefix. Header-only tables can fit
figures against the other prefix rows. Figure-bearing headers also qualify a
first note for its linked continuation notice. The original body/note failures
are reproducible in output/repeated-header-figure-before.log; the unchanged
content/placement tests now pass, including 24 prefix/depth/story combinations
with exact pixels, bounds and actual note targets. No source preferences change.


## Represent text styles explicitly and make font selection reviewable

Use the established ProseMirror/Tiptap textStyle pattern, with this profile's
numeric point size convention (not arbitrary CSS strings). See
[Tiptap TextStyle](https://tiptap.dev/docs/editor/extensions/marks/text-style) and
[OOXML half-point sizes](https://learn.microsoft.com/en-us/dotnet/api/documentformat.openxml.wordprocessing.fontsize?view=openxml-3.0.1).
Keep family/size/color independent of bold/italic/decorations and source provenance.

Retain resolved DOCX values from defaults/styles/character overrides. For our own
exports, checked metadata preserves unset values without inventing explicit font
requests from renderer defaults. A changed property must invalidate that metadata.
Materializing foreign font requests exposes previously discarded Calibri/Cambria;
old content/concurrency/CLI tests now select explicit Go mappings, with their
original source fixtures and content oracles intact. Never silently alias them.

Provide real built-in Go/Go Mono faces and in-memory TTF registration with explicit
family maps. Validate tables, cmap agreement and all outlines before registration,
using copied write-owned buffers and cancellation. Do not synthesize missing styles,
fetch fonts or hide shaping/glyph/precision errors. Measure selected advances and
metrics, align shared baselines and reserve full line height in every story.
An explicit smaller run must not inherit an unrelated renderer minimum, including
its trailing empty line. Keep declared PDF 0.01pt and DOCX half-point precision;
reject unsupported values instead of silently rounding source requests.

The four-page native example and two-page LibreOffice observation qualify only
targeted authored content. Font substitution/pagination in LibreOffice remains
explicit. Theme/script transformations, embedded source fonts, supplementary
planes, broader shaping/bidi and the other acceptance rows remain unfinished.


Unifying rich style resolution also applies OOXML style toggles correctly:
style b/i/strike enablement toggles inherited values, while direct run values
remain absolute ([published Bold reference](https://learn.microsoft.com/en-us/dotnet/api/documentformat.openxml.wordprocessing.bold?view=openxml-3.0.1)).
Tests distinguish paragraph, character and direct cases. Unknown style properties,
source page backgrounds and embedded font omission diagnose representation loss.
At that checkpoint pdf_editable_text_style identified the missing editable PDF
recovery; the subsequent recovery milestone below replaces that blanket warning
with specific evidence and loss diagnostics.


Register font programs lazily rather than preloading the whole face catalog.
A default-face benchmark exposed eight registered faces for a document using one;
that inflated output and allocations. Catalog validation remains eager, including
unused supplied programs, while font/CW/subset registration occurs once on use.
Generated note labels register their fixed Go face only when notes exist. The
four-page example now embeds its four used faces from nine configured choices;
its original glyph/size/color/geometry/link checks remain unchanged. The controlled
before/after benchmark records actual allocation/output effects in verification.json.


## 2026-11-23 PDF editable text styles and evidence

Recover editable Run fields alongside original geometry, rather than making source
provenance the only font record. Descriptor family and embedded sfnt typographic/
legacy family names have stronger evidence than arbitrary PostScript identifiers.
Known PDF core-face names can group by their standard family; unknown style suffixes
must not be guessed away. Embedded metadata is bounded and read-only. Output fonts
remain explicit caller configuration; source embedding rights are not inferred.

Device text color and effective physical size populate SF. Preserve effective
source components separately, with explicit RGB quantization/CMYK conversion,
unknown general-space/default override and text-paint/transform diagnostics. This
is not a color-managed PDF appearance engine. ActualText maps characters without
necessarily mapping formatting: keep common fields, clear ambiguous fields, report
uncertainty. Apply the same rule to content and structure replacements and gaps.

Existing publication source/content oracles remain intact. Imported fine-grained
sizes may exceed current writer precision and source fonts may be unavailable.
The Go reflow test selects alternatives and explicit 0.01pt rounding. The existing
nine-page independent oracle now writes a separate default-typography normalized SF;
original SF retains all source styles, including 10 families and 15 sizes. This
preserves the old controlled reflow comparison without implying source typography
or pagination fidelity. New independent span/style checks cover actual styles.

A stale positioned-PDF QA assertion bound pixels by paint-order array index while
inference had ordered figures geometrically. An isolated build of prior f8668e6
and the current build produced identical image order/pixels. Bind unchanged red,
inline and blue pixel oracles to source objects (8 0 R), inline:page:1:6 and
(12 0 R); no image fixture or expected pixel changed. Source-mark checks use mark
identity, independent of ordering beside the new textStyle mark.


## 2026-11-23 Theme color calculation and cached values

Resolve Word theme text to the existing RGB scalar instead of inventing another
canonical color format. Calculate HSL luminance tint/shade and apply settings
mappings before resolving the palette. Preserve current inherited/direct precedence.
Mapping targets are palette indices, not recursively remapped logical names.

[Microsoft MS-OI29500 2.1.72](https://learn.microsoft.com/en-us/openspecs/office_standards/ms-oi29500/8229a077-7fc8-4fba-96cc-c77b6a4fc768)
documents HSL luminance calculations and tint precedence. The implementation uses
floating HSL-equivalent chroma scaling and nearest 8-bit channel rounding; producer
quantization and cached values can differ. [ColorSchemeMapping documentation](https://learn.microsoft.com/en-us/dotnet/api/documentformat.openxml.wordprocessing.colorschememapping?view=openxml-3.0.1)
defines settings mappings. These are format/behavior references, not copied
implementation code. No proprietary UniDoc source or new runtime dependency.

A source fixture intentionally supplies a stale red cache for a blue palette
entry. LibreOffice renders the cache, while this reader calculates the active
palette color and reports mismatch. Preserve both observations; do not change the
RGB oracle to fit that producer. Ten other source colors agree, and all eleven
exported RGB-only DOCX colors render correctly in LibreOffice. This targeted QA
does not establish exact Word/producer color equivalence or professional fidelity.

Unsupported DrawingML palette choices/transforms may retain an already-final RGB
cache with explicit uncertainty. Do not ignore their children or tint that cache
a second time. No usable final cache means unsupported resolution rejects.
These constructs, theme bindings and script/font handling remain unfinished work;
this milestone does not turn remaining required capabilities into exclusions.


## 2026-11-23 Preserve font slots until source text is known

A single primary family erased used East Asian/high-ANSI choices before text
arrived. Keep slot/theme/style selectors in reader-private state, then resolve
current text into established SF textStyle spans. Do not add a competing canonical
Word font-selector representation. Preserve independent attribute inheritance and
explicitly diagnose lost future editing/language/theme selectors. Public Go/SF
font values and caller output font configuration remain unchanged.

[MS-OI29500 rFonts](https://learn.microsoft.com/en-us/openspecs/office_standards/ms-oi29500/aef3c9a6-5d6c-434b-90b7-85e761fd8e62)
provides the character/hint policy used here; [themeFontLang](https://learn.microsoft.com/en-us/dotnet/api/documentformat.openxml.wordprocessing.themefontlanguages?view=openxml-3.0.1)
provides language-based theme selection. BCP47/script lookup reuses pinned x/text,
including confidence diagnostics. These published rules do not prove current
Word/other-producer equivalence; modern/automatic complex selection and ambiguous
hints remain required work. Forced complex styles retain bCs/iCs/szCs with existing
style-toggle/direct semantics. Export public values to normal/complex twins.

An isolated 5b92a20 build compared every accepted DOCX source: the Unicode fixture
alone gains four text nodes, correctly separating its declared ＭＳ 明朝 Chinese/
supplementary spans from Cambria. No fixture or expected visible content changed.
Native readers may coalesce mapped source families into the same Go face, so the
new independent PDF check compares each actual character/face metric/style rather
than requiring the source SF's exact span partition. The SF eight-span oracle
remains exact. A new table/link test initially used a PDF-specific raw Go helper;
DOCX default cell spans zero and SF explicit one are equivalent by contract. The
fixed test compares canonical SF and unchanged font/link/table ownership facts.


## 2026-11-23 Real glyph output before relaxing Unicode guards

HarfBuzz returns glyph IDs/offsets rather than Unicode codes. Add CID transport
and original TTF embedding instead of reverse-mapping substitutions to guessed
Unicode, changing cmap data or rendering text as images. Use standard CIDToGIDMap,
ToUnicode and ActualText ([ISO 32000-1 reference](https://opensource.adobe.com/dc-acrobat-sdk-docs/standards/pdfstandards/pdf/PDF32000_2008.pdf), sections 9.7/9.10/14.9).
This is independent implementation against the format, not proprietary UniDoc code.

The existing primitive path and font validation remain for ordinary text; glyph
resources finalize only affected PDFs, in memory using the existing object writer.
Full unmodified fonts cost more bytes than subsetting, but preserve actual GIDs
and embedding correctness while a qualified glyph subsetter is absent. Per-face
CIDs distinguish logical mappings as well as glyphs. Grapheme and inventory bounds,
no implicit font substitution and atomic failures remain explicit.

Integrate supported LTR graphemes first with real glyphs/marks/supplementary text;
joining/bidi scripts remain required work, not simulated by per-grapheme shaping.
Default language und avoids host-locale dependence. Independent MuPDF Story shaping
supplies GID/GPOS reference evidence; exact font-byte hashes and pypdf fallback
extraction verify that correct plain text is not the only success criterion.

Tiny baseline precision differences reordered glyphs next to primitives in the
existing geometric inference. Quantize shared baseline placement consistently while
retaining GPOS offsets. Component movement inside ActualText must not create logical
breaks; noncollinear replacements keep uncertainty/paint order. Strict pdfcpu checks
cover the new pure-glyph font graph; hybrid legacy descriptors omit optional
FontFamily, which that validator's strict mode requires. Hybrid outputs instead
retain independent strict pypdf/MuPDF checks; no required PDF/A/UA claim is made.


## 2026-11-23 Preserve language/direction before broadening shaping

Use the existing ProseMirror profile for language scalars and independent paragraph/
run direction, including the forced complex-script flag. A single inferred language
would discard the independently inherited general/EA/complex bindings. A single
paragraph direction would discard run RTL characteristics. Public effective flags
retain standard DOCX behavior; dormant font/theme choices still diagnose separately.
Sources: [OOXML languages](https://learn.microsoft.com/en-us/dotnet/api/documentformat.openxml.wordprocessing.languages),
[RTL run characteristics](https://learn.microsoft.com/en-us/dotnet/api/documentformat.openxml.wordprocessing.righttolefttext),
[paragraph layout](https://learn.microsoft.com/en-us/dotnet/api/documentformat.openxml.wordprocessing.bidi),
[forced complex formatting](https://learn.microsoft.com/en-us/dotnet/api/documentformat.openxml.wordprocessing.complexscript).
Run ltr disables RTL characteristics; it must not be implemented as a strong-letter
bidi override. Forced cs also selects the complex language, even for Cyrillic text.

The independent C HarfBuzz oracle initially accepted localized GIDs but rejected
spacing: isolated graphemes had lost Cyrillic kerning. Preserve that oracle and
shape same-style/script language-tagged segments before breaking, then shape the
actual chosen line again. Emergency ligature widths are estimates only; split
source graphemes and re-shape instead of cutting glyphs. Recheck final advances
and back off, including removal of cross-boundary kerning. ToUnicode must map
individual logical clusters, not put a complete line on its first glyph.

Native HarfBuzz 15.0.0 via QA-only uharfbuzz 0.57.0 supplies independent GID/advance
evidence for localization, kerning, ligatures and every wrapped example run.
No native dependency enters runtime. The existing pinned pure-Go engine remains.
RTL and complex joining stay rejected until actual paragraph bidi/itemization/
line-order/context evidence qualifies support. A downloaded go-text/typesetting
v0.3.4 API inspection was research only; no new module was adopted. Its line
wrapper slices existing glyph ranges; do not assume that supplies line-boundary
re-shaping. Generalized default/style/font/language shaping remains required.


## 2026-11-23 Shape ordinary default text without an activation tag

An absent language is und, not permission to skip shaping. Route ordinary authored
text through the same contextual measurement and final-line shaper. Keep explicit
spacing/control policy and the existing positioned path for residual marks on
special-space graphemes. Keep unsupported bidi/joining guards until independently
qualified. Internal fixed-Go note UI still uses its primitive path.

Full original fonts cost bytes: the short default-face example grows from 10,896
to 71,869 bytes. Correct outlines/GIDs/GPOS and logical cluster mappings take
precedence over preserving the old subset fast path. A qualified glyph subsetter
can reduce this later. No new module/runtime executable is introduced.

Private subset aliases are not physical typefaces: App Sans and default Go were
both configured with the same Go Regular bytes. Replace alias-based reader checks
with original module-font SHA256/program/physical-name checks, retaining all content,
formatting, geometry and relationship assertions. Keep an independent primitive
font fixture for the reader's embedded-name fallback; the new descriptor path does
not replace that coverage. Control-only ActualText retains its 1.4 version check.

The unchanged decoration-width oracle caught three adjacent strokes where one
continuous underline/strike had existed. Join same-style decorations in production
instead of weakening that oracle. The spacing oracle still derives virtual widths
from the primitive font, while A's actual advance now comes from its verified
original shaped program (A is no longer needed in the control-only subset).


## 2026-11-23 Separate paint boundaries from shaping context

Color, decorations and links do not alter the resolved face, size or language.
Keep them in shared LTR shaping context, then partition actual glyph positions.
Use usable GDEF carets plus glyph-origin offsets for internal ligature paint edges;
fall back to equal original-grapheme advance only when matching carets are absent.
This is an allocation policy, not evidence of font-defined component ink bounds.
References: [CSS Text boundary shaping](https://www.w3.org/TR/css-text-3/#boundary-shaping)
and [HarfBuzz ligature carets](https://harfbuzz.github.io/harfbuzz-hb-ot-layout.html#hb-ot-layout-get-ligature-carets).
No CSS conformance or full script/bidi qualification is claimed.

Keep the independent complete-word oracle: clipped repeated ligatures initially
lost a middle f in MuPDF, even under a single outer ActualText. Use visible artifact
outlines plus unclipped invisible logical carriers with allocated CID widths. Both
MuPDF and pypdf must recover office/ffi in full, while original programs, GIDs,
GPOS, colors, clips and links remain independently checked. Carrier render mode
and invisible-text provenance are explicit, not disguised as visible source ink.

Applying rectangular text/font-box clips makes recovery bounds more useful; it
does not justify claiming arbitrary clipping/ink recovery. Retain fully clipped
operator strings with uncertainty. For mixed-style ActualText, align only on exact
decoded concatenation equality, never equal rune count. Existing genuinely
different replacement tests remain unchanged. No dependency or fixture change.


## 2026-11-24 Retain paragraph levels for line bidi layout

UAX #9 resolves paragraph context before wrapping; each line then applies L1/L2.
The current public x/text API hides embedding levels and re-resolves line slices.
Keep an attributed internal adaptation of its BSD v0.30.0 reference core/bracket
implementation, with original headers/LICENSE and pinned source hashes. Reuse the
existing module's Unicode class lookup rather than duplicating the tables. Use
published BidiBrackets canonical opening IDs and U+2329/U+232A normalization.
Sources: [x/text bidi](https://github.com/golang/text/tree/v0.30.0/unicode/bidi),
[Unicode 15 UAX #9](https://www.unicode.org/reports/tr9/tr9-46.html),
[paired brackets](https://www.unicode.org/Public/15.0.0/ucd/BidiBrackets.txt).
This is reusable open-source implementation code, not an independently authored
bidi algorithm. textprocessing/fribidi was inspected but not adopted; no LGPL
code or dependency enters manydoc.

Replace the reference's quadratic isolate suffix searches with a stack; retain
all original Unicode conformance expectations. Check 91,707 character cases and
770,241 class/direction combinations offline from original hash-checked data.
Preserve both FriBidi and ICU wrapped-line observations. FriBidi 1.0.16 leaves a
trailing PDI at its old level; UAX L1 and ICU 78.3 reset it to paragraph level.
The production resolver follows that rule, while tests preserve the exact native
disagreement. ICU may lower even levels in unidirectional lines; compare its
parity/order as allowed by UAX conformance, while original corpus levels stay exact.

Do not remove PDF RTL/joining guards based on algorithm tests alone. Carry scalar
mapping and paragraph levels into directional shaping/wrapping/placement first,
then qualify GIDs/GPOS/visual order/marks/mirroring and logical/paint/link/note
preservation. This checkpoint introduces no renderer behavior or public schema.


## 2026-11-24 Connect paragraph resolution to directional glyph layout

Carry original scalar intervals through segmentation/normalization rather than
reconstructing them from shaped strings. Keep P1 hard boundaries separate from
line boundaries, apply L1 before coalescing, and use numeric levels for L2 span
placement. Keep operators logical so editable styles/ActualText/links retain
ownership. Native HarfBuzz and C FriBidi independently qualify tested output;
algorithm conformance alone did not justify the earlier removal of RTL guards.
Hebrew/Arabic now have that glyph/order/layout evidence; other required scripts
and broader contexts remain unfinished.

Apply the published [OOXML rtl weak/neutral policy](https://learn.microsoft.com/en-us/dotnet/api/documentformat.openxml.wordprocessing.righttolefttext)
through class overrides, preserving strong letters and numeric exceptions. Explicit
ltr is not a strong-letter override. The authored first/RTL-space/second example
renders second first under this rule; LibreOffice displays first second. Retain
both observations, not a changed oracle or a Word-equivalence claim. Unset
paragraph flow stays the model's standard LTR default, not first-strong auto flow.

MuPDF initially dropped most words when full logical ActualText wrapped positioned
RTL glyphs. Reuse the artifact/logical-carrier pattern and extend it to nonzero
even levels too: Arabic numerals otherwise reversed there. Original fonts/GIDs/
GPOS still determine visible appearance; logical carriers expose their render mode
and advance-bound provenance. Zero-width marks need explicit zero CID widths,
separate from the nominal-width sentinel. ToUnicode and ActualText retain original
Unicode; third-party APIs may omit controls/add physical newlines.

Two real fuzz inputs found reader-invented spaces around bidi punctuation and a
leading mark. Preserve both seeds and exact text expectations. Track bidi state
per logical line, diagnose un-inferred gaps, and exclude empty logical replacements
from logical position state. Do not weaken text oracles. Existing generic RTL
rejection tests now verify implemented all-story output; unsupported-glyph all-story
atomic tests remain, alongside independent new glyph/layout evidence.


The first directional integration also exposed missing Arabic item context at
font/size/language and transparent-control boundaries. Pass the full normalized
paragraph or actual line to AddRunes with its item offset/length, as the
[HarfBuzz buffer API](https://harfbuzz.github.io/harfbuzz-hb-buffer.html#hb-buffer-add-codepoints)
recommends. Keep local returned clusters by subtracting the item offset. Do not
borrow a neighboring line or form cross-face ligatures. Fourteen native vectors
and seven actual PDF rows now check these cases, including a genuine original
DejaVu Sans Bold face. Its asset/license/hash are attributed; no new Go module.
Generated decimal note labels use LTR glyph direction independently of RTL anchor
placement; otherwise a multi-digit label can reverse. A focused 10/01 test pins it.


## 2026-11-24: measure rich running stories before body pagination

Reuse the independent cell/table/text planner for headers and footers rather
than introduce separate drawing rules or drop non-paragraph content. Carry the
8pt/1.25 running default through nested measurement and drawing; semantic headings
are bold. Preserve legacy small-text anchors, grow the body exclusion rectangle
for larger stories, and apply it consistently to body/table/note continuations.
Whole-story repetition makes row-prefix continuation flags inapplicable there;
clear them on temporary copies only. Page controls reject instead of recursively
creating pages from callbacks. The existing note-reference graph restriction is
unchanged. There is no public schema/API/module/dependency change.

Fit image preferences against actual measured competition, preserving text sizes,
source assets and canonical SF. Account for minimum body fragments and ancestor
padding; a .001pt floor guard prevents opposing fitted edges rounding a legal
line below its height. Strengthening the fuzz oracle from allowed unsupported
errors to required success for feasible image/text cases found that defect.
Keep the deterministic three-page floor test and exact content/pixel expectations.
Use bounded monotone budget bisection: serial image heights amplify budget
changes, making direct excess subtraction reject feasible stories. Nested/serial
image tests require six exact placements and unchanged SF. Only a specific
too-small-image failure establishes the invalid low-budget bound; never swallow font,
content or cancellation errors as a layout retry.

Independent source/native/producer checks remain separate. The four-page native
letterhead has a sparse footnote-only continuation page; LibreOffice uses two pages
and ignores a paragraph start in its table cell. Tight native PDF fits both
running images; existing DOCX encoding fits each independently to the full body
height, and LibreOffice paints only one of two placements. The new verifier
records that observed omission as unqualified required work. It also verifies the
existing rendered-format preferred-to-physical dimension normalization, not a
false lossless image-preference round trip. No old corpus/oracle was modified.


## 2026-11-24: couple DOCX running-image and body containers

Independently fitting each running picture to a full body page let the tight
fixture's footer disappear in LibreOffice. Plan both stories together, keep their
text/styles/relationships intact, and cap only pictures taller than the selected
shared height. Share exact DOCX integer column geometry; solve row-span constraints
with per-end-row buckets, include tail paragraphs and existing cell/note allowances,
and fit body/note images to the remaining container. Use an em-column estimate for
text without introducing PDF-font gates into DOCX. Preserve source preferences
and exact media bytes; verify physical normalization separately and require stable
second round trips. No schema/API/module/dependency change is needed here.

A body-line floor alone was insufficient: six serial pictures exposed an off-page
last footer. The tested LibreOffice 7.6.0.3 keeps a fifth of its original page text
area available when limiting frame growth. Consulted its pinned published
[GetVarSpace policy](https://github.com/LibreOffice/core/blob/69edd8b8ebc41d00b4de3915dc82f8f0fc3b6265/sw/source/core/layout/ftnfrm.cxx#L2411)
and [frame-growth caller](https://github.com/LibreOffice/core/blob/69edd8b8ebc41d00b4de3915dc82f8f0fc3b6265/sw/source/core/layout/wsfrm.cxx#L1744).
Reserve that allowance proactively. This is an independently authored compatibility
budget; no implementation code or dependency was adopted. Other producer/font/
section policies remain qualification work, not an assumed Word-equivalence claim.

The original source/pixel/content oracle stays fixed. The repaired tight DOCX
rectangles are about 8.25×66pt, and both now render fully. Four additional producer
pages preserve seventeen exact-pixel placements, including serial/nested/merged
stories, JPEG and nested body images. Source nominal paper twips remain exact;
LibreOffice maps the 420pt width to about 419.556pt. Preserve both observations.
The earlier independently fitted rectangles and initial body-floor failure remain
historical evidence, not passing behavior or revised source fixtures.

The new producer URI expectation initially failed. An independent python-docx
control with a valid external URL also emits zero annotations in this environment.
Keep expected two/observed zero for the new nested case and expected one/observed
zero for the control, with a reproducible optional strict gate. XML and python-docx
resolve the original standard DOCX links correctly; do not change writer semantics
or the reference URLs merely to make that producer PDF assertion pass. Native
manydoc PDF URI behavior remains independently verified. Producer URI export and
broader DOCX visual/font qualification stay open in the active goal.


## 2026-11-24: explicit four-edge page geometry in the existing SF

Extend Layout with Margins *PageMargins and HeaderDistance/FooterDistance pointers.
A group overrides all edges; pointer presence distinguishes real zero from the
legacy fallback. Retain even the ignored scalar value as persisted data, because
the existing whole-model SF preservation oracle correctly exposed its removal.
Clone these pointers on both bridges. The established ProseMirror layout object
remains canonical, with optional fields and no schema-version/dependency change.

DOCX follows the standard [header-top distance](https://learn.microsoft.com/en-us/dotnet/api/documentformat.openxml.wordprocessing.pagemargin.header)
and [footer-bottom distance](https://learn.microsoft.com/en-us/dotnet/api/documentformat.openxml.wordprocessing.pagemargin.footer)
properties. [pgMar signed/unsigned constraints](https://learn.microsoft.com/en-us/openspecs/office_standards/ms-oi29500/c13857a6-6aa5-4b53-a2a8-a8498e62085b)
inform the model; it does not claim Word's full page-layout limits. Import standard
twips and universal measures; export twips and explicit landscape orientation.
Use checked ignorable metadata only to preserve implicit/explicit distinctions,
not to overwrite physical settings. Own explicit groups retain the unused legacy
scalar; foreign standard distances are recovered explicitly. New groups use the
existing 18pt DOCX default in both writers. Legacy nil PDF bindings retain prior
anchors, avoiding an unrequested global change to existing examples/oracles.

Resolve geometry centrally and use all four sides for PDF body/table/note/running
placement. Header top and footer bottom are independent edge anchors. A running
picture already fitting a reserved header region must not be shrunk to the much
smaller body region. Signed offsets persist fully through SF/DOCX and work in PDF
when no running overlap occurs. Actual overlap still needs paint-order/layout
qualification and rejects explicitly; this is unfinished work, not completion proof.
Rounded empty containers and signed offsets becoming zero reject before output.
A new test initially compared a font-quad top with a paragraph origin; source
baseline evidence (60+11pt) fixes that mistaken test, not renderer coordinates.

Current section collection excludes history/deleted/text-box snapshots. A targeted
independent OPC case proves old missing relationships cannot override current page
properties. Unsupported mirror/columns/gutter/conditional story presentation has
explicit diagnostics; multiple-section flattening remains unfinished. Independent
page tests preserve source/pixel/content oracles and distinguish nine native from
six producer pages. The rich case uses a sparse native first page because table
selection and whole-note admission prefer a fresh page; the producer ignores its
cell start and draws a left note separator. Keep these observations while expanding
sections and pagination. Do not infer authored margin/anchor intent from untagged
PDF coordinates or claim Word equivalence from the new synthetic cases.


## Section containers preserve boundaries and independent story inheritance

Use an additive root-only `section` node in the established ProseMirror profile,
with ordinary body blocks and separately editable running-story attributes.
A missing story means inherit that variant; an explicit empty array means blank.
Layout nil inherits the prior effective layout, initially the document layout.
Require either all root sections or flat blocks, and keep global running stories
exclusive to flat documents. This avoids competing authoritative body/story models.

Read section end properties before list folding; their start type applies to the
current section, as described in the published [section-type documentation](https://learn.microsoft.com/en-us/dotnet/api/documentformat.openxml.wordprocessing.sectiontype?view=openxml-3.0.1).
Use same-variant header/footer inheritance per [header references](https://learn.microsoft.com/en-us/dotnet/api/documentformat.openxml.wordprocessing.headerreference?view=openxml-3.0.1)
and the [Word implementation correction for default references](https://learn.microsoft.com/en-us/openspecs/office_standards/ms-oi29500/5de15e59-3ec1-4f80-aa4d-00bdd447b86f).
Native even/odd selection follows the published [section-local counting rule](https://learn.microsoft.com/en-us/dotnet/api/documentformat.openxml.wordprocessing.evenandoddheaders?view=openxml-3.0.1).
Physical start parity and story parity are separate. Retain standards-derived and
producer observations separately when they disagree; do not rewrite content or
structural oracles to match one exporter. No proprietary implementation was used.

Reserve one body rectangle across active variants to keep continued tables/notes
safe, accepting conservative packing. Install new PDF geometry after old footer
painting. Drain footnotes before leaving their section. Keep changed continuous
frames explicit unfinished work. Shared DOCX note parts use minimum section
containers; broader reference-specific fitting and producer/layout qualification
remain required. Independent source/asset/reader checks and visual review support
this milestone, not a Word-equivalence or full-goal completion claim.


## Continuous stories retain the opened page's owner

Allow independent story changes inside a continuous chain with identical physical
geometry. Keep its already-opened header and queued footer, and select the new
stories on the next page. Count the shared page as the new section's first page;
the next page is its second, even when the earlier section has a different local
count. The old identical-policy shortcut skipped that counter reset. A new exact
five-page sequence pins both distinct and inherited variants without rewriting
existing fixture oracles.

Premeasure continuous-chain body bounds and the largest body/note floor before
painting. Refitting running images against a later section's floor prevents a
feasible large body line from being rejected by an earlier image budget. Clear
stale body bounds between geometries; scope note floors to actual section anchors,
prior materialization and the final endnote frame. These fixes preserve rich
content and source preferences while keeping already-shaped continuation widths
stable within the chain.

The [published section-type description](https://learn.microsoft.com/en-us/dotnet/api/documentformat.openxml.wordprocessing.sectiontype?view=openxml-3.0.1)
distinguishes paragraph-continuous starts from page-level properties, but does not
supply a complete page ownership/layout algorithm. Eight original python-docx
controls and the LibreOffice export confirm deferred stories in the tested cases;
that producer keeps earlier geometry even when later input changes it. Preserve
these observations, not a silent geometry-loss workaround. Changed continuous
geometry still rejects pending physical-page/body-frame resolution. Common-frame
packing and control/empty-section ownership remain qualification work. No Word-
equivalence or full-goal completion follows from this narrower milestone.


## Separate body frames from physical page canvases

Allow continuous margins/distances to change on a shared physical page. Keep each
body's horizontal width/left edge and its already-measured note flows; keep running
stories attached to the section that opened the page. Reserve a conservative
common vertical body before painting the chain. Flush notes and open a new canvas
when paper dimensions differ. This supports requested geometry without discarding
settings or trying to resize already-painted physical pages.

The published [page-size description](https://learn.microsoft.com/en-us/dotnet/api/documentformat.openxml.wordprocessing.pagesize?view=openxml-2.8.1)
defines width and height as section page dimensions. Microsoft's [orientation
example](https://learn.microsoft.com/en-us/office/open-xml/word/how-to-change-the-print-orientation-of-a-word-processing-document)
sets the orientation hint and dimensions separately. Native PDF uses the canonical
physical dimensions; DOCX emits a landscape hint from those dimensions. Keep the
original eight independently authored controls intact, including absent hints.
The tested producer retains their earlier geometry, while the rich example with
an explicit landscape hint switches paper. It also defers shared-page margins
where native horizontal frames change immediately. Record those observations,
not an unsupported claim that all producers or Word behave identically.

Replace the old unsupported-geometry assertion with positive page-size checks now
that the capability exists. Source fixtures, content and expected producer
observations remain intact. Verify old footer positions, mixed-frame/RTL note
labels/assets/links, long note continuation and narrow wrapped/table continuation
bounds. Common vertical packing, control/empty-section ownership and signed
running overlap remain qualification work under the original full objective.


## Signed margins preserve fixed edges and foreground content

Interpret negative vertical edges as absolute body offsets; positive edges reserve
running extents. This follows the fixed/minimum distinction in Microsoft's
[published section-property definitions](https://learn.microsoft.com/en-us/openspecs/office_file_formats/ms-doc/46c3ec54-53ff-4c0a-b0d6-07ad15d2546e).
Fit fixed stories to their own page anchors, not the body or opposite anchor.
A new exact round-trip test exposed a 370pt DOCX picture normalizing to 368pt under
the old combined anchor budget. Independent fixed caps correct that defect without
changing source preferences or positive fitting behavior.

Preserve body/note foregrounds over overlapping running artwork. The first native
raster probe used the legacy late-footer phase and tinted note ink; independent
producer rendering kept that ink black. Fixed-footer chains now paint their
running backgrounds before body/notes. This is a deliberate new overlap behavior,
not an altered committed fixture oracle. Positive-only page streams/packages remain
exact against a pinned prior build. Cross-story transparent-background ordering
still differs in the large producer example; do not infer Word-equivalent color/
layout behavior from synthetic vectors.

Restore signed fixed bounds after shared-chain envelope calculation. If measured
old notes cannot inhabit the new vertical interval, finish them before the new
frame opens, retaining both note geometry and page owner. Out-of-canvas stories
reject instead of clipping or disappearing. Large fixed/minimum mixtures with no
common envelope still require partition/refit/admission work under the full goal.


## 2026-11-24 Partition incompatible continuous story frames

The preceding signed-overlap checkpoint exposed valid sections whose common
running envelope crossed or left too little room for the body. Preserve valid
sections by splitting their native shared-page group instead of declaring those
source constructs unsupported. First attempt the whole chain to preserve existing
compatible output. Otherwise extend feasible prefixes using copied plans; failed
candidate fitting must not change the original plan or source assets. Check the
actual minimum body floor, not merely positive frame area.

At each internal admission boundary, finish existing notes before activating the
new page, with its own first-page story and page number. Subsequent compatible
sections may share that page again. Persist the authored continuous start through
SF and DOCX; do not serialize renderer decisions as authored page controls.
The measured frame strategy remains conservative and fallback prefix measurement
can take quadratic work; minimal packing and control/empty ownership qualification
are still required work. Independent producer outputs differ in packing and
running-story ownership. Record content, asset and visual observations separately;
neither producer equivalence nor full-goal completion is established.


## 2026-11-24 Qualify controls and declare section note numbering

Keep explicit controls and conditional paragraph starts separate. The native API
uses relative section starts, section-local variant counts and opened-page owner
retention. Qualify empty/control-only sources against these declared semantics,
including completely blank physical parity fillers, note drainage and final
empty-section endnote frames. Record independent producer differences instead of
rewriting the source or the expected native policy to match one producer.

The final-empty-section endnote regression revealed that sectioned DOCX output
omitted explicit note numbering. Its reimport added note_default_numbering while
flat output already declared decimal and continuous. Use one helper in both
writers and emit the policy for every section and document note kind, including
empty bodies. Microsoft documents the section-level [endnote numbering format](https://learn.microsoft.com/en-us/dotnet/api/documentformat.openxml.wordprocessing.endnoteproperties.numberingformat?view=openxml-3.0.1)
and [numbering restart](https://learn.microsoft.com/en-us/dotnet/api/documentformat.openxml.wordprocessing.endnoteproperties.numberingrestart?view=openxml-3.0.1)
properties. This change makes the existing model policy explicit; it does not
implement arbitrary custom numbering or broader placement bindings. Preserve
reader diagnostics for ambiguous external sources. The old pinned writer
reproduces the ambiguity for both endnote examples, while the new writer retains
exact SF without suppressing a diagnostic or altering an oracle.


## 2026-11-24 Add an original Word export before further compatibility claims

Use the [authoritative OASIS MQTT Version 5.0 Word source and paired PDF](https://docs.oasis-open.org/mqtt/mqtt/v5.0/os/mqtt-v5.0-os.html)
as unmodified real-export fixtures. Its publication Notices permit redistribution
of unmodified copies with the complete copyright/Notices retained. Keep those
sections embedded, attribute the source, pin hashes and do not redistribute
normalized QA output as an authoritative OASIS edition. No proprietary converter
implementation or embedded publisher font becomes manydoc code/output font.

Generate structural oracles independently from the original package and PDF tree,
not from successful manydoc output. Preserve per-cell text and hyperlink ownership
as well as counts. A normalized SF round trip proves the supported representation,
not the original automatic numbering, cached-field evaluation, table widths or
visual fidelity. Caller acknowledgement consumes conversion diagnostics for the
normalized DOCX; keep the original SF/report for the source loss record.

Retain the failing original PDF and supplementary CJK example. The tagged-table
failure is a required recovery gap: missing attributes coexist with recoverable
painted merges. Word raw continuation cells also remain explicit PDF members.
Use both sources to qualify future inferred spans and provenance; do not remove
the character, simplify tables, flatten logical structure or edit an oracle to
turn this source into an artificial passing example.


## 2026-11-24 Keep explicit table ownership separate from inferred spans

Use painted borders as evidence for missing attributes without overriding present
attributes or relabelling explicit membership as inferred. Keep truly empty continuation
IDs and row ownership on origins rather than deleting their logical identity. Refuse
ambiguous alignment and retain ragged explicit tables with diagnostics. Original MQTT
tables drive the tests; page 89 remains a required grid gap. The measured joins account
for crossing stroke thickness plus coordinate tolerance, without widening axis snapping.
See TAGGED_TABLE_SPANS.md; no proprietary converter source was used.


## 2026-11-24 Use actual short connector paint on established axes

Independent original paths corrected the initial missing-border diagnosis. Small filled
rectangles complete the collected longitudinal evidence; retain them as coverage-only
patches. They cannot invent axes or join independent ruling components. Keep existing
clipping/effect/budget rules and the unique grid/cell matcher. All 42 original
omitted-span grids now recover, including exact Word-oracle page-89 spans/text. A
genuinely missing connector remains uncertain. See TAGGED_TABLE_SPANS.md; no row-tag
border invention or broader snap tolerance was introduced.


## 2026-11-24 Subset already-shaped glyph fonts without renumbering

Retain original GIDs and their composite closure to preserve CIDToGIDMap, widths,
shaping and ToUnicode, while emptying unused outlines. Preserve names/notices, cmap and
metrics; strip already-applied layout tables and invalidated signatures, rebuild
sfnt/checksums, and identify each PDF subset uniquely. Work on admitted in-memory
programs instead of the installed-font registry/cache required by pdfcpu Subset. Keep
original font hash oracles for sources; validate optimized output with independent
fontTools outline/metric/notice and pinned raster comparisons. Do not normalize the
newly discovered Unicode URI corruption to make a corpus audit pass. See
FONT_SUBSETTING.md.


## 2026-11-24 Encode URI actions according to their ASCII type

ISO 32000-1 Table 206 defines URI as an ASCII string, so storing raw UTF-8 or a Unicode
text string is inappropriate. Use RFC 3987 section 3.1 percent encoding without
normalization or double escaping. Preserve input Unicode and expose its equivalent URI
on PDF import. Compare output to an independent RFC mapping of unchanged source targets,
and prove the old mojibake still fails. All 207 page streams and annotation rectangles
remain exact. Internal destination resolution is not inferred from a fragment string;
see URI_TARGETS.md.


## 2026-11-24 Preserve column preferences on the logical grid

Use a table-level point array instead of duplicating physical widths into each
cell or snapping the source to integer CSS pixels. Keep every narrow source axis;
merged cells sum their intervals. Preserve preferences before container shrinkage
through checked ignorable DOCX metadata, with standard grids independently usable.
Report Word auto-fit/override losses. PDF measured widths carry independent inferred
provenance; they do not change logical membership evidence. See TABLE_WIDTHS.md.


## 2026-11-24 Keep editable numbering templates and counter contexts

Extend ordered_list order with explicit formats/template/logical-level/fallback
ancestors. Enclosing counters drive edits unless the list is independent. Keep
source list identities outside the public application boundary; disclose inferred
containment and Word zero-override compatibility. Repeat alphabetic letters per
OOXML rather than use Excel column numbering. Export redundant starts and lowered
legal formats with checked metadata, driven by two independent-reader failures.
See NUMBERING.md for exact policies and the unchanged publisher heading oracle.


## 2026-11-24 Preserve split external Word URI fragments

Four source hyperlinks carry both relationship and anchor attributes. Word and
the original publisher PDF use the combined URI, unlike strict OOXML anchor-ignore
behavior. Keep the actual Word URI with disclosure, preserve the old normative
attribute oracle, and add a separately source/publisher-derived target/text oracle.
Three authority empty-path/trailing-slash source differences remain explicit; do not
normalize resource identities to mask them. See INTERNAL_DESTINATIONS.md.

## 2026-11-24 Typed internal destinations independent of URI fragments

Navigation uses explicit names and zero-width/height points in the existing SF,
not magic URI interpretation. Explicit missing declarations preserve original
broken references without inventing pages. DOCX retains point identity while
reporting range-editing/column-selection loss; private checked metadata bridges
portable Word names and standalone block grouping. Native PDF uses standard GoTo
and destination name trees, binding after paint admission with first-paint repeated
story policy. Inline points open their containing line at x=0, preserving shaping.
Independent real-source paint is exact across 189 pages and all 3,106 names survive.
PDF import still needs physical view provenance plus separately identified inferred
flow binding. This intermediate milestone does not complete that required path.

## 2026-11-24 Separate explicit PDF views from editable flow ownership

PDF names are byte identities, not Unicode-equivalence keys. Preserve namespaces
and raw bytes, with collision-checked display/synthetic IDs; retain original local
view modes/null parameters and transformed geometry separately from inferred flow
anchors. Resolve forward targets after all pages exist. Complete physical annotation
records preserve supported targets even without unambiguous text ownership. Tagged
and geometric URI/GoTo ownership share a typed target. SF/JSON preserve this source
provenance; native/Word reflow follows editable points and normalizes PDF viewing
state with source diagnostics. Full source view/rectangle/flow checks pass without
rewriting original source oracles. See PDF_NAVIGATION.md.

## 2026-11-24 Preserve provenance while reducing SF transport and codec overhead

The original export's 205,161 provenance records are required source evidence.
Keep them intact, retain the established ProseMirror profile, and use compact JSON
plus explicit standard gzip transport instead of inventing a compressed schema or
silently coarsening source positions. Reuse validated internal bridge results while
keeping all public graph checks. Stream body encoding to avoid another complete
output buffer. Independent normalized JSON hashes and all 205,918 ProseMirror nodes
prove retention. Full-tree allocation/editor costs remain explicit and must be
considered in the remaining acceptance audit. See SF_CODEC.md.

## 2026-11-24 Correct the Colleaf availability assumption and current audit

The initial local-only search did not inspect the remotely available application.
Read-only pinned GitHub inspection now establishes the actual .vdocs version-3
Univer 1.0.4 snapshot contract, with matching published schema examined. Keep the
logical ProseMirror SF and qualify a checked native snapshot adapter as the
application boundary; do not claim raw SF compatibility. Document rich source/range
and plugin-resource mappings, and keep application quotas/access/save guards out
of the library. See COLLEAF_FORMAT.md; no app/backend source was modified.

Current audit probes show explicit Note/Ref relationships and Header pagination
properties still flatten rather than recover into corresponding semantics. Keep
those concrete required gaps open. ActualText's loss of typed GoTo ownership is
fixed now, with URI/internal/mixed intersections tested. Replace stale cumulative
acceptance claims with current requirement evidence; do not use old TODOs or a
passing suite as a substitute for the actual completion criteria.

## 2026-11-24 Retain physical artifact roles without inventing templates

Capture Artifact Type/Subtype/BBox/Attached and page/stream identity, with explicit
source-content membership, in page/source provenance. This closes lost recoverable
page Header/Footer facts without duplicating content or guessing section/parity/
repeat behavior. Artifact BBox is default user space per ISO 32000-1 Table 330;
content cm applies to glyphs, not again to that box. Validate source graph/geometry
and retain nested/ActualText/Form membership. Full original object checks and exact
preexisting-value projection qualify this change. Typed note relationships remain
required next work. No dependency/runtime/app boundary changed.

## 2026-11-24 Preserve note marks independently of automatic numbering

Add an optional literal note label before recovering PDF note relationships: an
explicit reference glyph must not be replaced with an unrelated decimal number.
Custom marks do not increment automatic counters, matching the published
[OOXML customMarkFollows policy](https://learn.microsoft.com/en-us/dotnet/api/documentformat.openxml.wordprocessing.footnoteendnotereferencetype.custommarkfollows?view=openxml-3.0.1).
They remain ordinary Unicode text subject to measured layout and explicit glyph
failures. Do not embed identity in the mark or infer relationships from equal text.
A LibreOffice QA failure exposed duplication of multi-character custom marks in
one w:t; scalar text nodes fix the same logical standard mark without deleting
visible characters. Standard note-body reference atoms supply the body mark.

## 2026-11-24 Resolve explicit note edges before reflow

Use object-identity Ref edges in either direction; equal glyph text and paint order
are not relationship evidence. Retain source binary ID bytes independently of a
valid public identifier and all explicit owner/target edges. Move each rich body
once; preserve reference-site positions/formatting and note-body navigation.
An unspecified Note/FENote kind selects a disclosed editorial endnote default with
KindInferred, not an unsupported claim about physical authoring policy. Ambiguous
marks, nested/enclosing ActualText and unresolved cases retain visible flow/source
edges with diagnostics; they remain required broader graph work. See PDF_NOTES.md.

Independent visual QA found LibreOffice synthesized numbering between a list and
its following table. A checked empty paragraph cancels numbering while preserving
SF block semantics. Keep normal paragraph layout: tiny exact heights hid/clipped
the table in the independent reader and were rejected. Original sources/oracles
are unchanged; this is a generation interoperability fix discovered by the new
rich-note probe, not a golden update to accept missing content.


## 2026-11-24 Separate citation, body and inherited note display

Identity is still kind/ID. Use optional per-reference and body literal overrides,
with nil/empty distinct, instead of requiring all source marks to match or
silently flattening explicit relationships. Preserve different leading Lbl text
and its source evidence once. Treat unresolved MCR/paint as uncertainty, not proof
of a physically empty marker. Validate body-label glyphs/shaping too: independent
labels cannot rely on citation validation to catch missing characters.

Independent LibreOffice experiments showed that hidden note-body Ref runs still
produce an automatic prefix. A zero-width standard custom-note anchor plus literal
citation/body display preserves the actual visible marks and body links. Checked
roles restore semantic intent without hiding arbitrary content. Word's portable
labels become literal when variants coexist with automatic numbering; explicitly
gate that editing loss with AllowLossy and retain checked automatic intent for SF.
No generated Word editing/fidelity or full note-graph claim is implied. See
PDF_NOTES.md and the independent source/reader/visual verifier.

## 2026-11-24 Keep note relationship inference independently visible

Add NoteReference.Inferred rather than treating inferred ownership as a fabricated
explicit Ref or overwriting explicit Reference/Note roles. Preserve it in generated
Word references, cached repeats and literal citation aliases. Use matching logical
adjacency and separately identified unique physical GoTo/label-region evidence;
never infer from equal text or page-bottom placement alone. Reject ambiguous/far/
partial/conflicting evidence without consuming candidate glyphs.

The actual 71-page tagging guide contradicts an adjacency-only completion claim:
its two Note containers live at Document's end and its citations are Links. The
physical rule recovers both with exact independently decoded original body/label
text and navigation. Preserve all 87,381 old paint-order text records. Keep the
original ignored unless redistribution rights are established; record its hash
and mirror provenance. Its label backlinks expose a remaining editable-rich-label
gap, now specifically diagnosed rather than hidden by a generic reflow claim.


## 2026-11-24 Preserve body-label formatting and navigation without duplicate text

Use an optional text-free Run template and established ProseMirror marks. A note
has one body-label text authority; source LabelSource remains physical evidence.
Ordinary glyph/font/shaping/layout and destination validators apply to resolved
marks. Checked Word roles wrap actual visible rPr/hyperlinks; reject contradictory
text/nontext/hidden properties rather than letting metadata discard content.
Original publisher MCID operators and backlink objects independently qualify the
new semantics. LibreOffice font substitution and omitted custom-label PDF links
remain reader limitations, not native/package success claims. Keep fractional
DOCX size policy and heterogeneous/mixed-direction labels in the required queue;
no silent rounding or oracle/source changes were used. See PDF_NOTES.md.


## 2026-11-24 Acknowledge portable precision while preserving checked intent

Use AllowLossy for nearest half-point DOCX font sizes with a .25pt bound; strict
output keeps its previous rejection. Store the original float on the visible run,
validate explicit sz/szCs before restoring it, and diagnose the portable difference.
Do not round the canonical SF or mutate caller/source data. The original publisher
guide independently qualifies every projected run, assets and structured round trip;
14 image dimensions differ within one EMU rather than matching a revised oracle.

Unify note catalog initialization after a new sectioned styled-note test reproduced
a real panic; do not mask it as unsupported section content. Reject duplicate
expanded XML attributes, including aliases, exposed by malformed precision tests.
These changes do not broaden native PDF's .01pt contract or add font substitution,
services or application permission gates. See DOCX_PRECISION.md.


## 2026-11-24 Preserve heterogeneous body marks as ordinary rich text

Use a literal Run sequence / established ProseMirror text nodes instead of another
copy of the body-label string plus style offsets. Keep template/default numbering
as the separate alternative and reject conflicting authorities. Preserve per-run
source positions and links; do not collapse a differently styled leading Lbl to
its common intersection. Normalize bidi placement to the measured marker box.

Independent LibreOffice paint proved an inline content control moved/restyled the
mark, so emit ordinary visible runs inside a checked standard bookmark range.
Identity/bounds are metadata, actual label text remains editable authority. Reject
hidden/nontext/unbound content and protect user destinations sharing wire prefixes.
Keep observed font/hyperlink/endnote-page producer differences explicit; do not
revise source operators, expected styles or page claims to accept the prototype.
RICH_NOTE_LABELS.md records native/Word/raster/structure evidence and remaining
heterogeneous/mixed-direction citation and graph work.


## 2026-11-24 Preserve rich citations inside one measured note atom

Use ordinary text nodes/marks in label_content rather than another literal string
plus style offsets. Exclude Label when content exists; reference kind/ID owns the
link. Materialize outer positive/fallback typography into fragments in canonical
SF. Keep original source regions on each fragment; diagnose alternate source
navigation when editable citation ownership replaces it.

Shape/layout the interior independently and bind one outer atom to its preceding
grapheme. This preserves note reservation/link bounds and supports mixed-direction
plain labels without changing the existing homogeneous RTL shaping path. Word
uses ordinary reference runs in its checked hyperlink. LibreOffice ignored w:dir
for this case; tested Unicode LRI/PDI controls kept fragments together. Validate
exact boundary roles/content before stripping them on import, never arbitrary
user controls or visible text. Existing producer superscript/font/page differences
remain explicit. RICH_CITATIONS.md records structural and independent visual QA;
no source oracle or runtime/app boundary was changed.


## 2026-11-24 Resolve exact ActualText boundaries before moving notes

Use full character equality to preserve structural ActualText run intervals and
citation identity, consistent with the existing content-stream exact-alignment
policy. Remove only geometric synthetic separators; literal spaces remain text.
An isolated preliminary traversal is needed before moving note bodies/replacing
citation text. Its copied consumption and diagnostic state cannot claim original
paint or leak preliminary warnings. Main recovery verifies exact characters again.

Never map a repeated label substring into different replacement text. Keep opaque,
nontext/multiple-paragraph and definition-containing replacements as visible text
with graph evidence/diagnostics. A definition cannot move outside its replacement
range merely because its body text happened to match. Reject an internal changed
correspondence rather than silently swallowing a prepared atom. Independent raw
PDF objects/operators, rich asset/style/graph checks and native/Word visual QA qualify
the exact case; broader nested/untagged graphs and original producer qualification
remain open. No canonical schema or runtime dependency changed.


## 2026-11-24 Qualify original Office exports and preserve positional separators

Add pinned, unmodified Apache POI fixture exports with upstream license/notices,
actual editor metadata and independent original ZIP/XML expectations. Compare
relationships, paragraph/reference positions, marks, stories/geometry and assets
rather than accepting parse success. Preserve the established final-text/cached-
field/comment-diagnostic policies; review-markup printing is a different view.

Visual inspection found actual w:ptab controls silently concatenated header/footer
labels. Tighten the source oracle, reproduce the failure, then preserve tab
separation and diagnose alignment/base/leader loss. The logical SF does not claim
Word tab-position layout; metadata must not hide the loss. Reject malformed
nonempty controls. Original source files remain unchanged. Independently distinguish
package/native links from LibreOffice's observed zero-annotation exports, and use
real caller-supplied Unicode/style fonts rather than silent glyph substitutions.
REAL_DOCX_CORPUS.md records all33 inspected pages,762 actual PM nodes, commands and
remaining broader graph/native-adapter/final-audit work. Runtime modules unchanged.


## 2026-11-24 Distinguish nested definition containment from citation ownership

Recover independently cited source-nested definitions as separate editable notes,
retaining their validated parent_element and logical preorder. Child-first physical
materialization must not reverse definition order or copy a child body into its
parent. Source containment does not authorize recursive inline references. Diagnose
the flattened authoring containment while keeping original parent/ID/Ref/geometry.

Treat a same-target single-inline-child Ref wrapper as another explicit owner of
one citation, not another occurrence. Keep each original owner/target edge on the
specific atom. Different targets, extra paint and opaque replacement boundaries
remain uncollapsed source evidence/uncertainty. Joining paragraphs previously lost
wrapper block-level Ref edges even without conversion; aggregate original block and
run sources before flattening. Independent PDF object/graph/asset and native/Word
reader/visual checks qualify these rules, including nested mixed foot/endnotes.
No source/oracle or runtime dependency was changed. PDF_NOTES.md records the eight
inspected pages,130 actual PM nodes and broader recovery/native-adapter/final work.


## 2026-11-24 Ground native interoperability in the executable pinned reader

Start the native bridge with UTF-16 scalar safety and actual container/control
ownership, not a flat text wrapper. Keep private prerequisites private until rich
SF mappings are usable. Published table-end prose was stale; the executable 1.0.4
enum/parser establishes0x0E/0x0F, with0x1E/0x1F reserved for annotation controls.
Cells require CR/LF and tree ends differ from exclusive table metadata. Native
geometry and fs also have different units; preserve these distinctions explicitly.

Use independently hand-authored native records and actual Colleaf validator/core/
view/tree APIs to qualify offsets/styles/links/note references/tables. Do not copy
implementation code or embed SF as a second authoritative text payload. Actual
rich mapping/edit/save qualification remains required; current internal token and
metadata validation does not fulfill it. Optional SDK tools stay outside runtime,
production modules, application authorization/quota/save responsibilities.


## 2026-11-24 Qualify native semantic workers through SF and native mutations

Map actual native stream/style/link fields before exposing a native format.
Select the last overlapping text record as a whole, then apply declared inherited
style layers; do not union overlapping marks. Keep UTF-16 inclusive links separate
from exclusive style intervals and retain fs in points. Native authored language
has an explicit lang field in the pinned declarations. Script-specific alternate
language intent still needs a checked mapping.

Cross the canonical SF in tests, including its normalized internal href plus
matching destination, and re-import actual TextX edit/save output. Native edits
must win without consulting another text payload. Unknown presentation has explicit
diagnostics, hidden content and unmapped containers reject in these private workers.
No public rich-adapter support exception or runtime editor dependency is introduced.


## 2026-11-24 Validate native merge continuations and width semantics executably

SF origin-only table rows cannot be copied directly into native source rows.
The executable pinned reader requires empty rowSpan0 continuation records to
advance covered vertical grid columns. Omitting them shifts following cells left.
Keep native cell stream/source ordinals exact, check combined merge occupancy,
and reject visible continuation content. Accept checked optional horizontal
zero/zero placeholders without copying their empty bodies into SF.

Use full-width specified percentages for SF's equal-width default. Native zero
unspecified widths produced invisible table geometry. Legacy table unit0 POINT
values also behaved as pixels in the pinned layout, unlike their enum label;
qualify that field-specific behavior alongside unit3 and keep font fs in points.
Inspect skeTables as a Map, not a JSON object: earlier JSON dumps hid its entries.
Table geometry is now qualified without claiming browser/font/visual fidelity.
Keep native edits authoritative, diagnose mixed paragraph normalization and extra
presentation, and keep the rich adapter private until remaining mappings work.


## 2026-11-24 Bind native note identities without a second text authority

Native notes have one ID namespace; SF scopes IDs by foot/endnote kind and retains
definition order. Preserve these authoring identities in a sparse named native
resource, checked against real note ID/kind records. Do not cache note text, labels
or the entire SF there. Native edits to bodies/custom marks/styles remain real
fields and win directly. Ignore stale identity bindings with diagnostics; reject
bindings that alias distinct definitions. Removing the resource leaves native
IDs usable with deterministic definition order.

Treat the matching first body-note atom as the explicit body marker allowed by
the schema, not a recursive note reference. Preserve its style, reject nonleading
recursive atoms and contradictory hidden/explicit markers. Basic graph/text/style/
merged-table semantics now cross SF and actual reader/edit/save checks. Advanced
citation/body-label variants, note settings/assets/lists/provenance and generated
note placement remain unfinished private mappings. Headless metadata checks do
not qualify note layout or substitute for the final acceptance audit.


## 2026-11-24 Keep native list presentation authoritative and ownership checked

Use native bullet/list sources for real markers/counters. Native starts are zero-
based:0 displays1, -1 displays0. Share nested counter groups unless independent;
resolve canonical SF parent contexts without mutating callers. Qualify glyph text
through actual renderer LIST glyphs: the counter history cache resets deeper
levels and cannot prove all previously rendered markers.

Native flat bullets do not identify SF continuation paragraphs/tables or explicit
adjacent-list boundaries. Preserve only that missing ownership/implicit intent in
a checked per-story resource bound to actual paragraph IDs/type/group/counters.
No item text or whole SF is cached. Native edits invalidate stale ownership and
fall back to explicitly disclosed bullet inference, preserving visible block order.

Executable probes also found literal percent and nondecimal-zero differences,
and absent-level template dereferences that throw in the pinned renderer. Project
ignored deeper placeholders safely and restore sparse original intent only when
it still matches native fields. Reject the other private unmapped cases explicitly;
these are unfinished work, not public adapter completion/support exceptions. The
later dynamic literal-marker decision below supersedes the percent rejection;
nondecimal zero and broader numbering policies still need mapping. Keep
browser/resource persistence and visual fidelity qualification open.


## 2026-11-24 Store native image bytes once and check size intent against edits

Use real embedded PNG/JPEG data URIs and drawing records; reject external/hosted
sources rather than requiring services or silently dropping assets. Keep note
sources in their native story while using globally distinct drawing IDs for the
flat render registry. Native dimension units are pixels; SF stores preferred
points, optional dimensions and fractional precision.

Preserve only that missing size intent in a sparse resource bound to asset digest
and actual projected box. Never cache another copy of the bytes/text. Resizes or
asset substitutions invalidate old intent, preserving native edits. Reuse existing
Go pixel decoding/allocation checks and share the cache across main/note stories;
application image/snapshot quotas remain outside the library.

Qualify actual drawing ownership and layout boxes plus independent pixel/alpha
and original-byte checks. Those do not prove browser painting, editor resource
persistence or contextual image fitting. Keep those and remaining appearance/
profile mappings open, rather than exposing a partial native public format.


## 2026-11-24 Correct metadata transport using actual FDocument.save evidence

The pinned application calls FDocument.save, not DocumentDataModel.getSnapshot.
An executable facade probe dropped every unregistered manydoc.* resource while
regenerating registered resources. Preserve that failure as a regression; earlier
raw-model checks remain narrower historical evidence and cannot establish this
API's persistence.

Move checked, nonvisible intent records to one native CUSTOM annotation on the
terminal LF. It survives facade save/fresh reopen and normal TextX shifts without
copying the SF/body/assets. Check namespace/version/range/record identity and
existing semantic bindings; native edits remain authoritative. Retain read support
for older private resource transport. Verify registered drawing mirrors before
ignoring duplicate cache data. Do not inspect SDK permission payloads or add
performer-level gates. Browser/preset/render-service-dependent operations remain
unfinished rather than being inferred from successful headless saves.


## 2026-11-24 Compose native sections against executable story lookup

Retain the established SF as the canonical representation. Native page/section
properties and header/footer streams are an adapter projection, with sparse guarded
intent for precision/default omission and metadata that the native schema lacks.
Do not flatten foreign section properties into an implicit plain body. Preserve
nil inheritance versus explicit empty story overrides; native edits take priority.

Executable renderer checks found local header image sources were not rendered,
even though published reference-source types allow them. Promote running-story
assets to globally scoped document catalogs, including tables/list types and image
registry order. Keep one authority per asset. Validate references/orphans rather
than passing an arbitrary subset that silently hides unrelated visible sources.

Use actual sectionType/next override semantics and authored/soft page-break
separation. Disable native automatic hyphenation in generated projections to avoid
facade-injected behavior changes. Record measured seven-page selection/geometry
and actual FDocument.save/fresh-reopen equality, without claiming browser/font
fidelity. Contextual fitting, advanced intent and public native API remain work.


## 2026-11-24 Fit native frames and correct citation atoms using placement evidence

The pinned renderer retained an oversized 1000×2000px drawing on a 320px body;
source size equality was insufficient to establish usable placement. Bound individual
native image frames using actual section/cell/story contexts, preserving original
preferred dimensions in guarded fit intent. Reuse pure-Go raster/fit helpers; do
not add a runtime browser or claim complete native story/font layout from boxes.

The same skeleton probe exposed that the old custom-block note projection never
entered native note layout. Earlier source/view/save checks were narrower evidence,
not placement proof. Emit/read actual FOOTNOTE/ENDNOTE U+FFFC custom ranges and
retain the old input representation only for private compatibility. Keep the
original structural tests and add genuine glyph/placed-body assertions, including
table citations. No original document or fixture oracle was altered.

Stabilize generated shared SF automatic labels with real native customMark values,
plus sparse checked automatic intent in the existing carrier. Native default
endnotes are Roman, and repeated references consume native counters; neither may
silently change shared SF labels. Restore implicit numbering only when actual
marks/citation order match. Native edits retain visible marks with diagnostics.
Foreign numbering/settings and advanced marks remain required unfinished work.


## 2026-11-24 Resolve foreign counters into current explicit marks

Do not reinterpret native implicit endnotes as SF decimal defaults. Resolve native
root/section counters and format outcomes before rich body decoding, preserving
first/current citation labels and the final generated body mark on repeated notes.
Represent those current values with existing SF literal-label fields. Explicitly
diagnose loss of live restart/placement/column policies; do not invent another
canonical settings schema or hide the loss behind successful text extraction.

Qualify exact labels/body marks against the actual pinned skeleton and facade,
including zero starts, custom marks not consuming counters, markerless labels,
section restarts and repeated references. Keep unproven automatic per-page and
editable separator/notice cases explicit and rejecting while their mappings remain
unfinished. Keep native output for independent/rich marks on the required queue.

A genuine saved-input comparison exposed the earlier assumed foreign running
geometry default: native constructor/save fills 30px, so import must use 22.5pt.
Do not revise the source or ignore layout in the equality check. Independent
Word/PDF output relationships, annotation boxes and four visually inspected PDF
pages qualify this targeted case; they do not establish native-browser/Word parity.


## 2026-11-24 Keep independent body marks in editable native source

The native note schema has one generated shared marker. Independent/rich body
marks must be real runs and hyperlinks in the native note story, with the generated
body marker suppressed. Recover their SF role using a small local annotation and
checked paragraph/delimiter identity. Do not store another copy of the visible
label/body/SF or force old text over a real editor edit.

Executable rendering exposed a CUSTOM-range color/underline override even though
source styles were correct. Use neutral native active/textColorMode settings and
retain the failing-render regression alongside actual glyph/style/link assertions.
Actual TextX replacement also moved the annotation start past inserted text;
recover the leading mark through the stable paragraph and owned delimiter. Missing
or stale bindings preserve visible text with diagnostics instead of hiding it.

Qualify literal/rich/styled, table-first, empty and markerless-style cases through
real facade save/reopen and actual text/style edits. These prove this body-mark
projection, not per-citation rich output, browser paint/interaction, total-story
layout or native adapter completion. Keep those on the required next-work queue.


## 2026-11-24 Keep native per-citation labels editable and notes shared

Native generated labels are shared per definition. Represent different/rich/empty
citations as real editable text beside real markerless note atoms, with neutral
ownership annotations and an explicit end delimiter. Preserve a single note body.
Use existing SF per-reference/shared/body-label fields, with sparse checked intent
for the shared value not expressible in the markerless native field. Do not cache
whole labels/styles/bodies or force old values over editor changes.

The actual pinned superscript enum is 3; a symmetric Go-only round trip would not
have detected a wrong enum. Actual glyph checks are required. TextX replacement
also split one annotation into two fragments; preserve that unchanged edit as a
regression and combine only identical stable-token fragments. Validate original
records before normalization so malformed inputs cannot be hidden by joining.

Import derives current text/style from native intervals, preserving source style
precedence, and discloses stale/unsupported ownership. Long paragraphs use bounded
style searches and linear folding; a 2,048-reference case checks ordering/defaults.
Headless glyph/relationship/save/edit checks do not prove browser paint, atomic
wrapping, selection/click/navigation behavior or full native adapter completion.


## 2026-11-24 Validate native JSON before decoding changes its meaning

The actual native file boundary uses Colleaf's exact v3 envelope and pinned engine
identity. Keep it private until remaining mappings and interaction are qualified.
Do not rely on encoding/json alone: it accepts duplicate keys, malformed Unicode,
case aliases for struct tags and null scalar values. Scan original bytes first,
checking decoded object-key identity and Unicode scalar validity; preserve valid
surrogate pairs and literal replacement characters. Reject ambiguity before native
UTF-16 ranges or relationship maps are interpreted. Check interpreted JSON-in-string
resource payloads independently, including sparse carrier records. Do not parse
application permission resources as library admission policy.

Scan large string values in place; bound parser depth for stack protection, without
copying Colleaf's application byte/image budgets into the library. The whole input
is currently buffered, so broader resource auditing remains required. Actual
facade save adds settings even when omitted by the producer. Accept the documented
empty/positive zoom settings, disclose lost UI view/locale state and reject unknown
settings. Preserve unchanged independent-reader output as the regression rather
than removing fields to make a symmetric Go test pass. Envelope/facade evidence
does not establish browser interaction or complete native compatibility.


## 2026-11-24 Keep native literal list markers dynamic

Native glyphFormat has no percent escape and interprets %10 as level ten; SF/OOXML
interprets %10 as a level-one counter followed by0. Compile literal fragments to
non-active native fixed-symbol entries beyond the nine real levels. Keep real
counter placeholders and paragraph levels intact. Actual pinned glyph and facade
save/reopen evidence confirms this mechanism, including all-nine-level templates.
Do not replace the list with frozen per-item marker strings.

Reuse sparse checked original-template intent for ignored deeper placeholders and
unrepresentable normalized spellings. Match current format/helper values before
restoring it, so native edits win. Recover unbound representable helper templates
symbolically; reject a literal percent-digit sequence that would falsely become a
counter in SF. A numeric native glyph with a fixed symbol must import that actual
symbol, not silently infer the numeric format's changing label. Bound templates
and helper expansion by the existing SF representation. Counter edits resolve
through canonical SF's existing enclosing-counter rules; stored ancestor values
are fallbacks, not proof that an edited enclosing counter was lost. Full browser
and zero/legal numbering qualification remain required.


## 2026-11-24 Preserve SF zero/legal semantics with checked native format variants

Keep canonical SF's existing nondecimal-zero0 and fully decimal Legal meanings.
Native emits00 for decimalZero0 and empty letter/Roman0; its Legal flag preserves
padded decimalZero. Project only the needed glyph fields to decimal. A first-zero
variant and normal type share the actual native counter group, so labels remain
automatic. Bind item-type aliases and sparse original formats/zero-level conditions
rather than caching marker strings, counters or body trees. Scope those identities
when promoting running stories into the native global catalog.

Restore format intent only while current glyphs/legal flags/counters agree. Edits
must win. Check every item's effective numbering before restoring a container;
otherwise a changed first variant or a foreign zero could be hidden under one old
list format. Derive current foreign zero/legal presentation through explicit SF
templates/effective formats and disclose live-policy loss. Split groups at semantic
changes. Validate canonical integer member names before Go map conversion, bounded
levels/format conditions and projection ownership. Aggregate diagnostics in stable
order and deduplicate primary bindings to avoid alias-driven quadratic validation.

Actual pinned glyph/source/facade evidence qualifies default/compound/ancestor
zero, legal padding, main/cell/note/header catalogs and current counter/glyph edits.
It does not qualify browser/preset controls, all live native numbering behavior or
full adapter completion. Remaining appearance/intent/API/browser and broader goal
requirements stay open.


## 2026-11-24 Follow the actual registered hyperlink persistence path

Core/facade tests without the hyperlink plugin did not establish the app's real
URL persistence behavior. Pinned docs-hyper-link1.0.4 saves DOC_HYPER_LINK_PLUGIN
and applies payloads by global rangeId across main/header/footer models on load.
Scope exported running-story hyperlink IDs. Apply the known resource precedence,
with diagnostics for missing/different body URLs or inactive entries, rather than
ignoring it or treating a resource-only editor target as an unrelated mirror.
Reject malformed/ambiguous identities and conflicting external/internal targets.
Keep note-local URLs outside this resource, matching the actual plugin's scope.
Copy modified containers so import never mutates caller snapshots.

Qualify two actual registered-plugin saves/reopens, current URL-only resource edits
and exact Go/SF semantics, retaining text, style and relationship checks. Compare
only the declared0/1 unit-span normalization separately. Use hash-pinned unmodified
UI handler source against real loaded models to test external target selection
with a window.open stub. Explicitly distinguish that from loading widgets, browser
popup/event/paint behavior and internal bookmark navigation; those remain open.
Published UI source confirms bookmark/heading lookup mechanisms for the next
mapping step, but source inspection alone is not implementation/interaction proof.
No third-party implementation is incorporated into production Go or required at
runtime; optional QA source/package identities and notices stay attributed.


## 2026-11-24 Map native navigation as current points, not hidden text

Use real BOOKMARK3 annotations at scalar UTF-16 boundaries. Do not insert invisible
or visible characters to carry anchors. A point can precede an astral scalar even
though an inclusive formatting range cannot end on its first surrogate. Keep
point validation distinct from formatting extent validation. Preserve paragraph/
heading block placement through small version1 role/paragraph-ID hints and honor
current native edits; stale ownership becomes an inline point with a diagnostic.
No marker text, styles or document bodies are cached.

Native lookup accepts rangeId and bookmarkId in one namespace. Avoid every logical/
missing identity when allocating transport IDs, so an explicitly missing link
cannot accidentally resolve. Canonicalize actual main-body range aliases; namespace
foreign heading identities and retain typed SF destinations. Normalize redundant
fragment URLs consistently across SF/native round trips. Preserve known missing
IDs, diagnose new ones and reject points left at unrepresented structural cursors.
Native bookmark range editing extent becomes a disclosed start-point loss.

Pin save/reopen, actual TextX cursor movement and real selection-manager refresh
requests from the unmodified UI handler. Stub render/window services in this probe
and state its limits. These are not browser caret/scroll/event or full adapter
completion evidence. Structural/standalone/container anchors and nonmain target
lookup/remaining intent/API/browser qualification stay on the required queue.


## 2026-11-24 Bind container navigation to current native tokens

Table/image points belong to the current native table control/image atom and its
source ID, rather than a synthetic paragraph or a hidden cached SF block. Extend
sparse version1 placement hints with table/picture roles and ownerId; reject
contradictory paragraph/owner bindings. Rebind promoted running-story table IDs.
Classify real structural starts before paragraph fragments can consume them.
Current offsets win: foreign hint-free starts attach to the current container;
moved points in text retain inline locations, with stale-placement diagnostics.
Do not silently omit unknown bookmark metadata or range editing extent.

Qualify pinned real token/source bindings, plugin saves/reopens, TextX insertion
and internal selection-manager refresh requests across rich nested/story content.
The saved edit includes an independently styled text interval, so check that
actual interval graph instead of assuming the editor coalesces it into old text.
No original source oracle changes. Standalone/list/section/page-break anchors and
remaining structural cursors, nonmain lookup and browser qualification stay open.


## 2026-11-24 Restore current flow ownership without moving points

Native lists have no enclosing stream token. Bind list intent to the first
paragraph ID plus current point position, and promote only at the reconstructed
list's current start. A point moved within the same paragraph must remain inline;
matching an old owner ID alone is insufficient. Stale list grouping does not
justify moving or deleting its point. Keep metadata/range losses explicit.

Use actual hard-break controls and current section-ID/start bindings. Preserve a
soft-break point even when omitting that layout boundary. Keep section and child
points distinct at shared offsets, including empty sections. Do not cache body
content or insert characters for ownership. Qualify actual save/edit/selection
requests and the complete strict native/SF graph, without browser fidelity claims.

The temporary first-section note composition may need references to omitted root/
other-section anchors. Admit those only in that internal projection, after full
input validation. Persist the original unresolved declarations and validate final
composition against the complete actual graph. This is document interpretation,
not an application authorization or quota gate. Standalone/other structural
points, UI/API and the broader acceptance queue remain required.
