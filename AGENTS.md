# Working on manifold

Read `docs/STATUS.md` first for the current implementation state and next steps,
then `docs/ARCHITECTURE.md` and `docs/DECISIONS.md`. Keep these current whenever
you change behavior or leave work for another contributor.

## Product contract

manifold is an independent, pure-Go document processing library and CLI for
embedding in Daptin and applications such as Colleaf. Initial scope: DOCX and
PDF extraction/generation, plus JSON/text interchange. It is not currently a
drop-in replacement for UniOffice/UniPDF or a Word-compatible layout engine.

- No external executables, required services, CGO, or customer-document uploads.
- Do not copy or translate proprietary UniDoc implementation code. Use published
  formats, independently authored code, and explicitly attributed reusable
  open-source dependencies. Do not claim a formally isolated clean-room process.
- Keep document interpretation independent from Daptin identity, authorization,
  quota, storage and action handling. Daptin's action admission authorizes the
  workflow; this library must not introduce performer-level permission gates.
- Never silently discard visible content. Preserve supported content and emit
  explicit diagnostics for representation losses. Reject unsupported output.
- Keep reference fixtures and actual expected behavior; do not change an oracle
  merely to make a test pass. Distinguish structural, textual and visual tests.
- Do not claim professional/Word-equivalent fidelity from synthetic tests alone.

## Commands and completion

Use `go test ./...`, `go test -race ./...`, `go vet ./...`, and
`CGO_ENABLED=0 go build ./cmd/manifold` when the CLI exists. The module declares
Go 1.25.0. Prefer `GOWORK=off` during development to avoid surrounding workspace
files selecting a different module graph. Run `git diff --check` after changes.

Every handoff must state what exists, which checks actually ran and their
results, unresolved issues, and the next concrete commands. Keep production
dependencies in go.mod/go.sum and third-party notices in THIRD_PARTY.md. Do not
commit generated output, executables, virtual environments, or customer files.

The project was created at the user's explicit request under
`~/workspace/code/github.com/artpar/manydoc`. Do not modify Daptin as part of
building this standalone project unless separately requested.
