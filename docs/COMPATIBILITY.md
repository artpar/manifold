# Compatibility contract

Manifold implements rich DOCX/SF/PDF and positioned PPTX conversion in pure Go. The original acceptance
scope is qualified in [the audit](verification/FINAL_AUDIT.md). This is a declared
support contract, not arbitrary-document or Word-equivalent fidelity.

| Format | Supported behavior | Explicit limits |
| --- | --- | --- |
| SF | Versioned ProseMirror tree, formatting, relationships, assets, sections/stories/notes and provenance; plain or gzip persistence | Unknown/unrepresented semantics reject; `manydoc-prosemirror/1` remains the wire identity |
| DOCX input/output | Paragraphs/headings, styled runs, links/bookmarks, lists, merged/nested/repeated tables, images, metadata, page/section settings, conditional headers/footers and rich foot/endnotes | No Word layout engine/font embedding; unsupported OOXML editing/layout semantics diagnose or reject |
| PDF input | Text/images, physical pages/geometry/navigation, supported explicit tagged structure; disclosed geometric recovery of paragraphs/headings/lists/tables/notes | Ambiguous structure stays uncertain; unsupported codecs/encodings/operators reject; no OCR or general editable vector reconstruction |
| PDF output | Measured wrapping/pagination, tables/continuations, images, section frames, stories, rich notes, links and painted logical structure | Declared font/script contract; no matching source pagination, PDF/A/UA/WTPDF certification or silent font fallback |
| PPTX/SF | Typed slide/object/text/image/notes geometry, master/layout resources; bounded native OPC preservation; supported native text/frame edits; authored basic slides | Advanced native features round-trip or reject unsupported regeneration; legacy .ppt unsupported; no PowerPoint-equivalent layout claim |
| PPTX/PDF paths | Canonical SF slide objects paint native PDF text/vectors/images; PDF text/image quads recover into slide frames | Unsupported effects/groups/charts/SmartArt reject PDF output; text overflow rejects; flow/PDF reflow and notes omission require acknowledgement; no authoring-intent reconstruction |
| JSON/text | Strict typed model JSON; explicit reduced plain text | Text intentionally omits formatting/assets |
| VDOCS | Optional checked Univer snapshot-envelope adapter | Pinned native consumer discrepancies remain documented; full browser/editor parity is not claimed |

## Interpretation and losses

DOCX uses the final visible revision view: inserted content retained, deleted text
excluded. Cached field results are retained; programs are not executed. Comments,
history, content controls, positional/layout details and unsupported constructs
have explicit diagnostics. The library is not a lossless Office editing engine.
External relationships are never fetched; macros/scripts/fields never execute.

DOCX/PDF rendering of imported content with diagnostics requires
`WriteOptions.AllowLossy`. Physical PDF provenance also requires acknowledgement
when reflow discards it. This acknowledgement is not application authorization and
does not suppress unsupported-output or missing-glyph errors.

PDF source facts retain pages, positions, paint/navigation and ownership evidence.
Explicit roles/edges and inferred structure, note relationship and note placement
are separate. Headers/footer artifact facts do not become invented authoring
stories. Unsupported graphics and color behavior have explicit diagnostics; no
universal reconstruction or hostile-parser memory guarantee is claimed.

## Fonts and Unicode

Built-in Go faces provide qualified Latin/Greek/Cyrillic coverage. Caller-supplied
TTFs and explicit family mappings support qualified additional text, including
Arabic/Hebrew shaping and bidi. Missing glyphs, guarded unsupported scripts,
incompatible graphemes and unsupported font programs reject before output.
No fake bold/italic or automatic substitution is applied by the PDF writer.
Independent Word readers may substitute installed fonts; output is laid out anew.

## Reference contracts and retained discrepancies

- [SF and editor decision](reference/STRUCTURED_FORMAT.md)
- [DOCX originals/review/fields/stories](reference/REAL_DOCX_CORPUS.md)
- [DOCX numeric precision](reference/DOCX_PRECISION.md)
- [Fonts](reference/FONT_SUBSETTING.md), [numbering](reference/NUMBERING.md),
  [tables](reference/TABLE_WIDTHS.md), [literal markers](reference/LITERAL_LIST_MARKERS.md)
- [PDF source navigation](reference/PDF_NAVIGATION.md),
  [explicit notes](reference/PDF_NOTES.md), [untagged notes](reference/PDF_UNTAGGED_NOTES.md),
  [cross-page notes](reference/PDF_NOTE_PAGES.md), [placement](reference/PDF_NOTE_KIND.md)
- [PDF streams/resources](reference/PDF_RESOURCES.md),
  [initial parser boundary](reference/PDF_PARSER_STREAMS.md),
  [raster decoding](reference/PDF_IMAGE_DECODING.md),
  [font-width ambiguity](reference/PDF_FONT_WIDTHS.md)
- [Optional VDOCS contract](reference/VDOCS_FORMAT.md) and
  [pinned native browser discrepancy](reference/VDOCS_FORMAT.md#limits)

Known CID overlap, managed CMYK/JPEG interpolation, pinned pypdf LZW and optional
native browser ordering differences retain their strict recorded oracles. Earlier
per-feature qualification and the full former compatibility narrative are kept in
[history](history/COMPATIBILITY.md); later contracts supersede old pending notes.
