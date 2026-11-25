# Bounded PDF rasters and sample precision

PDF image decoding now bounds byte-filter expansion before invoking a raster
renderer. The implementation is independently authored around standard Go codecs,
the existing pdfcpu fallback and golang.org/x/image/ccitt. No dependency versions,
public API, external runtime requirements or application permission gates change.

## Interpretation and bounds

Image/stencil/soft-mask filter pipelines use the shared native stream decoder.
Raw, Flate, ASCIIHex, ASCII85, RunLength, LZW and TIFF/PNG predictors retain their
existing semantics. Each image-filter stage is bounded to200,000,000 bytes;
syntax streams and Indexed lookup streams retain their64MiB bound. All charge the
shared256MiB document decoded-work budget. At most16 filters are accepted,
including a terminal codec. Actual raster decoding is limited to100,000,000
cumulative pixels, including masks and repeated occurrences; the existing individual
25,000,000-pixel limit remains. Repeated placements retain their separate transforms,
source order and assets. There is no rendered-image cache that collapses instances.
These are finite parser-work limits, not a total process-memory guarantee or app quota.

JPEG headers must agree with declared dimensions before any consumer can decode
pixels, including the fallback's CMYK path. JPEG requires8-bit samples; CCITT
requires1-bit samples. CCITT K0 and negativeK use the existing Go Group3/Group4
codec, bounded by the validated image dimensions. Columns and effective Rows must
agree; Rows0 uses the declared height. BlackIs1 and EncodedByteAlign are honored.
Mixed Group3, JPX/JBIG2 and previously unsupported codecs remain explicit rejections.
Indexed lookup streams are bounded and converted to an unambiguous byte literal
before invoking the fallback. Nested soft masks reject. Masks are composed once,
after their own bounded decoding; differently sized masks retain the diagnosed
nearest-neighbor resampling behavior.

Native DeviceGray/DeviceRGB sample interpretation handles1/2/4/8/16-bit components,
MSB packing and row padding, with explicit Decode mapping.16-bit PNG assets and
soft-mask composition preserve low color/alpha bytes. DeviceCMYK non8-bit samples
now use an explicitly diagnosed device-to-RGB conversion instead of misaligned
samples. Existing8-bit CMYK/color-space fallback behavior remains. ICC/tint/device
color semantics are not fully preserved; color management is still a limitation.
Malformed streams, checksum errors, oversized work or dimension contradictions
reject without partial Document or committed CLI output.

