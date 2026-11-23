"""Independent checks for examples/merged-tables; optional QA dependencies only."""
from pathlib import Path
import math
import argparse
from docx import Document
from pypdf import PdfReader
from pypdf.generic import ContentStream

parser = argparse.ArgumentParser()
parser.add_argument("directory", nargs="?", type=Path, default=Path("output/merged-tables"))
parser.add_argument("--docx-render", type=Path, help="PDF freshly rendered from DOCX by an independent application")
args = parser.parse_args()
root = args.directory
doc = Document(root / "merged-table.docx")
assert len(doc.tables) == 1
b = doc.tables[0]
assert len(b.rows) == 4 and len(b.columns) == 4
for r, c in [(0, 1), (1, 0), (1, 1)]:
    assert b.cell(r, c)._tc is b.cell(0, 0)._tc
for r in [1, 2]:
    assert b.cell(r, 2)._tc is b.cell(0, 2)._tc
assert b.cell(0, 0).text == "Merged A"
assert b.cell(0, 2).text == "Merged B"
assert b.cell(1, 3).text == "Right D"
assert b.cell(2, 0).text == "Bottom E"
assert b.cell(3, 0)._tc is b.cell(3, 3)._tc
assert b.cell(3, 0).text == "Full G"
link = b.cell(0, 0).paragraphs[0].hyperlinks[0]
assert link.url == "https://example.invalid/merge"
assert link.runs[0].bold
print("python-docx checked merged-cell identities, content, formatting and relationship")

pdf = PdfReader(root / "merged-table.pdf")
assert len(pdf.pages) == 1
page = pdf.pages[0]
rectangles = [list(map(float, values)) for values, op in ContentStream(page.get_contents(), pdf).operations if op == b"re"]
expected = [[30, 270, 150, -52], [180, 270, 75, -78], [255, 270, 75, -26],
            [255, 244, 75, -26], [30, 218, 150, -26], [255, 218, 75, -26], [30, 192, 300, -26]]
assert len(rectangles) == len(expected)
for actual, wanted in zip(rectangles, expected):
    assert all(math.isclose(a, w, abs_tol=0.01) for a, w in zip(actual, wanted)), (actual, wanted)
text = page.extract_text()
for marker in ["Merged A", "Merged B", "Right C", "Right D", "Bottom E", "Right F", "Full G"]:
    assert text.count(marker) == 1, marker
assert any(a.get_object().get("/A", {}).get("/URI") == "https://example.invalid/merge" for a in page["/Annots"])
print("pypdf checked seven cell rectangles, exact spanning geometry, text and link annotation")

if args.docx_render:
    rendered = PdfReader(args.docx_render)
    assert len(rendered.pages) == 1
    visible = rendered.pages[0].extract_text()
    for marker in ["Merged A", "Merged B", "Right C", "Right D", "Bottom E", "Right F", "Full G"]:
        assert visible.count(marker) == 1, marker
    print("Independent DOCX-rendered PDF retained all seven cell texts on one page; inspect its PNG separately")
