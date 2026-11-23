"""Independent positioned-PDF/SF oracle. Run after building bin/manydoc.

The source is independently authored PDF syntax, not manydoc output. Python,
pypdf, Pillow and MuPDF are optional QA tools only.
"""
from pathlib import Path
import argparse
import base64
import io
import json
import subprocess
from PIL import Image
from pypdf import PdfReader
from docx import Document
import pymupdf

parser = argparse.ArgumentParser()
parser.add_argument("directory", nargs="?", type=Path, default=Path("output/pdf-recovery"))
args = parser.parse_args()
root = args.directory
root.mkdir(parents=True, exist_ok=True)

def stream(content, attrs=b""):
    if isinstance(content, str):
        content = content.encode("ascii")
    return b"<< " + attrs + b" /Length " + str(len(content)).encode() + b" >>\nstream\n" + content + b"\nendstream"

content = (
    b"BT /F 14 Tf 20 350 Td (Recovery) Tj ET\n"
    b"q 2 0 0 3 10 20 cm BT /F 10 Tf 50 Tz 2 Ts 1 0 0 1 5 7 Tm (AB) Tj (C) Tj ET Q\n"
    b"q 1 0 0 1 10 20 cm /X Do Q\n"
    b"q 40 0 0 20 200 100 cm /Img Do Q\n"
    b"q 40 0 0 20 200 150 cm BI /W 2 /H 1 /BPC 8 /CS /RGB ID EI \x00\xff\x01 EI Q\n"
    b"q 0 0 1 rg 40 0 0 20 200 200 cm /Stencil Do Q\n"
    b"BT /F 12 Tf 20 300 Td /P << /MCID 5 /ActualText <FEFF00660069> >> BDC (X) Tj (Y) Tj EMC ET\n"
)
objects = [
    b"<< /Type /Catalog /Pages 2 0 R >>",
    b"<< /Type /Pages /Kids [3 0 R 4 0 R] /Count 2 >>",
    b"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 400] /Resources << /Font << /F 5 0 R >> /XObject << /X 7 0 R /Img 8 0 R /Stencil 12 0 R >> >> /Contents 6 0 R /Annots [11 0 R] >>",
    b"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 400] /CropBox [20 20 220 320] /Rotate 90 /UserUnit 2 /Resources << /Font << /F 5 0 R >> >> /Contents 10 0 R >>",
    b"<< /Type /Font /Subtype /Type1 /BaseFont /Courier >>",
    stream(content),
    stream("BT /F 10 Tf 1 0 0 1 5 6 Tm (X) Tj ET", b"/Type /XObject /Subtype /Form /BBox [0 0 20 20] /Matrix [2 0 0 2 3 4]"),
    stream(b"\xff\x00\x00\x00\xff\x00", b"/Type /XObject /Subtype /Image /Width 2 /Height 1 /BitsPerComponent 8 /ColorSpace /DeviceRGB /SMask 9 0 R"),
    stream(b"\xff\x00", b"/Type /XObject /Subtype /Image /Width 2 /Height 1 /BitsPerComponent 8 /ColorSpace /DeviceGray"),
    stream("BT /F 10 Tf 1 0 0 1 30 50 Tm (A) Tj ET"),
    b"<< /Type /Annot /Subtype /Stamp /F 4 /Rect [100 200 200 300] /AP << /N 7 0 R >> >>",
    stream(b"\x40", b"/Type /XObject /Subtype /Image /Width 2 /Height 1 /ImageMask true /BitsPerComponent 1"),
]
data = bytearray(b"%PDF-1.7\n")
offsets = [0]
for i, obj in enumerate(objects, 1):
    offsets.append(len(data))
    data.extend(f"{i} 0 obj\n".encode() + obj + b"\nendobj\n")
xref = len(data)
data.extend(f"xref\n0 {len(offsets)}\n0000000000 65535 f \n".encode())
for offset in offsets[1:]:
    data.extend(f"{offset:010d} 00000 n \n".encode())
data.extend(f"trailer\n<< /Size {len(offsets)} /Root 1 0 R >>\nstartxref\n{xref}\n%%EOF\n".encode())
pdf_path = root / "positioned.pdf"
pdf_path.write_bytes(data)
reader = PdfReader(pdf_path, strict=True)
assert len(reader.pages) == 2
assert len(reader.pages[0].images) >= 3
sf_path = root / "positioned.sf"
subprocess.run(["bin/manydoc", "extract", "-format", "sf", "-o", str(sf_path), str(pdf_path)], check=True)
sf = json.loads(sf_path.read_text())
pages = sf["attrs"]["pages"]
assert [(p["width"], p["height"]) for p in pages] == [(300, 400), (600, 400)]
assert pages[1]["rotate"] == 90 and pages[1]["user_unit"] == 2

def source(node):
    if node["type"] == "text":
        return next(m["attrs"] for m in node["marks"] if m["type"] == "source")
    return node["attrs"]["source"]

runs = [run for block in sf["content"] for run in block.get("content", []) if run["type"] == "text"]
by_text = {r["text"]: source(r) for r in runs}

