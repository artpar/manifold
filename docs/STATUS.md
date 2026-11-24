# Current status and recovery

Updated 2026-11-25. Read this first, then AGENTS.md, ARCHITECTURE.md and DECISIONS.md.
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
into that native format. A checked standalone adapter through SF is now public; broader native rendering
and browser qualification remain open.
Read COLLEAF_FORMAT.md for exact primary sources, comparison and mapping queue.
No Colleaf or Daptin code was modified; app persistence/authorization is not part
of the adapter. Native snapshot budgets do not become library quota gates.

## Latest progress: untagged reciprocal note inference

PDF_UNTAGGED_NOTES.md records geometric note graphs from matching raised labels
and reciprocal physical GoTo views, with explicit relationship/kind/body-extent
uncertainty. Shared bounded inference preserves original glyph geometry and
navigation, repeated citations reuse one note, and ambiguous sources stay literal.
Actual POI Word/LibreOffice original PDFs plus separately identified untagged
catalog derivatives qualify source-owned note bodies, exact physical inventory,
DOCX semantics, independent PDF readers and visual output. This is a verified
single-paragraph geometric case. A trailing note-only source page now moves
without leaving an extra blank reflow page; physical Pages remain recorded. This
is not completion of arbitrary rich note recovery.
Normal tests85.087s, final focused race5.375s, vet/module verification and CGO-
disabled host/Linux builds pass. The default full race command timed out at601.321s
in an existing custom-font seed; identical full-suite -timeout20m requalification
is running and is not yet counted passed. Exact scopes are in verification.json.
The goal remains active/incomplete; broader body ownership and final audits remain.

## Prior progress: direct encryption dictionaries now decrypt

The four preserved MuPDF RC4/AES failures now pass the unchanged full-SF oracle
through a pure-Go in-memory trailer adaptation. Original ciphertext/object/file-ID
identities remain intact; passwords still authenticate through the existing Go
handler. Rich deterministic R2 and actual MuPDF sources qualify metadata, styles,
URI, raster bytes, geometry and both reader/owner passwords with denied copy bits.
Wrong/empty passwords reject atomically. Full tests45.412s, final-production
race458.796s, focused current race1.936s, vet/module verification and CGO-disabled
host/Linux builds pass (cross-builds are not execution). Exact scopes are in the
ledger. PDF_DIRECT_ENCRYPTION.md records default
Go tests, independent source/asset/visual evidence and retained repair-shaped reader
discrepancies. Original publication/MQTT SF remains unchanged. The full goal stays
active/incomplete; this closes the direct Encrypt defect, not all parser allocation
or rich-document acceptance work. Exact checks/scopes are in verification.json.

## Prior progress: parser-stream integrity and member ownership

Retained xref/object streams now validate original decoded source before page
access, sharing the existing stage/work bounds. Corrupt checksums and inconsistent
N/header identities/xref indexes reject without partial extraction or CLI output.
Ten positive original-syntax sources retain formatting, geometry and complete SF;
nine negative sources previously accepted by73fb39e now reject atomically. Original
publication/MQTT SF remains identical. PDF_PARSER_STREAMS.md records source oracles,
independent readers, checks and the precise remaining pre-expansion boundary.
Current tests28.161s, final-production full race344.060s, focused current race1.550s,
vet/module verification and CGO-disabled host/Linux cross-builds pass. The first
timed header-fuzz run reported a deadline failure; the unchanged fixed-count
100,000-execution campaign passes. Both outcomes remain in the ledger.

That audit also reproduced a separate real compatibility gap: MuPDF encrypted
exports with direct Encrypt dictionaries fail before this validator because the
initial parser recognizes only indirect Encrypt. Independent readers decrypt all
four supplied-password variants. The original failure is now fixed and its strict full-SF success oracle passes;
see PDF_DIRECT_ENCRYPTION.md. It was not waived as unsupported.
The goal remains active/incomplete, with no overall external blocker.

## Prior progress: bounded image decoding and exact sample precision

PDF image/stencil/mask byte filters and Indexed lookups now expand within explicit
limits before raster rendering. JPEG headers are checked before pixel decoding;
CCITT uses bounded declared dimensions. Native packed/16-bit RGB/gray samples and
Decode arrays retain their components;16-bit soft alpha no longer truncates colors.
See PDF_IMAGE_DECODING.md for stage/work bounds, exact pixel/SF/placement tests,
original-corpus hashes and independent reader/visual evidence. Original publication
and MQTT complete SF remain unchanged. CMYK management and JPEG chroma interpolation
have retained strict independent discrepancies, not revised source oracles.
The full goal remains active/incomplete; initial xref/object-stream parsing and the
remaining acceptance audit are still open. Required checks pass: current full
tests28.648s, final-production full race346.391s,
focused current race8.543s, vet/module verification, CGO-disabled tests and host/
Linux cross-builds. The public200MB stage probe and independent checks pass.
Exact scopes, retained strict failures and timings are in verification.json.

## Prior progress: bounded syntax-stream decoding and strict integrity

