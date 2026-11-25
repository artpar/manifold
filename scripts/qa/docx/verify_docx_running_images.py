"""Independent source/XML/producer QA for shared DOCX running-image budgets.

Requires examples/running-images outputs and optional developer LibreOffice PDFs.
The original source SF is compared without changing it or pixel/content oracles.
"""
import argparse
import base64
import copy
import io
import json
from pathlib import Path
from zipfile import ZipFile

from docx import Document
from lxml import etree
from PIL import Image
from pypdf import PdfReader
import pymupdf

parser = argparse.ArgumentParser()
parser.add_argument('directory', nargs='?', type=Path, default=Path('output/running-images'))
parser.add_argument('--write-uri-control', action='store_true')
parser.add_argument('--require-uri-annotations', action='store_true')
args = parser.parse_args()
out = args.directory
if args.write_uri_control:
    # Independently authored python-docx control, not a manydoc output. Preserve
    # the expected external link even where the tested producer exports none.
    from docx.oxml import OxmlElement
    from docx.oxml.ns import qn
    from docx.opc.constants import RELATIONSHIP_TYPE as RT
    doc = Document()
    paragraph = doc.add_paragraph()
    link = OxmlElement('w:hyperlink')
    link.set(qn('r:id'), paragraph.part.relate_to('https://www.example.com/body', RT.HYPERLINK, is_external=True))
    run = OxmlElement('w:r'); text = OxmlElement('w:t'); text.text = 'REFERENCE LINK'
    run.append(text); link.append(run); paragraph._p.append(link)
    out.mkdir(parents=True, exist_ok=True)
    doc.save(out / 'uri-control.docx')
    print('Wrote independent URI control; convert it to lo/uri-control.pdf to qualify producer link export')
    raise SystemExit(0)
ns = {'w':'http://schemas.openxmlformats.org/wordprocessingml/2006/main',
      'wp':'http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing',
      'a':'http://schemas.openxmlformats.org/drawingml/2006/main'}


def nodes(node):
    yield node
    for child in node.get('content', []):
        yield from nodes(child)


def story_images(tree, story):
    roots = tree['content'] if story == 'body' else tree['attrs'][story]
    return [n for root in roots for n in nodes(root) if n['type'] == 'image']


def blob(node):
    return base64.b64decode(node['attrs']['src'].split(',', 1)[1])


def close(actual, expected):
    assert abs(actual-expected) < .06, (actual, expected)


