# Architecture

## Boundaries

`manifold.go` is the public Go API: document types, formats, errors and conversion
entry points. Its type aliases preserve the shared typed model and its methods;
its functions delegate to the internal engine. `public_api_test.go` exercises the
API from an external Go package. `cmd/manifold` owns flags, files and atomic output.

`internal/engine` owns parsing, representation, validation and layout. It is one
Go package because codecs share the model, source provenance and conversion state.
Files group cohesive subsystems; they do not pretend to be isolated codec services.
There are no application identity, authorization, quota, storage or action gates.
No runtime path invokes external executables, conversion services or license servers.

| Engine modules | Responsibility |
| --- | --- |
| model, text, lists, tables, notes, sections | Typed tree, validation and shared layout semantics |
| structured, json | ProseMirror profile, strict JSON/gzip persistence and bridges |
| provenance, navigation | Physical PDF facts, source ownership and internal targets |
| docx_read, docx_write, docx_text, docx_notes, docx_tables_lists | OOXML packages, styles, relationships, stories, lists, tables and notes |
| pdf_read, pdf_graphics, pdf_images, pdf_resources | PDF objects/streams, text/raster paint, geometry, bounded interpretation |
| pdf_structure, pdf_note_structure, pdf_inference | Explicit logical ownership and separately diagnosed geometric recovery |
| pdf_write, pdf_layout, pdf_text, pdf_fonts, pdf_tables, pdf_lists | Font-aware wrapping, pagination and rich PDF layout |
| pdf_navigation | Physical PDF views/links and inferred editable bindings |
| vdocs_* | Optional Univer/VDOCS snapshot adapter and adapter provenance |
| fonts | Font validation and subsetting |

`internal/bidilevels` contains the attributed bidirectional algorithm and conformance
fixtures. Production dependencies are pinned in go.mod/go.sum; THIRD_PARTY.md records
licenses. Existing public source types and signatures remain available.

## Conversion flow

Read validates input and constructs the typed tree. SF is the versioned established
ProseMirror JSON profile, implemented entirely in Go. Renderers consume the same
tree. Writers stage output and require explicit `AllowLossy` acknowledgement of
known representation losses; unsupported output still rejects.

PDF source geometry and logical ownership are not editable Word layout. Explicit
source facts and inferred relationships/structure carry separate evidence,
confidence and diagnostics. Original pages/assets/navigation remain inspectable.
Glyphs, unsupported shaping and uncertain recovery have explicit outcomes.

The project rename keeps `manydoc-prosemirror/1`, embedded metadata namespaces and
native profile strings for persisted compatibility. See [rename](reference/RENAME.md),
[SF](reference/STRUCTURED_FORMAT.md), [compatibility](COMPATIBILITY.md) and
[testing](TESTING.md). Historical implementation details and the old-to-new file
map are in [history](history/README.md).
