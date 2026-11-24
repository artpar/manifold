# Literal list-marker content

PDF L/LI/Lbl structure can be explicit even when its label is not a representable
automatic counter. Flattening such a list into paragraphs loses a recoverable
container. Literal labels now have an independent model/SF owner.

## Model and SF

ListItem.Marker is an optional *ListMarker with Blocks and Source. Nil selects
the enclosing list’s automatic counter/bullet. A present marker owns visible label
content independently of item body blocks; an empty paragraph represents an empty
literal label. Its source owner can retain the actual Lbl identity, parent, regions
and style-bearing content, instead of overloading historical SourceInfo.Label.

The established manydoc-prosemirror/1 profile adds a non-group list_marker node
with block+ content and optional source. list_item accepts that node only before
its required paragraph/heading and other body blocks. Existing documents with no
marker retain their wire meaning. Older readers must reject the unfamiliar node
rather than flatten it. This is an additive profile capability, not a second
canonical format or cached SF body inside another format.

Markers can retain paragraphs, links, notes, tables, nested lists and images.
Page/section controls and standalone anchor blocks have no defined parallel
label/body pagination semantics and reject; inline/paragraph anchors participate
in the normal destination graph. Note references in markers participate in the
normal identity graph and note-body/running-story restrictions. Empty marker
containers reject instead of serializing away through omitempty.

JSON/SF/gzip persistence preserves the complete marker/body graph. Text includes
literal marker text/alt content before the item body. Source/fidelity admission,
PDF source-region collection, link diagnostics and navigation binding include
markers. Arbitrary automatic counters are not inferred from their current labels.

## PDF recovery

Canonical counter and existing constant-template recovery retain precedence. If
those cannot represent a regular explicit list, each original label becomes its
own literal marker and the original body paragraphs remain separate. L/LI/Lbl
ownership, styled runs, URI/internal links, note identities and image bytes remain
in the shared representation. SourceInfo.Label remains original display evidence.

Known explicit ordered families keep an ordered editing container, with the unused
start unobserved. Unknown/None/absent families use an unordered container with
pdf_list_kind_unobserved; this does not invent an automatic bullet. Every item has
an explicit literal marker, including an empty one. pdf_list_literal discloses
that no automatic counter/restart rule was reconstructed. Multiple Lbl children
retain their source-bearing content without inventing one original source owner.

## PDF generation

PDF supports literal labels in body lists, nested table cells, note bodies and
running stories. A layout-only two-column table plan reuses the existing measured
flow, continuation and note-admission machinery. It never creates a logical Table
for the list. Actual content owns standard L/LI/Lbl/LBody nodes; rich marker children
own their own paragraph/heading/table/list/figure/reference nodes and real MCIDs.
Real tables nested in labels keep their borders and table structure. Running
stories retain artifact paint behavior. Empty labels own no phantom painted text.

The deterministic label column is min(120pt, 25% of available width), with the
remaining width assigned to the body. Both columns have 6pt internal padding and
wrap/continue independently. RTL item bodies put the label column on the right.
This is a reflow policy, not recovery of source indents or a Word layout claim.
Body-list defaults use 11pt/1.4; cell/note/running contexts inherit their compact
flow style. Headings retain their structural level and bold flow presentation.
Images use existing aspect-preserving fitting; text/fonts/shaping use the same
checked pipeline as ordinary flow. Missing glyphs fail atomically.

Explicit block-bearing Lbl children stay literal on recovery, even when their
current text looks like 1. or a bullet. Direct inline/counter labels still use
canonical/constant recovery. No private cached SF or hidden marker text is used.
Marker blocks participate in immutable numbering resolution and minimum-body/
running-image fitting as well as painting. Existing source fidelity diagnostics
still require AllowLossy for reflow; this never authorizes missing content.

## Exact text across wraps

The narrow RTL/nested-list test exposed an existing recovery ambiguity: CHILD
painted as CHIL / D on separate baselines became CHIL D. Standard structural
ActualText now belongs to each contiguous ordinary Span group, with references
separate. It is an inline replacement for its actual MCID-bearing content, not
an SF payload or detached document cache. Group boundaries retain shaping across
paint/style changes; a trial per-run owner split duplicated a cross-style glyph
cluster in the unchanged FuzzPDFPaintContext seed, so it was corrected without
changing the seed/oracle. Linear builders accumulate the replacement text.

