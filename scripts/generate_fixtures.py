"""Independent synthetic fixtures. Python dependencies are test-only."""
from pathlib import Path
from io import BytesIO
import json
import zipfile
import struct
import hashlib

from docx import Document
from docx.shared import Inches
from docx.oxml import OxmlElement
from docx.oxml.ns import qn
from PIL import Image, ImageDraw
from reportlab.pdfgen import canvas
from reportlab.pdfbase import pdfmetrics
from reportlab.pdfbase.ttfonts import TTFont
from reportlab.pdfbase.cidfonts import UnicodeCIDFont
from reportlab.lib.utils import ImageReader
import reportlab
from pypdf import PdfReader, PdfWriter
from pypdf.generic import NameObject, DecodedStreamObject, DictionaryObject, ArrayObject, NumberObject
import pymupdf

ROOT = Path(__file__).resolve().parents[1] / "testdata"
FIX = ROOT / "corpus"
FIX.mkdir(exist_ok=True)
CASES = []
NS = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"

def record(name, kind, expect="accept", **kw):
    path = FIX / name
    CASES.append(dict(name=name, kind=kind, expect=expect, size=path.stat().st_size,
                      sha256=hashlib.sha256(path.read_bytes()).hexdigest(), **kw))

def docx_case(name, build, **kw):
    d = Document()
    build(d)
    d.save(FIX / name)
    record(name, "docx", **kw)

def patch_docx(name, source="docx_basic.docx", replace=None, add=None, remove=()):
    with zipfile.ZipFile(FIX / source) as z:
        parts = {n: z.read(n) for n in z.namelist() if n not in remove}
    if replace:
        for name_part, fn in replace.items():
            parts[name_part] = fn(parts[name_part])
    parts.update(add or {})
    with zipfile.ZipFile(FIX / name, "w", zipfile.ZIP_DEFLATED) as z:
        for n, data in parts.items():
            z.writestr(n, data)

def insert_body(raw):
    return lambda b: b.replace(b"<w:sectPr", raw.encode() + b"<w:sectPr", 1)

