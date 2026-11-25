# PDF destination recovery and editable navigation

## Contract and source of truth

`Document.PDFNavigation` / SF root `attrs.pdfNavigation` retains explicit PDF
navigation evidence. `PDFNavigation.Destinations` records IDs, optional original
name namespace/bytes, source page/object, view kind, original nullable parameters,
and derived display-point/constraint geometry. `PDFNavigation.Links` records each
supported URI/GoTo annotation's page, object, target and transformed regions.
Legacy JSON uses `pdf_navigation`. These structures are provenance, separate from
current editable-flow anchor definitions and link marks.

The primary format basis is ISO 32000-1:2008 §7.9.6, §12.3.2 and Tables 151/173:
[Adobe's published specification](https://opensource.adobe.com/dc-acrobat-sdk-docs/pdfstandards/PDF32000_2008.pdf).
The [PDF Association's destination errata](https://pdf-issues.pdfa.org/32000-2-2020/clause12.html)
clarifies newer structure-destination syntax. No proprietary implementation was
consulted or copied for this implementation. Production remains Go-only, using
the already attributed pdfcpu object parser; there are no new dependencies.

Named destinations are byte identities. Legacy `/Dests` name objects and name-tree
string keys have separate namespaces. `NameKind` is `name`, `string`, or empty for
anonymous arrays; `NameBytes` preserves the actual decoded object bytes, base64 in
SF/JSON. A valid, unique display string can serve as the editable ID. Otherwise a
deterministic, collision-checked `pdf:destination:` hash ID preserves distinct byte
identities. Different encodings of the same visible spelling do not collapse.
Anonymous equal page/view/parameter arrays share a synthetic view ID. Missing named
targets retain their original bytes plus an unresolved record and declaration;
no page is invented. Import resolves after all physical page references exist.

## Physical views versus editable inference

All standard views are represented: XYZ, Fit, FitH, FitV, FitR, FitB, FitBH, FitBV.
`Parameters` retains source user units, null coordinates/zoom and zero zoom.
Null means viewer-state retention, not zero. `Point` exists only when both XYZ
coordinates are known. `Region` records FitR corners or a line constraint for a
known single coordinate; page transforms apply crop/rotation/UserUnit. Fit/FitB
and wholly nullable views have no invented point. Geometry is checked against
parameters and the stored page transform during model/SF validation.

The editable ID binds to nearby recovered text/image content, with `FlowBinding`
`nearest_content` and its `FlowOrder`, or a `page_boundary` fallback. Paragraph-start
bindings use block anchors; later runs get zero-width atoms before the selected
source span. The original view stays in PDFNavigation. A missing blank-page flow
boundary gets a diagnosed tail fallback, without claiming its physical page
survived logical recovery. `pdf_destination_flow_inferred` explicitly distinguishes
these bindings from authorial bookmark ownership. Confidence is not a probability.

Tagged OBJR Link ownership supports URI and GoTo targets, independently of
geometric rectangle association. Conflicting/partial ownership remains diagnosed;
physical link evidence remains available even when no editable text mark can own
an annotation. Empty URI annotations remain physical evidence with an explicit
empty-href diagnostic. Unsupported non-URI/non-GoTo actions are diagnosed and are
never executed. Additional structure-destination navigation is not inferred from a
physical view alone; an SD-bearing destination dictionary emits a diagnostic.

SF/JSON retain this provenance and independent editable graph. DOCX export and
native PDF reflow use the editable anchors; they do not reproduce original PDF
fit/zoom, physical pages or annotation appearance. Native destinations use its
current XYZ line/block-point policy. Source diagnostics require acknowledgement
for formatting output. This is an explicit reflow policy, not exact PDF viewing-
state round-trip fidelity. Typed semantic footnote/endnote inference from physical
links is separate work; anonymous note-view recovery is not proof of note roles.

## Edited flow and retained source history

FlowBinding also accepts unbound when the original view/name evidence has no
current editable-flow assertion. Physical pages, nullable parameters, name bytes,
view/annotation geometry and last source FlowOrder stay historical. A formerly
resolved source view can lack a current anchor without losing its physical facts;
a source-missing name stays Unresolved even if an editor later creates a target
with that identity. No source page or editable anchor is invented. Ordinary
nearest_content/page_boundary values still require current anchor definitions.
Source-unresolved entries require current missing declarations unless explicitly
unbound; their physical page/view fields remain absent and source order remains0.

The private native adapter transports this document-level evidence separately
from current native text/style/layout. It validates physical syntax first, then
reconciles edited target availability with explicit diagnostics. Final full-model
validation checks the current graph. Public JSON/SF round trips retain unbound
values; formatting output still uses only the current editable graph. Per-node
native provenance bindings and full adapter/browser qualification remain open.

## Validation, robustness and evidence

Import bounds destination-tree depth at 64 and tree nodes at 100,000. The default
import navigation-entry policy remains 100,000; callers can explicitly select up
to 1,000,000 through ReadOptions.PDFLimits.MaxNavigationEntries. The model/SF
ceiling is 1,000,000 entries and 64 MiB aggregate raw destination-name bytes, in
addition to the 65,536-byte per-name bound. These separate work and aggregate-byte
bounds allow large, short-name registries without allowing unbounded long names.
Import
checks local page references, view syntax, finite coordinates, nullable positions,
name-byte duplication and cyclic/repeated tree nodes. Unknown view kinds reject.
Model/SF checks validate unique IDs/names, defined/unresolved graph membership,
geometry consistency, source pages and annotation target/region consistency.
Bridges clone pointer-bearing navigation values so SF editing does not mutate the
caller. Hand-authored tests cover all eight views, a forward cropped/rotated page,
byte/namespace collisions, binary/Unicode names, nulls, explicit named/direct
annotations, missing references, malformed trees/views and tagged partial ownership.

Independent `scripts/qa/layout/verify_navigation_import.py` reads the original PDFs through
pypdf. It compares every source named namespace/key byte sequence, anonymous view,
page, mode and nullable parameter, and every supported annotation target/rectangle
corner against SF. Every resolved view has exactly one flow anchor; inferred anchors
remain attached to the selected source page/order rather than an arbitrary location.

- Native two-page example: 17 named views, one anonymous note view, one missing
  name, 13 link annotations. The missing target stays unbound; bare #literal stays URI.
- Original ten-page InDesign publication: one named view and 19 link annotations.
  Page 2's GoTo reaches page 6 at XYZ(95,699,null); its 16 distinct editable URI
  targets remain unchanged. The old URI-count test now filters typed internal marks
  rather than misclassifying the newly recovered internal href as a seventeenth URI.
- Original 137-page MQTT PDF: 325 distinct anonymous views and all 1,245 annotation
  links. Existing table/merge/text/asset/source oracles remain unchanged and pass.
- Actual ProseMirror validates 8,904 nodes/marks/attributes across the two imported
  small SFs, including the new navigation root values. No full 477 MB ProseMirror
  check is claimed. Go validation and the independent view/flow audit use that full SF.

The original MQTT SF is **477,613,781 bytes** (about 455.5 MiB). This remains a
required provenance/codec usability issue, not a success criterion satisfied by
navigation correctness. Generated PDFs did not change painting; the previous
native/DOCX visual evidence still applies because this slice changes import only.

## Recovery and next work

Run the commands in TESTING.md. Evidence is ignored under output/navigation-import
and output/destinations; durable check summaries are in verification.json.
After this slice, perform a bounded requirement-by-requirement completion audit:
source/recovered rich semantics and note/running/page evidence, representative
producer corpus and visual qualification, and the large provenance/codec costs.
Do not expand the goal into Word-equivalent reflow, or declare the goal complete
merely because navigation paths and the existing suite pass.

Final checks: full tests passed (library 15.043s / CLI 0.383s), full race checks
(178.326s / 1.951s), vet, module verification, host CGO-disabled build/execution,
Linux/amd64 CGO-disabled cross-build and diff whitespace check. Ten-second SF/PDF
fuzz campaigns with new navigation seeds passed 2,331 / 43,899 executions, two
workers each; new metadata/seed cases also passed a focused race run. The host CLI
exercised all four operations with explicit `extract -format sf`; its imported SF
passes actual ProseMirror (37 nodes). None of these checks completes the full goal.
