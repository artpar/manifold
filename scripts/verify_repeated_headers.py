"""Independent repeated-header content, merge, geometry, pixel and link checks."""
import argparse
import base64
import io
import json
import subprocess
from pathlib import Path
from zipfile import ZipFile

from docx import Document
from lxml import etree
from PIL import Image
from pypdf import PdfReader
import pymupdf

parser = argparse.ArgumentParser()
parser.add_argument('directory', nargs='?', type=Path, default=Path('output/repeated-headers'))
parser.add_argument('--office', type=Path)
parser.add_argument('--cli', action='store_true')
args = parser.parse_args()
out = args.directory
sf = json.loads((out / 'repeated-headers.sf').read_text())
rows = sf['content'][0]['content']
assert [r.get('attrs', {}).get('repeatHeader', False) for r in rows] == [True, True, False, False, False]
assert rows[0]['content'][0]['attrs'] == {'colspan': 2, 'rowspan': 2}
assert rows[2]['content'][0]['attrs'] == {'colspan': 2, 'rowspan': 3}
header_image = base64.b64decode(rows[0]['content'][0]['content'][1]['attrs']['src'].split(',', 1)[1])
body_image = base64.b64decode(rows[2]['content'][0]['content'][20]['attrs']['src'].split(',', 1)[1])
tokens = [*(f'SPANNING_{i:02d}' for i in range(20)), *(f'RIGHT_B_{i:02d}' for i in range(12)),
          'RIGHT_A', 'RIGHT_C', 'SPANNING_LAST', 'AFTER_GRID', 'NOTE_EARLY', 'NOTE_LATE']

def approx(a, b):
    assert len(a) == len(b) and all(abs(x-y) < .04 for x, y in zip(a, b)), (a, b)

def pixels(doc, info, source):
    actual = Image.open(io.BytesIO(doc.extract_image(info['xref'])['image'])).convert('RGB')
    expected = Image.open(io.BytesIO(source)).convert('RGB')
    assert actual.size == expected.size and actual.tobytes() == expected.tobytes()

word = Document(out / 'repeated-headers.docx')
table = word.tables[0]
assert len(table.rows) == 5 and len(table.columns) == 3
assert all(table.cell(0, 0)._tc is table.cell(r, c)._tc for r in range(2) for c in range(2))
assert all(table.cell(2, 0)._tc is table.cell(r, c)._tc for r in range(2, 5) for c in range(2))
ns = {'w': 'http://schemas.openxmlformats.org/wordprocessingml/2006/main'}
with ZipFile(out / 'repeated-headers.docx') as z:
    xml = etree.fromstring(z.read('word/document.xml'))
    flags = [len(r.xpath('./w:trPr/w:tblHeader', namespaces=ns)) for r in xml.xpath('//w:body/w:tbl/w:tr', namespaces=ns)]
    assert flags == [1, 1, 0, 0, 0]
    media = [z.read(n) for n in z.namelist() if n.startswith('word/media/')]
    assert sorted(media) == sorted([header_image, body_image])

reader = PdfReader(out / 'repeated-headers.pdf', strict=True)
assert len(reader.pages) == 5
assert reader.metadata.title == 'Repeating merged table headers'
links, uris = [], []
for i, page in enumerate(reader.pages):
    for ref in page.get('/Annots', []):
        a = ref.get_object()
        if '/Dest' in a:
            dest = next(j for j, p in enumerate(reader.pages) if p.indirect_reference.idnum == a['/Dest'][0].idnum)
            links.append((i+1, dest+1))
        if '/A' in a:
            uris.append((i+1, str(a['/A']['/URI'])))
