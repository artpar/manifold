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

## Required unfinished generation work

This milestone implements extraction and persistence. DOCX, PDF and VDOCS writers
currently reject documents containing literal markers with ErrUnsupported before
writing any output, even with AllowLossy. This deliberate transitional guard
prevents a new field from being silently dropped. It is **not** an exemption from
the original all-four-path goal or a completed literal-marker conversion feature.

Next implement PDF label/body planning and actual Lbl paint ownership through the
existing flow/table/cell planner, including links/images/notes and pagination. Then
implement DOCX generation/recovery against an independently rendered marker/body
representation, followed by the checked native mapping. Keep the same source and
SF graph oracles. Word’s ordinary bullet glyph path truncated multi-character
labels in the preceding experiment, so it is not a valid generic marker encoding.
Do not replace marker assets/links with a text cache or claim output support from
successful parsing alone.

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

The native repeated-footnote ordering defect remains separate and unresolved.
See COLLEAF_BROWSER.md. The complete goal stays active; generation and remaining
semantic/source/resource/visual acceptance work are not closed by this milestone.
