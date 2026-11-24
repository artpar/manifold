# Current status and recovery

Updated 2026-11-24. Read this first, then AGENTS.md, ARCHITECTURE.md and DECISIONS.md.
Commit verified milestones with corresponding KT/evidence updates, as requested
by the user. Generated documents, binaries, source downloads and QA environments
remain ignored. Git history and verification.json retain the historical milestones;
this file and ACCEPTANCE.md are the current authoritative queue.

## Outcome and current contract

The full rich-document goal is **active/incomplete**. The standalone Go library
and CLI implement DOCX→SF, SF→DOCX, SF→PDF and PDF→SF around a versioned established
ProseMirror JSON profile, manydoc-prosemirror/1. Legacy model JSON/text remain
available; .sf.gz is standard gzip around the same SF, not another schema.

Required paragraphs/headings/formatting, links/internal destinations, lists,
merged/nested/continued/repeated tables with widths, embedded images, metadata,
section/page settings, conditional running content and rich foot/endnotes exist in
the shared model and generation paths. PDF import retains text/images/physical
pages/positions, forward logical structure and separately identified geometric
inference, source styles and physical navigation views. Read COMPATIBILITY.md for
the declared support contract, explicit losses and output rejection. No Word-
equivalent layout or full UniOffice/UniPDF API parity is claimed or required.

Production is pure Go, with no CGO/executables/services/license servers/customer
uploads. Daptin identity/access/quotas/storage/actions remain outside the library.
Proprietary UniDoc implementations have not been copied or translated; no formal
clean-room claim is made. go.mod/go.sum and THIRD_PARTY.md track reused components.

## Actual Colleaf inspection changes the integration evidence

The earlier local-only search missed the remotely available application. Read-only
GitHub inspection now pins artpar/colleaf at
5ce2ef507b28b92f5b03443ade87bc0e17f3d8b6 and matching published Univer 1.0.4 types.
Its .vdocs is a version-3 wrapper around IDocumentData with indexed streams,
source tables/drawings/stories/notes and plugin resources. SF cannot be renamed
into that native format. A checked standalone adapter through SF remains open.
Read COLLEAF_FORMAT.md for exact primary sources, comparison and mapping queue.
No Colleaf or Daptin code was modified; app persistence/authorization is not part
of the adapter. Native snapshot budgets do not become library quota gates.

## Latest progress: dynamic native literal-percent and digit-suffix markers

Native list output now compiles SF/OOXML single-digit placeholders into the pinned
native grammar, which reads whole decimal indices and has no percent escape.
Non-active fixed-symbol entries at index9 onward carry literal fragments; real
paragraph levels remain0..8 and real counters/formats stay automatic. This supports
literal %, percent-only/constant templates, digit suffixes such as SF %10, all
nine real levels, compound/continued lists, cells, notes and headers. Helpers are
native source entries, not extra SF outline levels or frozen per-item labels.

Existing sparse OriginalTemplate intent restores ignored deeper placeholders only
when the current format/helper projection agrees. Current helper/format/counter
edits win. Unbound representable helper templates derive from current native data;
ambiguous literal percent-digit cases reject rather than becoming false counters.
Known numeric glyphs with fixed glyphSymbol now import their actual constant
marker instead of silently treating it as a changing numeric format. Template and
helper bounds follow the SF512-byte representation; actual levels remain bounded.

Pinned actual glyph checks verify twelve main/cell labels, two note labels and two
header labels using the correct global running-story list catalog. Two actual
facade saves/reopens preserve exact Go/SF/native semantics. Native percent-to-per-
mille helper edits and parent2-to5 counter edits update the real child labels;
Go/SF import resolves current enclosing counters over stored ancestor fallbacks.
These checks do not qualify browser paint, preset controls or full native parity.

Final checks pass: tests16.097s/0.717s, race168.349s/2.129s, vet/modules,
CGO-disabled focused tests/host CLI execution and Linux amd64/arm64 cross-builds.
Template fuzz106,875 and list-graph fuzz37,843 executions pass; these are short
campaigns, not exhaustive proof. Pinned old/new list readers, app validator,
facade save/reopen and final edited/saved Go/SF import0.361s pass; diff check is
clean. Cross-builds are not Linux execution. verification.json records evidence.
The goal remains active/incomplete. Next investigate/implement native nondecimal-zero and legal
numbering differences (notably decimalZero), remaining marker appearance and
navigation/nonvisual/provenance intent, then public API/CLI and actual browser/
preset interaction. Per-page/decorated notes, broader PDF/source/resource audits
and final acceptance remain required. No external blocker exists.