def main():
    docx_case("docx_basic.docx", lambda d: [d.add_heading("Report title", 0),
        d.add_paragraph("Alpha paragraph"), d.add_paragraph("Beta paragraph")])
    docx_case("docx_blank.docx", lambda d: None)
    docx_case("docx_unicode.docx", lambda d: d.add_paragraph("café Ελληνικά Привет 中文 हिन्दी 😀 A&B <tag>"))
    def rich(d):
        p = d.add_paragraph("Leading ")
        p.add_run("bold").bold = True
        p.add_run(" italic").italic = True
        p.add_run(" underline").underline = True
        p.add_run(" trailing ")
    docx_case("docx_runs.docx", rich)
    docx_case("docx_whitespace.docx", lambda d: d.add_paragraph("  spaces  tab\tline\nend  "))
    docx_case("docx_lists.docx", lambda d: [d.add_paragraph("Bullet item", "List Bullet"),
        d.add_paragraph("Number item", "List Number")])
    def tables(d):
        d.add_paragraph("Before table")
        t = d.add_table(rows=3, cols=2)
        for i, row in enumerate(t.rows):
            for j, c in enumerate(row.cells): c.text = f"CELL-{i}-{j}"
        d.add_paragraph("After table")
    docx_case("docx_table.docx", tables)
    def nested(d):
        t = d.add_table(rows=2, cols=2)
        t.cell(0, 0).merge(t.cell(0, 1)).text = "Merged heading"
        t.cell(1, 0).text = "Outer cell"
        t.cell(1, 1).add_table(rows=1, cols=1).cell(0, 0).text = "Nested cell"
    docx_case("docx_nested_table.docx", nested)
    def header(d):
        d.add_paragraph("Body marker")
        d.sections[0].header.paragraphs[0].text = "Header marker"
        d.sections[0].footer.paragraphs[0].text = "Footer marker"
        d.core_properties.title = "Metadata title"
        d.core_properties.author = "Fixture author"
    docx_case("docx_header_footer.docx", header)
    img = Image.new("RGB", (400, 90), "white")
    ImageDraw.Draw(img).text((15, 30), "SCAN_ONLY_MARKER", fill="black")
    img.save(FIX / "image.png")
    docx_case("docx_image.docx", lambda d: [d.add_paragraph("Image caption"),
        d.add_picture(str(FIX / "image.png"), width=Inches(2))])
    def link(d):
        p = d.add_paragraph("Before ")
        h = OxmlElement("w:hyperlink")
        h.set(qn("r:id"), d.part.relate_to("https://example.invalid/no-fetch",
            "http://schemas.openxmlformats.org/officeDocument/2006/relationships/hyperlink", is_external=True))
        r = OxmlElement("w:r"); t = OxmlElement("w:t"); t.text = "Linked text"
        r.append(t); h.append(r); p._p.append(h); p.add_run(" after")
    docx_case("docx_hyperlink.docx", link)
    docx_case("docx_sections.docx", lambda d: [d.add_paragraph("Section one"), d.add_section(), d.add_paragraph("Section two")])
    docx_case("docx_page_break.docx", lambda d: [d.add_paragraph("Page one"), d.add_page_break(), d.add_paragraph("Page two")])
    docx_case("docx_large.docx", lambda d: [d.add_paragraph(f"Paragraph {i:04d}: " + "document evaluation " * 20) for i in range(2500)])
    special = {
        "docx_tracked.docx": '<w:p><w:ins w:id="1" w:author="Fixture"><w:r><w:t>INSERTED_MARKER</w:t></w:r></w:ins><w:del w:id="2" w:author="Fixture"><w:r><w:delText>DELETED_MARKER</w:delText></w:r></w:del></w:p>',
        "docx_content_control.docx": '<w:sdt><w:sdtPr><w:tag w:val="fixture"/></w:sdtPr><w:sdtContent><w:p><w:r><w:t>CONTROL_MARKER</w:t></w:r></w:p></w:sdtContent></w:sdt>',
        "docx_textbox.docx": '<w:p><w:r><w:pict><v:shape xmlns:v="urn:schemas-microsoft-com:vml"><v:textbox><w:txbxContent><w:p><w:r><w:t>TEXTBOX_MARKER</w:t></w:r></w:p></w:txbxContent></v:textbox></v:shape></w:pict></w:r></w:p>',
        "docx_field.docx": '<w:p><w:fldSimple w:instr="DATE"><w:r><w:t>FIELD_RESULT_MARKER</w:t></w:r></w:fldSimple></w:p>',
        "docx_bookmark.docx": '<w:p><w:bookmarkStart w:id="7" w:name="fixture"/><w:r><w:t>BOOKMARK_MARKER</w:t></w:r><w:bookmarkEnd w:id="7"/></w:p>',
    }
    for name, body in special.items():
        patch_docx(name, replace={"word/document.xml": insert_body(body)})
        record(name, "docx")
    patch_docx("docx_missing_styles.docx", remove=["word/styles.xml"])
    record("docx_missing_styles.docx", "docx", "reject", note="dangling styles relationship")
    patch_docx("docx_missing_document.docx", remove=["word/document.xml"])
    record("docx_missing_document.docx", "docx", "reject")
    patch_docx("docx_missing_rels.docx", remove=["word/_rels/document.xml.rels"])
    record("docx_missing_rels.docx", "docx", note="Optional document relationships part omitted; remaining styles/numbering parts are unreferenced")
    patch_docx("docx_bad_xml.docx", replace={"word/document.xml": lambda b: b"<document><unclosed>"})
    record("docx_bad_xml.docx", "docx", "reject")
    patch_docx("docx_wrong_root.docx", replace={"word/document.xml": lambda b: b'<unexpected xmlns="urn:wrong"/>'})
    record("docx_wrong_root.docx", "docx", "reject")
    patch_docx("docx_strict.docx", replace={"word/document.xml": lambda b: b.replace(NS.encode(), b"http://purl.oclc.org/ooxml/wordprocessingml/main")})
    record("docx_strict.docx", "docx", note="strict main namespace with transitional package")
    patch_docx("docx_compressed_payload.docx", add={"custom/unused.bin": b"A" * (8 * 1024 * 1024)})
    record("docx_compressed_payload.docx", "docx", note="8 MiB highly compressed unused member")
    for name, data in [("docx_empty.docx", b""), ("docx_random.docx", b"not a docx"),
                       ("docx_truncated.docx", (FIX / "docx_basic.docx").read_bytes()[:100])]:
        (FIX / name).write_bytes(data); record(name, "docx", "reject")
    with zipfile.ZipFile(FIX / "docx_empty_zip.docx", "w"): pass
    record("docx_empty_zip.docx", "docx", "reject")
    # Minimal valid package: styles and document relationships are optional.
    with zipfile.ZipFile(FIX / "docx_minimal.docx", "w", zipfile.ZIP_DEFLATED) as z:
        z.writestr("[Content_Types].xml", '<?xml version="1.0"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/></Types>')
        z.writestr("_rels/.rels", '<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/></Relationships>')
        z.writestr("word/document.xml", f'<w:document xmlns:w="{NS}"><w:body><w:p><w:r><w:t>MINIMAL_MARKER</w:t></w:r></w:p></w:body></w:document>')
    record("docx_minimal.docx", "docx", note="valid minimal package, no styles or document relationships")

    # Pinned font with actual Greek/Cyrillic glyphs; bundled Vera lacks them.
    font = ROOT / "assets" / "NotoSans-Regular.ttf"
    pdfmetrics.registerFont(TTFont("Vera", str(font)))
    pdfmetrics.registerFont(UnicodeCIDFont("STSong-Light"))
    pdfmetrics.registerFont(UnicodeCIDFont("HeiseiMin-W3"))
    def pdf_case(name, draw, **kw):
        c = canvas.Canvas(str(FIX / name), pagesize=(612, 792), invariant=1)
        draw(c); c.save(); record(name, "pdf", **kw)
    def line(c, text, y=740, font="Helvetica", x=50):
        c.setFont(font, 12); c.drawString(x, y, text)
    pdf_case("pdf_basic.pdf", lambda c: [line(c, "Alpha paragraph"), line(c, "Beta paragraph", 710)], markers=["Alpha paragraph", "Beta paragraph"])
    import shutil
    shutil.copyfile(ROOT / "assets/w3c-dummy.pdf", FIX / "pdf_w3c_dummy.pdf")
    record("pdf_w3c_dummy.pdf", "pdf", markers=["Dummy", "PDF", "file"], source="https://www.w3.org/WAI/ER/tests/xhtml/testfiles/resources/pdf/dummy.pdf")
    pdf_case("pdf_multipage.pdf", lambda c: [line(c, "PAGE_ONE"), c.showPage(), line(c, "PAGE_TWO"), c.showPage(), line(c, "PAGE_THREE")], markers=["PAGE_ONE", "PAGE_TWO", "PAGE_THREE"], pages=3)
    pdf_case("pdf_blank.pdf", lambda c: c.showPage(), pages=1)
    pdf_case("pdf_unicode.pdf", lambda c: line(c, "café naïve résumé Ελληνικά Привет", font="Vera"), markers=["café", "Ελληνικά", "Привет"])
    pdf_case("pdf_cjk.pdf", lambda c: line(c, "中文测试 日本語", font="STSong-Light"), markers=["中文测试", "日本語"])
    pdf_case("pdf_japanese.pdf", lambda c: line(c, "日本語テスト", font="HeiseiMin-W3"), markers=["日本語テスト"])
    pdf_case("pdf_ligatures.pdf", lambda c: line(c, "office ﬁ ﬂ ffi ffl", font="Vera"), markers=["office", "ﬁ", "ﬂ"])
    def columns(c):
        for i in range(4):
            line(c, f"LEFT_{i}", 740-i*25, x=50)
            line(c, f"RIGHT_{i}", 740-i*25, x=330)
    pdf_case("pdf_columns.pdf", columns, markers=["LEFT_0", "LEFT_3", "RIGHT_0", "RIGHT_3"])
    pdf_case("pdf_rotated.pdf", lambda c: [c.saveState(), c.translate(100, 100), c.rotate(90), line(c, "ROTATED_MARKER", y=0, x=0), c.restoreState()], markers=["ROTATED_MARKER"])
    def form(c):
        c.beginForm("fixture"); line(c, "FORM_XOBJECT_MARKER"); c.endForm()
        c.doForm("fixture"); line(c, "DIRECT_MARKER", 700)
    pdf_case("pdf_form_xobject.pdf", form, markers=["FORM_XOBJECT_MARKER", "DIRECT_MARKER"])
    pdf_case("pdf_scan.pdf", lambda c: c.drawImage(ImageReader(img), 50, 650, width=400, height=90), note="image only, no OCR expected")
    def tablepdf(c):
        for i, (a, b) in enumerate([("Product", "Quantity"), ("Daptin", "42"), ("Other", "7")]):
            line(c, a, 740-i*25); line(c, b, 740-i*25, x=250)
    pdf_case("pdf_table.pdf", tablepdf, markers=["Product", "Quantity", "Daptin", "42"])
    def large(c):
        for p in range(100):
            for i in range(35): line(c, f"PAGE_{p:03d}_LINE_{i:02d} " + "sample text " * 4, 750-i*19)
            c.showPage()
    pdf_case("pdf_large.pdf", large, pages=100, markers=["PAGE_000_LINE_00", "PAGE_099_LINE_34"])
    # Two independent subset fonts reuse /F2+0 but map code 0x80 differently.
    pdf_case("pdf_font_a.pdf", lambda c: line(c, "café Ω", font="Vera"), markers=["café", "Ω"])
    pdf_case("pdf_font_b.pdf", lambda c: line(c, "Привет Ж", font="Vera"), markers=["Привет", "Ж"])
    writer = PdfWriter()
    for name in ["pdf_font_a.pdf", "pdf_font_b.pdf"]: writer.append(str(FIX / name))
    writer.write(FIX / "pdf_font_collision.pdf")
    record("pdf_font_collision.pdf", "pdf", pages=2, markers=["café", "Ω", "Привет", "Ж"])
    data = (FIX / "pdf_basic.pdf").read_bytes().replace(b"%PDF-1.3", b"%PDF-2.0", 1)
    (FIX / "pdf_version_2.pdf").write_bytes(data)
    record("pdf_version_2.pdf", "pdf", markers=["Alpha paragraph"], note="synthetic PDF 2.0 header, same object syntax")
    for name, use128 in [("pdf_rc4_40.pdf", False), ("pdf_rc4_128.pdf", True)]:
        w = PdfWriter(); w.append(str(FIX / "pdf_basic.pdf")); w.encrypt("secret", "owner", use_128bit=use128)
        w.write(FIX / name); record(name, "pdf", "password", markers=["Alpha paragraph"])
    for algorithm in ["AES-128", "AES-256"]:
        w = PdfWriter(); w.append(str(FIX / "pdf_basic.pdf"))
        try:
            w.encrypt("secret", "owner", algorithm=algorithm); name=f"pdf_{algorithm.lower()}.pdf"
            w.write(FIX / name); record(name, "pdf", "password", markers=["Alpha paragraph"])
        except Exception as exc: print(f"SKIP encryption {algorithm}: {exc}")
    # Modern object streams and incrementally appended content, produced independently.
    with pymupdf.open(FIX / "pdf_basic.pdf") as doc:
        doc.save(FIX / "pdf_object_stream.pdf", use_objstms=1, deflate=True)
    record("pdf_object_stream.pdf", "pdf", markers=["Alpha paragraph", "Beta paragraph"])
    import shutil
    shutil.copyfile(FIX / "pdf_basic.pdf", FIX / "pdf_incremental.pdf")
    with pymupdf.open(FIX / "pdf_incremental.pdf") as doc:
        doc[0].insert_text((50, 120), "INCREMENTAL_MARKER"); doc.saveIncr()
    record("pdf_incremental.pdf", "pdf", markers=["Alpha paragraph", "INCREMENTAL_MARKER"])
    # Widget value is separate from painted page text.
    pdf_case("pdf_form_field.pdf", lambda c: [line(c, "Form label"), c.acroForm.textfield(name="answer", value="FIELD_VALUE_MARKER", x=50, y=650, width=250, height=25), c.showPage()], markers=["Form label"], note="report widget-value omission separately")
    def custom_stream(name, stream):
        w = PdfWriter(); p = w.add_blank_page(width=612, height=792)
        font = DictionaryObject({NameObject("/Type"): NameObject("/Font"), NameObject("/Subtype"): NameObject("/Type1"), NameObject("/BaseFont"): NameObject("/Helvetica")})
        p[NameObject("/Resources")] = DictionaryObject({NameObject("/Font"): DictionaryObject({NameObject("/F1"): w._add_object(font)})})
        s = DecodedStreamObject(); s.set_data(stream)
        p[NameObject("/Contents")] = w._add_object(s); w.write(FIX / name)
    custom_stream("pdf_tj_spacing.pdf", b"BT /F1 12 Tf 50 700 Td [(hello) -350 (world)] TJ ET")
    record("pdf_tj_spacing.pdf", "pdf", markers=["hello world"], note="spacing inferred from TJ glyph positioning")
    custom_stream("pdf_double_quote.pdf", b'BT /F1 12 Tf 50 700 Td 14 TL (FIRST) Tj 0 0 (SECOND) " ET')
    record("pdf_double_quote.pdf", "pdf", markers=["FIRST", "SECOND"], note="valid three-operand double-quote text operator")
    w = PdfWriter(); p = w.add_blank_page(width=612, height=792)
    s = DecodedStreamObject(); s.set_data(b" " * (8 * 1024 * 1024))
    p[NameObject("/Contents")] = w._add_object(s.flate_encode())
    w.write(FIX / "pdf_compressed_payload.pdf")
    record("pdf_compressed_payload.pdf", "pdf", note="8 MiB decompressed whitespace stream")
    custom_stream("pdf_bad_operator.pdf", b"BT /F1 12 Tf 50 700 Td Tj ET")
    record("pdf_bad_operator.pdf", "pdf", "reject")
    for name, data in [("pdf_empty.pdf", b""), ("pdf_random.pdf", b"not a PDF"),
                       ("pdf_truncated.pdf", (FIX / "pdf_basic.pdf").read_bytes()[:100]),
                       ("pdf_no_eof.pdf", (FIX / "pdf_basic.pdf").read_bytes().rsplit(b"%%EOF", 1)[0])]:
        (FIX / name).write_bytes(data); record(name, "pdf", "reject")
    (FIX / "manifest.json").write_text(json.dumps(CASES, ensure_ascii=False, indent=2))
    print(f"Created {len(CASES)} cases")

if __name__ == "__main__": main()
