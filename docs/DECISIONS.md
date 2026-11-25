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
   The adapter targets a pinned Univer snapshot profile and is named for VDOCS.
   It introduces no required editor service or host application dependency.
8. **Committed knowledge transfer.** Record actual check scopes, failures and
   limits with implementation milestones. Keep original source bytes and expected
   behavior; generated output and QA environments stay ignored.

The [acceptance audit](verification/FINAL_AUDIT.md) records original-goal evidence.
The complete chronological rationale is preserved in [history](history/DECISIONS.md),
with the former implementation maps and qualification ledger.


9. **Application-independent native format.** The user requested removal of host
   application references, then explicitly confirmed keeping VDOCS. Retain SF as
   the canonical hub for DOCX/PDF/VDOCS conversions. Use a Manifold-owned VDOCS
   envelope/MIME and native diagnostic namespace; remove host validators and their
   browser harness. Keep SDK reader/edit/save checks and the unchanged native
   document payload. Retired branded envelopes require explicit header migration.


10. **Automated delivery.** The user requested CI/CD instead of workstation
    releases. GitHub Actions gates publication on native OS checks and Linux race
    checks. A reviewed VERSION bump on main creates the tag, archives, checksums
    and GitHub release automatically. Release write permission is job-scoped;
    published versions are not force-moved or overwritten on ordinary pushes.

11. **Integrated presentations.** At the user's request, incorporate Puppt source
    under internal/presentation with a public presentation facade and namespaced
    CLI. Preserve original format oracles, diagnostic contracts and puppt.v1 report
    compatibility. Do not flatten slide geometry into document SF or advertise
    cross-format conversion without an explicit mapping and qualification.
    Use the existing CI/release pipeline and retain embedded font/dependency notices.

12. **Measured performance boundaries.** Use a pure-Go public-API load harness
    with immutable inputs, request-owned state, bounded overload queues, latency
    histograms, allocation/GC measurements and separate profiling/race runs. Report
    hardware/workload/limits and successful jobs per second; do not present embedded
    operation throughput as an unmeasured HTTP service capacity or fidelity claim.

13. **One SF hub includes presentations.** The earlier facade-only integration
    did not satisfy the user's intended scope. Add a typed, optional presentation
    extension to the existing ProseMirror SF attributes and root reader/writer API.
    Reuse SF rich-content nodes in slide objects/notes and retain native source
    parts as provenance/preservation data, not as a competing interchange format.
    Native PDF output must paint text/vector/image objects; no hidden whole-slide
    bitmap conversion. Unsupported native features must round-trip unchanged or
    reject unsupported edits/output explicitly. Performance results must be rerun
    for this completed path rather than advertised for the superseded facade.

14. **Request-owned PDF shaping cache.** The measured old shaping path inserted
    a freshly parsed font face into a process-global upstream cache on every PDF
    request. Use the already-pinned go-text/typesetting backend, whose plan cache
    is owned by the shaping buffer, and reuse one buffer per request-owned glyph
    face. Do not add a permanent process-wide font cache or lifetime font quotas.
    Preserve existing glyph/ligature/bidi/caret expectations and independently
    rerun load/heap qualification after the change.

15. **Real originals for huge-input qualification.** The user explicitly rejected
    synthetic huge-file evidence. Stop that scout and remove huge fixture generation.
    Download unmodified publisher documents, pin source URLs and SHA-256/size, and
    independently count pages/XML structure. Keep publisher bytes untracked. Count
    unsupported input, bounded-work rejection, memory-guard kills and timeouts as
    failures, never successful capacity; do not weaken fidelity or production work
    limits merely to complete a benchmark. Isolate extreme jobs in guarded processes.

16. **Preserve native source content discovered by huge originals.** A successful
    import counter is insufficient: validate original diagrams/equations as well
    as text. Retain WMF/EMF previews byte-for-byte and native OMML in typed SF blocks;
    reject unsupported rendering instead of flattening equations into incomplete
    text. Preserve unmapped embedded CFF outlines as diagnosed span images without
    inventing Unicode. Keep conservative default work limits; opt-in huge policies,
    soft Go heap targets and external RSS guards are separate measured controls.


17. **Qualified huge imports, explicit limits and honest capacity.** Fix the seven
    original blockers through source-backed interpretation and bounded lookup/cache
    improvements, not by skipping pages/assets, disabling tags, replacing source
    files or weakening defaults. Preserve native math/previews through the SF hub;
    reject unsupported rendering. Keep valid ActualText characters and original
    unrepresentable metadata bytes, and resolve placeholders only from declared
    glyph encodings. Require exact output hashes, independent content/consumer
    audits and separate regression/control runs. Publish observed per-workload
    conversion jobs/s; retain poor concurrency scaling and default-policy rejections
    alongside successes. Do not advertise HTTP/service or universal visual parity.
    v0.4.0 publication continues through the existing gated VERSION workflow.

- 2026-11-25: Investigate huge-original CPU and memory with stage-separated pprof
  runs before selecting optimizations. Separate CPU runs from forced-GC heap runs;
  distinguish cumulative allocation churn, live model retention and post-release
  residue. Preserve original hashes, large-policy limits and process RSS/deadline
  controls. Profiling remains development tooling, with no production codec change.

- 2026-11-25: Cross-check pprof attribution with concrete counters and controlled
  A/Bs. The ECMA caller-buffer control reduces file writes from 1,002,226 to 3,935
  but write time only 63.37s to 61.17s in one pair. Do not promote syscall samples
  into CPU accounting or latency savings. Prioritize measured layout/PNG/model
  allocation and repeated SF encoding; retain negative/modest experimental results.

- 2026-11-25: Completed pprof evidence ranks page-local layout index/scratch work
  first, strict SF key/decoder pass reduction next, and bounded PNG reuse before
  speculative global caching. Source metadata and mutable model ownership remain
  contractual. All 12 diagnostic outputs retain qualified hashes; codec changes
  are next work, not an optimization already delivered. See HUGE_PPROF.md and its
  curated results, with captured profiles kept outside version control.

- Performance change 1: preserve the frozen layout algorithm's decisions while
  moving recursive work to integer indices; validate stable ties/column labels
  against the previous implementation and pinned original SF hashes.