Exact character alignment retains original styles/links and removes only synthetic
geometry gaps. A separate strict alignment path can restore missing Unicode
whitespace from ActualText: every painted character must still match in order;
no non-whitespace substitution or painted-whitespace deletion is accepted by that
path. Restored whitespace has no physical quad, records adjacent_style_inferred,
and emits pdf_actualtext_whitespace because its style has no painted evidence.
The existing diagnosed replacement policy still handles unrelated ActualText.

See [PDF Association reuse rules, section 4.3.6.3](https://pdfa.org/wp-content/uploads/2019/06/Deriving_HTML_from_PDF.pdf)
and [PDF 2.0 errata, clause 14](https://pdf-issues.pdfa.org/32000-2-2020/clause14.html).
Independent QA matches CHILD's structure replacement against its actual two MCID
fragments CHIL and D, rather than accepting a detached logical text value. Tests
also cover mixed-style/link wrapping and rejection of non-whitespace mismatches.

## DOCX generation and recovery

DOCX now writes rich literal label/body cells as ordinary borderless two-column
OOXML tables inside the existing checked list/item bookmark ranges. Small ignorable
attributes record literal ownership, the standard numbering ID/level and marker
side. Text, marks, links, drawings and notes live only in actual Word content.
The reader checks the one-row/two-cell shape, nil borders, numbering, marker side,
body start and permitted label content before removing the layout container.
Unexpected visible features reject; edits to actual label text remain authoritative.
The ordinary table writer/reader owns nested tables, assets and relationships.

Literal items still consume an item index. A following automatic run uses a real
OOXML startOverride so mixed lists display e.g. Item 7), [done], Item 9). Recovery
compares every counter definition/level, resumed start against item position, and
all used numbering IDs across explicit ranges. Automatic items remain automatic;
no literal text cache substitutes for a Word counter.

Word ignored pageBreakBefore within the layout cell in the first experiment.
A checked empty 1pt paragraph now carries an item page start before the table;
conditional, explicit-before-list and combined intent restore separately. Content
in that control rejects. Actual Word renders prove both requested two-page starts.

LibreOffice omitted a real table nested inside the extra footnote layout cell.
Foot/endnote literal items therefore use sequential ordinary label/body flow,
with mdmarker1_ID_LEVEL_BOOKMARK checked label ranges rather than an extra table.
This reflow policy retains the actual rich label table, image, nested list and body;
it is not an unsupported-label exception. Reserved bookmark names remain separate
from editable anchors. Note identity resolution includes marker references. Running
story fitting/minimum-body measurement also includes parallel label flows.

Ten targeted authored cases retain exact normalized SF graphs, including assets,
relationships, note IDs, empty markers, counter intent, RTL and page settings.
Optional independent OOXML/LibreOffice/pypdf checks cover 17 rendered pages;
all were inspected after Poppler rendering. Actual ProseMirror validates 324 nodes.
These are scoped structural/content/asset/visual tests, not Word-equivalent fidelity.

