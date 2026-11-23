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