Page contents, Forms/annotation appearances and ToUnicode now use a shared native
Go decoder. Raw/Flate/ASCIIHex/ASCII85/RunLength/LZW(EarlyChange0/1), pipelines and
packed TIFF/PNG predictors remain supported. Each decoded stage/joined page is
bounded64MiB; document decoded work and actual interpreted syntax have separate
256MiB bounds. Cached indirect streams decode once while each actual placement
retains its transform/source. Cancellation and integrity failures reject without
partial Document or committed CLI output. No app quota/permission gate is added.

An unchanged bad-checksum source previously succeeded through the exact pre-change
Git-archive CLI; it now rejects. Independent limited zlib confirms oversized direct
and shrinking-pipeline intermediate data exceed the actual public cap. Eight negative
sources reject atomically. Twelve valid sources retain exact complete SF/source
identity; MuPDF verifies every decoded byte and all glyph origins. The pinned pypdf
ignores EarlyChange and fails on long0 dictionary transitions; its strict oracle
remains executable and FAILING, while MuPDF and Go agree exactly. See
PDF_SYNTAX_STREAMS.md for source, scope and reproduction.

The original InDesign source/structure/content/DOCX/nine-page reflow oracle passes.
Full publication SF remains byte-identical to exact pre-change7727679. The already-
pinned BSD hhrutter/lzw module is now direct, with its notice/THIRD_PARTY aligned;
versions unchanged. Tidy adds existing upstream testdata checksums, not runtime code.

Final tests pass (library28.772s/CLIcached;earlier fresh28.893s/CLI0.829s), full race
passes347.433s/3.203s on final production code before later test-only additions;
focused current race8.633s and CGO-disabled focus0.782s cover them. Vet/modules,
host/Linux amd64/arm64 CGO-disabled builds pass. Short fuzz16179executions/11.464s/
two workers passes;not exhaustive safety. Images, initial xref/object-stream parsing
and broader original acceptance audits remain unfinished; the full goal stays active.

## Earlier: bounded CID widths and explicit geometry ambiguity

CID font widths no longer replay every overlapping range. The reader parses source
assignments once, caches shared indirect numeric arrays and uses reverse successor
skipping for large expansions. At most65,536 final CID values are materialized,
while existing last-assignment values remain supported. No new font quota/rejection
budget or dependency is introduced. Deterministic cancellation during expansion now
returns context.Canceled instead of succeeding; invalid overwritten entries reject.

Independent readers exposed a preexisting discrepancy for overlapping declarations:
legacy origins10,14,20,28,38pt differ from MuPDF's observed10,15,21,29,39pt (and another
pattern for heavily repeated ranges). The strict consumer-agreement oracle remains
executable and FAILS. New pdf_cid_width_overlap diagnostics and estimated_advance/
0.5 run confidence disclose that uncertainty. The non-overlapping control matches
MuPDF exactly with font_metrics precision. Complete CID inventories, actual public
text/baselines/source/SF and shared-array recovery pass targeted checks.

The original InDesign source and9-page reflow checker pass unchanged. A CLI built
from exact pre-change Git archive2b4b4d0 yields byte-identical full publication SF;
synthetic controls differ only in the intended uncertainty fields when overlapping.
One-shot benchmark samples for1000 full ranges improve506.6ms→5.93ms with modest
extra bounded allocation; no statistical/universal speedup claim. See
PDF_FONT_WIDTHS.md for commands, primary format references and exact evidence scope.

The full goal remains active: decoded-stream/other font/source resources and the
broader original acceptance audit continue. Pinned-native repeated-footnote order,
Word-PDF annotation and ambiguous CID consumer differences remain recorded. Current
Go suite passes (library28.955s/CLI0.454s), focused race1.825s, CGO-disabled focus
0.467s, vet/modules and host/Linux amd64/arm64 CGO-disabled builds pass. Final
fuzz11316executions/10.551s/two workers passes;short sample only. Full-race
passes on final production/test code (library338.766s/CLI2.781s); all evidence
is in verification.json.

## Earlier: bound expanded PDF XObject execution

A compact acyclic Form graph exposed another concrete source/resource gap:
3203bytes/seven objects expanded to299,593 actual invocations and previously
returned success below the depth guard. The reader now rejects beyond200,000
Form/image invocations across the whole document, including annotation appearances,
before decoding the next source. It returns no partial Document; CLI commits no
partial destination. This is parser work protection, not Daptin/app quotas.

Independent pypdf confirms299,593 visits in one graph and224,694 across six pages
or six appearances. Small valid shared Forms retain eight actual transformed
placements, original stream/page identities, precise baselines/advances and SF.
The original InDesign publication checker still passes10lists/57items/exact K
membership/16URIs/20,531 normalized characters and nine reflowed PDF pages. A
pre-change Git-archive CLI also produces byte-identical full source SF.
See PDF_RESOURCES.md for reproduction and resource-scope limits.

The unchanged optional native note-order checker still FAILS: note2,note3 expected,
actual reversed; no-repeat control passes, range reordering breaks after TextX edit.
Published1.0.4 code overwrites placement references keyed by shared noteId. No safe
stable schema repair was established, and no duplicated definitions/static cache or
vendor/application patch was adopted. COLLEAF_BROWSER.md records exact evidence.
Other source/resource and original completion audits continue; no overall blocker
or goal completion is established. Final normal suite passes (library29.270s;
CLI cached, earlier fresh0.819s), focused race20.936s and CGO-disabled focus2.308s
pass. Vet/modules/host and Linux amd64/arm64 CGO-disabled builds pass. Short fuzz
run passes588 executions/10.719s/two workers; not exhaustive safety evidence.
Full race passes (library339.329s/CLI3.184s) on final production code, before the
optional test-only fixture-export hook; final focused race covers that hook. All
evidence is in verification.json.

