# Rich running stories

```sh
GOWORK=off go run ./examples/running-content
output/qa-venv/bin/python scripts/verify_running_content.py
NODE_PATH=output/sf-qa/node_modules node scripts/verify_structured.cjs output/running-content/*.sf
```

Writes source SF, DOCX, PDF and reimported SF under ignored
`output/running-content/`. Uses only independently authored, in-process PNG
fixtures and built-in fonts. No runtime external tools are required.

`letterhead` includes a row-spanning logo, nested/column-spanning header table,
formatted linked text, a footer picture/list, body table continuation and a rich
footnote. Native PDF uses four pages, reserving measured running content around
body and note pages. Its sparse note-only page is a pagination qualification gap.
SF→DOCX→SF exactly preserves this example's semantics.

`tight` has two preferred 1000pt running pictures on a 220pt page. Native PDF
shrinks both while keeping a measured body line and exact alpha pixels. Source SF
preferences stay unchanged. DOCX reimport returns its physically fitted 21.5×172pt
images under the existing rendered-format contract.

For optional producer QA, convert both DOCX files with LibreOffice into
`output/running-content/lo/`, then use `verify_running_content.py --office`.
See docs/TESTING.md for the exact command. The letterhead preserves content on two
producer pages; LibreOffice ignores the body table cell's paragraph-start property.
It paints only one of two oversized tight.docx pictures. The verifier records
that open DOCX containment gap; it does not claim this case passed producer
fidelity. The generated native PDFs retain positioned content and inferred grid
provenance on recovery, without inventing authored header/footer roles.
