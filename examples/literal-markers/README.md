# Rich literal list labels

`document.sf` is an authored ProseMirror-profile source: rich literal labels,
URI/internal links, a note citation in a label, and an explicitly empty label.
The enclosing ordered container does not generate numbers for these items.

From the repository root:

```sh
mkdir -p output/literal-example
GOWORK=off go run ./cmd/manifold convert -o output/literal-example/document.pdf examples/literal-markers/document.sf
GOWORK=off go run ./cmd/manifold convert -o output/literal-example/document.docx examples/literal-markers/document.sf
GOWORK=off go run ./cmd/manifold convert -o output/literal-example/document.vdocs examples/literal-markers/document.sf
GOWORK=off go run ./cmd/manifold extract -format sf -o output/literal-example/native-recovered.sf output/literal-example/document.vdocs
GOWORK=off go run ./cmd/manifold extract -format sf -o output/literal-example/recovered.sf output/literal-example/document.pdf
```

API callers use `Read(..., SF, ...)` and `Write(..., PDF, ...)` or `Write(..., DOCX, ...)`. PDF label/body
columns wrap and continue independently; original PDF indents are not reproduced.
PDF recovery reports uncertainty about hidden automatic numbering intent.
VDOCS uses actual native label/body cells with checked ownership and real counters.
Rich/empty labels, nested structures and six-page continuation have scoped browser
evidence; it does not reproduce original PDF indents. See
`docs/NATIVE_LITERAL_MARKERS.md` for evidence and remaining qualification limits.
