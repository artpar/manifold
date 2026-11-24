# Internal destinations: source evidence and implementation recovery

## Implemented: SF/DOCX points and native PDF navigation

The shared model now has `Block.Anchors`, zero-height `AnchorPoint` blocks,
zero-width `Run.Anchor` atoms, typed `Run.Destination`, and explicit
`Document.UnresolvedDestinations`. The ProseMirror profile carries block `anchors`,
inline `anchor` nodes (`attrs.name`), `anchor_point` blocks, `link.destination`,
and root `unresolvedDestinations`. Generic `Run.Link` references, including a bare
`#fragment`, remain URI references. A typed link's optional href must refer to its
same ID; an absent href derives a percent-encoded fragment for SF interoperability.
Names are nonempty valid UTF-8, at most 1,024 bytes; duplicate definitions,
contradictory hrefs and undeclared dangling targets reject before output. Anchors
cannot own visible text, formatting, notes or links. Source marks remain available.

DOCX import keeps bookmark starts at their inline positions and folds native
between-block markers into the next block, retaining a trailing zero-height point.
Range editing and column selection become explicit `bookmark_range` losses; point
navigation survives. Missing names become sorted explicit declarations with
`internal_destination_missing`. Existing declared missing names do not accumulate
new warnings on SF/DOCX round trips. Global names and part-local numeric pairs are
checked, including forward links. Export uses standard bookmark pairs and anchor
hyperlinks. Arbitrary IDs get deterministic collision-checked portable names;
ignorable, validated metadata retains original IDs, grouped standalone boundary
points and missing-target declarations. No blank paragraph is added for a point.
Required empty-cell/table-tail paragraphs stay structural even beside point pairs.
Zero-content vMerge continuation anchors move to their origin with an explicit
placement-loss diagnostic. Authored anchored paragraphs after an unconditional
break use a separate checked break control so the anchor cannot be consumed.

Native PDF creates a UTF-16BE destination name tree and standard named GoTo actions.
Positions bind only when content is admitted/painted, after wrapping, bidi/shaping,
pagination and table/note continuation. Inline points open the containing line at
x=0; no exact caret/grapheme-internal x is claimed. A point at the end opens the
last line. Block points open the admitted block top; standalone points bind the
current boundary. Repeated running stories/table-header points bind their first
painted occurrence. Unresolved targets retain a named action without an invented
binding. Existing note links remain separate. Intermediate URI tokens are allocated
outside every actual URI prefix and are replaced inside the staged PDF buffer;
they never escape as a conversion result. Glyph-font and destination finalization
share one pdfcpu parse. No production dependency or external runtime was added.

`examples/destinations` runs SF/DOCX/native PDF and also records the current PDF
import result, which **does not yet recover these destinations**. Tests verify SF,
JSON and DOCX navigation semantics, forward/backward links, zero-width ligature
boundaries, missing names, graph admission, grouped/tail points, empty/nested cells
and exact source identities. Actual ProseMirror checks 26,548 nodes/marks/attributes
across both example SFs and the original Word-source SF.

Independent pypdf/MuPDF checks verify 17 example names, actual page/top positions,
Unicode names, URI distinction, missing targets and existing note navigation.
Its page-content bytes match a version without navigation points. Both native
pages and both LibreOffice DOCX-render pages were visually inspected. LibreOffice's
ODT import retains all 17 public names and ten link target/text pairs, including
running content. Its PDF export omits most links; an independent minimal
python-docx internal-link probe reproduces that omission. ODT import is the DOCX
navigation evidence; that export is visual evidence, not a successful PDF link
oracle. No Word-equivalent reflow or general producer parity is claimed.

The original Word import retains all **3,106** bookmarks and **379** visible internal
link IDs. Its new native output has 189 pages, 1,145 GoTo annotations and 81 URI
annotations (56 URI targets). Every page-content stream and annotation rectangle
is exact against `eacd33c`'s URI-stage output. The only unbound name is the actual
absent `_Introduction`, with three links. Original source fixtures and original
oracles remain unchanged; independent XML supplies the point/link identity oracle.
The original fonts, embedded JPEG, metadata and Unicode checks still pass.

PDF-to-SF internal destination recovery is the next required implementation, not
an unsupported-feature exception. It must retain explicit physical view data
separately from inferred flow binding, handle direct/named/forward/nullable and
transformed destinations, and preserve link ownership in tagged/geometric paths.
The full goal remains incomplete.

## Previous milestone: Word external URI fragments

