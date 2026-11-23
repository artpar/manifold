"""Optional independent QA of recursive table/footnote continuation."""
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
parser.add_argument('directory', nargs='?', type=Path, default=Path('output/recursive-tables'))
parser.add_argument('--office', action='store_true', help='also inspect an existing LibreOffice DOCX rendering')
args = parser.parse_args()
out = args.directory
sf = json.loads((out / 'recursive-tables.sf').read_text())
reader = PdfReader(out / 'recursive-tables.pdf', strict=True)
assert len(reader.pages) == 7
assert reader.metadata.title == 'Recursive table and footnote continuation'
links, uris = [], []
for i, page in enumerate(reader.pages):
    for ref in page.get('/Annots', []):
        a = ref.get_object()
        if '/Dest' in a:
            target = next(j for j, p in enumerate(reader.pages) if p.indirect_reference.idnum == a['/Dest'][0].idnum)
            links.append((i + 1, target + 1))
        if '/A' in a:
            assert a['/A']['/S'] == '/URI'
            uris.append((i + 1, str(a['/A']['/URI'])))
assert links == [(1, 1), (3, 3)], links
assert uris == [(3, 'https://example.invalid/deep'), (6, 'https://example.invalid/note')], uris
assets = []
def collect(nodes):
    for node in nodes:
        if node['type'] == 'image':
            assets.append(base64.b64decode(node['attrs']['src'].split(',', 1)[1]))
        collect(node.get('content', []))
collect(sf['content'])
for note in sf['attrs']['notes']: collect(note['content'])
word = Document(out / 'recursive-tables.docx')
table = word.tables[0]
for depth, rows in enumerate((2, 2, 3)):
    assert len(table.rows) == rows and len(table.columns) == 3
    assert all(table.cell(0, 0)._tc is table.cell(r, c)._tc for r in range(rows) for c in range(2))
    if depth < 2: table = table.cell(0, 0).tables[0]
ns = {'w': 'http://schemas.openxmlformats.org/wordprocessingml/2006/main'}
with ZipFile(out / 'recursive-tables.docx') as z:
    assert all(blob in [z.read(n) for n in z.namelist() if n.startswith('word/media/')] for blob in assets)
    notes = etree.fromstring(z.read('word/footnotes.xml'))
    starts = notes.xpath('//w:vMerge[@w:val="restart"]', namespaces=ns)
    assert len(starts) == 2  # note outer merge and its nested merge
    assert len(notes.xpath('//w:footnote[not(@w:type)]', namespaces=ns)) == 2

sets = [('D_', 18), ('SB_', 8), ('N_', 14), ('NI_', 10)]
tokens = ['OUTER_START', 'OUTER_END', 'OUTER_A', 'OUTER_B', 'MIDDLE_START', 'MIDDLE_END', 'MID_A', 'MID_B', 'SIDE_A', 'SIDE_C', 'DEEP_END', 'BODY_AFTER', 'NOTE_EARLY', 'NOTE_LATE', 'NOTE_A', 'NOTE_B', 'NOTE_END', 'NI_A', 'NI_B']
for prefix, count in sets: tokens.extend(f'{prefix}{i:02d}' for i in range(count))

def approx(a, b):
    assert len(a) == len(b) and all(abs(x-y) < .04 for x, y in zip(a, b)), (a, b)

