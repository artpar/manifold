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
