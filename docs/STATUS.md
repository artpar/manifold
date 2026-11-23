# Current status and handoff

Updated 2026-11-23. Read this first, then the root AGENTS.md and the architecture,
decision, compatibility and testing documents in this directory.

Commit verified milestones with their tests and knowledge-transfer documents as
work progresses. Generated output and local QA environments remain ignored.

## Outcome

The active rich-document goal is **incomplete**. The initial pure-Go library and
CLI are present, and the SF foundation selects and implements a
versioned **ProseMirror JSON** profile (`manydoc-prosemirror/1`, `.sf`).
Read STRUCTURED_FORMAT.md for the decision, alternatives, schema and limitations;
ACCEPTANCE.md tracks the full objective without reducing it to this milestone.

`structured.go` adds public tree values, strict codecs and checked bridges to the
existing Document engine. CLI conversion/extraction and the report example now
support `.sf`. Legacy `.json` remains available. Current engine fields, assets,
links, table nesting, notes, running content and diagnostics survive the bridge.
No Colleaf schema was found in the searched checkout locations; `.vdocs`
compatibility has not been established. No Daptin files were changed.

## Latest milestone: default authored text uses contextual shaping

- All ordinary authored PDF text now shapes regardless of language metadata.
  Empty language selects deterministic und; language bindings affect locale rather
  than activating typography. Headers/footers, lists, cells and note bodies share
  the same measurement/final-line re-shaping path in pdf_shaping.go (renamed from
  pdf_language.go). No public API/schema/module/asset change.
- Existing explicit spacing/control policy remains. Residual marks attached to
  special spaces retain positioned glyphs; a new figure-space/mark test and native
  oracle preserve that earlier capability. Decorations join across adjacent
  same-style shaping/control/script spans; the unchanged continuity oracle caught
  fragmented strokes and production was corrected.
- Ordinary fonts now use actual descriptors/original full TTF programs and shared
  glyph resources. Authored shaped output is PDF 1.7; control-only output still
  qualifies its 1.4 ActualText path. Full-font size overhead now applies to normal
  output: the short default-font benchmark is 71,869 bytes versus the earlier
  10,896-byte primitive result. Timing is observational, not a regression guarantee.
  Internal fixed-Go note labels/captions and virtual controls still use primitives.
- Independent native HarfBuzz checks 24 untagged runs, six containing ligatures,
  across default Go, Go Mono and configured Noto Sans, plus emergency breaks and
  marked special spaces. Exact original font hashes, shared resources, glyph IDs,
  physical origins, metadata, DOCX content, URI/note links and running content pass.
  Four new native pages and two LibreOffice DOCX pages were visually inspected.
  ProseMirror validates 81 source/recovered nodes; 51 corpus files/9,031 nodes and
  28 Poppler comparisons pass. Rich-style/glyph/language/font/color/typography and
  report independent regressions pass, without changing fixture/content oracles.
- Unit provenance coverage keeps both the new descriptor and an independently
  authored primitive font with embedded-name fallback. Independent reader checks
  now verify physical original programs instead of private subset aliases; App
  Sans and default Go legitimately share the supplied Go Regular font. Spacing,
  baseline, bounds, formatting and relationship expectations remain intact.

The full goal remains **incomplete**. Bidi/joining, context across differing
styles/fonts/languages, complete script itemization, broader font admission/source
font handling, richer pages/running content, PDF structure/color and representative
real exports remain required. Resource bounds and explicit unsupported errors are
not completion evidence. Full tests/race/vet/CGO-disabled build/execution and relevant
fuzz checks are recorded in verification.json/TESTING.md. No external blocker.
Prior checkpoint: f96ebe9. Next commands: GOWORK=off go run ./examples/shaping;
output/qa-venv/bin/python scripts/verify_default_shaping.py; inspect pdf_shaping.go
and pdf_direction.go for actual paragraph bidi/context work.

## Earlier milestone: language/direction and contextual LTR lines

- Go/SF/JSON now retain paragraph flow, independent run RTL characteristics,
  general/East Asian/complex language bindings and forced complex-script selection.
  SF extends existing textStyle and paragraph/heading attrs; no second canonical
  schema. BCP47/direction validation rejects invalid values and malformed inputs.
- DOCX resolves each binding through defaults, paragraph/character inheritance and
  direct overrides, including explicit false direction and partial language-slot
  overrides. Standard bidi/rtl/lang/cs output preserves effective values. Direction,
  language and cs alone no longer produce false loss diagnostics; unused font
  choices/hints/theme bindings still diagnose. ltr is not a strong-letter override.
- Language-tagged LTR PDF shapes same-style/script segments across graphemes and
  equal formatting runs, with general/EA/forced-CS locale selection. Adjacent weak
  scripts resolve inside those segments. Actual cluster advances drive wrapping;
  emergency ligature estimates never paint sliced glyphs. Final lines shape again,
  recheck advances and back off if needed. Per-cluster ToUnicode preserves original
  logical mappings while complete-span ActualText retains original text.
- The independent C HarfBuzz oracle first found correct localized IDs but wrong
  Cyrillic spacing. The implementation was extended rather than changing that
  oracle. Native HarfBuzz 15.0.0 now matches four localized rows, five kerning/
  ligature wrapped runs and three emergency lines, including the forced-CS language
  case. Three native pages and one LibreOffice DOCX page were visually inspected.
  These are targeted synthetic checks, not broad script/Word qualification.
- Independently authored DOCX verifies four inherited/direct paragraph/run/CS
  bindings. Standard XML checks all stories; complete direction.sf → DOCX → SF
  values (including notes/links) compare identically. Actual ProseMirror validates
  40 nodes from five new SF files. The 51-file/9,031-node corpus and 28 Poppler
  comparisons pass unchanged, alongside font/color/glyph independent regressions.
- Full tests/race/vet/CGO-disabled build/execution pass. Binding fuzz 1,000,
  paragraph-line fuzz 1,000 and malformed DOCX fuzz 500 executions pass; glyph
  cluster fuzz also passed 1,000 after the cluster-map changes. verification.json
  records commands/results and scope. Runtime modules/assets remain unchanged;
  optional QA alone adds pinned uharfbuzz. No fixture oracle changed.

At that checkpoint untagged simple text still used primitives; the default-text
milestone above supplies current behavior. Still required are full itemization and context across differing styles/fonts/languages, bidi
levels/ordering and joining scripts remain required, alongside source fonts, richer
pages/running content, PDF structure/color and representative real exports. RTL
rejects atomically in all stories, including empty paragraphs and notes; rejection
is not completion evidence. No external blocker. Prior checkpoint: 76f5094.

Next commands: GOWORK=off go run ./examples/language; the independent command is
output/qa-venv/bin/python scripts/verify_language.py. Then inspect pdf_shaping.go,
pdf_text.go and existing bidi APIs to implement actual paragraph bidi/context; default shaping is now unified. See TESTING.md for exact reproducible qualification.

## Earlier milestone: positioned glyph transport and LTR graphemes

- SF → PDF now has a real glyph-ID path for supported left-to-right graphemes.
  Pinned pure-Go HarfBuzz computes glyphs/GPOS positions; Type0/CIDFontType2
  resources map document CIDs to original TrueType glyphs. ActualText and ToUnicode
  preserve original logical UTF-16, including surrogate pairs and multi-glyph
  graphemes. CID identity includes logical text, avoiding glyph/Unicode collisions.
- Remaining combining marks after NFC and supplementary outline glyphs use this
  path when the selected font contains them. Advances and ink extents participate
  in existing wrapping/line/cell/note/header layout. Missing glyphs, unsupported
  scripts/directions/joining controls and cross-style graphemes still reject
  atomically. This is not paragraph-wide joining/bidi shaping and does not complete
  the required broader Unicode capability.
- Glyph faces embed their original, unmodified TTF once and share resources across
  pages. The in-memory PDF finalization uses the existing pdfcpu object writer;
  no service/executable/CGO or new module dependency. Legacy simple text bypasses
  this finalization. Catalog lookup retains constant-time lazy registration.
- Primitive/glyph baselines share 0.01pt placement precision while relative GPOS
  offsets retain higher precision. Reader grouping no longer interprets component
  movement inside ActualText as logical breaks; noncollinear replacement baselines
  remain uncertain and keep paint order rather than inventing geometric order.
  General inferred note/paragraph semantics remain unfinished.
- examples/glyphs exercises marks, monochrome supplementary glyphs, tables, linked
  notes/URI and running content across two pages. Independent pypdf/MuPDF checks
  verify original font hashes/shared CID resources, actual supplementary GID and
  GPOS offsets against MuPDF's independent HTML/Story shaper, logical extraction
  through both ActualText and ToUnicode, relationships and DOCX package contents.
  Both native pages were visually inspected; 75 SF/recovered nodes validate in
  actual ProseMirror. This remains targeted synthetic QA, not Word equivalence.