## Earlier: public native rich labels and measured counters

VDOCS literal-marker export is now enabled through the checked native label/body
graph. The temporary writer guard is removed. Public API/CLI tests recover exact
label/source/formatting/relationship graphs; the authored CLI example's recovered
SF equals the canonicalized SF source byte-for-byte and validates23 ProseMirror
nodes/marks/attrs. Raw authored JSON omits empty layout/metadata and uses whitespace;
the writer normalizes these defaults, so raw input bytes are not equal.

Actual native insertion exposed the nested counter's missing indentation. Ordinary
native list levels now use21px indent steps with zero hanging so the engine retains
its measured counter advance. A fixed21px frame initially overlapped long counters;
the original numbering example exposed it and now visibly shows full Appendix AA.,
compound, Roman and cell labels followed by separate body text. Empty right-aligned
automatic-label cells keep measured layout without those properties. Custom foreign
indents still diagnose; source/body content never becomes an SF cache.

The scoped browser queue now passes: nested1. INNER_LIST; full Item7)/Item9) counters;
unchanged red source/pixels (nested441/footnote420) plus400 dimmed header pixels;
96 unique label/body tokens across six native pages. All six pages were visually
inspected, along with nested/mixed/original numbering and mobile nested views.
Actual desktop1000×1400 and mobile375×812 touch presets produce the same graphs;
six actual app saves and pinned SDK facade saves reimport exactly. No runtime errors.
Read NATIVE_LITERAL_MARKERS.md for reproducible probes and precise evidence scope.
This is targeted synthetic qualification, not universal native/Word fidelity.

Final Go suite passes (library26.966s/CLI0.880s), CGO-disabled focus0.425s,
vet/modules and host/Linux amd64/arm64 CGO-disabled builds pass. Full race passes on final production/test code (library319.534s/CLI3.154s), and
focused actual-save race passes (1.783s). Owned browser/server were closed after QA.
Superseded full-race snapshots were deliberately stopped after production changes
(the label-only exception, then measured hanging fix); they are not passing claims.

## Earlier: native drawing registration fixes missing image paint

Committed prototype b390dbf preserved graph/save evidence and the original failing
browser oracle. Actual native image insertion then exposed the missing built-in
DOC_DRAWING_PLUGIN registration resource. Whole-document encoding now emits its
root source/order mirror; notes remain local. Existing source/mirror contradiction
checks remain enforced. A valid editor-resize test now updates both descriptors;
its original actual-frame-wins expectation is unchanged.

The original strict red-source/red-pixel oracle now passes: standalone and nested
cases each441 red pixels, footnote420 at1000×1400×1. The header's visibly dimmed
pink image is separately checked (exact native inactive color255,128,128,255).
Full-page story and nested screenshots were inspected with no runtime errors.
Native SDK glyphs/edits/facade saves and exact Go recovery still pass. This is
scoped synthetic evidence. Nested ordinary list counters still appear clipped;
long-label pagination/mobile qualification and public marker enablement remain open.
See NATIVE_LITERAL_MARKERS.md for reproduction. Production dependencies unchanged.

Final whole-suite tests pass (library26.878s/CLI0.837s), vet passes, CGO-disabled
focus passes (0.390s); host/Linux amd64/arm64 CGO-disabled builds pass. Full race
passes (library321.182s/CLI3.152s) on the final production code, before the new
public-registry test. Final focused race (1.642s) includes that test. Modules verify
and diff checks pass. Owned browser/server were closed after QA.

## Earlier: native literal-label graph prototype, initial paint failure

An internal native mapping now writes real label/body tables with checked sparse
ownership/counter intent. It restores complete SF graphs, including source owners,
formatting, links, notes, images, nested structures and RTL, through actual native
TextX edits and facade saves. Marker trees now participate in native source/identity/
image-fitting walks. Mixed automatic label paragraphs align right to prevent the
browser clipping their prefixes; the generated fix was visually observed.

At that prototype checkpoint, the pinned browser exposed a gap: the source red
image decoded correctly and had a drawing record, but produced zero red canvas
pixels, including a standalone image control. No runtime error was reported. This
was subsequently resolved by registration as recorded above. The strict pixel
probe failed at that checkpoint; generic scene-transform input probes did not
fix it. The oracle remains in use and now passes. Public VDOCS
marker export stays guarded; this internal milestone is not a completed native
conversion capability or a completion exemption. Read NATIVE_LITERAL_MARKERS.md
for exact schema, evidence, source/pixel oracles and reproduction commands.

Full Go tests pass (library26.577s/CLI0.794s), full race passes
(library319.540s/CLI3.213s); final focused race passes (3.002s), including added
malformed/stale cases and standalone control. Vet/modules, CGO-disabled focus
(0.381s), host and Linux amd64/arm64 builds pass. Full race preceded test-only
malformed/control additions; no production changes followed its snapshot. SDK
and facade-save checks pass; browser image painting does not. QA server/tab were
closed after observation. The full goal remains active, with native drawing
registration/insertion comparison as the next safe implementation step.

