"""Independent PDF/SF/OOXML/Word-rendered constant-list check (optional QA).
Generate via MANYDOC_PDF_CONSTANT_LIST_QA=output/pdf-constant-lists go test ...
and render the four DOCX files using the commands in docs/NUMBERING.md.
"""
from pathlib import Path
from collections import Counter
import json
import re
from zipfile import ZipFile
import xml.etree.ElementTree as E
from pypdf import PdfReader
from pypdf.generic import IndirectObject

root = Path('output/pdf-constant-lists')
labels = ['§ fixed', 'Case A', '100% done', '']
ns = {'w': 'http://schemas.openxmlformats.org/wordprocessingml/2006/main'}
wa = '{' + ns['w'] + '}'

def walk(value):
    if isinstance(value, list):
        for child in value:
            yield from walk(child)
    elif isinstance(value, IndirectObject):
        node = value.get_object()
        if node.get('/Type') == '/StructElem':
            yield node
            yield from walk(node.get('/K', []))

for index, label in enumerate(labels):
    stem = 'case-' + str(index)
    original = PdfReader(root/(stem+'.pdf'), strict=True)
    nodes = list(walk(original.trailer['/Root']['/StructTreeRoot'].raw_get('/K')))
    roles = Counter(n['/S'] for n in nodes)
    assert roles['/L'] == 1 and roles['/LI'] == roles['/Lbl'] == roles['/LBody'] == 2
    explicit_list = next(n for n in nodes if n['/S'] == '/L')
    assert explicit_list['/A']['/ListNumbering'] == '/LowerRoman'
    if not label:
        assert all(not n.get('/K') for n in nodes if n['/S'] == '/Lbl'), 'empty label owns phantom painted content'
    sf = json.loads((root/(stem+'.sf')).read_text())
    node = sf['content'][0]
    assert node['type'] == 'ordered_list'
    attrs = node['attrs']
    assert attrs['order'] == 1
    assert attrs['numbering']['template'] == label and attrs['numbering']['formats'] == ['lowerRoman']
    assert 'start_unobserved' in attrs['source']['evidence']
    assert 'start_from_label' not in attrs['source']['evidence']
    assert attrs['source']['structure'] == 'explicit'
    assert any(d['code'] == 'pdf_list_constant_inferred' for d in sf['attrs']['diagnostics'])
    for ordinal, item in enumerate(node['content']):
        assert item['type'] == 'list_item' and item['attrs']['source'].get('label', '') == label
        runs = item['content'][0]['content']
        body = ['FIRST_BODY', 'SECOND_BODY'][ordinal]
        assert ''.join(r.get('text', '') for r in runs) == body
        assert any(m['type'] == ['strong', 'em'][ordinal] for r in runs for m in r.get('marks', []))
        if ordinal == 0:
            assert any(m['type'] == 'link' and m['attrs']['href'] == 'https://example.org/first' for r in runs for m in r.get('marks', []))
    with ZipFile(root/(stem+'.docx')) as archive:
        numbering = E.fromstring(archive.read('word/numbering.xml'))
        document = E.fromstring(archive.read('word/document.xml'))
        assert len(document.findall('.//w:numPr', ns)) == 2
        level = numbering.find('w:abstractNum/w:lvl', ns)
        assert level.find('w:numFmt', ns).get(wa+'val') == 'lowerRoman'
        assert level.find('w:lvlText', ns).get(wa+'val') == label
        assert numbering.find('w:num/w:lvlOverride/w:startOverride', ns).get(wa+'val') == '1'
    # Compare actual displayed labels in the native PDF and an independent Word
    # reader's rendering, not just OOXML declarations or body text substrings.
    for name, pdf in [('native', original), ('Word', PdfReader(root/'word'/(stem+'.pdf'), strict=True))]:
        assert len(pdf.pages) == 1
        visible = '\n'.join(p.extract_text() for p in pdf.pages)
        for body in ['FIRST_BODY', 'SECOND_BODY']:
            pattern = re.escape(label) + r'\s*' + body
            assert sum(bool(re.fullmatch(pattern, line.strip())) for line in visible.splitlines()) == 1, (stem, name, body, visible)
    print(stem, 'explicit L/LI/Lbl, constant template/default uncertainty, style/URI, OOXML and native/Word displayed labels pass')
print('Four cases/eight items pass; empty labels own no phantom MCIDs. No original hidden start or general Word-layout fidelity claim.')
