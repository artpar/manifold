"""Independent ReportLab -> SF -> DOCX/PDF style oracle; QA dependencies only.

No source font programs are reused. Helvetica/Times deliberately map to Go;
Courier maps to Go Mono. This checks editable styles, not visual equivalence.
"""
import argparse
from pathlib import Path
from zipfile import ZipFile
import json
import subprocess
import xml.etree.ElementTree as ET

import pymupdf
from pypdf import PdfReader
from reportlab.pdfgen.canvas import Canvas

parser=argparse.ArgumentParser()
parser.add_argument('--office',type=Path,help='optional LibreOffice rendering of reflow.docx')
args=parser.parse_args()
out = Path('output/pdf-text-styles')
out.mkdir(parents=True, exist_ok=True)
source = out / 'source.pdf'
# Every token has one known source span. Integer/half-point sizes can be
# re-exported to either writer without caller-side precision normalization.
cases = [
    ('RED_HEADING', 'Helvetica-Bold', 'Helvetica', 24, 0xCC0000, True, False),
    ('BLUE_ITALIC', 'Times-Italic', 'Times', 18, 0x0033CC, False, True),
    ('GREEN_MONO', 'Courier', 'Courier', 12, 0x008800, False, False),
    ('BLACK_BODY', 'Helvetica', 'Helvetica', 10.5, 0x000000, False, False),
    ('PURPLE_BOTH', 'Courier-BoldOblique', 'Courier', 14, 0x993399, True, True),
]
c = Canvas(str(source), pagesize=(420, 500), pageCompression=1, invariant=1)
c.setTitle('Independent PDF style recovery')
for page in range(2):
    for j, (token, face, family, size, color, bold, italic) in enumerate(cases):
        c.setFont(face, size)
        c.setFillColorRGB(*[((color >> shift) & 255) / 255 for shift in (16, 8, 0)])
        c.drawString(30, 450-j*65, token+str(page+1))
    c.showPage()
c.save()
subprocess.run(['bin/manydoc', 'extract', '-pdf-skip-inference', '-format', 'sf', '-o', str(out/'styles.sf'), str(source)], check=True)
sf = json.loads((out/'styles.sf').read_text())
def runs(node):
    if node['type'] == 'text':
        yield node
    for child in node.get('content', []):
        yield from runs(child)
recovered = {n['text']: n for n in runs(sf)}
assert len(recovered) == 10
with pymupdf.open(source) as doc:
    assert len(doc) == 2
    for page_no, page in enumerate(doc):
        observed = {s['text']: s for b in page.get_text('dict')['blocks'] for l in b.get('lines', []) for s in l['spans']}
        for j, (token, face, family, size, color, bold, italic) in enumerate(cases):
            token += str(page_no+1)
            independent = observed[token]
            assert independent['font'] == face and abs(independent['size']-size) < .001 and independent['color'] == color
            n = recovered[token]
            marks = {m['type']: m.get('attrs', {}) for m in n['marks']}
            assert marks['textStyle'] == dict(fontFamily=family, fontSize=size, color=f'#{color:06X}'), (token, marks)
            assert ('strong' in marks) == bold and ('em' in marks) == italic
            evidence = marks['source']
            assert evidence['font'] == face and evidence['font_family_evidence'] == 'core_font'
            assert evidence['page'] == page_no+1 and evidence['color_space'] == 'DeviceRGB'
            assert abs(evidence['baseline'][0]['x']-independent['origin'][0]) < .001
            assert abs(evidence['baseline'][0]['y']-independent['origin'][1]) < .001
        page.get_pixmap(matrix=pymupdf.Matrix(1.3, 1.3)).save(out/f'source-{page_no+1}.png')
for ext in ('docx', 'pdf'):
    subprocess.run(['bin/manydoc', 'convert', '-allow-lossy', '-pdf-font-map', 'Helvetica=Go', '-pdf-font-map', 'Times=Go', '-pdf-font-map', 'Courier=Go Mono', '-o', str(out/f'reflow.{ext}'), str(out/'styles.sf')], check=True)
ns = {'w': 'http://schemas.openxmlformats.org/wordprocessingml/2006/main'}
w = '{'+ns['w']+'}'
with ZipFile(out/'reflow.docx') as z:
    root = ET.fromstring(z.read('word/document.xml'))
    word_runs = {''.join(r.itertext()): r for r in root.findall('.//w:r', ns) if r.find('w:t',ns) is not None}
    for page in (1, 2):
        for token, face, family, size, color, bold, italic in cases:
            props = word_runs[token+str(page)].find('w:rPr', ns)
            assert props.find('w:rFonts',ns).get(w+'ascii') == family
            assert int(props.find('w:sz',ns).get(w+'val')) == size*2
            assert props.find('w:color',ns).get(w+'val') == f'{color:06X}'
            assert (props.find('w:b',ns) is not None) == bold
            assert (props.find('w:i',ns) is not None) == italic
pdf = PdfReader(out/'reflow.pdf', strict=True)
assert len(pdf.pages) == 2 and pdf.metadata.title == 'Independent PDF style recovery'
with pymupdf.open(out/'reflow.pdf') as doc:
    for page_no, page in enumerate(doc):
        observed = {s['text']: s for b in page.get_text('dict')['blocks'] for l in b.get('lines',[]) for s in l['spans']}
        for token, face, family, size, color, bold, italic in cases:
            s = observed[token+str(page_no+1)]
            assert abs(s['size']-size) < .001 and s['color'] == color, (token,s)
            assert bool(s['flags'] & 16) == bold and bool(s['flags'] & 2) == italic
        page.get_pixmap(matrix=pymupdf.Matrix(1.3,1.3)).save(out/f'reflow-{page_no+1}.png')
print('PDF style recovery passed: 10 independently authored spans, exact family/size/RGB/bold/italic, source baselines/pages, DOCX properties, two-page mapped-font PDF and native-reader styles')

if args.office:
    with pymupdf.open(args.office) as doc:
        assert len(doc) == 2
        fonts=set()
        for page_no,page in enumerate(doc):
            spans=[s for b in page.get_text('dict')['blocks'] for l in b.get('lines',[]) for s in l['spans']]
            observed={s['text']:s for s in spans}
            fonts.update(s['font'] for s in spans)
            for token,face,family,size,color,bold,italic in cases:
                span=observed[token+str(page_no+1)]
                assert abs(span['size']-size) < .01 and span['color']==color
                assert bool(span['flags'] & 16)==bold and bool(span['flags'] & 2)==italic
            page.get_pixmap(matrix=pymupdf.Matrix(1.3,1.3)).save(out/f'office-{page_no+1}.png')
        (out/'office-observation.json').write_text(json.dumps({'fonts':sorted(fonts),'pages':len(doc),'source_font_equivalence_claimed':False},indent=2))
    print('LibreOffice DOCX rendering passed: ten tokens/sizes/colors/bold/italic on two pages; actual substituted fonts recorded separately')
