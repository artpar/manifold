# Rich literal list labels

`document.sf` is an authored ProseMirror-profile source: rich literal labels,
URI/internal links, a note citation in a label, and an explicitly empty label.
The enclosing ordered container does not generate numbers for these items.

From the repository root:

```sh
mkdir -p output/literal-example
GOWORK=off go run ./cmd/manydoc convert -o output/literal-example/document.pdf examples/literal-markers/document.sf
GOWORK=off go run ./cmd/manydoc extract -format sf -o output/literal-example/recovered.sf output/literal-example/document.pdf
```

API callers use `Read(..., SF, ...)` and `Write(..., PDF, ...)`. PDF label/body
columns wrap and continue independently; original PDF indents are not reproduced.
PDF recovery reports uncertainty about hidden automatic numbering intent.
DOCX and VDOCS generation of literal markers remains required unfinished work and
currently returns `ErrUnsupported`. See `docs/LITERAL_LIST_MARKERS.md` for details.