When a Word hyperlink has both r:id and w:anchor, DOCX import now appends the
fragment to the relationship target, keeping Word's actual URI. This is the
published Word compatibility behavior in
[MS-OI29500 2.1.521](https://learn.microsoft.com/en-us/openspecs/office_standards/ms-oi29500/df06e423-11a6-4a36-bfb3-82139e531781).
A strict OOXML reader may ignore the anchor when r:id is present; import emits
`hyperlink_word_anchor` to disclose that interpretation. Canonical SF/JSON/DOCX
keeps the resulting IRI. Existing PDF UTF-8-to-ASCII URI encoding remains intact.
That earlier milestone did not implement bookmark navigation or change the API/schema/runtime dependencies.

Four original Word elements have three distinct split targets: the MQTT technical
committee URL plus technical, IPR plus Non-Assertion-Mode (twice), and TC process
plus wpComponentsCompLang. Every complete target occurs literally in the unchanged
publisher PDF. Two base targets do not occur there at all. Tests cover the actual
source target/text ownership plus five literal cases, including Unicode, existing
escapes, an existing fragment and an empty anchor, through SF/DOCX/PDF.

The old `mqtt-v5.0-oracle.json` is unchanged: its 434 target/text hashes describe
raw attribute projection, which ignores anchors beside r:id. The independent
`audit_mqtt_destinations.py` still proves every one of those hashes directly from
original XML. A separate `mqtt-v5.0-word-links.json` records 435 interpreted Word
targets and text hashes. It never reads manydoc output and also checks the four
split targets against the original publisher. The new Go import test compares
against this independently established behavior, without rewriting source facts.

There are 56 unique external Word targets and 56 publisher URI targets. Three
original spelling differences remain explicit: IBM, Microsoft and Privacy Shield
URLs have an empty path in Word and a trailing slash in PDF. The audit checks
those exact three differences; it does not normalize arbitrary URI components or
change the canonical Word values. Typed internal links now use GoTo actions; the earlier URI-stage output is the paint baseline.

The new full native file retains 189 pages. Independent before/after checks prove
all 189 page-content streams and annotation rectangles exact against the numbering
milestone, with five changed URI annotations and all 435 target identities exact.
There are 1,226 URI annotations. Fonts, JPEG, metadata and required Unicode checks
pass. No visual layout changed; previously inspected numbering pages remain the
paint baseline. This is a URI fix, not a full-source fidelity/completion claim.

## Original bookmark facts

`audit_mqtt_destinations.py` checks original Word/PDF SHA values and records:

- 642 hyperlink elements, of which 593 carry visible text and 49 are empty.
- 68 relationship-linked elements, with 55 distinct raw relationship targets.
- 574 anchor-only elements with 389 distinct names, including empty elements.
- 3,106 unique bookmark starts and matching end IDs/names; 3,073 are inline and
  33 occur between blocks.
- `_Introduction` is absent from the bookmark names but referenced by three
  visible links. Preserve its unresolved identity explicitly; do not invent a
  successful target.

Raw projection has 444 targets if empty hyperlink elements are included, versus
434 in the existing visible-text oracle. Do not confuse element counts, target
counts, raw attributes and actual Word URI behavior. The original fixtures and
original golden files remain unchanged.

## Next required implementation: PDF physical views and flow bindings

PDF import must handle Dest and GoTo, direct arrays and named destinations/name
trees, forward page references, transformed coordinates and nullable view parameters.
Retain explicit physical destination/view evidence separately from any inferred
binding into the recovered flow tree. `pdf_links.go` currently only has URI strings;
`logicalLink`, geometric ownership and run-style copying need typed destinations too.
Resolve after all page references exist. Keep source target/view data and uncertainty
rather than imply exact authoring semantics from page geometry. A PDF-only physical
view is not automatically the same destination after DOCX or native PDF reflow.

Bound name-tree recursion/object counts, reject duplicate or contradictory target
identities, and preserve actually missing named targets explicitly. Arrays need
collision-free application IDs distinct from original named strings. Keep the view
kind, nullable source parameters, original page reference and transformed physical
coordinates; a null coordinate means retaining the viewer's current setting, not
an invented zero. Flow placement inferred from nearby recovered content needs its
own evidence/diagnostic and must not overwrite the explicit physical source view.

Add independent fixtures for named/array destinations, forward pages, rotations,
crops and nullable views. Test both geometric and tagged Link ownership, including
partial/overlapping targets. Re-read the native example through PDF→SF and verify
actual destination definitions, link IDs and physical positions, not just visible
text. Keep native note destinations separate from inferred semantic note recovery.
Test wrapping/continued/repeated tables and combining/bidi cluster-edge points
beyond the current ligature boundary and all-story examples. Source relationships,
assets, heading/list/merge/font/text/URI oracles must remain intact.

Next concrete commands are in STATUS.md and TESTING.md. After destination paths
work, audit remaining required rich PDF semantics, source/corpus qualification and
the 451MB verbose SF/codec cost against the actual acceptance criteria; avoid an
unbounded Word-fidelity expansion. No external blocker prevents this work.

## Milestone check record

Full tests pass (library 14.107s / CLI 0.674s), race (171.846s / 1.981s), vet,
module verification, host CGO-disabled build/execution, Linux/amd64 cross-build and
whitespace check. SF/DOCX ten-second fuzz campaigns with navigation seeds passed
6,753 / 305 executions, two workers each. Raw logs and generated independent QA
are ignored under output/destinations; durable results are in verification.json.
No claim of successful PDF navigation import is included in these results.
