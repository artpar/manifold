"""Independent source operators, editable rich label, Word and PDF paint audit."""
from pathlib import Path
from zipfile import ZipFile
import base64,io,json
from lxml import etree
from docx import Document
from pypdf import PdfReader
from pypdf.generic import ContentStream
from PIL import Image
import pymupdf

root=Path('output/rich-note-labels')
ns={'w':'http://schemas.openxmlformats.org/wordprocessingml/2006/main','md':'urn:manydoc:metadata:1'}
def walk(nodes):
    for n in nodes:
        yield n
        yield from walk(n.get('content',[]))
def editable(v):
    if isinstance(v,list):return [editable(x) for x in v if not(isinstance(x,dict) and x.get('type')=='source')]
    if isinstance(v,dict):
        out={k:editable(x) for k,x in v.items() if k not in ('source','label_source')}
        return {k:x for k,x in out.items() if not(k in ('attrs','marks') and not x)}
    return v

def label_ranges(path,expected):
    Document(path);found=0
    with ZipFile(path) as z:
        for part in ('footnotes','endnotes'):
            name=f'word/{part}.xml'
            if name not in z.namelist():continue
            tree=etree.fromstring(z.read(name))
            for start in tree.xpath('//w:bookmarkStart[@md:noteBodyContent="1"]',namespaces=ns):
                assert start.get('{'+ns['w']+'}name').startswith('mdnoterich1_')
                run=start.getnext();fragments=[]
                while run is not None and run.tag!='{'+ns['w']+'}bookmarkEnd':
                    assert run.tag in ('{'+ns['w']+'}r','{'+ns['w']+'}hyperlink')
                    fragments.append(run);run=run.getnext()
                assert run is not None and run.get('{'+ns['w']+'}id')==start.get('{'+ns['w']+'}id') and run.get('{'+ns['md']+'}noteBodyContent')=='1'
                assert len(fragments)==2
                found+=1
    assert found==expected

reader=PdfReader('output/audit/rich-label-note.pdf');stack=[];size=None;color=(0.,0.,0.);paint=[]
for operands,operator in ContentStream(reader.pages[0]['/Contents'],reader).operations:
    if operator==b'BDC':stack.append(operands[1].get('/MCID') if isinstance(operands[1],dict) else None)
    elif operator==b'BMC':stack.append(None)
    elif operator==b'EMC':stack.pop()
    elif operator==b'Tf':size=float(operands[1])
    elif operator==b'rg':color=tuple(float(v) for v in operands)
    elif operator==b'Tj' and stack and stack[-1]==2:paint.append((str(operands[0]),size,color))
assert paint==[('*',10.,(0.,0.,0.)),('1',8.,(1.,0.,0.))]
sf=json.loads((root/'source.sf').read_text());word=json.loads((root/'word.sf').read_text())
note=sf['attrs']['notes'][0];rich=note['body_label_content']
assert len(rich)==2 and [n['text'] for n in rich]==[p[0] for p in paint]
assert 'body_label' not in note and 'body_label_marks' not in note
for i,node in enumerate(rich):
    style=next(m['attrs'] for m in node['marks'] if m['type']=='textStyle')
    assert style['fontSize']==paint[i][1] and style['color']==('#000000' if i==0 else '#FF0000') and style['fontFamily']=='Courier'
    assert any(m['type']=='strong' for m in node['marks'])
    source=next(m['attrs'] for m in node['marks'] if m['type']=='source')
    assert source['role']=='Lbl' and source['quad'] and source['page']==1