```sh
MANYDOC_COLLEAF_LITERAL_LIST_QA=output/colleaf-qa/literal-lists.json GOWORK=off go test -run TestColleafLiteral -count=1 .
node scripts/verify_colleaf_literal_lists.cjs
MANYDOC_COLLEAF_LITERAL_LIST_SAVED=output/colleaf-qa/literal-lists-saved.json MANYDOC_COLLEAF_LITERAL_LIST_EDITED=output/colleaf-qa/literal-lists-edited.json GOWORK=off go test -run TestColleafLiteral -count=1 .
```

## Earlier progress: strict native .vdocs envelope boundary

Private readColleaf/writeColleaf workers now read/write the exact pinned v3
vibedada-document/Univer1.0.4 envelope around the rich native projection, through
shared SF semantics. Bare snapshots/renamed SF, wrong identities/versions, unknown
typed fields and missing required snapshot fields reject. A scanner validates
syntax, decoded duplicate keys, UTF-8 and paired Unicode surrogate escapes before
encoding/json can replace text or collapse maps. Known typed record aliases and
null scalar offsets reject. Large string values scan in place without string
copies; object-key sets and schema metadata allocate. Nested interpreted resource
JSON is checked independently, including carrier records. A depth bound protects
the parser; no application byte/image quota or permission gate was introduced.

Actual pinned facade save exposed an added settings object that raw-snapshot
qualification missed. The codec accepts documented empty/positive zoom settings;
zoom and nondefault editor locale emit explicit logical-SF loss diagnostics.
Unrepresented settings reject. The unmodified hash-pinned Colleaf validator accepts
the generated envelope. Two actual facade saves/reopens and exact Go/SF reimport
preserve section/page settings, first/even/default/empty stories, tables, lists,
image bytes/relationships and metadata. Existing blank/Unicode cases pass too.
This is structural/save evidence, not browser paint or interaction qualification.

Final checks pass: tests 15.722s/0.722s, race 167.447s/2.236s, vet/modules,
CGO-disabled focused native/JSON tests and host CLI execution, Linux amd64/arm64
cross-builds, strict JSON fuzz 76,406 executions and native envelope fuzz 872
executions. Fuzz campaigns are short, not exhaustive; cross-builds are not Linux
execution. Actual app-validator/facade/saved-envelope reimport and diff checks
pass. verification.json records scope/evidence. The full goal remains active/incomplete. Next finish native list/appearance/navigation and
nonvisual/provenance intent, automatic per-page/decorated-note policies, public
API/CLI and actual full-preset/browser paint/wrapping/selection/navigation checks.
The broader PDF/source/resource/final acceptance audit remains required; native
input currently buffers the envelope and large-container allocation behavior
still needs that audit. No external blocker exists.

Reproduce this milestone with:

```sh
MANYDOC_COLLEAF_ENVELOPE_QA=output/colleaf-qa/sections.vdocs GOWORK=off go test -run 'TestColleafEnvelope|TestStrictJSON' -count=1 .
node scripts/verify_colleaf_envelope.cjs
MANYDOC_COLLEAF_ENVELOPE_SAVED=output/colleaf-qa/sections-saved.vdocs GOWORK=off go test -run 'TestColleafEnvelope|TestStrictJSON' -count=1 .
```

## Earlier progress: editable native per-citation literal and rich marks

Private native output now preserves per-reference literal/rich/empty/default labels
and inferred-relationship flags, including repeated citations in sections, cells
and lists. Real markerless note atoms retain the note relationship/placement;
editable superscript label runs and an owned zero-width delimiter carry display.
Separate real body marks preserve the note's shared default without duplicating
note definitions or whole bodies. Existing SF label fields remain canonical.