- DejaVu Sans 2.37 is a pinned original test-only asset with its license/hash.
  HarfBuzz/Graphite/font-package notices are preserved for the newly used packages.
  Generated PDFs, probes and outputs stay ignored. Put Go probes under output/_*
  directories: plain output/*.go participates in go test ./... and can conflict.

Full tests/race/vet/CGO-disabled build/execution, glyph/font/PDF malformed fuzz,
51 accepted corpus files/9,031 nodes, 28 Poppler comparisons and independent
recovery/inference/publication/rich-style regressions passed. verification.json
records actual commands/counts and scope. The active full goal remains incomplete.
At that checkpoint paragraph shaping/bidi/language bindings remained next; the
subsequent milestone above supplies current binding and limited LTR context
evidence. Prior checkpoint: 7fc0c6a. No external blocker.

## Earlier milestone: DOCX font slots and theme languages

- DOCX keeps ASCII/high-ANSI/East-Asian/complex font slots in private import
  state until text is known, then resolves ordinary public Run/SF textStyle spans.
  Independent slot/theme pairs inherit across defaults/paragraph/character/direct
  levels; a direct family replaces an inherited theme only for its own slot.
  Equal effective families retain their original text span. No new public schema.
- The published MS-OI29500 character/hint selection table supplies a deterministic
  policy, including East Asian hints, Chinese-language/font-charset exceptions,
  Times New Roman exception and UTF-16 surrogate classification. Settings
  themeFontLang chooses supplemental theme faces by BCP47 script; empty language
  settings use latin/ea/cs defaults. Low-confidence script inference, unknown
  charsets, unresolved defaults and cached theme fallback diagnose uncertainty.
- Explicit cs/rtl forces the complex slot; bCs/iCs/szCs resolve its formatting,
  including style toggles/direct cancellation. RTL editing/layout direction is
  still unrepresented and diagnoses. Generic DOCX output writes normal/complex
  size/bold/italic twins, retaining a selected public style in both font contexts.
  Checked implicit-default metadata also validates script choices before clearing.
- Five independently authored DOCX paragraphs supply eight exact SF spans. Checks
  cover families/sizes/marks, standard DOCX slots/complex twins, native PDF per-
  character fixed-pitch/size/style evidence and LibreOffice-rendered DOCX characters.
  Both generated pages were visually inspected. Explicit output maps deliberately
  select Go/Go Mono; exact source/producer typeface equivalence is not claimed.
- Existing corpus fixtures/oracles stay unchanged. An isolated prior-commit build
  shows only docx_unicode.docx changes font spans: Chinese and supplementary text
  now retain declared ＭＳ 明朝 instead of blanket Cambria. Actual ProseMirror corpus
  nodes increase 9,027 → 9,031, across the same 51 accepted files. A new table/link
  test compares declared SF semantics because Go default spans zero and SF one are
  equivalent; its content/font/relationship expectations are unchanged.
- Full tests/race/vet/CGO-disabled build/execution, font/package malformed fuzz,
  independent theme/rich-style regressions and 28 Poppler comparisons passed;
  verification.json records exact evidence. No new runtime dependency; x/text's
  existing license covers language/script metadata. Generated output stays ignored.

The full goal remains incomplete. Modern Unicode range updates, automatic complex
selection without cs/rtl, ambiguous cs hints, producer variants and complete
language/direction binding still require qualification/expansion. Next implement
Go shaping/bidi/glyph output and source-font handling; then remaining rich pages/
running content, PDF recovery/color and representative real exports. No external
blocker exists. Prior checkpoint: 5b92a20. Do not treat atomic missing-glyph/shaping
rejection as completion of the required Unicode capability.

## Earlier milestone: calculated DOCX theme colors

- DOCX text colors now calculate themeTint/themeShade in HSL luminance rather
  than blindly retaining a cached transformed RGB. Tint takes precedence when
  both modifiers are present. Identity/extreme/gray colors, named palette aliases,
  direct/default/paragraph/character inheritance and resolved RGB round trips have
  targeted tests. Both modifier bytes and RGB/mapping scalar values validate.
- Settings clrSchemeMapping applies logical text/background/accent/hyperlink
  references to actual palette slots. Targets resolve once; valid accent swaps
  do not recurse. Direct dark/light references address their palette slots.
- Plain sRGB palettes and system lastClr resolve; system binding loss diagnoses.
  Active theme values override stale final caches with a mismatch diagnostic.
  Unsupported DrawingML palette choices/transforms or missing palettes may use a
  valid final RGB cache with a specific uncertainty warning, never double-tint it;
  missing usable cache rejects. These DrawingML/theme/script capabilities remain
  required expansion. Inactive theme modifiers diagnose their omitted binding.
- The independent python-docx/explicit-XML fixture checks eleven bindings through
  SF, DOCX XML and native PDF. Ten source LibreOffice colors agree; the deliberate
  stale-red-cache case renders red there while the current theme calculates blue.
  The source observation remains separate from the RGB oracle. LibreOffice renders
  all eleven exported RGB-only DOCX colors correctly. Source/native/exported DOCX
  pages were visually inspected. This is synthetic QA, not Word-equivalent fidelity.
- Runtime remains pure Go with no new dependency or public model/schema change.
  The resolved Color scalar persists; editable theme binding still diagnoses loss.
  Tests, race, vet, CGO-disabled build/execution, malformed/package/luminance fuzz,
  independent reader and corpus results are recorded in verification.json.

The active full goal remains incomplete. Next implement theme/script font choice,
source-font handling and Go shaping/bidi, then remaining rich page/running-content,
PDF structure/color and real-export acceptance work. No external blocker exists.
Prior checkpoint: 5788954. This checkpoint groups implementation, tests and aligned
KT docs; generated sources/renderings/binaries stay ignored.

## Earlier milestone: editable PDF text style recovery

- PDF → SF now populates editable family, physical point size, RGB and existing
  bold/italic marks from text/graphics state. SourceInfo separately records
  font-family evidence and effective device-space components. Descriptor names,
  bounded embedded TrueType names and standard core-family grouping are explicit
  evidence; arbitrary PostScript fallback identifiers diagnose uncertain grouping.
  Source font programs are never registered as output fonts or exported assets.
- Fill/stroke colors follow graphics saves, Forms and resource aliases. Gray/RGB
  quantization, approximate CMYK/default overrides, unknown color spaces,
  opacity/render modes and nonuniform transforms have explicit diagnostics.
  Unknown paint stays unset rather than being falsely recovered as black.
- Content-stream and structural ActualText retain only common editable fields;
  mixed fields clear with diagnostics, and homogeneous source paint persists.
  Synthetic word gaps retain only common styles and do not claim painted colors.
  New source fields survive strict SF/legacy JSON codecs and validation.
- A new independently authored ReportLab fixture checks ten spans on two pages
  against MuPDF: family, size, RGB, bold/italic, source baselines/pages; DOCX XML
  and mapped-font native PDF independently preserve these values. All four
  source/native output pages and both LibreOffice DOCX pages were visually inspected.
  LibreOffice preserves tokens/styles but substitutes some font faces. This is targeted synthetic QA.
- Numeric content operands now parse directly: the general object parser could
  consume the R in `0 1 RG` as an indirect reference. Regression tests cover RG,
  numeric types and malformed numbers. Embedded-name decoding is bounded to
  32MiB with cancellation, filter and expansion checks.
- Source/content/structure oracles remain fixed. Existing PDF round-trip tests
  and QA scripts now select explicit font substitutes. The publication Go test
  explicitly rounds requested output sizes to 0.01pt; its original SF is intact.
  The nine-page publication QA oracle explicitly chooses its prior default
  typography in a separate normalized SF, while checking all ten recovered
  families/fifteen sizes. Writer precision and missing-font rejection remain
  explicit. A stale recovery script assumed paint-order image indices although
  both prior HEAD and current inference produce the same geometric order: its
  unchanged pixel values now bind to original PDF object identities. Mark checks
  select source marks by type rather than position beside the new textStyle.

The full goal remains incomplete. Finish broader source/theme/script/color
handling and shaping/bidi/font qualification, then remaining page/running-content,
PDF structure and representative real-export rows in ACCEPTANCE.md. No external
blocker exists. Prior checkpoint: f8668e6. Actual checks and recovery commands are
recorded in TESTING.md and verification.json; generated QA output stays ignored.

## Earlier milestone: rich text styles and configurable PDF fonts

- Public runs now retain FontFamily, FontSize (points) and Color (#RRGGBB/auto).
  SF uses a textStyle mark with fontFamily/fontSize/color attributes, following
  the established editor pattern while declaring numeric points in this profile.
  Existing omitted attributes retain default rendering; schema version is unchanged.
- DOCX resolves document defaults, paragraph style chains, character styles and
  direct overrides for these fields. Standard rFonts/sz/color output retains
  families, exact half-point sizes and RGB/auto. Checked ignorable textImplicit
  metadata preserves unset fields for our own exported runs without hiding
  changed properties. Theme Latin names and plain theme colors resolve with
  binding-loss diagnostics; cached tinted/shaded colors retain their cache with
  a diagnostic. Full theme transform/script selection remains required work.
  Embedded DOCX font programs now diagnose their omission rather than disappearing.
- Native PDF measures selected actual faces/sizes, aligns mixed-size baselines,
  reserves actual line heights in paragraphs/lists/cells/notes/running content,
  and applies run colors to text/decorations. Small explicit sizes and trailing
  blank lines retain their metrics. Built-in Go/Go Mono have four actual styles.
  WriteOptions.PDFFonts supplies in-memory TTF faces; PDFFontMap explicitly maps
  families. No missing family/style substitutes implicitly; glyph/shaping errors
  remain atomic. CLI -pdf-font and -pdf-font-map expose the same configuration.
- Custom TTF checks cover consumed table bounds, name records, Unicode BMP cmap
  agreement and every outline before subset registration. Font flags must permit
  the embedding/subsetting mode. Registration is lazy, avoiding unused font programs/CW allocations in ordinary
  PDFs. Buffers are write-owned and validation observes
  cancellation. CFF/collections, supplementary-plane output, broader shaping/bidi
  and additional font formats remain required expansion, not completion exemptions.
  PDF sizes require 0.01pt precision; derived note marker sizes use that precision.
  DOCX sizes must be exact positive half-points. Unsupported precision rejects.
- examples/text-style supplies four native pages with mixed sizes/families/colors,
  links/decorations, a styled note/list, repeated table headers and a caller-supplied
  face. Independent pypdf/MuPDF/python-docx checks sizes/colors/baselines, fixed-pitch
  advances, embedded font programs, URI/note destinations, wrapping and DOCX XML.
  LibreOffice retains every token in two pages but substitutes installed fonts;
  exact native/producer font or pagination equivalence is not claimed.
- Existing source fixtures/content oracles remain unchanged. Three old pipeline/
  concurrency tests now explicitly map retained Calibri/Cambria requests to Go;
  previously those properties were discarded. Actual check results and
  visual/fuzz evidence are recorded in verification.json. Used-face registration
  also avoids embedding/allocating unused catalog entries; a controlled default
  case dropped from 63,031 to 10,896 PDF bytes and about 38.3MB to 7.6MB B/op.

The full goal remains incomplete. Next finish font/theme/script/recovery details
and expand shaping/bidi/configurable font qualification, then the remaining
ACCEPTANCE.md rows: rich page/running content, broader PDF recovery and a
representative real-export corpus. Prior checkpoint: b8afa7c. This milestone groups code, tests, example and KT docs in one commit; no external
blocker exists.

## Earlier milestone: repeating table headers

- Public `Row.RepeatHeader` and optional SF `table_row.attrs.repeatHeader`
  preserve a pagination property separately from semantic header-cell roles.
  True rows must form a contiguous prefix. Merges within that prefix are supported;
  a merge crossing into body rows is invalid in authored SF/Go. Existing false/
  omitted attributes retain their meaning; the profile version is unchanged.
- DOCX imports/exports standard `w:trPr/w:tblHeader`, including all standard
  on/off forms. Invalid booleans/duplicates reject. OOXML says noncontiguous
  flags have no effect: import normalizes them off with a diagnostic. A merge
  crossing the prefix boundary retains its content/span and diagnoses disabled
  repetition. No visible content is discarded.
- PDF uses an immutable measured header prefix and a single body continuation
  cursor. Header copies retain original merged geometry, rich fragments and assets;
  selecting cells row by row would incorrectly add padding inside a spanning
  header. The prefix shares admission with a body band, repeats on body/nested/
  footnote continuation pages and reserves space before fitting body figures.
  Header image budgets remeasure against actual legal body minima, including
  nested starts and extra prefix rows. Figure-bearing note headers can defer
  using the linked continuation notice; authored dimensions/pixels remain intact.
  Repeated note markers reuse their original identity/body/destination. Native
  explicit body breaks also repeat the header; Word's manual-break behavior differs.
- Repeating prefixes are indivisible in PDF. A prefix that cannot fit its container after image fitting,
  or containing page controls, rejects atomically; SF/DOCX retain those requests.
  Header-only tables render once. Other page/running-content controls and broader
  pagination qualification remain open work, not claims of professional fidelity.
- `examples/repeated-headers` has two vertically/horizontally merged header rows,
  a repeated raster, an oversized body merge, a body image and two note references.
  Independent pypdf/MuPDF/python-docx checks verify five native pages, original
  header rectangles/pixels, unique body flow, notes/URI targets and DOCX flags/
  merge identities/media. Actual ProseMirror checks all 100 nodes/attributes.
  All five native and four LibreOffice-rendered pages were visually inspected.
  LibreOffice retains all body text/images and repeats both header rows, but
  fails exact header pixel preservation (64×640 reduced to 5×50), recorded in
  ignored office-observation.json, and paginates differently.
  Native pixels and DOCX original media remain exact; no producer pixel-fidelity
  claim is made. This is targeted synthetic qualification.
- Go tests/race/vet, malformed-input checks, CGO-disabled build/execution, layout/
  DOCX fuzzing and independent regression/corpus comparisons are recorded in
  verification.json. Tests include nesting through four levels, footnotes,
  header note destinations on every table page, partial-page starts, fitted
  figures, forced body boundaries, malformed flags and crossing merges.

The active goal remains incomplete. Next extend rich text font family/size/color
semantics across SF/Go/DOCX/PDF, then broader font/shaping coverage and the other
ACCEPTANCE.md rows. Page/running-content semantics, broader PDF recovery and
representative real exports remain required. No external blocker exists.
Prior checkpoint: `ff123a6`; this milestone groups implementation, tests, example
and aligned KT docs in one commit.

## Earlier milestone: image height fitting and raster validation

- `fitImageBox` now fits the preferred image rectangle to full container width
  and height, without cropping or resampling. One missing dimension retains the
  intrinsic ratio; two explicit dimensions retain their requested ratio. No
  dimensions means 96 DPI. Current page remainder does not shrink a figure;
  ordinary flow moves the fitted figure to a fresh page when necessary.
- PDF budgets include the 8pt image gap, 12pt cell padding and nested-table gaps.
  Footnote figures fit a continuation area's separator/heading reservation. DOCX
  uses a conservative full-page budget, a 14pt allowance per enclosing cell and
  note-story reference/marker allowance; this is not a Word layout engine.
  SF retains original preferences. Format readers recover physical extents.
- When a figure-bound first footnote start cannot coexist with its reference,
  a visible linked 'Continued on a later page' notice occupies the reference
  page. The actual figure starts on a continuation page at its full fitted size.
  Both the reference and notice target that actual body. Repeated references
  reuse it; competing figure notes retain separate correct destinations. Ordinary
  text/table starts keep their established behavior.
- Output validates complete PNG/JPEG raster data, not just configuration, before
  registration/embedding. A valid-header RGBA PNG with one of 640 rows reproduced
  a PDF dependency panic and an invalid DOCX asset; both now return errors with
  zero output. A 25-million-pixel allocation bound matches PDF import. Successful
  validation is cached by digest per write, retaining config only. No global cache,
  resampled pixels, application permission gate or production dependency was added.
- Both formats reject dimensions their actual serializer would round to zero;
  tests include a half-EMU tie. PDF JPEG streams and DOCX JPEG assets remain
  byte-identical; PDF's normalized extraction also retains decoded pixels.
  Empty/tiny containers and excessive/malformed rasters fail atomically.
- `examples/image-fit` provides a self-contained 64×640 checkerboard used in body,
  nested merged cells and two figure-first notes. Independent pypdf/MuPDF/
  python-docx checks verify seven native pages, four exact pixel placements,
  nested rectangles, original SF preferences, fitted DOCX extents and all four
  actual note destinations. All seven native pages and five LibreOffice 7.6
  DOCX-rendered pages were inspected; every original token/image remains inside
  margins. Producer pagination differs. This remains targeted synthetic evidence.
- Full Go tests/race/vet/module verification, host CGO-disabled build/execution
  and diff checks pass; exact timings/fuzz counts are in verification.json.
  Fuzz varies page sizes, nesting, preferred dimensions and body/note contexts,
  checking pixels, requested aspect, bounds, no mutation and real destinations.
  Regenerated prior examples preserve their independent geometry/content oracles;
  all 28 Poppler comparisons, 51 corpus SF files/9,027 nodes and the new 36-node
  ProseMirror example pass. Valid oversized rejection tests became positive
  content/pixel checks; original source fixtures/oracles were not weakened.

The active goal remains incomplete. Next implement repeated table headers, then
other ACCEPTANCE.md rows: rich fonts/shaping, page/running-content semantics,
broader PDF recovery, note-layout qualification and representative real exports.
No external blocker exists. Prior checkpoint: `0d2bbc4`; this milestone groups
its implementation, tests, example and aligned KT docs in one commit.

## Earlier milestone: paragraph page-start properties and DOCX mitigation

- Public `Block.PageBreakBefore` and SF paragraph/heading attrs.pageBreakBefore
  now preserve a conditional page-start request separately from unconditional
  page_break controls. The optional false default leaves existing SF meanings
  unchanged. DOCX import resolves document defaults, the default paragraph style,
  basedOn inheritance and direct overrides, including explicit false. Invalid
  booleans, duplicate properties and cyclic style chains reject.
- Native PDF applies the property before body/list paragraphs and passes the
  current page-start position through nested cell/note selection. It skips a
  conditional boundary already at page start, without skipping explicit controls.
  List markers move with their first paragraph; note links target the first
  actual body page. Running-content page-start controls still reject explicitly.
- DOCX output uses standard pageBreakBefore properties plus checked ignorable
  metadata to restore exported unconditional controls. Empty anchors/control
  placeholders preserve leading/repeated/trailing events; a control before a
  list attaches to its first numbered paragraph and restores outside the list.
  This avoids a synthetic blank page when that paragraph also has the property.
  No page-break run is needed for the declared block controls.
- Full-SF comparisons found two preservation defects, now fixed: default heading
  bold styling invented an explicit mark, and required empty-cell paragraphs
  invented authored content. Checked heading mark metadata retains the original
  mark without changing the standard heading style. Checked empty-cell/table-tail
  markers distinguish structural paragraphs. Contradictory metadata cannot hide
  visible content. Fuzz regression 1968518a8e616e7b is retained unchanged.
- `examples/paragraph-breaks` passes independent pypdf/MuPDF/python-docx/XML checks
  across eight native pages. A separate python-docx source validates inherited
  properties and cancellation through DOCX -> SF -> PDF across three pages.
  All eleven native pages were visually inspected. LibreOffice 7.6 renders the
  new example in five inspected pages, retaining every token with no synthetic
  blank page; cell/note pagination still differs from native.
- The earlier mixed-break DOCX now renders in two inspected LibreOffice pages
  with all original tokens and both images, including both formerly missing
  note side texts. Native five-page geometry/pixel/ownership checks are unchanged.
  This is a verified content mitigation, not Word-equivalent layout. Broader
  Word/producer and real-export qualification remain required.
- Full tests/race/vet/module checks, host CGO-disabled build/execution and diff
  checks pass; timings are in verification.json. Whole-SF DOCX fuzz passed
  12,060 executions in 15 seconds; conditional table-start fuzz passed 1,720 in
  15 seconds. Malformed DOCX fuzz with new metadata/property seeds passed 336
  executions in 10 seconds. All used two workers; finite campaigns are not
  exhaustive safety evidence. Existing exact layout/note checks, 28 Poppler
  comparisons, 51 corpus SFs/9,027 nodes and the two new SFs/42 nodes pass.

The goal remains incomplete. Next implement oversized image handling and repeated
headers, followed by the remaining ACCEPTANCE.md rows. Fonts/shaping, richer
page/running content, broader PDF recovery and real exported documents still need
work. No external blocker exists. Prior checkpoint: `7d83b91`; commit this
milestone with its tests, examples and aligned KT docs.

## Earlier milestone: explicit cell and footnote page breaks

- `pdfFragment.Break` is a zero-height control event. Break-aware table groups
  always use the planner, including groups otherwise small enough for one page.
  A cell's first forced boundary cuts the whole table band; every sibling is
  reselected from the same immutable cursor at that cut. Nested boundaries
  propagate to parent tables. Aligned sibling breaks share a boundary; consecutive
  breaks in one flow retain separate pages. Blank cell fragments keep padding.
- `pdf_notes.go` moves forced note tails to continuation pages. Leading breaks
  can move the first actual body away from its reference; links target that body,
  or the final blank page for a note containing only breaks. Such empty notes
  reserve no separator/text height. Trailing breaks create a fresh page that
  resumed body can reuse, avoiding an extra unintended blank page. Other note
  definitions can still start on the original reference page.
- `examples/page-breaks` supplies a five-page mixed case and a three-page
  `-empty-note` case. Independent pypdf/MuPDF/python-docx/XML checks verify exact
  page ownership, nested span frames/columns, original styles/URIs, both image
  pixels/positions and actual destinations. The empty note's reference targets
  page three; both empty continuation pages remain present. All eight native
  pages were visually inspected. SF/DOCX preserve all four mixed-story breaks.
- Full Go tests, race, vet, module verification, CGO-disabled host build/execution
  and diff checks pass; timings are in verification.json. Two fifteen-second,
  two-worker campaigns passed 1,825 cell-break and 1,831 note-break executions.
  Tests include simultaneous/staggered sibling breaks, four nesting depths,
  leading/consecutive/trailing controls, empty notes on a 20pt content area,
  adjacent visible/moved notes and atomic errors. Regenerated prior layout/note
  examples, all 28 Poppler comparisons and the 51-file/9,027-node ProseMirror
  corpus pass. Both new SF examples pass at 64 combined nodes.
- LibreOffice 7.6 rendered the mixed DOCX in three inspected pages, changed the
  cell/note break placement and omitted NOTE_SIDE_A/NOTE_SIDE_B. Both texts are
  present in independently checked OOXML and the native PDF. This is a failed
  LibreOffice visual check, not accepted content loss. `--office` records the
  failure explicitly in output/page-breaks/libreoffice-observation.json.
- A reproducible QA variant replaces four standalone break runs with
  pageBreakBefore on their following paragraphs. LibreOffice renders it in two
  pages with all original tokens and two images, but still differs from native
  forced cell pagination. `scripts/author_paragraph_break_variant.py` authors
  that experiment only; production encoding/import/round-trip support has not
  changed. This is a concrete compatibility mitigation to qualify next.

The goal remains incomplete. Prioritize paragraph break-property import/export
and qualification of that DOCX mitigation, preserving leading/consecutive/trailing
break intent. Then oversized images, repeated headers and the other ACCEPTANCE.md
rows remain. No external blocker prevents continuing. The prior checkpoint is
`959fe46`; this milestone is committed with its tests, examples and KT docs.

## Earlier milestone: recursive table and footnote continuation

- `pdf_fragment_continue.go` selects measured prefixes and immutable continuation
  tails. A table fragment can supply a smaller page band to its containing cell
  or note, recursively. Body and footnote placement use the same selection path;
  no SF/public model change or production dependency was needed.
- Partial tables retain their logical row/cell cursor, remaining content, final
  spacing and original plan. Only the selected head's references enter admission.
  Note-start reservation uses the minimum legal nested band rather than demanding
  the whole merged group. A footnote can begin with an oversized table directly.
- Remaining table height is remeasured without a page limit, accounting for
  continuation padding. Parents then allocate enough space instead of drawing
  several closed pieces of one nested merged cell on the same physical page.
  Planning can combine same-row bands before drawing one enclosing frame.
- `examples/recursive-tables/document.sf` exercises three body table levels with
  combined merges, a long adjoining cell, two delayed note references and two
  images. Its late note contains an oversized merged table and another merged
  table inside that cell. Independent pypdf/MuPDF/python-docx checks verify seven
  native pages, exact column ownership, one top/bottom pair for each spanning
  frame per page, original flow order/marks/URIs, pixels/placements and actual
  reference destinations on pages one/three. All seven native pages were inspected.
- The generated DOCX retains all nested merges, media and note structures in
  independent XML checks. LibreOffice 7.6 rendered six inspected pages with all
  original tokens and two images within bounds, but flattened footnote tables.
  That observed interoperability limit is not a note-table visual fidelity pass;
  broader Word/producer qualification remains required. No fixtures were weakened.
- Full tests (3.589s library), race (50.447s library), vet, module verification,
  CGO-disabled host CLI build/execution and diff checks pass. Recursive layout
  fuzz passed 1,100 executions in 20 seconds with two workers. Regenerated body
  continuation, rich-cell/note, normal/five-page note and seven-rectangle merged
  examples pass unchanged. All 28 Poppler comparisons, the 51-file/9,027-node
  ProseMirror corpus and the new 175-node SF example pass.
- Prior verified work and aligned knowledge-transfer docs were committed as
  `a5657b8`. This recursive milestone is committed with its tests/examples/docs;
  use `git log` for the current checkpoint. Generated output remains ignored.

The goal remains incomplete. Next implement explicit cell/footnote page breaks,
then oversized image handling, repeated table headers and the remaining
ACCEPTANCE.md requirements. Rich fonts/shaping, page/running-content semantics,
broader PDF recovery and a representative real-export corpus are still required.
No external blocker prevents continuing.

## Earlier milestone: oversized body-table continuation

- `pdf_table_continue.go` plans oversized body-table groups into physical page
  bands. Each active cell owns its unconsumed flow; row/column spans, text/list
  fragments, fitted images and fitting nested row groups retain their content
  order. Logical rows extend to consume ending cells after continuation padding.
  Ordinary groups keep their previous exact measured rectangles.
- Page fragments close their outer borders. Interior logical-row boundaries draw
  only where cells end, preserving rowspan ownership rather than dividing the
  spanning column. Each continuation band uses 6pt top/bottom content padding.
  This is an independently authored reflow policy, not Word-equivalent layout.
- Admission reserves notes for references actually painted in that page fragment.
  Planning includes the first fragment of each new note and the separator both
  on the current page and on a possible fresh page. A regression covers a table
  reference after an existing body footnote. Delayed references stay with their
  own note body, instead of reserving every group note on the first page.
- `examples/table-continuation/document.sf` has a three-row/two-column spanning
  cell, twenty labelled paragraphs, a checkerboard, a URI, a long adjoining cell
  and early/late notes. Independent pypdf/MuPDF/python-docx checks verify four
  native pages, ordered/exact-once flows, true rowspan borders, image pixels and
  bounds, destinations on pages one/three and actual DOCX merged identities.
  All four native pages and all three LibreOffice 7.6 DOCX-rendered pages were
  visually inspected. All original tokens and the image occur once in both.
  This is targeted synthetic qualification, not a new published real-world case.
- Full tests (4.558s library), race (47.500s library), vet, module verification,
  CGO-disabled host build/execution and diff checks pass. Continuation fuzz passed
  1,220 executions in 15 seconds; rich-layout fuzz passed 1,037 in 15 seconds,
  each with two workers. The retained continuation input exposed an invalid
  test assumption: taller pages may correctly put both notes on one page. The
  corrected invariant checks reference/body equality and nondecreasing order;
  the fixed small-page example independently checks distinct target pages.
- All 28 Poppler comparisons and the 51-file/9,027-node ProseMirror corpus pass;
  the new example passes at 88 nodes. Regenerated rich-cell/footnote and existing
  normal/five-page note and seven-rectangle merged-table checks pass unchanged.

The goal remains incomplete. A nested connected table group is still atomic when
placed inside a cell or footnote; recursive splitting of oversized nested/note
row groups is next, together with explicit cell/footnote page-break continuation.
Oversized image atoms, repeated headers and broader continuation layout still
need work. Other ACCEPTANCE.md rows remain required. No external blocker exists.

## Earlier milestone: measured rich cells and footnotes

- `pdf_cell_layout.go` measures a block tree into fragments before drawing.
  Cells now contain PNG/JPEG images, nested tables, lists, styled text and note
  references. Shared merged-grid constraints include every fragment's height;
  nested note reservations are admitted with the containing connected row group.
- `pdf_notes.go` uses the same fragments and actual heights for text, image and
  table footnotes. Long notes continue between text lines or connected table-row
  groups on labelled continuation pages before body flow resumes. Internal links
  target the first actual note page. Existing text-only geometry is unchanged.
- `image_layout.go` shares preferred-size fitting between DOCX and PDF. A single
  dimension preserves intrinsic ratio; neither dimension means 96 DPI. Images
  fit their available container width. DOCX/PDF use their respective cell padding,
  so the example's 400×200 preference renders at 232×116 / 228×114 points.
  SF retains requested values; format readers return physical drawing sizes.
  Extreme finite preferences avoid intermediate overflow; invalid dimensions,
  oversized atomic fragments and explicit cell/footnote page breaks reject with
  no partial output. Oversized connected groups still need continuation layout.
- `examples/rich-cells` provides self-contained SF, DOCX and PDF examples. The
  normal case includes combined merges, a nested rowspan, three distinct images,
  URI and nested-cell note links. The continued case contains a table/image
  footnote whose 12 rows appear exactly once, split 3/5/4 before body resumes.
- Independent pypdf/MuPDF/python-docx checks verify six cell rectangles, exact
  image pixels/placements, DOCX relationships and merge identity, text/style/link
  ownership, note target pages and bounds. Both native normal pages and all four
  continued pages were visually inspected. LibreOffice 7.6 rendered the normal
  DOCX to one inspected page; native pagination is two pages, not a Word match.
  This targeted fixture does not expand the representative published corpus.
- Full Go tests (3.557s library), race (41.460s library), vet, module verification,
  CGO-disabled host build and diff checks pass. Rich-layout fuzz passed 1,498
  executions in 15 seconds with two workers. All 28 Poppler comparisons, the
  51-file/9,027-node ProseMirror corpus, and both new SF examples (98 nodes) pass.
  Existing normal/five-page note checks and seven-rectangle merged-table checks
  pass unchanged. No production dependency was added in this milestone.

The active goal remains incomplete. Next implement oversized connected-row/group
splitting and explicit cell/footnote page-break continuation. Fonts/shaping, rich
page/running content and the other ACCEPTANCE.md rows remain required work.
No external blocker prevents continuing.

## Earlier milestone: painted-grid tables with merges and empty cells

- `pdf_rulings.go` tracks constructed paths independently of q/Q state, records
  painted axis-aligned strokes/thin filled rectangles, and applies rectangular
  page/path/Form clipping to ruling evidence. It handles implicit rectangle
  closure, rejects degenerate filled outlines as boundaries, and resolves
  relevant opacity/neutral blend/mask state without a service or executable.
  Text/image clipping and arbitrary vector appearance remain diagnosed losses.
- `pdf_ruled_table.go` joins boundary evidence, checks enclosure and partial
  edges, and derives rectangular cell groups from missing internal dividers.
  Colspans, rowspans, empty cells and wholly empty grids now persist in SF.
  Nested grids resolve inside-out; multiline cell paragraphs, links, formatting
  and cell images retain their original runs/assets/positions. Boundary-derived
  cell coordinates stay inferred, including empty cells; no header role is invented.
- Invalid/nonrectangular/partially divided grids or content crossing a divider
  retain original content with ambiguity diagnostics. Numeric-table inference
  does not override a rejected grid. Coordinate/line/slot budgets and context
  checks bound this inference; border appearance and authorial semantics remain
  distinct from the recovered geometry.
- Targeted tests cover strokes/fills, transforms/Form BBox, rectangular clipping,
  path lifetime across q/Q, opacity/masks/reset, mixed-width stroke joins,
  degenerate fills, nested grids, empty/multiline/merged cells, links and source
  coverage. Supported text grids preserve declared semantics through
  SF -> DOCX and SF -> PDF -> SF. Image-cell import/SF/DOCX persists, while native
  image/nested-cell layout is still required and currently rejects without output.
- `verify_pdf_rulings.py` independently checks two source grids with MuPDF,
  compares all seven cell rectangles/merges/empty-cell coordinates against SF,
  then verifies DOCX merge identities and two reflowed PDF grids/content/bounds.
  Both source/native pages were visually inspected. `verify_pdf_rulings_office.py`
  additionally authors a targeted DOCX with python-docx and obtains an actual
  LibreOffice 7.6 untagged PDF export. Its seven recovered cell rectangles/spans
  match MuPDF; source, native reflow and LibreOffice DOCX rendering were inspected.
  This is real-producer qualification of synthetic content, not an expanded
  representative published-document corpus. Existing fixture bytes are unchanged.
- That Writer export exposed half-stroke end gaps/overhangs at intersections.
  Recovery now accounts for crossing stroke thickness (including mixed widths)
  and floating-point tolerances, without treating join nubs as partial dividers.
  Implicitly closed rectangle clips and zero-area retraced fills have regressions.
- Full Go tests, race (40.8 seconds), vet, module verification, CGO-disabled
  host CLI build/execution and diff checks pass. Partition/position/stroke-width
  fuzz passed 4,122 executions in 15 seconds; parser fuzz with ruled-grid seeds
  passed 67,941 executions in 10 seconds. All 28 Poppler comparisons and the
  51-file ProseMirror corpus (9,027 nodes) pass. The two new SF examples passed
  ProseMirror too (83 combined nodes). No production dependency was added.

The full goal remains incomplete. Next implement native PDF layout for image and
nested-table cells, followed by oversized-row splitting/nontext notes and richer
font/shaping/page/semantic support. More complex ruled layouts, double borders,
occlusion, rotated structure and broader real exports still require qualification.
No external blocker prevents continuing.

## Earlier milestone: untagged geometric structure

- `pdf_infer.go` now orders untagged positioned content with whitespace cuts,
  groups baselines and aligned wrapped paragraphs, ranks typographic headings,
  recovers consecutive marker lists with starts/nesting/hanging continuations,
  and builds tables from repeated aligned rows with numeric-column evidence.
  Tagged input still follows its logical tree. Source runs retain original
  formatting, relationships and geometry; recovered semantics are inferred.
- Physical pages/page breaks and raster assets persist. Missing geometry,
  nonhorizontal/bidirectional text retain paint order with a diagnostic.
  Overlapping image/content regions and subdivision limits report uncertainty.
  Structural list labels preserve original markers; normalized display and
  conservatively retained prefix-span bounds are disclosed explicitly.
- Public `ReadOptions.PDFSkipInference` and CLI `-pdf-skip-inference` retain
  untagged paint order/baseline groups for physical inspection. They do not
  disable tagged-tree interpretation. Default PDF reads recover inferred structure.
- Targeted tests check reversed/interleaved paint order, full-width titles above
  columns, exact paragraphs/cells/list hierarchy/starts, continuations, links,
  image bytes/placements, pages, ambiguous fallbacks and SF semantics. Numeric
  list values with wide marker gutters are not mistaken for table cells.
- Corpus coverage compares original literal characters, source spans/styles/links,
  assets and page geometry before/after inference for all 28 accepted PDF cases.
  Source fixtures/oracles are unchanged. All 51 SF corpus files pass actual
  ProseMirror (9,027 nodes, fewer baseline wrappers/synthetic separators).
  All 28 independent Poppler text comparisons still pass.
- `scripts/verify_pdf_inference.py` creates an independent two-page hand-placed
  source, checks recovered structure and literal coverage against that source,
  and verifies SF -> DOCX/PDF cells, starts, links, images, pages and bounds.
  Both source and native reflow pages were visually inspected. A spanning-edge
  regression also prevents title/footer bands and a large mid-body gap from
  interleaving columns. It exposed duplicate WinAnsi aliases selecting missing
  core-font metrics: canonical space/hyphen/bullet slots now supply correct
  advances, checked against AFM values and independent MuPDF string advances.
  These targeted examples qualify the implemented rules, not general untagged
  reconstruction.
- Full tests, race (41.1 seconds), vet, module verification, host CGO-disabled
  CLI build/execution and diff checks pass. Source-span coverage fuzz passed
  3,313 executions in 15 seconds; malformed-PDF fuzz with the new inference seed
  passed 29,060 executions in 10 seconds. LibreOffice 7.6 rendered the generated
  DOCX to two pages; both were content-checked and visually inspected.

At that earlier milestone, required work included ruling/merge/empty/multiline
table inference, more complex layouts and broader real exports,
plus all rich model/font/shaping/layout requirements in ACCEPTANCE.md. No
external blocker prevents continuing.

## Earlier milestone: tagged implied paragraphs and word continuity

- Tagged containers group consecutive direct text/inline children into one
  inferred paragraph. Explicit P/headings/captions, grouping containers, nested
  lists/tables and images remain boundaries, including blocks within inline
  wrappers. Table cells use the same grouping. LBody ownership stays explicit
  on list items; paragraph provenance honestly records the inferred boundary.
- Structural Span ActualText is explicit replacement text, not evidence of an
  explicit paragraph. Homogeneous marks/links/font metrics survive replacement;
  mixed replacements retain common marks and disclose mapping ambiguity.
- Discretionary hyphens and explicit zero-width boundaries do not acquire
  synthetic spaces when painted lines join. Original logical characters/runs
  survive. Ordinary geometric word gaps remain inferred separators. Multi-page
  paragraph regions are unioned separately per page, never across coordinate
  systems. Custom-label fallback prefixes only the first body paragraph.
- New targeted tests check paragraph boundaries, inline wrappers, formatting,
  links, soft hyphens, table cells, custom labels and multi-page bounds through
  SF round trips. The published-export oracle now independently compares every
  LI's block membership and all 57 LBody element identities against original /K,
  and checks joined discretionary words. Character counts alone are insufficient.
- Full Go tests, race (36.7 seconds), vet, module verification, host CGO-disabled
  CLI build/execution and diff checks pass. PDF malformed-input fuzzing passed
  37,646 executions in 10 seconds with two workers. Independent tagged-source
  and original-publication conversions pass; all 51 corpus SF files still pass
  ProseMirror (12,417 nodes). The corrected nine-page publication was reviewed
  as a contact sheet, with detailed inspection of pages 1, 2 and 6. Joined words,
  nested lists and bounded reflow are visible; original pagination, literal
  note-number styling and unclaimed-footer placement remain limitations.

## Earlier milestone: Unicode typography and wrapping across runs

- `pdf_text.go` measures typographic spaces/control characters with proportional
  font/em advances, preserves logical Unicode through standard ActualText and
  renders canonically equivalent composed glyphs for supported NFC clusters.
  The declared spacing table includes en/em, fractional, figure/punctuation,
  thin/hair/narrow, mathematical and ideographic spaces, NBSP, ZWSP and joiners.
- Pinned MIT-licensed uniseg v0.4.7 supplies Unicode 15 grapheme/line boundaries
  across the whole paragraph, so formatting/source-run splits do not split words.
  Go font metrics supply proportional widths; terminal-cell widths are not used.
  Ordinary overlong words wrap at grapheme boundaries. Glue/joiner groups that
  cannot fit reject rather than violating no-break semantics. Note markers stay
  attached to the preceding grapheme. Soft hyphens paint only at selected breaks.
- Logical spaces/controls at wrapping boundaries persist with zero advance rather
  than disappearing. Underline/strike extents use measured advances. ActualText
  retains homogeneous bold/italic/font metrics/baselines where justified; mixed
  formatting remains diagnosed. Font descriptor flags/weight/angle now inform
  style recovery, including fonts whose names do not spell out Bold/Italic.
- Extraction no longer treats the initial Td after every BT as evidence of a new
  visual line: transformed baselines determine grouping. This fixes adjacent
  styled/ActualText spans on one line. Output declares PDF 1.4 or newer for
  ActualText; the equal-length header promotion preserves all xref offsets.
- The original published InDesign import now renders to a nine-page native PDF.
  Independent readers verify metadata, all 16 URI targets, all 20,531 normalized
  nonspace characters, exact em/hair/punctuation-space counts and text bounds.
  All nine pages were visually inspected. This proves bounded reflow/content,
  not original layout or fully correct imported semantic paragraphs.
- `examples/typography` generates usable SF/DOCX/PDF examples, including 15
  measured space/control rows, NFC text, decorations, links and note spacing.
  Independent pypdf/MuPDF checks verify coordinates/advances, ActualText, glyphs,
  text bounds and decoration extents. The native PDF and a one-page LibreOffice
  7.6 DOCX rendering were visually inspected. Actual ProseMirror checked 48 nodes.
- Full tests/race/vet/module verification and host CGO-disabled CLI builds pass.
  Layout fuzzing found a consecutive-soft-hyphen overflow. The saved input is
  retained; the fix backs off all overflowing discretionary candidates. A new
  20-second/two-worker campaign passed 6,898 executions; PDF-read fuzz passed
  39,642 executions in 10 seconds. Existing report, merged
  table and normal/five-page note independent checks still pass. The existing
  51 corpus SF outputs still pass ProseMirror (12,417 nodes).

The tagged LBody fragmentation identified at that earlier milestone is now fixed.
The full goal remains incomplete: untagged structure, richer fonts/shaping,
nontext/nested table/footnote layout, page settings, internal destinations and
broader real-export qualification remain required. No external blocker prevents
continuing.

## Earlier milestone: tagged PDF semantic recovery and a published export

- `pdf_structure.go` walks the forward logical `/K` tree with page/Form MCID
  scope, MCR/OBJR references, role maps, attribute classes/local overrides and
  inherited ListNumbering. Cycles, multiple ownership, contradictory parents,
  bad page/stream references, ambiguous CMaps and invalid grids reject.
- Recovered paragraphs/headings, list boundaries/items/starts/labels, table spans,
  image Alt text, URI links and logical reading order use actual painted content.
  Cross-page paragraphs stay intact; page boundaries persist in `Pages` and
  source regions. Unclaimed content is retained after the logical tree with a
  diagnostic. ActualText replacements preserve contributing geometry.
- `SourceInfo.Stream`, `Role` and `Element` distinguish paint object, MCID owner
  stream and logical role/element. `ListItem.Source`/SF list_item source attributes
  retain labels/item provenance without overwriting paragraph provenance. MCIDs
  alone still do not imply explicit structure; checked tree associations do.
- URI annotations bind through logical Link/OBJR ownership; untagged whole-span
  associations use disclosed geometry. Partial/overlapping spans and conflicting
  logical targets diagnose uncertainty. Internal destinations remain unfinished.
- Added original, published ten-page U.S. Copyright Office Circular 1 (InDesign
  15.1/PDF Library 15.0, 2021) under testdata/real/, with byte hash, provenance,
  rights notes and independent structural oracles. It exposed mixed/padded
  simple-font ToUnicode keys and a contradictory two-byte codespace; the reader
  now normalizes unambiguous numeric one-byte mappings with diagnostics. Conflicts
  reject. PDF Info title/author/subject now read directly with PDF text encoding.
- Independent pypdf/MuPDF checks match all 20,531 normalized nonspace characters
  (including stored list labels), 10 lists, 57 items and 16 unique URI targets.
  This character multiset check alone does not establish reading order: separate
  `/K`, labels and relationship checks supply structural evidence. The publisher
  role-maps styled headings to P, so no explicit heading rank is invented.
- Published SF round trips retain the complete current model; generated DOCX
  preserves body/metadata independently. Its SF passed actual ProseMirror. Source
  pages 1/8 were visually inspected. A separate two-page targeted tagged source
  exercises headings, cross-page paragraphs, merged tables, labels, Form/image
  scope and URI ownership; its DOCX/PDF passed independent readers. Both source
  pages, its one-page native PDF and its one-page LibreOffice 7.6 DOCX rendering
  were visually inspected.
- Full tests/race/vet/module verification and host CGO-disabled CLI builds pass.
  PDF fuzz passed 20,580 executions (10 seconds/two workers). An SF campaign ended
  with a harness shutdown deadline and no saved failing input; a separate 15-second
  rerun passed 249,815 executions. See verification.json for exact evidence.
  Runtime modules are unchanged. No Daptin code or proprietary UniDoc code changed.

The full goal remains incomplete. Untagged reading order/headings/lists/tables,
internal destinations, exact partial-span link recovery, broader roles/attributes,
namespace/revision semantics and inverse ParentTree validation remain unfinished.
The forward `/K` graph is recovered; this is not a complete PDF conformance checker.
At this earlier milestone SF -> PDF failed explicitly on U+2003 EM SPACE.
The later Unicode typography milestone above fixes that failure; broader
Unicode/layout capabilities remain required. Richer table/footnote layout, font/shaping,
page settings and a broader real-export corpus remain required work as well.

## Earlier milestone: positioned PDF content and raster assets

- `Document.Pages`/SF `doc.attrs.pages` preserve MediaBox, CropBox, rotation,
  UserUnit, displayed physical dimensions and an invertible coordinate mapping
  where the page boxes are valid. `SourceInfo` on actual text/image content
  records quads, baseline/render matrices, font metrics, paint order and origin.
  Displayed coordinates use physical points with top-left origin and y down.
- Text positioning now follows CTM/Tm/line matrices, horizontal scaling, rise,
  quote operators, q/Q, Form matrices and annotation Rect/BBox placement. CID
  W/DW widths and core-font glyph metrics drive advances. Character spacing is
  per encoded glyph, including multi-codepoint Unicode mappings. Missing metrics
  have explicit estimates/diagnostics; quads are font/advance bounds, not ink.
- External raster images and common inline images retain PNG assets and placement.
  Soft masks, stencil fill colors and RGB/Gray JPEG Decode are handled. Inline
  boundaries use sample length or filter framing, including JPEG segments;
  embedded `EI` bytes do not terminate raw/JPEG image data.
- MCIDs/tags and logical ActualText replacements persist, with replacement paint
  regions. Painted spans remain `unstructured`; baseline grouping and inserted
  separators are explicitly `inferred`. Logical tagged-PDF trees are **not yet
  reconstructed**, and their presence emits `pdf_structure_pending`.
- SF/legacy JSON preserve geometry and provenance without duplicating content.
  Rendered DOCX/PDF require AllowLossy for positioned provenance even if callers
  remove diagnostics; current writers reflow rather than reproduce those positions.
- New hand-authored-stream tests check coordinate values, state restoration,
  CID/ligature advances, page rotations/crops/units, Form/annotation placement,
  ActualText/MCID scopes, assets/masks, inline framing, strict SF round trips and
  malformed graph/operator/sample input. Existing 51 corpus cases pass ProseMirror
  validation (12,417 nodes after image/separator recovery).
- Independent pypdf/MuPDF checks qualify a targeted two-page source and its SF,
  including rendered annotation placement, image pixels/alpha, text baselines and
  rotated physical coordinates. SF-generated DOCX/PDF preserve supported content
  and three painted images under acknowledged reflow. Both source pages and both
  reflowed PDF pages were visually inspected. This is synthetic qualification.
- Full tests/race/vet and host CGO-disabled CLI build pass. A ten-second two-worker
  PDF fuzz campaign passed 5,572 executions after the final image changes; earlier
  campaigns passed 89,922/15,205. SF fuzz passed 69,741 with a provenance seed.
  No new runtime module was added. See verification.json for exact evidence.

Required unfinished recovery: tagged semantic trees, reading order, headings,
lists/tables (including spans), hyperlinks and broader image/color/font/graphics
qualification. Vertical text, explicit/color-key masks, Matte unpremultiplication,
JPX decoding and inline LZW/CCITT framing currently reject; these are outstanding
work, not exclusions that satisfy the active goal. Clipping, vector painting,
extended graphics-state effects, color-profile/resampling losses and out-of-crop
geometry are diagnosed. No OCR or visual-equivalence claim is made.

## Earlier milestone: typed notes and note-aware pagination

- `Run.Note` and SF inline `note_reference` atoms preserve reference sites and
  unique footnote/endnote identities. Draft definitions may persist without an
  anchor; rendered output rejects invisible unanchored notes. Duplicate/dangling
  identities, invalid stories and contradictory metadata reject.
- DOCX writes/reads standard note parts with rich bodies and per-part links and
  assets. Ignorable metadata retains opaque application IDs and definition order.
  Repeats use cached NOTEREF fields bound to checked first-reference bookmarks,
  avoiding duplicate note bodies. Custom/default numbering, cached-mark, layout
  and separator omissions have explicit diagnostics.
- PDF measures superscript markers and internal links, reserves text/list
  footnotes before painting body/list/table references, and uses labelled
  continuation-only pages for long notes. Endnotes use a separate normal flow
  section. Rich nontext footnotes remain required unfinished layout work.
- Independent DOCX readers checked parts, IDs/order, types/relationships, repeated
  field targets, bold and links. Native two-page and five-page PDFs passed content,
  reference-target, bounds, separation, footer and continuation checks. All seven
  native pages plus both LibreOffice 7.6 DOCX-rendered normal-example pages were
  visually inspected. ProseMirror checked 34 normal/28 continuation-example nodes.
- Independent checks caught an endnote destination pointing to page one; gofpdf
  requires SetLink page=-1 for the current page. Fixed with a direct page-object
  regression. Adjacent references also retain separate annotations.
- Tests cover raw OOXML note/reference oracles, SF/DOCX/JSON semantic round trips,
  cross-kind IDs, repeated references, note-local links/assets/tables, effective
  numbering settings, malformed graphs/bindings, inline references inside merge
  continuations, and long-note preservation.
- Full tests, race checks, vet and a host CGO-disabled CLI build passed. DOCX/SF
  ten-second two-worker fuzz campaigns with note seeds passed 15,506/172,203
  executions. No runtime dependencies were added. `git diff --check` passed.

The full goal remains incomplete. See `verification.json.typed_note_milestone`
for evidence and ACCEPTANCE.md for all remaining required work.

## Earlier milestone: explicit lists

- SF/engine lists now have explicit items, starts (including zero), boundaries,
  nested lists and multiple blocks per item. The legacy paragraph-list API is
  normalized by a compatibility adapter; explicit adjacent lists never merge.
- DOCX import resolves per-level/instance definitions, start overrides,
  effective styles, cancellation, counter continuation and restart rules.
  Unsupported number formats/labels and inferred/skipped hierarchy are diagnosed.
- DOCX output renders standard numbering, with invisible bookmark ranges for
  exact exported list/item membership. Broken or contradictory ranges reject.
  A content-control approach was discarded because LibreOffice exposed hyperlink
  instructions; the bookmark version preserves links and bold appearance.
- PDF body/cell lists measure hanging indents, wrap/paginate text and retain
  nested starts, markers and continuation paragraphs. The synthetic list example
  passed independent DOCX numbering/boundary/format/link checks, PDF positions
  and link checks, and ProseMirror checks (51 nodes per example). Native PDF
  and LibreOffice 7.6 DOCX rendering were visually inspected. Its compact native
  PDF passed independent checks and visual inspection across three pages.
- Go tests additionally cover hand-authored multilevel OOXML with start/restart
  oracles, zero starts, styles/cancellation, continuation across ordinary text,
  list item images/tables, inline image normalization and malformed ranges.
- Full ordinary tests, race checks, vet, and the host CGO-disabled CLI build
  passed. Ten-second, two-worker DOCX/SF fuzz campaigns passed 22,404/203,187
  executions with list seeds. Existing deterministic damaged-input tests pass.
- All 51 accepted corpus SF outputs still pass ProseMirror validation (12,415
  nodes). The report and merged-table independent reader checks also pass.
  `git diff --check` passed; no runtime dependencies were added.

Current list check results are in `verification.json.explicit_list_milestone`.
These remain synthetic qualifications; the full goal is still incomplete.

## Earlier milestone: merged tables and constrained DOCX widths

- `Cell.RowSpan` and SF `rowspan` now persist vertical and combined row/column
  merges. Covered cells are omitted from later model/SF rows. One shared,
  bounded grid validates geometry and drives DOCX/PDF output.
- DOCX restart/continue regions are folded/recreated without duplicating content.
  Invalid continuations reject; nonempty continuation content survives at its
  origin with an explicit placement diagnostic.
- PDF text tables measure spanning-cell height constraints and paginate connected
  merged row groups together. Oversized groups, nested/nontext cells and full
  long-row splitting are still required unfinished work.
- Independent LibreOffice 7.6 rendering caught off-page cells caused by fixed
  DOCX grid widths. The writer now constrains tables and nested grids to their
  page/parent-cell content area; a regression checks both container widths.
- `examples/merged-tables/document.sf` is a hand-authored SF source example.
  Its generated DOCX passed python-docx merge-identity/content/format/link checks;
  PDF passed pypdf's exact seven-rectangle geometry/text/link checks; ProseMirror
  checked 31 nodes. The native PDF and LibreOffice-rendered DOCX were visually
  inspected after the width fix, with all seven cell texts retained on one page.
- Full Go tests, race checks, vet and host CGO-disabled CLI build passed after
  the implementation changes. DOCX and SF fuzz campaigns with merge seeds
  passed 19,800 and 157,962 executions respectively (10 seconds, two workers).
  Existing damaged-input tests are included in the ordinary suite.
- The existing 51 accepted corpus SF exports still pass independent ProseMirror
  checks; the report's independent DOCX/PDF checks also pass after regeneration.

See `verification.json.merged_table_milestone` for the evidence summary. These
are targeted synthetic checks, not representative real-export qualification.

## Verified for the earlier SF milestone

- `GOWORK=off go test ./... -count=1`, `go test -race ./... -count=1`,
  `go vet ./...`, and `go mod verify` passed.
- `CGO_ENABLED=0 GOWORK=off go build -o bin/manydoc ./cmd/manydoc` passed;
  that host binary was executed against all 51 accepted corpus files.
- SF bridge tests compare all existing model fields across those 51 inputs,
  including encrypted PDFs. Rich synthetic coverage checks marks/links, lists,
  nested tables/spans, assets, metadata/layout, running content, notes and losses.
- Actual `prosemirror-model` 1.25.13 read and checked all 51 corpus SF outputs
  (12,415 nodes) and the report (40 nodes), including marks and attributes.
- A 10-second SF fuzz campaign with two workers passed 177,117 executions.
- CLI `.sf` → DOCX/PDF report outputs passed python-docx and pypdf/Poppler checks.
  Both PDF page PNGs were inspected. DOCX visual rendering remains unverified.
- `git diff --check` passed. No new runtime dependencies were introduced.

Current evidence is recorded in `verification.json.structured_format_milestone`.
The earlier report in that file and the baseline section below are historical;
coverage percentages and cross-platform builds have not been refreshed for SF.

## Code map

| File | Authority |
| --- | --- |
| structured.go / schema/prosemirror-v1.json | ProseMirror tree, codec, bridge and executable schema declaration |
| document.go | Public model, validation, Read/Write dispatch, JSON/text, diagnostics |
| notes.go / docx_notes.go / pdf_notes.go | Note graph, standard DOCX parts/references, PDF reservations/continuations/targets |
| lists.go / docx_lists.go / pdf_lists.go | Legacy adapter, explicit list boundaries, numbering and PDF indentation/layout |
| table_grid.go / docx_table.go | Shared span geometry, DOCX merge import/export and constrained widths |
| docx_read.go | OOXML package/XML reader, relationships and visible content |
| docx_write.go | OOXML generation, styles/numbering, images and part-local relationships |
| pdf_tokens.go | PDF operands and ToUnicode code maps |
| pdf_text.go | Unicode atom measurement, paragraph-wide segmentation, proportional wrapping and ActualText painting |
| pdf_read.go / pdf_geometry.go / pdf_font_metrics.go | PDF interpreter, positioned page/form/annotation text and font metrics |
| pdf_structure.go / pdf_structure_blocks.go / pdf_links.go | Checked forward logical tree, recovered semantic blocks and URI ownership |
| source.go / pdf_images.go / pdf_inline.go / pdf_marked.go | Provenance validation, raster assets/framing, marked content and ActualText |
| pdf_write.go | Flow layout, embedded fonts, text tables/images and PDF generation |
| cmd/manydoc | CLI, cancellation, diagnostics and atomic output replacement |
| examples/report / examples/merged-tables / examples/lists / examples/notes / examples/typography | Report and SF-first feature examples |
| testdata/corpus | Self-contained 64-case corpus and SHA-256 manifest |
| scripts | Independent fixture generation and verification tools |

## Initial implementation baseline (historical)

- Ordinary Go tests passed: 21 top-level tests, including named corpus cases,
  content regressions, generation round trips, invalid models/IO, passwords,
  cancellation, concurrent operations, hashes and CLI atomic-output tests.
- Coverage: library 76.4%, CLI 75.0%. The example program is exercised by
  independent verification, rather than covered as a unit test.
- Race tests passed after fixing font-buffer ownership. The initial failure is
  in ignored output/race.log; output/race-final.log is the successful check.
- go vet passed; go mod verify reported all modules verified.
- CGO-disabled CLI builds passed for darwin/arm64, linux/amd64 and linux/arm64.
  Linux binaries were cross-compiled, not executed on Linux.
- All 28 non-malformed named PDF inputs matched Poppler after whitespace
  normalization, including supported password cases.
- python-docx independently reopened the generated DOCX and verified metadata,
  text, links, formatting, a table/image and header/footer. pypdf/Poppler verified
  two-page PDF output; both PDF pages were rendered and visually inspected.
  No Word/LibreOffice visual acceptance is claimed for DOCX.
- 200 deterministic damaged-file mutations completed without escaping panics.
  This is a robustness count, not 200 real-world compatibility cases.
- Short DOCX/PDF/CMap fuzz campaigns passed. The durable counts and budgets are
  in verification.json; they do not establish exhaustive safety.

Ignored output/ contains raw test logs, comparisons, generated examples, page
PNGs and the optional Python environment. bin/ contains local/cross executables.
Both are regenerable. docs/verification.json preserves a compact source summary.

## Recover and reproduce

```sh
cd ~/workspace/code/github.com/artpar/manydoc
git status --short
GOWORK=off go test ./... -count=1
GOWORK=off go test -race ./... -count=1
GOWORK=off go vet ./...
CGO_ENABLED=0 GOWORK=off go build -o bin/manydoc ./cmd/manydoc
GOWORK=off go run ./examples/report -out output
```

TESTING.md contains independent checks and optional Python/Poppler setup.
Ordinary Go tests need no Python, Poppler, Daptin checkout, customer files,
license server or external converter. Initial module download uses normal Go
module access. The go.mod selects Go 1.25; do not force an older local toolchain.

## Important implementation details

- Never use pdfcpu NewDefaultConfiguration here: it may create user files or
  exit the process. Configure each read explicitly; do not mutate global state.
- PDF objects accept common version/extension conventions; page-stream decoding
  is subsequently strict so corrupt streams cannot yield successful partial text.
- Keep per-renderer gofpdf font byte copies. Subsetting mutates its input;
  sharing embedded slices caused the race detected and fixed in this session.
- PDF font resource names are page-local. Key fonts by actual objects, not a
  document-wide cache keyed by names such as /F2+0.
- The missing document-relationships fixture was corrected because that part
  is optional for its contents. DECISIONS.md explains this; required/dangling
  parts remain errors.
- Rich notes, output font coverage/shaping, some PDF image codecs/masks and advanced layouts
  have explicit limitations in COMPATIBILITY.md.

## Next work (required for the active goal)

No external blocker currently prevents development. Editable PDF family/physical
size/device-color recovery now exists with provenance and uncertainty diagnostics.
DOCX theme tint/shade and settings color mappings now calculate resolved RGB.
DOCX font-slot and theme-language selection now resolves current spans with explicit
uncertainty/binding-loss diagnostics. Glyph-ID transport plus supported LTR grapheme shaping now exists. Next implement
paragraph-wide shaping/bidi and language/direction semantics, source-font handling
and continued color management/selection qualification;
continue page/running content, broader PDF recovery and representative real exports.
These remain required acceptance work. Existing precision restrictions can require
explicit caller retypesetting of imported PDFs; preserve original SF and disclose
that choice. Do not silently substitute font programs or round source values.

Next concrete commands:

```sh
GOWORK=off go test -run 'TestPDFRecovered|TestPDFActualTextRetains|TestPDFSourceStyle|TestContentNumbers' -count=1
GOWORK=off go test -run '^$' -fuzz '^FuzzPDFEditableStyles$' -fuzztime=1000x -parallel=2 -timeout=90s
output/qa-venv/bin/python scripts/verify_pdf_text_styles.py
GOWORK=off go test -run 'TestDOCXTheme' -count=1
output/qa-venv/bin/python scripts/verify_docx_theme_colors.py
GOWORK=off go test -run 'TestDOCXFont|TestDOCXScript' -count=1
output/qa-venv/bin/python scripts/verify_docx_font_selection.py
GOWORK=off go run ./examples/shaping
output/qa-venv/bin/python scripts/verify_default_shaping.py
GOWORK=off go run ./examples/language
output/qa-venv/bin/python scripts/verify_language.py
GOWORK=off go test -run 'TestLanguage|TestDOCXLanguage|TestPDFLanguage|TestPDFDirection' -count=1
rg -n 'measureLanguageAtoms|shapeLanguageLine|emitFit|Direction' pdf_shaping.go pdf_text.go pdf_direction.go
GOWORK=off go doc golang.org/x/text/unicode/bidi
GOWORK=off go run ./examples/glyphs
output/qa-venv/bin/python scripts/verify_glyphs.py
GOWORK=off go test -run 'TestPDF.*Glyph|TestPDFNoncollinear' -count=1
rg -n 'validateFontText|textAtoms|CID|ToUnicode|glyph' pdf_write.go pdf_text.go pdf_font.go pdf_glyph.go
GOWORK=off go doc github.com/benoitkugler/textlayout/harfbuzz
GOWORK=off go run ./examples/text-style
output/qa-venv/bin/python scripts/verify_text_style.py --office output/text-style/lo/text-style.pdf
GOWORK=off go test -run 'TestTextStyle|TestDOCXText|TestSuppliedNoto|TestPDFExplicitFont' -count=1
GOWORK=off go test -run '^$' -fuzz '^FuzzRichTextStyles$' -fuzztime=1000x -parallel=2 -timeout=90s
GOWORK=off go test -run '^$' -fuzz '^FuzzCustomPDFFont$' -fuzztime=500x -parallel=2 -timeout=90s

rg -n 'type Run|rFonts|font|sz|color|size|SetFont|pdfAtom|text_style' document.go structured.go docx_read.go docx_write.go pdf_text.go pdf_write.go
GOWORK=off go run ./examples/repeated-headers
output/qa-venv/bin/python scripts/verify_repeated_headers.py --office output/repeated-headers/lo/repeated-headers.pdf
GOWORK=off go test -run 'TestRepeating|TestDOCX.*Header|TestInvalidRepeat' -count=1
GOWORK=off go test -run '^$' -fuzz '^FuzzRepeatingTableHeaders$' -fuzztime=20s -parallel=2 -timeout=50s

rg -n 'type Row|table_row|tblHeader|pdfTableGroup|continueTableGroup' document.go structured.go docx_table.go pdf_cell_layout.go pdf_table_continue.go
GOWORK=off go run ./examples/image-fit
output/qa-venv/bin/python scripts/verify_image_fit.py
GOWORK=off go test -run 'TestImage|TestImages|TestFirstImage|TestOutputImage|TestPreviouslyOversized|TestJPEG' -count=1
GOWORK=off go test -run '^$' -fuzz '^FuzzImageContainerFitting$' -fuzztime=20s -parallel=2 -timeout=50s
GOWORK=off go test -run 'TestImage|TestPDFRich|TestParagraph|TestDOCXParagraph' -count=1
GOWORK=off go run ./examples/paragraph-breaks
output/qa-venv/bin/python scripts/verify_paragraph_breaks.py
GOWORK=off go test -run '^$' -fuzz '^FuzzDOCXParagraphBreakRoundtrip$' -fuzztime=15s -parallel=2 -timeout=45s
GOWORK=off go test -run 'TestPDF.*Break|TestPageBreak' -count=1
GOWORK=off go run ./examples/page-breaks
GOWORK=off go run ./examples/page-breaks -empty-note -out output/page-breaks/empty
output/qa-venv/bin/python scripts/verify_page_breaks.py
output/qa-venv/bin/python scripts/verify_page_breaks.py output/page-breaks/empty --empty-note
output/qa-venv/bin/python scripts/author_paragraph_break_variant.py
GOWORK=off go test -run 'TestPDFRecursive|TestRecursiveTable' -count=1
GOWORK=off go run ./examples/recursive-tables
output/qa-venv/bin/python scripts/verify_recursive_tables.py
GOWORK=off go test -run '^$' -fuzz '^FuzzPDFRecursiveContinuation$' -fuzztime=20s -parallel=2 -timeout=50s
GOWORK=off go test -run 'TestPDFTableContinuation|TestTableContinuation|FuzzPDFTableContinuation' -count=1
GOWORK=off go run ./examples/table-continuation
output/qa-venv/bin/python scripts/verify_table_continuation.py
GOWORK=off go test -run '^$' -fuzz '^FuzzPDFTableContinuation$' -fuzztime=15s -parallel=2 -timeout=40s
GOWORK=off go test -run 'TestPDFRich|TestImage' -count=1
GOWORK=off go run ./examples/rich-cells -out output/rich-cells
GOWORK=off go run ./examples/rich-cells -out output/rich-cells/continued -continued
output/qa-venv/bin/python scripts/verify_rich_cells.py output/rich-cells
output/qa-venv/bin/python scripts/verify_rich_cells.py output/rich-cells/continued --continued
GOWORK=off go test -run 'TestPDF(Ruled|Merged|Untagged|Tagged|Published)' -count=1
GOWORK=off go test -run '^$' -fuzz '^FuzzPDFRuledGrid$' -fuzztime=15s -parallel=2 -timeout=40s
CGO_ENABLED=0 GOWORK=off go build -o bin/manydoc ./cmd/manydoc
output/qa-venv/bin/python scripts/verify_pdf_rulings.py
output/qa-venv/bin/python scripts/verify_pdf_rulings_office.py
output/qa-venv/bin/python scripts/verify_pdf_inference.py
output/qa-venv/bin/python scripts/verify_published_pdf.py
GOWORK=off go run ./examples/typography
output/qa-venv/bin/python scripts/verify_typography.py
NODE_PATH=output/sf-qa/node_modules node scripts/verify_structured.cjs output/typography/typography.sf
GOWORK=off go test ./... -count=1
GOWORK=off go test -race ./... -count=1
GOWORK=off go vet ./...
CGO_ENABLED=0 GOWORK=off go build -o bin/manydoc ./cmd/manydoc
python3 scripts/verify_structured_corpus.py
GOWORK=off go run ./examples/notes -out output/notes
GOWORK=off go run ./examples/notes -out output/notes/continued -page-height 220 -long-note
output/qa-venv/bin/python scripts/verify_notes.py output/notes
output/qa-venv/bin/python scripts/verify_notes.py output/notes/continued --continued
git diff --check
```

Install optional ProseMirror QA dependencies as documented in TESTING.md before
running the corpus verifier. Keep STATUS, ARCHITECTURE, DECISIONS, COMPATIBILITY,
ACCEPTANCE and the structured profile current as each required feature lands.
