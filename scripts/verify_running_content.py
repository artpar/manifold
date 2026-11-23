"""Optional independent rich-running-content QA; runtime remains pure Go.

Run after go run ./examples/running-content. --office also records the known
oversized-DOCX-footer producer gap; it does not qualify that case as preserved.
"""
import argparse
import base64
import io
import json
import re
from pathlib import Path
from zipfile import ZipFile

from docx import Document
from lxml import etree
from PIL import Image
from pypdf import PdfReader
import pymupdf

parser = argparse.ArgumentParser()
parser.add_argument('directory', nargs='?', type=Path, default=Path('output/running-content'))
parser.add_argument('--office', action='store_true')
args = parser.parse_args()
out = args.directory


def nodes(tree):
    yield tree
    for child in tree.get('content', []):
        yield from nodes(child)


def rgba(node):
    return Image.open(io.BytesIO(base64.b64decode(node['attrs']['src'].split(',', 1)[1]))).convert('RGBA')


def close(actual, expected):
    assert len(actual) == len(expected) and all(abs(a-b) < .03 for a, b in zip(actual, expected)), (actual, expected)


ns = {'w': 'http://schemas.openxmlformats.org/wordprocessingml/2006/main',
      'r': 'http://schemas.openxmlformats.org/officeDocument/2006/relationships',
      'a': 'http://schemas.openxmlformats.org/drawingml/2006/main'}
