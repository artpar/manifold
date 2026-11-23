# Next capability work

This list describes product work beyond the initial implementation. It is not
evidence that the current library supports these features.

1. Qualify DOCX output visually in Microsoft Word/LibreOffice, using a published
   corpus of real exported documents and intended deployment fonts. Compare text,
   structure, and appearance separately.
2. Extend the model deliberately for font families/sizes/colors, asymmetric
   margins, sections, note references, bookmarks, comment/revision history and
   vertical table merges. Each feature needs import, model, export and tests;
   avoid a second authoritative representation of the same content.
3. Add Go text shaping/bidi and configurable licensed fonts for broader PDF
   generation. Preserve explicit missing-glyph errors while this is incomplete.
4. Expand PDF extraction for inline images, more CMaps, coordinate-aware reading
   order and text/graphics transforms. A successful extraction must not imply
   reconstruction of editable Word structure.
5. Improve table layout: long-row splitting, repeated header rows, nested/nontext
   cells, and measured pagination tests.
6. Build the Colleaf .vdocs adapter once its actual schema and supported features
   are supplied or inspected. Do not invent a second native editor schema.
7. Integrate thin Daptin performers around this public library. Keep Daptin's
   existing action admission, resource/storage operations and identity context.

Use `docs/STATUS.md` as the current handoff and `docs/TESTING.md` for the commands.
Record compatibility changes and reference-corpus results before claiming a
broader professional-document contract.
