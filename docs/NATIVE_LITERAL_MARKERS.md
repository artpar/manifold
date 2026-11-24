# Native literal-marker prototype and remaining paint failure

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

## Browser failure retained

The unmodified pinned Colleaf mount reaches lifecycle stage 3, with no runtime
errors. Rich alpha/beta labels, the empty label, note body, the fixed automatic
prefixes and nested table text are visible. **The red label image is not painted.**
The strict pixel oracle was not weakened: both nested and standalone controls
have a real drawing record and a source image that independently decodes to
RGBA(255,0,0,255), yet the visible canvas has zero red pixels. The running/note
case also produced zero. Thus this is not proven to be caused only by added label
nesting; native drawing registration/initialization/paint must be investigated.
A nested ordinary numbered label also appeared clipped; wider native list-in-cell
layout remains unqualified.

The published drawing renderer skips images without a generic scene transform;
our snapshot carries docTransform. Isolated input probes adding generic transforms
at both zero and visible coordinates also produced zero red pixels. This is a
candidate to investigate, **not an established sole cause or a shipped fix**.
No application/vendor packages were patched, no source pixels changed, and no
customer documents or external conversion service were used.

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
const probe = Function('return (' + await (await fetch('/literal-browser-probe.js')).text() + ')')();
await probe('nested', false); // sourceDecodedRGBA:[255,0,0,255], redPixels:0
await probe('image-control', false); // same failure without a label table
await probe('nested', true); // retained FAIL: image was not painted
```

Next compare the same image inserted through the actual native image command with
the Go-generated snapshot, including drawing-manager registration and scene
transforms. Locate the insertion/render contracts in the pinned docs-drawing-ui
and drawing-ui packages. Fix the input/registration mapping where possible; retain
this oracle if the failure is external. Then qualify image pixels, list-in-cell
paint, long label pagination, mobile/preset saves and the original broader acceptance
matrix before enabling public marker export. The separate repeated-footnote order
failure remains in COLLEAF_BROWSER.md; the Word-PDF link-annotation discrepancy
remains in LITERAL_LIST_MARKERS.md. Neither is resolved by this prototype.
