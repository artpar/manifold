"""Development-only independent tree and painted-boundary audit of the original PDF.

Run examples/tagged-spans first. This does not compare plain text alone, alter
the published fixtures/oracles, or assert Word-equivalent layout.
"""
import json
import argparse
import hashlib
from pathlib import Path
from pypdf import PdfReader
import pymupdf

parser = argparse.ArgumentParser()
parser.add_argument('--out',type=Path,default=Path('output/tagged-spans'))
out = parser.parse_args().out
report = json.loads((out/'tables.json').read_text())
oracle = json.loads(Path('testdata/real/mqtt-v5.0-pdf-oracle.json').read_text())
reader = PdfReader('testdata/real/mqtt-v5.0-os.pdf', strict=True)
pdf = pymupdf.open('testdata/real/mqtt-v5.0-os.pdf')
assert report['pages'] == len(reader.pages) == len(pdf) == 137
actual = {t['source']['element']: t for t in report['tables']}
assert len(actual) == len(oracle['tables']) == 90

def has_content(node):
    node = node.get_object() if hasattr(node, 'get_object') else node
    if isinstance(node, (int, float)): return True  # MCID, including zero
    if isinstance(node, list): return any(has_content(n) for n in node)
    if isinstance(node, dict):
        if '/MCID' in node or node.get('/Type') == '/OBJR': return True
        return has_content(node.get('/K', []))
    return False

def border_rectangles(page):
    rects = []
    for draw in page.get_drawings():
        for item in draw['items']:
            if item[0] == 're' and 'f' in draw['type']:
                r = item[1]
                if min(r.width, r.height) <= 3: rects.append(r)
            if item[0] == 'l' and 's' in draw['type']:
                a,b = item[1:3]; w = (draw['width'] or 0)/2
                rects.append(pymupdf.Rect(min(a.x,b.x)-w,min(a.y,b.y)-w,max(a.x,b.x)+w,max(a.y,b.y)+w))
    return rects

rects = {}
inferred = cells = origins = continuations = boundaries = 0
uncertain = []
for expected in oracle['tables']:
    table = actual[f"({expected['id']} 0 R)"]
    source = table['source']; page = source['page']
    assert source['structure'] == 'explicit' and table['rows'] == expected['rows']
    members = {}
    changed = False
    for cell in table['cells']:
        s = cell['source']; origins += 1
        assert s['structure'] == 'explicit' and s['evidence'] == 'pdf_structure_tree'
        key = (cell['row'], s['element'])
        assert key not in members
        members[key] = '/'+s['role']
        span = s.get('table_spans')
        if not span: continue
        changed = True
        assert span['evidence'] == 'painted_grid_adjacency' and 0 < span['confidence'] < 1
        q = span['bounds']; x0,y0,x1,y1 = q[0]['x'],q[0]['y'],q[2]['x'],q[2]['y']
        if page not in rects: rects[page] = border_rectangles(pdf[page-1])
        # Sample every cell side against independently decoded painted paths.
        # Tolerance is .3pt; it does not snap different column axes together.
        edges = [(x0,y0,x1,y0),(x1,y0,x1,y1),(x0,y1,x1,y1),(x0,y0,x0,y1)]
        for a,b,c,d in edges:
            for i in range(1,100):
                x,y = a+(c-a)*i/100,b+(d-b)*i/100
                assert any(r.x0-.3 <= x <= r.x1+.3 and r.y0-.3 <= y <= r.y1+.3 for r in rects[page]), (s['element'],page,x,y)
            boundaries += 1
        for ref in span.get('continuations', []):
            key = (ref['row'],ref['element']); assert key not in members
            members[key] = '/'+ref['role']; continuations += 1
            objnum = int(ref['element'][1:].split()[0])
            node = reader.get_object(objnum)
            assert node['/S'] == '/'+ref['tag'] and not has_content(node)
    if changed: inferred += 1
    if source['evidence'] == 'pdf_structure_tree_ragged_grid': uncertain.append(source['element'])
    want = {(ri,f"({c['id']} 0 R)"): c['role'] for ri,row in enumerate(expected['cells']) for c in row}
    assert members == want, expected['id']
    cells += len(members)
assert cells == 3020 and origins == 3009 and continuations == 11 and inferred == 42
assert uncertain == []
# The original paired Word source independently declares all seventeen columns
# and all 79 origin spans/texts for this same page-local PDF table.
word = json.loads(Path('testdata/real/mqtt-v5.0-oracle.json').read_text())['tables'][51]
table = actual['(12539 0 R)']
slots = [[False]*17 for _ in range(table['rows'])]
got = []
for cell in table['cells']:
    row = cell['row']; col = slots[row].index(False)
    cs,rs = max(1,cell['colspan']),max(1,cell['rowspan'])
    for rr in range(row,row+rs):
        assert col+cs <= 17 and not any(slots[rr][col:col+cs])
        slots[rr][col:col+cs] = [True]*cs
    digest = hashlib.sha256(' '.join(cell['text'].split()).encode()).hexdigest()
    got.append(dict(row=row,column=col,colspan=cs,rowspan=rs,text_sha256=digest))
assert all(all(row) for row in slots) and got == word['cells']
# Independently resolve the formerly ambiguous junctions using every filled
# rectangle, including the short pieces excluded by longitudinal-line filters.
junctions = [(433.14999,433.98999,301.249995),
             (459.310005,460.030005,301.249995),
             (511.540005,512.50002,301.249995)]
paint = border_rectangles(pdf[88])
for x0,x1,y in junctions:
    intervals = sorted((r.x0,r.x1) for r in paint if r.y0 <= y <= r.y1 and r.x1>x0 and r.x0<x1)
    reach = x0
    for a,b in intervals:
        assert a <= reach+.001, (x0,x1,y,a,reach)
        reach = max(reach,b)
    assert reach>=x1-.001
for n in (17,25,53,69,89):
    pdf[n-1].get_pixmap(matrix=pymupdf.Matrix(1,1)).save(out/f'original-{n}.png')
pdf[88].get_pixmap(matrix=pymupdf.Matrix(2,2), clip=pymupdf.Rect(65,150,550,370)).save(out/'page-89-grid.png')
result = dict(pages=137, tables=90, rows=756, explicit_cell_identities=cells,
              editable_origins=origins, retained_empty_continuations=continuations,
              inferred_tables=inferred, painted_boundaries_checked=boundaries,
              uncertain_tables=uncertain,
              word_table_51_origin_spans_and_text_hashes=len(got),
              short_painted_junctions=junctions,
              limitation='All 42 omitted-span grids recovered from painted evidence; other recovery diagnostics remain. This is not visual equivalence or full-goal completion.')
(out/'independent.json').write_text(json.dumps(result,indent=2)+'\n')
print(json.dumps(result,indent=2))
