# Application-reference cleanup

The user requested removing application-specific names while explicitly keeping
VDOCS. SF remains the canonical structured representation: DOCX, PDF and VDOCS
convert to/from the same shared document model. VDOCS is optional native snapshot
interchange, not another canonical model or DOCX/PDF pipeline.

## Changes and compatibility

- Public/private code, comments, native diagnostics, QA environment names,
  examples, paths and current documentation use neutral Manifold/VDOCS names.
- The native fixture moved to testdata/univer. Its entire `document` payload is
  unchanged; only the container's identifying type changed.
- VDOCS now uses type `manifold-document` and MIME
  `application/vnd.manifold.docs+json;version=3`. Version3 and the pinned Univer
  payload profile remain. Retired branded headers are not silently accepted;
  existing files require an explicit header migration and full normal validation.
- SF schema/profile and the supported DOCX/PDF paths are unchanged.
- Host-specific source loading, validator calls and browser harnesses were removed.
  The remaining SDK/model/glyph/plugin/edit/save assertions remain in place.
- Application labels/source links were removed from retained historical narratives;
  those retired host observations are not current Manifold-container qualification.
  Git history was not rewritten.

## Verification

A current tracked-tree scan finds none of the requested application names.
Public native graph/boundary tests verify the new type/MIME and preserve complete
SF relationships/assets/history. Fresh native output passes actual pinned SDK
facade save/reopen and Go semantic re-import (0.388s export /0.377s re-import);
the public native suite passes0.390s. All33 retained SDK probes pass.

Normal Go tests pass: root5.943s, CLI1.051s, engine24.031s; a later full run is
cached. Full race passes: root76.791s, CLI3.055s, engine278.612s, with no races.
Vet/modules and CGO-disabled host/Linux builds pass; Linux builds are not execution.
All51 accepted core source fixtures retain byte-identical complete SF against
the pre-organization baseline. Actual ProseMirror validates9,034 nodes/marks;
section and native examples pass. QA syntax and current documentation links pass.
No reference document payload or standalone conversion oracle was weakened.

Ignored evidence is under output/reference-cleanup; current scopes/results are in
docs/verification.json. Existing declared codec/script/layout limits remain in
COMPATIBILITY.md. This change does not add legacy binary DOC or promise identical
PDF layout round trips.


## Published-tree verification

Published source commit `8536dc0` passes a fresh clone/build and a scan of all
tracked paths/text. SF→VDOCS→SF preserves the complete normalized graph, and native
VDOCS→DOCX/PDF→SF recovers the expected three typed notes without an ignored QA
environment. The first clone scan caught omitted neutral example/QA changes; the
follow-up source commit fixes those omissions. The report example independently
passes python-docx/Poppler readers (two PDF pages, one table and one image).
This evidence follow-up changes documentation only.
