# Current implementation queue

The complete original objective remains active/incomplete. ACCEPTANCE.md is the
current requirement-by-requirement audit; historical milestone narratives remain
in Git history and verification.json. Do not use an old pending statement as proof
that a feature is still missing or that a requirement can be dropped.

1. Finish and commit the ActualText typed-navigation preservation fix and its
   regression evidence. Mixed targets must not become a false single link.
2. Recover explicit tagged-PDF Note/Reference relationships and pagination-
   artifact properties. Preserve rich bodies, IDs, physical evidence and uncertainty;
   do not treat anonymous links or text retention alone as semantic recovery.
   `scripts/generate_pdf_semantic_audit.py` reproduces current gaps.
3. Implement/qualify a standalone Colleaf native Univer 1.0.4 snapshot adapter
   through SF. Read COLLEAF_FORMAT.md first: actual .vdocs data is an indexed
   IDocumentData snapshot with rich source maps and a versioned app envelope.
   No Daptin/Colleaf writes, permission/quota gates or app transport changes belong
   in this library. Preserve/reject meanings explicitly and test real reader APIs.
4. Broaden original DOCX source/producer and rich-note/running/review/field corpus
   coverage with rights, pinned bytes and independent structural/asset oracles.
   Render meaningful source/output samples; correct failures rather than edit goldens.
5. Re-audit remaining structure consistency and resource/interop costs, run the
   complete tests/race/vet/module/malformed/CGO-disabled and independent/visual
   checks, then prove every original acceptance item before completing the goal.

Current font/shaping behavior is governed by its declared contract and explicit
missing-glyph/unsupported-shaping errors. Full Word/UniOffice/UniPDF parity and
Word-equivalent pagination are outside the original completion criteria. This is
not permission to omit required paragraphs, formatting, links, lists, merged
structures, images, metadata, page/running/note semantics or meaningful PDF recovery.
