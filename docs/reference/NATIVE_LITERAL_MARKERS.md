# Native literal list markers

The optional VDOCS bridge maps literal/rich labels into editable native label/body
cells rather than pretending they are an automatic counter format. Ordinary
numbering remains native counters. Source owner metadata, relationships and assets
stay associated with the actual native document graph.

The label/body layout uses measured advance and shared table structure. Notes,
images, links, nesting and continued flow retain their supported logical ownership.
The bridge validates index/token/cell relationships and rejects unrepresentable
mappings. Unknown counter/start intent remains diagnosed rather than invented.

The implementation and tests are in `internal/engine/vdocs_lists.go` and
`internal/engine/vdocs_lists_test.go`. The generic [VDOCS contract](VDOCS_FORMAT.md)
defines its container and supported SDK profile. [Literal SF markers](LITERAL_LIST_MARKERS.md)
remain part of the canonical document semantics.

Optional QA under `scripts/qa/vdocs` exercises pinned Univer 1.0.4 models, glyphs,
plugins and actual facade saves/edits. The host-specific validator and browser
harness were removed; their historical observations do not certify the current
Manifold envelope or browser fidelity. Current Go/SF/DOCX/PDF and native-SDK scopes
are recorded in `docs/verification.json` and the current verification report.
