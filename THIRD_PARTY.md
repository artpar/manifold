# Third party components

manydoc's own code is MIT licensed. No UniOffice/UniPDF implementation code is
included. Third-party components remain under their respective licenses.

| Component | Version | Purpose | License |
| --- | --- | --- | --- |
| github.com/pdfcpu/pdfcpu | v0.11.1 | PDF objects/streams/xrefs/encryption, raster extraction and core font metrics | Apache-2.0 |
| github.com/phpdave11/gofpdf | v1.4.3 | PDF primitives and embedded fonts | MIT |
| github.com/benoitkugler/textlayout | v0.3.2 | Adobe glyph names and simple font encodings | MIT |
| github.com/rivo/uniseg | v0.4.7 | Unicode 15 grapheme/line segmentation (not terminal-cell width) | MIT; Unicode data terms |
| golang.org/x/text | v0.30.0 | NFC normalization for visual glyph selection | BSD-3-Clause; Unicode data terms |
| golang.org/x/image | v0.32.0 | Go fonts, font glyph inspection and TIFF decoding | BSD-3-Clause; additional Go font notice |

The embedded Go fonts are copyright 2016 Bigelow & Holmes Inc. Their full
notice is in `third_party/licenses/Go-fonts.txt`. Direct and runtime transitive
dependency license texts are preserved under `third_party/licenses/`.
Dependency revisions/checksums are pinned in go.mod/go.sum. Unicode segmentation
data in uniseg is Unicode 15.0.0; its MIT notice and the Unicode provider
license are preserved as `uniseg.txt` and `Unicode-V3.txt`. x/text was already a
runtime transitive module and is now imported directly for normalization.

## Test-only assets and tools

`testdata/assets/NotoSans-Regular.ttf` is redistributed under the SIL Open Font
License in `testdata/assets/NotoSans-LICENSE.txt`. It is used to generate PDF
fixtures independently and is not embedded into the manydoc binary.
Source: <https://github.com/notofonts/noto-fonts>.

`testdata/assets/w3c-dummy.pdf` and its corpus copy are a published W3C test
resource: <https://www.w3.org/WAI/ER/tests/xhtml/testfiles/resources/pdf/dummy.pdf>.
The remaining document content is synthetic. Fixture provenance, SHA-256 hashes,
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
