# PDF glyph-font subsetting and real-source font qualification

Updated 2026-11-24. Full goal incomplete; no external blocker. This checkpoint
changes in-process glyph embedding, not canonical SF or font substitution policy.

## Implementation and format evidence

`font_subset.go` consumes an already admitted TrueType program plus painted GIDs.
It preserves original glyph IDs and global metrics/cmap/names/notices/hinting.
Unused outlines become empty loca entries; used glyphs and every transitive
composite component retain their outline/instruction bytes. Iterative closure
and topological validation reject invalid component IDs and cycles without
recursive traversal. Context checks occur throughout selection and rebuilding.
Caller font bytes remain unchanged. Existing embedding/subsetting flag admission
still applies. Simple, supplementary, substituted ligature, composite and
positioned combining glyphs share this path.

The output retains the source maxp/hmtx/cmap and all name records. It rebuilds
long-format loca, aligned glyf, sorted sfnt directory/search fields, table
checksums and head.checkSumAdjustment. Layout/signature tables are omitted after
shaping; the cmap can still map unused GIDs to empty outlines. This is a private
embedding program for pre-shaped PDF text, not a standalone font-editing API.
PDF FontFile2 Length1 uses its actual new length. CIDToGIDMap, ToUnicode,
ActualText, original descriptors/advances and painting positions remain stable.
Each used face receives a deterministic, document-unique six-uppercase-letter
subset prefix; physical FontFamily and original name/notices remain intact.

