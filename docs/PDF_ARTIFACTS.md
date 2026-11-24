# Explicit PDF artifact properties and content membership

## Physical structure, not an invented authoring template

PDFPage.Artifacts records explicit Artifact marked-content groups. Each PDFArtifact
has a page-scoped stable ID, content stream, optional Type/Subtype, original BBox,
its display transform/bounds and Attached edges. SourceInfo.Artifacts links recovered
text/images and contributing aggregate bounds to these groups. Nested marked content
can therefore keep an inner Span/MCID and its outer Header/Footer artifact identity.
Empty or vector-only groups remain recorded even without an editable raster/text unit.

The same fields persist through legacy JSON, SF and gzip SF; the ProseMirror source
mark accepts artifacts. No second copy of visible content is introduced. The content
stays in its recovered logical/paint flow with source membership. Header/Footer here
is an explicit physical page role. It does not establish a reusable authorial Word
header template, its parity/section rules or its behavior under changed pagination.
Document.Headers/Footers remain that separate authoring model; no single observed
page is silently promoted into a repeated template.

This closes the audit's concrete lost Type/Subtype/BBox/Attached and membership
facts. It does not complete explicit Note/Reference recovery, native Colleaf
interoperability or the full original acceptance criteria.

## Published semantics and coordinates

Primary basis: ISO 32000-1:2008 §14.8.2.2.2/Table 330,
[Adobe's published specification](https://opensource.adobe.com/dc-acrobat-sdk-docs/pdfstandards/PDF32000_2008.pdf),
and [W3C PDF14 Example 4](https://www.w3.org/TR/2016/NOTE-WCAG20-TECHS-20160317/PDF14.html).
Artifact BBox coordinates are **default user space**. The page crop/rotation/UserUnit
transform maps them to displayed points; a content cm must not be applied again.
Actual glyph/image source geometry still uses its full graphics-state transform.
Attached retains source logical page-edge names; it is not rewritten into guessed
rendered-margin rules. Type/Subtype names retain their exact source values.

Both generic Artifact BMC and property-bearing BDC are retained. Named Properties
resources, nested groups, inherited Form context and repeated Form calls work.
An outer artifact applies to inner marked glyphs and images until its EMC. No context
leaks into a later unmarked Form call. Whole-span ActualText unions membership from
all contributors, as do exactly aligned mixed-style fragments and structural
aggregate bounds. Synthetic separators claim only groups shared by both neighbors.

Validation checks unique IDs, defined same-page source membership, finite/coherent
BBox/display geometry, page-coordinate matrices, Attached edge names and a 100,000
artifact document budget. Additional unrepresented property keys emit a diagnostic.
The parser retains publisher-declared bounds as evidence, not verified glyph-ink
extents. SF bridges deep-copy page artifact records and pointer/slice properties;
editing returned SF cannot mutate caller data.

## Independent and regression evidence

Original source bytes and old structure/text/merge/image/font/navigation oracles
remain unchanged. New `scripts/verify_pdf_artifacts.py` parses original operators
through pypdf and compares every recovered Type/Subtype/BBox/Attached, page/stream
ID, default-space transform and membership reference.

- Original MQTT PDF: 1,530 groups across 137 pages, including 127 Header and
  137 Footer groups. The other 1,266 groups have no subtype and no editable
  text/image membership (for example, vector-only artifacts). 15,324 source
  memberships survive in the structured result, including aggregate provenance.
- Original InDesign publication: 25 generic groups and 160 source memberships.
- Targeted Header probe: one property-bearing group and correct nested content
  membership, while HEADER and BODY stay visible without duplication.
- ProseMirror checks all 214,795 nodes/marks/attributes across the probe and both
  original imported files. SF/JSON/gzip tests preserve artifact graphs exactly.
- Whole real-source JSON projected before the additive artifacts fields has the
  original normalized SHA-256
  `4b9ccc6f060fc45958494e3cfdf85ed3bad4d54076a97e79a153662e7d1d1d1b`.
  All preexisting values remain exact; no source oracle was changed to accept loss.

Tests additionally cover crop/rotation/UserUnit with a content cm, default-space
BBox, properties resources/Form inheritance, nested and aligned/whole ActualText,
separator ownership, invalid properties, broken membership and bridge isolation.
Rendering code did not change; this is source-structure/codec evidence rather than
new visual or Word-pagination fidelity evidence.

## Reproduce and continue

```sh
output/qa-venv/bin/python scripts/generate_pdf_semantic_audit.py
bin/manydoc extract -format sf -o output/artifacts/header.sf output/audit/pagination-artifact.pdf
bin/manydoc extract -format sf.gz -o output/artifacts/mqtt.sf.gz testdata/real/mqtt-v5.0-os.pdf
bin/manydoc extract -format sf -o output/artifacts/indesign.sf testdata/real/copyright-office-circular-1-2021.pdf
output/qa-venv/bin/python scripts/verify_pdf_artifacts.py testdata/real/mqtt-v5.0-os.pdf output/artifacts/mqtt.sf.gz --projected-sha256 4b9ccc6f060fc45958494e3cfdf85ed3bad4d54076a97e79a153662e7d1d1d1b
NODE_PATH=output/sf-qa/node_modules node --max-old-space-size=8192 scripts/verify_structured.cjs output/artifacts/header.sf output/artifacts/mqtt.sf.gz output/artifacts/indesign.sf
GOWORK=off go test -run 'TestPDF.*Artifact' -count=1 .
```

Generated sources, artifacts and logs remain ignored under output/artifacts and
output/audit. Next recover explicit PDF note/reference relationships and rich bodies,
with source labels and placement/kind uncertainty preserved. Then continue the
native snapshot adapter and final corpus/visual/requirement audit in ACCEPTANCE.md.

Checks passed: full tests (18.860s/0.824s), race (173.268s/1.864s), focused new
seed/artifact race (1.912s), vet/module verification, host CGO-disabled build and
focused execution (0.357s), original PDF extraction, Linux amd64/arm64 CGO-disabled
cross-builds and diff whitespace check. Ten-second PDF/SF fuzz campaigns passed
4,738/273 executions with two workers each. The full objective remains incomplete.
