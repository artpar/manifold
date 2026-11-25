"""Independent untagged geometric structure oracle and generated-output QA.

This targeted source is hand placed, not created by manydoc. Its deliberately
shuffled paint order differs from the intended column/row order. Optional QA
dependencies are never required by the Go library or CLI.
"""
from pathlib import Path
from collections import Counter
from zipfile import ZipFile
import argparse
import json
import subprocess
import unicodedata
from pypdf import PdfReader
from docx import Document
import pymupdf

parser=argparse.ArgumentParser();parser.add_argument('directory',nargs='?',type=Path,default=Path('output/pdf-inference'))
out=parser.parse_args().directory;out.mkdir(parents=True,exist_ok=True)
def stream(content,attrs=''):
    if isinstance(content,str):content=content.encode('ascii')
    return f'<< {attrs} /Length {len(content)} >>\nstream\n'.encode()+content+b'\nendstream'
def text(font,size,x,y,value):
    value=value.replace('\\','\\\\').replace('(','\\(').replace(')','\\)')
    return f'BT /{font} {size} Tf 1 0 0 1 {x} {y} Tm ({value}) Tj ET\n'
title='Quarterly review and supporting measurements'
source1=text('F',10,330,676,'RIGHT continues in the same paragraph.')
source1+=text('F',10,40,640,'LEFT second paragraph is separated.')
source1+=text('B',22,40,750,title)
source1+=text('F',10,40,676,'LEFT continues in the same paragraph.')
source1+=text('F',10,330,690,'RIGHT begins here; then continues')
source1+=text('F',10,40,690,'LEFT starts with the first paragraph')
source1+=text('F',10,330,640,'RIGHT second paragraph is separated.')
values=[['Item','Count','Price'],['Apples','12','$5.00'],['Pears','34','$9.25']]
source2=''
for row,cells in enumerate(values):
    for col in (2,1,0):source2+=text('F',10,[40,220,340][col],650-row*14,cells[col])
for x,y,value in [(40,550,'3. First numbered item'),(54,536,'continued at the hanging indent.'),(40,522,'4. Second numbered item'),(58,508,'- Nested one'),(58,494,'- Nested two')]:source2+=text('F',10,x,y,value)
source2+='q 40 0 0 30 400 210 cm /Im Do Q\n'
resources='/Resources << /Font << /F 5 0 R /B 6 0 R >> /XObject << /Im 9 0 R >> >>'
objects=[
    '<< /Type /Catalog /Pages 2 0 R >>',
    '<< /Type /Pages /Kids [3 0 R 4 0 R] /Count 2 >>',
    f'<< /Type /Page /Parent 2 0 R /MediaBox [0 0 600 800] {resources} /Contents 7 0 R /Annots [10 0 R] >>',
    f'<< /Type /Page /Parent 2 0 R /MediaBox [0 0 600 800] {resources} /Contents 8 0 R >>',
    '<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>',
    '<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica-Bold >>',
    stream(source1),stream(source2),
    stream(bytes([255,0,0]),'/Type /XObject /Subtype /Image /Width 1 /Height 1 /BitsPerComponent 8 /ColorSpace /DeviceRGB'),
    '<< /Type /Annot /Subtype /Link /Rect [329 687 590 700] /A << /S /URI /URI (https://example.invalid/right) >> >>',
]
pdf=bytearray(b'%PDF-1.7\n');offsets=[0]
for i,obj in enumerate(objects,1):
    if isinstance(obj,str):obj=obj.encode('ascii')
    offsets.append(len(pdf));pdf.extend(f'{i} 0 obj\n'.encode()+obj+b'\nendobj\n')
xref=len(pdf);pdf.extend(f'xref\n0 {len(offsets)}\n0000000000 65535 f \n'.encode())
for offset in offsets[1:]:pdf.extend(f'{offset:010d} 00000 n \n'.encode())
pdf.extend(f'trailer\n<< /Size {len(offsets)} /Root 1 0 R >>\nstartxref\n{xref}\n%%EOF\n'.encode())
source=out/'untagged.pdf';source.write_bytes(pdf)
r=PdfReader(source,strict=True);assert len(r.pages)==2 and '/StructTreeRoot' not in r.trailer['/Root']
with pymupdf.open(source) as doc:
    original=''.join(p.get_text() for p in doc)
    words=doc[0].get_text('words')
    assert min(w[0] for w in words if w[4]=='RIGHT')==330
    assert min(w[0] for w in words if w[4]=='LEFT')==40
    assert sum(len(p.get_image_info()) for p in doc)==1
    for p in doc:
        for b in p.get_text('dict')['blocks']:
            for line in b.get('lines',[]):
                x0,y0,x1,y1=line['bbox'];assert x0>=0 and y0>=0 and x1<=p.rect.width and y1<=p.rect.height
    for i,p in enumerate(doc):p.get_pixmap(matrix=pymupdf.Matrix(1.4,1.4)).save(out/f'source-{i+1}.png')