## Earlier progress: rich literal labels persist and render in DOCX

DOCX now exports literal marker content as actual editable OOXML blocks with
checked list/item ownership. Body/running items use borderless parallel label/body
cells; foot/endnotes use checked sequential label ranges to retain rich tables
that LibreOffice omitted when nested in an extra layout cell. Conditional/explicit
item page starts move before layout cells because the reader ignored cell-local
pageBreakBefore. Mixed lists resume automatic counters using real standard start
overrides, validated against item positions and the shared counter definition.
No body/label SF cache is embedded. Actual XML edits remain authoritative.

Exact normalized SF round trips cover formatting, URI/internal links, note IDs,
images, tables/nested lists, RTL, empty labels, counters, page starts and stories.
Ten targeted cases retain structure/assets through independent OOXML/Word reader
checks; 17 rendered Word pages were inspected after Poppler rendering. Actual
ProseMirror validates 324 recovered nodes. This is scoped synthetic evidence.

Full tests pass (library26.621s/CLI0.794s); final full race passes
(library318.480s/CLI3.177s), including duplicate-container/namespace hardening.
Focused race also passes (1.819s), including the additional spoofed-namespace
regression. Vet/modules, final CGO-disabled focus (0.396s), host CLI and
Linux amd64/arm64 builds pass. Linux compilation is not execution. CLI SF extraction
matches API output byte-for-byte; DOCX→SF→native PDF keeps actual marker URI/internal
annotations. No new fuzz campaign is claimed; full suite includes existing seeds
and new malformed ownership/page-control/numbering/visible-edit tests.

The actual Word ODT save retains both exact authored link targets, but its PDF
export omits their two annotations. The additional --require-pdf-links checker
remains FAILING and recorded separately; the main independent checker requires the
actual ODT link graph and reports the discrepancy. An isolated w:history probe did
not fix it. This does not qualify Word-PDF hyperlink export or general Word fidelity;
read LITERAL_LIST_MARKERS.md for the evidence and reproducible commands.

At this earlier checkpoint VDOCS literal-marker mapping still rejected
before output. Native repeated-footnote ordering and broader resource/source/API/
visual/original acceptance audits remain open. The complete goal stays active.

## Earlier progress: rich literal labels render through PDF parallel flow

The completion audit found a remaining core structural gap: irregular, annotated
or pictorial labels could flatten an otherwise explicit PDF L/LI tree. List items
now have a separate optional ListMarker owner for literal blocks and source facts.
SF adds list_marker before the item body through the existing ProseMirror profile.
JSON/SF/gzip preserve the complete label/body graph, including styles, note and
anchor identities, URI relationships, original source bounds and raster assets.
Automatic/constant counter recovery still takes precedence; literal recovery
never claims an unobserved counter rule. Unknown list kind is separately diagnosed.

Existing source bytes, literal text/body paragraph boundaries, URI and original
red image pixel are unchanged. Logical assertions now additionally require actual
list/item/marker ownership. Independent pypdf checks four source fixtures/six items;
actual ProseMirror validates 68 nodes/marks/attributes across five SFs.

PDF generation now uses the existing measured cell/table continuation planner for
parallel label/body flows. Layout columns have no borders or logical Table node;
actual marker/body content owns Lbl/LBody paint. Rich nested tables/images, citation
notes, URI/internal destinations, continued lists and repeated running artifacts
retain their ordinary handling. Explicit block-bearing labels stay literal on
recovery even for numeric/bullet text. Numbering resolution and minimum-body/image
fitting now include marker trees without mutating caller content.

Five output cases/nine pages pass independent pypdf ownership/Ref/asset/destination/
operator checks; actual ProseMirror validates 225 recovered nodes. Every case page
and the authored CLI example were visually inspected after Poppler rendering.
No layout grid, clipping or omitted label figures was observed. This is scoped
synthetic evidence, not Word-equivalent fidelity. The independent ParentTree audit
also passes; a contradictory-owner mutation rejects with unchanged paint streams.
A narrow RTL nested marker also exposed mid-word wrapping recovery (CHIL D for
CHILD). Standard inline ActualText groups now retain the original text, while
strict matching preserves painted styles/relationships and restores only missing
whitespace with explicit adjacent-style uncertainty. A per-run ownership trial
failed the unchanged paint-context seed by duplicating a shaped cluster; contiguous
ownership fixes that failure without changing its oracle.
Read LITERAL_LIST_MARKERS.md and examples/literal-markers/README.md for policy,
implementation and reproducible commands. Original import fixture oracles remain.

Current full tests pass (library26.734s/CLI0.809s); full race passes
(library320.702s/CLI3.176s). Final focused race (2.719s) includes the last whitespace
uncertainty diagnostic, added after the full-race snapshot. Final vet/modules,
CGO-disabled focused tests (0.420s), host CLI and Linux amd64/arm64 builds pass;
cross-builds establish compilation, not Linux execution. The short paint-context
fuzz sample passes (eight executions/12.335s); it is not broad fuzz evidence.
Commits 5b806cc and the following evidence update record this PDF milestone.

