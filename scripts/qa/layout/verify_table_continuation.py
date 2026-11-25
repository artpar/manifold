"""Independent structural/geometry/pixel/link checks; optional QA dependencies only."""
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
parser.add_argument('directory', nargs='?', type=Path, default=Path('output/table-continuation'))
out = parser.parse_args().directory
sf = json.loads((out / 'table-continuation.sf').read_text())
left = sf['content'][0]['content'][0]['content'][0]
assert left['attrs'] == {'colspan': 2, 'rowspan': 3}
blob = base64.b64decode(left['content'][20]['attrs']['src'].split(',', 1)[1])
reader = PdfReader(out / 'table-continuation.pdf', strict=True)
assert len(reader.pages) == 4
assert reader.metadata.title == 'Merged table continuation'
links, uris = [], []
for i, page in enumerate(reader.pages):
    for ref in page.get('/Annots', []):
        annot = ref.get_object()
        if '/Dest' in annot:
            target = next(j for j, p in enumerate(reader.pages) if p.indirect_reference.idnum == annot['/Dest'][0].idnum)
            links.append((i + 1, target + 1))
        if '/A' in annot:
            assert annot['/A']['/S'] == '/URI'
            uris.append((i + 1, str(annot['/A']['/URI'])))
assert links == [(1, 1), (3, 3)], links
assert uris == [(3, 'https://example.invalid/continued')], uris
word = Document(out / 'table-continuation.docx')
table = word.tables[0]
assert len(table.rows) == 3 and len(table.columns) == 3
assert all(table.cell(0, 0)._tc is table.cell(r, c)._tc for r in range(3) for c in range(2))
ns = {'w': 'http://schemas.openxmlformats.org/wordprocessingml/2006/main'}
with ZipFile(out / 'table-continuation.docx') as z:
    assert blob in [z.read(n) for n in z.namelist() if n.startswith('word/media/')]
    notes = etree.fromstring(z.read('word/footnotes.xml'))
    main = etree.fromstring(z.read('word/document.xml'))
    assert len(main.xpath('//w:footnoteReference', namespaces=ns)) == 2
    assert len(notes.xpath('//w:footnote[not(@w:type)]', namespaces=ns)) == 2

def approx(a, b):
    assert len(a) == len(b) and all(abs(x - y) < .04 for x, y in zip(a, b)), (a, b)

with pymupdf.open(out / 'table-continuation.pdf') as doc:
    texts = [p.get_text() for p in doc]
    whole = ''.join(texts)
    for token in [*(f'SPANNING_{i:02d}' for i in range(20)), *(f'RIGHT_B_{i:02d}' for i in range(12)), 'RIGHT_A', 'RIGHT_C', 'SPANNING_LAST', 'NOTE_EARLY', 'NOTE_LATE', 'AFTER_GRID']:
        assert whole.count(token) == 1, token
    assert 'NOTE_EARLY' in texts[0] and 'NOTE_LATE' in texts[2] and 'AFTER_GRID' in texts[3]
    assert [len(p.get_image_info()) for p in doc] == [0, 0, 1, 0]
    info = doc[2].get_image_info(xrefs=True)[0]
    approx(info['bbox'], (26, 110, 56, 125))
    extracted = Image.open(io.BytesIO(doc.extract_image(info['xref'])['image'])).convert('RGB')
    assert extracted.tobytes() == Image.open(io.BytesIO(blob)).convert('RGB').tobytes()
    for prefix, count, x in [('SPANNING_', 20, 26), ('RIGHT_B_', 12, 212.6667)]:
        previous = None
        for n in range(count):
            token = f'{prefix}{n:02d}'
            found = [(i, r) for i, p in enumerate(doc) for r in p.search_for(token)]
            assert len(found) == 1, token
            i, r = found[0]
            assert abs(r.x0 - x) < .04, (token, r)
            key = (i, r.y0)
            assert previous is None or key > previous, token
            previous = key
    spans = [s for b in doc[0].get_text('dict')['blocks'] if b['type'] == 0 for l in b['lines'] for s in l['spans']]
    assert any(s['text'] == 'SPANNING_00' and s['flags'] & 16 for s in spans)
    for i, page in enumerate(doc):
        for block in page.get_text('dict')['blocks']:
            for line in block.get('lines', []):
                for span in line['spans']:
                    x0, y0, x1, y1 = span['bbox']
                    assert x0 >= 19.95 and y0 >= 19.95 and x1 <= 300.05 and y1 <= 180.05, (i, span)
        if i < 3:
            segments = []
            for path in page.get_drawings():
                for item in path['items']:
                    if item[0] == 'l':
                        a, b = item[1:]
                        segments.append((a.x, a.y, b.x, b.y))
            # Continuations close at the sum of their planned row/flow heights.
            bottom = 153 if i == 2 else 154
            for x in (20, 206.6667, 300):
                vertical = [s for s in segments if abs(s[0] - x) < .04 and abs(s[2] - x) < .04]
                assert vertical and abs(min(min(s[1], s[3]) for s in vertical) - 20) < .04
                assert abs(max(max(s[1], s[3]) for s in vertical) - bottom) < .04
            if i < 2:
                y = (46, 116)[i]
                assert any(all(abs(a-b) < .04 for a,b in zip(s, (206.6667, y, 300, y))) for s in segments)
                assert not any(abs(s[1]-y) < .04 and abs(s[3]-y) < .04 and min(s[0],s[2]) < 206 for s in segments), 'rowspan was divided'
        page.get_pixmap(matrix=pymupdf.Matrix(1.7, 1.7)).save(out / f'page-{i+1}.png')
print('Table continuation passed: four pages, ordered cell flows, true rowspan borders, exact image pixels/position, note target pages and DOCX merges/assets')
