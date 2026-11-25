# Explicit page geometry

```sh
GOWORK=off go run ./examples/page-layout
output/qa-venv/bin/python scripts/qa/layout/verify_page_layout.py
NODE_PATH=output/sf-qa/node_modules node scripts/qa/structured/verify_structured.cjs output/page-layout/*.sf
```

The Go program writes source SF, DOCX, PDF and both reimports under ignored
output/page-layout/. No external tools are required at runtime. Python,
ProseMirror and LibreOffice are optional independent developer QA.

The three cases are an asymmetric landscape document with merged running content,
a repeating body table, conditional cell start, linked RTL note and images;
explicit zero edges/distances; and independent anchors on a legacy scalar margin.
Source SF/DOCX settings/content/assets match exactly. Native PDFs use nine pages
with fourteen exact-pixel placements. See docs/TESTING.md for producer conversion;
`verify_page_layout.py --office` checks six producer pages/eight placements.

Use Layout.Margins to override every edge, including zero. HeaderDistance and
FooterDistance pointers measure header top and footer bottom from the page edge.
Nil preserves defaults; a nonnil zero is a real zero. The legacy scalar fallback
is retained as data even when the group overrides it. Explicit groups use 18pt
anchors by default; legacy nil geometry keeps its established writer behavior.
The source example includes an ignored 41pt fallback to verify this distinction.

SF retains full precision; DOCX physical settings quantize to twips. Signed
vertical offsets survive SF/DOCX and render natively when measured stories do not
overlap the fixed body. Actual signed overlap remains unqualified and rejects
atomically. See compatibility notes for section, binding and conditional-story gaps.

The native rich case has six pages, including a sparse first page from existing
note/table admission. LibreOffice uses three, ignores the cell start and draws a
left note separator. Both preserve all content, but this is not Word-equivalent
pagination. PDF recovery retains physical page/asset positions and uncertainty;
it does not invent authorial margins, anchors or header/footer roles.
