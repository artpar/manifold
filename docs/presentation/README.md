# Presentations in Manifold

The `github.com/artpar/manifold/presentation` Go package and
`manifold presentation` CLI integrate Puppt's PPTX workflows into Manifold.
Source: artpar/puppt commit 7f8d6d0b2dab54296fde444af7f9ac6a5816a2a7.

```sh
manifold presentation inspect deck.pptx --json
manifold presentation plan deck.pptx --edit edit.json --json
manifold presentation edit deck.pptx --edit edit.json --out edited.pptx --json
manifold presentation validate edited.pptx --json
manifold presentation render edited.pptx --slide 1 --out slide.png --json
```

```go
result, err := presentation.Inspect(ctx, "deck.pptx")
// Handle err, then inspect result.Status, Warnings and Unsupported.
imageResult, err := presentation.Render(ctx, "deck.pptx", presentation.RenderOptions{
    SlideNumber: 1, OutputPath: "slide.png", DPI: 144,
})
```

Public functions: Inspect, Plan, Edit, Validate, Review, SlideCount and Render.
Plan/Edit take a JSON specification filename; Review takes a prior change-report
filename. This first integration preserves the upstream file-based API. Public
result/specification types are available without importing internal packages.
No Puppt executable or separate Go module is required.

Central conversions use the root Manifold API and the same SF format:

```sh
manifold extract -format sf -o deck.sf input.pptx
manifold convert -o roundtrip.pptx deck.sf
manifold convert -allow-lossy -o slides.pdf deck.sf
manifold convert -allow-lossy -o recovered.pptx slides.pdf
```

`manifold.Read(..., manifold.PPTX, ...)` produces `Document.Presentation`;
`manifold.Write` accepts PPTX/SF/PDF. SF stores typed slide objects, coordinates,
text/image content, notes and master/layout resources, with an explicit feature
marker that old readers reject. Native parts preserve unmodeled features.
Supported text/frame edits patch native XML; unchanged source parts round-trip
byte-for-byte. Authored slide objects can create new decks/slides.

PDF output writes searchable text, vector shapes and images. It rejects unsupported
native effects/groups/graphic frames and overflowing text, rather than silently
rasterizing an incomplete slide. Explicit font-family mappings may be needed.
Physical PDF text/image quads can be recovered into PPTX; flowing SF is reflowed
into slide text boxes with AllowLossy acknowledgement. Unsupported complex flow
layouts, native layout/master rebinding and unmodeled regeneration reject.
Speaker notes remain in SF/PPTX; exporting visible PDF pages requires acknowledging
that notes are not page content. Legacy .ppt remains unsupported. The native
inspection/edit/review/PNG API remains available as a convenience, not a separate
canonical representation. Full PowerPoint visual equivalence is not claimed.

The `puppt.v1` command-result envelope is retained for existing consumers; it is
an operation report, not a second canonical document interchange format.

[Commands](COMMANDS.md), [support matrix](SUPPORT_MATRIX.md),
[rendering limits](RENDERING.md), [implementation KT](TECHNICAL_KT.md).
Those imported references describe source-project evidence; they do not establish
new Manifold qualification. Original synthetic tests and coverage metadata are
retained. External real-deck/pixel-oracle corpora were ignored upstream and are not
in this checkout; opt-in PUPPT_* tests continue to skip unless supplied explicitly.
Ordinary CI tests both document and presentation packages on all three OSes,
with Linux race checks and one CGO-disabled CLI release.
