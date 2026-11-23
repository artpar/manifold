# Third party components

manydoc's own code is MIT licensed. No UniOffice/UniPDF implementation code is
included. Third-party components remain under their respective licenses.

| Component | Version | Purpose | License |
| --- | --- | --- | --- |
| github.com/pdfcpu/pdfcpu | v0.11.1 | PDF objects/streams/xrefs/encryption, raster extraction and core font metrics | Apache-2.0 |
| github.com/phpdave11/gofpdf | v1.4.3 | PDF primitives and embedded fonts | MIT |
| github.com/benoitkugler/textlayout | v0.3.2 | Adobe glyph names/encodings, TrueType layout and HarfBuzz glyph shaping | MIT |
| github.com/rivo/uniseg | v0.4.7 | Unicode 15 grapheme/line segmentation (not terminal-cell width) | MIT; Unicode data terms |
| golang.org/x/text | v0.30.0 | NFC normalization, font-name case folding and BCP47/script metadata | BSD-3-Clause; Unicode data terms |
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