with pymupdf.open(out / 'recursive-tables.pdf') as doc:
    texts = [p.get_text() for p in doc]
    whole = ''.join(texts)
    for token in tokens: assert whole.count(token) == 1, token
    assert 'NOTE_EARLY' in texts[0] and 'NOTE_LATE' in texts[2] and 'BODY_AFTER' in texts[6]
    assert all('D_' not in texts[i] and 'OUTER_' not in texts[i] for i in (3, 4, 5))
    for prefix, count in sets:
        previous = None
        for n in range(count):
            token = f'{prefix}{n:02d}'
            found = [(i, r) for i, p in enumerate(doc) for r in p.search_for(token)]
            assert len(found) == 1, token
            i, r = found[0]
            expected_x = 38 if prefix == 'D_' else 155.037 if prefix == 'SB_' else 44 if prefix == 'N_' else 50
            assert abs(r.x0 - expected_x) < .04, (token, r)
            key = i, r.y0
            assert previous is None or key > previous, token
            previous = key
    # Analytical columns from source widths/spans, independent of manydoc's plans.
    outer_end = 20 + 440 * 2 / 3
    middle_end = 26 + (440 * 2 / 3 - 12) * 2 / 3
    deep_end = 32 + ((440 * 2 / 3 - 12) * 2 / 3 - 12) * 2 / 3
    frames = {0: [(20, outer_end), (26, middle_end), (32, deep_end)],
              1: [(20, outer_end), (26, middle_end), (32, deep_end)],
              2: [(20, outer_end), (26, middle_end), (32, deep_end)],
              3: [(38, 249)], 4: [(38, 249), (44, 143.5)],
              5: [(38, 249), (44, 143.5)], 6: [(20, outer_end)]}
    for i, page in enumerate(doc):
        horizontal = set()
        for path in page.get_drawings():
            for item in path['items']:
                if item[0] == 'l' and abs(item[1].y-item[2].y) < .01:
                    a, b = item[1:]
                    horizontal.add(tuple(round(v, 2) for v in (min(a.x, b.x), a.y, max(a.x, b.x))))
        for x0, x1 in frames[i]:
            # Exactly top/bottom: no fake internal divider or duplicate closed piece.
            edges = [s for s in horizontal if abs(s[0]-x0) < .04 and abs(s[2]-x1) < .04]
            assert len(edges) == 2, (i+1, x0, x1, edges)
        spans = [s for b in page.get_text('dict')['blocks'] if b['type'] == 0 for l in b['lines'] for s in l['spans']]
        for s in spans:
            x0, y0, x1, y1 = s['bbox']
            assert x0 >= 19.95 and y0 >= 19.95 and x1 <= 460.05 and y1 <= 220.05, (i, s)
        if i in (3, 4, 5):
            assert sum(s['text'] == '2' and abs(s['bbox'][0]-20) < .04 for s in spans) == 1
        if i == 0: assert any(s['text'] == 'D_00' and s['flags'] & 16 for s in spans)
        page.get_pixmap(matrix=pymupdf.Matrix(1.5, 1.5)).save(out / f'page-{i+1}.png')
    expected_images = {2: ((38, 108, 68, 123), assets[0]), 5: ((44, 172, 68, 184), assets[1])}
    assert [len(p.get_image_info()) for p in doc] == [0, 0, 1, 0, 0, 1, 0]
    for i, (bounds, original) in expected_images.items():
        info = doc[i].get_image_info(xrefs=True)[0]
        approx(info['bbox'], bounds)
        extracted = Image.open(io.BytesIO(doc.extract_image(info['xref'])['image'])).convert('RGB')
        source = Image.open(io.BytesIO(original)).convert('RGB')
        assert source.size == extracted.size and source.tobytes() == extracted.tobytes()
print('Recursive tables passed: seven pages, nested merge borders/columns, ordered cell flows, image pixels/bounds, note targets and DOCX merge/assets')
if args.office:
    with pymupdf.open(out / 'lo' / 'recursive-tables.pdf') as doc:
        whole = ''.join(p.get_text() for p in doc)
        for token in tokens: assert whole.count(token) == 1, token
        assert sum(len(p.get_image_info()) for p in doc) == 2
        for i, page in enumerate(doc):
            for block in page.get_text('dict')['blocks']:
                for line in block.get('lines', []):
                    for span in line['spans']:
                        x0, y0, x1, y1 = span['bbox']
                        assert x0 >= 19.95 and y0 >= 19.95 and x1 <= 460.05 and y1 <= 220.05, (i, span)
            page.get_pixmap(matrix=pymupdf.Matrix(1.5, 1.5)).save(out / 'lo' / f'page-{i+1}.png')
        print(f'LibreOffice observation: {len(doc)} pages; all content and two images within bounds. '
              'Visual inspection shows note tables flattened; this is not a note-table visual fidelity pass.')
