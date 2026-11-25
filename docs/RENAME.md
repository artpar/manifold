# Manifold project identity

The user renamed manydoc to **Manifold** and requested publication at
https://github.com/artpar/manifold on 2026-11-25.

- Go module/import: `github.com/artpar/manifold`.
- Go package: `manifold` (external tests use `manifold_test`).
- CLI source: `cmd/manifold`; executable: `manifold`.
- Install: `go install github.com/artpar/manifold/cmd/manifold@latest`.
- The existing local checkout is still under `artpar/manydoc`; its directory name
  does not determine the Go module or GitHub repository name.

## Persisted compatibility

This is a project/API identity change, not a new document format. The established
`manydoc-prosemirror/1` SF schema, OOXML metadata namespace/tags, PDF internal
navigation namespaces and native editor metadata profiles retain their original
spelling. Renaming those without a migration would make existing documents
unreadable or discard metadata. Their compatibility spelling is intentional.

Original source documents, independent reference oracles, third-party notices,
font fixture names, committed SF examples and historical verification ledger
entries remain unchanged. QA-only `MANYDOC_*` environment variables also retain
their spelling. Source import paths/package identifiers, CLI/help/error prefixes,
atomic-output temporary prefixes, current build commands and QA binary paths now
use manifold. Runtime dependencies and versions are unchanged.

## Verification and publication

Rename verification runs the complete Go suite, race suite with a 20-minute
package timeout, vet/module verification, CGO-disabled host execution and Linux
amd64/arm64 cross-builds. Existing structural/wire/source/asset round trips guard
compatibility without changing the source oracles. Actual ProseMirror checks all
51 accepted corpus files (9,034 nodes/marks), and the public sections example
exercises all four paths. Exact results and publication status are recorded in
STATUS.md and verification.json.

Future imports and builds should use the new identity. Historical commit commands
must use the layout at that commit; earlier commits contain cmd/manydoc. No
repository history is rewritten and no Daptin/Colleaf application is modified.
