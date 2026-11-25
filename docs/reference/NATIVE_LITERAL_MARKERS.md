# Native literal markers, measured counters and browser qualification

The complete goal remains active. Public VDOCS generation now writes rich literal
markers through checked native label/body content. Exact SF/source graphs, pinned
SDK saves/edits, browser image/counter paint and six-page label/body continuation
have scoped evidence. This does not establish general native/Word fidelity or close
the broader acceptance queue. DOCX/PDF mappings remain available.

## Implemented mapping

`internal/engine/vdocs_lists.go` writes actual native two-cell tables, with equal
relative columns and mirrored cell order for RTL. `manydocLiteralItem` contains
only version/group/index/count, list/counter intent, automatic/literal mode and
side. It contains no text, asset bytes or SF subtree. Literal label and item body
blocks remain actual paragraphs, tables, lists, drawings and citations.

Consecutive checked table owners reconstruct one SF list. Identity, contiguous
indices, count, shape, spans, side, body start, counter intent and observed automatic
counter values must agree. Unknown private fields (including a fake content cache)
and malformed metadata reject. Stale/missing ownership retains actual native tables
and their content with diagnostics instead of forcing an old list around edits.

Automatic items use actual native bullet definitions and empty label paragraphs,
with explicit observed starts. Their label paragraphs align right inside the cell;
without this, the native browser clipped Item 7) and Item 9) to their final suffix.
Only an empty paragraph in a checked automatic-label cell consumes that layout
alignment. The actual generated fix was observed in the browser; literal labels
retain their own formatting. This does not qualify arbitrary native list layout.

List/item/marker source owners use checked source-only references and fingerprints
on the actual body paragraph. Marker blocks participate in existing paragraph,
run, anchor, image, table, flow, citation and standalone-point walks. Image fitting
uses the same two-cell traversal/order as emission. There is one canonical SF;
this native metadata is role/counter/source intent, not a second document cache.

## Passing evidence and limits

Go tests compare complete normalized SF graphs for rich labels, exact source
owners, mixed counters, nested tables/lists/images, notes/running stories and RTL.
Actual native TextX edits and facade saves reimport with their current label text,
relationships and source facts. Stale group tests retain original visible tokens.
The independent checker pins the actual Colleaf validator and Univer 1.0.4 packages.
It verifies native label/body glyphs, table geometry, automatic counter glyphs,
citation/body presence, header caches, TextX edits and actual facade saves. SDK
geometry is not canvas paint evidence, and successful persistence is not fidelity.

## Browser image failure resolved; layout qualification remains

The original strict pixel oracle initially failed in nested, running/note and
standalone controls: real drawing descriptors and independently decoded pure red
sources produced zero red pixels. Actual native `FDocument.insertImage` painted
18020 red pixels and its saved snapshot also painted on reload. Comparing the
registered snapshot exposed the missing `DOC_DRAWING_PLUGIN` resource. The pinned
controller resets/registers root drawing sources from this resource on load; a
source map alone is insufficient. The Go whole-document encoder now emits the
SDK's root data/order mirror. Notes retain local sources and the controller
collects them; headers retain their existing global source relationships. No
proprietary code or vendor/application patch was used.

Unchanged source/pixel oracles now pass at the full-page 1000×1400×1 viewport:
standalone and nested labels each paint 441 red pixels; the running/note case paints
420 footnote red pixels. Its header image is visibly pink because the pinned UI
dims inactive running content (observed exact RGBA255,128,128,255). The probe adds
an independent upper-page pink-pixel check while retaining the original strict red
threshold and exact decoded source. This checks both displayed story copies; it
does not reinterpret the source color. Browser screenshots were visually inspected
for the nested case and the full-page header/note case, with no runtime errors.

Generic scene-transform probes alone had failed; they were not the fix. Current
source descriptors remain unchanged. The drawing mirror is derived registration
of actual native sources, not a hidden SF/text cache, and contradictions still
reject. The existing resize test now updates both source and mirror to model a
valid native edit; its original actual-frame-wins oracle remains unchanged.

The missing nested ordinary counter is now resolved. Actual native list insertion
showed the indent missing from the Go list definition. Ordinary logical levels now
use a21px indentation step with zero hanging, preserving the native engine's actual
measured counter advance. A fixed21px hanging frame initially made long automatic
labels overlap their body: the original numbering example exposed Appendix AA.,
compound, Roman and table-cell labels wider than that frame. Zero hanging retains
their measured advances (e.g. Appendix AA.88.874px content/94.5px advance), and the
actual browser shows full counters followed by separate body text. Custom foreign
indents still diagnose presentation loss; only the generated convention is consumed.

