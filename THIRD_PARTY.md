# Third party components

manydoc's own code is MIT licensed. No UniOffice/UniPDF implementation code is
included. Third-party components remain under their respective licenses.

| Component | Version | Purpose | License |
| --- | --- | --- | --- |
| github.com/pdfcpu/pdfcpu | v0.11.1 | PDF object parsing, streams, xrefs, encryption | Apache-2.0 |
| github.com/phpdave11/gofpdf | v1.4.3 | PDF primitives and embedded fonts | MIT |
| github.com/benoitkugler/textlayout | v0.3.2 | Adobe glyph names and simple font encodings | MIT |
| golang.org/x/image | v0.32.0 | Go fonts and font glyph inspection | BSD-3-Clause; additional Go font notice |

The embedded Go fonts are copyright 2016 Bigelow & Holmes Inc. Their full
notice is in `third_party/licenses/Go-fonts.txt`. Direct and runtime transitive
dependency license texts are preserved under `third_party/licenses/`.
Dependency revisions/checksums are pinned in go.mod/go.sum.

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
