"""Optional independent structure, asset, geometry and raster QA for admission."""
import argparse
import base64
import io
import json
from pathlib import Path

from docx import Document
from PIL import Image, ImageDraw
from pypdf import PdfReader
import pymupdf

parser = argparse.ArgumentParser()
parser.add_argument('--directory', type=Path, default=Path('output/continuous-admission'))
parser.add_argument('--office', action='store_true')
a = parser.parse_args()


def nodes(n):
    if isinstance(n, dict):
        if 'type' in n:
            yield n
        for value in n.values():
            yield from nodes(value)
    elif isinstance(n, list):
        for value in n:
            yield from nodes(value)


def rgba(node):
    return Image.open(io.BytesIO(base64.b64decode(node['attrs']['src'].split(',', 1)[1]))).convert('RGBA')


def close(actual, expected):
    assert len(actual) == len(expected) and all(abs(float(x)-float(y)) < .03 for x, y in zip(actual, expected)), (actual, expected)


def contact(files, name):
    thumbs = []
    for file in files:
        with pymupdf.open(file) as doc:
            for page in doc:
                pix = page.get_pixmap(matrix=pymupdf.Matrix(1, 1))
                frame = Image.new('RGB', (320, 430), 'white')
                frame.paste(Image.frombytes('RGB', (pix.width, pix.height), pix.samples), (10, 25))
                ImageDraw.Draw(frame).text((10, 6), f'{file.stem} / {page.number+1}', fill='black')
                thumbs.append(frame)
    sheet = Image.new('RGB', (320*4, 430*((len(thumbs)+3)//4)), '#cccccc')
    for i, thumb in enumerate(thumbs):
        sheet.paste(thumb, (i % 4*320, i//4*430))
    sheet.save(a.directory/name)


report = {}
for name, pagecount in [('fixed-first', 3), ('minimum-first', 3), ('long-note', 4), ('body-floor', 3)]:
    source = json.loads((a.directory/(name+'.sf')).read_text())
    assert source == json.loads((a.directory/(name+'-docx.sf')).read_text())
    original = rgba(next(n for n in nodes(source) if n['type'] == 'image'))
    word = Document(a.directory/(name+'.docx'))
    assert word.core_properties.title == source['attrs']['metadata']['title']
    assert len(word.sections) == 3
    assert any(r.target_ref == 'https://example.com/admission' for r in word.part.rels.values() if r.reltype.endswith('/hyperlink'))
    for i, section in enumerate(word.sections):
        close((section.page_width.pt, section.page_height.pt, section.left_margin.pt, section.right_margin.pt, section.header_distance.pt, section.footer_distance.pt), (300, 400, 30, 30, 12, 12))
        if i:
            assert section.start_type == 0  # OOXML continuous survives native pagination policy.
    reader = PdfReader(a.directory/(name+'.pdf'), strict=True)
    assert len(reader.pages) == pagecount
    first_fixed = 2 if name == 'minimum-first' else 1
    min_page = 1 if name == 'minimum-first' else (3 if name == 'long-note' else 2)
    second = 3 if name == 'long-note' else 2
    expected = {'FIXED BODY': first_fixed, 'BIG' if name == 'body-floor' else 'MIN BODY': min_page, 'SHARED BODY': second, 'THIRD NEXT': pagecount, 'MIN FIRST': min_page, 'THIRD HEADER': pagecount}
    placements = []
    alltext = '\n'.join(p.extract_text() for p in reader.pages)
    with pymupdf.open(a.directory/(name+'.pdf')) as doc:
        for text, want_page in expected.items():
            found = [(p.number+1, rect) for p in doc for rect in p.search_for(text)]
            assert len(found) == 1 and found[0][0] == want_page, (name, text, found)
            rect = found[0][1]
            lower = 11.99 if text in ('MIN FIRST', 'THIRD HEADER') else 23.99
            assert 29.99 <= rect.x0 < rect.x1 <= 270.01 and lower <= rect.y0 < rect.y1 <= 376.01, (name, text, rect)
        assert 'MIN DEFAULT' not in alltext and 'THIRD FIRST' not in alltext
        for p in doc:
            images = p.get_image_info(xrefs=True)
            fixed_page = p.number+1 == first_fixed or name == 'long-note' and p.number == 1
            assert len(images) == ((1 if name == 'body-floor' else 2) if fixed_page else 0)
            for i, im in enumerate(images):
                expected_rect = (30, 12, 130, 352 if name == 'body-floor' else 382) if i == 0 else (30, 10, 130, 380)
                close(im['bbox'], expected_rect)
                placements.append((p.number+1, list(im['bbox'])))
        # Fixed backgrounds must leave foreground body black after independent rendering.
        p = doc[first_fixed-1]
        pix = p.get_pixmap(matrix=pymupdf.Matrix(2, 2))
        rect = p.search_for('FIXED BODY')[0]
        pixels = [pix.pixel(x, y) for y in range(int(rect.y0*2), int(rect.y1*2)) for x in range(int(rect.x0*2), int(rect.x1*2))]
        assert sum(max(pixel) < 12 for pixel in pixels) > 10
    note_count = 32 if name == 'long-note' else 1
    for i in range(note_count):
        text = f'OLD NOTE {i:02d}'
        assert alltext.count(text) == 1
        pages = [j+1 for j, p in enumerate(reader.pages) if text in p.extract_text()]
        assert pages[0] < second, (name, 'old note escaped its frame', text, pages)
    uris, notes = [], []
    for i, page in enumerate(reader.pages):
        for image in page.images:
            actual = image.image.convert('RGBA')
            assert actual.size == original.size and actual.tobytes() == original.tobytes()
        for ref in page.get('/Annots', []):
            annotation = ref.get_object()
            if '/A' in annotation:
                uris.append(str(annotation['/A']['/URI']))
            else:
                target = next(j+1 for j, p in enumerate(reader.pages) if p.indirect_reference.idnum == annotation['/Dest'][0].idnum)
                notes.append((i+1, target))
    assert notes == [(1, 1)] and uris == ['https://example.com/admission']
    recovered = json.loads((a.directory/(name+'-pdf.sf')).read_text())
    assert len(recovered['attrs']['pages']) == pagecount
    images = [n for n in nodes(recovered) if n['type'] == 'image']
    assert len(images) == len(placements)
    for node in images:
        actual = rgba(node)
        assert actual.size == original.size and actual.tobytes() == original.tobytes()
        assert node['attrs']['source']['format'] == 'pdf' and node['attrs']['source']['quad']
    result = {'native_pages': pagecount, 'native_placements': placements, 'note_targets': notes, 'notes_preserved': note_count, 'foreground_pixels': 'black over fixed background'}
    if a.office:
        producer = PdfReader(a.directory/'office'/(name+'.pdf'), strict=True)
        text = '\n'.join(p.extract_text() for p in producer.pages)
        for token in ['FIXED BODY', 'BIG' if name == 'body-floor' else 'MIN BODY', 'SHARED BODY', 'THIRD NEXT'] + [f'OLD NOTE {i:02d}' for i in range(note_count)]:
            assert text.count(token) == 1, (name, token, text)
        for page in producer.pages:
            for image in page.images:
                actual = image.image.convert('RGBA')
                assert actual.size == original.size and actual.tobytes() == original.tobytes()
        with pymupdf.open(a.directory/'office'/(name+'.pdf')) as doc:
            result['office_pages'] = len(doc)
            result['office_placements'] = [(p.number+1, list(im['bbox'])) for p in doc for im in p.get_image_info()]
        result['office_texts'] = [p.extract_text() for p in producer.pages]
    report[name] = result
contact([a.directory/(name+'.pdf') for name in report], 'native-contact.png')
if a.office:
    contact([a.directory/'office'/(name+'.pdf') for name in report], 'office-contact.png')
regression = {}
for old in sorted(a.directory.glob('parent-*.pdf')):
    name = old.stem.removeprefix('parent-')
    current = a.directory/f'current-{name}.pdf'
    if not current.exists():
        continue
    before, after = PdfReader(old, strict=True), PdfReader(current, strict=True)
    assert len(before.pages) == len(after.pages)
    for p, q in zip(before.pages, after.pages):
        assert p.get_contents().get_data() == q.get_contents().get_data(), name
    assert (a.directory/f'parent-{name}.docx').read_bytes() == (a.directory/f'current-{name}.docx').read_bytes()
    regression[name] = {'page_streams_exact': len(before.pages), 'docx_package': 'exact'}
for name in list(report):
    cli = a.directory/f'{name}-cli.pdf'
    if not cli.exists():
        continue
    assert (a.directory/f'{name}.sf').read_bytes() == (a.directory/f'{name}-cli-docx.sf').read_bytes()
    before, after = PdfReader(a.directory/f'{name}.pdf', strict=True), PdfReader(cli, strict=True)
    assert len(before.pages) == len(after.pages)
    for p, q in zip(before.pages, after.pages):
        assert p.get_contents().get_data() == q.get_contents().get_data()
    recovered = json.loads((a.directory/f'{name}-cli-pdf.sf').read_text())
    assert len(recovered['attrs']['pages']) == len(before.pages)
    assert len([n for n in nodes(recovered) if n['type'] == 'image']) == len(report[name]['native_placements'])
    report[name]['four_cli_paths'] = 'exact DOCX SF; matching PDF page streams and recovery page/image counts'
if regression:
    report['_parent_regression'] = regression
(a.directory/'independent.json').write_text(json.dumps(report, indent=2)+'\n')
print(json.dumps(report, indent=2))
