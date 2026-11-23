# Next capability work

Items 1–5 are unfinished work required by the active rich-document goal, rather
than exemptions from it. SF foundation is in STRUCTURED_FORMAT.md and the full
acceptance audit is in ACCEPTANCE.md. Items 6–7 concern separate application
integration and do not authorize changes to Daptin.

1. Qualify DOCX output visually in Microsoft Word/LibreOffice, using a published
   corpus of real exported documents and intended deployment fonts. Compare text,
   structure, and appearance separately.
2. Extend the model deliberately for font families/sizes/colors, asymmetric
   margins, sections, richer note numbering/separator/layout behavior, bookmarks, comment/revision history and
   broader numbering formats. Each feature needs import, model, export and tests;
   avoid a second authoritative representation of the same content.
3. Extend the new Unicode-space/NFC/grapheme wrapping foundation with Go text shaping/bidi and configurable licensed fonts for broader PDF
   generation. Preserve explicit missing-glyph errors while this is incomplete.
4. Expand forward tagged-PDF recovery and the implemented untagged whitespace/heading/list/numeric/painted-grid table inference (including merges/empty/multiline cells) with more complex/double-border/occluded/rotated layouts; validate inverse ParentTree and internal links. Expand CMaps,
   vertical text and remaining image codecs/masks/colors. A successful extraction must not imply
   reconstruction of editable Word structure.
5. Extend recursive body/nested/footnote continuation with oversized images,
   repeated headers and broader pagination qualification; native explicit breaks
   now exist. Prioritize DOCX paragraph break-property import/export qualification:
   a QA pageBreakBefore variant recovers note side texts omitted by LibreOffice's
   rendering of break runs. Preserve SF control events and disclose the visual
   failure until the mitigation and broader Word behavior are qualified.
6. Build the Colleaf .vdocs adapter once its actual schema and supported features
   are supplied or inspected. Do not invent a second native editor schema.
7. Integrate thin Daptin performers around this public library. Keep Daptin's
   existing action admission, resource/storage operations and identity context.

Use `docs/STATUS.md` as the current handoff and `docs/TESTING.md` for the commands.
Record compatibility changes and reference-corpus results before claiming a
broader professional-document contract.
