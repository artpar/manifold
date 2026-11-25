# Source organization qualification

The user flagged the fragmented root/code organization while the renamed project
was being prepared for publication. No initial source push occurred before this
cleanup; the remote repository was created empty.

## Changes

- Root: public API facade and external API tests, rather than 247 Go files.
- Engine: 130 production files consolidated into 36 cohesive modules; related
  unit/integration/fuzz tests remain adjacent, with common reference fixtures.
- Private native-adapter identifiers use vdocs. Actual publisher/application names
  remain in source provenance, and persisted diagnostics/profile strings retain
  their compatibility spelling.
- QA: 134 tools grouped by format/concern, with repository-root discovery and
  subprocess call sites updated after moves.
- Docs: short current guides, detailed reference contracts, scoped qualification
  reports and predecessor/history snapshots. The cumulative ledger is preserved,
  not discarded to make a small current summary.

The public API retains its types, constants, methods and function signatures.
Function wrappers delegate; type aliases preserve the shared model. Persisted
SF/DOCX/PDF/native metadata identifiers and fixture/oracle bytes remain unchanged.
The engine is still one shared Go package. This is an organizational cleanup,
not a claim that the existing codec state has been decoupled into independent
packages. See [architecture](../ARCHITECTURE.md) for the actual boundary.

## Evidence

A mechanical function/method inventory retains all 1,729 definitions, including
691 test/fuzz/benchmark functions; normalization covers only private adapter and
test prefixes. The new TestMain establishes the repository-wide fixture working
directory before tests/fuzz workers. Old-to-new filenames are recorded in
[the source map](../history/source-file-map.json).

Full Go checks, independent SF/reader qualification, clean-checkout conversion and
publication verification are recorded in [the current ledger](../verification.json).
Generated logs/output remain ignored under output/reorg. Structural/input/output
oracles are retained; tests were not weakened to accommodate consolidation.
