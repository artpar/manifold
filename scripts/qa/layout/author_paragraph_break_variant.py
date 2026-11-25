"""QA experiment only: compare paragraph-break properties with break runs.

This does not establish SF round-trip semantics or matching Word pagination.
The optional LibreOffice conversion is documented in TESTING.md.
"""
from pathlib import Path
from zipfile import ZipFile, ZIP_DEFLATED
import argparse
from lxml import etree

parser = argparse.ArgumentParser()
parser.add_argument('directory', nargs='?', type=Path, default=Path('output/page-breaks'))
out = parser.parse_args().directory
w = '{http://schemas.openxmlformats.org/wordprocessingml/2006/main}'
changed = 0
with ZipFile(out / 'page-breaks.docx') as src, ZipFile(out / 'paragraph-breaks.docx', 'w', ZIP_DEFLATED) as dst:
    for name in src.namelist():
        data = src.read(name)
        if name in ('word/document.xml', 'word/footnotes.xml'):
            root = etree.fromstring(data)
            for p in list(root.iter(w + 'p')):
                breaks = p.findall('.//' + w + 'br')
                if len(breaks) != 1 or breaks[0].get(w + 'type') != 'page' or list(p.iter(w + 't')):
                    continue
                following = p.getnext()
                if following is None or following.tag != w + 'p':
                    continue
                props = following.find(w + 'pPr')
                if props is None:
                    props = etree.Element(w + 'pPr')
                    following.insert(0, props)
                props.insert(0, etree.Element(w + 'pageBreakBefore'))
                p.getparent().remove(p)
                changed += 1
            data = etree.tostring(root, xml_declaration=True, encoding='UTF-8')
        dst.writestr(name, data)
print(f'Authored experimental paragraph-break variant: {changed} replacements; not a production codec or fidelity pass')
