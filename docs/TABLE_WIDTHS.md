# Preferred table grid widths

`Block.ColumnWidths` / SF table `attrs.columnWidths` stores one positive finite
point width per logical grid column, including columns crossed by merged cells.
An absent/empty array keeps equal available columns. Explicit preferences retain
their total width when they fit and shrink proportionally when the containing
body, cell, note or running story is narrower. They do not grow to fill a larger
container. The total is bounded to 10 million points; count must match the shared
logical grid. Non-table width attributes, zero/negative/nonfinite widths and
count mismatches reject. No runtime dependency was added.

This is an optional extension to the existing ProseMirror profile, not a new
canonical format. [ProseMirror tables](https://github.com/ProseMirror/prosemirror-tables/blob/master/src/columnresizing.ts)
uses per-cell `colwidth` arrays in CSS pixels for interactive resizing. A single
physical grid avoids conflicting cell declarations and preserves subpixel/twip
columns without integer pixel rounding. An editor adapter must convert points to
CSS pixels (96/72) and distribute these widths over cell spans; direct stock
column-resizer mutation is not an established integration. Colleaf integration
still needs its actual schema.

## Import and export

DOCX reads [`w:tblGrid/w:gridCol/@w:w`](https://learn.microsoft.com/en-us/dotnet/api/documentformat.openxml.wordprocessing.gridcolumn.width?view=openxml-3.0.1)
initial preferences in twips, including narrow columns that disappear inside
horizontal merges. Incomplete/unspecified grids disclose equal-column fallback.
Automatic Word resizing and conflicting table/cell preferred widths have explicit
`table_autofit`, `table_width_preference`, `table_cell_width_preference` diagnostics.
The grid is preserved; the full Word auto-fit algorithm is not claimed. Table
shading and other source formatting remain separate unfinished fidelity work.

DOCX writes fixed standard grids, table/cell widths and merged-cell sums. Existing
unspecified equal-width output stays equal. Metadata also records the available
container so import recomputes the exact twip quantization rather than using a
rounded table total; contradictory metadata rejects. Cumulative rounding fits the grid to
whole twips; any column that would disappear rejects output. Ignorable
`md:columnWidths` and `md:columnWidthContainer` retain the original preference
before fitting/quantization (and distinguish implicit equal sizing). Import
recomputes the serialized grid from both values and requires exact agreement.
Metadata cannot suppress visible cell content. An external editor stripping the
metadata leaves the actual standard OOXML widths available for import.

PDF measurement, full groups, recursive continuation and repeated headers use
one cumulative boundary vector. Nested tables use their actual merged-cell
width. Painted-grid PDF recovery stores measured widths with
`source.table_widths` (`painted_grid_adjacency`, confidence 0.7, physical bounds).
That evidence is independent of explicit logical table membership. Widths remain
unknown if no unique grid matches; partial/conflicting evidence does not invent
widths. They describe painted geometry rather than recovered author intent.

## Verification and limits

The unchanged original OASIS Word fixture has 59 grids; independent ZIP/XML
inspection matches every SF width to original twips / 20. Table 51 retains all
17 axes `[1543,3587,20,501,522,19,503,18,504,17,505,16,506,15,507,19,548]`
in twips. Its merged description cell is 180.35pt, rather than one equal share.
The isolated actual table renders on one page, with a readable description and
all 79 origins. It was visually inspected, as was its full-document native page
120. Source fixtures and existing structural/text/hash oracles were not changed.

The full native source render now has 189 pages (previously 207), with font,
434 URI target, original JPEG, metadata, Han and figure-dash checks passing.
This count is an observation, not a fidelity target: the publisher PDF has 137
pages, and numbering, fields, paragraph formatting and other source behavior
still differ. The complete goal remains unfinished.

The all-four-path example has four native PDF pages with repeated headers,
vertical/horizontal merges, explicit shorter-than-container width, and a nested
300pt preference fitted into a 255pt merged cell. Independent OOXML checks find
900/3600/1500 twip outer columns and 1482/3458 nested columns (247pt after DOCX
margins). MuPDF checks boundaries 40/85/265/340pt on every native page and nested
91/163.9/334pt boundaries; recovered page-local tables retain inferred width
provenance. Actual ProseMirror checks 1,093 nodes/marks/attributes across three
example trees plus the isolated original table. All four native pages and all
three LibreOffice-rendered DOCX pages were visually inspected. LibreOffice uses
its own padding/split policy and flattens the nested table into its outer grid;
this is reader qualification, not visual equivalence or a production dependency.

Go checks cover literal original width data, strict admission, altered metadata,
120 deterministic small/oversized fractional-preference DOCX round trips,
explicit membership versus inferred width provenance, and absence of invented
widths without ruling paint. See STATUS/verification.json for completed checks.