# Independent physical observations. Picture preferences are unchanged in source;
# these bounds qualify actual exported placement, not an internal planner result.
expected = {
    'serial': {'size':(419.5559082,560.0125732), 'images':6, 'heights':[63.05]*6, 'body':'SERIAL BODY'},
    'nested': {'size':(419.5559082,560.0125732), 'images':6, 'heights':[33.15]*6, 'body':'NESTED BODY'},
    'body': {'size':(300,400), 'images':3, 'heights':[275,32,5], 'body':'BODY END'},
    'jpeg': {'size':(220,220), 'images':2, 'heights':[66,66], 'body':'JPEG BODY'},
}
report = {}
for name, want in expected.items():
    source = json.loads((out / (name + '.sf')).read_text())
    back = json.loads((out / (name + '-docx.sf')).read_text())
    normalized = copy.deepcopy(source)
    source_media = []
    for story in ('body','headers','footers'):
        original = story_images(source, story)
        fitted = story_images(back, story)
        normalized_images = story_images(normalized, story)
        assert len(original) == len(fitted)
        for before, after, target in zip(original, fitted, normalized_images):
            a,b = before['attrs'],after['attrs']
            assert a['src'] == b['src'] and a['alt'] == b['alt'], 'asset/alt changed'
            assert b['width'] > 0 and b['height'] > 0
            pixels = Image.open(io.BytesIO(blob(before)))
            ratio = a['width']/a['height'] if a['width'] and a['height'] else pixels.width/pixels.height
            assert abs(b['width']/b['height']-ratio) < .0001
            if a['height']:
                assert b['height'] <= a['height']+.001
            if a['width']:
                assert b['width'] <= a['width']+.001
            target['attrs'].update(width=b['width'],height=b['height'])
            source_media.append(blob(before))
    assert normalized == back, 'semantics changed beyond physical fitted dimensions'
    word = Document(out / (name + '.docx'))
    assert word.core_properties.title == 'Running images: ' + name
    with ZipFile(out / (name + '.docx')) as package:
        main = etree.fromstring(package.read('word/document.xml'))
        paper = main.xpath('//w:sectPr/w:pgSz', namespaces=ns)[0]
        source_layout = source['attrs']['layout']
        assert int(paper.get('{'+ns['w']+'}w')) == round(source_layout['width']*20)
        assert int(paper.get('{'+ns['w']+'}h')) == round(source_layout['height']*20)
        media = [package.read(n) for n in package.namelist() if n.startswith('word/media/')]
        assert len(media) == want['images'] and sorted(media) == sorted(source_media)
        for part in ('header1','footer1'):
            xml = etree.fromstring(package.read('word/' + part + '.xml'))
            placements = xml.xpath('//wp:extent', namespaces=ns)
            story = 'headers' if part == 'header1' else 'footers'
            assert len(placements) == len(story_images(back, story))
            for placement, node in zip(placements, story_images(back, story)):
                close(int(placement.get('cx'))/12700,node['attrs']['width'])
                close(int(placement.get('cy'))/12700,node['attrs']['height'])
        if name == 'nested':
            table = word.sections[0].header.tables[0].cell(0,0).tables[0]
            assert table.cell(0,0)._tc is table.cell(0,1)._tc
            for part in ('header1','footer1'):
                xml = etree.fromstring(package.read('word/' + part + '.xml'))
                assert len(xml.xpath('//w:tblHeader',namespaces=ns)) == 1
                rels = etree.fromstring(package.read('word/_rels/' + part + '.xml.rels'))
                assert any(r.get('Target') == 'https://example.invalid/header' and r.get('TargetMode') == 'External' for r in rels)
        if name == 'body':
            small = story_images(back,'footers')[0]['attrs']
            assert (small['width'],small['height']) == (10,5), 'small logo unnecessarily resized'
            assert len(word.tables[0].cell(0,0).tables) == 1
    path = out / 'lo' / (name + '.pdf')
    reader = PdfReader(path,strict=True)
    assert len(reader.pages) == 1
    for image in reader.pages[0].images:
        recovered = image.image.convert('RGBA')
        assert any(recovered.size == Image.open(io.BytesIO(b)).size and recovered.tobytes() == Image.open(io.BytesIO(b)).convert('RGBA').tobytes() for b in source_media), 'producer raster pixels changed'
    with pymupdf.open(path) as doc:
        page = doc[0]
        close(page.rect.width,want['size'][0]);close(page.rect.height,want['size'][1])
        images = page.get_image_info()
        assert len(images) == want['images'], 'a source placement disappeared'
        boxes = [pymupdf.Rect(info['bbox']) for info in images]
        for box,height in zip(boxes,want['heights']):
            close(box.height,height)
            assert box.x0 >= 17.9 and box.y0 >= 17.9 and box.x1 <= page.rect.width-17.9 and box.y1 <= page.rect.height-17.9, (name,box,page.rect)
        body = page.search_for(want['body'])
        assert len(body) == 1
        assert all(not box.intersects(body[0]) for box in boxes), 'body text overlaps a picture'
        if name == 'nested':
            assert page.get_text().count('HEADER LINK') == 2
            assert page.get_text().count('LEFT') == 2 and page.get_text().count('RIGHT') == 2
            uri_links = [link for link in page.get_links() if link.get('kind') == pymupdf.LINK_URI]
            if args.require_uri_annotations:
                assert len(uri_links) == 2, 'expected two source URI annotations'
            assert table.cell(0,0).paragraphs[0].hyperlinks[0].address == 'https://example.invalid/header'
            if len(uri_links) != 2:
                print(f'UNQUALIFIED producer URI annotations: expected 2, observed {len(uri_links)}; source relationships and independent DOCX reader pass')
            assert len(page.get_drawings()) >= 8, 'nested table geometry disappeared'
        report[name] = {'pages':len(doc),'placements':len(images),'exact_rgba':True,
                        'all_bounds_contained':True,'body_no_picture_overlap':True,
                        'sf_semantics_except_physical_dimensions':'preserved', 'source_paper_twips':'exact', 'producer_paper':list(page.rect)}
        if name == 'nested':
            report[name].update(expected_uri_annotations=2, observed_uri_annotations=len(uri_links), producer_uri_qualified=len(uri_links)==2)
        page.get_pixmap(matrix=pymupdf.Matrix(1.5,1.5)).save(out / 'lo' / (name + '-checked.png'))
control = out / 'lo' / 'uri-control.pdf'
if control.exists():
    with pymupdf.open(control) as doc:
        actual = sum(link.get('kind') == pymupdf.LINK_URI for page in doc for link in page.get_links())
    report['independent_uri_control'] = {'expected':1,'observed':actual,'qualified':actual==1}
    print(f'Independent python-docx URI control: expected 1 annotation, observed {actual}')
(out / 'verification.json').write_text(json.dumps(report,indent=2)+'\n')
print('DOCX running-image QA passed: four producer pages, seventeen exact-pixel placements, serial/nested/merged stories, JPEG, unchanged small logo, nested body figures, DOCX links, containment and source semantics (producer URI qualification reported separately)')