The pinned renderer checks seven markerless anchors, actual visible labels,
superscript 3 and authored color/size/bold/italic fields, plus two placed bodies.
Actual facade save/reopen and TextX/style edits retain exact Go/SF semantics.
TextX can split a label annotation around replaced text: a stable token combines
only matching fragments between the protected anchor and delimiter. Missing/stale
ownership leaves visible text/anchors in the story and reports losses; malformed
or nonneutral ownership rejects. A 2,048-reference paragraph retains every reference
and default, using effective style spans and linear paragraph consumption.

Final milestone checks pass: tests 16.855s/0.777s, race 167.692s/2.133s,
vet/modules, CGO-disabled focused tests/host CLI execution and Linux amd64/arm64
cross-builds. Citation fuzz 1,000 and document fuzz 50,749 executions pass; all twelve
pinned reader/save scripts, independent Python regressions, final combined Go
edited/saved re-import and diff check pass. Cross-builds are not Linux execution
evidence. verification.json records exact scope, timings and remaining limits.

Next complete remaining native list/appearance/navigation and nonvisual/provenance
intent, strict envelope/JSON/surrogates, public API/CLI and actual full-preset/
browser paint/wrapping/selection/navigation/interaction qualification. Automatic
per-page/decorated-note cases and the broader PDF/source/resource/final acceptance
audit remain on the required queue. No browser click/atomic wrapping or full native
adapter completion is claimed from headless glyph/save checks. Runtime stays
pure Go, with applications/backends untouched and no external blocker.

## Earlier progress: editable independent and rich native body marks

Private native output now writes literal, rich and uniformly styled independent
note-body marks as actual editable runs, including links, table-first and otherwise
empty note bodies. Native generated body markers are suppressed. A small local
body annotation binds role/representation to the paragraph and delimiter; marker
text/styles remain solely in normal source runs. Empty styled intent uses the real
referenceTextStyle field with nonvisible ownership metadata.

Pinned renderer/facade checks confirm six placed bodies, distinct citation/body
marks, actual red/green/font-size/bold/italic/underline/link fields, table placement
and empty variants. Actual TextX replacement and style edits survive Go re-import.
Native CUSTOM annotations otherwise override authored colors/underline: output
sets active=false and textColorMode=text, with a retained failing-render regression.
TextX can detach the start boundary during replacement, so recovery uses stable
paragraph identity and the owned terminal tab, rather than stale original text.
Missing/stale ownership retains visible story text and emits diagnostics; malformed
or nonneutral ownership rejects explicitly. Native API/CLI remains private.

Final milestone checks pass: tests 17.345s/0.799s, race 167.140s/2.180s,
vet/modules, CGO-disabled focused tests/host CLI execution and Linux amd64/arm64
cross-builds. Body-mark fuzz 1,000 executions and document fuzz 51,866 executions
pass; diff check and all eleven pinned reader/save scripts plus independent Python
regressions pass. Cross-builds are not Linux execution evidence. verification.json
records exact scope, timings and the remaining UI/font/layout qualification.

Per-citation output is now qualified above. Next complete remaining list/appearance/
provenance intent, strict native envelope/JSON/surrogates and public native API/CLI.
Actual browser/preset paint/wrapping/interaction and total story qualification also
remain required. Prior foreign-numbering/import and DOCX/PDF checks stay valid;
this milestone does not claim full adapter or overall goal completion.

## Earlier progress: foreign native note numbering and output proof

colleaf_note_numbering.go now recovers native automatic citation/body marks from
root and section settings: starts/restarts, decimal/zero/letter/Roman defaults,
markerless/custom marks, repeated citations with different labels and supported
symbol/Unicode formats. Current values become explicit SF labels with diagnostics
for live-counter/placement/column policy losses. First and final body marks stay
distinct; source range IDs carry per-citation overrides through cell fragments.
The actual reader confirms all ten example citations and eight placed note bodies,
fifteen format outcomes, native default Roman endnotes and facade save/reopen.

The save test also corrected foreign omitted header/footer distances: the native
constructor supplies 30px (22.5pt), not the earlier assumed 18pt. Generated own
projections already emit their distances explicitly. Section snapshot transport now
retains uncaptured properties so independent readers see actual noteProperties.

