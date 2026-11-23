# Compatibility and fidelity

This initial implementation targets explicit, testable document operations.
Passing the included corpus is not a guarantee for arbitrary customer files.

## Format support

| Operation | Implemented behavior | Important limits |
| --- | --- | --- |
| DOCX read | Paragraphs/headings, bold/italic/underline/strike, links, lists, tables/nested tables/column spans, embedded images, metadata, headers/footers, notes | Model is smaller than OOXML; diagnostics describe known representation losses |
| DOCX write | Paragraphs/headings/runs/links, lists, tables/nesting/column spans, PNG/JPEG images, metadata, one page layout and header/footer | Notes/reference generation is rejected; font embedding and Word layout are not provided |
| PDF read | Page-separated text, ToUnicode bfchar/bfrange/codespaces, surrogate pairs/multiple-codepoint mappings, common simple encodings, supported UCS2 CJK encodings, Form XObjects, widget appearances | Content-stream order; no OCR or editable Word structure; unsupported encodings/operators return errors |
| PDF write | Flowing text/headings/lists, run formatting/links, text tables with column spans, PNG/JPEG images, page breaks, metadata, text headers/footers | Embedded Go font coverage; complex shaping/bidi rejected; nested/nontext table cells and rows taller than a page rejected |
| JSON | Public model with base64 image data and diagnostics | Unknown JSON fields rejected; this is manydoc's model, not Colleaf's .vdocs schema |
| Text | Body/table/note text; form feeds represent page breaks | Formatting and binary image data are not represented |

PDF input was tested with RC4-40, RC4-128, AES-128 and AES-256 passwords, object
streams, incremental updates, and a PDF 2.0 header. Unsupported security handlers
and arbitrary newer PDF features are not implied by those results.

## DOCX interpretation

Inserted revision content is retained and deleted content excluded, producing
the final visible view. Revision history is not preserved. Content-control text,
cached field results and text-box text are retained, with diagnostics for their
unrepresented control/evaluation/layout semantics.

Bookmark destinations, comment annotations, vertical table merges, multiple
section layouts and asymmetric margins are not fully represented. Notes are
extracted separately; their inline references are represented textually and
diagnosed. Unknown body blocks are reported as unsupported, rather than claimed
as supported. The reader is not a lossless OOXML editor: fonts, sizes, colors,
themes, paragraph positioning and arbitrary style properties are outside the
current model. The declared formatting subset is bold/italic/underline/strike,
headings, lists and links.

External relationships are never fetched. DOCX/PDF output never executes macros,
scripts or field programs. DOCX font substitution depends on the reader's fonts;
the writer does not embed fonts. Word visual pagination remains unverified.

## PDF interpretation and output

PDF extraction always reports that editable layout was not reconstructed.
Scanned/image-only pages produce no OCR text. Inline PDF image data is currently
rejected; external image XObjects are skipped during text extraction. Arbitrary
spatial reading order, tagged-PDF semantics, tables, vertical text, rendering
equivalence and complex graphics transforms are not reconstructed.

PDF output lays out the model anew. It does not preserve imported DOCX line/page
positions. The built-in Go fonts support the tested Latin/Greek/Cyrillic output;
missing glyphs return errors before output. CJK can be extracted from supported
PDF encodings but cannot currently be generated with these built-in fonts.
Arabic/Hebrew/Devanagari output needs a shaping/bidi implementation and is
explicitly rejected. The output is not claimed to be PDF/A or tagged PDF.

## Conversion acknowledgement

Readers return `Document.Diagnostics`. Writing imported content to DOCX/PDF with
diagnostics returns `ErrLossy` unless `WriteOptions.AllowLossy` is true. This is a
fidelity acknowledgement, not authorization. The option does not suppress
unsupported-feature or missing-glyph errors.

Applications such as Daptin/Colleaf retain authority over identity, permission,
storage, actions and quotas. No Daptin integration or .vdocs adapter is included
in this project yet.
