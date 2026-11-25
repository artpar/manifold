# Optional qualification tools

These tools provide independent readers, source oracles and visual/geometry checks.
The Go library and CLI do not invoke them. Run from the repository root.

| Directory | Concern |
| --- | --- |
| common | Fixture generation and historical verification tooling |
| docx | Original OOXML exports, source-only oracles, precision/styles/stories |
| pdf | PDF source syntax, resources, structure, assets and note recovery |
| structured | ProseMirror schema and complete structured-document checks |
| layout | Fonts, Unicode, pagination, tables, lists, notes and running content |
| vdocs | Optional native snapshot/SDK/browser interoperability |

Python QA dependencies are in requirements.txt. Node checks use their documented
ignored environments under output. Detailed setup and pinned source/tool versions
are recorded in docs/reference and docs/history; current commands are in
[TESTING.md](../../docs/TESTING.md). Keep original fixtures/oracles unchanged.