SF→DOCX/PDF paths accept the imported marks with explicit AllowLossy acknowledgment.
Independent Word XML/python-docx and PDF pypdf/MuPDF verify eight bodies, variant/
markerless relationships, nine actual citation/link boxes and shared destinations.
PDF output uses explicitly supplied regular/bold DejaVu faces for the original
supplementary character; missing font/glyph attempts rejected explicitly. All four
Poppler-rendered PDF pages were visually inspected: marks/bodies are legible with
no clipping/overlap/missing glyphs. This is a sparse targeted fixture, not a general
Word/Colleaf-browser fidelity claim. Full check evidence is in verification.json.

Final milestone checks pass: tests 17.489s/0.746s, race 167.481s/2.265s,
vet/modules, CGO-disabled focused tests/host CLI execution and Linux amd64/arm64
cross-builds; document/note fuzz 22,371/2,551 executions and diff check. All ten
pinned reader/save scripts, Pillow, independent note-output readers and combined
Go edited/saved import pass. Cross-builds are not Linux execution evidence.

The adapter remains private/incomplete. Independent/rich body-mark output is now qualified above. Next implement native
literal/rich citation output, remaining list/appearance/provenance
intent, strict envelope/JSON/surrogates, then public native API/CLI and actual
browser/preset qualification. Automatic per-page note counters still require an
authoritative layout strategy; editable separators/notices and remaining native
number formats also remain explicit unfinished cases. They reject rather than
silently guessing or dropping visible content. Pure-Go runtime and application/
backend boundaries are unchanged; there is no external blocker to the next work.

## Earlier progress: native image bounds and real note placement

Private document output now fits individual image frames to page/cell/note/running
containers, including merged/nested cells and narrower inherited story contexts.
It retains preferred SF sizes in checked geometry intent; real native resizes win.
The executable renderer does not shrink oversized frames itself: a 1000×2000px
frame overflowed a 320px body on a 600px page. The new regression preserves that
failure, checks actual projected boxes and verifies facade save/Go re-import.

That probe exposed a substantive note representation error. Earlier custom-block
checks proved transport/source access but did not place notes. Native citation
layout consumes FOOTNOTE8/ENDNOTE9 ranges on U+FFFC, now emitted and read. The
pinned reader independently checks citation glyphs and actual placed foot/endnote
bodies, including a cell citation. Checked automatic-mark intent stabilizes shared
SF note labels across repeated native references and native endnote Roman defaults;
reordered native citations keep actual marks when automatic intent becomes stale.
Older private custom-block input remains readable, but is no longer emitted.

Final checks pass: tests 18.035s/0.815s, race 168.843s/2.995s, vet/modules,
CGO-disabled focused tests/host CLI execution and Linux amd64/arm64 cross-builds;
document/note fuzz 15,990/737 executions and diff check. Linux cross-builds are not
execution evidence. Full details and limits are in verification.json.

Nine pinned reader/save scripts plus Pillow and combined edited/saved Go re-import
pass. The latest full checks are recorded in verification.json. These are frame/
structure/placement/save checks, not browser font/paint or total-story-height
qualification. Basic foreign numbering is now qualified above. Rich citation/body
marks, per-page/decorated settings, remaining list/appearance/provenance intent, strict native envelope/JSON/
surrogates and actual full-preset/browser behavior remain work. Public native
API/CLI and the full goal remain incomplete. Production stays pure Go.

## Earlier progress: native sections, page settings and running stories

Private document composition now connects page geometry, section boundaries and
first/even/default header/footer variants to the existing text/table/list/image/
note workers. Zero/fractional margins and distances, optional/inherited layout,
explicit empty stories, authored page breaks and metadata survive SF round trips.
Native geometry edits win over checked original precision/default intent. Unknown
layout properties produce diagnostics; contradictory references/orphans reject.
Native rendered soft page breaks are disclosed and removed from logical flow.

The pinned executable reader confirms seven pages, odd-page blank insertion,
geometry transitions, story selection/inheritance and header merged table/bullet/
image boxes. Header assets must live in document catalogs: local-only sources
were ignored by the renderer. Actual FDocument.save/fresh reopen retains the same
Go model. This proves structure/geometry/save behavior, not browser paint or font
metrics. The native adapter remains private/incomplete, with no public API/CLI.

Next qualify total native story layout and complete rich note/marker/list/appearance
intent and strict native envelope/JSON/surrogate handling, then actual full-preset/
browser editing/rendering and the final acceptance audit. Broader untagged PDF
note recovery and final source/resource/API checks also remain. No external
blocker exists; production remains pure Go with applications/backends untouched.

