"""Optional independent OOXML and PDF note checks; no runtime dependency."""
from pathlib import Path
from zipfile import ZipFile
import argparse
import collections
import math
from docx import Document
from lxml import etree
from pypdf import PdfReader
import pymupdf

parser = argparse.ArgumentParser()
parser.add_argument("directory", nargs="?", type=Path, default=Path("output/notes"))
parser.add_argument("--docx-render", type=Path)
parser.add_argument("--continued", action="store_true")
args = parser.parse_args()
root = args.directory
ns = {"w":"http://schemas.openxmlformats.org/wordprocessingml/2006/main", "md":"urn:manydoc:metadata:1", "r":"http://schemas.openxmlformats.org/package/2006/relationships"}
Document(root / "notes.docx")  # independent package reader
with ZipFile(root / "notes.docx") as package:
    main = etree.fromstring(package.read("word/document.xml"))
    foot = etree.fromstring(package.read("word/footnotes.xml"))
    end = etree.fromstring(package.read("word/endnotes.xml"))
    fn = foot.xpath("w:footnote[not(@w:type)]", namespaces=ns)
    en = end.xpath("w:endnote[not(@w:type)]", namespaces=ns)
    assert len(fn) == 2 and len(en) == 1
    assert [n.get("{%s}id" % ns["w"]) for n in fn] == ["1","2"]
    assert [n.get("{%s}originalId" % ns["md"]) for n in fn] == ["alpha &<42>","shared"]
    assert en[0].get("{%s}originalId" % ns["md"]) == "shared"
    assert [n.get("{%s}order" % ns["md"]) for n in [en[0],*fn]] == ["0","1","2"]
    assert main.xpath("//w:footnoteReference/@w:id", namespaces=ns) == ["1","2"]
    assert main.xpath("//w:endnoteReference/@w:id", namespaces=ns) == ["1"]
    fields = main.xpath("//w:fldSimple", namespaces=ns)
    assert len(fields) == 1 and "NOTEREF mdnote1_footnote_1" in fields[0].get("{%s}instr" % ns["w"])
    assert fields[0].xpath("string(w:r/w:t)", namespaces=ns) == "1"
    assert main.xpath("//w:bookmarkStart[@w:name='mdnote1_footnote_1']", namespaces=ns)
    content_types = etree.fromstring(package.read("[Content_Types].xml"))
    types = {n.get("PartName"):n.get("ContentType") for n in content_types}
    for name in ["footnotes","endnotes"]:
        assert types["/word/"+name+".xml"].endswith("wordprocessingml."+name+"+xml")
    rels = etree.fromstring(package.read("word/_rels/document.xml.rels"))
    assert {r.get("Target") for r in rels if r.get("Type").endswith(("/footnotes","/endnotes"))} == {"footnotes.xml","endnotes.xml"}
    if not args.continued:
        assert foot.xpath("//w:r[w:t[contains(.,'FIRST_NOTE_BODY')]]/w:rPr/w:b", namespaces=ns)
        links = etree.fromstring(package.read("word/_rels/footnotes.xml.rels"))
        assert any(n.get("Target") == "https://example.invalid/note" and n.get("TargetMode") == "External" for n in links)
        assert len(foot.xpath("//w:numPr", namespaces=ns)) == 1
print("DOCX: three note definitions, four reference sites, scoped public IDs, original order, types/relationships and cached repeat checked")

pdf = PdfReader(root / "notes.pdf")
visible = "\n".join(p.extract_text() for p in pdf.pages)
for marker in ["SECOND_NOTE_BODY", "END_NOTE_BODY", "Repeated reference", "Body text remains"]:
    assert visible.count(marker) == 1, marker
native = pymupdf.open(root / "notes.pdf")
links = [link for page in native for link in page.get_links()]
internal = [link for link in links if link["kind"] == pymupdf.LINK_GOTO]
assert len(internal) == 4
assert sorted(link["page"] for link in internal) == [0,0,next(i for i,p in enumerate(native) if p.search_for("SECOND_NOTE_BODY")),len(native)-1]
assert visible.count("Typed note references and independent note bodies") == len(native)
for page in native:
    note_lines = []
    for block in page.get_text("dict")["blocks"]:
        for line in block.get("lines", []):
            spans = [span for span in line["spans"] if math.isclose(span["size"],10,abs_tol=.1)]
            if not spans:
                continue
            # Separately drawn list markers may share a baseline with text.
            for span in spans:
                x0,y0,x1,y1 = span["bbox"]
                assert x0 >= 36-.2 and x1 <= page.rect.width-36+.2
                assert y0 >= 36-.2 and y1 <= page.rect.height-36+2
            note_lines.append((min(s["bbox"][1] for s in spans),max(s["bbox"][3] for s in spans)))
    levels = sorted(set(round(y0,1) for y0,y1 in note_lines))
    assert all(b-a >= 13.5 for a,b in zip(levels,levels[1:])), levels
assert "NOTEREF" not in visible and "HYPERLINK" not in visible
if args.continued:
    assert len(pdf.pages) > 2
    words = collections.Counter(visible.split())
    assert words["Continued"] == words["citation"] == words["text."] == 50
    assert visible.count("LAST_NOTE_MARKER") == 1 and "Footnotes continued" in visible
else:
    assert len(pdf.pages) == 2
    assert visible.count("FIRST_NOTE_BODY") == 1 and visible.count("Note list item") == 1
    p = native[0]
    body = p.search_for("keep formatting, links, and list structure.")[0]
    first = p.search_for("FIRST_NOTE_BODY")[0]
    second = p.search_for("SECOND_NOTE_BODY")[0]
    assert first.y0 > body.y1 + 8 and second.y0 > first.y1
    assert second.y1 < p.rect.height - 36 + 2
    separators = [path for path in p.get_drawings() if any(i[0] == "l" and math.isclose(i[1].y,i[2].y,abs_tol=.1) for i in path["items"])]
    assert separators and body.y1 < separators[0]["rect"].y0 < first.y0
    superscripts = [s for block in p.get_text("dict")["blocks"] if "lines" in block for line in block["lines"] for s in line["spans"] if math.isclose(s["size"],7.7,abs_tol=.1)]
    assert len(superscripts) == 4
    assert any(link.get("uri") == "https://example.invalid/note" for link in links)
print(f"PDF: {len(pdf.pages)} pages, superscript/reference destinations, note text, footer counts and layout/continuation checked")

if args.docx_render:
    rendered = PdfReader(args.docx_render)
    text = "\n".join(page.extract_text() for page in rendered.pages)
    for marker in ["FIRST_NOTE_BODY","SECOND_NOTE_BODY","END_NOTE_BODY","Note list item","Repeated reference"]:
        assert text.count(marker) == 1, marker
    assert "NOTEREF" not in text and "HYPERLINK" not in text
    print("LibreOffice DOCX render retained every note once and repeated the reference without field instructions; inspect all pages separately")
