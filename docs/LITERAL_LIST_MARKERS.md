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

## Required unfinished mappings

DOCX and VDOCS writers still reject literal markers with ErrUnsupported before
writing output, even with AllowLossy. Those mappings remain required work, not a
completion exemption. Next implement DOCX generation/recovery against an
independently rendered marker/body representation, then the checked native mapping.
Keep the source/SF/asset oracles. Word's ordinary bullet glyph path truncated
multi-character labels in the preceding experiment and is not a generic encoding.
Do not replace assets/links with a text cache or claim output support from parsing.

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
See COLLEAF_BROWSER.md. The complete goal stays active; DOCX/native generation and remaining
semantic/source/resource/visual acceptance work are not closed by this milestone.
