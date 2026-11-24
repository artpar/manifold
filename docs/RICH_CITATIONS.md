# Rich and mixed-direction citations

NoteReference.LabelContent is an optional nonempty literal Run sequence, mapped to
note_reference.attrs.label_content using established ProseMirror text nodes/marks.
It excludes Label; kind/ID owns navigation, so fragments cannot carry links,
destinations, anchors or nested notes. Each retains text/font/size/color/language/
direction/emphasis and source positions. Shared outer positive marks and missing
style values materialize into children in canonical SF, without changing caller
data. JSON retains the public model. Notes' own Label/body-label choices and
first-reference numbering order remain independent.

PDF recovery retains heterogeneous explicit citation fragments instead of their
common typography, including individual source regions. Physical alternate links
remain source navigation; editable note ownership replaces fragment links with an
explicit diagnostic. Uniform citations keep the existing representation.

Native PDF measures the normal text/font/shaping/bidi pipeline inside one atom.
Each fragment scales to .7 of its resolved size (rounded to .01pt); shared rise is
.3 of the maximum unscaled size. The internal default flow is LTR; fragment
explicit directions apply normally. One outer note link, preceding-grapheme binding
and reservation cover the whole mark. Plain mixed-direction literal labels also
use this box; homogeneous labels keep their existing shaping path. Ordinary missing
glyph/shaping/size/width checks apply. Rich label text is not flattened into a
single style or split across pages/lines.

Word exports actual reference-styled runs inside the checked body-target hyperlink.
Existing real notes/zero-width custom anchors and static automatic-label editing
acknowledgement remain. Literal label edits are authoritative, with no duplicated
text in private attributes. Checked rich citations currently require ordinary
literal text runs; revision wrappers/fields/nontext/hidden content there reject.
General document revision/comment/field policy is unchanged.

Independent LibreOffice rendering exposed split Arabic/Latin labels in RTL flow.
OOXML [w:dir embedding](https://learn.microsoft.com/en-us/dotnet/api/documentformat.openxml.wordprocessing.bidirectionalembedding?view=openxml-3.0.1)
did not fix that reader case. Standard [Unicode LRI/PDI isolation](https://www.w3.org/International/questions/qa-bidi-unicode-controls.en)
kept fragments adjacent. Rich and mixed-direction plain variant citations use
exact start/end control runs with checked md:noteIsolation roles. Imports remove
only those exact generated controls after validating namespaces, roles, boundaries
and literal content. Arbitrary user controls remain text. No override changes the
Arabic character order. This is independent reader evidence, not a guarantee for
every Word consumer.

The original synthetic source MCID1 paints a black 10pt asterisk and red 8pt digit;
pypdf independently decodes these operators. Source/SF/Word retains both fragments
and source regions; native paints the digit at5.6pt and assigns exactly one complete
reference link per mark. The note's list/table/raster remains intact. A sectioned
example preserves four multi-style citations (including list/repeat sites), three
rich body marks and correct native foot/endnote link targets. Arabic/Latin rich
and plain citations qualify LTR/RTL surrounding flow with supplied DejaVu. Actual
ProseMirror validates184 nodes/marks/attributes. Eight native/LibreOffice pages
were visually inspected for labels, text, images, tables, notes and safe bounds.

LibreOffice substitutes unavailable Go/DejaVu family fonts and uses its own
superscript size/baselines (source red digit4.6pt versus native5.6pt); mixed fragments
need not share the native baseline. The example still uses a dedicated Letter
endnote page versus native420pt square. Its PDF exporter omits custom-label links.
Package/SF meaning and native navigation are verified separately; no source-layout
or Word-equivalent print-fidelity claim is made. Broader nested/untagged note recovery, actual Colleaf adapter, broader original DOCX
corpus/producer qualification and final acceptance audit remain required.

Reproduce (optional Python/Node/LibreOffice are QA only):

```sh
output/qa-venv/bin/python scripts/generate_pdf_semantic_audit.py
GOWORK=off CGO_ENABLED=0 go build -o bin/manydoc ./cmd/manydoc
bin/manydoc extract -format sf -o output/rich-citations/source.sf output/audit/rich-citation-note.pdf
bin/manydoc convert -allow-lossy -pdf-font-map 'Courier=Go Mono' -o output/rich-citations/native.pdf output/rich-citations/source.sf
bin/manydoc convert -allow-lossy -o output/rich-citations/word.docx output/rich-citations/source.sf
bin/manydoc extract -format sf -o output/rich-citations/word.sf output/rich-citations/word.docx
GOWORK=off go run ./examples/notes -rich-citations -rich-body-labels -sectioned -out output/rich-citations/example
bin/manydoc extract -format sf -o output/rich-citations/example/word.sf output/rich-citations/example/notes.docx
bin/manydoc convert -pdf-font DejaVu=testdata/assets/DejaVuSans.ttf -o output/rich-citations/mixed.pdf output/audit/mixed-citations.sf
bin/manydoc convert -allow-lossy -o output/rich-citations/mixed.docx output/audit/mixed-citations.sf
bin/manydoc extract -format sf -o output/rich-citations/mixed-word.sf output/rich-citations/mixed.docx
/Applications/LibreOffice.app/Contents/MacOS/soffice -env:UserInstallation=file:///tmp/manydoc-rich-citation-qa --headless --convert-to pdf --outdir output/rich-citations/lo output/rich-citations/word.docx output/rich-citations/mixed.docx
/Applications/LibreOffice.app/Contents/MacOS/soffice -env:UserInstallation=file:///tmp/manydoc-rich-citation-example-qa --headless --convert-to pdf --outdir output/rich-citations/example/lo output/rich-citations/example/notes.docx
output/qa-venv/bin/python scripts/verify_rich_citations.py
NODE_PATH=output/sf-qa/node_modules node scripts/verify_structured.cjs output/rich-citations/source.sf output/rich-citations/word.sf output/rich-citations/example/notes.sf output/rich-citations/example/word.sf output/audit/mixed-citations.sf output/rich-citations/mixed-word.sf
GOWORK=off go test -race -run 'TestRichCitation|TestDOCXCitationIsolation|TestPDFRichCitation' -count=1 .
```

Full checks/results live in verification.json. Dependencies/THIRD_PARTY remain
unchanged; runtime stays entirely inside Go with no CGO, required executables,
services, uploads or app-level authorization/quota handling.