report = {}
for name, count in [('letterhead', 4), ('tight', 1)]:
    sf = json.loads((out / (name + '.sf')).read_text())
    docx_sf = json.loads((out / (name + '-docx.sf')).read_text())
    sf_equal = sf == docx_sf
    if name == 'letterhead':
        assert sf_equal, 'SF/DOCX semantics changed'
    else:
        # Existing rendered-format contract: image preferences become physical
        # fitted sizes on reimport. Preserve the authored SF and verify this exact
        # normalization separately, rather than claiming a lossless round trip.
        expected = json.loads(json.dumps(sf))
        for part in ('headers', 'footers'):
            assert expected['attrs'][part][0]['attrs']['height'] == 1000
            assert expected['attrs'][part][0]['attrs']['width'] == 0
            expected['attrs'][part][0]['attrs'].update(width=21.5, height=172)
        assert expected == docx_sf, 'unexpected DOCX normalization beyond fitted dimensions'
    original = rgba(next(n for root in sf['attrs']['headers'] for n in nodes(root) if n['type'] == 'image'))
    recovered = json.loads((out / (name + '-pdf.sf')).read_text())
    assert len(recovered['attrs']['pages']) == count
    recovered_images = [n for n in nodes(recovered) if n['type'] == 'image']
    assert len(recovered_images) == (9 if name == 'letterhead' else 2)
    for n in recovered_images:
        im = rgba(n)
        assert im.size == original.size and im.tobytes() == original.tobytes()
        source = n['attrs']['source']
        assert source['format'] == 'pdf' and 1 <= source['page'] <= count and len(source['quad']) == 4
    assert not recovered['attrs'].get('headers') and not recovered['attrs'].get('footers'), 'untagged PDF must not claim authored running roles'
    reader = PdfReader(out / (name + '.pdf'), strict=True)
    assert len(reader.pages) == count
    all_images, all_words, positions, uris = [], [], [], []
    for i, page in enumerate(reader.pages):
        for im in page.images:
            image = im.image.convert('RGBA')
            assert image.size == original.size and image.tobytes() == original.tobytes(), 'RGBA pixels changed'
        for ref in page.get('/Annots', []):
            a = ref.get_object()
            if '/A' in a:
                uris.append((i, a['/A']['/URI']))
            else:
                target = next(j for j, p in enumerate(reader.pages) if p.indirect_reference.idnum == a['/Dest'][0].idnum)
                assert i == target == 0 and 156 <= 560-float(a['/Dest'][3]) <= 510.75
        # Render each page independently before inspecting text/image geometry.
        with pymupdf.open(out / (name + '.pdf')) as native:
            p = native[i]
            pix = p.get_pixmap(matrix=pymupdf.Matrix(1.5, 1.5))
            pix.save(out / f'{name}-page-{i+1}.png')
            if name == 'letterhead':
                # Check the actual raster too, beyond operator/image presence.
                assert pix.pixel(90, 60) == (200, 30, 50)
                assert pix.pixel(81, 795) == (200, 30, 50)
            words = p.get_text('words')
            all_words.extend(words)
            text = p.get_text()
            images = p.get_image_info(xrefs=True)
            all_images.extend(images)
            positions.extend((i+1, v['bbox']) for v in images)
            if name == 'letterhead':
                spans = [s for b in p.get_text('dict')['blocks'] for l in b.get('lines', []) for s in l['spans']]
                title = next(s for s in spans if s['text'] == 'LETTERHEAD')
                link = next(s for s in spans if s['text'] == 'HEADER LINK')
                assert (title['font'], title['size'], title['color']) == ('GoMono-Bold', 10, 0)
                assert (link['font'], link['size'], link['color']) == ('GoRegular', 9, 0x004499)
                for token in ['LETTERHEAD', 'HEADER LINK', 'DEPARTMENT', 'ENGINEERING', 'REFERENCE', 'FOOTER LINK', 'FOOTER ITEM']:
                    assert text.count(token) == 1, (i, token)
                close(images[0]['bbox'], (54, 30, 134, 70))
                close(images[-1]['bbox'], (48, 522, 88, 542))
                rects = [tuple(item[1]) for path in p.get_drawings() for item in path['items'] if item[0] == 're']
                want = [(48,24,210,84),(210,24,372,48.5),(210,48.5,372,84),(48,84,372,148),
                        (54,90,366,112),(54,112,210,134),(210,112,366,134)]
                for actual, expected in zip(rects[:7], want):
                    close(actual, expected)
                for w in words:
                    # Explicit running bounds; every other word is body/note content.
                    if w[1] < 148 or w[1] >= 510.7:
                        continue
                    assert 155.9 <= w[1] <= w[3] <= 510.8 and 47.9 <= w[0] <= w[2] <= 372.1, (i, w)
                if i == 1:
                    assert len(images) == 3
                    close(images[1]['bbox'], (66,479.55,90,491.55))
                else:
                    assert len(images) == 2
                if i == 3:
                    assert p.search_for('ROW-20') and not p.search_for('ROW-19')
            else:
                assert text.strip() == 'ONLY BODY' and len(images) == 2
                close(images[0]['bbox'], (20,10,31.1625,99.3))
                close(images[1]['bbox'], (20,122.7,31.1625,212))
                assert all(107.2 <= w[1] <= w[3] <= 122.8 for w in words)
    recovered_positions = []
    for node in recovered_images:
        source = node['attrs']['source']
        quad = source['quad']
        bbox = (min(v['x'] for v in quad), min(v['y'] for v in quad),
                max(v['x'] for v in quad), max(v['y'] for v in quad))
        recovered_positions.append((source['page'], bbox))
    for (actual_page, actual), (expected_page, expected) in zip(sorted(recovered_positions), sorted(positions)):
        assert actual_page == expected_page
        close(actual, expected)
    if name == 'letterhead':
        tables = [n for n in nodes(recovered) if n['type'] == 'table']
        assert tables and all(n['attrs']['source']['structure'] == 'inferred' for n in tables)
        assert uris == [(i, u) for i in range(4) for u in ('https://example.invalid/header', 'https://example.invalid/footer')]
        text = '\n'.join(p.extract_text() for p in reader.pages)
        for i in range(30):
            assert len(re.findall(rf'ROW-{i:02d}(?!\d)', text)) == 1
            assert len(re.findall(rf'VALUE-{i:02d}', text)) == 1
        for token in ('BODY BEGIN', 'BODY END', 'NOTE STAYS ABOVE THE FOOTER', 'NOTE END'):
            assert text.count(token) == 1, token
        # Same running raster is reused across stories/pages, retaining alpha.
        assert len({im['xref'] for im in all_images}) == 1
        word = Document(out / 'letterhead.docx')
        header = word.sections[0].header.tables[0]
        assert header.cell(0,0)._tc is header.cell(1,0)._tc
        assert header.cell(2,0)._tc is header.cell(2,1)._tc
        assert len(header.cell(2,0).tables) == 1
        assert len(word.tables[0].rows) == 31
    with ZipFile(out / (name + '.docx')) as z:
        for part in ('header1', 'footer1'):
            root = etree.fromstring(z.read('word/' + part + '.xml'))
            assert len(root.xpath('//a:blip', namespaces=ns)) == 1
            if name == 'letterhead':
                rels = etree.fromstring(z.read('word/_rels/' + part + '.xml.rels'))
                by_id = {n.get('Id'): n for n in rels}
                link = root.xpath('//w:hyperlink', namespaces=ns)[0].get('{'+ns['r']+'}id')
                assert by_id[link].get('Target') == 'https://example.invalid/' + part[:-1]
                assert by_id[link].get('TargetMode') == 'External'
        if name == 'letterhead':
            root = etree.fromstring(z.read('word/header1.xml'))
            assert len(root.xpath('//w:tblHeader', namespaces=ns)) == 2
            assert len(root.xpath('//w:vMerge', namespaces=ns)) == 2
            assert len(root.xpath('//w:gridSpan[@w:val="2"]', namespaces=ns)) == 2
            footer = etree.fromstring(z.read('word/footer1.xml'))
            assert len(footer.xpath('//w:numPr', namespaces=ns)) == 1
            note = etree.fromstring(z.read('word/footnotes.xml'))
            assert 'NOTE END' in ''.join(note.itertext()) and len(note.xpath('//a:blip', namespaces=ns)) == 1
    report[name] = {'native_pages': count, 'raster_placements': len(all_images), 'exact_rgba': True, 'running_font_size_color': 'passed' if name == 'letterhead' else 'not applicable',
                    'sf_docx_equal': sf_equal, 'pdf_recovery_positions': len(recovered_images)}
    if args.office:
        path = out / 'lo' / (name + '.pdf')
        with pymupdf.open(path) as office:
            images = [im for p in office for im in p.get_image_info()]
            text = '\n'.join(p.get_text() for p in office)
            report[name]['office_pages'] = len(office)
            report[name]['office_raster_placements'] = len(images)
            if name == 'letterhead':
                assert len(office) == 2 and len(images) == 5
                for token in ('LETTERHEAD','HEADER LINK','FOOTER LINK','FOOTER ITEM'):
                    assert text.count(token) == 2
                for i in range(30):
                    assert len(re.findall(rf'ROW-{i:02d}(?!\d)', text)) == 1
                for token in ('BODY BEGIN','BODY END','NOTE STAYS ABOVE THE FOOTER','NOTE END'):
                    assert text.count(token) == 1
            else:
                report[name]['office_expected_raster_placements'] = 2
                report[name]['office_qualified'] = len(images) == 2
                if len(images) != 2:
                    print('OPEN: oversized DOCX running images: LibreOffice paints one of two source placements; native PDF paints both')
        for i in range(report[name]['office_pages']):
            with pymupdf.open(path) as fresh:
                fresh[i].get_pixmap(matrix=pymupdf.Matrix(1.5,1.5)).save(out / 'lo' / f'{name}-fresh-{i+1}.png')
(out / 'verification.json').write_text(json.dumps(report, indent=2) + '\n')
print('Native running-content QA passed: five pages, eleven exact RGBA placements, merged/nested geometry, URI/note destinations, source immutability and declared SF/DOCX semantics (tight image preferences normalize to fitted sizes); PDF recovery retains positions without claiming authored running roles')
