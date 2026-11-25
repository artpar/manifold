# Original rich DOCX export qualification

Eight unchanged upstream DOCX files extend the original OASIS Word publication
with small, actual editor exports. They are not generated manydoc round trips.
They come from Apache POI's test-data/document at the pinned release commit
[44d254e441b983d1338e1b4d2248023b807ce55f](https://github.com/apache/poi/tree/44d254e441b983d1338e1b4d2248023b807ce55f/test-data/document).
Source URLs, hashes, byte counts and rights are in testdata/real/manifest.json.
The complete pinned upstream [LICENSE](https://github.com/apache/poi/blob/44d254e441b983d1338e1b4d2248023b807ce55f/legal/LICENSE)
and [NOTICE](https://github.com/apache/poi/blob/44d254e441b983d1338e1b4d2248023b807ce55f/legal/NOTICE)
are retained under third_party/licenses/apache-poi-fixtures-*.txt. Original author/
producer metadata stays embedded. No Apache POI implementation code was used.
These are reference assets under their upstream terms, not manydoc MIT code.

| Original file (poi- prefix locally) | Embedded editor | Source features independently qualified |
| --- | --- | --- |
| footnotes.docx | Microsoft Office Word12 | One actual reference site and independently owned footnote text/style |
| endnotes.docx | Microsoft Office Word12 | One mid-paragraph endnote relationship, Unicode body, note text/style |
| headerFooter.docx | Microsoft Macintosh Word12 | Default/first/even story relationships, explicit empty variants, page settings |
| DiffFirstPageHeadFoot.docx | Microsoft Office Word12 | First/default story selection on two pages, heading, positional tabs |
| HeaderFooterUnicode.docx | Microsoft Office Word12 | Two-page Unicode running text, currency/French text, bold24pt heading and italic text |
| FieldCodes.docx | Microsoft Office Word14 | Two complex AUTHOR/DATE fields, actual cached results and anchor |
| delins.docx | Microsoft Office Word12 |32 insertion/4 deletion containers, lists/styles, seven explicit URI relationships and11 complex HYPERLINK fields |
| comment.docx | LibreOffice6.0.7.3/Linux | Empty visible body with two comment text paragraphs, diagnosed annotation loss |

## Independent source expectations

scripts/qa/docx/audit_poi_sources.py reads only original ZIP/XML. The committed oracle
poi-docx-oracle.json captures50 nonempty body paragraphs in order, exact inline
reference sites/kind/ID,81 direct formatting intervals, two note definitions,
16 explicit story bindings including empty variants, exact section dimensions/
margins/first-page activation, seven URI targets/text,13 field instructions,
revision counts, deleted text and comment text. It never reads manydoc output.
The original source hashes are checked both against the oracle and real manifest.
Style intervals cover direct supported attributes; they are not a second complete
Office style/theme resolver. Existing original MQTT style/table/asset tests remain.

The Go test verifies text and actual reference positions, individual marks,
relationships, notes and conditional stories/geometry. SF, gzip SF, model JSON and
DOCX all preserve the same normalized SF semantics (DOCX source diagnostics are
acknowledged, then omitted from comparison). Revision history, comments and field
execution have explicit diagnostics; the visible view retains inserted/cached text
and excludes deleted history. This is not a claim of fully accepting/rejecting
Office review history or reconstructing comment editing. Deletion-only list
paragraphs retain empty item structure in the current final-text projection.

The independent producer renders the original delins with visible review markup;
its deleted struck text is intentionally absent from the library's final-text
view. These different views must not be used as pixel-equivalent oracles. Complex
HYPERLINK fields currently keep their cached text/formatting with the generic field
loss diagnostic; the seven standard w:hyperlink relationships retain targets.
No claim is made that those11 field targets become editable links.

## A source-discovered separator bug

Independent visual comparison exposed silently omitted w:ptab controls in the
first/default-story example. Original page two has separate left/center/right
labels; the initial projection joined their words. The source oracle was tightened
to require the actual separators, reproducing the failure before the fix.

The importer now preserves each positional-tab separator as a normal tab and emits
position_tab for alignment/base/leader presentation loss. Standard DOCX tab output
and native measured tab advance keep visible separation. The exact source remains
unchanged; neither oracle nor source was weakened to accept concatenated words.
Malformed nonempty ptab content rejects. This normalization does not preserve Word's
margin-relative centered/right placement or leader dots. It is an acknowledged
presentation loss, not a Word-compatible layout claim. The published
[OOXML positional tab](https://learn.microsoft.com/en-us/dotnet/api/documentformat.openxml.wordprocessing.positionaltab?view=openxml-3.0.1)
is a separate control from an ordinary tab.

## Independent output and visual evidence

All four CLI paths execute on these sources, including native PDF→SF physical
page/geometry recovery. Python-docx/lxml read originals and generated Word packages;
exact SF comparison is separate from independent-reader checks. Actual ProseMirror
checks762 nodes/marks/attributes across the eight source SFs, eight Word SFs and
eight recovered native-PDF SFs. pypdf/MuPDF verify physical page dimensions, all
literal text segments, note bodies and actual native target pages, seven native URI
targets and Word package relationships. Both pages select the expected running
variants in the first/default and Unicode examples.

Thirty-three pages across original LibreOffice, supplied-font native output and
generated-Word LibreOffice output were visually inspected using eight comparison
sheets. Text/notes/headings/stories are readable with safe separation and no observed
clipping. The reviewed source shows editing markup while the converted final-text
view omits deleted history. Paragraph spacing, fonts, superscript placement, note
layout, tab positions and wrapping differ; no Word/source print equivalence is
claimed. The endnote source/LibreOffice uses an implicit Roman i; manydoc uses its
diagnosed OOXML decimal default and native Endnotes section. Comments are annotations
on an otherwise empty source, so all three document pages are intentionally blank.

LibreOffice's PDF exporter emits zero URI annotations for both original and
generated delins despite visible hyperlink styling. This observed reader behavior
is recorded separately; the actual Word relationship targets and seven native PDF
annotations pass. Do not describe that producer's PDF as navigation-preserving.

Go-only mappings reject the real ṭ (U+1E6D) absent from Go's font before any bytes
reach the output. QA explicitly supplies all four original DejaVu Sans2.37 faces
from the previously pinned release; no fake bold/italic faces or silent fallback.
Production remains entirely in-process Go. Optional QA Python/Node/LibreOffice
and font downloads are never invoked by the library or CLI.

## Reproduction and remaining work

The source oracles can be independently inspected/regenerated; never regenerate
expected output merely to make a failing test pass. The full DejaVu release ZIP
hash/provenance is in THIRD_PARTY.md and FONT_SUBSETTING.md; place its real four
faces at output/font-subset/dejavu/ for the QA driver.

```sh
output/qa-venv/bin/python scripts/qa/docx/audit_poi_sources.py
GOWORK=off CGO_ENABLED=0 go build -o bin/manifold ./cmd/manifold
output/qa-venv/bin/python scripts/qa/docx/generate_poi_qa.py
/Applications/LibreOffice.app/Contents/MacOS/soffice -env:UserInstallation=file:///tmp/manydoc-poi-original-qa --headless --convert-to pdf --outdir output/real-docx-audit/original-render output/real-docx-audit/*.docx
/Applications/LibreOffice.app/Contents/MacOS/soffice -env:UserInstallation=file:///tmp/manydoc-poi-generated-qa --headless --convert-to pdf --outdir output/real-docx-audit/word-render output/real-docx-audit/generated/*.docx
output/qa-venv/bin/python scripts/qa/docx/verify_poi_corpus.py
NODE_PATH=output/sf-qa/node_modules node scripts/qa/structured/verify_structured.cjs output/real-docx-audit/*.sf output/real-docx-audit/generated/*.sf
GOWORK=off go test -race -run 'TestRealPOI|TestDOCXPositionalTab' -count=1 ./internal/engine
```

Exact commands/results are recorded in verification.json. Original images/merged
cells/full tables remain qualified by MQTT and the fixed rich targeted corpus;
these eight small files specifically broaden note/story/review/field/producer
coverage. Remaining goal work includes broader note graph recovery, the actual
VDOCS native adapter and final consistency/resource/public-API/acceptance audit.
There is no external blocker, and the full goal remains active/incomplete.