One independent-reader limitation remains explicit: LibreOffice retains both URI
and internal label links in its actual ODT save, but its PDF export omits those two
annotations while retaining note links. Adding w:history in an isolated probe did
not change that result. The strict --require-pdf-links oracle remains reproducible
and fails; the main checker separately requires the actual ODT link graph. The
native Go PDF generated from the same DOCX retains both annotations. No claim is
made that the Word-rendered PDF passes the missing-annotation oracle. LibreOffice
[documents automatic hyperlink export](https://help.libreoffice.org/latest/en-GB/text/shared/01/ref_pdf_export_links.html),
so the observed discrepancy stays in the reader-qualification queue; direct
DOCX-to-PDF fidelity is outside the original goal. Original label/text/asset oracles
and core all-four-path requirements are unchanged.

```sh
MANYDOC_LITERAL_DOCX_QA=output/literal-docx GOWORK=off go test -run TestDOCXLiteralMarker -count=1 .
mkdir -p output/literal-docx/word output/literal-docx/odt
/Applications/LibreOffice.app/Contents/MacOS/soffice -env:UserInstallation=file:///tmp/manydoc-literal-docx-qa --headless --convert-to pdf --outdir output/literal-docx/word output/literal-docx/{rich,mixed-0,mixed-7,nested,running,continued,page-start,explicit-break,rtl,endnote}.docx
/Applications/LibreOffice.app/Contents/MacOS/soffice -env:UserInstallation=file:///tmp/manydoc-literal-docx-qa --headless --convert-to odt --outdir output/literal-docx/odt output/literal-docx/rich.docx
output/qa-venv/bin/python scripts/verify_docx_literal_markers.py
# Retained additional Word-PDF annotation oracle currently fails:
output/qa-venv/bin/python scripts/verify_docx_literal_markers.py --require-pdf-links
NODE_PATH=output/sf-qa/node_modules node scripts/verify_structured.cjs output/literal-docx/{rich,mixed-0,mixed-7,nested,running,continued,page-start,explicit-break,rtl,endnote}.sf
```

## Required unfinished mapping

VDOCS still rejects literal markers with ErrUnsupported before writing output,
even with AllowLossy. The native mapping remains required work, not a completion
exemption. An internal checked native table/flow prototype now preserves its rich
model/source graph through SDK edits and facade saves. Actual browser red-image
painting still fails, including a standalone control, so publication remains
unfinished. See NATIVE_LITERAL_MARKERS.md for the precise oracle and next commands.
Word's ordinary bullet glyph path truncated multi-character labels in
the earlier experiment and is not a generic encoding. Keep actual editable content
and assets authoritative in the checked native mapping as in DOCX/PDF.

## Evidence and commands

Existing annotation/pictorial/custom-label fixtures retain their original source
bytes, literal text/body boundaries, URI and red image pixel. Their logical tests
now additionally require explicit list/marker/item ownership instead of the old
flattened stage. The original iv. First / Second paragraph text oracle is unchanged.

New tests verify exact JSON/SF/gzip graphs, note/destination admission, source-only
markers, malformed marker placement/payload, atomic unfinished-output rejection,
and variable None/UpperAlpha source labels. Independent pypdf compares actual
original L/LI/Lbl references, source bounds, marker/body correspondence, URI, raster
pixels and image dimensions/placement. Actual ProseMirror validates all new nodes.

```sh
MANYDOC_LITERAL_MARKER_QA=output/literal-markers GOWORK=off go test -run 'TestLiteralMarker|TestPDFVariableLiteral|TestPDFCounter|TestPDFPictorial|TestPDFCustomListLabel' -count=1 .
output/qa-venv/bin/python scripts/verify_literal_markers.py
NODE_PATH=output/sf-qa/node_modules node scripts/verify_structured.cjs output/literal-markers/*.sf
GOWORK=off go test -run '^$' -fuzz '^FuzzLiteralMarkerSF$' -fuzztime=10s -parallel=2 .
```

PDF output tests additionally verify label/body/source hierarchy, Unicode styles,
URI/note/internal destination graphs, four-page independent column continuation,
nested image/table labels in body and footnotes, repeated running artifacts,
explicit empty/numeric/bullet labels and atomic missing-glyph failure. Optional
pypdf checks real roles/refs, pixels, destinations and path operators; the layout
columns emit no grid strokes. Actual ProseMirror checks 225 recovered nodes across
five cases (nine pages). Poppler renders were inspected for rich, continued, nested and running
cases: legible parallel flows, real table borders only, preserved red figures and
proper note placement. This is targeted synthetic evidence, not universal fidelity.
The authored examples/literal-markers/document.sf and README provide CLI use.

```sh
MANYDOC_LITERAL_OUTPUT_QA=output/literal-output GOWORK=off go test -run 'TestPDFLiteralMarker' -count=1 .
output/qa-venv/bin/python scripts/verify_literal_marker_output.py
NODE_PATH=output/sf-qa/node_modules node scripts/verify_structured.cjs output/literal-output/{rich,continued,nested,running,narrow-rtl}.sf
pdftoppm -scale-to 1100 -png output/literal-output/continued.pdf output/literal-output/continued
```

The native repeated-footnote ordering defect remains separate and unresolved.
See COLLEAF_BROWSER.md. The complete goal stays active; native generation and remaining
semantic/source/resource/visual acceptance work are not closed by this milestone.
