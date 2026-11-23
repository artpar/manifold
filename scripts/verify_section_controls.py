"""Optional independent checks for authored control/empty-section expectations."""
import argparse
import json
import zipfile
from pathlib import Path
from lxml import etree
from docx import Document
from PIL import Image, ImageDraw
from pypdf import PdfReader
import pymupdf

parser = argparse.ArgumentParser()
parser.add_argument('--directory', type=Path, default=Path('output/section-controls'))
parser.add_argument('--office', action='store_true')
a = parser.parse_args()
expected = json.loads((a.directory/'expected.json').read_text())
ns = {'w': 'http://schemas.openxmlformats.org/wordprocessingml/2006/main'}
W = '{'+ns['w']+'}'


def nodes(n):
    if isinstance(n, dict):
        if 'type' in n:
            yield n
        for v in n.values():
            yield from nodes(v)
    elif isinstance(n, list):
        for v in n:
            yield from nodes(v)


def story_token(owner, role):
    if owner == '-':
        return ''
    name, variant = owner.split('_', 1)
    return name+'_'+role+'_'+variant


def contact(files, stem):
    frames = []
    for f in files:
        with pymupdf.open(f) as doc:
            for page in doc:
                pix = page.get_pixmap(matrix=pymupdf.Matrix(.7, .7))
                frame = Image.new('RGB', (300, 310), 'white')
                frame.paste(Image.frombytes('RGB', (pix.width, pix.height), pix.samples), (5, 20))
                ImageDraw.Draw(frame).text((5, 4), f'{f.stem} / {page.number+1}', fill='black')
                frames.append(frame)
    # Chunk the contact sheets so their text remains inspectable.
    for start in range(0, len(frames), 12):
        batch = frames[start:start+12]
        sheet = Image.new('RGB', (1200, 310*((len(batch)+3)//4)), '#cccccc')
        for i, frame in enumerate(batch):
            sheet.paste(frame, (i % 4*300, i//4*310))
        sheet.save(a.directory/f'{stem}-{start//12+1}.png')


report = {}
for name, pages in expected.items():
    source = json.loads((a.directory/(name+'.sf')).read_text())
    assert source == json.loads((a.directory/(name+'-docx.sf')).read_text())
    sections = source['content']
    body_tokens = [run['text'] for s in sections for block in s.get('content', []) for run in block.get('content', []) if run['type'] == 'text']
    note_tokens = [n['text'] for note in source['attrs'].get('notes', []) for n in nodes(note['content']) if n['type'] == 'text']
    wanted_kinds = {n['kind'] for n in source['attrs'].get('notes', [])}
    word = Document(a.directory/(name+'.docx'))
    assert len(word.sections) == len(sections)
    assert word.core_properties.title == source['attrs']['metadata']['title']
    layout = source['attrs']['layout']
    for word_section, source_section in zip(word.sections, sections):
        layout = source_section.get('attrs', {}).get('layout', layout)
        margin = layout.get('margin', 36)
        edges = layout.get('margins', dict(top=margin, right=margin, bottom=margin, left=margin))
        got = (word_section.page_width.pt, word_section.page_height.pt, word_section.top_margin.pt, word_section.right_margin.pt, word_section.bottom_margin.pt, word_section.left_margin.pt, word_section.header_distance.pt, word_section.footer_distance.pt)
        want = (layout['width'], layout['height'], edges['top'], edges['right'], edges['bottom'], edges['left'], layout.get('header_distance', 18), layout.get('footer_distance', 18))
        assert all(abs(x-y) < .03 for x, y in zip(got, want)), (name, got, want)
    relationships = {r.target_ref for r in word.part.rels.values() if r.reltype.endswith('/hyperlink')}
    assert relationships == {'https://example.com/controls/'+t for t in body_tokens}
    with zipfile.ZipFile(a.directory/(name+'.docx')) as package:
        xml = etree.fromstring(package.read('word/document.xml'))
        props = xml.xpath('.//w:sectPr', namespaces=ns)
        assert len(props) == len(sections)
        for source_section, p in zip(sections, props):
            attrs = source_section.get('attrs', {})
            assert p.find('w:type', ns).get(W+'val') == attrs.get('start', 'nextPage')
            for kind in wanted_kinds:
                pr = p.find('w:'+kind+'Pr', ns)
                assert pr is not None
                assert pr.find('w:numFmt', ns).get(W+'val') == 'decimal'
                assert pr.find('w:numRestart', ns).get(W+'val') == 'continuous'
            for role in ('headers', 'footers'):
                singular = role[:-1]
                bindings = {n.get(W+'type') for n in p.findall('w:'+singular+'Reference', ns)}
                assert bindings == set(attrs.get(role, {})), (name, role, bindings, attrs.get(role))
        # Explicit controls use standard pageBreakBefore plus checked metadata;
        # ordinary conditional properties use that same OOXML element directly.
        controls = sum(n['type'] == 'page_break' for s in sections for n in nodes(s.get('content', [])))
        conditional = sum(n.get('attrs', {}).get('pageBreakBefore', False) for s in sections for n in nodes(s.get('content', [])))
        assert len(xml.xpath('.//w:pPr/w:pageBreakBefore', namespaces=ns)) == controls+conditional
    reader = PdfReader(a.directory/(name+'.pdf'), strict=True)
    assert len(reader.pages) == len(pages), (name, len(reader.pages), len(pages))
    alltext = '\n'.join(p.extract_text() for p in reader.pages)
    note_targets, uri_pages = [], []
    native_spans = {}
    tracked = set(body_tokens+note_tokens+['Endnotes']) | {owner+'_'+role+'_'+variant for owner in 'ABC' for role in ('HEAD', 'FOOT') for variant in ('FIRST', 'EVEN', 'DEFAULT')}
    with pymupdf.open(a.directory/(name+'.pdf')) as doc:
        for i, (p, want) in enumerate(zip(reader.pages, pages)):
            for block in doc[i].get_text('dict')['blocks']:
                for line in block.get('lines', []):
                    for span in line['spans']:
                        if span['text'] in tracked:
                            key = (i+1, span['text'])
                            assert key not in native_spans
                            native_spans[key] = (span['origin'], span['size'], span['bbox'])
            tokens = want.split()
            owners = tokens[0].split('/') if tokens else ['-', '-']
            if len(owners) == 1:
                owners *= 2
            wanted_stories = {story_token(owners[0], 'HEAD'), story_token(owners[1], 'FOOT')} - {''}
            text = p.extract_text()
            for owner in 'ABC':
                for role in ('HEAD', 'FOOT'):
                    for variant in ('FIRST', 'EVEN', 'DEFAULT'):
                        token = owner+'_'+role+'_'+variant
                        assert text.count(token) == (1 if token in wanted_stories else 0), (name, i+1, token, text)
            for token in body_tokens:
                assert text.count(token) == tokens[1:].count(token), (name, i+1, token, text)
            expected_size = (400, 300) if name == 'empty-parity-canvas' and i >= 2 else (300, 400)
            assert (float(p.mediabox.width), float(p.mediabox.height)) == expected_size
            if not want:
                assert not text.strip() and not p.get('/Annots')
                pix = doc[i].get_pixmap()
                assert set(pix.samples) == {255}, (name, 'nonblank parity filler')
            for ref in p.get('/Annots', []):
                annotation = ref.get_object()
                if '/A' in annotation:
                    uri_pages.append((i+1, str(annotation['/A']['/URI'])))
                else:
                    target = next(j+1 for j, page in enumerate(reader.pages) if page.indirect_reference.idnum == annotation['/Dest'][0].idnum)
                    note_targets.append((i+1, target))
            if name == 'empty-parity-canvas' and i == 0:
                assert abs(doc[i].search_for('A_FOOT_FIRST')[0].x0-30) < .03
            if name == 'empty-final-endnote' and i == 2:
                rect = doc[i].search_for('ENDNOTE_CONTENT')[0]
                assert rect.x0 >= 70 and rect.y0 >= 60
    for token in note_tokens:
        assert alltext.count(token) == 1, (name, token, alltext)
    assert sorted(uri for _, uri in uri_pages) == sorted('https://example.com/controls/'+t for t in body_tokens)
    if name == 'note-controls':
        assert note_targets == [(1, 1)]
        assert all(next(i+1 for i, p in enumerate(reader.pages) if t in p.extract_text()) <= 2 for t in note_tokens)
    if name == 'empty-final-endnote':
        assert note_targets == [(1, 3)]
    if name == 'global-numbering':
        assert note_targets == [(1, 1), (1, 3), (2, 2), (2, 3)]
    recovered = json.loads((a.directory/(name+'-pdf.sf')).read_text())
    assert len(recovered['attrs']['pages']) == len(pages)
    assert not any(n['type'] == 'image' for n in nodes(recovered))
    assert sum(n['type'] == 'page_break' for n in nodes(recovered)) == len(pages)-1
    recovered_spans = {}
    for node in nodes(recovered):
        if node['type'] != 'text' or node['text'] not in tracked:
            continue
        marks = {m['type']: m.get('attrs', {}) for m in node.get('marks', [])}
        src = marks['source']
        assert src['format'] == 'pdf' and len(src['quad']) == 4 and src['baseline']
        key = (src['page'], node['text'])
        assert key in native_spans and key not in recovered_spans, (name, key)
        origin, size, bbox = native_spans[key]
        baseline = src['baseline'][0]
        assert abs(baseline['x']-origin[0]) < .03 and abs(baseline['y']-origin[1]) < .03, (name, key, baseline, origin)
        quad = src['quad']
        bounds = (min(p['x'] for p in quad), min(p['y'] for p in quad), max(p['x'] for p in quad), max(p['y'] for p in quad))
        assert all(abs(x-y) < .03 for x, y in zip(bounds, bbox)), (name, key, bounds, bbox)
        assert abs(marks['textStyle']['fontSize']-size) < .03
        if node['text'] in body_tokens:
            assert marks['link']['href'] == 'https://example.com/controls/'+node['text']
        recovered_spans[key] = True
    assert set(recovered_spans) == set(native_spans)
    if wanted_kinds:
        assert any(d['code'] == 'pdf_link_action' for d in recovered['attrs']['diagnostics'])

    result = {'native_pages': len(pages), 'note_targets': note_targets, 'body_uri_pages': uri_pages, 'source_docx_sf': 'exact', 'source_note_count': len(source['attrs'].get('notes', [])), 'recovered_spans_independently_positioned': len(recovered_spans)}
    if a.office:
        producer = PdfReader(a.directory/'office'/(name+'.pdf'), strict=True)
        text = '\n'.join(p.extract_text() for p in producer.pages)
        for token in body_tokens+note_tokens:
            assert text.count(token) == 1, (name, token, text)
        result['office_pages'] = len(producer.pages)
        result['office_texts'] = [p.extract_text() for p in producer.pages]
    report[name] = result
contact([a.directory/(name+'.pdf') for name in expected], 'native-contact')
if a.office:
    contact([a.directory/'office'/(name+'.pdf') for name in expected], 'office-contact')
regression = {}
for name in expected:
    cli = a.directory/f'{name}-cli.pdf'
    if cli.exists():
        assert (a.directory/f'{name}.sf').read_bytes() == (a.directory/f'{name}-cli-docx.sf').read_bytes()
        before, after = PdfReader(a.directory/f'{name}.pdf', strict=True), PdfReader(cli, strict=True)
        assert len(before.pages) == len(after.pages)
        for p, q in zip(before.pages, after.pages):
            assert p.get_contents().get_data() == q.get_contents().get_data()
        back = json.loads((a.directory/f'{name}-cli-pdf.sf').read_text())
        assert len(back['attrs']['pages']) == len(before.pages)
        report[name]['four_cli_paths'] = 'passed; exact DOCX SF and PDF page streams'
    parent = a.directory/f'parent-{name}.pdf'
    if not parent.exists():
        continue
    before, after = PdfReader(parent, strict=True), PdfReader(a.directory/f'{name}.pdf', strict=True)
    assert len(before.pages) == len(after.pages)
    for p, q in zip(before.pages, after.pages):
        assert p.get_contents().get_data() == q.get_contents().get_data()
    old, new = a.directory/f'parent-{name}.docx', a.directory/f'{name}.docx'
    changed = []
    with zipfile.ZipFile(old) as op, zipfile.ZipFile(new) as np:
        assert sorted(op.namelist()) == sorted(np.namelist())
        for part in op.namelist():
            if op.read(part) != np.read(part):
                changed.append(part)
                assert part == 'word/document.xml'
                original, updated = etree.fromstring(op.read(part)), etree.fromstring(np.read(part))
                added = updated.xpath('.//w:sectPr/w:footnotePr|.//w:sectPr/w:endnotePr', namespaces=ns)
                assert added
                for pr in added:
                    pr.getparent().remove(pr)
                assert etree.tostring(original) == etree.tostring(updated)
    if report[name]['source_note_count'] == 0:
        assert not changed and old.read_bytes() == new.read_bytes()
    else:
        assert changed == ['word/document.xml']
    old_sf = a.directory/f'parent-{name}-docx.sf'
    if old_sf.exists():
        old_value = json.loads(old_sf.read_text())
        assert old_value['attrs'].pop('diagnostics') == [{'code': 'note_default_numbering', 'message': 'Endnote format omitted; OOXML decimal default used (Word may display lowerRoman)'}]
        assert old_value == json.loads((a.directory/f'{name}.sf').read_text())
    regression[name] = {'page_streams_exact': len(before.pages), 'docx_changed_parts': changed}
if regression:
    report['_parent_97095a1'] = regression
flat_before, flat_after = a.directory/'parent-flat-notes.docx', a.directory/'current-flat-notes.docx'
if flat_before.exists() and flat_after.exists():
    assert flat_before.read_bytes() == flat_after.read_bytes()
    report['_flat_note_writer'] = 'whole DOCX package exact'
(a.directory/'independent.json').write_text(json.dumps(report, indent=2)+'\n')
print(json.dumps(report, indent=2))
