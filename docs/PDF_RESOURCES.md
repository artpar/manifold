# PDF structure traversal resources

Input byte and nesting bounds alone do not bound expanded graph traversal.
Tagged PDF attribute arrays may share indirect array objects. Following each
reference without memoization or a work limit can repeatedly visit a small shared
subgraph, even when its nesting is below the existing depth limit.

`pdfStructureRecovery.attributeTotal` now bounds expanded attribute work across
all forward structure owners. Each attribute value visit, class lookup/list entry
and attribute dictionary member costs one work entry. More than 200,000 entries
reject with `structure attributes exceed 200000 work entries`, before returning a
partial document. The existing attribute nesting limit remains 16. Class attributes
still precede local A overrides; revision-selection diagnostics remain unchanged.
Cancellation is checked during these visits, not just between structure elements.

The limit is per document, not per owner or unique indirect object. This matters
for repeated classes on multiple owners and repeated references to shared arrays.
It is an internal parser work bound, not Daptin identity/quota/action admission.
No public API/SF shape, output layout or production dependency changes.

## Regression evidence

- A compact acyclic fanout-eight/depth-six source expands to 262,144 leaf
  attribute visits. Before the fix it returned a successful document. It now
  rejects at the explicit work bound.
- Each of two owners sharing a depth-five graph fits a separate work budget,
  while their combined work exceeds the document budget. The regression
  requires document-wide rejection. Both source fixtures stay below 5,000 bytes.
- A valid smaller shared graph still resolves local LowerRoman over class Decimal,
  preserving the displayed iv. label, start four, body text, original source label
  and SF graph.
- A deterministic Context transition during attribute traversal previously went
  unobserved. It now returns an error wrapping context.Canceled, without a timer.
- FuzzPDFAttributeGraph varies bounded depth/fanout/owner count. Successful reads
  must retain explicit forward body membership/content/order and valid SF; expanded
  graphs can reject only with the declared work limit.

```sh
GOWORK=off go test -run 'TestPDFAttribute|TestPDFPublished' -count=1 .
GOWORK=off go test -race -run 'TestPDFAttribute|FuzzPDFAttributeGraph' -count=1 .
GOWORK=off go test -run '^$' -fuzz '^FuzzPDFAttributeGraph$' -fuzztime=10s -parallel=2 .
```

Original source bytes and existing independent oracles remain unchanged. The
published InDesign source also retains an exactly equal recovered SF and passes
its independent source/SF/DOCX/PDF checker with this bound in place.

## Other scoped structural guards

Forward structure parsing has 64-level and 200,000-entry bounds, with 200,000 Ref
targets. Reciprocal ParentTree parsing has its own graph/slot limits and explicit
missing/contradictory ownership behavior, documented in PDF_PARENT_TREE.md.
These are separate work budgets. They are not a claim of exhaustive PDF resource
safety: decompression, font/image interpretation, geometric inference and other
operators have separate behavior, and broader resource/final audits remain open.

## Expanded XObject execution across pages and appearances

The existing depth guard did not bound execution of a shallow acyclic shared Form
graph. A3203-byte source with seven unique Forms invokes them299,593 times and
previously returned a successful document. Counting only object identities or
resetting a counter between pages would miss this amplification.

`pdfReader.xobjectTotal` now counts every actual Form/image XObject invocation,
including annotation appearances and their descendants, across the whole read.
The200,001st invocation rejects with `PDF XObject expansion exceeds 200000 visits`
before decoding/executing that object. Cancellation is checked on entry. Existing
nesting, image, structure and attribute guards remain separate. This is an internal
parser work bound, not an application quota or performer authorization check. No
partial Document is returned, and the CLI commits no destination on this failure.
A valid document beyond this work bound also rejects explicitly; output is never
presented as a complete extraction after silently skipping the remaining objects.

The original page/Form object definitions are not memoized as rendered content:
each valid invocation still has its actual transform, placement and source order.
A small shared binary graph paints eight distinct LEAF placements from one source
stream, alongside KEEP; tests require all baselines0..70pt in10pt steps, actual
24pt advances, page/source-stream identity and exact SF persistence.

Three independent source-only fixtures stay below6000 bytes:

| Source | Unique Forms | Expanded visits | Purpose |
| --- | ---: | ---: | --- |
| acyclic-shared-form | 7 | 299593 | Shallow fanout-eight graph |
| shared-across-pages | 6 | 224694 | Six pages individually fit; combined work rejects |
| shared-appearances | 6 | 224694 | Six annotation appearances share the graph |

Optional pypdf parses actual content-stream Do operators, resolves their resource
edges, verifies acyclicity/depth and computes these counts without materializing
the expanded graph. It also drives the current CLI and requires atomic rejection.
The published InDesign source still passes its original structural/content/URI/
DOCX/reflowed-PDF oracle (10lists/57items/16URIs/20,531 normalized characters/nine
reflowed pages). This resource-only change has no new output-layout claim.

```sh
MANYDOC_PDF_XOBJECT_QA=output/pdf-xobject-qa GOWORK=off go test -run 'TestPDFXObject' -count=1 .
CGO_ENABLED=0 GOWORK=off go build -o bin/manifold ./cmd/manifold
output/qa-venv/bin/python scripts/verify_pdf_xobject_budget.py output/pdf-xobject-qa
GOWORK=off go test -race -run 'TestPDFXObject|FuzzPDFXObjectGraph' -count=1 .
GOWORK=off go test -run '^$' -fuzz '^FuzzPDFXObjectGraph$' -fuzztime=10s -parallel=2 .
output/qa-venv/bin/python scripts/verify_published_pdf.py output/pdf-xobject-published
```

The CLI oracle requires a fresh output directory without its rejected destination
files. The short fuzz campaign varies bounded graph depth/fanout/page count and
requires exact success below the bound or the declared rejection above it, retaining
per-page source content. It does not establish exhaustive PDF safety. Other decoded
stream/font/image resources and the full goal audit remain open.

A CLI built from the exact pre-change Git archive1d6879a produces a byte-identical
complete publication.sf to the guarded reader (SHA256
2d630c8ff72ec594b95b67542d66bf2df13886380672f6a213e5713b8095a7b2).
This includes the current physical/source/navigation/diagnostic fields, not just
plain text. An older unrelated historical output differed in those fields; it
was not used as a failing oracle or overwritten to establish this comparison.

## CID width amplification

CID width interpretation now avoids replaying overlapping full-range assignments,
converts shared numeric arrays once per font and observes cancellation during this
work. It preserves the previous values without adding a new rejection budget.
Independent reader checks also exposed geometry disagreement on overlapping W
entries, now represented by explicit diagnostics and estimated advance precision.
The unambiguous control agrees with MuPDF. PDF_FONT_WIDTHS.md records the algorithm,
bounded auxiliary domain, complete-inventory/source/SF checks, retained strict
consumer oracle, original publication comparison and remaining memory limits.


## Syntax stream decoding

Page/Form/appearance/ToUnicode paths now enforce bounded filter stages, cumulative
decoded/interpreted work, cancellation and strict completion/checksum handling.
Cache reuse retains repeated physical placements. Standard filters, LZW0/1 and
packed TIFF/PNG predictors are covered. PDF_SYNTAX_STREAMS.md records exact bounds,
independent raw/typed/geometry oracles, original bad-checksum acceptance/fix and
reader discrepancies. This does not bound image codecs or initial dependency
xref/object-stream parsing; those remain separate audit paths.