Final section milestone checks pass: tests 17.829s/0.786s, race 174.270s/1.773s,
vet/modules, CGO-disabled focused tests/host CLI execution, Linux amd64/arm64
cross-builds, short document fuzz 2,854 executions and diff check. All eight pinned
reader/save scripts, Pillow and edited/saved Go re-import pass. See verification.json
for exact scope; Linux cross-builds are not execution evidence.

The preceding facade correction remains: only checked nonvisible intent is stored
in a native CUSTOM range on the terminal LF because actual FDocument.save drops
unregistered manydoc resources. Registered drawing mirrors must match persisted
sources. SDK permission resources never introduce document admission gates.


Private image workers now map embedded PNG/JPEG bytes, alternative text, drawing
atoms/sources/order and preferred boxes through SF in main bodies, nested cells,
list continuations and notes. Drawing IDs are globally distinct across stories,
matching the native flat registry while retaining note-local source ownership.
Checked image geometry hints preserve omitted dimensions and exact fractional
point intent without duplicating bytes/text; native asset/resize edits win.

Pinned app/drawing lookup/skeleton box/save/Go re-import checks and independent
Pillow pixel/alpha checks pass, including the unchanged original MQTT JPEG hash.
Mixed inline image/text becomes ordered SF blocks with diagnostics. Malformed or
contradictory sources, orphans, remote/hosted sources and unmapped hidden/transformed
appearance reject instead of silently changing/dropping content. No public native
API/CLI exists. Browser bitmap painting/editor resource persistence, contextual
total story fitting and remaining profile mappings are open. Individual frames
now have contextual bounds; running stories
now have the section-composition qualification above.
The full adapter and overall goal remain active/incomplete.


Private list workers now map native bullets/list sources and checked item ownership
through SF, including nested lists, continuation paragraphs/tables, separate adjacent
lists, restarts, compound/legal/independent numbering and decimal zero starts.
Lists also work inside note bodies. Native paragraph IDs bind item/container bounds;
no item text is cached. Actual bullet/source edits remain authoritative. Unbound or
stale ownership falls back to disclosed native-bullet inference without dropping
unmarked text/tables. Native counter starts are zero-based, unlike SF starts.

Pinned app validator/renderer marker glyphs/TextX save/Go re-import qualify AA/BB,
compound Roman markers, independent8.1, restarts, bullets and decimal0 in main and
note stories. Counter-cache history alone was insufficient; actual LIST glyphs
retain earlier nested markers. Full browser/resource persistence and visual layout
remain unqualified. Literal percent templates and nondecimal zero policies differ
between SF and this native renderer: private workers reject these known mismatches
pending proper representation. Undefined deeper placeholders project safely with
checked intent. The full adapter and overall goal remain active/incomplete.


Private note workers now map real native foot/endnote definitions, styled UTF-16
reference atoms, repeated citations, custom shared marks, markerless body marks
and editable rich bodies including merged/nested tables. A sparse checked resource
preserves kind-scoped SF IDs and definition order only; native visible text/styles/
marks remain authoritative. Edited native identity/kind invalidates its old binding.
Actual pinned app validation/reader/TextX note edit/save/Go re-import checks pass.
No generated-note placement/browser/visual fidelity is claimed.

Basic note graph→SF→native records round-trip exactly, including colliding foot/
endnote IDs. A matching explicit native body-marker atom at offset zero maps to a
body label style, not a recursive citation. Contradictory/dangling graph records
reject. Independent rich citation/body-label variants, provenance/marker defaults,
notes with lists/assets, note numbering/placement settings, and native generated
note layout remain unfinished. No public native format/API/CLI is exposed.
Next complete remaining rich native mappings and actual native rendering evidence.


Private rich-body workers now map actual native table stream/source ownership
through SF, including combined row/column merges, nested tables, fixed widths,
equal full-width default columns and repeated headers. Vertical continuations use
empty rowSpan0 records; covered SF cells remain omitted. Contradictory source/
stream counts, overlap, missing/orphan sources and visible continuation content
reject before a partial document is returned. Mixed text/table paragraphs retain
ordered text/table blocks with an explicit normalization diagnostic.

