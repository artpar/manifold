"""Independent custom-mark, automatic-counter and note navigation QA."""
from pathlib import Path
from zipfile import ZipFile
import argparse
from lxml import etree
from docx import Document
from pypdf import PdfReader
import pymupdf

parser = argparse.ArgumentParser()
parser.add_argument('directory', nargs='?', type=Path, default=Path('output/note-labels'))
args = parser.parse_args()
root = args.directory
ns = {'w': 'http://schemas.openxmlformats.org/wordprocessingml/2006/main'}
Document(root/'notes.docx')
with ZipFile(root/'notes.docx') as z:
    main = etree.fromstring(z.read('word/document.xml'))
    custom = main.xpath('//w:r[w:footnoteReference[@w:customMarkFollows="1"] or w:endnoteReference[@w:customMarkFollows="1"]]', namespaces=ns)
    assert [''.join(n.xpath('w:t/text()', namespaces=ns)) for n in custom] == ['*α', '§E']
    assert all(len(n.xpath('w:t', namespaces=ns)) == 2 for n in custom)
    assert main.xpath('//w:fldSimple/w:r/w:t/text()', namespaces=ns) == ['*α']
    for kind in ('footnote', 'endnote'):
        tree = etree.fromstring(z.read('word/'+kind+'s.xml'))
        assert tree.xpath('//w:'+kind+'Ref', namespaces=ns)
for path in (root/'notes.pdf', root/'lo/notes.pdf'):
    pdf = PdfReader(path)
    text = '\n'.join(p.extract_text() for p in pdf.pages)
    assert len(pdf.pages) == 2
    assert text.count('*α') == 3 and text.count('§E') == 2, repr(text)
    for marker in ('FIRST_NOTE_BODY', 'SECOND_NOTE_BODY', 'END_NOTE_BODY', 'Note list item'):
        assert text.count(marker) == 1, marker
    native = pymupdf.open(path)
    # Automatic numbering resumes at 1 despite the custom footnote before it.
    assert native[0].search_for('1 SECOND_NOTE_BODY')
    links = [link for page in native for link in page.get_links() if link['kind'] == pymupdf.LINK_GOTO]
    assert len(links) >= 4
    for i, page in enumerate(native):
        page.get_pixmap(matrix=pymupdf.Matrix(1,1)).save(str(path)+f'.{i}.png')
    print(path, ': two pages, exact label/body counts, automatic counter and linked references pass')