At this earlier checkpoint VDOCS literal-marker generation still rejected
before output. The native repeated-footnote ordering failure and wider source/
resource/reflow/API/visual acceptance audits remain open; the goal is incomplete.
Final checks for this milestone are recorded in verification.json. Prior
import milestone checks and evidence remain in its historical ledger entry.

## Earlier progress: fixed ordered-list labels retain PDF structure

Tagged ordered lists whose labels are all the same now retain their explicit L/LI
hierarchy as a constant ListNumbering template when a canonical counter sequence
cannot be established. This uses the existing SF/model representation, including
empty templates; there is no new schema or public API. Explicit Decimal/Roman/Alpha
family attributes remain authoritative. None/absent/unknown/bullet families do not
acquire invented ordered semantics, and annotated/non-text markers keep their
literal-content fallback.

The unused hidden counter start/restart is not observable in PDF. Constant
recovery defaults it to one with pdf_list_constant_inferred and start_unobserved
source evidence; it never claims start_from_label. Percent placeholders that the
renderer would interpret are not mistaken for literal text. Variable custom labels
still require broader representation work; this closes the existing supported
constant-template PDF round-trip gap without a body cache or vendor patch.

Four two-item cases (section sign, words, literal percent and empty labels) retain
body bold/italic/URI, original labels and SF/DOCX numbering. Independent pypdf,
OOXML and actual Word-reader rendering check their displayed labels and structure;
empty Lbl nodes own no phantom painted MCIDs. Actual ProseMirror validates 32 nodes.
Native and Word section/empty page PNGs were inspected: labels and bodies are
legible without clipping or invented markers; indentation/fonts differ as expected
for independent reflow. This is targeted visual evidence, not Word equivalence.

```sh
MANYDOC_PDF_CONSTANT_LIST_QA=output/pdf-constant-lists GOWORK=off go test -run 'TestPDFConstant|TestGeneratedPDFConstant' -count=1 .
/Applications/LibreOffice.app/Contents/MacOS/soffice -env:UserInstallation=file:///tmp/manydoc-constant-list-qa --headless --convert-to pdf --outdir output/pdf-constant-lists/word output/pdf-constant-lists/*.docx
output/qa-venv/bin/python scripts/verify_pdf_constant_lists.py
NODE_PATH=output/sf-qa/node_modules node scripts/verify_structured.cjs output/pdf-constant-lists/*.sf
```

Full tests pass (final library 25.701s/CLI cached; preceding CLI 0.826s),
full race passes (318.104s/2.787s),
vet/modules, CGO-disabled focused tests (0.402s), host CLI build/help/extraction and
Linux amd64/arm64 cross-builds pass. Empty-template CLI SF is byte-identical to API
output. Constant-pattern fuzz passes 13,155 executions/11.457s/two workers (short
safety sample). Final focused checks also verify direct DOCX retains authored hidden
start 42; only PDF's unobservable state defaults to one. verification.json records
scope and the final focused results. Goal active/incomplete: native repeated-note
placement below remains unresolved, along with wider source/resource/untagged,
variable/rich custom labels, native settings/browser and final acceptance audits.

## Open native browser failure: repeated-note body order

The full pinned Colleaf/Univer desktop mount now builds in an isolated optional
QA harness. The unmodified actual mount/validator are hash-checked and all vendor
presets are pinned at 1.0.4. Browser readiness reaches lifecycle 3 with a canvas and
no reported errors. No application or production dependency is changed.

This is not a passing full-browser qualification: the unchanged public notes
example displays footnote 2 above footnote 1. Main indices first cite note-2 at 36,
note-3 at 127 and repeat note-2 at 147. The renderer retains the last index per
note, so its actual placement order is reversed. The browser page and an actual
headless body-order checker both reproduce it. A separate no-repeat control has
the correct order. The original example and expected first-reference order remain.

```sh
node scripts/prepare_colleaf_browser.cjs
python3 -m http.server 8768 --bind 127.0.0.1 --directory output/colleaf-browser
# Current native acceptance failure, not a passing smoke test:
node scripts/verify_colleaf_note_order.cjs
```

COLLEAF_BROWSER.md records source pins, controls and remaining work. Native browser
edit/save/reopen/mobile and wider paint/layout remain unqualified. No safe data-level
ordering fix is established yet; do not patch the applications, weaken the oracle,
or replace live citations with cached bodies. Other goal work can still progress.
The full Go checks below remain passing, but do not execute this native checker.
Goal active/incomplete; no full completion or blocked-state claim is made.

## Earlier progress: bounded PDF attribute graph traversal

A small acyclic attribute-array graph could amplify work through shared indirect
references despite the existing depth-16 guard. The new document-wide 200,000-work-
entry budget counts expanded value/class/member visits across all owners and
checks cancellation while traversing. Overflow rejects without a partial document.
Class/local override order, supported attribute semantics and revision diagnostics
are unchanged. No public API/SF schema, output layout or dependency changes.

