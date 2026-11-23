# Next capability work

Items 1–5 are unfinished work required by the active rich-document goal, rather
than exemptions from it. SF foundation is in STRUCTURED_FORMAT.md and the full
acceptance audit is in ACCEPTANCE.md. Items 6–7 concern separate application
integration and do not authorize changes to Daptin.

1. Qualify DOCX output visually in Microsoft Word/LibreOffice, using a published
   corpus of real exported documents and intended deployment fonts. Compare text,
   structure, and appearance separately.
2. Extend the implemented family/size/color fields with full theme/script/embedded
   font recovery, shaping/bidi/glyph output and paragraph properties; font-slot/
   theme-language resolution now preserves current spans, while broader selection
   qualification/modern ranges remain required; calculated run theme tint/shade
   and settings color mappings now exist; then asymmetric
   margins, sections, richer note numbering/separator/layout behavior, bookmarks, comment/revision history and
   broader numbering formats. Each feature needs import, model, export and tests;
   avoid a second authoritative representation of the same content.
3. Extend the new Unicode-space/NFC/grapheme wrapping foundation with Go text shaping/bidi and configurable licensed fonts for broader PDF
   generation. Preserve explicit missing-glyph errors while this is incomplete.
4. Expand forward tagged-PDF recovery and the implemented untagged whitespace/heading/list/numeric/painted-grid table inference (including merges/empty/multiline cells) with more complex/double-border/occluded/rotated layouts; validate inverse ParentTree and internal links. Expand CMaps,
   vertical text and remaining image codecs/masks/colors. Editable family/physical
   size/device-color recovery now has source evidence and independent span checks;
   broader source-font/color-management coverage remains required. A successful extraction must not imply
   reconstruction of editable Word structure.
5. Extend recursive body/nested/footnote continuation with broader pagination
   qualification; repeated header prefixes now exist across those contexts; full-container image fitting, native explicit breaks
   and conditional paragraph starts now exist. Production DOCX encoding recovers
   the previously omitted note side texts in LibreOffice and retains SF controls;
   cell/note pagination still differs. Qualify broader Word/producer behavior and
   real exports without treating that content mitigation as professional fidelity.
6. Build the Colleaf .vdocs adapter once its actual schema and supported features
   are supplied or inspected. Do not invent a second native editor schema.
7. Integrate thin Daptin performers around this public library. Keep Daptin's
   existing action admission, resource/storage operations and identity context.

Use `docs/STATUS.md` as the current handoff and `docs/TESTING.md` for the commands.
Record compatibility changes and reference-corpus results before claiming a
broader professional-document contract.


Glyph-ID output now exists for supported LTR graphemes, using full TrueType
programs, shared CID resources, ActualText/ToUnicode and checked GPOS offsets.
Language/direction/forced-CS semantics and language-tagged LTR segment/line shaping
now exist. The next slice needs paragraph bidi/joining and full itemization,
context across differing fonts/sizes/languages and within-grapheme styles. Ordinary default text now uses
the same LTR shaping pipeline. Reuse the transport; keep current unsupported-script guards until
independent glyph/order/layout evidence qualifies their replacement. Full-font
cost, generalization beyond the implemented compatible-face/size/language LTR line re-shaping,
style/font segmentation and internal text/link/note bidi order need deliberate
handling. Source-font recovery,
broader formats/qualification and other acceptance rows remain required.


Paint-only boundaries now preserve shared LTR context, including clipped ligature
colors, exact logical carriers and URI geometry. Keep the eight-row independent
paint-context oracle while implementing paragraph bidi/itemization. Broader
font/size/language and within-grapheme style context remain open; do not reintroduce
paint-based shaping segments or the observed MuPDF missing-f defect.


The qualified internal/bidilevels resolver now supplies paragraph levels and
line L1/L2 maps. Next carry original scalar ranges through normalization and
textAtoms, split shaping contexts by resolved level/script/font, shape final
lines in their resolved direction, and place units using L2 while retaining logical
extraction. Qualify L3/L4/paint/link/note behavior before removing existing RTL/
joining guards. Unicode 15 algorithm conformance is not renderer qualification.