Published interpretation references: ISO32000-1 sections7.4.6/7.4.8 and8.9
([Adobe specification](https://opensource.adobe.com/dc-acrobat-sdk-docs/pdfstandards/PDF32000_2008.pdf)),
[ISO working-group graphics clarifications](https://pdf-issues.pdfa.org/32000-2-2020/clause08.html),
and [Go CCITT API](https://pkg.go.dev/golang.org/x/image/ccitt).
No proprietary UniDoc implementation was copied or translated.

## Source and independent evidence

`internal/engine/pdf_resources_test.go` checks exact pixel components, low16-bit bytes, Decode
inversion, soft alpha, palettes, source geometry, repeated identity/order and full
SF round trips. It includes JPEG behind ASCII85, Group3/Group4 fax/inversion, row padding,
corrupt image/palette checksums, invalid Decode, truncated samples, JPEG header and
codec-depth contradictions, remaining decoded/raster budgets and cancellation.
The optional full public-stage probe expands200,000,001 bytes from a small source;
it rejects before rendering. Independent chunked Python zlib counts that exact
expanded length without retaining it. Six exported negative fixtures reject through
the CLI while preserving an existing destination.

The20 exported positive source PDFs are checked with pypdf and MuPDF for original
samples where applicable, placements, displayed pixels and CLI SF. Pillow separately
decodes the original JPEG. RGB/gray/fax/mask display agrees within one8-bit level;
Go tests independently retain exact16-bit asset values. Poppler also renders the
16-bit mask sample: centerRGB(136,170,204), versus MuPDF(135,169,203), within one
level. The swatch comparison and original publication reflow page1 were visually
inspected. This is targeted raster evidence, not general layout qualification.

Two original strict comparisons remain reproducible failures:

- CMYK device red becomesRGB(255,0,0); MuPDF's managed rendering is(237,28,36).
  The source CMYK samples and conversion diagnostic are checked; no color-fidelity
  claim follows. `--require-cmyk` retains the original strict display oracle.
- The sharp two-pixel JPEG edge gives Go(89,91,0), Pillow(89,92,0), and
  MuPDF(123,74,0) at the first displayed sample. This reflects different chroma
  interpolation; the source fixture is unchanged. `--require-mupdf-jpeg` retains
  the original strict MuPDF oracle. The default verifier requires Pillow agreement.

An exact Git-archive build of a239b38 accepted the corrupt checksum and corrupt
Indexed lookup fixtures. It misread the16-bit RGB sample as(18,52,86), whereas the
source components are0x1234/0x5678/0x9abc. It ignored the rawRGB Decode inversion.
The current reader rejects corruption and preserves the actual source samples.

Original publication structure/content/reflow oracles remain unchanged:
10lists/57items/16URIs/20,531 normalized nonspace characters and9 reflow pages pass.
Its complete SF remains byte-identical to a239b38:
`2d630c8ff72ec594b95b67542d66bf2df13886380672f6a213e5713b8095a7b2`.
The original MQTT PDF complete SF also remains byte-identical:
`14742c55e8295feb0058a74cc33afe394f1ac340fa82aa422af3ad325524f526`.
No original expected behavior was changed to pass.

## Reproduce and continue

```sh
GOWORK=off MANYDOC_PDF_IMAGE_QA=output/pdf-image-qa go test -run 'TestPDFImage|TestPDFRaster' -count=1 ./internal/engine
GOWORK=off MANYDOC_PDF_IMAGE_LARGE=1 MANYDOC_PDF_IMAGE_QA=output/pdf-image-qa go test -run TestPDFImagePublicStageLimit -count=1 ./internal/engine
GOWORK=off CGO_ENABLED=0 go build -o bin/manifold ./cmd/manifold
output/qa-venv/bin/python scripts/qa/pdf/verify_pdf_images.py output/pdf-image-qa
# Retained strict reader discrepancies; both commands currently fail:
output/qa-venv/bin/python scripts/qa/pdf/verify_pdf_images.py output/pdf-image-qa --require-cmyk
output/qa-venv/bin/python scripts/qa/pdf/verify_pdf_images.py output/pdf-image-qa --require-mupdf-jpeg
output/qa-venv/bin/python scripts/qa/layout/verify_published_pdf.py output/pdf-image-published
GOWORK=off go test ./...
GOWORK=off go test -race ./...
GOWORK=off go vet ./...
GOWORK=off CGO_ENABLED=0 go build -o output/manydoc ./cmd/manifold
git diff --check
```

Python readers/Poppler are optional local QA, never production dependencies.
Final commands/timings are in verification.json. The full goal remains active and
incomplete. Initial dependency xref/object-stream parsing and other source/resource
paths still require audit; these image limits do not cover that earlier parser.
The full acceptance audit, broader real source/visual qualification and pinned
native repeated-footnote placement remain open. Continue with STATUS/ACCEPTANCE,
not a completion claim based on this milestone.

Current required checks pass: go test ./... (library28.648s, CLI cached), full
race (final production346.391s, CLI3.223s), focused current race8.543s including the
later test-only Group3 case, vet, module verification, CGO-disabled focused tests
and host/Linux amd64/arm64 builds. Cross-builds are not Linux execution. The shared
syntax-decoder fuzz sample passed11,577 executions in11.469s; it is not image-codec
fuzz or exhaustive safety. The optional full-stage probe passed1.038s. Details are
in verification.json. Original MQTT SF includes one image node; the publication
has none, so its complete-SF equality is source/text/structure evidence.