Pinned SDK parser/view/skeleton geometry and TextX edit/save checks qualify these
records: the cell following a two-column vertical merge stays at x240px, nested
table geometry remains present, default-width tables are visible and Go re-imports
a native cell edit. This is structural/geometry evidence without canvas font
metrics or browser/visual layout qualification. No public .vdocs API/CLI exists.
Lists, assets/resources, notes, section/story settings, provenance/intent and
strict envelope/JSON decoding still require implementation. Next continue these
rich mappings, not a public wrapper around the current subset.

Private text workers now map paragraphs/headings (including level6), direction,
page-break-before, point-size typography, language, links and soft line breaks
through the actual SF. Overlapping/unsorted native text styles select the last
whole record; named/default/direct style layers resolve with cycle checks. Native
visible edits remain authoritative. Optional pinned SDK QA reads generated records,
uses TextX to replace astral heading text, saves, then Go re-imports the edit.
This is text-level interoperability, not rich adapter or layout completion.

Unknown presentation emits diagnostics; hidden text and other unmapped containers reject
rather than returning a partial document. Remaining rich containers, section/story settings,
provenance/nonvisual annotations, strict envelope decoding and full native edit/
layout qualification remain open. No public format/API/CLI was added. Next extend
these workers to lists/notes/assets and section/story ownership.

The native adapter's private first layer now validates UTF-16 scalar/range bounds,
paragraph/section/nested-table tokens and style/annotation/custom-block/soft-break
metadata positions. No public .vdocs conversion is exposed yet. Complete semantic
mapping through SF, assets/resources, lists/merges/sections/stories and native edit/
save qualification remain required; this foundation is not adapter completion.

Actual Univer 1.0.4 readers exposed stale published prose comments: table row/end
controls are0x0E/0x0F, while0x1E/0x1F belong to annotations. Cells require their own
CR/LF section terminator. Go token trees/UTF-16 extents now match the executable
parseDataStreamToTree exactly for a hand-authored rich native reference. Actual
DocumentDataModel/DocumentViewModel checks styled astral text, inclusive link range,
note anchor/body, nested table sources and unchanged stream text; the pinned
unmodified Colleaf validator accepts its envelope. Geometry is pixels; fs is pt.
See COLLEAF_FORMAT.md for exact source/hash/reader evidence and reproduction.

The codec rejects split surrogate boundaries, invalid UTF-8, malformed token
nesting, contradictory metadata positions/ranges and unbound table/custom-block
controls. Context cancellation and existing nesting bounds apply; app quotas and
permission gates are absent. All Go production dependencies and public API/CLI
remain unchanged. Runtime is pure Go; Node/Univer are ignored QA installations.
Full/focused/race/vet/modules/CGO-disabled/fuzz evidence is in verification.json.

Previous fd2fdd7 qualifies independently cited nested PDF notes, original parents/
order and wrapper Ref provenance;8061131 qualifies original Office sources and
positional-tab separation. The full goal remains active/incomplete. Next is both
rich native mappings, not another declaration-only or plain-text milestone.

## Existing verified evidence to preserve

- Original Word MQTT: 59 tables/756 rows/3,009 origins, all grid widths/spans/cell
  hashes, 291 headings and 4,998 nonempty body paragraphs; 3,106 bookmarks/379
  visible internal IDs and independently audited 435 Word URI/text target groups.
- Original 137-page PDF: all 90 page-local tables, 3,009 origins plus eleven merge
  continuation identities; all 42 omitted-span grids recovered from actual paint.
  All 325 anonymous views/1,245 annotation targets and transformed regions match
  independent original PDF objects. The InDesign publication's one named view/19
  annotations and native example's physical views also match. See PDF_NAVIGATION.md.
- Native full Word-source render: 189 pages, original JPEG/metadata/required Unicode,
  qualified supplied/built-in fonts and subsetting; all 291 heading labels/titles
  audited, with one original title hyphen spelling difference recorded explicitly.
  Named-navigation changes preserve all page commands and annotation rectangles.
- Plain compact SF and explicit gzip retain every original JSON value. Original
  477,613,781-byte SF persists as 8,880,248-byte .sf.gz; normalized hash matches all
  provenance/assets/relationships. Actual ProseMirror validates all 205,918 nodes
  and marks/attributes. Old/read/optimized/gzip text hashes match; full tree memory
  and editor cost remain material. See SF_CODEC.md for exact measurements/limits.