The new regressions reproduced successful over-budget imports and ignored
cancellation before the fix. They now pass; a valid shared graph still preserves
local Roman numbering over class Decimal, its literal/source label, start/body
and SF structure. Both amplification fixtures stay below 5,000 source bytes;
fanout eight/depth six expands to 262,144 leaf visits. See PDF_RESOURCES.md.

```sh
GOWORK=off go test -run 'TestPDFAttribute|TestPDFPublished' -count=1 .
GOWORK=off go test -run '^$' -fuzz '^FuzzPDFAttributeGraph$' -fuzztime=10s -parallel=2 .
```

Evidence is recorded below and in verification.json. The goal remains active/incomplete; this closes
the concrete unbounded attribute-walk gap, not general PDF resource safety or the
remaining native/semantic/reflow/API/visual acceptance audits.

Full tests pass (library 27.044s/CLI 0.534s), full race passes (317.245s/3.178s),
vet/modules and CGO-disabled focused tests (1.969s), host CLI build/help and Linux
amd64/arm64 cross-builds pass. Bounded graph fuzz passes 4,062 executions/10.537s/
two workers (short safety sample). The original InDesign SF is byte-identical to
its preceding-milestone result; independent source/DOCX/PDF checks pass. Details
and hashes are in verification.json and PDF_RESOURCES.md. Goal active/incomplete.

## Earlier progress: reciprocal PDF ParentTree audit

PDF import now cross-checks present inverse ParentTree entries against forward
page/Form MCIDs and whole-object annotation ownership. Number-tree keys/ranges,
optional Limits, repeated/cyclic nodes, owner types, container-key uniqueness and
array/scalar entry shape are validated. Contradictory ownership rejects; missing
index/container/slot information and inverse-only entries have distinct diagnostics
while preserving forward content/order. The index never invents additional content.
Absent indexes do not prevent forward recovery or certify reciprocal ownership.

Bounds are explicit: depth 64, 200,000 index nodes/key entries and 400,000 combined
index/owner slots. This is scoped defensive validation, not exhaustive PDF resource
safety or conformance certification. Reused Form/page MCID zero values remain
separate; annotation StructParent uses a scalar owner. No public SF/API shape,
production dependency or output-layout behavior changes.

Independent pypdf walks the unchanged original Copyright Office InDesign export:
413 forward elements and 550 reciprocal content owners agree. Generated notes
independently yield 40 elements and 24 reciprocal owners. Mutations substitute
another existing owner while preserving all decoded page content streams; CLI
imports must reject their contradictions. The original real-source SF validates
8,872 actual ProseMirror nodes. Existing source/oracle bytes are unchanged.

```sh
output/qa-venv/bin/python scripts/verify_pdf_parent_tree.py testdata/real/copyright-office-circular-1-2021.pdf
output/qa-venv/bin/python scripts/verify_pdf_parent_tree.py output/colleaf-tagged/document.pdf --out output/pdf-parent-tree/generated-mutated.pdf
GOWORK=off go test -run 'TestPDFParentTree|TestGeneratedPDF|TestPDFTagged' -count=1 .
```

Full tests pass (library 26.800s/CLI cached; preceding CLI 0.863s), full race
passes (317.351s/3.184s), vet/modules and CGO-disabled focused tests (0.656s),
host CLI execution and Linux amd64/arm64 cross-builds pass. Independent combined
tagged-structure and original-publication conversion verifiers pass. The latter's
original 16-URI oracle is unchanged; typed internal destinations now have a separate
source GoTo/name assertion. No output-layout change or new visual claim is made.
Final focused race passes (1.825s); bounded index fuzz passes 24,063 executions
in 11.459s/two workers (short safety sample). Object/xref injection is excluded
from this fixed-forward-fixture target. verification.json records commands/hashes.
The full goal remains
active/incomplete: broader source/resource/untagged recovery, custom/literal-list
representation, native rendering/settings/browser, note policies and final
semantic/reflow/API/visual acceptance remain open.

## Earlier progress: editable PDF list numbering

Tagged PDF Roman and Latin alphabetic lists now recover as editable ordered lists
when one canonical sequence reproduces every displayed label. Starting values,
case, repeated-letter alphabetic passes, two-digit decimal padding and consistent
literal prefixes/suffixes persist through SF and DOCX. Standard explicit format
families select parsing; start/pattern inference is separately diagnosed and
recorded in source evidence. Absent/None format attributes retain conservative
decimal/bullet inference. Observed nested prefixes remain literal; no hidden
ancestor-counter relationship is invented.

Padded decimal output now declares Decimal and legal numbering declares its actual
rendered decimal family. Markers containing links, bookmarks, notes or non-text
content stay literal paragraphs with diagnostics, preserving their content and
relationships. Marker typography may normalize on reflow and is diagnosed. Mixed
punctuation retains the existing decimal normalization policy; unsupported,
noncanonical, ambiguous and out-of-range sequences are not assigned false counters.
This is explicit partial recovery, not arbitrary label/locale/template fidelity.

Six PDF→SF→DOCX cases preserve 18 editable items, body style/URI relationships,
source labels and starts/templates/formats. Independent pypdf/SF/OOXML checks
confirm actual PDF attributes/rendered labels and emitted Word numbering. The
original iv./Body source fixture is unchanged: the former unsupported-stage test
now requires a typed Roman list, original label and inference disclosure. Existing
physical nested-marker strings are unchanged and inspected using the explicit
physical view; separate logical tests verify nested item structure/context.