assert editable(sf['attrs']['notes'])==editable(word['attrs']['notes']) and editable(sf['content'])==editable(word['content'])
image=next(n for n in walk(note['content']) if n['type']=='image')
im=Image.open(io.BytesIO(base64.b64decode(image['attrs']['src'].split(',',1)[1]))).convert('RGB')
assert im.size==(2,1) and [im.getpixel((i,0)) for i in range(2)]==[(255,0,0),(0,0,255)]
label_ranges(root/'word.docx',1)
for name in ('native.pdf','lo/word.pdf'):
    doc=pymupdf.open(root/name);assert len(doc)==1
    spans=[s for b in doc[0].get_text('dict')['blocks'] for l in b.get('lines',[]) for s in l['spans']]
    black=[s for s in spans if s['text']=='*' and abs(s['size']-10)<.001 and s['color']==0]
    red=[s for s in spans if s['text']=='1' and abs(s['size']-8)<.001 and s['color']==0xFF0000]
    body=[s for s in spans if s['text'].strip()=='NOTE BODY']
    assert len(black)==len(red)==len(body)==1
    assert black[0]['bbox'][0]<red[0]['bbox'][0]<body[0]['bbox'][0]
    assert red[0]['bbox'][2]+3<=body[0]['bbox'][0]
    doc[0].get_pixmap(matrix=pymupdf.Matrix(1.2,1.2)).save(root/('native.png' if name=='native.pdf' else 'word.png'))
print('Independent source MCID operators, individual 10pt black/8pt red styles/geometry, Word range/semantic round trip, native/Word label-before-body paint and exact note pixels pass')

before=json.loads((root/'example/notes.sf').read_text());after=json.loads((root/'example/word.sf').read_text())
assert before['content']==after['content'] and before['attrs']['notes']==after['attrs']['notes']
label_ranges(root/'example/notes.docx',3)
for filename in ('notes.pdf','lo/notes.pdf'):
    doc=pymupdf.open(root/'example'/filename);assert len(doc)==2
    count=0;uri_count=0;internal_count=0
    for i,page in enumerate(doc):
        for block in page.get_text('dict')['blocks']:
            for line in block.get('lines',[]):
                for span in line['spans']:
                    if span['text']=='N' and abs(span['size']-9)<.001 and span['color']==0x203040:count+=1
        for link in page.get_links():
            if link.get('uri')=='https://example.org/label':uri_count+=1
            if link.get('nameddest')=='rich-return':internal_count+=1
        page.get_pixmap(matrix=pymupdf.Matrix(1.4,1.4)).save(root/f'example/{"native" if filename=="notes.pdf" else "word"}-{i+1}.png')
    assert count==3,(filename,count)
    if filename=='notes.pdf':assert uri_count==internal_count==3
print('Sectioned example: exact rich-run SF/Word semantics; all three styled labels visible; native per-fragment URI and named backlinks pass')
print('LibreOffice installed-font substitution and note-label hyperlink omission remain separate reader limits; bookmark-range text/styles are verified')

mixed=pymupdf.open(root/'mixed.pdf');assert len(mixed)==2
for i,page in enumerate(mixed):
    spans=[s for b in page.get_text('dict')['blocks'] for line in b.get('lines',[]) for s in line['spans']]
    body=next(s for s in spans if s['text'].startswith('BODY_'))
    marks=[s for s in spans if 'DejaVu' in s['font']]
    assert len(marks)==2 and marks[0]['text']=='بب' and marks[1]['text']=='A'
    assert abs(marks[0]['size']-10)<.001 and abs(marks[1]['size']-9)<.001 and marks[1]['color']==0xFF0000
    if i==0:assert max(s['bbox'][2] for s in marks)<body['bbox'][0]-3
    else:assert min(s['bbox'][0] for s in marks)>body['bbox'][2]+3
    page.get_pixmap(matrix=pymupdf.Matrix(2,2)).save(root/f'mixed-{i+1}.png')
print('Independent mixed Arabic/Latin body marks: supplied DejaVu, separate 10pt/9pt paint, both gutter sides and body separation pass')
for filename in ('notes.pdf','lo/notes.pdf'):
    doc=pymupdf.open(root/'example'/filename)
    print(filename,'actual page dimensions:',[tuple(page.rect)[2:] for page in doc])
