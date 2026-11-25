# Current decisions

1. **Independent Go runtime.** No required CGO, external converters, services,
   license servers or document uploads. Daptin and other applications own identity,
   authorization, storage, quotas and action execution.
2. **Established shared SF.** Use ProseMirror JSON with an explicit versioned rich
   document profile. Go performs all validation, manipulation and persistence;
   running an editor is not a runtime requirement. Alternatives and actual editor
   inspection are recorded in [the SF decision](reference/STRUCTURED_FORMAT.md).
3. **Explicit interpretation.** Preserve supported visible content, relationships
   and assets. Diagnose representation loss and inferred PDF meanings; reject
   contradictory or unsupported output. Do not weaken independent source oracles.
4. **Declared fidelity.** Qualify practical rich conversion, not full OOXML/API
   parity or Word pagination identity. Tests check structure/styles/relationships/
   assets and source facts; independent readers and scoped visual checks supplement
   synthetic tests. Unsupported shaping and missing glyphs reject explicitly.
5. **Stable persisted compatibility during rename.** Package/module/CLI become
   `manifold`; existing SF and embedded metadata identifiers retain their spelling.
   The user requested publication at `artpar/manifold`.
6. **Cohesive source organization.** A small public facade delegates to one shared
   internal conversion engine. Consolidate related implementation and tests;
   separate QA by concern and current documentation from historical evidence.
   Do not introduce cyclic codec packages or mutable public function aliases.
7. **Optional native adapter.** VDOCS is an additional snapshot interchange path.
   Colleaf is the inspected source application, not a core runtime dependency or
   Manifold's identity. Keep application provenance accurate and adapter internals
   named for VDOCS. Existing serialized diagnostic/profile strings stay compatible.
8. **Committed knowledge transfer.** Record actual check scopes, failures and
   limits with implementation milestones. Keep original source bytes and expected
   behavior; generated output and QA environments stay ignored.

The [acceptance audit](verification/FINAL_AUDIT.md) records original-goal evidence.
The complete chronological rationale is preserved in [history](history/DECISIONS.md),
with the former implementation maps and qualification ledger.