```sh
MANYDOC_PDF_LIST_NUMBERING_QA=output/pdf-list-numbering GOWORK=off go test -run TestGeneratedPDFNumberedListSemantics -count=1 .
output/qa-venv/bin/python scripts/verify_pdf_list_numbering.py
NODE_PATH=output/sf-qa/node_modules node scripts/verify_structured.cjs output/pdf-list-numbering/*.sf
GOWORK=off go test -run 'TestPDFListNumbering|TestGeneratedPDFNumbered|TestPDFNestedCounter|TestPDFCounterAnnotations|TestPDFPictorialCounter' -count=1 .
```

Full tests pass (library 26.969s/CLI 0.477s), vet/modules and CGO-disabled
host/Linux amd64/arm64 builds pass. Final full race passes (318.303s/3.164s); the final guarded paths also pass
focused race (2.018s) and CGO-disabled tests (0.423s).
Canonical-pattern fuzz passes 53,792 executions/11.466s/two workers (short safety
sample). Actual ProseMirror validates 66 nodes across six SFs; diff check passes.
verification.json records commands/scope. Goal active/incomplete:
remaining literal/custom list representation, PDF inverse/source/resource and
reflow audits, native rendering/intent/browser, note policies and final acceptance
remain open. No new production dependency, backend or permission gate is added.

## Earlier progress: painted PDF structure and typed-note recovery

PDF output now emits a PDF 2.0 logical tree tied to actual page-scoped MCIDs,
MCRs, reciprocal parent-tree entries and named structure IDs. Paragraphs/headings,
list/item/label/body groups, table rows/cells/spans, image Alt and foot/endnote
roles describe current rendered content. Typed note definitions and citation Ref
edges are reciprocal. Global note IDs are retained; scoped collisions are qualified
once, with an IDTree, without growing prefixes on reflow. No SF/body/text cache is
embedded. Blank citations have empty structural owners rather than dangling paint
references. Running content and repeated headers/continued labels are artifacts.

The previously failing three-note CLI oracle is unchanged and now passes in every
standard run; its environment gate is removed. New Go tests check rich/default/
empty labels, repeated/continued/scoped citations, exact body content and ownership,
merged tables including documents without notes, list starts, formatting/URI,
image pixels/Alt, source roles/regions and caller immutability. Original physical
geometry/glyph/image/header oracles keep their expected values and explicitly use
PDFSkipStructure (plus PDFSkipInference where needed). Header-version checks now
require 2.0 for the emitted PDF 2.0 namespace; this is an intentional format upgrade.

ReadOptions.PDFSkipStructure and CLI -pdf-skip-structure expose the positioned
view without applying the logical tree. Combine with PDFSkipInference / the CLI
flag for paint-order inspection; pdf_structure_skipped discloses the choice.
Default tagged recovery remains logical; older tagged/untagged inputs still read.

Independent pypdf checks actual marked-content balance, 24 contiguous MCIDs,
forward/inverse page ownership, structure parents, IDs, three FENotes/two footnotes/
one endnote and four reciprocal citation edges. MuPDF checks both example pages
pixel-identical at 1.5x to output generated from commit 1d13d3c; Poppler renders of
both pages were visually inspected, with aligned labels, notes and footer text.
Independent recursive/repeated-header examples additionally cover 12 pages, seven
FENotes total across all three cases, and table/row/cell-span/image/marker ownership.
This is targeted structural/visual evidence, not PDF/UA/WTPDF conformance, full
Word fidelity or completion of all PDF/native/source/resource acceptance.

```sh
GOWORK=off go test -run '^TestGeneratedPDF|^FuzzGeneratedPDFNoteGraph$' -count=1 .
GOWORK=off go test -run 'TestCLIVDOCSSemantics/document.sf/pdf|TestCLIPDFPhysicalInspection' -count=1 ./cmd/manydoc
GOWORK=off go run ./examples/colleaf -out output/colleaf-tagged
# Recreate the independent pre-tagging baseline (fresh detached checkout):
git worktree add --detach output/tagged-baseline-1d13d3c 1d13d3c
MANYDOC_QA_OUT="$PWD/output/colleaf-baseline-1d13" GOWORK=off sh -c 'go -C output/tagged-baseline-1d13d3c run ./examples/colleaf -out "$MANYDOC_QA_OUT"'
git worktree remove output/tagged-baseline-1d13d3c
output/qa-venv/bin/python scripts/verify_generated_pdf_structure.py
```

Full tests pass (library 27.124s/CLI 0.838s), race passes (315.451s/3.200s),
vet/modules, CGO-disabled focused tests 0.605s, host build/help/logical and physical
CLI extraction and Linux amd64/arm64 cross-builds pass. Generated-note fuzz passes
951 executions/10.547s/two workers (short safety sample). ProseMirror checks 43
recovered nodes/marks/attributes. verification.json records exact QA and scopes.
Goal active/incomplete:
remaining native rendering/intent/browser, note policies/decorations, general
untagged recovery, broader source/resource and final acceptance audits stay open.

## Public native integration evidence to preserve

