# Detailed reference contracts

These documents record format mappings, representation losses, supported subsets
and scoped qualification evidence. Start with [current compatibility](../COMPATIBILITY.md)
and [testing](../TESTING.md); some chronological paragraphs describe earlier
milestones and do not supersede the current acceptance decision.

- SF_CODEC / STRUCTURED_FORMAT — persistence, schema, manipulation and codec limits
- DOCX_PRECISION / REAL_DOCX_CORPUS — OOXML precision and original editor exports
- PDF_* — extraction, explicit/inferred structure, notes, images, navigation and bounds
- FONT_SUBSETTING / NUMBERING / TABLE_WIDTHS — layout and formatting contracts
- VDOCS_FORMAT / VDOCS_BROWSER — optional native adapter and retained reader discrepancy
- RENAME — public project identity versus persisted compatibility identifiers

Older root Go filenames map through
[history/source-file-map.json](../history/source-file-map.json). Private engine tests
now run against `./internal/engine`; public API tests remain at the module root.
Original source bytes, independent oracles and scoped failures are retained.
