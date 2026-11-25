"""Independent explicit cell/footnote page-boundary checks; optional QA only."""
from pathlib import Path
from zipfile import ZipFile
import argparse
import base64
import io
import json
from docx import Document
from lxml import etree
from PIL import Image
from pypdf import PdfReader
import pymupdf

parser = argparse.ArgumentParser()
parser.add_argument('directory', nargs='?', type=Path, default=Path('output/page-breaks'))
parser.add_argument('--empty-note', action='store_true')
parser.add_argument('--office', action='store_true', help='record observations of an existing LibreOffice rendering')
args = parser.parse_args()
out = args.directory
sf = json.loads((out / 'page-breaks.sf').read_text())
reader = PdfReader(out / 'page-breaks.pdf', strict=True)
links, uris = [], []
for i, page in enumerate(reader.pages):
    for ref in page.get('/Annots', []):
        a = ref.get_object()
        if '/Dest' in a:
            target = next(j for j, p in enumerate(reader.pages) if p.indirect_reference.idnum == a['/Dest'][0].idnum)
            links.append((i+1, target+1))
        if '/A' in a:
            assert a['/A']['/S'] == '/URI'
            uris.append((i+1, str(a['/A']['/URI'])))
ns = {'w': 'http://schemas.openxmlformats.org/wordprocessingml/2006/main'}
with ZipFile(out / 'page-breaks.docx') as z:
    main = etree.fromstring(z.read('word/document.xml'))
    notes = etree.fromstring(z.read('word/footnotes.xml'))
    assert len(notes.xpath('//w:pageBreakBefore', namespaces=ns)) == 2
    assert not notes.xpath('//w:br[@w:type="page"]', namespaces=ns)
    assert len(main.xpath('//w:footnoteReference', namespaces=ns)) == 1
    if args.empty_note:
        assert not main.xpath('//w:br[@w:type="page"]', namespaces=ns)
    else:
        assert len(main.xpath('//w:pageBreakBefore', namespaces=ns)) == 2
        assert not main.xpath('//w:br[@w:type="page"]', namespaces=ns)
        assert notes.xpath('//w:t[text()="NOTE_AFTER"]/ancestor::w:p/w:pPr/w:pageBreakBefore', namespaces=ns)
        assert main.xpath('//w:t[text()="INNER_AFTER"]/ancestor::w:p/w:pPr/w:pageBreakBefore', namespaces=ns)
        original_media = [z.read(n) for n in z.namelist() if n.startswith('word/media/')]

