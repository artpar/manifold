# Internal destinations: source evidence and implementation recovery

## Current implemented change: Word external URI fragments

When a Word hyperlink has both r:id and w:anchor, DOCX import now appends the
fragment to the relationship target, keeping Word's actual URI. This is the
published Word compatibility behavior in
[MS-OI29500 2.1.521](https://learn.microsoft.com/en-us/openspecs/office_standards/ms-oi29500/df06e423-11a6-4a36-bfb3-82139e531781).
A strict OOXML reader may ignore the anchor when r:id is present; import emits
`hyperlink_word_anchor` to disclose that interpretation. Canonical SF/JSON/DOCX
keeps the resulting IRI. Existing PDF UTF-8-to-ASCII URI encoding remains intact.
This does not implement bookmark navigation yet. No API/schema/runtime dependency
changed in this milestone.

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
change the canonical Word values. Remaining internal fragment strings are still
URI actions in native output until the typed-destination work below lands.

The new full native file retains 189 pages. Independent before/after checks prove
all 189 page-content streams and annotation rectangles exact against the numbering
milestone, with five changed URI annotations and all 435 target identities exact.
There are 1,226 URI annotations. Fonts, JPEG, metadata and required Unicode checks
pass. No visual layout changed; previously inspected numbering pages remain the
paint baseline. This is a URI fix, not a full-source fidelity/completion claim.

## Original bookmark facts (not implemented navigation)

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

## Concrete next implementation

Implement typed flow anchors/internal links in the existing ProseMirror profile.
Keep generic URI references (including #fragment) distinct from explicitly typed
internal destinations, preserving the URI regression example. Candidate public
additions are inline zero-width anchor atoms, explicit link-destination IDs,
block-start anchor attributes, and declared unresolved destination identities.
These names are design notes, not current API/support claims.

Preserve inline bookmark starts at their logical positions. Do not insert visible
blank paragraphs for the 33 between-block markers; bind them to the following
block start, with an explicit zero-height end marker if needed. Define what
happens to bookmark range-editing/column semantics separately from point navigation.
Validate duplicate IDs, unresolved references, malformed metadata and unsupported
output. Word's portable-name length/character constraints need deterministic,
collision-checked mapping with checked metadata for application IDs.

For PDF, bind anchor placement after wrapping/pagination/continuation admission,
not during measurement. Anchors must not alter shaping, glyph coverage, bidi,
grapheme segmentation or text. `pdfBidiRange.start/end` count original scalars
before NFC/note substitution; post-layout zero-width spans can carry binding
information without putting placeholder characters into the shaped paragraph.
Define cluster-edge and repeated-story/header policies explicitly and test them.

`finishGlyphFonts` already parses/rewrites output through pdfcpu; it is a possible
shared finalization point for destination name trees and typed GoTo annotations,
including files without custom glyph faces. Keep known unresolved named targets
unresolved. Do not route them to an invented page or silently remove their links.
Any temporary annotation marker scheme must be collision-free with real URI values
and must never escape the staged output buffer.

PDF import must handle Dest and GoTo, direct arrays and named destinations/name
trees, forward page references, transformed coordinates and nullable view parameters.
Retain explicit physical destination/view evidence separately from any inferred
binding into the recovered flow tree. `pdf_links.go` currently only has URI strings;
`logicalLink`, geometric ownership and run-style copying need typed destinations too.
Resolve after all page references exist. Keep source target/view data and uncertainty
rather than imply exact authoring semantics from page geometry.

Required tests: intra-paragraph/empty/block/section/table/note/running targets,
Unicode names/URI distinction, forward/backward links, wrapping/continued and
repeated tables, bidi/ligatures/combining text, missing and duplicate IDs, named and
array PDF destinations, rotations/crops, explicit versus inferred target ownership,
source relationships/assets, independent readers and actual target page/position.
Use the 3,106-start original fixture alongside targeted cases. Keep paragraph,
merge, numbering, font and URI oracles intact; do not count successful parsing or
visible text alone as successful navigation.

The complete goal remains active. After destination paths work, audit required
rich PDF semantics, source/corpus qualification and the 451MB verbose SF/codec cost
against the actual acceptance criteria; avoid an unbounded Word-fidelity expansion.
