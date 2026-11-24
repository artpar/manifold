# Native browser qualification and repeated-note failure

The public offline VDOCS adapter preserves its qualified SF graph. That does not
prove native browser placement. The first full-preset browser probe now reproduces
a concrete ordering failure on the unchanged public notes example.

## Reproduction

Optional QA uses Node/npm/esbuild and a local development HTTP server. Production
remains in-process pure Go; none of these tools or services is a runtime dependency.
Only public/project fixtures are used. No Daptin or Colleaf application is modified.

```sh
GOWORK=off go run ./examples/colleaf -out output/colleaf-tagged
node scripts/prepare_colleaf_browser.cjs
python3 -m http.server 8768 --bind 127.0.0.1 --directory output/colleaf-browser
# Open http://127.0.0.1:8768 in a browser.
# Inspect window.qa.ready, qa.result, qa.errors, qa.snapshot(), qa.renderedNotes().
```

The preparation script uses the unmodified actual Colleaf mount and validator at
[commit 5ce2ef5](https://github.com/artpar/colleaf/tree/5ce2ef507b28b92f5b03443ade87bc0e17f3d8b6).
It checks SHA-256 before bundling:

| Source | SHA-256 |
| --- | --- |
| src/integrations/univer/document.ts | 11d3fe8caadd351a3871b82ceecba3285cd77e02f777f7f7b1fecc57252b499a |
| src/shared/files/docsFormat.ts | c54b27b8c303713f5d5d5dfdb7cd54ec9b8410f5045613150861855b11c899c0 |

It installs exact Univer 1.0.4 core/drawing/hyperlink presets and find/replace,
with pinned icons 1.43.1 and React/RxJS/esbuild development dependencies in ignored output.
The helper’s wrapper only mounts the actual integration, waits for its lifecycle,
exposes current snapshots/rendered-note records and reports errors. It is not a
replacement editor, source body cache or production converter.

## Observed result

The desktop Chromium probe reaches lifecycle stage 3 with one canvas and no
reported runtime errors. Fresh-load visual inspection shows the complete main
text, note bodies/markers/list and footer in the one native page. This does not
claim font or pagination equivalence with Word/PDF output.

The original main text first cites footnote manydoc-note-2 at UTF-16 index 36,
then manydoc-note-3 at 127, then repeats manydoc-note-2 at 147. The required body
order follows first references: note-2 then note-3. Actual browser render records
and the visible page instead place note-3 (label 2) above note-2 (label 1).
Both body texts remain; content/identity preservation alone missed this failure.

The pinned engine’s DocumentFootnoteLayout constructor indexes references by
note ID using the last occurrence, then sorts body placement by that stored
referenceIndex. Local inspection of the installed attributed open-source package
shows the cause. No vendor implementation was copied into the Go library.

A separate control replaces only the later repeated note atom/range with literal
text 1. Its body order is note-2 then note-3 in both browser and headless renderer.
The original fixture and first-reference-order oracle are unchanged. This control
isolates the repeated-reference behavior; it is not a replacement acceptance input.
Fresh-load original and control PNGs were visually inspected in ignored output.
Remount paint timing is not qualified by this probe; use a fresh load for its visual
reproduction.

The new acceptance checker intentionally fails on the current pinned engine:

```sh
node scripts/verify_colleaf_note_order.cjs
```

It derives the expected order from the original native reference graph, checks
actual body placement, and separately verifies the no-repeat control. The original
expects [manydoc-note-2, manydoc-note-3]; actual is reversed. Existing headless note
checks asserted labels and a set of placed bodies, so they did not detect order.
Ordinary Go tests do not execute this optional native-reader check. Their passing
result does not close the native rendering gap.

## Unfinished work

Resolve the repeated-note placement behavior without changing the original oracle,
duplicating definitions, replacing live citations with static body caches, or
patching Daptin/Colleaf as part of this project. No safe data-level solution is
established yet. The broader browser edit/save/reopen, navigation, image/font/
wrapping and mobile/preset checks remain unqualified. The full goal stays active;
this is a precise newly reproduced gap, not a declaration that all work is blocked
or a reason to narrow the objective.


## Raw range reordering is not a stable repair

The pinned API exposes noteId on a citation range, with no independent repeated-
citation placement identity. A new separate control swaps the first/repeated
range records without moving their actual text positions. This initially makes
the headless body order correct, proving the renderer depends on input ordering.
A normal TextX insertion of X at the start sorts ranges to their current positions;
body order immediately reverses again, with stored reference indices 128/148.

The acceptance checker now logs this before/after-edit control as well as the
original and no-repeat results. It still fails the unchanged original first-
reference-order assertion. Reordering raw arrays would therefore only hide the
problem until editing and is not adopted in the library. Other native API/schema
representations or the remaining goal work still require investigation; no overall
blocked state follows from this scoped external-engine limitation.
