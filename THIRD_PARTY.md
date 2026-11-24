# Third party components

manydoc's own code is MIT licensed. No UniOffice/UniPDF implementation code is
included. Third-party components remain under their respective licenses.

| Component | Version | Purpose | License |
| --- | --- | --- | --- |
| github.com/pdfcpu/pdfcpu | v0.11.1 | PDF objects/streams/xrefs/encryption, raster extraction and core font metrics | Apache-2.0 |
| github.com/phpdave11/gofpdf | v1.4.3 | PDF primitives and embedded fonts | MIT |
| github.com/benoitkugler/textlayout | v0.3.2 | Adobe glyph names/encodings, TrueType layout and HarfBuzz glyph shaping | MIT |
| github.com/rivo/uniseg | v0.4.7 | Unicode 15 grapheme/line segmentation (not terminal-cell width) | MIT; Unicode data terms |
| golang.org/x/text | v0.30.0 | NFC normalization, font-name case folding, BCP47/script metadata and bidi properties | BSD-3-Clause; Unicode data terms |
| golang.org/x/image | v0.32.0 | Go/Go Mono fonts, font glyph inspection and TIFF decoding | BSD-3-Clause; additional Go font notice |

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
pins in `scripts/requirements.txt`. They are not linked to or invoked by the Go
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

`scripts/font_oracles.py` records SHA256 values of original Go programs from the
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
DejaVu Sans asset. `scripts/verify_bidi_output.py` independently compares actual
PDF groups with native HarfBuzz and C FriBidi; these are optional QA tools only.
The two FuzzPDFBidiRoundTrip inputs are exact synthetic invented-space regressions.
No additional font/module/service/CGO or proprietary code is introduced.


`testdata/assets/DejaVuSans-Bold.ttf` is the original DejaVu Sans Bold 2.37 from
the same pinned official release ZIP (SHA256 7576310b219e04159d35ff61dd4a4ec4cdba4f35c00e002a136f00e96a908b0a).
The existing testdata/assets/DejaVu-LICENSE.txt applies. TTF SHA256:
`e6476c1b80502924294eed40894c5b18e06c181444ca953e5334262df9c27724`.
It is test/example data, not embedded into the library/CLI. The authored
`testdata/rtl-item-context.json` records fourteen original-font native HarfBuzz
item-context measurements; optional scripts/verify_bidi_context.py checks actual
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
scripts/requirements.txt. It is used only by independent development oracles,
not by the Go library or CLI. The glyph subsetter retains original embedded name
records/notices and honors the existing TrueType embedding/subsetting flags.

The ignored BabelStone Han 16.0.3 QA input is obtained from its
[primary publisher](https://www.babelstone.co.uk/Fonts/Han.html), under the original
Arphic Public License embedded in its name table. Source/derived font files are
not redistributed in this repository or linked into the binary. Caller-supplied
font programs remain subject to their own reuse/embedding terms. FONT_SUBSETTING.md
records hashes, checks and scope; no publisher/customer PDF font was extracted
or repurposed as a manydoc output font.