with pymupdf.open(out / 'page-breaks.pdf') as doc:
    if args.empty_note:
        assert len(reader.pages) == len(doc) == 3
        assert links == [(1, 3)] and not uris
        assert doc[0].get_text().strip() == 'REF1'
        assert all(not p.get_text().strip() and not p.get_image_info() for p in list(doc)[1:])
        assert len(doc[0].get_links()) == 1 and doc[0].get_links()[0]['page'] == 2
    else:
        assert len(reader.pages) == len(doc) == 5
        assert links == [(2, 2)]
        assert uris == [(2, 'https://example.invalid/break'), (4, 'https://example.invalid/note-break')]
        word = Document(out / 'page-breaks.docx')
        outer = word.tables[0]
        assert outer.cell(0, 0)._tc is outer.cell(1, 1)._tc
        inner = outer.cell(0, 0).tables[0]
        assert inner.cell(0, 0)._tc is inner.cell(1, 0)._tc
        expected = {'OUTER_BEFORE': 1, 'OUTER_SIDE_A': 1, 'OUTER_SIDE_B': 1,
                    'INNER_BEFORE': 1, 'INNER_SIDE_A': 1, 'INNER_AFTER': 2,
                    'INNER_SIDE_B': 2, 'OUTER_AFTER': 2, 'NOTE_BEFORE': 2,
                    'NOTE_AFTER': 3, 'NOTE_INNER_BEFORE': 3, 'NOTE_SIDE_A': 3,
                    'NOTE_INNER_AFTER': 4, 'NOTE_SIDE_B': 4, 'NOTE_END': 4,
                    'BODY_AFTER': 5}
        xml_text = main.xpath('//w:t/text()', namespaces=ns) + notes.xpath('//w:t/text()', namespaces=ns)
        for token in expected: assert xml_text.count(token) == 1, token
        spans = [[s for b in p.get_text('dict')['blocks'] if b['type'] == 0 for l in b['lines'] for s in l['spans']] for p in doc]
        for token, page in expected.items():
            found = [(i+1, s) for i, ss in enumerate(spans) for s in ss if s['text'] == token]
            assert len(found) == 1 and found[0][0] == page, (token, found)
        after = next(s for s in spans[1] if s['text'] == 'INNER_AFTER')
        assert after['flags'] & 16
        assert [len(p.get_image_info()) for p in doc] == [0, 1, 0, 1, 0]
        blue = sf['content'][0]['content'][0]['content'][0]['content'][1]['content'][0]['content'][0]['content'][3]
        green = sf['attrs']['notes'][0]['content'][3]['content'][0]['content'][0]['content'][3]
        for index, bounds, node in [(1, (32, 46, 62, 61), blue), (3, (44, 172, 68, 184), green)]:
            original = base64.b64decode(node['attrs']['src'].split(',', 1)[1])
            assert original in original_media
            info = doc[index].get_image_info(xrefs=True)[0]
            assert all(abs(a-b) < .04 for a,b in zip(info['bbox'], bounds)), info
            image = Image.open(io.BytesIO(doc.extract_image(info['xref'])['image'])).convert('RGB')
            source = Image.open(io.BytesIO(original)).convert('RGB')
            assert image.size == source.size and image.tobytes() == source.tobytes()
        # Source-derived spanning columns must have just one pair of page edges.
        frames = {0: [(20, 20+440*2/3), (26, 26+(440*2/3-12)/2)],
                  1: [(20, 20+440*2/3), (26, 26+(440*2/3-12)/2)],
                  2: [(38, 249)], 3: [(38, 249)]}
        for i, columns in frames.items():
            horizontal = set()
            for path in doc[i].get_drawings():
                for item in path['items']:
                    if item[0] == 'l' and abs(item[1].y-item[2].y) < .01:
                        a,b = item[1:]
                        horizontal.add(tuple(round(v, 2) for v in (min(a.x,b.x), a.y, max(a.x,b.x))))
            for x0,x1 in columns:
                assert sum(abs(s[0]-x0) < .04 and abs(s[2]-x1) < .04 for s in horizontal) == 2, (i, columns, horizontal)
        assert len(doc[1].get_links()) == 2 and any(l.get('page') == 1 for l in doc[1].get_links())
    margin = sf['attrs']['layout']['margin']
    for i, page in enumerate(doc):
        for b in page.get_text('dict')['blocks']:
            for l in b.get('lines', []):
                for s in l['spans']:
                    x0,y0,x1,y1 = s['bbox']
                    assert x0 >= margin-.05 and y0 >= margin-.05 and x1 <= page.rect.width-margin+.05 and y1 <= page.rect.height-margin+.05, (i,s)
        page.get_pixmap(matrix=pymupdf.Matrix(1.5, 1.5)).save(out / f'page-{i+1}.png')
print('Page breaks passed: physical boundaries, nested span frames, exact content/marks/assets, actual note targets and DOCX break ownership' + (' (empty note)' if args.empty_note else ''))
if args.office and not args.empty_note:
    import re
    with pymupdf.open(out / 'lo' / 'page-breaks.pdf') as doc:
        text = ''.join(p.get_text() for p in doc)
        missing = [token for token in expected if not re.search(r'(?<![A-Z_0-9])' + re.escape(token) + r'(?![A-Z_])', text)]
        observation = {'pages': len(doc), 'missing_tokens': missing,
                       'images': sum(len(p.get_image_info()) for p in doc),
                       'visual_acceptance': 'content mitigation passed' if not missing else 'failed: content missing',
                       'pagination': 'cell/note boundaries still differ from native; qualify Word/other producers'}
        for i, page in enumerate(doc): page.get_pixmap(matrix=pymupdf.Matrix(1.5, 1.5)).save(out / 'lo' / f'page-{i+1}.png')
        (out / 'libreoffice-observation.json').write_text(json.dumps(observation, indent=2) + '\n')
        assert not missing, ('LibreOffice omitted original content', missing)
        assert observation['images'] == 2
        print('LibreOffice content mitigation passed; pagination differs: ' + json.dumps(observation))
