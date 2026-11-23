"""Optional independent readers for the SF-first list example."""
from pathlib import Path
from zipfile import ZipFile
import argparse
import math
from docx import Document
from lxml import etree
from pypdf import PdfReader
import pymupdf

parser = argparse.ArgumentParser()
parser.add_argument("directory", nargs="?", type=Path, default=Path("output/lists"))
parser.add_argument("--docx-render", type=Path)
parser.add_argument("--expected-pages", type=int, default=1)
args = parser.parse_args()
root = args.directory
ns = {"w":"http://schemas.openxmlformats.org/wordprocessingml/2006/main"}
doc = Document(root / "lists.docx")
with ZipFile(root / "lists.docx") as package:
    numbering = etree.fromstring(package.read("word/numbering.xml"))
    xml = etree.fromstring(package.read("word/document.xml"))
    assert not xml.xpath("//w:sdt", namespaces=ns), "list grouping must not introduce content controls"
    assert not xml.xpath("//w:instrText", namespaces=ns), "unexpected visible field instruction"
    definitions = {}
    for num in numbering.xpath("w:num", namespaces=ns):
        definitions[num.get("{%s}numId" % ns["w"])] = int(num.xpath("string(w:lvlOverride/w:startOverride/@w:val)", namespaces=ns))
    assert list(definitions.values()) == [3, 1, 7, 0, 7, 10]
    paragraphs = xml.xpath("//w:p[w:pPr/w:numPr]", namespaces=ns)
    values = []
    for p in paragraphs:
        number_id = p.xpath("string(w:pPr/w:numPr/w:numId/@w:val)", namespaces=ns)
        level = int(p.xpath("string(w:pPr/w:numPr/w:ilvl/@w:val)", namespaces=ns))
        values.append((number_id, level))
    assert values == [("1",0),("2",1),("2",1),("3",2),("1",0),("4",0),("4",0),("5",0),("6",0),("6",0)]
    bookmark_names = xml.xpath("//w:bookmarkStart/@w:name", namespaces=ns)
    assert len(bookmark_names) == len(set(bookmark_names)) == 16
    assert sum(n.startswith("mdlist1_") for n in bookmark_names) == 6
    first_item = xml.xpath("//w:bookmarkStart[starts-with(@w:name,'mditem1_')]", namespaces=ns)[0]
    # Its bookmark range contains a numbered paragraph, continuation, nested
    # lists, and the final continuation; these remain ordinary DOCX blocks.
    assert first_item.getnext().tag == "{%s}p" % ns["w"]
    link = xml.xpath("//w:hyperlink", namespaces=ns)[0]
    rid = link.get("{http://schemas.openxmlformats.org/officeDocument/2006/relationships}id")
    assert doc.part.rels[rid].target_ref == "https://example.invalid/list"
    assert link.xpath("w:r/w:rPr/w:b", namespaces=ns)
print("DOCX: six list identities, starts, hierarchy, 10 numbered items, invisible boundaries and link/bold checked")

markers = ["Parent three", "Continuation paragraph", "Nested bullet one", "Nested bullet two", "Deep seven", "After nested list", "Parent four", "Restart zero", "Then one", "Separate seven", "Cell ten", "Cell eleven"]
pdf = PdfReader(root / "lists.pdf")
assert len(pdf.pages) == args.expected_pages
text = "\n".join(p.extract_text() for p in pdf.pages)
for marker in markers:
    assert text.count(marker) == 1, marker
assert "HYPERLINK" not in text
for number in ["3.", "4.", "0.", "1.", "7.", "10.", "11."]:
    assert number in text
native = pymupdf.open(root / "lists.pdf")
def find(word):
    matches = [rect for page in native for rect in page.search_for(word)]
    assert matches, word
    return matches[0]
positions = {word:find(word).x0 for word in ["Parent three", "Continuation paragraph", "Nested bullet one", "Deep seven", "Parent four", "Restart zero", "Separate seven"]}
for word in ["Parent three", "Continuation paragraph", "Parent four", "Restart zero", "Separate seven"]:
    assert math.isclose(positions[word], 54, abs_tol=0.1), (word, positions[word])
assert math.isclose(positions["Nested bullet one"], 72, abs_tol=0.1)
assert math.isclose(positions["Deep seven"], 90, abs_tol=0.1)
assert math.isclose(find("Cell ten").x0, find("Cell eleven").x0, abs_tol=0.1)
assert any(a.get_object().get("/A", {}).get("/URI") == "https://example.invalid/list" for page in pdf.pages for a in page.get("/Annots", []))
print(f"PDF: {len(pdf.pages)} pages, content, starts, three nesting positions, continuation indentation, cell list and link checked")

if args.docx_render:
    rendered = PdfReader(args.docx_render)
    assert len(rendered.pages) == 1
    visible = rendered.pages[0].extract_text()
    for marker in markers:
        assert visible.count(marker) == 1, marker
    assert "HYPERLINK" not in visible, "DOCX renderer exposed hyperlink instructions"
    for number in ["3.","4.","0.","1.","7.","10.","11."]:
        assert number in visible
    print("DOCX-rendered PDF: every item and numbering start retained without exposed field instructions; inspect PNG separately")