assert links == [(1, 1), (4, 4)], links
assert uris == [(5, 'https://example.invalid/continued')], uris
with pymupdf.open(out / 'repeated-headers.pdf') as doc:
    texts = [p.get_text() for p in doc]
    whole = ''.join(texts)
    for token in tokens:
        assert whole.count(token) == 1, token
    assert [len(p.get_image_info()) for p in doc] == [1, 1, 1, 1, 2]
    for i, page in enumerate(doc):
        for name in ('HEADER_MAIN', 'HDR_TOP', 'HDR_BOTTOM'):
            found = page.search_for(name)
            assert len(found) == 1, (i, name)
        assert 'SPANNING_' in texts[i], 'orphaned header'
        approx(page.search_for('HEADER_MAIN')[0][:2], (26, 26.56))
        images = page.get_image_info(xrefs=True)
        approx(images[0]['bbox'], (26, 40, 38, 52))
        pixels(doc, images[0], header_image)
        if i == 4:
            approx(images[1]['bbox'], (26, 92, 56, 107))
            pixels(doc, images[1], body_image)
        rectangles = [item[1] for d in page.get_drawings() for item in d['items'] if item[0] == 're']
        for expected in [(20, 20, 206.6667, 72), (206.6667, 20, 300, 46), (206.6667, 46, 300, 72)]:
            assert any(all(abs(a-b) < .04 for a, b in zip(rect, expected)) for rect in rectangles), (i, expected, rectangles)
        # The merged header has no false horizontal border through its origin.
        segments = [item[1:] for d in page.get_drawings() for item in d['items'] if item[0] == 'l']
        assert not any(abs(a.y-46)<.04 and abs(b.y-46)<.04 and min(a.x,b.x)<206 for a,b in segments)
        for b in page.get_text('dict')['blocks']:
            for line in b.get('lines', []):
                for span in line['spans']:
                    x0,y0,x1,y1 = span['bbox']
                    assert x0>=19.95 and y0>=19.95 and x1<=300.05 and y1<=180.05, (i,span)
        page.get_pixmap(matrix=pymupdf.Matrix(1.7, 1.7)).save(out / f'page-{i+1}.png')
    assert 'NOTE_EARLY' in texts[0] and 'NOTE_LATE' in texts[3] and 'AFTER_GRID' in texts[4]
    spans = [s for b in doc[0].get_text('dict')['blocks'] for l in b.get('lines',[]) for s in l['spans']]
    assert any(s['text']=='SPANNING_00' and s['flags']&16 for s in spans)
print('Repeated headers passed: five pages, merged header rectangles/pixels, unique body flow, correct note destinations, DOCX header flags/merges/assets')

if args.office:
    with pymupdf.open(args.office) as doc:
        assert len(doc) == 4
        texts = [p.get_text() for p in doc]
        for token in tokens:
            assert ''.join(texts).count(token) == 1, token
        for i, page in enumerate(doc):
            for token in ('HEADER_MAIN', 'HDR_TOP', 'HDR_BOTTOM'):
                assert texts[i].count(token) == 1, (i,token)
            images = page.get_image_info(xrefs=True)
            assert len(images) == (2 if i==2 else 1)
            assert images[0]["width"] == 5 and images[0]["height"] == 50  # Observed LibreOffice raster reduction, not pixel preservation.
            if i==2: pixels(doc,images[1],body_image)
            for b in page.get_text('dict')['blocks']:
                for line in b.get('lines',[]):
                    for span in line['spans']:
                        x0,y0,x1,y1=span['bbox']
                        assert x0>=19.5 and y0>=19.5 and x1<=300.5 and y1<=180.5, (i,span)
            for im in images:
                x0,y0,x1,y1=im['bbox']
                assert x0>=19.5 and y0>=19.5 and x1<=300.5 and y1<=180.5
            page.get_pixmap(matrix=pymupdf.Matrix(1.7,1.7)).save(out / f'office-page-{i+1}.png')
    (out/'office-observation.json').write_text(json.dumps({
        'producer':'LibreOffice 7.6', 'pages':4, 'header_repetition':'passed',
        'all_body_tokens':'passed', 'all_image_placements':'passed', 'bounds':'passed',
        'exact_header_pixels':'failed: raster dimensions reduced',
        'source_dimensions':[64,640], 'rendered_dimensions':[5,50],
        'native_exact_pixels':'passed', 'docx_original_media':'passed',
        'pagination_matches_native':False,
    },indent=2)+'\n')
    print('LibreOffice containment passed: four pages repeat both header rows and retain body text/images. Exact header pixel preservation failed (64x640 reduced to 5x50); pagination differs, see office-observation.json')


if args.cli:
    for command in [
        ['extract', '-format', 'sf', '-o', str(out/'from-docx.sf'), str(out/'repeated-headers.docx')],
        ['convert', '-o', str(out/'from-docx.pdf'), str(out/'from-docx.sf')],
        ['extract', '-format', 'sf', '-o', str(out/'from-pdf.sf'), str(out/'repeated-headers.pdf')],
    ]:
        subprocess.run(['bin/manydoc', *command], check=True)
    assert json.loads((out/'from-docx.sf').read_text()) == sf
    with pymupdf.open(out/'repeated-headers.pdf') as a, pymupdf.open(out/'from-docx.pdf') as b:
        assert len(a) == len(b) == 5
        for x,y in zip(a,b):
            assert x.rect == y.rect
            assert x.get_text('dict') == y.get_text('dict')
            assert x.get_drawings() == y.get_drawings()
            assert x.get_links() == y.get_links()
            assert x.get_image_info(hashes=True) == y.get_image_info(hashes=True)
    print('CLI passed: whole SF DOCX round trip and identical PDF text/geometry/raster hashes/links; generation dates may differ')
