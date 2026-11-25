# Central PPTX/SF/PDF qualification

The root reader/writer API now recognizes PPTX; this supersedes the facade-only
v0.2.0 integration. The canonical state is the typed presentation extension of SF,
not an independent VDOCS-like container or an opaque serialized deck snapshot.

## Actual checks

- Root API and CLI tests cover PPTX→SF→PPTX, explicit SF text/frame edits,
  searchable/vector SF→PDF, recovered PDF→PPTX and basic flowing SF→PPTX.
- Untouched native parts compare byte-for-byte across SF/plain/gzip/typed-JSON
  persistence. Two unmodified Apache POI source decks exercise original package
  preservation, including unmodeled SmartArt/native resources.
- Real-source SF text edits preserve unrelated package parts. A canonical slide
  addition creates valid blank layout/master relationships; an independent ZIP/XML
  test checks unique relationship IDs, existing internal targets and main MIME.
- Negative tests reject ambiguous flow/slide owners, missing feature markers,
  unsupported visible output, text overflow and Word packages mislabeled as PPTX.
  Notes deletion and hidden slide/object behavior are checked.
- The actual ProseMirror reader checks nested rich nodes and rejects presentations
  under an older schema missing the explicit presentation feature-marker node.
- The Go example creates a source PPTX, reads/writes SF, edits it, exports PPTX/PDF
  and recovers a PPTX from PDF. python-pptx 1.0.2 reads the produced decks, sees the
  edited title and exact 55pt x-position, and reads the speaker-notes body.
- PyMuPDF 1.28.2 reads the example PDF as one 720x540pt page, sees searchable edited
  title text, one raster image and two vector drawings. Its rendered page was
  visually inspected: colored title, gradient source image, green outlined ellipse
  and light page background are correctly positioned. This is a synthetic layout
  check, not proof of arbitrary PowerPoint visual parity.

## Reproduce

```sh
GOWORK=off go test ./... -timeout 20m
GOWORK=off go test -race ./... -timeout 20m
GOWORK=off go vet ./...
CGO_ENABLED=0 GOWORK=off go build -o bin/manifold ./cmd/manifold
GOWORK=off go run ./examples/presentation
NODE_PATH=output/sf-qa/node_modules node scripts/qa/structured/verify_structured.cjs output/presentation/source.sf output/presentation/edited.sf
git diff --check
```

Independent Python/ProseMirror readers are optional development tools. No Python,
JavaScript, external renderer, service or CGO is required by production conversion.
Generated test artifacts/profiles and reader environments remain ignored.

## Boundaries

Native charts/SmartArt/effects/complex groups and unknown metadata survive an
untouched PPTX round trip. Supported text/frame patches preserve unrelated XML;
unsupported regeneration/PDF output rejects. Native layout/master rebinding and
unsupported flow layouts also reject. Explicit font mappings remain necessary
for fonts outside the existing PDF font contract. PDF→PPTX recovers existing text/
image geometry; general vector reconstruction and PowerPoint authoring intent
are not claimed. Font-size recovery into DrawingML rounds to 0.01pt after explicit
loss acknowledgement. Source package/resource limits are documented in the SF
contract. Readable source fixtures are pinned in testdata/real/manifest.json.

Final production source `45bebca` passed all Linux/macOS/Windows jobs and the full
Linux race suite in [Actions run 45597860193](https://github.com/artpar/manifold/actions/runs/45597860193).
The automated [v0.3.0 release](https://github.com/artpar/manifold/releases/tag/v0.3.0)
is published with six archives and checksums. Expanded stress results, including
central PPTX/SF/PDF routes, are recorded in [PERFORMANCE.md](../PERFORMANCE.md).
