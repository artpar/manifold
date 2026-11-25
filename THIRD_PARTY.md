# Third party components

manifold's own code is MIT licensed. No UniOffice/UniPDF implementation code is
included. Third-party components remain under their respective licenses.

| Component | Version | Purpose | License |
| --- | --- | --- | --- |
| github.com/hhrutter/lzw | v1.0.0 | Bounded PDF LZW stream decoding with EarlyChange support (previously transitive) | BSD-3-Clause |
| github.com/pdfcpu/pdfcpu | v0.11.1 | PDF objects/streams/xrefs/encryption, raster extraction and core font metrics | Apache-2.0 |
| github.com/phpdave11/gofpdf | v1.4.3 | PDF primitives and embedded fonts | MIT |
| github.com/benoitkugler/textlayout | v0.3.2 | Adobe glyph names/encodings, TrueType layout and HarfBuzz glyph shaping | MIT |
| github.com/rivo/uniseg | v0.4.7 | Unicode 15 grapheme/line segmentation (not terminal-cell width) | MIT; Unicode data terms |
| golang.org/x/text | v0.30.0 | NFC normalization, font-name case folding, BCP47/script metadata and bidi properties | BSD-3-Clause; Unicode data terms |
| golang.org/x/image | v0.32.0 | Go/Go Mono fonts, font glyph inspection and TIFF/CCITT decoding | BSD-3-Clause; additional Go font notice |

The embedded Go fonts are copyright 2016 Bigelow & Holmes Inc. Their full
notice is in `third_party/licenses/Go-fonts.txt`. Direct and runtime transitive
dependency license texts are preserved under `third_party/licenses/`.
Dependency revisions/checksums are pinned in go.mod/go.sum. Unicode segmentation
data in uniseg is Unicode 15.0.0; its MIT notice and the Unicode provider
license are preserved as `uniseg.txt` and `Unicode-V3.txt`. x/text was already a
runtime transitive module and is now imported directly for normalization, font-name matching and language/script lookup.

## Test-only assets and tools

`testdata/assets/NotoSans-Regular.ttf` is redistributed under the SIL Open Font
License in `testdata/assets/NotoSans-LICENSE.txt`. It is used to generate PDF
fixtures independently and is not embedded into the manydoc binary.
Source: <https://github.com/notofonts/noto-fonts>.

`testdata/assets/w3c-dummy.pdf` and its corpus copy are a published W3C test
resource: <https://www.w3.org/WAI/ER/tests/xhtml/testfiles/resources/pdf/dummy.pdf>.
The remaining initial corpus document content is synthetic; the separately listed
real exports below retain their original notices. Fixture provenance, SHA-256 hashes,
and expectations are in `testdata/corpus/manifest.json`.

Python fixture/verification tools are optional development tools, with package
pins in `scripts/qa/requirements.txt`. They are not linked to or invoked by the Go
library or CLI.

`testdata/real/copyright-office-circular-1-2021.pdf` is the original published
U.S. Copyright Office Circular 1, downloaded from
<https://www.copyright.gov/circs/circ01.pdf>. Its manifest records hash, retrieval,
producer, source, structural expectations and rights references. The publication
text is a U.S. government work (17 USC 105). Original embedded font programs and
marks retain their respective rights/notices; they are kept inside the unmodified
reference PDF, not extracted or reused as manydoc output fonts. This reference
file is not licensed as manydoc implementation code.

The newly used textlayout HarfBuzz, Graphite and font packages have their own
MIT notices, preserved in `third_party/licenses/harfbuzz.txt`, `graphite.txt` and
`textlayout-fonts.txt`. No new module dependency is introduced. Graphite's compiled
support is not a claim of broad Graphite font/layout qualification.

