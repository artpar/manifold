"""Independent output check; run after go run ./examples/report -out output.

Requires python-docx, pypdf, and Poppler's pdftotext/pdfinfo (test tools only).
"""
from pathlib import Path
import json
import subprocess
import sys
from docx import Document
from pypdf import PdfReader

root = Path(sys.argv[1] if len(sys.argv) > 1 else "output")
doc = Document(root / "report.docx")
body = "\n".join(p.text for p in doc.paragraphs)
for marker in ["Quarterly report", "formatted text", "Unicode text: café Ελληνικά Привет", "Second page"]:
    assert marker in body, ("DOCX missing", marker)
assert len(doc.tables) == 1
assert doc.tables[0].cell(1, 0).text == "Colleaf"
assert doc.tables[0].cell(1, 1).text == "42"
assert len(doc.inline_shapes) == 1
assert "Example organization" in doc.sections[0].header.paragraphs[0].text
assert "Prepared with manydoc" in doc.sections[0].footer.paragraphs[0].text
assert any(r.bold for p in doc.paragraphs for r in p.runs)
assert any(len(p.hyperlinks) for p in doc.paragraphs)
assert doc.core_properties.title == "Quarterly report"

pdf = PdfReader(root / "report.pdf")
assert len(pdf.pages) == 2
text = subprocess.check_output(["pdftotext", str(root / "report.pdf"), "-"], text=True)
for marker in ["Quarterly report", "formatted text", "links", "Ελληνικά", "Привет", "Colleaf", "42", "Second page", "Prepared with manydoc"]:
    assert marker in text, ("PDF missing", marker)
assert text.count("Example organization") == 2
result = {"docx_independent_reader": "passed", "pdf_poppler_text": "passed", "pdf_pages": len(pdf.pages),
          "docx_tables": len(doc.tables), "docx_images": len(doc.inline_shapes),
          "word_visual_layout": "not verified by this script"}
(root / "independent-verification.json").write_text(json.dumps(result, indent=2) + "\n")
print(json.dumps(result, indent=2))