VDOCS, ReadVDOCS, WriteVDOCS, VDOCSMIME and .vdocs CLI dispatch expose the
checked offline Colleaf version-3/Univer 1.0.4 profile through SF. Public/external-
package/CLI/example tests cover rich graphs, assets, source facts, wrong envelopes,
I/O, cancellation, immutability and atomic failures. Native transport retains
source/diagnostic history without a lossy-output gate; unsupported mappings reject.

Actual pinned validator/plugins/facade save/reopen probes qualify rich containers,
notes/stories, languages, lists, links, anchors and owner-specific source metadata.
The original InDesign PDF preserves its full SF graph through two actual saves:
153 paragraph/8,651 run/67 list-item origins and 8,872 ProseMirror nodes. Checked
lossless metadata compression reduces its native envelope from 11 MiB to 3.6 MiB,
passing the unchanged app validator. Five original DOCX language graphs also
survive actual saves/edits/removal, with 516 ProseMirror nodes over 18 artifacts.
These are component/offline transport results; native browser behavior and broader
rendering/settings intent remain unqualified. See COLLEAF_FORMAT.md and the durable
verification milestones. No application code or admission/authorization was changed.

Earlier chronological progress and resolved failures remain in Git history and
verification.json, rather than contradictory current instructions here. In
particular, the old generated-PDF zero-note failure is fixed by painted structure
and enforced unconditionally; the public native adapter is already implemented.

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
  for the remaining semantic/native/source/resource and final acceptance work.

Latest scoped check results appear above and in verification.json. Linux builds
establish compilation, not Linux execution. Optional Node/Python/LibreOffice remain
QA only. Historical codec timings are retained in the verification ledger.

## Next concrete implementation order

Direct Encrypt and the reciprocal single-paragraph untagged note cases are
qualified in PDF_DIRECT_ENCRYPTION.md and PDF_UNTAGGED_NOTES.md. Broaden actual
untagged multi-paragraph/page note ownership against original DOCX graphs,
preserving physical source/asset/relationship oracles and inference uncertainty.
A precise next failure is now pinned by generate_pdf_note_continuation_probe.py:
an independently exported synthetic extension owns two footnote paragraphs, but
geometric recovery links only the first and leaves the italic continuation visible
in main flow. PDF_UNTAGGED_NOTES.md records the strict failing graph oracle and
reproduction; do not weaken it. Initial parser allocations remain separate.

```sh
cat docs/ACCEPTANCE.md docs/PDF_UNTAGGED_NOTES.md docs/REAL_DOCX_CORPUS.md
GOWORK=off go test -run TestPDFUntagged -count=1 .
python3 scripts/generate_pdf_note_continuation_probe.py
# Render/extract and run its strict --check as in PDF_UNTAGGED_NOTES.md.
rg -n 'untagged|infer|remaining' pdf_infer.go pdf_structure_notes*.go docs/PDF_NOTES.md
rg --files output | rg '(footnotes|endnotes).*\.pdf$'
GOWORK=off go test -run 'TestPDF.*Note|TestGeneratedPDF|TestCLIVDOCSSemantics' -count=1 ./...
```

1. Continue the wider source/resource/reflow/native corpus audit, including
   the remaining initial parser boundary and broader semantic recovery. The
   native repeated-footnote failure is revalidated in COLLEAF_BROWSER.md; retain
   its original oracle and seek new evidence rather than repeating the same check.
   Broaden source/structure and resource audits beyond the newly qualified
   reciprocal ParentTree checks, using malformed and independently authored sources.
   Retain current physical and explicit/inferred source oracles; never recover from an embedded SF cache.
2. Qualify remaining native rendering/intent/settings and browser/preset behavior
   against the pinned Colleaf/Univer packages. The public offline VDOCS adapter,
   rich mappings, source persistence and plugin saves already exist; do not repeat
   their implementation or modify the applications.
3. Complete the remaining note-policy, custom/literal-list representation and
   broader untagged/source/reflow audits. Record exact scope and reproducible
   failures rather than treating a current rejection as completed capability.
4. Preserve the original corpus and independent expectations, refresh the full
   acceptance matrix and visual/API/resource checks, and complete the goal only
   after every original criterion is proven. No external blocker is established.

```sh
cat docs/ACCEPTANCE.md docs/PDF_IMAGE_DECODING.md docs/COLLEAF_BROWSER.md
GOWORK=off MANYDOC_PDF_IMAGE_QA=output/pdf-image-qa go test -run 'TestPDFImage|TestPDFRaster' -count=1 .
output/qa-venv/bin/python scripts/verify_pdf_images.py output/pdf-image-qa
rg -n 'ReadContext|Decode\(|ObjectStream|XRefStream' pdf_read.go /Users/artpar/go/pkg/mod/github.com/pdfcpu/pdfcpu@v0.11.1/pkg/pdfcpu/read.go
GOWORK=off go test -run 'TestGeneratedPDF|TestPDFTagged|TestCLIVDOCSSemantics' -count=1 ./...
output/qa-venv/bin/python scripts/verify_pdf_parent_tree.py testdata/real/copyright-office-circular-1-2021.pdf
rg -n 'ErrUnsupported|requires mapping' colleaf_*.go
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
