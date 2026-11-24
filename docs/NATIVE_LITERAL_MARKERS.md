# Native literal-marker prototype and drawing registration

The complete goal remains active. Public VDOCS generation still rejects literal
markers before output. The internal implementation now preserves their actual
native content, relationships and source facts, but **does not yet qualify the
native rendered result**. This is required unfinished work, not an exception to
the rich-label goal. DOCX/PDF mappings remain available and unchanged.

## Implemented internal mapping

`colleaf_literal_markers.go` writes actual native two-cell tables, with equal
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

A nested ordinary numbered label remains absent/clipped in the browser despite
being present in the SDK glyph graph. Wider list-in-cell layout and long-label
pagination remain unqualified, so public marker generation stays guarded.

## Reproduction

```sh
MANYDOC_NATIVE_LITERAL_QA=output/colleaf-qa/literal-markers.json MANYDOC_NATIVE_LITERAL_QA_DIR=output/native-literal GOWORK=off go test -run TestColleafLiteralMarker -count=1 .
node scripts/verify_colleaf_literal_markers.cjs
MANYDOC_NATIVE_LITERAL_SAVED_DIR=output/native-literal MANYDOC_NATIVE_LITERAL_SAVED=output/colleaf-qa/literal-markers-edited.json GOWORK=off go test -run 'TestColleafLiteralMarkerActualSaved' -count=1 .
node scripts/prepare_colleaf_browser.cjs output/native-literal/rich.vdocs
cp output/native-literal/{nested,stories,mixed,image-control}.vdocs output/colleaf-browser/
cp scripts/colleaf_literal_browser_probe.js output/colleaf-browser/literal-browser-probe.js
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

Next compare the ordinary nested-list counter's actual skeleton glyph position
and clipping geometry against a native inserted numbered paragraph in a table.
Preserve its original counter/text oracle. The pinned engine default bullet
skeleton supplies indentStart/hanging21, while a concrete list definition uses
its own paragraphProperties; the Go list definition currently omits them. Its
layoutParagraph then defaults hanging to glyph width without a matching indent.
This is a candidate clipping cause, not a qualified fix. Check actual native
inserted list definitions and explicit/inferred indentation behavior before
changing the mapping or consuming those properties in the reader.
Qualify long label pagination,
mobile/preset saves and the original broader acceptance matrix before enabling
public marker export. The separate repeated-footnote order failure remains in
COLLEAF_BROWSER.md; the Word-PDF link-annotation discrepancy remains in
LITERAL_LIST_MARKERS.md. Neither is resolved by drawing registration.