`testdata/assets/DejaVuSans.ttf` is the unmodified DejaVu Sans 2.37 test/example
font from the [official 2.37 release](https://github.com/dejavu-fonts/dejavu-fonts/releases/tag/version_2_37).
Its license is `testdata/assets/DejaVu-LICENSE.txt`; it is not linked into the
library/CLI. TTF SHA-256: `7da195a74c55bef988d0d48f9508bd5d849425c1770dba5d7bfc6ce9ed848954`.
Release ZIP SHA-256: `7576310b219e04159d35ff61dd4a4ec4cdba4f35c00e002a136f00e96a908b0a`.
This is an original third-party asset, not a generated font or customer file.

The optional QA environment additionally pins uharfbuzz 0.57.0 to compare native
HarfBuzz 15.0.0 with actual Go-generated GIDs/positions. This is a development-only
reader/oracle, not a Go runtime dependency or required conversion executable.

`scripts/qa/layout/font_oracles.py` records SHA256 values of original Go programs from the
pinned x/image v0.32.0 byte constants. These QA facts qualify actual embedded
programs independently of PDF resource aliases; no new font asset is vendored.

internal/bidilevels/core.go and bracket.go adapt the Go Authors' BSD-licensed
x/text v0.30.0 reference bidi core; their original headers and adjacent LICENSE
are preserved. The internal README and testdata/manifest.json identify upstream
sources/hashes and changes (package name, linear isolate matcher). The wrapper
uses existing x/text property tables, not another new module. This is attributed
reusable open-source code, not independently authored algorithm code.

internal/bidilevels/data/BidiBrackets.txt and compressed original BidiTest/
BidiCharacterTest fixtures are Unicode 15.0.0 data with original notices retained.
Their canonical URLs, original hashes and sizes are pinned in the manifest; the
existing third_party/licenses/Unicode-V3.txt applies. Authored line scenarios retain
native FriBidi 1.0.16 and ICU 78.3 observations. Those native libraries are optional
QA tools only; neither native nor Go textprocessing/fribidi is a runtime dependency.

`testdata/rtl-shaping.json` contains authored Hebrew/Arabic/LTR vectors measured
with optional native HarfBuzz 15.0.0 / uharfbuzz 0.57.0 against the existing original
DejaVu Sans asset. `scripts/qa/layout/verify_bidi_output.py` independently compares actual
PDF groups with native HarfBuzz and C FriBidi; these are optional QA tools only.
The two FuzzPDFBidiRoundTrip inputs are exact synthetic invented-space regressions.
No additional font/module/service/CGO or proprietary code is introduced.

`testdata/assets/DejaVuSans-Bold.ttf` is the original DejaVu Sans Bold 2.37 from
the same pinned official release ZIP (SHA256 7576310b219e04159d35ff61dd4a4ec4cdba4f35c00e002a136f00e96a908b0a).
The existing testdata/assets/DejaVu-LICENSE.txt applies. TTF SHA256:
`e6476c1b80502924294eed40894c5b18e06c181444ca953e5334262df9c27724`.
It is test/example data, not embedded into the library/CLI. The authored
`testdata/rtl-item-context.json` records fourteen original-font native HarfBuzz
item-context measurements; optional scripts/qa/layout/verify_bidi_context.py checks actual
regular/bold PDF glyphs/positions. No proprietary implementation or new module.

`testdata/real/mqtt-v5.0-os.docx` and `.pdf` are the unmodified authoritative Word
source and paired published PDF of **MQTT Version 5.0**, OASIS Standard,
7 March 2019, edited by Andrew Banks, Ed Briggs, Ken Borgendale and Rahul Gupta.
Copyright © OASIS Open 2019. The publication's [Notices](https://docs.oasis-open.org/mqtt/mqtt/v5.0/os/mqtt-v5.0-os.html)
permit copying/furnishing the document with the copyright notice and Notices
included; both original files retain those sections in full. They are not MIT
licensed implementation code. Do not modify the originals or present generated
normalized QA conversions as an OASIS Standard. The manifest pins original bytes,
sources, rights and current conversion failures. The adjacent structural oracle
contains independently extracted counts, geometry and content hashes, not another
edition of the standard. Original embedded fonts/marks remain inside the originals
and are not reused as manydoc output fonts. No endorsement is implied. Optional
QA conversions stay ignored.

Optional font-subset verification now pins fontTools 4.68.0 (MIT) in
scripts/qa/requirements.txt. It is used only by independent development oracles,
not by the Go library or CLI. The glyph subsetter retains original embedded name
records/notices and honors the existing TrueType embedding/subsetting flags.

The ignored BabelStone Han 16.0.3 QA input is obtained from its
[primary publisher](https://www.babelstone.co.uk/Fonts/Han.html), under the original
Arphic Public License embedded in its name table. Source/derived font files are
not redistributed in this repository or linked into the binary. Caller-supplied
font programs remain subject to their own reuse/embedding terms. FONT_SUBSETTING.md
records hashes, checks and scope; no publisher/customer PDF font was extracted
or repurposed as a manydoc output font.

Eight unmodified original DOCX exports from Apache POI's test-data/document at
44d254e441b983d1338e1b4d2248023b807ce55f are retained as testdata/real/poi-*.docx.
They cover Word 2007/2010, Mac Word and LibreOffice 6 exports; they were not generated
by manydoc. The pinned upstream Apache-2.0 LICENSE and NOTICE are preserved as
third_party/licenses/apache-poi-fixtures-LICENSE.txt and
third_party/licenses/apache-poi-fixtures-NOTICE.txt, including upstream component
notices. Original embedded author/producer metadata remains inside the unchanged
files. They are reference fixtures, not MIT-licensed manydoc implementation code.
Manifest hashes/URLs and independently derived XML expectations are in
 testdata/real/manifest.json and poi-docx-oracle.json. No Apache POI implementation
or runtime dependency was copied; optional Python/LibreOffice readers are QA only.
See docs/REAL_DOCX_CORPUS.md for exact provenance, evidence and limits.
The source-only oracle/QA evidence for those exports is documented in
`docs/REAL_DOCX_CORPUS.md`; their embedded editor metadata identifies Word12/14,
Mac Word12 and LibreOffice6.0.7.3. Original XML is the expectation authority;
generated output and rendered comparison sheets remain ignored.

Pinned `@univerjs/core`, `@univerjs/docs` and `@univerjs/engine-render` 1.0.4
(Apache-2.0) are optional independently installed native-format QA readers under
ignored output/vdocs-qa. They are not production Go modules, vendored source or
runtime requirements. The private VDOCS stream implementation is independently
authored against published format declarations and executable reader APIs; no
Univer implementation code is incorporated or translated. Host-application
validators and source loading have been removed. The native reference JSON
fixture is independently authored test data. See VDOCS_FORMAT.md for
exact versions, source hashes, unit/token discrepancies, commands and remaining
semantic-adapter work.

The optional pinned native image QA additionally uses @univerjs/docs-drawing and
@univerjs/drawing1.0.4 (Apache-2.0). Published IDocImage/IImageData declarations and
actual drawing registry/scope APIs qualify embedded asset fields; no implementation
code is copied/translated or linked into manydoc. Their downloads/installations and
lockfile remain ignored QA material. The existing original OASIS MQTT reference's
JPEG is reused unchanged by image transport tests, with its original byte oracle.
Pillow is an optional independent raster reader only. VDOCS_FORMAT.md records
primary source/hash, commands, measured boxes/pixels and remaining qualification.

The optional native hyperlink QA uses @univerjs/docs-hyper-link1.0.4 and reads
@univerjs/docs-hyper-link-ui1.0.4 published source (Apache-2.0). Their pinned npm
tar URLs and source hashes are in VDOCS_FORMAT.md. The former is registered as
a real QA plugin; a hash-pinned unmodified click-handler/helper source region from
the latter runs in an optional Node VM with real models and a window.open stub.
Neither package/source is a production Go dependency or runtime requirement; no
implementation is copied/translated into Go. Downloads/installations/UI extraction
remain ignored. The independently authored resource mapping and synthetic fixture
are manydoc code/data. Source-handler evidence does not establish browser/UI or
full native navigation compatibility.

The optional navigation probe uses the same attributed pinned packages/source.
It executes the unmodified internal-link handler/helper region with the actual
DocSelectionManagerService and a stub render manager/window. It verifies selection
refresh requests and save/edit transport, without claiming rendered caret/scroll
or browser interaction. No additional package or production dependency was added.

The optional container-navigation probe reuses those same pinned packages,
validator and unmodified handler/selection manager. It qualifies table/image token
and source bindings, headless save/edit transport and selection-refresh requests;
no additional production dependency or browser fidelity claim is introduced.

The container-navigation probe reuses those same pinned QA packages and handler
source for table/image points, token/source bindings and selection refreshes.
No additional dependency or third-party production source is incorporated.

The flow-navigation probe reuses the same attributed pinned QA packages and
unmodified handler source for list/break/section save/edit/selection evidence.

The standalone-point probe reuses those same attributed pinned QA packages,
validator and unmodified handler for text-free point/group/save/edit evidence.

The structural-cursor probe reuses the same attributed pinned packages, validator
and unmodified handler for original/normalized/save/edit selection evidence.

The document-evidence probe reuses the attributed pinned core/renderer/plugins
and hash-pinned native SDK for opaque history transport and glyph/skeleton
checks. No additional package or production dependency is introduced.

The paragraph-origin probe reuses the same attributed pinned SDK/renderer/plugins
and hash-pinned validator; the existing ProseMirror reader checks new source
attributes. No additional package or production dependency is introduced.

The note-origin probe reuses those same pinned/attributed SDK plugins, renderer and
hash-pinned native SDK for nonvisual evidence, rich-marker glyphs and actual
save/edit checks. No new dependency or third-party production source is added.

The run-origin probe reuses the attributed pinned Univer SDK/renderer/plugins,
hash-pinned native SDK and existing ProseMirror reader. No additional
package, production dependency or copied third-party implementation is introduced.

The table-origin probe reuses the same attributed pinned SDK/renderer/plugins,
hash-pinned native SDK and existing ProseMirror reader. No new package,
production dependency or third-party implementation is incorporated.

The image-origin probe reuses the same attributed pinned SDK/renderer/plugins,
hash-pinned native SDK and existing ProseMirror reader. It reuses existing
attributed JPEG and synthetic PNG fixtures; no new production dependency or
third-party implementation is introduced.

The list-origin probe reuses the same attributed pinned SDK/renderer/plugins,
hash-pinned native SDK and existing ProseMirror reader. No new production
dependency or third-party implementation is introduced.

The anchor-origin probe reuses the same attributed pinned SDK/renderer/plugins,
hash-pinned native SDK and existing ProseMirror reader. No new production
dependency or third-party implementation is introduced.

The flow-origin probe reuses the same attributed pinned SDK/renderer/plugins,
hash-pinned native SDK and existing ProseMirror reader. No new production
dependency or third-party implementation is introduced.

The real-source native transport probe reuses the attributed original Copyright
Office InDesign PDF, pinned native SDK, Univer1.0.4 plugins/facade and
ProseMirror reader. Standard gzip uses Go compress/gzip at runtime and Node zlib
only for independent QA. No new production dependency or third-party implementation
is incorporated. Source publication rights and original hash remain unchanged.

The native language probe reuses the already attributed Apache POI original
DOCX fixtures, pinned Univer1.0.4 types/renderer/plugins/facade, hash-pinned VDOCS
validator and ProseMirror reader. No new production dependency or third-party
implementation is introduced. The original fixtures and source-only oracles are
unchanged; native shaping/paint equivalence is not claimed.

The public native profile/example reuses existing attributed source fixtures,
pinned native SDK/Univer facade plugins and ProseMirror. Independent
example checks use the existing optional pypdf and standard-library OOXML reader.
No new production dependency or third-party implementation is incorporated.

The generated PDF structure is independently authored against published PDF
mechanisms and the PDF Association's WTPDF1.0 FENote/NoteType description
(https://pdfa.org/wp-content/uploads/2024/02/Well-Tagged-PDF-WTPDF-1.0.pdf,
CC BY4.0; PDF Association/PDF Reuse TWG/PDF-UA TWG,2024) and ISO document-interchange
errata (https://pdf-issues.pdfa.org/32000-2-2020/clause14.html). No implementation
code is copied or translated. Existing attributed gofpdf/pdfcpu run in-process;
pypdf/MuPDF/Poppler are optional independent QA. No new production dependency.

Tagged PDF list recovery uses independently authored canonical parsing against
the Adobe PDF1.7 reference's standard ListNumbering families
(https://opensource.adobe.com/dc-acrobat-sdk-docs/pdfstandards/pdfreference1.7old.pdf)
and the existing attributed OOXML numbering contract. No implementation code is
copied/translated. Optional independent pypdf/OOXML checks add no production
dependency; existing source fixtures and physical expected labels are unchanged.

Reciprocal PDF ownership validation is independently authored against PDF 2.0
clause 14.7.5.4 and its public corrections:
https://pdf-issues.pdfa.org/32000-2-2020/clause14.html#14754-finding-structure-elements-from-content-items
Optional pypdf checks reuse the previously attributed original Copyright Office
PDF and generated notes; no production dependency or proprietary code is added.
