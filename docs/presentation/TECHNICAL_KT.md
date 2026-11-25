# Technical KT

## Architecture

Puppt is a Go CLI with business logic in internal packages. The command layer is intentionally thin and delegates to workflow packages.

| Package | Responsibility |
|---|---|
| `cmd/puppt` | CLI process entrypoint |
| `internal/presentation/cli` | Cobra command wiring and stream handling |
| `internal/presentation/pptx` | Owned Open Packaging Convention reader/writer |
| `internal/presentation/inspect` | Structured deck inspection |
| `internal/presentation/target` | Target resolution and ambiguity detection |
| `internal/presentation/edit` | Edit planning and mutation workflows |
| `internal/presentation/validate` | Package structure and relationship validation |
| `internal/presentation/review` | Change review summaries |
| `internal/presentation/report` | Stable JSON output |
| `internal/presentation/model` | Shared `puppt.v1` result and domain structs |
| `internal/presentation/fixtures` | Deterministic test deck builders |

## Data Flow

Inspection:

```text
CLI -> internal/presentation/inspect -> internal/presentation/pptx.Open -> model.CommandResult
```

Edit:

```text
CLI -> internal/presentation/edit.Plan -> internal/presentation/inspect -> internal/presentation/target
CLI -> internal/presentation/edit.Apply -> internal/presentation/pptx.Open -> mutate package parts -> internal/presentation/pptx.Write -> internal/presentation/validate
```

Review:

```text
CLI -> internal/presentation/review -> read changes JSON -> internal/presentation/inspect + internal/presentation/validate -> model.CommandResult
```

## Core Invariants

- Only modern `.pptx` ZIP/Open XML packages are supported.
- The authoritative package reader/writer and mutation path are owned by Puppt.
- Every edit must plan and resolve targets before mutation.
- Ambiguous and no-match targets must not mutate output.
- Unsupported operations must be rejected or reported explicitly.
- JSON output uses the additive `puppt.v1` envelope.
- Unknown package parts should be preserved where feasible.

## JSON Compatibility

The stable envelope is:

```json
{
  "schema_version": "puppt.v1",
  "command": "inspect",
  "status": "ok",
  "input": "input.pptx",
  "output": null,
  "warnings": [],
  "errors": [],
  "summary": {
    "human": "..."
  }
}
```

Fields may be added, but existing v1 fields should not be removed or repurposed without a compatibility decision.

## Validation Boundary

`internal/presentation/validate` checks core package readability and relationship target reachability. Edit workflows add expected-content checks after writing. General-purpose expected-content assertions are not yet part of `manifold presentation validate`.

## Dependency Boundary

Cobra owns CLI routing only. PPTX reading, writing, inspection, mutation, validation, and review logic remain in Puppt-owned Go code using standard library ZIP/XML/JSON primitives.

## Known Gaps

- Real-world deck fixture breadth.
- Advanced object extraction.
- Rich media metadata.
- Notes part creation for existing slides.
- Rendered visual validation.
- Release packaging and CI.
