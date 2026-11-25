"""Qualify placement separately from explicit PDF Note ownership (QA only)."""
import argparse
from collections import Counter
import hashlib
import gzip
import json
from pathlib import Path
import pymupdf
from pypdf import PdfReader

parser = argparse.ArgumentParser()
parser.add_argument('--source', type=Path, default=Path('output/note-inference/tagged-guide.pdf'))
parser.add_argument('--sf', type=Path, default=Path('output/note-inference/guide-kind.sf.gz'))
parser.add_argument('--uninferred', type=Path, default=Path('output/note-inference/guide-kind-uninferred.sf.gz'))
parser.add_argument('--render-dir', type=Path)
parser.add_argument('--historical-sf', type=Path)
args = parser.parse_args()
assert hashlib.sha256(args.source.read_bytes()).hexdigest() == '8185a30793869379ea4c2d82f214a930368b3fa7520faba61df38c52a502335e'

def load(path):
    with gzip.open(path, 'rt') as stream:
        return json.load(stream)

def normalize(value):
    # Mutate the loaded tree; avoid duplicating this large complete source SF.
    if isinstance(value, list):
        for v in value:
            normalize(v)
    elif isinstance(value, dict):
        for k, v in value.items():
            if k == 'kind' and v == 'footnote':
                value[k] = 'endnote'
            else:
                normalize(v)

current, uninferred = load(args.sf), load(args.uninferred)
notes = current['attrs']['notes']
assert [(n['id'], n['kind'], n['kind_inferred']) for n in notes] == [
    ('NOTE_1', 'footnote', True), ('NOTE_2', 'footnote', True)]
assert [n['kind'] for n in uninferred['attrs']['notes']] == ['endnote', 'endnote']
# Keep all fields, glyphs, sources, marks, links, tables and assets in this oracle.
# Only the documented placement selection and its diagnostic message may change.
a, b = current, uninferred
for document in (a, b):
    for d in document['attrs']['diagnostics']:
        if d['code'] == 'pdf_note_kind_inferred':
            d['message'] = '<placement policy>'
# Check the physical notes first; normalize only after their kind assertions below.

reader = PdfReader(args.source)
root = reader.trailer['/Root']['/StructTreeRoot']
assert any(isinstance(k, dict) for k in root['/ParentTree']['/Kids'])
assert any(d['code'] == 'pdf_parent_tree_direct_child' for d in current['attrs']['diagnostics'])
pdf = pymupdf.open(args.source)
for n in notes:
    # Element IDs are independently checked by verify_note_inference.py. Here
    # inspect original painted rulings, source labels and physical page frames.
    label = n['label_source']
    page = pdf[label['page'] - 1]
    left = min(p['x'] for p in label['quad'])
    top = min(p['y'] for p in label['quad'])
    em = label['font_size']
    assert top >= page.rect.height * .6
    lines = [d['rect'] for d in page.get_drawings() if d['rect'].height <= 3]
    assert any(r.x0 <= left + em and r.x1 >= left + 4*em and
               top - 4*em <= r.y0 <= r.y1 <= top + 1 for r in lines)
    print(n['id'], 'page', label['page'], ': source separator/label bounds and inferred footnote kind pass')
normalize(a)
normalize(b)
assert a == b, 'placement changed unrelated structured semantics'
print('Complete SF equality after only placement-kind/diagnostic normalization passes')

if args.render_dir:
    for path in (args.render_dir/'native.pdf', args.render_dir/'word-render/model.pdf'):
        output = pymupdf.open(path)
        assert len(output) == 1, (path, 'unexpected reflow pages')
        page = output[0]
        words = page.get_text('words')
        text = page.get_text()
        for token in ('BODY', 'MIDDLE', 'AFTER', 'NOTE BODY', 'LIST ITEM', 'TABLE CELL'):
            assert token in text, (path, token)
        main = page.search_for('BODY')[0]
        note = page.search_for('NOTE BODY')[0]
        assert main.y1 < page.rect.height*.3 and note.y0 > page.rect.height*.6
        assert all(0 <= w[0] <= w[2] <= page.rect.width and
                   0 <= w[1] <= w[3] <= page.rect.height for w in words)
        images = page.get_images()
        assert len(images) == 1
        pix = pymupdf.Pixmap(output, images[0][0])
        assert pix.width == 2 and pix.height == 1 and pix.n == 3
        assert pix.samples == bytes((255,0,0,0,0,255)), (path, 'raster changed')
        links = [l for l in page.get_links() if l.get('kind') == pymupdf.LINK_GOTO
                 and l.get('page') == 0 and l.get('to').y > page.rect.height*.6]
        assert len(links) >= 2, (path, 'repeated note targets absent')
        print(path, ': same-page footnote placement, bounds, repeated navigation and exact raster pass')

if args.historical_sf:
    def walk(nodes):
        for node in nodes:
            yield node
            yield from walk(node.get('content', []))
    def painted_records(document):
        labels = {n['id']: n['label'] for n in document['attrs'].get('notes', [])}
        records, synthetic = Counter(), Counter()
        nodes = list(walk(document['content']))
        for note in document['attrs'].get('notes', []):
            nodes.extend(walk(note['content']))
            source = note.get('label_source')
            if source and note['label']:
                records[(source['page'], source['order'], note['label'])] += 1
        for node in nodes:
            if node['type'] not in ('text', 'note_reference'):
                continue
            text = node.get('text') if node['type'] == 'text' else node['attrs'].get('label', labels[node['attrs']['id']])
            source = next((m['attrs'] for m in node.get('marks', []) if m['type'] == 'source'), None)
            if source and text:
                key = (source['page'], source['order'], text)
                records[key] += 1
                if source.get('precision') == 'synthetic_separator' and source.get('evidence') == 'tagged_paragraph_word_gap':
                    synthetic[key] += 1
        return records, synthetic
    old, synthetic = painted_records(load(args.historical_sf))
    now, _ = painted_records(current)
    omitted = old - now
    assert not (now - old), 'unexpected new historical text records'
    assert sum(omitted.values()) == 56
    assert not (omitted - synthetic), 'a painted record was lost'
    print('Historical comparison: all 56 omitted records are inferred synthetic word gaps; no painted text record omitted')
