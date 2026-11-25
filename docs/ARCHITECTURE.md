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


The native container is Manifold-owned (`manifold-document`, version3) around a
pinned Univer snapshot. It is optional interchange with the same SF/model, not a
second canonical representation. Application-specific validators/browser mounts
are no longer part of QA; remaining optional SDK checks inspect actual native
models, glyphs, plugins, edits and facade saves. See VDOCS_FORMAT.md for migration.

## Presentations

`presentation` exposes a public facade for PPTX inspection, planned edits,
validation, review and PNG rendering. `internal/presentation` contains the imported
cohesive reader/writer, model, edit/inspection workflows and raster renderer.
The shared CLI dispatches `manifold presentation …` there. Both subsystems use
one module, dependency graph, CI matrix and release binary. No runtime subprocess
or presentation service is introduced. The native facade now coexists with the central SF codec: positioned objects
are represented inside the shared SF root, while the native commands expose
inspection/edit reports and diagnostic PNG rendering. See [presentation KT](presentation/README.md).

## Central SF presentation extension

`Document.Presentation` is mutually exclusive with flowing root blocks. Its typed
slides contain positioned objects with rich Block content, images and notes;
resources identify master/layout object owners. SF stores this in the optional
`attrs.presentation` field using the existing rich ProseMirror nodes for object
content and notes. Original OPC parts preserve native features and are checked
against parsed canonical objects before regenerating an edited object.
Root Read/Write recognizes PPTX. Presentation PDF output consumes these canonical
objects and writes searchable text, vector shapes and native images. Flow/PDF
input can be mapped to slides with explicit lossy acknowledgement; unsupported
layouts reject. This replaces the facade-only architectural boundary above;
that facade remains a convenience API for source-native workflows.

## Capacity qualification tools

`tools/loadtest` measures warm embedded conversion operations. `tools/hugetest`
measures isolated cold conversions of downloaded, immutable originals pinned by
URL/size/SHA-256 in docs/verification/HUGE_CORPUS.json. The latter has no fixture
generator. A Unix development-only ps sampler guards aggregate child RSS and a
parent deadline kills overdue children. These process controls are test-harness
bounds, not new production admission limits. Publisher documents and raw profiles
remain untracked; curated evidence and actual unsupported outcomes belong in KT.

Huge-input repairs use existing seekable PDF sources without a redundant io.ReadAll
copy, retaining the direct-encryption adapter on parser errors. Geometric annotation
matching indexes links by physical page instead of scanning all document links for
every span. Image masks operate in source component/sample space; uncertainty in
device/profile color conversion remains diagnosed. The repair ledger distinguishes
per-feature checks, original-file completion, fidelity and final capacity evidence.

The internal flowing-SF writer preflights a validated model and bounded encoder
chunks before streaming them; it does not retain a complete second SF graph and
a third decoded model. Public SF graph constructors/readers still validate whole
graphs. Aggregate PDF work policy is explicit in ReadOptions.PDFLimits, independent
of bounded decoded-stream caching; conservative zero-value defaults remain.

## Native equation and legacy diagram preservation

Flow SF includes an additive `office_math` block with authoritative self-contained
OMML (`OfficeMath.OMML`). DOCX import/export preserves native equation structure;
plain text derives source text tokens only. Unsupported math renderers/adapters
reject before output. Legacy Word object wrappers retain their original static
WMF/EMF previews as image bytes, with bounded framing validation and explicit
floating/OLE-behavior diagnostics. DOCX re-embeds those bytes; PDF rendering remains
unsupported. No OLE application code or metafile commands execute.

PDF import may preserve genuinely unmapped simple CFF glyphs as span images from
the original embedded outlines, with source codes/geometry and a missing-Unicode
diagnostic. Known text remains text. This does not introduce OCR or bitmap pages.


## Huge-file processing and caller policy

Flow SF Read decodes top-level nodes into one typed model, then validates global
references at EOF. Write validates the model and preflights all encoder chunks
before emitting output. Public full-tree APIs still allocate a full SF tree.
Streaming reduces duplicate representations; it does not make model memory constant.

PDF parsing uses a seekable caller source directly where available. A request-owned
256-entry font cache, 32 MiB/4,096-entry decoded-stream cache and diagnostic index
bound retention/repeated work independently of cumulative work policy. Parser-only
state is released before untagged inference, or after tagged recovery. Page-sliced
inference releases processed ownership; stable sorting moves integer indices before
reordering wide block values. Geometric links/OBJR units index by physical page;
logical annotation ownership indexes by page/object instead of whole-document scans.

PDFReadLimits exposes decoded/raster/navigation/structure/ParentTree-slot policies.
Defaults stay conservative; the original IPCC's 22M mostly-null owner slots require
an explicit scan budget distinct from logical identities. Cycle, reciprocal owner,
name-byte and complete model validation remain. The development runner adds cold
child-process isolation, sampled aggregate RSS/deadline guards and output hashes.
These are caller/measurement controls, not identity/authorization gates.

Invalid ActualText controls retain exact original bytes in diagnostics while
representable replacement text remains text; when none remains, painted content
is retained. A ToUnicode U+FFFD placeholder may resolve only through a declared
source Encoding glyph name, with a separate diagnostic. Unknown Unicode is never
inferred from GIDs/family names; supported embedded CFF outlines may form span
images. See [measured qualification](verification/HUGE_QUALIFICATION.md).

The diagnostic runner's `-profile-dir` creates an isolated directory for each
child. `-profile-mode cpu` captures separate read/write CPU profiles; memory mode
captures cumulative allocations and sampled live heaps before/read/write/release.
Forced live snapshots run two GC cycles and keep the document alive explicitly.
They are diagnostic runs, not replacements for uninstrumented capacity results.

For I/O attribution, profiling can count underlying file writes and test an
explicit caller-side buffer with `-profile-output-buffer`. Zero leaves normal
output buffering in place. The hash still covers all SF bytes, and flushing is
included in the write phase with errors propagated. This is a diagnostic A/B knob,
not a codec default change. Child records also expose OS user/system CPU totals.

Performance change 1: recursive layout ordering partitions integer indices into
one page-owned box array; a final cycle permutation moves each wide box once.
Stable sorts elsewhere also apply index cycles rather than allocating a second
wide box array. Geometric decisions, stable ties and column labels remain intact.

Performance change 2: the strict scanner borrows validated unescaped key byte
views for stack-local duplicate detection (wide objects fall back to a map).
Streaming SF uses that scanner to enforce unknown fields before direct Unmarshal,
avoiding a second Decoder. SourceInfo attrs use strict unknown-field validation
and direct Unmarshal with immutable metadata for the fixed type graph; legacy
scalar-null/field-alias semantics remain. No customer graph enters a global cache.

Performance change 3: PDF raster/glyph PNG encoding reuses one request-owned
EncoderBuffer for images up to 8,192 pixels wide / 1M pixels total. Larger
images bypass reuse. Release the opaque buffer (which can retain its last image)
before navigation/structural recovery. Compression settings, pixels and bytes
remain unchanged; upstream intermediate image conversions are still independent.