sf_path=out/'recovered.sf'
subprocess.run(['bin/manifold','extract','-format','sf','-o',str(sf_path),str(source)],check=True)
sf=json.loads(sf_path.read_text());bs=sf['content'];assert len(sf['attrs']['pages'])==2
plain=lambda n:n.get('text','')+''.join(plain(c) for c in n.get('content',[]))
assert [n['type'] for n in bs]==['heading','paragraph','paragraph','paragraph','paragraph','page_break','table','ordered_list','image']
assert bs[0]['attrs']['level']==1 and plain(bs[0])==title
assert [plain(n) for n in bs[1:5]]==[
    'LEFT starts with the first paragraph LEFT continues in the same paragraph.',
    'LEFT second paragraph is separated.',
    'RIGHT begins here; then continues RIGHT continues in the same paragraph.',
    'RIGHT second paragraph is separated.',
]
assert all(n['attrs']['source']['structure']=='inferred' for n in bs if n['type'] not in ('page_break','image'))
assert bs[1]['attrs']['source']['quad'][1]['x']<300 and bs[3]['attrs']['source']['quad'][0]['x']==330
assert any(m['type']=='link' and m['attrs']['href']=='https://example.invalid/right' for m in bs[3]['content'][0]['marks'])
table=bs[6];assert [[plain(c) for c in row['content']] for row in table['content']]==values
assert all(c['attrs']['colspan']==1 and c['attrs']['rowspan']==1 for row in table['content'] for c in row['content'])
listing=bs[7];assert listing['attrs']['order']==3 and len(listing['content'])==2
first,second=listing['content'];assert plain(first)=='First numbered item continued at the hanging indent.'
assert first['attrs']['source']['label']=='3.' and second['attrs']['source']['label']=='4.'
assert second['content'][1]['type']=='bullet_list' and [plain(i) for i in second['content'][1]['content']]==['Nested one','Nested two']
first_source=next(m['attrs'] for m in first['content'][0]['content'][0]['marks'] if m['type']=='source')
assert first_source['precision']=='partial_text_span_bounds'
labels=[]
def walk(n):
    if n['type']=='list_item':labels.append(n['attrs']['source']['label'])
    for c in n.get('content',[]):walk(c)
walk(sf)
def check_advances(n):
    if n['type']=='text':
        source=next(m['attrs'] for m in n['marks'] if m['type']=='source')
        if source.get('precision')=='font_metrics' and source.get('font') in ('Helvetica','Helvetica-Bold'):
            baseline=source['baseline'];font='helv' if source['font']=='Helvetica' else 'hebo'
            expected=pymupdf.get_text_length(n['text'],fontname=font,fontsize=source['font_size'])
            assert abs(baseline[1]['x']-baseline[0]['x']-expected)<.001,(n['text'],baseline,expected)
    for c in n.get('content',[]):check_advances(c)
check_advances(sf)
normalized=lambda s:Counter(''.join(unicodedata.normalize('NFKC',s).split()))
assert normalized(plain(sf)+''.join(labels))==normalized(original)
codes={d['code'] for d in sf['attrs']['diagnostics']};assert {'pdf_structure_inferred','pdf_list_inferred','pdf_table_inferred'}<=codes
raw_path=out/'paint-order.sf'
subprocess.run(['bin/manifold','extract','-pdf-skip-inference','-format','sf','-o',str(raw_path),str(source)],check=True)
raw=json.loads(raw_path.read_text());assert plain(raw['content'][0])=='RIGHT continues in the same paragraph.'
assert 'pdf_structure_inferred' not in {d['code'] for d in raw['attrs']['diagnostics']}
for ext in ('docx','pdf'):subprocess.run(['bin/manifold','convert','-allow-lossy','-pdf-font-map','Helvetica=Go','-o',str(out/f'reflow.{ext}'),str(sf_path)],check=True)
word=Document(out/'reflow.docx');assert len(word.tables)==1 and len(word.inline_shapes)==1
assert [[c.text for c in row.cells] for row in word.tables[0].rows]==values
with ZipFile(out/'reflow.docx') as z:assert b'https://example.invalid/right' in z.read('word/_rels/document.xml.rels')
def rendered(n):
    if n['type'] in ('bullet_list','ordered_list'):
        start=n.get('attrs',{}).get('order',1)
        return ''.join(('•' if n['type']=='bullet_list' else f'{start+i}.')+rendered(c) for i,c in enumerate(n['content']))
    return n.get('text','')+''.join(rendered(c) for c in n.get('content',[]))
flow=PdfReader(out/'reflow.pdf',strict=True);assert len(flow.pages)==2
assert {str(a.get_object()['/A']['/URI']) for p in flow.pages for a in p.get('/Annots',[]) if a.get_object().get('/A',{}).get('/S')=='/URI'}=={'https://example.invalid/right'}
with pymupdf.open(out/'reflow.pdf') as doc:
    assert normalized(''.join(p.get_text() for p in doc))==normalized(rendered(sf))
    assert sum(len(p.get_image_info()) for p in doc)==1
    for i,p in enumerate(doc):
        for b in p.get_text('dict')['blocks']:
            for line in b.get('lines',[]):
                x0,y0,x1,y1=line['bbox'];assert x0>=53.9 and y0>=53.9 and x1<=p.rect.width-53.9 and y1<=p.rect.height-53.9
        p.get_pixmap(matrix=pymupdf.Matrix(1.4,1.4)).save(out/f'reflow-{i+1}.png')
print('Untagged inference passed: independent source geometry, column/paragraph order, heading, table cells, nested lists/starts/labels, source spans, URI, image, page boundaries, literal coverage, opt-out, generated DOCX and two-page PDF content/links/assets/bounds')
