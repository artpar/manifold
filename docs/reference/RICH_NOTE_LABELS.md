# Rich body-label content

## Shared representation

Note.BodyLabelContent is an optional literal sequence of ordinary Go Runs. SF uses
attrs.notes[].body_label_content, an array of established ProseMirror text nodes
with ordinary formatting/link/source marks. There is no new text node or alternate
canonical document format. The sequence owns its text, independently of citation
Label/default numbering; BodyLabel and BodyLabelStyle must both be absent. Use the
existing literal override for markerless labels. Empty arrays/runs, nested note or
anchor atoms and multiline/blank complete marks reject. Valid per-fragment source
geometry remains available, with LabelSource retaining aggregate provenance.

BodyLabelStyle still applies one text-free template to an inherited or overridden
mark, preserving automatic counter behavior. Rich content instead describes a
literal independently formatted mark. Its presence activates the existing variant
Word representation and automatic-editing acknowledgement policy. A rich body
mark does not rename the note or change citation identity, label or counter order.

PDF import retains all distinct leading-Lbl run styles instead of taking their
common intersection. Uniform labels keep the existing template representation.
Individual fonts/sizes/colors/languages/directions/emphasis and links, plus source
positions, survive SF/JSON/gzip. Native PDF uses the same font/glyph/shaping and
measured footnote/list-marker paths, including rich labels beside tables/images.
Marker bidi placement uses its own measured width, preventing a full paragraph's
right-alignment offset from escaping the label gutter. Gutter side follows the
first note-body paragraph; inline text uses the renderer's normal bidi defaults.
Arabic/Latin body marks with a supplied DejaVu font qualify both gutter sides.
Richly styled/internal mixed-direction citation atoms are qualified separately in
RICH_CITATIONS.md.

## Portable Word and checked role

The label remains ordinary visible Word runs/hyperlinks/rPr. A standard
[bookmark start](https://learn.microsoft.com/en-us/dotnet/api/documentformat.openxml.wordprocessing.bookmarkstart?view=openxml-3.0.1)
and [matching end](https://learn.microsoft.com/en-us/dotnet/api/documentformat.openxml.wordprocessing.bookmarkend?view=openxml-3.0.1)
delimit it inside one paragraph. Checked md:noteBodyContent roles and a
mdnoterich1_kind_wireID name bind that range to its actual note. No second label
string is stored in metadata. The existing checked tab/two-space layout separator
follows it. Independent Word edits to the actual label text are recovered; changed
gutter preferences are diagnosed rather than overriding the visible text.

The decoder checks expected note identity, marker namespace, matching IDs, range
membership and only ordinary text/link/style content. Fields, hidden properties,
nested controls/bookmarks, drawings, unbound/duplicate roles and unsupported inline
content reject. Ordinary inserted/moved-to text is retained; deleted/moved-from
history is excluded and diagnosed. Comment markers keep the existing annotation
loss diagnostic. An empty final-visible range becomes an explicit markerless body
override with a normalization diagnostic; it cannot invent inherited numbering.
Empty text spans have no visible authority and their editing styles/navigation
are not represented. User destinations beginning with the reserved wire prefix are
escaped and round trip independently; the range cannot steal a real destination.
Normal paragraphs avoid rich-range allocation. Both sectioned and ordinary writers
use the same checked representation and scoped note relationships.

An evaluated inline content-control wrapper failed independent LibreOffice QA: it
moved the mark after the body and flattened its styling. It is not emitted. The
ordinary bookmark-range representation preserves placement, 10pt black/8pt red
paint and rich per-fragment semantics in the independent reader. This is an output
fix driven by unchanged source operators, not a revised content/style oracle.

## Evidence and remaining scope

The hand-authored PDF paints one MCID2/Lbl as a black 10pt asterisk followed by a
red 8pt digit. pypdf independently reads those operators; SF preserves both
styles/source regions. Ordinary and sectioned SF/JSON/gzip/Word round trips pass.
Independent XML/python-docx, native and LibreOffice paint/geometry checks confirm
one body mark before the body, with its styles and safe separation. The note's
list/table and two exact red/blue image pixels remain present. A two-page sectioned
example retains three rich marks and native per-fragment URI/named backlinks.
Actual ProseMirror checks 138 nodes/marks/attributes across source/Word/example and
mixed-script SF. Eight generated pages (including two Arabic/Latin gutter cases)
were visually inspected. Tests include malformed roles, authority conflicts,
legitimate text edits, final-visible tracked edits/comments, empty/fully deleted
marks, private-prefix destination collisions and invalid SF nodes.

LibreOffice still chooses installed font substitutes and omits custom note-label
hyperlinks in PDF export. In the sectioned example its main page is approximately
419.556pt square and its dedicated endnote page is Letter; native pages are both
420pt square. These are measured producer differences, not proof of exact Word
page geometry or print equivalence. Standard source settings and semantic SF
round trips are separately verified; broader producer qualification remains open.
Broader nested/untagged note recovery, the Colleaf adapter and original DOCX corpus/final
audit remain required. The full goal is incomplete.

## Reproduction

```sh
output/qa-venv/bin/python scripts/qa/pdf/generate_pdf_semantic_audit.py
GOWORK=off CGO_ENABLED=0 go build -o bin/manifold ./cmd/manifold
bin/manifold extract -format sf -o output/rich-note-labels/source.sf output/audit/rich-label-note.pdf
bin/manifold convert -allow-lossy -pdf-font-map 'Courier=Go Mono' -o output/rich-note-labels/native.pdf output/rich-note-labels/source.sf
bin/manifold convert -allow-lossy -o output/rich-note-labels/word.docx output/rich-note-labels/source.sf
bin/manifold extract -format sf -o output/rich-note-labels/word.sf output/rich-note-labels/word.docx
GOWORK=off go run ./examples/notes -rich-body-labels -sectioned -out output/rich-note-labels/example
bin/manifold extract -format sf -o output/rich-note-labels/example/word.sf output/rich-note-labels/example/notes.docx
/Applications/LibreOffice.app/Contents/MacOS/soffice -env:UserInstallation=file:///tmp/manydoc-rich-label-source-lo --headless --convert-to pdf --outdir output/rich-note-labels/lo output/rich-note-labels/word.docx
/Applications/LibreOffice.app/Contents/MacOS/soffice -env:UserInstallation=file:///tmp/manydoc-rich-label-example-lo --headless --convert-to pdf --outdir output/rich-note-labels/example/lo output/rich-note-labels/example/notes.docx
bin/manifold convert -pdf-font DejaVu=testdata/assets/DejaVuSans.ttf -o output/rich-note-labels/mixed.pdf output/audit/mixed-note-labels.sf
output/qa-venv/bin/python scripts/qa/layout/verify_rich_note_labels.py
NODE_PATH=output/sf-qa/node_modules node scripts/qa/structured/verify_structured.cjs output/rich-note-labels/source.sf output/rich-note-labels/word.sf output/rich-note-labels/example/notes.sf output/rich-note-labels/example/word.sf output/audit/mixed-note-labels.sf
GOWORK=off go test -race -run 'TestRichBodyLabel|TestPDFRichBodyLabel' -count=1 ./internal/engine
```

Native runtime remains in-process Go. Optional Node/Python/LibreOffice are QA only;
no new production module, service, executable, CGO, upload or application gate.