def near(actual, expected, tol=0.02):
    assert len(actual) == len(expected)
    assert all(abs(a-b) < tol for a, b in zip(actual, expected)), (actual, expected)

def point(p):
    return p["x"], p["y"]

near(point(by_text["AB"]["baseline"][0]), (20, 353))
near(point(by_text["C"]["baseline"][0]), (32, 353))
near(point(by_text["A"]["baseline"][0]), (60, 20))
assert by_text["fi"]["mcid"] == 5 and len(by_text["fi"]["regions"]) == 2
assert by_text["AB"]["structure"] == "unstructured"
assert sf["content"][0]["attrs"]["source"]["structure"] == "inferred"
with pymupdf.open(pdf_path) as doc:
    chars = [ch for block in doc[0].get_text("rawdict")["blocks"] if block["type"] == 0
             for line in block["lines"] for span in line["spans"] for ch in span["chars"]]
    for text in ("AB", "C"):
        # MuPDF independently computes the first glyph's baseline, including CTM/Tz/Ts.
        candidates = [ch["origin"] for ch in chars if ch["c"] == text[0]]
        expected = point(by_text[text]["baseline"][0])
        assert any(abs(x-expected[0]) < .02 and abs(y-expected[1]) < .02 for x, y in candidates), (text, candidates)
    annotation_origins = [ch["origin"] for ch in chars if ch["c"]=="X"]
    # MuPDF's text extractor omits annotation appearances. Inspect its rendered
    # pixels at the independent appearance placement instead.
    pix0 = doc[0].get_pixmap(alpha=False)
    assert any(min(pix0.pixel(x,y)) < 80 for x in range(124,142) for y in range(150,172))
    near(point(by_text["X"]["baseline"][0]), (125,170))
    # Raw text origins include UserUnit. In this PyMuPDF build rotation_matrix's
    # translation uses the unscaled CropBox (300 rather than 600). Apply the
    # documented clockwise rotation using the physical rendered page width.
    p = doc[1]
    ch = next(ch for b in p.get_text("rawdict")["blocks"] if b["type"] == 0
              for line in b["lines"] for span in line["spans"] for ch in span["chars"] if ch["c"] == "A")
    near((p.rect.width - ch["origin"][1], ch["origin"][0]), point(by_text["A"]["baseline"][0]))
    pix = p.get_pixmap(alpha=False)
    # Independently check the rotated glyph is actually painted near its baseline.
    assert any(min(pix.pixel(x,y)) < 80 for x in range(58,78) for y in range(18,34))
    images = [n for n in sf["content"] if n["type"] == "image"]
    assert len(images) == 3
    infos = doc[0].get_image_info()
    for node in images:
        q = source(node)["quad"]
        bbox = (min(p["x"] for p in q), min(p["y"] for p in q), max(p["x"] for p in q), max(p["y"] for p in q))
        assert any(all(abs(a-b) < .02 for a,b in zip(info["bbox"], bbox)) for info in infos), bbox
    for i, page in enumerate(doc):
        page.get_pixmap(matrix=pymupdf.Matrix(2, 2), alpha=False).save(root / f"source-{i+1}.png")
assets = [Image.open(io.BytesIO(base64.b64decode(n["attrs"]["src"].split(",",1)[1]))).convert("RGBA") for n in images]
assert assets[0].getpixel((0,0)) == (255,0,0,255)
assert assets[0].getpixel((1,0))[3] == 0
assert assets[1].getpixel((0,0)) == (ord("E"),ord("I"),32,255)
assert assets[2].getpixel((0,0)) == (0,0,255,255) and assets[2].getpixel((1,0))[3] == 0
print("Independent pypdf/MuPDF checks passed: 2 page coordinate systems, transformed text, forms/annotation, 3 image placements/assets, masks, ActualText and SF provenance")

# Exercise recovered SF through both rendered formats, explicitly acknowledging
# flow-layout/provenance losses. Verify that supported content/assets survive.
for ext in ("pdf", "docx"):
    subprocess.run(["bin/manydoc", "convert", "-allow-lossy", "-o", str(root / f"reflow.{ext}"), str(sf_path)], check=True)
reflow = PdfReader(root / "reflow.pdf", strict=True)
assert len(reflow.pages) == 2
text = "".join(p.extract_text() for p in reflow.pages)
assert all(s in text for s in ("Recovery", "ABC", "fi", "X", "A"))
# gofpdf shares an image resource dictionary between pages. Count unique
# resource assets, then independently count actual paint operations with MuPDF.
assert len({im.name for p in reflow.pages for im in p.images}) == 3
word = Document(root / "reflow.docx")
assert len(word.inline_shapes) == 3
word_text = "".join(p.text for p in word.paragraphs)
assert all(s in word_text for s in ("Recovery", "ABC", "fi", "X", "A"))
with pymupdf.open(root / "reflow.pdf") as doc:
    assert sum(len(p.get_image_info()) for p in doc) == 3
    for i,page in enumerate(doc):
        page.get_pixmap(matrix=pymupdf.Matrix(1.2,1.2), alpha=False).save(root / f"reflow-{i+1}.png")
print("Recovered SF -> DOCX/PDF independent content/asset checks passed (acknowledged reflow)")