- Fixed original corpus and rich synthetic cases qualify supported SF semantics,
  fonts/shaping/bidi, tables/pagination, images, stories/notes and losses through
  independent reader/geometry/asset/pixel/visual checks. These do not substitute
  for the remaining representative original DOCX/semantic/native adapter work.

Last fully committed codec checks: tests 15.129s/0.371s, race 175.287s/2.119s,
vet/modules, host CGO-disabled execution, Linux amd64/arm64 cross-builds and diff
checks pass; gzip/plain SF fuzz 128,050/260 executions pass. Current audit-fix
checks pass: tests 14.736s/0.686s, race 175.468s/2.199s, vet/modules, host CGO-
disabled build/focused execution, Linux amd64/arm64 cross-builds and diff check.
Verification.json records the durable evidence. Linux builds are not
Linux execution evidence. Optional Node/Python/LibreOffice remain QA only.

## Next concrete implementation order

1. Implement/qualify the actual Colleaf native snapshot adapter through the shared
   SF. Include UTF-16 boundaries, rich source maps, notes/stories and resources;
   do not stop at plain-text wrapping. Leave applications/backends untouched.
2. Qualify remaining broader untagged note recovery and original source consistency,
   using PDF_NOTES.md and preserving source facts/content. Newly qualified nested
   definition/wrapper cases must remain regression evidence, not repeated blockers.
3. Preserve the now broadened original DOCX corpus and independent expectations;
   carry its measured source/producer/review/field limits into the final audit.
   Fix further real failures without changing original oracles.
4. Audit remaining source-structure consistency/resource and public API evidence,
   repeat the full acceptance matrix and final checks, and complete the goal only
   after every original criterion is proven. There is no external blocker now.

```sh
# Next: implement actual native integration from the pinned schema.
cat docs/COLLEAF_FORMAT.md
rg -n 'IDocumentData|dataStream|customRanges|tableSource' output/codec
GOWORK=off go test -race -run 'TestPDFNested|TestPDFRedundantNote|TestPDFNoteWrapper' -count=1 .
# Full nested source/generation/reader commands: PDF_NOTES.md
output/qa-venv/bin/python scripts/generate_pdf_semantic_audit.py
bin/manydoc extract -format sf -o output/audit/explicit-note.sf output/audit/explicit-note.pdf
bin/manydoc extract -format sf -o output/audit/pagination-artifact.sf output/audit/pagination-artifact.pdf
output/qa-venv/bin/python scripts/verify_pdf_notes.py
# Full probe/generation/reader commands: docs/PDF_NOTES.md
```

## Code map and recovery commands

| Area | Main authority |
| --- | --- |
| API/model/dispatch | document.go, source.go, destinations.go, notes.go, sections.go |
| SF/codec/schema | structured.go, structured_sections.go, structured_encode.go, schema/prosemirror-v1.json |
| DOCX | docx_read/write, docx_lists, docx_table/widths, docx_notes, docx_sections/running, docx_destinations |
| PDF recovery | pdf_read/geometry/images/marked/structure/links/navigation_read/infer/rulings/text_style |
| Native layout | pdf_write/text/shaping/bidi/glyph/font/notes/lists/cell_layout/table_continue |
| Source/QA | testdata/real and corpus manifests/oracles, scripts, examples, verification.json |
| Current contracts | ACCEPTANCE.md, COMPATIBILITY.md, STRUCTURED_FORMAT.md, COLLEAF_FORMAT.md, feature evidence docs |

```sh
GOWORK=off go test ./... -count=1
GOWORK=off go test -race ./... -count=1
GOWORK=off go vet ./...
GOWORK=off go mod verify
GOWORK=off CGO_ENABLED=0 go build -o bin/manydoc ./cmd/manydoc
GOWORK=off GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o bin/manydoc-linux-amd64 ./cmd/manydoc
git diff --check
```

Use extract -format sf or sf.gz explicitly; extract defaults to text irrespective
of output filename. Keep pdfcpu configuration explicit rather than using a global
constructor that creates user files/exits. Per-renderer font buffers must remain
owned because subsetting mutates them. Source font names are page/object scoped.
Do not discard visible content or revise a fixture oracle to obtain a green test.