Primary references are Microsoft's OpenType
[glyf](https://learn.microsoft.com/en-us/typography/opentype/spec/glyf),
[loca](https://learn.microsoft.com/en-us/typography/opentype/spec/loca) and
[sfnt/checksum](https://learn.microsoft.com/en-us/typography/opentype/spec/otff)
specifications. The existing pdfcpu Subset API reads a named installed/cached
font, whereas manydoc admits caller-owned byte programs in process. The new
routine is independently authored against the published format; no proprietary
converter implementation was used and no isolated clean-room claim is made.

## Verification and size evidence

Original fonts/oracles remain unchanged. The previous full-program byte hashes
still authenticate source Go-font constants from pinned x/image v0.32.0. The QA
helpers now verify subset semantics independently with fontTools 4.68.0: original
GIDs, every used/component outline and instruction program, metrics, name/license
records, cmap and all unused empty locations. This replaces the obsolete claim
that optimized output must contain an identical full TTF. Text, native-HarfBuzz
GID/GPOS, relationship, raster and source-font oracles are retained.

A detached e1bb449 build generated the six baseline pages. The glyph example's
two pages and the default/emergency/marked-space shaping examples' four pages
retain exact page content streams and every 1.5x raster pixel. Baseline PDFs are
preserved under ignored output/font-subset/before/output; the temporary worktree
was removed. The before/after size pairs are 758,586→110,807,
451,574→75,362, 289,371→34,567 and 289,448→34,725 bytes respectively.
Native HarfBuzz default shaping (24 runs/six ligature runs), glyph, typography,
text-style, paint-context (eight rows/six caret clips), bidi (29 groups), language
and emergency-wrap verification pass. Actual ProseMirror validates the six new
example SF files; see verification.json for final node count and command results.

BabelStone Han 16.0.3 was downloaded only into ignored output/font-research from
its [primary publisher](https://www.babelstone.co.uk/Fonts/Han.html). The unmodified
static TTF is 51,971,284 bytes with SHA-256
`d8bb747b3fdccd84a60bd0aa56bb90937270d0bd15f1101cd8f2a5a3709dd0a3`.
Its original Arphic license remains in source name ID13 and in the retained PDF
font name table. The publisher allows personal/commercial use and modification
under that license; independently distributed modified fonts require separate
name/reuse consideration. No source/derived font asset is added to the repository
or runtime binary. TTF mapping U+2A6D4→u2A6D4 and all 65,534 original GIDs are
verified. Its embedding subset retains two outlined glyphs including .notdef,
uses 1,371,528 uncompressed font bytes, and shrinks the one-character PDF from
20,348,885 to 464,435 bytes. Every 2x raster pixel and exact pypdf/MuPDF text remain.
This is the selected font's appearance, not MingLiU/Word glyph equivalence.

Go tests check composite/supplementary outlines, advances, source ownership,
checksums, duplicate selections, bad offsets/references/cycles, permissions and
cancellation. Fuzzing covers malformed font admission and 500 valid GID selections.
Final full tests/race/vet/module and CGO-disabled host/Linux build results are in
verification.json. Optional Python tools are development-only; runtime has no new
module, executable, service, CGO, license server or customer-document upload.

## Reproduce the smaller examples

```sh
GOWORK=off CGO_ENABLED=0 go run ./examples/font-subset
GOWORK=off CGO_ENABLED=0 go run ./examples/font-subset -font output/font-research/BabelStoneHan.ttf -family 'BabelStone Han' -text 𪛔 -out output/font-subset/han-example
GOWORK=off go run ./examples/glyphs
GOWORK=off go run ./examples/shaping
output/qa-venv/bin/python scripts/verify_glyphs.py
output/qa-venv/bin/python scripts/verify_default_shaping.py
output/qa-venv/bin/python scripts/verify_font_subsets.py --han-font output/font-research/BabelStoneHan.ttf
GOWORK=off go test -run 'TestGlyph|TestPDFGlyph|TestPDFPositionedGlyph' .
```

The comparison command needs the preserved baseline PDFs. To recreate them,
create a detached e1bb449 worktree under output/font-subset/baseline, run its
examples/glyphs and examples/shaping with GOWORK=off, and pass `--baseline` that
worktree path. It creates all reference glyphs from original fonts, not edited
oracles. The single-Han baseline came from the previous pinned writer with the
same font, 400×200pt page, 24pt margins and 60pt character. Optional font download:
`curl -fL --max-time 60 -o output/font-research/BabelStoneHan.ttf https://www.babelstone.co.uk/Fonts/Download/BabelStoneHan.ttf`.
Verify the recorded hash before relying on a changed upstream download.

## Full Word source: actual progress and precise open failure

The current DOCX→SF output is byte-exact against the earlier 8.3MB source SF.
Using explicit caller mappings to all four DejaVu Sans 2.37 styles, Go Mono and
BabelStone Han renders the original source as 207 native pages/4,990,468 bytes.
The DejaVu files came from the already pinned original release ZIP SHA-256
`7576310b219e04159d35ff61dd4a4ec4cdba4f35c00e002a136f00e96a908b0a`.
A first Go-only mapping correctly rejected missing U+2012; the DejaVu mapping
retains all four figure dashes and the original single U+2A6D4 without changing
source characters. Every used native glyph font program passes the independent
subset/notice/metric audit, the original JPEG bytes remain, and title/page counts
agree in independent readers. The Han appears once in all-page MuPDF text and
once on that page in pypdf. This is not a full source-content or structure oracle.

`scripts/verify_mqtt_render.py` deliberately **fails**: three source URI fragments
containing U+2013 become mojibake, despite both actual/expected sets containing
434 targets. Expected suffixes are `#_CONNECT_–_Connection`,
`#_CONNACK_–_Connect`, `#_Figure_4.3_–`; actual bytes decode as `â•ﬁ` in place of
`–`. The verifier preserves and reports both sets, and must not be relaxed to
make the corruption pass. All remaining source diagnostics persist.

Native pages 1/24/130 were visually inspected. Unequal source widths currently
normalize to equal columns, splitting the page-130 description into narrow
fragments. Source heading/list numbering differs and cached PAGE 6/7 repeats.
These are open required layout/numbering/source-interpretation issues; 207 versus
137 publisher pages is not a Word-fidelity claim. The PDF subset tests establish
embedding correctness, not complete original-document acceptance.

Next fix URI serialization using primary PDF/URI encoding evidence and explicit
Unicode target tests; retain this source oracle. Then add source table widths
and numbering formats, qualify further body/link/asset recovery, and reduce the
large PDF→SF provenance/codec overhead under the original full acceptance goal.

```sh
bin/manydoc extract -format sf -o output/font-subset/mqtt.sf testdata/real/mqtt-v5.0-os.docx
# Explicit font mappings used for the current native render:
bin/manydoc convert -allow-lossy -pdf-font Han=output/font-research/BabelStoneHan.ttf -pdf-font Sans=output/font-subset/dejavu/DejaVuSans.ttf -pdf-font Sans/B=output/font-subset/dejavu/DejaVuSans-Bold.ttf -pdf-font Sans/I=output/font-subset/dejavu/DejaVuSans-Oblique.ttf -pdf-font Sans/BI=output/font-subset/dejavu/DejaVuSans-BoldOblique.ttf -pdf-font-map 'Arial,MingLiU-ExtB=Han' -pdf-font-map Arial=Sans -pdf-font-map 'Arial,Consolas=Go Mono' -pdf-font-map 'Arial,Verdana=Sans' -pdf-font-map Calibri=Sans -pdf-font-map 'Courier New=Go Mono' -pdf-font-map 'Arial Unicode MS=Sans' -pdf-font-map 'Arial,Arial Unicode MS=Sans' -o output/font-subset/mqtt.pdf output/font-subset/mqtt.sf
output/qa-venv/bin/python scripts/verify_mqtt_render.py  # known three-target failure
rg -n 'LinkString|URI' pdf_text.go pdf_links.go
```
