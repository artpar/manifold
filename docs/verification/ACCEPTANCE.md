# Original goal acceptance

Audited 2026-11-25. **The original goal is complete for the declared support
contract.** [FINAL_AUDIT.md](FINAL_AUDIT.md) maps every original requirement to
implementation, original-source oracles, independent readers and visual evidence.
It distinguishes the required capabilities from optional third-party editor work
and from the explicitly excluded Word layout/API parity goals.

All four paths use manydoc-prosemirror/1: DOCX→SF, SF→DOCX, SF→PDF and PDF→SF.
Rich structure, formatting, relationships, assets, page settings, running content
and notes are represented and tested. PDF physical facts and inferred semantics
remain separate. Revisions/comments/fields and unsupported shaping/glyphs have
explicit policies. Public Go APIs, CLI and usable examples run in-process without
CGO, required executables/services/license servers, application authorization or
proprietary UniDoc implementation code.

Completion includes the representative original exports and targeted fixtures,
not just successful parsing or matching plain text. The fresh audit reproduced
and fixed an actual original-source ParentTree regression; PDF_NOTE_KIND.md
records the unchanged source, exact prior failure and current independent proof.

Final normal tests pass 75.053s; full final-production race passes 770.544s with
-timeout20m and no races. CLI/internal bidi results are cached. Later test-only
additions pass focused race 7.016s. Vet, module verification, malformed/fuzz checks
and CGO-disabled host/Linux builds pass. Linux builds are compilation only.
Exact scopes are in verification.json and FINAL_AUDIT.md.

Known limits are explicit in COMPATIBILITY.md and FINAL_AUDIT.md. They include
unsupported scripts/codecs/graphics/editing semantics, uncertainty in ambiguous
PDFs and no universal initial-parser memory cap. Retained CID/color/reader and
optional Colleaf browser discrepancies remain documented and reproducible; none
is substituted for a missing required conversion capability. Complete browser
parity and Word-equivalent pagination are not original acceptance gates.

For maintenance, start with STATUS.md and rerun the qualification commands in
FINAL_AUDIT.md after relevant changes. Keep source bytes/oracles unchanged unless
new independent source evidence justifies a correction. Preserve scoped failures,
update KT/evidence and commit verified changes together. Earlier incomplete
acceptance checkpoints remain in Git history and verification.json.