Empty right-aligned automatic-label cells retain their measured-width layout without
these ordinary paragraph properties. A regression test/view caught the fixed-frame
variant truncating Item 7)/Item 9); the exact original full-prefix oracle is retained.

New browser probes require complete counter strings and advance >= measured glyph
width. The nested marker shows1. INNER_LIST. A narrow300×400pt fixture wraps96 unique
label/body tokens across six native pages, with both columns continuing and every
token present exactly once. All six pages were visually inspected in two overview
screenshots. Desktop1000×1400 and mobile preset375×812 touch runs preserve the same
counter/token graphs; all six actual app saves reimport through ReadVDOCS with exact
SF semantics. The mobile nested view was also inspected. SDK facade saves cover the
long fixture separately. These are targeted synthetic tests, not universal fidelity.

## Reproduction

```sh
MANYDOC_NATIVE_LITERAL_QA=output/colleaf-qa/literal-markers.json MANYDOC_NATIVE_LITERAL_QA_DIR=output/native-literal GOWORK=off go test -run TestVDOCSLiteralMarker -count=1 ./internal/engine
node scripts/qa/vdocs/verify_vdocs_literal_markers.cjs
MANYDOC_NATIVE_LITERAL_SAVED_DIR=output/native-literal MANYDOC_NATIVE_LITERAL_SAVED=output/colleaf-qa/literal-markers-edited.json GOWORK=off go test -run 'TestVDOCSLiteralMarkerActualSaved' -count=1 ./internal/engine
node scripts/qa/vdocs/prepare_vdocs_browser.cjs output/native-literal/rich.vdocs
cp output/native-literal/{nested,stories,mixed,image-control,continued}.vdocs output/colleaf-browser/
cp scripts/qa/vdocs/vdocs_literal_browser_probe.js output/colleaf-browser/literal-browser-probe.js
cp scripts/qa/vdocs/vdocs_list_browser_probe.js output/colleaf-browser/list-browser-probe.js
python3 -m http.server 8768 --bind 127.0.0.1 --directory output/colleaf-browser
```

Open the owned local harness at localhost:8768. A 1000×1400 viewport exposes the
full reference page and its footnote. Evaluate in the browser:

```js
const probe = Function('return (' + await (await fetch('/literal-browser-probe.js', {cache:'no-store'})).text() + ')')();
await probe('nested', true); // unchanged source red; redPixels:441
await probe('image-control', true); // redPixels:441
await probe('stories', true); // footnote redPixels:420 plus dimmed header pixels
```

For the native list/counter/pagination probe:

```js
const lists = Function('return (' + await (await fetch('/list-browser-probe.js', {cache:'no-store'})).text() + ')')();
await lists('nested');
await lists('mixed');
await lists('continued'); // six pages,96 tokens
await lists('continued',true); // actual mobile preset; use375×812 touch viewport
```

Save each actual qa.snapshot() envelope into ignored output/native-literal using
nested/mixed/continued-browser.vdocs and -mobile-browser.vdocs names. Reimport:

```sh
MANYDOC_NATIVE_LIST_BROWSER_DIR=output/native-literal MANYDOC_NATIVE_LITERAL_SAVED_DIR=output/native-literal MANYDOC_NATIVE_LITERAL_SAVED=output/colleaf-qa/literal-markers-edited.json GOWORK=off go test -race -run 'TestVDOCSLiteralMarker(BrowserSaved|ActualSaved)' -count=1 ./internal/engine
```

The original broader acceptance/source/resource/reflow/corpus audits remain open.
Next reproduce the separate repeated-footnote native order failure without changing
its oracle, and continue the library's broader source/resource audit. The Word-PDF
link discrepancy remains in LITERAL_LIST_MARKERS.md. Neither is resolved by native
list generation. Oversized/unbreakable custom counter and broader font/layout cases
still need corpus qualification; successful synthetic counters are not a universal
layout guarantee. See STATUS.md and ACCEPTANCE.md for the full queue.

To qualify the existing original automatic-numbering example separately:

```sh
GOWORK=off go run ./examples/numbering
GOWORK=off go run ./cmd/manifold convert -o output/colleaf-browser/numbering.vdocs output/numbering/source.sf
```

Then evaluate `await lists('numbering')` in the actual harness. Its fixed original
counter oracle includes Appendix AA., compound, legal, Roman and table/note labels;
the measured-advance assertion fails if a fixed frame overlaps those glyphs.
